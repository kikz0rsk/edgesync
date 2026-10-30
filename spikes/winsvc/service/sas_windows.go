package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"

	"github.com/kikz0r/edgesync/spikes/winsvc/internal/proto"
)

const (
	pipeBufferSize  = 4096
	ioChunk         = 512
	sasPolicyKey    = `SOFTWARE\Microsoft\Windows\CurrentVersion\Policies\System`
	sasPolicyValue  = "SoftwareSASGeneration"
	waitStopOffset  = 1
	peerQueryAccess = windows.PROCESS_QUERY_LIMITED_INFORMATION
)

var (
	errChannelStopped = errors.New("helper channel stopped")
	errSASUnavailable = errors.New("sas.dll SendSAS is not available")

	sasDLL      = windows.NewLazySystemDLL("sas.dll")
	procSendSAS = sasDLL.NewProc("SendSAS")
)

type sasPolicy struct {
	value   uint32
	present bool
}

func (p sasPolicy) String() string {
	if !p.present {
		return "not set"
	}
	return fmt.Sprintf("%d", p.value)
}

// readSASPolicy is read-only. A missing value is a legitimate state and is reported as present=false.
func readSASPolicy() (sasPolicy, error) {
	key, err := registry.OpenKey(registry.LOCAL_MACHINE, sasPolicyKey, registry.QUERY_VALUE)
	if err != nil {
		return sasPolicy{}, fmt.Errorf("open HKLM\\%s: %w", sasPolicyKey, err)
	}
	defer key.Close()
	value, _, err := key.GetIntegerValue(sasPolicyValue)
	if errors.Is(err, registry.ErrNotExist) {
		return sasPolicy{}, nil
	}
	if err != nil {
		return sasPolicy{}, fmt.Errorf("read %s: %w", sasPolicyValue, err)
	}
	return sasPolicy{value: uint32(value), present: true}, nil
}

func sendSAS() error {
	if err := procSendSAS.Find(); err != nil {
		return fmt.Errorf("%w: %w", errSASUnavailable, err)
	}
	procSendSAS.Call(0)
	return nil
}

// overlappedPipe owns one server-side pipe instance. The OVERLAPPED structure and both transfer buffers live in this
// heap object on purpose: the kernel keeps writing to them after the syscall returns and the Go stack may move.
type overlappedPipe struct {
	handle    windows.Handle
	opEvent   windows.Handle
	stopEvent windows.Handle
	ov        windows.Overlapped
	done      uint32
	readBuf   [ioChunk]byte
	writeBuf  [ioChunk]byte
}

func newOverlappedPipe(name, sddl string, stopEvent windows.Handle) (*overlappedPipe, error) {
	descriptor, err := windows.SecurityDescriptorFromString(sddl)
	if err != nil {
		return nil, fmt.Errorf("pipe security descriptor: %w", err)
	}
	attributes := windows.SecurityAttributes{SecurityDescriptor: descriptor}
	attributes.Length = uint32(unsafe.Sizeof(attributes))

	name16, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return nil, fmt.Errorf("pipe name: %w", err)
	}
	handle, err := windows.CreateNamedPipe(
		name16,
		windows.PIPE_ACCESS_DUPLEX|windows.FILE_FLAG_FIRST_PIPE_INSTANCE|windows.FILE_FLAG_OVERLAPPED,
		windows.PIPE_TYPE_BYTE|windows.PIPE_READMODE_BYTE|windows.PIPE_WAIT|windows.PIPE_REJECT_REMOTE_CLIENTS,
		1, pipeBufferSize, pipeBufferSize, 0, &attributes,
	)
	if err != nil {
		return nil, fmt.Errorf("CreateNamedPipe(%s): %w", name, err)
	}
	opEvent, err := windows.CreateEvent(nil, 1, 0, nil)
	if err != nil {
		windows.CloseHandle(handle)
		return nil, fmt.Errorf("CreateEvent: %w", err)
	}
	return &overlappedPipe{handle: handle, opEvent: opEvent, stopEvent: stopEvent}, nil
}

func (p *overlappedPipe) close() {
	windows.CloseHandle(p.handle)
	windows.CloseHandle(p.opEvent)
}

func (p *overlappedPipe) arm() error {
	if err := windows.ResetEvent(p.opEvent); err != nil {
		return fmt.Errorf("ResetEvent: %w", err)
	}
	p.ov = windows.Overlapped{HEvent: p.opEvent}
	return nil
}

