# SP3 Linux spikes - manual script

Item: P0-SP3-M01 (Wayland steps 1 to 15) and P0-SP3-M02 (X11 steps 16 to 23)   Machines: GNOME box
Estimated time: 240 minutes   Needs admin: yes (only the apt package installs in Preparation; no step changes a system setting)

Nothing in this script has been run by its author. The commands and flags were read from the spike sources and READMEs
under `spikes/linux/` (the filepaste files as merged on main with P0-SP3-006), and the non-Gio build commands were
dry-run on the Windows host. The Gio builds (`wloverlay -mode gio`, `giodroplinux`) need cgo and the Gio Linux packages
and were not compiled anywhere. The Xvfb two-monitor setup with `xrandr --setmonitor` is UNVERIFIED: its syntax was
written from the xrandr manual and never run. Package names and external tool syntax (xdotool, xrandr, Xephyr options,
dbus-run-session, dpkg-query) come from the spike READMEs and from `.github/workflows/spikes.yml`, not from a run on the
box. Treat every output as the first real evidence, not as a confirmed fact.

Conventions in this script:
- The GNOME box is the one that runs the spikes. Commands run in Bash (or the shell of the GNOME session) from the
  repository root unless a step says `cd spikes`. Every binary is `spikes/bin/<name>` and every run writes its log under
  `spikes/out/` (both gitignored).
- Build form: `go -C spikes build -o bin/<name> ./<pkg>`. The `-o` path is relative to `spikes/`. Writing
  `-o spikes/bin/...` with `-C spikes` writes to `spikes/spikes/bin/` and is wrong.
- Run the binaries from `spikes/` (`./bin/<name>`), because the default log and token paths are `out/...` relative to the
  current directory.
- The X11 spikes run on their own X servers (Xvfb `:99`, Xephyr `:1` and `:2`). `DISPLAY=:0` is the XWayland display of the
  GNOME session and is used only where a step says so.

Questions the GNOME-box run must settle (from the portal and Clipboard notes in `plan.md`, checked against xdg-desktop-portal
tag 1.22.1 source only; the box decides):
- InputCapture interface version on the box. Version 2 uses `CreateSession2` (no Request object) then `Start`, with
  `persist_mode` and `restore_token`. Version 1 uses the deprecated `CreateSession`.
- Whether an activation followed by a barrier change on the same session still delivers `Activated` and EI events, or the
  portal closes the session or ends the EI connection (the mutter quirk), and whether a re-created session with the saved
  restore token captures again.
- Whether the RemoteDesktop EI devices need the `ei_device.ready` handling (`EI device resumed` after `done`).
- Whether the Clipboard portal transfers work with the InputCapture session inactive (`-arm none`) and during an activation
  (`-arm right`). The Clipboard methods have no Request object and `RequestClipboard` must come before `Start`.
- Whether Nautilus accepts `x-special/gnome-copied-files` with the payload the tested wlclip build produces. Until
  P0-SP3-010 (`fix(spike): wlclip gnome-copied-files without trailing LF`) is merged, that payload ends with LF, which the
  filepaste README says Nautilus rejects. A refusal from such a build is the expected result of that build, not a result
  for the no-LF payload.
- Whether a drag held in Nautilus survives an InputCapture activation (the experiment of `spikes/linux/dnd/README.md`).
- Whether Mutter bridges an X11 CLIPBOARD owner (filepaste `-mode x11` on XWayland) and whether a Wayland application pastes
  it.
- Whether the Notification portal banner, the XWayland override-redirect window and the Gio window meet the D-18 timing on
  GNOME (placement, duration, focus, fullscreen, lock screen). The Notification portal is not an on-screen overlay with
  that timing by design (R-22); the run records what GNOME does.
- Whether Gio on Wayland or XWayland delivers any OS file drop, and whether `-x11-proxy` receives XDND drops.
- Whether Nautilus, Thunar and Dolphin wait, show progress, time out and after how long, and whether the pasted file is
  complete (`sha256sum` against the `staging finished` digest).

## Preparation
1. Record the box and the session. Run from any terminal:
   ```
   cat /etc/os-release
   uname -a
   echo "XDG_SESSION_TYPE=$XDG_SESSION_TYPE XDG_CURRENT_DESKTOP=$XDG_CURRENT_DESKTOP WAYLAND_DISPLAY=$WAYLAND_DISPLAY DISPLAY=$DISPLAY"
   gnome-shell --version
   dpkg-query -W -f='${Package} ${Version}\n' 'gnome-shell' 'mutter*' 'libmutter*' 'xdg-desktop-portal*' 'xdg-desktop-portal-gnome'
   ```
   On a distribution without dpkg, record the same packages with `rpm -q` or `pacman -Q`. Keep the output for the results.
   Record also `gcc --version | head -n 1`.
2. Check that the X display numbers used below are free. Only the sockets of running X servers appear:
   `ls /tmp/.X11-unix/`. Expected: at most `X0` (the GNOME XWayland display). If `X1`, `X2` or `X99` exists, stop that
   server first or pick another number and use it in every command of this script that names the display.
