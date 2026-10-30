package wincom

import (
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	streamRead = iota + 3
	streamWrite
	streamSeek
	streamSetSize
	streamCopyTo
	streamCommit
	streamRevert
	streamLockRegion
	streamUnlockRegion
	streamStat
	streamClone
)

const (
	streamSeekSet uint32 = 0
	streamSeekCur uint32 = 1
	streamSeekEnd uint32 = 2

	stgtyStream      uint32 = 2
	statFlagNoName   uint32 = 1
	copyChunkBytes          = 64 << 10
	maxStatFlagValue uint32 = statFlagNoName
)

var procCoTaskMemAlloc = ole32.NewProc("CoTaskMemAlloc")

// statStg mirrors STATSTG (80 bytes on amd64).
type statStg struct {
	Name           uintptr
	Type           uint32
	Size           uint64
	ModifyTime     windows.Filetime
	CreateTime     windows.Filetime
	AccessTime     windows.Filetime
	Mode           uint32
	LocksSupported uint32
	Clsid          windows.GUID
	StateBits      uint32
	Reserved       uint32
}

type streamShared struct {
	mu        sync.Mutex
	source    io.ReadSeeker
	size      int64
	name      string
	cursor    int64
	live      atomic.Int32
	onRelease func()
}

type stream struct {
	shared *streamShared
	pos    int64
}

var streamVtable = sync.OnceValues(func() (uintptr, error) {
	return newInterfaceVtable(
		newCallback(streamReadCallback),
		newCallback(streamWriteCallback),
		newCallback(streamSeekCallback),
		newCallback(streamSetSizeCallback),
		newCallback(streamCopyToCallback),
		newCallback(streamCommitCallback),
		newCallback(streamRevertCallback),
		newCallback(streamLockRegionCallback),
		newCallback(streamUnlockRegionCallback),
		newCallback(streamStatCallback),
		newCallback(streamCloneCallback),
	)
})

// NewStream returns a read-only IStream of exactly size bytes over source, which must be positioned at its start.
// onRelease (optional) runs once when the last reference to the stream and all its clones is gone. The returned
// pointer carries one reference owned by the caller.
func NewStream(source io.ReadSeeker, size int64, name string, onRelease func()) (uintptr, error) {
	shared := &streamShared{source: source, size: size, name: name, onRelease: onRelease}
	shared.live.Store(1)
	vtbl, err := streamVtable()
	if err != nil {
		return 0, fmt.Errorf("IStream vtable: %w", err)
	}
	return newStreamObject(vtbl, &stream{shared: shared})
}

func newStreamObject(vtbl uintptr, s *stream) (uintptr, error) {
	iids := []windows.GUID{iidISequentialStream, iidIStream}
	return createObject(vtbl, iids, s, s.shared.releaseOne)
}

func (s *streamShared) releaseOne() {
	if s.live.Add(-1) == 0 && s.onRelease != nil {
		s.onRelease()
	}
}

// read fills as much of buf as the stream size allows. Any shortfall other than the end of the stream is an error.
func (s *stream) read(buf []byte) (int, error) {
	shared := s.shared
	shared.mu.Lock()
	defer shared.mu.Unlock()
	remaining := shared.size - s.pos
	if remaining <= 0 || len(buf) == 0 {
		return 0, nil
	}
	if int64(len(buf)) > remaining {
		buf = buf[:remaining]
	}
	if shared.cursor != s.pos {
		shared.cursor = -1
		if _, err := shared.source.Seek(s.pos, io.SeekStart); err != nil {
			return 0, fmt.Errorf("seek source to %d: %w", s.pos, err)
		}
		shared.cursor = s.pos
	}
	n, err := io.ReadFull(shared.source, buf)
	shared.cursor += int64(n)
	s.pos += int64(n)
	if err != nil {
		return n, fmt.Errorf("read source at %d: %w", s.pos, err)
	}
	return n, nil
}

func streamReadCallback(this, pv, cb, pcbRead uintptr) uintptr {
	s, ok := implOf[*stream](this)
	if !ok {
		return EUnexpected.ret()
	}
	want := int(uint32(cb))
	if pv == 0 && want > 0 {
		return STGEInvalidPointer.ret()
	}
	var buf []byte
	if want > 0 {
		buf = unsafe.Slice((*byte)(ptr(pv)), want)
	}
	n, err := s.read(buf)
	if pcbRead != 0 {
		putUint32(pcbRead, uint32(n))
	}
	if err != nil {
		return STGEReadFault.ret()
	}
	if n < want {
		return SFalse.ret()
	}
	return SOK.ret()
}

func streamWriteCallback(this, pv, cb, pcbWritten uintptr) uintptr {
	if pcbWritten != 0 {
		putUint32(pcbWritten, 0)
	}
	return STGEAccessDenied.ret()
}

