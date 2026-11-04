//go:build darwin

// Command trayoverlay runs the macOS tray spike: an accessory-policy NSApplication with an "ES" status item whose menu
// calls back into Go and an overlay panel that appears for 800 ms. With -selftest it exits by itself, which is how the
// CI job launches the .app bundle.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"runtime"
	"sync"
	"syscall"
	"time"

	"github.com/kikz0r/edgesync/spikes/macos/fw"
	"github.com/kikz0r/edgesync/spikes/macos/ui"
)

const (
	actionOverlay = "overlay"
	actionQuit    = "quit"

	trayClassName = "ESSpikeTrayTarget"
	trayTitle     = "ES"
	overlayText   = "EdgeSync"
	pulseInterval = time.Second
)

// AppKit needs the process main thread; init runs on it before main starts.
func init() {
	runtime.LockOSThread()
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "trayoverlay:", err)
		os.Exit(1)
	}
}

func run() (err error) {
	selftest := flag.Duration("selftest", 0, "exit after this duration and re-show the overlay every second (0 runs until Quit)")
	overlayFor := flag.Duration("overlay", 800*time.Millisecond, "how long the overlay stays visible")
	flag.Parse()

	if _, err := fw.Load(); err != nil {
		return err
	}
	app, err := ui.NewApp()
	if err != nil {
		return err
	}
	policy := app.Policy()
	slog.Info("activation policy", "initial", policy.Initial, "setCalled", policy.SetCalled,
		"setReturned", policy.SetReturned, "final", policy.Final)
	overlay, err := ui.NewOverlay(app)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, overlay.Close()) }()

	tray, err := ui.NewStatusItem(app, ui.StatusItemConfig{
		ClassName: trayClassName,
		Title:     trayTitle,
		Items: []ui.MenuItem{
			{ID: actionOverlay, Label: "Show overlay"},
			{Separator: true},
			{ID: actionQuit, Label: "Quit"},
		},
		OnAction: func(id string) { handleAction(app, overlay, *overlayFor, id) },
	})
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, tray.Close()) }()

	title, err := tray.ButtonTitle()
	if err != nil {
		return err
	}
	visible, err := tray.Visible()
	if err != nil {
		return err
	}
	slog.Info("status item created", "buttonTitle", title, "visible", visible)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if *selftest > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, *selftest)
		defer cancel()
	}

	var pulse sync.WaitGroup
	pulseCtx, stopPulse := context.WithCancel(ctx)
	defer func() {
		stopPulse()
		pulse.Wait()
	}()
	if err := showAndReport(app, overlay, *overlayFor); err != nil {
		return err
	}
	if *selftest > 0 {
		pulse.Go(func() { pulseOverlay(pulseCtx, app, overlay, *overlayFor) })
	}

	slog.Info("run loop starting", "selftest", *selftest)
	if err := app.Run(ctx); err != nil {
		return err
	}
	slog.Info("run loop ended")
	return nil
}

func handleAction(app *ui.App, overlay *ui.Overlay, overlayFor time.Duration, id string) {
	switch id {
	case actionOverlay:
		if err := showAndReport(app, overlay, overlayFor); err != nil {
			app.Fail(err)
		}
	case actionQuit:
		if err := app.Stop(); err != nil {
			app.Fail(err)
		}
	default:
		app.Fail(fmt.Errorf("menu action %q: %w", id, ui.ErrUnknownMenuTag))
	}
}

// showAndReport queues the overlay and then a main-thread read-back of its visibility, which runs after the show
// because the main-thread queue is ordered.
func showAndReport(app *ui.App, overlay *ui.Overlay, d time.Duration) error {
	if err := overlay.Show(overlayText, d); err != nil {
		return err
	}
	return app.Post(func() error {
		visible, err := overlay.Visible()
		if err != nil {
			return err
		}
		slog.Info("overlay shown", "visible", visible, "for", d)
		return nil
	})
}

func pulseOverlay(ctx context.Context, app *ui.App, overlay *ui.Overlay, d time.Duration) {
	ticker := time.NewTicker(pulseInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := showAndReport(app, overlay, d); err != nil {
				if !errors.Is(err, ui.ErrStopped) {
					app.Fail(err)
				}
				return
			}
		}
	}
}
