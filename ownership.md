# EdgeSync - File ownership and shared-file claims

Each stream may create and edit only the paths it owns plus the exact files listed in an item's `Files:` line.
Paths are relative to the repo root; `/**` means the directory and everything below it. When a phase is over,
its streams' paths become editable by later items that list them explicitly in `Files:` (the parent checks).

## Shared files (claim required)

These files are edited by many streams. An item that lists one in `Files:` must hold a claim while its
subagent runs. The parent enforces at most one active claim per shared file.

| Shared file / path | Notes |
|---|---|
| `go.mod`, `go.sum` | dependency additions only with the exact version from `compatibility-matrix.md`; a new dependency needs a decision first |
| `tools/tools.mod`, `tools/tools.sum` | P0-F creates; later additions need a decision |
| `Taskfile.yml` | new tasks only; never weaken a gate |
| `.golangci.yml` | changes need a decision (D-55) |
| `tools/coverage-exclude.txt` | additions need a decision referencing D-37 |
| `.github/workflows/ci.yml`, `.github/workflows/spikes.yml`, `.github/workflows/release.yml` | run `actionlint` |
| `internal/platform/*.go` (interfaces and types) | FROZEN after P1-F; changes need a new decision + user approval; `internal/platform/files.go` frozen after P4-F |
| `internal/keymap/usage.go` | FROZEN after P1-F |
| `proto/schema/*.capnp` and `internal/proto/capnp/**` (generated) | schema changes after P1-F need a decision; regenerate in the same item |
| `internal/proto/wire/**` | frame table FROZEN after P1-F |
| `internal/config/schema.go` | additive fields only after P1-F, each with a migration |
| `internal/i18n/locales/en.toml` | additive keys; keys never renamed without updating all users in the same item |
| `internal/platform/host/host_linux.go` | X1 creates; Y1 claims to add the Wayland branch |
| `internal/platform/windows/win32/**` | shared Win32 declarations; files are prefixed by owning stream (`w1_*.go`, `w2_*.go`, `f1_*.go`, `f2_*.go`) |
| `internal/platform/windows/com/**` | W2 owns; F1/F2 claim for additions |
| `internal/agent/**` | C5 owns in P1; G5 owns in P3; P4-F claims for vfile/dnd wiring |
| `qa/manual/README.md` | parent only |
| `README.md` | P0-F creates; R5 owns in P5 |

### Claim protocol
1. Before dispatching an item that lists a shared file, the parent appends to the "Active claims" table below: `| <file> | <item id> | <date> |`.
2. The parent refuses to dispatch a second item claiming the same file while a claim is active.
3. After the item is merged into `main` (or abandoned), the parent deletes nothing: it appends `released <date>` in the same row's last column by adding a new row `| <file> | <item id> | released <date> |`.
4. A subagent that discovers it must edit an unlisted shared file stops and reports BLOCKED.

### Active claims (append-only log)

| File | Item | State |
|---|---|---|

---

## P0 - Spikes & toolchain

| Stream | Owns |
|---|---|
| P0-F | `go.mod`, `LICENSE`, `README.md`, `.gitattributes`, `.editorconfig`, `tools/**`, `.golangci.yml`, `Taskfile.yml`, `.github/workflows/ci.yml`, `.github/workflows/spikes.yml` (initial), `spikes/go.mod`, `spikes/README.md`, `qa/manual/results/p0-f-toolchain.md` |
| SP1 | `spikes/internal/winhook/**`, `spikes/internal/stats/**`, `spikes/winhooks/**`, `spikes/wininject/**`, `spikes/wincursor/**`, `spikes/winsvc/**`, `spikes/winpipe/**`, `qa/manual/p0-sp1-*.md`, `briefs/spikes/sp1.md` |
| SP2 | `spikes/internal/wincom/**`, `spikes/winvfiles/**`, `spikes/winclip/**`, `spikes/windnd/**`, `spikes/giodrop/**`, `qa/manual/p0-sp2-*.md`, `briefs/spikes/sp2.md` |
| SP3 | `spikes/linux/**`, `qa/manual/p0-sp3-*.md`, `briefs/spikes/sp3.md` |
| SP4 | `spikes/macos/**`, macos job steps in `.github/workflows/spikes.yml` (claim), `qa/manual/p0-sp4-*.md`, `briefs/spikes/sp4.md` |
| SP5 | `spikes/gioeditor/**`, `spikes/quicbench/**`, `spikes/mdns/**`, `spikes/capnp/**`, `spikes/spake2/**`, `spikes/ipc/**`, `qa/manual/p0-sp5-*.md`, `briefs/spikes/sp5.md` |
| PARENT | `briefs/spikes/p0-consolidation.md`, `qa/manual/results/**` (records user results), all plan files |

