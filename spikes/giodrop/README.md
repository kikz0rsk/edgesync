# giodrop - native file drop onto a Gio window on Windows (P0-SP2-005)

## Warning

- Gio is `gioui.org` v0.10.3. The window is a plain Gio window; nothing else is drawn.
- The program registers an OLE drop target on the window. It only accepts `DROPEFFECT_COPY` and never moves, links or
  deletes anything. Use throw-away files for the drags all the same.
- Close the window with its title-bar button to end the run. Dropping several files at once is fine.

## Purpose

Find out whether a Gio window can receive files dragged from Explorer on Windows, without cgo (D-33, D-34, D-39). The
spike answers three questions:

1. Can the native HWND be obtained from Gio? Yes: `app.Win32ViewEvent{HWND}` carries it (`gioui.org@v0.10.3/app/os_windows.go`,
   sent on the window thread right after the window is created).
2. Can an OLE `IDropTarget` be registered on that window's own thread? Yes, with `app.Window.Run`, which Gio documents as
   "Run f in the same thread as the native window event loop" (`app/window.go`, `os.go`: `eventLoop.Run` queues `f` for the
   window loop and wakes it with a posted message). The closure calls `OleInitialize` then `RegisterDragDrop` there.
3. Does it conflict with Gio's COM state? Gio never calls `CoInitialize`/`OleInitialize` (no match in
   `gioui.org@v0.10.3/app`), so the window thread enters an STA for the first time. The window shows the HRESULT
   (`S_OK` expected). `S_FALSE` would mean something else already initialised COM on that thread.

The drop target is the hand-built `IDropTarget` from `spikes/internal/wincom`. The window shows the HWND, the OLE
registration state, the number of drops, the last drop point in dp and px, the last dropped paths, and the last error.

### Why `OleInitialize` and not `CoInitializeEx`

Learn, RegisterDragDrop: "If you use CoInitialize or CoInitializeEx instead of OleInitialize to initialize COM,
RegisterDragDrop will always return an E_OUTOFMEMORY error." The code therefore calls `OleInitialize(NULL)`.

### Coordinates

`IDropTarget::Drop` gives the point in screen coordinates. `ScreenToClient` converts it to client device pixels of the
HWND, which are Gio's pixels (the window thread is per-monitor DPI aware, set by Gio). The layout divides by the frame's
`unit.Metric.PxPerDp` to show device-independent pixels.

### Alternative not built: DragAcceptFiles + WM_DROPFILES

Feasible on paper, not implemented (the item names the OLE route). `DragAcceptFiles(hwnd, TRUE)` (Shell32, documented
"registers whether a window accepts dropped files") plus `WM_DROPFILES` with `DragQueryFileW` and `DragQueryPoint`.
Gio's `windowProc` has no `WM_DROPFILES` case (grep of the v0.10.3 source), so it would need a window-procedure subclass
(`SetWindowLongPtrW(GWLP_WNDPROC)`) that forwards every other message to Gio's procedure. It gives the paths and the
client point but no `DragEnter`/`DragOver`/effect negotiation and no `IDataObject`, which P4-F2 needs for virtual files
(`FileGroupDescriptorW`, `FileContents`). OLE is therefore the route to carry forward.

### Known gaps (recorded, not hidden)

- `RevokeDragDrop` is not called. Gio sends the empty `Win32ViewEvent` from `WM_DESTROY` on the window thread and then
  quits that thread, so there is no thread left to run the revocation through `Window.Run`. The process exits with the
  window. P4-F2 must decide the teardown order.
- `OleUninitialize` is not balanced for the same reason (the OLE call is made on the window thread, which ends with the
  window).
- ANSI (`fWide == 0`) `CF_HDROP` lists are rejected with an error shown in the window. Explorer always sends wide lists.
- Drops without `CF_HDROP` (for example text) are reported as an error (`drop data: ... GetData ...`) and not accepted.
- `Window.Run` waits for the window thread; it would block forever if the window closed before the attach closure ran.
  The attach runs straight after the window is created, so this is not expected.

## Build

From the worktree root (Windows host, `CGO_ENABLED=0`):

    go -C spikes vet ./giodrop
    go -C spikes build -o bin/giodrop.exe ./giodrop

Create `spikes/bin` first if it does not exist. The binary is gitignored. Non-Windows builds compile only the
`main_other.go` stub (`windows only`, exit 2).

## Run

    spikes\bin\giodrop.exe

Do not run this from an agent. Run it on a Windows PC with a scratch folder of files (any size, one or several).

## What to observe

1. A 720 x 480 dp window titled "EdgeSync Gio file-drop spike" opens with the text "OLE drop target: not registered"
   for an instant, then "Window HWND: 0x..." (non-zero), "OleInitialize on the window thread: S_OK (first COM
   initialisation on this thread)" and "OLE drop target: registered on the Gio window thread".
2. Drag two or more files from an Explorer window onto the window. The cursor shows a copy/plus badge while over the
   window; releasing the button drops them.
3. After the drop: "Drops: 1" (then 2 for the next drop), a "Last drop point" line with the drop point in dp and client
   px (the px value is about the cursor position inside the client area, dp = px / scale), and one line per dropped path
   with the full Windows path (`C:\...`, no trailing separator).
4. Drop a text selection or a web link onto the window: "Last error: drop data: ..." with the `CF_HDROP` `GetData`
   failure, and "Drops" unchanged.
5. Resize the window or change the display scale (move it to a monitor with another scale): the drop point stays
   correct in dp (the same physical spot gives the same dp value on both scales).
6. Close the window with the title-bar button. The process exits with status 0.

## Paste back

- The full text of the window after step 1 (HWND value, OleInitialize result, registration line).
- The window text after step 3, including the drop point line and the path lines.
- The exact "Last error" line from step 4.
- The dp values for step 5 (before and after the scale change), with the monitor scales.
- Whether the program printed anything on stderr (expected: nothing until the close in step 6).
