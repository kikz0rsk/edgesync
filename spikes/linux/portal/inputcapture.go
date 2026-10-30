package portal

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"

	"github.com/godbus/dbus/v5"
)

const (
	interfaceInputCapture = "org.freedesktop.portal.InputCapture"

	methodICCreateSession  = interfaceInputCapture + ".CreateSession"
	methodICCreateSession2 = interfaceInputCapture + ".CreateSession2"
	methodICStart          = interfaceInputCapture + ".Start"
	methodICGetZones       = interfaceInputCapture + ".GetZones"
	methodICSetBarriers    = interfaceInputCapture + ".SetPointerBarriers"
	methodICEnable         = interfaceInputCapture + ".Enable"
	methodICDisable        = interfaceInputCapture + ".Disable"
	methodICRelease        = interfaceInputCapture + ".Release"
	methodICConnectToEIS   = interfaceInputCapture + ".ConnectToEIS"

	propertySupportedCapabilities = "SupportedCapabilities"

	sessionVersionWithCreateSession2 = 2
)

var (
	ErrPersistenceUnsupported = errors.New("portal: restore tokens and persist_mode need interface version 2")
	ErrNoCapabilities         = errors.New("portal: no capability requested or granted")
	ErrEISNotConnected        = errors.New("portal: ConnectToEIS must be called before Enable")
	ErrEISAlreadyConnected    = errors.New("portal: ConnectToEIS was already called on this session")
	ErrNoZones                = errors.New("portal: no usable zones")
	ErrBarrierID              = errors.New("portal: barrier id must be non-zero")
	ErrBarrierDenied          = errors.New("portal: pointer barrier denied")
	ErrUnknownEdge            = errors.New("portal: unknown edge")
)

type Capability uint32

const (
	CapabilityKeyboard    Capability = 1
	CapabilityPointer     Capability = 2
	CapabilityTouchscreen Capability = 4
)

type PersistMode uint32

const (
	PersistNone         PersistMode = 0
	PersistWhileRunning PersistMode = 1
	PersistUntilRevoked PersistMode = 2
)

type Point struct {
	X, Y float64
}

type Zone struct {
	Width, Height uint32
	X, Y          int32
}

type ZoneSet struct {
	ID    uint32
	Zones []Zone
}

type Position struct {
	X1, Y1, X2, Y2 int32
}

type Barrier struct {
	ID       uint32
	Position Position
}

type Edge int

const (
	EdgeRight Edge = iota + 1
	EdgeLeft
)

func (e Edge) String() string {
	switch e {
	case EdgeRight:
		return "right"
	case EdgeLeft:
		return "left"
	default:
		return fmt.Sprintf("edge(%d)", int(e))
	}
}

// BarrierFor places one vertical barrier on the outer edge of the union of the zones: the right edge of the
// zone reaching furthest right, or the left edge of the zone reaching furthest left. Ties keep the first zone.
func BarrierFor(zones []Zone, edge Edge, id uint32) (Barrier, Zone, error) {
	if id == 0 {
		return Barrier{}, Zone{}, ErrBarrierID
	}
	if len(zones) == 0 {
		return Barrier{}, Zone{}, ErrNoZones
	}
	for _, z := range zones {
		if z.Width == 0 || z.Height == 0 {
			return Barrier{}, Zone{}, fmt.Errorf("%w: zone %+v is empty", ErrNoZones, z)
		}
	}
	chosen := zones[0]
	for _, z := range zones[1:] {
		switch edge {
		case EdgeRight:
			if rightEdge(z) > rightEdge(chosen) {
				chosen = z
			}
		case EdgeLeft:
			if z.X < chosen.X {
				chosen = z
			}
		default:
			return Barrier{}, Zone{}, fmt.Errorf("%w: %v", ErrUnknownEdge, edge)
		}
	}
	var x int32
	switch edge {
	case EdgeRight:
		x = rightEdge(chosen)
	case EdgeLeft:
		x = chosen.X
	default:
		return Barrier{}, Zone{}, fmt.Errorf("%w: %v", ErrUnknownEdge, edge)
	}
	return Barrier{ID: id, Position: Position{X1: x, Y1: chosen.Y, X2: x, Y2: chosen.Y + int32(chosen.Height) - 1}}, chosen, nil
}

