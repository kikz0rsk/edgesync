# SP5 core tech spikes - manual script

Item: P0-SP5-007   Machines: Windows PC 1, Windows PC 2, GNOME box
Estimated time: 150 minutes   Needs admin: no

Nothing in this script has been run by its author. The commands and flags were checked against the spike sources and
the READMEs of `spikes/gioeditor`, `spikes/quicbench`, `spikes/mdns`, `spikes/capnp`, `spikes/spake2` and `spikes/ipc`,
and the build paths were checked with a Go build dry run. No binary was started. Treat every output as the first real
evidence, not as a confirmed fact.

Conventions in this script:
- Every command runs from the repository root in Git Bash (the Git for Windows bash). PC 1 is `PC-A` and PC 2 is `PC-B`
  in the mDNS names, as in `spikes/mdns/README.md`.
- `<PC-1-ETH-IP>` and `<PC-2-ETH-IP>` are the IPv4 addresses of the wired Ethernet adapter of PC 1 and PC 2. Replace them
  with the real addresses, with no angle brackets.
- Each Windows binary is `spikes/bin/<name>.exe`; the GNOME box binaries have no `.exe`.

## Preparation
1. Check the repository state and record the commit: `git rev-parse --short HEAD`. Keep the printed hash for the results.
2. Check Go on each machine: `go version` must print `go version go1.27.3 windows/amd64` on the Windows PCs and
   `go version go1.27.3 linux/...` on the GNOME box. Go is installed at `D:\Program Files\Go\bin`. If `go` is not found in
   Git Bash, run `export PATH="/d/Program Files/Go/bin:$PATH"` in that Git Bash window only.
3. Check the Cap'n Proto compiler on PC 1 (Windows PC 1 only, the capnp steps need it):
   `"/d/Development/vcpkg/installed/x64-windows/tools/capnproto/capnp.exe" --version` must print `Cap'n Proto version 1.4.0`.
4. On PC 1 only, set the compiler path for `spikes/capnp/gen.sh` in the Git Bash window you use for step 9:
   `export CAPNP="/d/Development/vcpkg/installed/x64-windows/tools/capnproto/capnp.exe"`.
   `gen.sh` reads `CAPNP` and falls back to `capnp` on `PATH` when it is not set.
5. Build the Windows binaries on PC 1 and PC 2 (Go creates `spikes/bin/`; the directory is gitignored):
   ```
   go -C spikes build -o bin/gioeditor.exe ./gioeditor
   go -C spikes build -o bin/quicbench.exe ./quicbench/cmd/quicbench
   go -C spikes build -o bin/mdnsprobe.exe ./mdns/cmd/mdnsprobe
   ```
   Do not write `-o spikes/bin/...`: with `-C spikes` the `-o` path is relative to `spikes/`, so that form writes to
   `spikes/spikes/bin/`.
6. Build the GNOME box mDNS probe on the GNOME box, from the repository root:
   `go -C spikes build -o bin/mdnsprobe ./mdns/cmd/mdnsprobe`.
7. Put the three Windows PCs on the same wired LAN for the quicbench steps. Run `ipconfig` on PC 1 and PC 2 and note the
   IPv4 address of the Ethernet adapter as `<PC-1-ETH-IP>` and `<PC-2-ETH-IP>`. Turn Wi-Fi off on both PCs for steps 6 and
   7 so that the route is the wired one, and record the `ipconfig` output.
8. Record the Windows display scale (Settings > System > Display), the GPU and the CPU model of PC 1 and PC 2 for step 1.

## Steps
### Step 1 - Gio editor feel test on Windows PC 1 (and PC 2 if time allows)
- Do:
  1. Start the editor with four synthetic machines: `spikes/bin/gioeditor.exe`. Drag one machine next to another, zoom and
     pan. Controls: left drag moves a machine (snaps within 12 canvas units to facing real edges); Ctrl + mouse wheel
     zooms around the cursor; middle-button drag pans (also start the drag over a machine); arrow keys nudge the selected
     machine by 1 unit, Shift + arrow by 10 units; `T` toggles the light and dark palette. Close the window when done.
  2. Start the editor with 64 synthetic machines: `spikes/bin/gioeditor.exe -machines 64`. Keep dragging and panning for
     about 10 seconds so frames are rendered (the editor redraws only on input). Then close the window.
  3. Copy the stdout of both runs (the lines `machines=... frames=... fps=... frame_avg=...ms frame_max=...ms`, one per
     second with rendered frames).
