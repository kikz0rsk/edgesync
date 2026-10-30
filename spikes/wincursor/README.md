# wincursor - cursor hide and park strategies while input is grabbed (P0-SP1-003)

## Warning

Read this before running anything.

- While suppression is on, the low-level hooks swallow every keyboard and pointer event system-wide. Scroll Lock is the
  only way out that this spike provides. Know where Scroll Lock is before you press it.
- Strategy 1 confines the pointer with `ClipCursor` to a 1x1 rectangle. `ClipCursor` is machine-wide state, not
  private to the process. If a run is killed (`taskkill /F`, crash, power loss), the pointer may stay pinned until the
  confinement is cleared. Recovery: run `.\bin\wincursor.exe -release` (clears the confinement and exits), or sign out.
  Microsoft Learn documents only that the cursor is a shared resource and that NULL releases the clip; it says nothing
  about process exit. Recording what happens is part of this spike.
- No strategy replaces a system cursor. The hidden pointer is a transparent cursor set on this spike's own window class
  and windows, and the windows die with the process. Nothing here changes the cursor scheme, so there is no cursor
  scheme to restore.

## Purpose

Find out which mechanism keeps the local pointer parked and invisible while the hook suppresses input (D-50, FEAT-034):

1. `ClipCursor` to a 1x1 rectangle at the primary monitor centre, plus `SetCursorPos` to that point. The pointer stays
   visible at the centre.
2. A 1x1 topmost tool window at the primary centre with `SetCapture`, using a transparent cursor class. The window
   re-centres the pointer on every `WM_MOUSEMOVE` it receives while captured.
3. One layered window per monitor, covering the monitor rectangle (`WS_EX_LAYERED|WS_EX_TOPMOST|WS_EX_TOOLWINDOW|
   WS_EX_NOACTIVATE`, `SetLayeredWindowAttributes` alpha 1). Each window sets a transparent cursor in `WM_SETCURSOR`
   and re-centres the pointer on the primary monitor on `WM_MOUSEMOVE`.

Suppression is the `internal/winhook` low-level hook (mouse and keyboard, Raw Input not used here). The parked pointer
position is the primary monitor centre computed from `EnumDisplayMonitors` (`GetSystemMetrics` is not DPI aware for a
Per-Monitor-v2 thread).

## Build

From the repository root:

```
go -C spikes build -o bin/wincursor.exe ./wincursor
```

## Run

Run from `spikes\`. Ctrl+C ends the run and releases everything.

```
.\bin\wincursor.exe -strategy 1 -duration 60s
```

Flags:

- `-strategy 1|2|3`: strategy selected at start (default 1).
- `-duration 60s`: stop after this long (default 0, run until Ctrl+C).
- `-release`: clear a `ClipCursor` confinement left by a killed run, then exit. Does not start the hook.

Keys while running:

- Scroll Lock: toggles suppression. Suppression on engages the selected strategy; off releases it.
- `1`, `2`, `3`: select strategy. While suppression is on, the switch releases the old strategy and engages the new one.

Each engage and release prints one line. On exit the spike prints the hook summary (events, drops, reinstalls, max
callback duration) and how many times each strategy was engaged.

## What to observe

Record each row for each strategy, with suppression on and the pointer moved against every screen edge:

| Context | Check |
|---|---|
| Normal app (Notepad) | Pointer visible? Position fixed at the primary centre? Clicks and typing swallowed? |
| Fullscreen video | Same checks; does the transparent window or the layered windows change focus or playback? |
| Task Manager (elevated) | Pointer invisible and parked over it? Not verified in advance; UIPI may affect it. Record what happens. |
| Second monitor | Does the pointer leave the primary centre when moved onto a secondary? (Strategy 1 must not; strategies 2 and 3 re-centre.) |

Also record:

- Scroll Lock off: is the pointer free immediately, and does the old strategy release (summary line `engaged ... time(s)`)?
- Switching 1 to 2 to 3 with keys while suppression is on: any visible flicker, leftover window or stuck confinement?
- Normal exit (Ctrl+C) and `taskkill /F` while engaged. After `taskkill /F`, does the pointer stay confined under
  strategy 1? Do the layered or capture windows disappear (expected, process-owned)? Run `-release` to confirm recovery.
- `SetConsoleCtrlHandler` note: the spike loads user32, so the handler is not called for logoff or shutdown events
  (Microsoft Learn, SetConsoleCtrlHandler remarks). Record whether a sign-out while engaged leaves the pointer confined.
- Whether the strategy 2 or 3 window ever takes focus from the foreground app (`WS_EX_NOACTIVATE` is set on strategy 3
  only; strategy 2 uses `SW_SHOWNOACTIVATE`).

## What this does not prove

- Real-hardware behaviour of a 1000 Hz mouse under each strategy (no latency or drift numbers are taken here).
- Behaviour under UIPI or the secure desktop (UAC prompts, lock screen); that belongs to P0-SP1-004.
- Whether `ClipCursor` survives `taskkill /F`; this spike records the result, it does not assume it.

## Paste back

- The engage and release lines, the summary block, and the table above filled in per strategy.
- Anything that required `-release` to recover.
