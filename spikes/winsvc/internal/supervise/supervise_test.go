package supervise

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"testing/synctest"
	"time"
)

func launch(session uint32) Action { return Action{Kind: ActionLaunch, Session: session} }
func stop(session uint32) Action   { return Action{Kind: ActionStopHelper, Session: session} }
func retry(d time.Duration) Action { return Action{Kind: ActionScheduleRetry, Delay: d} }

func assertActions(t *testing.T, step string, got, want []Action) {
	t.Helper()
	if len(got) == 0 && len(want) == 0 {
		return
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%s: got %+v, want %+v", step, got, want)
	}
}

func TestMachine_StartLaunchesInActiveConsole(t *testing.T) {
	m := NewMachine()
	assertActions(t, "start", m.Handle(Event{Kind: EventStart, ActiveConsole: 1}), []Action{launch(1)})
	if session, active := m.HelperSession(); session != 1 || !active {
		t.Fatalf("helper session %d active %v", session, active)
	}
}

func TestMachine_StartWithoutConsoleDoesNothing(t *testing.T) {
	m := NewMachine()
	assertActions(t, "start", m.Handle(Event{Kind: EventStart, ActiveConsole: NoSession}), nil)
	assertActions(t, "logon", m.Handle(Event{Kind: EventSessionChange, WTS: WTSSessionLogon, ActiveConsole: 2}), []Action{launch(2)})
}

func TestMachine_SessionEventsThatRefreshTarget(t *testing.T) {
	for _, wts := range []uint32{WTSConsoleConnect, WTSSessionLogon, WTSSessionUnlock} {
		m := NewMachine()
		m.Handle(Event{Kind: EventStart, ActiveConsole: NoSession})
		assertActions(t, "refreshing event", m.Handle(Event{Kind: EventSessionChange, WTS: wts, ActiveConsole: 3}), []Action{launch(3)})
	}
}

func TestMachine_OtherSessionEventsAreIgnored(t *testing.T) {
	for _, wts := range []uint32{
		WTSConsoleDisconnect, WTSRemoteConnect, WTSRemoteDisconnect, WTSSessionLogoff, WTSSessionLock, 0x9, 0xa, 0xb,
	} {
		m := NewMachine()
		m.Handle(Event{Kind: EventStart, ActiveConsole: NoSession})
		assertActions(t, "ignored event", m.Handle(Event{Kind: EventSessionChange, WTS: wts, ActiveConsole: 1}), nil)
	}
}

func TestMachine_UnlockWithHelperRunningIsNoop(t *testing.T) {
	m := NewMachine()
	m.Handle(Event{Kind: EventStart, ActiveConsole: 1})
	assertActions(t, "unlock", m.Handle(Event{Kind: EventSessionChange, WTS: WTSSessionUnlock, ActiveConsole: 1}), nil)
}

func TestMachine_ConsoleMovesToAnotherSession(t *testing.T) {
	m := NewMachine()
	m.Handle(Event{Kind: EventStart, ActiveConsole: 1})

	assertActions(t, "console connect",
		m.Handle(Event{Kind: EventSessionChange, WTS: WTSConsoleConnect, ActiveConsole: 2}), []Action{stop(1)})
	assertActions(t, "second event while stopping",
		m.Handle(Event{Kind: EventSessionChange, WTS: WTSSessionUnlock, ActiveConsole: 2}), nil)
	assertActions(t, "helper exits after stop",
		m.Handle(Event{Kind: EventHelperExited, ActiveConsole: 2, Uptime: time.Second}), []Action{launch(2)})
	if session, active := m.HelperSession(); session != 2 || !active {
		t.Fatalf("helper session %d active %v", session, active)
	}
}

func TestMachine_CrashRestartsWithGrowingBackoff(t *testing.T) {
	m := NewMachine()
	m.Handle(Event{Kind: EventStart, ActiveConsole: 1})

	wantDelays := []time.Duration{time.Second, 2 * time.Second, 4 * time.Second}
	for i, delay := range wantDelays {
		assertActions(t, "crash", m.Handle(Event{Kind: EventHelperExited, ActiveConsole: 1, Uptime: time.Second}), []Action{retry(delay)})
		assertActions(t, "retry due", m.Handle(Event{Kind: EventRetryDue, ActiveConsole: 1}), []Action{launch(1)})
		if i == len(wantDelays)-1 {
			break
		}
	}
}

func TestMachine_StableRunResetsBackoff(t *testing.T) {
	m := NewMachine()
	m.Handle(Event{Kind: EventStart, ActiveConsole: 1})
	for range 3 {
		m.Handle(Event{Kind: EventHelperExited, ActiveConsole: 1, Uptime: time.Second})
		m.Handle(Event{Kind: EventRetryDue, ActiveConsole: 1})
	}
	assertActions(t, "stable crash", m.Handle(Event{Kind: EventHelperExited, ActiveConsole: 1, Uptime: StableUptime}), []Action{retry(time.Second)})
}

