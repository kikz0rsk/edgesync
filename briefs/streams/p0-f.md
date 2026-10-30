# Stream brief - P0-F Foundation block (toolchain + scaffold)

Phase 0. Sequential, one agent at a time, in item order. Items: `briefs/plan-phase-0.md` section "P0-F".

## Scope
Install the remaining toolchain (only after the user approves each installer in chat), create the repository
scaffold, the spikes module, the pinned tools file, the lint configuration, the coverage and fuzz helpers, the
Taskfile, and the CI workflow. No product code.

## Files you may touch
Only the files listed in your item's `Files:` line. Never edit `plan.md`, `briefs/**`, `decisions.md`,
`ownership.md`, `risk-register.md`, `perf-budgets.md`, `compatibility-matrix.md`, `CLAUDE.md`.

## Conventions (binding)
- Decisions: D-01, D-22, D-23, D-27, D-28, D-53, D-54, D-55, D-67, D-79 in `decisions.md`.
- No em dashes or en dashes anywhere; use "-".
- Never launch EdgeSync or spike binaries. Installing tools is allowed only in USER-CONFIRM items after approval.
- Never modify system or user PATH, registry, or environment persistently; if something is missing, report BLOCKED with the exact command the user should run.
- Verify every version and URL at execution time (proxy.golang.org, GitHub API, nuget API). If a pinned version is not available, BLOCKED.
- Downloads go to the session scratchpad directory, never into the repo.
- Commit format: `<type>(<scope>): <description> [FEAT-NNN]`, body first line `Item: <item id>`. No attribution trailers.

## Report format (reply with exactly this, nothing else)
```
FILES: <path> - <one clause>          (one line per file, or NONE)
GATES: <gate> pass|fail               (verbatim output only for a failure)
ITEMS: <item id> <FEAT-NNN, ...>
TESTS: <what the test phase must cover for this item, one line>
BLOCKED: <what stopped you, or NONE>
```

---

## Item P0-F-001
Install golangci-lint v2.15.0 (official binary; D-54).
1. Download `https://github.com/golangci/golangci-lint/releases/download/v2.15.0/golangci-lint-2.15.0-windows-amd64.zip` and `.../golangci-lint-2.15.0-checksums.txt` to the scratchpad.
2. Compare `sha256sum` of the zip with its line in the checksums file. Mismatch: BLOCKED.
3. Extract `golangci-lint.exe` into `%USERPROFILE%\go\bin\` (create the directory if missing).
4. Check resolution: Git Bash `command -v golangci-lint`, PowerShell `(Get-Command golangci-lint).Source` after reloading Machine+User PATH. Not found: BLOCKED (tell the user to add `%USERPROFILE%\go\bin` to their user PATH).
5. `golangci-lint version` must contain `2.15.0`.
6. Create `qa/manual/results/p0-f-toolchain.md` with a "golangci-lint" section: version output, path, checksum.

## Item P0-F-002
Install Cap'n Proto 1.4.0 via vcpkg (D-36).
1. MSVC check: `"C:\Program Files (x86)\Microsoft Visual Studio\Installer\vswhere.exe" -latest -products * -requires Microsoft.VisualStudio.Component.VC.Tools.x86.x64 -property installationPath`. Missing tool or empty output: BLOCKED ("Visual Studio Build Tools with the C++ workload are required for vcpkg").
2. `D:\Development\vcpkg\vcpkg.exe install capnproto:x64-windows` (the local port is 1.4.0; verify `D:\Development\vcpkg\ports\capnproto\vcpkg.json`).
3. Locate `capnp.exe` under `D:\Development\vcpkg\installed\x64-windows\tools\capnproto\` (verify the actual path).
4. `<path>\capnp.exe --version` must print `Cap'n Proto version 1.4.0`.
5. Append a "capnp" section to `qa/manual/results/p0-f-toolchain.md` with the absolute path (this becomes the `CAPNP` default in the Taskfile and in `tools/capnpgen`).

