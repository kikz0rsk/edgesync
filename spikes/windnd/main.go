//go:build windows

// Command windnd probes OLE drag-and-drop on Windows in both directions, with hand-built COM objects from
// spikes/internal/wincom. A 2 px topmost layered window on the right edge of the primary monitor is registered as an
// IDropTarget and prints what a file drag from Explorer carries (CF_HDROP paths, FileGroupDescriptorW entries). It is
// click-through while no drag is active; a low-level mouse hook makes it hit-testable when the left button goes down
// outside the strip. A small source window starts DoDragDrop with a virtual-file IDataObject (a folder, a 200 MiB file
// and a 10 KiB file, served lazily) when the left button goes down in it.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"math"
	"os"
	"os/signal"
	"time"

	"github.com/kikz0r/edgesync/spikes/internal/wincom"
	"github.com/kikz0r/edgesync/spikes/internal/winhook"
)

const (
	defaultEdgeThickness = 2
	defaultRateMiBps     = 16.0
	defaultLatency       = 200 * time.Millisecond
	mouseRingCapacity    = 1 << 16
	mousePollInterval    = 2 * time.Millisecond
)

type config struct {
	edgePx    int
	rateMiBps float64
	latency   time.Duration
}

func parseFlags(args []string) (config, error) {
	var cfg config
	fs := flag.NewFlagSet("windnd", flag.ContinueOnError)
	fs.IntVar(&cfg.edgePx, "edge", defaultEdgeThickness, "thickness of the edge drop-target window in pixels")
	fs.Float64Var(&cfg.rateMiBps, "rate", defaultRateMiBps, "per-stream throughput of the drag source in MiB/s (0 = unthrottled)")
	fs.DurationVar(&cfg.latency, "latency", defaultLatency, "delay before the first byte of every drag-source stream")
	if err := fs.Parse(args); err != nil {
		return config{}, err
	}
	switch {
	case cfg.edgePx < 1:
		return config{}, fmt.Errorf("-edge must be at least 1, got %d", cfg.edgePx)
	case math.IsNaN(cfg.rateMiBps) || math.IsInf(cfg.rateMiBps, 0) || cfg.rateMiBps < 0:
		return config{}, fmt.Errorf("-rate must be a finite value >= 0, got %v", cfg.rateMiBps)
	case cfg.latency < 0:
		return config{}, fmt.Errorf("-latency must be >= 0, got %v", cfg.latency)
	}
	return cfg, nil
}

func (c config) streamOptions() streamOptions {
	return streamOptions{rateBytesPerSec: int64(c.rateMiBps * bytesPerMiB), latency: c.latency}
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "windnd:", err)
		os.Exit(1)
	}
}

type surfaces struct {
	edge     *edgeWindow
	target   *edgeDropTarget
	observer *edgeObserver
	source   *sourceWindow
}

func run(args []string) (err error) {
	cfg, err := parseFlags(args)
	if err != nil {
		return err
	}
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	if err = winhook.SetPerMonitorV2(); err != nil {
		return err
	}
	formats, err := registerShellFormats()
	if err != nil {
		return err
	}

	sta := &wincom.STA{}
	if err = sta.Start(); err != nil {
		return err
	}
	defer func() { err = errors.Join(err, sta.Close()) }()

	var wins surfaces
	if err = onSTA(sta, func() error {
		var createErr error
		wins, createErr = createWindows(logger, cfg, formats)
		return createErr
	}); err != nil {
		return err
	}
	defer func() {
		err = errors.Join(err, onSTA(sta, wins.close))
	}()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	ring, err := winhook.NewRing(mouseRingCapacity)
	if err != nil {
		return err
	}
	hook := winhook.New(winhook.Options{})
	if err = hook.Start(runCtx, ring); err != nil {
		return err
	}
	defer func() { err = errors.Join(err, hook.Close()) }()

	logger.Info("running; drag files from Explorer to the right edge of the primary monitor, or drag from the source window; Ctrl+C or closing the source window exits",
		"edge", fmt.Sprintf("%+v", wins.edge.bounds), "callbacks", wincom.CallbackCount(),
		"warning", "Scroll Lock toggles the hook's input suppression; do not press it")

	pumpDone := make(chan error, 1)
	go func() { pumpDone <- pumpMouse(runCtx, ring, wins.edge, wins.observer, logger) }()

	var pumpErr error
	pumpFinished := false
	select {
	case <-ctx.Done():
	case <-wins.source.Closed():
		logger.Info("source window closed")
	case <-hook.Done():
		err = errors.Join(err, fmt.Errorf("mouse hook stopped: %w", hook.Err()))
	case pumpErr = <-pumpDone:
		pumpFinished = true
	}
	cancel()
	if !pumpFinished {
		pumpErr = <-pumpDone
	}
	err = errors.Join(err, pumpErr)
	report(wins, ring, hook, logger)
	return err
}

