# Stream brief - P1-F Foundation block (contracts)

Phase 1. Sequential, one agent at a time, in item order. Items: `briefs/plan-phase-1.md` section "P1-F".
Blocked until P0 exit (P0-X-003). Ends with the CONTRACT FREEZE (P1-F-022), after which C1..C5 fan out.

## Scope
Create every contract the five core streams and the P2 backends build against: build info, the HID usage type,
the peer ID type, the platform types and interfaces, fakes and conformance suites, Linux session detection, the
hot-path wire codec, the Cap'n Proto schemas with generated code, the Go-native message layer (`internal/proto/msg`),
the config schema with defaults, and the logging conventions. No business logic beyond what each item names.

## Inputs to read before your item
- `briefs/architecture.md` sections 2-8 (the contracts you transcribe).
- `briefs/spikes/p0-consolidation.md` and the approved P0 decisions in `decisions.md` (the parent pastes into your
  dispatch every approved change that touches your item; apply exactly those, nothing else).
- `briefs/spikes/sp5.md` for P1-F-011..019 (capnp generation command, limit API names, go-capnp v3.1.0-alpha.2 API).
- Decisions: D-08, D-36, D-38, D-41, D-42, D-43, D-46, D-47, D-48, D-49, D-51, D-55, D-56, D-57, D-58, D-67,
  D-69, D-75, D-76, D-78.

## Files you may touch
Only the files in your item's `Files:` line. `go.mod`/`go.sum` and `tools/coverage-exclude.txt` only when listed
(the parent holds the claim). Packages created here that are not yet in `ownership.md` (`internal/peerid`,
`internal/proto/msg`) are owned by P1-F until the freeze and frozen afterwards. Never edit plan files.

## Conventions (binding)
- Go 1.27, module `github.com/kikz0r/edgesync`. Dependencies only at the versions in `compatibility-matrix.md`.
- Package doc comment in every package (`doc.go`), zero other comments unless something is genuinely non-obvious.
- Enums are typed integers starting at 1; the zero value is invalid and prints as `<Type>(0)`. Every enum gets a
  `String()` method (table-driven, no `fmt` allocation for known values).
- Errors: sentinel per package (`var ErrX = errors.New("<pkg>: ...")`), wrapped with `%w`; no panics except the
  impossible-state assertion explicitly named in P1-F-010.
- Tests: stdlib `testing`, `testing/synctest` for timing, `github.com/google/go-cmp/cmp` for diffs, table-driven,
  `t.Parallel()` where safe (the `paralleltest` linter is on).
- Generated code is never hand-edited (D-67). Regenerate with `go generate ./internal/proto/capnp/...`.
- No em dashes or en dashes anywhere.

## Report format (reply with exactly this, nothing else)
```
FILES: <path> - <one clause>          (one line per file, or NONE)
GATES: <gate> pass|fail               (verbatim output only for a failure)
ITEMS: <item id> <FEAT-NNN, ...>
TESTS: <what the test phase must cover for this item, one line>
BLOCKED: <what stopped you, or NONE>
```

---

## Item P1-F-001
`internal/buildinfo` (package doc: "Package buildinfo exposes the build version, commit and wire protocol version.").
```go
var (
    Version = "0.0.0-dev" // set with -ldflags "-X github.com/kikz0r/edgesync/internal/buildinfo.Version=..."
    Commit  = "unknown"   // set with -ldflags "-X .../buildinfo.Commit=..."
)
const ProtocolVersion uint16 = 1
type Info struct {
    Version, Commit string
    ProtocolVersion uint16
    GoVersion, OS, Arch string // runtime.Version(), runtime.GOOS, runtime.GOARCH
}
func Get() Info
func (i Info) String() string // "EdgeSync 0.0.0-dev (unknown) go1.27.3 windows/amd64 protocol 1"
```
The two variables are the documented exception to D-75's "no package-level mutable state" (ldflags injection);
the short comment on them says so. Tests: defaults; assigning the vars in a test (restore with `t.Cleanup`) is
reflected by `Get()`; `String()` exact format; `ProtocolVersion == 1`. These tests cannot be parallel (they mutate
package vars): mark them with `//nolint:paralleltest // mutates ldflags variables`.

## Item P1-F-002
`internal/keymap/usage.go` (FROZEN after P1-F). Package doc: "Package keymap maps physical keys between USB HID
usages and the key codes of each operating system."
```go
type Usage uint32                      // page<<16 | id
const (PageKeyboard uint16 = 0x07; PageConsumer uint16 = 0x0C)
func Make(page, id uint16) Usage
func (u Usage) Page() uint16
func (u Usage) ID() uint16
func (u Usage) IsModifier() bool       // the 8 keyboard-page modifiers 0xE0..0xE7
type ModMask uint16
const (
    ModControlLeft ModMask = 1 << iota // bit order is the wire order of ModSync (architecture 5.4)
    ModControlRight
    ModShiftLeft
    ModShiftRight
    ModAltLeft
    ModAltRight
    ModMetaLeft
    ModMetaRight
    ModCapsLock
    ModNumLock
    ModScrollLock
)
func ModBit(u Usage) (ModMask, bool)   // modifiers and the three lock keys
func (m ModMask) Has(b ModMask) bool
type Remap map[Usage]Usage             // behaviour (validation, Apply) is implemented by C3 in remap.go
```
Named keyboard-page constants use the W3C/Chromium DOM `code` names so they match the generated tables of C3:
`KeyA`..`KeyZ` (0x04..0x1D), `Digit1`..`Digit9` (0x1E..0x26), `Digit0` (0x27), `Enter` 0x28, `Escape` 0x29,
`Backspace` 0x2A, `Tab` 0x2B, `Space` 0x2C, `CapsLock` 0x39, `F1`..`F12` (0x3A..0x45), `PrintScreen` 0x46,
`ScrollLock` 0x47, `Pause` 0x48, `Insert` 0x49, `Home` 0x4A, `PageUp` 0x4B, `Delete` 0x4C, `End` 0x4D,
`PageDown` 0x4E, `ArrowRight` 0x4F, `ArrowLeft` 0x50, `ArrowDown` 0x51, `ArrowUp` 0x52, `NumLock` 0x53,
`F13`..`F24` (0x68..0x73), `ControlLeft` 0xE0, `ShiftLeft` 0xE1, `AltLeft` 0xE2, `MetaLeft` 0xE3,
`ControlRight` 0xE4, `ShiftRight` 0xE5, `AltRight` 0xE6, `MetaRight` 0xE7 (all `Make(PageKeyboard, id)`).
Verify every id against the USB HID Usage Tables (keyboard/keypad page 0x07) from usb.org; name the document and
version in the test file header. The table lookup functions (`FromWindowsScan`, `ToWindowsScan`, `FromMacKeycode`,
`ToMacKeycode`, `FromEvdev`, `ToEvdev`) are NOT declared here; C3 adds them with exactly the signatures in
architecture section 3.

