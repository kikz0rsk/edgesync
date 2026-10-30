// Package supervise holds the pure supervision logic of the EdgeSyncSpike service: the state machine that turns
// session-change events into "launch / stop the helper" actions, the restart backoff and the hard maximum run time.
// The Windows glue feeds it synthetic or real WTS events and executes the returned actions.
package supervise

import (
	"context"
	"errors"
	"fmt"
	"time"
)

const (
	NoSession uint32 = 0xFFFFFFFF

	WTSConsoleConnect    uint32 = 0x1
	WTSConsoleDisconnect uint32 = 0x2
	WTSRemoteConnect     uint32 = 0x3
	WTSRemoteDisconnect  uint32 = 0x4
	WTSSessionLogon      uint32 = 0x5
	WTSSessionLogoff     uint32 = 0x6
	WTSSessionLock       uint32 = 0x7
	WTSSessionUnlock     uint32 = 0x8

	StableUptime = 30 * time.Second
	BackoffBase  = time.Second
	BackoffMax   = 30 * time.Second

	MaxRunLimit = 24 * time.Hour
)

var ErrMaxRunTime = errors.New("supervise: maximum run time reached")

var ErrInvalidMaxRun = errors.New("supervise: maximum run time must be between 1s and 24h")

type EventKind int

const (
	EventStart EventKind = iota
	EventSessionChange
	EventHelperExited
	EventLaunchFailed
	EventRetryDue
	EventShutdown
)

type Event struct {
	Kind EventKind
	// WTS is the SERVICE_CONTROL_SESSIONCHANGE event type; only read for EventSessionChange.
	WTS uint32
	// ActiveConsole is WTSGetActiveConsoleSessionId read by the caller when it received the event.
	ActiveConsole uint32
	// Uptime is how long the helper ran; only read for EventHelperExited.
	Uptime time.Duration
}

type ActionKind int

const (
	ActionLaunch ActionKind = iota
	ActionStopHelper
	ActionScheduleRetry
)

type Action struct {
	Kind    ActionKind
	Session uint32
	Delay   time.Duration
}

type Machine struct {
	want         uint32
	helperActive bool
	helperIn     uint32
	stopping     bool
	failures     int
	shutdown     bool
}

func NewMachine() *Machine {
	return &Machine{want: NoSession, helperIn: NoSession}
}

func Backoff(failures int) time.Duration {
	if failures < 1 {
		return 0
	}
	delay := BackoffBase
	for range failures - 1 {
		delay *= 2
		if delay >= BackoffMax {
			return BackoffMax
		}
	}
	return delay
}

func RefreshesTarget(wts uint32) bool {
	switch wts {
	case WTSConsoleConnect, WTSSessionLogon, WTSSessionUnlock:
		return true
	default:
		return false
	}
}

func (m *Machine) Handle(ev Event) []Action {
	if m.shutdown {
		return nil
	}
	switch ev.Kind {
	case EventStart, EventRetryDue:
		m.want = ev.ActiveConsole
		return m.reconcile()
	case EventSessionChange:
		if !RefreshesTarget(ev.WTS) {
			return nil
		}
		m.want = ev.ActiveConsole
		return m.reconcile()
	case EventHelperExited:
		return m.onHelperGone(ev)
	case EventLaunchFailed:
		m.helperActive = false
		m.helperIn = NoSession
		m.want = ev.ActiveConsole
		m.failures++
		return m.retry()
	case EventShutdown:
		m.shutdown = true
		if m.helperActive {
			return []Action{{Kind: ActionStopHelper, Session: m.helperIn}}
		}
		return nil
	default:
		return nil
	}
}

func (m *Machine) onHelperGone(ev Event) []Action {
	m.helperActive = false
	m.helperIn = NoSession
	m.want = ev.ActiveConsole
	if m.stopping {
		m.stopping = false
		return m.reconcile()
	}
	if ev.Uptime >= StableUptime {
		m.failures = 0
	}
	m.failures++
	return m.retry()
}

func (m *Machine) retry() []Action {
	if m.want == NoSession {
		return nil
	}
	return []Action{{Kind: ActionScheduleRetry, Delay: Backoff(m.failures)}}
}

func (m *Machine) reconcile() []Action {
	switch {
	case m.want == NoSession:
		return nil
	case m.helperActive && m.helperIn != m.want:
		if m.stopping {
			return nil
		}
		m.stopping = true
		return []Action{{Kind: ActionStopHelper, Session: m.helperIn}}
	case m.helperActive:
		return nil
	}
	m.helperActive = true
	m.helperIn = m.want
	return []Action{{Kind: ActionLaunch, Session: m.want}}
}

func (m *Machine) HelperSession() (session uint32, active bool) {
	return m.helperIn, m.helperActive
}

func WTSEventName(wts uint32) string {
	switch wts {
	case WTSConsoleConnect:
		return "WTS_CONSOLE_CONNECT"
	case WTSConsoleDisconnect:
		return "WTS_CONSOLE_DISCONNECT"
	case WTSRemoteConnect:
		return "WTS_REMOTE_CONNECT"
	case WTSRemoteDisconnect:
		return "WTS_REMOTE_DISCONNECT"
	case WTSSessionLogon:
		return "WTS_SESSION_LOGON"
	case WTSSessionLogoff:
		return "WTS_SESSION_LOGOFF"
	case WTSSessionLock:
		return "WTS_SESSION_LOCK"
	case WTSSessionUnlock:
		return "WTS_SESSION_UNLOCK"
	default:
		return fmt.Sprintf("WTS_EVENT_%#x", wts)
	}
}

func (k ActionKind) String() string {
	switch k {
	case ActionLaunch:
		return "launch"
	case ActionStopHelper:
		return "stop-helper"
	case ActionScheduleRetry:
		return "schedule-retry"
	default:
		return fmt.Sprintf("action-%d", int(k))
	}
}

// NewRunLimit returns a context cancelled with ErrMaxRunTime after limit, the service's hard self-stop.
func ValidateMaxRun(limit time.Duration) error {
	if limit < time.Second || limit > MaxRunLimit {
		return fmt.Errorf("%w: got %s", ErrInvalidMaxRun, limit)
	}
	return nil
}

func NewRunLimit(parent context.Context, limit time.Duration) (context.Context, context.CancelFunc, error) {
	if err := ValidateMaxRun(limit); err != nil {
		return nil, nil, err
	}
	ctx, cancel := context.WithTimeoutCause(parent, limit, ErrMaxRunTime)
	return ctx, cancel, nil
}
