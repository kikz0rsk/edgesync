package stats

import (
	"errors"
	"testing"
	"time"
)

func recordN(h *Histogram, d time.Duration, n int) {
	for i := 0; i < n; i++ {
		h.Record(d)
	}
}

func TestHistogram_Percentiles(t *testing.T) {
	var h Histogram
	recordN(&h, 3*time.Microsecond, 90)
	recordN(&h, 100*time.Microsecond, 9)
	h.Record(5 * time.Millisecond)

	cases := []struct {
		percentile float64
		want       time.Duration
	}{
		{50, 4 * time.Microsecond},
		{90, 4 * time.Microsecond},
		{99, 128 * time.Microsecond},
		{99.5, 5 * time.Millisecond},
		{100, 5 * time.Millisecond},
	}
	for _, tc := range cases {
		got, err := h.Percentile(tc.percentile)
		if err != nil {
			t.Fatalf("Percentile(%v): %v", tc.percentile, err)
		}
		if got != tc.want {
			t.Errorf("Percentile(%v) = %v, want %v", tc.percentile, got, tc.want)
		}
	}
	if got := h.Count(); got != 100 {
		t.Errorf("Count = %d, want 100", got)
	}
	if got := h.Max(); got != 5*time.Millisecond {
		t.Errorf("Max = %v, want 5ms", got)
	}
	mean, err := h.Mean()
	if err != nil {
		t.Fatalf("Mean: %v", err)
	}
	if want := (90*3*time.Microsecond + 9*100*time.Microsecond + 5*time.Millisecond) / 100; mean != want {
		t.Errorf("Mean = %v, want %v", mean, want)
	}
}

func TestHistogram_SubMicrosecondAndOverflow(t *testing.T) {
	var h Histogram
	h.Record(400 * time.Nanosecond)
	got, err := h.Percentile(100)
	if err != nil || got != 400*time.Nanosecond {
		t.Fatalf("sub-microsecond Percentile(100) = %v, %v, want 400ns", got, err)
	}

	h.Record(3 * time.Second)
	got, err = h.Percentile(100)
	if err != nil || got != 3*time.Second {
		t.Fatalf("overflow Percentile(100) = %v, %v, want the exact 3s maximum", got, err)
	}
	got, err = h.Percentile(50)
	if err != nil || got != time.Microsecond {
		t.Fatalf("Percentile(50) = %v, %v, want the 1us bucket bound", got, err)
	}
}

func TestHistogram_EmptyAndInvalidPercentile(t *testing.T) {
	var h Histogram
	if _, err := h.Percentile(50); !errors.Is(err, ErrEmpty) {
		t.Fatalf("empty Percentile error = %v, want ErrEmpty", err)
	}
	if _, err := h.Mean(); !errors.Is(err, ErrEmpty) {
		t.Fatalf("empty Mean error = %v, want ErrEmpty", err)
	}
	h.Record(time.Microsecond)
	for _, p := range []float64{0, -1, 100.5} {
		if _, err := h.Percentile(p); !errors.Is(err, ErrInvalidPercentile) {
			t.Fatalf("Percentile(%v) error = %v, want ErrInvalidPercentile", p, err)
		}
	}
}

func TestHistogram_RecordZeroAllocs(t *testing.T) {
	var h Histogram
	allocs := testing.AllocsPerRun(1000, func() {
		h.Record(7 * time.Microsecond)
		h.Record(-time.Second)
	})
	if allocs != 0 {
		t.Fatalf("Record allocates %v times per call, want 0", allocs)
	}
}
