//go:build windows

package main

import (
	"errors"
	"fmt"
	"image"
	"image/color"
	"log/slog"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/kikz0r/edgesync/spikes/internal/wincom"
)

const (
	cfUnicodeTextFormat uint32 = 13
	cfDIBV5             uint32 = 17

	clipboardOpenAttempts = 20
	clipboardOpenInterval = 10 * time.Millisecond
	formatNameCapacity    = 256

	sampleText         = "EdgeSync delayed rendering spike\nLine two: é 世界 \U0001F600\n"
	sampleHTMLFragment = `<div><b>EdgeSync</b> delayed rendering <i>spike</i></div>`
	sampleRTF          = `{\rtf1\ansi\deff0 {\b EdgeSync} delayed rendering \i spike\i0.\par}`
)

var (
	user32   = windows.NewLazySystemDLL("user32.dll")
	kernel32 = windows.NewLazySystemDLL("kernel32.dll")

	procOpenClipboard              = user32.NewProc("OpenClipboard")
	procCloseClipboard             = user32.NewProc("CloseClipboard")
	procEmptyClipboard             = user32.NewProc("EmptyClipboard")
	procSetClipboardData           = user32.NewProc("SetClipboardData")
	procGetClipboardOwner          = user32.NewProc("GetClipboardOwner")
	procGetClipboardSequenceNumber = user32.NewProc("GetClipboardSequenceNumber")
	procEnumClipboardFormats       = user32.NewProc("EnumClipboardFormats")
	procGetClipboardFormatName     = user32.NewProc("GetClipboardFormatNameW")
	procSetLastError               = kernel32.NewProc("SetLastError")
	procGlobalFree                 = kernel32.NewProc("GlobalFree")
)

var (
	errWinNoCode  = errors.New("call failed without a Win32 error code")
	errNotOffered = errors.New("format is not offered by this process")
)

type delayedFormat struct {
	id       uint32
	name     string
	data     []byte
	rendered bool
}

// clipboardOffer owns the delayed formats placed on the clipboard. Every method runs on the listener's message-loop
// thread, so the fields need no locking.
type clipboardOffer struct {
	hwnd    uintptr
	delay   time.Duration
	log     *slog.Logger
	formats []*delayedFormat
}

func buildDelayedFormats(sensitive bool) ([]*delayedFormat, error) {
	text, err := textToCFUnicodeText(sampleText)
	if err != nil {
		return nil, fmt.Errorf("CF_UNICODETEXT payload: %w", err)
	}
	pngData, err := encodePNG(sampleImage())
	if err != nil {
		return nil, fmt.Errorf("PNG payload: %w", err)
	}
	dib, err := pngToDIBV5(pngData)
	if err != nil {
		return nil, fmt.Errorf("CF_DIBV5 payload: %w", err)
	}
	htmlID, err := registerFormat(wincom.FormatNameHTML)
	if err != nil {
		return nil, err
	}
	rtfID, err := registerFormat(wincom.FormatNameRTF)
	if err != nil {
		return nil, err
	}
	pngID, err := registerFormat(wincom.FormatNamePNG)
	if err != nil {
		return nil, err
	}

	formats := []*delayedFormat{
		{id: cfUnicodeTextFormat, name: "CF_UNICODETEXT", data: text},
		{id: htmlID, name: wincom.FormatNameHTML, data: htmlPayload([]byte(sampleHTMLFragment))},
		{id: rtfID, name: wincom.FormatNameRTF, data: rtfPayload([]byte(sampleRTF))},
		{id: pngID, name: wincom.FormatNamePNG, data: pngData},
		{id: cfDIBV5, name: "CF_DIBV5", data: dib},
	}
	if !sensitive {
		return formats, nil
	}
	excludeID, err := registerFormat(wincom.FormatNameExcludeFromMonitor)
	if err != nil {
		return nil, err
	}
	canIncludeID, err := registerFormat(wincom.FormatNameCanIncludeInHistory)
	if err != nil {
		return nil, err
	}
	dword := make([]byte, 4)
	return append(formats,
		&delayedFormat{id: excludeID, name: wincom.FormatNameExcludeFromMonitor, data: dword},
		&delayedFormat{id: canIncludeID, name: wincom.FormatNameCanIncludeInHistory, data: dword},
	), nil
}

