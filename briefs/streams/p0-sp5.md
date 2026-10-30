# Stream brief - P0 SP5 Core tech (Gio editor, QUIC, mDNS, Cap'n Proto, SPAKE2, IPC)

Phase 0 spike stream. Items: `briefs/plan-phase-0.md` section "SP5". Owner tag: `[owner:SP5]`.

## Scope
Prove the cross-platform building blocks behind D-33 (Gio editor), D-35 (QUIC datagrams + priorities, TCP
fallback), D-15/D-43 (mDNS), D-36 (Cap'n Proto toolchain and limits), D-14 (SPAKE2 RFC 9382) and D-46 (IPC
verification). Deliverable: evidence plus `briefs/spikes/sp5.md`.

## Files you may touch
Only your item's `Files:` under `spikes/gioeditor/`, `spikes/quicbench/`, `spikes/mdns/`, `spikes/capnp/`,
`spikes/spake2/`, `spikes/ipc/`, plus the qa script/results. Never touch the main module, plan files, or other
streams' spike directories.

## Conventions (binding)
- Versions: gioui.org v0.10.3, github.com/quic-go/quic-go v0.64.1, github.com/hashicorp/mdns v1.0.7, github.com/brutella/dnssd v1.2.14, capnproto.org/go/capnp/v3 v3.1.0-alpha.2, filippo.io/nistec v0.0.4, github.com/Microsoft/go-winio v0.6.4. Verify each API against pkg.go.dev for that exact version before writing code; quote the API you relied on in the README.
- Host-safe tests only (pure logic, loopback unicast, local pipes/sockets). mDNS multicast and GUI runs are user-run.
- Benchmarks use `testing.B.Loop` and report allocs/op.
- No em dashes or en dashes. Commits `spike(gui|net|proto|crypto|ipc): ... [FEAT-177]`, body `Item: <id>`, no attribution trailers.
- Research pointers: `briefs/research-go-ecosystem.md` sections 2.3, 3.1-3.5, 5, 6.

## Report format
```
FILES: <path> - <one clause>
GATES: <gate> pass|fail
ITEMS: <item id> <FEAT-NNN, ...>
TESTS: <what the test phase must cover, one line>
BLOCKED: <reason or NONE>
```

---

## Item P0-SP5-001
Gio editor (`spikes/gioeditor/`).
- Model (`snap.go`, pure Go): machines as groups of monitor rects in logical units; `Snap(moving Group, others []Group, threshold float32) (dx, dy float32, guides []Segment)` snapping the moving group's edges to the nearest facing edge of another group within the threshold (12 logical px), preferring the smallest distance; `Overlap(a, b)` returns the shared segment of facing edges.
- Canvas (`canvas.go`): Gio v0.10.3 event loop (`app.Window`, frame events, `op.Ops`), per-machine pointer handling with the drag gesture for the exact v0.10.3 API (look up `gioui.org/gesture` and `gioui.org/io/pointer`), selection, overlap highlight, zoom with Ctrl+wheel around the cursor, pan with middle drag, arrow-key nudge (1 px, Shift 10 px), light/dark palette toggle (key `T`), FPS/frame-time logging to stdout every second, `-machines N` generates N synthetic machines (2-3 monitors each).
- Builds with `CGO_ENABLED=0` on Windows.

## Item P0-SP5-002
QUIC bench (`spikes/quicbench/`).
- `peer.go`: self-signed ECDSA P-256 certs, TLS 1.3, ALPN `edgesync/1`, `quic.Config{EnableDatagrams: true, KeepAlivePeriod: time.Second, MaxIdleTimeout: 5 * time.Second}`; stream priority API as available in v0.64.1 (find the exact method for RFC 9218 urgency; record it).
- Tests: 10 s at 1000 Hz, frame = 18-byte MotionAbs-like payload with a send timestamp (same process clock), collect one-way latency distribution; bulk = a stream with the lowest urgency writing 1 MiB buffers continuously; input stream = highest urgency, small frames. TCP comparison: connection A (`TCP_NODELAY`, TLS 1.3) for input, connection B for bulk.
- `cmd/quicbench`: `-mode server|client -addr host:port -duration 30s -bulk` with RTT-based measurement (echo) for the LAN run between the user's two Windows PCs.

## Item P0-SP5-003
mDNS probe (`spikes/mdns/cmd/mdnsprobe`): `-lib hashicorp|dnssd`, `-name`, advertise `_edgesync._udp` on port 24860 with TXT `v=1`, `id=<random>`, `role=client`, `tcp=24860`; browse continuously; print each discovery with timestamp since start, source interface/address; print local interfaces at start (name, flags, addresses). README asks the user to run it simultaneously on all three machines, with and without VPN adapters up, and to note any Windows firewall prompt.

