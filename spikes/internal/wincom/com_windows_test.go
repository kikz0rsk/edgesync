package wincom

import (
	"bytes"
	"errors"
	"io"
	"math"
	"slices"
	"sync"
	"syscall"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

const callbackBudget = 64

func rawCall(obj uintptr, index int, args ...uintptr) uintptr {
	return callMethod(obj, index, args...)
}

func hresultCall(obj uintptr, index int, args ...uintptr) HResult {
	return HResult(uint32(rawCall(obj, index, args...)))
}

// scratch allocates the value outside the Go heap, so its address can be handed around as a plain uintptr.
func scratch[T any](t *testing.T) (*T, uintptr) {
	t.Helper()
	addr, err := localAlloc(unsafe.Sizeof(*new(T)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := localFree(addr); err != nil {
			t.Error(err)
		}
	})
	return (*T)(ptr(addr)), addr
}

func scratchBytes(t *testing.T, size int) ([]byte, uintptr) {
	t.Helper()
	addr, err := localAlloc(uintptr(size))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := localFree(addr); err != nil {
			t.Error(err)
		}
	})
	return unsafe.Slice((*byte)(ptr(addr)), size), addr
}

func requireNoLeak(t *testing.T) {
	t.Helper()
	baseline := ObjectCount()
	t.Cleanup(func() {
		if got := ObjectCount(); got != baseline {
			t.Errorf("hosted objects alive: %d, want %d", got, baseline)
		}
	})
}

type fakeSource struct {
	formats []FormatEtc
	getData func(f FormatEtc) (Medium, error)
	setData func(f FormatEtc, m Medium) error
}

func (s *fakeSource) Formats() []FormatEtc { return s.formats }

func (s *fakeSource) GetData(f FormatEtc) (Medium, error) { return s.getData(f) }

func (s *fakeSource) SetData(f FormatEtc, m Medium) error { return s.setData(f, m) }

func hglobalFormat(format uint16) FormatEtc {
	return FormatEtc{Format: format, Aspect: DVAspectContent, Index: -1, Tymed: TymedHGlobal}
}

func TestCOM_VtableOrderMatchesSDK(t *testing.T) {
	expected := map[string]struct{ got, want int }{
		"IUnknown.Release":               {vtblRelease, 2},
		"IDataObject.GetData":            {dataGetData, 3},
		"IDataObject.GetDataHere":        {dataGetDataHere, 4},
		"IDataObject.QueryGetData":       {dataQueryGetData, 5},
		"IDataObject.GetCanonicalFormat": {dataGetCanonicalFormatEtc, 6},
		"IDataObject.SetData":            {dataSetData, 7},
		"IDataObject.EnumFormatEtc":      {dataEnumFormatEtc, 8},
		"IDataObject.DAdvise":            {dataDAdvise, 9},
		"IDataObject.DUnadvise":          {dataDUnadvise, 10},
		"IDataObject.EnumDAdvise":        {dataEnumDAdvise, 11},
		"IEnumFORMATETC.Next":            {enumNext, 3},
		"IEnumFORMATETC.Skip":            {enumSkip, 4},
		"IEnumFORMATETC.Reset":           {enumReset, 5},
		"IEnumFORMATETC.Clone":           {enumClone, 6},
		"ISequentialStream.Read":         {streamRead, 3},
		"ISequentialStream.Write":        {streamWrite, 4},
		"IStream.Seek":                   {streamSeek, 5},
		"IStream.SetSize":                {streamSetSize, 6},
		"IStream.CopyTo":                 {streamCopyTo, 7},
		"IStream.Commit":                 {streamCommit, 8},
		"IStream.Revert":                 {streamRevert, 9},
		"IStream.LockRegion":             {streamLockRegion, 10},
		"IStream.UnlockRegion":           {streamUnlockRegion, 11},
		"IStream.Stat":                   {streamStat, 12},
		"IStream.Clone":                  {streamClone, 13},
		"IDropSource.QueryContinueDrag":  {dropSourceQueryContinueDrag, 3},
		"IDropSource.GiveFeedback":       {dropSourceGiveFeedback, 4},
		"IDropTarget.DragEnter":          {dropTargetDragEnter, 3},
		"IDropTarget.DragOver":           {dropTargetDragOver, 4},
		"IDropTarget.DragLeave":          {dropTargetDragLeave, 5},
		"IDropTarget.Drop":               {dropTargetDrop, 6},
		"IUnknown.QueryInterface":        {vtblQueryInterface, 0},
		"IUnknown.AddRef":                {vtblAddRef, 1},
	}
	for name, pair := range expected {
		if pair.got != pair.want {
			t.Errorf("%s vtable index = %d, want %d", name, pair.got, pair.want)
		}
	}
}

