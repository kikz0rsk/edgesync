//go:build linux && cgo

package main

import (
	"context"
	"fmt"
	"image/color"
	"log/slog"
	"os"
	"time"

	"gioui.org/app"
	"gioui.org/font/gofont"
	"gioui.org/io/system"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget/material"
)

// runGio does not return on success: Gio's Main blocks forever on Linux, so the process exits when the window ends.
func runGio(ctx context.Context, log *slog.Logger, o gioOptions) error {
	go func() {
		if err := serveWindow(ctx, log, o); err != nil {
			fmt.Fprintln(os.Stderr, "wloverlay:", err)
			os.Exit(1)
		}
		os.Exit(0)
	}()
	app.Main()
	return nil
}

func serveWindow(ctx context.Context, log *slog.Logger, o gioOptions) error {
	window := new(app.Window)
	window.Option(
		app.Title(o.title),
		app.Size(unit.Dp(o.width), unit.Dp(o.height)),
		app.Decorated(false),
		app.TopMost(true),
	)
	started := time.Now()
	closer := time.AfterFunc(o.duration, func() { window.Perform(system.ActionClose) })
	defer closer.Stop()
	stop := context.AfterFunc(ctx, func() { window.Perform(system.ActionClose) })
	defer stop()

	theme := material.NewTheme()
	theme.Shaper = text.NewShaper(text.WithCollection(gofont.Collection()))
	var ops op.Ops
	firstFrame := true
	for {
		switch e := window.Event().(type) {
		case app.DestroyEvent:
			log.Info("window destroyed", "after", time.Since(started))
			return e.Err
		case app.ConfigEvent:
			log.Info("ConfigEvent", "config", fmt.Sprintf("%+v", e.Config), "at", time.Since(started))
		case app.X11ViewEvent:
			log.Info("ViewEvent", "kind", "x11", "window", e.Window, "display_set", e.Display != nil)
		case app.WaylandViewEvent:
			log.Info("ViewEvent", "kind", "wayland", "display_set", e.Display != nil, "surface_set", e.Surface != nil, "toplevel_set", e.Toplevel != nil)
		case app.FrameEvent:
			if firstFrame {
				log.Info("first frame", "after", time.Since(started), "size", e.Size)
				firstFrame = false
			}
			gtx := app.NewContext(&ops, e)
			paint.Fill(gtx.Ops, color.NRGBA{R: 0x10, G: 0x10, B: 0x18, A: 0xff})
			label := material.H4(theme, o.text)
			label.Color = color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}
			layout.Center.Layout(gtx, label.Layout)
			e.Frame(gtx.Ops)
		}
	}
}
