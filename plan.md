# EdgeSync - Build plan

## CONCURRENCY CONTRACT (binding - re-read at the start of every session and after every compaction)

Current phase: P0
Active concurrency (this phase): 5

| Phase | Parallel subagents (N) | Streams (owner -> scope) | Foundation block (sequential, runs first) |
|-------|------------------------|--------------------------|--------------------------------------------|
| P0 spikes & toolchain | 5 | SP1 Windows input + DPI + service/secure desktop/SendSAS; SP2 Windows COM shell (virtual files, delayed rendering, OLE DnD); SP3 Linux (Wayland portals + EI codec, X11 xgb, Linux file paste + DnD, GNOME overlay); SP4 macOS via purego (compile + CI only); SP5 core tech (Gio editor, quic-go bench, mDNS, capnp, SPAKE2, IPC) | P0-F items P0-F-001..P0-F-010 (1 agent, in order): toolchain installs (USER-CONFIRM), repo scaffold, spikes module, tools module, lint config, covercheck, Taskfile, CI |
| P1 contracts & core | 5 | C1 protocol + transport + session; C2 pki + spake2 + pairing + peers + discovery + wol; C3 layout + engine + keymap; C4 clipboard core + transfer core; C5 agent shell (ipc, config, logging, i18n, update, locksync, diag, paths, agent wiring on fakes) | P1-F (1 agent, in order), ends with CONTRACT FREEZE: platform interfaces + fakes + conformance suites, keymap types, capnp schemas + generated code, hot-path codec, IPC schema, config schema, logging/error conventions, buildinfo |
| P2 platform backends | 5 | W1 Windows input/displays/hotkeys/session/tray/overlay/autostart/theme/resources; W2 Windows clipboard + SYSTEM service + secure helper + SendSAS + pre-login; X1 Linux X11 + shared Linux (SNI tray, XDG autostart, theme, session); Y1 Linux Wayland GNOME/KDE (portals + EI, clipboard portal, indicator); M1 macOS (input, clipboard, tray, overlay, permissions, LaunchAgent, theme) | none (requires the P1-F freeze; each item lists the P1 items it needs) |
| P3 GUI & integration | 5 | G1 layout editor; G2 machines + pairing + first-run wizard; G3 settings (general, guards, hotkeys, remap, network); G4 transfers + logs + diagnostics + about + send-files; G5 agent end-to-end integration on real backends, tray actions, overlay, lock sync, WoL, reconnect, e2e tests | P3-F (1 agent, in order): Gio app skeleton, theme light/dark, navigation, IPC client binding, i18n wiring, single instance |
| P4 native files & DnD | 5 | F1 Windows virtual-file paste; F2 Windows cross-screen DnD + GUI file drop; F3 macOS file paste + DnD + GUI drop; F4 Linux X11 file paste + XDND + GUI drop; F5 Linux Wayland file paste + DnD + GUI drop | P4-F (1 agent, in order), ends with FILE CONTRACT FREEZE: vfile provider contract, platform FileClipboard/DragDrop interfaces, transfer-engine hooks, dnd protocol, staging |
| P5 packaging, hardening & release | 5 | R1 Windows installer (Inno Setup 7.1.1) + service install + firewall + autostart + SAS policy; R2 macOS .app zip + Linux tar.gz + release CI (GoReleaser OSS); R3 perf benches + tuning vs perf-budgets.md; R4 security hardening (fuzz campaigns, pairing/IPC/service/path audits); R5 QA matrix + docs + spikes removal | none |

Rules:
- While a phase is active and has >= N unblocked open items, exactly N subagents must be in flight. Fewer than N in flight is a bug. Refill immediately: when a stream's agent returns and is verified, dispatch that stream's next item (or the next unblocked item of the phase) so the count returns to N. Never let the fleet idle down to 1 because "the next item was convenient to do".
- The ONLY times fewer than N run: (a) a foundation block (P0-F, P1-F, P3-F, P4-F), which is intentionally sequential (1 agent) until green; (b) fewer than N unblocked open items remain in all active phases; (c) the user explicitly tells you to reduce concurrency.
- Global fleet cap is 5 across all active phases. When the active phase has fewer than 5 unblocked open items, fill from the next phase whose dependencies are met, and append a new marker line listing every active phase (`Current phase: P1, P2`) inside a dated `<!-- NOTE: ... -->` block. The last marker line in this file wins.
- Items owned by `USER` or `USER-CONFIRM` and items owned by `PARENT` do not count toward N; they are surfaced to the user (or done by the parent) without blocking dispatch of other unblocked items.
- N is a property of the PHASE, not of how much context you have left. Compaction does not lower N.