- Expect: the window opens without a C toolchain (CGO_ENABLED=0 is the default on this host). Snap guides (orange) appear
  while dragging and the machine stops at the facing edge. A three-monitor machine snaps and highlights only at its outer
  edges, never at the seams between its own monitors. The green overlap segment appears only where facing real edges share
  a range. Zoom keeps the point under the cursor in place. Arrow keys move exactly 1 unit, Shift + arrow 10 units. Every label
  is readable in both palettes. With 64 machines, the `frame_avg` and `frame_max` values are recorded; the drag stays
  smooth to the eye.
- Record: the two stdout blocks verbatim; the Windows display scale, GPU and CPU; yes or no for each item of the "What to
  observe" list in `spikes/gioeditor/README.md`, and whether the 12-unit snap threshold should be a constant screen distance
  instead; anything that behaved unexpectedly.

### Step 2 - mDNS round 1 (hashicorp, all three machines at once)
- Do: on each machine open two Git Bash windows in the repository root. Start the announce and the browse on every machine
  within about 10 seconds of each other.
  - PC 1, window 1: `spikes/bin/mdnsprobe.exe -lib hashicorp -mode announce -name PC-A`
  - PC 1, window 2: `spikes/bin/mdnsprobe.exe -lib hashicorp -mode browse`
  - PC 2, window 1: `spikes/bin/mdnsprobe.exe -lib hashicorp -mode announce -name PC-B`
  - PC 2, window 2: `spikes/bin/mdnsprobe.exe -lib hashicorp -mode browse`
  - GNOME box, window 1: `spikes/bin/mdnsprobe -lib hashicorp -mode announce -name gnome`
  - GNOME box, window 2: `spikes/bin/mdnsprobe -lib hashicorp -mode browse`
  Let all six run for about 60 seconds, then stop them: announces first (Ctrl+C), browses last (Ctrl+C). The first run on a
  Windows PC may show a Windows Defender Firewall prompt for `mdnsprobe.exe`; see the Safety section.
- Expect: each process first prints `START`, then `IFACE` lines (name, flags, addresses, `bound=`) and a `BIND` line.
  Each announce prints `ANNOUNCE hashicorp ...` and, on Ctrl+C, `SHUTDOWN hashicorp ... no goodbye packet`. Each browse
  prints `FOUND hashicorp ...` for the two other machines (and `UPDATE` when their data changes), and `SUMMARY lib=hashicorp
  found=<n> first_discovery=<s>` on exit. The browse on each machine should find both other machines. hashicorp prints no
  `GONE` lines.
- Record: the output from `START` to `SUMMARY` of each of the six processes, verbatim; the first discovery per machine pair
  in seconds; whether all three machines saw each other; the Windows firewall prompt, if one appeared (yes or no, the
  network profile offered (private, public or both), allowed or not, and whether browse found peers before and after
  allowing it); any `[ERR]` or `hashicorp/mdns` line on stderr.

### Step 3 - mDNS round 2 (dnssd, one process per machine, three passes)
- Do: run one dnssd process per machine in each pass. In each pass start the browse first, then the two announces, and run
  the pass for about 60 seconds. Stop the announces first (Ctrl+C), then the browse (Ctrl+C), so the browse prints its
  `GONE` lines and its `SUMMARY`.
  - Pass 2a:
    - PC 1: `spikes/bin/mdnsprobe.exe -lib dnssd -mode browse`
    - PC 2: `spikes/bin/mdnsprobe.exe -lib dnssd -mode announce -name PC-B`
    - GNOME box: `spikes/bin/mdnsprobe -lib dnssd -mode announce -name gnome`
  - Pass 2b:
    - PC 1: `spikes/bin/mdnsprobe.exe -lib dnssd -mode announce -name PC-A`
    - PC 2: `spikes/bin/mdnsprobe.exe -lib dnssd -mode browse`
    - GNOME box: `spikes/bin/mdnsprobe -lib dnssd -mode announce -name gnome`
  - Pass 2c:
    - PC 1: `spikes/bin/mdnsprobe.exe -lib dnssd -mode announce -name PC-A`
    - PC 2: `spikes/bin/mdnsprobe.exe -lib dnssd -mode announce -name PC-B`
    - GNOME box: `spikes/bin/mdnsprobe -lib dnssd -mode browse`
  Do not start a second dnssd process on the same machine: it either fails to start or runs with one IP family missing,
  silently.
