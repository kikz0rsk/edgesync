package winpipe

import (
	"bytes"
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"slices"
	"sync/atomic"
	"testing"
	"time"
)

const testImage = `C:\Program Files\EdgeSync\edgesync-agent.exe`

var (
	testExpectation = Expectation{SID: "S-1-5-21-1000-2000-3000-1001", Session: 1, Image: testImage}
	testPeer        = Peer{PID: 4242, SID: testExpectation.SID, Session: 1, Image: testImage}
	errPeerLookup   = errors.New("lookup refused")
)

type countingSigner struct {
	crypto.Signer
	calls atomic.Int32
}

func (c *countingSigner) Sign(r io.Reader, digest []byte, opts crypto.SignerOpts) ([]byte, error) {
	c.calls.Add(1)
	return c.Signer.Sign(r, digest, opts)
}

func newTestKey(t *testing.T) *ecdsa.PrivateKey {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	return key
}

func staticPeer(p Peer) PeerFunc {
	return func(net.Conn) (Peer, error) { return p, nil }
}

// startService serves one pipe connection in a goroutine and returns the client end. Errors after the test ends are
// the expected result of closing the connection.
func startService(t *testing.T, r *Responder, peer PeerFunc, want Expectation) net.Conn {
	t.Helper()
	client, server := net.Pipe()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- r.Serve(ctx, server, peer, want) }()
	t.Cleanup(func() {
		cancel()
		client.Close()
		err := <-done
		if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, io.ErrClosedPipe) {
			t.Errorf("Serve returned %v after shutdown", err)
		}
	})
	return client
}

func TestSignerProxy_TLS13HandshakeOverLoopback(t *testing.T) {
	key := newTestKey(t)
	service := &countingSigner{Signer: key}
	responder, err := NewResponder(service)
	if err != nil {
		t.Fatalf("NewResponder: %v", err)
	}
	channel := startService(t, responder, staticPeer(testPeer), testExpectation)

	proxy, err := NewProxySigner(channel)
	if err != nil {
		t.Fatalf("NewProxySigner: %v", err)
	}
	if !proxy.Public().(*ecdsa.PublicKey).Equal(&key.PublicKey) {
		t.Fatal("proxy public key differs from the service key")
	}

	cert, roots, err := SelfSignedLoopbackCert(proxy)
	if err != nil {
		t.Fatalf("SelfSignedLoopbackCert: %v", err)
	}
	afterCert := service.calls.Load()
	if afterCert != 1 {
		t.Fatalf("certificate creation caused %d service signatures, want 1", afterCert)
	}

	res, err := LoopbackHandshake(cert, roots)
	if err != nil {
		t.Fatalf("LoopbackHandshake: %v", err)
	}
	if res.Version != tls.VersionTLS13 {
		t.Fatalf("negotiated version %#x, want TLS 1.3", res.Version)
	}
	if delta := service.calls.Load() - afterCert; delta != 1 {
		t.Fatalf("handshake caused %d service signatures, want exactly 1 (CertificateVerify)", delta)
	}
	t.Logf("TLS 1.3 handshake through the signing proxy: %v", res.Elapsed)
}

func TestSignerProxy_HandshakeOverhead(t *testing.T) {
	const rounds = 10
	key := newTestKey(t)
	direct, directRoots, err := SelfSignedLoopbackCert(key)
	if err != nil {
		t.Fatalf("direct certificate: %v", err)
	}
	responder, err := NewResponder(key)
	if err != nil {
		t.Fatalf("NewResponder: %v", err)
	}
	channel := startService(t, responder, staticPeer(testPeer), testExpectation)
	proxy, err := NewProxySigner(channel)
	if err != nil {
		t.Fatalf("NewProxySigner: %v", err)
	}
	proxied, roots, err := SelfSignedLoopbackCert(proxy)
	if err != nil {
		t.Fatalf("proxied certificate: %v", err)
	}

	var directTimes, proxyTimes []time.Duration
	for range rounds {
		res, err := LoopbackHandshake(direct, directRoots)
		if err != nil {
			t.Fatalf("direct handshake: %v", err)
		}
		directTimes = append(directTimes, res.Elapsed)
		res, err = LoopbackHandshake(proxied, roots)
		if err != nil {
			t.Fatalf("proxied handshake: %v", err)
		}
		proxyTimes = append(proxyTimes, res.Elapsed)
	}
	t.Logf("handshake median direct %v, through proxy %v (%d rounds each)", median(directTimes), median(proxyTimes), rounds)
}

