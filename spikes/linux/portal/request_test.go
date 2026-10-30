package portal

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/godbus/dbus/v5"
)

const (
	testUnique    = ":1.42"
	testWait      = 2 * time.Second
	nameResponse  = interfaceRequest + "." + signalResponse
	nameActivated = interfaceInputCapture + ".Activated"
	nameDeactiv   = interfaceInputCapture + ".Deactivated"
	nameClosed    = interfaceSession + "." + signalSessionClose
)

type fakeCall struct {
	Path   dbus.ObjectPath
	Method string
	Args   []any
}

type fakeSub struct {
	match SignalMatch
	ch    chan Signal
	bus   *fakeBus
}

func (s *fakeSub) Signals() <-chan Signal { return s.ch }

func (s *fakeSub) Close() error {
	s.bus.mu.Lock()
	defer s.bus.mu.Unlock()
	s.bus.log = append(s.bus.log, "unsubscribe "+string(s.match.Path))
	s.bus.subs = slices.DeleteFunc(s.bus.subs, func(o *fakeSub) bool { return o == s })
	return nil
}

type fakeBus struct {
	t       *testing.T
	unique  string
	handler func(fakeCall) ([]any, []Signal, error)

	mu    sync.Mutex
	log   []string
	calls []fakeCall
	props map[string]dbus.Variant
	subs  []*fakeSub
}

func newFakeBus(t *testing.T) *fakeBus {
	t.Helper()
	return &fakeBus{t: t, unique: testUnique, props: map[string]dbus.Variant{}}
}

func (b *fakeBus) UniqueName() string { return b.unique }

func (b *fakeBus) Call(_ context.Context, path dbus.ObjectPath, method string, args ...any) ([]any, error) {
	call := fakeCall{Path: path, Method: method, Args: args}
	b.mu.Lock()
	b.log = append(b.log, "call "+method)
	b.calls = append(b.calls, call)
	b.mu.Unlock()
	if b.handler == nil {
		return nil, fmt.Errorf("fake bus: no handler for %s", method)
	}
	ret, emit, err := b.handler(call)
	if err != nil {
		return nil, err
	}
	for _, sig := range emit {
		b.emit(sig)
	}
	return ret, nil
}

func (b *fakeBus) CallFD(ctx context.Context, path dbus.ObjectPath, method string, args ...any) (*os.File, error) {
	ret, err := b.Call(ctx, path, method, args...)
	if err != nil {
		return nil, err
	}
	if len(ret) != 1 {
		return nil, fmt.Errorf("fake bus: %s returned %d values", method, len(ret))
	}
	f, ok := ret[0].(*os.File)
	if !ok {
		return nil, fmt.Errorf("fake bus: %s returned %T, want *os.File", method, ret[0])
	}
	return f, nil
}

func (b *fakeBus) Property(_ context.Context, _ dbus.ObjectPath, iface, name string) (dbus.Variant, error) {
	v, ok := b.props[iface+"."+name]
	if !ok {
		return dbus.Variant{}, fmt.Errorf("fake bus: no property %s.%s", iface, name)
	}
	return v, nil
}

func (b *fakeBus) Subscribe(_ context.Context, match SignalMatch) (Subscription, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.log = append(b.log, fmt.Sprintf("subscribe %s %s.%s", match.Path, match.Interface, match.Member))
	sub := &fakeSub{match: match, ch: make(chan Signal, 32), bus: b}
	b.subs = append(b.subs, sub)
	return sub, nil
}

func (b *fakeBus) emit(sig Signal) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, sub := range b.subs {
		if !sub.match.Matches(sig) {
			continue
		}
		select {
		case sub.ch <- sig:
		default:
			b.t.Errorf("fake bus: subscriber for %s is full", sub.match.Path)
		}
	}
}

func (b *fakeBus) callsTo(method string) []fakeCall {
	b.mu.Lock()
	defer b.mu.Unlock()
	var out []fakeCall
	for _, c := range b.calls {
		if c.Method == method {
			out = append(out, c)
		}
	}
	return out
}

func (b *fakeBus) methods() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]string, 0, len(b.calls))
	for _, c := range b.calls {
		out = append(out, c.Method)
	}
	return out
}

func (b *fakeBus) logCopy() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return slices.Clone(b.log)
}

func sequentialTokens(prefix string) TokenSource {
	var mu sync.Mutex
	n := 0
	return func() (string, error) {
		mu.Lock()
		defer mu.Unlock()
		n++
		return fmt.Sprintf("%s%d", prefix, n), nil
	}
}

func optionsOf(t *testing.T, v any) Options {
	t.Helper()
	o, ok := v.(map[string]dbus.Variant)
	if !ok {
		t.Fatalf("argument is %T, want a{sv}", v)
	}
	return o
}

func uintOption(t *testing.T, o Options, key string) uint32 {
	t.Helper()
	v, ok := o[key]
	if !ok {
		t.Fatalf("option %q missing in %v", key, o)
	}
	u, ok := v.Value().(uint32)
	if !ok {
		t.Fatalf("option %q is %T, want uint32", key, v.Value())
	}
	return u
}

