# Spike P0-SP5-005: SPAKE2 (RFC 9382, P-256) feasibility and TLS exporter binding

## Purpose

Prove that SPAKE2 per RFC 9382 for the ciphersuite SPAKE2-P256-SHA256-HKDF-HMAC can be implemented in-house on
`filippo.io/nistec` v0.0.4 and reproduces the RFC's official test vectors byte for byte (D-14), that the 6-digit code
becomes the password scalar as D-110 specifies, and that the exchange can be bound to a TLS 1.3 connection through
`ConnectionState.ExportKeyingMaterial` (architecture 6.5, FEAT-126, FEAT-127). This is a feasibility spike: the code is
the reference for the production package `internal/spake2` (P1-C2-005), not a library for anyone else, and it has had
no audit.

Host-safe only: pure computation plus TLS 1.3 over loopback (`127.0.0.1`, ephemeral port, in-process self-signed
certificate). Nothing here is a binary and nothing is run by hand.

## Primary sources (fetched, not recalled)

- RFC 9382, "SPAKE2, a Password-Authenticated Key Exchange", W. Ladd, Akamai, IRTF, Informational, September 2023,
  ISSN 2070-1721: https://www.rfc-editor.org/rfc/rfc9382.txt (landing page https://www.rfc-editor.org/info/rfc9382).
  The text was fetched on 2026-10-30; the file as served has SHA-256
  `057d9b93d198b2ad23cbf64d841bef6fb99b6f9817822d9b994c6159105db625` (it starts with a UTF-8 byte order mark).
- Go 1.27 standard library `crypto/hkdf` (`hkdf.Key`), `crypto/hmac`, `crypto/sha256`, `crypto/tls`.
- `filippo.io/nistec` v0.0.4 (module source read in the module cache, `C:\Users\kikz0r\go\pkg\mod\filippo.io\nistec@v0.0.4`).

## RFC sections implemented

| RFC 9382 | What | Where |
|---|---|---|
| 3.1 | Flow: A sends pA, B sends pB, then cA, then cB; neither side completes before verifying the peer MAC | `Exchange.Finish`, `Exchange.Verify` (`Verify` is the only way to obtain Ke) |
| 3.2 | Setup: `len()` as 8-byte little-endian, `nil` as empty string, AAD limit 2^16 - 128 bits, `w` is application defined | `transcript`, `maxAADLen`, `passwordScalar` |
| 3.3 | `pA = w*M + x*P`, `pB = w*N + y*P`, `K = h*x*(pB - w*N)` / `h*y*(pA - w*M)`, TT with six length-prefixed fields, `w` as 32-byte big-endian | `newExchange`, `Exchange.derive`, `transcript` |
| 4 | `Ke \|\| Ka = Hash(TT)`, `KcA \|\| KcB = KDF(Ka, nil, "ConfirmationKeys" \|\| AAD, 256 bits)`, `cA = MAC(KcA, TT)`, `cB = MAC(KcB, TT)`, output Ke | `Exchange.derive`, `confirmationMAC` |
| 6 | Ciphersuite P-256 + SHA-256 + HKDF + HMAC, uncompressed SEC1 on the wire, M and N (compressed constants) | `mCompressed`, `nCompressed`, `parsePeerShare` |
| 7 | Group-membership check of received elements, rejection-sampled uniform ephemerals, no secret-dependent timing on point or scalar arithmetic | `parsePeerShare`, `randomScalar`, nistec, `reduceWide` |
| Appendix A | M and N regenerated from the seed strings in the test | `TestConstants_MatchRFCAndAppendixA` |
| Appendix B | The four P-256 / SHA-256 test vectors | `TestRFC9382_VectorsP256SHA256` |

Not implemented (out of scope for the plan): section 5 (per-user M and N, the symmetric M = N variant), the SHA-512,
CMAC-AES-128 and other-group ciphersuites, any memory-hard function for the password.

Cofactor: P-256 has prime order (cofactor h = 1), so the multiplication by h in section 3.3 is the identity map. It is
omitted in `derive` (the comment there says so) and every point that passes nistec's on-curve check is already in the
prime-order subgroup, so no separate subgroup check exists or is needed.

## Test vectors: RFC 9382 Appendix B, P256-SHA256-HKDF-HMAC

Appendix B contains four vectors that differ only in the identities. All four are reproduced, every listed value, byte
for byte, through the unexported constructor `newExchange(role, w, x, ...)` (fixed scalars; the exported `New` always
draws fresh random scalars). The vector literals in `spake2_test.go` were extracted mechanically from the RFC text by a
script, not typed by hand, and the script asserted every length (w, x, y 32 bytes; pA, pB, K 65 bytes; Hash(TT)
32 bytes; Ke, Ka, KcA, KcB 16 bytes; both MACs 32 bytes; TT length equals the sum of the field lengths).

