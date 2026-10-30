//go:build windows

package main

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"log/slog"
	"strings"
	"syscall"
	"testing"
	"testing/synctest"
	"time"
	"unsafe"

	"github.com/kikz0r/edgesync/spikes/internal/wincom"
)

const (
	vtblStreamRead   = 3
	vtblDataSetData  = 7
	testChunk        = 64 << 10
	testLargeSize    = 300_000
	testSmallSize    = 10 << 10
	descriptorLength = 592
)

var testModTime = time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)

func testEntries() []entry {
	return []entry{
		{Path: "demo", Dir: true, ModTime: testModTime},
		{Path: "demo/a.bin", Size: testLargeSize, ModTime: testModTime},
		{Path: "b.txt", Size: testSmallSize, ModTime: testModTime},
	}
}

func expectedContent(start, length int) []byte {
	out := make([]byte, length)
	for i := range out {
		out[i] = byte((int64(start+i)*31 + 7) % 256)
	}
	return out
}

func newTestSource(t *testing.T, opts options) *syntheticSource {
	t.Helper()
	source, err := newSyntheticSource(testEntries(), opts)
	if err != nil {
		t.Fatal(err)
	}
	return source
}

func mustOpen(t *testing.T, source *syntheticSource, index int) *syntheticReader {
	t.Helper()
	reader, err := source.Open(index)
	if err != nil {
		t.Fatal(err)
	}
	return reader
}

func TestSource_ThrottledReadRespectsRate(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		const (
			size    = 2 << 20
			rate    = 1 << 20
			latency = 250 * time.Millisecond
		)
		source, err := newSyntheticSource([]entry{{Path: "f.bin", Size: size, ModTime: testModTime}}, options{
			rateBytesPerSec: rate,
			latency:         latency,
		})
		if err != nil {
			t.Fatal(err)
		}
		reader := mustOpen(t, source, 0)
		start := time.Now()
		buf := make([]byte, testChunk)

		if _, err := io.ReadFull(reader, buf); err != nil {
			t.Fatal(err)
		}
		if got, want := time.Since(start), latency+62500*time.Microsecond; got != want {
			t.Fatalf("first 64 KiB took %v, want latency plus 62.5 ms = %v", got, want)
		}

		if _, err := io.Copy(io.Discard, reader); err != nil {
			t.Fatal(err)
		}
		if got, want := time.Since(start), latency+2*time.Second; got != want {
			t.Fatalf("2 MiB at 1 MiB/s took %v, want %v", got, want)
		}
		summary := reader.Close()
		if !summary.Completed || summary.BytesServed != size {
			t.Fatalf("summary = %+v, want completed with %d bytes", summary, size)
		}
		if want := latency + 62500*time.Microsecond; summary.FirstByteAfter != want {
			t.Fatalf("first byte after %v, want %v", summary.FirstByteAfter, want)
		}
	})
}

func TestSource_PauseDoesNotBuyBurst(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		source, err := newSyntheticSource([]entry{{Path: "f.bin", Size: 1 << 20, ModTime: testModTime}}, options{rateBytesPerSec: 1 << 20})
		if err != nil {
			t.Fatal(err)
		}
		reader := mustOpen(t, source, 0)
		buf := make([]byte, testChunk)
		if _, err := io.ReadFull(reader, buf); err != nil {
			t.Fatal(err)
		}
		time.Sleep(10 * time.Second)
		before := time.Now()
		if _, err := io.ReadFull(reader, buf); err != nil {
			t.Fatal(err)
		}
		if got, want := time.Since(before), 62500*time.Microsecond; got != want {
			t.Fatalf("chunk after a pause took %v, want the full %v", got, want)
		}
	})
}

func TestSource_UnthrottledReadIsInstant(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		reader := mustOpen(t, newTestSource(t, options{}), 1)
		start := time.Now()
		if _, err := io.Copy(io.Discard, reader); err != nil {
			t.Fatal(err)
		}
		if got := time.Since(start); got != 0 {
			t.Fatalf("unthrottled read took %v", got)
		}
	})
}

