package dnd

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jezek/xgb/xproto"
)

// DefaultSelectionTimeout bounds the wait for the source to convert XdndSelection after a drop.
const DefaultSelectionTimeout = 10 * time.Second

type TargetOptions struct {
	X, Y          int16
	Width, Height uint16
	Title         string
}

// TargetReport is one observable step of a drag over the target. Err is set when a drop failed after it was
// accepted (refused or timed-out conversion, INCR, an unparsable list); Dropped is set for TargetEventDropped and for
// a rejected drop whose entries were all refused by the path rules.
type TargetReport struct {
	Event   TargetEvent
	Types   []uint32
	Dropped *DropResult
	Err     error
}

// Target receives XDND drops of text/uri-list. It either owns a visible window (NewTarget) or advertises a window of
// another client through an XdndProxy window of its own (NewProxyTarget), the mechanism a Gio window needs because Gio
// never sets XdndAware.
type Target struct {
	d          *Display
	advertised xproto.Window
	owned      xproto.Window
	proxied    bool
	state      *TargetState

	source      uint32
	types       []uint32
	waitingData bool
}

func (d *Display) NewTarget(opts TargetOptions) (*Target, error) {
	window, err := d.mapVisible(opts.X, opts.Y, opts.Width, opts.Height, opts.Title, 0)
	if err != nil {
		return nil, err
	}
	t := d.newTarget(window, window, false)
	if err := d.setWords(window, d.atoms.Aware, xproto.AtomAtom, []uint32{ProtocolVersion}); err != nil {
		return nil, errors.Join(err, d.destroy(window))
	}
	return t, nil
}

// NewProxyTarget makes foreign accept drops: XdndAware and XdndProxy are set on foreign and on a new window of this
// connection, which then receives the messages the sources address to foreign.
func (d *Display) NewProxyTarget(foreign xproto.Window) (*Target, error) {
	proxy, err := d.newWindow(xproto.WindowClassInputOnly, 0, 0, 1, 1, 0, nil)
	if err != nil {
		return nil, err
	}
	t := d.newTarget(foreign, proxy, true)
	version := []uint32{ProtocolVersion}
	self := []uint32{uint32(proxy)}
	steps := []func() error{
		func() error { return d.setWords(proxy, d.atoms.Aware, xproto.AtomAtom, version) },
		func() error { return d.setWords(proxy, d.atoms.Proxy, xproto.AtomWindow, self) },
		func() error { return d.setWords(foreign, d.atoms.Aware, xproto.AtomAtom, version) },
		func() error { return d.setWords(foreign, d.atoms.Proxy, xproto.AtomWindow, self) },
	}
	for _, step := range steps {
		if err := step(); err != nil {
			return nil, errors.Join(err, t.Close())
		}
	}
	return t, nil
}

func (d *Display) newTarget(advertised, owned xproto.Window, proxied bool) *Target {
	return &Target{
		d:          d,
		advertised: advertised,
		owned:      owned,
		proxied:    proxied,
		state:      NewTargetState(d.atoms.URIList, d.atoms.ActionCopy, nil),
	}
}

func (t *Target) Window() xproto.Window {
	return t.advertised
}

// Close removes what the target set up: the properties on a proxied window and the window of this connection.
func (t *Target) Close() error {
	var errs []error
	if t.proxied {
		for _, property := range []uint32{t.d.atoms.Aware, t.d.atoms.Proxy} {
			if err := xproto.DeletePropertyChecked(t.d.x, t.advertised, xproto.Atom(property)).Check(); err != nil && !isVanished(err) {
				errs = append(errs, fmt.Errorf("dnd: remove property %d from window %d: %w", property, t.advertised, err))
			}
		}
	}
	errs = append(errs, t.d.destroy(t.owned))
	return errors.Join(errs...)
}

// Run handles XDND messages until ctx is done or the connection fails; every step is passed to report.
func (t *Target) Run(ctx context.Context, report func(TargetReport)) error {
	var (
		timer   *time.Timer
		timeout <-chan time.Time
	)
	stopTimer := func() {
		if timer != nil {
			timer.Stop()
			timer, timeout = nil, nil
		}
	}
	defer stopTimer()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timeout:
			stopTimer()
			if err := t.failData(ErrSelectionTimeout, report); err != nil {
				return err
			}
		case de, open := <-t.d.events:
			ev, err := unwrap(de, open)
			if err != nil {
				return err
			}
			if err := t.handle(ev, report); err != nil {
				return err
			}
		}
		if t.waitingData && timer == nil {
			timer = time.NewTimer(DefaultSelectionTimeout)
			timeout = timer.C
		}
		if !t.waitingData {
			stopTimer()
		}
	}
}