---

## 1. Orchestration model (how this build is run)

### 1.1 Roles
- **Parent (Opus, the orchestrator)** - reads this file first, writes and refines briefs, dispatches subagents, verifies every diff, re-runs gates, merges worktree branches into `main`, toggles checkboxes, appends notes, asks the user for decisions and manual verification, runs the Opus consolidation review per stream. The parent does NOT write implementation or test files for any tagged item. If the parent catches itself opening Edit/Write on an item's implementation files: STOP and dispatch.
- **Subagents** - implement exactly one briefed item each, commit it on their worktree branch, report in the fixed format, never edit plan files.
- **User** - answers decision questions, confirms installers, runs spike binaries and manual checklists on their machines (2+ Windows PCs, one GNOME Wayland box), pushes to GitHub when they choose (CI runs macOS and X11 verification).

### 1.2 Phases, streams, blocking order
See the contract table. Order: P0-F -> P0 spikes (5) -> P0 exit (consolidation + user approval of any deviation) -> P1-F -> CONTRACT FREEZE -> C1..C5 (5) -> P2 W1/W2/X1/Y1/M1 fill in as P1 items finish -> P3-F (needs P1 C5) -> G1..G5 -> P4-F (needs P1 C4 + P0 SP2/SP3/SP4 findings) -> F1..F5 -> P5 R1..R5 (R3/R4 may start as fill once P3 G5 has e2e tests). File ownership per stream: `ownership.md`. Architecture and frozen contracts: `briefs/architecture.md`. Decisions: `decisions.md`. Feature IDs: `briefs/feature-inventory.md`.

### 1.3 Dispatch ceremony (per item)
1. Read the item in `briefs/plan-phase-<N>.md` and its stream brief section (`[brief:...]`). Confirm its `Needs:` items are `[x]`.
2. Dispatch a fresh subagent with `subagent_type: "go-dev"`, `isolation: "worktree"`, and `model:` set explicitly to the item's tier (`"haiku"` by default, `"sonnet"` only when the item is tagged `[tier:sonnet why:...]`). Never omit `model:`. The prompt contains: the item text verbatim (ID, Files, Tests, DoD), the stream brief path + anchor, the decisions it relies on, "touch only the listed files", the commit line, and "report in the go-dev report format".
3. Research, audits and cross-file lookups the parent needs go to a Fork (no `subagent_type`), so raw output stays out of the parent's context.
4. Keep N in flight (contract). Batch same-shape mechanical items of one stream into one dispatch when their files do not collide (B7.3).
5. On return: read the diff on the worktree branch, run the item's tests and the per-item gates (1.5), check DoD line by line, check that no file outside `Files:` changed (except `go.mod`/`go.sum` with a claim, see `ownership.md`). Then `git rebase main <branch>` and `git merge --ff-only <branch>` on `main`; toggle the checkbox; commit `chore(plan): complete <item-id> [FEAT-NNN]`.
6. If verification fails: re-brief at the same tier with the exact defect (an underwritten brief is the usual cause). Escalate haiku -> sonnet only after a verified failed attempt, and say in the new brief what the previous tier got wrong. Never skip a rung.

