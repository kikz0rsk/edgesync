# x11gen - XInput2 and XKB binding generation assessment (P0-SP3-005)

Item P0-SP3-005 (FEAT-177, FEAT-063). Decisions under test: D-39 ("XInput2/XKB only if SP3 shows they are needed,
generated with `xgbgen` from MIT-licensed xcb-proto XML"), D-27 (no GPL code), D-92 (X11 observation through RECORD,
"XInput2 bindings are not generated") and risk R-16 (jezek/xgb has no XInput2/XKB). The deliverable is this
assessment. No code in this directory is linked into any package: there is no `xinput/` output and no
`TestXI2_RawMotionOnXvfb`, because generation fails (item alternative: "the README records the exact generation
failure"), and because xgb cannot deliver XI2 events even from a perfect binding (finding 4).

## Verdict

- `xgbgen` v1.3.1 cannot generate XInput2 or XKB from xcb-proto 1.17.0. Unmodified, it panics on `xinput.xml` and on
  `xkb.xml` (exact errors below). The module's own `xgbgen/doc.go` says "XKB does not work".
- After removing the elements it rejects, it generates compilable request/reply code for XI2 (11,830 lines) but that
  subset contains none of the events P2-X1 needs (all raw events are among the removed elements), and the XI2 and XKB
  event code it emits for the rest is wrong at runtime.
- Independent of generation, `jezek/xgb` v1.3.1 reads every event as exactly 32 bytes. XI2 events are GenericEvents
  (type 35) whose payload extends past 32 bytes, so the connection desynchronises on the first raw event.
- Recommendation: do not generate. Hand-bind the narrow set P2-X1 needs, wire format in-house, golden-byte tests, over
  an in-house minimal X11 connection for the XI2 event stream (details and costs below). D-39's premise that XI2/XKB
  come from `xgbgen` does not hold and needs a user decision (section "Decision impact").

## Sources (primary)

- xcb-proto, https://gitlab.freedesktop.org/xorg/proto/xcbproto , tag `xcb-proto-1.17.0` (annotated tag object
  `9e213c72b819b6a7c1ac060220be93d3cf94cfe1`, commit `77d7fc04da729ddc5ed4aacf30253726fac24dca`, committed
  2024-04-12; the newest release tag on the remote on 2026-10-30). Files used: `src/xinput.xml` (XInputExtension, XI
  2.4), `src/xkb.xml` (XKEYBOARD 1.0), `src/xproto.xml` (event 35 `GeGeneric`, `xge="true"`), `NEWS` (release 1.12:
  "Define new switch-case construct"; release 1.13: "xge: Add safe generic-event sending support"),
  `doc/xml-xcb.txt` (element reference).
- Licence of xcb-proto: MIT-style permission notice, `COPYING` "Copyright (C) 2001-2006 Bart Massey, Jamey Sharp, and
  Josh Triplett", with the extra clause that the authors' names may not be used in advertising. `xinput.xml` carries
  the same notice for "Copyright (C) 2006 Peter Hutterer, Copyright (C) 2013 Daniel Martin"; `xkb.xml` for
  "Copyright (C) 2009 Open Text Corporation". The XML files are MIT-licensed, so generating from them is allowed
  (D-27) provided the notices are kept (`third_party/` per D-27).
- Generator: `github.com/jezek/xgb` v1.3.1 (the version pinned in `spikes/go.mod`), directory `xgbgen`. Its
  `COPYING` is WTFPL v2 (no conditions); the rest of the module is BSD-style (`LICENSE`), as recorded by the P0-SP3-004
  README. Module cache path read: `jezek/xgb@v1.3.1/xgbgen/*.go`, `xgb.go`, `conn.go`, `xproto/xproto.go`.
- Protocol background for the wire facts quoted below: XI2proto.txt (raw events are delivered to clients that
  selected them on the root window, also while another client holds a grab, XI 2.1 and later; quoted in
  `briefs/research-platform-internals.md` section 1.3) and the XKB protocol specification. Neither was re-fetched on
  this host; layouts were read from the xcb-proto XML, which is generated from and checked against them.
- No code was read from or taken from GPL projects (go-x11-client, Deskflow, lan-mouse, Input Leap, Barrier).

## Reproduce

Needs `bash`, `git`, `awk` and `go` on PATH (the script puts `$(go env GOROOT)/bin` first because `xgbgen` shells
out to `gofmt`). From the repository root, on any host (nothing is written inside the repository; the work directory
defaults to `${TMPDIR:-/tmp}/edgesync-x11gen`):

```
bash spikes/linux/x11gen/gen.sh strict
bash spikes/linux/x11gen/gen.sh subset
```

