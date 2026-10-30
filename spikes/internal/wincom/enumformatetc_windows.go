package wincom

import (
	"fmt"
	"slices"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	enumNext = iota + 3
	enumSkip
	enumReset
	enumClone
)

type formatEnumerator struct {
	mu      sync.Mutex
	formats []FormatEtc
	pos     int
}

var enumFormatEtcVtable = sync.OnceValues(func() (uintptr, error) {
	return newInterfaceVtable(
		newCallback(enumNextCallback),
		newCallback(enumSkipCallback),
		newCallback(enumResetCallback),
		newCallback(enumCloneCallback),
	)
})

// NewFormatEnumerator returns an IEnumFORMATETC over a snapshot of formats with one reference held by the caller.
func NewFormatEnumerator(formats []FormatEtc) (uintptr, error) {
	vtbl, err := enumFormatEtcVtable()
	if err != nil {
		return 0, fmt.Errorf("IEnumFORMATETC vtable: %w", err)
	}
	return newFormatEnumeratorAt(vtbl, formats, 0)
}

func newFormatEnumeratorAt(vtbl uintptr, formats []FormatEtc, pos int) (uintptr, error) {
	enum := &formatEnumerator{formats: slices.Clone(formats), pos: pos}
	return createObject(vtbl, []windows.GUID{iidIEnumFormatEtc}, enum, nil)
}

func enumNextCallback(this, celt, rgelt, pceltFetched uintptr) uintptr {
	enum, ok := implOf[*formatEnumerator](this)
	if !ok {
		return EUnexpected.ret()
	}
	want := int(uint32(celt))
	if rgelt == 0 && want > 0 {
		return EPointer.ret()
	}
	if pceltFetched == 0 && want != 1 {
		return EInvalidArg.ret()
	}
	enum.mu.Lock()
	fetched := min(want, len(enum.formats)-enum.pos)
	if fetched > 0 {
		copy(unsafe.Slice((*FormatEtc)(ptr(rgelt)), fetched), enum.formats[enum.pos:enum.pos+fetched])
	}
	enum.pos += fetched
	enum.mu.Unlock()
	if pceltFetched != 0 {
		putUint32(pceltFetched, uint32(fetched))
	}
	if fetched < want {
		return SFalse.ret()
	}
	return SOK.ret()
}

func enumSkipCallback(this, celt uintptr) uintptr {
	enum, ok := implOf[*formatEnumerator](this)
	if !ok {
		return EUnexpected.ret()
	}
	skip := int(uint32(celt))
	enum.mu.Lock()
	defer enum.mu.Unlock()
	if skip > len(enum.formats)-enum.pos {
		enum.pos = len(enum.formats)
		return SFalse.ret()
	}
	enum.pos += skip
	return SOK.ret()
}

func enumResetCallback(this uintptr) uintptr {
	enum, ok := implOf[*formatEnumerator](this)
	if !ok {
		return EUnexpected.ret()
	}
	enum.mu.Lock()
	enum.pos = 0
	enum.mu.Unlock()
	return SOK.ret()
}

func enumCloneCallback(this, ppenum uintptr) uintptr {
	if ppenum == 0 {
		return EPointer.ret()
	}
	putUintptr(ppenum, 0)
	enum, ok := implOf[*formatEnumerator](this)
	if !ok {
		return EUnexpected.ret()
	}
	enum.mu.Lock()
	pos := enum.pos
	enum.mu.Unlock()
	clone, err := newFormatEnumeratorAt(vtblOf(this), enum.formats, pos)
	if err != nil {
		return EFail.ret()
	}
	putUintptr(ppenum, clone)
	return SOK.ret()
}
