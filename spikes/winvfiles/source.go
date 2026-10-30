//go:build windows

package main

import (
	"errors"
	"fmt"
	"io"
	"math"
	"slices"
	"sync"
	"sync/atomic"
	"time"
)

const (
	demoLargeFileSize = 200 << 20
	demoSmallFileSize = 10 << 10
	bytesPerMiB       = 1 << 20
	maxRateBytes      = math.MaxInt64 / int64(time.Second)
)

var (
	ErrIndexOutOfRange = errors.New("entry index out of range")
	ErrDirectory       = errors.New("entry is a directory and has no content")
	ErrInvalidOptions  = errors.New("invalid source options")
	ErrNegativeOffset  = errors.New("negative offset")
	ErrInvalidWhence   = errors.New("invalid seek origin")
	ErrReaderClosed    = errors.New("reader is closed")
)

// entry describes one node of the synthetic tree; Path is relative and slash-separated.
type entry struct {
	Path    string
	Dir     bool
	Size    int64
	ModTime time.Time
}

func demoEntries(modTime time.Time) []entry {
	return []entry{
		{Path: "demo", Dir: true, ModTime: modTime},
		{Path: "demo/a.bin", Size: demoLargeFileSize, ModTime: modTime},
		{Path: "b.txt", Size: demoSmallFileSize, ModTime: modTime},
	}
}

func contentByte(offset int64) byte {
	return byte(offset*31 + 7)
}

// options configure the throttling of every reader. A zero rate means unthrottled; the latency is a delay before the
// first byte of each reader; blockRead stalls the very first read of the source once.
type options struct {
	rateBytesPerSec int64
	latency         time.Duration
	blockRead       time.Duration
}

func (o options) validate() error {
	switch {
	case o.rateBytesPerSec < 0 || o.rateBytesPerSec > maxRateBytes:
		return fmt.Errorf("%w: rate %d bytes/s outside 0..%d", ErrInvalidOptions, o.rateBytesPerSec, maxRateBytes)
	case o.latency < 0:
		return fmt.Errorf("%w: negative latency %v", ErrInvalidOptions, o.latency)
	case o.blockRead < 0:
		return fmt.Errorf("%w: negative block duration %v", ErrInvalidOptions, o.blockRead)
	}
	return nil
}

func (o options) transferTime(bytes int) time.Duration {
	if o.rateBytesPerSec == 0 {
		return 0
	}
	n := int64(bytes)
	whole := time.Duration(n/o.rateBytesPerSec) * time.Second
	fraction := time.Duration(n % o.rateBytesPerSec * int64(time.Second) / o.rateBytesPerSec)
	return whole + fraction
}

// fileStats accounts for what was actually pulled out of one entry.
type fileStats struct {
	Path        string
	Size        int64
	Opens       int
	Reads       int
	BytesServed int64
	Completed   int
	Aborted     int
}

// readSummary describes one reader's life, produced when it is closed.
type readSummary struct {
	Index          int
	Path           string
	Size           int64
	BytesServed    int64
	Reads          int
	Completed      bool
	FirstByteAfter time.Duration
	Lifetime       time.Duration
}

// syntheticSource serves deterministic content for the file entries of a tree.
type syntheticSource struct {
	entries      []entry
	opts         options
	blockPending atomic.Bool

	mu    sync.Mutex
	stats []fileStats
}

func newSyntheticSource(entries []entry, opts options) (*syntheticSource, error) {
	if err := opts.validate(); err != nil {
		return nil, err
	}
	s := &syntheticSource{entries: slices.Clone(entries), opts: opts, stats: make([]fileStats, len(entries))}
	for i, e := range entries {
		s.stats[i] = fileStats{Path: e.Path, Size: e.Size}
	}
	s.blockPending.Store(opts.blockRead > 0)
	return s, nil
}

func (s *syntheticSource) Entries() []entry {
	return slices.Clone(s.entries)
}

func (s *syntheticSource) Stats() []fileStats {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.stats)
}

func (s *syntheticSource) Open(index int) (*syntheticReader, error) {
	if index < 0 || index >= len(s.entries) {
		return nil, fmt.Errorf("%w: %d of %d", ErrIndexOutOfRange, index, len(s.entries))
	}
	e := s.entries[index]
	if e.Dir {
		return nil, fmt.Errorf("%w: %q", ErrDirectory, e.Path)
	}
	s.mu.Lock()
	s.stats[index].Opens++
	s.mu.Unlock()
	return &syntheticReader{source: s, index: index, size: e.Size, opened: time.Now(), reachedEnd: e.Size == 0}, nil
}

