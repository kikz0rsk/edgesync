# Stream brief - P0 SP1 Windows input, DPI, service and secure desktop

Phase 0 spike stream. Items: `briefs/plan-phase-0.md` section "SP1". Owner tag: `[owner:SP1]`.

## Scope
Prove (or disprove) the Windows mechanisms behind D-50 (capture), D-49 (injection), D-51 (DPI/edges), D-63/D-64
(service, secure helper, pre-login), D-47 (service IPC) and D-41 (signing proxy). Throwaway code in `spikes/`;
the deliverable is evidence plus `briefs/spikes/sp1.md`.

## Files you may touch
Only your item's `Files:`. Shared spike helpers you create live under `spikes/internal/` as listed. Never touch the
main module (`internal/`, `cmd/`, `gui/`), plan files, or other streams' spike directories.

## Conventions (binding)
- Pure Go, `CGO_ENABLED=0` (D-39). Win32 calls via `golang.org/x/sys/windows` v0.49.0 where it has wrappers, otherwise `windows.NewLazySystemDLL("user32.dll").NewProc(...)`. Verify every struct layout and constant against Microsoft Learn before use and assert sizes with `unsafe.Sizeof` in a test or init check.
- Create every `syscall.NewCallback` exactly once per process (finite, never freed - RPI 1.1).
- Threads that own hooks, windows or desktops: `runtime.LockOSThread()` for their whole life, with their own `GetMessageW` loop.
- Never launch spike binaries yourself; build them (`go -C spikes build -o bin/<name>.exe ./<dir>`) and let the user run them.
- No em dashes or en dashes; use "-". Comments only for genuinely non-obvious Win32 gotchas.
- Commit format `spike(win): <description> [FEAT-177]`, body `Item: <id>`. No attribution trailers.
- Research pointers: `briefs/research-platform-internals.md` sections 1.1, 2 (Windows), 3 (Windows), 9; Microsoft Learn pages cited there.

## Report format
```
FILES: <path> - <one clause>
GATES: <gate> pass|fail
ITEMS: <item id> <FEAT-NNN, ...>
TESTS: <what the test phase must cover, one line>
BLOCKED: <reason or NONE>
```

---

## Item P0-SP1-001
LL hooks under GC with watchdog and Raw Input.
- `spikes/internal/winhook`: `type Hook struct` started by `Start(ctx, sink *Ring) error` which spawns a goroutine that locks its OS thread, installs `WH_MOUSE_LL` (14) and `WH_KEYBOARD_LL` (13) with `SetWindowsHookExW(id, cb, hInstance=GetModuleHandleW(nil), 0)`, then runs `GetMessageW`. Callback signature `func(nCode int32, wParam, lParam uintptr) uintptr`, reading `MSLLHOOKSTRUCT {Pt POINT; MouseData, Flags, Time uint32; ExtraInfo uintptr}` / `KBDLLHOOKSTRUCT {VkCode, ScanCode, Flags, Time uint32; ExtraInfo uintptr}` through `unsafe.Pointer(lParam)`. The callback: if `nCode < 0` call `CallNextHookEx`; record start time (QueryPerformanceCounter via LazyDLL, no allocation), push a fixed-size event into the ring (no allocation), if suppression is on and the event is not injected-with-our-marker return 1, else `CallNextHookEx`; record duration into a preallocated histogram.
- `ring.go`: single-producer single-consumer ring of a fixed struct, power-of-two capacity 4096, atomic head/tail, `Push` returns false and increments a drop counter when full.
- `watchdog_windows.go`: every 1 s inject a marker event via `SendInput` with `dwExtraInfo = 0x45444753`; try both a zero relative mouse move and an F24 key-up and report which one the LL hook observes; if no marker seen within 2 s, post `WM_APP+1` to the hook thread, which unhooks and reinstalls; count reinstalls.
- `rawinput_windows.go`: hidden message-only window (`HWND_MESSAGE`) on the hook thread, `RegisterRawInputDevices` for usage page 0x01 usage 0x02 with `RIDEV_INPUTSINK` (0x100) and `hwndTarget`, handle `WM_INPUT` (0x00FF) with `GetRawInputData(RID_INPUT)`, accumulate `lLastX/lLastY` when `usFlags` has `MOUSE_MOVE_RELATIVE`.
- `spikes/internal/stats/histogram.go`: fixed buckets (log2 microseconds up to 2 s), percentiles.
- `spikes/winhooks/main.go` flags: `-duration 60s`, `-gcstress` (8 goroutines allocating 64 MiB/s and calling `runtime.GC()` every 100 ms), `-stall` (block one callback 1500 ms to force hook removal), `-csv out.csv`. Scroll Lock toggles suppression. Prints per-second lines and a final summary (callback p50/p99/max, events, drops, reinstalls, raw vs hook delta sums while the pointer pushes against each screen edge).
- AllocsPerRun test covers `Ring.Push` and the histogram record path.

