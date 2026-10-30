package main

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	wmSetCursor = 0x0020
	wmMouseMove = 0x0200
	wmTimer     = 0x0113

	wsPopup        = 0x80000000
	wsExTopmost    = 0x00000008
	wsExToolWindow = 0x00000080
	wsExLayered    = 0x00080000
	wsExNoActivate = 0x08000000

	swShowNoActivate = 4
	lwaAlpha         = 0x00000002
	layeredAlpha     = 1

	monitorInfoFPrimary = 0x00000001

	parkClassName = "EdgeSyncSpikeCursorPark"
	cursorSide    = 32
)

var (
	user32   = windows.NewLazySystemDLL("user32.dll")
	kernel32 = windows.NewLazySystemDLL("kernel32.dll")

	procClipCursor                 = user32.NewProc("ClipCursor")
	procSetCursorPos               = user32.NewProc("SetCursorPos")
	procSetCapture                 = user32.NewProc("SetCapture")
	procReleaseCapture             = user32.NewProc("ReleaseCapture")
	procShowWindow                 = user32.NewProc("ShowWindow")
	procDestroyWindow              = user32.NewProc("DestroyWindow")
	procSetLayeredWindowAttributes = user32.NewProc("SetLayeredWindowAttributes")
	procEnumDisplayMonitors        = user32.NewProc("EnumDisplayMonitors")
	procGetMonitorInfo             = user32.NewProc("GetMonitorInfoW")
	procRegisterClassEx            = user32.NewProc("RegisterClassExW")
	procUnregisterClass            = user32.NewProc("UnregisterClassW")
	procCreateWindowEx             = user32.NewProc("CreateWindowExW")
	procDefWindowProc              = user32.NewProc("DefWindowProcW")
	procCreateCursor               = user32.NewProc("CreateCursor")
	procDestroyCursor              = user32.NewProc("DestroyCursor")
	procSetCursor                  = user32.NewProc("SetCursor")
	procSetTimer                   = user32.NewProc("SetTimer")
	procKillTimer                  = user32.NewProc("KillTimer")
	procGetMessage                 = user32.NewProc("GetMessageW")
	procDispatchMessage            = user32.NewProc("DispatchMessageW")
	procGetModuleHandle            = kernel32.NewProc("GetModuleHandleW")
	procSetConsoleCtrlHandler      = kernel32.NewProc("SetConsoleCtrlHandler")
)

type point struct {
	X int32
	Y int32
}

type winRect struct {
	Left   int32
	Top    int32
	Right  int32
	Bottom int32
}

type monitorInfo struct {
	Size    uint32
	Monitor winRect
	Work    winRect
	Flags   uint32
}

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

type winMsg struct {
	Hwnd    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      point
	_       uint32
}

var (
	_ [unsafe.Sizeof(monitorInfo{}) - 40]struct{}
	_ [40 - unsafe.Sizeof(monitorInfo{})]struct{}
	_ [unsafe.Sizeof(wndClassEx{}) - 80]struct{}
	_ [80 - unsafe.Sizeof(wndClassEx{})]struct{}
	_ [unsafe.Sizeof(winMsg{}) - 48]struct{}
	_ [48 - unsafe.Sizeof(winMsg{})]struct{}
)

var errNoPrimaryMonitor = errors.New("EnumDisplayMonitors reported no primary monitor")

func winErr(name string, errno syscall.Errno) error {
	if errno == 0 {
		return fmt.Errorf("%s failed", name)
	}
	return fmt.Errorf("%s: %w", name, errno)
}

func boolResult(name string, ok uintptr, errno syscall.Errno) error {
	if ok != 0 {
		return nil
	}
	return winErr(name, errno)
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
	var info monitorInfo
	info.Size = uint32(unsafe.Sizeof(info))
	ok, _, errno := syscall.SyscallN(procGetMonitorInfo.Addr(), hmonitor, uintptr(unsafe.Pointer(&info)))
	if ok == 0 {
		list.err = winErr("GetMonitorInfoW", errno)
		return 0
	}
	list.monitors = append(list.monitors, info)
	return 1
}

