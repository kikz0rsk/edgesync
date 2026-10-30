//go:build windows

// Command winclip offers delayed-rendering clipboard formats (CF_UNICODETEXT, HTML Format, Rich Text Format, PNG and
// CF_DIBV5) from a message-only window. It answers WM_RENDERFORMAT after an artificial delay, renders every still
// delayed format on exit through WM_RENDERALLFORMATS, and logs each copy made by other applications through
// WM_CLIPBOARDUPDATE, ignoring its own offers.
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
	"time"
)

const defaultRenderDelay = 2 * time.Second

type config struct {
	delay     time.Duration
	sensitive bool
}

func parseFlags(args []string) (config, error) {
	var cfg config
	fs := flag.NewFlagSet("winclip", flag.ContinueOnError)
	fs.DurationVar(&cfg.delay, "delay", defaultRenderDelay, "sleep before each WM_RENDERFORMAT is answered (0 = none)")
	fs.BoolVar(&cfg.sensitive, "sensitive", false,
		"also offer ExcludeClipboardContentFromMonitorProcessing and CanIncludeInClipboardHistory")
	if err := fs.Parse(args); err != nil {
		return config{}, err
	}
	if cfg.delay < 0 {
		return config{}, fmt.Errorf("-delay must be >= 0, got %v", cfg.delay)
	}
	return cfg, nil
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "winclip:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	cfg, err := parseFlags(args)
	if err != nil {
		return err
	}
	runtime.LockOSThread()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	formats, err := buildDelayedFormats(cfg.sensitive)
	if err != nil {
		return err
	}
	offer := &clipboardOffer{delay: cfg.delay, log: logger}
	l, err := newListener(logger, offer)
	if err != nil {
		return err
	}
	if err := offer.place(formats); err != nil {
		return errors.Join(err, l.close())
	}
	logger.Info("delayed formats are on the clipboard; copy in other applications to see updates, "+
		"paste to see WM_RENDERFORMAT, press Ctrl+C to exit",
		"delay", cfg.delay, "sensitive", cfg.sensitive, "offered", len(formats))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	pumpErr := l.pump(ctx)
	return errors.Join(pumpErr, l.close())
}