func stringOption(t *testing.T, o Options, key string) string {
	t.Helper()
	v, ok := o[key]
	if !ok {
		t.Fatalf("option %q missing in %v", key, o)
	}
	s, ok := v.Value().(string)
	if !ok {
		t.Fatalf("option %q is %T, want string", key, v.Value())
	}
	return s
}

type fakePortal struct {
	t   *testing.T
	bus *fakeBus

	zones         []Zone
	zoneSet       uint32
	denyBarriers  bool
	startCode     uint32
	startCaps     uint32
	startDevices  uint32
	handleAsOther bool
	restoreToken  string
	stringSession bool

	sessionPath  dbus.ObjectPath
	eisConnected bool
	enabled      bool
	barriers     []Barrier
	closed       bool
	eisFiles     []*os.File
}

func newFakePortal(t *testing.T, bus *fakeBus, version uint32) *fakePortal {
	t.Helper()
	p := &fakePortal{
		t:            t,
		bus:          bus,
		zones:        []Zone{{Width: 1920, Height: 1080, X: 0, Y: 0}},
		zoneSet:      7,
		startCaps:    uint32(CapabilityKeyboard | CapabilityPointer),
		startDevices: uint32(DeviceKeyboard | DevicePointer),
		restoreToken: "new-token",
	}
	bus.props[interfaceInputCapture+".version"] = dbus.MakeVariant(version)
	bus.props[interfaceRemoteDesktop+".version"] = dbus.MakeVariant(version)
	bus.props[interfaceInputCapture+".SupportedCapabilities"] = dbus.MakeVariant(uint32(3))
	bus.props[interfaceRemoteDesktop+".AvailableDeviceTypes"] = dbus.MakeVariant(uint32(3))
	bus.handler = p.handle
	t.Cleanup(func() {
		for _, f := range p.eisFiles {
			_ = f.Close()
		}
	})
	return p
}

func (p *fakePortal) respond(opts Options, code uint32, results Options) ([]any, []Signal, error) {
	token := stringOption(p.t, opts, optionHandleToken)
	path, err := RequestPath(p.bus.unique, token)
	if err != nil {
		return nil, nil, err
	}
	returned := path
	if p.handleAsOther {
		returned = path + "x"
	}
	sig := Signal{Path: path, Name: nameResponse, Body: []any{code, results}}
	return []any{returned}, []Signal{sig}, nil
}

func (p *fakePortal) newSessionPath(opts Options) dbus.ObjectPath {
	path, err := SessionPath(p.bus.unique, stringOption(p.t, opts, optionSessionToken))
	if err != nil {
		p.t.Fatalf("session path: %v", err)
	}
	p.sessionPath = path
	return path
}

func (p *fakePortal) openFile() []any {
	r, w, err := os.Pipe()
	if err != nil {
		p.t.Fatalf("pipe: %v", err)
	}
	p.eisFiles = append(p.eisFiles, w)
	return []any{r}
}

func zonesVariant(zones []Zone) dbus.Variant {
	rows := make([][]any, 0, len(zones))
	for _, z := range zones {
		rows = append(rows, []any{z.Width, z.Height, z.X, z.Y})
	}
	return dbus.MakeVariantWithSignature(rows, dbus.ParseSignatureMust("a(uuii)"))
}

func (p *fakePortal) handle(c fakeCall) ([]any, []Signal, error) {
	switch c.Method {
	case methodICCreateSession2:
		path := p.newSessionPath(optionsOf(p.t, c.Args[0]))
		return []any{map[string]dbus.Variant{"session_handle": dbus.MakeVariant(path)}}, nil, nil
	case methodICStart:
		opts := optionsOf(p.t, c.Args[2])
		results := Options{
			"capabilities":      dbus.MakeVariant(p.startCaps),
			"clipboard_enabled": dbus.MakeVariant(false),
			"restore_token":     dbus.MakeVariant(p.restoreToken),
		}
		return p.respond(opts, p.startCode, results)
	case methodICCreateSession:
		opts := optionsOf(p.t, c.Args[1])
		path := p.newSessionPath(opts)
		results := Options{
			"session_handle": dbus.MakeVariant(path),
			"capabilities":   dbus.MakeVariant(p.startCaps),
		}
		return p.respond(opts, 0, results)
	case methodICGetZones:
		results := Options{"zone_set": dbus.MakeVariant(p.zoneSet), "zones": zonesVariant(p.zones)}
		return p.respond(optionsOf(p.t, c.Args[1]), 0, results)
	case methodICSetBarriers:
		return p.setBarriers(c)
	case methodICConnectToEIS, methodRDConnectToEIS:
		p.eisConnected = true
		return p.openFile(), nil, nil
	case methodICEnable:
		if !p.eisConnected {
			return nil, nil, errors.New("portal rejected Enable before ConnectToEIS")
		}
		p.enabled = true
		return nil, nil, nil
	case methodICDisable:
		p.enabled = false
		return nil, nil, nil
	case methodICRelease, methodRequestClose:
		return nil, nil, nil
	case methodSessionClose:
		p.closed = true
		return nil, nil, nil
	case methodRDCreateSession:
		opts := optionsOf(p.t, c.Args[0])
		path := p.newSessionPath(opts)
		var handle dbus.Variant
		if p.stringSession {
			handle = dbus.MakeVariant(string(path))
		} else {
			handle = dbus.MakeVariant(path)
		}
		return p.respond(opts, 0, Options{"session_handle": handle})
	case methodRDSelectDevices:
		return p.respond(optionsOf(p.t, c.Args[1]), 0, Options{})
	case methodRDStart:
		results := Options{
			"devices":       dbus.MakeVariant(p.startDevices),
			"restore_token": dbus.MakeVariant(p.restoreToken),
		}
		return p.respond(optionsOf(p.t, c.Args[2]), p.startCode, results)
	default:
		return nil, nil, fmt.Errorf("fake portal: unexpected call %s", c.Method)
	}
}

