# macOS event injection spike (P0-SP4-003)

Purpose: prove, from Go without cgo, that synthetic mouse move, left click, shifted key press and pixel scroll can be posted with `CGEventPost` at the HID or the session event tap, that each posted event carries an EdgeSync user-data tag, and that the cursor position can be read back after the move.

Evidence level: compile-checked only. The author never ran this on a Mac, and no agent runs it. Nobody can run the binary by hand on the Windows host (R-07). The `macos` CI job runs `go test -tags integration ./macos/...`, which includes the inject integration tests, but it builds only the `displays` and `pb` binaries, not `inject`.

## Packages

- `inject/inject_darwin.go`: `New` checks the location, binds the CoreGraphics symbols through purego, and creates an event source with the combined session state. `Move`, `Button`, `Key` and `Scroll` build one event, tag it with the user data, post it with `CGEventPost` at the configured location and release it. Button events carry a click state and an event number from a per-injector atomic counter. `Close` releases the source and is idempotent.
- `inject/inject_integration_test.go`: integration tests (`//go:build integration && darwin`).
- `cmd/inject/main.go`: the command line driver (`//go:build darwin`).

## Build

Cross-compile check from the Windows host (no run):

    GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go -C spikes vet ./macos/...
    mkdir -p spikes/bin
    GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go -C spikes build -o bin/inject ./macos/cmd/inject
    GOOS=darwin GOARCH=amd64 CGO_ENABLED=0 go -C spikes build -o bin/inject-amd64 ./macos/cmd/inject

`go -C spikes` changes into `spikes/` before it reads `-o`, so the binaries land in `spikes/bin/` (gitignored).

On a Mac, from the repository root:

    cd spikes && CGO_ENABLED=0 go build -o bin/inject ./macos/cmd/inject

## Run

    spikes/bin/inject [-location hid|session]

Flags, read from `cmd/inject/main.go`:

| Flag | Default | Meaning |
| --- | --- | --- |
| `-location` | `hid` | event tap location to post at: `hid` or `session`; any other value exits with status 1 before the frameworks are loaded |

There are no other flags. An unknown flag is rejected by the standard `flag` package with its usage message and status 2.

Runs to try, in this order:

    spikes/bin/inject
    spikes/bin/inject -location session

Each run does the same sequence at the centre of the main display: a move with the delta (3, -2) set on the event, a left button down and up with click state 1, a Shift-flagged key down and up for keycode 0x00 (the `KeyA` constant), and a pixel scroll with the vertical delta -12 and the horizontal delta 0. The click and the key press reach whatever is under the cursor and the frontmost application, so focus a harmless window (for example an empty text editor) before running it.

## Permissions

The command requests nothing. It prints the permission state and posts regardless of it. The Accessibility grant must be given by hand, in System Settings, to the process that runs the binary (for example the terminal). The integration test gates posting on `AXIsProcessTrusted`. `CGPreflightPostEventAccess` is printed on the first line, but the source does not gate on it and does not request it, so whether posting also needs Post Event access is unverified.

## Integration tests

    cd spikes && CGO_ENABLED=0 go test -tags integration -v ./macos/inject/

Test names: `TestInject_MoveClickKeyScroll` (for `hid` and `session`, it checks the fields of the built events, the rejected button and scroll unit, and posts and observes only when `AXIsProcessTrusted` is true), `TestInject_ClosedInjectorRejectsPosting`, `TestInject_NewRejectsInvalidLocation`. With posting enabled, the observation uses a listen-only session tap and expects the tagged mouse moved (delta 3, -2), left mouse down and up, key down and up for keycode 0x00, and scroll wheel events. The CI `macos` job runs these with `continue-on-error`.

## What to observe

- The first line is `TCC state: AXIsProcessTrusted=<bool> CGPreflightListenEventAccess=<bool> CGPreflightPostEventAccess=<bool>`, printed before anything is posted. Record it for every run.
- On success, stdout ends with `posted at <hid|session>, cursor=(<x>,<y>) target=(<x>,<y>)`. The target is the centre of the main display's bounds (origin plus half of the size). The cursor is read back once, immediately after the last event, with no wait. The integration test polls for up to 2 s, so one mismatched read on its own is not conclusive.
- With `AXIsProcessTrusted=false`, the integration test logs that posted events are expected to be dropped. This command does not check the state, so it posts anyway and reports the single read-back.
- Events posted by the command carry the user data `0x45444753` (the bytes spell `EDGS` in ASCII). This command does not observe them. Only the integration test does, through the listen-only session tap.
- Any failure prints `inject: <error>` on stderr and exits with status 1. Errors come from `CGEventSourceCreate returned NULL`, `CGEvent creation returned NULL` (wrapped with the CoreGraphics function name), an unresolved `resolve <symbol>` lookup, or a bad `-location` value (`location "<value>": want hid or session`).
- Exit: status 0 after the `posted at` line. Record the exit status.

## What to paste back

The complete stdout and stderr of each run, with the command line and the exit status; the `TCC state` line; the `posted at` line; whether the cursor visibly moved to the centre of the main display, whether the click reached the window under it, and whether the shifted `A` and the scroll reached the focused application; the output of `sw_vers` and `uname -m`; the output of `go test -v`, if run.

## Second-source constants

No doc comment in `inject/` or `cmd/inject/` records where these values come from. The stream brief names servo core-graphics bindings and Chromium kVK keycodes as the expected sources, but no file in the code names either, so this README does not attribute the values. None of them has been checked against Apple's headers on a Mac.

- `Location`: `LocationHID` = 0, `LocationSession` = 1.
- `Button`: left 0, right 1, centre 2.
- `ScrollUnit`: pixel 0, line 1.
- `Flags`: shift 0x00020000, control 0x00040000, option 0x00080000, command 0x00100000.
- `KeyA` = 0x00.
- Event source state: combined session state = 0.
- Mouse event types: left down 1, left up 2, right down 3, right up 4, mouse moved 5, other down 25, other up 26.
- Event fields: mouse event number 0, mouse click state 1, mouse button number 3, mouse delta X 4, mouse delta Y 5, keyboard keycode 9, event source user data 42.
- Scroll wheel count = 2.
- User data tag = 0x45444753 (`cmd/inject/main.go` and the integration test).

## Open questions

- Whether `hid` and `session` differ in what receivers observe. The code keeps both, and the integration test observes only at `session`.
- Whether Accessibility alone is enough for posted events to take effect, or whether Post Event access is also needed.
- Whether the single immediate cursor read-back lands within 1 point of the target.
- Whether the move's delta fields change what the cursor shows, given that the absolute position is also set to the centre.
- Whether keycode 0x00 types `A` under the active keyboard layout. The source posts the raw keycode, so the character depends on the layout.
- Whether the left click, the key press and the scroll reach the focused application and the window under the centre.
