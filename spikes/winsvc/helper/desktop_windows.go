package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"runtime"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/kikz0r/edgesync/spikes/winsvc/internal/proto"
)

const (
	uoiName             = 2
	desktopNameChars    = 256
	maxConsecutiveFails = 50
)

var (
	errDesktopPollStuck = errors.New("input desktop could not be opened repeatedly")

	user32                 = windows.NewLazySystemDLL("user32.dll")
	procOpenInputDesktop   = user32.NewProc("OpenInputDesktop")
	procSetThreadDesktop   = user32.NewProc("SetThreadDesktop")
	procCloseDesktop       = user32.NewProc("CloseDesktop")
	procGetUserObjectInfoW = user32.NewProc("GetUserObjectInformationW")
	procSendInput          = user32.NewProc("SendInput")
	procGetCursorPos       = user32.NewProc("GetCursorPos")
	procGetSystemMetrics   = user32.NewProc("GetSystemMetrics")
	user32Procs            = []*windows.LazyProc{
		procOpenInputDesktop, procSetThreadDesktop, procCloseDesktop, procGetUserObjectInfoW,
		procSendInput, procGetCursorPos, procGetSystemMetrics,
	}
)

func resolveUser32() error {
	for _, proc := range user32Procs {
		if err := proc.Find(); err != nil {
			return fmt.Errorf("resolve user32 export: %w", err)
		}
	}
	return nil
}

func openInputDesktop() (uintptr, error) {
	handle, _, err := procOpenInputDesktop.Call(0, 0, windows.GENERIC_ALL)
	if handle == 0 {
		return 0, fmt.Errorf("OpenInputDesktop: %w", err)
	}
	return handle, nil
}

func closeDesktop(handle uintptr) error {
	if r1, _, err := procCloseDesktop.Call(handle); r1 == 0 {
		return fmt.Errorf("CloseDesktop: %w", err)
	}
	return nil
}

func desktopName(handle uintptr) (string, error) {
	var buf [desktopNameChars]uint16
	var needed uint32
	r1, _, err := procGetUserObjectInfoW.Call(
		handle, uoiName, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)*2), uintptr(unsafe.Pointer(&needed)),
	)
	if r1 == 0 {
		return "", fmt.Errorf("GetUserObjectInformationW(UOI_NAME): %w", err)
	}
	return windows.UTF16ToString(buf[:]), nil
}

type desktopFollower struct {
	log      *slog.Logger
	poll     time.Duration
	onSwitch func(name string) error
	act      func() error
	actEvery time.Duration
	once     bool
}

// run owns its OS thread for good: SetThreadDesktop moves the thread to another desktop, so the goroutine never
// unlocks the thread and the runtime discards the thread when run returns.
func (f *desktopFollower) run(ctx context.Context) error {
	runtime.LockOSThread()

	var (
		current       string
		currentHandle uintptr
		lastFailure   string
		failures      int
		nextAct       = time.Now()
	)
	ticker := time.NewTicker(f.poll)
	defer ticker.Stop()

	for {
		handle, err := openInputDesktop()
		if err == nil {
			failures = 0
			lastFailure = ""
			switched, switchErr := f.follow(handle, &current, &currentHandle)
			if switchErr != nil {
				return switchErr
			}
			if switched {
				if err := f.onSwitch(current); err != nil {
					return fmt.Errorf("report desktop %q: %w", current, err)
				}
			}
		} else {
			failures++
			if err.Error() != lastFailure {
				lastFailure = err.Error()
				f.log.Warn("input desktop poll failed", "err", err, "current", current)
			}
			if failures >= maxConsecutiveFails {
				return fmt.Errorf("%w: %d polls: %w", errDesktopPollStuck, failures, err)
			}
		}

		if current != "" && f.act != nil && !time.Now().Before(nextAct) {
			if err := f.act(); err != nil {
				return fmt.Errorf("inject on desktop %q: %w", current, err)
			}
			if f.once {
				return nil
			}
			nextAct = time.Now().Add(f.actEvery)
		}

		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

// follow keeps the handle only when the desktop changed; the previous handle is closed after the thread has moved.
func (f *desktopFollower) follow(handle uintptr, current *string, currentHandle *uintptr) (bool, error) {
	name, err := desktopName(handle)
	if err != nil {
		if closeErr := closeDesktop(handle); closeErr != nil {
			return false, errors.Join(err, closeErr)
		}
		return false, err
	}
	if name == *current {
		return false, closeDesktop(handle)
	}
	if r1, _, setErr := procSetThreadDesktop.Call(handle); r1 == 0 {
		return false, errors.Join(fmt.Errorf("SetThreadDesktop(%q): %w", name, setErr), closeDesktop(handle))
	}
	previous, previousHandle := *current, *currentHandle
	*current, *currentHandle = name, handle
	f.log.Warn("input desktop switch",
		"from", previous,
		"to", name,
		"kind", proto.ClassifyDesktop(name).String(),
		"pollInterval", f.poll.String(),
	)
	if previousHandle != 0 {
		if err := closeDesktop(previousHandle); err != nil {
			return true, err
		}
	}
	return true, nil
}
