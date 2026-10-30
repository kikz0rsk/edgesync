package proto

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestFrame_RoundTrip(t *testing.T) {
	var buf bytes.Buffer
	for _, payload := range [][]byte{{1}, {2, 'a', 'b'}, bytes.Repeat([]byte{7}, MaxPayload)} {
		if err := WriteFrame(&buf, payload); err != nil {
			t.Fatalf("WriteFrame: %v", err)
		}
	}
	for _, want := range []int{1, 3, MaxPayload} {
		got, err := ReadFrame(&buf)
		if err != nil {
			t.Fatalf("ReadFrame: %v", err)
		}
		if len(got) != want {
			t.Fatalf("frame length %d, want %d", len(got), want)
		}
	}
	if _, err := ReadFrame(&buf); !errors.Is(err, io.EOF) {
		t.Fatalf("clean end of stream: got %v, want io.EOF", err)
	}
}

func TestFrame_RejectsBadSizes(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteFrame(&buf, nil); !errors.Is(err, ErrFrameSize) {
		t.Fatalf("empty payload: got %v", err)
	}
	if err := WriteFrame(&buf, make([]byte, MaxPayload+1)); !errors.Is(err, ErrFrameSize) {
		t.Fatalf("oversized payload: got %v", err)
	}
	if _, err := ReadFrame(bytes.NewReader([]byte{0, 0})); !errors.Is(err, ErrFrameSize) {
		t.Fatalf("zero length header: got %v", err)
	}
	if _, err := ReadFrame(bytes.NewReader([]byte{0xff, 0xff})); !errors.Is(err, ErrFrameSize) {
		t.Fatalf("huge length header: got %v", err)
	}
}

func TestFrame_TruncatedStreams(t *testing.T) {
	if _, err := ReadFrame(bytes.NewReader([]byte{0})); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("half header: got %v", err)
	}
	if _, err := ReadFrame(bytes.NewReader([]byte{0, 5, 1, 2})); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("short body: got %v", err)
	}
}

func TestRequest_RoundTrip(t *testing.T) {
	for _, req := range []Request{
		{Cmd: CmdHello, Version: Version},
		{Cmd: CmdReportDesktop, Desktop: "Winlogon"},
		{Cmd: CmdRequestSAS},
	} {
		payload, err := req.Marshal()
		if err != nil {
			t.Fatalf("%v Marshal: %v", req.Cmd, err)
		}
		got, err := ParseRequest(payload)
		if err != nil {
			t.Fatalf("%v ParseRequest: %v", req.Cmd, err)
		}
		if got != req {
			t.Fatalf("round trip: got %+v, want %+v", got, req)
		}
	}
}

func TestParseRequest_RejectsMalformed(t *testing.T) {
	cases := map[string]struct {
		payload []byte
		want    error
	}{
		"empty":               {nil, ErrMalformed},
		"unknown command":     {[]byte{99}, ErrUnknownCommand},
		"command zero":        {[]byte{0}, ErrUnknownCommand},
		"hello without body":  {[]byte{byte(CmdHello)}, ErrMalformed},
		"hello extra body":    {[]byte{byte(CmdHello), 1, 1}, ErrMalformed},
		"hello wrong version": {[]byte{byte(CmdHello), 9}, ErrBadVersion},
		"sas with body":       {[]byte{byte(CmdRequestSAS), 1}, ErrMalformed},
		"desktop empty":       {[]byte{byte(CmdReportDesktop)}, ErrBadName},
		"desktop control":     {append([]byte{byte(CmdReportDesktop)}, "a\x00b"...), ErrBadName},
		"desktop backslash":   {append([]byte{byte(CmdReportDesktop)}, `winsta0\x`...), ErrBadName},
		"desktop too long":    {append([]byte{byte(CmdReportDesktop)}, strings.Repeat("a", MaxDesktopName+1)...), ErrBadName},
		"desktop bad utf8":    {append([]byte{byte(CmdReportDesktop)}, 0xff, 0xfe), ErrBadName},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseRequest(tc.payload); !errors.Is(err, tc.want) {
				t.Fatalf("got %v, want %v", err, tc.want)
			}
		})
	}
}

func TestRequest_MarshalRejectsUnknownCommand(t *testing.T) {
	if _, err := (Request{Cmd: 42}).Marshal(); !errors.Is(err, ErrUnknownCommand) {
		t.Fatalf("got %v", err)
	}
}

func TestResponse_RoundTripAndValidation(t *testing.T) {
	payload, err := Response{Status: StatusRefused, Message: "sas disabled"}.Marshal()
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	got, err := ParseResponse(payload)
	if err != nil {
		t.Fatalf("ParseResponse: %v", err)
	}
	if got.Status != StatusRefused || got.Message != "sas disabled" {
		t.Fatalf("got %+v", got)
	}
	if _, err := (Response{Message: "bad\n"}).Marshal(); !errors.Is(err, ErrBadMessage) {
		t.Fatalf("control character: got %v", err)
	}
	if _, err := (Response{Message: strings.Repeat("a", MaxMessage+1)}).Marshal(); !errors.Is(err, ErrBadMessage) {
		t.Fatalf("too long: got %v", err)
	}
	if _, err := ParseResponse(nil); !errors.Is(err, ErrMalformed) {
		t.Fatalf("empty: got %v", err)
	}
}

