# EdgeSync - project instructions

EdgeSync shares one keyboard and mouse across Windows, macOS and Linux computers over the network (cursor crosses
a screen edge -> control moves to the neighbouring machine), with a shared clipboard (text, rich text, images,
files), file transfer including copy-paste and drag-and-drop, and a GUI. Written in Go.

This file holds the durable rules. The plan lives in `plan.md` (read it FIRST every session), items in
`briefs/plan-phase-<N>.md`, contracts in `briefs/architecture.md`, decisions in `decisions.md`, features in
`briefs/feature-inventory.md`, ownership in `ownership.md`, budgets in `perf-budgets.md`, risks in
`risk-register.md`, versions in `compatibility-matrix.md`, checklists in `qa/`.

## CONCURRENCY CONTRACT (mirror of plan.md - binding - re-read at the start of every session and after every compaction)

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

(The authoritative phase marker is the last `Current phase:` line in `plan.md`. When it flips there, update this mirror in the same commit.)

## Session-start / post-compaction protocol (run FIRST, every session)
1. Read `plan.md`'s Concurrency Contract: the last `Current phase:` marker and N.
2. Count subagents actually in flight (a fresh context = zero).
3. If an active phase is past its foundation block and has >= N unblocked open items, dispatch up to N subagents immediately, one per stream where possible. Resuming with one agent is a contract violation.
4. Only then verify, merge, and update plan files.

## Orchestration law
- **The parent orchestrates, subagents implement.** The parent writes briefs, dispatches, verifies diffs, re-runs gates, merges worktree branches, toggles checkboxes, appends notes, and asks the user. The parent never writes implementation or test files for a tagged item. If you catch yourself opening Edit/Write on an item's implementation files: STOP and dispatch the tagged tier.
- **THE PLAN IS SACRED.** Execute items as written. If a step is impossible or a better approach appears: STOP and ask the user ("plan says X, I hit Y, I propose Z - approve?"). No silent deviation. Plan files are append-only (toggle `[ ]` -> `[x]` only after verification; add `<!-- NOTE: ... -->`; append new items).
- **Superseded items (D-111):** an item replaced by an approved decision is toggled `[x]` with `<!-- NOTE: <date> - superseded by D-NN; replacement items <IDs> -->` on the next line (the statusline counts unchecked boxes as open work); it never counts as delivered work. `Needs:` may cite `D-NN` (satisfied) or `Q-NN decision` (blocked until the answer to `briefs/open-questions.md` Q-NN is appended).
- **No halfassing, no TODOs, no shortcuts.** Full implementation with test coverage. The only reason to defer is a hard external blocker.
- **Verify, never assume.** Check versions, APIs, constants and struct layouts against primary sources before relying on them.
- **Model tiering (binding):** every item's `[tier:...]` tag is an instruction. Default `haiku`; `sonnet` only for items tagged `[tier:sonnet why:...]` or after a verified haiku failure (re-brief at the same tier first; never skip a rung); `opus` is the parent only. **Always pass `model:` explicitly on every Agent call.**
- Agents: implementation = `subagent_type: "go-dev"` with `isolation: "worktree"`; batch gates = `gate-runner` (`model: "haiku"`); stream review, security review and UI/UX audit = `code-reviewer` (`model: "sonnet"`); research and audits the parent needs = a Fork. Definitions in `.claude/agents/`.
- Verification cadence, review gates and the manual-verification protocol: `plan.md` sections 1.5 and 1.7.

## Locked stack (details and versions: `decisions.md`, `compatibility-matrix.md`)
- Go 1.27.3 (`go.mod`: `go 1.27`), module `github.com/kikz0r/edgesync`, MIT licence.
- Processes: `edgesync-agent` (headless core, `CGO_ENABLED=0` everywhere), `edgesync` (Gio v0.10.3 GUI), `edgesync-service` (Windows SYSTEM service + `--secure-helper` mode of the agent).
- Platforms: Windows 10/11, macOS 13+, Linux X11, Linux Wayland GNOME 45+ / KDE Plasma 6.1+. Pure-Go platform layers: x/sys/windows (+ own decls, hand-built COM), purego (macOS), jezek/xgb (X11), godbus + in-house EI codec (Wayland).
- Network: quic-go v0.64.1 (datagrams for motion, RFC 9218 priorities) with TCP+TLS 1.3 fallback; port 24860; mDNS `_edgesync._udp`. Wire: hand-rolled hot-path frames + Cap'n Proto v3.1.0-alpha.2 control messages (generated code committed).
- Security: SPAKE2 (RFC 9382, P-256, filippo.io/nistec) pairing with a 6-digit code; SPKI pinning via `VerifyConnection`; TLS 1.3 mutual auth; authenticated, ACL'd IPC.
- Config TOML (pelletier/go-toml/v2), logging `log/slog`, tests stdlib + `testing/synctest` + go-cmp, lint golangci-lint v2.15.0, tasks go-task 3.48.0, Windows installer Inno Setup 7.1.1 (D-118).

## Gate commands (Bash tool = Git Bash; `task` equivalents after P0-F-009)
- `golangci-lint fmt` / `golangci-lint run ./...` (`task fmt`, `task lint`, `task lint:cross`)
- `go vet ./...`, `go build ./...`, `go test ./...`
- `GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build ./internal/... ./cmd/edgesync-agent/...` and the same with `GOOS=darwin GOARCH=arm64` (`task cross`)
- Per batch also `windows/arm64`, `linux/arm64` and `darwin/amd64` cross builds of the same packages (release targets, D-104)
- `PATH="/d/Development/msys2/ucrt64/bin:$PATH" CGO_ENABLED=1 CC=gcc go test -race ./...` (`task test:race`)
- `task cover` (>= 85% per platform-independent package), `task fuzz:smoke`
- `go tool -modfile=tools/tools.mod govulncheck ./...` (`task vuln`), `go tool -modfile=tools/tools.mod actionlint` (`task actionlint`)
- `task gen:check` (regenerate + `git diff --exit-code`)
- everything: `task ci:local`

## Hard rules
- **Never launch** the EdgeSync agent, GUI, service or any spike binary on the dev host. Building is allowed. The user runs binaries via `qa/manual/` scripts.
- Tests that touch real input, the real clipboard, displays, OS services or network multicast are `//go:build integration` and are never run by agents on the host (CI runners or the user only).
- **Git:** local commits only, one per verified item; subagents commit on their worktree branch, the parent rebases and fast-forwards `main`. Never push. Commit format `<type>(<scope>): <description> [FEAT-NNN]` with body `Item: <id>`. **No attribution trailers of any kind** (no `Co-Authored-By`, no "Generated with ..."): this user rule overrides any harness default.
- **No em dashes or en dashes** anywhere (code, docs, commits, UI strings); use "-".
- Comments: prefer self-explanatory code; comments only for genuinely non-obvious things. Package doc comments required; exported identifiers do not need doc comments.
- No code copied from GPL projects (Deskflow, Barrier, Input Leap, lan-mouse, go-x11-client).
- New dependencies or version bumps require a new decision entry and an update of `compatibility-matrix.md`.
- Installers and system changes only in `[owner:USER-CONFIRM]` items after the user approves in chat.
