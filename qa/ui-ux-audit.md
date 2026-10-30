# UI/UX audit checklist (Gio GUI, tray, overlay)

Used by the UI/UX audit gate (plan.md 1.5) for P3-F, G1-G4, and the tray/overlay items of W1, X1, Y1, M1. The
reviewer (`code-reviewer`, `model: "sonnet"`) checks the code and any screenshots the user supplies; findings are
reported as `file:line - defect`. If a `ui-ux-pro-max` skill is available in the session it is invoked first and
its output is attached; otherwise this checklist is the audit.

## A. Structure and states
- [ ] Every screen has explicit loading, empty, error and success states; errors show the reason from the agent (never a blank view).
- [ ] The GUI never blocks its frame loop on IPC; all requests are asynchronous with visible progress for > 300 ms operations.
- [ ] Agent disconnected state is visible app-wide with a retry action.
- [ ] Destructive actions (unpair, cancel transfer, reset layout) ask for confirmation and say what will happen.

## B. Keyboard and focus
- [ ] Every interactive element is reachable with Tab / Shift+Tab in a logical order.
- [ ] Visible focus indicator with >= 3:1 contrast against its background in light and dark themes.
- [ ] Enter activates the default action; Escape closes dialogs and cancels drags in the layout editor.
- [ ] The layout editor supports selection by keyboard and arrow-key nudging (Shift = 10 px).
- [ ] The hotkey editor captures combos without triggering them (agent `CaptureHotkey`).

## C. Visual
- [ ] Text contrast >= 4.5:1 (normal) and >= 3:1 (large) in both themes; theme follows the OS (D-52) and switches live.
- [ ] Layout scales correctly at 100 %, 125 %, 150 %, 200 % DPI; no clipped text; hit targets >= 24x24 dp.
- [ ] Machine rectangles in the editor show the machine name, monitor outlines, connection state, and the active machine distinctly (not by colour alone).
- [ ] Overlap/snapping guides are visible in both themes.
- [ ] Icons for tray states (idle, connected, remote active, error) are distinguishable in light and dark taskbars/menu bars.

## D. Content and i18n
- [ ] No hard-coded user-facing strings; every string comes from `internal/i18n` (D-68); the key test passes.
- [ ] Strings are short, specific, sentence case; no em dashes or en dashes.
- [ ] Numbers, sizes (KiB/MiB/GiB), speeds and durations are formatted consistently.
- [ ] Error messages say what happened and what the user can do (e.g. "Accessibility permission missing - open System Settings").

## E. Flows
- [ ] First-run wizard: role -> pairing -> permissions -> firewall check -> done; each step skippable only where safe; resumable.
- [ ] Pairing: the code is large and readable; attempts left and remaining time are shown; failure explains the cause.
- [ ] Layout editor: apply/revert are explicit; unapplied changes are indicated; clients show the layout read-only with an explanation.
- [ ] Transfers: progress, speed, ETA, cancel, retry, open folder; finished items stay listed until cleared.
- [ ] Logs: level filter, live tail toggle, copy selection; diagnostics export shows where the file was written.

## F. Tray and overlay
- [ ] Tray menu items match D-18/FEAT-144 and are translated; checked state for lock-to-screen is correct.
- [ ] The overlay appears on the machine receiving control, does not take focus, does not intercept clicks, and disappears after 800 ms + 200 ms fade (D-57).
- [ ] On GNOME without the AppIndicator extension the user is told how to get a tray icon (wizard and docs).