3. Install Go 1.27.3 from the go.dev tarball (amd64 box; for arm64 use `go1.27.3.linux-arm64.tar.gz`). The SHA-256 of the
   tarball is on https://go.dev/dl/ and must match the `sha256sum` output:
   ```
   curl -LO https://go.dev/dl/go1.27.3.linux-amd64.tar.gz
   sha256sum go1.27.3.linux-amd64.tar.gz
   sudo rm -rf /usr/local/go && sudo tar -C /usr/local -xzf go1.27.3.linux-amd64.tar.gz
   export PATH="/usr/local/go/bin:$PATH"
   go version
   ```
   `go version` must print `go version go1.27.3 linux/amd64`. The `export` applies to the shell you run the script in; add
   it to your shell profile if you want it in every terminal.
4. Install the packages (Debian and Ubuntu names, the same list as the spikes workflow in `.github/workflows/spikes.yml`).
   The Gio build dependencies list is the one of the "Install Gio build dependencies" step; the X tools are the ones of the
   "Install X11 test tools" step plus `xserver-xephyr` for the nested server of the DnD and filepaste X11 steps:
   ```
   sudo apt-get update
   sudo apt-get install -y gcc pkg-config libwayland-dev libx11-dev libx11-xcb-dev libxkbcommon-dev libxkbcommon-x11-dev libgles2-mesa-dev libegl1-mesa-dev libegl-dev libffi-dev libxcursor-dev libxfixes-dev libvulkan-dev
   sudo apt-get install -y xvfb xserver-xephyr x11-xserver-utils xdotool git
   ```
   Other distributions: install the equivalent packages. The Gio dependency list is also documented at
   https://gioui.org/doc/install/linux. Check that the file managers of the steps exist before the run:
   `command -v nautilus thunar dolphin dbus-run-session openbox`. Missing optional ones (Thunar, Dolphin, openbox) are
   recorded as not run; the steps name them as optional.
5. Get the repository at the commit under test on the box. The origin remote of the working copy is
   `git@github.com:kikz0rsk/edgesync.git`; check `git remote -v` before cloning:
   ```
   git clone git@github.com:kikz0rsk/edgesync.git edgesync
   cd edgesync
   git checkout main
   git rev-parse --short HEAD
   git log --oneline -3 -- spikes/linux/cmd/wlclip
   ```
   Keep the printed hash for the results. The `git log` of `wlclip` shows whether the P0-SP3-010 fix is in the checkout (its
   commit subject is `fix(spike): wlclip gnome-copied-files without trailing LF`). Record which one it is.
6. Host-side tests of the spike code (no display, no D-Bus; the integration tests are not run here):
   ```
   go -C spikes test -count=1 ./linux/...
   go -C spikes test -run '^$' -fuzz '^FuzzDecodeFrame$' -fuzztime 30s ./linux/ei
   ```
   Expected: `ok` for every package with tests (`linux/ei`, `linux/portal`, `linux/x11`, `linux/dnd`,
   `linux/cmd/filepaste`); the fuzz run ends without a crash. A FAIL line is recorded verbatim with the package name.
7. Build the cgo-free binaries (run from the repository root; `mkdir` creates the gitignored `spikes/bin/`):
   ```
   mkdir -p spikes/bin
   go -C spikes build -o bin/wlcapture ./linux/cmd/wlcapture
   go -C spikes build -o bin/wlinject ./linux/cmd/wlinject
   go -C spikes build -o bin/wlclip ./linux/cmd/wlclip
   go -C spikes build -o bin/x11demo ./linux/cmd/x11demo
   go -C spikes build -o bin/dndx11 ./linux/cmd/dndx11
   go -C spikes build -o bin/filepaste ./linux/cmd/filepaste
   ```
8. Build the two Gio binaries on the box only (they need cgo and the Gio packages of step 4; they cannot be built from
   Windows):
   ```
   CGO_ENABLED=1 go -C spikes build -o bin/wloverlay ./linux/cmd/wloverlay
   CGO_ENABLED=1 go -C spikes build -o bin/giodroplinux ./linux/cmd/giodroplinux
   ```
   If a build fails, paste the compiler output verbatim and stop the Gio steps (the Wayland steps 10 to 13 and the Gio steps
   14 and 15 record the failure; the other steps continue).
9. Check the binaries exist: `ls -l spikes/bin/`. Expected: `wlcapture`, `wlinject`, `wlclip`, `x11demo`, `dndx11`,
   `filepaste`, `wloverlay`, `giodroplinux`.

## Steps

Part A - Wayland (GNOME session, P0-SP3-M01). Run from `spikes/` unless a step says otherwise. Delete a token file only where
a step says so: it forces a fresh consent dialog.

### Step 1 - InputCapture, first run (`wlcapture`, consent dialog, all three phases)
- Do:
  1. `cd spikes`, then `rm -f out/ic-token`.
  2. `./bin/wlcapture`
  3. Accept the consent dialog when it appears at `Start`.
  4. Phase `right-edge`: after `armed` appears, the log prints `ACTION: push the pointer through the barrier`. Push the
     pointer through the right edge of the rightmost monitor. When `Activated` appears, for the next 5 s move the mouse,
     click, press keys and scroll the wheel. Then wait for `Release sent` and the phase `RESULT` line, and check that the
     pointer comes back on the compositor side.
  5. Phase `left-edge-same-session`: the next `ACTION` line appears. Push the pointer through the left edge of the leftmost
     monitor and repeat the input after `Activated`.
  6. Phase `left-edge-recreated-session`: the first session is closed and a new one is created with the restore token.
     Repeat the left-edge push and the input.
  7. Let the run end (it prints `SUMMARY`). Ctrl+C only if it hangs; a phase without an activation times out after 90 s
     and is recorded as `no activation before the timeout`.
