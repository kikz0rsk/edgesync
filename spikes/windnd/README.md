# windnd - OLE drag-and-drop edge drop target and virtual-file drag source (P0-SP2-004)

## Warning

- The program installs the `internal/winhook` low-level mouse and keyboard hooks (plus a Raw Input window) to watch the
  left mouse button. Scroll Lock toggles that hook's input suppression system-wide and is not wired to anything else
  here: do not press Scroll Lock during a run. If you do, press it again.
- The edge drop target never returns `DROPEFFECT_MOVE` or `DROPEFFECT_LINK`, only `DROPEFFECT_COPY` or
  `DROPEFFECT_NONE`, and it never copies or deletes anything. Dropping Explorer files on the edge strip leaves them
  where they are. Use throw-away files for the drags all the same.
- The drag source offers three virtual files. Dropping them into an Explorer folder or on the desktop creates real files
  there (a `demo` folder with a 200 MiB `a.bin` and a `b.txt`). Use a scratch folder and delete the result afterwards.
- Ctrl+C or closing the source window ends the run. If a drag is in progress the program exits when it ends.

## Purpose

Decide whether the OLE drag-and-drop half of D-12 and D-13 holds on Windows (architecture 6.4):

1. Edge drop target. A 2 px wide window on the right edge of the primary monitor (`WS_EX_LAYERED | WS_EX_TOPMOST |
   WS_EX_TOOLWINDOW | WS_EX_NOACTIVATE`, `SetLayeredWindowAttributes` alpha 1) is registered with `RegisterDragDrop`
   on the STA thread (`spikes/internal/wincom`, hand-built `IDropTarget`). When a file drag from Explorer enters it, the
   program lists every format of the data object, decodes `CF_HDROP` (the `DROPFILES` header and the double-NUL path
   list, wide or ANSI) and `FileGroupDescriptorW` (names, flags, sizes), and answers `DROPEFFECT_COPY`.
2. Click-through when idle. The strip carries `WS_EX_TRANSPARENT`, so clicks and the pointer go to the window underneath.
   A low-level mouse hook reports the left button: a press outside the strip clears `WS_EX_TRANSPARENT` (so a drag that
   starts anywhere can enter the strip), the release sets it again. A press inside the strip is left to the window
   underneath. Every gesture is summarised, including whether the pointer crossed the strip with the button held but
   no `DragEnter` fired, which is how the spike measures reliability and UIPI blocking.
3. Virtual-file drag source. A small window starts `DoDragDrop(dataObject, dropSource, DROPEFFECT_COPY, &effect)` on
   `WM_LBUTTONDOWN` with a hand-built virtual-file `IDataObject`, the same tree and formats as the `winvfiles` clipboard
   spike. The `IDropSource` follows the standard rules (Escape cancels, releasing the left button drops) and logs its
   decisions and feedback changes.

The offered tree (content is `byte(offset*31+7)`, served lazily at `-rate` with `-latency` before the first byte):

| lindex | path | kind | size |
|---|---|---|---|
| 0 | `demo` | directory | 0 |
| 1 | `demo\a.bin` | file | 200 MiB |
| 2 | `b.txt` | file | 10 KiB |

Formats offered by the drag source data object: `FileGroupDescriptorW` (`TYMED_HGLOBAL`, flags `FD_ATTRIBUTES |
FD_FILESIZE | FD_WRITESTIME | FD_PROGRESSUI | FD_UNICODE`), `FileContents` (`TYMED_ISTREAM`, one new read-only `IStream`
per `GetData`, directories and out-of-range indexes answer `DV_E_LINDEX`) and `Preferred DropEffect`
(`DROPEFFECT_COPY`). `SetData` accepts `Performed DropEffect` and `Paste Succeeded`; any other format is refused with
`DV_E_FORMATETC` and logged. Requests for formats the object does not offer are answered `DV_E_FORMATETC` by the COM
layer and do not reach the log.

The payload code in `source_windows.go` (descriptor builder with the D-99 name rules, synthetic lazy reader) is a copy of
the `winvfiles` code, because `winvfiles` is a `package main` and cannot be imported. The shared extraction belongs to a
later item.

## Build

From the repository root:

```
go -C spikes build -o bin/windnd.exe ./windnd
```

## Run

