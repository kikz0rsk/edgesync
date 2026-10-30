# dnd - Linux drag-and-drop, GNOME switch indicator and Gio file-drop spike (P0-SP3-007)

Item P0-SP3-007 (FEAT-177, FEAT-079, FEAT-111, FEAT-114, FEAT-115). Decisions and risks under test: D-12 (drag files
across screens), D-13 (spike first, own phase P4), D-18 (switch-indicator overlay), D-33 (Gio GUI, file drop native
per OS and bound to the Gio window handle), D-34 (agent is cgo-free, GUI may use cgo on Linux), D-39 (xgb, godbus),
D-92 (X11 observation without a grab uses RECORD), D-98 and D-99 (path rules and limits), D-100 (cgo allowed in
`gui/drop` on Linux Wayland), D-103 (in-house Wayland client codec for the agent), R-22 (no positionable overlay on
GNOME), R-29 (Wayland drag initiation needs an implicit-grab serial), R-30 (Gio has no OS file drop).

Nothing in this item was run: agents never run spike binaries or open an X connection or a D-Bus session (D-74). Every
statement below carries one of three evidence levels:

- **verified (source)**: read in a primary source or in the module source of the pinned version, cited;
- **INFERENCE**: reasoned from verified facts, not observed;
- **UNVERIFIED**: needs the GNOME box or an X server; the manual script P0-SP3-008 lists what to observe.

## Sources (cited, all read on 2026-10-30)

| Source | Version read | Used for |
|---|---|---|
| XDND specification, https://www.freedesktop.org/wiki/Specifications/XDND/ | **unreachable**: WebFetch returned HTTP 403 for the URL and for the `/XDND` variant; the Wayback copy is blocked by the tool | see next row |
| XDND specification at its author's site, https://johnlindal.wixsite.com/xdnd (landing page, `/technical-details`, `/example-walk-through`, `/optimizations`) | landing page "Current version: 5", "Last updated July 30, 2010"; the technical-details page is headed version 4 but documents the version 5 `XdndFinished` fields | message layouts, flags, `XdndAware`, `XdndProxy`, `XdndTypeList`, version negotiation, the source walk-through (status coalescing, release, leave without waiting, timeouts). The `/protocol` page returned only its introduction, so the per-message rules of that page are taken from the other pages and marked second-source where they rest on GLFW only |
| GLFW 3.4 `src/x11_window.c` (zlib licence, permissive, D-27 allows it), https://raw.githubusercontent.com/glfw/glfw/3.4/src/x11_window.c | tag 3.4 | **second-source** for the target side as real toolkits implement it (`XdndAware` as `XA_ATOM`, version in `data.l[1] >> 24`, list flag bit 0, position packing, `XdndStatus` reply, timestamp use in `XConvertSelection`, `XdndFinished` reply) |
| Wayland core protocol, https://wayland.freedesktop.org/docs/html/apa.html (protocol appendix) and https://wayland.app/protocols/wayland (rendering of the same XML, interface versions: `wl_data_device_manager` 4, `wl_data_device` 4, `wl_data_source` 4, `wl_data_offer` 4, `wl_pointer` 11, `wl_seat` 11) | `wayland.freedesktop.org` page covers `start_drag` and the data-device semantics (first 100000 characters read); the raw `wayland.xml` on gitlab.freedesktop.org was behind a bot check and the GitHub mirror path returned 404 | `wl_data_device.start_drag`, `enter`, `leave`, `drop`, `wl_data_source` events, `wl_data_offer.finish` |
| xdg-desktop-portal at tag `1.22.1` (the tag `../../portal/README.md` uses), `data/org.freedesktop.portal.Notification.xml` and `data/org.freedesktop.host.portal.Registry.xml`, and the `data/` directory listing through the GitHub API | tag 1.22.1 | Notification portal interface (documentation describes version 2) and Registry; **there is no drag-and-drop portal**: the 32 `org.freedesktop.portal.*` and 24 `org.freedesktop.impl.portal.*` files at that tag contain no Drag, Drop, DnD or DataDevice interface. The only transfer-related ones are `Clipboard`, `FileTransfer` and `Documents` |
| StatusNotifierItem specification, https://www.freedesktop.org/wiki/Specifications/StatusNotifierItem/ | **unreachable** (HTTP 403) | **second-source**: KDE's interface XML `org.kde.StatusNotifierItem.xml` from invent.kde.org `kstatusnotifieritem` master (properties, methods, signals) |
| GNOME Shell `js/ui/shellDBus.js`, `js/ui/layout.js` (gitlab.gnome.org, main) | main as of 2026-10-30 | `ShowOSD` access control, `addTopChrome` semantics |
| GNOME Mutter `NEWS` (gitlab.gnome.org, main) | latest entry 51.1 (first 100000 characters read) | no mention of layer-shell |
| wlr-layer-shell compositor support table, https://wayland.app/protocols/wlr-layer-shell-unstable-v1 | Mutter 51 listed as not supporting it | layer-shell absence on GNOME |
| GNOME Shell AppIndicator extension README, https://github.com/ubuntu/gnome-shell-extension-appindicator (master) | master | what the extension hosts, not enabled by default on the distributions the page names, no tooltips |
| Gio `gioui.org` v0.10.3 module source (Go module cache) | v0.10.3 | `app` and `io/transfer` behaviour; file and line numbers cited below |
| `github.com/jezek/xgb` v1.3.1, `github.com/godbus/dbus/v5` v5.2.4 | as pinned in `spikes/go.mod` | signatures read from the module source |

