package winhook

import (
	"fmt"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	rawInputClassName = "EdgeSyncSpikeRawInput"

	wmInput = 0x00FF

	hidUsagePageGenericDesktop = 0x01
	hidUsageMouse              = 0x02
	ridevInputSink             = 0x00000100
	ridInput                   = 0x10000003
	rimTypeMouse               = 0
	mouseMoveAbsolute          = 0x01
	getRawInputDataFailed      = 0xFFFFFFFF
)

const hwndMessage = ^uintptr(2)

var wndProcOnce = sync.OnceValue(func() uintptr { return syscall.NewCallback(wndTrampoline) })

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

type rawInputDevice struct {
	UsagePage uint16
	Usage     uint16
	Flags     uint32
	Target    uintptr
}

type rawInputHeader struct {
	Type   uint32
	Size   uint32
	Device uintptr
	WParam uintptr
}

type rawMouse struct {
	Flags      uint16
	_          uint16
	Buttons    uint32
	RawButtons uint32
	LastX      int32
	LastY      int32
	Extra      uint32
}

type rawInputMouse struct {
	Header rawInputHeader
	Mouse  rawMouse
}

var (
	_ [unsafe.Sizeof(wndClassEx{}) - 80]struct{}
	_ [80 - unsafe.Sizeof(wndClassEx{})]struct{}
	_ [unsafe.Sizeof(rawInputDevice{}) - 16]struct{}
	_ [16 - unsafe.Sizeof(rawInputDevice{})]struct{}
	_ [unsafe.Sizeof(rawInputHeader{}) - 24]struct{}
	_ [24 - unsafe.Sizeof(rawInputHeader{})]struct{}
	_ [unsafe.Sizeof(rawMouse{}) - 24]struct{}
	_ [24 - unsafe.Sizeof(rawMouse{})]struct{}
	_ [unsafe.Sizeof(rawInputMouse{}) - 48]struct{}
	_ [48 - unsafe.Sizeof(rawInputMouse{})]struct{}
)

func (h *Hook) createWindow() error {
	className, err := windows.UTF16PtrFromString(rawInputClassName)
	if err != nil {
		return fmt.Errorf("window class name: %w", err)
	}
	h.className = className

	class := wndClassEx{
		WndProc:   wndProcOnce(),
		Instance:  h.instance,
		ClassName: className,
	}
	class.Size = uint32(unsafe.Sizeof(class))
	atom, _, errno := syscall.SyscallN(procRegisterClassEx.Addr(), uintptr(unsafe.Pointer(&class)))
	if atom == 0 {
		return fmt.Errorf("RegisterClassExW: %w", errno)
	}
	h.classAdded = true

	hwnd, _, errno := syscall.SyscallN(procCreateWindowEx.Addr(),
		0, uintptr(unsafe.Pointer(className)), 0, 0, 0, 0, 0, 0, hwndMessage, 0, h.instance, 0)
	if hwnd == 0 {
		return fmt.Errorf("CreateWindowExW(HWND_MESSAGE): %w", errno)
	}
	h.hwnd = hwnd
	return nil
}

func (h *Hook) destroyWindow() error {
	var firstErr error
	if h.hwnd != 0 {
		ok, _, errno := syscall.SyscallN(procDestroyWindow.Addr(), h.hwnd)
		h.hwnd = 0
		if ok == 0 {
			firstErr = fmt.Errorf("DestroyWindow: %w", errno)
		}
	}
	if h.classAdded {
		ok, _, errno := syscall.SyscallN(procUnregisterClass.Addr(), uintptr(unsafe.Pointer(h.className)), h.instance)
		h.classAdded = false
		if ok == 0 && firstErr == nil {
			firstErr = fmt.Errorf("UnregisterClassW: %w", errno)
		}
	}
	return firstErr
}

func (h *Hook) registerRawMouse() error {
	device := rawInputDevice{
		UsagePage: hidUsagePageGenericDesktop,
		Usage:     hidUsageMouse,
		Flags:     ridevInputSink,
		Target:    h.hwnd,
	}
	ok, _, errno := syscall.SyscallN(procRegisterRawInputDevices.Addr(),
		uintptr(unsafe.Pointer(&device)), 1, unsafe.Sizeof(device))
	if ok == 0 {
		return fmt.Errorf("RegisterRawInputDevices: %w", errno)
	}
	return nil
}

func wndTrampoline(hwnd uintptr, message uint32, wParam, lParam uintptr) uintptr {
	if message == wmInput {
		if h := activeHook.Load(); h != nil {
			h.onRawInput(lParam)
		}
	}
	result, _, _ := syscall.SyscallN(procDefWindowProc.Addr(), hwnd, uintptr(message), wParam, lParam)
	return result
}

func (h *Hook) onRawInput(lParam uintptr) {
	h.rawSize = uint32(unsafe.Sizeof(h.rawBuf))
	copied, _, _ := syscall.SyscallN(procGetRawInputData.Addr(),
		lParam, ridInput, uintptr(unsafe.Pointer(&h.rawBuf[0])), uintptr(unsafe.Pointer(&h.rawSize)),
		unsafe.Sizeof(rawInputHeader{}))
	if uint32(copied) == getRawInputDataFailed {
		h.rawErrors.Add(1)
		return
	}

	raw := (*rawInputMouse)(unsafe.Pointer(&h.rawBuf[0]))
	if raw.Header.Type != rimTypeMouse {
		return
	}
	if raw.Mouse.Flags&mouseMoveAbsolute != 0 {
		h.rawNonRelative.Add(1)
		return
	}
	if raw.Mouse.LastX == 0 && raw.Mouse.LastY == 0 {
		return
	}
	h.rawEvents.Add(1)
	h.sink.Push(Event{Kind: KindRawMouse, X: raw.Mouse.LastX, Y: raw.Mouse.LastY, QPC: h.qpc()})
}