`spikes/go.mod` and `spikes/go.sum`: shared within P0 (each spike item adds its own dependency with the pinned version; claim required).

## P1 - Contracts & core

| Stream | Owns |
|---|---|
| P1-F | `internal/platform/*.go`, `internal/platform/fake/**`, `internal/platform/conformance/**`, `internal/platform/host/host.go`, `internal/keymap/usage.go`, `internal/proto/wire/**`, `proto/schema/**`, `internal/proto/capnp/**`, `tools/capnpgen/**`, `internal/config/schema.go`, `internal/config/defaults.go`, `internal/logging/conventions.go`, `internal/buildinfo/**` |
| C1 | `internal/transport/**`, `internal/session/**`, wire benches/fuzz under `internal/proto/wire/*_test.go` (claim) |
| C2 | `internal/pki/**`, `internal/spake2/**`, `internal/pairing/**`, `internal/peers/**`, `internal/discovery/**`, `internal/wol/**` |
| C3 | `internal/layout/**`, `internal/engine/**`, `internal/keymap/**` (except `usage.go`), `third_party/chromium/**` |
| C4 | `internal/clipboard/**`, `internal/transfer/**` |
| C5 | `internal/ipc/**`, `internal/config/**` (except `schema.go`, `defaults.go`), `internal/logging/**` (except `conventions.go`), `internal/i18n/**`, `internal/update/**`, `internal/locksync/**`, `internal/diag/**`, `internal/paths/**`, `internal/agent/**`, `cmd/edgesync-agent/**` |

## P2 - Platform backends

| Stream | Owns |
|---|---|
| W1 | `internal/platform/windows/**` except `clipboard/**`, `com/**`, `helper/**`, `vfile/**`, `dnd/**`; `internal/platform/windows/win32/w1_*.go`; `internal/platform/host/host_windows.go`; `cmd/*/versioninfo*.json`, `cmd/*/manifest.xml`, `cmd/*/resource_windows_amd64.syso`; `assets/icons/**` |
| W2 | `internal/platform/windows/clipboard/**`, `internal/platform/windows/com/**`, `internal/platform/windows/helper/**`, `internal/platform/windows/win32/w2_*.go`, `internal/service/**`, `cmd/edgesync-service/**`, the `--secure-helper` dispatch file `cmd/edgesync-agent/helper_windows.go` |
| X1 | `internal/platform/x11/**` except `vfile/**`, `dnd/**`; `internal/platform/linux/**`; `internal/platform/host/host_linux.go` |
| Y1 | `internal/platform/wayland/**` except `vfile/**`, `dnd/**`; claim on `internal/platform/host/host_linux.go` |
| M1 | `internal/platform/darwin/**` except `vfile/**`, `dnd/**`; `internal/platform/host/host_darwin.go` |

Integration-test CI jobs: each stream claims `.github/workflows/ci.yml` for its own job only.

## P3 - GUI & integration

| Stream | Owns |
|---|---|
| P3-F | `cmd/edgesync/**`, `gui/app/**`, `gui/shell/**`, `gui/theme/**`, `gui/widgets/**`, `gui/state/**` |
| G1 | `gui/layouteditor/**` |
| G2 | `gui/peers/**`, `gui/wizard/**` |
| G3 | `gui/settings/**` |
| G4 | `gui/transfers/**`, `gui/logs/**`, `gui/about/**` |
| G5 | `internal/agent/**` (claim from C5 era is closed), `test/e2e/**` |

## P4 - Native files & DnD

| Stream | Owns |
|---|---|
| P4-F | `internal/vfile/**`, `internal/dnd/**`, `internal/platform/files.go`, claim on `internal/transfer/**` and `internal/agent/**` |
| F1 | `internal/platform/windows/vfile/**`, `internal/platform/windows/win32/f1_*.go`, claim on `internal/platform/windows/com/**` |
| F2 | `internal/platform/windows/dnd/**`, `internal/platform/windows/win32/f2_*.go`, `gui/drop/drop_windows.go` (the shared `gui/drop/drop.go` contract belongs to P4-F, see the planning consolidation additions below) |
| F3 | `internal/platform/darwin/vfile/**`, `internal/platform/darwin/dnd/**`, `gui/drop/drop_darwin.go` |
| F4 | `internal/platform/x11/vfile/**`, `internal/platform/x11/dnd/**`, `gui/drop/drop_x11.go` |
| F5 | `internal/platform/wayland/vfile/**`, `internal/platform/wayland/dnd/**`, `gui/drop/drop_wayland.go` |

