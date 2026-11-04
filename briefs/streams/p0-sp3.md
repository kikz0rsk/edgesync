# Stream brief - P0 SP3 Linux (Wayland portals + EI, X11, file paste, DnD, GNOME indicator)

Phase 0 spike stream. Items: `briefs/plan-phase-0.md` section "SP3". Owner tag: `[owner:SP3]`.

## Scope
Prove the Linux mechanisms behind D-02 (GNOME 45+/KDE 6.1+ via portals), D-39 (pure-Go EI codec, xgb), D-10/D-11
(clipboard and lazy file paste on X11 and Wayland), D-12 (DnD), D-18 (switch indicator on GNOME) and D-05 (guard
semantics under barrier activation). The user has a GNOME Wayland box (no KDE, no X11 session); X11 is exercised
under Xvfb/Xephyr. Deliverable: evidence plus `briefs/spikes/sp3.md`.

## Files you may touch
Only your item's `Files:`, all under `spikes/linux/` (plus the qa script/results). Never touch the main module,
plan files, or other streams' spike directories.

## Conventions (binding)
- Pure Go with `CGO_ENABLED=0` for everything except Gio-based spikes (Gio needs cgo on Linux; those are built on the GNOME box per the README). Every non-Gio file must pass `GOOS=linux CGO_ENABLED=0 go -C spikes vet ./linux/...` on the Windows host.
- Libraries: `github.com/godbus/dbus/v5` v5.2.4, `github.com/jezek/xgb` v1.3.1, `golang.org/x/sys/unix` for SCM_RIGHTS.
- Licences: libei protocol XML and xcb-proto are MIT; you may generate code from them and must cite them. Never copy from Deskflow, lan-mouse, Input Leap or go-x11-client (GPL, D-27); `reis` (Rust, MIT) may be read for understanding.
- Portal request pattern: pass `handle_token` in options, compute the request object path `/org/freedesktop/portal/desktop/request/<sender with ':' removed and '.' -> '_'>/<token>`, subscribe to its `Response` signal BEFORE calling the method, then call; treat response code 1 (cancelled) and 2 (other) as explicit errors.
- Integration tests touching X servers or D-Bus sessions carry `//go:build integration && linux`; never run them on the host.
- No em dashes or en dashes. Commit `spike(linux): ... [FEAT-177]`, body `Item: <id>`, no attribution trailers.
- Research pointers: `briefs/research-platform-internals.md` sections 1.3, 1.4, 2 (Linux), 3 (Linux), 5, 6 (d), 9; portal docs https://flatpak.github.io/xdg-desktop-portal/docs/ ; libei docs https://libinput.pages.freedesktop.org/libei/ and protocol XML in https://gitlab.freedesktop.org/libinput/libei ; XDND spec https://www.freedesktop.org/wiki/Specifications/XDND/ ; Wayland `wl_data_device.start_drag` semantics https://wayland.app/protocols/wayland .

## Report format
```
FILES: <path> - <one clause>
GATES: <gate> pass|fail
ITEMS: <item id> <FEAT-NNN, ...>
TESTS: <what the test phase must cover, one line>
BLOCKED: <reason or NONE>
```

---

## Item P0-SP3-001
EI wire codec (`spikes/linux/ei/`).
- Read the EI protocol XML (record the libei version/commit you used) and implement: header (object id u64, length u32 incl. header, opcode u32, native byte order = little-endian on supported targets), argument encoding (uint32, int32, uint64, int64, float, string with length prefix and padding, new_id, object, fd out-of-band).
- Interfaces needed: ei_handshake (version negotiation, context type sender/receiver, name, interface versions), ei_connection (sync, disconnected, seat, ping), ei_callback, ei_pingpong, ei_seat (capabilities, bind), ei_device (name, device_type, dimensions, regions, interface, done, resumed, paused, start_emulating, stop_emulating, frame), ei_pointer (motion_relative), ei_pointer_absolute (motion_absolute), ei_button (button), ei_scroll (scroll, scroll_discrete, scroll_stop), ei_keyboard (keymap with fd, key, modifiers).
- `client.go`: connection over a `net.Conn` (unix socket from the portal fd) with a dispatch loop; receiver role (capture) and sender role (injection).
- Tests on the host use `net.Pipe` with a scripted fake server and golden byte frames; fuzz the frame decoder.

