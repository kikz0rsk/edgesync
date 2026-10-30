// Package quicbench measures one-way latency of 1000 Hz small frames carried as QUIC datagrams, on a QUIC stream
// sharing a connection with a saturating bulk stream, and on TCP+TLS 1.3 connections, for the transport decision in
// D-35. Traffic stays on loopback unicast unless cmd/quicbench is pointed at a remote address.
package quicbench

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"math/big"
	"net"
	"slices"
	"time"

	"github.com/quic-go/quic-go"
)

const (
	ALPN            = "edgesync/1"
	FrameSize       = 18
	RateHz          = 1000
	UrgencyInput    = int8(0)
	UrgencyBulk     = int8(7)
	BulkBufferSize  = 1 << 20
	keepAlivePeriod = time.Second
	maxIdleTimeout  = 5 * time.Second
)

var (
	ErrFrameSize = errors.New("quicbench: frame has wrong size")
	ErrNoSamples = errors.New("quicbench: no latency samples")
)

// Frame is the 18-byte MotionAbs-like payload: sequence, send time on the shared Clock, position and flags.
type Frame struct {
	Seq    uint32
	SentNS int64
	X, Y   int16
	Flags  uint16
}

func (f Frame) Encode() [FrameSize]byte {
	var b [FrameSize]byte
	binary.BigEndian.PutUint32(b[0:4], f.Seq)
	binary.BigEndian.PutUint64(b[4:12], uint64(f.SentNS))
	binary.BigEndian.PutUint16(b[12:14], uint16(f.X))
	binary.BigEndian.PutUint16(b[14:16], uint16(f.Y))
	binary.BigEndian.PutUint16(b[16:18], f.Flags)
	return b
}

func DecodeFrame(b []byte) (Frame, error) {
	if len(b) != FrameSize {
		return Frame{}, fmt.Errorf("%w: got %d bytes", ErrFrameSize, len(b))
	}
	return Frame{
		Seq:    binary.BigEndian.Uint32(b[0:4]),
		SentNS: int64(binary.BigEndian.Uint64(b[4:12])),
		X:      int16(binary.BigEndian.Uint16(b[12:14])),
		Y:      int16(binary.BigEndian.Uint16(b[14:16])),
		Flags:  binary.BigEndian.Uint16(b[16:18]),
	}, nil
}

// Clock is the monotonic clock shared by both ends of a single-process measurement. Now returns nanoseconds since
// NewClock.
type Clock struct {
	origin int64
	hz     int64
}

func NewClock() (Clock, error) {
	hz, err := counterHz()
	if err != nil {
		return Clock{}, err
	}
	return Clock{origin: rawCounter(), hz: hz}, nil
}

func (c Clock) Now() int64 { return ticksToNanos(rawCounter()-c.origin, c.hz) }

func (c Clock) Since(sentNS int64) time.Duration { return time.Duration(c.Now() - sentNS) }

func ticksToNanos(ticks, hz int64) int64 {
	whole, rem := ticks/hz, ticks%hz
	return whole*int64(time.Second) + rem*int64(time.Second)/hz
}

func newIdentity() (tls.Certificate, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("generate P-256 key: %w", err)
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 62))
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("generate serial: %w", err)
	}
	now := time.Now()
	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: "edgesync-quicbench"},
		NotBefore:    now.Add(-time.Hour),
		NotAfter:     now.Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		DNSNames:     []string{"localhost"},
		IPAddresses:  []net.IP{net.IPv4(127, 0, 0, 1)},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("create certificate: %w", err)
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}, nil
}

// ServerTLSConfig returns a TLS 1.3 server configuration with a fresh in-memory self-signed ECDSA P-256 certificate.
func ServerTLSConfig() (*tls.Config, error) {
	cert, err := newIdentity()
	if err != nil {
		return nil, err
	}
	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		NextProtos:   []string{ALPN},
		MinVersion:   tls.VersionTLS13,
		MaxVersion:   tls.VersionTLS13,
	}, nil
}

// ClientTLSConfig returns a TLS 1.3 client configuration. Chain verification is off because each bench run uses its
// own self-signed certificate and there is no trust store; this package does not authenticate peers.
func ClientTLSConfig() *tls.Config {
	return &tls.Config{
		NextProtos:         []string{ALPN},
		MinVersion:         tls.VersionTLS13,
		MaxVersion:         tls.VersionTLS13,
		InsecureSkipVerify: true,
	}
}

func quicConfig() *quic.Config {
	return &quic.Config{
		EnableDatagrams: true,
		KeepAlivePeriod: keepAlivePeriod,
		MaxIdleTimeout:  maxIdleTimeout,
	}
}

func ListenQUIC(addr string) (*quic.Listener, error) {
	tlsConf, err := ServerTLSConfig()
	if err != nil {
		return nil, err
	}
	ln, err := quic.ListenAddr(addr, tlsConf, quicConfig())
	if err != nil {
		return nil, fmt.Errorf("listen quic %s: %w", addr, err)
	}
	return ln, nil
}

func DialQUIC(ctx context.Context, addr string) (*quic.Conn, error) {
	conn, err := quic.DialAddr(ctx, addr, ClientTLSConfig(), quicConfig())
	if err != nil {
		return nil, fmt.Errorf("dial quic %s: %w", addr, err)
	}
	return conn, nil
}