func onSTA(sta *wincom.STA, fn func() error) error {
	var result error
	if err := sta.Do(func() { result = fn() }); err != nil {
		return err
	}
	return result
}

func createWindows(logger *slog.Logger, cfg config, formats shellFormats) (wins surfaces, err error) {
	monitor, err := primaryMonitorRect()
	if err != nil {
		return surfaces{}, err
	}
	bounds, err := rightEdgeRect(monitor, int32(cfg.edgePx))
	if err != nil {
		return surfaces{}, err
	}
	wins.observer = newEdgeObserver(bounds)
	wins.target = newEdgeDropTarget(logger, bounds, wins.observer, formats.Descriptor)
	if wins.edge, err = newEdgeWindow(bounds, wins.target); err != nil {
		return surfaces{}, err
	}
	logger.Info("edge drop target registered", "monitor", fmt.Sprintf("%+v", monitor), "edge", fmt.Sprintf("%+v", bounds))
	if wins.source, err = newSourceWindow(logger, formats, cfg.streamOptions()); err != nil {
		return surfaces{}, errors.Join(err, wins.edge.close())
	}
	return wins, nil
}

func (w surfaces) close() error {
	var errs []error
	if w.source != nil {
		errs = append(errs, w.source.close())
	}
	if w.edge != nil {
		errs = append(errs, w.edge.close())
	}
	return errors.Join(errs...)
}

// pumpMouse feeds the low-level hook events to the observer and applies its click-through decisions to the edge
// window. It runs on its own goroutine, not on the STA thread: the STA thread may sit inside the DoDragDrop modal loop,
// where queued work would not run.
func pumpMouse(ctx context.Context, ring *winhook.Ring, edge *edgeWindow, observer *edgeObserver, logger *slog.Logger) error {
	ticker := time.NewTicker(mousePollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
		for {
			ev, ok := ring.Pop()
			if !ok {
				break
			}
			if ev.Kind != winhook.KindMouse {
				continue
			}
			if err := applyMouse(ev, edge, observer, logger); err != nil {
				return err
			}
		}
	}
}

func applyMouse(ev winhook.Event, edge *edgeWindow, observer *edgeObserver, logger *slog.Logger) error {
	result := observer.observe(mouseEvent{Message: ev.Message, Pt: wincom.Point{X: ev.X, Y: ev.Y}})
	if result.Changed {
		if err := edge.setClickThrough(result.Transparent); err != nil {
			return fmt.Errorf("edge click-through to %t: %w", result.Transparent, err)
		}
		logger.Info("edge click-through changed", "click_through", result.Transparent, "x", ev.X, "y", ev.Y)
	}
	if report := result.Finished; report != nil {
		logger.Info("left button gesture finished",
			"x", ev.X, "y", ev.Y, "crossed_edge_while_pressed", report.CrossedEdge, "dragenter_fired", report.DragEntered)
		if report.CrossedEdge && !report.DragEntered {
			logger.Warn("the pointer crossed the edge strip with the left button held but no DragEnter arrived: " +
				"not an OLE file drag, a drag from an elevated process (UIPI), or the strip missed the drag")
		}
	}
	return nil
}

func report(wins surfaces, ring *winhook.Ring, hook *winhook.Hook, logger *slog.Logger) {
	gestures, missed := wins.observer.totals()
	totals := wins.target.Totals()
	logger.Info("edge target summary",
		"gestures", gestures, "crossed_without_dragenter", missed,
		"dragenter", totals.Enters, "dragover", totals.Overs, "dragover_outside_edge", totals.OverOutside,
		"dragleave", totals.Leaves, "drop", totals.Drops, "failures", totals.Failures)
	stats := hook.Stats()
	logger.Info("hook summary", "events", stats.Events, "ring_drops", ring.Drops(), "reinstalls", stats.Reinstalls,
		"max_callback", stats.MaxCallback)
	for i, record := range wins.source.History() {
		logger.Info("drag source session", "index", i, "result", record.Result, "failure", record.Failure,
			"phase", record.Session.Phase, "queries", record.Session.Queries, "feedback", record.Session.Feedback)
		if record.Data == nil {
			continue
		}
		for _, stream := range record.Data.Streams() {
			logger.Info("file accounting", "path", stream.Path, "size", stream.Size, "reads", stream.Reads,
				"bytes_served", stream.Served, "completed", stream.Completed)
		}
		outcomes := record.Data.Outcomes()
		logger.Info("drop outcomes", "performed_dropeffect_set", outcomes.PerformedSet,
			"performed_dropeffect", effectName(outcomes.Performed), "paste_succeeded_set", outcomes.PasteSucceeded,
			"paste_succeeded_effect", effectName(outcomes.PasteEffect))
	}
}
