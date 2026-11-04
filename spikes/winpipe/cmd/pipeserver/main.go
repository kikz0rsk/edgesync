//go:build windows

// Command pipeserver stands in for the service signing proxy of D-41 in the P0-SP1-005 spike. It generates an ECDSA
// P-256 key in memory and signs over a named pipe whose DACL admits only the agent account; every message re-checks the
// client account, session and image. The user runs it (D-74); agents build it and never run it.
package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/Microsoft/go-winio"

	"github.com/kikz0r/edgesync/spikes/winpipe"
)

const defaultPipe = `\\.\pipe\EdgeSyncWinpipeSpike`

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "pipeserver:", err)
		os.Exit(1)
	}
}

func run(args []string, out io.Writer) error {
	user, err := winpipe.CurrentUserSID()
	if err != nil {
		return err
	}
	session, err := winpipe.CurrentSession()
	if err != nil {
		return err
	}
	self, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate this executable: %w", err)
	}
	clientImage := filepath.Join(filepath.Dir(self), "pipeclient.exe")

	flags := flag.NewFlagSet("pipeserver", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	pipe := flags.String("pipe", defaultPipe, "named pipe path")
	sid := flags.String("sid", user, "SID of the only account allowed to connect (default: this account)")
	sessionID := flags.Uint("session", uint(session), "session the client must run in (default: this session)")
	imagePath := flags.String("image", clientImage, "full image path the client must run from (default: pipeclient.exe next to this executable)")
	duration := flags.Duration("duration", 10*time.Minute, "stop after this run time")
	if err := flags.Parse(args); err != nil {
		return fmt.Errorf("flags: %w", err)
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected arguments: %v", flags.Args())
	}

	log := slog.New(slog.NewTextHandler(out, nil))
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return fmt.Errorf("generate key: %w", err)
	}
	responder, err := winpipe.NewResponder(key)
	if err != nil {
		return err
	}
	sddl, err := winpipe.UserPipeSDDL(*sid)
	if err != nil {
		return err
	}
	ln, err := winio.ListenPipe(*pipe, &winio.PipeConfig{SecurityDescriptor: sddl})
	if err != nil {
		return fmt.Errorf("listen on %s: %w", *pipe, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), *duration)
	var clients sync.WaitGroup
	defer func() {
		cancel()
		clients.Wait()
	}()
	context.AfterFunc(ctx, func() { ln.Close() })

	want := winpipe.Expectation{SID: *sid, Session: uint32(*sessionID), Image: *imagePath}
	log.Info("serving", "pipe", *pipe, "sid", want.SID, "session", want.Session, "image", want.Image, "duration", duration.String())
	for {
		conn, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil {
				log.Info("stopped after run time")
				return nil
			}
			return fmt.Errorf("accept: %w", err)
		}
		clients.Add(1)
		go func() {
			defer clients.Done()
			if err := responder.Serve(ctx, conn, winpipe.ClientPeer, want); err != nil {
				log.Warn("client refused or ended with error", "err", err)
				return
			}
			log.Info("client disconnected cleanly")
		}()
	}
}
