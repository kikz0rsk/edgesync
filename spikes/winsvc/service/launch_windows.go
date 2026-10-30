package main

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/kikz0r/edgesync/spikes/winsvc/internal/proto"
)

const (
	winlogonExe       = "winlogon.exe"
	duplicatedAccess  = windows.TOKEN_QUERY | windows.TOKEN_DUPLICATE | windows.TOKEN_ASSIGN_PRIMARY | windows.TOKEN_ADJUST_DEFAULT | windows.TOKEN_ADJUST_SESSIONID
	sourceTokenAccess = windows.TOKEN_DUPLICATE | windows.TOKEN_QUERY
	launchFlags       = windows.CREATE_UNICODE_ENVIRONMENT | windows.CREATE_NO_WINDOW
	imagePathChars    = 32768
)

var (
	ErrNoWinlogon     = errors.New("winlogon.exe not found in the target session")
	ErrWinlogonImage  = errors.New("winlogon.exe image path is not the system one")
	ErrWinlogonNotSys = errors.New("winlogon.exe token user is not LocalSystem")
	ErrTokenSession   = errors.New("duplicated token is not in the target session")
	ErrUIAccessNotSet = errors.New("TokenUIAccess did not stick after SetTokenInformation")
	ErrHelperReleased = errors.New("helper process handle already released")
)

type launchSpec struct {
	session uint32
	exe     string
	args    []string
	desktop string
}

type launchReport struct {
	winlogonPID  uint32
	tokenUserSID string
	tokenSession uint32
	uiAccess     uint32
}

type helperProcess struct {
	pid     uint32
	session uint32
	exe     string
	started time.Time

	mu     sync.Mutex
	handle windows.Handle
}

func launchHelper(spec launchSpec) (*helperProcess, launchReport, error) {
	var report launchReport

	winlogonPID, err := findWinlogon(spec.session)
	if err != nil {
		return nil, report, err
	}
	report.winlogonPID = winlogonPID

	source, err := openWinlogonToken(winlogonPID)
	if err != nil {
		return nil, report, err
	}
	defer source.Close()

	var primary windows.Token
	if err := windows.DuplicateTokenEx(source, duplicatedAccess, nil, windows.SecurityImpersonation, windows.TokenPrimary, &primary); err != nil {
		return nil, report, fmt.Errorf("DuplicateTokenEx: %w", err)
	}
	defer primary.Close()

	if report.tokenSession, err = tokenUint32(primary, windows.TokenSessionId); err != nil {
		return nil, report, err
	}
	if report.tokenSession != spec.session {
		return nil, report, fmt.Errorf("%w: token session %d, target %d", ErrTokenSession, report.tokenSession, spec.session)
	}
	if report.tokenUserSID, err = tokenUserSID(primary); err != nil {
		return nil, report, err
	}

	uiAccess := uint32(1)
	if err := windows.SetTokenInformation(primary, windows.TokenUIAccess, (*byte)(unsafe.Pointer(&uiAccess)), uint32(unsafe.Sizeof(uiAccess))); err != nil {
		return nil, report, fmt.Errorf("SetTokenInformation(TokenUIAccess): %w", err)
	}
	if report.uiAccess, err = tokenUint32(primary, windows.TokenUIAccess); err != nil {
		return nil, report, err
	}
	if report.uiAccess != 1 {
		return nil, report, fmt.Errorf("%w: reads back %d", ErrUIAccessNotSet, report.uiAccess)
	}

	var env *uint16
	if err := windows.CreateEnvironmentBlock(&env, primary, false); err != nil {
		return nil, report, fmt.Errorf("CreateEnvironmentBlock: %w", err)
	}
	defer windows.DestroyEnvironmentBlock(env)

	appName, err := windows.UTF16PtrFromString(spec.exe)
	if err != nil {
		return nil, report, fmt.Errorf("helper path: %w", err)
	}
	cmdLine, err := windows.UTF16PtrFromString(windows.ComposeCommandLine(append([]string{spec.exe}, spec.args...)))
	if err != nil {
		return nil, report, fmt.Errorf("helper command line: %w", err)
	}
	desktop, err := windows.UTF16PtrFromString(spec.desktop)
	if err != nil {
		return nil, report, fmt.Errorf("desktop path: %w", err)
	}

	startup := windows.StartupInfo{Desktop: desktop}
	startup.Cb = uint32(unsafe.Sizeof(startup))
	var info windows.ProcessInformation
	if err := windows.CreateProcessAsUser(primary, appName, cmdLine, nil, nil, false, launchFlags, env, nil, &startup, &info); err != nil {
		return nil, report, fmt.Errorf("CreateProcessAsUser: %w", err)
	}
	if err := windows.CloseHandle(info.Thread); err != nil {
		return nil, report, fmt.Errorf("CloseHandle(thread): %w", err)
	}

	return &helperProcess{
		pid:     info.ProcessId,
		session: spec.session,
		exe:     spec.exe,
		started: time.Now(),
		handle:  info.Process,
	}, report, nil
}