No code was taken from GPL projects (Deskflow, lan-mouse, Input Leap, Barrier, go-x11-client). Reference reading of GLFW
(zlib) was limited to the behaviour of the XDND messages.

## Code layout

- `uri.go`: `ParseURIList` (RFC 2483 lines, CRLF or LF, comments, NUL refused), `FilePath` (file URI to decoded path:
  empty host or `localhost` only, no query or fragment, percent-decoding through `net/url`), `ValidatePath` (D-45, D-98,
  D-99: absolute, UTF-8, no NUL or control characters, no `.`, `..` or empty components, none of `<>:"|?*` in a
  component, 255 bytes per component, 4096 per path), `ResolveURIList` (per-entry rejections as D-98 requires, 100000
  entries as D-99), `EncodeURIList`.
- `xdnd.go`: atom names, message kinds, the version 5 `ClientMessage` layouts as `EnterMsg`, `PositionMsg`, `StatusMsg`,
  `LeaveMsg`, `DropMsg`, `FinishedMsg` with `Encode` and `Decode`, `NegotiateVersion`.
- `target.go`: `TargetState`, the pure drop-target state machine (type list loading, accept or decline per position,
  selection conversion on drop, `XdndFinished` only from version 2, late messages ignored).
- `source.go`: `SourceState`, the pure drag-source state machine (enter on an aware window, proxy addressing, position
  coalescing while a status is outstanding, status rectangle suppression, drop or leave on release, finish and timeout
  rules).
- `session.go`, `drag.go`, `drop.go`: the xgb driver (`Display` with an event pump goroutine, `Source`, `Target`,
  `NewProxyTarget`, window-tree search for the aware window with `XdndProxy` validation). Selection data uses the
  X11 selection protocol; an INCR answer to the drop is reported as `ErrINCRNotSupported` (this spike does not receive
  INCR; the INCR helpers of the x11 spike are unexported and a different package).
- `xdnd_test.go`, `target_test.go`, `source_test.go`, `uri_test.go`: host tests, no X server (about 100 percent of the
  statements of the four pure files).
- `dnd_integration_test.go` (`//go:build integration && linux`): source to target on one X server, release outside any
  target, leave delivery, a drop on a foreign window through `XdndProxy`, stale proxy property ignored. Compiled and
  vetted only here.
- `../cmd/dndx11/main.go`, `../cmd/wloverlay/main.go` (+ `gio_linux_cgo.go`, `gio_other.go`),
  `../cmd/giodroplinux/main.go` (+ `main_other.go`): the manual drivers.

## Build

From the repository root. The non-Gio parts cross-compile from Windows; on the GNOME box drop the two variables.

```
GOOS=linux CGO_ENABLED=0 go -C spikes build -o bin/dndx11 ./linux/cmd/dndx11
GOOS=linux CGO_ENABLED=0 go -C spikes vet ./linux/...
GOOS=linux CGO_ENABLED=0 go -C spikes vet -tags integration ./linux/...
go -C spikes test ./linux/dnd/...
```