| Vector | A | B | Reproduced values |
|---|---|---|---|
| 1 | `server` | `client` | w, x, pA, y, pB, K, TT, Hash(TT), Ke, Ka, KcA, KcB, A conf, B conf |
| 2 | empty | `client` | w, x, pA, y, pB, K, TT, Hash(TT), Ke, Ka, KcA, KcB, A conf, B conf |
| 3 | `server` | empty | w, x, pA, y, pB, K, TT, Hash(TT), Ke, Ka, KcA, KcB, A conf, B conf |
| 4 | empty | empty | w, x, pA, y, pB, K, TT, Hash(TT), Ke, Ka, KcA, KcB, A conf, B conf |

Per vector the test checks: the shares computed from the RFC's w, x, y equal pA and pB; both roles derive the same K,
TT, Hash(TT), Ke, Ka, KcA, KcB (each compared with the RFC value); `Finish` returns the RFC's A conf and B conf; the
cross `Verify` calls succeed and return the RFC's Ke. The RFC does not publish X, Y, Z or V values (those names belong
to the SPAKE2+ document, RFC 9383); the only published values are the ones in the table.

Caveat that matters for the exporter binding: the RFC vectors use no AAD (`KcA || KcB = KDF(Ka, nil, "ConfirmationKeys")`).
The AAD path is therefore covered by this spike's own tests (`TestExporter_BindingDiffersPerConnection`), not by an
official vector.

## Identities, password scalar and exporter binding (as the plan specifies)

- Roles: A is the initiator and uses M, B is the responder and uses N. Identities are idA = initiator's peer ID and
  idB = responder's peer ID (architecture 6.5, D-42: 52 lowercase base32 characters). Both appear in TT.
- Password scalar (D-110, the P1-C2-005 brief): `w = OS2IP(HKDF-SHA256(secret = code, salt = "EdgeSync SPAKE2 w v1",
  info = idA || 0x00 || idB, L = 48)) mod n`. The 48-byte output (16 bytes more than the group order needs) keeps the
  modular bias below 2^-128, as RFC 9382 section 3.2 recommends. `TestPasswordScalar_MatchesBigIntOracle` recomputes it
  independently with `math/big`.
- RFC 9382 section 3.2 says the hash SHOULD be a memory-hard function. D-110 and the P1-C2-005 brief omit it on purpose
  (one-time code, 3 attempts, 120 s TTL, no offline oracle in the transcript); the security-review gate checks that
  rationale. Nothing was left open for this spike.
- Exporter binding: `AAD = ConnectionState.ExportKeyingMaterial("EXPORTER-EdgeSync-pairing-v1", nil, 32)`, exactly as
  architecture 6.5 step 3 states. The AAD enters only the confirmation keys (RFC section 4), so a run bound to
  connection A produces different cA and cB than the same run bound to connection B.

## Security properties checked by tests

| Property | Test |
|---|---|
| Both sides with the same code derive the same Ke | `TestSPAKE2_SameCodeAgrees` |
| Wrong code: both confirmations fail, no key returned | `TestSPAKE2_WrongPasswordFailsConfirmation` |
| Different identities fail confirmation | `TestSPAKE2_DifferentIdentitiesFailConfirmation` |
| A confirmation cannot be reflected to its sender (KcA differs from KcB) | `TestSPAKE2_ConfirmationIsNotReflectable` |
| Identity point, off-curve point, wrong length/prefix, compressed or hybrid encoding, non-canonical coordinate, own share reflected: rejected with `ErrInvalidShare`; the exchange is then dead | `TestSPAKE2_RejectsIdentityAndOffCurvePoints` |
| Shared point K equal to the identity rejected | `TestSPAKE2_RejectsIdentitySharedPoint` |
| `Finish` twice, `Verify` twice, `Verify` before `Finish`, retry after a failed `Verify`: `ErrState`; wrong-length or bit-flipped MAC: `ErrConfirmation` | `TestExchange_StateMachine` |
| Ephemeral scalars rejection-sampled into [1, n-1]; weak or failing randomness fails loudly | `TestRandomScalar_RejectionSampling`, `TestNew_RejectsInvalidParameters` |
| Constant-time wide reduction equals `math/big` on edge cases and 2000 random inputs | `TestReduceWide_MatchesBigInt`, `TestScalarBounds` |
| M and N equal the RFC constants and are reproduced by the Appendix A algorithm | `TestConstants_MatchRFCAndAppendixA` |
| Exporter value equal on both ends of one connection, different across connections, different per label | `TestExporter_BindingDiffersPerConnection` |
| Run bound to connection A fails confirmation when relayed to connection B; with identical scalars only the binding decides | `TestExporter_BindingDiffersPerConnection` (subtests) |

Implementation rules worth a reviewer's attention:

- Group arithmetic is only nistec (`ScalarBaseMult`, `ScalarMult`, `Add`, `Negate`, `IsInfinity`): fiat-crypto or
  assembly backend, constant time, complete addition formulas.
- The password-derived scalar `w` never goes through `math/big`. The 384-bit to 256-bit reduction is a bit-serial
  shift-and-conditionally-subtract over four `uint64` limbs (`reduceWide`), which takes the same time for every value.
  `math/big` appears only in tests, as the oracle. Ephemeral scalars are sampled directly in [1, n-1]; the only
  data-dependent branch there rejects a candidate that is then discarded.
