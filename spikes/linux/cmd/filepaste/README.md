# filepaste - Linux lazy file-paste probe (P0-SP3-006)

Item P0-SP3-006 (FEAT-177, FEAT-110, FEAT-113). Decisions under test: D-10 (clipboard data moves lazily, when a local
application pastes), D-11 (Linux file paste is fetch-on-request; the mechanism is decided by this spike), D-13 (the spike
proves the mechanics before the P4 build), D-45, D-98 and D-99 (path rules, applied through `dnd.EncodeURIList`), D-48
(logging: the probe logs timings, outcomes, sizes and the digest of its own synthetic file, never clipboard contents of
other applications) and D-121 (nothing here filters by sensitivity).

Nothing in this item was run: agents build and test only the host-safe parts (D-74). The X11 and portal paths are
compiled and vetted, and the observations below come from the GNOME box run.

## Purpose

The probe owns the clipboard with file targets and answers each data request only after `-delay`. During the delay it
writes a synthetic file of `-size` bytes into the staging directory; the answer is the `file://` URI of that file. So the
run shows, for each file manager and each paste target:

- which targets are requested, and in which order (TARGETS, `text/uri-list`, `x-special/gnome-copied-files`,
  `application/x-kde-cutselection`);
- whether the request is made at copy time or only at paste time;
- how the application behaves while the answer is pending (wait, progress, timeout) and after how long it gives up;
- whether the file it pastes is complete (compare `sha256sum` of the pasted copy with the digest in the log).

Two modes:

- `-mode x11`: the probe connects to the X server named by `DISPLAY` and owns CLIPBOARD with its own window. Requests
  are answered with `xgb` directly (the `x11` spike package keeps its request handling private and answers without a
  delay, so this probe carries its own small owner). Delayed answers run in goroutines that share the connection.
- `-mode portal`: the probe opens a RemoteDesktop session with the clipboard attached (`portal.OpenWithClipboard`), sets
  the same targets through `org.freedesktop.portal.Clipboard.SetSelection` and answers each `SelectionTransfer` through
  `ServeTransfer` after `-delay`. The staging file is started before the delay, so the content function only reports.

## Targets and payloads

| Target | Payload | Convention |
|---|---|---|
| `text/uri-list` | `file:///abs/path` + CRLF | RFC 2483 list, the same encoder as the DnD spike (`dnd.EncodeURIList`) |
| `x-special/gnome-copied-files` | `copy` or `cut`, LF, `file:///abs/path` (no trailing LF) | GNOME convention, read from Nautilus and Thunar (sources below) |
| `application/x-kde-cutselection` | `0` (copy) or `1` (cut) | KDE convention, read from kio (sources below) |
| `TARGETS` | the atom list of the owner's targets (X11 mode only) | answered by the probe, not offered as data |

`-targets` selects the subset offered (default: all three). `-op copy|cut` sets the markers. `-refuse` answers every data
request with a refusal after `-delay` (the staging file is still written), which shows what each application does with a
refused request. Requests for a target that is not offered are refused at once and logged as `not-offered`.

The trailing LF is deliberately absent from `x-special/gnome-copied-files`: the Nautilus parser splits the payload on LF and
rejects an empty line, so a trailing LF makes Nautilus refuse the whole clipboard content.

## Sources (cited, read 2026-11-04)