func TestCOM_StructLayouts(t *testing.T) {
	checks := []struct {
		name      string
		got, want uintptr
	}{
		{"FormatEtc size", unsafe.Sizeof(FormatEtc{}), 32},
		{"FormatEtc.TargetDevice", unsafe.Offsetof(FormatEtc{}.TargetDevice), 8},
		{"FormatEtc.Aspect", unsafe.Offsetof(FormatEtc{}.Aspect), 16},
		{"FormatEtc.Index", unsafe.Offsetof(FormatEtc{}.Index), 20},
		{"FormatEtc.Tymed", unsafe.Offsetof(FormatEtc{}.Tymed), 24},
		{"Medium size", unsafe.Sizeof(Medium{}), 24},
		{"Medium.Handle", unsafe.Offsetof(Medium{}.Handle), 8},
		{"Medium.ReleaseOwner", unsafe.Offsetof(Medium{}.ReleaseOwner), 16},
		{"statStg size", unsafe.Sizeof(statStg{}), 80},
		{"statStg.Type", unsafe.Offsetof(statStg{}.Type), 8},
		{"statStg.Size", unsafe.Offsetof(statStg{}.Size), 16},
		{"statStg.ModifyTime", unsafe.Offsetof(statStg{}.ModifyTime), 24},
		{"statStg.Mode", unsafe.Offsetof(statStg{}.Mode), 48},
		{"statStg.Clsid", unsafe.Offsetof(statStg{}.Clsid), 56},
		{"statStg.StateBits", unsafe.Offsetof(statStg{}.StateBits), 72},
		{"FileDescriptorW size", unsafe.Sizeof(FileDescriptorW{}), 592},
		{"FileDescriptorW.Clsid", unsafe.Offsetof(FileDescriptorW{}.Clsid), 4},
		{"FileDescriptorW.SizeL", unsafe.Offsetof(FileDescriptorW{}.SizeL), 20},
		{"FileDescriptorW.PointL", unsafe.Offsetof(FileDescriptorW{}.PointL), 28},
		{"FileDescriptorW.FileAttributes", unsafe.Offsetof(FileDescriptorW{}.FileAttributes), 36},
		{"FileDescriptorW.LastWriteTime", unsafe.Offsetof(FileDescriptorW{}.LastWriteTime), 56},
		{"FileDescriptorW.FileSizeHigh", unsafe.Offsetof(FileDescriptorW{}.FileSizeHigh), 64},
		{"FileDescriptorW.FileName", unsafe.Offsetof(FileDescriptorW{}.FileName), 72},
		{"DropFiles size", unsafe.Sizeof(DropFiles{}), 20},
		{"DropFiles.NonClient", unsafe.Offsetof(DropFiles{}.NonClient), 12},
		{"msg size", unsafe.Sizeof(msg{}), 48},
		{"msg.WParam", unsafe.Offsetof(msg{}.WParam), 16},
		{"msg.Time", unsafe.Offsetof(msg{}.Time), 32},
		{"objectBlock size", unsafe.Sizeof(objectBlock{}), 16},
	}
	for _, check := range checks {
		if check.got != check.want {
			t.Errorf("%s = %d, want %d", check.name, check.got, check.want)
		}
	}
}

func TestCOM_QueryInterfaceAddRefRelease(t *testing.T) {
	requireNoLeak(t)
	obj, err := NewDataObject(&fakeSource{})
	if err != nil {
		t.Fatal(err)
	}

	if got := AddRef(obj); got != 2 {
		t.Fatalf("AddRef = %d, want 2", got)
	}
	viaDataObject, err := QueryInterface(obj, iidIDataObject)
	if err != nil || viaDataObject != obj {
		t.Fatalf("QueryInterface(IDataObject) = %#x, %v; want %#x", viaDataObject, err, obj)
	}
	viaUnknown, err := QueryInterface(obj, iidIUnknown)
	if err != nil || viaUnknown != obj {
		t.Fatalf("QueryInterface(IUnknown) = %#x, %v; want %#x", viaUnknown, err, obj)
	}
	if got := AddRef(obj); got != 5 {
		t.Fatalf("AddRef after two successful QueryInterface calls = %d, want 5", got)
	}

	out, outAddr := scratch[uintptr](t)
	*out = 0xDEAD
	if hr := hresultCall(obj, vtblQueryInterface, uintptr(unsafe.Pointer(&iidIStream)), outAddr); hr != ENoInterface {
		t.Fatalf("QueryInterface(IStream) = %v, want E_NOINTERFACE", hr)
	}
	if *out != 0 {
		t.Fatalf("failed QueryInterface left %#x in the out pointer, want nil", *out)
	}
	if hr := hresultCall(obj, vtblQueryInterface, uintptr(unsafe.Pointer(&iidIUnknown)), 0); hr != EPointer {
		t.Fatalf("QueryInterface with nil out pointer = %v, want E_POINTER", hr)
	}
	if hr := hresultCall(obj, vtblQueryInterface, 0, outAddr); hr != EPointer {
		t.Fatalf("QueryInterface with nil iid = %v, want E_POINTER", hr)
	}
	if got := AddRef(obj); got != 6 {
		t.Fatalf("failed QueryInterface calls changed the reference count: AddRef = %d, want 6", got)
	}

	for want := uint32(5); want > 0; want-- {
		if got := Release(obj); got != want {
			t.Fatalf("Release = %d, want %d", got, want)
		}
	}
	if got := Release(obj); got != 0 {
		t.Fatalf("final Release = %d, want 0", got)
	}
	if ObjectCount() != 0 {
		t.Fatalf("ObjectCount after final Release = %d, want 0", ObjectCount())
	}
}

func TestCOM_ReleaseRunsCleanupOnce(t *testing.T) {
	requireNoLeak(t)
	cleaned := 0
	stream, err := NewStream(bytes.NewReader(nil), 0, "", func() { cleaned++ })
	if err != nil {
		t.Fatal(err)
	}
	AddRef(stream)
	Release(stream)
	if cleaned != 0 {
		t.Fatal("cleanup ran while a reference was still held")
	}
	Release(stream)
	if cleaned != 1 {
		t.Fatalf("cleanup ran %d times, want 1", cleaned)
	}
}

func enumTestFormats() []FormatEtc {
	return []FormatEtc{hglobalFormat(101), hglobalFormat(102), hglobalFormat(103), hglobalFormat(104)}
}

