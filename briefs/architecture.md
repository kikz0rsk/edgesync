# EdgeSync - Architecture

Status: spine draft 2026-10-29. Binding for P1-P5 item writers. Sections marked **[SPIKE-MAY-CHANGE: SPn]**
are default designs that the named P0 spike validates; any change is appended to `decisions.md` and,
if it alters user-visible behaviour, approved by the user first. The interface signatures in section 4
become frozen at the end of P1-F (CONTRACT FREEZE); the file-related interfaces in section 4.9 freeze at
the end of P4-F.

Decision references: `D-NN` in `decisions.md`. Feature references: `FEAT-NNN` in `briefs/feature-inventory.md`.

---

## 1. Processes

| Process | Binary | Runs as | cgo | Responsibilities |
|---|---|---|---|---|
| Agent | `edgesync-agent` (`cmd/edgesync-agent`) | logged-in user, one per user session | never (`CGO_ENABLED=0`) | network sessions, discovery, pairing, engine (server and client roles), capture/injection via platform backend, clipboard, transfers, DnD, tray icon, switch overlay, hotkeys, lock sync, WoL, update check, IPC server for the GUI |
| GUI | `edgesync` (`cmd/edgesync`) | logged-in user | Windows: no; macOS/Linux: yes (Gio) | Gio window: layout editor, machines/pairing, settings, transfers, logs/diagnostics, first-run wizard, native file drop onto machine targets; IPC client only |
| Service | `edgesync-service` (`cmd/edgesync-service`, Windows only) | LocalSystem, session 0 | never | session tracking, starts the secure helper, machine identity + TLS signing proxy, `SendSAS`, machine-scoped peer store for pre-login mode (`%ProgramData%\EdgeSync\machine-peers.toml`, D-112) |
| Secure helper | `edgesync-agent --secure-helper` (Windows only) | SYSTEM token duplicated from the session's winlogon.exe, `TokenUIAccess`, in the active console session | never | injection on any desktop (UIPI-free), capture on the secure desktop, pre-login client mode (D-64) |

Lifecycle (D-21, D-77): the agent autostarts at login and owns the tray. The tray's "Open EdgeSync" launches
the GUI; closing the GUI exits only the GUI. Both are single-instance.

---

## 2. Repository layout (exact paths)

```
cmd/
  edgesync/                 GUI main (Gio)                                  P3-F
  edgesync-agent/           agent main + --secure-helper mode dispatch       P1 C5 (wiring), P2 W2 (helper flag)
  edgesync-service/         Windows service main                             P2 W2
internal/
  buildinfo/                Version, Commit (ldflags)                        P1-F
  paths/                    per-OS config/log/state/staging/receive dirs     P1 C5
  platform/                 interfaces + shared types (FROZEN at P1-F)       P1-F
    fake/                   in-memory fakes of every interface               P1-F
    conformance/            contract test suites run against fake + real     P1-F
    host/                   build-tagged Open(): selects the OS backend      P1-F (host.go), P2 (host_<os>.go)
    windows/                Win32 backend                                    P2 W1, W2; P4 F1, F2
      win32/                shared syscall/LazyDLL declarations (owner-prefixed files)
      com/                  hand-built COM vtables, STA thread               P2 W2 (from SP2)
    darwin/                 purego backend                                   P2 M1; P4 F3
    x11/                    xgb backend                                      P2 X1; P4 F4
    wayland/                portals + EI backend                             P2 Y1; P4 F5
      ei/                   in-house EI wire codec
      portal/               godbus portal clients
    linux/                  shared Linux bits: SNI tray, XDG autostart, Settings-portal theme, screensaver D-Bus   P2 X1
  keymap/                   HID usage type + per-OS tables (generated)       P1-F (types), P1 C3 (tables)
    gen/                    generator from Chromium dom_code_data.inc (BSD)  P1 C3
  layout/                   canvas model, real edges, adjacency, mapping     P1 C3
  engine/                   server + client state machines, guards, hotkeys  P1 C3
  proto/
    wire/                   hot-path binary codec (zero alloc)               P1-F (format), P1 C1 (benches/fuzz)
    capnp/                  GENERATED Go from proto/schema                   P1-F
  transport/                QUIC + TCP fallback, channel map                 P1 C1
  session/                  handshake, version negotiation, keepalive, reconnect, connection manager (star)   P1 C1
  pki/                      identity certs, key storage, pin verification    P1 C2
  spake2/                   RFC 9382 SPAKE2 P-256                            P1 C2
  pairing/                  pairing protocol over TLS + exporter binding     P1 C2
  peers/                    peer store (paired peers, names, MACs)           P1 C2
  discovery/                mDNS advertise/browse + manual peers             P1 C2
  wol/                      Wake-on-LAN                                      P1 C2
  clipboard/                format model, conversions, lazy sync protocol    P1 C4
  transfer/                 transfer engine, queue, resume, receive dir      P1 C4
    safepath/               path sanitiser                                   P1 C4
  vfile/                    lazy virtual-file provider contract (FROZEN at P4-F)  P4-F
  dnd/                      cross-screen drag-and-drop protocol + state machine  P4-F
  ipc/                      agent <-> GUI IPC server + client                P1 C5
  config/                   TOML schema (P1-F), store + migrations (P1 C5)
  logging/                  slog setup, rotation, ring buffer, redaction     P1-F (conventions), P1 C5
  i18n/                     catalogue + T()                                  P1 C5
    locales/en.toml
  update/                   GitHub release check                             P1 C5
  locksync/                 lock/screensaver propagation logic               P1 C5
  diag/                     diagnostics bundle                               P1 C5
  agent/                    composition root: wires everything               P1 C5 (on fakes), P3 G5 (real)
  service/                  Windows service internals                        P2 W2
gui/
  app/ shell/ theme/ widgets/ state/        GUI foundation                   P3-F
  layouteditor/                                                              P3 G1
  peers/ wizard/                                                             P3 G2
  settings/                                                                  P3 G3
  transfers/ logs/ about/                                                    P3 G4
  drop/                     native OS file drop bound to the Gio window      P4 F2-F5
proto/schema/               *.capnp schemas                                  P1-F
third_party/                licences of permissive sources (chromium, ...)   P1 C3
tools/                      alternate module file tools.mod (tool directives) + tools.sum, covercheck (part of the main module)   P0-F
spikes/                     separate module, throwaway                       P0
packaging/windows|macos|linux/                                               P5 R1, R2
bench/                      benchmark harnesses and results                  P5 R3
docs/                       user guide, troubleshooting                      P5 R5
qa/                         checklists, manual scripts, manual results
.github/workflows/          ci.yml (P0-F, extended later), spikes.yml (P0), release.yml (P5 R2)
```

