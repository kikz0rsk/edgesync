//go:build integration && linux

package x11

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
)

func openTest(t *testing.T) *Conn {
	t.Helper()
	c, err := Open("")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(c.Close)
	return c
}

func testContext(t *testing.T, d time.Duration) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), d)
	t.Cleanup(cancel)
	return ctx
}

func TestXTest_MotionObservedByOwnWindow(t *testing.T) {
	c := openTest(t)
	ctx := testContext(t, 10*time.Second)
	window, err := c.newWindow(xproto.WindowClassInputOutput, c.width, c.height, xproto.CwEventMask, []uint32{xproto.EventMaskPointerMotion})
	if err != nil {
		t.Fatalf("newWindow: %v", err)
	}
	if err := xproto.MapWindowChecked(c.x, window).Check(); err != nil {
		t.Fatalf("map window: %v", err)
	}
	inj := c.Injector()
	for _, p := range []struct{ x, y int16 }{{100, 120}, {300, 200}} {
		if err := inj.MoveAbsolute(p.x, p.y); err != nil {
			t.Fatalf("MoveAbsolute(%d,%d): %v", p.x, p.y, err)
		}
		_, err := c.WaitEvent(ctx, func(ev xgb.Event) bool {
			m, ok := ev.(xproto.MotionNotifyEvent)
			return ok && m.Event == window && m.EventX == p.x && m.EventY == p.y
		})
		if err != nil {
			t.Fatalf("no MotionNotify at (%d,%d) on own window: %v", p.x, p.y, err)
		}
	}
}

func TestGrab_WarpToCentreDeltas(t *testing.T) {
	grabConn := openTest(t)
	inputConn := openTest(t)
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	t.Cleanup(cancel)

	capture, err := grabConn.StartCapture()
	if err != nil {
		t.Fatalf("StartCapture: %v", err)
	}
	deltas := make(chan Delta, 16)
	pumped := make(chan error, 1)
	go func() {
		pumped <- capture.Deltas(ctx, func(d Delta) { deltas <- d })
	}()

	inj := inputConn.Injector()
	for _, step := range []Delta{{DX: 40, DY: 0}, {DX: -15, DY: 22}, {DX: 3, DY: -9}} {
		if err := inj.MoveRelative(int16(step.DX), int16(step.DY)); err != nil {
			t.Fatalf("MoveRelative %+v: %v", step, err)
		}
		select {
		case got := <-deltas:
			if got != step {
				t.Fatalf("delta = %+v, want %+v", got, step)
			}
		case <-ctx.Done():
			t.Fatalf("no delta for step %+v: %v", step, ctx.Err())
		}
	}
	cancel()
	if err := <-pumped; !errors.Is(err, context.Canceled) {
		t.Fatalf("Deltas returned %v, want context.Canceled", err)
	}
}

func TestXFixes_SelectionNotifyOnForeignOwner(t *testing.T) {
	observer := openTest(t)
	owner := openTest(t)
	ctx := testContext(t, 10*time.Second)

	watch, err := observer.WatchSelection("CLIPBOARD")
	if err != nil {
		t.Fatalf("WatchSelection: %v", err)
	}
	claimed, err := owner.Own("CLIPBOARD", nil)
	if err != nil {
		t.Fatalf("Own: %v", err)
	}
	got, err := watch.NextOwnerChange(ctx)
	if err != nil {
		t.Fatalf("NextOwnerChange: %v", err)
	}
	if got != claimed.window {
		t.Fatalf("new owner = %d, want %d", got, claimed.window)
	}
}

func TestSelection_INCRTransfer5MiB(t *testing.T) {
	const size = 5 << 20
	payload := pattern(size)
	ownerConn := openTest(t)
	readerConn := openTest(t)
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	t.Cleanup(cancel)

	owner, err := ownerConn.Own("CLIPBOARD", []Payload{{Target: "UTF8_STRING", Data: payload}})
	if err != nil {
		t.Fatalf("Own: %v", err)
	}
	served := make(chan error, 1)
	go func() {
		served <- owner.Serve(ctx)
	}()

	transfer, err := readerConn.ReadSelection(ctx, "CLIPBOARD", "UTF8_STRING")
	if err != nil {
		t.Fatalf("ReadSelection: %v", err)
	}
	if !transfer.Incremental {
		t.Fatalf("owner did not use INCR; server maximum request is %d bytes", readerConn.maxRequestBytes)
	}
	if len(transfer.Data) != len(payload) {
		t.Fatalf("got %d bytes, want %d", len(transfer.Data), len(payload))
	}
	if !bytes.Equal(transfer.Data, payload) {
		t.Fatalf("payload differs at byte %d", firstDifference(transfer.Data, payload))
	}
	cancel()
	if err := <-served; !errors.Is(err, context.Canceled) {
		t.Fatalf("Serve returned %v, want context.Canceled", err)
	}
}

func TestRandR_MonitorsListed(t *testing.T) {
	c := openTest(t)
	monitors, err := c.Monitors()
	if err != nil {
		t.Fatalf("Monitors: %v", err)
	}
	if len(monitors) < 2 {
		t.Fatalf("got %d monitors, want at least 2; define them with xrandr --setmonitor (see README.md)", len(monitors))
	}
	for _, m := range monitors {
		if m.Name == "" || m.Width == 0 || m.Height == 0 {
			t.Errorf("monitor %+v has no name or geometry", m)
		}
	}
	if err := c.WatchMonitorChanges(); err != nil {
		t.Fatalf("WatchMonitorChanges: %v", err)
	}
}

func pattern(n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = byte(i*131 + i>>8)
	}
	return b
}

func firstDifference(a, b []byte) int {
	for i := range min(len(a), len(b)) {
		if a[i] != b[i] {
			return i
		}
	}
	return min(len(a), len(b))
}