func rightEdge(z Zone) int32 {
	return z.X + int32(z.Width)
}

// ReleasePosition clamps the position reported by Activated, which lies outside the zones, back into the zone
// that carries the barrier, as the suggested cursor position of Release.
func ReleasePosition(zone Zone, p Point) Point {
	return Point{
		X: clampFloat(p.X, float64(zone.X), float64(zone.X)+float64(zone.Width)-1),
		Y: clampFloat(p.Y, float64(zone.Y), float64(zone.Y)+float64(zone.Height)-1),
	}
}

func clampFloat(v, lo, hi float64) float64 {
	return min(max(v, lo), hi)
}

type ActivatedEvent struct {
	ActivationID uint32
	Cursor       Point
	HasCursor    bool
	BarrierID    uint32
	HasBarrier   bool
}

type DeactivatedEvent struct {
	ActivationID    uint32
	HasActivationID bool
}

type DisabledEvent struct{}

type ZonesChangedEvent struct {
	ZoneSet    uint32
	HasZoneSet bool
}

func (ActivatedEvent) isEvent()    {}
func (DeactivatedEvent) isEvent()  {}
func (DisabledEvent) isEvent()     {}
func (ZonesChangedEvent) isEvent() {}

type CaptureParams struct {
	Capabilities Capability
	PersistMode  PersistMode
	RestoreToken string
}

type InputCapture struct {
	caller *Caller
}

func NewInputCapture(caller *Caller) *InputCapture {
	return &InputCapture{caller: caller}
}

func (ic *InputCapture) Version(ctx context.Context) (uint32, error) {
	return interfaceVersion(ctx, ic.caller.Bus(), interfaceInputCapture)
}

func (ic *InputCapture) SupportedCapabilities(ctx context.Context) (Capability, error) {
	v, err := ic.caller.Bus().Property(ctx, DesktopPath, interfaceInputCapture, propertySupportedCapabilities)
	if err != nil {
		return 0, err
	}
	caps, ok := v.Value().(uint32)
	if !ok {
		return 0, fmt.Errorf("%w: SupportedCapabilities is %T, want uint32", ErrResultType, v.Value())
	}
	return Capability(caps), nil
}

type CaptureSession struct {
	*Session
	caller *Caller

	Version          uint32
	Granted          Capability
	ClipboardEnabled bool
	RestoreToken     string

	eisMu        sync.Mutex
	eisConnected bool
}

// Open creates and starts an input capture session: CreateSession2 followed by Start on interface version 2 or
// later (the only versions with restore tokens), the deprecated CreateSession otherwise.
func (ic *InputCapture) Open(ctx context.Context, p CaptureParams) (*CaptureSession, error) {
	if p.Capabilities == 0 {
		return nil, ErrNoCapabilities
	}
	version, err := ic.Version(ctx)
	if err != nil {
		return nil, err
	}
	if version < sessionVersionWithCreateSession2 && (p.PersistMode != PersistNone || p.RestoreToken != "") {
		return nil, ErrPersistenceUnsupported
	}

	var cs *CaptureSession
	if version >= sessionVersionWithCreateSession2 {
		cs, err = ic.openV2(ctx, p)
	} else {
		cs, err = ic.openV1(ctx, p)
	}
	if err != nil {
		return nil, err
	}
	cs.Version = version
	if cs.Granted == 0 {
		return nil, errors.Join(fmt.Errorf("%w: the portal granted none of %d", ErrNoCapabilities, p.Capabilities), cs.Close(ctx))
	}
	return cs, nil
}

