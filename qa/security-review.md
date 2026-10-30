# Security review checklist

Used by the security-review gate (plan.md 1.5) for SP1 (service parts), C1, C2, C4, C5, W2, P4-F, F1-F5, R1, R4,
and by P5 R4's full audit. The reviewer (`code-reviewer`, `model: "sonnet"`) reports `file:line - defect` with
severity (critical/high/medium/low). Every finding becomes a fix item appended to the owning stream; R4 records
the consolidated results in `qa/security-review-results.md`.

## 1. Pairing and SPAKE2 (D-14, FEAT-126, FEAT-127)
- [ ] SPAKE2 reproduces every RFC 9382 P-256 test vector; implementation cites RFC sections.
- [ ] M and N constants match the RFC exactly; identity point and off-curve points rejected; scalar ops are constant time (nistec).
- [ ] Transcript includes both identities (peer IDs) and the TLS exporter value (channel binding) so a MITM relaying the code fails.
- [ ] Key confirmation MACs are verified with constant-time comparison before anything is stored.
- [ ] Code from crypto/rand, 6 digits, TTL 120 s, invalidated after 3 failures; attempts are counted per code, not per connection.
- [ ] Pairing never pins a certificate before confirmation succeeds; failures leak no timing difference between "wrong code" and "other error" visible to the peer.

## 2. TLS and identity (D-41, D-42)
- [ ] TLS 1.3 minimum; `VerifyConnection` checks the SPKI pin for every connection including resumptions; `InsecureSkipVerify` appears only together with `VerifyConnection`.
- [ ] Server requires a client certificate (`RequireAnyClientCert`) and checks the pin.
- [ ] Unknown, revoked (unpaired) or mismatched pins are rejected with a logged reason; no "trust on first use" path outside pairing.
- [ ] Private keys: 0600 files in 0700 dirs (Unix); DPAPI + owner/SYSTEM-only DACL (Windows); never logged, never in diagnostics bundles.

## 3. Local IPC (D-46)
- [ ] Named pipe DACL exactly owner + SYSTEM; first-instance flag prevents squatting; both ends verify the other's user SID.
- [ ] Unix socket dir 0700, socket 0600, peer UID checked.
- [ ] Session token compared in constant time; token file owner-only; token rotated per agent start.
- [ ] Message size cap and capnp traversal/depth limits enforced before allocation.

## 4. Windows service and secure helper (D-47, D-63, D-64)
- [ ] Service pipe SDDL as specified; each request authorised by client session ID, image path under `%ProgramFiles%\EdgeSync\`, and command whitelist.
- [ ] No request path executes programs, reads/writes arbitrary files, changes configuration, or accepts paths from the client.
- [ ] Helper pipe is SYSTEM-only; the helper accepts injection/capture requests only from the verified user agent of its own session.
- [ ] Token handling: duplicated tokens closed; `TokenUIAccess` set only on the helper token; process created with the minimal needed desktop.
- [ ] Pre-login mode: injection only; machine-scoped peer store read-only for the helper; no clipboard/files.
- [ ] Pre-login trust list (D-112): `machine-peers.toml` is written only by the elevated `edgesync-service allow-prelogin` command (elevation proof, peer ID format validated, audit record with the invoking user) and read by the service; the only service-pipe addition is the read-only `GetPreloginPeers`; DACL SYSTEM and Administrators only; a peer that is unpaired is removed from it first.
- [ ] Ctrl+Alt+Del path (D-113): the chord is only sent to a Windows client reporting the secure-desktop capability; the client turns it into `RequestSAS` and never injects the Delete key into the secure desktop.
- [ ] Installer (D-118): the service is installed only from `%ProgramFiles%\EdgeSync` (the `/DIR` override cannot move it), the firewall subcommand derives the agent path from its own executable, no `[Run]` entry executes a user-writable file as SYSTEM.
- [ ] Service files and `%ProgramData%\EdgeSync` ACLs prevent non-admin modification (binary planting).

## 5. Wire decoding (D-36, D-76)
- [ ] Hot-path decoder rejects unknown types (stream) / drops them (datagrams) without panics; fuzzed.
- [ ] Capnp decoding applies traversal and depth limits and per-channel size caps; fuzzed for every message family.
- [ ] Manifests: entry counts, path lengths and total sizes bounded before allocation.

## 6. File handling (D-44, D-45, D-62)
- [ ] All writes go through `os.Root`; sanitiser rejects absolute paths, drive letters, UNC, `..`, empty components, NUL/control chars, reserved device names, trailing dots/spaces, components > 255 bytes; fuzzed.
- [ ] Symlinks never transferred; existing symlinks in the destination never followed.
- [ ] Partial files are never visible under the final name until verified; hash mismatch deletes the partial and reports.
- [ ] Remote "cut" never deletes source files (D-62).
- [ ] Staging directory permissions owner-only; old staging entries cleaned (D-57).

## 7. Clipboard and privacy (D-48, D-61)
- [ ] Concealed/sensitive clipboard content (password managers) is shared like any other content (D-121); no backend detects or special-cases sensitivity; the master toggle and per-format toggles stop it.
- [ ] Only the CLIPBOARD selection is synchronised (no PRIMARY).
- [ ] Clipboard contents never logged; file names only at Debug; diagnostics bundle redaction verified by a test.
- [ ] Clipboard data is only sent in response to a fetch from a paired peer in the current session.

## 8. Network surface
- [ ] Listener accepts only TLS; unauthenticated peers get no information beyond the TLS handshake (no version banner before auth).
- [ ] mDNS TXT records contain only `v`, `id`, `role`, `tcp` (D-43).
- [ ] Wake-on-LAN sends only magic packets to stored addresses; no inbound handling.
- [ ] Update check: HTTPS to api.github.com only; response size capped; no code execution; no auto-install.

## 9. Supply chain and provenance (D-27)
- [ ] Every dependency is pinned and listed in `compatibility-matrix.md`; `govulncheck` clean.
- [ ] No code copied from GPL projects; permissive sources carry their notices in `third_party/`.
