package dnd

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jezek/xgb/xproto"
)

var ErrFeedClosed = errors.New("dnd: the pointer feed closed before the drag ended")

const (
	DefaultStatusTimeout = 2 * time.Second
	// DefaultFinishTimeout is long because the target converts the selection and may copy before it answers.
	DefaultFinishTimeout = 30 * time.Second

	dragGrabMask = xproto.EventMaskPointerMotion | xproto.EventMaskButtonPress | xproto.EventMaskButtonRelease
)

type SourceOptions struct {
	X, Y          int16
	Width, Height uint16
	Title         string
}

// Source is a visible window that owns XdndSelection with a text/uri-list and drags it to XDND targets.
type Source struct {
	d      *Display
	window xproto.Window
	list   []byte
	types  []uint32
}

func (d *Display) NewSource(paths []string, opts SourceOptions) (*Source, error) {
	list, err := EncodeURIList(paths)
	if err != nil {
		return nil, err
	}
	if len(list) > d.maxRequest-requestSlackBytes {
		return nil, fmt.Errorf("%w: %d bytes", ErrPayloadTooLarge, len(list))
	}
	window, err := d.mapVisible(opts.X, opts.Y, opts.Width, opts.Height, opts.Title, xproto.EventMaskButtonPress)
	if err != nil {
		return nil, err
	}
	return &Source{d: d, window: window, list: list, types: []uint32{d.atoms.URIList}}, nil
}

func (s *Source) Window() xproto.Window {
	return s.window
}

func (s *Source) Close() error {
	return s.d.destroy(s.window)
}

// WaitPress blocks until button 1 is pressed on the source window and returns the event timestamp.
func (s *Source) WaitPress(ctx context.Context) (uint32, error) {
	for {
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		case de, open := <-s.d.events:
			ev, err := unwrap(de, open)
			if err != nil {
				return 0, err
			}
			if press, ok := ev.(xproto.ButtonPressEvent); ok && press.Event == s.window && press.Detail == 1 {
				return uint32(press.Time), nil
			}
		}
	}
}

// Pointer is one scripted pointer input for Drag: a position, a button release or a cancel.
type Pointer struct {
	X, Y    int
	Time    uint32
	State   uint16
	Release bool
	Cancel  bool
}

// DragOptions configures Drag. With a nil Feed the pointer is grabbed and real pointer events drive the drag
// (button 1 release drops, button 3 press cancels); a non-nil Feed drives it from scripted input without a grab.
type DragOptions struct {
	Feed          <-chan Pointer
	Time          uint32
	StatusTimeout time.Duration
	FinishTimeout time.Duration
}

type DragResult struct {
	Outcome Outcome
	Action  uint32
	Target  uint32
}

func (s *Source) Drag(ctx context.Context, opts DragOptions) (DragResult, error) {
	if err := s.claimSelection(opts.Time); err != nil {
		return DragResult{}, err
	}
	state, err := NewSourceState(uint32(s.window), s.types, s.d.atoms.ActionCopy)
	if err != nil {
		return DragResult{}, err
	}
	if state.NeedsTypeList() {
		if err := s.d.setWords(s.window, s.d.atoms.TypeList, xproto.AtomAtom, s.types); err != nil {
			return DragResult{}, err
		}
	}
	if opts.Feed != nil {
		return s.run(ctx, state, opts)
	}
	if err := s.grabPointer(); err != nil {
		return DragResult{}, err
	}
	result, err := s.run(ctx, state, opts)
	if ungrabErr := xproto.UngrabPointerChecked(s.d.x, xproto.TimeCurrentTime).Check(); ungrabErr != nil {
		err = errors.Join(err, fmt.Errorf("dnd: release pointer grab: %w", ungrabErr))
	}
	return result, err
}

func (s *Source) run(ctx context.Context, state *SourceState, opts DragOptions) (DragResult, error) {
	timers := newWaitTimers(opts.StatusTimeout, opts.FinishTimeout)
	defer timers.stop()

	for {
		var (
			step SourceStep
			err  error
		)
		select {
		case <-ctx.Done():
			cancelled := state.Cancel()
			sendErr := s.sendAll(cancelled.Send)
			return DragResult{Outcome: OutcomeCancelled}, errors.Join(ctx.Err(), sendErr)
		case <-timers.fired():
			step = state.Timeout()
		case p, open := <-opts.Feed:
			if !open {
				return DragResult{}, ErrFeedClosed
			}
			step, err = s.applyPointer(state, p)
		case de, open := <-s.d.events:
			step, err = s.applyEvent(state, de, open, opts.Feed == nil)
		}
		if err != nil {
			return DragResult{}, err
		}
		if err := s.sendAll(step.Send); err != nil {
			return DragResult{}, err
		}
		if step.Outcome != OutcomePending {
			return DragResult{Outcome: step.Outcome, Action: step.Action, Target: state.Target().ID}, nil
		}
		timers.follow(state.Waiting())
	}
}