func TestCOM_EnumFormatEtcNextSkipReset(t *testing.T) {
	requireNoLeak(t)
	formats := enumTestFormats()
	enum, err := NewFormatEnumerator(formats)
	if err != nil {
		t.Fatal(err)
	}
	defer Release(enum)

	out, outAddr := scratch[[5]FormatEtc](t)
	fetched, fetchedAddr := scratch[uint32](t)

	next := func(count uintptr, fetchedPtr uintptr) HResult {
		*out = [5]FormatEtc{}
		*fetched = 0xFFFF
		return hresultCall(enum, enumNext, count, outAddr, fetchedPtr)
	}

	if hr := next(2, fetchedAddr); hr != SOK || *fetched != 2 || out[0] != formats[0] || out[1] != formats[1] {
		t.Fatalf("Next(2) = %v, fetched %d, %v", hr, *fetched, out[:2])
	}
	if hr := hresultCall(enum, enumSkip, 1); hr != SOK {
		t.Fatalf("Skip(1) = %v, want S_OK", hr)
	}
	if hr := next(5, fetchedAddr); hr != SFalse || *fetched != 1 || out[0] != formats[3] {
		t.Fatalf("Next(5) after Skip = %v, fetched %d, first %v; want S_FALSE, 1, format 104", hr, *fetched, out[0])
	}
	if hr := next(1, 0); hr != SFalse {
		t.Fatalf("Next(1) at the end with nil fetched = %v, want S_FALSE", hr)
	}
	if hr := next(2, 0); hr != EInvalidArg {
		t.Fatalf("Next(2) with nil fetched = %v, want E_INVALIDARG", hr)
	}
	if hr := hresultCall(enum, enumNext, 1, 0, fetchedAddr); hr != EPointer {
		t.Fatalf("Next with nil buffer = %v, want E_POINTER", hr)
	}

	if hr := hresultCall(enum, enumReset); hr != SOK {
		t.Fatalf("Reset = %v, want S_OK", hr)
	}
	if hr := hresultCall(enum, enumSkip, 10); hr != SFalse {
		t.Fatalf("Skip(10) = %v, want S_FALSE", hr)
	}
	if hr := next(1, fetchedAddr); hr != SFalse || *fetched != 0 {
		t.Fatalf("Next after over-skipping = %v, fetched %d; want S_FALSE, 0", hr, *fetched)
	}

	hresultCall(enum, enumReset)
	if hr := next(1, fetchedAddr); hr != SOK || out[0] != formats[0] {
		t.Fatalf("Next(1) after Reset = %v, %v", hr, out[0])
	}
	cloneOut, cloneAddr := scratch[uintptr](t)
	if hr := hresultCall(enum, enumClone, cloneAddr); hr != SOK || *cloneOut == 0 {
		t.Fatalf("Clone = %v, %#x", hr, *cloneOut)
	}
	clone := *cloneOut
	defer Release(clone)
	if hr := hresultCall(clone, enumNext, 1, outAddr, fetchedAddr); hr != SOK || out[0] != formats[1] {
		t.Fatalf("clone Next = %v, %v; want format 102", hr, out[0])
	}
	if hr := hresultCall(clone, enumNext, 1, outAddr, fetchedAddr); hr != SOK || out[0] != formats[2] {
		t.Fatalf("second clone Next = %v, %v; want format 103", hr, out[0])
	}
	if hr := next(1, fetchedAddr); hr != SOK || out[0] != formats[1] {
		t.Fatalf("original Next after clone advanced = %v, %v; want format 102", hr, out[0])
	}
	if hr := hresultCall(enum, enumClone, 0); hr != EPointer {
		t.Fatalf("Clone with nil out = %v, want E_POINTER", hr)
	}
}

type flakyReader struct {
	*bytes.Reader
	failAfter int
	read      int
}

func (f *flakyReader) Read(p []byte) (int, error) {
	if f.read >= f.failAfter {
		return 0, errors.New("simulated network failure")
	}
	if remaining := f.failAfter - f.read; len(p) > remaining {
		p = p[:remaining]
	}
	n, err := f.Reader.Read(p)
	f.read += n
	return n, err
}

func patternBytes(size int) []byte {
	data := make([]byte, size)
	for i := range data {
		data[i] = byte(i*31 + 7)
	}
	return data
}

func seekArg(offset int64) uintptr { return uintptr(offset) }

type streamHarness struct {
	t       *testing.T
	stream  uintptr
	count   *uint32
	countAt uintptr
}

func newStreamHarness(t *testing.T, stream uintptr) *streamHarness {
	count, countAt := scratch[uint32](t)
	return &streamHarness{t: t, stream: stream, count: count, countAt: countAt}
}

func (h *streamHarness) read(obj uintptr, size int) ([]byte, HResult) {
	h.t.Helper()
	buf, addr := scratchBytes(h.t, max(size, 1))
	*h.count = 0xFFFF
	hr := hresultCall(obj, streamRead, addr, uintptr(size), h.countAt)
	return slices.Clone(buf[:*h.count]), hr
}

