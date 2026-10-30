# winsvc - SYSTEM service, secure helper, desktop following, UIPI and SendSAS spike (P0-SP1-004)

Purpose: prove or disprove D-63 / D-47 / D-07 on a real Windows PC. A LocalSystem service tracks the active console
session and keeps one helper alive there, started with a duplicate of that session's `winlogon.exe` token plus
`TokenUIAccess`. The helper follows the input desktop (Default, Winlogon, Screen-saver), can inject a pointer square and
typed text on it, and can ask the service (over an authenticated pipe) to call `SendSAS`.

Spike rules (D-74): agents build this code and never run it. YOU run it, on your own PC, as administrator. Read the
whole WARNING section first.

## WARNING - what this installs and what it can do

- It installs a Windows service named `EdgeSyncSpike` that runs as **LocalSystem** and launches a helper process that also
  runs as SYSTEM with UIAccess. That is a privileged component on purpose; treat the machine as a test machine.
- With `-Inject`, every 10 seconds the helper moves the pointer in a 200 px square and types `edgesync` followed by Escape
  into whatever has focus on the input desktop. Do not leave a document, a terminal or a password prompt focused.
- With `-EnableSasPolicy` (opt-in) the script changes the machine policy `SoftwareSASGeneration` and the helper asks for one
  Ctrl+Alt+Del screen about 15 seconds after it starts. See "SendSAS policy" below.
