# Stream brief - P3-F Foundation block (GUI shell)

Phase 3. Sequential, one agent at a time, in item order. Items: `briefs/plan-phase-3.md` section "P3-F". Owner tag:
`[owner:P3-F]` (PARENT items are done by the orchestrator). The sections "Conventions" and "GUI design" below are
binding for every P3 GUI item (P3-F, G1, G2, G3, G4); the G briefs refer to them instead of repeating them.

## Scope
Build everything the GUI pages stand on: the GUI coverage policy and the spine-gap decisions (PARENT), per-area i18n
catalogue files, the IPC/config additions the GUI needs, theme tokens and the Gio theme, formatting/validation helpers,
the state layer (IPC adapter, store, agent launcher), the widget library, the `guitest` harness, the shell (view model
and view), the Gio app loop and `cmd/edgesync`, import-boundary tests, CI dependencies for Gio on Linux, the headless
screenshot harness, and the manual script. No feature pages (those are G1-G4).

## Files you may touch
Only your item's `Files:` line. P3-F owns `cmd/edgesync/**`, `gui/app/**`, `gui/shell/**`, `gui/theme/**`,
`gui/widgets/**`, `gui/state/**` and `internal/i18n/locales/en/gui_shell.toml`. Shared files (`go.mod`, `go.sum`,
`proto/schema/ipc.capnp`, `internal/proto/capnp/**`, `internal/config/schema.go`, `tools/coverage-exclude.txt`,
`.github/workflows/ci.yml`) only with the claim the parent records. Never edit plan files, `decisions.md`,
`ownership.md`, `risk-register.md`, `perf-budgets.md`, `compatibility-matrix.md`, `CLAUDE.md`.

## Report format (reply with exactly this, nothing else)
```
FILES: <path> - <one clause>          (one line per file, or NONE)
GATES: <gate> pass|fail               (verbatim output only for a failure)
ITEMS: <item id> <FEAT-NNN, ...>
TESTS: <what the test phase must cover for this item, one line>
BLOCKED: <what stopped you, or NONE>
```
Review items (`code-reviewer`) reply with the `VERDICT:` / `FINDINGS:` format from `.claude/agents/code-reviewer.md`.

---

## Conventions (binding for P3-F and G1-G4)

### Decisions
D-19 and D-68 (i18n), D-22 (never run binaries, `integration` tag), D-33 (Gio v0.10.3; tray/overlay/file drop not in
Gio), D-34 and D-77 (GUI is an IPC client; closing the GUI never stops the agent; GUI starts the agent if absent),
D-46 (IPC auth), D-48 (logging, no clipboard content), D-52 (theme from the agent), D-55 (depguard: only `gui/` and
`cmd/edgesync` import `gioui.org`), D-56 (`testing/synctest`, go-cmp), D-75 (context-first, wrapped errors, no
package-level mutable state), plus the decisions appended by P3-F-001 and P3-F-002.

### Verified Gio facts (module cache `gioui.org@v0.10.3`, `gioui.org/x@v0.10.3`, checked 2026-10-29)
- `gioui.org/io/input.Router`: `Queue(events...)`, `Frame(*op.Ops)`, `Source()`, `Event(filters...)`, `MoveFocus`,
  `AppendSemantics([]SemanticNode)` (nodes carry `Desc.Label`, `Desc.Description`, `Desc.Class`, `Desc.Disabled`,
  `Desc.Selected`, `Desc.Bounds`, `Desc.Gestures`), `SemanticAt`, `WriteClipboard() (mime, content, ok)`,
  `ClipboardRequested()`. This is what `guitest` drives; no window and no GPU are needed.
- `gioui.org/app.Window`: `Event()`, `Option(...)` (`app.Title`, `app.Size`, `app.MinSize`), `Invalidate()` (safe from
  any goroutine), `Perform(system.Action)` with `system.ActionRaise`; `app.NewContext(ops, FrameEvent)`;
  `app.DestroyEvent`, `app.FrameEvent`, `app.ConfigEvent`.
- `gioui.org/x/explorer`: `NewExplorer(w *app.Window)`, mandatory `ListenEvents(evt)` for every window event,
  `ChooseFile(exts...)`, `ChooseFiles(exts...)` (returns `ErrNotAvailable` on macOS), `ChooseFolder() (string, error)`
  (Linux, macOS, Windows), `CreateFile(name)`; sentinels `ErrUserDecline`, `ErrNotAvailable`; desktop implementations
  return `*os.File` (Windows `os.Open(path)`, Linux portal URIs opened with `os.Open`, macOS `os.Open`).
  These calls block: run them off the frame goroutine.
- `gioui.org/gpu/headless`: `NewWindow(w, h)`, `Frame(*op.Ops)`, `Screenshot(*image.RGBA)`. Windows backend =
  Direct3D 11 hardware device, pure Go; Linux (EGL/Vulkan) and macOS need cgo. Treated as display/GPU access, so
  only in `integration`-tagged tests run by the user (P3-F-021).
- cgo: `go list -deps` with `CGO_ENABLED=0` for linux and darwin shows that `widget`, `widget/material`, `layout`,
  `io/input`, `op`, `text`, `font/gofont` pull in no cgo package; only `gioui.org/app`, `gpu/...` and
  `internal/{egl,gl,vk,cocoainit}` use cgo. Therefore: only `gui/app` and `cmd/edgesync` may import `gioui.org/app` or
  `gioui.org/x/explorer`; every other GUI package cross-vets for linux and darwin with `CGO_ENABLED=0`.
- `widget.Draggable` is in-app drag-and-drop only (no OS file drop; that is P4 `gui/drop`).

### Package split (view model / render)
- Pure packages (no `gioui.org` import, no import of a non-pure GUI package; enforced by P3-F-019): `gui/state` and
  its subpackages, `gui/theme/tokens`, `gui/widgets/format`, `gui/shell/vm`, and every `gui/<area>/vm`. They hold all
  decisions, state machines, validation, text selection and formatting. Coverage >= 85% (P3-F-001 decision).
- Gio packages: `gui/theme`, `gui/widgets`, `gui/widgets/guitest`, `gui/shell`, `gui/<area>` (views), `gui/app`,
  `cmd/edgesync`. They translate view-model output into widgets and pointer/key events into view-model calls; no
  business rules. Excluded from the percentage gate, but every interaction named in `Tests:` is driven through
  `guitest`.
- A view-model `Model` holds only UI-local state (selection, drafts, open dialogs, pending op IDs). It derives what
  it shows from a `state.Snapshot` passed in on every frame (`Present(snap state.Snapshot) Presentation`) and starts
  agent work only through `state.Doer` (implemented by `*state.Store`). Strings in a `Presentation` are already
  translated with `i18n.T`.

