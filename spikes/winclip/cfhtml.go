package main

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

var (
	errCFHTMLHeader     = errors.New("CF_HTML header is malformed")
	errCFHTMLOffsets    = errors.New("CF_HTML fragment offsets are out of range")
	errTextUnterminated = errors.New("clipboard payload has no NUL terminator")
	errTextOddLength    = errors.New("UTF-16 payload has an odd byte length")
	errTextNotUTF8      = errors.New("text is not valid UTF-8")
	errTextNotUTF16     = errors.New("UTF-16 payload contains an unpaired surrogate")
)

const (
	cfHTMLVersion      = "Version:1.0"
	cfHTMLStartMarker  = "<!--StartFragment-->"
	cfHTMLEndMarker    = "<!--EndFragment-->"
	cfHTMLBodyPrefix   = "<html><body>\r\n"
	cfHTMLBodySuffix   = "\r\n</body></html>"
	cfHTMLOffsetDigits = 10
)

var cfHTMLKeywords = []string{"Version", "StartHTML", "EndHTML", "StartFragment", "EndFragment", "StartSelection", "EndSelection"}

// The header is fixed width: every offset is written with cfHTMLOffsetDigits digits, so its length does not depend on
// the offset values and the body offsets can be computed before the header is written.
func formatCFHTMLHeader(startHTML, endHTML, startFragment, endFragment int) []byte {
	var b strings.Builder
	b.WriteString(cfHTMLVersion + "\r\n")
	fmt.Fprintf(&b, "StartHTML:%0*d\r\n", cfHTMLOffsetDigits, startHTML)
	fmt.Fprintf(&b, "EndHTML:%0*d\r\n", cfHTMLOffsetDigits, endHTML)
	fmt.Fprintf(&b, "StartFragment:%0*d\r\n", cfHTMLOffsetDigits, startFragment)
	fmt.Fprintf(&b, "EndFragment:%0*d\r\n", cfHTMLOffsetDigits, endFragment)
	return []byte(b.String())
}

// buildCFHTML wraps a UTF-8 HTML fragment in the CF_HTML description header. The offsets are byte offsets from the
// start of the returned buffer; StartFragment and EndFragment bracket the fragment text and exclude the marker comments.
func buildCFHTML(fragment []byte) []byte {
	headerLen := len(formatCFHTMLHeader(0, 0, 0, 0))
	var body bytes.Buffer
	body.WriteString(cfHTMLBodyPrefix)
	body.WriteString(cfHTMLStartMarker)
	startFragment := headerLen + body.Len()
	body.Write(fragment)
	endFragment := startFragment + len(fragment)
	body.WriteString(cfHTMLEndMarker)
	body.WriteString(cfHTMLBodySuffix)
	startHTML := headerLen
	endHTML := headerLen + body.Len()

	out := formatCFHTMLHeader(startHTML, endHTML, startFragment, endFragment)
	return append(out, body.Bytes()...)
}

// parseCFHTML returns the fragment bracketed by the StartFragment and EndFragment offsets. It accepts any of the three
// header line endings and the optional selection keywords, and rejects offsets outside the buffer.
func parseCFHTML(data []byte) ([]byte, error) {
	offsets := make(map[string]int, len(cfHTMLKeywords))
	sawVersion := false
	rest := data
	for {
		line, next, ok := nextHeaderLine(rest)
		if !ok {
			break
		}
		key, value, found := strings.Cut(line, ":")
		if !found || !slices.Contains(cfHTMLKeywords, key) {
			break
		}
		rest = next
		if key == "Version" {
			sawVersion = true
			continue
		}
		number, err := strconv.Atoi(value)
		if err != nil {
			return nil, fmt.Errorf("%w: %s value %q: %w", errCFHTMLHeader, key, value, err)
		}
		offsets[key] = number
	}

	if !sawVersion {
		return nil, fmt.Errorf("%w: Version line missing", errCFHTMLHeader)
	}
	start, hasStart := offsets["StartFragment"]
	end, hasEnd := offsets["EndFragment"]
	if !hasStart || !hasEnd {
		return nil, fmt.Errorf("%w: StartFragment or EndFragment missing", errCFHTMLHeader)
	}
	if start < 0 || end < start || end > len(data) {
		return nil, fmt.Errorf("%w: StartFragment=%d EndFragment=%d length=%d", errCFHTMLOffsets, start, end, len(data))
	}
	return data[start:end], nil
}

// nextHeaderLine splits off the first line of b. A line ends at CRLF, LF or a lone CR.
func nextHeaderLine(b []byte) (line string, rest []byte, ok bool) {
	i := bytes.IndexAny(b, "\r\n")
	if i < 0 {
		return "", nil, false
	}
	skip := i + 1
	if b[i] == '\r' && skip < len(b) && b[skip] == '\n' {
		skip++
	}
	return string(b[:i]), b[skip:], true
}

// htmlPayload is the NUL-terminated CF_HTML clipboard payload for a fragment.
func htmlPayload(fragment []byte) []byte {
	return nulTerminate(buildCFHTML(fragment))
}

// fragmentFromHTMLPayload reads the NUL-terminated CF_HTML payload and returns its fragment.
func fragmentFromHTMLPayload(payload []byte) ([]byte, error) {
	document, err := cstring(payload)
	if err != nil {
		return nil, err
	}
	return parseCFHTML(document)
}

// rtfPayload is the Rich Text Format clipboard payload: the RTF bytes unchanged plus one NUL terminator.
func rtfPayload(rtf []byte) []byte {
	return nulTerminate(rtf)
}

// rtfFromPayload returns the RTF bytes that precede the terminating NUL.
func rtfFromPayload(payload []byte) ([]byte, error) {
	return cstring(payload)
}

// textToCFUnicodeText converts UTF-8 text to the CF_UNICODETEXT payload: LF becomes CRLF (existing CRLF is kept
// as is), the text is UTF-16LE and a 2-byte NUL terminates it.
func textToCFUnicodeText(text string) ([]byte, error) {
	if !utf8.ValidString(text) {
		return nil, errTextNotUTF8
	}
	normalized := strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\n", "\r\n")
	units := append(utf16.Encode([]rune(normalized)), 0)
	out := make([]byte, 0, 2*len(units))
	for _, unit := range units {
		out = binary.LittleEndian.AppendUint16(out, unit)
	}
	return out, nil
}

// cfUnicodeTextToUTF8 reads a NUL-terminated UTF-16LE payload and returns UTF-8 text with LF line ends.
func cfUnicodeTextToUTF8(payload []byte) (string, error) {
	if len(payload)%2 != 0 {
		return "", errTextOddLength
	}
	units := make([]uint16, 0, len(payload)/2)
	terminated := false
	for i := 0; i < len(payload); i += 2 {
		unit := binary.LittleEndian.Uint16(payload[i:])
		if unit == 0 {
			terminated = true
			break
		}
		units = append(units, unit)
	}
	if !terminated {
		return "", errTextUnterminated
	}
	runes := utf16.Decode(units)
	if !slices.Equal(utf16.Encode(runes), units) {
		return "", errTextNotUTF16
	}
	return strings.ReplaceAll(string(runes), "\r\n", "\n"), nil
}

func nulTerminate(b []byte) []byte {
	out := make([]byte, len(b)+1)
	copy(out, b)
	return out
}

func cstring(b []byte) ([]byte, error) {
	i := bytes.IndexByte(b, 0)
	if i < 0 {
		return nil, errTextUnterminated
	}
	return b[:i], nil
}