## Item P0-F-003
Inno Setup 7.1.1 (D-118; the WiX and .NET SDK steps of D-65 are superseded).
1. Read-only check: `winget show --id JRSoftware.InnoSetup.7 --exact` must report `Version: 7.1.1` and `Installer SHA256:
   8a5a6711c3766a6e5fb74f280ba0e3551e719ce94538d901fdbd1f8f9920b3f0`. Any other version or hash: BLOCKED (tell the user what winget reports).
   Do not use the id `JRSoftware.InnoSetup`: it is Inno Setup 6 (6.7.3 on 2026-10-30).
2. `winget install --id JRSoftware.InnoSetup.7 --exact --version 7.1.1` (the user approved the installer step in chat). Do not
   change PATH or any other system setting.
3. Locate `ISCC.exe` (the install directory printed by winget or the Inno Setup uninstall registry key; read-only lookup) and run
   `<path>\ISCC.exe --version`; it must report 7.1.1 (record the exact output format: P5-R1-002 matches it).
4. Compile a one-line scratch script (`[Setup]` with `AppName`, `AppVersion`, `DefaultDirName`, `OutputDir` in the session scratchpad
   directory, no files) with `ISCC.exe`; the compile must exit 0. Never run the produced setup.
5. Append an "Inno Setup" section to the results file: winget id, version, SHA-256, `ISCC.exe` path and `--version` output. Also record
   the licence note (commercial users above USD 5000 revenue are requested to buy a licence, R-37).

## Item P0-F-004
Repository scaffold.
- `go.mod`: `module github.com/kikz0r/edgesync`, `go 1.27`. No requires yet.
- `LICENSE`: standard MIT text, `Copyright (c) 2026 kikz0r`.
- `README.md`: title "EdgeSync"; one paragraph: share one keyboard and mouse across Windows, macOS and Linux computers on a network, move the cursor across a screen edge to control the neighbouring computer, shared clipboard (text, rich text, images, files), file transfer including copy-paste and drag-and-drop; "Status: in development"; links to `plan.md`, `decisions.md`, `briefs/architecture.md`; "License: MIT".
- `.gitattributes`: `* text=auto eol=lf`; `*.syso binary`, `*.png binary`, `*.ico binary`, `*.icns binary`, `*.zip binary`; `*.ps1 text eol=crlf`; `*.iss text eol=crlf`.
- `.editorconfig`: root = true; `[*]` charset utf-8, end_of_line lf, insert_final_newline true, trim_trailing_whitespace true; `[*.go]` indent_style tab; `[*.{yml,yaml,toml,md,capnp,json,xml,plist}]` indent_style space, indent_size 2; `[*.{ps1,iss}]` end_of_line crlf (Inno Setup scripts are kept CRLF like the PowerShell scripts).
- Gates: `go mod edit -json`, `go build ./...` (exit 0 with "matched no packages" warning is fine).

## Item P0-F-005
Spikes module and workflow (D-74).
- `spikes/go.mod`: `module github.com/kikz0r/edgesync/spikes`, `go 1.27`.
- `spikes/README.md`: rules verbatim from D-74, plus: every spike dir has its own README with build/run/observe/paste-back sections; binaries build into `spikes/bin/` (gitignored); outputs into `spikes/out/` (gitignored, uploaded by CI).
- `.github/workflows/spikes.yml`: `on: workflow_dispatch`. Jobs `windows` (`windows-latest`), `macos` (`macos-latest`), `linux` (`ubuntu-24.04`). Steps: checkout, setup-go with `go-version: '1.27.3'`, `go -C spikes vet ./...`, `go -C spikes build ./...`, `go -C spikes test -tags integration ./...`, upload-artifact `spikes/out/**` with `if: always()`. Linux job first runs `sudo apt-get update && sudo apt-get install -y xvfb x11-xserver-utils xdotool`, starts `Xvfb :99 -screen 0 3840x1080x24 &`, exports `DISPLAY=:99` via `$GITHUB_ENV`.
- Pin `actions/checkout`, `actions/setup-go`, `actions/upload-artifact` to the exact latest release tag from `https://api.github.com/repos/<owner>/<repo>/releases/latest`.