### Gio idioms
- One `op.Ops` per window, reset each frame by `app.NewContext`. Widgets are structs with state kept across frames
  (e.g. `widget.Clickable`, `widget.Editor`, `widget.List`); never allocate a new widget per frame.
- Event handling happens in the widget's layout function via `gtx.Event(filters...)` before drawing.
- Units: sizes in `unit.Dp`, text in `unit.Sp`; convert with `gtx.Dp(...)`/`gtx.Sp(...)`. Never hard-code pixels.
- Focus: focusable widgets call `event.Op(gtx.Ops, tag)` and filter `key.FocusFilter{Target: tag}`; draw the focus
  ring with `widgets.FocusRing` when `gtx.Focused(tag)`.
- Accessibility and tests: every interactive element emits a semantic class and label (`semantic.Button.Add`,
  `semantic.LabelOp(...)`, `semantic.DescriptionOp(...)`, `semantic.EnabledOp`, `semantic.SelectedOp`). `guitest`
  finds elements by these labels, so labels are the i18n text the user sees.
- The frame goroutine never blocks on IPC, files or pickers: requests go through `state.Doer`; pickers and other
  blocking calls run in goroutines that report back through the store and `Invalidate`.
- Keyboard shortcuts use `key.ModShortcut` (Ctrl on Windows/Linux, Cmd on macOS).

### i18n
- After P3-F-005 every GUI area has its own catalogue file in `internal/i18n/locales/en/` (gui_shell, gui_layout,
  gui_machines, gui_settings, gui_transfers) in the same TOML format as `locales/en.toml`. A key belongs to the file of
  its namespace (table below). Keys are added in the same commit as the code that uses them (the key test fails on
  unused keys), always as string literals passed to `i18n.T` (the test collects literals only).
- No user-visible string bypasses the catalogue: labels, tooltips, semantic labels, dialog texts, units, relative
  times. Machine names, peer IDs, paths, versions and log messages are data, not catalogue strings.
- Sentence case, short, specific; no em dashes or en dashes. Errors say what happened and what to do.

### Theme, DPI and look
- Colours only from `gui/theme/tokens` through `gui/theme`; never a literal colour in a page.
- The GUI follows the agent's `ThemeChanged` (D-52); light theme until the first theme value arrives.
- Sizes from the scale in "GUI design"; layouts must work at scale 1.0, 1.25, 1.5, 2.0 (tests use 1.0, 1.5, 2.0).
- State is never shown by colour alone: every badge has text and a distinct icon shape.

### Errors, logging, async
- Agent errors arrive as wrapped errors from the store; show `err` through the view model's message mapping (never a
  raw Go error string unless it is the agent's `ErrorInfo` message, which is already user-facing per C5).
- GUI logging: `log/slog` to the GUI log file decided by P3-F-002; log op names, durations and error kinds; never log
  clipboard data, pairing codes, file contents or the IPC session token.
- Operations longer than 300 ms show a spinner on the control that started them (`widgets.Button` pending state).

### Testing
- Pure packages: table-driven tests, go-cmp diffs, `testing/synctest` for anything time-based (countdowns, debounce,
  backoff, speed averages, searching delays).
- Gio packages: `guitest` (P3-F-013): build a frame with fixture snapshots from `gui/state/statetest`, queue pointer
  and key events, run the next frame, assert on semantic nodes, on view-model calls recorded by a fake `state.Doer`,
  and on clipboard writes. Every page test calls `guitest.AssertHitTargets` at scales 1.0, 1.5, 2.0.
- Visual checks are done by the user with the screenshot harness (P3-F-021) and the manual scripts; the UI/UX audit
  gates read those PNGs.

### Gates
Per-item gates from `plan.md` 1.5 plus the "GUI gates" at the top of `briefs/plan-phase-3.md`. Never run
`cmd/edgesync`, the agent, or `integration`-tagged tests.

### Commits
`<type>(<scope>): <description> [FEAT-NNN]`, body first line `Item: <item id>`. No attribution trailers. No em
dashes or en dashes anywhere.

---

## GUI design (binding; audited by P3-F-003)

### Window
- Title "EdgeSync" (`gui.shell.window_title`), default size 1000 x 700 dp, minimum 760 x 520 dp.
- Layout: left sidebar (200 dp) with the navigation list; to the right a content column made of: banner stack (0..n
  banners), page content (fills), status bar (32 dp) at the bottom.
- First run (config `first_run_completed = false`, P3-F-002 decision): the wizard route fills the whole window
  (no sidebar, no status bar) until it is finished.
- Closing the window exits the GUI process only (D-77). There is no "minimise to tray" in the GUI; the tray belongs to
  the agent.

### Routes and navigation map
| Route (`vm.Route`) | Sidebar | Owner | Page | Shortcut |
|---|---|---|---|---|
| `layout` | 1st | G1 | Layout editor | Shortcut+1 |
| `machines` | 2nd | G2 | Machines (paired, discovered, add by address, pairing dialogs) | Shortcut+2 |
| `transfers` | 3rd | G4 | Transfers (and the send-files dialog) | Shortcut+3 |
| `settings` | 4th | G3 | Settings tabs: General, Edge switching, Hotkeys, Key remap, Network | Shortcut+4 |
| `logs` | 5th | G4 | Logs (and diagnostics export) | Shortcut+5 |
| `about` | 6th | G4 | About and update notice | Shortcut+6 |
| `wizard` | hidden | G2 | First-run wizard (full window) | - |
| `permissions` | hidden | G2 | Permissions and environment checks | - |

- Default route after start: `layout` when the role is server, `machines` when the role is client; `-page <route>`
  overrides it (deep link, validated against this table).
- Navigation is by intent: `vm.Intent{Route, Params map[string]string}`. Parameters are consumed once by the target
  page. Defined parameters: `layout{select: <peerID>}`, `transfers{send: <peerID>}` (opens the send-files dialog with
  that target), `transfers{filter: active}`, `settings{tab: general|guards|hotkeys|remap|network}`,
  `wizard{}` (only through the gate), `permissions{}`.
- Cross-area links (no package imports between areas): machine card "Send files" -> `transfers{send}`; machine card
  "Show in layout" -> `layout{select}`; About "Change in Settings" -> `settings{tab: general}`; settings "Run setup
  again" -> config patch that re-enables the gate; banners -> `permissions`, `about`.
- History: Alt+Left / browser-style back is not provided; the sidebar is the navigation. Dialogs close with Escape.

