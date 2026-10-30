# EdgeSync - Phase 3 items

Read `plan.md` (contract, orchestration, item format) first. Stream briefs: `briefs/streams/p3-*.md`. The GUI
conventions and the GUI design in `briefs/streams/p3-f.md` (sections "Conventions" and "GUI design") are binding
for every P3 GUI item; every G1-G4 brief builds on them.

Cross-phase needs are written as stream references: `P1-C5-026` means every P1-C5 item is `[x]` and the
stream's "review chain green" note exists in `briefs/plan-phase-1.md`; likewise `P2-W1-029`, `P2-W2-035`.
The parent resolves them before dispatch.

GUI gates (named "GUI gates" in DoD lines; they add to the per-item gates of plan.md 1.5):
- `GOOS=linux CGO_ENABLED=0 go vet $(go list ./gui/... | grep -v '/gui/app$')` and the same with `GOOS=darwin GOARCH=arm64`
  (only `gui/app` and `cmd/edgesync` import `gioui.org/app`, the one Gio package that needs cgo on Linux/macOS)
- `PATH="/d/Development/msys2/ucrt64/bin:$PATH" CGO_ENABLED=1 CC=gcc go test -race ./gui/state/...` when the item touches `gui/state`
- pure packages (`gui/state/...`, `gui/theme/tokens`, `gui/widgets/format`, `gui/shell/vm`, `gui/*/vm`): coverage >= 85%
- Gio packages: every interaction named in `Tests:` is driven through `gui/widgets/guitest`; `guitest.AssertHitTargets` passes at scales 1.0, 1.5, 2.0
- UI items: the `qa/ui-ux-audit.md` sections named in the item's brief section are self-checked and listed on the report's TESTS line
- Never run `cmd/edgesync`, the agent, or `integration`-tagged tests; building is allowed

Shared files touched in P3 (claim per `ownership.md`): `go.mod`/`go.sum`, `cmd/edgesync/pages.go` (created by
P3-F-018; G items append their page factories), `proto/schema/ipc.capnp` + `internal/proto/capnp/**`,
`internal/config/schema.go`, `internal/i18n/locales/en.toml` (agent strings, G5 only), `tools/coverage-exclude.txt`,
`.github/workflows/ci.yml`. After P3-F-005 the per-area catalogue files are owned without claims:
`internal/i18n/locales/en/gui_shell.toml` (P3-F), `gui_layout.toml` (G1), `gui_machines.toml` (G2),
`gui_settings.toml` (G3), `gui_transfers.toml` (G4).

<!-- NOTE: 2026-10-29 - G5 has no GUI dependency. Its items list exact Needs (P3-F-006 and P2-W1/W2 complete); the parent may dispatch them as fill while P3-F is still running (plan.md contract fill rule). -->

## Phase 3 - GUI & integration

### P3-F - Foundation block (sequential, 1 agent, in order)

