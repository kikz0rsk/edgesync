// Package main is the Gio layout-editor proof for EdgeSync (P0-SP5-001).
//
// This file holds the pure model: machines as rigid groups of monitor
// rectangles in canvas logical units (D-51), edge snapping, overlap of facing
// real edges and the zoom/pan view transform. It imports nothing from Gio so
// that it is unit-testable on any host.
package main

import (
	"cmp"
	"fmt"
	"math"
	"slices"
)

const (
	geometryEpsilon = 0.05
	MinZoom         = 0.01
	MaxZoom         = 8
)

type Rect struct {
	X, Y, W, H float32
}

func (r Rect) Right() float32  { return r.X + r.W }
func (r Rect) Bottom() float32 { return r.Y + r.H }

func (r Rect) Translate(dx, dy float32) Rect {
	return Rect{X: r.X + dx, Y: r.Y + dy, W: r.W, H: r.H}
}

func (r Rect) Union(o Rect) Rect {
	x := min(r.X, o.X)
	y := min(r.Y, o.Y)
	return Rect{X: x, Y: y, W: max(r.Right(), o.Right()) - x, H: max(r.Bottom(), o.Bottom()) - y}
}

type Group struct {
	Monitors []Rect
}

func (g Group) Translate(dx, dy float32) Group {
	moved := make([]Rect, len(g.Monitors))
	for i, m := range g.Monitors {
		moved[i] = m.Translate(dx, dy)
	}
	return Group{Monitors: moved}
}

func (g Group) Bounds() Rect {
	if len(g.Monitors) == 0 {
		return Rect{}
	}
	bounds := g.Monitors[0]
	for _, m := range g.Monitors[1:] {
		bounds = bounds.Union(m)
	}
	return bounds
}

type Segment struct {
	X1, Y1, X2, Y2 float32
}

type side uint8

const (
	sideLeft side = iota
	sideRight
	sideTop
	sideBottom
)

func (s side) opposite() side {
	switch s {
	case sideLeft:
		return sideRight
	case sideRight:
		return sideLeft
	case sideTop:
		return sideBottom
	default:
		return sideTop
	}
}

func (s side) vertical() bool {
	return s == sideLeft || s == sideRight
}

type span struct {
	lo, hi float32
}

type edge struct {
	side  side
	coord float32
	span  span
}

func (e edge) segment(s span) Segment {
	if e.side.vertical() {
		return Segment{X1: e.coord, Y1: s.lo, X2: e.coord, Y2: s.hi}
	}
	return Segment{X1: s.lo, Y1: e.coord, X2: s.hi, Y2: e.coord}
}

func sideEdge(m Rect, s side) edge {
	switch s {
	case sideLeft:
		return edge{side: s, coord: m.X, span: span{m.Y, m.Bottom()}}
	case sideRight:
		return edge{side: s, coord: m.Right(), span: span{m.Y, m.Bottom()}}
	case sideTop:
		return edge{side: s, coord: m.Y, span: span{m.X, m.Right()}}
	default:
		return edge{side: s, coord: m.Bottom(), span: span{m.X, m.Right()}}
	}
}

func abs32(v float32) float32 {
	return float32(math.Abs(float64(v)))
}

func overlapSpan(a, b span) (span, bool) {
	lo := max(a.lo, b.lo)
	hi := min(a.hi, b.hi)
	return span{lo, hi}, hi-lo > geometryEpsilon
}

func spanGap(a, b span) float32 {
	return max(a.lo, b.lo) - min(a.hi, b.hi)
}

func connectingSpan(a, b span) span {
	first := max(a.lo, b.lo)
	second := min(a.hi, b.hi)
	return span{min(first, second), max(first, second)}
}

func subtractSpans(base span, cuts []span) []span {
	slices.SortFunc(cuts, func(a, b span) int {
		switch {
		case a.lo < b.lo:
			return -1
		case a.lo > b.lo:
			return 1
		default:
			return 0
		}
	})
	var remaining []span
	cursor := base.lo
	for _, cut := range cuts {
		if cut.lo > cursor+geometryEpsilon {
			remaining = append(remaining, span{cursor, min(cut.lo, base.hi)})
		}
		cursor = max(cursor, cut.hi)
		if cursor >= base.hi {
			return remaining
		}
	}
	if base.hi > cursor+geometryEpsilon {
		remaining = append(remaining, span{cursor, base.hi})
	}
	return remaining
}

func (g Group) realEdges() []edge {
	var edges []edge
	for i, m := range g.Monitors {
		for _, s := range [...]side{sideLeft, sideRight, sideTop, sideBottom} {
			base := sideEdge(m, s)
			var cuts []span
			for j, other := range g.Monitors {
				if j == i {
					continue
				}
				facing := sideEdge(other, s.opposite())
				if abs32(facing.coord-base.coord) <= geometryEpsilon {
					cuts = append(cuts, facing.span)
				}
			}
			for _, free := range subtractSpans(base.span, cuts) {
				edges = append(edges, edge{side: s, coord: base.coord, span: free})
			}
		}
	}
	return mergeCollinear(edges)
}

