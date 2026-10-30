# P0-SP4 macOS spikes (CI first, optional Mac run)

Item: P0-SP4-M01   Machines: CI (push + dispatch); optional Mac on macOS 13 or later
Estimated time: 60 minutes   Needs admin: no

This run is optional. The CI `macos` job (artifact `spikes-macos`) already builds the spikes, runs their integration tests, builds the bundle, launches it for 5 s and captures the screen. Do the Mac steps only if the CI artifacts do not answer the questions in `briefs/streams/p0-sp4.md`. Nothing in this script has ever been run by its author: the macOS code was only cross-compiled on a Windows host. Treat every output as the first real evidence, not as a confirmed fact.

## Preparation
1. Check the branch name and push it: `git branch --show-current`, then `git push -u origin <branch>` with the name it printed.
2. Dispatch the workflow (needs `gh auth status` to show a logged-in account): `gh workflow run spikes.yml --ref <branch>`. The `macos` job runs on macos-latest; its test and bundle steps are `continue-on-error`, so a failing spike does not stop the artifact upload. Read the job Summary for the outcome of each step.
3. Find the run and watch it: `gh run list --workflow spikes.yml --branch <branch> --limit 1`, then `gh run watch <run-id>`.
4. When the run ends, save the artifact and the log: `gh run download <run-id> --name spikes-macos --dir spikes-macos-ci` and `gh run view <run-id> --log > spikes-macos-ci/job-log.txt`. In `job-log.txt` keep the lines of the `macos` job.
5. On the Mac (only for the optional steps), check the system: `sw_vers` must report 13.0 or later (`LSMinimumSystemVersion` in the bundle is 13.0), and `uname -m` reports `arm64` or `x86_64`.
6. Check the tools: `xcode-select -p` must print a path (if it fails, run `xcode-select --install` and finish the dialog) and `go version` must print `go1.27.3`.
7. Get the same commit on the Mac: `git fetch origin` then `git checkout <branch>`, from the repository root.
8. Create the output folders from the repository root: `mkdir -p spikes/bin spikes/out`. Both are gitignored.
9. Choose the terminal app you run the spikes from (Terminal.app or iTerm2). macOS usually attributes Accessibility and Input Monitoring requests to the app that started the process. This is not verified for these spikes, so note which app you used.

## Steps
### Step 1 - Build the spike binaries and the bundle
- Do:
  ```
  cd spikes
  CGO_ENABLED=0 go build -o bin/displays ./macos/cmd/displays
  CGO_ENABLED=0 go build -o bin/pb ./macos/cmd/pb
  CGO_ENABLED=0 go build -o bin/tap ./macos/cmd/tap
  CGO_ENABLED=0 go build -o bin/inject ./macos/cmd/inject
  CGO_ENABLED=0 go build -o bin/trayoverlay ./macos/cmd/trayoverlay
  cd ..
  bash spikes/macos/bundle/make-bundle.sh
  ```
- Expect: no compiler error; `spikes/bin/` holds `displays`, `pb`, `tap`, `inject` and `trayoverlay`. The bundle script prints the `plutil` result, passes `codesign --verify`, prints one `lipo -archs` line that lists both `arm64` and `x86_64`, and ends with `built <path>/spikes/out/EdgeSyncSpike.app`.
- Record: the output of `sw_vers` and `uname -m`; the full output of `make-bundle.sh`; any compiler or script error verbatim.

### Step 2 - Displays (read-only)
- Do:
  ```
  spikes/bin/displays
  cd spikes && CGO_ENABLED=0 go test -tags integration -run 'TestDisplays_ListBoundsAndScale|TestNSString_RoundTrip' -v ./macos/fw/ ; cd ..
  ```
- Expect: one line per active display in the form `display=<id> main=<bool> origin=(x,y) size=WxH points pixels=WxH scale=<n>`. Exactly one line has `main=true`, and on a single-display Mac its origin is (0,0). Scale is 2 on Retina panels. The test passes. The test moves the cursor to the centre of the main display and does not move it back.
- Record: the `displays` output; the `go test -v` output; whether the cursor ended in the centre of the main display; the scale value and the Mac model.