func TestSource_BlockReadHappensOnce(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		source := newTestSource(t, options{blockRead: 3 * time.Second})
		first := mustOpen(t, source, 1)
		second := mustOpen(t, source, 2)
		buf := make([]byte, 16)

		start := time.Now()
		if _, err := first.Read(buf); err != nil {
			t.Fatal(err)
		}
		if got := time.Since(start); got != 3*time.Second {
			t.Fatalf("first read blocked %v, want 3s", got)
		}
		start = time.Now()
		if _, err := first.Read(buf); err != nil {
			t.Fatal(err)
		}
		if _, err := second.Read(buf); err != nil {
			t.Fatal(err)
		}
		if got := time.Since(start); got != 0 {
			t.Fatalf("later reads blocked %v, want none", got)
		}
	})
}

func TestSource_SeekAndRange(t *testing.T) {
	source := newTestSource(t, options{})
	reader := mustOpen(t, source, 1)
	size := int64(testLargeSize)

	if pos, err := reader.Seek(1000, io.SeekStart); err != nil || pos != 1000 {
		t.Fatalf("Seek(1000, Start) = %d, %v", pos, err)
	}
	if pos, err := reader.Seek(24, io.SeekCurrent); err != nil || pos != 1024 {
		t.Fatalf("Seek(24, Current) = %d, %v", pos, err)
	}
	buf := make([]byte, 700)
	if n, err := io.ReadFull(reader, buf); err != nil || n != len(buf) {
		t.Fatalf("read = %d, %v", n, err)
	}
	if !bytes.Equal(buf, expectedContent(1024, len(buf))) {
		t.Fatal("content after Seek does not follow byte(offset*31+7)")
	}

	if pos, err := reader.Seek(-3, io.SeekEnd); err != nil || pos != size-3 {
		t.Fatalf("Seek(-3, End) = %d, %v", pos, err)
	}
	tail := make([]byte, 10)
	n, err := reader.Read(tail)
	if err != nil || n != 3 || !bytes.Equal(tail[:n], expectedContent(int(size)-3, 3)) {
		t.Fatalf("tail read = %d bytes %v, %v", n, tail[:n], err)
	}
	if n, err := reader.Read(tail); n != 0 || !errors.Is(err, io.EOF) {
		t.Fatalf("read at end = %d, %v; want 0, EOF", n, err)
	}

	if _, err := reader.Seek(-1, io.SeekStart); !errors.Is(err, ErrNegativeOffset) {
		t.Fatalf("Seek(-1, Start) error = %v, want ErrNegativeOffset", err)
	}
	if _, err := reader.Seek(0, 99); !errors.Is(err, ErrInvalidWhence) {
		t.Fatalf("Seek with whence 99 error = %v, want ErrInvalidWhence", err)
	}
	if pos, err := reader.Seek(size+500, io.SeekStart); err != nil || pos != size+500 {
		t.Fatalf("seek past end = %d, %v; a position beyond the end is legal", pos, err)
	}
	if n, err := reader.Read(tail); n != 0 || !errors.Is(err, io.EOF) {
		t.Fatalf("read past end = %d, %v; want 0, EOF", n, err)
	}
}

func TestSource_ReadAtRanges(t *testing.T) {
	reader := mustOpen(t, newTestSource(t, options{}), 1)
	size := int64(testLargeSize)

	mid := make([]byte, 128)
	if n, err := reader.ReadAt(mid, 150_000); err != nil || n != len(mid) {
		t.Fatalf("ReadAt mid = %d, %v", n, err)
	}
	if !bytes.Equal(mid, expectedContent(150_000, len(mid))) {
		t.Fatal("ReadAt content mismatch")
	}
	if pos, _ := reader.Seek(0, io.SeekCurrent); pos != 0 {
		t.Fatalf("ReadAt moved the position to %d", pos)
	}

	straddle := make([]byte, 100)
	n, err := reader.ReadAt(straddle, size-40)
	if n != 40 || !errors.Is(err, io.EOF) || !bytes.Equal(straddle[:n], expectedContent(int(size)-40, 40)) {
		t.Fatalf("ReadAt across the end = %d, %v; want 40 bytes and EOF", n, err)
	}
	if n, err := reader.ReadAt(straddle, size); n != 0 || !errors.Is(err, io.EOF) {
		t.Fatalf("ReadAt at the end = %d, %v; want 0, EOF", n, err)
	}
	if _, err := reader.ReadAt(straddle, -1); !errors.Is(err, ErrNegativeOffset) {
		t.Fatalf("ReadAt(-1) error = %v, want ErrNegativeOffset", err)
	}
}

