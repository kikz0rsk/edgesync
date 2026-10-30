//go:build windows

package main

import (
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"path/filepath"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/kikz0r/edgesync/spikes/internal/wincom"
)

const (
	dropFilesHeaderSize = unsafe.Sizeof(wincom.DropFiles{})
	dropFilesWideOffset = unsafe.Offsetof(wincom.DropFiles{}.Wide)
)

var (
	ErrMalformedDropFiles = errors.New("malformed CF_HDROP data")
	ErrANSIDropFiles      = errors.New("CF_HDROP list is ANSI; only wide lists are supported")
	ErrRelativeDropPath   = errors.New("dropped path is not absolute")
)

var (
	ole32  = windows.NewLazySystemDLL("ole32.dll")
	user32 = windows.NewLazySystemDLL("user32.dll")

	procOleInitialize    = ole32.NewProc("OleInitialize")
	procRegisterDragDrop = ole32.NewProc("RegisterDragDrop")
	procScreenToClient   = user32.NewProc("ScreenToClient")
)

// parseDropFiles decodes a CF_HDROP payload: a DROPFILES header followed by a double-NUL terminated list of names.
func parseDropFiles(raw []byte) ([]string, error) {
	headerSize := int(dropFilesHeaderSize)
	if len(raw) < headerSize {
		return nil, fmt.Errorf("%w: %d bytes is shorter than the %d byte header", ErrMalformedDropFiles, len(raw), headerSize)
	}
	offset := int(binary.LittleEndian.Uint32(raw))
	if offset < headerSize || offset > len(raw) {
		return nil, fmt.Errorf("%w: file list offset %d outside [%d, %d]", ErrMalformedDropFiles, offset, headerSize, len(raw))
	}
	if binary.LittleEndian.Uint32(raw[dropFilesWideOffset:]) == 0 {
		return nil, ErrANSIDropFiles
	}
	return parseWideList(raw[offset:])
}

func parseWideList(list []byte) ([]string, error) {
	if len(list)%2 != 0 {
		return nil, fmt.Errorf("%w: wide list has an odd byte count %d", ErrMalformedDropFiles, len(list))
	}
	units := make([]uint16, len(list)/2)
	for i := range units {
		units[i] = binary.LittleEndian.Uint16(list[2*i:])
	}
	var names []string
	for start := 0; ; {
		if start >= len(units) {
			return nil, fmt.Errorf("%w: wide list is not double-NUL terminated", ErrMalformedDropFiles)
		}
		end := start
		for end < len(units) && units[end] != 0 {
			end++
		}
		if end == len(units) {
			return nil, fmt.Errorf("%w: wide list ends inside a name", ErrMalformedDropFiles)
		}
		if end == start {
			return names, nil
		}
		names = append(names, windows.UTF16ToString(units[start:end]))
		start = end + 1
	}
}

func normalisePath(name string) (string, error) {
	clean := filepath.Clean(name)
	if !filepath.IsAbs(clean) {
		return "", fmt.Errorf("%w: %q", ErrRelativeDropPath, name)
	}
	return clean, nil
}

// negotiateCopy accepts only DROPEFFECT_COPY: the spike never moves or links the dropped files.
func negotiateCopy(allowed uint32) uint32 {
	if allowed&wincom.DropEffectCopy != 0 {
		return wincom.DropEffectCopy
	}
	return wincom.DropEffectNone
}

// dropPointDp converts a client point in device pixels to Gio's device-independent pixels.
func dropPointDp(client image.Point, pxPerDp float32) (x, y float32) {
	return float32(client.X) / pxPerDp, float32(client.Y) / pxPerDp
}

type dropEvent struct {
	Paths  []string
	Client image.Point
}

type dropSnapshot struct {
	Registered bool
	HWND       uintptr
	OleResult  wincom.HResult
	Drops      int
	Last       dropEvent
	Err        error
}

// dropTarget is the IDropTarget registered on Gio's window. Windows calls it on the Gio window thread; the layout reads
// snapshots from the event goroutine, so every field is guarded by mu.
type dropTarget struct {
	invalidate func()

	mu         sync.Mutex
	hwnd       uintptr
	oleResult  wincom.HResult
	registered bool
	drops      int
	last       dropEvent
	err        error
}