func (t *Target) handle(ev any, report func(TargetReport)) error {
	switch e := ev.(type) {
	case xproto.ClientMessageEvent:
		return t.handleMessage(e, report)
	case xproto.SelectionNotifyEvent:
		if t.waitingData && e.Requestor == t.owned && uint32(e.Selection) == t.d.atoms.Selection {
			return t.receiveData(e, report)
		}
	}
	return nil
}

func (t *Target) handleMessage(ev xproto.ClientMessageEvent, report func(TargetReport)) error {
	kind, ok := t.d.atoms.Kind(uint32(ev.Type))
	if !ok {
		return nil
	}
	data, err := messageData(ev)
	if err != nil {
		report(TargetReport{Err: err})
		return nil
	}
	var step TargetStep
	if kind == KindEnter || kind == KindLeave {
		t.waitingData = false
	}
	switch kind {
	case KindEnter:
		enter, err := DecodeEnter(data)
		if err != nil {
			report(TargetReport{Err: err})
			return nil
		}
		t.source = enter.Source
		t.types = enter.Types
		step = t.state.Enter(uint32(ev.Window), enter)
	case KindPosition:
		step = t.state.Position(DecodePosition(data))
	case KindLeave:
		step = t.state.Leave(DecodeLeave(data))
	case KindDrop:
		step = t.state.Drop(DecodeDrop(data))
	default:
		return nil
	}
	return t.execute(step, report)
}

func (t *Target) execute(step TargetStep, report func(TargetReport)) error {
	if step.LoadTypeList != 0 {
		types, err := t.d.words(xproto.Window(step.LoadTypeList), t.d.atoms.TypeList, xproto.AtomAtom, typeListWords)
		if err != nil {
			return err
		}
		next, err := t.state.TypeList(types)
		if err != nil {
			return err
		}
		t.types = types
		return t.execute(next, report)
	}
	if step.Status != nil {
		if err := t.reply(KindStatus, step.Status.Encode); err != nil {
			return err
		}
	}
	if step.Convert != nil {
		if err := t.convert(*step.Convert); err != nil {
			return err
		}
	}
	if step.Finished != nil {
		if err := t.reply(KindFinished, func() (Data, error) { return step.Finished.Encode(), nil }); err != nil {
			return err
		}
	}
	if step.Event != TargetEventNone {
		report(TargetReport{Event: step.Event, Types: t.types, Dropped: step.Dropped})
	}
	return nil
}

func (t *Target) reply(kind Kind, encode func() (Data, error)) error {
	data, err := encode()
	if err != nil {
		return err
	}
	return t.d.send(Outbound{Destination: t.source, Window: t.source, Kind: kind, Data: data})
}

func (t *Target) convert(c Convert) error {
	selection := xproto.Atom(t.d.atoms.Selection)
	if err := xproto.ConvertSelectionChecked(t.d.x, t.owned, selection, xproto.Atom(c.Type), selection, xproto.Timestamp(c.Time)).Check(); err != nil {
		return fmt.Errorf("dnd: convert XdndSelection: %w", err)
	}
	t.waitingData = true
	return nil
}

func (t *Target) receiveData(ev xproto.SelectionNotifyEvent, report func(TargetReport)) error {
	t.waitingData = false
	if ev.Property == 0 {
		return t.failDataWith(ErrSelectionRefused, report)
	}
	reply, err := xproto.GetProperty(t.d.x, true, t.owned, ev.Property, xproto.AtomAny, 0, uint32(t.d.maxRequest/4)).Reply()
	if err != nil {
		return t.failDataWith(fmt.Errorf("dnd: read the dropped data: %w", err), report)
	}
	switch {
	case uint32(reply.Type) == t.d.incr:
		return t.failDataWith(ErrINCRNotSupported, report)
	case reply.BytesAfter != 0:
		return t.failDataWith(fmt.Errorf("%w: %d bytes after", ErrTruncated, reply.BytesAfter), report)
	}
	step, dataErr := t.state.Data(reply.Value)
	if errors.Is(dataErr, ErrNoDataPending) {
		return dataErr
	}
	return t.execute(step, func(r TargetReport) {
		r.Err = dataErr
		report(r)
	})
}

func (t *Target) failData(cause error, report func(TargetReport)) error {
	t.waitingData = false
	return t.failDataWith(cause, report)
}

// failDataWith finishes the drop as not accepted and reports cause; only a failure to tell the source ends Run.
func (t *Target) failDataWith(cause error, report func(TargetReport)) error {
	step, err := t.state.DataFailed()
	if err != nil {
		return err
	}
	return t.execute(step, func(r TargetReport) {
		r.Err = cause
		report(r)
	})
}
