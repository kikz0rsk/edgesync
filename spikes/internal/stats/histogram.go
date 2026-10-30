// Package stats provides the fixed-bucket latency histogram shared by the spikes.
package stats

import (
	"errors"
	"math"
	"math/bits"
	"sync/atomic"
	"time"
)

const bucketCount = 22

var (
	ErrEmpty             = errors.New("stats: histogram has no samples")
	ErrInvalidPercentile = errors.New("stats: percentile must be within (0, 100]")
)

// Histogram counts durations in log2 microsecond buckets: bucket 0 holds values below 1 us and bucket k holds
// [2^(k-1), 2^k) us. The last bucket also takes everything above 2 s. Record is allocation-free and safe for
// concurrent use; the exact maximum and sum are tracked beside the buckets.
type Histogram struct {
	buckets [bucketCount]atomic.Uint64
	sumNs   atomic.Uint64
	maxNs   atomic.Int64
}

func bucketIndex(d time.Duration) int {
	index := bits.Len64(uint64(d / time.Microsecond))
	if index >= bucketCount {
		return bucketCount - 1
	}
	return index
}

func bucketUpperBound(index int) time.Duration {
	return time.Duration(1<<index) * time.Microsecond
}

func (h *Histogram) Record(d time.Duration) {
	if d < 0 {
		d = 0
	}
	h.buckets[bucketIndex(d)].Add(1)
	h.sumNs.Add(uint64(d))
	for {
		current := h.maxNs.Load()
		if int64(d) <= current || h.maxNs.CompareAndSwap(current, int64(d)) {
			return
		}
	}
}

func (h *Histogram) Count() uint64 {
	var total uint64
	for i := range h.buckets {
		total += h.buckets[i].Load()
	}
	return total
}

func (h *Histogram) Max() time.Duration {
	return time.Duration(h.maxNs.Load())
}

func (h *Histogram) Mean() (time.Duration, error) {
	count := h.Count()
	if count == 0 {
		return 0, ErrEmpty
	}
	return time.Duration(h.sumNs.Load() / count), nil
}

// Percentile returns the upper bound of the bucket holding the p-th percentile sample, capped at the exact maximum.
func (h *Histogram) Percentile(p float64) (time.Duration, error) {
	if p <= 0 || p > 100 {
		return 0, ErrInvalidPercentile
	}
	var counts [bucketCount]uint64
	var total uint64
	for i := range h.buckets {
		counts[i] = h.buckets[i].Load()
		total += counts[i]
	}
	if total == 0 {
		return 0, ErrEmpty
	}
	rank := max(uint64(math.Ceil(float64(total)*p/100)), 1)
	var cumulative uint64
	for i, count := range counts {
		cumulative += count
		if cumulative >= rank {
			if i == bucketCount-1 {
				return h.Max(), nil
			}
			return min(bucketUpperBound(i), h.Max()), nil
		}
	}
	return h.Max(), nil
}