func (s *Source) applyPointer(state *SourceState, p Pointer) (SourceStep, error) {
	switch {
	case p.Cancel:
		return state.Cancel(), nil
	case p.Release:
		return state.Release(p.Time)
	}
	target, err := s.d.targetAt(p.X, p.Y, s.window)
	if err != nil {
		return SourceStep{}, err
	}
	return state.Move(Motion{X: p.X, Y: p.Y, Time: p.Time, State: p.State, Target: target})
}

func (s *Source) applyEvent(state *SourceState, de displayEvent, open, realPointer bool) (SourceStep, error) {
	ev, err := unwrap(de, open)
	if err != nil {
		return SourceStep{}, err
	}
	switch e := ev.(type) {
	case xproto.MotionNotifyEvent:
		if realPointer {
			return s.applyPointer(state, Pointer{X: int(e.RootX), Y: int(e.RootY), Time: uint32(e.Time), State: e.State})
		}
	case xproto.ButtonReleaseEvent:
		if realPointer && e.Detail == 1 {
			return state.Release(uint32(e.Time))
		}
	case xproto.ButtonPressEvent:
		if realPointer && e.Detail == 3 {
			return state.Cancel(), nil
		}
	case xproto.SelectionRequestEvent:
		return SourceStep{}, s.serve(e)
	case xproto.ClientMessageEvent:
		return s.applyMessage(state, e)
	}
	return SourceStep{}, nil
}

func (s *Source) applyMessage(state *SourceState, ev xproto.ClientMessageEvent) (SourceStep, error) {
	kind, ok := s.d.atoms.Kind(uint32(ev.Type))
	if !ok {
		return SourceStep{}, nil
	}
	data, err := messageData(ev)
	if err != nil {
		return SourceStep{}, err
	}
	switch kind {
	case KindStatus:
		return state.Status(DecodeStatus(data))
	case KindFinished:
		return state.Finished(DecodeFinished(data)), nil
	}
	return SourceStep{}, nil
}

func (s *Source) sendAll(messages []Outbound) error {
	for _, m := range messages {
		if err := s.d.send(m); err != nil {
			return err
		}
	}
	return nil
}

func (s *Source) claimSelection(timestamp uint32) error {
	d := s.d
	selection := xproto.Atom(d.atoms.Selection)
	if err := xproto.SetSelectionOwnerChecked(d.x, s.window, selection, xproto.Timestamp(timestamp)).Check(); err != nil {
		return fmt.Errorf("dnd: claim XdndSelection: %w", err)
	}
	owner, err := xproto.GetSelectionOwner(d.x, selection).Reply()
	if err != nil {
		return fmt.Errorf("dnd: read owner of XdndSelection: %w", err)
	}
	if owner.Owner != s.window {
		return errors.New("dnd: claim of XdndSelection did not take effect")
	}
	return nil
}

func (s *Source) grabPointer() error {
	reply, err := xproto.GrabPointer(s.d.x, false, s.window, dragGrabMask, xproto.GrabModeAsync, xproto.GrabModeAsync, 0, 0, xproto.TimeCurrentTime).Reply()
	if err != nil {
		return fmt.Errorf("dnd: grab pointer: %w", err)
	}
	if reply.Status != xproto.GrabStatusSuccess {
		return fmt.Errorf("%w: status %d", ErrGrabRefused, reply.Status)
	}
	return nil
}

