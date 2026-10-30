# EdgeSync - Open product questions (planning consolidation, 2026-10-30)

<!-- NOTE: 2026-10-30 - all ten questions are answered; the answers are mapped to decisions D-112 .. D-121 in the "Answers" section at the end. The question texts below are kept as asked. -->

These are the only plan gaps that are product or preference choices; every engineering gap was closed with D-80 ..
D-111 in `decisions.md`. The parent asks them in AskUserQuestion rounds of at most four (suggested batches: Q-01..Q-04,
Q-05..Q-08, Q-09..Q-10) before the plan is approved, then appends one decision per answer ("Answers Q-NN"; "Supersedes
D-NN" where the answer changes a locked decision) and toggles nothing: items keep their text unless the answer
supersedes them (D-111).

---

## Q-01 - Control at the Windows sign-in screen: who is allowed

Context: D-07/D-64 let a paired server control a Windows client at the sign-in screen (nobody logged on) through the
SYSTEM service's pre-login mode, using the machine identity (D-41: one identity per Windows machine, held by the
service, so every Windows user account on that PC presents the same identity to peers). Pre-login mode can only accept
servers listed in a machine-scoped peer store, and nothing defines how a server gets into that store (G-06): the
service pipe must not change configuration (D-47), and the per-user peer stores are not readable by the service by
design.

Options:
- A (Recommended): an explicit per-server switch "Allow control at the Windows sign-in screen" on the Machines page.
  Turning it on triggers a UAC prompt for a small elevated helper (`edgesync-service allow-prelogin <peer>`) that
  copies that server's pin into `%ProgramData%\EdgeSync\machine-peers.toml`; turning it off removes it. Trade-off: one
  UAC prompt per server, but no silent trust escalation.
- B: every server paired by any user on this PC is trusted at the sign-in screen automatically (the service copies pins
  when a user agent reports a new pairing). Trade-off: no prompt, but one user's pairing grants pre-login control for
  all accounts.
- C: no sign-in-screen control in v1 (UAC prompts and the lock screen inside a logged-on session still work through
  the secure helper). Trade-off: smallest surface; FEAT-049 is dropped.

Affects: D-64, D-83, FEAT-049, P2-W2-028 (and a new item for the elevated helper command and the Machines page switch
under options A/B), P3 G2 Machines page, P5-R1 (the installer keeps the machine store on uninstall, D-108).

## Q-02 - Ctrl+Alt+Del from a Windows server

Context: Windows never delivers a physical Ctrl+Alt+Del to user-mode hooks, so a user sitting at a Windows server cannot
press it for a client (G-08). From macOS/Linux servers the chord arrives as normal keys and W2 already maps it to
`RequestSAS` on Windows clients.

Options:
- A (Recommended): hotkey Ctrl+Alt+End (the Remote Desktop convention) sends Ctrl+Alt+Del to the active Windows client,
  plus a tray/GUI action "Send Ctrl+Alt+Del to <machine>". Rebindable like the D-58 hotkeys.
- B: tray/GUI action only (no hotkey).
- C: hotkey only, Ctrl+Alt+Insert.

Affects: D-58 (adds a default binding), P1-C3-010/P1-C3-018 hotkey tables (a new action), P2-W2-029, P3 G3 hotkey editor,
tray model P1-C5-016.

## Q-03 - Hotkeys on a Wayland server while the pointer is at home

Context: on GNOME/KDE Wayland, a server only receives keyboard input while it controls a remote machine (InputCapture
portal activation). While the pointer is on the server itself, EdgeSync cannot see or swallow hotkeys, and it cannot
start controlling a client without the pointer hitting an edge, so "jump to machine" and "cycle" cannot start from home
(G-15). The xdg-desktop-portal GlobalShortcuts interface can deliver registered shortcuts to apps; KDE implements it,
GNOME support for GNOME 45-47 is UNVERIFIED.

