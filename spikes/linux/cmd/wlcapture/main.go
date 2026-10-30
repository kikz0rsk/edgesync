//go:build linux

// Command wlcapture exercises the InputCapture portal on GNOME or KDE Wayland: it prints the interface versions
// and zones, arms a pointer barrier on the right edge, consumes the EI events of the capture, releases the
// pointer, then moves the barrier to the left edge on the same session and finally on a re-created session,
// persisting the restore token between runs.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/kikz0r/edgesync/spikes/linux/ei"
	"github.com/kikz0r/edgesync/spikes/linux/portal"
)

const (
	handshakeTimeout   = 10 * time.Second
	shutdownTimeout    = 5 * time.Second
	motionLogSampling  = 100
	motionLogFirst     = 5
	rightBarrierID     = 1
	leftBarrierID      = 2
	recreatedBarrierID = 3
)

type config struct {
	logPath   string
	tokenPath string
	wait      time.Duration
	hold      time.Duration
}

func parseFlags(args []string) (config, error) {
	var cfg config
	fs := flag.NewFlagSet("wlcapture", flag.ContinueOnError)
	fs.StringVar(&cfg.logPath, "log", "out/wlcapture.log", "log file (appended; the same lines go to stdout)")
	fs.StringVar(&cfg.tokenPath, "token-file", "out/ic-token", "file holding the InputCapture restore token")
	fs.DurationVar(&cfg.wait, "wait", 90*time.Second, "how long to wait for a barrier activation in each phase")
	fs.DurationVar(&cfg.hold, "hold", 5*time.Second, "how long to consume EI events after an activation")
	if err := fs.Parse(args); err != nil {
		return config{}, err
	}
	if cfg.wait <= 0 || cfg.hold <= 0 {
		return config{}, errors.New("-wait and -hold must be positive")
	}
	return cfg, nil
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "wlcapture:", err)
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

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	bus, err := portal.ConnectSessionBus()
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, bus.Close()) }()
	log.Info("connected", "unique_name", bus.UniqueName())

	caller := portal.NewCaller(bus, portal.RandomTokens("edgesync"))
	ic := portal.NewInputCapture(caller)
	version, err := logVersions(ctx, log, ic, portal.NewRemoteDesktop(caller))
	if err != nil {
		return err
	}

	persist := portal.PersistNone
	if version >= 2 {
		persist = portal.PersistUntilRevoked
	} else {
		log.Warn("InputCapture version 1: no persist_mode and no restore token")
	}
	token, err := readToken(cfg.tokenPath)
	if err != nil {
		return err
	}
	log.Info("restore token", "present", token != "", "file", cfg.tokenPath)

	first, err := openCapture(ctx, log, ic, cfg, persist, token)
	if err != nil {
		return err
	}
	right, err := first.phase(ctx, "right-edge", portal.EdgeRight, rightBarrierID)
	if err != nil {
		return errors.Join(err, first.close(ctx))
	}
	moved, err := first.phase(ctx, "left-edge-same-session", portal.EdgeLeft, leftBarrierID)
	if err != nil {
		return errors.Join(err, first.close(ctx))
	}
	if err := first.close(ctx); err != nil {
		return err
	}

	second, err := openCapture(ctx, log, ic, cfg, persist, first.restoreToken())
	if err != nil {
		return err
	}
	recreated, err := second.phase(ctx, "left-edge-recreated-session", portal.EdgeLeft, recreatedBarrierID)
	if err != nil {
		return errors.Join(err, second.close(ctx))
	}
	if err := second.close(ctx); err != nil {
		return err
	}

	log.Info("SUMMARY",
		"right_edge_capture_works", right.working(),
		"barrier_change_same_session_works", moved.working(),
		"barrier_change_session_closed_by_portal", moved.sessionClosed,
		"barrier_change_ei_ended", moved.eiEnded,
		"recreated_session_works", recreated.working(),
		"restore_token_issued", second.restoreToken() != "")
	return nil
}

func logVersions(ctx context.Context, log *slog.Logger, ic *portal.InputCapture, rd *portal.RemoteDesktop) (uint32, error) {
	version, err := ic.Version(ctx)
	if err != nil {
		return 0, err
	}
	caps, err := ic.SupportedCapabilities(ctx)
	if err != nil {
		return 0, err
	}
	log.Info("InputCapture", "version", version, "supported_capabilities", caps)
	rdVersion, rdErr := rd.Version(ctx)
	types, typesErr := rd.AvailableDeviceTypes(ctx)
	if err := errors.Join(rdErr, typesErr); err != nil {
		log.Error("RemoteDesktop interface not readable", "err", err)
		return version, nil
	}
	log.Info("RemoteDesktop", "version", rdVersion, "available_device_types", types)
	return version, nil
}

