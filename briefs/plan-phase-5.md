# EdgeSync - Phase 5 items

Read `plan.md` (contract, orchestration, item format) first. Stream briefs: `briefs/streams/p5-r1.md` .. `p5-r5.md`
(the P5-X phase-exit items are briefed at the end of `p5-r5.md`).

Dependency notation. P1-P4 item IDs live in their own phase files; P5 items depend on finished streams and phases:
- "P2-W2-035" = every item in the W2 section of `briefs/plan-phase-2.md` is `[x]` and the W2
  "review chain green" NOTE exists in that file.
- "P3-X-003" = the phase-exit items of `briefs/plan-phase-3.md` are `[x]` (plan.md registry "Exit" criteria met).
- P5 items reference each other by ID. Items appended later (fix, tuning, missing-fuzz-target items) use IDs from 101 upward
  within their stream (`P5-R4-101`, ...), so the numbered items below never collide with appended work.

Gates used in DoD lines (in addition to the per-item gates of plan.md 1.5 for every item that touches Go code):
- "installer gates": `go run ./packaging/windows/isbuild build -version v0.0.1-dev -arch amd64 -bin <dir with the three exes> -out dist/installer`
  exits 0 (ISCC compiles `packaging/windows/edgesync.iss`) and `go test -tags isstest ./packaging/windows/...` is green (host, Inno Setup 7.1.1
  from P0-F-003). Agents compile installers; they NEVER run a generated setup or `unins*.exe`, and never install, repair or uninstall
  anything (the install lifecycle runs only on CI runners, P5-R1-018).
- "script gates": `sh -n <script>` (Git Bash) on the host; `shellcheck` and functional script tests run in CI (P5-R2-013).
- "docs gates": `go run ./tools/docscheck` (after P5-R5-002) and no U+2013/U+2014 characters in touched files.
- Workflows touched: `go tool -modfile=tools/tools.mod actionlint`.

Ownership: `ownership.md` P5 table, plus the new paths listed in each stream brief under "Owned paths" (the parent appends them
to `ownership.md` before the first dispatch of the stream). Shared files (`.github/workflows/ci.yml`, `Taskfile.yml`, `.gitignore`,
`.github/workflows/spikes.yml`) need a claim.

## Phase 5 - packaging, hardening & release

### R1 - Windows installer (Inno Setup 7.1.1), service install, firewall, autostart, SAS policy