- Expect: dnssd announces print `ANNOUNCE dnssd ...` on start and `SHUTDOWN dnssd goodbye issued ...` on Ctrl+C. A browse
  prints `FOUND dnssd name=... host=... port=24860 iface=<adapter> ips=[...] txt=[...]` for each announcer, with the
  `iface=` value naming the adapter the record arrived on, and `GONE dnssd name=... iface=...` when an announcer sends its
  goodbye. The first discovery should follow the start of the announce within about 1 to 2 seconds. Check that
  `txt=` carries the keys `v`, `id`, `role` and `tcp`, with `v` 1, `role` client and `tcp` 24860.
- Record: the output from `START` to `SUMMARY` of each browse, and the `ANNOUNCE`/`SHUTDOWN` lines of each announce, verbatim;
  for each pass, which browse saw which announcer and the `iface=` values; the time from announce start to first discovery;
  whether `GONE` appeared within seconds of the goodbye; any `[ERR]` line.

### Step 4 - VPN adapter connected and disconnected (WireGuard or Netbird)
- Do: do this for PC 1 first, then for PC 2. For the PC under test, connect the VPN adapter and run step 2 (all three
  machines, hashicorp) and step 3 pass 2a (dnssd, PC 1 browses, PC 2 and the GNOME box announce). Then disconnect the VPN
  adapter on that PC and run the same two runs again. Note the VPN adapter's name from the `IFACE` lines of each run.
  Connect and disconnect the VPN only through its own application; do not edit adapters or routes by hand.
- Expect: the `IFACE` lines show the VPN adapter with its addresses while connected and without it when disconnected. For
  each run record whether a peer reachable only over the VPN appears in browse output, whether the announce lines list VPN
  addresses (hashicorp advertises every non-loopback, non-link-local address of the interfaces it binds), and the `iface=`
  value (dnssd) or `iface-by-subnet=` value (hashicorp).
- Record: for each of the four states (PC 1 connected, PC 1 disconnected, PC 2 connected, PC 2 disconnected) the VPN adapter
  name, the peers and addresses that appeared or disappeared, and the `iface=` or `iface-by-subnet=` values, with the full
  output of the browse runs.

### Step 5 - mDNS per-adapter and address-family runs (PC 1 and PC 2)
- Do:
  1. Read the Ethernet adapter name from the `IFACE` lines of step 2 (the `name=` value, for example `Ethernet`). Replace
     `<Ethernet-name>` with it, without angle brackets, on both PCs:
     - PC 1: `spikes/bin/mdnsprobe.exe -lib hashicorp -mode announce -name PC-A -ifaces "<Ethernet-name>"`
     - PC 2: `spikes/bin/mdnsprobe.exe -lib hashicorp -mode browse -ifaces "<Ethernet-name>"`
     Run them for about 30 seconds, then stop them with Ctrl+C (announce first).
  2. IPv4 and IPv6 query on PC 2 (hashicorp browse only), 30 seconds each. Start the announce on PC 1 again first:
     `spikes/bin/mdnsprobe.exe -lib hashicorp -mode announce -name PC-A`
     - `spikes/bin/mdnsprobe.exe -lib hashicorp -mode browse -ipv6=false`
     - `spikes/bin/mdnsprobe.exe -lib hashicorp -mode browse -ipv4=false`
  3. Optional, dnssd announce on one adapter (announce only; dnssd browse does not accept `-ifaces`). Stop the announce of
     item 2 first. PC 1: `spikes/bin/mdnsprobe.exe -lib dnssd -mode announce -name PC-A -ifaces "<Ethernet-name>"`. PC 2:
     `spikes/bin/mdnsprobe.exe -lib dnssd -mode browse`. GNOME box: `spikes/bin/mdnsprobe -lib dnssd -mode announce -name gnome`.
     Stop the announces first (Ctrl+C), then the browse.
- Expect: the `BIND` line names the chosen adapter (hashicorp) or the adapter list (dnssd). The `-ipv6=false` browse finds
  no IPv6 address (`v6=` empty); the `-ipv4=false` browse finds no IPv4 address. A failed `-ifaces` run prints an error line
  (`interface "<name>": ...`) and exits with status 1; record it verbatim.
- Record: the output of each run; for `-ifaces`, whether the browse on PC 2 still found PC 1; the IPv4 and IPv6 address
  families each browse saw; any error line verbatim.