Dependency direction (enforced by depguard, D-55):
`cmd/* -> internal/agent -> {engine, session, clipboard, transfer, dnd, ipc, ...} -> {platform (interfaces), proto, layout, keymap}`.
Platform-independent packages never import `internal/platform/<os>`. Only `internal/platform/host` imports OS packages.
Only `gui/` and `cmd/edgesync` import `gioui.org`.

---

## 3. Core types (package `platform`, file `internal/platform/types.go`, FROZEN at P1-F)

```go
type Point struct{ X, Y int32 }
type Rect struct{ X, Y, W, H int32 }            // physical pixels, machine desktop coordinates

type Monitor struct {
    ID      string   // stable per OS session (Windows device name, CGDirectDisplayID, RandR output name, portal zone index)
    Bounds  Rect
    ScalePermille uint16 // 1000 = 100 %, 1500 = 150 %
    Primary bool
    Name    string
}

type Desktop struct{ Monitors []Monitor }

type Side uint8 // SideLeft, SideRight, SideTop, SideBottom

// EdgeSegment is a stretch of a real edge (monitor side not shared with another local monitor).
type EdgeSegment struct {
    Monitor string
    Side    Side
    From, To int32 // along the side, physical px, From < To
    At       int32 // coordinate of the side line (x for left/right, y for top/bottom)
    ID       uint16 // assigned by the engine; echoed back in EdgeContact
}

type Button uint8 // ButtonLeft, ButtonRight, ButtonMiddle, ButtonX1, ButtonX2

type EventKind uint8 // KindMotion, KindButton, KindWheel, KindKey, KindEdgeContact, KindCaptureLost

// InputEvent is passed by value; no interface fields (D-76).
type InputEvent struct {
    Kind   EventKind
    Time   int64       // monotonic ns from the backend clock
    Pos    Point       // KindMotion: absolute local position when not grabbed; KindEdgeContact: contact point
    DX, DY int32       // KindMotion while grabbed: relative delta (physical px of this machine); KindWheel: 1/120 notch units
    Button Button
    Down   bool
    Key    keymap.Usage
    Repeat bool
    Edge   uint16      // KindEdgeContact: EdgeSegment.ID
}
```

`keymap.Usage` (file `internal/keymap/usage.go`, FROZEN at P1-F): `type Usage uint32` = HID usage page << 16 | usage id
(keyboard page 0x07, consumer page 0x0C for media keys), plus named constants for every modifier and the keys used by
default hotkeys, and the table function signatures (implemented in P1 C3):

```go
func FromWindowsScan(scan uint16, extended bool) (Usage, bool)
func ToWindowsScan(u Usage) (scan uint16, extended bool, ok bool)
func FromMacKeycode(code uint16) (Usage, bool)
func ToMacKeycode(u Usage) (uint16, bool)
func FromEvdev(code uint16) (Usage, bool)  // X11 keycode = evdev + 8; EI uses evdev
func ToEvdev(u Usage) (uint16, bool)
type Remap map[Usage]Usage                 // per-machine modifier remap (D-08)
```