### 1.4 Tier semantics (binding; the `[tier:...]` tag is an instruction)
- **haiku (default)** - execution from a precise brief: the hot-path codec from the frozen table, keymap generator runs and table checks, config schema fields and migrations, i18n catalogue entries, Taskfile tasks, CI YAML, fakes implementing a frozen interface, the path sanitiser from D-45's rule list, Win32/AppKit declaration files from a named API list, Inno Setup script/Info.plist/.desktop files from a spec, GUI pages from a described layout, test tables, benchmarks from a named bench definition.
- **sonnet (must carry `why:`)** - genuinely open design or concurrency inside the item: SPAKE2 per RFC 9382, hand-built COM vtables and STA threading, the EI protocol state machine, portal request/response flows, the LL-hook thread + watchdog, the engine guard state machine, transport fallback + priority writer, service token/session handling, the Gio layout-editor interaction model, spike findings synthesis, and the code-reviewer / security-review gates.
- **opus** - the parent only: planning, decomposition, contract design, spike consolidation, per-stream consolidation review. Never an item owner except `[owner:PARENT]` review items.
- `model:` is ALWAYS passed explicitly on every Agent call. Omitting it inherits Opus and breaks the cost model.

### 1.5 Verification cadence and exact gates
Shell: the Bash tool (Git Bash). Taskfile equivalents exist for every gate once P0-F-007 is done.

Per item (subagent before reporting, parent again before merging):
- `golangci-lint fmt` then `golangci-lint run ./...` (formatters gofumpt + goimports run inside it)
- `go vet ./...`
- `go build ./...`
- `go test ./<affected packages>/...`
- Cross-compile check for agent-side code: `GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build ./internal/... ./cmd/edgesync-agent/...` and `GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build ./internal/... ./cmd/edgesync-agent/...`
- Items touching `.github/workflows/`: `go tool -modfile=tools/tools.mod actionlint`

Per batch (about every 5 merged items, run by a `gate-runner` subagent with `model: "haiku"`), `task ci:local`, which runs:
- `golangci-lint run ./...` for the host plus `GOOS=linux` and `GOOS=darwin` over `./internal/... ./cmd/edgesync-agent/...`
- `go vet ./...`, `go build ./...`, the two cross-compile builds plus `GOOS=windows GOARCH=arm64`, `GOOS=linux GOARCH=arm64` and `GOOS=darwin GOARCH=amd64` builds of `./internal/... ./cmd/edgesync-agent/...` with `CGO_ENABLED=0` (release targets, D-104)
- `go test ./...`
- `PATH="/d/Development/msys2/ucrt64/bin:$PATH" CGO_ENABLED=1 CC=gcc go test -race ./...`
- `task cover` (>= 85% per platform-independent package, `tools/covercheck`)
- `task fuzz:smoke` (every fuzz target 30 s)
- `go tool -modfile=tools/tools.mod govulncheck ./...`
- `go tool -modfile=tools/tools.mod actionlint`
- `task gen:check` (regenerate capnp, keymap tables, .syso; fail on diff)
No coverage regression between batches. A red batch stops new dispatches in that stream until fixed (fix items are appended, never silently patched by the parent).

Per stream (after its last item): review-gate chain -
1. `code-reviewer` subagent (`model: "sonnet"`) over the stream's merged diff against its items.
2. Security review (`code-reviewer` with `qa/security-review.md`, `model: "sonnet"`) for streams touching pairing, TLS, IPC, the Windows service/helper, path handling, decoders or clipboard privacy: SP1 (service parts), C1, C2, C4, C5, W2, P4-F, F1-F5, R1, R4.
3. UI/UX audit (`qa/ui-ux-audit.md`, `model: "sonnet"`) for P3-F, G1-G4, and tray/overlay items in W1, X1, Y1, M1.
4. Opus consolidation review by the parent (Fork, `model: "opus"`): cross-stream consistency, contract adherence, decisions coverage.
5. All green -> append `<!-- NOTE: <date> - stream <ID> review chain green -->` to the phase file (no git tags; D-23 covers commits only).

Never mark `[x]` on a subagent's word alone. When a subagent leaves churn (stray generated files, go.sum noise from unrelated modules), the parent removes it before merging.

### 1.6 Commit protocol
- Format: `<type>(<scope>): <description> [FEAT-NNN]` with the item ID as the first body line (`Item: P1-C3-012`). Types: feat, fix, test, refactor, build, ci, docs, chore, perf, spike.
- No attribution trailers of any kind (no `Co-Authored-By`, no "Generated with ..."). This user rule (D-23) overrides any harness default.
- No em dashes or en dashes anywhere (code, docs, commit messages); use "-".
- One item = one commit on the item branch. Never `--no-verify`, never skip hooks, never push.