## P5 - Packaging, hardening & release

| Stream | Owns |
|---|---|
| R1 | `packaging/windows/**` |
| R2 | `packaging/macos/**`, `packaging/linux/**`, `.goreleaser.yaml`, `.github/workflows/release.yml` |
| R3 | `bench/**`, `cmd/edgesync-bench/**`, the "Results" sections of `perf-budgets.md` (via the parent) |
| R4 | `internal/**/testdata/fuzz/**` corpora, fixes claimed per file, `qa/security-review-results.md` |
| R5 | `docs/**`, `README.md`, `qa/manual/results/**` (via the parent), removal of `spikes/**` |

---

## Planning consolidation additions (2026-10-30)

Appended after every phase file existed; every path an item lists in `Files:` now has an owner here or above.

### Parent-owned in every phase (D-73)

| Owner | Paths |
|---|---|
| PARENT | `plan.md`, `CLAUDE.md`, `decisions.md`, `ownership.md`, `risk-register.md`, `perf-budgets.md`, `compatibility-matrix.md`, `briefs/**` (phase files, stream briefs, spike consolidation, `briefs/p4-preflight.md`, `briefs/open-questions.md`), `qa/manual/README.md`, `qa/manual/results/**`, `qa/release-readiness.md` |

### Additional shared files (claim required)

| Shared file / path | Notes |
|---|---|
| `.gitignore` | P0-F creates; R2 and R5 append patterns with a claim |
| `cmd/edgesync/pages.go` | P3-F creates the page registry; G1-G4 claim it for one append each (P3 spine gap 11) |
| `internal/i18n/locales/en.toml` | agent-side strings only after D-94 (C5, G5); GUI strings live in per-area files |
| `gui/drop/drop_linux.go` | F4 creates; F5 claims to add the Wayland branch |
| `packaging/release/THIRD_PARTY_NOTICES.txt` | R4 generates it (P5-R4-013) inside R2's `packaging/release/**` |
| `internal/engine/server.go`, `internal/engine/client.go` | C3 owns; P4-F-016 claims for the drag hooks |
| `internal/platform/x11/record_linux.go`, `internal/platform/x11/capturer_linux.go` | X1 owns; P4-F4-007 claims for passive mode (D-92) |
| `gui/peers/rows.go`, `gui/layouteditor/canvas.go`, `gui/shell/shell.go` | G2, G1, P3-F own; P4-F2-012 claims each for the drop targets |
| `internal/clipboard/**`, `internal/transfer/**` | C4 owns in P1; P4-F claims for vfile hooks and staging (D-101) |

### Per-stream manual scripts

| Pattern | Owner |
|---|---|
| `qa/manual/p[1-5]-*.md` | the stream named in the file name (`p2-w1-...` -> W1, `p3-g5-...` -> G5, `p5-r3-...` -> R3); written by that stream's script item, run by the user |

### P1 additions

| Stream | Owns |
|---|---|
| P1-F | `internal/proto/msg/**` (message types for the frozen schemas), `internal/peerid/**` |

### P2 additions

| Stream | Owns |
|---|---|
| W1 | `cmd/edgesync-agent/main.go`, `cmd/edgesync-agent/main_test.go`, `cmd/edgesync-agent/helper_windows.go`, `cmd/edgesync-agent/helper_other.go`, `internal/platform/host/runmain_other.go`, `internal/platform/host/runmain_other_test.go` (P2-X-003, D-84) |
| M1 | `internal/platform/host/runmain_darwin.go` (P2-M1-003, D-84) |
| Y1 | `third_party/libei/**` (vendored EI protocol XML with its licence) |

### P3 additions

| Stream | Owns |
|---|---|
| P3-F | `internal/i18n/locales/en/gui_shell.toml` |
| G1 | `internal/i18n/locales/en/gui_layout.toml` |
| G2 | `internal/i18n/locales/en/gui_machines.toml` |
| G3 | `internal/i18n/locales/en/gui_settings.toml` |
| G4 | `internal/i18n/locales/en/gui_transfers.toml` |
| G5 | `internal/firewall/**` (D-96) |

### P4 additions