func (p *fakePortal) setBarriers(c fakeCall) ([]any, []Signal, error) {
	wire, ok := c.Args[2].([]map[string]dbus.Variant)
	if !ok {
		p.t.Fatalf("barriers are %T, want aa{sv}", c.Args[2])
	}
	zoneSet, ok := c.Args[3].(uint32)
	if !ok {
		p.t.Fatalf("zone_set is %T, want uint32", c.Args[3])
	}
	p.enabled = false
	p.barriers = nil
	var failed []uint32
	for _, b := range wire {
		id := uintOption(p.t, b, "barrier_id")
		pos, ok := b["position"].Value().(Position)
		if !ok {
			p.t.Fatalf("position is %T, want (iiii)", b["position"].Value())
		}
		if p.denyBarriers || zoneSet != p.zoneSet {
			failed = append(failed, id)
			continue
		}
		p.barriers = append(p.barriers, Barrier{ID: id, Position: pos})
	}
	results := Options{}
	if len(failed) > 0 {
		results["failed_barriers"] = dbus.MakeVariant(failed)
	}
	return p.respond(optionsOf(p.t, c.Args[1]), 0, results)
}

func (p *fakePortal) emitCapture(name string, session dbus.ObjectPath, opts Options) {
	p.bus.emit(Signal{Path: DesktopPath, Name: name, Body: []any{session, opts}})
}

func newCaller(bus *fakeBus) *Caller {
	return NewCaller(bus, sequentialTokens("tok"))
}

func nextEvent(t *testing.T, s *EventStream) Event {
	t.Helper()
	select {
	case ev, ok := <-s.Events():
		if !ok {
			t.Fatalf("event stream closed: %v", s.Err())
		}
		return ev
	case <-time.After(testWait):
		t.Fatal("timed out waiting for an event")
		return nil
	}
}

func TestRequest_HandleTokenPathDerivation(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		unique  string
		token   string
		want    dbus.ObjectPath
		wantErr error
	}{
		{"typical", ":1.42", "edgesync_1", "/org/freedesktop/portal/desktop/request/1_42/edgesync_1", nil},
		{"multi dot", ":1.7.3", "abc", "/org/freedesktop/portal/desktop/request/1_7_3/abc", nil},
		{"empty token", ":1.42", "", "", ErrInvalidToken},
		{"dash in token", ":1.42", "a-b", "", ErrInvalidToken},
		{"dot in token", ":1.42", "a.b", "", ErrInvalidToken},
		{"empty name", "", "tok", "", ErrNoUniqueName},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := RequestPath(tt.unique, tt.token)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("error = %v, want %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Fatalf("path = %q, want %q", got, tt.want)
			}
		})
	}

	session, err := SessionPath(":1.42", "edgesync_s")
	if err != nil || session != "/org/freedesktop/portal/desktop/session/1_42/edgesync_s" {
		t.Fatalf("SessionPath = %q, %v", session, err)
	}
}

func TestRequest_RandomTokensAreValidAndDistinct(t *testing.T) {
	t.Parallel()
	next := RandomTokens("edgesync")
	seen := map[string]bool{}
	for range 16 {
		token, err := next()
		if err != nil {
			t.Fatalf("token: %v", err)
		}
		if !validElement(token) || !strings.HasPrefix(token, "edgesync_") || seen[token] {
			t.Fatalf("token %q invalid or repeated", token)
		}
		seen[token] = true
	}
}

