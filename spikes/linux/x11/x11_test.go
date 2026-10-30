package x11

import (
	"slices"
	"testing"

	"github.com/jezek/xgb/xproto"
)

func TestOutgoing_ChunksAndEndMarker(t *testing.T) {
	tests := []struct {
		name  string
		data  []byte
		limit int
		want  []string
	}{
		{name: "shorter than limit", data: []byte("abc"), limit: 4, want: []string{"abc", ""}},
		{name: "exact multiple of limit", data: []byte("abcdefgh"), limit: 4, want: []string{"abcd", "efgh", ""}},
		{name: "remainder chunk", data: []byte("abcdefghij"), limit: 4, want: []string{"abcd", "efgh", "ij", ""}},
		{name: "limit of one byte", data: []byte("ab"), limit: 1, want: []string{"a", "b", ""}},
		{name: "empty payload sends only the end marker", data: nil, limit: 4, want: []string{""}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			o := &outgoing{data: tc.data}
			var got []string
			for range len(tc.data) + 2 {
				chunk, finished := o.next(tc.limit)
				if finished {
					break
				}
				got = append(got, string(chunk))
			}
			if !slices.Equal(got, tc.want) {
				t.Fatalf("chunks = %q, want %q", got, tc.want)
			}
			if _, finished := o.next(tc.limit); !finished {
				t.Fatal("transfer not finished after the end marker was acknowledged")
			}
		})
	}
}

func TestOutgoing_FinishedStaysFinished(t *testing.T) {
	o := &outgoing{data: []byte("a")}
	for range 2 {
		o.next(4)
	}
	for range 3 {
		chunk, finished := o.next(4)
		if !finished || chunk != nil {
			t.Fatalf("next after end = (%q, %t), want (nil, true)", chunk, finished)
		}
	}
}

func TestIncrReceiver_AddUntilEmptyChunk(t *testing.T) {
	tests := []struct {
		name   string
		chunks []string
		want   string
	}{
		{name: "two chunks then end", chunks: []string{"ab", "cd", ""}, want: "abcd"},
		{name: "one chunk then end", chunks: []string{"xyz", ""}, want: "xyz"},
		{name: "end marker first", chunks: []string{""}, want: ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var r incrReceiver
			for i, chunk := range tc.chunks {
				done := r.add([]byte(chunk))
				last := i == len(tc.chunks)-1
				if done != last {
					t.Fatalf("add(%q) done = %t, want %t", chunk, done, last)
				}
			}
			if string(r.data) != tc.want {
				t.Fatalf("data = %q, want %q", r.data, tc.want)
			}
		})
	}
}

func TestOwner_AdvanceIgnoresUnknownAndFinishedTransfers(t *testing.T) {
	const (
		window   = xproto.Window(7)
		property = xproto.Atom(9)
	)
	t.Run("unknown requestor or property is ignored", func(t *testing.T) {
		o := &Owner{transfers: map[transferKey]*outgoing{
			{window: window, property: property}: {target: 3, data: []byte("ab"), offset: 1},
		}}
		for _, ev := range []xproto.PropertyNotifyEvent{
			{Window: window + 1, Atom: property},
			{Window: window, Atom: property + 1},
		} {
			if err := o.advance(ev); err != nil {
				t.Fatalf("advance(%+v) = %v, want nil", ev, err)
			}
		}
		if len(o.transfers) != 1 {
			t.Fatalf("transfers = %d, want 1 untouched", len(o.transfers))
		}
	})
	t.Run("delete after the end marker removes the transfer", func(t *testing.T) {
		o := &Owner{transfers: map[transferKey]*outgoing{
			{window: window, property: property}: {target: 3, data: []byte("ab"), offset: 2, ended: true},
		}}
		if err := o.advance(xproto.PropertyNotifyEvent{Window: window, Atom: property}); err != nil {
			t.Fatalf("advance = %v, want nil", err)
		}
		if len(o.transfers) != 0 {
			t.Fatalf("transfers = %d, want 0 after the transfer finished", len(o.transfers))
		}
	})
}

func TestMaxChunkBytes(t *testing.T) {
	tests := []struct {
		name            string
		maxRequestBytes int
		want            int
	}{
		{name: "default server limit of 65535 units", maxRequestBytes: 65535 * 4, want: 65535*4 - requestSlackBytes},
		{name: "limit equal to slack", maxRequestBytes: requestSlackBytes, want: 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := maxChunkBytes(tc.maxRequestBytes); got != tc.want {
				t.Fatalf("maxChunkBytes(%d) = %d, want %d", tc.maxRequestBytes, got, tc.want)
			}
		})
	}
}

func TestUnitsOf(t *testing.T) {
	tests := []struct {
		name   string
		size   int
		format byte
		want   uint32
	}{
		{name: "bytes at format 8", size: 5, format: 8, want: 5},
		{name: "atoms at format 32", size: 12, format: 32, want: 3},
		{name: "empty at format 32", size: 0, format: 32, want: 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := unitsOf(tc.size, tc.format); got != tc.want {
				t.Fatalf("unitsOf(%d, %d) = %d, want %d", tc.size, tc.format, got, tc.want)
			}
		})
	}
}

func TestEncodeAtoms(t *testing.T) {
	tests := []struct {
		name  string
		atoms []xproto.Atom
		want  []byte
	}{
		{name: "no atoms", atoms: nil, want: []byte{}},
		{name: "two atoms little endian", atoms: []xproto.Atom{0x01020304, 4}, want: []byte{4, 3, 2, 1, 4, 0, 0, 0}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := encodeAtoms(tc.atoms)
			if !slices.Equal(got, tc.want) {
				t.Fatalf("encodeAtoms(%v) = %v, want %v", tc.atoms, got, tc.want)
			}
		})
	}
}

func TestFirstDifference(t *testing.T) {
	tests := []struct {
		name string
		a, b []byte
		want int
	}{
		{name: "equal slices", a: []byte("abc"), b: []byte("abc"), want: 3},
		{name: "differs in the middle", a: []byte("abXd"), b: []byte("abcd"), want: 2},
		{name: "first byte differs", a: []byte("x"), b: []byte("y"), want: 0},
		{name: "shorter prefix", a: []byte("ab"), b: []byte("abc"), want: 2},
		{name: "both empty", a: nil, b: nil, want: 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := firstDifference(tc.a, tc.b); got != tc.want {
				t.Fatalf("firstDifference(%q, %q) = %d, want %d", tc.a, tc.b, got, tc.want)
			}
		})
	}
}

func firstDifference(a, b []byte) int {
	for i := range min(len(a), len(b)) {
		if a[i] != b[i] {
			return i
		}
	}
	return min(len(a), len(b))
}
