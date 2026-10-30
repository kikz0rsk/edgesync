package main

import (
	"fmt"
	"image"
	"image/color"
	"math"

	"gioui.org/f32"
	"gioui.org/font/gofont"
	"gioui.org/gesture"
	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget/material"
)

const (
	snapThresholdLogical = 12
	zoomPerScrollUnit    = 0.001
	scrollRange          = 1 << 15
	fitMarginPx          = 40
	labelMinWidthPx      = 90
	nudgeStep            = 1
	nudgeStepShift       = 10
)

type palette struct {
	background    color.NRGBA
	text          color.NRGBA
	monitorBorder color.NRGBA
	selection     color.NRGBA
	guide         color.NRGBA
	overlap       color.NRGBA
	fills         []color.NRGBA
}

func lightPalette() palette {
	return palette{
		background:    color.NRGBA{0xF2, 0xF2, 0xF5, 0xFF},
		text:          color.NRGBA{0x1B, 0x1B, 0x1F, 0xFF},
		monitorBorder: color.NRGBA{0x5A, 0x5F, 0x6B, 0xFF},
		selection:     color.NRGBA{0x15, 0x65, 0xC0, 0xFF},
		guide:         color.NRGBA{0xE6, 0x51, 0x00, 0xFF},
		overlap:       color.NRGBA{0x2E, 0x7D, 0x32, 0xFF},
		fills: []color.NRGBA{
			{0xBB, 0xDE, 0xFB, 0xFF},
			{0xC8, 0xE6, 0xC9, 0xFF},
			{0xFF, 0xE0, 0xB2, 0xFF},
			{0xE1, 0xBE, 0xE7, 0xFF},
			{0xFF, 0xCD, 0xD2, 0xFF},
			{0xB2, 0xEB, 0xF2, 0xFF},
		},
	}
}

func darkPalette() palette {
	return palette{
		background:    color.NRGBA{0x1E, 0x1F, 0x24, 0xFF},
		text:          color.NRGBA{0xEC, 0xEC, 0xF1, 0xFF},
		monitorBorder: color.NRGBA{0x9A, 0xA0, 0xAD, 0xFF},
		selection:     color.NRGBA{0x64, 0xB5, 0xF6, 0xFF},
		guide:         color.NRGBA{0xFF, 0xB7, 0x4D, 0xFF},
		overlap:       color.NRGBA{0x81, 0xC7, 0x84, 0xFF},
		fills: []color.NRGBA{
			{0x1E, 0x3A, 0x5F, 0xFF},
			{0x1F, 0x4D, 0x2B, 0xFF},
			{0x5B, 0x3A, 0x12, 0xFF},
			{0x43, 0x26, 0x4F, 0xFF},
			{0x5A, 0x23, 0x28, 0xFF},
			{0x16, 0x4B, 0x52, 0xFF},
		},
	}
}

type machineView struct {
	machine Machine
	fill    int
	drag    gesture.Drag
}

type dragState struct {
	target      *machineView
	startX      float32
	startY      float32
	pointerDown f32.Point
}

type Canvas struct {
	theme      *material.Theme
	dark       bool
	view       View
	fitted     bool
	machines   []*machineView
	selected   *machineView
	dragging   *dragState
	guides     []Segment
	panning    bool
	panLast    f32.Point
	backdrop   byte
	keyFilters []event.Filter
}

func NewCanvas(machineCount int) *Canvas {
	theme := material.NewTheme()
	theme.Shaper = text.NewShaper(text.NoSystemFonts(), text.WithCollection(gofont.Collection()))

	c := &Canvas{
		theme: theme,
		keyFilters: []event.Filter{
			key.Filter{Name: key.NameLeftArrow, Optional: key.ModShift},
			key.Filter{Name: key.NameRightArrow, Optional: key.ModShift},
			key.Filter{Name: key.NameUpArrow, Optional: key.ModShift},
			key.Filter{Name: key.NameDownArrow, Optional: key.ModShift},
			key.Filter{Name: "T"},
		},
	}
	for i, m := range SyntheticMachines(machineCount) {
		c.machines = append(c.machines, &machineView{machine: m, fill: i})
	}
	return c
}

func (c *Canvas) MachineCount() int {
	return len(c.machines)
}

func (c *Canvas) palette() palette {
	if c.dark {
		return darkPalette()
	}
	return lightPalette()
}