`wloverlay` and `giodroplinux` need cgo on Linux for the Gio window. Build them on the GNOME box (Go 1.27.3 and the Gio
dependencies of the spikes workflow: `gcc pkg-config libwayland-dev libx11-dev libx11-xcb-dev libxkbcommon-dev
libxkbcommon-x11-dev libgles2-mesa-dev libegl1-mesa-dev libegl-dev libffi-dev libxcursor-dev libxfixes-dev
libvulkan-dev`):

```
go -C spikes build -o bin/wloverlay ./linux/cmd/wloverlay
go -C spikes build -o bin/giodroplinux ./linux/cmd/giodroplinux
```

`wloverlay` built with `CGO_ENABLED=0` still runs `-mode notify` and `-mode x11`; `-mode gio` then fails with an explicit
error. `giodroplinux` without cgo is a program that prints why it cannot run and exits 2 (Gio does not compile on Linux
without cgo: `internal/vk` excludes every file).

## Spike 1: XDND source and target (`dndx11`), user run inside Xephyr

Nautilus is a single-instance GApplication, so a second `nautilus` started inside Xephyr on the same session bus would
hand over to the running Wayland instance. Give the nested session its own bus:

```
Xephyr :2 -screen 1280x800 -ac &
DISPLAY=:2 openbox &                # optional window manager, any will do
dbus-run-session -- env DISPLAY=:2 GDK_BACKEND=x11 nautilus --new-window "$HOME"
```

`pcmanfm` or `thunar` are acceptable substitutes when Nautilus does not start under bare Xephyr. Record which file
manager and version was used.

Target (a file manager drags a file onto the spike window):

```
DISPLAY=:2 ./bin/dndx11 -mode target -duration 3m
```