func TestSource_ContentIsDeterministicAtLargeOffsets(t *testing.T) {
	source, err := newSyntheticSource([]entry{{Path: "big.bin", Size: demoLargeFileSize, ModTime: testModTime}}, options{})
	if err != nil {
		t.Fatal(err)
	}
	reader := mustOpen(t, source, 0)
	buf := make([]byte, 64)
	offset := int64(demoLargeFileSize - len(buf))
	if _, err := reader.ReadAt(buf, offset); err != nil && !errors.Is(err, io.EOF) {
		t.Fatal(err)
	}
	for i, got := range buf {
		if want := contentByte(offset + int64(i)); got != want {
			t.Fatalf("byte %d = %d, want %d", offset+int64(i), got, want)
		}
	}
}

func TestSource_AccountingCompletedAndAborted(t *testing.T) {
	source := newTestSource(t, options{})
	aborted := mustOpen(t, source, 1)
	if _, err := io.ReadFull(aborted, make([]byte, 100)); err != nil {
		t.Fatal(err)
	}
	if summary := aborted.Close(); summary.Completed || summary.BytesServed != 100 || summary.Reads != 1 {
		t.Fatalf("aborted summary = %+v", summary)
	}
	if again := aborted.Close(); again.BytesServed != 100 {
		t.Fatalf("second Close changed the summary: %+v", again)
	}

	full := mustOpen(t, source, 1)
	if _, err := io.Copy(io.Discard, full); err != nil {
		t.Fatal(err)
	}
	if summary := full.Close(); !summary.Completed || summary.BytesServed != testLargeSize {
		t.Fatalf("completed summary = %+v", summary)
	}

	untouched := mustOpen(t, source, 2)
	stats := source.Stats()
	if stats[1].Opens != 2 || stats[1].Aborted != 1 || stats[1].Completed != 1 || stats[1].BytesServed != 100+testLargeSize {
		t.Fatalf("stats for the large file = %+v", stats[1])
	}
	if stats[2].Opens != 1 || stats[2].BytesServed != 0 || stats[2].Reads != 0 {
		t.Fatalf("an opened but unread file must account zero bytes: %+v", stats[2])
	}
	if _, err := untouched.Read(make([]byte, 1)); err != nil {
		t.Fatal(err)
	}
	untouched.Close()
	if _, err := untouched.Read(make([]byte, 1)); !errors.Is(err, ErrReaderClosed) {
		t.Fatalf("read after Close error = %v, want ErrReaderClosed", err)
	}
}

func TestSource_OpenRejectsDirectoryAndBadIndex(t *testing.T) {
	source := newTestSource(t, options{})
	if _, err := source.Open(0); !errors.Is(err, ErrDirectory) {
		t.Fatalf("Open(directory) error = %v, want ErrDirectory", err)
	}
	for _, index := range []int{-1, 3, 99} {
		if _, err := source.Open(index); !errors.Is(err, ErrIndexOutOfRange) {
			t.Fatalf("Open(%d) error = %v, want ErrIndexOutOfRange", index, err)
		}
	}
	if opens := source.Stats()[0].Opens; opens != 0 {
		t.Fatalf("a refused open was accounted: %d", opens)
	}
}

func TestSource_InvalidOptions(t *testing.T) {
	for name, opts := range map[string]options{
		"negative rate":    {rateBytesPerSec: -1},
		"rate overflow":    {rateBytesPerSec: maxRateBytes + 1},
		"negative latency": {latency: -time.Second},
		"negative block":   {blockRead: -time.Second},
	} {
		if _, err := newSyntheticSource(testEntries(), opts); !errors.Is(err, ErrInvalidOptions) {
			t.Errorf("%s: error = %v, want ErrInvalidOptions", name, err)
		}
	}
}

func TestDescriptor_StructSizes(t *testing.T) {
	if got := unsafe.Sizeof(wincom.FileDescriptorW{}); got != descriptorLength {
		t.Fatalf("FILEDESCRIPTORW is %d bytes, want %d", got, descriptorLength)
	}
	if got := unsafe.Sizeof(wincom.FormatEtc{}); got != 32 {
		t.Fatalf("FORMATETC is %d bytes, want 32", got)
	}
	if got := unsafe.Sizeof(wincom.Medium{}); got != 24 {
		t.Fatalf("STGMEDIUM is %d bytes, want 24", got)
	}
}

