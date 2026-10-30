package ei

import (
	"bufio"
	"errors"
	"fmt"
)

var (
	ErrProtocol = errors.New("ei: protocol violation")
	ErrConfig   = errors.New("ei: invalid configuration")
)

type Config struct {
	Name string
	Role Role
}

func (c Config) validate() error {
	if !c.Role.valid() {
		return fmt.Errorf("%w: role %d is neither receiver nor sender", ErrConfig, c.Role)
	}
	return nil
}

type HandshakeResult struct {
	EISHandshakeVersion  uint32
	Serial               uint32
	Connection           uint64
	ConnectionVersion    uint32
	EISInterfaceVersions map[string]uint32
}

func (r HandshakeResult) negotiatedLimit(iface Interface) uint32 {
	limit := iface.MaxVersion()
	if eis, ok := r.EISInterfaceVersions[iface.String()]; ok && eis < limit {
		return eis
	}
	return limit
}

func performHandshake(rd *bufio.Reader, t Transport, cfg Config) (HandshakeResult, error) {
	first, err := ReadFrame(rd)
	if err != nil {
		return HandshakeResult{}, fmt.Errorf("handshake: read ei_handshake.handshake_version: %w", err)
	}
	if first.ObjectID != HandshakeObjectID || first.Opcode != evHandshakeVersion {
		return HandshakeResult{}, fmt.Errorf("%w: first message is object %d opcode %d, want ei_handshake.handshake_version",
			ErrProtocol, first.ObjectID, first.Opcode)
	}
	hv, err := DecodeEvent(InterfaceHandshake, first, t)
	if err != nil {
		return HandshakeResult{}, fmt.Errorf("handshake: %w", err)
	}
	eisVersion := hv.(HandshakeVersionEvent).Version
	if eisVersion < 1 {
		return HandshakeResult{}, fmt.Errorf("%w: EIS announced ei_handshake version %d", ErrProtocol, eisVersion)
	}

	if err := sendHandshakeRequests(t, cfg, min(eisVersion, InterfaceHandshake.MaxVersion())); err != nil {
		return HandshakeResult{}, err
	}

	result := HandshakeResult{EISHandshakeVersion: eisVersion, EISInterfaceVersions: map[string]uint32{}}
	for {
		f, err := ReadFrame(rd)
		if err != nil {
			return HandshakeResult{}, fmt.Errorf("handshake: read: %w", err)
		}
		if f.ObjectID != HandshakeObjectID {
			return HandshakeResult{}, fmt.Errorf("%w: message for object %d before ei_handshake.connection", ErrProtocol, f.ObjectID)
		}
		ev, err := DecodeEvent(InterfaceHandshake, f, t)
		if err != nil {
			return HandshakeResult{}, fmt.Errorf("handshake: %w", err)
		}
		switch ev := ev.(type) {
		case HandshakeInterfaceVersionEvent:
			if err := recordInterfaceVersion(result.EISInterfaceVersions, ev); err != nil {
				return HandshakeResult{}, err
			}
		case HandshakeConnectionEvent:
			return completeHandshake(result, ev)
		default:
			return HandshakeResult{}, fmt.Errorf("%w: unexpected %T during handshake", ErrProtocol, ev)
		}
	}
}

func sendHandshakeRequests(t Transport, cfg Config, handshakeVersion uint32) error {
	frames := []Frame{handshakeVersionRequest(handshakeVersion), handshakeContextTypeRequest(cfg.Role)}
	if cfg.Name != "" {
		name, err := handshakeNameRequest(cfg.Name)
		if err != nil {
			return fmt.Errorf("handshake: client name: %w", err)
		}
		frames = append(frames, name)
	}
	for _, iface := range negotiableInterfaces() {
		f, err := handshakeInterfaceVersionRequest(iface.String(), iface.MaxVersion())
		if err != nil {
			return fmt.Errorf("handshake: %s: %w", iface, err)
		}
		frames = append(frames, f)
	}
	frames = append(frames, handshakeFinishRequest())

	var batch []byte
	for _, f := range frames {
		var err error
		if batch, err = f.AppendTo(batch); err != nil {
			return fmt.Errorf("handshake: encode: %w", err)
		}
	}
	if _, err := t.Write(batch); err != nil {
		return fmt.Errorf("handshake: write: %w", err)
	}
	return nil
}

func recordInterfaceVersion(versions map[string]uint32, ev HandshakeInterfaceVersionEvent) error {
	if _, dup := versions[ev.Name]; dup {
		return fmt.Errorf("%w: EIS announced %s twice", ErrProtocol, ev.Name)
	}
	if iface, ok := InterfaceByName(ev.Name); ok && ev.Version > iface.MaxVersion() {
		return fmt.Errorf("%w: EIS announced %s version %d above the client's %d",
			ErrProtocol, ev.Name, ev.Version, iface.MaxVersion())
	}
	versions[ev.Name] = ev.Version
	return nil
}

func completeHandshake(result HandshakeResult, ev HandshakeConnectionEvent) (HandshakeResult, error) {
	if ev.Connection < FirstServerObjectID {
		return HandshakeResult{}, fmt.Errorf("%w: ei_connection id %#x is below the EIS id range", ErrProtocol, ev.Connection)
	}
	limit := result.negotiatedLimit(InterfaceConnection)
	if ev.Version < 1 || ev.Version > limit {
		return HandshakeResult{}, fmt.Errorf("%w: ei_connection version %d outside 1..%d", ErrProtocol, ev.Version, limit)
	}
	result.Serial = ev.Serial
	result.Connection = ev.Connection
	result.ConnectionVersion = ev.Version
	return result, nil
}
