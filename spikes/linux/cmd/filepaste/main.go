//go:build linux

package main

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/godbus/dbus/v5"
	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"

	"github.com/kikz0r/edgesync/spikes/linux/portal"
)

const (
	atomClipboard   = "CLIPBOARD"
	atomTargets     = "TARGETS"
	shutdownTimeout = 5 * time.Second
)

var (
	ErrNotOwner       = errors.New("filepaste: CLIPBOARD owner did not change to this probe")
	ErrConnClosed     = errors.New("filepaste: X connection closed")
	errRefusedByProbe = errors.New("filepaste: refused by -refuse")
)

type probe struct {
	log     *slog.Logger
	cfg     Config
	staging string
	runTag  int64
	start   time.Time
	fails   *failures
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "filepaste:", err)
		os.Exit(1)
	}
}

func run(args []string) (err error) {
	cfg, err := ParseConfig(args)
	if err != nil {
		return err
	}
	log, closeLog, err := openLogger(cfg.LogPath)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, closeLog()) }()

	staging, err := filepath.Abs(cfg.Staging)
	if err != nil {
		return fmt.Errorf("resolve staging directory: %w", err)
	}
	if err := os.MkdirAll(staging, 0o750); err != nil {
		return fmt.Errorf("create staging directory: %w", err)
	}

	sigCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(sigCtx, cfg.Wait)
	defer cancel()

	start := time.Now()
	p := &probe{log: log, cfg: cfg, staging: staging, runTag: start.UnixNano(), start: start, fails: &failures{}}
	p.log.Info("start", "mode", cfg.Mode, "op", cfg.Op, "targets", cfg.Targets, "delay", cfg.Delay, "wait", cfg.Wait,
		"size", cfg.Size, "staging", staging, "refuse", cfg.Refuse)
	switch cfg.Mode {
	case ModeX11:
		err = runX11(ctx, p)
	case ModePortal:
		err = runPortal(ctx, p)
	}
	return errors.Join(err, p.fails.err())
}

func (p *probe) since() time.Duration {
	return time.Since(p.start)
}

func (p *probe) stage(ctx context.Context, serial uint32) (*Staging, error) {
	path := stagingPath(p.staging, p.runTag, serial, p.cfg.Size)
	st, err := StartStaging(ctx, path, p.cfg.Size)
	if err != nil {
		return nil, err
	}
	p.log.Info("staging started", "serial", serial, "path", path, "size", p.cfg.Size)
	return st, nil
}

func (p *probe) record(ex Exchange) {
	p.log.Info("exchange", "target", ex.Target, "outcome", ex.Outcome, "requested", ex.Requested,
		"answered", ex.Answered, "delay", ex.Delay(), "bytes_at_answer", ex.Bytes, "complete_at_answer", ex.Complete)
}

func (p *probe) reportFinished(serial uint32, st *Staging) {
	digest, err := st.Wait()
	if err != nil {
		p.log.Error("staging failed", "serial", serial, "path", st.Path(), "err", err)
		p.fails.add(err)
		return
	}
	p.log.Info("staging finished", "serial", serial, "path", st.Path(), "sha256", digest, "elapsed", p.since())
}

type x11Owner struct {
	*probe
	x          *xgb.Conn
	window     xproto.Window
	clipboard  xproto.Atom
	targets    xproto.Atom
	offered    map[xproto.Atom]string
	targetAtom []xproto.Atom
	serial     uint32
}

func runX11(ctx context.Context, p *probe) error {
	display := os.Getenv("DISPLAY")
	if display == "" {
		return errors.New("DISPLAY is not set")
	}
	x, err := xgb.NewConnDisplay(display)
	if err != nil {
		return fmt.Errorf("connect to X display %q: %w", display, err)
	}
	closeX := sync.OnceFunc(x.Close)
	defer closeX()
	stop := context.AfterFunc(ctx, closeX)
	defer stop()

	o := &x11Owner{probe: p, x: x}
	if err := o.claim(); err != nil {
		return err
	}
	p.log.Info("CLIPBOARD claimed", "window", o.window, "targets", p.cfg.Targets)
	return o.serve(ctx)
}