### Step 6 - quicbench LAN, PC 1 as server, PC 2 as client (wired Ethernet)
- Do: PC 1 is the server and PC 2 is the client. The port is 24860 (`quicbench` uses the product port, as named in
  `spikes/quicbench/README.md`). Start the server first and keep it running for both client runs.
  1. PC 1, window 1: `spikes/bin/quicbench.exe -mode server -addr :24860`
     Wait for `listening on` in the output. The first run may show a Windows Defender Firewall prompt for
     `quicbench.exe` (UDP inbound); see the Safety section.
  2. PC 2, window 2, without bulk: `spikes/bin/quicbench.exe -mode client -addr <PC-1-ETH-IP>:24860 -duration 30s`
  3. PC 2, window 2, with bulk: `spikes/bin/quicbench.exe -mode client -addr <PC-1-ETH-IP>:24860 -duration 30s -bulk`
  4. Stop the server on PC 1 with Ctrl+C.
- Expect: the server prints `listening on <addr>` and then `<peer> connected` for each client run, followed by a
  `bulk received <n> bytes` line when bulk ran, a `closed:` line when the client disconnects, and an `input echo` or
  `datagram echo` line only on an error. The client prints `datagram: sent=... received=... lost=... one-way
  n=... p50=... p99=... max=...` and `input stream: ...` with the same fields. The run with `-bulk` also prints
  `bulk sent <n> bytes, <x> MiB/s`. Compare the input stream p99 without and with bulk against PB-02 (p99 <= 5 ms). Compare
  the datagram p99 and max without and with bulk. Record the lost count. The bulk throughput is compared against PB-11
  (>= 105 MiB/s on GbE). The one-way values are RTT/2 measured on the client clock.
- Record: the output of the client runs 2 and 3, verbatim (the `datagram`, `input stream` and `bulk` lines); the server
  output lines for each run; the Ethernet IPv4 addresses and the `ipconfig` output showing Wi-Fi off; the link speed of the
  Ethernet adapter (Settings > Network & internet > Ethernet, or the adapter properties); the firewall prompt, if any.

### Step 7 - quicbench LAN, PC 2 as server, PC 1 as client (role swap)
- Do: the same as step 6 with the roles swapped. Start the server on PC 2 and keep it running, then run the client on PC 1.
  1. PC 2, window 1: `spikes/bin/quicbench.exe -mode server -addr :24860`
  2. PC 1, window 2, without bulk: `spikes/bin/quicbench.exe -mode client -addr <PC-2-ETH-IP>:24860 -duration 30s`
  3. PC 1, window 2, with bulk: `spikes/bin/quicbench.exe -mode client -addr <PC-2-ETH-IP>:24860 -duration 30s -bulk`
  4. Stop the server on PC 2 with Ctrl+C.
- Expect: as in step 6, with the directions reversed. `-bulk` sends from PC 1 to PC 2, so the `bulk received` line appears on
  the server.
- Record: the output of both client runs, verbatim, and the server output lines; any difference from step 6 in the input
  stream p99 or the datagram lost count.

### Step 8 - SPAKE2 vectors and benchmark on each machine (PC 1, PC 2, GNOME box)
- Do: on each machine, in the repository root:
  ```
  go -C spikes test -count=1 -v ./spake2/...
  go -C spikes test -run "^$" -bench BenchmarkSPAKE2Full -benchmem ./spake2
  ```
- Expect: the `-v` run ends with `ok` for `github.com/kikz0r/edgesync/spikes/spake2`. `TestRFC9382_VectorsP256SHA256` shows
  four passing subtests (the four RFC 9382 Appendix B vectors) and `TestExporter_BindingDiffersPerConnection` passes. The
  benchmark prints one `BenchmarkSPAKE2Full-<n>` line with ns/op, B/op and allocs/op. The author measured about 0.29 ms per
  exchange (both sides), 12,944 B/op and 146 allocs/op; the budget PB-22 is 50 ms per side.
- Record: the CPU model and the GNOME box kernel and CPU; the `-v` output (the `--- PASS` lines of the vector and exporter
  tests and the final `ok` line); the benchmark line verbatim; any FAIL line verbatim.

### Step 9 - Cap'n Proto regeneration determinism and limit tests (PC 1 only)
- Do: in the Git Bash window where Preparation step 4 set `CAPNP`, from the repository root:
  ```
  bash spikes/capnp/gen.sh
  echo "gen exit $?"
  git diff --exit-code spikes/capnp/gen
  echo "diff exit $?"
  go -C spikes test -count=1 -run TestLimits_ -v ./capnp/
  ```
