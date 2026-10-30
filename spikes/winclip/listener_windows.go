//go:build windows

package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	wmRenderFormat     = 0x0305
	wmRenderAllFormats = 0x0306
	wmDestroyClipboard = 0x0307
	wmClipboardUpdate  = 0x031D
	wmApp              = 0x8000
	wmShutdown         = wmApp + 1

	listenerClassName = "EdgeSyncSpikeWinclip"
	hwndMessage       = ^uintptr(2)
)

var (
	procRegisterClassEx               = user32.NewProc("RegisterClassExW")
	procUnregisterClass               = user32.NewProc("UnregisterClassW")
	procCreateWindowEx                = user32.NewProc("CreateWindowExW")
	procDestroyWindow                 = user32.NewProc("DestroyWindow")
	procDefWindowProc                 = user32.NewProc("DefWindowProcW")
	procGetMessage                    = user32.NewProc("GetMessageW")
	procDispatchMessage               = user32.NewProc("DispatchMessageW")
	procPostMessage                   = user32.NewProc("PostMessageW")
	procPostQuitMessage               = user32.NewProc("PostQuitMessage")
	procAddClipboardFormatListener    = user32.NewProc("AddClipboardFormatListener")
	procRemoveClipboardFormatListener = user32.NewProc("RemoveClipboardFormatListener")
	procGetModuleHandle               = kernel32.NewProc("GetModuleHandleW")

	activeListener  atomic.Pointer[listener]
	wndProcCallback = sync.OnceValue(func() uintptr { return syscall.NewCallback(wndProc) })
)

// wndClassEx is WNDCLASSEXW, 80 bytes on amd64 (asserted below).
type wndClassEx struct {
	Size       uint32
	Style      uint32
	WndProc    uintptr
	ClsExtra   int32
	WndExtra   int32
	Instance   uintptr
	Icon       uintptr
	Cursor     uintptr
	Background uintptr
	MenuName   *uint16
	ClassName  *uint16
	IconSm     uintptr
}

// winMsg is MSG, 48 bytes on amd64 including lPrivate (asserted below).
type winMsg struct {
	HWND     uintptr
	Message  uint32
	_        uint32
	WParam   uintptr
	LParam   uintptr
	Time     uint32
	PtX      int32
	PtY      int32
	LPrivate uint32
}

var (
	_ [unsafe.Sizeof(wndClassEx{}) - 80]struct{}
	_ [80 - unsafe.Sizeof(wndClassEx{})]struct{}
	_ [unsafe.Sizeof(winMsg{}) - 48]struct{}
	_ [48 - unsafe.Sizeof(winMsg{})]struct{}
)

// listener owns the message-only window. Its window procedure and message loop run on the thread that called
// newListener, which is locked to the OS thread for the whole run.
type listener struct {
	hwnd            uintptr
	instance        uintptr
	className       *uint16
	classRegistered bool
	offer           *clipboardOffer
	log             *slog.Logger
	err             error
}

func newListener(logger *slog.Logger, offer *clipboardOffer) (*listener, error) {
	instance, _, err := procGetModuleHandle.Call(0)
	if instance == 0 {
		return nil, winError("GetModuleHandleW", err)
	}
	className, err := windows.UTF16PtrFromString(listenerClassName)
	if err != nil {
		return nil, fmt.Errorf("window class name: %w", err)
	}
	l := &listener{instance: instance, className: className, offer: offer, log: logger}

	class := &wndClassEx{WndProc: wndProcCallback(), Instance: instance, ClassName: className}
	class.Size = uint32(unsafe.Sizeof(*class))
	atom, _, err := procRegisterClassEx.Call(uintptr(unsafe.Pointer(class)))
	if atom == 0 {
		return nil, winError("RegisterClassExW", err)
	}
	l.classRegistered = true

	activeListener.Store(l)
	hwnd, _, err := procCreateWindowEx.Call(0, uintptr(unsafe.Pointer(className)), 0, 0, 0, 0, 0, 0,
		hwndMessage, 0, instance, 0)
	if hwnd == 0 {
		return nil, errors.Join(winError("CreateWindowExW", err), l.close())
	}
	l.hwnd = hwnd
	offer.hwnd = hwnd

	if ok, _, err := procAddClipboardFormatListener.Call(hwnd); ok == 0 {
		return nil, errors.Join(winError("AddClipboardFormatListener", err), l.close())
	}
	return l, nil
}