- [ ] [P5-R1-001] Confirm the Windows packaging design points with the user and record the names found in merged code [FEAT-163, FEAT-041, FEAT-048] [tier:opus] [owner:PARENT] [brief:briefs/streams/p5-r1.md#item-p5-r1-001]
  Needs: P2-W1-029, P2-W2-035
  Files: decisions.md (parent appends an entry only if the user changes a point), briefs/plan-phase-5.md (parent appends a NOTE under this stream)
  Tests: design points (a)-(f) are closed (D-116 autostart, D-118 Inno Setup, D-104 targets, D-108 as amended; see the brief); the NOTE lists the SCM service name constant, the W1 HKCU Run value name, the W1 autostart constructor file, the W1 tray window procedure file, the agent lifecycle file that owns the root context cancel, the Windows icon file under `assets/icons/`, the default port constant and the `ISCC.exe` path and version recorded by P0-F-003, each verified by grep in `main` (or in `qa/manual/results/p0-f-toolchain.md`)
  DoD: NOTE appended; commit `docs(plan): Windows packaging names [FEAT-163]`

- [ ] [P5-R1-002] isbuild tool: version mapping, ISCC argument construction, preflight checks [FEAT-163, FEAT-145] [tier:haiku] [owner:R1] [brief:briefs/streams/p5-r1.md#item-p5-r1-002]
  Needs: P5-R1-001, P5-R2-002
  Files: packaging/windows/isbuild/main.go, packaging/windows/isbuild/version.go, packaging/windows/isbuild/version_test.go, packaging/windows/isbuild/isccargs.go, packaging/windows/isbuild/isccargs_test.go, packaging/windows/isbuild/preflight.go, packaging/windows/isbuild/preflight_test.go, packaging/windows/isbuild/runner.go, packaging/windows/isbuild/main_test.go, packaging/windows/README.md
  Tests: TestVersion_FromRelease, TestVersion_PrereleaseKeptInAppVersionDroppedFromVersionInfo, TestVersion_RejectsOutOfRange, TestIsccArgs_BuildCommand, TestIsccArgs_ArchMapping (amd64 -> x64os, arm64 -> arm64), TestIsccArgs_DefinesAndOutputName, TestIsccArgs_SignToolOnlyWhenConfigured, TestPreflight_MissingBinary, TestPreflight_IsccVersionMismatch, TestPreflight_IsccNotFound, TestCLI_UsageErrorsExit2
  DoD: `go run ./packaging/windows/isbuild build -version <v> -arch amd64|arm64 -bin <dir> -out <dir>` builds the ISCC command line from the brief and requires ISCC 7.1.1; all process execution behind the `runner` interface (tests use a fake); coverage of the package >= 85%; per-item gates green; commit `build(installer): isbuild tool with version mapping and preflight [FEAT-163]`

- [ ] [P5-R1-003] Inno Setup script skeleton: edgesync.iss with AppId, privileges, architectures, files, shortcut, downgrade refusal, and the isstest harness [FEAT-163] [tier:haiku] [owner:R1] [brief:briefs/streams/p5-r1.md#item-p5-r1-003]
  Needs: P5-R1-002, P3-F-R04, P5-R4-013
  Files: packaging/windows/edgesync.iss, packaging/windows/isbuild/isstest_helpers_test.go, packaging/windows/isbuild/iss_package_test.go, packaging/windows/README.md
  Tests (tag isstest): TestISS_CompilesForBothArchitectures, TestISS_AppIDIsTheFixedGUID, TestISS_PrivilegesRequiredAdmin, TestISS_ArchitecturesPerBuild, TestISS_InstallDirIsProgramFilesEdgeSync, TestISS_FilesFromBinDirAndLicense, TestISS_StartMenuShortcutTargetsGui, TestISS_OutputNameMatchesAssetName, TestISS_DowngradeRefusedMessage, TestISS_VersionInfoFromDefines
  DoD: AppId is a fixed GUID generated once with crypto/rand and documented as "never change" in packaging/windows/README.md; install directory `{commonpf}\EdgeSync` in 64-bit install mode with `DisableDirPage=yes`; the assertions run on `ISCC --preprocess` output and on a real compile; installer gates green; commit `build(installer): Inno Setup script skeleton [FEAT-163]`

- [ ] [P5-R1-004] Service installation, removal and recovery in the script: [Run]/[UninstallRun] entries, PrepareToInstall and failure policy [FEAT-163, FEAT-044] [tier:sonnet why:Inno Setup ordering of service removal, Restart Manager closing and failure detection across install, upgrade, uninstall and silent runs] [owner:R1] [brief:briefs/streams/p5-r1.md#item-p5-r1-004]
  Needs: P5-R1-003, P2-W2-017
  Files: packaging/windows/edgesync.iss ([Run], [UninstallRun] and [Code] sections), packaging/windows/isbuild/iss_service_test.go
  Tests (tag isstest): TestISS_ServiceInstallRunEntry, TestISS_ServiceUninstallRunEntry, TestISS_PrepareToInstallRemovesRunningService, TestISS_ServiceNameMatchesCode (parses `internal/service` with go/parser), TestISS_InstallFailureFailsSetup, TestISS_NoScExeUsed
  DoD: service name equals the `internal/service` constant; the entries call `edgesync-service install` and `edgesync-service uninstall` (x/sys/windows/svc/mgr, never sc.exe) with `runhidden waituntilterminated`; if the Inno Setup 7.1.1 help shows that `[Run]` cannot surface a non-zero exit code, the install call moves to `Exec` with a ResultCode check in `CurStepChanged(ssPostInstall)` and the choice is recorded in the README; a failed install call fails Setup (silent: non-zero exit code); recovery actions are those of P2-W2-017 (proven by the lifecycle job P5-R1-018); installer gates green; commit `build(installer): service install, removal and recovery [FEAT-163]`

- [ ] [P5-R1-005] Firewall rules for the agent: `edgesync-service firewall add|remove` and the script entries [FEAT-163] [tier:haiku] [owner:R1] [brief:briefs/streams/p5-r1.md#item-p5-r1-005]
  Needs: P5-R1-004
  Files: cmd/edgesync-service/firewall.go, cmd/edgesync-service/firewall_test.go, cmd/edgesync-service/main.go (one dispatch case), packaging/windows/edgesync.iss ([Run] and [UninstallRun] entries), packaging/windows/isbuild/iss_firewall_test.go
  Tests: TestFirewallArgs_AgentOnlyQuicUDP, TestFirewallArgs_TCPFallbackAndMdns, TestFirewallArgs_AllProfilesNoRemoteRestriction, TestFirewallArgs_PortFromDefaultsConstant, TestFirewallAdd_ReplacesExistingByName (fake runner: delete then add), TestFirewallRemove_DeletesAllThreeByName, TestFirewallCmd_RequiresElevation; tag isstest: TestISS_FirewallEntriesPresent
  DoD: inbound program-scoped rules for `edgesync-agent.exe` created through `netsh advfirewall firewall` behind a runner interface: UDP 24860, TCP 24860 (D-43), UDP 5353 (mDNS, D-15); all profiles, any remote address (D-16); no rule for the GUI or the service; rule names `EdgeSync agent (QUIC)`, `EdgeSync agent (TCP fallback)`, `EdgeSync agent (discovery)`; the netsh syntax is verified against Microsoft Learn at execution time; the read-only `internal/firewall` checker (D-96) is not changed; installer gates green; commit `build(installer): firewall rules for the agent [FEAT-163]`

- [ ] [P5-R1-006] Registry: machine autostart Run entry and conditional SoftwareSASGeneration policy in [Registry] [FEAT-163, FEAT-041, FEAT-048] [tier:sonnet why:conditional policy values that must preserve pre-existing machine policy across install, upgrade and uninstall with Inno Setup uninsdeletevalue semantics] [owner:R1] [brief:briefs/streams/p5-r1.md#item-p5-r1-006]
  Needs: P5-R1-005
  Files: packaging/windows/edgesync.iss ([Registry] section and the Check functions in [Code]), packaging/windows/isbuild/iss_registry_test.go
  Tests (tag isstest): TestISS_RunEntryQuotedWithAutostartFlag, TestISS_RunEntryDeletedOnUninstall, TestISS_SASPolicy_AbsentOrZeroWritesOne, TestISS_SASPolicy_TwoWritesThreePermanent, TestISS_SASPolicy_OneOrThreeUntouched, TestISS_SASReadUses64BitView
  DoD: Run value name equals the W1 value name from the P5-R1-001 NOTE; command line quoted (no unquoted-path vulnerability) with `--autostart` (D-116); the SAS rules are those of D-108 (absent or 0 -> 1 and removed on uninstall; 2 -> 3 and kept; 1 or 3 -> untouched); the 64-bit registry view is used; the lifecycle job (P5-R1-018) proves each rule on the runner; installer gates green; commit `build(installer): autostart entry and SAS policy [FEAT-163]`

- [ ] [P5-R1-007] ProgramData folder with a protected DACL, kept on uninstall [FEAT-163, FEAT-050] [tier:haiku] [owner:R1] [brief:briefs/streams/p5-r1.md#item-p5-r1-007]
  Needs: P5-R1-006
  Files: packaging/windows/edgesync.iss ([Dirs] entry and the icacls [Run] entry), packaging/windows/isbuild/iss_permissions_test.go
  Tests (tag isstest): TestISS_ProgramDataDirKeptOnUninstall, TestISS_ProgramDataIcaclsProtectsAndGrantsSystemAndAdmins, TestISS_InstallDirHasNoExtraPermissions
  DoD: `%ProgramData%\EdgeSync` created by a `[Dirs]` entry with `uninsneveruninstall`, then `icacls` removes inheritance and grants SYSTEM (`*S-1-5-18`) and Administrators (`*S-1-5-32-544`) full control with `(OI)(CI)` (Inno Setup `Permissions` can only add entries, it cannot create a protected DACL); no permission entries on the install dir; the lifecycle job reads the resulting SDDL back and expects `D:PAI(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)`; installer gates green; commit `build(installer): ProgramData folder ACL [FEAT-163]`

- [ ] [P5-R1-008] Machine-wide autostart mode: `--autostart` gate in the agent and per-user toggle for installer installs [FEAT-041, FEAT-140] [tier:sonnet why:integrates the installer's HKLM Run entry with the existing W1 Autostart backend and C5 startup reconcile logic whose code is only known after P2/P3] [owner:R1] [brief:briefs/streams/p5-r1.md#item-p5-r1-008]
  Needs: P5-R1-001, P3-X-003
  Files: internal/platform/windows/autostart_machine_windows.go, internal/platform/windows/autostart_machine_windows_test.go, cmd/edgesync-agent/autostart_gate_windows.go, cmd/edgesync-agent/autostart_gate_other.go, cmd/edgesync-agent/autostart_gate_test.go, cmd/edgesync-agent/main.go (one call), internal/platform/windows/autostart/autostart_windows.go (one edit: select machine mode)
  Tests: TestMachineAutostart_DetectsHKLMEntryForThisExe, TestMachineAutostart_DefaultEnabled, TestMachineAutostart_SetDisabledPersists, TestMachineAutostart_RemovesUserRunDuplicate, TestMachineAutostart_NotMachineModeWithoutHKLM, TestAutostartGate_ExitsWhenDisabled, TestAutostartGate_ContinuesWhenEnabled, TestAutostartGate_NoFlagNoCheck; integration: TestMachineAutostart_RealRegistryTempKey (HKCU temp subkey only, never HKLM)
  DoD: registry access behind an interface with a fake; the gate runs before any other initialisation; per-item gates incl. cross-compile green; commit `feat(autostart): machine-wide autostart mode for installer installs [FEAT-041]`

- [ ] [P5-R1-009] Installer close handshake: Restart Manager closing in the script and graceful agent shutdown on WM_CLOSE / WM_ENDSESSION [FEAT-163, FEAT-142] [tier:sonnet why:window-message shutdown path that must release grabbed input and notify peers before the installer replaces files] [owner:R1] [brief:briefs/streams/p5-r1.md#item-p5-r1-009]
  Needs: P5-R1-001, P5-R1-007, P3-X-003
  Files: packaging/windows/edgesync.iss ([Setup] close directives), packaging/windows/isbuild/iss_closeapps_test.go, internal/platform/windows/endsession_windows.go, internal/platform/windows/endsession_windows_test.go, internal/platform/windows/tray/tray_windows.go, internal/agent/lifecycle.go
  Tests: TestEndSession_CloseTriggersShutdownOnce, TestEndSession_QueryEndSessionReturnsTrue, TestEndSession_EndSessionCloseAppTriggersShutdown, TestEndSession_WaitsForShutdownUpToDeadline; tag isstest: TestISS_CloseApplicationsEnabledForExecutables
  DoD: the script sets `CloseApplications=yes`, `CloseApplicationsFilter=*.exe` and `RestartApplications=no` so Restart Manager closes `edgesync-agent.exe` and `edgesync.exe`; the agent's existing graceful shutdown path (verified to Ungrab, ReleaseAll and send Goodbye) runs on WM_CLOSE / WM_ENDSESSION and the process exits 0 within 3 s; installer gates and per-item gates green; commit `feat(win): graceful shutdown on installer close requests [FEAT-163]`

- [ ] [P5-R1-010] Release-time version stamping of the Windows resources: `isbuild stamp` [FEAT-037, FEAT-145] [tier:haiku] [owner:R1] [brief:briefs/streams/p5-r1.md#item-p5-r1-010]
  Needs: P5-R1-002, P2-W1-029
  Files: packaging/windows/isbuild/stamp.go, packaging/windows/isbuild/stamp_test.go, packaging/windows/isbuild/stamp_isstest_test.go
  Tests: TestParseGenerateDirective, TestStampArgs_VersionFlags, TestStamp_RunsOncePerCommand; tag isstest: TestStamp_SysoContainsVersionString
  DoD: stamp reuses each cmd package's own `//go:generate` goversioninfo invocation plus version flags verified with `goversioninfo -h`; used only in release CI (stamped .syso files are never committed); coverage >= 85%; commit `build(installer): release version stamping of Windows resources [FEAT-037]`

- [ ] [P5-R1-011] Resource manifest audit test for the three executables [FEAT-037, FEAT-163] [tier:haiku] [owner:R1] [brief:briefs/streams/p5-r1.md#item-p5-r1-011]
  Needs: P5-R1-002, P2-W1-029, P2-W2-035
  Files: packaging/windows/isbuild/resources_test.go
  Tests: TestResources_ManifestPerMonitorV2, TestResources_ManifestAsInvokerNoUIAccess, TestResources_ManifestSupportedOS, TestResources_SysoEmbedsManifest
  DoD: tests read `cmd/edgesync`, `cmd/edgesync-agent`, `cmd/edgesync-service` manifest.xml and committed .syso files (D-66); per-item gates green; commit `test(installer): Windows resource manifest audit [FEAT-037]`

- [ ] [P5-R1-012] Optional Authenticode signing, skipped without a certificate: `isbuild sign` and the ISCC SignTool wiring [FEAT-166, FEAT-163] [tier:haiku] [owner:R1] [brief:briefs/streams/p5-r1.md#item-p5-r1-012]
  Needs: P5-R1-002, P5-R1-009
  Files: packaging/windows/isbuild/sign.go, packaging/windows/isbuild/sign_test.go, packaging/windows/edgesync.iss ([Setup] SignTool and SignedUninstaller behind `#ifdef`), packaging/windows/README.md
  Tests: TestSign_SkippedWithoutCertificate, TestSign_CommandLineWithCertificate, TestSign_PasswordRedactedInLogs, TestSign_RequiresTimestampURL, TestLocateSigntool_PicksNewestKit, TestLocateSigntool_EnvOverride, TestSign_IsccSignToolArgOnlyWithCertificate
  DoD: no env -> prints `signing skipped: no certificate configured (D-26)`, exits 0 and the ISCC invocation carries no sign-tool argument; with env -> signtool sign + verify per exe and the installer is signed through the Inno Setup `SignTool` directive with `SignedUninstaller=yes`; coverage >= 85%; commit `build(installer): optional Authenticode signing step [FEAT-166]`

- [ ] [P5-R1-013] CI job `installer`: install the pinned Inno Setup, build, sign step, compile and upload the installers [FEAT-167, FEAT-163] [tier:haiku] [owner:R1] [brief:briefs/streams/p5-r1.md#item-p5-r1-013]
  Needs: P5-R1-004, P5-R1-005, P5-R1-006, P5-R1-007, P5-R1-009, P5-R1-010, P5-R1-012
  Files: .github/workflows/ci.yml (claim; adds job `installer` only)
  Tests: actionlint exits 0; the job uses only actions already pinned in ci.yml; steps exactly as in the brief
  DoD: commit `ci(installer): build and compile the installers on the Windows runners [FEAT-167]`

- [ ] [P5-R1-014] Manual script: installer install, upgrade, repair and uninstall on both Windows PCs [FEAT-163, FEAT-041, FEAT-048] [tier:haiku] [owner:R1] [brief:briefs/streams/p5-r1.md#item-p5-r1-014]
  Needs: P5-R1-008, P5-R1-011, P5-R1-013, P5-R1-018
  Files: qa/manual/p5-r1-installer.md
  Tests: script follows `qa/manual/README.md`; covers every step listed in the brief; references installers the agent prebuilt (versions 0.9.1 and 0.9.2) under `dist/manual/` by absolute path
  DoD: installers built (never run) by the agent; commit `docs(qa): installer install, upgrade and uninstall script [FEAT-163]`

- [ ] [P5-R1-M01] Run the installer script on Windows PC 1 and PC 2 and record results [FEAT-163] [tier:haiku] [owner:USER] [brief:briefs/streams/p5-r1.md#item-p5-r1-m01]
  Needs: P5-R1-014
  Files: qa/manual/results/p5-r1-installer.md
  Tests: every step has a recorded observation on both PCs
  DoD: verdict pass (failures become appended fix items first); results committed by the parent `docs(qa): installer results [FEAT-163]`

- [ ] [P5-R1-015] Stream review gate: code review of the R1 diff [FEAT-163] [tier:sonnet why:review gate] [owner:R1] [brief:briefs/streams/p5-r1.md#item-p5-r1-015]
  Needs: every other R1 item including P5-R1-M01 and P5-R1-018
  Files: none (findings are returned to the parent)
  Tests: `code-reviewer` over `git diff <R1 base>..main -- packaging/windows internal/platform/windows cmd/edgesync-agent cmd/edgesync-service .github/workflows/ci.yml` against the R1 items
  DoD: every finding appended by the parent as a fix item `P5-R1-101+`; zero open findings

- [ ] [P5-R1-016] Stream review gate: security review of the installer [FEAT-163, FEAT-173] [tier:sonnet why:security review gate] [owner:R1] [brief:briefs/streams/p5-r1.md#item-p5-r1-016]
  Needs: P5-R1-015 and its fix items
  Files: qa/security/r1-installer.md
  Tests: `code-reviewer` with `qa/security-review.md` sections 4 and 9 plus the installer checks in the brief
  DoD: findings appended as fix items; results file committed by the parent `docs(security): R1 installer review [FEAT-173]`

- [ ] [P5-R1-017] Opus consolidation review of R1 [FEAT-163] [tier:opus] [owner:PARENT] [brief:briefs/streams/p5-r1.md#item-p5-r1-017]
  Needs: P5-R1-016 and its fix items
  Files: briefs/plan-phase-5.md (NOTE)
  Tests: cross-stream consistency (service name, paths, Run value, firewall port vs D-43, autostart semantics vs D-21 and D-116, D-25, D-47, D-63, D-66, D-118)
  DoD: `<!-- NOTE: <date> - stream R1 review chain green -->` appended; commit `chore(plan): R1 review chain green [FEAT-163]`

- [ ] [P5-R1-018] CI job `installer-lifecycle`: silent install, state checks, upgrade, downgrade refusal, uninstall and cleanup checks on the runners [FEAT-163, FEAT-167, FEAT-041, FEAT-048] [tier:haiku] [owner:R1] [brief:briefs/streams/p5-r1.md#item-p5-r1-018]
  Needs: P5-R1-013, P5-R1-005, P5-R1-006, P5-R1-007, P5-R1-008
  Files: packaging/windows/ci/lifecycle.ps1, .github/workflows/ci.yml (claim; adds job `installer-lifecycle` only)
  Tests: actionlint exits 0; the PowerShell parser accepts `lifecycle.ps1` (parse only, never executed on the dev host); the script exits 2 unless `GITHUB_ACTIONS` is `true` (read, not run); steps exactly as in the brief
  DoD: the job runs on `windows-latest` (amd64) and `windows-11-arm` (arm64, R-55) and only on those runners; each leg installs the pinned Inno Setup, builds two installers (v0.0.1 and v0.0.2), then runs `lifecycle.ps1`: silent install -> service registered, running, automatic start, recovery actions as P2-W2-017; HKLM Run value; three firewall rules; SAS policy value; ProgramData SDDL; upgrade in place keeps one service, one Run value, three rules; downgrade refused with a non-zero exit; uninstall removes the service, Run value, firewall rules, the SAS value EdgeSync wrote and the install directory, and keeps `%ProgramData%\EdgeSync` (a seeded `machine-peers.toml` survives); a second pass with `SoftwareSASGeneration` pre-set to 2 ends at 3 and stays 3; the install logs are uploaded; commit `ci(installer): install, upgrade and uninstall lifecycle job [FEAT-167]`

### R2 - macOS .app zip, Linux tar.gz, release CI (GoReleaser OSS)

- [ ] [P5-R2-001] Confirm the macOS, Linux and release design points with the user and record the names found in merged code [FEAT-164, FEAT-165, FEAT-166] [tier:opus] [owner:PARENT] [brief:briefs/streams/p5-r2.md#item-p5-r2-001]
  Needs: P2-M1-027, P2-X1-025, P3-X-003
  Files: decisions.md (parent appends), briefs/plan-phase-5.md (NOTE), ownership.md (parent appends the P5 paths from the stream briefs)
  Tests: design points (a)-(h) are closed (D-104 .. D-107, D-117 (Q-06 answered); see the brief); the NOTE lists the M1 LaunchAgent label and writer file, the X1 autostart file name constant, the GUI file that launches the agent, the master PNG icon under `assets/icons/`, the `internal/buildinfo` variable names, and whether Windows GUI/agent builds already use `-H windowsgui`
  DoD: NOTE appended; commit `docs(plan): macOS, Linux and release packaging names [FEAT-166]`

- [ ] [P5-R2-002] Release version helper: SemVer parsing, ldflags, asset names (`packaging/release/relver` + `buildflags` CLI) [FEAT-145, FEAT-166] [tier:haiku] [owner:R2] [brief:briefs/streams/p5-r2.md#item-p5-r2-002]
  Needs: P5-R2-001
  Files: packaging/release/relver/relver.go, packaging/release/relver/relver_test.go, packaging/release/buildflags/main.go, packaging/release/buildflags/run.go, packaging/release/buildflags/run_test.go
  Tests: TestParse_ValidReleaseAndPrerelease, TestParse_RejectsInvalid, TestCIVersion, TestLDFlags_Common, TestLDFlags_WindowsGUISubsystem, TestAssetName_AllKinds (incl. windows-arm64 setup and linux-arm64 tarball, D-107, D-118), TestBuildflagsRun_OutputsAndExitCodes
  DoD: package doc comments; coverage >= 85% for both packages; per-item gates green; commit `build(release): version parsing, ldflags and asset names [FEAT-145]`

- [ ] [P5-R2-003] macOS Info.plist templates and renderer [FEAT-164] [tier:haiku] [owner:R2] [brief:briefs/streams/p5-r2.md#item-p5-r2-003]
  Needs: P5-R2-002
  Files: packaging/macos/plist/plist.go, packaging/macos/plist/plist_test.go, packaging/macos/plist/templates/gui.plist.tmpl, packaging/macos/plist/templates/agent.plist.tmpl
  Tests: TestRenderGUIPlist_Keys, TestRenderAgentPlist_KeysAndUsageStrings, TestRenderAgentPlist_BonjourServices, TestRender_VersionFields, TestRender_XMLEscaping
  DoD: keys exactly as in the brief, each usage-string key verified against Apple's Information Property List reference at execution time; per-item gates green; commit `build(macos): Info.plist templates for app and agent bundles [FEAT-164]`

- [ ] [P5-R2-004] appbundle assembler: nested app and agent bundles, PkgInfo, resources [FEAT-164] [tier:haiku] [owner:R2] [brief:briefs/streams/p5-r2.md#item-p5-r2-004]
  Needs: P5-R2-003, P5-R4-013
  Files: packaging/macos/appbundle/main.go, packaging/macos/appbundle/bundle.go, packaging/macos/appbundle/bundle_test.go, packaging/macos/appbundle/main_test.go
  Tests: TestBundle_Tree, TestBundle_PkgInfo, TestBundle_Modes (skipped on Windows hosts, runs in CI), TestBundle_RefusesExistingOutput, TestBundle_MissingInputs, TestCLI_ExitCodes
  DoD: coverage >= 85%; per-item gates green; commit `build(macos): app bundle assembler [FEAT-164]`

- [ ] [P5-R2-005] macOS universal build script for CI: `packaging/macos/build.sh` [FEAT-164, FEAT-166] [tier:haiku] [owner:R2] [brief:briefs/streams/p5-r2.md#item-p5-r2-005]
  Needs: P5-R2-004, P5-R2-006
  Files: packaging/macos/build.sh, packaging/macos/README.md
  Tests: script gates; the steps from the brief run on macos-latest in the release dry run (P5-R2-M01)
  DoD: deployment target 13.0 asserted, universal binaries asserted with `lipo -archs`, ad-hoc signing + verification, Developer ID step skipped without identity (D-26), zip via `ditto`; commit `build(macos): universal app bundle build script [FEAT-164]`

- [ ] [P5-R2-006] Locate and launch the agent inside the macOS bundle (`internal/paths` + GUI launch path) [FEAT-164, FEAT-146, FEAT-061] [tier:sonnet why:changes the existing P3-F agent-launch path and the M1 LaunchAgent target for the nested bundle without regressing Windows and Linux] [owner:R2] [brief:briefs/streams/p5-r2.md#item-p5-r2-006]
  Needs: P5-R2-001
  Files: internal/paths/agentexe.go, internal/paths/agentexe_darwin.go, internal/paths/agentexe_other.go, internal/paths/agentexe_test.go, gui/state/agentproc/agentproc_unix.go (the GUI agent-launch file; macOS uses `/usr/bin/open -g -j` per D-106) and (only if it does not derive the path from os.Executable) internal/platform/darwin/autostart/autostart_darwin.go
  Tests: TestAgentExecutableFor_DarwinBundle, TestAgentExecutableFor_DarwinBareBinary, TestAgentExecutableFor_WindowsSibling, TestAgentExecutableFor_LinuxSibling, TestLaunchCommand_DarwinUsesOpen, TestLaunchCommand_OtherExecsDirectly
  DoD: pure path logic table-tested on every host; per-item gates incl. cross-compile green; commit `feat(paths): locate and launch the agent inside the macOS bundle [FEAT-164]`

- [ ] [P5-R2-007] Linux desktop entry, autostart entry and their validator [FEAT-165, FEAT-070] [tier:haiku] [owner:R2] [brief:briefs/streams/p5-r2.md#item-p5-r2-007]
  Needs: P5-R2-001
  Files: packaging/linux/share/applications/io.github.kikz0r.edgesync.desktop, packaging/linux/autostart/edgesync-agent.desktop.tmpl, packaging/linux/desktopfile/desktopfile.go, packaging/linux/desktopfile/desktopfile_test.go
  Tests: TestDesktopFile_RequiredKeys, TestAutostartTemplate_Keys, TestAutostartTemplate_NameMatchesX1, TestParseDesktop_RejectsMalformed
  DoD: the autostart file name used by the tarball equals the X1 constant from the P5-R2-001 NOTE (test parses it with go/parser); per-item gates green; commit `build(linux): desktop and autostart entries [FEAT-165]`

- [ ] [P5-R2-008] Linux install and uninstall scripts with a functional test script [FEAT-165] [tier:haiku] [owner:R2] [brief:briefs/streams/p5-r2.md#item-p5-r2-008]
  Needs: P5-R2-007
  Files: packaging/linux/install.sh, packaging/linux/uninstall.sh, packaging/linux/test-install.sh
  Tests: script gates; `test-install.sh` (run in CI by P5-R2-013) installs a fake payload into a temp prefix and temp XDG dirs, asserts every installed path and the rewritten `Exec=`, runs uninstall and asserts nothing remains
  DoD: POSIX sh, no sudo inside the scripts, manifest-driven uninstall; commit `build(linux): install and uninstall scripts [FEAT-165]`

- [ ] [P5-R2-009] Linux tarball assembler and README-linux.txt [FEAT-165] [tier:haiku] [owner:R2] [brief:briefs/streams/p5-r2.md#item-p5-r2-009]
  Needs: P5-R2-008, P5-R2-002, P5-R4-013
  Files: packaging/linux/tarball/main.go, packaging/linux/tarball/tarball.go, packaging/linux/tarball/tarball_test.go, packaging/linux/tarball/main_test.go, packaging/linux/README-linux.txt
  Tests: TestTarball_Tree, TestTarball_ModesAndOwners, TestTarball_Deterministic, TestTarball_AutostartRendered, TestTarball_MissingInputs, TestCLI_ExitCodes
  DoD: byte-identical output for identical inputs; coverage >= 85%; per-item gates green; commit `build(linux): deterministic tarball assembler [FEAT-165]`

- [ ] [P5-R2-010] Linux build script for CI: `packaging/linux/build.sh` with runtime-dependency check [FEAT-165, FEAT-166] [tier:haiku] [owner:R2] [brief:briefs/streams/p5-r2.md#item-p5-r2-010]
  Needs: P5-R2-009
  Files: packaging/linux/build.sh, packaging/linux/README.md, packaging/linux/README-linux.txt (runtime package list section)
  Tests: script gates; on ubuntu-24.04 in the release dry run: Gio build deps installed per gioui.org/doc/install, `go version -m` shows the stamped version in both binaries, `desktop-file-validate` passes, `ldd` inside pinned `ubuntu:24.04` and `fedora` containers with only the documented runtime packages reports no "not found"
  DoD: commit `build(linux): CI build script for the tarball [FEAT-165]`

- [ ] [P5-R2-011] GoReleaser OSS configuration: draft release, extra files, checksums, changelog [FEAT-166] [tier:haiku] [owner:R2] [brief:briefs/streams/p5-r2.md#item-p5-r2-011]
  Needs: P5-R2-002
  Files: .goreleaser.yaml, .gitignore (claim; adds `build/goreleaser/`)
  Tests: `go run github.com/goreleaser/goreleaser/v2@v2.19.0 check` exits 0; `go run github.com/goreleaser/goreleaser/v2@v2.19.0 release --snapshot --skip=publish --clean` with placeholder files in `dist/release/` produces `checksums.txt` covering every placeholder
  DoD: every key used is verified as available in GoReleaser OSS v2.19.0 docs (a Pro-only key is BLOCKED, not substituted); commit `build(release): GoReleaser OSS configuration [FEAT-166]`

- [ ] [P5-R2-012] Release workflow: native per-OS jobs and draft publishing [FEAT-166, FEAT-164, FEAT-165, FEAT-163] [tier:haiku] [owner:R2] [brief:briefs/streams/p5-r2.md#item-p5-r2-012]
  Needs: P5-R2-005, P5-R2-010, P5-R2-011, P5-R1-010, P5-R1-012, P5-R1-013
  Files: .github/workflows/release.yml
  Tests: actionlint exits 0; jobs `version`, `windows`, `macos`, `linux`, `publish` exactly as in the brief; `permissions: contents: read` at top, `contents: write` only in `publish`; publish skipped on dry runs
  DoD: actions pinned to the exact tags already used in ci.yml (any new action pinned to its latest release tag verified via the GitHub API at execution time); commit `ci(release): native per-OS release workflow with draft publishing [FEAT-166]`

- [ ] [P5-R2-013] CI job `packaging`: shellcheck, desktop-file-validate, Linux install test [FEAT-167, FEAT-165] [tier:haiku] [owner:R2] [brief:briefs/streams/p5-r2.md#item-p5-r2-013]
  Needs: P5-R2-008, P5-R2-005, P5-R2-010
  Files: .github/workflows/ci.yml (claim; adds job `packaging` only)
  Tests: actionlint exits 0; job runs `shellcheck` on every `packaging/**/*.sh`, `desktop-file-validate` on the desktop files, `sh packaging/linux/test-install.sh`
  DoD: commit `ci(packaging): script and desktop-file checks [FEAT-167]`

- [ ] [P5-R2-014] Update-check compatibility fixtures for the release format [FEAT-137, FEAT-166] [tier:haiku] [owner:R2] [brief:briefs/streams/p5-r2.md#item-p5-r2-014]
  Needs: P5-R2-002, P5-R2-011
  Files: internal/update/testdata/release_latest_v1.0.0.json, internal/update/testdata/release_latest_prerelease.json, internal/update/release_compat_test.go
  Tests: TestReleaseCompat_ParsesTagAndURL, TestReleaseCompat_NewerThanBuild, TestReleaseCompat_SameVersionNoNotice, TestReleaseCompat_IgnoresPrerelease, TestReleaseCompat_AssetNamesMatchRelver
  DoD: tests use `internal/update`'s existing parser and comparison API unchanged; per-item gates green; commit `test(update): release format compatibility fixtures [FEAT-137]`

- [ ] [P5-R2-015] Manual script: release dry run on CI plus Linux tarball install on the GNOME box [FEAT-166, FEAT-165, FEAT-164] [tier:haiku] [owner:R2] [brief:briefs/streams/p5-r2.md#item-p5-r2-015]
  Needs: P5-R2-012, P5-R2-013, P5-R2-014
  Files: qa/manual/p5-r2-release-dryrun.md
  Tests: script follows `qa/manual/README.md`; covers the steps in the brief
  DoD: commit `docs(qa): release dry-run script [FEAT-166]`

- [ ] [P5-R2-M01] Push, dispatch the release dry run, install the tarball on the GNOME box, record results [FEAT-166, FEAT-165, FEAT-164] [tier:haiku] [owner:USER] [brief:briefs/streams/p5-r2.md#item-p5-r2-m01]
  Needs: P5-R2-015
  Files: qa/manual/results/p5-r2-release-dryrun.md
  Tests: run link shared; artefact listings and job logs recorded; every GNOME step observed
  DoD: verdict pass; results committed by the parent `docs(qa): release dry-run results [FEAT-166]`

- [ ] [P5-R2-016] Stream review gate: code review of the R2 diff [FEAT-166] [tier:sonnet why:review gate] [owner:R2] [brief:briefs/streams/p5-r2.md#item-p5-r2-016]
  Needs: every other R2 item including P5-R2-M01
  Files: none
  Tests: `code-reviewer` over the R2 diff against the R2 items
  DoD: findings appended as fix items `P5-R2-101+`; zero open findings

- [ ] [P5-R2-017] Stream review gate: release-integrity security review [FEAT-166, FEAT-173] [tier:sonnet why:security review gate for the supply chain of release artefacts] [owner:R2] [brief:briefs/streams/p5-r2.md#item-p5-r2-017]
  Needs: P5-R2-016 and its fix items
  Files: qa/security/r2-release.md
  Tests: `code-reviewer` with `qa/security-review.md` section 9 plus the release checks in the brief
  DoD: findings appended as fix items; results committed by the parent `docs(security): R2 release integrity review [FEAT-173]`

- [ ] [P5-R2-018] Opus consolidation review of R2 [FEAT-166] [tier:opus] [owner:PARENT] [brief:briefs/streams/p5-r2.md#item-p5-r2-018]
  Needs: P5-R2-017 and its fix items
  Files: briefs/plan-phase-5.md (NOTE)
  Tests: consistency of asset names, versions, bundle ids, autostart names and paths across R1, R2, M1, X1, C5 and D-25, D-26, D-72, D-78
  DoD: `<!-- NOTE: <date> - stream R2 review chain green -->` appended; commit `chore(plan): R2 review chain green [FEAT-166]`

### R3 - Performance benches, soak tests and tuning against perf-budgets.md

- [ ] [P5-R3-001] Re-check soak budgets PB-26 (memory growth) and PB-27 (handle/fd growth) against P3 measurements and append the soak bench definition [FEAT-172] [tier:opus] [owner:PARENT] [brief:briefs/streams/p5-r3.md#item-p5-r3-001]
  Needs: P3-X-003
  Files: perf-budgets.md (parent appends the bench/soak definition; PB-26/PB-27 rows exist since D-109), decisions.md (a superseding decision only if a threshold changes)
  Tests: PB-26/PB-27 compared with the P3 idle/active measurements; the user approved any threshold change
  DoD: commit `docs(perf): soak bench definition [FEAT-172]`

- [ ] [P5-R3-002] Bench report package: allocation-free histogram, percentiles, budget table, results-log rows [FEAT-170, FEAT-172] [tier:haiku] [owner:R3] [brief:briefs/streams/p5-r3.md#item-p5-r3-002]
  Needs: P5-R3-001
  Files: bench/report/report.go, bench/report/histogram.go, bench/report/budgets.go, bench/report/report_test.go, bench/report/histogram_test.go, bench/report/budgets_test.go
  Tests: TestHistogram_Percentiles, TestHistogram_RecordZeroAllocs, TestHistogram_Merge, TestBudgets_AllIDsPresent, TestBudgets_Evaluate, TestRow_MarkdownMatchesResultsLog
  DoD: budget table covers PB-01..PB-27 with the thresholds from perf-budgets.md; coverage >= 85%; per-item gates green; commit `perf(bench): report package [FEAT-170]`

- [ ] [P5-R3-003] Deterministic synthetic input trace (60 s at 1000 Hz) and its binary format [FEAT-170] [tier:haiku] [owner:R3] [brief:briefs/streams/p5-r3.md#item-p5-r3-003]
  Needs: P5-R3-002
  Files: bench/trace/trace.go, bench/trace/generate.go, bench/trace/trace_test.go
  Tests: TestGenerate_DeterministicForSeed, TestGenerate_RateAndDuration, TestGenerate_ContainsAllEventKinds, TestEncodeDecode_RoundTrip, TestDecode_RejectsCorrupt
  DoD: coverage >= 85%; per-item gates green; commit `perf(bench): synthetic 1000 Hz input trace [FEAT-170]`

- [ ] [P5-R3-004] bench/latency core: engine + session + transport between two peers, clock-offset estimation, fake timestamping injector [FEAT-170] [tier:sonnet why:composes the real engine, session and transport APIs into a measurement harness with heartbeat-based clock-offset estimation] [owner:R3] [brief:briefs/streams/p5-r3.md#item-p5-r3-004]
  Needs: P5-R3-003, P3-X-003
  Files: bench/latency/latency.go, bench/latency/peer.go, bench/latency/clock.go, bench/latency/latency_test.go, bench/latency/clock_test.go
  Tests: TestClockOffset_EstimateWithSkewAndJitter, TestLatency_LoopbackShortRunReportsPercentiles, TestLatency_LoopbackSanityP99 (loopback p99 <= 2 ms, PB-03 bound), TestLatency_PairsWithCode
  DoD: loopback tests run on the host (allowed: loopback only); coverage >= 85%; per-item gates incl. race green; commit `perf(bench): latency harness [FEAT-170]`

- [ ] [P5-R3-005] bench/latency modes `-bulk` (concurrent transfer) and `-switch` (edge switch timing) [FEAT-170, FEAT-171] [tier:sonnet why:drives the transfer engine and engine edge contacts concurrently with the latency stream] [owner:R3] [brief:briefs/streams/p5-r3.md#item-p5-r3-005]
  Needs: P5-R3-004
  Files: bench/latency/bulk.go, bench/latency/switch.go, bench/latency/bulk_test.go, bench/latency/switch_test.go
  Tests: TestBulk_SyntheticSourceNoDisk, TestBulk_LoopbackP99UnderLoad (<= 2 ms, PB-03 under bulk), TestSwitch_LoopbackMeasuresContactToShow, TestSwitch_IntervalHonoured
  DoD: coverage >= 85%; per-item gates incl. race green; commit `perf(bench): bulk and switch latency modes [FEAT-171]`

- [ ] [P5-R3-006] bench/transfer: raw TLS 1.3 baseline vs EdgeSync transfer, many-small-files mode [FEAT-171] [tier:sonnet why:baseline must use identical TLS settings and identities as the session so the comparison is fair] [owner:R3] [brief:briefs/streams/p5-r3.md#item-p5-r3-006]
  Needs: P5-R3-002, P3-X-003
  Files: bench/transfer/transfer.go, bench/transfer/baseline.go, bench/transfer/small.go, bench/transfer/transfer_test.go
  Tests: TestBaseline_LoopbackCopiesExactBytes, TestEdgeSync_LoopbackVerifiedTransfer, TestSmallFiles_CountsAndRate, TestReport_RatioAgainstBaseline
  DoD: coverage >= 85%; per-item gates incl. race green; commit `perf(bench): transfer throughput harness [FEAT-171]`

- [ ] [P5-R3-007] bench/resources: process CPU, RSS and handle/fd sampler (Windows and Linux) [FEAT-172] [tier:haiku] [owner:R3] [brief:briefs/streams/p5-r3.md#item-p5-r3-007]
  Needs: P5-R3-002
  Files: bench/resources/resources.go, bench/resources/sample_windows.go, bench/resources/sample_linux.go, bench/resources/sample_other.go, bench/resources/resources_test.go
  Tests: TestSample_SelfProcess, TestSampler_AveragesAndMax, TestFindProcessByName_NotFound, TestUnsupportedOS_ExplicitError
  DoD: self-sampling tests run on the host; per-item gates incl. cross-compile green; commit `perf(bench): process resource sampler [FEAT-172]`

- [ ] [P5-R3-008] bench/soak: loopback and LAN soak harness with growth-slope analysis [FEAT-172] [tier:sonnet why:IPC-driven exerciser plus leak heuristics over long runs] [owner:R3] [brief:briefs/streams/p5-r3.md#item-p5-r3-008]
  Needs: P5-R3-007, P5-R3-004
  Files: bench/soak/soak.go, bench/soak/exercise.go, bench/soak/slope.go, bench/soak/soak_test.go, bench/soak/slope_test.go
  Tests: TestSlope_LinearFit, TestSlope_IgnoresWarmup, TestSoak_LoopbackShortRunNoGrowth (60 s), TestExercise_LANPlanUsesIPCRequests (fake IPC client)
  DoD: coverage >= 85%; per-item gates incl. race green; commit `perf(bench): soak harness [FEAT-172]`

- [ ] [P5-R3-009] `cmd/edgesync-bench` CLI wiring all bench modes [FEAT-170, FEAT-171, FEAT-172] [tier:haiku] [owner:R3] [brief:briefs/streams/p5-r3.md#item-p5-r3-009]
  Needs: P5-R3-005, P5-R3-006, P5-R3-008
  Files: cmd/edgesync-bench/main.go, cmd/edgesync-bench/run.go, cmd/edgesync-bench/run_test.go
  Tests: TestRun_SubcommandDispatch, TestRun_UsageErrors, TestRun_OutputFormats
  DoD: subcommands and flags exactly as in the brief; per-item gates green; commit `perf(bench): edgesync-bench CLI [FEAT-170]`

- [ ] [P5-R3-010] CI workflow `bench.yml` for relative trend runs on the three runners [FEAT-172, FEAT-167] [tier:haiku] [owner:R3] [brief:briefs/streams/p5-r3.md#item-p5-r3-010]
  Needs: P5-R3-009
  Files: .github/workflows/bench.yml
  Tests: actionlint exits 0; workflow_dispatch only; matrix windows-latest, macos-latest, ubuntu-24.04; uploads results
  DoD: commit `ci(bench): dispatchable benchmark workflow [FEAT-172]`

- [ ] [P5-R3-011] Host and loopback budget run at the release-candidate commit [FEAT-170, FEAT-171, FEAT-172] [tier:haiku] [owner:R3] [brief:briefs/streams/p5-r3.md#item-p5-r3-011]
  Needs: P5-R3-009
  Files: bench/results/p5-host.md
  Tests: runs every host/loopback budget listed in the brief (PB-03, PB-04, PB-05, PB-06, PB-09 loopback, PB-14 loopback, PB-22, PB-23, PB-24, plus the bench/latency and bench/transfer loopback modes) and records each as a results-log row
  DoD: rows in the exact results-log format; misses listed under "Misses" with the command to reproduce; commit `perf(bench): host and loopback budget results [FEAT-170]`

- [ ] [P5-R3-012] Manual script: LAN performance measurements on the two Windows PCs [FEAT-170, FEAT-171, FEAT-172] [tier:haiku] [owner:R3] [brief:briefs/streams/p5-r3.md#item-p5-r3-012]
  Needs: P5-R3-009, P5-R1-M01
  Files: qa/manual/p5-r3-perf.md
  Tests: script follows `qa/manual/README.md`; covers PB-01, PB-02, PB-07, PB-08, PB-09, PB-10, PB-11, PB-12, PB-13, PB-14, PB-15, PB-16, PB-17, PB-18, PB-19, PB-20, PB-21 with exact commands and the pprof capture steps from the brief
  DoD: bench binary prebuilt by the agent into `dist/manual/`; commit `docs(qa): LAN performance script [FEAT-170]`

- [ ] [P5-R3-M01] Run the LAN performance script and record results [FEAT-170, FEAT-171, FEAT-172] [tier:haiku] [owner:USER] [brief:briefs/streams/p5-r3.md#item-p5-r3-m01]
  Needs: P5-R3-012
  Files: qa/manual/results/p5-r3-perf.md, bench/results/p5-lan/ (pprof files the user provides)
  Tests: every budget has a recorded number
  DoD: results committed by the parent; parent appends each number to the perf-budgets.md results log `docs(perf): LAN results [FEAT-170]`

- [ ] [P5-R3-013] Manual script: 8-hour LAN soak run [FEAT-172] [tier:haiku] [owner:R3] [brief:briefs/streams/p5-r3.md#item-p5-r3-013]
  Needs: P5-R3-009, P5-R1-M01
  Files: qa/manual/p5-r3-soak.md
  Tests: script follows `qa/manual/README.md`; covers setup, the `edgesync-bench soak -mode lan` command on both PCs, what to leave untouched, and the exact output to paste
  DoD: commit `docs(qa): LAN soak script [FEAT-172]`

- [ ] [P5-R3-M02] Run the 8-hour soak and record results [FEAT-172] [tier:haiku] [owner:USER] [brief:briefs/streams/p5-r3.md#item-p5-r3-m02]
  Needs: P5-R3-013
  Files: qa/manual/results/p5-r3-soak.md
  Tests: soak summary pasted for both agents
  DoD: results committed by the parent; PB-26/PB-27 rows appended to the results log `docs(perf): soak results [FEAT-172]`

- [ ] [P5-R3-014] Analysis of all budget results and tuning proposals [FEAT-170, FEAT-171, FEAT-172] [tier:sonnet why:performance diagnosis from profiles and measurements] [owner:R3] [brief:briefs/streams/p5-r3.md#item-p5-r3-014]
  Needs: P5-R3-011, P5-R3-M01, P5-R3-M02, P5-R3-010
  Files: bench/results/p5-analysis.md
  Tests: one verdict per budget PB-01..PB-27 with evidence; for each miss: profile evidence, root-cause hypothesis, proposed change with files, expected gain
  DoD: parent appends one tuning item per proposed change (`P5-R3-101+`, `[tier:sonnet why:perf tuning]`) or asks the user for an exception decision; commit `perf(bench): P5 budget analysis [FEAT-170]`

- [ ] [P5-R3-015] Re-measure after tuning: host re-run and a manual script limited to the budgets that missed [FEAT-170, FEAT-171, FEAT-172] [tier:haiku] [owner:R3] [brief:briefs/streams/p5-r3.md#item-p5-r3-015]
  Needs: P5-R3-014 and every tuning item appended from it
  Files: bench/results/p5-host-rerun.md, qa/manual/p5-r3-perf-rerun.md
  Tests: host budgets re-run; rerun script covers exactly the LAN budgets that missed (or states "none" and the USER item P5-R3-M03 is then marked n/a by the parent)
  DoD: commit `perf(bench): post-tuning re-measurement [FEAT-170]`

- [ ] [P5-R3-M03] Run the rerun script and record results [FEAT-170, FEAT-171, FEAT-172] [tier:haiku] [owner:USER] [brief:briefs/streams/p5-r3.md#item-p5-r3-m03]
  Needs: P5-R3-015
  Files: qa/manual/results/p5-r3-perf-rerun.md
  Tests: every re-measured budget has a number
  DoD: every budget met or covered by a user-approved exception decision; results committed by the parent `docs(perf): post-tuning LAN results [FEAT-170]`

- [ ] [P5-R3-016] Stream review gate: code review of the R3 diff [FEAT-170] [tier:sonnet why:review gate] [owner:R3] [brief:briefs/streams/p5-r3.md#item-p5-r3-016]
  Needs: every other R3 item including P5-R3-M03
  Files: none
  Tests: `code-reviewer` over the R3 diff and the tuning items against their items
  DoD: findings appended as fix items; zero open findings

- [ ] [P5-R3-017] Opus consolidation review of R3 [FEAT-170] [tier:opus] [owner:PARENT] [brief:briefs/streams/p5-r3.md#item-p5-r3-017]
  Needs: P5-R3-016 and its fix items
  Files: briefs/plan-phase-5.md (NOTE), perf-budgets.md (results log complete)
  Tests: every PB-01..PB-27 has a final row marked pass or an exception decision ID
  DoD: `<!-- NOTE: <date> - stream R3 review chain green -->` appended; commit `chore(plan): R3 review chain green [FEAT-170]`

### R4 - Security hardening: fuzz campaigns, protocol and ACL audits, licences, threat model

- [ ] [P5-R4-001] Fuzz target inventory against every untrusted-input decoder [FEAT-174, FEAT-119, FEAT-120] [tier:haiku] [owner:R4] [brief:briefs/streams/p5-r4.md#item-p5-r4-001]
  Needs: P4-X-001
  Files: qa/security/fuzz-inventory.md
  Tests: every decoder class in the brief's list appears with its package, function and existing `Fuzz*` target or "MISSING"
  DoD: parent appends one item per MISSING target (`P5-R4-101+`, haiku, Files = the package's fuzz test file + seed corpus); commit `docs(security): fuzz target inventory [FEAT-174]`

- [ ] [P5-R4-002] Fuzz campaign workflow `fuzz.yml` (matrix over every target, crasher artefacts) [FEAT-174, FEAT-167] [tier:haiku] [owner:R4] [brief:briefs/streams/p5-r4.md#item-p5-r4-002]
  Needs: P5-R4-001
  Files: .github/workflows/fuzz.yml
  Tests: actionlint exits 0; `workflow_dispatch` with input `fuzztime` (default `30m`) plus a weekly schedule; a `targets` job builds the matrix from `go test -list '^Fuzz'`; each matrix job uploads new `testdata/fuzz/**` files on failure
  DoD: commit `ci(fuzz): long fuzz campaign workflow [FEAT-174]`

- [ ] [P5-R4-M01] Push and dispatch the fuzz campaign; share the run link [FEAT-174] [tier:haiku] [owner:USER] [brief:briefs/streams/p5-r4.md#item-p5-r4-m01]
  Needs: P5-R4-002 and every MISSING-target item appended from P5-R4-001
  Files: qa/manual/results/p5-r4-fuzz-campaign.md
  Tests: run link, per-target outcome and crasher artefacts recorded
  DoD: results committed by the parent `docs(security): fuzz campaign results [FEAT-174]`

- [ ] [P5-R4-003] Triage fuzz crashers and commit regression corpora [FEAT-174] [tier:sonnet why:crash root-cause triage across decoders] [owner:R4] [brief:briefs/streams/p5-r4.md#item-p5-r4-003]
  Needs: P5-R4-M01
  Files: qa/security/fuzz-triage.md, internal/**/testdata/fuzz/** (only the crasher inputs from the campaign)
  Tests: each crasher reproduced with `go test -run '<Fuzz>/<id>'`; triage table: target, input, root cause, severity
  DoD: every real defect appended by the parent as a fix item; regression inputs committed; commit `test(fuzz): regression corpora from the P5 campaign [FEAT-174]`

- [ ] [P5-R4-004] SPAKE2 conformance review against RFC 9382 [FEAT-126, FEAT-173] [tier:sonnet why:cryptographic protocol review] [owner:R4] [brief:briefs/streams/p5-r4.md#item-p5-r4-004]
  Needs: P4-X-001
  Files: qa/security/spake2-review.md
  Tests: every check in the brief answered with `file:line` evidence and the RFC section
  DoD: findings appended as fix items; questions needing a decision (for example the password-to-scalar function) listed for the parent to ask; commit `docs(security): SPAKE2 RFC 9382 review [FEAT-126]`

- [ ] [P5-R4-005] Pairing protocol review: channel binding, attempts, TTL, error and timing uniformity, pin storage order [FEAT-127, FEAT-173] [tier:sonnet why:protocol state machine review against D-14 and the security checklist] [owner:R4] [brief:briefs/streams/p5-r4.md#item-p5-r4-005]
  Needs: P4-X-001
  Files: qa/security/pairing-review.md
  Tests: every check in the brief answered with evidence
  DoD: findings appended as fix items; commit `docs(security): pairing protocol review [FEAT-127]`

- [ ] [P5-R4-006] Adversarial TLS and pinning tests over QUIC and the TCP fallback [FEAT-125, FEAT-124, FEAT-173] [tier:sonnet why:adversarial tests composed from the session, transport and pki public APIs] [owner:R4] [brief:briefs/streams/p5-r4.md#item-p5-r4-006]
  Needs: P4-X-001
  Files: test/security/doc.go, test/security/tls_test.go, test/security/helpers_test.go, tools/coverage-exclude.txt (claim; adds `test/security`, D-97)
  Tests: TestTLS_UnpairedPeerRejectedQUIC, TestTLS_UnpairedPeerRejectedTCPFallback, TestTLS_DataConnectionRequiresSamePin, TestTLS_ReissuedCertSameKeyAccepted, TestTLS_SameNameDifferentKeyRejected, TestTLS_TLS12Rejected, TestTLS_NoApplicationDataBeforeAuth, TestTLS_ResumptionRechecksPin; unix CI only: TestIdentity_KeyFileModes
  DoD: tests green on the host (loopback); failures reveal defects -> parent appends fix items before this item is `[x]`; commit `test(security): adversarial TLS and pinning tests [FEAT-125]`

- [ ] [P5-R4-007] Adversarial local IPC tests [FEAT-132, FEAT-173] [tier:sonnet why:adversarial tests against the IPC server's authentication and framing limits] [owner:R4] [brief:briefs/streams/p5-r4.md#item-p5-r4-007]
  Needs: P4-X-001, P5-R4-006
  Files: test/security/ipc_test.go
  Tests: TestIPC_WrongTokenRejected, TestIPC_TokenCompareConstantTime (code inspection assertion via go/ast for `subtle.ConstantTimeCompare`), TestIPC_OversizeFrameRejectedBeforeAllocation, TestIPC_DeepNestingRejected, TestIPC_SecondServerCannotSquat, TestIPC_RequestBeforeHelloRejected; unix CI only: TestIPC_SocketAndDirModes
  DoD: tests green on the host (local pipes only); commit `test(security): adversarial IPC tests [FEAT-132]`

- [ ] [P5-R4-008] Windows service and secure-helper review: SDDL, per-request authorisation, image-path canonicalisation, token handling [FEAT-050, FEAT-044, FEAT-045, FEAT-046, FEAT-049, FEAT-051, FEAT-173] [tier:sonnet why:privilege-boundary review of SYSTEM code paths] [owner:R4] [brief:briefs/streams/p5-r4.md#item-p5-r4-008]
  Needs: P4-X-001
  Files: qa/security/service-review.md
  Tests: every check in the brief and in `qa/security-review.md` section 4 answered with `file:line` evidence
  DoD: findings appended as fix items; commit `docs(security): service and secure helper review [FEAT-050]`

- [ ] [P5-R4-009] Manual script: ACL and privilege checks on an installed Windows PC [FEAT-050, FEAT-163, FEAT-173] [tier:haiku] [owner:R4] [brief:briefs/streams/p5-r4.md#item-p5-r4-009]
  Needs: P5-R4-008, P5-R1-M01
  Files: qa/manual/p5-r4-acl.md
  Tests: script follows `qa/manual/README.md`; covers every step in the brief, including the standard-user and second-account checks
  DoD: commit `docs(qa): Windows ACL and privilege script [FEAT-050]`

- [ ] [P5-R4-M02] Run the ACL script and record results [FEAT-050] [tier:haiku] [owner:USER] [brief:briefs/streams/p5-r4.md#item-p5-r4-m02]
  Needs: P5-R4-009
  Files: qa/manual/results/p5-r4-acl.md
  Tests: every step observed
  DoD: verdict pass; results committed by the parent `docs(qa): ACL results [FEAT-050]`

- [ ] [P5-R4-010] Path-safety audit of every file write site plus the D-45 adversarial vector test [FEAT-095, FEAT-173] [tier:sonnet why:cross-cutting audit of every place that creates files from remote input] [owner:R4] [brief:briefs/streams/p5-r4.md#item-p5-r4-010]
  Needs: P4-X-001
  Files: internal/transfer/safepath/adversarial_test.go, qa/security/path-safety.md
  Tests: TestSanitize_D45Vectors (every class from D-45, each with several spellings), TestSanitize_LongWindowsPaths, TestWriteSites_UseRoot (go/ast scan: no `os.Create`/`os.OpenFile`/`os.WriteFile`/`os.MkdirAll` on remote-derived paths outside `os.Root` in the packages listed in the brief)
  DoD: write-site table in path-safety.md; vectors outside D-45 (listed in the brief) recorded with observed behaviour as findings for a user decision, not asserted; commit `test(security): path-safety audit and adversarial vectors [FEAT-095]`

- [ ] [P5-R4-011] Decode-limit audit and adversarial capnp/manifest tests [FEAT-119, FEAT-120, FEAT-173] [tier:sonnet why:crafting hostile Cap'n Proto segments and manifests against every decode entry point] [owner:R4] [brief:briefs/streams/p5-r4.md#item-p5-r4-011]
  Needs: P4-X-001, P5-R4-006
  Files: test/security/decode_test.go, qa/security/decode-limits.md
  Tests: TestDecode_TraversalLimitEnforced (per channel), TestDecode_DepthLimitEnforced, TestDecode_OversizeRejectedBeforeDecode, TestDecode_ManifestEntryCountBound, TestDecode_ManifestPathLengthBound, TestDecode_ManifestTotalSizeBound, TestDecode_HotPathUnknownTypeClosesStream
  DoD: decode-site table in decode-limits.md; commit `test(security): decode limit adversarial tests [FEAT-120]`

- [ ] [P5-R4-012] Clipboard privacy and log-redaction audit with a diagnostics-bundle secret test [FEAT-088, FEAT-139, FEAT-173] [tier:sonnet why:audit that no backend special-cases concealed content (D-121) and of redaction across logging and diagnostics] [owner:R4] [brief:briefs/streams/p5-r4.md#item-p5-r4-012]
  Needs: P4-X-001, P5-R4-006
  Files: test/security/redaction_test.go, qa/security/clipboard-privacy.md
  Tests: TestDiag_BundleContainsNoSecrets (seeded token, private key PEM, clipboard text, file names, full peer IDs), TestLogging_ClipboardContentNeverLogged (go/ast scan of slog calls in the packages listed in the brief), TestClipboard_NoSensitivitySpecialCasing (go/ast scan: no backend or manager code branches on concealed-content markers; D-121)
  DoD: per-backend table (Windows, macOS, X11, Wayland) with `file:line` showing the clipboard formats read on watch and confirming that no marker is interpreted; commit `test(security): clipboard privacy and redaction audit [FEAT-088]`

- [ ] [P5-R4-013] Dependency licence audit tool and generated THIRD_PARTY_NOTICES.txt [FEAT-173, FEAT-166] [tier:haiku] [owner:R4] [brief:briefs/streams/p5-r4.md#item-p5-r4-013]
  Needs: P4-X-001
  Files: tools/licenses/main.go, tools/licenses/licenses.go, tools/licenses/classify.go, tools/licenses/licenses_test.go, tools/licenses/classify_test.go, tools/licenses/generate.go, tools/licenses/testdata/** (licence text fixtures), packaging/release/THIRD_PARTY_NOTICES.txt (generated), qa/security/licence-audit.md
  Tests: TestClassify_KnownLicences, TestClassify_DualMITUnlicense, TestClassify_GPLFamilyDenied, TestClassify_UnknownDenied, TestCollect_AllGOOSAndBinaries (fake `go list` output), TestRender_SortedDeterministic
  DoD: `go generate ./tools/licenses/...` regenerates the notices file deterministically (covered by `task gen:check`); run against the real module passes (no denied or unknown licence) or BLOCKED with the list; coverage >= 85%; commit `build(licenses): dependency licence audit and notices [FEAT-173]`

- [ ] [P5-R4-014] govulncheck for every GOOS and the release binaries [FEAT-173] [tier:haiku] [owner:R4] [brief:briefs/streams/p5-r4.md#item-p5-r4-014]
  Needs: P4-X-001
  Files: qa/security/vulncheck.md
  Tests: `GOOS=windows`, `GOOS=linux`, `GOOS=darwin` source-mode runs over `./...` and binary-mode runs over the three host-built Windows binaries recorded verbatim (summary lines)
  DoD: zero findings, or each finding listed for the parent to turn into a bump decision + item; commit `docs(security): govulncheck across platforms [FEAT-173]`

- [ ] [P5-R4-015] Provenance and clean-room check (D-27) [FEAT-173, FEAT-024] [tier:haiku] [owner:R4] [brief:briefs/streams/p5-r4.md#item-p5-r4-015]
  Needs: P5-R4-013
  Files: qa/security/provenance.md
  Tests: grep results for the identifiers in the brief; every `third_party/` directory has its licence and is listed in THIRD_PARTY_NOTICES.txt; keymap generator input cites Chromium's BSD notice
  DoD: findings listed for the parent; commit `docs(security): provenance and clean-room check [FEAT-173]`

- [ ] [P5-R4-016] Network surface review with mDNS TXT and update-check tests [FEAT-129, FEAT-131, FEAT-137, FEAT-173] [tier:sonnet why:surface review across listener, discovery, wake-on-LAN and update-check code] [owner:R4] [brief:briefs/streams/p5-r4.md#item-p5-r4-016]
  Needs: P4-X-001, P5-R4-006
  Files: test/security/network_test.go, qa/security/network-surface.md
  Tests: TestMDNS_TXTOnlyAllowedKeys, TestListener_NoBannerBeforeTLS, TestListener_RejectsUnknownALPN, TestUpdate_ResponseSizeCapped, TestUpdate_OnlyGitHubAPIHost, TestWoL_NoInboundListener (go/ast scan)
  DoD: surface table in network-surface.md; commit `test(security): network surface review [FEAT-173]`

- [ ] [P5-R4-017] Threat model document [FEAT-173] [tier:sonnet why:threat modelling across components and trust boundaries] [owner:R4] [brief:briefs/streams/p5-r4.md#item-p5-r4-017]
  Needs: P5-R4-004, P5-R4-005, P5-R4-008, P5-R4-010, P5-R4-011, P5-R4-012, P5-R4-016
  Files: qa/threat-model.md
  Tests: structure exactly as in the brief (assets, actors, trust boundaries, STRIDE per component, mitigations with D-IDs and tests, residual risks with R-IDs, external review recommendation)
  DoD: commit `docs(security): threat model [FEAT-173]`

- [ ] [P5-R4-018] Stream review gate: code review of the R4 diff [FEAT-173] [tier:sonnet why:review gate] [owner:R4] [brief:briefs/streams/p5-r4.md#item-p5-r4-018]
  Needs: every other R4 item except P5-R4-019 and P5-R4-020, including P5-R4-M01, P5-R4-M02 and all appended fix items
  Files: none
  Tests: `code-reviewer` over the R4 diff against the R4 items
  DoD: findings appended as fix items; zero open findings

- [ ] [P5-R4-019] Full security review: every section of qa/security-review.md with evidence [FEAT-173] [tier:sonnet why:security review gate over the whole product] [owner:R4] [brief:briefs/streams/p5-r4.md#item-p5-r4-019]
  Needs: P5-R4-018 and its fix items, P5-R1-016, P5-R2-017
  Files: qa/security-review-results.md
  Tests: every checklist line of `qa/security-review.md` sections 1-9 marked pass or fail with `file:line` or test-name evidence; consolidated table of all P5 security findings and their fix items
  DoD: zero open critical/high/medium findings; low findings either fixed or accepted by a user decision; results committed by the parent `docs(security): full security review results [FEAT-173]`

- [ ] [P5-R4-020] Opus consolidation review of R4 [FEAT-173] [tier:opus] [owner:PARENT] [brief:briefs/streams/p5-r4.md#item-p5-r4-020]
  Needs: P5-R4-019
  Files: briefs/plan-phase-5.md (NOTE), risk-register.md (parent appends status notes for R-03, R-04, R-24, R-33, R-39, R-44)
  Tests: every R4 finding closed or decided; risk statuses updated
  DoD: `<!-- NOTE: <date> - stream R4 review chain green -->` appended; commit `chore(plan): R4 review chain green [FEAT-173]`

### R5 - QA matrix, documentation, spikes removal, release readiness

- [ ] [P5-R5-001] Ask the user about extra test machines (Windows 10, KDE Plasma) and record the second PC's Windows version [FEAT-175] [tier:opus] [owner:PARENT] [brief:briefs/streams/p5-r5.md#item-p5-r5-001]
  Needs: P3-X-003
  Files: compatibility-matrix.md (parent appends notes), risk-register.md (status notes for R-09, R-51)
  Tests: answers recorded; matrix rows for the QA run decided
  DoD: commit `docs(qa): P5 QA matrix machines [FEAT-175]`

- [ ] [P5-R5-002] Documentation checker `tools/docscheck` with a Taskfile task and CI step [FEAT-176] [tier:haiku] [owner:R5] [brief:briefs/streams/p5-r5.md#item-p5-r5-002]
  Needs: P3-X-003
  Files: tools/docscheck/main.go, tools/docscheck/docscheck.go, tools/docscheck/docscheck_test.go, tools/docscheck/testdata/**, Taskfile.yml (claim; adds `docs:check` and appends it to `ci:local`), .github/workflows/ci.yml (claim; adds a `docs` step to the `lint` job)
  Tests: TestCheck_BrokenRelativeLink, TestCheck_MissingAnchor, TestCheck_ForbiddenDashes, TestCheck_TodoMarkers, TestCheck_AbsoluteLocalPaths, TestCheck_UnclosedFence, TestCheck_CleanTreePasses
  DoD: coverage >= 85%; actionlint green; per-item gates green; commit `build(docs): documentation checker [FEAT-176]`

- [ ] [P5-R5-003] README.md for v1 [FEAT-176] [tier:haiku] [owner:R5] [brief:briefs/streams/p5-r5.md#item-p5-r5-003]
  Needs: P5-R5-002, P5-R5-001
  Files: README.md
  Tests: docs gates; sections exactly as in the brief
  DoD: commit `docs(readme): v1 README [FEAT-176]`

- [ ] [P5-R5-004] docs/install.md: install, upgrade, uninstall and first launch per OS [FEAT-176, FEAT-163, FEAT-164, FEAT-165, FEAT-183] [tier:haiku] [owner:R5] [brief:briefs/streams/p5-r5.md#item-p5-r5-004]
  Needs: P5-R5-002, P5-R1-017, P5-R2-018
  Files: docs/install.md
  Tests: docs gates; the macOS re-grant-after-update paragraph (D-117) and the Windows silent-install and downgrade facts are present; every fact cross-checked against the R1/R2 decisions and packaging files named in the brief
  DoD: commit `docs(install): installation guide [FEAT-176]`

- [ ] [P5-R5-005] docs/user-guide.md: roles, pairing, layout, guards, hotkeys, tray, indicator, sync features, updates [FEAT-176, FEAT-179, FEAT-180, FEAT-181] [tier:haiku] [owner:R5] [brief:briefs/streams/p5-r5.md#item-p5-r5-005]
  Needs: P5-R5-002
  Files: docs/user-guide.md
  Tests: docs gates; the Send Ctrl+Alt+Del, sign-in screen switch and Wayland home-hotkey limitation paragraphs (D-112, D-113, D-114) are present; every default value matches D-57/D-58 and `internal/config/defaults.go`
  DoD: commit `docs(guide): user guide [FEAT-176]`

- [ ] [P5-R5-006] docs/clipboard-and-files.md: clipboard, file paste, drag-and-drop, GUI send, transfer queue [FEAT-176] [tier:haiku] [owner:R5] [brief:briefs/streams/p5-r5.md#item-p5-r5-006]
  Needs: P5-R5-002, P4-X-001
  Files: docs/clipboard-and-files.md
  Tests: docs gates; states that concealed password-manager content is shared like any other content (D-121); per-OS behaviour matches the decisions appended after the P0 spikes and in P4
  DoD: commit `docs(guide): clipboard and file transfer guide [FEAT-176]`

- [ ] [P5-R5-007] docs/config-reference.md with a completeness test against the config schema [FEAT-176, FEAT-134] [tier:haiku] [owner:R5] [brief:briefs/streams/p5-r5.md#item-p5-r5-007]
  Needs: P5-R5-002
  Files: docs/config-reference.md, internal/config/docs_test.go
  Tests: TestConfigReference_EveryKeyDocumented, TestConfigReference_DefaultsMatch, TestConfigReference_NoUnknownKeys
  DoD: per-item gates and docs gates green; commit `docs(config): configuration reference [FEAT-176]`

- [ ] [P5-R5-008] docs/troubleshooting.md [FEAT-176] [tier:haiku] [owner:R5] [brief:briefs/streams/p5-r5.md#item-p5-r5-008]
  Needs: P5-R5-002, P5-R1-017, P5-R2-018
  Files: docs/troubleshooting.md
  Tests: docs gates; covers every topic in the brief (macOS permissions after updates and Gatekeeper, lock unsupported on a Mac, Wayland hotkeys at home, sign-in screen control, Wayland portal consent, GNOME tray extension, SmartScreen and AV for unsigned builds, firewall, discovery, SAS policy, UIPI, keyboard layouts, logs and diagnostics)
  DoD: commit `docs(troubleshooting): troubleshooting guide [FEAT-176]`

- [ ] [P5-R5-009] docs/security.md: user-facing security and privacy summary [FEAT-176, FEAT-173] [tier:haiku] [owner:R5] [brief:briefs/streams/p5-r5.md#item-p5-r5-009]
  Needs: P5-R5-002, P5-R4-017
  Files: docs/security.md
  Tests: docs gates; states that password-manager content is shared (D-121) and that the sign-in screen trust list is machine-wide (D-112); every claim traceable to a decision or to qa/threat-model.md
  DoD: commit `docs(security): security and privacy summary [FEAT-176]`

- [ ] [P5-R5-010] docs/development.md: building from source, gates, integration tests, CI, release process [FEAT-176, FEAT-169] [tier:haiku] [owner:R5] [brief:briefs/streams/p5-r5.md#item-p5-r5-010]
  Needs: P5-R5-002, P5-R2-012
  Files: docs/development.md
  Tests: docs gates; commands match `Taskfile.yml`, `CLAUDE.md` gate list and the workflows
  DoD: commit `docs(dev): development guide [FEAT-176]`

- [ ] [P5-R5-011] Release notes for v1.0.0 [FEAT-176, FEAT-166] [tier:haiku] [owner:R5] [brief:briefs/streams/p5-r5.md#item-p5-r5-011]
  Needs: P5-R5-003, P5-R5-004, P5-R5-005, P5-R5-006, P5-R5-007, P5-R5-008, P5-R5-009, P5-R5-010, P5-R3-017, P5-R4-020
  Files: docs/release-notes/v1.0.0.md
  Tests: docs gates; sections exactly as in the brief, including known limitations and the out-of-scope list from the feature inventory
  DoD: commit `docs(release): v1.0.0 release notes [FEAT-176]`

- [ ] [P5-R5-012] Release checklist [FEAT-166, FEAT-175] [tier:haiku] [owner:R5] [brief:briefs/streams/p5-r5.md#item-p5-r5-012]
  Needs: P5-R5-002, P5-R2-012
  Files: qa/release-checklist.md
  Tests: docs gates; pre-release, tagging (user), CI, draft review, publish and post-release sections as in the brief
  DoD: commit `docs(qa): release checklist [FEAT-166]`

- [ ] [P5-R5-013] Remove the spikes module after verifying its findings are archived [FEAT-178] [tier:haiku] [owner:R5] [brief:briefs/streams/p5-r5.md#item-p5-r5-013]
  Needs: P4-X-001
  Files: spikes/** (deleted), .github/workflows/spikes.yml (claim; deleted), .gitignore (claim; spikes entries removed)
  Tests: archive check passes (every `briefs/spikes/<stream>.md`, `briefs/spikes/p0-consolidation.md` and `qa/manual/results/p0-*.md` exists in `main`); `git grep -n "spikes/"` matches only plan, brief, decision, qa and docs history; `task ci:local` green
  DoD: the last commit hash containing `spikes/` reported in FILES so the parent records it in a NOTE; commit `chore(spikes): remove the spikes module after archiving findings [FEAT-178]`

- [ ] [P5-R5-014] QA matrix script: smoke tests and cross-OS combinations on release-candidate packages [FEAT-175, FEAT-168] [tier:haiku] [owner:R5] [brief:briefs/streams/p5-r5.md#item-p5-r5-014]
  Needs: P5-R5-001, P5-R1-M01, P5-R2-M01
  Files: qa/manual/p5-r5-matrix.md
  Tests: script follows `qa/manual/README.md`; covers the matrix in the brief (Windows pair, Windows + GNOME both directions, Xephyr X11 on the GNOME box, optional Windows 10 and KDE rows per P5-R5-001)
  DoD: commit `docs(qa): P5 QA matrix script [FEAT-175]`

- [ ] [P5-R5-M01] Run the Windows-pair part of the QA matrix and record results [FEAT-175] [tier:haiku] [owner:USER] [brief:briefs/streams/p5-r5.md#item-p5-r5-m01]
  Needs: P5-R5-014
  Files: qa/manual/results/p5-r5-matrix-windows.md
  Tests: smoke S-01..S-22 observed on the installer release candidate
  DoD: verdict pass (failures become appended fix items first); results committed by the parent `docs(qa): QA matrix Windows results [FEAT-175]`

- [ ] [P5-R5-M02] Run the GNOME, cross-OS and Xephyr parts of the QA matrix and record results [FEAT-175] [tier:haiku] [owner:USER] [brief:briefs/streams/p5-r5.md#item-p5-r5-m02]
  Needs: P5-R5-014
  Files: qa/manual/results/p5-r5-matrix-linux.md
  Tests: every matrix row observed
  DoD: verdict pass; results committed by the parent `docs(qa): QA matrix Linux results [FEAT-175]`

- [ ] [P5-R5-M03] Push main and share the CI run with all integration jobs green (Windows, macOS, Ubuntu + Xvfb) [FEAT-168, FEAT-175] [tier:haiku] [owner:USER] [brief:briefs/streams/p5-r5.md#item-p5-r5-m03]
  Needs: P5-R5-013
  Files: qa/manual/results/p5-r5-ci.md
  Tests: run link and the job list with outcomes recorded
  DoD: every integration job green; results committed by the parent `docs(qa): CI integration evidence [FEAT-168]`

- [ ] [P5-R5-015] Stream review gate: documentation accuracy and tool review [FEAT-176] [tier:sonnet why:review gate checking docs against decisions and code] [owner:R5] [brief:briefs/streams/p5-r5.md#item-p5-r5-015]
  Needs: every other R5 item including P5-R5-M01, P5-R5-M02, P5-R5-M03
  Files: none
  Tests: `code-reviewer` over the R5 diff: every documented default, path, port, hotkey and behaviour traced to a decision or code
  DoD: findings appended as fix items; zero open findings

- [ ] [P5-R5-016] Opus consolidation review of R5 [FEAT-176] [tier:opus] [owner:PARENT] [brief:briefs/streams/p5-r5.md#item-p5-r5-016]
  Needs: P5-R5-015 and its fix items
  Files: briefs/plan-phase-5.md (NOTE)
  Tests: docs cover FEAT-176's scope; QA matrix verdicts pass; spikes removed
  DoD: `<!-- NOTE: <date> - stream R5 review chain green -->` appended; commit `chore(plan): R5 review chain green [FEAT-176]`

### P5-X - Phase exit and release

- [ ] [P5-X-001] Release-readiness review: P5 exit criteria, FEAT coverage across all phases, pinned-version re-verification [FEAT-166, FEAT-175] [tier:opus] [owner:PARENT] [brief:briefs/streams/p5-r5.md#item-p5-x-001]
  Needs: P5-R1-017, P5-R2-018, P5-R3-017, P5-R4-020, P5-R5-016
  Files: qa/release-readiness.md, compatibility-matrix.md (append notes), plan.md (NOTE)
  Tests: every P5 item `[x]`; every FEAT-001..FEAT-178 has at least one `[x]` item across `briefs/plan-phase-*.md`; plan.md P5 exit criteria each evidenced; pins re-checked against proxy.golang.org by a Fork (newer versions recorded as notes only); `task ci:local` green on `main`
  DoD: readiness verdict "ready to tag v1.0.0" with the commit hash; commit `chore(plan): v1.0.0 release readiness [FEAT-166]`

- [ ] [P5-X-M01] Tag v1.0.0, push, review the draft release and publish it [FEAT-166] [tier:haiku] [owner:USER] [brief:briefs/streams/p5-r5.md#item-p5-x-m01]
  Needs: P5-X-001
  Files: qa/manual/results/p5-x-release.md
  Tests: following `qa/release-checklist.md` "Tagging" and "Draft review": annotated tag on the readiness commit, release workflow green, draft contains the three packages and `checksums.txt`, notes from `docs/release-notes/v1.0.0.md`, then published
  DoD: release URL recorded; results committed by the parent `docs(qa): v1.0.0 release record [FEAT-166]`

- [ ] [P5-X-M02] Post-release verification: checksums, clean install from the published assets, update notice from an older build [FEAT-166, FEAT-137] [tier:haiku] [owner:USER] [brief:briefs/streams/p5-r5.md#item-p5-x-m02]
  Needs: P5-X-M01
  Files: qa/manual/results/p5-x-postrelease.md
  Tests: following `qa/release-checklist.md` "Post-release": checksums verified, the installer run over the 0.9.x release candidate on PC 2, tarball installed on the GNOME box, a 0.9.x agent shows the v1.0.0 update notice (D-72)
  DoD: verdict pass; results committed by the parent `docs(qa): post-release verification [FEAT-166]`

- [ ] [P5-X-002] Close the build: final plan note, CLAUDE.md mirror, risk register statuses [FEAT-166] [tier:opus] [owner:PARENT] [brief:briefs/streams/p5-r5.md#item-p5-x-002]
  Needs: P5-X-M02
  Files: plan.md (NOTE), CLAUDE.md (mirror note), risk-register.md (status notes for every open risk)
  Tests: every risk has a final status note (closed, accepted with decision ID, or carried into post-v1 work)
  DoD: `<!-- NOTE: <date> - P5 exit: v1.0.0 released (<url>) -->` appended; commit `chore(plan): P5 exit, v1.0.0 released [FEAT-166]`
