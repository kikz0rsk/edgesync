package dnd

import (
	"errors"
	"testing"
)

func testAtoms() Atoms {
	return Atoms{
		Aware: 1, Proxy: 2, Selection: 3, TypeList: 4, Enter: 5, Position: 6, Status: 7, Leave: 8, Drop: 9,
		Finished: 10, ActionCopy: 11, ActionMove: 12, ActionLink: 13, ActionAsk: 14, ActionPrivate: 15,
		URIList: 100, Targets: 101,
	}
}

func TestEnter_ThreeTypesRoundTrip(t *testing.T) {
	msg := EnterMsg{Source: 0x1234, Version: 5, Types: []uint32{10, 11, 12}}
	data, err := msg.Encode()
	if err != nil {
		t.Fatal(err)
	}
	if want := (Data{0x1234, 0x05000000, 10, 11, 12}); data != want {
		t.Fatalf("encoded %#x, want %#x", data, want)
	}
	got, err := DecodeEnter(data)
	if err != nil {
		t.Fatal(err)
	}
	if got.Source != 0x1234 || got.Version != 5 || got.TypeListRequired || len(got.Types) != 3 {
		t.Fatalf("decoded %+v", got)
	}
}

func TestEnter_MoreThanThreeTypesSetsListFlag(t *testing.T) {
	data, err := EnterMsg{Source: 7, Version: 4, Types: []uint32{1, 2, 3, 4}}.Encode()
	if err != nil {
		t.Fatal(err)
	}
	if want := (Data{7, 0x04000001, 1, 2, 3}); data != want {
		t.Fatalf("encoded %#x, want %#x", data, want)
	}
	got, err := DecodeEnter(data)
	if err != nil {
		t.Fatal(err)
	}
	if !got.TypeListRequired || len(got.Types) != 3 {
		t.Fatalf("decoded %+v", got)
	}
}

func TestEnter_FewerThanThreeTypesLeavesNone(t *testing.T) {
	data, err := EnterMsg{Source: 7, Version: 5, Types: []uint32{9}}.Encode()
	if err != nil {
		t.Fatal(err)
	}
	if want := (Data{7, 0x05000000, 9, 0, 0}); data != want {
		t.Fatalf("encoded %#x, want %#x", data, want)
	}
	got, err := DecodeEnter(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Types) != 1 || got.Types[0] != 9 {
		t.Fatalf("decoded %+v", got)
	}
}

func TestEnter_RejectsNoTypesAndVersionAboveFive(t *testing.T) {
	if _, err := (EnterMsg{Source: 1, Version: 5}).Encode(); !errors.Is(err, ErrNoTypes) {
		t.Fatalf("no types: %v", err)
	}
	if _, err := (EnterMsg{Source: 1, Version: 6, Types: []uint32{1}}).Encode(); !errors.Is(err, ErrVersion) {
		t.Fatalf("encode version 6: %v", err)
	}
	if _, err := DecodeEnter(Data{1, 6 << 24, 1}); !errors.Is(err, ErrVersion) {
		t.Fatalf("decode version 6: %v", err)
	}
}

func TestPosition_PacksRootXY(t *testing.T) {
	data, err := PositionMsg{Source: 3, State: 0x104, X: 300, Y: 200, Time: 99, Action: 11}.Encode()
	if err != nil {
		t.Fatal(err)
	}
	if want := (Data{3, 0x04, 0x012C00C8, 99, 11}); data != want {
		t.Fatalf("encoded %#x, want %#x", data, want)
	}
	got := DecodePosition(data)
	if got.X != 300 || got.Y != 200 || got.Time != 99 || got.Action != 11 || got.State != 4 {
		t.Fatalf("decoded %+v", got)
	}
}

func TestPosition_RejectsCoordinatesOutsideSixteenBits(t *testing.T) {
	for _, p := range []PositionMsg{{X: -1}, {Y: -1}, {X: 0x10000}, {Y: 0x10000}} {
		if _, err := p.Encode(); !errors.Is(err, ErrCoordinate) {
			t.Fatalf("%+v: %v", p, err)
		}
	}
}

func TestStatus_AcceptWantPositionsAndRect(t *testing.T) {
	msg := StatusMsg{Target: 5, Accept: true, WantPositions: true, Rect: Rect{X: 10, Y: 20, W: 30, H: 40}, Action: 11}
	data, err := msg.Encode()
	if err != nil {
		t.Fatal(err)
	}
	if want := (Data{5, 3, 10<<16 | 20, 30<<16 | 40, 11}); data != want {
		t.Fatalf("encoded %#x, want %#x", data, want)
	}
	if got := DecodeStatus(data); got != msg {
		t.Fatalf("decoded %+v, want %+v", got, msg)
	}
}

func TestStatus_DeclineHasNoFlagsAndNoneAction(t *testing.T) {
	data, err := StatusMsg{Target: 5}.Encode()
	if err != nil {
		t.Fatal(err)
	}
	if want := (Data{5}); data != want {
		t.Fatalf("encoded %#x, want %#x", data, want)
	}
}

