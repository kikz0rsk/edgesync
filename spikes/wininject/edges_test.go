package main

import (
	"errors"
	"slices"
	"testing"
)

func assertSegments(t *testing.T, rects []Rect, want []Segment) {
	t.Helper()
	got, err := RealEdges(rects)
	if err != nil {
		t.Fatalf("RealEdges: unexpected error: %v", err)
	}
	if !slices.Equal(got, want) {
		t.Errorf("RealEdges:\n got  %v\n want %v", got, want)
	}
}

func TestRealEdges_SideBySide(t *testing.T) {
	assertSegments(t, []Rect{
		{0, 0, 1920, 1080},
		{1920, 0, 1920, 1080},
	}, []Segment{
		{0, SideLeft, 0, 1080, 0},
		{0, SideTop, 0, 1920, 0},
		{0, SideBottom, 0, 1920, 1080},
		{1, SideRight, 0, 1080, 3840},
		{1, SideTop, 1920, 3840, 0},
		{1, SideBottom, 1920, 3840, 1080},
	})
}

func TestRealEdges_LShape(t *testing.T) {
	assertSegments(t, []Rect{
		{0, 0, 1920, 1080},
		{1920, 0, 1920, 1080},
		{0, 1080, 1920, 1080},
	}, []Segment{
		{0, SideLeft, 0, 1080, 0},
		{0, SideTop, 0, 1920, 0},
		{1, SideRight, 0, 1080, 3840},
		{1, SideTop, 1920, 3840, 0},
		{1, SideBottom, 1920, 3840, 1080},
		{2, SideLeft, 1080, 2160, 0},
		{2, SideRight, 1080, 2160, 1920},
		{2, SideBottom, 0, 1920, 2160},
	})
}

func TestRealEdges_StackedWithOffset(t *testing.T) {
	assertSegments(t, []Rect{
		{0, 0, 1920, 1080},
		{200, 1080, 2560, 1440},
	}, []Segment{
		{0, SideLeft, 0, 1080, 0},
		{0, SideRight, 0, 1080, 1920},
		{0, SideTop, 0, 1920, 0},
		{0, SideBottom, 0, 200, 1080},
		{1, SideLeft, 1080, 2520, 200},
		{1, SideRight, 1080, 2520, 2760},
		{1, SideTop, 1920, 2760, 1080},
		{1, SideBottom, 200, 2760, 2520},
	})
}

func TestRealEdges_GapBetweenMonitors(t *testing.T) {
	assertSegments(t, []Rect{
		{0, 0, 1920, 1080},
		{1930, 0, 1920, 1080},
	}, []Segment{
		{0, SideLeft, 0, 1080, 0},
		{0, SideRight, 0, 1080, 1920},
		{0, SideTop, 0, 1920, 0},
		{0, SideBottom, 0, 1920, 1080},
		{1, SideLeft, 0, 1080, 1930},
		{1, SideRight, 0, 1080, 3850},
		{1, SideTop, 1930, 3850, 0},
		{1, SideBottom, 1930, 3850, 1080},
	})
}

func TestRealEdges_MixedSizes(t *testing.T) {
	assertSegments(t, []Rect{
		{0, 0, 3840, 2160},
		{3840, 540, 1920, 1080},
	}, []Segment{
		{0, SideLeft, 0, 2160, 0},
		{0, SideRight, 0, 540, 3840},
		{0, SideRight, 1620, 2160, 3840},
		{0, SideTop, 0, 3840, 0},
		{0, SideBottom, 0, 3840, 2160},
		{1, SideRight, 540, 1620, 5760},
		{1, SideTop, 3840, 5760, 540},
		{1, SideBottom, 3840, 5760, 1620},
	})
}

func TestRealEdges_RejectsEmptyAndOverlap(t *testing.T) {
	if _, err := RealEdges([]Rect{{0, 0, 0, 10}}); !errors.Is(err, ErrEmptyRect) {
		t.Errorf("empty rect: got %v, want ErrEmptyRect", err)
	}
	if _, err := RealEdges([]Rect{{0, 0, 10, 10}, {5, 5, 10, 10}}); !errors.Is(err, ErrOverlap) {
		t.Errorf("overlap: got %v, want ErrOverlap", err)
	}
}

func TestNormalize_IdentityDesktopMapsPixelsExactly(t *testing.T) {
	desk := Rect{100, 50, 65536, 65536}
	cases := []struct {
		name string
		p    Point
		want Point
	}{
		{"top-left", Point{100, 50}, Point{0, 0}},
		{"bottom-right", Point{100 + 65535, 50 + 65535}, Point{65535, 65535}},
		{"interior", Point{100 + 30000, 50 + 1}, Point{30000, 1}},
	}
	for _, c := range cases {
		got, err := Normalize(desk, c.p)
		if err != nil {
			t.Fatalf("%s: unexpected error: %v", c.name, err)
		}
		if got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}

func TestNormalize_TruncatesAndSpansRange(t *testing.T) {
	desk := Rect{0, 0, 3, 3}
	cases := []struct {
		p    Point
		want Point
	}{
		{Point{0, 0}, Point{0, 0}},
		{Point{1, 1}, Point{32767, 32767}},
		{Point{2, 2}, Point{65535, 65535}},
	}
	for _, c := range cases {
		got, err := Normalize(desk, c.p)
		if err != nil {
			t.Fatalf("point %v: unexpected error: %v", c.p, err)
		}
		if got != c.want {
			t.Errorf("point %v: got %v, want %v", c.p, got, c.want)
		}
	}
}

func TestNormalize_RejectsOutsideAndDegenerate(t *testing.T) {
	if _, err := Normalize(Rect{0, 0, 3, 3}, Point{3, 0}); !errors.Is(err, ErrOutsideDesktop) {
		t.Errorf("outside point: got %v, want ErrOutsideDesktop", err)
	}
	if _, err := Normalize(Rect{0, 0, 1, 3}, Point{0, 0}); !errors.Is(err, ErrDegenerate) {
		t.Errorf("degenerate desktop: got %v, want ErrDegenerate", err)
	}
}

func TestProbes_InsetCornersAndCentre(t *testing.T) {
	got, err := Probes(Rect{1920, 0, 1920, 1080})
	if err != nil {
		t.Fatalf("Probes: unexpected error: %v", err)
	}
	want := []Point{{1921, 1}, {3838, 1}, {1921, 1078}, {3838, 1078}, {2880, 540}}
	if !slices.Equal(got, want) {
		t.Errorf("Probes: got %v, want %v", got, want)
	}
	if _, err := Probes(Rect{0, 0, 1, 10}); !errors.Is(err, ErrDegenerate) {
		t.Errorf("degenerate monitor: got %v, want ErrDegenerate", err)
	}
}
