//go:build windows

package winpipe

import (
	"errors"
	"fmt"
	"net"

	"golang.org/x/sys/windows"
)

var ErrServerIdentity = errors.New("winpipe: pipe server runs as another account")

// CurrentUserSID returns the SID of the account running this process.
func CurrentUserSID() (string, error) {
	tok, err := windows.OpenCurrentProcessToken()
	if err != nil {
		return "", fmt.Errorf("winpipe: open current token: %w", err)
	}
	defer tok.Close()
	return tokenUserSID(tok)
}

// CurrentSession returns the terminal session of this process.
func CurrentSession() (uint32, error) {
	var session uint32
	if err := windows.ProcessIdToSessionId(windows.GetCurrentProcessId(), &session); err != nil {
		return 0, fmt.Errorf("winpipe: session of current process: %w", err)
	}
	return session, nil
}

// CurrentImage returns the full path of this process executable.
func CurrentImage() (string, error) {
	image, err := processImage(windows.CurrentProcess())
	if err != nil {
		return "", fmt.Errorf("winpipe: image of current process: %w", err)
	}
	return image, nil
}

// VerifyServer is the client-side check: the process that answers on the pipe must run as wantSID.
func VerifyServer(conn net.Conn, wantSID string) error {
	handle, err := connHandle(conn)
	if err != nil {
		return err
	}
	var pid uint32
	if err := windows.GetNamedPipeServerProcessId(handle, &pid); err != nil {
		return fmt.Errorf("winpipe: server process id: %w", err)
	}
	proc, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return fmt.Errorf("winpipe: open server process %d: %w", pid, err)
	}
	defer windows.CloseHandle(proc)
	sid, err := processUserSID(proc)
	if err != nil {
		return err
	}
	if sid != wantSID {
		return fmt.Errorf("%w: got %s, want %s", ErrServerIdentity, sid, wantSID)
	}
	return nil
}

// ClientPeer is the server-side check run before every message. The process handle is opened once and the SID,
// image and session are all read from it.
func ClientPeer(conn net.Conn) (Peer, error) {
	handle, err := connHandle(conn)
	if err != nil {
		return Peer{}, err
	}
	var pid uint32
	if err := windows.GetNamedPipeClientProcessId(handle, &pid); err != nil {
		return Peer{}, fmt.Errorf("winpipe: client process id: %w", err)
	}
	proc, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return Peer{}, fmt.Errorf("winpipe: open client process %d: %w", pid, err)
	}
	defer windows.CloseHandle(proc)
	sid, err := processUserSID(proc)
	if err != nil {
		return Peer{}, err
	}
	image, err := processImage(proc)
	if err != nil {
		return Peer{}, err
	}
	var session uint32
	if err := windows.ProcessIdToSessionId(pid, &session); err != nil {
		return Peer{}, fmt.Errorf("winpipe: session of client process %d: %w", pid, err)
	}
	return Peer{PID: pid, SID: sid, Session: session, Image: image}, nil
}

func processUserSID(proc windows.Handle) (string, error) {
	var tok windows.Token
	if err := windows.OpenProcessToken(proc, windows.TOKEN_QUERY, &tok); err != nil {
		return "", fmt.Errorf("winpipe: open process token: %w", err)
	}
	defer tok.Close()
	return tokenUserSID(tok)
}

func tokenUserSID(tok windows.Token) (string, error) {
	user, err := tok.GetTokenUser()
	if err != nil {
		return "", fmt.Errorf("winpipe: read token user: %w", err)
	}
	return user.User.Sid.String(), nil
}

func processImage(proc windows.Handle) (string, error) {
	buf := make([]uint16, windows.MAX_PATH*4)
	size := uint32(len(buf))
	if err := windows.QueryFullProcessImageName(proc, 0, &buf[0], &size); err != nil {
		return "", fmt.Errorf("winpipe: query image name: %w", err)
	}
	return windows.UTF16ToString(buf[:size]), nil
}

func connHandle(conn net.Conn) (windows.Handle, error) {
	withFd, ok := conn.(interface{ Fd() uintptr })
	if !ok {
		return 0, fmt.Errorf("winpipe: connection %T exposes no pipe handle", conn)
	}
	return windows.Handle(withFd.Fd()), nil
}