func (o *x11Owner) claim() error {
	setup := xproto.Setup(o.x)
	if len(setup.Roots) == 0 {
		return errors.New("X server reports no screens")
	}
	clipboard, err := o.atom(atomClipboard)
	if err != nil {
		return err
	}
	o.clipboard = clipboard
	if o.targets, err = o.atom(atomTargets); err != nil {
		return err
	}
	o.offered = make(map[xproto.Atom]string, len(o.cfg.Targets))
	o.targetAtom = []xproto.Atom{o.targets}
	for _, name := range o.cfg.Targets {
		atom, err := o.atom(name)
		if err != nil {
			return err
		}
		o.offered[atom] = name
		o.targetAtom = append(o.targetAtom, atom)
	}

	id, err := o.x.NewId()
	if err != nil {
		return fmt.Errorf("allocate window id: %w", err)
	}
	o.window = xproto.Window(id)
	root := setup.Roots[0].Root
	if err := xproto.CreateWindowChecked(o.x, 0, o.window, root, 0, 0, 1, 1, 0, xproto.WindowClassInputOutput, 0, 0, nil).Check(); err != nil {
		return fmt.Errorf("create owner window: %w", err)
	}
	if err := xproto.SetSelectionOwnerChecked(o.x, o.window, o.clipboard, xproto.TimeCurrentTime).Check(); err != nil {
		return fmt.Errorf("claim CLIPBOARD: %w", err)
	}
	owner, err := xproto.GetSelectionOwner(o.x, o.clipboard).Reply()
	if err != nil {
		return fmt.Errorf("read CLIPBOARD owner: %w", err)
	}
	if owner.Owner != o.window {
		return ErrNotOwner
	}
	return nil
}

func (o *x11Owner) atom(name string) (xproto.Atom, error) {
	reply, err := xproto.InternAtom(o.x, false, uint16(len(name)), name).Reply()
	if err != nil {
		return 0, fmt.Errorf("intern atom %s: %w", name, err)
	}
	return reply.Atom, nil
}

func (o *x11Owner) atomName(atom xproto.Atom) (string, error) {
	reply, err := xproto.GetAtomName(o.x, atom).Reply()
	if err != nil {
		return "", fmt.Errorf("name of atom %d: %w", atom, err)
	}
	return reply.Name, nil
}

// serve answers SelectionRequest events until the context ends, another client takes CLIPBOARD or the connection fails.
// Delayed answers run in goroutines that share the connection; the xgb request path is safe for that. They are waited
// for on return, after workCtx is cancelled.
func (o *x11Owner) serve(ctx context.Context) error {
	workCtx, cancel := context.WithCancel(ctx)
	var wg sync.WaitGroup
	defer func() {
		cancel()
		wg.Wait()
	}()
	for {
		ev, xerr := o.x.WaitForEvent()
		if xerr != nil {
			return fmt.Errorf("X error: %w", xerr)
		}
		if ev == nil {
			if ctx.Err() != nil {
				return nil
			}
			return ErrConnClosed
		}
		switch e := ev.(type) {
		case xproto.SelectionRequestEvent:
			if err := o.request(workCtx, &wg, e); err != nil {
				return err
			}
		case xproto.SelectionClearEvent:
			o.log.Info("CLIPBOARD taken by another client, probe stops serving")
			return nil
		}
	}
}

