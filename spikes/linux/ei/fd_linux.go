//go:build linux

package ei

import (
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"runtime"
	"sync"

	"golang.org/x/sys/unix"
)

const maxFDsPerRead = 32

var (
	ErrNotUnixSocket    = errors.New("ei: file descriptor is not a unix socket")
	ErrControlTruncated = errors.New("ei: ancillary data truncated, file descriptors were lost")
)

type UnixTransport struct {
	conn *net.UnixConn
	oob  []byte

	mu  sync.Mutex
	fds []*os.File
}

func NewUnixTransport(conn *net.UnixConn) *UnixTransport {
	return &UnixTransport{conn: conn, oob: make([]byte, unix.CmsgSpace(4*maxFDsPerRead))}
}

func UnixTransportFromFile(f *os.File) (*UnixTransport, error) {
	conn, err := net.FileConn(f)
	if err != nil {
		return nil, fmt.Errorf("ei: wrap socket fd: %w", err)
	}
	uc, ok := conn.(*net.UnixConn)
	if !ok {
		return nil, errors.Join(ErrNotUnixSocket, conn.Close())
	}
	if err := f.Close(); err != nil {
		return nil, errors.Join(fmt.Errorf("ei: close original socket fd: %w", err), uc.Close())
	}
	return NewUnixTransport(uc), nil
}

func (t *UnixTransport) Read(p []byte) (int, error) {
	n, oobn, flags, _, err := t.conn.ReadMsgUnix(p, t.oob)
	if oobn > 0 {
		if qerr := t.queueFDs(t.oob[:oobn]); qerr != nil {
			return n, qerr
		}
	}
	if flags&unix.MSG_CTRUNC != 0 {
		return n, ErrControlTruncated
	}
	return n, err
}

func (t *UnixTransport) queueFDs(oob []byte) error {
	msgs, err := unix.ParseSocketControlMessage(oob)
	if err != nil {
		return fmt.Errorf("ei: parse control message: %w", err)
	}
	for i := range msgs {
		if msgs[i].Header.Level != unix.SOL_SOCKET || msgs[i].Header.Type != unix.SCM_RIGHTS {
			continue
		}
		raw, err := unix.ParseUnixRights(&msgs[i])
		if err != nil {
			return fmt.Errorf("ei: parse SCM_RIGHTS: %w", err)
		}
		t.mu.Lock()
		for _, fd := range raw {
			t.fds = append(t.fds, os.NewFile(uintptr(fd), "ei-fd"))
		}
		t.mu.Unlock()
	}
	return nil
}

func (t *UnixTransport) PopFD() (*os.File, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if len(t.fds) == 0 {
		return nil, ErrNoFD
	}
	f := t.fds[0]
	t.fds = t.fds[1:]
	return f, nil
}

func (t *UnixTransport) Write(p []byte) (int, error) {
	return t.conn.Write(p)
}

func (t *UnixTransport) WriteFDs(p []byte, fds []*os.File) error {
	raw := make([]int, len(fds))
	for i, f := range fds {
		raw[i] = int(f.Fd())
	}
	n, _, err := t.conn.WriteMsgUnix(p, unix.UnixRights(raw...), nil)
	runtime.KeepAlive(fds)
	if err != nil {
		return fmt.Errorf("ei: sendmsg: %w", err)
	}
	if n < len(p) {
		return io.ErrShortWrite
	}
	return nil
}

func (t *UnixTransport) Close() error {
	err := t.conn.Close()
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, f := range t.fds {
		err = errors.Join(err, f.Close())
	}
	t.fds = nil
	return err
}
