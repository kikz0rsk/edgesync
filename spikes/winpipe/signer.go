// Package winpipe is the P0-SP1-005 spike for the Windows named-pipe channels of D-46 and D-47 and the TLS signing
// proxy of D-41: an agent completes a TLS 1.3 handshake whose private-key operation is performed by the service over
// an authenticated pipe. This file holds the OS-independent parts (wire format, per-message authorisation, the signing
// proxy and responder, the loopback handshake helper) so they build and test on every platform; the pipe specifics
// live in the *_windows.go files.
package winpipe

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"strings"
	"sync"
	"time"
)

const (
	MaxFrame = 4096

	OpPublicKey byte = 1
	OpSign      byte = 2

	StatusOK         byte = 0
	StatusDenied     byte = 1
	StatusBadRequest byte = 2
	StatusUnknownOp  byte = 3
	StatusStale      byte = 4
	StatusFailed     byte = 5

	headerLen = 9
)

var (
	ErrFrameSize     = errors.New("winpipe: frame size out of range")
	ErrNoExpectation = errors.New("winpipe: expectation names no account or image")
	ErrWrongUser     = errors.New("winpipe: client account differs")
	ErrWrongSession  = errors.New("winpipe: client session differs")
	ErrWrongImage    = errors.New("winpipe: client image differs")
	ErrBadRequest    = errors.New("winpipe: malformed request")
	ErrUnknownOp     = errors.New("winpipe: unknown operation")
	ErrStale         = errors.New("winpipe: sequence number is not newer than the last accepted one")
	ErrRemote        = errors.New("winpipe: service refused the request")
	ErrResponse      = errors.New("winpipe: malformed response")
	ErrHash          = errors.New("winpipe: only SHA-256 digests are signed")
	ErrDigest        = errors.New("winpipe: digest must be 32 bytes")
	ErrKey           = errors.New("winpipe: key must be ECDSA P-256")
	ErrSignature     = errors.New("winpipe: returned signature does not verify")
)

// Peer is what the service learns about the client process of one pipe message.
type Peer struct {
	PID     uint32
	SID     string
	Session uint32
	Image   string
}

// Expectation is the only client identity the service accepts.
type Expectation struct {
	SID     string
	Session uint32
	Image   string
}

// Authorize is evaluated before every message: the account, the session and the image must all match. An empty
// expectation fails closed.
func Authorize(p Peer, want Expectation) error {
	switch {
	case want.SID == "" || want.Image == "":
		return ErrNoExpectation
	case p.SID != want.SID:
		return fmt.Errorf("%w: %q, want %q", ErrWrongUser, p.SID, want.SID)
	case p.Session != want.Session:
		return fmt.Errorf("%w: session %d, want %d", ErrWrongSession, p.Session, want.Session)
	case !strings.EqualFold(p.Image, want.Image):
		return fmt.Errorf("%w: %q, want %q", ErrWrongImage, p.Image, want.Image)
	}
	return nil
}

// PeerFunc resolves the client of one connection at the moment a message arrives.
type PeerFunc func(conn net.Conn) (Peer, error)

func writeFrame(w io.Writer, payload []byte) error {
	if len(payload) == 0 || len(payload) > MaxFrame {
		return fmt.Errorf("%w: %d bytes", ErrFrameSize, len(payload))
	}
	buf := make([]byte, 4+len(payload))
	binary.BigEndian.PutUint32(buf, uint32(len(payload)))
	copy(buf[4:], payload)
	if _, err := w.Write(buf); err != nil {
		return fmt.Errorf("winpipe: write frame: %w", err)
	}
	return nil
}

// readFrame returns an error wrapping io.EOF only when the stream ends exactly on a frame boundary.
func readFrame(r io.Reader) ([]byte, error) {
	var header [4]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return nil, fmt.Errorf("winpipe: read frame header: %w", err)
	}
	size := binary.BigEndian.Uint32(header[:])
	if size == 0 || size > MaxFrame {
		return nil, fmt.Errorf("%w: %d bytes", ErrFrameSize, size)
	}
	payload := make([]byte, size)
	if _, err := io.ReadFull(r, payload); err != nil {
		if errors.Is(err, io.EOF) {
			err = io.ErrUnexpectedEOF
		}
		return nil, fmt.Errorf("winpipe: read frame body: %w", err)
	}
	return payload, nil
}