`strict` clones xcb-proto at the pinned tag (the script fails if the commit differs), builds `xgbgen` with
`go -C spikes build github.com/jezek/xgb/xgbgen`, and runs it on the unmodified `xinput.xml` and `xkb.xml`. It exits 1
when the generator rejects a file. `subset` deletes the elements listed in the script (lists `xinput_drop`,
`xkb_drop`), generates `xinput/xinput.go` and `xkb/xkb.go`, creates a scratch module that requires the xgb version from
`spikes/go.mod`, and runs `GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go vet ./...` on both.

Paste back: the full output of both commands. Expected: `strict` prints the two FAIL lines below and exits 1;
`subset` prints the two line counts and `subset generated and vetted in ...`. No X server is involved.

## Findings

### 1. Unmodified generation fails for both files

Observed with `gen.sh strict` (Windows host, Go 1.27.3, xgb v1.3.1):

```
FAIL xinput: Unrecognized tag 'case' in expression context. Expected one of op, fieldref, value, bit, enumref, unop, sumof or popcount.
FAIL xkb: panic: switch field's expression not a fieldref
```

- xinput: `xgbgen/translation.go:317` (called from `XMLField.Translate`, `XMLStruct.Translate`). `xinput.xml` has 12
  `<switch>` elements with 50 `<case>` children (InputInfo, FeedbackState, FeedbackCtl, InputState, DeviceState,
  DeviceCtl, HierarchyChange, DeviceClass, and the `items` switch of ChangeDeviceProperty, GetDeviceProperty,
  XIChangeProperty, XIGetProperty). `XMLField` models only `<bitcase>` children (`xml_fields.go:30`); the `<case>`
  construct was added to xcb-proto in release 1.12 (2016), after xgbgen was written. `XIQueryDevice` (needed to know
  the devices) depends on `DeviceClass`, which uses it.
- xkb: `xgbgen/field.go:381`, raised from `Request.Initialize` for `SelectEvents`: the switch expression is an `<op>`
  (`affectWhich & ~clear & ~selectAll`), xgbgen accepts only a plain `<fieldref>`. `SelectEvents` is the request that
  enables StateNotify/NewKeyboardNotify delivery, so XKB state tracking is impossible without it.
- `xgbgen/doc.go` lines 65-71: "XKB does not work ... XKB poses several extremely difficult problems that XCB also
  has trouble with"; the module `Makefile` generates every extension "EXCEPT XKB".

### 2. What the generator can do once rejected elements are removed

Peeling the failures one layer at a time (scratch copies of the XML, each step re-run through unmodified `xgbgen`):

| Step | Result |
|---|---|
| xinput: drop the 8 switch structs, the 4 switch requests, `DeviceTimeCoord` and `EventForSend` with their dependants (`DeviceTimeCoord` uses `<paramref>`: "Unrecognized tag 'paramref' in expression context") | generation succeeds, 13,302 lines |
| ... compile that | 9 errors, all `undefined: sum` (`<sumof>` over a popcount of `<listelement-ref>`: xgbgen emits a call to a `sum` helper that xgb does not define, and ignores the popcount child, so the semantics would be wrong even if it compiled). Sites: the KeyPress, ButtonPress, TouchBegin, RawKeyPress, RawButtonPress and RawTouchBegin decoders (their copies share them) |
| xinput: also drop those events and everything depending on them (47 elements in `gen.sh`) | 11,830 lines, 142 structs, 609 functions, `go vet` clean for linux/amd64. Contains no raw events, no `XIQueryDevice`, no `DeviceChanged` |
| xkb: drop `SelectEvents` (the op switch) | generation succeeds, 12,213 lines, 203 compile errors (115 `cannot convert xgb.Get64`, 60 `cannot convert xgb.Get16`: the `Action` and `Behavior` unions emit conversions between unrelated struct and integer types). Dropping all 6 switch requests (`SelectEvents`, `GetMap`, `SetMap`, `GetNames`, `SetNames`, `GetKbdByName`) gives 11,401 lines and the same 203 errors |
| xkb: also drop the unions and their dependants (`Action`, `Behavior`, `SetBehavior`, `GetDeviceInfo`, `SetDeviceInfo`) | 7,753 lines, 18 compile errors: `xgb.PopCount` called with a `uint32` or `byte` field (it takes `int`) and `int()` applied to a `bool` field (`DeviceLedInfo`, `KeyType`, `SetKeyType`, `GetCompatMap`, `GetIndicatorMap`, `SetCompatMap`, `SetIndicatorMap`) |
| xkb: also drop those 7 | 7,000 lines, 74 structs, `go vet` clean. Contains 12 of 26 requests (`UseExtension`, `Bell`, `GetState`, `LatchLockState`, `GetControls`, `SetControls`, `GetIndicatorState`, `GetNamedIndicator`, `SetNamedIndicator`, `PerClientFlags`, `ListComponents`, `SetDebuggingFlags`) and the 12 event types, but not `SelectEvents` |