func TestStatus_RejectsRectOutsideSixteenBits(t *testing.T) {
	if _, err := (StatusMsg{Rect: Rect{W: 0x10000}}).Encode(); !errors.Is(err, ErrCoordinate) {
		t.Fatalf("width: %v", err)
	}
	if _, err := (StatusMsg{Rect: Rect{X: -1}}).Encode(); !errors.Is(err, ErrCoordinate) {
		t.Fatalf("x: %v", err)
	}
}

func TestRect_EmptyAndContains(t *testing.T) {
	r := Rect{X: 10, Y: 10, W: 5, H: 5}
	for _, tc := range []struct {
		x, y int
		in   bool
	}{{10, 10, true}, {14, 14, true}, {15, 10, false}, {10, 15, false}, {9, 10, false}} {
		if got := r.Contains(tc.x, tc.y); got != tc.in {
			t.Errorf("Contains(%d,%d) = %t", tc.x, tc.y, got)
		}
	}
	if !(Rect{W: 5}).Empty() || !(Rect{H: 5}).Empty() || r.Empty() {
		t.Fatal("Empty is wrong")
	}
}

func TestLeaveDropFinished_Layouts(t *testing.T) {
	if got, want := (LeaveMsg{Source: 4}).Encode(), (Data{4}); got != want {
		t.Fatalf("leave %#x", got)
	}
	drop := DropMsg{Source: 4, Time: 777}
	if got, want := drop.Encode(), (Data{4, 0, 777}); got != want {
		t.Fatalf("drop %#x", got)
	}
	if got := DecodeDrop(drop.Encode()); got != drop {
		t.Fatalf("drop round trip %+v", got)
	}
	finished := FinishedMsg{Target: 8, Accepted: true, Action: 11}
	if got, want := finished.Encode(), (Data{8, 1, 11}); got != want {
		t.Fatalf("finished %#x", got)
	}
	if got := DecodeFinished(finished.Encode()); got != finished {
		t.Fatalf("finished round trip %+v", got)
	}
	if got := DecodeFinished((FinishedMsg{Target: 8}).Encode()); got.Accepted || got.Action != 0 {
		t.Fatalf("rejected finished %+v", got)
	}
	if got := DecodeLeave(Data{4}); got.Source != 4 {
		t.Fatalf("leave round trip %+v", got)
	}
}

func TestNegotiateVersion_LowerOfBothCappedAtFive(t *testing.T) {
	for _, tc := range []struct{ source, target, want uint8 }{{5, 3, 3}, {3, 5, 3}, {5, 5, 5}, {5, 200, 5}, {0, 4, 0}} {
		if got := NegotiateVersion(tc.source, tc.target); got != tc.want {
			t.Errorf("NegotiateVersion(%d,%d) = %d, want %d", tc.source, tc.target, got, tc.want)
		}
	}
}

func TestAtoms_KindAndMessageTypeAreInverse(t *testing.T) {
	a := testAtoms()
	for _, k := range []Kind{KindEnter, KindPosition, KindStatus, KindLeave, KindDrop, KindFinished} {
		got, ok := a.Kind(a.MessageType(k))
		if !ok || got != k {
			t.Errorf("%s: got %s, %t", k, got, ok)
		}
	}
	if _, ok := a.Kind(9999); ok {
		t.Fatal("unknown message type classified")
	}
	if a.MessageType(Kind(0)) != 0 {
		t.Fatal("unknown kind has a message type")
	}
}

func TestNewAtoms_InternsEveryNameOnce(t *testing.T) {
	seen := map[string]int{}
	next := uint32(0)
	a, err := NewAtoms(func(name string) (uint32, error) {
		seen[name]++
		next++
		return next, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(seen) != len(atomNames) {
		t.Fatalf("interned %d names, want %d", len(seen), len(atomNames))
	}
	for name, n := range seen {
		if n != 1 {
			t.Errorf("%s interned %d times", name, n)
		}
	}
	if a.Aware != 1 || a.Targets != uint32(len(atomNames)) {
		t.Fatalf("atoms not assigned in name order: %+v", a)
	}
}

func TestNewAtoms_FailureNamesTheAtom(t *testing.T) {
	boom := errors.New("boom")
	_, err := NewAtoms(func(name string) (uint32, error) {
		if name == "XdndLeave" {
			return 0, boom
		}
		return 1, nil
	})
	if !errors.Is(err, boom) {
		t.Fatalf("got %v", err)
	}
}

func TestKind_StringCoversEveryKind(t *testing.T) {
	for _, k := range []Kind{KindEnter, KindPosition, KindStatus, KindLeave, KindDrop, KindFinished, Kind(99)} {
		if k.String() == "" {
			t.Errorf("kind %d has an empty name", k)
		}
	}
}