func TestMachine_LaunchFailureSchedulesRetry(t *testing.T) {
	m := NewMachine()
	assertActions(t, "start", m.Handle(Event{Kind: EventStart, ActiveConsole: 1}), []Action{launch(1)})
	assertActions(t, "launch failed", m.Handle(Event{Kind: EventLaunchFailed, ActiveConsole: 1}), []Action{retry(time.Second)})
	if _, active := m.HelperSession(); active {
		t.Fatal("helper still marked active after launch failure")
	}
	assertActions(t, "retry", m.Handle(Event{Kind: EventRetryDue, ActiveConsole: 1}), []Action{launch(1)})
	assertActions(t, "failed again", m.Handle(Event{Kind: EventLaunchFailed, ActiveConsole: 1}), []Action{retry(2 * time.Second)})
}

func TestMachine_HelperGoneWithoutConsoleWaitsForSessionEvent(t *testing.T) {
	m := NewMachine()
	m.Handle(Event{Kind: EventStart, ActiveConsole: 1})
	assertActions(t, "exit with no console", m.Handle(Event{Kind: EventHelperExited, ActiveConsole: NoSession, Uptime: time.Hour}), nil)
	assertActions(t, "console returns", m.Handle(Event{Kind: EventSessionChange, WTS: WTSConsoleConnect, ActiveConsole: 1}), []Action{launch(1)})
}

func TestMachine_ShutdownStopsHelperAndFreezes(t *testing.T) {
	m := NewMachine()
	m.Handle(Event{Kind: EventStart, ActiveConsole: 1})
	assertActions(t, "shutdown", m.Handle(Event{Kind: EventShutdown}), []Action{stop(1)})
	assertActions(t, "exit after shutdown", m.Handle(Event{Kind: EventHelperExited, ActiveConsole: 1}), nil)
	assertActions(t, "logon after shutdown", m.Handle(Event{Kind: EventSessionChange, WTS: WTSSessionLogon, ActiveConsole: 1}), nil)
	assertActions(t, "retry after shutdown", m.Handle(Event{Kind: EventRetryDue, ActiveConsole: 1}), nil)
}

func TestMachine_ShutdownWithoutHelperIsQuiet(t *testing.T) {
	m := NewMachine()
	assertActions(t, "shutdown", m.Handle(Event{Kind: EventShutdown}), nil)
}

func TestWTSEventName(t *testing.T) {
	if got := WTSEventName(WTSSessionUnlock); got != "WTS_SESSION_UNLOCK" {
		t.Fatalf("got %q", got)
	}
	if got := WTSEventName(0x9); got != "WTS_EVENT_0x9" {
		t.Fatalf("got %q", got)
	}
}

func TestBackoff_DoublesAndCaps(t *testing.T) {
	want := map[int]time.Duration{
		0: 0, 1: time.Second, 2: 2 * time.Second, 3: 4 * time.Second, 4: 8 * time.Second,
		5: 16 * time.Second, 6: BackoffMax, 7: BackoffMax, 100: BackoffMax,
	}
	for failures, delay := range want {
		if got := Backoff(failures); got != delay {
			t.Errorf("Backoff(%d) = %s, want %s", failures, got, delay)
		}
	}
}

func TestNewRunLimit_RejectsOutOfRange(t *testing.T) {
	for _, limit := range []time.Duration{0, -time.Minute, time.Millisecond, MaxRunLimit + time.Second} {
		if _, _, err := NewRunLimit(context.Background(), limit); !errors.Is(err, ErrInvalidMaxRun) {
			t.Errorf("NewRunLimit(%s): got %v", limit, err)
		}
	}
}

func TestRunLimit_StopsExactlyAtLimit(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel, err := NewRunLimit(context.Background(), 30*time.Minute)
		if err != nil {
			t.Fatalf("NewRunLimit: %v", err)
		}
		defer cancel()

		time.Sleep(30*time.Minute - time.Second)
		if ctx.Err() != nil {
			t.Fatalf("limit fired %s early: %v", time.Second, ctx.Err())
		}
		time.Sleep(time.Second)
		<-ctx.Done()
		if !errors.Is(context.Cause(ctx), ErrMaxRunTime) {
			t.Fatalf("cause %v, want ErrMaxRunTime", context.Cause(ctx))
		}
	})
}

func TestRunLimit_ParentCancelKeepsParentCause(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		parent, cancelParent := context.WithCancelCause(context.Background())
		ctx, cancel, err := NewRunLimit(parent, time.Hour)
		if err != nil {
			t.Fatalf("NewRunLimit: %v", err)
		}
		defer cancel()

		stopped := errors.New("service stop requested")
		cancelParent(stopped)
		<-ctx.Done()
		if !errors.Is(context.Cause(ctx), stopped) {
			t.Fatalf("cause %v, want the parent's cause", context.Cause(ctx))
		}
	})
}
