package dnd

import (
	"errors"
	"slices"
	"testing"
)

const (
	tgtURIList = 100
	tgtCopy    = 11
	tgtSource  = 500
	tgtWindow  = 600
)

func newTarget(accept func(x, y int) bool) *TargetState {
	return NewTargetState(tgtURIList, tgtCopy, accept)
}

func enter(t *testing.T, ts *TargetState, version uint8, types ...uint32) TargetStep {
	t.Helper()
	return ts.Enter(tgtWindow, EnterMsg{Source: tgtSource, Version: version, Types: types})
}

func position(x, y int) PositionMsg {
	return PositionMsg{Source: tgtSource, X: x, Y: y}
}

func TestTargetState_EnterPositionStatusAccept(t *testing.T) {
	ts := newTarget(nil)
	if step := enter(t, ts, 5, 7, tgtURIList); step.Event != TargetEventEntered {
		t.Fatalf("enter event %s", step.Event)
	}
	step := ts.Position(position(50, 60))
	if step.Status == nil {
		t.Fatal("no status")
	}
	want := StatusMsg{Target: tgtWindow, Accept: true, Action: tgtCopy}
	if *step.Status != want {
		t.Fatalf("status %+v, want %+v", *step.Status, want)
	}
}

func TestTargetState_TypeListFlagLoadsTheList(t *testing.T) {
	ts := newTarget(nil)
	step := ts.Enter(tgtWindow, EnterMsg{Source: tgtSource, Version: 5, Types: []uint32{1, 2, 3}, TypeListRequired: true})
	if step.LoadTypeList != tgtSource || step.Event != TargetEventNone {
		t.Fatalf("step %+v", step)
	}
	if step := ts.Position(position(1, 1)); !step.Ignored {
		t.Fatal("position before the type list must be ignored")
	}
	next, err := ts.TypeList([]uint32{1, 2, 3, tgtURIList})
	if err != nil {
		t.Fatal(err)
	}
	if next.Event != TargetEventEntered {
		t.Fatalf("event %s", next.Event)
	}
}

func TestTargetState_TypeListWithoutRequestFails(t *testing.T) {
	if _, err := newTarget(nil).TypeList(nil); !errors.Is(err, ErrNoTypeListPending) {
		t.Fatalf("got %v", err)
	}
}

func TestTargetState_NoURIListEmitsUnsupportedAndDeclines(t *testing.T) {
	ts := newTarget(nil)
	if step := enter(t, ts, 5, 7, 8); step.Event != TargetEventUnsupported {
		t.Fatalf("enter event %s", step.Event)
	}
	status := ts.Position(position(1, 1)).Status
	if status == nil || status.Accept || status.Action != 0 {
		t.Fatalf("status %+v", status)
	}
	drop := ts.Drop(DropMsg{Source: tgtSource, Time: 1})
	if drop.Event != TargetEventRejected || drop.Convert != nil {
		t.Fatalf("drop step %+v", drop)
	}
	if drop.Finished == nil || drop.Finished.Accepted || drop.Finished.Action != 0 || drop.Finished.Target != tgtWindow {
		t.Fatalf("finished %+v", drop.Finished)
	}
}

func TestTargetState_LeaveEmitsLeftAndLatePositionIsIgnored(t *testing.T) {
	ts := newTarget(nil)
	enter(t, ts, 5, tgtURIList)
	if step := ts.Leave(LeaveMsg{Source: tgtSource}); step.Event != TargetEventLeft {
		t.Fatalf("leave event %s", step.Event)
	}
	if ts.Active() {
		t.Fatal("still active after leave")
	}
	if step := ts.Position(position(1, 1)); !step.Ignored {
		t.Fatal("late position was not ignored")
	}
	if step := ts.Leave(LeaveMsg{Source: tgtSource}); !step.Ignored {
		t.Fatal("second leave was not ignored")
	}
}

func TestTargetState_MessagesFromAnotherSourceAreIgnored(t *testing.T) {
	ts := newTarget(nil)
	enter(t, ts, 5, tgtURIList)
	if !ts.Position(PositionMsg{Source: 999}).Ignored {
		t.Fatal("position from another source was handled")
	}
	if !ts.Leave(LeaveMsg{Source: 999}).Ignored {
		t.Fatal("leave from another source was handled")
	}
	if !ts.Drop(DropMsg{Source: 999}).Ignored {
		t.Fatal("drop from another source was handled")
	}
}

func TestTargetState_NewEnterReplacesTheDrag(t *testing.T) {
	ts := newTarget(nil)
	enter(t, ts, 5, 7)
	step := ts.Enter(601, EnterMsg{Source: 501, Version: 5, Types: []uint32{tgtURIList}})
	if step.Event != TargetEventEntered {
		t.Fatalf("event %s", step.Event)
	}
	if status := ts.Position(PositionMsg{Source: 501}).Status; status == nil || status.Target != 601 || !status.Accept {
		t.Fatalf("status %+v", status)
	}
}

func TestTargetState_DropConvertsThenFinishesAccepted(t *testing.T) {
	ts := newTarget(nil)
	enter(t, ts, 5, tgtURIList)
	ts.Position(position(5, 5))
	drop := ts.Drop(DropMsg{Source: tgtSource, Time: 4242})
	if drop.Convert == nil || *drop.Convert != (Convert{Type: tgtURIList, Time: 4242}) {
		t.Fatalf("convert %+v", drop.Convert)
	}
	step, err := ts.Data([]byte("file:///home/u/a%20b.txt\r\nfile:///home/u/c.txt\r\n"))
	if err != nil {
		t.Fatal(err)
	}
	if step.Event != TargetEventDropped {
		t.Fatalf("event %s", step.Event)
	}
	if want := []string{"/home/u/a b.txt", "/home/u/c.txt"}; !slices.Equal(step.Dropped.Paths, want) {
		t.Fatalf("paths %q", step.Dropped.Paths)
	}
	if want := (FinishedMsg{Target: tgtWindow, Accepted: true, Action: tgtCopy}); step.Finished == nil || *step.Finished != want {
		t.Fatalf("finished %+v", step.Finished)
	}
	if ts.Active() {
		t.Fatal("still active after the drop completed")
	}
}