## Item P0-SP3-002
Portals (`spikes/linux/portal/`, cmd `wlcapture`, `wlinject`).
- InputCapture (record the interface `version` property): `CreateSession2` when version >= 2 (else `CreateSession` with capabilities keyboard|pointer), `GetZones`, `SetPointerBarriers` (one barrier on the right edge of the rightmost zone), `ConnectToEIS` (returns an fd; wrap as unix socket conn for the EI client), `Enable`; on `Activated(session, options)` read `activation_id` and cursor position, consume EI events for 5 s, then `Release(session, {activation_id, cursor_position})`; then change the barrier (move it to the left edge) and record whether events still arrive, and whether a full session re-creation is needed (mutter quirk). `persist_mode=2`, save `restore_token` to `spikes/out/ic-token` and reuse it.
- RemoteDesktop: `CreateSession`, `SelectDevices(types=1|2, persist_mode=2, restore_token)`, `Start`, `ConnectToEIS`, then via EI sender: pointer square (relative motion), button click, type `edgesync` with evdev keycodes.
- Print everything with timestamps into `spikes/out/*.log`.

## Item P0-SP3-003
Portal clipboard (`wlclip`). Attach `RequestClipboard` to a RemoteDesktop session before `Start`; separately try with an InputCapture session (v2 clipboard). `SetSelection(session, {mime_types: [...]})` with the five MIME types from the item; on `SelectionTransfer(session, mime, serial)` call `SelectionWrite(session, serial)` -> fd, wait `-delay`, write data, close, `SelectionWriteDone(session, serial, success)`; on `SelectionOwnerChanged` (local copy) call `SelectionRead(session, mime)` for each offered MIME and log sizes. Record whether transfers fail while the session is inactive.

## Item P0-SP3-004
X11 (`spikes/linux/x11/`, cmd `x11demo`).
- `conn.go`: xgb connection; load extensions xtest, randr, xfixes.
- `grab.go`: create a fullscreen override-redirect InputOnly window, `GrabPointer`/`GrabKeyboard`, warp to centre after each MotionNotify and accumulate deltas.
- `inject.go`: `xtest.FakeInput` for motion, buttons, keys.
- `randr.go`: `GetMonitors` (RandR 1.5) with names and geometry; `SelectInput` for screen/CRTC change notify.
- `selection.go`: own CLIPBOARD, answer `SelectionRequest` for TARGETS, UTF8_STRING, text/html, image/png; INCR for payloads above the max request size; XFixes `SelectSelectionInput` notifications for foreign owners.
- Integration tests run under Xvfb `:99`; the test setup defines two monitors with `xrandr --setmonitor` (document the exact commands in README; if `--setmonitor` is not supported by Xvfb, record it and use Xinerama-equivalent `-screen` layout instead).

## Item P0-SP3-005
XI2/XKB generation. Clone xcb-proto (record tag) and run jezek/xgb's `xgbgen` on `xinput.xml` and `xkb.xml` (`gen.sh` documents exact commands; generated output into `spikes/linux/x11gen/xinput/` and `xkb/` if it works). Try compiling with `GOOS=linux CGO_ENABLED=0`. If it compiles, write `xi2_integration_test.go` selecting `XI_RawMotion` on the root while a grab is active. If it does not, record exact errors and an estimate of hand-fix effort.

## Item P0-SP3-006
File-manager lazy paste (`filepaste`). Modes `-mode x11` (own CLIPBOARD with targets `text/uri-list`, `x-special/gnome-copied-files` (`copy\nfile:///...`), `TARGETS`) and `-mode portal` (via the SP3-003 code). On a data request: start writing a synthetic file of `-size` into `-staging` while delaying the reply by `-delay` (5s/30s/120s), then reply with the `file://` URI. Log timing; README tells the user what to observe in Nautilus (wait cursor, progress, error, completeness of the pasted file via `sha256sum`).

