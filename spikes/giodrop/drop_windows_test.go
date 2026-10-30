package main

import (
	"encoding/binary"
	"errors"
	"image"
	"slices"
	"testing"

	"golang.org/x/sys/windows"

	"github.com/kikz0r/edgesync/spikes/internal/wincom"
)

const maxCallbacks = 64

func buildDropFiles(t *testing.T, names ...string) []byte {
	t.Helper()
	raw := make([]byte, dropFilesHeaderSize)
	binary.LittleEndian.PutUint32(raw, uint32(dropFilesHeaderSize))
	binary.LittleEndian.PutUint32(raw[dropFilesWideOffset:], 1)
	for _, name := range names {
		units, err := windows.UTF16FromString(name)
		if err != nil {
			t.Fatalf("UTF16FromString(%q): %v", name, err)
		}
		for _, u := range units {
			raw = binary.LittleEndian.AppendUint16(raw, u)
		}
	}
	return binary.LittleEndian.AppendUint16(raw, 0)
}

func TestDropFilesLayout(t *testing.T) {
	if dropFilesHeaderSize != 20 {
		t.Fatalf("DROPFILES size = %d, want 20", dropFilesHeaderSize)
	}
	if dropFilesWideOffset != 16 {
		t.Fatalf("DROPFILES fWide offset = %d, want 16", dropFilesWideOffset)
	}
}

func TestParseDropFilesWideList(t *testing.T) {
	names := []string{`C:\Users\kikz0r\a.txt`, `D:\Ünïcode\b c.bin`}
	got, err := parseDropFiles(buildDropFiles(t, names...))
	if err != nil {
		t.Fatalf("parseDropFiles: %v", err)
	}
	if !slices.Equal(got, names) {
		t.Fatalf("parseDropFiles = %q, want %q", got, names)
	}
}

func TestParseDropFilesEmptyList(t *testing.T) {
	got, err := parseDropFiles(buildDropFiles(t))
	if err != nil {
		t.Fatalf("parseDropFiles: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("parseDropFiles = %q, want no names", got)
	}
}

func TestParseDropFilesRejects(t *testing.T) {
	valid := buildDropFiles(t, `C:\a.txt`)
	badOffset := slices.Clone(valid)
	binary.LittleEndian.PutUint32(badOffset, uint32(len(valid)+1))
	ansi := slices.Clone(valid)
	binary.LittleEndian.PutUint32(ansi[dropFilesWideOffset:], 0)
	oddBytes := append(slices.Clone(valid), 0)

	tests := []struct {
		name string
		raw  []byte
		want error
	}{
		{name: "short header", raw: valid[:dropFilesHeaderSize-1], want: ErrMalformedDropFiles},
		{name: "offset past end", raw: badOffset, want: ErrMalformedDropFiles},
		{name: "missing terminator", raw: valid[:len(valid)-2], want: ErrMalformedDropFiles},
		{name: "odd byte count", raw: oddBytes, want: ErrMalformedDropFiles},
		{name: "ANSI list", raw: ansi, want: ErrANSIDropFiles},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := parseDropFiles(tt.raw); !errors.Is(err, tt.want) {
				t.Fatalf("parseDropFiles error = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestNormalisePath(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "dot dot and trailing separator", in: `C:\Users\x\..\shared\`, want: `C:\Users\shared`},
		{name: "UNC share", in: `\\server\share\dir\`, want: `\\server\share\dir`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalisePath(tt.in)
			if err != nil {
				t.Fatalf("normalisePath(%q): %v", tt.in, err)
			}
			if got != tt.want {
				t.Fatalf("normalisePath(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
	for _, in := range []string{`docs\a.txt`, ""} {
		if _, err := normalisePath(in); !errors.Is(err, ErrRelativeDropPath) {
			t.Fatalf("normalisePath(%q) error = %v, want %v", in, err, ErrRelativeDropPath)
		}
	}
}

func TestNegotiateCopy(t *testing.T) {
	tests := []struct {
		name    string
		allowed uint32
		want    uint32
	}{
		{name: "copy and move", allowed: wincom.DropEffectCopy | wincom.DropEffectMove, want: wincom.DropEffectCopy},
		{name: "move only", allowed: wincom.DropEffectMove, want: wincom.DropEffectNone},
		{name: "nothing allowed", allowed: 0, want: wincom.DropEffectNone},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := negotiateCopy(tt.allowed); got != tt.want {
				t.Fatalf("negotiateCopy(%d) = %d, want %d", tt.allowed, got, tt.want)
			}
		})
	}
}

func TestDropPointDp(t *testing.T) {
	x, y := dropPointDp(image.Pt(300, 150), 1.5)
	if x != 200 || y != 100 {
		t.Fatalf("dropPointDp = (%v, %v), want (200, 100)", x, y)
	}
}

func TestDropTargetRecordsDropsAndErrors(t *testing.T) {
	target := newDropTarget(func() {})
	if got := target.snapshot(); got.Drops != 0 || got.Err != nil || got.Registered {
		t.Fatalf("initial snapshot = %+v, want empty", got)
	}

	boom := errors.New("boom")
	target.recordError(boom)
	ev := dropEvent{Paths: []string{`C:\a.txt`}, Client: image.Pt(10, 20)}
	target.recordDrop(ev)
	got := target.snapshot()
	if got.Drops != 1 || got.Err != nil || !slices.Equal(got.Last.Paths, ev.Paths) || got.Last.Client != ev.Client {
		t.Fatalf("snapshot after drop = %+v, want drops 1, last %+v, no error", got, ev)
	}

	target.recordError(boom)
	if got := target.snapshot(); got.Drops != 1 || !errors.Is(got.Err, boom) {
		t.Fatalf("snapshot after failed drop = %+v, want drops 1 and error %v", got, boom)
	}
}

func TestDropTargetObjectAndCallbackBudget(t *testing.T) {
	before := wincom.ObjectCount()
	obj, err := wincom.NewDropTarget(newDropTarget(func() {}))
	if err != nil {
		t.Fatalf("NewDropTarget: %v", err)
	}
	if got := wincom.CallbackCount(); got > maxCallbacks {
		t.Fatalf("callbacks created = %d, want at most %d", got, maxCallbacks)
	}
	wincom.Release(obj)
	if got := wincom.ObjectCount(); got != before {
		t.Fatalf("hosted objects = %d after release, want %d", got, before)
	}
}
