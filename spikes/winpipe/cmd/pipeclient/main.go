//go:build windows

// Command pipeclient is the agent side of the P0-SP1-005 spike. It dials the signing proxy over a named pipe, verifies
// that the server runs as the expected account, then runs TLS 1.3 handshakes whose server signature the proxy produces,
// next to a baseline handshake with a local key. Run by the user from a second account too (D-74); agents build it and
// never run it.
package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"flag"
	"fmt"
	"io"
	"os"
	"slices"
	"time"

	"github.com/Microsoft/go-winio"

	"github.com/kikz0r/edgesync/spikes/winpipe"
)

const defaultPipe = `\\.\pipe\EdgeSyncWinpipeSpike`

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "pipeclient:", err)
		os.Exit(1)
	}
}

func run(args []string, out io.Writer) error {
	user, err := winpipe.CurrentUserSID()
	if err != nil {
		return err
	}

	flags := flag.NewFlagSet("pipeclient", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	pipe := flags.String("pipe", defaultPipe, "named pipe path")
	serverSID := flags.String("server-sid", user, "account the server must run as (use S-1-5-18 for a LocalSystem server)")
	rounds := flags.Int("handshakes", 10, "handshakes per variant")
	if err := flags.Parse(args); err != nil {
		return fmt.Errorf("flags: %w", err)
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected arguments: %v", flags.Args())
	}
	if *rounds < 1 {
		return fmt.Errorf("-handshakes must be at least 1, got %d", *rounds)
	}

	timeout := 5 * time.Second
	channel, err := winio.DialPipe(*pipe, &timeout)
	if err != nil {
		return fmt.Errorf("dial %s: %w", *pipe, err)
	}
	defer channel.Close()
	if err := winpipe.VerifyServer(channel, *serverSID); err != nil {
		return err
	}
	proxy, err := winpipe.NewProxySigner(channel)
	if err != nil {
		return err
	}
	proxied, proxyRoots, err := winpipe.SelfSignedLoopbackCert(proxy)
	if err != nil {
		return err
	}

	localKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return fmt.Errorf("generate local key: %w", err)
	}
	local, localRoots, err := winpipe.SelfSignedLoopbackCert(localKey)
	if err != nil {
		return err
	}

	directTimes := make([]time.Duration, 0, *rounds)
	proxyTimes := make([]time.Duration, 0, *rounds)
	for i := range *rounds {
		direct, err := winpipe.LoopbackHandshake(local, localRoots)
		if err != nil {
			return fmt.Errorf("baseline handshake %d: %w", i+1, err)
		}
		directTimes = append(directTimes, direct.Elapsed)
		viaProxy, err := winpipe.LoopbackHandshake(proxied, proxyRoots)
		if err != nil {
			return fmt.Errorf("proxied handshake %d: %w", i+1, err)
		}
		proxyTimes = append(proxyTimes, viaProxy.Elapsed)
		fmt.Fprintf(out, "handshake %2d: local key %s, proxy %s, TLS version %#x\n",
			i+1, ms(direct.Elapsed), ms(viaProxy.Elapsed), viaProxy.Version)
	}
	directMedian := median(directTimes)
	proxyMedian := median(proxyTimes)
	fmt.Fprintf(out, "median handshake: local key %s, proxy %s, overhead %s (%d handshakes each)\n",
		ms(directMedian), ms(proxyMedian), ms(proxyMedian-directMedian), *rounds)
	return nil
}

func median(d []time.Duration) time.Duration {
	s := slices.Clone(d)
	slices.Sort(s)
	return s[len(s)/2]
}

func ms(d time.Duration) string {
	return fmt.Sprintf("%.3f ms", float64(d.Microseconds())/1000)
}
