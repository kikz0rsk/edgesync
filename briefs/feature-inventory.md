# EdgeSync - Feature inventory

Every feature and sub-feature from the user's spec (GUI, shared clipboard, seamless edge control, file transfer
including copy-paste) and every locked decision, with a stable ID. Item lines in `briefs/plan-phase-*.md` carry
these IDs; a FEAT with no item in any phase file means the plan is incomplete.

"Delivered by" names the phase and stream whose items implement the feature; "Also" lists contributing streams.
Stream codes: P0-F, SP1..SP5 (P0); P1-F, C1..C5 (P1); W1, W2, X1, Y1, M1 (P2); P3-F, G1..G5 (P3); P4-F, F1..F5 (P4); R1..R5 (P5).

## A. Roles, topology, layout

| FEAT | Feature | Decisions | Delivered by | Also |
|---|---|---|---|---|
| FEAT-001 | Switchable role (server/client) per install, persisted, switchable without restart | D-03 | P1 C5 | P3 G3 |
| FEAT-002 | Star topology: every agent listens, server dials paired layout members, client holds one server session, `Busy` rejection | D-59 | P1 C1 | P3 G5 |
| FEAT-003 | Layout model: machines as rigid monitor groups on a logical canvas, up to 64 machines | D-04, D-30, D-51 | P1 C3 | P3 G1 |
| FEAT-004 | Real-edge computation for multi-monitor desktops (L-shapes, gaps, mixed DPI) | D-51 | P1 C3 | - |
| FEAT-005 | Crossing mapping inside edge overlaps (canvas continuity, scale conversion), walls elsewhere | D-04, D-51 | P1 C3 | - |
| FEAT-006 | Layout validation (overlaps, unreachable machines, duplicates) and snapping helpers | D-04 | P1 C3 | P3 G1 |
| FEAT-007 | Layout authority: server owns the layout and pushes `LayoutSnapshot`; clients read-only | D-60 | P1 C1 | P1 C3, P3 G1 |
| FEAT-008 | Desktop change propagation (client monitor change updates server geometry) | D-51 | P1 C1 | P1 C3 |

## B. Engine and input semantics

| FEAT | Feature | Decisions | Delivered by | Also |
|---|---|---|---|---|
| FEAT-009 | Server engine state machine: home/remote, switch, return, virtual cursor | D-49 | P1 C3 | - |
| FEAT-010 | Client engine: Enter/Leave handling, injection, cursor show, overlay trigger | D-49 | P1 C3 | - |
| FEAT-011 | Guard: dwell delay | D-05, D-57 | P1 C3 | P3 G3 |
| FEAT-012 | Guard: double-tap edge | D-05, D-57 | P1 C3 | P3 G3 |
| FEAT-013 | Guard: corner dead zones | D-05, D-57 | P1 C3 | P3 G3 |
| FEAT-014 | Guard: modifier required | D-05, D-57 | P1 C3 | P3 G3 |
| FEAT-015 | Hotkey: lock to screen | D-06, D-58 | P1 C3 | - |
| FEAT-016 | Hotkey: jump to machine | D-06, D-58 | P1 C3 | - |
| FEAT-017 | Hotkey: cycle next/previous | D-06, D-58 | P1 C3 | - |
| FEAT-018 | Hotkey: panic/return home with release-all | D-06, D-58 | P1 C3 | - |
| FEAT-019 | Hotkey binding model: parse/format, conflicts, OS-reserved warnings, swallowing | D-06, D-58 | P1 C3 | P3 G3 |
| FEAT-020 | Physical key transport: HID usage, key-state tracking, release by physical id | D-08, D-49 | P1 C3 | - |
| FEAT-021 | Keymap table HID <-> Windows scancodes | D-08, D-27 | P1 C3 | - |
| FEAT-022 | Keymap table HID <-> macOS virtual keycodes | D-08 | P1 C3 | - |
| FEAT-023 | Keymap table HID <-> evdev (X11 keycode, EI) | D-08 | P1 C3 | - |
| FEAT-024 | Keymap generator from Chromium `dom_code_data.inc` with BSD notice in `third_party/` | D-27, D-67 | P1 C3 | - |
| FEAT-025 | Per-machine modifier remap (macOS default Win<->Cmd, Alt<->Option) | D-08 | P1 C3 | P3 G3 |
| FEAT-026 | `ReleaseAll` + `ModSync` on every switch | D-49 | P1 C3 | - |
| FEAT-027 | Motion as absolute coordinates via latest-wins datagrams with sequence numbers, scale-ratio speed | D-49 | P1 C3 | P1 C1 |
| FEAT-028 | Motion coalescing (1 ms) for 1000 Hz mice | D-29, D-57 | P1 C3 | - |
| FEAT-029 | Mouse buttons incl. X1/X2 | D-49 | P1 C3 | P2 W1, M1, X1, Y1 |
| FEAT-030 | High-resolution vertical + horizontal wheel | D-49 | P1 C3 | P2 W1, M1, X1, Y1 |
| FEAT-031 | Return home when the active client is lost or disconnects | D-49, D-57 | P1 C3 | P3 G5 |
| FEAT-032 | Edge towards an offline neighbour acts as a wall | D-70 | P1 C3 | - |