func (p *overlappedPipe) await(startErr error) (uint32, error) {
	if startErr != nil && !errors.Is(startErr, windows.ERROR_IO_PENDING) {
		return 0, startErr
	}
	index, err := windows.WaitForMultipleObjects([]windows.Handle{p.opEvent, p.stopEvent}, false, windows.INFINITE)
	if err != nil {
		return 0, fmt.Errorf("WaitForMultipleObjects: %w", err)
	}
	var transferred uint32
	if index == windows.WAIT_OBJECT_0+waitStopOffset {
		if err := windows.CancelIoEx(p.handle, &p.ov); err != nil && !errors.Is(err, windows.ERROR_NOT_FOUND) {
			return 0, fmt.Errorf("CancelIoEx: %w", err)
		}
		windows.GetOverlappedResult(p.handle, &p.ov, &transferred, true)
		return 0, errChannelStopped
	}
	if err := windows.GetOverlappedResult(p.handle, &p.ov, &transferred, false); err != nil {
		return 0, err
	}
	return transferred, nil
}

func (p *overlappedPipe) connect() error {
	if err := p.arm(); err != nil {
		return err
	}
	err := windows.ConnectNamedPipe(p.handle, &p.ov)
	if errors.Is(err, windows.ERROR_PIPE_CONNECTED) {
		return nil
	}
	if _, err := p.await(err); err != nil {
		return fmt.Errorf("ConnectNamedPipe: %w", err)
	}
	return nil
}

func (p *overlappedPipe) Read(dst []byte) (int, error) {
	if len(dst) == 0 {
		return 0, nil
	}
	if err := p.arm(); err != nil {
		return 0, err
	}
	chunk := min(len(dst), len(p.readBuf))
	n, err := p.await(windows.ReadFile(p.handle, p.readBuf[:chunk], &p.done, &p.ov))
	switch {
	case errors.Is(err, windows.ERROR_BROKEN_PIPE), errors.Is(err, windows.ERROR_PIPE_NOT_CONNECTED):
		return 0, io.EOF
	case err != nil:
		return 0, err
	}
	return copy(dst, p.readBuf[:n]), nil
}

func (p *overlappedPipe) Write(src []byte) (int, error) {
	written := 0
	for written < len(src) {
		if err := p.arm(); err != nil {
			return written, err
		}
		chunk := copy(p.writeBuf[:], src[written:])
		n, err := p.await(windows.WriteFile(p.handle, p.writeBuf[:chunk], &p.done, &p.ov))
		if err != nil {
			return written, err
		}
		written += int(n)
	}
	return written, nil
}

func (p *overlappedPipe) clientPID() (uint32, error) {
	var pid uint32
	if err := windows.GetNamedPipeClientProcessId(p.handle, &pid); err != nil {
		return 0, fmt.Errorf("GetNamedPipeClientProcessId: %w", err)
	}
	return pid, nil
}

func identifyProcess(pid uint32) (proto.Peer, error) {
	process, err := windows.OpenProcess(peerQueryAccess, false, pid)
	if err != nil {
		return proto.Peer{}, fmt.Errorf("OpenProcess(client %d): %w", pid, err)
	}
	defer windows.CloseHandle(process)

	var token windows.Token
	if err := windows.OpenProcessToken(process, windows.TOKEN_QUERY, &token); err != nil {
		return proto.Peer{}, fmt.Errorf("OpenProcessToken(client %d): %w", pid, err)
	}
	defer token.Close()

	sid, err := tokenUserSID(token)
	if err != nil {
		return proto.Peer{}, err
	}
	session, err := tokenUint32(token, windows.TokenSessionId)
	if err != nil {
		return proto.Peer{}, err
	}
	image, err := processImagePath(process)
	if err != nil {
		return proto.Peer{}, err
	}
	return proto.Peer{PID: pid, UserSID: sid, SessionID: session, ImagePath: image}, nil
}

// helperChannel is the only IPC surface of the service. It verifies the connecting process before every message and
// accepts nothing but the whitelist in package proto; no command can run programs, read files or change configuration.
type helperChannel struct {
	log      *slog.Logger
	gate     *proto.Gate
	pipe     *overlappedPipe
	stop     windows.Handle
	expected func() proto.Expected
}