func decodeUTF16Name(raw []byte) string {
	units := make([]uint16, 0, len(raw)/2)
	for i := 0; i+1 < len(raw); i += 2 {
		unit := binary.LittleEndian.Uint16(raw[i:])
		if unit == 0 {
			break
		}
		units = append(units, unit)
	}
	return syscall.UTF16ToString(units)
}

func TestDescriptor_LayoutAndFlags(t *testing.T) {
	data, err := buildFileGroupDescriptor(testEntries())
	if err != nil {
		t.Fatal(err)
	}
	if want := 4 + 3*descriptorLength; len(data) != want {
		t.Fatalf("descriptor is %d bytes, want %d", len(data), want)
	}
	if count := binary.LittleEndian.Uint32(data); count != 3 {
		t.Fatalf("cItems = %d, want 3", count)
	}
	const wantFlags = 0x4 | 0x40 | 0x20 | 0x4000 | 0x80000000
	wantNames := []string{"demo", `demo\a.bin`, "b.txt"}
	wantAttributes := []uint32{0x10, 0x80, 0x80}
	wantSizes := []uint64{0, testLargeSize, testSmallSize}
	wantWrite := uint64(testModTime.Unix()+11_644_473_600) * 10_000_000

	for i := range 3 {
		item := data[4+i*descriptorLength:][:descriptorLength]
		if flags := binary.LittleEndian.Uint32(item[0:]); flags != wantFlags {
			t.Errorf("entry %d flags = %#x, want %#x", i, flags, uint32(wantFlags))
		}
		if attrs := binary.LittleEndian.Uint32(item[36:]); attrs != wantAttributes[i] {
			t.Errorf("entry %d attributes = %#x, want %#x", i, attrs, wantAttributes[i])
		}
		write := uint64(binary.LittleEndian.Uint32(item[60:]))<<32 | uint64(binary.LittleEndian.Uint32(item[56:]))
		if write != wantWrite {
			t.Errorf("entry %d last write ticks = %d, want %d", i, write, wantWrite)
		}
		size := uint64(binary.LittleEndian.Uint32(item[64:]))<<32 | uint64(binary.LittleEndian.Uint32(item[68:]))
		if size != wantSizes[i] {
			t.Errorf("entry %d size = %d, want %d", i, size, wantSizes[i])
		}
		if name := decodeUTF16Name(item[72:]); name != wantNames[i] {
			t.Errorf("entry %d name = %q, want %q", i, name, wantNames[i])
		}
	}
}

func TestDescriptor_SizeSplitHighLow(t *testing.T) {
	const size = 5<<30 + 123
	data, err := buildFileGroupDescriptor([]entry{{Path: "huge.bin", Size: size, ModTime: testModTime}})
	if err != nil {
		t.Fatal(err)
	}
	item := data[4:]
	if high, low := binary.LittleEndian.Uint32(item[64:]), binary.LittleEndian.Uint32(item[68:]); high != 1 || low != (size&0xFFFFFFFF) {
		t.Fatalf("high = %d, low = %d for size %d", high, low, int64(size))
	}
}

func TestDescriptor_UnicodeNames(t *testing.T) {
	data, err := buildFileGroupDescriptor([]entry{{Path: "dir/čučoriedka 🍇.txt", Size: 1, ModTime: testModTime}})
	if err != nil {
		t.Fatal(err)
	}
	if name := decodeUTF16Name(data[4+72:]); name != `dir\čučoriedka 🍇.txt` {
		t.Fatalf("name = %q", name)
	}
}

