// Package wincom hosts COM objects (IDataObject, IEnumFORMATETC, IStream, IDropSource, IDropTarget) implemented in
// pure Go. Every COM object is a block of non-Go memory shaped { lpVtbl, handle }; the handle indexes a Go-side
// table that holds the implementation and the COM reference count, so Windows never retains a Go heap pointer.
package wincom

import "sync"

type entry[T any] struct {
	value T
	refs  uint32
}

// Table maps opaque handles to Go values together with a COM-style reference count. Handles are never reused.
type Table[T any] struct {
	mu      sync.Mutex
	next    uintptr
	entries map[uintptr]*entry[T]
}

func NewTable[T any]() *Table[T] {
	return &Table[T]{next: 1, entries: make(map[uintptr]*entry[T])}
}

// Add stores v with a reference count of one.
func (t *Table[T]) Add(v T) uintptr {
	t.mu.Lock()
	defer t.mu.Unlock()
	handle := t.next
	t.next++
	t.entries[handle] = &entry[T]{value: v, refs: 1}
	return handle
}

func (t *Table[T]) Get(handle uintptr) (T, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	e, ok := t.entries[handle]
	if !ok {
		var zero T
		return zero, false
	}
	return e.value, true
}

// AddRef returns the new reference count; ok is false for an unknown handle.
func (t *Table[T]) AddRef(handle uintptr) (count uint32, ok bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	e, found := t.entries[handle]
	if !found {
		return 0, false
	}
	e.refs++
	return e.refs, true
}

// Release returns the new reference count. When it reaches zero the entry is removed and freed is true, so the caller
// can finalise the value returned with it. ok is false for an unknown handle.
func (t *Table[T]) Release(handle uintptr) (count uint32, value T, freed, ok bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	e, found := t.entries[handle]
	if !found {
		var zero T
		return 0, zero, false, false
	}
	e.refs--
	if e.refs > 0 {
		return e.refs, e.value, false, true
	}
	delete(t.entries, handle)
	return 0, e.value, true, true
}

func (t *Table[T]) Len() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.entries)
}
