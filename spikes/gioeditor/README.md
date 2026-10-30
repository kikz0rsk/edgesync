# gioeditor - Gio layout-editor proof (P0-SP5-001)

## Purpose
Prove that the machine-layout editor of D-33 / D-51 is feasible on Gio `gioui.org` v0.10.3 with `CGO_ENABLED=0` on
Windows: machines as rigid groups of monitor rectangles, drag with edge snapping (12 canvas logical units), overlap
highlight of facing real edges, Ctrl+wheel zoom around the cursor, middle-drag pan, arrow-key nudge, light/dark palette
and a frame-time log with up to 64 synthetic machines. The result feeds the P3 editor design and `briefs/spikes/sp5.md`.

Files:
- `snap.go` - pure model (no Gio import): `Snap`, `Overlap`, real-edge computation, machines, `View` zoom/pan math.
- `snap_test.go` - host-safe unit tests of the pure model.
- `canvas.go` - Gio canvas: pointer, gesture, key handling and drawing.
- `main.go` - `app.Window` event loop, `-machines` flag, frame-time log.

## Build
From the repository root (Windows host, no C toolchain needed):

```
go -C spikes build -o bin/gioeditor.exe ./gioeditor
```

`CGO_ENABLED` defaults to 0 on this host; Gio renders through Direct3D 11 on Windows (`app/d3d11_windows.go` in the
module). On Linux and macOS Gio needs cgo and platform dev packages (https://gioui.org/doc/install), so this spike is
excluded from the cross-compiled vet gates.

Host-safe tests (pure logic only):

```
go -C spikes test ./gioeditor/...
```

## Run
```
spikes\bin\gioeditor.exe
spikes\bin\gioeditor.exe -machines 64
```

`-machines N` generates N synthetic machines (default 4), each with 2 or 3 monitors, laid out on a grid with gaps larger
than the snap threshold so nothing snaps at start.

## Controls
| Input | Action |
|---|---|
| Left drag on a machine | Move the whole monitor group, snapping to facing real edges of other machines within 12 canvas units |
| Left click on a machine / on empty canvas | Select (raised to the top) / deselect |
| Ctrl + wheel | Zoom around the cursor |
| Middle-button drag | Pan |
| Arrow keys | Nudge the selected machine by 1 canvas unit, Shift = 10 (no snapping) |
| `T` | Toggle light/dark palette |

Orange lines are snap guides shown while dragging. Green thick lines are the overlap of facing real edges between the
selected machine and every other machine (the segment the cursor may cross, D-51). Monitor sides shared with another
monitor of the same machine are not real edges and never snap or highlight.

## What to observe
1. Drag a machine next to another: does it feel natural when the edge snaps (12 canvas units)? The threshold is in
   canvas logical units as in D-51, so at low zoom it is only a few screen pixels. Note whether it should instead be a
   constant screen distance.
2. Drag a 3-monitor L-shaped machine: only outer edges snap and highlight, never the seams between its own monitors.
3. Overlap highlight appears only where facing real edges share a range, including partial overlaps.
4. Ctrl+wheel zooms toward the cursor without the point under the cursor drifting; middle-drag pans, including when the
   pointer starts over a machine.
5. Arrow keys nudge exactly 1 unit, Shift+arrow 10 units; the status line shows the machine position.
6. `T` switches palette with every label readable.
7. With `-machines 64`: drag, zoom and pan stay smooth. The frame log prints one line per second in which frames were
   rendered: `machines=64 frames=<n> fps=<f> frame_avg=<ms> frame_max=<ms>`. `frame_*` is the time from receiving the
   frame event to `Frame` returning (ops build plus submit and present). Gio redraws only on events, so keep dragging or
   panning for several seconds; an idle window prints nothing.

## Gio v0.10.3 APIs relied on (checked against the module source)
- `app.Window.Event() event.Event`, `app.FrameEvent{Now, Metric, Size, Insets, Frame, Source}`,
  `app.NewContext(ops *op.Ops, e FrameEvent) layout.Context`, `app.DestroyEvent{Err}`, `app.Title`, `app.Size`,
  `app.Main`.
- `gesture.Drag`: `Add(ops *op.Ops)`, `Update(cfg unit.Metric, q input.Source, axis Axis) (pointer.Event, bool)`; it only
  accepts primary-button presses and grabs the pointer after the touch slop.
- `pointer.Filter{Target, Kinds, ScrollX, ScrollY}`, `pointer.Press|Drag|Release|Cancel|Scroll`,
  `pointer.Event{Position, Buttons, Scroll, Modifiers}`, `pointer.CursorGrab.Add(ops)`.
- `key.Filter{Name, Optional}`, `key.Event{Name, Modifiers, State}`, `key.NameLeftArrow` and friends; key filters with no
  `Focus` match regardless of focus.
- `event.Op(ops, tag)`, `clip.Rect(r).Push(ops)`, `paint.FillShape`, `material.Label`, `text.NewShaper` with
  `gofont.Collection()`.

## Design notes for P3
- One backdrop handler (tag on a window-sized clip area) plus one `gesture.Drag` tag per machine, registered inside the
  backdrop's clip scope. Machine areas are non-pass siblings: the topmost machine blocks machines below it, while the
  backdrop handler still receives the same pointer events, so middle-drag pan and Ctrl+wheel zoom work over machines.
- No `op.Offset` is used for hit areas, so `pointer.Event.Position` is in window coordinates for both handlers.
- Drag position is `start + (pointer - pressPointer) / zoom`, recomputed from the raw pointer on every event and then
  snapped, so snapping never accumulates drift and is idempotent when several events arrive in one frame.
- Hit areas of a multi-monitor machine are one `clip.Rect` per monitor under the same tag, so empty bounding-box corners
  of L-shaped machines are not grabbable.

## Paste back
- Stdout of `gioeditor.exe -machines 4` and `gioeditor.exe -machines 64` after dragging and zooming for about 10 seconds.
- Windows display scaling (percent), GPU and CPU model.
- Whether the window opened on a machine without a C toolchain.
- Subjective notes for each item under "What to observe", and anything that behaved unexpectedly.