## C. Windows platform

| FEAT | Feature | Decisions | Delivered by | Also |
|---|---|---|---|---|
| FEAT-033 | Capture: LL hooks on a locked thread, ring buffer, watchdog re-install, Raw Input deltas | D-50 | P2 W1 | P0 SP1 |
| FEAT-034 | Grab: suppress local delivery, hide/park cursor, ungrab placement | D-50 | P2 W1 | P0 SP1 |
| FEAT-035 | Direct injection: `SendInput` ABSOLUTE+VIRTUALDESK, scancodes + extended keys, wheel/hwheel, X buttons | D-49, D-63 | P2 W1 | P0 SP1 |
| FEAT-036 | Displays: monitor enumeration, per-monitor DPI, change watch | D-51 | P2 W1 | - |
| FEAT-037 | Exe resources: PerMonitorV2 manifest, version info, icons (`.syso`) | D-66 | P2 W1 | - |
| FEAT-038 | Session events: lock/unlock, screensaver, display power, suspend, secure-desktop detection | D-71 | P2 W1 | - |
| FEAT-039 | Tray icon + menu (`Shell_NotifyIcon`) | D-18 | P2 W1 | - |
| FEAT-040 | Switch overlay (layered topmost window) | D-18, D-57 | P2 W1 | - |
| FEAT-041 | Autostart (HKCU Run key; under the installer a machine-wide HKLM entry with a per-user HKCU opt-out) | D-21, D-116 | P2 W1 | P5 R1 |
| FEAT-042 | Theme detection | D-52 | P2 W1 | - |
| FEAT-043 | Clipboard backend: format listener, delayed rendering, text / CF_HTML / RTF / PNG + DIBV5 | D-10, D-40, D-61, D-121 | P2 W2 | P0 SP2 |
| FEAT-044 | `edgesync-service`: SYSTEM service, session tracking, machine config and logs | D-07, D-63 | P2 W2 | P0 SP1 |
| FEAT-045 | Secure helper launch (winlogon token + `TokenUIAccess`), input-desktop following, restart | D-63 | P2 W2 | P0 SP1 |
| FEAT-046 | Helper-routed injection (UIPI-free: UAC, lock screen, elevated windows) | D-07, D-63 | P2 W2 | - |
| FEAT-047 | Secure-desktop capture via helper; auto return home when the server's secure desktop activates | D-63 | P2 W2 | - |
| FEAT-048 | Ctrl+Alt+Del via `RequestSAS` -> `SendSAS` | D-07, D-63 | P2 W2 | P5 R1 |
| FEAT-049 | Pre-login client mode (machine identity, injection only, servers from the machine peer store) | D-64, D-112 | P2 W2 | - |
| FEAT-050 | Service IPC: SDDL, per-request authorisation, whitelist, audit logging | D-47 | P2 W2 | P5 R4 |
| FEAT-051 | Machine-scoped identity + TLS signing proxy over the service pipe | D-41, D-47 | P2 W2 | P1 C2 |

## D. macOS platform

