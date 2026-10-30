// Package dnd is the Linux drag-and-drop spike (P0-SP3-007). It holds the pure XDND protocol logic (message
// encoding, drag source and drop target state machines, text/uri-list parsing and path sanitising) and a driver over
// github.com/jezek/xgb that runs the state machines against a real X server. See README.md for the sources, the
// Wayland analysis and the GNOME indicator and Gio file-drop findings.
package dnd

import (
	"bytes"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"unicode/utf8"
)

var (
	ErrMalformedURI     = errors.New("dnd: malformed URI")
	ErrNotFileURI       = errors.New("dnd: URI is not a file URI")
	ErrRemoteHost       = errors.New("dnd: file URI names a remote host")
	ErrBadEscape        = errors.New("dnd: invalid percent escape")
	ErrNotAbsolute      = errors.New("dnd: path is not absolute")
	ErrInvalidUTF8      = errors.New("dnd: path is not valid UTF-8")
	ErrControlChar      = errors.New("dnd: path contains a NUL or control character")
	ErrTraversal        = errors.New("dnd: path contains a . or .. component")
	ErrEmptyComponent   = errors.New("dnd: path contains an empty component")
	ErrReservedChar     = errors.New("dnd: path component contains one of <>:\"|?*")
	ErrComponentTooLong = errors.New("dnd: path component is longer than 255 bytes")
	ErrPathTooLong      = errors.New("dnd: path is longer than 4096 bytes")
	ErrTooManyEntries   = errors.New("dnd: more than 100000 entries")
	ErrNoEntries        = errors.New("dnd: no paths to encode")
)

// Limits follow D-98 and D-99: 255 bytes per component, 4096 bytes per path, 100000 entries per drop.
const (
	MaxComponentBytes = 255
	MaxPathBytes      = 4096
	MaxEntries        = 100000

	reservedChars = `<>:"|?*`
)

type Rejection struct {
	URI string
	Err error
}

// DropResult is the outcome of resolving a dropped text/uri-list: the accepted absolute paths and, as D-98 requires,
// one explicit rejection per entry that failed while the rest continue.
type DropResult struct {
	Paths    []string
	Rejected []Rejection
}

// ParseURIList splits a text/uri-list (RFC 2483): one URI per line, CRLF or LF terminated, lines starting with "#"
// are comments and empty lines are ignored.
func ParseURIList(data []byte) ([]string, error) {
	if bytes.IndexByte(data, 0) >= 0 {
		return nil, fmt.Errorf("%w: NUL byte in the list", ErrMalformedURI)
	}
	var uris []string
	for line := range strings.SplitSeq(string(data), "\n") {
		line = strings.TrimSuffix(line, "\r")
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		uris = append(uris, line)
	}
	return uris, nil
}

// FilePath turns a file URI into the decoded absolute path. Only an empty host or "localhost" is accepted, and a
// query or fragment is refused (a literal "?" or "#" in a name must arrive percent-encoded).
func FilePath(uri string) (string, error) {
	u, err := url.Parse(uri)
	if err != nil {
		var escape url.EscapeError
		if errors.As(err, &escape) {
			return "", fmt.Errorf("%w: %q", ErrBadEscape, uri)
		}
		return "", fmt.Errorf("%w: %v", ErrMalformedURI, err)
	}
	if !strings.EqualFold(u.Scheme, "file") {
		return "", fmt.Errorf("%w: %q", ErrNotFileURI, uri)
	}
	if u.Opaque != "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
		return "", fmt.Errorf("%w: %q", ErrMalformedURI, uri)
	}
	if u.Host != "" && !strings.EqualFold(u.Host, "localhost") {
		return "", fmt.Errorf("%w: %q", ErrRemoteHost, u.Host)
	}
	return u.Path, nil
}

// ValidatePath applies the D-45, D-98 and D-99 rules to an absolute source path: no NUL or control characters, no
// "." or ".." or empty components, none of <>:"|?* in a component, components of at most 255 bytes and at most 4096
// bytes in total. One trailing slash (a directory) is tolerated.
func ValidatePath(p string) error {
	if !utf8.ValidString(p) {
		return ErrInvalidUTF8
	}
	if !strings.HasPrefix(p, "/") {
		return fmt.Errorf("%w: %q", ErrNotAbsolute, p)
	}
	if len(p) > MaxPathBytes {
		return fmt.Errorf("%w: %d bytes", ErrPathTooLong, len(p))
	}
	trimmed := p
	if len(trimmed) > 1 {
		trimmed = strings.TrimSuffix(trimmed, "/")
	}
	if trimmed == "/" {
		return fmt.Errorf("%w: the root directory", ErrEmptyComponent)
	}
	for _, r := range trimmed {
		if r < 0x20 || r == 0x7f {
			return ErrControlChar
		}
	}
	for component := range strings.SplitSeq(trimmed[1:], "/") {
		switch {
		case component == "":
			return fmt.Errorf("%w: %q", ErrEmptyComponent, p)
		case component == "." || component == "..":
			return fmt.Errorf("%w: %q", ErrTraversal, p)
		case len(component) > MaxComponentBytes:
			return fmt.Errorf("%w: %d bytes", ErrComponentTooLong, len(component))
		case strings.ContainsAny(component, reservedChars):
			return fmt.Errorf("%w: %q", ErrReservedChar, component)
		}
	}
	return nil
}

// ResolveURIList parses a dropped text/uri-list and sanitises every entry. A list over MaxEntries fails as a whole.
func ResolveURIList(data []byte) (DropResult, error) {
	uris, err := ParseURIList(data)
	if err != nil {
		return DropResult{}, err
	}
	if len(uris) > MaxEntries {
		return DropResult{}, fmt.Errorf("%w: %d", ErrTooManyEntries, len(uris))
	}
	var result DropResult
	for _, uri := range uris {
		p, err := FilePath(uri)
		if err == nil {
			err = ValidatePath(p)
		}
		if err != nil {
			result.Rejected = append(result.Rejected, Rejection{URI: uri, Err: err})
			continue
		}
		result.Paths = append(result.Paths, p)
	}
	return result, nil
}

// EncodeURIList builds the text/uri-list of absolute paths, one percent-encoded file URI per CRLF terminated line.
func EncodeURIList(paths []string) ([]byte, error) {
	if len(paths) == 0 {
		return nil, ErrNoEntries
	}
	if len(paths) > MaxEntries {
		return nil, fmt.Errorf("%w: %d", ErrTooManyEntries, len(paths))
	}
	var out bytes.Buffer
	for _, p := range paths {
		if err := ValidatePath(p); err != nil {
			return nil, fmt.Errorf("dnd: encode %q: %w", p, err)
		}
		out.WriteString((&url.URL{Scheme: "file", Path: p}).String())
		out.WriteString("\r\n")
	}
	return out.Bytes(), nil
}