Options:
- A (Recommended): use the GlobalShortcuts portal where the compositor provides it, for lock-to-screen and panic while at
  home (the user confirms the shortcuts once in the portal dialog); jump/cycle from home stay unavailable on Wayland
  servers; the GUI and docs state the limitation. Adds Y1 items (portal session, binding sync, detection).
- B: accept the limitation as is: on Wayland servers, hotkeys work only while controlling a remote machine;
  lock-to-screen from home via the tray menu.

Affects: D-06, D-58, FEAT for hotkeys on Wayland, P2-Y1-010 and new Y1 items under A, P3 G3 hotkey editor hints, docs.

## Q-04 - Locking macOS clients

Context: D-71 locks every client when the server locks. macOS has no public "lock screen now" API; the reliable way is
the private `SACLockScreenImmediate` in `login.framework` (G-16). Private APIs can disappear in a macOS update, and this
cannot be hand-tested (no Mac available).

Options:
- A (Recommended): use the private API, detect its absence at start, and report "lock unsupported on this Mac" in the GUI
  and log when missing (no silent fallback).
- B: on macOS clients, "lock" starts the screensaver instead (locks only when the user's "require password immediately"
  setting is on).
- C: macOS clients ignore lock propagation (screensaver and display wake still sync).

Affects: D-71, P2-M1-013, risk register (new private-API risk under A).

## Q-05 - Autostart with the per-machine Windows installer

Context: D-21 makes autostart a per-user toggle (default on). The Windows installer installs per machine (D-25, D-108), so other
Windows accounts on the PC would not autostart EdgeSync unless the installer registers it machine-wide.

Options:
- A (Recommended): the installer writes `HKLM\...\Run` for all users with `--autostart`; each user's toggle lives in
  `HKCU\Software\EdgeSync\Autostart` (missing = on), and an agent started with `--autostart` exits at once when that
  user turned it off; in machine mode the agent removes its own HKCU Run value so it never starts twice (P5-R1-008 as
  written).
- B: no HKLM entry; each user's agent registers its own HKCU Run value on first start (other accounts start EdgeSync
  manually once). P5-R1-008 is superseded.
- C: autostart only for the account that ran the installer.

Affects: D-21, D-25, P2-W1-017, P5-R1-008.

## Q-06 - macOS permissions after updates

Context: with no Apple Developer ID (D-26), the macOS bundles are ad-hoc signed (D-106, required on Apple Silicon).
macOS ties Accessibility, Input Monitoring and Local Network grants to the code signature, so after every EdgeSync
update the user has to grant these permissions again (System Settings, then restart the agent).

Options:
- A (Recommended): accept this for v1; the first-run wizard and the docs explain the re-grant after updates.
- B: plan an Apple Developer ID (paid account) now: Developer ID signing and notarization become part of P5 R2, grants
  survive updates. Supersedes D-26 for macOS.

Affects: D-26, D-106, P5-R2 items, P3 G2 wizard text, docs (P5 R5).

## Q-07 - Windows installer toolset and version

Context: D-65 pinned a Windows installer toolset whose newer versions ship under a licence that can require a paid fee. The question
offered the pinned older version (A) or the newest version with the fee terms accepted (B). The full original wording and the
alternatives are recorded in `decisions.md` D-65 (superseded) and D-118.

Affects: D-65, P0-F-003, P5 R1 items.

## Q-08 - LICENSE copyright holder

Context: D-79 writes "Copyright (c) 2026 kikz0r" into the MIT LICENSE (P0-F scaffold).

Options:
- A (Recommended): keep "kikz0r" (matches the module path owner).
- B: use a legal name or organisation (the user types it via "Other").

Affects: D-79, P0-F-004 (writes LICENSE), THIRD_PARTY_NOTICES (P5-R4-013).

## Q-09 - How the cursor maps across machines of different sizes

Context: D-04 asked for proportional mapping across partial-edge overlaps. D-51 interprets it as geometric continuity:
the cursor crosses only where the two screens' edges overlap on the layout canvas and keeps its canvas position (like
Windows display settings); outside the overlap the edge is a wall.

