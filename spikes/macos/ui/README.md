# macOS tray, overlay and bundle spike (P0-SP4-005)

Purpose: prove, from Go without cgo, that a Go program can own the AppKit run loop on the process main thread, show an `NSStatusItem` ("ES") whose menu calls back into Go, show a borderless click-through `NSPanel` overlay with a text label, return work to the main thread from other goroutines, exit cleanly, and be packaged as a minimal ad-hoc signed `.app` bundle (D-18, D-84, D-106, D-25).

Evidence level: compile-checked only. The author never ran this on a Mac, and no agent runs it. Nobody can run the binary by hand on the Windows host (R-07). The `macos` job of `.github/workflows/spikes.yml` runs `go -C spikes vet`, `build` and `test -tags integration` only; it does not launch `trayoverlay` or the bundle yet (P0-SP4-006 is open). `spikes/macos/ui` has no tests.

## Packages

- `ui/app_darwin.go`: `NewApp` (checks the main thread, reads `activationPolicy` and calls `setActivationPolicy:` for accessory only when it is not already accessory, as with `LSUIElement` in the bundle; a policy that is not accessory afterwards fails with the observed value; `Policy` reports initial, call result and final values, logged by `trayoverlay`); `Run` (`[NSApp run]` on the main thread until `Stop`, a failed posted function, or context cancellation); `Post` (queues a function on the main queue through `dispatch_async_f` on `_dispatch_main_q`, safe from any goroutine); `Stop` and `Fail`. Each posted function runs in its own autorelease pool.
- `ui/statusitem_darwin.go`: `NewStatusItem` (variable-length `NSStatusItem` with a title, an `NSMenu` with items and separators, and a target class registered with `objc.RegisterClass` under a process-unique name). Menu actions find their item by tag and call `OnAction`. `ButtonTitle`, `Visible` and `Close` are the read-back and teardown calls.
- `ui/overlay_darwin.go`: `NewOverlay` (borderless, non-activating, ignores mouse events, level `NSFloatingWindowLevel`, collection behaviour that joins all spaces and is full-screen auxiliary, 360 x 96 points, 28 pt white centred text on an 85% opaque near-black background). `Show(text, d)` replaces a visible overlay and hides it after `d`; a generation counter stops an older timer from hiding a newer overlay.
- `cmd/trayoverlay/main.go`: the program. `init` calls `runtime.LockOSThread()`. It loads the frameworks, creates the app, the overlay and the status item, reads back the button title and visibility, shows the overlay once, and runs the loop.
- `bundle/Info.plist`: the bundle property list.
- `bundle/make-bundle.sh`: builds the `.app`.

## Build

Cross-compile check from the Windows host (no run):

    GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go -C spikes vet ./macos/...
    mkdir -p spikes/bin
    GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go -C spikes build -o bin/trayoverlay ./macos/cmd/trayoverlay
    GOOS=darwin GOARCH=amd64 CGO_ENABLED=0 go -C spikes build -o bin/trayoverlay-amd64 ./macos/cmd/trayoverlay

`go -C spikes` changes into `spikes/` before it reads `-o`, so the binaries land in `spikes/bin/` (gitignored).

On a Mac, from the repository root:

    cd spikes && CGO_ENABLED=0 go build -o bin/trayoverlay ./macos/cmd/trayoverlay

Bundle (macOS only; needs `go`, `lipo`, `plutil` and `codesign`; run from the repository root):

    bash spikes/macos/bundle/make-bundle.sh [output-directory]

The script builds `trayoverlay` for arm64 and amd64 with `-trimpath`, joins them with `lipo`, copies `Info.plist`, runs `plutil -lint`, ad-hoc signs with `codesign -s - --force`, verifies the signature, prints `lipo -archs`, and prints `built <path>`. The default output directory is `spikes/out` (gitignored), which gives `spikes/out/EdgeSyncSpike.app`. An optional first argument replaces the directory. Any existing `EdgeSyncSpike.app` in that directory is removed first.

## Run

    spikes/bin/trayoverlay [-selftest 10s] [-overlay 800ms]

Flags, read from `cmd/trayoverlay/main.go`:

| Flag | Default | Meaning |
| --- | --- | --- |
| `-selftest` | `0` (run until Quit) | exit after this duration; while it is non-zero the overlay is re-shown every second (`pulseInterval`) |
| `-overlay` | `800ms` | how long each overlay stays visible |

No other flags. Runs to try, in this order:

    spikes/bin/trayoverlay
    spikes/bin/trayoverlay -selftest 10s
    spikes/bin/trayoverlay -overlay 3s
    spikes/out/EdgeSyncSpike.app/Contents/MacOS/trayoverlay -selftest 10s

The bundle executable takes the same flags. `open spikes/out/EdgeSyncSpike.app` starts it with the defaults (until Quit), but its log lines are not shown in the terminal.

Stop the program with the menu item "Quit", with Ctrl+C or SIGTERM in the terminal, or let `-selftest` expire.

