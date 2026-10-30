package winhook

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/kikz0r/edgesync/spikes/internal/stats"
)

const (
	MarkerExtraInfo = 0x45444753

	whKeyboardLL = 13
	whMouseLL    = 14

	wmUser       = 0x0400
	wmQuit       = 0x0012
	wmApp        = 0x8000
	wmReinstall  = wmApp + 1
	wmKeyDown    = 0x0100
	wmKeyUp      = 0x0101
	wmSysKeyDown = 0x0104
	wmSysKeyUp   = 0x0105

	vkScroll      = 0x91
	llkhfInjected = 0x00000010
	pmNoRemove    = 0x0000

	smXVirtualScreen  = 76
	smYVirtualScreen  = 77
	smCXVirtualScreen = 78
	smCYVirtualScreen = 79

	dpiAwarenessContextPerMonitorV2 = ^uintptr(3)
)

var (
	ErrAlreadyStarted = errors.New("winhook: hook already started")
	ErrAnotherActive  = errors.New("winhook: another hook instance is active in this process")
	ErrNilSink        = errors.New("winhook: nil ring")
)

var (
	user32   = windows.NewLazySystemDLL("user32.dll")
	kernel32 = windows.NewLazySystemDLL("kernel32.dll")

	procSetWindowsHookEx          = user32.NewProc("SetWindowsHookExW")
	procUnhookWindowsHookEx       = user32.NewProc("UnhookWindowsHookEx")
	procCallNextHookEx            = user32.NewProc("CallNextHookEx")
	procGetMessage                = user32.NewProc("GetMessageW")
	procPeekMessage               = user32.NewProc("PeekMessageW")
	procDispatchMessage           = user32.NewProc("DispatchMessageW")
	procPostThreadMessage         = user32.NewProc("PostThreadMessageW")
	procRegisterClassEx           = user32.NewProc("RegisterClassExW")
	procUnregisterClass           = user32.NewProc("UnregisterClassW")
	procCreateWindowEx            = user32.NewProc("CreateWindowExW")
	procDestroyWindow             = user32.NewProc("DestroyWindow")
	procDefWindowProc             = user32.NewProc("DefWindowProcW")
	procRegisterRawInputDevices   = user32.NewProc("RegisterRawInputDevices")
	procGetRawInputData           = user32.NewProc("GetRawInputData")
	procSendInput                 = user32.NewProc("SendInput")
	procGetSystemMetrics          = user32.NewProc("GetSystemMetrics")
	procSetProcessDpiAwareContext = user32.NewProc("SetProcessDpiAwarenessContext")
	procGetModuleHandle           = kernel32.NewProc("GetModuleHandleW")
	procQueryPerformanceCounter   = kernel32.NewProc("QueryPerformanceCounter")
	procQueryPerformanceFrequency = kernel32.NewProc("QueryPerformanceFrequency")

	allProcs = []*windows.LazyProc{
		procSetWindowsHookEx, procUnhookWindowsHookEx, procCallNextHookEx, procGetMessage, procPeekMessage,
		procDispatchMessage, procPostThreadMessage, procRegisterClassEx, procUnregisterClass, procCreateWindowEx,
		procDestroyWindow, procDefWindowProc, procRegisterRawInputDevices, procGetRawInputData, procSendInput,
		procGetSystemMetrics, procSetProcessDpiAwareContext, procGetModuleHandle, procQueryPerformanceCounter,
		procQueryPerformanceFrequency,
	}
)

var (
	activeHook     atomic.Pointer[Hook]
	mouseProcOnce  = sync.OnceValue(func() uintptr { return syscall.NewCallback(mouseTrampoline) })
	keyProcOnce    = sync.OnceValue(func() uintptr { return syscall.NewCallback(keyTrampoline) })
	procsResolveMu sync.Mutex
)

type point struct {
	X int32
	Y int32
}

type msllHookStruct struct {
	Pt        point
	MouseData uint32
	Flags     uint32
	Time      uint32
	ExtraInfo uintptr
}

