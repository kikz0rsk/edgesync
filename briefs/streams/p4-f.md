# Stream brief - P4-F Foundation block (file contract, transfer hooks, clipboard file sync, DnD protocol)

Phase 4. Sequential, one agent at a time, in item order. Items: `briefs/plan-phase-4.md` section "P4-F".
The block ends with the FILE CONTRACT FREEZE (P4-F-023), after which F1-F5 fan out.

## Scope
Build everything platform-independent that native file paste and cross-screen drag-and-drop need:
- the `internal/vfile` contract (manifest, validation, builder, URI codecs, read-ahead);
- the platform file interfaces in `internal/platform/files.go`, with fakes and conformance suites;
- the transfer-engine hooks: staging, source-side offers, `RemoteProvider`;
- clipboard file sync, the dnd protocol and coordinator, and the engine hooks;
- agent wiring, end-to-end tests on fakes, loopback benches, and the GUI drop contract.

No OS-specific code except tiny `_unix.go`/`_windows.go` free-space helpers.

## Owned paths
- `internal/vfile/**`, `internal/dnd/**`, `internal/platform/files.go`, `internal/platform/files_test.go`.
- `gui/drop/drop.go`, `gui/drop/area.go`, `gui/drop/router.go` and their tests. Deviation from `ownership.md`: P4-F creates the shared drop contract so that no F stream has to run first.
- Files explicitly listed in an item's `Files:` line inside:
  - `internal/transfer/` (claim)
  - `internal/clipboard/`
  - `internal/engine/`
  - `internal/agent/` (claim)
  - `internal/platform/fake/`
  - `internal/platform/conformance/`
  - `test/e2e/`
  - `internal/i18n/locales/en/files.toml` (D-94; P4-F owns it)
  - `proto/schema/dnd.capnp` (claim, only with an approved delta)

## Leave alone
Plan files. All OS backends. `internal/platform/*.go` other than `files.go` (frozen at P1-F). `internal/proto/wire/**`. GUI pages (wired in P4-F2-012).

## Conventions (binding)
- Decisions:
  - Files and safety: D-11, D-12, D-13, D-17, D-44, D-45, D-61, D-62.
  - Topology and sessions: D-49, D-57, D-59.
  - Code and contracts: D-67, D-73, D-75, D-76.
  - Anything appended by P4-F-002.
- Go rules from `.claude/agents/go-dev.md`:
  - Context-first APIs, `%w` wrapping, sentinel errors per package.
  - No panics, no silent fallbacks, goroutines owned and stoppable.
  - `testing/synctest` for anything timer-driven, go-cmp for diffs.
- Engine hot path stays allocation-free (D-76, PB-05).
- Import direction:
  - `internal/vfile` imports only the stdlib and `internal/transfer/safepath`.
  - `internal/platform` may import `internal/vfile`.
  - `internal/transfer`, `internal/clipboard` and `internal/dnd` import `vfile` and `platform`.
  - `internal/engine` never imports `dnd`: the engine defines a hook interface, `dnd.Coordinator` implements it, and `internal/agent` wires them.
- Logging: file names and paths only at Debug, never clipboard contents (D-48). Transfer stats are logged without names.
- User-facing strings only via `internal/i18n` (D-68).
- No em dashes or en dashes. Commit `<type>(<scope>): <description> [FEAT-NNN]`, body `Item: <id>`, no attribution trailers.
- Never launch EdgeSync binaries. Tests touching the real clipboard or OS shell carry `//go:build integration` (P4-F has none).

## Research and dependencies
- `briefs/architecture.md` sections 4.9 (draft file interfaces, finalised below), 5.1 (file data streams: urgency 5 incremental, max 4 per peer), 5.3 (`transfer.capnp`, `dnd.capnp`), 6.3, 6.4.
- `briefs/research-platform-internals.md` section 6 (approaches a-d), section 5 (formats incl. `text/uri-list`, `x-special/gnome-copied-files`).
- Spike findings `briefs/spikes/sp2.md`, `sp3.md`, `sp4.md` and `briefs/spikes/p0-consolidation.md`.
- Risks R-24 .. R-30, R-42.
- P1 C4 delivered `internal/transfer` (engine, chunk requests with per-chunk SHA-256, queue, resume, os.Root receive writer, relay) and `internal/clipboard` (lazy announce/fetch, origin + seq). Read their exported APIs first. If an API this brief needs does not exist (for example a receive writer usable with a caller-chosen root), report BLOCKED: do not reimplement P1 C4 functionality.

## Report format (reply with exactly this, nothing else)
```
FILES: <path> - <one clause>          (one line per file, or NONE)
GATES: <gate> pass|fail               (verbatim output only for a failure)
ITEMS: <item id> <FEAT-NNN, ...>
TESTS: <what the test phase must cover for this item, one line>
BLOCKED: <what stopped you, or NONE>
```

