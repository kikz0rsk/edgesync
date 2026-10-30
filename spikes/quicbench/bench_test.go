package quicbench_test

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/quic-go/quic-go"

	"github.com/kikz0r/edgesync/spikes/quicbench"
)

const (
	setupTimeout  = 10 * time.Second
	waitTimeout   = 30 * time.Second
	drainGrace    = 500 * time.Millisecond
	benchDuration = 10 * time.Second
	bytesPerMiB   = 1 << 20
)

func newClock(t *testing.T) quicbench.Clock {
	t.Helper()
	clock, err := quicbench.NewClock()
	if err != nil {
		t.Fatalf("new clock: %v", err)
	}
	return clock
}

func TestClock_ReadsDoNotAllocate(t *testing.T) {
	clock := newClock(t)
	if allocs := testing.AllocsPerRun(1000, func() { _ = clock.Now() }); allocs != 0 {
		t.Fatalf("clock.Now allocates %v times per read", allocs)
	}
}

func TestClock_Resolution(t *testing.T) {
	clock := newClock(t)
	const wantSteps = 1000
	smallest := time.Duration(0)
	seen := 0
	last := clock.Now()
	for seen < wantSteps {
		now := clock.Now()
		if now == last {
			continue
		}
		step := time.Duration(now - last)
		if smallest == 0 || step < smallest {
			smallest = step
		}
		last = now
		seen++
	}
	t.Logf("clock smallest observed step over %d increments: %v", wantSteps, smallest)
	if smallest >= 100*time.Microsecond {
		t.Fatalf("clock step %v is too coarse for sub-millisecond latency", smallest)
	}
}

type outcome struct {
	n   int64
	err error
}

func spawn(fn func() (int64, error)) <-chan outcome {
	c := make(chan outcome, 1)
	go func() {
		n, err := fn()
		c <- outcome{n: n, err: err}
	}()
	return c
}

func collect(t *testing.T, what string, c <-chan outcome) outcome {
	t.Helper()
	select {
	case o := <-c:
		if o.err != nil {
			t.Fatalf("%s: %v", what, o.err)
		}
		return o
	case <-time.After(waitTimeout):
		t.Fatalf("%s: no result after %v", what, waitTimeout)
	}
	return outcome{}
}

type bulkFlow struct {
	cancel context.CancelFunc
	write  <-chan outcome
	read   <-chan outcome
}

func startBulk(ctx context.Context, write func(context.Context) (int64, error), read func(context.Context) (int64, error)) *bulkFlow {
	bctx, cancel := context.WithCancel(ctx)
	return &bulkFlow{
		cancel: cancel,
		write:  spawn(func() (int64, error) { return write(bctx) }),
		read:   spawn(func() (int64, error) { return read(ctx) }),
	}
}

func (b *bulkFlow) stop(t *testing.T) int64 {
	t.Helper()
	b.cancel()
	w := collect(t, "bulk write", b.write)
	r := collect(t, "bulk read", b.read)
	if w.n != r.n {
		t.Fatalf("bulk bytes written %d, read %d", w.n, r.n)
	}
	return w.n
}

func closeListener(t *testing.T, ln *quic.Listener) {
	t.Cleanup(func() {
		if err := ln.Close(); err != nil {
			t.Errorf("close listener: %v", err)
		}
	})
}

