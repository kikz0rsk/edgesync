# EdgeSync - Phase 2 items

Read `plan.md` (contract, orchestration, item format) first. Stream briefs: `briefs/streams/p2-w1.md`,
`p2-w2.md`, `p2-x1.md`, `p2-y1.md`, `p2-m1.md`. Contracts: `briefs/architecture.md` section 4 (frozen at P1-F).

Phase 2 implements the OS backends behind the frozen `internal/platform` interfaces. There is no foundation block:
W1, W2, X1, Y1 and M1 start as soon as the CONTRACT FREEZE note exists and an item's `Needs:` are `[x]`. Under the
global fleet cap (plan.md contract) P2 items fill the fleet while P1 C1-C5 items are still running.

## Dependency notation

- Every P2 item implicitly needs P0 exit (P0-X-003) and the P1-F CONTRACT FREEZE (P1-F-022).
- P1 prerequisites are cited by item ID. The table below records which P1 items deliver what P2 uses (resolved at the
  planning consolidation on 2026-10-30; it replaces the earlier `P1:<KEY>` notation).
- `spN: <mechanism>` = the mechanism has a "go" verdict in `briefs/spikes/spN.md` as approved in P0-X-002. If the
  approved outcome differs from what the item describes, the subagent reports BLOCKED and the parent asks the user
  (plan is sacred); the item is never adapted silently.
- `D-NN` in `Needs:` = the decision that closed a former spine gap (table "Spine gaps" below); it exists already, so it
  never blocks. `Q-NN decision` = the decision appended when the user answers `briefs/open-questions.md` Q-NN at plan
  approval; the item is blocked until that decision exists (Q-01 .. Q-10 were answered on 2026-10-30: D-112 .. D-121, so the items now cite `D-NN`).

| P1 deliverable | P1 item IDs | P2 use |
|---|---|---|
| CONTRACT FREEZE: `internal/platform/*.go` types + interfaces, `internal/platform/fake`, `internal/platform/conformance`, `internal/platform/host/host.go`, `internal/keymap/usage.go`, `proto/schema/*.capnp` + `internal/proto/capnp` (incl. `service.capnp` ServiceRequest/HelperRequest), `internal/proto/wire`, `internal/config/schema.go` | P1-F-022 (built by P1-F-001 .. P1-F-021) | every item |
| Real-edge computation and crossing mapping in `internal/layout` (`EdgeSegment.At/From/To`, D-88) | P1-C3-006, P1-C3-007 | edge-contact rules |
| `keymap.FromWindowsScan` / `ToWindowsScan`, `FromMacKeycode` / `ToMacKeycode`, `FromEvdev` / `ToEvdev` (`internal/keymap/tables.go`) | P1-C3-003 | W1, W2, M1, X1, Y1 |
| Engine client role (Enter/Leave, frames -> `InputInjector`; `internal/engine/client.go`) | P1-C3-020 | W2 pre-login |
| `internal/clipboard/convert` (text normalisation, CF_HTML wrap/unwrap, RTF validation, DIB/DIBV5/TIFF <-> PNG) | P1-C4-002, P1-C4-003, P1-C4-004, P1-C4-005, P1-C4-006 | all clipboard backends |
| `internal/transport` listener/dialer, session handshake, `internal/session` session object | P1-C1-007, P1-C1-010, P1-C1-013 | W2 pre-login |
| `internal/pki` identity (cert generation/re-issue, pluggable signer) | P1-C2-001 | W2 machine identity |
| `internal/peers` store | P1-C2-009 | W2 pre-login |
| `internal/paths` | P1-C5-001 | state/log/config dirs |
| `internal/logging` (slog setup, rotation) | P1-C5-003 | W2 service |
| Atomic TOML store in `internal/config` (`store.go`) | P1-C5-002 | W2 machine config, Y1 portal tokens |
| `agent.Main` and the composition root (`internal/agent/main.go`, `agent.go`); `cmd/edgesync-agent/main.go` itself is created by P2-X-003 (D-84) | P1-C5-017 | W2 `--secure-helper` dispatch, M1 main thread |

## Standard gates (SG)

Every item's DoD starts with "SG green". SG = the per-item gates of plan.md 1.5 plus the P2 additions:

1. `golangci-lint fmt`, then `golangci-lint run ./...`
2. `go vet ./...` and `go vet -tags integration ./internal/platform/...` (integration tests must compile even though agents never run them)
3. `go build ./...`
4. `go test ./internal/platform/...` (fakes, conformance-over-fakes, host-safe backend tests) plus the item's packages
5. `GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build ./internal/... ./cmd/edgesync-agent/...` and `GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build ./internal/... ./cmd/edgesync-agent/...`
6. Stream OS checks. X1/Y1: `GOOS=linux CGO_ENABLED=0 go vet -tags integration ./internal/platform/...` and `GOOS=linux CGO_ENABLED=0 golangci-lint run ./internal/platform/linux/... ./internal/platform/x11/... ./internal/platform/wayland/... ./internal/platform/host/...`. M1: the same with `GOOS=darwin` (arm64) over `./internal/platform/darwin/... ./internal/platform/host/...`, plus `GOOS=darwin GOARCH=amd64 CGO_ENABLED=0 go build ./internal/platform/darwin/...`. W1/W2: the host is Windows, steps 1-4 cover it.
7. `SG+race` (items with goroutines, channels, atomics or locks): `PATH="/d/Development/msys2/ucrt64/bin:$PATH" CGO_ENABLED=1 CC=gcc go test -race ./<item packages>/...` (host-compiled packages; Linux/macOS race runs in CI).
8. New fuzz targets: `go test -run '^$' -fuzz '^<Name>$' -fuzztime 30s ./<pkg>`.
9. Workflows touched: `go tool -modfile=tools/tools.mod actionlint`.

`HC85` = `go test -coverprofile=c.out ./<pkg>` on the host reports >= 85% of the host-compiled statements of the
package (OS-tagged files are excluded by the build, so this measures the untagged pure logic; the platform
directories stay excluded from `task cover` per `tools/coverage-exclude.txt`).

Agents never run binaries they build and never run `-tags integration` tests. Integration tests run in the CI jobs
added by this phase or on the user's machines through the USER items.

## Spine gaps (closed at the planning consolidation, 2026-10-30)

Every gap below is closed by the decision or open question in the last column; where the decision differs from
the proposal in the middle column, the decision wins (G-14: native coordinate spaces instead of "logical x scale").

| ID | Gap | Proposal at writing time | Closed by |
|---|---|---|---|
| G-01 | D-75 forbids package-level mutable state, but LL hook procs have no context parameter and `syscall.NewCallback` / `purego.NewCallback` are finite and never freed; WNDPROC and COM method trampolines need process-wide dispatch tables | Narrow exception: one trampoline per callback signature, created once (`sync.Once`), plus a mutex/atomic-guarded dispatch table keyed by HWND, `this` pointer or a single active-instance `atomic.Pointer`; nothing else package-level | D-80 |
| G-02 | Sign and units of `InputEvent.DX/DY` for `KindWheel` and of `InputInjector.Wheel` are not stated in architecture section 3 | Follow the doc comment in `internal/platform/types.go` (P1-F); if absent: 120 per notch, `DY > 0` = wheel rotated away from the user (content scrolls up), `DX > 0` = scroll right (Windows convention) | D-87 |
| G-03 | `EdgeSegment.At` (last pixel row/column vs boundary line) is only described as "coordinate of the side line" | Contact rule follows the convention documented and tested in `internal/layout` (P1 C3) | D-88 |
| G-04 | Architecture 6.6 step 4 has the user agent "connect to the helper pipe", while D-47 and `qa/security-review.md` require the helper pipe to be SYSTEM-only | The service opens the SYSTEM-only helper pipe on behalf of an authorised agent and duplicates the client handle into the agent process (`DuplicateHandle`); the agent never opens the helper pipe itself | D-81 |
| G-05 | D-47 whitelists `ReportInputDesktop` without semantics; FEAT-047 says "secure-desktop capture via helper" while D-63 returns control home when the server's input desktop turns secure | `ReportInputDesktop{accessible bool}`: the agent reports when its own input desktop becomes inaccessible/accessible; the service then tells the helper to start/stop forwarding secure-desktop input events; the engine (C3) keeps the D-63 "return home" policy and uses forwarded events only for the panic hotkey and release-all | D-82 |
| G-06 | D-64 pre-login mode reads a machine-scoped peer store, but its location is not in D-69 and D-47 forbids configuration changes via the service pipe, so no path populates it | Needs a user decision (for example `%ProgramData%\EdgeSync\machine-peers.toml` populated by an elevated "Allow control at the sign-in screen" action that is not a service-pipe command) | D-112 (Q-01 answered) |
| G-07 | `machine.toml` (D-69) has no schema | `schema_version = 1`; `[service] helper_enabled = true`; `[prelogin] enabled = true`, `port = 24860`; `[log] level = "info"`; strict decoding, atomic writes, SYSTEM+Administrators write ACL | D-83 |
| G-08 | A physical Ctrl+Alt+Del on a Windows server never reaches the hooks, so a Windows server user has no way to send SAS to a client | W2 implements client-side detection of the Ctrl+Alt+Delete chord (reachable from macOS/Linux servers); a Windows-server trigger (tray action or Ctrl+Alt+End) needs a decision | D-113 (Q-02 answered) |
| G-09 | AppKit (tray, overlay, pasteboard provider callbacks) needs the process main thread, but `cmd/edgesync-agent` (C5) owns `main` and `internal/platform/host/host.go` (P1-F) only has `Open` | Add `host.RunMain(run func() error) error` (all OSes; darwin hands the main thread to `NSApplication` and runs `run` on a goroutine, others call `run` directly) and make C5's `main` call it | D-84 |
| G-10 | `cmd/edgesync-agent/main.go` (C5) must dispatch `--secure-helper` to W2's `cmd/edgesync-agent/helper_windows.go` | W2-025 expects `main` to call `secureHelperMain(ctx, args) error` when the flag is present; if C5's `main` lacks the call, the parent appends a claim item on `main.go` | D-84 (new item P2-X-003) |
| G-11 | The X11 switch overlay must draw text, and the stdlib has no font rasteriser | New dependency `golang.org/x/image` v0.47.0 (BSD-3, tag 2026-10-09, `go 1.26.0`, requires `x/sys` v0.49.0 + `x/text` v0.43.0): `font/opentype`, `font/gofont/goregular`, `vector` | D-85 |
| G-12 | SP3 tested grabs and warp-to-centre deltas but not how to observe pointer position globally without a grab (needed for edge contacts and hotkeys while home) | RECORD extension (`jezek/xgb/record`) for passive observation; confirm, or choose XI2 if sp3.md generated working XI2 bindings | D-92 |
| G-13 | D-57 has no macOS pasteboard poll interval | Poll `changeCount` every 500 ms | D-86 |
| G-14 | macOS (points) and Wayland (logical zones) have no global physical-pixel space; per-monitor `Bounds` = logical x scale can leave gaps between mixed-scale monitors | M1/Y1 report `Bounds` = logical rect x that monitor's scale; C3 must compute real edges and adjacency in canvas (logical) units; injectors map points back per monitor and reject points outside every monitor | D-89 (supersedes the proposal in this row) |
| G-15 | Wayland servers receive input only during a portal activation: hotkeys cannot be observed or swallowed while the pointer is home, and `Grab` without a barrier activation is impossible (no jump/cycle from home) | `Grab` without a pending activation and home-side swallowing return an error wrapping `platform.ErrUnsupported`; the limitation is shown in the GUI and docs | D-114 (Q-03 answered) |
| G-16 | macOS has no public "lock screen now" API | `SACLockScreenImmediate` from the private `login.framework`, explicit error when absent; new risk entry | D-115 (Q-04 answered) |
| G-17 | After a barrier activation the Wayland backend needs the engine to accept (`Grab`) or reject (`Ungrab(at)`) | Engine contract as in architecture 4.3 note; pending activations are released after 500 ms with a Warn log so the pointer never stays captured | D-90 |
| G-18 | Display sleep/wake has no cross-desktop source on Linux | GNOME: `org.gnome.Mutter.DisplayConfig` `PowerSaveMode`; KDE: a documented source must be verified by introspection, otherwise the item reports BLOCKED (no guessed events) | D-91 |

## Phase 2 - platform backends

### P2-X - phase coordination (parent and user)