## Item P0-F-006
Tools alternate module (D-54).
1. Create `tools/tools.mod` with `module github.com/kikz0r/edgesync` and `go 1.27`.
2. From the repo root: `go get -modfile=tools/tools.mod -tool mvdan.cc/gofumpt@v0.12.0`, `golang.org/x/vuln/cmd/govulncheck@v1.9.0`, `github.com/rhysd/actionlint/cmd/actionlint@v1.7.12`, `github.com/josephspurrier/goversioninfo/cmd/goversioninfo@v1.7.0`, `capnproto.org/go/capnp/v3/capnpc-go@v3.1.0-alpha.2`. This writes `tools/tools.sum`.
3. Verify every invocation from the item's Tests line. Root `go.mod`/`go.sum` must be unchanged (`git diff --exit-code go.mod`).
4. `bin/` is gitignored; do not commit binaries.

## Item P0-F-007
`.golangci.yml` (D-55). Write exactly this shape (adjust only if `golangci-lint config verify` rejects a key; report any adjustment in FILES):
```yaml
version: "2"
run:
  go: "1.27"
  tests: true
  build-tags: [integration]
linters:
  default: none
  enable: [errcheck, govet, ineffassign, staticcheck, unused, gosec, errorlint, bodyclose, noctx, contextcheck, exhaustive, nilerr, sloglint, revive, gocritic, unparam, unconvert, perfsprint, copyloopvar, intrange, errname, usestdlibvars, paralleltest, thelper, nolintlint, forbidigo, depguard, wrapcheck]
  settings:
    govet: { enable-all: true, disable: [fieldalignment] }
    errcheck: { check-type-assertions: true, check-blank: true }
    exhaustive: { default-signifies-exhaustive: true }
    nolintlint: { require-explanation: true, require-specific: true }
    sloglint: { no-mixed-args: true }
    revive:
      rules: [package-comments, blank-imports, context-as-argument, context-keys-type, dot-imports, error-return, error-strings, error-naming, if-return, increment-decrement, var-naming, var-declaration, range, receiver-naming, time-naming, unexported-return, indent-error-flow, errorf, empty-block, superfluous-else, unused-parameter, unreachable-code, redefines-builtin-id]   # each as {name: <rule>}; `exported` intentionally absent
    forbidigo:
      forbid:
        - { pattern: '^(fmt\.Print.*|print|println)$', msg: 'use log/slog (D-48)' }
        - { pattern: '^log\.(Print|Fatal|Panic).*$', msg: 'use log/slog (D-48)' }
    depguard:
      rules:
        gio-only-in-gui:
          files: ['$all', '!**/gui/**', '!**/cmd/edgesync/**', '!**/spikes/**']
          deny: [{ pkg: gioui.org, desc: 'Gio is used only by the GUI (D-33, D-55)' }]
        no-os-backends-in-core:
          files: ['**/internal/**', '!**/internal/platform/**', '!**/internal/service/**']
          deny:
            - { pkg: github.com/kikz0r/edgesync/internal/platform/windows, desc: 'use internal/platform interfaces (D-55)' }
            - { pkg: github.com/kikz0r/edgesync/internal/platform/darwin, desc: 'use internal/platform interfaces (D-55)' }
            - { pkg: github.com/kikz0r/edgesync/internal/platform/x11, desc: 'use internal/platform interfaces (D-55)' }
            - { pkg: github.com/kikz0r/edgesync/internal/platform/wayland, desc: 'use internal/platform interfaces (D-55)' }
            - { pkg: github.com/kikz0r/edgesync/internal/platform/linux, desc: 'use internal/platform interfaces (D-55)' }
        capnp-boundary:
          files: ['$all', '!**/internal/proto/**', '!**/internal/session/**', '!**/internal/ipc/**', '!**/internal/service/**', '!**/tools/capnpgen/**']
          deny: [{ pkg: github.com/kikz0r/edgesync/internal/proto/capnp, desc: 'generated capnp types stay behind proto/session/ipc/service (D-55)' }]
  exclusions:
    generated: strict
    presets: [std-error-handling]
    rules:
      - { path: '_test\.go', linters: [wrapcheck, gosec] }
formatters:
  enable: [gofumpt, goimports]
  settings:
    gofumpt: { extra-rules: true }
    goimports: { local-prefixes: [github.com/kikz0r/edgesync] }
issues:
  max-issues-per-linter: 0
  max-same-issues: 0
```
Do not add the `comments` exclusion preset (it would also hide missing package comments). Gates: `golangci-lint config verify`, `golangci-lint run ./...`.