## Item P1-F-003
`internal/peerid` (package doc: "Package peerid defines the peer identity derived from a certificate's public
key (D-42)."). Rationale: transport, session, pki, peers, msg and agent all need the type; putting it in
`internal/peers` would make C1 depend on C2.
```go
type ID string
var ErrInvalid = errors.New("peerid: invalid peer id")
func FromSPKI(spkiDER []byte) ID              // base32.StdEncoding.WithPadding(NoPadding), lowercased, of sha256(spkiDER): 52 chars
func FromCertificate(c *x509.Certificate) ID  // FromSPKI(c.RawSubjectPublicKeyInfo)
func FromHash(h [32]byte) ID
func Parse(s string) (ID, error)              // exactly 52 chars of [a-z2-7]; anything else ErrInvalid (wrapped with the reason)
func (id ID) Hash() ([32]byte, error)         // inverse of FromHash; ErrInvalid on malformed ids
func (id ID) Short() string                   // first 8 chars as "xxxx-xxxx"; malformed ids return "invalid"
func (id ID) String() string
```
Known vector test: compute the expected id in the test from a fixed 91-byte SPKI fixture of a P-256 key (stored as
hex in the test) with an independent hand-rolled base32 encoding loop, not the same stdlib call.

## Item P1-F-004
Planning consolidation: the doc comment of `KindWheel` states D-87 (1/120 notch units, `DY > 0` = away from the user, `DX > 0` = right); the doc comment of `EdgeSegment` states D-88 (half-open rects, `At` = boundary line, Left/Top contact at `At`, Right/Bottom contact at `At - 1`, along-coordinate in `[From, To)`); `TestEdgeSegment_ContactRule` checks a helper `func (e EdgeSegment) Touches(p Point) bool` implementing exactly that rule. Coordinates are native units per D-89.
`internal/platform/types.go` and `errors.go` exactly as architecture section 3, with:
- `Side`: `SideLeft=1, SideRight, SideTop, SideBottom`. `Button`: `ButtonLeft=1, ButtonRight, ButtonMiddle, ButtonX1,
  ButtonX2` (these byte values are also the wire values, P1-F-010). `EventKind`: `KindMotion=1, KindButton,
  KindWheel, KindKey, KindEdgeContact, KindCaptureLost`.
- `Rect` helpers: `Right() int32` (X+W), `Bottom() int32` (Y+H), `Contains(p Point) bool` (half-open:
  X <= p.X < X+W, same for Y), `Empty() bool` (W <= 0 || H <= 0).
- `errors.go`: `ErrUnsupported = errors.New("platform: unsupported")`, `ErrPermission = errors.New("platform:
  permission denied")`, `ErrClosed = errors.New("platform: closed")`.
- Package doc: "Package platform defines the operating-system boundary: shared types and the interfaces every
  backend implements."
`TestInputEvent_HasNoReferenceFields` walks `reflect.TypeOf(InputEvent{})` fields recursively and fails on any
kind in {Pointer, Interface, Slice, Map, String, Func, Chan, UnsafePointer} (D-76).

## Item P1-F-005
Planning consolidation: `capture.go` also declares the optional interface `type ContactHolder interface { HoldsContact() bool }` (D-90) with a doc comment: capturers whose edge contact already holds the pointer (Wayland barrier activation) implement it; the engine answers every contact with Grab or Ungrab and decides within 400 ms for holders. `TestContactHolder_IsOptional` asserts a fake capturer without the method does not satisfy it.
Transcribe architecture sections 4.1-4.7 into the listed files, character-identical signatures, one concern per
file (`backend.go`: `Backend`, `Options`; `capture.go`: `InputCapturer`, `KeySwallower`, `CaptureStats`;
`inject.go`: `InputInjector`; `displays.go`: `Displays`; `clipboard.go`: `Format` + constants, `FormatInfo`,
`LocalChange`, `Provider`, `ClipboardBackend`; `session.go`: `SessionEventKind` + constants, `SessionEvents`;
`ui.go`: `TrayState`, `TrayItem`, `TrayModel`, `Tray`, `Overlay`; `autostart.go`; `permissions.go`:
`PermissionKind`, `PermissionState`, `Permissions`; `theme.go`: `Theme`, `ThemeWatcher`).
- `CaptureStats struct { Events, Drops, WatchdogReinstalls uint64; MaxCallback time.Duration }`.
- Enums start at 1: `SessLocked=1, SessUnlocked, SessScreensaverOn, SessScreensaverOff, SessDisplaySleep,
  SessDisplayWake, SessSuspend, SessResume, SessSecureDesktopOn, SessSecureDesktopOff`; `TrayIdle=1,
  TrayConnected, TrayRemoteActive, TrayError`; `PermAccessibility=1, PermInputMonitoring, PermPortalInputCapture,
  PermPortalRemoteDesktop, PermPortalClipboard`; `ThemeLight=1, ThemeDark`.
- `Format` constants: `FormatText = "text/plain;charset=utf-8"`, `FormatHTML = "text/html"`, `FormatRTF =
  "text/rtf"`, `FormatPNG = "image/png"`.
- `host.Open` is NOT declared anywhere in this item (see P1-F-009).
- Apply only the interface changes the parent pastes from the approved P0 decisions; if none are pasted, the
  architecture text is exact.

## Item P1-F-006
`internal/platform/fake`, package doc "Package fake provides deterministic in-memory implementations of every
platform interface for tests." All types are safe for concurrent use (one `sync.Mutex` each) and never start
goroutines except where an interface method blocks by contract (`Watch`, `Run`), which return on ctx cancellation.
- `Displays`: `NewDisplays(d platform.Desktop) *Displays`; `Set(d platform.Desktop)` stores and calls every active
  watcher fn synchronously in registration order; `Current` returns a deep copy; `Watch` registers fn, blocks until
  ctx done, unregisters, returns `ctx.Err()`.
- `Capturer`: `NewCapturer() *Capturer`. `Start(ctx, sink)` stores sink (second Start -> error);
  `Emit(ev platform.InputEvent) bool` non-blocking send, on full sink increments `Stats().Drops` and returns false;
  `Events` counts sends; `SetEdges` stores a copy, readable via `Edges()`; `Grab`/`Ungrab(at)` toggle `Grabbed()`
  and record `LastUngrab() platform.Point`; `SwallowKeys(set)` stores a copy readable via `Swallowed()`;
  `Close` idempotent; every method after Close returns `platform.ErrClosed`; `SetError(method string, err error)`
  makes the named method fail once (for engine error-path tests).
- `Injector`: `NewInjector() *Injector`; every method appends `Call{Op string; P platform.Point; B platform.Button;
  Down bool; U keymap.Usage; DX, DY int32}` (`Op` in {"move", "button", "wheel", "key", "releaseall", "show"});
  tracks pressed keys and buttons; `ReleaseAll` appends one "key" up call per pressed key sorted by Usage, then one
  "button" up per pressed button sorted by Button, then the "releaseall" marker; `Log() []Call` returns a copy;
  `Reset()`; `SetError` as above; Close semantics as above.
- Adds `github.com/google/go-cmp v0.7.0` to go.mod (tests use `cmp.Diff`).

## Item P1-F-007
- `Clipboard`: `NewClipboard()`; `SimulateLocalCopy(data map[platform.Format][]byte) uint64` sets
  local content, increments seq, calls every watcher with `LocalChange{Seq, Formats (sorted by Format, Size =
  len)}`, returns seq; `Read(ctx, f, w)` writes local data for f (unknown format -> error wrapping
  `ErrFormatUnavailable`, exported from fake); `Offer(ctx, formats, p)` replaces local content with a remote offer
  (no watcher call: echo prevention); `Read` of an offered format calls `p.Render`; `SimulatePaste(ctx, f) ([]byte,
  error)` is the "local application pastes" path and calls the provider (or local data); `Offers() int` counts
  Offer calls.
- `Session`: `Emit(k platform.SessionEventKind)` calls watchers; `Lock`, `StartScreensaver`, `WakeDisplay` record
  calls readable via `Calls() []string`.
- `Tray`: `Run(ctx, m, onAction)` stores model and handler, blocks until ctx done; `Update(m)` stores (error if Run
  has not started); `Model()`; `Click(id string) error` invokes the handler (error when not running).
  `Overlay`: `Show` records `Shown{Text string; D time.Duration}`.
- `Autostart`: in-memory bool, `SetError`. `Permissions`: `SetStatus([]platform.PermissionState)`, `Request`
  records kinds. `Theme`: `Set(t)` notifies watchers; `Current`.
- `Backend`: `New() *Backend` with exported fields `Displays *Displays; Capturer *Capturer; Injector *Injector;
  Clipboard *Clipboard; Session *Session; Tray *Tray; Overlay *Overlay; Autostart *Autostart; Permissions
  *Permissions; Theme *Theme`; methods implement `platform.Backend`; `Close` closes capturer, injector and
  clipboard and is idempotent.
- `var _ platform.<Iface> = (*<Type>)(nil)` for every pair.

## Item P1-F-008
Planning consolidation: `RunInjector`'s `WheelHighRes` subtest asserts the D-87 sign convention through the observer; `RunCapturer` includes a subtest that every `KindEdgeContact` position satisfies `EdgeSegment.Touches` (D-88).
`internal/platform/conformance`, package doc "Package conformance holds the behavioural test suites every platform
backend must pass (fakes in unit tests, OS backends under the integration build tag)." The package imports
`testing` in non-test files (like `testing/fstest`).
Drivers (`drivers.go`) abstract how a suite causes and observes real OS effects:
```go
type CaptureDriver interface {   // produces physical-looking input on the machine under test
    MovePointer(ctx context.Context, p platform.Point) error
    PressKey(ctx context.Context, u keymap.Usage, down bool) error
    PressButton(ctx context.Context, b platform.Button, down bool) error
    PointerPosition(ctx context.Context) (platform.Point, error)
}
type InjectObserver interface {  // observes what reached the OS after injection
    Next(ctx context.Context) (platform.InputEvent, error)
    PointerPosition(ctx context.Context) (platform.Point, error)
}
type ClipboardDriver interface { // acts as "another application" on the local clipboard
    CopyLocal(ctx context.Context, data map[platform.Format][]byte) error
    PasteLocal(ctx context.Context, f platform.Format) ([]byte, error)
}
type SessionDriver interface { Trigger(ctx context.Context, k platform.SessionEventKind) error }
type DisplaysDriver interface { Change(ctx context.Context) (expect platform.Desktop, err error) }
```
Suites (each `func RunX(t *testing.T, ...)` with named `t.Run` subtests):
- `RunCapturer(t, newCap func(t *testing.T) platform.InputCapturer, d CaptureDriver)`: StartDeliversKeys,
  EdgeContactOnDeclaredEdgeOnly (declares one segment; moving onto it yields `KindEdgeContact` with its ID; moving
  elsewhere does not), GrabSuppressesAndDeliversDeltas (after Grab, MovePointer yields KindMotion with non-zero
  DX/DY and `PointerPosition` stays parked), UngrabPlacesCursor, SwallowKeysSuppressesLocalDelivery,
  CloseStopsDelivery, StatsCount.
- `RunInjector(t, newInj, o InjectObserver)`: MoveAbsLandsExactly, ButtonDownUp (incl. X1/X2), WheelHighRes,
  KeyDownUpByUsage, ReleaseAllReleasesOnlyInjected, ShowCursorAt, CloseIdempotent.
- `RunDisplays(t, newDisp, d DisplaysDriver)`: CurrentNonEmptyWithPrimary, WatchReportsChange, WatchStopsOnCancel.
- `RunClipboard(t, newClip, d ClipboardDriver)`: WatchReportsLocalCopy, OwnOfferNotReported, ReadCanonical,
  OfferRenderedLazilyOnPaste (provider called only at paste time, once per format), ProviderErrorFailsPaste
  (paste returns an error, never empty data). There is no sensitivity subtest (D-121).
- `RunSession`, `RunTray` (UpdateBeforeRunErrors, ClickInvokesAction where the driver can click), `RunOverlay`
  (ShowReturnsPromptly), `RunAutostart` (RoundTrip, Idempotent), `RunTheme` (CurrentValid, WatchStopsOnCancel).
The fakes satisfy the drivers through small adapters in `conformance_test.go` (fake capturer: MovePointer emits
motion/contact events computed against declared edges; fake injector: observer reads `Log()`).
`TestSuites_FailOnBrokenImplementation`: wrap each fake in a deliberately broken decorator (drops one behaviour) and
run the suite with a recording `testing.TB` substitute (implement the suites against an internal interface
`tb` = `testing.TB` subset so a recorder can be passed); assert at least one failure per suite.

## Item P1-F-009
`internal/platform/host/host.go`, package doc: "Package host opens the platform backend for the running operating
system. Open is implemented per OS in host_<goos>.go (P2) with the signature `func Open(ctx context.Context, opts
platform.Options) (platform.Backend, error)`."
```go
type LinuxSession uint8
const (LinuxX11 LinuxSession = 1; LinuxWayland LinuxSession = 2)
var ErrUnknownSession = errors.New("host: cannot determine the Linux graphical session type")
func DetectLinuxSession(getenv func(string) string) (LinuxSession, error)
```
Rules: `XDG_SESSION_TYPE == "wayland"` -> Wayland; `== "x11"` -> X11; empty: `WAYLAND_DISPLAY != ""` -> Wayland,
else `DISPLAY != ""` -> X11, else ErrUnknownSession; any other value (`tty`, `mir`, ...) -> ErrUnknownSession wrapped
with the value. No guessing beyond these rules. The function is exported so `host_linux.go` (P2 X1) uses it and the
Windows build has no unused code.

## Item P1-F-010
`internal/proto/wire` (FROZEN). Package doc: "Package wire encodes the fixed-size hot-path input frames
(architecture 5.4)." Little-endian; layout offsets (byte 0 = type, byte 1 = flags):

| Type | Value | Size | Payload offsets |
|---|---|---|---|
| MotionAbs | 0x01 | 18 | [2:6] seq u32, [6:10] x i32, [10:14] y i32, [14:18] tsDelta u32 |
| Button | 0x02 | 4 | [2] button u8 (1..5), [3] down u8 (0/1) |
| Wheel | 0x03 | 10 | [2:6] dx i32, [6:10] dy i32 |
| Key | 0x04 | 8 | [2:6] usage u32, [6] down u8, [7] repeat u8 |
| ReleaseAll | 0x05 | 2 | - |
| ModSync | 0x06 | 4 | [2:4] mods u16 (keymap.ModMask bits) |
| Heartbeat | 0x07 | 10 | [2:6] seq u32, [6:10] tsDelta u32 |
| HeartbeatAck | 0x08 | 6 | [2:6] seq u32 |

```go
type Type uint8 // TypeMotionAbs .. TypeHeartbeatAck with the values above
const MaxFrameSize = 18
func Size(t Type) (int, bool)
type Frame struct {
    Type    Type
    Flags   uint8        // reserved: encoders write the given value (callers use 0); decoders return it unchanged
    Seq     uint32       // MotionAbs, Heartbeat, HeartbeatAck
    X, Y    int32        // MotionAbs
    DX, DY  int32        // Wheel (1/120 notch units)
    TSDelta uint32       // MotionAbs, Heartbeat (microseconds since session start)
    Button  uint8        // equals platform.Button values
    Down    bool
    Usage   keymap.Usage // Key
    Repeat  bool
    Mods    keymap.ModMask
}
var (ErrShort, ErrUnknownType, ErrMalformed = ...)          // "wire: ..." sentinels
func AppendFrame(dst []byte, f Frame) []byte                // panics on an unknown Type (impossible-state assertion; callers use constants)
func DecodeFrame(src []byte) (Frame, int, error)            // ErrShort when src is shorter than the type's size (or empty)
func DecodeDatagram(src []byte) (Frame, error)              // exactly one frame; len mismatch -> ErrMalformed
type Decoder struct { /* r io.Reader; buf [MaxFrameSize]byte */ }
func NewDecoder(r io.Reader) *Decoder
func (d *Decoder) Next() (Frame, error)                     // io.EOF only at a frame boundary, io.ErrUnexpectedEOF mid-frame
```
Strict decoding: bool bytes other than 0/1 and button bytes outside 1..5 -> ErrMalformed; unknown type ->
ErrUnknownType wrapped with the type value. Fields not used by a type are zero after decoding. Allocation tests:
`testing.AllocsPerRun(1000, ...)` == 0 for `AppendFrame` into a preallocated `make([]byte, 0, 64)` (reset each run),
`DecodeFrame`, `DecodeDatagram` and `Decoder.Next` over a `bytes.Reader` reset per run. Errors on the hot path must
not allocate for ErrShort (return the sentinel unwrapped).

## Item P1-F-011
`tools/capnpgen` (package main, part of the main module). Usage:
`go run ./tools/capnpgen -schema-dir <dir> -out <dir>` (relative paths resolve against the current directory, which
is the package directory under `go generate`).
Steps in order, each failure exits 2 with a one-line reason on stderr:
1. `findCapnp(getenv func(string) string, lookPath func(string) (string, error)) (string, error)`: `$CAPNP` if set
   (must exist), else `lookPath("capnp")`; neither -> error naming `CAPNP`.
2. Module root: `go env GOMOD` -> its directory.
3. Build capnpc-go: `go build -modfile=<root>/tools/tools.mod -o <tmpdir>/capnpc-go<.exe> capnproto.org/go/capnp/v3/capnpc-go`.
4. Std include dir: `go list -modfile=<root>/tools/tools.mod -m -f '{{.Dir}}' capnproto.org/go/capnp/v3` + `/std`
   (`stdIncludeDir(goListOutput string) (string, error)` trims and validates).
5. `schemaFiles(dir) ([]string, error)`: `*.capnp` only, sorted.
6. Run the compile command recorded in `briefs/spikes/sp5.md` ("exact generation command"), built by
   `compileArgs(capnpcGo, stdDir, schemaDir, outDir string, files []string) []string`. Default shape (use only if
   sp5.md matches it): `compile -I<stdDir> -o<capnpcGo>:<outDir> --src-prefix=<schemaDir> <files...>`.
7. Generated files must land at `internal/proto/capnp/<name>cp/<name>.capnp.go`. If capnpc-go writes another
   layout, `-out` handling moves files into this layout (documented in sp5.md); anything else -> BLOCKED.
`main.go` holds only flag parsing and `exec.CommandContext` calls; all decisions are pure functions in
`capnpgen.go` tested by the listed tests (TestCompileArgs_MatchRecordedCommand compares with the literal string from
sp5.md).

## Item P1-F-012
Planning consolidation: D-97 already excludes `internal/proto/capnp/...` from the coverage gate; this item adds that line to `tools/coverage-exclude.txt` (claim) with a comment citing D-97.
Write three schema files and the generate wiring; then `go generate ./internal/proto/capnp/...`.
- Every schema file starts with its own id from `capnp id`, imports go-capnp's `/go.capnp` and sets
  `$Go.package("<name>cp")` and `$Go.import("github.com/kikz0r/edgesync/internal/proto/capnp/<name>cp")`.
- `internal/proto/capnp/doc.go`: package `capnp`, doc "Package capnp holds Cap'n Proto code generated from
  proto/schema; never edit it by hand (D-67).", and
  `//go:generate go run ../../../tools/capnpgen -schema-dir ../../../proto/schema -out .`
- `tools/coverage-exclude.txt`: append `internal/proto/capnp/...` with a comment citing the decision number the
  parent gives you (generated code, D-67). If the parent has not given a decision number: BLOCKED.
- Adds `capnproto.org/go/capnp/v3 v3.1.0-alpha.2` to go.mod.

`common.capnp`:
```
enum Role { unspecified @0; server @1; client @2; }
enum Side { unspecified @0; left @1; right @2; top @3; bottom @4; }
enum ErrorCode { unspecified @0; internal @1; invalidArgument @2; notFound @3; permissionDenied @4;
  unsupported @5; busy @6; timeout @7; tooLarge @8; cancelled @9; unavailable @10; conflict @11; }
struct ErrorInfo { code @0 :ErrorCode; message @1 :Text; }
struct Rect { x @0 :Int32; y @1 :Int32; w @2 :Int32; h @3 :Int32; }
struct Monitor { id @0 :Text; bounds @1 :Rect; scalePermille @2 :UInt16; primary @3 :Bool; name @4 :Text; }
struct Desktop { monitors @0 :List(Monitor); }
```
Peer IDs travel as `Data` holding the 32-byte SHA-256 hash (`peerid.ID.Hash()`).

`handshake.capnp`:
```
struct Capabilities { datagrams @0 :Bool; clipboardFormats @1 :List(Text); files @2 :Bool; dnd @3 :Bool;
  lockSync @4 :Bool; secureDesktop @5 :Bool; }
struct Hello { protocolVersion @0 :UInt16; buildVersion @1 :Text; role @2 :Common.Role; name @3 :Text;
  peerId @4 :Data; capabilities @5 :Capabilities; macs @6 :List(Data); broadcastAddrs @7 :List(Text);
  desktop @8 :Common.Desktop; os @9 :Text; sessionNonce @10 :Data; }
struct Welcome { info @0 :Hello; }
enum RejectReason { unspecified @0; busy @1; versionMismatch @2; notPaired @3; roleConflict @4; shuttingDown @5; }
struct Reject { reason @0 :RejectReason; detail @1 :Text; holderName @2 :Text; holderPeerId @3 :Data; }
```
`sessionNonce` (16 random bytes chosen by the dialer) ties TCP-fallback data connections to their session
(architecture 5.2); it is an addition to architecture 5.3 recorded by the freeze.

`session.capnp`:
```
struct Enter { x @0 :Int32; y @1 :Int32; mods @2 :UInt16; clipboardSeq @3 :UInt64; clipboardOrigin @4 :Data; }
enum LeaveReason { unspecified @0; edge @1; hotkey @2; panic @3; lost @4; secureDesktop @5; roleChange @6; }
struct Leave { reason @0 :LeaveReason; }
struct LayoutMachine { peerId @0 :Data; name @1 :Text; originX @2 :Int32; originY @3 :Int32;
  desktop @4 :Common.Desktop; online @5 :Bool; }
struct LayoutSnapshot { version @0 :UInt64; machines @1 :List(LayoutMachine); }
struct DesktopChanged { desktop @0 :Common.Desktop; }
enum LockSyncKind { unspecified @0; lock @1; screensaverStart @2; wakeDisplay @3; }
struct LockSync { kind @0 :LockSyncKind; }
struct Goodbye { reason @0 :Text; }
struct Ping { id @0 :UInt64; sentMicros @1 :UInt64; }
struct Pong { id @0 :UInt64; sentMicros @1 :UInt64; }
struct WakeRequest { targetPeerId @0 :Data; }
```

## Item P1-F-013
`clipboard.capnp`:
```
struct FormatInfo { mime @0 :Text; size @1 :Int64; }
struct ClipAnnounce { origin @0 :Data; seq @1 :UInt64; formats @2 :List(FormatInfo); files @3 :Bool; }
struct ClipFetch { requestId @0 :UInt64; origin @1 :Data; seq @2 :UInt64; mime @3 :Text; requester @4 :Data; }
struct ClipData { requestId @0 :UInt64; mime @1 :Text; size @2 :Int64; }
struct ClipError { requestId @0 :UInt64; error @1 :Common.ErrorInfo; }
```
`transfer.capnp` (manifests are paged so one message stays far below the 16 MiB control cap):
```
struct Entry { path @0 :Text; size @1 :Int64; modTimeUnixNano @2 :Int64; mode @3 :UInt32; isDir @4 :Bool; }
enum OfferPurpose { unspecified @0; send @1; clipboard @2; dnd @3; }
struct FileOffer { transferId @0 :Text; origin @1 :Data; target @2 :Data; purpose @3 :OfferPurpose;
  totalEntries @4 :UInt32; totalSize @5 :Int64; skippedSymlinks @6 :List(Text); skippedSymlinkCount @7 :UInt32;
  entries @8 :List(Entry); last @9 :Bool; }
struct FileOfferMore { transferId @0 :Text; startIndex @1 :UInt32; entries @2 :List(Entry); last @3 :Bool; }
struct FileAccept { transferId @0 :Text; }
struct FileReject { transferId @0 :Text; error @1 :Common.ErrorInfo; }
struct ChunkRequest { transferId @0 :Text; fileIndex @1 :UInt32; offset @2 :Int64; length @3 :Int64; }
struct ChunkHeader { transferId @0 :Text; fileIndex @1 :UInt32; offset @2 :Int64; length @3 :Int64; }
struct Progress { transferId @0 :Text; bytesDone @1 :Int64; filesDone @2 :UInt32; }
struct Cancel { transferId @0 :Text; reason @1 :Text; }
struct Complete { transferId @0 :Text; fileHashes @1 :List(Data); }
struct ResumeInfo { transferId @0 :Text; fileIndex @1 :UInt32; verifiedOffset @2 :Int64; }
```
`dnd.capnp` (P4-F may add fields additively under a decision):
```
struct DragBegin { dragId @0 :Text; offer @1 :Transfer.FileOffer; x @2 :Int32; y @3 :Int32; }
struct DragMove { dragId @0 :Text; x @1 :Int32; y @2 :Int32; }
struct DragDrop { dragId @0 :Text; x @1 :Int32; y @2 :Int32; }
struct DragCancel { dragId @0 :Text; reason @1 :Text; }
struct DragResult { dragId @0 :Text; dropped @1 :Bool; target @2 :Text; }
```
`control.capnp` (the single message type on the Control stream, plus the data-stream header):
```
struct ControlMessage { union {
  hello @0 :Handshake.Hello; welcome @1 :Handshake.Welcome; reject @2 :Handshake.Reject;
  enter @3 :Session.Enter; leave @4 :Session.Leave; layoutSnapshot @5 :Session.LayoutSnapshot;
  desktopChanged @6 :Session.DesktopChanged; lockSync @7 :Session.LockSync; goodbye @8 :Session.Goodbye;
  ping @9 :Session.Ping; pong @10 :Session.Pong; wakeRequest @11 :Session.WakeRequest;
  clipAnnounce @12 :Clipboard.ClipAnnounce; clipFetch @13 :Clipboard.ClipFetch; clipError @14 :Clipboard.ClipError;
  fileOffer @15 :Transfer.FileOffer; fileOfferMore @16 :Transfer.FileOfferMore; fileAccept @17 :Transfer.FileAccept;
  fileReject @18 :Transfer.FileReject; chunkRequest @19 :Transfer.ChunkRequest; progress @20 :Transfer.Progress;
  cancel @21 :Transfer.Cancel; complete @22 :Transfer.Complete; resumeInfo @23 :Transfer.ResumeInfo;
  dragBegin @24 :Dnd.DragBegin; dragMove @25 :Dnd.DragMove; dragDrop @26 :Dnd.DragDrop;
  dragCancel @27 :Dnd.DragCancel; dragResult @28 :Dnd.DragResult; } }
struct DataHeader { union { clip @0 :Clipboard.ClipData; chunk @1 :Transfer.ChunkHeader; } }
```

## Item P1-F-014
`pairing.capnp`:
```
struct PairHello { name @0 :Text; peerId @1 :Data; role @2 :Common.Role; protocolVersion @3 :UInt16; os @4 :Text; }
struct Spake2Share { share @0 :Data; }
struct Spake2Confirm { mac @0 :Data; }
enum PairFailure { unspecified @0; wrongCode @1; expired @2; tooManyAttempts @3; cancelled @4; busy @5;
  versionMismatch @6; internal @7; }
struct PairResult { accepted @0 :Bool; reason @1 :PairFailure; }
struct PairingMessage { union { hello @0 :PairHello; share @1 :Spake2Share; confirm @2 :Spake2Confirm;
  result @3 :PairResult; } }
```
`service.capnp` (request union = exactly the D-47 whitelist; helper messages are control-plane only, the injection
data plane between agent and helper is designed by P2 W2 from SP1 findings):
```
enum HashAlg { unspecified @0; sha256 @1; sha384 @2; sha512 @3; }
struct ServiceHello { agentVersion @0 :Text; sessionId @1 :UInt32; }
struct SignTlsRequest { digest @0 :Data; hash @1 :HashAlg; }
struct InputDesktopReport { desktopName @0 :Text; }
struct ServiceRequest { id @0 :UInt64; union { hello @1 :ServiceHello; helperStatus @2 :Void;
  signTls @3 :SignTlsRequest; requestSas @4 :Void; reportInputDesktop @5 :InputDesktopReport; } }
struct ServiceHelloResult { serviceVersion @0 :Text; machinePeerId @1 :Data; certificate @2 :Data; }
struct HelperStatus { running @0 :Bool; sessionId @1 :UInt32; desktop @2 :Text; preLogin @3 :Bool; }
struct ServiceResponse { id @0 :UInt64; union { error @1 :Common.ErrorInfo; hello @2 :ServiceHelloResult;
  helperStatus @3 :HelperStatus; signature @4 :Data; ok @5 :Void; } }
struct HelperHello { agentPid @0 :UInt32; sessionId @1 :UInt32; }
struct HelperRequest { id @0 :UInt64; union { hello @1 :HelperHello; startCapture @2 :Void; stopCapture @3 :Void; } }
struct HelperResponse { id @0 :UInt64; union { error @1 :Common.ErrorInfo; ok @2 :Void; } }
```

## Item P1-F-015
`ipc.capnp` (architecture section 7). Imports common, session (for `LayoutSnapshot`), pairing (for `PairFailure`).
```
enum LogLevel { unspecified @0; debug @1; info @2; warn @3; error @4; }
enum PermissionKind { unspecified @0; accessibility @1; inputMonitoring @2; portalInputCapture @3;
  portalRemoteDesktop @4; portalClipboard @5; }
enum Theme { unspecified @0; light @1; dark @2; }
enum AgentState { unspecified @0; idle @1; connected @2; remoteActive @3; error @4; }
struct IpcHello { token @0 :Data; guiVersion @1 :Text; }
struct PeerRef { peerId @0 :Data; }
struct ManualPeer { host @0 :Text; port @1 :UInt16; }
struct StartPairing { peerId @0 :Data; addresses @1 :List(Text); port @2 :UInt16; }
struct PairingCode { sessionId @0 :UInt64; code @1 :Text; }
struct PairingSessionRef { sessionId @0 :UInt64; }
struct RenamePeer { peerId @0 :Data; name @1 :Text; }
struct SendFiles { peerId @0 :Data; paths @1 :List(Text); }
struct TransferRef { transferId @0 :Text; }
struct GetLogs { sinceSeq @0 :UInt64; minLevel @1 :LogLevel; }
struct ExportDiagnostics { path @0 :Text; }
struct ConfigDoc { toml @0 :Text; version @1 :UInt64; }
struct UpdateConfig { toml @0 :Text; baseVersion @1 :UInt64; }
struct HotkeyBinding { action @0 :Text; keys @1 :Text; }
struct Hotkeys { bindings @0 :List(HotkeyBinding); }
struct SessionInfo { peerId @0 :Data; name @1 :Text; kind @2 :Text; rttMicros @3 :UInt32; online @4 :Bool; }
struct ActiveMachine { peerId @0 :Data; name @1 :Text; home @2 :Bool; }
struct Status { role @0 :Common.Role; state @1 :AgentState; active @2 :ActiveMachine; lockToScreen @3 :Bool;
  sessions @4 :List(SessionInfo); agentVersion @5 :Text; updateAvailable @6 :Bool; }
struct PeerInfo { peerId @0 :Data; name @1 :Text; role @2 :Common.Role; online @3 :Bool; lastAddress @4 :Text;
  os @5 :Text; connectionKind @6 :Text; }
enum DiscoverySource { unspecified @0; mdns @1; manual @2; }
struct DiscoveredPeer { peerId @0 :Data; name @1 :Text; role @2 :Common.Role; addresses @3 :List(Text);
  port @4 :UInt16; tcpPort @5 :UInt16; protocolVersion @6 :UInt16; paired @7 :Bool; source @8 :DiscoverySource; }
enum PairingState { unspecified @0; waitingForCode @1; codeShown @2; verifying @3; succeeded @4; failed @5; cancelled @6; }
struct PairingSession { sessionId @0 :UInt64; state @1 :PairingState; code @2 :Text; attemptsLeft @3 :UInt8;
  expiresUnixMs @4 :Int64; failure @5 :Pairing.PairFailure; peer @6 :PeerInfo; }
enum TransferDirection { unspecified @0; send @1; receive @2; }
enum TransferState { unspecified @0; queued @1; active @2; completed @3; failed @4; cancelled @5; }
struct TransferInfo { id @0 :Text; peerId @1 :Data; direction @2 :TransferDirection; state @3 :TransferState;
  files @4 :UInt32; filesDone @5 :UInt32; bytesTotal @6 :Int64; bytesDone @7 :Int64; bytesPerSecond @8 :Int64;
  etaSeconds @9 :Int64; error @10 :Common.ErrorInfo; destination @11 :Text; }
struct LogAttr { key @0 :Text; value @1 :Text; }
struct LogRecord { seq @0 :UInt64; timeUnixNano @1 :Int64; level @2 :LogLevel; message @3 :Text; attrs @4 :List(LogAttr); }
struct PermissionState { kind @0 :PermissionKind; granted @1 :Bool; detail @2 :Text; }
struct UpdateInfo { available @0 :Bool; latestVersion @1 :Text; url @2 :Text; checkedUnixMs @3 :Int64; }
enum NotificationKind { unspecified @0; info @1; warning @2; error @3; }
struct Notification { kind @0 :NotificationKind; title @1 :Text; body @2 :Text; }
struct IpcRequest { id @0 :UInt64; union {
  hello @1 :IpcHello; getStatus @2 :Void; getConfig @3 :Void; updateConfig @4 :UpdateConfig; getLayout @5 :Void;
  setLayout @6 :Session.LayoutSnapshot; listPeers @7 :Void; listDiscovered @8 :Void; addManualPeer @9 :ManualPeer;
  startPairing @10 :StartPairing; submitPairingCode @11 :PairingCode; showPairingCode @12 :Void;
  cancelPairing @13 :PairingSessionRef; unpair @14 :PeerRef; renamePeer @15 :RenamePeer; setRole @16 :Common.Role;
  connect @17 :PeerRef; disconnect @18 :PeerRef; sendFiles @19 :SendFiles; listTransfers @20 :Void;
  cancelTransfer @21 :TransferRef; retryTransfer @22 :TransferRef; openReceiveFolder @23 :Void; getLogs @24 :GetLogs;
  exportDiagnostics @25 :ExportDiagnostics; wakePeer @26 :PeerRef; setLockToScreen @27 :Bool; jumpTo @28 :PeerRef;
  returnHome @29 :Void; checkUpdates @30 :Void; setAutostart @31 :Bool; getPermissions @32 :Void;
  requestPermission @33 :PermissionKind; getHotkeys @34 :Void; setHotkeys @35 :Hotkeys; captureHotkey @36 :Void;
  quit @37 :Void; } }
struct IpcResponse { id @0 :UInt64; union {
  error @1 :Common.ErrorInfo; ok @2 :Void; status @3 :Status; config @4 :ConfigDoc; layout @5 :Session.LayoutSnapshot;
  peers @6 :List(PeerInfo); discovered @7 :List(DiscoveredPeer); pairing @8 :PairingSession;
  transfers @9 :List(TransferInfo); transferId @10 :Text; logs @11 :List(LogRecord);
  permissions @12 :List(PermissionState); hotkeys @13 :Hotkeys; hotkeyCaptured @14 :HotkeyBinding;
  update @15 :UpdateInfo; } }
struct IpcEvent { union {
  statusChanged @0 :Status; activeMachineChanged @1 :ActiveMachine; peersChanged @2 :List(PeerInfo);
  discoveryChanged @3 :List(DiscoveredPeer); pairingProgress @4 :PairingSession; transferProgress @5 :TransferInfo;
  transferFinished @6 :TransferInfo; logRecord @7 :LogRecord; configChanged @8 :ConfigDoc;
  layoutChanged @9 :Session.LayoutSnapshot; updateAvailable @10 :UpdateInfo;
  permissionStateChanged @11 :List(PermissionState); themeChanged @12 :Theme; focusGui @13 :Void;
  notification @14 :Notification; } }
struct IpcFrame { union { request @0 :IpcRequest; response @1 :IpcResponse; event @2 :IpcEvent; } }
```
Config travels as TOML text with an optimistic version (`UpdateConfig.baseVersion` must equal the agent's current
version, else `conflict`); the GUI uses `internal/config` types to edit it.

## Item P1-F-016
`internal/proto/msg`: the Go-native message layer. Only `internal/proto/...` (plus session/ipc/service, D-55) may
import the generated packages; every other package uses these Go types. Package doc: "Package msg defines the
Go-native control, pairing, IPC and service messages and their Cap'n Proto encoding with decode limits (D-36)."
`codec.go`:
```go
const (
    MaxControlBytes = 16 << 20; MaxIPCBytes = 16 << 20; MaxPairingBytes = 64 << 10; MaxServiceBytes = 1 << 20
    controlTraversalWords = 8 << 20; ipcTraversalWords = 2 << 20; smallTraversalWords = 64 << 10; maxDepth = 32
)
var (ErrTooLarge, ErrMalformed, ErrUnknownMessage, ErrBounds = ...) // "msg: ..." sentinels
```
Internal helpers `unmarshal(b []byte, maxBytes int, traversalWords uint64) (*capnp.Message, error)` (length check
first, then `capnp.Unmarshal`, then the traversal and depth limit APIs recorded in `briefs/spikes/sp5.md`) and
`marshal(build func(seg *capnp.Segment) error) ([]byte, error)`. capnp errors are wrapped as ErrMalformed.
Bounds (checked while converting, before building Go slices; violation -> ErrBounds): monitors per desktop <= 32,
machines per layout <= 64, formats per announce <= 32, manifest entries per page <= 50 000, total manifest entries
<= 1 000 000, Text fields <= 4096 bytes except: TOML config <= 4 MiB, log message <= 64 KiB, diagnostics/receive
paths <= 32 KiB, MAC list <= 16, broadcast addrs <= 16, file hashes list <= 50 000 per Complete.
`common.go`: `Role` (`RoleServer=1, RoleClient`), `ErrorCode` + `ErrorInfo{Code; Message}` with `Error() string`,
conversions for Rect/Monitor/Desktop to and from `platform` types, peer ids to and from 32-byte `Data`.
`handshake.go`: `Capabilities`, `Hello`, `Welcome{Info Hello}`, `RejectReason` + `Reject`. `session.go`: `Enter`,
`LeaveReason` + `Leave`, `LayoutMachine{Peer peerid.ID; Name string; Origin platform.Point; Desktop
platform.Desktop; Online bool}`, `LayoutSnapshot{Version uint64; Machines []LayoutMachine}`, `DesktopChanged`,
`LockSyncKind` + `LockSync`, `Goodbye`, `Ping`, `Pong`, `WakeRequest`. Each type has unexported
`toCapnp`/`fromCapnp` converters; tests round-trip each type through a real capnp message (internal test package).
Field names follow Go naming of the schema fields; Data peer ids become `peerid.ID`; MACs `[]net.HardwareAddr`;
broadcast addresses `[]netip.Addr`; nonce `[16]byte`.

## Item P1-F-017
`clipboard.go`: `FormatInfo{MIME string; Size int64}`, `ClipAnnounce{Origin peerid.ID; Seq uint64; Formats
[]FormatInfo; Files bool}`, `ClipFetch`, `ClipData`, `ClipError{RequestID uint64; Err ErrorInfo}`.
`transfer.go`: `Entry{Path string; Size int64; ModTime time.Time; Mode fs.FileMode; IsDir bool}` (mode carries
permission bits only; `IsDir` is authoritative), `OfferPurpose`, `FileOffer`, `FileOfferMore`, `FileAccept`,
`FileReject`, `ChunkRequest`, `ChunkHeader`, `Progress`, `Cancel`, `Complete{TransferID string; FileHashes
[][32]byte}`, `ResumeInfo`. `dnd.go`: `DragBegin`, `DragMove`, `DragDrop`, `DragCancel`, `DragResult`.
`control.go`:
```go
type Control interface{ isControl() }    // implemented by the 29 member types (pointer-free value types)
func EncodeControl(m Control) ([]byte, error)
func DecodeControl(b []byte) (Control, error) // MaxControlBytes, controlTraversalWords; unknown union -> ErrUnknownMessage
type Kind uint8     // one constant per union member: KindHello = 1 (ordinal 0 + 1) .. KindDragResult = 29
func KindOf(m Control) Kind
func (k Kind) String() string
type Category uint8 // CatHandshake=1, CatSession, CatClipboard, CatTransfer, CatDnD
func CategoryOf(m Control) Category
```
`dataheader.go`: `type DataHeader struct { Clip *ClipData; Chunk *ChunkHeader }` (exactly one set),
`EncodeDataHeader`, `DecodeDataHeader` (MaxControlBytes, smallTraversalWords).
Fuzz targets decode arbitrary bytes and must never panic; seed corpora = one encoded instance of every member.

## Item P1-F-018
`pairing.go`: `PairHello{Name string; Peer peerid.ID; Role Role; ProtocolVersion uint16; OS string}`,
`Spake2Share{Share []byte}` (<= 128 bytes), `Spake2Confirm{MAC []byte}` (<= 64 bytes), `PairFailure` + `PairResult`,
`type Pairing interface{ isPairing() }`, `EncodePairing`/`DecodePairing` (MaxPairingBytes, smallTraversalWords).
`service.go`: `HashAlg`, `ServiceRequest{ID uint64; Body ServiceRequestBody}` with body types `ServiceHello`,
`HelperStatusRequest`, `SignTLSRequest{Digest []byte (<= 64 bytes); Hash HashAlg}`, `RequestSAS`,
`ReportInputDesktop{DesktopName string}`; `ServiceResponse{ID; Body}` with `ErrorInfo`, `ServiceHelloResult{
ServiceVersion string; MachinePeer peerid.ID; CertificateDER []byte (<= 16 KiB)}`, `HelperStatus`, `Signature
[]byte`, `OK`; `HelperRequest`/`HelperResponse` likewise. Encode/Decode functions per top-level message
(MaxServiceBytes, smallTraversalWords).

## Item P1-F-019
`ipc_types.go`: Go types for every struct and enum of `ipc.capnp` (names without the `Ipc` prefix where clear:
`Status`, `PeerInfo`, `DiscoveredPeer`, `PairingSession`, `TransferInfo`, `LogRecord`, `PermissionState`,
`UpdateInfo`, `Notification`, `HotkeyBinding`, `Hotkeys`, `ConfigDoc`, `UpdateConfig`, `SessionInfo`,
`ActiveMachine`, enums `LogLevel`, `PermissionKind`, `Theme`, `AgentState`, `DiscoverySource`, `PairingState`,
`TransferDirection`, `TransferState`, `NotificationKind`).
`ipc.go`:
```go
type IPCRequest struct { ID uint64; Body RequestBody }   // RequestBody: sealed interface; one Go type per union member
type IPCResponse struct { ID uint64; Body ResponseBody } // sealed; ErrorInfo is one of the bodies
type IPCEvent interface{ isIPCEvent() }                  // sealed; one Go type per union member
type IPCFrame struct { Request *IPCRequest; Response *IPCResponse; Event IPCEvent } // exactly one set
func EncodeIPCFrame(f IPCFrame) ([]byte, error)
func DecodeIPCFrame(b []byte) (IPCFrame, error)          // MaxIPCBytes, ipcTraversalWords
```
Request body type names: `Hello`-collision is avoided with an `IPC` prefix only where needed (`IPCHello`); others:
`GetStatus`, `GetConfig`, `UpdateConfig`, `GetLayout`, `SetLayout{Layout LayoutSnapshot}`, `ListPeers`,
`ListDiscovered`, `AddManualPeer`, `StartPairing`, `SubmitPairingCode`, `ShowPairingCode`, `CancelPairing`,
`Unpair`, `RenamePeer`, `SetRole`, `Connect`, `Disconnect`, `SendFiles`, `ListTransfers`, `CancelTransfer`,
`RetryTransfer`, `OpenReceiveFolder`, `GetLogs`, `ExportDiagnostics`, `WakePeer`, `SetLockToScreen`, `JumpTo`,
`ReturnHome`, `CheckUpdates`, `SetAutostart`, `GetPermissions`, `RequestPermission`, `GetHotkeys`, `SetHotkeys`,
`CaptureHotkey`, `Quit`. Response bodies: `ErrorInfo`, `OK`, `Status`, `ConfigDoc`, `LayoutSnapshot`, `PeerList`,
`DiscoveredList`, `PairingSession`, `TransferList`, `TransferID`, `LogList`, `PermissionList`, `Hotkeys`,
`HotkeyCaptured`, `UpdateInfo`. Event types: `StatusChanged`, `ActiveMachineChanged`, `PeersChanged`,
`DiscoveryChanged`, `PairingProgress`, `TransferProgress`, `TransferFinished`, `LogRecordEvent`, `ConfigChanged`,
`LayoutChanged`, `UpdateAvailable`, `PermissionStateChanged`, `ThemeChanged`, `FocusGUI`, `NotificationEvent`.

## Item P1-F-020
`internal/config` (package doc: "Package config defines EdgeSync's TOML configuration, its defaults and (from C5)
its store and migrations (D-38)."). Adds `github.com/pelletier/go-toml/v2 v2.4.3`.
`schema.go` (TOML keys in snake_case via `toml:"..."` tags):
```go
const SchemaVersion = 1
type Config struct {
    SchemaVersion int `toml:"schema_version"`
    Machine Machine; Identity Identity; Layout Layout; Guards Guards; Hotkeys Hotkeys
    Remap map[string]RemapTable `toml:"remap"`          // key: peer id; RemapTable map[string]string of DOM code names
    Clipboard Clipboard; Transfer Transfer; Sync Sync; WoL WoL `toml:"wol"`; Update Update; Startup Startup
    Windows Windows; Log Log; Discovery Discovery
}
type Machine struct { Name string; Role string /* "server"|"client" */; Port int }
type Identity struct { Mode string /* "user"|"machine" */ }
type Layout struct { Version uint64; Machines []LayoutMachine }
type LayoutMachine struct { Peer, Name string; X, Y int32; Monitors []Monitor }
type Monitor struct { ID string; X, Y, W, H int32; ScalePermille uint16 `toml:"scale_permille"`; Primary bool; Name string }
type Guards struct { Dwell DwellGuard; DoubleTap DoubleTapGuard `toml:"double_tap"`; Corner CornerGuard; Modifier ModifierGuard }
type DwellGuard struct { Enabled bool; DelayMS int `toml:"delay_ms"` }
type DoubleTapGuard struct { Enabled bool; WindowMS int `toml:"window_ms"`; MinMovePX int `toml:"min_move_px"` }
type CornerGuard struct { Enabled bool; SizePX int `toml:"size_px"` }
type ModifierGuard struct { Enabled bool; Keys []string }
type Hotkeys struct { LockToScreen string `toml:"lock_to_screen"`; Next, Previous, Home string; SendCAD string `toml:"send_cad"`; Jump map[string]string }
type Clipboard struct { Enabled, Text bool; RichText bool `toml:"rich_text"`; Images, Files bool }
type Transfer struct { ReceiveDir string `toml:"receive_dir"` }
type Sync struct { Lock, Screensaver, Wake bool }
type WoL struct { Auto bool }
type Update struct { Enabled bool }
type Startup struct { Autostart bool }
type Windows struct { Injection string /* "helper"|"direct" */ }
type Log struct { Level string /* "debug"|"info"|"warn"|"error" */ }
type Discovery struct { MDNS bool `toml:"mdns"`; Manual []ManualPeer }
type ManualPeer struct { Host string; Port int }
```
`defaults.go`: `func Default(hostname, goos string) Config` with: schema 1; name = hostname; role "client" (no
global capture until the user configures a server; the wizard sets the role); port 24860 (D-43); identity mode
"machine" when goos == "windows", else "user" (D-41); empty layout; guards all disabled with dwell 150, double-tap
400 / 8, corner 32, modifier keys ["ControlLeft", "ControlRight"] (D-57); hotkeys lock "ScrollLock", next
"Ctrl+Alt+Shift+ArrowRight", previous "Ctrl+Alt+Shift+ArrowLeft", home "Ctrl+Alt+Pause", send_cad "Ctrl+Alt+End" (D-113), jump empty map (defaults
for machines 1..12 are derived at runtime by C3, D-58); clipboard all true; receive dir "" (meaning the D-69
default); sync all true; wol auto true; update enabled true; autostart true; windows injection "helper"; log
"info"; discovery mdns true, manual empty. `TestDefault_MatchesDecisionsD57D58` asserts every value with a comment
referencing the decision row. `internal/config/testdata/default.toml` is the golden encoding of
`Default("pc1", "linux")`, created by this item.

## Item P1-F-021
`internal/logging/conventions.go` (frozen). Package doc: "Package logging configures log/slog for EdgeSync and
defines the attribute conventions (D-48): clipboard contents are never logged; paths and file names only at Debug;
peer ids only in short form."
```go
const (KeyComponent = "component"; KeyPeer = "peer"; KeyErr = "err"; KeyTransfer = "transfer"; KeyPath = "path";
       KeyFormat = "format"; KeySize = "size"; KeyDuration = "duration"; KeySession = "session"; KeyRequest = "request")
func Component(l *slog.Logger, name string) *slog.Logger // l.With(KeyComponent, name)
func Peer(id peerid.ID) slog.Attr                         // short form only
func Err(err error) slog.Attr                             // err.Error(), "<nil>" for nil
func Path(p string) slog.Attr                             // KeyPath; the C5 handler drops it above Debug
type Secret string                                        // LogValue() -> "[redacted]"
```
Tests use `slog.NewJSONHandler` and `slog.NewTextHandler` into buffers and assert the secret value never appears.

## Item P1-F-M01
User-owned. Push `main`; open the `ci.yml` run; confirm `gen-check` is green and copy the capnp version line the job
prints (add `capnp --version` output to the request). Record in `qa/manual/results/p1-f-gen-check.md`. If the job
fails because Ubuntu's apt `capnproto` differs from the vcpkg 1.4.0 used locally, the parent proposes a decision
(pin CI to a 1.4.0 build or regenerate on CI) before any C-stream item regenerates schemas.

## Item P1-F-022
Parent-only (Opus). CONTRACT FREEZE checklist:
1. Read `briefs/spikes/sp1.md` .. `sp5.md`, `briefs/spikes/p0-consolidation.md` and every decision appended at P0
   exit. For each interface method in `internal/platform`, confirm every backend's spike evidence fits it (Windows
   hooks + helper routing, macOS taps, X11 grabs, Wayland barriers with implicit grab and Release).
2. Frame table vs `internal/proto/wire`; schema families vs architecture 5.3 plus the recorded additions
   (`control.capnp`, `DataHeader`, `FileOfferMore`, `Hello.sessionNonce`); msg bounds vs D-57 caps.
3. IPC surface vs architecture 7; config schema vs architecture 8 plus `[discovery]`.
4. Known points raised by the P1 planner, each resolved (decision + appended item) or explicitly accepted before
   freezing: (a) `KeySwallower.SwallowKeys([]keymap.Usage)` can only swallow whole keys, so a hotkey such as
   Ctrl+Alt+Shift+F1 would block plain F1 at home; a combo-based method is needed (P1-C3-018 handles both outcomes);
   (b) `transport.Conn.ReceiveMotion` and `peerid.ID` (instead of `peers.ID`) are additions to architecture 5.2;
   (c) `internal/peerid` and `internal/proto/msg` are new frozen packages not yet listed in `ownership.md`;
   (d) `cmd/edgesync-agent/main.go` cannot exist before P2 adds `host_<goos>.go`, so an item creating it must be
   appended to P2 or P3.
5. Any needed change: append a P1-F item, run it, then freeze. Record additions to architecture with a dated note.
6. `task ci:local` green; append to this phase file and to `briefs/architecture.md`:
   `<!-- NOTE: <date> - CONTRACT FREEZE: internal/platform/*.go, internal/keymap/usage.go, internal/peerid,
   internal/proto/wire, proto/schema + internal/proto/capnp, internal/proto/msg, internal/config/schema.go +
   defaults.go, internal/logging/conventions.go. Changes need a decision + user approval. -->`

## Item P1-F-023
D-114 (Q-03 option A). `internal/platform/homeshortcuts.go`, documented as an optional interface in the manner of `ContactHolder` (capture.go):
```go
type HomeShortcut struct { ID string; Description string; Binding string } // Binding = canonical hotkey text, e.g. "ScrollLock" or "Ctrl+Alt+Pause"
type HomeShortcutState uint8                                              // HomeShortcutsUnavailable = 1, HomeShortcutsPending (waiting for the user's confirmation), HomeShortcutsBound; 0 invalid
type HomeShortcutStatus struct { State HomeShortcutState; Detail string; Triggers map[string]string } // Triggers: shortcut ID -> trigger the compositor bound
type HomeShortcutProvider interface {
    SetHomeShortcuts(ctx context.Context, shortcuts []HomeShortcut, onActivated func(id string)) error
    HomeShortcutStatus() HomeShortcutStatus
}
```
Capturers that can observe hotkeys while the pointer is home only through a compositor API (the Wayland capturer, P2-Y1-027) implement it; every other capturer does not. `SetHomeShortcuts` replaces the previous set (nil or empty clears it), returns an error wrapping `ErrUnsupported` when the mechanism is unavailable, and `onActivated` may be called from any goroutine. `String()` for the state enum like the other enums (`TestHomeShortcutState_String`).
Fake: `fake.NewHomeShortcuts(available bool) *fake.HomeShortcuts` implementing the interface with `Activate(id string)` (invokes the stored callback), `SetStatus(platform.HomeShortcutStatus)` and the recorded `SetHomeShortcuts` calls; unavailable -> `SetHomeShortcuts` returns an error wrapping `platform.ErrUnsupported` and the status state is Unavailable. Do not edit the fake capturer; engine tests wrap a fake capturer and the fake provider in a small test type.
Record the addition in `briefs/architecture.md` section 11 (already noted by the parent).