- Expect: the first lines show `connected`, `InputCapture` with `version` and `supported_capabilities`, `RemoteDesktop` with
  `version`, `restore token` with `present=false`. `session started` shows `granted`, `clipboard_enabled` and
  `restore_token_returned`. `armed` shows the zone set, zones and barrier positions. `Activated` shows `activation_id` and
  `cursor`; the EI line `EI start_emulating` carries the `sequence`, which equals the activation id. EI lines for pointer
  motion (relative deltas), buttons, keys and scroll follow. `Release sent`, a `RESULT` line per phase, and a final
  `SUMMARY` with `right_edge_capture_works`, `barrier_change_same_session_works`, `barrier_change_session_closed_by_portal`,
  `barrier_change_ei_ended`, `recreated_session_works` and `restore_token_issued`. A `Session.Closed`, `Disabled`,
  `ZonesChanged` or `EI connection ended` line is an observation, not a crash.
- Record:
  - the whole log `spikes/out/wlcapture.log` for this run (or at least every `RESULT`, `SUMMARY`, `Activated`,
    `Deactivated`, `Session.Closed`, `EI connection ended`, `ERROR` line plus the first five EI motion lines);
  - the `InputCapture` and `RemoteDesktop` interface versions and the capabilities value;
  - the consent dialog: yes or no;
  - for each phase: whether `Activated` appeared, and whether the pointer came back after `Release`;
  - the `SUMMARY` line verbatim.

### Step 2 - InputCapture, second run (restore token)
- Do:
  1. In `spikes/`, run `./bin/wlcapture` again without deleting the token.
  2. Note whether the consent dialog appears at `Start`.
- Expect: `restore token` with `present=true`. The dialog being skipped is not written to the log: note it by hand.
  `restore token saved` appears only if the portal returned a new token; with no such line the log does not say why.
- Record: the log lines of step 1 that apply to this run, the `SUMMARY` line verbatim, the consent dialog yes or no, and
  whether `restore_token_returned` was true.

### Step 3 - Input injection, first run (`wlinject`, consent dialog)
- Do:
  1. Open GNOME Text Editor, create an empty document and click into it so it has keyboard focus. Keep it focused.
  2. In `spikes/`: `rm -f out/rd-token`, then `./bin/wlinject -delay 5s`.
  3. Accept the consent dialog at `Start`.
  4. During the 5 s pause keep Text Editor focused. The run then moves the pointer around a 200 px square in 5 px steps,
     clicks the left button and types `edgesync`.
- Expect: `RemoteDesktop` with `version` and `available_device_types`; `session started` with `devices`,
  `clipboard_enabled` and `restore_token_returned`; `ConnectToEIS returned a file descriptor`; `EI handshake done`; `EI seat`
  with its name; `EI seat capability` lines; `EI seat bound`; `EI device announced`, `EI device name`, `EI device interface`,
  `EI device done`, then `EI device resumed` (the README asks whether the `ready` handling was needed: the log shows the
  order); `EI devices ready` when a pointer and a keyboard device were resumed; `ACTION: focus a text editor now`;
  `injecting pointer square`; `injecting left click`; `injecting keys`; and
  `SUMMARY square_px=200 typed=edgesync session_closed_by_portal=false`.
- Record: the log `spikes/out/wlinject.log` of this run (or the lines above plus every `ERROR` line); the consent dialog
  yes or no; on screen: did the pointer draw the square, did the click land, did `edgesync` appear in Text Editor; the EI
  devices announced (name and interface of each).

### Step 4 - Input injection, second run (restore token)
- Do: in `spikes/`, run `./bin/wlinject -delay 5s` again (the token file is reused). Keep Text Editor focused.
- Expect: `restore token` with `present=true`; the rest as in step 3.
- Record: the log of this run (the same lines as step 3), the consent dialog yes or no, and what happened on screen.

### Step 5 - Clipboard portal on a RemoteDesktop session (`wlclip -mode rd`, delay 5 s)
- Do:
  1. Confirm the wlclip commit of Preparation step 5 and write it down.
  2. In `spikes/`: `rm -f out/clip-rd-token`, then `./bin/wlclip -mode rd -wait 180s -delay 5s`.
  3. Accept the consent dialog at `Start`.
  4. Within the 180 s, in Text Editor paste (Ctrl+V) and note the paste.
  5. In Nautilus paste (Ctrl+V) into a folder other than the one you copied from. Before that, copy one file in Nautilus
     (Ctrl+C) so that the sample file is the clipboard content. Nautilus pastes one target per paste.
  6. In Text Editor copy some text (Ctrl+C). This is a local copy that the probe sees as `SelectionOwnerChanged`.
  7. Let the run end.
