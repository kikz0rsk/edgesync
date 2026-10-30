package main

import (
	"errors"
	"fmt"
)

var ErrScreenTooSmall = errors.New("virtual screen is too small for the demo square")

type point struct{ x, y int32 }

type screenRect struct{ left, top, width, height int32 }

// squarePath returns the points of a size x size square starting at start, interpolated with stepsPerSide points per
// side. The square grows towards the screen centre side of start so that it never leaves the virtual screen. The last
// point is start itself.
func squarePath(start point, size int32, screen screenRect, stepsPerSide int) ([]point, error) {
	if stepsPerSide < 1 {
		return nil, fmt.Errorf("steps per side must be positive, got %d", stepsPerSide)
	}
	if screen.width <= size || screen.height <= size {
		return nil, fmt.Errorf("%w: %dx%d for a %d px square", ErrScreenTooSmall, screen.width, screen.height, size)
	}
	if start.x < screen.left || start.x >= screen.left+screen.width || start.y < screen.top || start.y >= screen.top+screen.height {
		return nil, fmt.Errorf("start point (%d,%d) is outside the virtual screen", start.x, start.y)
	}
	dx, dy := size, size
	if start.x+size >= screen.left+screen.width {
		dx = -size
	}
	if start.y+size >= screen.top+screen.height {
		dy = -size
	}
	corners := [...]point{
		{start.x + dx, start.y},
		{start.x + dx, start.y + dy},
		{start.x, start.y + dy},
		start,
	}
	path := make([]point, 0, len(corners)*stepsPerSide)
	from := start
	for _, to := range corners {
		for step := 1; step <= stepsPerSide; step++ {
			path = append(path, point{
				x: from.x + (to.x-from.x)*int32(step)/int32(stepsPerSide),
				y: from.y + (to.y-from.y)*int32(step)/int32(stepsPerSide),
			})
		}
		from = to
	}
	return path, nil
}

// normalizeAbsolute maps a virtual-screen pixel to the 0..65535 range of MOUSEEVENTF_ABSOLUTE|MOUSEEVENTF_VIRTUALDESK.
func normalizeAbsolute(p point, screen screenRect) (nx, ny int32, err error) {
	if screen.width < 2 || screen.height < 2 {
		return 0, 0, fmt.Errorf("%w: %dx%d", ErrScreenTooSmall, screen.width, screen.height)
	}
	nx = int32(int64(p.x-screen.left) * 65535 / int64(screen.width-1))
	ny = int32(int64(p.y-screen.top) * 65535 / int64(screen.height-1))
	return nx, ny, nil
}