### Step 3 - Permissions (Accessibility, Input Monitoring, Local Network)
- Do:
  1. Run `spikes/bin/tap -duration 3s`. Its first line (`before: AXIsProcessTrusted=... CGPreflightListenEventAccess=... CGPreflightPostEventAccess=...`) reads the state without prompting.
  2. Run `spikes/bin/tap -prompt -duration 15s`. This may show the Accessibility and Input Monitoring dialogs.
  3. In System Settings > Privacy & Security, open Accessibility and then Input Monitoring. Enable the terminal app from the preparation step (and `EdgeSyncSpike` if it is listed). The tap README says the Input Monitoring pane mapping is unverified. If a change does not take effect, quit the terminal app, reopen it and repeat from item 1.
  4. Run `spikes/bin/tap -duration 3s` again.
  5. Local Network: the spikes do not open network sockets, so no Local Network prompt is expected. If one appears during any run, note which binary was running.
- Expect: after the grants, the `before:` line shows `AXIsProcessTrusted=true` and `CGPreflightListenEventAccess=true`. `CGPreflightPostEventAccess` may stay `false` until a posting spike is granted; record it as it is.
- Record: the `before:` line of each run; the text of each dialog and the pane it pointed to; the System Settings entries you saw for the terminal app; yes or no for a Local Network prompt.

### Step 4 - Pasteboard (writes to your clipboard)
- Do: copy anything you want to keep before this step, because these commands replace the clipboard content. Then run:
  ```
  spikes/bin/pb -mode status
  spikes/bin/pb -mode provider
  spikes/bin/pb -mode promise
  spikes/bin/pb -mode watch -duration 30s
  cd spikes && CGO_ENABLED=0 go test -tags integration -v ./macos/pb/ ; cd ..
  ```
  During `watch`, copy a line of text in TextEdit. If you have a password manager, copy a password from it too.
- Expect: `status` prints `changeCount=<n> concealed=<bool>` and the type list. `provider` reports the provider call count: 0 right after the write, once per type after the first read. `promise` prints either `NSFilePromiseReceiver fulfilled the promise in-process` or `OBSERVED: in-process NSFilePromiseReceiver path did not fulfil the promise`. `watch` prints a line for each change count change; the TextEdit copy appears, and the password copy shows `concealed=true` if the manager uses the concealed marker.
- Record: the full output of each mode; the `pasteboardFinishedWithDataProvider:` count printed by `provider`; the `watch` lines for your two copies; the `go test -v` output.

### Step 5 - Event tap
- Do: run in this order. Move the mouse and type in another app during each run. Run the `-suppress` run last.
  ```
  spikes/bin/tap -duration 15s
  spikes/bin/tap -events -duration 20s
  spikes/bin/tap -listen-only -events -duration 15s
  spikes/bin/tap -location hid -events -duration 10s
  spikes/bin/tap -suppress -duration 5s
  echo $?
  ```
  Read the Safety section before the `-suppress` run. Run `echo $?` after each run to get its exit status.
- Expect: the first line is the `before:` line. Then `tap installed: location=<name> listenOnly=<bool> suppress=<bool>`, one `stats:` line per second, and `final stats:` at the end with exit status 0. With `-events`, one `event type=<n> ...` line per event. With `-listen-only`, `Suppressed` stays at 0. With `-suppress`, `Suppressed` grows and your mouse and keyboard input does not reach other apps for those 5 s.
- Record: the complete output of each run with its command line and exit status; whether `tap installed` appeared or the `CGEventTapCreate returned NULL` error did, with the `before:` line for that run; the final values of `Events`, `Suppressed`, `DisabledByTimeout`, `DisabledByUserInput`, `Reenabled` and `ReenableFailures`; whether `Events` grew while you moved the mouse.

### Step 6 - Inject (posts real events)
- Do: before this step, leave a harmless spot (an empty desktop or a window that ignores clicks) at the centre of the main display, and leave any text field out of that spot. The inject spike moves the cursor, clicks, presses Shift+A and scrolls at that spot. There is no README for `spikes/macos/inject`; the flags come from `spikes/macos/cmd/inject/main.go` (`-location` is `hid` by default, or `session`).
  ```
  spikes/bin/inject -location hid
  echo $?
  spikes/bin/inject -location session
  echo $?
  ```
