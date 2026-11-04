# P0-SP1 Windows spikes - results (orchestrator-run part, D-126)

Date: 2026-11-04. Host: the dev PC (Windows 11, two monitors, 96 dpi / 100 %, virtual screen x=-1920 y=0 w=3840 h=1080,
`\\.\DISPLAY1` left non-primary, `\\.\DISPLAY2` primary). Binaries built from `main` into the gitignored `spikes\bin`.
Raw outputs: `spikes\out\` (gitignored). Status: PARTIAL. The M01 item P0-SP1-M01 stays open; the section "Still needs you" lists the rest.

## What the orchestrator ran

### winhooks (low-level hooks, 25 s each, hooks fed by the user's own mouse plus the injected events of wininject and the watchdog heartbeats)

| Run | events | callback count | p50 | p99 | max | mean | ring drops | reinstalls | heartbeat seen (mouse/key) | hook vs raw pointer deltas |
|---|---|---|---|---|---|---|---|---|---|---|
| baseline | 1654 (66.4/s) | 1678 | 1 us | 425.8 us | 425.8 us | 7.1 us | 0 | 0 | 0 / 24 | equal (-1366, -32) |
| `-gcstress` | 819 (32.8/s) | 843 | 1 us | 256 us | 430.3 us | 11.9 us | 0 | 0 | 0 / 24 | equal (-130, 54) |
| `-stall` (one 1500 ms callback block) | 74 (3.0/s) | 97 | 64 us | 1.5001 s | 1.5001 s | 15.6 ms | 0 | 1 | 0 / 23 | no pointer motion in this run |

Observations:
- Callback duration is far below any plausible LowLevelHooksTimeout and the 5 ms latency budget; garbage-collection stress did not change it (p99 256 us versus 425.8 us in the baseline, same order of magnitude, no drops).
- The watchdog recovered a deliberately stalled callback with exactly one reinstall and no stale unhooks.
- The watchdog's zero-relative-move MOUSE heartbeat was never delivered to the hooks (0 of ~72), while its F24 key-up heartbeat was seen every second (24, 24, 23). Evidence for P0-SP1-007: use the keyboard heartbeat; the mouse heartbeat is not a usable liveness signal here.
- Hook pointer deltas (non-injected WM_MOUSEMOVE) equal the Raw Input deltas in both runs that had pointer motion. Edge-clamping was NOT exercised (no pushing against the screen edges), so the "hook stays near 0 on the pushing axis" question is still open.
- Not exercised: Scroll Lock suppression (input blocking), edge pushes, a run on a machine with `LowLevelHooksTimeout` changed.

### wininject (topology, DPI, absolute probes, scan-code typing, wheel, X buttons)

- Process DPI awareness 2 (per-monitor), thread context PER_MONITOR_AWARE_V2: true.
- Two monitors, both 96 dpi / 100 %. `GetSystemMetrics` virtual desktop equals the `GetMonitorInfoW` bounding box (-1920,0 3840x1080), so the 0..65535 virtual-desk normalisation is valid on this layout.
- Real edges computed: monitor 0 left/top/bottom, monitor 1 right/top/bottom; the shared seam at x=0 is correctly not an edge.
- Absolute probes at both monitors' corners and centres: worst error 1 px (read back with GetCursorPos).
- SendInput cost per call: mean about 0.26 ms (typing about 0.5 ms), quantised to Go's 0.5 ms time.Now step on Windows (D-123): treat as an upper bound, not a measurement.
- Whether the typed text, wheel and X-button events reached the intended window is NOT verified (see the incident below).

### winpipe (TLS 1.3 handshake whose private-key operation is done by a signing proxy over a user-owned named pipe)

- Same account: 10 of 10 handshakes succeeded, TLS version 0x304. Median handshake 0.516 ms with a local key versus 1.028 ms through the proxy (overhead 0.511 ms). Both medians sit on Go's coarse Windows clock steps (0.5 ms; D-123), so the overhead is indicative only. Server log: serving pipe `\\.\pipe\EdgeSyncWinpipeSpike` for the current user's SID, session 1; "client disconnected cleanly" per client.
- Wrong server SID (`-server-sid S-1-5-18`): refused by the client with `pipe server runs as another account: got <own SID>, want S-1-5-18` (exit 1), as designed.

## Incident during the runs (disclosed)

wininject sends keystrokes, wheel and mouse-button events to whichever window has focus. I opened Notepad with `Start-Process notepad.exe` and focused it with `AppActivate` on the launcher PID. On Windows 11 the launcher PID is not the real Notepad process, so focus was never verified. The new Notepad also restored an existing tab (a `.ovpn` file of yours). Afterwards Notepad's saved tab state was 62 bytes with no injected text, so nothing of mine was kept as an unsaved edit there, but the keystrokes (`edgesync 1234567890`, arrows, Enter, Escape, numpad Enter, Right Ctrl, wheel, XBUTTON1/XBUTTON2) may have landed in another window during three runs of about 30 seconds each. If any window changed unexpectedly around 18:26 to 18:29 local time, that was this. Rule added (D-126 limit (e), plan.md NOTE): injection runs only into a window whose handle I created and whose foreground state I verified with `GetForegroundWindow` before the first event, aborting otherwise.

## Still needs you (P0-SP1-M01)

1. winhooks: Scroll Lock suppression ON/OFF while moving the mouse, and pushing the pointer against all four virtual-screen edges (also with suppression ON) to see the pt-clamping evidence; optionally `-duration 60s` baseline with your hands on the mouse.
2. wincursor: strategies 1, 2, 3 (Scroll Lock toggles suppression; keys 1/2/3 switch strategy) over Notepad, a full-screen video and an elevated Task Manager; exit by Ctrl+C and after `taskkill /F` (then `-release`).
3. winsvc (SYSTEM service, helper, desktop following, SendSAS): elevated PowerShell, follow `spikes\winsvc\README.md`, including the WARNING, `-Inject`, the opt-in `-EnableSasPolicy`, recovery and `uninstall.ps1`.
4. winpipe: a second Windows account running `pipeclient.exe` must be refused at dial.
5. wininject: confirm on the screen that the typed text, wheel and X-button events reach a focused scratch window (the orchestrator will rerun this only with a verified foreground window if you ask).
