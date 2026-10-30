//go:build linux

// Command wlclip exercises the Clipboard portal on GNOME or KDE Wayland. It attaches the clipboard to a RemoteDesktop
// session (-mode rd) or to an InputCapture session version 2 (-mode ic), claims the clipboard with five MIME types,
// answers paste requests after a delay, and reads every offered MIME type of a local copy. Only sizes are logged;
// clipboard contents never reach the log.
package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"io/fs"
	"log/slog"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/godbus/dbus/v5"

	"github.com/kikz0r/edgesync/spikes/linux/portal"
)

const (
	modeRemoteDesktop = "rd"
	modeInputCapture  = "ic"
	armNone           = "none"
	armRight          = "right"
	armLeft           = "left"

	mimeText       = "text/plain;charset=utf-8"
	mimeHTML       = "text/html"
	mimePNG        = "image/png"
	mimeURIList    = "text/uri-list"
	mimeGnomeFiles = "x-special/gnome-copied-files"

	barrierID       = 1
	pngSide         = 32
	shutdownTimeout = 5 * time.Second
	sampleContents  = "edgesync wlclip sample\n"
)

type config struct {
	mode      string
	logPath   string
	tokenPath string
	sample    string
	delay     time.Duration
	wait      time.Duration
	arm       string
	hold      time.Duration
}

func offeredMimes() []string {
	return []string{mimeText, mimeHTML, mimePNG, mimeURIList, mimeGnomeFiles}
}

func parseFlags(args []string) (config, error) {
	var cfg config
	flags := flag.NewFlagSet("wlclip", flag.ContinueOnError)
	flags.StringVar(&cfg.mode, "mode", modeRemoteDesktop, "session the clipboard attaches to: rd (RemoteDesktop) or ic (InputCapture version 2)")
	flags.StringVar(&cfg.logPath, "log", "out/wlclip.log", "log file (appended; the same lines go to stdout)")
	flags.StringVar(&cfg.tokenPath, "token-file", "", "restore token file (default out/clip-<mode>-token)")
	flags.StringVar(&cfg.sample, "sample", "out/wlclip-sample.txt", "file that text/uri-list and x-special/gnome-copied-files point to")
	flags.DurationVar(&cfg.delay, "delay", 5*time.Second, "pause before a SelectionTransfer is answered")
	flags.DurationVar(&cfg.wait, "wait", 120*time.Second, "how long the run serves the clipboard")
	flags.StringVar(&cfg.arm, "arm", armNone, "ic only: none (session started, capture disabled), right or left (enable capture on that edge)")
	flags.DurationVar(&cfg.hold, "hold", 5*time.Second, "ic with -arm: pause between Activated and Release")
	if err := flags.Parse(args); err != nil {
		return config{}, err
	}
	switch cfg.mode {
	case modeRemoteDesktop, modeInputCapture:
	default:
		return config{}, fmt.Errorf("-mode must be %q or %q, got %q", modeRemoteDesktop, modeInputCapture, cfg.mode)
	}
	switch cfg.arm {
	case armNone, armRight, armLeft:
	default:
		return config{}, fmt.Errorf("-arm must be %q, %q or %q, got %q", armNone, armRight, armLeft, cfg.arm)
	}
	if cfg.mode == modeRemoteDesktop && cfg.arm != armNone {
		return config{}, errors.New("-arm needs -mode ic")
	}
	if cfg.delay < 0 || cfg.hold < 0 {
		return config{}, errors.New("-delay and -hold must not be negative")
	}
	if cfg.wait <= 0 {
		return config{}, errors.New("-wait must be positive")
	}
	if cfg.tokenPath == "" {
		cfg.tokenPath = filepath.Join("out", "clip-"+cfg.mode+"-token")
	}
	return cfg, nil
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "wlclip:", err)
		os.Exit(1)
	}
}