type kbdllHookStruct struct {
	VkCode    uint32
	ScanCode  uint32
	Flags     uint32
	Time      uint32
	ExtraInfo uintptr
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
	_ [unsafe.Sizeof(msllHookStruct{}) - 32]struct{}
	_ [32 - unsafe.Sizeof(msllHookStruct{})]struct{}
	_ [unsafe.Sizeof(kbdllHookStruct{}) - 24]struct{}
	_ [24 - unsafe.Sizeof(kbdllHookStruct{})]struct{}
	_ [unsafe.Sizeof(winMsg{}) - 48]struct{}
	_ [48 - unsafe.Sizeof(winMsg{})]struct{}
)

type Options struct {
	StallOnce time.Duration
}

type Rect struct {
	X, Y, W, H int32
}

type Stats struct {
	Events         uint64
	Drops          uint64
	Reinstalls     uint64
	StaleUnhooks   uint64
	MarkerMouse    uint64
	MarkerKey      uint64
	RawEvents      uint64
	RawNonRelative uint64
	RawErrors      uint64
	MaxCallback    time.Duration
}

type Hook struct {
	stall   time.Duration
	sink    *Ring
	epoch   time.Time
	qpcFreq int64

	started    atomic.Bool
	cancel     context.CancelFunc
	wg         sync.WaitGroup
	done       chan struct{}
	threadID   atomic.Uint32
	errMu      sync.Mutex
	err        error
	suppress   atomic.Bool
	stallArmed atomic.Bool

	events       atomic.Uint64
	markerMouse  atomic.Uint64
	markerKey    atomic.Uint64
	reinstalls   atomic.Uint64
	staleUnhooks atomic.Uint64
	lastMarkerNs atomic.Int64
	hist         stats.Histogram

	rawEvents      atomic.Uint64
	rawNonRelative atomic.Uint64
	rawErrors      atomic.Uint64

	instance   uintptr
	hookMouse  uintptr
	hookKey    uintptr
	hwnd       uintptr
	className  *uint16
	classAdded bool
	scrollDown bool
	msg        winMsg
	qpcScratch int64
	rawSize    uint32
	rawBuf     [8]uint64
}

func New(opts Options) *Hook {
	h := &Hook{stall: opts.StallOnce, done: make(chan struct{})}
	h.stallArmed.Store(opts.StallOnce > 0)
	return h
}

func resolveProcs() error {
	procsResolveMu.Lock()
	defer procsResolveMu.Unlock()
	for _, proc := range allProcs {
		if err := proc.Find(); err != nil {
			return fmt.Errorf("resolve win32 procedure: %w", err)
		}
	}
	return nil
}

func SetPerMonitorV2() error {
	if err := resolveProcs(); err != nil {
		return err
	}
	ok, _, errno := syscall.SyscallN(procSetProcessDpiAwareContext.Addr(), dpiAwarenessContextPerMonitorV2)
	if ok == 0 {
		return fmt.Errorf("SetProcessDpiAwarenessContext: %w", errno)
	}
	return nil
}

func VirtualScreen() (Rect, error) {
	if err := resolveProcs(); err != nil {
		return Rect{}, err
	}
	metric := func(index uintptr) int32 {
		value, _, _ := syscall.SyscallN(procGetSystemMetrics.Addr(), index)
		return int32(value)
	}
	rect := Rect{
		X: metric(smXVirtualScreen),
		Y: metric(smYVirtualScreen),
		W: metric(smCXVirtualScreen),
		H: metric(smCYVirtualScreen),
	}
	if rect.W <= 0 || rect.H <= 0 {
		return Rect{}, fmt.Errorf("GetSystemMetrics returned an empty virtual screen %+v", rect)
	}
	return rect, nil
}