func registerFormat(name string) (uint32, error) {
	id, err := wincom.RegisterFormat(name)
	if err != nil {
		return 0, err
	}
	return uint32(id), nil
}

// sampleImage is a 16x8 image whose alpha ramps from 0 to 255 left to right, so the alpha path through PNG and
// CF_DIBV5 is visible when the picture is pasted into Paint.
func sampleImage() *image.NRGBA {
	const width, height = 16, 8
	img := image.NewNRGBA(image.Rect(0, 0, width, height))
	for y := range height {
		for x := range width {
			img.SetNRGBA(x, y, color.NRGBA{R: uint8(x * 16), G: uint8(y * 32), B: 0x80, A: uint8(x * 17)})
		}
	}
	return img
}

func (o *clipboardOffer) place(formats []*delayedFormat) error {
	if err := openClipboard(o.hwnd); err != nil {
		return err
	}
	return errors.Join(o.placeOpen(formats), closeClipboard())
}

func (o *clipboardOffer) placeOpen(formats []*delayedFormat) error {
	if r, _, err := procEmptyClipboard.Call(); r == 0 {
		return winError("EmptyClipboard", err)
	}
	o.formats = formats
	for _, f := range formats {
		if err := setClipboardData(f.id, 0); err != nil {
			return fmt.Errorf("offer %s: %w", f.name, err)
		}
		o.log.Info("offered delayed format", "format", f.name, "bytes", len(f.data))
	}
	return nil
}

// render answers WM_RENDERFORMAT. It must not open the clipboard: the requester already holds it open.
func (o *clipboardOffer) render(format uint32) error {
	f := o.lookup(format)
	if f == nil {
		return fmt.Errorf("WM_RENDERFORMAT for format %d: %w", format, errNotOffered)
	}
	started := time.Now()
	time.Sleep(o.delay)
	handle, err := allocHGlobal(f.data)
	if err != nil {
		return fmt.Errorf("render %s: %w", f.name, err)
	}
	if err := setClipboardData(f.id, handle); err != nil {
		return errors.Join(fmt.Errorf("render %s: %w", f.name, err), globalFree(handle))
	}
	f.rendered = true
	o.log.Info("rendered delayed format", "format", f.name, "bytes", len(f.data),
		"delay", o.delay, "elapsed", time.Since(started))
	return nil
}

// renderAll answers WM_RENDERALLFORMATS sent before the owner window is destroyed: every format still delayed is
// placed on the clipboard so it survives the exit. The owner check follows Microsoft Learn: another application may
// have taken the clipboard before the message arrived, and its data must not be overwritten.
func (o *clipboardOffer) renderAll() error {
	pending := o.unrendered()
	if len(pending) == 0 {
		o.log.Info("WM_RENDERALLFORMATS: no delayed formats left")
		return nil
	}
	if err := openClipboard(o.hwnd); err != nil {
		return err
	}
	return errors.Join(o.renderAllOpen(pending), closeClipboard())
}

func (o *clipboardOffer) renderAllOpen(pending []*delayedFormat) error {
	if owner := getClipboardOwner(); owner != o.hwnd {
		o.log.Warn("WM_RENDERALLFORMATS: clipboard owner changed, nothing rendered",
			"owner_hwnd", fmt.Sprintf("0x%X", owner))
		return nil
	}
	var errs []error
	for _, f := range pending {
		handle, err := allocHGlobal(f.data)
		if err != nil {
			errs = append(errs, fmt.Errorf("render-all %s: %w", f.name, err))
			continue
		}
		if err := setClipboardData(f.id, handle); err != nil {
			errs = append(errs, errors.Join(fmt.Errorf("render-all %s: %w", f.name, err), globalFree(handle)))
			continue
		}
		f.rendered = true
		o.log.Info("rendered delayed format at exit", "format", f.name, "bytes", len(f.data))
	}
	return errors.Join(errs...)
}

// lost handles WM_DESTROYCLIPBOARD: another application emptied the clipboard, so the delayed formats are gone.
func (o *clipboardOffer) lost() {
	o.log.Info("delayed offer replaced by another copy", "unrendered_formats", len(o.unrendered()))
	o.formats = nil
}