// pump runs the message loop until the window is destroyed. Cancelling ctx posts a shutdown message to the window,
// whose handler destroys it; WM_RENDERALLFORMATS is delivered during that destruction.
func (l *listener) pump(ctx context.Context) error {
	hwnd := l.hwnd
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			if ok, _, err := procPostMessage.Call(hwnd, wmShutdown, 0, 0); ok == 0 {
				l.log.Error("posting shutdown message failed", "err", winError("PostMessageW", err))
			}
		case <-done:
		}
	}()

	msg := new(winMsg)
	for {
		r, _, err := procGetMessage.Call(uintptr(unsafe.Pointer(msg)), 0, 0, 0)
		switch int32(r) {
		case 0:
			return l.err
		case -1:
			return winError("GetMessageW", err)
		}
		procDispatchMessage.Call(uintptr(unsafe.Pointer(msg)))
	}
}

func (l *listener) close() error {
	errs := []error{l.destroyWindow(), l.unregisterClass()}
	activeListener.Store(nil)
	return errors.Join(errs...)
}

// destroyWindow keeps l.hwnd set until DestroyWindow returns, because the WM_RENDERALLFORMATS sent during destruction
// is only dispatched to this listener while the handle still matches.
func (l *listener) destroyWindow() error {
	if l.hwnd == 0 {
		return nil
	}
	hwnd := l.hwnd
	var errs []error
	if ok, _, err := procRemoveClipboardFormatListener.Call(hwnd); ok == 0 {
		errs = append(errs, winError("RemoveClipboardFormatListener", err))
	}
	if ok, _, err := procDestroyWindow.Call(hwnd); ok == 0 {
		errs = append(errs, winError("DestroyWindow", err))
	}
	l.hwnd = 0
	return errors.Join(errs...)
}

func (l *listener) unregisterClass() error {
	if !l.classRegistered {
		return nil
	}
	l.classRegistered = false
	if ok, _, err := procUnregisterClass.Call(uintptr(unsafe.Pointer(l.className)), l.instance); ok == 0 {
		return winError("UnregisterClassW", err)
	}
	return nil
}

func (l *listener) shutdown() {
	l.record(l.destroyWindow())
	procPostQuitMessage.Call(0)
}

func (l *listener) record(err error) {
	if err == nil {
		return
	}
	l.log.Error("clipboard event failed", "err", err)
	l.err = errors.Join(l.err, err)
}

func (l *listener) onClipboardUpdate() error {
	sequence := clipboardSequenceNumber()
	owner := getClipboardOwner()
	if owner == l.hwnd {
		l.log.Info("clipboard update ignored: own delayed offer", "sequence", sequence)
		return nil
	}
	if err := openClipboard(l.hwnd); err != nil {
		return err
	}
	formats, listErr := listClipboardFormats()
	closeErr := closeClipboard()
	if err := errors.Join(listErr, closeErr); err != nil {
		return err
	}

	names := make([]string, 0, len(formats))
	sensitive := false
	for _, id := range formats {
		name := clipboardFormatName(id)
		names = append(names, name)
		sensitive = sensitive || isSensitivityFormat(name)
	}
	l.log.Info("clipboard update", "sequence", sequence, "owner_hwnd", fmt.Sprintf("0x%X", owner),
		"formats", strings.Join(names, ", "), "sensitivity_format", sensitive)
	return nil
}

func wndProc(hwnd uintptr, message uint32, wParam, lParam uintptr) uintptr {
	l := activeListener.Load()
	if l == nil || l.hwnd != hwnd {
		return defWindowProc(hwnd, message, wParam, lParam)
	}
	switch message {
	case wmClipboardUpdate:
		l.record(l.onClipboardUpdate())
	case wmRenderFormat:
		l.record(l.offer.render(uint32(wParam)))
	case wmRenderAllFormats:
		l.record(l.offer.renderAll())
	case wmDestroyClipboard:
		l.offer.lost()
	case wmShutdown:
		l.shutdown()
	default:
		return defWindowProc(hwnd, message, wParam, lParam)
	}
	return 0
}

func defWindowProc(hwnd uintptr, message uint32, wParam, lParam uintptr) uintptr {
	r, _, _ := procDefWindowProc.Call(hwnd, uintptr(message), wParam, lParam)
	return r
}
