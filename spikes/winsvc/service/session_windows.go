package main

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"sync"
	"time"

	"golang.org/x/sys/windows"

	"github.com/kikz0r/edgesync/spikes/winsvc/internal/proto"
	"github.com/kikz0r/edgesync/spikes/winsvc/internal/supervise"
)

const (
	helperStopWait     = 5 * time.Second
	helperDeadlineSlop = 30 * time.Second
)

type helperConfig struct {
	exe        string
	logPath    string
	desktop    string
	inject     bool
	sas        bool
	servicePID uint32
	deadline   time.Time
}

type exitNotice struct {
	proc *helperProcess
	code uint32
	err  error
}

// supervisor owns the supervise.Machine and is the only goroutine that calls it.
type supervisor struct {
	log      *slog.Logger
	cfg      helperConfig
	machine  *supervise.Machine
	sessions chan uint32
	exited   chan exitNotice
	retry    chan struct{}
	stopped  chan struct{}

	mu      sync.Mutex
	current *helperProcess
}

func newSupervisor(log *slog.Logger, cfg helperConfig) *supervisor {
	return &supervisor{
		log:      log,
		cfg:      cfg,
		machine:  supervise.NewMachine(),
		sessions: make(chan uint32, 16),
		exited:   make(chan exitNotice),
		retry:    make(chan struct{}),
		stopped:  make(chan struct{}),
	}
}

func activeConsole() uint32 {
	return windows.WTSGetActiveConsoleSessionId()
}

func (s *supervisor) notifySession(ctx context.Context, wts uint32) {
	select {
	case s.sessions <- wts:
	case <-ctx.Done():
	}
}

func (s *supervisor) expected() proto.Expected {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.current == nil {
		return proto.Expected{}
	}
	return proto.Expected{PID: s.current.pid, SessionID: s.current.session, ImagePath: s.current.exe}
}

func (s *supervisor) run(ctx context.Context) {
	defer close(s.stopped)
	console := activeConsole()
	s.log.Info("supervisor start", "activeConsoleSession", sessionLabel(console))
	s.dispatch(supervise.Event{Kind: supervise.EventStart, ActiveConsole: console})
	for {
		select {
		case <-ctx.Done():
			s.shutdown()
			return
		case wts := <-s.sessions:
			console := activeConsole()
			s.log.Info("session change", "event", supervise.WTSEventName(wts), "activeConsoleSession", sessionLabel(console))
			s.dispatch(supervise.Event{Kind: supervise.EventSessionChange, WTS: wts, ActiveConsole: console})
		case notice := <-s.exited:
			s.onExit(notice)
		case <-s.retry:
			s.dispatch(supervise.Event{Kind: supervise.EventRetryDue, ActiveConsole: activeConsole()})
		}
	}
}

func (s *supervisor) shutdown() {
	s.dispatch(supervise.Event{Kind: supervise.EventShutdown})
	s.mu.Lock()
	waiting := s.current != nil
	s.mu.Unlock()
	if !waiting {
		return
	}
	select {
	case notice := <-s.exited:
		s.onExit(notice)
	case <-time.After(helperStopWait):
		s.log.Error("helper did not exit after TerminateProcess", "wait", helperStopWait.String())
	}
}

func (s *supervisor) dispatch(event supervise.Event) {
	for _, action := range s.machine.Handle(event) {
		s.log.Info("supervisor action", "action", action.Kind.String(), "session", sessionLabel(action.Session), "delay", action.Delay.String())
		switch action.Kind {
		case supervise.ActionLaunch:
			s.launch(action.Session)
		case supervise.ActionStopHelper:
			s.stopHelper()
		case supervise.ActionScheduleRetry:
			time.AfterFunc(action.Delay, s.signalRetry)
		}
	}
}

func (s *supervisor) signalRetry() {
	select {
	case s.retry <- struct{}{}:
	case <-s.stopped:
	}
}

func (s *supervisor) launch(session uint32) {
	proc, report, err := launchHelper(launchSpec{
		session: session,
		exe:     s.cfg.exe,
		args:    s.helperArgs(),
		desktop: s.cfg.desktop,
	})
	if err != nil {
		s.log.Error("launch helper failed", "session", sessionLabel(session), "err", err)
		s.dispatch(supervise.Event{Kind: supervise.EventLaunchFailed, ActiveConsole: activeConsole()})
		return
	}
	s.mu.Lock()
	s.current = proc
	s.mu.Unlock()
	s.log.Info("helper launched",
		"pid", proc.pid,
		"session", session,
		"desktop", s.cfg.desktop,
		"winlogonPID", report.winlogonPID,
		"tokenUser", report.tokenUserSID,
		"tokenSession", report.tokenSession,
		"tokenUIAccess", report.uiAccess,
	)
	go s.watch(proc)
}

func (s *supervisor) watch(proc *helperProcess) {
	code, err := proc.wait()
	select {
	case s.exited <- exitNotice{proc: proc, code: code, err: err}:
	case <-s.stopped:
	}
}

func (s *supervisor) onExit(notice exitNotice) {
	s.mu.Lock()
	isCurrent := s.current == notice.proc
	if isCurrent {
		s.current = nil
	}
	s.mu.Unlock()
	uptime := time.Since(notice.proc.started)
	if !isCurrent {
		s.log.Warn("exit of a helper that is no longer current", "pid", notice.proc.pid)
		return
	}
	s.log.Warn("helper exited", "pid", notice.proc.pid, "exitCode", notice.code, "uptime", uptime.Round(time.Millisecond).String(), "err", notice.err)
	s.dispatch(supervise.Event{Kind: supervise.EventHelperExited, ActiveConsole: activeConsole(), Uptime: uptime})
}

func (s *supervisor) stopHelper() {
	s.mu.Lock()
	proc := s.current
	s.mu.Unlock()
	if proc == nil {
		s.log.Warn("stop requested but no helper is running")
		return
	}
	if err := proc.terminate(); err != nil {
		s.log.Error("terminate helper", "pid", proc.pid, "err", err)
		return
	}
	s.log.Info("helper terminated by the service", "pid", proc.pid)
}

func (s *supervisor) helperArgs() []string {
	remaining := time.Until(s.cfg.deadline) + helperDeadlineSlop
	args := []string{
		"-service-pid", strconv.FormatUint(uint64(s.cfg.servicePID), 10),
		"-log", s.cfg.logPath,
		"-maxrun", remaining.Round(time.Second).String(),
	}
	if s.cfg.inject {
		args = append(args, "-inject")
	}
	if s.cfg.sas {
		args = append(args, "-sas")
	}
	return args
}

func sessionLabel(session uint32) string {
	if session == supervise.NoSession {
		return "none"
	}
	return fmt.Sprintf("%d", session)
}