func reply(status byte, seq uint64, body []byte) []byte {
	out := make([]byte, headerLen+len(body))
	out[0] = status
	binary.BigEndian.PutUint64(out[1:headerLen], seq)
	copy(out[headerLen:], body)
	return out
}

func requestSeq(payload []byte) uint64 {
	if len(payload) < headerLen {
		return 0
	}
	return binary.BigEndian.Uint64(payload[1:headerLen])
}

// ProxySigner is the agent-side crypto.Signer: the private key stays with the service, Sign forwards the digest over
// the authenticated channel and verifies the answer before returning it.
type ProxySigner struct {
	mu  sync.Mutex
	rw  io.ReadWriter
	seq uint64
	pub *ecdsa.PublicKey
}

// NewProxySigner fetches the public key once; the channel is used strictly request by request.
func NewProxySigner(rw io.ReadWriter) (*ProxySigner, error) {
	p := &ProxySigner{rw: rw}
	der, err := p.roundTrip(OpPublicKey, nil)
	if err != nil {
		return nil, err
	}
	key, err := x509.ParsePKIXPublicKey(der)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrResponse, err)
	}
	pub, ok := key.(*ecdsa.PublicKey)
	if !ok || pub.Curve != elliptic.P256() {
		return nil, ErrKey
	}
	p.pub = pub
	return p, nil
}

func (p *ProxySigner) Public() crypto.PublicKey {
	return p.pub
}

func (p *ProxySigner) Sign(_ io.Reader, digest []byte, opts crypto.SignerOpts) ([]byte, error) {
	if opts == nil || opts.HashFunc() != crypto.SHA256 {
		return nil, ErrHash
	}
	if len(digest) != sha256.Size {
		return nil, fmt.Errorf("%w: %d bytes", ErrDigest, len(digest))
	}
	body := make([]byte, 1+len(digest))
	body[0] = byte(crypto.SHA256)
	copy(body[1:], digest)

	p.mu.Lock()
	defer p.mu.Unlock()
	sig, err := p.roundTrip(OpSign, body)
	if err != nil {
		return nil, err
	}
	if !ecdsa.VerifyASN1(p.pub, digest, sig) {
		return nil, ErrSignature
	}
	return sig, nil
}

// roundTrip must be called with p.mu held (or before the signer is shared).
func (p *ProxySigner) roundTrip(op byte, body []byte) ([]byte, error) {
	p.seq++
	seq := p.seq
	req := make([]byte, headerLen+len(body))
	req[0] = op
	binary.BigEndian.PutUint64(req[1:headerLen], seq)
	copy(req[headerLen:], body)
	if err := writeFrame(p.rw, req); err != nil {
		return nil, err
	}
	resp, err := readFrame(p.rw)
	if err != nil {
		return nil, err
	}
	if len(resp) < headerLen {
		return nil, fmt.Errorf("%w: %d bytes", ErrResponse, len(resp))
	}
	if status := resp[0]; status != StatusOK {
		return nil, fmt.Errorf("%w: status %d", ErrRemote, status)
	}
	if binary.BigEndian.Uint64(resp[1:headerLen]) != seq {
		return nil, fmt.Errorf("%w: sequence mismatch", ErrResponse)
	}
	return resp[headerLen:], nil
}

// Responder runs in the service and holds the only copy of the private key.
type Responder struct {
	key    crypto.Signer
	pubDER []byte
}

func NewResponder(key crypto.Signer) (*Responder, error) {
	pub, ok := key.Public().(*ecdsa.PublicKey)
	if !ok || pub.Curve != elliptic.P256() {
		return nil, ErrKey
	}
	der, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		return nil, fmt.Errorf("winpipe: marshal public key: %w", err)
	}
	return &Responder{key: key, pubDER: der}, nil
}

// Serve answers requests on conn until the client closes it or a request is refused. The client is checked before
// every request. A refusal is answered with one status frame and ends the connection.
func (r *Responder) Serve(ctx context.Context, conn net.Conn, peer PeerFunc, want Expectation) error {
	stop := context.AfterFunc(ctx, func() { conn.Close() })
	defer stop()
	defer conn.Close()
	var lastSeq uint64
	for {
		payload, err := readFrame(conn)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return ctxOr(ctx, err)
		}
		out, err := r.respond(conn, peer, want, &lastSeq, payload)
		if werr := writeFrame(conn, out); werr != nil {
			return ctxOr(ctx, werr)
		}
		if err != nil {
			return err
		}
	}
}