---

## 4. Platform interfaces (package `platform`, FROZEN at P1-F except 4.9)

All methods are safe for concurrent use unless stated. Every blocking method takes a `context.Context`.
Backends return `ErrUnsupported` (sentinel) only for capabilities a platform genuinely lacks; callers surface it to the GUI.
`ErrPermission` (sentinel, wraps details) signals missing OS consent (macOS TCC, Wayland portal denial).

### 4.1 Backend and host selection

```go
type Backend interface {
    Displays() Displays
    Capturer() (InputCapturer, error)
    Injector() (InputInjector, error)
    Clipboard() (ClipboardBackend, error)
    Session() SessionEvents
    Tray() (Tray, error)
    Overlay() (Overlay, error)
    Autostart() Autostart
    Permissions() Permissions
    Theme() ThemeWatcher
    Close() error
}

type Options struct {
    Logger      *slog.Logger
    SecureHelper bool   // Windows: running as --secure-helper
    Injection   string  // Windows: "helper" | "direct" (D-63)
}

// internal/platform/host
func Open(ctx context.Context, opts platform.Options) (platform.Backend, error)
// host_windows.go, host_darwin.go, host_linux.go (selects x11 or wayland from XDG_SESSION_TYPE / WAYLAND_DISPLAY; unknown -> error, no guessing)
```

### 4.2 Displays

```go
type Displays interface {
    Current(ctx context.Context) (Desktop, error)
    Watch(ctx context.Context, fn func(Desktop)) error // blocks until ctx done; fn on every topology/DPI change
}
```
Windows: `EnumDisplayMonitors` + `GetMonitorInfo` + `GetDpiForMonitor`, `WM_DISPLAYCHANGE`/`WM_DPICHANGED`. macOS: `CGGetActiveDisplayList` + `CGDisplayBounds` + pixel/point ratio, reconfiguration callback. X11: RandR monitors + notify. Wayland: InputCapture `GetZones` + `ZonesChanged` (zones become monitors; scale from zone size vs logical size where available) **[SPIKE-MAY-CHANGE: SP3]**.

### 4.3 InputCapturer (server side)

Designed so that position-polling backends (Windows hooks, macOS taps, X11 grabs) and barrier backends (Wayland InputCapture) fit one contract: the engine declares where edges are; the backend reports contacts; the engine decides whether to switch; the backend grabs.

```go
type InputCapturer interface {
    // Start begins observing. Local delivery is unaffected until Grab. Events go to sink (non-blocking sends;
    // a full sink increments a drop counter exposed via Stats and is a bug the engine sizes against).
    Start(ctx context.Context, sink chan<- InputEvent) error
    // SetEdges declares the real-edge segments that have neighbours. Position backends emit KindEdgeContact when the
    // pointer touches one; the Wayland backend installs pointer barriers (and recreates the session, mutter quirk).
    SetEdges(ctx context.Context, edges []EdgeSegment) error
    // Grab suppresses local delivery of keyboard and pointer, hides and parks the cursor; motion arrives as DX/DY.
    Grab(ctx context.Context) error
    // Ungrab restores local delivery and places the cursor at `at` (local coordinates).
    Ungrab(ctx context.Context, at Point) error
    Stats() CaptureStats // events, drops, watchdog re-installs, max callback duration
    Close() error
}
```
Key events are always delivered to sink (grabbed or not) so the engine sees hotkeys; when not grabbed the backend
passes them through locally except the ones the engine asked to swallow via `SwallowKeys`:

```go
type KeySwallower interface { SwallowKeys(set []keymap.Usage) error } // implemented by every capturer
```
**[SPIKE-MAY-CHANGE: SP1 (Raw Input vs re-centre), SP3 (Wayland: grab is implicit on barrier activation; Grab() then
confirms, Ungrab() maps to portal Release(activation_id, cursor_position)), SP4 (session vs HID tap)]**

### 4.4 InputInjector (client side)

```go
type InputInjector interface {
    MoveAbs(ctx context.Context, p Point) error
    Button(ctx context.Context, b Button, down bool) error
    Wheel(ctx context.Context, dx, dy int32) error     // 1/120 notch units
    Key(ctx context.Context, u keymap.Usage, down bool) error
    ReleaseAll(ctx context.Context) error              // releases every key/button this injector pressed
    ShowCursorAt(ctx context.Context, p Point) error   // on Enter
    Close() error
}
```
Windows "helper" injection forwards these calls over the helper pipe (D-63).

### 4.5 ClipboardBackend (lazy, non-file formats; files in 4.9)