## Item P0-SP5-004
Cap'n Proto (`spikes/capnp/`).
- Schemas: `handshake.capnp` (Hello: protocolVersion UInt16, buildVersion Text, role enum, name Text, peerId Data, macs List(Data), desktop with monitors List(struct {id Text, x, y, w, h Int32, scalePermille UInt16})); `clipboard.capnp` (ClipAnnounce: origin Data, seq UInt64, formats List(struct {mime Text, size Int64})). Use the Go annotations from go-capnp's `std/go.capnp` (`$Go.package`, `$Go.import`).
- `gen.sh` (also runnable in Git Bash on the host): build capnpc-go from `tools/tools.mod`, locate go-capnp's std dir via `go list -modfile=tools/tools.mod -m -f '{{.Dir}}' capnproto.org/go/capnp/v3`, run `capnp compile -I<std> -o<capnpc-go>:gen --src-prefix=schema schema/*.capnp` with `capnp` from `$CAPNP` (vcpkg path from P0-F-002). Record the exact working command line and output layout.
- Limits: set the message traversal limit and depth limit APIs available in v3.1.0-alpha.2 (find their exact names); a crafted message with a far-pointer/list amplification must be rejected; oversized messages rejected before decode by a length check.
- Fuzz target decodes arbitrary bytes as Hello and must not panic or exceed limits.

## Item P0-SP5-005
SPAKE2 (`spikes/spake2/`).
- Implement RFC 9382 for the ciphersuite SPAKE2-P256-SHA256-HKDF-HMAC on `filippo.io/nistec` (`P256Point` SetBytes/Bytes/Add/ScalarMult/ScalarBaseMult): constants M and N from the RFC (section 6, compressed encodings), password scalar w derived as specified by the RFC's recommendation for the vectors (for test vectors use the RFC's given w), shares pA = x*P + w*M and pB = y*P + w*N, K = h*x*(pB - w*N) with cofactor h = 1 for P-256, transcript TT (length-prefixed little-endian u64 fields: A, B, pA, pB, K, w), Ke || Ka = Hash(TT), KDF to KcA || KcB with HKDF, confirmation MACs with HMAC.
- Fetch the RFC text from https://www.rfc-editor.org/rfc/rfc9382.txt and reproduce every P-256 vector in Appendix B byte for byte (TT, Ka, Ke, KcA, KcB, cA, cB).
- Reject the identity point and points not on the curve (nistec SetBytes rejects off-curve; test the identity explicitly).
- `exporter_test.go`: two TLS 1.3 loopback connections; `ConnectionState().ExportKeyingMaterial("EXPORTER-EdgeSync-pairing-v1", nil, 32)` equal on both ends of one connection and different across connections.

## Item P0-SP5-006
IPC (`spikes/ipc/`): Windows pipe via go-winio `ListenPipe` with `PipeConfig{SecurityDescriptor: "D:P(A;;GA;;;SY)(A;;GA;;;<user SID>)", MessageMode: false}` and first-instance behaviour; client verification of the server's user SID (`GetNamedPipeServerProcessId`); round-trip latency benchmark with 1 KiB messages. Unix: socket in a temp 0700 dir with 0600 mode; peer uid via `unix.GetsockoptUcred` (Linux) and `unix.GetsockoptXucred` or `Getpeereid` equivalent on darwin (verify which x/sys/unix function exists for darwin); tests compile on the host with GOOS=linux/darwin and run in the spikes workflow.

## Item P0-SP5-007
`qa/manual/p0-sp5-core.md` per `qa/manual/README.md`: gioeditor feel test (drag, snap, zoom, pan, nudge, theme, `-machines 64` frame times), mdnsprobe on the three machines (two libraries), quicbench LAN mode between the two Windows PCs on wired Ethernet (with and without `-bulk`).

## Item P0-SP5-M01
User-owned. Results into `qa/manual/results/p0-sp5-core.md`.

## Item P0-SP5-008
`briefs/spikes/sp5.md`: summary table; sections per mechanism with numbers (frame times, latency p50/p99/max idle and under bulk for QUIC datagram, QUIC stream and TCP fallback; discovery times per library and interface; capnp allocs/op and limit behaviour; SPAKE2 timing); "Recommended design for P1 and P3" (mDNS library choice, priority API usage, capnp generation command for `tools/capnpgen`, limit settings, SPAKE2 package API); "Requires user decision" for contradictions of D-14, D-15, D-33, D-35, D-36, D-46.
