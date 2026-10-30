//go:build windows

package main

import (
	"encoding/binary"
	"errors"
	"log/slog"
	"slices"
	"syscall"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/kikz0r/edgesync/spikes/internal/wincom"
)

const (
	vtblDropTargetDragEnter = 3
	vtblDropTargetDragOver  = 4
	vtblDropTargetDragLeave = 5
	vtblDropTargetDrop      = 6

	allEffects = wincom.DropEffectCopy | wincom.DropEffectMove | wincom.DropEffectLink
)

func pointerAt(addr uintptr) unsafe.Pointer {
	return *(*unsafe.Pointer)(unsafe.Pointer(&addr))
}

func deref(addr uintptr) uintptr {
	return *(*uintptr)(pointerAt(addr))
}

// comCall invokes the index-th method of a hosted COM object through its vtable, the way a foreign caller would.
func comCall(obj uintptr, index int, args ...uintptr) wincom.HResult {
	method := deref(deref(obj) + uintptr(index)*unsafe.Sizeof(uintptr(0)))
	ret, _, _ := syscall.SyscallN(method, append([]uintptr{obj}, args...)...)
	return wincom.HResult(uint32(ret))
}

// scratch allocates a zeroed value outside the Go heap so its address can be passed as a plain uintptr.
func scratch[T any](t *testing.T) (*T, uintptr) {
	t.Helper()
	addr, err := windows.LocalAlloc(windows.LPTR, uint32(unsafe.Sizeof(*new(T))))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := windows.LocalFree(windows.Handle(addr)); err != nil {
			t.Error(err)
		}
	})
	return (*T)(pointerAt(addr)), addr
}

func requireNoLeak(t *testing.T) {
	t.Helper()
	baseline := wincom.ObjectCount()
	t.Cleanup(func() {
		if got := wincom.ObjectCount(); got != baseline {
			t.Errorf("hosted objects alive: %d, want %d", got, baseline)
		}
	})
}

func discardLogger() *slog.Logger {
	return slog.New(slog.DiscardHandler)
}

func packPoint(x, y int32) uintptr {
	return uintptr(uint32(x)) | uintptr(uint32(y))<<32
}

func buildDropFiles(paths []string, wide bool) []byte {
	header := make([]byte, unsafe.Sizeof(wincom.DropFiles{}))
	binary.LittleEndian.PutUint32(header, uint32(len(header)))
	if wide {
		binary.LittleEndian.PutUint32(header[16:], 1)
	}
	out := header
	for _, p := range paths {
		if wide {
			for _, unit := range windows.StringToUTF16(p) {
				out = binary.LittleEndian.AppendUint16(out, unit)
			}
		} else {
			out = append(out, p...)
			out = append(out, 0)
		}
	}
	if wide {
		return binary.LittleEndian.AppendUint16(out, 0)
	}
	return append(out, 0)
}

type fakeDataSource struct {
	formats []wincom.FormatEtc
	data    map[uint16][]byte
	failFor uint16
}

func (s *fakeDataSource) Formats() []wincom.FormatEtc { return s.formats }

func (s *fakeDataSource) GetData(f wincom.FormatEtc) (wincom.Medium, error) {
	if f.Format == s.failFor {
		return wincom.Medium{}, wincom.EFail
	}
	return wincom.NewHGlobalMedium(s.data[f.Format])
}

func (s *fakeDataSource) SetData(wincom.FormatEtc, wincom.Medium) error { return wincom.ENotImpl }

func hglobalFormat(id uint16) wincom.FormatEtc {
	return wincom.FormatEtc{Format: id, Aspect: wincom.DVAspectContent, Index: -1, Tymed: wincom.TymedHGlobal}
}

func registerDescriptorFormat(t *testing.T) uint16 {
	t.Helper()
	id, err := wincom.RegisterFormat(wincom.FormatNameFileGroupDescriptorW)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func hostDataObject(t *testing.T, source wincom.DataSource) uintptr {
	t.Helper()
	obj, err := wincom.NewDataObject(source)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { wincom.Release(obj) })
	return obj
}

