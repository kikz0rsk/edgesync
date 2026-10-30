package dnd

import "fmt"

// TargetWindow is the window under the pointer that the driver found to be XdndAware. Proxy is the validated
// XdndProxy window, or 0. A zero ID means no aware window.
type TargetWindow struct {
	ID      uint32
	Version uint8
	Proxy   uint32
}

func (w TargetWindow) destination() uint32 {
	if w.Proxy != 0 {
		return w.Proxy
	}
	return w.ID
}

// Motion is one pointer position (root coordinates) with the aware window found under it.
type Motion struct {
	X, Y   int
	Time   uint32
	State  uint16
	Target TargetWindow
}

// Outbound is one ClientMessage the driver must send: to Destination (the proxy when there is one), carrying Window
// (the window under the pointer) in the event's window field.
type Outbound struct {
	Destination uint32
	Window      uint32
	Kind        Kind
	Data        Data
}

type Outcome uint8

const (
	OutcomePending Outcome = iota
	OutcomeDropped
	OutcomeRejected
	OutcomeCancelled
	OutcomeNoTarget
	OutcomeTimedOut
)

func (o Outcome) String() string {
	switch o {
	case OutcomePending:
		return "pending"
	case OutcomeDropped:
		return "dropped"
	case OutcomeRejected:
		return "rejected"
	case OutcomeCancelled:
		return "cancelled"
	case OutcomeNoTarget:
		return "no-target"
	case OutcomeTimedOut:
		return "timed-out"
	}
	return fmt.Sprintf("Outcome(%d)", uint8(o))
}

type Wait uint8

const (
	WaitNone Wait = iota
	WaitStatus
	WaitFinished
)

// SourceStep is what the driver must do after one input. Outcome stays OutcomePending until the drag is over.
type SourceStep struct {
	Send    []Outbound
	Outcome Outcome
	Action  uint32
	Ignored bool
}

type sourcePhase uint8

const (
	sourceIdle sourcePhase = iota
	sourceOver
	sourceDropping
	sourceDone
)

// SourceState is the XDND drag source side. It performs no X requests: the driver feeds it pointer motion (with the
// aware window found under it), XdndStatus and XdndFinished messages and the button release, and sends the messages
// of the returned steps. Positions are never sent while a Status is outstanding and are coalesced to the latest one.
type SourceState struct {
	self   uint32
	types  []uint32
	action uint32

	phase     sourcePhase
	target    TargetWindow
	version   uint8
	awaiting  bool
	haveState bool
	status    StatusMsg
	accepted  bool
	pending   *Motion

	releaseRequested bool
	releaseTime      uint32
}

// NewSourceState creates a source for the window self offering types and requesting action in every Position.
func NewSourceState(self uint32, types []uint32, action uint32) (*SourceState, error) {
	if len(types) == 0 {
		return nil, ErrNoTypes
	}
	return &SourceState{self: self, types: types, action: action}, nil
}

// NeedsTypeList reports whether the drag offers more than three types, in which case the driver must set the
// XdndTypeList property on the source window before the first Move.
func (s *SourceState) NeedsTypeList() bool {
	return len(s.types) > maxInlineTypes
}

// Target is the aware window of the drag in progress or, after the drag ended, the last one it was over.
func (s *SourceState) Target() TargetWindow {
	return s.target
}

// Waiting is the reply the driver is timing: the Status that must arrive before a pending drop, or the Finished.
func (s *SourceState) Waiting() Wait {
	switch {
	case s.phase == sourceOver && s.awaiting && s.releaseRequested:
		return WaitStatus
	case s.phase == sourceDropping:
		return WaitFinished
	}
	return WaitNone
}

func (s *SourceState) Move(m Motion) (SourceStep, error) {
	if s.phase == sourceDropping || s.phase == sourceDone || s.releaseRequested {
		return SourceStep{Ignored: true}, nil
	}
	var step SourceStep
	if m.Target.ID != s.target.ID {
		if s.phase == sourceOver {
			step.Send = append(step.Send, s.leave())
		}
		s.resetTarget()
		if m.Target.ID == 0 {
			return step, nil
		}
		s.target = m.Target
		s.version = NegotiateVersion(ProtocolVersion, m.Target.Version)
		s.phase = sourceOver
		enter, err := EnterMsg{Source: s.self, Version: s.version, Types: s.types}.Encode()
		if err != nil {
			return SourceStep{}, err
		}
		step.Send = append(step.Send, s.outbound(KindEnter, enter))
		return s.appendPosition(step, m)
	}
	if s.phase != sourceOver {
		return step, nil
	}
	if s.awaiting {
		s.pending = &m
		return step, nil
	}
	if s.suppressed(m) {
		return step, nil
	}
	return s.appendPosition(step, m)
}

