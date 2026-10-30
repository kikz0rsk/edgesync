# portal - InputCapture and RemoteDesktop portal spike (P0-SP3-002)

Item P0-SP3-002 (FEAT-177, FEAT-074, FEAT-075, FEAT-076, FEAT-077). Decisions under test: D-02 (Wayland on GNOME 45+
and KDE Plasma 6.1+ through portals), D-39 (pure-Go platform layers: godbus for the portals, the in-house EI codec of
`../ei` for the event transport) and the mutter session-recreation quirk recorded in the research brief.

## Purpose

Drive the two portals from pure Go (`CGO_ENABLED=0`) and hand their EIS file descriptors to the EI codec:

- `wlcapture`: InputCapture. Prints the interface versions and the zones, arms a pointer barrier on the right edge of
  the rightmost zone, waits for the `Activated` signal, consumes the EI events of the capture for 5 s (relative motion,
  buttons, keys, scroll), then `Release(activation_id, cursor_position)`. It then moves the barrier to the left edge on
  the same session and records whether `Activated` and EI events still arrive, and finally closes the session, creates
  a new one (with the saved restore token) and records whether a re-created session captures.
- `wlinject`: RemoteDesktop. `CreateSession`, `SelectDevices(types=keyboard|pointer, persist_mode=2, restore_token)`,
  `Start`, `ConnectToEIS`, then through an EI sender client: a pointer square from relative motion, a left click and the
  typed text `edgesync` with evdev keycodes.

Both persist the single-use `restore_token` returned by `Start` (`out/ic-token`, `out/rd-token`) and send it on the next
run, which shows whether the backend restores a session without the consent dialog.

## Sources (cited)

Every method name, signature, option key and signal below was checked against the portal interface definitions:

- Repository: https://github.com/flatpak/xdg-desktop-portal
- Version read: tag `1.22.1`, commit `1d20fadc304f6601452b5db65ed91197dba77041` (released 2026-06-17)
- Files: `data/org.freedesktop.portal.InputCapture.xml` (interface version 2), `data/org.freedesktop.portal.RemoteDesktop.xml`
  (interface version 2), `data/org.freedesktop.portal.Session.xml`, `data/org.freedesktop.portal.Request.xml`
- Licence: LGPL-2.1-or-later (SPDX header of the XML files). Only interface facts were taken, no code.
- D-Bus library: `github.com/godbus/dbus/v5` v5.2.4 (tag commit `4e46d5eb6d1926b98ce405d4909cf3588698ace5`, 2026-10-13)
- EI protocol: libei 1.6.0, see `../ei/README.md`

Facts from those files that shape the code:

- Interface `version` is a property of the same object path `/org/freedesktop/portal/desktop`; the destination is
  `org.freedesktop.portal.Desktop`.
- InputCapture version 2 adds `CreateSession2` (returns `a{sv}` with `session_handle`, no Request object) and `Start`
  (a Request that takes `capabilities`, `persist_mode`, `restore_token`; its Response carries `capabilities`,
  `clipboard_enabled`, `restore_token`). `CreateSession` is deprecated in version 2; on version 1 it is the only way and
  is a Request with `capabilities` and `session_handle_token`. Restore tokens and `persist_mode` exist only from
  version 2, so a version 1 portal with a persistence request is rejected with `ErrPersistenceUnsupported`.
- `GetZones` Response: `zones` as `a(uuii)` (width, height, x, y) and `zone_set`. `SetPointerBarriers(session, options,
  barriers aa{sv}, zone_set u)`: each barrier has `barrier_id` (`u`, non-zero) and `position` (`(iiii)`, x1 y1 x2 y2);
  a stale `zone_set` fails the barriers; the Response lists denied ids in `failed_barriers` (`au`). A vertical barrier
  on the right edge sits at `x = zone.x + zone.width`, on the left edge at `x = zone.x`, spanning `y .. y + height - 1`.
- `SetPointerBarriers` suspends the session, so `Enable` must follow. `ConnectToEIS` must be called before `Enable` and
  once per session. `Enable`, `Disable` and `Release` have no Request.
- `Activated(session, options)` carries `activation_id`, `cursor_position` (`(dd)`, usually outside the zones) and an
  optional `barrier_id`; the activation id equals the sequence of the EIS `start_emulating` event. `Release` takes
  `activation_id` and a suggested `cursor_position`.
- RemoteDesktop `CreateSession` returns `session_handle` typed `s` (documented erratum, it is an object path), unlike
  InputCapture's `o`. `ConnectToEIS` exists from version 2 and must follow `Start`; afterwards the `Notify*` methods error.
- Request pattern: pass `handle_token`, derive `/org/freedesktop/portal/desktop/request/<sender without ':' and '.' -> '_'>/<token>`,
  subscribe to `Response` before the call; response `0` success, `1` cancelled, `2` ended otherwise. A returned handle
  that differs from the derived path is an error here (only portals older than 0.9 do that).

## Code layout

- `request.go`: `Bus` interface (the only door to D-Bus), path and token derivation, `Response` decoding, `Results`
  accessors that keep "key missing" and "wrong type" apart, `Caller.Request` (subscribe, call, wait, `Request.Close` on
  cancellation).
- `session.go`: `Session`, `EventStream` (typed events from the session signals, one goroutine per source, stoppable) and
  `SessionBus`, the godbus implementation of `Bus` (signal multiplexer, fd passing, `os.NewFile` on the returned `h`).