- Everything is temporary: the service stops itself after `-MaxRunMinutes` (default 30) and `uninstall.ps1` removes it.
- All logs go to `%ProgramData%\EdgeSyncSpike\` (`service.log`, `helper.log`). Nothing else on disk is written. The registry
  is touched only by `install.ps1 -EnableSasPolicy` (one value, backed up first) and by the SCM when the service is created.

## Recovery if something goes wrong (type these in an ELEVATED PowerShell or cmd)

```
sc.exe stop EdgeSyncSpike
sc.exe delete EdgeSyncSpike
taskkill /F /IM spikehelper.exe
taskkill /F /IM spikesvc.exe
```

Policy: if you used `-EnableSasPolicy`, the previous value is in `%ProgramData%\EdgeSyncSpike\sas-policy-backup.json`
(`existed`, `previous`, `set`, `changed`). `uninstall.ps1` restores it. By hand:

```
reg query "HKLM\SOFTWARE\Microsoft\Windows\CurrentVersion\Policies\System" /v SoftwareSASGeneration
reg add "HKLM\SOFTWARE\Microsoft\Windows\CurrentVersion\Policies\System" /v SoftwareSASGeneration /t REG_DWORD /d <previous> /f
reg delete "HKLM\SOFTWARE\Microsoft\Windows\CurrentVersion\Policies\System" /v SoftwareSASGeneration /f
```

Use the `reg add` line when `existed` is true, the `reg delete` line when it was false. If the machine stays at a blank sign-in
screen, the service stops itself within `-MaxRunMinutes`; or boot to Safe Mode (the service is not started there) and run the
`sc.exe delete` line.

## Safety design

- Service: LocalSystem, start type Manual by default, hard self-stop after `-maxrun` (1 s to 24 h, default 30 min, install
  script 1 to 240 min). On stop it terminates the helper. The helper also stops itself after the remaining time + 30 s and
  when the service process exits, so it cannot be orphaned.
- Directory `%ProgramData%\EdgeSyncSpike\` is locked by `install.ps1` to SYSTEM and Administrators (full control, no
  inheritance), because the helper binary runs as SYSTEM from `bin\`. The service refuses a `-helper` outside `-dir`.
- Helper-to-service channel (D-47, D-63; Deskflow CVE-2026-41477 lesson):
  - `\\.\pipe\EdgeSyncSpike` is created by the service with `FILE_FLAG_FIRST_PIPE_INSTANCE`, one instance, remote clients
    rejected, and an explicit security descriptor `O:SYD:P(A;;GA;;;SY)`: protected DACL, one ACE, SYSTEM only (asserted by
    `proto/sddl_windows_test.go`). There is no default DACL.
  - Before every message the service takes `GetNamedPipeClientProcessId`, opens the process and requires: token user is
    LocalSystem (S-1-5-18), PID equals the helper the service launched, token session equals the session it was launched into,
    image path equals the helper executable. Anything else gets `denied` and the connection is closed.
  - Only three whitelisted commands exist: `Hello`, `ReportDesktop` (name is logged, never used), `RequestSAS`. Nothing on the
    pipe runs programs, reads files or changes configuration. Frames are length-prefixed, at most 256 bytes, strictly decoded.
  - `SendSAS` is called only by the service, only after `Hello`, only when started with `-allow-sas` (otherwise `refused`), and
    at most `-max-sas` (default 3) times per service run.
  - The helper opens the pipe with `SECURITY_ANONYMOUS`, verifies that the server PID is the service that launched it and that
    the server runs as LocalSystem. The helper has no listening surface and accepts no commands from anyone.
- Token handling: winlogon is located by name and session in a process snapshot, its image path must be
  `%SystemRoot%\System32\winlogon.exe` and its token user must be SYSTEM; the token is duplicated as a primary token, its
  session id is checked against the target session, `TokenUIAccess` is set and read back.

## Build (does not run anything)

From the repository root:

```
go -C spikes build -o bin/spikesvc.exe ./winsvc/service
go -C spikes build -o bin/spikehelper.exe ./winsvc/helper
```

Both are `CGO_ENABLED=0` windows/amd64 binaries in `spikes\bin\` (gitignored).

## Install, run, observe

Open an elevated 64-bit PowerShell in `spikes\winsvc\`. If scripts are blocked: `powershell -ExecutionPolicy Bypass -File .\install.ps1 ...`.

```
.\install.ps1 -Inject                       # steps 1 to 5
.\install.ps1 -Inject -EnableSasPolicy      # adds step 6 (SendSAS)
.\install.ps1 -Inject -AutoStart            # adds step 7 (reboot with nobody logged on)
```

Watch the logs live: `Get-Content $env:ProgramData\EdgeSyncSpike\service.log -Wait` and the same for `helper.log`.

| install.ps1 parameter | Meaning |
|---|---|
| `-SourceDir` | folder with `spikesvc.exe` and `spikehelper.exe` (default `spikes\bin`) |
| `-MaxRunMinutes` | hard self-stop, 1 to 240, default 30 |
| `-Inject` | helper injects square, `edgesync`, Escape every 10 s |
| `-EnableSasPolicy` | OPT-IN: set `SoftwareSASGeneration`, service honours `RequestSAS`, helper requests one SAS |
| `-AutoStart` | start type Automatic (needed for the reboot test) |
| `-LaunchDesktop` | `winlogon` (default) or `default`: lpDesktop of the helper at creation |
| `-NoStart` | install only |

What to observe, in order:

1. Launch and token: `service.log` has `helper launched` with `tokenUser=S-1-5-18`, `tokenSession` equal to the console
   session, `tokenUIAccess=1`; `helper.log` has `helper starting` with the same three values read from inside the helper.
   Try `-LaunchDesktop default` once and note whether anything differs from `winlogon`.
2. Desktop following: with the helper running, press Win+L, unlock, trigger a UAC prompt (for example run `cmd` as administrator),
   and let a screen saver start (set a 1 minute screen saver in the personalisation settings if you want that case). `helper.log`
   shows `input desktop switch from=... to=Winlogon|Default|Screen-saver`; the poll interval is 200 ms, so each switch is logged within
   200 ms of the real switch (compare the log timestamp with when you pressed the key). `service.log` repeats each switch as
   `helper input desktop`.
3. Injection (needs `-Inject`): open an elevated Task Manager and give it focus: the pointer draws a square and `edgesync` is typed
   into it (search box or none; the Escape closes dialogs). On a UAC prompt: the pointer moves and Escape cancels the prompt.
   On the lock screen (Win+L): the pointer moves and the password field shows `edgesync` typed into it. `helper.log` line
   `injected demo square, text and Escape` has the start point and the `GetCursorPos` read-back. A `SendInput injected fewer events`
   error means the desktop or UIPI blocked the input; record the desktop name and the error.
4. Restart: `taskkill /F /IM spikehelper.exe`; `service.log` shows `helper exited` and, after the backoff (1 s, then 2 s, 4 s ...,
   capped at 30 s; reset after the helper ran 30 s), `helper launched` with a new pid.
5. Session changes: lock/unlock, sign out and in, fast user switching. `service.log` shows `session change event=WTS_...` and which
   session is active; a helper in the wrong session is terminated and a new one launched in the active console session.
6. SendSAS (needs `-EnableSasPolicy`): about 15 s after the helper starts, `service.log` shows `calling SendSAS(FALSE) ...
   SoftwareSASGeneration=<value>` and the Ctrl+Alt+Del screen opens on the console (press Escape to leave it). Also run once
   WITHOUT `-EnableSasPolicy` (policy value read and logged at service start, request `refused`) and, if the policy is `not set`
   on your machine, note whether SendSAS still works with the value absent. `SendSAS` has no return value: the evidence is the screen.
7. Pre-login: install with `-AutoStart -Inject`, reboot, do NOT sign in. Within 30 minutes of boot the pointer at the sign-in screen
   moves in a square. After signing in, `helper.log` of the boot shows `input desktop switch` entries starting with `Winlogon`.
   If the helper did not start at boot, `service.log` shows `launch helper failed ... winlogon.exe not found` and the retries.
8. Self-stop: leave it running; at the time shown in `service.log` (`selfStopAt`) it logs `hard self-stop` and stops.

## Uninstall

```
.\uninstall.ps1               # stops and deletes the service, kills the helper, restores the policy, keeps the directory
.\uninstall.ps1 -PurgeLogs    # same, then deletes %ProgramData%\EdgeSyncSpike\ (after you copied the logs)
```

Both scripts require an elevated 64-bit PowerShell, print every action before doing it, and are idempotent (installing twice
updates in place; uninstalling when nothing is installed is not an error).

## SendSAS policy (opt-in)

`SendSAS` only works when the machine policy "Disable or enable software Secure Attention Sequence" allows services. Registry value:
`HKLM\SOFTWARE\Microsoft\Windows\CurrentVersion\Policies\System\SoftwareSASGeneration` (REG_DWORD): 0 none, 1 services,
2 Ease of Access applications, 3 services and Ease of Access applications. A Group Policy can override or lock it.
`-EnableSasPolicy` records the current value in `sas-policy-backup.json`, then sets 1 (absent or 0), 3 (was 2), or leaves it
(already 1 or 3). `uninstall.ps1` writes the old value back, or deletes the value if it did not exist, unless the value
was changed by someone else in the meantime (then it warns and keeps the backup).

## Source layout

- `service/` - `main.go` (SCM handler, flags, run limit), `session_windows.go` (supervisor: session events to launch/stop/retry),
  `launch_windows.go` (winlogon token, UIAccess, CreateProcessAsUser), `sas_windows.go` (SYSTEM-only pipe, peer verification, SendSAS, policy read).
- `helper/` - `main.go` (flags, service watch, pipe client), `desktop_windows.go` (input-desktop follower),
  `inject_windows.go` (SendInput square and text), `geometry.go` (pure square path and absolute-coordinate maths).
- `internal/proto` - pure framing, command whitelist, authorisation decisions, desktop-name parsing (host-tested).
- `internal/supervise` - pure session state machine, backoff and the max-run-time limit (host-tested, `testing/synctest`).

Host tests: `go -C spikes test ./winsvc/...` (pure logic and the SDDL assertion; no service, token, desktop or pipe is touched).

## What to paste back

- `service.log` and `helper.log` for each run (they contain no secrets: pids, session ids, SIDs, desktop names).
- For each observation step above: what happened on screen and the matching log lines.
- Output of `reg query "HKLM\SOFTWARE\Microsoft\Windows\CurrentVersion\Policies\System" /v SoftwareSASGeneration` before and after.
- Windows build (`winver`), whether Group Policy or an MDM manages the machine, and whether the `-LaunchDesktop default` run behaved differently.

## Known limits

- The service does not read the session id from the `SERVICE_CONTROL_SESSIONCHANGE` payload (the buffer may be gone when it is
  read); it re-reads `WTSGetActiveConsoleSessionId` for every event.
- If the helper is created a few milliseconds before the service records its pid, the first `Hello` is answered `denied`, the helper
  exits and is restarted after the 1 s backoff.
- Secure-desktop input forwarding to a user agent (D-82) and the brokered handle (D-81) are not part of this spike.
