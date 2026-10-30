//go:build integration && linux

package dnd

import (
	"context"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/jezek/xgb/xproto"
)

const scriptedPause = 150 * time.Millisecond

type targetRun struct {
	reports chan TargetReport
	done    chan error
}

func openDisplay(t *testing.T) *Display {
	t.Helper()
	d, err := Open("")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(d.Close)
	return d
}

func testContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func runTarget(ctx context.Context, target *Target) targetRun {
	run := targetRun{reports: make(chan TargetReport, 16), done: make(chan error, 1)}
	runCtx, cancel := context.WithCancel(ctx)
	go func() {
		run.done <- target.Run(runCtx, func(r TargetReport) { run.reports <- r })
		cancel()
	}()
	return run
}

func (r targetRun) waitFor(t *testing.T, event TargetEvent) TargetReport {
	t.Helper()
	timeout := time.After(10 * time.Second)
	for {
		select {
		case report := <-r.reports:
			if report.Err != nil {
				t.Fatalf("report %s carries error: %v", report.Event, report.Err)
			}
			if report.Event == event {
				return report
			}
		case <-timeout:
			t.Fatalf("no %s report", event)
		}
	}
}

// scriptedDrag paces the inputs so the XdndStatus of one position reaches the source before the next input: a button
// release before any status makes the source send XdndLeave without waiting.
func scriptedDrag(points ...Pointer) <-chan Pointer {
	feed := make(chan Pointer, len(points))
	go func() {
		for _, p := range points {
			time.Sleep(scriptedPause)
			feed <- p
		}
	}()
	return feed
}

func TestXdnd_SourceDropsOnTargetOnTheSameServer(t *testing.T) {
	ctx := testContext(t)
	target, err := openDisplay(t).NewTarget(TargetOptions{X: 300, Y: 300, Width: 200, Height: 200, Title: "edgesync dnd target"})
	if err != nil {
		t.Fatal(err)
	}
	run := runTarget(ctx, target)
	dropped := filepath.Join(t.TempDir(), "dropped file.txt")
	source, err := openDisplay(t).NewSource([]string{dropped}, SourceOptions{Width: 100, Height: 100, Title: "edgesync dnd source"})
	if err != nil {
		t.Fatal(err)
	}

	result, err := source.Drag(ctx, DragOptions{Feed: scriptedDrag(
		Pointer{X: 50, Y: 50},
		Pointer{X: 350, Y: 350},
		Pointer{X: 360, Y: 360},
		Pointer{Release: true},
	)})
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome != OutcomeDropped || result.Target != uint32(target.Window()) {
		t.Fatalf("result %+v", result)
	}
	report := run.waitFor(t, TargetEventDropped)
	if !slices.Equal(report.Dropped.Paths, []string{dropped}) {
		t.Fatalf("paths %q, want %q", report.Dropped.Paths, dropped)
	}
}

func TestXdnd_ReleaseOutsideAnyTargetEndsNoTarget(t *testing.T) {
	ctx := testContext(t)
	source, err := openDisplay(t).NewSource([]string{"/tmp/edgesync-dnd-none"}, SourceOptions{Width: 100, Height: 100, Title: "edgesync dnd source"})
	if err != nil {
		t.Fatal(err)
	}
	result, err := source.Drag(ctx, DragOptions{Feed: scriptedDrag(Pointer{X: 1000, Y: 700}, Pointer{Release: true})})
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome != OutcomeNoTarget {
		t.Fatalf("outcome %s", result.Outcome)
	}
}

func TestXdnd_LeaveReachesTheTarget(t *testing.T) {
	ctx := testContext(t)
	target, err := openDisplay(t).NewTarget(TargetOptions{X: 300, Y: 300, Width: 200, Height: 200, Title: "edgesync dnd target"})
	if err != nil {
		t.Fatal(err)
	}
	run := runTarget(ctx, target)
	source, err := openDisplay(t).NewSource([]string{"/tmp/edgesync-dnd-leave"}, SourceOptions{Width: 100, Height: 100, Title: "edgesync dnd source"})
	if err != nil {
		t.Fatal(err)
	}
	result, err := source.Drag(ctx, DragOptions{Feed: scriptedDrag(Pointer{X: 350, Y: 350}, Pointer{Cancel: true})})
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome != OutcomeCancelled {
		t.Fatalf("outcome %s", result.Outcome)
	}
	run.waitFor(t, TargetEventEntered)
	run.waitFor(t, TargetEventLeft)
}

func TestXdnd_ProxyTargetAcceptsADropOnAForeignWindow(t *testing.T) {
	ctx := testContext(t)
	foreign, err := openDisplay(t).mapVisible(300, 300, 200, 200, "edgesync foreign window", 0)
	if err != nil {
		t.Fatal(err)
	}
	proxyDisplay := openDisplay(t)
	target, err := proxyDisplay.NewProxyTarget(foreign)
	if err != nil {
		t.Fatal(err)
	}
	run := runTarget(ctx, target)
	dropped := filepath.Join(t.TempDir(), "via proxy.txt")
	sourceDisplay := openDisplay(t)
	source, err := sourceDisplay.NewSource([]string{dropped}, SourceOptions{Width: 100, Height: 100, Title: "edgesync dnd source"})
	if err != nil {
		t.Fatal(err)
	}

	found, err := sourceDisplay.targetAt(350, 350, source.Window())
	if err != nil {
		t.Fatal(err)
	}
	if found.ID != uint32(foreign) || found.Proxy == 0 || found.Version != ProtocolVersion {
		t.Fatalf("targetAt found %+v for the foreign window %d", found, foreign)
	}
	result, err := source.Drag(ctx, DragOptions{Feed: scriptedDrag(Pointer{X: 350, Y: 350}, Pointer{Release: true})})
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome != OutcomeDropped {
		t.Fatalf("outcome %s", result.Outcome)
	}
	report := run.waitFor(t, TargetEventDropped)
	if !slices.Equal(report.Dropped.Paths, []string{dropped}) {
		t.Fatalf("paths %q", report.Dropped.Paths)
	}

	if err := target.Close(); err != nil {
		t.Fatal(err)
	}
	versions, err := sourceDisplay.words(foreign, sourceDisplay.atoms.Aware, xproto.AtomAtom, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != 0 {
		t.Fatalf("XdndAware left on the foreign window after Close: %v", versions)
	}
}

func TestXdnd_StaleProxyPropertyIsIgnored(t *testing.T) {
	d := openDisplay(t)
	window, err := d.mapVisible(300, 300, 200, 200, "edgesync stale proxy", 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := d.setWords(window, d.atoms.Aware, xproto.AtomAtom, []uint32{ProtocolVersion}); err != nil {
		t.Fatal(err)
	}
	if err := d.setWords(window, d.atoms.Proxy, xproto.AtomWindow, []uint32{0x1}); err != nil {
		t.Fatal(err)
	}
	found, err := d.targetAt(350, 350, 0)
	if err != nil {
		t.Fatal(err)
	}
	if found.ID != uint32(window) || found.Proxy != 0 {
		t.Fatalf("found %+v, want the window itself without a proxy", found)
	}
}
