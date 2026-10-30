//go:build windows

package main

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"slices"
	"testing"
	"testing/synctest"
	"time"

	"github.com/kikz0r/edgesync/spikes/internal/wincom"
)

const (
	vtblStreamRead      = 3
	vtblDataSetData     = 7
	vtblQueryContinue   = 3
	vtblGiveFeedback    = 4
	testSmallFile       = 10 << 10
	descriptorByteCount = 592
)

var testModTime = time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)

func expectedContent(start, length int) []byte {
	out := make([]byte, length)
	for i := range out {
		out[i] = byte((int64(start+i)*31 + 7) % 256)
	}
	return out
}

func TestContentReader_DeterministicContentAndSeek(t *testing.T) {
	r := newContentReader(testSmallFile, streamOptions{})
	got, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, expectedContent(0, testSmallFile)) {
		t.Error("content differs from byte(offset*31+7)")
	}

	for _, tc := range []struct {
		name   string
		offset int64
		whence int
		want   int64
	}{
		{"start", 100, io.SeekStart, 100},
		{"current", -40, io.SeekCurrent, 60},
		{"end", -10, io.SeekEnd, testSmallFile - 10},
		{"past", 5, io.SeekEnd, testSmallFile + 5},
	} {
		pos, err := r.Seek(tc.offset, tc.whence)
		if err != nil || pos != tc.want {
			t.Errorf("%s: Seek = %d, %v, want %d", tc.name, pos, err, tc.want)
		}
	}

	if _, err := r.Seek(-1, io.SeekStart); !errors.Is(err, ErrNegativeOffset) {
		t.Errorf("negative seek error = %v, want ErrNegativeOffset", err)
	}
	if _, err := r.Seek(0, 99); !errors.Is(err, ErrInvalidWhence) {
		t.Errorf("bad whence error = %v, want ErrInvalidWhence", err)
	}

	if _, err := r.Seek(5000, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 64)
	if n, err := r.Read(buf); err != nil || n != 64 || !bytes.Equal(buf, expectedContent(5000, 64)) {
		t.Errorf("read after seek = %d, %v, content match %t", n, err, bytes.Equal(buf, expectedContent(5000, 64)))
	}
	if _, err := r.Seek(0, io.SeekEnd); err != nil {
		t.Fatal(err)
	}
	if n, err := r.Read(buf); n != 0 || !errors.Is(err, io.EOF) {
		t.Errorf("read at the end = %d, %v, want 0 and io.EOF", n, err)
	}
}

func TestContentReader_ThrottledReadRespectsRateAndLatency(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		const (
			size    = 2 << 20
			rate    = 1 << 20
			latency = 250 * time.Millisecond
		)
		r := newContentReader(size, streamOptions{rateBytesPerSec: rate, latency: latency})
		start := time.Now()
		first := make([]byte, 1)
		if _, err := r.Read(first); err != nil {
			t.Fatal(err)
		}
		if elapsed := time.Since(start); elapsed < latency {
			t.Errorf("first byte after %v, want at least the latency %v", elapsed, latency)
		}
		if _, err := io.Copy(io.Discard, r); err != nil {
			t.Fatal(err)
		}
		if elapsed, want := time.Since(start), latency+2*time.Second; elapsed != want {
			t.Errorf("2 MiB at 1 MiB/s took %v, want %v", elapsed, want)
		}
		if got := r.served.Load(); got != size {
			t.Errorf("served = %d, want %d", got, size)
		}
	})
}

func TestBuildFileGroupDescriptor_NameRules(t *testing.T) {
	long := string(bytes.Repeat([]byte("a"), 260))
	for name, entry := range map[string]payloadEntry{
		"reserved question mark": {Path: "a?b.txt", Size: 1, ModTime: testModTime},
		"reserved colon":         {Path: "dir/a:b", Size: 1, ModTime: testModTime},
		"backslash in name":      {Path: `a\b`, Size: 1, ModTime: testModTime},
		"control character":      {Path: "a\x01b", Size: 1, ModTime: testModTime},
		"empty component":        {Path: "a//b", Size: 1, ModTime: testModTime},
		"dot-dot component":      {Path: "../b", Size: 1, ModTime: testModTime},
		"empty path":             {Path: "", Size: 1, ModTime: testModTime},
		"path of 260 units":      {Path: long, Size: 1, ModTime: testModTime},
		"directory with a size":  {Path: "d", Dir: true, Size: 5, ModTime: testModTime},
		"negative size":          {Path: "n", Size: -1, ModTime: testModTime},
	} {
		if _, err := buildFileGroupDescriptor([]payloadEntry{entry}); !errors.Is(err, ErrNotRepresentable) {
			t.Errorf("%s: error = %v, want ErrNotRepresentable", name, err)
		}
	}

	atLimit := string(bytes.Repeat([]byte("a"), maxPathUnits))
	raw, err := buildFileGroupDescriptor([]payloadEntry{{Path: atLimit, Size: 1, ModTime: testModTime}})
	if err != nil {
		t.Fatalf("259 UTF-16 units must be accepted: %v", err)
	}
	if want := groupHeaderSize + descriptorByteCount; len(raw) != want {
		t.Errorf("descriptor length = %d, want %d", len(raw), want)
	}
}

