# macOS event tap spike (P0-SP4-002)

Purpose: prove, from Go without cgo, that a `CGEventTap` can be installed on a dedicated CFRunLoop thread through purego, that its callback can be driven by Go, that a disabled tap is re-enabled (timeout and user-input disable), that events can be consumed (suppressed) by an active tap, and that the Accessibility and event-access permission state can be read and requested. The cursor association and hide calls (`Cursor`) are in the package and covered by an integration test, but `cmd/tap` does not call them.

Evidence level: compile-checked only. The author never ran this on a Mac, and no agent runs it. Nobody can run the binary by hand on the Windows host (R-07). Behaviour is unverified until someone runs it on macOS or the CI `macos` job runs the integration tests.

## Packages

- `tap/tap_darwin.go`: `CGEventTapCreate` with a purego callback that is created once per process; the event mask (mouse moved and dragged, left, right and other buttons, scroll wheel, key down and up, flags changed); `CFMachPortCreateRunLoopSource`; the callback re-enables the tap on `kCGEventTapDisabledByTimeout` and `kCGEventTapDisabledByUserInput`, counts the events, and returns NULL for every event while suppression is on.
- `tap/runloop_darwin.go`: a goroutine locked to its OS thread calls `CFRunLoopGetCurrent`, adds the source in `kCFRunLoopCommonModes`, and runs `CFRunLoopRun`. `Close` calls `CFRunLoopStop` and `CFRunLoopWakeUp` from another goroutine every 10 ms until the loop exits.
- `tap/permissions_darwin.go`: the permission calls listed under "TCC permission flow" below.
- `cmd/tap/main.go`: the command line driver (`//go:build darwin`).
- `tap/tap_integration_test.go`: integration tests (`//go:build integration && darwin`).

## Build

Cross-compile check from the Windows host (no run):

    GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go -C spikes vet ./macos/...
    mkdir -p spikes/bin
    GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go -C spikes build -o bin/tap ./macos/cmd/tap
    GOOS=darwin GOARCH=amd64 CGO_ENABLED=0 go -C spikes build -o bin/tap-amd64 ./macos/cmd/tap

`go -C spikes` changes into `spikes/` before it reads `-o`, so the binaries land in `spikes/bin/` (gitignored).

On a Mac, from the repository root:

    cd spikes && CGO_ENABLED=0 go build -o bin/tap ./macos/cmd/tap

## Run

    spikes/bin/tap [-location session|hid|annotated] [-listen-only] [-suppress] [-duration 10s] [-prompt] [-events]

Flags, read from `cmd/tap/main.go`:

| Flag | Default | Meaning |
| --- | --- | --- |
| `-location` | `session` | tap location: `session`, `hid` or `annotated` (any other value exits with an error) |
| `-listen-only` | `false` | create a listen-only tap instead of an active one |
| `-suppress` | `false` | consume every event (active tap only; with `-listen-only` the run exits with `a listen-only tap cannot suppress events`) |
| `-duration` | `10s` | how long to keep the tap installed |
| `-prompt` | `false` | request Accessibility, listen and post access, which may show system dialogs |
| `-events` | `false` | print every received event |

Runs to try, in this order:

    spikes/bin/tap -duration 15s
    spikes/bin/tap -prompt -duration 15s
    spikes/bin/tap -events -duration 20s
    spikes/bin/tap -listen-only -events -duration 15s
    spikes/bin/tap -location hid -events -duration 10s
    spikes/bin/tap -suppress -duration 5s

`-suppress` consumes mouse and keyboard events for the whole run, and keystrokes include Ctrl+C, so the keyboard may not stop the process. Keep its `-duration` short, or send SIGINT from another terminal (`kill -INT <pid>`). Suppression is switched on right after the tap is installed, so events that arrive in that gap are not consumed.

Integration tests (CI `macos` job uses `-tags integration`):

    cd spikes && CGO_ENABLED=0 go test -tags integration -v ./macos/tap/