func (r *Responder) respond(conn net.Conn, peer PeerFunc, want Expectation, lastSeq *uint64, payload []byte) ([]byte, error) {
	seq := requestSeq(payload)
	client, err := peer(conn)
	if err != nil {
		return reply(StatusDenied, seq, nil), fmt.Errorf("winpipe: client identity: %w", err)
	}
	if err := Authorize(client, want); err != nil {
		return reply(StatusDenied, seq, nil), err
	}
	if len(payload) < headerLen {
		return reply(StatusBadRequest, seq, nil), fmt.Errorf("%w: %d bytes", ErrBadRequest, len(payload))
	}
	op := payload[0]
	body := payload[headerLen:]
	switch op {
	case OpPublicKey:
		if len(body) != 0 {
			return reply(StatusBadRequest, seq, nil), fmt.Errorf("%w: public key request carries a body", ErrBadRequest)
		}
	case OpSign:
		if len(body) != 1+sha256.Size || body[0] != byte(crypto.SHA256) {
			return reply(StatusBadRequest, seq, nil), fmt.Errorf("%w: sign needs a SHA-256 id and a 32-byte digest", ErrBadRequest)
		}
	default:
		return reply(StatusUnknownOp, seq, nil), fmt.Errorf("%w: %d", ErrUnknownOp, op)
	}
	if seq <= *lastSeq {
		return reply(StatusStale, seq, nil), fmt.Errorf("%w: %d after %d", ErrStale, seq, *lastSeq)
	}
	*lastSeq = seq
	if op == OpPublicKey {
		return reply(StatusOK, seq, r.pubDER), nil
	}
	sig, err := r.key.Sign(rand.Reader, body[1:], crypto.SHA256)
	if err != nil {
		return reply(StatusFailed, seq, nil), fmt.Errorf("winpipe: sign: %w", err)
	}
	return reply(StatusOK, seq, sig), nil
}

func ctxOr(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return err
}

// SelfSignedLoopbackCert issues the test server certificate with the given signer, which may be the proxy itself.
// The returned pool trusts exactly that certificate.
func SelfSignedLoopbackCert(signer crypto.Signer) (tls.Certificate, *x509.CertPool, error) {
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "edgesync-winpipe-spike"},
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(time.Hour),
		IPAddresses:  []net.IP{net.IPv4(127, 0, 0, 1)},
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, signer.Public(), signer)
	if err != nil {
		return tls.Certificate{}, nil, fmt.Errorf("winpipe: create certificate: %w", err)
	}
	leaf, err := x509.ParseCertificate(der)
	if err != nil {
		return tls.Certificate{}, nil, fmt.Errorf("winpipe: parse certificate: %w", err)
	}
	roots := x509.NewCertPool()
	roots.AddCert(leaf)
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: signer, Leaf: leaf}, roots, nil
}

type HandshakeResult struct {
	Elapsed time.Duration
	Version uint16
}

// LoopbackHandshake runs one TLS 1.3 handshake over 127.0.0.1: cert is presented by the server side, roots verify it
// on the client side. Elapsed covers the client dial and handshake, including every server-side signature.
func LoopbackHandshake(cert tls.Certificate, roots *x509.CertPool) (HandshakeResult, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return HandshakeResult{}, fmt.Errorf("winpipe: listen: %w", err)
	}
	defer ln.Close()

	serverDone := make(chan error, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			serverDone <- fmt.Errorf("winpipe: accept: %w", err)
			return
		}
		defer conn.Close()
		srv := tls.Server(conn, &tls.Config{
			MinVersion:   tls.VersionTLS13,
			Certificates: []tls.Certificate{cert},
		})
		if err := srv.Handshake(); err != nil {
			serverDone <- fmt.Errorf("winpipe: server handshake: %w", err)
			return
		}
		serverDone <- nil
	}()

	start := time.Now()
	client, dialErr := tls.Dial("tcp", ln.Addr().String(), &tls.Config{
		MinVersion: tls.VersionTLS13,
		RootCAs:    roots,
	})
	elapsed := time.Since(start)
	if dialErr != nil {
		ln.Close()
		serverErr := <-serverDone
		return HandshakeResult{}, errors.Join(fmt.Errorf("winpipe: client handshake: %w", dialErr), serverErr)
	}
	version := client.ConnectionState().Version
	client.Close()
	if err := <-serverDone; err != nil {
		return HandshakeResult{}, err
	}
	return HandshakeResult{Elapsed: elapsed, Version: version}, nil
}