func TestTargetState_DropAtDeclinedPositionIsRejectedWithoutConversion(t *testing.T) {
	ts := newTarget(func(x, y int) bool { return x < 100 })
	enter(t, ts, 5, tgtURIList)
	if status := ts.Position(position(200, 0)).Status; status == nil || status.Accept {
		t.Fatalf("status %+v", status)
	}
	drop := ts.Drop(DropMsg{Source: tgtSource})
	if drop.Event != TargetEventRejected || drop.Convert != nil || drop.Finished == nil || drop.Finished.Accepted {
		t.Fatalf("drop step %+v", drop)
	}
}

func TestTargetState_DropWithoutAnyPositionIsRejected(t *testing.T) {
	ts := newTarget(nil)
	enter(t, ts, 5, tgtURIList)
	if drop := ts.Drop(DropMsg{Source: tgtSource}); drop.Event != TargetEventRejected || drop.Convert != nil {
		t.Fatalf("drop step %+v", drop)
	}
}

func TestTargetState_AllEntriesRejectedFinishesNotAccepted(t *testing.T) {
	ts := newTarget(nil)
	enter(t, ts, 5, tgtURIList)
	ts.Position(position(1, 1))
	ts.Drop(DropMsg{Source: tgtSource})
	step, err := ts.Data([]byte("file:///a/b%3Ac\r\nfile://remote/x\r\n"))
	if err != nil {
		t.Fatal(err)
	}
	if step.Event != TargetEventRejected || step.Finished == nil || step.Finished.Accepted {
		t.Fatalf("step %+v", step)
	}
	if len(step.Dropped.Paths) != 0 || len(step.Dropped.Rejected) != 2 {
		t.Fatalf("dropped %+v", step.Dropped)
	}
}

func TestTargetState_PartialRejectionStillDrops(t *testing.T) {
	ts := newTarget(nil)
	enter(t, ts, 5, tgtURIList)
	ts.Position(position(1, 1))
	ts.Drop(DropMsg{Source: tgtSource})
	step, err := ts.Data([]byte("file:///ok.txt\r\nfile:///bad%2A\r\n"))
	if err != nil {
		t.Fatal(err)
	}
	if step.Event != TargetEventDropped || len(step.Dropped.Paths) != 1 || len(step.Dropped.Rejected) != 1 {
		t.Fatalf("step %+v dropped %+v", step, step.Dropped)
	}
}

func TestTargetState_UnparsableListFinishesNotAcceptedAndReturnsTheError(t *testing.T) {
	ts := newTarget(nil)
	enter(t, ts, 5, tgtURIList)
	ts.Position(position(1, 1))
	ts.Drop(DropMsg{Source: tgtSource})
	step, err := ts.Data([]byte("file:///a\x00\r\n"))
	if !errors.Is(err, ErrMalformedURI) {
		t.Fatalf("err %v", err)
	}
	if step.Finished == nil || step.Finished.Accepted || step.Event != TargetEventRejected {
		t.Fatalf("step %+v", step)
	}
}

func TestTargetState_DataFailedFinishesNotAccepted(t *testing.T) {
	ts := newTarget(nil)
	enter(t, ts, 5, tgtURIList)
	ts.Position(position(1, 1))
	ts.Drop(DropMsg{Source: tgtSource})
	step, err := ts.DataFailed()
	if err != nil {
		t.Fatal(err)
	}
	if step.Event != TargetEventRejected || step.Finished == nil || step.Finished.Accepted {
		t.Fatalf("step %+v", step)
	}
	if _, err := ts.DataFailed(); !errors.Is(err, ErrNoDataPending) {
		t.Fatalf("second DataFailed: %v", err)
	}
	if _, err := ts.Data(nil); !errors.Is(err, ErrNoDataPending) {
		t.Fatalf("Data after finish: %v", err)
	}
}

func TestTargetState_VersionBelowTwoSendsNoFinished(t *testing.T) {
	ts := newTarget(nil)
	enter(t, ts, 1, tgtURIList)
	ts.Position(position(1, 1))
	drop := ts.Drop(DropMsg{Source: tgtSource, Time: 9})
	if drop.Convert == nil || drop.Convert.Time != 9 {
		t.Fatalf("convert %+v", drop.Convert)
	}
	step, err := ts.Data([]byte("file:///a.txt\r\n"))
	if err != nil {
		t.Fatal(err)
	}
	if step.Finished != nil || step.Event != TargetEventDropped {
		t.Fatalf("step %+v", step)
	}
}

func TestTargetState_VersionZeroConvertsWithCurrentTime(t *testing.T) {
	ts := newTarget(nil)
	enter(t, ts, 0, tgtURIList)
	ts.Position(position(1, 1))
	drop := ts.Drop(DropMsg{Source: tgtSource, Time: 9})
	if drop.Convert == nil || drop.Convert.Time != 0 {
		t.Fatalf("convert %+v", drop.Convert)
	}
}

func TestTargetEvent_StringCoversEveryEvent(t *testing.T) {
	for _, e := range []TargetEvent{TargetEventNone, TargetEventEntered, TargetEventUnsupported, TargetEventLeft, TargetEventRejected, TargetEventDropped, TargetEvent(99)} {
		if e.String() == "" {
			t.Errorf("event %d has an empty name", e)
		}
	}
}
