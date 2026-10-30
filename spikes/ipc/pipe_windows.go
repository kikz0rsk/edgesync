//go:build windows

// Package ipc is the P0-SP5-006 spike for the per-user agent IPC transports: a Windows named pipe with an owner-only
// DACL and server identity checks, and a unix domain socket in a 0700 directory with peer credentials (D-46).
package ipc

import (
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/Microsoft/go-winio"
	"golang.org/x/sys/windows"
)

var ErrServerIdentity = errors.New("ipc: pipe server runs as another user")

func currentUserSID() (string, error) {
	tok, err := windows.OpenCurrentProcessToken()
	if err != nil {
		return "", fmt.Errorf("open current process token: %w", err)
	}
	defer tok.Close()
	user, err := tok.GetTokenUser()
	if err != nil {
		return "", fmt.Errorf("read token user: %w", err)
	}
	return user.User.Sid.String(), nil
}

func ownerOnlySDDL(userSID string) string {
	return "D:P(A;;GA;;;SY)(A;;GA;;;" + userSID + ")"
}

func listenOwnerOnly(path, userSID string) (net.Listener, error) {
	return winio.ListenPipe(path, &winio.PipeConfig{SecurityDescriptor: ownerOnlySDDL(userSID)})
}

func dialVerified(path, userSID string, timeout time.Duration) (net.Conn, error) {
	conn, err := winio.DialPipe(path, &timeout)
	if err != nil {
		return nil, fmt.Errorf("dial %s: %w", path, err)
	}
	if err := verifyServerUser(conn, userSID); err != nil {
		conn.Close()
		return nil, err
	}
	return conn, nil
}

func verifyServerUser(conn net.Conn, userSID string) error {
	handle, err := connHandle(conn)
	if err != nil {
		return err
	}
	var pid uint32
	if err := windows.GetNamedPipeServerProcessId(handle, &pid); err != nil {
		return fmt.Errorf("server process id: %w", err)
	}
	proc, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return fmt.Errorf("open server process %d: %w", pid, err)
	}
	defer windows.CloseHandle(proc)
	var tok windows.Token
	if err := windows.OpenProcessToken(proc, windows.TOKEN_QUERY, &tok); err != nil {
		return fmt.Errorf("open server process token: %w", err)
	}
	defer tok.Close()
	user, err := tok.GetTokenUser()
	if err != nil {
		return fmt.Errorf("read server token user: %w", err)
	}
	if got := user.User.Sid.String(); got != userSID {
		return fmt.Errorf("%w: got %s, want %s", ErrServerIdentity, got, userSID)
	}
	return nil
}

func connHandle(conn net.Conn) (windows.Handle, error) {
	withFd, ok := conn.(interface{ Fd() uintptr })
	if !ok {
		return 0, fmt.Errorf("pipe connection %T exposes no handle", conn)
	}
	return windows.Handle(withFd.Fd()), nil
}