- Confirmation MACs are compared with `hmac.Equal` (constant time for equal lengths); a length mismatch returns early,
  which reveals only the public MAC length.
- `Verify` is the only way to obtain Ke and a failed `Verify` or `Finish` makes the exchange unusable.
- `Finish` zeroes the ephemeral scalar and `w` arrays. Go gives no guarantee about copies made by the runtime.
- Beyond the RFC, two defensive rejections: a peer share byte-equal to the own share, and a shared point K equal to the
  identity. The latter can only occur when the peer used the right `w`, so it reveals nothing extra, and it avoids the
  one-byte identity encoding corrupting the TT layout.

## Known non-goals

- Online guessing is not limited here: with a 6-digit code an attacker who can start sessions wins by trying codes. The
  attempt limit (3 wrong attempts per code, TTL 120 s, D-57) and per-code accounting belong to `internal/pairing`
  (P1-C2-008), outside this package.
- No independent audit. No audited Go PAKE exists (research-go-ecosystem 3.4); this is in-house per D-14. The
  unaudited alternatives only for the record: `github.com/schollz/pake/v3` (not the RFC 9382 key schedule or
  confirmation), `github.com/mendsec/cpace` (CPace, a different protocol), `github.com/backkem/spake2-go` (RFC 9382,
  "no formal security review").
- Not a library for other projects; no stable API; no section 5 variants; no memory-hard password hashing.
- No protocol framing, retry policy, certificate pinning or pairing state machine: those are P1-C2-007/008.

## Findings for the P1 port (P1-C2-005)

- Everything the brief says about the construction held: constants from section 6, TT with 8-byte little-endian
  prefixes, `w` as 32-byte big-endian, uncompressed 65-byte points, `Ke || Ka` as the two halves of SHA-256(TT),
  `KcA || KcB` as 32 bytes of HKDF with nil salt, MAC = HMAC-SHA256 over TT. No deviation from the RFC was needed to
  reproduce a vector.
- The brief says to use `math/big` once per exchange for `w mod n` and `n - w`. This spike avoids both: `w mod n` is the
  constant-time `reduceWide`, and `-w*N` is `Negate` on the point `w*N`, so no scalar negation is needed. The P1 port
  can drop `math/big` from production code (decision for the parent at the freeze).
- Per exchange M and N are parsed (two compressed-point decodes, each a field square root). The spike keeps no
  package-level state; P1 can hoist the two points into immutable package values if the cost matters (it does not at
  these numbers, see below).
- `crypto/hkdf` in Go 1.27 takes `info` as a `string`; byte identities are converted with `string(...)`.
- The `0x00` separator in the password info makes `idA || 0x00 || idB` ambiguous when an identity may contain `0x00`.
  Peer IDs are 52 base32 characters, so it cannot happen in EdgeSync; a reviewer should still note it.

## Build and run (tests only; no binary exists, none is run)

From the repository root, Git Bash, Go on `PATH`:

```
go -C spikes vet ./...
go -C spikes build ./...
go -C spikes test -count=1 ./spake2/...
PATH="/d/Development/msys2/ucrt64/bin:$PATH" CGO_ENABLED=1 CC=gcc go -C spikes test -race -count=1 ./spake2/...
go -C spikes test -tags purego -count=1 ./spake2/...
GOOS=linux CGO_ENABLED=0 go -C spikes vet ./spake2/...
GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go -C spikes vet ./spake2/...
go -C spikes test -run "^$" -bench BenchmarkSPAKE2Full -benchmem ./spake2
```

## Measured

`BenchmarkSPAKE2Full` (both sides end to end: two `New` including the HKDF password derivation, two `Finish`, two
`Verify`, shares handed over in memory), Windows 11, AMD Ryzen 7 7800X3D, Go 1.27.3 amd64 (nistec assembly backend),
`go test -bench -benchmem -count=3`:

```
BenchmarkSPAKE2Full-16    4141    286236 ns/op    12945 B/op    146 allocs/op
BenchmarkSPAKE2Full-16    4131    287931 ns/op    12944 B/op    146 allocs/op
BenchmarkSPAKE2Full-16    4239    284261 ns/op    12944 B/op    146 allocs/op
```

About 0.29 ms for the whole exchange, roughly 0.14 ms per side, far below PB-22 (50 ms per side).

## What to observe

- `TestRFC9382_VectorsP256SHA256` passes for all four vectors (four subtests).
- `TestExporter_BindingDiffersPerConnection` passes: the TLS 1.3 exporter value is equal on both ends of a connection,
  different across two connections, and a SPAKE2 run bound to connection A fails confirmation against connection B.

## What to paste back

Nothing from the user: there is no manual step for this spike. The findings feed `briefs/spikes/sp5.md`
(P0-SP5-008): vectors reproduced, the benchmark above, and the P1 port notes.

## Licence of the dependency

`filippo.io/nistec` v0.0.4 (tag of 2025-09-29): BSD-3-Clause, "Copyright 2009 The Go Authors" (it is the exported
form of the Go standard library's internal nistec package).