Drag a file from the file manager onto the "edgesync dndx11 target" window. Observe the log lines: `entered` or
`unsupported` with the list of offered types (`offered=`), `dropped` with `paths=` and the `XdndFinished` that is sent
afterwards (the file manager's drop progress ends). A file with `:` or `*` in its name must appear as `entry rejected`
(D-98).

Source (the spike window drags a file URI into the file manager):

```
DISPLAY=:2 ./bin/dndx11 -mode source -duration 3m            # synthetic out/dndx11-sample.txt
DISPLAY=:2 ./bin/dndx11 -mode source /some/file /some/dir   # or explicit paths
```

Press button 1 on the "edgesync dndx11 source" window and drag onto a folder view or a folder icon of the file manager,
release. Button 3 during the drag cancels. Observe: the `drag ended` line (`outcome=dropped|rejected|timed-out|no-target`),
whether the file was copied into the folder, whether the file manager showed a copy cursor while hovering, and the
`elapsed` time. Try also: dropping on a non-aware window (outcome `no-target`), moving the pointer from the file manager to
a second window and back (leave and enter), releasing very quickly after entering a window (the source sends
`XdndLeave` without waiting when it never received a status, as the specification's walk-through says).

The source never offers `move` and the target always answers `copy` (D-62: the source files are never deleted).

## Spike 2: switch-indicator candidates on GNOME Wayland (`wloverlay`), user run

GNOME's compositor (Mutter) has no wlr-layer-shell (verified: wayland.app compositor table, Mutter 51 not listed as
supporting it; the Mutter `NEWS` file has no entry for it), and the XDG shell has no request to position or raise a
toplevel. D-18 asked for an overlay "shown 800 ms, fades 200 ms"; no candidate below can promise that on GNOME.

Run each candidate, with another application focused (type in Text Editor) and then with a full-screen video or
fullscreen window, and record the observations:

```
./bin/wloverlay -mode notify -text "EdgeSync: Laptop" -duration 5s
./bin/wloverlay -mode notify -hint transient -priority high -duration 5s
./bin/wloverlay -mode notify -count 5 -interval 700ms -same-id          # does each one replace the previous?
./bin/wloverlay -mode notify -count 3 -interval 500ms                    # separate ids: do they stack?
./bin/wloverlay -mode notify -app-id <desktop-file-basename> -duration 5s   # Registry.Register first
DISPLAY=:0 ./bin/wloverlay -mode x11 -duration 5s                        # XWayland override-redirect window
./bin/wloverlay -mode gio -duration 5s                                   # native Wayland Gio window
WAYLAND_DISPLAY= DISPLAY=:0 ./bin/wloverlay -mode gio -duration 5s       # Gio on XWayland
```

`-mode notify` records the portal `version` and `SupportedOptions`, the time `AddNotification` takes, `ActionInvoked`
signals and the `RemoveNotification` result. Without `-app-id` the application is not registered with the portal;
`org.freedesktop.host.portal.Registry.Register(app_id, options)` is called on the portal object
`/org/freedesktop/portal/desktop`, which is an assumption (the XML declares its node as `/`): an error is returned
verbatim. The id must match a `.desktop` file basename and registration must precede every other portal call
(documented in the XML at 1.22.1).

Observe for each candidate: where on screen it appears (and on which monitor with two monitors), how long it stays and
whether GNOME removes it earlier or later than `-duration`, whether it steals keyboard focus (keep typing in another
window), whether it covers a fullscreen window, whether it shows when the screen is locked, whether it leaves an entry in
the notification list afterwards, and the first-paint delay (`first frame` for Gio, `took` for X11, `call_took` for
the portal). For `-mode x11` the log prints the requested and the measured root coordinates; for `-mode gio` it prints
every `ConfigEvent` (actual decorated state and size) and the view event kind.

Facts that shape the candidates:

- Notification portal (verified, xdg-desktop-portal 1.22.1 XML): `AddNotification(s id, a{sv} notification)` and
  `RemoveNotification(s id)`; keys `title`, `body`, `priority` (`low`, `normal`, `high`, `urgent`), `icon`, `buttons`,
  `default-action` and, since interface version 2, `display-hint` (`transient`, `tray`, `persistent`,
  `hide-on-lockscreen`, `hide-content-on-lockscreen`, `show-as-new`; `transient` and `tray` must not be combined),
  `category`, `sound`. There is no position, duration or opacity key: the compositor decides. Which backend serves it on
  GNOME and whether `transient` is honoured is UNVERIFIED.
- Gio windows (verified, v0.10.3): `app.Decorated(false)` is honoured by the platform layer, there is no position option
  (options are `Title`, `Size`, `MaxSize`, `MinSize`, `StatusColor`, `NavigationColor`, `CustomRenderer`, `Decorated`,
  `TopMost`, plus the window mode), and `app.TopMost` is documented as supported on macOS and Windows only
  (`app/window.go:983-989`). On Wayland the compositor places a normal toplevel and gives it focus (INFERENCE from the
  XDG shell model, to be observed).
- X11 override-redirect windows under XWayland (INFERENCE): the X server honours the requested position and Mutter maps
  override-redirect windows as unmanaged surfaces above normal windows; Mutter 50 "considers override-redirect windows
  for monitor fullscreen" (Mutter `NEWS`, 50.beta), so a covering window may be treated like a fullscreen one. Whether
  such a window is stacked above native Wayland windows, and whether XWayland is running when no X client exists (it
  starts on demand), is UNVERIFIED. The text is drawn with the core font `fixed`; a missing font fails loudly (install the
  X11 misc fonts).
- GNOME Shell `ShowOSD` (verified, `shellDBus.js`): every call goes through a sender check that admits only
  `org.gnome.Settings`, `org.gnome.SettingsDaemon.MediaKeys` and `org.freedesktop.impl.portal.desktop.gnome`; `Eval` needs
  `unsafe_mode`. Neither is available to an application. Not implemented.
- GNOME Shell extension (verified, `layout.js`): `Main.layoutManager.addTopChrome(actor)` adds an actor "above all
  windows, including popups" and `trackFullscreen` can hide it over fullscreen windows. This is the only route to a true
  on-screen overlay on GNOME, but it is JavaScript inside the Shell, must be installed and enabled by the user, tracks
  the Shell's extension API per release, and needs a D-Bus bridge to the agent. Not implemented here (new deliverable
  and a new dependency class), recorded as the decision to put to the user.