func (ic *InputCapture) openV2(ctx context.Context, p CaptureParams) (*CaptureSession, error) {
	sessionToken, err := ic.caller.NewToken()
	if err != nil {
		return nil, err
	}
	ret, err := ic.caller.Bus().Call(ctx, DesktopPath, methodICCreateSession2,
		Options{optionSessionToken: dbus.MakeVariant(sessionToken)})
	if err != nil {
		return nil, err
	}
	if len(ret) != 1 {
		return nil, fmt.Errorf("%w: CreateSession2 returned %d values, want 1", ErrUnexpectedReturn, len(ret))
	}
	created, ok := ret[0].(map[string]dbus.Variant)
	if !ok {
		return nil, fmt.Errorf("%w: CreateSession2 returned %T, want a{sv}", ErrUnexpectedReturn, ret[0])
	}
	path, err := Results(created).ObjectPath("session_handle")
	if err != nil {
		return nil, fmt.Errorf("create session 2: %w", err)
	}
	cs := ic.newSession(path)

	opts := Options{
		"capabilities": dbus.MakeVariant(uint32(p.Capabilities)),
		"persist_mode": dbus.MakeVariant(uint32(p.PersistMode)),
	}
	if p.RestoreToken != "" {
		opts["restore_token"] = dbus.MakeVariant(p.RestoreToken)
	}
	resp, err := ic.caller.Request(ctx, methodICStart, opts, func(o Options) []any {
		return []any{path, "", o}
	})
	if err != nil {
		return nil, errors.Join(err, cs.Close(ctx))
	}
	if err := cs.readStartResults(resp.Results); err != nil {
		return nil, errors.Join(fmt.Errorf("start: %w", err), cs.Close(ctx))
	}
	return cs, nil
}

func (ic *InputCapture) openV1(ctx context.Context, p CaptureParams) (*CaptureSession, error) {
	sessionToken, err := ic.caller.NewToken()
	if err != nil {
		return nil, err
	}
	opts := Options{
		optionSessionToken: dbus.MakeVariant(sessionToken),
		"capabilities":     dbus.MakeVariant(uint32(p.Capabilities)),
	}
	resp, err := ic.caller.Request(ctx, methodICCreateSession, opts, func(o Options) []any {
		return []any{"", o}
	})
	if err != nil {
		return nil, err
	}
	path, err := resp.Results.ObjectPath("session_handle")
	if err != nil {
		return nil, fmt.Errorf("create session: %w", err)
	}
	cs := ic.newSession(path)
	if err := cs.readStartResults(resp.Results); err != nil {
		return nil, errors.Join(fmt.Errorf("create session: %w", err), cs.Close(ctx))
	}
	return cs, nil
}

func (ic *InputCapture) newSession(path dbus.ObjectPath) *CaptureSession {
	return &CaptureSession{Session: newSession(ic.caller.Bus(), path), caller: ic.caller}
}

func (cs *CaptureSession) readStartResults(r Results) error {
	granted, err := r.Uint32("capabilities")
	if err != nil {
		return err
	}
	cs.Granted = Capability(granted)
	if r.Has("clipboard_enabled") {
		if cs.ClipboardEnabled, err = r.Bool("clipboard_enabled"); err != nil {
			return err
		}
	}
	if r.Has("restore_token") {
		if cs.RestoreToken, err = r.String("restore_token"); err != nil {
			return err
		}
	}
	return nil
}

func (cs *CaptureSession) GetZones(ctx context.Context) (ZoneSet, error) {
	if err := cs.ensureOpen(); err != nil {
		return ZoneSet{}, err
	}
	resp, err := cs.caller.Request(ctx, methodICGetZones, Options{}, func(o Options) []any {
		return []any{cs.path, o}
	})
	if err != nil {
		return ZoneSet{}, err
	}
	id, err := resp.Results.Uint32("zone_set")
	if err != nil {
		return ZoneSet{}, fmt.Errorf("get zones: %w", err)
	}
	raw, ok := resp.Results["zones"]
	if !ok {
		return ZoneSet{}, fmt.Errorf("get zones: %w: %q", ErrMissingResult, "zones")
	}
	zones, err := decodeZones(raw)
	if err != nil {
		return ZoneSet{}, fmt.Errorf("get zones: %w", err)
	}
	return ZoneSet{ID: id, Zones: zones}, nil
}