func TestRightEdgeRect_GeometryAndErrors(t *testing.T) {
	monitor := screenRect{Left: 0, Top: 0, Right: 2560, Bottom: 1440}
	got, err := rightEdgeRect(monitor, 2)
	if err != nil {
		t.Fatal(err)
	}
	if want := (screenRect{Left: 2558, Top: 0, Right: 2560, Bottom: 1440}); got != want {
		t.Errorf("rightEdgeRect = %+v, want %+v", got, want)
	}
	if got.width() != 2 || got.height() != 1440 {
		t.Errorf("size = %dx%d, want 2x1440", got.width(), got.height())
	}

	offset := screenRect{Left: -1920, Top: 100, Right: 0, Bottom: 1180}
	got, err = rightEdgeRect(offset, 3)
	if err != nil {
		t.Fatal(err)
	}
	if want := (screenRect{Left: -3, Top: 100, Right: 0, Bottom: 1180}); got != want {
		t.Errorf("rightEdgeRect on an offset monitor = %+v, want %+v", got, want)
	}

	for name, tc := range map[string]struct {
		monitor   screenRect
		thickness int32
	}{
		"zero thickness":     {monitor, 0},
		"negative thickness": {monitor, -1},
		"wider than monitor": {monitor, 2561},
		"empty monitor":      {screenRect{}, 2},
	} {
		if _, err := rightEdgeRect(tc.monitor, tc.thickness); !errors.Is(err, ErrInvalidEdge) {
			t.Errorf("%s: error = %v, want ErrInvalidEdge", name, err)
		}
	}
}

func TestScreenRect_ContainsAndLocalAreHalfOpen(t *testing.T) {
	edge := screenRect{Left: 2558, Top: 0, Right: 2560, Bottom: 1440}
	for name, tc := range map[string]struct {
		pt   wincom.Point
		want bool
	}{
		"left column":         {wincom.Point{X: 2558, Y: 10}, true},
		"right column":        {wincom.Point{X: 2559, Y: 10}, true},
		"one past the right":  {wincom.Point{X: 2560, Y: 10}, false},
		"one before the left": {wincom.Point{X: 2557, Y: 10}, false},
		"top row":             {wincom.Point{X: 2558, Y: 0}, true},
		"one past the bottom": {wincom.Point{X: 2558, Y: 1440}, false},
		"above":               {wincom.Point{X: 2558, Y: -1}, false},
	} {
		if got := edge.contains(tc.pt); got != tc.want {
			t.Errorf("%s: contains(%+v) = %t, want %t", name, tc.pt, got, tc.want)
		}
	}
	if got := edge.local(wincom.Point{X: 2559, Y: 700}); got != (wincom.Point{X: 1, Y: 700}) {
		t.Errorf("local = %+v, want {1 700}", got)
	}
	left := screenRect{Left: -3, Top: 100, Right: 0, Bottom: 1180}
	if got := left.local(wincom.Point{X: -2, Y: 100}); got != (wincom.Point{X: 1, Y: 0}) {
		t.Errorf("local on negative origin = %+v, want {1 0}", got)
	}
}