func (c *Canvas) Layout(gtx layout.Context) {
	size := gtx.Constraints.Max
	if !c.fitted {
		machines := make([]Machine, len(c.machines))
		for i, mv := range c.machines {
			machines[i] = mv.machine
		}
		c.view = FitView(BoundsOf(machines), float32(size.X), float32(size.Y), fitMarginPx)
		c.fitted = true
	}

	c.handleKeys(gtx)
	c.handleMachines(gtx)
	c.handleBackdrop(gtx)

	colors := c.palette()
	window := clip.Rect(image.Rectangle{Max: size}).Push(gtx.Ops)
	paint.Fill(gtx.Ops, colors.background)
	event.Op(gtx.Ops, &c.backdrop)

	visible := image.Rectangle{Max: size}
	for _, mv := range c.machines {
		c.drawMachine(gtx, mv, colors, visible)
	}
	c.drawOverlapHighlight(gtx, colors)
	c.drawGuides(gtx, colors)
	c.drawHUD(gtx, colors, size)
	window.Pop()
}

func (c *Canvas) handleKeys(gtx layout.Context) {
	for {
		ev, ok := gtx.Event(c.keyFilters...)
		if !ok {
			return
		}
		ke, ok := ev.(key.Event)
		if !ok || ke.State != key.Press {
			continue
		}
		step := float32(nudgeStep)
		if ke.Modifiers.Contain(key.ModShift) {
			step = nudgeStepShift
		}
		switch ke.Name {
		case "T":
			c.dark = !c.dark
		case key.NameLeftArrow:
			c.nudge(-step, 0)
		case key.NameRightArrow:
			c.nudge(step, 0)
		case key.NameUpArrow:
			c.nudge(0, -step)
		case key.NameDownArrow:
			c.nudge(0, step)
		}
	}
}

func (c *Canvas) nudge(dx, dy float32) {
	if c.selected == nil {
		return
	}
	c.selected.machine.X += dx
	c.selected.machine.Y += dy
}

func (c *Canvas) handleMachines(gtx layout.Context) {
	var pressed *machineView
	for _, mv := range c.machines {
		for {
			e, ok := mv.drag.Update(gtx.Metric, gtx.Source, gesture.Both)
			if !ok {
				break
			}
			switch e.Kind {
			case pointer.Press:
				pressed = mv
				c.dragging = &dragState{
					target:      mv,
					startX:      mv.machine.X,
					startY:      mv.machine.Y,
					pointerDown: e.Position,
				}
			case pointer.Drag:
				c.dragTo(mv, e.Position)
			case pointer.Release, pointer.Cancel:
				if c.dragging != nil && c.dragging.target == mv {
					c.dragging = nil
					c.guides = nil
				}
			}
		}
	}
	if pressed != nil {
		c.selected = pressed
		c.raise(pressed)
	}
}

func (c *Canvas) dragTo(mv *machineView, at f32.Point) {
	if c.dragging == nil || c.dragging.target != mv {
		return
	}
	moved := mv.machine
	moved.X = c.dragging.startX + (at.X-c.dragging.pointerDown.X)/c.view.Zoom
	moved.Y = c.dragging.startY + (at.Y-c.dragging.pointerDown.Y)/c.view.Zoom

	others := make([]Group, 0, len(c.machines)-1)
	for _, other := range c.machines {
		if other != mv {
			others = append(others, other.machine.Group())
		}
	}
	dx, dy, guides := Snap(moved.Group(), others, snapThresholdLogical)
	mv.machine.X = moved.X + dx
	mv.machine.Y = moved.Y + dy
	c.guides = guides
}

func (c *Canvas) raise(mv *machineView) {
	for i, candidate := range c.machines {
		if candidate == mv {
			copy(c.machines[i:], c.machines[i+1:])
			c.machines[len(c.machines)-1] = mv
			return
		}
	}
}

func (c *Canvas) handleBackdrop(gtx layout.Context) {
	filter := pointer.Filter{
		Target:  &c.backdrop,
		Kinds:   pointer.Press | pointer.Drag | pointer.Release | pointer.Cancel | pointer.Scroll,
		ScrollX: pointer.ScrollRange{Min: -scrollRange, Max: scrollRange},
		ScrollY: pointer.ScrollRange{Min: -scrollRange, Max: scrollRange},
	}
	claimed := c.dragging != nil
	for {
		ev, ok := gtx.Event(filter)
		if !ok {
			return
		}
		e, ok := ev.(pointer.Event)
		if !ok {
			continue
		}
		switch e.Kind {
		case pointer.Press:
			switch {
			case e.Buttons == pointer.ButtonTertiary:
				c.panning = true
				c.panLast = e.Position
			case e.Buttons == pointer.ButtonPrimary && !claimed:
				c.selected = nil
			}
		case pointer.Drag:
			if c.panning && e.Buttons.Contain(pointer.ButtonTertiary) {
				c.view = c.view.Panned(e.Position.X-c.panLast.X, e.Position.Y-c.panLast.Y)
				c.panLast = e.Position
			}
		case pointer.Release, pointer.Cancel:
			c.panning = false
		case pointer.Scroll:
			if e.Modifiers.Contain(key.ModCtrl) {
				factor := float32(math.Exp(float64(-e.Scroll.Y) * zoomPerScrollUnit))
				c.view = c.view.ZoomAround(factor, e.Position.X, e.Position.Y)
			}
		}
	}
}

