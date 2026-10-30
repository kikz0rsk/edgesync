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

## golangci-lint v2.15.0 (P0-F-001) - 2026-10-30

- Source: official release binary `golangci-lint-2.15.0-windows-amd64.zip` from `https://github.com/golangci/golangci-lint/releases/download/v2.15.0/`.
- SHA-256 of the downloaded zip: `CD7EA6063C95DE245E17E5246E42BDC14BB221792C429DBD239AB3A6ADE6CA6A`
- Checksum file `golangci-lint-2.15.0-checksums.txt` line for the zip: `cd7ea6063c95de245e17e5246e42bdc14bb221792c429dbd239ab3a6ade6ca6a  golangci-lint-2.15.0-windows-amd64.zip`. Result: match.
- Install path: `C:\Users\kikz0r\go\bin\golangci-lint.exe` (directory created; `golangci-lint.exe` copied from the archive folder `golangci-lint-2.15.0-windows-amd64\`).
- `golangci-lint version` (absolute path `C:\Users\kikz0r\go\bin\golangci-lint.exe`), output verbatim:

  ```
  golangci-lint has version 2.15.0 built with go1.27.0 from 25c5369a on 2026-10-16T11:03:09Z
  ```

- PATH: `C:\Users\kikz0r\go\bin` is on the User PATH (registry). It is not on the Machine PATH. A fresh PowerShell session with Machine+User PATH reloaded resolves `golangci-lint` to `C:\Users\kikz0r\go\bin\golangci-lint.exe`. No PATH was edited.

## Cap'n Proto 1.4.0 via vcpkg (P0-F-002) - 2026-10-30

- MSVC check: `"${env:ProgramFiles(x86)}\Microsoft Visual Studio\Installer\vswhere.exe" -products * -latest -requires Microsoft.VisualStudio.Component.VC.Tools.x86.x64 -property installationPath` returned `C:\Program Files\Microsoft Visual Studio\18\Community` (exit 0).
- vcpkg port `D:\Development\vcpkg\ports\capnproto\vcpkg.json` declares version `1.4.0`.
- Command: `D:\Development\vcpkg\vcpkg.exe install capnproto:x64-windows` (completed, "All requested installations completed successfully"; no retry needed).
- Absolute path of `capnp.exe`: `D:\Development\vcpkg\installed\x64-windows\tools\capnproto\capnp.exe`
- `capnp --version` output, verbatim:

  ```
  Cap'n Proto version 1.4.0
  ```

- Companion compilers beside it in the same directory: `capnpc-c++.exe`, `capnpc-capnp.exe`. Directory: `D:\Development\vcpkg\installed\x64-windows\tools\capnproto\`.
