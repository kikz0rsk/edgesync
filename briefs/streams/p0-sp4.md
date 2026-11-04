# Stream brief - P0 SP4 macOS via purego (compile + CI only)

Phase 0 spike stream. Items: `briefs/plan-phase-0.md` section "SP4". Owner tag: `[owner:SP4]`.

## Scope
Prove the macOS mechanisms behind D-39 (purego, no cgo), D-10/D-11 (lazy pasteboard, file promises), D-18 (tray,
overlay) and D-25 (minimal .app bundle). Nobody can run anything on a Mac by hand (R-07): evidence comes from
cross-compilation on the Windows host and from the `macos` job of `.github/workflows/spikes.yml` (macos-latest),
which the user dispatches after pushing. Every finding must state its evidence level: CI-verified, compile-only,
or unverifiable.

## Files you may touch
Only your item's `Files:`, under `spikes/macos/`, plus `.github/workflows/spikes.yml` (macos job steps, P0-SP4-006
only) and the qa script/results. Never touch the main module, plan files, or other streams' spike directories.

## Conventions (binding)
- `github.com/ebitengine/purego` v0.11.2 and its `objc` package; `CGO_ENABLED=0`. Every file builds with `GOOS=darwin GOARCH=arm64` and `GOARCH=amd64` from Windows.
- Load frameworks with `purego.Dlopen("/System/Library/Frameworks/<F>.framework/<F>", purego.RTLD_NOW|purego.RTLD_GLOBAL)`; bind with `purego.RegisterLibFunc`. Global constants exported as data (e.g. `kCFRunLoopCommonModes`) are read through `purego.Dlsym` + pointer dereference.
- Verify every enum value and struct layout against Apple's headers/docs before use (CGEventTapLocation, CGEventField, CGEventType, CGPoint/CGRect as float64 pairs). Do not trust memory for constants.
- AppKit requires the main thread: a spike `main` package locks the main goroutine to the main OS thread in `init()` (`runtime.LockOSThread()`) and runs `NSApplication` there; event taps run on their own locked thread with their own CFRunLoop.
- `CGEventCreateScrollWheelEvent` is variadic: use `CGEventCreateScrollWheelEvent2` (fixed arguments).
- Integration tests: `//go:build integration && darwin`; they must log the TCC state (`AXIsProcessTrusted`, `CGPreflightListenEventAccess`, `CGPreflightPostEventAccess`) and report observed facts; they fail only on crashes, unexpected errors, or wrong results for operations that do not need TCC.
- No em dashes or en dashes. Commit `spike(macos): ... [FEAT-177]`, body `Item: <id>`, no attribution trailers.
- Research pointers: `briefs/research-platform-internals.md` sections 1.2, 2 (macOS), 3 (macOS), 5, 6 (c), 9; golang.design/x/clipboard v0.11.0 `clipboard_darwin.go` (MIT) is existence proof of purego + AppKit pasteboard use and may be read.

## Report format
```
FILES: <path> - <one clause>
GATES: <gate> pass|fail
ITEMS: <item id> <FEAT-NNN, ...>
TESTS: <what the test phase must cover, one line>
BLOCKED: <reason or NONE>
```

---

## Item P0-SP4-001
`spikes/macos/fw/`: framework loading (CoreFoundation, CoreGraphics, ApplicationServices, AppKit, Foundation); CoreGraphics bindings for `CGGetActiveDisplayList`, `CGDisplayBounds` (returns CGRect by value), `CGDisplayPixelsWide/High`, `CGMainDisplayID`, `CGWarpMouseCursorPosition` (CGPoint by value), `CGEventCreate(nil)` + `CGEventGetLocation` for position read-back, `CFRelease`. `objc_darwin.go`: helpers for `objc.GetClass`, `objc.RegisterName`, `ID.Send`, NSString conversion. `cmd/displays` prints displays with points, pixels and scale. The integration test asserts at least one display, scale >= 1, and that a warp to the centre of the main display reads back within 1 point.

