# P0-SP4 macOS spikes - results (GitHub Actions run, P0-SP4-M01)

Run: https://github.com/kikz0rsk/edgesync/actions/runs/41880720011 (workflow `spikes`, workflow_dispatch on main at 4e85925, job `macos`,
2026-11-04 17:49 to 17:52 UTC). Provided by the user: job logs under `spikes/out/ci/logs/macos/` and the artifact `spikes-macos`
(unpacked under `spikes/out/ci/macos/artifact/`); both are gitignored. Author of the spikes never ran any of this code before; this is the first
execution on a Mac.

## Environment (what GitHub's macOS runner gives us)

- macOS 26.6.2 (build 25G83), arm64, `launchctl managername` = Aqua (a GUI session exists), console user `runner`.
- One virtual display: id 1, main, origin (0,0), 1024x768 points, 1024x768 pixels, scale 1.
- TCC is pre-granted for the test process: `AXIsProcessTrusted=true`, `CGPreflightListenEventAccess=true`, `CGPreflightPostEventAccess=true`
  (the prompt and request calls also return true). So event taps and injection can be exercised in CI without a person.
- Pasteboard at start: changeCount 22, no types, `concealed=false`.
- `make-bundle.sh` produced a universal (`x86_64 arm64`) `EdgeSyncSpike.app`; `plutil -lint` OK; the ad-hoc signature is "valid on disk" and
  "satisfies its Designated Requirement" (D-106 holds mechanically).
- Hosted macOS arm64 runners have long queue times: the first `ci` run's macOS job was cancelled after 15 minutes without a runner.

## What passed (real macOS evidence)

| Mechanism | Tests | Notes |
|---|---|---|
| purego framework loading, display enumeration | `TestDisplays_ListBoundsAndScale`, `TestNSString_RoundTrip` | bounds, pixels, scale and the CGWarpMouseCursorPosition read-back worked |
| CGEventPost injection | `TestInject_MoveClickKeyScroll` (hid and session), `TestInject_ClosedInjectorRejectsPosting`, `TestInject_NewRejectsInvalidLocation` | posted events were observed by a listen-only tap at both locations |
| Lazy pasteboard provider | `TestProvider_LazyTextHTMLRTFPNG` | provider is called lazily: after the second read the call count per type is exactly 1 (utf8-plain-text, html, rtf, png); `pasteboardFinishedWithDataProvider:` fired once after `clearContents` |
| Concealed marker | `TestConcealedType_Detected` | detection works (D-121: informational only) |
| UTI constants | `TestAppKitTypes_MatchLiterals` | AppKit values: `public.utf8-plain-text`, `public.html`, `public.rtf`, `public.png`, `public.file-url` match our literals |
| Event tap creation | `TestTap_CreateSessionTapReportsTrustState` (creation and the synthetic event part), `TestTap_ListenOnlyRejectsSuppression` | an active and a listen-only session tap were created; a synthetic mouse-moved event with our user-data tag `0x45444753` was observed at (512,384) |
| Permissions, cursor | `TestPermissions_PromptAndRequestReturnValues`, `TestCursor_AssociateAndHideReturnCodes` | all calls returned success codes (0 or true) |

## What failed (each becomes a fix item)

1. **Tap close crashes with SIGTRAP** (step "go test -tags integration ./..."): `SIGTRAP: trace trap ... signal arrived during cgo execution` in
   `tap.(*loop).stop` (`runloop_darwin.go:98`), called from `Tap.Close` via `inject.observePosted`'s deferred close
   (`inject_integration_test.go:194`). The same test passed in the second run of the identical test (step "macOS integration tests with log"),
   so this is a race: `stop()` calls into CoreFoundation for a run loop that may already have exited. Fix item P0-SP4-011.
2. **`CGEventTapEnable(false)` is not reflected at once** (`TestTap_CreateSessionTapReportsTrustState/active`): "tap still enabled after
   CGEventTapEnable(false)"; and in the listen-only subtest the disable/re-enable counters moved by 2 where the test expects +1 ("timeout:
   reenabled 0 -> 2", "user input: disable counter 1 -> 3, want +1", "reenabled 2 -> 4"): the disabled-by-timeout and disabled-by-user-input
   notifications are delivered twice per injected event (or the harness injects both paths twice). The M1 backend must treat these notifications as
   idempotent and must not read `CGEventTapIsEnabled` immediately after `CGEventTapEnable(false)`. Fix item P0-SP4-011.
3. **Pasteboard change-count poller reports 0** (`TestPasteboard_ChangeCountPolling`): "change count: before clear 12, baseline 13, after write 13"
   but "Watch reported change count 0, want at least 13": the poll loop's value is wrong (a type/width or first-sample bug in the spike's
   poller), so lazy-paste change detection is unproven. Fix item P0-SP4-012.
4. **File promises could not start** (`TestPromise_ProviderRegistersAndWritesFile`): `NewPromise: NSFilePromiseProviderDelegate: Objective-C
   protocol not found`. The runtime protocol lookup returned nothing (the protocol object is not registered until something references it), so
   the file-promise path (D-11, P4-F3) is unproven. Fix item P0-SP4-013.
5. **Tray/overlay spike exits at once**: `trayoverlay: setActivationPolicy: rejected the accessory policy` on stderr, empty stdout, process
   not running after 30 s, no crash report, so the status item and overlay were never created. Fix item P0-SP4-014.

## Open

- The screenshot `spikes/out/ci/macos/artifact/screen.png` was taken while the app was already gone; it shows no status item.
- Whether `NSFloatingWindowLevel` suffices above full-screen spaces, Finder paste of file promises, permission re-grant after updates, and
  anything needing a person at a Mac remain unverified.
- P0-SP4-007 (findings) waits for the fix items and a second dispatch of the workflow (user action) so the go/no-go rests on working code.
