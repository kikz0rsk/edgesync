package wincom

import (
	"sync"
	"testing"
)

func TestHandles_AddGetRelease(t *testing.T) {
	table := NewTable[string]()

	first := table.Add("first")
	second := table.Add("second")
	if first == 0 || second == 0 || first == second {
		t.Fatalf("handles must be non-zero and distinct, got %d and %d", first, second)
	}
	if got, ok := table.Get(first); !ok || got != "first" {
		t.Fatalf("Get(first) = %q, %v", got, ok)
	}

	if count, ok := table.AddRef(first); !ok || count != 2 {
		t.Fatalf("AddRef = %d, %v; want 2, true", count, ok)
	}
	if count, _, freed, ok := table.Release(first); !ok || freed || count != 1 {
		t.Fatalf("first Release = %d, freed=%v, ok=%v; want 1, false, true", count, freed, ok)
	}
	count, value, freed, ok := table.Release(first)
	if !ok || !freed || count != 0 || value != "first" {
		t.Fatalf("last Release = %d, %q, freed=%v, ok=%v", count, value, freed, ok)
	}
	if _, ok := table.Get(first); ok {
		t.Fatal("released handle must not resolve")
	}
	if _, _, _, ok := table.Release(first); ok {
		t.Fatal("releasing a freed handle must report unknown")
	}
	if _, ok := table.AddRef(first); ok {
		t.Fatal("AddRef on a freed handle must report unknown")
	}
	if table.Len() != 1 {
		t.Fatalf("Len = %d, want 1", table.Len())
	}

	reused := table.Add("third")
	if reused == first {
		t.Fatal("handles must never be reused")
	}
}

func TestHandles_ConcurrentAccess(t *testing.T) {
	const (
		workers    = 16
		iterations = 500
	)
	table := NewTable[int]()
	shared := table.Add(-1)

	var wg sync.WaitGroup
	for worker := range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range iterations {
				own := table.Add(worker*iterations + i)
				if got, ok := table.Get(own); !ok || got != worker*iterations+i {
					t.Errorf("Get(own) = %d, %v", got, ok)
					return
				}
				table.AddRef(shared)
				table.Release(shared)
				if _, _, freed, ok := table.Release(own); !ok || !freed {
					t.Errorf("Release(own) freed=%v ok=%v", freed, ok)
					return
				}
			}
		}()
	}
	wg.Wait()

	if table.Len() != 1 {
		t.Fatalf("Len = %d, want only the shared entry", table.Len())
	}
	if count, _, freed, ok := table.Release(shared); !ok || !freed || count != 0 {
		t.Fatalf("shared reference count drifted: count=%d freed=%v ok=%v", count, freed, ok)
	}
}
