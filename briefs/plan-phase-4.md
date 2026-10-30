# EdgeSync - Phase 4 items

Read `plan.md` (contract, orchestration, item format) first. Stream briefs: `briefs/streams/p4-*.md`.
Phase scope: native lazy file copy-paste and cross-screen drag-and-drop on every OS, plus native file drop onto
the GUI (D-11, D-12, D-13, D-17, D-62). The foundation block P4-F runs first with one agent, in order, and ends
with the FILE CONTRACT FREEZE. Then F1-F5 fan out to N = 5.

Spike findings referenced in `Needs:` lines live in `briefs/spikes/sp2.md`, `sp3.md`, `sp4.md` (written in P0).
They are cited by the summary-table row names that the P0 briefs mandate, for example
`sp2.md row "virtual-file paste" = GO`. A row that is not GO, or a P0-X-002 decision that changed the mechanism,
blocks the item. The parent then asks the user (THE PLAN IS SACRED, D-13) and appends replacement items.

Mechanism checkpoints: F2-001, F3-001, F4-001 and F5-001 are USER approvals of the per-OS mechanism, given
before any implementation item of that stream starts. Each stream brief describes a planned mechanism
(variant A) and a fallback (variant B) with ready-to-append items. When the user approves variant B, the parent
appends the variant-B items at the end of the stream section. The superseded variant-A items each get a
`<!-- NOTE: <date> - superseded by D-NN -->` and stay unchecked. P4-X-001 counts them as closed only when they
carry such a note.

Standard gates for every implementation item ("per-item gates"): plan.md 1.5. That includes both
`CGO_ENABLED=0` cross-compiles of `./internal/... ./cmd/edgesync-agent/...` for linux/amd64 and darwin/arm64.
GUI files under `gui/drop/` that need cgo on macOS/Linux are built on the matching CI runner (F3-011, F4-012,
F5-011).

## Phase 4 - native files & DnD

### P4-F - Foundation block (sequential, 1 agent, in order; ends with FILE CONTRACT FREEZE)