- [ ] [P2-X-001] Verify the delivered P1 code against the gap decisions before P2 dispatch [FEAT-168] [tier:opus] [owner:PARENT] [brief:briefs/streams/p2-w1.md#item-p2-x-001]
  Needs: P1-F-022
  Files: briefs/plan-phase-2.md (append a NOTE with the verification outcome)
  Tests: the P1 item IDs in the dependency table still match what was merged; P1 code implements D-87 (wheel doc on `KindWheel`), D-88 (`EdgeSegment` doc and P1-C3-006 rule), D-89 (canvas math unchanged, native units), D-90 (`platform.ContactHolder` and engine Grab/Ungrab answer for every contact), D-84 (`agent.Main` signature); any mismatch becomes an appended fix item for the owning P1 stream before the dependent P2 item is dispatched
  DoD: NOTE appended; commit `docs(plan): P2 pre-dispatch verification [FEAT-168]`

- [ ] [P2-X-M01] User answers the P2 product questions Q-01 .. Q-04 [FEAT-168] [tier:haiku] [owner:USER] [brief:briefs/streams/p2-w1.md#item-p2-x-m01]
  Needs: none (asked at plan approval, `briefs/open-questions.md`)
  Files: decisions.md (parent appends one decision per answer), risk-register.md (Q-03, Q-04 outcomes), briefs/plan-phase-2.md (parent appends items when an answer adds work, e.g. Q-01 option A, Q-03 option A)
  Tests: Q-01 .. Q-04 each have a recorded decision; every item whose `Needs:` cites a `Q-NN decision` can name that decision
  DoD: commit `docs(decisions): P2 product answers [FEAT-168]`
<!-- NOTE: 2026-10-30 - Q-01 .. Q-04 are answered (D-112 .. D-115, user Q&A); the work they add is appended as P2-W2-036 .. P2-W2-038, P2-Y1-026, P2-Y1-027, P2-M1-028 (plus P1, P3 items). The parent only has to confirm and toggle this item. -->

- [ ] [P2-X-003] Agent entry point: cmd/edgesync-agent/main.go, host.RunMain for non-darwin, secure-helper dispatch [FEAT-142, FEAT-045] [tier:haiku] [owner:W1] [brief:briefs/streams/p2-w1.md#item-p2-x-003]
  Needs: P1-C5-017, P2-W1-020, P2-X1-019, P2-M1-018, P2-M1-003, P2-W2-025, D-84
  Files: cmd/edgesync-agent/main.go, cmd/edgesync-agent/helper_windows.go, cmd/edgesync-agent/helper_other.go, cmd/edgesync-agent/main_test.go, internal/platform/host/runmain_other.go, internal/platform/host/runmain_other_test.go
  Tests: TestRunMain_CallsRunAndReturnsItsCode, TestDispatch_SecureHelperFlagGoesToHelperMain, TestDispatch_NoFlagGoesToAgentMain, TestHelperOther_ReturnsUsageError; `go build ./cmd/edgesync-agent` for windows/amd64, windows/arm64, linux/amd64, linux/arm64, darwin/arm64, darwin/amd64 with CGO_ENABLED=0
  DoD: `main` contains only the D-84 dispatch (logic in testable functions); SG green; commit `feat(agent): agent entry point and main-thread runner [FEAT-142]`

### W1 - Windows input, displays, session, tray, overlay, autostart, theme, resources

- [ ] [P2-W1-001] Win32 declarations for W1 with struct-size assertions [FEAT-033, FEAT-035, FEAT-036, FEAT-039, FEAT-040] [tier:haiku] [owner:W1] [brief:briefs/streams/p2-w1.md#item-p2-w1-001]
  Needs: P1-F-022
  Files: internal/platform/windows/win32/doc.go, internal/platform/windows/win32/w1_user32.go, internal/platform/windows/win32/w1_kernel32.go, internal/platform/windows/win32/w1_shcore.go, internal/platform/windows/win32/w1_shell32.go, internal/platform/windows/win32/w1_gdi32.go, internal/platform/windows/win32/w1_wtsapi32.go, internal/platform/windows/win32/w1_types.go, internal/platform/windows/win32/w1_types_test.go
  Tests: TestW1StructSizes_amd64 (every struct in w1_types.go against its documented size), TestW1ProcsResolve (each declared LazyProc `Find()` succeeds on the host; host-safe: library load only)
  DoD: SG green; every constant and layout cited to its Microsoft Learn page in the brief and verified at execution time; commit `feat(win): Win32 declarations for input, displays and shell [FEAT-033]`

- [ ] [P2-W1-002] msgloop: OS-thread-locked message loop, window classes and a shared WNDPROC trampoline [FEAT-033, FEAT-039, FEAT-040] [tier:sonnet why:OS-thread-affine Win32 message loop with cross-goroutine marshalling that every W1/W2 component builds on] [owner:W1] [brief:briefs/streams/p2-w1.md#item-p2-w1-002]
  Needs: P2-W1-001, D-80
  Files: internal/platform/windows/msgloop/doc.go, internal/platform/windows/msgloop/thread_windows.go, internal/platform/windows/msgloop/window_windows.go, internal/platform/windows/msgloop/trampoline_windows.go, internal/platform/windows/msgloop/thread_windows_test.go
  Tests: TestThread_DoRunsOnSameOSThread, TestThread_DoReturnsHandlerError, TestThread_DoAfterCloseReturnsErrClosed, TestThread_CloseIsIdempotent, TestThread_NoGoroutineLeakAfterClose, TestWindow_MessageOnlyDispatchesToHandler, TestWindow_HiddenTopLevelDispatchesToHandler, TestWindow_DestroyUnregistersHandler, TestTrampoline_CreatedOnce (host-safe: hidden windows only)
  DoD: SG+race green; commit `feat(win): message-loop threads and window trampoline [FEAT-033]`

- [ ] [P2-W1-003] Icon generator and committed icon assets for all platforms [FEAT-037, FEAT-039, FEAT-059, FEAT-068] [tier:haiku] [owner:W1] [brief:briefs/streams/p2-w1.md#item-p2-w1-003]
  Needs: P1-F-022
  Files: assets/icons/doc.go, assets/icons/icons.go, assets/icons/icons_test.go, assets/icons/gen/main.go, assets/icons/gen/draw.go, assets/icons/gen/ico.go, assets/icons/gen/draw_test.go, assets/icons/gen/ico_test.go, assets/icons/*.ico and assets/icons/*.png (generated, exact list in the brief)
  Tests: TestDraw_Deterministic (same bytes twice), TestDraw_StateBadgesDiffer, TestICO_HeaderAndDirectory, TestICO_RoundTripPNGEntries, TestIcons_EmbeddedListComplete (every file in the brief's list is embedded with the stated pixel sizes)
  DoD: SG green; `go generate ./assets/icons/...` reproduces the committed files byte for byte (`task gen:check`); HC85 for `assets/icons/gen`; commit `feat(icons): generated app and tray icons [FEAT-039]`

- [ ] [P2-W1-004] Per-Monitor-v2 DPI guard, display enumeration and change watch [FEAT-036] [tier:haiku] [owner:W1] [brief:briefs/streams/p2-w1.md#item-p2-w1-004]
  Needs: P2-W1-001, P2-W1-002
  Files: internal/platform/windows/display/doc.go, internal/platform/windows/display/dpi_windows.go, internal/platform/windows/display/displays_windows.go, internal/platform/windows/display/convert.go, internal/platform/windows/display/convert_test.go, internal/platform/windows/display/displays_integration_test.go
  Tests: TestScalePermille_FromDPI (96 -> 1000, 120 -> 1250, 144 -> 1500, 168 -> 1750, 192 -> 2000), TestDesktopEqual_IgnoresOrder, TestDesktopEqual_DetectsScaleChange, TestDebounce_CoalescesBurst (synctest); integration: TestDisplays_Conformance (`conformance.RunDisplays`), TestDisplays_PrimaryPresentAndBoundsPhysical, TestDPI_ProcessIsPerMonitorV2
  DoD: SG+race green; HC85 for `display`; commit `feat(win): displays with per-monitor DPI [FEAT-036]`

- [ ] [P2-W1-005] Capture event ring and duration histogram [FEAT-033, FEAT-172] [tier:haiku] [owner:W1] [brief:briefs/streams/p2-w1.md#item-p2-w1-005]
  Needs: P1-F-022
  Files: internal/platform/windows/input/doc.go, internal/platform/windows/input/ring.go, internal/platform/windows/input/ring_test.go, internal/platform/windows/input/histogram.go, internal/platform/windows/input/histogram_test.go
  Tests: TestRing_PushPopOrder, TestRing_WrapAround, TestRing_FullDropsAndCounts, TestRing_ConcurrentSPSC, TestRing_PushAllocsZero, TestHistogram_Percentiles, TestHistogram_RecordAllocsZero, TestHistogram_MaxTracked; BenchmarkRingPush, BenchmarkHistogramRecord
  DoD: SG+race green; 0 allocs/op in both benchmarks; HC85 for `input`; commit `feat(win): capture ring and duration histogram [FEAT-033]`

- [ ] [P2-W1-006] Hook data translation and key state (pure) [FEAT-033, FEAT-029, FEAT-030] [tier:haiku] [owner:W1] [brief:briefs/streams/p2-w1.md#item-p2-w1-006]
  Needs: P2-W1-005, P1-C3-003, D-87
  Files: internal/platform/windows/input/translate.go, internal/platform/windows/input/translate_test.go, internal/platform/windows/input/keystate.go, internal/platform/windows/input/keystate_test.go
  Tests: TestMouse_MoveAbsolute, TestMouse_ButtonsLeftRightMiddle, TestMouse_XButtons1And2, TestMouse_WheelVerticalAndHorizontalSigns, TestMouse_HighResWheelFractionalUnits, TestMouse_OwnMarkerIgnored, TestMouse_ForeignInjectedKept, TestKey_ExtendedScancodes (Right Ctrl, arrows, numpad Enter, Insert/Delete block), TestKey_PauseAndPrintScreen, TestKey_UnknownScancodeDropped (counted), TestKeyState_RepeatDetected, TestKeyState_ReleaseClearsRepeat, TestTranslate_AllocsZero
  DoD: SG green; HC85 for `input`; commit `feat(win): hook event translation [FEAT-033]`

- [ ] [P2-W1-007] Edge-contact rule and swallow set (pure) [FEAT-033, FEAT-019] [tier:haiku] [owner:W1] [brief:briefs/streams/p2-w1.md#item-p2-w1-007]
  Needs: P2-W1-005, P1-C3-006, P1-C3-007, D-88
  Files: internal/platform/windows/input/contact.go, internal/platform/windows/input/contact_test.go, internal/platform/windows/input/swallow.go, internal/platform/windows/input/swallow_test.go
  Tests: TestContact_EachSide, TestContact_OutsideSegmentRange, TestContact_SegmentBoundsHalfOpen, TestContact_TwoSegmentsSameSide, TestContact_NoEdgesNoContact, TestContact_AllocsZero, TestSwallow_Contains, TestSwallow_ReplaceIsAtomic (race), TestSwallow_LookupAllocsZero
  DoD: SG+race green; HC85 for `input`; commit `feat(win): edge contact and key swallow set [FEAT-033]`

- [ ] [P2-W1-008] Low-level hook thread with non-allocating callbacks [FEAT-033, FEAT-172] [tier:sonnet why:LL-hook message-loop thread with non-allocating callbacks under GC pressure] [owner:W1] [brief:briefs/streams/p2-w1.md#item-p2-w1-008]
  Needs: P2-W1-002, P2-W1-006, P2-W1-007, D-80, sp1: LL hooks under GC
  Files: internal/platform/windows/input/hook_windows.go, internal/platform/windows/input/hookstate_windows.go, internal/platform/windows/input/hook_integration_test.go
  Tests: TestHookCallbackPath_AllocsZero (host: calls the Go callback body with synthetic structs, no hook installed); integration: TestHook_ObservesUnmarkedSendInput, TestHook_IgnoresOwnMarker, TestHook_SuppressionBlocksDeliveryToTestWindow, TestHook_SwallowedKeyNotDelivered, TestHook_EdgeContactEmittedAtSegment, TestHook_UninstallOnClose
  DoD: SG+race green; commit `feat(win): low-level hook thread [FEAT-033]`

- [ ] [P2-W1-009] Hook liveness watchdog [FEAT-033, FEAT-172] [tier:sonnet why:watchdog re-install races with the hook thread and the marker-event semantics measured by SP1] [owner:W1] [brief:briefs/streams/p2-w1.md#item-p2-w1-009]
  Needs: P2-W1-008, sp1: watchdog marker kind
  Files: internal/platform/windows/input/watchdog.go, internal/platform/windows/input/watchdog_test.go, internal/platform/windows/input/watchdog_windows.go, internal/platform/windows/input/watchdog_integration_test.go
  Tests: TestWatchdogPolicy_NoReinstallWhileMarkersSeen, TestWatchdogPolicy_ReinstallAfter2sSilence, TestWatchdogPolicy_CountsReinstalls, TestWatchdogPolicy_StopsOnClose (all synctest); integration: TestWatchdog_ReinstallsAfterForcedStall (test hook blocks one callback 1500 ms)
  DoD: SG+race green; HC85 for `input`; commit `feat(win): hook liveness watchdog [FEAT-033]`

- [ ] [P2-W1-010] Raw Input deltas while grabbed [FEAT-033] [tier:haiku] [owner:W1] [brief:briefs/streams/p2-w1.md#item-p2-w1-010]
  Needs: P2-W1-008, sp1: Raw Input deltas
  Files: internal/platform/windows/input/rawinput_windows.go, internal/platform/windows/input/rawdelta.go, internal/platform/windows/input/rawdelta_test.go
  Tests: TestRawDelta_RelativePassthrough, TestRawDelta_AbsoluteVirtualDesktopToPixels, TestRawDelta_AbsoluteFirstSampleYieldsNoDelta, TestRawDelta_ResetOnGrab, TestRawDelta_AllocsZero
  DoD: SG green; HC85 for `input`; commit `feat(win): raw input deltas [FEAT-033]`

- [ ] [P2-W1-011] Capturer: grab, cursor hide/park, Stats, InputCapturer + KeySwallower [FEAT-033, FEAT-034] [tier:sonnet why:SP1's cursor-hide strategy interacts with focus, fullscreen and elevated windows and ties hooks, Raw Input and watchdog into the frozen contract] [owner:W1] [brief:briefs/streams/p2-w1.md#item-p2-w1-011]
  Needs: P2-W1-009, P2-W1-010, P2-W1-004, sp1: cursor hide strategy
  Files: internal/platform/windows/input/capturer_windows.go, internal/platform/windows/input/cursor_windows.go, internal/platform/windows/input/driver_windows_test.go, internal/platform/windows/input/capturer_integration_test.go
  Tests: integration: TestCapturer_Conformance (`conformance.RunCapturer` with the SendInput driver), TestCapturer_GrabHidesCursorAndSuppresses, TestCapturer_UngrabPlacesCursor, TestCapturer_DeltasWhilePushingPastEdge, TestCapturer_StatsCountsDropsAndReinstalls, TestCapturer_CloseRestoresCursor
  DoD: SG+race green; commit `feat(win): input capturer with grab [FEAT-034]`

- [ ] [P2-W1-012] Direct injector via SendInput [FEAT-035, FEAT-029, FEAT-030] [tier:haiku] [owner:W1] [brief:briefs/streams/p2-w1.md#item-p2-w1-012]
  Needs: P2-W1-001, P2-W1-004, P1-C3-003, D-87
  Files: internal/platform/windows/input/inject_windows.go, internal/platform/windows/input/injectnorm.go, internal/platform/windows/input/injectnorm_test.go, internal/platform/windows/input/inject_integration_test.go
  Tests: TestNormalise_VirtualDeskCornersAndCentre, TestNormalise_NegativeOrigin, TestNormalise_RoundTripWithin1px, TestPressedSet_ReleaseAllOrder; integration: TestInjector_Conformance (`conformance.RunInjector`), TestInjector_MoveReadBackEveryMonitor (corners inset 1 px + centres, error <= 1 px), TestInjector_ExtendedKeysReachTestWindow, TestInjector_WheelAndHWheel, TestInjector_XButtons, TestInjector_ReleaseAllReleasesHeld
  DoD: SG green; HC85 for `input`; commit `feat(win): SendInput injector [FEAT-035]`

- [ ] [P2-W1-013] Session watcher: lock, display power, suspend, screensaver, secure desktop [FEAT-038] [tier:haiku] [owner:W1] [brief:briefs/streams/p2-w1.md#item-p2-w1-013]
  Needs: P2-W1-002
  Files: internal/platform/windows/session/doc.go, internal/platform/windows/session/watch_windows.go, internal/platform/windows/session/transitions.go, internal/platform/windows/session/transitions_test.go, internal/platform/windows/session/watch_integration_test.go
  Tests: TestTransitions_ScreensaverPollEdges, TestTransitions_SecureDesktopOnOffDebounced, TestTransitions_DisplayStateMapping (0 off, 1 on, 2 dimmed = no event), TestTransitions_NoDuplicateEvents (all synctest); integration: TestWatch_RegistersAndUnregisters, TestWatch_ScreensaverStartObserved (with P2-W1-014's action; skipped with an explicit reason unless `EDGESYNC_ALLOW_SCREENSAVER=1`)
  DoD: SG+race green; HC85 for `session`; commit `feat(win): session event watcher [FEAT-038]`

- [ ] [P2-W1-014] Session actions: lock, start screensaver, wake display [FEAT-038] [tier:haiku] [owner:W1] [brief:briefs/streams/p2-w1.md#item-p2-w1-014]
  Needs: P2-W1-013, P2-W1-012
  Files: internal/platform/windows/session/actions_windows.go, internal/platform/windows/session/session_windows.go, internal/platform/windows/session/actions_integration_test.go
  Tests: integration: TestSession_Conformance (`conformance.RunSession`; Lock exercised only with `EDGESYNC_ALLOW_LOCK=1`, otherwise skipped with that reason), TestActions_WakeDisplayInjectsMarkedMotion, TestActions_StartScreensaverReturnsNoError
  DoD: SG green; commit `feat(win): session actions [FEAT-038]`

- [ ] [P2-W1-015] Tray icon and menu via Shell_NotifyIcon [FEAT-039] [tier:haiku] [owner:W1] [brief:briefs/streams/p2-w1.md#item-p2-w1-015]
  Needs: P2-W1-002, P2-W1-003
  Files: internal/platform/windows/tray/doc.go, internal/platform/windows/tray/tray_windows.go, internal/platform/windows/tray/menu.go, internal/platform/windows/tray/menu_test.go, internal/platform/windows/tray/tray_integration_test.go
  Tests: TestMenu_FlattenAssignsStableCommandIDs, TestMenu_SubmenuAndSeparator, TestMenu_CheckedAndDisabledFlags, TestMenu_CommandToActionID, TestIconFor_StateAndTaskbarTheme; integration: TestTray_RunAddsIconAndRemovesOnCancel, TestTray_UpdateChangesTooltipAndIcon, TestTray_TaskbarCreatedReAdds (sends the registered message to the tray window), TestTray_CommandInvokesOnAction
  DoD: SG+race green; HC85 for `tray`; commit `feat(win): tray icon and menu [FEAT-039]`

- [ ] [P2-W1-016] Switch overlay: layered, topmost, click-through, DPI-scaled [FEAT-040] [tier:haiku] [owner:W1] [brief:briefs/streams/p2-w1.md#item-p2-w1-016]
  Needs: P2-W1-002, P2-W1-004
  Files: internal/platform/windows/overlay/doc.go, internal/platform/windows/overlay/overlay_windows.go, internal/platform/windows/overlay/geometry.go, internal/platform/windows/overlay/geometry_test.go, internal/platform/windows/overlay/fade.go, internal/platform/windows/overlay/fade_test.go, internal/platform/windows/overlay/overlay_integration_test.go
  Tests: TestGeometry_CentredOnMonitorScaledByDPI, TestGeometry_TextWiderThanMinimum, TestFade_AlphaSteps (synctest: 800 ms full, 200 ms fade), TestFade_ReplaceRestartsTimer; integration: TestOverlay_ShowCreatesTransparentTopmostWindow, TestOverlay_HiddenAfterDuration, TestOverlay_SecondShowReplacesText
  DoD: SG+race green; HC85 for `overlay`; commit `feat(win): switch overlay [FEAT-040]`

- [ ] [P2-W1-017] Autostart via the HKCU Run key with StartupApproved awareness [FEAT-041] [tier:haiku] [owner:W1] [brief:briefs/streams/p2-w1.md#item-p2-w1-017]
  Needs: P1-F-022
  Files: internal/platform/windows/autostart/doc.go, internal/platform/windows/autostart/autostart_windows.go, internal/platform/windows/autostart/cmdline.go, internal/platform/windows/autostart/cmdline_test.go, internal/platform/windows/autostart/autostart_integration_test.go
  Tests: TestCmdline_QuotesPathWithSpaces, TestCmdline_ParseMatchesOwnPath, TestStartupApproved_EnabledDisabledAbsent, TestStartupApproved_ShortValueIsError; integration: TestAutostart_Conformance (`conformance.RunAutostart` against a test value name), TestAutostart_DisabledInStartupApprovedReportsFalse
  DoD: SG green; HC85 for `autostart`; commit `feat(win): autostart via Run key [FEAT-041]`

- [ ] [P2-W1-018] Theme watcher [FEAT-042] [tier:haiku] [owner:W1] [brief:briefs/streams/p2-w1.md#item-p2-w1-018]
  Needs: P1-F-022
  Files: internal/platform/windows/theme/doc.go, internal/platform/windows/theme/theme_windows.go, internal/platform/windows/theme/theme_integration_test.go
  Tests: integration: TestTheme_Conformance (`conformance.RunTheme`), TestTheme_WatchFiresOnValueChange (writes a test copy under `HKCU\Software\EdgeSyncTest\Personalize` through an injectable key path), TestTheme_TaskbarLightFlag
  DoD: SG+race green; commit `feat(win): theme watcher [FEAT-042]`

- [ ] [P2-W1-019] Executable resources: manifests, version info, icons (.syso) for all three binaries [FEAT-037] [tier:haiku] [owner:W1] [brief:briefs/streams/p2-w1.md#item-p2-w1-019]
  Needs: P2-W1-003
  Files: internal/platform/windows/resources/doc.go, internal/platform/windows/resources/resources_test.go, cmd/edgesync-agent/versioninfo.json, cmd/edgesync-agent/manifest.xml, cmd/edgesync-agent/resource_windows_amd64.syso, cmd/edgesync-agent/resource_windows_arm64.syso, cmd/edgesync-service/versioninfo.json, cmd/edgesync-service/manifest.xml, cmd/edgesync-service/resource_windows_amd64.syso, cmd/edgesync-service/resource_windows_arm64.syso, cmd/edgesync/versioninfo.json, cmd/edgesync/manifest.xml, cmd/edgesync/resource_windows_amd64.syso, cmd/edgesync/resource_windows_arm64.syso
  Tests: TestManifest_PerMonitorV2AndDpiAware, TestManifest_AsInvokerAndSupportedOS, TestVersionInfo_FieldsPresent, TestSyso_RegeneratesIdentically (runs the generator into a temp dir and compares bytes, amd64 and arm64)
  DoD: SG green; `task gen:check` clean; commit `build(win): executable manifests and version resources [FEAT-037]`

- [ ] [P2-W1-020] Windows backend assembly, permissions and host_windows.go [FEAT-036, FEAT-039, FEAT-046, FEAT-047] [tier:haiku] [owner:W1] [brief:briefs/streams/p2-w1.md#item-p2-w1-020]
  Needs: P2-W1-011, P2-W1-012, P2-W1-014, P2-W1-015, P2-W1-016, P2-W1-017, P2-W1-018, P2-W2-006, P2-W2-024, P2-W2-026, P2-W2-027
  Files: internal/platform/windows/doc.go, internal/platform/windows/backend_windows.go, internal/platform/windows/permissions_windows.go, internal/platform/host/host_windows.go, internal/platform/windows/backend_integration_test.go
  Tests: integration: TestOpen_DirectInjectionAllAccessorsWork, TestOpen_HelperInjectionWithoutServiceFailsExplicitly, TestOpen_SecureHelperModeHasNoTrayOrClipboard (errors wrap `platform.ErrUnsupported`), TestPermissions_EmptyOnWindows, TestBackend_CloseIdempotent
  DoD: SG+race green; commit `feat(win): Windows backend assembly [FEAT-036]`

- [ ] [P2-W1-021] Win32 API floor audit (Windows 10 22H2 vs 11) [FEAT-168] [tier:haiku] [owner:W1] [brief:briefs/streams/p2-w1.md#item-p2-w1-021]
  Needs: P2-W1-001, P2-W2-001, P2-W2-008, P2-W2-014
  Files: internal/platform/windows/win32/apifloor_test.go, internal/platform/windows/win32/apifloor_integration_test.go
  Tests: TestAPIFloor_EveryDeclaredProcHasMinimumVersion (host: parses every `w*_*.go` in `win32/` with go/ast and checks each `NewProc("...")` name against the table), TestAPIFloor_NoEntryNewerThanWindows10_22H2 (host); integration: TestAPIFloor_ProcsResolveOnThisOS (logs OS version/build and calls `Find()` on every proc)
  DoD: SG green; table rows cite the Microsoft Learn "Minimum supported client" line; commit `test(win): Win32 API floor audit [FEAT-168]`

- [ ] [P2-W1-022] Hook callback budget test and bench/hooks harness (PB-07) [FEAT-172, FEAT-033] [tier:haiku] [owner:W1] [brief:briefs/streams/p2-w1.md#item-p2-w1-022]
  Needs: P2-W1-011
  Files: internal/platform/windows/input/hookbudget_integration_test.go, bench/hooks/main.go, bench/hooks/README.md
  Tests: integration: TestHookCallbackBudget (60 s of unmarked SendInput motion at 1000 Hz plus GC stress; asserts p99 <= 50 us, max <= 1 ms, 0 watchdog re-installs; logs the histogram); `go build ./bench/hooks` succeeds
  DoD: SG green; README gives the 10-minute run command with and without `-gcstress`; commit `perf(win): hook callback budget test and harness [FEAT-172]`

- [ ] [P2-W1-023] Injection latency bench (SendInput call cost) [FEAT-035, FEAT-170] [tier:haiku] [owner:W1] [brief:briefs/streams/p2-w1.md#item-p2-w1-023]
  Needs: P2-W1-012
  Files: internal/platform/windows/input/inject_bench_integration_test.go
  Tests: integration: BenchmarkInjectMoveAbs, BenchmarkInjectKeyDownUp, TestInjectLatency_Report (10 000 moves, logs p50/p99/max of the call duration)
  DoD: SG green; the parent records the CI and user numbers in perf-budgets.md under PB-01 (local receive-to-inject segment); commit `perf(win): injection latency bench [FEAT-035]`

- [ ] [P2-W1-024] Windows platform probe tool for manual verification [FEAT-168] [tier:haiku] [owner:W1] [brief:briefs/streams/p2-w1.md#item-p2-w1-024]
  Needs: P2-W1-020
  Files: internal/platform/windows/probe/main.go, internal/platform/windows/probe/cmd_input.go, internal/platform/windows/probe/cmd_shell.go, internal/platform/windows/probe/cmd_session.go, internal/platform/windows/probe/safety.go, internal/platform/windows/probe/safety_test.go, internal/platform/windows/probe/README.md
  Tests: TestSafety_GrabAutoReleasesAfterLimit (synctest), TestSafety_FlagValidation; `go build -o bin/edgesync-probe-win.exe ./internal/platform/windows/probe` succeeds (built, never run)
  DoD: SG+race green; every subcommand documented in README with the expected observation; commit `feat(win): platform probe tool [FEAT-168]`

- [ ] [P2-W1-025] CI job: Windows backend integration tests [FEAT-168] [tier:haiku] [owner:W1] [brief:briefs/streams/p2-w1.md#item-p2-w1-025]
  Needs: P2-W1-020, P2-W1-021, P2-W1-022, P2-W1-023
  Files: .github/workflows/ci.yml (claim: adds job `integration-windows-w1` only)
  Tests: actionlint clean; job on windows-latest runs `go test -tags integration -json` over the W1 package list from the brief, prints a summary table of pass/fail/skip with skip reasons into `$GITHUB_STEP_SUMMARY`, uploads the JSON log; `EDGESYNC_ALLOW_LOCK` and `EDGESYNC_ALLOW_SCREENSAVER` are not set
  DoD: actions pinned to exact tags verified via the GitHub API at execution time; commit `ci: Windows W1 integration job [FEAT-168]`

- [ ] [P2-W1-026] Manual verification script for W1 [FEAT-175] [tier:haiku] [owner:W1] [brief:briefs/streams/p2-w1.md#item-p2-w1-026]
  Needs: P2-W1-024, P2-W1-025
  Files: qa/manual/p2-w1-windows-input.md
  Tests: script follows `qa/manual/README.md`; covers both Windows PCs (OS name, version, build recorded; Windows 10 if available), every probe subcommand, the hook bench (10 min, with and without `-gcstress`), fullscreen video, elevated Task Manager, mixed-DPI monitors, Explorer restart for the tray, Task Manager startup tab, theme switch, and how to run the integration tests by hand
  DoD: binaries prebuilt into `bin/` and referenced; commit `docs(qa): W1 manual verification script [FEAT-175]`

- [ ] [P2-W1-M01] Run the W1 script on both Windows PCs and record results [FEAT-175, FEAT-172] [tier:haiku] [owner:USER] [brief:briefs/streams/p2-w1.md#item-p2-w1-m01]
  Needs: P2-W1-026
  Files: qa/manual/results/p2-w1-windows-input.md
  Tests: every step has an observation; PB-07 numbers recorded
  DoD: parent records results and PB-07 in perf-budgets.md; commit `docs(qa): W1 manual results [FEAT-175]`

- [ ] [P2-W1-027] W1 code-review gate [FEAT-168] [tier:sonnet why:code-review gate run by the code-reviewer agent] [owner:W1] [brief:briefs/streams/p2-w1.md#item-p2-w1-027]
  Needs: P2-W1-001 .. P2-W1-026
  Files: none (the parent appends the verdict as a NOTE under this stream; each finding becomes an appended fix item)
  Tests: reviewer applies `.claude/agents/code-reviewer.md` to the merged W1 diff
  DoD: VERDICT pass, or every finding fixed by appended items and re-reviewed

- [ ] [P2-W1-028] W1 UI/UX audit of tray and overlay [FEAT-160, FEAT-039, FEAT-040] [tier:sonnet why:UI/UX audit gate run by the code-reviewer agent with qa/ui-ux-audit.md] [owner:W1] [brief:briefs/streams/p2-w1.md#item-p2-w1-028]
  Needs: P2-W1-027, P2-W1-M01
  Files: none (verdict NOTE by the parent; findings become appended fix items)
  Tests: tray icons in light/dark taskbars, menu wording via i18n keys supplied by callers, overlay contrast/size at 100/150/200% scale using the user's screenshots
  DoD: VERDICT pass or findings fixed

- [ ] [P2-W1-029] W1 consolidation review [FEAT-168] [tier:opus] [owner:PARENT] [brief:briefs/streams/p2-w1.md#item-p2-w1-029]
  Needs: P2-W1-027, P2-W1-028
  Files: briefs/plan-phase-2.md (NOTE "stream W1 review chain green")
  Tests: contract adherence, decisions D-35/D-49/D-50/D-51/D-57/D-58/D-66 honoured, cross-stream consistency with W2
  DoD: NOTE appended; commit `chore(plan): W1 review chain green [FEAT-168]`

### W2 - Windows clipboard, COM, SYSTEM service, secure helper, SendSAS, pre-login

- [ ] [P2-W2-001] Win32 clipboard and memory declarations [FEAT-043] [tier:haiku] [owner:W2] [brief:briefs/streams/p2-w2.md#item-p2-w2-001]
  Needs: P2-W1-001
  Files: internal/platform/windows/win32/w2_clipboard.go, internal/platform/windows/win32/w2_kernel32.go, internal/platform/windows/win32/w2_clipboard_test.go
  Tests: TestW2ClipboardProcsResolve, TestW2ClipboardConstants (table against documented values)
  DoD: SG green; commit `feat(winclip): clipboard Win32 declarations [FEAT-043]`

- [ ] [P2-W2-002] Clipboard format table and native conversion glue (pure) [FEAT-043, FEAT-088] [tier:haiku] [owner:W2] [brief:briefs/streams/p2-w2.md#item-p2-w2-002]
  Needs: P1-C4-002, P1-C4-003, P1-C4-004, P1-C4-005, P1-C4-006
  Files: internal/platform/windows/clipboard/doc.go, internal/platform/windows/clipboard/formats.go, internal/platform/windows/clipboard/formats_test.go, internal/platform/windows/clipboard/native.go, internal/platform/windows/clipboard/native_test.go
  Tests: TestFormats_CanonicalToNativeOrder, TestFormats_NativeToCanonicalPrefersPNGOverDIBV5, TestNative_UTF16LEWithTerminatingNUL, TestNative_UTF16OddLengthIsError, TestNative_CFHTMLViaConvert, TestNative_PNGToDIBV5ViaConvert
  DoD: SG green; HC85 for `clipboard`; commit `feat(winclip): format table and native conversions [FEAT-043]`

- [ ] [P2-W2-003] Clipboard window thread and Watch [FEAT-043, FEAT-088] [tier:haiku] [owner:W2] [brief:briefs/streams/p2-w2.md#item-p2-w2-003]
  Needs: P2-W2-001, P2-W2-002, P2-W1-002, sp2: delayed rendering
  Files: internal/platform/windows/clipboard/window_windows.go, internal/platform/windows/clipboard/watch_windows.go, internal/platform/windows/clipboard/open_windows.go, internal/platform/windows/clipboard/retry.go, internal/platform/windows/clipboard/retry_test.go, internal/platform/windows/clipboard/watch_integration_test.go
  Tests: TestRetry_SucceedsWithinBudget, TestRetry_ExhaustedReturnsErrClipboardBusy (synctest); integration: TestWatch_ForeignCopyReported, TestWatch_OwnOfferNotReported, TestWatch_SequenceDedup, TestWatch_SizesUnknown
  DoD: SG+race green; HC85 for `clipboard`; commit `feat(winclip): clipboard watch [FEAT-043]`

- [ ] [P2-W2-004] Clipboard Read of local content in canonical form [FEAT-043] [tier:haiku] [owner:W2] [brief:briefs/streams/p2-w2.md#item-p2-w2-004]
  Needs: P2-W2-003
  Files: internal/platform/windows/clipboard/read_windows.go, internal/platform/windows/clipboard/read_integration_test.go
  Tests: integration: TestRead_TextNormalised, TestRead_HTMLUnwrappedFromCFHTML, TestRead_RTFPassthrough, TestRead_PNGPreferred, TestRead_DIBV5ConvertedToPNG, TestRead_MissingFormatIsErrFormatUnavailable, TestRead_VirtualOnlyUpdateNotAnnounced (D-102)
  DoD: SG green; commit `feat(winclip): clipboard read [FEAT-043]`

- [ ] [P2-W2-005] Offer with delayed rendering [FEAT-043] [tier:sonnet why:delayed-rendering message pump with blocking provider calls, timeouts and render-all on exit] [owner:W2] [brief:briefs/streams/p2-w2.md#item-p2-w2-005]
  Needs: P2-W2-004
  Files: internal/platform/windows/clipboard/offer_windows.go, internal/platform/windows/clipboard/render_windows.go, internal/platform/windows/clipboard/offer_integration_test.go
  Tests: integration: TestOffer_EachFormatRenderedOnFirstGetData, TestOffer_ProviderCalledOncePerFormat, TestOffer_ProviderErrorYieldsNoData, TestOffer_TimeoutYieldsNoData (slow provider, D-57 timeout), TestOffer_SecondOfferReplacesProvider, TestOffer_ForeignOwnerReleasesProvider, TestOffer_RenderAllOnCloseLogsFailures
  DoD: SG+race green; commit `feat(winclip): lazy offer via delayed rendering [FEAT-043]`

- [ ] [P2-W2-006] Clipboard backend type and conformance [FEAT-043] [tier:haiku] [owner:W2] [brief:briefs/streams/p2-w2.md#item-p2-w2-006]
  Needs: P2-W2-005
  Files: internal/platform/windows/clipboard/backend_windows.go, internal/platform/windows/clipboard/conformance_integration_test.go
  Tests: integration: TestClipboard_Conformance (`conformance.RunClipboard`), TestClipboard_CloseRemovesListenerAndWindow
  DoD: SG+race green; commit `feat(winclip): clipboard backend [FEAT-043]`

- [ ] [P2-W2-007] Clipboard render latency bench [FEAT-043, FEAT-170] [tier:haiku] [owner:W2] [brief:briefs/streams/p2-w2.md#item-p2-w2-007]
  Needs: P2-W2-006
  Files: internal/platform/windows/clipboard/render_bench_integration_test.go
  Tests: integration: TestRenderLatency_1MiBText, TestRenderLatency_10MiBPNG (in-memory provider; 50 runs; logs p50/p99 of `GetClipboardData` latency)
  DoD: SG green; parent records numbers against PB-09/PB-10 (local render segment); commit `perf(winclip): clipboard render latency bench [FEAT-043]`

- [ ] [P2-W2-008] OLE declarations and struct layouts [FEAT-103, FEAT-104, FEAT-105] [tier:haiku] [owner:W2] [brief:briefs/streams/p2-w2.md#item-p2-w2-008]
  Needs: P2-W1-001
  Files: internal/platform/windows/win32/w2_ole32.go, internal/platform/windows/win32/w2_oletypes.go, internal/platform/windows/win32/w2_oletypes_test.go
  Tests: TestOLEStructSizes_amd64 (FORMATETC 32, STGMEDIUM 24, FILEDESCRIPTORW 592, DROPFILES 20, STATSTG and POINTL per headers), TestOLEProcsResolve
  DoD: SG green; commit `feat(wincom): OLE declarations [FEAT-103]`

- [ ] [P2-W2-009] COM core: non-Go object memory, vtables, handle table, IUnknown [FEAT-103, FEAT-104, FEAT-105] [tier:sonnet why:hand-built COM vtables, reference counting and callback budget in pure Go] [owner:W2] [brief:briefs/streams/p2-w2.md#item-p2-w2-009]
  Needs: P2-W2-008, D-80, sp2: COM infrastructure
  Files: internal/platform/windows/com/doc.go, internal/platform/windows/com/alloc_windows.go, internal/platform/windows/com/vtable_windows.go, internal/platform/windows/com/handles.go, internal/platform/windows/com/handles_test.go, internal/platform/windows/com/unknown_windows.go, internal/platform/windows/com/unknown_windows_test.go
  Tests: TestHandles_AddGetRelease, TestHandles_ConcurrentAccess, TestHandles_ReleaseUnknownIsError; TestUnknown_QueryInterfaceKnownAndUnknownIID, TestUnknown_AddRefReleaseCounts, TestUnknown_FinalReleaseFreesMemoryAndHandle, TestCallbacks_CreatedOncePerMethod (host, calls through vtables with `syscall.SyscallN`)
  DoD: SG+race green; HC85 for `com`; commit `feat(wincom): COM object core [FEAT-103]`

- [ ] [P2-W2-010] STA thread on top of msgloop [FEAT-103, FEAT-104, FEAT-105] [tier:haiku] [owner:W2] [brief:briefs/streams/p2-w2.md#item-p2-w2-010]
  Needs: P2-W2-009, P2-W1-002
  Files: internal/platform/windows/com/sta_windows.go, internal/platform/windows/com/sta_windows_test.go
  Tests: TestSTA_OleInitializedOnThread (`CoGetApartmentType` reports STA), TestSTA_DoRunsOnSTAThread, TestSTA_CloseUninitializes, TestSTA_DoAfterCloseErrClosed
  DoD: SG+race green; commit `feat(wincom): STA thread [FEAT-103]`

- [ ] [P2-W2-011] IDataObject and IEnumFORMATETC over a Go DataSource [FEAT-103, FEAT-105] [tier:sonnet why:IDataObject GetData/QueryGetData semantics with FORMATETC matching, lindex and tymed rules] [owner:W2] [brief:briefs/streams/p2-w2.md#item-p2-w2-011]
  Needs: P2-W2-009, P2-W2-010
  Files: internal/platform/windows/com/dataobject_windows.go, internal/platform/windows/com/enumformatetc_windows.go, internal/platform/windows/com/dataobject_windows_test.go
  Tests: TestEnum_NextSkipResetClone, TestEnum_NextPartialReturnsSFalse, TestDataObject_QueryGetDataMatchesFormatTymedAspect, TestDataObject_GetDataUnknownFormatDVEFormatEtc, TestDataObject_GetDataPassesLindex, TestDataObject_SetDataAcceptedFormats, TestDataObject_AdviseNotSupported
  DoD: SG+race green; HC85 for `com`; commit `feat(wincom): IDataObject [FEAT-103]`

- [ ] [P2-W2-012] IStream over io.ReaderAt [FEAT-103] [tier:haiku] [owner:W2] [brief:briefs/streams/p2-w2.md#item-p2-w2-012]
  Needs: P2-W2-009
  Files: internal/platform/windows/com/stream_windows.go, internal/platform/windows/com/stream_windows_test.go
  Tests: TestStream_ReadSequential, TestStream_SeekSetCurEnd, TestStream_StatSizeAndName, TestStream_WriteDenied, TestStream_CloneIndependentPosition, TestStream_ReadErrorPropagatesHRESULT
  DoD: SG+race green; commit `feat(wincom): IStream [FEAT-103]`

- [ ] [P2-W2-013] IDropSource and IDropTarget wrappers [FEAT-104, FEAT-105] [tier:haiku] [owner:W2] [brief:briefs/streams/p2-w2.md#item-p2-w2-013]
  Needs: P2-W2-011
  Files: internal/platform/windows/com/dropsource_windows.go, internal/platform/windows/com/droptarget_windows.go, internal/platform/windows/com/drop_windows_test.go
  Tests: TestDropSource_QueryContinueDragEscapeCancels, TestDropSource_ButtonReleasedDrops, TestDropSource_GiveFeedbackDefault, TestDropTarget_DispatchEnterOverLeaveDrop, TestDropTarget_POINTLByValue, TestDropTarget_EffectReturned
  DoD: SG+race green; commit `feat(wincom): drag-drop source and target [FEAT-104]`

- [ ] [P2-W2-014] Service and helper Win32 declarations [FEAT-044, FEAT-045, FEAT-048] [tier:haiku] [owner:W2] [brief:briefs/streams/p2-w2.md#item-p2-w2-014]
  Needs: P2-W1-001
  Files: internal/platform/windows/win32/w2_service.go, internal/platform/windows/win32/w2_service_test.go
  Tests: TestW2ServiceProcsResolve, TestW2ServiceConstants
  DoD: SG green; commit `feat(service): Win32 declarations for service and helper [FEAT-044]`

- [ ] [P2-W2-015] Machine config and ProgramData ACL helpers [FEAT-044] [tier:haiku] [owner:W2] [brief:briefs/streams/p2-w2.md#item-p2-w2-015]
  Needs: P1-C5-002, P1-C5-001, D-83
  Files: internal/service/doc.go, internal/service/machineconfig.go, internal/service/machineconfig_test.go, internal/service/acl_windows.go, internal/service/acl_windows_test.go
  Tests: TestMachineConfig_DefaultsWhenAbsent (file absent = documented defaults; unreadable or invalid = error), TestMachineConfig_StrictRejectsUnknownKey, TestMachineConfig_SchemaVersionMismatchIsError, TestMachineConfig_AtomicWrite; TestACL_IdentityDirSDDL (SYSTEM + Administrators only), TestACL_ConfigDirSDDL, TestACL_ApplyToTempDirAndReadBack (host-safe: temp dir)
  DoD: SG green; HC85 for `internal/service` (host-compiled); commit `feat(service): machine config and ACLs [FEAT-044]`

- [ ] [P2-W2-016] edgesync-service skeleton and lifecycle [FEAT-044] [tier:haiku] [owner:W2] [brief:briefs/streams/p2-w2.md#item-p2-w2-016]
  Needs: P2-W2-015, P2-W2-014, P1-C5-003
  Files: cmd/edgesync-service/main.go, internal/service/service_windows.go, internal/service/lifecycle.go, internal/service/lifecycle_test.go
  Tests: TestLifecycle_StartsComponentsInOrder, TestLifecycle_StopsInReverseWithTimeout, TestLifecycle_StartFailureStopsStartedOnes (synctest); `go build ./cmd/edgesync-service` succeeds
  DoD: SG+race green; commit `feat(service): service skeleton [FEAT-044]`

- [ ] [P2-W2-017] Service install and uninstall subcommands [FEAT-044] [tier:haiku] [owner:W2] [brief:briefs/streams/p2-w2.md#item-p2-w2-017]
  Needs: P2-W2-016
  Files: cmd/edgesync-service/main.go, internal/service/install_windows.go, internal/service/installpath.go, internal/service/installpath_test.go
  Tests: TestInstallPath_AcceptsProgramFilesEdgeSync, TestInstallPath_RejectsOtherLocations, TestInstallPath_CaseInsensitiveAndLongPathForm, TestRecoveryActions_Values
  DoD: SG green; commit `feat(service): install and uninstall [FEAT-044]`

- [ ] [P2-W2-018] Session tracker [FEAT-044, FEAT-045, FEAT-049] [tier:sonnet why:service session tracking across console connect/disconnect, logon/logoff and remote sessions] [owner:W2] [brief:briefs/streams/p2-w2.md#item-p2-w2-018]
  Needs: P2-W2-016
  Files: internal/service/sessions.go, internal/service/sessions_test.go, internal/service/sessions_windows.go
  Tests: TestSessions_BootNoUserWantsPreLogin, TestSessions_LogonSwitchesToUserMode, TestSessions_LogoffReturnsToPreLogin, TestSessions_ConsoleDisconnectStopsHelper, TestSessions_FastUserSwitchTargetsNewSession, TestSessions_NoActiveConsoleSession (0xFFFFFFFF) (all synctest)
  DoD: SG+race green; HC85 for `internal/service`; commit `feat(service): session tracker [FEAT-044]`

- [ ] [P2-W2-019] Secure helper launcher and supervisor [FEAT-045] [tier:sonnet why:winlogon token duplication with TokenUIAccess and cross-session process creation under a job object] [owner:W2] [brief:briefs/streams/p2-w2.md#item-p2-w2-019]
  Needs: P2-W2-018, sp1: service/helper/token launch
  Files: internal/service/launch_windows.go, internal/service/supervise.go, internal/service/supervise_test.go
  Tests: TestSupervise_RestartsWithBackoff (250 ms .. 30 s, factor 2, full jitter), TestSupervise_ResetsBackoffAfterStableRun, TestSupervise_StopsOnSessionChange, TestSupervise_NoRestartAfterStop (synctest)
  DoD: SG+race green; commit `feat(service): secure helper launcher [FEAT-045]`

- [ ] [P2-W2-020] Service pipe server with per-request authorisation [FEAT-050] [tier:sonnet why:security-critical authorisation of a SYSTEM pipe per D-47] [owner:W2] [brief:briefs/streams/p2-w2.md#item-p2-w2-020]
  Needs: P2-W2-016, sp1: pipe SDDL and SID verification
  Files: internal/service/pipe_windows.go, internal/service/authz.go, internal/service/authz_test.go, internal/service/pipe_windows_test.go
  Tests: TestAuthz_AllowsWhitelistedFromInstalledAgentSameSession, TestAuthz_RejectsUnknownCommand, TestAuthz_RejectsOtherImagePath, TestAuthz_RejectsSessionMismatch, TestAuthz_DecisionLoggedEitherWay; host pipe: TestPipe_SDDLAces, TestPipe_FirstInstance, TestPipe_OversizedMessageRejectedBeforeDecode, TestPipe_TraversalLimitApplied
  DoD: SG+race green; HC85 for `internal/service`; commit `feat(service): authorised service pipe [FEAT-050]`

- [ ] [P2-W2-021] Machine identity and SignTLS [FEAT-051, FEAT-124] [tier:sonnet why:key custody with DPAPI machine scope and the signing-proxy contract of D-41] [owner:W2] [brief:briefs/streams/p2-w2.md#item-p2-w2-021]
  Needs: P2-W2-020, P2-W2-015, P1-C2-001, sp1: signing proxy
  Files: internal/service/identity.go, internal/service/identity_test.go, internal/service/identity_windows.go, internal/service/cmd_sign.go, internal/service/cmd_sign_test.go
  Tests: TestIdentity_CreatesOnFirstStart, TestIdentity_LoadsExisting, TestIdentity_ReissuesWithin30DaysSameKey, TestIdentity_CorruptKeyIsError, TestSign_ECDSAP256DigestVerifies, TestSign_RejectsWrongDigestLength (protector injected for host tests; DPAPI used only in the windows file)
  DoD: SG+race green; HC85 for `internal/service`; commit `feat(service): machine identity and TLS signing [FEAT-051]`

- [ ] [P2-W2-022] RequestSAS, Hello, HelperStatus and ReportInputDesktop handlers [FEAT-048, FEAT-047, FEAT-050] [tier:haiku] [owner:W2] [brief:briefs/streams/p2-w2.md#item-p2-w2-022]
  Needs: P2-W2-020, P2-W2-019, D-82
  Files: internal/service/cmd_sas_windows.go, internal/service/saspolicy.go, internal/service/saspolicy_test.go, internal/service/cmd_status.go, internal/service/cmd_status_test.go
  Tests: TestSASPolicy_Values (absent, 0, 1, 2, 3), TestSASPolicy_DisallowedIsErrSASPolicy, TestHello_ReportsVersionsAndHelperState, TestReportInputDesktop_TogglesHelperForwarding
  DoD: SG green; commit `feat(service): SAS and status commands [FEAT-048]`

- [ ] [P2-W2-023] Brokered helper channel: SYSTEM-only helper pipe and handle duplication into the agent [FEAT-046, FEAT-050] [tier:sonnet why:handle brokering across processes and sessions on a security boundary] [owner:W2] [brief:briefs/streams/p2-w2.md#item-p2-w2-023]
  Needs: P2-W2-020, P2-W2-019, D-81
  Files: internal/service/broker_windows.go, internal/platform/windows/helper/doc.go, internal/platform/windows/helper/pipe_windows.go, internal/platform/windows/helper/pipe_windows_test.go
  Tests: TestHelperPipe_SDDLSystemOnly, TestBroker_DuplicatesHandleIntoTargetProcess (host: duplicates into the test process itself), TestBroker_RejectsUnauthorisedAgent, TestHelperPipe_AcceptsOnlyBrokeredClients
  DoD: SG+race green; commit `feat(service): brokered helper channel [FEAT-046]`

- [ ] [P2-W2-024] Service client library for the agent [FEAT-051, FEAT-048] [tier:haiku] [owner:W2] [brief:briefs/streams/p2-w2.md#item-p2-w2-024]
  Needs: P2-W2-021, P2-W2-022, P2-W2-023
  Files: internal/service/client/doc.go, internal/service/client/client_windows.go, internal/service/client/signer.go, internal/service/client/signer_test.go, internal/service/client/client_windows_test.go
  Tests: TestSigner_TLS13HandshakeOverPipeWithProxyKey, TestSigner_PublicMatchesCertificate, TestSigner_ErrorPropagates; TestClient_VerifiesServerIsLocalSystem, TestClient_HelloReturnsBrokeredHandle (host pipes with a test server)
  DoD: SG+race green; HC85 for `internal/service/client`; commit `feat(service): agent-side service client [FEAT-051]`

- [ ] [P2-W2-025] Secure helper process main and input-desktop follower [FEAT-045] [tier:sonnet why:thread-desktop switching for several locked OS threads following the input desktop] [owner:W2] [brief:briefs/streams/p2-w2.md#item-p2-w2-025]
  Needs: P2-W2-023, P2-W1-002, D-84
  Files: internal/platform/windows/helper/run_windows.go (exports `Main(ctx context.Context, args []string) int`, D-84), internal/platform/windows/helper/desktop_windows.go, internal/platform/windows/helper/desktopstate.go, internal/platform/windows/helper/desktopstate_test.go
  Tests: TestDesktopState_NameChangeNotifiesThreads, TestDesktopState_AccessDeniedIsReportedNotSwallowed, TestDesktopState_PollInterval200ms (synctest); `go build ./internal/platform/windows/helper/...` for windows/amd64 and windows/arm64 (the `cmd/edgesync-agent` dispatch is P2-X-003)
  DoD: SG+race green; HC85 for `helper`; commit `feat(helper): secure helper main and desktop follower [FEAT-045]`

- [ ] [P2-W2-026] Helper injection server and helper-routed InputInjector [FEAT-046] [tier:haiku] [owner:W2] [brief:briefs/streams/p2-w2.md#item-p2-w2-026]
  Needs: P2-W2-025, P2-W1-012
  Files: internal/platform/windows/helper/inject_server_windows.go, internal/platform/windows/helper/injector_client.go, internal/platform/windows/helper/injector_client_test.go
  Tests: TestInjectorClient_EachMethodSendsRequest, TestInjectorClient_ServerErrorPropagates, TestInjectorClient_ReleaseAllTracksPressed, TestInjectorClient_CloseClosesChannel (net.Pipe with a fake server decoding HelperRequest)
  DoD: SG+race green; HC85 for `helper`; commit `feat(helper): helper-routed injection [FEAT-046]`

- [ ] [P2-W2-027] Secure-desktop capture forwarding [FEAT-047] [tier:sonnet why:hooks on a desktop-switching thread inside a SYSTEM-token process with event forwarding to the agent] [owner:W2] [brief:briefs/streams/p2-w2.md#item-p2-w2-027]
  Needs: P2-W2-025, P2-W2-022, P2-W1-011, D-82
  Files: internal/platform/windows/helper/capture_server_windows.go, internal/platform/windows/helper/capture_client.go, internal/platform/windows/helper/capture_client_test.go
  Tests: TestCaptureClient_DecodesEventsIntoSink, TestCaptureClient_FullSinkCountsDrops, TestCaptureClient_StopsOnContextCancel
  DoD: SG+race green; commit `feat(helper): secure-desktop capture forwarding [FEAT-047]`

- [ ] [P2-W2-028] Pre-login client mode [FEAT-049, FEAT-179] [tier:sonnet why:minimal composition of session, engine client role and helper injection under SYSTEM with the machine identity] [owner:W2] [brief:briefs/streams/p2-w2.md#item-p2-w2-028]
  Needs: P2-W2-026, P2-W2-021, P1-C1-007, P1-C1-010, P1-C1-013, P1-C2-009, P1-C3-020, D-112, P2-W2-036
  Files: internal/platform/windows/helper/prelogin.go, internal/platform/windows/helper/prelogin_test.go, internal/platform/windows/helper/prelogin_windows.go
  Tests: TestPreLogin_AcceptsListedServerAndInjects (loopback session, fake injector log), TestPreLogin_RejectsServerNotInMachineStore, TestPreLogin_StoreChangeAppliesToNextSession, TestPreLogin_MissingStoreFileRejectsEveryServer, TestPreLogin_RejectsClientRole, TestPreLogin_NoClipboardOrFileChannels, TestPreLogin_ReleaseAllOnDisconnect
  DoD: SG+race green; HC85 for `helper`; commit `feat(helper): pre-login client mode [FEAT-049]`

- [ ] [P2-W2-029] Ctrl+Alt+Delete chord to RequestSAS in helper injection mode [FEAT-048, FEAT-180] [tier:haiku] [owner:W2] [brief:briefs/streams/p2-w2.md#item-p2-w2-029]
  Needs: P2-W2-026, P2-W2-024, D-113
  Files: internal/platform/windows/helper/saschord.go, internal/platform/windows/helper/saschord_test.go, internal/platform/windows/helper/injector_client.go
  Tests: TestSASChord_LeftOrRightModifiers, TestSASChord_DeleteWithoutModifiersPassesThrough, TestSASChord_ChordKeysNotInjected, TestSASChord_RequestErrorReturned, TestSASChord_EngineSendCADSequenceRequestsSAS (the exact sequence of P1-C3-026)
  DoD: SG green; commit `feat(helper): Ctrl+Alt+Del to SendSAS [FEAT-048]`

- [ ] [P2-W2-030] W2 probe tool (clipboard, service, helper, pre-login driver) [FEAT-168] [tier:haiku] [owner:W2] [brief:briefs/streams/p2-w2.md#item-p2-w2-030]
  Needs: P2-W2-006, P2-W2-024, P2-W2-028, P2-W2-029
  Files: internal/service/probe/main.go, internal/service/probe/cmd_clip.go, internal/service/probe/cmd_service.go, internal/service/probe/cmd_prelogin.go, internal/service/probe/README.md
  Tests: `go build -o bin/edgesync-probe-w2.exe ./internal/service/probe` succeeds; flag parsing tests TestProbeFlags_Validation
  DoD: SG green; commit `feat(service): W2 probe tool [FEAT-168]`

- [ ] [P2-W2-031] CI job: W2 integration tests (clipboard, COM, pipes) [FEAT-168] [tier:haiku] [owner:W2] [brief:briefs/streams/p2-w2.md#item-p2-w2-031]
  Needs: P2-W2-007, P2-W2-013, P2-W2-024
  Files: .github/workflows/ci.yml (claim: adds job `integration-windows-w2` only)
  Tests: actionlint clean; job runs `go test -tags integration -json` over `./internal/platform/windows/clipboard/... ./internal/platform/windows/com/... ./internal/service/...` with a skip summary and uploaded log
  DoD: actions pinned; commit `ci: Windows W2 integration job [FEAT-168]`

- [ ] [P2-W2-032] Manual verification scripts for W2 [FEAT-175, FEAT-179, FEAT-180] [tier:haiku] [owner:W2] [brief:briefs/streams/p2-w2.md#item-p2-w2-032]
  Needs: P2-W2-030, P2-W2-031, P2-W2-036, P2-W2-037, P2-W2-038
  Files: qa/manual/p2-w2-clipboard.md, qa/manual/p2-w2-service.md
  Tests: scripts follow `qa/manual/README.md`; clipboard script: Notepad, WordPad/Word, browser HTML, Paint PNG, password manager copy (announced like any other content, D-121), render latency; service script: admin install, `SoftwareSASGeneration`, elevated Task Manager, UAC prompt, lock screen, Ctrl+Alt+Del chord (also Ctrl+Alt+End from the other PC acting as server, D-113), sign-in screen driven from the other PC after `edgesync-service allow-prelogin <peer-id>` (UAC prompt) and rejected again after `allow-prelogin --remove <peer-id>` (D-112), helper restart after `taskkill /F`, second Windows account rejection, uninstall
  DoD: binaries prebuilt into `bin/`; commit `docs(qa): W2 manual verification scripts [FEAT-175]`

- [ ] [P2-W2-M01] Run the W2 clipboard script on both Windows PCs [FEAT-175, FEAT-043] [tier:haiku] [owner:USER] [brief:briefs/streams/p2-w2.md#item-p2-w2-m01]
  Needs: P2-W2-032
  Files: qa/manual/results/p2-w2-clipboard.md
  Tests: every step observed
  DoD: parent commits results `docs(qa): W2 clipboard results [FEAT-175]`

- [ ] [P2-W2-M02] Run the W2 service script on both Windows PCs (admin) [FEAT-175, FEAT-044, FEAT-045, FEAT-046, FEAT-048, FEAT-049] [tier:haiku] [owner:USER] [brief:briefs/streams/p2-w2.md#item-p2-w2-m02]
  Needs: P2-W2-032, P2-X-003
  Files: qa/manual/results/p2-w2-service.md
  Tests: every step observed
  DoD: parent commits results `docs(qa): W2 service results [FEAT-175]`

- [ ] [P2-W2-033] W2 code-review gate [FEAT-168] [tier:sonnet why:code-review gate run by the code-reviewer agent] [owner:W2] [brief:briefs/streams/p2-w2.md#item-p2-w2-033]
  Needs: P2-W2-001 .. P2-W2-032, P2-W2-036 .. P2-W2-038
  Files: none (verdict NOTE by the parent; findings become appended fix items)
  Tests: `.claude/agents/code-reviewer.md` over the merged W2 diff
  DoD: VERDICT pass or findings fixed and re-reviewed

- [ ] [P2-W2-034] W2 security review [FEAT-173, FEAT-050] [tier:sonnet why:security-review gate with qa/security-review.md sections 2, 3, 4, 7] [owner:W2] [brief:briefs/streams/p2-w2.md#item-p2-w2-034]
  Needs: P2-W2-033, P2-W2-M02
  Files: none (verdict NOTE by the parent; findings become appended fix items, R4 consolidates)
  Tests: every point of `qa/security-review.md` sections 2, 4 and 7 (and 3 for the pipe patterns) applied to the W2 diff and the M02 results
  DoD: VERDICT pass or findings fixed and re-reviewed

- [ ] [P2-W2-035] W2 consolidation review [FEAT-168] [tier:opus] [owner:PARENT] [brief:briefs/streams/p2-w2.md#item-p2-w2-035]
  Needs: P2-W2-034, P2-W2-M01
  Files: briefs/plan-phase-2.md (NOTE "stream W2 review chain green")
  Tests: D-07, D-10, D-40, D-41, D-47, D-61, D-63, D-64 honoured; consistency with W1 and with P4 F1/F2 needs (COM package API)
  DoD: NOTE appended; commit `chore(plan): W2 review chain green [FEAT-168]`

- [ ] [P2-W2-036] Machine-scoped peer store for pre-login mode (D-112) [FEAT-179, FEAT-049] [tier:haiku] [owner:W2] [brief:briefs/streams/p2-w2.md#item-p2-w2-036]
  Needs: P2-W2-015, P1-C5-002, D-112
  Files: internal/service/machinepeers/doc.go, internal/service/machinepeers/store.go, internal/service/machinepeers/store_test.go
  Tests: TestStore_AbsentFileIsEmpty, TestStore_AddIsIdempotentAndSortedByID, TestStore_RemoveAbsentIsNoError, TestStore_StrictDecodeRejectsUnknownKey, TestStore_SchemaVersionMismatchIsError, TestStore_InvalidPeerIDRejected (length, uppercase, alphabet), TestStore_NameLimitsAndControlCharacters, TestStore_AtomicWriteLeavesNoTempFile, TestStore_ContainsRereadsFile, TestStore_ConcurrentAddRemove (race)
  DoD: SG+race green; HC85 for `machinepeers`; commit `feat(service): machine peer store [FEAT-179]`

- [ ] [P2-W2-037] Elevated command `edgesync-service allow-prelogin` (D-112) [FEAT-179] [tier:haiku] [owner:W2] [brief:briefs/streams/p2-w2.md#item-p2-w2-037]
  Needs: P2-W2-036, P2-W2-017
  Files: internal/service/cmd_allowprelogin.go, internal/service/cmd_allowprelogin_test.go, cmd/edgesync-service/allowprelogin_windows.go, cmd/edgesync-service/main.go (one dispatch case)
  Tests: TestAllowPrelogin_AddsListedPeer, TestAllowPrelogin_RemoveDeletesPeer, TestAllowPrelogin_NotElevatedExit3NothingWritten, TestAllowPrelogin_InvalidPeerIDExit4, TestAllowPrelogin_UsageErrorsExit2, TestAllowPrelogin_NameSanitisedAndLimited, TestAllowPrelogin_AuditRecordHasNoSecrets, TestAllowPrelogin_AddIsIdempotent
  DoD: SG+race green; HC85 for `internal/service`; `go build ./cmd/edgesync-service` succeeds; the command never contacts the service; commit `feat(service): allow-prelogin elevated command [FEAT-179]`

- [ ] [P2-W2-038] Read-only service request GetPreloginPeers (D-112, amends D-47) [FEAT-179, FEAT-050, FEAT-049] [tier:sonnet why:extends the D-47 whitelist and the frozen service schema while keeping the per-request authorisation rules intact] [owner:W2] [brief:briefs/streams/p2-w2.md#item-p2-w2-038]
  Needs: P2-W2-036, P2-W2-020, P2-W2-024
  Files: proto/schema/service.capnp (claim), internal/proto/capnp/servicecp/service.capnp.go (regenerated, claim), internal/proto/msg/service.go (claim), internal/proto/msg/service_test.go (claim), internal/service/authz.go, internal/service/authz_test.go, internal/service/cmd_preloginpeers.go, internal/service/cmd_preloginpeers_test.go, internal/service/client/prelogin.go, internal/service/client/prelogin_test.go
  Tests: TestService_GetPreloginPeersRoundTrip, TestService_UnknownRequestStillRejected, TestAuthz_AllowsGetPreloginPeersFromInstalledAgentSameSession, TestAuthz_RejectsGetPreloginPeersFromOtherImagePath, TestAuthz_RejectsGetPreloginPeersSessionMismatch, TestGetPreloginPeers_ReturnsStoreContentAndChangesNothing, TestClient_PreloginPeersOverFakePipe
  DoD: `task gen:check` green; SG+race green; fuzz smoke green for the service decoders; HC85 for `internal/service` and `internal/service/client`; commit `feat(service): read-only GetPreloginPeers request [FEAT-179]`

### X1 - Linux X11 and shared Linux (SNI tray, XDG autostart, theme, session)

- [ ] [P2-X1-001] D-Bus connection helpers (session/system bus, name-owner watch, signal subscriptions) [FEAT-068, FEAT-071, FEAT-072] [tier:haiku] [owner:X1] [brief:briefs/streams/p2-x1.md#item-p2-x1-001]
  Needs: P1-F-022
  Files: internal/platform/linux/doc.go, internal/platform/linux/dbusx/doc.go, internal/platform/linux/dbusx/match.go, internal/platform/linux/dbusx/match_test.go, internal/platform/linux/dbusx/conn_linux.go, internal/platform/linux/dbusx/conn_integration_test.go
  Tests: TestMatchRule_Escaping, TestMatchRule_AllFields; integration (private `dbus-daemon`): TestConn_SessionAndInjectedSystemBus, TestNameOwner_WatchReportsAppearAndVanish, TestSignals_UnsubscribeOnCancel
  DoD: SG green; HC85 for `dbusx`; commit `feat(linux): D-Bus helpers [FEAT-068]`

- [ ] [P2-X1-002] Theme via the Settings portal [FEAT-071] [tier:haiku] [owner:X1] [brief:briefs/streams/p2-x1.md#item-p2-x1-002]
  Needs: P2-X1-001
  Files: internal/platform/linux/theme/doc.go, internal/platform/linux/theme/theme_linux.go, internal/platform/linux/theme/scheme.go, internal/platform/linux/theme/scheme_test.go, internal/platform/linux/theme/theme_integration_test.go
  Tests: TestScheme_Mapping (0 -> light, 1 -> dark, 2 -> light, other -> error), TestScheme_V1DoubleVariantUnwrap; integration (fake portal on a private bus): TestTheme_Conformance, TestTheme_WatchOnSettingChanged, TestTheme_NoPortalIsUnsupportedError
  DoD: SG green; HC85 for `theme`; commit `feat(linux): theme from the Settings portal [FEAT-071]`

- [ ] [P2-X1-003] XDG autostart entry [FEAT-070] [tier:haiku] [owner:X1] [brief:briefs/streams/p2-x1.md#item-p2-x1-003]
  Needs: P1-F-022
  Files: internal/platform/linux/autostart/doc.go, internal/platform/linux/autostart/autostart.go, internal/platform/linux/autostart/autostart_test.go, internal/platform/linux/autostart/autostart_linux.go
  Tests: TestAutostart_EnableWritesDesktopFile, TestAutostart_ExecQuotingPerDesktopEntrySpec, TestAutostart_DisableRemovesFile, TestAutostart_HiddenTrueReportsDisabled, TestAutostart_ForeignExecReportsDisabled, TestAutostart_Conformance (`conformance.RunAutostart` with a temp XDG_CONFIG_HOME, host-safe)
  DoD: SG green; HC85 for `autostart`; commit `feat(linux): XDG autostart [FEAT-070]`

- [ ] [P2-X1-004] Session events and actions (logind, ScreenSaver, display power) [FEAT-072] [tier:haiku] [owner:X1] [brief:briefs/streams/p2-x1.md#item-p2-x1-004]
  Needs: P2-X1-001, D-91
  Files: internal/platform/linux/session/doc.go, internal/platform/linux/session/session_linux.go, internal/platform/linux/session/sources.go, internal/platform/linux/session/sources_test.go, internal/platform/linux/session/transitions.go, internal/platform/linux/session/transitions_test.go, internal/platform/linux/session/session_integration_test.go
  Tests: TestSources_ByDesktop (GNOME, KDE, unknown), TestTransitions_LockedHint, TestTransitions_PrepareForSleep, TestTransitions_ScreensaverActiveChanged, TestTransitions_PowerSaveMode (synctest); integration (fake login1/ScreenSaver/Mutter objects on private buses): TestSession_Conformance, TestSession_LockCallsLogindLock, TestSession_WakeDisplayCallsSimulateUserActivity
  DoD: SG green; HC85 for `session`; commit `feat(linux): session events and actions [FEAT-072]`

- [ ] [P2-X1-005] StatusNotifierItem export and watcher registration [FEAT-068] [tier:haiku] [owner:X1] [brief:briefs/streams/p2-x1.md#item-p2-x1-005]
  Needs: P2-X1-001, P2-W1-003
  Files: internal/platform/linux/sni/doc.go, internal/platform/linux/sni/item_linux.go, internal/platform/linux/sni/pixmap.go, internal/platform/linux/sni/pixmap_test.go, internal/platform/linux/sni/item_integration_test.go
  Tests: TestPixmap_ARGB32NetworkOrder, TestPixmap_AllSizesFromAssets; integration (test-provided StatusNotifierWatcher on a private bus): TestItem_RegistersWithWatcher, TestItem_PropertiesAfterUpdate, TestItem_NewIconOnStateChange, TestItem_ReRegistersAfterWatcherRestart, TestItem_NoWatcherIsUnsupportedError, TestItem_ActivateInvokesOpen
  DoD: SG green; HC85 for `sni`; commit `feat(linux): StatusNotifierItem tray [FEAT-068]`

- [ ] [P2-X1-006] dbusmenu export from TrayModel [FEAT-068] [tier:sonnet why:com.canonical.dbusmenu layout export with revisions and event dispatch] [owner:X1] [brief:briefs/streams/p2-x1.md#item-p2-x1-006]
  Needs: P2-X1-005
  Files: internal/platform/linux/sni/menu_linux.go, internal/platform/linux/sni/menulayout.go, internal/platform/linux/sni/menulayout_test.go, internal/platform/linux/sni/menu_integration_test.go, internal/platform/linux/sni/tray_linux.go
  Tests: TestLayout_IDsStableAcrossUpdates, TestLayout_PropertiesForCheckDisabledSeparatorSubmenu, TestLayout_RevisionIncrementsOnChange; integration: TestMenu_GetLayoutMatchesModel, TestMenu_EventClickedInvokesOnAction, TestMenu_LayoutUpdatedSignalOnUpdate, TestTray_Conformance
  DoD: SG green; HC85 for `sni`; commit `feat(linux): dbusmenu for the tray [FEAT-068]`

- [ ] [P2-X1-007] X11 connection, extensions and atoms [FEAT-063, FEAT-064, FEAT-065, FEAT-067] [tier:haiku] [owner:X1] [brief:briefs/streams/p2-x1.md#item-p2-x1-007]
  Needs: P1-F-022, sp3: X11 xgb
  Files: internal/platform/x11/doc.go, internal/platform/x11/conn_linux.go, internal/platform/x11/atoms_linux.go, internal/platform/x11/version.go, internal/platform/x11/version_test.go, internal/platform/x11/conn_integration_test.go
  Tests: TestVersion_RequirementsTable, TestVersion_TooOldIsError; integration (Xvfb): TestConn_ExtensionsPresentWithVersions (logs vendor, release, extension versions), TestAtoms_InternedOnce
  DoD: SG green; HC85 for `x11`; commit `feat(x11): connection and extensions [FEAT-063]`

- [ ] [P2-X1-008] Displays via RandR monitors and Xft.dpi [FEAT-065] [tier:haiku] [owner:X1] [brief:briefs/streams/p2-x1.md#item-p2-x1-008]
  Needs: P2-X1-007
  Files: internal/platform/x11/displays_linux.go, internal/platform/x11/xftdpi.go, internal/platform/x11/xftdpi_test.go, internal/platform/x11/displays_integration_test.go
  Tests: TestXftDPI_Parse (present, absent = 96 per X conventions, malformed = error), TestXftDPI_ToPermille; integration (Xvfb + two `xrandr --setmonitor` monitors): TestDisplays_Conformance, TestDisplays_TwoMonitorsListed, TestDisplays_WatchFiresOnMonitorChange
  DoD: SG green; HC85 for `x11`; commit `feat(x11): displays via RandR [FEAT-065]`

- [ ] [P2-X1-009] Keymap verification (evdev + 8) [FEAT-066] [tier:haiku] [owner:X1] [brief:briefs/streams/p2-x1.md#item-p2-x1-009]
  Needs: P2-X1-007, P1-C3-003
  Files: internal/platform/x11/keymapcheck.go, internal/platform/x11/keymapcheck_test.go, internal/platform/x11/keymapcheck_linux.go, internal/platform/x11/keymapcheck_integration_test.go
  Tests: TestKeymapCheck_AllProbesMatch, TestKeymapCheck_MismatchListsKeycodes; integration: TestKeymapCheck_XvfbIsEvdev
  DoD: SG green; HC85 for `x11`; commit `feat(x11): keymap verification [FEAT-066]`

- [ ] [P2-X1-010] X11 event translation (pure) [FEAT-063, FEAT-029, FEAT-030] [tier:haiku] [owner:X1] [brief:briefs/streams/p2-x1.md#item-p2-x1-010]
  Needs: P1-C3-003, D-87
  Files: internal/platform/x11/translate.go, internal/platform/x11/translate_test.go
  Tests: TestTranslate_KeycodeMinus8ToUsage, TestTranslate_Buttons123, TestTranslate_Buttons4567ToWheel, TestTranslate_Buttons89ToX1X2, TestTranslate_MotionRootCoords, TestTranslate_RepeatTracking, TestTranslate_AllocsZero
  DoD: SG green; HC85 for `x11`; commit `feat(x11): event translation [FEAT-063]`

- [ ] [P2-X1-011] Passive observation via RECORD, edge contacts and key swallowing [FEAT-063, FEAT-019] [tier:sonnet why:RECORD data-stream parsing on a second connection plus edge-contact and passive key-grab semantics] [owner:X1] [brief:briefs/streams/p2-x1.md#item-p2-x1-011]
  Needs: P2-X1-007, P2-X1-010, P1-C3-006, P1-C3-007, D-88, D-92
  Files: internal/platform/x11/record_linux.go, internal/platform/x11/recordparse.go, internal/platform/x11/recordparse_test.go, internal/platform/x11/contact.go, internal/platform/x11/contact_test.go, internal/platform/x11/swallow_linux.go, internal/platform/x11/record_integration_test.go
  Tests: TestRecordParse_GoldenDeviceEvents, TestRecordParse_TruncatedIsError, TestRecordParse_IgnoresNonDeviceCategories, TestContact_EachSide, TestContact_SegmentBounds; integration (Xvfb): TestRecord_ObservesXTestMotionKeysButtons, TestSwallow_GrabKeyBlocksDeliveryToTestWindow, TestContact_EmittedAtRightEdgeSegment
  DoD: SG green; HC85 for `x11`; commit `feat(x11): passive observation and edge contacts [FEAT-063]`

- [ ] [P2-X1-012] Grab with warp-to-centre deltas; X11 InputCapturer [FEAT-063] [tier:sonnet why:X grab semantics with warp-echo suppression and RECORD dedupe while grabbed] [owner:X1] [brief:briefs/streams/p2-x1.md#item-p2-x1-012]
  Needs: P2-X1-008, P2-X1-011, sp3: X11 grab and warp deltas
  Files: internal/platform/x11/grab_linux.go, internal/platform/x11/warpdelta.go, internal/platform/x11/warpdelta_test.go, internal/platform/x11/capturer_linux.go, internal/platform/x11/driver_linux_test.go, internal/platform/x11/capturer_integration_test.go
  Tests: TestWarpDelta_SumsMotionAndSkipsWarpEcho, TestWarpDelta_RecentreThreshold; integration (Xvfb): TestCapturer_Conformance (XTest driver), TestCapturer_GrabSuppressesTestWindow, TestCapturer_DeltasMatchDrivenMotion, TestCapturer_UngrabWarpsToPoint, TestCapturer_GrabRetriesThenExplicitError
  DoD: SG green; HC85 for `x11`; commit `feat(x11): grab and capturer [FEAT-063]`

- [ ] [P2-X1-013] XTest injector [FEAT-064, FEAT-029, FEAT-030] [tier:haiku] [owner:X1] [brief:briefs/streams/p2-x1.md#item-p2-x1-013]
  Needs: P2-X1-007, P1-C3-003, D-87
  Files: internal/platform/x11/inject_linux.go, internal/platform/x11/wheelaccum.go, internal/platform/x11/wheelaccum_test.go, internal/platform/x11/inject_integration_test.go
  Tests: TestWheelAccum_WholeNotches, TestWheelAccum_RemainderCarried, TestWheelAccum_SignPerAxis; integration: TestInjector_Conformance, TestInjector_MoveReadBack, TestInjector_X1X2Buttons, TestInjector_ReleaseAll
  DoD: SG green; HC85 for `x11`; commit `feat(x11): XTest injector [FEAT-064]`

- [ ] [P2-X1-014] X11 injection latency bench [FEAT-064, FEAT-170] [tier:haiku] [owner:X1] [brief:briefs/streams/p2-x1.md#item-p2-x1-014]
  Needs: P2-X1-013
  Files: internal/platform/x11/inject_bench_integration_test.go
  Tests: integration: BenchmarkInjectMoveAbs, TestInjectLatency_Report (10 000 moves with `xproto.GetInputFocus` sync every 100; p50/p99 logged)
  DoD: SG green; parent records CI numbers (relative only, per perf-budgets.md "CI"); commit `perf(x11): injection latency bench [FEAT-064]`

- [ ] [P2-X1-015] X11 clipboard read side: XFixes watch, TARGETS, INCR receive [FEAT-067, FEAT-088] [tier:haiku] [owner:X1] [brief:briefs/streams/p2-x1.md#item-p2-x1-015]
  Needs: P2-X1-007, P1-C4-002, P1-C4-003, P1-C4-004, P1-C4-005, P1-C4-006
  Files: internal/platform/x11/clipwatch_linux.go, internal/platform/x11/clipread_linux.go, internal/platform/x11/targets.go, internal/platform/x11/targets_test.go, internal/platform/x11/incrrecv.go, internal/platform/x11/incrrecv_test.go
  Tests: TestTargets_ToCanonicalFormats, TestTargets_PrimaryIgnored, TestIncrRecv_AssemblesChunks, TestIncrRecv_ZeroLengthEnds, TestIncrRecv_SizeCapEnforced
  DoD: SG green; HC85 for `x11`; commit `feat(x11): clipboard watch and read [FEAT-067]`

- [ ] [P2-X1-016] X11 clipboard owner side and backend [FEAT-067] [tier:sonnet why:ICCCM selection-owner state machine with INCR, MULTIPLE and concurrent requestors] [owner:X1] [brief:briefs/streams/p2-x1.md#item-p2-x1-016]
  Needs: P2-X1-015
  Files: internal/platform/x11/clipowner_linux.go, internal/platform/x11/incrsend.go, internal/platform/x11/incrsend_test.go, internal/platform/x11/clipboard_linux.go, internal/platform/x11/clipboard_integration_test.go
  Tests: TestIncrSend_ChunkSizes, TestIncrSend_TerminatesWithZeroLength; integration (Xvfb, second connection as requestor): TestClipboard_Conformance, TestOwner_TargetsAndTimestamp, TestOwner_EachFormat, TestOwner_INCR5MiB, TestOwner_Multiple, TestOwner_SelectionClearDropsProvider, TestOwner_ProviderErrorRefusesRequest, TestWatch_OwnOfferNotReported
  DoD: SG green; HC85 for `x11`; commit `feat(x11): clipboard owner [FEAT-067]`

- [ ] [P2-X1-017] X11 clipboard render latency bench [FEAT-067, FEAT-170] [tier:haiku] [owner:X1] [brief:briefs/streams/p2-x1.md#item-p2-x1-017]
  Needs: P2-X1-016
  Files: internal/platform/x11/clip_bench_integration_test.go
  Tests: integration: TestRenderLatency_1MiBText, TestRenderLatency_10MiBPNG (requestor connection; 50 runs; p50/p99 logged)
  DoD: SG green; commit `perf(x11): clipboard render latency bench [FEAT-067]`

- [ ] [P2-X1-018] X11 switch overlay [FEAT-069] [tier:haiku] [owner:X1] [brief:briefs/streams/p2-x1.md#item-p2-x1-018]
  Needs: P2-X1-008, D-85
  Files: internal/platform/x11/overlay_linux.go, internal/platform/x11/overlayrender.go, internal/platform/x11/overlayrender_test.go, internal/platform/x11/testdata/overlay_golden.png, internal/platform/x11/overlay_integration_test.go, go.mod, go.sum
  Tests: TestOverlayRender_GoldenImage, TestOverlayRender_LongNameEllipsised, TestOverlayRender_ScaleFromPermille; integration (Xvfb): TestOverlay_MapsOverrideRedirectOnPointerMonitor, TestOverlay_InputShapeEmpty, TestOverlay_UnmappedAfterDuration, TestOverlay_SecondShowReplaces
  DoD: SG green (claim on go.mod/go.sum; dependency exactly as in the G-11 decision); HC85 for `x11`; commit `feat(x11): switch overlay [FEAT-069]`

- [ ] [P2-X1-019] Linux session-type selection, X11 backend assembly and host_linux.go [FEAT-063, FEAT-065, FEAT-068] [tier:haiku] [owner:X1] [brief:briefs/streams/p2-x1.md#item-p2-x1-019]
  Needs: P2-X1-002, P2-X1-003, P2-X1-004, P2-X1-006, P2-X1-009, P2-X1-012, P2-X1-013, P2-X1-016, P2-X1-018
  Files: internal/platform/linux/sessiontype/doc.go, internal/platform/linux/sessiontype/sessiontype.go, internal/platform/linux/sessiontype/sessiontype_test.go, internal/platform/x11/backend_linux.go, internal/platform/host/host_linux.go, internal/platform/x11/backend_integration_test.go
  Tests: TestSessionType_X11, TestSessionType_Wayland, TestSessionType_FromDisplayVarsOnly, TestSessionType_AmbiguousIsError, TestSessionType_NoneIsError; integration (Xvfb): TestOpen_X11AllAccessors, TestOpen_PermissionsEmpty, TestBackend_CloseIdempotent
  DoD: SG green; HC85 for `sessiontype`; in a Wayland session `host.Open` returns an error wrapping `platform.ErrUnsupported` naming P2-Y1-017 until that item adds the branch; commit `feat(x11): X11 backend assembly [FEAT-063]`

- [ ] [P2-X1-020] X11 probe tool [FEAT-168] [tier:haiku] [owner:X1] [brief:briefs/streams/p2-x1.md#item-p2-x1-020]
  Needs: P2-X1-019
  Files: internal/platform/x11/probe/main.go, internal/platform/x11/probe/cmd_input.go, internal/platform/x11/probe/cmd_shell.go, internal/platform/x11/probe/README.md
  Tests: `GOOS=linux CGO_ENABLED=0 go build ./internal/platform/x11/probe` succeeds; TestProbeFlags_Validation
  DoD: SG green; commit `feat(x11): probe tool [FEAT-168]`

- [ ] [P2-X1-021] CI job: X11 and shared-Linux integration tests under Xvfb [FEAT-168] [tier:haiku] [owner:X1] [brief:briefs/streams/p2-x1.md#item-p2-x1-021]
  Needs: P2-X1-019, P2-X1-014, P2-X1-017
  Files: .github/workflows/ci.yml (claim: adds job `integration-linux-x11` only)
  Tests: actionlint clean; ubuntu-24.04 job installs xvfb, x11-xserver-utils, dbus, xfonts-base; starts `Xvfb :99 -screen 0 3840x1080x24`, defines two RandR monitors, runs `dbus-run-session -- go test -tags integration -json ./internal/platform/x11/... ./internal/platform/linux/...`, writes the skip/fail summary and extension versions to `$GITHUB_STEP_SUMMARY`, uploads logs
  DoD: actions pinned; commit `ci: X11 integration job [FEAT-168]`

- [ ] [P2-X1-022] X1 verification script (CI only) [FEAT-175] [tier:haiku] [owner:X1] [brief:briefs/streams/p2-x1.md#item-p2-x1-022]
  Needs: P2-X1-021
  Files: qa/manual/p2-x1-x11-ci.md
  Tests: script follows `qa/manual/README.md`; states plainly that X11 is verified on CI only (D-37, R-08); tells the user how to push, find the `integration-linux-x11` run, and which summary and artefacts to share
  DoD: commit `docs(qa): X1 CI verification script [FEAT-175]`

- [ ] [P2-X1-M01] Push and share the X11 CI run [FEAT-175, FEAT-168] [tier:haiku] [owner:USER] [brief:briefs/streams/p2-x1.md#item-p2-x1-m01]
  Needs: P2-X1-022
  Files: qa/manual/results/p2-x1-x11-ci.md
  Tests: run URL, summary table, extension versions recorded
  DoD: parent commits results `docs(qa): X11 CI results [FEAT-175]`

- [ ] [P2-X1-023] X1 code-review gate [FEAT-168] [tier:sonnet why:code-review gate run by the code-reviewer agent] [owner:X1] [brief:briefs/streams/p2-x1.md#item-p2-x1-023]
  Needs: P2-X1-001 .. P2-X1-022
  Files: none (verdict NOTE by the parent; findings become appended fix items)
  Tests: `.claude/agents/code-reviewer.md` over the merged X1 diff
  DoD: VERDICT pass or findings fixed

- [ ] [P2-X1-024] X1 UI/UX audit of SNI tray and X11 overlay [FEAT-160, FEAT-068, FEAT-069] [tier:sonnet why:UI/UX audit gate with qa/ui-ux-audit.md] [owner:X1] [brief:briefs/streams/p2-x1.md#item-p2-x1-024]
  Needs: P2-X1-023, P2-X1-M01
  Files: none (verdict NOTE by the parent)
  Tests: overlay golden image and CI screenshots, tray pixmaps at all sizes, menu structure from the dbusmenu test logs
  DoD: VERDICT pass or findings fixed

- [ ] [P2-X1-025] X1 consolidation review [FEAT-168] [tier:opus] [owner:PARENT] [brief:briefs/streams/p2-x1.md#item-p2-x1-025]
  Needs: P2-X1-024
  Files: briefs/plan-phase-2.md (NOTE "stream X1 review chain green")
  Tests: D-39, D-51, D-52, D-61, D-71 honoured; shared Linux packages fit Y1
  DoD: NOTE appended; commit `chore(plan): X1 review chain green [FEAT-168]`

### Y1 - Linux Wayland (GNOME 45+, KDE Plasma 6.1+) via portals and EI

- [ ] [P2-Y1-001] EI protocol table generator from the libei protocol XML [FEAT-073] [tier:haiku] [owner:Y1] [brief:briefs/streams/p2-y1.md#item-p2-y1-001]
  Needs: P1-F-022, sp3: EI codec
  Files: third_party/libei/LICENSE, third_party/libei/README.md, third_party/libei/protocol.xml, internal/platform/wayland/ei/doc.go, internal/platform/wayland/ei/gen/main.go, internal/platform/wayland/ei/gen/parse.go, internal/platform/wayland/ei/gen/parse_test.go, internal/platform/wayland/ei/protocol_gen.go (generated)
  Tests: TestParse_InterfacesRequestsEvents, TestParse_ArgTypes, TestParse_UnknownArgTypeIsError, TestGenerated_UpToDate (re-runs the generator in memory and compares)
  DoD: SG green; `task gen:check` clean; HC85 for `ei/gen`; commit `feat(ei): protocol tables from the libei XML [FEAT-073]`

- [ ] [P2-Y1-002] EI wire codec [FEAT-073] [tier:haiku] [owner:Y1] [brief:briefs/streams/p2-y1.md#item-p2-y1-002]
  Needs: P2-Y1-001
  Files: internal/platform/wayland/ei/wire.go, internal/platform/wayland/ei/wire_test.go, internal/platform/wayland/ei/wire_fuzz_test.go
  Tests: TestWire_HeaderRoundTrip, TestWire_StringPaddingAndNUL, TestWire_AllArgTypesRoundTrip, TestWire_LengthIncludesHeader, TestWire_RejectsShortOversizedAndMisaligned, TestWire_GoldenHandshakeFrames; FuzzDecodeFrame
  DoD: SG green; fuzz smoke 30 s; HC85 for `ei`; commit `feat(ei): wire codec [FEAT-073]`

- [ ] [P2-Y1-003] EI fd passing [FEAT-073] [tier:haiku] [owner:Y1] [brief:briefs/streams/p2-y1.md#item-p2-y1-003]
  Needs: P2-Y1-002
  Files: internal/platform/wayland/ei/fd_linux.go, internal/platform/wayland/ei/fd_linux_test.go
  Tests (linux, run by the CI `test` job; socketpair only): TestFD_SendRecvOne, TestFD_ClosesUnusedKeymapFD, TestFD_TruncatedControlMessageIsError
  DoD: SG green (incl. `GOOS=linux go vet`); commit `feat(ei): fd passing [FEAT-073]`

- [ ] [P2-Y1-004] EI client state machine (receiver and sender roles) [FEAT-073, FEAT-074, FEAT-075] [tier:sonnet why:EI protocol state machine across handshake, seats, device lifecycle and emulation sequences] [owner:Y1] [brief:briefs/streams/p2-y1.md#item-p2-y1-004]
  Needs: P2-Y1-002
  Files: internal/platform/wayland/ei/client.go, internal/platform/wayland/ei/handshake.go, internal/platform/wayland/ei/seat.go, internal/platform/wayland/ei/device.go, internal/platform/wayland/ei/receiver.go, internal/platform/wayland/ei/sender.go, internal/platform/wayland/ei/fakeeis_test.go, internal/platform/wayland/ei/client_test.go
  Tests: TestHandshake_NegotiatesInterfaceVersions, TestHandshake_ServerRejectsIsError, TestSeat_BindRequestedCapabilities, TestDevice_LifecycleAddedResumedPausedRemoved, TestReceiver_FrameGroupsEvents, TestReceiver_StartStopEmulatingSequence, TestSender_StartEmulatingFrameStop, TestSender_RejectsEventsWhilePaused, TestClient_PingPong, TestClient_DisconnectedReason, TestClient_NoGoroutineLeak (scripted fake EIS over net.Pipe)
  DoD: SG+race green; HC85 for `ei`; commit `feat(ei): client state machine [FEAT-073]`

- [ ] [P2-Y1-005] Portal request/response helper [FEAT-074, FEAT-075, FEAT-078] [tier:sonnet why:asynchronous portal Request/Response flows with subscribe-before-call ordering] [owner:Y1] [brief:briefs/streams/p2-y1.md#item-p2-y1-005]
  Needs: P2-X1-001
  Files: internal/platform/wayland/portal/doc.go, internal/platform/wayland/portal/request_linux.go, internal/platform/wayland/portal/token.go, internal/platform/wayland/portal/token_test.go, internal/platform/wayland/portal/fakeportal_integration_test.go, internal/platform/wayland/portal/request_integration_test.go
  Tests: TestToken_UniqueAndValidChars, TestRequestPath_FromSenderAndToken; integration (fake portal on a private bus): TestRequest_SuccessResults, TestRequest_CancelledIsErrPortalCancelled, TestRequest_OtherIsErrPortalFailed, TestRequest_SubscribedBeforeCall, TestRequest_ContextCancelClosesRequest
  DoD: SG green; HC85 for `portal`; commit `feat(portal): request helper [FEAT-074]`

- [ ] [P2-Y1-006] InputCapture portal client with restore tokens [FEAT-074, FEAT-077] [tier:sonnet why:InputCapture session flows across interface versions with zones, barriers and EIS connection] [owner:Y1] [brief:briefs/streams/p2-y1.md#item-p2-y1-006]
  Needs: P2-Y1-005, P1-C5-001, P1-C5-002, sp3: InputCapture
  Files: internal/platform/wayland/portal/inputcapture_linux.go, internal/platform/wayland/portal/tokens.go, internal/platform/wayland/portal/tokens_test.go, internal/platform/wayland/portal/inputcapture_integration_test.go
  Tests: TestTokens_StorePerInterfaceMode0600, TestTokens_MissingIsNotError (absent = no token; unreadable = error), TestTokens_ReplacedAfterStart; integration (fake portal): TestInputCapture_VersionSelectsCreateSessionVariant, TestInputCapture_ZonesParsed, TestInputCapture_FailedBarriersIsError, TestInputCapture_ConnectToEISReturnsUnixConn, TestInputCapture_SignalsDelivered
  DoD: SG green; HC85 for `portal`; commit `feat(portal): InputCapture client [FEAT-074]`

- [ ] [P2-Y1-007] RemoteDesktop portal client [FEAT-075, FEAT-077] [tier:haiku] [owner:Y1] [brief:briefs/streams/p2-y1.md#item-p2-y1-007]
  Needs: P2-Y1-006, sp3: RemoteDesktop injection
  Files: internal/platform/wayland/portal/remotedesktop_linux.go, internal/platform/wayland/portal/remotedesktop_integration_test.go
  Tests: integration (fake portal): TestRemoteDesktop_SelectDevicesPersistModeAndToken, TestRemoteDesktop_StartStoresNewToken, TestRemoteDesktop_ConnectToEISAfterStart, TestRemoteDesktop_DeniedIsErrPermission
  DoD: SG green; commit `feat(portal): RemoteDesktop client [FEAT-075]`

- [ ] [P2-Y1-008] Clipboard portal client [FEAT-078] [tier:haiku] [owner:Y1] [brief:briefs/streams/p2-y1.md#item-p2-y1-008]
  Needs: P2-Y1-005, sp3: portal clipboard
  Files: internal/platform/wayland/portal/clipboard_linux.go, internal/platform/wayland/portal/clipboard_integration_test.go
  Tests: integration (fake portal): TestClipboard_RequestBeforeStart, TestClipboard_SetSelectionMimeTypes, TestClipboard_SelectionWriteAndDone, TestClipboard_SelectionReadFD, TestClipboard_OwnerChangedSignal
  DoD: SG green; commit `feat(portal): Clipboard client [FEAT-078]`

- [ ] [P2-Y1-009] Capture session manager: barriers from edge segments, recreation, activations [FEAT-074, FEAT-076] [tier:sonnet why:session recreation on barrier and zone changes (mutter quirks R-19) with barrier-to-segment mapping] [owner:Y1] [brief:briefs/streams/p2-y1.md#item-p2-y1-009]
  Needs: P2-Y1-004, P2-Y1-006, P1-C3-006, P1-C3-007, D-88, D-89, sp3: InputCapture recreation
  Files: internal/platform/wayland/capsession_linux.go, internal/platform/wayland/barriers.go, internal/platform/wayland/barriers_test.go, internal/platform/wayland/capstate.go, internal/platform/wayland/capstate_test.go
  Tests: TestBarriers_SegmentToZoneLogical, TestBarriers_RejectsInteriorSegment, TestBarriers_IDMapping, TestCapState_RecreateOnSetEdges, TestCapState_RecreateOnZonesChanged, TestCapState_RecreateOnUnexpectedClose, TestCapState_BackoffOnRepeatedFailure (synctest)
  DoD: SG green; HC85 for `wayland`; commit `feat(wayland): capture session manager [FEAT-074]`

- [ ] [P2-Y1-010] Wayland InputCapturer: activation, Grab/Ungrab, EI receiver translation [FEAT-074, FEAT-029, FEAT-030, FEAT-181] [tier:sonnet why:activation and release semantics coupled to engine accept/reject decisions and guard interplay (R-23)] [owner:Y1] [brief:briefs/streams/p2-y1.md#item-p2-y1-010]
  Needs: P2-Y1-009, P1-C3-003, D-87, D-90, D-114
  Files: internal/platform/wayland/capturer_linux.go, internal/platform/wayland/translate.go, internal/platform/wayland/translate_test.go, internal/platform/wayland/pending.go, internal/platform/wayland/pending_test.go, internal/platform/wayland/capturer_integration_test.go
  Tests: TestTranslate_EvdevKeys, TestTranslate_ButtonsBTNCodes, TestTranslate_ScrollDiscreteAndContinuous, TestTranslate_RelativeMotionScaledToPhysical, TestPending_GrabAccepts, TestPending_UngrabReleasesAtPoint, TestPending_SafetyReleaseAfter500ms (synctest), TestCapturer_GrabWithoutActivationUnsupported, TestCapturer_SwallowWhileHomeUnsupported; integration (GNOME box): TestCapturer_Conformance (driven by the RemoteDesktop EI sender)
  DoD: SG green; HC85 for `wayland`; commit `feat(wayland): input capturer [FEAT-074]`

- [ ] [P2-Y1-011] Wayland injector via RemoteDesktop + EI sender [FEAT-075, FEAT-029, FEAT-030] [tier:haiku] [owner:Y1] [brief:briefs/streams/p2-y1.md#item-p2-y1-011]
  Needs: P2-Y1-004, P2-Y1-007, P1-C3-003, D-87, D-89
  Files: internal/platform/wayland/injector_linux.go, internal/platform/wayland/regions.go, internal/platform/wayland/regions_test.go, internal/platform/wayland/injector_integration_test.go
  Tests: TestRegions_PhysicalToRegionLogical, TestRegions_PointOutsideEveryRegionIsError, TestRegions_FractionalScale125; integration (GNOME box): TestInjector_Conformance, TestInjector_SquareAndText
  DoD: SG green; HC85 for `wayland`; commit `feat(wayland): EI injector [FEAT-075]`

- [ ] [P2-Y1-012] Displays from portal zones and EI regions [FEAT-076] [tier:haiku] [owner:Y1] [brief:briefs/streams/p2-y1.md#item-p2-y1-012]
  Needs: P2-Y1-006, P2-Y1-004, D-89, sp3: zones and scale
  Files: internal/platform/wayland/displays_linux.go, internal/platform/wayland/zones.go, internal/platform/wayland/zones_test.go
  Tests: TestZones_ToDesktopWithScales, TestZones_ZoneWithoutRegionIsError, TestZones_StableMonitorIDs, TestZones_ChangeDetected
  DoD: SG green; HC85 for `wayland`; commit `feat(wayland): displays from zones [FEAT-076]`

- [ ] [P2-Y1-013] Wayland clipboard backend over the Clipboard portal [FEAT-078, FEAT-088] [tier:sonnet why:portal clipboard transfers with fd passing and session-activity constraints (R-20)] [owner:Y1] [brief:briefs/streams/p2-y1.md#item-p2-y1-013]
  Needs: P2-Y1-008, P2-Y1-007, P1-C4-002, P1-C4-003, P1-C4-004, P1-C4-005, P1-C4-006
  Files: internal/platform/wayland/clipboard_linux.go, internal/platform/wayland/mimes.go, internal/platform/wayland/mimes_test.go, internal/platform/wayland/clipboard_integration_test.go
  Tests: TestMimes_ToCanonical, TestMimes_OfferList; integration (fake portal): TestClipboard_WatchSkipsOwnSelection, TestClipboard_ReadViaSelectionRead, TestClipboard_OfferServesTransferAndDone, TestClipboard_ProviderErrorWritesDoneFalse, TestClipboard_InactiveSessionIsExplicitError; integration (GNOME box): TestClipboard_Conformance
  DoD: SG green; HC85 for `wayland`; commit `feat(wayland): portal clipboard backend [FEAT-078]`

- [ ] [P2-Y1-014] Portal permissions and degraded-mode reporting [FEAT-077] [tier:haiku] [owner:Y1] [brief:briefs/streams/p2-y1.md#item-p2-y1-014]
  Needs: P2-Y1-006, P2-Y1-007, P2-Y1-008
  Files: internal/platform/wayland/permissions_linux.go, internal/platform/wayland/permstate.go, internal/platform/wayland/permstate_test.go
  Tests: TestPermState_GrantedWithToken, TestPermState_NoPersistenceDetail, TestPermState_DeniedDetail, TestPermState_ClipboardUnavailableDetail
  DoD: SG green; HC85 for `wayland`; commit `feat(wayland): portal permissions [FEAT-077]`

- [ ] [P2-Y1-015] Compositor and portal capability detection [FEAT-077, FEAT-168] [tier:haiku] [owner:Y1] [brief:briefs/streams/p2-y1.md#item-p2-y1-015]
  Needs: P2-Y1-005
  Files: internal/platform/wayland/caps.go, internal/platform/wayland/caps_test.go, internal/platform/wayland/caps_linux.go
  Tests: TestCaps_GNOME45Supported, TestCaps_GNOME44Unsupported, TestCaps_Plasma61Supported, TestCaps_Plasma60Unsupported, TestCaps_UnknownDesktopUnsupported, TestCaps_InputCaptureMissingUnsupported, TestCaps_VersionStringParsing
  DoD: SG green; HC85 for `wayland`; commit `feat(wayland): capability detection [FEAT-077]`

- [ ] [P2-Y1-016] Switch indicator on Wayland (approved SP3 mechanism) [FEAT-079] [tier:haiku] [owner:Y1] [brief:briefs/streams/p2-y1.md#item-p2-y1-016]
  Needs: P2-Y1-005, sp3: GNOME switch indicator (user decision recorded in P0-X-002)
  Files: internal/platform/wayland/indicator_linux.go, internal/platform/wayland/indicator_integration_test.go
  Tests: integration (fake Notification portal): TestIndicator_AddsThenRemovesAfterDuration, TestIndicator_SecondShowReplaces, TestIndicator_PortalErrorReturned
  DoD: SG green; commit `feat(wayland): switch indicator [FEAT-079]`

- [ ] [P2-Y1-017] Wayland backend assembly and the host_linux.go Wayland branch [FEAT-074, FEAT-075, FEAT-068, FEAT-072] [tier:haiku] [owner:Y1] [brief:briefs/streams/p2-y1.md#item-p2-y1-017]
  Needs: P2-Y1-010, P2-Y1-011, P2-Y1-012, P2-Y1-013, P2-Y1-014, P2-Y1-015, P2-Y1-016, P2-X1-019
  Files: internal/platform/wayland/doc.go, internal/platform/wayland/backend_linux.go, internal/platform/host/host_linux.go (claim), internal/platform/wayland/backend_integration_test.go
  Tests: integration (GNOME box): TestOpen_WaylandAllAccessors, TestOpen_UnsupportedCompositorExplicitError, TestBackend_CloseIdempotent
  DoD: SG green; commit `feat(wayland): Wayland backend assembly [FEAT-074]`

- [ ] [P2-Y1-018] Wayland injection and clipboard render latency benches [FEAT-075, FEAT-078, FEAT-170] [tier:haiku] [owner:Y1] [brief:briefs/streams/p2-y1.md#item-p2-y1-018]
  Needs: P2-Y1-011, P2-Y1-013
  Files: internal/platform/wayland/bench_integration_test.go
  Tests: integration (GNOME box): TestInjectLatency_Report, TestRenderLatency_1MiBText, TestRenderLatency_10MiBPNG
  DoD: SG green; parent records the user's numbers; commit `perf(wayland): latency benches [FEAT-075]`

- [ ] [P2-Y1-019] Wayland probe tool [FEAT-168] [tier:haiku] [owner:Y1] [brief:briefs/streams/p2-y1.md#item-p2-y1-019]
  Needs: P2-Y1-017
  Files: internal/platform/wayland/probe/main.go, internal/platform/wayland/probe/cmd_input.go, internal/platform/wayland/probe/cmd_shell.go, internal/platform/wayland/probe/README.md
  Tests: `GOOS=linux CGO_ENABLED=0 go build ./internal/platform/wayland/probe` succeeds; TestProbeFlags_Validation
  DoD: SG green; commit `feat(wayland): probe tool [FEAT-168]`

- [ ] [P2-Y1-020] CI job: portal and EI tests against fake buses (no compositor) [FEAT-168] [tier:haiku] [owner:Y1] [brief:briefs/streams/p2-y1.md#item-p2-y1-020]
  Needs: P2-Y1-013, P2-Y1-016
  Files: .github/workflows/ci.yml (claim: adds job `integration-linux-portal-fakes` only)
  Tests: actionlint clean; ubuntu-24.04 job installs dbus, runs `dbus-run-session -- go test -tags integration -json -run 'Fake|Portal|Request|Tokens|Clipboard_(Request|SetSelection|Selection|Owner)|Indicator|RemoteDesktop|InputCapture_' ./internal/platform/wayland/...` (exact pattern in the brief), summary + logs; states in the job summary that compositor-backed tests run only on the user's GNOME box
  DoD: actions pinned; commit `ci: Wayland fake-bus integration job [FEAT-168]`

- [ ] [P2-Y1-021] Manual verification script for the GNOME box [FEAT-175, FEAT-181] [tier:haiku] [owner:Y1] [brief:briefs/streams/p2-y1.md#item-p2-y1-021]
  Needs: P2-Y1-018, P2-Y1-019, P2-Y1-020, P2-Y1-027
  Files: qa/manual/p2-y1-gnome.md
  Tests: script follows `qa/manual/README.md`; GNOME/mutter/xdg-desktop-portal versions; integration tests by hand with consent dialogs; probe flows (barrier activation, release, recreation after barrier change, injection square/text, GlobalShortcuts confirmation dialog and lock/panic activation while the pointer is home (D-114), clipboard across Text Editor, Firefox HTML and an image viewer, restore tokens on a second run, indicator, SNI tray with the AppIndicator extension on and off, lock/screensaver/suspend, theme switch, autostart, 125% fractional scaling, benches); states KDE Plasma is untested (R-09)
  DoD: commit `docs(qa): Y1 GNOME verification script [FEAT-175]`

- [ ] [P2-Y1-M01] Run the Y1 script on the GNOME box [FEAT-175, FEAT-074, FEAT-075, FEAT-078] [tier:haiku] [owner:USER] [brief:briefs/streams/p2-y1.md#item-p2-y1-m01]
  Needs: P2-Y1-021
  Files: qa/manual/results/p2-y1-gnome.md
  Tests: every step observed; versions and bench numbers recorded
  DoD: parent commits results and records portal versions in compatibility-matrix.md; commit `docs(qa): Y1 GNOME results [FEAT-175]`

- [ ] [P2-Y1-022] Y1 code-review gate [FEAT-168] [tier:sonnet why:code-review gate run by the code-reviewer agent] [owner:Y1] [brief:briefs/streams/p2-y1.md#item-p2-y1-022]
  Needs: P2-Y1-001 .. P2-Y1-021, P2-Y1-026, P2-Y1-027
  Files: none (verdict NOTE by the parent; findings become appended fix items)
  Tests: `.claude/agents/code-reviewer.md` over the merged Y1 diff
  DoD: VERDICT pass or findings fixed

- [ ] [P2-Y1-023] Y1 security review (portal sessions, restore tokens, clipboard privacy) [FEAT-173] [tier:sonnet why:security-review gate with qa/security-review.md sections 5 and 7 plus portal session handling] [owner:Y1] [brief:briefs/streams/p2-y1.md#item-p2-y1-023]
  Needs: P2-Y1-022, P2-Y1-M01
  Files: none (verdict NOTE by the parent; findings become appended fix items)
  Tests: EI decoder limits and fuzzing, token file permissions, sessions closed on every exit path, clipboard served only on transfer requests, concealed content is not special-cased (D-121)
  DoD: VERDICT pass or findings fixed

- [ ] [P2-Y1-024] Y1 UI/UX audit of the indicator and tray on GNOME [FEAT-160, FEAT-079, FEAT-068] [tier:sonnet why:UI/UX audit gate with qa/ui-ux-audit.md] [owner:Y1] [brief:briefs/streams/p2-y1.md#item-p2-y1-024]
  Needs: P2-Y1-023
  Files: none (verdict NOTE by the parent)
  Tests: indicator wording, timing and placement from the user's results; tray behaviour with and without the AppIndicator extension; consent and degraded-mode messages are actionable
  DoD: VERDICT pass or findings fixed

- [ ] [P2-Y1-025] Y1 consolidation review [FEAT-168] [tier:opus] [owner:PARENT] [brief:briefs/streams/p2-y1.md#item-p2-y1-025]
  Needs: P2-Y1-024
  Files: briefs/plan-phase-2.md (NOTE "stream Y1 review chain green")
  Tests: D-02, D-05 (guard behaviour under activation), D-10, D-39, D-61 honoured; R-17, R-19, R-20, R-50 mitigations evidenced
  DoD: NOTE appended; commit `chore(plan): Y1 review chain green [FEAT-168]`

- [ ] [P2-Y1-026] GlobalShortcuts portal client: session, BindShortcuts, ListShortcuts and signals (D-114) [FEAT-181] [tier:sonnet why:asynchronous portal session flow with one-shot BindShortcuts semantics, a user confirmation dialog and compositor-chosen triggers] [owner:Y1] [brief:briefs/streams/p2-y1.md#item-p2-y1-026]
  Needs: P2-Y1-005, P2-Y1-015, D-114
  Files: internal/platform/wayland/portal/globalshortcuts_linux.go, internal/platform/wayland/portal/shortcuttrigger.go, internal/platform/wayland/portal/shortcuttrigger_test.go, internal/platform/wayland/portal/globalshortcuts_integration_test.go
  Tests: TestTrigger_FromBindingCtrlAltPause, TestTrigger_FromBindingBareScrollLock, TestTrigger_UnrepresentableKeyIsError, TestShortcutIDs_StableAndValid; integration (fake portal): TestGlobalShortcuts_CreateSessionThenBind, TestGlobalShortcuts_ChangedSetRecreatesSession, TestGlobalShortcuts_ActivatedDeliversShortcutID, TestGlobalShortcuts_ShortcutsChangedUpdatesTriggers, TestGlobalShortcuts_InterfaceMissingIsErrUnsupported, TestGlobalShortcuts_UserCancelIsErrPortalCancelled
  DoD: SG green; HC85 for `portal`; commit `feat(portal): GlobalShortcuts client [FEAT-181]`

- [ ] [P2-Y1-027] Wayland home hotkeys: HomeShortcutProvider on the capturer, binding sync and capability detection (D-114) [FEAT-181, FEAT-077] [tier:haiku] [owner:Y1] [brief:briefs/streams/p2-y1.md#item-p2-y1-027]
  Needs: P2-Y1-026, P2-Y1-010, P1-F-023
  Files: internal/platform/wayland/homeshortcuts.go, internal/platform/wayland/homeshortcuts_test.go, internal/platform/wayland/homecaps.go, internal/platform/wayland/homecaps_test.go, internal/platform/wayland/homeshortcuts_integration_test.go
  Tests: TestHomeShortcuts_SetMapsEngineShortcutsToPortal, TestHomeShortcuts_ChangedBindingsRecreateSession, TestHomeShortcuts_ActivationCallsCallbackWithEngineID, TestHomeShortcuts_StatusBoundShowsCompositorTriggers, TestHomeShortcuts_StatusUnavailableNamesReason, TestHomeShortcuts_RejectsShortcutsOtherThanLockAndPanic, TestHomeCaps_AvailableWhenInterfaceAndVersionPresent, TestHomeCaps_InterfaceMissingUnavailable, TestHomeCaps_DerivedFromProbeNotFromGnomeVersion; integration (GNOME box): TestHomeShortcuts_LockActivatedWhileHome (the confirmation dialog is answered by hand)
  DoD: SG+race green; HC85 for `wayland`; an unavailable status always carries a reason; commit `feat(wayland): home shortcuts via GlobalShortcuts [FEAT-181]`

### M1 - macOS via purego (CI-verified; no hand testing available)

- [ ] [P2-M1-001] Framework loading and CoreFoundation/CoreGraphics/ApplicationServices bindings [FEAT-052, FEAT-053, FEAT-054, FEAT-055] [tier:haiku] [owner:M1] [brief:briefs/streams/p2-m1.md#item-p2-m1-001]
  Needs: P1-F-022, sp4: purego bindings and CGPoint by value
  Files: internal/platform/darwin/fw/doc.go, internal/platform/darwin/fw/frameworks_darwin.go, internal/platform/darwin/fw/cf_darwin.go, internal/platform/darwin/fw/cg_darwin.go, internal/platform/darwin/fw/cgevent_darwin.go, internal/platform/darwin/fw/ax_darwin.go, internal/platform/darwin/fw/consts.go, internal/platform/darwin/fw/consts_test.go, internal/platform/darwin/fw/fw_integration_test.go
  Tests: TestConsts_ValuesFromSDKHeaders (host table), TestConsts_FlagMasksDisjoint; integration (macOS CI): TestFW_EverySymbolResolves, TestFW_WarpAndReadBackPointByValue
  DoD: SG green (darwin arm64 + amd64 builds); HC85 for `fw`; commit `feat(darwin): framework bindings [FEAT-052]`

- [ ] [P2-M1-002] Objective-C helpers [FEAT-057, FEAT-059] [tier:haiku] [owner:M1] [brief:briefs/streams/p2-m1.md#item-p2-m1-002]
  Needs: P2-M1-001
  Files: internal/platform/darwin/objcx/doc.go, internal/platform/darwin/objcx/objcx_darwin.go, internal/platform/darwin/objcx/strings_darwin.go, internal/platform/darwin/objcx/pool_darwin.go, internal/platform/darwin/objcx/objcx_integration_test.go
  Tests: integration (macOS CI): TestNSString_RoundTripUnicode, TestNSData_RoundTrip, TestNSArray_OfStrings, TestPool_DrainsAutoreleased, TestSelectors_CachedOnce
  DoD: SG green; commit `feat(darwin): Objective-C helpers [FEAT-057]`

- [ ] [P2-M1-003] Main-thread runner and NSApplication accessory app [FEAT-059, FEAT-060] [tier:sonnet why:AppKit main-thread ownership from Go via purego] [owner:M1] [brief:briefs/streams/p2-m1.md#item-p2-m1-003]
  Needs: P2-M1-002, D-84, P1-C5-017
  Files: internal/platform/host/runmain_darwin.go (`host.RunMain` calling `appkit.RunMain`, D-84), internal/platform/darwin/appkit/doc.go, internal/platform/darwin/appkit/mainthread_darwin.go, internal/platform/darwin/appkit/app_darwin.go, internal/platform/darwin/appkit/queue.go, internal/platform/darwin/appkit/queue_test.go, internal/platform/darwin/appkit/main_integration_test.go
  Tests: TestQueue_FIFOAndClose, TestQueue_DoAfterCloseErrClosed (host); integration (macOS CI, TestMain locks the main thread): TestMain_RunsOnMainThread, TestApp_AccessoryPolicy, TestApp_StopReturnsFromRun
  DoD: SG+race green (host part); HC85 for `appkit`; commit `feat(darwin): AppKit main-thread runner [FEAT-059]`

- [ ] [P2-M1-004] CGEvent translation (pure) [FEAT-052, FEAT-029, FEAT-030] [tier:haiku] [owner:M1] [brief:briefs/streams/p2-m1.md#item-p2-m1-004]
  Needs: P2-M1-001, P1-C3-003, D-87
  Files: internal/platform/darwin/input/doc.go, internal/platform/darwin/input/translate.go, internal/platform/darwin/input/translate_test.go, internal/platform/darwin/input/flags.go, internal/platform/darwin/input/flags_test.go
  Tests: TestTranslate_KeycodesToUsage, TestTranslate_AutorepeatField, TestTranslate_ButtonsLeftRightOther, TestTranslate_OtherButtonNumbers234, TestTranslate_ScrollLinesAndContinuous, TestTranslate_MotionDeltaFields, TestFlags_DiffLeftRightModifiers, TestFlags_CapsLockToggle, TestFlags_FnKey, TestTranslate_AllocsZero
  DoD: SG green; HC85 for `input`; commit `feat(darwin): event translation [FEAT-052]`

- [ ] [P2-M1-005] Event tap thread with CFRunLoop, re-enable, edge contacts and swallowing [FEAT-052, FEAT-019] [tier:sonnet why:CGEventTap callbacks on a locked CFRunLoop thread with disable/re-enable semantics] [owner:M1] [brief:briefs/streams/p2-m1.md#item-p2-m1-005]
  Needs: P2-M1-004, P1-C3-006, P1-C3-007, D-88, D-80, sp4: session tap
  Files: internal/platform/darwin/input/tap_darwin.go, internal/platform/darwin/input/runloop_darwin.go, internal/platform/darwin/input/contact.go, internal/platform/darwin/input/contact_test.go, internal/platform/darwin/input/swallow.go, internal/platform/darwin/input/swallow_test.go, internal/platform/darwin/input/tap_integration_test.go
  Tests: TestContact_EachSidePoints, TestContact_SegmentBounds, TestSwallow_LookupAllocsZero; integration (macOS CI): TestTap_CreateReportsTrustState, TestTap_ObservesPostedEventsWhenTrusted (skipped with the TCC state as reason when untrusted), TestTap_ReEnableCountedOnDisable, TestTap_StopsRunLoopOnClose
  DoD: SG green; HC85 for `input`; commit `feat(darwin): event tap [FEAT-052]`

- [ ] [P2-M1-006] Grab, cursor association and hiding; macOS InputCapturer [FEAT-053, FEAT-052] [tier:sonnet why:cursor association and background hiding through a private CGS property inside an accessory app] [owner:M1] [brief:briefs/streams/p2-m1.md#item-p2-m1-006]
  Needs: P2-M1-005, P2-M1-003, P2-M1-008
  Files: internal/platform/darwin/input/grab_darwin.go, internal/platform/darwin/input/cursor_darwin.go, internal/platform/darwin/input/capturer_darwin.go, internal/platform/darwin/input/driver_darwin_test.go, internal/platform/darwin/input/capturer_integration_test.go
  Tests: integration (macOS CI): TestCapturer_Conformance (CGEventPost driver; evidence level logged), TestCursor_HideShowReturnCodes, TestCursor_BackgroundPropertyMissingIsExplicitError, TestCapturer_UngrabWarpsAndReassociates
  DoD: SG green; commit `feat(darwin): capturer with grab [FEAT-053]`

- [ ] [P2-M1-007] Injector via CGEventPost [FEAT-054, FEAT-029, FEAT-030] [tier:haiku] [owner:M1] [brief:briefs/streams/p2-m1.md#item-p2-m1-007]
  Needs: P2-M1-004, P2-M1-008, P1-C3-003, D-87, D-89, sp4: injection fields
  Files: internal/platform/darwin/input/inject_darwin.go, internal/platform/darwin/input/coords.go, internal/platform/darwin/input/coords_test.go, internal/platform/darwin/input/clickstate.go, internal/platform/darwin/input/clickstate_test.go, internal/platform/darwin/input/scrollaccum.go, internal/platform/darwin/input/scrollaccum_test.go, internal/platform/darwin/input/inject_integration_test.go
  Tests: TestCoords_PhysicalToGlobalPointsPerMonitor, TestCoords_OutsideEveryMonitorIsErrOutOfDesktop, TestClickState_DoubleAndTripleWithinInterval, TestClickState_ResetOnDistance, TestScrollAccum_Lines; integration (macOS CI): TestInjector_Conformance, TestInjector_MoveReadBack, TestInjector_ModifierFlagsCarried, TestInjector_DragWhileButtonHeld
  DoD: SG green; HC85 for `input`; commit `feat(darwin): CGEventPost injector [FEAT-054]`

- [ ] [P2-M1-008] Displays with points-to-pixels scale and reconfiguration watch [FEAT-055] [tier:haiku] [owner:M1] [brief:briefs/streams/p2-m1.md#item-p2-m1-008]
  Needs: P2-M1-001, P2-M1-002, D-89
  Files: internal/platform/darwin/display/doc.go, internal/platform/darwin/display/display_darwin.go, internal/platform/darwin/display/desktop.go, internal/platform/darwin/display/desktop_test.go, internal/platform/darwin/display/display_integration_test.go
  Tests: TestDesktop_PointsTimesScale, TestDesktop_MixedScaleMonitors, TestDesktop_PrimaryIsMainDisplay, TestDesktop_EqualityAndDebounce (synctest); integration (macOS CI): TestDisplays_Conformance, TestDisplays_AtLeastOneWithScale
  DoD: SG green; HC85 for `display`; commit `feat(darwin): displays [FEAT-055]`

- [ ] [P2-M1-009] Permissions: Accessibility and Input Monitoring [FEAT-056] [tier:haiku] [owner:M1] [brief:briefs/streams/p2-m1.md#item-p2-m1-009]
  Needs: P2-M1-001
  Files: internal/platform/darwin/permissions/doc.go, internal/platform/darwin/permissions/permissions_darwin.go, internal/platform/darwin/permissions/state.go, internal/platform/darwin/permissions/state_test.go, internal/platform/darwin/permissions/permissions_integration_test.go
  Tests: TestState_BuildsBothKinds, TestState_UnknownKindIsUnsupported; integration (macOS CI): TestPermissions_StatusReportsBothKinds (logs values), TestPermissions_RequestUnknownKindUnsupported
  DoD: SG green; HC85 for `permissions`; commit `feat(darwin): permissions [FEAT-056]`

- [ ] [P2-M1-010] Pasteboard watch and read [FEAT-057, FEAT-088] [tier:haiku] [owner:M1] [brief:briefs/streams/p2-m1.md#item-p2-m1-010]
  Needs: P2-M1-002, P1-C4-002, P1-C4-003, P1-C4-004, P1-C4-005, P1-C4-006, D-86
  Files: internal/platform/darwin/clipboard/doc.go, internal/platform/darwin/clipboard/watch_darwin.go, internal/platform/darwin/clipboard/read_darwin.go, internal/platform/darwin/clipboard/utis.go, internal/platform/darwin/clipboard/utis_test.go, internal/platform/darwin/clipboard/poll.go, internal/platform/darwin/clipboard/poll_test.go
  Tests: TestUTIs_ToCanonical, TestUTIs_TIFFMapsToPNG, TestPoll_ChangeCountEdges, TestPoll_OwnWriteSuppressed (synctest)
  DoD: SG green; HC85 for `clipboard`; commit `feat(darwin): pasteboard watch and read [FEAT-057]`

- [ ] [P2-M1-011] Lazy pasteboard offer via NSPasteboardItemDataProvider; macOS clipboard backend [FEAT-057] [tier:sonnet why:Objective-C class registration with lazy provider callbacks on the AppKit main thread] [owner:M1] [brief:briefs/streams/p2-m1.md#item-p2-m1-011]
  Needs: P2-M1-010, P2-M1-003, sp4: lazy provider
  Files: internal/platform/darwin/clipboard/provider_darwin.go, internal/platform/darwin/clipboard/offer_darwin.go, internal/platform/darwin/clipboard/backend_darwin.go, internal/platform/darwin/clipboard/clipboard_integration_test.go
  Tests: integration (macOS CI): TestClipboard_Conformance, TestOffer_ProviderCalledOncePerType, TestOffer_ProviderErrorLeavesTypeEmptyAndLogs, TestOffer_FinishedReleasesProvider, TestWatch_OwnOfferNotReported, TestWatch_ReadsTypesOnlyNeverData
  DoD: SG green; commit `feat(darwin): lazy pasteboard offer [FEAT-057]`

- [ ] [P2-M1-012] Pasteboard render latency bench [FEAT-057, FEAT-170] [tier:haiku] [owner:M1] [brief:briefs/streams/p2-m1.md#item-p2-m1-012]
  Needs: P2-M1-011
  Files: internal/platform/darwin/clipboard/bench_integration_test.go
  Tests: integration (macOS CI): TestRenderLatency_1MiBText, TestRenderLatency_10MiBPNG
  DoD: SG green; commit `perf(darwin): pasteboard render latency bench [FEAT-057]`

- [ ] [P2-M1-013] Session events and actions [FEAT-058, FEAT-182] [tier:haiku] [owner:M1] [brief:briefs/streams/p2-m1.md#item-p2-m1-013]
  Needs: P2-M1-002, P2-M1-003, D-115
  Files: internal/platform/darwin/session/doc.go, internal/platform/darwin/session/session_darwin.go, internal/platform/darwin/session/actions_darwin.go, internal/platform/darwin/session/names.go, internal/platform/darwin/session/names_test.go, internal/platform/darwin/session/session_integration_test.go
  Tests: TestNames_NotificationToKind, TestNames_UnknownIgnoredAndCounted; integration (macOS CI): TestSession_Conformance (Lock only with `EDGESYNC_ALLOW_LOCK=1`), TestActions_LockSymbolPresenceLogged, TestActions_WakeDisplayAssertion
  DoD: SG green; HC85 for `session`; commit `feat(darwin): session events and actions [FEAT-058]`

- [ ] [P2-M1-014] Tray via NSStatusItem [FEAT-059] [tier:haiku] [owner:M1] [brief:briefs/streams/p2-m1.md#item-p2-m1-014]
  Needs: P2-M1-003, P2-W1-003
  Files: internal/platform/darwin/tray/doc.go, internal/platform/darwin/tray/tray_darwin.go, internal/platform/darwin/tray/menumodel.go, internal/platform/darwin/tray/menumodel_test.go, internal/platform/darwin/tray/tray_integration_test.go
  Tests: TestMenuModel_TagsStable, TestMenuModel_StatesAndSubmenus; integration (macOS CI): TestTray_Conformance, TestTray_ActionSelectorInvokesOnAction, TestTray_TemplateImageSet
  DoD: SG green; HC85 for `tray`; commit `feat(darwin): status item tray [FEAT-059]`

- [ ] [P2-M1-015] Switch overlay panel [FEAT-060] [tier:haiku] [owner:M1] [brief:briefs/streams/p2-m1.md#item-p2-m1-015]
  Needs: P2-M1-003, P2-M1-008
  Files: internal/platform/darwin/overlay/doc.go, internal/platform/darwin/overlay/overlay_darwin.go, internal/platform/darwin/overlay/geometry.go, internal/platform/darwin/overlay/geometry_test.go, internal/platform/darwin/overlay/overlay_integration_test.go
  Tests: TestGeometry_CentredInScreenOfPointer, TestGeometry_AppKitBottomLeftConversion; integration (macOS CI): TestOverlay_PanelPropertiesNonActivatingIgnoresMouse, TestOverlay_HiddenAfterDuration, TestOverlay_ScreenshotArtifact
  DoD: SG green; HC85 for `overlay`; commit `feat(darwin): switch overlay [FEAT-060]`

- [ ] [P2-M1-016] Autostart via LaunchAgent plist [FEAT-061] [tier:haiku] [owner:M1] [brief:briefs/streams/p2-m1.md#item-p2-m1-016]
  Needs: P1-F-022
  Files: internal/platform/darwin/autostart/doc.go, internal/platform/darwin/autostart/autostart.go, internal/platform/darwin/autostart/autostart_test.go, internal/platform/darwin/autostart/autostart_darwin.go
  Tests: TestPlist_ContentExact, TestPlist_XMLEscaping, TestAutostart_EnableDisableRoundTrip, TestAutostart_ForeignProgramReportsDisabled, TestAutostart_Conformance (`conformance.RunAutostart` with a temp LaunchAgents dir, host-safe)
  DoD: SG green; HC85 for `autostart`; commit `feat(darwin): LaunchAgent autostart [FEAT-061]`

- [ ] [P2-M1-017] Theme watcher [FEAT-062] [tier:haiku] [owner:M1] [brief:briefs/streams/p2-m1.md#item-p2-m1-017]
  Needs: P2-M1-002, P2-M1-013
  Files: internal/platform/darwin/theme/doc.go, internal/platform/darwin/theme/theme_darwin.go, internal/platform/darwin/theme/theme_integration_test.go
  Tests: integration (macOS CI): TestTheme_Conformance, TestTheme_WatchRegistersDistributedObserver
  DoD: SG green; commit `feat(darwin): theme watcher [FEAT-062]`

- [ ] [P2-M1-018] macOS backend assembly and host_darwin.go [FEAT-052, FEAT-054, FEAT-057] [tier:haiku] [owner:M1] [brief:briefs/streams/p2-m1.md#item-p2-m1-018]
  Needs: P2-M1-006, P2-M1-007, P2-M1-009, P2-M1-011, P2-M1-013, P2-M1-014, P2-M1-015, P2-M1-016, P2-M1-017, D-84
  Files: internal/platform/darwin/doc.go, internal/platform/darwin/backend_darwin.go, internal/platform/host/host_darwin.go, internal/platform/darwin/backend_integration_test.go
  Tests: integration (macOS CI): TestOpen_AllAccessors, TestOpen_MissingTCCSurfacesErrPermission, TestBackend_CloseIdempotent
  DoD: SG green; commit `feat(darwin): macOS backend assembly [FEAT-052]`

- [ ] [P2-M1-019] macOS 13+ API availability audit [FEAT-168] [tier:haiku] [owner:M1] [brief:briefs/streams/p2-m1.md#item-p2-m1-019]
  Needs: P2-M1-018
  Files: internal/platform/darwin/fw/availability_test.go, internal/platform/darwin/fw/availability_integration_test.go
  Tests: TestAvailability_EveryBoundSymbolListed (host: go/ast scan of `RegisterLibFunc`/`Dlsym` names and Objective-C selector strings in `internal/platform/darwin/...`), TestAvailability_NoneIntroducedAfter13 (entries above 13.0 must name their runtime guard); integration: TestAvailability_ResolveOnRunner (logs `sw_vers`)
  DoD: SG green; table rows cite Apple documentation availability; commit `test(darwin): API availability audit [FEAT-168]`

- [ ] [P2-M1-020] macOS injection latency bench [FEAT-054, FEAT-170] [tier:haiku] [owner:M1] [brief:briefs/streams/p2-m1.md#item-p2-m1-020]
  Needs: P2-M1-007
  Files: internal/platform/darwin/input/inject_bench_integration_test.go
  Tests: integration (macOS CI): BenchmarkPostMove, TestInjectLatency_Report
  DoD: SG green; commit `perf(darwin): injection latency bench [FEAT-054]`

- [ ] [P2-M1-021] macOS probe tool [FEAT-168] [tier:haiku] [owner:M1] [brief:briefs/streams/p2-m1.md#item-p2-m1-021]
  Needs: P2-M1-018
  Files: internal/platform/darwin/probe/main.go, internal/platform/darwin/probe/cmd_input.go, internal/platform/darwin/probe/cmd_shell.go, internal/platform/darwin/probe/selftest.go, internal/platform/darwin/probe/README.md
  Tests: `GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build ./internal/platform/darwin/probe` and `GOARCH=amd64` succeed; TestProbeFlags_Validation
  DoD: SG green; `-selftest <duration>` exits on its own (used by CI); commit `feat(darwin): probe tool [FEAT-168]`

- [ ] [P2-M1-022] CI job: macOS backend integration tests [FEAT-168] [tier:haiku] [owner:M1] [brief:briefs/streams/p2-m1.md#item-p2-m1-022]
  Needs: P2-M1-019, P2-M1-020, P2-M1-021, P2-M1-012
  Files: .github/workflows/ci.yml (claim: adds job `integration-macos` only)
  Tests: actionlint clean; matrix macos-latest + macos-15-intel; logs `sw_vers` and TCC state; `go test -tags integration -json ./internal/platform/darwin/...`; probe `-selftest 5s` with `screencapture -x` artefacts; crash-report check under `~/Library/Logs/DiagnosticReports`; skip summary
  DoD: actions pinned; commit `ci: macOS integration job [FEAT-168]`

- [ ] [P2-M1-023] macOS verification scripts: CI dispatch now, hands-on script for a future Mac owner [FEAT-175] [tier:haiku] [owner:M1] [brief:briefs/streams/p2-m1.md#item-p2-m1-023]
  Needs: P2-M1-022
  Files: qa/manual/p2-m1-macos-ci.md, qa/manual/p2-m1-macos-handson.md
  Tests: both follow `qa/manual/README.md`; the CI script states that macOS is verified on CI only (D-37, R-07) and lists which artefacts to share; the hands-on script covers TCC grant flows (Accessibility, Input Monitoring), tray, overlay, capture/grab, injection, lazy paste into TextEdit/Pages/Preview, session events and theme, and is marked "not run until a Mac is available"
  DoD: commit `docs(qa): macOS verification scripts [FEAT-175]`

- [ ] [P2-M1-M01] Push and share the macOS CI run [FEAT-175, FEAT-168] [tier:haiku] [owner:USER] [brief:briefs/streams/p2-m1.md#item-p2-m1-m01]
  Needs: P2-M1-023
  Files: qa/manual/results/p2-m1-macos-ci.md
  Tests: run URL, summary table, TCC state, screenshots recorded
  DoD: parent commits results; behaviours that remain unverifiable are listed as a note under R-07; commit `docs(qa): macOS CI results [FEAT-175]`

- [ ] [P2-M1-024] M1 code-review gate [FEAT-168] [tier:sonnet why:code-review gate run by the code-reviewer agent] [owner:M1] [brief:briefs/streams/p2-m1.md#item-p2-m1-024]
  Needs: P2-M1-001 .. P2-M1-023, P2-M1-028
  Files: none (verdict NOTE by the parent; findings become appended fix items)
  Tests: `.claude/agents/code-reviewer.md` over the merged M1 diff
  DoD: VERDICT pass or findings fixed

- [ ] [P2-M1-025] M1 security review (permissions, private APIs, pasteboard privacy) [FEAT-173] [tier:sonnet why:security-review gate with qa/security-review.md section 7 plus TCC and private-API use] [owner:M1] [brief:briefs/streams/p2-m1.md#item-p2-m1-025]
  Needs: P2-M1-024, P2-M1-M01
  Files: none (verdict NOTE by the parent; findings become appended fix items)
  Tests: permission prompts only on explicit request; private symbols resolved defensively with explicit errors; pasteboard contents read only on fetch requests (R-15)
  DoD: VERDICT pass or findings fixed

- [ ] [P2-M1-026] M1 UI/UX audit of tray and overlay [FEAT-160, FEAT-059, FEAT-060] [tier:sonnet why:UI/UX audit gate with qa/ui-ux-audit.md] [owner:M1] [brief:briefs/streams/p2-m1.md#item-p2-m1-026]
  Needs: P2-M1-025
  Files: none (verdict NOTE by the parent)
  Tests: CI screenshots of the status item and overlay in light and dark appearance; template image correctness; menu wording
  DoD: VERDICT pass or findings fixed

- [ ] [P2-M1-027] M1 consolidation review [FEAT-168] [tier:opus] [owner:PARENT] [brief:briefs/streams/p2-m1.md#item-p2-m1-027]
  Needs: P2-M1-026
  Files: briefs/plan-phase-2.md (NOTE "stream M1 review chain green")
  Tests: D-34, D-39, D-51, D-61 honoured; every behaviour classified as CI-verified, compile-only or unverifiable; R-07, R-10 .. R-15 mitigations evidenced
  DoD: NOTE appended; commit `chore(plan): M1 review chain green [FEAT-168]`

- [ ] [P2-M1-028] macOS lock availability detection at start and explicit "lock unsupported" state (D-115) [FEAT-182, FEAT-058] [tier:haiku] [owner:M1] [brief:briefs/streams/p2-m1.md#item-p2-m1-028]
  Needs: P2-M1-013, D-115
  Files: internal/platform/darwin/session/locksupport.go, internal/platform/darwin/session/locksupport_test.go, internal/platform/darwin/session/locksupport_darwin.go
  Tests: TestLockSupport_SymbolPresent, TestLockSupport_FrameworkMissingReportsReason, TestLockSupport_SymbolMissingReportsReason, TestLockSupport_ReasonNamesFrameworkAndSymbol, TestLock_UnsupportedReturnsErrUnsupportedWithoutCalling, TestOpen_UnsupportedLogsWarnOnce (slog handler capture, fake resolver); integration (macOS CI): TestLockSupport_RealFrameworkResolution (logs the result; either outcome passes)
  DoD: SG green; HC85 for `session`; no fallback to the screensaver; commit `feat(darwin): lock availability detection [FEAT-182]`

### P2-X - phase exit

- [ ] [P2-X-002] P2 exit: criteria, pinned-version re-verification, phase marker [FEAT-168] [tier:opus] [owner:PARENT] [brief:briefs/streams/p2-w1.md#item-p2-x-002]
  Needs: P2-W1-029, P2-W2-035, P2-X1-025, P2-Y1-025, P2-M1-027, P2-X-001, P2-X-M01, P2-X-003
  Files: plan.md (append the phase-marker NOTE per plan.md rules), CLAUDE.md (mirror the marker), compatibility-matrix.md (append re-verification notes and recorded OS/portal versions), perf-budgets.md (results rows recorded during P2)
  Tests: plan.md P2 exit criteria all met (conformance under integration per backend on its CI job or user machine, Windows and GNOME manual results recorded, PB-07 recorded, review chains green incl. W2 security and UI/UX); every pinned module re-checked against proxy.golang.org (newer versions recorded as notes, never adopted silently)
  DoD: `task ci:local` green; commit `chore(plan): P2 exit [FEAT-168]`