- StatusNotifierItem (second-source: KDE interface XML): `org.kde.StatusNotifierItem` has `IconName`, `IconPixmap`,
  `OverlayIconName`, `AttentionIconName`, `ToolTip`, `Status` and the signals `NewIcon`, `NewAttentionIcon`,
  `NewToolTip`, `NewStatus`. GNOME Shell has no host for it; the AppIndicator extension adds one, is not enabled by default
  on the distributions its README names and "doesn't support tooltips". An SNI icon can only change the tray icon (state
  indicator), it cannot show the machine name as a transient overlay. P2-X1 already builds the SNI tray (R-21); this item
  adds no SNI code.

Recommendation (GNOME indicator): use the Notification portal as the Wayland switch indicator. It needs no extension,
no cgo, and no surface of ours, and it works with the D-Bus session the agent already uses for portals (P2-Y1-016). It is
**not an on-screen overlay with the D-18 timing**, so this is a user decision per `architecture.md` 4.7 and R-22. The
optional second step, a GNOME Shell extension using `addTopChrome`, is the only way to meet D-18 literally; its cost is
listed above. The XWayland override-redirect window is a possible third option for X11-session fallback behaviour, pending
the observed stacking.

## Spike 3: Gio file drop on Linux (`giodroplinux`), user run

```
./bin/giodroplinux -duration 2m                                  # whatever display server Gio picks (Wayland on GNOME)
WAYLAND_DISPLAY= DISPLAY=:0 ./bin/giodroplinux -duration 2m      # force the X11 path under XWayland
WAYLAND_DISPLAY= DISPLAY=:0 ./bin/giodroplinux -x11-proxy -duration 2m
```

Drag files from Nautilus onto the window in each run. The log prints the environment, the Gio version, the view event
(`kind=wayland` with `display_set`, `surface_set`, `toplevel_set`, or `kind=x11` with the window id) and every Gio
transfer event received for `text/uri-list` and `text/plain`. With `-x11-proxy` the program installs `XdndAware` and
`XdndProxy` on the Gio window through a second X connection and logs the `XDND` events (offered types, dropped paths).

What the module source says (verified, Gio v0.10.3):

- `io/transfer` brokers drags **inside the window** and clipboard reads: "When a user initiates a pointer-guided drag and
  drop transfer, the source as well as all potential targets receive an InitiateEvent" (`io/transfer/transfer.go`).
  Every producer of `transfer.DataEvent` in `app/` is a clipboard read of type `application/text`
  (`os_x11.go:758`, `os_wayland.go:1066`, `os_windows.go:851`, `os_macos.go:405`, `os_js.go:106`, `os_android.go:1346`,
  `os_ios.go:291`). No platform delivers an OS file drop.
- X11: `app/os_x11.go` handles one `ClientMessage` (`WM_DELETE_WINDOW`, `os_x11.go:810-820`); there is no `XdndAware`
  property and no `Xdnd*` atom anywhere in `app/`. Gio does not advertise drop support, so file managers refuse the drop.
  `app.X11ViewEvent{Display unsafe.Pointer, Window uintptr}` (`app/os_unix.go:14-20`) gives the window id.
- Wayland: Gio binds `wl_data_device_manager` at version 3 (`os_wayland.go:729`) and installs a `wl_data_device`
  listener, but its `enter`, `leave`, `motion` and `drop` handlers are empty (`os_wayland.go:756-773`; the C listener table
  is `os_wayland.c:100-106`); only `selection` is used, for the clipboard. `app.WaylandViewEvent{Display, Surface,
  Toplevel unsafe.Pointer}` (`app/os_unix.go:27-34`) exposes the `wl_display`, the `wl_surface` and the `xdg_toplevel`.

What follows (INFERENCE unless noted):

- **X11 (possible, pure Go)**: set `XdndAware` (version 5) on the Gio window and `XdndProxy` on it pointing at a window
  of our own connection. A source's `XSendEvent` with an empty event mask is delivered to the client that created the
  destination window, which is Gio's Xlib connection; the proxy redirects the messages to our connection, which Gio's
  event loop never sees. This is the XDND proxy mechanism of the specification (version 4: "the proxy is used only to
  check XdndAware and as the target of XSendEvent"; the message `data.l[0]` and the event window still name the window under
  the pointer, verified from the technical-details page). The spike implements it in `Display.NewProxyTarget` and the
  source side follows a valid proxy (a proxy must name itself, otherwise it is stale and ignored) in `targetAt`.
  `-x11-proxy` is the experiment on a real Gio window.
