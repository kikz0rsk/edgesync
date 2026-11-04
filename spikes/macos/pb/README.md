# macOS pasteboard spike (P0-SP4-004)

Purpose: prove, from Go without cgo, the macOS mechanisms behind D-10 and D-11 (lazy clipboard rendering and file promises) and the D-86 polling rule. The package registers Objective-C classes through `purego/objc` and drives `NSPasteboard`:

- `pasteboard_darwin.go`: `NSPasteboard generalPasteboard`, `changeCount` polling at 500 ms (D-86, reads the counter only), `clearContents`, `types`, `dataForType:`, `stringForType:`, `setData:forType:`, `writeObjects:`, detection of `org.nspasteboard.ConcealedType`.
- `provider_darwin.go`: a class registered with `objc.RegisterClass` that conforms to `NSPasteboardItemDataProvider` (`pasteboard:item:provideDataForType:` calls `setData:forType:` on the item with Go data, `pasteboardFinishedWithDataProvider:` is counted); an `NSPasteboardItem` gets the provider for `public.utf8-plain-text`, `public.html`, `public.rtf`, `public.png` and is written with `writeObjects:`.
- `promise_darwin.go`: a class conforming to `NSFilePromiseProviderDelegate` (`filePromiseProvider:fileNameForType:`, `filePromiseProvider:writePromiseToURL:completionHandler:`), an `NSFilePromiseProvider` with file type `public.data`, direct fulfilment by sending the delegate messages from Go, and fulfilment through `NSFilePromiseReceiver` read back from the pasteboard.

D-121: the spike detects the concealed marker only to record that the type is visible; the product shares concealed content like any other.

## Build

Cross-compile check from the Windows host (no run):

    GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go -C spikes vet ./macos/...
    GOOS=darwin GOARCH=amd64 CGO_ENABLED=0 go -C spikes vet ./macos/...
    GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go -C spikes vet -tags integration ./macos/...

On a Mac, from the repository root:

    cd spikes && CGO_ENABLED=0 go build -o bin/pb ./macos/cmd/pb

## Run

    spikes/bin/pb -mode status
    spikes/bin/pb -mode provider
    spikes/bin/pb -mode promise
    spikes/bin/pb -mode watch -duration 30s

`status` prints the change count, the type list and whether the concealed marker is present. `provider` writes the lazy item, prints the provider call count per type after the write and after each of two reads, then clears the pasteboard and prints the `pasteboardFinishedWithDataProvider:` count. `promise` fulfils the promise directly into a temp directory, writes the promise to the pasteboard and asks `NSFilePromiseReceiver` to fulfil it. `watch` prints every change count change with the type list (copy something in another app while it runs; copy from a password manager to see `concealed=true`).

## Tests

Integration tests (CI `macos` job, `-tags integration`):

    cd spikes && CGO_ENABLED=0 go test -tags integration -v ./macos/pb/

- `TestAppKitTypes_MatchLiterals`: the `NSPasteboardType*` constants read from AppKit equal the spike's `public.*` literals.
- `TestPasteboard_ChangeCountPolling`: `clearContents` raises the count and the 20 ms poller reports the new value.
- `TestProvider_LazyTextHTMLRTFPNG`: provider called 0 times right after `writeObjects:`, exactly once per type after the first in-process read, content equal to what Go provided; the second-read call count is logged as an observed fact.
- `TestConcealedType_Detected`: the marker is found when present and absent otherwise.
- `TestPromise_ProviderRegistersAndWritesFile`: the delegate class registers, answers `fileNameForType:`, writes the file when its write message is sent with a completion block; the `NSFilePromiseReceiver` path is attempted and its outcome logged.

## What to observe and paste back

The full `go test -v` output and the output of the four `pb` modes. Look for these facts in the log:

- whether the provider is invoked at write time (expected: no) and how many times per type (expected: once);
- the `types on the pasteboard after the write` list (AppKit may add synthesised types);
- whether a read blocks without a run loop (the test fails with "no answer within 10s" if it does);
- the `pasteboardFinishedWithDataProvider:` call count after `clearContents`;
- the line `NSFilePromiseProviderDelegate conformance: declared|omitted`: the first CI run showed `objc_getProtocol` returning nil for this protocol (Apple: `NSProtocolFromString` returns nil if no protocol of that name is currently loaded), so `NewPromise` now asks `NSProtocolFromString` and registers the delegate class without a conformance declaration when that is nil too (the selectors are implemented either way); `omitted` means the protocol was still not loaded;
- the line `NSFilePromiseReceiver fulfilled the promise in-process` or `OBSERVED: in-process NSFilePromiseReceiver path did not fulfil the promise`.

## Evidence level

- Selectors and protocols: verified against Apple's documentation (developer.apple.com/tutorials/data, 2026-10-30): `NSPasteboardItemDataProvider` (`pasteboard:item:provideDataForType:` required, `pasteboardFinishedWithDataProvider:`), `NSFilePromiseProviderDelegate` (`filePromiseProvider:fileNameForType:`, `filePromiseProvider:writePromiseToURL:completionHandler:` with `void (^)(NSError * _Nullable)`, optional `operationQueueForFilePromiseProvider:` not implemented), `NSPasteboardItem setDataProvider:forTypes:`, `NSFilePromiseProvider initWithFileType:delegate:`, `NSFilePromiseReceiver receivePromisedFilesAtDestination:options:operationQueue:reader:`, `NSPasteboard` members. Apple's pages for several members show only Swift signatures; the Objective-C argument types are reconstructed from those and from the selector names.
- UTI strings `public.utf8-plain-text`, `public.html`, `public.rtf`, `public.png`, `public.file-url`, `public.data`: Apple's archived "System-Declared Uniform Type Identifiers" reference. The current `NSPasteboardType*` pages do not state the strings, so `TestAppKitTypes_MatchLiterals` cross-checks them against the AppKit symbols at run time. golang-design/clipboard v0.11.0 (MIT) uses the same strings.
- `org.nspasteboard.ConcealedType`: second-source, the nspasteboard.org community convention (not an Apple API).
- The completion handler is a block created by AppKit, which `objc.Block.Invoke` cannot call (it only knows blocks made by `objc.NewBlock`), so the spike calls the block's `invoke` pointer at offset 16 (Clang Block ABI `Block_literal_1`, same layout as purego's `blockLayout`).
- Host: compile-only (cross vet and build for darwin/arm64 and darwin/amd64, with and without `-tags integration`).
- Runtime: unverified until the CI `macos` job runs the integration tests.

## Cannot be verified without a Mac (or interactively)

- Finder (or any other application) pasting a promise from the general pasteboard: UNVERIFIED (RPI 6c). Apple documents promises for drag and drop; paste support is not documented, and no CI step can paste into Finder.
- Whether the in-process `NSFilePromiseReceiver` path fulfils a promise written to the general pasteboard: only the CI log can say; it is attempted but not asserted.
- Provider requests from a different process (a real paste in another application) and their threading: the tests read in-process, where AppKit may call the provider directly. Cross-process requests may need the owning thread's run loop to be serviced, which this spike does not provide.
- Whether macOS 15.4+ shows the pasteboard access alert for programmatic reads (R-15): the CI runner has no user to observe it. `Types()` and `ChangeCount()` read no data by design.
- Callbacks arriving on threads Go did not create (GCD workers used by `NSFilePromiseReceiver`'s operation queue): exercised by the receiver path only if AppKit fulfils the promise.