func TestEdgeObserver_ClickThroughStateMachine(t *testing.T) {
	edge := screenRect{Left: 2558, Top: 0, Right: 2560, Bottom: 1440}
	inside := wincom.Point{X: 2559, Y: 500}
	outside := wincom.Point{X: 1000, Y: 500}

	t.Run("press outside opens the strip and release restores click-through", func(t *testing.T) {
		o := newEdgeObserver(edge)
		down := o.observe(mouseEvent{wmLButtonDown, outside})
		if down.Transparent || !down.Changed {
			t.Errorf("press outside = %+v, want opaque and changed", down)
		}
		if move := o.observe(mouseEvent{wmMouseMove, inside}); move.Changed || move.Transparent {
			t.Errorf("move inside while pressed = %+v, want no change and still opaque", move)
		}
		o.dragEntered()
		up := o.observe(mouseEvent{wmLButtonUp, inside})
		if !up.Transparent || !up.Changed {
			t.Errorf("release = %+v, want click-through and changed", up)
		}
		if up.Finished == nil || !up.Finished.CrossedEdge || !up.Finished.DragEntered {
			t.Errorf("gesture report = %+v, want crossed and entered", up.Finished)
		}
		if gestures, missed := o.totals(); gestures != 1 || missed != 0 {
			t.Errorf("totals = %d gestures, %d missed, want 1 and 0", gestures, missed)
		}
	})

	t.Run("crossing without DragEnter is counted as a miss", func(t *testing.T) {
		o := newEdgeObserver(edge)
		o.observe(mouseEvent{wmLButtonDown, outside})
		o.observe(mouseEvent{wmMouseMove, inside})
		up := o.observe(mouseEvent{wmLButtonUp, outside})
		if up.Finished == nil || !up.Finished.CrossedEdge || up.Finished.DragEntered {
			t.Errorf("gesture report = %+v, want crossed without entered", up.Finished)
		}
		if gestures, missed := o.totals(); gestures != 1 || missed != 1 {
			t.Errorf("totals = %d gestures, %d missed, want 1 and 1", gestures, missed)
		}
	})

	t.Run("press inside the strip leaves click-through alone", func(t *testing.T) {
		o := newEdgeObserver(edge)
		down := o.observe(mouseEvent{wmLButtonDown, inside})
		if !down.Transparent || down.Changed {
			t.Errorf("press inside = %+v, want click-through unchanged", down)
		}
		up := o.observe(mouseEvent{wmLButtonUp, inside})
		if up.Finished != nil || up.Changed {
			t.Errorf("release after an inside press = %+v, want no report and no change", up)
		}
	})

	t.Run("a new press resets the previous gesture", func(t *testing.T) {
		o := newEdgeObserver(edge)
		o.observe(mouseEvent{wmLButtonDown, outside})
		o.observe(mouseEvent{wmMouseMove, inside})
		o.dragEntered()
		o.observe(mouseEvent{wmLButtonUp, outside})
		o.observe(mouseEvent{wmLButtonDown, outside})
		up := o.observe(mouseEvent{wmLButtonUp, outside})
		if up.Finished == nil || up.Finished.CrossedEdge || up.Finished.DragEntered {
			t.Errorf("second gesture report = %+v, want a clean slate", up.Finished)
		}
	})

	t.Run("moves without a press change nothing", func(t *testing.T) {
		o := newEdgeObserver(edge)
		if move := o.observe(mouseEvent{wmMouseMove, inside}); move.Changed || !move.Transparent || move.Finished != nil {
			t.Errorf("idle move = %+v, want nothing", move)
		}
	})
}

func TestEffects_NegotiationAndKeyMapping(t *testing.T) {
	for name, tc := range map[string]struct {
		allowed uint32
		want    uint32
	}{
		"all allowed":            {allEffects, wincom.DropEffectCopy},
		"copy only":              {wincom.DropEffectCopy, wincom.DropEffectCopy},
		"move never returned":    {wincom.DropEffectMove, wincom.DropEffectNone},
		"link never returned":    {wincom.DropEffectLink, wincom.DropEffectNone},
		"copy with scroll":       {wincom.DropEffectCopy | dropEffectScroll, wincom.DropEffectCopy},
		"nothing allowed":        {wincom.DropEffectNone, wincom.DropEffectNone},
		"move and link, no copy": {wincom.DropEffectMove | wincom.DropEffectLink, wincom.DropEffectNone},
	} {
		if got := negotiateEffect(tc.allowed); got != tc.want {
			t.Errorf("%s: negotiateEffect(%s) = %s, want %s", name, effectName(tc.allowed), effectName(got), effectName(tc.want))
		}
	}

	for name, tc := range map[string]struct {
		keys uint32
		want uint32
	}{
		"no keys":           {mkLButton, wincom.DropEffectMove},
		"shift":             {mkLButton | mkShift, wincom.DropEffectMove},
		"control":           {mkLButton | mkControl, wincom.DropEffectCopy},
		"control and shift": {mkLButton | mkControl | mkShift, wincom.DropEffectLink},
		"right button only": {mkRButton, wincom.DropEffectMove},
	} {
		if got := effectForKeys(tc.keys); got != tc.want {
			t.Errorf("%s: effectForKeys(%s) = %s, want %s", name, keyStateName(tc.keys), effectName(got), effectName(tc.want))
		}
	}
}