### App-wide states (replace the page content, rendered by the shell)
| State (`state.ConnState`) | Shown | Actions |
|---|---|---|
| Connecting / StartingAgent | spinner + "Connecting to the EdgeSync agent..." / "Starting the EdgeSync agent..." | none |
| Disconnected | error state "The EdgeSync agent is not running" + last error detail | [Retry] (store.RetryNow), [Start agent] (launcher) |
| VersionMismatch | error state "This window (version X) does not match the agent (version Y). Restart EdgeSync." | [Retry] |
| AuthFailed | error state "Could not authenticate with the EdgeSync agent." | [Retry] |
| Connected | the page | - |

### Banners (stack above the page, highest priority first, max 3 visible)
1. Permission or environment problem (any `PermissionState.Granted == false`, or an environment check in state
   `problem`): warning, "EdgeSync needs your attention" + [Review] -> `permissions`.
2. Update available (`UpdateAvailable` with a version not dismissed in this GUI session): info, "EdgeSync <v> is
   available" + [Details] -> `about` + [Dismiss].
3. Transient result banners pushed by pages (e.g. "Layout applied"), auto-hide after 4 s (synctest-tested), info or
   error kind.

### Status bar (left to right)
- Role and control: server -> "Home: this computer" or "Controlling <machine>"; client -> "Controlled by <server>",
  "Waiting for the server", or "Held by <server>" when another server holds this client (Busy, D-59).
- Lock indicator when lock-to-screen is on: "Locked to this screen" with a lock icon.
- Active transfers: "<n> transfers" (hidden at 0), clickable -> `transfers{filter: active}`.

### Screens and their states (each page implements all of these)
| Screen | Loading | Empty | Error | Notes |
|---|---|---|---|---|
| Layout | until `Layout` and `Peers` slices are loaded | server with no other machines: home machine + hint and [Go to Machines] | slice load error + [Retry] | client: read-only banner |
| Machines | until `Peers` and `Discovered` loaded | no paired machines: explanation; discovered: "Searching..." 5 s then "No EdgeSync machines found" with guidance | per-slice error + [Retry]; per-row request errors inline | pairing dialogs |
| Transfers | until `Transfers` loaded | "No transfers yet" + [Send files] | slice error + [Retry]; per-row failure reason | send-files dialog |
| Settings | until `Config` (+ `Hotkeys` for that tab, + `Peers` for remap) loaded | remap with no paired machines: explanation | slice error + [Retry]; per-field error inline | client-role notes on guards, hotkeys, remap |
| Logs | until `Logs` loaded | "No log records at this level" | slice error + [Retry] | diagnostics export |
| About | never blocks (local data) + agent version when loaded | - | update-check error inline | |
| Wizard | step-local spinners | - | step-local errors | full window |
| Permissions | until `Permissions` and `EnvChecks` loaded | "Everything is set up." | slice error + [Retry] | hidden route |

### Keyboard map
- Global: Shortcut+1..6 pages; Tab / Shift+Tab focus order sidebar -> banners -> page -> status bar; Escape closes the
  top dialog; Enter activates the focused button or the dialog's default action.
- Tabs (settings): Shortcut+Tab / Shortcut+Shift+Tab cycle tabs; arrow keys move between tab headers.
- Layout canvas (G1): arrows nudge 1 px, Shift+arrows 10 px; PageDown/PageUp select next/previous machine; Delete
  removes from layout; Escape cancels a drag or clears the selection; F or Shortcut+0 fit; Shortcut+= / Shortcut+-
  zoom; Shortcut+S apply.
- Logs (G4): Shortcut+A select all visible, Shortcut+C copy selection.

### Theme tokens (`gui/theme/tokens`, values sRGB hex)
| Token | Light | Dark | Use |
|---|---|---|---|
| Bg | #FFFFFF | #15181C | window background |
| Surface | #F5F6F8 | #1E2228 | sidebar, cards |
| SurfaceAlt | #EBEDF0 | #272C33 | hover, selected rows, status bar |
| Divider | #D5D9DF | #333941 | decorative lines (no contrast requirement) |
| ControlBorder | #7A8494 | #7D8794 | field and control outlines (>= 3:1 on Bg and Surface) |
| Text | #1B1F24 | #E8EAED | body text |
| TextMuted | #4D5562 | #A9B1BC | secondary text (>= 4.5:1 on Bg, Surface, SurfaceAlt) |
| Primary | #1F5FBF | #6EA8FE | primary buttons, links, focus ring, active machine |
| OnPrimary | #FFFFFF | #0B1220 | text on Primary |
| Selection | #D6E4F7 | #23395B | selected rows / machines fill |
| Success | #1A7F37 | #4CC38A | online badge |
| Warning | #8A5300 | #F5B94A | warning text/icon |
| WarningBg | #FFF4E5 | #2E2614 | warning banner background |
| Danger | #B42318 | #FF6B6B | errors, destructive buttons |
| DangerBg | #FDECEA | #3A1D1D | error banner background |
| InfoBg | #EAF2FD | #182638 | info banner background |
| MachineFill | #E3E8EF | #2A3038 | layout machine group |
| MonitorFill | #FFFFFF | #1B1F24 | layout monitor |
| GuideLine | #D9480F | #FF8A4C | snap guides |
| OverlapLine | #1A7F37 | #4CC38A | shared edge segments |

Required pairs (checked by P3-F-007): Text and TextMuted on Bg, Surface, SurfaceAlt, Selection >= 4.5:1;
OnPrimary on Primary >= 4.5:1; Primary (as link text) on Bg and Surface >= 4.5:1; Warning on WarningBg, Danger on
DangerBg, Text on InfoBg >= 4.5:1; Danger and Success on Bg >= 4.5:1; ControlBorder, Primary (focus ring), GuideLine,
OverlapLine on Bg, Surface and MachineFill >= 3:1. If a listed pair fails with these values, report BLOCKED with the
computed ratio; do not change values silently.

### Typography, spacing, sizes
- Fonts: `gofont.Collection()` (Go sans for UI, Go Mono for codes, peer IDs, logs).
- Text sizes (sp): title 20, section 16, body 14, small 12 (never smaller); pairing code 40 (mono).
- Spacing scale (dp): 4, 8, 12, 16, 24, 32. Corner radius 6 dp. Focus ring 2 dp, offset 2 dp.
- Hit targets: buttons and toggles >= 32 x 32 dp; nothing interactive below 24 x 24 dp (asserted in tests).
- List rows 40 dp (two-line rows 56 dp). Dialog width 480 dp (wide dialogs 640 dp), max height 80 % of the window.