- [ ] [P3-F-001] Decide the GUI coverage policy (pure view-model packages in the 85% gate, Gio packages excluded but guitest-covered) [FEAT-160, FEAT-146] [tier:opus] [owner:PARENT] [brief:briefs/streams/p3-f.md#item-p3-f-001]
  Needs: P1-C5-026
  Files: briefs/plan-phase-3.md (append note confirming D-97 against the merged P1 code)
  Tests: D-97 (appended at the planning consolidation) names every included pure package pattern, every excluded Gio package, `test/e2e` excluded and `test/e2e/harness` included; the parent confirms the package list still matches `briefs/streams/p3-f.md` and `main`, and appends a superseding decision only if a package was renamed
  DoD: commit `docs(plan): GUI coverage policy confirmed (D-97) [FEAT-160]`

- [ ] [P3-F-002] Resolve the GUI spine gaps (environment checks, manual-address removal, GUI single instance, first-run state, per-area i18n files, GUI log file, theme on connect, pairing event variants) [FEAT-150, FEAT-068, FEAT-130, FEAT-141, FEAT-146] [tier:opus] [owner:PARENT] [brief:briefs/streams/p3-f.md#item-p3-f-002]
  Needs: P1-C5-026
  Files: briefs/plan-phase-3.md (append note confirming D-93 .. D-96 and the exact file names against `main`)
  Tests: every gap in the brief section "Spine gaps" is closed by D-93 (IPC/config), D-94 (per-area i18n), D-95 (GUI log), D-96 (internal/firewall) and the P3 ownership rows (appended at the planning consolidation); the parent confirms the P1 files named in D-93 exist on `main` with the expected APIs, quotes any element the P1 implementation already provides so P3-F-006 skips it, and appends a fix item if a name differs; each new IPC element stays additive (D-36) with a config migration where it touches `internal/config/schema.go`
  DoD: commit `docs(decisions): GUI IPC, config and i18n additions [FEAT-146]`

- [ ] [P3-F-003] UI/UX audit of the GUI design before any UI code [FEAT-160] [tier:sonnet why:UI/UX audit gate on the GUI design (qa/ui-ux-audit.md)] [owner:P3-F] [brief:briefs/streams/p3-f.md#item-p3-f-003]
  Needs: P3-F-002
  Files: none (read-only `code-reviewer` run; the parent appends findings as notes and fix items to the briefs)
  Tests: sections A-F of `qa/ui-ux-audit.md` applied to "GUI design" in `briefs/streams/p3-f.md` and to the screen sections of `p3-g1.md` .. `p3-g4.md` (screens list, states, navigation map, keyboard map, tokens)
  DoD: VERDICT pass, or every finding turned into an appended note/item by the parent before any G1-G4 item is dispatched; no commit by the reviewer

- [ ] [P3-F-004] Coverage exclusion patterns for the GUI policy [FEAT-160] [tier:haiku] [owner:P3-F] [brief:briefs/streams/p3-f.md#item-p3-f-004]
  Needs: P3-F-001, P3-F-002
  Files: tools/coverage-exclude.txt (claim)
  Tests: exact-package patterns verified against `tools/covercheck` matching rules; `task cover` passes on the current tree
  DoD: patterns exactly as in the P3-F-001 and P3-F-002 decisions; per-item gates green; commit `build(cover): GUI coverage policy patterns [FEAT-160]`

- [ ] [P3-F-005] Per-area i18n catalogue files [FEAT-146] [tier:haiku] [owner:P3-F] [brief:briefs/streams/p3-f.md#item-p3-f-005]
  Needs: P3-F-002
  Files: internal/i18n/i18n.go, internal/i18n/multifile_test.go, internal/i18n/testdata/multi/en.toml, internal/i18n/testdata/multi/en/area_a.toml, internal/i18n/testdata/multi/en/area_b.toml, internal/i18n/testdata/dup/en.toml, internal/i18n/testdata/dup/en/area.toml
  Tests: TestLoad_MergesBaseAndAreaFiles, TestLoad_DuplicateKeyAcrossFilesFails, TestLoad_AreaFileOutsideNamespaceFails, TestKeys_MissingAndUnusedDetectionSpansAllFiles, TestLoad_EmbeddedCatalogueLoads
  DoD: coverage of internal/i18n >= 85%; per-item gates green; commit `feat(i18n): per-area catalogue files [FEAT-146]`

- [ ] [P3-F-006] IPC and config additions for the GUI (environment checks, manual-address removal, GUI single instance, first-run state, theme on connect) [FEAT-150, FEAT-068, FEAT-130, FEAT-141, FEAT-146] [tier:sonnet why:additive change across the frozen IPC schema, generated code, IPC server/client and agent handlers] [owner:P3-F] [brief:briefs/streams/p3-f.md#item-p3-f-006]
  Needs: P3-F-002
  Files: proto/schema/ipc.capnp (claim), internal/proto/capnp/ipccp/ipc.capnp.go (regenerated, claim), internal/config/schema.go (claim), internal/config/migrate_gui.go, internal/config/migrate_gui_test.go, internal/ipc/gui_additions.go, internal/ipc/gui_additions_test.go, internal/agent/envchecks.go, internal/agent/envchecks_test.go, internal/agent/gui_requests.go, internal/agent/gui_requests_test.go, plus internal/ipc/server.go, internal/ipc/client.go, internal/agent/ipc_handler.go, internal/agent/ipc_peers.go
  Tests: TestIPC_GetEnvironmentChecksRoundTrip, TestIPC_EnvironmentChangedEvent, TestIPC_RemoveManualPeer, TestIPC_HelloReportsPrimaryGUI, TestIPC_RequestFocusGUIForwardsRoute, TestIPC_ThemeSentAfterHello, TestIPC_PairingProgressVariants, TestConfig_MigrateAddsGUITable, TestConfig_GUITableDefaults, TestAgent_EnvChecksFromChecker, TestAgent_TrayUnavailableReportedAsCheck, TestAgent_SecondGUIGetsPrimaryFalse
  DoD: only the elements approved by the P3-F-002 decisions; `task gen:check` green; coverage of internal/ipc, internal/agent and internal/config >= 85%; `task fuzz:smoke` green for the IPC decoders; per-item gates green; commit `feat(ipc): GUI additions [FEAT-146]`

- [ ] [P3-F-007] Theme tokens with WCAG contrast checks (pure) [FEAT-146, FEAT-160] [tier:haiku] [owner:P3-F] [brief:briefs/streams/p3-f.md#item-p3-f-007]
  Needs: P3-F-004
  Files: gui/theme/tokens/tokens.go, gui/theme/tokens/contrast.go, gui/theme/tokens/tokens_test.go, gui/theme/tokens/contrast_test.go
  Tests: TestContrast_KnownPairs, TestContrast_Symmetric, TestTokens_TextPairsMeetAA, TestTokens_LargeTextPairsMeet3, TestTokens_NonTextPairsMeet3, TestTokens_StateColoursDistinct, TestTokens_LightAndDarkComplete
  DoD: no import outside the standard library; coverage >= 85%; GUI gates green; commit `feat(gui): theme tokens with contrast checks [FEAT-160]`

- [ ] [P3-F-008] Formatting and validation helpers (sizes, speeds, durations, timestamps, host/port/name) [FEAT-156, FEAT-160, FEAT-130] [tier:haiku] [owner:P3-F] [brief:briefs/streams/p3-f.md#item-p3-f-008]
  Needs: P3-F-005, P3-F-007
  Files: gui/widgets/format/format.go, gui/widgets/format/validate.go, gui/widgets/format/format_test.go, gui/widgets/format/validate_test.go, internal/i18n/locales/en/gui_shell.toml
  Tests: TestBytes, TestSpeed, TestETA, TestDuration, TestCountdown, TestTimestamp, TestRelativeTime, TestValidateHost, TestValidatePort, TestValidateMachineName
  DoD: every user-visible unit and phrase via `i18n.T`; coverage >= 85%; GUI gates green; commit `feat(gui): formatting and validation helpers [FEAT-160]`

- [ ] [P3-F-009] GUI state client interface, IPC adapter, fake client and fixtures [FEAT-146] [tier:haiku] [owner:P3-F] [brief:briefs/streams/p3-f.md#item-p3-f-009]
  Needs: P3-F-006
  Files: gui/state/client.go, gui/state/ipcadapter.go, gui/state/ipcadapter_test.go, gui/state/statetest/fake.go, gui/state/statetest/fixtures.go, gui/state/statetest/fake_test.go
  Tests: TestAdapter_EveryRequestRoundTrips, TestAdapter_EventsDelivered, TestAdapter_ErrorInfoMapped, TestAdapter_CloseStopsEvents, TestFake_RecordsCallsAndScriptsResults, TestFake_EmitEvent, TestFixtures_TypicalEmptyErrorComplete
  DoD: the adapter covers every request of architecture section 7 plus the P3-F-006 additions (a missing request is BLOCKED, never skipped); coverage of gui/state/... >= 85%; `-race` green; GUI gates green; commit `feat(gui): state client, IPC adapter and fakes [FEAT-146]`

- [ ] [P3-F-010] GUI state store: connection lifecycle, snapshots, events, async operations [FEAT-146] [tier:sonnet why:concurrent store with an event stream, reconnect backoff and async operation tracking] [owner:P3-F] [brief:briefs/streams/p3-f.md#item-p3-f-010]
  Needs: P3-F-009
  Files: gui/state/store.go, gui/state/snapshot.go, gui/state/ops.go, gui/state/store_test.go, gui/state/ops_test.go
  Tests: TestStore_InitialSyncLoadsAllSlices, TestStore_SliceLoadErrorRecordedAndReload, TestStore_EventsUpdateSlices, TestStore_DisconnectReconnectBackoff (synctest), TestStore_RetryNowSkipsBackoff, TestStore_VersionMismatchState, TestStore_AuthFailedState, TestStore_SecondaryGUIRequestsFocusAndStops, TestStore_DoTracksPendingDoneFailed, TestStore_DoTimeout, TestStore_OpsPrunedAfter30s (synctest), TestStore_SnapshotImmutable, TestStore_InvalidateOnChange, TestStore_LogsBounded5000, TestStore_OpNamesAndErrorsOnlyInLogs, TestStore_CloseStopsGoroutines
  DoD: `-race` green; coverage >= 85%; GUI gates green; commit `feat(gui): state store [FEAT-146]`

- [ ] [P3-F-011] Agent locate-and-launch for the GUI (D-77) [FEAT-146, FEAT-141] [tier:haiku] [owner:P3-F] [brief:briefs/streams/p3-f.md#item-p3-f-011]
  Needs: P3-F-010
  Files: gui/state/agentproc/agentproc.go, gui/state/agentproc/agentproc_windows.go, gui/state/agentproc/agentproc_unix.go, gui/state/agentproc/agentproc_test.go, gui/state/agentproc/agentproc_windows_test.go, gui/state/agentproc/agentproc_unix_test.go
  Tests: TestAgentPath_SiblingOfExecutable, TestAgentPath_NeverSearchesPATH, TestCommand_DetachedAttributesWindows, TestCommand_SetsidUnix, TestEnsure_NoLaunchWhenAgentUp, TestEnsure_LaunchesOnceWhenEndpointMissing, TestEnsure_TimesOutAfter5s (synctest), TestEnsure_LaunchErrorReported, TestEnsure_MissingBinaryError
  DoD: tests never start a real process (injected starter); coverage >= 85%; GUI gates green; commit `feat(gui): agent launcher [FEAT-146]`

- [ ] [P3-F-012] Gio theme from tokens and the Gio dependency [FEAT-146] [tier:haiku] [owner:P3-F] [brief:briefs/streams/p3-f.md#item-p3-f-012]
  Needs: P3-F-007
  Files: go.mod, go.sum (claim), gui/theme/theme.go, gui/theme/theme_test.go
  Tests: TestTheme_LightPaletteFromTokens, TestTheme_DarkPaletteFromTokens, TestTheme_FontCollectionHasSansAndMono, TestTheme_TypographyScale, TestTheme_SwitchKeepsShaper
  DoD: `gioui.org v0.10.3` and `gioui.org/x v0.10.3` exactly (compatibility-matrix.md); the first line of each module's LICENSE file quoted in the report for the parent to record; GUI gates green; commit `feat(gui): Gio theme from tokens [FEAT-146]`

- [ ] [P3-F-013] guitest harness: frames, input, semantics, clipboard, hit targets [FEAT-160] [tier:haiku] [owner:P3-F] [brief:briefs/streams/p3-f.md#item-p3-f-013]
  Needs: P3-F-012
  Files: gui/widgets/guitest/guitest.go, gui/widgets/guitest/guitest_test.go
  Tests: TestHarness_FrameProducesSemantics, TestHarness_ClickByLabel, TestHarness_DragSequence, TestHarness_KeyAndShortcut, TestHarness_TypeText, TestHarness_ScrollWithModifiers, TestHarness_ClipboardWriteCaptured, TestHarness_AssertHitTargetsFlagsSmall, TestHarness_ScaleMetric, TestHarness_FocusMovesWithTab
  DoD: does not import `gioui.org/app`; GUI gates green; commit `test(gui): guitest harness [FEAT-160]`

- [ ] [P3-F-014] Feedback widgets and icons (banner, empty/loading/error states, badge, progress, spinner) [FEAT-146, FEAT-160] [tier:haiku] [owner:P3-F] [brief:briefs/streams/p3-f.md#item-p3-f-014]
  Needs: P3-F-008, P3-F-013
  Files: gui/widgets/icons.go, gui/widgets/banner.go, gui/widgets/states.go, gui/widgets/badge.go, gui/widgets/progress.go, gui/widgets/feedback_test.go, internal/i18n/locales/en/gui_shell.toml
  Tests: TestIcons_AllKindsDraw, TestBanner_KindsAndActions, TestBanner_SemanticLabel, TestEmptyState_ActionInvokes, TestLoadingState_SpinnerAnimates, TestErrorState_RetryInvokes, TestBadge_ShapeAndTextPerKind, TestProgress_Clamped0to1, TestFeedback_HitTargets
  DoD: GUI gates green; `qa/ui-ux-audit.md` A and C self-checked; commit `feat(gui): feedback widgets and icons [FEAT-160]`

- [ ] [P3-F-015] Input widgets (buttons, toggle, text and number fields, select, radio, tabs, dialogs, focus ring, pickers interface) [FEAT-146, FEAT-160] [tier:haiku] [owner:P3-F] [brief:briefs/streams/p3-f.md#item-p3-f-015]
  Needs: P3-F-014
  Files: gui/widgets/button.go, gui/widgets/toggle.go, gui/widgets/field.go, gui/widgets/selectbox.go, gui/widgets/radio.go, gui/widgets/tabs.go, gui/widgets/dialog.go, gui/widgets/focus.go, gui/widgets/pickers.go, gui/widgets/inputs_test.go, internal/i18n/locales/en/gui_shell.toml
  Tests: TestButton_ClickEnterSpaceActivate, TestButton_DisabledIgnoresInput, TestButton_PendingSpinnerAfter300ms, TestToggle_SpaceToggles, TestField_ErrorMessageShown, TestField_EnterCommitsAndBlurCommits, TestNumberField_DigitsOnlyAndRange, TestSelect_KeyboardChoose, TestRadio_ArrowKeysMove, TestTabs_CtrlTabCycles, TestDialog_EscapeClosesAndFocusTrapped, TestConfirm_EnterConfirmsEscapeCancels, TestFocusRing_DrawnOnlyWhenFocused, TestInputs_HitTargets
  DoD: GUI gates green; `qa/ui-ux-audit.md` B and C self-checked; commit `feat(gui): input widgets and dialogs [FEAT-160]`

- [ ] [P3-F-016] Shell view model: routes, navigation, intents, banners, status texts, shortcuts, first-run gate (pure) [FEAT-146] [tier:haiku] [owner:P3-F] [brief:briefs/streams/p3-f.md#item-p3-f-016]
  Needs: P3-F-010
  Files: gui/shell/vm/routes.go, gui/shell/vm/nav.go, gui/shell/vm/banners.go, gui/shell/vm/status.go, gui/shell/vm/shortcuts.go, gui/shell/vm/vm_test.go, internal/i18n/locales/en/gui_shell.toml
  Tests: TestRoutes_SidebarOrderAndHidden, TestNav_GoAndBack, TestNav_IntentParamsConsumedOnce, TestNav_DeepLinkValidation, TestGate_WizardUntilFirstRunCompleted, TestBanners_Priority, TestBanners_UpdateDismissedPerVersion, TestStatus_ServerTexts, TestStatus_ClientTexts, TestStatus_LockIndicator, TestStatus_ActiveTransfersCount, TestShortcuts_Map
  DoD: coverage >= 85%; GUI gates green; commit `feat(gui): shell view model [FEAT-146]`

- [ ] [P3-F-017] Shell view: sidebar, status bar, banners, page host, modal host, keyboard shortcuts [FEAT-146, FEAT-160] [tier:haiku] [owner:P3-F] [brief:briefs/streams/p3-f.md#item-p3-f-017]
  Needs: P3-F-015, P3-F-016
  Files: gui/shell/page.go, gui/shell/shell.go, gui/shell/sidebar.go, gui/shell/statusbar.go, gui/shell/modal.go, gui/shell/shell_test.go, internal/i18n/locales/en/gui_shell.toml
  Tests: TestShell_SidebarShowsRegisteredPages, TestShell_ObserversCalledEveryFrame, TestShell_ShortcutNavigates, TestShell_ConnectingState, TestShell_DisconnectedRetryAndStartAgent, TestShell_VersionMismatchState, TestShell_WizardGateFullWindow, TestShell_ModalEscapeAndFocusTrap, TestShell_BannerActionNavigates, TestShell_StatusBarTransfersNavigates, TestShell_ThemeSwitchLive, TestShell_TabOrder, TestShell_HitTargets
  DoD: GUI gates green; `qa/ui-ux-audit.md` A, B, C, D self-checked; commit `feat(gui): application shell [FEAT-146]`

- [ ] [P3-F-018] Gio app loop and cmd/edgesync: window, pickers, URL opening, deep links, perf log, logging, single instance, focus [FEAT-146, FEAT-141] [tier:haiku] [owner:P3-F] [brief:briefs/streams/p3-f.md#item-p3-f-018]
  Needs: P3-F-011, P3-F-017
  Files: gui/app/app.go, gui/app/pickers.go, gui/app/openurl.go, gui/app/openurl_windows.go, gui/app/openurl_darwin.go, gui/app/openurl_linux.go, gui/app/app_test.go, gui/state/perfstats/perfstats.go, gui/state/perfstats/perfstats_test.go, cmd/edgesync/main.go, cmd/edgesync/flags.go, cmd/edgesync/flags_test.go, cmd/edgesync/pages.go, cmd/edgesync/pages_test.go
  Tests: TestFlags_Defaults, TestFlags_PageValidated, TestFlags_SyntheticRange, TestFlags_LogLevel, TestPages_NoDuplicateRoutes, TestPerfStats_WindowPercentiles, TestOpenURL_Validation, TestPickerPath_FromOSFile, TestPickerPath_NotAFileError, TestFocus_RaiseAndNavigate
  DoD: `go build ./cmd/edgesync` with `CGO_ENABLED=0` on the host succeeds (never run); `cmd/edgesync/pages.go` holds the page list (empty until G items append); GUI gates green; commit `feat(gui): app loop and edgesync command [FEAT-146]`

- [ ] [P3-F-019] Purity and import-boundary tests for the GUI packages [FEAT-160] [tier:haiku] [owner:P3-F] [brief:briefs/streams/p3-f.md#item-p3-f-019]
  Needs: P3-F-018
  Files: gui/state/purity_test.go
  Tests: TestPurePackagesImportNoGio, TestOnlyAppImportsGioAppAndExplorer, TestPurePatternsMatchCoveragePolicy
  DoD: GUI gates green; commit `test(gui): import boundary checks [FEAT-160]`

- [ ] [P3-F-020] CI: Gio Linux build dependencies for lint, test and cover jobs [FEAT-146, FEAT-167] [tier:haiku] [owner:P3-F] [brief:briefs/streams/p3-f.md#item-p3-f-020]
  Needs: P3-F-018
  Files: .github/workflows/ci.yml (claim)
  Tests: `go tool -modfile=tools/tools.mod actionlint` clean; the ubuntu `lint`, `test` and `gen-check` jobs install the Gio Linux packages listed on https://gioui.org/doc/install/linux at execution time (list quoted in the report); macOS job prerequisites confirmed from the runner-images README
  DoD: commit `ci: Gio build dependencies for Linux jobs [FEAT-146]`

- [ ] [P3-F-021] Headless screenshot harness for every registered page (integration, Windows) [FEAT-160] [tier:haiku] [owner:P3-F] [brief:briefs/streams/p3-f.md#item-p3-f-021]
  Needs: P3-F-018
  Files: cmd/edgesync/screens_integration_test.go
  Tests (integration, Windows, user-run): TestScreens_RenderEveryPage writes one PNG per registered page x fixture {typical, empty, error} x theme {light, dark} x scale {1.0, 1.5, 2.0}, plus the shell connecting, disconnected and version-mismatch states, to `$EDGESYNC_SCREENS_OUT` (default `bin/screens/<UTC timestamp>/`); fails on a panic or an all-background image
  DoD: `go vet -tags integration ./cmd/edgesync/` green on the host; never run by the agent; commit `test(gui): headless page screenshots [FEAT-160]`

- [ ] [P3-F-022] Manual script for the GUI shell [FEAT-146, FEAT-160] [tier:haiku] [owner:P3-F] [brief:briefs/streams/p3-f.md#item-p3-f-022]
  Needs: P3-F-018, P3-F-021
  Files: qa/manual/p3-f-gui-shell.md
  Tests: script follows `qa/manual/README.md`; covers build commands, PB-21 launch-to-first-frame from the GUI log, agent auto-start, disconnected state and recovery, single instance, live theme switch, DPI 100/125/150/200 %, keyboard navigation, the screenshot harness run and where the PNGs land
  DoD: commit `docs(qa): P3-F GUI shell manual script [FEAT-146]`

- [ ] [P3-F-M01] Run the P3-F GUI shell script on Windows PC 1 and record results (incl. screenshots) [FEAT-146, FEAT-160] [tier:haiku] [owner:USER] [brief:briefs/streams/p3-f.md#item-p3-f-m01]
  Needs: P3-F-022
  Files: qa/manual/results/p3-f-gui-shell.md
  Tests: every step has a recorded observation; PB-21 number recorded; screenshot folder path recorded
  DoD: results committed by the parent `docs(qa): P3-F GUI shell results [FEAT-146]`

- [ ] [P3-F-R01] Code review of the P3-F stream [FEAT-146] [tier:sonnet why:per-stream code-review gate] [owner:P3-F] [brief:briefs/streams/p3-f.md#item-p3-f-r01]
  Needs: P3-F-004 .. P3-F-022 all [x]
  Files: none (read-only `code-reviewer`)
  Tests: every P3-F item diff checked against its item text and the brief
  DoD: VERDICT pass, or each finding appended by the parent as a fix item and fixed; no commit by the reviewer

- [ ] [P3-F-R02] Security review of the P3-F stream (IPC additions, GUI token handling, agent launch path, URL opening) [FEAT-146] [tier:sonnet why:per-stream security-review gate] [owner:P3-F] [brief:briefs/streams/p3-f.md#item-p3-f-r02]
  Needs: P3-F-R01
  Files: none (read-only `code-reviewer` with `qa/security-review.md`)
  Tests: sections 3, 5, 7 and 8 of `qa/security-review.md` plus the brief's extra points
  DoD: VERDICT pass or every finding fixed via appended items; no commit by the reviewer

- [ ] [P3-F-R03] UI/UX audit of the P3-F stream [FEAT-160] [tier:sonnet why:per-stream UI/UX audit gate] [owner:P3-F] [brief:briefs/streams/p3-f.md#item-p3-f-r03]
  Needs: P3-F-R02, P3-F-M01
  Files: none (read-only `code-reviewer` with `qa/ui-ux-audit.md`, plus the screenshots from P3-F-M01)
  Tests: sections A-D and F (banner/state parts) of `qa/ui-ux-audit.md`
  DoD: VERDICT pass or every finding fixed via appended items; no commit by the reviewer

- [ ] [P3-F-R04] Opus consolidation review of the P3-F stream [FEAT-146] [tier:opus] [owner:PARENT] [brief:briefs/streams/p3-f.md#item-p3-f-r04]
  Needs: P3-F-R03
  Files: briefs/plan-phase-3.md (append note)
  Tests: contract adherence (IPC additions only as decided), purity boundaries, decisions coverage (D-19, D-33, D-52, D-68, D-77 and the P3-F decisions)
  DoD: note `<!-- NOTE: <date> - stream P3-F review chain green -->` appended; commit `chore(plan): P3-F review chain green [FEAT-146]`

### G1 - Layout editor

- [ ] [P3-G1-001] Layout editor document model: machines, monitors, unplaced peers, dirty state, apply payload [FEAT-147, FEAT-003] [tier:haiku] [owner:G1] [brief:briefs/streams/p3-g1.md#item-p3-g1-001]
  Needs: P3-F-003, P3-F-018, P3-F-019
  Files: gui/layouteditor/vm/document.go, gui/layouteditor/vm/document_test.go
  Tests: TestDocument_PhysicalToLogicalConversion, TestDocument_HomeAndPlacedMachines, TestDocument_UnplacedPairedPeers, TestDocument_StateFlags, TestDocument_DirtyAndRevert, TestDocument_ApplyPayloadRoundTrip, TestDocument_64Machines
  DoD: coverage >= 85%; GUI gates green; commit `feat(gui): layout editor document model [FEAT-147]`

- [ ] [P3-G1-002] Layout editor viewport: zoom, pan, fit, transforms, hit testing [FEAT-147] [tier:haiku] [owner:G1] [brief:briefs/streams/p3-g1.md#item-p3-g1-002]
  Needs: P3-G1-001
  Files: gui/layouteditor/vm/viewport.go, gui/layouteditor/vm/viewport_test.go
  Tests: TestViewport_ScreenCanvasRoundTrip, TestViewport_ZoomAtKeepsPointFixed, TestViewport_ZoomClamped, TestViewport_FitWithMargin, TestViewport_FitSingleMachine, TestViewport_PanBy, TestHitTest_SelectedFirstThenTopmost, TestHitTest_Miss
  DoD: coverage >= 85%; GUI gates green; commit `feat(gui): layout editor viewport [FEAT-147]`

- [ ] [P3-G1-003] Drag with snapping, guides, overlap segments and cancel [FEAT-147, FEAT-006] [tier:sonnet why:drag and snap interaction state machine shared by pointer and keyboard paths] [owner:G1] [brief:briefs/streams/p3-g1.md#item-p3-g1-003]
  Needs: P3-G1-002
  Files: gui/layouteditor/vm/drag.go, gui/layouteditor/vm/drag_test.go
  Tests: TestDrag_MovesByCanvasDelta, TestDrag_SnapsWithinThreshold, TestDrag_NoSnapWhenDisabled, TestDrag_ThresholdScalesWithZoom, TestDrag_GuidesAndOverlapsReported, TestDrag_CancelRestoresOrigin, TestDrag_EndMarksDirty, TestDrag_ReadOnlyRejected, BenchmarkDragTo64
  DoD: coverage >= 85%; snapping delegates to the `internal/layout` helper (no second snap implementation); benchmark ns/op and allocs/op in the report; GUI gates green; commit `feat(gui): layout editor drag and snap [FEAT-147]`

- [ ] [P3-G1-004] Keyboard operations: nudge, select next/previous, remove from layout, place unplaced machine [FEAT-147, FEAT-160] [tier:haiku] [owner:G1] [brief:briefs/streams/p3-g1.md#item-p3-g1-004]
  Needs: P3-G1-003
  Files: gui/layouteditor/vm/keyboard.go, gui/layouteditor/vm/keyboard_test.go
  Tests: TestNudge_OneAndTen, TestSelectNextPrev_ReadingOrder, TestRemove_HomeNotRemovable, TestPlace_RightOfRightmostTopAligned, TestKeyboardOps_ReadOnlyRejected
  DoD: coverage >= 85%; GUI gates green; commit `feat(gui): layout editor keyboard operations [FEAT-147]`

- [ ] [P3-G1-005] Validation issues and edge links (overlaps, unreachable, duplicates, isolated home) [FEAT-006, FEAT-147] [tier:haiku] [owner:G1] [brief:briefs/streams/p3-g1.md#item-p3-g1-005]
  Needs: P3-G1-001
  Files: gui/layouteditor/vm/validate.go, gui/layouteditor/vm/validate_test.go
  Tests: TestIssues_Overlap, TestIssues_Unreachable, TestIssues_DuplicateMachine, TestIssues_HomeWithoutNeighbours, TestApply_BlockedByErrorsNotWarnings, TestEdgeLinks_SharedSegmentsOfSelected
  DoD: validation delegates to `internal/layout`; coverage >= 85%; GUI gates green; commit `feat(gui): layout validation issues [FEAT-006]`

- [ ] [P3-G1-006] Layout authority and live sync: read-only clients, external changes, conflicts, apply results [FEAT-007, FEAT-147] [tier:haiku] [owner:G1] [brief:briefs/streams/p3-g1.md#item-p3-g1-006]
  Needs: P3-G1-001
  Files: gui/layouteditor/vm/sync.go, gui/layouteditor/vm/sync_test.go
  Tests: TestSync_ClientReadOnlyWithServerName, TestSync_CleanDocRefreshes, TestSync_DirtyDocConflict, TestConflict_ReloadDiscards, TestConflict_KeepMineApplies, TestApply_ErrorKeepsDirty, TestApply_SuccessClearsDirty, TestSync_PeerRenameUpdatesLabel, TestSync_ActiveBadgeFollowsStatus
  DoD: coverage >= 85%; GUI gates green; commit `feat(gui): layout authority and sync [FEAT-007]`

- [ ] [P3-G1-007] Canvas rendering: machine groups, monitors, labels, state badges, selection, guides, overlaps [FEAT-147, FEAT-160] [tier:haiku] [owner:G1] [brief:briefs/streams/p3-g1.md#item-p3-g1-007]
  Needs: P3-G1-003, P3-G1-005
  Files: gui/layouteditor/canvas.go, gui/layouteditor/canvas_test.go, internal/i18n/locales/en/gui_layout.toml
  Tests: TestCanvas_SemanticNodePerMachine, TestCanvas_BadgeTextPerState, TestCanvas_SelectedOutline, TestCanvas_GuidesAndOverlapsWhileDragging, TestCanvas_MonitorLabelsHiddenWhenSmall, TestCanvas_HomeOnlyHint, TestCanvas_BothThemes
  DoD: GUI gates green; `qa/ui-ux-audit.md` C (machine rectangles, guides) self-checked; commit `feat(gui): layout canvas rendering [FEAT-147]`

- [ ] [P3-G1-008] Canvas input: pointer drag, pan, zoom, keyboard shortcuts, focus [FEAT-147, FEAT-160] [tier:sonnet why:Gio layout-editor interaction model (plan.md 1.4)] [owner:G1] [brief:briefs/streams/p3-g1.md#item-p3-g1-008]
  Needs: P3-G1-004, P3-G1-007
  Files: gui/layouteditor/input.go, gui/layouteditor/input_test.go
  Tests: TestInput_DragMovesAndSnaps, TestInput_AltDisablesSnap, TestInput_EscapeCancelsDrag, TestInput_MiddleDragPans, TestInput_ShortcutWheelZoomsAtCursor, TestInput_WheelPans, TestInput_ArrowNudgeShiftTen, TestInput_PageDownUpSelects, TestInput_DeleteRemoves, TestInput_FitKeys, TestInput_ZoomKeys, TestInput_ClickEmptyClearsSelection, TestInput_ReadOnlyIgnoresMutations, TestInput_ClickFocusesCanvas
  DoD: GUI gates green; `qa/ui-ux-audit.md` B self-checked; commit `feat(gui): layout canvas input [FEAT-147]`

- [ ] [P3-G1-009] Editor toolbar, side panel, machine list, unplaced list, issues list and banners [FEAT-147, FEAT-007, FEAT-160] [tier:haiku] [owner:G1] [brief:briefs/streams/p3-g1.md#item-p3-g1-009]
  Needs: P3-G1-006, P3-G1-007
  Files: gui/layouteditor/toolbar.go, gui/layouteditor/panel.go, gui/layouteditor/panel_test.go, internal/i18n/locales/en/gui_layout.toml
  Tests: TestToolbar_ApplyRevertEnablement, TestToolbar_UnappliedIndicator, TestToolbar_ShortcutSApplies, TestToolbar_PendingApply, TestPanel_SelectedDetails, TestPanel_MachineListKeyboardSelect, TestPanel_PlaceUnplaced, TestPanel_IssueSelectsMachines, TestBanners_ReadOnlyConflictSyntheticApplyError, TestPanel_HitTargets
  DoD: GUI gates green; `qa/ui-ux-audit.md` A, B, D, E (layout editor) self-checked; commit `feat(gui): layout editor panels [FEAT-147]`

- [ ] [P3-G1-010] Layout page: states, intents, registration [FEAT-147] [tier:haiku] [owner:G1] [brief:briefs/streams/p3-g1.md#item-p3-g1-010]
  Needs: P3-G1-008, P3-G1-009
  Files: gui/layouteditor/page.go, gui/layouteditor/page_test.go, cmd/edgesync/pages.go (claim), internal/i18n/locales/en/gui_layout.toml
  Tests: TestPage_LoadingEmptyErrorStates, TestPage_SelectIntent, TestPage_DocKeptAcrossNavigation, TestPage_SyntheticMode, TestPages_NoDuplicateRoutes (existing, still green)
  DoD: GUI gates green; commit `feat(gui): layout editor page [FEAT-147]`

- [ ] [P3-G1-011] Synthetic layouts and editor frame benchmarks (PB-19 proxy) [FEAT-172, FEAT-147] [tier:haiku] [owner:G1] [brief:briefs/streams/p3-g1.md#item-p3-g1-011]
  Needs: P3-G1-010
  Files: gui/layouteditor/vm/synthetic.go, gui/layouteditor/vm/synthetic_test.go, gui/layouteditor/bench_test.go
  Tests: TestSynthetic_DeterministicCount, TestSynthetic_NoOverlapsAllReachable, TestSynthetic_ReadOnlyReason, BenchmarkEditorFrame64, BenchmarkEditorDragFrame64
  DoD: benchmark numbers (ns/op, allocs/op) in the report for the parent's perf-budgets.md results row (proxy for PB-19); coverage of the vm package >= 85%; GUI gates green; commit `perf(gui): synthetic layouts and editor benchmarks [FEAT-172]`

- [ ] [P3-G1-012] Manual script for the layout editor (incl. PB-19 and PB-18) [FEAT-147, FEAT-172] [tier:haiku] [owner:G1] [brief:briefs/streams/p3-g1.md#item-p3-g1-012]
  Needs: P3-G1-011
  Files: qa/manual/p3-g1-layout-editor.md
  Tests: script follows `qa/manual/README.md`; covers arranging the real machines, snapping, apply/revert, conflict, client read-only view, keyboard-only arrangement, DPI and theme checks, `-synthetic-layout 64 -perflog` frame times while dragging (PB-19) and GUI working set (PB-18)
  DoD: commit `docs(qa): layout editor manual script [FEAT-147]`

- [ ] [P3-G1-M01] Run the layout editor script on Windows PC 1 (server) and PC 2 (client) and record results [FEAT-147, FEAT-172] [tier:haiku] [owner:USER] [brief:briefs/streams/p3-g1.md#item-p3-g1-m01]
  Needs: P3-G1-012
  Files: qa/manual/results/p3-g1-layout-editor.md
  Tests: every step has a recorded observation; PB-18 and PB-19 numbers recorded
  DoD: results committed by the parent `docs(qa): layout editor results [FEAT-147]`

- [ ] [P3-G1-R01] Code review of the G1 stream [FEAT-147] [tier:sonnet why:per-stream code-review gate] [owner:G1] [brief:briefs/streams/p3-g1.md#item-p3-g1-r01]
  Needs: P3-G1-001 .. P3-G1-012 all [x]
  Files: none (read-only `code-reviewer`)
  Tests: every G1 diff against its item and brief
  DoD: VERDICT pass or every finding fixed via appended items; no commit by the reviewer

- [ ] [P3-G1-R02] UI/UX audit of the layout editor [FEAT-160, FEAT-147] [tier:sonnet why:per-stream UI/UX audit gate] [owner:G1] [brief:briefs/streams/p3-g1.md#item-p3-g1-r02]
  Needs: P3-G1-R01, P3-G1-M01
  Files: none (read-only `code-reviewer` with `qa/ui-ux-audit.md` and the screenshots of the layout route)
  Tests: sections A, B, C, D, E (layout editor) of `qa/ui-ux-audit.md`
  DoD: VERDICT pass or every finding fixed via appended items; no commit by the reviewer

- [ ] [P3-G1-R03] Opus consolidation review of the G1 stream [FEAT-147] [tier:opus] [owner:PARENT] [brief:briefs/streams/p3-g1.md#item-p3-g1-r03]
  Needs: P3-G1-R02
  Files: briefs/plan-phase-3.md (append note), perf-budgets.md (append PB-18/PB-19 results rows)
  Tests: D-04, D-30, D-51, D-60 honoured; snapping/validation only via `internal/layout`; PB-19 met or a user-approved exception recorded
  DoD: note `<!-- NOTE: <date> - stream G1 review chain green -->`; commit `chore(plan): G1 review chain green [FEAT-147]`

### G2 - Machines, pairing, permissions and first-run wizard

- [ ] [P3-G2-001] Machines view model: paired and discovered rows, states, sorting, searching [FEAT-148] [tier:haiku] [owner:G2] [brief:briefs/streams/p3-g2.md#item-p3-g2-001]
  Needs: P3-F-003, P3-F-018, P3-F-019
  Files: gui/peers/vm/rows.go, gui/peers/vm/rows_test.go
  Tests: TestRows_PairedSortedConnectedFirst, TestRows_DiscoveredExcludesPaired, TestRows_StateTexts, TestRows_BusyShowsHolder, TestRows_ShortIDFormat, TestRows_SearchingThenEmptyAfter5s (synctest), TestRows_ClientRoleHidesConnect, TestRows_ManualSourceMarked
  DoD: coverage >= 85%; GUI gates green; commit `feat(gui): machines view model [FEAT-148]`

- [ ] [P3-G2-002] Machines actions: connect, disconnect, rename, unpair, wake, add or remove address, intents [FEAT-148, FEAT-130] [tier:haiku] [owner:G2] [brief:briefs/streams/p3-g2.md#item-p3-g2-002]
  Needs: P3-G2-001
  Files: gui/peers/vm/actions.go, gui/peers/vm/actions_test.go
  Tests: TestActions_AvailabilityMatrix, TestRename_Validation, TestUnpair_RequiresConfirm, TestWake_OfflineWithMACOnly, TestAddAddress_Validation, TestAddAddress_DefaultPort, TestRemoveManualAddress, TestIntents_SendFilesAndShowInLayout, TestActions_ErrorPerRow
  DoD: coverage >= 85%; GUI gates green; commit `feat(gui): machines actions [FEAT-148]`

- [ ] [P3-G2-003] Machines page view with rename, unpair and add-by-address dialogs; registration [FEAT-148, FEAT-160] [tier:haiku] [owner:G2] [brief:briefs/streams/p3-g2.md#item-p3-g2-003]
  Needs: P3-G2-002
  Files: gui/peers/page.go, gui/peers/rows.go, gui/peers/dialogs.go, gui/peers/page_test.go, cmd/edgesync/pages.go (claim), internal/i18n/locales/en/gui_machines.toml
  Tests: TestPage_PairedAndDiscoveredSections, TestPage_LoadingEmptyErrorStates, TestPage_RowActions, TestRenameDialog_EnterSaves, TestUnpairDialog_Confirm, TestAddAddressDialog_Validation, TestPage_KeyboardThroughRows, TestPage_HitTargets
  DoD: GUI gates green; `qa/ui-ux-audit.md` A, B, D self-checked; commit `feat(gui): machines page [FEAT-148]`

- [ ] [P3-G2-004] Pairing view model: initiator and responder state machines, countdown, attempts, code handling [FEAT-149] [tier:sonnet why:two-sided pairing state machine with TTL, attempt rules and secret handling] [owner:G2] [brief:briefs/streams/p3-g2.md#item-p3-g2-004]
  Needs: P3-G2-001
  Files: gui/peers/vm/pairing.go, gui/peers/vm/pairing_test.go
  Tests: TestInitiator_HappyPath, TestInitiator_WrongCodeDecrementsAttempts, TestInitiator_ThirdWrongCodeFails, TestInitiator_ExpiresAtZero (synctest), TestInitiator_CodeNormalisation, TestInitiator_SubmitNeedsSixDigits, TestInitiator_Cancel, TestResponder_CodeGrouped, TestResponder_PeerJoined, TestResponder_ExpiredOffersNewCode, TestPairing_FailureReasonsMapped, TestPairing_CodeClearedOnClose, TestPairing_CodeNeverLogged, TestPairing_EventMappingAllVariants
  DoD: coverage >= 85%; GUI gates green; commit `feat(gui): pairing view model [FEAT-149]`

- [ ] [P3-G2-005] Pairing dialogs: code entry and code display [FEAT-149, FEAT-160] [tier:haiku] [owner:G2] [brief:briefs/streams/p3-g2.md#item-p3-g2-005]
  Needs: P3-G2-003, P3-G2-004
  Files: gui/peers/pairingdialog.go, gui/peers/pairingdialog_test.go, internal/i18n/locales/en/gui_machines.toml
  Tests: TestInitiatorDialog_TypeAndPaste, TestInitiatorDialog_EnterSubmits, TestInitiatorDialog_AttemptsAndCountdown, TestResponderDialog_LargeGroupedCode, TestPairingDialogs_EscapeCancels, TestPairingDialogs_ErrorTexts, TestPairingDialogs_IncomingOpensResponder, TestPairingDialogs_HitTargets
  DoD: GUI gates green; `qa/ui-ux-audit.md` E (pairing) self-checked; commit `feat(gui): pairing dialogs [FEAT-149]`

- [ ] [P3-G2-006] Permissions and environment-check view model (shared by wizard, permissions page and shell banner) [FEAT-056, FEAT-077, FEAT-068, FEAT-150] [tier:haiku] [owner:G2] [brief:briefs/streams/p3-g2.md#item-p3-g2-006]
  Needs: P3-G2-001
  Files: gui/wizard/vm/checks.go, gui/wizard/vm/checks_test.go
  Tests: TestPermissions_RowsPerKind, TestPermissions_RequestOp, TestPermissions_LiveUpdate, TestPermissions_DegradedDetailShown, TestEnvChecks_FirewallProblemHint, TestEnvChecks_TrayGnomeHint, TestEnvChecks_UnsupportedHidden, TestChecks_NothingApplicable
  DoD: coverage >= 85%; GUI gates green; commit `feat(gui): permissions and environment checks model [FEAT-150]`

- [ ] [P3-G2-007] First-run wizard state machine [FEAT-150] [tier:sonnet why:wizard flow state machine with skip rules, auto-skip and resume] [owner:G2] [brief:briefs/streams/p3-g2.md#item-p3-g2-007]
  Needs: P3-G2-004, P3-G2-006
  Files: gui/wizard/vm/wizard.go, gui/wizard/vm/wizard_test.go
  Tests: TestWizard_StepsServer, TestWizard_StepsClient, TestWizard_RoleRequired, TestWizard_SetRoleOnNext, TestWizard_SkipRules, TestWizard_PermissionsAutoSkipped, TestWizard_ChecksAutoSkipped, TestWizard_SkipPermissionsConfirms, TestWizard_PersistsStep, TestWizard_ResumesAtPersistedStep, TestWizard_BackRules, TestWizard_FinishSetsCompleted, TestWizard_StepCounter
  DoD: coverage >= 85%; GUI gates green; commit `feat(gui): first-run wizard model [FEAT-150]`

- [ ] [P3-G2-008] First-run wizard view (six steps) and registration [FEAT-150, FEAT-160] [tier:haiku] [owner:G2] [brief:briefs/streams/p3-g2.md#item-p3-g2-008]
  Needs: P3-G2-005, P3-G2-007
  Files: gui/wizard/page.go, gui/wizard/steps.go, gui/wizard/page_test.go, cmd/edgesync/pages.go (claim), internal/i18n/locales/en/gui_machines.toml
  Tests: TestWizardPage_EachStepRenders, TestWizardPage_EnterAdvances, TestWizardPage_BackSkipButtons, TestWizardPage_ServerPairingStepListsDiscovered, TestWizardPage_ClientPairingStepShowsCode, TestWizardPage_PermissionsRequest, TestWizardPage_ChecksRecheck, TestWizardPage_FinishLeavesGate, TestWizardPage_HitTargets
  DoD: GUI gates green; `qa/ui-ux-audit.md` E (wizard) self-checked; commit `feat(gui): first-run wizard [FEAT-150]`

- [ ] [P3-G2-009] Permissions and checks page (hidden route, reached from the shell banner); registration [FEAT-056, FEAT-077, FEAT-068, FEAT-150] [tier:haiku] [owner:G2] [brief:briefs/streams/p3-g2.md#item-p3-g2-009]
  Needs: P3-G2-008
  Files: gui/wizard/permissionspage.go, gui/wizard/permissionspage_test.go, cmd/edgesync/pages.go (claim), internal/i18n/locales/en/gui_machines.toml
  Tests: TestPermissionsPage_RowsAndRequest, TestPermissionsPage_EnvChecksAndRecheck, TestPermissionsPage_AllGrantedState, TestPermissionsPage_HitTargets
  DoD: GUI gates green; `qa/ui-ux-audit.md` D (error messages) self-checked; commit `feat(gui): permissions page [FEAT-150]`

- [ ] [P3-G2-010] Manual script for machines, pairing, wizard and permissions [FEAT-148, FEAT-149, FEAT-150, FEAT-179, FEAT-180, FEAT-182, FEAT-183] [tier:haiku] [owner:G2] [brief:briefs/streams/p3-g2.md#item-p3-g2-010]
  Needs: P3-G2-009, P3-G2-011, P3-G2-012, P3-G2-013
  Files: qa/manual/p3-g2-machines-pairing.md
  Tests: script follows `qa/manual/README.md`; covers first run on both Windows PCs and the GNOME box, pairing both directions, wrong code 3 times, expiry, cancel, rename, unpair, add/remove address, wake, permissions on GNOME (portal consent), tray hint on GNOME without the AppIndicator extension, firewall check on Windows, the sign-in screen switch (UAC prompt, on and off, unpair of an allowed server) and Send Ctrl+Alt+Del from the row menu (D-112, D-113), the macOS re-grant note and lock-unsupported row as far as the screenshots show them (D-117, D-115), the Wayland home-hotkey row on the GNOME box (D-114)
  DoD: commit `docs(qa): machines and pairing manual script [FEAT-148]`

- [ ] [P3-G2-M01] Run the machines/pairing script on both Windows PCs and the GNOME box and record results [FEAT-148, FEAT-149, FEAT-150] [tier:haiku] [owner:USER] [brief:briefs/streams/p3-g2.md#item-p3-g2-m01]
  Needs: P3-G2-010
  Files: qa/manual/results/p3-g2-machines-pairing.md
  Tests: every step has a recorded observation incl. GNOME and xdg-desktop-portal versions
  DoD: results committed by the parent `docs(qa): machines and pairing results [FEAT-148]`

- [ ] [P3-G2-R01] Code review of the G2 stream [FEAT-148] [tier:sonnet why:per-stream code-review gate] [owner:G2] [brief:briefs/streams/p3-g2.md#item-p3-g2-r01]
  Needs: P3-G2-001 .. P3-G2-010 all [x], P3-G2-011 .. P3-G2-013 all [x]
  Files: none (read-only `code-reviewer`)
  Tests: every G2 diff against its item and brief
  DoD: VERDICT pass or every finding fixed via appended items; no commit by the reviewer

- [ ] [P3-G2-R02] Security review of the pairing UI (code never logged, cleared, never copied) [FEAT-149] [tier:sonnet why:per-stream security-review gate] [owner:G2] [brief:briefs/streams/p3-g2.md#item-p3-g2-r02]
  Needs: P3-G2-R01
  Files: none (read-only `code-reviewer` with `qa/security-review.md`)
  Tests: sections 1 and 7 of `qa/security-review.md` as they apply to the GUI plus the brief's extra points
  DoD: VERDICT pass or every finding fixed via appended items; no commit by the reviewer

- [ ] [P3-G2-R03] UI/UX audit of machines, pairing, wizard and permissions [FEAT-160] [tier:sonnet why:per-stream UI/UX audit gate] [owner:G2] [brief:briefs/streams/p3-g2.md#item-p3-g2-r03]
  Needs: P3-G2-R02, P3-G2-M01
  Files: none (read-only `code-reviewer` with `qa/ui-ux-audit.md` and the screenshots of the G2 routes)
  Tests: sections A-F of `qa/ui-ux-audit.md` (F: the GNOME tray hint)
  DoD: VERDICT pass or every finding fixed via appended items; no commit by the reviewer

- [ ] [P3-G2-R04] Opus consolidation review of the G2 stream [FEAT-148, FEAT-149, FEAT-150] [tier:opus] [owner:PARENT] [brief:briefs/streams/p3-g2.md#item-p3-g2-r04]
  Needs: P3-G2-R03
  Files: briefs/plan-phase-3.md (append note)
  Tests: D-14, D-15, D-18, D-57 (pairing TTL/attempts) honoured; wizard steps match D-18; no pairing logic duplicated from internal/pairing
  DoD: note `<!-- NOTE: <date> - stream G2 review chain green -->`; commit `chore(plan): G2 review chain green [FEAT-148]`

- [ ] [P3-G2-011] Machines view model: sign-in screen switch state and intents, Send Ctrl+Alt+Del row action (D-112, D-113) [FEAT-179, FEAT-180, FEAT-148] [tier:haiku] [owner:G2] [brief:briefs/streams/p3-g2.md#item-p3-g2-011]
  Needs: P3-G2-002, P3-G5-020
  Files: gui/peers/vm/prelogin.go, gui/peers/vm/prelogin_test.go
  Tests: TestPrelogin_SwitchOnlyOnWindowsWithService, TestPrelogin_StateFromGetPreloginAccess, TestPrelogin_TurnOnBuildsElevatedAddIntent, TestPrelogin_TurnOffBuildsElevatedRemoveIntent, TestPrelogin_ElevationDeclinedKeepsState, TestPrelogin_ElevationFailureShowsExitCodeMessage, TestPrelogin_RefreshAfterChange, TestUnpair_AllowedPeerRunsRemovalFirstAndCancelsOnFailure, TestSendCAD_ActionOnlyForConnectedWindowsClientsWithCapability, TestSendCAD_ActionCallsIPCRequestAndShowsError, TestSendCAD_HiddenInClientRole
  DoD: coverage >= 85%; GUI gates green; commit `feat(gui): sign-in screen switch and Send Ctrl+Alt+Del model [FEAT-179]`

- [ ] [P3-G2-012] Machines page: sign-in screen switch, elevation launcher and Send Ctrl+Alt+Del row action (D-112, D-113) [FEAT-179, FEAT-180, FEAT-160] [tier:haiku] [owner:G2] [brief:briefs/streams/p3-g2.md#item-p3-g2-012]
  Needs: P3-G2-003, P3-G2-011
  Files: gui/peers/prelogin.go, gui/peers/prelogin_test.go, gui/peers/elevate_windows.go, gui/peers/elevate_other.go, gui/peers/elevate_test.go, gui/peers/rows.go, internal/i18n/locales/en/gui_machines.toml
  Tests: TestRow_PreloginSwitchShownForEligiblePeers, TestRow_PreloginSwitchTogglesThroughElevator, TestRow_PreloginHelpTextStatesScopeAndAdminNeed, TestRow_SendCADActionShownAndInvokes, TestUnpairDialog_MentionsSignInScreenRemoval, TestElevate_BuildsRunasShellExecuteRequest (pure construction, host), TestElevate_RejectsServiceExeOutsideProgramFiles, TestElevate_UserCancelMapsToExitCode3, TestElevate_OtherOSReturnsUnsupported, TestPrelogin_HitTargets
  DoD: GUI gates green; `qa/ui-ux-audit.md` A, B, D self-checked; commit `feat(gui): sign-in screen switch and Send Ctrl+Alt+Del on the Machines page [FEAT-179]`

- [ ] [P3-G2-013] Wizard and permissions page: macOS re-grant note, lock-unsupported and Wayland home-hotkey check rows (D-117, D-115, D-114) [FEAT-183, FEAT-182, FEAT-181, FEAT-150] [tier:haiku] [owner:G2] [brief:briefs/streams/p3-g2.md#item-p3-g2-013]
  Needs: P3-G2-008, P3-G2-009, P3-G5-020
  Files: gui/wizard/vm/notes.go, gui/wizard/vm/notes_test.go, gui/wizard/steps.go, gui/wizard/permissionspage.go, gui/wizard/notes_page_test.go, internal/i18n/locales/en/gui_machines.toml
  Tests: TestNotes_RegrantShownOnMacOSOnly, TestNotes_LockUnsupportedRowFromEnvCheck, TestNotes_HomeHotkeysRowFromEnvCheck, TestNotes_UnavailableCarriesReason, TestNotes_NothingApplicableHidden, TestWizardPage_PermissionsStepShowsRegrantNote, TestPermissionsPage_ShowsLockAndHotkeyRows, TestPermissionsPage_NoteHitTargets
  DoD: GUI gates green; `qa/ui-ux-audit.md` D (error messages) self-checked; commit `feat(gui): macOS re-grant note and platform check rows [FEAT-183]`

### G3 - Settings

- [ ] [P3-G3-001] Settings field model: optimistic edits, validation, agent confirmation, revert on error, config patches [FEAT-151] [tier:haiku] [owner:G3] [brief:briefs/streams/p3-g3.md#item-p3-g3-001]
  Needs: P3-F-003, P3-F-018, P3-F-019
  Files: gui/settings/vm/field.go, gui/settings/vm/patch.go, gui/settings/vm/field_test.go, gui/settings/vm/patch_test.go
  Tests: TestField_OptimisticThenConfirmed, TestField_ErrorRevertsWithMessage, TestField_InvalidNotSent, TestField_ConfigChangedUpdatesIdle, TestField_PendingNotOverwritten, TestPatch_SingleFieldPatch, TestPatch_NestedTables
  DoD: coverage >= 85%; GUI gates green; commit `feat(gui): settings field model [FEAT-151]`

- [ ] [P3-G3-002] General settings model: role, autostart, receive folder, updates, sync, Wake-on-LAN, clipboard, log level, run setup again [FEAT-151, FEAT-001, FEAT-090] [tier:haiku] [owner:G3] [brief:briefs/streams/p3-g3.md#item-p3-g3-002]
  Needs: P3-G3-001
  Files: gui/settings/vm/general.go, gui/settings/vm/general_test.go
  Tests: TestGeneral_RoleChangeNeedsConfirm, TestGeneral_RoleUsesSetRole, TestGeneral_AutostartUsesSetAutostart, TestGeneral_ReceiveDirMustBeAbsolute, TestGeneral_BrowseUsesFolderPicker, TestGeneral_BrowseCancelNoChange, TestGeneral_UpdateToggle, TestGeneral_SyncToggles, TestGeneral_WolAutoToggle, TestGeneral_ClipboardFormatsDisabledWhenMasterOff, TestGeneral_LogLevel, TestGeneral_RunSetupAgain
  DoD: coverage >= 85%; GUI gates green; commit `feat(gui): general settings model [FEAT-151]`

- [ ] [P3-G3-003] Edge-switching guards model [FEAT-152, FEAT-011, FEAT-012, FEAT-013, FEAT-014] [tier:haiku] [owner:G3] [brief:briefs/streams/p3-g3.md#item-p3-g3-003]
  Needs: P3-G3-001
  Files: gui/settings/vm/guards.go, gui/settings/vm/guards_test.go
  Tests: TestGuards_DefaultsMatchD57, TestGuards_RangesValidated, TestGuards_ModifierLabelsPerOS, TestGuards_ResetDefaults, TestGuards_ClientRoleNote
  DoD: coverage >= 85%; GUI gates green; commit `feat(gui): edge guard settings model [FEAT-152]`

- [ ] [P3-G3-004] Hotkey editor model: capture, conflicts, reserved warnings, clear, reset [FEAT-153, FEAT-019] [tier:haiku] [owner:G3] [brief:briefs/streams/p3-g3.md#item-p3-g3-004]
  Needs: P3-G3-001
  Files: gui/settings/vm/hotkeys.go, gui/settings/vm/hotkeys_test.go
  Tests: TestHotkeys_RowsWithJumpPerLayoutOrder, TestHotkeys_Machines13PlusUnbound, TestCapture_SetsAndSaves, TestCapture_EscapeCancels, TestCapture_TimesOutAfter15s (synctest), TestCapture_DuplicateRejected, TestCapture_ReservedWarnsButSaves, TestClear_NotAllowedForPanic, TestReset_DefaultsMatchD58, TestHotkeys_ClientRoleNote
  DoD: binding parse/format/conflict/reserved logic comes from the P1-C3 hotkey model (no second implementation); coverage >= 85%; GUI gates green; commit `feat(gui): hotkey editor model [FEAT-153]`

- [ ] [P3-G3-005] Per-machine modifier remap model [FEAT-154, FEAT-025] [tier:haiku] [owner:G3] [brief:briefs/streams/p3-g3.md#item-p3-g3-005]
  Needs: P3-G3-001
  Files: gui/settings/vm/remap.go, gui/settings/vm/remap_test.go
  Tests: TestRemap_NoneIsIdentity, TestRemap_MacPreset, TestRemap_CustomRow, TestRemap_DefaultShownForMacPeer, TestRemap_SavePatch, TestRemap_ResetToDefault, TestRemap_ClientRoleNote
  DoD: uses `keymap.Usage` and `keymap.Remap` (internal/keymap); coverage >= 85%; GUI gates green; commit `feat(gui): modifier remap model [FEAT-154]`

- [ ] [P3-G3-006] Network settings model: port, manual addresses [FEAT-155, FEAT-130] [tier:haiku] [owner:G3] [brief:briefs/streams/p3-g3.md#item-p3-g3-006]
  Needs: P3-G3-001
  Files: gui/settings/vm/network.go, gui/settings/vm/network_test.go
  Tests: TestNetwork_PortRange, TestNetwork_PortChangeNeedsConfirm, TestNetwork_ManualListFromDiscovered, TestNetwork_AddValidatedAndSent, TestNetwork_RemoveSent
  DoD: coverage >= 85%; GUI gates green; commit `feat(gui): network settings model [FEAT-155]`

- [ ] [P3-G3-007] Settings page frame with tabs and the General tab; registration [FEAT-151, FEAT-160] [tier:haiku] [owner:G3] [brief:briefs/streams/p3-g3.md#item-p3-g3-007]
  Needs: P3-G3-002
  Files: gui/settings/page.go, gui/settings/general.go, gui/settings/page_test.go, gui/settings/general_test.go, cmd/edgesync/pages.go (claim), internal/i18n/locales/en/gui_settings.toml
  Tests: TestPage_TabsAndTabIntent, TestPage_ShortcutTabCycles, TestPage_LoadingErrorStates, TestGeneral_AllControlsRender, TestGeneral_RoleConfirmDialog, TestGeneral_FieldErrorAndPending, TestGeneral_ClipboardFormatsDisabledLook, TestGeneral_ClipboardPrivacyNoteShown (password-manager content is shared like any other; the master toggle turns sharing off, D-121), TestGeneral_HitTargets
  DoD: GUI gates green; `qa/ui-ux-audit.md` A, B, C, D self-checked; commit `feat(gui): settings page and general tab [FEAT-151]`

- [ ] [P3-G3-008] Edge switching tab [FEAT-152, FEAT-160] [tier:haiku] [owner:G3] [brief:briefs/streams/p3-g3.md#item-p3-g3-008]
  Needs: P3-G3-003, P3-G3-007
  Files: gui/settings/guards.go, gui/settings/guards_test.go, internal/i18n/locales/en/gui_settings.toml
  Tests: TestGuardsTab_FourGuardsWithParameters, TestGuardsTab_ParameterDisabledWhenOff, TestGuardsTab_ResetConfirm, TestGuardsTab_ClientNote, TestGuardsTab_HitTargets
  DoD: GUI gates green; commit `feat(gui): edge switching settings tab [FEAT-152]`

- [ ] [P3-G3-009] Hotkeys tab with capture dialog [FEAT-153, FEAT-160] [tier:haiku] [owner:G3] [brief:briefs/streams/p3-g3.md#item-p3-g3-009]
  Needs: P3-G3-004, P3-G3-007
  Files: gui/settings/hotkeys.go, gui/settings/hotkeys_test.go, internal/i18n/locales/en/gui_settings.toml
  Tests: TestHotkeysTab_RowsAndBindings, TestHotkeysTab_ChangeOpensCaptureDialog, TestCaptureDialog_ShowsComboAndConflict, TestCaptureDialog_EscapeCancels, TestHotkeysTab_ClearAndReset, TestHotkeysTab_HitTargets
  DoD: GUI gates green; `qa/ui-ux-audit.md` B (hotkey capture) self-checked; commit `feat(gui): hotkeys settings tab [FEAT-153]`

- [ ] [P3-G3-010] Key remap tab [FEAT-154, FEAT-160] [tier:haiku] [owner:G3] [brief:briefs/streams/p3-g3.md#item-p3-g3-010]
  Needs: P3-G3-005, P3-G3-007
  Files: gui/settings/remap.go, gui/settings/remap_test.go, internal/i18n/locales/en/gui_settings.toml
  Tests: TestRemapTab_MachineSelector, TestRemapTab_PresetSwitch, TestRemapTab_CustomTableEdit, TestRemapTab_ClientNote, TestRemapTab_EmptyNoPeers, TestRemapTab_HitTargets
  DoD: GUI gates green; commit `feat(gui): key remap settings tab [FEAT-154]`

- [ ] [P3-G3-011] Network tab [FEAT-155, FEAT-130, FEAT-160] [tier:haiku] [owner:G3] [brief:briefs/streams/p3-g3.md#item-p3-g3-011]
  Needs: P3-G3-006, P3-G3-007
  Files: gui/settings/network.go, gui/settings/network_test.go, internal/i18n/locales/en/gui_settings.toml
  Tests: TestNetworkTab_PortFieldAndConfirm, TestNetworkTab_AddressListAddRemove, TestNetworkTab_ValidationMessages, TestNetworkTab_EmptyList, TestNetworkTab_HitTargets
  DoD: GUI gates green; commit `feat(gui): network settings tab [FEAT-155]`

- [ ] [P3-G3-012] Manual script for settings [FEAT-151, FEAT-152, FEAT-153, FEAT-154, FEAT-155, FEAT-180] [tier:haiku] [owner:G3] [brief:briefs/streams/p3-g3.md#item-p3-g3-012]
  Needs: P3-G3-008, P3-G3-009, P3-G3-010, P3-G3-011, P3-G3-013
  Files: qa/manual/p3-g3-settings.md
  Tests: script follows `qa/manual/README.md`; every setting changed through the GUI and its effect observed (role switch, autostart after sign-out/in, receive folder, each guard, each hotkey incl. capture and conflict and the Send Ctrl+Alt+Del row, the clipboard privacy note, remap on a client, port change, manual address), keyboard-only operation, DPI/theme checks
  DoD: commit `docs(qa): settings manual script [FEAT-151]`

- [ ] [P3-G3-M01] Run the settings script on both Windows PCs and record results [FEAT-151, FEAT-152, FEAT-153, FEAT-154, FEAT-155] [tier:haiku] [owner:USER] [brief:briefs/streams/p3-g3.md#item-p3-g3-m01]
  Needs: P3-G3-012
  Files: qa/manual/results/p3-g3-settings.md
  Tests: every step has a recorded observation
  DoD: results committed by the parent `docs(qa): settings results [FEAT-151]`

- [ ] [P3-G3-R01] Code review of the G3 stream [FEAT-151] [tier:sonnet why:per-stream code-review gate] [owner:G3] [brief:briefs/streams/p3-g3.md#item-p3-g3-r01]
  Needs: P3-G3-001 .. P3-G3-012 all [x], P3-G3-013 all [x]
  Files: none (read-only `code-reviewer`)
  Tests: every G3 diff against its item and brief
  DoD: VERDICT pass or every finding fixed via appended items; no commit by the reviewer

- [ ] [P3-G3-R02] UI/UX audit of the settings page [FEAT-160] [tier:sonnet why:per-stream UI/UX audit gate] [owner:G3] [brief:briefs/streams/p3-g3.md#item-p3-g3-r02]
  Needs: P3-G3-R01, P3-G3-M01
  Files: none (read-only `code-reviewer` with `qa/ui-ux-audit.md` and the screenshots of the settings route)
  Tests: sections A-D of `qa/ui-ux-audit.md`
  DoD: VERDICT pass or every finding fixed via appended items; no commit by the reviewer

- [ ] [P3-G3-R03] Opus consolidation review of the G3 stream [FEAT-151] [tier:opus] [owner:PARENT] [brief:briefs/streams/p3-g3.md#item-p3-g3-r03]
  Needs: P3-G3-R02
  Files: briefs/plan-phase-3.md (append note)
  Tests: D-03, D-05, D-06, D-08, D-17, D-21, D-31, D-43, D-57, D-58, D-61, D-70, D-71 reflected in the settings exactly
  DoD: note `<!-- NOTE: <date> - stream G3 review chain green -->`; commit `chore(plan): G3 review chain green [FEAT-151]`

- [ ] [P3-G3-013] Hotkeys tab: Send Ctrl+Alt+Del row and Wayland home-hotkey hints (D-113, D-114) [FEAT-180, FEAT-181, FEAT-153, FEAT-160] [tier:haiku] [owner:G3] [brief:briefs/streams/p3-g3.md#item-p3-g3-013]
  Needs: P3-G3-009, P3-G5-020
  Files: gui/settings/vm/hotkeys_extra.go, gui/settings/vm/hotkeys_extra_test.go, gui/settings/hotkeys.go, gui/settings/hotkeys_extra_test.go, internal/i18n/locales/en/gui_settings.toml
  Tests: TestHotkeysExtra_SendCADRowDefaultAndRebind, TestHotkeysExtra_SendCADClearAllowedOnlyPanicRequired, TestHotkeysExtra_SendCADConflictDetected, TestHotkeysExtra_WaylandServerAnnotatesJumpAndCycle, TestHotkeysExtra_HomeShortcutStatusShowsBoundTriggers, TestHotkeysExtra_HomeShortcutUnavailableShowsReason, TestHotkeysExtra_NoHintOnOtherPlatformsOrClientRole
  DoD: GUI gates green; `qa/ui-ux-audit.md` B (hotkey capture) self-checked; commit `feat(gui): send-CAD hotkey row and Wayland hints [FEAT-180]`

### G4 - Transfers, send files, logs, diagnostics, about

- [ ] [P3-G4-001] Transfers view model: rows, speed, ETA, filters, clear finished, actions [FEAT-156, FEAT-093] [tier:haiku] [owner:G4] [brief:briefs/streams/p3-g4.md#item-p3-g4-001]
  Needs: P3-F-003, P3-F-018, P3-F-019
  Files: gui/transfers/vm/transfers.go, gui/transfers/vm/transfers_test.go
  Tests: TestRows_FromListAndEvents, TestRows_TitleSingleAndMore, TestSpeed_MovingAverage3s (synctest), TestETA_UnknownWhenStalled, TestFilters_AllActiveFinished, TestClearFinished_HidesOnlyFinished, TestActions_CancelNeedsConfirm, TestActions_RetryOnlyFailedOrCancelled, TestActions_OpenFolderOnlyReceivedCompleted
  DoD: coverage >= 85%; GUI gates green; commit `feat(gui): transfers view model [FEAT-156]`

- [ ] [P3-G4-002] Transfers page view; registration [FEAT-156, FEAT-160] [tier:haiku] [owner:G4] [brief:briefs/streams/p3-g4.md#item-p3-g4-002]
  Needs: P3-G4-001
  Files: gui/transfers/page.go, gui/transfers/rows.go, gui/transfers/page_test.go, cmd/edgesync/pages.go (claim), internal/i18n/locales/en/gui_transfers.toml
  Tests: TestPage_RowsWithProgressSpeedETA, TestPage_FilterTabs, TestPage_CancelConfirm, TestPage_RetryAndOpenFolder, TestPage_ClearFinished, TestPage_LoadingEmptyErrorStates, TestPage_SendIntentOpensDialogSlot, TestPage_HitTargets
  DoD: GUI gates green; `qa/ui-ux-audit.md` A, D, E (transfers) self-checked; commit `feat(gui): transfers page [FEAT-156]`

- [ ] [P3-G4-003] Send-files view model: targets, picked items, validation, send [FEAT-098] [tier:haiku] [owner:G4] [brief:briefs/streams/p3-g4.md#item-p3-g4-003]
  Needs: P3-G4-001
  Files: gui/transfers/vm/send.go, gui/transfers/vm/send_test.go
  Tests: TestSend_TargetsConnectedPairedOnly, TestSend_PreselectFromIntent, TestSend_AddFilesAndFolderDeduplicated, TestSend_PickerCancelledNoError, TestSend_ItemSizesAndFolderLabel, TestSend_StatErrorShownPerItem, TestSend_EnablementRules, TestSend_SuccessNavigatesToTransfers, TestSend_ErrorInline
  DoD: coverage >= 85%; GUI gates green; commit `feat(gui): send files view model [FEAT-098]`

- [ ] [P3-G4-004] Send-files dialog [FEAT-098, FEAT-160] [tier:haiku] [owner:G4] [brief:briefs/streams/p3-g4.md#item-p3-g4-004]
  Needs: P3-G4-002, P3-G4-003
  Files: gui/transfers/senddialog.go, gui/transfers/senddialog_test.go, internal/i18n/locales/en/gui_transfers.toml
  Tests: TestSendDialog_TargetSelect, TestSendDialog_AddFilesAddFolder, TestSendDialog_RemoveItem, TestSendDialog_SendDisabledUntilReady, TestSendDialog_EscapeCloses, TestSendDialog_HitTargets
  DoD: GUI gates green; commit `feat(gui): send files dialog [FEAT-098]`

- [ ] [P3-G4-005] Logs view model: level filter, live tail, selection, copy formatting [FEAT-157] [tier:haiku] [owner:G4] [brief:briefs/streams/p3-g4.md#item-p3-g4-005]
  Needs: P3-F-003, P3-F-018, P3-F-019
  Files: gui/logs/vm/logs.go, gui/logs/vm/logs_test.go
  Tests: TestLogs_FilterMinLevel, TestLogs_TailFollowsNewest, TestLogs_ScrollUpPausesTail, TestLogs_SelectionClickShiftCtrl, TestLogs_SelectAllVisible, TestLogs_CopyFormat, TestLogs_Bounded5000
  DoD: coverage >= 85%; GUI gates green; commit `feat(gui): logs view model [FEAT-157]`

- [ ] [P3-G4-006] Logs page view with copy to clipboard; registration [FEAT-157, FEAT-160] [tier:haiku] [owner:G4] [brief:briefs/streams/p3-g4.md#item-p3-g4-006]
  Needs: P3-G4-002, P3-G4-005
  Files: gui/logs/page.go, gui/logs/page_test.go, cmd/edgesync/pages.go (claim), internal/i18n/locales/en/gui_transfers.toml
  Tests: TestPage_LevelFilterControl, TestPage_TailToggle, TestPage_CopyWritesClipboard, TestPage_ShortcutCopyAndSelectAll, TestPage_5000RowsListRenders, TestPage_LoadingEmptyErrorStates, TestPage_HitTargets
  DoD: GUI gates green; `qa/ui-ux-audit.md` E (logs) self-checked; commit `feat(gui): logs page [FEAT-157]`

- [ ] [P3-G4-007] Diagnostics export (model and view on the logs page) [FEAT-158, FEAT-139] [tier:haiku] [owner:G4] [brief:briefs/streams/p3-g4.md#item-p3-g4-007]
  Needs: P3-G4-006
  Files: gui/logs/vm/diag.go, gui/logs/vm/diag_test.go, gui/logs/diag.go, gui/logs/diag_test.go, internal/i18n/locales/en/gui_transfers.toml
  Tests: TestDiag_FileNameFormat, TestDiag_PickerCancelledNoOp, TestDiag_SuccessShowsPathAndCopy, TestDiag_ErrorShown, TestDiag_PendingAfter300ms, TestDiagView_ExplanationAndButtons, TestDiagView_HitTargets
  DoD: coverage of gui/logs/vm >= 85%; GUI gates green; commit `feat(gui): diagnostics export [FEAT-158]`

- [ ] [P3-G4-008] About page with versions and the update notice; registration [FEAT-159] [tier:haiku] [owner:G4] [brief:briefs/streams/p3-g4.md#item-p3-g4-008]
  Needs: P3-G4-006
  Files: gui/about/vm/about.go, gui/about/vm/about_test.go, gui/about/page.go, gui/about/page_test.go, cmd/edgesync/pages.go (claim), internal/i18n/locales/en/gui_transfers.toml
  Tests: TestAbout_VersionsAndMismatch, TestAbout_UpdateAvailable, TestAbout_CheckNowResults, TestAbout_ChecksDisabledLinksSettings, TestAbout_ReleaseURLValidation, TestAboutPage_Renders, TestAboutPage_HitTargets
  DoD: coverage of gui/about/vm >= 85%; GUI gates green; commit `feat(gui): about page and update notice [FEAT-159]`

- [ ] [P3-G4-009] Manual script for transfers, send files, logs, diagnostics and about [FEAT-156, FEAT-157, FEAT-158, FEAT-159, FEAT-098] [tier:haiku] [owner:G4] [brief:briefs/streams/p3-g4.md#item-p3-g4-009]
  Needs: P3-G4-004, P3-G4-007, P3-G4-008
  Files: qa/manual/p3-g4-transfers-logs-about.md
  Tests: script follows `qa/manual/README.md`; covers sending files and a folder between the Windows PCs, progress/speed/ETA, cancel, retry with resume, open folder, clash rename, log filter/tail/copy, diagnostics export and inspection of the zip (no clipboard content, no keys), about versions, update check
  DoD: commit `docs(qa): transfers and logs manual script [FEAT-156]`

- [ ] [P3-G4-M01] Run the transfers/logs/about script on both Windows PCs and record results [FEAT-156, FEAT-157, FEAT-158, FEAT-159] [tier:haiku] [owner:USER] [brief:briefs/streams/p3-g4.md#item-p3-g4-m01]
  Needs: P3-G4-009
  Files: qa/manual/results/p3-g4-transfers-logs-about.md
  Tests: every step has a recorded observation
  DoD: results committed by the parent `docs(qa): transfers and logs results [FEAT-156]`

- [ ] [P3-G4-R01] Code review of the G4 stream [FEAT-156] [tier:sonnet why:per-stream code-review gate] [owner:G4] [brief:briefs/streams/p3-g4.md#item-p3-g4-r01]
  Needs: P3-G4-001 .. P3-G4-009 all [x]
  Files: none (read-only `code-reviewer`)
  Tests: every G4 diff against its item and brief
  DoD: VERDICT pass or every finding fixed via appended items; no commit by the reviewer

- [ ] [P3-G4-R02] Security review of diagnostics, logs copy, send paths and URL opening [FEAT-158, FEAT-159] [tier:sonnet why:per-stream security-review gate] [owner:G4] [brief:briefs/streams/p3-g4.md#item-p3-g4-r02]
  Needs: P3-G4-R01
  Files: none (read-only `code-reviewer` with `qa/security-review.md`)
  Tests: sections 7 and 8 of `qa/security-review.md` plus the brief's extra points
  DoD: VERDICT pass or every finding fixed via appended items; no commit by the reviewer

- [ ] [P3-G4-R03] UI/UX audit of transfers, send files, logs, diagnostics and about [FEAT-160] [tier:sonnet why:per-stream UI/UX audit gate] [owner:G4] [brief:briefs/streams/p3-g4.md#item-p3-g4-r03]
  Needs: P3-G4-R02, P3-G4-M01
  Files: none (read-only `code-reviewer` with `qa/ui-ux-audit.md` and the screenshots of the G4 routes)
  Tests: sections A-E of `qa/ui-ux-audit.md`
  DoD: VERDICT pass or every finding fixed via appended items; no commit by the reviewer

- [ ] [P3-G4-R04] Opus consolidation review of the G4 stream [FEAT-156, FEAT-157, FEAT-158, FEAT-159] [tier:opus] [owner:PARENT] [brief:briefs/streams/p3-g4.md#item-p3-g4-r04]
  Needs: P3-G4-R03
  Files: briefs/plan-phase-3.md (append note)
  Tests: D-12, D-17, D-18, D-31, D-48, D-72 honoured
  DoD: note `<!-- NOTE: <date> - stream G4 review chain green -->`; commit `chore(plan): G4 review chain green [FEAT-156]`

### G5 - Agent end-to-end integration

- [ ] [P3-G5-001] End-to-end harness: in-process agents on fake backends over loopback, IPC clients, pairing helper, event waiting, restart [FEAT-162] [tier:sonnet why:multi-agent in-process harness design with isolation seams and deterministic teardown] [owner:G5] [brief:briefs/streams/p3-g5.md#item-p3-g5-001]
  Needs: P3-F-006, P2-W1-029, P2-W2-035
  Files: test/e2e/harness/harness.go, test/e2e/harness/pair.go, test/e2e/harness/wait.go, test/e2e/harness/harness_test.go, internal/agent/agent.go (isolation seams only)
  Tests: TestHarness_StartStopTwoAgents, TestHarness_IsolatedDirsAndEndpoints, TestHarness_ListensOnLoopbackOnly, TestHarness_PairViaIPC, TestHarness_WaitEventTimeout, TestHarness_RestartKeepsIdentity, TestHarness_NoGoroutineLeak
  DoD: `-race` green; coverage of test/e2e/harness >= 85%; never binds a non-loopback address; per-item gates green; commit `test(e2e): multi-agent harness [FEAT-162]`

- [ ] [P3-G5-002] Real backend selection and reported startup failures in edgesync-agent [FEAT-161, FEAT-142] [tier:haiku] [owner:G5] [brief:briefs/streams/p3-g5.md#item-p3-g5-002]
  Needs: P3-G5-001
  Files: cmd/edgesync-agent/main.go, internal/agent/backend.go, internal/agent/backend_test.go
  Tests: TestBackend_OpenedWithConfigInjectionMode, TestBackend_AgentNeverOpensInSecureHelperMode, TestBackend_OpenFailureReportedNotFatal, TestBackend_OpenFailureVisibleInStatusAndTray, TestBackend_RetriesOpenEvery30s (synctest), TestBackend_ClosedOnShutdown
  DoD: coverage of internal/agent >= 85%; cross-compile gates green; commit `feat(agent): real backend selection [FEAT-161]`

- [ ] [P3-G5-003] Backend supervision: capture loss, permission errors, injector errors, display changes [FEAT-161, FEAT-142] [tier:sonnet why:supervision of backend goroutines with restart and permission-state transitions] [owner:G5] [brief:briefs/streams/p3-g5.md#item-p3-g5-003]
  Needs: P3-G5-002
  Files: internal/agent/supervise.go, internal/agent/supervise_test.go
  Tests: TestSupervise_CaptureLostRestartsWithBackoff (synctest), TestSupervise_PermissionErrorWaitsForGrant, TestSupervise_InjectorErrorReturnsHome, TestSupervise_DisplayChangeSendsDesktopChanged, TestSupervise_StatsExposedInStatus, TestSupervise_CloseStopsAll
  DoD: `-race` green; coverage >= 85%; commit `feat(agent): backend supervision [FEAT-161]`

- [ ] [P3-G5-004] Tray integration on the real tray: model updates, actions, GUI launch or focus [FEAT-144, FEAT-161] [tier:haiku] [owner:G5] [brief:briefs/streams/p3-g5.md#item-p3-g5-004]
  Needs: P3-G5-002
  Files: internal/agent/tray.go, internal/agent/guilaunch.go, internal/agent/guilaunch_windows.go, internal/agent/guilaunch_unix.go, internal/agent/tray_test.go, internal/agent/guilaunch_test.go, internal/i18n/locales/en.toml (claim)
  Tests: TestTray_ModelFollowsStatus, TestTray_ActionOpenLaunchesGUIWhenNoneAttached, TestTray_ActionOpenFocusesAttachedGUI, TestTray_ActionLockToggles, TestTray_ActionJumpAndHome, TestTray_ActionWake, TestTray_ActionUpdateOpensAbout, TestTray_ActionQuitReleasesAndStops, TestTray_UnavailableTrayNotFatal, TestGUILaunch_SiblingPathOnly
  DoD: coverage >= 85%; commit `feat(agent): tray integration [FEAT-144]`

- [ ] [P3-G5-005] Overlay, theme, autostart and permissions wiring [FEAT-161] [tier:haiku] [owner:G5] [brief:briefs/streams/p3-g5.md#item-p3-g5-005]
  Needs: P3-G5-002
  Files: internal/agent/uiwiring.go, internal/agent/uiwiring_test.go
  Tests: TestOverlay_ShownOnEnterWithMachineName, TestOverlay_DurationsFromD57, TestTheme_ChangedEventForwarded, TestAutostart_SetAutostartUsesBackend, TestPermissions_PolledWhileGUIAttached (synctest), TestPermissions_ChangedEventOnlyOnChange
  DoD: coverage >= 85%; commit `feat(agent): overlay, theme, autostart and permission wiring [FEAT-161]`

- [ ] [P3-G5-006] Wake-on-LAN integration: tray/IPC wake and auto-wake on an offline edge with throttle [FEAT-131] [tier:haiku] [owner:G5] [brief:briefs/streams/p3-g5.md#item-p3-g5-006]
  Needs: P3-G5-002
  Files: internal/agent/wake.go, internal/agent/wake_test.go
  Tests: TestWake_IPCRequestSendsPacket, TestWake_AutoOnOfflineEdgeContact, TestWake_ThrottledPer60sPerPeer (synctest), TestWake_AutoDisabledBySetting, TestWake_UnknownMACError
  DoD: coverage >= 85%; commit `feat(agent): Wake-on-LAN integration [FEAT-131]`

- [ ] [P3-G5-007] Lock and screensaver sync wiring on real session events [FEAT-138] [tier:haiku] [owner:G5] [brief:briefs/streams/p3-g5.md#item-p3-g5-007]
  Needs: P3-G5-002
  Files: internal/agent/locksync_wiring.go, internal/agent/locksync_wiring_test.go
  Tests: TestLockSync_ServerLockLocksClients, TestLockSync_ScreensaverStarts, TestLockSync_UnlockWakesDisplayOnly, TestLockSync_TogglesRespected, TestLockSync_ClientEventsNotPropagated, TestLockSync_SecureDesktopReturnsHome
  DoD: coverage >= 85%; commit `feat(agent): lock and screensaver sync wiring [FEAT-138]`

- [ ] [P3-G5-008] Real environment checks: Windows firewall query and tray availability [FEAT-150, FEAT-068] [tier:sonnet why:COM client calls into INetFwPolicy2 from pure Go] [owner:G5] [brief:briefs/streams/p3-g5.md#item-p3-g5-008]
  Needs: P3-G5-002
  Files: internal/firewall/firewall.go, internal/firewall/firewall_windows.go, internal/firewall/firewall_other.go, internal/firewall/firewall_test.go, internal/firewall/firewall_windows_integration_test.go, internal/agent/envchecks_real.go, internal/agent/envchecks_real_test.go, tools/coverage-exclude.txt (claim; adds `internal/firewall`, D-97)
  Tests: TestEvaluate_FirewallOffIsOK, TestEvaluate_DefaultInboundAllowIsOK, TestEvaluate_MatchingRuleIsOK, TestEvaluate_NoRuleIsProblem, TestEvaluate_DisabledOrBlockRuleIgnored, TestOther_Unsupported, TestEnvReal_TrayCheckFromBackend; TestFirewall_QueryReadsPolicy (integration, Windows)
  DoD: pure evaluation coverage >= 85% (`GOOS=linux go test -cover ./internal/firewall` in the Linux CI job, D-97); COM calls released on every path; read-only (never changes firewall state); cross-compile gates green; commit `feat(agent): firewall and tray environment checks [FEAT-150]`

- [ ] [P3-G5-009] E2E: pairing through IPC with real SPAKE2 over loopback [FEAT-162, FEAT-127] [tier:haiku] [owner:G5] [brief:briefs/streams/p3-g5.md#item-p3-g5-009]
  Needs: P3-G5-001
  Files: test/e2e/pairing_test.go
  Tests: TestE2E_PairingSucceedsAndPinsBothSides, TestE2E_WrongCodeThreeTimesInvalidates, TestE2E_CancelOnEitherSide, TestE2E_UnpairRejectsNextSession
  DoD: `-race` green; commit `test(e2e): pairing [FEAT-162]`

- [ ] [P3-G5-010] E2E: edge switching both directions, release-all and modifier sync, overlay, guard config via IPC [FEAT-162] [tier:haiku] [owner:G5] [brief:briefs/streams/p3-g5.md#item-p3-g5-010]
  Needs: P3-G5-005, P3-G5-009
  Files: test/e2e/switch_test.go
  Tests: TestE2E_SwitchRightAndBack, TestE2E_EntryPointMapping, TestE2E_ReleaseAllAndModSyncOnSwitch, TestE2E_OverlayOnReceivingMachine, TestE2E_ModifierGuardFromConfig, TestE2E_MotionDeliveredInOrderLatestWins
  DoD: `-race` green; commit `test(e2e): edge switching [FEAT-162]`

- [ ] [P3-G5-011] E2E: hotkeys lock, jump, cycle, panic [FEAT-162] [tier:haiku] [owner:G5] [brief:briefs/streams/p3-g5.md#item-p3-g5-011]
  Needs: P3-G5-010
  Files: test/e2e/hotkeys_test.go
  Tests: TestE2E_LockToScreenBlocksSwitch, TestE2E_JumpToMachine, TestE2E_CycleNextPrev, TestE2E_PanicReturnsHomeAndReleases, TestE2E_HotkeyKeysNeverForwarded, TestE2E_SetHotkeysTakesEffect
  DoD: `-race` green; commit `test(e2e): hotkeys [FEAT-162]`

- [ ] [P3-G5-012] E2E: lazy clipboard for every non-file format, relay between clients, password-manager style content, toggles [FEAT-162] [tier:haiku] [owner:G5] [brief:briefs/streams/p3-g5.md#item-p3-g5-012]
  Needs: P3-G5-009
  Files: test/e2e/clipboard_test.go
  Tests: TestE2E_ClipboardTextHTMLRTFPNG, TestE2E_NothingTransfersBeforePaste, TestE2E_RelayClientToClient, TestE2E_PasswordManagerStyleCopyAnnouncedLikeAny (D-121), TestE2E_FormatToggleOff, TestE2E_MasterToggleOff, TestE2E_NewerAnnouncementSupersedes
  DoD: `-race` green; commit `test(e2e): clipboard [FEAT-162]`

- [ ] [P3-G5-013] E2E: transfers via SendFiles, progress events, cancel, retry with resume, relay, clash rename [FEAT-162, FEAT-093] [tier:haiku] [owner:G5] [brief:briefs/streams/p3-g5.md#item-p3-g5-013]
  Needs: P3-G5-009
  Files: test/e2e/transfer_test.go
  Tests: TestE2E_SendFileAndFolder, TestE2E_ProgressEventsMonotonic, TestE2E_CancelThenRetryResumes, TestE2E_RelayBetweenClients, TestE2E_NameClashSuffix, TestE2E_ListTransfersAfterRestart
  DoD: `-race` green; commit `test(e2e): transfers [FEAT-162]`

- [ ] [P3-G5-014] E2E: star topology with three agents, Busy rejection, layout authority, desktop changes [FEAT-162, FEAT-002, FEAT-007] [tier:haiku] [owner:G5] [brief:briefs/streams/p3-g5.md#item-p3-g5-014]
  Needs: P3-G5-009
  Files: test/e2e/topology_test.go
  Tests: TestE2E_ServerDialsLayoutMembers, TestE2E_SecondServerGetsBusy, TestE2E_BusyHolderInClientStatus, TestE2E_LayoutSnapshotPushedToClients, TestE2E_SetLayoutRejectedOnClient, TestE2E_DesktopChangeUpdatesServer
  DoD: `-race` green; commit `test(e2e): star topology and layout authority [FEAT-162]`

- [ ] [P3-G5-015] E2E: link loss, return home, reconnect, offline edge wall and auto Wake-on-LAN [FEAT-162, FEAT-031, FEAT-131] [tier:haiku] [owner:G5] [brief:briefs/streams/p3-g5.md#item-p3-g5-015]
  Needs: P3-G5-006, P3-G5-010
  Files: test/e2e/resilience_test.go
  Tests: TestE2E_ActiveClientLostReturnsHome, TestE2E_ReconnectAfterRestart, TestE2E_OfflineNeighbourEdgeIsWall, TestE2E_AutoWakeSentOnceWithinThrottle, TestE2E_GUIIPCSurvivesPeerLoss
  DoD: `-race` green; commit `test(e2e): resilience and Wake-on-LAN [FEAT-162]`

- [ ] [P3-G5-016] E2E: lock and screensaver sync [FEAT-162, FEAT-138] [tier:haiku] [owner:G5] [brief:briefs/streams/p3-g5.md#item-p3-g5-016]
  Needs: P3-G5-007, P3-G5-009
  Files: test/e2e/locksync_test.go
  Tests: TestE2E_ServerLockLocksAllClients, TestE2E_ScreensaverPropagates, TestE2E_UnlockOnlyWakesDisplays, TestE2E_ToggleOffStopsPropagation
  DoD: `-race` green; commit `test(e2e): lock sync [FEAT-162]`

- [ ] [P3-G5-017] E2E budgets on loopback: motion latency, switch time, start to IPC ready [FEAT-170, FEAT-172] [tier:haiku] [owner:G5] [brief:briefs/streams/p3-g5.md#item-p3-g5-017]
  Needs: P3-G5-010
  Files: test/e2e/budget_test.go, test/e2e/bench_test.go
  Tests: TestE2E_MotionLatencyBudget (loopback p99 <= 5 ms, D-29), TestE2E_SwitchTimeBudget (PB-08 loopback p99 <= 20 ms), TestE2E_StartToIPCReadyBudget (PB-20 <= 500 ms), BenchmarkE2EMotion1000Hz
  DoD: budget tests carry `//go:build !race`; measured p50/p99/max reported for the parent's perf-budgets.md results rows; commit `perf(e2e): loopback latency and startup budgets [FEAT-170]`

- [ ] [P3-G5-018] Real-backend integration tests per OS and the CI e2e-integration job [FEAT-161, FEAT-168] [tier:haiku] [owner:G5] [brief:briefs/streams/p3-g5.md#item-p3-g5-018]
  Needs: P3-G5-003, P3-G5-004, P3-G5-005
  Files: test/e2e/realbackend_windows_test.go, test/e2e/realbackend_linux_test.go, test/e2e/realbackend_darwin_test.go, .github/workflows/ci.yml (claim)
  Tests (integration): TestReal_ClientInjectsFromFakeServer, TestReal_ServerCapturesAndSwitchesToFakeClient, TestReal_TrayAndOverlayStart, TestReal_ClipboardOfferAndRender; CI job `e2e-integration` (windows-latest, macos-latest, ubuntu-24.04 under Xvfb) runs `go test -tags integration ./test/e2e/...`
  DoD: `go vet -tags integration ./test/e2e/...` green on the host for all three GOOS values (cgo-free); actionlint clean; never run by the agent; commit `test(e2e): real-backend integration tests [FEAT-161]`

- [ ] [P3-G5-019] Manual script for the full end-to-end session (two Windows PCs + GNOME box) incl. PB-15, PB-17 idle and PB-20 [FEAT-161, FEAT-162, FEAT-170, FEAT-172, FEAT-179, FEAT-180, FEAT-181] [tier:haiku] [owner:G5] [brief:briefs/streams/p3-g5.md#item-p3-g5-019]
  Needs: P3-G5-008, P3-G5-018, P3-G5-021
  Files: qa/manual/p3-g5-e2e-session.md
  Tests: script follows `qa/manual/README.md`; covers edge switching across every configured edge in both directions, each guard, each hotkey, clipboard text/HTML/RTF/image both ways and client-to-client, lock/screensaver sync, Wake-on-LAN, cable pull and reconnect, tray actions, overlay, secure desktop smoke (UAC, Ctrl+Alt+Del from a macOS/Linux server keys, Ctrl+Alt+End and the tray action from a Windows server, D-113; the sign-in screen switch and driving the sign-in screen, D-112), lock and panic hotkeys from home with the GNOME box as server (portal dialog, D-114), idle CPU and working set sampling, start-to-IPC-ready from the agent log
  DoD: commit `docs(qa): end-to-end session script [FEAT-161]`

- [ ] [P3-G5-M01] Run the end-to-end session on the two Windows PCs and record results [FEAT-161, FEAT-162, FEAT-170, FEAT-172] [tier:haiku] [owner:USER] [brief:briefs/streams/p3-g5.md#item-p3-g5-m01]
  Needs: P3-G5-019
  Files: qa/manual/results/p3-g5-e2e-session.md
  Tests: every Windows step has a recorded observation; PB-15, PB-17 (idle), PB-20 numbers recorded
  DoD: results committed by the parent `docs(qa): end-to-end session results (Windows) [FEAT-161]`

- [ ] [P3-G5-M02] Run the end-to-end session with the GNOME box as client and as server and record results [FEAT-161, FEAT-162] [tier:haiku] [owner:USER] [brief:briefs/streams/p3-g5.md#item-p3-g5-m02]
  Needs: P3-G5-019
  Files: qa/manual/results/p3-g5-e2e-session.md (GNOME section)
  Tests: every GNOME step has a recorded observation incl. GNOME, mutter and xdg-desktop-portal versions
  DoD: results committed by the parent `docs(qa): end-to-end session results (GNOME) [FEAT-161]`

- [ ] [P3-G5-M03] Push and run the CI e2e-integration job; share the macOS and X11 results [FEAT-161, FEAT-168] [tier:haiku] [owner:USER] [brief:briefs/streams/p3-g5.md#item-p3-g5-m03]
  Needs: P3-G5-018
  Files: qa/manual/results/p3-g5-ci-integration.md
  Tests: run URL and the job logs of all three OS legs recorded
  DoD: results committed by the parent `docs(qa): e2e integration CI results [FEAT-161]`

- [ ] [P3-G5-R01] Code review of the G5 stream [FEAT-161, FEAT-162] [tier:sonnet why:per-stream code-review gate] [owner:G5] [brief:briefs/streams/p3-g5.md#item-p3-g5-r01]
  Needs: P3-G5-001 .. P3-G5-019 all [x], P3-G5-020, P3-G5-021 all [x]
  Files: none (read-only `code-reviewer`)
  Tests: every G5 diff against its item and brief
  DoD: VERDICT pass or every finding fixed via appended items; no commit by the reviewer

- [ ] [P3-G5-R02] Security review of the G5 stream (IPC usage, process launching, firewall COM, network surface) [FEAT-161] [tier:sonnet why:per-stream security-review gate] [owner:G5] [brief:briefs/streams/p3-g5.md#item-p3-g5-r02]
  Needs: P3-G5-R01
  Files: none (read-only `code-reviewer` with `qa/security-review.md`)
  Tests: sections 3, 7 and 8 of `qa/security-review.md` plus the brief's extra points
  DoD: VERDICT pass or every finding fixed via appended items; no commit by the reviewer

- [ ] [P3-G5-R03] Opus consolidation review of the G5 stream [FEAT-161, FEAT-162] [tier:opus] [owner:PARENT] [brief:briefs/streams/p3-g5.md#item-p3-g5-r03]
  Needs: P3-G5-R02, P3-G5-M01, P3-G5-M02, P3-G5-M03
  Files: briefs/plan-phase-3.md (append note), perf-budgets.md (append PB-01 loopback, PB-08, PB-15, PB-17 idle, PB-20 results rows)
  Tests: D-21, D-49, D-59, D-63, D-70, D-71, D-77 honoured end to end; budgets met or user-approved exceptions recorded
  DoD: note `<!-- NOTE: <date> - stream G5 review chain green -->`; commit `chore(plan): G5 review chain green [FEAT-161]`

- [ ] [P3-G5-020] IPC additions: GetPreloginAccess, SendCtrlAltDel and the environment-check kinds homeHotkeys and lockScreen (D-112 .. D-115) [FEAT-179, FEAT-180, FEAT-181, FEAT-182, FEAT-133] [tier:sonnet why:additive change across the frozen IPC schema, generated code, message types and IPC client, as in P3-F-006] [owner:G5] [brief:briefs/streams/p3-g5.md#item-p3-g5-020]
  Needs: P3-F-006, D-112, D-113
  Files: proto/schema/ipc.capnp (claim), internal/proto/capnp/ipccp/ipc.capnp.go (regenerated, claim), internal/proto/msg/ipc.go (claim), internal/proto/msg/ipc_types.go (claim), internal/proto/msg/ipc_additions_test.go, internal/ipc/client.go (claim; typed methods), internal/ipc/client_additions_test.go
  Tests: TestIPC_GetPreloginAccessRoundTrip, TestIPC_SendCtrlAltDelRoundTrip, TestIPC_EnvCheckKindsHomeHotkeysAndLockScreenRoundTrip, TestIPC_UnknownEnvCheckKindDecodesAsUnknownNotError, TestIPC_AdditionsUseIPCTraversalLimit, TestClient_TypedMethodsForNewRequests; FuzzDecodeIPCFrame corpus extended
  DoD: additive ordinals only; `task gen:check` green; coverage of internal/ipc and internal/proto/msg >= 85%; `task fuzz:smoke` green for the IPC decoders; per-item gates green; commit `feat(ipc): pre-login, Send Ctrl+Alt+Del and environment check additions [FEAT-133]`

- [ ] [P3-G5-021] Agent handlers: pre-login access through the service, Send Ctrl+Alt+Del request, home-hotkey and lock-screen environment checks (D-112 .. D-115) [FEAT-179, FEAT-180, FEAT-181, FEAT-182] [tier:haiku] [owner:G5] [brief:briefs/streams/p3-g5.md#item-p3-g5-021]
  Needs: P3-G5-020, P3-G5-008, P1-C3-026, P1-C3-027, P2-W2-038, P2-M1-028
  Files: internal/agent/ipc_prelogin.go, internal/agent/ipc_prelogin_windows.go, internal/agent/ipc_prelogin_other.go, internal/agent/ipc_prelogin_test.go, internal/agent/ipc_sendcad.go, internal/agent/ipc_sendcad_test.go, internal/agent/envchecks_extra.go, internal/agent/envchecks_extra_test.go, internal/agent/capabilities.go, internal/agent/capabilities_test.go, internal/agent/ipc_handler.go (registration lines), internal/agent/agent.go (one line: capabilities passed to the session options)
  Tests: TestPrelogin_AccessFromServiceClient (fake service client), TestPrelogin_NotWindowsReportsNotWindows, TestPrelogin_ServiceMissingReportsNoService, TestPrelogin_ServiceErrorReportedNotSilent, TestSendCADRequest_CallsEngineAndMapsError, TestSendCADRequest_ClientRoleUnavailable, TestEnvChecks_HomeHotkeysFromEngineStatus (bound, pending, unavailable, engine without provider), TestEnvChecks_LockScreenFromSessionLockSupport, TestEnvChecks_ExtraKindsNotApplicableOnOtherOSes, TestCapabilities_SecureDesktopTrueOnWindowsWithHelperInjectionAndService, TestCapabilities_SecureDesktopFalseWithoutServiceOrOnOtherOSes, TestE2E_SendCADChordReachesFakeClientInjector
  DoD: coverage of internal/agent >= 85%; per-item gates incl. cross-compile green; commit `feat(agent): pre-login, Send Ctrl+Alt+Del and platform check handlers [FEAT-179]`

### P3-X - Phase exit

- [ ] [P3-X-001] Consolidate P3: FEAT coverage, cross-stream consistency, perf results [FEAT-146, FEAT-161, FEAT-172] [tier:opus] [owner:PARENT] [brief:briefs/streams/p3-f.md#item-p3-x-001]
  Needs: P3-F-R04, P3-G1-R03, P3-G2-R04, P3-G3-R03, P3-G4-R04, P3-G5-R03
  Files: briefs/plan-phase-3.md (append note)
  Tests: every FEAT whose "Delivered by" or "Also" names P3 has its items `[x]`; PB-18, PB-19, PB-20, PB-21, PB-15 and the loopback PB-01/PB-08 rows exist in perf-budgets.md; `task ci:local` green
  DoD: commit `chore(plan): P3 consolidation [FEAT-146]`

- [ ] [P3-X-002] Run the smoke tests for the P3 build and record results [FEAT-161, FEAT-175] [tier:haiku] [owner:USER] [brief:briefs/streams/p3-f.md#item-p3-x-002]
  Needs: P3-X-001
  Files: qa/manual/results/smoke-p3-<date>.md
  Tests: every applicable row of `qa/smoke-tests.md` (S-11 and S-12 are n/a until P4) has an observation
  DoD: results committed by the parent `docs(qa): P3 smoke test results [FEAT-175]`

- [ ] [P3-X-003] P3 exit: verify exit criteria and flip the phase marker [FEAT-146] [tier:opus] [owner:PARENT] [brief:briefs/streams/p3-f.md#item-p3-x-003]
  Needs: P3-X-002
  Files: plan.md (append marker note), CLAUDE.md (mirror the marker)
  Tests: plan.md P3 exit criteria all met (GUI screens with i18n, e2e loopback green, GUI frame-time recorded, UI/UX audits passed, two-PC session passed)
  DoD: marker note appended listing every phase still active; commit `chore(plan): P3 exit [FEAT-146]`
