package dnd

import (
	"errors"
	"testing"
)

const (
	srcSelf = 700
	srcCopy = 11
	winA    = 800
	winB    = 801
	proxyP  = 900
)

func newSource(t *testing.T, types ...uint32) *SourceState {
	t.Helper()
	if len(types) == 0 {
		types = []uint32{tgtURIList}
	}
	s, err := NewSourceState(srcSelf, types, srcCopy)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func aware(id uint32, version uint8) TargetWindow {
	return TargetWindow{ID: id, Version: version}
}

func move(t *testing.T, s *SourceState, x, y int, w TargetWindow) SourceStep {
	t.Helper()
	step, err := s.Move(Motion{X: x, Y: y, Time: 5, Target: w})
	if err != nil {
		t.Fatal(err)
	}
	return step
}

func status(t *testing.T, s *SourceState, st StatusMsg) SourceStep {
	t.Helper()
	step, err := s.Status(st)
	if err != nil {
		t.Fatal(err)
	}
	return step
}

func kinds(step SourceStep) []Kind {
	out := make([]Kind, len(step.Send))
	for i, m := range step.Send {
		out[i] = m.Kind
	}
	return out
}

func wantKinds(t *testing.T, step SourceStep, want ...Kind) {
	t.Helper()
	got := kinds(step)
	if len(got) != len(want) {
		t.Fatalf("sent %v, want %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("sent %v, want %v", got, want)
		}
	}
}

func accept(target uint32) StatusMsg {
	return StatusMsg{Target: target, Accept: true, Action: srcCopy}
}

func TestSourceState_EnterOnAwareWindow(t *testing.T) {
	s := newSource(t)
	step := move(t, s, 300, 200, aware(winA, 5))
	wantKinds(t, step, KindEnter, KindPosition)
	enterMsg, err := DecodeEnter(step.Send[0].Data)
	if err != nil {
		t.Fatal(err)
	}
	if enterMsg.Source != srcSelf || enterMsg.Version != 5 || len(enterMsg.Types) != 1 || enterMsg.Types[0] != tgtURIList {
		t.Fatalf("enter %+v", enterMsg)
	}
	pos := DecodePosition(step.Send[1].Data)
	if pos.X != 300 || pos.Y != 200 || pos.Action != srcCopy || pos.Time != 5 {
		t.Fatalf("position %+v", pos)
	}
	if step.Send[0].Destination != winA || step.Send[0].Window != winA {
		t.Fatalf("addressing %+v", step.Send[0])
	}
}

func TestSourceState_VersionIsTheLowerOfBoth(t *testing.T) {
	s := newSource(t)
	step := move(t, s, 1, 1, aware(winA, 3))
	enterMsg, err := DecodeEnter(step.Send[0].Data)
	if err != nil {
		t.Fatal(err)
	}
	if enterMsg.Version != 3 {
		t.Fatalf("version %d", enterMsg.Version)
	}
}

func TestSourceState_ProxyFollowed(t *testing.T) {
	s := newSource(t)
	step := move(t, s, 1, 1, TargetWindow{ID: winA, Version: 5, Proxy: proxyP})
	for _, m := range step.Send {
		if m.Destination != proxyP || m.Window != winA {
			t.Fatalf("message %s goes to %d with window %d", m.Kind, m.Destination, m.Window)
		}
	}
	if st := status(t, s, accept(winA)); len(st.Send) != 0 {
		t.Fatalf("unexpected send %v", kinds(st))
	}
}

func TestSourceState_PositionsCoalescedWhileStatusPending(t *testing.T) {
	s := newSource(t)
	move(t, s, 10, 10, aware(winA, 5))
	if step := move(t, s, 20, 20, aware(winA, 5)); len(step.Send) != 0 {
		t.Fatalf("sent %v while a status was outstanding", kinds(step))
	}
	if step := move(t, s, 30, 30, aware(winA, 5)); len(step.Send) != 0 {
		t.Fatalf("sent %v while a status was outstanding", kinds(step))
	}
	step := status(t, s, accept(winA))
	wantKinds(t, step, KindPosition)
	if pos := DecodePosition(step.Send[0].Data); pos.X != 30 || pos.Y != 30 {
		t.Fatalf("coalesced position %+v, want the latest", pos)
	}
}

func TestSourceState_RectSuppressesPositionsUnlessWanted(t *testing.T) {
	s := newSource(t)
	move(t, s, 10, 10, aware(winA, 5))
	status(t, s, StatusMsg{Target: winA, Accept: true, Action: srcCopy, Rect: Rect{X: 0, Y: 0, W: 100, H: 100}})
	if step := move(t, s, 50, 50, aware(winA, 5)); len(step.Send) != 0 {
		t.Fatalf("sent %v inside the rectangle", kinds(step))
	}
	wantKinds(t, move(t, s, 150, 50, aware(winA, 5)), KindPosition)

	w := newSource(t)
	move(t, w, 10, 10, aware(winA, 5))
	status(t, w, StatusMsg{Target: winA, Accept: true, WantPositions: true, Action: srcCopy, Rect: Rect{W: 100, H: 100}})
	wantKinds(t, move(t, w, 50, 50, aware(winA, 5)), KindPosition)
}

func TestSourceState_EmptyRectSendsOnEveryMove(t *testing.T) {
	s := newSource(t)
	move(t, s, 10, 10, aware(winA, 5))
	status(t, s, accept(winA))
	wantKinds(t, move(t, s, 11, 10, aware(winA, 5)), KindPosition)
}

func TestSourceState_LeaveOnWindowChange(t *testing.T) {
	s := newSource(t)
	move(t, s, 10, 10, aware(winA, 5))
	step := move(t, s, 500, 10, aware(winB, 5))
	wantKinds(t, step, KindLeave, KindEnter, KindPosition)
	if step.Send[0].Window != winA || step.Send[1].Window != winB {
		t.Fatalf("windows %d then %d", step.Send[0].Window, step.Send[1].Window)
	}
	if got := status(t, s, accept(winA)); !got.Ignored {
		t.Fatal("a late status of the previous window was handled")
	}
}

func TestSourceState_MovingOffTheTargetSendsLeave(t *testing.T) {
	s := newSource(t)
	move(t, s, 10, 10, aware(winA, 5))
	wantKinds(t, move(t, s, 900, 900, TargetWindow{}), KindLeave)
	if step := move(t, s, 901, 900, TargetWindow{}); len(step.Send) != 0 {
		t.Fatalf("sent %v with no target", kinds(step))
	}
}

func TestSourceState_ReleaseSendsDropWhenAccepted(t *testing.T) {
	s := newSource(t)
	move(t, s, 10, 10, aware(winA, 5))
	status(t, s, accept(winA))
	step, err := s.Release(77)
	if err != nil {
		t.Fatal(err)
	}
	wantKinds(t, step, KindDrop)
	if drop := DecodeDrop(step.Send[0].Data); drop.Source != srcSelf || drop.Time != 77 {
		t.Fatalf("drop %+v", drop)
	}
	if step.Outcome != OutcomePending || s.Waiting() != WaitFinished {
		t.Fatalf("outcome %s waiting %d", step.Outcome, s.Waiting())
	}
}

func TestSourceState_FinishedAcceptedEndsDropped(t *testing.T) {
	s := newSource(t)
	move(t, s, 10, 10, aware(winA, 5))
	status(t, s, accept(winA))
	if _, err := s.Release(1); err != nil {
		t.Fatal(err)
	}
	done := s.Finished(FinishedMsg{Target: winA, Accepted: true, Action: srcCopy})
	if done.Outcome != OutcomeDropped || done.Action != srcCopy {
		t.Fatalf("step %+v", done)
	}
	if s.Target().ID != winA {
		t.Fatalf("target %+v", s.Target())
	}
}

func TestSourceState_FinishedRejectedAtVersionFive(t *testing.T) {
	s := newSource(t)
	move(t, s, 10, 10, aware(winA, 5))
	status(t, s, accept(winA))
	if _, err := s.Release(1); err != nil {
		t.Fatal(err)
	}
	if done := s.Finished(FinishedMsg{Target: winA}); done.Outcome != OutcomeRejected {
		t.Fatalf("step %+v", done)
	}
}

func TestSourceState_FinishedBelowVersionFiveCountsAsAccepted(t *testing.T) {
	s := newSource(t)
	move(t, s, 10, 10, aware(winA, 4))
	status(t, s, accept(winA))
	if _, err := s.Release(1); err != nil {
		t.Fatal(err)
	}
	if done := s.Finished(FinishedMsg{Target: winA}); done.Outcome != OutcomeDropped {
		t.Fatalf("step %+v", done)
	}
}

func TestSourceState_FinishedFromAnotherWindowIsIgnored(t *testing.T) {
	s := newSource(t)
	move(t, s, 10, 10, aware(winA, 5))
	status(t, s, accept(winA))
	if _, err := s.Release(1); err != nil {
		t.Fatal(err)
	}
	if done := s.Finished(FinishedMsg{Target: winB, Accepted: true}); !done.Ignored {
		t.Fatalf("step %+v", done)
	}
}

func TestSourceState_ReleaseNotAcceptedLeavesAndRejects(t *testing.T) {
	s := newSource(t)
	move(t, s, 10, 10, aware(winA, 5))
	status(t, s, StatusMsg{Target: winA})
	step, err := s.Release(1)
	if err != nil {
		t.Fatal(err)
	}
	wantKinds(t, step, KindLeave)
	if step.Outcome != OutcomeRejected {
		t.Fatalf("outcome %s", step.Outcome)
	}
}

func TestSourceState_AcceptWithNoneActionIsNotAccepted(t *testing.T) {
	s := newSource(t)
	move(t, s, 10, 10, aware(winA, 5))
	status(t, s, StatusMsg{Target: winA, Accept: true})
	step, err := s.Release(1)
	if err != nil {
		t.Fatal(err)
	}
	if step.Outcome != OutcomeRejected {
		t.Fatalf("outcome %s", step.Outcome)
	}
}

func TestSourceState_ReleaseBeforeAnyStatusLeavesWithoutWaiting(t *testing.T) {
	s := newSource(t)
	move(t, s, 10, 10, aware(winA, 5))
	step, err := s.Release(9)
	if err != nil {
		t.Fatal(err)
	}
	wantKinds(t, step, KindLeave)
	if step.Outcome != OutcomeRejected || s.Waiting() != WaitNone {
		t.Fatalf("outcome %s waiting %d", step.Outcome, s.Waiting())
	}
}

func TestSourceState_ReleaseWaitsForOutstandingStatus(t *testing.T) {
	s := newSource(t)
	move(t, s, 10, 10, aware(winA, 5))
	status(t, s, accept(winA))
	move(t, s, 11, 10, aware(winA, 5))
	step, err := s.Release(9)
	if err != nil {
		t.Fatal(err)
	}
	if len(step.Send) != 0 || step.Outcome != OutcomePending || s.Waiting() != WaitStatus {
		t.Fatalf("step %+v waiting %d", step, s.Waiting())
	}
	if moved := move(t, s, 11, 11, aware(winA, 5)); !moved.Ignored {
		t.Fatal("motion after the release was handled")
	}
	drop := status(t, s, accept(winA))
	wantKinds(t, drop, KindDrop)
	if got := DecodeDrop(drop.Send[0].Data); got.Time != 9 {
		t.Fatalf("drop time %d, want the release time", got.Time)
	}
}

func TestSourceState_StatusTimeoutLeavesAndGivesUp(t *testing.T) {
	s := newSource(t)
	move(t, s, 10, 10, aware(winA, 5))
	status(t, s, accept(winA))
	move(t, s, 11, 10, aware(winA, 5))
	if _, err := s.Release(1); err != nil {
		t.Fatal(err)
	}
	step := s.Timeout()
	wantKinds(t, step, KindLeave)
	if step.Outcome != OutcomeTimedOut {
		t.Fatalf("outcome %s", step.Outcome)
	}
}

func TestSourceState_FinishTimeoutGivesUp(t *testing.T) {
	s := newSource(t)
	move(t, s, 10, 10, aware(winA, 5))
	status(t, s, accept(winA))
	if _, err := s.Release(1); err != nil {
		t.Fatal(err)
	}
	step := s.Timeout()
	if len(step.Send) != 0 || step.Outcome != OutcomeTimedOut {
		t.Fatalf("step %+v", step)
	}
}

func TestSourceState_TimeoutWithoutWaitIsIgnored(t *testing.T) {
	s := newSource(t)
	move(t, s, 10, 10, aware(winA, 5))
	if step := s.Timeout(); !step.Ignored {
		t.Fatalf("step %+v", step)
	}
}

func TestSourceState_TargetBelowVersionTwoCompletesWithoutFinished(t *testing.T) {
	s := newSource(t)
	first := move(t, s, 10, 10, aware(winA, 1))
	if pos := DecodePosition(first.Send[1].Data); pos.Action != 0 {
		t.Fatalf("action %d sent to a version 1 target", pos.Action)
	}
	status(t, s, StatusMsg{Target: winA, Accept: true})
	step, err := s.Release(1)
	if err != nil {
		t.Fatal(err)
	}
	wantKinds(t, step, KindDrop)
	if step.Outcome != OutcomeDropped {
		t.Fatalf("outcome %s", step.Outcome)
	}
}

func TestSourceState_CancelSendsLeave(t *testing.T) {
	s := newSource(t)
	move(t, s, 10, 10, aware(winA, 5))
	step := s.Cancel()
	wantKinds(t, step, KindLeave)
	if step.Outcome != OutcomeCancelled {
		t.Fatalf("outcome %s", step.Outcome)
	}
	if again := s.Cancel(); !again.Ignored {
		t.Fatal("second cancel was handled")
	}
}

func TestSourceState_CancelAfterDropIsIgnored(t *testing.T) {
	s := newSource(t)
	move(t, s, 10, 10, aware(winA, 5))
	status(t, s, accept(winA))
	if _, err := s.Release(1); err != nil {
		t.Fatal(err)
	}
	if step := s.Cancel(); !step.Ignored {
		t.Fatalf("step %+v", step)
	}
}

func TestSourceState_CancelWithoutTargetEndsCancelled(t *testing.T) {
	s := newSource(t)
	if step := s.Cancel(); step.Outcome != OutcomeCancelled || len(step.Send) != 0 {
		t.Fatalf("step %+v", step)
	}
}

func TestSourceState_ReleaseWithoutTargetEndsNoTarget(t *testing.T) {
	s := newSource(t)
	move(t, s, 10, 10, TargetWindow{})
	step, err := s.Release(1)
	if err != nil {
		t.Fatal(err)
	}
	if step.Outcome != OutcomeNoTarget || len(step.Send) != 0 {
		t.Fatalf("step %+v", step)
	}
	if again, err := s.Release(2); err != nil || !again.Ignored {
		t.Fatalf("second release %+v %v", again, err)
	}
}

func TestSourceState_UnsolicitedStatusIsIgnored(t *testing.T) {
	s := newSource(t)
	if step := status(t, s, accept(winA)); !step.Ignored {
		t.Fatal("status with no drag was handled")
	}
	move(t, s, 10, 10, aware(winA, 5))
	status(t, s, accept(winA))
	if step := status(t, s, accept(winA)); !step.Ignored {
		t.Fatal("status with no position outstanding was handled")
	}
}

func TestSourceState_MoveRejectsUnencodableCoordinates(t *testing.T) {
	s := newSource(t)
	if _, err := s.Move(Motion{X: -5, Y: 0, Target: aware(winA, 5)}); !errors.Is(err, ErrCoordinate) {
		t.Fatalf("err %v", err)
	}
}

func TestSourceState_MoreThanThreeTypesNeedTheTypeList(t *testing.T) {
	if newSource(t).NeedsTypeList() {
		t.Fatal("one type needs no list")
	}
	many := newSource(t, 1, 2, 3, 4)
	if !many.NeedsTypeList() {
		t.Fatal("four types need the list")
	}
	step := move(t, many, 1, 1, aware(winA, 5))
	enterMsg, err := DecodeEnter(step.Send[0].Data)
	if err != nil || !enterMsg.TypeListRequired {
		t.Fatalf("enter %+v %v", enterMsg, err)
	}
}

func TestNewSourceState_RejectsNoTypes(t *testing.T) {
	if _, err := NewSourceState(1, nil, srcCopy); !errors.Is(err, ErrNoTypes) {
		t.Fatalf("err %v", err)
	}
}

func TestOutcome_StringCoversEveryOutcome(t *testing.T) {
	for _, o := range []Outcome{OutcomePending, OutcomeDropped, OutcomeRejected, OutcomeCancelled, OutcomeNoTarget, OutcomeTimedOut, Outcome(99)} {
		if o.String() == "" {
			t.Errorf("outcome %d has an empty name", o)
		}
	}
}