### Widget catalogue (`gui/widgets`, P3-F-014/015)
Banner (info/warning/error, up to 2 actions, optional dismiss), EmptyState (icon, title, text, optional action),
LoadingState (spinner + text), ErrorState (title, detail, Retry), Badge (kinds: Home, Online, Offline, Active,
Connecting, Busy, Warning; text + shape), ProgressBar (0..1), Spinner, Icon set (drawn with `clip.Path`: monitor,
machine, home, lock, check, warning, error, info, close, plus, minus, folder, file, arrow-right, arrow-left, refresh,
copy, link), Button (primary, secondary, danger, text; pending state), Toggle, TextField, NumberField (range, step),
SelectBox, RadioGroup, Tabs, Dialog (modal, focus trap, Escape), ConfirmDialog, FocusRing, and the `Pickers` interface.

### i18n namespaces and files
| Namespace | File | Owner |
|---|---|---|
| `gui.common.*` (OK, Cancel, Retry, Close, units, relative times), `gui.shell.*` | `gui_shell.toml` | P3-F |
| `gui.layout.*` | `gui_layout.toml` | G1 |
| `gui.machines.*`, `gui.pairing.*`, `gui.wizard.*`, `gui.permissions.*` | `gui_machines.toml` | G2 |
| `gui.settings.*` | `gui_settings.toml` | G3 |
| `gui.transfers.*`, `gui.send.*`, `gui.logs.*`, `gui.diag.*`, `gui.about.*` | `gui_transfers.toml` | G4 |

### Store and data flow
Agent -> IPC events -> `state.Store` (background goroutine) -> new immutable `Snapshot` -> `Invalidate` -> next frame:
shell and the current page call `Present(snapshot)` on their view models -> widgets. User input -> view model ->
`state.Doer.Do(...)` (async op) -> IPC request -> result recorded in `Snapshot.Ops` and/or arrives as an event.

---

## Spine gaps (closed at the planning consolidation by D-93 .. D-96; P3-F-002 confirms against main)
The spine (architecture section 7, config section 8, D-48) does not define these; P3-F-002 checks the P1
implementation first and appends decisions only for what is missing.
1. Environment checks for the wizard and the permissions page: request `GetEnvironmentChecks` -> list of
   `EnvCheck{kind: firewallInbound | trayAvailable, state: ok | problem | unsupported, hint: none |
   windowsFirewallAllow | gnomeAppIndicatorExtension, detail: text}` and event `EnvironmentChanged`. Agent side:
   `internal/agent/envchecks.go` with an `EnvChecker` interface (fake in tests; real Windows firewall checker in the
   new package `internal/firewall`, added by P3-G5-008, excluded from the coverage gate as OS glue per D-37; tray
   availability from `Backend.Tray()` errors).
2. Manual address removal: request `RemoveManualPeer(host, port)`; manual addresses appear in `ListDiscovered` with
   source `manual`.
3. GUI single instance (FEAT-141, D-77): `Hello` result field `primaryGUI` (false when another GUI is attached); a
   secondary GUI calls `RequestFocusGUI(route)` and exits 0; the agent forwards `FocusGUI{route}` to the primary GUI.
4. First-run state: config table `[gui]` with `first_run_completed` (bool, default false; the migration sets it to true
   for an existing config that already has paired peers) and `wizard_step` (int, default 0), changed through
   `UpdateConfig`.
5. Per-area i18n catalogue files (`locales/en/*.toml`, duplicates rejected, key test spans all files), amending D-68.
6. GUI log file: `edgesync-gui.log` in the D-48 per-user log directory, same rotation (10 MiB x 5), level from the
   `-log-level` flag (default info).
7. Theme on connect: the agent sends `ThemeChanged` with the current theme right after a successful GUI `Hello`.
8. Pairing event variants needed by the GUI: `codeShown(code, expiresAt)` (responder), `awaitingCode(target,
   expiresAt, attemptsLeft)` (initiator), `peerJoined(name)`, `attemptFailed(attemptsLeft)`, `succeeded(peer)`,
   `failed(reason: tooManyAttempts | expired | cancelledLocal | cancelledRemote | unreachable | rejected | protocol,
   detail)`.
9. Busy holder (D-59): status carries the server peer that currently holds this client and the server that was
   rejected.
10. GUI IPC reconnect backoff: 250 ms min, 5 s max, factor 2, full jitter (GUI-local, separate from D-57 peers).
11. P3 shared-file rows for `ownership.md`: `cmd/edgesync/pages.go` (claim per append), per-area catalogue file
    ownership as in the i18n table above.

---

## Item P3-F-001
Parent-only (Opus). Closed at the planning consolidation (2026-10-30): D-97 holds the GUI coverage policy
(included: `gui/state/...`, `gui/theme/tokens`, `gui/widgets/format`, `gui/shell/vm`, every `gui/<area>/vm`,
`test/e2e/harness`; excluded: `gui/app`, `gui/shell`, `gui/theme`, `gui/widgets`, `gui/widgets/guitest`,
`gui/layouteditor`, `gui/peers`, `gui/wizard`, `gui/settings`, `gui/transfers`, `gui/logs`, `gui/about`,
`gui/drop/...`, `test/e2e`). Remaining: confirm the list still matches this brief and `main`; if a package was
renamed, append a superseding decision. Append a note to `briefs/plan-phase-3.md` citing D-97.

## Item P3-F-002
Parent-only (Opus). Closed at the planning consolidation (2026-10-30) by D-93 (IPC and config additions, with the
exact P1 files: `internal/ipc/server.go`, `internal/ipc/client.go`, `internal/agent/ipc_handler.go`,
`internal/agent/ipc_peers.go`, `internal/proto/capnp/ipccp/ipc.capnp.go`, `internal/proto/msg/ipc.go`,
`internal/proto/msg/ipc_types.go`; agent options in `internal/agent/agent.go`; i18n loader `internal/i18n/i18n.go`),
D-94 (per-area catalogue files), D-95 (GUI log file), D-96 (`internal/firewall`) and the P3 rows in `ownership.md`.
Remaining: read the merged P1 code, confirm those files and APIs exist, quote every element of "Spine gaps" that P1
already provides (P1-C5 already publishes `FocusGUI` on a second GUI `Hello`; P1-F's `PairingState` enum and
`RejectReason.busy` exist) so P3-F-006 implements only what is missing, append a fix item if a name differs, and append
a note to `briefs/plan-phase-3.md`.