func enumerateMonitors() ([]monitorInfo, error) {
	list := &monitorList{}
	monitorTarget.Store(list)
	defer monitorTarget.Store(nil)
	ok, _, errno := syscall.SyscallN(procEnumDisplayMonitors.Addr(), 0, 0, monitorEnumCallback(), 0)
	if list.err != nil {
		return nil, list.err
	}
	if err := boolResult("EnumDisplayMonitors", ok, errno); err != nil {
		return nil, err
	}
	return list.monitors, nil
}

// primaryCentre takes the primary monitor from EnumDisplayMonitors because GetSystemMetrics is not DPI aware for a
// Per-Monitor-v2 thread; the rectangle is in physical pixels.
func primaryCentre() (point, error) {
	monitors, err := enumerateMonitors()
	if err != nil {
		return point{}, err
	}
	for _, m := range monitors {
		if m.Flags&monitorInfoFPrimary != 0 {
			return point{X: (m.Monitor.Left + m.Monitor.Right) / 2, Y: (m.Monitor.Top + m.Monitor.Bottom) / 2}, nil
		}
	}
	return point{}, errNoPrimaryMonitor
}

func setCursorPos(p point) error {
	ok, _, errno := syscall.SyscallN(procSetCursorPos.Addr(), uintptr(p.X), uintptr(p.Y))
	return boolResult("SetCursorPos", ok, errno)
}

func clipCursor(rect *winRect) error {
	ok, _, errno := syscall.SyscallN(procClipCursor.Addr(), uintptr(unsafe.Pointer(rect)))
	return boolResult("ClipCursor", ok, errno)
}

func showNoActivate(hwnd uintptr) {
	syscall.SyscallN(procShowWindow.Addr(), hwnd, swShowNoActivate)
}

// parkClass owns the window class, the transparent cursor and every parking window of the process. Its single
// active instance is what the window procedure dispatches to (D-80).
type parkClass struct {
	instance  uintptr
	cursor    uintptr
	className *uint16
	centre    point
	windows   []uintptr
	asyncErr  error
}

var (
	activePark          atomic.Pointer[parkClass]
	parkWndProcCallback = sync.OnceValue(func() uintptr { return syscall.NewCallback(parkWndProc) })
)

func newParkClass() (*parkClass, error) {
	instance, _, errno := syscall.SyscallN(procGetModuleHandle.Addr(), 0)
	if instance == 0 {
		return nil, winErr("GetModuleHandleW", errno)
	}
	cursor, err := createTransparentCursor()
	if err != nil {
		return nil, err
	}
	name, err := windows.UTF16PtrFromString(parkClassName)
	if err != nil {
		return nil, errors.Join(err, destroyCursor(cursor))
	}
	class := wndClassEx{
		Size:      uint32(unsafe.Sizeof(wndClassEx{})),
		WndProc:   parkWndProcCallback(),
		Instance:  instance,
		Cursor:    cursor,
		ClassName: name,
	}
	atom, _, errno := syscall.SyscallN(procRegisterClassEx.Addr(), uintptr(unsafe.Pointer(&class)))
	if atom == 0 {
		return nil, errors.Join(winErr("RegisterClassExW", errno), destroyCursor(cursor))
	}
	c := &parkClass{instance: instance, cursor: cursor, className: name}
	activePark.Store(c)
	return c, nil
}

// An AND plane of all ones with an XOR plane of zeros leaves the screen pixel unchanged, which makes the cursor
// invisible without touching any system cursor.
func createTransparentCursor() (uintptr, error) {
	and := make([]byte, cursorSide*cursorSide/8)
	for i := range and {
		and[i] = 0xFF
	}
	xor := make([]byte, len(and))
	cursor, _, errno := syscall.SyscallN(procCreateCursor.Addr(), 0, 0, 0, cursorSide, cursorSide,
		uintptr(unsafe.Pointer(&and[0])), uintptr(unsafe.Pointer(&xor[0])))
	if cursor == 0 {
		return 0, winErr("CreateCursor", errno)
	}
	return cursor, nil
}