Quality of what does compile (read from the generated XI2 subset): idiomatic xgb style (cookie types, checked and
unchecked request variants, `Read`/`Bytes`/`ListSize` per struct). `XIQueryVersion` is 104 lines, `XISelectEvents` 68
lines plus 75 lines for `EventMask`, `XIGrabDevice` 129 lines; the wire layout of `XIQueryVersion` and
`XISelectEvents` matches the xcb-proto XML (opcode 47 and 46, `num_mask`, per-mask `mask_len` words). Reply decoding
does no length checks (a short buffer panics on slice bounds). Request functions panic when `Init` was not called
(same as the existing xtest/randr/xfixes packages). Only requests with fixed or simple counted layouts are usable.

### 3. Code that compiles but is wrong at runtime

- XI2 events (`xge="true"` in the XML, 14 events plus their copies): `xgbgen` has no handling for the attribute
  (no `xge` string anywhere in `xgbgen/`). A GenericEvent has the layout type(35), extension opcode, sequence,
  length, evtype, then the body. The generated `HierarchyEventNew` instead reads a 2-byte `deviceid` at byte 1 and the
  sequence at byte 3. Registration is `NewExtEventFuncs["XInputExtension"][<xml number>]`, which `Init` copies to
  `NewEventFuncs[FirstEvent + number]`. XI2 events never arrive under those codes, and the numbers collide with the XI
  1.x events: index 7 is both `DeviceFocusOut` and `Enter`, 8 both `ProximityIn` and `Leave`, 9 both `ProximityOut`
  and `FocusIn`, 10 both `DeviceStateNotify` and `FocusOut`, 11 both `DeviceMappingNotify` and `Hierarchy`, 12 both
  `ChangeDeviceNotify` and `Property`; the last `init` wins.
- XKB events: all 12 XKB events share one event code (`FirstEvent`) and are told apart by the `xkbType` byte. The
  generated `Init` registers them at `FirstEvent + xkbType` (indices 0..11 in `xkb.go`). The server only sends
  `FirstEvent`, so every XKB event is decoded as `NewKeyboardNotify`, and the indices above 0 shadow other
  extensions' event codes (each extension's `FirstEvent` is dynamic).
- xproto's own `GeGenericEventNew` (xgb v1.3.1, `xproto/xproto.go:2746`) reads nothing: the type carries only a
  `Sequence` field, never filled.

### 4. xgb cannot deliver XI2 events at all

`Conn.readResponses` (`xgb.go:490-565`) reads 32 bytes per message, extends the read only for replies (`size*4`
extra bytes), and for events calls `NewEventFuncs[buf[0]&127]` on the 32-byte buffer. It never reads the GenericEvent
`length` field. XI2 raw events carry a valuator mask and two arrays of 8-byte FP3232 values after the 32-byte header
(`RawMotion`: `valuators_len` words of mask, then `popcount(mask)` transformed and `popcount(mask)` raw values), so
`length` is never zero for them. After the first raw event, xgb would parse the payload bytes as events and the
stream is out of sync. This cannot be fixed by regenerating: it needs a change in `xgb.go` (a fork or vendored patch)
or an XI2 stream that does not go through `xgb.Conn`.

`xgb.NewConnNet` does exist, but with a plain `net.Conn` it calls `readAuthority(c.host, c.display)` on empty values
(`conn.go:56-58`), logs "Trying connection without authority info" and connects unauthenticated; authenticated
servers need `NewConnNetWithCookieHex`, so the caller must find the cookie itself. A reframing `net.Conn` shim would
need the same Xauthority code plus a framing parser, and still depends on xgb internals.

Not verified on a live server: no X connection was opened (rule of this item), so the desynchronisation is concluded
from the source above, not observed. The XI2 `RawMotion`-during-a-grab behaviour is from the XI2 protocol text quoted
in `briefs/research-platform-internals.md` and is not tested here. P0-SP3-M02 can confirm both on Xvfb once a
hand-bound prototype exists.

## Quantification for P2-X1

Protocol size, in lines of xcb-proto XML including dependencies (counted from the pinned tag):

| Needed by P2-X1 | XML lines | Notes |
|---|---|---|
| XI2: `XIQueryVersion`, `XISelectEvents`, `EventMask`, `RawKeyPress`/`RawButtonPress` (+ release/motion copies), `FP3232` | 87 | requests are 8 and 12+ bytes; events need `popcount` list sizing |
| XI2: `XIGrabDevice`, `XIUngrabDevice` (only if XI2 grabs replace core grabs) | 24 | not needed: core `GrabPointer` keeps working and raw events still arrive |
| XI2: `XIQueryDevice` + `XIDeviceInfo` + `DeviceClass` (device list, valuator axes, scroll classes) | 103 | `DeviceClass` is a 6-way variable-length union |
| XKB: `UseExtension`, `GetState`, `SelectEvents`, `StateNotify`, `NewKeyboardNotify` | 143 | `SelectEvents` is a 72-line switch on a flag expression |
| XKB: `GetMap` with its closure (KeyType, KeySymMap, Behavior and Action unions, 15 action structs, ...) | 341 | 8 bit-selected sections, nested lists of lists |

