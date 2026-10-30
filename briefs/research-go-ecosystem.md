# EdgeSync - Go ecosystem research brief

Research date: 2026-10-29. Dev host: Windows 11, msys2 ucrt64 gcc, Go not installed.

## 0. How to read this brief

- Version numbers and dates come from primary sources: the Go module proxy (`proxy.golang.org/<module>/@latest` returns the tag commit time), the GitHub releases/API, go.dev, pkg.go.dev and official docs. Source URLs are inline.
- "Tag time" means the git tag timestamp reported by the Go module proxy. "Release date" means the date on the project's own release page.
- The WebFetch tool summarises pages with a small model, so numbers quoted below were cross-checked against a second source (usually the module proxy JSON) wherever possible. Anything I could not confirm is marked **UNVERIFIED**.
- Sources that blocked fetching (HTTP 403): wails.io and v3.wails.io. For Wails v3 docs I used docs-preview.wails.io, the GitHub release notes and pkg.go.dev instead.
- The Fyne v2.8.2 date differs by source: the proxy tag time is 2026-10-07 (https://proxy.golang.org/fyne.io/fyne/v2/@latest) while the GitHub releases page shows "12 Oct" (https://github.com/fyne-io/fyne/releases). Treat the release as "early-to-mid October 2026".
- No em-dashes are used anywhere in this file by request.

---

## 1. Go toolchain

### 1.1 Latest stable and support policy

- Newest major: **Go 1.27.0, released 2026-08-19**. Latest patch: **go1.27.3, released 2026-10-19** (security fixes to `crypto/x509`). Source: https://go.dev/doc/devel/release
- Previous major: **go1.26.10, released 2026-10-19** (same `crypto/x509` fix). Go 1.26.0 shipped in February 2026 (exact day not stated on the release-notes page, https://go.dev/doc/go1.26 - UNVERIFIED day).
- go1.27.2 (2026-10-08) also carried security fixes to `go` command, `crypto/tls`, `html/template`, `net/http`, `net/textproto`, `os`. Source: https://go.dev/doc/devel/release
- Policy (verbatim from the release page): "Each major Go release is supported until there are two newer major releases." So 1.27 and 1.26 are supported; 1.25 (latest go1.25.14, 2026-08-19) left support when 1.27 shipped (my inference from the policy text; the page itself does not say it). Go 1.26 will leave support when Go 1.28 ships (inference; roughly Feb 2027 on the six-month cadence).
- Current stable per the download page: go1.27.3, with MSIs `go1.27.3.windows-amd64.msi`, `-386.msi`, `-arm64.msi`. Source: https://go.dev/dl/ and https://go.dev/dl/?mode=json (verify the SHA-256 on the download page before installing; I do not reproduce it here).
- Minimum OS requirements (https://go.dev/wiki/MinimumRequirements): Windows 10 / Server 2016 and newer for Go 1.21+; macOS 13 Ventura or newer for Go 1.27 (macOS 12 for 1.25/1.26); Linux kernel 3.2+ for Go 1.24+. Confirmed for 1.27 in https://go.dev/doc/go1.27 (macOS 13 default target, SDK 26.2.0).
- Toolchain management: the `go` line in go.mod is a strict minimum and the `toolchain` line a preferred toolchain; `GOTOOLCHAIN=auto` downloads newer toolchains on demand (verified against checksum DB). Source: https://go.dev/doc/toolchain

### 1.2 Recommended install on Windows

- **Official MSI from https://go.dev/dl/** (`go1.27.3.windows-amd64.msi`). Official instructions: run the MSI, installs to `Program Files`, then close and reopen shells so environment changes apply (https://go.dev/doc/install).
- winget id is **`GoLang.Go`** (`winget install --id=GoLang.Go -e`), per package listings (https://winpkg.pages.dev/apps/GoLang.Go) and the manifest tree `manifests/g/GoLang/Go` in https://github.com/microsoft/winget-pkgs. **Caveat observed on 2026-10-29:** the winget-pkgs directory listing (via https://api.github.com/repos/microsoft/winget-pkgs/contents/manifests/g/GoLang/Go) showed 1.27.0 as the highest 1.27 manifest and 1.26.7 as highest 1.26, i.e. winget lags go.dev (go1.27.3 and go1.26.10 contain a `crypto/x509` security fix). Prefer the MSI for the newest patch. (The listing may be a cached API view; recheck with `winget show GoLang.Go` before relying on it.)
- Local C toolchain check (read-only file inspection, nothing executed): `D:\Development\msys2\ucrt64\bin\gcc.exe` exists and `ucrt64\include\_mingw_mac.h` declares `__MINGW64_VERSION_MAJOR 14`. That satisfies Go's race-detector requirement of mingw-w64 runtime v8 or later (https://go.dev/doc/articles/race_detector). `D:\Development\msys2\ucrt64\bin` must be on PATH for cgo.

### 1.3 Language and stdlib features that matter for EdgeSync

| Feature | Since | Relevance | Source |
|---|---|---|---|
| `log/slog` structured logging | 1.21 | Use as the only logger; 1.26 adds `slog.NewMultiHandler` (file + console + UI ring buffer) | https://go.dev/doc/go1.21 , https://go.dev/doc/go1.26 |
| `slices`, `maps`, `min`/`max` | 1.21 | General | https://go.dev/doc/go1.21 |
| Range-over-func iterators, `iter` package, `unique` | 1.23 | Iterate peers/screens/frames without allocating slices | https://go.dev/doc/go1.23 |
| Timer/Ticker channels unbuffered and GC-able | 1.23 | Safer edge-dwell timers (applies when go.mod says 1.23+) | https://go.dev/doc/go1.23 |
| `os.Root` / `os.OpenRoot` (directory-confined FS ops, rejects symlink escape) | 1.24 | **Important for file transfer**: confine received files to the destination folder and defeat path traversal | https://go.dev/doc/go1.24 |
| `tool` directive in go.mod, `go get -tool` | 1.24 | Pin dev tools (govulncheck, mockgen) without tools.go | https://go.dev/doc/go1.24 |
| `testing.B.Loop` | 1.24 | Correct benchmarks for codec comparisons | https://go.dev/doc/go1.24 |
| `weak` package, `runtime.AddCleanup` | 1.24 | Caches (icons, thumbnails) | https://go.dev/doc/go1.24 |
| `testing/synctest` **GA** (`synctest.Test`, `Wait`) | 1.25 (experiment in 1.24) | Deterministic tests of timers/edge-dwell/keepalive logic; 1.27 adds `synctest.Sleep` | https://go.dev/doc/go1.25 , https://pkg.go.dev/testing/synctest |
| `sync.WaitGroup.Go`, container-aware GOMAXPROCS | 1.25 | Convenience | https://go.dev/doc/go1.25 |
| Windows: `os.NewFile` supports overlapped handles; `net.FileConn` etc. implemented | 1.25 | Named-pipe/async handle use | https://go.dev/doc/go1.25 |
| Green Tea GC default, ~30% cheaper cgo calls, `errors.AsType`, `new(expr)`, `crypto/hpke`, `T.ArtifactDir` | 1.26 | cgo call cost matters for per-event native calls; lower GC overhead | https://go.dev/doc/go1.26 |
| Generic methods, `encoding/json` backed by v2, `testing/synctest.Sleep`, goroutine leak profile GA | 1.27 | Leak profile useful for connection goroutines | https://go.dev/doc/go1.27 |
| Windows 32-bit ARM port removed | 1.26 | None | https://go.dev/doc/go1.26 |

Notes:
- `testing/synctest` treats network I/O as **not** durably blocking and recommends in-memory fakes (e.g. `net.Pipe`); do not expect to test real quic-go sockets inside a bubble (https://pkg.go.dev/testing/synctest).
- Go 1.27 changes `crypto/tls` (`Config.Rand` deprecated, ML-KEM-1024 support) and `crypto/x509` (SystemCertPool honours `SSL_CERT_FILE` on Windows/Darwin) - not relevant to pinned self-signed certs but good to know (https://go.dev/doc/go1.27).
- Dependencies set the Go floor: **quic-go v0.64.1 requires `go 1.26.0`** (its go.mod, https://raw.githubusercontent.com/quic-go/quic-go/v0.64.1/go.mod), **Wails v3.0.0 requires `go 1.26.0`** (https://raw.githubusercontent.com/wailsapp/wails/v3.0.0/v3/go.mod), Fyne v2.8.2 `go 1.22.0`, Gio v0.10.3 `go 1.24.0`.

---

## 2. GUI frameworks

### 2.1 Fyne (`fyne.io/fyne/v2`)

- **Version:** v2.8.2, tag time 2026-10-07 (https://proxy.golang.org/fyne.io/fyne/v2/@latest); GitHub shows it as Latest. Cadence over the last year: 2.7.0 (16 Oct 2025), 2.7.1 (14 Nov), 2.7.2 (06 Jan), 2.7.3 (21 Feb), 2.7.4 (12 May), 2.8.0 (11 Jul), 2.8.1 (26 Aug), 2.8.2 (12 Oct) per https://github.com/fyne-io/fyne/releases (years inferred; the page shows day and month only). Active, roughly one release per 6 to 10 weeks.
- **Licence:** BSD-3-Clause (https://pkg.go.dev/fyne.io/fyne/v2). **Go floor:** 1.22.0 (go.mod, https://raw.githubusercontent.com/fyne-io/fyne/v2.8.2/go.mod).
- **v2.8.0 highlights** (https://github.com/fyne-io/fyne/releases/tag/v2.8.0): described as the biggest release since 2.0; Wayland supported by default and auto-selected at runtime; drops Windows 7/8 and macOS 10.14 and earlier; new canvas objects (ArbitraryPolygon, BezierCurve, Ellipse, Shader...), hardware shadows, scheduled notifications API, secondary-monitor support, always-on-top and window-position requests; breaking renames (`Polygon` -> `RegularPolygon`), popups lose default padding. Accessibility support exists but is off by default (`-tags accessibility`).
- **Threading model (important):** since v2.6.0 (10 April 2025) all Fyne callbacks run on one goroutine and background goroutines must use **`fyne.Do`** / **`fyne.DoAndWait`** to touch UI (https://github.com/fyne-io/fyne/releases/tag/v2.6.0, https://pkg.go.dev/fyne.io/fyne/v2). v2.8.0 prints a warning on launch for apps that have not migrated. Every network/transfer-progress callback must therefore marshal through `fyne.Do`. Canvas docs confirm: "use the fyne.Do function to queue the updates safely on the main goroutine" (https://docs.fyne.io/explore/canvas/).
- **cgo / OpenGL / OS requirements:** needs Go plus **a C compiler** and a system graphics driver on every desktop OS (https://docs.fyne.io/started/quick/). Windows: MSYS2 MinGW-64 toolchain recommended (you already have ucrt64 gcc). macOS: Xcode command line tools. Linux: `gcc libgl1-mesa-dev xorg-dev libwayland-dev libxkbcommon-dev` (Debian/Ubuntu). go.mod depends on `github.com/go-gl/glfw/v3.4/glfw` and `go-gl/gl` (cgo-based GLFW/OpenGL). A `CGO_ENABLED=0` desktop build is not documented anywhere I found (UNVERIFIED; all sources treat cgo as required).
- **System tray:** built in since v2.2.0 via `desktop.App` (`SetSystemTrayMenu`, `SetSystemTrayIcon`, plus `SetSystemTrayWindow` since 2.7 for left-click windows) on Windows, macOS and Linux (https://docs.fyne.io/explore/systray/, https://pkg.go.dev/fyne.io/fyne/v2/driver/desktop). Implemented with `fyne.io/systray` (module v1.12.2, tag 2026-06-09, https://proxy.golang.org/fyne.io/systray/@latest; fyne's go.mod pins a v1.12.3 pre-release). Linux uses D-Bus StatusNotifierItem; older tray hosts need a proxy such as snixembed; macOS needs an app bundle (https://github.com/fyne-io/systray). Recent tray fixes: 2.7.3 Snap sandbox, 2.7.4 various, 2.8.1 restored `MenuItem.Shortcut` and fixed a hidden "SystrayMonitor" window on KDE Wayland (releases page above). Pattern: `w.SetCloseIntercept(func(){ w.Hide() })` to keep the app running in the tray.
- **Custom drag-and-drop canvas (layout editor):** feasible. `fyne.Draggable` (`Dragged(*DragEvent)`, `DragEnd()`), `desktop.Mouseable`/`Hoverable` interfaces, `canvas.Rectangle`/`Text`/`Line`, and `container.NewWithoutLayout` for manual positioning (https://pkg.go.dev/fyne.io/fyne/v2, https://pkg.go.dev/fyne.io/fyne/v2/driver/desktop). Custom widgets implement `fyne.Widget` + `WidgetRenderer` (https://docs.fyne.io/extend/custom-widget/). Snap logic is plain Go in `Dragged`. Resize cursors (`NESWResizeCursor`, `NWSEResizeCursor`) arrived in 2.8. `Window.SetOnDropped` exists for OS file drops (fixed a stale-position bug in 2.8.2; release notes above).
- **Headless UI tests:** `fyne.io/fyne/v2/test` provides a virtual driver (`NewApp`, `NewWindow`), `Tap`, `Drag`, and golden-image/markup assertions under `testdata` (https://pkg.go.dev/fyne.io/fyne/v2/test). Good fit for testing the layout editor without a screen.
- **Theming / dark mode:** `theme.DefaultTheme()` "can adapt to the user preference of light or dark colors"; custom themes implement `Color/Font/Icon/Size` with a variant hint (https://pkg.go.dev/fyne.io/fyne/v2/theme, https://docs.fyne.io/extend/custom-theme/). Widgets are Fyne-drawn (non-native look).
- **Clipboard:** `fyne.Clipboard` is text only (`Content()`, `SetContent()`); file clipboard needs native code (https://pkg.go.dev/fyne.io/fyne/v2).
- **Packaging:** `fyne.io/tools/cmd/fyne` (module v1.7.4, tag 2026-10-19, https://proxy.golang.org/fyne.io/tools/@latest). `fyne package -os windows -icon x.png` -> exe with embedded icon/metadata; `-os darwin` -> `.app`; `-os linux` -> `.tar.gz` (not tar.xz) with `usr/local` layout; `-release` strips debug and reduces footprint (https://docs.fyne.io/started/packaging/). The page does not cover signing, notarization or MSI. `fyne-cross` (Docker) cross-builds to Windows/Linux/macOS/Android, but darwin builds need a macOS SDK extracted from Xcode (**only SDK 11.3 supported**), and release-mode packaging for macOS/Windows needs a macOS/Windows host respectively (https://github.com/fyne-io/fyne-cross).
- **Binary size:** UNVERIFIED (no measured figure found; blog-level anecdotes only). Rymdport wiki notes the `-release` flag and the `no_emoji` build tag shrink binaries (https://github.com/Jacalz/rymdport/wiki/Compiling). Measure locally.
- **Known Windows issues:** needs a working OpenGL driver; reports of "WGL: The driver does not appear to support OpenGL" on VMs/servers with the Basic Display driver and a blank "GLFW message window" after upgrades (community threads, secondary sources: https://github-users.yhajoa.workers.dev/fyne-io/fyne/discussions/6036). 2.8.2 fixed a blank window after minimising with `FixedSize` on Windows. Console window appears unless built with `-ldflags -H=windowsgui` (or via `fyne package`). Mesa software-GL DLL workaround is from a low-quality blog post - UNVERIFIED.

### 2.2 Wails v2 and v3

- **Wails v3 IS stable as of 2026-10-26.** `v3.0.0` is marked `prerelease: false`, published **2026-10-26T11:15:13Z** (https://api.github.com/repos/wailsapp/wails/releases/latest; module proxy tag time 2026-10-26T10:57:44Z, https://proxy.golang.org/github.com/wailsapp/wails/v3/@latest). It is 3 days old. It follows 34 beta releases (last: v3.0.0-beta.34, 2026-10-20) and nightlies of v3.1.0 are already published (v3.1.0-nightly.20261028). A web search did not surface an announcement (search index lag), but the GitHub API and release page are authoritative.
- **Wails v2:** latest **v2.17.0**, tag 2026-10-18 (https://proxy.golang.org/github.com/wailsapp/wails/v2/@latest). Release notes say v2 "continues to receive critical updates". v2 has no built-in system tray runtime API (stale issue https://github.com/wailsapp/wails/issues/4990), so v2 is out for this app.
- **v3.0.0 release notes** (https://github.com/wailsapp/wails/releases/tag/v3.0.0):
  - Rewrite: runtime multi-window, services (plain Go structs exposed to the frontend), manager APIs for windows/menus/dialogs/events/screens/clipboard, typed events, bindings generated by static analysis to TS/JS.
  - Native UI: **system tray** (menus, light/dark icons, macOS template icons, attach a window to the tray icon), app/context/tray menus, native dialogs, frameless windows, drag regions, notifications, global/window shortcuts, clipboard, drag and drop, start at login, single instance, file associations, per-window permissions.
  - Platforms: Windows amd64/arm64, macOS Intel/Apple Silicon, Linux amd64/arm64; iOS/Android experimental. **Go 1.26+** (release recommends 1.26.10 or 1.27.3+). Linux default stack is **GTK4 + WebKitGTK 6.0** (Ubuntu 24.04+/Debian 13+); legacy GTK3 via `-tags gtk3` only **until v3.1**.
  - Tooling: `wails3` CLI (`setup`, `init`, `doctor`, `dev`, `generate bindings`), Taskfile-based builds, packaging: **NSIS and MSIX (Windows), DMG and app bundles (macOS), AppImage/deb/rpm/Arch (Linux)**, code signing, an in-app updater with rollback. MSI and notarization are not mentioned in the notes. Templates: vanilla/React/Vue/Svelte (JS or TS).
  - Cadence: "weekly releases bump the minor version, nightlies preview the next weekly, hotfixes bump the patch"; pin exact versions (`@wailsio/runtime` npm package must match the Go version).
  - Known limitations: child-frame access to bound methods (treat any iframe as having access to bound Go methods); migration from v2 is a real port.
  - Last-minute fixes in the final beta: Windows crashes in menu-bar painting (#6311) and `applyDarkMode` (#6358).
- **API surface relevant to EdgeSync** (https://pkg.go.dev/github.com/wailsapp/wails/v3/pkg/application, v3.0.0): `SystemTray` (`SetIcon`, `SetMenu`, `AttachWindow`, `OnClick`), `ClipboardManager` (`SetText`, `Text` - text only), `ScreenManager` (`GetAll`, `GetPrimary`, `LayoutScreens`), OS file-drop via `WindowEventContext.DroppedFiles()`.
- **Frontend requirement:** a Node toolchain for the JS/TS frontend (templates use Vite). Layout editor would be SVG/Canvas in the webview, where drag-and-snap is easy but it is a second language.
- **WebView2 on Windows:** "already included in Windows 11; on Windows 10 download from Microsoft" (https://docs-preview.wails.io/contributing/setup/); the generated NSIS installer includes a WebView2 bootstrapper that downloads the runtime if needed; offline needs Microsoft's Evergreen Standalone Installer (https://docs-preview.wails.io/guides/build/windows/). v2 docs cite the embedded bootstrapper adding ~150 KB (https://wails.io/docs/guides/windows/ via search snippet).
- **cgo:** Wails v3 setup docs list MSYS2/MinGW-w64 and say "Ensure you have a C compiler (MinGW-w64 via MSYS2) in your PATH" for `CGO_ENABLED` errors on Windows (https://docs-preview.wails.io/contributing/setup/); macOS needs Xcode CLT; Linux needs `libgtk-4-dev libwebkitgtk-6.0-dev`. Whether Windows builds can run with `CGO_ENABLED=0` is **UNVERIFIED**.
- **Binary size:** UNVERIFIED for v3 (third-party posts claim 6-15 MB for v2 apps; not benchmarks).
- **Theming:** CSS (`prefers-color-scheme`), native window dark mode handled by Wails; Liquid Glass on macOS 26.

### 2.3 Gio (`gioui.org`)

- **Version:** v0.10.3, tag 2026-09-28 (https://proxy.golang.org/gioui.org/@latest). Still 0.x (pre-1.0, API may break; "small breaking change" precedent in v0.7.1). Release news: v0.10.0 (May 2026) returned after a long hiatus; v0.10.2 (Aug 2026) rendering fix; v0.10.3 (Sep 2026) Linux IME support (https://gioui.org/news). Source hosted on sourcehut (`git.sr.ht/~eliasnaur/gio`). Go floor 1.24.0 (https://raw.githubusercontent.com/gioui/gio/v0.10.3/go.mod).
- **Requirements:** Windows - "no special compiler is needed, as we don't use CGo for Windows support" (https://gioui.org/doc/install); macOS needs Xcode; Linux needs dev packages for Wayland, X11/xkbcommon, GLES, EGL, libXcursor plus gcc/pkg-config (https://gioui.org/doc/install/linux). Windows rendering API (Direct3D 11) is not stated on the install page - UNVERIFIED.
- **System tray:** not part of Gio; none of `app` docs mention it (https://pkg.go.dev/gioui.org/app). Would need `fyne.io/systray` (`RunWithExternalLoop`) alongside Gio; unproven combination (UNVERIFIED).
- **OS file drop:** not documented. `io/transfer` handles in-app drag-and-drop only (https://pkg.go.dev/gioui.org/io/transfer). UNVERIFIED for dropping files from the OS file manager.
- **Custom canvas:** best-in-class flexibility (immediate-mode, `op`/`paint`/`pointer` APIs, `widget.Draggable`), but you build more yourself.
- **Other:** `TopMost` windows supported on macOS/Windows (v0.10.0); more than one window is not supported on iOS/Android/wasm (desktop multi-window implied). Packaging: no official tool equivalent to `fyne package` (a `gogio` tool exists - UNVERIFIED details). Binary size UNVERIFIED.

### 2.4 Other credible options

- **Qt via `github.com/mappu/miqt`** (v0.15.0, tag 2026-10-09; https://proxy.golang.org/github.com/mappu/miqt/@latest). MIT bindings for Qt 5.15 and Qt 6.4+. cgo; first build about 10 minutes; hello world 43 MB -> 6 MB with `-ldflags "-s -w"`; Windows works (native, MSYS2, Docker); static linking needs a Qt commercial licence, otherwise dynamic LGPL; README says bindings "may be immature in some ways" (https://github.com/mappu/miqt). QSystemTrayIcon not mentioned in README (UNVERIFIED). Qt's `QGraphicsView` would be the strongest native layout editor but the build/licence/cgo cost is high.
- **Cogent Core** (`cogentcore.org/core` v0.5.14, tag 2026-10-28; https://proxy.golang.org/cogentcore.org/core/@latest). Pre-1.0; tray support and Windows maturity UNVERIFIED. Not recommended on current evidence.
- **Tray-only + local web UI.** Use a tray library (`fyne.io/systray` v1.12.2, MIT-style fork of getlantern; README says "requires cgo"; Linux via D-Bus SNI; https://github.com/fyne-io/systray) and serve the UI on `127.0.0.1` from the Go binary. Cheapest layout editor (any JS drag library), no GUI toolkit, but no real app window (opens a browser tab), weaker native feel, and a localhost HTTP API needs CSRF/origin hardening. Equivalent to Wails v3 minus the packaged webview. `getlantern/systray` last tag v1.2.2 (2023-03-27, https://proxy.golang.org/github.com/getlantern/systray/@latest) and needs GTK3 headers on Linux - prefer the fyne fork. `energye/systray` v1.0.3 (2026-01-31) is another fork (README also says cgo required; 27 commits; https://github.com/energye/systray).
- **webview/webview_go**: last pseudo-version 2024-08-31 (https://proxy.golang.org/github.com/webview/webview_go/@latest) - stale; skip.

### 2.5 Ranking for EdgeSync

Criteria: one-language Go codebase, tray, a drag-and-snap layout editor, dark mode, packaging story across 3 OSes, risk of immaturity, and testability.

| Rank | Option | Why |
|---|---|---|
| 1 | **Fyne v2.8.2** | Mature 2.x line with steady releases; pure-Go UI; tray, light/dark, `fyne package`, OS file drop, and a headless `test` driver with `Drag`/golden images all exist. Custom layout editor is a modest custom widget. Costs: cgo + OpenGL on every OS (you already have gcc), non-native look, `fyne.Do` discipline, text-only clipboard API, no MSI/notarization in `fyne package`. |
| 2 | **Wails v3.0.0** | Best result for a polished layout editor and the richest built-in packaging (NSIS, MSIX, DMG, AppImage/deb/rpm), tray and screen/clipboard managers. But GA is 3 days old, weekly minor releases, requires Go 1.26+, a Node/JS frontend, WebView2 dependency on Windows 10, GTK4/WebKitGTK 6.0 on Linux (Ubuntu 24.04+), and a Windows cgo status that I could not verify. Re-evaluate after v3.0.x patch releases settle (suggest about 4 to 6 weeks). |
| 3 | **Tray + local web UI** | Fallback if both toolkits disappoint; reuses the same Go core; weakest app feel. |
| 4 | **Gio v0.10.3** | No tray, no documented OS file drop, pre-1.0 API; attractive only for a fully custom-drawn UI and for cgo-free Windows builds. |
| 5 | **Qt via miqt** | Most native/powerful widgets but heaviest build, licence and cgo burden. |

**Architecture note (applies to every option):** put input capture/injection, networking, clipboard and transfers in a headless core (daemon/service) and make the UI a separate client over local IPC (named pipe / unix socket). This (a) keeps Win32 hook message loops, macOS event taps and the GUI main-thread requirements from fighting each other, (b) lets Fyne-now/Wails-later be swapped without touching the core, and (c) lets the core keep running when the UI is closed. This recommendation is my design judgement, not from a source.

---

## 3. Networking and security

### 3.1 QUIC with quic-go

- **Latest:** `github.com/quic-go/quic-go` **v0.64.1**, tag 2026-10-19 (https://proxy.golang.org/github.com/quic-go/quic-go/@latest). Recent: v0.64.0 (16 Oct), v0.63.0 (22 Sep), v0.62.0 (30 Aug), v0.61.0 (24 Jul), v0.60.0 (06 Jun), v0.59.1 (11 May) (https://github.com/quic-go/quic-go/releases). Pre-1.0, so expect API churn on minor bumps (v0.63.0 changed HTTP/3 URL handling and error types).
- **Go requirement:** v0.62.0 release notes: "quic-go now requires Go 1.26 or newer" and drops 1.25 (https://github.com/quic-go/quic-go/releases/tag/v0.62.0); v0.64.1 go.mod says `go 1.26.0`. Dependencies include `golang.org/x/crypto`, `x/net`, `x/sys`.
- **Datagrams (RFC 9221):** set `quic.Config.EnableDatagrams`; check `ConnectionState().SupportsDatagrams.Local/Remote`; `conn.SendDatagram([]byte)` and `conn.ReceiveDatagram(ctx)`. Datagrams are encrypted, congestion-controlled and **not retransmitted** when lost. Docs caveats: the datagram send/receive path "is not yet optimized" (suits occasional datagrams, lower throughput than streams; issue #3766), and APIs to query the maximum size and get ack/loss notifications are planned (#4259, #4273) (https://quic-go.net/docs/quic/datagrams/). Recent fixes: `ReceiveDatagram` fails immediately on an already-cancelled context, `SendDatagram` returns the close error after shutdown, dropped datagrams are logged in qlog (v0.64.x release notes). Mouse-move rate (about 125 to 1000 Hz) is well within "occasional".
- **Stream multiplexing and priorities:** `OpenStream`, `OpenStreamSync`, `OpenUniStream`, `AcceptStream`, `AcceptUniStream`, `Config.MaxIncomingStreams`, per-stream `CancelRead`/`CancelWrite` (https://quic-go.net/docs/quic/streams/). **Stream priorities (RFC 9218) were added in v0.62.0**: `SetPriority(urgency, incremental)` on `SendStream`/`Stream`, urgency 0 to 7 with lower sent first, and retransmissions respect them (release notes + streams doc). This is the direct answer to "bulk file transfer vs latency-sensitive input on one connection".
- **Keepalive and timeouts:** `Config.KeepAlivePeriod`, `MaxIdleTimeout`, `HandshakeIdleTimeout` (https://quic-go.net/docs/quic/connection/). DPLPMTUD is on by default (`DisablePathMTUDiscovery` to turn off).
- **Windows UDP notes:** buffer-size enlargement is implemented for "Linux, Windows and macOS"; **GSO is Linux-only (kernel 4.18+) and "GSO on Windows" is listed as future work**; `QUIC_GO_DISABLE_GSO=true` disables GSO; GRO and ECN are not covered on the optimizations page (https://quic-go.net/docs/quic/optimizations/). The Windows source sets the DF bit with `IP_DONTFRAGMENT` / `IPV6_DONTFRAG` and handles `WSAEMSGSIZE`; no ECN handling in that file (https://raw.githubusercontent.com/quic-go/quic-go/v0.64.1/sys_conn_df_windows.go). For LAN input traffic and occasional large transfers, lack of GSO on Windows only limits peak bulk throughput, not input latency.
- **TLS:** QUIC mandates TLS 1.3. Custom verification via `tls.Config` callbacks works the same as for TCP (see 3.5). Connection handshake callbacks (`GetConfigForClient`, `GetCertificate`, `GetClientCertificate`) are supported (connection doc above). A QUIC server response before address validation is capped at 3x client bytes, so keep certificates small (use ECDSA P-256 or Ed25519 leaf certs; https://quic-go.net/docs/quic/server/).
- **Practical caveats (my analysis):** UDP can be blocked by corporate firewalls and Windows Firewall prompts per binary; keep a TCP+TLS fallback behind a `Transport` interface.

### 3.2 TCP + TLS with yamux or smux

- **hashicorp/yamux** v0.1.2, tag 2024-09-06 (https://proxy.golang.org/github.com/hashicorp/yamux/@latest). Spec: 256 KB default per-stream window, window-update frames, ping frames; **no stream priorities defined and no mention of head-of-line blocking** (https://github.com/hashicorp/yamux/blob/master/spec.md). Low release cadence.
- **xtaci/smux** v1.5.58, tag 2026-10-28 (https://proxy.golang.org/github.com/xtaci/smux/@latest). README advertises per-stream sliding-window flow control (protocol v2), "built-in fair queue traffic shaping", token-bucket receive and keepalive; **no per-stream priority** (https://github.com/xtaci/smux). Its own sample benchmark shows about 2.3x lower throughput than raw TCP (2,247 vs 5,124 MB/s on darwin/amd64).
- **HOL implications (my analysis):** a muxed single TCP connection has two coupled queues. (1) TCP itself: one lost segment stalls every stream behind it until retransmission. (2) Mux write queue and kernel send buffer: an input frame must wait for bulk bytes already queued. With yamux's default 256 KB (2,097,152 bits) per-stream window, a full window ahead of an input frame is about 42 ms at 50 Mbit/s Wi-Fi, 21 ms at 100 Mbit/s, 2 ms at 1 Gbit/s, before counting kernel send-buffer autotuning. Neither yamux nor smux has priorities to fix this.
- **Better TCP design if QUIC is not used:** do not multiplex input with bulk. Use connection A (TCP_NODELAY, small writes, input and control) and open a separate TCP connection B per file transfer, optionally rate-limited. This removes mux HOL but both flows still share the bottleneck queue; QUIC priorities handle that more cleanly.

### 3.3 mDNS / DNS-SD libraries

| Library | Latest tag | Repo activity (GitHub API, 2026-10-29) | Notes |
|---|---|---|---|
| `github.com/hashicorp/mdns` | **v1.0.7**, tag 2026-06-11 | pushed 2026-10-19, 1.4k stars, not archived, MIT | Most active. Simple client/server; Windows behaviour UNVERIFIED (spike required) |
| `github.com/brutella/dnssd` | v1.2.14, tag 2024-10-22 | pushed 2026-02-27, 239 stars, MIT | Full responder (probing, announcing, `UpdateText`) and "passes the multicast DNS tests of Apple's Bonjour Conformance Test"; README TODO: no hot-plug support, no negative responses, cache cleanup incomplete; Windows support not stated (https://github.com/brutella/dnssd) |
| `github.com/grandcat/zeroconf` | v1.0.0, tag 2020-01-29 | pushed 2023-12-07, 904 stars, 53 open issues, licence "other" | Stale. Known Windows problem: responses went only to loopback until the multicast interface was set (issue #54; PR #110 proposes `SetMulticastInterface`; merge status UNVERIFIED) |
| `github.com/libp2p/zeroconf/v2` | v2.2.0, tag 2022-08-07 | pushed 2025-08-20, 28 stars | Fork of grandcat; low activity |

Sources: proxy `@latest` URLs for each module and https://api.github.com/repos/<owner>/<repo>. Windows issue: https://www.github.com/grandcat/zeroconf/issues/54 and https://github.com/grandcat/zeroconf/pull/110.

**Recommendation:** do a 1-day spike with `hashicorp/mdns` and `brutella/dnssd` on Windows 11 (multi-NIC, VPN adapters, Wi-Fi + Ethernet) before committing. Whatever library wins, add a LocalSend-style fallback: UDP multicast announce plus HTTP/scan fallback and **manual add-by-IP**. LocalSend discovery uses multicast `224.0.0.167:53317` with an HTTP "register" reply, and falls back to sending the register request to all local IPs when multicast fails (https://github.com/localsend/protocol). Since peers are authenticated by pinned certificates, discovery does not need to be trusted.

### 3.4 PAKE / short-code pairing

**Go PAKE options (none audited):**

| Library | Version | Protocol | Audit and status |
|---|---|---|---|
| `github.com/schollz/pake/v3` | **v3.2.0**, tag 2026-07-31; 243 stars, 3 open issues, MIT | "PAKE2" per Boneh-Shoup book p. 789, curves siec/P-256/P-384/P-521; built for croc | README: no third-party audit mentioned; **not** the RFC 9382 key schedule or key confirmation, apps "must perform mutual key confirmation before treating the exchange as authenticated" (https://github.com/schollz/pake) |
| `github.com/mendsec/cpace` | **v0.1.0**, tag 2026-10-13, Apache-2.0 | CPace per draft-irtf-cfrg-cpace-21, CPACE-RISTRETTO255-SHA512; checked against the draft's test vectors | Pre-1.0, no independent audit, no known importers (https://pkg.go.dev/github.com/mendsec/cpace). Depends on `gtank/ristretto255`, `filippo.io/edwards25519` |
| `github.com/backkem/spake2-go` | v0.0.1, 2025-05-29, MIT | SPAKE2 per RFC 9382 | README: no formal security review |
| `github.com/bytemare/opaque` | v0.18.0, 2026-03-24 | OPAQUE per RFC 9807 (augmented PAKE; server-side password file model) | "has not been independently audited" (https://github.com/bytemare/opaque); wrong shape for symmetric peer pairing |
| `github.com/FiloSottile/cpace` | 2021 | CPace ristretto255 | Marked experimental, last push Feb 2021 (stale) |

- CPace is **not yet an RFC**: draft-irtf-cfrg-cpace is at -21, sent to the RFC Editor with IESG review done, "awaiting first editor", no RFC number (https://datatracker.ietf.org/doc/draft-irtf-cfrg-cpace/). SPAKE2 is RFC 9382 and SPAKE2+ is RFC 9383 (per library READMEs/search results).
- Cloudflare CIRCL (v1.6.5, tag 2026-08-05, https://proxy.golang.org/github.com/cloudflare/circl/@latest) is a well-reviewed crypto library, but I found no `pake/cpace` package page (the pkg.go.dev URL returned 404) - treat as not available.
- **How the incumbents pair:**
  - **Deskflow / Barrier / Input Leap:** self-signed X.509 certificate generated per machine (Deskflow: `generatePemSelfSignedCert`, 2048-bit default); trust is **SHA-256 fingerprint pinning** against a trusted-fingerprint file. In Deskflow `SecureSocket.cpp`, `verifyCertFingerprint()` computes SHA-256 of the peer certificate and checks `FingerprintDatabase::isTrusted`; the server checks clients against `tlsTrustedClientsDb` only at security level `PeerAuth` (client certificate required via `SSL_VERIFY_PEER | FAIL_IF_NO_PEER_CERT`, with chain validation neutralised by a callback that always returns 1); the client always checks the server against `tlsTrustedServersDb`; TLS 1.2 is the floor; an unknown fingerprint is logged and the connection dropped (the UI obtains the fingerprint over IPC) (https://raw.githubusercontent.com/deskflow/deskflow/master/src/lib/net/SecureSocket.cpp, https://raw.githubusercontent.com/deskflow/deskflow/master/src/lib/net/SecureUtils.h, https://raw.githubusercontent.com/deskflow/deskflow/master/src/lib/net/Fingerprint.h). Fingerprint types are `sha1` and `sha256`, serialised as db lines; display adds ":" separators and a "randomart" image. Barrier/Input Leap use files `Fingerprints/Local.txt`, `TrustedServers.txt`, `TrustedClients.txt` with `v2:sha256:` lines; Barrier 2.4.0 broke older SHA-1 entries (community wiki/mailing-list results; https://www.github.com/debauchee/barrier/wiki/Troubleshooting). Deskflow latest release **v1.27.0, 2026-10-01** (https://api.github.com/repos/deskflow/deskflow/releases/latest); Input Leap latest release v3.0.3, 2025-06-13 (https://api.github.com/repos/input-leap/input-leap/releases/latest).
  - **KDE Connect:** UDP identity broadcast on 1716, TCP connection upgraded to TLS with self-signed certs, packet-level `kdeconnect.pair` request/accept with a 30 s timeout, and since the December 2020 security fix a user-visible **verification key = SHA-256 of the concatenated public keys of both certificates** (initially only an 8 hex prefix was shown) (https://advisories.mageia.org/MGASA-2020-0475.html, https://valent.andyholmes.ca/documentation/protocol.html, https://mail.kde.org/pipermail/kdeconnect/2016-November/001723.html). The exact UDP/TCP port details come from third-party implementations (e.g. https://pkg.go.dev/github.com/bethropolis/kcd) and are secondary.
  - **LocalSend (protocol v2):** HTTPS with self-signed certs where "the fingerprint is the SHA-256 hash of the certificate"; no formal pairing: trust rests on remembering fingerprints plus the receiver accepting each transfer, with an optional per-transfer PIN (`?pin=`) (https://github.com/localsend/protocol).
  - **Lan Mouse (Rust, comparable product):** DTLS via WebRTC.rs, per-machine certificate fingerprint, receivers accept input only from **authorised fingerprints** (GTK UI or config) (https://github.com/feschber/lan-mouse).
- **Design analysis (mine, not from a source):** pure SHA-256 pinning with a user-confirmed "numeric comparison" is only safe if the compared string is long or committed to before it is revealed; a 6-digit value of a cert hash can be ground by an attacker generating keypairs. Options: (a) compare a long fingerprint/verification key like KDE Connect, (b) commit-reveal SAS as in Bluetooth numeric comparison/ZRTP, or (c) a PAKE short code bound to the TLS session with `ConnectionState.ExportKeyingMaterial` (RFC 5705; works on TLS 1.3, https://pkg.go.dev/crypto/tls). A PAKE removes the need to trust a human comparison but all Go PAKE libraries above are unaudited.

### 3.5 Mutual TLS with self-signed certs and pinning in Go

All from https://pkg.go.dev/crypto/tls and https://pkg.go.dev/crypto/x509:

- Generate a leaf per install with `x509.CreateCertificate` (parent == template gives a self-signed cert). Supported key types: RSA, ECDSA, Ed25519 (and ML-DSA in 1.27). Prefer ECDSA P-256 or Ed25519 for small certificates and fast QUIC handshakes.
- **Server:** `ClientAuth: tls.RequireAnyClientCert` - "a certificate is required ... but that certificate is not required to be valid", exactly the self-signed pinning case. **Client:** `InsecureSkipVerify: true` **only together with** a custom check ("should be used only for testing or in combination with VerifyConnection or VerifyPeerCertificate").
- **Use `VerifyConnection`, not only `VerifyPeerCertificate`:** `VerifyPeerCertificate` "is not invoked on resumed connections" (including resumption across Configs from `Clone`/`GetConfigForClient`), while `VerifyConnection` "will run for all connections, including resumptions, regardless of InsecureSkipVerify or ClientAuth settings". Inside it, compare `SHA-256(cert.RawSubjectPublicKeyInfo)` of `cs.PeerCertificates[0]` against the paired-peer store; pinning the SPKI hash lets you re-issue certificates without re-pairing, pinning the whole-cert hash does not (design choice; Deskflow pins whole-cert fingerprint).
- Set `MinVersion: tls.VersionTLS13`; QUIC requires it anyway. Use `ExportKeyingMaterial` (RFC 5705) for channel binding during pairing; it errors if renegotiation is allowed or neither TLS 1.3 nor EMS is in use.
- Keep Go patched: go1.27.3/go1.26.10 fix a `crypto/x509` vulnerability (https://go.dev/doc/devel/release), and 1.27.2 fixed `crypto/tls` issues.

---

## 4. Tooling

### 4.1 Versions (module proxy tag times)

| Tool | Version | Date | Source |
|---|---|---|---|
| golangci-lint | **v2.15.0** | 2026-10-16 | https://proxy.golang.org/github.com/golangci/golangci-lint/v2/@latest |
| gofumpt | **v0.12.0** | 2026-09-07 | https://proxy.golang.org/mvdan.cc/gofumpt/@latest |
| staticcheck | **2026.2.2** (module `honnef.co/go/tools` v0.8.2) | 2026-10-12 | https://proxy.golang.org/honnef.co/go/tools/@latest , https://staticcheck.dev/changes/ (lists 2026.2 = v0.8.0) |
| govulncheck | ships in `golang.org/x/vuln` **v1.9.0** | 2026-10-11 | https://proxy.golang.org/golang.org/x/vuln/@latest ; install per https://go.dev/doc/tutorial/govulncheck |
| GoReleaser | **v2.19.0** | 2026-10-22 | https://proxy.golang.org/github.com/goreleaser/goreleaser/v2/@latest |

Notes:
- golangci-lint v2.15.0 bumps `honnef.co/go/tools` 0.8.1 -> 0.8.2 and errcheck, exhaustive, `x/tools`; no new linters listed (https://github.com/golangci/golangci-lint/releases/tag/v2.15.0). Standalone staticcheck is redundant: v2 merged `stylecheck`, `gosimple` and `staticcheck` into one `staticcheck` linter (https://golangci-lint.run/docs/product/migration-guide/).
- gofumpt and goimports are now **formatters**, configured under `formatters:` (migration guide above). Run them through golangci-lint itself so a single pinned tool covers lint and format (a `golangci-lint fmt` subcommand exists in v2 to my knowledge, but I did not verify it against the docs - UNVERIFIED).
- golangci-lint "supports Go versions lower or equal to the Go version used to compile it" (https://golangci-lint.run/docs/welcome/faq/). Use the official binary (built with a current Go) rather than `go install`: docs say "We recommend using binary installation" and "We don't recommend using `go tool`" (https://golangci-lint.run/docs/welcome/install/local/). Windows: `choco install golangci-lint` or `scoop install main/golangci-lint` (scoop not officially maintained); winget is not mentioned. CI: use `golangci/golangci-lint-action`, pin the linter version.
- Linting cgo packages requires a working C toolchain on the lint machine (FAQ above: "If using CGO, ensure all required system libraries are installed").
- Defaults: with no config the `standard` set is errcheck, govet, ineffassign, staticcheck, unused (https://golangci-lint.run/welcome/quick-start via search result). Existence of all of these linters on the v2 page was confirmed: gosec, errcheck, govet, staticcheck, revive, gocritic, errorlint, bodyclose, noctx, contextcheck, exhaustive, nilerr, sloglint, testifylint, paralleltest, thelper, wrapcheck, forbidigo, depguard, gochecknoglobals, nolintlint, copyloopvar, intrange, perfsprint, gosmopolitan, musttag, errname, usestdlibvars, unparam, unconvert, prealloc, makezero, protogetter (https://golangci-lint.run/docs/linters/).

### 4.2 Strict v2 config shape

Top-level keys of the v2 file: `version`, `linters` (`default`, `enable`, `disable`, `settings`, `exclusions`), `formatters` (`enable`, `settings`, `exclusions`), `issues`, `output`, `run`, `severity` (https://golangci-lint.run/docs/configuration/file/). `linters.default` accepts `standard`, `all`, `none`, `fast` (https://raw.githubusercontent.com/golangci/golangci-lint/main/.golangci.reference.yml). Exclusions moved to `linters.exclusions` (`rules`, `paths`, `generated`, `presets`). Suggested shape (validate with `golangci-lint config verify` before use; per-linter setting keys below were not individually verified because the settings page returned 404):

```yaml
version: "2"
run:
  go: "1.26"          # match go.mod
  tests: true
linters:
  default: none
  enable:
    - errcheck
    - govet
    - ineffassign
    - staticcheck
    - unused
    - gosec
    - errorlint
    - bodyclose
    - noctx
    - contextcheck
    - exhaustive
    - nilerr
    - sloglint
    - revive
    - gocritic
    - unparam
    - unconvert
    - perfsprint
    - copyloopvar
    - intrange
    - errname
    - usestdlibvars
    - testifylint
    - paralleltest
    - thelper
    - nolintlint
    - forbidigo      # e.g. forbid fmt.Print*, time.Sleep in non-test code
    - depguard       # keep GUI imports out of the core packages
    - wrapcheck
  settings:
    govet:
      enable-all: true
    errcheck:
      check-type-assertions: true
    nolintlint:
      require-explanation: true
      require-specific: true
  exclusions:
    generated: strict
    presets:
      - comments
      - std-error-handling
    rules:
      - path: _test\.go
        linters: [wrapcheck, gosec]
formatters:
  enable:
    - gofumpt
    - goimports
issues:
  max-issues-per-linter: 0
  max-same-issues: 0
```

Architecture lint idea: `depguard` rules so `core/...` cannot import `fyne.io/...` (supports the headless-core design above).

### 4.3 GoReleaser v2.19.0: what is OSS and what is Pro

Based on the per-feature pages under https://goreleaser.com/customization/:

| Capability | Edition | Notes |
|---|---|---|
| Archives, checksums, changelog, GitHub releases, Winget/Scoop/Homebrew publishers, SBOMs, Snap, Flatpak, Chocolatey | listed without a Pro marker | not individually verified |
| **nFPM: deb, rpm, apk, archlinux, ipk** | OSS (only `if`, `templated_contents`, `templated_scripts` are Pro) | also experimental `msix` format (added v2.17) (https://goreleaser.com/customization/package/nfpm/) |
| **MSI** | **Pro only** | uses `msitools` (schema v3 only) off Windows, or WiX (`candle`/`light` for v3, `wix` for v4/5) on Windows; `version` option since v2.7 (https://goreleaser.com/customization/package/msi/) |
| **NSIS** | **Pro only**, since v2.14 | uses `makensis` (https://goreleaser.com/customization/package/nsis/) |
| **macOS App Bundles (`.app`)** | **Pro only**, since v2.4; only usable together with DMGs (as of 2.4) | https://goreleaser.com/customization/package/app_bundles/ |
| **DMG** | **Pro only** | `mkisofs` or `hdiutil` (https://goreleaser.com/customization/package/dmg/) |
| **Notarization** | native (`codesign` + `xcrun notarytool`, needed for `.app`/pkg) is **Pro only**, macOS runner required; cross-platform notarization via anchore/quill works on any OS but only for **binaries and universal binaries**, and a signed binary inside a non-notarized `.app` is flagged damaged | needs Apple Developer account ($99/yr per the doc), Developer ID Application cert (.p12) and App Store Connect API key (.p8) (https://goreleaser.com/customization/sign/notarize/) |
| **AppImage** | **no GoReleaser section exists** | use the Wails v3 pipeline or `appimagetool` in CI (UNVERIFIED tool choice) |
| Split-and-merge (per-OS native cgo builds merged into one release) | Pro (recommended by GoReleaser for cgo) | https://goreleaser.com/limitations/cgo/ |

Implication: with GoReleaser OSS you get cross-platform archives and Linux deb/rpm/apk/archlinux packages. Windows installers (MSI/NSIS), signed `.app` and DMG require Pro or separate scripts (e.g. WiX/NSIS/Inno via CI, `fyne package` for `.app`). Pro pricing was not checked (UNVERIFIED). Wails v3 covers NSIS/MSIX/DMG/AppImage/deb/rpm in its own Taskfile pipeline if you choose it.

### 4.4 cgo cross-compilation constraints and CI runners

- GoReleaser: "Compiling with CGO is tricky, especially when cross-compiling"; options are Pro split-and-merge, `goreleaser-cross` Docker images, or Zig as C compiler which "might not work for all cases" (https://goreleaser.com/limitations/cgo/).
- **Can macOS cgo binaries be built from Windows? Not natively.** GUI toolkits (Fyne, Wails, Gio) link Apple frameworks via cgo, so you need the macOS SDK and a cross compiler. `osxcross` lists host OSes **Linux and *BSD only** (no Windows) and needs an SDK extracted from Xcode / Command Line Tools after reading Apple's Xcode licence terms (https://github.com/tpoechtrager/osxcross, https://raw.githubusercontent.com/tpoechtrager/osxcross/master/README.SDK.md). Whether Apple's licence permits SDK use on non-Apple hardware is not answered by those pages (UNVERIFIED; legal check needed). `goreleaser-cross` Docker image bundles osxcross and mingw for macOS (amd64, arm64), Linux (amd64, arm64, armhf, s390x) and Windows (amd64, arm64) targets (https://github.com/goreleaser/goreleaser-cross). `fyne-cross` supports only SDK 11.3 and cannot package macOS release builds off a darwin host (https://github.com/fyne-io/fyne-cross). Wails v3 and Gio on macOS need Xcode; Gio notes osxcross + SDK for cross-compiling from Linux (https://gioui.org/doc/install).
- **Cross-compiling from Windows to Linux** is similarly painful for cgo (needs GTK/X11/Wayland headers and a sysroot); do it in a Linux runner or container.
- **Recommended CI shape: native builds per OS**, using GitHub-hosted runner labels (https://raw.githubusercontent.com/actions/runner-images/main/README.md, checked 2026-10-29):
  - Windows: `windows-latest` (= Windows Server 2025), also `windows-11-arm` for arm64.
  - macOS arm64: `macos-latest` (= macOS 26 arm64); macOS x64: `macos-15-intel` or `macos-26-intel`. Go 1.27 needs macOS 13+ at runtime. How long Intel runners remain available is UNVERIFIED.
  - Linux: `ubuntu-24.04` (= `ubuntu-latest`), `ubuntu-24.04-arm`, newer `ubuntu-26.04` exists. Pin versions explicitly because `-latest` moves.
  - Notarization runs on macOS runners (or via quill on any OS for plain binaries).
- Local Windows dev: native Windows builds use the msys2 ucrt64 gcc; for Fyne/Wails Windows installers build natively on Windows.

---

## 5. Testing

- **Interfaces for OS-facing code:** define small ports (`InputInjector`, `InputCapturer`, `Clipboard`, `ScreenGeometry`, `Clock`, `PeerTransport`) and keep Win32/Cocoa/X11/portal code in thin adapters behind build tags. Test the core (edge detection, layout math, state machines, protocol) with fakes and table-driven tests; run **contract tests** (one shared suite executed against both fake and real adapters, real ones gated by build tag or env var) on real OS runners. This is my recommendation, not from a source.
- **Mocks and assertions:** `go.uber.org/mock` v0.6.0 (tag 2025-08-18, https://proxy.golang.org/go.uber.org/mock/@latest; quic-go still uses v0.5.2), `stretchr/testify` v1.12.1 (tag 2026-08-17), `go.uber.org/goleak` v1.3.0 (tag 2023-10-24, stable but old), `pgregory.net/rapid` v1.3.0 (tag 2026-03-30) for property tests of codecs and layout snapping (proxy URLs: https://proxy.golang.org/github.com/stretchr/testify/@latest, https://proxy.golang.org/pgregory.net/rapid/@latest, https://proxy.golang.org/go.uber.org/goleak/@latest).
- **`go test -race` on Windows:** the race detector "requires cgo to be enabled, and on non-Darwin systems requires an installed C compiler"; supported on `windows/amd64` (not windows/arm64 per the list); on Windows it needs a C compiler with **mingw-w64 runtime v8 or later**, checkable with `--print-file-name libsynchronization.a`; typical overhead 5-10x memory and 2-20x time (https://go.dev/doc/articles/race_detector). Your msys2 ucrt64 gcc (mingw-w64 14.0 headers) qualifies; make sure it is first on PATH and `go env CGO_ENABLED` prints 1. Also run `-race` in Linux/macOS CI.
- **Fuzzing protocol decoders:** `go test -fuzz=FuzzX -fuzztime=...` with `testing.F`, seed corpus via `f.Add` and `testdata/fuzz/FuzzX/`; failures are written as regression files; arguments limited to string, []byte, ints, floats, bool; fuzzing needs coverage instrumentation (the docs name **AMD64 and ARM64**); the docs page says nothing specific about Windows, so run long fuzz sessions on Linux CI (https://go.dev/doc/security/fuzz/). Fuzz every decoder (frame header, handshake, clipboard offer, file manifest) and the path sanitiser used with `os.Root`. Check round-trip properties: `decode(encode(x)) == x` and "never panics, never allocates unbounded".
- **Golden files:** store expected encodings under `testdata/` with an `-update` flag; wire-format goldens double as a compatibility contract across protocol versions. For UI, Fyne's test package supports `AssertImageMatches` and `AssertRendersToMarkup` masters in `testdata` with `testdata/failed/` output (https://pkg.go.dev/fyne.io/fyne/v2/test).
- **`testing/synctest`:** GA since Go 1.25; `synctest.Test` runs a bubble with a fake clock starting 2000-01-01 UTC, time advances only when all goroutines are durably blocked; 1.27 adds `synctest.Sleep`. Network I/O and syscalls are not durably blocking, so test keepalive, edge-dwell/hysteresis, retry/backoff and pairing timeouts against in-memory transports, not real sockets (https://pkg.go.dev/testing/synctest).
- **Integration tests:** loopback QUIC tests between two in-process peers (no synctest), plus a TCP-fallback transport running the same contract suite. For network impairment use a user-space shim (a `net.PacketConn` wrapper that delays/drops) around quic-go's `Transport` (my suggestion; quic-go accepts a custom `net.PacketConn` via `quic.Transport{Conn: ...}` per https://quic-go.net/docs/quic/server/).
- **OS behaviours worth explicit tests:** Windows `SendInput` is subject to UIPI (fails against higher-integrity windows with no distinguishing error) and does not reset keyboard state (https://learn.microsoft.com/en-us/windows/win32/api/winuser/nf-winuser-sendinput); low-level hook callbacks must return within `LowLevelHooksTimeout` (max 1000 ms since Windows 10 1709) or the hook is **silently removed** on Windows 7+, must run on a thread with a message loop, and Microsoft recommends a dedicated thread that hands work off immediately, or Raw Input (https://learn.microsoft.com/en-us/windows/win32/winmsg/lowlevelmouseproc).

---

## 6. Wire serialization

Versions (proxy tag times): `google.golang.org/protobuf` **v1.36.12** (2026-08-10), `github.com/fxamacker/cbor/v2` **v2.10.0** (2026-10-28), `github.com/google/flatbuffers` v25.12.19 (2025-12-19), `github.com/vmihailenco/msgpack/v5` v5.4.1 (2023-10-26, stale), `github.com/planetscale/vtprotobuf` v0.6.0 (2024-01-29). URLs: https://proxy.golang.org/google.golang.org/protobuf/@latest, https://proxy.golang.org/github.com/fxamacker/cbor/v2/@latest, https://proxy.golang.org/github.com/google/flatbuffers/@latest, https://proxy.golang.org/github.com/vmihailenco/msgpack/v5/@latest, https://proxy.golang.org/github.com/planetscale/vtprotobuf/@latest.

**Measured latency/alloc numbers: UNVERIFIED.** The alecthomas benchmark site (https://alecthomas.github.io/go_serialization_benchmarks/) renders its table client-side and I could not extract rows; its benchmark struct is a six-field record (Name, BirthDay, Phone, Siblings, Spouse, Money), which is not a 12-to-24 byte input event anyway. Write a local benchmark with `testing.B.Loop` using your real message shapes.

Qualitative facts that are sourced:

| Option | Hot-path suitability (small, high-rate events) | Schema evolution | Notes |
|---|---|---|---|
| **Hand-rolled binary framing** (type byte, flags, varints or fixed ints via `encoding/binary` `Append*`) | Best: zero or near-zero allocations, fixed size, no reflection | Manual: version byte, length-prefixed frames, "ignore unknown trailing bytes" rule, reserved type ranges | You own fuzzing and compat tests; ideal for mouse-move, key down/up, button, wheel |
| **protobuf-go** v1.36.12 | Allocates per message by default; Opaque API reduces allocations only for messages with many scalar fields ("Messages with few elementary fields perform about the same as before"); lazy decoding is opt-in for submessages (https://go.dev/blog/protobuf-opaque) | Strongest tooling: "Adding new fields is safe", never reuse field numbers, use `reserved`, unknown fields preserved (https://protobuf.dev/programming-guides/proto3/) | Needs protoc/buf codegen; Edition 2024 makes the Opaque API default. `vtprotobuf` can add pooling/fast paths (benefits UNVERIFIED) |
| **FlatBuffers** v25.12.19 | Reads without parsing or heap allocation ("The only memory needed to access your data is that of the buffer"), but Go builder ergonomics are the weakest of the options (UNVERIFIED) and tiny events gain little | Add fields only at table end, never remove, `deprecated` attribute, `flatc --conform` checks (https://flatbuffers.dev/evolution/) | Not recommended here |
| **CBOR** (fxamacker/cbor v2.10.0, RFC 8949) | Compact (struct `toarray`, `keyasint` tags), v2.10.0 claims encode up to 2.88x and decode up to 1.87x faster and fewer allocations; `MarshalToBuffer` accepts a caller buffer | Self-describing maps with int keys; omit/unknown handling via tags; no codegen | Security options: `MaxArrayElements`, `MaxMapPairs`, `MaxNestedLevels`, duplicate-key policy, UTF-8 checks; fuzzed "billions of execs" before tagging; NCC Group assessed a subset of v2.4 with no vulnerabilities (https://github.com/fxamacker/cbor). Good for control messages |
| MessagePack (vmihailenco v5.4.1) | n/a | n/a | Last tag 2023-10; skip |

**Recommendation:** two-tier protocol.
1. Hot path (pointer motion, buttons, keys, wheel, heartbeat): hand-rolled fixed layout, sent as QUIC datagrams for motion (latest wins) and on a high-priority reliable stream for key/button state changes.
2. Control plane (hello/version negotiation, layout, clipboard offers, file manifests and acks): protobuf for strong schema evolution and non-Go clients later, or CBOR if you want no codegen. Pick one; benchmark both with real messages before freezing.
3. File bytes: raw length-delimited chunks on dedicated low-priority streams, not wrapped in the control codec. Verify with SHA-256 per file and write through `os.Root`.

---

## Recommended stack (with versions)

| Layer | Choice | Version (date) |
|---|---|---|
| Language/toolchain | Go, installed from the official MSI | **1.27.3** (2026-10-19); `go.mod` can say `go 1.26.0` (quic-go and Wails floor) unless 1.27 features are wanted |
| Windows C toolchain | msys2 ucrt64 gcc already present | mingw-w64 headers 14.0 (verified locally) |
| GUI | **Fyne** `fyne.io/fyne/v2` behind a local IPC boundary; `fyne.io/tools/cmd/fyne` for packaging | **v2.8.2** (tag 2026-10-07); tools **v1.7.4** (2026-10-19) |
| GUI alternative to re-evaluate | Wails v3 | **v3.0.0** (2026-10-26); revisit after v3.0.x patches |
| Tray (if not using Fyne's) | `fyne.io/systray` | v1.12.2 (2026-06-09) |
| Transport | `github.com/quic-go/quic-go` (datagrams for motion, priority streams for the rest) with a TCP+TLS fallback (two connections, no mux) | **v0.64.1** (2026-10-19) |
| Mux (only if a muxed fallback is unavoidable) | `github.com/xtaci/smux` | v1.5.58 (2026-10-28); yamux v0.1.2 is stale (2024-09) |
| Discovery | spike `github.com/hashicorp/mdns` (and `brutella/dnssd`), plus UDP multicast/scan fallback and manual add-by-IP | hashicorp/mdns **v1.0.7** (2026-06-11); dnssd v1.2.14 |
| Identity/auth | stdlib `crypto/tls` TLS 1.3 mutual auth, self-signed ECDSA P-256 or Ed25519 certs, SPKI pinning in `VerifyConnection` | Go 1.27.3 |
| Pairing | channel-bound long verification key or commit-reveal SAS first; optional PAKE short code later (`mendsec/cpace` v0.1.0 or `schollz/pake/v3` v3.2.0, both unaudited) | - |
| Wire format | hand-rolled binary hot path + `google.golang.org/protobuf` for control (or `fxamacker/cbor/v2`) | protobuf **v1.36.12** (2026-08-10); cbor **v2.10.0** (2026-10-28) |
| OS bindings | `golang.org/x/sys` for Win32 (hooks, SendInput, OLE clipboard via `go-ole` v1.3.0); `ebitengine/purego` for cgo-free Apple calls where possible (suitability for event taps UNVERIFIED) | x/sys **v0.49.0** (2026-10-06); purego v0.11.2 (2026-10-15) |
| Lint/format | golangci-lint v2 (includes staticcheck, gofumpt, goimports via `formatters`) | **v2.15.0**; gofumpt v0.12.0; staticcheck 2026.2.2 (bundled) |
| Vulnerability scanning | `govulncheck` | from `golang.org/x/vuln` **v1.9.0** |
| Release | GoReleaser OSS (archives, nFPM deb/rpm/apk) + `fyne package` for `.app`/exe + native-OS CI runners; Pro (MSI/NSIS/DMG/app bundle/notarize) or scripts for installers | GoReleaser **v2.19.0** (2026-10-22) |
| Testing | stdlib `testing` + `testing/synctest` + `-race` + native fuzzing; testify, gomock, rapid, goleak; Fyne `test` package | testify v1.12.1, uber mock v0.6.0, rapid v1.3.0, goleak v1.3.0 |
| CI | GitHub Actions native matrix: `windows-latest`, `macos-latest` + `macos-15-intel`, `ubuntu-24.04` | per runner-images README (2026-10-29) |

## Open risks

1. **Per-OS input capture and injection is the real project risk, not the GUI.** Windows: `SendInput` is blocked by UIPI for higher-integrity windows without an identifying error, and low-level hooks are silently removed if the callback is slow (Microsoft docs above); UAC/secure-desktop and lock-screen behaviour is UNVERIFIED here. Linux Wayland: capture/injection requires libei plus portals; Deskflow supports it since v1.17 with "some features may be unavailable or broken", Lan Mouse targets GNOME 45+, KDE Plasma 6.1+ and wlroots, X11 capture is not supported by Lan Mouse, and KDE InputCapture portal status was lagging in 2024 and is unconfirmed now (https://github.com/deskflow/deskflow, https://github.com/feschber/lan-mouse). macOS: needs Accessibility/Input Monitoring permissions and an event tap (not researched here - UNVERIFIED). Budget spikes per OS first.
2. **GUI choice is a bet.** Wails v3.0.0 went GA only 3 days ago (weekly minor cadence, Go 1.26+, WebView2 and Node dependencies, Windows cgo behaviour unverified). Fyne needs cgo + OpenGL everywhere (VM/RDP/Basic-Display-Driver failures reported), enforces `fyne.Do` threading and has a text-only clipboard API. Gio lacks a tray and documented OS file drop. Mitigation: headless core + IPC so the UI is replaceable.
3. **Pairing security design.** No audited Go PAKE exists (schollz/pake, mendsec/cpace, spake2-go, opaque are all unaudited; CPace is still an IETF draft). Short numeric comparison of cert hashes can be ground by an attacker unless long or committed (my analysis). Pin SPKI via `VerifyConnection` (not `VerifyPeerCertificate`, which is skipped on resumption), and consider a third-party review of the pairing flow.
4. **Cross-machine file copy-paste is platform-specific and has no Go library I found.** Windows needs OLE `IDataObject` virtual files (`CFSTR_FILEDESCRIPTOR` + `CFSTR_FILECONTENTS` with `IStream`, delayed rendering; https://learn.microsoft.com/en-us/windows/win32/shell/clipboard), macOS has `NSFilePromiseProvider` (availability and semantics are from general knowledge because the Apple page returned only a title - UNVERIFIED), Linux uses file-manager-specific URI lists (not researched). Fyne/Wails clipboard APIs are text only. Expect native adapters per OS.
5. **quic-go is pre-1.0 and Go 1.26+ only**, with an "unoptimized" datagram path, no GSO on Windows, and UDP that corporate firewalls may block. Keep transports behind an interface and a TCP fallback; pin the exact quic-go version.
6. **Packaging and signing cost.** GoReleaser's MSI, NSIS, DMG, App Bundle and native notarization are Pro features; macOS notarization needs an Apple Developer account ($99/yr per GoReleaser docs), certificates and macOS runners; fyne-cross cannot ship macOS release builds from non-macOS hosts. Apple SDK licensing for osxcross on non-Apple hardware needs a legal check (UNVERIFIED).
7. **mDNS reliability on Windows** (multi-NIC, VPN adapters, firewall prompts): grandcat/zeroconf has a documented Windows multicast-interface issue and is stale; hashicorp/mdns and brutella/dnssd Windows behaviour is untested. Always provide manual add-by-IP and a non-mDNS discovery fallback.
8. **Unmeasured items:** binary sizes (Fyne, Wails v3, Gio), serializer latency/alloc numbers, Gio tray/file-drop feasibility, Wails v3 Windows cgo requirement, fuzzing on Windows, winget freshness. Each is cheap to settle with a one-day spike or a local benchmark.