func (h *Hook) Start(ctx context.Context, sink *Ring) error {
	if sink == nil {
		return ErrNilSink
	}
	if err := resolveProcs(); err != nil {
		return err
	}
	if !h.started.CompareAndSwap(false, true) {
		return ErrAlreadyStarted
	}
	if !activeHook.CompareAndSwap(nil, h) {
		h.started.Store(false)
		return ErrAnotherActive
	}
	freq, err := queryPerformanceFrequency()
	if err != nil {
		activeHook.Store(nil)
		h.started.Store(false)
		return err
	}
	h.qpcFreq = freq
	h.sink = sink
	h.epoch = time.Now()
	h.lastMarkerNs.Store(0)

	ctx, h.cancel = context.WithCancel(ctx)
	ready := make(chan error, 1)
	go h.run(ready)
	if err := <-ready; err != nil {
		h.cancel()
		<-h.done
		return err
	}

	h.wg.Add(2)
	go h.watchContext(ctx)
	go h.runWatchdog(ctx)
	return nil
}

func (h *Hook) Close() error {
	if h.cancel == nil {
		return nil
	}
	h.cancel()
	<-h.done
	h.wg.Wait()
	return h.Err()
}

func (h *Hook) Done() <-chan struct{} {
	return h.done
}

func (h *Hook) Err() error {
	h.errMu.Lock()
	defer h.errMu.Unlock()
	return h.err
}

func (h *Hook) Suppressed() bool {
	return h.suppress.Load()
}

func (h *Hook) Callback() *stats.Histogram {
	return &h.hist
}

func (h *Hook) Stats() Stats {
	var drops uint64
	if h.sink != nil {
		drops = h.sink.Drops()
	}
	return Stats{
		Events:         h.events.Load(),
		Drops:          drops,
		Reinstalls:     h.reinstalls.Load(),
		StaleUnhooks:   h.staleUnhooks.Load(),
		MarkerMouse:    h.markerMouse.Load(),
		MarkerKey:      h.markerKey.Load(),
		RawEvents:      h.rawEvents.Load(),
		RawNonRelative: h.rawNonRelative.Load(),
		RawErrors:      h.rawErrors.Load(),
		MaxCallback:    h.hist.Max(),
	}
}

func (h *Hook) recordErr(err error) {
	if err == nil {
		return
	}
	h.errMu.Lock()
	h.err = errors.Join(h.err, err)
	h.errMu.Unlock()
}

func (h *Hook) fail(err error) {
	h.recordErr(err)
	h.cancel()
}

func (h *Hook) run(ready chan<- error) {
	runtime.LockOSThread()
	defer close(h.done)
	defer activeHook.CompareAndSwap(h, nil)

	if err := h.setup(); err != nil {
		ready <- errors.Join(err, h.teardown())
		return
	}
	h.threadID.Store(windows.GetCurrentThreadId())
	ready <- nil

	h.recordErr(h.pump())
	h.recordErr(h.teardown())
}

func (h *Hook) setup() error {
	syscall.SyscallN(procPeekMessage.Addr(), uintptr(unsafe.Pointer(&h.msg)), 0, wmUser, wmUser, pmNoRemove)

	module, _, errno := syscall.SyscallN(procGetModuleHandle.Addr(), 0)
	if module == 0 {
		return fmt.Errorf("GetModuleHandleW: %w", errno)
	}
	h.instance = module

	if err := h.createWindow(); err != nil {
		return err
	}
	if err := h.registerRawMouse(); err != nil {
		return err
	}
	return h.installHooks()
}

func (h *Hook) teardown() error {
	removeErr := h.removeHooks()
	destroyErr := h.destroyWindow()
	return errors.Join(removeErr, destroyErr)
}

func (h *Hook) pump() error {
	for {
		result, _, errno := syscall.SyscallN(procGetMessage.Addr(), uintptr(unsafe.Pointer(&h.msg)), 0, 0, 0)
		switch int32(result) {
		case -1:
			return fmt.Errorf("GetMessageW: %w", errno)
		case 0:
			return nil
		}
		if h.msg.Hwnd == 0 && h.msg.Message == wmReinstall {
			if err := h.reinstall(); err != nil {
				return err
			}
			continue
		}
		syscall.SyscallN(procDispatchMessage.Addr(), uintptr(unsafe.Pointer(&h.msg)))
	}
}