func TestDescriptor_RejectsNotRepresentable(t *testing.T) {
	longName := strings.Repeat("a", 260)
	cases := map[string]entry{
		"empty path":          {Path: "", Size: 1, ModTime: testModTime},
		"absolute":            {Path: "/etc/passwd", Size: 1, ModTime: testModTime},
		"empty component":     {Path: "a//b", Size: 1, ModTime: testModTime},
		"trailing slash":      {Path: "a/", Size: 1, ModTime: testModTime},
		"dot-dot":             {Path: "a/../b", Size: 1, ModTime: testModTime},
		"dot":                 {Path: "./a", Size: 1, ModTime: testModTime},
		"alternate stream":    {Path: "file.txt:stream", Size: 1, ModTime: testModTime},
		"drive letter":        {Path: "C:/x", Size: 1, ModTime: testModTime},
		"angle bracket":       {Path: "a<b", Size: 1, ModTime: testModTime},
		"pipe":                {Path: "a|b", Size: 1, ModTime: testModTime},
		"question mark":       {Path: "a?b", Size: 1, ModTime: testModTime},
		"asterisk":            {Path: "a*b", Size: 1, ModTime: testModTime},
		"quote":               {Path: `a"b`, Size: 1, ModTime: testModTime},
		"backslash":           {Path: `a\b`, Size: 1, ModTime: testModTime},
		"NUL":                 {Path: "a\x00b", Size: 1, ModTime: testModTime},
		"tab":                 {Path: "a\tb", Size: 1, ModTime: testModTime},
		"invalid UTF-8":       {Path: "a\xffb", Size: 1, ModTime: testModTime},
		"260 units":           {Path: longName, Size: 1, ModTime: testModTime},
		"negative size":       {Path: "a", Size: -1, ModTime: testModTime},
		"directory with size": {Path: "a", Dir: true, Size: 1, ModTime: testModTime},
		"zero time":           {Path: "a", Size: 1},
		"time before 1601":    {Path: "a", Size: 1, ModTime: time.Date(1500, 1, 1, 0, 0, 0, 0, time.UTC)},
	}
	for name, e := range cases {
		_, err := buildFileGroupDescriptor([]entry{{Path: "ok", Size: 1, ModTime: testModTime}, e})
		if !errors.Is(err, ErrNotRepresentable) {
			t.Errorf("%s: error = %v, want ErrNotRepresentable", name, err)
			continue
		}
		if !strings.Contains(err.Error(), "entry 1") {
			t.Errorf("%s: error %q does not name the entry index", name, err)
		}
	}
}

func TestDescriptor_Accepts259Units(t *testing.T) {
	name := strings.Repeat("a", 259)
	if _, err := buildFileGroupDescriptor([]entry{{Path: name, Size: 1, ModTime: testModTime}}); err != nil {
		t.Fatalf("259 UTF-16 units must be accepted: %v", err)
	}
}

func pointerAt(addr uintptr) unsafe.Pointer {
	return *(*unsafe.Pointer)(unsafe.Pointer(&addr))
}

// callMethod invokes a COM method by vtable index. uintptrescapes keeps pointer arguments on the heap while the
// hosted callee re-enters Go.
//
//go:uintptrescapes
func callMethod(obj uintptr, index int, args ...uintptr) wincom.HResult {
	vtbl := *(*uintptr)(pointerAt(obj))
	method := *(*uintptr)(pointerAt(vtbl + uintptr(index)*unsafe.Sizeof(uintptr(0))))
	ret, _, _ := syscall.SyscallN(method, append([]uintptr{obj}, args...)...)
	return wincom.HResult(uint32(ret))
}

type hostedData struct {
	t       *testing.T
	object  uintptr
	data    *clipboardData
	source  *syntheticSource
	formats shellFormats
	log     *bytes.Buffer
}

func newHostedData(t *testing.T) *hostedData {
	t.Helper()
	baseline := wincom.ObjectCount()
	formats, err := registerShellFormats()
	if err != nil {
		t.Fatal(err)
	}
	source := newTestSource(t, options{})
	logs := &bytes.Buffer{}
	data, err := newClipboardData(source, formats, slog.New(slog.NewTextHandler(logs, nil)))
	if err != nil {
		t.Fatal(err)
	}
	object, err := wincom.NewDataObject(data)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		wincom.Release(object)
		if got := wincom.ObjectCount(); got != baseline {
			t.Errorf("hosted objects alive: %d, want %d", got, baseline)
		}
	})
	return &hostedData{t: t, object: object, data: data, source: source, formats: formats, log: logs}
}

func (h *hostedData) format(id uint16, index int32, tymed uint32) wincom.FormatEtc {
	return wincom.FormatEtc{Format: id, Aspect: wincom.DVAspectContent, Index: index, Tymed: tymed}
}

