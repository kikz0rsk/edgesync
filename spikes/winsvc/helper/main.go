//go:build windows

// Command spikehelper is the EdgeSyncSpike secure helper. The service starts it in the active console session with a
// duplicated winlogon token carrying TokenUIAccess. It follows the input desktop (Default, Winlogon, Screen-saver),
// reports every switch to the service over the SYSTEM-only pipe, optionally injects a pointer square and text on the
// current input desktop and, only when started with -sas, asks the service once for a Secure Attention Sequence.
// The helper has no listening surface: it never accepts commands from anyone. Started by the service only (D-74);
// agents build it and never execute it.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/kikz0r/edgesync/spikes/winsvc/internal/proto"
	"github.com/kikz0r/edgesync/spikes/winsvc/internal/supervise"
)

const (
	defaultMaxRun         = 35 * time.Minute
	defaultPoll           = 200 * time.Millisecond
	defaultInjectInterval = 10 * time.Second
	defaultSASDelay       = 15 * time.Second
	pipeBusyAttempts      = 10
	pipeBusyDelay         = 200 * time.Millisecond
	exitFailure           = 1
	exitBadFlags          = 2
	identityQueryAccess   = windows.PROCESS_QUERY_LIMITED_INFORMATION
)

var (
	errServiceGone    = errors.New("service process exited")
	errServiceRefused = errors.New("service refused the request")
	errNotService     = errors.New("pipe server is not the service process")
	errServerNotSys   = errors.New("pipe server does not run as LocalSystem")
	errPollInterval   = errors.New("-poll must be between 10ms and 5s")
)

type options struct {
	servicePID     uint32
	logPath        string
	maxRun         time.Duration
	poll           time.Duration
	inject         bool
	injectInterval time.Duration
	once           bool
	sas            bool
	sasDelay       time.Duration
}

func main() {
	os.Exit(run(os.Args[1:], os.Stderr))
}