| FEAT | Feature | Decisions | Delivered by | Also |
|---|---|---|---|---|
| FEAT-052 | Capture: CGEventTap (session tap) on a locked thread + CFRunLoop, re-enable on disable events | D-39 | P2 M1 | P0 SP4 |
| FEAT-053 | Grab: cursor dissociation, hiding (incl. background), park/warp | D-39 | P2 M1 | P0 SP4 |
| FEAT-054 | Injection: `CGEventPost` with deltas, click counts/event numbers, modifier flags | D-39 | P2 M1 | P0 SP4 |
| FEAT-055 | Displays: active display list, point/pixel scale, reconfiguration callback | D-51 | P2 M1 | - |
| FEAT-056 | Permissions: Accessibility + Input Monitoring status and prompt | D-39 | P2 M1 | P3 G2 |
| FEAT-057 | Clipboard backend: changeCount poll, `NSPasteboardItemDataProvider` lazy, text/html/rtf/png | D-10, D-40, D-61, D-121 | P2 M1 | P0 SP4 |
| FEAT-058 | Session events | D-71 | P2 M1 | - |
| FEAT-059 | Tray (`NSStatusItem`, accessory activation policy) | D-18 | P2 M1 | P0 SP4 |
| FEAT-060 | Switch overlay (borderless panel) | D-18 | P2 M1 | P0 SP4 |
| FEAT-061 | Autostart (LaunchAgent plist) | D-21 | P2 M1 | - |
| FEAT-062 | Theme detection | D-52 | P2 M1 | - |

## E. Linux X11 and shared Linux

| FEAT | Feature | Decisions | Delivered by | Also |
|---|---|---|---|---|
| FEAT-063 | X11 capture: grabs on an override-redirect window, deltas (warp-to-centre or XI2 per SP3), edge contacts | D-39 | P2 X1 | P0 SP3 |
| FEAT-064 | X11 injection via XTest | D-39 | P2 X1 | P0 SP3 |
| FEAT-065 | X11 displays via RandR monitors + change notify | D-51 | P2 X1 | - |
| FEAT-066 | X11 keymap verification (`GetKeyboardMapping`) for evdev mapping | D-08 | P2 X1 | - |
| FEAT-067 | X11 clipboard: XFixes notify, selection ownership, INCR, CLIPBOARD only | D-10, D-61 | P2 X1 | P0 SP3 |
| FEAT-068 | Linux tray via StatusNotifierItem D-Bus (+ GNOME AppIndicator extension detection and guidance) | D-18 | P2 X1 | P2 Y1, P3 G2 |
| FEAT-069 | X11 switch overlay (override-redirect window) | D-18 | P2 X1 | - |
| FEAT-070 | Linux autostart (XDG autostart `.desktop`) | D-21 | P2 X1 | P5 R2 |
| FEAT-071 | Linux theme detection via the Settings portal | D-52 | P2 X1 | - |
| FEAT-072 | Linux session events (logind, `org.freedesktop.ScreenSaver`, GNOME ScreenSaver) | D-71 | P2 X1 | P2 Y1 |

## F. Linux Wayland (GNOME 45+, KDE Plasma 6.1+)

| FEAT | Feature | Decisions | Delivered by | Also |
|---|---|---|---|---|
| FEAT-073 | Pure-Go EI wire codec (handshake, connection, seat, device, pointer, pointer_absolute, button, scroll, keyboard, fd passing) | D-39 | P2 Y1 | P0 SP3 |
| FEAT-074 | Capture via InputCapture portal: zones, barriers, activation, release, session recreation | D-02, D-39 | P2 Y1 | P0 SP3 |
| FEAT-075 | Injection via RemoteDesktop portal + EI | D-39 | P2 Y1 | P0 SP3 |
| FEAT-076 | Displays from portal zones | D-51 | P2 Y1 | - |
| FEAT-077 | Portal consent, restore-token persistence, degraded-mode reporting | D-02 | P2 Y1 | P3 G2 |
| FEAT-078 | Clipboard via the Clipboard portal inside the active session | D-10 | P2 Y1 | P0 SP3 |
| FEAT-079 | Switch indicator on Wayland (mechanism from SP3, user-approved) | D-18 | P2 Y1 | P0 SP3 |

## G. Clipboard core