Estimated cost, in Go lines and person-days (estimates, not measurements; they assume a developer who has read this
README):

| Approach | Production Go | Tests | Effort | Result |
|---|---|---|---|---|
| A. Repair `xgbgen` (add `<case>`, XGE events and their registration, `sumof`/`popcount`/`listelement-ref`, `paramref`, op-switch, union and popcount typing, XKB event registration) and patch the xgb reader | 1,500-2,500 changed generator lines, then 20k+ generated | generator golden tests | 3-5 weeks | generated XI2 and XKB; owns a fork of `xgbgen` and `xgb.go` |
| B. Generate only the supported subset (`gen.sh subset`) and hand-write the events and transport | 11,830 + 7,000 generated, plus the hand-written part of C | as C | about as C | generated lines unused except 3 requests (about 250 lines); no gain |
| C. Hand-bind the P2-X1 set, in-house transport for the XI2 stream | XI2 about 450 (version, select events, raw decoder, delta extraction, GE reader, connection setup with Xauthority); XKB about 250 (UseExtension, GetState, SelectEvents for state and new keyboard, two event decoders and the FirstEvent+xkbType dispatcher); skip GetMap | about 600 golden-byte tests | 5-8 days | exactly the needed protocol, no dependency change |
| D. No XI2: keep warp-to-centre deltas (SP3-004) and RECORD (D-92); XKB via `_XKB_RULES_NAMES` root property and xproto `GetKeyboardMapping` | 0 | 0 | 0 | accelerated deltas, no group-change events |

The generator saves nothing for the requests (XIQueryVersion + XISelectEvents + EventMask is 247 generated lines,
about 70 hand-written) and fails exactly on the pieces that are hard (events, unions, lists of lists).

## Recommendation

Hand-bind (approach C), narrowly, and only when P2-X1 needs it:

1. XI2 raw events (`RawMotion`, `RawButtonPress/Release`, `RawKeyPress/Release`) for unaccelerated deltas and device
   ids; requests `XIQueryVersion` (require 2.1 or later) and `XISelectEvents` on the root window. Use a dedicated
   in-house X11 connection for the event stream (setup handshake, Xauthority cookie, request/reply and GenericEvent
   framing). D-93 already adds a second `xgb` connection for RECORD, so a separate event connection is the same shape.
   Do not patch or fork `xgb`: a fork changes the pinned dependency and has to be rebased on every release.
2. XKB: `UseExtension`, `GetState` and `SelectEvents` limited to `StateNotify`/`NewKeyboardNotify`, decoded through a
   single dispatcher on the `xkbType` byte, on the normal `xgb` connection. Do not bind `GetMap`/`GetNames`: layout
   names come from the `_XKB_RULES_NAMES` root property and keycode to keysym tables from xproto `GetKeyboardMapping`.
3. Tests are golden byte frames written from the protocol layout (pure host tests, no X server), plus Xvfb
   integration tests under `//go:build integration && linux` for the live checks that this assessment could not run.

Reasons: the generator cannot produce the events, which are the point of XI2; the xgb reader cannot receive them
anyway; the hand-bound set is a few hundred lines of fixed layouts with stable protocols (XI 2.4 and XKB 1.0 have not
changed in years), which is cheaper than owning a generator fork.

Licence and maintenance consequences:

- Hand-binding writes the layouts from the protocol specification, so no third-party text enters the repository.
  Cite XI2proto.txt, the XKB specification and xcb-proto 1.17.0 in the package documentation. If any XML-derived text
  is ever copied (field names, constants), keep the xcb-proto copyright notices in `third_party/` (D-27).
- Generation would add MIT-derived generated code (notices required), a WTFPL generator fork (no obligation, but ours
  to maintain) and a BSD xgb fork (notice required, rebase burden). No GPL source is involved in either route.
- Maintenance of C: the two protocols are frozen in practice; the risk is server behaviour differences (Xvfb, Xorg,
  XWayland), covered by the Xvfb integration tests and by reading the version replies.

## Decision impact

- D-39 states XInput2/XKB "generated with `xgbgen`". This assessment shows that route is not viable. If P2-X1 needs
  XI2 or XKB beyond D-92 (RECORD and warp-to-centre), the plan needs a decision to hand-bind them (approach C) or to
  stay with approach D.
- D-92 is unaffected: RECORD and `GrabKey` come from `jezek/xgb` without generation.
- R-16 mitigation "generated XI2 per SP3-005" should read "hand-bound XI2 per SP3-005 or warp-to-centre".