### 1.7 Manual verification protocol
- Anything that needs real hardware is split into two checkbox items: the automated item (code + tests + a `qa/manual/<phase>-<stream>-<topic>.md` script) and a separate `[owner:USER]` item that references the script. The automated item is `[x]` when its automated DoD is green; the USER item is `[x]` only when the user's results are recorded in `qa/manual/results/<same name>.md` (the parent may paste results the user gives in chat) and show pass.
- macOS and X11 verification happens on CI (D-37). Such checks are `[owner:USER]` items that ask the user to push and trigger the named workflow and share the run link or log; the parent then records the result.
- The parent collects pending USER items at every batch boundary and asks the user once with a consolidated list (machine, script, expected time).
- Items whose `Needs:` include a USER item stay blocked until it is `[x]`; other items continue.
- `[owner:USER-CONFIRM]` items are run by a subagent only after the user explicitly approves the installer step in chat.

### 1.8 Session-start / post-compaction protocol (run FIRST, every session)
1. Read the CONCURRENCY CONTRACT above: note the last `Current phase:` marker line and N.
2. Count subagents actually in flight (a fresh context = zero).
3. If an active phase is past its foundation block and has >= N unblocked open items, dispatch up to N subagents immediately, one per stream where possible. Resuming with one agent is a contract violation.
4. Only then continue verification, merges and plan updates.

---

## 2. How to read an item

```
- [ ] [P1-C3-012] Dwell guard: contact must persist 150 ms before switching [FEAT-011] [tier:sonnet why:timer-driven guard state machine] [owner:C3] [brief:briefs/streams/p1-c3.md#item-p1-c3-012]
  Needs: P1-F-014, P1-C3-008
  Files: internal/engine/guard_dwell.go, internal/engine/guard_dwell_test.go
  Tests: TestDwell_SwitchesAfterDelay, TestDwell_CancelsWhenPointerLeaves, TestDwell_DisabledSwitchesImmediately (synctest)
  DoD: tests green under synctest; 0 allocs/op in BenchmarkDwellEvaluate; coverage of internal/engine >= 85%; per-item gates green; commit `feat(engine): dwell guard [FEAT-011]`
```

- ID scheme: `[P<phase>-<stream>-NNN]` (`P0-F-001`, `P0-SP1-003`, `P1-C3-012`, `P2-W1-004`, `P3-G1-007`, `P4-F2-003`, `P5-R4-002`); manual items use `M` numbers (`P0-SP1-M01`).
- Tags: `[FEAT-NNN]` (one or more), `[tier:haiku]` default or `[tier:sonnet why:<reason>]`, `[owner:<stream>|USER|USER-CONFIRM|PARENT]`, `[brief:briefs/streams/p<N>-<stream>.md#item-<id-lowercase>]`.
- Lines: `Needs:` (blocking items), `Files:` (exact files created or edited, nothing else), `Tests:` (named test cases), `DoD:` (objective done criteria + commit line).
- Items live in `briefs/plan-phase-<N>.md` under `## Phase <N> - <name>` with streams as `###` headings. Stream briefs live in `briefs/streams/p<N>-<stream>.md` and contain no checkboxes.

---

## 3. Phase registry

### P0 - Spikes & toolchain
- Items: `briefs/plan-phase-0.md`. Stream briefs: `briefs/streams/p0-f.md`, `p0-sp1.md` .. `p0-sp5.md`.
- Entry: plan approved by the user.
- Exit: every P0 item `[x]` (including USER items); each spike stream's `briefs/spikes/<stream>.md` gives go/no-go per mechanism with measurements; parent consolidation `briefs/spikes/p0-consolidation.md`; every deviation from a locked decision approved by the user and appended to `decisions.md`; `task ci:local` green.

### P1 - Contracts & core
- Items: `briefs/plan-phase-1.md`. Stream briefs: `briefs/streams/p1-f.md`, `p1-c1.md` .. `p1-c5.md`.
- Entry: P0 exit.
- Exit: CONTRACT FREEZE note recorded after P1-F; C1-C5 items `[x]`; >= 85% coverage on every P1 package; fuzz smoke green on every decoder; mid-build benches recorded in `perf-budgets.md` (codec, engine, transport loopback incl. input under bulk load, transfer loopback); review chains green.

