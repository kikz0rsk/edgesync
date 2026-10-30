# winhooks - Windows low-level hooks under GC pressure (P0-SP1-001)

## Purpose

Decide whether D-50 holds: `WH_MOUSE_LL` and `WH_KEYBOARD_LL` installed from Go on a dedicated locked OS thread,
with a non-allocating callback that pushes into a lock-free ring, a liveness watchdog that re-installs the hooks,
and Raw Input (`RIDEV_INPUTSINK` on a hidden message-only window) as the source of relative deltas.

The spike measures:

- callback duration p50 / p99 / max, with and without `-gcstress`
- events per second and ring drops (ring capacity 4096, consumer drains every 1 ms)
- watchdog behaviour: which marker kind the hooks observe (zero relative mouse move, F24 key-up) and how many
  re-installs happen (forced once by `-stall`)
- suppression toggled by Scroll Lock
- Raw Input delta sums against hook `pt` delta sums while the pointer pushes against each virtual screen edge
  (evidence for clamping of the hook coordinates)

The process declares Per-Monitor-v2 DPI awareness through `SetProcessDpiAwarenessContext` at start-up so hook
coordinates are physical pixels.

## Build

From the repository root:

```
go -C spikes build -o bin/winhooks.exe ./winhooks
```

## Run

While suppression is ON the keyboard and mouse are blocked system-wide. Scroll Lock itself is never blocked, press it
again to release. The run also ends by itself after `-duration` and releases suppression on exit; Ctrl+C works only
while suppression is OFF.

Run each command from `spikes\` so the output lands in `spikes\out\`:

```
mkdir out
.\bin\winhooks.exe -duration 60s -csv out\winhooks-baseline.csv
.\bin\winhooks.exe -duration 60s -gcstress -csv out\winhooks-gcstress.csv
.\bin\winhooks.exe -duration 30s -stall -csv out\winhooks-stall.csv
```

Flags: `-duration` (default 60s), `-gcstress` (8 goroutines allocating 64 MiB/s in total plus `runtime.GC()` every
100 ms), `-stall` (the first hook callback blocks 1500 ms once), `-csv <file>` (one row per second).

During every run:

1. Move the mouse continuously for about 10 s, scroll and click a few times, type a few keys.
2. Press Scroll Lock once: suppression turns ON (the per-second line shows `suppress=ON`, the pointer does not move
   on screen). Move the mouse for about 10 s, then press Scroll Lock again to turn it OFF.
3. Push the pointer hard against each edge of the virtual screen (left, right, top, bottom) for about 5 s each and
   keep moving the mouse in the pushing direction. Do the same once with suppression ON.
4. Leave the pointer and keyboard idle for the last 10 s (markers only).

For `-stall` the first event (the watchdog marker arrives within 1 s) blocks the callback for 1500 ms. Keep the mouse
moving afterwards and watch the `reinstalls` counter rise within about 3 s.

## What to observe

- Callback `p50` / `p99` / `max` in the summary; `max` with `-stall` shows the blocked callback, `max` with
  `-gcstress` shows the GC induced delay. The Windows default for `LowLevelHooksTimeout` is not known to the program,
  compare `max` against the registry value if it is set.
- `ring drops` must stay 0 (the consumer is the main goroutine and competes with the GC stress workers).
- `markers (mouse/key)`: which watchdog marker the hooks observed. A kind that stays 0 is not observable and must
  not be used as the liveness signal.
- `reinstalls`: 0 without `-stall`; at least 1 with `-stall`. `stale_unhooks` counts hooks the OS had already
  removed when the watchdog unhooked them (evidence of the silent removal).
- Delta table: `hook_dx/dy` against `raw_dx/raw_dy` for the rows `left`, `right`, `top`, `bottom`. Expected if the
  hook `pt` is clamped: the hook sums stay near 0 along the pushing axis while the raw sums keep growing.
- `raw input: non_relative` and `errors` must be 0 on a normal mouse (remote desktop or touch devices report
  absolute movement).

## Paste back

For each of the three runs paste the complete console output from the `virtual screen:` line to the end of the
`=== summary ===` table, plus the CSV file from `spikes\out\`. Also paste:

- the output of `reg query "HKCU\Control Panel\Desktop" /v LowLevelHooksTimeout` (it may report not found)
- Windows version (`winver`), mouse polling rate if known, and whether the machine has more than one monitor
- anything unexpected: pointer freezing, keyboard staying blocked, a hook that stopped delivering events