func newQUICPair(t *testing.T) (server, client *quic.Conn) {
	t.Helper()
	ln, err := quicbench.ListenQUIC("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	closeListener(t, ln)

	ctx, cancel := context.WithTimeout(t.Context(), setupTimeout)
	defer cancel()
	type accepted struct {
		conn *quic.Conn
		err  error
	}
	acceptC := make(chan accepted, 1)
	go func() {
		conn, err := ln.Accept(ctx)
		acceptC <- accepted{conn: conn, err: err}
	}()
	client, err = quicbench.DialQUIC(ctx, ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { closeQUIC(t, client) })
	acc := <-acceptC
	if acc.err != nil {
		t.Fatalf("accept quic: %v", acc.err)
	}
	t.Cleanup(func() { closeQUIC(t, acc.conn) })
	return acc.conn, client
}

func closeQUIC(t *testing.T, conn *quic.Conn) {
	if err := conn.CloseWithError(0, "bench done"); err != nil {
		t.Errorf("close quic connection: %v", err)
	}
}

func startQUICBulk(t *testing.T, server, client *quic.Conn) *bulkFlow {
	t.Helper()
	s, err := server.OpenUniStreamSync(t.Context())
	if err != nil {
		t.Fatalf("open bulk stream: %v", err)
	}
	s.SetPriority(quicbench.UrgencyBulk, false)
	write := func(ctx context.Context) (int64, error) {
		n, err := quicbench.WriteBulk(ctx, s)
		if err != nil {
			return n, err
		}
		return n, s.Close()
	}
	read := func(ctx context.Context) (int64, error) {
		rs, err := client.AcceptUniStream(ctx)
		if err != nil {
			return 0, fmt.Errorf("accept bulk stream: %w", err)
		}
		return quicbench.DrainBulk(rs)
	}
	return startBulk(t.Context(), write, read)
}

type result struct {
	name       string
	sent       int
	samples    []time.Duration
	bulkBytes  int64
	bulkWindow time.Duration
}

func measureDatagram(t *testing.T, withBulk bool) result {
	server, client := newQUICPair(t)
	clock := newClock(t)

	var bulk *bulkFlow
	var bulkStart time.Time
	if withBulk {
		bulkStart = time.Now()
		bulk = startQUICBulk(t, server, client)
	}

	recvCtx, stopRecv := context.WithCancel(t.Context())
	defer stopRecv()
	type recvResult struct {
		samples []time.Duration
		err     error
	}
	recvC := make(chan recvResult, 1)
	go func() {
		samples, err := quicbench.ReceiveDatagrams(recvCtx, client, clock)
		recvC <- recvResult{samples: samples, err: err}
	}()

	sent, err := quicbench.SendDatagrams(t.Context(), server, clock, benchDuration)
	if err != nil {
		t.Fatalf("send datagrams: %v", err)
	}
	time.Sleep(drainGrace)
	stopRecv()
	rr := <-recvC
	if rr.err != nil {
		t.Fatalf("receive datagrams: %v", rr.err)
	}

	res := result{sent: sent, samples: rr.samples}
	if bulk != nil {
		res.bulkBytes = bulk.stop(t)
		res.bulkWindow = time.Since(bulkStart)
	}
	return res
}

func measureInputStream(t *testing.T) result {
	server, client := newQUICPair(t)
	clock := newClock(t)

	bulkStart := time.Now()
	bulk := startQUICBulk(t, server, client)

	in, err := server.OpenStreamSync(t.Context())
	if err != nil {
		t.Fatalf("open input stream: %v", err)
	}
	in.SetPriority(quicbench.UrgencyInput, false)
	sendC := spawn(func() (int64, error) {
		n, err := quicbench.SendFrames(t.Context(), in, clock, benchDuration)
		if err != nil {
			return int64(n), err
		}
		return int64(n), in.Close()
	})

	ctx, cancel := context.WithTimeout(t.Context(), setupTimeout)
	defer cancel()
	inClient, err := client.AcceptStream(ctx)
	if err != nil {
		t.Fatalf("accept input stream: %v", err)
	}
	samples, err := quicbench.ReceiveFrames(inClient, clock)
	if err != nil {
		t.Fatalf("receive input frames: %v", err)
	}
	sent := collect(t, "send input frames", sendC)

	res := result{sent: int(sent.n), samples: samples}
	res.bulkBytes = bulk.stop(t)
	res.bulkWindow = time.Since(bulkStart)
	return res
}

func newTCPPair(t *testing.T) (server, client *tls.Conn) {
	t.Helper()
	srvConf, err := quicbench.ServerTLSConfig()
	if err != nil {
		t.Fatal(err)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen tcp: %v", err)
	}
	t.Cleanup(func() {
		if err := ln.Close(); err != nil {
			t.Errorf("close tcp listener: %v", err)
		}
	})

	ctx, cancel := context.WithTimeout(t.Context(), setupTimeout)
	defer cancel()
	type accepted struct {
		conn *tls.Conn
		err  error
	}
	acceptC := make(chan accepted, 1)
	go func() {
		raw, err := ln.Accept()
		if err != nil {
			acceptC <- accepted{err: err}
			return
		}
		if err := raw.(*net.TCPConn).SetNoDelay(true); err != nil {
			acceptC <- accepted{err: err}
			return
		}
		tc := tls.Server(raw, srvConf)
		err = tc.HandshakeContext(ctx)
		acceptC <- accepted{conn: tc, err: err}
	}()

	raw, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("dial tcp: %v", err)
	}
	if err := raw.(*net.TCPConn).SetNoDelay(true); err != nil {
		t.Fatal(err)
	}
	client = tls.Client(raw, quicbench.ClientTLSConfig())
	t.Cleanup(func() {
		if err := client.Close(); err != nil {
			t.Errorf("close tls client: %v", err)
		}
	})
	if err := client.HandshakeContext(ctx); err != nil {
		t.Fatalf("tls client handshake: %v", err)
	}
	acc := <-acceptC
	if acc.err != nil {
		t.Fatalf("tls server handshake: %v", acc.err)
	}
	t.Cleanup(func() {
		if err := acc.conn.Close(); err != nil {
			t.Errorf("close tls server: %v", err)
		}
	})
	for _, st := range []tls.ConnectionState{client.ConnectionState(), acc.conn.ConnectionState()} {
		if st.Version != tls.VersionTLS13 || st.NegotiatedProtocol != quicbench.ALPN {
			t.Fatalf("tls state version=%x alpn=%q, want TLS 1.3 and %q", st.Version, st.NegotiatedProtocol, quicbench.ALPN)
		}
	}
	return acc.conn, client
}

