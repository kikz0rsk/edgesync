//go:build linux

// Command wlinject exercises input injection on GNOME or KDE Wayland through the RemoteDesktop portal and an EI
// sender connection: it moves the pointer around a square with relative motion, clicks the left button and types
// text with evdev keycodes, persisting the restore token between runs.
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
	"syscall"
	"time"

	"golang.org/x/sys/unix"

	"github.com/kikz0r/edgesync/spikes/linux/ei"
	"github.com/kikz0r/edgesync/spikes/linux/portal"
)

const (
	handshakeTimeout = 10 * time.Second
	readyTimeout     = 15 * time.Second
	shutdownTimeout  = 5 * time.Second
	motionInterval   = 8 * time.Millisecond
	keyInterval      = 25 * time.Millisecond
	clickHold        = 60 * time.Millisecond
	btnLeft          = 0x110
)

var keyRows = []struct {
	letters string
	first   uint32
}{
	{"qwertyuiop", 16},
	{"asdfghjkl", 30},
	{"zxcvbnm", 44},
}

type config struct {
	logPath   string
	tokenPath string
	square    int
	step      int
	delay     time.Duration
	text      string
}

func parseFlags(args []string) (config, error) {
	var cfg config
	fs := flag.NewFlagSet("wlinject", flag.ContinueOnError)
	fs.StringVar(&cfg.logPath, "log", "out/wlinject.log", "log file (appended; the same lines go to stdout)")
	fs.StringVar(&cfg.tokenPath, "token-file", "out/rd-token", "file holding the RemoteDesktop restore token")
	fs.IntVar(&cfg.square, "square", 200, "side of the pointer square in pixels")
	fs.IntVar(&cfg.step, "step", 5, "pixels per relative motion event")
	fs.DurationVar(&cfg.delay, "delay", 5*time.Second, "pause before injecting, to focus a text editor")
	fs.StringVar(&cfg.text, "text", "edgesync", "lowercase letters to type")
	if err := fs.Parse(args); err != nil {
		return config{}, err
	}
	if cfg.square <= 0 || cfg.step <= 0 || cfg.square%cfg.step != 0 {
		return config{}, errors.New("-square must be a positive multiple of a positive -step")
	}
	if cfg.delay < 0 {
		return config{}, errors.New("-delay must not be negative")
	}
	return cfg, nil
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "wlinject:", err)
		os.Exit(1)
	}
}

func run(args []string) (err error) {
	cfg, err := parseFlags(args)
	if err != nil {
		return err
	}
	keycodes, err := keycodesFor(cfg.text)
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

	rd := portal.NewRemoteDesktop(portal.NewCaller(bus, portal.RandomTokens("edgesync")))
	version, err := rd.Version(ctx)
	if err != nil {
		return err
	}
	types, err := rd.AvailableDeviceTypes(ctx)
	if err != nil {
		return err
	}
	log.Info("RemoteDesktop", "version", version, "available_device_types", types)

	persist := portal.PersistNone
	if version >= 2 {
		persist = portal.PersistUntilRevoked
	} else {
		log.Warn("RemoteDesktop version 1: no persist_mode, no restore token and no ConnectToEIS")
	}
	token, err := readToken(cfg.tokenPath)
	if err != nil {
		return err
	}
	log.Info("restore token", "present", token != "", "file", cfg.tokenPath)

	session, err := rd.Open(ctx, portal.RemoteParams{
		Types:        portal.DeviceKeyboard | portal.DevicePointer,
		PersistMode:  persist,
		RestoreToken: token,
	})
	if err != nil {
		return err
	}
	defer func() {
		closeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), shutdownTimeout)
		defer cancel()
		err = errors.Join(err, session.Close(closeCtx))
	}()
	log.Info("session started", "path", session.Path(), "devices", session.Devices,
		"clipboard_enabled", session.ClipboardEnabled, "restore_token_returned", session.RestoreToken != "")
	if session.RestoreToken != "" {
		if err := writeToken(cfg.tokenPath, session.RestoreToken); err != nil {
			return err
		}
		log.Info("restore token saved", "file", cfg.tokenPath)
	}

	events, err := session.Events(ctx)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, events.Close()) }()

	eisFile, err := session.ConnectToEIS(ctx)
	if err != nil {
		return err
	}
	log.Info("ConnectToEIS returned a file descriptor")
	inj, err := startInjector(ctx, log, eisFile)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, inj.close()) }()

	if err := inj.waitReady(ctx); err != nil {
		return err
	}
	log.Info("ACTION: focus a text editor now", "injecting_in", cfg.delay)
	if err := sleep(ctx, cfg.delay); err != nil {
		return err
	}
	if err := inj.drawSquare(ctx, cfg); err != nil {
		return err
	}
	if err := inj.click(ctx); err != nil {
		return err
	}
	if err := inj.typeKeys(ctx, keycodes); err != nil {
		return err
	}
	if err := inj.client.Sync(ctx); err != nil {
		return fmt.Errorf("EI sync after injection: %w", err)
	}
	log.Info("SUMMARY", "square_px", cfg.square, "typed", cfg.text, "session_closed_by_portal", sessionEnded(events))
	return nil
}

