# ei - pure-Go EI (libei) wire codec spike

Item P0-SP3-001 (FEAT-177, FEAT-073). Decision under test: D-39 (Wayland input via an in-house EI codec over the
portal-provided socket, `CGO_ENABLED=0`).

## Purpose

A codec and a small client for the EI protocol (the libei wire protocol between an EI client and an EIS
implementation such as GNOME/mutter or KDE/KWin), with no cgo and no libei. It covers the header, all argument
types (uint32, int32, uint64, int64, float, string, new_id, object, fd out-of-band), the handshake, and the interfaces
`ei_handshake`, `ei_connection`, `ei_callback`, `ei_pingpong`, `ei_seat`, `ei_device`, `ei_pointer`,
`ei_pointer_absolute`, `ei_button`, `ei_scroll` and `ei_keyboard`, for both context types (receiver = capture,
sender = injection). `ei_touchscreen` and `ei_text` are deliberately not announced to the EIS, so it never creates
those objects for this client.

This item proves the codec on the host against golden frames and a scripted fake EIS over `net.Pipe`. Proving it
against a real compositor is P0-SP3-002 (`wlcapture`, `wlinject`), which feeds the fd returned by the portal's
`ConnectToEIS` into `UnixTransportFromFile`.

## Protocol source (cited)

- Repository: https://gitlab.freedesktop.org/libinput/libei
- Version read: tag `1.6.0`, commit `8a46bf3d4b6af7f25a61be53386cf618218114ef` (2026-05-15)
- Protocol definition: `proto/protocol.xml` and `proto/protocol.dtd` at that tag
- Prose specification: `doc/protocol/doc/overview.md`, `types.md`, `initial-handshake.md`, `specification.md` at that tag
- Opcode numbering: `proto/ei-scanner` assigns opcodes in declaration order, separately for requests and events
  (the first request and the first event both have opcode 0)
- Licence: MIT (`COPYING`, Copyright 2020 Red Hat, Inc.; the XML carries the same MIT notice)

The codec was written from those documents. Two libei C files were read only to confirm behaviour the prose leaves
open, and no code was taken from them: `src/brei-shared.c` (messages above 1 MiB are rejected, so `MaxFrameSize` is
1 MiB; a string length of 0 is the NULL string; strings must be zero-terminated; padding bytes are not inspected;
64-bit arguments are not 8-byte aligned) and `src/libeis-device.c` (an EIS waits for `ei_device.ready` from clients
that announced `ei_device` version 3 or higher). No GPL project (lan-mouse, Deskflow, Input Leap) was consulted (D-27).

## Wire facts implemented

- Header: object id u64, length u32 (including the 16 header bytes), opcode u32. Integers use the EIS native byte
  order; the codec fixes little-endian, which holds on every supported target (amd64, arm64).
- Frames must be a multiple of 4 bytes and at most `MaxFrameSize`; shorter, unaligned or oversized lengths are
  rejected before any allocation of the declared size.
- Object 0 is `ei_handshake`. Client object ids start at 1 (below `0xff00000000000000`), EIS ids start at
  `0xff00000000000000`; the client checks that every new_id received is in the EIS range and not reused.
- Handshake: the EIS sends `handshake_version` first; the client answers with `handshake_version`, `context_type`,
  `name`, one `interface_version` per supported interface, then `finish`; the EIS answers with any number of
  `interface_version` events and finally `connection` (new_id, version, serial), which destroys object 0.
- Versions announced: `ei_connection`, `ei_callback`, `ei_pingpong`, `ei_pointer`, `ei_pointer_absolute`,
  `ei_scroll`, `ei_button`, `ei_keyboard` 1; `ei_seat` 2; `ei_device` 3. Object versions in later events are checked
  against `min(client, EIS-announced)`.
- `ei_device` version 3 requires the client to send `ei_device.ready` after `ei_device.done`. The client does this
  automatically for any role when the device object was created with version 3 or higher. The XML marks `ready` as
  `context-type="sender"`, libeis does not enforce that, and the prose says any client supporting version 3 must send
  it; whether mutter and KWin behave the same is part of what P0-SP3-002 observes.
- `ei_keyboard.keymap` carries its fd out-of-band. `UnixTransport` queues fds from `SCM_RIGHTS` ancillary data in
  arrival order and the decoder pops one per fd argument; a truncated control message is an error.
- Receiver-only events arriving at a sender client, unknown opcodes, unknown object ids, trailing bytes after the last
  argument and invalid strings are protocol violations that end the client with an error (no silent skipping).
- `ei_connection.ping` is answered automatically; `ei_connection.sync` is exposed as `Client.Sync`.

## API outline

- `wire.go`: `Header`, `Frame`, `DecodeFrame`, `ReadFrame`, `Encoder`, `Decoder`, `Transport`, `NewStreamTransport`
- `objects.go`: `Interface`, opcode tables, typed events, `DecodeEvent`, request builders
- `handshake.go`: `Config`, `HandshakeResult`, the handshake state machine
- `client.go`: `Connect`, `Client` (`Events`, `Err`, `Close`, `Sync`, `Bind`, `StartEmulating`, `StopEmulating`,
  `SendFrame`, `MotionRelative`, `MotionAbsolute`, `Scroll`, `ScrollDiscrete`, `ScrollStop`, `Button`, `Key`, `Release`)
- `fd_linux.go`: `UnixTransport`, `NewUnixTransport`, `UnixTransportFromFile` (SCM_RIGHTS via `golang.org/x/sys/unix`)

`Connect` owns one dispatch goroutine. The consumer must drain `Events()`; the goroutine blocks on a full channel
and `Sync` completes only while the loop is running. Cancelling the context passed to `Connect` closes the
transport. After `Events()` is closed, `Err()` returns why: nil after `Close`, `*DisconnectError` when the EIS sent
`ei_connection.disconnected`, `ErrPeerClosed` when the socket closed without it, or the decode or protocol error.

## Build

```
go -C spikes build ./linux/ei/...
GOOS=linux CGO_ENABLED=0 go -C spikes vet ./linux/...
```

## Run

This item has no binary. The host-safe tests are the run step:

```
go -C spikes test ./linux/ei/...
go -C spikes test -run '^$' -fuzz '^FuzzDecodeFrame$' -fuzztime 30s ./linux/ei
```

`fd_linux.go` is only compiled on Linux and is covered by `vet` and `build` on the host. It is exercised for real
by P0-SP3-002 on the GNOME box.

## What to observe

All tests pass: `TestWire_HeaderEncodeDecode`, `TestWire_GoldenHandshakeFrames`, `TestWire_RejectsShortAndOversized`,
`TestClient_HandshakeAgainstScriptedServer` (full handshake, seat bind, device setup with automatic `ready`,
emulation requests, sync, ping, disconnect over `net.Pipe`), plus the supporting wire and client tests, and the fuzz
run finishes without a crash.

## What to paste back

Nothing for this item beyond the test and fuzz output. The real-compositor observations (handshake accepted, seat and
device events, keymap fd received, `ready` required or not, devices resumed) are pasted back from the P0-SP3-002 run.