func (o *x11Owner) request(ctx context.Context, wg *sync.WaitGroup, e xproto.SelectionRequestEvent) error {
	name, err := o.atomName(e.Target)
	if err != nil {
		return err
	}
	property := e.Property
	if property == 0 {
		property = e.Target
	}
	ex := Exchange{Target: name, Requested: o.since()}

	if e.Target == o.targets {
		ex.Outcome = OutcomeAnswered
		ex.Answered = o.since()
		o.record(ex)
		return o.writeProperty(e, property, xproto.AtomAtom, 32, encodeAtoms(o.targetAtom))
	}
	if _, ok := o.offered[e.Target]; !ok {
		ex.Outcome = OutcomeNotOffered
		ex.Answered = o.since()
		o.record(ex)
		return o.notify(e, 0)
	}

	o.serial++
	serial := o.serial
	st, err := o.stage(ctx, serial)
	if err != nil {
		o.fails.add(err)
		ex.Outcome = OutcomeFailed
		ex.Answered = o.since()
		o.record(ex)
		o.log.Error("staging failed, request refused", "serial", serial, "err", err)
		return o.notify(e, 0)
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		o.answerLater(ctx, e, property, name, serial, st, ex)
	}()
	return nil
}

func (o *x11Owner) answerLater(ctx context.Context, e xproto.SelectionRequestEvent, property xproto.Atom, name string, serial uint32, st *Staging, ex Exchange) {
	timer := time.NewTimer(o.cfg.Delay)
	defer timer.Stop()
	select {
	case <-timer.C:
	case <-ctx.Done():
		state := st.Snapshot()
		ex.Outcome = OutcomeCancelled
		ex.Answered = o.since()
		ex.Bytes = state.Written
		o.record(ex)
		return
	}
	state := st.Snapshot()
	ex.Answered = o.since()
	ex.Bytes = state.Written
	ex.Complete = state.Finished && state.Err == nil
	if o.cfg.Refuse {
		ex.Outcome = OutcomeRefused
		o.record(ex)
		o.reportAnswerError(o.notify(e, 0), e.Requestor)
		return
	}
	payload, err := Payload(name, o.cfg.Op, st.Path())
	if err != nil {
		o.fails.add(err)
		ex.Outcome = OutcomeFailed
		o.record(ex)
		o.reportAnswerError(o.notify(e, 0), e.Requestor)
		return
	}
	ex.Outcome = OutcomeAnswered
	o.record(ex)
	o.reportAnswerError(o.writeProperty(e, property, e.Target, 8, payload), e.Requestor)
	o.reportFinished(serial, st)
}

func (o *x11Owner) reportAnswerError(err error, requestor xproto.Window) {
	if err != nil {
		o.fails.add(err)
		o.log.Error("answer failed", "requestor", requestor, "err", err)
	}
}

// writeProperty stores data on the requestor's property and sends SelectionNotify. Format 32 data is an atom list
// in little-endian order; format 8 data is the raw payload, and units are counted in the format's unit size.
func (o *x11Owner) writeProperty(e xproto.SelectionRequestEvent, property, typ xproto.Atom, format byte, data []byte) error {
	units := uint32(len(data) * 8 / int(format))
	if err := xproto.ChangePropertyChecked(o.x, xproto.PropModeReplace, e.Requestor, property, typ, format, units, data).Check(); err != nil {
		return fmt.Errorf("write property for requestor %d: %w", e.Requestor, err)
	}
	return o.notify(e, property)
}

func (o *x11Owner) notify(e xproto.SelectionRequestEvent, property xproto.Atom) error {
	ev := xproto.SelectionNotifyEvent{
		Time:      e.Time,
		Requestor: e.Requestor,
		Selection: e.Selection,
		Target:    e.Target,
		Property:  property,
	}
	if err := xproto.SendEventChecked(o.x, false, e.Requestor, 0, string(ev.Bytes())).Check(); err != nil {
		return fmt.Errorf("send SelectionNotify to %d: %w", e.Requestor, err)
	}
	return nil
}