type captureRun struct {
	log          *slog.Logger
	cfg          config
	session      *portal.CaptureSession
	events       *portal.EventStream
	receiver     *receiver
	portalClosed bool
	activation   portal.ActivatedEvent
}

func openCapture(ctx context.Context, log *slog.Logger, ic *portal.InputCapture, cfg config, persist portal.PersistMode, token string) (*captureRun, error) {
	session, err := ic.Open(ctx, portal.CaptureParams{
		Capabilities: portal.CapabilityKeyboard | portal.CapabilityPointer,
		PersistMode:  persist,
		RestoreToken: token,
	})
	if err != nil {
		return nil, err
	}
	log.Info("session started", "path", session.Path(), "granted", session.Granted,
		"clipboard_enabled", session.ClipboardEnabled, "restore_token_returned", session.RestoreToken != "")
	run := &captureRun{log: log, cfg: cfg, session: session}
	fail := func(err error) (*captureRun, error) {
		return nil, errors.Join(err, run.close(ctx))
	}

	if session.RestoreToken != "" {
		if err := writeToken(cfg.tokenPath, session.RestoreToken); err != nil {
			return fail(err)
		}
		log.Info("restore token saved", "file", cfg.tokenPath)
	}
	if run.events, err = session.Events(ctx); err != nil {
		return fail(err)
	}
	eisFile, err := session.ConnectToEIS(ctx)
	if err != nil {
		return fail(err)
	}
	log.Info("ConnectToEIS returned a file descriptor")
	if run.receiver, err = startReceiver(ctx, log, eisFile); err != nil {
		return fail(err)
	}
	return run, nil
}

func (r *captureRun) restoreToken() string {
	return r.session.RestoreToken
}

func (r *captureRun) close(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), shutdownTimeout)
	defer cancel()
	var err error
	if r.events != nil {
		err = errors.Join(err, r.events.Close())
	}
	if r.receiver != nil {
		err = errors.Join(err, r.receiver.close())
	}
	if !r.portalClosed {
		err = errors.Join(err, r.session.Close(ctx))
	}
	return err
}

type phaseResult struct {
	activated     bool
	activationID  uint32
	events        counts
	deactivated   bool
	sessionClosed bool
	eiEnded       bool
}

func (p phaseResult) working() bool {
	return p.activated && p.events.total() > 0
}

func (r *captureRun) phase(ctx context.Context, name string, edge portal.Edge, barrierID uint32) (phaseResult, error) {
	armed, err := r.session.Arm(ctx, edge, barrierID)
	if err != nil {
		return phaseResult{}, fmt.Errorf("phase %s: %w", name, err)
	}
	r.log.Info("armed", "phase", name, "edge", edge, "zone_set", armed.ZoneSet.ID, "zones", fmt.Sprintf("%+v", armed.ZoneSet.Zones),
		"barrier_id", armed.Barrier.ID, "barrier", fmt.Sprintf("%+v", armed.Barrier.Position))
	r.log.Info("ACTION: push the pointer through the barrier", "phase", name, "edge", edge, "within", r.cfg.wait)

	res, err := r.awaitActivation(ctx, name)
	if err != nil || !res.activated {
		r.logResult(name, res)
		return res, err
	}
	res, err = r.hold(ctx, name, armed, res)
	r.logResult(name, res)
	return res, err
}

func (r *captureRun) awaitActivation(ctx context.Context, name string) (phaseResult, error) {
	timeout := time.NewTimer(r.cfg.wait)
	defer timeout.Stop()
	for {
		select {
		case <-ctx.Done():
			return phaseResult{}, context.Cause(ctx)
		case <-timeout.C:
			r.log.Warn("no activation before the timeout", "phase", name)
			return phaseResult{}, nil
		case <-r.receiver.done:
			r.receiver.logEnd(name)
			return phaseResult{eiEnded: true}, nil
		case ev, ok := <-r.events.Events():
			if !ok {
				return phaseResult{}, r.streamEnded()
			}
			if act, isAct := ev.(portal.ActivatedEvent); isAct {
				r.log.Info("Activated", "phase", name, "activation_id", act.ActivationID, "cursor", act.Cursor,
					"barrier_id", act.BarrierID, "barrier_known", act.HasBarrier)
				r.activation = act
				return phaseResult{activated: true, activationID: act.ActivationID}, nil
			}
			if closed := r.observe(name, ev); closed {
				return phaseResult{sessionClosed: true}, nil
			}
		}
	}
}

