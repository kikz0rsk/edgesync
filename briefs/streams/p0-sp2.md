# Stream brief - P0 SP2 Windows COM shell (virtual files, delayed rendering, OLE drag-and-drop)

Phase 0 spike stream. Items: `briefs/plan-phase-0.md` section "SP2". Owner tag: `[owner:SP2]`.

## Scope
Prove the pure-Go COM/OLE mechanics behind D-10 (delayed rendering), D-11 (virtual-file paste), D-12 (cross-screen
drag-and-drop and native drop onto the GUI) and D-61 (sensitive clipboard formats). Deliverable: evidence plus
`briefs/spikes/sp2.md`. This is the highest-risk area of the project (R-23, R-24, R-27): be exact.

## Files you may touch
Only your item's `Files:`. Shared COM code lives in `spikes/internal/wincom/` (created by P0-SP2-001, reused by
002, 004, 005). Never touch the main module, plan files, or other streams' spike directories.

## Conventions (binding)
- Pure Go, `CGO_ENABLED=0` (D-39). No go-ole for hosting objects (it targets calling COM, hosting is UNVERIFIED - RPI 6b).
- COM object memory and vtables are allocated outside the Go heap (`windows.LocalAlloc(LMEM_FIXED|LMEM_ZEROINIT, n)` or `CoTaskMemAlloc`), because Windows retains these pointers. Never hand a Go heap pointer to Windows for retention. Layout per object: `{ lpVtbl uintptr; handle uintptr }`; the handle indexes a Go-side table (`handles.go`, mutex-protected) holding the Go implementation and the reference count.
- One `syscall.NewCallback` per interface method, created once at package init, shared by all instances (dispatch via `this` -> handle). Assert the number of callbacks created stays <= 64.
- All OLE work on one STA thread: `runtime.LockOSThread`, `OleInitialize(nil)`, a `GetMessageW` pump, and a work queue (posted thread message + channel) to marshal calls onto it.
- Verify every struct layout against Microsoft Learn and assert with `unsafe.Sizeof`: `FORMATETC` (32 bytes on amd64), `STGMEDIUM` (24), `FILEDESCRIPTORW` (592), `STATSTG`, `DROPFILES`. Relevant constants: `TYMED_HGLOBAL=1`, `TYMED_ISTREAM=4`, `DVASPECT_CONTENT=1`, `FD_ATTRIBUTES=0x4`, `FD_WRITESTIME=0x20`, `FD_FILESIZE=0x40`, `FD_PROGRESSUI=0x4000`, `FD_UNICODE=0x80000000`, `DROPEFFECT_COPY=1`; registered formats `FileGroupDescriptorW`, `FileContents`, `Preferred DropEffect`, `Performed DropEffect`, `Paste Succeeded`, `HTML Format`, `Rich Text Format`, `PNG`, `ExcludeClipboardContentFromMonitorProcessing`, `CanIncludeInClipboardHistory`.
- Never launch spike binaries; prebuild into `spikes/bin/`.
- No em dashes or en dashes. Commit `spike(win): ... [FEAT-177]` or `spike(gui): ...`, body `Item: <id>`, no attribution trailers.
- Research pointers: `briefs/research-platform-internals.md` sections 5 (Windows rows, delayed rendering) and 6 (approaches a, b; MWB drag-drop hack); Microsoft Learn "Shell Clipboard Formats", "Clipboard Operations", `OleSetClipboard`, `DoDragDrop`, `RegisterDragDrop`, `IDataObject`, `IStream`.

## Report format
```
FILES: <path> - <one clause>
GATES: <gate> pass|fail
ITEMS: <item id> <FEAT-NNN, ...>
TESTS: <what the test phase must cover, one line>
BLOCKED: <reason or NONE>
```

---

## Item P0-SP2-001
COM infrastructure in `spikes/internal/wincom/`.
- `vtable_windows.go`: helpers to allocate a vtable (array of uintptr callback addresses) in non-Go memory and an object block pointing to it.
- `unknown_windows.go`: `QueryInterface` (match IIDs per object kind; return `E_NOINTERFACE` 0x80004002 otherwise), `AddRef`, `Release` (free object memory and handle at zero).
- `dataobject_windows.go`: IDataObject methods in vtable order: QueryInterface, AddRef, Release, GetData, GetDataHere, QueryGetData, GetCanonicalFormatEtc, SetData, EnumFormatEtc, DAdvise, DUnadvise, EnumDAdvise. Go interface `DataSource { Formats() []FormatEtc; GetData(f FormatEtc) (Medium, error); SetData(f FormatEtc, m Medium) error }`. Unsupported advise methods return `OLE_E_ADVISENOTSUPPORTED`.
- `enumformatetc_windows.go`: Next, Skip, Reset, Clone.
- `stream_windows.go`: IStream over a Go `io.ReadSeeker` + size: QueryInterface (IUnknown, ISequentialStream, IStream), AddRef, Release, Read, Write (STG_E_ACCESSDENIED), Seek, SetSize, CopyTo, Commit, Revert, LockRegion, UnlockRegion, Stat (fills STATSTG with size and name when requested), Clone.
- `dropsource_windows.go` (QueryContinueDrag, GiveFeedback) and `droptarget_windows.go` (DragEnter, DragOver, DragLeave, Drop) with Go callback interfaces.
- `sta_windows.go`: `type STA struct` with `Start()`, `Do(func()) error` (runs on the STA thread, returns after completion), `Close()`.
- Tests on the host call through the vtables with `syscall.SyscallN(vtbl[i], this, ...)` to prove QueryInterface/AddRef/Release counting, enumerator semantics, and IStream Read/Seek/Stat.

