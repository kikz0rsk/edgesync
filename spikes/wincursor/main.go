//go:build windows

// Command wincursor compares three ways to keep the local pointer still and hidden while input suppression is on
// (FEAT-034, D-50): ClipCursor confinement to a 1x1 rectangle, a 1x1 captured window with a transparent cursor class,
// and per-monitor layered windows with a transparent cursor set in WM_SETCURSOR. Suppression itself is the
// low-level hook from internal/winhook, toggled with Scroll Lock. No strategy replaces a system cursor. Run by the
// user only (D-74); agents build it and never execute it.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"runtime"
	"sync/atomic"
	"time"

	"github.com/kikz0r/edgesync/spikes/internal/winhook"
)

const (
	wmKeyDown    = 0x0100
	wmKeyUp      = 0x0101
	wmSysKeyDown = 0x0104
	wmSysKeyUp   = 0x0105
	vkDigit1     = 0x31
	vkDigit3     = 0x33
	tickMillis   = 20
)

var errHookStopped = errors.New("low-level hook stopped")

type app struct {
	hook      *winhook.Hook
	ring      *winhook.Ring
	park      *parkClass
	parkers   [3]parker
	interrupt *atomic.Bool
	selected  int
	engaged   int
	heldDigit uint32
	engages   [3]uint64
	started   time.Time
	duration  time.Duration
}

func main() {
	runtime.LockOSThread()
	strategy := flag.Int("strategy", 1, "initial strategy: 1 ClipCursor, 2 captured window, 3 layered windows")
	duration := flag.Duration("duration", 0, "stop after this long; 0 runs until Ctrl+C")
	releaseOnly := flag.Bool("release", false, "release a ClipCursor confinement left by a killed run and exit")
	flag.Parse()
	if err := run(*strategy, *duration, *releaseOnly); err != nil {
		fmt.Fprintln(os.Stderr, "wincursor:", err)
		os.Exit(1)
	}
}

func run(initial int, duration time.Duration, releaseOnly bool) (err error) {
	if releaseOnly {
		return clipParker{}.release()
	}
	if initial < 1 || initial > 3 {
		return fmt.Errorf("-strategy must be 1, 2 or 3, got %d", initial)
	}
	if err := winhook.SetPerMonitorV2(); err != nil {
		return err
	}
	ring, err := winhook.NewRing(winhook.DefaultRingCapacity)
	if err != nil {
		return err
	}
	hook := winhook.New(winhook.Options{})
	if err := hook.Start(context.Background(), ring); err != nil {
		return err
	}
	defer func() { err = errors.Join(err, hook.Close()) }()

	park, err := newParkClass()
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, park.close()) }()

	var interrupt atomic.Bool
	if err := installInterrupt(&interrupt); err != nil {
		return err
	}
	defer func() { err = errors.Join(err, removeInterrupt()) }()

	a := &app{
		hook:      hook,
		ring:      ring,
		park:      park,
		parkers:   [3]parker{clipParker{}, captureParker{class: park}, layeredParker{class: park}},
		interrupt: &interrupt,
		selected:  initial - 1,
		engaged:   -1,
		started:   time.Now(),
		duration:  duration,
	}
	defer func() { err = errors.Join(err, a.releaseAll()) }()

	fmt.Printf("wincursor: started, %s selected\n", a.parkers[a.selected].name())
	fmt.Println("Scroll Lock toggles suppression, keys 1/2/3 select a strategy, Ctrl+C stops")
	if err := pumpMessages(tickMillis, a.tick); err != nil {
		return err
	}
	a.printSummary()
	return nil
}

func (a *app) tick() (bool, error) {
	if err := a.park.takeAsyncErr(); err != nil {
		return true, err
	}
	select {
	case <-a.hook.Done():
		return true, errors.Join(errHookStopped, a.hook.Err())
	default:
	}
	if err := a.drainKeys(); err != nil {
		return true, err
	}
	if a.interrupt.Load() {
		fmt.Println("wincursor: interrupted")
		return true, nil
	}
	if a.duration > 0 && time.Since(a.started) >= a.duration {
		return true, nil
	}
	return false, a.reconcile()
}

func (a *app) reconcile() error {
	suppressed := a.hook.Suppressed()
	switch {
	case suppressed && a.engaged < 0:
		return a.engage(a.selected)
	case !suppressed && a.engaged >= 0:
		return a.release()
	}
	return nil
}

func (a *app) drainKeys() error {
	for {
		ev, ok := a.ring.Pop()
		if !ok {
			return nil
		}
		if ev.Kind != winhook.KindKey {
			continue
		}
		switch ev.Message {
		case wmKeyDown, wmSysKeyDown:
			if ev.Data < vkDigit1 || ev.Data > vkDigit3 || ev.Data == a.heldDigit {
				continue
			}
			a.heldDigit = ev.Data
			if err := a.selectStrategy(int(ev.Data - vkDigit1)); err != nil {
				return err
			}
		case wmKeyUp, wmSysKeyUp:
			if ev.Data == a.heldDigit {
				a.heldDigit = 0
			}
		}
	}
}

func (a *app) selectStrategy(i int) error {
	if i == a.selected {
		return nil
	}
	a.selected = i
	fmt.Printf("selected %s\n", a.parkers[i].name())
	if a.engaged < 0 {
		return nil
	}
	if err := a.release(); err != nil {
		return err
	}
	return a.engage(i)
}

// The index is recorded before the call so a partial engage is still released by releaseAll.
func (a *app) engage(i int) error {
	a.engaged = i
	a.engages[i]++
	fmt.Printf("engage %s\n", a.parkers[i].name())
	return a.parkers[i].engage()
}

func (a *app) release() error {
	i := a.engaged
	a.engaged = -1
	fmt.Printf("release %s\n", a.parkers[i].name())
	return a.parkers[i].release()
}

func (a *app) releaseAll() error {
	var err error
	if a.engaged >= 0 {
		err = a.release()
	}
	return errors.Join(err, clipParker{}.release())
}

func (a *app) printSummary() {
	s := a.hook.Stats()
	fmt.Printf("summary: duration=%s events=%d drops=%d reinstalls=%d max callback=%s\n",
		time.Since(a.started).Round(time.Millisecond), s.Events, s.Drops, s.Reinstalls, s.MaxCallback)
	for i, p := range a.parkers {
		fmt.Printf("engaged %d time(s): %s\n", a.engages[i], p.name())
	}
}