func streamSeekCallback(this, move, origin, plibNewPosition uintptr) uintptr {
	s, ok := implOf[*stream](this)
	if !ok {
		return EUnexpected.ret()
	}
	displacement := int64(move)
	s.shared.mu.Lock()
	defer s.shared.mu.Unlock()
	var target int64
	switch uint32(origin) {
	case streamSeekSet:
		target = displacement
	case streamSeekCur:
		target = s.pos + displacement
	case streamSeekEnd:
		target = s.shared.size + displacement
	default:
		return STGEInvalidFunction.ret()
	}
	if target < 0 {
		return STGEInvalidFunction.ret()
	}
	s.pos = target
	if plibNewPosition != 0 {
		putUint64(plibNewPosition, uint64(target))
	}
	return SOK.ret()
}

func streamSetSizeCallback(this, newSize uintptr) uintptr {
	return STGEInvalidFunction.ret()
}

func streamCopyToCallback(this, pstm, cb, pcbRead, pcbWritten uintptr) uintptr {
	s, ok := implOf[*stream](this)
	if !ok {
		return EUnexpected.ret()
	}
	if pstm == 0 {
		return STGEInvalidPointer.ret()
	}
	limit := uint64(cb)
	var copied uint64
	buf := make([]byte, copyChunkBytes)
	result := SOK
	for copied < limit {
		chunk := buf[:min(uint64(len(buf)), limit-copied)]
		n, err := s.read(chunk)
		if err != nil {
			result = STGEReadFault
			break
		}
		if n == 0 {
			break
		}
		if hr := writeAll(pstm, buf[:n]); hr != SOK {
			result = hr
			break
		}
		copied += uint64(n)
		if n < len(chunk) {
			break
		}
	}
	if pcbRead != 0 {
		putUint64(pcbRead, copied)
	}
	if pcbWritten != 0 {
		putUint64(pcbWritten, copied)
	}
	return result.ret()
}

func writeAll(pstm uintptr, data []byte) HResult {
	var written uint32
	hr := callMethod(pstm, streamWrite, uintptr(unsafe.Pointer(&data[0])), uintptr(len(data)), uintptr(unsafe.Pointer(&written)))
	if result := HResult(hr); result.Failed() {
		return result
	}
	if int(written) != len(data) {
		return STGEMediumFull
	}
	return SOK
}

func streamCommitCallback(this, flags uintptr) uintptr {
	return SOK.ret()
}

func streamRevertCallback(this uintptr) uintptr {
	return SOK.ret()
}

func streamLockRegionCallback(this, offset, cb, lockType uintptr) uintptr {
	return STGEInvalidFunction.ret()
}

func streamUnlockRegionCallback(this, offset, cb, lockType uintptr) uintptr {
	return STGEInvalidFunction.ret()
}

func streamStatCallback(this, pstatstg, flag uintptr) uintptr {
	s, ok := implOf[*stream](this)
	if !ok {
		return EUnexpected.ret()
	}
	if pstatstg == 0 {
		return STGEInvalidPointer.ret()
	}
	statFlag := uint32(flag)
	if statFlag > maxStatFlagValue {
		return STGEInvalidFlag.ret()
	}
	st := statStg{Type: stgtyStream, Size: uint64(s.shared.size)}
	if statFlag&statFlagNoName == 0 && s.shared.name != "" {
		name, err := coTaskMemString(s.shared.name)
		if err != nil {
			return STGEInsufficientMemory.ret()
		}
		st.Name = name
	}
	*(*statStg)(ptr(pstatstg)) = st
	return SOK.ret()
}

func coTaskMemString(value string) (uintptr, error) {
	units, err := windows.UTF16FromString(value)
	if err != nil {
		return 0, fmt.Errorf("stream name %q: %w", value, err)
	}
	size := uintptr(len(units)) * unsafe.Sizeof(units[0])
	mem, _, callErr := procCoTaskMemAlloc.Call(size)
	if mem == 0 {
		return 0, fmt.Errorf("CoTaskMemAlloc(%d): %w", size, callErr)
	}
	copy(unsafe.Slice((*uint16)(ptr(mem)), len(units)), units)
	return mem, nil
}

func streamCloneCallback(this, ppstm uintptr) uintptr {
	if ppstm == 0 {
		return STGEInvalidPointer.ret()
	}
	putUintptr(ppstm, 0)
	s, ok := implOf[*stream](this)
	if !ok {
		return EUnexpected.ret()
	}
	s.shared.mu.Lock()
	clone := &stream{shared: s.shared, pos: s.pos}
	s.shared.mu.Unlock()
	s.shared.live.Add(1)
	obj, err := newStreamObject(vtblOf(this), clone)
	if err != nil {
		s.shared.live.Add(-1)
		return STGEInsufficientMemory.ret()
	}
	putUintptr(ppstm, obj)
	return SOK.ret()
}
