// Package ui drives the NSApplication run loop, an NSStatusItem tray and an overlay NSPanel through purego for the
// macOS tray spike (D-18, D-84, D-106). Everything runs without cgo.
//
// AppKit is main-thread only. The program must call runtime.LockOSThread in an init function so that the main
// goroutine stays on the process main thread, and must create every object in this package and call App.Run there.
// Work started from other goroutines reaches the main thread through App.Post.
package ui

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"

	"github.com/kikz0r/edgesync/spikes/macos/fw"
)

const libSystemPath = "/usr/lib/libSystem.B.dylib"

// The numeric values of these AppKit constants are not stated on Apple's documentation pages, which only name them
// (second-source: SDK headers as transcribed by progrium/macdriver enumtypes.gen.go (MIT) and, for the activation
// policy and event type, servo/core-foundation-rs cocoa appkit.rs (MIT/Apache-2.0)).
const (
	activationPolicyAccessory   = 1  // NSApplicationActivationPolicyAccessory
	eventTypeApplicationDefined = 15 // NSEventTypeApplicationDefined
)

var (
	ErrNotMainThread    = errors.New("not on the process main thread")
	ErrStopped          = errors.New("application run loop is not running")
	ErrActivationPolicy = errors.New("setActivationPolicy: rejected the accessory policy")
	ErrNilObject        = errors.New("Objective-C call returned nil")
	ErrSymbolNull       = errors.New("libSystem symbol resolved to NULL")
)

type baseSelectors struct {
	alloc, init, release, retain, drain objc.SEL
}

type appSelectors struct {
	sharedApplication, setActivationPolicy, run, stop, postEvent objc.SEL
	isMainThread, otherEvent                                     objc.SEL
}

type dispatchAPI struct {
	asyncF    func(queue, context, work uintptr)
	mainQueue uintptr
}

func bindDispatch() (dispatchAPI, error) {
	handle, err := purego.Dlopen(libSystemPath, purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		return dispatchAPI{}, fmt.Errorf("open %s: %w", libSystemPath, err)
	}
	asyncF, err := purego.Dlsym(handle, "dispatch_async_f")
	if err != nil {
		return dispatchAPI{}, fmt.Errorf("resolve dispatch_async_f: %w", err)
	}
	// dispatch_get_main_queue() is an inline function returning &_dispatch_main_q (libdispatch queue.h), so the
	// exported symbol's address is the queue itself, not a pointer to read through.
	mainQueue, err := purego.Dlsym(handle, "_dispatch_main_q")
	if err != nil {
		return dispatchAPI{}, fmt.Errorf("resolve _dispatch_main_q: %w", err)
	}
	if mainQueue == 0 {
		return dispatchAPI{}, fmt.Errorf("_dispatch_main_q: %w", ErrSymbolNull)
	}
	api := dispatchAPI{mainQueue: mainQueue}
	purego.RegisterFunc(&api.asyncF, asyncF)
	return api, nil
}

// App owns NSApplication, the accessory activation policy and the Go-to-main-thread work queue.
type App struct {
	base     baseSelectors
	sel      appSelectors
	pool     objc.Class
	nsApp    objc.ID
	nsEvent  objc.Class
	nsThread objc.Class
	dispatch dispatchAPI
	drainFn  uintptr

	mu      sync.Mutex
	pending []func() error
	running bool
	stopped bool
	failure error
}