func (h *hostedData) getData(f wincom.FormatEtc) (wincom.Medium, wincom.HResult) {
	h.t.Helper()
	medium, err := wincom.GetData(h.object, f)
	if err == nil {
		return medium, wincom.SOK
	}
	var hr wincom.HResult
	if !errors.As(err, &hr) {
		h.t.Fatalf("GetData error is not an HRESULT: %v", err)
	}
	return wincom.Medium{}, hr
}

func (h *hostedData) setData(f wincom.FormatEtc, payload []byte) wincom.HResult {
	h.t.Helper()
	medium, err := wincom.NewHGlobalMedium(payload)
	if err != nil {
		h.t.Fatal(err)
	}
	hr := callMethod(h.object, vtblDataSetData, uintptr(unsafe.Pointer(&f)), uintptr(unsafe.Pointer(&medium)), 1)
	if hr.Failed() {
		wincom.ReleaseMedium(&medium)
	}
	return hr
}

func readStream(t *testing.T, stream uintptr, size int) ([]byte, wincom.HResult) {
	t.Helper()
	buf := make([]byte, size)
	var read uint32
	hr := callMethod(stream, vtblStreamRead, uintptr(unsafe.Pointer(&buf[0])), uintptr(size), uintptr(unsafe.Pointer(&read)))
	return buf[:read], hr
}

func TestDataSource_EnumeratesThreeFormats(t *testing.T) {
	h := newHostedData(t)
	formats, err := wincom.EnumFormats(h.object)
	if err != nil {
		t.Fatal(err)
	}
	want := []wincom.FormatEtc{
		h.format(h.formats.Descriptor, -1, wincom.TymedHGlobal),
		h.format(h.formats.Contents, -1, wincom.TymedIStream),
		h.format(h.formats.Preferred, -1, wincom.TymedHGlobal),
	}
	if len(formats) != len(want) {
		t.Fatalf("enumerated %d formats, want %d: %+v", len(formats), len(want), formats)
	}
	for i := range want {
		if formats[i] != want[i] {
			t.Errorf("format %d = %+v, want %+v", i, formats[i], want[i])
		}
	}
	if err := wincom.QueryGetData(h.object, h.format(h.formats.Contents, 1, wincom.TymedIStream)); err != nil {
		t.Fatalf("QueryGetData(FileContents, ISTREAM) = %v", err)
	}
	if err := wincom.QueryGetData(h.object, h.format(h.formats.Contents, 1, wincom.TymedHGlobal)); !errors.Is(err, wincom.DVETymed) {
		t.Fatalf("QueryGetData(FileContents, HGLOBAL) = %v, want DV_E_TYMED", err)
	}
}

func TestDataSource_GroupDescriptorMatchesEntries(t *testing.T) {
	h := newHostedData(t)
	medium, hr := h.getData(h.format(h.formats.Descriptor, -1, wincom.TymedHGlobal))
	if hr != wincom.SOK {
		t.Fatalf("GetData(FileGroupDescriptorW) = %v", hr)
	}
	got, err := medium.HGlobalBytes()
	wincom.ReleaseMedium(&medium)
	if err != nil {
		t.Fatal(err)
	}
	want, err := buildFileGroupDescriptor(testEntries())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("descriptor over COM differs from the built one (%d vs %d bytes)", len(got), len(want))
	}
}

func TestDataSource_PreferredDropEffectIsCopy(t *testing.T) {
	h := newHostedData(t)
	medium, hr := h.getData(h.format(h.formats.Preferred, -1, wincom.TymedHGlobal))
	if hr != wincom.SOK {
		t.Fatalf("GetData(Preferred DropEffect) = %v", hr)
	}
	got, err := medium.HGlobalBytes()
	wincom.ReleaseMedium(&medium)
	if err != nil || len(got) != 4 || binary.LittleEndian.Uint32(got) != wincom.DropEffectCopy {
		t.Fatalf("Preferred DropEffect = %v, %v; want DROPEFFECT_COPY", got, err)
	}
}

