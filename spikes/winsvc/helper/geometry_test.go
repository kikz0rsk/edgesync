package main

import (
	"errors"
	"testing"
)

func TestSquarePath_GrowsRightAndDown(t *testing.T) {
	screen := screenRect{left: 0, top: 0, width: 1920, height: 1080}
	path, err := squarePath(point{100, 100}, 200, screen, 4)
	if err != nil {
		t.Fatalf("squarePath: %v", err)
	}
	if len(path) != 16 {
		t.Fatalf("got %d points, want 16", len(path))
	}
	wantCorners := map[int]point{3: {300, 100}, 7: {300, 300}, 11: {100, 300}, 15: {100, 100}}
	for index, want := range wantCorners {
		if path[index] != want {
			t.Errorf("point %d = %v, want %v", index, path[index], want)
		}
	}
}

func TestSquarePath_FlipsNearFarEdges(t *testing.T) {
	screen := screenRect{left: -1920, top: 0, width: 3840, height: 1080}
	path, err := squarePath(point{1900, 1000}, 200, screen, 1)
	if err != nil {
		t.Fatalf("squarePath: %v", err)
	}
	want := []point{{1700, 1000}, {1700, 800}, {1900, 800}, {1900, 1000}}
	if len(path) != len(want) {
		t.Fatalf("got %v", path)
	}
	for i := range want {
		if path[i] != want[i] {
			t.Errorf("point %d = %v, want %v", i, path[i], want[i])
		}
	}
}

func TestSquarePath_StaysInsideScreen(t *testing.T) {
	screen := screenRect{left: -1920, top: -200, width: 3840, height: 1280}
	for _, start := range []point{{-1920, -200}, {1919, 1079}, {0, 0}, {1919, -200}, {-1920, 1079}} {
		path, err := squarePath(start, 200, screen, 10)
		if err != nil {
			t.Fatalf("start %v: %v", start, err)
		}
		for _, p := range path {
			if p.x < screen.left || p.x >= screen.left+screen.width || p.y < screen.top || p.y >= screen.top+screen.height {
				t.Fatalf("start %v: point %v outside the screen", start, p)
			}
		}
		if path[len(path)-1] != start {
			t.Fatalf("start %v: path ends at %v", start, path[len(path)-1])
		}
	}
}

func TestSquarePath_Errors(t *testing.T) {
	small := screenRect{width: 150, height: 1080}
	if _, err := squarePath(point{1, 1}, 200, small, 4); !errors.Is(err, ErrScreenTooSmall) {
		t.Fatalf("small screen: got %v", err)
	}
	screen := screenRect{width: 1920, height: 1080}
	if _, err := squarePath(point{1, 1}, 200, screen, 0); err == nil {
		t.Fatal("zero steps accepted")
	}
	if _, err := squarePath(point{5000, 1}, 200, screen, 4); err == nil {
		t.Fatal("start outside the screen accepted")
	}
}

func TestNormalizeAbsolute_Extremes(t *testing.T) {
	screen := screenRect{left: -1920, top: 0, width: 3840, height: 1080}
	nx, ny, err := normalizeAbsolute(point{-1920, 0}, screen)
	if err != nil || nx != 0 || ny != 0 {
		t.Fatalf("top-left: %d %d %v", nx, ny, err)
	}
	nx, ny, err = normalizeAbsolute(point{1919, 1079}, screen)
	if err != nil || nx != 65535 || ny != 65535 {
		t.Fatalf("bottom-right: %d %d %v", nx, ny, err)
	}
	nx, _, err = normalizeAbsolute(point{0, 0}, screen)
	if err != nil || nx < 32760 || nx > 32780 {
		t.Fatalf("centre: %d %v", nx, err)
	}
}

func TestNormalizeAbsolute_RejectsDegenerateScreen(t *testing.T) {
	if _, _, err := normalizeAbsolute(point{0, 0}, screenRect{width: 1, height: 1080}); !errors.Is(err, ErrScreenTooSmall) {
		t.Fatalf("got %v", err)
	}
}
