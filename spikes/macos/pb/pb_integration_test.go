//go:build integration && darwin

package pb

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kikz0r/edgesync/spikes/macos/fw"
)

const (
	readTimeout    = 10 * time.Second
	receiveTimeout = 10 * time.Second
	pollTestPeriod = 20 * time.Millisecond
	observeTimeout = 3 * time.Second
)

func newBoard(t *testing.T) (*fw.Lib, *Pasteboard) {
	t.Helper()
	lib, err := fw.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	board, err := New(lib)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { board.Clear() })
	return lib, board
}

// withTimeout turns a hang into a failure: a provider request that needs a running main run loop would block the
// reading thread forever.
func withTimeout[T any](t *testing.T, what string, fn func() (T, error)) T {
	t.Helper()
	type result struct {
		value T
		err   error
	}
	done := make(chan result, 1)
	go func() {
		v, err := fn()
		done <- result{v, err}
	}()
	select {
	case r := <-done:
		if r.err != nil {
			t.Fatalf("%s: %v", what, r.err)
		}
		return r.value
	case <-time.After(readTimeout):
		t.Fatalf("%s: no answer within %s (provider request needs a serviced run loop?)", what, readTimeout)
		panic("unreachable")
	}
}

func TestAppKitTypes_MatchLiterals(t *testing.T) {
	lib, _ := newBoard(t)
	types, err := ResolveAppKitTypes(lib)
	if err != nil {
		t.Fatalf("ResolveAppKitTypes: %v", err)
	}
	t.Logf("AppKit values: %+v", types)
	for _, c := range []struct{ name, got, want string }{
		{"NSPasteboardTypeString", types.String, TypePlainText},
		{"NSPasteboardTypeHTML", types.HTML, TypeHTML},
		{"NSPasteboardTypeRTF", types.RTF, TypeRTF},
		{"NSPasteboardTypePNG", types.PNG, TypePNG},
		{"NSPasteboardTypeFileURL", types.FileURL, TypeFileURL},
	} {
		if c.got != c.want {
			t.Errorf("%s = %q, spike literal is %q", c.name, c.got, c.want)
		}
	}
}

func TestPasteboard_ChangeCountPolling(t *testing.T) {
	_, board := newBoard(t)

	start := board.ChangeCount()
	var observed atomic.Int64
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	watching := make(chan error, 1)
	go func() {
		watching <- board.Watch(ctx, pollTestPeriod, start, func(count int) { observed.Store(int64(count)) })
	}()

	cleared := board.Clear()
	if cleared <= start {
		t.Fatalf("clearContents returned change count %d, want more than %d", cleared, start)
	}
	if err := board.SetData(TypePlainText, []byte("poll")); err != nil {
		t.Fatalf("SetData: %v", err)
	}
	afterWrite := board.ChangeCount()
	// macOS bumps changeCount on ownership changes (clearContents, declareTypes:owner:), not on setData:forType:.
	t.Logf("change count: start %d, after clear %d, after write %d (write moved the count: %t)",
		start, cleared, afterWrite, afterWrite != cleared)

	deadline := time.Now().Add(observeTimeout)
	for observed.Load() <= int64(start) && time.Now().Before(deadline) {
		time.Sleep(pollTestPeriod)
	}
	cancel()
	if err := <-watching; !errors.Is(err, context.Canceled) {
		t.Fatalf("Watch returned %v, want context.Canceled", err)
	}
	got := int(observed.Load())
	t.Logf("Watch reported change count %d", got)
	if got <= start || got < cleared {
		t.Errorf("Watch reported change count %d, want more than %d and at least %d", got, start, cleared)
	}
}