---

## The file contract (frozen by P4-F-023)

```go
// internal/vfile/vfile.go
package vfile

type Entry struct {
    Path    string      // slash-separated relative path, validated (D-45); top-level entries contain no "/"
    Size    int64       // 0 for directories
    ModTime time.Time
    Mode    fs.FileMode // fs.ModeDir for directories; permission bits applied on Unix targets only
}

type Manifest struct {
    ID      string  // 32 lowercase hex characters (128-bit crypto/rand)
    Origin  string  // peer ID (D-42) of the machine that owns the files
    Entries []Entry // every parent directory appears before its children
}

type Limits struct {
    MaxEntries   int // value from the decision recorded at P4-F-002 (proposal: 100000)
    MaxPathBytes int // value from the decision recorded at P4-F-002 (proposal: 4096)
}

type Outcome uint8

const (
    OutcomeCompleted Outcome = iota + 1 // the local consumer took every byte it asked for
    OutcomeCancelled                    // the user cancelled (OS progress UI, queue cancel, Escape)
    OutcomeFailed                       // an error ended the consumer
    OutcomeAbandoned                    // the offer was replaced or released without any outcome
)

// Provider is implemented by transfer.RemoteProvider; OS backends consume it.
type Provider interface {
    // Open streams entry `index` from `offset`; the reader yields exactly Size-offset bytes or returns an error.
    Open(ctx context.Context, index int, offset int64) (io.ReadCloser, error)
    // Done reports how the local consumer finished with this manifest. Only the first call counts.
    Done(o Outcome)
}

var (
    ErrInvalidManifest  = errors.New("vfile: invalid manifest")
    ErrIndexOutOfRange  = errors.New("vfile: entry index out of range")
    ErrNotRepresentable = errors.New("vfile: manifest not representable on this platform")
    ErrSourceChanged    = errors.New("vfile: source file changed since it was announced")
    ErrOfferRetired     = errors.New("vfile: offer was superseded")
    ErrPeerOffline      = errors.New("vfile: origin peer is not connected")
)

func (m Manifest) Validate(l Limits) error
func (m Manifest) TotalBytes() int64
func (m Manifest) TopLevel() []int

// Local is the source-side view produced by Build.
type Local struct {
    Manifest Manifest
    Paths    []string  // absolute local path per entry, same order as Manifest.Entries
    Skipped  []Skipped // symlinks and special files that were not included (D-45)
}
type SkipReason uint8 // SkipSymlink, SkipSpecial
type Skipped struct { Path string; Reason SkipReason }

func Build(ctx context.Context, origin string, roots []string, l Limits) (Local, error)

// URI codecs (uri.go)
func EncodeURIList(paths []string) []byte
func EncodeGnomeCopiedFiles(paths []string) []byte
func DecodeURIList(b []byte) ([]string, error)
func DecodeGnomeCopiedFiles(b []byte) ([]string, error)

// Read-ahead (readahead.go)
type ReadAhead struct{ /* unexported */ }
func NewReadAhead(ctx context.Context, p Provider, index int, size int64, window int) *ReadAhead
func (r *ReadAhead) Read(b []byte) (int, error)
func (r *ReadAhead) Seek(offset int64, whence int) (int64, error)
func (r *ReadAhead) Close() error
```

```go
// internal/platform/files.go
package platform

const FormatFiles Format = "application/x-edgesync-files"

type FileClipboard interface {
    // WatchFiles reports local copies whose content is a list of files/folders. Own OfferFiles are never reported.
    // seq is the backend's change counter (Windows: GetClipboardSequenceNumber, macOS: changeCount, X11/Wayland: local counter).
    WatchFiles(ctx context.Context, fn func(seq uint64, paths []string)) error
    // OfferFiles takes clipboard ownership with a lazily provided manifest. It replaces the previous offer, whose
    // provider then receives Done(OutcomeAbandoned) unless it already had an outcome. Returns
    // vfile.ErrNotRepresentable (wrapped) when the platform cannot express the manifest.
    OfferFiles(ctx context.Context, m vfile.Manifest, p vfile.Provider) error
    Close() error
}

type FileClipboardSource interface{ FileClipboard() (FileClipboard, error) }

type DragOutKind uint8

const (
    DragOutEnter       DragOutKind = iota + 1 // a local drag carrying files reached an armed edge strip
    DragOutLeave                              // it left the strip (cursor moved back into the desktop)
    DragOutDrop                               // the user released over the strip without a switch
    DragOutUnsupported                        // a local drag reached the strip but carries no file list
)

type DragOutEvent struct {
    Kind  DragOutKind
    Paths []string // DragOutEnter only
    At    Point    // local physical pixels
    Edge  uint16   // EdgeSegment.ID of the strip
}

type DropResult struct {
    Dropped bool
    Target  string // destination folder when the platform knows it, else ""
}

type IncomingDrag interface {
    Release() error                                // drop at the current pointer position
    Cancel() error                                 // abort the OS drag (Escape semantics)
    Wait(ctx context.Context) (DropResult, error)  // returns once the OS drag ended
}

type DragDrop interface {
    SetEdges(ctx context.Context, edges []EdgeSegment) error // edge strips for outgoing detection
    Arm(ctx context.Context, armed bool) error                // strips hit-testable only while the left button is held
    WatchOutgoing(ctx context.Context, fn func(DragOutEvent)) error
    // FinishOutgoing ends the local OS drag that produced DragOutEnter: accepted = the remote drop happened.
    // Implementations never request a move operation (D-62).
    FinishOutgoing(ctx context.Context, accepted bool) error
    BeginIncoming(ctx context.Context, m vfile.Manifest, p vfile.Provider, at Point) (IncomingDrag, error)
    Close() error
}

type DragDropSource interface{ DragDrop() (DragDrop, error) }

func (k DragOutKind) String() string
```