func (s *syntheticSource) claimBlock() bool {
	return s.blockPending.CompareAndSwap(true, false)
}

func (s *syntheticSource) recordRead(index, bytes int) {
	s.mu.Lock()
	s.stats[index].Reads++
	s.stats[index].BytesServed += int64(bytes)
	s.mu.Unlock()
}

func (s *syntheticSource) recordClose(index int, completed bool) {
	s.mu.Lock()
	if completed {
		s.stats[index].Completed++
	} else {
		s.stats[index].Aborted++
	}
	s.mu.Unlock()
}

// syntheticReader is one open handle on a file entry. Throttling is per reader: the channel is never faster than the
// configured rate, and a pause by the consumer does not buy a burst afterwards.
type syntheticReader struct {
	source *syntheticSource
	index  int
	size   int64
	opened time.Time

	mu             sync.Mutex
	pos            int64
	started        bool
	due            time.Time
	firstDelivered time.Time
	served         int64
	reads          int
	reachedEnd     bool
	closed         bool
	summary        readSummary
}

var (
	_ io.ReadSeeker = (*syntheticReader)(nil)
	_ io.ReaderAt   = (*syntheticReader)(nil)
)

func (r *syntheticReader) Read(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	n, err := r.deliver(p, r.pos)
	r.pos += int64(n)
	return n, err
}

func (r *syntheticReader) ReadAt(p []byte, offset int64) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	n, err := r.deliver(p, offset)
	if err == nil && n < len(p) {
		return n, io.EOF
	}
	return n, err
}

func (r *syntheticReader) Seek(offset int64, whence int) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return 0, ErrReaderClosed
	}
	var target int64
	switch whence {
	case io.SeekStart:
		target = offset
	case io.SeekCurrent:
		target = r.pos + offset
	case io.SeekEnd:
		target = r.size + offset
	default:
		return 0, fmt.Errorf("%w: %d", ErrInvalidWhence, whence)
	}
	if target < 0 {
		return 0, fmt.Errorf("%w: seek to %d", ErrNegativeOffset, target)
	}
	r.pos = target
	return target, nil
}

// Close ends the reader and accounts it as completed when the last byte of the file was delivered. Closing again
// returns the first summary.
func (r *syntheticReader) Close() readSummary {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return r.summary
	}
	r.closed = true
	r.summary = readSummary{
		Index:       r.index,
		Path:        r.source.entries[r.index].Path,
		Size:        r.size,
		BytesServed: r.served,
		Reads:       r.reads,
		Completed:   r.reachedEnd,
		Lifetime:    time.Since(r.opened),
	}
	if !r.firstDelivered.IsZero() {
		r.summary.FirstByteAfter = r.firstDelivered.Sub(r.opened)
	}
	r.source.recordClose(r.index, r.reachedEnd)
	return r.summary
}

func (r *syntheticReader) deliver(p []byte, offset int64) (int, error) {
	if r.closed {
		return 0, ErrReaderClosed
	}
	if offset < 0 {
		return 0, fmt.Errorf("%w: %d", ErrNegativeOffset, offset)
	}
	if len(p) == 0 {
		return 0, nil
	}
	if offset >= r.size {
		return 0, io.EOF
	}
	n := int(min(int64(len(p)), r.size-offset))
	r.begin()
	for i := range p[:n] {
		p[i] = contentByte(offset + int64(i))
	}
	r.pace(n)
	r.served += int64(n)
	r.reads++
	if offset+int64(n) == r.size {
		r.reachedEnd = true
	}
	r.source.recordRead(r.index, n)
	return n, nil
}

func (r *syntheticReader) begin() {
	if r.started {
		return
	}
	if r.source.claimBlock() {
		time.Sleep(r.source.opts.blockRead)
	}
	r.started = true
	r.due = time.Now().Add(r.source.opts.latency)
}

func (r *syntheticReader) pace(bytes int) {
	r.due = laterOf(r.due, time.Now()).Add(r.source.opts.transferTime(bytes))
	if wait := time.Until(r.due); wait > 0 {
		time.Sleep(wait)
	}
	if r.firstDelivered.IsZero() {
		r.firstDelivered = time.Now()
	}
}

func laterOf(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}
