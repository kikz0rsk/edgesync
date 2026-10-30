package main

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"testing"
	"unicode/utf16"
	"unicode/utf8"
)

func TestCFHTML_BuildOffsetsMatch(t *testing.T) {
	got := buildCFHTML([]byte("Hello"))
	want := "Version:1.0\r\n" +
		"StartHTML:0000000105\r\n" +
		"EndHTML:0000000178\r\n" +
		"StartFragment:0000000139\r\n" +
		"EndFragment:0000000144\r\n" +
		"<html><body>\r\n" +
		"<!--StartFragment-->Hello<!--EndFragment-->\r\n" +
		"</body></html>"
	if string(got) != want {
		t.Fatalf("buildCFHTML bytes mismatch\n got: %q\nwant: %q", got, want)
	}
	if string(got[105:111]) != "<html>" {
		t.Fatalf("StartHTML=105 does not point at <html>: %q", got[105:111])
	}
	if string(got[139:144]) != "Hello" {
		t.Fatalf("StartFragment=139/EndFragment=144 do not bracket the fragment: %q", got[139:144])
	}
	if string(got[171:178]) != "</html>" {
		t.Fatalf("EndHTML=178 does not follow </html>: %q", got[171:178])
	}
}

func TestCFHTML_ParseFragment(t *testing.T) {
	const fragment = "<b>bold</b> text"
	payload := nulTerminate(lfSampleDocument(fragment))
	got, err := fragmentFromHTMLPayload(payload)
	if err != nil {
		t.Fatalf("fragmentFromHTMLPayload: %v", err)
	}
	if string(got) != fragment {
		t.Fatalf("fragment mismatch: got %q want %q", got, fragment)
	}

	cases := []struct {
		name    string
		payload []byte
		want    error
	}{
		{
			name:    "end offset beyond buffer",
			payload: []byte("Version:1.0\r\nStartFragment:0000000010\r\nEndFragment:0000000999\r\nx\x00"),
			want:    errCFHTMLOffsets,
		},
		{
			name:    "EndFragment missing",
			payload: []byte("Version:1.0\r\nStartFragment:0000000010\r\n\x00"),
			want:    errCFHTMLHeader,
		},
		{
			name:    "Version missing",
			payload: []byte("StartFragment:0000000010\r\nEndFragment:0000000011\r\nx\x00"),
			want:    errCFHTMLHeader,
		},
		{
			name:    "non-numeric offset",
			payload: []byte("Version:1.0\r\nStartFragment:abc\r\nEndFragment:0000000011\r\nx\x00"),
			want:    errCFHTMLHeader,
		},
		{
			name:    "missing NUL terminator",
			payload: []byte("Version:1.0\r\nStartFragment:0000000010\r\nEndFragment:0000000011\r\nx"),
			want:    errTextUnterminated,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := fragmentFromHTMLPayload(tc.payload); !errors.Is(err, tc.want) {
				t.Fatalf("error = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestCFHTML_RoundTripUnicode(t *testing.T) {
	const fragment = "<p>héllo 世界 \U0001F600</p>"
	if len(fragment) == utf8.RuneCountInString(fragment) {
		t.Fatal("fixture must contain multi-byte characters so byte offsets differ from rune offsets")
	}
	got, err := fragmentFromHTMLPayload(htmlPayload([]byte(fragment)))
	if err != nil {
		t.Fatalf("fragmentFromHTMLPayload: %v", err)
	}
	if string(got) != fragment {
		t.Fatalf("round trip mismatch: got %q want %q", got, fragment)
	}
}

func TestCFUnicodeText_RoundTrip(t *testing.T) {
	const text = "first\nsecond\r\nthird é 世 \U0001F600"
	payload, err := textToCFUnicodeText(text)
	if err != nil {
		t.Fatalf("textToCFUnicodeText: %v", err)
	}
	wantUnits := utf16.Encode([]rune("first\r\nsecond\r\nthird é 世 \U0001F600"))
	wantUnits = append(wantUnits, 0)
	want := make([]byte, 0, 2*len(wantUnits))
	for _, unit := range wantUnits {
		want = binary.LittleEndian.AppendUint16(want, unit)
	}
	if !bytes.Equal(payload, want) {
		t.Fatalf("payload mismatch\n got: % X\nwant: % X", payload, want)
	}
	back, err := cfUnicodeTextToUTF8(payload)
	if err != nil {
		t.Fatalf("cfUnicodeTextToUTF8: %v", err)
	}
	if back != "first\nsecond\nthird é 世 \U0001F600" {
		t.Fatalf("round trip mismatch: got %q", back)
	}
}

func TestCFUnicodeText_RejectsMalformedPayloads(t *testing.T) {
	if _, err := textToCFUnicodeText("bad \xff utf8"); !errors.Is(err, errTextNotUTF8) {
		t.Fatalf("invalid UTF-8 error = %v, want %v", err, errTextNotUTF8)
	}
	cases := []struct {
		name    string
		payload []byte
		want    error
	}{
		{name: "odd length", payload: []byte{0x61, 0x00, 0x62}, want: errTextOddLength},
		{name: "no terminator", payload: []byte{0x61, 0x00}, want: errTextUnterminated},
		{name: "unpaired surrogate", payload: []byte{0x00, 0xD8, 0x00, 0x00}, want: errTextNotUTF16},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := cfUnicodeTextToUTF8(tc.payload); !errors.Is(err, tc.want) {
				t.Fatalf("error = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestRTFPayload_Passthrough(t *testing.T) {
	rtf := []byte(`{\rtf1\ansi\deff0 {\b EdgeSync} delayed.\par}`)
	payload := rtfPayload(rtf)
	if len(payload) != len(rtf)+1 {
		t.Fatalf("payload length = %d, want %d", len(payload), len(rtf)+1)
	}
	if !bytes.Equal(payload[:len(rtf)], rtf) {
		t.Fatalf("RTF bytes were altered: %q", payload[:len(rtf)])
	}
	if payload[len(rtf)] != 0 {
		t.Fatalf("payload is not NUL terminated: %#x", payload[len(rtf)])
	}
	back, err := rtfFromPayload(payload)
	if err != nil {
		t.Fatalf("rtfFromPayload: %v", err)
	}
	if !bytes.Equal(back, rtf) {
		t.Fatalf("round trip mismatch: got %q", back)
	}
	if _, err := rtfFromPayload(rtf); !errors.Is(err, errTextUnterminated) {
		t.Fatalf("unterminated RTF error = %v, want %v", err, errTextUnterminated)
	}
}

// lfSampleDocument builds a CF_HTML document with LF line endings, zero-padded offsets, StartSelection and
// EndSelection, and a body that uses LF separators, which is what other writers produce.
func lfSampleDocument(fragment string) []byte {
	const bodyPrefix = "<html>\n<body>\n<!--StartFragment-->"
	const bodySuffix = "<!--EndFragment-->\n</body>\n</html>"
	header := func(startHTML, endHTML, startFragment, endFragment int) string {
		return fmt.Sprintf("Version:0.9\nStartHTML:%010d\nEndHTML:%010d\nStartFragment:%010d\nEndFragment:%010d\n"+
			"StartSelection:%010d\nEndSelection:%010d\n", startHTML, endHTML, startFragment, endFragment, startFragment, endFragment)
	}
	headerLen := len(header(0, 0, 0, 0))
	startFragment := headerLen + len(bodyPrefix)
	endFragment := startFragment + len(fragment)
	endHTML := headerLen + len(bodyPrefix) + len(fragment) + len(bodySuffix)
	return []byte(header(headerLen, endHTML, startFragment, endFragment) + bodyPrefix + fragment + bodySuffix)
}
