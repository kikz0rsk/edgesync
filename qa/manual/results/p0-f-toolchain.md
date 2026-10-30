# P0-F toolchain results

Recorded by the orchestrator from user-performed installs and read-only verification. Later P0-F items append sections.

## Inno Setup 7.1.1 (P0-F-003) - 2026-10-30

- Installed by the user, manually (not through winget), into `D:\Program Files\Inno Setup 7\`.
- `ISCC.exe` absolute path: `D:\Program Files\Inno Setup 7\ISCC.exe`
- `ISCC.exe --version` output: `7.1.1` (exit code 0)
- Uninstall registry entry: DisplayName `Inno Setup 7.1.1`, DisplayVersion `7.1.1`, InstallLocation `D:\Program Files\Inno Setup 7\`
- SHA-256 of the installed `ISCC.exe`: `E0F9F1DC32032650C6629F6DEE41EB6D20BA140E4E3C07C25D8A24F9BA781644`
- Scratch compile: a minimal `.iss` compiled successfully with ISCC (0.828 s) into the scratchpad directory; the produced setup was not run.
- Deviation from the item text: the item expected `winget install --id JRSoftware.InnoSetup.7`. The user installed it by hand, so the winget installer SHA-256 recorded in D-118 was not checked against the file the user ran. Version 7.1.1 is confirmed by `--version` and the registry. The path above is not on `PATH`; downstream items resolve ISCC from this file (P5-R1 CI installs and locates its own copy on the runner).