func TestCOM_IStreamReadSeekStat(t *testing.T) {
	requireNoLeak(t)
	data := patternBytes(100)
	released := 0
	stream, err := NewStream(bytes.NewReader(data), int64(len(data)), `demo\a.bin`, func() { released++ })
	if err != nil {
		t.Fatal(err)
	}
	h := newStreamHarness(t, stream)

	for _, iid := range []windows.GUID{iidIUnknown, iidISequentialStream, iidIStream} {
		got, err := QueryInterface(stream, iid)
		if err != nil || got != stream {
			t.Fatalf("QueryInterface(%v) = %#x, %v", iid, got, err)
		}
		Release(got)
	}
	if _, err := QueryInterface(stream, iidIDataObject); !errors.Is(err, ENoInterface) {
		t.Fatalf("QueryInterface(IDataObject) on a stream = %v, want E_NOINTERFACE", err)
	}

	if got, hr := h.read(stream, 10); hr != SOK || !bytes.Equal(got, data[:10]) {
		t.Fatalf("Read(10) = %v, % x", hr, got)
	}

	position, positionAddr := scratch[uint64](t)
	if hr := hresultCall(stream, streamSeek, seekArg(50), uintptr(streamSeekSet), positionAddr); hr != SOK || *position != 50 {
		t.Fatalf("Seek(SET 50) = %v, position %d", hr, *position)
	}
	if got, hr := h.read(stream, 80); hr != SFalse || !bytes.Equal(got, data[50:]) {
		t.Fatalf("Read(80) at 50 = %v, %d bytes; want S_FALSE and the 50 byte tail", hr, len(got))
	}
	if got, hr := h.read(stream, 4); hr != SFalse || len(got) != 0 {
		t.Fatalf("Read at the end = %v, %d bytes; want S_FALSE, 0", hr, len(got))
	}

	if hr := hresultCall(stream, streamSeek, seekArg(-10), uintptr(streamSeekEnd), positionAddr); hr != SOK || *position != 90 {
		t.Fatalf("Seek(END -10) = %v, position %d", hr, *position)
	}
	if hr := hresultCall(stream, streamSeek, seekArg(-5), uintptr(streamSeekCur), positionAddr); hr != SOK || *position != 85 {
		t.Fatalf("Seek(CUR -5) = %v, position %d", hr, *position)
	}
	if hr := hresultCall(stream, streamSeek, seekArg(0), uintptr(streamSeekCur), 0); hr != SOK {
		t.Fatalf("Seek with nil position = %v, want S_OK", hr)
	}
	if hr := hresultCall(stream, streamSeek, seekArg(-1), uintptr(streamSeekSet), positionAddr); hr != STGEInvalidFunction {
		t.Fatalf("Seek before the start = %v, want STG_E_INVALIDFUNCTION", hr)
	}
	if hr := hresultCall(stream, streamSeek, seekArg(0), 7, positionAddr); hr != STGEInvalidFunction {
		t.Fatalf("Seek with invalid origin = %v, want STG_E_INVALIDFUNCTION", hr)
	}
	if hr := hresultCall(stream, streamSeek, seekArg(500), uintptr(streamSeekSet), positionAddr); hr != SOK || *position != 500 {
		t.Fatalf("Seek past the end = %v, position %d", hr, *position)
	}
	if got, hr := h.read(stream, 4); hr != SFalse || len(got) != 0 {
		t.Fatalf("Read past the end = %v, %d bytes", hr, len(got))
	}

	stat, statAddr := scratch[statStg](t)
	if hr := hresultCall(stream, streamStat, statAddr, 0); hr != SOK {
		t.Fatalf("Stat = %v", hr)
	}
	if stat.Type != stgtyStream || stat.Size != 100 || stat.Mode != 0 || stat.Name == 0 {
		t.Fatalf("Stat = %+v", *stat)
	}
	name := windows.UTF16PtrToString((*uint16)(ptr(stat.Name)))
	windows.CoTaskMemFree(ptr(stat.Name))
	if name != `demo\a.bin` {
		t.Fatalf("Stat name = %q", name)
	}
	if hr := hresultCall(stream, streamStat, statAddr, uintptr(statFlagNoName)); hr != SOK || stat.Name != 0 || stat.Size != 100 {
		t.Fatalf("Stat(NONAME) = %v, %+v", hr, *stat)
	}
	if hr := hresultCall(stream, streamStat, statAddr, 2); hr != STGEInvalidFlag {
		t.Fatalf("Stat with invalid flag = %v, want STG_E_INVALIDFLAG", hr)
	}
	if hr := hresultCall(stream, streamStat, 0, 0); hr != STGEInvalidPointer {
		t.Fatalf("Stat with nil pointer = %v, want STG_E_INVALIDPOINTER", hr)
	}

	_, scratchAddr := scratchBytes(t, 8)
	if hr := hresultCall(stream, streamWrite, scratchAddr, 8, h.countAt); hr != STGEAccessDenied {
		t.Fatalf("Write = %v, want STG_E_ACCESSDENIED", hr)
	}
	if hr := hresultCall(stream, streamSetSize, 10); hr != STGEInvalidFunction {
		t.Fatalf("SetSize = %v, want STG_E_INVALIDFUNCTION", hr)
	}
	if hr := hresultCall(stream, streamLockRegion, 0, 10, 1); hr != STGEInvalidFunction {
		t.Fatalf("LockRegion = %v, want STG_E_INVALIDFUNCTION", hr)
	}
	if hr := hresultCall(stream, streamUnlockRegion, 0, 10, 1); hr != STGEInvalidFunction {
		t.Fatalf("UnlockRegion = %v, want STG_E_INVALIDFUNCTION", hr)
	}
	if hr := hresultCall(stream, streamCommit, 0); hr != SOK {
		t.Fatalf("Commit = %v, want S_OK", hr)
	}
	if hr := hresultCall(stream, streamRevert); hr != SOK {
		t.Fatalf("Revert = %v, want S_OK", hr)
	}

	if released != 0 {
		t.Fatal("onRelease ran while the stream was still referenced")
	}
	Release(stream)
	if released != 1 {
		t.Fatalf("onRelease ran %d times, want 1", released)
	}
}

func TestCOM_IStreamCloneHasIndependentSeekPointer(t *testing.T) {
	requireNoLeak(t)
	data := patternBytes(64)
	released := 0
	stream, err := NewStream(bytes.NewReader(data), int64(len(data)), "", func() { released++ })
	if err != nil {
		t.Fatal(err)
	}
	h := newStreamHarness(t, stream)

	h.read(stream, 20)
	cloneOut, cloneAddr := scratch[uintptr](t)
	if hr := hresultCall(stream, streamClone, cloneAddr); hr != SOK || *cloneOut == 0 {
		t.Fatalf("Clone = %v, %#x", hr, *cloneOut)
	}
	clone := *cloneOut

	if got, hr := h.read(clone, 5); hr != SOK || !bytes.Equal(got, data[20:25]) {
		t.Fatalf("clone Read = %v, % x; want bytes 20..25 (clone starts at the original's position)", hr, got)
	}
	if got, hr := h.read(stream, 5); hr != SOK || !bytes.Equal(got, data[20:25]) {
		t.Fatalf("original Read after clone moved = %v, % x; want bytes 20..25", hr, got)
	}
	if got, hr := h.read(clone, 5); hr != SOK || !bytes.Equal(got, data[25:30]) {
		t.Fatalf("clone second Read = %v, % x; want bytes 25..30", hr, got)
	}

	Release(stream)
	if released != 0 {
		t.Fatal("onRelease ran while a clone was alive")
	}
	Release(clone)
	if released != 1 {
		t.Fatalf("onRelease ran %d times after the last clone, want 1", released)
	}
}

