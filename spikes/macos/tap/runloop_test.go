package tap

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

var errInstall = errors.New("install failed")

// fakeRunLoop stands in for CoreFoundation. dead flips when the loop thread reaches teardown, which is the point
// after which the real run loop may be deallocated; any stop, wakeUp or release observed then is the crash window
// of the SIGTRAP in the CI log.
type fakeRunLoop struct {
	t        *testing.T
	running  atomic.Bool
	stopped  chan struct{}
	stopOnce sync.Once
	dead     atomic.Bool
	retained atomic.Int32
	released atomic.Int32
	enterRun time.Duration
}

func newFakeRunLoop(t *testing.T, enterRun time.Duration) *fakeRunLoop {
	return &fakeRunLoop{t: t, stopped: make(chan struct{}), enterRun: enterRun}
}

func (f *fakeRunLoop) guard(call string) {
	f.t.Helper()
	if f.dead.Load() {
		f.t.Errorf("%s called after the loop thread exited", call)
	}
}

func (f *fakeRunLoop) api() *loopAPI {
	return &loopAPI{
		getCurrent: func() uintptr { return 0x1000 },
		run: func() {
			time.Sleep(f.enterRun)
			f.running.Store(true)
			<-f.stopped
		},
		stop: func(uintptr) {
			f.guard("CFRunLoopStop")
			if f.running.Load() {
				f.stopOnce.Do(func() { close(f.stopped) })
			}
		},
		wakeUp: func(uintptr) {
			f.guard("CFRunLoopWakeUp")
			time.Sleep(time.Millisecond)
			f.guard("CFRunLoopWakeUp (end)")
		},
		addSource:    func(_, _, _ uintptr) {},
		removeSource: func(_, _, _ uintptr) {},
		retain: func(ref uintptr) uintptr {
			f.retained.Add(1)
			return ref
		},
		release: func(uintptr) {
			f.released.Add(1)
			if !f.dead.Load() {
				f.t.Error("run loop reference released before the loop thread exited")
			}
		},
	}
}

func (f *fakeRunLoop) install() (uintptr, func(), error) {
	return 0x2000, func() { f.dead.Store(true) }, nil
}

func TestLoop_StopNeverTouchesExitedLoop(t *testing.T) {
	for range 200 {
		f := newFakeRunLoop(t, 0)
		l, err := startLoop(context.Background(), f.api(), f.install)
		if err != nil {
			t.Fatalf("startLoop: %v", err)
		}
		var wg sync.WaitGroup
		for range 4 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				l.stop()
			}()
		}
		wg.Wait()
		l.stop()

		if f.retained.Load() != 1 || f.released.Load() != 1 {
			t.Fatalf("retained %d, released %d, want 1 and 1", f.retained.Load(), f.released.Load())
		}
		if !f.dead.Load() {
			t.Fatal("stop returned before the loop thread exited")
		}
	}
}

func TestLoop_StopRetriesUntilRunLoopIsEntered(t *testing.T) {
	f := newFakeRunLoop(t, 40*time.Millisecond)
	l, err := startLoop(context.Background(), f.api(), f.install)
	if err != nil {
		t.Fatalf("startLoop: %v", err)
	}
	l.stop()
	if !f.dead.Load() {
		t.Fatal("stop returned before the loop thread exited")
	}
}

func TestStartLoop_InstallErrorIsReportedWithoutRetain(t *testing.T) {
	f := newFakeRunLoop(t, 0)
	failing := func() (uintptr, func(), error) { return 0, nil, errInstall }
	l, err := startLoop(context.Background(), f.api(), failing)
	if !errors.Is(err, errInstall) {
		t.Fatalf("startLoop error = %v, want %v", err, errInstall)
	}
	if l != nil {
		t.Fatal("startLoop returned a loop together with an error")
	}
	if f.retained.Load() != 0 || f.released.Load() != 0 {
		t.Fatalf("retained %d, released %d, want 0 and 0", f.retained.Load(), f.released.Load())
	}
}

func TestStartLoop_CancelledContextStopsTheThread(t *testing.T) {
	f := newFakeRunLoop(t, 0)
	gate := make(chan struct{})
	blocked := func() (uintptr, func(), error) {
		<-gate
		return f.install()
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	time.AfterFunc(50*time.Millisecond, func() { close(gate) })

	l, err := startLoop(ctx, f.api(), blocked)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("startLoop error = %v, want %v", err, context.Canceled)
	}
	if l != nil {
		t.Fatal("startLoop returned a loop together with an error")
	}
	if !f.dead.Load() {
		t.Fatal("startLoop returned before the loop thread exited")
	}
	if f.retained.Load() != 1 || f.released.Load() != 1 {
		t.Fatalf("retained %d, released %d, want 1 and 1", f.retained.Load(), f.released.Load())
	}
}
