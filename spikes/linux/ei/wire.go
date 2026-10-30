// Package ei is a pure-Go codec and client for the libei (EI) wire protocol, written from the protocol
// specification (libei 1.6.0, proto/protocol.xml and doc/protocol/doc). See README.md for the source citation.
package ei

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"os"
	"strings"
)

const (
	HeaderSize   = 16
	MaxFrameSize = 1 << 20

	HandshakeObjectID   uint64 = 0
	FirstClientObjectID uint64 = 1
	FirstServerObjectID uint64 = 0xff00000000000000
)

var (
	ErrShortFrame         = errors.New("ei: short frame")
	ErrOversizedFrame     = errors.New("ei: oversized frame")
	ErrInvalidFrameLength = errors.New("ei: invalid frame length")
	ErrTruncatedArgument  = errors.New("ei: truncated argument")
	ErrTrailingBytes      = errors.New("ei: trailing bytes after last argument")
	ErrInvalidString      = errors.New("ei: invalid string")
	ErrNoFD               = errors.New("ei: no file descriptor available")
	ErrFDUnsupported      = errors.New("ei: transport cannot carry file descriptors")
)

type Header struct {
	ObjectID uint64
	Length   uint32
	Opcode   uint32
}

func (h Header) Append(dst []byte) []byte {
	dst = binary.LittleEndian.AppendUint64(dst, h.ObjectID)
	dst = binary.LittleEndian.AppendUint32(dst, h.Length)
	return binary.LittleEndian.AppendUint32(dst, h.Opcode)
}

func DecodeHeader(b []byte) (Header, error) {
	if len(b) < HeaderSize {
		return Header{}, fmt.Errorf("%w: %d bytes, header needs %d", ErrShortFrame, len(b), HeaderSize)
	}
	h := Header{
		ObjectID: binary.LittleEndian.Uint64(b[0:8]),
		Length:   binary.LittleEndian.Uint32(b[8:12]),
		Opcode:   binary.LittleEndian.Uint32(b[12:16]),
	}
	if h.Length > MaxFrameSize {
		return Header{}, fmt.Errorf("%w: declared length %d, limit %d", ErrOversizedFrame, h.Length, MaxFrameSize)
	}
	if h.Length < HeaderSize || h.Length%4 != 0 {
		return Header{}, fmt.Errorf("%w: declared length %d", ErrInvalidFrameLength, h.Length)
	}
	return h, nil
}

type Frame struct {
	ObjectID uint64
	Opcode   uint32
	Payload  []byte
	FDs      []*os.File
}

func (f Frame) AppendTo(dst []byte) ([]byte, error) {
	if len(f.Payload)%4 != 0 {
		return dst, fmt.Errorf("%w: payload of %d bytes is not a multiple of 4", ErrInvalidFrameLength, len(f.Payload))
	}
	total := HeaderSize + len(f.Payload)
	if total > MaxFrameSize {
		return dst, fmt.Errorf("%w: %d bytes, limit %d", ErrOversizedFrame, total, MaxFrameSize)
	}
	dst = Header{ObjectID: f.ObjectID, Length: uint32(total), Opcode: f.Opcode}.Append(dst)
	return append(dst, f.Payload...), nil
}

func DecodeFrame(b []byte) (Frame, int, error) {
	h, err := DecodeHeader(b)
	if err != nil {
		return Frame{}, 0, err
	}
	if len(b) < int(h.Length) {
		return Frame{}, 0, fmt.Errorf("%w: %d bytes available, frame declares %d", ErrShortFrame, len(b), h.Length)
	}
	payload := make([]byte, int(h.Length)-HeaderSize)
	copy(payload, b[HeaderSize:h.Length])
	return Frame{ObjectID: h.ObjectID, Opcode: h.Opcode, Payload: payload}, int(h.Length), nil
}

func ReadFrame(r io.Reader) (Frame, error) {
	var head [HeaderSize]byte
	if _, err := io.ReadFull(r, head[:]); err != nil {
		return Frame{}, err
	}
	h, err := DecodeHeader(head[:])
	if err != nil {
		return Frame{}, err
	}
	payload := make([]byte, h.Length-HeaderSize)
	if _, err := io.ReadFull(r, payload); err != nil {
		if errors.Is(err, io.EOF) {
			return Frame{}, io.ErrUnexpectedEOF
		}
		return Frame{}, err
	}
	return Frame{ObjectID: h.ObjectID, Opcode: h.Opcode, Payload: payload}, nil
}

type Encoder struct {
	buf []byte
	fds []*os.File
}

func (e *Encoder) Uint32(v uint32) {
	e.buf = binary.LittleEndian.AppendUint32(e.buf, v)
}

func (e *Encoder) Int32(v int32) {
	e.Uint32(uint32(v))
}

func (e *Encoder) Uint64(v uint64) {
	e.buf = binary.LittleEndian.AppendUint64(e.buf, v)
}