### P2 - Platform backends
- Items: `briefs/plan-phase-2.md`. Stream briefs: `briefs/streams/p2-w1.md`, `p2-w2.md`, `p2-x1.md`, `p2-y1.md`, `p2-m1.md`.
- Entry: CONTRACT FREEZE (P1-F); per-item `Needs:` on P1 items.
- Exit: each backend passes the conformance suites under `integration` on its CI runner or the user's machine; manual checklists for Windows (2 PCs) and GNOME recorded; capture callback benches recorded; review chains green incl. security (W2) and UI/UX (tray/overlay).

### P3 - GUI & integration
- Items: `briefs/plan-phase-3.md`. Stream briefs: `briefs/streams/p3-f.md`, `p3-g1.md` .. `p3-g5.md`.
- Entry: P1 C5 done (for P3-F); G5 additionally needs P2 W1 + W2.
- Exit: every GUI screen implemented with i18n; e2e loopback tests green; GUI frame-time bench recorded; UI/UX audit passed; manual two-Windows-PC session checklist passed.

### P4 - Native files & DnD
- Items: `briefs/plan-phase-4.md`. Stream briefs: `briefs/streams/p4-f.md`, `p4-f1.md` .. `p4-f5.md`.
- Entry: P1 C4 done, P0 SP2/SP3/SP4 findings approved; each F stream also needs its P2 stream.
- Exit: FILE CONTRACT FREEZE note after P4-F; file paste + DnD manual checklists passed on Windows and GNOME; macOS/X11 CI integration green; security review green.

### P5 - Packaging, hardening & release
- Items: `briefs/plan-phase-5.md`. Stream briefs: `briefs/streams/p5-r1.md` .. `p5-r5.md`.
- Entry: P3 and P4 exit (R3/R4 may start earlier as fill, R1 after P2 W2).
- Exit: the Windows installer installs, upgrades and uninstalls cleanly on the user's PCs; release workflow produces all artefacts; every perf budget met or a user-approved exception recorded; all security findings fixed; docs written; spikes module removed.

---

## 4. Mid-build checkpoints (performance and compatibility)

| When | What | Who | Recorded in |
|---|---|---|---|
| During P0 SP5 | quic-go datagram latency on loopback, with and without bulk | SP5 | `briefs/spikes/sp5.md`, `perf-budgets.md` |
| During P0 SP1 | LL hook callback duration under GC stress, watchdog behaviour | SP1 + USER | `briefs/spikes/sp1.md` |
| End of P1 C1 | codec allocs/op, transport loopback latency incl. input under bulk load | C1 | `perf-budgets.md` |
| End of P1 C3 | engine per-event cost, coalescing | C3 | `perf-budgets.md` |
| End of P1 C4 | transfer loopback throughput, clipboard fetch latency | C4 | `perf-budgets.md` |
| End of P2 W1 | real hook callback timing (integration, user PC) | W1 + USER | `perf-budgets.md` |
| End of P3 G1 | layout editor frame time with 64 machines | G1 | `perf-budgets.md` |
| End of P3 G5 | e2e loopback input latency, agent idle CPU/RSS | G5 | `perf-budgets.md` |
| During P5 R3 | LAN measurements on the user's two Windows PCs against every budget | R3 + USER | `perf-budgets.md` |
| Every dependency bump | compatibility re-check, new decision entry | parent | `compatibility-matrix.md`, `decisions.md` |
| At P0-F exit, P2 exit and P5 exit | Go toolchain and pinned versions re-verified against the proxy | parent (Fork) | `compatibility-matrix.md` |

---

