# EdgeSync - Performance budgets

Hard numbers. A budget that is not met blocks the owning stream's exit unless the user approves an exception
(recorded as a decision). Results are appended to the "Results log" at the end (append-only, by the parent).

Reference environments:
- **LAN-W**: the user's two Windows PCs on wired gigabit Ethernet, same switch.
- **Loopback**: one machine, two in-process peers over 127.0.0.1 (agents may run these as tests).
- **CI**: GitHub-hosted runners (only relative/regression use; never compared against absolute LAN budgets).

## Budgets

| ID | Metric | Budget | Environment | Bench / harness | Owner (phase/stream) |
|---|---|---|---|---|---|
| PB-01 | Added input latency, physical event on server -> injection call returns on client | p50 <= 1.5 ms, p99 <= 5 ms (D-29) | LAN-W | `bench/latency` harness: server timestamps in MotionAbs/Key `tsDelta`, client measures RTT via Heartbeat echo and reports one-way estimates; plus hook-to-send and receive-to-inject local timings | P5 R3 (prototype P0 SP5, loopback P1 C1, e2e P3 G5) |
| PB-02 | Same as PB-01 while a saturating file transfer runs on the same peer | p99 <= 5 ms | LAN-W, Loopback | `bench/latency -bulk` | P5 R3, P1 C1 (loopback) |
| PB-03 | Loopback transport latency (datagram MotionAbs, input stream Key) | p99 <= 1 ms idle, <= 2 ms under bulk | Loopback | `internal/transport` `BenchmarkLatency*` / `TestLatencyBudget` | P1 C1 |
| PB-04 | Hot-path codec | 0 allocs/op; <= 50 ns/op encode and decode per frame | Loopback (host) | `internal/proto/wire` benchmarks | P1-F, P1 C1 |
| PB-05 | Engine per-event processing (capture event -> frame queued) | <= 5 us p99, 0 allocs/op | Loopback (host) | `internal/engine` `BenchmarkServerMotion`, `BenchmarkKey` | P1 C3 |
| PB-06 | Motion coalescing | at 1000 Hz input: <= 1000 MotionAbs/s sent, no queued backlog older than 2 ms | Loopback | `internal/engine` `TestCoalescing1000Hz` (synctest) | P1 C3 |
| PB-07 | Windows LL hook callback duration | p99 <= 50 us, max <= 1 ms over 10 min incl. GC stress; 0 watchdog re-installs without forced stalls | user PC | `integration` test + `bench/hooks` | P2 W1 (spike P0 SP1) |
| PB-08 | Edge switch time (contact, no guards -> cursor shown on client at entry point) | p99 <= 20 ms | LAN-W | `bench/latency -switch` | P5 R3, P3 G5 (loopback) |
| PB-09 | Clipboard text fetch (<= 1 MiB) paste latency | p99 <= 50 ms | LAN-W; Loopback <= 10 ms | `internal/clipboard` loopback bench; manual LAN test | P1 C4, P5 R3 |
| PB-10 | Clipboard image fetch (10 MiB PNG) | <= 300 ms | LAN-W | same | P1 C4, P5 R3 |
| PB-11 | File throughput, single 4 GiB file | >= 90% of the measured raw TLS-over-TCP baseline between the same PCs, and >= 105 MiB/s on GbE | LAN-W | `bench/transfer` (baseline mode + EdgeSync mode) | P5 R3 (loopback P1 C4) |
| PB-12 | Many small files (10 000 x 4 KiB) | >= 500 files/s | LAN-W | `bench/transfer -small` | P5 R3 |
| PB-13 | Discovery: peer visible after both agents start | <= 3 s p95 | LAN-W | manual + agent log timestamps | P1 C2 (spike P0 SP5), P5 R3 |
| PB-14 | Reconnect after link restore | session re-established <= 2 s p95 | LAN-W; Loopback with fault injection <= 1 s | `internal/session` `TestReconnectBudget` (synctest + fake conn), manual cable pull | P1 C1, P5 R3 |
| PB-15 | Agent idle CPU | <= 0.5% of one core averaged over 60 s, connected, no input | LAN-W | `bench/resources` (process CPU sampling) | P3 G5, P5 R3 |
| PB-16 | Agent CPU during 1000 Hz mouse movement | server <= 5%, client <= 5% of one core | LAN-W | `bench/resources` | P5 R3 |
| PB-17 | Agent memory | RSS <= 60 MiB idle, <= 120 MiB during a 4 GiB transfer | LAN-W | `bench/resources` | P5 R3 |
| PB-18 | GUI memory | RSS <= 150 MiB with 64-machine layout open | user PC | `bench/resources -gui` | P3 G1, P5 R3 |
| PB-19 | Layout editor frame time with 64 machines while dragging | p99 <= 16.7 ms | user PC | GUI frame-time log (`-perflog`) | P3 G1 (spike P0 SP5) |
| PB-20 | Agent start to IPC ready | <= 500 ms | user PC | agent log timestamps | P3 G5 |
| PB-21 | GUI launch to first frame | <= 1 s | user PC | GUI log timestamps | P3-F |
| PB-22 | SPAKE2 pairing computation per side | <= 50 ms | Loopback | `internal/spake2` benchmark | P1 C2 |
| PB-23 | Control message encode/decode (capnp Hello, ClipAnnounce) | <= 5 us/op; allocs recorded, no regression > 20% between batches | Loopback | `internal/proto` benchmarks | P1 C1 |
| PB-24 | IPC request round trip (GetStatus) | p99 <= 2 ms | user PC | `internal/ipc` benchmark on host pipes | P1 C5 |
| PB-25 | File paste time-to-first-byte (paste request -> first byte delivered to the target application) | p95 <= 50 ms loopback; p95 <= 150 ms LAN-W | Loopback, LAN-W | `internal/vfile` loopback bench (P4-F-019 `TestVFileTTFBBudget`), manual Explorer paste log stats | P4 F1 (D-109) |
| PB-26 | Soak memory growth | LAN-W 8 h: agent RSS slope over hours 1-8 <= 1 MiB/h on both agents, peak RSS within PB-17; loopback CI 30 min (after 5 min warm-up): heap-in-use slope <= 2 MiB/h | LAN-W, Loopback | `bench/soak` | P5 R3 (D-109) |
| PB-27 | Soak handle growth | LAN-W: Windows handle-count slope <= 5 handles/h; loopback: goroutines at the end <= count after warm-up + 5; Linux fd-count slope <= 1/h when the GNOME box takes part | LAN-W, Loopback | `bench/soak` | P5 R3 (D-109) |