- **Wayland (possible only with native code in the GUI process)**: Gio's view event hands out the `wl_display`. The GUI
  (cgo is allowed there, D-100) can create a separate event queue, wrap the display (`wl_proxy_create_wrapper`), bind its
  own `wl_data_device_manager` and `wl_seat`, create a second `wl_data_device` for the seat and attach a listener for
  `enter`, `motion`, `drop` and the offer events, dispatching that queue on its own thread with
  `wl_display_prepare_read_queue` next to Gio's loop. Whether Mutter delivers the drag events to every `wl_data_device` of
  the focused client (and not only to the one Gio created) is UNVERIFIED and is the first thing to test; the
  specification text read does not state a restriction to one data device per seat and client. A fallback that avoids the
  question is a small patch to Gio (empty handlers) which is out of scope (vendoring a modified Gio).
- Gio's own drag events are not an option for file managers: `transfer.TargetFilter` only sees drags that start in the
  same window.

Recommendation (Gio file drop): X11 through `XdndProxy` on the Gio window (pure Go, bound to the Gio window handle as
D-33 says); Wayland through a second `wl_data_device` on Gio's `wl_display` from cgo code in `gui/drop` (D-100, R-30). If
the second data device does not receive the events on the GNOME box, the fallback is "GUI drop unsupported on Wayland,
use the file picker and the tray", a user decision per R-30.

## Analysis: drag and drop on Wayland (GNOME, D-12 and R-28, R-29)

There is no drag-and-drop portal. At tag 1.22.1 the portal frontend has no interface for it (directory listing above);
`Clipboard` carries `application/vnd.portal.filetransfer` only for clipboard content, `FileTransfer` and `Documents`
serve sandboxed file access. A Wayland drag is therefore the core protocol and the compositor is the only broker.

### (1) Outgoing drag: detecting a local drag that leaves through an edge

Protocol facts (verified, Wayland core): a drag is a `wl_data_device.start_drag` by the source client; the target client
receives `wl_data_device.enter` with a `wl_data_offer` (the offered MIME types arrive as `wl_data_offer.offer` events,
and `wl_data_offer.receive` fetches the data over an fd before or after the drop), then `motion`, and either `leave` or
`drop`; `drop` is "sent when a drag-and-drop operation is ended because the implicit grab is removed". The drag grab
is held by the compositor for the duration.

Consequences for EdgeSync:

- An application can see a foreign drag only when one of its own surfaces is under the pointer: the `enter` event goes to the
  client whose surface has the pointer. Detecting a drag "approaching an edge" therefore needs surfaces of ours along
  the edge ("armed edge strips", P4-F5-007): transparent surfaces with an input region at the screen edge. The agent
  has no layer-shell on GNOME, so those would be toplevels the compositor places itself (fullscreen on an output with an
  input region is the only deterministic placement the XDG shell allows). Whether a transparent fullscreen toplevel can
  coexist with GNOME's shell chrome (top bar hidden for fullscreen windows) without visible side effects is UNVERIFIED
  and likely the deciding risk.
- Whether an InputCapture **activation happens while a drag is in progress** is not documented anywhere that was read.
  The portal text says "The compositor is always in control of input capturing"; the pointer barrier is a compositor
  constraint on pointer motion. INFERENCE: the barrier fires regardless of a client drag, activation then redirects all
  pointer input to the EIS connection, the drag grab no longer receives motion or the button release, and the source
  client sees its drag stuck or cancelled when capture ends. The observable alternatives are: (a) activation occurs and the
  drag is cancelled or stuck; (b) activation occurs and the drop is delivered on release at the cursor position given to
  `Release`; (c) no activation while a drag grab is active (the barrier is suspended during grabs).

  Experiment with the already merged tool, no new code: on the GNOME box start `./bin/wlcapture` (phase `right-edge`,
  see `../portal/README.md`), then in Nautilus press and **hold** the left button on a file, drag to the right edge of the
  rightmost monitor and keep holding. Record: whether `Activated` is logged, whether the Nautilus drag icon disappears
  or the file list freezes, what happens on button release (the log shows the EI events if the capture is active),
  and after `Release` whether the pointer reappears with the drag still alive. Repeat with the drag started from a Firefox
  link and from an XWayland application (which Xwayland bridges to a Wayland drag).