func mergeCollinear(edges []edge) []edge {
	slices.SortFunc(edges, func(a, b edge) int {
		return cmp.Or(
			cmp.Compare(a.side, b.side),
			cmp.Compare(a.coord, b.coord),
			cmp.Compare(a.span.lo, b.span.lo),
		)
	})
	var merged []edge
	for _, e := range edges {
		last := len(merged) - 1
		if last >= 0 && merged[last].side == e.side &&
			abs32(merged[last].coord-e.coord) <= geometryEpsilon &&
			e.span.lo <= merged[last].span.hi+geometryEpsilon {
			merged[last].span.hi = max(merged[last].span.hi, e.span.hi)
			continue
		}
		merged = append(merged, e)
	}
	return merged
}

func Overlap(a, b Group) []Segment {
	var segments []Segment
	edgesB := b.realEdges()
	for _, ea := range a.realEdges() {
		for _, eb := range edgesB {
			if ea.side.opposite() != eb.side || abs32(ea.coord-eb.coord) > geometryEpsilon {
				continue
			}
			if shared, ok := overlapSpan(ea.span, eb.span); ok {
				segments = append(segments, ea.segment(shared))
			}
		}
	}
	return segments
}

func Snap(moving Group, others []Group, threshold float32) (dx, dy float32, guides []Segment) {
	var otherEdges []edge
	for _, o := range others {
		otherEdges = append(otherEdges, o.realEdges()...)
	}
	dx, xGuides := snapAxis(moving.realEdges(), otherEdges, true, threshold)
	dy, yGuides := snapAxis(moving.Translate(dx, 0).realEdges(), otherEdges, false, threshold)
	return dx, dy, append(xGuides, yGuides...)
}

func snapAxis(mine, theirs []edge, vertical bool, threshold float32) (float32, []Segment) {
	type candidate struct {
		distance float32
		guide    Segment
	}
	var candidates []candidate
	for _, me := range mine {
		if me.side.vertical() != vertical {
			continue
		}
		for _, oe := range theirs {
			if oe.side != me.side.opposite() {
				continue
			}
			distance := oe.coord - me.coord
			if abs32(distance) > threshold || spanGap(me.span, oe.span) > threshold {
				continue
			}
			candidates = append(candidates, candidate{
				distance: distance,
				guide:    oe.segment(connectingSpan(me.span, oe.span)),
			})
		}
	}
	if len(candidates) == 0 {
		return 0, nil
	}
	best := candidates[0].distance
	for _, c := range candidates[1:] {
		if abs32(c.distance) < abs32(best) {
			best = c.distance
		}
	}
	var guides []Segment
	for _, c := range candidates {
		if abs32(c.distance-best) <= geometryEpsilon {
			guides = append(guides, c.guide)
		}
	}
	return best, guides
}

type Machine struct {
	Name     string
	X, Y     float32
	Monitors []Rect
}

func (m Machine) Group() Group {
	return Group{Monitors: m.Monitors}.Translate(m.X, m.Y)
}

func (m Machine) Bounds() Rect {
	return m.Group().Bounds()
}

func BoundsOf(machines []Machine) Rect {
	if len(machines) == 0 {
		return Rect{}
	}
	bounds := machines[0].Bounds()
	for _, m := range machines[1:] {
		bounds = bounds.Union(m.Bounds())
	}
	return bounds
}

const (
	syntheticCellWidth  = 6400
	syntheticCellHeight = 3200
)

func monitorTemplate(index int) []Rect {
	switch index % 4 {
	case 0:
		return []Rect{{0, 0, 1920, 1080}, {1920, 0, 1920, 1080}}
	case 1:
		return []Rect{{0, 0, 2560, 1440}, {2560, 360, 1920, 1080}}
	case 2:
		return []Rect{{0, 0, 1920, 1080}, {1920, 0, 1920, 1080}, {3840, 0, 1920, 1080}}
	default:
		return []Rect{{0, 1080, 2560, 1440}, {320, 0, 1920, 1080}, {2560, 0, 1080, 1920}}
	}
}

func SyntheticMachines(n int) []Machine {
	columns := int(math.Ceil(math.Sqrt(float64(n))))
	machines := make([]Machine, n)
	for i := range machines {
		machines[i] = Machine{
			Name:     fmt.Sprintf("PC-%02d", i+1),
			X:        float32(i%columns) * syntheticCellWidth,
			Y:        float32(i/columns) * syntheticCellHeight,
			Monitors: monitorTemplate(i),
		}
	}
	return machines
}

type View struct {
	Zoom       float32
	PanX, PanY float32
}

func (v View) ToScreen(x, y float32) (float32, float32) {
	return x*v.Zoom + v.PanX, y*v.Zoom + v.PanY
}

func (v View) ToCanvas(sx, sy float32) (float32, float32) {
	return (sx - v.PanX) / v.Zoom, (sy - v.PanY) / v.Zoom
}

func (v View) ZoomAround(factor, cursorX, cursorY float32) View {
	x, y := v.ToCanvas(cursorX, cursorY)
	zoom := min(max(v.Zoom*factor, MinZoom), MaxZoom)
	return View{Zoom: zoom, PanX: cursorX - x*zoom, PanY: cursorY - y*zoom}
}

func (v View) Panned(dx, dy float32) View {
	v.PanX += dx
	v.PanY += dy
	return v
}

func FitView(bounds Rect, width, height, margin float32) View {
	zoom := min(max(min((width-2*margin)/bounds.W, (height-2*margin)/bounds.H), MinZoom), 1)
	return View{
		Zoom: zoom,
		PanX: (width-bounds.W*zoom)/2 - bounds.X*zoom,
		PanY: (height-bounds.H*zoom)/2 - bounds.Y*zoom,
	}
}