func TestDataSource_FileContentsStreamsLazily(t *testing.T) {
	h := newHostedData(t)
	medium, hr := h.getData(h.format(h.formats.Contents, 1, wincom.TymedIStream))
	if hr != wincom.SOK || medium.Tymed != wincom.TymedIStream {
		t.Fatalf("GetData(FileContents, 1) = %v, medium %+v", hr, medium)
	}
	if stats := h.source.Stats()[1]; stats.Opens != 1 || stats.BytesServed != 0 || stats.Reads != 0 {
		t.Fatalf("GetData must not read any content: %+v", stats)
	}

	var collected []byte
	for {
		chunk, hr := readStream(t, medium.Handle, testChunk)
		if hr.Failed() {
			t.Fatalf("IStream::Read = %v", hr)
		}
		if len(chunk) == 0 {
			break
		}
		collected = append(collected, chunk...)
		if got := h.source.Stats()[1].BytesServed; got != int64(len(collected)) {
			t.Fatalf("source served %d bytes while the consumer holds %d", got, len(collected))
		}
	}
	if !bytes.Equal(collected, expectedContent(0, testLargeSize)) {
		t.Fatalf("streamed %d bytes that differ from byte(offset*31+7)", len(collected))
	}
	wincom.ReleaseMedium(&medium)

	stats := h.source.Stats()[1]
	if stats.Completed != 1 || stats.Aborted != 0 || stats.BytesServed != testLargeSize {
		t.Fatalf("stats after a full read = %+v", stats)
	}
	if !strings.Contains(h.log.String(), "completed=true") {
		t.Fatalf("release of the stream was not logged:\n%s", h.log)
	}
}

func TestDataSource_FileContentsAbortedStreamIsAccounted(t *testing.T) {
	h := newHostedData(t)
	medium, hr := h.getData(h.format(h.formats.Contents, 2, wincom.TymedIStream))
	if hr != wincom.SOK {
		t.Fatalf("GetData(FileContents, 2) = %v", hr)
	}
	if chunk, hr := readStream(t, medium.Handle, 1000); hr != wincom.SOK || len(chunk) != 1000 {
		t.Fatalf("first read = %d bytes, %v", len(chunk), hr)
	}
	wincom.ReleaseMedium(&medium)

	stats := h.source.Stats()[2]
	if stats.Aborted != 1 || stats.Completed != 0 || stats.BytesServed != 1000 {
		t.Fatalf("a stream released early must be accounted as aborted: %+v", stats)
	}
	if !strings.Contains(h.log.String(), "completed=false") {
		t.Fatalf("early release was not logged:\n%s", h.log)
	}
}

func TestDataSource_EachGetDataOpensItsOwnStream(t *testing.T) {
	h := newHostedData(t)
	first, hr := h.getData(h.format(h.formats.Contents, 2, wincom.TymedIStream))
	if hr != wincom.SOK {
		t.Fatalf("first GetData = %v", hr)
	}
	second, hr := h.getData(h.format(h.formats.Contents, 2, wincom.TymedIStream))
	if hr != wincom.SOK {
		t.Fatalf("second GetData = %v", hr)
	}
	if first.Handle == second.Handle {
		t.Fatal("two GetData calls returned the same IStream")
	}
	if chunk, _ := readStream(t, first.Handle, 10); len(chunk) != 10 {
		t.Fatalf("first stream read %d bytes", len(chunk))
	}
	if chunk, _ := readStream(t, second.Handle, 10); !bytes.Equal(chunk, expectedContent(0, 10)) {
		t.Fatal("the second stream does not start at offset 0")
	}
	wincom.ReleaseMedium(&first)
	wincom.ReleaseMedium(&second)
	if opens := h.source.Stats()[2].Opens; opens != 2 {
		t.Fatalf("opens = %d, want 2", opens)
	}
}

func TestDataSource_FileContentsRejectsDirectoryAndBadLindex(t *testing.T) {
	h := newHostedData(t)
	for _, index := range []int32{0, -1, 3, 1000} {
		medium, hr := h.getData(h.format(h.formats.Contents, index, wincom.TymedIStream))
		if hr != wincom.DVELindex {
			t.Errorf("GetData(FileContents, lindex %d) = %v, want DV_E_LINDEX", index, hr)
		}
		if medium != (wincom.Medium{}) {
			t.Errorf("lindex %d returned a medium %+v", index, medium)
		}
	}
	if medium, hr := h.getData(h.format(h.formats.Contents, 1, wincom.TymedHGlobal)); hr != wincom.DVETymed || medium.Handle != 0 {
		t.Errorf("GetData(FileContents, HGLOBAL) = %v, %+v; want DV_E_TYMED", hr, medium)
	}
	if medium, hr := h.getData(h.format(h.formats.Performed, -1, wincom.TymedHGlobal)); hr != wincom.DVEFormatEtc || medium.Handle != 0 {
		t.Errorf("GetData(Performed DropEffect) = %v; the format is set-only, want DV_E_FORMATETC", hr)
	}
	for _, stats := range h.source.Stats() {
		if stats.Opens != 0 {
			t.Errorf("a refused request opened %q", stats.Path)
		}
	}
}