- Recommendation (outgoing): armed edge-strip surfaces, if and only if experiment (1) shows activation or an equivalent hook
  works during a drag; otherwise outgoing Wayland drags are unsupported and the product statement is "on GNOME Wayland,
  drag files with copy and paste or the EdgeSync window". This is an unconfirmed scope reduction and a user decision per R-28.

### (2) Incoming drag: starting a drag for files that arrive from another machine

`wl_data_device.start_drag(source, origin, icon, serial)` (verified, `apa.html` and `wayland.app`): "the client must have
an active implicit grab that matches the serial" on the origin surface, and a used source cannot be reused. The serial is
that of the `wl_pointer.button` press (or touch down) event the compositor sent to that same client. A client cannot
start a drag out of thin air; no portal can start one for it.

Could an injected press supply the serial? Chain (INFERENCE): the RemoteDesktop portal with EI (`wlinject`, already proven
to inject buttons) feeds events into Mutter as a virtual input device; Mutter treats them like physical input and sends
`wl_pointer.enter` and `wl_pointer.button` with a fresh serial to the surface under the pointer. If an agent-owned
surface is mapped, has pointer focus, and an EI button press is injected while the pointer is over it, the agent receives
`wl_pointer.button(serial)` and can call `start_drag` with the offer source (`text/uri-list`, staged files) at once.
After that the drag grab belongs to the compositor: injected relative motion moves the drag icon, an injected release
drops it on whatever surface is under the pointer (a file manager window, or the desktop), and the file manager reads
the URI list through `wl_data_source.send` (the agent writes the staged `file://` paths to the fd). This is the plan of
P4-F5-008 (`SourceState_SerialFromButtonPressOnOwnSurface`, `InputRegionClearedAfterStartDrag`, `ReleaseInjectsButtonUp`).

Obstacles, all UNVERIFIED and each needing the GNOME box:

- The origin surface must be under the injected pointer at the moment of the press, but the XDG shell gives no client control
  over toplevel placement. The deterministic cover is a transparent fullscreen toplevel on the output with an input region
  limited to the press point; the same shell-chrome side effects as in (1) apply, and the pointer must first be moved over
  the surface by EI (the absolute position of the pointer on the output is known from the EIS device regions).
- Mutter validates the serial against its own record of the last implicit grab; an EI-injected press must be recorded like a
  physical one. Not read in Mutter's source; the check is on the box.
- While the grab is active the drop target is chosen by the compositor from the pointer position, which EI motion controls.
  The final `wl_data_source.dnd_finished` (version 3) tells the agent that the destination is done, and
  `wl_data_source.cancelled` that the drop failed.

Recommendation (incoming): feasible in principle and the only mechanism available; high risk (R-29). Prototype with the
in-house Wayland client of D-103 in P4-F5; the check on the box is: map the transparent surface, inject a press with
`wlinject`, log the `wl_pointer.button` serial and whether `start_drag` is accepted. Without that experiment the decision to
keep Wayland incoming drags in scope cannot be made. If it fails, Wayland incoming drags fall back to "files land in
the staging directory and are pasted", again a user decision.

### (3) X11: XDND is feasible both ways (verified protocol, implemented and unit tested, run pending)

- Target: the logic of `TargetState` is the complete drop target for `text/uri-list`: `XdndAware` 5 on a top-level window,
  `XdndEnter` (types inline up to three, the rest in `XdndTypeList` of the source), `XdndPosition` answered with
  `XdndStatus`, `XdndDrop` then `ConvertSelection(XdndSelection, text/uri-list, time)`, parse, `XdndFinished` (from version
  2; the version 5 accepted flag and the performed action only for version 5 sources).