- Expect: `Clipboard` with `version`; `RemoteDesktop` with `version`; `session started` with `clipboard_enabled=true`;
  `SetSelection` with the five MIME types (`text/plain;charset=utf-8`, `text/html`, `image/png`, `text/uri-list`,
  `x-special/gnome-copied-files`); one `SelectionTransfer` per paste with `SelectionTransfer answered` after 5 s; a
  `SelectionOwnerChanged` line for the local copy with `session_is_owner` false and `mime_types`; one `SelectionRead` line
  per offered type with its byte count.
- Record: the log `spikes/out/wlclip.log` of this run (every line, it contains only sizes and MIME types); which MIME types
  Text Editor and Nautilus each requested; the delay before `answered`; whether the file pasted into Nautilus is there and
  complete (`sha256sum out/wlclip-sample.txt` and `sha256sum` of the pasted copy must match); any `failed` line verbatim.
  For the Nautilus paste, record the commit of Preparation step 5 (whether P0-SP3-010 was in it).

### Step 6 - Clipboard portal, 30 s answer delay (`wlclip -mode rd -delay 30s`)
- Do: in `spikes/`, run `./bin/wlclip -mode rd -wait 180s -delay 30s` (the token is reused). Paste in Text Editor and in
  Nautilus as in step 5 (the copy of a file in Nautilus happens before the Nautilus paste).
- Expect: `answered` after 30 s for each transfer whose application waits. If an application gives up earlier, its paste
  shows no content and the probe still logs `answered` after the 30 s delay, when the requestor is already gone.
- Record: for Text Editor and Nautilus: waited yes or no, the wait cursor or progress you saw, after how long each one gave
  up (if it did), and whether the pasted file is complete. Paste the log of this run (every line).

### Step 7 - Clipboard portal with the InputCapture session inactive (`wlclip -mode ic -arm none`)
- Do:
  1. In `spikes/`: `rm -f out/clip-ic-token`, then `./bin/wlclip -mode ic -arm none -wait 120s`.
  2. Accept the consent dialog at `Start`.
  3. Copy text in Text Editor and paste it again in Text Editor during the run. Paste a copied file in Nautilus.
- Expect: `InputCapture` with `version`; the Clipboard portal layer refuses InputCapture version 1 with an error (the
  message names "InputCapture version 1"), so a version 2 line is expected;
  `session started` with `clipboard_enabled`; `session inactive: capture not enabled, clipboard served without activation`;
  `SelectionTransfer answered` lines when the pastes worked, or `SelectionTransfer failed` with the error text when the
  portal refused.
- Record: the log of this run; for each paste yes or no (did the content arrive) and any error text verbatim. Note the
  InputCapture version on the box.

### Step 8 - Clipboard portal during an activation (`wlclip -mode ic -arm right -hold 5s`)
- Do:
  1. In `spikes/`: `./bin/wlclip -mode ic -arm right -hold 5s -wait 180s`.
  2. Accept the consent dialog at `Start`. After `armed` appears, push the pointer through the right edge of the rightmost
     monitor.
  3. When `Activated` appears, paste in Text Editor (the session is active). Wait for `Release sent` (5 s after
     `Activated`) and check that the pointer comes back.
  4. After `Deactivated` (or `Disabled`), paste in Text Editor again.
- Expect: `armed` with `edge`, `zone_set`, `zone` and `barrier`; `Activated` with `activation_id` and `cursor_present`;
  `Release sent` with the activation id; `Deactivated`; the `SelectionTransfer` lines of the two pastes.
- Record: the log of this run; the result of the paste during the activation and after `Deactivated` (content yes or no);
  whether the pointer came back at `Release`.

### Step 9 - Drag held through the barrier (`wlcapture`, DnD experiment 1)
- Do:
  1. In `spikes/`: `./bin/wlcapture -wait 120s`. The token of step 1 is reused.
  2. In Nautilus (Wayland), press and hold the left button on a file. Drag it to the right edge of the rightmost monitor and
     keep the button held.
  3. Observe the drag icon and the file list while the phase waits for `Activated`. When `Activated` appears, keep holding
     for the 5 s hold, then let the pointer go back (after `Release sent`), and then release the button.
  4. Optional, only if installed: repeat from a link in Firefox, and from an XWayland application (any X11 program on
     `DISPLAY=:0`).
  5. Ctrl+C after the observations. The session closes and releases the pointer.
- Expect: the spike does not decide the result; the observations are the result. The question is whether `Activated`
  appears while the drag is in progress, what happens to the drag icon and the file list, what happens on the button
  release, and whether the pointer reappears with the drag still alive after `Release`.
- Record: the log of this run; for each question yes, no or the observed behaviour in your own words; the application
  names and versions used for the drag.

### Step 10 - Lazy file paste in the portal mode (`filepaste -mode portal`, 5 s, 30 s and 120 s)
- Do: run three separate runs, each with a different delay. For each run, copy one file in Nautilus (Ctrl+C) and then paste it
  (Ctrl+V) into another folder in Nautilus while the run is waiting. Watch the file manager for the wait cursor or spinner,
  a progress bar, an error dialog, the time until the file appears, and whether the pasted file is complete.
  1. `./bin/filepaste -mode portal -delay 5s -wait 180s`
  2. `./bin/filepaste -mode portal -delay 30s -wait 180s`
  3. `./bin/filepaste -mode portal -delay 120s -wait 240s`
  Accept the consent dialog at `Start` in the first run if it appears. After each run, compare the pasted copy with the
  digest: `sha256sum <pasted file>` against the `sha256` of the matching `staging finished` line.