func TestDataSource_SetDataRecordsOutcomes(t *testing.T) {
	h := newHostedData(t)
	dword := func(v uint32) []byte { return binary.LittleEndian.AppendUint32(nil, v) }

	if hr := h.setData(h.format(h.formats.Performed, -1, wincom.TymedHGlobal), dword(wincom.DropEffectCopy)); hr != wincom.SOK {
		t.Fatalf("SetData(Performed DropEffect) = %v", hr)
	}
	if hr := h.setData(h.format(h.formats.PasteSucceeded, -1, wincom.TymedHGlobal), dword(wincom.DropEffectMove)); hr != wincom.SOK {
		t.Fatalf("SetData(Paste Succeeded) = %v", hr)
	}
	want := dropOutcomes{PerformedSet: true, Performed: wincom.DropEffectCopy, PasteSucceeded: true, PasteEffect: wincom.DropEffectMove}
	if got := h.data.Outcomes(); got != want {
		t.Fatalf("outcomes = %+v, want %+v", got, want)
	}
	for _, fragment := range []string{"Performed DropEffect", "Paste Succeeded", "effect=COPY", "effect=MOVE"} {
		if !strings.Contains(h.log.String(), fragment) {
			t.Errorf("log lacks %q:\n%s", fragment, h.log)
		}
	}
}

func TestDataSource_SetDataRefusesUnknownAndMalformed(t *testing.T) {
	h := newHostedData(t)
	if hr := h.setData(h.format(h.formats.Contents, 1, wincom.TymedHGlobal), []byte{1, 0, 0, 0}); hr != wincom.DVEFormatEtc {
		t.Fatalf("SetData(FileContents) = %v, want DV_E_FORMATETC", hr)
	}
	if hr := h.setData(h.format(h.formats.Performed, -1, wincom.TymedHGlobal), []byte{1, 0}); hr != wincom.EInvalidArg {
		t.Fatalf("SetData with a 2 byte payload = %v, want E_INVALIDARG", hr)
	}
	if got := h.data.Outcomes(); got != (dropOutcomes{}) {
		t.Fatalf("a refused SetData changed the outcomes: %+v", got)
	}
}

func TestDataSource_LogsEveryCallWithFormatLindexTymedAspect(t *testing.T) {
	h := newHostedData(t)
	medium, hr := h.getData(h.format(h.formats.Contents, 2, wincom.TymedIStream))
	if hr != wincom.SOK {
		t.Fatalf("GetData = %v", hr)
	}
	wincom.ReleaseMedium(&medium)
	for _, fragment := range []string{"call=GetData", "format=FileContents", "lindex=2", "tymed=ISTREAM", "aspect=CONTENT"} {
		if !strings.Contains(h.log.String(), fragment) {
			t.Errorf("log lacks %q:\n%s", fragment, h.log)
		}
	}
}

func TestNewClipboardData_RejectsUnrepresentableTree(t *testing.T) {
	formats, err := registerShellFormats()
	if err != nil {
		t.Fatal(err)
	}
	source, err := newSyntheticSource([]entry{{Path: "bad:name", Size: 1, ModTime: testModTime}}, options{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := newClipboardData(source, formats, slog.New(slog.DiscardHandler)); !errors.Is(err, ErrNotRepresentable) {
		t.Fatalf("error = %v, want ErrNotRepresentable", err)
	}
}

func TestCallbackBudgetStaysWithinLimit(t *testing.T) {
	h := newHostedData(t)
	for range 5 {
		medium, hr := h.getData(h.format(h.formats.Contents, 2, wincom.TymedIStream))
		if hr != wincom.SOK {
			t.Fatalf("GetData = %v", hr)
		}
		wincom.ReleaseMedium(&medium)
	}
	if got := wincom.CallbackCount(); got > 64 {
		t.Fatalf("wincom created %d callbacks, budget is 64", got)
	}
}
