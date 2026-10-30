// Command quicbench is the two-machine LAN probe for the QUIC transport bench. The server echoes datagrams and input
// frames and drains an optional bulk stream; the client sends 1000 Hz frames, measures round trips on its own clock
// and reports one-way estimates as RTT/2.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/quic-go/quic-go"

	"github.com/kikz0r/edgesync/spikes/quicbench"
)

const (
	dialTimeout = 10 * time.Second
	drainGrace  = 500 * time.Millisecond
	bytesPerMiB = 1 << 20
)

type countResult struct {
	n   int
	err error
}

type samplesResult struct {
	samples []time.Duration
	err     error
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "quicbench:", err)
		os.Exit(1)
	}
}

func run() error {
	mode := flag.String("mode", "", "server or client")
	addr := flag.String("addr", ":24860", "server: listen address; client: server host:port (required)")
	duration := flag.Duration("duration", 30*time.Second, "client: measurement duration (the server runs until interrupted)")
	bulk := flag.Bool("bulk", false, "client: also send a saturating client-to-server bulk stream at urgency 7")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	switch *mode {
	case "server":
		return runServer(ctx, *addr)
	case "client":
		return runClient(ctx, *addr, *duration, *bulk)
	default:
		return fmt.Errorf("-mode must be server or client, got %q", *mode)
	}
}

func runServer(ctx context.Context, addr string) error {
	ln, err := quicbench.ListenQUIC(addr)
	if err != nil {
		return err
	}
	defer func() {
		if err := ln.Close(); err != nil {
			fmt.Fprintln(os.Stderr, "quicbench: close listener:", err)
		}
	}()
	fmt.Printf("listening on %s\n", ln.Addr())
	for {
		conn, err := ln.Accept(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("accept: %w", err)
		}
		go serveConn(ctx, conn)
	}
}

func serveConn(ctx context.Context, conn *quic.Conn) {
	peer := conn.RemoteAddr()
	fmt.Printf("%s connected\n", peer)
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	go func() {
		if err := quicbench.EchoDatagrams(ctx, conn); err != nil {
			fmt.Printf("%s datagram echo: %v\n", peer, err)
		}
	}()
	go func() {
		u, err := conn.AcceptUniStream(ctx)
		if err != nil {
			return
		}
		n, err := quicbench.DrainBulk(u)
		fmt.Printf("%s bulk received %d bytes: %v\n", peer, n, err)
	}()

	s, err := conn.AcceptStream(ctx)
	if err != nil {
		fmt.Printf("%s accept input stream: %v\n", peer, err)
		return
	}
	if err := quicbench.EchoFrames(s, s); err != nil {
		fmt.Printf("%s input echo: %v\n", peer, err)
		return
	}
	if err := s.Close(); err != nil {
		fmt.Printf("%s close input stream: %v\n", peer, err)
	}
	<-conn.Context().Done()
	fmt.Printf("%s closed: %v\n", peer, context.Cause(conn.Context()))
}

func runClient(ctx context.Context, addr string, dur time.Duration, bulk bool) error {
	if strings.TrimSpace(addr) == "" || strings.HasPrefix(addr, ":") {
		return errors.New("client needs -addr host:port of the server")
	}
	dialCtx, cancelDial := context.WithTimeout(ctx, dialTimeout)
	conn, err := quicbench.DialQUIC(dialCtx, addr)
	cancelDial()
	if err != nil {
		return err
	}
	defer func() {
		if err := conn.CloseWithError(0, "done"); err != nil {
			fmt.Fprintln(os.Stderr, "quicbench: close connection:", err)
		}
	}()

	clock, err := quicbench.NewClock()
	if err != nil {
		return err
	}
	in, err := conn.OpenStreamSync(ctx)
	if err != nil {
		return fmt.Errorf("open input stream: %w", err)
	}
	in.SetPriority(quicbench.UrgencyInput, false)

	bulkC := make(chan countResult, 1)
	bulkStart := time.Now()
	if bulk {
		u, err := conn.OpenUniStreamSync(ctx)
		if err != nil {
			return fmt.Errorf("open bulk stream: %w", err)
		}
		u.SetPriority(quicbench.UrgencyBulk, false)
		go func() {
			bctx, cancel := context.WithTimeout(ctx, dur)
			defer cancel()
			n, err := quicbench.WriteBulk(bctx, u)
			if err == nil {
				err = u.Close()
			}
			bulkC <- countResult{n: int(n), err: err}
		}()
	} else {
		bulkC <- countResult{}
	}

	recvCtx, stopRecv := context.WithCancel(ctx)
	defer stopRecv()
	dgRecvC := make(chan samplesResult, 1)
	go func() {
		samples, err := quicbench.ReceiveDatagrams(recvCtx, conn, clock)
		dgRecvC <- samplesResult{samples: samples, err: err}
	}()
	dgSendC := make(chan countResult, 1)
	go func() {
		n, err := quicbench.SendDatagrams(ctx, conn, clock, dur)
		dgSendC <- countResult{n: n, err: err}
	}()

	inRecvC := make(chan samplesResult, 1)
	go func() {
		samples, err := quicbench.ReceiveFrames(in, clock)
		inRecvC <- samplesResult{samples: samples, err: err}
	}()
	inSendC := make(chan countResult, 1)
	go func() {
		n, err := quicbench.SendFrames(ctx, in, clock, dur)
		if err == nil {
			err = in.Close()
		}
		inSendC <- countResult{n: n, err: err}
	}()

	dgSent := <-dgSendC
	if dgSent.err != nil {
		return fmt.Errorf("send datagrams: %w", dgSent.err)
	}
	inSent := <-inSendC
	if inSent.err != nil {
		return fmt.Errorf("send input frames: %w", inSent.err)
	}
	time.Sleep(drainGrace)
	stopRecv()

	dgRecv := <-dgRecvC
	if dgRecv.err != nil {
		return fmt.Errorf("datagram receive: %w", dgRecv.err)
	}
	inRecv := <-inRecvC
	if inRecv.err != nil {
		return fmt.Errorf("input stream receive: %w", inRecv.err)
	}

	report("datagram", dgSent.n, dgRecv.samples)
	report("input stream", inSent.n, inRecv.samples)
	if bulk {
		b := <-bulkC
		if b.err != nil {
			return fmt.Errorf("bulk write: %w", b.err)
		}
		elapsed := time.Since(bulkStart)
		fmt.Printf("bulk sent %d bytes, %.1f MiB/s\n", b.n, float64(b.n)/bytesPerMiB/elapsed.Seconds())
	}
	return nil
}

// report prints RTT/2 as the one-way estimate; both ends of each sample use the client clock.
func report(name string, sent int, rtts []time.Duration) {
	oneWay := make([]time.Duration, len(rtts))
	for i, d := range rtts {
		oneWay[i] = d / 2
	}
	s, err := quicbench.Summarize(oneWay)
	if err != nil {
		fmt.Printf("%s: sent=%d received=0 (%v)\n", name, sent, err)
		return
	}
	fmt.Printf("%s: sent=%d received=%d lost=%d one-way %s\n", name, sent, s.N, sent-s.N, s)
}
