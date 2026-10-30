# EdgeSync - Phase 1 items

Read `plan.md` (contract, orchestration, item format) first. Stream briefs: `briefs/streams/p1-*.md`.
Phase 1 builds the frozen contracts (P1-F, sequential, one agent) and then the platform-independent core in five
parallel streams (C1..C5). Every package written here is platform-independent (or has thin per-OS files that are
tested on the host and on CI) and must reach >= 85% statement coverage (D-37). Nothing in this phase launches the
agent; tests that touch real input, clipboard, displays, OS services or multicast carry `//go:build integration`.

Common DoD shorthand used below:
- "item gates" = the per-item gates of `plan.md` 1.5 (golangci-lint fmt + run, go vet, go build, go test of the
  touched packages, both cross-compile builds, `-race` when the item has goroutines/channels/locks, fuzz smoke for
  new fuzz targets, actionlint when workflows change).
- "bench report" = the subagent runs `go test -run '^$' -bench <name> -benchmem -count 5 <pkg>` and puts the
  numbers in its TESTS line; the parent appends the row to the `perf-budgets.md` results log.
- Every commit uses `<type>(<scope>): <description> [FEAT-NNN]` with body `Item: <id>` and no attribution trailer.

## Phase 1 - contracts & core

### P1-F - Foundation block (sequential, 1 agent, in order; blocked on P0 exit)