func destroyCursor(cursor uintptr) error {
	ok, _, errno := syscall.SyscallN(procDestroyCursor.Addr(), cursor)
	return boolResult("DestroyCursor", ok, errno)
}

func (c *parkClass) createWindow(exStyle uint32, r winRect) (uintptr, error) {
	hwnd, _, errno := syscall.SyscallN(procCreateWindowEx.Addr(),
		uintptr(exStyle), uintptr(unsafe.Pointer(c.className)), 0, wsPopup,
		uintptr(r.Left), uintptr(r.Top), uintptr(r.Right-r.Left), uintptr(r.Bottom-r.Top),
		0, 0, c.instance, 0)
	if hwnd == 0 {
		return 0, winErr("CreateWindowExW", errno)
	}
	c.windows = append(c.windows, hwnd)
	return hwnd, nil
}

func (c *parkClass) destroyWindows() error {
	var errs []error
	for _, hwnd := range c.windows {
		ok, _, errno := syscall.SyscallN(procDestroyWindow.Addr(), hwnd)
		errs = append(errs, boolResult("DestroyWindow", ok, errno))
	}
	c.windows = c.windows[:0]
	return errors.Join(errs...)
}

func (c *parkClass) takeAsyncErr() error {
	err := c.asyncErr
	c.asyncErr = nil
	return err
}

func (c *parkClass) close() error {
	errs := []error{c.destroyWindows()}
	ok, _, errno := syscall.SyscallN(procUnregisterClass.Addr(), uintptr(unsafe.Pointer(c.className)), c.instance)
	errs = append(errs, boolResult("UnregisterClassW", ok, errno), destroyCursor(c.cursor))
	activePark.CompareAndSwap(c, nil)
	return errors.Join(errs...)
}

// The window procedure runs on the message-loop thread only, so the fields it reads need no synchronisation. Errors
// cannot be returned from the callback and are kept for the loop to report.
func parkWndProc(hwnd uintptr, msg uint32, wParam, lParam uintptr) uintptr {
	if c := activePark.Load(); c != nil {
		switch msg {
		case wmSetCursor:
			syscall.SyscallN(procSetCursor.Addr(), c.cursor)
			return 1
		case wmMouseMove:
			if err := setCursorPos(c.centre); err != nil {
				c.asyncErr = errors.Join(c.asyncErr, err)
			}
			return 0
		}
	}
	result, _, _ := syscall.SyscallN(procDefWindowProc.Addr(), hwnd, uintptr(msg), wParam, lParam)
	return result
}

type parker interface {
	name() string
	engage() error
	release() error
}

type clipParker struct{}

func (clipParker) name() string { return "strategy 1: ClipCursor 1x1 + SetCursorPos" }

func (clipParker) engage() error {
	centre, err := primaryCentre()
	if err != nil {
		return err
	}
	rect := winRect{Left: centre.X, Top: centre.Y, Right: centre.X + 1, Bottom: centre.Y + 1}
	if err := clipCursor(&rect); err != nil {
		return err
	}
	return setCursorPos(centre)
}

func (clipParker) release() error {
	return clipCursor(nil)
}

type captureParker struct {
	class *parkClass
}

func (captureParker) name() string { return "strategy 2: 1x1 topmost window + SetCapture" }

func (p captureParker) engage() error {
	centre, err := primaryCentre()
	if err != nil {
		return err
	}
	p.class.centre = centre
	hwnd, err := p.class.createWindow(wsExToolWindow|wsExTopmost,
		winRect{Left: centre.X, Top: centre.Y, Right: centre.X + 1, Bottom: centre.Y + 1})
	if err != nil {
		return err
	}
	showNoActivate(hwnd)
	syscall.SyscallN(procSetCapture.Addr(), hwnd)
	return setCursorPos(centre)
}

