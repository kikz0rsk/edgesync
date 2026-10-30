//go:build integration && darwin

package fw

import (
	"math"
	"testing"

	"github.com/ebitengine/purego/objc"
)

func TestDisplays_ListBoundsAndScale(t *testing.T) {
	lib, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	displays, err := lib.CoreGraphics.ActiveDisplays()
	if err != nil {
		t.Fatalf("ActiveDisplays: %v", err)
	}
	if len(displays) == 0 {
		t.Fatal("no active displays")
	}

	var mainDisplay *Display
	for i := range displays {
		d := &displays[i]
		t.Logf("display=%d main=%t origin=(%g,%g) size=%gx%g points pixels=%dx%d scale=%g",
			d.ID, d.Main, d.Bounds.Origin.X, d.Bounds.Origin.Y,
			d.Bounds.Size.Width, d.Bounds.Size.Height, d.PixelsWide, d.PixelsHigh, d.Scale)
		if d.Scale < 1 {
			t.Errorf("display %d: scale %g is below 1", d.ID, d.Scale)
		}
		if d.Main {
			mainDisplay = d
		}
	}
	if mainDisplay == nil {
		t.Fatal("main display is not among the active displays")
	}

	want := CGPoint{
		X: mainDisplay.Bounds.Origin.X + mainDisplay.Bounds.Size.Width/2,
		Y: mainDisplay.Bounds.Origin.Y + mainDisplay.Bounds.Size.Height/2,
	}
	if err := lib.CoreGraphics.WarpCursor(want); err != nil {
		t.Fatalf("WarpCursor: %v", err)
	}
	got, err := lib.CoreGraphics.CursorLocation(lib.CoreFoundation)
	if err != nil {
		t.Fatalf("CursorLocation: %v", err)
	}
	if math.Abs(got.X-want.X) > 1 || math.Abs(got.Y-want.Y) > 1 {
		t.Errorf("cursor read back at (%g,%g), want (%g,%g) within 1 point", got.X, got.Y, want.X, want.Y)
	}
}

func TestNSString_RoundTrip(t *testing.T) {
	if _, err := Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	poolClass, err := Class("NSAutoreleasePool")
	if err != nil {
		t.Fatalf("Class: %v", err)
	}
	pool := objc.ID(poolClass).Send(Sel("new"))
	defer pool.Send(Sel("drain"))

	const want = "EdgeSync café"
	s, err := NSString(want)
	if err != nil {
		t.Fatalf("NSString: %v", err)
	}
	got, err := GoString(s)
	if err != nil {
		t.Fatalf("GoString: %v", err)
	}
	if got != want {
		t.Errorf("round trip = %q, want %q", got, want)
	}
}