func run(args []string) (err error) {
	cfg, err := parseFlags(args)
	if err != nil {
		return err
	}
	log, closeLog, err := openLogger(cfg.logPath)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, closeLog()) }()

	sample, err := writeSample(cfg.sample)
	if err != nil {
		return err
	}

	sigCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(sigCtx, cfg.wait)
	defer cancel()

	bus, err := portal.ConnectSessionBus()
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, bus.Close()) }()
	log.Info("connected", "unique_name", bus.UniqueName(), "mode", cfg.mode, "arm", cfg.arm,
		"delay", cfg.delay, "wait", cfg.wait, "sample", sample)

	caller := portal.NewCaller(bus, portal.RandomTokens("edgesync"))
	clip := portal.NewClipboard(caller)
	clipVersion, err := clip.Version(ctx)
	if err != nil {
		return err
	}
	log.Info("Clipboard", "version", clipVersion)

	fails := &failures{}
	switch cfg.mode {
	case modeRemoteDesktop:
		err = runRemoteDesktop(ctx, log, caller, clip, cfg, sample, fails)
	case modeInputCapture:
		err = runInputCapture(ctx, log, caller, clip, cfg, sample, fails)
	}
	return errors.Join(err, fails.err())
}

func runRemoteDesktop(ctx context.Context, log *slog.Logger, caller *portal.Caller, clip *portal.Clipboard, cfg config, sample string, fails *failures) (err error) {
	rd := portal.NewRemoteDesktop(caller)
	version, err := rd.Version(ctx)
	if err != nil {
		return err
	}
	log.Info("RemoteDesktop", "version", version)
	token, err := readToken(cfg.tokenPath)
	if err != nil {
		return err
	}
	session, err := rd.OpenWithClipboard(ctx, portal.RemoteParams{
		Types:        portal.DeviceKeyboard | portal.DevicePointer,
		PersistMode:  portal.PersistUntilRevoked,
		RestoreToken: token,
	})
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, closeSession(ctx, session.Close)) }()
	log.Info("session started", "path", session.Path(), "devices", session.Devices,
		"clipboard_enabled", session.ClipboardEnabled, "restore_token_returned", session.RestoreToken != "")
	if err := saveRestoreToken(log, cfg.tokenPath, session.RestoreToken); err != nil {
		return err
	}
	return exchange(ctx, log, clip, session.Path(), cfg, sample, fails)
}

func runInputCapture(ctx context.Context, log *slog.Logger, caller *portal.Caller, clip *portal.Clipboard, cfg config, sample string, fails *failures) (err error) {
	ic := portal.NewInputCapture(caller)
	version, err := ic.Version(ctx)
	if err != nil {
		return err
	}
	log.Info("InputCapture", "version", version)
	token, err := readToken(cfg.tokenPath)
	if err != nil {
		return err
	}
	session, err := ic.OpenWithClipboard(ctx, portal.CaptureParams{
		Capabilities: portal.CapabilityKeyboard | portal.CapabilityPointer,
		PersistMode:  portal.PersistUntilRevoked,
		RestoreToken: token,
	})
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, closeSession(ctx, session.Close)) }()
	log.Info("session started", "path", session.Path(), "granted", session.Granted,
		"clipboard_enabled", session.ClipboardEnabled, "restore_token_returned", session.RestoreToken != "")
	if err := saveRestoreToken(log, cfg.tokenPath, session.RestoreToken); err != nil {
		return err
	}
	if cfg.arm == armNone {
		log.Info("session inactive: capture not enabled, clipboard served without activation")
		return exchange(ctx, log, clip, session.Path(), cfg, sample, fails)
	}
	return armAndExchange(ctx, log, clip, session, cfg, sample, fails)
}

func armAndExchange(ctx context.Context, log *slog.Logger, clip *portal.Clipboard, session *portal.CaptureSession, cfg config, sample string, fails *failures) (err error) {
	events, err := session.Events(ctx)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, events.Close()) }()
	eis, err := session.ConnectToEIS(ctx)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, eis.Close()) }()
	edge := portal.EdgeRight
	if cfg.arm == armLeft {
		edge = portal.EdgeLeft
	}
	armed, err := session.Arm(ctx, edge, barrierID)
	if err != nil {
		return err
	}
	log.Info("armed", "edge", edge, "zone_set", armed.ZoneSet.ID, "zone", armed.Zone, "barrier", armed.Barrier.Position)

	activations, cancel := context.WithCancel(ctx)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := releaseOnActivation(activations, log, session, events, armed.Zone, cfg.hold); err != nil {
			fails.add(err)
		}
	}()
	exchangeErr := exchange(ctx, log, clip, session.Path(), cfg, sample, fails)
	cancel()
	wg.Wait()
	return exchangeErr
}

