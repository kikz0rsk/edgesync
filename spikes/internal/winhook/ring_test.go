package winhook

import (
	"errors"
	"sync"
	"testing"
)

func newTestRing(t *testing.T, capacity int) *Ring {
	t.Helper()
	ring, err := NewRing(capacity)
	if err != nil {
		t.Fatalf("NewRing(%d): %v", capacity, err)
	}
	return ring
}

func TestRing_PushPopWrapAround(t *testing.T) {
	ring := newTestRing(t, 4)

	next := int32(0)
	want := int32(0)
	for round := 0; round < 10; round++ {
		for i := 0; i < 3; i++ {
			if !ring.Push(Event{Kind: KindMouse, X: next}) {
				t.Fatalf("round %d: push %d rejected", round, next)
			}
			next++
		}
		for i := 0; i < 3; i++ {
			ev, ok := ring.Pop()
			if !ok {
				t.Fatalf("round %d: pop returned empty", round)
			}
			if ev.X != want {
				t.Fatalf("round %d: popped X=%d, want %d", round, ev.X, want)
			}
			want++
		}
		if _, ok := ring.Pop(); ok {
			t.Fatalf("round %d: ring should be empty", round)
		}
	}
	if ring.Drops() != 0 {
		t.Fatalf("drops = %d, want 0", ring.Drops())
	}
}

func TestRing_FullDropsAndCounts(t *testing.T) {
	ring := newTestRing(t, 4)

	for i := 0; i < 4; i++ {
		if !ring.Push(Event{X: int32(i)}) {
			t.Fatalf("push %d rejected before the ring is full", i)
		}
	}
	for i := 0; i < 3; i++ {
		if ring.Push(Event{X: 100}) {
			t.Fatal("push accepted on a full ring")
		}
	}
	if got := ring.Drops(); got != 3 {
		t.Fatalf("drops = %d, want 3", got)
	}
	if got := ring.Len(); got != 4 {
		t.Fatalf("len = %d, want 4", got)
	}
	for i := 0; i < 4; i++ {
		ev, ok := ring.Pop()
		if !ok || ev.X != int32(i) {
			t.Fatalf("pop %d = (%d, %v), dropped events must not overwrite queued ones", i, ev.X, ok)
		}
	}
	if !ring.Push(Event{X: 7}) {
		t.Fatal("push rejected after the ring was drained")
	}
}

func TestRing_RejectsInvalidCapacity(t *testing.T) {
	for _, capacity := range []int{-4, 0, 1, 3, 4095} {
		if _, err := NewRing(capacity); !errors.Is(err, ErrInvalidCapacity) {
			t.Fatalf("NewRing(%d) error = %v, want ErrInvalidCapacity", capacity, err)
		}
	}
	ring := newTestRing(t, DefaultRingCapacity)
	if ring.Capacity() != 4096 {
		t.Fatalf("default capacity = %d, want 4096", ring.Capacity())
	}
}

func TestRing_PushZeroAllocs(t *testing.T) {
	ring := newTestRing(t, DefaultRingCapacity)
	ev := Event{Kind: KindMouse, Message: WMMouseMove, X: 10, Y: 20}

	allocs := testing.AllocsPerRun(1000, func() {
		if !ring.Push(ev) {
			ring.Pop()
			ring.Push(ev)
		}
	})
	if allocs != 0 {
		t.Fatalf("Push allocates %v times per call, want 0", allocs)
	}

	full := newTestRing(t, 2)
	full.Push(ev)
	full.Push(ev)
	allocs = testing.AllocsPerRun(1000, func() { full.Push(ev) })
	if allocs != 0 {
		t.Fatalf("Push on a full ring allocates %v times per call, want 0", allocs)
	}
}

func TestRing_ConcurrentProducerConsumer(t *testing.T) {
	const total = 200000
	ring := newTestRing(t, 64)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := int32(0); i < total; {
			if ring.Push(Event{X: i}) {
				i++
			}
		}
	}()

	for want := int32(0); want < total; {
		ev, ok := ring.Pop()
		if !ok {
			continue
		}
		if ev.X != want {
			t.Fatalf("popped X=%d, want %d", ev.X, want)
		}
		want++
	}
	wg.Wait()
	if ring.Len() != 0 {
		t.Fatalf("len = %d after draining, want 0", ring.Len())
	}
}
