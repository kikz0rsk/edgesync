//go:build windows

// Command spikesvc is the EdgeSyncSpike Windows service (LocalSystem). It tracks the active console session, keeps
// one secure helper running there with a duplicated winlogon token carrying TokenUIAccess, restarts it with backoff,
// serves a SYSTEM-only authenticated pipe for the helper and calls SendSAS only for an authorised helper request.
// It stops itself after -maxrun. Installed and removed by the user with install.ps1 and uninstall.ps1 (D-74); agents
// build it and never execute it.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"

	"github.com/kikz0r/edgesync/spikes/winsvc/internal/proto"
	"github.com/kikz0r/edgesync/spikes/winsvc/internal/supervise"
)

const (
	serviceName      = "EdgeSyncSpike"
	defaultMaxRun    = 30 * time.Minute
	defaultMaxSAS    = 3
	logFileName      = "service.log"
	helperLogName    = "helper.log"
	helperExeName    = "spikehelper.exe"
	exitBadConfig    = 2
	exitRuntimeError = 1
)

var (
	errStopRequested = errors.New("service stop requested")
	errNotAService   = errors.New("spikesvc must be started by the Service Control Manager; use install.ps1")
	errHelperOutside = errors.New("helper executable must live inside the data directory")
	errRelativePath  = errors.New("-dir and -helper must be absolute paths")
)

type options struct {
	dir           string
	helper        string
	maxRun        time.Duration
	launchDesktop string
	helperInject  bool
	allowSAS      bool
	maxSAS        int
}

func main() {
	os.Exit(run(os.Args[1:], os.Stderr))
}

func run(args []string, stderr io.Writer) int {
	opts, err := parseOptions(args)
	if err != nil {
		fmt.Fprintln(stderr, "spikesvc:", err)
		return exitBadConfig
	}
	isService, err := svc.IsWindowsService()
	if err != nil {
		fmt.Fprintln(stderr, "spikesvc:", err)
		return exitRuntimeError
	}
	if !isService {
		fmt.Fprintln(stderr, errNotAService)
		return exitBadConfig
	}
	logFile, err := os.OpenFile(filepath.Join(opts.dir, logFileName), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		fmt.Fprintln(stderr, "spikesvc: open log:", err)
		return exitRuntimeError
	}
	defer logFile.Close()
	log := slog.New(slog.NewTextHandler(logFile, &slog.HandlerOptions{Level: slog.LevelDebug}))

	if err := svc.Run(serviceName, &handler{opts: opts, log: log}); err != nil {
		log.Error("svc.Run", "err", err)
		return exitRuntimeError
	}
	return 0
}