func sessionEnded(events *portal.EventStream) bool {
	select {
	case ev, ok := <-events.Events():
		_, closed := ev.(portal.ClosedEvent)
		return ok && closed
	default:
		return false
	}
}

func keycodesFor(text string) ([]uint32, error) {
	if text == "" {
		return nil, errors.New("-text must not be empty")
	}
	codes := make([]uint32, 0, len(text))
	for _, r := range text {
		code, ok := keycode(r)
		if !ok {
			return nil, fmt.Errorf("-text: %q has no evdev keycode in this spike (lowercase letters only)", r)
		}
		codes = append(codes, code)
	}
	return codes, nil
}

func keycode(r rune) (uint32, bool) {
	for _, row := range keyRows {
		if i := strings.IndexRune(row.letters, r); i >= 0 {
			return row.first + uint32(i), true
		}
	}
	return 0, false
}

type device struct {
	id      uint64
	name    string
	objects map[ei.Interface]uint64
	done    bool
	resumed bool
}

type injector struct {
	client *ei.Client
	log    *slog.Logger
	done   chan struct{}
	ready  chan struct{}

	mu        sync.Mutex
	mask      uint64
	devices   map[uint64]*device
	readyOnce sync.Once
}

func startInjector(ctx context.Context, log *slog.Logger, eis *os.File) (*injector, error) {
	transport, err := ei.UnixTransportFromFile(eis)
	if err != nil {
		return nil, err
	}
	hsCtx, cancel := context.WithTimeout(ctx, handshakeTimeout)
	defer cancel()
	client, err := ei.Connect(hsCtx, transport, ei.Config{Name: "edgesync-wlinject", Role: ei.RoleSender})
	if err != nil {
		return nil, fmt.Errorf("EI handshake: %w", err)
	}
	hs := client.Handshake()
	log.Info("EI handshake done", "eis_handshake_version", hs.EISHandshakeVersion, "connection_version", hs.ConnectionVersion,
		"eis_interface_versions", fmt.Sprintf("%v", hs.EISInterfaceVersions))
	inj := &injector{
		client:  client,
		log:     log,
		done:    make(chan struct{}),
		ready:   make(chan struct{}),
		devices: map[uint64]*device{},
	}
	go inj.pump()
	return inj, nil
}

func (i *injector) close() error {
	err := i.client.Close()
	<-i.done
	return err
}

func (i *injector) pump() {
	defer close(i.done)
	for in := range i.client.Events() {
		i.handle(in)
	}
}

func (i *injector) deviceFor(id uint64) *device {
	d, ok := i.devices[id]
	if !ok {
		d = &device{id: id, objects: map[ei.Interface]uint64{}}
		i.devices[id] = d
	}
	return d
}

func (i *injector) handle(in ei.Inbound) {
	i.mu.Lock()
	defer i.mu.Unlock()
	switch ev := in.Event.(type) {
	case ei.SeatNameEvent:
		i.log.Info("EI seat", "name", ev.Name)
	case ei.SeatCapabilityEvent:
		i.mask |= ev.Mask
		i.log.Info("EI seat capability", "interface", ev.Interface, "mask", ev.Mask)
	case ei.SeatDoneEvent:
		if err := i.client.Bind(in.Object, i.mask); err != nil {
			i.log.Error("EI seat bind failed", "err", err)
			return
		}
		i.log.Info("EI seat bound", "mask", i.mask)
	case ei.SeatDeviceEvent:
		i.deviceFor(ev.Device)
		i.log.Info("EI device announced", "device", fmt.Sprintf("%#x", ev.Device), "version", ev.Version)
	case ei.DeviceNameEvent:
		i.deviceFor(in.Object).name = ev.Name
		i.log.Info("EI device name", "device", fmt.Sprintf("%#x", in.Object), "name", ev.Name)
	case ei.DeviceInterfaceEvent:
		if iface, known := ei.InterfaceByName(ev.Name); known {
			i.deviceFor(in.Object).objects[iface] = ev.Object
		}
		i.log.Info("EI device interface", "device", fmt.Sprintf("%#x", in.Object), "interface", ev.Name)
	case ei.DeviceDoneEvent:
		i.deviceFor(in.Object).done = true
		i.log.Info("EI device done", "device", fmt.Sprintf("%#x", in.Object))
	case ei.DeviceResumedEvent:
		i.deviceFor(in.Object).resumed = true
		i.log.Info("EI device resumed", "device", fmt.Sprintf("%#x", in.Object), "serial", ev.Serial)
		i.signalReadyLocked()
	case ei.DevicePausedEvent:
		i.deviceFor(in.Object).resumed = false
		i.log.Info("EI device paused", "device", fmt.Sprintf("%#x", in.Object), "serial", ev.Serial)
	case ei.KeyboardKeymapEvent:
		i.log.Info("EI keymap", "type", ev.Type, "size", ev.Size)
		if err := ev.FD.Close(); err != nil {
			i.log.Error("closing keymap fd", "err", err)
		}
	default:
		i.log.Info("EI event", "interface", in.Interface, "event", fmt.Sprintf("%T", ev))
	}
}