## Item P0-F-008
`tools/covercheck` (part of the main module, D-54). Package `main` in `main.go` (flag parsing, exit codes) and logic in `covercheck.go` (same package; exported nothing).
- Input: a Go cover profile (`mode: set|count|atomic` then `file:sl.sc,el.ec numStmts count` lines), `-min` percent (float), `-exclude` file (one pattern per line, `#` comments; a pattern ending in `/...` matches the package and everything below it; patterns are module-relative like `internal/platform/windows/...`), `-packages` file (one import path per line, the output of `go list ./...`).
- Per package (import path = directory of the file path): merge duplicate blocks (same file + range) by max count; coverage = covered statements / total statements.
- Violations: included package below `-min`; included package absent from the profile ("no coverage data": every included package must have tests, or be added to the exclude file by a decision).
- Output: a table `package  stmts  covered  pct  status` to stdout via `fmt.Fprintf(os.Stdout, ...)`, then `PASS` or `FAIL (<n> violations)`; exit 0/1; exit 2 for usage or parse errors.
- `tools/coverage-exclude.txt` lists the patterns from the item's DoD with a comment `# D-37: OS backends are covered by conformance + integration tests` and `# GUI policy decided in P3-F`. Helper programs under `tools/` are NOT excluded; they meet the 85% bar like any other package.

## Item P0-F-009
`Taskfile.yml` (go-task v3 schema, D-53) and `tools/fuzzsmoke`.
- `version: '3'`. Top-level `vars`: `UCRT64: 'D:/Development/msys2/ucrt64/bin'`, `CAPNP_DEFAULT` = the path recorded by P0-F-002. Top-level `env`: none.
- Tasks (commands verbatim; the task shell is mvdan/sh, so POSIX `if`/`for` work on Windows):
  - `fmt`: `golangci-lint fmt`
  - `lint`: `golangci-lint run ./...`
  - `lint:cross`: for GOOS in linux darwin: `GOOS=<os> CGO_ENABLED=0 golangci-lint run <patterns>` where patterns are built from the directories `internal`, `cmd/edgesync-agent`, `cmd/edgesync-service`, `tools` that exist (`./<dir>/...`), so the task works before those directories exist.
  - `vet`: `go vet ./...`
  - `build`: `go build ./...`
  - `cross`: `GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build ./internal/... ./cmd/edgesync-agent/...` and `GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build ./internal/... ./cmd/edgesync-agent/...` (Go prints "matched no packages" and exits 0 while directories are absent).
  - `test`: `go test ./...`
  - `test:race`: env `CGO_ENABLED: '1'`, `CC: gcc`, and PATH prefixed with `{{.UCRT64}}` on Windows (`platforms`-specific command or `{{if eq OS "windows"}}`), command `go test -race ./...`.
  - `cover`: `go test -coverprofile=coverage.out ./...`, `go list ./... > coverage.pkgs`, `go run ./tools/covercheck -profile coverage.out -min 85 -exclude tools/coverage-exclude.txt -packages coverage.pkgs`.
  - `fuzz:smoke`: `go run ./tools/fuzzsmoke -fuzztime 30s ./...`
  - `vuln`: `go tool -modfile=tools/tools.mod govulncheck ./...`
  - `actionlint`: `go tool -modfile=tools/tools.mod actionlint`
  - `gen`: env `CAPNP: '{{.CAPNP | default .CAPNP_DEFAULT}}'`; `go generate ./...`
  - `gen:check`: `task: gen`, then `git diff --exit-code`
  - `ci:local`: runs in order lint, lint:cross, vet, build, cross, test, test:race, cover, fuzz:smoke, vuln, actionlint, gen:check