The backend exposes files through the optional `FileClipboardSource` / `DragDropSource` interfaces (type
assertion in the agent), so the frozen `Backend` interface from P1-F does not change. A backend that does not
implement them, or returns `platform.ErrUnsupported`, makes the agent report the feature as unsupported once
(Notification + status flag). It never fails silently.

```go
// gui/drop/drop.go
package drop

type Drop struct {
    Paths []string
    Pos   image.Point // window pixels
}

type Target interface {
    // Attach registers the native drop target for the window described by ev. fn may be called on a native
    // thread; callers forward to the Gio loop (channel + window.Invalidate).
    Attach(ev app.ViewEvent, fn func(Drop)) error
    Detach() error
}

var (
    ErrUnsupportedView = errors.New("drop: unsupported view type")
    ErrNoTarget        = errors.New("drop: no machine under the drop point")
)

// Per-OS files provide: func NewTarget() Target  (drop_windows.go, drop_darwin.go, drop_linux.go)
```

---

## Flow summaries (binding for P4-F-011 .. P4-F-018)

**Lazy file paste** (architecture 6.3, D-59 relay):
1. Origin A: `FileClipboard.WatchFiles(seq, paths)` -> `vfile.Build(origin=A, paths, limits)` -> `transfer.Offers.Register(local)` -> clipboard sends `ClipAnnounce{origin A, seq, formats [{FormatFiles, TotalBytes}]}` plus `FileOffer{manifest}` (existing `transfer.capnp` message). A `ClipboardBackend.Watch` change carrying the same seq is ignored: a file copy announces files only.
2. Receiver B: `FileClipboard.OfferFiles(manifest, transfer.NewRemoteProvider(...))`.
3. Paste on B: the OS consumer calls `Provider.Open(index, offset)` -> `ChunkRequest` to A (relayed by S) -> chunks verified per chunk -> bytes to the OS. The queue shows one entry per manifest paste.
4. A newer announcement from anyone supersedes. The origin calls `Offers.Retire(id)`: in-flight streams finish, new opens get `ErrOfferRetired`. Receivers replace their OS offer.
5. Cut (D-62): no move semantics exist anywhere in the manifest or protocol. Sources are never deleted. Backends always advertise copy.

**Cross-machine drag** (architecture 6.4). Roles: origin O (machine whose OS runs the local drag), target T (machine where the virtual cursor is), server S (relays every message, owns the session id).
1. Left button down while the virtual cursor is on machine M: M's coordinator calls `DragDrop.Arm(true)`. On S this comes from captured button events. On a client it comes from the injected Button frames that its engine routes through the hooks. Button up gives `Arm(false)` unless a drag session is active.
2. `DragOutEnter{paths}` on O: O builds the manifest, registers the offer and sends `DragBegin{session, origin O, manifest, pos}` to S (local call when O = S).
3. When S switches the virtual cursor from O to T with the left button still held, S forwards `DragBegin` to T. If S learns of the drag after the switch already happened, it forwards it immediately as long as the button is still held. Otherwise it sends `DragCancel` to O. ReleaseAll to O excludes the left button while O is the origin of an active session (engine hook).
4. T: `BeginIncoming(manifest, RemoteProvider(origin O), at = current pointer)`. T's engine routes left Button-up to `IncomingDrag.Release()` and Escape to `Cancel()` instead of the injector. `Wait` returns, then T sends `DragDrop{session, dropped, target}` to S, which relays it to O. O calls `FinishOutgoing(accepted=dropped)`. The offer stays registered until the resulting transfer ends (queue completion or cancel), then it is retired.
5. The cursor crosses from T to U (not O): S sends `DragCancel` to T, then `DragBegin` to U.
6. The cursor returns to O: S sends `DragCancel` to T. O's local drag continues natively, and the strip produces `DragOutLeave`/`DragOutEnter` as usual.
7. `DragOutDrop` on O (released on the strip without a switch): `FinishOutgoing(false)`, session ends.
8. Peer loss, panic hotkey, hotkey jump, `BeginIncoming` error, or no `Wait` result within the handshake timeout (D-57, 5 s) after `DragBegin`: the session is cancelled on both sides. `FinishOutgoing(false)` runs exactly once.
9. `DragMove` from `dnd.capnp` is not used: pointer motion already flows on the input channel. P4-F-001 records whether the schema keeps it as reserved.