## Item P0-SP4-002
Event tap (`spikes/macos/tap/`).
- `runloop_darwin.go`: a goroutine locked to its OS thread obtains `CFRunLoopGetCurrent()`, adds the tap's run loop source (`CFMachPortCreateRunLoopSource`) in `kCFRunLoopCommonModes`, runs `CFRunLoopRun`; stop via `CFRunLoopStop` from another goroutine.
- `tap_darwin.go`: `CGEventTapCreate(kCGSessionEventTap, kCGHeadInsertEventTap, options, mask, callback, refcon)` with a `purego.NewCallback` created once; options active (`kCGEventTapOptionDefault`) and listen-only; mask covers mouse moved/dragged, buttons, scroll, key down/up, flags changed; the callback handles `kCGEventTapDisabledByTimeout` and `kCGEventTapDisabledByUserInput` by calling `CGEventTapEnable(tap, true)` and counting; when suppression is on it returns nil for consumed events.
- `permissions_darwin.go`: `AXIsProcessTrustedWithOptions` (with and without the prompt option), `CGPreflightListenEventAccess`, `CGRequestListenEventAccess`, `CGPreflightPostEventAccess`, `CGRequestPostEventAccess`.
- Also test `CGAssociateMouseAndMouseCursorPosition(false/true)` and `CGDisplayHideCursor/ShowCursor` return codes.

## Item P0-SP4-003
Injection (`spikes/macos/inject/`): `CGEventSourceCreate(kCGEventSourceStateCombinedSessionState)`, `CGEventCreateMouseEvent` (move, left down/up with `kCGMouseEventClickState` and an incrementing `kCGMouseEventNumber`), `CGEventSetIntegerValueField` for `kCGMouseEventDeltaX/Y`, `CGEventCreateKeyboardEvent` with virtual keycodes and `CGEventSetFlags` for modifiers, `CGEventCreateScrollWheelEvent2` (pixel units), `CGEventPost(kCGHIDEventTap, e)` and `kCGSessionEventTap` variants; read back the cursor after moves.

## Item P0-SP4-004
Pasteboard (`spikes/macos/pb/`).
- `pasteboard_darwin.go`: `NSPasteboard generalPasteboard`, `changeCount`, `clearContents`, `types`, `dataForType:`, `stringForType:`; detect `org.nspasteboard.ConcealedType`.
- `provider_darwin.go`: register an Objective-C class (purego `objc.RegisterClass`) conforming to `NSPasteboardItemDataProvider` implementing `pasteboard:item:provideDataForType:` (calls `setData:forType:` on the item with data from Go) and `pasteboardFinishedWithDataProvider:`; create an `NSPasteboardItem`, `setDataProvider:forTypes:` for `public.utf8-plain-text`, `public.html`, `public.rtf`, `public.png`; `writeObjects:`; read back each type in-process and count provider invocations.
- `promise_darwin.go`: register a class conforming to `NSFilePromiseProviderDelegate` (`filePromiseProvider:fileNameForType:`, `filePromiseProvider:writePromiseToURL:completionHandler:`); create `NSFilePromiseProvider` with `fileType` `public.data`; write it to the pasteboard; in the test, fulfil it programmatically if an API allows (record if not) and verify the delegate writes a file to a temp dir.
- README lists what cannot be verified without a Mac: Finder paste honouring promises from the general pasteboard (RPI 6c: UNVERIFIED).

## Item P0-SP4-005
Tray, overlay, bundle (`spikes/macos/ui/`, `cmd/trayoverlay`, `bundle/`).
- `app_darwin.go`: `NSApplication sharedApplication`, `setActivationPolicy:` accessory (verify the enum value), run loop on the main thread; a Go channel bridge to schedule work on the main thread (`performSelectorOnMainThread` or a timer source).
- `statusitem_darwin.go`: `NSStatusBar systemStatusBar`, `statusItemWithLength:` variable, button title "ES", an `NSMenu` with items wired to an Objective-C target class whose action calls Go.
- `overlay_darwin.go`: borderless `NSPanel` (non-activating, floating level, ignores mouse events, all spaces), centred text, shown for 800 ms then `orderOut:`.
- `bundle/Info.plist`: `CFBundleIdentifier` `io.github.kikz0r.edgesync.spike`, `CFBundleExecutable`, `LSUIElement` true, `LSMinimumSystemVersion` 13.0, and the usage-description keys that Apple documents for the permissions in use (verify names in Apple docs; record which exist).
- `make-bundle.sh`: builds the darwin binary (arm64 + amd64 + `lipo` on CI), assembles `EdgeSyncSpike.app`, ad-hoc signs with `codesign -s - --force`.

## Item P0-SP4-006
Extend `.github/workflows/spikes.yml` macos job: after tests, run `make-bundle.sh`, `open -a spikes/out/EdgeSyncSpike.app --args -selftest 5s` (the app exits by itself), wait, check for crash reports under `~/Library/Logs/DiagnosticReports`, `screencapture -x spikes/out/screen.png`, upload artifacts. `qa/manual/p0-sp4-macos.md`: how to push the branch, dispatch the workflow (`gh workflow run spikes.yml` or the Actions UI), and which artifacts and log sections to share.

## Item P0-SP4-M01
User-owned. Results (run URL, logs, screenshots) into `qa/manual/results/p0-sp4-macos.md`.