func TestProvider_LazyTextHTMLRTFPNG(t *testing.T) {
	lib, board := newBoard(t)
	content, err := SampleContent()
	if err != nil {
		t.Fatalf("SampleContent: %v", err)
	}
	provider, err := NewProvider(lib, ProviderConfig{ClassName: "ESSpikeTestProvider", Render: RendererFor(content)})
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	t.Cleanup(provider.Close)
	t.Cleanup(func() { board.Clear() })

	pbTypes := []string{TypePlainText, TypeHTML, TypeRTF, TypePNG}
	if err := provider.Write(board, pbTypes...); err != nil {
		t.Fatalf("Write: %v", err)
	}
	for _, pbType := range pbTypes {
		if n := provider.Calls(pbType); n != 0 {
			t.Errorf("provider called %d times for %s right after the write, want 0 (lazy)", n, pbType)
		}
	}

	announced, err := board.Types()
	if err != nil {
		t.Fatalf("Types: %v", err)
	}
	t.Logf("types on the pasteboard after the write: %v", announced)
	for _, pbType := range pbTypes {
		if !slices.Contains(announced, pbType) {
			t.Errorf("type %s is not announced", pbType)
		}
	}
	if n := provider.Calls(TypePlainText) + provider.Calls(TypeHTML) + provider.Calls(TypeRTF) + provider.Calls(TypePNG); n != 0 {
		t.Errorf("reading the type list invoked the provider %d times, want 0", n)
	}

	for _, pbType := range pbTypes {
		got := withTimeout(t, "read "+pbType, func() ([]byte, error) { return board.Data(pbType) })
		if !bytes.Equal(got, content[pbType]) {
			t.Errorf("%s: read %d bytes, want %d matching bytes", pbType, len(got), len(content[pbType]))
		}
		if n := provider.Calls(pbType); n != 1 {
			t.Errorf("%s: provider called %d times after the first read, want 1", pbType, n)
		}
	}
	for _, pbType := range pbTypes {
		withTimeout(t, "second read "+pbType, func() ([]byte, error) { return board.Data(pbType) })
		t.Logf("%s: provider calls after the second read: %d", pbType, provider.Calls(pbType))
	}
	if err := provider.Err(); err != nil {
		t.Errorf("provider callbacks failed: %v", err)
	}

	board.Clear()
	t.Logf("pasteboardFinishedWithDataProvider: calls after clearContents: %d", provider.Finished())
}

func TestConcealedType_Detected(t *testing.T) {
	_, board := newBoard(t)

	board.Clear()
	if err := board.SetData(TypePlainText, []byte("secret")); err != nil {
		t.Fatalf("SetData text: %v", err)
	}
	if err := board.SetData(TypeConcealed, []byte("1")); err != nil {
		t.Fatalf("SetData concealed: %v", err)
	}
	concealed, err := board.IsConcealed()
	if err != nil {
		t.Fatalf("IsConcealed: %v", err)
	}
	if !concealed {
		types, _ := board.Types()
		t.Errorf("concealed marker not detected, types: %v", types)
	}

	board.Clear()
	if err := board.SetData(TypePlainText, []byte("plain")); err != nil {
		t.Fatalf("SetData plain: %v", err)
	}
	concealed, err = board.IsConcealed()
	if err != nil {
		t.Fatalf("IsConcealed: %v", err)
	}
	if concealed {
		t.Error("plain text reported as concealed")
	}
}

func TestPromise_ProviderRegistersAndWritesFile(t *testing.T) {
	lib, board := newBoard(t)
	contents := []byte("EdgeSync promised file")
	promise, err := NewPromise(lib, PromiseConfig{
		ClassName: "ESSpikeTestPromiseDelegate",
		FileType:  PromiseFileType,
		FileName:  "promised.bin",
		Contents:  contents,
	})
	if err != nil {
		t.Fatalf("NewPromise: %v", err)
	}
	t.Cleanup(promise.Close)
	t.Cleanup(func() { board.Clear() })

	dir := t.TempDir()
	path := withTimeout(t, "fulfil directly", func() (string, error) { return promise.FulfillDirect(dir) })
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read promised file: %v", err)
	}
	if !bytes.Equal(got, contents) {
		t.Errorf("promised file holds %q, want %q", got, contents)
	}
	if want := filepath.Join(dir, "promised.bin"); path != want {
		t.Errorf("promised file at %s, want %s", path, want)
	}
	if err := promise.Err(); err != nil {
		t.Errorf("delegate callbacks failed: %v", err)
	}

	if err := promise.Write(board); err != nil {
		t.Fatalf("Write: %v", err)
	}
	types, err := board.Types()
	if err != nil {
		t.Fatalf("Types: %v", err)
	}
	t.Logf("types on the pasteboard after writing the promise: %v", types)

	receiveDir := t.TempDir()
	ctx, cancel := context.WithTimeout(t.Context(), receiveTimeout)
	defer cancel()
	received, err := ReceivePromised(ctx, board, receiveDir)
	switch {
	case err == nil:
		t.Logf("NSFilePromiseReceiver fulfilled the promise in-process: %v, delegate name requests: %d, written: %v",
			received, promise.NameRequests(), promise.Written())
		for _, p := range received {
			data, err := os.ReadFile(p)
			if err != nil {
				t.Errorf("read received file %s: %v", p, err)
			} else if !bytes.Equal(data, contents) {
				t.Errorf("received file %s holds %q, want %q", p, data, contents)
			}
		}
	case errors.Is(err, ErrNoPromise), errors.Is(err, context.DeadlineExceeded):
		t.Logf("OBSERVED: in-process NSFilePromiseReceiver path did not fulfil the promise: %v", err)
	default:
		t.Errorf("ReceivePromised: %v", err)
	}
}