func TestCOM_IStreamSourceFailuresAreReported(t *testing.T) {
	requireNoLeak(t)
	data := patternBytes(32)

	flaky, err := NewStream(&flakyReader{Reader: bytes.NewReader(data), failAfter: 10}, 32, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer Release(flaky)
	h := newStreamHarness(t, flaky)
	if got, hr := h.read(flaky, 32); hr != STGEReadFault || len(got) != 10 {
		t.Fatalf("Read over a failing source = %v, %d bytes; want STG_E_READFAULT after 10 bytes", hr, len(got))
	}

	short, err := NewStream(bytes.NewReader(data[:8]), 32, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer Release(short)
	if _, hr := h.read(short, 32); hr != STGEReadFault {
		t.Fatalf("Read over a source shorter than the declared size = %v, want STG_E_READFAULT", hr)
	}
}

type writeSink struct {
	data []byte
}

func newWriteSink(t *testing.T) (*writeSink, uintptr) {
	t.Helper()
	sink := &writeSink{}
	unknown := unknownMethods()
	notImplemented := syscall.NewCallback(func(this uintptr) uintptr { return ENotImpl.ret() })
	write := syscall.NewCallback(func(this, pv, cb, pcbWritten uintptr) uintptr {
		target, ok := implOf[*writeSink](this)
		if !ok {
			return EUnexpected.ret()
		}
		target.data = append(target.data, unsafe.Slice((*byte)(ptr(pv)), int(uint32(cb)))...)
		putUint32(pcbWritten, uint32(cb))
		return SOK.ret()
	})
	vtbl, err := allocVtable([]uintptr{unknown[0], unknown[1], unknown[2], notImplemented, write})
	if err != nil {
		t.Fatal(err)
	}
	obj, err := createObject(vtbl, []windows.GUID{iidIStream}, sink, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { Release(obj) })
	return sink, obj
}

func TestCOM_IStreamCopyTo(t *testing.T) {
	requireNoLeak(t)
	data := patternBytes(3*copyChunkBytes/2 + 17)
	stream, err := NewStream(bytes.NewReader(data), int64(len(data)), "", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer Release(stream)
	sink, sinkObj := newWriteSink(t)

	counts, countsAddr := scratch[[2]uint64](t)
	if hr := hresultCall(stream, streamCopyTo, sinkObj, uintptr(math.MaxUint64), countsAddr, countsAddr+8); hr != SOK {
		t.Fatalf("CopyTo = %v", hr)
	}
	if !bytes.Equal(sink.data, data) || counts[0] != uint64(len(data)) || counts[1] != uint64(len(data)) {
		t.Fatalf("CopyTo moved %d bytes (read %d, written %d), want %d identical bytes", len(sink.data), counts[0], counts[1], len(data))
	}

	hresultCall(stream, streamSeek, seekArg(10), uintptr(streamSeekSet), 0)
	sink.data = nil
	if hr := hresultCall(stream, streamCopyTo, sinkObj, 25, 0, 0); hr != SOK || !bytes.Equal(sink.data, data[10:35]) {
		t.Fatalf("bounded CopyTo = %v, % x", hr, sink.data)
	}
	if hr := hresultCall(stream, streamCopyTo, 0, 1, 0, 0); hr != STGEInvalidPointer {
		t.Fatalf("CopyTo with nil destination = %v, want STG_E_INVALIDPOINTER", hr)
	}
}

func TestCOM_DataObjectQueryGetDataAndGetData(t *testing.T) {
	requireNoLeak(t)
	payload := []byte("virtual file bytes")
	var asked []FormatEtc
	source := &fakeSource{
		formats: []FormatEtc{hglobalFormat(200), {Format: 201, Aspect: DVAspectContent, Index: -1, Tymed: TymedIStream}},
		getData: func(f FormatEtc) (Medium, error) {
			asked = append(asked, f)
			switch f.Format {
			case 200:
				return NewHGlobalMedium(payload)
			default:
				return Medium{}, DVELindex
			}
		},
	}
	obj, err := NewDataObject(source)
	if err != nil {
		t.Fatal(err)
	}
	defer Release(obj)

	query := func(f FormatEtc) HResult {
		in, inAddr := scratch[FormatEtc](t)
		*in = f
		return hresultCall(obj, dataQueryGetData, inAddr)
	}
	if hr := query(hglobalFormat(200)); hr != SOK {
		t.Fatalf("QueryGetData(supported) = %v", hr)
	}
	if hr := query(hglobalFormat(999)); hr != DVEFormatEtc {
		t.Fatalf("QueryGetData(unknown format) = %v, want DV_E_FORMATETC", hr)
	}
	wrongTymed := hglobalFormat(201)
	if hr := query(wrongTymed); hr != DVETymed {
		t.Fatalf("QueryGetData(wrong tymed) = %v, want DV_E_TYMED", hr)
	}
	wrongAspect := hglobalFormat(200)
	wrongAspect.Aspect = 2
	if hr := query(wrongAspect); hr != DVEAspect {
		t.Fatalf("QueryGetData(wrong aspect) = %v, want DV_E_DVASPECT", hr)
	}
	if hr := hresultCall(obj, dataQueryGetData, 0); hr != EPointer {
		t.Fatalf("QueryGetData(nil) = %v, want E_POINTER", hr)
	}

	in, inAddr := scratch[FormatEtc](t)
	medium, mediumAddr := scratch[Medium](t)
	*in = hglobalFormat(200)
	*medium = Medium{Tymed: 0xFF, Handle: 1}
	if hr := hresultCall(obj, dataGetData, inAddr, mediumAddr); hr != SOK {
		t.Fatalf("GetData = %v", hr)
	}
	got, err := medium.HGlobalBytes()
	if err != nil || !bytes.Equal(got, payload) {
		t.Fatalf("GetData medium = %q, %v", got, err)
	}
	ReleaseMedium(medium)
	if *medium != (Medium{}) {
		t.Fatalf("ReleaseMedium left %+v", *medium)
	}

	*in = FormatEtc{Format: 201, Aspect: DVAspectContent, Index: 7, Tymed: TymedIStream}
	if hr := hresultCall(obj, dataGetData, inAddr, mediumAddr); hr != DVELindex || *medium != (Medium{}) {
		t.Fatalf("GetData with source HResult error = %v, medium %+v; want DV_E_LINDEX and an empty medium", hr, *medium)
	}
	if asked[len(asked)-1].Index != 7 {
		t.Fatalf("the source did not see lindex 7: %+v", asked)
	}

	*in = hglobalFormat(999)
	if hr := hresultCall(obj, dataGetData, inAddr, mediumAddr); hr != DVEFormatEtc {
		t.Fatalf("GetData(unknown) = %v, want DV_E_FORMATETC", hr)
	}
	if hr := hresultCall(obj, dataGetData, 0, mediumAddr); hr != EPointer {
		t.Fatalf("GetData(nil format) = %v, want E_POINTER", hr)
	}
	if hr := hresultCall(obj, dataGetData, inAddr, 0); hr != EPointer {
		t.Fatalf("GetData(nil medium) = %v, want E_POINTER", hr)
	}
}

func TestCOM_DataObjectGetDataErrorMapping(t *testing.T) {
	requireNoLeak(t)
	var result func() (Medium, error)
	source := &fakeSource{
		formats: []FormatEtc{hglobalFormat(300)},
		getData: func(FormatEtc) (Medium, error) { return result() },
	}
	obj, err := NewDataObject(source)
	if err != nil {
		t.Fatal(err)
	}
	defer Release(obj)
	in, inAddr := scratch[FormatEtc](t)
	medium, mediumAddr := scratch[Medium](t)
	*in = hglobalFormat(300)

	result = func() (Medium, error) { return Medium{}, errors.New("backend unavailable") }
	if hr := hresultCall(obj, dataGetData, inAddr, mediumAddr); hr != EFail {
		t.Fatalf("plain error mapped to %v, want E_FAIL", hr)
	}
	result = func() (Medium, error) { return Medium{}, errors.Join(errors.New("context"), DVELindex) }
	if hr := hresultCall(obj, dataGetData, inAddr, mediumAddr); hr != DVELindex {
		t.Fatalf("wrapped HResult mapped to %v, want DV_E_LINDEX", hr)
	}
	result = func() (Medium, error) { return NewHGlobalMedium([]byte{1}) }
	*in = FormatEtc{Format: 300, Aspect: DVAspectContent, Index: -1, Tymed: TymedHGlobal | TymedIStream}
	if hr := hresultCall(obj, dataGetData, inAddr, mediumAddr); hr != SOK {
		t.Fatalf("GetData = %v", hr)
	}
	ReleaseMedium(medium)

	result = func() (Medium, error) { return Medium{Tymed: TymedIStream}, nil }
	*in = hglobalFormat(300)
	if hr := hresultCall(obj, dataGetData, inAddr, mediumAddr); hr != DVETymed {
		t.Fatalf("a medium of an unrequested type mapped to %v, want DV_E_TYMED", hr)
	}
}

func TestCOM_DataObjectSetData(t *testing.T) {
	requireNoLeak(t)
	var seenFormat FormatEtc
	var seenBytes []byte
	var setErr error
	source := &fakeSource{
		formats: []FormatEtc{hglobalFormat(400)},
		setData: func(f FormatEtc, m Medium) error {
			seenFormat = f
			if setErr != nil {
				return setErr
			}
			var err error
			seenBytes, err = m.HGlobalBytes()
			return err
		},
	}
	obj, err := NewDataObject(source)
	if err != nil {
		t.Fatal(err)
	}
	defer Release(obj)
	in, inAddr := scratch[FormatEtc](t)
	medium, mediumAddr := scratch[Medium](t)
	*in = hglobalFormat(400)

	*medium, err = NewHGlobalMedium([]byte{4, 0, 0, 0})
	if err != nil {
		t.Fatal(err)
	}
	const garbageHighBits = uintptr(1) << 40
	if hr := hresultCall(obj, dataSetData, inAddr, mediumAddr, garbageHighBits); hr != SOK {
		t.Fatalf("SetData(fRelease=FALSE) = %v", hr)
	}
	if medium.Tymed != TymedHGlobal || !bytes.Equal(seenBytes, []byte{4, 0, 0, 0}) || seenFormat.Format != 400 {
		t.Fatalf("fRelease=FALSE must leave the medium to the caller: %+v, bytes % x", *medium, seenBytes)
	}

	if hr := hresultCall(obj, dataSetData, inAddr, mediumAddr, 1); hr != SOK {
		t.Fatalf("SetData(fRelease=TRUE) = %v", hr)
	}
	if *medium != (Medium{}) {
		t.Fatalf("fRelease=TRUE must release the medium, left %+v", *medium)
	}

	*medium, err = NewHGlobalMedium([]byte{1})
	if err != nil {
		t.Fatal(err)
	}
	setErr = DVEFormatEtc
	if hr := hresultCall(obj, dataSetData, inAddr, mediumAddr, 1); hr != DVEFormatEtc {
		t.Fatalf("failing SetData = %v, want DV_E_FORMATETC", hr)
	}
	if medium.Tymed != TymedHGlobal {
		t.Fatal("a failed SetData must not take ownership of the medium")
	}
	ReleaseMedium(medium)

	if hr := hresultCall(obj, dataSetData, 0, mediumAddr, 0); hr != EPointer {
		t.Fatalf("SetData(nil format) = %v, want E_POINTER", hr)
	}
}

func TestCOM_DataObjectEnumAndUnsupportedMethods(t *testing.T) {
	requireNoLeak(t)
	formats := enumTestFormats()
	obj, err := NewDataObject(&fakeSource{formats: formats})
	if err != nil {
		t.Fatal(err)
	}
	defer Release(obj)

	listed, err := EnumFormats(obj)
	if err != nil || !slices.Equal(listed, formats) {
		t.Fatalf("EnumFormats = %v, %v; want %v", listed, err, formats)
	}

	out, outAddr := scratch[uintptr](t)
	*out = 0xDEAD
	if hr := hresultCall(obj, dataEnumFormatEtc, uintptr(dataDirSet), outAddr); hr != ENotImpl || *out != 0 {
		t.Fatalf("EnumFormatEtc(SET) = %v, out %#x; want E_NOTIMPL and nil", hr, *out)
	}
	if hr := hresultCall(obj, dataEnumFormatEtc, 3, outAddr); hr != EInvalidArg {
		t.Fatalf("EnumFormatEtc(3) = %v, want E_INVALIDARG", hr)
	}
	if hr := hresultCall(obj, dataEnumFormatEtc, uintptr(dataDirGet), 0); hr != EPointer {
		t.Fatalf("EnumFormatEtc(nil out) = %v, want E_POINTER", hr)
	}

	in, inAddr := scratch[FormatEtc](t)
	canonical, canonicalAddr := scratch[FormatEtc](t)
	*in = hglobalFormat(101)
	in.TargetDevice = 0x1234
	if hr := hresultCall(obj, dataGetCanonicalFormatEtc, inAddr, canonicalAddr); hr != DataSSameFormatEtc {
		t.Fatalf("GetCanonicalFormatEtc = %v, want DATA_S_SAMEFORMATETC", hr)
	}
	if canonical.Format != 101 || canonical.TargetDevice != 0 {
		t.Fatalf("canonical format = %+v, want format 101 with a nil target device", *canonical)
	}
	_, mediumAddr := scratch[Medium](t)
	if hr := hresultCall(obj, dataGetDataHere, inAddr, mediumAddr); hr != ENotImpl {
		t.Fatalf("GetDataHere = %v, want E_NOTIMPL", hr)
	}

	connection, connectionAddr := scratch[uint32](t)
	*connection = 99
	if hr := hresultCall(obj, dataDAdvise, inAddr, 0, 0, connectionAddr); hr != OleEAdviseNotSupported || *connection != 0 {
		t.Fatalf("DAdvise = %v, connection %d", hr, *connection)
	}
	if hr := hresultCall(obj, dataDUnadvise, 1); hr != OleEAdviseNotSupported {
		t.Fatalf("DUnadvise = %v", hr)
	}
	*out = 0xDEAD
	if hr := hresultCall(obj, dataEnumDAdvise, outAddr); hr != OleEAdviseNotSupported || *out != 0 {
		t.Fatalf("EnumDAdvise = %v, out %#x", hr, *out)
	}
}

type recordingTarget struct {
	enterData uintptr
	keyState  uint32
	point     Point
	effectIn  uint32
	leaves    int
	err       error
	effect    uint32
}

func (r *recordingTarget) DragEnter(data uintptr, keyState uint32, pt Point, effect uint32) (uint32, error) {
	r.enterData, r.keyState, r.point, r.effectIn = data, keyState, pt, effect
	return r.effect, r.err
}

func (r *recordingTarget) DragOver(keyState uint32, pt Point, effect uint32) (uint32, error) {
	r.keyState, r.point, r.effectIn = keyState, pt, effect
	return r.effect, r.err
}

func (r *recordingTarget) DragLeave() error {
	r.leaves++
	return r.err
}

func (r *recordingTarget) Drop(data uintptr, keyState uint32, pt Point, effect uint32) (uint32, error) {
	r.enterData, r.keyState, r.point, r.effectIn = data, keyState, pt, effect
	return r.effect, r.err
}

func packPoint(x, y int32) uintptr {
	return uintptr(uint32(x)) | uintptr(uint32(y))<<32
}

func TestCOM_DropTargetForwardsArgumentsAndEffect(t *testing.T) {
	requireNoLeak(t)
	target := &recordingTarget{effect: DropEffectCopy}
	obj, err := NewDropTarget(target)
	if err != nil {
		t.Fatal(err)
	}
	defer Release(obj)
	effect, effectAddr := scratch[uint32](t)

	*effect = DropEffectCopy | DropEffectMove
	garbageHigh := uintptr(0xABCD) << 32
	if hr := hresultCall(obj, dropTargetDragEnter, 0x7777, 8|garbageHigh, packPoint(-20, 300), effectAddr); hr != SOK {
		t.Fatalf("DragEnter = %v", hr)
	}
	if target.enterData != 0x7777 || target.keyState != 8 || target.point != (Point{-20, 300}) || target.effectIn != DropEffectCopy|DropEffectMove || *effect != DropEffectCopy {
		t.Fatalf("DragEnter forwarded %+v, effect out %d", *target, *effect)
	}

	target.effect = DropEffectMove
	if hr := hresultCall(obj, dropTargetDragOver, 4, packPoint(5, -6), effectAddr); hr != SOK || *effect != DropEffectMove || target.point != (Point{5, -6}) {
		t.Fatalf("DragOver = %v, effect %d, point %+v", hr, *effect, target.point)
	}
	if hr := hresultCall(obj, dropTargetDrop, 0x8888, 0, packPoint(1, 2), effectAddr); hr != SOK || target.enterData != 0x8888 {
		t.Fatalf("Drop = %v, data %#x", hr, target.enterData)
	}
	if hr := hresultCall(obj, dropTargetDragLeave); hr != SOK || target.leaves != 1 {
		t.Fatalf("DragLeave = %v, leaves %d", hr, target.leaves)
	}

	target.err = errors.New("refused")
	*effect = DropEffectCopy
	if hr := hresultCall(obj, dropTargetDrop, 0x8888, 0, packPoint(1, 2), effectAddr); hr != EFail || *effect != DropEffectNone {
		t.Fatalf("failing Drop = %v, effect %d; want E_FAIL and DROPEFFECT_NONE", hr, *effect)
	}
	if hr := hresultCall(obj, dropTargetDragLeave); hr != EFail {
		t.Fatalf("failing DragLeave = %v, want E_FAIL", hr)
	}
	if hr := hresultCall(obj, dropTargetDragEnter, 1, 0, 0, 0); hr != EPointer {
		t.Fatalf("DragEnter with nil effect pointer = %v, want E_POINTER", hr)
	}
}

type recordingSource struct {
	escape   bool
	keyState uint32
	effect   uint32
	verdict  HResult
}

func (r *recordingSource) QueryContinueDrag(escapePressed bool, keyState uint32) HResult {
	r.escape, r.keyState = escapePressed, keyState
	return r.verdict
}

func (r *recordingSource) GiveFeedback(effect uint32) HResult {
	r.effect = effect
	return DragDropSUseDefaultCur
}

func TestCOM_DropSourceForwardsArguments(t *testing.T) {
	requireNoLeak(t)
	source := &recordingSource{verdict: DragDropSDrop}
	obj, err := NewDropSource(source)
	if err != nil {
		t.Fatal(err)
	}
	defer Release(obj)

	garbageHigh := uintptr(0xABCD) << 32
	if hr := hresultCall(obj, dropSourceQueryContinueDrag, 1|garbageHigh, 3); hr != DragDropSDrop || !source.escape || source.keyState != 3 {
		t.Fatalf("QueryContinueDrag = %v, escape %v, keys %d", hr, source.escape, source.keyState)
	}
	if hr := hresultCall(obj, dropSourceQueryContinueDrag, garbageHigh, 0); source.escape || hr != DragDropSDrop {
		t.Fatalf("QueryContinueDrag(FALSE with garbage high bits) = %v, escape %v", hr, source.escape)
	}
	if hr := hresultCall(obj, dropSourceGiveFeedback, uintptr(DropEffectLink)); hr != DragDropSUseDefaultCur || source.effect != DropEffectLink {
		t.Fatalf("GiveFeedback = %v, effect %d", hr, source.effect)
	}
}

func TestCOM_CallbackBudget(t *testing.T) {
	objectsAlive := ObjectCount()
	var created []uintptr
	for range 20 {
		for _, build := range []func() (uintptr, error){
			func() (uintptr, error) { return NewDataObject(&fakeSource{}) },
			func() (uintptr, error) { return NewFormatEnumerator(nil) },
			func() (uintptr, error) { return NewStream(bytes.NewReader(nil), 0, "", nil) },
			func() (uintptr, error) { return NewDropSource(&recordingSource{}) },
			func() (uintptr, error) { return NewDropTarget(&recordingTarget{}) },
		} {
			obj, err := build()
			if err != nil {
				t.Fatal(err)
			}
			created = append(created, obj)
		}
	}
	after := CallbackCount()
	if after > callbackBudget {
		t.Fatalf("created %d callbacks, budget is %d", after, callbackBudget)
	}
	extra, err := NewDataObject(&fakeSource{})
	if err != nil {
		t.Fatal(err)
	}
	created = append(created, extra)
	if CallbackCount() != after {
		t.Fatalf("callbacks grew from %d to %d when creating one more object; they must be created once per method", after, CallbackCount())
	}
	for _, obj := range created {
		Release(obj)
	}
	if ObjectCount() != objectsAlive {
		t.Fatalf("hosted objects alive: %d, want %d", ObjectCount(), objectsAlive)
	}
	t.Logf("callbacks created: %d (budget %d)", after, callbackBudget)
}

func TestCOM_ConcurrentReferenceCounting(t *testing.T) {
	requireNoLeak(t)
	obj, err := NewDataObject(&fakeSource{})
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 200 {
				AddRef(obj)
				Release(obj)
			}
		}()
	}
	wg.Wait()
	if got := Release(obj); got != 0 {
		t.Fatalf("final Release = %d, want 0 after balanced concurrent AddRef/Release", got)
	}
}

func newTestSTA() *STA {
	return &STA{initialize: func() error { return nil }, uninitialize: func() {}}
}

func TestSTA_DoRunsOnOneDedicatedThread(t *testing.T) {
	sta := newTestSTA()
	if err := sta.Start(); err != nil {
		t.Fatal(err)
	}
	if err := sta.Start(); !errors.Is(err, ErrSTAAlreadyStarted) {
		t.Fatalf("second Start = %v, want ErrSTAAlreadyStarted", err)
	}

	var first, second uint32
	if err := sta.Do(func() { first = windows.GetCurrentThreadId() }); err != nil {
		t.Fatal(err)
	}
	if err := sta.Do(func() { second = windows.GetCurrentThreadId() }); err != nil {
		t.Fatal(err)
	}
	if first == 0 || first != second || first == windows.GetCurrentThreadId() {
		t.Fatalf("Do ran on threads %d and %d (caller %d); want one dedicated thread", first, second, windows.GetCurrentThreadId())
	}

	nested := 0
	if err := sta.Do(func() {
		if err := sta.Do(func() { nested++ }); err != nil {
			t.Errorf("nested Do = %v", err)
		}
		if err := sta.Close(); !errors.Is(err, ErrSTACloseOnThread) {
			t.Errorf("Close from the STA thread = %v, want ErrSTACloseOnThread", err)
		}
	}); err != nil || nested != 1 {
		t.Fatalf("nested Do = %v, ran %d times", err, nested)
	}

	if err := sta.Close(); err != nil {
		t.Fatal(err)
	}
	if err := sta.Do(func() {}); !errors.Is(err, ErrSTANotRunning) {
		t.Fatalf("Do after Close = %v, want ErrSTANotRunning", err)
	}
	if err := sta.Close(); err != nil {
		t.Fatalf("second Close = %v, want nil", err)
	}
}

func TestSTA_ConcurrentDoIsSerialised(t *testing.T) {
	sta := newTestSTA()
	if err := sta.Start(); err != nil {
		t.Fatal(err)
	}
	counter := 0
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 50 {
				if err := sta.Do(func() { counter++ }); err != nil {
					t.Errorf("Do = %v", err)
					return
				}
			}
		}()
	}
	wg.Wait()
	if err := sta.Close(); err != nil {
		t.Fatal(err)
	}
	if counter != 400 {
		t.Fatalf("counter = %d, want 400", counter)
	}
}

