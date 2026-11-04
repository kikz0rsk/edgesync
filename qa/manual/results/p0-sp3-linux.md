# P0-SP3 Linux spikes - results

## X11 integration tests on ubuntu-24.04 (GitHub Actions, P0-SP3-M02 evidence)

Run: https://github.com/kikz0rsk/edgesync/actions/runs/41880720011 (workflow `spikes`, job `linux`, 2026-11-04 17:49 to 17:51 UTC, main at 4e85925).
Logs provided by the user under `spikes/out/ci/logs/linux/`. Wayland/GNOME, Xephyr, filepaste and dndx11 observations on the GNOME box are NOT done yet.

- Build: Gio Linux packages installed (P0-F-012), `go vet ./...` and `go build ./...` PASSED, including the cgo-gated Gio files
  (`cmd/wloverlay/gio_linux_cgo.go`, `cmd/giodroplinux/main.go`): the residual compile risk recorded at P0-SP3-007 is closed.
- `go test -tags integration ./...` against Xvfb `:99` (3840x1080x24): everything passed except three tests:
  - `TestRandR_MonitorsListed` (linux/x11): "got 1 monitors, want at least 2; define them with xrandr --setmonitor". The workflow starts Xvfb but
    never runs `xrandr --setmonitor`, so the test expectation was never met (workflow gap, not a code defect). Fix item P0-SP3-011.
  - `TestXdnd_LeaveReachesTheTarget` and `TestXdnd_StaleProxyPropertyIsIgnored` (linux/dnd): `dnd: connect to display "": EOF` on the first
    connection of the test, while the other three dnd integration tests (source drop, release outside, proxy target) and the x11 tests passed in the same job.
    Hypothesis (strongly supported, not yet proven): Xvfb resets when its last client disconnects; the tests run one after another, so a
    connection attempt that lands during the reset window gets EOF. The standard remedy is `Xvfb -noreset`. Fix item P0-SP3-011 applies it
    and a rerun confirms or refutes it.
- Passed in the same step on the runner: capnp, gioeditor, stats, wincom, winhook, ipc, linux/ei, linux/portal, quicbench (41.0 s including the four
  latency tests), spake2, winclip, wininject, winsvc/helper, proto, supervise.