func TestRequest_ResponseDecoding(t *testing.T) {
	t.Parallel()
	results := map[string]dbus.Variant{"k": dbus.MakeVariant(uint32(9))}
	tests := []struct {
		name    string
		body    []any
		wantErr error
		wantRes error
	}{
		{"success", []any{uint32(0), results}, nil, nil},
		{"cancelled", []any{uint32(1), results}, nil, ErrCancelled},
		{"other", []any{uint32(2), results}, nil, ErrEnded},
		{"unknown code", []any{uint32(3), results}, nil, ErrUnknownResponse},
		{"short body", []any{uint32(0)}, ErrBadResponse, nil},
		{"code type", []any{int32(0), results}, ErrBadResponse, nil},
		{"results type", []any{uint32(0), "x"}, ErrBadResponse, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			resp, err := DecodeResponse(tt.body)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("decode error = %v, want %v", err, tt.wantErr)
			}
			if err != nil {
				return
			}
			if err := resp.Err("Test.Method"); !errors.Is(err, tt.wantRes) {
				t.Fatalf("response error = %v, want %v", err, tt.wantRes)
			}
			if v, err := resp.Results.Uint32("k"); err != nil || v != 9 {
				t.Fatalf("results k = %d, %v", v, err)
			}
		})
	}
}

func TestResults_MissingAndWrongTypeAreDistinct(t *testing.T) {
	t.Parallel()
	r := Results{"n": dbus.MakeVariant("text"), "p": dbus.MakeVariant("/a/b")}
	if _, err := r.Uint32("absent"); !errors.Is(err, ErrMissingResult) {
		t.Fatalf("absent key error = %v, want ErrMissingResult", err)
	}
	if _, err := r.Uint32("n"); !errors.Is(err, ErrResultType) {
		t.Fatalf("wrong type error = %v, want ErrResultType", err)
	}
	if got, err := r.PathOrString("p"); err != nil || got != "/a/b" {
		t.Fatalf("PathOrString = %q, %v", got, err)
	}
	if _, err := (Results{"x": dbus.MakeVariant(uint32(1))}).PathOrString("x"); !errors.Is(err, ErrResultType) {
		t.Fatalf("PathOrString wrong type error = %v", err)
	}
}

func TestSignalMatch_Matches(t *testing.T) {
	t.Parallel()
	sig := Signal{Path: "/p", Name: "a.b.Member"}
	tests := []struct {
		match SignalMatch
		want  bool
	}{
		{SignalMatch{}, true},
		{SignalMatch{Path: "/p"}, true},
		{SignalMatch{Path: "/q"}, false},
		{SignalMatch{Interface: "a.b"}, true},
		{SignalMatch{Interface: "a"}, false},
		{SignalMatch{Interface: "a.b", Member: "Member"}, true},
		{SignalMatch{Interface: "a.b", Member: "Other"}, false},
	}
	for _, tt := range tests {
		if got := tt.match.Matches(sig); got != tt.want {
			t.Errorf("%+v matches = %v, want %v", tt.match, got, tt.want)
		}
	}
}

func TestCaller_RequestSubscribesBeforeCall(t *testing.T) {
	t.Parallel()
	bus := newFakeBus(t)
	newFakePortal(t, bus, 2)
	caller := newCaller(bus)

	resp, err := caller.Request(t.Context(), methodICGetZones, Options{}, func(o Options) []any {
		return []any{dbus.ObjectPath("/s"), o}
	})
	if err != nil {
		t.Fatalf("Request: %v", err)
	}
	if v, err := resp.Results.Uint32("zone_set"); err != nil || v != 7 {
		t.Fatalf("zone_set = %d, %v", v, err)
	}
	log := bus.logCopy()
	want := []string{
		"subscribe /org/freedesktop/portal/desktop/request/1_42/tok1 org.freedesktop.portal.Request.Response",
		"call " + methodICGetZones,
		"unsubscribe /org/freedesktop/portal/desktop/request/1_42/tok1",
	}
	if !slices.Equal(log, want) {
		t.Fatalf("bus log = %q, want %q", log, want)
	}
	call := bus.callsTo(methodICGetZones)[0]
	if got := stringOption(t, optionsOf(t, call.Args[1]), optionHandleToken); got != "tok1" {
		t.Fatalf("handle_token = %q, want tok1", got)
	}
}

func TestCaller_RequestHandleMismatch(t *testing.T) {
	t.Parallel()
	bus := newFakeBus(t)
	p := newFakePortal(t, bus, 2)
	p.handleAsOther = true
	_, err := newCaller(bus).Request(t.Context(), methodICGetZones, Options{}, func(o Options) []any {
		return []any{dbus.ObjectPath("/s"), o}
	})
	if !errors.Is(err, ErrHandleMismatch) {
		t.Fatalf("error = %v, want ErrHandleMismatch", err)
	}
}

func TestCaller_RequestCancelledResponse(t *testing.T) {
	t.Parallel()
	bus := newFakeBus(t)
	p := newFakePortal(t, bus, 2)
	p.startCode = 1
	_, err := NewInputCapture(newCaller(bus)).Open(t.Context(), CaptureParams{Capabilities: CapabilityPointer})
	if !errors.Is(err, ErrCancelled) {
		t.Fatalf("error = %v, want ErrCancelled", err)
	}
	if !p.closed {
		t.Fatal("session created by CreateSession2 was not closed after the failed Start")
	}
}