func TestNames_KeyStateEffectTymed(t *testing.T) {
	for _, tc := range []struct{ got, want string }{
		{keyStateName(0), "NONE"},
		{keyStateName(mkLButton | mkControl), "LBUTTON|CONTROL"},
		{keyStateName(mkLButton | mkRButton | mkMButton | mkShift | mkControl), "LBUTTON|RBUTTON|MBUTTON|SHIFT|CONTROL"},
		{keyStateName(mkLButton | 0x20), "LBUTTON|0x20"},
		{effectName(allEffects | dropEffectScroll), "COPY|MOVE|LINK|SCROLL"},
		{effectName(0), "NONE"},
		{effectName(0x8), "0x8"},
		{tymedName(wincom.TymedHGlobal | wincom.TymedIStream), "HGLOBAL|ISTREAM"},
		{aspectName(wincom.DVAspectContent), "CONTENT"},
	} {
		if tc.got != tc.want {
			t.Errorf("got %q, want %q", tc.got, tc.want)
		}
	}
}

func TestParseDropFiles_WideAndANSI(t *testing.T) {
	want := []string{`C:\Users\demo\a.txt`, `D:\dir with space\ž.bin`}
	got, err := parseDropFiles(buildDropFiles(want, true))
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got, want) {
		t.Errorf("wide paths = %q, want %q", got, want)
	}

	ansi := []string{`C:\Temp\a.txt`, `C:\Temp\b.txt`}
	got, err = parseDropFiles(buildDropFiles(ansi, false))
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got, ansi) {
		t.Errorf("ANSI paths = %q, want %q", got, ansi)
	}

	got, err = parseDropFiles(buildDropFiles(nil, true))
	if err != nil || len(got) != 0 {
		t.Errorf("empty list = %q, %v, want no paths and no error", got, err)
	}
}

func TestParseDropFiles_Malformed(t *testing.T) {
	valid := buildDropFiles([]string{`C:\a`}, true)
	withOffset := func(offset uint32) []byte {
		raw := slices.Clone(valid)
		binary.LittleEndian.PutUint32(raw, offset)
		return raw
	}
	for name, raw := range map[string][]byte{
		"shorter than header":       valid[:19],
		"offset inside the header":  withOffset(8),
		"offset beyond the data":    withOffset(uint32(len(valid)) + 1),
		"odd wide byte count":       valid[:len(valid)-1],
		"single terminator":         valid[:len(valid)-2],
		"ends inside a name":        valid[:len(valid)-4],
		"narrow without terminator": append(buildDropFiles(nil, false)[:20], 'a', 'b'),
	} {
		if _, err := parseDropFiles(raw); !errors.Is(err, ErrMalformedDropFiles) {
			t.Errorf("%s: error = %v, want ErrMalformedDropFiles", name, err)
		}
	}
}

func TestParseFileGroupDescriptor_RoundTripAndMalformed(t *testing.T) {
	raw, err := buildFileGroupDescriptor(demoEntries(testModTime))
	if err != nil {
		t.Fatal(err)
	}
	infos, err := parseFileGroupDescriptor(raw)
	if err != nil {
		t.Fatal(err)
	}
	want := []descriptorInfo{
		{Name: "demo", Flags: descriptorFlags, Attributes: fileAttributeDirectory, HasSize: true, Size: 0},
		{Name: `demo\a.bin`, Flags: descriptorFlags, Attributes: fileAttributeNormal, HasSize: true, Size: demoLargeFileSize},
		{Name: "b.txt", Flags: descriptorFlags, Attributes: fileAttributeNormal, HasSize: true, Size: demoSmallFileSize},
	}
	if !slices.Equal(infos, want) {
		t.Errorf("descriptors = %+v, want %+v", infos, want)
	}

	huge := binary.LittleEndian.AppendUint32(nil, maxDescriptorEntries+1)
	for name, bad := range map[string][]byte{
		"shorter than the count": {1, 2},
		"count exceeds the data": raw[:len(raw)-1],
		"count above the limit":  huge,
	} {
		if _, err := parseFileGroupDescriptor(bad); !errors.Is(err, ErrMalformedGroup) {
			t.Errorf("%s: error = %v, want ErrMalformedGroup", name, err)
		}
	}
}