- Expect: `start` with `mode=portal` and the targets; `session started` with `clipboard_enabled`; `SetSelection`; a
  `SelectionTransfer` per paste; `staging started`; `staging finished` with `sha256` and `elapsed`; one `exchange` line per
  request with `outcome`, `requested`, `answered`, `delay`, `bytes_at_answer` and `complete_at_answer`. The synthetic file
  is 64 MiB per request by default (`-size`).
- Record: the log `spikes/out/filepaste.log` of each run (every line); the Nautilus (Wayland, portal mode) row of the
  observation matrix in `spikes/linux/cmd/filepaste/README.md`: target requested at copy time or only at paste time, wait
  behaviour at 5 s, 30 s and 120 s, whether it gave up and after how long, and whether the pasted file is complete (the
  `sha256sum` output of each pasted file).

### Step 11 - Clipboard of an X11 owner under XWayland (`filepaste -mode x11`, DISPLAY of the GNOME session)
- Do:
  1. Check the display: `echo "$DISPLAY"` must print the XWayland display of the session (usually `:0`).
  2. `./bin/filepaste -mode x11 -delay 5s -wait 120s` (uses `DISPLAY` from the session).
  3. Copy a file in Nautilus (a Wayland application), then paste it into Text Editor and into another Nautilus folder while
     the run waits. Also paste into an X11 application if one is open on the same display.
- Expect: `CLIPBOARD claimed` with the targets; for each Wayland paste a `exchange` line or no request at all. Whether Mutter
  bridges the X11 owner to Wayland applications is the observation. If another client copies, `CLIPBOARD taken by another
  client, probe stops serving` appears.
- Record: the log of this run; whether a Wayland application requested any target and which one; whether the pasted file
  arrived in Nautilus (Wayland); the answer to the bridging question in your own words.

### Step 12 - Switch indicator candidates, Notification portal (`wloverlay -mode notify`)
- Do: with Text Editor focused and typing in it, run each command in `spikes/`, one after another (the duration is 5 s each,
  except where stated):
  1. `./bin/wloverlay -mode notify -text "EdgeSync: Laptop" -duration 5s`
  2. `./bin/wloverlay -mode notify -hint transient -priority high -duration 5s`
  3. `./bin/wloverlay -mode notify -count 5 -interval 700ms -same-id`
  4. `./bin/wloverlay -mode notify -count 3 -interval 500ms`
  5. Only if the desktop file exists (check with `ls /usr/share/applications/org.gnome.TextEditor.desktop`):
     `./bin/wloverlay -mode notify -app-id org.gnome.TextEditor -duration 5s`. Otherwise record that it was not run.
  6. Repeat command 1 while a fullscreen window is shown (for example Text Editor or a browser in fullscreen mode).
  7. Optional: repeat command 1 while the screen is locked, only if you can unlock the screen afterwards.
- Expect: `Notification portal` with `version`; `Notification portal SupportedOptions` (when the portal reports them);
  `AddNotification returned` with `call_took`; `RemoveNotification returned` for each id; `ActionInvoked` only if a button was
  pressed. The spike does not show the banner's placement or duration in its log: those are the observations.
- Record: for each command, where the banner appeared (monitor and position), how long it stayed (and whether GNOME removed it
  before or after `-duration`), whether it stole keyboard focus (you kept typing in Text Editor), whether it covered a
  fullscreen window, whether it showed on the lock screen, whether it left an entry in the notification list, and the
  `call_took` value. Record the `version` and `SupportedOptions` lines.

### Step 13 - XWayland override-redirect window (`wloverlay -mode x11`)
- Do:
  1. Check that `DISPLAY` names the XWayland display: `echo "$DISPLAY"`.
  2. `DISPLAY=:0 ./bin/wloverlay -mode x11 -duration 5s`. Keep Text Editor focused and typing; repeat once with a fullscreen
     window in front.
- Expect: `window mapped` with `requested_x`, `requested_y`, `width`, `height` and `took`; `Expose handled, text drawn`;
  `ConfigureNotify` with `override_redirect`; `window placement` with `geometry_x`, `geometry_y`, `root_x` and `root_y`;
  `panel destroyed` after the duration. A missing core font fails loudly; record the error text verbatim (the X11 misc
  fonts of the distribution provide the core font `fixed`).
- Record: the log lines above; the requested and actual placement; whether the window stacked above normal Wayland windows;
  whether it covered a fullscreen window; whether it took keyboard focus.

### Step 14 - Gio window, native Wayland and XWayland (`wloverlay -mode gio`, built in Preparation step 8)
- Do:
  1. In `spikes/`: `./bin/wloverlay -mode gio -duration 5s`.
  2. `WAYLAND_DISPLAY= DISPLAY=:0 ./bin/wloverlay -mode gio -duration 5s` (forces the X11 path under XWayland).
  3. Keep Text Editor focused and typing during each run.
