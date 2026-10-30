package dnd

import (
	"errors"
	"slices"
	"strings"
	"testing"
)

func TestParseURIList_CRLFCommentsAndBlankLines(t *testing.T) {
	data := []byte("# a comment\r\nfile:///a\r\n\r\nfile:///b\r\n#another\r\nfile:///c")
	got, err := ParseURIList(data)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"file:///a", "file:///b", "file:///c"}; !slices.Equal(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestParseURIList_LFOnlyAndEmpty(t *testing.T) {
	got, err := ParseURIList([]byte("file:///a\nfile:///b\n"))
	if err != nil || len(got) != 2 {
		t.Fatalf("got %q %v", got, err)
	}
	got, err = ParseURIList(nil)
	if err != nil || len(got) != 0 {
		t.Fatalf("empty list: %q %v", got, err)
	}
}

func TestParseURIList_RejectsNUL(t *testing.T) {
	if _, err := ParseURIList([]byte("file:///a\x00\r\n")); !errors.Is(err, ErrMalformedURI) {
		t.Fatalf("got %v", err)
	}
}

func TestFilePath_DecodesPercentEscapes(t *testing.T) {
	for _, tc := range []struct{ uri, want string }{
		{"file:///home/u/a.txt", "/home/u/a.txt"},
		{"file:///home/u/a%20b.txt", "/home/u/a b.txt"},
		{"file:///home/u/caf%C3%A9", "/home/u/café"},
		{"file:///home/u/100%25", "/home/u/100%"},
		{"file:///home/u/a%23b", "/home/u/a#b"},
		{"file://localhost/home/u/a", "/home/u/a"},
		{"FILE:///home/u/a", "/home/u/a"},
		{"file:/home/u/a", "/home/u/a"},
		{"file:///home/u/raw space", "/home/u/raw space"},
	} {
		got, err := FilePath(tc.uri)
		if err != nil {
			t.Errorf("%s: %v", tc.uri, err)
			continue
		}
		if got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.uri, got, tc.want)
		}
	}
}

func TestFilePath_Errors(t *testing.T) {
	for _, tc := range []struct {
		uri  string
		want error
	}{
		{"http://example.org/a", ErrNotFileURI},
		{"/just/a/path", ErrNotFileURI},
		{"file://remote.example/a", ErrRemoteHost},
		{"file:///a%zz", ErrBadEscape},
		{"file:///a#fragment", ErrMalformedURI},
		{"file:///a?query", ErrMalformedURI},
		{"file:relative", ErrMalformedURI},
		{"file://user@localhost/a", ErrMalformedURI},
		{"file:///a\x01b", ErrMalformedURI},
	} {
		if _, err := FilePath(tc.uri); !errors.Is(err, tc.want) {
			t.Errorf("%q: got %v, want %v", tc.uri, err, tc.want)
		}
	}
}

func TestValidatePath_Accepts(t *testing.T) {
	for _, p := range []string{"/a", "/home/u/a b.txt", "/home/u/dir/", "/home/u/.hidden", "/home/u/café", "/a/b..c", "/" + strings.Repeat("x", MaxComponentBytes)} {
		if err := ValidatePath(p); err != nil {
			t.Errorf("%q: %v", p, err)
		}
	}
}

func TestValidatePath_Rejects(t *testing.T) {
	for _, tc := range []struct {
		name string
		path string
		want error
	}{
		{"relative", "home/u", ErrNotAbsolute},
		{"empty", "", ErrNotAbsolute},
		{"root", "/", ErrEmptyComponent},
		{"double slash", "/a//b", ErrEmptyComponent},
		{"dot", "/a/./b", ErrTraversal},
		{"dotdot", "/a/../b", ErrTraversal},
		{"trailing dotdot", "/a/..", ErrTraversal},
		{"nul", "/a\x00b", ErrControlChar},
		{"newline", "/a\nb", ErrControlChar},
		{"del", "/a\x7fb", ErrControlChar},
		{"colon", "/a:b", ErrReservedChar},
		{"asterisk", "/a*", ErrReservedChar},
		{"question", "/a?", ErrReservedChar},
		{"quote", "/a\"b", ErrReservedChar},
		{"pipe", "/a|b", ErrReservedChar},
		{"angle brackets", "/<a>", ErrReservedChar},
		{"invalid utf8", "/a\xffb", ErrInvalidUTF8},
		{"component too long", "/" + strings.Repeat("x", MaxComponentBytes+1), ErrComponentTooLong},
		{"path too long", strings.Repeat("/abcdefgh", MaxPathBytes/9+1), ErrPathTooLong},
	} {
		if err := ValidatePath(tc.path); !errors.Is(err, tc.want) {
			t.Errorf("%s: got %v, want %v", tc.name, err, tc.want)
		}
	}
}

