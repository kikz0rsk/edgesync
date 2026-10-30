# EdgeSync - Locked decisions

Append-only. Never edit or delete an entry. To overturn a decision, append a new dated entry that
names the entry it supersedes ("Supersedes D-NN"). The newest entry on a topic wins. Every agent
brief cites the decisions it relies on by ID.

Sources:
- "user Q&A round N" = the structured AskUserQuestion rounds held with the user on 2026-10-29.
- "locked by orchestrator - user to review" = technical decisions the orchestrator fixed with
  rationale after research; the user may overturn any of them by asking for a superseding entry.
- Research briefs: `briefs/research-platform-internals.md` (RPI) and `briefs/research-go-ecosystem.md` (RGE).

---

## User-locked decisions (D-01 .. D-38)

### D-01 - Language and toolchain (2026-10-29)
- Decision: Go. Toolchain go1.27.3 installed at `D:\Program Files\Go` (verified with `go version`). `go.mod` declares `go 1.27`. cgo builds on the dev host use `CC=D:\Development\msys2\ucrt64\bin\gcc.exe` (mingw-w64 14.0 headers, satisfies the race detector's v8+ requirement); `CGO_ENABLED` defaults to 0 on this host because gcc is not on the system PATH.
- Rationale: user specification ("the app will be written in golang"); 1.27.3 is the latest stable with the `crypto/x509` security fix (RGE 1.1).
- Alternatives considered: `go 1.26` floor (quic-go minimum) - rejected, no reason to support an older toolchain for a new app.
- Source: user spec + user installed Go mid-session.

### D-02 - Supported platforms in v1 (2026-10-29)
- Decision: Windows 10/11, macOS 13+, Linux X11, Linux Wayland limited to GNOME 45+ and KDE Plasma 6.1+. Every platform acts as both server (controlling) and client (controlled). wlroots compositors (Sway, Hyprland, Wayfire) are out of scope.
- Rationale: user selected all four OS families; GNOME and KDE are the only compositors with the InputCapture portal (RPI 1.4).
- Alternatives considered: Windows-only first; X11-only Linux; adding wlroots via layer-shell + wlr virtual input (user declined).
- Source: user Q&A round 1 (platforms), round 9 (Wayland compositors).

### D-03 - Topology: switchable roles (2026-10-29)
- Decision: Synergy-style. One machine is the server (its physical keyboard and mouse drive all machines); the others are clients. Any install can be either role, chosen in the GUI.
- Rationale: user choice; simpler than a fully symmetric peer model.
- Alternatives considered: fixed role at install; fully symmetric peers (Mouse Without Borders style).
- Source: user Q&A round 1.

### D-04 - Layout model: free-form (2026-10-29)
- Decision: free-form arrangement like OS display settings. Each machine (with all its monitors) is placed anywhere on a canvas and snaps to neighbours. Partial-edge overlaps map proportionally so machines with different sizes and offsets line up. See D-51 for the exact mapping rule.
- Rationale: user choice; precise for mixed resolutions.
- Alternatives considered: grid cells (whole edge maps to whole neighbour edge).
- Source: user Q&A round 1.

### D-05 - Edge-switch guards (2026-10-29)
- Decision: four optional guards, each a separate user setting, all OFF by default (switching is immediate when all are off): dwell delay, double-tap edge, corner dead zones, modifier required. Default parameter values in D-57.
- Rationale: user selected all four.
- Alternatives considered: no guards; a single global guard.
- Source: user Q&A round 2.

### D-06 - Global hotkeys (2026-10-29)
- Decision: lock-to-screen toggle, jump-to-machine (per machine), cycle next/previous machine, panic/return-home (also releases all held keys and buttons). All rebindable in the GUI. Defaults in D-58.
- Rationale: user selected all four.
- Alternatives considered: subset.
- Source: user Q&A round 2.
<!-- NOTE: 2026-10-30 superseded in part by D-114 (Wayland servers while the pointer is home) -->

### D-07 - Windows secure desktop control (2026-10-29)
- Decision: full support. A Windows SYSTEM service plus a per-session helper make the lock screen, sign-in screen, UAC prompts and elevated admin windows controllable from the server; Ctrl+Alt+Del is delivered via `SendSAS`. Architecture in D-63/D-64.
- Rationale: user choice; unelevated injection is silently blocked by UIPI (RPI 2, Windows).
- Alternatives considered: user-level only (no secure desktop, no elevated windows).
- Source: user Q&A round 2.

### D-08 - Key mapping: physical keys (2026-10-29)
- Decision: keys travel as physical key identities encoded as USB HID usages (page and usage id); the target machine's own keyboard layout produces the character. Per-machine modifier remap is configurable; the default remap for macOS peers is Win<->Cmd and Alt<->Option. No character-mode fallback in v1.
- Rationale: user choice; stateless and symmetric across all four platforms, game- and shortcut-friendly (RPI 4).
- Alternatives considered: characters (keysyms); physical with per-machine character fallback.
- Source: user Q&A round 2.

### D-09 - Clipboard formats (2026-10-29)
- Decision: plain text, rich text (HTML and RTF), images (PNG on the wire, converted to each OS's native format), files and folders.
- Rationale: user selected all four.
- Alternatives considered: text only; text + images.
- Source: user Q&A round 3.

### D-10 - Clipboard timing: lazy on paste (2026-10-29)
- Decision: on a local copy only a small announcement (format list + sizes) goes to peers; the data is fetched when a local application on the receiving machine actually pastes, using each OS's delayed-rendering mechanism.
- Rationale: user choice; instant switching, no wasted bandwidth, large content works.
- Alternatives considered: transfer on screen switch (Synergy); push on every change.
- Source: user Q&A round 3.

### D-11 - File copy-paste: native lazy streaming (2026-10-29)
- Decision: paste triggers the transfer. Windows: virtual files via an `IDataObject` exposing `CFSTR_FILEDESCRIPTORW` + `CFSTR_FILECONTENTS` (IStream) with Explorer's own progress UI. macOS: file promises. Linux: fetch-on-request; the exact mechanism is decided by the P0 SP3 spike and confirmed with the user before P4.
- Rationale: user choice; files move only when pasted.
- Alternatives considered: stage-then-paste on all OSes; hybrid.
- Source: user Q&A round 3.

### D-12 - Additional file transfer paths (2026-10-29)
- Decision: drag files across screens (drop on the target machine's desktop or folder); send via the GUI (drop onto a machine in the EdgeSync window or pick files); a transfer queue with progress, speed, cancel, retry and resume of interrupted transfers. OS "Send to" menu integration is NOT in scope.
- Rationale: user selection.
- Alternatives considered: OS "Send to"/Share/Services menus (declined).
- Source: user Q&A round 3.

### D-13 - Sequencing of the risky file features (2026-10-29)
- Decision: native lazy file paste and cross-screen drag-and-drop stay in v1 exactly as chosen. P0 spikes prove the mechanics per OS; full implementation runs in the dedicated phase P4 after core input and clipboard work.
- Rationale: research found no Go prior art for Windows virtual files or OLE drag-and-drop, and Deskflow dropped its own drag-and-drop (RPI 6); user chose "spike, then own phase".
- Alternatives considered: build inline in the clipboard/file streams; switch to staging.
- Source: user Q&A round 9.

### D-14 - Pairing and trust (2026-10-29)
- Decision: one machine shows a 6-digit code, the user types it on the other. A SPAKE2 handshake per RFC 9382 over P-256 (implemented in-house on `filippo.io/nistec`, validated against the RFC's official test vectors, with a dedicated security-review gate) proves both sides know the code and authenticates each side's certificate. Afterwards both certificates are pinned; all sessions use TLS 1.3 with mutual authentication. Details in D-41, D-42, D-57.
- Rationale: user choice; no audited Go PAKE library exists (RGE 3.4), SPAKE2 is a small standardised construction with test vectors.
- Alternatives considered: `schollz/pake` v3.2.0 (unaudited, not RFC 9382); compare-codes commit-reveal; fingerprint confirmation; pre-shared key.
- Source: user Q&A rounds 4 and 10.

### D-15 - Discovery (2026-10-29)
- Decision: automatic discovery of EdgeSync peers on the LAN via mDNS/DNS-SD, plus manual add by IP address or hostname.
- Rationale: user choice.
- Alternatives considered: manual only.
- Source: user Q&A round 4.

### D-16 - Network scope (2026-10-29)
- Decision: same LAN plus any directly reachable IP (other subnets, WireGuard/Netbird/Tailscale VPNs). No cloud relay, no NAT traversal.
- Rationale: user choice.
- Alternatives considered: LAN only; internet with relay.
- Source: user Q&A round 4.

### D-17 - Receive directory and name clashes (2026-10-29)
- Decision: files not dropped or pasted into a specific folder (GUI sends) land in `~/Downloads/EdgeSync` (configurable). On a name clash the incoming item is renamed with a ` (1)`, ` (2)`, ... suffix before the extension.
- Rationale: user choice; matches browser behaviour.
- Alternatives considered: ask on clash; ask for a folder every time.
- Source: user Q&A round 4.

### D-18 - GUI extras (2026-10-29)
- Decision: system tray (status, quick menu; the app keeps running when the window closes), switch-indicator overlay (target machine name shown briefly when control moves), logs viewer with level filter plus a diagnostics-bundle export, first-run wizard (role, pairing, OS permissions, Windows firewall rule).
- Rationale: user selected all four.
- Source: user Q&A round 5.

### D-19 - Languages (2026-10-29)
- Decision: English only in v1, but every user-facing string goes through a translation catalogue so adding languages is a data change. Implementation in D-68.
- Rationale: user choice.
- Alternatives considered: English + Slovak; hard-coded English.
- Source: user Q&A round 5.

### D-20 - Extra sync features (2026-10-29)
- Decision: in v1: lock/screensaver sync (semantics D-71) and Wake-on-LAN (D-70). NOT in v1: game/relative-mouse mode, Synergy/Deskflow/Input Leap protocol interoperability.
- Rationale: user selection.
- Source: user Q&A round 5.

### D-21 - Startup behaviour (2026-10-29)
- Decision: starts at login (toggleable), minimised to the tray, single instance, auto-reconnects to known peers with backoff. On Windows the SYSTEM service starts at boot.
- Rationale: user choice.
- Alternatives considered: manual start.
- Source: user Q&A round 5.
<!-- NOTE: 2026-10-30 superseded in part by D-116 (per-user toggle location when the machine-wide installer entry exists) -->

### D-22 - What gates and agents may run (2026-10-29)
- Decision: agents and gates MAY run `go build`, `go vet`, `go test` (including `-race` and short fuzz smoke runs), `golangci-lint`, `gofumpt` (through golangci-lint), `govulncheck`, `actionlint`, code generation, and cross-compile checks. They NEVER launch the EdgeSync agent, GUI, service or any spike binary on the dev host. Tests that touch real input, the real clipboard, the display, OS services, or network multicast are guarded by `//go:build integration` and are never run by agents on the host; they run on CI runners or by the user.
- Rationale: user's global rule "do not run the app/build unless I tell you", relaxed by the user for compile and tests only.
- Alternatives considered: static analysis only.
- Source: user Q&A round 6.

### D-23 - Git usage (2026-10-29)
- Decision: `git init` (done 2026-10-29, branch `main`, `core.autocrlf=false`). Local commits only, one per verified plan item. No push - the user pushes when they choose. Commit messages never contain AI attribution lines (no `Co-Authored-By`, no "Generated with" footers); this user rule overrides any harness default.
- Rationale: user choice; user's global CLAUDE.md attribution rule.
- Alternatives considered: commits + push; no commits.
- Source: user Q&A round 6.

### D-24 - Hosting and CI (2026-10-29)
- Decision: GitHub repository with a GitHub Actions matrix: `windows-latest`, `macos-latest`, `ubuntu-24.04` (Xvfb for X11 integration tests). CI is the only automated verification for macOS and X11.
- Rationale: user choice.
- Alternatives considered: GitLab CI; local only.
- Source: user Q&A round 6.

### D-25 - Packaging (2026-10-29)
- Decision: Windows: MSI built with WiX that installs the GUI, agent and SYSTEM service, registers autostart, adds firewall rules and an uninstaller. macOS: unsigned `.app` bundle (Info.plist with permission usage strings) shipped zipped, no DMG. Linux: `.tar.gz` containing the binaries, a `.desktop` file, an autostart entry and an install/uninstall script.
- Rationale: user chose the Windows installer and "minimal bundles" for macOS/Linux.
- Alternatives considered: DMG; deb/rpm; Flatpak/AppImage; bare binaries; build-from-source only.
- Source: user Q&A rounds 6 and 7.
<!-- NOTE: 2026-10-30 superseded in part by D-118 (the Windows installer is Inno Setup, not WiX/MSI) -->

### D-26 - Code signing (2026-10-29)
- Decision: no certificates now; ship unsigned. The release pipeline contains a signing step that is skipped when no certificate is configured.
- Rationale: user has no certificate.
- Source: user Q&A round 7.

### D-27 - License and clean-room rule (2026-10-29)
- Decision: MIT. No code, tables or structures copied from GPL projects (Deskflow, Barrier, Input Leap, Synergy 1.x lineage, lan-mouse, linuxdeepin/go-x11-client). Permissive references (Mouse Without Borders/PowerToys MIT, purego Apache-2.0, golang-design/clipboard MIT, Chromium `dom_code_data.inc` BSD) may be studied, and their material used with the licence notice kept in `third_party/`.
- Rationale: user choice; licence hygiene (RPI 7).
- Source: user Q&A round 7.

### D-28 - Module path and name (2026-10-29)
- Decision: module `github.com/kikz0r/edgesync`; product name EdgeSync; binaries `edgesync` (GUI), `edgesync-agent`, `edgesync-service` (Windows only).
- Source: user Q&A round 7.

### D-29 - Input latency budget (2026-10-29)
- Decision: added latency between a physical event on the server and its injection on a client over wired gigabit Ethernet: p99 <= 5 ms. Wi-Fi is best effort. 1000 Hz mice are supported with coalescing. Benches in `perf-budgets.md`.
- Source: user Q&A round 8.

### D-30 - Layout size (2026-10-29)
- Decision: up to 64 machines in one layout.
- Source: user Q&A round 8.

### D-31 - Update checks (2026-10-29)
- Decision: notify only. Checks GitHub Releases at most once a day (toggleable), shows a tray/GUI notice with a link. No auto-install, no telemetry. Details in D-72.
- Source: user Q&A round 8.

### D-32 - Build concurrency (2026-10-29)
- Decision: N = 5 parallel subagents during fan-out; foundation blocks run one agent at a time.
- Source: user Q&A round 8.

### D-33 - GUI toolkit: Gio (2026-10-29)
- Decision: Gio `gioui.org` v0.10.3 (and `gioui.org/x` v0.10.3). The tray icon and the switch-indicator overlay are native code inside the agent, not Gio. File drop into the GUI window is native per OS, bound to the Gio window handle.
- Rationale: user choice over the Fyne recommendation; Gio is pure Go on Windows (no cgo), immediate-mode canvas suits the layout editor.
- Alternatives considered: Fyne v2.8.2 (recommended), Wails v3.0.0.
- Source: user Q&A round 10.

### D-34 - Process model (2026-10-29)
- Decision: `edgesync-agent` is the headless core (network, input, clipboard, files, tray, overlay) and builds with `CGO_ENABLED=0` on every OS, so its macOS and Linux code cross-compiles from Windows for checking. `edgesync` is the Gio GUI (pure Go on Windows, cgo on macOS/Linux) and talks to the agent over authenticated local IPC. `edgesync-service` is the Windows SYSTEM service.
- Rationale: user choice; a GUI crash never drops input; Windows already needs service IPC.
- Alternatives considered: single process.
- Source: user Q&A round 10.

### D-35 - Transport (2026-10-29)
- Decision: QUIC via `github.com/quic-go/quic-go` v0.64.1: RFC 9221 datagrams for mouse motion, RFC 9218 stream priorities for everything else. Automatic fallback to TCP + TLS 1.3 with separate connections for input/control and for bulk data when UDP is blocked. Channel map in `briefs/architecture.md`.
- Rationale: user choice; QUIC priorities stop file transfers from delaying input (RGE 3.1, 3.2).
- Alternatives considered: QUIC only; TCP + TLS only.
- Source: user Q&A round 10.

### D-36 - Wire formats (2026-10-29)
- Decision: hot-path input events use a hand-rolled fixed binary format with zero allocations. Control messages use Cap'n Proto: `capnproto.org/go/capnp/v3` pinned at v3.1.0-alpha.2 (MIT, last commit 2026-07-27, alpha-tagged for years), serialization only (no capnp RPC). Generated `*.capnp.go` is committed; the `capnp` compiler (vcpkg port 1.4.0) is needed only when schemas change; CI regenerates and diffs. Decoding uses ReadLimiter traversal limits and a nesting-depth limit, and every decoder is fuzzed.
- Rationale: user asked for "CapnProto if suitable"; verified suitable with these mitigations by the orchestrator (2026-10-29: proxy versions, licence file, vcpkg port, go.mod floor go 1.19).
- Alternatives considered: CBOR (fxamacker/cbor v2.10.0, recommended), protobuf v1.36.12.
- Source: user Q&A round 11.

### D-37 - Test coverage policy (2026-10-29)
- Decision: >= 85% statement coverage on platform-independent packages (enforced by `task cover`). OS backends are covered by contract tests run against fakes plus `integration`-tagged tests on CI runners (Xvfb for X11) and on the user's machines (2+ Windows PCs, one GNOME Wayland box). macOS and X11 can never be hand-tested by the user: CI only.
- Source: user Q&A rounds 1 and 11.

### D-38 - Config file (2026-10-29)
- Decision: TOML with a `schema_version` key and ordered migrations, written atomically (temp file + fsync + rename). Library: `github.com/pelletier/go-toml/v2` v2.4.3 (verified 2026-10-29, tag 2026-07-05, go floor 1.21).
- Rationale: user chose TOML. go-toml/v2 is preferred over BurntSushi/toml v1.6.0 because it is faster, strict-mode decoding (`DisallowUnknownFields`) catches typos, and it is actively released.
- Alternatives considered: JSON, YAML; BurntSushi/toml.
- Source: user Q&A round 11 (format); library pick locked by orchestrator - user to review.

---

## Orchestrator-locked technical decisions (D-39 ..) - locked by orchestrator - user to review

### D-39 - Pure-Go platform layers (2026-10-29)
- Decision: no cgo in the agent or service. Windows: `golang.org/x/sys` v0.49.0 (`windows`, `windows/svc`) plus own `LazyDLL` declarations for the user32/ole32/shell32/sas calls x/sys lacks; COM vtables built by hand with one `syscall.NewCallback` per method created once at start-up. macOS: `github.com/ebitengine/purego` v0.11.2 (+ `purego/objc`). X11: `github.com/jezek/xgb` v1.3.1 (xproto, xtest, randr, xfixes, xinerama, screensaver; XInput2/XKB only if SP3 shows they are needed, generated with `xgbgen` from MIT-licensed xcb-proto XML). Wayland: `github.com/godbus/dbus/v5` v5.2.4 for portals plus an in-house EI (libei) wire codec over the portal-provided socket, using `golang.org/x/sys/unix` for SCM_RIGHTS. Windows named pipes: `github.com/Microsoft/go-winio` v0.6.4.
- Rationale: D-34 requires `CGO_ENABLED=0` agents; research shows each layer is reachable in pure Go (RPI 8.2).
- Alternatives considered: cgo helpers (Objective-C, libei, libX11) - rejected by D-34; a spike failure that would need cgo is a STOP-and-ask event.

### D-40 - In-house native clipboard backends (2026-10-29)
- Decision: EdgeSync implements its own clipboard backend per OS. `golang.design/x/clipboard` v0.11.0 (MIT) may be studied but is not a dependency.
- Rationale: it is eager-only (no delayed rendering or lazy provider), lacks CF_HTML aliasing and RTF, and has no virtual files (RPI 5); D-10 and D-11 require laziness.

### D-41 - Identity certificates and key storage (2026-10-29)
- Decision: each identity is an ECDSA P-256 key with a self-signed X.509 certificate (CN = machine display name, serial random 128-bit, validity 10 years, re-issued with the same key 30 days before expiry). Storage: macOS and Linux - per-user PKCS#8 PEM file in the config dir with mode 0600 in a 0700 directory. Windows - machine-scoped identity owned by `edgesync-service` in `%ProgramData%\EdgeSync\identity\` (DACL: SYSTEM and Administrators only), key encrypted with DPAPI machine scope; the user agent never reads the key and signs TLS handshakes through a `crypto.Signer` proxy over the authenticated service IPC (D-47). Windows development builds without the service installed use an explicit `identity.mode = "user"` config setting: per-user key under `%APPDATA%\EdgeSync\identity\`, DPAPI user scope, owner-only DACL. The mode is explicit configuration, never a silent fallback.
- Rationale: small certs keep QUIC handshakes inside the 3x anti-amplification limit (RGE 3.1); one machine identity on Windows lets the pre-login client (D-64) and the user agent present the same identity.
- Alternatives considered: Ed25519 (fine, but P-256 matches SPAKE2 group and nistec); OS keychains (Keychain via purego, Secret Service) - more platform risk for little gain in v1.
- Note: SP1 may refine the signing-proxy design; any change is appended as a new decision.

### D-42 - Peer identity and pinning (2026-10-29)
- Decision: peer ID = lowercase RFC 4648 base32 without padding of SHA-256(SubjectPublicKeyInfo) (52 chars); short ID for display = first 8 chars as `xxxx-xxxx`. TLS: `MinVersion: tls.VersionTLS13`; server `ClientAuth: tls.RequireAnyClientCert`; client `InsecureSkipVerify: true` used ONLY together with `VerifyConnection`; `VerifyConnection` (never `VerifyPeerCertificate`, which is skipped on resumption) checks the SPKI hash of `PeerCertificates[0]` against the peer store. Pinning the SPKI (not the whole cert) allows certificate re-issue without re-pairing.
- Rationale: RGE 3.5.

### D-43 - Ports and service discovery names (2026-10-29)
- Decision: QUIC on UDP 24860 and TCP fallback on TCP 24860, configurable. IANA registry checked 2026-10-29: 24850/udp is assigned (Microsoft Device Association Discovery), 24851-24921 unassigned; 24800 is avoided (Synergy/Deskflow default). mDNS/DNS-SD service type `_edgesync._udp.local.`, instance name = machine display name, TXT keys: `v` (protocol version), `id` (peer ID), `role` (`server`|`client`), `tcp` (fallback port).
- Rationale: avoid clashes with incumbents and registered services.

### D-44 - Transfer integrity and resume (2026-10-29)
- Decision: files stream in 1 MiB chunks; each chunk carries its SHA-256; the manifest carries each file's SHA-256 once known (sender computes while reading; receiver verifies at the end). Resume restarts from the last verified chunk boundary of the partial file (partial files live as `<name>.edgesync-partial` next to the destination until verified, then are renamed). Max 4 concurrent file streams per peer.
- Rationale: stdlib-only hashing; chunk hashes allow verified-prefix resume without re-reading the whole file.

### D-45 - Safe file writes (2026-10-29)
- Decision: every received file is written through an `os.Root` opened on the destination directory. Incoming relative paths are sanitised before use: reject absolute paths, drive letters, UNC prefixes, `..` components, empty components, NUL and control characters, Windows reserved device names (CON, PRN, AUX, NUL, COM1-9, LPT1-9, with or without extension), trailing dots/spaces on Windows, and names longer than 255 bytes per component. Symlinks are never transferred: the sender skips them and lists them in the transfer report. Windows paths longer than 260 characters must work; P1 C4 verifies `os.Root` behaviour for them with tests and reports BLOCKED if it does not.
- Rationale: `os.Root` (Go 1.24+) confines operations and defeats symlink escape (RGE 1.3).

### D-46 - Agent IPC transport and authentication (2026-10-29)
- Decision: GUI <-> agent IPC. Windows: named pipe `\\.\pipe\edgesync-agent-<UserSID>` created with go-winio using SDDL `D:P(A;;GA;;;SY)(A;;GA;;;<UserSID>)` and first-instance semantics; the client verifies the server process's user SID and the server verifies the client's (via `GetNamedPipeServerProcessId` / `GetNamedPipeClientProcessId` + process token). Unix: socket `$XDG_RUNTIME_DIR/edgesync/agent.sock` (Linux) or `~/Library/Application Support/EdgeSync/run/agent.sock` (macOS), directory 0700, socket 0600, peer credentials checked (`SO_PEERCRED` on Linux, `LOCAL_PEERCRED` via getpeereid semantics on macOS) to equal the agent's uid. Both: a 32-byte random session token written to an owner-only file at agent start, sent by the GUI in the first message. Framing: standard Cap'n Proto stream framing with a 16 MiB message cap and ReadLimiter limits. The endpoint also implements single-instance detection.
- Rationale: least privilege on a channel that can drive input and read clipboard metadata.

### D-47 - Windows service IPC (2026-10-29)
- Decision: `\\.\pipe\edgesync-service`, SDDL grants SYSTEM full access and Authenticated Users connect (read/write) only; the service authorises every request: the client process must run in the session the request targets (token session ID), its image path must equal the installed agent path under `%ProgramFiles%\EdgeSync\` (admin-writable only), and the command must be on a fixed whitelist (`Hello`, `HelperStatus`, `SignTLS`, `RequestSAS`, `ReportInputDesktop`). No command executes programs, reads files, or changes configuration. All requests are logged. The secure helper connects back to the service over a second pipe whose DACL grants SYSTEM only.
- Rationale: Deskflow's world-writable SYSTEM pipe was CVE-2026-41477 (RPI 2, Windows).
<!-- NOTE: 2026-10-30 superseded in part by D-112 (the whitelist gains the read-only GetPreloginPeers) -->

### D-48 - Logging (2026-10-29)
- Decision: `log/slog` only. Agent and service write JSON lines to rotating files (10 MiB per file, 5 files kept) and keep an in-memory ring of the last 2000 records for the GUI log viewer; debug builds also write text to stderr. Default level Info. Clipboard contents are never logged at any level; file names and paths only at Debug; peer IDs only in short form. Locations: Windows `%LOCALAPPDATA%\EdgeSync\logs\`, service `%ProgramData%\EdgeSync\logs\`; macOS `~/Library/Logs/EdgeSync/`; Linux `$XDG_STATE_HOME/edgesync/logs/` (default `~/.local/state/edgesync/logs/`). Rotation is implemented in-house in `internal/logging` (no dependency).

### D-49 - Mouse and key event model (2026-10-29)
- Decision: the server owns the virtual cursor. While a client is active the server converts captured relative motion into an absolute position in the client's desktop coordinates (physical pixels, scaled by the server/client scale ratio so logical speed is preserved) and sends `MotionAbs` as QUIC datagrams, latest-wins, with a 32-bit sequence number (receiver drops older sequences). The first position after `Enter` and every position when datagrams are unavailable go on the reliable input stream. Buttons (left, right, middle, X1, X2), keys and wheel (high-resolution units, 120 per notch, vertical and horizontal) go on the reliable ordered input stream. On every switch the server sends `ReleaseAll` to the machine being left and a modifier-state sync to the machine being entered. Release is keyed by physical key identity.
- Rationale: latest-wins motion keeps latency flat under loss; reliable ordered keys/buttons avoid stuck keys (RPI 4).

### D-50 - Windows capture design (2026-10-29, pending SP1)
- Decision (default design): `WH_MOUSE_LL` + `WH_KEYBOARD_LL` installed on a dedicated goroutine locked to its OS thread with its own `GetMessage` loop; callbacks do only an atomic flag read, a non-blocking push into a preallocated ring buffer and a return; injected events carrying EdgeSync's `dwExtraInfo` marker are ignored; a liveness watchdog injects a marked no-op event every 1 s and re-installs the hooks if it is not observed within 2 s; relative deltas while grabbed come from Raw Input (`RIDEV_INPUTSINK` on a hidden message window); `GOGC` and `debug.SetMemoryLimit` tuned per SP1 measurements. SP1 may replace Raw Input with re-centring; such a change is appended as a new decision.

### D-51 - Displays, DPI and the layout canvas (2026-10-29)
- Decision: each machine reports its monitors as rectangles in physical pixels in its own desktop coordinate space plus a per-monitor scale factor (Windows: Per-Monitor-v2 DPI awareness declared in the exe manifest). The layout canvas uses logical units = physical pixels / scale. A machine is placed on the canvas as a rigid group of its monitors. "Real edges" are monitor sides not shared with another monitor of the same machine. Interpretation of D-04 "map proportionally": crossing happens only within the overlap of a real edge and the neighbour's facing real edge; the crossing point keeps its canvas coordinate (geometric continuity), converted into the neighbour's physical pixels with the neighbour's scale. Points outside any overlap behave as a wall.
- Rationale: continuity is what "lines up correctly" means for free-form layouts; stretching a whole edge would distort aligned monitors.
- Alternative considered: stretch the source overlap onto the target overlap. The user may request it via a superseding decision.
<!-- NOTE: 2026-10-30 confirmed by D-120 -->

### D-52 - Theme (2026-10-29)
- Decision: the GUI follows the OS light/dark setting. The agent detects it (Windows registry `AppsUseLightTheme`, macOS `AppleInterfaceStyle` via `NSUserDefaults`, Linux `org.freedesktop.appearance color-scheme` via the Settings portal) and publishes `ThemeChanged` over IPC.

### D-53 - Task runner (2026-10-29)
- Decision: go-task, `Taskfile.yml` schema version 3. Verified installed: task 3.48.0. Every gate has a `task` entry and an equivalent raw command documented in `CLAUDE.md`.

### D-54 - Dev tool pinning (2026-10-29)
- Decision: an alternate module file `tools/tools.mod` (with `tools/tools.sum`) pins Go-based tools with `tool` directives and is used as `go tool -modfile=tools/tools.mod <tool>`. Its `module` line is identical to the root module (`github.com/kikz0r/edgesync`), which is how `-modfile` alternate files must be shaped; it is deliberately NOT named `go.mod`, so `tools/` does not become a nested module and small helper programs such as `tools/covercheck` stay inside the main module and its gates. Pinned tools: gofumpt v0.12.0 (`mvdan.cc/gofumpt`), govulncheck (`golang.org/x/vuln` v1.9.0), capnpc-go (`capnproto.org/go/capnp/v3` v3.1.0-alpha.2), goversioninfo (`github.com/josephspurrier/goversioninfo` v1.7.0), actionlint (`github.com/rhysd/actionlint` v1.7.12). golangci-lint v2.15.0 is installed from its official release binary, because its docs recommend binary installation and advise against `go tool` (RGE 4.1); CI uses `golangci/golangci-lint-action` pinned to v2.15.0. If `-modfile` invocation fails for a tool, P0-F reports BLOCKED and the orchestrator decides.
- Note: the planning directive said "tools/go.mod"; a file named `go.mod` would make `tools/` a separate module that `-modfile` cannot cleanly target, so the alternate-file name was chosen.

### D-55 - Lint policy (2026-10-29)
- Decision: golangci-lint v2 config `.golangci.yml`, `version: "2"`, `linters.default: none` plus an explicit strict list (errcheck with type-assertion checks, govet enable-all, ineffassign, staticcheck, unused, gosec, errorlint, bodyclose, noctx, contextcheck, exhaustive, nilerr, sloglint, revive, gocritic, unparam, unconvert, perfsprint, copyloopvar, intrange, errname, usestdlibvars, paralleltest, thelper, nolintlint (explanation + specific required), forbidigo, depguard, wrapcheck); formatters gofumpt + goimports (local prefix `github.com/kikz0r/edgesync`). revive's `exported` rule is disabled: no doc comment is forced on exported identifiers (user prefers self-explanatory code); `package-comments` stays on, every package has a package doc comment. `depguard` enforces boundaries: nothing outside `gui/` and `cmd/edgesync` imports `gioui.org`; platform-independent packages never import `internal/platform/windows|darwin|x11|wayland|linux`; `internal/proto/capnp` is imported only by `internal/proto/...`, `internal/session`, `internal/ipc`, `internal/service` and the generator wrapper `tools/capnpgen`. Generated code is excluded (`exclusions.generated: strict`).

### D-56 - Test libraries (2026-10-29)
- Decision: stdlib `testing`, `testing/synctest` (GA since Go 1.25, `synctest.Sleep` added in 1.27 per RGE 1.3) for every timer-driven behaviour (dwell, double-tap, keepalive, backoff, pairing TTL), native fuzzing, and `github.com/google/go-cmp` v0.7.0 for diffs. No assertion or mocking frameworks; fakes are hand-written in `internal/platform/fake` and per-package `testdata/`.

### D-57 - Default values (2026-10-29)
| Setting | Default |
|---|---|
| Dwell delay (when enabled) | 150 ms |
| Double-tap window (when enabled) | 400 ms between the two edge contacts; the pointer must move >= 8 logical px away from the edge between them |
| Corner dead zone (when enabled) | 32 logical px from each corner of the real edge segment |
| Modifier required (when enabled) | Left or Right Ctrl |
| Motion coalescing | at most one `MotionAbs` per 1 ms; latest wins |
| Capture ring buffer | 4096 events |
| QUIC keepalive period / max idle timeout | 1 s / 5 s |
| App heartbeat on input stream | every 1 s; peer considered lost after 3 s without any packet |
| Reconnect backoff | 250 ms min, 30 s max, factor 2, full jitter |
| Handshake timeout | 5 s |
| Clipboard non-file payload cap | 64 MiB per format |
| Clipboard fetch timeout | 10 s for <= 1 MiB, otherwise 10 s + 1 s per started MiB |
| Transfer chunk size | 1 MiB |
| Max concurrent file streams per peer | 4 |
| Pairing code | 6 decimal digits from crypto/rand, TTL 120 s, max 3 wrong attempts then invalidated |
| Update check | first 5 min after start, then every 24 h |
| Log rotation | 10 MiB x 5 files |
| Switch-indicator overlay | shown 800 ms, fades 200 ms |
| Auto Wake-on-LAN throttle | once per 60 s per peer |
| IPC message cap | 16 MiB |
| Staging retention | entries older than 24 h removed at agent start |

### D-58 - Hotkey defaults (2026-10-29)
| Action | Default binding |
|---|---|
| Lock to screen (toggle) | Scroll Lock |
| Jump to machine 1..12 | Ctrl+Alt+Shift+F1 .. F12 (by layout order; machines 13..64 unbound until assigned) |
| Cycle next / previous | Ctrl+Alt+Shift+Right / Ctrl+Alt+Shift+Left |
| Panic / return home | Ctrl+Alt+Pause |
- Hotkeys are recognised by the engine from the captured key stream on the server (not `RegisterHotKey`), so they work whether the cursor is home or remote; the hotkey key events are swallowed and never forwarded. The GUI hotkey editor rejects duplicates and warns on known OS-reserved combos.
<!-- NOTE: 2026-10-30 superseded in part by D-113 (adds the Send Ctrl+Alt+Del binding) and D-114 (Wayland servers while the pointer is home) -->

### D-59 - Connection topology (2026-10-29)
- Decision: star topology. Every agent listens on the configured port. The server dials every paired machine placed in its layout (with backoff, D-57). A client accepts a session from any paired peer that announces the server role, but holds at most one server session; a second server is rejected with `Busy` and the GUI shows which server holds the client. Clipboard data and file streams between two clients are relayed by the server as pass-through streams (no disk staging on the server).
- Rationale: matches D-03; one connectivity model; pairing is per machine pair.

### D-60 - Layout authority (2026-10-29)
- Decision: the server owns the layout and pushes a `LayoutSnapshot` to every client on connect and on change; clients display it read-only. The layout is stored in the server's config file.

### D-61 - Clipboard scope rules (2026-10-29)
- Decision: X11/Wayland sync the CLIPBOARD selection only (PRIMARY is not synced). Content marked sensitive/concealed by its source (Windows `ExcludeClipboardContentFromMonitorProcessing` or `CanIncludeInClipboardHistory`=0 formats, macOS `org.nspasteboard.ConcealedType`, `x-kde-passwordManagerHint` on Linux) is never announced. Settings: a master "share clipboard" toggle and per-format toggles (text, rich text, images, files), all ON by default. Echo prevention: every announcement carries an origin peer ID and a sequence number; a backend never re-announces content it placed itself.
<!-- NOTE: 2026-10-30 superseded in part by D-121 (concealed content is now shared; CLIPBOARD-only, toggles and no-logging stay) -->

### D-62 - Cut semantics across machines (2026-10-29)
- Decision: a cross-machine paste of files that were "cut" on the source behaves as copy; the source files are never deleted remotely.
- Rationale: deleting files on another machine after a remote paste is surprising and dangerous.

### D-63 - Windows service and secure-helper architecture (2026-10-29, pending SP1)
- Decision (default design): `edgesync-service` (LocalSystem, session 0) tracks sessions via `SERVICE_ACCEPT_SESSIONCHANGE` and keeps exactly one `edgesync-agent --secure-helper` process running in the active console session, started with a duplicate of that session's `winlogon.exe` token with `TokenUIAccess` set (`CreateProcessAsUser`). The helper follows the input desktop (`OpenInputDesktop` poll every 200 ms + `SetThreadDesktop`), and provides two capabilities to the user agent over an authenticated pipe: injection on any desktop (covers UIPI, UAC, lock screen) and capture on the secure desktop. When the service is installed, all Windows injection goes through the helper (configuration `windows.injection = "helper"`); development builds without the service use `windows.injection = "direct"` explicitly. Ctrl+Alt+Del: the agent sends `RequestSAS` to the service, which calls `SendSAS(FALSE)`; the MSI sets the `SoftwareSASGeneration` policy to 1. If the server machine's own input desktop switches to a secure desktop while a client is active, control returns home.
- SP1 validates every element; changes are appended as new decisions after user approval if they alter behaviour.
<!-- NOTE: 2026-10-30 superseded in part by D-118 (the Inno Setup installer, not an MSI, sets the SoftwareSASGeneration policy) -->

### D-64 - Pre-login client mode (2026-10-29)
- Decision: when no user is logged on to a Windows client, the service runs the secure helper in "pre-login client" mode: it uses the machine identity (D-41) and machine-scoped peer store, accepts only the server session from already-paired servers, and supports input injection only (no clipboard, no files, no tray). A Windows machine at the sign-in screen cannot act as a server.
- Rationale: covers "login screen" control (D-07) with minimal attack surface.

### D-65 - WiX version (2026-10-29)
- Decision: WiX Toolset 5.0.2 (`dotnet tool install --global wix --version 5.0.2`), licence MS-RL. WiX 6.x and 7.x (latest 7.0.1, published 2026-10-26) ship under the "Open Source Maintenance Fee" EULA (verified from the nuget nuspec), which can require a paid maintenance fee; 5.0.2 avoids it. Needs a .NET SDK (8 or newer) on the build machine. USER TO CONFIRM.
<!-- NOTE: 2026-10-30 superseded by D-118 (Inno Setup 7.1.1 replaces WiX) -->

### D-66 - Windows executable resources (2026-10-29)
- Decision: goversioninfo v1.7.0 generates `resource_windows_amd64.syso` for `cmd/edgesync`, `cmd/edgesync-agent`, `cmd/edgesync-service` with the application manifest (`<dpiAwareness>PerMonitorV2</dpiAwareness>` + `<dpiAware>true</dpiAware>`, `asInvoker`, supportedOS Windows 10/11), version info and icon. `uiAccess` is NOT used (it requires signing; the helper token approach replaces it).

### D-67 - Generated artefacts are committed (2026-10-29)
- Decision: generated Cap'n Proto Go code, `.syso` resources and the keymap tables are committed. Every generator is wired as a `//go:generate` directive in the package that owns the output, so `task gen` is simply `go generate ./...` and `task gen:check` (CI) regenerates and fails on any `git diff`. Cap'n Proto generation goes through a small cross-platform wrapper `tools/capnpgen` (added in P1-F) that builds capnpc-go from `tools/tools.mod` and finds the `capnp` compiler via the `CAPNP` environment variable or PATH (dev host: the vcpkg path recorded by P0-F-002; CI: apt `capnproto`). Generated files are never hand-edited.

### D-68 - i18n implementation (2026-10-29)
- Decision: `internal/i18n` with an embedded `locales/en.toml` catalogue (keys like `layout.title`), `i18n.T(key, args...)` with named placeholders and one/other plural forms. A test parses all Go sources with `go/ast`, collects every literal key passed to `i18n.T` and fails on missing or unused keys. No string shown to the user (GUI, tray, overlay, notifications) bypasses the catalogue. Log messages are not translated.

### D-69 - Filesystem locations (2026-10-29)
| Purpose | Windows | macOS | Linux |
|---|---|---|---|
| Config | `%APPDATA%\EdgeSync\config.toml` | `~/Library/Application Support/EdgeSync/config.toml` | `$XDG_CONFIG_HOME/edgesync/config.toml` |
| Machine config (service) | `%ProgramData%\EdgeSync\machine.toml` | - | - |
| Logs | see D-48 | see D-48 | see D-48 |
| Staging | `%LOCALAPPDATA%\EdgeSync\staging\` | `~/Library/Caches/EdgeSync/staging/` | `$XDG_CACHE_HOME/edgesync/staging/` |
| Receive dir | Downloads known folder + `EdgeSync` | `~/Downloads/EdgeSync` | `XDG_DOWNLOAD_DIR` (from `user-dirs.dirs`, else `~/Downloads`) + `EdgeSync` |
- Resolved in one package, `internal/paths`.

### D-70 - Wake-on-LAN (2026-10-29)
- Decision: peers share their NIC MAC addresses and IPv4 broadcast addresses in the handshake; the server stores them. Wake = UDP magic packet to port 9 on each stored subnet-directed broadcast and on 255.255.255.255. Triggers: GUI/tray action "Wake <machine>", and automatically when the cursor hits an edge whose neighbour is offline (setting, default ON, throttle D-57). While a neighbour is offline its edge acts as a wall.

### D-71 - Lock/screensaver sync semantics (2026-10-29)
- Decision: server lock -> lock all clients; server screensaver start -> start the screensaver on all clients; server unlock/screensaver end -> wake client displays only (never unlock, never bypass authentication). Each of the three is a toggle, all ON by default. Clients never propagate to the server.
<!-- NOTE: 2026-10-30 macOS lock mechanism confirmed by D-115 -->

### D-72 - Update check details (2026-10-29)
- Decision: `GET https://api.github.com/repos/kikz0r/edgesync/releases/latest` with `If-None-Match` (ETag cached in the state dir), semantic-version comparison against the embedded build version, pre-releases ignored, result shown in tray + GUI with the release URL. Disabled by a setting. No other outbound calls exist besides peers and this one.

### D-73 - Subagent workflow and git integration (2026-10-29)
- Decision: every implementation subagent runs with Agent `isolation: "worktree"` and commits its single item on its worktree branch using the commit format; it never commits to `main` and never edits plan files. The parent verifies (diff + gates), rebases the branch onto `main`, fast-forwards `main`, then toggles the item and commits `chore(plan): ...`. Plan files (`plan.md`, `briefs/plan-phase-*.md`, `decisions.md`, `ownership.md`, `risk-register.md`, `perf-budgets.md`, `compatibility-matrix.md`) are edited by the parent only.

### D-74 - Spikes (2026-10-29)
- Decision: spike code lives in `spikes/` as a separate Go module (`github.com/kikz0r/edgesync/spikes`) excluded from the main module's gates, linted only with `go vet` and `go build`. Spike binaries are run by the user following `qa/manual/p0-*.md` scripts (never by agents); automatable checks are tests in the spikes module (loopback-only tests may run on the host; anything touching real input/clipboard/display/multicast is `integration`-tagged). Each spike stream writes `briefs/spikes/<stream>.md` with a go/no-go per mechanism. A spike result contradicting a locked decision stops the stream: the parent asks the user before anything changes. The spikes module is deleted in P5 (item in `briefs/plan-phase-5.md`), after its findings are archived.

### D-75 - Error and API conventions (2026-10-29)
- Decision: context-first APIs for anything that blocks; errors wrapped with `%w`, sentinel errors per package (`var ErrX = errors.New(...)`), typed errors where callers branch on fields; no panics outside `main` and impossible-state assertions; no swallowed errors; no silent fallbacks; no package-level mutable state except build-tag-selected constructors; goroutines are owned by a struct with `Close`/`ctx` cancellation and are leak-checked in tests.

### D-76 - Hot-path event representation (2026-10-29)
- Decision: input events inside the agent are a single fixed-size struct `platform.InputEvent` with a `Kind` field (no interface values) passed by value through preallocated rings and channels, so the capture -> engine -> encode path performs zero heap allocations per event (benchmarked, `perf-budgets.md`).

### D-77 - GUI and agent lifecycle (2026-10-29)
- Decision: the agent is the long-running process (autostart, tray). "Open EdgeSync" in the tray launches the GUI process; closing the GUI window exits the GUI process, never the agent. Launching the GUI when no agent is running starts the agent first. Both are single-instance: a second GUI launch asks the running GUI to focus via the agent (`FocusGUI` event).

### D-78 - Versioning (2026-10-29)
- Decision: SemVer tags `vMAJOR.MINOR.PATCH`; build version embedded with `-ldflags "-X github.com/kikz0r/edgesync/internal/buildinfo.Version=..."`; wire protocol version is an integer starting at 1, negotiated in `Hello` (peers with a different major protocol version are rejected with a clear message).

### D-79 - Copyright holder (2026-10-29)
- Decision: LICENSE reads "Copyright (c) 2026 kikz0r" (matches the module owner). USER TO CONFIRM or replace with a legal name.
<!-- NOTE: 2026-10-30 confirmed by D-119 -->

---

## Planning consolidation (D-80 ..) - 2026-10-30 - locked by orchestrator - user to review

Closes the spine gaps raised while the P1-P5 item files were written (P2 gap table G-01..G-18, the P3-F spine-gap
list, the P4-F-001 pre-flight proposals, the P5 design points). Product questions that could not be closed as
engineering calls are listed in `briefs/open-questions.md` (Q-01..Q-10); their answers are appended as decisions at
plan approval.

### D-80 - Callback trampolines and dispatch tables (2026-10-30; amends D-75; closes G-01)
- Decision: D-75's "no package-level mutable state" has one narrow exception for OS callbacks that carry no Go
  context (LL hook procs, WNDPROC, COM method vtables, CFRunLoop/CGEventTap and `dispatch_async_f` callbacks, other
  purego callbacks). Per callback signature exactly one trampoline is created once (`sync.Once`) with
  `syscall.NewCallback` / `purego.NewCallback`, and it dispatches through a package-level table guarded by a mutex or
  held in an `atomic.Pointer`, keyed by HWND, COM `this` pointer, refcon handle, or a single active-instance pointer.
  Nothing else is package-level. Entries are unregistered on `Close`; tests assert empty tables after `Close`.
- Rationale: callbacks are a finite, never-freed resource (`NewCallback` limit) and the OS passes no Go context to
  hooks; the table is the minimal state that makes them safe.
- Alternatives considered: one callback per instance (exhausts the callback budget); cgo shims (excluded by D-34).

### D-81 - Brokered secure-helper channel (2026-10-30; closes G-04)
- Decision: the helper pipe keeps a SYSTEM-only DACL (D-47). The user agent never opens it. On an authorised `Hello`
  over `\\.\pipe\edgesync-service`, the service opens a client end of the helper pipe itself and passes it to the
  verified agent process with `DuplicateHandle` (target = the agent's process handle obtained from the pipe client
  PID, after the D-47 image-path and session checks) and returns the duplicated handle value in the response. The
  duplicated handle is the agent's only route to the helper; it is closed when the agent disconnects or the helper
  restarts, and the agent re-requests it with `Hello`.
- Rationale: the helper pipe stays unreachable for every other process in the session while the authorised agent gets
  a channel without widening any DACL.
- Alternatives considered: grant the session user access to the helper pipe (any process of that user could inject on
  the secure desktop); relay all helper traffic through the service (extra hop on the injection path).

### D-82 - ReportInputDesktop semantics (2026-10-30; closes G-05)
- Decision: `ReportInputDesktop{accessible: bool}` is sent by the user agent whenever `OpenInputDesktop` from its own
  session switches between accessible and inaccessible (secure desktop: UAC, Ctrl+Alt+Del screen, lock screen). On
  `accessible = false` the service tells the helper to start forwarding secure-desktop key and button events to the
  agent through the brokered channel (D-81); on `true` it stops. The server engine keeps the D-63 policy (control
  returns home when the server's own input desktop turns secure) and uses forwarded events only to recognise the panic
  hotkey and to issue release-all; they are never forwarded to clients.
- Rationale: gives FEAT-047 a concrete, minimal meaning without letting secure-desktop input leave the machine.

### D-83 - machine.toml schema (2026-10-30; closes G-07)
- Decision: `%ProgramData%\EdgeSync\machine.toml`, written only by the service or an elevated process; DACL SYSTEM and
  Administrators full control, Users read. Content: `schema_version = 1`; `[service] helper_enabled = true`;
  `[prelogin] enabled = true`, `port = 24860`; `[log] level = "info"`. Strict decoding (unknown keys are an error),
  atomic writes (temp file + rename in the same directory), migrations keyed by `schema_version` like the user config
  (D-38).
- Note: how the machine-scoped peer store for pre-login mode gets populated is a product choice (Q-01).
<!-- NOTE: 2026-10-30 the population of the machine-scoped peer store is answered by D-112 -->

### D-84 - Agent entry point, main thread and secure-helper dispatch (2026-10-30; closes G-09, G-10 and the P1-C5 main.go gap)
- Decision: `cmd/edgesync-agent/main.go` is created by item P2-X-003 once every `host_<goos>.go` exists (P2-W1-020,
  P2-X1-019, P2-M1-018) and the darwin main-thread runner exists (P2-M1-003). It contains only: an `init` that calls
  `runtime.LockOSThread()`; `main`, which dispatches `--secure-helper` to `secureHelperMain` in
  `cmd/edgesync-agent/helper_windows.go` (calls `helper.Main(ctx, args) int` from `internal/platform/windows/helper`;
  `helper_other.go` returns a usage error on other OSes) and otherwise runs
  `os.Exit(host.RunMain(func() int { return agent.Main(ctx, os.Args[1:], host.Open, os.Stdout, os.Stderr) }))`.
  `host.RunMain(run func() int) int` lives in `internal/platform/host/runmain_other.go` (`//go:build !darwin`, calls
  `run` directly; P2-X-003) and `internal/platform/host/runmain_darwin.go` (hands the main thread to `appkit.RunMain`;
  P2-M1-003). The normal agent always opens its backend with `platform.Options.SecureHelper = false`; only
  `helper.Main` opens the Windows backend with `SecureHelper = true`.
- Rationale: one owner for `main`, no package `main` without a `main` function at any commit, and AppKit gets the real
  main thread on macOS.

### D-85 - golang.org/x/image dependency (2026-10-30; closes G-11 and the P1-C4-006 dependency gap)
- Decision: `golang.org/x/image` v0.47.0 (BSD-3-Clause, tag 2026-10-09, `go 1.26.0`, verified on proxy.golang.org
  2026-10-30), which requires `golang.org/x/text` v0.43.0 (tag 2026-10-09) and `golang.org/x/sys` v0.49.0 (already
  pinned). Used by `internal/clipboard/convert` (TIFF; P1-C4-006, which adds it first) and by the X11 overlay text
  (`font/opentype`, `font/gofont/goregular`, `vector`; P2-X1-018). `golang.org/x/text/unicode/norm` serves D-98.

### D-86 - macOS pasteboard polling and read minimisation (2026-10-30; closes G-13, addresses R-15)
- Decision: the macOS clipboard backend polls `NSPasteboard.generalPasteboard.changeCount` every 500 ms. On a change it
  reads only the pasteboard item types (never data) to build the announcement; data is read only when a peer fetches
  it. For files, the announcement carries the `files` format only; file URLs and the manifest are read on the first
  fetch. If the P1-F transfer schema requires the manifest at announce time, P4-F-001 records it and P4-F adds the
  additive schema field `manifestDeferred` (this decision pre-authorises that additive change).
- Rationale: macOS 15.4+ pasteboard-access alerts are triggered by data reads; reading types only keeps the announce
  path silent (to be confirmed with CI evidence, R-15).

### D-87 - Wheel units and sign (2026-10-30; closes G-02)
- Decision: `InputEvent.DX/DY` for `KindWheel` and `InputInjector.Wheel(dx, dy)` use 1/120 notch units; `DY > 0` means
  the wheel was rotated away from the user (content scrolls up), `DX > 0` means scroll right (the Windows `WHEEL_DELTA`
  convention). Backends convert from their native convention (macOS natural-scrolling flag, EI positive y = down, X11
  buttons 4-7). The convention is the doc comment of `KindWheel` in `internal/platform/types.go` (P1-F-004) and is
  tested by the injector conformance suite (P1-F-008).

### D-88 - EdgeSegment.At and the contact rule (2026-10-30; closes G-03)
- Decision: `platform.Rect` is half-open. `EdgeSegment.At` is the boundary line as computed by P1-C3-006: `A.X` for
  Left, `A.Right()` for Right, `A.Y` for Top, `A.Bottom()` for Bottom. A pointer touches a Left/Top segment when its
  coordinate equals `At`, and a Right/Bottom segment when its coordinate equals `At - 1` (the last pixel inside), with
  the along-coordinate inside `[From, To)`. Documented on `EdgeSegment` in `internal/platform/types.go` (P1-F-004) and
  tested in `internal/layout` and the capturer conformance suite.

### D-89 - Native desktop coordinate spaces (2026-10-30; amends D-51 and D-49; closes G-14)
- Decision: each backend reports monitors, edges, contacts, motion deltas and injection targets in its native,
  contiguous desktop coordinate space: Windows physical pixels (Per-Monitor-v2), X11 physical pixels (RandR), macOS
  global points (`CGDisplayBounds`), Wayland compositor logical pixels (portal zones and EI regions).
  `Monitor.ScalePermille` = native units per canvas unit: the DPI scale on Windows and X11, always 1000 on macOS and
  Wayland (their native units are already logical). The P1-C3 canvas formula, real-edge computation and crossing
  mapping are unchanged. Where D-49/D-51 say "physical pixels", read "native units".
- Rationale: real edges need contiguous coordinates; macOS points and Wayland logical pixels are contiguous, while
  "logical x per-monitor scale" leaves gaps between mixed-scale monitors.
- Alternatives considered: computing real edges in canvas units on some platforms (two edge algorithms); rejected.

### D-90 - Edge-contact decision contract (2026-10-30; closes G-17)
- Decision: the server engine answers every `KindEdgeContact` it receives while not grabbed with exactly one of `Grab`
  (switch accepted) or `Ungrab(at = contact point)` (rejected). Capturers whose contact already holds the pointer
  (Wayland barrier activation) implement the optional interface `platform.ContactHolder { HoldsContact() bool }` in
  `internal/platform/capture.go` (P1-F-005). For such capturers the engine decides within 400 ms: modifier and corner
  guards decide immediately; the dwell guard holds the contact for min(dwell, 400 ms) and the GUI shows that dwell is
  capped on that machine; the double-tap guard releases the first contact immediately and accepts the second. The
  backend releases a pending contact itself after 500 ms with a Warn log, so the pointer never stays captured.
- Rationale: one engine contract for polling and barrier backends, with a bounded hold time on Wayland.

### D-91 - Linux session and display-power event sources (2026-10-30; closes G-18)
- Decision: lock and unlock from logind (`org.freedesktop.login1` session `Lock`/`Unlock` signals and `LockedHint`);
  screensaver from `org.freedesktop.ScreenSaver` `ActiveChanged` (KDE) and `org.gnome.ScreenSaver` `ActiveChanged`
  (GNOME); display power on X11 from the DPMS extension (`jezek/xgb/dpms`), on GNOME Wayland from
  `org.gnome.Mutter.DisplayConfig` `PowerSaveMode`. A KDE display-power source is used only if the implementing item
  verifies a documented interface by introspection; otherwise the KDE backend reports display-power events as
  `platform.ErrUnsupported`, the GUI status page shows the limitation, and R-09 records it. No guessed events.

### D-92 - X11 observation without a grab via RECORD (2026-10-30; closes G-12)
- Decision: while not grabbed, the X11 capturer observes pointer and key events with the RECORD extension
  (`jezek/xgb/record`, present in jezek/xgb, checked 2026-10-30) on a second connection: absolute root coordinates for
  edge contacts, key events for hotkeys; hotkey swallowing while home uses passive `GrabKey` on the configured
  combinations. The same RECORD stream serves P4-F4-007's passive mode during foreign pointer grabs. XInput2 bindings
  are not generated. If `briefs/spikes/sp3.md` shows RECORD unavailable or unusable, STOP and ask.
- Rationale: RECORD ships in jezek/xgb (no code generation) and keeps working while another client holds a grab.

### D-93 - GUI <-> agent IPC and config additions (2026-10-30; closes P3 spine gaps 1-4 and 7-10)
- Decision: additive to `proto/schema/ipc.capnp` (new ordinals only), implemented by P3-F-006:
  1. `GetEnvironmentChecks` -> list of `EnvCheck{kind: firewallInbound | trayAvailable, state: ok | problem |
     unsupported, hint: none | windowsFirewallAllow | gnomeAppIndicatorExtension, detail: text}` and event
     `EnvironmentChanged`; agent side `internal/agent/envchecks.go` with an `EnvChecker` interface.
  2. `RemoveManualPeer(host, port)`; manual addresses appear in `ListDiscovered` with source `manual`.
  3. GUI single instance (D-77): the `Hello` result carries `primaryGUI` (false when another GUI is attached); a
     secondary GUI calls `RequestFocusGUI(route)` and exits 0; the agent forwards `FocusGUI{route}` to the primary GUI.
     P1-C5's "a second GUI Hello publishes FocusGUI" behaviour is kept and extended with the route.
  4. Config table `[gui]`: `first_run_completed` (bool, default false; the migration sets it true when the existing
     config already has paired peers) and `wizard_step` (int, default 0), changed through `UpdateConfig`.
  7. Theme on connect: the agent sends `ThemeChanged` with the current theme right after a successful GUI `Hello`.
  8. `PairingProgress` variants: `codeShown(code, expiresAt)`, `awaitingCode(target, expiresAt, attemptsLeft)`,
     `peerJoined(name)`, `attemptFailed(attemptsLeft)`, `succeeded(peer)`, `failed(reason: tooManyAttempts | expired |
     cancelledLocal | cancelledRemote | unreachable | rejected | protocol, detail)`.
  9. Status carries `heldBy` (the server peer currently holding this client) and `rejectedServer` (the last server
     refused with `Busy`, D-59).
  10. GUI IPC reconnect backoff: 250 ms min, 5 s max, factor 2, full jitter (GUI-local).
- Exact files P3 edits (from the P1 items): `internal/ipc/server.go`, `internal/ipc/client.go`,
  `internal/agent/ipc_handler.go`, `internal/agent/ipc_peers.go`, `internal/proto/capnp/ipccp/ipc.capnp.go`
  (regenerated), `internal/proto/msg/ipc.go`, `internal/proto/msg/ipc_types.go`; agent options live in
  `internal/agent/agent.go`; the i18n loader is `internal/i18n/i18n.go`.

### D-94 - Per-area i18n catalogue files (2026-10-30; amends D-68; closes P3 spine gap 5)
- Decision: the catalogue is `internal/i18n/locales/en.toml` (agent-side strings: common, tray, notifications,
  pairing; P1 C5 and P3 G5 add keys with a claim) plus per-area files `internal/i18n/locales/en/<area>.toml`: the GUI
  areas `gui_shell`, `gui_layout`, `gui_machines`, `gui_settings`, `gui_transfers` (P3, per the namespace table in
  `briefs/streams/p3-f.md`), `gui_drop` (P4 F2, keys `drop.*`) and `files` (P4-F, agent keys `files.*` and `dnd.*`).
  The loader (`internal/i18n/i18n.go`, extended by P3-F-005) merges all files; a key defined twice is an error; the
  missing/unused-key test spans all files. Each area file has one owning stream (ownership.md), so UI streams do not
  serialise on one shared file.

### D-95 - GUI log file (2026-10-30; amends D-48; closes P3 spine gap 6)
- Decision: the GUI writes `edgesync-gui.log` (JSON lines) in the D-48 per-user log directory with the same rotation
  (10 MiB x 5) and redaction rules; level from the GUI's `-log-level` flag (default `info`).

### D-96 - internal/firewall package (2026-10-30)
- Decision: new package `internal/firewall` (owner P3 G5, item P3-G5-008): a read-only `Checker` that reports whether
  inbound UDP and TCP rules for the configured port and the installed agent path exist and are enabled. Windows uses
  the `INetFwPolicy2` COM interface through `internal/platform/windows/com`; the agent never creates or changes rules
  (the MSI creates them, P5 R1). Other OSes report state `unsupported`. The package is excluded from the per-package
  gate (D-97); the pure decision logic in `firewall.go` is held to 85% on the Linux CI job and `firewall_windows.go` is
  tested by `integration` tests.

### D-97 - Coverage gate exclusions (2026-10-30; amends the D-37 gate mechanics; closes the P1-F-012, P3-F-001 and P5 coverage gaps)
- Decision: `tools/coverage-exclude.txt` excludes exactly these from the 85% percentage gate: generated code
  `internal/proto/capnp/...` (added by P1-F-012); GUI rendering packages `gui/app`, `gui/shell`, `gui/theme`,
  `gui/widgets`, `gui/widgets/guitest`, `gui/layouteditor`, `gui/peers`, `gui/wizard`, `gui/settings`, `gui/transfers`,
  `gui/logs`, `gui/about`, `gui/drop/...` (added by P3-F-004; every interaction in these packages is covered by named
  `guitest` tests and the UI/UX audit applies); test-only packages `test/e2e` (P3-F-004) and `test/security`
  (P5-R4-006); `internal/firewall` (P3-G5-008; the exclude file is package-granular and the package holds Windows COM
  glue, so its pure evaluation logic is held to 85% by the item's DoD, measured on the Linux CI job where only the pure
  files compile). Included at 85%: `gui/state/...`, `gui/theme/tokens`, `gui/widgets/format`, `gui/shell/vm`, every
  `gui/<area>/vm`, `test/e2e/harness`.

### D-98 - Path sanitiser additions (2026-10-30; amends D-45)
- Decision: `internal/transfer/safepath` (P1-C4-011) additionally rejects, on every receiving OS, components that
  contain `<`, `>`, `:`, `"`, `|`, `?` or `*` (a `:` would create an NTFS alternate data stream on Windows and is
  unportable) and components longer than 255 UTF-16 code units (on top of the 255-byte rule); for Windows targets it
  also rejects device names with superscript digits (`COM¹`-`COM³`, `LPT¹`-`LPT³`, with or without extension), like
  the other reserved names. Within one manifest, entries that collide after case folding and
  Unicode NFC normalisation are not rejected: the later entry is renamed with the D-17 clash rule (" (1)") by
  `internal/transfer/naming.go` (P1-C4-013). A rejected entry fails that entry with an explicit error shown in the
  transfer queue; the rest of the transfer continues.

### D-99 - File-transfer limits for native paste and DnD (2026-10-30; closes P4-F-001 proposals)
- Decision: manifests offered through native paste or drag-and-drop are limited to 100000 entries (a
  `FILEGROUPDESCRIPTORW` for 100000 entries is about 59 MB) and 4096 bytes per relative path (UTF-8); the general
  transfer-engine bound of P1-C4-014 (1000000 entries for GUI sends) is unchanged. Windows virtual-file names (F1)
  reject `<>:"|?*`, control characters and full relative paths longer than 259 UTF-16 units with
  `ErrNotRepresentable` plus the notification `files.notRepresentable`; read-ahead window 8 MiB per open
  `FileContents` / file-promise stream.

### D-100 - depguard and cgo allowances for gui/drop (2026-10-30; amends D-55; closes P4-F-001 proposals)
- Decision: `gui/drop/*_windows.go` may import `internal/platform/windows/com` and `internal/platform/windows/win32`;
  `gui/drop/*_x11.go`, `gui/drop/proxy_x11.go` and `gui/drop/drop_linux.go` may import `github.com/jezek/xgb/...`.
  cgo is allowed in `gui/drop` on darwin (`drop_darwin.m`, Objective-C) and on Linux Wayland (`drop_wayland_c.go`,
  libwayland-client), within D-34 ("the GUI may use cgo on macOS/Linux"). The agent stays cgo-free. P4-F-002 applies
  the `.golangci.yml` change (claim) and runs `golangci-lint config verify`.

### D-101 - File contract source of truth (2026-10-30; closes P4 point 6)
- Decision: section "The file contract" in `briefs/streams/p4-f.md` is the authoritative draft of architecture 4.9 (it
  adds the `IncomingDrag` handle, `SetEdges`, `Arm`, `FinishOutgoing`, optional source interfaces and `FormatFiles`);
  staging lives in `internal/transfer` (avoids an import cycle with `internal/vfile`). Architecture 4.9 carries a note
  pointing there; P4-F-023 freezes the final contract.

### D-102 - Forward requirements on P2 backends from P4 (2026-10-30; closes P4 point 7)
- Decision: P2 implements these so P4 does not reopen P2 code:
  W1 - while grabbed, the capturer parks the hidden cursor at the edge contact point (1x1 `ClipCursor` there, deltas
  from Raw Input per D-50); if SP1 selects re-centring instead, STOP and ask.
  W2 - the clipboard backend never announces an update whose only formats are virtual-file formats
  (`FileGroupDescriptorW` / `FileContents` without `CF_HDROP`).
  X1 - the INCR receive and send helpers in package `internal/platform/x11` (`incrrecv.go`, `incrsend.go`) are exported
  for P4-F4.
  M1 - `appkit.Do` (main-thread dispatcher) and `clipboard.PollInterval` (500 ms, D-86) are exported for P4-F3.
  Y1 - the portal Clipboard session object is exported from the Wayland backend for P4-F5.
  P4-F4-007 still edits X1's capturer files for passive mode (D-92) and claims them explicitly.

### D-103 - In-house Wayland client codec for drag-and-drop (2026-10-30; closes P4 point 10)
- Decision: agent-side Wayland DnD needs a Wayland client; it is a pure-Go wire codec generated from the MIT-licensed
  `wayland.xml` (vendored under `third_party/wayland/` with its licence) by P4-F5-004/005, keeping the agent cgo-free
  (D-34). The GUI drop target uses libwayland through cgo (D-100). Risk R-54.

### D-104 - Release targets, CI runners and glibc floor (2026-10-30; replaces the P5-R2 proposal (d) and P5-R1 (f) "x64 only")
- Decision: release targets windows/amd64 and windows/arm64 (one MSI each), macOS universal (arm64 + amd64 via `lipo`),
  linux/amd64 and linux/arm64. CI runners (checked in the actions/runner-images README on 2026-10-30): `windows-latest`
  and `windows-11-arm`; `macos-latest` (arm64) and `macos-15-intel`; `ubuntu-22.04` and `ubuntu-22.04-arm` for Linux
  release builds; `ubuntu-24.04` for lint, cross, gen-check, fuzz and Xvfb jobs. Linux release binaries are built on
  ubuntu-22.04 images, so the GUI needs glibc >= 2.35 (the agent is static). arm64 Windows and Linux are verified on CI
  only (R-55).
- Rationale: the runners exist; glibc 2.35 covers Ubuntu 22.04 and Debian 12 X11 sessions.
<!-- NOTE: 2026-10-30 superseded in part by D-118 (one installer per architecture instead of one MSI; targets and runners unchanged) -->

### D-105 - Windows subsystem flags (2026-10-30)
- Decision: `edgesync.exe` (GUI) and `edgesync-agent.exe` are linked with `-ldflags "-H windowsgui"` in every Windows
  build outside tests (Taskfile build tasks, CI release jobs); `edgesync-service.exe` keeps the console subsystem (its
  `install`/`uninstall` subcommands print to the console). The agent's `-version` output goes to the log.

### D-106 - macOS bundle layout and signing (2026-10-30; clarifies D-26)
- Decision: `EdgeSync.app` (GUI, bundle id `io.github.kikz0r.edgesync`) contains the agent as its own bundle
  `Contents/Helpers/EdgeSync Agent.app` (bundle id `io.github.kikz0r.edgesync.agent`, `LSUIElement` true), so TCC lists
  "EdgeSync Agent" for Accessibility, Input Monitoring and Local Network. The GUI starts the agent with
  `/usr/bin/open -g -j "<agent bundle>"`; the M1 LaunchAgent points at the executable inside the nested bundle. Agent
  Info.plist: `NSLocalNetworkUsageDescription`, `NSBonjourServices = ["_edgesync._udp"]`,
  `NSDownloadsFolderUsageDescription`, `NSDesktopFolderUsageDescription`, `NSDocumentsFolderUsageDescription`. Both
  bundles get an ad-hoc signature (`codesign --force --sign -`): arm64 macOS refuses unsigned code and `lipo` output must
  be re-signed. "Unsigned" in D-26 means no Developer ID certificate and no notarization. Consequence: macOS ties
  permission grants to the code hash, so permissions must be granted again after every update (Q-06).

### D-107 - Release publishing and asset names (2026-10-30)
- Decision: GoReleaser OSS v2.19.0 only publishes: `builds` skipped (the prebuilt builder is Pro-only, checked in the
  GoReleaser docs on 2026-10-30); packages built on native runners are attached with `release.extra_files` and added to
  the checksum file with `checksum.extra_files` (available in OSS; only `templated_extra_files` is Pro, checked
  2026-10-30); GoReleaser writes the changelog and a draft release that the user publishes. Invoked as
  `go run github.com/goreleaser/goreleaser/v2@v2.19.0` locally and via `goreleaser/goreleaser-action` with
  `version: v2.19.0` in CI. Asset names: `EdgeSync-<ver>-windows-amd64.msi`, `EdgeSync-<ver>-windows-arm64.msi`,
  `EdgeSync-<ver>-macos-universal.zip`, `EdgeSync-<ver>-linux-amd64.tar.gz`, `EdgeSync-<ver>-linux-arm64.tar.gz`,
  `checksums.txt`. Linux `install.sh` defaults to `~/.local` (no root); `--system` installs to `/usr/local` (sudo); the
  shipped autostart entry uses the X1 Autostart backend's file name.
<!-- NOTE: 2026-10-30 superseded in part by D-118 (Windows asset names are EdgeSync-<ver>-windows-<arch>-setup.exe) -->

### D-108 - Windows installer details (2026-10-30; closes the P5-R1 design points b-f)
- Decision: the MSI uses msiexec's basic UI (progress only; no WixUI dialogs, so no `WixToolset.UI.wixext`).
  `SoftwareSASGeneration` under `HKLM\SOFTWARE\Microsoft\Windows\CurrentVersion\Policies\System`: absent or 0 -> write 1
  (removed on uninstall), 2 -> write 3 (left on uninstall), 1 or 3 -> untouched (Group Policy may override, R-05).
  Uninstall keeps `%ProgramData%\EdgeSync` and per-user data so a reinstall keeps pairings (`docs/install.md` lists the
  paths). Running `edgesync-agent.exe` and `edgesync.exe` are closed with `util:CloseApplication` (WM_CLOSE, then
  termination after 5 s) and the agent shuts down gracefully on WM_CLOSE / WM_ENDSESSION. Fixed install directory
  `%ProgramFiles%\EdgeSync` (D-47 authorises service clients by that path), per-machine, one MSI per architecture
  (D-104). Machine-wide autostart is Q-05.
<!-- NOTE: 2026-10-30 superseded in part by D-118 (msiexec basic UI and util:CloseApplication are replaced by Inno Setup mechanisms; SAS rules, keep-data rule and the fixed directory stay) -->

### D-109 - Performance budgets PB-25..PB-27 (2026-10-30)
- Decision: PB-25 file paste time-to-first-byte (P4): loopback p95 <= 50 ms, LAN-W p95 <= 150 ms. PB-26 soak memory
  growth (P5 R3): LAN-W 8 h soak, agent RSS slope over hours 1-8 <= 1 MiB/h on both agents with peak RSS within PB-17;
  loopback CI soak (30 min after a 5 min warm-up) heap-in-use slope <= 2 MiB/h. PB-27 soak handle growth: LAN-W Windows
  handle-count slope <= 5 handles/h; loopback goroutine count at the end <= the count after warm-up + 5; Linux fd-count
  slope <= 1/h when the GNOME box takes part. P5 items that called the soak budgets PB-25/PB-26 now use PB-26/PB-27.

### D-110 - SPAKE2 password-to-scalar derivation (2026-10-30; confirms D-14)
- Decision: the 6-digit code becomes the SPAKE2 password scalar exactly as specified in the P1-C2-005 brief (HKDF-SHA256
  with salt "EdgeSync SPAKE2 w v1" and the identities as info, reduced mod n). No user-visible choice is involved;
  P1-C2-016 and P5-R4-004 review it against RFC 9382.

### D-111 - Superseded plan items (2026-10-30)
- Decision: when an approved decision replaces a plan item, the parent toggles that item to `[x]` and appends on the
  next line `<!-- NOTE: <date> - superseded by D-NN; replacement items <IDs> -->`. A superseded item never counts as
  delivered work in FEAT audits (the replacement items carry the FEAT tags). Toggling keeps the statusline's open-item
  count honest, because the statusline counts every unchecked box as open work. Recorded in `plan.md` section 5.

---

## Product answers (D-112 ..) - 2026-10-30 - user Q&A, `briefs/open-questions.md` Q-01 .. Q-10

One decision per answer (Q-NN -> D-(111+NN)). Superseded text stays in place and carries an inline NOTE.

### D-112 - Control at the Windows sign-in screen: per-server opt-in (2026-10-30; answers Q-01; supersedes D-47 (part); amends D-64, D-69)
- Decision: option A. Pre-login control (D-64) is opt-in per paired server. The Machines page shows, on a Windows machine
  with the EdgeSync service installed, a per-server switch "Allow control at the Windows sign-in screen". Turning it on or
  off runs the elevated command `edgesync-service allow-prelogin <peer-id>` (add) or
  `edgesync-service allow-prelogin --remove <peer-id>` (remove), started by the GUI with the `runas` verb (one UAC prompt per
  change). The command requires an elevated token, validates the peer ID (52 lowercase base32 characters, D-42; the peer ID is
  the SPKI pin, so nothing else needs copying), and updates `%ProgramData%\EdgeSync\machine-peers.toml` atomically
  (`schema_version = 1`, `[[peer]]` with `id`, `name`, `added_at`; strict decoding; DACL SYSTEM and Administrators only).
  Pre-login mode accepts only servers listed there and re-reads the file at every incoming session. D-47 gains one read-only
  whitelist entry `GetPreloginPeers` (returns the peer IDs, changes nothing) so the unprivileged agent and GUI can show the
  switch state; no service-pipe command changes configuration. Unpairing a server that is allowed at the sign-in screen first
  runs the removal (the unpair is cancelled when the user declines the UAC prompt). The file is machine-scoped: it applies to
  every Windows account on the PC (D-41), which `docs/security.md` states. Uninstall keeps the file (D-108 intent).
- Rationale: explicit trust escalation with an auditable admin consent; the per-user peer stores stay unreadable by the service.
- Alternatives considered: B (every server paired by any user is trusted at the sign-in screen automatically), C (no
  sign-in-screen control in v1, FEAT-049 dropped).
- Supersedes D-47 (part: the fixed whitelist gains the read-only `GetPreloginPeers`); amends D-64 (how the machine-scoped peer store is populated) and D-69 (adds `machine-peers.toml`).
- Source: user Q&A, `briefs/open-questions.md` Q-01.

### D-113 - Ctrl+Alt+Del from a Windows server (2026-10-30; answers Q-02; amends D-58)
- Decision: option A. New hotkey action "Send Ctrl+Alt+Del", config key `[hotkeys] send_cad`, default binding Ctrl+Alt+End (the
  Remote Desktop convention), rebindable like every D-58 hotkey. While the active client is a Windows machine whose handshake
  reports `os = "windows"` and `capabilities.secureDesktop = true` (the agent sets the capability, P3-G5-021), the server engine sends the chord (Ctrl down, Alt down,
  Delete down and up, Alt up, Ctrl up, physical HID usages) on the reliable input stream; the chord is swallowed locally like
  every hotkey. The client's helper-mode injector recognises the chord and calls `RequestSAS` (P2-W2-029). A tray and GUI
  action "Send Ctrl+Alt+Del to <machine>" (IPC `SendCtrlAltDel{peer}`) is offered for connected Windows clients with the
  capability; when the target is not the active machine the engine jumps to it first (guards skipped) and then sends the chord.
  The reserved-combo list keeps warning about Ctrl+Alt+Delete itself.
- Rationale: Windows never delivers a physical Ctrl+Alt+Del to user-mode hooks (G-08); the chord reuses the existing W2 path
  with no new wire message.
- Alternatives considered: B (tray/GUI action only), C (hotkey only, Ctrl+Alt+Insert).
- Amends D-58 (adds the Send Ctrl+Alt+Del binding); supersedes nothing else.
- Source: user Q&A, `briefs/open-questions.md` Q-02.

### D-114 - Hotkeys on a Wayland server while the pointer is home (2026-10-30; answers Q-03; supersedes D-06 and D-58 (part, Wayland servers))
- Decision: option A. On GNOME and KDE Wayland the Y1 backend uses the xdg-desktop-portal
  `org.freedesktop.portal.GlobalShortcuts` interface (CreateSession, BindShortcuts with shortcut ID plus `description` and
  `preferred_trigger`, ListShortcuts, signals Activated, Deactivated and ShortcutsChanged, `version` property; checked against
  the portal documentation on 2026-10-30) for lock-to-screen and panic/return-home while the pointer is on the server. The
  compositor or portal asks the user to confirm the shortcuts; the triggers it actually bound are read back and shown in the
  GUI. Optional platform interface `platform.HomeShortcutProvider` in the new file `internal/platform/homeshortcuts.go`
  (P1-F-023, added before the contract freeze) is implemented by the Wayland capturer; the engine registers the lock and panic
  bindings when the capturer implements it and runs the same actions on activation. Jump-to-machine and cycle from home stay
  unavailable on Wayland servers (a `Grab` without a barrier activation returns `platform.ErrUnsupported`, G-15); the GUI hotkey
  tab and the docs state the limitation. Capability detection reports `unavailable` with a reason when the interface is
  missing (GNOME 45-47 support is UNVERIFIED, R-56); the tray menu remains the way to lock from home. While the pointer is on a
  client, every hotkey is recognised from the captured key stream as before.
- Rationale: gives Wayland servers the two hotkeys that matter at home without any compositor-specific code.
- Alternatives considered: B (accept the limitation entirely; lock from home via the tray only).
- Supersedes D-06 (part) and D-58 (part): the statement that hotkeys are recognised from the captured key stream whether the cursor is home or remote does not hold on Wayland servers.
- Source: user Q&A, `briefs/open-questions.md` Q-03.

### D-115 - Locking macOS clients (2026-10-30; answers Q-04; confirms D-71)
- Decision: option A. The macOS session backend locks with the private `SACLockScreenImmediate` from
  `/System/Library/PrivateFrameworks/login.framework/login` (resolved with dlsym). Availability is detected when the backend
  opens: when the symbol is missing the backend logs a Warn "lock unsupported on this Mac", `Lock` returns an error wrapping
  `platform.ErrUnsupported`, and the agent reports an environment check `lockScreen` (state `problem`, hint
  `macLockUnsupported`) that the GUI shows on that Mac. There is no silent fallback (no screensaver substitute). Risk R-52
  becomes active; the behaviour cannot be hand-tested (R-07).
- Rationale: the only reliable "lock now" on macOS; absence is surfaced instead of hidden.
- Alternatives considered: B (start the screensaver instead), C (macOS clients ignore lock propagation).
- Supersedes nothing; confirms D-71 and adds the availability detection and the `lockScreen` check.
- Source: user Q&A, `briefs/open-questions.md` Q-04.

### D-116 - Autostart with the per-machine Windows installer (2026-10-30; answers Q-05; supersedes D-21 (part))
- Decision: option A. The installer writes `HKLM\SOFTWARE\Microsoft\Windows\CurrentVersion\Run\<W1 value name>` =
  `"<install dir>\edgesync-agent.exe" --autostart` for all users. Each user's toggle lives in
  `HKCU\Software\EdgeSync\Autostart` (DWORD 1 on, 0 off, missing = on). An agent started with `--autostart` exits at once,
  before any other initialisation, when that user opted out. In machine mode (the HKLM value points at this executable) the
  agent deletes its own HKCU Run value so it never starts twice, and the GUI toggle writes the HKCU value. Without the HKLM
  value (development builds) the D-21 per-user HKCU Run behaviour is unchanged. Implemented by P5-R1-006 (the HKLM value) and
  P5-R1-008 (gate and toggle).
- Rationale: every Windows account on the PC autostarts EdgeSync without another per-user step, and each user can opt out.
- Alternatives considered: B (each user's agent registers its own HKCU Run value on first start), C (autostart only for the
  account that ran the installer).
- Supersedes D-21 (part): where the per-user autostart toggle lives when the machine-wide installer entry exists.
- Source: user Q&A, `briefs/open-questions.md` Q-05.

### D-117 - macOS permissions after updates (2026-10-30; answers Q-06; confirms D-26 and D-106)
- Decision: option A. For v1 the ad-hoc signature (D-106) is accepted: macOS ties Accessibility, Input Monitoring and Local
  Network grants to the code signature, so after every EdgeSync update the user grants them again (System Settings, then
  restart the agent). The first-run wizard permissions step, the permissions page and the docs (`docs/install.md`,
  `docs/troubleshooting.md`) explain this. No Apple Developer ID, signing or notarization work in P5 R2.
- Rationale: no paid Apple account; R-14 and R-53 record the cost.
- Alternatives considered: B (Apple Developer ID and notarization now, which would supersede D-26 for macOS).
- Supersedes nothing; confirms D-26 and D-106 and records the acceptance that Q-06 asked for.
- Source: user Q&A, `briefs/open-questions.md` Q-06.

### D-118 - Windows installer: Inno Setup 7.1.1 (2026-10-30; answers Q-07; supersedes D-65, and D-25, D-63, D-104, D-107, D-108 (the WiX/MSI parts))
- Decision: the user chose Inno Setup for the Windows installer instead of WiX/MSI. Pinned: Inno Setup 7.1.1 (jrsoftware.org
  `innosetup-7.1.1-x64.exe`, dated 2026-10-28; 6.7.4 is also offered and is not used). winget package id
  `JRSoftware.InnoSetup.7`, version 7.1.1, installer SHA-256
  `8a5a6711c3766a6e5fb74f280ba0e3551e719ce94538d901fdbd1f8f9920b3f0` (read-only `winget show JRSoftware.InnoSetup.7` on
  2026-10-30). Note: `winget show JRSoftware.InnoSetup` reports Inno Setup 6, version 6.7.3, which is the wrong package for this
  pin. Licence (verified by the user 2026-10-30): permissive zlib-style terms that permit commercial use; commercial users with
  annual revenue above USD 5000 are requested to buy a licence, which the FAQ calls not strictly required (risk R-37). GitHub's
  windows-2025 image ships Inno Setup 6.7.1 (the user's check), which is not relied upon: CI downloads the pinned installer from the GitHub
  release `is-7_1_1`, verifies the SHA-256 above, installs it silently on the runner and checks `ISCC.exe --version`; locally
  P0-F-003 (USER-CONFIRM) installs it with winget.
- Design (items in P5 R1): one script `packaging/windows/edgesync.iss` compiled once per architecture by
  `packaging/windows/isbuild` (D-104 targets unchanged); stable `AppId` GUID, `PrivilegesRequired=admin`, `SetupArchitecture=x64`
  with `ArchitecturesAllowed` and `ArchitecturesInstallIn64BitMode` set to `x64os` for the amd64 installer and `arm64` for the
  arm64 installer (identifiers per the Inno Setup help; `x64compatible` would also match Arm64 Windows); fixed directory
  `%ProgramFiles%\EdgeSync` (D-47; `DisableDirPage=yes`); upgrade in place through the same `AppId`; the service is installed
  and removed by our own `edgesync-service install|uninstall` subcommands (x/sys/windows/svc/mgr, recovery options included)
  invoked from `[Run]` and `[UninstallRun]`, not by sc.exe; firewall rules by `edgesync-service firewall add|remove`;
  `SoftwareSASGeneration` and the HKLM Run autostart value (D-116) in `[Registry]`; `PrepareToInstall` stops and removes the
  running service before files are replaced and Restart Manager (`CloseApplications`) closes the agent and the GUI gracefully
  (replaces `util:CloseApplication`); the D-108 rules for the SAS policy values, for keeping `%ProgramData%\EdgeSync` and the
  per-user data on uninstall, and for the fixed install directory are unchanged; Setup uses its normal wizard (no MSI basic-UI
  mode); version info and the downgrade refusal come from the release version (D-78); a SignTool step is passed to ISCC only
  when a certificate is configured (D-26). Output name `EdgeSync-<ver>-windows-<arch>-setup.exe` (replaces the `.msi` names of
  D-107; `packaging/release/relver` and the release workflow follow). Validation: ISCC compile in CI plus an install, check,
  upgrade, uninstall lifecycle job on the runners only (never on the dev host), and the manual script on the user's Windows PCs.
- Rationale: user choice; removes the WiX MS-RL versus OSMF EULA question (Q-07 context) and the .NET SDK dependency.
- Alternatives considered: WiX 5.0.2 (D-65, MS-RL), WiX 7.0.1 (OSMF EULA, possible fee); both rejected by the user.
- Supersedes D-65 (whole), and the WiX/MSI parts of D-25, D-63, D-104, D-107 and D-108.
- Source: user Q&A, `briefs/open-questions.md` Q-07.

### D-119 - LICENSE copyright holder (2026-10-30; answers Q-08; confirms D-79)
- Decision: option A. LICENSE reads "Copyright (c) 2026 kikz0r" (D-79 confirmed; P0-F-004 and THIRD_PARTY_NOTICES unchanged).
- Alternatives considered: B (a legal name or organisation).
- Supersedes nothing; confirms D-79.
- Source: user Q&A, `briefs/open-questions.md` Q-08.

### D-120 - Cursor mapping across machines of different sizes (2026-10-30; answers Q-09; confirms D-04 and D-51)
- Decision: option A. Geometric continuity (D-51): the cursor crosses only where the two screens' real edges overlap on the
  layout canvas and keeps its canvas position; outside the overlap the edge is a wall. P1-C3-007 and the G1 layout guides stay
  as written.
- Alternatives considered: B (stretch the source overlap onto the target overlap).
- Supersedes nothing; confirms D-04 and D-51.
- Source: user Q&A, `briefs/open-questions.md` Q-09.

### D-121 - Clipboard privacy: concealed content is shared (2026-10-30; answers Q-10; supersedes D-61 (part))
- Decision: option B. Clipboard content that its source marks as concealed or sensitive (Windows
  `ExcludeClipboardContentFromMonitorProcessing` and `CanIncludeInClipboardHistory` = 0, macOS `org.nspasteboard.ConcealedType`,
  `x-kde-passwordManagerHint`) is announced and shared like any other content. Backends no longer detect sensitivity and the
  `Sensitive` field is removed from `platform.LocalChange` (before the contract freeze); the clipboard manager has no
  sensitivity filter. Unchanged from D-61: only the CLIPBOARD selection syncs (no PRIMARY), the master toggle and the per-format
  toggles (all on by default), echo prevention, and clipboard contents are never logged at any level (D-48). The data still
  moves only when a receiving application pastes (D-10), so a copied password reaches another machine only when it is pasted
  there. `docs/security.md`, the settings page text and `qa/security-review.md` state that password-manager content is shared
  and that the master toggle turns sharing off. The D-102 rule (an update whose only formats are virtual-file formats is not
  announced) is not about sensitivity and stays.
- Rationale: user choice; the behaviour is predictable and each user controls it with the toggles.
- Alternatives considered: A (as locked in D-61), C (as A plus PRIMARY selection sync between Linux machines).
- Supersedes D-61 (part: the rule that concealed or sensitive content is never announced).
- Source: user Q&A, `briefs/open-questions.md` Q-10.

### D-122 - gofumpt extra rules via the non-deprecated keys (2026-10-30; amends D-55; locked by orchestrator - user to review)
- Decision: in `.golangci.yml` the formatter setting `gofumpt: { extra-rules: true }` is replaced by
  `gofumpt: { extra: { group-params: true, clothe-returns: true } }`. Behaviour is identical: gofumpt v0.12.0's `-extra` flag
  consists of exactly two rules (group adjacent parameters of the same type; avoid naked returns), and golangci-lint v2.15.0
  prints a deprecation warning on every run for the old key and offers the two rules as separate keys (verified 2026-10-30
  against the gofumpt v0.12.0 README and the golangci-lint v2.15.0 JSON schema).
- Rationale: P0-F-007 implemented D-55 literally and every lint run now prints the warning twice; the warning would train
  agents and people to ignore lint output.
- Alternatives considered: keep the deprecated key (noise, and the key can be removed in a later v2 release; rejected).
- Amends D-55 (the extra-rules setting only). Implemented by P0-F-011.
- Source: orchestrator verification of the P0-F-007 result.