| FEAT | Feature | Decisions | Delivered by | Also |
|---|---|---|---|---|
| FEAT-080 | Format model, canonical MIME set, size info | D-09 | P1 C4 | - |
| FEAT-081 | Lazy protocol: announce, fetch, supersede, timeouts, explicit errors | D-10, D-57 | P1 C4 | - |
| FEAT-082 | Echo prevention via origin + sequence | D-61 | P1 C4 | - |
| FEAT-083 | Text normalisation (UTF-8, per-OS line endings) | D-09 | P1 C4 | - |
| FEAT-084 | HTML <-> CF_HTML conversion | D-09 | P1 C4 | - |
| FEAT-085 | RTF passthrough | D-09 | P1 C4 | - |
| FEAT-086 | Image conversion PNG <-> DIB/DIBV5 <-> TIFF | D-09 | P1 C4 | - |
| FEAT-087 | 64 MiB per-format payload cap with clear errors | D-57 | P1 C4 | - |
| FEAT-088 | Concealed content (password managers) is shared like any other content; no sensitivity detection; contents never logged | D-61, D-121 | P1 C4 | P2 W2, M1, X1, Y1 |
| FEAT-089 | Clipboard relay through the server between clients | D-59 | P1 C4 | - |
| FEAT-090 | Master and per-format clipboard toggles | D-61 | P1 C4 | P3 G3 |

## H. Transfer core

| FEAT | Feature | Decisions | Delivered by | Also |
|---|---|---|---|---|
| FEAT-091 | Transfer engine: manifest, chunked streams, per-chunk + per-file SHA-256 | D-44 | P1 C4 | - |
| FEAT-092 | Resume from the last verified chunk, `.edgesync-partial` files | D-44 | P1 C4 | - |
| FEAT-093 | Transfer queue: progress, speed, ETA, cancel, retry, max 4 streams per peer | D-12, D-44 | P1 C4 | P3 G4 |
| FEAT-094 | Folders: recursive, empty dirs, mtime + mode preservation; symlinks skipped and reported | D-12, D-45 | P1 C4 | - |
| FEAT-095 | Safe writes through `os.Root` + path sanitiser | D-45 | P1 C4 | P5 R4 |
| FEAT-096 | Receive dir + ` (n)` clash renaming | D-17 | P1 C4 | - |
| FEAT-097 | Transfer relay through the server between clients | D-59 | P1 C4 | - |
| FEAT-098 | Send files via the GUI (picker, machine targets) | D-12 | P3 G4 | - |

## I. Native file paste and drag-and-drop (P4)

| FEAT | Feature | Decisions | Delivered by | Also |
|---|---|---|---|---|
| FEAT-099 | `vfile` provider contract + platform `FileClipboard`/`DragDrop` freeze | D-11, D-13 | P4 P4-F | - |
| FEAT-100 | Lazy file copy-paste protocol (files announce, manifest, on-demand streaming) | D-11 | P4 P4-F | - |
| FEAT-101 | Staging management for fetch-on-request mechanisms + cleanup | D-57, D-69 | P4 P4-F | - |
| FEAT-102 | Cut treated as copy across machines | D-62 | P4 P4-F | - |
| FEAT-103 | Windows: file copy detection (CF_HDROP) + virtual-file paste (IDataObject, FILEDESCRIPTORW/FILECONTENTS IStream, progress, cancel) | D-11 | P4 F1 | P0 SP2 |
| FEAT-104 | Windows: cross-screen DnD outgoing (edge drop-target window) | D-12 | P4 F2 | P0 SP2 |
| FEAT-105 | Windows: cross-screen DnD incoming (`DoDragDrop` with virtual files) | D-12 | P4 F2 | P0 SP2 |
| FEAT-106 | Windows: native file drop onto GUI machine targets | D-12, D-33 | P4 F2 | P0 SP2 |
| FEAT-107 | macOS: file copy detection + lazy paste (file promises or SP4 mechanism) | D-11 | P4 F3 | P0 SP4 |
| FEAT-108 | macOS: DnD both directions | D-12 | P4 F3 | P0 SP4 |
| FEAT-109 | macOS: native file drop onto GUI | D-12, D-33 | P4 F3 | - |
| FEAT-110 | X11: file copy detection + lazy paste (SP3 mechanism) | D-11 | P4 F4 | P0 SP3 |
| FEAT-111 | X11: XDND both directions | D-12 | P4 F4 | P0 SP3 |
| FEAT-112 | X11: native file drop onto GUI | D-12, D-33 | P4 F4 | - |
| FEAT-113 | Wayland: file copy detection + lazy paste via the portal clipboard | D-11 | P4 F5 | P0 SP3 |
| FEAT-114 | Wayland: DnD both directions (SP3 result, user-approved scope) | D-12 | P4 F5 | P0 SP3 |
| FEAT-115 | Wayland: native file drop onto GUI | D-12, D-33 | P4 F5 | P0 SP3 |
| FEAT-116 | DnD protocol + engine integration (button held across switch, cancel on Escape/return) | D-12 | P4 P4-F | - |