func (c *Canvas) screenRect(r Rect) image.Rectangle {
	x0, y0 := c.view.ToScreen(r.X, r.Y)
	x1, y1 := c.view.ToScreen(r.Right(), r.Bottom())
	return image.Rect(roundPx(x0), roundPx(y0), roundPx(x1), roundPx(y1))
}

func roundPx(v float32) int {
	return int(math.Round(float64(v)))
}

func (c *Canvas) drawMachine(gtx layout.Context, mv *machineView, colors palette, visible image.Rectangle) {
	group := mv.machine.Group()
	if !c.screenRect(group.Bounds()).Overlaps(visible) {
		return
	}
	fill := colors.fills[mv.fill%len(colors.fills)]
	border := colors.monitorBorder
	borderWidth := 1
	if mv == c.selected {
		border = colors.selection
		borderWidth = 3
	}
	for i, monitor := range group.Monitors {
		r := c.screenRect(monitor)
		paint.FillShape(gtx.Ops, fill, clip.Rect(r).Op())
		strokeRect(gtx.Ops, r, borderWidth, border)

		area := clip.Rect(r).Push(gtx.Ops)
		mv.drag.Add(gtx.Ops)
		pointer.CursorGrab.Add(gtx.Ops)
		area.Pop()

		if i == 0 && r.Dx() >= labelMinWidthPx {
			c.drawLabel(gtx, mv.machine.Name, r.Min.Add(image.Pt(8, 6)), 14, colors.text)
		}
	}
}

func (c *Canvas) drawLabel(gtx layout.Context, content string, at image.Point, size unit.Sp, textColor color.NRGBA) layout.Dimensions {
	label := material.Label(c.theme, size, content)
	label.Color = textColor
	offset := op.Offset(at).Push(gtx.Ops)
	defer offset.Pop()
	gtx.Constraints = layout.Constraints{Max: image.Pt(4096, 4096)}
	return label.Layout(gtx)
}

func strokeRect(ops *op.Ops, r image.Rectangle, width int, c color.NRGBA) {
	sides := [...]image.Rectangle{
		{Min: r.Min, Max: image.Pt(r.Max.X, r.Min.Y+width)},
		{Min: image.Pt(r.Min.X, r.Max.Y-width), Max: r.Max},
		{Min: r.Min, Max: image.Pt(r.Min.X+width, r.Max.Y)},
		{Min: image.Pt(r.Max.X-width, r.Min.Y), Max: r.Max},
	}
	for _, s := range sides {
		paint.FillShape(ops, c, clip.Rect(s).Op())
	}
}

func (c *Canvas) fillSegment(gtx layout.Context, s Segment, thickness int, fillColor color.NRGBA) {
	x0, y0 := c.view.ToScreen(min(s.X1, s.X2), min(s.Y1, s.Y2))
	x1, y1 := c.view.ToScreen(max(s.X1, s.X2), max(s.Y1, s.Y2))
	r := image.Rect(roundPx(x0), roundPx(y0), roundPx(x1), roundPx(y1))
	half := thickness / 2
	if s.X1 == s.X2 {
		r.Min.X -= half
		r.Max.X += thickness - half
	} else {
		r.Min.Y -= half
		r.Max.Y += thickness - half
	}
	paint.FillShape(gtx.Ops, fillColor, clip.Rect(r).Op())
}

func (c *Canvas) drawOverlapHighlight(gtx layout.Context, colors palette) {
	if c.selected == nil {
		return
	}
	selectedGroup := c.selected.machine.Group()
	for _, other := range c.machines {
		if other == c.selected {
			continue
		}
		for _, segment := range Overlap(selectedGroup, other.machine.Group()) {
			c.fillSegment(gtx, segment, 5, colors.overlap)
		}
	}
}

func (c *Canvas) drawGuides(gtx layout.Context, colors palette) {
	for _, guide := range c.guides {
		c.fillSegment(gtx, guide, 2, colors.guide)
	}
}

func (c *Canvas) drawHUD(gtx layout.Context, colors palette, size image.Point) {
	help := "drag: move | Ctrl+wheel: zoom | middle-drag: pan | arrows: nudge 1 (Shift 10) | T: theme"
	status := fmt.Sprintf("zoom %.1f%% | machines %d", c.view.Zoom*100, len(c.machines))
	if c.selected != nil {
		status += fmt.Sprintf(" | %s at (%.0f, %.0f)", c.selected.machine.Name, c.selected.machine.X, c.selected.machine.Y)
	}
	lineHeight := gtx.Dp(18)
	c.drawLabel(gtx, help, image.Pt(10, size.Y-2*lineHeight-6), 13, colors.text)
	c.drawLabel(gtx, status, image.Pt(10, size.Y-lineHeight-6), 13, colors.text)
}