## 5. Mutation rules for plan files (sacred)
- Append-only. Allowed mutations: toggle `[ ]` -> `[x]` after an item is done AND verified; add `<!-- NOTE: <date> - ... -->` notes; append new items at the end of the relevant stream section (never insert, never reorder, never delete).
- A step that is impossible as written, or a better approach: STOP and ask the user ("plan says X, I hit Y, I propose Z - approve?"). No silent deviation. Never mark an item complete when its result does not match its text.
- New work discovered mid-build is appended as new items with new IDs.
- Superseded items (D-111): when an approved decision replaces an item, the parent toggles it to `[x]` and appends on the next line `<!-- NOTE: <date> - superseded by D-NN; replacement items <IDs> -->`. The statusline counts every unchecked box as open work, so a superseded item must not stay `[ ]`; it never counts as delivered work in FEAT audits (the replacement items carry the FEAT tags).
- `Needs:` may cite a decision (`D-NN`, always satisfied once it exists) or `Q-NN decision` (blocked until the parent appends the decision answering `briefs/open-questions.md` Q-NN).

---

## 6. Notes (append-only)

<!-- NOTE: 2026-10-29 - Plan spine written: decisions D-01..D-79, FEAT-001..FEAT-178, P0 items in briefs/plan-phase-0.md. P1-P5 item files are written by planning forks after the spine. -->
<!-- NOTE: 2026-10-30 - Planning consolidation: all six phase files and stream briefs written (P0 57, P1 139, P2 151, P3 116, P4 103, P5 102 items). Cross-phase prerequisites resolved to item IDs; spine gaps closed by D-80 .. D-111; product questions Q-01 .. Q-10 in briefs/open-questions.md are asked before approval; ownership, perf budgets (PB-25 .. PB-27), risks (R-52 .. R-57) and the compatibility matrix updated; new item P2-X-003 (agent entry point, D-84). -->
<!-- NOTE: 2026-10-30 - Product answers Q-01 .. Q-10 applied (user Q&A): decisions D-112 .. D-121 (Q-NN -> D-(111+NN)); the Windows installer is Inno Setup 7.1.1 and the WiX/MSI plan of D-25 and D-65 is superseded (D-118, P0-F-003 and the P5 R1 items rewritten, same IDs); new items: P1-F-023, P1-C3-026, P1-C3-027, P1-C5-027, P2-W2-036 .. P2-W2-038, P2-Y1-026, P2-Y1-027, P2-M1-028, P3-G2-011 .. P3-G2-013, P3-G3-013, P3-G5-020, P3-G5-021, P5-R1-018; concealed clipboard content is shared (D-121, sensitivity detection removed from the clipboard contract and backends); FEAT-179 .. FEAT-183 added; item counts now P0 57, P1 143, P2 157, P3 122, P4 103, P5 103. -->

<!-- NOTE: 2026-10-30 - Plan approved by the user (commit 41ce2e1). The plan is now sacred and append-only. P0 starts with the foundation block P0-F.
  P0-F-003 done by the user: Inno Setup 7.1.1 installed manually at D:\Program Files\Inno Setup 7\ (not via winget); ISCC path and checks recorded in qa/manual/results/p0-f-toolchain.md. -->

<!-- NOTE: 2026-10-30 - P0-F-007 and P0-F-008 done. P0-F-011 appended (D-122, amends D-55: deprecated gofumpt extra-rules key); P0 now has 58 items. -->

<!-- NOTE: 2026-10-30 - P0-F foundation block complete (P0-F-001..011 all [x]). task ci:local exits 0 on the tree. Verified deviations, accepted by the orchestrator: Taskfile cross/lint:cross patterns are built from directories that exist (Go exits 1 on literal patterns naming absent directories); ci.yml adds top-level permissions: contents: read and builds the windows/darwin lint package list at runtime. ci.yml and spikes.yml have not run on GitHub yet (first push). Spike fan-out (SP1..SP5, N=5) starts next. -->

<!-- NOTE: 2026-10-30 - P0-F-012 and P0-SP4-008 appended (found while merging SP1-001/SP3-001/SP4-002/SP5-001): the spikes.yml linux job lacks Gio's native build dependencies (gioeditor needs cgo on Linux), and spikes/macos/tap has no README. P0 now has 60 items. -->

<!-- NOTE: 2026-10-30 - P0-SP4-003 merged. Evidence level for the CGEvent constants in spikes/macos/inject is second-source (servo core-graphics bindings, Chromium dom_code_data.inc): the Apple SDK header URLs were unreachable for the agent. The macOS CI integration tests are the real check; SP4-007 must list these as "second-source until CI confirms". -->

