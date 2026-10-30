// Package proto holds the pure, OS-independent parts of the EdgeSyncSpike service/helper channel: length-prefixed
// framing, the fixed command whitelist, the authorisation decisions the service makes before it trusts a helper
// message, and desktop-name parsing. Nothing here can change configuration, run programs or read files (D-47).
package proto

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	PipePath = `\\.\pipe\EdgeSyncSpike`

	// PipeSDDL: owner SYSTEM, protected DACL with exactly one ACE granting SYSTEM full access. The helper runs with a
	// duplicated winlogon token, which is LocalSystem, so no other principal needs access.
	PipeSDDL = "O:SYD:P(A;;GA;;;SY)"

	LocalSystemSID = "S-1-5-18"

	Version        byte = 1
	MaxPayload          = 256
	MaxDesktopName      = 64
	MaxMessage          = 128
)

type Command byte

const (
	CmdHello         Command = 1
	CmdReportDesktop Command = 2
	CmdRequestSAS    Command = 3
)

type Status byte

const (
	StatusOK         Status = 0
	StatusDenied     Status = 1
	StatusRefused    Status = 2
	StatusLimited    Status = 3
	StatusFailed     Status = 4
	StatusBadRequest Status = 5
)

var (
	ErrFrameSize      = errors.New("proto: frame size out of range")
	ErrMalformed      = errors.New("proto: malformed message")
	ErrUnknownCommand = errors.New("proto: command not on the whitelist")
	ErrBadVersion     = errors.New("proto: unsupported protocol version")
	ErrBadName        = errors.New("proto: invalid desktop name")
	ErrBadMessage     = errors.New("proto: invalid response message")

	ErrNoHelper     = errors.New("proto: no helper is registered with the service")
	ErrNotSystem    = errors.New("proto: client token user is not LocalSystem")
	ErrWrongProcess = errors.New("proto: client process is not the helper the service launched")
	ErrWrongSession = errors.New("proto: client token session differs from the helper session")
	ErrWrongImage   = errors.New("proto: client image path differs from the helper executable")
)

func (s Status) String() string {
	switch s {
	case StatusOK:
		return "ok"
	case StatusDenied:
		return "denied"
	case StatusRefused:
		return "refused"
	case StatusLimited:
		return "limited"
	case StatusFailed:
		return "failed"
	case StatusBadRequest:
		return "bad-request"
	default:
		return fmt.Sprintf("status-%d", byte(s))
	}
}

func (c Command) String() string {
	switch c {
	case CmdHello:
		return "Hello"
	case CmdReportDesktop:
		return "ReportDesktop"
	case CmdRequestSAS:
		return "RequestSAS"
	default:
		return fmt.Sprintf("command-%d", byte(c))
	}
}

func WriteFrame(w io.Writer, payload []byte) error {
	if len(payload) == 0 || len(payload) > MaxPayload {
		return fmt.Errorf("%w: %d bytes", ErrFrameSize, len(payload))
	}
	buf := make([]byte, 2+len(payload))
	binary.BigEndian.PutUint16(buf, uint16(len(payload)))
	copy(buf[2:], payload)
	n, err := w.Write(buf)
	if err != nil {
		return fmt.Errorf("proto: write frame: %w", err)
	}
	if n != len(buf) {
		return fmt.Errorf("proto: write frame: %w", io.ErrShortWrite)
	}
	return nil
}

// ReadFrame returns io.EOF only when the stream ends exactly on a frame boundary.
func ReadFrame(r io.Reader) ([]byte, error) {
	var header [2]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, io.EOF
		}
		return nil, fmt.Errorf("proto: read frame header: %w", err)
	}
	size := int(binary.BigEndian.Uint16(header[:]))
	if size == 0 || size > MaxPayload {
		return nil, fmt.Errorf("%w: %d bytes", ErrFrameSize, size)
	}
	payload := make([]byte, size)
	if _, err := io.ReadFull(r, payload); err != nil {
		if errors.Is(err, io.EOF) {
			err = io.ErrUnexpectedEOF
		}
		return nil, fmt.Errorf("proto: read frame body: %w", err)
	}
	return payload, nil
}

type Request struct {
	Cmd     Command
	Version byte
	Desktop string
}

func (r Request) Marshal() ([]byte, error) {
	switch r.Cmd {
	case CmdHello:
		return []byte{byte(CmdHello), r.Version}, nil
	case CmdReportDesktop:
		if err := ValidateDesktopName(r.Desktop); err != nil {
			return nil, err
		}
		return append([]byte{byte(CmdReportDesktop)}, r.Desktop...), nil
	case CmdRequestSAS:
		return []byte{byte(CmdRequestSAS)}, nil
	default:
		return nil, fmt.Errorf("%w: %d", ErrUnknownCommand, byte(r.Cmd))
	}
}

func ParseRequest(payload []byte) (Request, error) {
	if len(payload) == 0 {
		return Request{}, fmt.Errorf("%w: empty payload", ErrMalformed)
	}
	body := payload[1:]
	switch cmd := Command(payload[0]); cmd {
	case CmdHello:
		if len(body) != 1 {
			return Request{}, fmt.Errorf("%w: Hello body is %d bytes", ErrMalformed, len(body))
		}
		if body[0] != Version {
			return Request{}, fmt.Errorf("%w: %d", ErrBadVersion, body[0])
		}
		return Request{Cmd: cmd, Version: body[0]}, nil
	case CmdReportDesktop:
		name := string(body)
		if err := ValidateDesktopName(name); err != nil {
			return Request{}, err
		}
		return Request{Cmd: cmd, Desktop: name}, nil
	case CmdRequestSAS:
		if len(body) != 0 {
			return Request{}, fmt.Errorf("%w: RequestSAS carries no body", ErrMalformed)
		}
		return Request{Cmd: cmd}, nil
	default:
		return Request{}, fmt.Errorf("%w: %d", ErrUnknownCommand, payload[0])
	}
}

