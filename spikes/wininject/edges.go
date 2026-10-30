package main

import (
	"cmp"
	"errors"
	"fmt"
	"slices"
)

var (
	ErrEmptyRect      = errors.New("wininject: empty rectangle")
	ErrOverlap        = errors.New("wininject: monitors overlap")
	ErrDegenerate     = errors.New("wininject: rectangle narrower or shorter than 2 px")
	ErrOutsideDesktop = errors.New("wininject: point outside virtual desktop")
)

const normalizedMax = 65535

type Point struct{ X, Y int32 }

type Rect struct{ X, Y, W, H int32 }

func (r Rect) Right() int32  { return r.X + r.W }
func (r Rect) Bottom() int32 { return r.Y + r.H }
func (r Rect) Empty() bool   { return r.W <= 0 || r.H <= 0 }

type Side uint8

const (
	SideLeft Side = iota
	SideRight
	SideTop
	SideBottom
)

func (s Side) String() string {
	switch s {
	case SideLeft:
		return "left"
	case SideRight:
		return "right"
	case SideTop:
		return "top"
	case SideBottom:
		return "bottom"
	}
	return "unknown"
}

// Segment.At follows D-88: the Right and Bottom lines sit at Right() and Bottom(), the last pixel inside is At-1.
type Segment struct {
	Monitor  int
	Side     Side
	From, To int32
	At       int32
}

type interval struct{ from, to int32 }

func RealEdges(rects []Rect) ([]Segment, error) {
	for i, r := range rects {
		if r.Empty() {
			return nil, fmt.Errorf("monitor %d: %w", i, ErrEmptyRect)
		}
	}
	for i := range rects {
		for j := i + 1; j < len(rects); j++ {
			if overlaps(rects[i], rects[j]) {
				return nil, fmt.Errorf("monitors %d and %d: %w", i, j, ErrOverlap)
			}
		}
	}

	var out []Segment
	for i, m := range rects {
		for _, side := range [...]Side{SideLeft, SideRight, SideTop, SideBottom} {
			lo, hi, line := sideSpan(m, side)
			var covered []interval
			for j, n := range rects {
				if j == i {
					continue
				}
				if c, ok := coverage(n, side, line, lo, hi); ok {
					covered = append(covered, c)
				}
			}
			for _, g := range uncovered(lo, hi, covered) {
				out = append(out, Segment{Monitor: i, Side: side, From: g.from, To: g.to, At: line})
			}
		}
	}
	return out, nil
}

func overlaps(a, b Rect) bool {
	return a.X < b.Right() && b.X < a.Right() && a.Y < b.Bottom() && b.Y < a.Bottom()
}

func sideSpan(m Rect, side Side) (lo, hi, line int32) {
	switch side {
	case SideLeft:
		return m.Y, m.Bottom(), m.X
	case SideRight:
		return m.Y, m.Bottom(), m.Right()
	case SideTop:
		return m.X, m.Right(), m.Y
	default:
		return m.X, m.Right(), m.Bottom()
	}
}

func coverage(n Rect, side Side, line, lo, hi int32) (interval, bool) {
	var facing bool
	var nlo, nhi int32
	switch side {
	case SideLeft:
		facing = n.Right() == line
		nlo, nhi = n.Y, n.Bottom()
	case SideRight:
		facing = n.X == line
		nlo, nhi = n.Y, n.Bottom()
	case SideTop:
		facing = n.Bottom() == line
		nlo, nhi = n.X, n.Right()
	default:
		facing = n.Y == line
		nlo, nhi = n.X, n.Right()
	}
	if !facing {
		return interval{}, false
	}
	c := interval{from: max(nlo, lo), to: min(nhi, hi)}
	return c, c.from < c.to
}

func uncovered(lo, hi int32, covered []interval) []interval {
	slices.SortFunc(covered, func(a, b interval) int { return cmp.Compare(a.from, b.from) })
	var gaps []interval
	cur := lo
	for _, c := range covered {
		if c.from > cur {
			gaps = append(gaps, interval{from: cur, to: c.from})
		}
		cur = max(cur, c.to)
	}
	if cur < hi {
		gaps = append(gaps, interval{from: cur, to: hi})
	}
	return gaps
}

func Normalize(desk Rect, p Point) (Point, error) {
	if desk.W < 2 || desk.H < 2 {
		return Point{}, fmt.Errorf("desktop %v: %w", desk, ErrDegenerate)
	}
	if p.X < desk.X || p.X >= desk.Right() || p.Y < desk.Y || p.Y >= desk.Bottom() {
		return Point{}, fmt.Errorf("point %v in desktop %v: %w", p, desk, ErrOutsideDesktop)
	}
	nx := int64(p.X-desk.X) * normalizedMax / int64(desk.W-1)
	ny := int64(p.Y-desk.Y) * normalizedMax / int64(desk.H-1)
	return Point{X: int32(nx), Y: int32(ny)}, nil
}

func Probes(r Rect) ([]Point, error) {
	if r.W < 2 || r.H < 2 {
		return nil, fmt.Errorf("monitor %v: %w", r, ErrDegenerate)
	}
	return []Point{
		{r.X + 1, r.Y + 1},
		{r.Right() - 2, r.Y + 1},
		{r.X + 1, r.Bottom() - 2},
		{r.Right() - 2, r.Bottom() - 2},
		{r.X + r.W/2, r.Y + r.H/2},
	}, nil
}