| Source | Version read | Used for |
|---|---|---|
| Nautilus `src/nautilus-clipboard.c`, https://raw.githubusercontent.com/GNOME/nautilus/main/src/nautilus-clipboard.c | `main` branch on the GitHub mirror of GNOME/nautilus (no release tag read) | serialisation (`copy` or `cut`, then `\n`-joined URIs), parser (first line must be `cut` or `copy`, empty line rejected, each line checked with `g_uri_is_valid`), registration of `x-special/gnome-copied-files` |
| Nautilus `src/nautilus-files-view.c`, https://raw.githubusercontent.com/GNOME/nautilus/main/src/nautilus-files-view.c | `main` | paste order: `NautilusClipboard` first, then `GDK_TYPE_FILE_LIST` (`text/uri-list`), then `G_TYPE_FILE`; `gdk_clipboard_read_value_async` is called for the chosen type only, so one data request per paste |
| Thunar `thunar/thunar-clipboard-manager.c`, https://raw.githubusercontent.com/xfce-mirror/thunar/master/thunar/thunar-clipboard-manager.c | `master` on the xfce-mirror GitHub mirror (GTK 3 code) | `owner_changed` requests `TARGETS` at owner change; `can-paste` requires `x-special/gnome-copied-files` in TARGETS; the paste reads that target; the provider writes `copy\n` or `cut\n` plus `\n`-joined URIs without a trailing LF; `text/uri-list` and `UTF8_STRING` are also provided. Second-source for the mirror |
| KDE kio `src/widgets/paste.cpp`, https://raw.githubusercontent.com/KDE/kio/master/src/widgets/paste.cpp | `master` on the GitHub mirror of invent.kde.org (file located through the repository tree) | `KIO::setClipboardDataCut` writes `application/x-kde-cutselection` as `1` for cut and `0` for copy; `isClipboardDataCut` reads the first byte, `1` means cut. Second-source (mirror, master) |
| Qt xcb clipboard `src/plugins/platforms/xcb/qxcbclipboard.cpp` and `.h`, https://raw.githubusercontent.com/qt/qtbase/dev/src/plugins/platforms/xcb/qxcbclipboard.cpp | `dev` branch | `QXcbClipboard::clipboard_timeout = 5000` ms; `clipboardTimeout()` returns it; the read transaction arms an abort timer with it. The value a Qt application waits for a conversion before giving up (Dolphin and other Qt applications). Second-source (dev branch, not a release) |
| GDK X11 selection `gdk/x11/gdkselection-x11.c`, https://raw.githubusercontent.com/GNOME/gtk/main/gdk/x11/gdkselection-x11.c | GTK `main` | searched for any timeout constant: none in this file. Whether GTK gives up at all, and after how long, is left to the observation below. Second-source (one file read) |
| `spikes/linux/portal` (xdg-desktop-portal `org.freedesktop.portal.Clipboard`, tag 1.22.1, as documented in `../wlclip/README.md`) | tag 1.22.1 | method and signal names, the `ServeTransfer` order (`SelectionWrite`, write, close, `SelectionWriteDone`) |
| `spikes/linux/dnd/uri.go` | this repository | path rules (D-45, D-98, D-99) and the CRLF URI list encoder |
| `github.com/jezek/xgb` v1.3.1 and `github.com/godbus/dbus/v5` v5.2.4 | as pinned in `spikes/go.mod` | signatures read from the module source in the Go module cache: `xproto.SetSelectionOwnerChecked`, `ChangePropertyChecked`, `SendEventChecked`, `CreateWindowChecked`, `GetAtomName`, `GetSelectionOwner`; `Conn.WaitForEvent` returns `(nil, nil)` after `Close`; `NewRequest` goes through channels, so the delayed answers may share the connection |

No code was taken from GPL projects (Deskflow, lan-mouse, Input Leap, Barrier, go-x11-client). Source reading covered
behaviour only.

Not read in this run (marked UNVERIFIED where it matters): the Mutter (GNOME Wayland) side of `SelectionTransfer` timing,
the ICCCM text on property `None` refusals (the convention matches `spikes/linux/x11/selection.go`), and the Dolphin
(Qt) source beyond kio's cut marker.

## Predictions from the sources (to be confirmed on the GNOME box)

These follow from the sources above; the run decides them. Statements marked INFERENCE are reasoned from the sources,
not observed.

- Nautilus (GTK 4): requests the data only at paste time, and only one target: `x-special/gnome-copied-files` when it is
  offered, otherwise `text/uri-list` (verified in the source). The paste reads asynchronously
  (`gdk_clipboard_read_value_async`), and GTK's X11 selection source has no timeout constant (verified in the source).
  INFERENCE: the paste waits with a busy or wait cursor until the answer arrives, shows no progress until the copy starts,
  and copies the file after the answer. Whether it gives up at 30 s or 120 s is the open question.
- Thunar (GTK 3): requests `TARGETS` when the owner changes (at claim time), and the data only at paste time, for
  `x-special/gnome-copied-files`.
- Dolphin (Qt): reads `text/uri-list` and `application/x-kde-cutselection`. Qt's xcb transaction gives up after
  5000 ms (the `clipboard_timeout` above). Expected: a 5 s delay is at the edge; 30 s and 120 s time out on the Qt side
  and the paste yields nothing or an error, while the probe logs an answer after the delay and the requestor has gone.
- Text Editor and other text-only targets: no file target is requested; the probe logs `not-offered` for `UTF8_STRING`
  or `text/plain` requests that appear.

## Build

From `spikes/` (cross-compiles from Windows; on the GNOME box drop the two variables):

```
GOOS=linux CGO_ENABLED=0 go -C spikes build -o bin/filepaste ./linux/cmd/filepaste
GOOS=linux CGO_ENABLED=0 go -C spikes vet ./linux/...
```

Host checks (no X server, no D-Bus):

```
go -C spikes vet ./linux/cmd/filepaste
go -C spikes test ./linux/cmd/filepaste/...
```

The host tests cover the target list parser, the flag validation, the payload encoding of all three targets for copy and
cut (including the rejected trailing LF and the D-45 path rules), the staging name, the deterministic content (offset
consistency), the staging writer (digest, content, progress snapshots under `-race`, cancellation, existing file, zero
size) and the exchange delay.

## Run (GNOME box, user run only)

Run from `spikes/` so the output lands in `spikes/out/` (created by the program). Default staging directory:
`spikes/out/filepaste-staging`.

X11 targets: start a nested X server and a file manager inside it (user-run):

```
Xephyr :1 -screen 1280x800 -br &
DISPLAY=:1 ./bin/filepaste -mode x11 -delay 5s -wait 180s
DISPLAY=:1 GDK_BACKEND=x11 nautilus            # Nautilus on X11 (GTK 4 X11 backend)
DISPLAY=:1 thunar                              # Thunar (GTK 3)
DISPLAY=:1 dolphin                             # Dolphin (Qt), only if installed
```