## Item P3-F-003
Read-only `code-reviewer` run, `model: "sonnet"`, with `qa/ui-ux-audit.md`. Input: this brief's "GUI design" and
"Conventions", and the screen sections of `p3-g1.md` .. `p3-g4.md`. Apply sections A-F to the design: every screen has
loading/empty/error/success states, keyboard path for every action, contrast pairs, hit targets, i18n coverage, flows
(wizard, pairing, layout apply/revert, transfers, logs). Output: VERDICT and FINDINGS as `<brief file>:<section> -
<severity> - <defect>`. The parent turns findings into appended notes/items before dispatching any G1-G4 item.

## Item P3-F-004
Edit `tools/coverage-exclude.txt`:
1. Read `tools/covercheck/covercheck.go` and confirm how patterns match: a pattern ending in `/...` matches the package
   and everything below; a pattern without it must match exactly one package. If exact matching is not supported,
   report BLOCKED (do not change covercheck in this item).
2. Replace the `gui/...` line by the exact excluded package list from the P3-F-001 decision, add `test/e2e` and
   `internal/firewall` (P3-F-002 decision), keep every other existing line and comment; add the comment
   `# P3-F-001: Gio rendering packages are covered by guitest tests, not by the percentage gate`.
3. `task cover` must pass on the current tree.

## Item P3-F-005
Extend `internal/i18n` (`internal/i18n/i18n.go`) so the embedded catalogue is `locales/en.toml`
plus every `locales/en/*.toml`:
- Embed pattern covers both (`//go:embed locales/en.toml locales/en/*.toml` or the equivalent for the existing embed
  variable). Files are loaded in lexical order; a key defined twice (in any two files, or twice in one) fails loading
  with an error naming both files.
- An area file may only define keys in its namespaces (table "i18n namespaces and files"); `en.toml` may not define
  `gui.*` keys. Violation fails loading.
- The existing missing/unused-key test must consider keys from all files; extend it, do not duplicate it.
- Tests use `testdata/multi/...` and `testdata/dup/...` through an `fs.FS`-taking load function (add one if the loader
  only reads the embedded FS; keep the embedded path as the production path).
- `TestLoad_EmbeddedCatalogueLoads` loads the real embedded catalogue.

## Item P3-F-006
Implement exactly the IPC and config elements the P3-F-002 decisions approved, in this order:
1. `proto/schema/ipc.capnp`: new request/event union members and fields with new ordinals only; regenerate with
   `go generate ./internal/proto/...` (needs `CAPNP`, see D-67). Never hand-edit generated code.
2. `internal/ipc`: server dispatch and client methods for the new requests/events in `gui_additions.go` (plus the
   minimal edit in the existing dispatch/client files named in the note); Go types mirror the existing style.
3. `internal/config`: `[gui]` table fields + migration (`migrate_gui.go`) bumping `schema_version` by one, with the
   "existing config with paired peers -> first_run_completed = true" rule; defaults in the schema's defaults function.
4. `internal/agent`: `envchecks.go` (`EnvChecker` interface: `Checks(ctx) ([]EnvCheck, error)`; the agent polls it
   every 30 s while a GUI is attached and on `GetEnvironmentChecks`, emits `EnvironmentChanged` only on change; the
   default checker reports `trayAvailable` from the backend's tray state and `firewallInbound` as `unsupported` until
   G5-008 adds the real one), `gui_requests.go` (`RemoveManualPeer` via `internal/discovery`'s manual-peer API,
   `primaryGUI` tracking per attached GUI connection, `RequestFocusGUI` forwarding, `ThemeChanged` after Hello).
Tests as named in the item; IPC tests run over the host-local transport with a test-only endpoint name (the
`internal/ipc` test pattern from C5). Run `task gen:check` and `task fuzz:smoke`.

## Item P3-F-007
`gui/theme/tokens` (pure, stdlib only).
- `tokens.go`: `type RGB struct{ R, G, B uint8 }`, `type Palette struct{ Bg, Surface, SurfaceAlt, Divider,
  ControlBorder, Text, TextMuted, Primary, OnPrimary, Selection, Success, Warning, WarningBg, Danger, DangerBg, InfoBg,
  MachineFill, MonitorFill, GuideLine, OverlapLine RGB }`, `func Light() Palette`, `func Dark() Palette` with the
  exact values from the token table; `func Pairs() []Pair` returning every required pair with its minimum ratio
  (4.5 or 3.0) and a name.
- `contrast.go`: WCAG 2.x relative luminance (sRGB linearisation with the 0.04045 threshold) and
  `func Contrast(a, b RGB) float64` (symmetric, 1.0..21.0).
- Tests: known pairs (black/white = 21.0 within 0.01; equal colours = 1.0), symmetry, every pair of `Pairs()` meets its
  minimum in both palettes, state colours (Success, Warning, Danger, Primary) pairwise distinct, both palettes have no
  zero-value token.

## Item P3-F-008
`gui/widgets/format` (pure; imports stdlib and `internal/i18n` only). Creates `gui_shell.toml` with the
`gui.common.*` keys it uses.
- `Bytes(n int64) string`: binary units, one decimal from KiB up: "0 B", "1023 B", "1.0 KiB", "1.5 MiB", "4.0 GiB";
  negative -> "-" (unknown).
- `Speed(bytesPerSec float64) string`: `<Bytes>/s`; 0 or NaN -> "-".
- `ETA(d time.Duration, known bool) string`: unknown -> "-"; < 60 s "less than a minute"; < 60 min "about N min";
  else "about H h M min".
- `Duration(d)`: "1.2 s", "45 s", "3 min 05 s", "1 h 02 min". `Countdown(d)`: "m:ss" (e.g. "1:42", "0:05").
- `Timestamp(t time.Time) string`: local time `2006-01-02 15:04:05`; `RelativeTime(now, t)`: "just now", "N min
  ago", "N h ago", "yesterday", else date.
- `validate.go`: `ValidateHost(s string) error` (trimmed; IPv4, IPv6 literal with or without brackets, or RFC 1123
  hostname <= 253 chars with labels <= 63; rejects empty, spaces, scheme `://`, path, port suffix), `ValidatePort(s
  string) (uint16, error)` (1..65535 digits only), `ValidateMachineName(s string) error` (trimmed, 1..64 runes, no
  control characters). Errors are sentinel errors (`ErrHostEmpty`, `ErrHostInvalid`, `ErrPortInvalid`,
  `ErrNameEmpty`, `ErrNameTooLong`, `ErrNameControl`) so view models map them to catalogue messages.

