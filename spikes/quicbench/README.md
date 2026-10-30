# QUIC datagram and priority latency bench (P0-SP5-002)

## Purpose

Measure one-way latency of 1000 Hz, 18-byte MotionAbs-like frames on three carriers, for the transport decision in D-35:

- QUIC datagrams (RFC 9221), idle and with a saturating bulk stream on the same connection.
- A reliable QUIC input stream at urgency 0 (RFC 9218), with a saturating bulk stream at urgency 7 on the same connection.
- TCP fallback: two TLS 1.3 connections (`TCP_NODELAY`), input on one, bulk on the other.

Budget reference: PB-02 and D-29 (input p99 <= 5 ms while a saturating file transfer runs). The bench does not change any budget.

The bench is loopback only (127.0.0.1, ephemeral ports) in `go test`. The `cmd/quicbench` LAN mode is built but is run by the user only, per `qa/manual/p0-sp5-core.md` (P0-SP5-007).

## Build

From the repository root (Git Bash or PowerShell):

```
go -C spikes vet ./quicbench/...
go -C spikes build -o bin/quicbench.exe ./quicbench/cmd/quicbench
go -C spikes test -count=1 -v ./quicbench/...
```

The LAN binary is written to `spikes/bin/quicbench.exe` (gitignored). Host gates: `go -C spikes vet ./...`, `go -C spikes build ./...`, `go -C spikes test ./quicbench/...`.

## Run

Loopback tests (host-safe, about 42 s): `go -C spikes test -count=1 -v ./quicbench/...`

LAN mode, two Windows PCs on wired Ethernet (user-run only):

```
PC-A (server):  spikes\bin\quicbench.exe -mode server -addr :24860
PC-B (client):  spikes\bin\quicbench.exe -mode client -addr <PC-A-LAN-IP>:24860 -duration 30s
PC-B (client):  spikes\bin\quicbench.exe -mode client -addr <PC-A-LAN-IP>:24860 -duration 30s -bulk
```

The server runs until interrupted (Ctrl+C). `-duration` applies to the client only. `-bulk` adds a client-to-server stream at urgency 7 for the same duration. Run the client once without and once with `-bulk`, and swap the roles for a second pass if time allows.

LAN mode output: `datagram` and `input stream` lines with sent, received, lost and one-way p50/p99/max (RTT/2, both timestamps taken on the client clock, so no clock sync is needed), plus `bulk ... MiB/s` with `-bulk`.

## What to observe

- LAN: input stream one-way p99 with and without `-bulk` against PB-02 (<= 5 ms). Datagram p99 and max with and without `-bulk`. Lost datagram count. Bulk throughput against PB-11 (>= 105 MiB/s on GbE).
- Loopback: whether datagram p99 rises under bulk (interference from the bulk stream), and whether the input stream p99 stays below the datagram p99 under bulk.

Paste back: the four `datagram` and `input stream` lines per run (verbatim) and the `bulk` line.

## API relied on (quic-go v0.64.1)

- `quic.Config{EnableDatagrams: true, KeepAlivePeriod: time.Second, MaxIdleTimeout: 5 * time.Second}`. Source: `config.go` / `interface.go`.
- `func (c *Conn) SendDatagram(p []byte) error` and `func (c *Conn) ReceiveDatagram(ctx context.Context) ([]byte, error)` (`connection.go`).
- `func (s *Stream) SetPriority(urgency int8, incremental bool)` and `func (s *SendStream) SetPriority(urgency int8, incremental bool)` (`stream.go`, `send_stream.go`). Urgency is clipped to 0..7, lower is sent first. Both `Stream` and `SendStream` take it; no return value.
- `quic.ListenAddr`, `quic.DialAddr`, `Conn.OpenStreamSync`, `Conn.OpenUniStreamSync`, `Conn.AcceptStream`, `Conn.AcceptUniStream`, `Conn.CloseWithError`.
- Datagram send is queued (32 frames) and not retransmitted. The receive queue holds 128 datagrams; overflow is dropped and logged (qlog or debug log) by quic-go. The lost counts above are the bench's own sent-minus-received totals.

