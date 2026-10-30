//go:build linux || darwin

package ipc

import (
	"net"
	"os"
	"path/filepath"
	"testing"
)

func TestSock_PeerCredMatchesUID(t *testing.T) {
	dir, err := os.MkdirTemp("", "edgesync-ipc-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	dirInfo, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := dirInfo.Mode().Perm(); got != 0o700 {
		t.Fatalf("socket directory mode = %o, want 700", got)
	}

	ln, err := listenSocket(dir, "agent.sock")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	sockPath := filepath.Join(dir, "agent.sock")
	sockInfo, err := os.Stat(sockPath)
	if err != nil {
		t.Fatal(err)
	}
	if sockInfo.Mode().Type() != os.ModeSocket {
		t.Fatalf("%s is %v, want a socket", sockPath, sockInfo.Mode().Type())
	}
	if got := sockInfo.Mode().Perm(); got != 0o600 {
		t.Fatalf("socket mode = %o, want 600", got)
	}

	type accepted struct {
		uid uint32
		err error
	}
	acceptedCh := make(chan accepted, 1)
	go func() {
		conn, err := ln.AcceptUnix()
		if err != nil {
			acceptedCh <- accepted{err: err}
			return
		}
		defer conn.Close()
		uid, err := peerUID(conn)
		acceptedCh <- accepted{uid: uid, err: err}
	}()

	client, err := net.DialUnix("unix", nil, &net.UnixAddr{Name: sockPath, Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	got := <-acceptedCh
	if got.err != nil {
		t.Fatal(got.err)
	}
	if want := uint32(os.Getuid()); got.uid != want {
		t.Fatalf("peer uid = %d, want %d", got.uid, want)
	}
}