## Item P3-F-009
`gui/state` client side.
- `client.go`: `type Client interface` with one method per IPC request of architecture section 7 plus the P3-F-006
  additions (names as in section 7, context first, payload types exported by `internal/ipc`; do not redefine
  `internal/ipc` types), plus `Events() <-chan Event` and `Close() error`. `type Event` is the `internal/ipc` event
  type. `type Doer interface{ Do(name string, fn func(ctx context.Context, c Client) error) OpID; DoTimeout(name
  string, d time.Duration, fn func(ctx context.Context, c Client) error) OpID }`.
- `ipcadapter.go`: `Dial(ctx context.Context, opts DialOptions) (Client, HelloInfo, error)` using the `internal/ipc`
  client (endpoint, token file and Hello exactly as C5 implemented them); `HelloInfo{AgentVersion string; Primary bool;
  ProtocolOK bool}`; maps `ErrorInfo` to wrapped errors with sentinel kinds `ErrAgentNotRunning`, `ErrAuth`,
  `ErrVersionMismatch`, `ErrAgent` (with the agent message).
- `statetest/fake.go`: `type Fake struct` implementing `Client` with scriptable results per method
  (`Fake.On("ListPeers", result, err)`), a call log (`Fake.Calls()`), `Fake.Emit(ev Event)`; `statetest/fixtures.go`:
  `Typical()`, `Empty()`, `Errored()` returning complete `state.Snapshot` values (all slices loaded, one server with 3
  paired peers incl. one busy and one offline, 2 discovered, 4 transfers in different states, 50 log records of all
  levels, permissions for macOS-like kinds, env checks incl. one problem, hotkeys defaults, update available) and
  `FakeDoer` recording `Do` calls.
- Adapter tests run an in-process `internal/ipc` server with a test endpoint (the C5 test pattern) and a stub handler.

## Item P3-F-010
`gui/state` store.
- `snapshot.go`: `type ConnState uint8` (`ConnConnecting`, `ConnStartingAgent`, `ConnConnected`, `ConnDisconnected`,
  `ConnVersionMismatch`, `ConnAuthFailed`); `type Slice uint8` (`SliceStatus`, `SliceConfig`, `SliceLayout`,
  `SlicePeers`, `SliceDiscovered`, `SliceTransfers`, `SliceLogs`, `SlicePermissions`, `SliceEnv`, `SliceHotkeys`);
  `type Snapshot struct` with `Conn`, `ConnErr`, `AgentVersion`, `GUIVersion`, `Theme`, the payload of every slice,
  `Update` (latest update info or nil), `Pairing` (latest pairing progress or nil), `Loaded [sliceCount]bool`,
  `LoadErr [sliceCount]error`, `Ops map[OpID]Op` (read-only), `Seq uint64`. Slices are never mutated in place: an
  update builds a new slice value (copy-on-write), so `Snapshot()` returns a cheap value copy that is safe to read on
  the frame goroutine.
- `store.go`: `type Options struct{ Dial func(ctx) (Client, HelloInfo, error); Ensure func(ctx) error (agent
  launcher, P3-F-011); Logger *slog.Logger; GUIVersion string; FocusRoute string }`; `New(Options) *Store`;
  `Run(ctx) error` (connection loop: Ensure -> Dial -> if `!Primary`: `RequestFocusGUI(FocusRoute)` then return
  `ErrSecondaryInstance` -> initial sync: request every slice concurrently, record per-slice errors -> apply events
  until the event channel closes -> Disconnected -> backoff 250 ms .. 5 s, factor 2, full jitter -> retry);
  `Snapshot() Snapshot`; `SetInvalidate(func())`; `RetryNow()`; `Reload(Slice) OpID`; `Close() error`.
- Event application: an event carrying a full value replaces the slice; an event that only signals change triggers a
  reload of that slice; `LogRecord` appends to a 5000-entry ring (oldest dropped); `TransferProgress` updates one row
  by ID; `ThemeChanged` sets `Theme`; `UpdateAvailable` sets `Update`; `PairingProgress` sets `Pairing`;
  `FocusGUI` is delivered through `Options.OnFocus func(route string)`.
- `ops.go`: `type OpID uint64`, `type OpState uint8` (`OpPending`, `OpDone`, `OpFailed`), `type Op struct{ Name
  string; Started time.Time; State OpState; Err error }`; `Do` runs `fn` in a goroutine with a 30 s default timeout
  (`DoTimeout` for others), records the result, invalidates, prunes finished ops 30 s after completion. Op names are
  constant strings chosen by callers and never contain user data.
- Logging: connection state changes and op failures (name + error kind) at Info/Warn; nothing else.

## Item P3-F-011
`gui/state/agentproc`.
- `AgentPath() (string, error)`: `filepath.Join(filepath.Dir(os.Executable()), "edgesync-agent"+suffix)` with
  `.exe` on Windows; resolves symlinks of the executable; never consults PATH; missing file -> `ErrAgentMissing`.
- `type Starter func(path string) error`; production starter builds an `exec.Cmd` with no stdio, detached:
  Windows `SysProcAttr{CreationFlags: windows.CREATE_NEW_PROCESS_GROUP | windows.DETACHED_PROCESS, HideWindow: true}`;
  unix `SysProcAttr{Setsid: true}`; `cmd.Start()` then `cmd.Process.Release()`. A command-builder function returns the
  `*exec.Cmd` without starting it, so tests assert its fields.
- `Ensure(ctx, probe func(ctx) error, start Starter, path string) error`: if `probe` succeeds, return nil; if it fails
  with "endpoint missing", start once and poll `probe` every 100 ms for up to 5 s (D-77, PB-20); other errors are
  returned as they are. Launch errors are wrapped with the path.

## Item P3-F-012
`gui/theme`: `type Theme struct{ Material *material.Theme; Palette tokens.Palette; Mode Mode }` (`ModeLight`,
`ModeDark`); `New(mode Mode) *Theme` builds a `material.Theme` (`material.NewTheme()`, shaper with
`text.NewShaper(text.WithCollection(gofont.Collection()))`, `Palette.Fg/Bg/ContrastBg/ContrastFg` from tokens,
`TextSize` 14 sp); `(*Theme).SetMode(Mode)` swaps colours and keeps the shaper; helpers `Color(tokens RGB)
color.NRGBA`, typography helpers `Title`, `Section`, `Body`, `Small`, `Mono` returning `material.LabelStyle` with the
sizes from "Typography". Add `gioui.org v0.10.3` and `gioui.org/x v0.10.3` to `go.mod` (claim), `go mod tidy`; report
the first line of both LICENSE files.

