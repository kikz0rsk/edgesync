# Smoke tests

Fast end-to-end checks run by the user after every phase exit from P2 onward (and before any release). Each
line is a pass/fail observation; record results in `qa/manual/results/smoke-<phase>-<date>.md` using the results
format from `qa/manual/README.md`. Steps that a phase has not delivered yet are marked "n/a".

## Setup
- Windows PC 1 = server, Windows PC 2 = client, GNOME box = second client (from P2 Y1 on).
- Fresh install of the build under test (P5: the setup installer; earlier: binaries from `task build`, run by the user).

## Checklist

| # | Check | Expected |
|---|---|---|
| S-01 | Start agent on all machines | tray icon appears (GNOME: with AppIndicator extension), no error notification |
| S-02 | Discovery | each machine lists the others within 3 s (PB-13) |
| S-03 | Pair PC 1 with PC 2 using the 6-digit code | both show "paired"; wrong code 3 times invalidates the code |
| S-04 | Arrange PC 2 to the right of PC 1 in the layout editor, apply | client shows the layout read-only |
| S-05 | Move the cursor across the right edge of PC 1 | cursor appears on PC 2 at the matching height; switch indicator shows "PC 2" on PC 2 |
| S-06 | Type, click, scroll (both axes), side buttons on PC 2 | all delivered; no stuck keys after returning |
| S-07 | Return across the left edge of PC 2 | control returns to PC 1 at the matching height |
| S-08 | Each guard enabled one at a time (dwell, double-tap, corner, modifier) | switching follows the guard; disabling restores immediate switching |
| S-09 | Hotkeys: lock to screen, jump to PC 2, cycle, panic; Ctrl+Alt+End (Send Ctrl+Alt+Del, D-113) while a Windows client with the service is active | each works; panic releases held keys; the client shows the Ctrl+Alt+Del screen |
| S-10 | Copy text, HTML (from a browser), RTF (from WordPad/Word), an image and a password from a password manager on PC 1; paste on PC 2 | content and formatting arrive (the password too, D-121); nothing transfers until paste |
| S-11 | Copy a file and a folder in Explorer on PC 1; paste in Explorer on PC 2 | files arrive with Explorer progress; folder structure and timestamps preserved |
| S-12 | Drag a file from PC 1's Explorer across the edge and drop on PC 2's desktop | file appears on PC 2's desktop |
| S-13 | Send a file via the GUI to PC 2 | appears in `Downloads\EdgeSync`; second send of the same name gets " (1)" |
| S-14 | Cancel a large transfer mid-way; retry it | cancel stops it; retry resumes from the last verified chunk |
| S-15 | Lock PC 1 (Win+L) | PC 2 locks; unlocking PC 1 wakes PC 2's display only |
| S-16 | UAC prompt on PC 2 while controlling it | prompt can be answered with the shared mouse/keyboard |
| S-17 | Ctrl+Alt+Del to PC 2 via the menu/hotkey | secure attention screen appears on PC 2 |
| S-18 | Pull PC 2's network cable while it is active | control returns to PC 1 within 3 s; reconnects within 2 s after replug |
| S-19 | Put PC 2 to sleep; move to its edge | Wake-on-LAN packet sent; PC 2 wakes if its NIC supports WoL; edge is a wall until reconnected |
| S-20 | GNOME box: repeat S-05..S-11 with the GNOME box as client | works; portal consent dialog appears once per session unless restore tokens are supported |
| S-21 | Export diagnostics bundle | zip contains logs, redacted config, system info; no clipboard content, no keys |
| S-22 | Quit GUI; agent keeps working; quit agent from tray | sharing continues after GUI quit; agent quit stops sharing and releases all input |