```go
type Format string // canonical MIME: FormatText="text/plain;charset=utf-8", FormatHTML="text/html", FormatRTF="text/rtf", FormatPNG="image/png"

type FormatInfo struct {
    Format Format
    Size   int64 // -1 when unknown before rendering
}

type LocalChange struct {
    Seq       uint64       // backend-local monotonically increasing
    Formats   []FormatInfo
}

// Provider renders announced remote content when a local application pastes.
type Provider interface {
    Render(ctx context.Context, f Format, w io.Writer) error
}

type ClipboardBackend interface {
    Watch(ctx context.Context, fn func(LocalChange)) error        // local copies only; own Offer()s are not reported (echo prevention)
    Read(ctx context.Context, f Format, w io.Writer) error         // current local content in canonical form (CF_HTML unwrapped, DIB -> PNG ...)
    Offer(ctx context.Context, formats []FormatInfo, p Provider) error // take ownership; formats rendered lazily via p
    Close() error
}
```
Conversions to and from native forms (CF_HTML header, CF_DIBV5, TIFF) live in `internal/clipboard/convert` (pure Go, P1 C4) and are used by backends.

### 4.6 SessionEvents

```go
type SessionEventKind uint8 // SessLocked, SessUnlocked, SessScreensaverOn, SessScreensaverOff, SessDisplaySleep, SessDisplayWake, SessSuspend, SessResume, SessSecureDesktopOn, SessSecureDesktopOff

type SessionEvents interface {
    Watch(ctx context.Context, fn func(SessionEventKind)) error
    Lock(ctx context.Context) error
    StartScreensaver(ctx context.Context) error
    WakeDisplay(ctx context.Context) error
}
```

### 4.7 Tray, Overlay, Autostart, Permissions, Theme

```go
type TrayState uint8 // TrayIdle, TrayConnected, TrayRemoteActive, TrayError

type TrayItem struct {
    ID       string   // stable action id, e.g. "open", "lock", "jump:<peerID>", "home", "wake:<peerID>", "update", "quit"
    Label    string   // already translated by the caller via i18n
    Checked, Enabled, Separator bool
    Children []TrayItem
}

type TrayModel struct {
    State   TrayState
    Tooltip string
    Items   []TrayItem
}

type Tray interface {
    Run(ctx context.Context, m TrayModel, onAction func(id string)) error // blocks until ctx done
    Update(m TrayModel) error
}

type Overlay interface {
    Show(ctx context.Context, text string, d time.Duration) error // non-blocking display; replaces a visible overlay
}

type Autostart interface {
    Enabled() (bool, error)
    SetEnabled(on bool) error
}

type PermissionKind uint8 // PermAccessibility, PermInputMonitoring, PermPortalInputCapture, PermPortalRemoteDesktop, PermPortalClipboard
type PermissionState struct { Kind PermissionKind; Granted bool; Detail string }

type Permissions interface {
    Status(ctx context.Context) ([]PermissionState, error)
    Request(ctx context.Context, k PermissionKind) error
}

type Theme uint8 // ThemeLight, ThemeDark
type ThemeWatcher interface {
    Current(ctx context.Context) (Theme, error)
    Watch(ctx context.Context, fn func(Theme)) error
}
```
**[SPIKE-MAY-CHANGE: SP3]** GNOME Wayland offers no positionable always-on-top surface (no layer-shell); SP3 determines how the
switch indicator is shown there (candidate: Notification portal). Any outcome other than an on-screen overlay is
a user decision before P2.

### 4.8 Fakes and conformance

- `internal/platform/fake`: `fake.Backend` implementing every interface in memory, scriptable (`fake.Capturer.Emit(ev)`,
  `fake.Injector.Log()`, `fake.Clipboard.SimulateLocalCopy(...)`, `fake.Displays.Set(...)`), deterministic under `synctest`.
- `internal/platform/conformance`: `func RunCapturer(t *testing.T, newFn func(t *testing.T) platform.InputCapturer, drive Driver)`
  style suites for every interface. Fakes run them in unit tests; real backends run them under `//go:build integration`.

### 4.9 File-related interfaces (package `platform`, file `internal/platform/files.go`, FROZEN at P4-F)

Draft for P4-F (SP2/SP3/SP4 findings may change it):

```go
// in internal/vfile
type Entry struct {
    Path    string      // slash-separated relative path, sanitised (D-45)
    Size    int64
    ModTime time.Time
    Mode    fs.FileMode // dir bit for directories
}
type Manifest struct {
    ID      string     // transfer id
    Origin  string     // peer id
    Entries []Entry
}
type Provider interface {
    Open(ctx context.Context, index int, offset int64) (io.ReadCloser, error) // streams from the remote peer
}

// in internal/platform
type FileClipboard interface {
    WatchFiles(ctx context.Context, fn func(seq uint64, local []string)) error   // local file copy detected
    OfferFiles(ctx context.Context, m vfile.Manifest, p vfile.Provider) error     // lazy paste target
}
type DragDrop interface {
    WatchOutgoing(ctx context.Context, fn func(DragOut)) error                    // a local drag is leaving through an edge
    BeginIncoming(ctx context.Context, m vfile.Manifest, p vfile.Provider, at Point) (DropResult, error)
}
type DragOut struct { Local []string; At Point }
type DropResult struct { Dropped bool; Target string /* folder path when known */ }
```

