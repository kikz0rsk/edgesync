# EdgeSync - platform internals research

Date of research: 2026-10-29. Target: Go, per OS (Windows 10/11, macOS 13+, Linux X11, Linux Wayland).

## 0. Method, confidence legend, version snapshot

- Microsoft Learn pages, Apple docs (via developer.apple.com/tutorials/data JSON), xdg-desktop-portal docs, X.Org protocol specs, wayland.app, go.dev and proxy.golang.org were read directly.
- Reference-project behaviour (Deskflow, lan-mouse, Mouse Without Borders) was verified by downloading and grepping the raw source files, not from summaries. Line numbers refer to the default-branch snapshot read on 2026-10-29 and will drift.
- Anything not confirmed from a primary source is tagged UNVERIFIED. Statements that are my own engineering inference (not from a source) are tagged INFERENCE.
- Apple header comments come from an old SDK mirror (MacOSX11.3.sdk, https://github.com/phracker/MacOSX-SDKs). They can lag current behaviour; where Apple's current docs differ, I say so.

Version snapshot (all read 2026-10-29):

| Item | Version | Source |
|---|---|---|
| Go toolchain | go1.27.3 stable (go1.26.10 also stable) | https://go.dev/dl/?mode=json |
| golang.org/x/sys | v0.49.0 (2026-10-06) | https://proxy.golang.org/golang.org/x/sys/@latest |
| github.com/ebitengine/purego | v0.11.2 (2026-10-15), README says "beta software", Apache-2.0 | https://proxy.golang.org/github.com/ebitengine/purego/@latest , https://github.com/ebitengine/purego |
| golang.design/x/clipboard | v0.11.0 (2026-09-26), MIT | https://proxy.golang.org/golang.design/x/clipboard/@latest |
| github.com/jezek/xgb | v1.3.1 (2026-04-18), BSD-style | https://proxy.golang.org/github.com/jezek/xgb/@latest |
| github.com/godbus/dbus/v5 | v5.2.4 (2026-10-13), BSD-style | https://proxy.golang.org/github.com/godbus/dbus/v5/@latest |
| xdg-desktop-portal | 1.22.0 released 2026-06-08 (latest dated entry in NEWS) | https://raw.githubusercontent.com/flatpak/xdg-desktop-portal/main/NEWS.md |
| libei | 1.4.1 announced Apr 2025; 1.5.0 is packaged by Debian (upstream announcement not found, UNVERIFIED) | https://lists.freedesktop.org/archives/wayland-devel/2025-April/044103.html , https://sources.debian.org/src/libei/1.5.0-3/ |
| Deskflow wire protocol | 1.8 | https://raw.githubusercontent.com/deskflow/deskflow/master/src/lib/deskflow/ProtocolTypes.h |
| lan-mouse | 0.11.0, GPL-3.0-or-later | https://raw.githubusercontent.com/feschber/lan-mouse/main/Cargo.toml |

---

## 1. Global input capture on the controlling machine

### 1.1 Windows

Facts from Microsoft docs:

- `WH_MOUSE_LL` and `WH_KEYBOARD_LL`: returning a nonzero value from the hook procedure "prevent[s] the system from passing the message to the rest of the hook chain or the target window procedure". If the proc does not process the event it should call `CallNextHookEx`, otherwise other LL hooks in the chain stop receiving events. https://learn.microsoft.com/en-us/windows/win32/winmsg/lowlevelmouseproc , https://learn.microsoft.com/en-us/windows/win32/winmsg/lowlevelkeyboardproc
- The hook runs in the context of the installing thread, delivered "by sending a message to the thread that installed the hook", so that thread "must have a message loop". Same pages.
- Timeout: the proc must finish within `LowLevelHooksTimeout` (ms, `HKEY_CURRENT_USER\Control Panel\Desktop`). On timeout the system passes the message on, but "on Windows 7 and later, the hook is silently removed without being called. There is no way for the application to know whether the hook is removed." Windows 10 1709+ caps the value at 1000 ms. Same pages. The default value is not stated on these pages (commonly quoted as 300 ms, UNVERIFIED).
- Microsoft's own advice: run LL hooks "on a dedicated thread that passes the work off to a worker thread and then immediately returns", and "in most cases ... monitor raw input instead". Same pages.
- Input from `mouse_event`/`keybd_event`/`SendInput` is reported as "injected" (flag `LLMHF_INJECTED` / `LLKHF_INJECTED`); Deskflow ignores injected mouse events on secondaries and lan-mouse uses the same hooks. https://raw.githubusercontent.com/deskflow/deskflow/master/src/lib/platform/MSWindowsHook.cpp (mouseLLHook, ~line 630) , https://raw.githubusercontent.com/feschber/lan-mouse/main/input-capture/src/windows/event_thread.rs
- Raw Input (`RegisterRawInputDevices`, `WM_INPUT`): `RIDEV_INPUTSINK` lets a non-foreground app receive input but "hwndTarget must be specified". `RIDEV_NOLEGACY` only stops legacy messages "for the application" that registered, so it cannot suppress delivery to other apps. Raw Input is therefore a monitoring API, not a blocking API. https://learn.microsoft.com/en-us/windows/win32/api/winuser/ns-winuser-rawinputdevice , https://learn.microsoft.com/en-us/windows/win32/inputdev/about-raw-input
- Raw Input gives per-device data and a buffered read path for 1000 Hz mice (`GetRawInputBuffer`). Same about-raw-input page. That RAWMOUSE reports relative deltas via `MOUSE_MOVE_RELATIVE` is UNVERIFIED in the pages I read (well-known, not re-checked).

How the references do it:

- Deskflow (GPL-2.0): in-process `SetWindowsHookEx(WH_MOUSE_LL / WH_KEYBOARD_LL)` on a dedicated desk thread with a `GetMessage` loop; the hook posts to the main thread with `PostThreadMessage` and returns true to "eat" events when the cursor is inside a jump zone. It also clamps coordinates to the local screen and uses a "bogus" flag. MSWindowsHook.cpp lines 590-700, MSWindowsDesks.cpp (per-desktop thread, `SetThreadDesktop`, `GetMessage` loop).
- To keep the local cursor from moving/clicking while the pointer is "away", Deskflow shows a full-screen (regular hooks) or 1x1 centred (LL hooks) window on the primary, and a hider window with `SetCapture` on secondaries; cursor hidden with a transparent cursor + `ShowCursor`. https://raw.githubusercontent.com/deskflow/deskflow/master/src/lib/platform/MSWindowsDesks.cpp
- lan-mouse (GPL-3.0): dedicated thread, `GetMessageW` loop, `SetWindowsHookExW(WH_MOUSE_LL)` + `WH_KEYBOARD_LL`, `PostThreadMessageW` for control. https://raw.githubusercontent.com/feschber/lan-mouse/main/input-capture/src/windows/event_thread.rs
- Mouse Without Borders (MIT) also uses `InputHook.cs` (LL hooks). https://github.com/microsoft/PowerToys/tree/main/src/modules/MouseWithoutBorders/App/Class

Doing it from Go without cgo:

- Yes, feasible. `golang.org/x/sys/windows` provides `NewCallback`, `NewLazySystemDLL`, services (`x/sys/windows/svc`, incl. `AcceptSessionChange`), tokens (`WTSQueryUserToken`, `DuplicateTokenEx`, `CreateProcessAsUser`, `SetTokenInformation`) but I grepped `zsyscall_windows.go` (v0.49.0 era master) and it has NO wrappers for `SetWindowsHookEx`, `CallNextHookEx`, `GetMessage`, `PeekMessage`, `PostThreadMessage`, `RegisterRawInputDevices`, `SendInput`, `EnumDisplayMonitors`, `GetSystemMetrics`, `SetProcessDpiAwarenessContext`, `OpenInputDesktop`, `SetThreadDesktop`, clipboard APIs or `CreateWindowEx`. You declare them yourself through `windows.NewLazySystemDLL("user32.dll")`. https://raw.githubusercontent.com/golang/sys/master/windows/zsyscall_windows.go , https://raw.githubusercontent.com/golang/sys/master/windows/svc/service.go
- Third-party Win32 bindings: `github.com/tailscale/win` (BSD-3, fork of lxn/win), `github.com/gonutz/w32` (BSD), `github.com/zzl/go-win32api` (MPL-2.0), `github.com/moutend/go-hook` (MIT, pure Go LL hooks, README says tested on Windows 10 64-bit only, last push 2020-05-20). Licenses read from the raw LICENSE files.
- `syscall.NewCallback` rules (Go source comment): "Only a limited number of callbacks may be created in a single Go process", at least 1024 are always available, callback memory is never freed, the function must have arguments no larger than a uintptr and one uintptr-sized result. https://raw.githubusercontent.com/golang/go/master/src/syscall/syscall_windows.go . Consequence: create one callback per hook proc / per COM method once at start-up and dispatch by `this` pointer; never create callbacks per event or per object.
- Thread affinity: the hook installer thread must pump messages, so the goroutine that calls `SetWindowsHookEx` and `GetMessage` must call `runtime.LockOSThread()` for its whole life. Evidence: Go issue 34301 (GetMessage loop froze after GC when not locked; fix was `LockOSThread` at top of main). https://golang.org/issue/34301
- INFERENCE: Go GC stop-the-world or a busy scheduler can delay the callback; a delayed callback beyond `LowLevelHooksTimeout` silently removes the hook. Keep the callback to a few non-blocking operations (atomic read of "capturing" flag, non-blocking channel/ring-buffer push, return 1 or `CallNextHookEx`), and run a liveness watchdog (periodically inject a marked synthetic event via `SendInput` with a private `dwExtraInfo` and re-install the hook if it is not seen).
- INFERENCE (not from a doc): the LL hook `pt` is clamped to the virtual screen, so relative motion past the edge is lost. Robust relative motion while capturing = LL hook swallows events, `WM_INPUT` (`RIDEV_INPUTSINK`, hidden message window) supplies deltas. Deskflow instead recentres/clamps; prototype both.

Alternatives (cursor warp / clip): Deskflow pins with a window + `SetCapture` rather than `ClipCursor`; I did not verify `ClipCursor`/`SetCursorPos` behaviour from docs. UNVERIFIED.

### 1.2 macOS

Facts:

- `CGEventTapCreate(tap, place, options, eventsOfInterest, callback, userInfo)`. Locations: `kCGHIDEventTap`, `kCGSessionEventTap`, `kCGAnnotatedSessionEventTap` (CGEventTypes.h). Active (`kCGEventTapOptionDefault`) taps can modify or discard events; listen-only cannot. Callback "is invoked from the run loop to which the event tap is added as a source". https://developer.apple.com/tutorials/data/documentation/coregraphics/cgevent/tapcreate(tap:place:options:eventsofinterest:callback:userinfo:).json
- Apple's current doc says "Only processes running as the root user may locate an event tap at the point where HID events enter the window server; for other users, this function returns NULL", and key up/down events are received only if root or "Access for assistive devices is enabled" (same URL). In practice Deskflow creates its tap at `kCGHIDEventTap` as a normal user with Accessibility trust (OSXComputer.mm:703), while lan-mouse uses `CGEventTapLocation::Session` (macos.rs:559). Both use the active (default) option. The doc/practice mismatch for the HID tap is real; test on macOS 13, 14, 15, 26 before choosing. https://raw.githubusercontent.com/deskflow/deskflow/master/src/lib/platform/OSXComputer.mm , https://raw.githubusercontent.com/feschber/lan-mouse/main/input-capture/src/macos.rs
- Permissions: Accessibility (`AXIsProcessTrusted`) is required for the active tap; Deskflow refuses to start as server if not trusted (OSXComputer.mm:134) and lan-mouse returns an `AccessibilityPermission` error (macos.rs:737). Since 10.15 the SDK has `CGPreflightListenEventAccess`/`CGRequestListenEventAccess` and `CGPreflightPostEventAccess`/`CGRequestPostEventAccess` (CGEvent.h lines 399-408). A non-Apple forum answer says active taps map to Accessibility and listen-only taps to Input Monitoring; contested in the same thread. https://developer.apple.com/forums/thread/122492 . Plan on requiring both Accessibility (capture + inject) and being ready to prompt for Input Monitoring if a listen-only tap is ever used (UNVERIFIED which TCC entries macOS 13-26 actually require for an active tap).
- Taps get disabled: the callback receives `kCGEventTapDisabledByTimeout` or `kCGEventTapDisabledByUserInput` and must call `CGEventTapEnable(tap, true)` (Deskflow does it on timeout, OSXComputer.mm:1711-1718; lan-mouse does too, macos.rs:463-493; CGEvent.h line 320-326).
- Cursor: `CGAssociateMouseAndMouseCursorPosition(false)` "disconnect[s] the mouse from the cursor ... events your application receives ... have a constant absolute location but contain delta updates"; the API is described as working "while an application is in the foreground". `CGWarpMouseCursorPosition` moves the cursor "without generating or posting an event". https://developer.apple.com/tutorials/data/documentation/coregraphics/cgassociatemouseandmousecursorposition(_:).json , https://developer.apple.com/tutorials/data/documentation/coregraphics/cgwarpmousecursorposition(_:).json
- Hiding the cursor from a background process: Deskflow calls `CGDisplayHideCursor` plus a PRIVATE CGS connection property `"SetsCursorInBackground"` via `_CGSDefaultConnection` (OSXComputer.mm:650-690). lan-mouse also uses `_CGSDefaultConnection` (macos.rs:821,873). Private API risk.
- Deskflow primary `leave()` sequence: hide cursor, `avoidHesitatingCursor`, then `CGAssociateMouseAndMouseCursorPosition(false)`; `enter()` re-associates and sets suppression interval (OSXComputer.mm:774-830). It also uses `CGSetLocalEventsSuppressionInterval` (deprecated since 10.6 in the SDK header; replacement is `CGEventSourceSetLocalEventsSuppressionInterval`).
- Deskflow notes that "Synthetic mouse button and drag events require event numbers on macOS 27 and later" (OSXComputer.mm:64): expect injection-side behaviour changes every major release.

Go without cgo: possible via purego. purego v0.11.2: Apache-2.0, "beta software", macOS amd64 and arm64 are Tier 1 with `CGO_ENABLED=0`, `NewCallback` supported (struct args/returns are not supported inside callbacks on platforms flagged in its footnote 2, which does not include macOS), `objc` subpackage has `RegisterClass(name, superClass, protocols, ivars, methods)`, `AddProtocol`, `NewBlock`; its docs warn "This package is easy to use incorrectly" and "Stable version" is unchecked. https://github.com/ebitengine/purego , https://pkg.go.dev/github.com/ebitengine/purego/objc . golang.design/x/clipboard v0.11.0 already drives AppKit/NSPasteboard through purego with no cgo (clipboard_darwin.go, go.mod requires purego v0.10.1), which is existence proof for the approach. https://raw.githubusercontent.com/golang-design/clipboard/main/clipboard_darwin.go
- Practical shape: a goroutine with `runtime.LockOSThread()` that creates the tap, adds `CFMachPortCreateRunLoopSource` to its `CFRunLoop`, calls `CFRunLoopRun`; callback via `purego.NewCallback` with signature (proxy, type, event, refcon) all pointer-sized. Deskflow runs its tap on a dedicated thread with its own CFRunLoop for the same reason (OSXComputer.mm ~line 725). UNVERIFIED in practice: passing `CGPoint` (two doubles) by value to `CGWarpMouseCursorPosition` via purego.
- TCC identity: Accessibility grants are tied to the executable identity. A bare Go binary is a poor TCC citizen; ship a signed `.app` bundle with stable bundle id (general macOS practice, not verified in this pass, UNVERIFIED).
- cgo is not required but a tiny Objective-C/Swift helper is the fallback if purego proves too fragile.

### 1.3 Linux X11

Facts:

- XInput2 raw events (`XI_RawMotion`, `XI_RawKeyPress/Release`, `XI_RawButtonPress/Release`) go to root windows; "Clients supporting XI 2.1 or later receive raw events at all times, even when the device is grabbed by another client". Good for relative deltas and for observing input while another client holds a grab. https://www.x.org/releases/current/doc/inputproto/XI2proto.txt
- Suppression needs an active grab. Deskflow grabs with `XGrabKeyboard` and `XGrabPointer` on its own override-redirect window (XWindowsComputer.cpp:1861, 1876), also selects `XI_RawMotion`/`XI_RawKeyRelease` on the root (1952-1958), uses `XWarpPointer` (1806), `XTestFakeMotionEvent`, `XTestGrabControl` (134), `XRRSelectInput` with `RRScreenChangeNotifyMask | RRCrtcChangeNotifyMask` (882), and Xinerama queries (925-928). https://raw.githubusercontent.com/deskflow/deskflow/master/src/lib/platform/XWindowsComputer.cpp
- Pure-Go options:
  - `github.com/jezek/xgb` v1.3.1 (active, last push 2026-04-18, BSD-style 3-clause text read from LICENSE): directories present include xproto (core requests: grabs, warp, keymap, selections), xtest, randr, xfixes, xinerama, record, shape, damage, composite, screensaver, shm. There is NO `xinput` and NO `xkb` directory. `xgbgen` is in the repo so XI2/XKB bindings can be generated from the XML protocol descriptions, but that is real work. https://github.com/jezek/xgb , directory listing via https://api.github.com/repos/jezek/xgb/contents/
  - `github.com/BurntSushi/xgb`: same layout, last push 2022-02-12 (dormant).
  - `github.com/linuxdeepin/go-x11-client`: has `ext/input` and `ext/xkb` (and xfixes, randr, record, test) but is GPL-3.0 and last pushed 2024-07-11. Licence conflict for a permissive project. https://github.com/linuxdeepin/go-x11-client
  - `github.com/jezek/xgbutil` (BSD-3): keybind/mousebind/xevent helpers on top of xgb.
- cgo option: libX11 + libXtst + libXi + libXrandr (what Deskflow and lan-mouse's x11 emulator use).
- INFERENCE: with xgb alone, a feasible pure-Go capture is: fullscreen override-redirect input window, `GrabPointer`/`GrabKeyboard`, derive deltas from `MotionNotify` with warp-to-centre (classic Synergy approach) and skip XI2 in v1; add generated XI2 raw events later for precise deltas. XRecord (`xgb/record`) is passive only; do not rely on it for suppression.
- Injection: XTest (`XTestFakeKeyEvent`, `XTestFakeButtonEvent`, `XTestFakeMotionEvent`, `XTestFakeRelativeMotionEvent`), available in xgb `xtest`. `XTestGrabControl` lets the client be "impervious to server grabs". https://www.x.org/releases/current/doc/xextproto/xtest.html

### 1.4 Linux Wayland

Portal facts (docs read at https://flatpak.github.io/xdg-desktop-portal/docs/):

- InputCapture portal, interface version 2: `CreateSession` (deprecated in v2), `CreateSession2`, `Start` (shows the consent dialog, supports `restore_token` and `persist_mode` 0/1/2, returns `clipboard_enabled` and new token), `GetZones` (zone_set id + width/height/x/y tuples), `SetPointerBarriers` (horizontal or vertical segments on the outer boundary of the union of zones, returns `failed_barriers`; setting barriers suspends the session so `Enable` must be called again), `Enable`, `Disable`, `Release(activation_id, optional cursor_position)`, `ConnectToEIS` (must be called before `Enable`), signals `Activated`, `Deactivated`, `Disabled`, `ZonesChanged`. "The compositor is always in control of input capturing." Events do not travel over D-Bus: after `ConnectToEIS` they arrive on the libei fd, and the portal `activation_id` equals the EIS `start_emulating` sequence. https://flatpak.github.io/xdg-desktop-portal/docs/doc-org.freedesktop.portal.InputCapture.html
- RemoteDesktop portal v2 for injection: `CreateSession`, `SelectDevices` (keyboard 1, pointer 2, touchscreen 4), `Start` (user dialog), `NotifyPointerMotion`, `NotifyKeyboardKeycode`, `NotifyKeyboardKeysym`, and `ConnectToEIS` (EIS recommended; after it the `Notify*` methods error). Persistence via `persist_mode` + single-use `restore_token`. https://flatpak.github.io/xdg-desktop-portal/docs/doc-org.freedesktop.portal.RemoteDesktop.html
- Portal release history: InputCapture and Clipboard portals introduced in 1.17.0 (2023-08-04) "centered around the InputLeap and Synergy use cases"; "Add Clipboard support to the Input Capture Portal" and "session persistence support to the Input Capture Portal" only in 1.21.1 (2026-04-08). https://raw.githubusercontent.com/flatpak/xdg-desktop-portal/main/NEWS.md . Whether the GNOME and KDE backends already implement v2 (persistence, clipboard) is UNVERIFIED; Deskflow guards for it ("clipboard not enabled on session, discarding restore token", PortalInputCapture.cpp:304).
- Clipboard portal (v1) attaches to RemoteDesktop or InputCapture sessions: `RequestClipboard` before start, `SetSelection(mime types)`, signals `SelectionOwnerChanged` and `SelectionTransfer(serial)`, `SelectionWrite`/`SelectionWriteDone`, `SelectionRead(mime)` returning an fd; for InputCapture "clipboard access works only while the session is active". File transfers use `application/vnd.portal.filetransfer`. https://flatpak.github.io/xdg-desktop-portal/docs/doc-org.freedesktop.portal.Clipboard.html

Compositor support (capture = InputCapture portal + EIS; injection = RemoteDesktop portal + EIS):

| Compositor | Capture | Injection | Evidence |
|---|---|---|---|
| GNOME / mutter | 45+ | 45+ | GNOME 45 release notes: "GNOME 45 allows this software [Input Leap] to be used with modern Wayland sessions" https://release.gnome.org/45/ ; lan-mouse README "GNOME >= 45" https://raw.githubusercontent.com/feschber/lan-mouse/main/README.md |
| KDE Plasma / KWin | 6.1+ | 6.1+ | Plasma 6.1 beta announcement lists "Support for the Input Capture portal" https://kde.org/announcements/plasma/6/6.0.90/ ; lan-mouse README "KDE Plasma >= 6.1" |
| Sway / wlroots | no InputCapture portal known | wlr virtual-pointer + virtual-keyboard protocols | xdg-desktop-portal-wlr issue #278 "InputCapture portal" open since 2023-08, last comment 2024-09, no maintainer plan visible https://github.com/emersion/xdg-desktop-portal-wlr/issues/278 ; lan-mouse uses layer-shell for capture and wlr protocols for injection (README tables) |
| Hyprland | no InputCapture portal found in XDPH README (UNVERIFIED for Oct 2026); lan-mouse code comments mention Hyprland handling EIS sessions (libei backend "hyprland leaks a keymap fd per eis session") so some EIS support exists, details UNVERIFIED | wlr protocols | https://raw.githubusercontent.com/feschber/lan-mouse/main/input-capture/src/libei.rs (comment near line 53) |
| Wayfire | needs newer-than-Oct-2023 build + shortcuts-inhibit plugin for lan-mouse layer-shell path | wlr protocols | lan-mouse README |

Known compositor bugs worth designing around (from lan-mouse libei.rs, lines 49-60): mutter does not deliver events after a session has been disabled once, so the session must be recreated whenever barriers change; mutter also kills the session when EI devices come and go.

Libraries and Go:

- libei = C libraries `libei` (client), `libeis` (server), `liboeffis` (D-Bus helper for RemoteDesktop). Roles: EI client sends or receives; server is the compositor. https://libinput.pages.freedesktop.org/libei/
- EI wire protocol: UNIX socket; header is 64-bit object id + 32-bit length + 32-bit opcode in the EIS implementation's native byte order, length includes the 16 header bytes, object 0 is `ei_handshake`, handshake-version must be the first request (summaries of overview.md at https://sources.debian.org/src/libei/1.5.0-3/doc/protocol/doc/overview.md and https://whot.pages.freedesktop.org/libei/doc/index.html). A from-scratch implementation exists in Rust (`reis`, MIT, https://github.com/ids1024/reis) and lan-mouse uses it (via the `reis` crate). I found NO Go binding for libei or the InputCapture portal.
- Go path: `godbus/dbus/v5` (BSD-style, pure Go, has `SupportsUnixFDs`/`EnableUnixFDs`) for portal calls + file-descriptor passing, plus your own EI wire codec over the received fd (needs `SCM_RIGHTS` for keymap fds, doable with `golang.org/x/sys/unix`). Est. effort: protocol codec for ~10 interfaces (handshake, connection, seat, device, pointer, pointer_absolute, button, scroll, keyboard, touchscreen). The alternative is cgo to libei/libportal (what Deskflow does, requiring libei >= 1.3 and libportal >= 0.8 per its README https://github.com/deskflow/deskflow ).
- Deskflow Wayland code: `PortalInputCapture.cpp` (libportal `xdp_input_capture_session_*`: request clipboard, persistence + restore token, zones, `set_pointer_barriers`, `connect_to_eis`, `release_at`, per-output zone handling), `PortalRemoteDesktop.cpp`, `EiComputer.cpp`, `EiKeyState.cpp`, `PortalClipboard.cpp`. https://github.com/deskflow/deskflow/tree/master/src/lib/platform
- Consent UX: `Start` shows a permission dialog; without a valid restore token the user is prompted per session. Plan for a first-run flow and a degraded mode when the portal denies.

---

## 2. Input injection on the controlled machine

### Windows

- `SendInput` is the API (Deskflow's MSWindowsDesks uses `SendInput` for keyboard and mouse, not `mouse_event`/`keybd_event`). https://learn.microsoft.com/en-us/windows/win32/api/winuser/nf-winuser-sendinput , https://raw.githubusercontent.com/deskflow/deskflow/master/src/lib/platform/MSWindowsDesks.cpp
- Absolute mouse: `MOUSEEVENTF_ABSOLUTE` takes coordinates normalised 0..65535; without `MOUSEEVENTF_VIRTUALDESK` they "map to the primary monitor"; with it they map to the whole virtual desktop. Deskflow does NOT use VIRTUALDESK (scales by `SM_CXSCREEN`/`SM_CYSCREEN`), so it effectively addresses the primary monitor. https://learn.microsoft.com/en-us/windows/win32/api/winuser/ns-winuser-mouseinput . For EdgeSync use `ABSOLUTE | VIRTUALDESK` normalised against `SM_XVIRTUALSCREEN/SM_YVIRTUALSCREEN/SM_CXVIRTUALSCREEN/SM_CYVIRTUALSCREEN`.
- Relative mouse moves are subject to pointer speed and acceleration (up to 4x per the doc). Deskflow temporarily sets mouse speed/acceleration to 1:1 with `SystemParametersInfo` around relative moves (MSWindowsDesks.cpp `deskMouseRelativeMove`). Prefer absolute moves, or do the same 1:1 dance.
- Wheel: `mouseData` is a multiple of `WHEEL_DELTA` (120); `MOUSEEVENTF_HWHEEL` for horizontal; `XBUTTON1/2` in `mouseData` for side buttons. Same MOUSEINPUT page.
- Keyboard: `KEYEVENTF_SCANCODE` "wVk is ignored", scan code "will always be the same" regardless of layout, `KEYEVENTF_EXTENDEDKEY` for 0xE0-prefixed keys, `KEYEVENTF_UNICODE` sends `VK_PACKET` for characters (wVk must be 0). https://learn.microsoft.com/en-us/windows/win32/api/winuser/ns-winuser-keybdinput . `SendInput` "does not reset the keyboard's current state" so stuck local modifiers can corrupt injected input (SendInput page).
- UIPI: "Applications are permitted to inject input only into applications that are at an equal or lesser integrity level", and the function "fails when it is blocked by UIPI. Note that neither GetLastError nor the return value will indicate the failure was caused by UIPI blocking." (SendInput page). So an unelevated EdgeSync agent silently cannot drive elevated windows.
- Fix for elevated windows: run the injector with a token that has `TokenUIAccess` set. Deskflow's watchdog duplicates `winlogon.exe`'s token (`DuplicateTokenEx`), calls `SetTokenInformation(..., TokenUIAccess, ...)` and starts the core with `CreateProcessAsUser` in the active session (MSWindowsWatchdog.cpp:118-160, 292; MSWindowsSession.cpp uses `WTSGetActiveConsoleSessionId`, `WTSQueryUserToken`). The manifest route (`uiAccess="true"`) requires Authenticode signing and a protected install location (`\Program Files\` or `\windows\system32\`). https://learn.microsoft.com/en-us/windows/win32/api/sas/nf-sas-sendsas ; AutoHotkey documents the same trusted-location rule https://www.autohotkey.com/docs/v2/Program.htm
- Secure desktop (UAC consent, Ctrl+Alt+Del screen, lock/sign-in): UAC prompts go to the secure desktop which "only Windows processes can access" (https://learn.microsoft.com/windows/security/identity-protection/user-account-control/how-user-account-control-works). Deskflow handles it with: a Windows service ("deskflow-daemon") + watchdog that restarts the core in the right session, per-desktop worker threads that poll `OpenInputDesktop` every 0.2 s and `SetThreadDesktop` to follow desktop switches (MSWindowsDesks.cpp, comment: Windows does not reliably notify), and a named event that makes the service call `SendSAS` from `sas.dll`.
- `SendSAS(AsUser)`: caller "must either be running as a service or have the uiAccess attribute"; the policy "Disable or enable software Secure Attention Sequence" must allow it (Deskflow comment: `SoftwareSASGeneration` must be 1). https://learn.microsoft.com/en-us/windows/win32/api/sas/nf-sas-sendsas , MSWindowsWatchdog.cpp:497-545.
- Security lesson: Deskflow's SYSTEM daemon exposed a named pipe `\\.\pipe\deskflow-daemon` with world access and unauthenticated commands (`command=`, `elevate=`, `start`); CVE-2026-41477, CVSS 7.8, Windows builds 1.20.0 through 1.26.0.160, fixed in 1.26.0.161. Any EdgeSync service IPC must be ACL-restricted and authenticated from day one. https://github.com/deskflow/deskflow/security/advisories/GHSA-6rx5-g478-775c , https://vuln.today/cve/CVE-2026-41477
- Clipboard in services: a service gets its own clipboard unseen by the user (golang.design/x/clipboard docs). Do clipboard work in the user-session process.
- Go: all of the above is reachable without cgo (`windows/svc`, `WTSQueryUserToken`, `DuplicateTokenEx`, `SetTokenInformation`, `CreateProcessAsUser` are in x/sys; `SendInput`, `OpenInputDesktop`, `SetThreadDesktop`, `SendSAS` via LazyDLL).

### macOS

- `CGEventPost` (Deskflow posts at `kCGHIDEventTap`: OSXComputer.mm:499, 582, 639; lan-mouse posts at `CGEventTapLocation::HID`, macos.rs emulation lines 224, 424, 504). Requires Accessibility; `CGPreflightPostEventAccess`/`CGRequestPostEventAccess` exist since 10.15 (CGEvent.h). Use `CGEventSource` with `CombinedSessionState` (lan-mouse). Mouse moves must carry `kCGMouseEventDeltaX/Y` to look like relative motion to games/apps (lan-mouse sets `EventField::MOUSE_EVENT_DELTA_X/Y`, macos.rs:422).
- Text injection that ignores layouts: `CGEventKeyboardSetUnicodeString` (CGEvent.h line 205).
- Modifier fidelity: lan-mouse documents that modifier events must carry the real virtual keycode, a bare `CGEvent` defaults to keycode 0 ("A") (macos.rs emulation ~line 230-250). Expect similar traps with `flagsChanged`.
- Deskflow/lan-mouse need Accessibility only; no Screen Recording.

### Linux X11

- XTest as above. Keycode translation requires the server keymap: `GetKeyboardMapping` (xproto) gives keycode -> keysyms; layout groups and modifiers need XKB (not in xgb). For characters absent from the layout, tools temporarily remap an unused keycode; Deskflow does this in XWindowsKeyState.cpp (UNVERIFIED in detail).

### Linux Wayland

- RemoteDesktop portal `NotifyKeyboardKeycode`/`NotifyKeyboardKeysym`/`NotifyPointerMotion` or, preferred, `ConnectToEIS` and send EI events (portal doc above). GNOME and KDE: portal path. wlroots: `zwlr_virtual_pointer_v1` and `zwp_virtual_keyboard_v1`; known caveat (lan-mouse README): modifier keys do not work on wlroots receivers when the sender is not on its layer-shell backend. https://wayland.app/protocols/wlr-virtual-pointer-unstable-v1 , lan-mouse README. A Go library for wlroots virtual input exists: `github.com/bnema/wayland-virtual-input-go` (found via search, quality UNVERIFIED).
- Deskflow requires libei >= 1.3 and libportal >= 0.8 for Wayland and says some features may be unavailable on Wayland. https://github.com/deskflow/deskflow

---

## 3. Multi-monitor and DPI

### Windows

- Declare Per-Monitor v2 in the manifest, not in code: Microsoft "recommended that you specify the default process DPI awareness via a manifest setting"; `<dpiAwareness>PerMonitorV2</dpiAwareness>` (Windows 10 1607+) together with `<dpiAware>true</dpiAware>` as fallback. The API `SetProcessDpiAwarenessContext` (min Windows 10 1703) "must" be called before any window or DPI-dependent call, fails with `ERROR_ACCESS_DENIED` if awareness is already set (including via manifest), and defaults to unaware if never set. https://learn.microsoft.com/en-us/windows/win32/hidpi/setting-the-default-dpi-awareness-for-a-process , https://learn.microsoft.com/en-us/windows/win32/api/winuser/nf-winuser-setprocessdpiawarenesscontext
- Go exes embed the manifest as a `.syso` resource: `github.com/tc-hib/go-winres` (permissive 0BSD-style licence text) or `github.com/akavel/rsrc` (MIT); both LICENSE files read. Whether these tools are still maintained in 2026 was not checked, UNVERIFIED. Fallback if no manifest: call `SetProcessDpiAwarenessContext(DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2)` as the first statement in `main`/`init` before any window is created (works, but not the recommended route).
- INFERENCE: in a Per-Monitor v2 process, hook coordinates and `SendInput` absolute coordinates are physical pixels, so edge logic and wire coordinates should be physical-pixel based per monitor with the monitor's scale factor shipped in the layout handshake.
- Topology: `EnumDisplayMonitors(NULL, NULL, cb, 0)` enumerates all monitors with rectangles in virtual-screen coordinates (negative origins are normal); `GetSystemMetrics(SM_XVIRTUALSCREEN, SM_YVIRTUALSCREEN, SM_CXVIRTUALSCREEN, SM_CYVIRTUALSCREEN)` is the bounding box; the "pseudo-monitors" of mirroring drivers appear too, so `GetMonitorInfo` + `EnumDisplayDevices` filtering is needed. https://learn.microsoft.com/en-us/windows/win32/api/winuser/nf-winuser-enumdisplaymonitors . Deskflow only reads the `SM_*VIRTUALSCREEN` bounding box and reacts to `WM_DISPLAYCHANGE` (MSWindowsComputer.cpp:950, 1434-1442), which means its "edge" is the bounding box edge and it cannot distinguish interior holes. For EdgeSync, "real edges" = segments of each monitor rectangle that are not shared with another monitor (compute from the rectangle list, handle L-shaped layouts and gaps).
- Re-enumerate on `WM_DISPLAYCHANGE` and `WM_DPICHANGED`.

### macOS

- `CGGetActiveDisplayList` + `CGDisplayBounds(display)` ("screen size and screen origin of display in global coordinates"); `CGDisplayPixelsWide/High` are separate pixel counts (CGDirectDisplay.h lines 113-127), so bounds are in the point coordinate space and Retina scale = pixels / points (INFERENCE; not stated in the header). Reconfiguration callback: `CGDisplayRegisterReconfigurationCallback` (Deskflow OSXComputer.mm:139, display list code 1316-1349). CG global coordinates have origin at the main display's top-left with y down (UNVERIFIED in fetched pages, standard Quartz convention); AppKit `NSScreen` uses bottom-left, do not mix.
- Event deltas from taps and injected deltas are in points (INFERENCE).

### Linux X11

- XRandR (`randr` in xgb: `GetScreenResources`/`GetMonitors`, `SelectInput` for `ScreenChangeNotify`/`CrtcChangeNotify`) is the proper topology source; Deskflow uses `XRRSelectInput` for change events and Xinerama for geometry (XWindowsComputer.cpp:882, 925). XRandR 1.5 monitor objects give logical monitors; mirrored outputs need dedup (UNVERIFIED details).

### Linux Wayland

- No global coordinate query. The InputCapture portal's `GetZones` returns zones; barriers must lie on the outer boundary of the zone union, `ZonesChanged` fires on changes, and Deskflow notes "The portal reports per-output zones, while Deskflow models the whole computer as one computer" and maps the activated barrier back to a side (PortalInputCapture.cpp ~line 448). Barriers are 1-D lines, so each real edge segment becomes a barrier. Fractional scaling interaction UNVERIFIED.

---

## 4. Keyboard mapping across OSes

How the references transmit keys:

| Project | On the wire | Notes |
|---|---|---|
| Synergy 1.x / Barrier / Input Leap / Deskflow <= 1.7 protocol | `KeyID` (32-bit "key symbol", UTF-32 for printable plus 0xE000-0xEFFF for control keys, "most identifiers equal the corresponding X11 keysym minus 0x1000"), `KeyModifierMask`, `KeyButton` (16-bit raw physical scancode) | `DKDN/DKUP/DKRP` messages; release is matched by `KeyButton`, not `KeyID`, because "Dead keys can change the KeyID between press and release", layouts differ, and modifiers released first alter KeyID. https://raw.githubusercontent.com/deskflow/deskflow/master/src/lib/deskflow/KeyTypes.h , https://raw.githubusercontent.com/deskflow/deskflow/master/src/lib/deskflow/ProtocolTypes.h |
| Deskflow protocol 1.8 | same plus a language code string on `DKDN`/`DKRP` ("helps clients handle characters from a keyboard layout they do not have active"), only for peers that said 1.8 in hello | ProtocolTypes.h; compat note: 1.1-1.7 covers "Synergy 1.4 to 1.14.1, Barrier, Input Leap". |
| lan-mouse | Linux evdev key code (`Key { time, key: u32, state }`) plus a `Modifiers` event with depressed/latched/locked/group; button codes are evdev `BTN_*` (0x110...) | Windows<->Linux scan-code tables in `input-event/src/scancode.rs`; macOS uses the `keycode` crate table. https://raw.githubusercontent.com/feschber/lan-mouse/main/input-event/src/lib.rs , https://raw.githubusercontent.com/feschber/lan-mouse/main/input-event/src/scancode.rs |
| Mouse Without Borders | Windows virtual keys (Windows-only) | Not cross-OS. |

Deskflow's Windows key state does heavy lifting for AltGr (it models Ctrl+Alt as AltGr, `KeyModifierAltGr`), dead keys (probes with `ToUnicodeEx` and a space to leave dead-key state) and keysym<->VK tables (MSWindowsKeyState.cpp:195, 662-690, 1307-1321).

Trade-offs:

| Approach | Pros | Cons |
|---|---|---|
| Keysym / character (Synergy style) | Types the intended character even if layouts differ (US keyboard driving a German machine produces German-layout expectations only if mapped); receiver can synthesise via Unicode injection | Receiver needs a full keysym->(keycode, modifiers) solver, dead-key and AltGr handling per OS; shortcuts (Ctrl+C) ride on layout letters which breaks on non-Latin layouts; complex, source of most Synergy bugs |
| Physical scancode / HID usage (lan-mouse style) | OS-neutral identity (position on keyboard), trivial receiver (`KEYEVENTF_SCANCODE`, CGEvent keycode, XTest keycode, EI keycode), game-friendly | Receiver's layout decides the character: sender with Dvorak and receiver with QWERTY yields wrong text unless layouts are matched; need per-OS modifier remap (Win/Cmd/Ctrl) and media/IME keys fall through |
| Hybrid | Both; use scancode by default and character payload only when layouts differ | More protocol surface |

Cross-OS specifics to handle (all INFERENCE / design points unless cited):
- Physical modifier mapping: Windows `Ctrl` + `Win` + `Alt` vs macOS `Control` + `Command` + `Option`. A user-configurable per-peer remap, with the common default `Win <-> Cmd`, `Alt <-> Option`, and an option to map `Ctrl <-> Cmd` for shortcut parity, is what users of these tools expect (not verified from a source).
- AltGr: Windows reports Right Alt as Ctrl+Alt in some layouts (Deskflow code above); send the physical `AltRight` and let the receiver's layout interpret it; do not send synthetic Ctrl.
- Dead keys: receiver-side state; scancode mode preserves it naturally; character mode must inject the composed character as Unicode (`KEYEVENTF_UNICODE`, `CGEventKeyboardSetUnicodeString`, XTest keysym remap, EI/keysym portal call).
- Chromium's `dom_code_data.inc` is a maintained table that cross-references USB HID usage, evdev, XKB, Windows and macOS scan codes (header cites USB HID Usage Tables, `HIToolbox/Events.h`, `linux/input.h`, Microsoft scan-code spec) under a BSD-style licence: a legally clean source for EdgeSync's own table. https://raw.githubusercontent.com/chromium/chromium/main/ui/events/keycodes/dom/dom_code_data.inc

Recommendation (justified): put the physical key identity on the wire as the USB HID usage (or W3C UI Events `code`), plus an optional `char` (Unicode code point) and a `layout` hint in the same event. Default mode = physical; receiver injects scancodes. If the handshake shows differing active layouts (or the user selects "type what I see"), switch to character mode for printable keys only and inject Unicode while still sending physical modifiers/shortcuts as scancodes. Release always keyed by the physical id (Synergy's hard-won lesson). Why: the scancode path is stateless and symmetric across all four platforms (`KEYEVENTF_SCANCODE`, `CGEventCreateKeyboardEvent` keycodes, XTest keycodes, EIS keycodes), cheapest to test, and avoids copying Synergy's keysym solver; the character fallback covers the layout-mismatch case that is the main weakness of lan-mouse's design.

---

## 5. Clipboard

### Change notification

| OS | Mechanism | Evidence |
|---|---|---|
| Windows | `AddClipboardFormatListener(hwnd)` -> `WM_CLIPBOARDUPDATE`; older viewer chain (`SetClipboardViewer`/`WM_DRAWCLIPBOARD`) as fallback; `GetClipboardSequenceNumber` for dedup | Deskflow uses the listener with fallback to `SetClipboardViewer` (MSWindowsComputer.cpp:187, 1471); golang.design/x/clipboard v0.11.0 uses the listener, with polling only when no window can be created (service session 0) (clipboard_windows.go ~747-800). |
| macOS | No change notification; poll `NSPasteboard.changeCount` ("increments each time the pasteboard ownership changes") | https://developer.apple.com/tutorials/data/documentation/appkit/nspasteboard/changecount.json ; Deskflow polls with a 1.0 s timer (OSXComputer.mm `m_clipboardTimer`); golang.design watches by polling at 1 s on non-event platforms (README). |
| X11 | `XFixesSelectSelectionInput(window, selection, SetSelectionOwner|...)` on `CLIPBOARD` and `PRIMARY` -> `XFixesSelectionNotify`; without XFixes you must poll | https://www.x.org/releases/current/doc/fixesproto/fixesproto.txt section 6 ("Applications wishing to monitor the contents of current selections must poll ... XFIXES improves this by providing an event"). xgb ships an `xfixes` package. |
| Wayland (wlroots, COSMIC, some others) | `zwlr_data_control_manager_v1` (deprecated) or `ext_data_control_v1` (staging) | See table below. |
| Wayland via portal | Clipboard portal `SelectionOwnerChanged` / `SelectionTransfer`, only inside an active RemoteDesktop or InputCapture session | Clipboard portal doc above. Deskflow implements it (PortalClipboard.cpp). |

Wayland data-control support (wayland.app tables read 2026-10-29; the tables' symbols are not defined on the page, I read "version number" = supported, "x" = not supported):
- `ext-data-control-v1` (staging "testing phase"): COSMIC, Hyprland 0.52.1, Jay, KWin 6.7, Labwc, Mir, niri, phoc, river, Sway 1.11, Treeland supported; Mutter 51, Muffin, Wayfire 0.9.0, Weston, Cage, Gamescope listed unsupported. https://wayland.app/protocols/ext-data-control-v1
- `wlr-data-control-unstable-v1` page calls it "deprecated and not intended for production use" and recommends ext-data-control; supported by Sway, Hyprland, COSMIC, Labwc, Wayfire, niri, river, others; listed unsupported: Mutter 51, KWin 6.7, Mir, Weston. https://wayland.app/protocols/wlr-data-control-unstable-v1
- GNOME: conflicting evidence on GNOME 49+. golang.design/x/clipboard README claims GNOME 49+ offers data-control; wayland.app lists Mutter 51 as supporting neither protocol; a May 2026 third-party commit says mutter exposes neither; Mutter issue 524 (wlr-data-control request) shows no resolution in my results. Treat "GNOME has no background clipboard access without the portal" as the safe assumption. UNVERIFIED.

### Formats

| Content | Windows | macOS | X11 | Wayland |
|---|---|---|---|---|
| Text | `CF_UNICODETEXT` (UTF-16LE, NUL terminated); `CF_TEXT`/`CF_OEMTEXT` synthesised | `public.utf8-plain-text` / `NSPasteboardTypeString` | `UTF8_STRING`, `text/plain;charset=utf-8`, `STRING`, `TEXT` | `text/plain;charset=utf-8` |
| HTML | registered format "HTML Format" = ASCII header (`Version:1.0`, `StartHTML`, `EndHTML`, `StartFragment`, `EndFragment`, optional `StartSelection`/`EndSelection`, byte offsets) then UTF-8 body with `<!--StartFragment-->`/`<!--EndFragment-->` markers; Version became 1.0 as of Windows 10 20H2 | `public.html` | `text/html` | `text/html` |
| RTF | registered format "Rich Text Format" | `public.rtf` | `text/rtf` / `application/rtf` | `text/rtf` |
| Image | `CF_DIB`/`CF_DIBV5` (raw pixels), plus registered "PNG" used by Chromium, Firefox, Office | `public.png` (`NSPasteboardTypePNG`), `public.tiff` | `image/png` | `image/png` |
| Files | `CF_HDROP` (DROPFILES + double-NUL paths); virtual: `CFSTR_FILEDESCRIPTORW` + `CFSTR_FILECONTENTS` | `public.file-url` (one per item), legacy `NSFilenamesPboardType` | `text/uri-list`, `x-special/gnome-copied-files` (first line `copy` or `cut`, then `file://` URIs), `application/x-kde-cutselection` | `text/uri-list` etc.; sandboxed apps use `application/vnd.portal.filetransfer` |

The exact X11 and Wayland MIME/atom names in this table (`UTF8_STRING`, `text/plain;charset=utf-8`, `text/html`, `image/png`, and so on) are standard conventions that I did not individually verify against a spec (UNVERIFIED); the Windows and macOS entries are verified as cited.

Sources: CF_HTML spec https://learn.microsoft.com/en-us/windows/win32/dataxchg/html-clipboard-format ; shell formats https://learn.microsoft.com/en-us/windows/win32/shell/clipboard ; mapping table of MIME <-> registered-format aliases ("PNG", "Rich Text Format") and macOS UTIs read from golang.design/x/clipboard source (clipboard_windows.go `windowsNativeNames`, clipboard_darwin.go lines 141-146). GNOME/KDE file-clipboard conventions come from search results (commit logs, API docs, Q&A), not a formal spec, so exact current behaviour of Nautilus 4x/Dolphin is UNVERIFIED. Deskflow's clipboard abstraction supports only Text, HTML and Bitmap (BMP 24/32bpp) formats: no RTF, no files. https://raw.githubusercontent.com/deskflow/deskflow/master/src/lib/deskflow/IClipboard.h

### Delayed rendering / laziness

- Windows: `SetClipboardData(format, NULL)` then answer `WM_RENDERFORMAT` (do NOT `OpenClipboard` inside it), `WM_RENDERALLFORMATS` on owner destruction (open clipboard, render everything), `WM_DESTROYCLIPBOARD` on loss of ownership. Microsoft suggests direct placement for single small text (<= 4 KiB) and says the rendering message blocks the UI thread. https://learn.microsoft.com/en-us/windows/win32/dataxchg/clipboard-operations . The OLE route `OleSetClipboard(IDataObject*)` offers all formats lazily via `IDataObject::GetData` and requires `OleFlushClipboard` to persist after exit. https://learn.microsoft.com/en-us/windows/win32/api/ole2/nf-ole2-olesetclipboard
- macOS: `NSPasteboardItemDataProvider` ("provide the data for a particular UTI type" on request) set via `NSPasteboardItem.setDataProvider(_:forTypes:)`. https://developer.apple.com/tutorials/data/documentation/appkit/nspasteboarditemdataprovider.json . From Go this needs a purego `objc.RegisterClass` subclass; UNVERIFIED in practice.
- X11: inherently lazy: the owner serves `SelectionRequest` on demand and must use INCR for large data (Deskflow implements INCR, XWindowsClipboard.cpp:936-969). The owner process must stay alive (golang.design docs: "the data is gone once the program exits").
- Wayland: lazy via `wl_data_source.send` (data-control) or portal `SelectionTransfer` -> `SelectionWrite`.

### Go libraries (verified by reading source on 2026-10-29)

- `golang.design/x/clipboard` v0.11.0 (MIT, active, 864 stars, pushed 2026-09-26). Major rewrite since the old 0.7.x: pure Go on all desktops, `CGO_ENABLED=0` builds on Linux and macOS (macOS via `purego`, Linux via own X11 package `golang.design/x/x11 v0.2.0` and a native Wayland backend), Windows via `syscall`. API: `Init`, `Read/Write(ctx, Format, ...)`, `WriteAll` (multi-format in one copy), `Formats`, `Register(mime)` custom MIME formats, `FmtText` (UTF-8), `FmtImage` (PNG), `FmtFiles` with `ReadFiles/WriteFiles`, `Watch` (event-based on Windows and Wayland, 1 s polling elsewhere per README), `Sensitive` flag for password-manager data, `FromPrimary()` for PRIMARY on X11/Wayland. Limits that matter for EdgeSync: eager data only (no delayed-render or lazy provider API), CF_HTML intentionally NOT aliased (payload is a header-wrapped fragment, not text/html, so you must build/parse CF_HTML yourself via a custom format), no virtual files, custom formats are raw passthrough (per `windowsNativeNames` comment), `ErrUnsupported` for custom formats in CGO-disabled macOS? (`ErrUnsupported` docs mention "a custom format in a CGO-disabled build", platform unspecified, UNVERIFIED), and on GNOME without data-control it falls back to XWayland. https://github.com/golang-design/clipboard , source files read at https://raw.githubusercontent.com/golang-design/clipboard/main/
- `github.com/atotto/clipboard` (BSD-3, repo pushed 2026-10-02, latest tag v0.1.4): text only; not sufficient.
- For input-hook libraries (not clipboard): `github.com/robotn/gohook` (MIT, v0.50.0 2026-10-04, wraps libuiohook, needs cgo), `github.com/go-vgo/robotgo` (Apache-2.0, v1.3.7 2026-10-22, cgo), `golang.design/x/hotkey` (MIT, v0.6.4) for registered hotkeys only. None gives suppression plus relative deltas; do not build on them.
- Deskflow's own clipboard model (GPL, do not copy) for reference: sequence-numbered grab (`CCLP clipboardID seq`) and chunked `DCLP` data with mark 0/1/2/3 (single/first/middle/last). That chunking and sequencing idea (not code) is a good pattern for loop prevention and streaming. ProtocolTypes.h.
- macOS privacy: from macOS 15.4 `NSPasteboard.accessBehavior` and the `detect*` methods exist (docs show `introducedAt: 15.4`); programmatic reads can trigger a user-visible access alert that the user can configure per app in System Settings, and the detect APIs read metadata "without notifying the person using the app". https://developer.apple.com/tutorials/data/documentation/appkit/nspasteboard.json . Reporting says the alert appears only when access was not paste-related user action (MacRumors/Michael Tsai posts, announced May 2025 as a developer preview; whether enforcement is default-on in macOS 26 is UNVERIFIED). A background clipboard-sync app polling `changeCount` is fine (changeCount does not read contents) but reading contents on every change is the risk.

---

## 6. File copy-paste across machines and drag-and-drop

What references do:

- Deskflow (GPL-2.0): `DDRG` (drag info) and `DFTR` (file chunks: start/chunk/end marks) exist in protocol 1.5+, but ProtocolTypes.h marks `kMsgDFileTransfer` with "@deprecated File drag and drop is no longer implemented." The platform clipboard code in Deskflow handles text, HTML and bitmaps only. So Deskflow no longer provides file copy-paste or drag-and-drop. https://raw.githubusercontent.com/deskflow/deskflow/master/src/lib/deskflow/ProtocolTypes.h (lines ~1023-1090)
- Input Leap / Barrier (GPL-2.0): README says clipboard sharing is supported "(Not currently supported on Linux/Wayland)"; no file feature advertised. https://raw.githubusercontent.com/input-leap/input-leap/master/README.md
- Synergy (commercial, lineage of the above): third-party mirror text says drag-and-drop of files is Windows and macOS only (Pro), legacy docs: no folders, may fail with SSL, a support reply says Windows drops land on the desktop only. Official site not read; UNVERIFIED. https://forums.symless.com/topic/2477-drag-and-drop-files-win10-win10-not-working/ , https://www.filehorse.com/download-synergy
- ShareMouse (closed source): official doc page says you drag files or folders onto the target computer's display and a progress bar appears; listings say the clipboard sync covers text, bitmaps and files/folders and can be sent to one specific computer. Mechanism UNVERIFIED (closed). https://www.sharemouse.com/doc/drag-drop/
- Mouse Without Borders (PowerToys, MIT): verified from source. Clipboard files: only when the clipboard holds a file drop list; sends a single path <= 100 MB (`MAX_CLIPBOARD_FILE_SIZE_CAN_BE_SENT = 100 MB`), "File too big (greater than 100MB), please drag and drop the file instead!" otherwise; text-like clipboard data over 1 MB goes via a separate TCP stream. Receiver saves into `Desktop\MouseWithoutBorders\<name>` and then calls `SetFileDropList` so paste works in Explorer (staging-directory approach). Drag-and-drop is a documented hack: the source machine shows a transparent "Mouse Without Borders Helper" window under the cursor to receive `DragEnter` and learn the dragged file name, then broadcasts `ClipboardDragDrop`; the target shows a small drop-icon window and on mouse-up pulls the data. It works for a single file only. Header comment also records the UIPI problem: "Drop action cannot be done from a lower integrity app to a higher one. We have to run a helper process". https://raw.githubusercontent.com/microsoft/PowerToys/main/src/modules/MouseWithoutBorders/App/Core/Clipboard.cs , .../Core/DragDrop.cs
- Why Deskflow removed file transfer is not stated in the source (UNVERIFIED); treat it as a signal that OS-native drag-and-drop internals are costly to maintain.

Approach evaluation:

(a) Eager (or on-demand pull) transfer to a staging dir, then put local file paths on the target clipboard (`CF_HDROP`, `public.file-url`, `text/uri-list` + `x-special/gnome-copied-files`).
- Pros: works with every file manager on every OS, trivially portable, library support (`WriteFiles`), resumable/cancellable transfer, integrity checks, can pre-announce metadata so the target UI can show "N files, X MB incoming".
- Cons: data must arrive before paste can start (or paste blocks/fails); large selections waste bandwidth/disk if never pasted; staging cleanup and disk space; cut vs copy semantics (`CFSTR_PREFERREDDROPEFFECT`, `x-special/gnome-copied-files cut`, KDE cut marker) need handling; symlinks/permissions/ACL fidelity; Windows long paths.
- Mitigation: threshold (e.g. eager under N MB, "pull when the cursor enters the other machine or when the user pastes" otherwise); announce-only metadata first.

(b) Windows virtual files: place an `IDataObject` that exposes `CFSTR_FILEDESCRIPTORW` (an hGlobal `FILEGROUPDESCRIPTOR` + `FILEDESCRIPTOR` array carrying `cFileName`, `FD_FILESIZE`, times, `FD_PROGRESSUI`) and one `CFSTR_FILECONTENTS` per file with `lindex` = descriptor index and `TYMED_ISTREAM`, set via `OleSetClipboard` (formats are delayed-rendered), so Explorer pulls bytes straight from the network only on paste and shows its own progress UI. Microsoft doc: contents "normally represented as a stream object, which avoids having to place the contents of the file in memory". https://learn.microsoft.com/en-us/windows/win32/shell/clipboard , https://learn.microsoft.com/en-us/windows/win32/api/shlobj_core/ns-shlobj_core-filedescriptorw , https://learn.microsoft.com/en-us/windows/win32/api/ole2/nf-ole2-olesetclipboard
- Feasibility from Go: possible but no existing Go implementation found. `go-ole` (MIT, v1.3.0 2023-08-15, no cgo) is built for calling COM objects (IUnknown/IDispatch/VARIANT); whether it can host a Go-implemented COM server is not stated (UNVERIFIED). You would hand-build IDataObject and IStream vtables (about 9 + 14 methods) in Go with `syscall.NewCallback` (>= 1024 callbacks guaranteed, never freed: create one callback per method once and dispatch on a `this` handle table), keep OLE on a locked STA thread with `OleInitialize` and a message pump, handle `IDataObject::GetData` being called on the clipboard/drag thread while network IO happens (blocking the Explorer UI thread; use async prefetch + range reads), and implement `IStream::Read/Seek/Stat` over a remote file. Medium-high effort, high payoff, Windows-only. Pitfalls known from C#/C++ implementations: Explorer calls `GetData` for CFSTR_PERFORMEDDROPEFFECT afterwards (cleanup hook), may query descriptors repeatedly, and `FILEDESCRIPTOR` sizes must be set for progress.
- This is the best UX on Windows target; recommend as phase 2, not v1.

(c) macOS file promises: `NSFilePromiseProvider` "provides a promise for the pasteboard"; file type is a UTI; you implement an `NSFilePromiseProviderDelegate` that supplies the file name and "writes the promised file to the destination directory" when the promise is fulfilled; receiving side uses `NSFilePromiseReceiver`. Apple's text focuses on drag-and-drop ("When you're working with drag and drop, use promises"); use for general pasteboard (copy/paste in Finder) is UNVERIFIED; Finder paste does not necessarily honour file promises (UNVERIFIED). https://developer.apple.com/tutorials/data/documentation/appkit/nsfilepromiseprovider.json , https://developer.apple.com/tutorials/data/documentation/appkit/nsfilepromisereceiver.json . Implementation from Go needs `objc.RegisterClass` with protocol `NSFilePromiseProviderDelegate` (supported by purego objc package, UNVERIFIED in practice). Practical macOS answer: approach (a), then write `public.file-url` items (what golang.design does via `NSFilenamesPboardType`).

(d) Linux file managers: no lazy mechanism that Nautilus/Dolphin honour for remote data; convention is real local paths as `file://` URIs: GNOME `x-special/gnome-copied-files` (`copy`/`cut` + URIs), KDE/others `text/uri-list` (+ `application/x-kde-cutselection`). Therefore staging dir (approach a) is the Linux strategy. Optionally FUSE/GVfs-style lazy mount is out of scope. On Wayland the Clipboard portal carries these MIME types only while a RemoteDesktop/InputCapture session is active; sandboxed apps get files through `application/vnd.portal.filetransfer` and the Documents portal. https://flatpak.github.io/xdg-desktop-portal/docs/doc-org.freedesktop.portal.FileTransfer.html (the doc does not discuss cross-machine use).

Cross-screen drag-and-drop: very high complexity and platform-specific (MWB hack, Synergy per-OS native drag code, Deskflow dropped it). Recommend deferring until clipboard-file path is solid; if built, reuse the clipboard-file staging machinery: the cursor-crossing event while the left button is down starts a transfer, drop on the target creates a copy into the drop location via a real OS drag operation (needs per-OS code) and MWB's lessons (UIPI between helper and Explorer; single-file limits) apply.

Recommendation: ship (a) everywhere in v1 with metadata-first announce, per-file streaming, size caps and a cancel UI; add Windows virtual files (b) in phase 2; skip macOS promises unless Finder testing shows paste benefit; defer drag-and-drop.

---

## 7. Licensing notes

Do not copy code from GPL projects if EdgeSync is not GPL. Reading them to understand behaviour is fine; copying structures, tables or code is not. This is engineering guidance, not legal advice.

| Project | Licence | Verified how |
|---|---|---|
| Deskflow | GPL-2.0-only WITH OpenSSL exception (repo also carries LGPL-2.1 and MIT texts under `LICENSES/`, scope unverified) | File headers, `LICENSES/` tree, GitHub README |
| Synergy 1.x lineage | GPL-2.0 with OpenSSL exception (inferred from Deskflow headers "Synergy App Ltd" + same licence) | UNVERIFIED for current Symless repos; Synergy 3 is a commercial product, source licence UNVERIFIED |
| Barrier | GPL-2.0 (LICENSE has "GNU GENERAL PUBLIC LICENSE Version 2" + OpenSSL exception text) | raw LICENSE |
| Input Leap | GPL-2.0 + OpenSSL exception (same) | raw LICENSE |
| lan-mouse | GPL-3.0-or-later | `Cargo.toml` `license = "GPL-3.0-or-later"` |
| Mouse Without Borders / PowerToys | MIT | raw LICENSE ("The MIT License", Microsoft) |
| ShareMouse | proprietary, closed source | product listings (UNVERIFIED) |
| libei | MIT per distro listing (UNVERIFIED upstream), `reis` Rust port MIT | search result, GitHub README |
| xdg-desktop-portal | LGPL-2.1-or-later (SPDX header in NEWS.md) | NEWS.md |
| Chromium (dom_code_data.inc) | BSD-style | file header |
| purego | Apache-2.0 (some Go-runtime derived files BSD-3) | README |
| golang-design/clipboard | MIT | LICENSE |
| atotto/clipboard | BSD-3-Clause | GitHub API |
| jezek/xgb | BSD-style 3-clause | LICENSE text |
| godbus/dbus | BSD-style 2/3-clause | LICENSE text |
| go-ole | MIT | GitHub README |
| tailscale/win, gonutz/w32 | BSD-style | LICENSE text |
| go-win32api (zzl) | MPL-2.0 (file-level copyleft) | LICENSE text |
| moutend/go-hook | MIT | GitHub |
| robotgo | Apache-2.0 | GitHub API |
| gohook | MIT (wraps libuiohook, cgo) | GitHub API |
| linuxdeepin/go-x11-client | GPL-3.0 | GitHub API |
| go-winres | permissive (0BSD-style text) | LICENSE text |
| rsrc | MIT | LICENSE text |

Interop note: speaking Deskflow/Input Leap wire protocol means re-implementing a documented protocol; the headers describing it are GPL, so write from the wire behaviour, not from their source, and take legal advice before claiming compatibility. Deskflow states compatibility with Synergy 1 (>= v1.17.0.96) and with Input Leap/Barrier, and describes lan-mouse as wanting interop. https://github.com/deskflow/deskflow

---

## 8. Recommendations

1. Architecture seams: define platform interfaces `Capturer`, `Injector`, `Topology`, `ClipboardBackend`, `FileTransferBackend`, `SessionHelper`. Keep wire types physical-pixel, per-monitor aware, platform-agnostic.
2. Pure Go (`CGO_ENABLED=0`) is viable on Windows, macOS (purego, beta) and X11 (xgb + generated XI2) and Wayland (godbus + own EI codec). Accept cgo only as a contained fallback (macOS helper, libei) if a prototype fails; decide after spikes (see risks).
3. Windows: dedicated locked OS thread for LL hooks + message loop, trivial callbacks, watchdog that re-installs hooks; `WM_INPUT` for deltas (prototype); manifest-declared PerMonitorV2 via go-winres or rsrc; `SendInput` with `ABSOLUTE|VIRTUALDESK` and `KEYEVENTF_SCANCODE`; split into SYSTEM service + per-session helper with `TokenUIAccess` token (Deskflow pattern) and `SendSAS`; ACL-restricted, authenticated IPC from day one (CVE-2026-41477 lesson); follow the active input desktop (`OpenInputDesktop` + `SetThreadDesktop`).
4. macOS: active `CGEventTap` on a locked thread with CFRunLoop; start with `kCGSessionEventTap` (documented non-root) and keep `kCGHIDEventTap` as test-matrix variant; require Accessibility, prompt through `CGRequestPostEventAccess`/AX trust; re-enable on `kCGEventTapDisabledBy*`; `CGAssociateMouseAndMouseCursorPosition(false)` + hide cursor (accept private `SetsCursorInBackground` risk behind a feature flag); ship a signed, notarised `.app` with stable bundle id.
5. Linux: ship X11 first (grabs + XTest, xgb), then Wayland Tier 1 = GNOME >= 45 and KDE Plasma >= 6.1 via InputCapture + RemoteDesktop + Clipboard portals with a Go EI implementation; Tier 2 = wlroots (layer-shell capture + wlr virtual input) later; design for session recreation on barrier change (mutter bug) and persist restore tokens when the portal supports them.
6. Keyboard: physical key (USB HID usage / W3C code) on the wire plus optional Unicode char and layout hint; configurable modifier remap per peer pair (default Win<->Cmd, Alt<->Option); release by physical id; character mode via Unicode injection when layouts differ.
7. Clipboard: v1 on top of golang.design/x/clipboard v0.11.0 (text, PNG, files, custom MIME, Watch) behind our own interface, build CF_HTML/RTF handling ourselves; announce formats as metadata and pull lazily where the OS supports delayed rendering, eager with size cap elsewhere; sequence numbers/origin ids against echo loops; honour `Sensitive`; on macOS poll `changeCount` only and avoid content reads without need (15.4 pasteboard access alerts); on Wayland use data-control when present, otherwise the portal clipboard inside the input session.
8. Files: staging-directory model (approach a) on all OSes in v1 with metadata-first announce, streaming, cancel, size/disk limits, cut/copy semantics; Windows virtual-files `IDataObject` as phase 2 spike; defer macOS promises and cross-screen drag-and-drop.
9. Licence hygiene: clean-room from public docs and protocol behaviour; avoid linking `go-x11-client` (GPL-3.0); use Chromium `dom_code_data.inc` (BSD) as the key-table source; check MPL-2.0 impact before using `go-win32api`.
10. Spikes to run before committing (each 1-3 days): (i) Go LL hook under GC load with the watchdog, (ii) macOS purego CGEventTap + CGWarp + hide cursor on macOS 13, 15, 26 on Intel and Apple Silicon, (iii) xgb-only X11 capture with deltas and Xinerama/RandR edges, (iv) Go EI codec against GNOME 45+ and Plasma 6.1+, (v) Go IDataObject virtual file served to Explorer.

## 9. Open risks

- Go GC pauses or scheduler stalls could exceed `LowLevelHooksTimeout`, silently removing the Windows hooks (no notification API).
- Windows LL hook coordinates are clamped to the virtual screen, so relative-delta capture design (raw input vs recentre) is unproven (INFERENCE).
- Unelevated Windows injector silently fails against elevated windows (UIPI, no error code); requires service + uiAccess token path and code signing.
- Secure desktop handling (UAC, Ctrl+Alt+Del, lock screen) needs a SYSTEM service; Deskflow had a SYSTEM named-pipe privilege-escalation CVE (CVE-2026-41477) in exactly this component.
- `SoftwareSASGeneration` policy must be enabled for `SendSAS`; Group Policy can block it.
- macOS HID-level tap is documented root-only but works for Accessibility-trusted apps in Deskflow; behaviour on macOS 26 and 27 unverified.
- macOS cursor hiding from a background app relies on a private CGS property (`SetsCursorInBackground`); Apple could break it.
- macOS 27 reportedly needs event numbers for synthetic mouse button/drag events (Deskflow comment); injection behaviour changes each release.
- purego is beta and its Apple callback/struct handling for CoreGraphics (`CGPoint` by value) is unproven here.
- TCC (Accessibility/Input Monitoring) grants depend on code-signing identity; unsigned or re-signed builds lose permission.
- macOS 15.4+ pasteboard access alerts may annoy users of background clipboard sync; enforcement level in macOS 26 unverified.
- X11: `xgb` has no XInput2 or XKB, so precise deltas and layout-group-aware keymaps need generated or hand-written bindings.
- Wayland: only GNOME >= 45 and KDE >= 6.1 have the InputCapture portal; Sway/wlroots have no portal path (issue open since 2023); Hyprland status unclear.
- Wayland: no Go libei or portal binding exists; writing an EI codec is a real cost and the protocol is still evolving (libei 1.5.0 vs 1.4.x).
- mutter quirks: events stop after a session is disabled once and sessions die when EI devices change; sessions must be recreated on barrier updates.
- Portal v2 features (persistence, clipboard in InputCapture, xdg-desktop-portal >= 1.21.1) may not be implemented by installed GNOME/KDE backends yet; users will see consent dialogs every session.
- Wayland background clipboard access on GNOME is unresolved (data-control support conflicting); portal clipboard only works inside an active input session.
- Windows virtual-file clipboard in Go has no prior art found and needs hand-built COM vtables plus an STA message pump; callback budget (>= 1024) is finite and never freed.
- File staging semantics (cut vs copy, partial transfers, very large files, long paths, permissions) are easy to get wrong; Deskflow removed its file transfer feature entirely.
- Cross-screen drag-and-drop has no portable implementation; Mouse Without Borders needs helper windows and is limited to single files, Synergy reports Windows-only-desktop drops.
- Keyboard: layout mismatch between peers produces wrong characters in physical mode; Win/Cmd/Ctrl/Alt remapping defaults will not suit every user.
- Licence boundary with GPL references (Deskflow, Input Leap, Barrier, lan-mouse) and GPL-3.0 `go-x11-client`; wire-protocol compatibility claims need legal review.
- Several support claims rest on third-party summaries (Synergy 3 features, ShareMouse internals, Nautilus/Dolphin clipboard details) and are marked UNVERIFIED above.
