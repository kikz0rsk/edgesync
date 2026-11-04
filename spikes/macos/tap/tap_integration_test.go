//go:build integration && darwin

package tap

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/ebitengine/purego"

	"github.com/kikz0r/edgesync/spikes/macos/fw"
)

const (
	testUserData    int64 = 0x45444753
	observeTimeout        = 2 * time.Second
	startTimeout          = 5 * time.Second
	pollInterval          = 5 * time.Millisecond
	settleQuiet           = 150 * time.Millisecond
	closeRounds           = 50
	mouseButtonLeft       = 0
)

type poster struct {
	createSource func(stateID int32) uintptr
	createMouse  func(source uintptr, mouseType uint32, position fw.CGPoint, button uint32) uintptr
	setField     func(event uintptr, field uint32, value int64)
	post         func(location uint32, event uintptr)
	release      func(ref uintptr)
}

func newPoster(t *testing.T, lib *fw.Lib) *poster {
	t.Helper()
	s := symbols{handle: lib.Handles.CoreGraphics}
	p := &poster{release: lib.CoreFoundation.Release}
	s.bind(&p.createSource, "CGEventSourceCreate")
	s.bind(&p.createMouse, "CGEventCreateMouseEvent")
	s.bind(&p.setField, "CGEventSetIntegerValueField")
	s.bind(&p.post, "CGEventPost")
	if s.err != nil {
		t.Fatalf("bind posting functions: %v", s.err)
	}
	return p
}

// postMouseMoved posts a tagged mouse-moved event at the current cursor position, so the cursor does not move.
func (p *poster) postMouseMoved(t *testing.T, at fw.CGPoint) {
	t.Helper()
	const combinedSessionState = 0
	source := p.createSource(combinedSessionState)
	if source == 0 {
		t.Fatal("CGEventSourceCreate returned NULL")
	}
	defer p.release(source)
	event := p.createMouse(source, uint32(EventMouseMoved), at, mouseButtonLeft)
	if event == 0 {
		t.Fatal("CGEventCreateMouseEvent returned NULL")
	}
	defer p.release(event)
	p.setField(event, FieldEventSourceUserData, testUserData)
	p.post(uint32(LocationHID), event)
}

func waitForTagged(events <-chan Event) (Event, bool) {
	deadline := time.After(observeTimeout)
	for {
		select {
		case e := <-events:
			if e.UserData == testUserData {
				return e, true
			}
		case <-deadline:
			return Event{}, false
		}
	}
}

func TestTap_CreateSessionTapReportsTrustState(t *testing.T) {
	lib, err := fw.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	perms, err := NewPermissions(lib)
	if err != nil {
		t.Fatalf("NewPermissions: %v", err)
	}
	state := perms.Preflight()
	t.Logf("TCC state: %s", state)
	t.Logf("AXIsProcessTrusted (legacy symbol)=%t", perms.AccessibilityTrustedLegacy())

	if callbackAddress() == 0 {
		t.Fatal("trampoline address is zero")
	}
	if callbackAddress() != callbackAddress() {
		t.Fatal("trampoline is not created once")
	}

	post := newPoster(t, lib)
	for _, variant := range []struct {
		name       string
		listenOnly bool
	}{
		{"active", false},
		{"listen-only", true},
	} {
		t.Run(variant.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), startTimeout)
			defer cancel()
			tap, err := Start(ctx, lib, Config{Location: LocationSession, ListenOnly: variant.listenOnly, Buffer: 256})
			if errors.Is(err, ErrTapCreate) {
				t.Logf("session tap listenOnly=%t was not created: %v (TCC state: %s)", variant.listenOnly, err, state)
				return
			}
			if err != nil {
				t.Fatalf("Start: %v", err)
			}
			t.Logf("session tap listenOnly=%t created", variant.listenOnly)
			defer func() {
				tap.Close()
				if n := registeredTaps(); n != 0 {
					t.Errorf("dispatch table holds %d taps after Close, want 0", n)
				}
			}()

			if !tap.Enabled() {
				t.Error("tap is not enabled after creation")
			}
			observeSyntheticEvent(t, lib, post, tap, state)
			exerciseReenable(t, tap)
			if !variant.listenOnly {
				exerciseSuppression(t, lib, post, tap)
			}
		})
	}
}

