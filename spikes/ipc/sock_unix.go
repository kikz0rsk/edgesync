//go:build linux || darwin

// Package ipc is the P0-SP5-006 spike for the per-user agent IPC transports (D-46); see README.md.
package ipc

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
)

const socketFileMode os.FileMode = 0o600

func listenSocket(dir, name string) (*net.UnixListener, error) {
	path := filepath.Join(dir, name)
	ln, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		return nil, fmt.Errorf("listen on %s: %w", path, err)
	}
	if err := os.Chmod(path, socketFileMode); err != nil {
		ln.Close()
		return nil, fmt.Errorf("chmod %s: %w", path, err)
	}
	return ln, nil
}

func peerUID(conn *net.UnixConn) (uint32, error) {
	raw, err := conn.SyscallConn()
	if err != nil {
		return 0, fmt.Errorf("syscall conn: %w", err)
	}
	var uid uint32
	var credErr error
	if err := raw.Control(func(fd uintptr) {
		uid, credErr = peerCredentialUID(int(fd))
	}); err != nil {
		return 0, fmt.Errorf("control socket: %w", err)
	}
	if credErr != nil {
		return 0, fmt.Errorf("peer credentials: %w", credErr)
	}
	return uid, nil
}