// Nothing is captured until engage created a window, so an idle release has nothing to undo.
func (p captureParker) release() error {
	if len(p.class.windows) == 0 {
		return nil
	}
	ok, _, errno := syscall.SyscallN(procReleaseCapture.Addr())
	return errors.Join(boolResult("ReleaseCapture", ok, errno), p.class.destroyWindows())
}

type layeredParker struct {
	class *parkClass
}

func (layeredParker) name() string {
	return "strategy 3: fullscreen layered windows (alpha 1) + transparent cursor"
}

func (p layeredParker) engage() error {
	centre, err := primaryCentre()
	if err != nil {
		return err
	}
	p.class.centre = centre
	monitors, err := enumerateMonitors()
	if err != nil {
		return err
	}
	for _, m := range monitors {
		hwnd, err := p.class.createWindow(wsExLayered|wsExTopmost|wsExToolWindow|wsExNoActivate, m.Monitor)
		if err != nil {
			return err
		}
		ok, _, errno := syscall.SyscallN(procSetLayeredWindowAttributes.Addr(), hwnd, 0, layeredAlpha, lwaAlpha)
		if err := boolResult("SetLayeredWindowAttributes", ok, errno); err != nil {
			return err
		}
		showNoActivate(hwnd)
	}
	return setCursorPos(centre)
}

func (p layeredParker) release() error {
	return p.class.destroyWindows()
}

// The handler only sets a flag and returns TRUE so the default handler (which exits without running deferred calls)
// does not run; the message loop notices the flag on its next timer tick and unwinds normally.
var (
	interruptFlag       atomic.Pointer[atomic.Bool]
	ctrlHandlerCallback = sync.OnceValue(func() uintptr { return syscall.NewCallback(ctrlHandler) })
)

func ctrlHandler(_ uint32) uintptr {
	flag := interruptFlag.Load()
	if flag == nil {
		return 0
	}
	flag.Store(true)
	return 1
}

func installInterrupt(flag *atomic.Bool) error {
	interruptFlag.Store(flag)
	ok, _, errno := syscall.SyscallN(procSetConsoleCtrlHandler.Addr(), ctrlHandlerCallback(), 1)
	if err := boolResult("SetConsoleCtrlHandler(add)", ok, errno); err != nil {
		interruptFlag.Store(nil)
		return err
	}
	return nil
}

func removeInterrupt() error {
	interruptFlag.Store(nil)
	ok, _, errno := syscall.SyscallN(procSetConsoleCtrlHandler.Addr(), ctrlHandlerCallback(), 0)
	return boolResult("SetConsoleCtrlHandler(remove)", ok, errno)
}

// Timer messages posted with a NULL window carry a NULL hwnd and are handled here instead of being dispatched.
func pumpMessages(intervalMs uint32, tick func() (bool, error)) error {
	timer, _, errno := syscall.SyscallN(procSetTimer.Addr(), 0, 0, uintptr(intervalMs), 0)
	if timer == 0 {
		return winErr("SetTimer", errno)
	}
	loopErr := messageLoop(tick)
	ok, _, errno := syscall.SyscallN(procKillTimer.Addr(), 0, timer)
	return errors.Join(loopErr, boolResult("KillTimer", ok, errno))
}

func messageLoop(tick func() (bool, error)) error {
	var msg winMsg
	for {
		result, _, errno := syscall.SyscallN(procGetMessage.Addr(), uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		switch int32(result) {
		case -1:
			return winErr("GetMessageW", errno)
		case 0:
			return nil
		}
		if msg.Hwnd == 0 && msg.Message == wmTimer {
			done, err := tick()
			if err != nil || done {
				return err
			}
			continue
		}
		syscall.SyscallN(procDispatchMessage.Addr(), uintptr(unsafe.Pointer(&msg)))
	}
}
