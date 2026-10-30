package main

import (
	"math"
	"testing"
)

const snapThreshold = 12

func near(a, b float32) bool {
	return math.Abs(float64(a-b)) <= 0.01
}

func single(r Rect) Group {
	return Group{Monitors: []Rect{r}}
}

func requireSegments(t *testing.T, got, want []Segment) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("segments = %v, want %v", got, want)
	}
	for i := range want {
		if !near(got[i].X1, want[i].X1) || !near(got[i].Y1, want[i].Y1) ||
			!near(got[i].X2, want[i].X2) || !near(got[i].Y2, want[i].Y2) {
			t.Fatalf("segment %d = %v, want %v", i, got[i], want[i])
		}
	}
}

func TestSnap_ToNeighbourEdgeWithinThreshold(t *testing.T) {
	other := single(Rect{0, 0, 1920, 1080})
	moving := single(Rect{1930, 100, 1920, 1080})

	dx, dy, guides := Snap(moving, []Group{other}, snapThreshold)

	if !near(dx, -10) || !near(dy, 0) {
		t.Fatalf("snap = (%v, %v), want (-10, 0)", dx, dy)
	}
	requireSegments(t, guides, []Segment{{1920, 100, 1920, 1080}})
}

func TestSnap_NoSnapBeyondThreshold(t *testing.T) {
	other := single(Rect{0, 0, 1920, 1080})
	moving := single(Rect{1933, 100, 1920, 1080})

	dx, dy, guides := Snap(moving, []Group{other}, snapThreshold)

	if dx != 0 || dy != 0 || len(guides) != 0 {
		t.Fatalf("snap = (%v, %v, %v), want no snap", dx, dy, guides)
	}
}

func TestSnap_PrefersClosestEdge(t *testing.T) {
	left := single(Rect{0, 0, 1920, 1080})
	right := single(Rect{2735, 0, 500, 1080})
	moving := single(Rect{1930, 100, 800, 500})

	dx, _, _ := Snap(moving, []Group{left, right}, snapThreshold)

	if !near(dx, 5) {
		t.Fatalf("dx = %v, want 5 (right neighbour is closer than left neighbour at -10)", dx)
	}
}

func TestSnap_OverlapSegment(t *testing.T) {
	a := single(Rect{0, 0, 1920, 1080})
	b := single(Rect{1920, 200, 1920, 1080})

	requireSegments(t, Overlap(a, b), []Segment{{1920, 200, 1920, 1080}})
	requireSegments(t, Overlap(b, a), []Segment{{1920, 200, 1920, 1080}})
}

func TestSnap_SnapsBothAxesAndReportsBothGuides(t *testing.T) {
	other := single(Rect{0, 0, 1920, 1080})
	moving := single(Rect{1925, 1085, 800, 600})

	dx, dy, guides := Snap(moving, []Group{other}, snapThreshold)

	if !near(dx, -5) || !near(dy, -5) {
		t.Fatalf("snap = (%v, %v), want (-5, -5)", dx, dy)
	}
	if len(guides) != 2 {
		t.Fatalf("guides = %v, want one per axis", guides)
	}
}

func TestSnap_IgnoresFacingEdgeWithoutCrossAxisContact(t *testing.T) {
	other := single(Rect{0, 0, 1920, 1080})
	moving := single(Rect{1925, 5000, 800, 600})

	dx, dy, guides := Snap(moving, []Group{other}, snapThreshold)

	if dx != 0 || dy != 0 || len(guides) != 0 {
		t.Fatalf("snap = (%v, %v, %v), want no snap", dx, dy, guides)
	}
}

func TestOverlap_SeamBetweenMonitorsOfOneMachineIsNotAnEdge(t *testing.T) {
	machine := Group{Monitors: []Rect{{0, 0, 1920, 1080}, {1920, 0, 1920, 1080}}}
	probe := single(Rect{1920, 100, 500, 500})

	if got := Overlap(machine, probe); len(got) != 0 {
		t.Fatalf("overlap = %v, want none for an interior seam", got)
	}
}

func TestOverlap_MultiMonitorGroupSharesOnlyRealEdge(t *testing.T) {
	machine := Group{Monitors: []Rect{{0, 0, 1920, 1080}, {1920, 0, 1920, 1080}}}
	below := single(Rect{1000, 1080, 1920, 1080})

	requireSegments(t, Overlap(machine, below), []Segment{{1000, 1080, 2920, 1080}})
}

func TestOverlap_PartialSeamExposesOnlyUncoveredPart(t *testing.T) {
	group := Group{Monitors: []Rect{{0, 0, 2560, 1440}, {2560, 360, 1920, 1080}}}
	probe := single(Rect{2560, -400, 800, 800})

	requireSegments(t, Overlap(group, probe), []Segment{{2560, 0, 2560, 360}})
}

func TestView_ZoomAroundKeepsCursorPointFixed(t *testing.T) {
	view := View{Zoom: 0.5, PanX: 40, PanY: -20}
	cx, cy := float32(300), float32(200)
	beforeX, beforeY := view.ToCanvas(cx, cy)

	zoomed := view.ZoomAround(1.8, cx, cy)

	afterX, afterY := zoomed.ToCanvas(cx, cy)
	if !near(beforeX, afterX) || !near(beforeY, afterY) || !near(zoomed.Zoom, 0.9) {
		t.Fatalf("canvas point moved from (%v, %v) to (%v, %v), zoom %v", beforeX, beforeY, afterX, afterY, zoomed.Zoom)
	}
}

func TestView_ZoomIsClamped(t *testing.T) {
	view := View{Zoom: 1}

	if got := view.ZoomAround(1e6, 0, 0).Zoom; got != MaxZoom {
		t.Fatalf("zoom = %v, want %v", got, float32(MaxZoom))
	}
	if got := view.ZoomAround(1e-6, 0, 0).Zoom; got != MinZoom {
		t.Fatalf("zoom = %v, want %v", got, float32(MinZoom))
	}
}

func TestSyntheticMachines_HaveTwoOrThreeMonitorsAndNoInitialSnap(t *testing.T) {
	machines := SyntheticMachines(64)
	if len(machines) != 64 {
		t.Fatalf("machines = %d, want 64", len(machines))
	}
	groups := make([]Group, len(machines))
	for i, m := range machines {
		if n := len(m.Monitors); n < 2 || n > 3 {
			t.Fatalf("machine %s has %d monitors, want 2 or 3", m.Name, n)
		}
		groups[i] = m.Group()
	}
	for i, g := range groups {
		others := append(append([]Group{}, groups[:i]...), groups[i+1:]...)
		if dx, dy, _ := Snap(g, others, snapThreshold); dx != 0 || dy != 0 {
			t.Fatalf("machine %s snaps at start: (%v, %v)", machines[i].Name, dx, dy)
		}
	}
}

func TestFitView_CentresBoundsInsideViewport(t *testing.T) {
	bounds := Rect{X: 100, Y: 50, W: 4000, H: 2000}

	view := FitView(bounds, 1000, 800, 20)

	left, top := view.ToScreen(bounds.X, bounds.Y)
	right, bottom := view.ToScreen(bounds.Right(), bounds.Bottom())
	if left < 20-0.01 || top < 20-0.01 || right > 980+0.01 || bottom > 780+0.01 {
		t.Fatalf("fitted bounds (%v,%v)-(%v,%v) leave the margin", left, top, right, bottom)
	}
	if !near(left+right, 1000) || !near(top+bottom, 800) {
		t.Fatalf("fitted bounds are not centred: (%v,%v)-(%v,%v)", left, top, right, bottom)
	}
}