func TestSTA_InitializeFailureAndUninitialize(t *testing.T) {
	failure := errors.New("apartment refused")
	failing := &STA{initialize: func() error { return failure }, uninitialize: func() { t.Error("uninitialize after a failed initialize") }}
	if err := failing.Start(); !errors.Is(err, failure) {
		t.Fatalf("Start = %v, want %v", err, failure)
	}
	if err := failing.Do(func() {}); !errors.Is(err, ErrSTANotRunning) {
		t.Fatalf("Do after failed Start = %v, want ErrSTANotRunning", err)
	}
	if err := failing.Close(); err != nil {
		t.Fatalf("Close after failed Start = %v", err)
	}

	uninitialised := make(chan struct{}, 1)
	sta := &STA{initialize: func() error { return nil }, uninitialize: func() { uninitialised <- struct{}{} }}
	if err := sta.Start(); err != nil {
		t.Fatal(err)
	}
	if err := sta.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-uninitialised:
	default:
		t.Fatal("Close returned before the STA thread uninitialised")
	}

	idle := &STA{}
	if err := idle.Do(func() {}); !errors.Is(err, ErrSTANotRunning) {
		t.Fatalf("Do on an unstarted STA = %v, want ErrSTANotRunning", err)
	}
	if err := idle.Close(); err != nil {
		t.Fatalf("Close on an unstarted STA = %v", err)
	}
}

var _ io.ReadSeeker = (*flakyReader)(nil)