- `inputcapture.go`: `InputCapture.Open` (version 2: `CreateSession2` then `Start`; version 1: `CreateSession`),
  `CaptureSession` (`GetZones`, `SetPointerBarriers`, `ConnectToEIS`, `Enable`, `Disable`, `Release`, `Arm`, `Events`),
  `BarrierFor`, `ReleasePosition`. `Arm` is the sequence required after every barrier change: `GetZones`,
  `SetPointerBarriers`, `Enable`. Subscribe with `Events` before `Arm` so no `Activated` is missed.
- `remotedesktop.go`: `RemoteDesktop.Open` (`CreateSession`, `SelectDevices`, `Start`) and `RemoteSession.ConnectToEIS`.
- `request_test.go`: host tests against a fake bus that behaves like the portal (see below).

## Build

From the repository root (cross-compiles from Windows; on the GNOME box drop the two variables):

```
GOOS=linux CGO_ENABLED=0 go -C spikes build -o bin/wlcapture ./linux/cmd/wlcapture
GOOS=linux CGO_ENABLED=0 go -C spikes build -o bin/wlinject ./linux/cmd/wlinject
GOOS=linux CGO_ENABLED=0 go -C spikes vet ./linux/...
```

## Host tests

```
go -C spikes test ./linux/portal/...
```

`TestRequest_HandleTokenPathDerivation` and `TestRequest_ResponseDecoding` cover the path derivation and the Response
decoding. The fake bus also exercises the whole state machine without a session bus: subscription before call, handle
mismatch, cancellation with `Request.Close`, `CreateSession2` then `Start` with `persist_mode` and `restore_token`,
`ConnectToEIS` before `Enable`, `GetZones` then `SetPointerBarriers` then `Enable`, a stale `zone_set` and denied
barriers, `Activated` and `Deactivated` filtered by session, `Release` options, re-arming after a barrier change,
`Session.Closed`, version 1 behaviour, and the RemoteDesktop flow with both `s` and `o` session handles. With goroutines
in play run it with `-race` (`CGO_ENABLED=1`, a C compiler on PATH).

## Run (GNOME Wayland session, user run only)

Run from `spikes/` so the output lands in `spikes/out/` (created by the programs):

```
./bin/wlcapture
./bin/wlcapture
./bin/wlinject -delay 5s
./bin/wlinject
```

Flags: `wlcapture -log out/wlcapture.log -token-file out/ic-token -wait 90s -hold 5s`;
`wlinject -log out/wlinject.log -token-file out/rd-token -square 200 -step 5 -delay 5s -text edgesync`. Every line goes
to stdout and is appended with a timestamp to the log file. Ctrl+C stops a run and closes the session. Delete the token
file to force a fresh consent dialog.

`wlcapture` phases (each says `ACTION:` when it needs you):

1. `right-edge`: after `armed`, push the pointer through the right edge of the rightmost monitor. When `Activated`
   appears, for the next 5 s move the mouse, click, press keys and scroll the wheel. Then `Release sent` and the pointer
   must reappear on the compositor side.
2. `left-edge-same-session`: the barrier moves to the left edge of the leftmost zone on the same session and the same
   EIS connection. Push the pointer through the left edge and repeat the input.
3. `left-edge-recreated-session`: the first session is closed, a new one is created with the restore token, and the
   left edge is armed again. Push the pointer through the left edge and repeat the input.

The final `SUMMARY` line states `right_edge_capture_works`, `barrier_change_same_session_works`,
`barrier_change_session_closed_by_portal`, `barrier_change_ei_ended`, `recreated_session_works` and
`restore_token_issued`. A phase that times out is recorded, not hidden.

## What to observe

- `InputCapture` and `RemoteDesktop` interface `version` and capability bitmasks in the first lines.
- First run: a consent dialog at `Start`. Second run: whether the dialog is skipped (restore token honoured), which the
  log cannot show, so note it by hand. Also whether `restore_token_returned` is true at all.
- `armed` shows the zones (width, height, x, y), the zone set id and the barrier positions.
- `Activated` with `activation_id`, `cursor` and `barrier_id`; the EI `start_emulating` line carries `sequence`, which
  must equal the activation id.
- EI lines for pointer motion (relative deltas), buttons (evdev codes), keys (evdev codes) and scroll; the `RESULT`
  line per phase with the counters.
- Phase 2: whether `Activated` arrives, whether EI events arrive, or whether `Session.Closed` or `EI connection ended`
  appears (the mutter quirk: events stop after a disable, or the session dies, so it must be re-created).
- `wlinject`: whether the pointer draws a square, the click lands, `edgesync` appears in the focused editor, and which
  EI devices were announced (names, interfaces); whether `ready` handling was needed (`EI device resumed` follows `done`).

## What to paste back

- `out/wlcapture.log` from both runs and `out/wlinject.log` from both runs (or at least every `RESULT`, `SUMMARY`,
  `Activated`, `Session.Closed`, `EI connection ended`, `ERROR` line plus the first 5 EI motion lines).
- `gnome-shell --version`, `/usr/libexec/xdg-desktop-portal --version` (or the package version) and the distribution.
- Whether the consent dialog appeared on the second `wlcapture` and the second `wlinject` run, and what the injected
  square, click and text did on screen.