- Source: `SourceState` plus the driver: own `XdndSelection`, find the deepest aware window under the pointer with
  `TranslateCoordinates` down from the root (window managers add layers, so subwindows are searched), follow `XdndProxy`,
  one outstanding `XdndPosition` with coalescing, status rectangle suppression, drop on release when the last status
  accepted, leave when no status ever arrived or when the status times out (`DefaultStatusTimeout` 2 s), give up on
  `XdndFinished` after `DefaultFinishTimeout` 30 s. The specification gives no timeout values ("a reasonable amount of
  time"), these two are spike choices.
- Outgoing without owning the drag (EdgeSync is not the source): a foreign client holds the pointer grab for the whole
  drag, so a second `GrabPointer` fails and warp-to-centre deltas are impossible. What does work is the target side: an
  `XdndAware` window at the screen edge receives `XdndEnter` and `XdndPosition` from the foreign source, and the offered
  paths are read with `XConvertSelection` on `XdndSelection` as soon as the drag enters (the specification allows
  inspecting the data during the drag). Pointer position for the edge contact while the grab is foreign comes from RECORD
  (D-92). Detection plus the edge window is verified in logic (`TargetState`), the Xephyr run confirms it with a real file
  manager.
- Interop details that bit toolkits (INFERENCE from GLFW and the specification text): a target that sets an empty status
  rectangle receives a position for every pointer move; the timestamp of `XdndPosition` and `XdndDrop` must be used in
  the conversion (version 1 and above); a data type may arrive through INCR (this spike reports it as an error; P4 reuses
  the X1 INCR helpers per D-102).
- Move semantics: the source requests the data then sends the `DELETE` target before `XdndFinished` for a move. EdgeSync
  never performs or requests a move (D-62); the target always answers copy.

## Recommendations (one line each)

- Linux DnD approach: X11 via XDND source and target as implemented here (go for P4-F4, edge windows plus RECORD for the
  foreign grab); Wayland GNOME incoming via a compositor drag started with an EI-injected press on an agent surface
  and outgoing via armed edge strips, both gated on the box experiments above and both user decisions (R-28, R-29).
- GNOME indicator approach: Notification portal banner (not an on-screen overlay, user decision per R-22); optional GNOME
  Shell extension with `addTopChrome` as the only literal overlay; XWayland override-redirect window only as a measured
  candidate.
- Gio file drop: X11 by `XdndAware` plus `XdndProxy` on the Gio window (implemented and unit tested here); Wayland by a
  second `wl_data_device` on `WaylandViewEvent.Display` from cgo code in `gui/drop`; if Mutter does not deliver the events
  to it, Wayland GUI drop is unsupported (user decision per R-30).

## What could not be verified here, and why

- Anything that needs an X server or a Wayland session: the XDND exchange with a real file manager, the integration
  tests, all stacking, focus and timing of the indicator candidates, whether Mutter delivers `wl_data_device` events to a
  second data device, whether an EI-injected press yields a usable `start_drag` serial, whether InputCapture activates
  during a drag, whether XWayland is running when `-mode x11` starts.
- The XDND specification page on freedesktop.org (HTTP 403) and the StatusNotifierItem specification page (HTTP 403):
  the first was read at the author's site, the second through KDE's interface XML (second-source).
- The `/protocol` page of the XDND specification (introduction only returned): the per-message rules not stated on the other
  pages (for example that a source must not send a new position while a status is outstanding) are taken from the
  walk-through page, which states them, and the target behaviour from GLFW (second-source).
- Raw `wayland.xml` (bot check on gitlab.freedesktop.org): the core protocol was read through the generated
  `wayland.freedesktop.org` documentation and `wayland.app`. Only the first 100000 characters of each were read, which
  cover the data-device interfaces.
- Mutter's own source for drag grab validation and pointer-barrier behaviour during grabs was not read.
- The Notification portal backend on GNOME (which backend, how `display-hint` is honoured, whether an unregistered host
  application is accepted) and the Registry object path.

## What to paste back (for P0-SP3-008 and the findings)

- `dndx11` source and target logs from Xephyr with the file manager name and version: offered types, outcomes, `elapsed`,
  whether the file arrived.
- `wlcapture` with a held drag: whether `Activated` fired, what happened to the drag, what the release did.
- `wloverlay` logs for every mode and a note per candidate on placement, duration, focus, fullscreen, lock screen, notification
  list; the Notification portal `version` and `SupportedOptions`; GNOME Shell and xdg-desktop-portal versions.
- `giodroplinux` logs for Wayland, X11 and `-x11-proxy`: view event kind and handles, any Gio transfer events, any `XDND`
  lines.
- Anything unexpected (X errors, D-Bus errors) verbatim; none of them is swallowed.
