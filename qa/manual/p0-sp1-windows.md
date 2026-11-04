# SP1 Windows input, service and secure-desktop spikes - manual script

Item: P0-SP1-M01   Machines: Windows PC 1
Estimated time: 150 minutes   Needs admin: yes (the winsvc steps only)

Nothing in this script has been run by its author. The only parts run so far are the two the orchestrator ran on this PC
under D-126, and they are marked "orchestrator ran this" below: (1) the 25 s winhooks baseline with wininject generating
events, and (2) the wininject topology, DPI, absolute-probe, typing and wheel report (worst absolute error 1 px, PerMonitorV2
true). Their results are in `qa/manual/results/p0-sp1-windows.md`. Every other step is a first run by you.

The commands and flags were checked against the sources and the READMEs of `spikes/winhooks`, `spikes/wininject`,
`spikes/wincursor`, `spikes/winsvc` and `spikes/winpipe` (the winpipe sources are on `main`). Treat every output as first
evidence, not as a confirmed fact.

Conventions in this script:
- Runs use PowerShell (or cmd) with backslashes, exactly as the READMEs show. `cd spikes` from the repository root, then
  `.\bin\<name>.exe`, for the winhooks, wininject and wincursor steps.
- Build commands run from the repository root and use `-o bin/<name>.exe`. The `-o` path is relative to `spikes\`. Never
  write `-o spikes/bin/...`: with `-C spikes` that writes to `spikes/spikes/bin/`.
- Each step has a Status line. "orchestrator ran this" means the orchestrator already covered part of the step; the rest
  is marked "needs you". "needs you" means only you can run it (a person at the keyboard, an elevated PowerShell, a reboot,
  a second account or a registry policy change).

## Preparation
1. Open PowerShell in the repository root and record the commit: `git rev-parse --short HEAD`. Keep the hash for the results file.
2. Record the machine: `winver`; the number of monitors with resolution and scale (Settings > System > Display); whether the
   primary monitor is not at the top-left of the virtual desktop; the output of `reg query "HKCU\Control Panel\Desktop" /v LowLevelHooksTimeout`
   (it may report not found) and of `reg query "HKCU\Control Panel\Desktop" /v LogPixels`; mouse polling rate if known.
3. Build the seven binaries from the repository root. If `spikes\bin\` already holds them, building again only replaces them;
   the build runs nothing. Go 1.27.3 must be on PATH.
   ```
   go -C spikes build -o bin/winhooks.exe ./winhooks
   go -C spikes build -o bin/wininject.exe ./wininject
   go -C spikes build -o bin/wincursor.exe ./wincursor
   go -C spikes build -o bin/spikesvc.exe ./winsvc/service
   go -C spikes build -o bin/spikehelper.exe ./winsvc/helper
   go -C spikes build -o bin/pipeserver.exe ./winpipe/cmd/pipeserver
   go -C spikes build -o bin/pipeclient.exe ./winpipe/cmd/pipeclient
   ```
   The binaries land in `spikes\bin\` (gitignored). Each Windows binary is `spikes\bin\<name>.exe`.
4. Close Notepad and every window with unsaved or sensitive content. The wininject run types text and presses keys into
   whatever has focus, and the winsvc injection types into the focused window or the lock screen.
5. Know where Scroll Lock is before Step 1 (see Safety). Scroll Lock is the release for winhooks and wincursor.
6. Open two PowerShell windows in the repository's `spikes\` folder: one for the runs, one spare for the log commands later.
7. Create the results file `qa/manual/results/p0-sp1-windows.md` in the results format of `qa/manual/README.md` (the parent
   may create it from your pasted output in chat).

## Steps
### Step 1 - winhooks baseline, 60 s (Scroll Lock suppression and edge pushes)
- Status: orchestrator ran this, partly. The 25 s baseline with wininject generating events gave callback p50 1 us, p99 425.8 us,
  max 425.8 us, 0 drops, hook and Raw Input pointer-delta sums equal; the keyboard heartbeat was seen 24 of 24 times and the
  zero-move mouse heartbeat 0 times. Needs you: this full 60 s run, with the Scroll Lock suppression and the edge pushes.
- Do: in `spikes\`: `mkdir out`, then `.\bin\winhooks.exe -duration 60s -csv out\winhooks-baseline.csv`. During the run, follow
  the README steps in order:
  1. Move the mouse continuously for about 10 s, scroll and click a few times, type a few keys.
  2. Press Scroll Lock once (suppression ON). Move the mouse for about 10 s, then press Scroll Lock again (OFF).
  3. Push the pointer hard against the left, right, top and bottom edges of the virtual screen, about 5 s each, and keep
     moving in the pushing direction. Then do the same once with suppression ON (Scroll Lock pressed first).
  4. Leave the pointer and keyboard idle for the last 10 s.
- Expect: the first line is `virtual screen: x=... y=... w=... h=...  ring=4096 ... gcstress=false stall=false`. Per-second
  lines show `suppress=ON` only while Scroll Lock is on, and the pointer does not move on screen then. `drops=0`.
  `reinstalls=0`. The key marker count rises. The mouse marker count may stay 0: the README says a kind that stays 0 is not
  usable as the liveness signal, so record it, do not treat it as a failure. In the summary, if the hook coordinates are
  clamped at an edge, the hook sum stays near 0 on the pushing axis while the raw sum keeps growing; record which happens.
- Record: the console output from the `virtual screen:` line to the end of the `=== summary ===` table; the contents of
  `spikes\out\winhooks-baseline.csv`; the output of both `reg query` commands from Preparation; `winver`; yes or no for:
  pointer froze while ON, keyboard blocked while ON, Scroll Lock released both, a hook stopped delivering events; the mouse
  marker count.

### Step 2 - winhooks with GC stress, 60 s
- Status: needs you.
- Do: in `spikes\`: `.\bin\winhooks.exe -duration 60s -gcstress -csv out\winhooks-gcstress.csv`. Follow the four README steps
  of Step 1 during the run.
- Expect: the first line shows `gcstress=true`. The callback max may be larger than in Step 1 (GC-induced delay). `drops` stays 0.
- Record: as Step 1, for `spikes\out\winhooks-gcstress.csv`.

### Step 3 - winhooks with a forced callback stall, 30 s
- Status: needs you.
- Warning: this run freezes keyboard and mouse once, for about 1.5 s, when the first callback blocks. Wait for the pointer to move again.
- Do: in `spikes\`: `.\bin\winhooks.exe -duration 30s -stall -csv out\winhooks-stall.csv`. Follow the README steps of Step 1
  (the README's first event blocks the callback 1500 ms, the watchdog marker arrives within 1 s). Keep the mouse moving
  afterwards.
- Expect: the first line shows `stall=true`. The callback max is close to 1.5 s. `reinstalls` rises to at least 1 within about
  3 s after the stall. `stale_unhooks` counts hooks the OS had already removed when the watchdog unhooked them. The pointer and
  keyboard work again after the stall.
- Record: as Step 1, for `spikes\out\winhooks-stall.csv`, plus the time at which input froze and resumed.

### Step 4 - wininject with Notepad focus
- Status: orchestrator ran this, partly. The run (topology, DPI, absolute probes, typing and wheel report) was done by the
  orchestrator: worst absolute error 1 px, PerMonitorV2 true. Needs you: the Notepad observation (typed text, caret, wheel and
  X buttons), which needs a person at the keyboard.
- Do: in `spikes\`: `.\bin\wininject.exe -countdown 5s`. Do not touch the mouse while the probes run (about 50 ms per probe).
  When the line `typing starts in 5s` appears, open Notepad, maximise it on the primary monitor, click into the text area and
  keep the pointer over the text area. The scancode pattern is typed, then the wheel and X-button phase runs with the pointer
  where the last step left it. Keep Notepad under the pointer; X buttons may trigger navigation in other windows.
  Ctrl+C aborts the run at any point.
- Expect:
  - `process DPI awareness: 2` and `thread is DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2: true`.
  - The `virtual desktop (GetSystemMetrics, used for normalization)` line equals the `bounding box of monitors (GetMonitorInfoW)` line.
  - `real edge` lines: a shared side does not appear; a gap between monitors appears as a real edge on both sides.
  - Every `abs ... error=(0,0)px` line, at most 1 px per axis; the `absolute: worst |error| = N px` line shows N of 0 or 1.
  - Notepad reads `edgesync 1234567890` followed by a new line; the arrows move the caret; Right Ctrl does nothing visible.
  - The `wheel and buttons:` line: the wheel scrolls the window under the pointer by one notch and by a 30-unit step in each
    direction; X buttons reach the window under the pointer.
  - The `SendInput latency` lines: `absolute:`, `typing:` and `wheel and buttons:`, each with min, mean and max.
- Record: the complete console output of the run; the Notepad observation (text, caret, wheel, X buttons: yes or no each);
  the monitor count, resolutions, scales and primary position; the `LogPixels` output; any line on stderr, verbatim.

### Step 5 - wincursor strategy 1 (ClipCursor) with suppression on
- Status: needs you.
- Do: in `spikes\`: `.\bin\wincursor.exe -strategy 1 -duration 60s`. Press Scroll Lock (suppression ON; the engage line names
  strategy 1). Then, with suppression ON, check each row of this table and fill in the row:
  - Notepad: move the pointer against every screen edge; click; type a few keys.
  - Fullscreen video: play a video fullscreen in another application; move the pointer; click.
  - Task Manager (elevated): open Task Manager as administrator and keep it focused; move the pointer; click. UIPI may affect
    this; record what happens, do not expect a result.
  - Second monitor: move the pointer onto a secondary monitor.
  Press Scroll Lock again (OFF) when done. The run ends by itself after 60 s.
- Expect: the pointer is visible and stays at the primary monitor centre. Strategy 1 must not leave the centre when moved onto a
  secondary monitor. Clicks and typing are swallowed. Scroll Lock OFF frees the pointer at once. The summary shows
  `engaged 1 time(s): strategy 1: ClipCursor 1x1 + SetCursorPos`.
- Record: the table rows (Context | pointer visible | fixed at centre | clicks and typing swallowed | focus or playback changed);
  the `engage`, `release` and `summary` lines verbatim.

### Step 6 - wincursor strategy 2 (captured 1x1 window) with suppression on
- Status: needs you.
- Do: in `spikes\`: `.\bin\wincursor.exe -strategy 2 -duration 60s`. Repeat the table of Step 5 with suppression ON.
- Expect: the pointer is kept at the primary centre by the captured window (it re-centres on every move it receives). Check
  whether the window takes focus from the foreground application: strategy 2 uses `SW_SHOWNOACTIVATE`, so it should not.
  The summary shows `engaged 1 time(s): strategy 2: 1x1 topmost window + SetCapture`.
- Record: the table rows; the `engage`, `release` and `summary` lines; yes or no for focus taken from the foreground application.

### Step 7 - wincursor strategy 3 (layered windows) with suppression on
- Status: needs you.
- Do: in `spikes\`: `.\bin\wincursor.exe -strategy 3 -duration 60s`. Repeat the table of Step 5 with suppression ON.
- Expect: one fullscreen layered window per monitor with a transparent cursor; the pointer is re-centred on the primary monitor.
  The windows are `WS_EX_NOACTIVATE`, so they should not take focus. The summary shows
  `engaged 1 time(s): strategy 3: fullscreen layered windows (alpha 1) + transparent cursor`.
- Record: the table rows; the `engage`, `release` and `summary` lines; yes or no for focus taken; whether any leftover window
  stayed after the run.

### Step 8 - wincursor strategy switching, taskkill /F and recovery
- Status: needs you.
- Do:
  1. `.\bin\wincursor.exe -strategy 1` (no duration; Ctrl+C ends the run). Press Scroll Lock (ON). While ON, press 1, 2, 3 in turn
     to switch strategies. Note any flicker, leftover window or stuck confinement. Press Scroll Lock (OFF). Press Ctrl+C.
  2. Start `.\bin\wincursor.exe -strategy 1` again and press Scroll Lock (ON). In the spare window run
     `taskkill /F /IM wincursor.exe` (the README names `taskkill /F` for this test). Record whether the pointer stays confined
     under strategy 1 and whether the layered or capture windows are gone.
  3. Recovery: `.\bin\wincursor.exe -release` (clears a ClipCursor confinement left by the killed run and exits). Confirm the
     pointer moves freely afterwards.
  4. Sign-out test: start `.\bin\wincursor.exe -strategy 1`, press Scroll Lock (ON), sign out from the Start menu, sign back in.
     Record whether the pointer is confined after sign-in. If it is, run `.\bin\wincursor.exe -release`.
- Expect: switching keys work while suppression is ON with no leftover window. After `taskkill /F` the layered and capture
  windows are gone (process-owned); whether the ClipCursor confinement survives is what this step records. `-release` restores
  the pointer. The sign-out result is recorded as observed, not assumed.
- Record: the engage and release lines of each part; the result of `taskkill /F` (confined yes or no, windows gone yes or no);
  the result of `-release`; the sign-out result; the summary block of part 1.

### Step 9 - winsvc: install the SYSTEM service with injection, token and launch check
- Status: needs you. Needs an elevated 64-bit PowerShell (D-126 (d)).
- Do:
  1. Before installing, record the policy: `reg query "HKLM\SOFTWARE\Microsoft\Windows\CurrentVersion\Policies\System" /v SoftwareSASGeneration`
     (it may report not found).
  2. Open PowerShell as administrator (Run as administrator, 64-bit). Go to the repository's `spikes\winsvc` folder. If scripts
     are blocked, use `powershell -ExecutionPolicy Bypass -File .\install.ps1 -Inject` (the README form).
  3. Run `.\install.ps1 -Inject`. It prints `[ACTION]` lines and, at the end, the Name, Status and StartType of the service.
  4. In the spare window watch the logs: `Get-Content $env:ProgramData\EdgeSyncSpike\service.log -Wait`, and the same for
     `helper.log`.
  5. Try the other launch desktop once: `.\install.ps1 -Inject -LaunchDesktop default`, observe the logs, then return to the
     default with `.\install.ps1 -Inject`.
- Expect: `service.log` has `helper launched` with `tokenUser=S-1-5-18`, `tokenSession` equal to your console session number
  (Task Manager, Details, Session# column), and `tokenUIAccess=1`. `helper.log` has `helper starting` with the same three values.
  The `-LaunchDesktop default` run is compared with the winlogon run; record any difference.
- Record: the install.ps1 console output; the `helper launched` and `helper starting` lines of each launch; the policy
  `reg query` output; `winver`; whether Group Policy or an MDM manages the machine; the difference (or none) for the
  `default` launch desktop.

### Step 10 - winsvc: desktop following (Win+L, UAC, screen saver)
- Status: needs you.
- Do: with the helper running from Step 9 (after the last install, `.\install.ps1 -Inject`):
  1. Press Win+L and unlock. Note the time of each key press.
  2. Trigger a UAC prompt: run `cmd` as administrator from the Start menu, then cancel it.
  3. Optional: set a 1 minute screen saver in the personalisation settings and let it start, then move the mouse to end it.
- Expect: `helper.log` has `input desktop switch` lines (level WARN) with `from=...` and `to=...`, where `to` is `Winlogon`,
  `Default` or `Screen-saver`, and the `pollInterval=200ms` field. Each switch is logged within about 200 ms of the real switch:
  compare the log timestamp with the time you noted. `service.log` repeats each switch as `helper input desktop` with `name`
  and `kind`.
- Record: the `input desktop switch` and `helper input desktop` lines with timestamps; the times of your key presses and the
  UAC and screen saver events; any switch that was not logged.

### Step 11 - winsvc: injection into Task Manager, a UAC prompt and the lock screen
- Status: needs you. Requires the `-Inject` install from Step 9.
- Warning: the injected text and Escape go to whatever has focus. Do not leave a document, a terminal or a password prompt focused.
- Do:
  1. Open Task Manager as administrator and give it focus. Watch the pointer draw a 200 px square, and `edgesync` typed into
     the focused window. Escape closes dialogs.
  2. Trigger a UAC prompt. Watch the pointer move and Escape cancel the prompt.
  3. Press Win+L. On the lock screen watch the pointer move and the password field show `edgesync`. Then sign in again.
- Expect: `helper.log` has the line `injected demo square, text and Escape` with `desktop`, `start`, `endReadBack` and
  `squareSize`. The `endReadBack` value is the `GetCursorPos` read-back. An error line `SendInput injected fewer events than
  requested (UIPI or desktop denial gives no error code)` means the desktop or UIPI blocked the input.
- Record: the injected lines verbatim; the on-screen observation for each of the three contexts (pointer moved, text shown,
  Escape worked: yes or no); the name of the desktop in each case; any `injected fewer events` line verbatim.

### Step 12 - winsvc: helper restart after taskkill /F
- Status: needs you.
- Do: in the spare window run `taskkill /F /IM spikehelper.exe`. Watch `service.log`.
- Expect: `helper exited` (with `pid`, `exitCode`, `uptime`, `err`), then `helper launched` with a new `pid` after the backoff
  (1 s, then 2 s, 4 s and so on, capped at 30 s; the backoff resets after the helper ran 30 s).
- Record: the `helper exited` and `helper launched` lines with timestamps.

### Step 13 - winsvc: session changes
- Status: needs you.
- Do: lock and unlock (Win+L); sign out and sign in again. If a second Windows account exists on this PC, use fast user
  switching as well (optional).
- Expect: `service.log` has `session change` lines with `event=WTS_...` and `activeConsoleSession`. A helper in the wrong
  session is terminated (`helper terminated by the service`) and a new one is launched in the active console session
  (`helper launched`).
- Record: the `session change`, `helper terminated by the service` and `helper launched` lines, with timestamps and the
  action that caused each.

### Step 14 - winsvc: SendSAS (Ctrl+Alt+Del screen) with the opt-in policy
- Status: needs you. This step changes the machine policy `SoftwareSASGeneration` (D-126 (f)): run it only with your yes.
  Opt-in is `-EnableSasPolicy`, the README's name for it; the SAS request is not a flag you pass by hand.
- Do:
  1. Install with the policy opt-in: `.\install.ps1 -Inject -EnableSasPolicy`. The script first saves the previous value to
     `%ProgramData%\EdgeSyncSpike\sas-policy-backup.json`, then sets the value (1, or 3 when it was 2).
  2. Record the policy: `reg query "HKLM\SOFTWARE\Microsoft\Windows\CurrentVersion\Policies\System" /v SoftwareSASGeneration`.
  3. About 15 s after the helper starts, the helper asks the service once for SAS. The Ctrl+Alt+Del screen opens on the
     console. Press Escape to leave it.
  4. Variant without the policy: run `.\uninstall.ps1` (it restores the policy), then `.\install.ps1 -Inject`. Read the policy
     again with the `reg query` above.
- Expect: `service.log` `service starting` shows `SoftwareSASGeneration=<value>` and `allowSAS=true` (opt-in run). Then
  `calling SendSAS(FALSE) on authenticated helper request` with `SoftwareSASGeneration=<value>`, then
  `SendSAS returned (it has no result; observe the screen)`. The helper logs `SAS requested: waiting before asking the service`
  and `SAS request answered` with `status=ok`. `SendSAS` has no return value: the evidence is the screen. In the variant without
  the policy, the install starts the helper without `-sas` (the helper requests SAS only when the service started it with
  `-allow-sas`), so no SAS request line is expected at all; the README's `refused` line is not produced by `install.ps1`.
  Record what appears. The README asks whether SendSAS works with the value absent; `install.ps1` always sets the value
  when it is run with `-EnableSasPolicy`, so that case is not testable with these scripts; record it as not run.
- Record: the service.log and helper.log lines of both variants; the `reg query` output before and after each variant; whether
  the Ctrl+Alt+Del screen opened (yes or no) and how long after the helper start; `winver`.

### Step 15 - winsvc: pre-login test (reboot with nobody signed in)
- Status: needs you. Needs a reboot.
- Do:
  1. In the elevated PowerShell (repository's `spikes\winsvc`): `.\install.ps1 -Inject -AutoStart`. This sets the start type to
     Automatic. Record the time.
  2. Reboot the PC. Do NOT sign in. The service runs for at most 30 minutes after boot (default `-MaxRunMinutes`).
  3. Watch the pointer at the sign-in screen for about a minute. It should move in a square.
  4. Sign in. Read `helper.log` of the boot.
- Expect: the pointer moves at the sign-in screen. `helper.log` of the boot has `input desktop switch` lines whose first
  switches name `Winlogon`. If the helper did not start at boot, `service.log` shows `launch helper failed` with
  `winlogon.exe not found in the target session` and the retries.
- Record: the boot time; yes or no for pointer movement at the sign-in screen; the first `input desktop switch` lines; the
  `launch helper failed` lines verbatim if any; `winver`.

### Step 16 - winsvc: hard self-stop
- Status: needs you. Observed in the boot of Step 15 or in any run that is still going.
- Do: leave the service running. Read the `selfStopAt` value from the `service starting` line of `service.log`. Wait until that
  time has passed (the README default is 30 minutes after start).
- Expect: `service.log` has `hard self-stop: maximum run time reached` with `maxRun`, then `service stopped`. The helper stops too,
  after its own limit or when the service process exits. Nothing stays running.
- Record: the `selfStopAt` value, the `hard self-stop` and `service stopped` lines with timestamps, and the time they were logged.

### Step 17 - winpipe run 1: same account, positive path
- Status: needs you.
- Do: open two Windows terminals as the same user, in the repository root.
  - Terminal A (server): `spikes\bin\pipeserver.exe -duration 5m`
  - Terminal B (client): `spikes\bin\pipeclient.exe -handshakes 10`
  Stop the server with Ctrl+C or let `-duration` expire.
- Expect: `pipeclient` prints one line per handshake, `handshake  N: local key ... ms, proxy ... ms, TLS version 0x304`, then
  `median handshake: local key ... ms, proxy ... ms, overhead ... ms (10 handshakes each)`. `pipeserver` logs `serving` (pipe, sid,
  session, image, duration) once, and `client disconnected cleanly` when the client exits.
- Record: all `pipeclient` lines, including the median and the overhead; all `pipeserver` log lines.

### Step 18 - winpipe run 2: a second Windows account must be refused at dial
- Status: needs you.
- Do: keep Terminal A of Step 17 running. Sign in as a second Windows account on this PC (switch user; the first session stays
  open). In a terminal there run `spikes\bin\pipeclient.exe`. If that account has no repository folder, use the full path of the
  same `pipeclient.exe` file. Do not change any file.
- Expect: the dial fails before any TLS. The error starts with `pipeclient: dial \\.\pipe\EdgeSyncWinpipeSpike:` and the Windows
  reason is access denied. `pipeserver` logs no new client.
- Record: the exact error line; whether `pipeserver` logged anything for it; if the second account could not reach the file,
  the exact message it printed.

### Step 19 - winpipe run 3: wrong expected server account
- Status: needs you.
- Do: from Terminal B (same account as the server, Step 17 still running in Terminal A): `spikes\bin\pipeclient.exe -server-sid S-1-5-18`.
  Then stop the server in Terminal A with Ctrl+C.
- Expect: the dial succeeds (same account), then the client refuses the server because it runs as the user, not LocalSystem:
  `pipeclient: winpipe: pipe server runs as another account`.
- Record: the exact error line; any `pipeserver` line about the refused client (`client refused or ended with error`, if logged).

## Cleanup
1. Winsvc, elevated PowerShell in `spikes\winsvc`: `.\uninstall.ps1`. It stops and deletes the service, kills the helper,
   restores the policy from the backup and keeps `%ProgramData%\EdgeSyncSpike\`. Paste its output.
2. Winsvc, after you copied the logs into the results file: `.\uninstall.ps1 -PurgeLogs` deletes `%ProgramData%\EdgeSyncSpike\`.
   This step is optional.
3. Run `reg query "HKLM\SOFTWARE\Microsoft\Windows\CurrentVersion\Policies\System" /v SoftwareSASGeneration` and compare with the
   value recorded in Step 9 (it may report not found). If the policy is not as before, paste the output and do not change it by hand.
4. Winhooks, wininject and wincursor: close Notepad and any video or Task Manager window you opened. Confirm no run is still
   going (Ctrl+C in each window whose run has not ended).
5. Pipe runs: stop `pipeserver` and close both terminals.
6. Run `git status --short` in the repository root. It must print nothing. Outputs in `spikes\out\` and binaries in `spikes\bin\`
   are gitignored. If anything else shows, paste it and do not commit it.
7. Paste the recorded observations into `qa/manual/results/p0-sp1-windows.md`, in the results format of `qa/manual/README.md`
   (date, tester, the commit hash from Preparation step 1, the machine description, one row per step, the verdict), or paste
   them in chat for the parent to commit.

## Safety
- Scroll Lock toggles suppression in winhooks and wincursor. While suppression is ON, the keyboard and mouse are blocked
  system-wide; Scroll Lock itself always passes. Press it again to release. Know where it is before Step 1.
- winhooks README: Ctrl+C works only while suppression is OFF, and the run ends by itself after `-duration`, releasing suppression
  on exit. Press Scroll Lock to turn suppression OFF before Ctrl+C. The wincursor README says Ctrl+C ends the run but does not say
  whether it works with suppression ON; record what happens in Step 8.
- The `-stall` run (Step 3) freezes input for about 1.5 s once. Wait for the pointer to move again.
- wincursor strategy 1 confines the pointer with `ClipCursor`, which is machine-wide state. If a run is killed while
  strategy 1 is engaged, run `.\bin\wincursor.exe -release` (it clears the confinement and exits), or sign out. Step 8 tests this.
- If input stays blocked after Scroll Lock: the READMEs name no other release. Try Ctrl+Alt+Del (the Windows secure attention
  sequence) and record whether it works while input is blocked. Then end `winhooks.exe` or `wincursor.exe` in Task Manager
  and record whether input recovers.
- Winsvc: the service is LocalSystem with a helper that runs as SYSTEM with UIAccess, and `-Inject` types into whatever has
  focus. Uninstall when you are done (Cleanup 1). Recovery, from the winsvc README, in an ELEVATED PowerShell or cmd:
  ```
  sc.exe stop EdgeSyncSpike
  sc.exe delete EdgeSyncSpike
  taskkill /F /IM spikehelper.exe
  taskkill /F /IM spikesvc.exe
  ```
  If you used `-EnableSasPolicy`, the previous value is in `%ProgramData%\EdgeSyncSpike\sas-policy-backup.json` (`existed`,
  `previous`, `set`, `changed`). By hand, per the README:
  ```
  reg query "HKLM\SOFTWARE\Microsoft\Windows\CurrentVersion\Policies\System" /v SoftwareSASGeneration
  reg add "HKLM\SOFTWARE\Microsoft\Windows\CurrentVersion\Policies\System" /v SoftwareSASGeneration /t REG_DWORD /d <previous> /f
  reg delete "HKLM\SOFTWARE\Microsoft\Windows\CurrentVersion\Policies\System" /v SoftwareSASGeneration /f
  ```
  Use the `reg add` line when `existed` is true, the `reg delete` line when it was false. If the machine stays at a blank
  sign-in screen, the service stops itself within `-MaxRunMinutes`; or boot to Safe Mode (the service is not started there)
  and run the `sc.exe delete` line.
- Winsvc `uninstall.ps1` is idempotent: running it twice, or with nothing installed, is safe.
- Winpipe runs install nothing and change no system setting. Stop `pipeserver` with Ctrl+C, or let `-duration` expire.
  Do not change the firewall, the registry or any service for them.