func decodeZones(v dbus.Variant) ([]Zone, error) {
	rows, ok := v.Value().([][]any)
	if !ok {
		return nil, fmt.Errorf("%w: zones are %T, want a(uuii)", ErrResultType, v.Value())
	}
	zones := make([]Zone, 0, len(rows))
	for i, row := range rows {
		if len(row) != 4 {
			return nil, fmt.Errorf("%w: zone %d has %d fields, want 4", ErrResultType, i, len(row))
		}
		w, wOK := row[0].(uint32)
		h, hOK := row[1].(uint32)
		x, xOK := row[2].(int32)
		y, yOK := row[3].(int32)
		if !wOK || !hOK || !xOK || !yOK {
			return nil, fmt.Errorf("%w: zone %d is %T, want (uuii)", ErrResultType, i, row)
		}
		zones = append(zones, Zone{Width: w, Height: h, X: x, Y: y})
	}
	return zones, nil
}

// SetPointerBarriers returns the ids of the barriers the portal denied. The call suspends the session, so
// Enable must follow.
func (cs *CaptureSession) SetPointerBarriers(ctx context.Context, zoneSet uint32, barriers []Barrier) ([]uint32, error) {
	if err := cs.ensureOpen(); err != nil {
		return nil, err
	}
	wire := make([]map[string]dbus.Variant, 0, len(barriers))
	for _, b := range barriers {
		if b.ID == 0 {
			return nil, ErrBarrierID
		}
		wire = append(wire, map[string]dbus.Variant{
			"barrier_id": dbus.MakeVariant(b.ID),
			"position":   dbus.MakeVariant(b.Position),
		})
	}
	resp, err := cs.caller.Request(ctx, methodICSetBarriers, Options{}, func(o Options) []any {
		return []any{cs.path, o, wire, zoneSet}
	})
	if err != nil {
		return nil, err
	}
	if !resp.Results.Has("failed_barriers") {
		return nil, nil
	}
	failed, err := resp.Results.Uint32s("failed_barriers")
	if err != nil {
		return nil, fmt.Errorf("set pointer barriers: %w", err)
	}
	return failed, nil
}

func (cs *CaptureSession) ConnectToEIS(ctx context.Context) (*os.File, error) {
	if err := cs.ensureOpen(); err != nil {
		return nil, err
	}
	cs.eisMu.Lock()
	defer cs.eisMu.Unlock()
	if cs.eisConnected {
		return nil, ErrEISAlreadyConnected
	}
	f, err := cs.caller.Bus().CallFD(ctx, DesktopPath, methodICConnectToEIS, cs.path, Options{})
	if err != nil {
		return nil, err
	}
	cs.eisConnected = true
	return f, nil
}

func (cs *CaptureSession) Enable(ctx context.Context) error {
	if err := cs.ensureOpen(); err != nil {
		return err
	}
	cs.eisMu.Lock()
	connected := cs.eisConnected
	cs.eisMu.Unlock()
	if !connected {
		return ErrEISNotConnected
	}
	_, err := cs.caller.Bus().Call(ctx, DesktopPath, methodICEnable, cs.path, Options{})
	return err
}

func (cs *CaptureSession) Disable(ctx context.Context) error {
	if err := cs.ensureOpen(); err != nil {
		return err
	}
	_, err := cs.caller.Bus().Call(ctx, DesktopPath, methodICDisable, cs.path, Options{})
	return err
}

func (cs *CaptureSession) Release(ctx context.Context, activationID uint32, cursor Point) error {
	if err := cs.ensureOpen(); err != nil {
		return err
	}
	_, err := cs.caller.Bus().Call(ctx, DesktopPath, methodICRelease, cs.path, Options{
		"activation_id":   dbus.MakeVariant(activationID),
		"cursor_position": dbus.MakeVariant(cursor),
	})
	return err
}

type Armed struct {
	ZoneSet ZoneSet
	Barrier Barrier
	Zone    Zone
}

// Arm refreshes the zones, installs one barrier on the requested edge and enables capture. It is the
// sequence the portal requires after every barrier change, because SetPointerBarriers suspends the session.
func (cs *CaptureSession) Arm(ctx context.Context, edge Edge, barrierID uint32) (Armed, error) {
	zones, err := cs.GetZones(ctx)
	if err != nil {
		return Armed{}, err
	}
	barrier, zone, err := BarrierFor(zones.Zones, edge, barrierID)
	if err != nil {
		return Armed{}, err
	}
	failed, err := cs.SetPointerBarriers(ctx, zones.ID, []Barrier{barrier})
	if err != nil {
		return Armed{}, err
	}
	if len(failed) > 0 {
		return Armed{}, fmt.Errorf("%w: ids %v for %+v on zone set %d", ErrBarrierDenied, failed, barrier.Position, zones.ID)
	}
	if err := cs.Enable(ctx); err != nil {
		return Armed{}, err
	}
	return Armed{ZoneSet: zones, Barrier: barrier, Zone: zone}, nil
}