func encodeAtoms(atoms []xproto.Atom) []byte {
	out := make([]byte, 4*len(atoms))
	for i, atom := range atoms {
		binary.LittleEndian.PutUint32(out[4*i:], uint32(atom))
	}
	return out
}

func runPortal(ctx context.Context, p *probe) (err error) {
	bus, err := portal.ConnectSessionBus()
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, bus.Close()) }()

	caller := portal.NewCaller(bus, portal.RandomTokens("edgesync"))
	session, err := portal.NewRemoteDesktop(caller).OpenWithClipboard(ctx, portal.RemoteParams{
		Types:       portal.DeviceKeyboard | portal.DevicePointer,
		PersistMode: portal.PersistNone,
	})
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, closeSession(ctx, session.Close)) }()
	p.log.Info("session started", "path", session.Path(), "devices", session.Devices, "clipboard_enabled", session.ClipboardEnabled)

	clip := portal.NewClipboard(caller)
	events, err := clip.Events(ctx, session.Path())
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, events.Close()) }()
	if err := clip.SetSelection(ctx, session.Path(), p.cfg.Targets); err != nil {
		return fmt.Errorf("set selection: %w", err)
	}
	p.log.Info("SetSelection", "mime_types", p.cfg.Targets)

	workCtx, cancel := context.WithCancel(ctx)
	var wg sync.WaitGroup
	defer func() {
		cancel()
		wg.Wait()
	}()
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
				p.log.Info("SelectionTransfer", "mime", e.MimeType, "serial", e.Serial)
				st, stageErr := p.stage(workCtx, e.Serial)
				if stageErr != nil {
					p.log.Error("staging failed, transfer refused", "serial", e.Serial, "err", stageErr)
				}
				wg.Add(1)
				go func() {
					defer wg.Done()
					p.serveTransfer(workCtx, clip, session.Path(), e, st, stageErr)
				}()
			case portal.SelectionOwnerChangedEvent:
				p.log.Info("SelectionOwnerChanged", "mime_types", e.MimeTypes, "session_is_owner", e.SessionIsOwner)
			case portal.ClosedEvent:
				p.log.Warn("Clipboard session closed by the portal")
				return nil
			}
		}
	}
}

func (p *probe) serveTransfer(ctx context.Context, clip *portal.Clipboard, session dbus.ObjectPath, ev portal.SelectionTransferEvent, st *Staging, stageErr error) {
	ex := Exchange{Target: ev.MimeType, Requested: p.since()}
	content := func(mime string) ([]byte, error) {
		if stageErr != nil {
			ex.Outcome = OutcomeFailed
			ex.Answered = p.since()
			return nil, stageErr
		}
		state := st.Snapshot()
		ex.Answered = p.since()
		ex.Bytes = state.Written
		ex.Complete = state.Finished && state.Err == nil
		if p.cfg.Refuse {
			ex.Outcome = OutcomeRefused
			return nil, errRefusedByProbe
		}
		data, err := Payload(mime, p.cfg.Op, st.Path())
		if err != nil {
			ex.Outcome = OutcomeFailed
			return nil, err
		}
		ex.Outcome = OutcomeAnswered
		return data, nil
	}
	err := clip.ServeTransfer(ctx, session, ev, p.cfg.Delay, content)
	switch {
	case ctx.Err() != nil && ex.Outcome == "":
		ex.Outcome = OutcomeCancelled
		ex.Answered = p.since()
		p.record(ex)
		return
	case errors.Is(err, errRefusedByProbe):
		p.record(ex)
		return
	case err != nil:
		p.fails.add(err)
		p.log.Error("SelectionTransfer failed", "mime", ev.MimeType, "serial", ev.Serial, "err", err)
		return
	}
	p.record(ex)
	if st != nil {
		p.reportFinished(ev.Serial, st)
	}
}

func closeSession(ctx context.Context, closeFn func(context.Context) error) error {
	closeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), shutdownTimeout)
	defer cancel()
	return closeFn(closeCtx)
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