func (o *clipboardOffer) lookup(format uint32) *delayedFormat {
	for _, f := range o.formats {
		if f.id == format {
			return f
		}
	}
	return nil
}

func (o *clipboardOffer) unrendered() []*delayedFormat {
	var pending []*delayedFormat
	for _, f := range o.formats {
		if !f.rendered {
			pending = append(pending, f)
		}
	}
	return pending
}

func openClipboard(hwnd uintptr) error {
	var lastErr error
	for range clipboardOpenAttempts {
		r, _, err := procOpenClipboard.Call(hwnd)
		if r != 0 {
			return nil
		}
		lastErr = err
		time.Sleep(clipboardOpenInterval)
	}
	return winError(fmt.Sprintf("OpenClipboard after %d attempts", clipboardOpenAttempts), lastErr)
}

func closeClipboard() error {
	if r, _, err := procCloseClipboard.Call(); r == 0 {
		return winError("CloseClipboard", err)
	}
	return nil
}

// setClipboardData with a zero handle places a delayed format. Success then returns NULL as well, so the thread's
// last-error code is cleared beforehand and checked afterwards.
func setClipboardData(format uint32, handle uintptr) error {
	procSetLastError.Call(0)
	r, _, err := procSetClipboardData.Call(uintptr(format), handle)
	if r == 0 && (handle != 0 || errnoOf(err) != 0) {
		return winError(fmt.Sprintf("SetClipboardData(%d)", format), err)
	}
	return nil
}

func listClipboardFormats() ([]uint32, error) {
	var formats []uint32
	var previous uintptr
	for {
		procSetLastError.Call(0)
		r, _, err := procEnumClipboardFormats.Call(previous)
		if r == 0 {
			if errnoOf(err) != 0 {
				return nil, winError("EnumClipboardFormats", err)
			}
			return formats, nil
		}
		formats = append(formats, uint32(r))
		previous = r
	}
}

func clipboardFormatName(id uint32) string {
	if name, ok := standardFormatName(id); ok {
		return name
	}
	buf := make([]uint16, formatNameCapacity)
	n, _, _ := procGetClipboardFormatName.Call(uintptr(id), uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	if n == 0 {
		return fmt.Sprintf("unnamed(0x%04X)", id)
	}
	return windows.UTF16ToString(buf[:n])
}

func standardFormatName(id uint32) (string, bool) {
	switch id {
	case 1:
		return "CF_TEXT", true
	case 2:
		return "CF_BITMAP", true
	case 7:
		return "CF_OEMTEXT", true
	case 8:
		return "CF_DIB", true
	case 13:
		return "CF_UNICODETEXT", true
	case 15:
		return "CF_HDROP", true
	case 16:
		return "CF_LOCALE", true
	case 17:
		return "CF_DIBV5", true
	}
	return "", false
}

func isSensitivityFormat(name string) bool {
	return name == wincom.FormatNameExcludeFromMonitor || name == wincom.FormatNameCanIncludeInHistory
}

func getClipboardOwner() uintptr {
	r, _, _ := procGetClipboardOwner.Call()
	return r
}

func clipboardSequenceNumber() uint32 {
	r, _, _ := procGetClipboardSequenceNumber.Call()
	return uint32(r)
}

func allocHGlobal(data []byte) (uintptr, error) {
	medium, err := wincom.NewHGlobalMedium(data)
	if err != nil {
		return 0, err
	}
	return medium.Handle, nil
}

func globalFree(handle uintptr) error {
	if r, _, err := procGlobalFree.Call(handle); r != 0 {
		return winError("GlobalFree", err)
	}
	return nil
}

func winError(name string, err error) error {
	if errno := errnoOf(err); errno != 0 {
		return fmt.Errorf("%s: %w", name, errno)
	}
	return fmt.Errorf("%s: %w", name, errWinNoCode)
}

// errnoOf extracts the GetLastError value. syscall.Proc.Call always returns a non-nil error, so a zero Errno means
// "no error code" and the call's return value alone decides success.
func errnoOf(err error) syscall.Errno {
	var errno syscall.Errno
	errors.As(err, &errno)
	return errno
}
