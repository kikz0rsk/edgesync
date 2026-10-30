# macOS spikes (purego, no cgo)

Spike code for the macOS mechanisms behind D-39 (purego, `CGO_ENABLED=0`), D-10/D-11 and D-18. Rules: see `spikes/README.md` (D-74). Nobody can run these by hand on the Windows host (R-07); evidence comes from cross-compilation and from the `macos` job of `.github/workflows/spikes.yml`.

## Packages

- `fw/`: framework loading (CoreFoundation, CoreGraphics, ApplicationServices, AppKit, Foundation) through `purego.Dlopen`, CoreGraphics bindings for display enumeration, cursor warp and position read-back, CFRelease, and Objective-C helpers (`Class`, `Sel`, `NSString`, `GoString`).
- `cmd/displays/`: prints each active display with its bounds in points, pixel size and scale (pixels / points).

## Build

Cross-compile check from the Windows host (no run):

    GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go -C spikes vet ./macos/...
    GOOS=darwin GOARCH=amd64 CGO_ENABLED=0 go -C spikes vet ./macos/...

On a Mac, from the repository root:

    cd spikes && CGO_ENABLED=0 go build -o bin/displays ./macos/cmd/displays

## Run

    spikes/bin/displays

## Tests

Integration tests (CI `macos` job, `-tags integration`):

    cd spikes && CGO_ENABLED=0 go test -tags integration -run 'TestDisplays_ListBoundsAndScale|TestNSString_RoundTrip' -v ./macos/fw/

`TestDisplays_ListBoundsAndScale` asserts at least one active display, scale >= 1 for every display, and that a `CGWarpMouseCursorPosition` to the centre of the main display reads back within 1 point through `CGEventCreate(NULL)` + `CGEventGetLocation`.

## What to observe

- `displays` prints one line per active display; exactly one has `main=true`, and its origin is (0,0) on a single-display Mac.
- Scale is 2 on Retina panels and 1 on non-Retina panels (INFERENCE: pixel count of the current mode divided by the point width; not stated by the header).
- Coordinates are global points with the main display's top-left at the origin and y growing downwards (standard Quartz convention; UNVERIFIED on a Mac).

## What to paste back

The `displays` output and the `go test -v` output of the integration run.

## Evidence level for P0-SP4-001

- Signatures: verified against the CoreGraphics SDK headers (CGDirectDisplay.h, CGRemoteOperation.h, CGEvent.h, CGGeometry.h, CGError.h) and purego v0.11.2 source.
- Host: compile-only (cross vet and build for darwin/arm64 and darwin/amd64).
- Runtime: unverified until the CI `macos` job runs the integration test.