func TestCaller_RequestContextCancelClosesRequest(t *testing.T) {
	t.Parallel()
	bus := newFakeBus(t)
	bus.handler = func(c fakeCall) ([]any, []Signal, error) {
		if c.Method == methodRequestClose {
			return nil, nil, nil
		}
		token := stringOption(t, optionsOf(t, c.Args[1]), optionHandleToken)
		path, err := RequestPath(bus.unique, token)
		return []any{path}, nil, err
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err := newCaller(bus).Request(ctx, methodICGetZones, Options{}, func(o Options) []any {
		return []any{dbus.ObjectPath("/s"), o}
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
	closes := bus.callsTo(methodRequestClose)
	if len(closes) != 1 || closes[0].Path != "/org/freedesktop/portal/desktop/request/1_42/tok1" {
		t.Fatalf("Request.Close calls = %+v", closes)
	}
}

func TestBarrierFor_OuterEdgeOfZoneUnion(t *testing.T) {
	t.Parallel()
	dual := []Zone{{1920, 1080, 0, 0}, {2560, 1440, 1920, -180}}
	tests := []struct {
		name     string
		zones    []Zone
		edge     Edge
		wantPos  Position
		wantZone Zone
		wantErr  error
	}{
		{"single right", []Zone{{1920, 1080, 0, 0}}, EdgeRight, Position{1920, 0, 1920, 1079}, Zone{1920, 1080, 0, 0}, nil},
		{"single left", []Zone{{1920, 1080, 0, 0}}, EdgeLeft, Position{0, 0, 0, 1079}, Zone{1920, 1080, 0, 0}, nil},
		{"dual right picks rightmost", dual, EdgeRight, Position{4480, -180, 4480, 1259}, dual[1], nil},
		{"dual left picks leftmost", dual, EdgeLeft, Position{0, 0, 0, 1079}, dual[0], nil},
		{"no zones", nil, EdgeRight, Position{}, Zone{}, ErrNoZones},
		{"empty zone", []Zone{{0, 1080, 0, 0}}, EdgeRight, Position{}, Zone{}, ErrNoZones},
		{"unknown edge", dual, Edge(9), Position{}, Zone{}, ErrUnknownEdge},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			b, zone, err := BarrierFor(tt.zones, tt.edge, 3)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("error = %v, want %v", err, tt.wantErr)
			}
			if err != nil {
				return
			}
			if b.ID != 3 || b.Position != tt.wantPos || zone != tt.wantZone {
				t.Fatalf("barrier = %+v zone %+v, want %+v zone %+v", b, zone, tt.wantPos, tt.wantZone)
			}
		})
	}
	if _, _, err := BarrierFor(dual, EdgeRight, 0); !errors.Is(err, ErrBarrierID) {
		t.Fatalf("zero id error = %v, want ErrBarrierID", err)
	}
}

func TestReleasePosition_ClampsIntoZone(t *testing.T) {
	t.Parallel()
	zone := Zone{Width: 1920, Height: 1080, X: 0, Y: 0}
	tests := []struct {
		in, want Point
	}{
		{Point{1925.5, 300}, Point{1919, 300}},
		{Point{-4, 2000}, Point{0, 1079}},
		{Point{10, 20}, Point{10, 20}},
	}
	for _, tt := range tests {
		if got := ReleasePosition(zone, tt.in); got != tt.want {
			t.Errorf("ReleasePosition(%+v) = %+v, want %+v", tt.in, got, tt.want)
		}
	}
}

