# winpipe - named-pipe ACLs, peer verification and TLS signing proxy spike (P0-SP1-005)

Purpose: prove or disprove the pipe half of D-46 / D-47 and the signing half of D-41 on a real Windows PC. A per-user
agent completes a TLS 1.3 handshake whose private-key operation is performed by the process that holds the key (the
service in production, `pipeserver` here) over an authenticated named pipe. The key never leaves the server process.

Spike rules (D-74): agents build this code and run only host-safe tests (local named pipes and in-process fakes). YOU run
the two binaries, following `qa/manual/p0-sp1-windows.md` (P0-SP1-006). Nothing here installs a service or changes
system settings.

## What is in this directory

- `sddl_windows.go`: `UserPipeSDDL` builds `D:P(A;;GA;;;SY)(A;;GA;;;<user SID>)` (owner-only, protected DACL) and
  `ServiceSDDL` is `D:P(A;;GA;;;SY)(A;;GRGW;;;AU)` (D-47 service pipe). `ParseDACL` parses an SDDL string back and lists
  its ACEs.
- `verify_windows.go`: `VerifyServer` (client side: the pipe server process must run as the expected account),
  `ClientPeer` (server side: client process SID, session and image, read from one process handle per message).
- `signer.go` (OS-independent): wire format, `Authorize` (account, session and image must all match), `ProxySigner`
  (agent-side `crypto.Signer`), `Responder` (service-side, holds the key, checks the peer before every message),
  `SelfSignedLoopbackCert` and `LoopbackHandshake` (TLS 1.3 over 127.0.0.1).
- `cmd/pipeserver`: the signing proxy stand-in. `cmd/pipeclient`: the agent side.
- Tests: `signer_test.go` (host-safe, in-process: handshake over loopback, hostile-client table, framing, latency) and
  `pipe_windows_test.go` (host local pipes: SDDL ACE list, first-instance rejection, SID verification, TLS through a pipe).

## Wire protocol (signer.go)

Frame: 4-byte big-endian length, payload 1..4096 bytes. Request: `op (1) | seq (8) | body`; response:
`status (1) | seq (8) | body`. `OpPublicKey` (1) returns the PKIX public key; `OpSign` (2) takes the SHA-256 identifier
byte and a 32-byte digest and returns an ASN.1 ECDSA signature. The client verifies the signature before using it.
Sequence numbers must strictly increase per connection (replay and reordering are refused with `StatusStale`). Any refusal
is answered with one status frame and then the connection closes.

## Build (run from the repository root)

```
go -C spikes build -o bin/pipeserver.exe ./winpipe/cmd/pipeserver
go -C spikes build -o bin/pipeclient.exe ./winpipe/cmd/pipeclient
```

## Host tests (agents run these; the user may too)

```
go -C spikes test -count=1 -v ./winpipe/...
```

These create local named pipes under the `\\.\pipe\edgesync-winpipe-test-*` namespace only.

## Run 1 - same account, positive path (the user runs this)

Open two Windows terminals as the same user. Terminal A (server):

```
spikes\bin\pipeserver.exe -duration 5m
```

It defaults to: pipe `\\.\pipe\EdgeSyncWinpipeSpike`, the current account SID, the current session, and the expected
client image `pipeclient.exe` in the same directory. Terminal B (client):

```
spikes\bin\pipeclient.exe -handshakes 10
```

Expected: `pipeclient` prints one line per handshake (`local key` and `proxy` durations, `TLS version 0x304`) and a
median with the overhead in milliseconds. `pipeserver` logs `serving` once and `client disconnected cleanly` when the
client exits. Stop the server with Ctrl+C or let `-duration` expire.

## Run 2 - second Windows account must be refused (the user runs this)

Keep Run 1's server running in terminal A. From a second Windows account (a different user SID, signed in on this PC) run:

```
spikes\bin\pipeclient.exe
```

Expected: the dial fails before any TLS happens. The error printed starts with `pipeclient: dial \\.\pipe\EdgeSyncWinpipeSpike:`
and the Windows reason is access denied. The server log shows no new client. Paste the exact line.

## Run 3 - wrong expected server account (optional, the user runs this)

From terminal B (same account as the server) run `spikes\bin\pipeclient.exe -server-sid S-1-5-18`. Expected:
`VerifyServer` refuses because the server runs as the user, not LocalSystem: `winpipe: pipe server runs as another account`.

## What to observe and paste back

1. The `pipeclient` output of Run 1 (all lines, including the median and overhead).
2. The `pipeserver` log lines of Run 1.
3. The exact error text of Run 2 and whether `pipeserver` logged anything for it.
4. The exact error text of Run 3.

## Design notes and known limits

- `Authorize` runs before every message, so a client that changes session or image mid-connection is refused on its next
  message (covered by the host test `session changes between messages`).
- The client PID is read from the pipe and used to open a process handle; SID, image and session are then read from
  that handle. A PID reused after the client exits is not detected by this spike beyond the image and SID checks.
- The signing oracle is the real exposure: an authorised client can get any SHA-256 digest signed. The spike does not
  bind digests to handshake transcripts; that is a design question for P1 (C2 pki) and the service item (P2 W2).
- go-winio v0.6.4 `ListenPipe` creates the first instance with `FILE_CREATE` (not `FILE_FLAG_FIRST_PIPE_INSTANCE`), so a
  second server on the same name fails; `TestPipe_FirstInstanceRejectsSecondServer` asserts that. It also always sets
  `FILE_PIPE_REJECT_REMOTE_CLIENTS`.
- `x/sys/windows.TokenSessionId` is an access-right constant, not the token information class, so the session is read
  with `ProcessIdToSessionId` on the already-open process instead.
- Latency: the in-process numbers use `net.Pipe` and TLS over TCP loopback on the host. The Windows monotonic clock is too
  coarse for single-signature timing, so the spike reports batch means. The real pipe numbers come from Run 1.