## Clock (D-123)

`time.Now` and `time.Since` on Windows advance on timer ticks: a probe on this host (200,000 back-to-back reads) saw only two distinct deltas, and steps cluster at about 0.5, 1.0 and 1.5 ms. Loopback latency is well below that, so an earlier run with the stdlib clock reported `p50=0s` and was invalid. The bench now reads `QueryPerformanceCounter` through `syscall.SyscallN` (`clock_windows.go`), and `clock_other.go` uses `time.Since` on other platforms. `TestClock_Resolution` checks the smallest step (100 ns here) and `TestClock_ReadsDoNotAllocate` checks the read path is allocation-free.

Frames carry their send time from this clock, and receive latency is `clock.Now() - SentNS` on the same clock, so no cross-machine synchronisation is involved in the loopback tests.

## Measured loopback numbers

Host: Windows 11, Go 1.27.3, single process (client and server share the CPU), `go -C spikes test -count=1 -v ./quicbench/...`, run 1 of this item. Clock step 100 ns. One-way latency = receive time minus send time on the same clock, over 10 s nominal at 1000 Hz.

| Test | sent | received | lost | p50 | p99 | max | bulk |
|---|---|---|---|---|---|---|---|
| TestDatagramLatency_Idle | 9976 | 9976 | 0 | 53.5 us | 106 us | 1.348 ms | none |
| TestDatagramLatency_UnderBulk | 9526 | 9526 | 0 | 100.8 us | 397.8 us | 2.560 ms | 133.9 MiB/s |
| TestInputStreamLatency_UnderBulk | 9505 | 9505 | 0 | 100.8 us | 335.6 us | 14.194 ms | 134.8 MiB/s |
| TestTCPFallbackLatency_UnderBulk | 9580 | 9580 | 0 | 19.4 us | 49.1 us | 293.6 us | 771.9 MiB/s |

Achieved send rate (sent over 10 s nominal 1000 Hz): idle datagram 998 Hz, datagram under bulk 953 Hz, input stream under bulk 951 Hz, TCP input under bulk 958 Hz. Pacing uses a Go ticker; under bulk about 4 to 5 percent of ticks were not delivered on time. This is recorded as measured, not tuned.

Reading the numbers:

- Under bulk, datagram p99 rises from 106 us to 398 us and p50 from 53 us to 101 us. Datagrams are packed ahead of stream frames by the packet packer, but they share the same congestion window, so bulk traffic still delays them. Priorities (urgency) do not apply to datagrams.
- The reliable input stream at urgency 0 has p99 336 us under bulk, well inside PB-02's 5 ms on loopback. Its max is 14.2 ms in one sample of about 9500; this single outlier is recorded, not explained.
- TCP fallback has the lowest latency here because the input connection has no shared queue with the bulk connection. Its bulk throughput is much higher than QUIC's (772 vs 134 MiB/s) in this single-process setup, where quic-go CPU cost is shared with the bulk writer. quic-go lists GSO on Windows as future work (`briefs/research-go-ecosystem.md` 3.1), so these loopback throughput numbers say nothing about LAN throughput (PB-11).
- Loopback numbers include in-process scheduling contention. LAN numbers (P0-SP5-007) are the ones that matter for the budgets.

## Caveats

- `ClientTLSConfig` skips chain verification (`InsecureSkipVerify`): each run uses its own in-memory self-signed P-256 certificate and there is no trust store. The LAN mode does not authenticate the peer. Peer identity is out of scope for this spike.
- Certificates are generated in memory per process and never written to disk.
- Tests are not parallel. Timing measurements share the machine.

## Paste back

- Loopback: the four `go test -v` latency lines and the `TestClock_Resolution` line.
- LAN: the server and client output for each run (`datagram`, `input stream`, `bulk`).