func (h *Hook) watchContext(ctx context.Context) {
	defer h.wg.Done()
	select {
	case <-h.done:
		return
	case <-ctx.Done():
	}
	if err := postThreadMessage(h.threadID.Load(), wmQuit); err != nil {
		select {
		case <-h.done:
		default:
			h.recordErr(err)
		}
	}
}

func postThreadMessage(threadID uint32, message uint32) error {
	ok, _, errno := syscall.SyscallN(procPostThreadMessage.Addr(), uintptr(threadID), uintptr(message), 0, 0)
	if ok == 0 {
		return fmt.Errorf("PostThreadMessageW(%#x): %w", message, errno)
	}
	return nil
}

func (h *Hook) installHooks() error {
	mouse, err := setWindowsHook(whMouseLL, mouseProcOnce(), h.instance)
	if err != nil {
		return err
	}
	key, err := setWindowsHook(whKeyboardLL, keyProcOnce(), h.instance)
	if err != nil {
		return errors.Join(err, h.unhook(mouse))
	}
	h.hookMouse, h.hookKey = mouse, key
	return nil
}

func (h *Hook) removeHooks() error {
	mouse, key := h.hookMouse, h.hookKey
	h.hookMouse, h.hookKey = 0, 0
	var errs []error
	for _, handle := range []uintptr{mouse, key} {
		if handle != 0 {
			errs = append(errs, h.unhook(handle))
		}
	}
	return errors.Join(errs...)
}

// The OS removes a timed-out hook without notice, so unhooking a stale handle is an expected, counted outcome.
func (h *Hook) unhook(handle uintptr) error {
	ok, _, errno := syscall.SyscallN(procUnhookWindowsHookEx.Addr(), handle)
	if ok != 0 {
		return nil
	}
	if errors.Is(errno, windows.ERROR_INVALID_HOOK_HANDLE) {
		h.staleUnhooks.Add(1)
		return nil
	}
	return fmt.Errorf("UnhookWindowsHookEx: %w", errno)
}

func (h *Hook) reinstall() error {
	if err := h.removeHooks(); err != nil {
		return fmt.Errorf("reinstall: %w", err)
	}
	if err := h.installHooks(); err != nil {
		return fmt.Errorf("reinstall: %w", err)
	}
	h.reinstalls.Add(1)
	h.lastMarkerNs.Store(int64(time.Since(h.epoch)))
	return nil
}

func setWindowsHook(id int32, proc, instance uintptr) (uintptr, error) {
	handle, _, errno := syscall.SyscallN(procSetWindowsHookEx.Addr(), uintptr(id), proc, instance, 0)
	if handle == 0 {
		return 0, fmt.Errorf("SetWindowsHookExW(%d): %w", id, errno)
	}
	return handle, nil
}

func queryPerformanceFrequency() (int64, error) {
	var freq int64
	ok, _, errno := syscall.SyscallN(procQueryPerformanceFrequency.Addr(), uintptr(unsafe.Pointer(&freq)))
	if ok == 0 || freq <= 0 {
		return 0, fmt.Errorf("QueryPerformanceFrequency: %w", errno)
	}
	return freq, nil
}

func (h *Hook) qpc() int64 {
	syscall.SyscallN(procQueryPerformanceCounter.Addr(), uintptr(unsafe.Pointer(&h.qpcScratch)))
	return h.qpcScratch
}

func (h *Hook) recordDuration(start int64) {
	elapsed := h.qpc() - start
	h.hist.Record(time.Duration(elapsed * int64(time.Second) / h.qpcFreq))
}

func callNextHookEx(handle uintptr, nCode int32, wParam, lParam uintptr) uintptr {
	result, _, _ := syscall.SyscallN(procCallNextHookEx.Addr(), handle, uintptr(nCode), wParam, lParam)
	return result
}

func pointerFrom(address uintptr) unsafe.Pointer {
	return *(*unsafe.Pointer)(unsafe.Pointer(&address))
}