func TestDragSession_EnforcesNotificationOrder(t *testing.T) {
	var s dragSession
	if err := s.over(); !errors.Is(err, ErrDragSequence) {
		t.Errorf("over before enter: %v, want ErrDragSequence", err)
	}
	if err := s.finish("DragLeave"); !errors.Is(err, ErrDragSequence) {
		t.Errorf("leave before enter: %v, want ErrDragSequence", err)
	}
	if err := s.enter(); err != nil {
		t.Fatal(err)
	}
	if err := s.enter(); !errors.Is(err, ErrDragSequence) {
		t.Errorf("second enter: %v, want ErrDragSequence", err)
	}
	if err := s.over(); err != nil {
		t.Errorf("over after enter: %v", err)
	}
	if err := s.finish("Drop"); err != nil {
		t.Errorf("drop after enter: %v", err)
	}
	if err := s.finish("DragLeave"); !errors.Is(err, ErrDragSequence) {
		t.Errorf("leave after drop: %v, want ErrDragSequence", err)
	}
	if err := s.enter(); err != nil {
		t.Errorf("enter after a finished drag: %v", err)
	}
}

func TestOverTrace_ReportsOnlyChanges(t *testing.T) {
	var trace overTrace
	if !trace.changed(mkLButton, allEffects) {
		t.Error("first observation must count as a change")
	}
	if trace.changed(mkLButton, allEffects) {
		t.Error("identical observation reported as a change")
	}
	if !trace.changed(mkLButton|mkControl, allEffects) {
		t.Error("key state change not reported")
	}
	if !trace.changed(mkLButton|mkControl, wincom.DropEffectCopy) {
		t.Error("allowed effect change not reported")
	}
}

func TestReadPayload_HDropAndDescriptor(t *testing.T) {
	requireNoLeak(t)
	descriptorFormat := registerDescriptorFormat(t)
	descriptor, err := buildFileGroupDescriptor(demoEntries(testModTime))
	if err != nil {
		t.Fatal(err)
	}
	files := []string{`C:\a.txt`, `C:\b.txt`}
	source := &fakeDataSource{
		formats: []wincom.FormatEtc{hglobalFormat(wincom.CFHDrop), hglobalFormat(descriptorFormat), hglobalFormat(wincom.CFUnicodeText)},
		data: map[uint16][]byte{
			wincom.CFHDrop:       buildDropFiles(files, true),
			descriptorFormat:     descriptor,
			wincom.CFUnicodeText: {'x', 0, 0, 0},
		},
	}
	got, err := readPayload(hostDataObject(t, source), descriptorFormat)
	if err != nil {
		t.Fatal(err)
	}
	if !got.HasHDrop || !slices.Equal(got.Files, files) {
		t.Errorf("CF_HDROP = %t %q, want %q", got.HasHDrop, got.Files, files)
	}
	if !got.HasDescriptor || len(got.Descriptors) != 3 || got.Descriptors[1].Name != `demo\a.bin` {
		t.Errorf("descriptor = %t %+v, want three entries", got.HasDescriptor, got.Descriptors)
	}
	if len(got.Formats) != 3 {
		t.Errorf("formats = %d, want 3", len(got.Formats))
	}
}

func TestReadPayload_AbsentFormatsAreNotAnError(t *testing.T) {
	requireNoLeak(t)
	descriptorFormat := registerDescriptorFormat(t)
	source := &fakeDataSource{
		formats: []wincom.FormatEtc{hglobalFormat(wincom.CFUnicodeText)},
		data:    map[uint16][]byte{wincom.CFUnicodeText: {'x', 0, 0, 0}},
	}
	got, err := readPayload(hostDataObject(t, source), descriptorFormat)
	if err != nil {
		t.Fatal(err)
	}
	if got.HasHDrop || got.HasDescriptor || len(got.Files) != 0 {
		t.Errorf("payload = %+v, want neither CF_HDROP nor descriptor", got)
	}
}

func TestReadPayload_OfferedButUnreadableFails(t *testing.T) {
	requireNoLeak(t)
	descriptorFormat := registerDescriptorFormat(t)
	failing := &fakeDataSource{
		formats: []wincom.FormatEtc{hglobalFormat(wincom.CFHDrop)},
		failFor: wincom.CFHDrop,
	}
	if _, err := readPayload(hostDataObject(t, failing), descriptorFormat); err == nil {
		t.Error("an unreadable CF_HDROP must fail the read")
	}
	malformed := &fakeDataSource{
		formats: []wincom.FormatEtc{hglobalFormat(wincom.CFHDrop)},
		data:    map[uint16][]byte{wincom.CFHDrop: {1, 2, 3}},
	}
	if _, err := readPayload(hostDataObject(t, malformed), descriptorFormat); !errors.Is(err, ErrMalformedDropFiles) {
		t.Errorf("malformed CF_HDROP error = %v, want ErrMalformedDropFiles", err)
	}
}