func TestAuthorizePeer_Decisions(t *testing.T) {
	want := Expected{PID: 4242, SessionID: 1, ImagePath: `C:\ProgramData\EdgeSyncSpike\bin\spikehelper.exe`}
	good := Peer{PID: 4242, UserSID: LocalSystemSID, SessionID: 1, ImagePath: `c:\programdata\edgesyncspike\bin\SPIKEHELPER.EXE`}

	if err := AuthorizePeer(good, want); err != nil {
		t.Fatalf("matching peer rejected: %v", err)
	}

	cases := map[string]struct {
		mutate func(*Peer, *Expected)
		want   error
	}{
		"no helper registered": {func(_ *Peer, e *Expected) { e.PID = 0 }, ErrNoHelper},
		"ordinary user":        {func(p *Peer, _ *Expected) { p.UserSID = "S-1-5-21-1-2-3-1001" }, ErrNotSystem},
		"administrators":       {func(p *Peer, _ *Expected) { p.UserSID = "S-1-5-32-544" }, ErrNotSystem},
		"other process":        {func(p *Peer, _ *Expected) { p.PID = 99 }, ErrWrongProcess},
		"other session":        {func(p *Peer, _ *Expected) { p.SessionID = 2 }, ErrWrongSession},
		"other image":          {func(p *Peer, _ *Expected) { p.ImagePath = `C:\Windows\System32\cmd.exe` }, ErrWrongImage},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			p, e := good, want
			tc.mutate(&p, &e)
			if err := AuthorizePeer(p, e); !errors.Is(err, tc.want) {
				t.Fatalf("got %v, want %v", err, tc.want)
			}
		})
	}
}

func TestGate_RequiresHelloFirst(t *testing.T) {
	g := NewGate(Policy{AllowSAS: true, MaxSAS: 1})
	if got := g.Decide(CmdReportDesktop); got != StatusDenied {
		t.Fatalf("ReportDesktop before Hello: %v", got)
	}
	if got := g.Decide(CmdRequestSAS); got != StatusDenied {
		t.Fatalf("RequestSAS before Hello: %v", got)
	}
	if got := g.Decide(CmdHello); got != StatusOK {
		t.Fatalf("Hello: %v", got)
	}
	if got := g.Decide(CmdReportDesktop); got != StatusOK {
		t.Fatalf("ReportDesktop after Hello: %v", got)
	}
}

func TestGate_SASPolicyAndLimit(t *testing.T) {
	refusing := NewGate(Policy{AllowSAS: false, MaxSAS: 3})
	refusing.Decide(CmdHello)
	if got := refusing.Decide(CmdRequestSAS); got != StatusRefused {
		t.Fatalf("SAS with policy off: %v", got)
	}

	limited := NewGate(Policy{AllowSAS: true, MaxSAS: 2})
	limited.Decide(CmdHello)
	for i := range 2 {
		if got := limited.Decide(CmdRequestSAS); got != StatusOK {
			t.Fatalf("SAS %d: %v", i, got)
		}
	}
	if got := limited.Decide(CmdRequestSAS); got != StatusLimited {
		t.Fatalf("SAS over limit: %v", got)
	}
}

func TestGate_DisconnectKeepsSASBudget(t *testing.T) {
	g := NewGate(Policy{AllowSAS: true, MaxSAS: 1})
	g.Decide(CmdHello)
	if got := g.Decide(CmdRequestSAS); got != StatusOK {
		t.Fatalf("first SAS: %v", got)
	}
	g.Disconnect()
	if got := g.Decide(CmdRequestSAS); got != StatusDenied {
		t.Fatalf("SAS on a new connection without Hello: %v", got)
	}
	g.Decide(CmdHello)
	if got := g.Decide(CmdRequestSAS); got != StatusLimited {
		t.Fatalf("SAS budget must survive reconnects: %v", got)
	}
}

func TestGate_UnknownCommandIsBadRequest(t *testing.T) {
	g := NewGate(Policy{AllowSAS: true, MaxSAS: 1})
	g.Decide(CmdHello)
	if got := g.Decide(Command(77)); got != StatusBadRequest {
		t.Fatalf("got %v", got)
	}
}

func TestClassifyDesktop(t *testing.T) {
	cases := map[string]DesktopKind{
		"Default":      DesktopDefault,
		"default":      DesktopDefault,
		"Winlogon":     DesktopWinlogon,
		"WINLOGON":     DesktopWinlogon,
		"Screen-saver": DesktopScreenSaver,
		"screen-saver": DesktopScreenSaver,
		"Disconnect":   DesktopOther,
		"":             DesktopOther,
		"Default2":     DesktopOther,
	}
	for name, want := range cases {
		if got := ClassifyDesktop(name); got != want {
			t.Errorf("ClassifyDesktop(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestSplitDesktopPath(t *testing.T) {
	station, desktop, err := SplitDesktopPath(`winsta0\winlogon`)
	if err != nil || station != "winsta0" || desktop != "winlogon" {
		t.Fatalf("got %q %q %v", station, desktop, err)
	}
	for _, bad := range []string{"", "winlogon", `\winlogon`, `winsta0\`, `winsta0\a\b`} {
		if _, _, err := SplitDesktopPath(bad); !errors.Is(err, ErrBadName) {
			t.Errorf("SplitDesktopPath(%q): got %v", bad, err)
		}
	}
}

func TestLaunchDesktop(t *testing.T) {
	for choice, want := range map[string]string{"winlogon": `winsta0\winlogon`, "Default": `winsta0\default`} {
		got, err := LaunchDesktop(choice)
		if err != nil || got != want {
			t.Errorf("LaunchDesktop(%q) = %q, %v", choice, got, err)
		}
	}
	if _, err := LaunchDesktop("secure"); !errors.Is(err, ErrBadName) {
		t.Fatalf("unknown choice: got %v", err)
	}
}