func mouseTrampoline(nCode int32, wParam, lParam uintptr) uintptr {
	h := activeHook.Load()
	if h == nil {
		return callNextHookEx(0, nCode, wParam, lParam)
	}
	return h.onMouse(nCode, wParam, lParam)
}

func keyTrampoline(nCode int32, wParam, lParam uintptr) uintptr {
	h := activeHook.Load()
	if h == nil {
		return callNextHookEx(0, nCode, wParam, lParam)
	}
	return h.onKey(nCode, wParam, lParam)
}

func (h *Hook) stallIfArmed() {
	if h.stallArmed.Load() && h.stallArmed.CompareAndSwap(true, false) {
		time.Sleep(h.stall)
	}
}

func (h *Hook) noteMarker() {
	h.lastMarkerNs.Store(int64(time.Since(h.epoch)))
}

func (h *Hook) onMouse(nCode int32, wParam, lParam uintptr) uintptr {
	if nCode < 0 {
		return callNextHookEx(h.hookMouse, nCode, wParam, lParam)
	}
	start := h.qpc()
	h.stallIfArmed()
	ms := (*msllHookStruct)(pointerFrom(lParam))

	var result uintptr
	if ms.Flags&LLMHFInjected != 0 && ms.ExtraInfo == MarkerExtraInfo {
		h.markerMouse.Add(1)
		h.noteMarker()
		result = callNextHookEx(h.hookMouse, nCode, wParam, lParam)
	} else {
		h.events.Add(1)
		h.sink.Push(Event{
			Kind:    KindMouse,
			Message: uint32(wParam),
			X:       ms.Pt.X,
			Y:       ms.Pt.Y,
			Data:    ms.MouseData,
			Flags:   ms.Flags,
			Time:    ms.Time,
			Extra:   ms.ExtraInfo,
			QPC:     start,
		})
		if h.suppress.Load() {
			result = 1
		} else {
			result = callNextHookEx(h.hookMouse, nCode, wParam, lParam)
		}
	}
	h.recordDuration(start)
	return result
}

func (h *Hook) onKey(nCode int32, wParam, lParam uintptr) uintptr {
	if nCode < 0 {
		return callNextHookEx(h.hookKey, nCode, wParam, lParam)
	}
	start := h.qpc()
	h.stallIfArmed()
	kb := (*kbdllHookStruct)(pointerFrom(lParam))

	var result uintptr
	switch {
	case kb.Flags&llkhfInjected != 0 && kb.ExtraInfo == MarkerExtraInfo:
		h.markerKey.Add(1)
		h.noteMarker()
		result = callNextHookEx(h.hookKey, nCode, wParam, lParam)
	case kb.VkCode == vkScroll:
		h.events.Add(1)
		h.pushKey(kb, wParam, start)
		h.toggleSuppressionOnScrollLock(wParam)
		result = callNextHookEx(h.hookKey, nCode, wParam, lParam)
	default:
		h.events.Add(1)
		h.pushKey(kb, wParam, start)
		if h.suppress.Load() {
			result = 1
		} else {
			result = callNextHookEx(h.hookKey, nCode, wParam, lParam)
		}
	}
	h.recordDuration(start)
	return result
}

func (h *Hook) pushKey(kb *kbdllHookStruct, wParam uintptr, start int64) {
	h.sink.Push(Event{
		Kind:    KindKey,
		Message: uint32(wParam),
		Data:    kb.VkCode,
		Scan:    kb.ScanCode,
		Flags:   kb.Flags,
		Time:    kb.Time,
		Extra:   kb.ExtraInfo,
		QPC:     start,
	})
}

func (h *Hook) toggleSuppressionOnScrollLock(wParam uintptr) {
	switch wParam {
	case wmKeyDown, wmSysKeyDown:
		if !h.scrollDown {
			h.scrollDown = true
			h.suppress.Store(!h.suppress.Load())
		}
	case wmKeyUp, wmSysKeyUp:
		h.scrollDown = false
	}
}