func parseOptions(args []string) (options, error) {
	programData, err := windows.KnownFolderPath(windows.FOLDERID_ProgramData, 0)
	if err != nil {
		return options{}, fmt.Errorf("resolve ProgramData: %w", err)
	}
	defaultDir := filepath.Join(programData, "EdgeSyncSpike")

	var opts options
	flags := flag.NewFlagSet("spikesvc", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.StringVar(&opts.dir, "dir", defaultDir, "data directory holding logs and the helper binary")
	flags.StringVar(&opts.helper, "helper", "", "helper executable (default <dir>\\bin\\"+helperExeName+")")
	flags.DurationVar(&opts.maxRun, "maxrun", defaultMaxRun, "hard self-stop after this run time")
	flags.StringVar(&opts.launchDesktop, "desktop", "winlogon", "lpDesktop of the helper: winlogon or default")
	flags.BoolVar(&opts.helperInject, "helper-inject", false, "helper injects the demo square and text every interval")
	flags.BoolVar(&opts.allowSAS, "allow-sas", false, "honour authenticated RequestSAS and let the helper request it once")
	flags.IntVar(&opts.maxSAS, "max-sas", defaultMaxSAS, "SendSAS calls allowed per service run")
	if err := flags.Parse(args); err != nil {
		return options{}, fmt.Errorf("flags: %w", err)
	}
	if flags.NArg() != 0 {
		return options{}, fmt.Errorf("unexpected arguments: %v", flags.Args())
	}
	if _, err := proto.LaunchDesktop(opts.launchDesktop); err != nil {
		return options{}, err
	}
	if err := supervise.ValidateMaxRun(opts.maxRun); err != nil {
		return options{}, err
	}
	opts.dir = filepath.Clean(opts.dir)
	if opts.helper == "" {
		opts.helper = filepath.Join(opts.dir, "bin", helperExeName)
	}
	opts.helper = filepath.Clean(opts.helper)
	if !filepath.IsAbs(opts.helper) || !filepath.IsAbs(opts.dir) {
		return options{}, errRelativePath
	}
	if !isInside(opts.dir, opts.helper) {
		return options{}, fmt.Errorf("%w: %s not under %s", errHelperOutside, opts.helper, opts.dir)
	}
	return opts, nil
}

func isInside(dir, path string) bool {
	rel, err := filepath.Rel(dir, path)
	return err == nil && rel != "." && !strings.HasPrefix(rel, "..")
}

type handler struct {
	opts options
	log  *slog.Logger
}

func (h *handler) Execute(_ []string, requests <-chan svc.ChangeRequest, status chan<- svc.Status) (bool, uint32) {
	status <- svc.Status{State: svc.StartPending}
	opts := h.opts

	stopCtx, stop := context.WithCancelCause(context.Background())
	defer stop(nil)
	ctx, cancel, err := supervise.NewRunLimit(stopCtx, opts.maxRun)
	if err != nil {
		h.log.Error("run limit", "err", err)
		return true, exitRuntimeError
	}
	defer cancel()
	deadline := time.Now().Add(opts.maxRun)

	policy, err := readSASPolicy()
	if err != nil {
		h.log.Error("read SoftwareSASGeneration", "err", err)
		return true, exitRuntimeError
	}
	h.log.Warn("service starting",
		"pid", os.Getpid(),
		"maxRun", opts.maxRun.String(),
		"selfStopAt", deadline.Format(time.RFC3339),
		"dir", opts.dir,
		"helper", opts.helper,
		"launchDesktop", opts.launchDesktop,
		"helperInject", opts.helperInject,
		"allowSAS", opts.allowSAS,
		"SoftwareSASGeneration", policy.String(),
	)

	desktop, err := proto.LaunchDesktop(opts.launchDesktop)
	if err != nil {
		h.log.Error("launch desktop", "err", err)
		return true, exitBadConfig
	}
	sup := newSupervisor(h.log, helperConfig{
		exe:        opts.helper,
		logPath:    filepath.Join(opts.dir, helperLogName),
		desktop:    desktop,
		inject:     opts.helperInject,
		sas:        opts.allowSAS,
		servicePID: uint32(os.Getpid()),
		deadline:   deadline,
	})
	channel, err := newHelperChannel(h.log, proto.Policy{AllowSAS: opts.allowSAS, MaxSAS: opts.maxSAS}, sup.expected)
	if err != nil {
		h.log.Error("create helper channel", "err", err)
		return true, exitRuntimeError
	}

	failed := make(chan error, 1)
	finished := make(chan struct{}, 2)
	go func() {
		defer func() { finished <- struct{}{} }()
		if err := channel.run(ctx); err != nil {
			failed <- fmt.Errorf("helper channel: %w", err)
		}
	}()
	go func() {
		defer func() { finished <- struct{}{} }()
		sup.run(ctx)
	}()

	status <- svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown | svc.AcceptSessionChange}
	h.log.Info("service running")

	exitCode := uint32(0)
loop:
	for {
		select {
		case request := <-requests:
			switch request.Cmd {
			case svc.Interrogate:
				status <- request.CurrentStatus
			case svc.Stop, svc.Shutdown:
				h.log.Info("stop requested by the Service Control Manager")
				stop(errStopRequested)
				break loop
			case svc.SessionChange:
				// EventData is deliberately not dereferenced: x/sys hands it over after the control handler
				// returned, so the notification buffer may already be gone. The target session is read fresh
				// from WTSGetActiveConsoleSessionId instead.
				sup.notifySession(ctx, request.EventType)
			default:
				h.log.Warn("unexpected service control", "cmd", uint32(request.Cmd))
			}
		case err := <-failed:
			h.log.Error("fatal", "err", err)
			exitCode = exitRuntimeError
			stop(err)
			break loop
		case <-ctx.Done():
			break loop
		}
	}

	status <- svc.Status{State: svc.StopPending}
	cause := context.Cause(ctx)
	if errors.Is(cause, supervise.ErrMaxRunTime) {
		h.log.Warn("hard self-stop: maximum run time reached", "maxRun", opts.maxRun.String())
	}
	stop(errStopRequested)
	<-finished
	<-finished
	h.log.Warn("service stopped", "cause", fmt.Sprint(cause))
	return false, exitCode
}