| Stream | Owns |
|---|---|
| P4-F | `gui/drop/drop.go`, `gui/drop/area.go`, `gui/drop/area_test.go`, `gui/drop/router.go`, `gui/drop/router_test.go` (the shared drop contract, created by P4-F so no F stream has to run first), `internal/i18n/locales/en/files.toml` (D-94) |
| F2 | `gui/drop/drop_windows*.go`, `internal/i18n/locales/en/gui_drop.toml` (D-94) |
| F3 | `gui/drop/drop_darwin*` (incl. `drop_darwin.m`, D-100) |
| F4 | `gui/drop/drop_x11*`, `gui/drop/proxy_x11*.go`, `gui/drop/drop_linux.go` (creates; F5 claims) |
| F5 | `gui/drop/drop_wayland*` (incl. `drop_wayland_c.go`, D-100), `third_party/wayland/**` (D-103) |

### P5 additions

| Stream | Owns |
|---|---|
| R2 | `packaging/release/**` |
| R3 | `.github/workflows/bench.yml` |
| R4 | `test/security/**`, `qa/security/**`, `qa/threat-model.md`, `tools/licenses/**`, `.github/workflows/fuzz.yml` |
| R5 | `qa/release-checklist.md`, `tools/docscheck/**` |

## Product answer additions (2026-10-30, D-112 .. D-121)

Every path below is created or edited by an item appended or rewritten for the answers to `briefs/open-questions.md`. Paths inside a stream's
glob above are listed anyway so the rows cover every `Files:` entry; "claim" rows follow the claim protocol.

### Windows installer (Inno Setup 7.1.1, D-118): the superseded WiX paths are removed

The superseded R1 plan named `packaging/windows/wix/**` and `packaging/windows/msibuild/**`; neither exists any more.

| Stream | Owns |
|---|---|
| R1 | `packaging/windows/edgesync.iss`, `packaging/windows/README.md`, `packaging/windows/isbuild/main.go`, `packaging/windows/isbuild/version.go`, `packaging/windows/isbuild/version_test.go`, `packaging/windows/isbuild/isccargs.go`, `packaging/windows/isbuild/isccargs_test.go`, `packaging/windows/isbuild/preflight.go`, `packaging/windows/isbuild/preflight_test.go`, `packaging/windows/isbuild/runner.go`, `packaging/windows/isbuild/main_test.go`, `packaging/windows/isbuild/isstest_helpers_test.go`, `packaging/windows/isbuild/iss_package_test.go`, `packaging/windows/isbuild/iss_service_test.go`, `packaging/windows/isbuild/iss_firewall_test.go`, `packaging/windows/isbuild/iss_registry_test.go`, `packaging/windows/isbuild/iss_permissions_test.go`, `packaging/windows/isbuild/iss_closeapps_test.go`, `packaging/windows/isbuild/stamp.go`, `packaging/windows/isbuild/stamp_test.go`, `packaging/windows/isbuild/stamp_isstest_test.go`, `packaging/windows/isbuild/resources_test.go`, `packaging/windows/isbuild/sign.go`, `packaging/windows/isbuild/sign_test.go`, `packaging/windows/ci/lifecycle.ps1`, `qa/manual/p5-r1-installer.md`, `cmd/edgesync-service/firewall.go`, `cmd/edgesync-service/firewall_test.go` (P5-R1-005; one dispatch case in `cmd/edgesync-service/main.go`, W2 file) |

### P1 additions

| Stream | Owns |
|---|---|
| P1-F | `internal/platform/homeshortcuts.go`, `internal/platform/homeshortcuts_test.go`, `internal/platform/fake/homeshortcuts.go`, `internal/platform/fake/homeshortcuts_test.go` (P1-F-023, D-114; added before the contract freeze) |
| C3 | `internal/engine/server_sendcad.go`, `internal/engine/server_sendcad_test.go`, `internal/engine/home_shortcuts.go`, `internal/engine/home_shortcuts_test.go` (P1-C3-026, P1-C3-027; one dispatch case in `internal/engine/server_hotkeys.go`) |
| C5 | `internal/agent/tray.go`, `internal/agent/tray_test.go` (P1-C5-027 extends the files created by P1-C5-016) |

### P2 additions

| Stream | Owns |
|---|---|
| W2 | `internal/service/machinepeers/doc.go`, `internal/service/machinepeers/store.go`, `internal/service/machinepeers/store_test.go` (P2-W2-036); `internal/service/cmd_allowprelogin.go`, `internal/service/cmd_allowprelogin_test.go`, `cmd/edgesync-service/allowprelogin_windows.go` (P2-W2-037); `internal/service/cmd_preloginpeers.go`, `internal/service/cmd_preloginpeers_test.go`, `internal/service/client/prelogin.go`, `internal/service/client/prelogin_test.go`, edits to `internal/service/authz.go` and `internal/service/authz_test.go` (P2-W2-038) |
| Y1 | `internal/platform/wayland/portal/globalshortcuts_linux.go`, `internal/platform/wayland/portal/shortcuttrigger.go`, `internal/platform/wayland/portal/shortcuttrigger_test.go`, `internal/platform/wayland/portal/globalshortcuts_integration_test.go` (P2-Y1-026); `internal/platform/wayland/homeshortcuts.go`, `internal/platform/wayland/homeshortcuts_test.go`, `internal/platform/wayland/homecaps.go`, `internal/platform/wayland/homecaps_test.go`, `internal/platform/wayland/homeshortcuts_integration_test.go` (P2-Y1-027) |
| M1 | `internal/platform/darwin/session/locksupport.go`, `internal/platform/darwin/session/locksupport_test.go`, `internal/platform/darwin/session/locksupport_darwin.go` (P2-M1-028) |