## Bench definitions

- **bench/latency** (P5 R3): two modes. Server mode replays a recorded 60 s mouse trace at 1000 Hz through the real engine and transport into a client whose injector is a timestamping fake (`-fakeinject`) or the real injector (`-realinject`, manual). Reports p50/p90/p99/max of one-way estimates (RTT/2 corrected with Heartbeat echoes) and local segments (hook->send, recv->inject). `-bulk` starts a 4 GiB transfer concurrently. `-switch` measures edge switches every 500 ms.
- **bench/transfer** (P5 R3): baseline mode = raw TLS 1.3 over TCP stream copy between the same hosts; EdgeSync mode = full transfer engine over the session; `-small` = many small files.
- **bench/resources** (P5 R3): samples process CPU time and RSS every second via OS APIs (Windows `GetProcessTimes`/`GetProcessMemoryInfo`), reports averages and max.
- **bench/hooks** (P2 W1): wraps the W1 capture backend's `Stats()` over a 10-minute run with optional GC stress.
- **bench/soak** (P5 R3, PB-26/PB-27): loopback mode exercises a switch every 10 s, a clipboard announce + fetch every 30 s and a 10 MiB transfer every 2 min between two in-process agents; LAN mode drives the local agent over IPC (`JumpTo`/`ReturnHome` every 10 s, `SendFiles` of generated files) for 8 h and samples RSS, handles/fds and goroutines every minute; the slope is a least-squares fit over hours 1-8 (P5-R3-001 re-checks the thresholds).
- Unit-level benchmarks named above live next to their packages; `task ci:local` only compiles them (via `go vet`/`go test`), while the numbers come from explicit runs (`go test -run '^$' -bench <name> -benchmem <pkg>`) recorded below. Budget tests named `Test*Budget` run in `go test` and fail when a loopback budget is exceeded.

## Mid-build bench schedule
See `plan.md` section 4. Each scheduled bench appends a row to the results log with date, commit, environment, numbers and pass/fail.

## Results log (append-only)

| Date | Commit | Budget ID | Environment | Result | Pass |
|---|---|---|---|---|---|
