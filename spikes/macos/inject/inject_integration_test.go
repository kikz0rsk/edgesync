//go:build integration && darwin

package inject

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/kikz0r/edgesync/spikes/macos/fw"
	"github.com/kikz0r/edgesync/spikes/macos/tap"
)

const (
	testUserData    int64 = 0x45444753
	observeTimeout        = 2 * time.Second
	startTimeout          = 5 * time.Second
	cursorTolerance       = 1.0
	cursorPollStep        = 10 * time.Millisecond
)

func mainDisplayCentre(t *testing.T, lib *fw.Lib) fw.CGPoint {
	t.Helper()
	displays, err := lib.CoreGraphics.ActiveDisplays()
	if err != nil {
		t.Fatalf("ActiveDisplays: %v", err)
	}
	for _, d := range displays {
		if d.Main {
			return fw.CGPoint{
				X: d.Bounds.Origin.X + d.Bounds.Size.Width/2,
				Y: d.Bounds.Origin.Y + d.Bounds.Size.Height/2,
			}
		}
	}
	t.Fatal("main display is not among the active displays")
	return fw.CGPoint{}
}

func requireInt(t *testing.T, inj *Injector, event uintptr, field uint32, want int64, name string) {
	t.Helper()
	if got := inj.api.getInt(event, field); got != want {
		t.Errorf("%s = %d, want %d", name, got, want)
	}
}

func discard(inj *Injector, event uintptr) {
	inj.release(event)
}

// exerciseBuilders checks the fields of constructed events without posting them, which needs no TCC grant.
func exerciseBuilders(t *testing.T, inj *Injector, target fw.CGPoint) {
	t.Helper()

	move, err := inj.mouseMove(target, 3, -2)
	if err != nil {
		t.Fatalf("mouseMove: %v", err)
	}
	requireInt(t, inj, move, fieldMouseEventDeltaX, 3, "move deltaX")
	requireInt(t, inj, move, fieldMouseEventDeltaY, -2, "move deltaY")
	requireInt(t, inj, move, fieldEventSourceUserData, testUserData, "move userData")
	discard(inj, move)

	first, err := inj.mouseButton(target, ButtonLeft, true, 2)
	if err != nil {
		t.Fatalf("mouseButton left down: %v", err)
	}
	requireInt(t, inj, first, fieldMouseClickState, 2, "left down clickState")
	requireInt(t, inj, first, fieldMouseButtonNumber, int64(ButtonLeft), "left down buttonNumber")
	firstNumber := inj.api.getInt(first, fieldMouseEventNumber)
	discard(inj, first)

	second, err := inj.mouseButton(target, ButtonCenter, false, 1)
	if err != nil {
		t.Fatalf("mouseButton center up: %v", err)
	}
	requireInt(t, inj, second, fieldMouseButtonNumber, int64(ButtonCenter), "center up buttonNumber")
	if got := inj.api.getInt(second, fieldMouseEventNumber); got != firstNumber+1 {
		t.Errorf("event number %d follows %d, want %d", got, firstNumber, firstNumber+1)
	}
	discard(inj, second)

	key, err := inj.key(KeyA, true, FlagShift)
	if err != nil {
		t.Fatalf("key: %v", err)
	}
	requireInt(t, inj, key, fieldKeyboardKeycode, int64(KeyA), "key keycode")
	if got := inj.api.getFlags(key); got != uint64(FlagShift) {
		t.Errorf("key flags = %#x, want %#x", got, uint64(FlagShift))
	}
	requireInt(t, inj, key, fieldEventSourceUserData, testUserData, "key userData")
	discard(inj, key)

	scroll, err := inj.scroll(ScrollUnitPixel, 0, -12)
	if err != nil {
		t.Fatalf("scroll: %v", err)
	}
	requireInt(t, inj, scroll, fieldEventSourceUserData, testUserData, "scroll userData")
	discard(inj, scroll)
}

func exerciseRejections(t *testing.T, inj *Injector, target fw.CGPoint) {
	t.Helper()
	if _, err := inj.mouseButton(target, Button(7), true, 1); !errors.Is(err, ErrInvalidButton) {
		t.Errorf("invalid button error = %v, want %v", err, ErrInvalidButton)
	}
	if _, err := inj.scroll(ScrollUnit(9), 0, 1); !errors.Is(err, ErrInvalidScrollUnit) {
		t.Errorf("invalid scroll unit error = %v, want %v", err, ErrInvalidScrollUnit)
	}
}

func waitForCursor(lib *fw.Lib, target fw.CGPoint) (fw.CGPoint, bool) {
	deadline := time.Now().Add(observeTimeout)
	for {
		got, err := lib.CoreGraphics.CursorLocation(lib.CoreFoundation)
		if err == nil && math.Abs(got.X-target.X) <= cursorTolerance && math.Abs(got.Y-target.Y) <= cursorTolerance {
			return got, true
		}
		if time.Now().After(deadline) {
			return got, false
		}
		time.Sleep(cursorPollStep)
	}
}