func measureTCPFallback(t *testing.T) result {
	inServer, inClient := newTCPPair(t)
	bulkServer, bulkClient := newTCPPair(t)
	clock := newClock(t)

	bulkStart := time.Now()
	bulk := startBulk(t.Context(),
		func(ctx context.Context) (int64, error) {
			n, err := quicbench.WriteBulk(ctx, bulkServer)
			if err != nil {
				return n, err
			}
			return n, bulkServer.CloseWrite()
		},
		func(ctx context.Context) (int64, error) {
			return quicbench.DrainBulk(bulkClient)
		},
	)

	sendC := spawn(func() (int64, error) {
		n, err := quicbench.SendFrames(t.Context(), inServer, clock, benchDuration)
		if err != nil {
			return int64(n), err
		}
		return int64(n), inServer.CloseWrite()
	})
	samples, err := quicbench.ReceiveFrames(inClient, clock)
	if err != nil {
		t.Fatalf("receive input frames: %v", err)
	}
	sent := collect(t, "send input frames", sendC)

	res := result{sent: int(sent.n), samples: samples}
	res.bulkBytes = bulk.stop(t)
	res.bulkWindow = time.Since(bulkStart)
	return res
}

func logResult(t *testing.T, r result) {
	t.Helper()
	s, err := quicbench.Summarize(r.samples)
	if err != nil {
		t.Fatalf("%s: %v", r.name, err)
	}
	lost := r.sent - s.N
	line := fmt.Sprintf("%s: sent=%d received=%d lost=%d latency %s", r.name, r.sent, s.N, lost, s)
	if r.bulkWindow > 0 {
		mibps := float64(r.bulkBytes) / bytesPerMiB / r.bulkWindow.Seconds()
		line += fmt.Sprintf(" bulk=%.1f MiB/s", mibps)
	}
	t.Log(line)
}

func TestDatagramLatency_Idle(t *testing.T) {
	r := measureDatagram(t, false)
	r.name = "datagram idle"
	logResult(t, r)
}

func TestDatagramLatency_UnderBulk(t *testing.T) {
	r := measureDatagram(t, true)
	r.name = "datagram under bulk"
	logResult(t, r)
}

func TestInputStreamLatency_UnderBulk(t *testing.T) {
	r := measureInputStream(t)
	r.name = "input stream under bulk"
	if len(r.samples) != r.sent {
		t.Fatalf("input stream delivered %d of %d frames", len(r.samples), r.sent)
	}
	logResult(t, r)
}

func TestTCPFallbackLatency_UnderBulk(t *testing.T) {
	r := measureTCPFallback(t)
	r.name = "tcp fallback under bulk"
	if len(r.samples) != r.sent {
		t.Fatalf("tcp input delivered %d of %d frames", len(r.samples), r.sent)
	}
	logResult(t, r)
}