type edgeFixture struct {
	target *edgeDropTarget
	obj    uintptr
	data   uintptr
	edge   screenRect
}

func newEdgeFixture(t *testing.T) edgeFixture {
	t.Helper()
	requireNoLeak(t)
	descriptorFormat := registerDescriptorFormat(t)
	descriptor, err := buildFileGroupDescriptor(demoEntries(testModTime))
	if err != nil {
		t.Fatal(err)
	}
	edge := screenRect{Left: 2558, Top: 0, Right: 2560, Bottom: 1440}
	observer := newEdgeObserver(edge)
	target := newEdgeDropTarget(discardLogger(), edge, observer, descriptorFormat)
	obj, err := wincom.NewDropTarget(target)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { wincom.Release(obj) })
	data := hostDataObject(t, &fakeDataSource{
		formats: []wincom.FormatEtc{hglobalFormat(wincom.CFHDrop), hglobalFormat(descriptorFormat)},
		data: map[uint16][]byte{
			wincom.CFHDrop:   buildDropFiles([]string{`C:\a.txt`}, true),
			descriptorFormat: descriptor,
		},
	})
	return edgeFixture{target: target, obj: obj, data: data, edge: edge}
}

func effectCell(t *testing.T) (*uint32, uintptr) {
	t.Helper()
	cell, addr := scratch[uint32](t)
	*cell = allEffects
	return cell, addr
}

func TestEdgeDropTarget_EnterOverLeaveThroughVtable(t *testing.T) {
	f := newEdgeFixture(t)
	effect, effectAddr := effectCell(t)

	hr := comCall(f.obj, vtblDropTargetDragEnter, f.data, uintptr(mkLButton), packPoint(2559, 700), effectAddr)
	if hr != wincom.SOK || *effect != wincom.DropEffectCopy {
		t.Fatalf("DragEnter = %v effect %s, want S_OK and COPY", hr, effectName(*effect))
	}

	*effect = allEffects
	hr = comCall(f.obj, vtblDropTargetDragOver, uintptr(mkLButton|mkShift), packPoint(2558, 710), effectAddr)
	if hr != wincom.SOK || *effect != wincom.DropEffectCopy {
		t.Fatalf("DragOver = %v effect %s, want S_OK and COPY even with Shift", hr, effectName(*effect))
	}

	*effect = wincom.DropEffectMove
	hr = comCall(f.obj, vtblDropTargetDragOver, uintptr(mkLButton), packPoint(2558, 720), effectAddr)
	if hr != wincom.SOK || *effect != wincom.DropEffectNone {
		t.Fatalf("DragOver with only MOVE allowed = %v effect %s, want S_OK and NONE", hr, effectName(*effect))
	}

	if hr = comCall(f.obj, vtblDropTargetDragLeave); hr != wincom.SOK {
		t.Fatalf("DragLeave = %v, want S_OK", hr)
	}
	want := edgeTotals{Enters: 1, Overs: 2, Leaves: 1}
	if got := f.target.Totals(); got != want {
		t.Errorf("totals = %+v, want %+v", got, want)
	}
}

func TestEdgeDropTarget_DropReadsPayloadAndReturnsCopy(t *testing.T) {
	f := newEdgeFixture(t)
	effect, effectAddr := effectCell(t)

	comCall(f.obj, vtblDropTargetDragEnter, f.data, uintptr(mkLButton), packPoint(2559, 5), effectAddr)
	*effect = allEffects
	hr := comCall(f.obj, vtblDropTargetDrop, f.data, 0, packPoint(2559, 6), effectAddr)
	if hr != wincom.SOK || *effect != wincom.DropEffectCopy {
		t.Fatalf("Drop = %v effect %s, want S_OK and COPY", hr, effectName(*effect))
	}
	want := edgeTotals{Enters: 1, Drops: 1}
	if got := f.target.Totals(); got != want {
		t.Errorf("totals = %+v, want %+v", got, want)
	}
}

