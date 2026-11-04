# P0-SP5 core tech spikes - results (orchestrator-run part, D-126)

Date: 2026-11-04. Host: the dev PC (hostname ALPHA, Windows 11). Script: `qa/manual/p0-sp5-core.md`. Status: PARTIAL. P0-SP5-M01 stays open:
the two-PC, GNOME-box and VPN parts need the user (see "Still needs you").

## mDNS (steps 2 to 5 of the script, single-PC subset)

Binary: `spikes\bin\mdnsprobe.exe`. Service `_edgesync._udp.local.`, TXT keys per D-43 (`id`, `role`, `tcp`, `v`). Each probe was stopped by killing the
process after a fixed time (about 20 s), so goodbye packets (SHUTDOWN/GONE) were NOT exercised. No stderr output in any run.

Interfaces on this PC as enumerated by the probe: `Ethernet` (up, 192.168.100.10/24 plus global and link-local IPv6), `Ethernet 2` (up, VirtualBox-style
host-only 192.168.56.1/24), `vEthernet (Default Switch)` (up, Hyper-V 172.23.240.1/20), `Loopback Pseudo-Interface 1` (up), and several adapters that are down
(`Wi-Fi`, `OpenVPN TAP-Windows6`, `OpenVPN Data Channel Offload`, two `Local Area Connection*`, `Bluetooth Network Connection`). No WireGuard or Netbird adapter was
present or up on this PC during the run, so the VPN-connected case was NOT covered.

| Run | Result |
|---|---|
| A: hashicorp announce (`-name spike-self`) plus hashicorp browse, both on this PC, 20 s | browse found NOTHING (0 FOUND). The hashicorp announcer logs "answers queries only, no unsolicited announcement" and binds "the system default multicast interface (the library does not report which one)". Same-host announce/browse with this library is not a usable self-test on Windows. |
| B: dnssd announce (`-name spike-dnssd`) plus hashicorp browse, both on this PC, 20 s | FOUND in 0.515 s: `spike-dnssd._edgesync._udp.local.` host `ALPHA.local.` port 24860, TXT `id`, `role=client`, `tcp=24860`, `v=1`. Cross-library discovery works. |

Findings:
1. The IPv4 address returned by the browser was `172.23.240.1` (the Hyper-V Default Switch adapter, `iface-by-subnet=vEthernet (Default Switch)`), NOT the LAN address `192.168.100.10`. A peer that trusted a single resolved address would dial an address that is unreachable from the LAN. The discovery code in the product must treat every resolved address as a candidate (try all, prefer the subnet of the receiving interface, skip loopback, link-local when a routable one exists, and known virtual adapters), and manual add-by-IP stays essential (D-15).
2. dnssd announces on every up interface it binds: `Ethernet`, `Ethernet 2`, `Loopback Pseudo-Interface 1` and `vEthernet (Default Switch)` all showed `bound=true`/`BIND`. Loopback and virtual adapters are advertised as well; the product should pass an explicit interface list.
3. dnssd logs `dnssd: unable to wait for link updates` on Windows (`netlink_others.go`): it does not follow adapter changes at run time here; a periodic re-announce or restart on network change would be needed if dnssd were chosen.
4. hashicorp (announce) stays silent until queried and does not report the interface it uses; dnssd probes, then announces per interface and reports the bound interfaces. For the choice at P0-SP5-008 this favours dnssd for announcing and either library for browsing, subject to the two-PC and GNOME results.
5. No Windows firewall prompt text was captured (the orchestrator cannot see the screen); no error output was produced.

## Other SP5 spikes already evidenced elsewhere

Loopback numbers (quicbench), Cap'n Proto limits, SPAKE2 vectors and benchmark, and IPC tests are in `plan.md` NOTE blocks and the spike READMEs; they were produced by agent `go test` runs, not by this session.

## Still needs you (P0-SP5-M01)

1. mDNS on two machines: the same runs with PC-A, PC-B and the GNOME box at once (round 1 and the dnssd passes 2a to 2c), with a WireGuard or Netbird adapter connected and disconnected, and the IPv4-only / IPv6-only browse runs (`-ipv4=false`, `-ipv6=false`).
2. quicbench LAN between the two Windows PCs on wired Ethernet, with and without `-bulk`, both role directions.
3. The Gio layout editor feel test (4 and 64 machines) with your hands on the mouse.
4. SPAKE2 and capnp regeneration and IPC pipe tests on the second PC and the GNOME box.