- Expect: each run prints `TCC state: ...` and then `posted at <location>, cursor=(x,y) target=(x,y)`, with exit status 0. The `TCC state` line matches the grants from step 3.
- Record: both complete outputs with the exit statuses; what the click reached at the centre of the main display; whether an `A` appeared if a text field was there; whether anything scrolled.

### Step 7 - Tray, overlay and bundle
- Do:
  ```
  spikes/bin/trayoverlay
  spikes/bin/trayoverlay -selftest 10s
  spikes/bin/trayoverlay -overlay 3s
  spikes/out/EdgeSyncSpike.app/Contents/MacOS/trayoverlay -selftest 10s
  open spikes/out/EdgeSyncSpike.app
  ```
  For the first run and for the `open` run, end the program with the status item menu entry "Quit". The `open` run does not show its log lines in the terminal. Then make Safari (or another app) full-screen with its green button, make it the frontmost app, and run `spikes/bin/trayoverlay -selftest 10s` again.
- Expect: an "ES" item in the menu bar and no Dock icon. The overlay with the text "EdgeSync" appears centred for 800 ms by default and again every second with `-selftest`. "Show overlay" in the menu shows it again. The `-selftest 10s` runs end by themselves after about 10 s with exit status 0 and no `trayoverlay:` line. Clicks on the overlay reach the window below, and the frontmost app keeps keyboard focus.
- Record: the complete stdout and stderr of each run with its command line and exit status; whether the "ES" item appeared; whether the overlay was visible over the full-screen space (yes or no) and on which spaces; a screenshot of the full-screen case if you take one (name it `fullscreen-overlay.png`).

### Step 8 - Crash reports and what to share
- Do:
  ```
  ls ~/Library/Logs/DiagnosticReports | grep -E '^(tap|pb|inject|displays|trayoverlay|EdgeSync)'
  ```
  Then send the following to the parent (in chat, or attach the files to the results file):
  - the Mac outputs of steps 1 to 7 (copy them as recorded there);
  - from `spikes-macos-ci/`: `environment.txt`, `macos-tests.log`, `displays.txt`, `pb-status.txt`, `bundle-build.txt`, `trayoverlay.stdout.log`, `trayoverlay.stderr.log`, `still-running.txt`, `screen.png` and the `crash-reports/` folder;
  - from `spikes-macos-ci/job-log.txt`: the `macos` job lines, including the Summary outcomes.
- Expect: no matching lines if no spike crashed. A matching file name is a crash report of that spike.
- Record: the matching file names (or "none"); the list of files you are sending.

## Cleanup
1. Quit `trayoverlay` from its menu entry "Quit". If a run is still going in a terminal, press Ctrl+C in that terminal. To stop a process from another terminal, send SIGTERM with `kill <pid>`, where `<pid>` comes from `pgrep -x trayoverlay`.
2. Confirm nothing is left running: `pgrep -l -x tap`, `pgrep -l -x trayoverlay` and `pgrep -l -x inject` must print nothing.
3. Optional: in System Settings > Privacy & Security, remove the Accessibility and Input Monitoring entries that step 3 added, if you do not want them to stay.
4. Optional: `rm -rf spikes/bin spikes/out` from the repository root. Both folders are gitignored.

## Safety
- `tap -suppress` consumes all mouse and keyboard input for its whole run, and Ctrl+C may not reach the process while it runs. Keep its `-duration` at 5s, do not run it with other apps you need open, and if you need to stop it early, send SIGINT from another terminal with `kill -INT <pid>`, where `<pid>` comes from `pgrep -x tap`. If input stays blocked after the duration, the last resort is a forced power off (hold the power button); the tap ends with the process.
- `pb -mode provider`, `pb -mode promise` and the `pb` integration test replace the clipboard content and clear the clipboard at the end. Copy anything you need before step 4.
- `inject` posts real clicks, a Shift+A and a scroll at the centre of the main display. Keep that spot harmless (step 6).
- The displays integration test in step 2 moves the cursor to the centre of the main display and does not restore it.
- `trayoverlay` has no Dock icon and runs until "Quit", so if its menu item cannot be reached, stop it from the terminal that started it with Ctrl+C, or with `kill <pid>` as in Cleanup.