func (i *injector) signalReadyLocked() {
	_, _, pointerOK := i.findLocked(ei.InterfacePointer, ei.InterfaceButton)
	_, _, keyboardOK := i.findLocked(ei.InterfaceKeyboard)
	if pointerOK && keyboardOK {
		i.readyOnce.Do(func() { close(i.ready) })
	}
}

func (i *injector) findLocked(ifaces ...ei.Interface) (*device, map[ei.Interface]uint64, bool) {
	for _, d := range i.devices {
		if !d.resumed || !d.done {
			continue
		}
		all := true
		for _, iface := range ifaces {
			if _, has := d.objects[iface]; !has {
				all = false
				break
			}
		}
		if all {
			return d, d.objects, true
		}
	}
	return nil, nil, false
}

func (i *injector) waitReady(ctx context.Context) error {
	timeout := time.NewTimer(readyTimeout)
	defer timeout.Stop()
	select {
	case <-i.ready:
		i.log.Info("EI devices ready: a resumed pointer+button device and a resumed keyboard device exist")
		return nil
	case <-i.done:
		return fmt.Errorf("EI connection ended before the devices were ready: %w", i.client.Err())
	case <-timeout.C:
		return fmt.Errorf("no resumed pointer and keyboard device within %v", readyTimeout)
	case <-ctx.Done():
		return context.Cause(ctx)
	}
}

func (i *injector) target(ifaces ...ei.Interface) (*device, map[ei.Interface]uint64, error) {
	i.mu.Lock()
	defer i.mu.Unlock()
	d, objects, ok := i.findLocked(ifaces...)
	if !ok {
		return nil, nil, fmt.Errorf("no resumed EI device offers %v", ifaces)
	}
	return d, objects, nil
}

func monotonicMicros() (uint64, error) {
	var ts unix.Timespec
	if err := unix.ClockGettime(unix.CLOCK_MONOTONIC, &ts); err != nil {
		return 0, fmt.Errorf("read CLOCK_MONOTONIC: %w", err)
	}
	return uint64(ts.Sec)*1_000_000 + uint64(ts.Nsec)/1_000, nil
}

func (i *injector) emulate(d *device, body func(frame func() error) error) error {
	if err := i.client.StartEmulating(d.id); err != nil {
		return err
	}
	frame := func() error {
		ts, err := monotonicMicros()
		if err != nil {
			return err
		}
		return i.client.SendFrame(d.id, ts)
	}
	bodyErr := body(frame)
	stopErr := i.client.StopEmulating(d.id)
	return errors.Join(bodyErr, stopErr)
}

func (i *injector) drawSquare(ctx context.Context, cfg config) error {
	d, objects, err := i.target(ei.InterfacePointer)
	if err != nil {
		return err
	}
	pointer := objects[ei.InterfacePointer]
	steps := cfg.square / cfg.step
	step := float32(cfg.step)
	legs := []struct{ dx, dy float32 }{{step, 0}, {0, step}, {-step, 0}, {0, -step}}
	i.log.Info("injecting pointer square", "device", d.name, "side_px", cfg.square, "step_px", cfg.step)
	return i.emulate(d, func(frame func() error) error {
		for _, leg := range legs {
			for range steps {
				if err := i.client.MotionRelative(pointer, leg.dx, leg.dy); err != nil {
					return err
				}
				if err := frame(); err != nil {
					return err
				}
				if err := sleep(ctx, motionInterval); err != nil {
					return err
				}
			}
		}
		return nil
	})
}

func (i *injector) click(ctx context.Context) error {
	d, objects, err := i.target(ei.InterfacePointer, ei.InterfaceButton)
	if err != nil {
		return err
	}
	button := objects[ei.InterfaceButton]
	i.log.Info("injecting left click", "device", d.name)
	return i.emulate(d, func(frame func() error) error {
		for _, state := range []ei.ButtonState{ei.ButtonPressed, ei.ButtonReleased} {
			if err := i.client.Button(button, btnLeft, state); err != nil {
				return err
			}
			if err := frame(); err != nil {
				return err
			}
			if err := sleep(ctx, clickHold); err != nil {
				return err
			}
		}
		return nil
	})
}

func (i *injector) typeKeys(ctx context.Context, codes []uint32) error {
	d, objects, err := i.target(ei.InterfaceKeyboard)
	if err != nil {
		return err
	}
	keyboard := objects[ei.InterfaceKeyboard]
	i.log.Info("injecting keys", "device", d.name, "count", len(codes))
	return i.emulate(d, func(frame func() error) error {
		for _, code := range codes {
			for _, state := range []ei.KeyState{ei.KeyPressed, ei.KeyReleased} {
				if err := i.client.Key(keyboard, code, state); err != nil {
					return err
				}
				if err := frame(); err != nil {
					return err
				}
				if err := sleep(ctx, keyInterval); err != nil {
					return err
				}
			}
		}
		return nil
	})
}

func sleep(ctx context.Context, d time.Duration) error {
	if d == 0 {
		return nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return context.Cause(ctx)
	}
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
