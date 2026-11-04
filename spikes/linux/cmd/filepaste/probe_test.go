package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/kikz0r/edgesync/spikes/linux/dnd"
)

func digestOf(size int) string {
	var buf bytes.Buffer
	chunk := make([]byte, chunkBytes)
	for off := 0; off < size; off += chunkBytes {
		n := min(chunkBytes, size-off)
		fill(chunk[:n], int64(off))
		buf.Write(chunk[:n])
	}
	sum := sha256.Sum256(buf.Bytes())
	return hex.EncodeToString(sum[:])
}

func contentOf(size int) []byte {
	out := make([]byte, size)
	fill(out, 0)
	return out
}

func TestParseTargets(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		list    string
		want    []string
		wantErr error
	}{
		{name: "all", list: MimeURIList + "," + MimeGnomeFiles + "," + MimeKDECut, want: []string{MimeURIList, MimeGnomeFiles, MimeKDECut}},
		{name: "subset with spaces", list: " " + MimeGnomeFiles + " ", want: []string{MimeGnomeFiles}},
		{name: "unknown", list: "text/plain", wantErr: ErrUnknownTarget},
		{name: "empty entry", list: MimeURIList + ",", wantErr: ErrUnknownTarget},
		{name: "duplicate", list: MimeURIList + "," + MimeURIList, wantErr: ErrDuplicateTarget},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := ParseTargets(tc.list)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("ParseTargets(%q) error = %v, want %v", tc.list, err, tc.wantErr)
			}
			if tc.wantErr == nil && !equalStrings(got, tc.want) {
				t.Fatalf("ParseTargets(%q) = %q, want %q", tc.list, got, tc.want)
			}
		})
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestParseConfigDefaults(t *testing.T) {
	t.Parallel()
	cfg, err := ParseConfig(nil)
	if err != nil {
		t.Fatalf("ParseConfig(nil): %v", err)
	}
	want := Config{
		Mode:    ModeX11,
		Op:      OpCopy,
		Targets: knownTargets(),
		Delay:   5 * time.Second,
		Wait:    120 * time.Second,
		Size:    64 << 20,
		Staging: DefaultStagingDir,
		LogPath: DefaultLogPath,
	}
	if !equalStrings(cfg.Targets, want.Targets) {
		t.Fatalf("Targets = %q, want %q", cfg.Targets, want.Targets)
	}
	cfg.Targets, want.Targets = nil, nil
	if !reflect.DeepEqual(cfg, want) {
		t.Fatalf("ParseConfig(nil) = %+v, want %+v", cfg, want)
	}
}

func TestParseConfigRejects(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		args []string
		want error
	}{
		{name: "mode", args: []string{"-mode", "wayland"}, want: ErrBadMode},
		{name: "op", args: []string{"-op", "move"}, want: ErrBadOp},
		{name: "target", args: []string{"-targets", "image/png"}, want: ErrUnknownTarget},
		{name: "size", args: []string{"-size", "0"}, want: ErrBadSize},
		{name: "delay", args: []string{"-delay", "-1s"}},
		{name: "wait", args: []string{"-wait", "0s"}},
		{name: "staging", args: []string{"-staging", ""}},
		{name: "flag", args: []string{"-unknown"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := ParseConfig(tc.args)
			if err == nil {
				t.Fatalf("ParseConfig(%q) accepted invalid input", tc.args)
			}
			if tc.want != nil && !errors.Is(err, tc.want) {
				t.Fatalf("ParseConfig(%q) error = %v, want %v", tc.args, err, tc.want)
			}
		})
	}
}

func TestParseConfigAcceptsExplicitValues(t *testing.T) {
	t.Parallel()
	cfg, err := ParseConfig([]string{"-mode", ModePortal, "-op", OpCut, "-targets", MimeKDECut, "-delay", "30s", "-refuse"})
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}
	if cfg.Mode != ModePortal || cfg.Op != OpCut || !cfg.Refuse || cfg.Delay != 30*time.Second {
		t.Fatalf("ParseConfig = %+v", cfg)
	}
	if !equalStrings(cfg.Targets, []string{MimeKDECut}) {
		t.Fatalf("Targets = %q", cfg.Targets)
	}
}

func TestPayloadURIList(t *testing.T) {
	t.Parallel()
	got, err := Payload(MimeURIList, OpCopy, "/var/tmp/edgesync staging/a.bin")
	if err != nil {
		t.Fatalf("Payload: %v", err)
	}
	want := "file:///var/tmp/edgesync%20staging/a.bin\r\n"
	if string(got) != want {
		t.Fatalf("Payload = %q, want %q", got, want)
	}
}

func TestPayloadGnomeCopiedFiles(t *testing.T) {
	t.Parallel()
	const path = "/var/tmp/edgesync/a.bin"
	for _, op := range []string{OpCopy, OpCut} {
		got, err := Payload(MimeGnomeFiles, op, path)
		if err != nil {
			t.Fatalf("Payload(%s): %v", op, err)
		}
		want := op + "\nfile:///var/tmp/edgesync/a.bin"
		if string(got) != want {
			t.Fatalf("Payload(%s) = %q, want %q", op, got, want)
		}
		if strings.HasSuffix(string(got), "\n") {
			t.Fatalf("Payload(%s) ends with a newline, which Nautilus rejects", op)
		}
	}
}