// Pace calls send once per tick at RateHz until dur elapses and returns how many frames were sent. Each frame's
// SentNS is read from clock immediately before send, so pacing jitter is not counted as latency.
func Pace(ctx context.Context, clock Clock, dur time.Duration, send func(Frame) error) (int, error) {
	ticker := time.NewTicker(time.Second / RateHz)
	defer ticker.Stop()
	deadline := time.NewTimer(dur)
	defer deadline.Stop()
	var seq uint32
	for {
		select {
		case <-ctx.Done():
			return int(seq), ctx.Err()
		case <-deadline.C:
			return int(seq), nil
		case <-ticker.C:
			if err := send(Frame{Seq: seq, SentNS: clock.Now()}); err != nil {
				return int(seq), fmt.Errorf("send frame %d: %w", seq, err)
			}
			seq++
		}
	}
}

func SendDatagrams(ctx context.Context, conn *quic.Conn, clock Clock, dur time.Duration) (int, error) {
	return Pace(ctx, clock, dur, func(f Frame) error {
		b := f.Encode()
		return conn.SendDatagram(b[:])
	})
}

// ReceiveDatagrams returns one-way latencies until ctx is canceled. Datagrams are unreliable, so the sample count
// is compared against the sent count by the caller.
func ReceiveDatagrams(ctx context.Context, conn *quic.Conn, clock Clock) ([]time.Duration, error) {
	var samples []time.Duration
	for {
		payload, err := conn.ReceiveDatagram(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return samples, nil
			}
			return samples, fmt.Errorf("receive datagram: %w", err)
		}
		f, err := DecodeFrame(payload)
		if err != nil {
			return samples, err
		}
		samples = append(samples, clock.Since(f.SentNS))
	}
}

// EchoDatagrams returns every received datagram to the sender until ctx is canceled or the connection closes.
func EchoDatagrams(ctx context.Context, conn *quic.Conn) error {
	for {
		payload, err := conn.ReceiveDatagram(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("receive datagram: %w", err)
		}
		if err := conn.SendDatagram(payload); err != nil {
			return fmt.Errorf("echo datagram: %w", err)
		}
	}
}

// SendFrames paces frames onto w. The caller ends the stream after it returns.
func SendFrames(ctx context.Context, w io.Writer, clock Clock, dur time.Duration) (int, error) {
	return Pace(ctx, clock, dur, func(f Frame) error {
		b := f.Encode()
		_, err := w.Write(b[:])
		return err
	})
}

// ReceiveFrames reads frames until the peer ends the stream with a clean EOF on a frame boundary.
func ReceiveFrames(r io.Reader, clock Clock) ([]time.Duration, error) {
	var (
		buf     [FrameSize]byte
		samples []time.Duration
	)
	for {
		_, err := io.ReadFull(r, buf[:])
		if errors.Is(err, io.EOF) {
			return samples, nil
		}
		if err != nil {
			return samples, fmt.Errorf("read frame: %w", err)
		}
		f, err := DecodeFrame(buf[:])
		if err != nil {
			return samples, err
		}
		samples = append(samples, clock.Since(f.SentNS))
	}
}

// EchoFrames copies frames from r back to w until r reaches EOF.
func EchoFrames(r io.Reader, w io.Writer) error {
	var buf [FrameSize]byte
	for {
		_, err := io.ReadFull(r, buf[:])
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("read frame: %w", err)
		}
		if _, err := w.Write(buf[:]); err != nil {
			return fmt.Errorf("echo frame: %w", err)
		}
	}
}

// WriteBulk writes BulkBufferSize chunks to w until ctx is done and returns the number of bytes written.
func WriteBulk(ctx context.Context, w io.Writer) (int64, error) {
	buf := make([]byte, BulkBufferSize)
	var total int64
	for ctx.Err() == nil {
		n, err := w.Write(buf)
		total += int64(n)
		if err != nil {
			return total, fmt.Errorf("write bulk: %w", err)
		}
	}
	return total, nil
}

func DrainBulk(r io.Reader) (int64, error) {
	n, err := io.Copy(io.Discard, r)
	if err != nil {
		return n, fmt.Errorf("drain bulk: %w", err)
	}
	return n, nil
}

type Summary struct {
	N             int
	P50, P99, Max time.Duration
}

// Summarize computes nearest-rank percentiles over a copy of samples.
func Summarize(samples []time.Duration) (Summary, error) {
	if len(samples) == 0 {
		return Summary{}, ErrNoSamples
	}
	sorted := slices.Clone(samples)
	slices.Sort(sorted)
	return Summary{
		N:   len(sorted),
		P50: percentile(sorted, 0.50),
		P99: percentile(sorted, 0.99),
		Max: sorted[len(sorted)-1],
	}, nil
}

func percentile(sorted []time.Duration, p float64) time.Duration {
	rank := int(math.Ceil(p * float64(len(sorted))))
	return sorted[rank-1]
}

func (s Summary) String() string {
	return fmt.Sprintf("n=%d p50=%v p99=%v max=%v", s.N, s.P50, s.P99, s.Max)
}