---

## 5. Network

### 5.1 QUIC channel map (D-35, D-49)

| Channel | Carrier | Priority (RFC 9218 urgency, 0 = highest) | Content |
|---|---|---|---|
| Motion | QUIC datagrams | n/a | `MotionAbs` hot-path frames, latest wins |
| Input | bidi stream opened first by the server | 0 | hot-path frames: Key, Button, Wheel, MotionAbs (reliable cases), ReleaseAll, ModSync, Heartbeat |
| Control | bidi stream opened second by the server | 1 | Cap'n Proto control messages (section 5.3) |
| Clipboard data | one uni stream per fetch (opened by the data owner) | 2 | stream header (capnp `ClipData`) + raw bytes |
| File data | one uni stream per file range (max 4 per peer) | 5, incremental | stream header (capnp `ChunkHeader`) + raw chunk bytes with per-chunk SHA-256 trailer |
| Pairing | separate short-lived QUIC connection, ALPN `edgesync-pair/1` | - | Cap'n Proto pairing messages |

ALPN for sessions: `edgesync/1`. Datagram support is negotiated (`EnableDatagrams`); if the peer lacks it, motion uses the input stream.

### 5.2 TCP fallback mapping

Automatic: the dialer tries QUIC first; if no QUIC handshake completes within 1.5 s (UDP blocked) it dials TCP on the
same port. Connection A (TLS 1.3, `TCP_NODELAY`): a tiny framing layer `[1 byte channel][3 bytes length][payload]`
multiplexing Motion, Input and Control with a strict-priority writer (Motion and Input always before Control).
Connections B..n (TLS 1.3, same identity and pinning): one per clipboard fetch or file range, at most 4 concurrent,
first frame identifies the session and request. The transport exposes the same Go API either way:

```go
// internal/transport
type Conn interface {
    Peer() peers.ID
    SendMotion(f wire.Frame) error                         // datagram or input stream
    Input() FrameStream                                     // ordered reliable hot-path frames
    Control() MessageStream                                 // capnp messages
    OpenData(ctx context.Context, class DataClass) (io.WriteCloser, error) // DataClipboard | DataFile
    AcceptData(ctx context.Context) (DataClass, io.ReadCloser, error)
    Kind() Kind                                             // KindQUIC | KindTCP
    Close(reason error) error
}
```

### 5.3 Cap'n Proto schema families (`proto/schema/`, generated into `internal/proto/capnp/`)

| File | Messages |
|---|---|
| `common.capnp` | PeerId, Rect, Monitor, Desktop, Side, ErrorInfo |
| `handshake.capnp` | Hello (protocol version, build version, role, name, capabilities, MACs, broadcast addrs, desktop), Welcome, Reject (reason enum incl. Busy, VersionMismatch, NotPaired) |
| `session.capnp` | Enter (x, y, modifier state, clipboard seq), Leave, LayoutSnapshot, DesktopChanged, LockSync (kind), Goodbye, Ping/Pong (RTT), WakeRequest |
| `clipboard.capnp` | ClipAnnounce (origin, seq, formats+sizes), ClipFetch (req id, origin, seq, format), ClipData (stream header), ClipError |
| `transfer.capnp` | FileOffer (manifest), FileAccept/FileReject, ChunkRequest (file index, offset, length), ChunkHeader, Progress, Cancel, Complete (file hashes), ResumeInfo |
| `dnd.capnp` | DragBegin (manifest, position), DragMove, DragDrop (target), DragCancel |
| `pairing.capnp` | PairHello (name, peer id, role), Spake2Share, Spake2Confirm, PairResult (accepted, reason) |
| `ipc.capnp` | IpcRequest (union of requests, request id), IpcResponse (request id, union of results or ErrorInfo), IpcEvent (union) |
| `service.capnp` | ServiceRequest/ServiceResponse (whitelist D-47), HelperRequest/HelperResponse |

Decoding limits: ReadLimiter traversal limit 8 Mi words for control, 2 Mi words for IPC; nesting depth 32; messages larger than
the per-channel cap (control 16 MiB, IPC 16 MiB) are rejected before decoding.

### 5.4 Hot-path wire format (`internal/proto/wire`, FROZEN at P1-F)

Little-endian, fixed layouts, `[1 byte type][1 byte flags][payload]`; no length field (sizes are implied by type; unknown types
on the reliable stream close the session with a protocol error; on datagrams they are dropped and counted).