func releaseOnActivation(ctx context.Context, log *slog.Logger, session *portal.CaptureSession, events *portal.EventStream, zone portal.Zone, hold time.Duration) error {
	for {
		select {
		case <-ctx.Done():
			return nil
		case ev, ok := <-events.Events():
			if !ok {
				return events.Err()
			}
			switch e := ev.(type) {
			case portal.ActivatedEvent:
				log.Info("Activated", "activation_id", e.ActivationID, "cursor_present", e.HasCursor,
					"cursor_x", e.Cursor.X, "cursor_y", e.Cursor.Y, "barrier_id", e.BarrierID, "barrier_present", e.HasBarrier)
				if err := holdThenRelease(ctx, log, session, e, zone, hold); err != nil {
					return err
				}
			case portal.DeactivatedEvent:
				log.Info("Deactivated", "activation_id", e.ActivationID, "activation_id_present", e.HasActivationID)
			case portal.DisabledEvent:
				log.Info("Disabled")
			case portal.ZonesChangedEvent:
				log.Info("ZonesChanged", "zone_set", e.ZoneSet, "zone_set_present", e.HasZoneSet)
			case portal.ClosedEvent:
				log.Warn("InputCapture session closed by the portal")
				return nil
			}
		}
	}
}

func holdThenRelease(ctx context.Context, log *slog.Logger, session *portal.CaptureSession, e portal.ActivatedEvent, zone portal.Zone, hold time.Duration) error {
	timer := time.NewTimer(hold)
	select {
	case <-timer.C:
	case <-ctx.Done():
		timer.Stop()
	}
	position := portal.ReleasePosition(zone, e.Cursor)
	if err := session.Release(context.WithoutCancel(ctx), e.ActivationID, position); err != nil {
		return fmt.Errorf("release: %w", err)
	}
	log.Info("Release sent", "activation_id", e.ActivationID, "x", position.X, "y", position.Y)
	return nil
}

func exchange(ctx context.Context, log *slog.Logger, clip *portal.Clipboard, session dbus.ObjectPath, cfg config, sample string, fails *failures) (err error) {
	events, err := clip.Events(ctx, session)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, events.Close()) }()

	mimes := offeredMimes()
	if err := clip.SetSelection(ctx, session, mimes); err != nil {
		return fmt.Errorf("set selection: %w", err)
	}
	log.Info("SetSelection", "mime_types", strings.Join(mimes, ", "))

	content := func(mime string) ([]byte, error) {
		return payload(mime, sample)
	}
	var wg sync.WaitGroup
	defer wg.Wait()
	for {
		select {
		case <-ctx.Done():
			return nil
		case ev, ok := <-events.Events():
			if !ok {
				return events.Err()
			}
			switch e := ev.(type) {
			case portal.SelectionTransferEvent:
				wg.Add(1)
				go func() {
					defer wg.Done()
					serveTransfer(ctx, log, clip, session, e, cfg.delay, content, fails)
				}()
			case portal.SelectionOwnerChangedEvent:
				log.Info("SelectionOwnerChanged", "mime_types_present", e.HasMimeTypes, "mime_types", strings.Join(e.MimeTypes, ", "),
					"session_is_owner_present", e.HasSessionIsOwner, "session_is_owner", e.SessionIsOwner)
				if e.HasSessionIsOwner && e.SessionIsOwner {
					continue
				}
				wg.Add(1)
				go func() {
					defer wg.Done()
					readOffered(ctx, log, clip, session, e.MimeTypes, fails)
				}()
			case portal.ClosedEvent:
				log.Warn("Clipboard session closed by the portal")
				return nil
			}
		}
	}
}