## Second-source constants

Apple's pages name these AppKit and CoreGraphics constants without stating their values. The values below are copied from the doc comments in `spikes/macos/ui` and are not verified against Apple's headers on a Mac.

- `ui/app_darwin.go`:
  - `NSApplicationActivationPolicyAccessory` = 1 (servo core-foundation-rs `cocoa` `appkit.rs`, MIT/Apache-2.0).
  - `NSEventTypeApplicationDefined` = 15 (servo `appkit.rs`).
- `ui/statusitem_darwin.go`:
  - `NSVariableStatusItemLength` = -1.0 (servo `appkit.rs`).
- `ui/overlay_darwin.go` (SDK headers as transcribed by progrium/macdriver `enumtypes.gen.go`, MIT; the collection behaviour bits and the backing store also match servo `appkit.rs`):
  - `NSWindowStyleMaskBorderless` = 0.
  - `NSWindowStyleMaskNonactivatingPanel` = 128 (in the macdriver table only).
  - `NSBackingStoreBuffered` = 2.
  - `NSFloatingWindowLevel` = 3.
  - `NSWindowCollectionBehavior`: `CanJoinAllSpaces` = 1, `Stationary` = 16, `IgnoresCycle` = 64, `FullScreenAuxiliary` = 256.
  - `NSTextAlignmentCenter` = 2 on macOS (0 left, 1 right; unlike iOS).

## Open question

Is `NSFloatingWindowLevel` (3) enough for the overlay to show above full-screen spaces, given the `FullScreenAuxiliary` and `CanJoinAllSpaces` collection behaviour? Unverified. If it is not, the fallback is `NSStatusWindowLevel` = 25 (the value comes from the stream brief; it is not in the code and not verified). The change is the single constant `floatingWindowLevel` in `ui/overlay_darwin.go`.

## What to observe

- Menu bar: an "ES" status item appears. The menu has "Show overlay", a separator and "Quit". The program has no Dock icon (the accessory policy in code; `LSUIElement` in the bundle).
- Log lines on stderr from the default `slog` logger (timestamp, `INFO`, message, then `key=value` pairs): `status item created buttonTitle=ES visible=true`, `overlay shown visible=true for=800ms`, `run loop starting selftest=...`, and `run loop ended` on an orderly stop.
- Start: the overlay "EdgeSync" appears centred for `-overlay` and disappears.
- "Show overlay" in the menu shows the overlay again for `-overlay`.
- `-selftest 10s`: the overlay reappears every second and the process exits after about 10 seconds with status 0 and no `trayoverlay:` error line.
- Click-through: clicks on the overlay text reach the window underneath (the panel ignores mouse events).
- Focus: showing the overlay does not take keyboard focus from the frontmost application (non-activating panel).
- Full-screen: start a full-screen application in its own space, then run with `-selftest 10s`. Record whether the overlay is visible above that space, and on which spaces it is visible. This is the open question above.
- Bundle: `EdgeSyncSpike.app` has the "ES" item and the same behaviour as the raw binary. `plutil`, `codesign` and `lipo` report success.
- Exit: status 0 after Quit, after Ctrl+C or SIGTERM, and after `-selftest` expires.

## What to paste back

The complete stdout and stderr of each run with the command line and exit status; the output of `make-bundle.sh` (including the `lipo -archs` line); the output of `plutil -lint` and `codesign --verify --verbose=2` if run by hand; the macOS version (`sw_vers`), the Mac architecture (`uname -m`); a description (or screenshot) of the overlay over a full-screen space with the flags used; whether the menu bar item showed "ES".

## Bundle properties (`bundle/Info.plist`)

Bundle identifier `io.github.kikz0r.edgesync.spike`, name `EdgeSyncSpike`, version `0.0.1` (build `1`), executable `trayoverlay`, `LSMinimumSystemVersion` 13.0, `LSUIElement` true, `NSHighResolutionCapable` true, `NSPrincipalClass` `NSApplication`, `NSBonjourServices` containing `_edgesync._udp`. The Desktop, Documents, Downloads and local-network usage strings are declared to check whether macOS prompts are tied to the bundle identity. This spike does not touch those folders or the network, so no such prompt is expected. `NSAccessibilityUsageDescription` is deliberately not declared: Apple's documentation page for it could not be found when the file was written.

## Cannot be verified without a Mac

- Whether the overlay is visible above full-screen spaces (the open question).
- Whether the status item and the overlay appear at all under the accessory policy on the tested macOS version.
- Whether `dispatch_async_f` on `_dispatch_main_q` drains while `[NSApp run]` is active, and whether `stop:` plus the synthetic application-defined event ends the loop promptly.
- Whether the `ESSpikeTrayTarget` class registration and the menu target callback work on the main thread without warnings.
- Gatekeeper and quarantine behaviour for an ad-hoc signed bundle copied to another Mac.
- Whether the bundle with a `lipo`-joined binary runs on both arm64 and x86_64 Macs.