Run from `spikes\`. Every console line is a `log/slog` text record with a timestamp.

```
.\bin\windnd.exe
.\bin\windnd.exe -rate 4 -latency 500ms
.\bin\windnd.exe -edge 4
```

Flags:

- `-edge <px>` thickness of the edge strip (default 2).
- `-rate <MiB/s>` per-stream throughput of the drag source (default 16; 0 = unthrottled).
- `-latency <duration>` delay before the first byte of every drag-source stream (default 200ms).

The startup line shows the strip rectangle (physical pixels, the process is Per-Monitor-v2 DPI aware) and the number of
syscall callbacks created. Two windows exist: the strip (invisible) and the titled source window.

## What to do and observe

Prepare a scratch folder `C:\Temp\dnd` with a few files, a sub-folder with files, and a `.zip` (Explorer shows the
content of a zip folder as virtual files). Open a second Explorer window on `C:\Temp\dnd-out` as the drop target for
step 5.

1. Edge target, plain files. Drag two files from `C:\Temp\dnd` to the right edge of the primary monitor, hold there for a
   second, and release (try also: move away without releasing).
   - Is there a `DragEnter` line, with `inside_edge=true`, and a `formats` list? Which formats does Explorer offer
     (`CF_HDROP`, `Shell IDList Array`, `FileGroupDescriptorW`, ...), with which `tymed`?
   - Are the `files` paths exactly the dragged files? Does `FileGroupDescriptorW` show up for plain files?
   - On release: a `Drop` line with the same payload. On moving away: a `DragLeave` line. What does the cursor show over
     the strip (copy plus sign)? How many `DragOver` calls were counted?
   - With Ctrl, Shift and Ctrl+Shift held: the `DragOver state changed` lines show the keys and the effect OLE would
     request; the returned effect stays `COPY`.
2. Edge target, folders, zip and virtual sources. Drag a folder, a multi-selection and files from inside the `.zip`.
   Record which formats and paths arrive (a zip entry is a virtual file: `FileGroupDescriptorW` and `FileContents`).
3. Click-through. With the program idle, click, hold and drag in applications at the right screen edge (for example a
   maximised window's close button or scroll bar at the edge, the desktop, a snap gesture at the edge) and confirm the
   strip does not intercept anything. Then press and hold the left button elsewhere and move to the edge: the
   `edge click-through changed` lines show `click_through=false` on the press and `true` on the release.
   - Does a normal (non-drag) press-and-hold at the edge behave differently from before the program ran?
   - After every gesture the program prints `left button gesture finished` with `crossed_edge_while_pressed` and
     `dragenter_fired`. Repeat the Explorer drag from step 1 at least 20 times (varying speed and approach direction)
     and count the gestures with `crossed_edge_while_pressed=true` and `dragenter_fired=false`: that is the DragEnter
     reliability result. The warning line is printed for each such case.
4. Drag source to the desktop. Press the left button in the source window, keep it held, move onto the desktop and
   release. Record: the `IDataObject call` lines (which `GetData` format, `lindex`, `tymed` and `aspect`, in which order),
   the `stream created` / `stream released` lines, `GiveFeedback effect changed` lines, `DoDragDrop returned`
   (`hresult`, `outcome`), the `Performed DropEffect` `SetData`, the files that appear on the desktop (names, sizes,
   times), Explorer's progress dialog, and the final `file accounting` / `drop outcomes` lines after Ctrl+C.
5. Drag source to an Explorer folder. Same as step 4 with the drop on `C:\Temp\dnd-out`. Try `-rate 4` and press Cancel
   in Explorer's dialog after about 10 s: do reads stop at once, and what does `stream released` report
   (`completed=false`)?
6. Drag source cancel and misses. Start a drag and press Escape (expect `QueryContinueDrag decision=cancel` and
   `outcome=cancelled`), release over a window that accepts nothing, and release inside the source window.
7. Drag source onto the own edge strip. Start a drag in the source window and release on the right edge strip. The strip
   is a drop target in the same process on the same STA thread, so this exercises `DoDragDrop` calling `IDropTarget` and
   `IDataObject` re-entrantly. Record the `DragEnter`/`Drop` lines (the payload shows `FileGroupDescriptorW` with three
   entries and no `CF_HDROP`) and whether anything hangs.
8. Elevated Explorer (UIPI). Start File Explorer elevated (run a File Explorer instance as administrator, or use an
   elevated file manager), keep `windnd.exe` unelevated, and repeat steps 1, 4 and 5 using the elevated window as drag
   source and as drop target.
   - Elevated source onto the strip: expect the gesture summary with `crossed_edge_while_pressed=true` and
     `dragenter_fired=false`. Does the cursor show the prohibited sign?
   - Source window onto an elevated Explorer folder: does the drop work, or does the cursor show the prohibited sign?
   - Then start `windnd.exe` from an elevated prompt and repeat: does the strip receive drags from an unelevated
     Explorer, and from the elevated one?

## Paste back

For every numbered step: the complete console output of the run from the first line to the summary lines printed after
Ctrl+C (`edge target summary`, `hook summary`, `drag source session`, `file accounting`, `drop outcomes`), plus what
Explorer and the cursor showed (copy dialog, progress, prohibited sign, resulting files). For step 3 the count of
gestures with and without `DragEnter`. For step 8 which of the four elevation combinations worked. Also paste the Windows
version (`winver`), the monitor layout and DPI scaling, and whether `windnd.exe` was elevated.