func observeSyntheticEvent(t *testing.T, lib *fw.Lib, post *poster, tap *Tap, state State) {
	t.Helper()
	at, err := lib.CoreGraphics.CursorLocation(lib.CoreFoundation)
	if err != nil {
		t.Fatalf("CursorLocation: %v", err)
	}
	post.postMouseMoved(t, at)
	event, seen := waitForTagged(tap.Events())
	switch {
	case seen:
		t.Logf("synthetic mouse-moved observed: type=%d location=(%g,%g) userData=%#x", event.Type, event.Location.X, event.Location.Y, event.UserData)
	case state.PostAccess:
		t.Errorf("synthetic mouse-moved was not observed although CGPreflightPostEventAccess is true")
	default:
		t.Logf("synthetic mouse-moved not observed (CGPreflightPostEventAccess=false, posting is expected to be dropped)")
	}
}

type disableCounters struct {
	timeout, userInput, reenabled uint64
}

func disableCountersOf(s Stats) disableCounters {
	return disableCounters{timeout: s.DisabledByTimeout, userInput: s.DisabledByUserInput, reenabled: s.Reenabled}
}

func waitFor(timeout time.Duration, condition func() bool) bool {
	deadline := time.Now().Add(timeout)
	for !condition() {
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(pollInterval)
	}
	return true
}

// settled waits until the tap is enabled and the disable counters stop moving, so a late notification from the
// previous step cannot land in the next step's window.
func settled(tap *Tap) bool {
	last := disableCountersOf(tap.Stats())
	stableSince := time.Now()
	return waitFor(observeTimeout, func() bool {
		current := disableCountersOf(tap.Stats())
		if current != last {
			last, stableSince = current, time.Now()
			return false
		}
		return time.Since(stableSince) >= settleQuiet && tap.Enabled()
	})
}

// exerciseReenable calls the registered C trampoline the way the window server does, after disabling the tap.
// The CI run showed two notifications per step: our synthetic call plus one kCGEventTapDisabledByUserInput that
// macOS delivers on the loop thread after CGEventTapEnable(false). The handler re-enables on each, so the disabled
// state may be gone before it can be read; the test therefore polls instead of reading it right after the call.
func exerciseReenable(t *testing.T, tap *Tap) {
	t.Helper()
	var callTrampoline func(proxy uintptr, eventType uint32, event, refcon uintptr) uintptr
	purego.RegisterFunc(&callTrampoline, callbackAddress())

	for _, c := range []struct {
		name      string
		eventType EventType
		count     func(disableCounters) uint64
	}{
		{"timeout", EventTapDisabledByTimeout, func(d disableCounters) uint64 { return d.timeout }},
		{"user input", EventTapDisabledByUserInput, func(d disableCounters) uint64 { return d.userInput }},
	} {
		if !settled(tap) {
			t.Fatalf("%s: tap did not settle as enabled before the step", c.name)
		}
		before := disableCountersOf(tap.Stats())

		tap.api.enable(tap.port, false)
		reacted := waitFor(observeTimeout, func() bool {
			return !tap.Enabled() || tap.Stats().DisabledByUserInput > before.userInput
		})
		if !reacted {
			t.Fatalf("%s: CGEventTapEnable(false) had no visible effect within %v", c.name, observeTimeout)
		}

		callTrampoline(0, uint32(c.eventType), 0, tap.refcon)
		if !settled(tap) {
			t.Fatalf("%s: tap was not re-enabled and settled within %v", c.name, observeTimeout)
		}
		after := disableCountersOf(tap.Stats())
		t.Logf("%s: timeout %d -> %d, user input %d -> %d, reenabled %d -> %d", c.name,
			before.timeout, after.timeout, before.userInput, after.userInput, before.reenabled, after.reenabled)

		notifications := after.timeout - before.timeout + after.userInput - before.userInput
		if own := c.count(after) - c.count(before); own < 1 {
			t.Errorf("%s: own disable counter moved by %d, want at least 1", c.name, own)
		}
		if notifications < 1 || notifications > 2 {
			t.Errorf("%s: %d disable notifications, want 1 (synthetic) or 2 (synthetic plus the macOS notification)", c.name, notifications)
		}
		if got := after.reenabled - before.reenabled; got != notifications {
			t.Errorf("%s: reenabled moved by %d, want one per notification (%d)", c.name, got, notifications)
		}
	}
}