func TestInputCapture_CaptureLifecycleVersion2(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	bus := newFakeBus(t)
	portal := newFakePortal(t, bus, 2)
	ic := NewInputCapture(newCaller(bus))

	if caps, err := ic.SupportedCapabilities(ctx); err != nil || caps != CapabilityKeyboard|CapabilityPointer {
		t.Fatalf("SupportedCapabilities = %d, %v", caps, err)
	}
	cs, err := ic.Open(ctx, CaptureParams{
		Capabilities: CapabilityKeyboard | CapabilityPointer,
		PersistMode:  PersistUntilRevoked,
		RestoreToken: "old-token",
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if cs.Version != 2 || cs.RestoreToken != "new-token" || cs.Granted != CapabilityKeyboard|CapabilityPointer {
		t.Fatalf("session = %+v", cs)
	}
	if got := bus.methods(); !slices.Equal(got, []string{methodICCreateSession2, methodICStart}) {
		t.Fatalf("open calls = %v, want CreateSession2 then Start", got)
	}
	startOpts := optionsOf(t, bus.callsTo(methodICStart)[0].Args[2])
	if uintOption(t, startOpts, "capabilities") != 3 || uintOption(t, startOpts, "persist_mode") != 2 ||
		stringOption(t, startOpts, "restore_token") != "old-token" {
		t.Fatalf("Start options = %v", startOpts)
	}

	if err := cs.Enable(ctx); !errors.Is(err, ErrEISNotConnected) {
		t.Fatalf("Enable before ConnectToEIS = %v, want ErrEISNotConnected", err)
	}
	if len(bus.callsTo(methodICEnable)) != 0 {
		t.Fatal("Enable reached the portal before ConnectToEIS")
	}

	stream, err := cs.Events(ctx)
	if err != nil {
		t.Fatalf("Events: %v", err)
	}
	t.Cleanup(func() {
		if err := stream.Close(); err != nil {
			t.Errorf("stream close: %v", err)
		}
	})
	eis, err := cs.ConnectToEIS(ctx)
	if err != nil {
		t.Fatalf("ConnectToEIS: %v", err)
	}
	t.Cleanup(func() { _ = eis.Close() })
	if _, err := cs.ConnectToEIS(ctx); !errors.Is(err, ErrEISAlreadyConnected) {
		t.Fatalf("second ConnectToEIS = %v, want ErrEISAlreadyConnected", err)
	}

	armed, err := cs.Arm(ctx, EdgeRight, 1)
	if err != nil {
		t.Fatalf("Arm right: %v", err)
	}
	if armed.Barrier.Position != (Position{1920, 0, 1920, 1079}) || armed.ZoneSet.ID != 7 {
		t.Fatalf("armed = %+v", armed)
	}
	wantOrder := []string{methodICCreateSession2, methodICStart, methodICConnectToEIS, methodICGetZones, methodICSetBarriers, methodICEnable}
	if got := bus.methods(); !slices.Equal(got, wantOrder) {
		t.Fatalf("call order = %v, want %v", got, wantOrder)
	}
	if !portal.enabled || len(portal.barriers) != 1 {
		t.Fatalf("portal state enabled=%v barriers=%v", portal.enabled, portal.barriers)
	}

	other := dbus.ObjectPath("/org/freedesktop/portal/desktop/session/1_42/other")
	portal.emitCapture(nameActivated, other, Options{"activation_id": dbus.MakeVariant(uint32(99))})
	portal.emitCapture(nameActivated, portal.sessionPath, Options{
		"activation_id":   dbus.MakeVariant(uint32(5)),
		"cursor_position": dbus.MakeVariantWithSignature([]any{1925.5, 300.0}, dbus.ParseSignatureMust("(dd)")),
		"barrier_id":      dbus.MakeVariant(uint32(1)),
	})
	activated, ok := nextEvent(t, stream).(ActivatedEvent)
	if !ok || activated != (ActivatedEvent{ActivationID: 5, Cursor: Point{1925.5, 300}, HasCursor: true, BarrierID: 1, HasBarrier: true}) {
		t.Fatalf("event = %+v, want the Activated of our session", activated)
	}

	if err := cs.Release(ctx, activated.ActivationID, ReleasePosition(armed.Zone, activated.Cursor)); err != nil {
		t.Fatalf("Release: %v", err)
	}
	release := optionsOf(t, bus.callsTo(methodICRelease)[0].Args[1])
	if uintOption(t, release, "activation_id") != 5 {
		t.Fatalf("Release options = %v", release)
	}
	if cursor, ok := release["cursor_position"].Value().(Point); !ok || cursor != (Point{1919, 300}) {
		t.Fatalf("Release cursor = %v", release["cursor_position"].Value())
	}

	portal.emitCapture(nameDeactiv, portal.sessionPath, Options{"activation_id": dbus.MakeVariant(uint32(5))})
	if ev, ok := nextEvent(t, stream).(DeactivatedEvent); !ok || ev != (DeactivatedEvent{ActivationID: 5, HasActivationID: true}) {
		t.Fatalf("event = %+v, want Deactivated", ev)
	}

	portal.zoneSet = 8
	if _, err := cs.Arm(ctx, EdgeLeft, 2); err != nil {
		t.Fatalf("Arm left: %v", err)
	}
	if portal.barriers[0].Position != (Position{0, 0, 0, 1079}) || !portal.enabled {
		t.Fatalf("barriers after re-arm = %+v enabled=%v", portal.barriers, portal.enabled)
	}

	bus.emit(Signal{Path: portal.sessionPath, Name: nameClosed, Body: []any{Options{}}})
	if _, ok := nextEvent(t, stream).(ClosedEvent); !ok {
		t.Fatal("want a ClosedEvent")
	}

	if err := cs.Close(ctx); err != nil || !portal.closed {
		t.Fatalf("Close = %v, portal closed = %v", err, portal.closed)
	}
	if _, err := cs.GetZones(ctx); !errors.Is(err, ErrSessionClosed) {
		t.Fatalf("GetZones after Close = %v, want ErrSessionClosed", err)
	}
	if err := cs.Close(ctx); err != nil || len(bus.callsTo(methodSessionClose)) != 1 {
		t.Fatalf("second Close = %v, Session.Close calls = %d", err, len(bus.callsTo(methodSessionClose)))
	}
}

func TestInputCapture_ArmReportsDeniedBarrier(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	bus := newFakeBus(t)
	portal := newFakePortal(t, bus, 2)
	cs, err := NewInputCapture(newCaller(bus)).Open(ctx, CaptureParams{Capabilities: CapabilityPointer})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	eis, err := cs.ConnectToEIS(ctx)
	if err != nil {
		t.Fatalf("ConnectToEIS: %v", err)
	}
	t.Cleanup(func() { _ = eis.Close() })

	portal.denyBarriers = true
	if _, err := cs.Arm(ctx, EdgeRight, 4); !errors.Is(err, ErrBarrierDenied) {
		t.Fatalf("Arm = %v, want ErrBarrierDenied", err)
	}
	if len(bus.callsTo(methodICEnable)) != 0 {
		t.Fatal("Enable was called although the barrier was denied")
	}
}

func TestInputCapture_StaleZoneSetIsDenied(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	bus := newFakeBus(t)
	newFakePortal(t, bus, 2)
	cs, err := NewInputCapture(newCaller(bus)).Open(ctx, CaptureParams{Capabilities: CapabilityPointer})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	failed, err := cs.SetPointerBarriers(ctx, 1, []Barrier{{ID: 9, Position: Position{1920, 0, 1920, 1079}}})
	if err != nil || !slices.Equal(failed, []uint32{9}) {
		t.Fatalf("failed = %v, %v; want barrier 9 denied for the stale zone set", failed, err)
	}
	if _, err := cs.SetPointerBarriers(ctx, 7, []Barrier{{ID: 0}}); !errors.Is(err, ErrBarrierID) {
		t.Fatalf("zero barrier id = %v, want ErrBarrierID", err)
	}
}

func TestInputCapture_GetZonesDecodesZoneSet(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	bus := newFakeBus(t)
	portal := newFakePortal(t, bus, 2)
	portal.zones = []Zone{{1920, 1080, 0, 0}, {1280, 1024, 1920, 56}}
	cs, err := NewInputCapture(newCaller(bus)).Open(ctx, CaptureParams{Capabilities: CapabilityPointer})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	zs, err := cs.GetZones(ctx)
	if err != nil || zs.ID != 7 || !slices.Equal(zs.Zones, portal.zones) {
		t.Fatalf("GetZones = %+v, %v", zs, err)
	}
}

func TestInputCapture_OpenVersion1(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	bus := newFakeBus(t)
	newFakePortal(t, bus, 1)
	ic := NewInputCapture(newCaller(bus))

	if _, err := ic.Open(ctx, CaptureParams{Capabilities: CapabilityPointer, PersistMode: PersistUntilRevoked}); !errors.Is(err, ErrPersistenceUnsupported) {
		t.Fatalf("persistence on version 1 = %v, want ErrPersistenceUnsupported", err)
	}
	if _, err := ic.Open(ctx, CaptureParams{}); !errors.Is(err, ErrNoCapabilities) {
		t.Fatalf("no capabilities = %v, want ErrNoCapabilities", err)
	}
	if len(bus.methods()) != 0 {
		t.Fatalf("rejected parameters still reached the portal: %v", bus.methods())
	}

	cs, err := ic.Open(ctx, CaptureParams{Capabilities: CapabilityKeyboard | CapabilityPointer})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if got := bus.methods(); !slices.Equal(got, []string{methodICCreateSession}) {
		t.Fatalf("calls = %v, want only CreateSession", got)
	}
	if cs.Version != 1 || cs.RestoreToken != "" || cs.Granted != CapabilityKeyboard|CapabilityPointer {
		t.Fatalf("session = %+v", cs)
	}
}

func TestInputCapture_NothingGrantedClosesSession(t *testing.T) {
	t.Parallel()
	bus := newFakeBus(t)
	portal := newFakePortal(t, bus, 2)
	portal.startCaps = 0
	_, err := NewInputCapture(newCaller(bus)).Open(t.Context(), CaptureParams{Capabilities: CapabilityPointer})
	if !errors.Is(err, ErrNoCapabilities) || !portal.closed {
		t.Fatalf("error = %v, closed = %v; want ErrNoCapabilities and a closed session", err, portal.closed)
	}
}

func TestEventStream_UndecodableSignalEndsStream(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	bus := newFakeBus(t)
	portal := newFakePortal(t, bus, 2)
	cs, err := NewInputCapture(newCaller(bus)).Open(ctx, CaptureParams{Capabilities: CapabilityPointer})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	stream, err := cs.Events(ctx)
	if err != nil {
		t.Fatalf("Events: %v", err)
	}
	portal.emitCapture(nameActivated, portal.sessionPath, Options{})
	select {
	case ev, ok := <-stream.Events():
		if ok {
			t.Fatalf("unexpected event %+v", ev)
		}
	case <-time.After(testWait):
		t.Fatal("stream did not end")
	}
	if err := stream.Err(); !errors.Is(err, ErrMissingResult) {
		t.Fatalf("Err = %v, want ErrMissingResult", err)
	}
	if err := stream.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if subs := len(bus.subs); subs != 0 {
		t.Fatalf("%d subscriptions left open", subs)
	}
}

func TestRemoteDesktop_OpenAndConnectVersion2(t *testing.T) {
	t.Parallel()
	for _, stringHandle := range []bool{true, false} {
		t.Run(fmt.Sprintf("string session handle %v", stringHandle), func(t *testing.T) {
			t.Parallel()
			ctx := t.Context()
			bus := newFakeBus(t)
			portal := newFakePortal(t, bus, 2)
			portal.stringSession = stringHandle
			rd := NewRemoteDesktop(newCaller(bus))

			if types, err := rd.AvailableDeviceTypes(ctx); err != nil || types != DeviceKeyboard|DevicePointer {
				t.Fatalf("AvailableDeviceTypes = %d, %v", types, err)
			}
			rs, err := rd.Open(ctx, RemoteParams{
				Types:        DeviceKeyboard | DevicePointer,
				PersistMode:  PersistUntilRevoked,
				RestoreToken: "old-token",
			})
			if err != nil {
				t.Fatalf("Open: %v", err)
			}
			want := []string{methodRDCreateSession, methodRDSelectDevices, methodRDStart}
			if got := bus.methods(); !slices.Equal(got, want) {
				t.Fatalf("calls = %v, want %v", got, want)
			}
			sel := optionsOf(t, bus.callsTo(methodRDSelectDevices)[0].Args[1])
			if uintOption(t, sel, "types") != 3 || uintOption(t, sel, "persist_mode") != 2 || stringOption(t, sel, "restore_token") != "old-token" {
				t.Fatalf("SelectDevices options = %v", sel)
			}
			if rs.Devices != DeviceKeyboard|DevicePointer || rs.RestoreToken != "new-token" || rs.Path() != portal.sessionPath {
				t.Fatalf("session = %+v", rs)
			}

			eis, err := rs.ConnectToEIS(ctx)
			if err != nil {
				t.Fatalf("ConnectToEIS: %v", err)
			}
			t.Cleanup(func() { _ = eis.Close() })
			if _, err := rs.ConnectToEIS(ctx); !errors.Is(err, ErrEISAlreadyConnected) {
				t.Fatalf("second ConnectToEIS = %v, want ErrEISAlreadyConnected", err)
			}
			if got := bus.methods(); got[len(got)-1] != methodRDConnectToEIS {
				t.Fatalf("ConnectToEIS did not follow Start: %v", got)
			}
		})
	}
}

func TestRemoteDesktop_Version1HasNoEISAndNoPersistence(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	bus := newFakeBus(t)
	newFakePortal(t, bus, 1)
	rd := NewRemoteDesktop(newCaller(bus))

	if _, err := rd.Open(ctx, RemoteParams{Types: DevicePointer, RestoreToken: "x"}); !errors.Is(err, ErrPersistenceUnsupported) {
		t.Fatalf("restore token on version 1 = %v, want ErrPersistenceUnsupported", err)
	}
	rs, err := rd.Open(ctx, RemoteParams{Types: DevicePointer})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	sel := optionsOf(t, bus.callsTo(methodRDSelectDevices)[0].Args[1])
	if _, has := sel["persist_mode"]; has {
		t.Fatalf("persist_mode sent on version 1: %v", sel)
	}
	if _, err := rs.ConnectToEIS(ctx); !errors.Is(err, ErrEISUnsupported) {
		t.Fatalf("ConnectToEIS on version 1 = %v, want ErrEISUnsupported", err)
	}
}

func TestRemoteDesktop_NoDevicesGrantedClosesSession(t *testing.T) {
	t.Parallel()
	bus := newFakeBus(t)
	portal := newFakePortal(t, bus, 2)
	portal.startDevices = 0
	_, err := NewRemoteDesktop(newCaller(bus)).Open(t.Context(), RemoteParams{Types: DevicePointer})
	if !errors.Is(err, ErrNoDevices) || !portal.closed {
		t.Fatalf("error = %v, closed = %v; want ErrNoDevices and a closed session", err, portal.closed)
	}
}

func TestRemoteDesktop_CancelledStartClosesSession(t *testing.T) {
	t.Parallel()
	bus := newFakeBus(t)
	portal := newFakePortal(t, bus, 2)
	portal.startCode = 1
	_, err := NewRemoteDesktop(newCaller(bus)).Open(t.Context(), RemoteParams{Types: DevicePointer})
	if !errors.Is(err, ErrCancelled) || !portal.closed {
		t.Fatalf("error = %v, closed = %v; want ErrCancelled and a closed session", err, portal.closed)
	}
}

func TestSession_VersionPropertyMustBeUint32(t *testing.T) {
	t.Parallel()
	bus := newFakeBus(t)
	bus.props[interfaceInputCapture+".version"] = dbus.MakeVariant("two")
	if _, err := NewInputCapture(newCaller(bus)).Version(t.Context()); !errors.Is(err, ErrResultType) {
		t.Fatalf("error = %v, want ErrResultType", err)
	}
}
