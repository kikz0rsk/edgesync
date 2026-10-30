# mDNS discovery comparison (P0-SP5-003)

## Purpose

Choose the mDNS/DNS-SD library for `internal/discovery` (D-15, D-43, R-35). Two candidates:

- `github.com/hashicorp/mdns` v1.0.7 (MIT): query/answer server and client.
- `github.com/brutella/dnssd` v1.2.14 (MIT): full responder with probing, announcements and goodbye packets.

One probe binary, `cmd/mdnsprobe`, runs either library, selected by `-lib`. It announces or browses the service type `_edgesync._udp.local.` with the TXT keys fixed by D-43:

- `v=1` (protocol version)
- `id=<random>` (peer ID placeholder)
- `role=client` (or `server`)
- `tcp=24860` (TCP fallback port)

No fingerprint or other key is added; the security review (qa/security-review.md) allows only these four keys.

Every output line starts with the time since the probe started, taken from the monotonic clock (`time.Since`). The probe prints the interfaces at start (`IFACE` lines, with flags and addresses) and the interfaces it binds (`BIND` lines). Discovery times are in seconds, so the D-123 QueryPerformanceCounter rule does not apply.

Rules (D-74): agents build this code and run `go vet` only. The user runs the binaries. Multicast must not be sent from the development host.

## Build

From the repository root, Git Bash or PowerShell:

```
go -C spikes vet ./mdns/...
go -C spikes build -o bin/mdnsprobe.exe ./mdns/cmd/mdnsprobe
```

Expected: vet and build print nothing and exit 0. The binary is written to `spikes/bin/mdnsprobe.exe` (gitignored). Cross-compile checks: `GOOS=linux CGO_ENABLED=0 go -C spikes vet ./mdns/...` and `GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go -C spikes vet ./mdns/...`.

GNOME box (Linux, no cgo needed):

```
go -C spikes build -o bin/mdnsprobe ./mdns/cmd/mdnsprobe
```

## Flags

| Flag | Default | Meaning |
|---|---|---|
| `-lib` | (required) | `hashicorp` or `dnssd` |
| `-mode` | (required) | `announce` or `browse` |
| `-name` | host name | instance name (announce) |
| `-port` | 24860 | advertised port (announce) |
| `-tcp` | 24860 | value of the TXT `tcp` key |
| `-role` | client | value of the TXT `role` key (`server` or `client`) |
| `-id` | random 8 bytes, hex | value of the TXT `id` key |
| `-ifaces` | library default | comma-separated interface names. `hashicorp`: at most one. `dnssd browse`: not allowed (see below) |
| `-ipv4`, `-ipv6` | true | `hashicorp` browse only: query over that family |
| `-window` | 3s | `hashicorp` browse only: length of one query round |

## Run

Each probe prints `START`, `IFACE`, `BIND`, then `ANNOUNCE` or `FOUND` lines, and `SUMMARY` on browse. Stop with Ctrl+C. Announce on shutdown prints `SHUTDOWN`.

Names: PC-A and PC-B are the two Windows PCs (use their host names as `-name`, for example `-name PC-A`). GNOME is the GNOME box (`-name gnome`).

Round 1, `hashicorp`, all three machines at the same time, two terminals each:

```
PC-A terminal 1:  spikes\bin\mdnsprobe.exe -lib hashicorp -mode announce -name PC-A
PC-A terminal 2:  spikes\bin\mdnsprobe.exe -lib hashicorp -mode browse
PC-B terminal 1:  spikes\bin\mdnsprobe.exe -lib hashicorp -mode announce -name PC-B
PC-B terminal 2:  spikes\bin\mdnsprobe.exe -lib hashicorp -mode browse
GNOME terminal 1: ./bin/mdnsprobe -lib hashicorp -mode announce -name gnome
GNOME terminal 2: ./bin/mdnsprobe -lib hashicorp -mode browse
```

Round 2, `dnssd`: run at most one dnssd process per machine at a time. A second dnssd process on the same machine cannot bind 5353 (see "Library behaviour" below). Use the same three machines with one role each, then rotate so every machine browses once:

```
Pass 2a: PC-A  -lib dnssd -mode browse
         PC-B  -lib dnssd -mode announce -name PC-B
         GNOME -lib dnssd -mode announce -name gnome
Pass 2b: PC-A  -lib dnssd -mode announce -name PC-A
         PC-B  -lib dnssd -mode browse
         GNOME -lib dnssd -mode announce -name gnome
Pass 2c: PC-A  -lib dnssd -mode announce -name PC-A
         PC-B  -lib dnssd -mode announce -name PC-B
         GNOME -lib dnssd -mode browse
```

Start the browse first in each pass, then the announces, and run each pass for about 60 seconds. Then stop everything with Ctrl+C in the reverse order.

Adapter and VPN runs: repeat round 1 and pass 2a with the WireGuard or Netbird adapter connected, then disconnected, on each Windows PC. Record the `IFACE` lines of each run. Add `-ifaces <name>` for one run per machine (for example the Ethernet adapter), with `-lib hashicorp` (one name) or `-lib dnssd -mode announce` (any number).

IPv4 and IPv6 runs (hashicorp browse only): add `-ipv4=false` for an IPv6-only query, or `-ipv6=false` for an IPv4-only query, on PC-A.

Windows firewall: the first run may show a prompt for `mdnsprobe.exe`. Record the network profile offered (private, public or both), whether you allowed it, and whether browse found peers before and after allowing it. Do not change firewall rules by hand for this test.

## What to observe

Time to first discovery:

