// Command filepaste is the Linux lazy file-paste probe (P0-SP3-006). It owns the clipboard on X11 (-mode x11) or
// through the Clipboard portal (-mode portal) and offers file targets whose data is produced only when a file manager
// asks for it: every data request starts a synthetic file in the staging directory, waits -delay and then answers with
// the file:// URI. This file holds the platform-neutral parts (flags, target and payload encoding, the staging writer,
// the exchange log model) so they run on any host; the X11 and portal drivers are in main.go (linux only).
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"time"

	"github.com/kikz0r/edgesync/spikes/linux/dnd"
)

const (
	MimeURIList    = "text/uri-list"
	MimeGnomeFiles = "x-special/gnome-copied-files"
	MimeKDECut     = "application/x-kde-cutselection"

	ModeX11    = "x11"
	ModePortal = "portal"

	OpCopy = "copy"
	OpCut  = "cut"

	DefaultStagingDir = "out/filepaste-staging"
	DefaultLogPath    = "out/filepaste.log"

	chunkBytes = 64 << 10
)

var (
	ErrUnknownTarget   = errors.New("filepaste: unknown target")
	ErrDuplicateTarget = errors.New("filepaste: target listed twice")
	ErrBadOp           = errors.New("filepaste: operation must be copy or cut")
	ErrBadMode         = errors.New("filepaste: mode must be x11 or portal")
	ErrBadSize         = errors.New("filepaste: staging size must be positive")
)

func knownTargets() []string {
	return []string{MimeURIList, MimeGnomeFiles, MimeKDECut}
}

// ParseTargets reads the -targets list. Every entry must be a known file target and appear once.
func ParseTargets(list string) ([]string, error) {
	var out []string
	for _, field := range strings.Split(list, ",") {
		target := strings.TrimSpace(field)
		if !slices.Contains(knownTargets(), target) {
			return nil, fmt.Errorf("%w: %q", ErrUnknownTarget, target)
		}
		if slices.Contains(out, target) {
			return nil, fmt.Errorf("%w: %q", ErrDuplicateTarget, target)
		}
		out = append(out, target)
	}
	return out, nil
}

func validOp(op string) error {
	switch op {
	case OpCopy, OpCut:
		return nil
	default:
		return fmt.Errorf("%w: %q", ErrBadOp, op)
	}
}

// Payload is the data offered for one target when the file at path is the answer. The copy or cut marker is written
// the way each convention expects it: a first line for x-special/gnome-copied-files (no trailing newline, because the
// Nautilus parser rejects an empty line), and "1" or "0" for application/x-kde-cutselection.
func Payload(target, op, path string) ([]byte, error) {
	if err := validOp(op); err != nil {
		return nil, err
	}
	switch target {
	case MimeURIList:
		return encodeURIs(path)
	case MimeGnomeFiles:
		uris, err := encodeURIs(path)
		if err != nil {
			return nil, err
		}
		return []byte(op + "\n" + strings.TrimSuffix(string(uris), "\r\n")), nil
	case MimeKDECut:
		if op == OpCut {
			return []byte("1"), nil
		}
		return []byte("0"), nil
	default:
		return nil, fmt.Errorf("%w: %q", ErrUnknownTarget, target)
	}
}

func encodeURIs(path string) ([]byte, error) {
	list, err := dnd.EncodeURIList([]string{path})
	if err != nil {
		return nil, fmt.Errorf("filepaste: encode %q: %w", path, err)
	}
	return list, nil
}

// StagingName is the file name of one synthetic file: the run tag keeps files of separate runs apart, the serial
// separates the requests of one run.
func StagingName(runTag int64, serial uint32, size int64) string {
	return fmt.Sprintf("edgesync-filepaste-%d-%d-%d.bin", runTag, serial, size)
}

// fill writes the deterministic content at offset off, so a pasted copy can be checked with sha256sum against the
// digest the probe logs when the file is finished.
func fill(buf []byte, off int64) {
	for i := range buf {
		v := uint64(off) + uint64(i)
		buf[i] = byte(v*131 + (v >> 8))
	}
}

// Staging writes one synthetic file in the background. Written is readable at any time; the digest and the final
// error are available from Wait once the writer has finished.
type Staging struct {
	path    string
	size    int64
	written atomic.Int64
	done    chan struct{}
	digest  string
	err     error
}

// StagingState is a point-in-time view of a Staging. Finished and Err are only meaningful once Finished is true.
type StagingState struct {
	Written  int64
	Finished bool
	Err      error
}

// StartStaging creates path exclusively and writes size bytes of the deterministic content in a goroutine that stops
// when ctx is done.
func StartStaging(ctx context.Context, path string, size int64) (*Staging, error) {
	if size <= 0 {
		return nil, fmt.Errorf("%w: %d", ErrBadSize, size)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, fmt.Errorf("filepaste: create staging file: %w", err)
	}
	s := &Staging{path: path, size: size, done: make(chan struct{})}
	go s.run(ctx, f)
	return s, nil
}