### P3 additions

| Stream | Owns |
|---|---|
| G2 | `gui/peers/vm/prelogin.go`, `gui/peers/vm/prelogin_test.go`, `gui/peers/prelogin.go`, `gui/peers/prelogin_test.go`, `gui/peers/elevate_windows.go`, `gui/peers/elevate_other.go`, `gui/peers/elevate_test.go` (P3-G2-011, P3-G2-012; edit of `gui/peers/rows.go`); `gui/wizard/vm/notes.go`, `gui/wizard/vm/notes_test.go`, `gui/wizard/notes_page_test.go` (P3-G2-013; edits of `gui/wizard/steps.go` and `gui/wizard/permissionspage.go`) |
| G3 | `gui/settings/vm/hotkeys_extra.go`, `gui/settings/vm/hotkeys_extra_test.go`, `gui/settings/hotkeys_extra_test.go` (P3-G3-013; edit of `gui/settings/hotkeys.go`) |
| G5 | `internal/agent/ipc_prelogin.go`, `internal/agent/ipc_prelogin_windows.go`, `internal/agent/ipc_prelogin_other.go`, `internal/agent/ipc_prelogin_test.go`, `internal/agent/ipc_sendcad.go`, `internal/agent/ipc_sendcad_test.go`, `internal/agent/envchecks_extra.go`, `internal/agent/envchecks_extra_test.go` (P3-G5-021); `internal/proto/msg/ipc_additions_test.go`, `internal/ipc/client_additions_test.go` (P3-G5-020) |

### Additional shared files (claim required)

| Shared file / path | Notes |
|---|---|
| `internal/proto/msg/service.go`, `internal/proto/msg/service_test.go` | P1-F owns; P2-W2-038 claims them for the read-only `GetPreloginPeers` message (D-112) |
| `internal/proto/msg/ipc.go`, `internal/proto/msg/ipc_types.go`, `internal/ipc/client.go` | P1-F / C5 own; P3-G5-020 claims them for the IPC additions (same mechanism as D-93) |
| `proto/schema/service.capnp`, `internal/proto/capnp/servicecp/service.capnp.go` | P1-F owns; P2-W2-038 claims them (additive ordinal, D-112) |
| `proto/schema/ipc.capnp`, `internal/proto/capnp/ipccp/ipc.capnp.go` | P1-F owns; P3-G5-020 claims them (additive ordinals, D-112 .. D-115) |
| `cmd/edgesync-service/main.go` | W2 owns; P2-W2-037 and P5-R1-005 each add one dispatch case (one at a time) |
| `.github/workflows/ci.yml` | P5-R1-013 adds job `installer`, P5-R1-018 adds job `installer-lifecycle` (claims, one at a time) |

### R1 explicit main-module files (named in the R1 items; everything else under `internal/` stays with its stream)

| Stream | Owns |
|---|---|
| R1 | `internal/platform/windows/autostart_machine_windows.go`, `internal/platform/windows/autostart_machine_windows_test.go`, `internal/platform/windows/endsession_windows.go`, `internal/platform/windows/endsession_windows_test.go`, `cmd/edgesync-agent/autostart_gate_windows.go`, `cmd/edgesync-agent/autostart_gate_other.go`, `cmd/edgesync-agent/autostart_gate_test.go`; one-line edits in `cmd/edgesync-agent/main.go`, `internal/platform/windows/autostart/autostart_windows.go`, `internal/platform/windows/tray/tray_windows.go` and `internal/agent/lifecycle.go` (P5-R1-008, P5-R1-009); `qa/manual/results/p5-r1-installer.md` is written by the parent from the user's results (PARENT row above) |
| G5 | one registration edit in `internal/agent/ipc_handler.go` (P3-G5-021) |
| G5 | `internal/agent/capabilities.go`, `internal/agent/capabilities_test.go` (P3-G5-021; one line in `internal/agent/agent.go`) |
