# EdgeSync - Compatibility matrix

All versions verified on 2026-10-29 against proxy.golang.org (`@latest`, tag time), GitHub releases, nuget, or
local commands. Bump rule at the end.

## Toolchain

| Component | Version | Verified how / date | Notes |
|---|---|---|---|
| Go | go1.27.3 | `go version` on dev host; go.dev release 2026-10-19 | `go.mod`: `go 1.27` (D-01) |
| C compiler for cgo (dev host) | msys2 ucrt64 gcc, mingw-w64 14.0 headers | file inspection (RGE 1.2) | `CC=D:\Development\msys2\ucrt64\bin\gcc.exe`; needed for `-race` and GUI on macOS/Linux only |
| go-task | 3.48.0 | `task --version` | D-53 |
| golangci-lint | v2.15.0 | proxy tag 2026-10-16 | official binary (D-54); bundles staticcheck 2026.2.2 |
| gofumpt | v0.12.0 | proxy tag 2026-09-07 | via golangci-lint formatters and `tools/tools.mod` |
| govulncheck | golang.org/x/vuln v1.9.0 | proxy tag 2026-10-11 | `tools/tools.mod` |
| actionlint | v1.7.12 | proxy tag 2026-03-30 | `tools/tools.mod` |
| goversioninfo | v1.7.0 | proxy tag 2026-05-11 | `tools/tools.mod` (D-66) |
| capnpc-go | capnproto.org/go/capnp/v3 v3.1.0-alpha.2 | proxy tag 2025-10-24 | `tools/tools.mod` |
| Cap'n Proto compiler | 1.4.0 | vcpkg port `D:\Development\vcpkg\ports\capnproto` (baseline 1.4.0) | install in P0-F-002; CI uses Ubuntu's apt `capnproto` |
| Inno Setup | 7.1.1 | jrsoftware.org/isdl.php (`innosetup-7.1.1-x64.exe`, dated 2026-10-28) and read-only `winget show JRSoftware.InnoSetup.7` on 2026-10-30; installer SHA-256 `8a5a6711c3766a6e5fb74f280ba0e3551e719ce94538d901fdbd1f8f9920b3f0` | D-118; winget id `JRSoftware.InnoSetup.7` (the id `JRSoftware.InnoSetup` is Inno Setup 6, version 6.7.3, and is not this pin); installed explicitly in CI and locally by P0-F-003, the runner image's preinstalled 6.7.1 is not used; licence requests (does not require) a purchase from commercial users above USD 5000 annual revenue (R-37); no .NET SDK needed |
| GoReleaser (OSS) | v2.19.0 | proxy tag 2026-10-22 | P5 R2; Windows installers/app bundles/DMG are Pro-only, so the installer comes from Inno Setup (D-118) and the .app from a script |

## Go module dependencies (main module)

Added to `go.mod` only by the item that first needs them, at exactly these versions.

| Module | Version | Tag date | Licence | Used by |
|---|---|---|---|---|
| github.com/quic-go/quic-go | v0.64.1 | 2026-10-19 | MIT | `internal/transport` (requires go >= 1.26) |
| capnproto.org/go/capnp/v3 | v3.1.0-alpha.2 | 2025-10-24 | MIT | `internal/proto/capnp` |
| filippo.io/nistec | v0.0.4 | 2025-09-29 | BSD-3-Clause | `internal/spake2` |
| golang.org/x/sys | v0.49.0 | 2026-10-06 | BSD-3-Clause | Windows and unix backends |
| golang.org/x/net | v0.62.0 | 2026-10-27 | BSD-3-Clause | indirect (quic-go), `ipv4`/`ipv6` for mDNS/WoL if needed |
| golang.org/x/crypto | v0.58.0 | 2026-10-09 | BSD-3-Clause | indirect (quic-go) |
| github.com/ebitengine/purego | v0.11.2 | 2026-10-15 | Apache-2.0 | `internal/platform/darwin` |
| github.com/jezek/xgb | v1.3.1 | 2026-04-18 | BSD-3-Clause style | `internal/platform/x11` |
| github.com/godbus/dbus/v5 | v5.2.4 | 2026-10-13 | BSD-2-Clause style | `internal/platform/wayland`, `internal/platform/linux` |
| github.com/Microsoft/go-winio | v0.6.4 | 2026-10-13 | MIT | `internal/ipc` (Windows), `internal/service` |
| github.com/hashicorp/mdns | v1.0.7 | 2026-06-11 | MIT | `internal/discovery` (pending SP5 comparison) |
| github.com/brutella/dnssd | v1.2.14 | 2024-10-22 | MIT | spike comparison only unless SP5 selects it |
| github.com/pelletier/go-toml/v2 | v2.4.3 | 2026-07-05 | MIT | `internal/config`, `internal/peers`, `internal/i18n` (D-38) |
| github.com/google/go-cmp | v0.7.0 | 2025-01-14 | BSD-3-Clause | tests (D-56) |
| gioui.org | v0.10.3 | 2026-09-28 | MIT or Unlicense (dual) | `gui/`, `cmd/edgesync` (D-33) |
| gioui.org/x | v0.10.3 | 2026-09-28 | MIT or Unlicense (dual) | `gui/` (file picker via `explorer`) |
| golang.org/x/image | v0.47.0 | 2026-10-09 | BSD-3-Clause | `internal/clipboard/convert` (TIFF), X11 overlay text (D-85; added by P1-C4-006) |
| golang.org/x/text | v0.43.0 | 2026-10-09 | BSD-3-Clause | required by x/image; direct for `cases` + `unicode/norm` in `internal/transfer` (D-98; P1-C4-013) |