On the GNOME Wayland session the default DISPLAY is XWayland (`DISPLAY=:0`). Running `./bin/filepaste -mode x11` there
checks whether mutter bridges an X11 CLIPBOARD owner to Wayland applications (record it).

Portal target (GNOME Wayland session, consent dialog expected at `Start`):

```
./bin/filepaste -mode portal -delay 5s  -wait 180s
./bin/filepaste -mode portal -delay 30s -wait 180s
./bin/filepaste -mode portal -delay 120s -wait 240s
```

Refusal and cut:

```
./bin/filepaste -mode x11 -refuse -delay 5s -wait 60s
./bin/filepaste -mode x11 -op cut -delay 5s -wait 120s
```

Flags: `-mode x11|portal`, `-op copy|cut`, `-targets` (comma list of the three targets), `-delay` (default 5s), `-wait`
(default 120s, Ctrl+C stops earlier), `-size` (default 64 MiB per staging file), `-staging`, `-log out/filepaste.log`,
`-refuse`. The probe exits 1 when a staging write or an answer failed; a refusal requested with `-refuse` is not a failure.

## Steps

1. Start the probe with the delay under test and the wait long enough for the paste (the log shows `CLIPBOARD claimed`
   or `session started` and `SetSelection`).
2. Copy one file in the file manager: the log shows the TARGETS request at claim time (Thunar, Nautilus) and any other
   request at once. Note which targets appear at copy time and which only at paste time.
3. Paste in the same file manager (Ctrl+V in another folder). For each request the log shows `requested`, the
   `staging started` line and `answered` after `-delay`. Note in the file manager: wait cursor or spinner, progress bar,
   error dialog, the time until the file appears, and whether the pasted file is complete.
4. Compare the pasted file with the digest: `sha256sum <pasted file>` against the `sha256` of the matching
   `staging finished` line. A mismatch or a shorter file means the paste ran before the staging write was finished
   (`complete_at_answer false` on that exchange).
5. Repeat step 3 with `-delay 30s` and `-delay 120s`. Record whether the application gives up, after how long, and what it
   shows.
6. Repeat with `-refuse`: record what each application shows for a refusal.
7. Repeat with `-op cut` and check whether the file manager removes the source after a paste (cut semantics).
8. Portal mode: the same steps in GNOME Wayland applications (Text Editor, Nautilus on Wayland). Record the consent dialog on
   the first run and whether `SelectionTransfer` arrives while the session is active.

## Observation matrix (fill in from the run)

| Application | Target requested (at copy / at paste) | Wait behaviour at 5 s | 30 s | 120 s | Pasted file complete (sha256 match) | Notes |
|---|---|---|---|---|---|---|
| Nautilus (X11, GDK_BACKEND=x11) | | | | | | |
| Nautilus (Wayland, portal mode) | | | | | | |
| Thunar (X11) | | | | | | |
| Dolphin (X11, Qt) | | | | | | |
| Text Editor (text only) | | | | | | |

## Log lines to read

- `start` (mode, op, targets, delay, wait, size, staging, refuse).
- `CLIPBOARD claimed` and `session started` (with `clipboard_enabled`).
- `SetSelection` (portal mode).
- `SelectionRequest` through the `exchange` line: `target`, `outcome` (`answered`, `refused`, `not-offered`, `failed`,
  `cancelled`), `requested`, `answered`, `delay`, `bytes_at_answer`, `complete_at_answer`.
- `staging started` and `staging finished` (`sha256`, `elapsed`).
- `SelectionTransfer` (portal mode) and `SelectionOwnerChanged` (portal mode).
- `CLIPBOARD taken by another client` (X11 mode: another application copied, the probe stops serving).
- Errors: `staging failed`, `answer failed`, `SelectionTransfer failed`.

## What to paste back

- `spikes/out/filepaste.log` from every run (every line).
- The completed observation matrix, and the answer to each question in step 5 and step 6 in your own words.
- `gnome-shell --version`, the mutter and xdg-desktop-portal package versions, the Nautilus, Thunar and Dolphin versions,
  Xephyr version and the distribution.
- The output of `sha256sum` for the pasted files of steps 4 and 7.

## Limits and notes

- Every data request writes its own staging file of `-size` bytes. Two requests for two targets in one paste write two
  files. The staging directory is not cleaned up by the probe.
- The answer may point to a file that is still being written. The log records `bytes_at_answer` and `complete_at_answer`,
  so an incomplete paste is attributable to the timing and not to the probe.
- The X11 owner answers only with payloads that fit one request. The payloads are a URI and a marker, far below the X
  maximum request size; no INCR is implemented in this probe.
- Clipboard contents of other applications are never logged; the probe logs its own synthetic file digest only.
- Nothing in this probe filters by sensitivity (D-121).
- Finding for the Clipboard portal probe (P0-SP3-003, `../wlclip`): its `x-special/gnome-copied-files` payload ends with
  an LF. The Nautilus parser (source above) rejects an empty line, so that payload would make Nautilus refuse the paste.
  This probe does not change `wlclip`; the finding is recorded here for the findings item P0-SP3-009.