// serve answers a SelectionRequest for XdndSelection: TARGETS and the text/uri-list. The list always fits one request
// (NewSource checks), so INCR is never needed on this side.
func (s *Source) serve(req xproto.SelectionRequestEvent) error {
	d := s.d
	property := req.Property
	if property == 0 {
		property = req.Target
	}
	granted := xproto.Atom(0)
	switch {
	case uint32(req.Selection) != d.atoms.Selection:
	case uint32(req.Target) == d.atoms.Targets:
		if err := d.setWords(req.Requestor, uint32(property), uint32(xproto.AtomAtom), []uint32{d.atoms.Targets, d.atoms.URIList}); err != nil {
			return err
		}
		granted = property
	case uint32(req.Target) == d.atoms.URIList:
		if err := xproto.ChangePropertyChecked(d.x, xproto.PropModeReplace, req.Requestor, property, req.Target, 8, uint32(len(s.list)), s.list).Check(); err != nil {
			return fmt.Errorf("dnd: write the URI list for requestor %d: %w", req.Requestor, err)
		}
		granted = property
	}
	notify := xproto.SelectionNotifyEvent{Time: req.Time, Requestor: req.Requestor, Selection: req.Selection, Target: req.Target, Property: granted}
	if err := xproto.SendEventChecked(d.x, false, req.Requestor, 0, string(notify.Bytes())).Check(); err != nil {
		return fmt.Errorf("dnd: send SelectionNotify to %d: %w", req.Requestor, err)
	}
	return nil
}

// targetAt walks the window tree under a root position and returns the deepest XdndAware window, resolving a valid
// XdndProxy. The own window is never a target.
func (d *Display) targetAt(x, y int, exclude xproto.Window) (TargetWindow, error) {
	var found TargetWindow
	current := d.root
	for {
		reply, err := xproto.TranslateCoordinates(d.x, d.root, current, int16(x), int16(y)).Reply()
		if err != nil {
			if isVanished(err) {
				return found, nil
			}
			return TargetWindow{}, fmt.Errorf("dnd: translate coordinates: %w", err)
		}
		if reply.Child == 0 {
			return found, nil
		}
		current = reply.Child
		if current == exclude {
			return TargetWindow{}, nil
		}
		candidate, aware, err := d.awareWindow(current)
		if err != nil {
			if isVanished(err) {
				return found, nil
			}
			return TargetWindow{}, err
		}
		if aware {
			found = candidate
		}
	}
}

func (d *Display) awareWindow(window xproto.Window) (TargetWindow, bool, error) {
	proxy, err := d.validProxy(window)
	if err != nil {
		return TargetWindow{}, false, err
	}
	holder := window
	if proxy != 0 {
		holder = xproto.Window(proxy)
	}
	versions, err := d.words(holder, d.atoms.Aware, xproto.AtomAtom, 1)
	if err != nil || len(versions) == 0 {
		return TargetWindow{}, false, err
	}
	return TargetWindow{ID: uint32(window), Version: uint8(min(versions[0], 255)), Proxy: proxy}, true, nil
}

// validProxy returns the XdndProxy window of window, or 0 when there is none or it is stale: a proxy must carry an
// XdndProxy property that names itself, otherwise the property is a leftover of a crash and is ignored.
func (d *Display) validProxy(window xproto.Window) (uint32, error) {
	named, err := d.words(window, d.atoms.Proxy, xproto.AtomWindow, 1)
	if err != nil || len(named) == 0 {
		return 0, err
	}
	proxy := named[0]
	self, err := d.words(xproto.Window(proxy), d.atoms.Proxy, xproto.AtomWindow, 1)
	if err != nil {
		if isVanished(err) {
			return 0, nil
		}
		return 0, err
	}
	if len(self) == 1 && self[0] == proxy {
		return proxy, nil
	}
	return 0, nil
}

// waitTimers times the reply a SourceState is waiting for; it arms one timer per wait and disarms it when the wait ends.
type waitTimers struct {
	status, finish time.Duration
	current        Wait
	timer          *time.Timer
}

func newWaitTimers(status, finish time.Duration) *waitTimers {
	if status <= 0 {
		status = DefaultStatusTimeout
	}
	if finish <= 0 {
		finish = DefaultFinishTimeout
	}
	return &waitTimers{status: status, finish: finish}
}

func (w *waitTimers) fired() <-chan time.Time {
	if w.timer == nil {
		return nil
	}
	return w.timer.C
}

func (w *waitTimers) follow(wait Wait) {
	if wait == w.current {
		return
	}
	w.stop()
	w.current = wait
	switch wait {
	case WaitStatus:
		w.timer = time.NewTimer(w.status)
	case WaitFinished:
		w.timer = time.NewTimer(w.finish)
	}
}

func (w *waitTimers) stop() {
	if w.timer != nil {
		w.timer.Stop()
		w.timer = nil
	}
}