- Expect: `ConfigEvent` lines with the decorated state and size (`config`, `at`); `ViewEvent` with `kind=wayland` and
  `display_set`, `surface_set`, `toplevel_set`, or `kind=x11` with the window; `first frame` with `after` and `size`;
  `window destroyed` at the end.
- Record: the log lines above; where the window appeared; whether it had decorations; whether it took focus; whether it
  stayed on top; the first-frame delay.

### Step 15 - Gio file drop from Nautilus (`giodroplinux`, built in Preparation step 8)
- Do:
  1. In `spikes/`: `./bin/giodroplinux -duration 2m`. Drag one or two files from Nautilus onto the window that appears.
  2. `WAYLAND_DISPLAY= DISPLAY=:0 ./bin/giodroplinux -duration 2m`. Drag files from Nautilus onto the window.
  3. `WAYLAND_DISPLAY= DISPLAY=:0 ./bin/giodroplinux -x11-proxy -duration 2m`. Drag files from Nautilus onto the window.
  The windows close after the duration. Use the close button or Ctrl+C to stop earlier.
- Expect: `environment` with `XDG_SESSION_TYPE`, `WAYLAND_DISPLAY`, `DISPLAY` and the Gio version; `ViewEvent` with
  `kind=wayland` (with `display_set`, `surface_set`, `toplevel_set`) or `kind=x11`; `Gio transfer event` lines with their
  type for in-window transfers. With `-x11-proxy`: `XdndProxy installed on the Gio window`, then `XDND` lines with `offered`,
  `paths` and `rejected`, or `XDND drop failed` with the error. The module source says Gio has no OS file drop, so a drop
  with no `XDND` line is an expected observation.
- Record: the log of each run; whether the drop reached the window (yes or no per run); the `XDND` or `Gio transfer event`
  lines, verbatim; any error verbatim.

Part B - X11 (Xvfb and Xephyr on the box, P0-SP3-M02). The X servers here are separate from the GNOME session. Commands
name their display with `-display` or `DISPLAY=`; never use `DISPLAY=:0` for the grab, the injection or any run that holds
input.

### Step 16 - Start Xvfb and define the two monitors (`xrandr --setmonitor`, UNVERIFIED)
- Do: in one terminal (it keeps the server running):
  ```
  Xvfb :99 -screen 0 3840x1080x24 &
  export DISPLAY=:99
  xrandr --setmonitor edgesync-left 1920x1080+0+0 none
  xrandr --setmonitor edgesync-right 1920x1080+1920+0 none
  xrandr --listmonitors
  ```
  The `--setmonitor` lines are written from the xrandr manual and were never run. The Xvfb screen size and monitor geometry
  are the ones of `spikes/linux/x11/README.md` and `.github/workflows/spikes.yml`.
- Expect: `xrandr --listmonitors` lists two monitors or more, including `edgesync-left` and `edgesync-right` with their
  geometry (the header line format of xrandr is not checked here). If xrandr rejects `--setmonitor` or Xvfb does not support it, the rejection text is the observation.
- Record: the full output of the three xrandr commands verbatim, and the `xvfb` package version from Preparation step 4
  (`dpkg-query -W xvfb`). Do not change the layout by hand: a failed setup is recorded, and step 17 then shows the expected
  `TestRandR_MonitorsListed` failure.

### Step 17 - X11 integration tests under Xvfb (`go test -tags integration`)
- Do: from the repository root, with Xvfb `:99` of step 16 running:
  ```
  DISPLAY=:99 go -C spikes test -tags integration -count=1 -v ./linux/x11/... ./linux/dnd/...
  ```
- Expect: `TestXTest_MotionObservedByOwnWindow`, `TestGrab_WarpToCentreDeltas` (exact deltas (40,0), (-15,22), (3,-9)),
  `TestXFixes_SelectionNotifyOnForeignOwner`, `TestSelection_INCRTransfer5MiB` (INCR used, byte-identical read) and
  `TestRandR_MonitorsListed` (at least two monitors; this one fails if step 16 did not define them) in `linux/x11`; and
  `TestXdnd_SourceDropsOnTargetOnTheSameServer`, `TestXdnd_ReleaseOutsideAnyTargetEndsNoTarget`,
  `TestXdnd_LeaveReachesTheTarget`, `TestXdnd_ProxyTargetAcceptsADropOnAForeignWindow` and
  `TestXdnd_StaleProxyPropertyIsIgnored` in `linux/dnd`. The run ends with `ok` per package, or `FAIL` with the failing test.
- Record: the whole `-v` output verbatim (it is the M02 evidence), the `ok` or `FAIL` line per package, and each failing test
  with its message.

### Step 18 - Monitors and RandR changes (`x11demo -mode monitors`)
- Do: `./bin/x11demo -display :99 -mode monitors -duration 20s`. During the 20 s, change a monitor on Xvfb with
  `xrandr --setmonitor` or `xrandr --delmonitor edgesync-right` (the change notify path) and watch the events.
  The `--delmonitor` form is UNVERIFIED like `--setmonitor` (step 16); record its output.