func expectTagged(t *testing.T, events <-chan tap.Event, want tap.EventType, match func(tap.Event) bool) {
	t.Helper()
	deadline := time.After(observeTimeout)
	for {
		select {
		case e := <-events:
			if e.UserData == testUserData && e.Type == want && (match == nil || match(e)) {
				return
			}
		case <-deadline:
			t.Errorf("tagged event type %d not observed within %s", want, observeTimeout)
			return
		}
	}
}

func TestInject_MoveClickKeyScroll(t *testing.T) {
	lib, err := fw.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	perms, err := tap.NewPermissions(lib)
	if err != nil {
		t.Fatalf("NewPermissions: %v", err)
	}
	state := perms.Preflight()
	t.Logf("TCC state: %s", state)
	target := mainDisplayCentre(t, lib)

	for _, variant := range []struct {
		name     string
		location Location
	}{
		{"hid", LocationHID},
		{"session", LocationSession},
	} {
		t.Run(variant.name, func(t *testing.T) {
			inj, err := New(lib, Config{Location: variant.location, UserData: testUserData})
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			defer inj.Close()

			exerciseBuilders(t, inj, target)
			exerciseRejections(t, inj, target)
			if !state.AccessibilityTrusted {
				t.Logf("posting not observed: AXIsProcessTrusted=false, posted events are expected to be dropped")
				return
			}
			observePosted(t, lib, inj, target)
		})
	}
}

func observePosted(t *testing.T, lib *fw.Lib, inj *Injector, target fw.CGPoint) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), startTimeout)
	defer cancel()
	listener, err := tap.Start(ctx, lib, tap.Config{Location: tap.LocationSession, ListenOnly: true, Buffer: 256})
	if errors.Is(err, tap.ErrTapCreate) {
		t.Logf("listen-only session tap not created: %v; posted events are checked through the cursor only", err)
	} else if err != nil {
		t.Fatalf("tap.Start: %v", err)
	}
	var events <-chan tap.Event
	if listener != nil {
		defer listener.Close()
		events = listener.Events()
	}

	if err := inj.Move(target, 3, -2); err != nil {
		t.Fatalf("Move: %v", err)
	}
	if got, ok := waitForCursor(lib, target); !ok {
		t.Errorf("cursor read back at (%g,%g), want (%g,%g) within %g point", got.X, got.Y, target.X, target.Y, cursorTolerance)
	}
	if events != nil {
		expectTagged(t, events, tap.EventMouseMoved, func(e tap.Event) bool {
			return e.DeltaX == 3 && e.DeltaY == -2
		})
	}

	if err := inj.Button(target, ButtonLeft, true, 1); err != nil {
		t.Fatalf("Button down: %v", err)
	}
	if err := inj.Button(target, ButtonLeft, false, 1); err != nil {
		t.Fatalf("Button up: %v", err)
	}
	if events != nil {
		expectTagged(t, events, tap.EventLeftMouseDown, nil)
		expectTagged(t, events, tap.EventLeftMouseUp, nil)
	}

	if err := inj.Key(KeyA, true, FlagShift); err != nil {
		t.Fatalf("Key down: %v", err)
	}
	if err := inj.Key(KeyA, false, FlagShift); err != nil {
		t.Fatalf("Key up: %v", err)
	}
	if events != nil {
		expectTagged(t, events, tap.EventKeyDown, func(e tap.Event) bool { return e.Keycode == int64(KeyA) })
		expectTagged(t, events, tap.EventKeyUp, func(e tap.Event) bool { return e.Keycode == int64(KeyA) })
	}

	if err := inj.Scroll(ScrollUnitPixel, 0, -12); err != nil {
		t.Fatalf("Scroll: %v", err)
	}
	if events != nil {
		expectTagged(t, events, tap.EventScrollWheel, nil)
	}
}

func TestInject_ClosedInjectorRejectsPosting(t *testing.T) {
	lib, err := fw.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	inj, err := New(lib, Config{Location: LocationHID, UserData: testUserData})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	inj.Close()
	inj.Close()
	if err := inj.Move(mainDisplayCentre(t, lib), 0, 0); !errors.Is(err, ErrClosed) {
		t.Errorf("Move after Close error = %v, want %v", err, ErrClosed)
	}
}

func TestInject_NewRejectsInvalidLocation(t *testing.T) {
	lib, err := fw.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if _, err := New(lib, Config{Location: Location(9), UserData: testUserData}); !errors.Is(err, ErrInvalidLocation) {
		t.Errorf("New with location 9 error = %v, want %v", err, ErrInvalidLocation)
	}
}