- `coverage.out` and `coverage.pkgs` are gitignored.
- `tools/fuzzsmoke`: for each package from `go list <patterns>`, run `go test -list '^Fuzz' <pkg>`, collect names (lines matching `^Fuzz`), then run `go test -run '^$' -fuzz '^<name>$' -fuzztime <d> <pkg>` sequentially; stream output; exit 1 on the first failure; print "no fuzz targets" and exit 0 when none exist. Parsing and command construction are pure functions tested by the listed tests; process execution is in `main.go`.

## Item P0-F-010
`.github/workflows/ci.yml`.
- Triggers: `push` and `pull_request` on `main`.
- Jobs: `lint` (ubuntu-24.04: setup-go 1.27.3, golangci-lint-action pinned with `version: v2.15.0`, then two more runs with `GOOS=windows` and `GOOS=darwin` env over the agent-side patterns using the action's `args`), `test` (matrix windows-latest, macos-latest, ubuntu-24.04: setup-go, go-task setup action pinned, `task build test test:race cover`), `cross` (`task cross`), `gen-check` (ubuntu-24.04: `sudo apt-get install -y capnproto`, `CAPNP=capnp task gen:check`), `vuln` (`task vuln`), `actionlint` (`task actionlint`).
- Pin every action to its exact latest release tag from the GitHub API at execution time (checkout, setup-go, golangci-lint-action, go-task's setup action).
- The Windows test job must have gcc for `-race`: GitHub's Windows image provides MinGW; verify with a `gcc --version` step and fail loudly if absent.

## Item P0-X-001
Parent-only (Opus). Read `briefs/spikes/sp1.md` .. `sp5.md`; write `briefs/spikes/p0-consolidation.md` with: confirmed decisions, every "Requires user decision" item with options + recommendation, and the list of P1-P5 items to append or annotate.

## Item P0-X-002
Planning consolidation (2026-10-30): the spine fork's confirmation points (installer toolset, LICENSE holder, D-41, D-51, D-61, D-64) and every gap found while writing P1-P5 were closed before plan approval by D-80 .. D-111 and the answers to `briefs/open-questions.md` Q-01 .. Q-10. This item covers only decision changes proposed by the P0 spike findings (`briefs/spikes/p0-consolidation.md`); a spike result that contradicts D-80 .. D-111 is raised here like any other locked decision.
User answers each proposal (the parent asks via AskUserQuestion, at most 4 per round). The parent appends approved decisions to `decisions.md` and notes/items to the affected phase files.

## Item P0-X-003
Parent-only. A Fork re-checks every pinned version in `compatibility-matrix.md` against proxy.golang.org; newer versions are recorded as notes only. Run `task ci:local`. Append the phase-flip note to `plan.md`:
```
<!-- NOTE: <date> - P0 exit criteria met.
Current phase: P1
-->
```

## Item P0-F-011
`.golangci.yml`, one setting (D-122). In `formatters.settings.gofumpt` replace `extra-rules: true` with
`extra: { group-params: true, clothe-returns: true }`. Change nothing else in the file. Verify with `golangci-lint config verify`,
then `golangci-lint run ./...` (0 issues, no deprecation warning on stderr) and `golangci-lint fmt --diff ./...` (no diff). If
the formatter now reports diffs in existing Go files (the rules are the same as before, so none are expected), stop and report
BLOCKED with the output.
