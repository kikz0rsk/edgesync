# x11 - X11 capture, injection, RandR and selections spike (P0-SP3-004)

Item P0-SP3-004 (FEAT-177, FEAT-063, FEAT-064, FEAT-065, FEAT-067). Decisions under test: D-39 (pure-Go X11 through
`github.com/jezek/xgb` v1.3.1, no cgo), D-61 and D-121 (CLIPBOARD only, PRIMARY is never synced), D-89 (X11 coordinates
are physical pixels from RandR), and the warp-to-centre delta source named in `briefs/research-platform-internals.md`
section 1.3. The RECORD observation of D-92 is not part of this item and is not exercised here.

## Purpose

Prove on a real X server that the xgb calls the X11 backend needs behave as expected:

- `StartCapture` and `Deltas`: fullscreen override-redirect InputOnly window, `GrabPointer` and `GrabKeyboard`, a
  warp to the screen centre after every motion, and the offset from the centre as the delta.
- `Injector`: `xtest.FakeInput` for relative and absolute motion, buttons and keys.
- `Monitors` and `WatchMonitorChanges`: RandR 1.5 `GetMonitors` with names and geometry, and `SelectInput` for
  screen and CRTC change notify.
- `Own`, `Serve`, `ReadSelection`: CLIPBOARD ownership, `SelectionRequest` answers for `TARGETS` and the data targets,
  INCR in both directions (ICCCM 2.7.2) once a payload is larger than the server's maximum request.
- `WatchSelection`: XFixes `SelectSelectionInput` owner change notifications for a foreign owner.

Not in scope and not implemented: XInput2 and XKB (P0-SP3-005 assesses them), XRecord, keymap translation, PRIMARY.

## Sources (cited)

- Dependency: `github.com/jezek/xgb` v1.3.1 (tag commit `a225c310b6db5bd387e0c7b7a555711767d5ffa6`, 2026-04-18,
  BSD-style licence). Method names, signatures and constants were read from the module source. The package
  `xtest`, `randr` and `xfixes` `Init` functions register the extension and its events, and their request functions
  panic when the extension was not initialised.
- XTest semantics: https://xorg.freedesktop.org/archive/current/doc/xextproto/xtest.html . The `detail` argument of
  `FakeInput` is True (1) for relative motion and False (0) for absolute motion. Valid types are KeyPress, KeyRelease,
  ButtonPress, ButtonRelease and MotionNotify.
- INCR: ICCCM section 2.7.2 (https://x.org/releases/current/doc/xorg-docs/icccm/icccm.html). Not fetched on this
  host; the sender and receiver follow the protocol as described there (chunk per property delete, zero-length chunk
  ends the transfer).
- No code was taken from GPL projects (go-x11-client, Deskflow, lan-mouse, Input Leap, Barrier).

## Code layout

- `conn.go`: `Open` (xtest, randr 1.5, xfixes), the atom and window helpers, `WaitEvent` (cancelling the context
  closes the connection).
- `grab.go`: `StartCapture`, `Deltas`, warp to centre, and the abort path that releases partial grabs.
- `inject.go`: `Injector` over `xtest.FakeInputChecked`.
- `randr.go`: `Monitors` and `WatchMonitorChanges`.
- `selection.go`: `Own` and `Owner.Serve` (sender side, `outgoing` INCR state machine), `ReadSelection` (receiver side,
  `incrReceiver`), `WatchSelection`.
- `x11_integration_test.go` (`//go:build integration && linux`): the five integration tests below.
- `../cmd/x11demo/main.go`: a manual driver for every mode.

## Build

From `spikes/` (cross-compiles from Windows; on a Linux box drop the two variables):

```
GOOS=linux CGO_ENABLED=0 go -C spikes build -o bin/x11demo ./linux/cmd/x11demo
GOOS=linux CGO_ENABLED=0 go -C spikes vet ./linux/...
GOOS=linux CGO_ENABLED=0 go -C spikes vet -tags integration ./linux/...
```

## Xvfb setup for the integration tests

Start a display with a 3840x1080 screen and define two RandR 1.5 monitors side by side. The exact `--setmonitor`
geometry syntax below was written from the xrandr manual and was NOT run on the authoring host; if xrandr rejects it
or Xvfb does not support `--setmonitor`, record the xrandr output and use an equivalent layout with `-screen` (the
test then fails on `TestRandR_MonitorsListed` until two monitors exist, which is the expected signal):

```
Xvfb :99 -screen 0 3840x1080x24 &
export DISPLAY=:99
xrandr --setmonitor edgesync-left 1920x1080+0+0 none
xrandr --setmonitor edgesync-right 1920x1080+1920+0 none
xrandr --listmonitors
```

## Run the integration tests (Linux with Xvfb `:99`, user run only)

```
DISPLAY=:99 go -C spikes test -tags integration -count=1 -v ./linux/x11/...
```

Tests and what each checks:

- `TestXTest_MotionObservedByOwnWindow`: absolute XTest motion produces MotionNotify on an own window at the expected
  coordinates.
- `TestGrab_WarpToCentreDeltas`: under an active pointer grab, three relative XTest motions arrive as the exact deltas
  (40,0), (-15,22), (3,-9).
- `TestXFixes_SelectionNotifyOnForeignOwner`: a second connection claiming CLIPBOARD produces an XFixes owner change
  naming the claimed window.
- `TestSelection_INCRTransfer5MiB`: a 5 MiB UTF8_STRING payload is served by a second connection and read back
  byte-identical, and the transfer must have used INCR.
- `TestRandR_MonitorsListed`: at least two named monitors with geometry, and the change-notify selection succeeds.

## Run the manual driver (user run only, `DISPLAY` set)

Run from `spikes/`. Each mode stops after `-duration` or Ctrl+C.

```
./bin/x11demo -mode monitors -duration 20s
./bin/x11demo -mode grab -duration 15s
./bin/x11demo -mode own -size 5242880 -duration 60s
./bin/x11demo -mode read -duration 60s
```

- `monitors` prints each monitor and then every RandR change event while it runs (plug or move a monitor to observe).
- `grab` holds the pointer and keyboard. Move the mouse and watch the `delta` lines; the pointer stays at the centre
  and no key reaches other windows until the program exits.
- `own` claims CLIPBOARD with a deterministic payload of `-size` bytes. Run `read` in a second terminal to fetch it
  (`read` prints the size, whether INCR was used and the SHA-256); the two sides must agree.

## What to observe and paste back

- Output of `xrandr --listmonitors` and of `x11demo -mode monitors`: names, origins, sizes.
- Whether the integration test run passes; paste the `-v` output of `go test -tags integration`.
- For `grab`: whether the pointer is confined to the centre (deltas are reported and the pointer does not drift).
- For `own` and `read`: the `incremental=true` line and matching `sha256` values.
- Any X error text returned by `Deltas`, `Serve` or `ReadSelection` (they are returned, never swallowed).