// NewApp must run on the main thread after fw.Load has opened AppKit. The drain callback is a purego callback that
// is never freed, so one App is created per process.
func NewApp() (*App, error) {
	a := &App{
		base: baseSelectors{
			alloc:   fw.Sel("alloc"),
			init:    fw.Sel("init"),
			release: fw.Sel("release"),
			retain:  fw.Sel("retain"),
			drain:   fw.Sel("drain"),
		},
		sel: appSelectors{
			sharedApplication:   fw.Sel("sharedApplication"),
			setActivationPolicy: fw.Sel("setActivationPolicy:"),
			run:                 fw.Sel("run"),
			stop:                fw.Sel("stop:"),
			postEvent:           fw.Sel("postEvent:atStart:"),
			isMainThread:        fw.Sel("isMainThread"),
			otherEvent:          fw.Sel("otherEventWithType:location:modifierFlags:timestamp:windowNumber:context:subtype:data1:data2:"),
		},
	}
	classes := []struct {
		name string
		dst  *objc.Class
	}{
		{"NSAutoreleasePool", &a.pool},
		{"NSEvent", &a.nsEvent},
		{"NSThread", &a.nsThread},
	}
	for _, c := range classes {
		class, err := fw.Class(c.name)
		if err != nil {
			return nil, err
		}
		*c.dst = class
	}
	appClass, err := fw.Class("NSApplication")
	if err != nil {
		return nil, err
	}
	if err := a.requireMainThread(); err != nil {
		return nil, err
	}
	dispatch, err := bindDispatch()
	if err != nil {
		return nil, err
	}
	a.dispatch = dispatch
	a.drainFn = purego.NewCallback(a.drain)

	err = a.inPool(func() error {
		a.nsApp = objc.ID(appClass).Send(a.sel.sharedApplication)
		if a.nsApp == 0 {
			return fmt.Errorf("NSApplication sharedApplication: %w", ErrNilObject)
		}
		if !objc.Send[bool](a.nsApp, a.sel.setActivationPolicy, activationPolicyAccessory) {
			return ErrActivationPolicy
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return a, nil
}

func (a *App) requireMainThread() error {
	if !objc.Send[bool](objc.ID(a.nsThread), a.sel.isMainThread) {
		return ErrNotMainThread
	}
	return nil
}

// inPool runs fn inside an autorelease pool; a pool must be drained on the thread that created it.
func (a *App) inPool(fn func() error) error {
	pool := objc.ID(a.pool).Send(a.base.alloc).Send(a.base.init)
	defer pool.Send(a.base.drain)
	return fn()
}

// Post schedules fn on the main thread and returns at once. A failure of fn stops the run loop and is returned by
// Run. Post is safe from any goroutine, including the main thread.
func (a *App) Post(fn func() error) error {
	a.mu.Lock()
	if a.stopped {
		a.mu.Unlock()
		return ErrStopped
	}
	a.pending = append(a.pending, fn)
	a.mu.Unlock()
	a.dispatch.asyncF(a.dispatch.mainQueue, 0, a.drainFn)
	return nil
}

// Stop asks the run loop to end. It is idempotent and safe from any goroutine.
func (a *App) Stop() error {
	return a.Post(func() error {
		return a.stopLoop()
	})
}

// Fail records the first failure, stops the run loop and makes Run return it.
func (a *App) Fail(err error) {
	a.mu.Lock()
	if a.failure == nil {
		a.failure = err
	}
	a.mu.Unlock()
	if stopErr := a.Stop(); stopErr != nil && !errors.Is(stopErr, ErrStopped) {
		a.mu.Lock()
		a.failure = errors.Join(a.failure, stopErr)
		a.mu.Unlock()
	}
}

// Run blocks on the main thread inside [NSApp run] until Stop, a failed posted function, or ctx is done. It returns
// nil for an orderly stop and the first recorded failure otherwise.
func (a *App) Run(ctx context.Context) error {
	if err := a.requireMainThread(); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("run loop not started: %w", err)
	}
	a.mu.Lock()
	a.running = true
	a.mu.Unlock()

	runDone := make(chan struct{})
	watcherDone := make(chan struct{})
	go func() {
		defer close(watcherDone)
		select {
		case <-ctx.Done():
			a.requestStop()
		case <-runDone:
		}
	}()

	a.nsApp.Send(a.sel.run)

	close(runDone)
	<-watcherDone
	a.mu.Lock()
	a.running = false
	a.stopped = true
	a.pending = nil
	failure := a.failure
	a.mu.Unlock()
	return failure
}

func (a *App) requestStop() {
	if err := a.Stop(); err != nil && !errors.Is(err, ErrStopped) {
		a.Fail(err)
	}
}

// drain is the dispatch_async_f work function and always runs on the main thread. Each posted function gets its own
// autorelease pool because the work does not arrive through an AppKit event.
func (a *App) drain(uintptr) {
	a.mu.Lock()
	batch := a.pending
	a.pending = nil
	a.mu.Unlock()
	for _, fn := range batch {
		if err := a.inPool(fn); err != nil {
			a.Fail(err)
			return
		}
	}
}

// stopLoop must run on the main thread. stop: only takes effect after an event has been dispatched, so a
// synthetic application-defined event follows it (NSApplication stop: discussion).
func (a *App) stopLoop() error {
	a.nsApp.Send(a.sel.stop, objc.ID(0))
	event := objc.ID(a.nsEvent).Send(a.sel.otherEvent,
		uint(eventTypeApplicationDefined), fw.CGPoint{}, uint(0), float64(0), int(0), objc.ID(0), int16(0), int(0), int(0))
	if event == 0 {
		return fmt.Errorf("NSEvent otherEventWithType: %w", ErrNilObject)
	}
	a.nsApp.Send(a.sel.postEvent, event, true)
	return nil
}