---

## Item P4-F-001
Parent pre-flight (Opus). Write `briefs/p4-preflight.md` with these sections:
1. **Spike verdicts.** Every sp2/sp3/sp4 row named in the item, with verdict, evidence level and consequence (which P4 items proceed, change or need a user decision). Include the P0-X-002 outcomes that touch D-11/D-12.
2. **Contract deltas.** Compare architecture 4.9 (draft) with the contract above. The draft-to-final changes (IncomingDrag handle, SetEdges/Arm/FinishOutgoing, source interfaces, FormatFiles, staging in `internal/transfer`) are within P4-F's remit. List anything the spikes require beyond them.
3. **Schema deltas.** Check `proto/schema/transfer.capnp` and `dnd.capnp` as delivered by P1-F:
   - ChunkRequest carries the manifest/transfer id.
   - DragBegin carries session id + origin + manifest + position.
   - DragDrop carries session + dropped + target + error.
   - DragCancel carries session + reason.

   Every missing field is a proposal (schema change after P1-F needs a decision).
4. **Decision proposals for P4-F-002.** Closed at the planning consolidation (2026-10-30): manifest limits, virtual-file names and read-ahead (D-99), the gui/drop depguard and cgo allowances (D-100), PB-25 (D-109) and the macOS pasteboard question (D-86, with the pre-authorised additive `manifestDeferred` field if the schema needs it). The list below is kept for reference; propose only what the spike verdicts or the merged code add:
   - manifest limits (proposal 100000 entries, 4096-byte relative path);
   - Windows virtual-file names (reject `<>:"|?*`, control characters and paths longer than 259 UTF-16 units with `ErrNotRepresentable` plus notification `files.notRepresentable`);
   - read-ahead window per open stream (proposal 8 MiB);
   - PB-25 time-to-first-byte budget (proposal loopback p95 <= 50 ms, LAN-W p95 <= 150 ms);
   - depguard allowance for `gui/drop/*_windows.go` to import `internal/platform/windows/com` and `internal/platform/windows/win32`, and for `gui/drop/*_x11.go` to import `github.com/jezek/xgb`;
   - cgo Objective-C in `gui/drop/drop_darwin.m` and cgo libwayland in `gui/drop/drop_wayland_c.go` (within D-34's "GUI may use cgo on macOS/Linux", confirm);
   - the macOS pasteboard-alert question (R-15): read file URLs at copy time, or defer the manifest until first fetch, which would need a schema delta.
5. **Path notes.** The planning consolidation wrote the exact paths into P4-F-016 (`internal/engine/server.go`, `internal/engine/client.go`), P4-F2-012 (`gui/peers/rows.go`, `gui/layouteditor/canvas.go`, `gui/shell/shell.go`) and P4-F4-007 (`internal/platform/x11/record_linux.go`, `internal/platform/x11/capturer_linux.go`); confirm them against `main` and append a NOTE for:
   - engine switch and client dispatch files (P4-F-016);
   - W1/M1/X1/Y1 backend type names (F1-007, F2-009, F3-005, F3-009, F4-004, F4-010, F5-003, F5-009);
   - X1 capturer files (F4-007);
   - Machines tile, layout-editor machine widget and GUI shell view-event file (F2-012).
6. **COM inventory.** Which COM objects `internal/platform/windows/com` (W2) already provides (IDataObject, IEnumFORMATETC, IStream, STA, handle table). Append NOTEs to P4-F1-002 and P4-F1-003.
7. **Cross-stream requirements.** D-102 put these into the P2 items (W1-011, W2-004, X1-015/016, M1-010, Y1-013); verify each on `main` and propose a fix item for any that is missing:
   - W1 capturer parks the cursor at the contact point while grabbed (needed by F2 FinishOutgoing);
   - W2 clipboard backend does not announce a clipboard update whose only formats are virtual-file formats;
   - X1 exports or lacks an INCR helper (P4-F4-002);
   - M1 exposes a main-thread dispatcher and its changeCount poll interval;
   - Y1 portal clipboard API surface.

   Any missing requirement becomes a proposed fix item for the owning P2 stream.
8. **P1 C4 APIs** used by P4-F-010 .. P4-F-012 (receive writer with caller root, chunk request/serve API, queue entry API, relay): names and signatures.
9. **Item impacts.** Items that must be annotated or replaced (append-only), including any variant-B selections already implied.

## Item P4-F-002
User approval of the remaining proposals. The parent presents only the proposals that `briefs/p4-preflight.md` adds
beyond the planning consolidation (D-86, D-99 .. D-103, D-109 are already locked) in one consolidated question round,
then appends:
- one decision per approved proposal to `decisions.md` (dated, "Supersedes" where applicable);
- claims/notes to `ownership.md`;
- the D-100 depguard change to `.golangci.yml` (claim; run `golangci-lint config verify`).

## Item P4-F-003
`internal/vfile`: `doc.go` (package doc), `vfile.go` (types, errors and Outcome from the contract), `validate.go`. Plus `validate_test.go` and `validate_fuzz_test.go`.

`Validate` returns `ErrInvalidManifest` wrapped with the entry index and reason. It checks, in order:
1. ID is 32 lowercase hex.
2. Origin non-empty.
3. 1 <= len(Entries) <= MaxEntries.
4. For each entry:
   - every path component passes `internal/transfer/safepath` (D-45 rules);
   - path uses "/" only (backslash rejected);
   - no "." or ".." components;
   - len(Path) <= MaxPathBytes;
   - directories have Size 0 and `fs.ModeDir`;
   - files have Size >= 0 and no type bits other than regular;
   - the parent directory of a nested entry appears at a lower index;
   - no two entries equal under `strings.EqualFold` (Windows and macOS targets are case-insensitive).

`TotalBytes` sums file sizes. `TopLevel` returns the indexes of entries without "/".

Test the table of every D-45 rule with one valid and one invalid case each. The fuzz target asserts `Validate` never panics and that accepted manifests round-trip through `TopLevel`/`TotalBytes` consistently.

## Item P4-F-004
`uri.go`:
- `EncodeURIList`:
  - each absolute local path becomes `file://` + percent-encoded path (RFC 8089);
  - on Windows `C:\a b` becomes `file:///C:/a%20b`;
  - percent-encode every byte outside RFC 3986 unreserved plus "/";
  - lines end with CRLF (RFC 2483 section 5).
- `EncodeGnomeCopiedFiles`: first line `copy`, then one URI per line separated by "\n", no trailing newline. Always `copy`, never `cut` (D-62). Verify the format against GNOME documentation or the Nautilus behaviour recorded in sp3.md; record the source in a code comment only if non-obvious.
- `DecodeURIList`:
  - skip lines starting with "#" and blank lines;
  - accept only scheme `file` with an empty host or `localhost`;
  - percent-decode strictly (invalid escape -> error);
  - reject NUL;
  - return local paths.
- `DecodeGnomeCopiedFiles`: first line `copy` or `cut` (both accepted, both treated as copy), rest as URIs.

Fuzz `DecodeURIList` for panics and round trip with `EncodeURIList`.

## Item P4-F-005
`build.go`: `Build(ctx, origin, roots, limits)`.
- Each root is an absolute path. Its base name becomes a top-level entry; two roots with the same base name (case-insensitive) give `ErrInvalidManifest`.
- Use `os.Lstat` and `filepath.WalkDir`:
  - symlinks are skipped and reported (`SkipSymlink`);
  - devices, sockets and FIFOs are skipped and reported (`SkipSpecial`);
  - directories are emitted before their children, children in lexical order of name;
  - empty directories are included.
- Stop with `ErrInvalidManifest` as soon as MaxEntries or MaxPathBytes is exceeded, without walking the rest.
- Check `ctx` between entries.
- ID from `crypto/rand` (16 bytes, hex).
- ModTime and Mode from `Lstat`.
- `Paths[i]` is the absolute local path of `Entries[i]`.
- The result passes `Validate`. Call it before returning.

`build_unix_test.go` (`//go:build unix`) covers FIFOs. Symlink tests call `os.Symlink`; on a privilege error, `t.Skipf` with the error text.

## Item P4-F-006
`readahead.go`.
- A worker goroutine fills a ring buffer of `window` bytes from `p.Open(ctx, index, offset)`.
- `Read` blocks until `len(b)` bytes are available or EOF/error. It returns fewer bytes only at EOF (`io.EOF` on the following call, per `io.Reader`).
- `Seek` (Start/Current/End relative to size):
  - inside the buffered range [pos, pos+buffered): drop the skipped bytes, no reopen;
  - backwards or beyond the buffer: stop the worker, close the reader, reopen at the new offset.
- Provider errors are returned by the next `Read` unchanged (wrapped), never turned into zeros.
- `Close` cancels the worker and waits for it.
- Memory never exceeds `window` plus one copy buffer of 64 KiB.

Tests use a fake provider with configurable latency (synctest) and error injection.

## Item P4-F-007
`internal/platform/files.go` exactly as the contract (doc comments only where non-obvious). `files_test.go`: `DragOutKind.String` table, and a `go list -deps` check (run through `os/exec` of `go list`, skipping when `go` is not on PATH is NOT allowed: fail) that `internal/platform` does not depend on `internal/transfer` or `internal/clipboard`.

## Item P4-F-008
`internal/platform/fake/files.go`.

`fake.FileClipboard`:
- `SimulateForeignCopy(paths)` calls watchers with an incrementing seq.
- `OfferFiles` stores the manifest/provider and gives `Done(OutcomeAbandoned)` to the previous provider if it had no outcome.
- `Paste(ctx, dir)` reads every entry through the provider (entries written with `os.Root` into `dir`) and calls `Done(OutcomeCompleted)`, or `Done(OutcomeFailed)` on error.
- `CancelPaste()` calls `Done(OutcomeCancelled)`.
- `Offered()` returns the current manifest.

`fake.DragDrop`:
- scripted `Emit(DragOutEvent)`;
- records `Arm`, `SetEdges`, `FinishOutgoing` calls;
- `BeginIncoming` returns a `fake.IncomingDrag` whose `Release` gives `DropResult{Dropped: true}`, `Cancel` gives `Dropped: false`, and `Wait` blocks on either;
- a configurable start error.

`fake.Backend` gets `FileClipboard()` and `DragDrop()` methods, plus switches to make them return `platform.ErrUnsupported`.

## Item P4-F-009
`internal/platform/conformance/files.go`.

`RunFileClipboard(t, newFn func(t *testing.T) platform.FileClipboard, d FileDriver)`. `FileDriver` methods: `ForeignCopy(paths []string) error`, `Paste(ctx, dir string) error`, `CancelPaste() error`, `ReplaceWithForeignText() error`. Cases:
- WatchReportsForeignCopy;
- WatchIgnoresOwnOffer;
- OfferReplacesPreviousAndAbandons;
- PasteReadsAllEntriesAndCompletes (hashes compared);
- CancelledPasteReportsCancelled;
- ForeignReplaceAbandonsOffer;
- CloseReleasesOwnership;
- NotRepresentableManifestReturnsErr (driver-provided manifest; drivers that can represent everything return `nil` from `UnrepresentableManifest()` and the case is skipped with a logged reason).

`RunDragDrop(t, newFn, d DragDriver)`. `DragDriver` methods: `ForeignDragToStrip(paths) error`, `ForeignDragAway() error`, `ForeignReleaseOnStrip() error`, `ForeignNonFileDragToStrip() error`, `DropTargetFolder() string`. Cases:
- SetEdgesThenEnterLeave;
- UnarmedStripIgnoresDrags;
- UnsupportedSourceReported;
- DropOnStripReported;
- FinishOutgoingAcceptedAndCancelled;
- BeginIncomingReleaseDropsIntoFolder;
- BeginIncomingCancel;
- BeginIncomingContextCancel;
- CloseEndsActiveDrag.

Both suites run against fakes in `files_test.go`.

## Item P4-F-010
`internal/transfer/staging.go`: `type Stager`, constructed with the staging dir (D-69 via `internal/paths`), limits, and the P1 C4 receive writer.
- `Materialize(ctx, m vfile.Manifest, p vfile.Provider, progress func(done, total int64)) ([]string, error)`:
  1. Validate the manifest.
  2. Check free space >= TotalBytes (`staging_unix.go`: `unix.Statfs`; `staging_windows.go`: `windows.GetDiskFreeSpaceEx`), else an explicit error.
  3. Create `<staging>/<m.ID>/` (0700 on Unix).
  4. Open an `os.Root` on it and write every entry through the receive writer (`.edgesync-partial` + per-chunk verification + rename).
  5. Return the absolute paths of the top-level entries.
  - Concurrent calls for the same ID share one run (single flight). A completed ID returns its paths immediately. Cancel removes partial files and the ID directory.
- `Sweep(now time.Time)` removes ID directories older than 24 h (D-57), skipping active ones.
- File permissions: 0600 files and 0700 dirs on Unix (tested in `staging_unix_test.go`). On Windows the staging dir inherits the owner-only ACL of `%LOCALAPPDATA%`; no extra ACL code.

## Item P4-F-011
`internal/transfer/offers.go`: `type Offers`.
- `Register(local vfile.Local) error`.
- `Serve(ctx, req)` handles incoming ChunkRequests through the P1 C4 serve path:
  - looks up manifest ID + entry index; directories are rejected;
  - `os.Lstat` must still show a regular file with the same Size and ModTime, else `vfile.ErrSourceChanged`;
  - opens the absolute path registered for that index; a chunk request can never name a path;
  - streams with per-chunk SHA-256 (P1 C4);
  - at most 4 concurrent streams per peer (D-44), extra requests wait with ctx.
- `Retire(id)`: in-flight streams continue, new opens get `vfile.ErrOfferRetired`.
- `Release(id)`: retired and no stream left, forget it.
- `Close`.

No code path removes or renames source files (D-62); a test asserts it with a read-only tree.

## Item P4-F-012
`internal/transfer/remoteprovider.go`: `NewRemoteProvider(sess <P1 C4 request API>, m vfile.Manifest, q *Queue) vfile.Provider`.
- `Open` sends a ChunkRequest for (manifest ID, index, offset) through the session (relay through S is transparent per D-59). It returns a reader that verifies each chunk hash before releasing its bytes; a mismatch returns an error and no bytes of that chunk.
- When no session to the origin exists, return `vfile.ErrPeerOffline` immediately. The OS consumer decides whether to retry.
- A later `Open` at an offset resumes from that offset (D-44 verified-prefix semantics apply to staging, while OS consumers resume by offset).
- `queue_vfile.go`:
  - registers one queue entry per manifest at the first `Open` (direction incoming, peer = origin, total = TotalBytes) and aggregates bytes across files;
  - queue cancel cancels every open reader's context and calls `Done(OutcomeCancelled)`;
  - `Done` from the consumer finishes the entry; only the first `Done` counts;
  - records TTFB (first `Open` to first byte delivered), bytes, duration and peak buffered bytes;
  - logs them at Info without file names (D-48).

## Item P4-F-013
`internal/clipboard/files.go`: `type FileSync`, wired with a `platform.FileClipboard`, `transfer.Offers`, a provider factory, the announcement sender of the P1 C4 clipboard sync, settings (master + files toggle, D-61) and a notifier.

Behaviour per the flow summary:
- **Echo prevention:** own offers are not reported by backends; additionally, announcements with our own origin are ignored.
- **Same-seq rule:** a `ClipboardBackend.Watch` change with the same seq as the last `WatchFiles` seq is dropped.
- **Sensitive content:** none (D-121): file copies are announced like any other content; there is no sensitivity flag and no backend rule, so this item has no sensitivity test.
- **Errors:** `ErrNotRepresentable` from `OfferFiles` raises Notification `files.notRepresentable`. Build limit errors raise `files.tooMany`. Keys are added in P4-F-017.

## Item P4-F-014
`internal/dnd/proto.go`:
- Encode/decode `DragBegin`, `DragDrop`, `DragCancel` through the generated capnp types.
- Decoding applies the control-channel ReadLimiter traversal and depth limits (architecture 5.3) and rejects oversized messages before decoding.
- `DragBegin` decoding validates the embedded manifest with the approved limits.

If P4-F-002 approved a schema delta: edit `proto/schema/dnd.capnp`, run `go generate ./internal/proto/...`, commit the generated files, and make `task gen:check` green.

## Item P4-F-015
`internal/dnd/coordinator.go`: `type Coordinator` with options (role, local peer ID, `platform.DragDrop` or nil, `transfer.Offers`, provider factory, message sender/receiver for the control channel, notifier, clock via synctest-friendly timers).
- It implements the engine hook interface defined in P4-F-016.
- State per session: `Idle`, `Armed(machine)`, `OutgoingAtStrip(origin)`, `Remote(origin, target)`, `Finishing`.
- Implement flow steps 1-9 exactly. Every transition is logged at Debug with session id and peers (short IDs).
- `FinishOutgoing` is called exactly once per session.
- An unsupported backend raises `dnd.unsupported` once per agent run.
- `DragOutUnsupported` raises `dnd.unsupportedSource` (throttled once per 60 s).

## Item P4-F-016
Engine hooks, `internal/engine/draghooks.go`:
```go
type DragHooks interface {
    LeftButton(machine string, down bool)           // every left transition on the machine holding the virtual cursor
    Switched(from, to string)                       // after every switch
    KeepLeftHeld(machine string) bool               // server: exclude left from ReleaseAll sent to `machine`
    ConsumeTargetInput(f wire.Frame) bool           // client: true when the frame (left button or Escape key) belongs to an active incoming drag
    CancelAll(reason string)                        // hotkey jump, panic, lock-to-screen toggle during a drag
}
```
- The engine holds `DragHooks` as an interface value set once at construction (nil allowed = previous behaviour).
- Calls on the hot path must not allocate. The benchmark proves 0 allocs/op with a no-op implementation.
- Only `internal/engine/server.go` and `internal/engine/client.go` change (claims), plus the new files.

## Item P4-F-017
`internal/agent/files.go`:
- Type-assert the backend for `FileClipboardSource` / `DragDropSource`.
- Construct `Offers`, `Stager`, `FileSync`, `Coordinator`; wire the coordinator into the engine hooks.
- `Stager.Sweep` at start and every hour (synctest-driven test).
- Report unsupported features once via Notification and the status flags exposed over IPC (existing `StatusChanged` fields; no schema change).
- Secure-helper / pre-login mode (D-64): files and DnD are not constructed at all.

Add the i18n keys to `internal/i18n/locales/en/files.toml` (D-94): `files.notRepresentable`, `files.tooMany`, `files.unsupported`, `files.pasteFailed`, `dnd.unsupported`, `dnd.unsupportedSource`, `dnd.failed`. Each has a one/other form where counts appear.

## Item P4-F-018
`test/e2e/files_test.go` and `dnd_test.go`, using the G5 e2e harness (two or three agents on fakes over loopback):
- copy on S, paste on C, hashes equal;
- copy on C1, paste on C2 through S;
- paste after a newer text copy elsewhere fails with `ErrOfferRetired` surfaced as `files.pasteFailed`;
- drop the C-S connection at 50% of a paste, reconnect, a re-open at the offset completes with equal hashes;
- DnD S to C and C to S with Release;
- Escape cancels and the origin sees `FinishOutgoing(false)`.

## Item P4-F-019
`internal/transfer/vfile_bench_test.go`: loopback session pair from the P1 C4 test helpers, synthetic 1 GiB deterministic file. Budget tests follow plan.md `Test*Budget` conventions. Print measured numbers with `t.Logf` and copy them into the report's TESTS line:
- plain transfer-engine throughput vs vfile paste throughput (read-ahead window from the P4-F-002 decision);
- TTFB p95 over 200 opens;
- heap-in-use growth sampled with `runtime.ReadMemStats` every 100 ms;
- resume test with an injected connection drop at 40%.

## Item P4-F-020
`gui/drop/`:
- `drop.go` (contract above).
- `area.go`: `type Area struct` with `Layout(gtx layout.Context, r *Router, peer string, w layout.Widget) layout.Dimensions`. It records the widget's clip rectangle in window pixels into the router for the current frame. `Router.BeginFrame()` clears the previous frame's rectangles.
- `router.go`:
  - `Router` with a mutex;
  - `HitTest(p image.Point) (peer string, ok bool)`: the last registered rect containing p wins, being topmost in paint order;
  - `Deliver(ctx, d Drop) error`: hit test, then `Sender.SendFiles(ctx, peer, d.Paths)`; `ErrNoTarget` when nothing is hit; no call for empty paths;
  - `type Sender interface{ SendFiles(ctx context.Context, peer string, paths []string) error }`, implemented by the P3 IPC client binding.

No per-OS code here.

## Item P4-F-021
Dispatch `code-reviewer` (`model: "sonnet"`) over the merged P4-F diff. The review focuses on:
- contract equals this brief;
- import direction;
- goroutine ownership;
- error wrapping;
- no silent fallbacks;
- hot-path allocations;
- test coverage per item.

## Item P4-F-022
Security review with `qa/security-review.md` sections 5, 6, 7. P4-F focus list:
- manifest bounds before allocation;
- every receive path through `os.Root` + safepath (staging);
- offers can only read registered paths;
- `ErrSourceChanged` prevents serving a swapped file;
- staging permissions and the 24 h sweep;
- URI decoding rejects non-file schemes, remote hosts, NUL and bad escapes;
- dnd decode limits;
- no move semantics anywhere (D-62);
- no file names at Info.

## Item P4-F-023
Parent:
1. Compare `internal/platform/files.go`, `internal/vfile/vfile.go`, `gui/drop/drop.go` with the contract sections above.
2. Run `task ci:local`.
3. Append the FILE CONTRACT FREEZE and review-chain NOTEs.
4. Append the ownership freeze note and the P4-F-019 result rows.

After the freeze, changes to these files need a new decision and user approval.

## Item P4-X-001
Parent phase exit. Check each plan.md P4 exit criterion and the FEAT coverage (FEAT-099 .. FEAT-116). Append a dated NOTE to `plan.md` whose body contains the new marker line on its own line. Example when P5 R3/R4 already run as fill:
```
<!-- NOTE: <date> - P4 exit criteria met.
Current phase: P5
-->
```
List every phase that is still active on the marker line. Mirror the marker in `CLAUDE.md`.