## Item P0-SP1-002
Injection, DPI manifest, real edges.
- `versioninfo.json` + `manifest.xml`: manifest with `<dpiAwareness xmlns="http://schemas.microsoft.com/SMI/2016/WindowsSettings">PerMonitorV2</dpiAwareness>` and `<dpiAware xmlns="http://schemas.microsoft.com/SMI/2005/WindowsSettings">true</dpiAware>`, `requestedExecutionLevel level="asInvoker"`, supportedOS Windows 10/11 GUID. Generate `resource_windows_amd64.syso` with `go tool -modfile=tools/tools.mod goversioninfo -64 -o spikes/wininject/resource_windows_amd64.syso spikes/wininject/versioninfo.json` (verify flags with `-h`).
- Monitors: `EnumDisplayMonitors` callback (created once), `GetMonitorInfoW` (MONITORINFOEXW), `GetDpiForMonitor` (shcore.dll, MDT_EFFECTIVE_DPI). Print physical bounds, work area, DPI, scale.
- `edges.go` (pure Go, host-tested): given monitor rects, return real edge segments (sides not shared with another monitor), splitting a side where it is partly shared. Table tests from the item.
- Injection: `SendInput` with `INPUT` (assert size 40 on amd64), `MOUSEEVENTF_MOVE|MOUSEEVENTF_ABSOLUTE|MOUSEEVENTF_VIRTUALDESK` normalised against `SM_XVIRTUALSCREEN/SM_YVIRTUALSCREEN/SM_CXVIRTUALSCREEN/SM_CYVIRTUALSCREEN` (76/77/78/79) as `((x - vx) * 65535) / (vw - 1)`; read back `GetCursorPos`, print error in px per target (corners inset 1 px and centre of every monitor). Keys: `KEYEVENTF_SCANCODE` with `KEYEVENTF_EXTENDEDKEY` for E0 keys; a 5 s countdown lets the user focus Notepad. Wheel `MOUSEEVENTF_WHEEL`/`HWHEEL` with 120 and 30 units, `XDOWN/XUP` with `XBUTTON1/2`. Time each `SendInput` call.

## Item P0-SP1-003
Cursor strategies while suppression is on (reuse `spikes/internal/winhook`).
- (a) `ClipCursor` to a 1x1 rect at the primary centre + `SetCursorPos`; release on exit.
- (b) 1x1 topmost tool window at the centre + `SetCapture`, transparent cursor class.
- (c) fullscreen per-monitor layered windows (`WS_EX_LAYERED|WS_EX_TOPMOST|WS_EX_TOOLWINDOW`, alpha 1) with a transparent cursor set in `WM_SETCURSOR`. Never call `SetSystemCursor`.
- Switch strategy with keys 1/2/3; Scroll Lock toggles suppression. README lists the observations to record.

## Item P0-SP1-004
Service, secure helper, SendSAS.
- `service/main.go`: `golang.org/x/sys/windows/svc` handler accepting `svc.AcceptStop | svc.AcceptSessionChange`; logs to `%ProgramData%\EdgeSyncSpike\service.log`.
- `session_windows.go`: on start and on `WTS_CONSOLE_CONNECT`, `WTS_SESSION_LOGON`, `WTS_SESSION_UNLOCK`: target session = `WTSGetActiveConsoleSessionId()`; if no helper runs there, launch it.
- `launch_windows.go`: find `winlogon.exe` in the target session (process snapshot + `ProcessIdToSessionId`), `OpenProcess` + `OpenProcessToken(TOKEN_DUPLICATE|TOKEN_QUERY|TOKEN_ASSIGN_PRIMARY|TOKEN_ADJUST_DEFAULT|TOKEN_ADJUST_SESSIONID)`, `DuplicateTokenEx(..., SecurityImpersonation, TokenPrimary)`, `SetTokenInformation(TokenUIAccess=1)`, `CreateProcessAsUser` with `lpDesktop = "winsta0\\winlogon"` first (record if `"winsta0\\default"` behaves differently); restart with backoff on exit.
- `sas_windows.go`: on a request from the helper (a named event `Global\EdgeSyncSpikeSAS` with SYSTEM-only DACL), call `sas.dll!SendSAS(FALSE)`; log `HKLM\SOFTWARE\Microsoft\Windows\CurrentVersion\Policies\System\SoftwareSASGeneration`.
- `helper/desktop_windows.go`: locked thread polls `OpenInputDesktop(0, FALSE, GENERIC_ALL)` every 200 ms, logs the desktop name (`GetUserObjectInformationW(UOI_NAME)`), and `SetThreadDesktop` on change.
- `helper/inject_windows.go`: every 10 s (and on `-once`) on the current input desktop: move the pointer in a 200 px square, type `edgesync` by scancodes, press Escape; signal the SAS event when started with `-sas`.
- `install.ps1`/`uninstall.ps1`: require admin (`#Requires -RunAsAdministrator`), `sc.exe create EdgeSyncSpikeSvc binPath= "<abs path>\spikesvc.exe" start= demand`, start/stop/delete; idempotent.