func newDropTarget(invalidate func()) *dropTarget {
	return &dropTarget{invalidate: invalidate}
}

// attach must run on the Gio window thread. OleInitialize is used rather than CoInitializeEx because RegisterDragDrop
// fails with E_OUTOFMEMORY after CoInitializeEx. The creation reference is released once OLE holds its own.
func (t *dropTarget) attach(hwnd uintptr) error {
	target, err := wincom.NewDropTarget(t)
	if err != nil {
		return fmt.Errorf("IDropTarget: %w", err)
	}
	defer wincom.Release(target)

	hr, _, _ := procOleInitialize.Call(0)
	oleResult := wincom.HResult(hr)
	if oleResult.Failed() {
		return fmt.Errorf("OleInitialize: %w", oleResult)
	}
	t.mu.Lock()
	t.hwnd = hwnd
	t.oleResult = oleResult
	t.mu.Unlock()

	hr, _, _ = procRegisterDragDrop.Call(hwnd, target)
	if result := wincom.HResult(hr); result.Failed() {
		return fmt.Errorf("RegisterDragDrop: %w", result)
	}
	t.mu.Lock()
	t.registered = true
	t.mu.Unlock()
	return nil
}

func (t *dropTarget) snapshot() dropSnapshot {
	t.mu.Lock()
	defer t.mu.Unlock()
	return dropSnapshot{
		Registered: t.registered,
		HWND:       t.hwnd,
		OleResult:  t.oleResult,
		Drops:      t.drops,
		Last:       t.last,
		Err:        t.err,
	}
}

func (t *dropTarget) recordDrop(ev dropEvent) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.drops++
	t.last = ev
	t.err = nil
}

func (t *dropTarget) recordError(err error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.err = err
}

func (t *dropTarget) DragEnter(_ uintptr, _ uint32, _ wincom.Point, effect uint32) (uint32, error) {
	return negotiateCopy(effect), nil
}

func (t *dropTarget) DragOver(_ uint32, _ wincom.Point, effect uint32) (uint32, error) {
	return negotiateCopy(effect), nil
}

func (t *dropTarget) DragLeave() error {
	return nil
}

func (t *dropTarget) Drop(data uintptr, _ uint32, pt wincom.Point, effect uint32) (uint32, error) {
	defer t.invalidate()
	paths, err := readDroppedPaths(data)
	if err != nil {
		err = fmt.Errorf("drop data: %w", err)
		t.recordError(err)
		return wincom.DropEffectNone, err
	}
	t.mu.Lock()
	hwnd := t.hwnd
	t.mu.Unlock()
	client, err := clientPoint(hwnd, pt)
	if err != nil {
		t.recordError(err)
		return wincom.DropEffectNone, err
	}
	t.recordDrop(dropEvent{Paths: paths, Client: client})
	return negotiateCopy(effect), nil
}

func readDroppedPaths(data uintptr) ([]string, error) {
	medium, err := wincom.GetData(data, wincom.FormatEtc{
		Format: wincom.CFHDrop, Aspect: wincom.DVAspectContent, Index: -1, Tymed: wincom.TymedHGlobal,
	})
	if err != nil {
		return nil, err
	}
	defer wincom.ReleaseMedium(&medium)
	raw, err := medium.HGlobalBytes()
	if err != nil {
		return nil, err
	}
	names, err := parseDropFiles(raw)
	if err != nil {
		return nil, err
	}
	paths := make([]string, 0, len(names))
	for _, name := range names {
		path, err := normalisePath(name)
		if err != nil {
			return nil, err
		}
		paths = append(paths, path)
	}
	return paths, nil
}

// clientPoint converts the screen point OLE passes to IDropTarget into client coordinates of hwnd, in device pixels.
func clientPoint(hwnd uintptr, screen wincom.Point) (image.Point, error) {
	client := screen
	ok, _, callErr := procScreenToClient.Call(hwnd, uintptr(unsafe.Pointer(&client)))
	if ok == 0 {
		return image.Point{}, fmt.Errorf("ScreenToClient: %w", callErr)
	}
	return image.Pt(int(client.X), int(client.Y)), nil
}