func serveTransfer(ctx context.Context, log *slog.Logger, clip *portal.Clipboard, session dbus.ObjectPath, ev portal.SelectionTransferEvent, delay time.Duration, content func(string) ([]byte, error), fails *failures) {
	log.Info("SelectionTransfer", "mime", ev.MimeType, "serial", ev.Serial, "delay", delay)
	if err := clip.ServeTransfer(ctx, session, ev, delay, content); err != nil {
		log.Error("SelectionTransfer failed", "mime", ev.MimeType, "serial", ev.Serial, "err", err)
		fails.add(err)
		return
	}
	log.Info("SelectionTransfer answered", "mime", ev.MimeType, "serial", ev.Serial)
}

func readOffered(ctx context.Context, log *slog.Logger, clip *portal.Clipboard, session dbus.ObjectPath, mimes []string, fails *failures) {
	for _, mime := range mimes {
		data, err := clip.ReadAll(ctx, session, mime)
		if err != nil {
			log.Error("SelectionRead failed", "mime", mime, "err", err)
			fails.add(err)
			continue
		}
		log.Info("SelectionRead", "mime", mime, "bytes", len(data))
	}
}

func payload(mime, sample string) ([]byte, error) {
	switch mime {
	case mimeText:
		return []byte("edgesync wlclip " + time.Now().UTC().Format(time.RFC3339) + "\n"), nil
	case mimeHTML:
		return []byte("<meta charset=\"utf-8\"><p><b>edgesync</b> wlclip</p>"), nil
	case mimePNG:
		return solidPNG()
	case mimeURIList:
		return []byte(sampleURI(sample) + "\r\n"), nil
	case mimeGnomeFiles:
		return []byte("copy\n" + sampleURI(sample) + "\n"), nil
	default:
		return nil, fmt.Errorf("no payload for %q", mime)
	}
}

func sampleURI(path string) string {
	return (&url.URL{Scheme: "file", Path: path}).String()
}

func solidPNG() ([]byte, error) {
	img := image.NewRGBA(image.Rect(0, 0, pngSide, pngSide))
	pixel := color.RGBA{R: 0x2e, G: 0x7d, B: 0x32, A: 0xff}
	for y := range pngSide {
		for x := range pngSide {
			img.SetRGBA(x, y, pixel)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, fmt.Errorf("encode png: %w", err)
	}
	return buf.Bytes(), nil
}

type failures struct {
	mu   sync.Mutex
	errs []error
}

func (f *failures) add(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.errs = append(f.errs, err)
}

func (f *failures) err() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	return errors.Join(f.errs...)
}

func closeSession(ctx context.Context, closeFn func(context.Context) error) error {
	closeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), shutdownTimeout)
	defer cancel()
	return closeFn(closeCtx)
}

func writeSample(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve sample path: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o750); err != nil {
		return "", fmt.Errorf("create sample directory: %w", err)
	}
	if err := os.WriteFile(abs, []byte(sampleContents), 0o600); err != nil {
		return "", fmt.Errorf("write sample file: %w", err)
	}
	return abs, nil
}

func openLogger(path string) (*slog.Logger, func() error, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return nil, nil, fmt.Errorf("create log directory: %w", err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, nil, fmt.Errorf("open log file: %w", err)
	}
	handler := slog.NewTextHandler(io.MultiWriter(os.Stdout, f), &slog.HandlerOptions{Level: slog.LevelDebug})
	return slog.New(handler), f.Close, nil
}

func readToken(path string) (string, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("read restore token: %w", err)
	}
	return strings.TrimSpace(string(data)), nil
}

func saveRestoreToken(log *slog.Logger, path, token string) error {
	if token == "" {
		log.Info("no restore token returned")
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return fmt.Errorf("create token directory: %w", err)
	}
	if err := os.WriteFile(path, []byte(token), 0o600); err != nil {
		return fmt.Errorf("write restore token: %w", err)
	}
	log.Info("restore token saved", "file", path)
	return nil
}
