package portal

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
)

var (
	ErrNoPaths   = errors.New("portal: no file paths to offer")
	ErrBadCopyOp = errors.New("portal: copy or cut expected")
)

// FileURI returns the file:// URI of an absolute path, percent-encoded the way url.URL encodes it.
func FileURI(path string) string {
	return (&url.URL{Scheme: "file", Path: path}).String()
}

// URIListPayload is text/uri-list (RFC 2483): one URI per line, every line CRLF-terminated.
func URIListPayload(paths []string) ([]byte, error) {
	if len(paths) == 0 {
		return nil, ErrNoPaths
	}
	var b strings.Builder
	for _, p := range paths {
		b.WriteString(FileURI(p))
		b.WriteString("\r\n")
	}
	return []byte(b.String()), nil
}

// CopiedFilesPayload is x-special/gnome-copied-files: "copy" or "cut", LF, then the URIs joined by LF with no
// trailing LF. Nautilus rejects an empty line, so a trailing LF makes it refuse the whole selection.
func CopiedFilesPayload(op string, paths []string) ([]byte, error) {
	if op != "copy" && op != "cut" {
		return nil, fmt.Errorf("%w: %q", ErrBadCopyOp, op)
	}
	if len(paths) == 0 {
		return nil, ErrNoPaths
	}
	uris := make([]string, len(paths))
	for i, p := range paths {
		uris[i] = FileURI(p)
	}
	return []byte(op + "\n" + strings.Join(uris, "\n")), nil
}
