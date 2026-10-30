# wlclip - Clipboard portal spike (P0-SP3-003)

Item P0-SP3-003 (FEAT-177, FEAT-078). Decisions under test: D-10 (clipboard data moves lazily, when a local application
pastes), D-02 (Wayland on GNOME 45+ and KDE Plasma 6.1+ through portals), D-40 (in-house clipboard backends, no
clipboard library) and D-121 (concealed content is shared like any other: nothing here filters by sensitivity). Clipboard
contents are never logged (D-48): wlclip logs sizes only.

## Purpose

Attach the Clipboard portal (`org.freedesktop.portal.Clipboard`) to a session and measure what the compositor does with
it:

- `-mode rd`: RemoteDesktop session. `CreateSession`, `SelectDevices`, `RequestClipboard`, `Start`.
- `-mode ic`: InputCapture session, interface version 2. `CreateSession2`, `RequestClipboard`, `Start`. With `-arm right`
  (or `left`) it also runs `ConnectToEIS`, `Arm` (zones, barrier, `Enable`) and releases the capture after each
  `Activated` (`-hold`), so transfers can be compared while the session is inactive and while it is active.

Then it claims the clipboard with five MIME types (`SetSelection`), answers every `SelectionTransfer` after `-delay`
(`SelectionWrite`, write, close, `SelectionWriteDone`), and for every local copy (`SelectionOwnerChanged` with
`session_is_owner` false) reads each offered MIME type with `SelectionRead` and logs its size.

Offered payloads:

| MIME type | Payload |
|---|---|
| `text/plain;charset=utf-8` | one line with a UTC timestamp |
| `text/html` | `<meta charset="utf-8"><p><b>edgesync</b> wlclip</p>` |
| `image/png` | a 32x32 solid PNG (stdlib `image/png`) |
| `text/uri-list` | `file://` URI of the sample file, CRLF |
| `x-special/gnome-copied-files` | `copy` line, then the `file://` URI of the sample file |

The sample file is `out/wlclip-sample.txt` (absolute path resolved at start).

## Sources (cited)

Every method, signature, option key and signal below was checked against the portal interface definitions:

- Repository: https://github.com/flatpak/xdg-desktop-portal
- Version read: tag `1.22.1`. The annotated tag object is `fd69e91052c03fa05ec4fb971328ec9cbe2f4682`, which points to
  commit `1d20fadc304f6601452b5db65ed91197dba77041` (the commit recorded in `../../portal/README.md`; both checked on
  the GitHub API while implementing this item).
- Files: `data/org.freedesktop.portal.Clipboard.xml` (describes version 1 of the interface),
  `data/org.freedesktop.portal.RemoteDesktop.xml` (version 2), `data/org.freedesktop.portal.InputCapture.xml`
  (version 2).
- Licence: LGPL-2.1-or-later (SPDX header of the XML). Only interface facts were taken, no code.

Interface facts used by the code (`../../portal/clipboard.go`):

| Member | Definition at tag 1.22.1 |
|---|---|
| `RequestClipboard` | method, `(o session_handle, a{sv} options)`, no return |
| `SetSelection` | method, `(o session_handle, a{sv} options)`, no return; option `mime_types` (`as`) |
| `SelectionWrite` | method, `(o session_handle, u serial) -> h fd` |
| `SelectionWriteDone` | method, `(o session_handle, u serial, b success)`, no return |
| `SelectionRead` | method, `(o session_handle, s mime_type) -> h fd` |
| `SelectionOwnerChanged` | signal, `(o session_handle, a{sv} options)`; options `mime_types` (`as`), `session_is_owner` (`b`) |
| `SelectionTransfer` | signal, `(o session_handle, s mime_type, u serial)` |
| `version` | property, `u`, read |

## Deviations from the Request pattern and findings

1. No Request object. None of the Clipboard methods takes a `handle_token` or answers with a `Response` signal, so the
   subscribe-before-call rule of the Request pattern does not apply here. The clipboard signals are subscribed once per
   session (`Clipboard.Events`) before `SetSelection`. The Clipboard methods are called on
   `/org/freedesktop/portal/desktop`.