- `SUMMARY ... first_discovery=` for each browse run (seconds since the browse started).
- hashicorp answers queries only, so its first discovery depends on the first query round (`-window`, default 3 s). dnssd announces on start and listens, so its first discovery should follow the start of the announce within about 1 to 2 seconds (probing takes about 1 second).

Interfaces:

- The `IFACE` and `BIND` lines for each machine, with and without the VPN adapter.
- dnssd `FOUND` lines name the interface on which the A record arrived (`iface=`). Compare with the IP list (`ips=`). A peer reached only through an adapter that is not the default route should show that adapter.
- hashicorp `FOUND` lines do not carry the arrival interface. The `iface-by-subnet=` field is inferred by matching the advertised IPv4 (or IPv6) address to the local subnets. Treat it as a hint, not a measurement.

VPN adapters (WireGuard, Netbird):

- Does a peer that is reachable only over the VPN appear in browse output?
- Do announce lines list VPN addresses? (hashicorp advertises every non-loopback, non-link-local address of the selected interfaces, or all interfaces when `-ifaces` is not given.)

IPv4 vs IPv6:

- Address families in `v4=` and `v6=` (hashicorp) and `ips=` (dnssd). Note whether any peer is seen only on IPv6.

Goodbye handling:

- dnssd: after Ctrl+C on an announcing machine, the browse on another machine should print `GONE dnssd ...` within seconds, and the announce should print `SHUTDOWN dnssd goodbye issued`.
- hashicorp: the announce prints `no goodbye packet`. Its browse has no disappearance output at all, so note whether the stopped announcer is still shown in later browse rounds.

Multiple adapters on the announcing side:

- hashicorp listens on one interface only (`BIND` line, or the system default when `-ifaces` is absent). With two adapters on the same subnet, check whether a browse on the other machine still answers.
- dnssd announces on each selected interface (two announcements per interface, one second apart). Check the `iface=` values on the browsing side.

Known library limitations to check against the output:

- dnssd: sends per interface via `ControlMessage.IfIndex`. The x/net/ipv4 package notes that on Windows the control message for WriteTo is not implemented (`ipv4/payload.go`, BUG note). If announcements land on one adapter only on Windows, record that.
- dnssd: interfaces are joined only when the responder is created. An adapter that comes up later is not joined.
- hashicorp: `NewServer` ignores the error from each family's multicast bind. A family that failed to bind is silently absent; the probe cannot show it. Check that the advertised address families match what browse sees.

## Library behaviour (from the module source, v1.0.7 and v1.2.14)

hashicorp/mdns:

- `mdns.NewMDNSService(instance, service, domain, hostName string, port int, ips []net.IP, txt []string) (*MDNSService, error)` (zone.go). The probe passes the IPs explicitly, so no hostname lookup runs.
- `mdns.NewServer(config *mdns.Config) (*Server, error)` with `Config{Zone, Iface *net.Interface, Logger *log.Logger}` and `(*Server).Shutdown() error` (server.go). Shutdown closes the sockets; no goodbye is sent.
- `mdns.DefaultParams(service string) *QueryParam` and `mdns.QueryContext(ctx, params *QueryParam) error` (client.go). `QueryParam` fields used: `Entries`, `Timeout`, `Interface`, `DisableIPv4`, `DisableIPv6`, `Logger`. Each call opens new sockets and returns after `Timeout`.
- `mdns.ServiceEntry` fields used: `Name`, `Host`, `AddrV4`, `AddrV6`, `AddrV6IPAddr.Zone`, `Port`, `InfoFields`.

brutella/dnssd:

- `dnssd.NewService(cfg dnssd.Config) (dnssd.Service, error)` with `Config{Name, Type, Domain, Host, Text, Port, Ifaces}` (service.go).
- `dnssd.NewResponder() (dnssd.Responder, error)`, `Responder.Add(srv) (ServiceHandle, error)`, `Responder.Respond(ctx) error` (responder.go). On context cancel, Respond sends goodbye packets (TTL 0 PTR, twice per interface) and returns `ctx.Err()`.
- `ServiceHandle.Service() dnssd.Service` (serviceHandle.go).
- `dnssd.LookupType(ctx, service string, add dnssd.AddFunc, rmv dnssd.RmvFunc) error` with `BrowseEntry{IPs, Host, Port, IfaceName, Name, Type, Domain, Text}` (browse.go). The service string carries the trailing dot: `_edgesync._udp.local.`.
- `dnssd.MulticastInterfaces(filters ...string) []*net.Interface` (service.go).

Other behaviour from the source:

- dnssd binds `224.0.0.251:5353` and `[ff02::fb]:5353` with plain `net.ListenUDP` and returns an error only when both families fail (mdns.go, `newMDNSConn`). A second dnssd process on the same machine therefore either fails or runs without one family, silently. That is why round 2 uses one dnssd process per machine.
- dnssd browse sends one query per multicast interface at start, then listens. It does not re-query periodically.
- dnssd `LookupType` has no interface parameter. The probe rejects `-ifaces` for dnssd browse.
- Neither library re-probes or re-announces on adapter changes (dnssd README TODO: no hot-plug support).

## What to paste back

For every run, paste the terminal output from `START` to `SUMMARY` (or `SHUTDOWN`) verbatim. Then answer:

1. Round 1: first discovery per machine pair (seconds), and whether all three saw each other.
2. Pass 2a, 2b, 2c: which browse runs saw which announcers, and any `GONE` lines.
3. VPN adapter connected vs disconnected: which peers and addresses appeared or disappeared, and the `iface=` or `iface-by-subnet=` values.
4. Any `[ERR]` or `hashicorp/mdns` log lines on stderr.
5. Windows firewall prompt: yes or no, which profile, and the result of allowing it.
6. Any `-ifaces` run that failed, with the error line.