// Per-call timing is not used: the Windows monotonic clock is too coarse for a single signature, so the mean is taken
// over a batch.
func TestSignerProxy_SignRoundTripLatency(t *testing.T) {
	const samples = 500
	key := newTestKey(t)
	responder, err := NewResponder(key)
	if err != nil {
		t.Fatalf("NewResponder: %v", err)
	}
	channel := startService(t, responder, staticPeer(testPeer), testExpectation)
	proxy, err := NewProxySigner(channel)
	if err != nil {
		t.Fatalf("NewProxySigner: %v", err)
	}
	digest := sha256.Sum256([]byte("transcript"))
	start := time.Now()
	for range samples {
		if _, err := proxy.Sign(rand.Reader, digest[:], crypto.SHA256); err != nil {
			t.Fatalf("Sign: %v", err)
		}
	}
	elapsed := time.Since(start)
	t.Logf("signing round trip over in-process pipe (incl. client-side verify): mean %v over %d samples",
		elapsed/samples, samples)
}

func TestProxySigner_RefusesWrongHashAndDigestWithoutContactingService(t *testing.T) {
	key := newTestKey(t)
	proxy := &ProxySigner{rw: failingRW{t: t}, pub: &key.PublicKey}
	digest := sha256.Sum256([]byte("x"))
	if _, err := proxy.Sign(rand.Reader, digest[:], crypto.SHA384); !errors.Is(err, ErrHash) {
		t.Errorf("SHA-384 request: got %v, want ErrHash", err)
	}
	if _, err := proxy.Sign(rand.Reader, digest[:31], crypto.SHA256); !errors.Is(err, ErrDigest) {
		t.Errorf("31-byte digest: got %v, want ErrDigest", err)
	}
	if _, err := proxy.Sign(rand.Reader, digest[:], nil); !errors.Is(err, ErrHash) {
		t.Errorf("nil options: got %v, want ErrHash", err)
	}
}

type failingRW struct{ t *testing.T }

func (f failingRW) Read([]byte) (int, error) { f.t.Error("proxy read the channel"); return 0, io.EOF }

func (f failingRW) Write([]byte) (int, error) {
	f.t.Error("proxy wrote to the channel")
	return 0, io.EOF
}

func TestAuthorize(t *testing.T) {
	ok := testPeer
	cases := []struct {
		name    string
		peer    Peer
		want    Expectation
		wantErr error
	}{
		{name: "matching client", peer: ok, want: testExpectation},
		{name: "image compared case-insensitively", peer: Peer{SID: ok.SID, Session: 1, Image: `c:\program files\edgesync\EDGESYNC-AGENT.exe`}, want: testExpectation},
		{name: "other user account", peer: Peer{SID: "S-1-5-21-1000-2000-3000-1002", Session: 1, Image: testImage}, want: testExpectation, wantErr: ErrWrongUser},
		{name: "empty account SID", peer: Peer{Session: 1, Image: testImage}, want: testExpectation, wantErr: ErrWrongUser},
		{name: "LocalSystem instead of the user", peer: Peer{SID: "S-1-5-18", Session: 1, Image: testImage}, want: testExpectation, wantErr: ErrWrongUser},
		{name: "wrong session", peer: Peer{SID: ok.SID, Session: 2, Image: testImage}, want: testExpectation, wantErr: ErrWrongSession},
		{name: "wrong image", peer: Peer{SID: ok.SID, Session: 1, Image: `C:\Users\Public\tool.exe`}, want: testExpectation, wantErr: ErrWrongImage},
		{name: "empty expectation SID fails closed", peer: ok, want: Expectation{Session: 1, Image: testImage}, wantErr: ErrNoExpectation},
		{name: "empty expectation image fails closed", peer: Peer{SID: ok.SID, Session: 1}, want: Expectation{SID: ok.SID, Session: 1}, wantErr: ErrNoExpectation},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := Authorize(tc.peer, tc.want)
			if tc.wantErr == nil {
				if err != nil {
					t.Fatalf("Authorize: %v", err)
				}
				return
			}
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("Authorize: got %v, want %v", err, tc.wantErr)
			}
		})
	}
}

type peerOutcome struct {
	peer Peer
	err  error
}

type hostileCase struct {
	name     string
	peers    []peerOutcome
	steps    [][]byte
	statuses []byte
	serveErr error
	signs    int32
}

