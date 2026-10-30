// Package winhook implements the SP1 low-level hook spike: WH_MOUSE_LL and WH_KEYBOARD_LL on a dedicated locked
// OS thread, a lock-free event ring, a liveness watchdog and Raw Input deltas.
package winhook

import (
	"errors"
	"sync/atomic"
)

const (
	DefaultRingCapacity = 4096

	WMMouseMove   = 0x0200
	LLMHFInjected = 0x00000001
)

var ErrInvalidCapacity = errors.New("winhook: ring capacity must be a power of two and at least 2")

type EventKind uint8

const (
	KindMouse EventKind = iota + 1
	KindKey
	KindRawMouse
)

type Event struct {
	Kind    EventKind
	Message uint32
	X       int32
	Y       int32
	Data    uint32
	Scan    uint32
	Flags   uint32
	Time    uint32
	Extra   uintptr
	QPC     int64
}

type Ring struct {
	buf   []Event
	mask  uint64
	_     [48]byte
	head  atomic.Uint64
	_     [56]byte
	tail  atomic.Uint64
	_     [56]byte
	drops atomic.Uint64
}

func NewRing(capacity int) (*Ring, error) {
	if capacity < 2 || capacity&(capacity-1) != 0 {
		return nil, ErrInvalidCapacity
	}
	return &Ring{buf: make([]Event, capacity), mask: uint64(capacity - 1)}, nil
}

func (r *Ring) Capacity() int {
	return len(r.buf)
}

func (r *Ring) Push(ev Event) bool {
	head := r.head.Load()
	if head-r.tail.Load() == uint64(len(r.buf)) {
		r.drops.Add(1)
		return false
	}
	r.buf[head&r.mask] = ev
	r.head.Store(head + 1)
	return true
}

func (r *Ring) Pop() (Event, bool) {
	tail := r.tail.Load()
	if tail == r.head.Load() {
		return Event{}, false
	}
	ev := r.buf[tail&r.mask]
	r.tail.Store(tail + 1)
	return ev, true
}

func (r *Ring) Len() int {
	return int(r.head.Load() - r.tail.Load())
}

func (r *Ring) Drops() uint64 {
	return r.drops.Load()
}