<!-- NOTE: 2026-10-30 - P0-SP2-002 merged. Finding for P0-SP2-007 (findings) and P4-F1: wincom.DataSource hides the FORMATETC of QueryGetData / GetData requests for formats the object does not offer, so the spike logs a Formats line for those instead of per-call format/lindex/tymed/aspect; the contract may need widening before P4. Also: the descriptor builder rejects backslash in paths beyond the D-99 list (input paths are slash-separated). -->

<!-- NOTE: 2026-10-30 - P0-SP3-002 merged. Findings for P0-SP3-009 (findings), P2-Y1 and architecture 4.3: (1) xdg-desktop-portal 1.22.1 InputCapture v2 changed the session flow versus the brief wording: CreateSession2 (returns a{sv}, no Request) then Start (Request carrying capabilities, persist_mode, restore_token); the spike branches on the interface version property and uses the deprecated CreateSession on v1; persist_mode and restore_token exist only from v2 (v1 plus a persistence request returns ErrPersistenceUnsupported). (2) RemoteDesktop.CreateSession returns session_handle typed "s" (documented erratum) - handled alongside "o". (3) godbus return shapes and mutter behaviour (EI device resumed ordering, barrier moves on the same session) are verified from source only; the GNOME box run decides. -->

<!-- NOTE: 2026-10-30 - P0-SP4-004 merged. spikes/macos/pb/README.md was added although the item Files line omits it: the item DoD requires a README stating what is unverifiable without a Mac (Finder paste of promises). Findings for P0-SP4-007 / P2-M1 / P4-F3: the NSFilePromiseProvider completion block is created by AppKit so objc.Block.Invoke cannot call it (the spike calls the invoke pointer at offset 16, Clang Block ABI); every Provider/Promise registers its own ObjC class under a unique name because purego callbacks are never freed (budget 2000); org.nspasteboard.ConcealedType is second-source (community convention); UTI strings come from Apple archived UTI docs cross-checked with golang-design/clipboard and re-checked against AppKit symbols by a CI test; whether in-process pasteboard reads hang without a serviced run loop is the most likely CI failure. -->

<!-- NOTE: 2026-10-30 - P0-SP5-003 merged. The mDNS service type is _edgesync._udp (D-43, the plan item and the stream brief); an orchestrator dispatch prompt said _tcp by mistake and the agent correctly followed the plan item. Licences verified from the module sources: hashicorp/mdns v1.0.7 is MIT (copyright IBM Corp.), brutella/dnssd v1.2.14 MIT, miekg/dns v1.1.72 BSD-3, vishvananda/netlink and netns Apache-2.0. spikes/go.mod pins: only additions, no existing pin changed. -->

<!-- NOTE: 2026-10-30 - P0-SP2-003 merged. Open Windows details for the user run and P0-SP2-007: (1) CF_DIBV5 alpha is stored straight (as PNG); Microsoft Learn does not document CF_DIBV5 alpha handling - confirm in Paint. (2) CF_HTML StartFragment is placed after the <!--StartFragment--> marker and EndFragment before <!--EndFragment--> - Learn does not say which side; confirm in Word. (3) LCS_sRGB and LCS_GM_IMAGES numeric values come from wingdi.h convention (names verified on Learn, values not shown there). (4) The spike logs a sensitivity-format flag for observation only; per D-121 it is not a go/no-go criterion. -->

<!-- NOTE: 2026-10-30 - P0-SP3-003 merged. Findings from xdg-desktop-portal 1.22.1 (commit 1d20fadc) for P0-SP3-009, P2-Y1 and architecture 4.5: the Clipboard portal methods have no Request object (the handle_token/Response pattern does not apply); RequestClipboard must precede Start (RemoteDesktop: after SelectDevices; InputCapture v2: after CreateSession2); InputCapture v1 cannot carry the clipboard because its CreateSession starts immediately; clipboard_enabled must be true on both RemoteDesktop and InputCapture. Source-verified only; the GNOME box run decides. -->