func (e *Encoder) Int64(v int64) {
	e.Uint64(uint64(v))
}

func (e *Encoder) Float32(v float32) {
	e.Uint32(math.Float32bits(v))
}

func (e *Encoder) NewID(id uint64) {
	e.Uint64(id)
}

func (e *Encoder) ObjectID(id uint64) {
	e.Uint64(id)
}

func (e *Encoder) FD(f *os.File) {
	e.fds = append(e.fds, f)
}

func (e *Encoder) String(s string) error {
	if strings.IndexByte(s, 0) >= 0 {
		return fmt.Errorf("%w: embedded NUL byte", ErrInvalidString)
	}
	e.Uint32(uint32(len(s) + 1))
	e.buf = append(e.buf, s...)
	e.buf = append(e.buf, 0)
	for len(e.buf)%4 != 0 {
		e.buf = append(e.buf, 0)
	}
	return nil
}

func (e *Encoder) NullString() {
	e.Uint32(0)
}

func (e *Encoder) Frame(objectID uint64, opcode uint32) Frame {
	return Frame{ObjectID: objectID, Opcode: opcode, Payload: e.buf, FDs: e.fds}
}

type FDSource interface {
	PopFD() (*os.File, error)
}

type Decoder struct {
	buf []byte
	fds FDSource
	err error
}

func NewDecoder(payload []byte, fds FDSource) *Decoder {
	return &Decoder{buf: payload, fds: fds}
}

func (d *Decoder) take(n int) []byte {
	if d.err != nil {
		return nil
	}
	if len(d.buf) < n {
		d.err = fmt.Errorf("%w: need %d bytes, %d remaining", ErrTruncatedArgument, n, len(d.buf))
		return nil
	}
	out := d.buf[:n]
	d.buf = d.buf[n:]
	return out
}

func (d *Decoder) Uint32() uint32 {
	b := d.take(4)
	if b == nil {
		return 0
	}
	return binary.LittleEndian.Uint32(b)
}

func (d *Decoder) Int32() int32 {
	return int32(d.Uint32())
}

func (d *Decoder) Uint64() uint64 {
	b := d.take(8)
	if b == nil {
		return 0
	}
	return binary.LittleEndian.Uint64(b)
}

func (d *Decoder) Int64() int64 {
	return int64(d.Uint64())
}

func (d *Decoder) Float32() float32 {
	return math.Float32frombits(d.Uint32())
}

func (d *Decoder) NewID() uint64 {
	return d.Uint64()
}

func (d *Decoder) ObjectID() uint64 {
	return d.Uint64()
}

func (d *Decoder) FD() *os.File {
	if d.err != nil {
		return nil
	}
	if d.fds == nil {
		d.err = ErrNoFD
		return nil
	}
	f, err := d.fds.PopFD()
	if err != nil {
		d.err = err
		return nil
	}
	return f
}

func (d *Decoder) NullableString() (string, bool) {
	lenField := d.Uint32()
	if d.err != nil {
		return "", false
	}
	if lenField == 0 {
		return "", false
	}
	padded := (uint64(lenField) + 3) / 4 * 4
	if padded > uint64(len(d.buf)) {
		d.err = fmt.Errorf("%w: string of length %d, %d bytes remaining", ErrTruncatedArgument, lenField, len(d.buf))
		return "", false
	}
	raw := d.take(int(padded))
	if raw[lenField-1] != 0 {
		d.err = fmt.Errorf("%w: not zero-terminated", ErrInvalidString)
		return "", false
	}
	body := raw[:lenField-1]
	if bytes.IndexByte(body, 0) >= 0 {
		d.err = fmt.Errorf("%w: embedded NUL byte", ErrInvalidString)
		return "", false
	}
	return string(body), true
}

func (d *Decoder) String() string {
	s, ok := d.NullableString()
	if d.err == nil && !ok {
		d.err = fmt.Errorf("%w: NULL string where a string is required", ErrInvalidString)
	}
	return s
}

func (d *Decoder) Err() error {
	return d.err
}

func (d *Decoder) Finish() error {
	if d.err != nil {
		return d.err
	}
	if len(d.buf) != 0 {
		return fmt.Errorf("%w: %d bytes", ErrTrailingBytes, len(d.buf))
	}
	return nil
}

type Transport interface {
	io.ReadWriteCloser
	FDSource
	WriteFDs(p []byte, fds []*os.File) error
}

type streamTransport struct {
	net.Conn
}

func NewStreamTransport(conn net.Conn) Transport {
	return streamTransport{Conn: conn}
}

func (streamTransport) PopFD() (*os.File, error) {
	return nil, ErrNoFD
}

func (t streamTransport) WriteFDs(p []byte, fds []*os.File) error {
	if len(fds) > 0 {
		return ErrFDUnsupported
	}
	_, err := t.Write(p)
	return err
}