| Type | Payload | Size |
|---|---|---|
| 0x01 MotionAbs | seq u32, x i32, y i32, tsDelta u32 (server µs since session start, for RTT/latency stats) | 18 |
| 0x02 Button | button u8, down u8 | 4 |
| 0x03 Wheel | dx i32, dy i32 | 10 |
| 0x04 Key | usage u32, down u8, repeat u8 | 8 |
| 0x05 ReleaseAll | - | 2 |
| 0x06 ModSync | bitmask u16 (LCtrl, RCtrl, LShift, RShift, LAlt, RAlt, LMeta, RMeta, CapsLock, NumLock, ScrollLock) | 4 |
| 0x07 Heartbeat | seq u32, tsDelta u32 | 10 |
| 0x08 HeartbeatAck | seq u32 | 6 |

API: `func AppendFrame(dst []byte, f Frame) []byte`, `func DecodeFrame(src []byte) (Frame, int, error)` where `Frame` is a fixed struct; 0 allocs/op (benchmarked).

### 5.5 Session lifecycle (`internal/session`)

Dial (server -> client, D-59) -> TLS 1.3 + pin check (D-42) -> Hello/Welcome (version, role, capabilities, desktop) -> client sends
DesktopChanged on change -> server sends LayoutSnapshot -> steady state with Heartbeat every 1 s -> loss after 3 s silence ->
server returns control home if the lost peer was active -> reconnect with backoff (D-57).

---

## 6. Sequence flows

### 6.1 Edge switch (server S, client C)
1. S engine computes S's real edges with neighbours from the layout and calls `Capturer.SetEdges`.
2. Backend emits `KindEdgeContact{Edge}` (Windows/macOS/X11: from positions; Wayland: portal `Activated`).
3. Engine evaluates guards (dwell, double-tap, corner, modifier); hotkey-driven jumps skip guards.
4. Switch: `Capturer.Grab`; engine maps contact point to C's entry point (D-51); sends `ReleaseAll`+`ModSync` as needed; Control `Enter{x,y,mods,clipSeq}`; Input `MotionAbs` reliable.
5. C: `Injector.ShowCursorAt`, then `Overlay.Show(name of C)` on C (the overlay always appears on the machine receiving control).
6. Motion: S converts DX/DY to C coordinates (scale ratio), coalesces per 1 ms, sends datagrams.
7. When the virtual cursor crosses one of C's real edges with a neighbour N (or back to S), the engine repeats from 3 for N; leaving C sends `Leave` + `ReleaseAll`; returning to S calls `Ungrab(at)`.
8. Panic hotkey: immediate return to S, `ReleaseAll` to every client.

### 6.2 Lazy clipboard paste (copy on A, paste on B; S relays when A and B are clients)
1. A backend `Watch` fires `LocalChange{seq, formats}` (concealed content is shared like any other, D-121). A engine sends `ClipAnnounce{origin A, seq, formats}` to S; S forwards to all others and offers it locally too.
2. B's clipboard module calls `ClipboardBackend.Offer(formats, provider)`; B's OS now advertises the formats lazily.
3. User pastes on B; OS asks for one format -> provider `Render(f)` -> `ClipFetch` to origin (via S) -> origin `Read(f)` streams on a clipboard data stream -> bytes flow to B's OS. Timeout D-57; on failure the paste fails and the GUI/log shows the reason (never empty data).
4. A newer announcement supersedes older ones (by origin + seq).

### 6.3 Lazy file paste (P4)
1. A `FileClipboard.WatchFiles` fires; A builds a manifest (walks folders, sizes, mtimes; hashes computed while streaming) and sends `ClipAnnounce` with the files format + `FileOffer` manifest.
2. B `FileClipboard.OfferFiles(manifest, provider)`: Windows places an `IDataObject` with FILEDESCRIPTORW/FILECONTENTS; macOS file promises; Linux per SP3.
3. Paste on B: the OS asks for contents -> provider `Open(index, offset)` -> `ChunkRequest` -> chunks stream from A via S with per-chunk hashes; resume uses offsets.
4. Transfer appears in both agents' queues (progress, cancel).

### 6.4 Cross-screen drag-and-drop (P4) **[SPIKE-MAY-CHANGE: SP2, SP3, SP4]**
1. Server S, button held while dragging files in a local file manager; pointer touches an edge with neighbour C. `DragDrop.WatchOutgoing` detects the drag (Windows: transparent topmost drop-target window along the edge receives `DragEnter` and reads the IDataObject).
2. Engine switches to C as in 6.1 while the button remains logically held on C's side; S sends `DragBegin{manifest, pos}`.
3. C `DragDrop.BeginIncoming(manifest, provider, at)` starts a real OS drag on C with lazily provided files; motion continues; release on C drops; files stream as in 6.3. Escape or return to S sends `DragCancel`.