## J. Network and security

| FEAT | Feature | Decisions | Delivered by | Also |
|---|---|---|---|---|
| FEAT-117 | QUIC transport: datagrams, priority streams, ALPN, keepalive | D-35, D-57 | P1 C1 | P0 SP5 |
| FEAT-118 | TCP+TLS fallback: automatic selection, priority writer, data connections | D-35 | P1 C1 | - |
| FEAT-119 | Hot-path binary codec (zero alloc) + fuzzing | D-36, D-76 | P1 P1-F | P1 C1 |
| FEAT-120 | Cap'n Proto schemas, generated code, decode limits | D-36, D-67 | P1 P1-F | P1 C1, P0 SP5 |
| FEAT-121 | Handshake: version negotiation, capabilities, `Reject` reasons | D-78 | P1 C1 | - |
| FEAT-122 | Heartbeat, loss detection, reconnect with backoff + jitter | D-57 | P1 C1 | - |
| FEAT-123 | LAN + routed/VPN reachability (listen on all interfaces, dial any IP/hostname) | D-16 | P1 C1 | - |
| FEAT-124 | Identity cert generation + storage (per-user; Windows machine scope via service) | D-41 | P1 C2 | P2 W2 |
| FEAT-125 | SPKI pinning via `VerifyConnection`, TLS 1.3 mutual auth | D-42 | P1 C2 | - |
| FEAT-126 | SPAKE2 RFC 9382 P-256 with official test vectors | D-14 | P1 C2 | P0 SP5 |
| FEAT-127 | Pairing protocol: 6-digit code, TTL, attempts, exporter binding, key confirmation | D-14, D-57 | P1 C2 | - |
| FEAT-128 | Peer store: pins, names (default hostname), MACs, rename, unpair, atomic writes | D-14, D-38 | P1 C2 | - |
| FEAT-129 | mDNS/DNS-SD advertise + browse | D-15, D-43 | P1 C2 | P0 SP5 |
| FEAT-130 | Manual add by IP/hostname | D-15 | P1 C2 | P3 G3 |
| FEAT-131 | Wake-on-LAN: MAC exchange, magic packet, auto-wake on edge with throttle | D-20, D-70 | P1 C2 | P3 G5 |

## K. Agent shell

| FEAT | Feature | Decisions | Delivered by | Also |
|---|---|---|---|---|
| FEAT-132 | Local IPC: OS ACLs, peer credentials, session token, framing limits | D-46 | P1 C5 | P0 SP5 |
| FEAT-133 | IPC API: requests + events (architecture section 7) | D-46 | P1 P1-F | P1 C5 |
| FEAT-134 | Config store: TOML, `schema_version`, migrations, atomic writes, strict decoding | D-38 | P1 C5 | P1 P1-F |
| FEAT-135 | Logging: slog JSON, rotation 10 MiB x 5, ring buffer, redaction | D-48 | P1 C5 | P1 P1-F |
| FEAT-136 | i18n catalogue + missing/unused key test | D-19, D-68 | P1 C5 | - |
| FEAT-137 | Update check, notify only (ETag, semver, daily, toggle) | D-31, D-72 | P1 C5 | - |
| FEAT-138 | Lock/screensaver sync logic (server to clients, wake only on unlock, toggles) | D-20, D-71 | P1 C5 | P3 G5 |
| FEAT-139 | Diagnostics bundle (logs, redacted config, system info, permissions) | D-18 | P1 C5 | P3 G4 |
| FEAT-140 | Autostart abstraction + setting | D-21 | P1 C5 | - |
| FEAT-141 | Single-instance agent and GUI, focus hand-off | D-21, D-77 | P1 C5 | P3 P3-F |
| FEAT-142 | Agent composition root + lifecycle (signals, graceful shutdown) | D-34, D-75 | P1 C5 | P3 G5 |
| FEAT-143 | Per-OS filesystem paths | D-69 | P1 C5 | - |
| FEAT-144 | Tray model + actions (open GUI, lock, jump, home, wake, update, quit) | D-18 | P1 C5 | P3 G5 |
| FEAT-145 | Build info and versioning | D-78 | P1 P1-F | P5 R2 |