- Expect: one `monitor <name> primary=... automatic=... origin=x,y size=WxH mm=... outputs=...` line per monitor, then
  `event: ...` lines while it runs.
- Record: the monitor lines verbatim, the xrandr commands you ran during the run and the `event:` lines that followed.

### Step 19 - Pointer grab with warp-to-centre deltas (`x11demo -mode grab`)
- Do:
  1. Start the grab on Xvfb only: `./bin/x11demo -display :99 -mode grab -duration 15s`.
  2. In a second terminal, with the same display, move the Xvfb pointer relative to its centre:
     `DISPLAY=:99 xdotool mousemove_relative -- 40 0`, then `DISPLAY=:99 xdotool mousemove_relative -- -15 22`, then
     `DISPLAY=:99 xdotool mousemove_relative -- 3 -9`. The Xvfb has no physical pointer, so xdotool is the only source of
     motion. The `xdotool` syntax is from its manual and was not run here.
  3. The grab ends after 15 s or with Ctrl+C.
- Expect: `delta <dx>,<dy> total <x>,<y>` lines with the offsets from the centre; the pointer stays at the centre (the spike
  warps it back).
- Record: the `delta` lines verbatim; whether the pointer stayed at the centre; any X error text verbatim.

### Step 20 - CLIPBOARD with INCR, serving and reading (`x11demo -mode own` and `-mode read`)
- Do:
  1. Terminal 1: `./bin/x11demo -display :99 -mode own -size 5242880 -duration 60s`
  2. Terminal 2, within the 60 s: `./bin/x11demo -display :99 -mode read -duration 60s`
- Expect: terminal 1 prints `serving 5242880 bytes on CLIPBOARD as UTF8_STRING`. Terminal 2 prints
  `read 5242880 bytes incremental=true sha256=<hex>`. The serving side prints no digest, so the README statement that both
  sides must agree cannot be read from the output; record the digest of the read side and say so.
- Record: both output lines verbatim; `incremental=true` or false; any error text (returned by `Serve` or `ReadSelection`).