### 6.5 Pairing (D-14)
1. Initiator I opens a pairing QUIC connection (ALPN `edgesync-pair/1`) to responder R; both present their self-signed certs; no pin check in pairing mode.
2. R generates a 6-digit code, shows it in its GUI/tray notification; the user types it on I.
3. Both derive `ctx = ExportKeyingMaterial("EXPORTER-EdgeSync-pairing-v1", nil, 32)`; SPAKE2 per RFC 9382 with password = code, idA = I's peer ID, idB = R's peer ID, AAD = ctx; key confirmation MACs exchanged.
4. On success both store the other's SPKI pin + name + MACs in the peer store; on failure the attempt counter increments; after 3 failures or 120 s the code is invalid.

### 6.6 Windows service and secure helper (D-63, D-64) **[SPIKE-MAY-CHANGE: SP1]**
1. Service starts at boot; reads machine config; owns the machine identity.
2. On console session connect/logon (and at boot): duplicate `winlogon.exe` token of the active console session, set `TokenUIAccess`, `CreateProcessAsUser(edgesync-agent --secure-helper)` into that session; restart on exit with backoff.
3. No user logged on: helper runs pre-login client mode (accepts only paired servers listed in `machine-peers.toml`, injects only; the list is changed by the elevated `edgesync-service allow-prelogin`, D-112).
4. User logged on: the user agent connects to the helper pipe (SYSTEM-only DACL on the helper side; helper verifies the agent via the service) and routes all injection through it; capture on the secure desktop is provided by the helper.
5. `RequestSAS` from the agent -> service calls `SendSAS(FALSE)`.
6. User agent TLS handshakes call `SignTLS` on the service pipe (crypto.Signer proxy).

---

## 7. IPC API surface (GUI <-> agent, `proto/schema/ipc.capnp`)

Requests (each returns a typed result or `ErrorInfo`):
`Hello(token, gui version)`, `GetStatus`, `GetConfig`, `UpdateConfig(patch)`, `GetLayout`, `SetLayout(layout)`, `ListPeers`,
`ListDiscovered`, `AddManualPeer(host, port)`, `StartPairing(target)` (initiator side), `SubmitPairingCode(session, code)`,
`ShowPairingCode` (responder side opens a code), `CancelPairing`, `Unpair(peer)`, `RenamePeer(peer, name)`, `SetRole(role)`,
`Connect(peer)`, `Disconnect(peer)`, `SendFiles(peer, paths)`, `ListTransfers`, `CancelTransfer(id)`, `RetryTransfer(id)`,
`OpenReceiveFolder`, `GetLogs(since, minLevel)`, `ExportDiagnostics(path)`, `WakePeer(peer)`, `SetLockToScreen(on)`,
`JumpTo(peer)`, `ReturnHome`, `CheckUpdates`, `SetAutostart(on)`, `GetPermissions`, `RequestPermission(kind)`,
`GetHotkeys`, `SetHotkeys(bindings)`, `CaptureHotkey` (next key combo is reported, not acted on), `Quit`.

Events (server-push): `StatusChanged`, `ActiveMachineChanged`, `PeersChanged`, `DiscoveryChanged`, `PairingProgress`,
`TransferProgress`, `TransferFinished`, `LogRecord`, `ConfigChanged`, `LayoutChanged`, `UpdateAvailable`,
`PermissionStateChanged`, `ThemeChanged`, `FocusGUI`, `Notification`.

---

## 8. Config schema (TOML, `internal/config/schema.go`, FROZEN at P1-F)

Top-level tables: `schema_version`, `[machine]` (name, role, port), `[identity]` (mode), `[layout]` (machines with origin + desktop
snapshot + per-machine settings), `[guards]` (each guard enabled + parameter), `[hotkeys]`, `[remap.<peer>]`, `[clipboard]`
(master + per-format toggles), `[transfer]` (receive_dir), `[sync]` (lock, screensaver, wake toggles), `[wol]` (auto),
`[update]` (enabled), `[startup]` (autostart), `[windows]` (injection), `[log]` (level). Defaults from D-57/D-58.
Peers live in a separate file `peers.toml` (owned by `internal/peers`, same atomic-write rules).

---

## 9. What each spike may change

| Spike | Default design under test | Possible outcomes needing a new decision |
|---|---|---|
| SP1 | D-50 hooks + Raw Input + watchdog; D-63 service/helper/token; D-41 signing proxy | re-centre instead of Raw Input; helper on demand instead of always-on; signing proxy replaced |
| SP2 | COM vtables in pure Go; virtual files; delayed rendering; OLE DnD hack | virtual files infeasible (STOP + ask); DnD limited (STOP + ask) |
| SP3 | EI codec; portals; GNOME clipboard in session; Linux lazy file paste; DnD; overlay on GNOME | XI2 generation needed; Linux file paste mechanism; Wayland DnD/overlay limits (STOP + ask) |
| SP4 | purego taps, CGPoint by value, NSPasteboardItemDataProvider, file promises, NSStatusItem | Finder ignores promises (STOP + ask); purego insufficient (STOP + ask, cgo would violate D-34) |
| SP5 | Gio editor feasibility; quic-go latency; hashicorp/mdns; capnp toolchain; SPAKE2 vectors | switch mDNS library (decision); capnp unsuitable (STOP + ask, D-36 fallback CBOR needs user) |

