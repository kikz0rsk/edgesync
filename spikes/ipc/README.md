# Spike P0-SP5-006: agent IPC transport (named pipe and unix socket peer verification)

## Purpose

Prove the two per-user transports of D-46 (GUI <-> agent IPC): a Windows named pipe created with go-winio and an
owner-only DACL plus first-instance semantics, with the client verifying the server process's user SID, and a unix
domain socket in a 0700 directory with a 0600 socket file and peer credentials checked against the agent's uid. Also
measure the round trip of a 1 KiB message over the local pipe (FEAT-177, FEAT-132). This is a feasibility spike: the
code is the reference for `internal/ipc` (P1), not a library. It is independent of the SYSTEM service pipe (D-47,
spike SP1-004), which is not part of this spike.

Host-safe only: the pipe tests create a pipe with a random name (`\\.\pipe\edgesync-spike-ipc-<32 hex>`) and an
owner-only DACL, closed at test end; they never touch the real agent pipe `edgesync-agent-<UserSID>`. The socket test
uses `os.MkdirTemp` and removes the directory at test end. No test needs another process or admin rights, and nothing
here is a binary that is run by hand.

## Primary sources (read from the module cache, not recalled)

- github.com/Microsoft/go-winio v0.6.4 (tag b900fd8a, file `pipe.go`):
  `ListenPipe(path string, c *PipeConfig) (net.Listener, error)` builds the first instance with `NtCreateNamedPipeFile`
  and `FILE_CREATE` (a second listener on the same name fails, which is the first-instance behaviour);
  `PipeConfig.SecurityDescriptor string` (SDDL, converted by `SddlToSecurityDescriptor`), `MessageMode bool` (false
  here, byte mode); `FILE_PIPE_REJECT_REMOTE_CLIENTS` is always set by `makeServerPipeHandle`.
  `DialPipe(path string, timeout *time.Duration) (net.Conn, error)`. The returned connection is `*win32Pipe`, which
  promotes `Fd() uintptr` from `win32File` (`file.go`); `Fd` is used to reach the handle for the x/sys calls below.
- golang.org/x/sys v0.49.0 `windows`: `GetNamedPipeServerProcessId(pipe Handle, *uint32)`, `OpenCurrentProcessToken()`,
  `Token.GetTokenUser() (*Tokenuser, error)` (`User.Sid.String()`), `OpenProcess(desiredAccess, inherit, pid)`,
  `OpenProcessToken`, `GetSecurityInfo(handle, SE_KERNEL_OBJECT, DACL_SECURITY_INFORMATION)`,
  `SECURITY_DESCRIPTOR.String()` (SDDL form), constants `PROCESS_QUERY_LIMITED_INFORMATION`, `TOKEN_QUERY`.
- golang.org/x/sys v0.49.0 `unix`: linux `GetsockoptUcred(fd, level, opt int) (*Ucred, error)` with `SOL_SOCKET` and
  `SO_PEERCRED` (`Ucred.Uid`); darwin `GetsockoptXucred(fd, level, opt int) (*Xucred, error)` with `SOL_LOCAL` (0) and
  `LOCAL_PEERCRED` (0x1) (`Xucred.Uid`). `LOCAL_PEERPID` is not used. darwin has no `Getpeereid` in x/sys.
- Go 1.27 standard library: `net.ListenUnix`, `os.MkdirTemp` (directory mode 0700), `testing.B.Loop`.

## Design under test

| Item | Where | Notes |
|---|---|---|
| DACL string `D:P(A;;GA;;;SY)(A;;GA;;;<UserSID>)` | `pipe_windows.go` `ownerOnlySDDL` | SYSTEM and the user only, protected DACL |
| Listener, first instance | `pipe_windows.go` `listenOwnerOnly` | `winio.ListenPipe` with the SDDL |
| Server SID check (client side) | `pipe_windows.go` `verifyServerUser` | `GetNamedPipeServerProcessId` -> `OpenProcess` -> `OpenProcessToken` -> `GetTokenUser`; mismatch returns `ErrServerIdentity` |
| Socket in 0700 directory, 0600 socket | `sock_unix.go` `listenSocket` | the 0700 directory is what protects the window between `Listen` (umask mode) and `Chmod` |
| Peer uid | `sock_unix.go` `peerUID`, `sock_linux.go`, `sock_darwin.go` | per-OS `peerCredentialUID` (see below) |

Message framing: the benchmark and round trip use fixed 1024-byte messages, so byte mode needs no length prefix.
The P1 framing (Cap'n Proto stream framing, 16 MiB cap, D-46) is not part of this spike.

Not covered by this spike (left to P1): the 32-byte session token of D-46, the server-side check of the client SID
(`GetNamedPipeClientProcessId`), single-instance behaviour beyond the first-instance create, and any Cap'n Proto
framing.

Per-OS peer credentials: `sock_unix.go` is shared by linux and darwin and calls `peerCredentialUID`, which is
implemented in `sock_linux.go` (`GetsockoptUcred`) and `sock_darwin.go` (`GetsockoptXucred`, `SOL_LOCAL`,
`LOCAL_PEERCRED`) because the two x/sys calls exist on one OS each.

## Build and run

Build check (host, pure compile):

    go -C spikes vet ./ipc
    go -C spikes build -o NUL ./ipc
    GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go -C spikes vet ./ipc
    GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go -C spikes vet ./ipc

Host-safe tests (Windows host, creates one local named pipe per test):

    go -C spikes test -count=1 -v ./ipc

Race detector (needs the ucrt64 gcc):

    PATH="/d/Development/msys2/ucrt64/bin:$PATH" CGO_ENABLED=1 CC=gcc go -C spikes test -race -count=1 ./ipc

Round-trip benchmark (host):

    go -C spikes test -count=1 -run=NONE -bench=BenchmarkPipeRoundTrip1KiB ./ipc

Socket test, run by the spikes workflow on ubuntu-24.04 and macos-latest (not run on the Windows host):

    go -C spikes test -count=1 -v -run TestSock_PeerCredMatchesUID ./ipc

## Host measurements (Windows 11, Ryzen 7 7800X3D, 2026-10-30)

- `TestPipe_OwnerOnlySDDL`: pass. The kernel stores `GA` of the requested SDDL as `FA` (FILE_ALL_ACCESS) on the
  pipe; the test asserts the stored DACL `D:P(A;;FA;;;SY)(A;;FA;;;<UserSID>)`, a second listener on the same name
  is refused, and a client rejects a server running as another SID.
- `TestPipe_RoundTripLatency1KiB`: pass, 5000 round trips of 1024 bytes, mean 18.0 us per round trip (one run).
- `BenchmarkPipeRoundTrip1KiB`: 21.96 us/op, 640 B/op, 8 allocs/op (20000 iterations, one run).
- Linux and darwin: cross `go vet` and `go build` pass for `./ipc`; `TestSock_PeerCredMatchesUID` is compile-checked
  only on the host and runs in the spikes workflow.

## What to observe

- The DACL test passes on the user's Windows machines (the SID differs per machine; the test derives it).
- Round-trip mean stays in the tens of microseconds on an idle machine; a result in the milliseconds points at
  antivirus or a loaded host and is recorded, not hidden.
- On Linux and macOS the peer uid equals `id -u` of the test user.

## What to paste back

- The `go -C spikes test -count=1 -v ./ipc` output (Windows) and the CI log lines for `TestSock_PeerCredMatchesUID`.
- The benchmark line.