func TestTap_CloseIsIdempotentAndRaceFree(t *testing.T) {
	lib, err := fw.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	for i := range closeRounds {
		ctx, cancel := context.WithTimeout(context.Background(), startTimeout)
		tap, err := Start(ctx, lib, Config{Location: LocationSession, ListenOnly: true})
		cancel()
		if errors.Is(err, ErrTapCreate) {
			t.Logf("session tap was not created: %v; Close is not exercised", err)
			return
		}
		if err != nil {
			t.Fatalf("round %d: Start: %v", i, err)
		}

		var wg sync.WaitGroup
		for range 4 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				tap.Close()
			}()
		}
		wg.Wait()
		tap.Close()
		if tap.Enabled() {
			t.Errorf("round %d: tap reports enabled after Close", i)
		}
	}
	if n := registeredTaps(); n != 0 {
		t.Errorf("dispatch table holds %d taps after the rounds, want 0", n)
	}
}

func exerciseSuppression(t *testing.T, lib *fw.Lib, post *poster, tap *Tap) {
	t.Helper()
	if err := tap.SetSuppress(true); err != nil {
		t.Fatalf("SetSuppress(true): %v", err)
	}
	defer func() {
		if err := tap.SetSuppress(false); err != nil {
			t.Errorf("SetSuppress(false): %v", err)
		}
	}()

	before := tap.Stats().Suppressed
	at, err := lib.CoreGraphics.CursorLocation(lib.CoreFoundation)
	if err != nil {
		t.Fatalf("CursorLocation: %v", err)
	}
	post.postMouseMoved(t, at)
	if _, seen := waitForTagged(tap.Events()); !seen {
		t.Logf("suppression not exercised: synthetic event not observed")
		return
	}
	if got := tap.Stats().Suppressed; got <= before {
		t.Errorf("suppressed counter %d -> %d, want an increase", before, got)
	}
}

func TestTap_ListenOnlyRejectsSuppression(t *testing.T) {
	tap := &Tap{listenOnly: true}
	if err := tap.SetSuppress(true); !errors.Is(err, ErrListenOnly) {
		t.Errorf("SetSuppress(true) error = %v, want %v", err, ErrListenOnly)
	}
	if err := tap.SetSuppress(false); err != nil {
		t.Errorf("SetSuppress(false) error = %v, want nil", err)
	}
}

func TestPermissions_PromptAndRequestReturnValues(t *testing.T) {
	lib, err := fw.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	perms, err := NewPermissions(lib)
	if err != nil {
		t.Fatalf("NewPermissions: %v", err)
	}
	prompting, err := perms.AccessibilityTrustedPrompting()
	if err != nil {
		t.Fatalf("AccessibilityTrustedPrompting: %v", err)
	}
	t.Logf("AXIsProcessTrustedWithOptions(prompt)=%t CGRequestListenEventAccess=%t CGRequestPostEventAccess=%t",
		prompting, perms.RequestListenAccess(), perms.RequestPostAccess())
}

func TestCursor_AssociateAndHideReturnCodes(t *testing.T) {
	lib, err := fw.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	cursor, err := NewCursor(lib)
	if err != nil {
		t.Fatalf("NewCursor: %v", err)
	}
	displays, err := lib.CoreGraphics.ActiveDisplays()
	if err != nil {
		t.Fatalf("ActiveDisplays: %v", err)
	}
	var mainID uint32
	for _, d := range displays {
		if d.Main {
			mainID = d.ID
		}
	}
	if mainID == 0 {
		t.Fatal("no main display")
	}

	t.Logf("CGAssociateMouseAndMouseCursorPosition(false)=%d", cursor.Associate(false))
	t.Logf("CGAssociateMouseAndMouseCursorPosition(true)=%d", cursor.Associate(true))
	t.Logf("CGDisplayHideCursor=%d", cursor.Hide(mainID))
	t.Logf("CGDisplayShowCursor=%d", cursor.Show(mainID))
}
