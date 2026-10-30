package dnd

import (
	"errors"
	"fmt"
)

var (
	ErrNoTypeListPending = errors.New("dnd: no XdndTypeList was requested")
	ErrNoDataPending     = errors.New("dnd: no selection data was requested")
)

type TargetEvent uint8

const (
	TargetEventNone TargetEvent = iota
	TargetEventEntered
	TargetEventUnsupported
	TargetEventLeft
	TargetEventRejected
	TargetEventDropped
)

func (e TargetEvent) String() string {
	switch e {
	case TargetEventNone:
		return "none"
	case TargetEventEntered:
		return "entered"
	case TargetEventUnsupported:
		return "unsupported"
	case TargetEventLeft:
		return "left"
	case TargetEventRejected:
		return "rejected"
	case TargetEventDropped:
		return "dropped"
	}
	return fmt.Sprintf("TargetEvent(%d)", uint8(e))
}

// Convert asks the driver to convert XdndSelection to Type with the given timestamp.
type Convert struct {
	Type uint32
	Time uint32
}

// TargetStep is what the driver must do after feeding one input to a TargetState. Status and Finished are sent to the
// source window; Ignored marks a message that belongs to no active drag (a late Position after Leave is normal).
type TargetStep struct {
	Event        TargetEvent
	LoadTypeList uint32
	Status       *StatusMsg
	Finished     *FinishedMsg
	Convert      *Convert
	Dropped      *DropResult
	Ignored      bool
}

type targetPhase uint8

const (
	targetIdle targetPhase = iota
	targetAwaitingTypes
	targetEntered
	targetAwaitingData
)

// TargetState is the XDND drop target side for text/uri-list drops that are always answered with the copy action.
// It performs no X requests: the driver feeds it decoded messages and executes the returned steps.
type TargetState struct {
	uriList    uint32
	copyAction uint32
	accept     func(x, y int) bool

	phase    targetPhase
	source   uint32
	window   uint32
	version  uint8
	offers   bool
	accepted bool
}

// NewTargetState creates a target that accepts drops of uriList. accept decides per root position whether a drop
// would be taken there; nil accepts everywhere.
func NewTargetState(uriList, copyAction uint32, accept func(x, y int) bool) *TargetState {
	return &TargetState{uriList: uriList, copyAction: copyAction, accept: accept}
}

func (t *TargetState) Active() bool {
	return t.phase != targetIdle
}

// Enter starts a drag over window. A new Enter replaces any drag in progress.
func (t *TargetState) Enter(window uint32, e EnterMsg) TargetStep {
	t.phase = targetEntered
	t.source = e.Source
	t.window = window
	t.version = e.Version
	t.offers = false
	t.accepted = false
	if e.TypeListRequired {
		t.phase = targetAwaitingTypes
		return TargetStep{LoadTypeList: e.Source}
	}
	return t.settle(e.Types)
}

// TypeList supplies the full type list read from the XdndTypeList property of the source after Enter asked for it.
func (t *TargetState) TypeList(types []uint32) (TargetStep, error) {
	if t.phase != targetAwaitingTypes {
		return TargetStep{}, ErrNoTypeListPending
	}
	t.phase = targetEntered
	return t.settle(types), nil
}

func (t *TargetState) settle(types []uint32) TargetStep {
	for _, atom := range types {
		if atom == t.uriList {
			t.offers = true
		}
	}
	if t.offers {
		return TargetStep{Event: TargetEventEntered}
	}
	return TargetStep{Event: TargetEventUnsupported}
}

func (t *TargetState) Position(p PositionMsg) TargetStep {
	if t.phase != targetEntered || p.Source != t.source {
		return TargetStep{Ignored: true}
	}
	t.accepted = t.offers && (t.accept == nil || t.accept(p.X, p.Y))
	status := StatusMsg{Target: t.window, Accept: t.accepted}
	if t.accepted {
		status.Action = t.copyAction
	}
	return TargetStep{Status: &status}
}

func (t *TargetState) Leave(l LeaveMsg) TargetStep {
	if !t.Active() || l.Source != t.source {
		return TargetStep{Ignored: true}
	}
	t.phase = targetIdle
	return TargetStep{Event: TargetEventLeft}
}

// Drop answers XdndDrop: a drop the last status did not accept is declined at once, otherwise the driver must
// convert the selection and report the data with Data or DataFailed.
func (t *TargetState) Drop(d DropMsg) TargetStep {
	if t.phase != targetEntered || d.Source != t.source {
		return TargetStep{Ignored: true}
	}
	if !t.offers || !t.accepted {
		t.phase = targetIdle
		return TargetStep{Event: TargetEventRejected, Finished: t.finished(false)}
	}
	t.phase = targetAwaitingData
	convert := Convert{Type: t.uriList}
	if t.version >= 1 {
		convert.Time = d.Time
	}
	return TargetStep{Convert: &convert}
}

// Data completes a drop with the converted text/uri-list. A list that cannot be parsed, or whose entries were all
// rejected, finishes the drop as not accepted; the parse error is returned together with the step to send.
func (t *TargetState) Data(list []byte) (TargetStep, error) {
	if t.phase != targetAwaitingData {
		return TargetStep{}, ErrNoDataPending
	}
	t.phase = targetIdle
	result, err := ResolveURIList(list)
	if err != nil {
		return TargetStep{Event: TargetEventRejected, Finished: t.finished(false)}, err
	}
	if len(result.Paths) == 0 {
		return TargetStep{Event: TargetEventRejected, Finished: t.finished(false), Dropped: &result}, nil
	}
	return TargetStep{Event: TargetEventDropped, Finished: t.finished(true), Dropped: &result}, nil
}

// DataFailed completes a drop whose selection conversion was refused or timed out.
func (t *TargetState) DataFailed() (TargetStep, error) {
	if t.phase != targetAwaitingData {
		return TargetStep{}, ErrNoDataPending
	}
	t.phase = targetIdle
	return TargetStep{Event: TargetEventRejected, Finished: t.finished(false)}, nil
}

// finished is nil below version 2, where the target sends no XdndFinished.
func (t *TargetState) finished(accepted bool) *FinishedMsg {
	if t.version < 2 {
		return nil
	}
	msg := FinishedMsg{Target: t.window, Accepted: accepted}
	if accepted {
		msg.Action = t.copyAction
	}
	return &msg
}