func TestPayloadKDECutSelection(t *testing.T) {
	t.Parallel()
	for op, want := range map[string]string{OpCopy: "0", OpCut: "1"} {
		got, err := Payload(MimeKDECut, op, "/var/tmp/a.bin")
		if err != nil {
			t.Fatalf("Payload(%s): %v", op, err)
		}
		if string(got) != want {
			t.Fatalf("Payload(%s) = %q, want %q", op, got, want)
		}
	}
}

func TestPayloadErrors(t *testing.T) {
	t.Parallel()
	if _, err := Payload("image/png", OpCopy, "/a"); !errors.Is(err, ErrUnknownTarget) {
		t.Fatalf("unknown target error = %v", err)
	}
	if _, err := Payload(MimeURIList, "paste", "/a"); !errors.Is(err, ErrBadOp) {
		t.Fatalf("bad op error = %v", err)
	}
	if _, err := Payload(MimeURIList, OpCopy, "/a:b/c.bin"); !errors.Is(err, dnd.ErrReservedChar) {
		t.Fatalf("reserved character error = %v", err)
	}
}

func TestStagingName(t *testing.T) {
	t.Parallel()
	if got, want := StagingName(42, 7, 1024), "edgesync-filepaste-42-7-1024.bin"; got != want {
		t.Fatalf("StagingName = %q, want %q", got, want)
	}
}

func TestFillIsOffsetConsistent(t *testing.T) {
	t.Parallel()
	whole := contentOf(3*chunkBytes + 17)
	chunk := make([]byte, 1000)
	for off := 0; off < len(whole); off += len(chunk) {
		n := min(len(chunk), len(whole)-off)
		fill(chunk[:n], int64(off))
		if !bytes.Equal(chunk[:n], whole[off:off+n]) {
			t.Fatalf("fill at offset %d differs from the whole-file content", off)
		}
	}
}

func TestStagingWritesContentAndDigest(t *testing.T) {
	t.Parallel()
	const size = 2*chunkBytes + 123
	path := filepath.Join(t.TempDir(), "staged.bin")
	st, err := StartStaging(context.Background(), path, size)
	if err != nil {
		t.Fatalf("StartStaging: %v", err)
	}
	digest, err := st.Wait()
	if err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if digest != digestOf(size) {
		t.Fatalf("digest = %s, want %s", digest, digestOf(size))
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read staged file: %v", err)
	}
	if !bytes.Equal(data, contentOf(size)) {
		t.Fatalf("staged file has %d bytes that differ from the deterministic content", len(data))
	}
	state := st.Snapshot()
	if !state.Finished || state.Err != nil || state.Written != size {
		t.Fatalf("Snapshot = %+v, want finished with %d bytes", state, size)
	}
}

func TestStagingSnapshotWhileWriting(t *testing.T) {
	t.Parallel()
	const size = 4 * chunkBytes
	st, err := StartStaging(context.Background(), filepath.Join(t.TempDir(), "live.bin"), size)
	if err != nil {
		t.Fatalf("StartStaging: %v", err)
	}
	var last int64
	for {
		state := st.Snapshot()
		if state.Written < last {
			t.Fatalf("Written went backwards: %d after %d", state.Written, last)
		}
		last = state.Written
		if state.Finished {
			break
		}
	}
	if _, err := st.Wait(); err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if last != size {
		t.Fatalf("finished snapshot reported %d bytes, want %d", last, size)
	}
}

func TestStagingStopsWhenCancelled(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	st, err := StartStaging(ctx, filepath.Join(t.TempDir(), "cancelled.bin"), chunkBytes)
	if err != nil {
		t.Fatalf("StartStaging: %v", err)
	}
	if _, err := st.Wait(); !errors.Is(err, context.Canceled) {
		t.Fatalf("Wait error = %v, want context.Canceled", err)
	}
	state := st.Snapshot()
	if !state.Finished || !errors.Is(state.Err, context.Canceled) || state.Written != 0 {
		t.Fatalf("Snapshot = %+v, want finished with context.Canceled and no bytes", state)
	}
}

func TestStagingRejectsExistingFileAndBadSize(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	existing := filepath.Join(dir, "exists.bin")
	if err := os.WriteFile(existing, nil, 0o600); err != nil {
		t.Fatalf("prepare file: %v", err)
	}
	if _, err := StartStaging(context.Background(), existing, 10); !errors.Is(err, fs.ErrExist) {
		t.Fatalf("existing file error = %v, want fs.ErrExist", err)
	}
	if _, err := StartStaging(context.Background(), filepath.Join(dir, "zero.bin"), 0); !errors.Is(err, ErrBadSize) {
		t.Fatalf("zero size error = %v, want ErrBadSize", err)
	}
}

func TestExchangeDelay(t *testing.T) {
	t.Parallel()
	ex := Exchange{Target: MimeGnomeFiles, Requested: 3 * time.Second, Answered: 8 * time.Second}
	if got := ex.Delay(); got != 5*time.Second {
		t.Fatalf("Delay = %v, want 5s", got)
	}
}