## L. GUI

| FEAT | Feature | Decisions | Delivered by | Also |
|---|---|---|---|---|
| FEAT-146 | GUI shell: Gio window, navigation, light/dark theme from agent, IPC binding, starts agent if absent | D-33, D-52, D-77 | P3 P3-F | - |
| FEAT-147 | Layout editor: drag machines, snapping, monitor groups, zoom/pan, keyboard nudging, apply/revert, read-only on clients | D-04, D-30, D-60 | P3 G1 | P0 SP5 |
| FEAT-148 | Machines page: paired + discovered peers, state, rename, unpair, connect, wake | D-14, D-15 | P3 G2 | - |
| FEAT-149 | Pairing UI: show code / enter code, progress, errors, attempts left | D-14 | P3 G2 | - |
| FEAT-150 | First-run wizard: role, pairing, OS permissions, Windows firewall check, GNOME tray extension hint | D-18 | P3 G2 | - |
| FEAT-151 | Settings - general: role, autostart, receive dir, update check, lock/WoL sync toggles, clipboard toggles | D-03, D-21, D-61, D-71 | P3 G3 | - |
| FEAT-152 | Settings - edge guards | D-05 | P3 G3 | - |
| FEAT-153 | Settings - hotkey editor with capture + conflict detection | D-06, D-58 | P3 G3 | - |
| FEAT-154 | Settings - per-machine modifier remap | D-08 | P3 G3 | - |
| FEAT-155 | Settings - network (port, manual peers) | D-43 | P3 G3 | - |
| FEAT-156 | Transfers view: queue, progress, speed, cancel, retry, open folder | D-12 | P3 G4 | - |
| FEAT-157 | Logs viewer: level filter, live tail | D-18 | P3 G4 | - |
| FEAT-158 | Diagnostics export UI | D-18 | P3 G4 | - |
| FEAT-159 | About page + update notice | D-31 | P3 G4 | - |
| FEAT-160 | GUI accessibility + i18n compliance (keyboard navigation, focus, contrast, DPI scaling) | D-19 | P3 G1-G4 | qa/ui-ux-audit.md |

## M. Integration

| FEAT | Feature | Decisions | Delivered by | Also |
|---|---|---|---|---|
| FEAT-161 | Agent wiring on real backends (engine + platform + transport + clipboard + transfer) | D-34 | P3 G5 | - |
| FEAT-162 | Loopback end-to-end tests: two agents on fakes (switch, clipboard, transfer, reconnect, lock sync) | D-37 | P3 G5 | - |

## N. Packaging, CI, release

| FEAT | Feature | Decisions | Delivered by | Also |
|---|---|---|---|---|
| FEAT-163 | Windows installer (Inno Setup 7.1.1): GUI, agent, service, autostart, firewall rules, `SoftwareSASGeneration`, upgrade in place, uninstall | D-25, D-118 | P5 R1 | - |
| FEAT-164 | macOS unsigned `.app` (LSUIElement agent, Info.plist usage strings), zipped | D-25 | P5 R2 | - |
| FEAT-165 | Linux tar.gz: binaries, `.desktop`, autostart, install/uninstall script | D-25 | P5 R2 | - |
| FEAT-166 | Release CI: GoReleaser OSS archives + checksums, the Windows installer and bundles from native runners, signing step skipped without certificate | D-24, D-26 | P5 R2 | - |
| FEAT-167 | CI pipeline: lint (3 GOOS), build, test, race, coverage gate, cross-compile, gen freshness, vuln, actionlint | D-24, D-37 | P0 P0-F | every phase extends |
| FEAT-168 | Integration test jobs on CI runners (Windows, macOS, Ubuntu + Xvfb) | D-24, D-37 | P2 W1, W2, X1, Y1, M1 | P5 R5 |
| FEAT-169 | Developer toolchain, repo scaffold, quality gates (go.mod, tools module, golangci config, Taskfile, covercheck) | D-53, D-54, D-55 | P0 P0-F | - |