- Expect: `gen.sh` prints nothing on success and `gen exit 0`. `git diff --exit-code spikes/capnp/gen` prints nothing and
  `diff exit 0`: regenerating the schemas reproduces the committed `handshake.capnp.go` and `clipboard.capnp.go` byte for
  byte. The limit tests end with `ok`. Their `-v` output logs the observed errors, which should match the README table: the
  traversal bomb fails at element 15 with `read traversal limit reached`; the pointer-chain depth limit fails after 30
  dereferences with `read pointer: depth limit reached`; the schema depth check with DepthLimit 4 fails on the monitor `id`
  read; the 16 MiB + 1 byte message fails with `message exceeds control size cap: 16777217 bytes`.
  `TestLimits_CompositeListDepthBypass` is a characterisation test: it passes when go-capnp v3.1.0-alpha.2 still does not
  enforce the depth limit for composite-list elements (D-124).
- Record: the output of `gen.sh` (or the word `none`), the two `exit` lines, the `git diff` output if it is not empty, and
  the `-v` output of the limit tests verbatim, with the observed error text of each `t.Log` line. If `gen.sh` fails, paste the
  error verbatim.

### Step 10 - IPC pipe tests and round-trip benchmark (PC 1 and PC 2)
- Do: on each Windows PC, in the repository root:
  ```
  go -C spikes test -count=1 -v ./ipc
  go -C spikes test -count=1 -run=NONE -bench=BenchmarkPipeRoundTrip1KiB ./ipc
  ```
  The tests create a named pipe with a random name under `\\.\pipe\edgesync-spike-ipc-` and never touch the real agent pipe.
  The Unix socket test is not part of these Windows runs (it has a linux or darwin build tag); the spikes CI workflow runs it
  on Linux and macOS.
- Expect: `TestPipe_OwnerOnlySDDL` and `TestPipe_RoundTripLatency1KiB` pass. The DACL test asserts the stored DACL
  `D:P(A;;FA;;;SY)(A;;FA;;;<user SID>)` with this machine's user SID, a second listener on the same pipe name is refused, and a
  client that expects another server SID is rejected. The round-trip test reports a mean in the tens of microseconds on an
  idle machine (the author measured 18 us). The benchmark prints `BenchmarkPipeRoundTrip1KiB-<n>` with ns/op, B/op and
  allocs/op (the author measured about 22 us/op, 640 B/op, 8 allocs/op). A result in milliseconds is recorded, not hidden;
  note whether antivirus was active.
- Record: the `-v` output verbatim, with the user SID line if the test logs it; the benchmark line verbatim; the mean
  round-trip value of `TestPipe_RoundTripLatency1KiB`; any FAIL line.

## Cleanup
1. Stop every running process with Ctrl+C in its window: `gioeditor.exe`, `mdnsprobe` (all six in step 2 and the GNOME
   processes), `quicbench.exe` (server and client). Close the Git Bash windows.
2. Disconnect the VPN adapter again if you connected it for step 4 and it was not connected before the run.
3. If you allowed a Windows Defender Firewall prompt for `mdnsprobe.exe` or `quicbench.exe`, you may remove the entry in
   Windows Security > Firewall & network protection > Allow an app through firewall. Nothing else was changed by the script.
4. Run `git status --short`; it must print nothing. The spike binaries are in the gitignored `spikes/bin/`, and
   `bin/capnpc-go.exe` (written by `gen.sh`) is in the gitignored `/bin/`. If `spikes/capnp/gen` shows a change, do not
   commit it; paste the diff.
5. Paste the recorded observations into `qa/manual/results/p0-sp5-core.md`, using the results format of
   `qa/manual/README.md` (date, tester, commit hash from step 1 of Preparation, machine descriptions, one row per step and
   the verdict), or paste them in chat for the parent to commit.

## Safety
- The Gio editor is a plain window. It installs no input hooks and injects no input, so it cannot take over the keyboard or
  mouse. To leave it, close the window (Alt + F4) or press Ctrl+C in its Git Bash window. If the window is off-screen, press
  Alt + Space then M and move the window with the arrow keys.
- `mdnsprobe` and `quicbench` open network sockets only. Stop them with Ctrl+C. If a process does not stop, close its Git
  Bash window or end `mdnsprobe.exe`, `quicbench.exe` or `gioeditor.exe` in Task Manager. Nothing is installed and no
  service is started by any step.
- Windows Defender Firewall prompts: choose the Private network profile only, record what was offered, and do not create or
  edit firewall rules by hand, as `spikes/mdns/README.md` requires for the discovery runs. If you deny the prompt, the probe
  still runs and the results show the missing peers; record that instead.
- Do not connect the VPN adapter from the command line and do not change adapter or route settings. Use the VPN application's
  own connect and disconnect buttons.
- The capnp steps only regenerate files inside `spikes/capnp/gen` and the gitignored `bin/`. If `git diff` shows a change in
  `spikes/capnp/gen`, stop and paste the diff; do not run `git checkout` or `git stash` to hide it.