Options:
- A (Recommended): geometric continuity (D-51 as locked).
- B: stretch: the whole source overlap maps onto the whole target overlap, so the full height of a tall screen reaches
  the full height of a short neighbour (cursor jumps vertically when crossing).

Affects: D-04, D-51, P1-C3-007 (mapping), P3 G1 layout editor guides.

## Q-10 - Clipboard privacy defaults

Context: D-61 never announces clipboard content that its source marks as concealed or sensitive (password managers via
`ExcludeClipboardContentFromMonitorProcessing`, `org.nspasteboard.ConcealedType`, `x-kde-passwordManagerHint`), syncs
only CLIPBOARD (not the X11 PRIMARY selection), and adds per-format toggles (all on by default).

Options:
- A (Recommended): as locked in D-61.
- B: also sync concealed content (passwords copied from a password manager reach other machines).
- C: as A, plus sync the X11/Wayland PRIMARY selection (middle-click paste) between Linux machines.

Affects: D-61, P1-C4-007, P2 clipboard backends, P3 G3 settings.

---

## Answers (2026-10-30)

The user answered on 2026-10-30; the parent recorded one decision per answer (`decisions.md`, "Product answers" section).

| Question | Answer | Decision | Effect |
|---|---|---|---|
| Q-01 Control at the sign-in screen | A: per-server switch, one UAC prompt, `edgesync-service allow-prelogin <peer>` | D-112 | New items P2-W2-036 .. P2-W2-038 (store, elevated command, read-only `GetPreloginPeers`), P3-G2-011, P3-G2-012 (Machines page switch), P3-G5-020, P3-G5-021 (IPC `GetPreloginAccess`); P2-W2-028 reads the store; FEAT-179 |
| Q-02 Ctrl+Alt+Del from a Windows server | A: hotkey Ctrl+Alt+End plus tray/GUI action | D-113 | P1-F-020 (config key), P1-C3-010 (action and default), new P1-C3-026, P1-C5-027 (tray), P3-G2-011/012 (row action), P3-G3-013 (hotkey editor), P3-G5-020/021 (IPC `SendCtrlAltDel`), P2-W2-029 (chord from the engine); FEAT-180 |
| Q-03 Hotkeys on a Wayland server at home | A: GlobalShortcuts portal for lock and panic | D-114 | New P1-F-023, P1-C3-027, P2-Y1-026, P2-Y1-027, P3-G3-013 (hints), P3-G2-013 and P3-G5-021 (check row); docs; FEAT-181 |
| Q-04 Locking macOS clients | A: private `SACLockScreenImmediate`, detect at start, surface when missing | D-115 | New P2-M1-028, P3-G5-021 (`lockScreen` check), P3-G2-013 (check row); risk R-52 active, new R-58; FEAT-182 |
| Q-05 Autostart with the installer | A: HKLM Run entry with `--autostart`, per-user HKCU opt-out | D-116 | P5-R1-006 (HKLM value) and P5-R1-008 as written |
| Q-06 macOS permissions after updates | A: accept the re-grant for v1 | D-117 | P3-G2-013 (wizard and permissions page text), docs (P5-R5-004, P5-R5-008); FEAT-183 |
| Q-07 Windows installer toolset | Inno Setup 7.1.1 (the previously pinned toolset is superseded) | D-118 | P0-F-003 and every P5 R1 item rewritten (same IDs), new P5-R1-018 (lifecycle CI job), P5 R2 asset names, plan, CLAUDE.md, compatibility matrix, risk register (R-37, R-61) |
| Q-08 LICENSE copyright holder | A: "kikz0r" | D-119 | none (D-79 confirmed) |
| Q-09 Cursor mapping | A: geometric continuity | D-120 | none (D-51 confirmed) |
| Q-10 Clipboard privacy defaults | B: concealed content is shared | D-121 | P1-C4-007 and the clipboard backend items rewritten (no sensitivity detection), `LocalChange.Sensitive` removed, P3-G5-012, P4 file backends, P5-R4-012, `qa/security-review.md`, docs; risk R-59 |