func frameOf(payload []byte) []byte {
	out := make([]byte, 4+len(payload))
	binary.BigEndian.PutUint32(out, uint32(len(payload)))
	copy(out[4:], payload)
	return out
}

func requestOf(op byte, seq uint64, body []byte) []byte {
	out := make([]byte, headerLen+len(body))
	out[0] = op
	binary.BigEndian.PutUint64(out[1:headerLen], seq)
	copy(out[headerLen:], body)
	return out
}

func signBody(digest []byte) []byte {
	return append([]byte{byte(crypto.SHA256)}, digest...)
}

func TestResponder_HostileClients(t *testing.T) {
	digest := sha256.Sum256([]byte("transcript"))
	signReq := func(seq uint64) []byte { return frameOf(requestOf(OpSign, seq, signBody(digest[:]))) }
	good := peerOutcome{peer: testPeer}
	withPeer := func(mutate func(*Peer)) peerOutcome {
		p := testPeer
		mutate(&p)
		return peerOutcome{peer: p}
	}

	cases := []hostileCase{
		{
			name:     "authorised public key request",
			peers:    []peerOutcome{good},
			steps:    [][]byte{frameOf(requestOf(OpPublicKey, 1, nil))},
			statuses: []byte{StatusOK},
		},
		{
			name:     "authorised signature",
			peers:    []peerOutcome{good},
			steps:    [][]byte{signReq(1)},
			statuses: []byte{StatusOK},
			signs:    1,
		},
		{
			name:     "other user account",
			peers:    []peerOutcome{withPeer(func(p *Peer) { p.SID = "S-1-5-21-1000-2000-3000-1002" })},
			steps:    [][]byte{signReq(1)},
			statuses: []byte{StatusDenied},
			serveErr: ErrWrongUser,
		},
		{
			name:     "wrong SID (empty)",
			peers:    []peerOutcome{withPeer(func(p *Peer) { p.SID = "" })},
			steps:    [][]byte{signReq(1)},
			statuses: []byte{StatusDenied},
			serveErr: ErrWrongUser,
		},
		{
			name:     "wrong session",
			peers:    []peerOutcome{withPeer(func(p *Peer) { p.Session = 0 })},
			steps:    [][]byte{signReq(1)},
			statuses: []byte{StatusDenied},
			serveErr: ErrWrongSession,
		},
		{
			name:     "wrong image",
			peers:    []peerOutcome{withPeer(func(p *Peer) { p.Image = `C:\Temp\agent.exe` })},
			steps:    [][]byte{signReq(1)},
			statuses: []byte{StatusDenied},
			serveErr: ErrWrongImage,
		},
		{
			name:     "client identity lookup fails",
			peers:    []peerOutcome{{err: errPeerLookup}},
			steps:    [][]byte{signReq(1)},
			statuses: []byte{StatusDenied},
			serveErr: errPeerLookup,
		},
		{
			name:     "replayed signature request",
			peers:    []peerOutcome{good},
			steps:    [][]byte{signReq(1), signReq(1)},
			statuses: []byte{StatusOK, StatusStale},
			serveErr: ErrStale,
			signs:    1,
		},
		{
			name:     "older sequence number after a newer one",
			peers:    []peerOutcome{good},
			steps:    [][]byte{signReq(2), signReq(1)},
			statuses: []byte{StatusOK, StatusStale},
			serveErr: ErrStale,
			signs:    1,
		},
		{
			name:     "zero sequence number",
			peers:    []peerOutcome{good},
			steps:    [][]byte{signReq(0)},
			statuses: []byte{StatusStale},
			serveErr: ErrStale,
		},
		{
			name:     "session changes between messages",
			peers:    []peerOutcome{good, withPeer(func(p *Peer) { p.Session = 7 })},
			steps:    [][]byte{signReq(1), signReq(2)},
			statuses: []byte{StatusOK, StatusDenied},
			serveErr: ErrWrongSession,
			signs:    1,
		},
		{
			name:     "unknown command",
			peers:    []peerOutcome{good},
			steps:    [][]byte{frameOf(requestOf(0x7f, 1, nil))},
			statuses: []byte{StatusUnknownOp},
			serveErr: ErrUnknownOp,
		},
		{
			name:     "oversized frame",
			peers:    []peerOutcome{good},
			steps:    [][]byte{{0x00, 0x00, 0x10, 0x01}},
			serveErr: ErrFrameSize,
		},
		{
			name:     "zero-length frame",
			peers:    []peerOutcome{good},
			steps:    [][]byte{{0x00, 0x00, 0x00, 0x00}},
			serveErr: ErrFrameSize,
		},
		{
			name:     "truncated frame body",
			peers:    []peerOutcome{good},
			steps:    [][]byte{append([]byte{0, 0, 0, 20}, 1, 2, 3, 4, 5)},
			serveErr: io.ErrUnexpectedEOF,
		},
		{
			name:     "request shorter than the header",
			peers:    []peerOutcome{good},
			steps:    [][]byte{frameOf([]byte{OpSign, 0, 0})},
			statuses: []byte{StatusBadRequest},
			serveErr: ErrBadRequest,
		},
		{
			name:     "non SHA-256 hash identifier",
			peers:    []peerOutcome{good},
			steps:    [][]byte{frameOf(requestOf(OpSign, 1, append([]byte{byte(crypto.SHA384)}, digest[:]...)))},
			statuses: []byte{StatusBadRequest},
			serveErr: ErrBadRequest,
		},
		{
			name:     "digest of the wrong length",
			peers:    []peerOutcome{good},
			steps:    [][]byte{frameOf(requestOf(OpSign, 1, signBody(digest[:31])))},
			statuses: []byte{StatusBadRequest},
			serveErr: ErrBadRequest,
		},
		{
			name:     "public key request with a body",
			peers:    []peerOutcome{good},
			steps:    [][]byte{frameOf(requestOf(OpPublicKey, 1, []byte{0}))},
			statuses: []byte{StatusBadRequest},
			serveErr: ErrBadRequest,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			runHostile(t, tc)
		})
	}
}