## Item P0-SP2-002
Virtual files on the clipboard (`spikes/winvfiles/`).
- `source.go` (pure Go): a synthetic file source producing deterministic bytes (`byte(offset*31+7)`), throttled to `-rate` MB/s with `-latency` initial delay; `ReadAt`/`Seek`.
- `descriptor_windows.go`: build an HGLOBAL `FILEGROUPDESCRIPTORW` with entries: `demo` (directory attribute), `demo\a.bin` (200 MiB), `b.txt` (10 KiB); flags FD_ATTRIBUTES|FD_FILESIZE|FD_WRITESTIME|FD_PROGRESSUI|FD_UNICODE.
- DataSource: FileGroupDescriptorW (TYMED_HGLOBAL), FileContents per `lindex` (TYMED_ISTREAM, a new IStream per GetData call), Preferred DropEffect (DROPEFFECT_COPY); accepts SetData for Performed DropEffect and Paste Succeeded (log them).
- `main.go`: STA start, `OleSetClipboard`, log every IDataObject call with format name, lindex, tymed, aspect, and time; `-flush` decides whether `OleFlushClipboard` runs on exit; `-blockread 3s` makes one Read block to observe Explorer responsiveness. Wait for Ctrl+C.

## Item P0-SP2-003
Delayed rendering (`spikes/winclip/`).
- `listener_windows.go`: message-only window, `AddClipboardFormatListener`, on `WM_CLIPBOARDUPDATE` (0x031D) log `GetClipboardSequenceNumber`, `GetClipboardOwner`, enumerate formats (names via `GetClipboardFormatNameW`), skip events where the owner is our own window, and flag sensitivity formats.
- `render_windows.go`: offer mode: `OpenClipboard(hwnd)`, `EmptyClipboard`, `SetClipboardData(fmt, 0)` for CF_UNICODETEXT (13), HTML Format, Rich Text Format, PNG, CF_DIBV5 (17); `WM_RENDERFORMAT` (0x0305): sleep `-delay` then `SetClipboardData(fmt, hglobal)` without opening the clipboard; `WM_RENDERALLFORMATS` (0x0306): open, render all still-owned formats, close; `WM_DESTROYCLIPBOARD` logged. `-sensitive` adds the two sensitivity formats.
- `cfhtml.go`: build and parse CF_HTML (Version:0.9 or 1.0 header with byte offsets StartHTML/EndHTML/StartFragment/EndFragment, `<!--StartFragment-->`/`<!--EndFragment-->` markers, UTF-8 body).
- `dib.go`: PNG <-> BITMAPV5HEADER + pixels (32 bpp BGRA, bottom-up and top-down), using `image/png`.

## Item P0-SP2-004
OLE drag-and-drop (`spikes/windnd/`).
- `edgetarget_windows.go`: for the primary monitor's right edge create a 2 px wide topmost layered tool window (`WS_EX_LAYERED|WS_EX_TOPMOST|WS_EX_TOOLWINDOW|WS_EX_NOACTIVATE`, alpha 1) registered with `RegisterDragDrop` on the STA thread. When no drag is in progress the window must be click-through: toggle `WS_EX_TRANSPARENT` when a low-level mouse hook (reuse `spikes/internal/winhook`) reports the left button down outside the window, and log whether DragEnter still fires reliably. On DragEnter/Drop read `CF_HDROP` (DROPFILES + double-NUL list) and FileGroupDescriptorW if present; print; return DROPEFFECT_COPY.
- `source_windows.go`: a small window; on `WM_LBUTTONDOWN` call `DoDragDrop(dataObject, dropSource, DROPEFFECT_COPY, &effect)` with the SP2-002 virtual-file data source; log the result effect.
- README asks the user to also run Explorer elevated ("Run as administrator") and record UIPI behaviour.

## Item P0-SP2-005
Gio drop target (`spikes/giodrop/`, gioui.org v0.10.3).
- Obtain the HWND from Gio's Win32 view event (find the exact type in `gioui.org/app` v0.10.3 docs, e.g. a `Win32ViewEvent` with `HWND`); verify the thread on which Gio delivers it and call `RegisterDragDrop` on the window's own thread (use Gio's mechanism to run code on the window thread if one exists; otherwise record the failure precisely).
- Show the last dropped paths and the drop point converted from screen coordinates to Gio's dp coordinates in the window.

## Item P0-SP2-006
`qa/manual/p0-sp2-windows-shell.md` per `qa/manual/README.md`. Steps: winclip (copy in Notepad/Word/browser/password manager, paste into Notepad/WordPad/Word/Paint, exit with formats still owned), winvfiles (paste into an Explorer folder, watch progress, cancel mid-way, `-blockread`, `-flush` on/off), windnd (drag files to the right edge; drag from the source window to an Explorer folder and to the desktop; elevated Explorer), giodrop (drop files from Explorer). Each step: command, expected observation, paste-back.

## Item P0-SP2-M01
User-owned. Results into `qa/manual/results/p0-sp2-windows-shell.md`.

## Item P0-SP2-007
`briefs/spikes/sp2.md`: summary table (mechanism | go/no-go | evidence | decision confirmed/contradicted), sections per mechanism, "Recommended design for P2 W2 and P4 F1/F2" (COM package API for the main module, threading, prefetch strategy for Explorer reads, edge drop-target strategy), "Requires user decision" for anything that contradicts D-10, D-11, D-12, D-13, D-40 or D-61.