## Item P3-F-013
`gui/widgets/guitest` (Gio, no `gioui.org/app`).
- `type H struct` created by `New(t testing.TB, size image.Point, scale float32)`; `H.Frame(fn func(gtx
  layout.Context))` builds a `layout.Context{Ops, Constraints: layout.Exact(size), Metric: unit.Metric{PxPerDp: scale,
  PxPerSp: scale}, Now: <synctest-friendly time.Now()>, Source: router.Source()}`, runs `fn`, calls `router.Frame(ops)`.
- Input: `Click(label string)` (finds the semantic node by label, queues press+release at its centre, then a frame),
  `ClickAt(pt)`, `Drag(from, to f32.Point, button pointer.Buttons, mods key.Modifiers, steps int)`,
  `Scroll(at, delta f32.Point, mods)`, `Key(name key.Name, mods key.Modifiers)`, `Type(text string)` (key.EditEvent),
  `Tab()`, `ShiftTab()`.
- Queries: `Nodes() []input.SemanticNode` (flattened), `Find(label) (input.SemanticNode, bool)`, `MustFind`,
  `Clipboard() (mime string, data []byte, ok bool)` via `router.WriteClipboard`.
- `AssertHitTargets(t, h)`: every node with the click gesture has bounds >= 24 x 24 dp at the harness scale.
- Self-tests use a small test widget defined in the test file.

## Item P3-F-014
Feedback widgets in `gui/widgets` (keys `gui.common.*`/`gui.shell.*` added to `gui_shell.toml` as used):
`Icon(kind IconKind)` drawing with `clip.Path` in a square of the requested dp size (kinds listed in "Widget
catalogue"); `Banner{Kind, Text, Actions []BannerAction, Dismissible}`; `EmptyState`, `LoadingState` (spinner rotates,
requests the next frame with `gtx.Execute(op.InvalidateCmd{At: ...})`), `ErrorState{Title, Detail, OnRetry}`;
`Badge{Kind}` with text and shape per kind (Online filled circle, Offline hollow circle, Active triangle, Home house,
Connecting dotted circle, Busy square, Warning exclamation triangle); `ProgressBar(value float32)` clamped to 0..1 with
the semantic description "<n> %". Every interactive part has semantic labels.

## Item P3-F-015
Input widgets in `gui/widgets`: `Button{Kind: Primary|Secondary|Danger|Text, Label, Icon, Disabled, Pending}` (Enter
and Space activate when focused; pending shows a spinner after 300 ms and ignores input); `Toggle`; `TextField`
(wraps `widget.Editor`, single line, `Error` text below in Danger colour, commits on Enter and on focus loss through
`OnCommit`); `NumberField{Min, Max, Step}` (digits only, clamps on commit, arrow up/down step); `SelectBox` (opens a
list; arrows + Enter choose; Escape closes); `RadioGroup` (arrow keys move); `Tabs` (headers, Shortcut+Tab cycling);
`Dialog` (modal surface, scrim, focus trapped inside, Escape closes, default action on Enter) and
`ConfirmDialog{Title, Text, ConfirmLabel, Danger bool}`; `FocusRing(gtx, rect)`; `pickers.go`: `type Pickers
interface{ ChooseFiles(ctx) ([]string, error); ChooseFolder(ctx) (string, error) }` and `ErrPickerCancelled`,
`ErrPickerUnavailable`.

## Item P3-F-016
`gui/shell/vm` (pure).
- `routes.go`: `type Route string` and the constants of the routes table; `SidebarRoutes() []Route` in order;
  `Hidden(Route) bool`; `TitleKey(Route) string`; `ParseRoute(s) (Route, error)`.
- `nav.go`: `type Intent struct{ Route Route; Params map[string]string }`; `type Nav struct` with `Go(Intent)`,
  `Current() Route`, `TakeParams(route) map[string]string` (consumed once), `Back()` (returns from a hidden route to
  the previous sidebar route); default route by role.
- Gate: `func GateRoute(snap state.Snapshot) (Route, bool)` -> `wizard`, true while `first_run_completed` is false.
- `banners.go`: `Banners(snap, dismissed map[string]bool) []BannerSpec` in the priority order of "Banners"; transient
  banners with 4 s expiry (`Push(text, kind)`, `Expire(now)`).
- `status.go`: status-bar texts per role/state, lock indicator, active transfer count (Active + Queued).
- `shortcuts.go`: Shortcut+1..6 -> sidebar routes.

## Item P3-F-017
`gui/shell` (Gio).
- `page.go`: `type Page interface{ Route() vm.Route; Layout(gtx layout.Context, env *Env) layout.Dimensions }`,
  `type PageFactory func(env *Env) Page`, `type Env struct{ Theme *theme.Theme; Store *state.Store; Doer state.Doer;
  Nav *vm.Nav; Pickers widgets.Pickers; OpenURL func(string) error; Modals *ModalHost; Options AppOptions }`,
  `type AppOptions struct{ SyntheticMachines int; PerfLog bool }`, and the optional interface `type Observer
  interface{ Observe(snap state.Snapshot, env *Env) }`: before laying out each frame, the shell calls `Observe` on every
  registered page that implements it (current or not), so a page can react to events app-wide (for example G2 opens the
  responder pairing dialog through `Env.Modals` on any page). `Observe` must not draw.
- `shell.go`: lays out sidebar, banners, page/app-wide state, status bar, modal host; renders the wizard route full
  window when the gate is active; app-wide states per "App-wide states".
- `sidebar.go`: registered sidebar routes only, icon + label, selected state, Shortcut+N.
- `statusbar.go`, `modal.go` (`ModalHost.Open(m Modal)`, `Close()`, Escape handling, focus trap).
- Tests use `guitest` with `statetest` fixtures and two test pages defined in the test file.

## Item P3-F-018
- `gui/app/app.go`: `Run(opts Options, pages []shell.PageFactory) error`: create `app.Window` (title, size, min size),
  `explorer.NewExplorer(w)`, theme, store (`state.New` with the IPC adapter's `Dial`, `agentproc.Ensure`, `OnFocus` ->
  `w.Perform(system.ActionRaise)` + `Nav.Go`), shell; event loop: `e := w.Event(); expl.ListenEvents(e)`; on
  `app.FrameEvent`: `gtx := app.NewContext(&ops, e)`, shell layout, `e.Frame(gtx.Ops)`, perf sample; on
  `app.DestroyEvent`: close store, return `e.Err`. Store `Run` in a goroutine; `ErrSecondaryInstance` exits 0 without
  opening a window frame (log line). First frame logs `first frame` with `since_start_ms` (PB-21). With `PerfLog`, log
  frame stats every second (`gui/state/perfstats`: rolling one-second window, p50/p99/max of frame build durations).