func runHostile(t *testing.T, tc hostileCase) {
	t.Helper()
	service := &countingSigner{Signer: newTestKey(t)}
	responder, err := NewResponder(service)
	if err != nil {
		t.Fatalf("NewResponder: %v", err)
	}
	client, server := net.Pipe()
	defer client.Close()

	calls := 0
	peer := func(net.Conn) (Peer, error) {
		outcome := tc.peers[min(calls, len(tc.peers)-1)]
		calls++
		return outcome.peer, outcome.err
	}
	done := make(chan error, 1)
	go func() { done <- responder.Serve(context.Background(), server, peer, testExpectation) }()

	var statuses []byte
	for i, frame := range tc.steps {
		if _, err := client.Write(frame); err != nil {
			break
		}
		if i >= len(tc.statuses) {
			continue
		}
		resp, err := readFrame(client)
		if err != nil {
			break
		}
		statuses = append(statuses, resp[0])
	}
	client.Close()
	serveErr := <-done

	if !bytes.Equal(tc.statuses, statuses) {
		t.Errorf("statuses %v, want %v", statuses, tc.statuses)
	}
	switch {
	case tc.serveErr == nil && serveErr != nil:
		t.Errorf("Serve: got %v, want nil", serveErr)
	case tc.serveErr != nil && !errors.Is(serveErr, tc.serveErr):
		t.Errorf("Serve: got %v, want %v", serveErr, tc.serveErr)
	}
	if got := service.calls.Load(); got != tc.signs {
		t.Errorf("service signed %d times, want %d", got, tc.signs)
	}
}

func TestFrame_Bounds(t *testing.T) {
	cases := []struct {
		name string
		in   []byte
		want error
	}{
		{name: "empty", in: []byte{0, 0, 0, 0}, want: ErrFrameSize},
		{name: "one over the cap", in: []byte{0, 0, 0x10, 0x01}, want: ErrFrameSize},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := readFrame(bytes.NewReader(tc.in)); !errors.Is(err, tc.want) {
				t.Fatalf("readFrame: got %v, want %v", err, tc.want)
			}
		})
	}
	if err := writeFrame(io.Discard, make([]byte, MaxFrame+1)); !errors.Is(err, ErrFrameSize) {
		t.Fatalf("writeFrame over the cap: got %v, want ErrFrameSize", err)
	}
	if err := writeFrame(io.Discard, nil); !errors.Is(err, ErrFrameSize) {
		t.Fatalf("writeFrame empty: got %v, want ErrFrameSize", err)
	}
	if _, err := readFrame(bytes.NewReader(nil)); !errors.Is(err, io.EOF) {
		t.Fatalf("readFrame at a boundary: got %v, want io.EOF", err)
	}
}

func median(d []time.Duration) time.Duration {
	s := slices.Clone(d)
	slices.Sort(s)
	return s[len(s)/2]
}
