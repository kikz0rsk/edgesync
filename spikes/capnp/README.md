# Spike P0-SP5-004: Cap'n Proto toolchain and go-capnp API

## Purpose

Prove the Cap'n Proto toolchain for control messages (D-36): the schema compiler 1.4.0, the Go generator `capnpc-go`
from the pinned tools module, the go-capnp runtime `capnproto.org/go/capnp/v3 v3.1.0-alpha.2`, the message traversal and
depth limits, the oversize check, and the allocations of encode and decode. Schemas follow the families in
`briefs/architecture.md` 5.3 (`handshake.capnp` Hello, `clipboard.capnp` ClipAnnounce).

Host-safe only: nothing here touches input, clipboard, displays or multicast. Nothing is run as a binary.

## Build and regenerate (exact command)

Run from the repository root, Git Bash, with the vcpkg compiler:

```
CAPNP=/d/Development/vcpkg/installed/x64-windows/tools/capnproto/capnp.exe bash spikes/capnp/gen.sh
```

`gen.sh` does, in order:

1. `go build -modfile=tools/tools.mod -o bin/capnpc-go$(go env GOEXE) capnproto.org/go/capnp/v3/capnpc-go`
   (on the host: `bin/capnpc-go.exe`, the same command as P0-F-006).
2. Resolves the go-capnp `std` schema directory with
   `go list -modfile=tools/tools.mod -m -f '{{.Dir}}' capnproto.org/go/capnp/v3` plus `/std`
   (contains `go.capnp`, which the schemas import as `using Go = import "/go.capnp";`).
3. Puts `bin/` on `PATH` so `capnp` finds the plugin named `go` (`capnpc-go`).
4. Removes and recreates `spikes/capnp/gen/`, then runs:

```
capnp compile --no-standard-import -I"<std dir>" -ogo:gen --src-prefix=schema schema/*.capnp
```

`CAPNP` defaults to `capnp` on `PATH` (CI uses apt `capnproto`); if neither resolves, the script fails.

Output layout: `spikes/capnp/gen/handshake.capnp.go` and `spikes/capnp/gen/clipboard.capnp.go`, Go package `gen`,
import path `github.com/kikz0r/edgesync/spikes/capnp/gen`. The generated files are committed and never hand-edited.

Determinism (run in this worktree): `gen.sh` run once, `git add spikes/capnp/gen`, `gen.sh` run again, then
`git diff --exit-code spikes/capnp/gen` exits 0 (clean). The file IDs `@0x8518642835b94285` and `@0xc5f3611bd0ea5432`
come from `capnp id`.

## Run (host-safe tests, gates)

```
go -C spikes vet ./...
go -C spikes build ./...
go -C spikes test -count=1 ./capnp/...
GOOS=linux CGO_ENABLED=0 go -C spikes vet ./capnp/...
GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go -C spikes vet ./capnp/...
go -C spikes test -run "^$" -fuzz "^FuzzDecodeHello$" -fuzztime 30s ./capnp
go -C spikes test -bench . -benchmem -run "^$" ./capnp
```

## go-capnp API used (verified in the module source, v3.1.0-alpha.2)

- `capnp.NewMessage(arena Arena) (*Message, *Segment, error)` (message.go:65), with `capnp.SingleSegment(b []byte) Arena`.
- `capnp.Unmarshal(data []byte) (*Message, error)` (codec.go:155): no copy, reads directly from `data`.
- `Message.TraverseLimit uint64`: traversal budget in BYTES, default 64 MiB when 0 (message.go:52). The budget is
  initialised lazily on the first read, so setting it after `Unmarshal` and before any read applies.
- `Message.DepthLimit uint`: nesting depth, default 64 when 0 (message.go:56).
- `Message.Marshal() ([]byte, error)` (message.go:303).
- `Message.Root() (Ptr, error)` (message.go:210).
- `capnp.NewPointerList`, `PointerList.At`, `PointerList.Set` (list.go:336-371); `Set` of a pointer whose target is in
  the same message writes a pointer, it does not copy (segment.go:301-320), which is what makes the aliased bomb.
- `capnp.NewCompositeList(s, ObjectSize, n)`, `List.Struct(i) Struct` (list.go:51, 208). `List.Struct` panics on an
  out-of-range index (list.go:209-211): every loop here is bounded by `Len()`.