<!-- NOTE: 2026-10-30 - P0-SP4-005 merged; P0-SP4-009 appended (README for spikes/macos/ui, cmd/trayoverlay and bundle; the item Files line had none). Second-source constants (servo core-foundation-rs, progrium/macdriver enumtypes MIT): NSApplicationActivationPolicyAccessory=1, NSEventTypeApplicationDefined=15, NSVariableStatusItemLength=-1.0, NonactivatingPanel=128, Floating=3, CanJoinAllSpaces=1/Stationary=16/IgnoresCycle=64/FullScreenAuxiliary=256, NSBackingStoreBuffered=2, NSTextAlignmentCenter=2. NSAccessibilityUsageDescription has no Apple doc page (404) and is deliberately not declared in Info.plist. Overlay level is NSFloatingWindowLevel per the brief; whether it suffices above full-screen spaces is unverified until CI/Mac run (fallback NSStatusWindowLevel=25). P0 now has 61 items. -->

<!-- NOTE: 2026-10-30 - P0-SP5-004 merged. D-124 added (amends D-36): go-capnp v3.1.0-alpha.2 does not enforce the depth limit for composite-list elements and its default 64 MiB traversal budget exceeds a 16 MiB message cap; rules: non-recursive schemas without AnyPointer/interfaces (lint test), traversal limit never above the message cap, fuzz target for aliased expansion. Measured (host, Windows): BenchmarkEncodeHello 1235 ns/op 1482 B/op 5 allocs/op; BenchmarkDecodeClipAnnounce 468.5 ns/op 328 B/op 7 allocs/op; 30 s fuzz 2,134,568 execs clean after the cap fix; gen.sh deterministic. The research brief has no Cap'n Proto section; schema role values (client/server) and the nested Desktop/Monitor layout were chosen in the spike. -->

<!-- NOTE: 2026-10-30 - P0-SP4-006 merged (macos job extended only; windows and linux jobs, triggers and pins unchanged; steps continue on error so partial results survive; artifact spikes-macos uploaded always). P0-SP4-010 appended (README for spikes/macos/inject). P0 now has 62 items. -->

<!-- NOTE: 2026-10-30 - P0-SP3-004 merged (xgb v1.3.1 direct; host tests added via the approved Files extension). Findings for P0-SP3-009 / P2-X1: the X11 INCR receiver does not check the INCR size hint against the received total (the hint is advisory per ICCCM; a strict check would be a behaviour change needing a decision); the x11 package has no pure geometry helpers (Monitors reads the server); Xvfb two-monitor setup via xrandr --setmonitor was not run here (syntax unverified). -->

<!-- NOTE: 2026-10-30 - P0-SP5-005 merged. All 4 RFC 9382 Appendix B P-256/SHA-256 vectors reproduced byte for byte (w, x, pA, y, pB, K, TT, Hash(TT), Ke, Ka, KcA, KcB, confirmation MACs); the orchestrator re-ran the tests and cross-checked: the RFC's P-256 M and N constants equal the spike constants and are regenerated from the RFC Appendix A seed algorithm in a test; the other RFC hex values belong to P-384, P-521 and Ed25519 (not implemented). filippo.io/nistec v0.0.4 (BSD-3). Measured: BenchmarkSPAKE2Full about 286 us for both sides (about 0.14 ms per side, PB-22 budget 50 ms), 12,944 B/op, 146 allocs/op (Windows 11, Ryzen 7 7800X3D, Go 1.27.3, nistec asm backend). Findings for P0-SP5-008 and the P1-F freeze: (1) the spike uses a constant-time bit-serial reduction for w mod n (math/big only as a test oracle) and negates the point w*N instead of the scalar - P1-C2-005 may drop math/big from production code (parent decision at the freeze); (2) two rejections go beyond the RFC: a peer share byte-equal to the own share, and a shared point K equal to the identity; (3) the password info idA || 0x00 || idB is ambiguous only if an identity can contain 0x00 - peer ids are base32 (D-42) so it cannot occur; (4) RFC 9382 section 3.2 says the password hash SHOULD be memory-hard: D-110 omits it on purpose (6-digit code protected by attempt limits and TTL), rationale recorded in the spike README for the security-review gate; (5) the RFC vectors use AAD = nil, so the exporter-AAD binding path is covered only by this spike's own tests, not by an official vector. -->

