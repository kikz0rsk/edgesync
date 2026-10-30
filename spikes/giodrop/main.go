//go:build windows

// Package main is the Gio file-drop spike (P0-SP2-005). It takes the native HWND from Gio's Win32ViewEvent, registers an
// OLE IDropTarget on Gio's own window thread and shows the dropped paths and the drop point in device-independent pixels.
package main

import (
	"fmt"
	"os"

	"gioui.org/app"
	"gioui.org/font/gofont"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget/material"

	"github.com/kikz0r/edgesync/spikes/internal/wincom"
)

type view struct {
	theme *material.Theme
}

func newView() *view {
	theme := material.NewTheme()
	theme.Shaper = text.NewShaper(text.NoSystemFonts(), text.WithCollection(gofont.Collection()))
	return &view{theme: theme}
}

func (v *view) layout(gtx layout.Context, s dropSnapshot) layout.Dimensions {
	lines := describe(s, gtx.Metric.PxPerDp)
	children := make([]layout.FlexChild, len(lines))
	for i, line := range lines {
		children[i] = layout.Rigid(material.Body1(v.theme, line).Layout)
	}
	return layout.UniformInset(unit.Dp(16)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
	})
}

func describe(s dropSnapshot, pxPerDp float32) []string {
	lines := []string{"Drop files from Explorer onto this window."}
	if s.Registered {
		lines = append(lines,
			fmt.Sprintf("Window HWND: 0x%X", s.HWND),
			fmt.Sprintf("OleInitialize on the window thread: %s", oleResultName(s.OleResult)),
			"OLE drop target: registered on the Gio window thread",
		)
	} else {
		lines = append(lines, "OLE drop target: not registered")
	}
	lines = append(lines, fmt.Sprintf("Drops: %d", s.Drops))
	if s.Drops > 0 {
		x, y := dropPointDp(s.Last.Client, pxPerDp)
		lines = append(lines, fmt.Sprintf("Last drop point: %.1f x %.1f dp (client %d x %d px)",
			x, y, s.Last.Client.X, s.Last.Client.Y))
		for _, path := range s.Last.Paths {
			lines = append(lines, "  "+path)
		}
	}
	if s.Err != nil {
		lines = append(lines, "Last error: "+s.Err.Error())
	}
	return lines
}

func oleResultName(r wincom.HResult) string {
	switch r {
	case wincom.SOK:
		return "S_OK (first COM initialisation on this thread)"
	case wincom.SFalse:
		return "S_FALSE (COM already initialised on this thread)"
	default:
		return r.Error()
	}
}

// attachOnWindowThread runs the registration on Gio's window thread. Window.Run blocks until the closure has run.
func attachOnWindowThread(window *app.Window, target *dropTarget, hwnd uintptr) error {
	var err error
	window.Run(func() {
		err = target.attach(hwnd)
	})
	return err
}

func run(window *app.Window) error {
	target := newDropTarget(window.Invalidate)
	v := newView()
	var ops op.Ops
	for {
		switch e := window.Event().(type) {
		case app.Win32ViewEvent:
			// Gio sends an empty Win32ViewEvent when the window is destroyed; there is nothing to attach then.
			if e.HWND == 0 {
				continue
			}
			if err := attachOnWindowThread(window, target, e.HWND); err != nil {
				return err
			}
		case app.DestroyEvent:
			return e.Err
		case app.FrameEvent:
			gtx := app.NewContext(&ops, e)
			v.layout(gtx, target.snapshot())
			e.Frame(gtx.Ops)
		}
	}
}

func main() {
	go func() {
		window := new(app.Window)
		window.Option(app.Title("EdgeSync Gio file-drop spike"), app.Size(unit.Dp(720), unit.Dp(480)))
		if err := run(window); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		os.Exit(0)
	}()
	app.Main()
}
