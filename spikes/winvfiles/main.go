//go:build windows

// Command winvfiles places a folder, a 200 MiB file and a 10 KiB file on the Windows clipboard as virtual files
// (FileGroupDescriptorW plus one FileContents IStream per file) through a hand-built IDataObject and OleSetClipboard,
// serves their content lazily from a throttled synthetic source, and logs every call the shell makes on the data
// object.
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
	"golang.org/x/sys/windows"
)

const (
	currentPollInterval = 500 * time.Millisecond
	defaultRateMiBps    = 16.0
	defaultLatency      = 200 * time.Millisecond
)

var (
	ole32                     = windows.NewLazySystemDLL("ole32.dll")
	procOleSetClipboard       = ole32.NewProc("OleSetClipboard")
	procOleFlushClipboard     = ole32.NewProc("OleFlushClipboard")
	procOleIsCurrentClipboard = ole32.NewProc("OleIsCurrentClipboard")
)

type config struct {
	rateMiBps float64
	latency   time.Duration
	blockRead time.Duration
	flush     bool
}

func parseFlags(args []string) (config, error) {
	var cfg config
	fs := flag.NewFlagSet("winvfiles", flag.ContinueOnError)
	fs.Float64Var(&cfg.rateMiBps, "rate", defaultRateMiBps, "per-stream throughput in MiB/s (0 = unthrottled)")
	fs.DurationVar(&cfg.latency, "latency", defaultLatency, "delay before the first byte of every stream")
	fs.DurationVar(&cfg.blockRead, "blockread", 0, "block the very first Read for this long (0 = off)")
	fs.BoolVar(&cfg.flush, "flush", false, "call OleFlushClipboard on exit instead of leaving the clipboard to OLE")
	if err := fs.Parse(args); err != nil {
		return config{}, err
	}
	if math.IsNaN(cfg.rateMiBps) || math.IsInf(cfg.rateMiBps, 0) || cfg.rateMiBps < 0 {
		return config{}, fmt.Errorf("-rate must be a finite value >= 0, got %v", cfg.rateMiBps)
	}
	return cfg, nil
}

func (c config) sourceOptions() options {
	return options{
		rateBytesPerSec: int64(c.rateMiBps * bytesPerMiB),
		latency:         c.latency,
		blockRead:       c.blockRead,
	}
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "winvfiles:", err)
		os.Exit(1)
	}
}

func run(args []string) (err error) {
	cfg, err := parseFlags(args)
	if err != nil {
		return err
	}
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	source, err := newSyntheticSource(demoEntries(time.Now()), cfg.sourceOptions())
	if err != nil {
		return err
	}
	formats, err := registerShellFormats()
	if err != nil {
		return err
	}
	data, err := newClipboardData(source, formats, logger)
	if err != nil {
		return err
	}

	sta := &wincom.STA{}
	if err := sta.Start(); err != nil {
		return err
	}
	defer func() { err = errors.Join(err, sta.Close()) }()

	var dataObject uintptr
	if err := onSTA(sta, func() error {
		var createErr error
		dataObject, createErr = offerOnClipboard(data)
		return createErr
	}); err != nil {
		return err
	}
	logger.Info("virtual files are on the clipboard; paste into Explorer, press Ctrl+C to exit",
		"rate_mibps", cfg.rateMiBps, "latency", cfg.latency, "blockread", cfg.blockRead, "flush_on_exit", cfg.flush,
		"callbacks", wincom.CallbackCount())

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	watchOwnership(ctx, sta, dataObject, logger)

	if err := onSTA(sta, func() error { return shutdown(dataObject, cfg.flush, logger) }); err != nil {
		return err
	}
	report(source, data, logger)
	return nil
}

func onSTA(sta *wincom.STA, fn func() error) error {
	var result error
	if err := sta.Do(func() { result = fn() }); err != nil {
		return err
	}
	return result
}

func offerOnClipboard(data *clipboardData) (uintptr, error) {
	dataObject, err := wincom.NewDataObject(data)
	if err != nil {
		return 0, err
	}
	hr, _, _ := procOleSetClipboard.Call(dataObject)
	if result := wincom.HResult(hr); result.Failed() {
		wincom.Release(dataObject)
		return 0, fmt.Errorf("OleSetClipboard: %w", result)
	}
	return dataObject, nil
}

func isCurrentClipboard(dataObject uintptr) (bool, error) {
	hr, _, _ := procOleIsCurrentClipboard.Call(dataObject)
	switch result := wincom.HResult(hr); {
	case result == wincom.SOK:
		return true, nil
	case result == wincom.SFalse:
		return false, nil
	default:
		return false, fmt.Errorf("OleIsCurrentClipboard: %w", result)
	}
}

func watchOwnership(ctx context.Context, sta *wincom.STA, dataObject uintptr, logger *slog.Logger) {
	ticker := time.NewTicker(currentPollInterval)
	defer ticker.Stop()
	owned := true
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		var current bool
		err := onSTA(sta, func() error {
			var checkErr error
			current, checkErr = isCurrentClipboard(dataObject)
			return checkErr
		})
		switch {
		case err != nil:
			logger.Error("ownership check failed", "err", err)
		case owned && !current:
			owned = false
			logger.Warn("clipboard ownership lost: another copy replaced the virtual files")
		}
	}
}

func shutdown(dataObject uintptr, flush bool, logger *slog.Logger) (err error) {
	defer wincom.Release(dataObject)
	if !flush {
		logger.Info("exiting without OleFlushClipboard", "hosted_objects", wincom.ObjectCount())
		return nil
	}
	current, err := isCurrentClipboard(dataObject)
	if err != nil {
		return err
	}
	if !current {
		logger.Warn("OleFlushClipboard skipped: the data object is no longer on the clipboard")
		return nil
	}
	hr, _, _ := procOleFlushClipboard.Call()
	if result := wincom.HResult(hr); result.Failed() {
		return fmt.Errorf("OleFlushClipboard: %w", result)
	}
	logger.Info("OleFlushClipboard returned S_OK")
	return nil
}

func report(source *syntheticSource, data *clipboardData, logger *slog.Logger) {
	for _, stats := range source.Stats() {
		logger.Info("file accounting",
			"path", stats.Path, "size", stats.Size, "opens", stats.Opens, "reads", stats.Reads,
			"bytes_served", stats.BytesServed, "completed", stats.Completed, "aborted", stats.Aborted)
	}
	outcomes := data.Outcomes()
	logger.Info("paste outcomes",
		"performed_dropeffect_set", outcomes.PerformedSet, "performed_dropeffect", effectName(outcomes.Performed),
		"paste_succeeded_set", outcomes.PasteSucceeded, "paste_succeeded_effect", effectName(outcomes.PasteEffect))
}