func findWinlogon(session uint32) (uint32, error) {
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return 0, fmt.Errorf("CreateToolhelp32Snapshot: %w", err)
	}
	defer windows.CloseHandle(snapshot)

	entry := windows.ProcessEntry32{}
	entry.Size = uint32(unsafe.Sizeof(entry))
	err = windows.Process32First(snapshot, &entry)
	for err == nil {
		if strings.EqualFold(windows.UTF16ToString(entry.ExeFile[:]), winlogonExe) {
			var entrySession uint32
			if sessErr := windows.ProcessIdToSessionId(entry.ProcessID, &entrySession); sessErr != nil {
				return 0, fmt.Errorf("ProcessIdToSessionId(%d): %w", entry.ProcessID, sessErr)
			}
			if entrySession == session {
				return entry.ProcessID, nil
			}
		}
		err = windows.Process32Next(snapshot, &entry)
	}
	if !errors.Is(err, windows.ERROR_NO_MORE_FILES) {
		return 0, fmt.Errorf("process snapshot walk: %w", err)
	}
	return 0, fmt.Errorf("%w: session %d", ErrNoWinlogon, session)
}

func openWinlogonToken(pid uint32) (windows.Token, error) {
	process, err := windows.OpenProcess(windows.PROCESS_QUERY_INFORMATION, false, pid)
	if err != nil {
		return 0, fmt.Errorf("OpenProcess(winlogon %d): %w", pid, err)
	}
	defer windows.CloseHandle(process)

	image, err := processImagePath(process)
	if err != nil {
		return 0, err
	}
	systemDir, err := windows.GetSystemDirectory()
	if err != nil {
		return 0, fmt.Errorf("GetSystemDirectory: %w", err)
	}
	if want := filepath.Join(systemDir, winlogonExe); !strings.EqualFold(image, want) {
		return 0, fmt.Errorf("%w: %q, expected %q", ErrWinlogonImage, image, want)
	}

	var token windows.Token
	if err := windows.OpenProcessToken(process, sourceTokenAccess, &token); err != nil {
		return 0, fmt.Errorf("OpenProcessToken(winlogon %d): %w", pid, err)
	}
	user, err := tokenUserSID(token)
	if err != nil {
		token.Close()
		return 0, err
	}
	if user != proto.LocalSystemSID {
		token.Close()
		return 0, fmt.Errorf("%w: %s", ErrWinlogonNotSys, user)
	}
	return token, nil
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

func processImagePath(process windows.Handle) (string, error) {
	buf := make([]uint16, imagePathChars)
	size := uint32(len(buf))
	if err := windows.QueryFullProcessImageName(process, 0, &buf[0], &size); err != nil {
		return "", fmt.Errorf("QueryFullProcessImageName: %w", err)
	}
	return windows.UTF16ToString(buf[:size]), nil
}

// wait blocks until the helper exits and then releases the process handle; only the waiting goroutine closes it.
func (p *helperProcess) wait() (uint32, error) {
	event, err := windows.WaitForSingleObject(p.handle, windows.INFINITE)
	if err != nil {
		return 0, fmt.Errorf("WaitForSingleObject(helper %d): %w", p.pid, err)
	}
	if event != windows.WAIT_OBJECT_0 {
		return 0, fmt.Errorf("WaitForSingleObject(helper %d): unexpected result %#x", p.pid, event)
	}
	var code uint32
	if err := windows.GetExitCodeProcess(p.handle, &code); err != nil {
		return 0, fmt.Errorf("GetExitCodeProcess(helper %d): %w", p.pid, err)
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	handle := p.handle
	p.handle = 0
	if err := windows.CloseHandle(handle); err != nil {
		return code, fmt.Errorf("CloseHandle(helper %d): %w", p.pid, err)
	}
	return code, nil
}

func (p *helperProcess) terminate() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.handle == 0 {
		return ErrHelperReleased
	}
	if err := windows.TerminateProcess(p.handle, 1); err != nil {
		if event, waitErr := windows.WaitForSingleObject(p.handle, 0); waitErr == nil && event == windows.WAIT_OBJECT_0 {
			return nil
		}
		return fmt.Errorf("TerminateProcess(helper %d): %w", p.pid, err)
	}
	return nil
}