- [ ] [P4-F-001] Pre-flight: reconcile P0 spike findings, P0-X-002 decisions and the delivered P1-P3 code with the P4 design; list contract/schema deltas, decision proposals and per-item path notes [FEAT-099] [tier:opus] [owner:PARENT] [brief:briefs/streams/p4-f.md#item-p4-f-001]
  Needs: P0-X-003; P1-X-001; P2-W1-029, P2-W2-035, P2-X1-025, P2-Y1-025, P2-M1-027, P3-G5-R03
  Files: briefs/p4-preflight.md (new), briefs/plan-phase-4.md (append `<!-- NOTE -->` path and COM-inventory notes under affected items)
  Tests: covers checklist points 1-9 of the brief. For each listed spike row the verdict and its consequence for named P4 items is stated: sp2.md rows "virtual-file paste", "Explorer progress/cancel", "blocking behaviour", "edge drop target", "drag source", "UIPI", "Gio drop"; sp3.md rows "lazy file paste mechanism recommendation for X11 and Wayland", "XDND", "Wayland DnD limits", "Gio Linux drop", "XI2 generation"; sp4.md rows "lazy provider", "file promises (verifiable vs not)"
  DoD: every proposal that needs the user is listed with options and a recommendation for P4-F-002 (only new ones: the planning consolidation already closed manifest limits, virtual-file names and read-ahead (D-99), the gui/drop allowances (D-100), the file contract source (D-101), the P2 forward requirements (D-102), the Wayland client scope (D-103), PB-25 (D-109) and the macOS pasteboard rule (D-86)); the exact paths of P4-F-016, P4-F2-012 and P4-F4-007 are already in their `Files:` lines, the preflight confirms them against `main` and records the backend type names; commit `docs(plan): P4 pre-flight [FEAT-099]`

- [ ] [P4-F-002] User approves the remaining P4 pre-flight proposals (spike-driven contract and schema deltas) and the parent applies the D-100 depguard change [FEAT-099] [tier:haiku] [owner:USER] [brief:briefs/streams/p4-f.md#item-p4-f-002]
  Needs: P4-F-001
  Files: decisions.md, ownership.md, .golangci.yml (claim; the D-100 depguard allowance, `golangci-lint config verify` green)
  Tests: every new proposal in briefs/p4-preflight.md has a recorded answer (none may reopen D-86, D-99 .. D-103 or D-109 without a superseding decision); PB-25 is already in perf-budgets.md (D-109)
  DoD: commit `docs(decisions): P4 pre-flight decisions [FEAT-099]`

- [ ] [P4-F-003] vfile manifest types and validation [FEAT-099, FEAT-100] [tier:haiku] [owner:P4-F] [brief:briefs/streams/p4-f.md#item-p4-f-003]
  Needs: P4-F-002
  Files: internal/vfile/doc.go, internal/vfile/vfile.go, internal/vfile/validate.go, internal/vfile/validate_test.go, internal/vfile/validate_fuzz_test.go
  Tests: TestValidate_AcceptsWellFormedTree, TestValidate_RejectsUnsafePaths (table: every D-45 rule plus backslash separators and "." components), TestValidate_RejectsDuplicatePathsCaseInsensitive, TestValidate_ParentMustPrecedeChild, TestValidate_RejectsTooManyEntries, TestValidate_RejectsPathTooLong, TestValidate_RejectsNegativeSizeAndSizedDirectory, TestValidate_RejectsBadID, TestValidate_RejectsEmptyManifest, TestTotalBytes, TestTopLevel, FuzzValidate (30 s)
  DoD: coverage of internal/vfile >= 85%; per-item gates green; commit `feat(vfile): manifest types and validation [FEAT-099]`

- [ ] [P4-F-004] vfile URI list and GNOME copied-files codecs [FEAT-100, FEAT-110, FEAT-113] [tier:haiku] [owner:P4-F] [brief:briefs/streams/p4-f.md#item-p4-f-004]
  Needs: P4-F-003
  Files: internal/vfile/uri.go, internal/vfile/uri_test.go, internal/vfile/uri_fuzz_test.go
  Tests: TestEncodeURIList_PercentEncodesReservedAndNonASCII, TestEncodeURIList_CRLFTerminatedLines, TestEncodeGnomeCopiedFiles_AlwaysCopyVerb, TestDecodeURIList_FileSchemeOnly, TestDecodeURIList_LocalhostHostAccepted, TestDecodeURIList_RemoteHostRejected, TestDecodeURIList_CommentsAndBlankLinesSkipped, TestDecodeURIList_InvalidPercentEscapeRejected, TestDecodeURIList_NULRejected, TestDecodeGnomeCopiedFiles_CopyAndCutBothYieldPaths, TestRoundTrip_UnicodeAndSpaces, FuzzDecodeURIList (30 s)
  DoD: coverage of internal/vfile >= 85%; per-item gates green; commit `feat(vfile): uri-list and gnome-copied-files codecs [FEAT-100]`

- [ ] [P4-F-005] vfile manifest builder from local paths [FEAT-100, FEAT-102] [tier:haiku] [owner:P4-F] [brief:briefs/streams/p4-f.md#item-p4-f-005]
  Needs: P4-F-003
  Files: internal/vfile/build.go, internal/vfile/build_test.go, internal/vfile/build_unix_test.go
  Tests: TestBuild_SingleFile, TestBuild_FolderTreeParentsFirstLexical, TestBuild_EmptyDirectoriesIncluded, TestBuild_SymlinksSkippedAndReported (skips with an explicit reason when the OS refuses symlink creation), TestBuild_SpecialFilesSkipped (unix, fifo), TestBuild_DuplicateTopLevelNamesRejected, TestBuild_LimitsStopWalkEarly, TestBuild_ContextCancelStopsWalk, TestBuild_ModTimeAndModePreserved, TestBuild_IDIsRandom128BitHex, TestBuild_PathsAlignWithEntries
  DoD: coverage of internal/vfile >= 85%; per-item gates green; commit `feat(vfile): manifest builder [FEAT-100]`

- [ ] [P4-F-006] vfile read-ahead reader over a Provider [FEAT-100] [tier:haiku] [owner:P4-F] [brief:briefs/streams/p4-f.md#item-p4-f-006]
  Needs: P4-F-003
  Files: internal/vfile/readahead.go, internal/vfile/readahead_test.go
  Tests (synctest where timing matters): TestReadAhead_SequentialMatchesSourceHash, TestReadAhead_SeekWithinWindowNoReopen, TestReadAhead_SeekOutsideWindowReopensAtOffset, TestReadAhead_SeekFromEnd, TestReadAhead_ProviderErrorSurfacesOnRead, TestReadAhead_ReadFullUntilEOF, TestReadAhead_CloseStopsWorker, TestReadAhead_MemoryBoundedByWindow, TestReadAhead_ContextCancelUnblocksRead
  DoD: race test green; coverage of internal/vfile >= 85%; per-item gates green; commit `feat(vfile): bounded read-ahead reader [FEAT-100]`

- [ ] [P4-F-007] Platform file contract: FormatFiles, FileClipboard, DragDrop, IncomingDrag, source interfaces [FEAT-099] [tier:haiku] [owner:P4-F] [brief:briefs/streams/p4-f.md#item-p4-f-007]
  Needs: P4-F-003
  Files: internal/platform/files.go, internal/platform/files_test.go
  Tests: TestDragOutKind_String, TestFormatFiles_IsCanonical, compile-time assertions that internal/platform does not import internal/transfer (go list check in test)
  DoD: signatures exactly as in the brief; per-item gates green; commit `feat(platform): file clipboard and drag-and-drop contract [FEAT-099]`

- [ ] [P4-F-008] Fakes for FileClipboard and DragDrop [FEAT-099] [tier:haiku] [owner:P4-F] [brief:briefs/streams/p4-f.md#item-p4-f-008]
  Needs: P4-F-007
  Files: internal/platform/fake/files.go, internal/platform/fake/files_test.go
  Tests: TestFakeFileClipboard_SimulateForeignCopy, TestFakeFileClipboard_OfferReplacesAndAbandons, TestFakeFileClipboard_PasteReadsThroughProvider, TestFakeDragDrop_EnterLeaveDropScript, TestFakeDragDrop_IncomingReleaseCancel, TestFakeBackend_ImplementsSources
  DoD: deterministic under synctest; coverage of internal/platform/fake >= 85%; per-item gates green; commit `test(platform): file clipboard and drag-drop fakes [FEAT-099]`

- [ ] [P4-F-009] Conformance suites for FileClipboard and DragDrop, run against the fakes [FEAT-099] [tier:sonnet why:contract suites for asynchronous file and drag semantics that four OS backends must pass] [owner:P4-F] [brief:briefs/streams/p4-f.md#item-p4-f-009]
  Needs: P4-F-008
  Files: internal/platform/conformance/files.go, internal/platform/conformance/files_test.go
  Tests: suites RunFileClipboard and RunDragDrop with the case lists in the brief, all passing against fakes
  DoD: Driver interfaces documented in code; per-item gates green; commit `test(platform): file clipboard and drag-drop conformance suites [FEAT-099]`

- [ ] [P4-F-010] Staging for fetch-on-request mechanisms (Stager, sweep, free-space check) [FEAT-101] [tier:haiku] [owner:P4-F] [brief:briefs/streams/p4-f.md#item-p4-f-010]
  Needs: P4-F-005, P4-F-006
  Files: internal/transfer/staging.go, internal/transfer/staging_unix.go, internal/transfer/staging_windows.go, internal/transfer/staging_test.go, internal/transfer/staging_unix_test.go
  Tests: TestStager_MaterializesTreeAndVerifiesHashes, TestStager_ReturnsTopLevelAbsolutePaths, TestStager_SingleFlightPerManifest, TestStager_SecondCallReturnsCompleted, TestStager_CancelRemovesPartials, TestStager_InsufficientSpaceExplicitError, TestStager_PreExistingSymlinkNotFollowed, TestStager_UnsafeManifestRejectedBeforeIO, TestStager_SweepRemovesOlderThan24h (synctest), TestStager_SweepKeepsActive, TestStager_PermissionsOwnerOnly (unix)
  DoD: coverage of internal/transfer >= 85%; per-item gates green; commit `feat(transfer): staging for fetch-on-request file paste [FEAT-101]`

- [ ] [P4-F-011] Source-side offers registry serving chunk requests for announced manifests [FEAT-100, FEAT-102] [tier:sonnet why:offer lifetime across supersede, in-flight streams and relay] [owner:P4-F] [brief:briefs/streams/p4-f.md#item-p4-f-011]
  Needs: P4-F-005
  Files: internal/transfer/offers.go, internal/transfer/offers_test.go
  Tests: TestOffers_ServesChunkOfRegisteredEntry, TestOffers_UnknownManifestRejected, TestOffers_IndexOutOfRange, TestOffers_DirectoryIndexRejected, TestOffers_SourceChangedSizeOrMtime, TestOffers_RetiredRejectsNewOpensKeepsInflight, TestOffers_OnlyRegisteredPathsReadable, TestOffers_ConcurrentStreamsCappedAt4, TestOffers_NeverDeletesSources, TestOffers_CloseReleasesHandles
  DoD: race test green; coverage of internal/transfer >= 85%; per-item gates green; commit `feat(transfer): source-side virtual-file offers [FEAT-100]`

- [ ] [P4-F-012] RemoteProvider (vfile.Provider over a session) with queue integration, resume and stats [FEAT-100] [tier:sonnet why:request/stream lifecycle with cancellation, resume after reconnect and relay] [owner:P4-F] [brief:briefs/streams/p4-f.md#item-p4-f-012]
  Needs: P4-F-006, P4-F-011
  Files: internal/transfer/remoteprovider.go, internal/transfer/remoteprovider_test.go, internal/transfer/queue_vfile.go, internal/transfer/queue_vfile_test.go
  Tests: TestRemoteProvider_OpenStreamsVerifiedChunks, TestRemoteProvider_HashMismatchFailsWithoutDeliveringBytes, TestRemoteProvider_ResumeAfterReconnectAtOffset, TestRemoteProvider_PeerOfflineError, TestRemoteProvider_QueueCancelCancelsOpensAndReportsDoneOnce, TestRemoteProvider_RelayThroughServer (3-node fake), TestRemoteProvider_DoneIdempotent, TestQueueVFile_ProgressAggregatesFiles, TestQueueVFile_StatsTTFBBytesDuration, TestQueueVFile_LogsWithoutFileNamesAtInfo
  DoD: race test green; coverage of internal/transfer >= 85%; per-item gates green; commit `feat(transfer): remote provider for lazy file paste [FEAT-100]`

- [ ] [P4-F-013] Clipboard file sync: announce local file copies, offer remote ones, supersede, echo prevention, toggles, cut-as-copy [FEAT-100, FEAT-102] [tier:haiku] [owner:P4-F] [brief:briefs/streams/p4-f.md#item-p4-f-013]
  Needs: P4-F-008, P4-F-011, P4-F-012
  Files: internal/clipboard/files.go, internal/clipboard/files_test.go
  Tests: TestFileSync_LocalCopyAnnouncesFilesOnly, TestFileSync_SameSeqWatchChangeIgnored, TestFileSync_RemoteAnnounceOffersFiles, TestFileSync_NewerAnnounceSupersedesAndRetires, TestFileSync_OwnOfferNotReannounced, TestFileSync_FilesToggleOff, TestFileSync_MasterToggleOff, TestFileSync_NeverDeletesSources, TestFileSync_NotRepresentableRaisesNotification, TestFileSync_BuildLimitExceededRaisesNotification
  DoD: coverage of internal/clipboard >= 85%; per-item gates green; commit `feat(clipboard): lazy file copy-paste sync [FEAT-100]`

- [ ] [P4-F-014] dnd protocol codec with decode limits [FEAT-116] [tier:haiku] [owner:P4-F] [brief:briefs/streams/p4-f.md#item-p4-f-014]
  Needs: P4-F-003
  Files: internal/dnd/doc.go, internal/dnd/proto.go, internal/dnd/proto_test.go, internal/dnd/proto_fuzz_test.go; plus proto/schema/dnd.capnp and the regenerated internal/proto/capnp/ files ONLY when P4-F-002 approved a dnd schema delta (claim)
  Tests: TestRoundTrip_DragBegin, TestRoundTrip_DragDrop, TestRoundTrip_DragCancel, TestDecode_ManifestValidated, TestDecode_TraversalAndDepthLimits, TestDecode_OversizedRejectedBeforeDecode, FuzzDecodeDragBegin (30 s)
  DoD: coverage of internal/dnd >= 85%; `task gen:check` green; per-item gates green; commit `feat(dnd): protocol codec [FEAT-116]`

- [ ] [P4-F-015] dnd Coordinator state machine (origin, target, server relay, release, cancel, timeouts) [FEAT-116] [tier:sonnet why:cross-machine drag state machine with races, timeouts and peer loss] [owner:P4-F] [brief:briefs/streams/p4-f.md#item-p4-f-015]
  Needs: P4-F-008, P4-F-012, P4-F-014
  Files: internal/dnd/coordinator.go, internal/dnd/coordinator_test.go
  Tests (synctest): TestCoordinator_ServerToClientDrop, TestCoordinator_ClientToServerDrop, TestCoordinator_ClientToClientViaServer, TestCoordinator_EscapeCancels, TestCoordinator_ReturnToOriginCancelsRemote, TestCoordinator_CrossToThirdMachineMovesDrag, TestCoordinator_ReleaseBeforeIncomingStarted, TestCoordinator_BeginIncomingTimeout, TestCoordinator_LocalDropAtEdgeDeclined, TestCoordinator_UnsupportedSourceNotifies, TestCoordinator_BackendUnsupportedNotifiesOnce, TestCoordinator_PeerLostCancels, TestCoordinator_PanicCancels, TestCoordinator_FinishOutgoingExactlyOnce, TestCoordinator_OfferRetiredOnlyAfterTransferEnds
  DoD: race test green; coverage of internal/dnd >= 85%; per-item gates green; commit `feat(dnd): cross-machine drag coordinator [FEAT-116]`

- [ ] [P4-F-016] Engine drag hooks: left button kept held on the drag origin, target-side routing of left button and Escape, cancel on jump/panic [FEAT-116] [tier:sonnet why:routing changes on the engine hot path without allocations] [owner:P4-F] [brief:briefs/streams/p4-f.md#item-p4-f-016]
  Needs: P4-F-015
  Files: internal/engine/draghooks.go, internal/engine/draghooks_test.go, internal/engine/server.go (claim), internal/engine/client.go (claim)
  Tests: TestServer_SwitchDuringDragKeepsLeftHeldOnOrigin, TestClient_LeftUpRoutedToDragWhileTarget, TestClient_EscapeRoutedToDragWhileTarget, TestServer_HotkeyJumpCancelsDrag, TestServer_PanicCancelsDrag, TestServer_LockToScreenBlocksSwitchDuringDrag, TestEngine_NilHooksBehaveAsBefore, BenchmarkServerMotionWithHooks (0 allocs/op, PB-05 unchanged)
  DoD: all pre-existing engine tests green; coverage of internal/engine >= 85%; per-item gates green; commit `feat(engine): drag-and-drop hooks [FEAT-116]`

- [ ] [P4-F-017] Agent wiring: file sync, offers, coordinator, staging sweeps, unsupported reporting, i18n keys [FEAT-099, FEAT-101, FEAT-116] [tier:haiku] [owner:P4-F] [brief:briefs/streams/p4-f.md#item-p4-f-017]
  Needs: P4-F-013, P4-F-016
  Files: internal/agent/files.go, internal/agent/files_test.go, internal/i18n/locales/en/files.toml (D-94)
  Tests: TestAgent_WiresFileClipboardWhenSupported, TestAgent_ReportsFileClipboardUnsupportedOnce, TestAgent_WiresDragDropWhenSupported, TestAgent_ReportsDragDropUnsupportedOnce, TestAgent_StagingSweepAtStartAndHourly (synctest), TestAgent_SecureHelperModeHasNoFiles, i18n missing/unused key test green
  DoD: coverage of internal/agent >= 85%; per-item gates green; commit `feat(agent): wire lazy file paste and drag-and-drop [FEAT-099]`

- [ ] [P4-F-018] Loopback end-to-end tests: file paste and drag-and-drop between agents on fakes [FEAT-100, FEAT-116] [tier:haiku] [owner:P4-F] [brief:briefs/streams/p4-f.md#item-p4-f-018]
  Needs: P4-F-017
  Files: test/e2e/files_test.go, test/e2e/dnd_test.go
  Tests: TestE2E_FilePasteServerToClient, TestE2E_FilePasteClientToClientRelay, TestE2E_PasteAfterSupersedeFailsCleanly, TestE2E_DisconnectMidPasteThenResume, TestE2E_DnDServerToClient, TestE2E_DnDClientToServer, TestE2E_DnDEscapeCancels
  DoD: tests green with -race; per-item gates green; commit `test(e2e): lazy file paste and drag-and-drop [FEAT-100]`

- [ ] [P4-F-019] Mid-build loopback benches: throughput, time-to-first-byte, memory ceiling, resume after interruption [FEAT-100, FEAT-101] [tier:haiku] [owner:P4-F] [brief:briefs/streams/p4-f.md#item-p4-f-019]
  Needs: P4-F-012
  Files: internal/transfer/vfile_bench_test.go
  Tests: BenchmarkVFilePasteLoopback1GiB, TestVFileThroughputBudget (>= 90% of the plain transfer-engine loopback throughput measured in the same run), TestVFileTTFBBudget (loopback p95 within the PB-25 loopback value from P4-F-002), TestVFileMemoryCeiling (heap-in-use growth <= 4 x read-ahead window + 16 MiB with 4 concurrent streams), TestVFileResumeAfterInterruption (connection dropped at 40%, resumed, SHA-256 matches)
  DoD: numbers in the report's TESTS line; the parent appends the result rows (PB-11 loopback-relative, PB-25 loopback, PB-17 loopback) to perf-budgets.md; per-item gates green; commit `perf(transfer): virtual-file loopback benches [FEAT-100]`

- [ ] [P4-F-020] GUI drop contract: Drop, Target, Area, Router (no per-OS code) [FEAT-106, FEAT-109, FEAT-112, FEAT-115] [tier:haiku] [owner:P4-F] [brief:briefs/streams/p4-f.md#item-p4-f-020]
  Needs: P4-F-002
  Files: gui/drop/drop.go, gui/drop/area.go, gui/drop/router.go, gui/drop/router_test.go, gui/drop/area_test.go
  Tests: TestRouter_HitTestTopmostWins, TestRouter_NoTargetReturnsErrNoTarget, TestRouter_EmptyPathsIgnored, TestRouter_DeliverCallsSenderWithPeer, TestRouter_ConcurrentHitTestDuringFrameUpdate (race), TestArea_RecordsRectInWindowPixels, TestArea_RectsClearedEachFrame
  DoD: builds on the host with `CGO_ENABLED=0`; per-item gates green; commit `feat(gui): native file-drop contract and router [FEAT-106]`

- [ ] [P4-F-021] P4-F code review [FEAT-099, FEAT-100, FEAT-101, FEAT-102, FEAT-116] [tier:sonnet why:review gate] [owner:P4-F] [brief:briefs/streams/p4-f.md#item-p4-f-021]
  Needs: P4-F-003 .. P4-F-020
  Files: none (findings go to the parent; each finding becomes an appended fix item)
  Tests: `code-reviewer` over the merged P4-F diff against the items and the brief's contract
  DoD: no open finding (fix items appended and `[x]`); commit none (the parent records the result in P4-F-023)

- [ ] [P4-F-022] P4-F security review against qa/security-review.md [FEAT-099, FEAT-100, FEAT-101, FEAT-102, FEAT-116] [tier:sonnet why:security review gate] [owner:P4-F] [brief:briefs/streams/p4-f.md#item-p4-f-022]
  Needs: P4-F-021
  Files: none (findings become appended fix items)
  Tests: checklist sections 5, 6, 7 plus the brief's P4-F focus list (manifest bounds, path sanitising, offers read confinement, staging permissions and cleanup, URI decoding, dnd decode limits, D-62)
  DoD: no open finding; commit none

- [ ] [P4-F-023] Opus consolidation, FILE CONTRACT FREEZE note, review-chain note, bench rows recorded [FEAT-099] [tier:opus] [owner:PARENT] [brief:briefs/streams/p4-f.md#item-p4-f-023]
  Needs: P4-F-022
  Files: briefs/plan-phase-4.md (append `<!-- NOTE: <date> - FILE CONTRACT FREEZE -->` and `<!-- NOTE: <date> - stream P4-F review chain green -->`), ownership.md (append note: internal/platform/files.go, internal/vfile/vfile.go and gui/drop/drop.go frozen), perf-budgets.md (results rows from P4-F-019)
  Tests: contract in code equals the brief's contract section line by line; `task ci:local` green
  DoD: commit `chore(plan): P4 file contract freeze [FEAT-099]`

### F1 - Windows virtual-file paste

- [ ] [P4-F1-001] Win32/OLE declarations and struct layouts for virtual files [FEAT-103] [tier:haiku] [owner:F1] [brief:briefs/streams/p4-f1.md#item-p4-f1-001]
  Needs: P4-F-023; P2-W2-035; sp2.md row "COM infrastructure" = GO
  Files: internal/platform/windows/win32/f1_ole_windows.go, internal/platform/windows/win32/f1_types_windows.go, internal/platform/windows/win32/f1_types_windows_test.go
  Tests: TestLayout_FORMATETC (32 bytes, offsets), TestLayout_STGMEDIUM (24), TestLayout_FILEDESCRIPTORW (592, offsets), TestLayout_FILEGROUPDESCRIPTORWHeader (array at offset 4), TestLayout_DROPFILES (20), TestLayout_STATSTG (80), TestConstants_HRESULTs (values table from the brief, verified against Microsoft Learn)
  DoD: no symbol redeclared that W2 already declares in w2_*.go (reuse); per-item gates green; commit `feat(win): OLE and shell declarations for virtual files [FEAT-103]`

- [ ] [P4-F1-002] COM IDataObject and IEnumFORMATETC objects (only when the P4-F-001 NOTE says W2 did not deliver them) [FEAT-103] [tier:sonnet why:hand-built COM vtables with reference counting on the STA] [owner:F1] [brief:briefs/streams/p4-f1.md#item-p4-f1-002]
  Needs: P4-F1-001
  Files: internal/platform/windows/com/dataobject_windows.go, internal/platform/windows/com/enumformatetc_windows.go, internal/platform/windows/com/dataobject_windows_test.go (claim on internal/platform/windows/com/**)
  Tests: TestDataObject_QueryInterfaceAddRefRelease, TestDataObject_GetDataDelegatesToSource, TestDataObject_QueryGetDataFormatTymedAspect, TestDataObject_GetCanonicalFormatEtcSameFormat, TestDataObject_AdviseNotSupported, TestEnumFormatEtc_NextSkipResetClone, TestCOM_CallbackBudgetUnder64 (all through vtables via syscall.SyscallN)
  DoD: when the NOTE says W2 delivered equivalent objects, this item is closed by the parent with a NOTE naming the W2 commit and is not dispatched; otherwise per-item gates green; commit `feat(win): COM IDataObject and IEnumFORMATETC [FEAT-103]`

- [ ] [P4-F1-003] COM IStream over a seekable source [FEAT-103] [tier:sonnet why:COM object lifetime and blocking reads serviced on the STA] [owner:F1] [brief:briefs/streams/p4-f1.md#item-p4-f1-003]
  Needs: P4-F1-001, P4-F1-002
  Files: internal/platform/windows/com/stream_windows.go, internal/platform/windows/com/stream_windows_test.go (claim on internal/platform/windows/com/**)
  Tests: TestStream_QueryInterfaceIStreamISequentialStreamIUnknown, TestStream_ReadFillsUntilEOF, TestStream_ReadAtEOFReturnsZero, TestStream_ReadErrorReturnsSTG_E_READFAULT, TestStream_SeekSetCurEnd, TestStream_StatSizeAndName, TestStream_StatNoName, TestStream_WriteDenied, TestStream_ReleaseClosesSource, TestStream_UnsupportedMethodsReturnCodes
  DoD: per-item gates green; commit `feat(win): COM IStream over seekable sources [FEAT-103]`

- [ ] [P4-F1-004] FILEGROUPDESCRIPTORW builder with Windows name rules [FEAT-103] [tier:haiku] [owner:F1] [brief:briefs/streams/p4-f1.md#item-p4-f1-004]
  Needs: P4-F1-001
  Files: internal/platform/windows/vfile/descriptor_windows.go, internal/platform/windows/vfile/descriptor_windows_test.go
  Tests: TestDescriptor_GoldenBytesTwoEntries, TestDescriptor_UnicodeNames, TestDescriptor_DirectoryAttributes, TestDescriptor_BackslashSeparators, TestDescriptor_RejectsReservedCharacters (includes ':' alternate data streams), TestDescriptor_RejectsPathLongerThan259UTF16, TestDescriptor_FiletimeConversion, TestDescriptor_SizeSplitHighLow (5 GiB)
  DoD: rejection returns `vfile.ErrNotRepresentable` wrapping the entry index; per-item gates green; commit `feat(win): FILEGROUPDESCRIPTORW builder [FEAT-103]`

- [ ] [P4-F1-005] Virtual-file data source: FileGroupDescriptorW, FileContents per lindex, Preferred DropEffect copy, SetData outcomes, read-ahead streams [FEAT-103, FEAT-102] [tier:sonnet why:IDataObject GetData/SetData semantics and lindex handling across apartments] [owner:F1] [brief:briefs/streams/p4-f1.md#item-p4-f1-005]
  Needs: P4-F1-003, P4-F1-004; sp2.md rows "virtual-file paste" = GO, "Explorer progress/cancel" = GO, "blocking behaviour" (threading model taken from its recommendation)
  Files: internal/platform/windows/vfile/datasource_windows.go, internal/platform/windows/vfile/datasource_windows_test.go
  Tests: TestDataSource_EnumeratesThreeFormats, TestDataSource_GroupDescriptorMatchesManifest, TestDataSource_FileContentsStreamsEntryContent (SHA-256), TestDataSource_FileContentsDirectoryIsDV_E_LINDEX, TestDataSource_LindexOutOfRange, TestDataSource_TymedMismatch, TestDataSource_PreferredDropEffectIsCopy, TestDataSource_PerformedDropEffectMoveTreatedAsCopy, TestDataSource_PasteSucceededReportsCompleted, TestDataSource_ReleaseWithoutOutcomeReportsAbandoned, TestDataSource_ConcurrentStreamsCappedAt4, TestDataSource_DoneCalledOnce
  DoD: per-item gates green; commit `feat(win): virtual-file data source [FEAT-103]`

- [ ] [P4-F1-006] Windows FileClipboard: STA, listener, WatchFiles (CF_HDROP), OfferFiles (OleSetClipboard), Close without flush [FEAT-103, FEAT-102] [tier:sonnet why:STA threading and OLE clipboard ownership lifecycle] [owner:F1] [brief:briefs/streams/p4-f1.md#item-p4-f1-006]
  Needs: P4-F1-005
  Files: internal/platform/windows/vfile/fileclipboard_windows.go, internal/platform/windows/vfile/hdrop_windows.go, internal/platform/windows/vfile/hdrop_windows_test.go
  Tests: TestHDROP_ParseWideList, TestHDROP_ParseRejectsNonWide, TestHDROP_ParseRejectsUnterminated (host, byte-level); behaviour of WatchFiles/OfferFiles/Close is covered by the conformance suite in P4-F1-008
  DoD: never calls OleFlushClipboard; ignores own offers (OleIsCurrentClipboard and own sequence number); per-item gates green; commit `feat(win): virtual-file clipboard backend [FEAT-103]`

- [ ] [P4-F1-007] Windows backend exposes FileClipboardSource (unsupported in secure-helper and pre-login modes) [FEAT-103] [tier:haiku] [owner:F1] [brief:briefs/streams/p4-f1.md#item-p4-f1-007]
  Needs: P4-F1-006
  Files: internal/platform/windows/files_windows.go, internal/platform/windows/files_windows_test.go
  Tests: TestBackend_ImplementsFileClipboardSource, TestBackend_SecureHelperFileClipboardUnsupported
  DoD: per-item gates green; commit `feat(win): expose file clipboard on the Windows backend [FEAT-103]`

- [ ] [P4-F1-008] Integration tests: conformance suite, shell folder drop of virtual files, clipboard round trip (windows-latest CI) [FEAT-103] [tier:sonnet why:driving Shell extraction programmatically through IShellFolder and IDropTarget] [owner:F1] [brief:briefs/streams/p4-f1.md#item-p4-f1-008]
  Needs: P4-F1-007
  Files: internal/platform/windows/vfile/vfile_integration_test.go, internal/platform/windows/vfile/shelldrop_integration_test.go, .github/workflows/ci.yml (claim; only if the Windows integration job does not already run `./internal/platform/windows/...` with `-tags integration`)
  Tests: TestConformance_FileClipboardWindows (conformance.RunFileClipboard with a Windows driver), TestShellDrop_FilesFoldersUnicodeIntoTempFolder, TestShellDrop_LargeFile64MiBHashMatches, TestShellDrop_PreferredEffectCopyLeavesSource, TestClipboard_OfferThenOleGetClipboardReadsAll, TestClipboard_WatchFilesForeignHDROP, TestClose_RevokesOwnershipWithoutFlush
  DoD: tests carry `//go:build integration && windows`; compile on the host (`go vet -tags integration ./internal/platform/windows/...`); actionlint green when ci.yml changed; commit `test(win): virtual-file integration tests [FEAT-103]`

- [ ] [P4-F1-009] Manual script: Explorer paste of virtual files between the two Windows PCs [FEAT-103] [tier:haiku] [owner:F1] [brief:briefs/streams/p4-f1.md#item-p4-f1-009]
  Needs: P4-F1-008
  Files: qa/manual/p4-f1-virtual-files.md
  Tests: script follows qa/manual/README.md and contains every scenario of the brief's list (large, many small, nested, unicode, clash, cancel, disconnect, supersede, cut-as-copy, too-long path, elevated Explorer, measurements)
  DoD: binaries prebuilt by the agent (build allowed, not run); commit `docs(qa): P4 F1 virtual-file paste script [FEAT-103]`

- [ ] [P4-F1-M01] Run the virtual-file paste script on both Windows PCs and record results incl. LAN throughput, TTFB and agent RSS [FEAT-103] [tier:haiku] [owner:USER] [brief:briefs/streams/p4-f1.md#item-p4-f1-m01]
  Needs: P4-F1-009
  Files: qa/manual/results/p4-f1-virtual-files.md
  Tests: every step has a recorded observation; measurement rows for PB-11 (4 GiB paste), PB-25 (TTFB), PB-17 (RSS)
  DoD: results committed by the parent `docs(qa): P4 F1 results [FEAT-103]`; parent appends the LAN rows to perf-budgets.md

- [ ] [P4-F1-010] F1 code review [FEAT-103] [tier:sonnet why:review gate] [owner:F1] [brief:briefs/streams/p4-f1.md#item-p4-f1-010]
  Needs: P4-F1-001 .. P4-F1-009
  Files: none (findings become appended fix items)
  Tests: `code-reviewer` over the merged F1 diff: COM refcounts, STA confinement, no Go heap pointers retained by Windows, error mapping
  DoD: no open finding

- [ ] [P4-F1-011] F1 security review [FEAT-103] [tier:sonnet why:security review gate] [owner:F1] [brief:briefs/streams/p4-f1.md#item-p4-f1-011]
  Needs: P4-F1-010
  Files: none (findings become appended fix items)
  Tests: qa/security-review.md sections 5-7 plus the brief's F1 focus list (descriptor names, ADS, path length, cut-as-copy, ownership after exit)
  DoD: no open finding

- [ ] [P4-F1-012] F1 Opus consolidation and review-chain note [FEAT-103] [tier:opus] [owner:PARENT] [brief:briefs/streams/p4-f1.md#item-p4-f1-012]
  Needs: P4-F1-011, P4-F1-M01
  Files: briefs/plan-phase-4.md (append `<!-- NOTE: <date> - stream F1 review chain green -->`), risk-register.md (append status notes for R-24, R-25)
  Tests: cross-stream consistency with F2 (shared COM package), contract adherence, decisions coverage (D-11, D-45, D-62)
  DoD: commit `chore(plan): F1 review chain green [FEAT-103]`

### F2 - Windows cross-screen drag-and-drop and GUI file drop

- [ ] [P4-F2-001] Checkpoint: user approves the Windows drag-and-drop scope [FEAT-104, FEAT-105, FEAT-106] [tier:haiku] [owner:USER] [brief:briefs/streams/p4-f2.md#item-p4-f2-001]
  Needs: P4-F-001
  Files: decisions.md (parent appends the approved scope as a new decision)
  Tests: answers recorded for: (a) outgoing via edge drop-target windows, CF_HDROP sources only, virtual-only sources rejected with a notification; (b) W1 capturer parks the cursor at the contact point during a drag (or the alternative from sp2.md); (c) incoming via DoDragDrop from an agent window with IDataObjectAsyncCapability; (d) drops into elevated targets unsupported (UIPI) unless sp2.md row "UIPI" says otherwise; (e) no DnD on the secure desktop or in pre-login mode; (f) GUI drop via RegisterDragDrop on the Gio HWND per sp2.md row "Gio drop"
  DoD: commit `docs(decisions): Windows drag-and-drop scope [FEAT-104]`

- [ ] [P4-F2-002] Win32/OLE declarations for drag-and-drop [FEAT-104, FEAT-105] [tier:haiku] [owner:F2] [brief:briefs/streams/p4-f2.md#item-p4-f2-002]
  Needs: P4-F2-001, P4-F1-001
  Files: internal/platform/windows/win32/f2_dnd_windows.go, internal/platform/windows/win32/f2_dnd_windows_test.go
  Tests: TestConstants_DragDrop (values table from the brief), TestIIDs_DropSourceDropTargetAsync (GUID bytes)
  DoD: no symbol redeclared that W1, W2 or F1 already declare; per-item gates green; commit `feat(win): drag-and-drop declarations [FEAT-104]`

- [ ] [P4-F2-003] COM IDropSource and IDropTarget objects plus foreign IDataObject client helpers [FEAT-104, FEAT-105, FEAT-106] [tier:sonnet why:COM vtables with by-value POINTL arguments and reference counting] [owner:F2] [brief:briefs/streams/p4-f2.md#item-p4-f2-003]
  Needs: P4-F2-002, P4-F1-002
  Files: internal/platform/windows/com/dropsource_windows.go, internal/platform/windows/com/droptarget_windows.go, internal/platform/windows/com/foreign_windows.go, internal/platform/windows/com/drop_windows_test.go (claim on internal/platform/windows/com/**)
  Tests: TestDropSource_QueryContinueDragDelegates, TestDropSource_GiveFeedbackDefaultCursors, TestDropTarget_PointLUnpackedXY (negative coordinates included), TestDropTarget_EnterOverLeaveDropDelegate, TestDropTarget_EffectWrittenBack, TestForeign_HasFormat, TestForeign_ReadHDROPPaths (in-process foreign object built from a test DataSource), TestCOM_CallbackBudgetUnder64
  DoD: per-item gates green; commit `feat(win): COM drop source and drop target [FEAT-104]`

- [ ] [P4-F2-004] IDataObjectAsyncCapability on the virtual-file data object (asynchronous extraction for drops) [FEAT-105] [tier:sonnet why:QueryInterface aggregation of a second interface onto an existing COM object] [owner:F2] [brief:briefs/streams/p4-f2.md#item-p4-f2-004]
  Needs: P4-F2-003, P4-F1-005
  Files: internal/platform/windows/com/asynccap_windows.go, internal/platform/windows/com/asynccap_windows_test.go, internal/platform/windows/vfile/datasource_windows.go (claim, F1 file: add the async option)
  Tests: TestAsyncCap_QueryInterfaceFromDataObject, TestAsyncCap_GetAsyncModeTrue, TestAsyncCap_StartInEndOperation, TestAsyncCap_EndOperationMapsEffectToOutcome, TestDataSource_ClipboardInstanceHasNoAsyncCapability
  DoD: per-item gates green; commit `feat(win): asynchronous extraction capability for drags [FEAT-105]`

- [ ] [P4-F2-005] Edge strip geometry: real-edge segments to drop-window rectangles [FEAT-104] [tier:haiku] [owner:F2] [brief:briefs/streams/p4-f2.md#item-p4-f2-005]
  Needs: P4-F2-001
  Files: internal/platform/windows/dnd/geometry.go, internal/platform/windows/dnd/geometry_test.go
  Tests: TestStrip_LeftRightTopBottom, TestStrip_InsideMonitorBounds, TestStrip_TwoPixelThickness, TestStrip_SegmentSubrange, TestStrip_NegativeVirtualDesktopCoordinates, TestStrip_PointInsideStrip
  DoD: coverage of the file's functions >= 85%; per-item gates green; commit `feat(win): drag edge strip geometry [FEAT-104]`

- [ ] [P4-F2-006] Edge drop-target windows: SetEdges, Arm (click-through toggle), WatchOutgoing [FEAT-104] [tier:sonnet why:OLE drop-target window lifecycle on the STA and hit-testing at screen edges] [owner:F2] [brief:briefs/streams/p4-f2.md#item-p4-f2-006]
  Needs: P4-F2-003, P4-F2-005; sp2.md row "edge drop target" = GO
  Files: internal/platform/windows/dnd/dragdrop_windows.go, internal/platform/windows/dnd/edge_windows.go, internal/platform/windows/dnd/edge_windows_test.go
  Tests: TestEdge_EnterWithHDROPEmitsEnter, TestEdge_EnterWithoutHDROPEmitsUnsupportedAndNone, TestEdge_LeaveEmitsLeave, TestEdge_DropAtEdgeEmitsDropAndReturnsNone, TestEdge_NeverReturnsMove, TestEdge_SetEdgesReplacesWindows (logic with an injected window factory)
  DoD: windows carry WS_EX_TRANSPARENT whenever not armed; per-item gates green; commit `feat(win): edge drop-target windows for outgoing drags [FEAT-104]`

- [ ] [P4-F2-007] FinishOutgoing: end the foreign local drag loop by marked local injection [FEAT-104] [tier:sonnet why:ending a foreign modal drag loop safely through injected input] [owner:F2] [brief:briefs/streams/p4-f2.md#item-p4-f2-007]
  Needs: P4-F2-006
  Files: internal/platform/windows/dnd/finish_windows.go, internal/platform/windows/dnd/finish_windows_test.go
  Tests: TestFinish_AcceptedInjectsLeftUpAndDropReturnsCopy, TestFinish_CancelInjectsEscapeAndDropReturnsNone, TestFinish_EventsCarryEdgeSyncMarker, TestFinish_IdempotentSecondCall, TestFinish_NoActiveDragError (with a fake SendInput seam)
  DoD: per-item gates green; commit `feat(win): finish outgoing drags [FEAT-104]`

- [ ] [P4-F2-008] BeginIncoming: DoDragDrop driver window with remote release and cancel [FEAT-105] [tier:sonnet why:driving DoDragDrop from a remotely controlled drop source] [owner:F2] [brief:briefs/streams/p4-f2.md#item-p4-f2-008]
  Needs: P4-F2-004, P4-F2-006; sp2.md row "drag source" = GO
  Files: internal/platform/windows/dnd/incoming_windows.go, internal/platform/windows/dnd/incoming_windows_test.go
  Tests: TestIncoming_QueryContinueDragFollowsReleaseNotKeyState, TestIncoming_CancelReturnsDRAGDROP_S_CANCEL, TestIncoming_ContextCancelCancels, TestIncoming_EscapeFlagCancels, TestIncoming_ResultDroppedOnlyForCopy, TestIncoming_ButtonStateRestoredAfterLoop (fake SendInput seam), TestIncoming_SecondBeginWhileActiveRejected
  DoD: per-item gates green; commit `feat(win): incoming drags with virtual files [FEAT-105]`

- [ ] [P4-F2-009] Windows backend exposes DragDropSource (unsupported in secure-helper and pre-login modes) [FEAT-104, FEAT-105] [tier:haiku] [owner:F2] [brief:briefs/streams/p4-f2.md#item-p4-f2-009]
  Needs: P4-F2-007, P4-F2-008
  Files: internal/platform/windows/dnd_windows.go, internal/platform/windows/dnd_windows_test.go
  Tests: TestBackend_ImplementsDragDropSource, TestBackend_SecureHelperDragDropUnsupported
  DoD: per-item gates green; commit `feat(win): expose drag-and-drop on the Windows backend [FEAT-104]`

- [ ] [P4-F2-010] Integration tests: conformance suite, incoming drag into a shell folder, edge target fed by a test drag source (windows-latest CI) [FEAT-104, FEAT-105] [tier:sonnet why:driving OLE drag-and-drop programmatically on a CI desktop] [owner:F2] [brief:briefs/streams/p4-f2.md#item-p4-f2-010]
  Needs: P4-F2-009
  Files: internal/platform/windows/dnd/dnd_integration_test.go
  Tests: TestConformance_DragDropWindows, TestIncoming_DropIntoShellFolderMaterialisesFiles, TestIncoming_CancelLeavesFolderEmpty, TestEdge_TestDragSourceProducesEnterAndPaths, TestEdge_UnarmedWindowIsClickThrough
  DoD: `//go:build integration && windows`; compiles on the host with `go vet -tags integration`; evidence level per test recorded in the report (CI desktop session may lack an interactive input desktop: such tests log and fail loudly, never pass silently); commit `test(win): drag-and-drop integration tests [FEAT-104]`

- [ ] [P4-F2-011] GUI file drop on Windows: IDropTarget on the Gio HWND [FEAT-106] [tier:sonnet why:native OLE drop target bound to the Gio window thread] [owner:F2] [brief:briefs/streams/p4-f2.md#item-p4-f2-011]
  Needs: P4-F-020, P4-F2-003; sp2.md row "Gio drop" = GO; P4-F-002 depguard allowance recorded
  Files: gui/drop/drop_windows.go, gui/drop/drop_windows_test.go
  Tests: TestWindowsTarget_ScreenToClientConversion, TestWindowsTarget_EffectCopyOnlyOverArea, TestWindowsTarget_NonHDROPRejected, TestWindowsTarget_DetachRevokes (logic with injected seams), TestWindowsTarget_AttachRejectsNonWin32View
  DoD: `go build ./cmd/edgesync` on the host with `CGO_ENABLED=0`; per-item gates green; commit `feat(gui): native file drop on Windows [FEAT-106]`

- [ ] [P4-F2-012] Wire GUI drop targets into the Machines page tiles and layout-editor machines (all OSes) [FEAT-106, FEAT-109, FEAT-112, FEAT-115] [tier:haiku] [owner:F2] [brief:briefs/streams/p4-f2.md#item-p4-f2-012]
  Needs: P4-F2-011, P4-F3-010, P4-F4-011, P4-F5-010 (or their approved variant-B replacements)
  Files: gui/peers/rows.go (claim), gui/layouteditor/canvas.go (claim), gui/shell/shell.go (claim), internal/i18n/locales/en/gui_drop.toml (D-94; additive keys `drop.hover`, `drop.unsupported`, `drop.failed`)
  Tests: TestMachinesTile_RegistersDropArea, TestLayoutEditor_RegistersDropAreaPerMachine, TestShell_AttachesTargetOnViewEvent, TestShell_UnsupportedViewShowsHint (fake Target and fake Sender)
  DoD: GUI builds on Windows (host) and on macOS/Linux CI; per-item gates green; commit `feat(gui): drop files onto machines [FEAT-106]`

- [ ] [P4-F2-013] Manual script: drag-and-drop between the two Windows PCs and GUI drop on Windows [FEAT-104, FEAT-105, FEAT-106] [tier:haiku] [owner:F2] [brief:briefs/streams/p4-f2.md#item-p4-f2-013]
  Needs: P4-F2-010, P4-F2-012
  Files: qa/manual/p4-f2-dnd-windows.md
  Tests: script follows qa/manual/README.md and contains every scenario of the brief's list
  DoD: binaries prebuilt; commit `docs(qa): P4 F2 drag-and-drop script [FEAT-104]`

- [ ] [P4-F2-M01] Run the Windows drag-and-drop and GUI drop script on both Windows PCs and record results [FEAT-104, FEAT-105, FEAT-106] [tier:haiku] [owner:USER] [brief:briefs/streams/p4-f2.md#item-p4-f2-m01]
  Needs: P4-F2-013
  Files: qa/manual/results/p4-f2-dnd-windows.md
  Tests: every step has a recorded observation
  DoD: results committed by the parent `docs(qa): P4 F2 results [FEAT-104]`

- [ ] [P4-F2-014] F2 code review [FEAT-104, FEAT-105, FEAT-106] [tier:sonnet why:review gate] [owner:F2] [brief:briefs/streams/p4-f2.md#item-p4-f2-014]
  Needs: P4-F2-002 .. P4-F2-013
  Files: none (findings become appended fix items)
  Tests: `code-reviewer` over the merged F2 diff
  DoD: no open finding

- [ ] [P4-F2-015] F2 security review [FEAT-104, FEAT-105, FEAT-106] [tier:sonnet why:security review gate] [owner:F2] [brief:briefs/streams/p4-f2.md#item-p4-f2-015]
  Needs: P4-F2-014
  Files: none (findings become appended fix items)
  Tests: qa/security-review.md sections 5-7 plus the brief's F2 focus list (never MOVE, injected-event marker, untrusted names in incoming drops, GUI drop path handling)
  DoD: no open finding

- [ ] [P4-F2-016] F2 Opus consolidation and review-chain note [FEAT-104, FEAT-105, FEAT-106] [tier:opus] [owner:PARENT] [brief:briefs/streams/p4-f2.md#item-p4-f2-016]
  Needs: P4-F2-015, P4-F2-M01
  Files: briefs/plan-phase-4.md (append `<!-- NOTE: <date> - stream F2 review chain green -->`), risk-register.md (status notes R-28, R-30)
  Tests: consistency with F1 and the coordinator contract; D-12, D-62 coverage
  DoD: commit `chore(plan): F2 review chain green [FEAT-104]`

### F3 - macOS file paste, drag-and-drop and GUI drop (compile + CI only)

- [ ] [P4-F3-001] Checkpoint: user approves the macOS file mechanisms and their evidence level [FEAT-107, FEAT-108, FEAT-109] [tier:haiku] [owner:USER] [brief:briefs/streams/p4-f3.md#item-p4-f3-001]
  Needs: P4-F-001
  Files: decisions.md (parent appends the approved mechanisms as a new decision)
  Tests: answers recorded for: paste variant A (file promises, D-11; Finder paste unverifiable, R-26) or B (lazy public.file-url with staging); macOS 15.4+ pasteboard access alerts when reading file URLs on copy (R-15); DnD both directions via edge drop panels and an agent drag source; GUI drop via a cgo Objective-C drag destination on Gio's NSView; acceptance that all of this is CI-verified or compile-only (R-07)
  DoD: commit `docs(decisions): macOS file mechanisms [FEAT-107]`

- [ ] [P4-F3-002] purego AppKit/Foundation bindings for pasteboard files, promises and drag sessions [FEAT-107, FEAT-108] [tier:haiku] [owner:F3] [brief:briefs/streams/p4-f3.md#item-p4-f3-002]
  Needs: P4-F3-001, P4-F-023; P2-M1-027; sp4.md row "purego bindings" = GO
  Files: internal/platform/darwin/vfile/appkit_darwin.go, internal/platform/darwin/vfile/selectors.go, internal/platform/darwin/vfile/selectors_test.go
  Tests: TestBindings_SelectorNamesTable (host, build-tag free table of selector strings checked against the brief); compile with `GOOS=darwin GOARCH=arm64` and `GOARCH=amd64` on the host; runtime registration is exercised by P4-F3-011
  DoD: reuses M1's objc helpers (no duplicates); per-item gates green; commit `feat(macos): AppKit bindings for files and drags [FEAT-107]`

- [ ] [P4-F3-003] File promise delegate class and promise writer (off main thread, os.Root, partial files) [FEAT-107] [tier:sonnet why:Objective-C delegate registration via purego and promise completion threading] [owner:F3] [brief:briefs/streams/p4-f3.md#item-p4-f3-003]
  Needs: P4-F3-002; sp4.md row "file promises (verifiable vs not)" not NO-GO
  Files: internal/platform/darwin/vfile/promise_darwin.go, internal/platform/darwin/vfile/writer.go, internal/platform/darwin/vfile/writer_test.go
  Tests: TestWriter_WritesTreeUnderRoot, TestWriter_PartialRenamedAfterHashVerify, TestWriter_ClashSuffix, TestWriter_RejectsUnsafeNames, TestWriter_CancelRemovesPartial (writer.go is build-tag free and host-tested); promise class behaviour in P4-F3-011
  DoD: completion handler always called exactly once (success or NSError); per-item gates green; commit `feat(macos): file promise delegate [FEAT-107]`

- [ ] [P4-F3-004] macOS FileClipboard: changeCount watch for file URLs, OfferFiles with one promise per top-level entry, Close [FEAT-107, FEAT-102] [tier:sonnet why:pasteboard ownership and change detection on the AppKit main thread] [owner:F3] [brief:briefs/streams/p4-f3.md#item-p4-f3-004]
  Needs: P4-F3-003
  Files: internal/platform/darwin/vfile/fileclipboard_darwin.go
  Tests: compile checks on the host; behaviour via the conformance suite in P4-F3-011
  DoD: own writes ignored by changeCount; per-item gates green; commit `feat(macos): file clipboard backend [FEAT-107]`

- [ ] [P4-F3-005] macOS backend exposes FileClipboardSource [FEAT-107] [tier:haiku] [owner:F3] [brief:briefs/streams/p4-f3.md#item-p4-f3-005]
  Needs: P4-F3-004
  Files: internal/platform/darwin/files_darwin.go
  Tests: cross-compile; TestBackend_ImplementsFileClipboardSource in P4-F3-011
  DoD: per-item gates green; commit `feat(macos): expose file clipboard on the macOS backend [FEAT-107]`

- [ ] [P4-F3-006] Edge drop panels (NSDraggingDestination), Arm and WatchOutgoing [FEAT-108] [tier:sonnet why:registering a dragging-destination view class via purego and toggling mouse transparency] [owner:F3] [brief:briefs/streams/p4-f3.md#item-p4-f3-006]
  Needs: P4-F3-002
  Files: internal/platform/darwin/dnd/dragdrop_darwin.go, internal/platform/darwin/dnd/edge_darwin.go, internal/platform/darwin/dnd/geometry.go, internal/platform/darwin/dnd/geometry_test.go
  Tests: TestStrip_PointsFromPhysicalSegments (host, scale conversion), TestStrip_FlippedYCoordinates (host); panel behaviour via P4-F3-011
  DoD: never returns a move operation; per-item gates green; commit `feat(macos): edge drop panels for outgoing drags [FEAT-108]`

- [ ] [P4-F3-007] FinishOutgoing on macOS [FEAT-108] [tier:sonnet why:ending Finder's drag session through marked synthetic events] [owner:F3] [brief:briefs/streams/p4-f3.md#item-p4-f3-007]
  Needs: P4-F3-006
  Files: internal/platform/darwin/dnd/finish_darwin.go
  Tests: cross-compile; behaviour via P4-F3-011
  DoD: injected events carry M1's synthetic-event marker; per-item gates green; commit `feat(macos): finish outgoing drags [FEAT-108]`

- [ ] [P4-F3-008] BeginIncoming: agent drag-source view starting an NSDraggingSession with promise items [FEAT-108] [tier:sonnet why:starting an AppKit dragging session from a remotely controlled event] [owner:F3] [brief:briefs/streams/p4-f3.md#item-p4-f3-008]
  Needs: P4-F3-003, P4-F3-006
  Files: internal/platform/darwin/dnd/incoming_darwin.go
  Tests: cross-compile; behaviour via P4-F3-011
  DoD: per-item gates green; commit `feat(macos): incoming drags with file promises [FEAT-108]`

- [ ] [P4-F3-009] macOS backend exposes DragDropSource [FEAT-108] [tier:haiku] [owner:F3] [brief:briefs/streams/p4-f3.md#item-p4-f3-009]
  Needs: P4-F3-007, P4-F3-008
  Files: internal/platform/darwin/dnd_darwin.go
  Tests: cross-compile; TestBackend_ImplementsDragDropSource in P4-F3-011
  DoD: per-item gates green; commit `feat(macos): expose drag-and-drop on the macOS backend [FEAT-108]`

- [ ] [P4-F3-010] GUI file drop on macOS: cgo Objective-C drag destination added to Gio's NSView [FEAT-109] [tier:sonnet why:adding NSDraggingDestination methods to a foreign view class at runtime] [owner:F3] [brief:briefs/streams/p4-f3.md#item-p4-f3-010]
  Needs: P4-F-020
  Files: gui/drop/drop_darwin.go, gui/drop/drop_darwin.m
  Tests: built on macos-latest CI; TestDarwinTarget_AttachRejectsNonAppKitView and drop delivery in P4-F3-011
  DoD: builds on CI with cgo; commit `feat(gui): native file drop on macOS [FEAT-109]`

- [ ] [P4-F3-011] macOS integration tests on CI (conformance, promises, pasteboard, edge panels, incoming session, GUI drop) and CI job extension [FEAT-107, FEAT-108, FEAT-109] [tier:haiku] [owner:F3] [brief:briefs/streams/p4-f3.md#item-p4-f3-011]
  Needs: P4-F3-005, P4-F3-009, P4-F3-010
  Files: internal/platform/darwin/vfile/vfile_integration_test.go, internal/platform/darwin/dnd/dnd_integration_test.go, gui/drop/drop_darwin_integration_test.go, .github/workflows/ci.yml (claim: macOS integration job covers these packages)
  Tests: TestConformance_FileClipboardDarwin, TestPromise_FulfilledProgrammaticallyWritesTree, TestOfferFiles_PasteboardCarriesPromises, TestWatchFiles_ForeignFileURLs, TestClose_ClearsOwnPasteboard, TestBackend_ImplementsFileClipboardSource, TestBackend_ImplementsDragDropSource, TestEdge_PanelRegisteredForFileURLs, TestIncoming_SessionStartsOrLogsTCCState, TestDarwinTarget_AttachRejectsNonAppKitView; each test logs its evidence level
  DoD: `//go:build integration && darwin`; vet clean with `GOOS=darwin`; actionlint green; commit `test(macos): file and drag integration tests [FEAT-107]`

- [ ] [P4-F3-012] CI dispatch script for the macOS file and drag tests [FEAT-107, FEAT-108, FEAT-109] [tier:haiku] [owner:F3] [brief:briefs/streams/p4-f3.md#item-p4-f3-012]
  Needs: P4-F3-011
  Files: qa/manual/p4-f3-macos-ci.md
  Tests: script tells the user how to push, dispatch, which logs and artifacts to share, and lists the behaviours that stay unverifiable without a Mac
  DoD: commit `docs(qa): P4 F3 macOS CI script [FEAT-107]`

- [ ] [P4-F3-M01] Push and dispatch CI; share the macOS integration job log [FEAT-107, FEAT-108, FEAT-109] [tier:haiku] [owner:USER] [brief:briefs/streams/p4-f3.md#item-p4-f3-m01]
  Needs: P4-F3-012
  Files: qa/manual/results/p4-f3-macos-ci.md
  Tests: run URL and test output recorded
  DoD: results committed by the parent `docs(qa): P4 F3 CI results [FEAT-107]`

- [ ] [P4-F3-013] F3 code review [FEAT-107, FEAT-108, FEAT-109] [tier:sonnet why:review gate] [owner:F3] [brief:briefs/streams/p4-f3.md#item-p4-f3-013]
  Needs: P4-F3-002 .. P4-F3-012
  Files: none (findings become appended fix items)
  Tests: `code-reviewer` over the merged F3 diff (main-thread rules, Objective-C object lifetimes, block invocation)
  DoD: no open finding

- [ ] [P4-F3-014] F3 security review [FEAT-107, FEAT-108, FEAT-109] [tier:sonnet why:security review gate] [owner:F3] [brief:briefs/streams/p4-f3.md#item-p4-f3-014]
  Needs: P4-F3-013
  Files: none (findings become appended fix items)
  Tests: qa/security-review.md sections 5-7 plus the brief's F3 focus list (promise destination confinement, names, pasteboard reads)
  DoD: no open finding

- [ ] [P4-F3-015] F3 Opus consolidation and review-chain note [FEAT-107, FEAT-108, FEAT-109] [tier:opus] [owner:PARENT] [brief:briefs/streams/p4-f3.md#item-p4-f3-015]
  Needs: P4-F3-014, P4-F3-M01
  Files: briefs/plan-phase-4.md (append `<!-- NOTE: <date> - stream F3 review chain green -->`), risk-register.md (status notes R-07, R-15, R-26)
  Tests: unverifiable behaviours listed for the user in the note
  DoD: commit `chore(plan): F3 review chain green [FEAT-107]`

### F4 - Linux X11 file paste, XDND and GUI drop

- [ ] [P4-F4-001] Checkpoint: user approves the X11 file mechanisms [FEAT-110, FEAT-111, FEAT-112] [tier:haiku] [owner:USER] [brief:briefs/streams/p4-f4.md#item-p4-f4-001]
  Needs: P4-F-001
  Files: decisions.md (parent appends the approved mechanisms as a new decision)
  Tests: answers recorded for: paste variant A (delayed selection reply until staging completes, per sp3.md row "lazy file paste mechanism recommendation for X11 and Wayland") or B (pre-stage on announce below a size threshold, delayed reply above); outgoing XDND without a pointer grab using XI2 raw motion (requires sp3.md row "XI2 generation" = GO; otherwise outgoing unsupported); incoming as XDND source; GUI drop via XdndProxy on the Gio window
  DoD: commit `docs(decisions): X11 file mechanisms [FEAT-110]`

- [ ] [P4-F4-002] X11 atoms and INCR helpers for the file packages [FEAT-110, FEAT-111] [tier:haiku] [owner:F4] [brief:briefs/streams/p4-f4.md#item-p4-f4-002]
  Needs: P4-F4-001, P4-F-023; P2-X1-025
  Files: internal/platform/x11/vfile/atoms_linux.go, internal/platform/x11/vfile/incr_linux.go (only when the P4-F-001 NOTE says X1 exports no reusable INCR helper), internal/platform/x11/vfile/incr_test.go
  Tests: TestINCR_ChunkPlanForMaxRequestLength, TestINCR_ZeroLengthTerminator, TestINCR_ReassemblyOrder (pure chunk planning, build-tag free part)
  DoD: `GOOS=linux CGO_ENABLED=0` vet and build green; per-item gates green; commit `feat(x11): atoms and INCR helpers for file transfer [FEAT-110]`

- [ ] [P4-F4-003] X11 FileClipboard: XFixes watch for uri-list owners, CLIPBOARD ownership with delayed staged replies [FEAT-110, FEAT-102] [tier:sonnet why:X11 selection protocol with delayed replies, INCR and concurrent requestors] [owner:F4] [brief:briefs/streams/p4-f4.md#item-p4-f4-003]
  Needs: P4-F4-002; sp3.md row "lazy file paste mechanism recommendation for X11 and Wayland" = variant approved in P4-F4-001
  Files: internal/platform/x11/vfile/fileclipboard_linux.go, internal/platform/x11/vfile/reply.go, internal/platform/x11/vfile/reply_test.go
  Tests: TestReplyPlan_TargetsList, TestReplyPlan_URIListAfterStaging, TestReplyPlan_FailureRefusesWithNone, TestReplyPlan_CoalescesRequestorsForSameManifest, TestReplyPlan_GnomeCopiedFilesUsesCopyVerb (pure state machine in reply.go, host-tested)
  DoD: the xgb event loop never blocks on staging; per-item gates green; commit `feat(x11): lazy file clipboard [FEAT-110]`

- [ ] [P4-F4-004] X11 backend exposes FileClipboardSource [FEAT-110] [tier:haiku] [owner:F4] [brief:briefs/streams/p4-f4.md#item-p4-f4-004]
  Needs: P4-F4-003
  Files: internal/platform/x11/files_linux.go
  Tests: cross-compile; TestBackend_ImplementsFileClipboardSource in P4-F4-012
  DoD: per-item gates green; commit `feat(x11): expose file clipboard on the X11 backend [FEAT-110]`

- [ ] [P4-F4-005] XDND message codec (version 5 ClientMessage data) [FEAT-111, FEAT-112] [tier:haiku] [owner:F4] [brief:briefs/streams/p4-f4.md#item-p4-f4-005]
  Needs: P4-F4-002
  Files: internal/platform/x11/dnd/xdnd.go, internal/platform/x11/dnd/xdnd_test.go
  Tests: TestXdnd_EnterThreeTypesAndTypeListFlag, TestXdnd_PositionPacksRootXY, TestXdnd_StatusAcceptAndRect, TestXdnd_LeaveDropFinished, TestXdnd_FinishedAcceptedFlagV5, TestXdnd_DecodeRejectsWrongFormatOrVersionAbove5
  DoD: coverage of xdnd.go >= 85%; per-item gates green; commit `feat(x11): XDND message codec [FEAT-111]`

- [ ] [P4-F4-006] Edge XdndAware windows, Arm via input shape, WatchOutgoing [FEAT-111] [tier:sonnet why:XDND target protocol on override-redirect windows with input-shape toggling] [owner:F4] [brief:briefs/streams/p4-f4.md#item-p4-f4-006]
  Needs: P4-F4-005; sp3.md row "XDND" = GO
  Files: internal/platform/x11/dnd/dragdrop_linux.go, internal/platform/x11/dnd/edge_linux.go, internal/platform/x11/dnd/target.go, internal/platform/x11/dnd/target_test.go
  Tests: TestTargetState_EnterPositionStatusAccept, TestTargetState_RequestsURIListOnEnter, TestTargetState_LeaveEmitsLeave, TestTargetState_DropAtEdgeDeclined, TestTargetState_NoURIListEmitsUnsupported (pure target state machine in target.go)
  DoD: per-item gates green; commit `feat(x11): edge XDND targets for outgoing drags [FEAT-111]`

- [ ] [P4-F4-007] X11 capturer passive mode while a foreign client holds the pointer grab (RECORD observation, D-92) [FEAT-111] [tier:sonnet why:capture without a grab during a foreign drag, changing X1 capture code] [owner:F4] [brief:briefs/streams/p4-f4.md#item-p4-f4-007]
  Needs: P4-F4-006; sp3.md row "XI2 generation" = GO
  Files: internal/platform/x11/record_linux.go (claim), internal/platform/x11/capturer_linux.go (claim), internal/platform/x11/capture_passive_linux.go, internal/platform/x11/capture_passive_test.go
  Tests: TestPassive_GrabAlreadyGrabbedSwitchesToPassiveDuringDrag, TestPassive_RawMotionBecomesDXDY, TestPassive_NotUsedWithoutActiveDrag (pure decision logic host-tested)
  DoD: X1's conformance suite still passes on CI (P4-F4-012); per-item gates green; commit `feat(x11): passive capture during foreign drags [FEAT-111]`

- [ ] [P4-F4-008] FinishOutgoing on X11 (XdndFinished on drop, cancel on leave) [FEAT-111] [tier:sonnet why:reconciling the source's own drop with the remote outcome] [owner:F4] [brief:briefs/streams/p4-f4.md#item-p4-f4-008]
  Needs: P4-F4-007
  Files: internal/platform/x11/dnd/finish_linux.go, internal/platform/x11/dnd/finish_test.go
  Tests: TestFinish_DropBeforeOutcomeWaits, TestFinish_OutcomeBeforeDropAppliedOnDrop, TestFinish_SourceLeaveCancels, TestFinish_NeverPerformsMove (pure reconciliation logic)
  DoD: per-item gates green; commit `feat(x11): finish outgoing drags [FEAT-111]`

- [ ] [P4-F4-009] BeginIncoming: XDND source driver with staged uri-list replies [FEAT-111] [tier:sonnet why:driving the XDND source protocol from injected motion] [owner:F4] [brief:briefs/streams/p4-f4.md#item-p4-f4-009]
  Needs: P4-F4-003, P4-F4-005
  Files: internal/platform/x11/dnd/incoming_linux.go, internal/platform/x11/dnd/source.go, internal/platform/x11/dnd/source_test.go
  Tests: TestSourceState_EnterOnAwareWindow, TestSourceState_ProxyFollowed, TestSourceState_LeaveOnWindowChange, TestSourceState_ReleaseSendsDropWhenAccepted, TestSourceState_ReleaseNotAcceptedCancels, TestSourceState_FinishedResult, TestSourceState_CancelSendsLeave (pure source state machine)
  DoD: per-item gates green; commit `feat(x11): incoming drags as XDND source [FEAT-111]`

- [ ] [P4-F4-010] X11 backend exposes DragDropSource [FEAT-111] [tier:haiku] [owner:F4] [brief:briefs/streams/p4-f4.md#item-p4-f4-010]
  Needs: P4-F4-008, P4-F4-009
  Files: internal/platform/x11/dnd_linux.go
  Tests: cross-compile; TestBackend_ImplementsDragDropSource in P4-F4-012
  DoD: per-item gates green; commit `feat(x11): expose drag-and-drop on the X11 backend [FEAT-111]`

- [ ] [P4-F4-011] GUI file drop on X11 via XdndProxy, plus the Linux view dispatcher [FEAT-112] [tier:sonnet why:XDND proxy on a window owned by a foreign toolkit connection] [owner:F4] [brief:briefs/streams/p4-f4.md#item-p4-f4-011]
  Needs: P4-F-020, P4-F4-005; sp3.md row "Gio Linux drop"
  Files: gui/drop/drop_linux.go, gui/drop/drop_x11.go, gui/drop/proxy_x11.go, gui/drop/proxy_x11_test.go
  Tests: TestProxy_PropertiesSetOnTargetAndProxy, TestProxy_RootToWindowCoordinates, TestProxy_StatusAcceptOnlyOverArea (pure parts host-tested via `GOOS=linux` vet; logic tests build-tag free)
  DoD: drop_linux.go returns ErrUnsupportedView for view types other than X11 until F5 adds the Wayland case; builds on ubuntu CI; commit `feat(gui): native file drop on X11 [FEAT-112]`

- [ ] [P4-F4-012] X11 integration tests under Xvfb and CI job extension [FEAT-110, FEAT-111, FEAT-112] [tier:haiku] [owner:F4] [brief:briefs/streams/p4-f4.md#item-p4-f4-012]
  Needs: P4-F4-004, P4-F4-010, P4-F4-011
  Files: internal/platform/x11/vfile/vfile_integration_test.go, internal/platform/x11/dnd/dnd_integration_test.go, gui/drop/drop_x11_integration_test.go, .github/workflows/ci.yml (claim: Linux X11 integration job covers these packages)
  Tests: TestConformance_FileClipboardX11, TestFilePaste_ForeignRequestorGetsURIsAfterStaging, TestFilePaste_INCRForLargeURIList, TestWatchFiles_ForeignOwnerURIList, TestTargets_ListsFileTargets, TestSelectionClear_ReleasesOffer, TestConformance_DragDropX11, TestEdge_ForeignXdndSourceProducesEnterAndPaths, TestIncoming_DrivesForeignXdndTarget, TestProxy_ForeignSourceDropsOnProxiedWindow, TestBackend_ImplementsSources, X1 conformance suite rerun
  DoD: `//go:build integration && linux`; vet with `GOOS=linux`; actionlint green; commit `test(x11): file and drag integration tests [FEAT-110]`

- [ ] [P4-F4-013] Manual script: X11 CI dispatch and Xephyr checks on the GNOME box [FEAT-110, FEAT-111, FEAT-112] [tier:haiku] [owner:F4] [brief:briefs/streams/p4-f4.md#item-p4-f4-013]
  Needs: P4-F4-012
  Files: qa/manual/p4-f4-x11.md
  Tests: script follows qa/manual/README.md; covers CI dispatch plus Xephyr scenarios from the brief
  DoD: commit `docs(qa): P4 F4 X11 script [FEAT-110]`

- [ ] [P4-F4-M01] Dispatch the X11 integration job and run the Xephyr checks; record results [FEAT-110, FEAT-111, FEAT-112] [tier:haiku] [owner:USER] [brief:briefs/streams/p4-f4.md#item-p4-f4-m01]
  Needs: P4-F4-013
  Files: qa/manual/results/p4-f4-x11.md
  Tests: CI run URL, test output and every Xephyr step recorded
  DoD: results committed by the parent `docs(qa): P4 F4 results [FEAT-110]`

- [ ] [P4-F4-014] F4 code review [FEAT-110, FEAT-111, FEAT-112] [tier:sonnet why:review gate] [owner:F4] [brief:briefs/streams/p4-f4.md#item-p4-f4-014]
  Needs: P4-F4-002 .. P4-F4-013
  Files: none (findings become appended fix items)
  Tests: `code-reviewer` over the merged F4 diff
  DoD: no open finding

- [ ] [P4-F4-015] F4 security review [FEAT-110, FEAT-111, FEAT-112] [tier:sonnet why:security review gate] [owner:F4] [brief:briefs/streams/p4-f4.md#item-p4-f4-015]
  Needs: P4-F4-014
  Files: none (findings become appended fix items)
  Tests: qa/security-review.md sections 5-7 plus the brief's F4 focus list (URI decoding of foreign drops, staging, selection requestor trust, property sizes)
  DoD: no open finding

- [ ] [P4-F4-016] F4 Opus consolidation and review-chain note [FEAT-110, FEAT-111, FEAT-112] [tier:opus] [owner:PARENT] [brief:briefs/streams/p4-f4.md#item-p4-f4-016]
  Needs: P4-F4-015, P4-F4-M01
  Files: briefs/plan-phase-4.md (append `<!-- NOTE: <date> - stream F4 review chain green -->`), risk-register.md (status notes R-08, R-27, R-28)
  Tests: consistency with F5 (shared staging, URI codec, drop_linux.go)
  DoD: commit `chore(plan): F4 review chain green [FEAT-110]`

### F5 - Linux Wayland (GNOME 45+, KDE Plasma 6.1+) file paste, drag-and-drop and GUI drop

- [ ] [P4-F5-001] Checkpoint: user approves the Wayland file mechanisms and drag-and-drop scope [FEAT-113, FEAT-114, FEAT-115] [tier:haiku] [owner:USER] [brief:briefs/streams/p4-f5.md#item-p4-f5-001]
  Needs: P4-F-001
  Files: decisions.md (parent appends the approved mechanisms as a new decision)
  Tests: answers recorded for: paste via portal clipboard SelectionTransfer with staging (variant A) and its session-activity limits (R-20); incoming drag via an in-house Wayland client surface with an EI-injected press for the implicit-grab serial (R-29) or unsupported; outgoing drag via armed edge-strip surfaces or unsupported (sp3.md row "Wayland DnD limits"); GUI drop via a data device on Gio's display, Gio's own transfer events, or unsupported (sp3.md row "Gio Linux drop", R-30); KDE stays untested (R-09)
  DoD: commit `docs(decisions): Wayland file mechanisms [FEAT-113]`

- [ ] [P4-F5-002] Wayland FileClipboard via the Clipboard portal (owner changes, SelectionRead, SetSelection, SelectionTransfer with staging) [FEAT-113, FEAT-102] [tier:sonnet why:portal clipboard transfer flows with fd passing and delayed staged replies] [owner:F5] [brief:briefs/streams/p4-f5.md#item-p4-f5-002]
  Needs: P4-F5-001, P4-F-023; P2-Y1-025
  Files: internal/platform/wayland/vfile/fileclipboard_linux.go, internal/platform/wayland/vfile/transfer.go, internal/platform/wayland/vfile/transfer_test.go
  Tests: TestTransferPlan_URIListAfterStaging, TestTransferPlan_FailureWritesDoneFalse, TestTransferPlan_CoalescesSerialsForSameManifest, TestTransferPlan_IgnoresOwnSelection (session_is_owner), TestTransferPlan_PasswordHintNotReported (pure logic host-tested)
  DoD: unavailability of the portal clipboard is reported as `platform.ErrUnsupported` with the reason, never silently ignored; per-item gates green; commit `feat(wayland): lazy file clipboard via the portal [FEAT-113]`

- [ ] [P4-F5-003] Wayland backend exposes FileClipboardSource [FEAT-113] [tier:haiku] [owner:F5] [brief:briefs/streams/p4-f5.md#item-p4-f5-003]
  Needs: P4-F5-002
  Files: internal/platform/wayland/files_linux.go
  Tests: cross-compile; TestBackend_ImplementsFileClipboardSource in P4-F5-011
  DoD: per-item gates green; commit `feat(wayland): expose file clipboard on the Wayland backend [FEAT-113]`

- [ ] [P4-F5-004] Wayland protocol generator from wayland.xml and xdg-shell.xml, plus generated code [FEAT-114] [tier:sonnet why:code generator for typed requests and events from protocol XML] [owner:F5] [brief:briefs/streams/p4-f5.md#item-p4-f5-004]
  Needs: P4-F5-001
  Files: internal/platform/wayland/dnd/wl/gen/main.go, internal/platform/wayland/dnd/wl/gen/gen.go, internal/platform/wayland/dnd/wl/gen/gen_test.go, internal/platform/wayland/dnd/wl/gen/testdata/mini.xml, internal/platform/wayland/dnd/wl/protocol/wayland.xml, internal/platform/wayland/dnd/wl/protocol/xdg-shell.xml, internal/platform/wayland/dnd/wl/protocol/LICENSE, internal/platform/wayland/dnd/wl/generate.go, internal/platform/wayland/dnd/wl/proto_gen.go (generated)
  Tests: TestGen_GoldenMiniProtocol, TestGen_ArgumentTypesCovered (int, uint, fixed, string, object, new_id, array, fd), TestGen_EnumsAndSinceVersions
  DoD: `task gen:check` green; protocol XML versions and MIT licence recorded; per-item gates green; commit `feat(wayland): protocol generator for the drag client [FEAT-114]`

- [ ] [P4-F5-005] In-house Wayland client connection: wire codec, fd passing, dispatch loop, registry, sync, errors [FEAT-114] [tier:sonnet why:Wayland wire protocol connection with SCM_RIGHTS and object lifetimes] [owner:F5] [brief:briefs/streams/p4-f5.md#item-p4-f5-005]
  Needs: P4-F5-004
  Files: internal/platform/wayland/dnd/wl/wire.go, internal/platform/wayland/dnd/wl/wire_test.go, internal/platform/wayland/dnd/wl/conn_linux.go, internal/platform/wayland/dnd/wl/conn.go, internal/platform/wayland/dnd/wl/conn_test.go, internal/platform/wayland/dnd/wl/wire_fuzz_test.go
  Tests: TestWire_HeaderSizeOpcodePacking, TestWire_StringPaddingAndNUL, TestWire_ArrayPadding, TestWire_FixedPoint, TestWire_RejectsShortAndOversized, TestConn_RegistryGlobalsOverPipe (scripted server over net.Pipe), TestConn_SyncCallback, TestConn_DeleteIDFreesObject, TestConn_ProtocolErrorSurfaced, FuzzDecodeMessage (30 s)
  DoD: coverage of the wl package (excluding generated code) >= 85%; per-item gates green; commit `feat(wayland): minimal Wayland client connection [FEAT-114]`

- [ ] [P4-F5-006] Transparent surfaces: shm buffer, xdg_toplevel fullscreen per output, input regions [FEAT-114] [tier:haiku] [owner:F5] [brief:briefs/streams/p4-f5.md#item-p4-f5-006]
  Needs: P4-F5-005
  Files: internal/platform/wayland/dnd/surface_linux.go, internal/platform/wayland/dnd/surface.go, internal/platform/wayland/dnd/surface_test.go
  Tests: TestSurfacePlan_EdgeStripsToInputRegion, TestSurfacePlan_OutputLogicalCoordinates, TestSurfacePlan_EmptyRegionWhenUnarmed (pure planning in surface.go)
  DoD: memfd created with `unix.MemfdCreate` and sealed; per-item gates green; commit `feat(wayland): transparent drag surfaces [FEAT-114]`

- [ ] [P4-F5-007] Outgoing drags on Wayland per the P4-F5-001 decision (armed edge-strip surfaces receiving data offers) [FEAT-114] [tier:sonnet why:data-device target protocol on armed edge strips combined with InputCapture activation] [owner:F5] [brief:briefs/streams/p4-f5.md#item-p4-f5-007]
  Needs: P4-F5-006; P4-F5-001 approved variant A for outgoing
  Files: internal/platform/wayland/dnd/dragdrop_linux.go, internal/platform/wayland/dnd/outgoing_linux.go, internal/platform/wayland/dnd/offer.go, internal/platform/wayland/dnd/offer_test.go
  Tests: TestOfferState_EnterAcceptsURIList, TestOfferState_ReceiveDecodesPaths, TestOfferState_LeaveEmitsLeave, TestOfferState_NoURIListUnsupported, TestOfferState_FinishOnlyAfterDrop (pure state machine)
  DoD: per-item gates green; commit `feat(wayland): outgoing drag detection [FEAT-114]`

- [ ] [P4-F5-008] Incoming drags on Wayland per the P4-F5-001 decision (origin surface, EI-injected press, start_drag, staged send) [FEAT-114] [tier:sonnet why:implicit-grab serial acquisition and drag source protocol] [owner:F5] [brief:briefs/streams/p4-f5.md#item-p4-f5-008]
  Needs: P4-F5-006, P4-F5-002; P4-F5-001 approved variant A for incoming
  Files: internal/platform/wayland/dnd/incoming_linux.go, internal/platform/wayland/dnd/source.go, internal/platform/wayland/dnd/source_test.go
  Tests: TestSourceState_SerialFromButtonPressOnOwnSurface, TestSourceState_InputRegionClearedAfterStartDrag, TestSourceState_SendStagesThenWritesURIs, TestSourceState_DropPerformedFinishedDropped, TestSourceState_CancelledNotDropped, TestSourceState_ReleaseInjectsButtonUp (pure state machine with fake EI and fake connection)
  DoD: per-item gates green; commit `feat(wayland): incoming drags as data source [FEAT-114]`

- [ ] [P4-F5-009] Wayland backend exposes DragDropSource (or reports the approved unsupported directions) [FEAT-114] [tier:haiku] [owner:F5] [brief:briefs/streams/p4-f5.md#item-p4-f5-009]
  Needs: P4-F5-007, P4-F5-008
  Files: internal/platform/wayland/dnd_linux.go
  Tests: cross-compile; TestBackend_ImplementsDragDropSource in P4-F5-011
  DoD: per-item gates green; commit `feat(wayland): expose drag-and-drop on the Wayland backend [FEAT-114]`

- [ ] [P4-F5-010] GUI file drop on Wayland per the P4-F5-001 decision, plus the Wayland case in the Linux view dispatcher [FEAT-115] [tier:sonnet why:data device on a display connection owned by Gio via cgo libwayland-client] [owner:F5] [brief:briefs/streams/p4-f5.md#item-p4-f5-010]
  Needs: P4-F-020, P4-F4-011; P4-F5-001 approved variant A for GUI drop
  Files: gui/drop/drop_wayland.go, gui/drop/drop_wayland_c.go, gui/drop/drop_linux.go (claim, F4 file: add the Wayland case)
  Tests: built on ubuntu CI; TestWaylandTarget_AttachRejectsNonWaylandView, TestWaylandTarget_OnlyGioSurfaceAccepted (logic with seams, build-tag free helper file allowed)
  DoD: builds on ubuntu CI with cgo; commit `feat(gui): native file drop on Wayland [FEAT-115]`

- [ ] [P4-F5-011] Tests: weston-headless CI job for the Wayland client, GNOME-box integration tests and CI job extension [FEAT-113, FEAT-114, FEAT-115] [tier:haiku] [owner:F5] [brief:briefs/streams/p4-f5.md#item-p4-f5-011]
  Needs: P4-F5-003, P4-F5-009, P4-F5-010
  Files: internal/platform/wayland/dnd/wl/weston_integration_test.go, internal/platform/wayland/vfile/vfile_integration_test.go, internal/platform/wayland/dnd/dnd_integration_test.go, .github/workflows/ci.yml (claim: new job `wayland-weston` on ubuntu-24.04)
  Tests: TestWeston_RegistryHasCompositorShmSeatDataDevice, TestWeston_SurfaceCommitAndConfigure, TestWeston_DataSourceOfferRoundTripBetweenTwoClients (selection path if the headless seat allows it, else logged as unverifiable); GNOME-box only (`integration && linux && wayland`): TestConformance_FileClipboardWayland, TestPortalPaste_StagedURIs, TestConformance_DragDropWayland, TestBackend_ImplementsSources
  DoD: vet with `GOOS=linux`; actionlint green; commit `test(wayland): weston and GNOME integration tests [FEAT-113]`

- [ ] [P4-F5-012] Manual script: Wayland paste, drag-and-drop with the Windows PCs, GUI drop on the GNOME box [FEAT-113, FEAT-114, FEAT-115] [tier:haiku] [owner:F5] [brief:briefs/streams/p4-f5.md#item-p4-f5-012]
  Needs: P4-F5-011
  Files: qa/manual/p4-f5-wayland.md
  Tests: script follows qa/manual/README.md; covers every scenario of the brief incl. GNOME box as client and as server, cross-OS DnD with the Windows PCs, consent dialogs, versions to record
  DoD: commit `docs(qa): P4 F5 Wayland script [FEAT-113]`

- [ ] [P4-F5-M01] Run the Wayland script on the GNOME box with the Windows PCs and record results [FEAT-113, FEAT-114, FEAT-115] [tier:haiku] [owner:USER] [brief:briefs/streams/p4-f5.md#item-p4-f5-m01]
  Needs: P4-F5-012
  Files: qa/manual/results/p4-f5-wayland.md
  Tests: every step recorded incl. GNOME, mutter, xdg-desktop-portal versions and the GNOME-box integration test output
  DoD: results committed by the parent `docs(qa): P4 F5 results [FEAT-113]`

- [ ] [P4-F5-013] F5 code review [FEAT-113, FEAT-114, FEAT-115] [tier:sonnet why:review gate] [owner:F5] [brief:briefs/streams/p4-f5.md#item-p4-f5-013]
  Needs: P4-F5-002 .. P4-F5-012
  Files: none (findings become appended fix items)
  Tests: `code-reviewer` over the merged F5 diff
  DoD: no open finding

- [ ] [P4-F5-014] F5 security review [FEAT-113, FEAT-114, FEAT-115] [tier:sonnet why:security review gate] [owner:F5] [brief:briefs/streams/p4-f5.md#item-p4-f5-014]
  Needs: P4-F5-013
  Files: none (findings become appended fix items)
  Tests: qa/security-review.md sections 5-7 plus the brief's F5 focus list (Wayland message bounds, fd handling, URI decoding, staging, clipboard only inside sessions)
  DoD: no open finding

- [ ] [P4-F5-015] F5 Opus consolidation and review-chain note [FEAT-113, FEAT-114, FEAT-115] [tier:opus] [owner:PARENT] [brief:briefs/streams/p4-f5.md#item-p4-f5-015]
  Needs: P4-F5-014, P4-F5-M01
  Files: briefs/plan-phase-4.md (append `<!-- NOTE: <date> - stream F5 review chain green -->`), risk-register.md (status notes R-09, R-20, R-27, R-29, R-30)
  Tests: consistency with F4 and the coordinator
  DoD: commit `chore(plan): F5 review chain green [FEAT-113]`

### P4-X - Phase exit

- [ ] [P4-X-001] P4 exit: verify exit criteria and flip the phase marker [FEAT-099, FEAT-116] [tier:opus] [owner:PARENT] [brief:briefs/streams/p4-f.md#item-p4-x-001]
  Needs: P4-F-023, P4-F1-012, P4-F2-016, P4-F3-015, P4-F4-016, P4-F5-015
  Files: plan.md (append a dated NOTE containing the new `Current phase:` marker line listing every active phase), CLAUDE.md (mirror the marker), briefs/plan-phase-4.md (exit NOTE)
  Tests: plan.md P4 exit criteria met: FILE CONTRACT FREEZE note present; F1-M01, F2-M01, F5-M01 pass; F3-M01 and F4-M01 CI green; every security review green; FEAT-099 .. FEAT-116 each have only `[x]` items or items closed by a superseding NOTE; PB-11 interim, PB-25 and PB-17 rows recorded; `task ci:local` green
  DoD: commit `chore(plan): P4 exit [FEAT-099]`