func TestEdgeDropTarget_OutOfOrderNotificationsFailLoudly(t *testing.T) {
	f := newEdgeFixture(t)
	effect, effectAddr := effectCell(t)

	if hr := comCall(f.obj, vtblDropTargetDragOver, 0, packPoint(1, 1), effectAddr); hr != wincom.EFail || *effect != wincom.DropEffectNone {
		t.Errorf("DragOver without DragEnter = %v effect %s, want E_FAIL and NONE", hr, effectName(*effect))
	}
	if hr := comCall(f.obj, vtblDropTargetDragLeave); hr != wincom.EFail {
		t.Errorf("DragLeave without DragEnter = %v, want E_FAIL", hr)
	}
	*effect = allEffects
	if hr := comCall(f.obj, vtblDropTargetDrop, f.data, 0, packPoint(1, 1), effectAddr); hr != wincom.EFail || *effect != wincom.DropEffectNone {
		t.Errorf("Drop without DragEnter = %v effect %s, want E_FAIL and NONE", hr, effectName(*effect))
	}
	if got := f.target.Totals().Failures; got != 3 {
		t.Errorf("failures = %d, want 3", got)
	}
}

func TestEdgeDropTarget_NullEffectPointerIsRejected(t *testing.T) {
	f := newEdgeFixture(t)
	if hr := comCall(f.obj, vtblDropTargetDragEnter, f.data, 0, packPoint(0, 0), 0); hr != wincom.EPointer {
		t.Errorf("DragEnter with a null effect pointer = %v, want E_POINTER", hr)
	}
}

func TestEdgeDropTarget_PayloadFailureRefusesTheDrop(t *testing.T) {
	requireNoLeak(t)
	descriptorFormat := registerDescriptorFormat(t)
	edge := screenRect{Left: 2558, Top: 0, Right: 2560, Bottom: 1440}
	target := newEdgeDropTarget(discardLogger(), edge, newEdgeObserver(edge), descriptorFormat)
	obj, err := wincom.NewDropTarget(target)
	if err != nil {
		t.Fatal(err)
	}
	defer wincom.Release(obj)
	broken := hostDataObject(t, &fakeDataSource{formats: []wincom.FormatEtc{hglobalFormat(wincom.CFHDrop)}, failFor: wincom.CFHDrop})

	effect, effectAddr := effectCell(t)
	hr := comCall(obj, vtblDropTargetDragEnter, broken, uintptr(mkLButton), packPoint(2559, 5), effectAddr)
	if hr != wincom.EFail || *effect != wincom.DropEffectNone {
		t.Errorf("DragEnter with an unreadable payload = %v effect %s, want E_FAIL and NONE", hr, effectName(*effect))
	}
	if hr = comCall(obj, vtblDropTargetDragLeave); hr != wincom.SOK {
		t.Errorf("DragLeave after a failed DragEnter = %v, want S_OK: the enter notification was delivered", hr)
	}
}

func TestPointFromArg_PacksSignedCoordinates(t *testing.T) {
	requireNoLeak(t)
	var got wincom.Point
	obj, err := wincom.NewDropTarget(&pointRecorder{out: &got})
	if err != nil {
		t.Fatal(err)
	}
	defer wincom.Release(obj)
	_, effectAddr := effectCell(t)

	for _, pt := range []wincom.Point{{X: 0, Y: 0}, {X: 2559, Y: 1439}, {X: -5, Y: 10}, {X: 7, Y: -3000}, {X: -1920, Y: -1080}} {
		if hr := comCall(obj, vtblDropTargetDragOver, 0, packPoint(pt.X, pt.Y), effectAddr); hr != wincom.SOK {
			t.Fatalf("DragOver = %v, want S_OK", hr)
		}
		if got != pt {
			t.Errorf("POINTL round trip of %+v = %+v", pt, got)
		}
	}
}

var errUnexpectedCall = errors.New("unexpected call")

type pointRecorder struct {
	out *wincom.Point
}

func (r *pointRecorder) DragEnter(uintptr, uint32, wincom.Point, uint32) (uint32, error) {
	return wincom.DropEffectNone, errUnexpectedCall
}

func (r *pointRecorder) DragOver(_ uint32, pt wincom.Point, _ uint32) (uint32, error) {
	*r.out = pt
	return wincom.DropEffectNone, nil
}

func (r *pointRecorder) DragLeave() error { return errUnexpectedCall }

func (r *pointRecorder) Drop(uintptr, uint32, wincom.Point, uint32) (uint32, error) {
	return wincom.DropEffectNone, errUnexpectedCall
}