## Item P0-SP4-007
`briefs/spikes/sp4.md`: summary table with an "evidence level" column; sections per mechanism; "Recommended design for P2 M1 and P4 F3"; "Unverifiable without a Mac" list; "Requires user decision" for contradictions of D-11, D-33, D-34, D-39 (for example: purego cannot implement a required delegate - cgo would violate D-34; Finder paste of promises unverifiable).

## Item P0-SP4-008
`spikes/macos/tap/README.md`. The spike rules (briefs/plan-phase-0.md, top) require a README in every spike directory; P0-SP4-002's file list
did not include one. Write it from the code in `spikes/macos/tap` and `spikes/macos/cmd/tap` and from the P0-SP4-002 report: purpose, exact
build command, exact run commands with every flag read from `cmd/tap/main.go`, what to observe, what to paste back, evidence level. Match the
shape of `spikes/macos/README.md`. Do not edit any Go file.

## Item P0-SP4-009
`spikes/macos/ui/README.md`. Same purpose and shape as P0-SP4-008 for the code merged by P0-SP4-005: `spikes/macos/ui`, `spikes/macos/cmd/trayoverlay`
and `spikes/macos/bundle` (Info.plist, make-bundle.sh). Read the code and the P0-SP4-005 report (second-source constants are listed in doc comments in
`spikes/macos/ui`; the open question is whether NSFloatingWindowLevel suffices above full-screen spaces, fallback NSStatusWindowLevel = 25). Do not edit
any other file.

## Item P0-SP4-010
`spikes/macos/inject/README.md`. Same purpose and shape as P0-SP4-008 and P0-SP4-009 for `spikes/macos/inject` and `spikes/macos/cmd/inject`
(found missing when P0-SP4-006 was merged). Read the code and its doc comments (constant provenance is second-source: servo core-graphics bindings,
Chromium kVK keycodes). Do not edit any other file.

## Item P0-SP4-011
Evidence: `qa/manual/results/p0-sp4-macos.md` findings 1 and 2 (SIGTRAP in `tap.(*loop).stop` at `runloop_darwin.go:98` from `Tap.Close`; `CGEventTapEnable(false)` not reflected at once;
disable and re-enable counters +2 where +1 was expected). Read `spikes/macos/tap/runloop_darwin.go` and `tap_darwin.go`, find the window in which `stop()` touches a run loop
whose thread has ended, and make close race-free; handle the two notifications idempotently; fix the test expectations to what macOS does and document it. CoreFoundation facts
must be verified against Apple documentation (CFRunLoopStop, CFRunLoopWakeUp, CFMachPortInvalidate lifetime rules).

## Item P0-SP4-012
Corrected 2026-11-04 (supersedes the first text): the CI log shows `change count: before clear 12, baseline 13, after write 13` and `Watch reported change count 0`. The poller
(`Pasteboard.Watch`: report whenever `changeCount` differs from the last value, starting from `since`) is correct. The count did not change because on macOS `changeCount`
increments when pasteboard ownership changes (`clearContents`, `declareTypes:owner:`), not when `setData:forType:` adds data after the clear. The test starts watching after `Clear()` at the
baseline and then only writes, so there is nothing for Watch to report. Fix the TEST: start Watch at the current count, THEN call `Clear()` (the count must increase), then `SetData`; assert Watch reports a
value greater than the starting count within the timeout; also assert (and log) that `SetData` alone does not change the count, so the behaviour is documented by the test. Verify the ownership-change rule
against Apple documentation (NSPasteboard changeCount, clearContents, declareTypes:owner:; cite it) and add one README line to `spikes/macos/pb/README.md`. Touch `pasteboard_darwin.go` only if you find a real defect (say which).

## Item P0-SP4-013
Evidence: finding 4: `NSFilePromiseProviderDelegate: Objective-C protocol not found`. Read how `spikes/macos/pb/promise_darwin.go` declares conformance when registering the delegate class
(purego `objc.RegisterClass` with a protocol list) and fix the lookup per the item text; verify `NSProtocolFromString` and the objc package API from sources.

## Item P0-SP4-014
Evidence: finding 5: `trayoverlay: setActivationPolicy: rejected the accessory policy`, process exits. Read `spikes/macos/ui/app_darwin.go`, `spikes/macos/bundle/Info.plist` (LSUIElement is set) and
`spikes/macos/cmd/trayoverlay/main.go`; decide from Apple documentation why the call returns NO and fix the cause. If the plist already makes the app an accessory app the explicit call may be
unnecessary: read the policy back and proceed only when it is accessory.