func TestBuildFileGroupDescriptor_LargeSizeAndTimestamp(t *testing.T) {
	const size = 5<<32 + 7
	raw, err := buildFileGroupDescriptor([]payloadEntry{{Path: "big.bin", Size: size, ModTime: testModTime}})
	if err != nil {
		t.Fatal(err)
	}
	infos, err := parseFileGroupDescriptor(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(infos) != 1 || infos[0].Size != size {
		t.Errorf("descriptors = %+v, want one entry of size %d", infos, int64(size))
	}
	if _, err := buildFileGroupDescriptor([]payloadEntry{{Path: "old", Size: 1, ModTime: time.Date(1500, 1, 1, 0, 0, 0, 0, time.UTC)}}); !errors.Is(err, ErrNotRepresentable) {
		t.Errorf("pre-1601 timestamp error = %v, want ErrNotRepresentable", err)
	}
}

func newTestDragData(t *testing.T) (*dragData, uintptr, shellFormats) {
	t.Helper()
	formats, err := registerShellFormats()
	if err != nil {
		t.Fatal(err)
	}
	data, err := newDragData(discardLogger(), formats, demoEntries(testModTime), streamOptions{})
	if err != nil {
		t.Fatal(err)
	}
	return data, hostDataObject(t, data), formats
}

func TestDragData_OffersVirtualFileFormats(t *testing.T) {
	requireNoLeak(t)
	_, obj, formats := newTestDragData(t)

	offered, err := wincom.EnumFormats(obj)
	if err != nil {
		t.Fatal(err)
	}
	want := []wincom.FormatEtc{
		{Format: formats.Descriptor, Aspect: wincom.DVAspectContent, Index: -1, Tymed: wincom.TymedHGlobal},
		{Format: formats.Contents, Aspect: wincom.DVAspectContent, Index: -1, Tymed: wincom.TymedIStream},
		{Format: formats.Preferred, Aspect: wincom.DVAspectContent, Index: -1, Tymed: wincom.TymedHGlobal},
	}
	if !slices.Equal(offered, want) {
		t.Errorf("formats = %+v, want %+v", offered, want)
	}
	if got := formatName(formats.Contents); got != wincom.FormatNameFileContents {
		t.Errorf("formatName(FileContents) = %q", got)
	}

	if err := wincom.QueryGetData(obj, want[1]); err != nil {
		t.Errorf("QueryGetData(FileContents) = %v, want S_OK", err)
	}
	cfHDrop := wincom.FormatEtc{Format: wincom.CFHDrop, Aspect: wincom.DVAspectContent, Index: -1, Tymed: wincom.TymedHGlobal}
	var hr wincom.HResult
	if err := wincom.QueryGetData(obj, cfHDrop); !errors.As(err, &hr) || hr != wincom.DVEFormatEtc {
		t.Errorf("QueryGetData(CF_HDROP) = %v, want DV_E_FORMATETC: the payload is virtual only", err)
	}
}

func TestDragData_GetDataDescriptorAndPreferredEffect(t *testing.T) {
	requireNoLeak(t)
	_, obj, formats := newTestDragData(t)

	medium, err := wincom.GetData(obj, wincom.FormatEtc{Format: formats.Descriptor, Aspect: wincom.DVAspectContent, Index: -1, Tymed: wincom.TymedHGlobal})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := medium.HGlobalBytes()
	wincom.ReleaseMedium(&medium)
	if err != nil {
		t.Fatal(err)
	}
	infos, err := parseFileGroupDescriptor(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(infos) != 3 || infos[0].Name != "demo" || infos[1].Name != `demo\a.bin` || infos[1].Size != demoLargeFileSize || infos[2].Name != "b.txt" {
		t.Errorf("descriptors = %+v, want demo, demo\\a.bin (200 MiB), b.txt", infos)
	}

	medium, err = wincom.GetData(obj, wincom.FormatEtc{Format: formats.Preferred, Aspect: wincom.DVAspectContent, Index: -1, Tymed: wincom.TymedHGlobal})
	if err != nil {
		t.Fatal(err)
	}
	raw, err = medium.HGlobalBytes()
	wincom.ReleaseMedium(&medium)
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) != 4 || binary.LittleEndian.Uint32(raw) != wincom.DropEffectCopy {
		t.Errorf("Preferred DropEffect = %v, want DROPEFFECT_COPY", raw)
	}
}

func TestDragData_FileContentsStreamIsLazyAndAccounted(t *testing.T) {
	requireNoLeak(t)
	data, obj, formats := newTestDragData(t)
	contents := func(index int32) wincom.FormatEtc {
		return wincom.FormatEtc{Format: formats.Contents, Aspect: wincom.DVAspectContent, Index: index, Tymed: wincom.TymedIStream}
	}

	medium, err := wincom.GetData(obj, contents(2))
	if err != nil {
		t.Fatal(err)
	}
	if got := data.Streams(); len(got) != 0 {
		t.Fatalf("stream accounted before it was released: %+v", got)
	}
	buf, bufAddr := scratch[[16]byte](t)
	read, readAddr := scratch[uint32](t)
	if hr := comCall(medium.Handle, vtblStreamRead, bufAddr, uintptr(len(buf)), readAddr); hr != wincom.SOK || *read != 16 {
		t.Fatalf("IStream::Read = %v, %d bytes, want S_OK and 16", hr, *read)
	}
	if !bytes.Equal(buf[:], expectedContent(0, 16)) {
		t.Errorf("stream content = %v, want %v", buf[:], expectedContent(0, 16))
	}
	wincom.ReleaseMedium(&medium)

	got := data.Streams()
	want := []streamRecord{{Path: "b.txt", Size: demoSmallFileSize, Served: 16, Reads: 1, Completed: false}}
	if !slices.Equal(got, want) {
		t.Errorf("stream records = %+v, want %+v", got, want)
	}

	var hr wincom.HResult
	for _, index := range []int32{0, 3, -1} {
		if _, err := wincom.GetData(obj, contents(index)); !errors.As(err, &hr) || hr != wincom.DVELindex {
			t.Errorf("GetData(FileContents, lindex %d) = %v, want DV_E_LINDEX", index, err)
		}
	}
}

func TestDragData_SetDataRecordsDropOutcomes(t *testing.T) {
	requireNoLeak(t)
	data, obj, formats := newTestDragData(t)

	set := func(format uint16, payload []byte) wincom.HResult {
		t.Helper()
		f, fAddr := scratch[wincom.FormatEtc](t)
		*f = wincom.FormatEtc{Format: format, Aspect: wincom.DVAspectContent, Index: -1, Tymed: wincom.TymedHGlobal}
		m, mAddr := scratch[wincom.Medium](t)
		medium, err := wincom.NewHGlobalMedium(payload)
		if err != nil {
			t.Fatal(err)
		}
		*m = medium
		hr := comCall(obj, vtblDataSetData, fAddr, mAddr, 1)
		if hr != wincom.SOK {
			wincom.ReleaseMedium(m)
		}
		return hr
	}

	if hr := set(formats.Performed, binary.LittleEndian.AppendUint32(nil, wincom.DropEffectCopy)); hr != wincom.SOK {
		t.Fatalf("SetData(Performed DropEffect) = %v", hr)
	}
	if hr := set(formats.PasteSucceeded, binary.LittleEndian.AppendUint32(nil, wincom.DropEffectMove)); hr != wincom.SOK {
		t.Fatalf("SetData(Paste Succeeded) = %v", hr)
	}
	want := dropOutcomes{PerformedSet: true, Performed: wincom.DropEffectCopy, PasteSucceeded: true, PasteEffect: wincom.DropEffectMove}
	if got := data.Outcomes(); got != want {
		t.Errorf("outcomes = %+v, want %+v", got, want)
	}

	if hr := set(formats.Descriptor, []byte{1, 0, 0, 0}); hr != wincom.DVEFormatEtc {
		t.Errorf("SetData(unsupported format) = %v, want DV_E_FORMATETC", hr)
	}
	if hr := set(formats.Performed, []byte{1, 2}); hr != wincom.EInvalidArg {
		t.Errorf("SetData(short DWORD) = %v, want E_INVALIDARG", hr)
	}
}

func TestContinueDecision_EscapeCancelsLeftReleaseDrops(t *testing.T) {
	for name, tc := range map[string]struct {
		escape bool
		keys   uint32
		want   wincom.HResult
	}{
		"left held":                 {false, mkLButton, wincom.SOK},
		"left held with modifiers":  {false, mkLButton | mkControl | mkShift, wincom.SOK},
		"left released":             {false, 0, wincom.DragDropSDrop},
		"left released, right held": {false, mkRButton, wincom.DragDropSDrop},
		"escape with left held":     {true, mkLButton, wincom.DragDropSCancel},
		"escape wins over release":  {true, 0, wincom.DragDropSCancel},
	} {
		if got := continueDecision(tc.escape, tc.keys); got != tc.want {
			t.Errorf("%s: continueDecision = %s, want %s", name, decisionName(got), decisionName(tc.want))
		}
	}
}

func TestDragOutcome_ClassifiesDoDragDropResult(t *testing.T) {
	for name, tc := range map[string]struct {
		result  wincom.HResult
		effect  uint32
		want    string
		wantErr bool
	}{
		"dropped as copy": {wincom.DragDropSDrop, wincom.DropEffectCopy, "dropped with effect COPY", false},
		"dropped nowhere": {wincom.DragDropSDrop, wincom.DropEffectNone, "dropped with effect NONE", false},
		"cancelled":       {wincom.DragDropSCancel, wincom.DropEffectNone, "cancelled", false},
		"plain success":   {wincom.SOK, wincom.DropEffectCopy, "completed with effect COPY", false},
		"unspecified":     {0x80040005, 0, "", true},
		"generic failure": {wincom.EFail, 0, "", true},
	} {
		got, err := dragOutcome(tc.result, tc.effect)
		if (err != nil) != tc.wantErr || got != tc.want {
			t.Errorf("%s: dragOutcome = %q, %v, want %q (error %t)", name, got, err, tc.want, tc.wantErr)
		}
		if tc.wantErr && !errors.Is(err, ErrDragFailed) {
			t.Errorf("%s: error %v does not wrap ErrDragFailed", name, err)
		}
	}
}

func TestSourceSession_StateMachineThroughVtable(t *testing.T) {
	requireNoLeak(t)
	session := &sourceSession{log: discardLogger()}
	obj, err := wincom.NewDropSource(session)
	if err != nil {
		t.Fatal(err)
	}
	defer wincom.Release(obj)

	if err := session.begin(); err != nil {
		t.Fatal(err)
	}
	if err := session.begin(); !errors.Is(err, ErrDragSequence) {
		t.Errorf("second begin = %v, want ErrDragSequence", err)
	}

	if hr := comCall(obj, vtblQueryContinue, 0, uintptr(mkLButton)); hr != wincom.SOK {
		t.Errorf("QueryContinueDrag with the button held = %v, want S_OK", hr)
	}
	if got := session.summary().Phase; got != sourceDragging {
		t.Errorf("phase = %s, want dragging", got)
	}

	for _, effect := range []uint32{wincom.DropEffectNone, wincom.DropEffectNone, wincom.DropEffectCopy, wincom.DropEffectCopy | dropEffectScroll} {
		if hr := comCall(obj, vtblGiveFeedback, uintptr(effect)); hr != wincom.DragDropSUseDefaultCur {
			t.Errorf("GiveFeedback(%s) = %v, want DRAGDROP_S_USEDEFAULTCURSORS", effectName(effect), hr)
		}
	}

	if hr := comCall(obj, vtblQueryContinue, 0, 0); hr != wincom.DragDropSDrop {
		t.Errorf("QueryContinueDrag after release = %v, want DRAGDROP_S_DROP", hr)
	}
	summary := session.summary()
	if summary.Phase != sourceDropped || summary.Queries != 2 {
		t.Errorf("summary = %+v, want dropped after 2 queries", summary)
	}
	if want := []string{"NONE", "COPY", "COPY|SCROLL"}; !slices.Equal(summary.Feedback, want) {
		t.Errorf("feedback history = %q, want %q", summary.Feedback, want)
	}
}

func TestSourceSession_EscapeCancels(t *testing.T) {
	requireNoLeak(t)
	session := &sourceSession{log: discardLogger()}
	obj, err := wincom.NewDropSource(session)
	if err != nil {
		t.Fatal(err)
	}
	defer wincom.Release(obj)
	if err := session.begin(); err != nil {
		t.Fatal(err)
	}
	if hr := comCall(obj, vtblQueryContinue, 1, uintptr(mkLButton)); hr != wincom.DragDropSCancel {
		t.Errorf("QueryContinueDrag with Escape = %v, want DRAGDROP_S_CANCEL", hr)
	}
	if got := session.summary().Phase; got != sourceCancelled {
		t.Errorf("phase = %s, want cancelled", got)
	}
}