- `gui/app/pickers.go`: `Pickers` over explorer; `ChooseFiles` on darwin uses `ChooseFile` (one file per call, because
  `ChooseFiles` returns `ErrNotAvailable` there); returned readers must be `*os.File` (path = `Name()`, closed
  immediately), anything else -> error "picker returned no file path"; `ErrUserDecline` -> `ErrPickerCancelled`;
  `ErrNotAvailable` -> `ErrPickerUnavailable`. The path extraction is a pure helper with tests.
- `gui/app/openurl.go`: `ValidateURL(s) error` (https only, host required); `openurl_windows.go`
  (`windows.ShellExecute` with verb "open"), `openurl_darwin.go` (`exec.Command("open", url)`), `openurl_linux.go`
  (`exec.Command("xdg-open", url)`); always validate first.
- `cmd/edgesync/flags.go`: `-page`, `-perflog`, `-synthetic-layout` (0..64), `-log-level` (debug|info|warn|error);
  pure `parseFlags(args []string) (Options, error)`. `main.go`: logging to the GUI log file (P3-F-002 decision) via
  `internal/logging`, `app.Run(opts, pages())`, `os.Exit` codes 0/1. `pages.go`: `func pages() []shell.PageFactory`
  returning the registered page factories (empty list until G items append; each G registration item appends one
  line). `pages_test.go`: no duplicate routes among the factories (built with a fixture `Env`).

## Item P3-F-019
`gui/state/purity_test.go`: walk the module's `gui/` tree with `go/parser` (imports only):
- packages matching the pure set (`gui/state`, `gui/state/...`, `gui/theme/tokens`, `gui/widgets/format`,
  `gui/shell/vm`, `gui/*/vm`) import only the standard library, `github.com/kikz0r/edgesync/internal/...`, and other
  pure GUI packages;
- no package other than `gui/app` and `cmd/edgesync` imports `gioui.org/app` or `gioui.org/x/explorer`;
- the pure set equals the "included" GUI patterns of the P3-F-001 decision (hard-coded list in the test, comment
  referencing the decision number).

## Item P3-F-020
`.github/workflows/ci.yml` (claim): read https://gioui.org/doc/install/linux at execution time and add an
`apt-get install` step with exactly the listed development packages (plus `pkg-config` and `gcc` if listed) to the
ubuntu `lint`, `test`, and `gen-check` jobs (any ubuntu job that compiles `./...`). Confirm from the runner-images
README that macOS runners have the Xcode command line tools; report the list. actionlint clean.

## Item P3-F-021
`cmd/edgesync/screens_integration_test.go`, build tags `integration && windows`. For each page factory from `pages()`
and each fixture (`statetest.Typical`, `Empty`, `Errored`), theme (light, dark) and scale (1.0, 1.5, 2.0): create
`headless.NewWindow(1000*scale, 700*scale)`, lay out the shell with the page selected and a fake store snapshot, `Frame`,
`Screenshot`, write PNG `<route>-<fixture>-<theme>-<scale>.png`. Also the shell states connecting, disconnected,
version mismatch, and the wizard gate. Output dir `$EDGESYNC_SCREENS_OUT` or `bin/screens/<UTC yyyymmdd-hhmmss>/`;
log the directory. Fail when rendering panics or an image has a single colour.

## Item P3-F-022
`qa/manual/p3-f-gui-shell.md` per `qa/manual/README.md`. Machines: Windows PC 1. Steps: build `go build
-ldflags=-H=windowsgui -o bin/edgesync.exe ./cmd/edgesync` and `go build -ldflags=-H=windowsgui -o
bin/edgesync-agent.exe ./cmd/edgesync-agent` (same folder); launch the GUI with no agent running (agent auto-start,
time to first frame from `edgesync-gui.log`, PB-21 <= 1 s); quit the agent from the tray (disconnected state, Retry,
Start agent); launch a second GUI (first one raised, second exits); switch Windows light/dark (live theme change);
DPI 100/125/150/200 % (Settings > Display > Scale); keyboard-only navigation (Tab order, Shortcut+1..6, Escape);
run the screenshot harness `go test -tags integration -run TestScreens ./cmd/edgesync/` and record the output folder.
Safety: how to stop the agent (tray Quit or `taskkill /IM edgesync-agent.exe`).

## Item P3-F-M01
User-owned. Results into `qa/manual/results/p3-f-gui-shell.md`; zip or folder path of the screenshots recorded so the
UI/UX audit can read them.

## Item P3-F-R01
`code-reviewer`, `model: "sonnet"`, over the merged P3-F diff (P3-F-004 .. P3-F-022). Extra points: store
concurrency (copy-on-write, no data race, goroutines stopped on Close), purity boundaries, no blocking call on the frame
goroutine, explorer `ListenEvents` called for every event.

## Item P3-F-R02
`code-reviewer`, `model: "sonnet"`, with `qa/security-review.md` sections 3, 5, 7, 8. Extra points: the session token
is read only from the owner-only token file and never logged; the agent is launched only from the sibling path (no
PATH lookup, no shell, fixed arguments); `OpenURL` validates before calling the OS; pairing data and clipboard content
never appear in GUI logs; IPC additions keep decode limits.

## Item P3-F-R03
`code-reviewer`, `model: "sonnet"`, with `qa/ui-ux-audit.md` sections A-D and the banner/state parts, reading the
shell, widgets and theme code plus the PNGs from P3-F-M01 (paths in its results file).

## Item P3-F-R04
Parent-only (Opus) Fork: cross-check P3-F against D-19, D-33, D-52, D-68, D-77 and the P3-F decisions; confirm the
IPC additions match the decision exactly; append the stream-green note.

## Item P3-X-001
Parent-only (Opus) Fork: list every FEAT whose "Delivered by" or "Also" names P3 and check its items are `[x]`; check
cross-stream consistency (routes and intents used by G1-G4 match the shell; no area imports another area); confirm
perf rows (PB-01 loopback, PB-08 loopback, PB-15, PB-17 idle, PB-18, PB-19, PB-20, PB-21) are in `perf-budgets.md`;
run `task ci:local`.

## Item P3-X-002
User-owned: run `qa/smoke-tests.md` on the P3 build (binaries from `go build`, launched by the user); S-11 and S-12
are n/a until P4. Results into `qa/manual/results/smoke-p3-<date>.md`.

## Item P3-X-003
Parent-only. Verify the P3 exit criteria in `plan.md` section 3. Append the marker note listing every phase still
active (for example only P4, or P4 and P5 when R streams already run as fill):
```
<!-- NOTE: <date> - P3 exit criteria met.
Current phase: P4
-->
```
Mirror the marker in `CLAUDE.md` in the same commit.