## O. Performance, security, QA, docs, spikes

| FEAT | Feature | Decisions | Delivered by | Also |
|---|---|---|---|---|
| FEAT-170 | Input latency budget p99 <= 5 ms + measurement harness | D-29 | P5 R3 | P1 C1, P3 G5, P0 SP5 |
| FEAT-171 | File throughput >= 90% of GbE; input latency under bulk load | D-35 | P5 R3 | P1 C4 |
| FEAT-172 | Capture callback, CPU, memory, GUI frame-time budgets | D-29 | P5 R3 | P2 W1, P3 G1 |
| FEAT-173 | Security review: pairing, TLS, IPC, service, path safety, decode limits, clipboard privacy | D-14, D-46, D-47 | P5 R4 | per-stream security gates |
| FEAT-174 | Long fuzz campaigns on Linux CI | D-36 | P5 R4 | - |
| FEAT-175 | Manual QA matrix (user's Windows PCs + GNOME box) | D-37 | P5 R5 | - |
| FEAT-176 | Docs: README, user guide, troubleshooting (permissions, firewall, GNOME tray extension, Wayland consent) | - | P5 R5 | - |
| FEAT-177 | Platform de-risking spikes with findings and go/no-go per mechanism | D-13, D-74 | P0 SP1-SP5 | - |
| FEAT-178 | Spikes module removal after findings are archived | D-74 | P5 R5 | - |

## P. Product answers (2026-10-30, D-112 .. D-121)

| FEAT | Feature | Decisions | Delivered by | Also |
|---|---|---|---|---|
| FEAT-179 | Control at the Windows sign-in screen is opt-in per server: machine peer store, elevated `edgesync-service allow-prelogin`, read-only `GetPreloginPeers`, Machines page switch | D-112, D-64 | P2 W2 | P3 G2, G5 |
| FEAT-180 | Send Ctrl+Alt+Del to a Windows client: hotkey Ctrl+Alt+End (rebindable) and tray/GUI action "Send Ctrl+Alt+Del to <machine>" | D-113, D-58 | P1 C3 | P1 C5, P2 W2, P3 G2, G3, G5 |
| FEAT-181 | Wayland servers: lock-to-screen and panic hotkeys while the pointer is home through the GlobalShortcuts portal, capability detection, limitation hints for jump/cycle | D-114 | P2 Y1 | P1 P1-F, C3, P3 G2, G3, G5 |
| FEAT-182 | macOS lock availability detected at start (private `SACLockScreenImmediate`), explicit "lock unsupported on this Mac" status in GUI and log | D-115, D-71 | P2 M1 | P3 G2, G5 |
| FEAT-183 | macOS permission re-grant guidance after updates (first-run wizard, permissions page, docs) | D-117 | P3 G2 | P5 R5 |

## Out of scope for v1 (by decision)

| Item | Decision |
|---|---|
| Game/relative-mouse mode | D-20 |
| Synergy/Deskflow/Input Leap protocol interoperability | D-20 |
| wlroots compositors (Sway, Hyprland, Wayfire) | D-02 |
| OS "Send to" / Share / Services menu integration | D-12 |
| Character-mode keyboard fallback | D-08 |
| Internet relay, NAT traversal | D-16 |
| Auto-installing updates, telemetry | D-31 |
| DMG, deb/rpm, Flatpak, AppImage | D-25 |
| Code signing and notarization (until certificates exist) | D-26 |
| Translations beyond English | D-19 |
| PRIMARY selection sync on Linux | D-61 |
| A Windows machine at the sign-in screen acting as server | D-64 |

Count: 183 features (FEAT-001 .. FEAT-183).