type Response struct {
	Status  Status
	Message string
}

func (r Response) Marshal() ([]byte, error) {
	if len(r.Message) > MaxMessage || !printableASCII(r.Message) {
		return nil, ErrBadMessage
	}
	return append([]byte{byte(r.Status)}, r.Message...), nil
}

func ParseResponse(payload []byte) (Response, error) {
	if len(payload) == 0 {
		return Response{}, fmt.Errorf("%w: empty payload", ErrMalformed)
	}
	message := string(payload[1:])
	if len(message) > MaxMessage || !printableASCII(message) {
		return Response{}, ErrBadMessage
	}
	return Response{Status: Status(payload[0]), Message: message}, nil
}

func printableASCII(s string) bool {
	for i := range len(s) {
		if s[i] < 0x20 || s[i] > 0x7e {
			return false
		}
	}
	return true
}

type Peer struct {
	PID       uint32
	UserSID   string
	SessionID uint32
	ImagePath string
}

type Expected struct {
	PID       uint32
	SessionID uint32
	ImagePath string
}

// AuthorizePeer is evaluated before every message: the pipe client must be the very process the service launched, in
// the session it was launched into, running as LocalSystem from the expected image.
func AuthorizePeer(p Peer, e Expected) error {
	switch {
	case e.PID == 0:
		return ErrNoHelper
	case p.UserSID != LocalSystemSID:
		return fmt.Errorf("%w: %q", ErrNotSystem, p.UserSID)
	case p.PID != e.PID:
		return fmt.Errorf("%w: pid %d, expected %d", ErrWrongProcess, p.PID, e.PID)
	case p.SessionID != e.SessionID:
		return fmt.Errorf("%w: session %d, expected %d", ErrWrongSession, p.SessionID, e.SessionID)
	case !samePath(p.ImagePath, e.ImagePath):
		return fmt.Errorf("%w: %q, expected %q", ErrWrongImage, p.ImagePath, e.ImagePath)
	}
	return nil
}

func samePath(a, b string) bool {
	return strings.EqualFold(strings.ReplaceAll(a, "/", `\`), strings.ReplaceAll(b, "/", `\`))
}

type Policy struct {
	AllowSAS bool
	MaxSAS   int
}

// Gate decides what an already authenticated helper may ask for. A StatusOK answer to CmdRequestSAS is the only
// authorisation the service accepts before it calls SendSAS.
type Gate struct {
	policy     Policy
	hello      bool
	sasGranted int
}

func NewGate(policy Policy) *Gate {
	return &Gate{policy: policy}
}

// Disconnect forgets the Hello of the closed connection but keeps the SAS budget, which is per service run.
func (g *Gate) Disconnect() {
	g.hello = false
}

func (g *Gate) Decide(cmd Command) Status {
	switch cmd {
	case CmdHello:
		g.hello = true
		return StatusOK
	case CmdReportDesktop:
		if !g.hello {
			return StatusDenied
		}
		return StatusOK
	case CmdRequestSAS:
		switch {
		case !g.hello:
			return StatusDenied
		case !g.policy.AllowSAS:
			return StatusRefused
		case g.sasGranted >= g.policy.MaxSAS:
			return StatusLimited
		}
		g.sasGranted++
		return StatusOK
	default:
		return StatusBadRequest
	}
}

type DesktopKind int

const (
	DesktopOther DesktopKind = iota
	DesktopDefault
	DesktopWinlogon
	DesktopScreenSaver
)

func (k DesktopKind) String() string {
	switch k {
	case DesktopDefault:
		return "default"
	case DesktopWinlogon:
		return "winlogon"
	case DesktopScreenSaver:
		return "screen-saver"
	default:
		return "other"
	}
}

func ClassifyDesktop(name string) DesktopKind {
	switch {
	case strings.EqualFold(name, "Default"):
		return DesktopDefault
	case strings.EqualFold(name, "Winlogon"):
		return DesktopWinlogon
	case strings.EqualFold(name, "Screen-saver"):
		return DesktopScreenSaver
	default:
		return DesktopOther
	}
}

func ValidateDesktopName(name string) error {
	if name == "" || len(name) > MaxDesktopName || !utf8.ValidString(name) {
		return fmt.Errorf("%w: length %d", ErrBadName, len(name))
	}
	for _, r := range name {
		if unicode.IsControl(r) || r == '\\' {
			return fmt.Errorf("%w: contains %U", ErrBadName, r)
		}
	}
	return nil
}

// SplitDesktopPath splits a "station\desktop" string as used for STARTUPINFO.lpDesktop.
func SplitDesktopPath(path string) (station, desktop string, err error) {
	station, desktop, found := strings.Cut(path, `\`)
	if !found || station == "" {
		return "", "", fmt.Errorf("%w: %q has no station", ErrBadName, path)
	}
	if err := ValidateDesktopName(desktop); err != nil {
		return "", "", err
	}
	return station, desktop, nil
}

// LaunchDesktop maps the -desktop flag choice to the lpDesktop string used when the helper is created.
func LaunchDesktop(choice string) (string, error) {
	switch strings.ToLower(choice) {
	case "winlogon":
		return `winsta0\winlogon`, nil
	case "default":
		return `winsta0\default`, nil
	default:
		return "", fmt.Errorf("%w: launch desktop must be winlogon or default, got %q", ErrBadName, choice)
	}
}
