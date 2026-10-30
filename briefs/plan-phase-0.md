# EdgeSync - Phase 0 items

Read `plan.md` (contract, orchestration, item format) first. Stream briefs: `briefs/streams/p0-*.md`.
Spike rules (D-74): spike code lives in the separate module `spikes/` (`github.com/kikz0r/edgesync/spikes`);
agents may build spike code and run host-safe tests (pure logic, loopback unicast, local named pipes) but never
run spike binaries; the user runs them via `qa/manual/p0-*.md`. Built binaries go to `spikes/bin/` (gitignored).

Spike gate (used in every spike item's DoD, "spike gates"):
- `go -C spikes vet ./...` and `go -C spikes build ./...` on the host (Windows)
- `GOOS=linux CGO_ENABLED=0 go -C spikes vet ./linux/...` when the item touches `spikes/linux/` (Gio-based Linux spikes are excluded because they need cgo; they are built on the GNOME box per the qa script)
- `GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go -C spikes vet ./macos/...` when the item touches `spikes/macos/`
- `go -C spikes test ./<touched dirs>/...` (host-safe tests only)
- Each spike directory has a README.md with: purpose, exact build command, exact run command(s), what to observe, what to paste back.

## Phase 0 - Spikes & toolchain

### P0-F - Foundation block (sequential, 1 agent, in order)

- [x] [P0-F-001] Install golangci-lint v2.15.0 from the official release binary [FEAT-169] [tier:haiku] [owner:USER-CONFIRM] [brief:briefs/streams/p0-f.md#item-p0-f-001]
  Needs: user approval in chat
  Files: qa/manual/results/p0-f-toolchain.md (created; records version output, install path, checksum)
  Tests: SHA-256 of `golangci-lint-2.15.0-windows-amd64.zip` equals the line in `golangci-lint-2.15.0-checksums.txt`; `golangci-lint version` prints `2.15.0` in Git Bash and PowerShell
  DoD: binary at `%USERPROFILE%\go\bin\golangci-lint.exe` and resolvable on PATH (if that directory is not on PATH: BLOCKED, do not edit PATH); results file committed `build(toolchain): install golangci-lint v2.15.0 [FEAT-169]`

- [x] [P0-F-002] Install the Cap'n Proto compiler 1.4.0 via vcpkg [FEAT-169, FEAT-120] [tier:haiku] [owner:USER-CONFIRM] [brief:briefs/streams/p0-f.md#item-p0-f-002]
  Needs: user approval in chat
  Files: qa/manual/results/p0-f-toolchain.md (append section)
  Tests: vswhere reports an MSVC x64 toolset (else BLOCKED with the exact missing component); `D:\Development\vcpkg\vcpkg.exe install capnproto:x64-windows` succeeds; `capnp --version` prints `Cap'n Proto version 1.4.0`
  DoD: absolute path of `capnp.exe` recorded in the results file; commit `build(toolchain): install capnp 1.4.0 via vcpkg [FEAT-169]`

- [x] [P0-F-003] Install Inno Setup 7.1.1 with winget and verify ISCC [FEAT-169, FEAT-163] [tier:haiku] [owner:USER-CONFIRM] [brief:briefs/streams/p0-f.md#item-p0-f-003]
  Needs: user approval in chat
  Files: qa/manual/results/p0-f-toolchain.md (append section)
  Tests: `winget show --id JRSoftware.InnoSetup.7 --exact` (read-only) reports version 7.1.1 and the installer SHA-256 recorded in D-118 (anything else: BLOCKED, ask the user); `winget install --id JRSoftware.InnoSetup.7 --exact --version 7.1.1` succeeds; `ISCC.exe --version` reports 7.1.1; a one-line scratch script compiles with ISCC into the scratchpad directory (the produced setup is never run)
  DoD: package id, version, installer SHA-256 and the absolute path of `ISCC.exe` recorded in the results file (the id `JRSoftware.InnoSetup` is Inno Setup 6 and must not be used); commit `build(toolchain): install Inno Setup 7.1.1 [FEAT-169]`

- [x] [P0-F-004] Repository scaffold: go.mod, LICENSE, README, .gitattributes, .editorconfig [FEAT-169] [tier:haiku] [owner:P0-F] [brief:briefs/streams/p0-f.md#item-p0-f-004]
  Files: go.mod, LICENSE, README.md, .gitattributes, .editorconfig
  Tests: `go mod edit -json` shows `"Path": "github.com/kikz0r/edgesync"` and `"Go": "1.27"`; `go build ./...` exits 0
  DoD: LICENSE is the MIT text with "Copyright (c) 2026 kikz0r" (D-79); README has project summary, status "in development", link to plan.md; .gitattributes sets `* text=auto eol=lf`, binary for `*.syso *.png *.ico *.icns *.zip`; .editorconfig (LF, UTF-8, tabs for .go, 2 spaces for yml/yaml/toml/md/capnp); commit `chore(repo): scaffold module and repository files [FEAT-169]`

- [x] [P0-F-005] Spikes module and spikes CI workflow [FEAT-177, FEAT-167] [tier:haiku] [owner:P0-F] [brief:briefs/streams/p0-f.md#item-p0-f-005]
  Needs: P0-F-004
  Files: spikes/go.mod, spikes/README.md, .github/workflows/spikes.yml
  Tests: `go -C spikes build ./...` exits 0; `spikes.yml` is a `workflow_dispatch` workflow with jobs `windows` (windows-latest), `macos` (macos-latest), `linux` (ubuntu-24.04 with `xvfb` installed and `Xvfb :99` started, `DISPLAY=:99`), each running `go -C spikes vet ./...`, `go -C spikes build ./...`, `go -C spikes test -tags integration ./...` and uploading `spikes/out/**` as an artifact
  DoD: spikes/go.mod is `module github.com/kikz0r/edgesync/spikes`, `go 1.27`; README states the spike rules (D-74) verbatim; action versions pinned to exact release tags verified via the GitHub API at execution time; commit `ci(spikes): add spikes module and dispatch workflow [FEAT-177]`

- [x] [P0-F-006] Tools alternate module with pinned tool directives [FEAT-169] [tier:haiku] [owner:P0-F] [brief:briefs/streams/p0-f.md#item-p0-f-006]
  Needs: P0-F-004
  Files: tools/tools.mod, tools/tools.sum
  Tests: from the repo root each of these runs and prints its version or help: `go tool -modfile=tools/tools.mod gofumpt -version` (v0.12.0), `go tool -modfile=tools/tools.mod govulncheck -version` (x/vuln v1.9.0), `go tool -modfile=tools/tools.mod actionlint -version` (1.7.12), `go tool -modfile=tools/tools.mod goversioninfo -h`, `go build -modfile=tools/tools.mod -o bin/capnpc-go.exe capnproto.org/go/capnp/v3/capnpc-go`; root `go.mod` unchanged
  DoD: tools.mod `module github.com/kikz0r/edgesync`, `go 1.27`, exact versions per D-54; any failing invocation -> BLOCKED with output; commit `build(tools): pin dev tools via tools.mod [FEAT-169]`

- [x] [P0-F-007] golangci-lint v2 strict configuration [FEAT-169] [tier:haiku] [owner:P0-F] [brief:briefs/streams/p0-f.md#item-p0-f-007]
  Needs: P0-F-001, P0-F-004
  Files: .golangci.yml
  Tests: `golangci-lint config verify` exits 0; `golangci-lint run ./...` exits 0
  DoD: contents exactly per D-55 and the brief (linter list, settings, depguard rules, forbidigo patterns, exclusions, formatters gofumpt + goimports with local prefix); commit `build(lint): strict golangci-lint v2 config [FEAT-169]`

- [x] [P0-F-008] Coverage policy checker tools/covercheck [FEAT-169, FEAT-167] [tier:haiku] [owner:P0-F] [brief:briefs/streams/p0-f.md#item-p0-f-008]
  Needs: P0-F-007
  Files: tools/covercheck/main.go, tools/covercheck/covercheck.go, tools/covercheck/covercheck_test.go, tools/covercheck/testdata/ok.out, tools/covercheck/testdata/low.out, tools/covercheck/testdata/notests.out, tools/coverage-exclude.txt
  Tests: TestParseProfile_SetAndAtomicModes, TestPerPackageCoverage_MergesDuplicateBlocks, TestCheck_FailsBelowMinimum, TestCheck_FailsWhenIncludedPackageHasNoProfile, TestCheck_ExcludedPatternsIgnored, TestCLI_ExitCodes
  DoD: CLI `go run ./tools/covercheck -profile coverage.out -min 85 -exclude tools/coverage-exclude.txt -packages "<go list output file>"` prints a per-package table and exits 1 on any violation; coverage of tools/covercheck >= 85%; exclude file lists `internal/platform/windows/...`, `internal/platform/darwin/...`, `internal/platform/x11/...`, `internal/platform/wayland/...`, `internal/platform/linux/...`, `internal/platform/host/...`, `internal/service/...`, `cmd/...`, `gui/...` (GUI policy is revisited in P3-F by a new decision); per-item gates green; commit `build(cover): per-package coverage gate [FEAT-169]`

- [x] [P0-F-009] Taskfile with every gate + fuzz smoke runner [FEAT-169, FEAT-167] [tier:haiku] [owner:P0-F] [brief:briefs/streams/p0-f.md#item-p0-f-009]
  Needs: P0-F-006, P0-F-007, P0-F-008
  Files: Taskfile.yml, tools/fuzzsmoke/main.go, tools/fuzzsmoke/fuzzsmoke.go, tools/fuzzsmoke/fuzzsmoke_test.go
  Tests: TestParseListOutput_MultiplePackages, TestParseListOutput_NoFuzzTargets, TestCommandsFor_TargetsAndDuration; `task --list` shows fmt, lint, lint:cross, vet, build, cross, test, test:race, cover, fuzz:smoke, vuln, actionlint, gen, gen:check, ci:local; `task ci:local` exits 0 on the current tree
  DoD: commands exactly as in the brief (race uses ucrt64 gcc on PATH, cross uses CGO_ENABLED=0, `gen` = `go generate ./...`, `gen:check` = gen + `git diff --exit-code`); coverage of tools/fuzzsmoke >= 85%; commit `build(task): Taskfile with all quality gates [FEAT-169]`

- [x] [P0-F-010] CI workflow ci.yml [FEAT-167] [tier:haiku] [owner:P0-F] [brief:briefs/streams/p0-f.md#item-p0-f-010]
  Needs: P0-F-009
  Files: .github/workflows/ci.yml
  Tests: `go tool -modfile=tools/tools.mod actionlint` exits 0 for all workflows; jobs present: `lint` (ubuntu-24.04, golangci-lint-action pinned v2.15.0, plus `GOOS=windows` and `GOOS=darwin` lint runs over agent-side packages), `test` (matrix windows-latest, macos-latest, ubuntu-24.04: setup-go 1.27.3, go-task setup, `task build test test:race cover`), `cross` (ubuntu-24.04: `task cross`), `gen-check` (ubuntu-24.04: apt `capnproto`, `task gen:check`), `vuln` (`task vuln`), `actionlint`
  DoD: every action pinned to an exact release tag verified via the GitHub API at execution time; triggers push + pull_request on main; commit `ci: main CI pipeline [FEAT-167]`

<!-- NOTE: 2026-10-30 - D-121 removed the clipboard sensitivity skip (concealed content is shared). The sensitivity-format observations in SP2 and SP4 stay informational: they are not go/no-go criteria and nothing in P1-P5 depends on them. the WiX toolchain item P0-F-003 is superseded by the Inno Setup install of the same ID (D-118). -->

- [x] [P0-F-011] Replace the deprecated gofumpt extra-rules key (D-122) [FEAT-169] [tier:haiku] [owner:P0-F] [brief:briefs/streams/p0-f.md#item-p0-f-011]
  Needs: P0-F-007
  Files: .golangci.yml
  Tests: `golangci-lint config verify` exits 0; `golangci-lint run ./...` exits 0 and prints no `extra-rules` deprecation warning; `golangci-lint fmt --diff ./...` prints no diff
  DoD: only the gofumpt setting changes, to `extra: { group-params: true, clothe-returns: true }`; commit `build(lint): use non-deprecated gofumpt extra keys [FEAT-169]`

### SP1 - Windows input, DPI, service and secure desktop

- [ ] [P0-SP1-001] LL hook spike with ring buffer, GC stress, watchdog and Raw Input deltas [FEAT-177, FEAT-033] [tier:sonnet why:LL-hook message-loop threading and non-allocating callbacks under GC pressure] [owner:SP1] [brief:briefs/streams/p0-sp1.md#item-p0-sp1-001]
  Needs: P0-F-005
  Files: spikes/internal/winhook/hook_windows.go, spikes/internal/winhook/rawinput_windows.go, spikes/internal/winhook/watchdog_windows.go, spikes/internal/winhook/ring.go, spikes/internal/winhook/ring_test.go, spikes/internal/stats/histogram.go, spikes/internal/stats/histogram_test.go, spikes/winhooks/main.go, spikes/winhooks/README.md
  Tests: TestRing_PushPopWrapAround, TestRing_FullDropsAndCounts, TestHistogram_Percentiles (host); spike must demonstrate (user run): callback duration p50/p99/max with and without `-gcstress`, events/s, ring drops, watchdog re-installs (forced by `-stall` that blocks the callback 1500 ms once), suppression toggle via Scroll Lock, Raw Input delta sums vs hook pt deltas at screen edges (clamping evidence)
  DoD: spike gates green; 0 allocs in the callback path (`testing.AllocsPerRun` over the push path); README run steps; commit `spike(win): low-level hooks under GC with watchdog [FEAT-177]`

- [ ] [P0-SP1-002] Injection, DPI manifest and real-edge spike [FEAT-177, FEAT-035, FEAT-036, FEAT-037] [tier:haiku] [owner:SP1] [brief:briefs/streams/p0-sp1.md#item-p0-sp1-002]
  Needs: P0-F-006
  Files: spikes/wininject/main.go, spikes/wininject/monitors_windows.go, spikes/wininject/inject_windows.go, spikes/wininject/edges.go, spikes/wininject/edges_test.go, spikes/wininject/versioninfo.json, spikes/wininject/manifest.xml, spikes/wininject/resource_windows_amd64.syso, spikes/wininject/README.md
  Tests: TestRealEdges_SideBySide, TestRealEdges_LShape, TestRealEdges_StackedWithOffset, TestRealEdges_GapBetweenMonitors, TestRealEdges_MixedSizes (host); spike must demonstrate: monitor list with physical bounds and DPI; absolute moves to each monitor's corners and centre with `GetCursorPos` read-back error in px; scancode typing of a fixed pattern incl. extended keys (arrows, Right Ctrl, numpad Enter) into Notepad; wheel, hwheel, X1/X2; per-call SendInput latency
  DoD: spike gates green; .syso generated with `go tool -modfile=tools/tools.mod goversioninfo` from versioninfo.json + manifest.xml (PerMonitorV2); commit `spike(win): injection and per-monitor DPI edges [FEAT-177]`

- [ ] [P0-SP1-003] Cursor hide and park strategies while grabbed [FEAT-177, FEAT-034] [tier:haiku] [owner:SP1] [brief:briefs/streams/p0-sp1.md#item-p0-sp1-003]
  Needs: P0-SP1-001
  Files: spikes/wincursor/main.go, spikes/wincursor/strategies_windows.go, spikes/wincursor/README.md
  Tests: spike must demonstrate for strategies (a) ClipCursor 1x1 + SetCursorPos, (b) 1x1 topmost window + SetCapture, (c) transparent cursor on a fullscreen layered window: cursor invisible over normal apps, over a fullscreen video, over Task Manager (elevated), restoration after normal exit and after `taskkill /F`
  DoD: spike gates green; strategy (c) never uses `SetSystemCursor`; commit `spike(win): cursor hide and park strategies [FEAT-177]`

- [ ] [P0-SP1-004] SYSTEM service, secure helper, desktop following, UIPI and SendSAS spike [FEAT-177, FEAT-044, FEAT-045, FEAT-046, FEAT-048, FEAT-049] [tier:sonnet why:token duplication, session and desktop security semantics] [owner:SP1] [brief:briefs/streams/p0-sp1.md#item-p0-sp1-004]
  Needs: P0-F-005
  Files: spikes/winsvc/service/main.go, spikes/winsvc/service/session_windows.go, spikes/winsvc/service/launch_windows.go, spikes/winsvc/service/sas_windows.go, spikes/winsvc/helper/main.go, spikes/winsvc/helper/desktop_windows.go, spikes/winsvc/helper/inject_windows.go, spikes/winsvc/install.ps1, spikes/winsvc/uninstall.ps1, spikes/winsvc/README.md
  Tests: spike must demonstrate (user run, admin for install): helper launched into the active console session with a winlogon-derived token + TokenUIAccess; helper logs input-desktop switches (Default, Winlogon, Screen-saver) within 200 ms; injected mouse square + typed text land in an elevated Task Manager, on a UAC prompt (Escape cancels it), on the lock screen (moves pointer, shows the password field); `SoftwareSASGeneration` value read and logged; SendSAS opens the Ctrl+Alt+Del screen; after reboot with nobody logged on, the helper runs at the sign-in screen and injects movement; service restarts the helper after `taskkill /F`
  DoD: spike gates green; install/uninstall scripts idempotent and require admin; all logs to `%ProgramData%\EdgeSyncSpike\`; commit `spike(win): service, secure helper and SendSAS [FEAT-177]`

- [ ] [P0-SP1-005] Named-pipe security and TLS signing-proxy spike [FEAT-177, FEAT-050, FEAT-051] [tier:haiku] [owner:SP1] [brief:briefs/streams/p0-sp1.md#item-p0-sp1-005]
  Needs: P0-F-005
  Files: spikes/winpipe/sddl_windows.go, spikes/winpipe/verify_windows.go, spikes/winpipe/signer.go, spikes/winpipe/signer_test.go, spikes/winpipe/pipe_windows_test.go, spikes/winpipe/cmd/pipeserver/main.go, spikes/winpipe/cmd/pipeclient/main.go, spikes/winpipe/README.md
  Tests: TestSignerProxy_TLS13HandshakeOverLoopback (server private key only reachable through the proxy), TestPipe_SDDLDeniesOtherSID (constructs the descriptor and checks the ACE list), TestPipe_ClientVerifiesServerSID, TestPipe_FirstInstanceRejectsSecondServer (host, local named pipes); spike must demonstrate (user run): a second Windows account cannot open the pipe; handshake overhead of the proxy in ms
  DoD: spike gates green; uses github.com/Microsoft/go-winio v0.6.4; commit `spike(win): pipe ACLs and TLS signing proxy [FEAT-177]`

- [ ] [P0-SP1-006] Manual script for SP1 [FEAT-177] [tier:haiku] [owner:SP1] [brief:briefs/streams/p0-sp1.md#item-p0-sp1-006]
  Needs: P0-SP1-001, P0-SP1-002, P0-SP1-003, P0-SP1-004, P0-SP1-005
  Files: qa/manual/p0-sp1-windows.md
  Tests: script follows `qa/manual/README.md` format; every spike README's run steps appear in order with expected observations and the exact output to paste; safety notes (how to stop each spike, how to uninstall the service)
  DoD: binaries prebuilt into spikes/bin/ by the agent (build allowed, not run) and the script references them; commit `docs(qa): SP1 manual spike script [FEAT-177]`

- [ ] [P0-SP1-M01] Run the SP1 spikes on a Windows PC and record results [FEAT-177] [tier:haiku] [owner:USER] [brief:briefs/streams/p0-sp1.md#item-p0-sp1-m01]
  Needs: P0-SP1-006
  Files: qa/manual/results/p0-sp1-windows.md
  Tests: every step of qa/manual/p0-sp1-windows.md has a recorded observation
  DoD: results file committed by the parent `docs(qa): SP1 spike results [FEAT-177]`

- [ ] [P0-SP1-007] SP1 findings: go/no-go per mechanism [FEAT-177] [tier:sonnet why:synthesis of measurements into design recommendations] [owner:SP1] [brief:briefs/streams/p0-sp1.md#item-p0-sp1-007]
  Needs: P0-SP1-M01
  Files: briefs/spikes/sp1.md
  Tests: covers hooks under GC, watchdog, Raw Input vs re-centre, cursor hiding, SendInput absolute/scancode, DPI + edges, service/helper/token, desktop following, UIPI, SendSAS + policy, pre-login, pipe SDDL, signing proxy; each with go/no-go, numbers, and the decision it confirms or contradicts (D-41, D-47, D-50, D-63, D-64)
  DoD: no recommendation silently changes a decision (contradictions are listed under "Requires user decision"); commit `docs(spikes): SP1 findings [FEAT-177]`

### SP2 - Windows COM shell (virtual files, delayed rendering, OLE drag-and-drop)

- [ ] [P0-SP2-001] Pure-Go COM infrastructure: vtables, handle table, STA thread [FEAT-177, FEAT-103] [tier:sonnet why:hand-built COM vtables, reference counting and STA threading in pure Go] [owner:SP2] [brief:briefs/streams/p0-sp2.md#item-p0-sp2-001]
  Needs: P0-F-005
  Files: spikes/internal/wincom/vtable_windows.go, spikes/internal/wincom/unknown_windows.go, spikes/internal/wincom/dataobject_windows.go, spikes/internal/wincom/enumformatetc_windows.go, spikes/internal/wincom/stream_windows.go, spikes/internal/wincom/dropsource_windows.go, spikes/internal/wincom/droptarget_windows.go, spikes/internal/wincom/sta_windows.go, spikes/internal/wincom/handles.go, spikes/internal/wincom/handles_test.go, spikes/internal/wincom/com_windows_test.go
  Tests: TestHandles_AddGetRelease, TestHandles_ConcurrentAccess (host); TestCOM_QueryInterfaceAddRefRelease, TestCOM_EnumFormatEtcNextSkipReset, TestCOM_IStreamReadSeekStat (host, calls through the vtables in-process via syscall.SyscallN); callbacks created once per method (count asserted <= 64)
  DoD: spike gates green; commit `spike(win): pure-Go COM infrastructure [FEAT-177]`

- [ ] [P0-SP2-002] Virtual-file clipboard spike (FILEGROUPDESCRIPTORW + FILECONTENTS IStream) [FEAT-177, FEAT-103] [tier:sonnet why:IDataObject GetData semantics and Explorer interaction for virtual files] [owner:SP2] [brief:briefs/streams/p0-sp2.md#item-p0-sp2-002]
  Needs: P0-SP2-001
  Files: spikes/winvfiles/main.go, spikes/winvfiles/descriptor_windows.go, spikes/winvfiles/source.go, spikes/winvfiles/source_test.go, spikes/winvfiles/README.md
  Tests: TestSource_ThrottledReadRespectsRate, TestSource_SeekAndRange (host); spike must demonstrate (user run): Explorer paste of a folder + 200 MB file + small file from a throttled synthetic source; Explorer progress UI shows sizes; cancel in Explorer stops reads; log of every IDataObject call (format, lindex, tymed, aspect) incl. PERFORMEDDROPEFFECT/PASTESUCCEEDED; Explorer responsiveness while Read blocks 3 s; behaviour after the spike exits with and without OleFlushClipboard
  DoD: spike gates green; commit `spike(win): virtual files on the clipboard [FEAT-177]`

- [ ] [P0-SP2-003] Delayed-rendering clipboard spike (text, CF_HTML, RTF, PNG, DIBV5) [FEAT-177, FEAT-043] [tier:haiku] [owner:SP2] [brief:briefs/streams/p0-sp2.md#item-p0-sp2-003]
  Needs: P0-F-005
  Files: spikes/winclip/main.go, spikes/winclip/render_windows.go, spikes/winclip/listener_windows.go, spikes/winclip/cfhtml.go, spikes/winclip/cfhtml_test.go, spikes/winclip/dib.go, spikes/winclip/dib_test.go, spikes/winclip/README.md
  Tests: TestCFHTML_BuildOffsetsMatch, TestCFHTML_ParseFragment, TestCFHTML_RoundTripUnicode, TestDIB_PNGRoundTrip32bpp, TestDIB_BottomUpAndTopDown (host); spike must demonstrate: WM_CLIPBOARDUPDATE with sequence number + owner on local copies (own offers ignored), delayed WM_RENDERFORMAT with a 2 s artificial delay for each format pasted into Notepad, WordPad/Word, Paint; WM_RENDERALLFORMATS on exit; detection of `ExcludeClipboardContentFromMonitorProcessing` and `CanIncludeInClipboardHistory` when copying from a password manager or from the spike's own `-sensitive` mode
  DoD: spike gates green; commit `spike(win): delayed clipboard rendering [FEAT-177]`

- [ ] [P0-SP2-004] OLE drag-and-drop spike: edge drop target and virtual-file drag source [FEAT-177, FEAT-104, FEAT-105] [tier:sonnet why:OLE drag-drop modal loop and IDropTarget/IDropSource semantics] [owner:SP2] [brief:briefs/streams/p0-sp2.md#item-p0-sp2-004]
  Needs: P0-SP2-002
  Files: spikes/windnd/main.go, spikes/windnd/edgetarget_windows.go, spikes/windnd/source_windows.go, spikes/windnd/README.md
  Tests: spike must demonstrate (user run): dragging files from Explorer onto a 2 px transparent topmost window on the primary monitor's right edge fires DragEnter and the spike prints CF_HDROP paths and the FILEGROUPDESCRIPTORW entries; the edge window is click-through when no drag is active; pressing the left button on the spike's source window starts DoDragDrop with the SP2-002 virtual-file IDataObject and dropping on an Explorer folder and on the desktop materialises the files; behaviour when Explorer is elevated (UIPI) recorded
  DoD: spike gates green; commit `spike(win): OLE drag-and-drop edge target and source [FEAT-177]`

- [ ] [P0-SP2-005] Gio window HWND file-drop spike [FEAT-177, FEAT-106] [tier:haiku] [owner:SP2] [brief:briefs/streams/p0-sp2.md#item-p0-sp2-005]
  Needs: P0-SP2-001
  Files: spikes/giodrop/main.go, spikes/giodrop/drop_windows.go, spikes/giodrop/README.md
  Tests: spike must demonstrate (user run): HWND obtained from Gio's Win32 view event; IDropTarget registered on Gio's window thread; dropped paths and the drop point converted to Gio coordinates are shown in the window; no conflict with Gio's own OLE/COM initialisation
  DoD: spike gates green with `CGO_ENABLED=0`; commit `spike(gui): native file drop onto a Gio window on Windows [FEAT-177]`

- [ ] [P0-SP2-006] Manual script for SP2 [FEAT-177] [tier:haiku] [owner:SP2] [brief:briefs/streams/p0-sp2.md#item-p0-sp2-006]
  Needs: P0-SP2-002, P0-SP2-003, P0-SP2-004, P0-SP2-005
  Files: qa/manual/p0-sp2-windows-shell.md
  Tests: script follows `qa/manual/README.md`; every spike's steps, expected observations and paste-back outputs present
  DoD: binaries prebuilt into spikes/bin/; commit `docs(qa): SP2 manual spike script [FEAT-177]`

- [ ] [P0-SP2-M01] Run the SP2 spikes on a Windows PC and record results [FEAT-177] [tier:haiku] [owner:USER] [brief:briefs/streams/p0-sp2.md#item-p0-sp2-m01]
  Needs: P0-SP2-006
  Files: qa/manual/results/p0-sp2-windows-shell.md
  Tests: every step has a recorded observation
  DoD: results committed by the parent `docs(qa): SP2 spike results [FEAT-177]`

- [ ] [P0-SP2-007] SP2 findings: go/no-go per mechanism [FEAT-177] [tier:sonnet why:synthesis of COM/OLE observations into the P2 W2 and P4 F1/F2 designs] [owner:SP2] [brief:briefs/streams/p0-sp2.md#item-p0-sp2-007]
  Needs: P0-SP2-M01
  Files: briefs/spikes/sp2.md
  Tests: covers COM infrastructure, virtual-file paste, Explorer progress/cancel, blocking behaviour, delayed rendering per format, sensitivity formats, edge drop target, drag source, UIPI, Gio drop; each with go/no-go and the decision it confirms or contradicts (D-11, D-12, D-13, D-40, D-61)
  DoD: contradictions listed under "Requires user decision"; commit `docs(spikes): SP2 findings [FEAT-177]`

### SP3 - Linux (Wayland portals + EI, X11, file paste, DnD, GNOME indicator)

- [ ] [P0-SP3-001] Pure-Go EI wire codec spike [FEAT-177, FEAT-073] [tier:sonnet why:binary protocol implementation from the EI protocol specification] [owner:SP3] [brief:briefs/streams/p0-sp3.md#item-p0-sp3-001]
  Needs: P0-F-005
  Files: spikes/linux/ei/wire.go, spikes/linux/ei/wire_test.go, spikes/linux/ei/handshake.go, spikes/linux/ei/objects.go, spikes/linux/ei/client.go, spikes/linux/ei/client_test.go, spikes/linux/ei/fd_linux.go, spikes/linux/ei/README.md
  Tests: TestWire_HeaderEncodeDecode, TestWire_GoldenHandshakeFrames, TestWire_RejectsShortAndOversized, TestClient_HandshakeAgainstScriptedServer (net.Pipe, host); FuzzDecodeFrame (30 s smoke)
  DoD: spike gates green incl. `GOOS=linux` vet; protocol source cited (libei protocol XML, MIT) in README; commit `spike(linux): EI wire codec [FEAT-177]`

- [ ] [P0-SP3-002] InputCapture and RemoteDesktop portal spike [FEAT-177, FEAT-074, FEAT-075, FEAT-076, FEAT-077] [tier:sonnet why:asynchronous portal Request/Response flows over D-Bus with fd passing] [owner:SP3] [brief:briefs/streams/p0-sp3.md#item-p0-sp3-002]
  Needs: P0-SP3-001
  Files: spikes/linux/portal/request.go, spikes/linux/portal/inputcapture.go, spikes/linux/portal/remotedesktop.go, spikes/linux/portal/session.go, spikes/linux/portal/request_test.go, spikes/linux/cmd/wlcapture/main.go, spikes/linux/cmd/wlinject/main.go, spikes/linux/portal/README.md
  Tests: TestRequest_HandleTokenPathDerivation, TestRequest_ResponseDecoding (host); spike must demonstrate on GNOME (user run): portal interface versions; zones printed; barrier on the right edge activates capture; relative motion, buttons, keys and scroll arrive over EI; Release(activation_id, cursor_position) returns the pointer; session recreation after a barrier change; restore token persisted and reused (second run without dialog if the backend supports it); injection of a pointer square + typed text via RemoteDesktop + EI
  DoD: spike gates green incl. `GOOS=linux` vet; uses github.com/godbus/dbus/v5 v5.2.4; commit `spike(linux): InputCapture and RemoteDesktop portals [FEAT-177]`

- [ ] [P0-SP3-003] Portal clipboard spike [FEAT-177, FEAT-078] [tier:haiku] [owner:SP3] [brief:briefs/streams/p0-sp3.md#item-p0-sp3-003]
  Needs: P0-SP3-002
  Files: spikes/linux/portal/clipboard.go, spikes/linux/cmd/wlclip/main.go, spikes/linux/cmd/wlclip/README.md
  Tests: spike must demonstrate on GNOME (user run): RequestClipboard on a RemoteDesktop session and on an InputCapture session (record which works); SetSelection with text/plain;charset=utf-8, text/html, image/png, text/uri-list, x-special/gnome-copied-files; SelectionTransfer served after a configurable delay with paste behaviour in Text Editor and Nautilus; SelectionOwnerChanged + SelectionRead of each MIME for a local copy; whether clipboard access stops when the session is inactive
  DoD: spike gates green incl. `GOOS=linux` vet; commit `spike(linux): portal clipboard [FEAT-177]`

- [ ] [P0-SP3-004] X11 spike with jezek/xgb: grab, deltas, XTest, RandR, XFixes, INCR [FEAT-177, FEAT-063, FEAT-064, FEAT-065, FEAT-067] [tier:haiku] [owner:SP3] [brief:briefs/streams/p0-sp3.md#item-p0-sp3-004]
  Needs: P0-F-005
  Files: spikes/linux/x11/conn.go, spikes/linux/x11/grab.go, spikes/linux/x11/inject.go, spikes/linux/x11/randr.go, spikes/linux/x11/selection.go, spikes/linux/x11/x11_integration_test.go, spikes/linux/cmd/x11demo/main.go, spikes/linux/x11/README.md
  Tests (integration, Xvfb `:99` with two RandR monitors set via `xrandr --setmonitor`): TestXTest_MotionObservedByOwnWindow, TestGrab_WarpToCentreDeltas, TestXFixes_SelectionNotifyOnForeignOwner, TestSelection_INCRTransfer5MiB, TestRandR_MonitorsListed
  DoD: spike gates green incl. `GOOS=linux` vet; uses github.com/jezek/xgb v1.3.1; commit `spike(linux): X11 capture, injection and selections [FEAT-177]`

- [ ] [P0-SP3-005] XInput2/XKB binding generation assessment [FEAT-177, FEAT-063] [tier:sonnet why:code-generation feasibility assessment with xgbgen on xcb-proto] [owner:SP3] [brief:briefs/streams/p0-sp3.md#item-p0-sp3-005]
  Needs: P0-SP3-004
  Files: spikes/linux/x11gen/gen.sh, spikes/linux/x11gen/README.md, spikes/linux/x11gen/xinput/ (generated), spikes/linux/x11/xi2_integration_test.go
  Tests: generated package compiles with `GOOS=linux`; TestXI2_RawMotionOnXvfb (integration) receives XI_RawMotion while a grab is active, or the README records the exact generation failure
  DoD: spike gates green; xcb-proto version + licence (MIT) recorded; commit `spike(linux): XI2 binding generation assessment [FEAT-177]`

- [ ] [P0-SP3-006] Linux lazy file-paste behaviour of file managers [FEAT-177, FEAT-110, FEAT-113] [tier:haiku] [owner:SP3] [brief:briefs/streams/p0-sp3.md#item-p0-sp3-006]
  Needs: P0-SP3-003, P0-SP3-004
  Files: spikes/linux/cmd/filepaste/main.go, spikes/linux/cmd/filepaste/README.md
  Tests: spike must demonstrate (user run): X11 mode (Xephyr nested server or XWayland app) and portal mode offer text/uri-list + x-special/gnome-copied-files; the reply is delayed 5 s, 30 s and 120 s while a synthetic file is written to staging; record for Nautilus whether paste waits, shows progress, times out (and after how long), and whether the pasted file is complete
  DoD: spike gates green incl. `GOOS=linux` vet; commit `spike(linux): file manager lazy paste behaviour [FEAT-177]`

- [ ] [P0-SP3-007] Linux drag-and-drop, GNOME switch indicator and Gio file-drop feasibility [FEAT-177, FEAT-079, FEAT-111, FEAT-114, FEAT-115] [tier:sonnet why:protocol research across XDND, Wayland data-device and GNOME surface constraints] [owner:SP3] [brief:briefs/streams/p0-sp3.md#item-p0-sp3-007]
  Needs: P0-SP3-002, P0-SP3-004
  Files: spikes/linux/cmd/dndx11/main.go, spikes/linux/cmd/wloverlay/main.go, spikes/linux/cmd/giodroplinux/main.go, spikes/linux/dnd/README.md
  Tests: spike must demonstrate (user run): XDND source and target between the spike and a file manager inside Xephyr; on GNOME Wayland, candidates for the switch indicator (Notification portal `AddNotification`, an undecorated Gio window) with observed placement/stacking; whether an InputCapture activation occurs while a drag is in progress; whether a Gio window on Wayland can accept file drops (via Gio APIs or its view handles); README contains the written analysis of Wayland incoming-drag initiation (implicit grab serial requirement) with citations
  DoD: spike gates green for the non-cgo parts; the Gio Linux spike builds on the GNOME box per README; commit `spike(linux): DnD, GNOME indicator and Gio drop feasibility [FEAT-177]`

- [ ] [P0-SP3-008] Manual script for SP3 [FEAT-177] [tier:haiku] [owner:SP3] [brief:briefs/streams/p0-sp3.md#item-p0-sp3-008]
  Needs: P0-SP3-002, P0-SP3-003, P0-SP3-004, P0-SP3-005, P0-SP3-006, P0-SP3-007
  Files: qa/manual/p0-sp3-linux.md
  Tests: script follows `qa/manual/README.md`; includes GNOME box preparation (Go 1.27.3 install, packages for Xephyr/Xvfb and Gio build deps, GNOME and xdg-desktop-portal versions to record), every spike's steps, expected observations and paste-back outputs; the X11 integration tests can run locally under Xvfb or via the spikes workflow
  DoD: commit `docs(qa): SP3 manual spike script [FEAT-177]`

- [ ] [P0-SP3-M01] Run the SP3 Wayland spikes on the GNOME box and record results [FEAT-177] [tier:haiku] [owner:USER] [brief:briefs/streams/p0-sp3.md#item-p0-sp3-m01]
  Needs: P0-SP3-008
  Files: qa/manual/results/p0-sp3-linux.md
  Tests: every Wayland step has a recorded observation incl. GNOME, mutter and xdg-desktop-portal versions
  DoD: results committed by the parent `docs(qa): SP3 Wayland spike results [FEAT-177]`

- [ ] [P0-SP3-M02] Run the SP3 X11 integration tests (Xvfb on the GNOME box or the spikes workflow) and record results [FEAT-177] [tier:haiku] [owner:USER] [brief:briefs/streams/p0-sp3.md#item-p0-sp3-m02]
  Needs: P0-SP3-008
  Files: qa/manual/results/p0-sp3-linux.md (X11 section)
  Tests: test output of `go -C spikes test -tags integration ./linux/x11/...` recorded, plus Xephyr observations for dndx11 and filepaste X11 mode
  DoD: results committed by the parent `docs(qa): SP3 X11 spike results [FEAT-177]`

- [ ] [P0-SP3-009] SP3 findings: go/no-go per mechanism [FEAT-177] [tier:sonnet why:synthesis of Linux observations into the P2 X1/Y1 and P4 F4/F5 designs] [owner:SP3] [brief:briefs/streams/p0-sp3.md#item-p0-sp3-009]
  Needs: P0-SP3-M01, P0-SP3-M02
  Files: briefs/spikes/sp3.md
  Tests: covers EI codec, InputCapture (zones, barriers, activation, release, recreation, restore tokens), RemoteDesktop injection, portal clipboard and session-activity dependence, X11 capture/injection/selections, XI2 generation, lazy file paste mechanism recommendation for X11 and Wayland, XDND, Wayland DnD limits, GNOME switch indicator, Gio Linux drop, guard semantics under barrier activation; each with go/no-go and the decision it confirms or contradicts (D-02, D-05, D-11, D-12, D-18, D-39)
  DoD: contradictions listed under "Requires user decision"; commit `docs(spikes): SP3 findings [FEAT-177]`

### SP4 - macOS via purego (compile + CI only)

- [x] [P0-SP4-001] purego framework bindings and display enumeration [FEAT-177, FEAT-055] [tier:haiku] [owner:SP4] [brief:briefs/streams/p0-sp4.md#item-p0-sp4-001]
  Needs: P0-F-005
  Files: spikes/macos/fw/frameworks_darwin.go, spikes/macos/fw/cg_darwin.go, spikes/macos/fw/cf_darwin.go, spikes/macos/fw/objc_darwin.go, spikes/macos/cmd/displays/main.go, spikes/macos/fw/displays_integration_test.go, spikes/macos/README.md
  Tests: `GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go -C spikes build ./macos/...` and `GOARCH=amd64` on the host; TestDisplays_ListBoundsAndScale (integration, CI macOS) incl. a `CGWarpMouseCursorPosition` call with CGPoint by value followed by a position read-back
  DoD: spike gates green; uses github.com/ebitengine/purego v0.11.2; commit `spike(macos): purego bindings and displays [FEAT-177]`

- [ ] [P0-SP4-002] CGEventTap on a locked CFRunLoop thread with re-enable handling [FEAT-177, FEAT-052, FEAT-053, FEAT-056] [tier:sonnet why:purego callbacks on a CFRunLoop thread and tap disable/re-enable semantics] [owner:SP4] [brief:briefs/streams/p0-sp4.md#item-p0-sp4-002]
  Needs: P0-SP4-001
  Files: spikes/macos/tap/tap_darwin.go, spikes/macos/tap/runloop_darwin.go, spikes/macos/tap/permissions_darwin.go, spikes/macos/tap/tap_integration_test.go, spikes/macos/cmd/tap/main.go
  Tests: TestTap_CreateSessionTapReportsTrustState (integration, CI): logs `AXIsProcessTrusted`, `CGPreflightListenEventAccess`, `CGPreflightPostEventAccess`, whether an active session tap and a listen-only tap can be created; when created, synthetic events posted by the test are observed and timeout re-enable code path executes
  DoD: spike gates green; commit `spike(macos): event tap [FEAT-177]`

- [ ] [P0-SP4-003] Event injection via CGEventPost [FEAT-177, FEAT-054] [tier:haiku] [owner:SP4] [brief:briefs/streams/p0-sp4.md#item-p0-sp4-003]
  Needs: P0-SP4-001
  Files: spikes/macos/inject/inject_darwin.go, spikes/macos/inject/inject_integration_test.go, spikes/macos/cmd/inject/main.go
  Tests: TestInject_MoveClickKeyScroll (integration, CI): posts move with delta fields, click with click state and event number fields, key down/up with keycode and modifier flags, scroll via `CGEventCreateScrollWheelEvent2` (non-variadic); verifies via cursor position read-back and, when trusted, a listen-only tap
  DoD: spike gates green; commit `spike(macos): event injection [FEAT-177]`

- [ ] [P0-SP4-004] Lazy pasteboard provider and file promises via purego objc [FEAT-177, FEAT-057, FEAT-107] [tier:sonnet why:Objective-C class registration implementing NSPasteboardItemDataProvider and NSFilePromiseProviderDelegate via purego] [owner:SP4] [brief:briefs/streams/p0-sp4.md#item-p0-sp4-004]
  Needs: P0-SP4-001
  Files: spikes/macos/pb/pasteboard_darwin.go, spikes/macos/pb/provider_darwin.go, spikes/macos/pb/promise_darwin.go, spikes/macos/pb/pb_integration_test.go, spikes/macos/cmd/pb/main.go
  Tests (integration, CI): TestPasteboard_ChangeCountPolling, TestProvider_LazyTextHTMLRTFPNG (write provider-backed item, read back in-process, provider called once per type), TestConcealedType_Detected, TestPromise_ProviderRegistersAndWritesFile (promise delegate writes to a temp dir when fulfilled programmatically)
  DoD: spike gates green; README states plainly which behaviours remain unverifiable without a Mac (Finder paste of promises); commit `spike(macos): lazy pasteboard and file promises [FEAT-177]`

- [ ] [P0-SP4-005] NSStatusItem tray, overlay panel and minimal .app bundle [FEAT-177, FEAT-059, FEAT-060, FEAT-164] [tier:sonnet why:AppKit run-loop ownership from a Go program via purego] [owner:SP4] [brief:briefs/streams/p0-sp4.md#item-p0-sp4-005]
  Needs: P0-SP4-001
  Files: spikes/macos/ui/app_darwin.go, spikes/macos/ui/statusitem_darwin.go, spikes/macos/ui/overlay_darwin.go, spikes/macos/cmd/trayoverlay/main.go, spikes/macos/bundle/Info.plist, spikes/macos/bundle/make-bundle.sh
  Tests (CI): bundle built and ad-hoc signed (`codesign -s - --force`), launched for 5 s, process alive, no crash report, `screencapture` artifact uploaded showing the status item and overlay
  DoD: spike gates green; Info.plist has `LSUIElement`, `NSAccessibilityUsageDescription`-style usage strings verified against Apple docs at execution time; commit `spike(macos): tray, overlay and app bundle [FEAT-177]`

- [ ] [P0-SP4-006] macOS CI steps and manual dispatch script [FEAT-177] [tier:haiku] [owner:SP4] [brief:briefs/streams/p0-sp4.md#item-p0-sp4-006]
  Needs: P0-SP4-002, P0-SP4-003, P0-SP4-004, P0-SP4-005
  Files: .github/workflows/spikes.yml (macos job steps: bundle build/launch, artifacts), qa/manual/p0-sp4-macos.md
  Tests: actionlint clean; script tells the user how to push the branch, dispatch the workflow, and which artifacts/logs to share
  DoD: commit `ci(spikes): macOS spike steps and dispatch script [FEAT-177]`

- [ ] [P0-SP4-M01] Push and dispatch the spikes workflow; share the macOS job log and artifacts [FEAT-177] [tier:haiku] [owner:USER] [brief:briefs/streams/p0-sp4.md#item-p0-sp4-m01]
  Needs: P0-SP4-006
  Files: qa/manual/results/p0-sp4-macos.md
  Tests: run URL, test output, screenshots recorded
  DoD: results committed by the parent `docs(qa): SP4 CI results [FEAT-177]`

- [ ] [P0-SP4-007] SP4 findings: go/no-go per mechanism [FEAT-177] [tier:sonnet why:synthesis of CI-only evidence into the P2 M1 and P4 F3 designs with explicit unverifiable items] [owner:SP4] [brief:briefs/streams/p0-sp4.md#item-p0-sp4-007]
  Needs: P0-SP4-M01
  Files: briefs/spikes/sp4.md
  Tests: covers purego bindings, CGPoint by value, taps (session vs HID), TCC on CI, injection fields, lazy provider, concealed type, file promises (verifiable vs not), status item, overlay, bundle; each with go/no-go, evidence level (CI-verified / compile-only / unverifiable) and the decision it confirms or contradicts (D-11, D-33, D-34, D-39)
  DoD: contradictions and unverifiable-but-required behaviours listed under "Requires user decision"; commit `docs(spikes): SP4 findings [FEAT-177]`

### SP5 - Core tech (Gio editor, QUIC, mDNS, Cap'n Proto, SPAKE2, IPC)

- [x] [P0-SP5-001] Gio layout-editor proof [FEAT-177, FEAT-147] [tier:sonnet why:Gio immediate-mode pointer and drag model drives the P3 editor design] [owner:SP5] [brief:briefs/streams/p0-sp5.md#item-p0-sp5-001]
  Needs: P0-F-005
  Files: spikes/gioeditor/main.go, spikes/gioeditor/canvas.go, spikes/gioeditor/snap.go, spikes/gioeditor/snap_test.go, spikes/gioeditor/README.md
  Tests: TestSnap_ToNeighbourEdgeWithinThreshold, TestSnap_NoSnapBeyondThreshold, TestSnap_PrefersClosestEdge, TestSnap_OverlapSegment (host); spike must demonstrate (user run): drag of multi-monitor machine groups, snapping within 12 logical px, overlap highlight, Ctrl+wheel zoom, middle-drag pan, arrow-key nudge (Shift = 10 px), light/dark palette toggle, frame-time log with `-machines 64`
  DoD: spike gates green; builds with `CGO_ENABLED=0` on Windows; uses gioui.org v0.10.3; commit `spike(gui): Gio layout editor proof [FEAT-177]`

- [ ] [P0-SP5-002] quic-go latency and priority bench with TCP comparison [FEAT-177, FEAT-117, FEAT-118, FEAT-170] [tier:haiku] [owner:SP5] [brief:briefs/streams/p0-sp5.md#item-p0-sp5-002]
  Needs: P0-F-005
  Files: spikes/quicbench/peer.go, spikes/quicbench/bench_test.go, spikes/quicbench/cmd/quicbench/main.go, spikes/quicbench/README.md
  Tests (host, loopback unicast only): TestDatagramLatency_Idle, TestDatagramLatency_UnderBulk (urgency 7 stream saturating), TestInputStreamLatency_UnderBulk (urgency 0 small frames), TestTCPFallbackLatency_UnderBulk (two TLS connections); each logs p50/p99/max over 10 s at 1000 Hz; cmd/quicbench provides an echo-based LAN mode for the user's two PCs
  DoD: spike gates green; numbers copied into README; uses quic-go v0.64.1; commit `spike(net): QUIC datagram and priority latency bench [FEAT-177]`

- [ ] [P0-SP5-003] mDNS discovery spike (hashicorp/mdns vs brutella/dnssd) [FEAT-177, FEAT-129] [tier:haiku] [owner:SP5] [brief:briefs/streams/p0-sp5.md#item-p0-sp5-003]
  Needs: P0-F-005
  Files: spikes/mdns/cmd/mdnsprobe/main.go, spikes/mdns/README.md
  Tests: spike must demonstrate (user run on 2 Windows PCs + GNOME box, incl. machines with WireGuard/Netbird adapters): advertise `_edgesync._udp` with TXT keys from D-43, browse, time-to-first-discovery, interfaces used, behaviour with VPN adapters, Windows firewall prompt; `-lib=hashicorp|dnssd`
  DoD: spike gates green; uses hashicorp/mdns v1.0.7 and brutella/dnssd v1.2.14; commit `spike(net): mDNS discovery comparison [FEAT-177]`

- [ ] [P0-SP5-004] Cap'n Proto toolchain and go-capnp API spike [FEAT-177, FEAT-120] [tier:haiku] [owner:SP5] [brief:briefs/streams/p0-sp5.md#item-p0-sp5-004]
  Needs: P0-F-002, P0-F-006
  Files: spikes/capnp/schema/handshake.capnp, spikes/capnp/schema/clipboard.capnp, spikes/capnp/gen.sh, spikes/capnp/gen/ (generated), spikes/capnp/roundtrip_test.go, spikes/capnp/limits_test.go, spikes/capnp/fuzz_test.go, spikes/capnp/bench_test.go, spikes/capnp/README.md
  Tests: TestRoundTrip_Hello, TestRoundTrip_ClipAnnounce, TestLimits_TraversalBombRejected, TestLimits_DepthLimitRejected, TestLimits_OversizedMessageRejected, FuzzDecodeHello (30 s), BenchmarkEncodeHello/BenchmarkDecodeClipAnnounce (allocs/op reported)
  DoD: spike gates green; exact generation command recorded; commit `spike(proto): Cap'n Proto toolchain and limits [FEAT-177]`

- [ ] [P0-SP5-005] SPAKE2 (RFC 9382, P-256) feasibility with official test vectors and TLS exporter binding [FEAT-177, FEAT-126, FEAT-127] [tier:sonnet why:cryptographic protocol implementation from RFC 9382 incl. M/N constants and key schedule] [owner:SP5] [brief:briefs/streams/p0-sp5.md#item-p0-sp5-005]
  Needs: P0-F-005
  Files: spikes/spake2/spake2.go, spikes/spake2/spake2_test.go, spikes/spake2/exporter_test.go, spikes/spake2/bench_test.go, spikes/spake2/README.md
  Tests: TestRFC9382_VectorsP256SHA256 (every vector in RFC 9382 Appendix B for P-256 reproduced byte for byte), TestSPAKE2_WrongPasswordFailsConfirmation, TestSPAKE2_RejectsIdentityAndOffCurvePoints, TestExporter_BindingDiffersPerConnection (TLS 1.3 loopback), BenchmarkSPAKE2Full
  DoD: spike gates green; uses filippo.io/nistec v0.0.4; RFC text cited by section; commit `spike(crypto): SPAKE2 RFC 9382 feasibility [FEAT-177]`

- [ ] [P0-SP5-006] Agent IPC transport spike (named pipe + unix socket peer credentials) [FEAT-177, FEAT-132] [tier:haiku] [owner:SP5] [brief:briefs/streams/p0-sp5.md#item-p0-sp5-006]
  Needs: P0-F-005
  Files: spikes/ipc/pipe_windows.go, spikes/ipc/pipe_windows_test.go, spikes/ipc/sock_unix.go, spikes/ipc/sock_unix_test.go, spikes/ipc/README.md
  Tests: TestPipe_OwnerOnlySDDL, TestPipe_RoundTripLatency1KiB (host, local pipe); TestSock_PeerCredMatchesUID (linux/darwin, compile-checked on host via cross vet, run in the spikes workflow)
  DoD: spike gates green incl. cross vet; commit `spike(ipc): pipe and socket peer verification [FEAT-177]`

- [ ] [P0-SP5-007] Manual script for SP5 [FEAT-177] [tier:haiku] [owner:SP5] [brief:briefs/streams/p0-sp5.md#item-p0-sp5-007]
  Needs: P0-SP5-001, P0-SP5-002, P0-SP5-003
  Files: qa/manual/p0-sp5-core.md
  Tests: script follows `qa/manual/README.md`; covers the Gio editor feel test, mdnsprobe on all three machines, quicbench LAN mode between the two Windows PCs (wired)
  DoD: binaries prebuilt into spikes/bin/; commit `docs(qa): SP5 manual spike script [FEAT-177]`

- [ ] [P0-SP5-M01] Run the SP5 manual steps and record results [FEAT-177] [tier:haiku] [owner:USER] [brief:briefs/streams/p0-sp5.md#item-p0-sp5-m01]
  Needs: P0-SP5-007
  Files: qa/manual/results/p0-sp5-core.md
  Tests: every step has a recorded observation
  DoD: results committed by the parent `docs(qa): SP5 spike results [FEAT-177]`

- [ ] [P0-SP5-008] SP5 findings: go/no-go per mechanism [FEAT-177] [tier:sonnet why:synthesis of measurements into the P1 and P3 designs] [owner:SP5] [brief:briefs/streams/p0-sp5.md#item-p0-sp5-008]
  Needs: P0-SP5-004, P0-SP5-005, P0-SP5-006, P0-SP5-M01
  Files: briefs/spikes/sp5.md
  Tests: covers Gio editor feasibility + frame times, QUIC datagram/stream latency incl. under bulk vs TCP fallback, mDNS library choice, capnp toolchain + limits + allocs, SPAKE2 vectors + timing, IPC verification; each with go/no-go, numbers, and the decision it confirms or contradicts (D-14, D-15, D-33, D-35, D-36, D-46)
  DoD: contradictions listed under "Requires user decision"; commit `docs(spikes): SP5 findings [FEAT-177]`

### P0-X - Phase exit

- [ ] [P0-X-001] Consolidate spike findings and propose superseding decisions [FEAT-177] [tier:opus] [owner:PARENT] [brief:briefs/streams/p0-f.md#item-p0-x-001]
  Needs: P0-SP1-007, P0-SP2-007, P0-SP3-009, P0-SP4-007, P0-SP5-008
  Files: briefs/spikes/p0-consolidation.md
  Tests: every "Requires user decision" entry from the five findings is listed with options and a recommendation; impacts on P1-P5 items listed (items to append or annotate)
  DoD: commit `docs(spikes): P0 consolidation [FEAT-177]`

- [ ] [P0-X-002] User approves or rejects each proposed decision change [FEAT-177] [tier:haiku] [owner:USER] [brief:briefs/streams/p0-f.md#item-p0-x-002]
  Needs: P0-X-001
  Files: decisions.md (parent appends the approved entries), briefs/plan-phase-1.md .. briefs/plan-phase-5.md (parent appends notes/items per approved changes)
  Tests: every proposal has a recorded answer
  DoD: commit `docs(decisions): record P0 spike outcomes [FEAT-177]`

- [ ] [P0-X-003] Re-verify pinned versions and flip the phase marker to P1 [FEAT-169] [tier:opus] [owner:PARENT] [brief:briefs/streams/p0-f.md#item-p0-x-003]
  Needs: P0-X-002, all other P0 items
  Files: compatibility-matrix.md (append notes), plan.md (append `Current phase: P1` marker note)
  Tests: every pinned module re-checked against proxy.golang.org; any newer version recorded as a note, not adopted without a decision
  DoD: `task ci:local` green; commit `chore(plan): P0 exit [FEAT-169]`