2. Ordering. RemoteDesktop: `RequestClipboard` must come before `Start` (the Clipboard documentation says "must be made
   before the session starts"; the RemoteDesktop `CreateSession` documentation points to it), so the order is
   `CreateSession`, `SelectDevices`, `RequestClipboard`, `Start`. InputCapture version 2: `CreateSession2`,
   `RequestClipboard`, `Start` (the `CreateSession2` documentation says the clipboard "may be accessed, but only when the
   session is active").
3. Version gate. InputCapture version 1 `CreateSession` is deprecated and "Sessions created by the deprecated
   CreateSession are immediately started", so no clipboard request can precede `Start`: wlclip refuses version 1
   (`ErrClipboardVersion`). RemoteDesktop documents `clipboard_enabled` in the Start response as "Since version 2", so
   wlclip also requires RemoteDesktop version 2. An absent or false `clipboard_enabled` is `ErrClipboardNotEnabled`
   and the session is closed; there is no fallback.
4. Activity gate. InputCapture: clipboard access "may be accessed, but only when the session is active". Whether a
   transfer fails while the session is inactive is what the `-arm none` and `-arm right` runs compare.
5. The portal creates no session for the Clipboard interface; the calls attach to the RemoteDesktop or InputCapture
   session handle.
6. `SelectionWrite` and `SelectionRead` return a descriptor that the portal creates. wlclip writes the payload into the
   descriptor from `SelectionWrite` and reads the descriptor from `SelectionRead`.
7. Not verified from a primary source: the exact `x-special/gnome-copied-files` format (`copy` line, then `file://`
   URIs). It is a community convention for Nautilus, not a specification. The run below is the test of it.

## Code layout

- `../../portal/clipboard.go`: `Clipboard` (`Version`, `Request`, `SetSelection`, `Write`, `WriteDone`, `Read`, `ReadAll`,
  `Events`, `ServeTransfer`), the event types `SelectionOwnerChangedEvent` and `SelectionTransferEvent`, and
  `RemoteDesktop.OpenWithClipboard` and `InputCapture.OpenWithClipboard`. `Open` in `remotedesktop.go` and
  `inputcapture.go` cannot take the clipboard request, so the two open paths are composed here from the same primitives.
- `main.go`: flags, the RemoteDesktop and InputCapture runs, `-arm` activation handling (`Activated` to `Release`),
  the transfer and read loop, and the payloads.

## Build

From the repository root (cross-compiles from Windows; on the GNOME box drop the two variables):

```
GOOS=linux CGO_ENABLED=0 go -C spikes build -o bin/wlclip ./linux/cmd/wlclip
GOOS=linux CGO_ENABLED=0 go -C spikes vet ./linux/...
```

## Host checks

```
go -C spikes vet ./...
go -C spikes test ./linux/portal/...
```

No host test is committed for the clipboard flows: the item names no test file. The request and response, event
decoding and transfer paths were verified against the fake bus (`request_test.go`) during implementation, including the
ordering `CreateSession`, `SelectDevices`, `RequestClipboard`, `Start` and `CreateSession2`, `RequestClipboard`, `Start`,
the version and `clipboard_enabled` rejections, a refused `SelectionWrite` and a failing payload both reported with
`SelectionWriteDone` success false, and the filtering of `SelectionTransfer` and `SelectionOwnerChanged` by session.

## Run (GNOME Wayland session, user run only)

Run from `spikes/` so the output lands in `spikes/out/` (created by the program):

```
./bin/wlclip -mode rd -wait 180s -delay 5s
./bin/wlclip -mode rd -wait 180s -delay 30s
./bin/wlclip -mode ic -arm none -wait 120s
./bin/wlclip -mode ic -arm right -hold 5s -wait 180s
```

Flags: `-mode rd|ic`, `-arm none|right|left` (ic only), `-delay` (pause before a transfer is answered, default 5s),
`-hold` (ic with `-arm`: between `Activated` and `Release`, default 5s), `-wait` (run length, default 120s, Ctrl+C
stops earlier), `-log out/wlclip.log`, `-token-file` (default `out/clip-rd-token` or `out/clip-ic-token`), `-sample`.
Delete the token file to force a fresh consent dialog. A run ends with a non-zero exit code when a transfer or a read
failed or the run itself failed.

Steps:

1. `-mode rd`: at `Start` a consent dialog may appear; accept it. The log must show `clipboard_enabled=true`, then
   `SetSelection` with the five MIME types.
2. Within the wait, in Text Editor paste (Ctrl+V): the log shows `SelectionTransfer` with the MIME type the application
   asked for and `answered` after the delay. Note how long the paste waited.
3. In Nautilus paste (Ctrl+V into a folder): note which MIME types Nautilus asked for, whether the sample file was
   copied, and whether the copy is complete.
4. Copy some text in Text Editor: the log shows `SelectionOwnerChanged` with `session_is_owner=false` and the MIME types
   of that copy, then one `SelectionRead` line per type with its size.
5. Run the same paste test with `-delay 30s` and note whether Text Editor waits or times out.
6. `-mode ic -arm none`: the session is started but capture is not enabled (inactive). Paste in Text Editor and record
   whether `SelectionTransfer` arrives.
7. `-mode ic -arm right -hold 5s`: move the pointer through the right edge of the rightmost monitor. `Activated` appears,
   the log records `Release sent` after 5 seconds and the pointer must return. Paste during the activation and after
   `Deactivated`, and record whether transfers fail while the session is inactive.

## What to observe

- The first lines: `Clipboard version`, `RemoteDesktop version` or `InputCapture version`, `session started` with
  `clipboard_enabled`, `restore token saved` or `no restore token returned`, and `SetSelection`.
- Per paste: the MIME type each application requests, the delay before `answered`, and whether the paste into Text
  Editor and into Nautilus succeeded.
- Per local copy: the `SelectionOwnerChanged` line (`mime_types`, `session_is_owner`) and the `SelectionRead` size of each
  type.
- `-arm` runs: `armed` (edge, zone set, zone, barrier), `Activated` (activation id, cursor), `Release sent`,
  `Deactivated`, `Disabled`, `ZonesChanged`.
- Failures: `SelectionTransfer failed`, `SelectionRead failed`, `Clipboard session closed by the portal`,
  `InputCapture session closed by the portal`, and any error line.

## What to paste back

- `out/wlclip.log` from every run (every line).
- `gnome-shell --version`, the xdg-desktop-portal package version, and the distribution.
- Whether the consent dialog appeared on the second run of each mode, and the observations of steps 2 to 7 in your
  own words (which application pasted which MIME type, delays, failures).

## Limits and notes

- Payloads are small (well below the 64 KiB pipe buffer), so a `Write` into the `SelectionWrite` descriptor does not
  block on the reader.
- `ReadAll` blocks until the writer closes its end of the `SelectionRead` descriptor. A writer that never closes stalls
  shutdown; kill the process if that happens and record it.
- `-arm` keeps the EI descriptor from `ConnectToEIS` open and does not read it. EI events queue on that socket during
  the run, which can change compositor behaviour; the log notes this when it matters.
- Clipboard contents are never logged; only MIME types, serials and sizes are.
- Nothing in this spike filters by sensitivity (D-121).