## 10. Planning consolidation notes (2026-10-30)

These notes amend the sections above; the cited decisions are authoritative.
- Section 3 / 4.2 (D-89): monitors, edges, contacts, deltas and injection targets use each backend's native,
  contiguous coordinate space: physical pixels on Windows (Per-Monitor-v2) and X11, global points on macOS, compositor
  logical pixels on Wayland. `ScalePermille` is native units per canvas unit (1000 on macOS and Wayland). Read
  "physical pixels" above as "native units".
- Section 3 (D-87, D-88): `KindWheel` and `Wheel` use 1/120 notch units, `DY > 0` = away from the user, `DX > 0` =
  right. `EdgeSegment.At` is the boundary line of a half-open rect; Left/Top contact at `At`, Right/Bottom contact at
  `At - 1`; `EdgeSegment.Touches(p)` implements the rule.
- Section 4.3 (D-90): optional `ContactHolder { HoldsContact() bool }` in `capture.go`; the engine answers every edge
  contact with `Grab` or `Ungrab(contact point)`, within 400 ms for holders; holder backends self-release after 500 ms.
- Section 4.1 / 1 (D-84): `host.RunMain(run func() int) int` (`runmain_other.go`, `runmain_darwin.go`);
  `cmd/edgesync-agent/main.go` (P2-X-003) dispatches `--secure-helper` to `helper.Main` before `agent.Main`; the
  normal agent always opens its backend with `SecureHelper = false`.
- Section 4.9 (D-101): the authoritative draft of the file contract is "The file contract" in
  `briefs/streams/p4-f.md` (adds `IncomingDrag`, `SetEdges`, `Arm`, `FinishOutgoing`, optional source interfaces,
  `FormatFiles`); staging lives in `internal/transfer`. P4-F-023 freezes it.
- Section 6.6 (D-81, D-82, D-83): the service brokers the SYSTEM-only helper pipe to the authorised agent with
  `DuplicateHandle`; `ReportInputDesktop{accessible}` switches secure-desktop event forwarding for the panic hotkey and
  release-all only; `machine.toml` schema version 1.
- Section 7 (D-93): IPC additions `GetEnvironmentChecks`/`EnvironmentChanged`, `RemoveManualPeer`, `primaryGUI` in the
  `Hello` result, `RequestFocusGUI(route)` -> `FocusGUI{route}`, `ThemeChanged` right after GUI `Hello`, the
  `PairingProgress` variants, `heldBy`/`rejectedServer` in status; section 8 gains the `[gui]` table.
- X11 (D-92): observation without a grab uses RECORD on a second connection; no XInput2 bindings.

## 11. Product answer notes (2026-10-30, D-112 .. D-121)

These notes amend the sections above; the cited decisions are authoritative.
- Section 3 / 4.4 (D-121): `LocalChange` has no `Sensitive` field; no backend detects concealed content and the clipboard manager has
  no sensitivity filter (the clipboard stays CLIPBOARD-only, per-format toggles stay, contents are never logged).
- Section 4.3 (D-114): optional `platform.HomeShortcutProvider` (`internal/platform/homeshortcuts.go`, P1-F-023), implemented by the
  Wayland capturer: `SetHomeShortcuts(ctx, []HomeShortcut, onActivated func(id string)) error` and `HomeShortcutStatus()`; the server
  engine registers only the lock and panic bindings and runs the same actions on activation (P1-C3-027).
- Section 5.3 / 6.6 (D-112): `service.capnp` gains the read-only request `GetPreloginPeers` (P2-W2-038, additive ordinal); the D-47
  whitelist is otherwise unchanged. `machine-peers.toml` is written only by the elevated `edgesync-service allow-prelogin` command.
- Section 7 (D-112, D-113, D-114, D-115): IPC additions `GetPreloginAccess`, `SendCtrlAltDel(peer)` and the environment-check kinds
  `homeHotkeys` and `lockScreen` (P3-G5-020, same additive mechanism as D-93).
- Section 8 (D-113): `[hotkeys]` gains `send_cad` (default `Ctrl+Alt+End`, P1-F-020); the engine action `SendCAD` sends the chord to a
  Windows client that reports `os = "windows"` and `capabilities.secureDesktop` (P1-C3-026).
- Section 1 / 6.6 (D-118): the Windows installer is Inno Setup 7.1.1 (`packaging/windows/edgesync.iss`, built by `packaging/windows/isbuild`);
  it installs and removes the service through `edgesync-service install|uninstall`, the firewall rules through
  `edgesync-service firewall add|remove`, and writes the HKLM Run autostart value (D-116) and the `SoftwareSASGeneration` policy.