## Item P0-SP1-005
Named pipes and the signing proxy (github.com/Microsoft/go-winio v0.6.4).
- `sddl_windows.go`: build `D:P(A;;GA;;;SY)(A;;GA;;;<current user SID>)` and a variant for the service pipe `D:P(A;;GA;;;SY)(A;;GRGW;;;AU)`; parse back and list ACEs for the test.
- `verify_windows.go`: client side `GetNamedPipeServerProcessId` -> `OpenProcess` -> token user SID must equal ours; server side `GetNamedPipeClientProcessId` -> token user SID + `QueryFullProcessImageNameW`.
- `signer.go`: `type ProxySigner struct` implementing `crypto.Signer` (`Public()`, `Sign(rand, digest, opts)`) by sending the digest + hash id over an `io.ReadWriter` to a server that holds an ECDSA P-256 key. `signer_test.go` runs a TLS 1.3 handshake over `net.Pipe` where the server's `tls.Certificate.PrivateKey` is the proxy; measure handshake time with and without proxy.
- `pipe_windows_test.go`: local pipes on the host (allowed); first-instance flag, SDDL ACE assertions, SID verification.
- cmd/pipeserver + cmd/pipeclient for the user's "second Windows account" test.

## Item P0-SP1-006
`qa/manual/p0-sp1-windows.md` per `qa/manual/README.md`. Sections in order: preparation (prebuilt binaries in `spikes\bin\`), winhooks (with and without `-gcstress`, with `-stall`, push against every edge), wininject (Notepad focus), wincursor (strategies 1-3 incl. fullscreen video and Task Manager), winsvc (install as admin, UAC prompt, Win+L, Ctrl+Alt+Del via `-sas`, reboot with nobody logged on, `taskkill /F` restart, uninstall), winpipe (second account). Each step: command, expected observation, what to paste.

## Item P0-SP1-M01
User-owned. The parent sends the script and records results into `qa/manual/results/p0-sp1-windows.md`.

## Item P0-SP1-007
`briefs/spikes/sp1.md` structure: Summary table (mechanism | go/no-go | evidence | numbers | decision confirmed/contradicted); one section per mechanism; "Recommended design for P2 W1/W2" (concrete: which delta source, which hide strategy, GC settings, watchdog marker kind, helper launch details); "Requires user decision" (every contradiction of D-41, D-47, D-50, D-63, D-64, with options). Never restate a decision as changed.

## Item P0-SP1-011
Found by the orchestrator while merging P0-SP2-004 (a whole-tree cross build, which no per-directory gate had run): `spikes/wininject/main.go` and
`spikes/wincursor/main.go` carry no build tag, so `go vet ./...` and `go build ./...` fail on the linux and macOS jobs of the spikes workflow
(`undefined: monitorInfo`, `undefined: winhook.Hook`). Add `//go:build windows` plus a blank line as the first lines of both files. `wininject`
also has untagged pure logic (`edges.go`, `edges_test.go`) that must keep building and testing on every OS, so after tagging `main.go` the package has no
`main` on non-Windows; add `spikes/wininject/main_other.go` (`//go:build !windows`) with `package main` and a `main` that writes "windows only" to stderr
and exits 2. `wincursor` has only Windows files besides `main.go`, so tagging is enough. Verify per package with `go build -o NUL` and `go vet` for
GOOS=linux and GOOS=darwin (CGO_ENABLED=0), and confirm the windows build and tests are unchanged.

## Item P0-SP1-012
`spikes/winsvc/helper` has untagged pure logic (`geometry.go`, `geometry_test.go`, which must keep running on every OS) and a windows-tagged `main.go`, so on linux and
macOS the package is a `package main` without `main` (found by the whole-package cross build while merging P0-SP1-004). Add `spikes/winsvc/helper/main_other.go`
(`//go:build !windows`, `package main`) with a `main` that writes "windows only" to stderr and exits 2. Do not touch any other file. `spikes/winsvc/service` has only
windows-tagged files and needs nothing.

## Item P0-SP1-013
Evidence: `qa/manual/results/p0-sp5-core.md` (Windows runner section). `TestPipe_OwnerOnlySDDL` compares the pipe's DACL as a string with an expected string built from the
current user's SID. On the GitHub runner the user is the built-in Administrator (RID 500) and Windows renders that SID as the alias `LA` when the descriptor is read back, so the
strings differ although the DACL is right. Rewrite only the assertion: read the security descriptor, parse the DACL into ACEs and compare SIDs, not text (the spike
`spikes/winpipe/sddl_windows.go` has a `ParseDACL` you may study but not modify here). Cover the alias case with a pure table test. Do not change the production pipe code.