- `capnp.NewData(s, []byte) (UInt8List, error)`, `DataList.At`, `DataList.Set`, `Struct.SetPtr`, `Ptr.List`, `Ptr.Data`.
- Generated (capnpc-go): `NewRootHello`, `ReadRootHello`, `Hello.Macs() (capnp.DataList, error)`,
  `Hello_Desktop.Monitors() (Hello_Desktop_Monitor_List, error)` and the typed setters and getters.

Not used by this spike: `Decoder.MaxMessageSize` (the stream decoder), `Message.Reset`, `UnmarshalPacked`, RPC. The
oversize check is done by the spike wrapper before `Unmarshal`.

## Limits and observed errors

Configured values (architecture 5.3): traversal 8 Mi words = 64 MiB (`TraverseLimit = words * 8`), depth 32, control
message cap 16 MiB (`controlMaxMessageBytes`, checked before decode).

| Test | Input | Observed error |
|---|---|---|
| TestLimits_TraversalBombRejected | Hello whose `macs` holds 32 references to one 4 MiB blob (4 MiB message, under the cap) | element 15 fails: `read pointer: read traversal limit reached` (the root struct and list pointer reads consume part of the 64 MiB budget, so 15 of the 4 MiB reads fit). Control with a 1 GiB-word limit reads all 32 elements. |
| TestLimits_DepthLimitRejected, pointer chain | 40 nested pointer lists, DepthLimit 32 | after 30 dereferences: `read pointer: depth limit reached`. Control with DepthLimit 64 walks all 40 levels. |
| TestLimits_DepthLimitRejected, schema | Hello with one monitor, DepthLimit 4 | monitor `id` read: `read pointer: depth limit reached` |
| TestLimits_OversizedMessageRejected | 16 MiB + 1 zero bytes | `message exceeds control size cap: 16777217 bytes` (zeros would otherwise be accepted by `capnp.Unmarshal`; the test checks that precondition) |

## Finding 1: depth limit is bypassed through composite-list elements (for the parent)

go-capnp computes a composite-list element's depth as `p.depthLimit - 1` (list.go:225) with no guard. When the list
pointer itself is at depth 0 the subtraction wraps to the maximum `uint`, so the elements, and everything below them,
are no longer depth-limited. Observed:

- Hello with one monitor, DepthLimit 3: the monitor `id` read succeeds (`TestLimits_CompositeListDepthBypass`).
  With DepthLimit 4 the same read is rejected (`read pointer: depth limit reached`).
- A 40-level chain alternating composite lists and structs, DepthLimit 32: fully readable (the same chain of pointer
  lists is rejected at 30 dereferences).

The two spike schemas are non-recursive (Hello reaches at most 4 pointer dereferences, ClipAnnounce 3), so the schema bounds the
depth in practice. Recursive or AnyPointer-based schemas (service, IPC unions) would not be protected by the limit
alone. The characterisation test pins the current behaviour; if go-capnp fixes it, that test fails and the mitigation
must be re-decided. This contradicts the D-36 assumption that the depth limit is sufficient on its own, so it needs a
parent decision (for example a schema-depth check in `tools/capnpgen`, or an upstream fix).

## Finding 2: decoded input can expand past the cap when re-serialized

The traversal budget (64 MiB) is larger than the message cap (16 MiB). Aliased references accepted by the decoder
(one blob referenced many times) are copied on re-serialization, so an accepted input can re-encode to more than the
cap. `marshalCapped` rejects that with `errMessageTooLarge`. The fuzz property treats this outcome as the documented
expansion case. The first fuzz run found it (the re-encoded size was 17039664 bytes); the property was then corrected
and the run repeated.

## Fuzz and benchmarks (measured on this host)

Host: Windows 11, AMD Ryzen 7 7800X3D, GOOS=windows GOARCH=amd64, Go 1.27.3, `-benchmem`.

- `FuzzDecodeHello`, 30 s: PASS, 2,134,568 execs (no panic, decode stable under re-encoding).
- `BenchmarkEncodeHello`: 1235 ns/op, 1482 B/op, 5 allocs/op.
- `BenchmarkDecodeClipAnnounce`: 468.5 ns/op, 328 B/op, 7 allocs/op.

## What to observe, what to paste back

Nothing here needs a manual run. For a regeneration check, paste the output of `gen.sh` followed by
`git diff --exit-code spikes/capnp/gen`. For the limits, paste the `-v` output of
`go -C spikes test -count=1 -run TestLimits_ -v ./capnp/` (the logged observed errors).
