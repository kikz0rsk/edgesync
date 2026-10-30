# wininject - Windows injection, DPI manifest and real edges (P0-SP1-002)

## Purpose

Decide whether D-49 (absolute injection), D-51 and D-89 (per-monitor DPI and real edges in native physical pixels) hold
on Windows:

- the process declares Per-Monitor-v2 DPI awareness through the embedded manifest (`manifest.xml`, compiled into
  `resource_windows_amd64.syso`), not through code
- monitor list with physical bounds, work area, DPI and scale, and the real edges computed from them (monitor sides not
  shared with another monitor, split where a side is partly shared)
- absolute `SendInput` moves (`MOUSEEVENTF_MOVE|ABSOLUTE|VIRTUALDESK`, normalised to 0..65535 against the virtual
  desktop) to the inset corners and centre of every monitor, with the `GetCursorPos` read-back error in px
- scancode typing (`KEYEVENTF_SCANCODE`, `KEYEVENTF_EXTENDEDKEY` for E0 keys) into Notepad
- wheel, horizontal wheel, X1 and X2 buttons
- per-call `SendInput` latency

The pure logic (real edges, normalization, probe targets) is covered by host tests in `edges_test.go`.

## Build

From the repository root:

```
go -C spikes build -o bin/wininject.exe ./wininject
```

The resource file is regenerated only when the manifest or version info changes, from the repository root:

```
go tool -modfile=tools/tools.mod goversioninfo -64 -o spikes/wininject/resource_windows_amd64.syso spikes/wininject/versioninfo.json
```

## Run

Run from `spikes\`. Ctrl+C aborts the run at any point; otherwise it ends by itself after the wheel and X-button phase.

```
.\bin\wininject.exe -countdown 5s
```

Steps during the run:

1. The monitor and DPI section prints. Nothing is focused yet, the pointer moves to every probe in turn (about 50 ms
   settle per probe). Do not touch the mouse during this phase.
2. The countdown starts. Open Notepad, maximise it on the primary monitor and click into the text area, then keep the
   pointer over the text area.
3. The scancode pattern is typed: `edgesync 1234567890`, numpad Enter, Right Ctrl, Left, Right, Up, Down, Enter.
4. The wheel and X-button phase runs with the pointer where the last step left it. Keep Notepad under the pointer; the
   X buttons may trigger browser or Explorer navigation in other windows.

## What to observe

- `process DPI awareness: 2` and `thread is DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2: true`. If the thread is not
  Per-Monitor-v2 the manifest did not apply and the other numbers are not meaningful.
- `virtual desktop (GetSystemMetrics, used for normalization)` against `bounding box of monitors (GetMonitorInfoW)`.
  They must match. A mismatch means `GetSystemMetrics` returns system-DPI-scaled values for this process (the API is
  documented as not DPI aware); the absolute errors below will show it.
- `real edge` lines: a shared side must not appear; a gap between monitors must appear as a real edge on both sides.
- `abs ... error=(dx,dy)px`: expected 0,0 on every probe (at most 1 px per axis is acceptable from rounding of the
  0..65535 mapping). Corners and centre of every monitor must land on their monitor, and the `worst |error|` summary
  line is the headline number.
- `key ...` lines: every key in the pattern reaches Notepad. The text must read `edgesync 1234567890` followed by a new
  line; the arrows move the caret, Right Ctrl does nothing visible.
- `wheel and buttons`: the wheel scrolls the window under the pointer by one notch and by a 30-unit fine step in each
  direction; X buttons are delivered to the window under the pointer.
- `SendInput latency` min / mean / max per run, for the probes, the key taps and the wheel calls.

## Paste back

Paste the complete console output of the run, and also:

- Windows version (`winver`), number of monitors, their resolutions and scale factors, and whether the primary monitor
  is not at the top-left of the virtual desktop
- the output of `reg query "HKCU\Control Panel\Desktop" /v LogPixels` if available
- anything unexpected: pointer landing off the target monitor, keys missing in Notepad, a run that exits with an error
  line on stderr (paste the exact error)