## Item P0-SP3-007
DnD, GNOME indicator, Gio drop.
- `dndx11`: XDND source (drag a synthetic file URI from a spike window into a file manager inside Xephyr) and target (accept a drop from the file manager, print URIs). Follow the XDND spec version 5.
- `wloverlay`: (a) Notification portal `AddNotification` with a short title, record placement, duration and whether it steals focus; (b) an undecorated Gio window (built on the box) requesting a position, record actual placement and stacking under GNOME.
- `giodroplinux` (built on the box with cgo): check whether Gio v0.10.3 exposes file drops on Wayland or X11 (search its API: `io/transfer`, `app` events) and whether its view event exposes `wl_display`/`wl_surface` handles that could host our own `wl_data_device` listener; record exactly what is and is not possible.
- `dnd/README.md`: written analysis with citations: (1) outgoing drag detection on GNOME Wayland - does an InputCapture activation happen while a drag is in progress (test in wlcapture with a drag held), (2) incoming drag initiation - `wl_data_device.start_drag` needs the serial of an implicit grab on a surface of the same client; evaluate whether injecting a button press via EI onto an agent-owned surface could provide it; (3) XDND feasibility for X11.

## Item P0-SP3-008
`qa/manual/p0-sp3-linux.md` per `qa/manual/README.md`: preparation on the GNOME box (install Go 1.27.3 from go.dev tarball; packages for Xvfb, Xephyr, xdotool, Gio build deps per https://gioui.org/doc/install/linux; record `gnome-shell --version`, `/usr/libexec/xdg-desktop-portal --version` or package version, distro), then every spike in order with commands, expected observations, paste-back. X11 tests: `Xvfb :99 &` then `DISPLAY=:99 go -C spikes test -tags integration ./linux/x11/... -v`, or dispatch the spikes workflow.

## Item P0-SP3-M01
User-owned. Wayland results into `qa/manual/results/p0-sp3-linux.md` (Wayland section).

## Item P0-SP3-M02
User-owned. X11 results (Xvfb test output or the spikes workflow log, Xephyr observations) into `qa/manual/results/p0-sp3-linux.md` (X11 section).

## Item P0-SP3-009
`briefs/spikes/sp3.md`: summary table; sections per mechanism; "Recommended design for P2 X1/Y1 and P4 F4/F5" (EI codec API, portal session manager incl. recreation, X11 delta source (XI2 or warp), clipboard availability rules on GNOME, the lazy file paste mechanism for X11 and for Wayland, DnD scope per display server, switch indicator on GNOME, how edge guards behave with barrier activation); "Requires user decision" for every contradiction of D-02, D-05, D-11, D-12, D-18, D-39 (e.g. indicator not an overlay on GNOME, DnD direction not possible, file paste not lazy).

## Item P0-SP3-010
P0-SP3-006 (`spikes/linux/cmd/filepaste/README.md`) found that the `x-special/gnome-copied-files` payload built by `spikes/linux/cmd/wlclip` ends with an LF
and that the Nautilus parser rejects that (the format is `copy` or `cut`, then LF, then the file URIs joined by LF, no trailing LF; read the source the filepaste README cites and
cite it again in the wlclip README). Fix the encoder in `spikes/linux/cmd/wlclip/main.go` only; keep `text/uri-list` as it is (CRLF-terminated lines per RFC 2483). Add a table test for
the payload builder (if the builder lives in `spikes/linux/portal`, test it in `spikes/linux/portal/clipboard_test.go`). Do not touch any other behaviour. If the builder is not a pure
function yet, extract exactly that function (no other refactoring) so it can be tested on the host.

## Item P0-SP3-011
Evidence: `qa/manual/results/p0-sp3-linux.md`. In the linux job of `.github/workflows/spikes.yml`: (1) `TestRandR_MonitorsListed` needs two monitors but the workflow never defines
them; add the `xrandr --setmonitor` steps after Xvfb starts and print `xrandr --listmonitors`; (2) two XDND integration tests failed with `connect to display "": EOF`, most likely
because Xvfb resets when its last client disconnects: start it with `-noreset`. Apply the same two lines to the Xvfb instructions in `qa/manual/p0-sp3-linux.md` and
`spikes/linux/x11/README.md`. The xrandr RandR 1.5 monitor syntax must be verified against the xrandr manual (do not trust memory) and the source recorded in the commit body.
