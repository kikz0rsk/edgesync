//go:build linux && cgo

// Command giodroplinux probes file drops onto a Gio v0.10.3 window on Linux (P0-SP3-007). It prints the display
// server Gio chose and the native handles of its view event, listens for Gio's own transfer events (which exist for
// in-window drags and the clipboard) and, with -x11-proxy on an X11 window, sets XdndAware and XdndProxy on the Gio
// window through a second X connection so that XDND drops from a file manager reach the spike's drop target.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"image"
	"image/color"
	"log/slog"
	"os"
	"runtime/debug"
	"time"

	"gioui.org/app"
	"gioui.org/font/gofont"
	"gioui.org/io/event"
	"gioui.org/io/system"
	"gioui.org/io/transfer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget/material"
	"github.com/jezek/xgb/xproto"

	"github.com/kikz0r/edgesync/spikes/linux/dnd"
)

const gioModule = "gioui.org"

type options struct {
	duration time.Duration
	display  string
	proxy    bool
}

func main() {
	fs := flag.NewFlagSet("giodroplinux", flag.ContinueOnError)
	duration := fs.Duration("duration", 2*time.Minute, "how long the window stays open")
	display := fs.String("display", "", "X display for -x11-proxy (default $DISPLAY)")
	proxy := fs.Bool("x11-proxy", false, "on an X11 Gio window, accept XDND drops through an XdndProxy window")
	if err := fs.Parse(os.Args[1:]); err != nil {
		os.Exit(2)
	}
	if *duration <= 0 {
		fmt.Fprintln(os.Stderr, "giodroplinux: -duration must be positive")
		os.Exit(2)
	}
	log := slog.New(slog.NewTextHandler(os.Stdout, nil))
	go func() {
		if err := serve(context.Background(), log, options{duration: *duration, display: *display, proxy: *proxy}); err != nil {
			fmt.Fprintln(os.Stderr, "giodroplinux:", err)
			os.Exit(1)
		}
		os.Exit(0)
	}()
	app.Main()
}

func serve(ctx context.Context, log *slog.Logger, o options) error {
	logEnvironment(log)
	window := new(app.Window)
	window.Option(app.Title("EdgeSync Gio drop probe"), app.Size(unit.Dp(520), unit.Dp(260)))
	closer := time.AfterFunc(o.duration, func() { window.Perform(system.ActionClose) })
	defer closer.Stop()

	theme := material.NewTheme()
	theme.Shaper = text.NewShaper(text.WithCollection(gofont.Collection()))
	tag := new(int)
	filters := []event.Filter{
		transfer.TargetFilter{Target: tag, Type: "text/uri-list"},
		transfer.TargetFilter{Target: tag, Type: "text/plain"},
	}
	var (
		ops       op.Ops
		x11Proxy  *proxy
		gioEvents int
	)
	for {
		switch e := window.Event().(type) {
		case app.DestroyEvent:
			return errors.Join(e.Err, x11Proxy.stop())
		case app.X11ViewEvent:
			log.Info("ViewEvent", "kind", "x11", "valid", e.Valid(), "display_set", e.Display != nil, "window", e.Window)
			if err := x11Proxy.stop(); err != nil {
				return err
			}
			x11Proxy = nil
			if o.proxy && e.Valid() {
				p, err := startProxy(ctx, log, o.display, e.Window)
				if err != nil {
					return err
				}
				x11Proxy = p
			}
		case app.WaylandViewEvent:
			log.Info("ViewEvent", "kind", "wayland", "valid", e.Valid(), "display_set", e.Display != nil, "surface_set", e.Surface != nil, "toplevel_set", e.Toplevel != nil)
			if o.proxy {
				return errors.New("-x11-proxy needs an X11 window: start with WAYLAND_DISPLAY unset and DISPLAY set")
			}
		case app.FrameEvent:
			gtx := app.NewContext(&ops, e)
			area := clip.Rect(image.Rectangle{Max: gtx.Constraints.Max}).Push(gtx.Ops)
			event.Op(gtx.Ops, tag)
			area.Pop()
			for {
				ev, ok := gtx.Event(filters...)
				if !ok {
					break
				}
				gioEvents++
				log.Info("Gio transfer event", "type", fmt.Sprintf("%T", ev), "total", gioEvents)
			}
			paint.Fill(gtx.Ops, color.NRGBA{R: 0xf4, G: 0xf4, B: 0xf8, A: 0xff})
			label := material.Body1(theme, fmt.Sprintf("Drop files from the file manager here (Gio transfer events: %d)", gioEvents))
			layout.Center.Layout(gtx, label.Layout)
			e.Frame(gtx.Ops)
		}
	}
}

func logEnvironment(log *slog.Logger) {
	log.Info("environment",
		"XDG_SESSION_TYPE", os.Getenv("XDG_SESSION_TYPE"),
		"WAYLAND_DISPLAY", os.Getenv("WAYLAND_DISPLAY"),
		"DISPLAY", os.Getenv("DISPLAY"),
		"gio", moduleVersion(gioModule),
	)
}

func moduleVersion(path string) string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown (no build info)"
	}
	for _, dep := range info.Deps {
		if dep.Path == path {
			return dep.Version
		}
	}
	return "not found"
}

// proxy runs the XDND target that stands in for the Gio window. A nil *proxy is a valid, stopped value.
type proxy struct {
	display *dnd.Display
	target  *dnd.Target
	cancel  context.CancelFunc
	done    chan error
}

func startProxy(ctx context.Context, log *slog.Logger, display string, window uintptr) (*proxy, error) {
	d, err := dnd.Open(display)
	if err != nil {
		return nil, err
	}
	target, err := d.NewProxyTarget(xproto.Window(window))
	if err != nil {
		d.Close()
		return nil, err
	}
	runCtx, cancel := context.WithCancel(ctx)
	p := &proxy{display: d, target: target, cancel: cancel, done: make(chan error, 1)}
	go func() {
		p.done <- target.Run(runCtx, func(r dnd.TargetReport) { logReport(d, log, r) })
	}()
	log.Info("XdndProxy installed on the Gio window", "window", window)
	return p, nil
}

func (p *proxy) stop() error {
	if p == nil {
		return nil
	}
	p.cancel()
	runErr := <-p.done
	if errors.Is(runErr, context.Canceled) {
		runErr = nil
	}
	closeErr := p.target.Close()
	p.display.Close()
	return errors.Join(runErr, closeErr)
}

func logReport(d *dnd.Display, log *slog.Logger, r dnd.TargetReport) {
	attrs := []any{"event", r.Event}
	if len(r.Types) > 0 {
		names := make([]string, len(r.Types))
		for i, id := range r.Types {
			name, err := d.AtomName(id)
			if err != nil {
				name = fmt.Sprintf("atom %d (%v)", id, err)
			}
			names[i] = name
		}
		attrs = append(attrs, "offered", names)
	}
	if r.Dropped != nil {
		attrs = append(attrs, "paths", r.Dropped.Paths, "rejected", len(r.Dropped.Rejected))
	}
	if r.Err != nil {
		log.Error("XDND drop failed", append(attrs, "error", r.Err)...)
		return
	}
	log.Info("XDND", attrs...)
}
