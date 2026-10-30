package main

import (
	"fmt"
	"sync"
	"sync/atomic"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	mdtEffectiveDPI     = 0
	monitorInfoFPrimary = 0x1
	cchDeviceName       = 32
	smXVirtualScreen    = 76
	smYVirtualScreen    = 77
	smCXVirtualScreen   = 78
	smCYVirtualScreen   = 79

	dpiAwarenessContextPerMonitorV2 = ^uintptr(3)
)

var (
	user32                     = windows.NewLazySystemDLL("user32.dll")
	shcore                     = windows.NewLazySystemDLL("shcore.dll")
	procEnumDisplayMonitors    = user32.NewProc("EnumDisplayMonitors")
	procGetMonitorInfo         = user32.NewProc("GetMonitorInfoW")
	procGetSystemMetrics       = user32.NewProc("GetSystemMetrics")
	procGetDpiForMonitor       = shcore.NewProc("GetDpiForMonitor")
	procGetProcessDpiAwareness = shcore.NewProc("GetProcessDpiAwareness")
	procGetThreadDpiContext    = user32.NewProc("GetThreadDpiAwarenessContext")
	procAreDpiContextsEqual    = user32.NewProc("AreDpiAwarenessContextsEqual")
)

type winRect struct {
	Left, Top, Right, Bottom int32
}

type monitorInfoEx struct {
	CbSize  uint32
	Monitor winRect
	Work    winRect
	Flags   uint32
	Device  [cchDeviceName]uint16
}

var (
	_ [unsafe.Sizeof(monitorInfoEx{}) - 104]struct{}
	_ [104 - unsafe.Sizeof(monitorInfoEx{})]struct{}
)

type monitorInfo struct {
	Name    string
	Bounds  Rect
	Work    Rect
	Primary bool
	DPI     uint32
}

type monitorList struct {
	monitors []monitorInfo
	err      error
}

var (
	monitorTarget       atomic.Pointer[monitorList]
	monitorEnumCallback = sync.OnceValue(func() uintptr { return syscall.NewCallback(monitorEnumProc) })
)

func monitorEnumProc(hmonitor, _, _, _ uintptr) uintptr {
	list := monitorTarget.Load()
	if list == nil {
		return 0
	}
	info, err := queryMonitor(hmonitor)
	if err != nil {
		list.err = err
		return 0
	}
	list.monitors = append(list.monitors, info)
	return 1
}

func queryMonitor(hmonitor uintptr) (monitorInfo, error) {
	var info monitorInfoEx
	info.CbSize = uint32(unsafe.Sizeof(info))
	ok, _, errno := syscall.SyscallN(procGetMonitorInfo.Addr(), hmonitor, uintptr(unsafe.Pointer(&info)))
	if ok == 0 {
		return monitorInfo{}, callError("GetMonitorInfoW", errno)
	}
	var dpiX, dpiY uint32
	hr, _, _ := syscall.SyscallN(procGetDpiForMonitor.Addr(), hmonitor, mdtEffectiveDPI,
		uintptr(unsafe.Pointer(&dpiX)), uintptr(unsafe.Pointer(&dpiY)))
	if hr != 0 {
		return monitorInfo{}, fmt.Errorf("GetDpiForMonitor: HRESULT 0x%08x", uint32(hr))
	}
	return monitorInfo{
		Name:    syscall.UTF16ToString(info.Device[:]),
		Bounds:  toRect(info.Monitor),
		Work:    toRect(info.Work),
		Primary: info.Flags&monitorInfoFPrimary != 0,
		DPI:     dpiX,
	}, nil
}

func toRect(r winRect) Rect {
	return Rect{X: r.Left, Y: r.Top, W: r.Right - r.Left, H: r.Bottom - r.Top}
}

func enumerateMonitors() ([]monitorInfo, error) {
	list := &monitorList{}
	monitorTarget.Store(list)
	defer monitorTarget.Store(nil)
	ok, _, errno := syscall.SyscallN(procEnumDisplayMonitors.Addr(), 0, 0, monitorEnumCallback(), 0)
	if list.err != nil {
		return nil, list.err
	}
	if ok == 0 {
		return nil, callError("EnumDisplayMonitors", errno)
	}
	return list.monitors, nil
}

func virtualDesktopMetrics() (Rect, error) {
	x := systemMetric(smXVirtualScreen)
	y := systemMetric(smYVirtualScreen)
	w := systemMetric(smCXVirtualScreen)
	h := systemMetric(smCYVirtualScreen)
	if w <= 0 || h <= 0 {
		return Rect{}, fmt.Errorf("GetSystemMetrics virtual screen %dx%d: %w", w, h, ErrDegenerate)
	}
	return Rect{X: x, Y: y, W: w, H: h}, nil
}

func systemMetric(index uintptr) int32 {
	v, _, _ := syscall.SyscallN(procGetSystemMetrics.Addr(), index)
	return int32(v)
}

func processDPIAwareness() (int32, error) {
	var value int32
	hr, _, _ := syscall.SyscallN(procGetProcessDpiAwareness.Addr(), 0, uintptr(unsafe.Pointer(&value)))
	if hr != 0 {
		return 0, fmt.Errorf("GetProcessDpiAwareness: HRESULT 0x%08x", uint32(hr))
	}
	return value, nil
}

func threadIsPerMonitorV2() bool {
	ctx, _, _ := syscall.SyscallN(procGetThreadDpiContext.Addr())
	equal, _, _ := syscall.SyscallN(procAreDpiContextsEqual.Addr(), ctx, dpiAwarenessContextPerMonitorV2)
	return equal != 0
}

func callError(name string, errno syscall.Errno) error {
	if errno == 0 {
		return fmt.Errorf("%s failed", name)
	}
	return fmt.Errorf("%s: %w", name, errno)
}