func (r *captureRun) streamEnded() error {
	if err := r.events.Err(); err != nil {
		return err
	}
	return portal.ErrStreamClosed
}

func (r *captureRun) observe(name string, ev portal.Event) (sessionClosed bool) {
	switch ev := ev.(type) {
	case portal.ClosedEvent:
		r.log.Warn("Session.Closed", "phase", name)
		r.portalClosed = true
		return true
	case portal.DeactivatedEvent:
		r.log.Info("Deactivated", "phase", name, "activation_id", ev.ActivationID)
	case portal.DisabledEvent:
		r.log.Warn("Disabled", "phase", name)
	case portal.ZonesChangedEvent:
		r.log.Warn("ZonesChanged", "phase", name, "zone_set", ev.ZoneSet)
	}
	return false
}

func (r *captureRun) hold(ctx context.Context, name string, armed portal.Armed, res phaseResult) (phaseResult, error) {
	before := r.receiver.counts.snapshot()
	timer := time.NewTimer(r.cfg.hold)
	defer timer.Stop()
holding:
	for {
		select {
		case <-ctx.Done():
			return res, context.Cause(ctx)
		case <-timer.C:
			break holding
		case <-r.receiver.done:
			res.eiEnded = true
			break holding
		case ev, ok := <-r.events.Events():
			if !ok {
				return res, r.streamEnded()
			}
			if _, isDeactivated := ev.(portal.DeactivatedEvent); isDeactivated {
				res.deactivated = true
			}
			if r.observe(name, ev) {
				res.sessionClosed = true
				break holding
			}
		}
	}
	res.events = r.receiver.counts.snapshot().minus(before)
	if res.eiEnded {
		r.receiver.logEnd(name)
	}
	if res.sessionClosed || res.eiEnded {
		return res, nil
	}
	release := portal.ReleasePosition(armed.Zone, r.activation.Cursor)
	if err := r.session.Release(ctx, res.activationID, release); err != nil {
		return res, fmt.Errorf("phase %s: %w", name, err)
	}
	r.log.Info("Release sent", "phase", name, "activation_id", res.activationID, "cursor_position", release)
	return res, nil
}

func (r *captureRun) logResult(name string, res phaseResult) {
	r.log.Info("RESULT", "phase", name, "activated", res.activated, "activation_id", res.activationID,
		"ei_pointer_motion", res.events.motion, "ei_button", res.events.button, "ei_key", res.events.key,
		"ei_scroll", res.events.scroll, "ei_frames", res.events.frames,
		"deactivated_during_hold", res.deactivated, "session_closed_by_portal", res.sessionClosed, "ei_connection_ended", res.eiEnded)
}

type counts struct {
	motion, button, key, scroll, frames int64
}

func (c counts) total() int64 {
	return c.motion + c.button + c.key + c.scroll
}

func (c counts) minus(o counts) counts {
	return counts{c.motion - o.motion, c.button - o.button, c.key - o.key, c.scroll - o.scroll, c.frames - o.frames}
}

type counters struct {
	motion, button, key, scroll, frames atomic.Int64
}

func (c *counters) snapshot() counts {
	return counts{c.motion.Load(), c.button.Load(), c.key.Load(), c.scroll.Load(), c.frames.Load()}
}

type receiver struct {
	client *ei.Client
	log    *slog.Logger
	counts counters
	done   chan struct{}

	mu   sync.Mutex
	mask uint64
}

func startReceiver(ctx context.Context, log *slog.Logger, eis *os.File) (*receiver, error) {
	transport, err := ei.UnixTransportFromFile(eis)
	if err != nil {
		return nil, err
	}
	hsCtx, cancel := context.WithTimeout(ctx, handshakeTimeout)
	defer cancel()
	client, err := ei.Connect(hsCtx, transport, ei.Config{Name: "edgesync-wlcapture", Role: ei.RoleReceiver})
	if err != nil {
		return nil, fmt.Errorf("EI handshake: %w", err)
	}
	hs := client.Handshake()
	log.Info("EI handshake done", "eis_handshake_version", hs.EISHandshakeVersion, "connection_version", hs.ConnectionVersion,
		"eis_interface_versions", fmt.Sprintf("%v", hs.EISInterfaceVersions))
	r := &receiver{client: client, log: log, done: make(chan struct{})}
	go r.pump()
	return r, nil
}