func parseOptions(args []string) (options, error) {
	var opts options
	var servicePID uint64
	flags := flag.NewFlagSet("spikehelper", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.Uint64Var(&servicePID, "service-pid", 0, "process id of the service that launched this helper (required)")
	flags.StringVar(&opts.logPath, "log", "", "log file (required)")
	flags.DurationVar(&opts.maxRun, "maxrun", defaultMaxRun, "hard self-stop after this run time")
	flags.DurationVar(&opts.poll, "poll", defaultPoll, "input desktop poll interval")
	flags.BoolVar(&opts.inject, "inject", false, "inject the pointer square, text and Escape every -inject-interval")
	flags.DurationVar(&opts.injectInterval, "inject-interval", defaultInjectInterval, "time between injections")
	flags.BoolVar(&opts.once, "once", false, "with -inject: inject once on the first input desktop and exit")
	flags.BoolVar(&opts.sas, "sas", false, "ask the service once for SendSAS")
	flags.DurationVar(&opts.sasDelay, "sas-delay", defaultSASDelay, "delay before the SAS request")
	if err := flags.Parse(args); err != nil {
		return options{}, fmt.Errorf("flags: %w", err)
	}
	if flags.NArg() != 0 {
		return options{}, fmt.Errorf("unexpected arguments: %v", flags.Args())
	}
	if servicePID == 0 || servicePID > uint64(^uint32(0)) {
		return options{}, errors.New("-service-pid is required")
	}
	opts.servicePID = uint32(servicePID)
	if opts.logPath == "" {
		return options{}, errors.New("-log is required")
	}
	if err := supervise.ValidateMaxRun(opts.maxRun); err != nil {
		return options{}, err
	}
	if opts.poll < 10*time.Millisecond || opts.poll > 5*time.Second {
		return options{}, errPollInterval
	}
	if opts.inject && opts.injectInterval < time.Second {
		return options{}, errors.New("-inject-interval must be at least 1s")
	}
	if opts.once && !opts.inject {
		return options{}, errors.New("-once requires -inject")
	}
	return opts, nil
}

func run(args []string, stderr io.Writer) int {
	opts, err := parseOptions(args)
	if err != nil {
		fmt.Fprintln(stderr, "spikehelper:", err)
		return exitBadFlags
	}
	logFile, err := os.OpenFile(opts.logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		fmt.Fprintln(stderr, "spikehelper: open log:", err)
		return exitFailure
	}
	defer logFile.Close()
	log := slog.New(slog.NewTextHandler(logFile, &slog.HandlerOptions{Level: slog.LevelDebug}))

	if err := execute(opts, log); err != nil {
		log.Error("helper failed", "err", err)
		return exitFailure
	}
	return 0
}

func execute(opts options, log *slog.Logger) error {
	if err := verifyInputLayout(); err != nil {
		return err
	}
	if err := resolveUser32(); err != nil {
		return err
	}
	if err := logIdentity(log, opts); err != nil {
		return err
	}

	stopCtx, stop := context.WithCancelCause(context.Background())
	defer stop(nil)
	ctx, cancel, err := supervise.NewRunLimit(stopCtx, opts.maxRun)
	if err != nil {
		return err
	}
	defer cancel()

	if err := watchService(opts.servicePID, stop); err != nil {
		return err
	}
	client, err := connectService(ctx, opts.servicePID)
	if err != nil {
		return err
	}
	defer client.close()

	if err := client.expectOK(proto.Request{Cmd: proto.CmdHello, Version: proto.Version}); err != nil {
		return fmt.Errorf("hello: %w", err)
	}
	log.Info("service accepted the helper")

	var sasDone sync.WaitGroup
	if opts.sas {
		sasDone.Add(1)
		go func() {
			defer sasDone.Done()
			requestSAS(ctx, log, client, opts.sasDelay)
		}()
	}

	var currentDesktop string
	follower := desktopFollower{
		log:  log,
		poll: opts.poll,
		onSwitch: func(name string) error {
			currentDesktop = name
			return client.expectOK(proto.Request{Cmd: proto.CmdReportDesktop, Desktop: name})
		},
		once: opts.once,
	}
	if opts.inject {
		follower.actEvery = opts.injectInterval
		follower.act = func() error { return injectDemo(log, currentDesktop) }
	}

	followErr := follower.run(ctx)
	if opts.once && opts.sas {
		sasDone.Wait()
	}
	if cause := context.Cause(ctx); cause != nil {
		log.Warn("helper stopping", "cause", cause.Error(), "maxRun", opts.maxRun.String())
	}
	cancel()
	sasDone.Wait()
	return followErr
}

func logIdentity(log *slog.Logger, opts options) error {
	token := windows.GetCurrentProcessToken()
	user, err := tokenUserSID(token)
	if err != nil {
		return err
	}
	session, err := tokenUint32(token, windows.TokenSessionId)
	if err != nil {
		return err
	}
	uiAccess, err := tokenUint32(token, windows.TokenUIAccess)
	if err != nil {
		return err
	}
	log.Warn("helper starting",
		"pid", os.Getpid(),
		"servicePID", opts.servicePID,
		"tokenUser", user,
		"tokenSession", session,
		"tokenUIAccess", uiAccess,
		"maxRun", opts.maxRun.String(),
		"inject", opts.inject,
		"sas", opts.sas,
		"once", opts.once,
	)
	return nil
}

func tokenUint32(token windows.Token, class uint32) (uint32, error) {
	var value, returned uint32
	if err := windows.GetTokenInformation(token, class, (*byte)(unsafe.Pointer(&value)), uint32(unsafe.Sizeof(value)), &returned); err != nil {
		return 0, fmt.Errorf("GetTokenInformation(class %d): %w", class, err)
	}
	return value, nil
}

func tokenUserSID(token windows.Token) (string, error) {
	user, err := token.GetTokenUser()
	if err != nil {
		return "", fmt.Errorf("GetTokenUser: %w", err)
	}
	return user.User.Sid.String(), nil
}

// watchService ends the helper when the service process goes away, so an orphaned helper cannot outlive it.
func watchService(servicePID uint32, stop context.CancelCauseFunc) error {
	process, err := windows.OpenProcess(windows.SYNCHRONIZE, false, servicePID)
	if err != nil {
		return fmt.Errorf("OpenProcess(service %d): %w", servicePID, err)
	}
	go func() {
		defer windows.CloseHandle(process)
		event, err := windows.WaitForSingleObject(process, windows.INFINITE)
		switch {
		case err != nil:
			stop(fmt.Errorf("wait for service process: %w", err))
		case event != windows.WAIT_OBJECT_0:
			stop(fmt.Errorf("wait for service process: unexpected result %#x", event))
		default:
			stop(errServiceGone)
		}
	}()
	return nil
}

func requestSAS(ctx context.Context, log *slog.Logger, client *serviceClient, delay time.Duration) {
	log.Warn("SAS requested: waiting before asking the service", "delay", delay.String())
	select {
	case <-time.After(delay):
	case <-ctx.Done():
		return
	}
	response, err := client.call(proto.Request{Cmd: proto.CmdRequestSAS})
	if err != nil {
		log.Error("SAS request failed", "err", err)
		return
	}
	log.Warn("SAS request answered", "status", response.Status.String(), "message", response.Message)
}

type pipeFile struct{ handle windows.Handle }

func (p pipeFile) Read(dst []byte) (int, error) {
	if len(dst) == 0 {
		return 0, nil
	}
	var n uint32
	if err := windows.ReadFile(p.handle, dst, &n, nil); err != nil {
		if errors.Is(err, windows.ERROR_BROKEN_PIPE) {
			return int(n), io.EOF
		}
		return int(n), err
	}
	return int(n), nil
}

func (p pipeFile) Write(src []byte) (int, error) {
	var n uint32
	if err := windows.WriteFile(p.handle, src, &n, nil); err != nil {
		return int(n), err
	}
	return int(n), nil
}

type serviceClient struct {
	mu   sync.Mutex
	pipe pipeFile
}

// connectService opens the pipe with SECURITY_ANONYMOUS so a rogue server could not impersonate this SYSTEM process,
// then verifies that the server is the very service process that launched the helper.
func connectService(ctx context.Context, servicePID uint32) (*serviceClient, error) {
	name, err := windows.UTF16PtrFromString(proto.PipePath)
	if err != nil {
		return nil, fmt.Errorf("pipe name: %w", err)
	}
	var handle windows.Handle
	for attempt := 1; ; attempt++ {
		handle, err = windows.CreateFile(name, windows.GENERIC_READ|windows.GENERIC_WRITE, 0, nil, windows.OPEN_EXISTING,
			windows.SECURITY_SQOS_PRESENT|windows.SECURITY_ANONYMOUS, 0)
		if err == nil {
			break
		}
		if !errors.Is(err, windows.ERROR_PIPE_BUSY) || attempt >= pipeBusyAttempts {
			return nil, fmt.Errorf("open %s: %w", proto.PipePath, err)
		}
		select {
		case <-time.After(pipeBusyDelay):
		case <-ctx.Done():
			return nil, fmt.Errorf("open %s: %w", proto.PipePath, context.Cause(ctx))
		}
	}
	if err := verifyServer(handle, servicePID); err != nil {
		windows.CloseHandle(handle)
		return nil, err
	}
	return &serviceClient{pipe: pipeFile{handle: handle}}, nil
}

func verifyServer(handle windows.Handle, servicePID uint32) error {
	var serverPID uint32
	if err := windows.GetNamedPipeServerProcessId(handle, &serverPID); err != nil {
		return fmt.Errorf("GetNamedPipeServerProcessId: %w", err)
	}
	if serverPID != servicePID {
		return fmt.Errorf("%w: pid %d, expected %d", errNotService, serverPID, servicePID)
	}
	process, err := windows.OpenProcess(identityQueryAccess, false, serverPID)
	if err != nil {
		return fmt.Errorf("OpenProcess(server %d): %w", serverPID, err)
	}
	defer windows.CloseHandle(process)
	var token windows.Token
	if err := windows.OpenProcessToken(process, windows.TOKEN_QUERY, &token); err != nil {
		return fmt.Errorf("OpenProcessToken(server %d): %w", serverPID, err)
	}
	defer token.Close()
	user, err := tokenUserSID(token)
	if err != nil {
		return err
	}
	if user != proto.LocalSystemSID {
		return fmt.Errorf("%w: %s", errServerNotSys, user)
	}
	return nil
}

func (c *serviceClient) close() {
	windows.CloseHandle(c.pipe.handle)
}

func (c *serviceClient) call(request proto.Request) (proto.Response, error) {
	payload, err := request.Marshal()
	if err != nil {
		return proto.Response{}, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := proto.WriteFrame(c.pipe, payload); err != nil {
		return proto.Response{}, err
	}
	reply, err := proto.ReadFrame(c.pipe)
	if err != nil {
		return proto.Response{}, err
	}
	return proto.ParseResponse(reply)
}

func (c *serviceClient) expectOK(request proto.Request) error {
	response, err := c.call(request)
	if err != nil {
		return err
	}
	if response.Status != proto.StatusOK {
		return fmt.Errorf("%w: %s %s (%s)", errServiceRefused, request.Cmd, response.Status, response.Message)
	}
	return nil
}
