// Command dndx11 exercises XDND on a running X server (P0-SP3-007), normally a nested Xephyr with a file manager.
// With -mode source it opens a window; pressing button 1 on it starts a drag of a file URI (a synthetic sample file
// unless paths are given) that is dropped on whatever XDND target is under the pointer. With -mode target it opens a
// window that accepts text/uri-list drops from a file manager and prints the offered types and the dropped paths.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"time"

	"github.com/kikz0r/edgesync/spikes/linux/dnd"
)

const sampleName = "dndx11-sample.txt"

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "dndx11:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("dndx11", flag.ContinueOnError)
	display := fs.String("display", "", "X display (default $DISPLAY)")
	mode := fs.String("mode", "target", "source or target")
	duration := fs.Duration("duration", 2*time.Minute, "how long to run")
	outDir := fs.String("out", "out", "directory for the synthetic sample file of source mode")
	x := fs.Int("x", 40, "window x")
	y := fs.Int("y", 40, "window y")
	width := fs.Int("w", 240, "window width")
	height := fs.Int("h", 160, "window height")
	statusTimeout := fs.Duration("status-timeout", dnd.DefaultStatusTimeout, "source: wait for the XdndStatus before a drop")
	finishTimeout := fs.Duration("finish-timeout", dnd.DefaultFinishTimeout, "source: wait for the XdndFinished after a drop")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *duration <= 0 || *width <= 0 || *height <= 0 {
		return errors.New("-duration, -w and -h must be positive")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, *duration)
	defer cancel()

	d, err := dnd.Open(*display)
	if err != nil {
		return err
	}
	defer d.Close()
	log := slog.New(slog.NewTextHandler(os.Stdout, nil))

	switch *mode {
	case "target":
		return runTarget(ctx, d, log, dnd.TargetOptions{X: int16(*x), Y: int16(*y), Width: uint16(*width), Height: uint16(*height), Title: "edgesync dndx11 target"})
	case "source":
		paths, err := sourcePaths(*outDir, fs.Args())
		if err != nil {
			return err
		}
		opts := dnd.SourceOptions{X: int16(*x), Y: int16(*y), Width: uint16(*width), Height: uint16(*height), Title: "edgesync dndx11 source"}
		return runSource(ctx, d, log, paths, opts, dnd.DragOptions{StatusTimeout: *statusTimeout, FinishTimeout: *finishTimeout})
	default:
		return fmt.Errorf("unknown -mode %q", *mode)
	}
}

func sourcePaths(outDir string, args []string) ([]string, error) {
	if len(args) == 0 {
		sample, err := writeSample(outDir)
		if err != nil {
			return nil, err
		}
		return []string{sample}, nil
	}
	paths := make([]string, len(args))
	for i, a := range args {
		abs, err := filepath.Abs(a)
		if err != nil {
			return nil, fmt.Errorf("resolve %q: %w", a, err)
		}
		paths[i] = abs
	}
	return paths, nil
}

func writeSample(outDir string) (string, error) {
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return "", fmt.Errorf("create %s: %w", outDir, err)
	}
	path, err := filepath.Abs(filepath.Join(outDir, sampleName))
	if err != nil {
		return "", fmt.Errorf("resolve sample path: %w", err)
	}
	if err := os.WriteFile(path, []byte("edgesync dndx11 sample file\n"), 0o644); err != nil {
		return "", fmt.Errorf("write %s: %w", path, err)
	}
	return path, nil
}

func runSource(ctx context.Context, d *dnd.Display, log *slog.Logger, paths []string, opts dnd.SourceOptions, drag dnd.DragOptions) error {
	source, err := d.NewSource(paths, opts)
	if err != nil {
		return err
	}
	log.Info("source ready: press button 1 on the window and drag to a target, button 3 cancels", "window", source.Window(), "paths", paths)
	for {
		pressTime, err := source.WaitPress(ctx)
		if err != nil {
			return ignoreContextEnd(ctx, err)
		}
		drag.Time = pressTime
		started := time.Now()
		result, err := source.Drag(ctx, drag)
		if err != nil {
			return ignoreContextEnd(ctx, err)
		}
		log.Info("drag ended", "outcome", result.Outcome, "target", result.Target, "action", result.Action, "elapsed", time.Since(started))
	}
}

func runTarget(ctx context.Context, d *dnd.Display, log *slog.Logger, opts dnd.TargetOptions) error {
	target, err := d.NewTarget(opts)
	if err != nil {
		return err
	}
	log.Info("target ready: drag a file from a file manager onto the window", "window", target.Window())
	err = target.Run(ctx, func(r dnd.TargetReport) { printReport(d, log, r) })
	return ignoreContextEnd(ctx, err)
}

func printReport(d *dnd.Display, log *slog.Logger, r dnd.TargetReport) {
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
		attrs = append(attrs, "paths", r.Dropped.Paths)
		for _, rejection := range r.Dropped.Rejected {
			log.Warn("entry rejected", "uri", rejection.URI, "error", rejection.Err)
		}
	}
	if r.Err != nil {
		log.Error("drop failed", append(attrs, "error", r.Err)...)
		return
	}
	log.Info("target", attrs...)
}

// ignoreContextEnd treats the end of -duration or an interrupt as the normal end of a mode.
func ignoreContextEnd(ctx context.Context, err error) error {
	if ctx.Err() != nil && errors.Is(err, ctx.Err()) {
		return nil
	}
	return err
}
