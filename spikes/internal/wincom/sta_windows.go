package wincom

import (
	"errors"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	wmQuit      = 0x0012
	wmUser      = 0x0400
	wmWake      = 0x8000
	pmNoRemove  = 0x0000
	getMsgError = -1
)

var (
	ErrSTAAlreadyStarted = errors.New("wincom: STA thread was already started")
	ErrSTANotRunning     = errors.New("wincom: STA thread is not running")
	ErrSTAClosed         = errors.New("wincom: STA thread stopped before the call completed")
	ErrSTACloseOnThread  = errors.New("wincom: STA thread cannot be closed from itself")
)

var (
	procGetMessage        = user32.NewProc("GetMessageW")
	procPeekMessage       = user32.NewProc("PeekMessageW")
	procTranslateMessage  = user32.NewProc("TranslateMessage")
	procDispatchMessage   = user32.NewProc("DispatchMessageW")
	procPostThreadMessage = user32.NewProc("PostThreadMessageW")
	procOleInitialize     = ole32.NewProc("OleInitialize")
	procOleUninitialize   = ole32.NewProc("OleUninitialize")
)

// msg mirrors MSG (48 bytes on amd64).
type msg struct {
	Hwnd    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	PointX  int32
	PointY  int32
}

type staState int

const (
	staIdle staState = iota
	staStarting
	staRunning
	staStopped
)

type staTask struct {
	fn   func()
	done chan struct{}
}

// STA owns one OS thread that runs OleInitialize and a message pump, and executes submitted functions on it. All COM
// objects that Windows calls into must be created and used on this thread.
type STA struct {
	qmu      sync.Mutex
	state    staState
	queue    []staTask
	threadID atomic.Uint32
	stopped  chan struct{}
	exitErr  error

	initialize   func() error
	uninitialize func()
}

func (s *STA) Start() error {
	s.qmu.Lock()
	if s.state != staIdle {
		s.qmu.Unlock()
		return ErrSTAAlreadyStarted
	}
	s.state = staStarting
	s.stopped = make(chan struct{})
	s.qmu.Unlock()

	ready := make(chan error, 1)
	go s.run(ready)
	if err := <-ready; err != nil {
		<-s.stopped
		return err
	}
	return nil
}

func (s *STA) run(ready chan<- error) {
	// The thread is never unlocked: it exits together with this goroutine, after OleUninitialize.
	runtime.LockOSThread()
	defer func() {
		s.qmu.Lock()
		s.state = staStopped
		s.qmu.Unlock()
		close(s.stopped)
	}()

	var m msg
	// Posting thread messages requires the thread to own a message queue; PeekMessage creates it.
	procPeekMessage.Call(uintptr(unsafe.Pointer(&m)), 0, wmUser, wmUser, pmNoRemove)
	if err := s.oleInitialize(); err != nil {
		ready <- err
		return
	}
	defer s.oleUninitialize()

	s.threadID.Store(windows.GetCurrentThreadId())
	s.qmu.Lock()
	s.state = staRunning
	s.qmu.Unlock()
	ready <- nil
	s.exitErr = s.pump(&m)
}

func (s *STA) oleInitialize() error {
	if s.initialize != nil {
		return s.initialize()
	}
	hr, _, _ := procOleInitialize.Call(0)
	if result := HResult(hr); result.Failed() {
		return fmt.Errorf("OleInitialize: %w", result)
	}
	return nil
}

func (s *STA) oleUninitialize() {
	if s.uninitialize != nil {
		s.uninitialize()
		return
	}
	procOleUninitialize.Call()
}

// pump drains the work queue before every blocking wait, so a wake-up message swallowed by a nested modal loop
// (DoDragDrop, menus) cannot strand work.
func (s *STA) pump(m *msg) error {
	for {
		s.drain()
		ret, _, callErr := procGetMessage.Call(uintptr(unsafe.Pointer(m)), 0, 0, 0)
		switch int32(ret) {
		case 0:
			return nil
		case getMsgError:
			return fmt.Errorf("GetMessageW: %w", callErr)
		}
		if m.Message == wmWake {
			continue
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(m)))
		procDispatchMessage.Call(uintptr(unsafe.Pointer(m)))
	}
}

func (s *STA) drain() {
	s.qmu.Lock()
	tasks := s.queue
	s.queue = nil
	s.qmu.Unlock()
	for _, task := range tasks {
		task.fn()
		close(task.done)
	}
}

func (s *STA) onThread() bool {
	id := s.threadID.Load()
	return id != 0 && id == windows.GetCurrentThreadId()
}

func postThreadMessage(threadID uint32, message uint32) error {
	ok, _, callErr := procPostThreadMessage.Call(uintptr(threadID), uintptr(message), 0, 0)
	if uint32(ok) == 0 {
		return fmt.Errorf("PostThreadMessageW: %w", callErr)
	}
	return nil
}

// Do runs fn on the STA thread and returns after it completed. Called from the STA thread itself, fn runs inline.
func (s *STA) Do(fn func()) error {
	if s.onThread() {
		fn()
		return nil
	}
	task := staTask{fn: fn, done: make(chan struct{})}
	s.qmu.Lock()
	if s.state != staRunning {
		s.qmu.Unlock()
		return ErrSTANotRunning
	}
	s.queue = append(s.queue, task)
	s.qmu.Unlock()
	if err := postThreadMessage(s.threadID.Load(), wmWake); err != nil {
		return err
	}
	select {
	case <-task.done:
		return nil
	case <-s.stopped:
		select {
		case <-task.done:
			return nil
		default:
			return ErrSTAClosed
		}
	}
}

// Close stops the pump and waits for the thread to finish. It returns the error that ended the pump, if any.
func (s *STA) Close() error {
	if s.onThread() {
		return ErrSTACloseOnThread
	}
	s.qmu.Lock()
	switch s.state {
	case staIdle:
		s.state = staStopped
		s.qmu.Unlock()
		return nil
	case staStarting:
		s.qmu.Unlock()
		return ErrSTANotRunning
	case staRunning:
		s.qmu.Unlock()
		if err := postThreadMessage(s.threadID.Load(), wmQuit); err != nil {
			return err
		}
	default:
		s.qmu.Unlock()
	}
	<-s.stopped
	return s.exitErr
}