### Step 21 - XDND source and target inside Xephyr (`dndx11`, file manager in a nested server)
- Do:
  1. Terminal 1: `Xephyr :2 -screen 1280x800 -ac &`, then optionally `DISPLAY=:2 openbox &`, then
     `dbus-run-session -- env DISPLAY=:2 GDK_BACKEND=x11 nautilus --new-window "$HOME"`. The `dbus-run-session` gives the
     nested Nautilus its own session bus so it does not hand over to the running Wayland instance. If Nautilus does not start
     under bare Xephyr, use Thunar (`dbus-run-session -- env DISPLAY=:2 thunar`) and record which one.
  2. Target: terminal 2, in `spikes/`: `DISPLAY=:2 ./bin/dndx11 -mode target -duration 3m`. Drag a file from the file manager
     onto the window titled `edgesync dndx11 target`. Also drag a file whose name contains `:` or `*` (it must appear as an
     entry rejected line; create it inside Xephyr's file manager if the name is not allowed on your file system).
  3. Source: `DISPLAY=:2 ./bin/dndx11 -mode source -duration 3m` (a synthetic `out/dndx11-sample.txt` is written). Press button
     1 on the window titled `edgesync dndx11 source` and drag onto a folder of the nested file manager; release. Repeat with
     button 3 during the drag (it cancels), with a drop on an empty part of the desktop (no target), and with a quick release
     after entering a window (the source sends `XdndLeave` without waiting for a status).
- Expect: target: a `target` line with `event=entered` (or `unsupported`), `offered=[...]`, and after the drop `event=dropped`
  with `paths=[...]`; a `drop failed` error line if the conversion fails. Source: a `drag ended` line with `outcome` (one of
  `dropped`, `rejected`, `cancelled`, `no-target`, `timed-out`), `target`, `action` and `elapsed`. A copy appears in the
  folder for `dropped`.
- Record: the file manager name and version; every log line of both modes verbatim; whether the file arrived and is complete
  (`sha256sum`); whether the file manager showed a copy cursor while hovering; the `elapsed` values.

### Step 22 - Lazy file paste inside Xephyr (`filepaste -mode x11`, delays, refusal and cut)
- Do:
  1. Terminal 1: `Xephyr :1 -screen 1280x800 -br &`.
  2. Terminal 2, in `spikes/`: `DISPLAY=:1 ./bin/filepaste -mode x11 -delay 5s -wait 180s`.
  3. In a file manager inside Xephyr `:1` (start it with `DISPLAY=:1 dbus-run-session -- env GDK_BACKEND=x11 nautilus --new-window "$HOME"`
     for Nautilus, or `DISPLAY=:1 dbus-run-session -- thunar`, or `DISPLAY=:1 dbus-run-session -- dolphin` if installed):
     copy one file, then paste it into another folder while the run waits. Note whether the file manager asked for TARGETS at
     copy time and which target it asked for at paste time.
  4. Repeat step 2 with `-delay 30s -wait 180s` and `-delay 120s -wait 240s`.
  5. Refusal: `DISPLAY=:1 ./bin/filepaste -mode x11 -refuse -delay 5s -wait 60s`, paste during the run, and record what the
     file manager shows.
  6. Cut: `DISPLAY=:1 ./bin/filepaste -mode x11 -op cut -delay 5s -wait 120s`, paste during the run, and check whether the
     source file is removed after the paste.
  7. After each paste, compare `sha256sum <pasted file>` with the `staging finished` digest of the matching request.
- Expect: `CLIPBOARD claimed` with the targets; one `exchange` line per request with `outcome` (`answered`, `refused`,
  `not-offered`, `failed` or `cancelled`), `answered`, `delay`, `bytes_at_answer` and `complete_at_answer`. A `refused` outcome
  is expected for step 5, not a failure of the probe.
- Record: `spikes/out/filepaste.log` for every run (every line); the observation matrix rows for Nautilus, Thunar and Dolphin
  (X11) of the `filepaste` README (target at copy or at paste, wait behaviour at 5, 30 and 120 s, whether it gave up and
  after how long, completeness of the pasted file); the refusal and cut results in your own words.

### Step 23 - Lazy XI2 and XKB assessment reproduction (`x11gen`, no display needed)
- Do: from the repository root (needs network for the xcb-proto clone and the Go module cache):
  ```
  bash spikes/linux/x11gen/gen.sh strict
  echo "strict exit $?"
  bash spikes/linux/x11gen/gen.sh subset
  echo "subset exit $?"
  ```
  The work directory defaults to `${TMPDIR:-/tmp}/edgesync-x11gen`. Nothing is written into the repository.
- Expect: `strict` prints two `FAIL` lines (`xinput` with `Unrecognized tag 'case'`, `xkb` with the `panic: switch field's
  expression not a fieldref` message) and `strict exit 1`. `subset` prints two line counts and `subset generated and vetted
  in ...`, then `subset exit 0`. The README has the numbers this run is compared with.
- Record: the full output of both commands verbatim, both exit lines, and any difference from the expected output.

## Cleanup
1. Stop every running spike with Ctrl+C in its window: `wlcapture`, `wlinject`, `wlclip`, `filepaste`, `wloverlay`,
   `giodroplinux`, `x11demo`, `dndx11`. Close the Text Editor and file manager windows you opened for the test.
2. Stop the X servers and the optional window manager you started: `pkill -x openbox`, `pkill -x Xephyr`, `pkill -x Xvfb`
   (or Ctrl+C in the terminal that started Xephyr; `kill %1` for an Xvfb started in the same shell). Only stop the ones you
   started; the GNOME session is not one of them.
3. The token files (`out/ic-token`, `out/rd-token`, `out/clip-rd-token`, `out/clip-ic-token`) are single-use restore tokens.
   Keep them until the results are pasted; delete them to force a fresh consent dialog later.
4. Delete the 64 MiB staging files after their digests are recorded: `rm -rf spikes/out/filepaste-staging` and
   `rm -f spikes/out/dndx11-sample.txt spikes/out/wlclip-sample.txt` (the sample files are recreated by the next run).
5. Run `git status --short` from the repository root. It must print nothing: `spikes/bin/` and `spikes/out/` are gitignored.
   If it prints a file, paste the output and do not commit anything.
6. Paste the observations into `qa/manual/results/p0-sp3-linux.md` in the results format of `qa/manual/README.md` (date,
   tester, commit hash from Preparation step 5, machine description with the versions of Preparation step 1). Put the Wayland
   observations under a `Wayland (P0-SP3-M01)` heading and the X11 observations under an `X11 (P0-SP3-M02)` heading, one row
   per step, with the verdict. A step that did not run is recorded as not run, with the reason. Or paste them in chat for the
   parent to commit.

## Safety
- Pointer capture (`wlcapture`, `wlclip -mode ic -arm`): while a phase is active the portal hands the pointer to the spike.
  Ctrl+C in its terminal ends the run and closes the session, which releases the pointer. If the pointer does not come back,
  switch to a text console (Ctrl+Alt+F3), run `pkill -f bin/wlcapture` (or `pkill -f bin/wlclip`), and switch back to the
  desktop with the key combination that brings you back to the session you came from.
- Injection (`wlinject`): the run types `edgesync` and clicks the left button into the focused window. Keep an empty Text
  Editor document focused, or nothing you care about. Ctrl+C stops the run and closes the session.
- Keyboard and pointer grab (`x11demo -mode grab`): holds the pointer and the keyboard of its display. Run it only with
  `-display :99` (Xvfb) or `-display :2` or `:1` (Xephyr). Never with the XWayland display of the GNOME session (`:0`).
- Drag (`dndx11 -mode source`): press button 3 during the drag to cancel it.
- Gio windows (`wloverlay -mode gio`, `giodroplinux`): close the window or press Ctrl+C in the terminal.
- Notification banners and overlays do not take input; they are closed by their duration or by the compositor.
- Disk: `filepaste` writes one 64 MiB file per request into `spikes/out/filepaste-staging`. Keep more than 1 GiB free.
- Nothing in this script changes a system setting: no service, no registry, no firewall, no portal configuration, no PATH
  change beyond the shell of step 3 of Preparation (`export PATH=...`). Only `apt-get install` of the packages of Preparation
  step 4 needs administrator rights.