func newHelperChannel(log *slog.Logger, policy proto.Policy, expected func() proto.Expected) (*helperChannel, error) {
	stop, err := windows.CreateEvent(nil, 1, 0, nil)
	if err != nil {
		return nil, fmt.Errorf("CreateEvent(stop): %w", err)
	}
	pipe, err := newOverlappedPipe(proto.PipePath, proto.PipeSDDL, stop)
	if err != nil {
		windows.CloseHandle(stop)
		return nil, err
	}
	return &helperChannel{log: log, gate: proto.NewGate(policy), pipe: pipe, stop: stop, expected: expected}, nil
}

func (c *helperChannel) run(ctx context.Context) error {
	var stopper sync.WaitGroup
	stopper.Add(1)
	go func() {
		defer stopper.Done()
		<-ctx.Done()
		if err := windows.SetEvent(c.stop); err != nil {
			c.log.Error("signal helper channel stop", "err", err)
		}
	}()
	defer func() {
		stopper.Wait()
		c.pipe.close()
		windows.CloseHandle(c.stop)
	}()

	for {
		if err := c.pipe.connect(); err != nil {
			if errors.Is(err, errChannelStopped) {
				return nil
			}
			return err
		}
		c.log.Info("helper pipe client connected")
		err := c.serveConnection()
		c.gate.Disconnect()
		if disconnectErr := windows.DisconnectNamedPipe(c.pipe.handle); disconnectErr != nil {
			return fmt.Errorf("DisconnectNamedPipe: %w", disconnectErr)
		}
		switch {
		case errors.Is(err, errChannelStopped):
			return nil
		case err != nil:
			c.log.Warn("helper pipe connection dropped", "err", err)
		default:
			c.log.Info("helper pipe client disconnected")
		}
	}
}

func (c *helperChannel) authenticate() error {
	pid, err := c.pipe.clientPID()
	if err != nil {
		return err
	}
	peer, err := identifyProcess(pid)
	if err != nil {
		return err
	}
	return proto.AuthorizePeer(peer, c.expected())
}

func (c *helperChannel) serveConnection() error {
	if err := c.authenticate(); err != nil {
		c.log.Error("pipe client rejected at connect", "err", err)
		return nil
	}
	for {
		payload, err := proto.ReadFrame(c.pipe)
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		if err := c.authenticate(); err != nil {
			c.log.Error("pipe message rejected: client not authorised", "err", err)
			return c.respond(proto.Response{Status: proto.StatusDenied, Message: "client not authorised"})
		}
		request, err := proto.ParseRequest(payload)
		if err != nil {
			c.log.Error("pipe message rejected: malformed", "err", err)
			if respondErr := c.respond(proto.Response{Status: proto.StatusBadRequest, Message: "malformed request"}); respondErr != nil {
				return respondErr
			}
			return nil
		}
		if err := c.respond(c.handle(request)); err != nil {
			return err
		}
	}
}

func (c *helperChannel) handle(request proto.Request) proto.Response {
	status := c.gate.Decide(request.Cmd)
	c.log.Info("helper request", "cmd", request.Cmd.String(), "decision", status.String())
	if status != proto.StatusOK {
		return proto.Response{Status: status, Message: "request not granted"}
	}
	switch request.Cmd {
	case proto.CmdReportDesktop:
		c.log.Info("helper input desktop", "name", request.Desktop, "kind", proto.ClassifyDesktop(request.Desktop).String())
	case proto.CmdRequestSAS:
		policy, err := readSASPolicy()
		if err != nil {
			c.log.Error("read SoftwareSASGeneration before SendSAS", "err", err)
			return proto.Response{Status: proto.StatusFailed, Message: "policy read failed"}
		}
		c.log.Warn("calling SendSAS(FALSE) on authenticated helper request", "SoftwareSASGeneration", policy.String())
		if err := sendSAS(); err != nil {
			c.log.Error("SendSAS", "err", err)
			return proto.Response{Status: proto.StatusFailed, Message: "SendSAS unavailable"}
		}
		c.log.Info("SendSAS returned (it has no result; observe the screen)")
	case proto.CmdHello:
		c.log.Info("helper hello accepted")
	}
	return proto.Response{Status: proto.StatusOK}
}

func (c *helperChannel) respond(response proto.Response) error {
	payload, err := response.Marshal()
	if err != nil {
		return fmt.Errorf("marshal response: %w", err)
	}
	return proto.WriteFrame(c.pipe, payload)
}