Test names: `TestTap_CreateSessionTapReportsTrustState`, `TestTap_ListenOnlyRejectsSuppression`, `TestPermissions_PromptAndRequestReturnValues`, `TestCursor_AssociateAndHideReturnCodes`.

## TCC permission flow (as the code uses it)

- Accessibility: `AXIsProcessTrustedWithOptions` with no options is used by `Preflight` and never prompts. With `-prompt`, `AXIsProcessTrustedWithOptions` is called with `kAXTrustedCheckOptionPrompt` set to true. The legacy `AXIsProcessTrusted` is only logged by the integration test.
- Listen-event access (the Input Monitoring category in System Settings; the source does not name the pane, and this mapping is unverified on a Mac): `CGPreflightListenEventAccess` is read by `Preflight` and never prompts; `CGRequestListenEventAccess` is called only with `-prompt`.
- Post-event access: `CGPreflightPostEventAccess` is read by `Preflight`; `CGRequestPostEventAccess` is called only with `-prompt`. The tap itself does not post events.
- Order with `-prompt`: Accessibility prompt, then listen request, then post request, then a second `Preflight` line. The tap is created after all of this, and its creation does not check the permission state first. A NULL from `CGEventTapCreate` is reported as `CGEventTapCreate returned NULL`.

## What to observe

- The first line is `before: AXIsProcessTrusted=<bool> CGPreflightListenEventAccess=<bool> CGPreflightPostEventAccess=<bool>`. Record it for every run.
- With `-prompt`: `AXIsProcessTrustedWithOptions(prompt)=<bool> CGRequestListenEventAccess=<bool> CGRequestPostEventAccess=<bool>` and then an `after:` line. Note whether a system dialog appeared and which pane it pointed to.
- `tap installed: location=<name> listenOnly=<bool> suppress=<bool>` means the tap and its run loop source exist. If the tap cannot be created, the run exits with `tap: install run loop source: CGEventTapCreate returned NULL` on stderr and status 1. Record which case happened, with the `before:` line.
- Once per second: `stats: {Events:N Dropped:N Suppressed:N DisabledByTimeout:N DisabledByUserInput:N Reenabled:N ReenableFailures:N} enabled=<bool>`. `Events` grows while you move the mouse or type in other apps. `Dropped` grows only when the 1024-slot event channel is full because the main loop has not drained it.
- Re-enable path: `DisabledByTimeout` and `DisabledByUserInput` count the disable callbacks; `Reenabled` should match them and `ReenableFailures` should stay at 0; `enabled=true` should hold afterwards. These disable events are not printed by `-events`. They appear only in the counters.
- `-events`: one line per event, `event type=<n> location=(<x>,<y>) delta=(<dx>,<dy>) keycode=<n> userData=<hex>`. The type numbers are `CGEventType` values (for example 5 for mouse moved, 10 for key down, 11 for key up).
- `-listen-only`: `Suppressed` stays at 0 and events still arrive; `-suppress` with `-listen-only` fails with `a listen-only tap cannot suppress events`.
- `-suppress`: `Suppressed` grows with `Events`, and other apps stop receiving the same input.
- Exit: after `-duration` or SIGINT the run prints `final stats: ...` and exits with status 0. Record the exit status.

## What to paste back

The complete stdout and stderr of each run, with the command line and the exit status. For the `-prompt` run, also say whether a system dialog appeared and what it said. Add the output of `sw_vers`, the Mac architecture (`uname -m`), and the output of the integration test (`go test -v`), if run.

## Cannot be verified without a Mac

- Whether `CGEventTapCreate` returns NULL before the permissions are granted, and which permission it depends on.
- Whether a session tap receives events from other applications, and whether `hid` and `annotated` differ.
- Whether the timeout disable (`0xFFFFFFFE`) and user-input disable (`0xFFFFFFFF`) occur during a run, and whether `CGEventTapEnable` brings the tap back.
- Whether `CFRunLoopStop` from another goroutine stops the locked thread reliably in the retry loop, and whether a purego callback on that thread is safe.
- Whether suppression stops input for the frontmost application as well as for the system.
- The text and the pane of the TCC dialogs on the macOS version being tested.