// Events subscribes to Activated, Deactivated, Disabled and ZonesChanged of this session plus Session.Closed.
// Subscribe before Arm so that no Activated signal can be missed.
func (cs *CaptureSession) Events(ctx context.Context) (*EventStream, error) {
	if err := cs.ensureOpen(); err != nil {
		return nil, err
	}
	return newEventStream(ctx, cs.caller.Bus(), eventSource{
		match:  SignalMatch{Path: DesktopPath, Interface: interfaceInputCapture},
		decode: decodeCaptureSignal(cs.path),
	}, closedSource(cs.path))
}

func decodeCaptureSignal(session dbus.ObjectPath) func(Signal) (Event, bool, error) {
	return func(sig Signal) (Event, bool, error) {
		if len(sig.Body) != 2 {
			return nil, false, fmt.Errorf("%w: %d values, want 2", ErrBadResponse, len(sig.Body))
		}
		target, ok := sig.Body[0].(dbus.ObjectPath)
		if !ok {
			return nil, false, fmt.Errorf("%w: session handle is %T", ErrBadResponse, sig.Body[0])
		}
		if target != session {
			return nil, false, nil
		}
		opts, ok := sig.Body[1].(map[string]dbus.Variant)
		if !ok {
			return nil, false, fmt.Errorf("%w: options are %T", ErrBadResponse, sig.Body[1])
		}
		_, member := splitSignalName(sig.Name)
		ev, err := decodeCaptureEvent(member, Results(opts))
		if err != nil {
			return nil, false, err
		}
		return ev, ev != nil, nil
	}
}

func decodeCaptureEvent(member string, r Results) (Event, error) {
	switch member {
	case "Activated":
		return decodeActivated(r)
	case "Deactivated":
		ev := DeactivatedEvent{}
		if r.Has("activation_id") {
			id, err := r.Uint32("activation_id")
			if err != nil {
				return nil, err
			}
			ev.ActivationID, ev.HasActivationID = id, true
		}
		return ev, nil
	case "Disabled":
		return DisabledEvent{}, nil
	case "ZonesChanged":
		ev := ZonesChangedEvent{}
		if r.Has("zone_set") {
			id, err := r.Uint32("zone_set")
			if err != nil {
				return nil, err
			}
			ev.ZoneSet, ev.HasZoneSet = id, true
		}
		return ev, nil
	default:
		return nil, nil
	}
}

func decodeActivated(r Results) (Event, error) {
	id, err := r.Uint32("activation_id")
	if err != nil {
		return nil, fmt.Errorf("activated: %w", err)
	}
	ev := ActivatedEvent{ActivationID: id}
	if r.Has("cursor_position") {
		if ev.Cursor, err = decodePoint(r["cursor_position"]); err != nil {
			return nil, fmt.Errorf("activated: %w", err)
		}
		ev.HasCursor = true
	}
	if r.Has("barrier_id") {
		if ev.BarrierID, err = r.Uint32("barrier_id"); err != nil {
			return nil, fmt.Errorf("activated: %w", err)
		}
		ev.HasBarrier = true
	}
	return ev, nil
}

func decodePoint(v dbus.Variant) (Point, error) {
	fields, ok := v.Value().([]any)
	if !ok || len(fields) != 2 {
		return Point{}, fmt.Errorf("%w: cursor_position is %T, want (dd)", ErrResultType, v.Value())
	}
	x, xOK := fields[0].(float64)
	y, yOK := fields[1].(float64)
	if !xOK || !yOK {
		return Point{}, fmt.Errorf("%w: cursor_position is %T, want (dd)", ErrResultType, fields)
	}
	return Point{X: x, Y: y}, nil
}