Licence column values for gioui.org are to be re-read from the module's LICENSE file by the first item that adds it.

## CI runners (D-24)

| Label | Image (2026-10-29, runner-images README) | Used for |
|---|---|---|
| `windows-latest` | Windows Server 2025 | build, test, race, Windows integration jobs, installer build and install lifecycle job (the image ships Inno Setup 6.7.1, which is not used, D-118) |
| `macos-latest` | macOS 26 arm64 | build, test, race, macOS integration jobs, .app bundle (universal via `lipo` with an amd64 cross-build) |
| `ubuntu-24.04` | Ubuntu 24.04 | lint, cross, gen-check, vuln, actionlint, X11 integration under Xvfb, fuzz campaigns |
| `windows-11-arm` | Windows 11 arm64 (runner-images README, checked 2026-10-30) | windows/arm64 build, test, installer build and install lifecycle job (D-104) |
| `macos-15-intel` | macOS 15 x64 | darwin/amd64 tests (the release bundle stays universal from `macos-latest`) (D-104) |
| `ubuntu-22.04` | Ubuntu 22.04 x64 | Linux amd64 release build (glibc floor 2.35) (D-104) |
| `ubuntu-22.04-arm` | Ubuntu 22.04 arm64 | Linux arm64 release build and tests (D-104) |

## OS test matrix (who verifies what)

| Platform | Versions in scope | Hand testing | Automated |
|---|---|---|---|
| Windows | 10 22H2, 11 | user's dev PC (Windows 11, build 26200) + second PC (version recorded at P0) | windows-latest (Server 2025) unit + integration |
| macOS | 13 .. 26 | none (R-07) | macos-latest (26) unit + integration; compile checks for amd64 |
| Linux X11 | any X.Org server with RandR 1.5, XTest, XFixes | Xephyr nested server on the GNOME box | ubuntu-24.04 + Xvfb integration |
| Linux Wayland GNOME | 45+ | user's GNOME box (version recorded at P0 SP3) | none (no headless portal stack in CI) |
| Linux Wayland KDE Plasma | 6.1+ | none (R-09) | none |

## Bump rule
1. Any version change (Go, a dependency, a tool, a runner label) requires a new dated entry in `decisions.md` naming old and new versions and the reason.
2. The item performing the bump updates this file in the same commit, runs `task ci:local`, and re-runs the affected integration tests on CI or the user's machines.
3. Security fixes (govulncheck findings, Go patch releases) are bumped promptly but follow the same rule.
4. Re-verification of all pins happens at P0 exit, P2 exit and P5 exit (plan.md section 4); newer versions found there are recorded as notes, not adopted silently.

## Notes (append-only)
- 2026-10-30: Inno Setup 7.1.1 pinned (D-118): version, release date, winget id and installer SHA-256 verified from jrsoftware.org/isdl.php and `winget show JRSoftware.InnoSetup.7`; the WiX pin of D-65 is superseded and removed from the toolchain. The licence statement (permissive, commercial use permitted, purchase requested above USD 5000 revenue) was verified by the user on 2026-10-30.