func (r *receiver) close() error {
	err := r.client.Close()
	<-r.done
	return err
}

func (r *receiver) logEnd(phase string) {
	r.log.Warn("EI connection ended", "phase", phase, "err", r.client.Err())
}

func (r *receiver) pump() {
	defer close(r.done)
	for in := range r.client.Events() {
		r.handle(in)
	}
}

func (r *receiver) handle(in ei.Inbound) {
	switch ev := in.Event.(type) {
	case ei.SeatNameEvent:
		r.log.Info("EI seat", "name", ev.Name)
	case ei.SeatCapabilityEvent:
		r.mu.Lock()
		r.mask |= ev.Mask
		r.mu.Unlock()
		r.log.Info("EI seat capability", "interface", ev.Interface, "mask", ev.Mask)
	case ei.SeatDoneEvent:
		r.bind(in.Object)
	case ei.SeatDeviceEvent:
		r.log.Info("EI device announced", "device", fmt.Sprintf("%#x", ev.Device), "version", ev.Version)
	case ei.DeviceNameEvent:
		r.log.Info("EI device name", "device", fmt.Sprintf("%#x", in.Object), "name", ev.Name)
	case ei.DeviceInterfaceEvent:
		r.log.Info("EI device interface", "device", fmt.Sprintf("%#x", in.Object), "interface", ev.Name)
	case ei.DeviceResumedEvent:
		r.log.Info("EI device resumed", "device", fmt.Sprintf("%#x", in.Object))
	case ei.DevicePausedEvent:
		r.log.Info("EI device paused", "device", fmt.Sprintf("%#x", in.Object))
	case ei.DeviceStartEmulatingEvent:
		r.log.Info("EI start_emulating", "device", fmt.Sprintf("%#x", in.Object), "serial", ev.Serial, "sequence", ev.Sequence)
	case ei.DeviceStopEmulatingEvent:
		r.log.Info("EI stop_emulating", "device", fmt.Sprintf("%#x", in.Object), "serial", ev.Serial)
	case ei.DeviceFrameEvent:
		r.counts.frames.Add(1)
	case ei.PointerMotionRelativeEvent:
		n := r.counts.motion.Add(1)
		if n <= motionLogFirst || n%motionLogSampling == 0 {
			r.log.Info("EI pointer motion", "n", n, "dx", ev.X, "dy", ev.Y)
		}
	case ei.ButtonEvent:
		r.counts.button.Add(1)
		r.log.Info("EI button", "code", ev.Button, "state", ev.State)
	case ei.KeyboardKeyEvent:
		r.counts.key.Add(1)
		r.log.Info("EI key", "code", ev.Key, "state", ev.State)
	case ei.ScrollEvent:
		r.counts.scroll.Add(1)
		r.log.Info("EI scroll", "x", ev.X, "y", ev.Y)
	case ei.ScrollDiscreteEvent:
		r.counts.scroll.Add(1)
		r.log.Info("EI scroll discrete", "x", ev.X, "y", ev.Y)
	case ei.KeyboardKeymapEvent:
		r.log.Info("EI keymap", "type", ev.Type, "size", ev.Size)
		if err := ev.FD.Close(); err != nil {
			r.log.Error("closing keymap fd", "err", err)
		}
	default:
		r.log.Info("EI event", "interface", in.Interface, "event", fmt.Sprintf("%T", ev))
	}
}

func (r *receiver) bind(seat uint64) {
	r.mu.Lock()
	mask := r.mask
	r.mu.Unlock()
	if err := r.client.Bind(seat, mask); err != nil {
		r.log.Error("EI seat bind failed", "err", err)
		return
	}
	r.log.Info("EI seat bound", "mask", mask)
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

func writeToken(path, token string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return fmt.Errorf("create token directory: %w", err)
	}
	if err := os.WriteFile(path, []byte(token), 0o600); err != nil {
		return fmt.Errorf("write restore token: %w", err)
	}
	return nil
}