func (s *Staging) run(ctx context.Context, f *os.File) {
	defer close(s.done)
	h := sha256.New()
	buf := make([]byte, chunkBytes)
	var off int64
	for off < s.size {
		if err := ctx.Err(); err != nil {
			s.err = fmt.Errorf("filepaste: staging stopped at %d of %d bytes: %w", off, s.size, context.Cause(ctx))
			break
		}
		n := min(int64(chunkBytes), s.size-off)
		fill(buf[:n], off)
		if _, err := f.Write(buf[:n]); err != nil {
			s.err = fmt.Errorf("filepaste: write staging file: %w", err)
			break
		}
		h.Write(buf[:n])
		off += n
		s.written.Store(off)
	}
	if err := f.Close(); err != nil {
		s.err = errors.Join(s.err, fmt.Errorf("filepaste: close staging file: %w", err))
	}
	if s.err == nil {
		s.digest = hex.EncodeToString(h.Sum(nil))
	}
}

func (s *Staging) Path() string {
	return s.path
}

func (s *Staging) Snapshot() StagingState {
	st := StagingState{Written: s.written.Load()}
	select {
	case <-s.done:
		st.Finished = true
		st.Err = s.err
	default:
	}
	return st
}

// Wait blocks until the writer has finished and returns the sha256 of the complete file.
func (s *Staging) Wait() (string, error) {
	<-s.done
	return s.digest, s.err
}

// Outcome is how one data request was answered.
type Outcome string

const (
	OutcomeAnswered   Outcome = "answered"
	OutcomeRefused    Outcome = "refused"
	OutcomeNotOffered Outcome = "not-offered"
	OutcomeFailed     Outcome = "failed"
	OutcomeCancelled  Outcome = "cancelled"
)

// Exchange is one data request as the probe saw it. Requested and Answered are offsets from the start of the run.
type Exchange struct {
	Target    string
	Requested time.Duration
	Answered  time.Duration
	Outcome   Outcome
	Bytes     int64
	Complete  bool
}

func (e Exchange) Delay() time.Duration {
	return e.Answered - e.Requested
}

type Config struct {
	Mode    string
	Op      string
	Targets []string
	Delay   time.Duration
	Wait    time.Duration
	Size    int64
	Staging string
	LogPath string
	Refuse  bool
}

func ParseConfig(args []string) (Config, error) {
	var cfg Config
	var targets string
	flags := flag.NewFlagSet("filepaste", flag.ContinueOnError)
	flags.StringVar(&cfg.Mode, "mode", ModeX11, "x11 (own CLIPBOARD on the X server in DISPLAY) or portal (Clipboard portal on a RemoteDesktop session)")
	flags.StringVar(&cfg.Op, "op", OpCopy, "copy or cut: the marker written into x-special/gnome-copied-files and application/x-kde-cutselection")
	flags.StringVar(&targets, "targets", strings.Join(knownTargets(), ","), "comma-separated file targets to offer")
	flags.DurationVar(&cfg.Delay, "delay", 5*time.Second, "pause between a data request and its answer")
	flags.DurationVar(&cfg.Wait, "wait", 120*time.Second, "how long the run serves requests")
	flags.Int64Var(&cfg.Size, "size", 64<<20, "bytes of each synthetic staging file")
	flags.StringVar(&cfg.Staging, "staging", DefaultStagingDir, "directory the synthetic files are written to")
	flags.StringVar(&cfg.LogPath, "log", DefaultLogPath, "log file (appended; the same lines go to stdout)")
	flags.BoolVar(&cfg.Refuse, "refuse", false, "refuse every data request after -delay (the file is still staged)")
	if err := flags.Parse(args); err != nil {
		return Config{}, err
	}
	switch cfg.Mode {
	case ModeX11, ModePortal:
	default:
		return Config{}, fmt.Errorf("%w: %q", ErrBadMode, cfg.Mode)
	}
	if err := validOp(cfg.Op); err != nil {
		return Config{}, err
	}
	parsed, err := ParseTargets(targets)
	if err != nil {
		return Config{}, err
	}
	cfg.Targets = parsed
	if cfg.Delay < 0 {
		return Config{}, errors.New("filepaste: -delay must not be negative")
	}
	if cfg.Wait <= 0 {
		return Config{}, errors.New("filepaste: -wait must be positive")
	}
	if cfg.Size <= 0 {
		return Config{}, fmt.Errorf("%w: -size %d", ErrBadSize, cfg.Size)
	}
	if cfg.Staging == "" {
		return Config{}, errors.New("filepaste: -staging must not be empty")
	}
	return cfg, nil
}

func stagingPath(dir string, runTag int64, serial uint32, size int64) string {
	return filepath.Join(dir, StagingName(runTag, serial, size))
}
