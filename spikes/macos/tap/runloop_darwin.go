package tap

import (
	"context"
	"fmt"
	"runtime"
	"sync/atomic"
	"time"

	"github.com/kikz0r/edgesync/spikes/macos/fw"
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
	release             func(ref uintptr)
	commonModes         uintptr
}

func bindLoopAPI(lib *fw.Lib) (*loopAPI, error) {
	s := symbols{handle: lib.Handles.CoreFoundation}
	api := &loopAPI{release: lib.CoreFoundation.Release}
	s.bind(&api.getCurrent, "CFRunLoopGetCurrent")
	s.bind(&api.run, "CFRunLoopRun")
	s.bind(&api.stop, "CFRunLoopStop")
	s.bind(&api.wakeUp, "CFRunLoopWakeUp")
	s.bind(&api.addSource, "CFRunLoopAddSource")
	s.bind(&api.removeSource, "CFRunLoopRemoveSource")
	s.bind(&api.createRunLoopSource, "CFMachPortCreateRunLoopSource")
	s.bind(&api.machPortInvalidate, "CFMachPortInvalidate")
	api.commonModes = s.pointerVariable("kCFRunLoopCommonModes")
	if s.err != nil {
		return nil, s.err
	}
	return api, nil
}

type loopInstall func() (source uintptr, teardown func(), err error)

type loop struct {
	api  *loopAPI
	ref  atomic.Uintptr
	done chan struct{}
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
		l.ref.Store(runLoopRef)
		started <- nil
		api.run()
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

// stop retries because CFRunLoopStop does nothing when the run loop has not entered CFRunLoopRun yet.
func (l *loop) stop() {
	ticker := time.NewTicker(stopRetryInterval)
	defer ticker.Stop()
	for {
		if ref := l.ref.Load(); ref != 0 {
			l.api.stop(ref)
			l.api.wakeUp(ref)
		}
		select {
		case <-l.done:
			return
		case <-ticker.C:
		}
	}
}