- [ ] [P1-F-001] Build info and protocol version package [FEAT-145] [tier:haiku] [owner:P1-F] [brief:briefs/streams/p1-f.md#item-p1-f-001]
  Needs: P0-X-003
  Files: internal/buildinfo/buildinfo.go, internal/buildinfo/buildinfo_test.go
  Tests: TestGet_Defaults, TestGet_ReflectsAssignedVersionAndCommit, TestInfo_String, TestProtocolVersion_IsOne
  DoD: API exactly as the brief; coverage of internal/buildinfo >= 85%; item gates green; commit `feat(buildinfo): build and protocol version info [FEAT-145]`

- [ ] [P1-F-002] keymap.Usage, ModMask and Remap types (frozen) [FEAT-020, FEAT-025] [tier:haiku] [owner:P1-F] [brief:briefs/streams/p1-f.md#item-p1-f-002]
  Needs: P1-F-001
  Files: internal/keymap/doc.go, internal/keymap/usage.go, internal/keymap/usage_test.go
  Tests: TestMake_PageAndID, TestConstants_MatchHIDUsageTables, TestIsModifier, TestModBit_ModifiersAndLocks, TestModBit_OtherKeysFalse, TestModMask_Has
  DoD: every constant value verified against the USB HID Usage Tables keyboard page (source cited in the test file header comment); coverage >= 85%; item gates green; commit `feat(keymap): HID usage, modifier mask and remap types [FEAT-020]`

- [ ] [P1-F-003] Peer identity type peerid.ID from SPKI (D-42) [FEAT-125, FEAT-128] [tier:haiku] [owner:P1-F] [brief:briefs/streams/p1-f.md#item-p1-f-003]
  Needs: P1-F-002
  Files: internal/peerid/peerid.go, internal/peerid/peerid_test.go
  Tests: TestFromSPKI_KnownVector, TestFromCertificate_UsesRawSPKI, TestParse_ValidRoundTrip, TestParse_RejectsLengthAlphabetCase, TestShort_Format, TestHash_RoundTrip, TestFromHash_RoundTrip
  DoD: 52-char lowercase unpadded base32 of SHA-256(SPKI); coverage >= 85%; item gates green; commit `feat(peerid): peer identity from SPKI hash [FEAT-125]`

- [ ] [P1-F-004] Platform core types and sentinel errors (frozen) [FEAT-003, FEAT-009, FEAT-020] [tier:haiku] [owner:P1-F] [brief:briefs/streams/p1-f.md#item-p1-f-004]
  Needs: P1-F-003
  Files: internal/platform/doc.go, internal/platform/types.go, internal/platform/errors.go, internal/platform/types_test.go
  Tests: TestRect_RightBottom, TestRect_ContainsIsHalfOpen, TestRect_Empty, TestEnums_StringAndZeroInvalid, TestInputEvent_HasNoReferenceFields (reflection: no pointer, interface, slice, map, string, func or chan fields, D-76), TestErrors_Sentinels, TestEdgeSegment_ContactRule (D-88)
  DoD: types identical to `briefs/architecture.md` section 3 plus the helpers in the brief; enum values start at 1 (0 invalid); coverage >= 85%; item gates green; commit `feat(platform): core types and sentinel errors [FEAT-009]`

- [ ] [P1-F-005] Platform interfaces (frozen at CONTRACT FREEZE) [FEAT-009, FEAT-010, FEAT-004, FEAT-080, FEAT-081, FEAT-138, FEAT-140, FEAT-144] [tier:haiku] [owner:P1-F] [brief:briefs/streams/p1-f.md#item-p1-f-005]
  Needs: P1-F-004, P0-X-002 (approved interface changes listed in `briefs/spikes/p0-consolidation.md`)
  Files: internal/platform/backend.go, internal/platform/capture.go, internal/platform/inject.go, internal/platform/displays.go, internal/platform/clipboard.go, internal/platform/session.go, internal/platform/ui.go, internal/platform/autostart.go, internal/platform/permissions.go, internal/platform/theme.go, internal/platform/enums_test.go
  Tests: TestFormat_CanonicalValues, TestSessionEventKind_String, TestTrayState_String, TestPermissionKind_String, TestTheme_String, TestCaptureStats_ZeroValue, TestContactHolder_IsOptional (D-90: a capturer without the method is treated as not holding)
  DoD: signatures character-identical to architecture sections 4.1-4.7 plus only the approved P0 changes the parent pastes into the dispatch; no `Open` in this package; coverage >= 85%; item gates green; commit `feat(platform): backend interfaces [FEAT-009]`

- [ ] [P1-F-006] In-memory fakes: Displays, Capturer, Injector [FEAT-009, FEAT-010] [tier:haiku] [owner:P1-F] [brief:briefs/streams/p1-f.md#item-p1-f-006]
  Needs: P1-F-005
  Files: internal/platform/fake/doc.go, internal/platform/fake/displays.go, internal/platform/fake/capturer.go, internal/platform/fake/injector.go, internal/platform/fake/input_test.go, go.mod, go.sum (claim; adds github.com/google/go-cmp v0.7.0)
  Tests: TestDisplays_SetNotifiesWatchers, TestDisplays_WatchStopsOnCancel, TestCapturer_EmitDeliversToSink, TestCapturer_FullSinkCountsDrops, TestCapturer_GrabUngrabState, TestCapturer_SetEdgesRecorded, TestCapturer_SwallowKeysRecorded, TestCapturer_UseAfterCloseFails, TestInjector_LogsCalls, TestInjector_ReleaseAllReleasesPressedSorted, TestInjector_ConcurrentUse (race)
  DoD: compile-time assertions `var _ platform.X = (*fake.X)(nil)`; deterministic under `testing/synctest`; coverage of internal/platform/fake >= 85%; item gates green incl. -race; commit `feat(platform): input-side fakes [FEAT-009]`

- [ ] [P1-F-007] In-memory fakes: Clipboard, Session, Tray, Overlay, Autostart, Permissions, Theme, Backend [FEAT-080, FEAT-081, FEAT-138, FEAT-140, FEAT-144] [tier:haiku] [owner:P1-F] [brief:briefs/streams/p1-f.md#item-p1-f-007]
  Needs: P1-F-006
  Files: internal/platform/fake/clipboard.go, internal/platform/fake/session.go, internal/platform/fake/ui.go, internal/platform/fake/autostart.go, internal/platform/fake/permissions.go, internal/platform/fake/theme.go, internal/platform/fake/backend.go, internal/platform/fake/rest_test.go
  Tests: TestClipboard_LocalCopyNotifiesWatch, TestClipboard_OfferIsNotReported, TestClipboard_PasteRendersThroughProvider, TestClipboard_ReadAfterOfferUsesProvider, TestSession_EmitAndRecordedCalls, TestTray_RunUpdateClick, TestOverlay_RecordsShows, TestAutostart_RoundTrip, TestPermissions_StatusAndRequest, TestTheme_SetNotifies, TestBackend_ImplementsAndCloses
  DoD: compile-time interface assertions; coverage of internal/platform/fake >= 85%; item gates green incl. -race; commit `feat(platform): remaining fakes and fake backend [FEAT-080]`

- [ ] [P1-F-008] Conformance suites for every platform interface [FEAT-009, FEAT-010, FEAT-080, FEAT-168] [tier:sonnet why:contract-test design that every OS backend must pass, incl. driver interfaces for real input] [owner:P1-F] [brief:briefs/streams/p1-f.md#item-p1-f-008]
  Needs: P1-F-007
  Files: internal/platform/conformance/doc.go, internal/platform/conformance/drivers.go, internal/platform/conformance/capturer.go, internal/platform/conformance/injector.go, internal/platform/conformance/displays.go, internal/platform/conformance/clipboard.go, internal/platform/conformance/session.go, internal/platform/conformance/ui.go, internal/platform/conformance/conformance_test.go
  Tests: TestFakes_PassAllSuites (runs RunCapturer, RunInjector, RunDisplays, RunClipboard, RunSession, RunTray, RunOverlay, RunAutostart, RunTheme against the fakes), TestSuites_FailOnBrokenImplementation (a deliberately wrong wrapper per suite makes the suite report failures via a recording testing.TB)
  DoD: every behavioural promise in architecture section 4 has at least one named subtest; coverage of internal/platform/conformance >= 85%; item gates green; commit `test(platform): conformance suites [FEAT-168]`

- [ ] [P1-F-009] host: Linux session detection (Open itself is per OS, P2) [FEAT-063, FEAT-074] [tier:haiku] [owner:P1-F] [brief:briefs/streams/p1-f.md#item-p1-f-009]
  Needs: P1-F-005
  Files: internal/platform/host/host.go, internal/platform/host/host_test.go
  Tests: TestDetectLinuxSession table (XDG_SESSION_TYPE wayland/x11, unset with WAYLAND_DISPLAY, unset with DISPLAY only, tty, empty everything, conflicting values)
  DoD: no `Open` function in this item (P2 streams add `host_<goos>.go`); coverage >= 85%; item gates green; commit `feat(platform): linux session detection [FEAT-074]`

- [ ] [P1-F-010] Hot-path wire codec (frozen frame table) [FEAT-119] [tier:haiku] [owner:P1-F] [brief:briefs/streams/p1-f.md#item-p1-f-010]
  Needs: P1-F-002
  Files: internal/proto/wire/doc.go, internal/proto/wire/frame.go, internal/proto/wire/encode.go, internal/proto/wire/decode.go, internal/proto/wire/decoder.go, internal/proto/wire/wire_test.go
  Tests: TestGolden_EveryType (exact bytes), TestRoundTrip_EveryType, TestDecodeFrame_Short, TestDecodeFrame_UnknownType, TestDecodeFrame_InvalidBoolAndButton, TestDecodeDatagram_SizeMismatch, TestDecoder_StreamOfMixedFrames, TestDecoder_EOFAndUnexpectedEOF, TestAppendFrame_PanicsOnUnknownType, TestAllocs_ZeroOnEncodeDecodeAndDecoder (testing.AllocsPerRun)
  DoD: layouts byte-exact per architecture 5.4 and the brief; 0 allocs/op asserted; coverage >= 85%; item gates green; commit `feat(proto): hot-path binary frame codec [FEAT-119]`

- [ ] [P1-F-011] tools/capnpgen generator wrapper (D-67) [FEAT-120] [tier:haiku] [owner:P1-F] [brief:briefs/streams/p1-f.md#item-p1-f-011]
  Needs: P1-F-010, P0-SP5-008 (exact generation command in `briefs/spikes/sp5.md`)
  Files: tools/capnpgen/main.go, tools/capnpgen/capnpgen.go, tools/capnpgen/capnpgen_test.go
  Tests: TestFindCapnp_EnvWins, TestFindCapnp_PathLookup, TestFindCapnp_MissingIsError, TestSchemaFiles_SortedCapnpOnly, TestCompileArgs_MatchRecordedCommand, TestStdIncludeDir_FromGoListOutput
  DoD: CLI behaviour per brief; coverage of tools/capnpgen >= 85%; item gates green; commit `build(proto): capnp generator wrapper [FEAT-120]`

- [ ] [P1-F-012] Cap'n Proto schemas common, handshake, session + generated code [FEAT-120, FEAT-121, FEAT-007, FEAT-008] [tier:haiku] [owner:P1-F] [brief:briefs/streams/p1-f.md#item-p1-f-012]
  Needs: P1-F-011, D-97 (generated `internal/proto/capnp/...` excluded from the coverage gate)
  Files: proto/schema/common.capnp, proto/schema/handshake.capnp, proto/schema/session.capnp, internal/proto/capnp/doc.go, internal/proto/capnp/commoncp/common.capnp.go, internal/proto/capnp/handshakecp/handshake.capnp.go, internal/proto/capnp/sessioncp/session.capnp.go, tools/coverage-exclude.txt (claim), go.mod, go.sum (claim; adds capnproto.org/go/capnp/v3 v3.1.0-alpha.2)
  Tests: `go generate ./internal/proto/capnp/...` reproduces the committed files byte for byte; `go build ./...` and both cross builds pass
  DoD: field lists exactly per brief; file IDs from `capnp id`; generated files never hand-edited; `task gen:check` green; item gates green; commit `feat(proto): common, handshake and session schemas [FEAT-120]`

- [ ] [P1-F-013] Schemas clipboard, transfer, dnd, control envelope + generated code [FEAT-120, FEAT-081, FEAT-091, FEAT-116] [tier:haiku] [owner:P1-F] [brief:briefs/streams/p1-f.md#item-p1-f-013]
  Needs: P1-F-012
  Files: proto/schema/clipboard.capnp, proto/schema/transfer.capnp, proto/schema/dnd.capnp, proto/schema/control.capnp, internal/proto/capnp/clipboardcp/clipboard.capnp.go, internal/proto/capnp/transfercp/transfer.capnp.go, internal/proto/capnp/dndcp/dnd.capnp.go, internal/proto/capnp/controlcp/control.capnp.go
  Tests: `task gen:check` green; builds and cross builds pass
  DoD: field lists exactly per brief (incl. `FileOfferMore` paging and `DataHeader`); item gates green; commit `feat(proto): clipboard, transfer, dnd and control schemas [FEAT-120]`

- [ ] [P1-F-014] Schemas pairing and service + generated code [FEAT-120, FEAT-127, FEAT-050] [tier:haiku] [owner:P1-F] [brief:briefs/streams/p1-f.md#item-p1-f-014]
  Needs: P1-F-013
  Files: proto/schema/pairing.capnp, proto/schema/service.capnp, internal/proto/capnp/pairingcp/pairing.capnp.go, internal/proto/capnp/servicecp/service.capnp.go
  Tests: `task gen:check` green; builds and cross builds pass
  DoD: service request union is exactly the D-47 whitelist; item gates green; commit `feat(proto): pairing and service schemas [FEAT-120]`

- [ ] [P1-F-015] Schema ipc + generated code (IPC API surface) [FEAT-120, FEAT-133] [tier:haiku] [owner:P1-F] [brief:briefs/streams/p1-f.md#item-p1-f-015]
  Needs: P1-F-014
  Files: proto/schema/ipc.capnp, internal/proto/capnp/ipccp/ipc.capnp.go
  Tests: `task gen:check` green; builds and cross builds pass
  DoD: every request and event of architecture section 7 present with the payloads in the brief; item gates green; commit `feat(proto): ipc schema [FEAT-133]`

- [ ] [P1-F-016] proto/msg core: limits, errors, common, handshake and session Go types [FEAT-120, FEAT-121] [tier:haiku] [owner:P1-F] [brief:briefs/streams/p1-f.md#item-p1-f-016]
  Needs: P1-F-015
  Files: internal/proto/msg/doc.go, internal/proto/msg/codec.go, internal/proto/msg/common.go, internal/proto/msg/handshake.go, internal/proto/msg/session.go, internal/proto/msg/codec_test.go, internal/proto/msg/common_test.go, internal/proto/msg/handshake_test.go, internal/proto/msg/session_test.go
  Tests: TestUnmarshal_RejectsOversizeBeforeDecode, TestUnmarshal_TraversalLimit, TestUnmarshal_DepthLimit, TestCommon_RoundTripRectMonitorDesktopPeerID, TestCommon_RejectsTooManyMonitors, TestHandshake_RoundTripHelloWelcomeReject, TestSession_RoundTripEveryMessage, TestSession_RejectsTooManyMachines
  DoD: bounds per brief enforced on decode; coverage of internal/proto/msg >= 85%; item gates green; commit `feat(proto): message layer core, handshake and session [FEAT-120]`

- [ ] [P1-F-017] proto/msg clipboard, transfer, dnd types + Control envelope + DataHeader [FEAT-120, FEAT-081, FEAT-091, FEAT-116] [tier:haiku] [owner:P1-F] [brief:briefs/streams/p1-f.md#item-p1-f-017]
  Needs: P1-F-016
  Files: internal/proto/msg/clipboard.go, internal/proto/msg/transfer.go, internal/proto/msg/dnd.go, internal/proto/msg/control.go, internal/proto/msg/dataheader.go, internal/proto/msg/clipboard_test.go, internal/proto/msg/transfer_test.go, internal/proto/msg/dnd_test.go, internal/proto/msg/control_test.go
  Tests: TestControl_RoundTripEveryMember (table over all 29 members), TestControl_UnknownMemberIsError, TestDataHeader_RoundTripClipAndChunk, TestTransfer_RejectsUnboundedManifestPage, TestClipboard_RejectsTooManyFormats, FuzzDecodeControl, FuzzDecodeDataHeader
  DoD: fuzz smoke 30 s per target green; coverage >= 85%; item gates green; commit `feat(proto): control envelope, clipboard, transfer and dnd messages [FEAT-120]`

- [ ] [P1-F-018] proto/msg pairing and service/helper messages [FEAT-120, FEAT-127, FEAT-050] [tier:haiku] [owner:P1-F] [brief:briefs/streams/p1-f.md#item-p1-f-018]
  Needs: P1-F-017
  Files: internal/proto/msg/pairing.go, internal/proto/msg/service.go, internal/proto/msg/pairing_test.go, internal/proto/msg/service_test.go
  Tests: TestPairing_RoundTripEveryMember, TestService_RoundTripWhitelist, TestService_UnknownRequestIsError, TestHelper_RoundTrip, FuzzDecodePairing, FuzzDecodeServiceRequest, FuzzDecodeServiceResponse, FuzzDecodeHelperRequest
  DoD: fuzz smoke green; coverage >= 85%; item gates green; commit `feat(proto): pairing and service messages [FEAT-127]`

- [ ] [P1-F-019] proto/msg IPC frames, requests, responses, events [FEAT-120, FEAT-133] [tier:haiku] [owner:P1-F] [brief:briefs/streams/p1-f.md#item-p1-f-019]
  Needs: P1-F-018
  Files: internal/proto/msg/ipc.go, internal/proto/msg/ipc_types.go, internal/proto/msg/ipc_test.go
  Tests: TestIPC_RoundTripEveryRequest, TestIPC_RoundTripEveryResponse, TestIPC_RoundTripEveryEvent, TestIPC_UsesIPCTraversalLimit, FuzzDecodeIPCFrame
  DoD: fuzz smoke green; coverage >= 85%; item gates green; commit `feat(proto): ipc messages [FEAT-133]`

- [ ] [P1-F-020] Config schema and defaults (frozen; additive later) [FEAT-134, FEAT-001, FEAT-130, FEAT-180] [tier:haiku] [owner:P1-F] [brief:briefs/streams/p1-f.md#item-p1-f-020]
  Needs: P1-F-019
  Files: internal/config/doc.go, internal/config/schema.go, internal/config/defaults.go, internal/config/schema_test.go, internal/config/testdata/default.toml, go.mod, go.sum (claim; adds github.com/pelletier/go-toml/v2 v2.4.3)
  Tests: TestDefault_MatchesDecisionsD57D58 (incl. `hotkeys.send_cad` = Ctrl+Alt+End, D-113), TestDefault_IdentityModePerOS, TestTOML_RoundTripDefault, TestTOML_StrictRejectsUnknownKey, TestTOML_GoldenDefaultFile (testdata/default.toml)
  DoD: tables and keys exactly per brief; coverage >= 85%; item gates green; commit `feat(config): schema and defaults [FEAT-134]`

- [ ] [P1-F-021] Logging conventions (attribute keys, peer/secret helpers) [FEAT-135] [tier:haiku] [owner:P1-F] [brief:briefs/streams/p1-f.md#item-p1-f-021]
  Needs: P1-F-020
  Files: internal/logging/doc.go, internal/logging/conventions.go, internal/logging/conventions_test.go
  Tests: TestPeer_ShortForm, TestErr_NilAndWrapped, TestSecret_NeverPrintsValue (JSON and text handlers), TestPath_UsesKeyPath, TestComponent_AddsAttr
  DoD: coverage >= 85%; item gates green; commit `feat(logging): logging conventions [FEAT-135]`

- [ ] [P1-F-M01] Push and confirm the CI gen-check job reproduces the generated Cap'n Proto code with Ubuntu's capnp [FEAT-120, FEAT-167] [tier:haiku] [owner:USER] [brief:briefs/streams/p1-f.md#item-p1-f-m01]
  Needs: P1-F-015
  Files: qa/manual/results/p1-f-gen-check.md
  Tests: run URL of `ci.yml` on main; `gen-check` job green; capnp version printed by the job recorded
  DoD: results committed by the parent `docs(qa): CI gen-check result [FEAT-120]`; a red job becomes a BLOCKED note and a decision about the CI capnp version before C-stream items that regenerate schemas

- [ ] [P1-F-022] CONTRACT FREEZE: review interfaces against all P0 spike findings and freeze [FEAT-009, FEAT-119, FEAT-120, FEAT-133, FEAT-134] [tier:opus] [owner:PARENT] [brief:briefs/streams/p1-f.md#item-p1-f-022]
  Needs: P1-F-001 .. P1-F-021, P1-F-023
  Files: briefs/plan-phase-1.md (append freeze note), briefs/architecture.md (append freeze note), decisions.md (append any contract decisions)
  Tests: every interface, the frame table, every schema family, the msg API and the config schema checked against `briefs/spikes/sp1.md` .. `sp5.md`, `briefs/spikes/p0-consolidation.md` and the approved decisions; a written list of checked points in the note
  DoD: `task ci:local` green on main; freeze note `<!-- NOTE: <date> - CONTRACT FREEZE ... -->` appended; any needed change was made by an appended P1-F item first; commit `chore(plan): P1 contract freeze [FEAT-120]`

- [ ] [P1-F-023] Optional platform interface HomeShortcutProvider and its fake (D-114) [FEAT-181, FEAT-019] [tier:haiku] [owner:P1-F] [brief:briefs/streams/p1-f.md#item-p1-f-023]
  Needs: P1-F-005, P1-F-006
  Files: internal/platform/homeshortcuts.go, internal/platform/homeshortcuts_test.go, internal/platform/fake/homeshortcuts.go, internal/platform/fake/homeshortcuts_test.go
  Tests: TestHomeShortcutProvider_IsOptional (a capturer without the methods is treated as not providing them), TestHomeShortcutState_String, TestFakeHomeShortcuts_ActivateInvokesCallbackWithID, TestFakeHomeShortcuts_SetReplacesPreviousSet, TestFakeHomeShortcuts_StatusReportsTriggers, TestFakeHomeShortcuts_UnavailableReturnsErrUnsupported
  DoD: types and signatures exactly as in the brief; the fake is a standalone type (the fake capturer is not edited); coverage of internal/platform and internal/platform/fake >= 85%; item gates green; commit `feat(platform): optional home shortcut provider [FEAT-181]`

### C1 - Protocol, transport and session

- [ ] [P1-C1-001] Wire fuzz targets and codec benchmarks (PB-04, PB-23) [FEAT-119, FEAT-120] [tier:haiku] [owner:C1] [brief:briefs/streams/p1-c1.md#item-p1-c1-001]
  Needs: P1-F-022
  Files: internal/proto/wire/fuzz_test.go (claim), internal/proto/wire/bench_test.go (claim), internal/proto/msg/bench_test.go (claim)
  Tests: FuzzDecodeFrame, FuzzDecoderStream, FuzzDecodeDatagram, BenchmarkAppendFrame/<type>, BenchmarkDecodeFrame/<type>, BenchmarkDecoderNext, BenchmarkEncodeHello, BenchmarkDecodeHello, BenchmarkEncodeClipAnnounce, BenchmarkDecodeClipAnnounce
  DoD: fuzz smoke green; PB-04 met (0 allocs/op, <= 50 ns/op) and PB-23 met (<= 5 us/op) on the host; bench report; item gates green; commit `test(proto): wire fuzzing and codec benchmarks [FEAT-119]`

- [ ] [P1-C1-002] transport API types and length-prefixed framing [FEAT-117, FEAT-118] [tier:haiku] [owner:C1] [brief:briefs/streams/p1-c1.md#item-p1-c1-002]
  Needs: P1-F-022
  Files: internal/transport/doc.go, internal/transport/conn.go, internal/transport/errors.go, internal/transport/framing.go, internal/transport/framing_test.go
  Tests: TestMessageStream_RoundTrip, TestMessageStream_RejectsOversizeBeforeRead, TestMessageStream_ZeroLength, TestFrameStream_RoundTripMixedFrames, TestHeader_WriteRead, TestHeader_TruncatedIsUnexpectedEOF
  DoD: interfaces exactly per brief; coverage of internal/transport >= 85%; item gates green; commit `feat(transport): connection API and framing [FEAT-117]`

- [ ] [P1-C1-003] TLS 1.3 configuration builders for sessions and pairing [FEAT-117, FEAT-125] [tier:haiku] [owner:C1] [brief:briefs/streams/p1-c1.md#item-p1-c1-003]
  Needs: P1-C1-002
  Files: internal/transport/tlsconfig.go, internal/transport/tlsconfig_test.go
  Tests: TestServerConfig_TLS13RequireClientCertALPN, TestClientConfig_SkipVerifyOnlyWithVerifyConnection, TestVerifyConnection_CalledOnHandshakeAndResumption (loopback), TestPairingConfig_NoPinButCertRequired
  DoD: coverage >= 85%; item gates green; commit `feat(transport): TLS 1.3 configuration [FEAT-125]`

- [ ] [P1-C1-004] QUIC connection: channel map, datagrams, RFC 9218 priorities, data streams [FEAT-117, FEAT-027] [tier:sonnet why:quic-go stream and datagram lifecycle with RFC 9218 priority mapping] [owner:C1] [brief:briefs/streams/p1-c1.md#item-p1-c1-004]
  Needs: P1-C1-003
  Files: internal/transport/quic.go, internal/transport/quic_conn.go, internal/transport/quic_test.go, go.mod, go.sum (claim; adds github.com/quic-go/quic-go v0.64.1)
  Tests: TestQUIC_StreamOrderInputThenControl, TestQUIC_MotionViaDatagram, TestQUIC_MotionFallsBackToInputStreamWithoutDatagrams, TestQUIC_OpenAcceptDataClipboardAndFile, TestQUIC_DataStreamLimitFourPerPeer, TestQUIC_CloseReasonPropagates, TestQUIC_KeepaliveAndIdleTimeout (synctest where possible, else loopback with short config)
  DoD: priorities set exactly per architecture 5.1; coverage >= 85%; item gates green incl. -race; commit `feat(transport): QUIC connection with datagrams and priorities [FEAT-117]`

- [ ] [P1-C1-005] TCP fallback multiplexer with strict-priority writer [FEAT-118] [tier:sonnet why:strict-priority multiplexing writer concurrency] [owner:C1] [brief:briefs/streams/p1-c1.md#item-p1-c1-005]
  Needs: P1-C1-002
  Files: internal/transport/tcpmux.go, internal/transport/tcpmux_test.go
  Tests: TestMux_FrameHeaderLayout, TestMux_DemuxesChannels, TestMux_MotionAndInputBeforeControlUnderBacklog, TestMux_RejectsUnknownChannelAndOversize, TestMux_CloseUnblocksWriters, TestMux_ConcurrentWriters (race)
  DoD: coverage >= 85%; item gates green incl. -race; commit `feat(transport): TCP fallback multiplexer [FEAT-118]`

- [ ] [P1-C1-006] TCP connection: connection A + data connections with session preamble [FEAT-118] [tier:sonnet why:correlating data connections to sessions under a concurrency limit] [owner:C1] [brief:briefs/streams/p1-c1.md#item-p1-c1-006]
  Needs: P1-C1-005, P1-C1-003
  Files: internal/transport/tcp.go, internal/transport/tcp_conn.go, internal/transport/tcp_test.go
  Tests: TestTCP_ConnAOrdersInputBeforeControl, TestTCP_DataConnPreambleMatchesSession, TestTCP_DataConnWrongNonceRejected, TestTCP_DataConnLimitFour, TestTCP_NoDelaySet, TestTCP_CloseClosesDataConns
  DoD: coverage >= 85%; item gates green incl. -race; commit `feat(transport): TCP fallback connection [FEAT-118]`

- [ ] [P1-C1-007] Listener and dialer: QUIC first, TCP after 1.5 s, hostname resolution, ALPN routing incl. pairing [FEAT-118, FEAT-123] [tier:haiku] [owner:C1] [brief:briefs/streams/p1-c1.md#item-p1-c1-007]
  Needs: P1-C1-004, P1-C1-006
  Files: internal/transport/listen.go, internal/transport/dial.go, internal/transport/resolve.go, internal/transport/listen_dial_test.go
  Tests: TestDial_PrefersQUIC, TestDial_FallsBackToTCPWhenUDPSilent (UDP port bound but never answering), TestDial_ResolvesHostnameTriesAllAddresses, TestListen_RoutesSessionAndPairingByALPN, TestDialPairing_QUICAndTCP, TestListen_AllInterfaces
  DoD: coverage >= 85%; item gates green incl. -race; commit `feat(transport): listener and dialer with fallback [FEAT-118]`

- [ ] [P1-C1-008] Conn conformance suite run against QUIC and TCP on loopback [FEAT-117, FEAT-118] [tier:haiku] [owner:C1] [brief:briefs/streams/p1-c1.md#item-p1-c1-008]
  Needs: P1-C1-007
  Files: internal/transport/conformance_test.go
  Tests: TestConnSuite/QUIC and TestConnSuite/TCP with subtests Motion, InputOrdering, ControlMessages, OversizeControlRejected, DataClipboard, DataFile, DataLimit, PeerID, CloseReason, ConcurrentSenders
  DoD: identical behaviour on both kinds; item gates green incl. -race; commit `test(transport): connection conformance on QUIC and TCP [FEAT-118]`

- [ ] [P1-C1-009] Loopback latency budgets idle and under bulk (PB-03, PB-02 loopback) [FEAT-170, FEAT-171] [tier:haiku] [owner:C1] [brief:briefs/streams/p1-c1.md#item-p1-c1-009]
  Needs: P1-C1-008
  Files: internal/transport/latency_test.go
  Tests: TestLatencyBudget_QUICIdle, TestLatencyBudget_QUICUnderBulk, TestLatencyBudget_TCPIdle, TestLatencyBudget_TCPUnderBulk, BenchmarkLatencyMotionDatagram, BenchmarkLatencyInputStream
  DoD: PB-03 met (p99 <= 1 ms idle, <= 2 ms under bulk); p50/p99/max in the bench report; item gates green; commit `test(transport): loopback latency budgets [FEAT-170]`

- [ ] [P1-C1-010] Session handshake: Hello, Welcome, Reject, version and role checks [FEAT-121] [tier:haiku] [owner:C1] [brief:briefs/streams/p1-c1.md#item-p1-c1-010]
  Needs: P1-C1-008
  Files: internal/session/doc.go, internal/session/handshake.go, internal/session/handshake_test.go
  Tests: TestHandshake_Success, TestHandshake_VersionMismatchRejected, TestHandshake_RoleConflictRejected, TestHandshake_BusyCarriesHolder, TestHandshake_TimeoutFiveSeconds (synctest), TestHandshake_PeerIDMustMatchTLS, TestHandshake_NonceGenerated
  DoD: coverage of internal/session >= 85%; item gates green; commit `feat(session): handshake and version negotiation [FEAT-121]`

- [ ] [P1-C1-011] Heartbeat, RTT and loss detection [FEAT-122] [tier:haiku] [owner:C1] [brief:briefs/streams/p1-c1.md#item-p1-c1-011]
  Needs: P1-C1-010
  Files: internal/session/heartbeat.go, internal/session/heartbeat_test.go
  Tests: TestHeartbeat_SendsEverySecond, TestHeartbeat_AckUpdatesRTT, TestHeartbeat_AnyPacketResetsLoss, TestHeartbeat_LostAfterThreeSeconds, TestHeartbeat_StopsOnCancel (all synctest)
  DoD: coverage >= 85%; item gates green incl. -race; commit `feat(session): heartbeat and loss detection [FEAT-122]`

- [ ] [P1-C1-012] Reconnect backoff with full jitter and reconnect budget (PB-14) [FEAT-122] [tier:haiku] [owner:C1] [brief:briefs/streams/p1-c1.md#item-p1-c1-012]
  Needs: P1-C1-010
  Files: internal/session/backoff.go, internal/session/backoff_test.go
  Tests: TestBackoff_BoundsAndGrowth, TestBackoff_FullJitterUsesRand, TestBackoff_ResetAfterSuccess, TestBackoff_CapAtThirtySeconds, TestReconnectBudget (synctest + fault-injecting fake dialer: re-established <= 1 s after the link returns)
  DoD: PB-14 loopback budget met; coverage >= 85%; item gates green; commit `feat(session): reconnect backoff [FEAT-122]`

- [ ] [P1-C1-013] Session object: control routing, frame and motion delivery, data dispatch, latest-wins motion [FEAT-002, FEAT-027] [tier:sonnet why:ownership of per-stream reader goroutines and ordered shutdown] [owner:C1] [brief:briefs/streams/p1-c1.md#item-p1-c1-013]
  Needs: P1-C1-011
  Files: internal/session/session.go, internal/session/router.go, internal/session/motion.go, internal/session/session_test.go, internal/session/motion_test.go
  Tests: TestSession_RoutesControlByCategory, TestSession_UnroutedControlIsProtocolError, TestSession_DeliversInputFramesInOrder, TestSession_DatagramUnknownTypeDroppedAndCounted, TestSession_StreamUnknownTypeClosesSession, TestSession_DataDispatchedByHeader, TestMotionFilter_DropsOlderSeqWithWraparound, TestSession_GoodbyeAndCloseStopAllGoroutines (leak check)
  DoD: coverage >= 85%; item gates green incl. -race; commit `feat(session): session routing and motion filter [FEAT-002]`

- [ ] [P1-C1-014] Connection manager: star topology, dialing, single server session, Busy [FEAT-002, FEAT-123] [tier:sonnet why:star-topology connection lifecycle with role rules and Busy rejection] [owner:C1] [brief:briefs/streams/p1-c1.md#item-p1-c1-014]
  Needs: P1-C1-012, P1-C1-013
  Files: internal/session/manager.go, internal/session/netwatch.go, internal/session/manager_test.go, internal/session/netwatch_test.go
  Tests: TestManager_ServerDialsEveryLayoutMember, TestManager_ServerRedialsWithBackoff, TestManager_KickRedialsImmediately, TestManager_ClientAcceptsOneServer, TestManager_SameServerReconnectReplacesOldSession, TestManager_SecondServerGetsBusyWithHolder, TestManager_ClientRejectsUnpairedPeer, TestManager_RoleChangeTearsDownSessions, TestManager_EventsUpDown, TestManager_TargetsChangeCancelsDials, TestNetwatch_AddressChangeKicksAll (synctest, injected interface lister)
  DoD: coverage >= 85%; item gates green incl. -race; commit `feat(session): star topology connection manager [FEAT-002]`

- [ ] [P1-C1-015] Layout authority: LayoutSnapshot push, read-only client copy, DesktopChanged [FEAT-007, FEAT-008] [tier:haiku] [owner:C1] [brief:briefs/streams/p1-c1.md#item-p1-c1-015]
  Needs: P1-C1-014
  Files: internal/session/layoutsync.go, internal/session/layoutsync_test.go
  Tests: TestLayoutSync_PushOnConnect, TestLayoutSync_PushOnChangeToAll, TestLayoutSync_ClientStoresReadOnlyCopy, TestLayoutSync_ClientRejectsSetFromNonServer, TestLayoutSync_DesktopChangedUpdatesServerGeometry, TestLayoutSync_StaleVersionIgnored
  DoD: coverage >= 85%; item gates green; commit `feat(session): layout authority and desktop changes [FEAT-007]`

- [ ] [P1-C1-016] Loopback session end-to-end on QUIC and TCP [FEAT-002, FEAT-122] [tier:haiku] [owner:C1] [brief:briefs/streams/p1-c1.md#item-p1-c1-016]
  Needs: P1-C1-015
  Files: internal/session/loopback_test.go
  Tests: TestLoopback/QUIC and TestLoopback/TCP: connect, handshake, layout snapshot, input frames, motion, control, data stream, peer loss detected, reconnect after listener restart
  DoD: item gates green incl. -race; coverage of internal/session >= 85%; commit `test(session): loopback end-to-end [FEAT-002]`

- [ ] [P1-C1-017] C1 code review gate [FEAT-117, FEAT-118, FEAT-121, FEAT-122] [tier:sonnet why:stream review gate (code-reviewer)] [owner:C1] [brief:briefs/streams/p1-c1.md#item-p1-c1-017]
  Needs: P1-C1-001 .. P1-C1-016
  Files: none (findings become appended fix items)
  Tests: code-reviewer over the C1 merged range against C1 items
  DoD: VERDICT pass, or every finding turned into an appended C1 item and fixed, then re-reviewed to pass

- [ ] [P1-C1-018] C1 security review gate (decoders, TLS, network surface) [FEAT-173] [tier:sonnet why:security review gate against qa/security-review.md] [owner:C1] [brief:briefs/streams/p1-c1.md#item-p1-c1-018]
  Needs: P1-C1-017
  Files: none (findings become appended fix items)
  Tests: qa/security-review.md sections 2, 5, 8 applied to internal/transport, internal/session, internal/proto
  DoD: VERDICT pass after fixes

- [ ] [P1-C1-019] C1 Opus consolidation review [FEAT-117] [tier:opus] [owner:PARENT] [brief:briefs/streams/p1-c1.md#item-p1-c1-019]
  Needs: P1-C1-018
  Files: none
  Tests: cross-stream consistency with C2 (TLS verifier, pairing conns), C3 (engine Sender), C4 (data streams), C5 (agent wiring); contract adherence; decisions D-35, D-43, D-49, D-57, D-59, D-60, D-78
  DoD: no open finding

- [ ] [P1-C1-020] C1 review chain green note [FEAT-117] [tier:opus] [owner:PARENT] [brief:briefs/streams/p1-c1.md#item-p1-c1-020]
  Needs: P1-C1-019
  Files: briefs/plan-phase-1.md (append note)
  Tests: none
  DoD: `<!-- NOTE: <date> - stream C1 review chain green -->` appended; commit `chore(plan): C1 review chain green [FEAT-117]`

### C2 - PKI, SPAKE2, pairing, peers, discovery, Wake-on-LAN

- [ ] [P1-C2-001] pki identity: ECDSA P-256 self-signed cert, re-issue, pluggable signer [FEAT-124, FEAT-051] [tier:haiku] [owner:C2] [brief:briefs/streams/p1-c2.md#item-p1-c2-001]
  Needs: P1-F-022
  Files: internal/pki/doc.go, internal/pki/identity.go, internal/pki/identity_test.go
  Tests: TestGenerate_CertFields (CN, 128-bit serial, 10-year validity, P-256, self-signed verifies), TestReissue_SameKeyWithin30Days, TestReissue_NotNeededEarly, TestIdentity_RemoteSignerHandshake (wrapping signer completes a TLS 1.3 loopback handshake), TestIdentity_PeerIDMatchesSPKI
  DoD: coverage of internal/pki >= 85%; item gates green; commit `feat(pki): identity certificates [FEAT-124]`

- [ ] [P1-C2-002] pki storage on macOS/Linux: PKCS#8 PEM 0600 in 0700 [FEAT-124] [tier:haiku] [owner:C2] [brief:briefs/streams/p1-c2.md#item-p1-c2-002]
  Needs: P1-C2-001
  Files: internal/pki/store.go, internal/pki/store_unix.go, internal/pki/store_unix_test.go
  Tests (unix build tag, run on CI linux and macOS): TestStore_CreateThenLoad, TestStore_PermissionsExact, TestStore_RefusesGroupOrWorldReadableKey, TestStore_AtomicWriteLeavesNoTemp, TestStore_ReissuesExpiringCert
  DoD: cross builds green; host tests of store.go (shared logic) pass; item gates green; commit `feat(pki): unix identity storage [FEAT-124]`

- [ ] [P1-C2-003] pki storage on Windows (user mode): DPAPI user scope + owner-only DACL [FEAT-124] [tier:haiku] [owner:C2] [brief:briefs/streams/p1-c2.md#item-p1-c2-003]
  Needs: P1-C2-002
  Files: internal/pki/store_windows.go, internal/pki/store_windows_test.go, go.mod, go.sum (claim; golang.org/x/sys v0.49.0 if not yet required)
  Tests (host): TestStoreWindows_CreateThenLoad, TestStoreWindows_KeyFileIsDPAPIBlob, TestStoreWindows_DACLOwnerOnly (reads back the security descriptor), TestStoreWindows_MachineModeIsExplicitError (mode "machine" returns ErrServiceRequired without the service, D-41)
  DoD: coverage of internal/pki >= 85% on the host; item gates green; commit `feat(pki): windows user-mode identity storage [FEAT-124]`

- [ ] [P1-C2-004] Pin verification via VerifyConnection (D-42) [FEAT-125] [tier:haiku] [owner:C2] [brief:briefs/streams/p1-c2.md#item-p1-c2-004]
  Needs: P1-C2-001, P1-C1-003
  Files: internal/pki/verify.go, internal/pki/verify_test.go
  Tests: TestVerifier_AcceptsPinnedPeer, TestVerifier_RejectsUnknownPeer, TestVerifier_RejectsUnpairedAfterRemoval, TestVerifier_NoCertificateRejected, TestVerifier_RunsOnResumedConnection (TLS 1.3 session ticket resumption over loopback), TestVerifier_ReissuedCertSameKeyAccepted
  DoD: coverage >= 85%; item gates green; commit `feat(pki): SPKI pin verification [FEAT-125]`

- [ ] [P1-C2-005] SPAKE2 RFC 9382 P-256 core with official test vectors [FEAT-126] [tier:sonnet why:RFC 9382 cryptographic core (M/N constants, transcript, key schedule)] [owner:C2] [brief:briefs/streams/p1-c2.md#item-p1-c2-005]
  Needs: P1-F-022, P0-SP5-008
  Files: internal/spake2/doc.go, internal/spake2/spake2.go, internal/spake2/constants.go, internal/spake2/spake2_test.go, internal/spake2/testdata/rfc9382_p256.json, go.mod, go.sum (claim; adds filippo.io/nistec v0.0.4)
  Tests: TestRFC9382_VectorsP256SHA256 (every Appendix B P-256 vector byte for byte), TestExchange_SameCodeAgrees, TestExchange_WrongCodeFailsConfirmation, TestConstants_MatchRFC
  DoD: RFC sections cited in code comments where non-obvious; coverage >= 85%; item gates green; commit `feat(spake2): RFC 9382 SPAKE2 over P-256 [FEAT-126]`

- [ ] [P1-C2-006] SPAKE2 negative tests, message-parsing fuzz, benchmark (PB-22) [FEAT-126] [tier:haiku] [owner:C2] [brief:briefs/streams/p1-c2.md#item-p1-c2-006]
  Needs: P1-C2-005
  Files: internal/spake2/negative_test.go, internal/spake2/fuzz_test.go, internal/spake2/bench_test.go
  Tests: TestPeerShare_IdentityRejected, TestPeerShare_OffCurveRejected, TestPeerShare_WrongLengthRejected, TestConfirm_ConstantTimeCompareUsed, TestFinish_TwiceFails, FuzzPeerShare, FuzzConfirm, BenchmarkExchangeFull
  DoD: PB-22 met (<= 50 ms per side); fuzz smoke green; bench report; item gates green; commit `test(spake2): negative cases, fuzzing and benchmark [FEAT-126]`

- [ ] [P1-C2-007] Pairing code issuer: 6 digits, TTL 120 s, 3 attempts [FEAT-127] [tier:haiku] [owner:C2] [brief:briefs/streams/p1-c2.md#item-p1-c2-007]
  Needs: P1-F-022
  Files: internal/pairing/doc.go, internal/pairing/code.go, internal/pairing/code_test.go
  Tests: TestCode_SixDigitsUniform (rejection sampling, no modulo bias), TestCode_ExpiresAfter120s (synctest), TestCode_InvalidAfterThreeFailures, TestCode_AttemptsCountedPerCodeNotPerConnection, TestCode_SingleActiveCode, TestCode_CancelInvalidates
  DoD: coverage of internal/pairing >= 85%; item gates green; commit `feat(pairing): pairing code issuer [FEAT-127]`

- [ ] [P1-C2-008] Pairing protocol: PairHello, exporter binding, SPAKE2 shares, confirmation, result [FEAT-127] [tier:sonnet why:pairing state machine with channel binding and attempt accounting] [owner:C2] [brief:briefs/streams/p1-c2.md#item-p1-c2-008]
  Needs: P1-C2-006, P1-C2-007, P1-F-018
  Files: internal/pairing/protocol.go, internal/pairing/initiator.go, internal/pairing/responder.go, internal/pairing/protocol_test.go
  Tests: TestPair_Success (net.Pipe + fake connection state), TestPair_WrongCodeDecrementsAttempts, TestPair_ThirdFailureInvalidates, TestPair_ExpiredCode, TestPair_ExporterMismatchFails (MITM simulation), TestPair_NoPinBeforeConfirmation, TestPair_UniformFailureToPeer, TestPair_CancelMidway, TestPair_TimeoutPerStep (synctest)
  DoD: coverage >= 85%; item gates green incl. -race; commit `feat(pairing): pairing protocol [FEAT-127]`

- [ ] [P1-C2-009] Peer store peers.toml (pins, names, MACs, addresses) [FEAT-128] [tier:haiku] [owner:C2] [brief:briefs/streams/p1-c2.md#item-p1-c2-009]
  Needs: P1-C2-004
  Files: internal/peers/doc.go, internal/peers/store.go, internal/peers/store_test.go
  Tests: TestStore_AddGetListSorted, TestStore_Rename, TestStore_RemoveUnpairs, TestStore_PinLookupImplementsPinStore, TestStore_AtomicPersistAndReload, TestStore_StrictDecodeRejectsUnknownKeys, TestStore_SetNetInfo, TestStore_ConcurrentAccess (race), TestStore_DefaultNameIsHostname
  DoD: coverage of internal/peers >= 85%; item gates green incl. -race; commit `feat(peers): peer store [FEAT-128]`

- [ ] [P1-C2-010] Pairing end-to-end over loopback transport (QUIC and TCP) [FEAT-127, FEAT-125] [tier:haiku] [owner:C2] [brief:briefs/streams/p1-c2.md#item-p1-c2-010]
  Needs: P1-C2-008, P1-C2-009, P1-C1-007
  Files: internal/pairing/loopback_test.go
  Tests: TestLoopbackPairing/QUIC, TestLoopbackPairing/TCP (success stores both pins; a session then connects with pin verification), TestLoopbackPairing_ThreeWrongCodes, TestLoopbackPairing_ExpiredCode
  DoD: item gates green incl. -race; commit `test(pairing): loopback pairing end-to-end [FEAT-127]`

- [ ] [P1-C2-011] mDNS TXT model, advertiser and browser [FEAT-129] [tier:haiku] [owner:C2] [brief:briefs/streams/p1-c2.md#item-p1-c2-011]
  Needs: P1-F-022, P0-SP5-008 (library choice)
  Files: internal/discovery/doc.go, internal/discovery/txt.go, internal/discovery/txt_test.go, internal/discovery/mdns.go, internal/discovery/mdns_integration_test.go, go.mod, go.sum (claim; adds the library chosen in `briefs/spikes/sp5.md`, default github.com/hashicorp/mdns v1.0.7)
  Tests: TestTXT_FormatExactKeys, TestTXT_ParseValid, TestTXT_ParseRejectsMissingOrBadValues, TestTXT_IgnoresUnknownKeys, TestServiceName (host); TestMDNS_AdvertiseAndBrowseLocal (integration)
  DoD: TXT contains only v, id, role, tcp (D-43); coverage >= 85% (integration file excluded by tag); item gates green; commit `feat(discovery): mDNS advertise and browse [FEAT-129]`

- [ ] [P1-C2-012] Discovery registry: mDNS + manual peers, expiry, dedupe, events [FEAT-129, FEAT-130] [tier:haiku] [owner:C2] [brief:briefs/streams/p1-c2.md#item-p1-c2-012]
  Needs: P1-C2-011, P1-C2-009
  Files: internal/discovery/registry.go, internal/discovery/registry_test.go
  Tests: TestRegistry_MergesSourcesByPeerID, TestRegistry_ExpiresAfterTTL (synctest), TestRegistry_ManualEntriesNeverExpire, TestRegistry_MarksPaired, TestRegistry_ManualResolveFailureReported, TestRegistry_EventsOnChange, TestRegistry_SkipsSelf
  DoD: coverage >= 85%; item gates green incl. -race; commit `feat(discovery): discovery registry [FEAT-130]`

- [ ] [P1-C2-013] Wake-on-LAN: magic packet, local net info, sender, auto-wake throttle [FEAT-131] [tier:haiku] [owner:C2] [brief:briefs/streams/p1-c2.md#item-p1-c2-013]
  Needs: P1-F-022
  Files: internal/wol/doc.go, internal/wol/packet.go, internal/wol/netinfo.go, internal/wol/sender.go, internal/wol/broadcast_unix.go, internal/wol/broadcast_windows.go, internal/wol/throttle.go, internal/wol/wol_test.go, internal/wol/wol_integration_test.go
  Tests: TestMagicPacket_Layout, TestMagicPacket_RejectsBadMAC, TestDirectedBroadcast_IPv4Masks, TestLocalNetInfo_SkipsLoopbackAndVirtualless (injected interface list), TestSender_SendsToEveryTarget (UDP listener on 127.0.0.1), TestThrottle_OncePer60sPerPeer (synctest); TestSender_RealBroadcast (integration)
  DoD: coverage >= 85%; item gates green; commit `feat(wol): wake-on-lan [FEAT-131]`

- [ ] [P1-C2-014] Manual script: discovery time and VPN interfaces (PB-13) [FEAT-129] [tier:haiku] [owner:C2] [brief:briefs/streams/p1-c2.md#item-p1-c2-014]
  Needs: P1-C2-012
  Files: qa/manual/p1-c2-discovery.md
  Tests: script follows `qa/manual/README.md`; runs `go test -tags integration -run 'TestMDNS_' -v ./internal/discovery/` on Windows PC 1, Windows PC 2 and the GNOME box simultaneously with the advertise-for-60 s mode; records time to first discovery and interfaces
  DoD: commit `docs(qa): discovery manual script [FEAT-129]`

- [ ] [P1-C2-M01] Run the discovery script and record results [FEAT-129] [tier:haiku] [owner:USER] [brief:briefs/streams/p1-c2.md#item-p1-c2-m01]
  Needs: P1-C2-014
  Files: qa/manual/results/p1-c2-discovery.md
  Tests: every step has a recorded observation; PB-13 (<= 3 s p95) evaluated
  DoD: results committed by the parent `docs(qa): discovery results [FEAT-129]`; PB-13 row appended by the parent

- [ ] [P1-C2-015] C2 code review gate [FEAT-124, FEAT-126, FEAT-127, FEAT-128, FEAT-129, FEAT-131] [tier:sonnet why:stream review gate (code-reviewer)] [owner:C2] [brief:briefs/streams/p1-c2.md#item-p1-c2-015]
  Needs: P1-C2-001 .. P1-C2-014
  Files: none (findings become appended fix items)
  Tests: code-reviewer over the C2 merged range
  DoD: VERDICT pass after fixes

- [ ] [P1-C2-016] C2 security review gate (pki, SPAKE2, pairing, discovery, WoL) [FEAT-173] [tier:sonnet why:security review gate against qa/security-review.md] [owner:C2] [brief:briefs/streams/p1-c2.md#item-p1-c2-016]
  Needs: P1-C2-015
  Files: none (findings become appended fix items)
  Tests: qa/security-review.md sections 1, 2, 8, 9 applied to internal/pki, internal/spake2, internal/pairing, internal/peers, internal/discovery, internal/wol
  DoD: VERDICT pass after fixes; recommendation for external review of internal/spake2 recorded in the stream note (R-33)

- [ ] [P1-C2-017] C2 Opus consolidation review [FEAT-126] [tier:opus] [owner:PARENT] [brief:briefs/streams/p1-c2.md#item-p1-c2-017]
  Needs: P1-C2-016
  Files: none
  Tests: consistency with C1 (TLS configs, pairing conns), C5 (IPC pairing flows), decisions D-14, D-15, D-41, D-42, D-43, D-57, D-70
  DoD: no open finding

- [ ] [P1-C2-018] C2 review chain green note [FEAT-126] [tier:opus] [owner:PARENT] [brief:briefs/streams/p1-c2.md#item-p1-c2-018]
  Needs: P1-C2-017
  Files: briefs/plan-phase-1.md (append note)
  Tests: none
  DoD: note appended; commit `chore(plan): C2 review chain green [FEAT-126]`

### C3 - Layout, engine and keymap

- [ ] [P1-C3-001] Vendor Chromium dom_code_data.inc with BSD notice [FEAT-024] [tier:haiku] [owner:C3] [brief:briefs/streams/p1-c3.md#item-p1-c3-001]
  Needs: P1-F-022
  Files: third_party/chromium/dom_code_data.inc, third_party/chromium/LICENSE, third_party/chromium/README.md
  Tests: file fetched from chromium.googlesource.com at a recorded commit; SHA-256 recorded in README; LICENSE is Chromium's BSD-3-Clause text
  DoD: commit `chore(keymap): vendor chromium dom code table [FEAT-024]`

- [ ] [P1-C3-002] Keymap table generator and generated tables [FEAT-024] [tier:haiku] [owner:C3] [brief:briefs/streams/p1-c3.md#item-p1-c3-002]
  Needs: P1-C3-001
  Files: internal/keymap/gen/main.go, internal/keymap/gen/parse.go, internal/keymap/gen/parse_test.go, internal/keymap/gen/testdata/sample.inc, internal/keymap/generate.go, internal/keymap/tables_gen.go
  Tests: TestParse_DomCodeLines, TestParse_SkipsCommentsAndMacros, TestParse_RejectsMalformed, TestRender_DeterministicSorted; `go generate ./internal/keymap/...` reproduces tables_gen.go byte for byte
  DoD: coverage of internal/keymap/gen >= 85%; `task gen:check` green; item gates green; commit `feat(keymap): generate HID tables from chromium data [FEAT-024]`

- [ ] [P1-C3-003] Table functions for Windows scancodes, macOS keycodes, evdev [FEAT-021, FEAT-022, FEAT-023] [tier:haiku] [owner:C3] [brief:briefs/streams/p1-c3.md#item-p1-c3-003]
  Needs: P1-C3-002
  Files: internal/keymap/tables.go, internal/keymap/tables_test.go
  Tests: TestWindows_KnownKeys (A, Enter, Escape, ArrowLeft extended, ControlRight extended, NumpadEnter extended, Pause, NumLock, PrintScreen), TestWindows_RoundTripAllMapped, TestMac_KnownKeys (KeyA=0x00, Enter=0x24, MetaLeft=0x37), TestMac_RoundTripAllMapped, TestEvdev_KnownKeys (KeyA=30, ArrowLeft=105), TestEvdev_RoundTripAllMapped, TestUnmapped_ReturnsFalse
  DoD: signatures exactly as architecture section 3; coverage of internal/keymap >= 85%; item gates green; commit `feat(keymap): per-OS key tables [FEAT-021]`

- [ ] [P1-C3-004] Key names and per-machine modifier remap [FEAT-025, FEAT-019] [tier:haiku] [owner:C3] [brief:briefs/streams/p1-c3.md#item-p1-c3-004]
  Needs: P1-C3-003
  Files: internal/keymap/names.go, internal/keymap/remap.go, internal/keymap/names_test.go, internal/keymap/remap_test.go
  Tests: TestName_RoundTripAllMapped, TestParseName_Aliases (Ctrl, Alt, Shift, Win, Cmd, Meta), TestRemap_ApplyIdentityByDefault, TestRemap_SwapCtrlMeta, TestRemap_RejectsNonModifiers, TestRemap_FromConfigTable, TestDefaultRemap_EmptyForEveryOS
  DoD: coverage >= 85%; item gates green; commit `feat(keymap): key names and modifier remap [FEAT-025]`

- [ ] [P1-C3-005] Layout model and canvas geometry (D-51) [FEAT-003] [tier:haiku] [owner:C3] [brief:briefs/streams/p1-c3.md#item-p1-c3-005]
  Needs: P1-F-022
  Files: internal/layout/doc.go, internal/layout/layout.go, internal/layout/canvas.go, internal/layout/canvas_test.go
  Tests: TestMonitorCanvasRect_SingleMonitor, TestMonitorCanvasRect_MixedScale, TestMonitorCanvasRect_NegativeDesktopOrigin, TestMachineBounds, TestLayout_FromConfigAndBack
  DoD: formulas exactly per brief; coverage of internal/layout >= 85%; item gates green; commit `feat(layout): layout model and canvas geometry [FEAT-003]`

- [ ] [P1-C3-006] Real edges for multi-monitor desktops [FEAT-004] [tier:haiku] [owner:C3] [brief:briefs/streams/p1-c3.md#item-p1-c3-006]
  Needs: P1-C3-005
  Files: internal/layout/edges.go, internal/layout/edges_test.go
  Tests: TestRealEdges_Single, TestRealEdges_SideBySide, TestRealEdges_LShape, TestRealEdges_StackedWithOffset, TestRealEdges_GapBetweenMonitors, TestRealEdges_MixedSizes, TestRealEdges_ThreeInRowPartialOverlaps, TestRealEdges_DeterministicOrder
  DoD: coverage >= 85%; item gates green; commit `feat(layout): real edge computation [FEAT-004]`

- [ ] [P1-C3-007] Adjacency and crossing mapping with walls (D-51) [FEAT-005, FEAT-007] [tier:haiku] [owner:C3] [brief:briefs/streams/p1-c3.md#item-p1-c3-007]
  Needs: P1-C3-006
  Files: internal/layout/adjacency.go, internal/layout/mapping.go, internal/layout/adjacency_test.go, internal/layout/mapping_test.go
  Tests: TestAdjacency_TouchingMachines, TestAdjacency_ToleranceTwoLogicalPx, TestAdjacency_NoFacingNoLink, TestEdgesWithNeighbours_SplitsSegmentsByOverlap, TestMap_ContinuitySameScale, TestMap_ContinuityMixedScale, TestMap_OutsideOverlapIsWall, TestMap_ClampsInsideTarget, TestMap_UnknownEdgeIsError, TestExit_ThroughRealEdgeWithNeighbour, TestExit_WallReturnsFalse, TestExit_InternalEdgeIsNotExit, TestClamp_IntoGapStaysOnSourceMonitor
  DoD: coverage >= 85%; item gates green; commit `feat(layout): adjacency and crossing mapping [FEAT-005]`

- [ ] [P1-C3-008] Layout validation and snapping helpers [FEAT-006] [tier:haiku] [owner:C3] [brief:briefs/streams/p1-c3.md#item-p1-c3-008]
  Needs: P1-C3-007
  Files: internal/layout/validate.go, internal/layout/snap.go, internal/layout/validate_test.go, internal/layout/snap_test.go
  Tests: TestValidate_Overlap, TestValidate_Unreachable, TestValidate_DuplicateID, TestValidate_TooManyMachines, TestValidate_EmptyDesktop, TestValidate_OK, TestSnap_ToNeighbourEdgeWithinThreshold, TestSnap_NoSnapBeyondThreshold, TestSnap_PrefersClosestEdge, TestSnap_AlignsCorners
  DoD: coverage >= 85%; item gates green; commit `feat(layout): validation and snapping [FEAT-006]`

- [ ] [P1-C3-009] Key state tracker, release-all and modifier sync [FEAT-020, FEAT-026] [tier:haiku] [owner:C3] [brief:briefs/streams/p1-c3.md#item-p1-c3-009]
  Needs: P1-F-022
  Files: internal/engine/doc.go, internal/engine/keystate.go, internal/engine/keystate_test.go
  Tests: TestKeyState_TracksDownUp, TestKeyState_RepeatDoesNotDuplicate, TestKeyState_ModMask, TestKeyState_ReleaseAllFramesSorted, TestKeyState_ButtonsTracked, TestKeyState_ZeroAllocs
  DoD: coverage of internal/engine >= 85%; item gates green; commit `feat(engine): key state tracking and release-all [FEAT-026]`

- [ ] [P1-C3-010] Hotkey bindings: parse, format, defaults, conflicts, reserved list, matcher [FEAT-019, FEAT-180] [tier:haiku] [owner:C3] [brief:briefs/streams/p1-c3.md#item-p1-c3-010]
  Needs: P1-C3-004, P1-C3-009
  Files: internal/engine/hotkey.go, internal/engine/hotkey_test.go
  Tests: TestParseBinding_Valid, TestParseBinding_Invalid, TestFormatBinding_Canonical, TestDefaults_D58, TestJumpDefaults_ByLayoutOrderFirst12, TestConflicts_Duplicates, TestReserved_KnownCombosWarn, TestMatcher_MatchesWithEitherSideModifier, TestMatcher_SwallowsTriggerDownAndUp, TestMatcher_SwallowSetForLocalMode, TestDefaults_SendCADCtrlAltEnd, TestConflicts_SendCADDuplicates, TestMatcher_SendCADMatchesAndSwallowsInRemoteMode, TestMatcher_SwallowSetForLocalModeExcludesSendCAD, TestMatcher_ZeroAllocs
  DoD: coverage >= 85%; item gates green; commit `feat(engine): hotkey bindings and matcher [FEAT-019]`

- [ ] [P1-C3-011] Guard framework and modifier-required guard [FEAT-014] [tier:haiku] [owner:C3] [brief:briefs/streams/p1-c3.md#item-p1-c3-011]
  Needs: P1-C3-009
  Files: internal/engine/guard.go, internal/engine/guard_modifier.go, internal/engine/guard_test.go, internal/engine/guard_modifier_test.go
  Tests: TestGuardSet_AllOffAllowsImmediately, TestGuardSet_OrderAndShortCircuit, TestGuardSet_HotkeySkipsGuards, TestModifierGuard_AllowsWithLeftOrRightCtrl, TestModifierGuard_DeniesWithout, TestModifierGuard_ConfiguredKeys
  DoD: coverage >= 85%; item gates green; commit `feat(engine): guard framework and modifier guard [FEAT-014]`

- [ ] [P1-C3-012] Dwell guard: contact must persist 150 ms before switching [FEAT-011] [tier:sonnet why:timer-driven guard state machine] [owner:C3] [brief:briefs/streams/p1-c3.md#item-p1-c3-012]
  Needs: P1-C3-011
  Files: internal/engine/guard_dwell.go, internal/engine/guard_dwell_test.go
  Tests: TestDwell_SwitchesAfterDelay, TestDwell_CancelsWhenPointerLeaves, TestDwell_DisabledSwitchesImmediately, TestDwell_ContactOnOtherEdgeRestarts, TestDwell_ConfiguredDelay (synctest)
  DoD: tests green under synctest; 0 allocs/op in BenchmarkDwellEvaluate; coverage of internal/engine >= 85%; item gates green; commit `feat(engine): dwell guard [FEAT-011]`

- [ ] [P1-C3-013] Double-tap guard [FEAT-012] [tier:haiku] [owner:C3] [brief:briefs/streams/p1-c3.md#item-p1-c3-013]
  Needs: P1-C3-012
  Files: internal/engine/guard_doubletap.go, internal/engine/guard_doubletap_test.go
  Tests: TestDoubleTap_SecondContactWithinWindowSwitches, TestDoubleTap_WindowExpired, TestDoubleTap_RequiresMoveAwayEightPx, TestDoubleTap_DifferentEdgeResets (synctest)
  DoD: coverage >= 85%; item gates green; commit `feat(engine): double-tap guard [FEAT-012]`

- [ ] [P1-C3-014] Corner dead-zone guard [FEAT-013] [tier:haiku] [owner:C3] [brief:briefs/streams/p1-c3.md#item-p1-c3-014]
  Needs: P1-C3-011
  Files: internal/engine/guard_corner.go, internal/engine/guard_corner_test.go
  Tests: TestCorner_DeniesNearSegmentEnds, TestCorner_AllowsMiddle, TestCorner_UsesLogicalPxWithScale, TestCorner_ShortSegmentFullyDead
  DoD: coverage >= 85%; item gates green; commit `feat(engine): corner dead-zone guard [FEAT-013]`

- [ ] [P1-C3-015] Server engine core: switch, virtual cursor, crossings, Enter/Leave, release-all + mod sync [FEAT-009, FEAT-026, FEAT-027, FEAT-029, FEAT-030] [tier:sonnet why:engine state machine concurrency (single owner loop, commands, capture sink)] [owner:C3] [brief:briefs/streams/p1-c3.md#item-p1-c3-015]
  Needs: P1-C3-007, P1-C3-010, P1-C3-013, P1-C3-014
  Files: internal/engine/server.go, internal/engine/sender.go, internal/engine/cursor.go, internal/engine/server_test.go, internal/engine/cursor_test.go
  Tests: TestServer_ContactSwitchesToNeighbour, TestServer_AnswersEveryContactWithGrabOrUngrab (D-90), TestServer_ContactHolderDecidedWithin400ms (D-90, synctest), TestServer_GrabAndEnterSent, TestServer_MotionScaledToClient, TestServer_CrossClientEdgeToThirdMachine, TestServer_CrossBackHomeUngrabsAtMappedPoint, TestServer_ReleaseAllAndModSyncOnSwitch, TestServer_ButtonsIncludingX1X2Forwarded, TestServer_WheelHighResForwarded, TestServer_KeysForwardedWhileRemote, TestServer_GuardDenyKeepsHome, TestCursor_ClampsToClientDesktop
  DoD: coverage >= 85%; item gates green incl. -race; commit `feat(engine): server engine core [FEAT-009]`

- [ ] [P1-C3-016] Layout updates, SetEdges recomputation, online state walls, auto-wake trigger [FEAT-007, FEAT-008, FEAT-032] [tier:haiku] [owner:C3] [brief:briefs/streams/p1-c3.md#item-p1-c3-016]
  Needs: P1-C3-015
  Files: internal/engine/server_layout.go, internal/engine/server_layout_test.go
  Tests: TestServer_SetLayoutCallsSetEdges, TestServer_DesktopChangeRecomputesEdges, TestServer_OfflineNeighbourIsWall, TestServer_OfflineContactRequestsWake, TestServer_ComingOnlineReenablesEdge, TestServer_LayoutChangeWhileRemoteKeepsOrReturns
  DoD: coverage >= 85%; item gates green incl. -race; commit `feat(engine): layout updates and offline walls [FEAT-032]`

- [ ] [P1-C3-017] Motion coalescing 1 ms latest-wins (PB-06) [FEAT-028] [tier:haiku] [owner:C3] [brief:briefs/streams/p1-c3.md#item-p1-c3-017]
  Needs: P1-C3-015
  Files: internal/engine/coalesce.go, internal/engine/coalesce_test.go
  Tests: TestCoalesce_OnePerMillisecond, TestCoalesce_LatestWins, TestCoalesce_FlushOnNonMotionKeepsOrder, TestCoalescing1000Hz (synctest: <= 1000 MotionAbs/s, no backlog older than 2 ms), TestCoalesce_ZeroAllocs
  DoD: PB-06 met; coverage >= 85%; item gates green; commit `feat(engine): motion coalescing [FEAT-028]`

- [ ] [P1-C3-018] Hotkey actions: lock to screen, jump, cycle, panic, capture-next [FEAT-015, FEAT-016, FEAT-017, FEAT-018] [tier:haiku] [owner:C3] [brief:briefs/streams/p1-c3.md#item-p1-c3-018]
  Needs: P1-C3-016
  Files: internal/engine/server_hotkeys.go, internal/engine/server_hotkeys_test.go
  Tests: TestLock_BlocksEdgeSwitching, TestLock_ToggleEvent, TestJump_SkipsGuardsEntersAtCentre, TestJump_UnknownOrOfflineIgnoredWithEvent, TestCycle_NextPrevByLayoutOrderWrapsAndSkipsOffline, TestPanic_ReturnsHomeAndReleasesAllClients, TestHotkeyKeysNeverForwarded, TestCaptureNextHotkey_ReportsComboWithoutActing
  DoD: coverage >= 85%; item gates green incl. -race; commit `feat(engine): hotkey actions [FEAT-015]`

- [ ] [P1-C3-019] Return home when the active client is lost [FEAT-031] [tier:haiku] [owner:C3] [brief:briefs/streams/p1-c3.md#item-p1-c3-019]
  Needs: P1-C3-016
  Files: internal/engine/server_failure.go, internal/engine/server_failure_test.go
  Tests: TestLoss_ActiveClientLostReturnsHome, TestLoss_InactiveClientLostNoSwitch, TestLoss_SendErrorTreatedAsLoss, TestLoss_CaptureLostEventReturnsHome, TestLoss_UngrabAtLastHomeExitPoint
  DoD: coverage >= 85%; item gates green incl. -race; commit `feat(engine): return home on loss [FEAT-031]`

- [ ] [P1-C3-020] Client engine: Enter/Leave, injection, remap, mod sync, overlay trigger [FEAT-010, FEAT-025, FEAT-029, FEAT-030] [tier:haiku] [owner:C3] [brief:briefs/streams/p1-c3.md#item-p1-c3-020]
  Needs: P1-C3-009, P1-C3-004
  Files: internal/engine/client.go, internal/engine/client_test.go
  Tests: TestClient_EnterShowsCursorAndOverlay, TestClient_MotionInjectsAbs, TestClient_KeyAppliesRemap, TestClient_ButtonsAndWheel, TestClient_ReleaseAllReleasesInjected, TestClient_ModSyncReconciles, TestClient_LeaveReleasesAll, TestClient_FramesBeforeEnterIgnoredAndCounted, TestClient_InjectErrorReported
  DoD: coverage >= 85%; item gates green incl. -race; commit `feat(engine): client engine [FEAT-010]`

- [ ] [P1-C3-021] Engine integration on fakes: server + two clients in memory [FEAT-009, FEAT-010] [tier:haiku] [owner:C3] [brief:briefs/streams/p1-c3.md#item-p1-c3-021]
  Needs: P1-C3-017, P1-C3-018, P1-C3-019, P1-C3-020
  Files: internal/engine/integration_test.go
  Tests: TestIntegration_SwitchTypeAndReturn, TestIntegration_ChainToThirdMachine, TestIntegration_PanicFromThirdMachine, TestIntegration_LockToScreen, TestIntegration_DwellAndCornerGuards, TestIntegration_ClientLossReturnsHome, TestIntegration_NoStuckKeysAcrossSwitches (synctest)
  DoD: item gates green incl. -race; commit `test(engine): engine integration on fakes [FEAT-009]`

- [ ] [P1-C3-022] Engine and layout benchmarks (PB-05, 64-machine mapping) [FEAT-170, FEAT-172] [tier:haiku] [owner:C3] [brief:briefs/streams/p1-c3.md#item-p1-c3-022]
  Needs: P1-C3-021
  Files: internal/engine/bench_test.go, internal/layout/bench_test.go
  Tests: BenchmarkServerMotion, BenchmarkServerKey, BenchmarkClientMotion, BenchmarkDwellEvaluate, BenchmarkMap64Machines, BenchmarkEdgesWithNeighbours64, TestEngineBudget (p99 per-event <= 5 us over 100 000 events, 0 allocs)
  DoD: PB-05 met; bench report; item gates green; commit `test(engine): engine and layout benchmarks [FEAT-170]`

- [ ] [P1-C3-023] C3 code review gate [FEAT-009, FEAT-010, FEAT-003, FEAT-019, FEAT-021] [tier:sonnet why:stream review gate (code-reviewer)] [owner:C3] [brief:briefs/streams/p1-c3.md#item-p1-c3-023]
  Needs: P1-C3-001 .. P1-C3-022, P1-C3-026, P1-C3-027
  Files: none (findings become appended fix items)
  Tests: code-reviewer over the C3 merged range, incl. provenance of the keymap data (D-27)
  DoD: VERDICT pass after fixes

- [ ] [P1-C3-024] C3 Opus consolidation review [FEAT-009] [tier:opus] [owner:PARENT] [brief:briefs/streams/p1-c3.md#item-p1-c3-024]
  Needs: P1-C3-023
  Files: none
  Tests: consistency with C1 (Sender adapter, motion filter), C5 (agent wiring, IPC hotkeys), P2 backends' use of SetEdges/Grab semantics incl. any SP3 barrier decisions; decisions D-04, D-05, D-06, D-08, D-49, D-51, D-57, D-58, D-70
  DoD: no open finding

- [ ] [P1-C3-025] C3 review chain green note [FEAT-009] [tier:opus] [owner:PARENT] [brief:briefs/streams/p1-c3.md#item-p1-c3-025]
  Needs: P1-C3-024
  Files: briefs/plan-phase-1.md (append note)
  Tests: none
  DoD: note appended; commit `chore(plan): C3 review chain green [FEAT-009]`

- [ ] [P1-C3-026] Send Ctrl+Alt+Del: hotkey action and engine method for Windows clients (D-113) [FEAT-180] [tier:haiku] [owner:C3] [brief:briefs/streams/p1-c3.md#item-p1-c3-026]
  Needs: P1-C3-010, P1-C3-018, P1-C3-020, P1-F-020
  Files: internal/engine/server_sendcad.go, internal/engine/server_sendcad_test.go, internal/engine/server_hotkeys.go (one dispatch case)
  Tests: TestSendCAD_HotkeySendsChordToActiveWindowsClient, TestSendCAD_ChordKeyOrderAndRelease, TestSendCAD_IgnoredWhenActiveClientIsNotWindows, TestSendCAD_IgnoredWithoutSecureDesktopCapability, TestSendCAD_HotkeyKeysNeverForwarded, TestSendCADTo_JumpsToInactiveClientFirstSkippingGuards, TestSendCADTo_UnknownOrOfflineIsErrorAndEvent, TestSendCAD_RebindingViaSetHotkeysApplies
  DoD: coverage >= 85%; item gates green incl. -race; commit `feat(engine): send Ctrl+Alt+Del action [FEAT-180]`

- [ ] [P1-C3-027] Home shortcuts: register lock and panic with a HomeShortcutProvider capturer and run them on activation (D-114) [FEAT-181, FEAT-015, FEAT-018] [tier:haiku] [owner:C3] [brief:briefs/streams/p1-c3.md#item-p1-c3-027]
  Needs: P1-F-023, P1-C3-018
  Files: internal/engine/home_shortcuts.go, internal/engine/home_shortcuts_test.go
  Tests: TestHomeShortcuts_RegistersLockAndPanicOnly, TestHomeShortcuts_NothingRegisteredWithoutProvider, TestHomeShortcuts_ActivationTogglesLock, TestHomeShortcuts_ActivationPanicReturnsHomeAndReleasesAll, TestHomeShortcuts_SetHotkeysResyncsBindings, TestHomeShortcuts_UnboundBindingNotRegistered, TestHomeShortcuts_StatusExposedForAgent, TestHomeShortcuts_JumpAndCycleNeverRegistered
  DoD: coverage >= 85%; item gates green incl. -race; commit `feat(engine): home shortcuts for capturers without home-side key events [FEAT-181]`

### C4 - Clipboard core and transfer core

- [ ] [P1-C4-001] Clipboard format model, policy toggles, payload caps [FEAT-080, FEAT-087, FEAT-090] [tier:haiku] [owner:C4] [brief:briefs/streams/p1-c4.md#item-p1-c4-001]
  Needs: P1-F-022
  Files: internal/clipboard/doc.go, internal/clipboard/format.go, internal/clipboard/policy.go, internal/clipboard/format_test.go, internal/clipboard/policy_test.go
  Tests: TestFormat_KnownSet, TestFormat_UnknownRejected, TestCap_64MiBPerFormat, TestCap_UnknownSizeAllowedButEnforcedWhileStreaming, TestPolicy_MasterOffFiltersAll, TestPolicy_PerFormatToggles, TestPolicy_FromConfig
  DoD: coverage of internal/clipboard >= 85%; item gates green; commit `feat(clipboard): format model and policy [FEAT-080]`

- [ ] [P1-C4-002] Text normalisation (UTF-8, per-OS line endings) [FEAT-083] [tier:haiku] [owner:C4] [brief:briefs/streams/p1-c4.md#item-p1-c4-002]
  Needs: P1-C4-001
  Files: internal/clipboard/convert/doc.go, internal/clipboard/convert/text.go, internal/clipboard/convert/text_test.go
  Tests: TestTextToCanonical_CRLFAndCR, TestTextToCanonical_StripsTrailingNUL, TestTextToCanonical_InvalidUTF8Error, TestTextToNative_Windows, TestTextToNative_Unix, TestUTF16_RoundTripSurrogates
  DoD: coverage of internal/clipboard/convert >= 85%; item gates green; commit `feat(clipboard): text normalisation [FEAT-083]`

- [ ] [P1-C4-003] CF_HTML build and parse [FEAT-084] [tier:haiku] [owner:C4] [brief:briefs/streams/p1-c4.md#item-p1-c4-003]
  Needs: P1-C4-002
  Files: internal/clipboard/convert/cfhtml.go, internal/clipboard/convert/cfhtml_test.go
  Tests: TestCFHTML_BuildOffsetsMatch, TestCFHTML_ParseFragment, TestCFHTML_RoundTripUnicode, TestCFHTML_ParseRejectsBadOffsets, TestCFHTML_ParseWithoutFragmentMarkers, FuzzParseCFHTML
  DoD: fuzz smoke green; coverage >= 85%; item gates green; commit `feat(clipboard): CF_HTML conversion [FEAT-084]`

- [ ] [P1-C4-004] RTF passthrough validation [FEAT-085] [tier:haiku] [owner:C4] [brief:briefs/streams/p1-c4.md#item-p1-c4-004]
  Needs: P1-C4-002
  Files: internal/clipboard/convert/rtf.go, internal/clipboard/convert/rtf_test.go
  Tests: TestRTF_AcceptsRtfHeader, TestRTF_RejectsNonRTF, TestRTF_StripsTrailingNUL, TestRTF_PreservesBytes
  DoD: coverage >= 85%; item gates green; commit `feat(clipboard): RTF passthrough [FEAT-085]`

- [ ] [P1-C4-005] Image conversion PNG <-> DIB/DIBV5 [FEAT-086] [tier:haiku] [owner:C4] [brief:briefs/streams/p1-c4.md#item-p1-c4-005]
  Needs: P1-C4-002
  Files: internal/clipboard/convert/dib.go, internal/clipboard/convert/dib_test.go
  Tests: TestDIB_PNGRoundTrip32bpp, TestDIB_BottomUpAndTopDown, TestDIB_24bppRGB, TestDIB_Bitfields, TestDIBV5_AlphaPreserved, TestDIB_RejectsHugeDimensionsBeforeAlloc, FuzzDecodeDIB
  DoD: fuzz smoke green; coverage >= 85%; item gates green; commit `feat(clipboard): PNG and DIB conversion [FEAT-086]`

- [ ] [P1-C4-006] Image conversion PNG <-> TIFF [FEAT-086] [tier:haiku] [owner:C4] [brief:briefs/streams/p1-c4.md#item-p1-c4-006]
  Needs: P1-C4-005, D-85 (golang.org/x/image v0.47.0)
  Files: internal/clipboard/convert/tiff.go, internal/clipboard/convert/tiff_test.go, go.mod, go.sum (claim)
  Tests: TestTIFF_PNGRoundTrip, TestTIFF_DecodesLZWAndUncompressed, TestTIFF_RejectsHugeDimensionsBeforeAlloc, FuzzDecodeTIFF
  DoD: fuzz smoke green; coverage >= 85%; item gates green; commit `feat(clipboard): PNG and TIFF conversion [FEAT-086]`

- [ ] [P1-C4-007] Clipboard manager: announce, remote offer, supersede, echo prevention [FEAT-081, FEAT-082, FEAT-088] [tier:sonnet why:lazy clipboard protocol state across peers with supersede and echo prevention] [owner:C4] [brief:briefs/streams/p1-c4.md#item-p1-c4-007]
  Needs: P1-C4-001, P1-C1-002
  Files: internal/clipboard/manager.go, internal/clipboard/announce.go, internal/clipboard/offer.go, internal/clipboard/manager_test.go
  Tests: TestManager_LocalCopyAnnounced, TestManager_NoSensitivityFilter (D-121: every local change is announced subject to the policy), TestManager_NoContentInLogs (slog capture), TestManager_PolicyFiltersFormats, TestManager_RemoteAnnounceOffersLocally, TestManager_OwnOfferNotReAnnounced, TestManager_NewerSeqSupersedes, TestManager_OlderSeqIgnored, TestManager_OriginOwnAnnounceIgnored, TestManager_ConcurrentEvents (race)
  DoD: coverage >= 85%; item gates green incl. -race; commit `feat(clipboard): lazy announce and offer [FEAT-081]`

- [ ] [P1-C4-008] Clipboard fetch and serve: streaming, timeouts, explicit errors, caps [FEAT-081, FEAT-087] [tier:sonnet why:request correlation with timeouts and streaming error propagation] [owner:C4] [brief:briefs/streams/p1-c4.md#item-p1-c4-008]
  Needs: P1-C4-007
  Files: internal/clipboard/fetch.go, internal/clipboard/serve.go, internal/clipboard/fetch_test.go
  Tests: TestFetch_RenderStreamsData, TestFetch_TimeoutSmall10s, TestFetch_TimeoutScalesPerMiB (synctest), TestFetch_SupersededAnnounceFails, TestFetch_RemoteErrorPropagates, TestFetch_NeverReturnsEmptyOnFailure, TestServe_ReadsCurrentLocalContent, TestServe_RejectsStaleSeq, TestServe_EnforcesCap, TestServe_ValidatesCanonicalText, TestFetch_SizeMismatchIsError
  DoD: coverage >= 85%; item gates green incl. -race; commit `feat(clipboard): lazy fetch and serve [FEAT-081]`

- [ ] [P1-C4-009] Clipboard relay through the server between clients [FEAT-089] [tier:haiku] [owner:C4] [brief:briefs/streams/p1-c4.md#item-p1-c4-009]
  Needs: P1-C4-008
  Files: internal/clipboard/relay.go, internal/clipboard/relay_test.go
  Tests: TestRelay_AnnounceFanOutExceptOrigin, TestRelay_FetchPassThroughWithoutBuffering, TestRelay_OriginGoneReturnsError, TestRelay_ServerLocalPasteOfClientContent
  DoD: coverage >= 85%; item gates green incl. -race; commit `feat(clipboard): server relay [FEAT-089]`

- [ ] [P1-C4-010] Clipboard loopback benchmarks (PB-09, PB-10) [FEAT-081] [tier:haiku] [owner:C4] [brief:briefs/streams/p1-c4.md#item-p1-c4-010]
  Needs: P1-C4-009, P1-C1-008
  Files: internal/clipboard/bench_test.go
  Tests: TestClipboardBudget_Text1MiBLoopback (<= 10 ms), BenchmarkFetchText1MiB, BenchmarkFetchPNG10MiB (over real loopback transport)
  DoD: PB-09 loopback met; PB-10 number reported; bench report; item gates green; commit `test(clipboard): loopback fetch benchmarks [FEAT-081]`

- [ ] [P1-C4-011] Path sanitiser safepath (D-45) [FEAT-095] [tier:haiku] [owner:C4] [brief:briefs/streams/p1-c4.md#item-p1-c4-011]
  Needs: P1-F-022
  Files: internal/transfer/doc.go, internal/transfer/safepath/doc.go, internal/transfer/safepath/safepath.go, internal/transfer/safepath/safepath_test.go
  Tests: TestClean_AcceptsNormal, TestClean_RejectsAbsoluteDriveUNC, TestClean_RejectsDotDotAndEmpty, TestClean_RejectsNULAndControl, TestClean_WindowsReservedNames, TestClean_WindowsTrailingDotSpace, TestClean_UnixAllowsReservedNames, TestClean_ComponentOver255Bytes, TestClean_BackslashRejectedForWindowsTarget, TestClean_WindowsForbiddenChars, FuzzClean, TestClean_RejectsWindowsInvalidChars (D-98), TestClean_RejectsSuperscriptDeviceNames (D-98), TestClean_RejectsComponentOver255UTF16Units (D-98)
  DoD: fuzz smoke green; coverage of internal/transfer/safepath >= 85%; item gates green; commit `feat(transfer): path sanitiser [FEAT-095]`

- [ ] [P1-C4-012] os.Root writer: partial files, verify-then-rename, no symlink follow, long paths [FEAT-095] [tier:haiku] [owner:C4] [brief:briefs/streams/p1-c4.md#item-p1-c4-012]
  Needs: P1-C4-011
  Files: internal/transfer/writer.go, internal/transfer/writer_test.go, internal/transfer/writer_windows_test.go
  Tests: TestWriter_PartialNameUntilCommit, TestWriter_CommitRenames, TestWriter_AbortRemovesPartial, TestWriter_RefusesSymlinkEscape, TestWriter_CreatesNestedDirs, TestWriter_DirOnlyEntries, TestWriterWindows_PathLongerThan260 (host)
  DoD: long-path behaviour verified on the host; if os.Root fails for > 260 chars: BLOCKED per D-45; coverage of internal/transfer >= 85%; item gates green; commit `feat(transfer): os.Root confined writer [FEAT-095]`

- [ ] [P1-C4-013] Receive directory and clash renaming [FEAT-096] [tier:haiku] [owner:C4] [brief:briefs/streams/p1-c4.md#item-p1-c4-013]
  Needs: P1-C4-012, P1-C4-006 (adds golang.org/x/text indirectly via x/image, D-85)
  Files: internal/transfer/naming.go, internal/transfer/naming_test.go, go.mod, go.sum (claim; golang.org/x/text v0.43.0 becomes a direct requirement)
  Tests: TestClashName_CaseAndNFCCollisionsRenamed (D-98), TestClashName_BeforeExtension, TestClashName_NoExtension, TestClashName_Dotfile, TestClashName_Folder, TestClashName_Increments, TestReserveName_ConcurrentReservationsUnique, TestReserveName_ExistingOnDiskSkipped
  DoD: coverage >= 85%; item gates green incl. -race; commit `feat(transfer): receive dir clash naming [FEAT-096]`

- [ ] [P1-C4-014] Manifest builder: walk, folders, metadata, symlink skip, bounds, paging [FEAT-094] [tier:haiku] [owner:C4] [brief:briefs/streams/p1-c4.md#item-p1-c4-014]
  Needs: P1-C4-011
  Files: internal/transfer/manifest.go, internal/transfer/manifest_test.go
  Tests: TestManifest_FilesAndFolders, TestManifest_EmptyDirsIncluded, TestManifest_ModTimeAndMode, TestManifest_SymlinksSkippedAndReported, TestManifest_BoundsEnforced, TestManifest_PagesUnderByteLimit, TestManifest_DeterministicOrder
  DoD: coverage >= 85%; item gates green; commit `feat(transfer): manifest builder [FEAT-094]`

- [ ] [P1-C4-015] Sender: 1 MiB chunks with SHA-256 trailers, per-file hash, ChunkRequest serving [FEAT-091] [tier:haiku] [owner:C4] [brief:briefs/streams/p1-c4.md#item-p1-c4-015]
  Needs: P1-C4-014
  Files: internal/transfer/chunk.go, internal/transfer/sender.go, internal/transfer/chunk_test.go, internal/transfer/sender_test.go
  Tests: TestChunk_EncodeDecodeTrailer, TestChunk_CorruptTrailerDetected, TestSender_StreamsWholeFile, TestSender_ServesRangeRequest, TestSender_FileHashComputedWhileReading, TestSender_FileChangedDuringSendFails, TestSender_ZeroLengthFile
  DoD: coverage >= 85%; item gates green; commit `feat(transfer): chunked sender [FEAT-091]`

- [ ] [P1-C4-016] Receiver: verify chunks, write via os.Root, final hash, rename, metadata [FEAT-091, FEAT-094] [tier:haiku] [owner:C4] [brief:briefs/streams/p1-c4.md#item-p1-c4-016]
  Needs: P1-C4-015, P1-C4-013
  Files: internal/transfer/receiver.go, internal/transfer/receiver_test.go
  Tests: TestReceiver_WritesVerifiedFile, TestReceiver_ChunkHashMismatchAborts, TestReceiver_FinalHashMismatchDeletesPartial, TestReceiver_SetsModTimeAndMode, TestReceiver_CreatesEmptyDirs, TestReceiver_ClashRenamesTopLevel, TestReceiver_SanitisesEveryPath
  DoD: coverage >= 85%; item gates green; commit `feat(transfer): verifying receiver [FEAT-091]`

- [ ] [P1-C4-017] Resume from the last verified chunk boundary [FEAT-092] [tier:haiku] [owner:C4] [brief:briefs/streams/p1-c4.md#item-p1-c4-017]
  Needs: P1-C4-016
  Files: internal/transfer/resume.go, internal/transfer/resume_test.go
  Tests: TestResume_TruncatesToChunkBoundary, TestResume_RequestsFromVerifiedOffset, TestResume_FinalHashCoversWholeFile, TestResume_NoPartialStartsAtZero, TestResume_PartialLargerThanFileRestarts
  DoD: coverage >= 85%; item gates green; commit `feat(transfer): resume [FEAT-092]`

- [ ] [P1-C4-018] Transfer manager and queue: offer/accept/chunk protocol, progress, speed, ETA, cancel, retry, 4 streams per peer [FEAT-093, FEAT-091] [tier:sonnet why:protocol orchestration with per-peer stream scheduling, cancellation and retry] [owner:C4] [brief:briefs/streams/p1-c4.md#item-p1-c4-018]
  Needs: P1-C4-017, P1-C1-002
  Files: internal/transfer/manager.go, internal/transfer/queue.go, internal/transfer/manager_test.go, internal/transfer/queue_test.go
  Tests: TestManager_SendReceiveOverInMemoryNetwork, TestManager_OfferPagesAssembled, TestManager_CompleteVerifiesHashesAndCommits, TestManager_ReceiverRejectsOversizedManifest, TestQueue_MaxFourStreamsPerPeer, TestQueue_ProgressEvents, TestQueue_SpeedEWMAAndETA (synctest), TestQueue_CancelStopsStreamsAndKeepsPartial, TestQueue_RetryResumes, TestQueue_FailureState, TestQueue_ListSnapshot, TestManager_Close (leak check)
  DoD: coverage >= 85%; item gates green incl. -race; commit `feat(transfer): transfer manager and queue [FEAT-093]`

- [ ] [P1-C4-019] Transfer relay through the server between clients [FEAT-097] [tier:haiku] [owner:C4] [brief:briefs/streams/p1-c4.md#item-p1-c4-019]
  Needs: P1-C4-018
  Files: internal/transfer/relay.go, internal/transfer/relay_test.go
  Tests: TestRelay_OfferForwarded, TestRelay_ChunkStreamsPassThrough, TestRelay_CancelPropagatesBothWays, TestRelay_NoDiskWrites
  DoD: coverage >= 85%; item gates green incl. -race; commit `feat(transfer): server relay [FEAT-097]`

- [ ] [P1-C4-020] Transfer loopback throughput and scale tests (PB-11 loopback) [FEAT-171] [tier:haiku] [owner:C4] [brief:briefs/streams/p1-c4.md#item-p1-c4-020]
  Needs: P1-C4-019, P1-C1-008
  Files: internal/transfer/bench_test.go, internal/transfer/large_test.go
  Tests: BenchmarkTransferLoopback1GiB (QUIC and TCP), BenchmarkManySmallFiles, TestLarge_OneGiBFileResumes (skipped with -short), TestLarge_TenThousandSmallFiles
  DoD: throughput numbers in the bench report (MiB/s per transport kind); item gates green; commit `test(transfer): loopback throughput and scale [FEAT-171]`

- [ ] [P1-C4-021] C4 code review gate [FEAT-080, FEAT-081, FEAT-091, FEAT-093, FEAT-095] [tier:sonnet why:stream review gate (code-reviewer)] [owner:C4] [brief:briefs/streams/p1-c4.md#item-p1-c4-021]
  Needs: P1-C4-001 .. P1-C4-020
  Files: none (findings become appended fix items)
  Tests: code-reviewer over the C4 merged range
  DoD: VERDICT pass after fixes

- [ ] [P1-C4-022] C4 security review gate (path safety, decode limits, clipboard privacy) [FEAT-173] [tier:sonnet why:security review gate against qa/security-review.md] [owner:C4] [brief:briefs/streams/p1-c4.md#item-p1-c4-022]
  Needs: P1-C4-021
  Files: none (findings become appended fix items)
  Tests: qa/security-review.md sections 5, 6, 7 applied to internal/clipboard, internal/transfer
  DoD: VERDICT pass after fixes

- [ ] [P1-C4-023] C4 Opus consolidation review [FEAT-081] [tier:opus] [owner:PARENT] [brief:briefs/streams/p1-c4.md#item-p1-c4-023]
  Needs: P1-C4-022
  Files: none
  Tests: consistency with C1 data streams, C5 IPC transfer events, P4-F hooks (vfile provider needs range reads from the sender), decisions D-09, D-10, D-17, D-44, D-45, D-57, D-59, D-61, D-62
  DoD: no open finding

- [ ] [P1-C4-024] C4 review chain green note [FEAT-081] [tier:opus] [owner:PARENT] [brief:briefs/streams/p1-c4.md#item-p1-c4-024]
  Needs: P1-C4-023
  Files: briefs/plan-phase-1.md (append note)
  Tests: none
  DoD: note appended; commit `chore(plan): C4 review chain green [FEAT-081]`

### C5 - Agent shell

- [ ] [P1-C5-001] Per-OS filesystem paths (D-69, D-48) [FEAT-143] [tier:haiku] [owner:C5] [brief:briefs/streams/p1-c5.md#item-p1-c5-001]
  Needs: P1-F-022
  Files: internal/paths/doc.go, internal/paths/paths.go, internal/paths/xdg.go, internal/paths/paths_windows.go, internal/paths/paths_darwin.go, internal/paths/paths_linux.go, internal/paths/paths_test.go, internal/paths/xdg_test.go, internal/paths/paths_windows_test.go
  Tests: TestXDG_UserDirsParsing (quoted, $HOME expansion, missing), TestXDG_Defaults, TestPaths_LinuxFromEnv (pure resolver with injected env), TestPaths_DarwinFromHome, TestPathsWindows_KnownFolders (host), TestPaths_ReceiveDirOverride
  DoD: coverage of internal/paths >= 85%; cross builds green; item gates green; commit `feat(paths): per-OS filesystem locations [FEAT-143]`

- [ ] [P1-C5-002] Config store: atomic TOML load/save, strict decoding, migrations, validation [FEAT-134] [tier:haiku] [owner:C5] [brief:briefs/streams/p1-c5.md#item-p1-c5-002]
  Needs: P1-C5-001
  Files: internal/config/store.go, internal/config/migrate.go, internal/config/validate.go, internal/config/store_test.go, internal/config/migrate_test.go, internal/config/validate_test.go, internal/config/fuzz_test.go
  Tests: TestStore_LoadMissingCreatesDefault, TestStore_SaveAtomicTempFsyncRename, TestStore_StrictUnknownKeyError, TestStore_VersionedUpdateConflict, TestMigrate_OrderedFromV0Fixture, TestMigrate_NewerSchemaRejected, TestValidate_PortRoleGuardsLogLevel, TestValidate_DuplicateManualPeers, FuzzDecodeConfig
  DoD: fuzz smoke green; coverage of internal/config >= 85%; item gates green; commit `feat(config): config store and migrations [FEAT-134]`

- [ ] [P1-C5-003] Logging: JSON rotation 10 MiB x 5, ring buffer 2000, redaction, setup [FEAT-135] [tier:haiku] [owner:C5] [brief:briefs/streams/p1-c5.md#item-p1-c5-003]
  Needs: P1-C5-001
  Files: internal/logging/rotate.go, internal/logging/ring.go, internal/logging/redact.go, internal/logging/setup.go, internal/logging/rotate_test.go, internal/logging/ring_test.go, internal/logging/redact_test.go, internal/logging/setup_test.go
  Tests: TestRotate_RollsAt10MiB, TestRotate_KeepsFiveFiles, TestRotate_ReopenAppends, TestRing_KeepsLast2000, TestRing_SinceSeqAndMinLevel, TestRing_Subscribe, TestRedact_DropsPathAttrsAboveDebug, TestRedact_SecretNeverPrinted, TestSetup_LevelFromConfig, TestSetup_DebugBuildAddsStderr
  DoD: coverage of internal/logging >= 85%; item gates green incl. -race; commit `feat(logging): rotation, ring buffer and redaction [FEAT-135]`

- [ ] [P1-C5-004] i18n catalogue, T(), plurals, missing/unused key test [FEAT-136] [tier:haiku] [owner:C5] [brief:briefs/streams/p1-c5.md#item-p1-c5-004]
  Needs: P1-F-022
  Files: internal/i18n/doc.go, internal/i18n/i18n.go, internal/i18n/locales/en.toml, internal/i18n/i18n_test.go, internal/i18n/keys_test.go
  Tests: TestT_NamedPlaceholders, TestT_PluralOneOther, TestT_MissingKeyIsVisibleMarkerAndError, TestCatalogue_ParsesEmbedded, TestKeys_AllUsedKeysExistAndNoneUnused (go/ast scan of the module)
  DoD: coverage of internal/i18n >= 85%; item gates green; commit `feat(i18n): message catalogue [FEAT-136]`

- [ ] [P1-C5-005] IPC endpoint on Windows: named pipe, owner+SYSTEM DACL, first instance, SID checks [FEAT-132] [tier:haiku] [owner:C5] [brief:briefs/streams/p1-c5.md#item-p1-c5-005]
  Needs: P1-F-022, P0-SP5-008
  Files: internal/ipc/doc.go, internal/ipc/endpoint.go, internal/ipc/endpoint_windows.go, internal/ipc/endpoint_windows_test.go, go.mod, go.sum (claim; adds github.com/Microsoft/go-winio v0.6.4)
  Tests (host, local pipes): TestPipe_NameContainsUserSID, TestPipe_SDDLOwnerAndSystemOnly, TestPipe_SecondServerFails, TestPipe_ClientVerifiesServerSID, TestPipe_ServerVerifiesClientSID, TestPipe_RoundTrip
  DoD: coverage of internal/ipc >= 85% on the host; item gates green; commit `feat(ipc): windows named pipe endpoint [FEAT-132]`

- [ ] [P1-C5-006] IPC endpoint on macOS/Linux: 0700 dir, 0600 socket, peer UID check [FEAT-132] [tier:haiku] [owner:C5] [brief:briefs/streams/p1-c5.md#item-p1-c5-006]
  Needs: P1-C5-005
  Files: internal/ipc/endpoint_unix.go, internal/ipc/endpoint_linux.go, internal/ipc/endpoint_darwin.go, internal/ipc/endpoint_unix_test.go
  Tests (unix tag, run on CI): TestSocket_DirAndFileModes, TestSocket_PeerUIDMatches, TestSocket_StaleSocketReplacedOnlyIfDead, TestSocket_SecondServerFails, TestSocket_RoundTrip
  DoD: cross builds and `GOOS=linux`/`GOOS=darwin` lint green; item gates green; commit `feat(ipc): unix socket endpoint [FEAT-132]`

- [ ] [P1-C5-007] IPC session token and framing limits [FEAT-132] [tier:haiku] [owner:C5] [brief:briefs/streams/p1-c5.md#item-p1-c5-007]
  Needs: P1-C5-006
  Files: internal/ipc/token.go, internal/ipc/framing.go, internal/ipc/token_test.go, internal/ipc/framing_test.go
  Tests: TestToken_32RandomBytesOwnerOnlyFile, TestToken_RotatedPerStart, TestToken_ConstantTimeCompare, TestFraming_RejectsOver16MiBBeforeAlloc, TestFraming_RoundTrip, TestFraming_TruncatedIsError, FuzzReadFrame
  DoD: coverage >= 85%; item gates green; commit `feat(ipc): session token and framing [FEAT-132]`

- [ ] [P1-C5-008] IPC server: Hello auth, request dispatch, event broadcast, slow consumers [FEAT-133] [tier:sonnet why:request/response multiplexing with server-push events and slow-consumer handling] [owner:C5] [brief:briefs/streams/p1-c5.md#item-p1-c5-008]
  Needs: P1-C5-007
  Files: internal/ipc/server.go, internal/ipc/server_test.go
  Tests: TestServer_RejectsWrongToken, TestServer_RequiresHelloFirst, TestServer_DispatchesConcurrentRequests, TestServer_ResponseIDsMatch, TestServer_HandlerErrorBecomesErrorInfo, TestServer_BroadcastEventsToAllClients, TestServer_SlowClientDisconnected, TestServer_ShutdownClosesClients (leak check)
  DoD: coverage >= 85%; item gates green incl. -race; commit `feat(ipc): IPC server [FEAT-133]`

- [ ] [P1-C5-009] IPC client: typed methods for every request, event channel [FEAT-133] [tier:haiku] [owner:C5] [brief:briefs/streams/p1-c5.md#item-p1-c5-009]
  Needs: P1-C5-008
  Files: internal/ipc/client.go, internal/ipc/client_test.go
  Tests: TestClient_EveryMethodRoundTrip (table over every request against a recording handler), TestClient_ErrorInfoMappedToTypedError, TestClient_EventsDelivered, TestClient_ContextCancelAbandonsRequest, TestClient_CloseUnblocksPending
  DoD: coverage >= 85%; item gates green incl. -race; commit `feat(ipc): IPC client [FEAT-133]`

- [ ] [P1-C5-010] Single instance and GUI focus hand-off [FEAT-141] [tier:haiku] [owner:C5] [brief:briefs/streams/p1-c5.md#item-p1-c5-010]
  Needs: P1-C5-009
  Files: internal/ipc/instance.go, internal/ipc/instance_test.go
  Tests: TestInstance_SecondAgentDetectsRunning, TestInstance_FirstGUIRegistered, TestInstance_SecondGUIRejectedAndFocusEventSentToFirst, TestInstance_GUISlotFreedOnDisconnect, TestInstance_StaleEndpointRecovered
  DoD: coverage >= 85%; item gates green; commit `feat(ipc): single instance and focus hand-off [FEAT-141]`

- [ ] [P1-C5-011] IPC round-trip benchmark (PB-24) [FEAT-132] [tier:haiku] [owner:C5] [brief:briefs/streams/p1-c5.md#item-p1-c5-011]
  Needs: P1-C5-010
  Files: internal/ipc/bench_test.go
  Tests: BenchmarkGetStatusRoundTrip, TestIPCBudget_GetStatusP99 (<= 2 ms over 10 000 calls on host pipes)
  DoD: PB-24 met; bench report; item gates green; commit `test(ipc): round-trip benchmark [FEAT-132]`

- [ ] [P1-C5-012] Update checker: GitHub latest release, ETag, semver, schedule, toggle [FEAT-137] [tier:haiku] [owner:C5] [brief:briefs/streams/p1-c5.md#item-p1-c5-012]
  Needs: P1-C5-001
  Files: internal/update/doc.go, internal/update/update.go, internal/update/semver.go, internal/update/update_test.go, internal/update/semver_test.go
  Tests: TestSemver_CompareAndPrerelease, TestSemver_RejectsInvalid, TestCheck_NewerAvailable, TestCheck_SameVersion, TestCheck_PrereleaseIgnored, TestCheck_ETagNotModified, TestCheck_ResponseSizeCapped, TestCheck_HTTPErrorReported, TestSchedule_FirstAfter5MinThenDaily (synctest), TestSchedule_DisabledNoCalls
  DoD: tests use httptest TLS server on loopback only; coverage >= 85%; item gates green; commit `feat(update): release update checker [FEAT-137]`

- [ ] [P1-C5-013] Lock/screensaver sync logic (D-71) [FEAT-138] [tier:haiku] [owner:C5] [brief:briefs/streams/p1-c5.md#item-p1-c5-013]
  Needs: P1-F-022
  Files: internal/locksync/doc.go, internal/locksync/locksync.go, internal/locksync/locksync_test.go
  Tests: TestServerLockLocksClients, TestServerScreensaverStartsClientScreensavers, TestServerUnlockOnlyWakesDisplays, TestTogglesIndividually, TestClientEventsNeverPropagate, TestClientAppliesLockSyncMessages, TestRoleChangeStopsPropagation
  DoD: coverage >= 85%; item gates green; commit `feat(locksync): lock and screensaver sync [FEAT-138]`

- [ ] [P1-C5-014] Diagnostics bundle (logs, redacted config, system info, permissions) [FEAT-139] [tier:haiku] [owner:C5] [brief:briefs/streams/p1-c5.md#item-p1-c5-014]
  Needs: P1-C5-003, P1-C5-002
  Files: internal/diag/doc.go, internal/diag/bundle.go, internal/diag/sysinfo.go, internal/diag/sysinfo_windows.go, internal/diag/sysinfo_darwin.go, internal/diag/sysinfo_linux.go, internal/diag/bundle_test.go
  Tests: TestBundle_ContainsExpectedEntries, TestBundle_NeverContainsKeysOrTokens, TestBundle_ConfigPeerIDsShortened, TestBundle_LogsCopiedNotMoved, TestBundle_AtomicZipWrite, TestSysinfo_Fields
  DoD: coverage >= 85%; item gates green; commit `feat(diag): diagnostics bundle [FEAT-139]`

- [ ] [P1-C5-015] Autostart reconcile with the startup setting [FEAT-140] [tier:haiku] [owner:C5] [brief:briefs/streams/p1-c5.md#item-p1-c5-015]
  Needs: P1-C5-002
  Files: internal/agent/doc.go, internal/agent/autostart.go, internal/agent/autostart_test.go
  Tests: TestAutostart_EnablesWhenConfigured, TestAutostart_DisablesWhenNot, TestAutostart_NoChangeWhenInSync, TestAutostart_ErrorSurfaced
  DoD: coverage of internal/agent >= 85%; item gates green; commit `feat(agent): autostart reconcile [FEAT-140]`

- [ ] [P1-C5-016] Tray model and action dispatch [FEAT-144] [tier:haiku] [owner:C5] [brief:briefs/streams/p1-c5.md#item-p1-c5-016]
  Needs: P1-C5-015, P1-C5-004
  Files: internal/agent/tray.go, internal/agent/tray_test.go, internal/i18n/locales/en.toml (claim; tray keys)
  Tests: TestTrayModel_StatesAndTooltip, TestTrayModel_JumpAndWakeSubmenus, TestTrayModel_UpdateItemOnlyWhenAvailable, TestTrayModel_LabelsTranslated, TestTrayAction_DispatchesEveryID, TestTrayAction_UnknownIDLogged, TestTrayAction_OpenLaunchesGUIViaLauncher
  DoD: coverage >= 85%; item gates green; commit `feat(agent): tray model and actions [FEAT-144]`

- [ ] [P1-C5-017] Agent composition root, lifecycle and agent.Main on fakes [FEAT-142, FEAT-001] [tier:sonnet why:composition root lifecycle ordering and graceful shutdown across all subsystems] [owner:C5] [brief:briefs/streams/p1-c5.md#item-p1-c5-017]
  Needs: P1-C5-016, P1-C5-013, P1-C5-014, P1-C5-012, P1-C5-010, P1-C1-016, P1-C2-012, P1-C2-013, P1-C3-021, P1-C4-009, P1-C4-018
  Files: internal/agent/agent.go, internal/agent/lifecycle.go, internal/agent/main.go, internal/agent/agent_test.go, internal/agent/main_test.go
  Tests: TestMain_FlagsAndExitCodes, TestMain_SignalCancelsAndShutsDown, TestMain_SecondInstanceExits, TestAgent_StartsSubsystemsInOrder, TestAgent_ShutdownReverseOrderWithinDeadline, TestAgent_StartFailureUnwindsStarted, TestAgent_ThemeChangesPublished, TestAgent_NoGoroutineLeak
  DoD: no `cmd/edgesync-agent` changes in this item (see brief); coverage >= 85%; item gates green incl. -race; commit `feat(agent): composition root and lifecycle [FEAT-142]`

- [ ] [P1-C5-018] Role switching without restart [FEAT-001] [tier:haiku] [owner:C5] [brief:briefs/streams/p1-c5.md#item-p1-c5-018]
  Needs: P1-C5-017
  Files: internal/agent/role.go, internal/agent/role_test.go
  Tests: TestRole_ServerToClientStopsCaptureStartsClientEngine, TestRole_ClientToServerStartsCaptureAndDials, TestRole_PersistedInConfig, TestRole_SameRoleNoop, TestRole_SwitchWhileRemoteReturnsHomeFirst
  DoD: coverage >= 85%; item gates green incl. -race; commit `feat(agent): role switching [FEAT-001]`

- [ ] [P1-C5-019] IPC handler group A: status, config, layout, role, autostart, permissions, hotkeys, capture, quit [FEAT-133] [tier:haiku] [owner:C5] [brief:briefs/streams/p1-c5.md#item-p1-c5-019]
  Needs: P1-C5-018
  Files: internal/agent/ipc_handler.go, internal/agent/ipc_status.go, internal/agent/ipc_handler_test.go
  Tests: one subtest per request in the group (success and error path), TestHandler_UnknownRequestIsInvalidArgument, TestHandler_StatusEventsPublished
  DoD: coverage >= 85%; item gates green; commit `feat(agent): IPC status, config and layout handlers [FEAT-133]`

- [ ] [P1-C5-020] IPC handler group B: peers, discovery, manual peers, pairing, connect, wake [FEAT-133] [tier:haiku] [owner:C5] [brief:briefs/streams/p1-c5.md#item-p1-c5-020]
  Needs: P1-C5-019, P1-C2-010
  Files: internal/agent/ipc_peers.go, internal/agent/ipc_peers_test.go, internal/i18n/locales/en.toml (claim; pairing notification keys)
  Tests: one subtest per request in the group, TestPairingProgressEvents, TestDiscoveryChangedEvents
  DoD: coverage >= 85%; item gates green; commit `feat(agent): IPC peer and pairing handlers [FEAT-133]`

- [ ] [P1-C5-021] IPC handler group C: transfers, receive folder, logs, diagnostics, lock, jump, home, updates [FEAT-133] [tier:haiku] [owner:C5] [brief:briefs/streams/p1-c5.md#item-p1-c5-021]
  Needs: P1-C5-020
  Files: internal/agent/ipc_transfers.go, internal/agent/ipc_transfers_test.go
  Tests: one subtest per request in the group, TestTransferEventsForwarded, TestLogRecordEventsForwarded
  DoD: coverage >= 85%; item gates green; commit `feat(agent): IPC transfer, log and control handlers [FEAT-133]`

- [ ] [P1-C5-022] Agent smoke tests on fakes over real IPC and loopback transport [FEAT-142] [tier:haiku] [owner:C5] [brief:briefs/streams/p1-c5.md#item-p1-c5-022]
  Needs: P1-C5-021
  Files: internal/agent/smoke_test.go
  Tests: TestSmoke_HelloStatusQuit, TestSmoke_TwoAgentsPairConnectAndSwitch (fake backends, loopback QUIC), TestSmoke_ClipboardTextAcrossAgents, TestSmoke_SendFileAcrossAgents
  DoD: item gates green incl. -race; commit `test(agent): agent smoke tests on fakes [FEAT-142]`

- [ ] [P1-C5-023] C5 code review gate [FEAT-132, FEAT-133, FEAT-134, FEAT-135, FEAT-142] [tier:sonnet why:stream review gate (code-reviewer)] [owner:C5] [brief:briefs/streams/p1-c5.md#item-p1-c5-023]
  Needs: P1-C5-001 .. P1-C5-022, P1-C5-027
  Files: none (findings become appended fix items)
  Tests: code-reviewer over the C5 merged range
  DoD: VERDICT pass after fixes

- [ ] [P1-C5-024] C5 security review gate (IPC auth, logging privacy, diagnostics, update check) [FEAT-173] [tier:sonnet why:security review gate against qa/security-review.md] [owner:C5] [brief:briefs/streams/p1-c5.md#item-p1-c5-024]
  Needs: P1-C5-023
  Files: none (findings become appended fix items)
  Tests: qa/security-review.md sections 3, 7, 8 applied to internal/ipc, internal/logging, internal/diag, internal/update, internal/agent
  DoD: VERDICT pass after fixes

- [ ] [P1-C5-025] C5 Opus consolidation review [FEAT-142] [tier:opus] [owner:PARENT] [brief:briefs/streams/p1-c5.md#item-p1-c5-025]
  Needs: P1-C5-024
  Files: none
  Tests: composition matches every C1-C4 API; IPC surface matches architecture section 7; decisions D-19, D-21, D-31, D-46, D-48, D-68, D-69, D-71, D-72, D-77
  DoD: no open finding

- [ ] [P1-C5-026] C5 review chain green note [FEAT-142] [tier:opus] [owner:PARENT] [brief:briefs/streams/p1-c5.md#item-p1-c5-026]
  Needs: P1-C5-025
  Files: briefs/plan-phase-1.md (append note)
  Tests: none
  DoD: note appended; commit `chore(plan): C5 review chain green [FEAT-142]`

- [ ] [P1-C5-027] Tray: "Send Ctrl+Alt+Del to <machine>" for connected Windows clients (D-113) [FEAT-180, FEAT-144] [tier:haiku] [owner:C5] [brief:briefs/streams/p1-c5.md#item-p1-c5-027]
  Needs: P1-C5-016, P1-C3-026
  Files: internal/agent/tray.go, internal/agent/tray_test.go, internal/i18n/locales/en.toml (claim; tray keys)
  Tests: TestTrayModel_SendCADSubmenuOnlyForWindowsClientsWithCapability, TestTrayModel_SendCADHiddenInClientRole, TestTrayModel_SendCADLabelsTranslated, TestTrayAction_SendCADDispatchesToEngine, TestTrayAction_SendCADErrorLoggedAndNotified
  DoD: coverage >= 85%; item gates green; commit `feat(agent): tray action Send Ctrl+Alt+Del [FEAT-180]`

### P1-X - Phase exit

- [ ] [P1-X-M01] Push main and share the CI run (test matrix on Windows, macOS, Ubuntu; lint, cross, gen-check, vuln) [FEAT-167, FEAT-168] [tier:haiku] [owner:USER] [brief:briefs/streams/p1-c5.md#item-p1-x-m01]
  Needs: P1-C1-020, P1-C2-018, P1-C3-025, P1-C4-024, P1-C5-026
  Files: qa/manual/results/p1-x-ci.md
  Tests: run URL; every job green, in particular the unix-only tests of internal/pki, internal/ipc, internal/paths on macOS and Ubuntu
  DoD: results committed by the parent `docs(qa): P1 CI results [FEAT-167]`; any red job becomes an appended fix item

- [ ] [P1-X-001] P1 exit criteria and phase marker [FEAT-167] [tier:opus] [owner:PARENT] [brief:briefs/streams/p1-c5.md#item-p1-x-001]
  Needs: P1-X-M01, P1-C2-M01, P1-F-M01, every other P1 item
  Files: plan.md (append marker note), CLAUDE.md (mirror marker), perf-budgets.md (results rows already appended)
  Tests: plan.md P1 exit list checked: CONTRACT FREEZE note present; every P1 package >= 85% (`task cover`); fuzz smoke green on every decoder; PB-03, PB-04, PB-05, PB-06, PB-09, PB-14, PB-22, PB-23, PB-24 rows recorded and passing; PB-02/PB-11 loopback numbers recorded; review chains green for C1-C5
  DoD: `task ci:local` green on main; marker note appended listing every still-active phase (for example `Current phase: P2` or `Current phase: P2, P3`); commit `chore(plan): P1 exit [FEAT-167]`