func TestResolveURIList_RejectsEntriesIndividually(t *testing.T) {
	list := "file:///ok/one.txt\r\n" +
		"file:///ok/a%2F..%2Fescape\r\n" +
		"file:///ok/nul%00byte\r\n" +
		"file://other/host\r\n" +
		"http://example.org/x\r\n" +
		"file:///ok/two.txt\r\n"
	result, err := ResolveURIList([]byte(list))
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"/ok/one.txt", "/ok/two.txt"}; !slices.Equal(result.Paths, want) {
		t.Fatalf("paths %q", result.Paths)
	}
	want := []error{ErrTraversal, ErrControlChar, ErrRemoteHost, ErrNotFileURI}
	if len(result.Rejected) != len(want) {
		t.Fatalf("rejected %+v", result.Rejected)
	}
	for i, r := range result.Rejected {
		if !errors.Is(r.Err, want[i]) {
			t.Errorf("rejection %d (%s): %v, want %v", i, r.URI, r.Err, want[i])
		}
	}
}

func TestResolveURIList_TooManyEntriesFailsAsAWhole(t *testing.T) {
	line := "file:///a\r\n"
	if _, err := ResolveURIList([]byte(strings.Repeat(line, MaxEntries))); err != nil {
		t.Fatalf("exactly the limit: %v", err)
	}
	if _, err := ResolveURIList([]byte(strings.Repeat(line, MaxEntries+1))); !errors.Is(err, ErrTooManyEntries) {
		t.Fatalf("over the limit: %v", err)
	}
}

func TestResolveURIList_MalformedListFails(t *testing.T) {
	if _, err := ResolveURIList([]byte("file:///a\x00")); !errors.Is(err, ErrMalformedURI) {
		t.Fatalf("got %v", err)
	}
}

func TestEncodeURIList_RoundTrip(t *testing.T) {
	paths := []string{"/home/u/a b.txt", "/home/u/café", "/home/u/100%", "/home/u/a#b", "/home/u/dir/"}
	list, err := EncodeURIList(paths)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(list), "file:///home/u/a%20b.txt\r\nfile:///home/u/caf%C3%A9\r\n") {
		t.Fatalf("list %q", list)
	}
	if !strings.HasSuffix(string(list), "\r\n") {
		t.Fatalf("list %q is not CRLF terminated", list)
	}
	result, err := ResolveURIList(list)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Rejected) != 0 || !slices.Equal(result.Paths, paths) {
		t.Fatalf("round trip %+v", result)
	}
}

func TestEncodeURIList_Rejects(t *testing.T) {
	if _, err := EncodeURIList(nil); !errors.Is(err, ErrNoEntries) {
		t.Fatalf("empty: %v", err)
	}
	if _, err := EncodeURIList([]string{"relative"}); !errors.Is(err, ErrNotAbsolute) {
		t.Fatalf("relative: %v", err)
	}
	if _, err := EncodeURIList([]string{"/ok", "/bad|name"}); !errors.Is(err, ErrReservedChar) {
		t.Fatalf("reserved: %v", err)
	}
	if _, err := EncodeURIList(make([]string, MaxEntries+1)); !errors.Is(err, ErrTooManyEntries) {
		t.Fatalf("too many: %v", err)
	}
}
