package tap

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"time"
)

const stopRetryInterval = 10 * time.Millisecond

type loopAPI struct {
	getCurrent          func() uintptr
	run                 func()
	stop                func(runLoop uintptr)
	wakeUp              func(runLoop uintptr)
	addSource           func(runLoop, source, mode uintptr)
	removeSource        func(runLoop, source, mode uintptr)
	createRunLoopSource func(allocator, port uintptr, order int) uintptr
	machPortInvalidate  func(port uintptr)
	retain              func(ref uintptr) uintptr
	release             func(ref uintptr)
	commonModes         uintptr
}

type loopInstall func() (source uintptr, teardown func(), err error)

// loop owns one CFRunLoop thread. ref is a retained reference to that thread's run loop (CFRunLoopGetCurrent follows
// the Get rule, so the loop takes its own reference). mu serialises every CoreFoundation call made on ref with the
// thread's transition to exited, so no call can reach a run loop whose thread is ending.
type loop struct {
	api      *loopAPI
	done     chan struct{}
	stopOnce sync.Once

	mu     sync.Mutex
	ref    uintptr
	exited bool
}

// startLoop runs install and CFRunLoopRun on one dedicated OS thread. A CFRunLoop belongs to the thread that
// created it, so everything touching the source happens on that thread.
func startLoop(ctx context.Context, api *loopAPI, install loopInstall) (*loop, error) {
	l := &loop{api: api, done: make(chan struct{})}
	started := make(chan error, 1)

	go func() {
		defer close(l.done)
		// The goroutine exits while locked, which makes the runtime discard this thread instead of reusing it.
		runtime.LockOSThread()
		runLoopRef := api.getCurrent()
		source, teardown, err := install()
		if err != nil {
			started <- err
			return
		}
		api.addSource(runLoopRef, source, api.commonModes)
		l.publish(api.retain(runLoopRef))
		started <- nil
		api.run()
		l.markExited()
		api.removeSource(runLoopRef, source, api.commonModes)
		teardown()
	}()

	select {
	case err := <-started:
		if err != nil {
			<-l.done
			return nil, fmt.Errorf("install run loop source: %w", err)
		}
		return l, nil
	case <-ctx.Done():
		l.stop()
		return nil, fmt.Errorf("start run loop: %w", ctx.Err())
	}
}

func (l *loop) publish(ref uintptr) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.ref = ref
}

func (l *loop) markExited() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.exited = true
}

func (l *loop) signalStop() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.exited || l.ref == 0 {
		return
	}
	l.api.stop(l.ref)
	l.api.wakeUp(l.ref)
}

func (l *loop) releaseRef() {
	l.mu.Lock()
	ref := l.ref
	l.ref = 0
	l.mu.Unlock()
	if ref != 0 {
		l.api.release(ref)
	}
}

// stop is idempotent and safe from any goroutine; concurrent callers all return after the thread has exited and the
// run loop reference is released. It retries because CFRunLoopStop does nothing when the run loop has not entered
// CFRunLoopRun yet.
func (l *loop) stop() {
	l.stopOnce.Do(func() {
		ticker := time.NewTicker(stopRetryInterval)
		defer ticker.Stop()
		for {
			l.signalStop()
			select {
			case <-l.done:
				l.releaseRef()
				return
			case <-ticker.C:
			}
		}
	})
}