func (s *SourceState) Status(st StatusMsg) (SourceStep, error) {
	if s.phase != sourceOver || st.Target != s.target.ID || !s.awaiting {
		return SourceStep{Ignored: true}, nil
	}
	s.awaiting = false
	s.haveState = true
	s.status = st
	s.accepted = st.Accept && (s.version < 2 || st.Action != 0)
	if s.releaseRequested {
		return s.resolveRelease(s.releaseTime)
	}
	if s.pending == nil {
		return SourceStep{}, nil
	}
	m := *s.pending
	s.pending = nil
	if s.suppressed(m) {
		return SourceStep{}, nil
	}
	return s.appendPosition(SourceStep{}, m)
}

// Release is the button release that ends the drag with the given timestamp. While a Status is outstanding the drop
// waits for it (see Waiting and Timeout), except when no Status was ever received: the walk-through of the XDND
// specification then says to send XdndLeave without waiting.
func (s *SourceState) Release(t uint32) (SourceStep, error) {
	switch s.phase {
	case sourceIdle:
		s.phase = sourceDone
		return SourceStep{Outcome: OutcomeNoTarget}, nil
	case sourceOver:
		if s.awaiting && s.haveState {
			s.releaseRequested = true
			s.releaseTime = t
			return SourceStep{}, nil
		}
		return s.resolveRelease(t)
	}
	return SourceStep{Ignored: true}, nil
}

// Cancel abandons the drag (Escape or a context cancel). A drop already sent cannot be taken back.
func (s *SourceState) Cancel() SourceStep {
	switch s.phase {
	case sourceIdle:
		s.phase = sourceDone
		return SourceStep{Outcome: OutcomeCancelled}
	case sourceOver:
		step := SourceStep{Send: []Outbound{s.leave()}, Outcome: OutcomeCancelled}
		s.phase = sourceDone
		return step
	}
	return SourceStep{Ignored: true}
}

func (s *SourceState) Finished(f FinishedMsg) SourceStep {
	if s.phase != sourceDropping || f.Target != s.target.ID {
		return SourceStep{Ignored: true}
	}
	s.phase = sourceDone
	if s.version >= ProtocolVersion && !f.Accepted {
		return SourceStep{Outcome: OutcomeRejected}
	}
	return SourceStep{Outcome: OutcomeDropped, Action: f.Action}
}

// Timeout ends the wait reported by Waiting: the target never answered, so the drag is abandoned (a pending drop
// gets an XdndLeave, a sent drop is simply given up on).
func (s *SourceState) Timeout() SourceStep {
	switch s.Waiting() {
	case WaitStatus:
		step := SourceStep{Send: []Outbound{s.leave()}, Outcome: OutcomeTimedOut}
		s.phase = sourceDone
		return step
	case WaitFinished:
		s.phase = sourceDone
		return SourceStep{Outcome: OutcomeTimedOut}
	}
	return SourceStep{Ignored: true}
}

func (s *SourceState) resolveRelease(t uint32) (SourceStep, error) {
	s.releaseRequested = false
	if !s.accepted {
		step := SourceStep{Send: []Outbound{s.leave()}, Outcome: OutcomeRejected}
		s.phase = sourceDone
		return step, nil
	}
	step := SourceStep{Send: []Outbound{s.outbound(KindDrop, DropMsg{Source: s.self, Time: t}.Encode())}}
	if s.version < 2 {
		s.phase = sourceDone
		step.Outcome = OutcomeDropped
		return step, nil
	}
	s.phase = sourceDropping
	return step, nil
}

func (s *SourceState) appendPosition(step SourceStep, m Motion) (SourceStep, error) {
	msg := PositionMsg{Source: s.self, State: m.State, X: m.X, Y: m.Y, Time: m.Time}
	if s.version >= 2 {
		msg.Action = s.action
	}
	data, err := msg.Encode()
	if err != nil {
		return SourceStep{}, err
	}
	step.Send = append(step.Send, s.outbound(KindPosition, data))
	s.awaiting = true
	return step, nil
}

func (s *SourceState) outbound(kind Kind, data Data) Outbound {
	return Outbound{Destination: s.target.destination(), Window: s.target.ID, Kind: kind, Data: data}
}

func (s *SourceState) leave() Outbound {
	return s.outbound(KindLeave, LeaveMsg{Source: s.self}.Encode())
}

func (s *SourceState) suppressed(m Motion) bool {
	return s.haveState && !s.status.WantPositions && !s.status.Rect.Empty() && s.status.Rect.Contains(m.X, m.Y)
}

func (s *SourceState) resetTarget() {
	s.target = TargetWindow{}
	s.version = 0
	s.awaiting = false
	s.haveState = false
	s.status = StatusMsg{}
	s.accepted = false
	s.pending = nil
	s.phase = sourceIdle
}
