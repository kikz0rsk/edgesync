package ei

import (
	"errors"
	"fmt"
	"os"
)

var (
	ErrUnknownOpcode        = errors.New("ei: unknown opcode")
	ErrUnsupportedInterface = errors.New("ei: unsupported interface")
)

type Interface uint8

const (
	InterfaceHandshake Interface = iota + 1
	InterfaceConnection
	InterfaceCallback
	InterfacePingpong
	InterfaceSeat
	InterfaceDevice
	InterfacePointer
	InterfacePointerAbsolute
	InterfaceScroll
	InterfaceButton
	InterfaceKeyboard
)

func (i Interface) String() string {
	switch i {
	case InterfaceHandshake:
		return "ei_handshake"
	case InterfaceConnection:
		return "ei_connection"
	case InterfaceCallback:
		return "ei_callback"
	case InterfacePingpong:
		return "ei_pingpong"
	case InterfaceSeat:
		return "ei_seat"
	case InterfaceDevice:
		return "ei_device"
	case InterfacePointer:
		return "ei_pointer"
	case InterfacePointerAbsolute:
		return "ei_pointer_absolute"
	case InterfaceScroll:
		return "ei_scroll"
	case InterfaceButton:
		return "ei_button"
	case InterfaceKeyboard:
		return "ei_keyboard"
	default:
		return fmt.Sprintf("ei_interface(%d)", uint8(i))
	}
}

func (i Interface) MaxVersion() uint32 {
	switch i {
	case InterfaceSeat:
		return 2
	case InterfaceDevice:
		return 3
	default:
		return 1
	}
}

func InterfaceByName(name string) (Interface, bool) {
	for _, i := range negotiableInterfaces() {
		if i.String() == name {
			return i, true
		}
	}
	return 0, false
}

func negotiableInterfaces() []Interface {
	return []Interface{
		InterfaceConnection, InterfaceCallback, InterfacePingpong, InterfaceSeat, InterfaceDevice,
		InterfacePointer, InterfacePointerAbsolute, InterfaceScroll, InterfaceButton, InterfaceKeyboard,
	}
}

type Role uint32

const (
	RoleReceiver Role = 1
	RoleSender   Role = 2
)

func (r Role) valid() bool {
	return r == RoleReceiver || r == RoleSender
}

type DeviceType uint32

const (
	DeviceTypeVirtual  DeviceType = 1
	DeviceTypePhysical DeviceType = 2
)

type ButtonState uint32

const (
	ButtonReleased ButtonState = 0
	ButtonPressed  ButtonState = 1
)

type KeyState uint32

const (
	KeyReleased KeyState = 0
	KeyPressed  KeyState = 1
)

type KeymapType uint32

const KeymapTypeXKB KeymapType = 1

type DisconnectReason uint32

const (
	DisconnectReasonDisconnected DisconnectReason = 0
	DisconnectReasonError        DisconnectReason = 1
	DisconnectReasonMode         DisconnectReason = 2
	DisconnectReasonProtocol     DisconnectReason = 3
	DisconnectReasonValue        DisconnectReason = 4
	DisconnectReasonTransport    DisconnectReason = 5
)

const (
	opHandshakeVersion          = 0
	opHandshakeFinish           = 1
	opHandshakeContextType      = 2
	opHandshakeName             = 3
	opHandshakeInterfaceVersion = 4

	evHandshakeVersion          = 0
	evHandshakeInterfaceVersion = 1
	evHandshakeConnection       = 2

	opConnectionSync       = 0
	opConnectionDisconnect = 1

	evConnectionDisconnected  = 0
	evConnectionSeat          = 1
	evConnectionInvalidObject = 2
	evConnectionPing          = 3

	evCallbackDone = 0

	opPingpongDone = 0

	opRelease = 0

	opSeatBind = 1

	evSeatDestroyed  = 0
	evSeatName       = 1
	evSeatCapability = 2
	evSeatDone       = 3
	evSeatDevice     = 4

	opDeviceStartEmulating = 1
	opDeviceStopEmulating  = 2
	opDeviceFrame          = 3
	opDeviceReady          = 4

	evDeviceDestroyed       = 0
	evDeviceName            = 1
	evDeviceDeviceType      = 2
	evDeviceDimensions      = 3
	evDeviceRegion          = 4
	evDeviceInterface       = 5
	evDeviceDone            = 6
	evDeviceResumed         = 7
	evDevicePaused          = 8
	evDeviceStartEmulating  = 9
	evDeviceStopEmulating   = 10
	evDeviceFrame           = 11
	evDeviceRegionMappingID = 12

	opPointerMotionRelative = 1
	evPointerDestroyed      = 0
	evPointerMotionRelative = 1

	opPointerAbsoluteMotionAbsolute = 1
	evPointerAbsoluteDestroyed      = 0
	evPointerAbsoluteMotionAbsolute = 1

	opScrollScroll         = 1
	opScrollScrollDiscrete = 2
	opScrollScrollStop     = 3
	evScrollDestroyed      = 0
	evScrollScroll         = 1
	evScrollScrollDiscrete = 2
	evScrollScrollStop     = 3

	opButtonButton    = 1
	evButtonDestroyed = 0
	evButtonButton    = 1

	opKeyboardKey       = 1
	evKeyboardDestroyed = 0
	evKeyboardKeymap    = 1
	evKeyboardKey       = 2
	evKeyboardModifiers = 3
)

type Event interface {
	isEvent()
}

type serialEvent interface {
	Event
	serialNumber() uint32
}

type HandshakeVersionEvent struct{ Version uint32 }

type HandshakeInterfaceVersionEvent struct {
	Name    string
	Version uint32
}

type HandshakeConnectionEvent struct {
	Serial     uint32
	Connection uint64
	Version    uint32
}

type ConnectionDisconnectedEvent struct {
	LastSerial     uint32
	Reason         DisconnectReason
	Explanation    string
	HasExplanation bool
}

type ConnectionSeatEvent struct {
	Seat    uint64
	Version uint32
}

type ConnectionInvalidObjectEvent struct {
	LastSerial uint32
	InvalidID  uint64
}

type ConnectionPingEvent struct {
	Ping    uint64
	Version uint32
}

type CallbackDoneEvent struct{ Data uint64 }

type DestroyedEvent struct{ Serial uint32 }

type SeatNameEvent struct{ Name string }

type SeatCapabilityEvent struct {
	Mask      uint64
	Interface string
}

type SeatDoneEvent struct{}

type SeatDeviceEvent struct {
	Device  uint64
	Version uint32
}

type DeviceNameEvent struct{ Name string }

type DeviceTypeEvent struct{ Type DeviceType }

type DeviceDimensionsEvent struct{ Width, Height uint32 }

type DeviceRegionEvent struct {
	OffsetX, OffsetY, Width, Height uint32
	Scale                           float32
}

type DeviceInterfaceEvent struct {
	Object  uint64
	Name    string
	Version uint32
}

type DeviceDoneEvent struct{}

type DeviceResumedEvent struct{ Serial uint32 }

type DevicePausedEvent struct{ Serial uint32 }

type DeviceStartEmulatingEvent struct{ Serial, Sequence uint32 }

type DeviceStopEmulatingEvent struct{ Serial uint32 }

type DeviceFrameEvent struct {
	Serial    uint32
	Timestamp uint64
}

type DeviceRegionMappingIDEvent struct{ MappingID string }

type PointerMotionRelativeEvent struct{ X, Y float32 }

type PointerMotionAbsoluteEvent struct{ X, Y float32 }

type ScrollEvent struct{ X, Y float32 }

type ScrollDiscreteEvent struct{ X, Y int32 }

type ScrollStopEvent struct {
	X, Y     uint32
	IsCancel uint32
}

type ButtonEvent struct {
	Button uint32
	State  ButtonState
}

type KeyboardKeymapEvent struct {
	Type KeymapType
	Size uint32
	FD   *os.File
}

type KeyboardKeyEvent struct {
	Key   uint32
	State KeyState
}

type KeyboardModifiersEvent struct {
	Serial                            uint32
	Depressed, Locked, Latched, Group uint32
}

func (HandshakeVersionEvent) isEvent()          {}
func (HandshakeInterfaceVersionEvent) isEvent() {}
func (HandshakeConnectionEvent) isEvent()       {}
func (ConnectionDisconnectedEvent) isEvent()    {}
func (ConnectionSeatEvent) isEvent()            {}
func (ConnectionInvalidObjectEvent) isEvent()   {}
func (ConnectionPingEvent) isEvent()            {}
func (CallbackDoneEvent) isEvent()              {}
func (DestroyedEvent) isEvent()                 {}
func (SeatNameEvent) isEvent()                  {}
func (SeatCapabilityEvent) isEvent()            {}
func (SeatDoneEvent) isEvent()                  {}
func (SeatDeviceEvent) isEvent()                {}
func (DeviceNameEvent) isEvent()                {}
func (DeviceTypeEvent) isEvent()                {}
func (DeviceDimensionsEvent) isEvent()          {}
func (DeviceRegionEvent) isEvent()              {}
func (DeviceInterfaceEvent) isEvent()           {}
func (DeviceDoneEvent) isEvent()                {}
func (DeviceResumedEvent) isEvent()             {}
func (DevicePausedEvent) isEvent()              {}
func (DeviceStartEmulatingEvent) isEvent()      {}
func (DeviceStopEmulatingEvent) isEvent()       {}
func (DeviceFrameEvent) isEvent()               {}
func (DeviceRegionMappingIDEvent) isEvent()     {}
func (PointerMotionRelativeEvent) isEvent()     {}
func (PointerMotionAbsoluteEvent) isEvent()     {}
func (ScrollEvent) isEvent()                    {}
func (ScrollDiscreteEvent) isEvent()            {}
func (ScrollStopEvent) isEvent()                {}
func (ButtonEvent) isEvent()                    {}
func (KeyboardKeymapEvent) isEvent()            {}
func (KeyboardKeyEvent) isEvent()               {}
func (KeyboardModifiersEvent) isEvent()         {}

func (e HandshakeConnectionEvent) serialNumber() uint32     { return e.Serial }
func (e ConnectionDisconnectedEvent) serialNumber() uint32  { return e.LastSerial }
func (e ConnectionInvalidObjectEvent) serialNumber() uint32 { return e.LastSerial }
func (e DestroyedEvent) serialNumber() uint32               { return e.Serial }
func (e DeviceResumedEvent) serialNumber() uint32           { return e.Serial }
func (e DevicePausedEvent) serialNumber() uint32            { return e.Serial }
func (e DeviceStartEmulatingEvent) serialNumber() uint32    { return e.Serial }
func (e DeviceStopEmulatingEvent) serialNumber() uint32     { return e.Serial }
func (e DeviceFrameEvent) serialNumber() uint32             { return e.Serial }
func (e KeyboardModifiersEvent) serialNumber() uint32       { return e.Serial }

func isReceiverOnly(ev Event) bool {
	switch ev.(type) {
	case DeviceStartEmulatingEvent, DeviceStopEmulatingEvent, DeviceFrameEvent,
		PointerMotionRelativeEvent, PointerMotionAbsoluteEvent,
		ScrollEvent, ScrollDiscreteEvent, ScrollStopEvent,
		ButtonEvent, KeyboardKeyEvent:
		return true
	default:
		return false
	}
}

func DecodeEvent(iface Interface, f Frame, fds FDSource) (Event, error) {
	d := NewDecoder(f.Payload, fds)
	ev, err := decodeEventBody(iface, f.Opcode, d)
	if err != nil {
		return nil, err
	}
	if err := d.Finish(); err != nil {
		closeEventFiles(ev)
		return nil, fmt.Errorf("%s event opcode %d: %w", iface, f.Opcode, err)
	}
	return ev, nil
}

func closeEventFiles(ev Event) {
	if k, ok := ev.(KeyboardKeymapEvent); ok && k.FD != nil {
		_ = k.FD.Close()
	}
}

func unknownOpcode(iface Interface, opcode uint32) error {
	return fmt.Errorf("%w: %s event %d", ErrUnknownOpcode, iface, opcode)
}

func decodeEventBody(iface Interface, opcode uint32, d *Decoder) (Event, error) {
	switch iface {
	case InterfaceHandshake:
		return decodeHandshakeEvent(opcode, d)
	case InterfaceConnection:
		return decodeConnectionEvent(opcode, d)
	case InterfaceCallback:
		if opcode == evCallbackDone {
			return CallbackDoneEvent{Data: d.Uint64()}, d.Err()
		}
	case InterfacePingpong:
		return nil, unknownOpcode(iface, opcode)
	case InterfaceSeat:
		return decodeSeatEvent(opcode, d)
	case InterfaceDevice:
		return decodeDeviceEvent(opcode, d)
	case InterfacePointer:
		switch opcode {
		case evPointerDestroyed:
			return DestroyedEvent{Serial: d.Uint32()}, d.Err()
		case evPointerMotionRelative:
			return PointerMotionRelativeEvent{X: d.Float32(), Y: d.Float32()}, d.Err()
		}
	case InterfacePointerAbsolute:
		switch opcode {
		case evPointerAbsoluteDestroyed:
			return DestroyedEvent{Serial: d.Uint32()}, d.Err()
		case evPointerAbsoluteMotionAbsolute:
			return PointerMotionAbsoluteEvent{X: d.Float32(), Y: d.Float32()}, d.Err()
		}
	case InterfaceScroll:
		switch opcode {
		case evScrollDestroyed:
			return DestroyedEvent{Serial: d.Uint32()}, d.Err()
		case evScrollScroll:
			return ScrollEvent{X: d.Float32(), Y: d.Float32()}, d.Err()
		case evScrollScrollDiscrete:
			return ScrollDiscreteEvent{X: d.Int32(), Y: d.Int32()}, d.Err()
		case evScrollScrollStop:
			return ScrollStopEvent{X: d.Uint32(), Y: d.Uint32(), IsCancel: d.Uint32()}, d.Err()
		}
	case InterfaceButton:
		switch opcode {
		case evButtonDestroyed:
			return DestroyedEvent{Serial: d.Uint32()}, d.Err()
		case evButtonButton:
			return ButtonEvent{Button: d.Uint32(), State: ButtonState(d.Uint32())}, d.Err()
		}
	case InterfaceKeyboard:
		return decodeKeyboardEvent(opcode, d)
	}
	return nil, unknownOpcode(iface, opcode)
}

func decodeHandshakeEvent(opcode uint32, d *Decoder) (Event, error) {
	switch opcode {
	case evHandshakeVersion:
		return HandshakeVersionEvent{Version: d.Uint32()}, d.Err()
	case evHandshakeInterfaceVersion:
		return HandshakeInterfaceVersionEvent{Name: d.String(), Version: d.Uint32()}, d.Err()
	case evHandshakeConnection:
		return HandshakeConnectionEvent{Serial: d.Uint32(), Connection: d.NewID(), Version: d.Uint32()}, d.Err()
	default:
		return nil, unknownOpcode(InterfaceHandshake, opcode)
	}
}

func decodeConnectionEvent(opcode uint32, d *Decoder) (Event, error) {
	switch opcode {
	case evConnectionDisconnected:
		ev := ConnectionDisconnectedEvent{LastSerial: d.Uint32(), Reason: DisconnectReason(d.Uint32())}
		ev.Explanation, ev.HasExplanation = d.NullableString()
		return ev, d.Err()
	case evConnectionSeat:
		return ConnectionSeatEvent{Seat: d.NewID(), Version: d.Uint32()}, d.Err()
	case evConnectionInvalidObject:
		return ConnectionInvalidObjectEvent{LastSerial: d.Uint32(), InvalidID: d.Uint64()}, d.Err()
	case evConnectionPing:
		return ConnectionPingEvent{Ping: d.NewID(), Version: d.Uint32()}, d.Err()
	default:
		return nil, unknownOpcode(InterfaceConnection, opcode)
	}
}

func decodeSeatEvent(opcode uint32, d *Decoder) (Event, error) {
	switch opcode {
	case evSeatDestroyed:
		return DestroyedEvent{Serial: d.Uint32()}, d.Err()
	case evSeatName:
		return SeatNameEvent{Name: d.String()}, d.Err()
	case evSeatCapability:
		return SeatCapabilityEvent{Mask: d.Uint64(), Interface: d.String()}, d.Err()
	case evSeatDone:
		return SeatDoneEvent{}, d.Err()
	case evSeatDevice:
		return SeatDeviceEvent{Device: d.NewID(), Version: d.Uint32()}, d.Err()
	default:
		return nil, unknownOpcode(InterfaceSeat, opcode)
	}
}

func decodeDeviceEvent(opcode uint32, d *Decoder) (Event, error) {
	switch opcode {
	case evDeviceDestroyed:
		return DestroyedEvent{Serial: d.Uint32()}, d.Err()
	case evDeviceName:
		return DeviceNameEvent{Name: d.String()}, d.Err()
	case evDeviceDeviceType:
		return DeviceTypeEvent{Type: DeviceType(d.Uint32())}, d.Err()
	case evDeviceDimensions:
		return DeviceDimensionsEvent{Width: d.Uint32(), Height: d.Uint32()}, d.Err()
	case evDeviceRegion:
		return DeviceRegionEvent{
			OffsetX: d.Uint32(), OffsetY: d.Uint32(), Width: d.Uint32(), Height: d.Uint32(), Scale: d.Float32(),
		}, d.Err()
	case evDeviceInterface:
		return DeviceInterfaceEvent{Object: d.NewID(), Name: d.String(), Version: d.Uint32()}, d.Err()
	case evDeviceDone:
		return DeviceDoneEvent{}, d.Err()
	case evDeviceResumed:
		return DeviceResumedEvent{Serial: d.Uint32()}, d.Err()
	case evDevicePaused:
		return DevicePausedEvent{Serial: d.Uint32()}, d.Err()
	case evDeviceStartEmulating:
		return DeviceStartEmulatingEvent{Serial: d.Uint32(), Sequence: d.Uint32()}, d.Err()
	case evDeviceStopEmulating:
		return DeviceStopEmulatingEvent{Serial: d.Uint32()}, d.Err()
	case evDeviceFrame:
		return DeviceFrameEvent{Serial: d.Uint32(), Timestamp: d.Uint64()}, d.Err()
	case evDeviceRegionMappingID:
		return DeviceRegionMappingIDEvent{MappingID: d.String()}, d.Err()
	default:
		return nil, unknownOpcode(InterfaceDevice, opcode)
	}
}

func decodeKeyboardEvent(opcode uint32, d *Decoder) (Event, error) {
	switch opcode {
	case evKeyboardDestroyed:
		return DestroyedEvent{Serial: d.Uint32()}, d.Err()
	case evKeyboardKeymap:
		ev := KeyboardKeymapEvent{Type: KeymapType(d.Uint32()), Size: d.Uint32()}
		ev.FD = d.FD()
		return ev, d.Err()
	case evKeyboardKey:
		return KeyboardKeyEvent{Key: d.Uint32(), State: KeyState(d.Uint32())}, d.Err()
	case evKeyboardModifiers:
		return KeyboardModifiersEvent{
			Serial: d.Uint32(), Depressed: d.Uint32(), Locked: d.Uint32(), Latched: d.Uint32(), Group: d.Uint32(),
		}, d.Err()
	default:
		return nil, unknownOpcode(InterfaceKeyboard, opcode)
	}
}

func handshakeVersionRequest(version uint32) Frame {
	var e Encoder
	e.Uint32(version)
	return e.Frame(HandshakeObjectID, opHandshakeVersion)
}

func handshakeFinishRequest() Frame {
	return Frame{ObjectID: HandshakeObjectID, Opcode: opHandshakeFinish}
}

func handshakeContextTypeRequest(role Role) Frame {
	var e Encoder
	e.Uint32(uint32(role))
	return e.Frame(HandshakeObjectID, opHandshakeContextType)
}

func handshakeNameRequest(name string) (Frame, error) {
	var e Encoder
	if err := e.String(name); err != nil {
		return Frame{}, err
	}
	return e.Frame(HandshakeObjectID, opHandshakeName), nil
}

func handshakeInterfaceVersionRequest(name string, version uint32) (Frame, error) {
	var e Encoder
	if err := e.String(name); err != nil {
		return Frame{}, err
	}
	e.Uint32(version)
	return e.Frame(HandshakeObjectID, opHandshakeInterfaceVersion), nil
}

func connectionSyncRequest(connection, callback uint64, version uint32) Frame {
	var e Encoder
	e.NewID(callback)
	e.Uint32(version)
	return e.Frame(connection, opConnectionSync)
}

func connectionDisconnectRequest(connection uint64) Frame {
	return Frame{ObjectID: connection, Opcode: opConnectionDisconnect}
}

func pingpongDoneRequest(pingpong, data uint64) Frame {
	var e Encoder
	e.Uint64(data)
	return e.Frame(pingpong, opPingpongDone)
}

func releaseRequest(object uint64) Frame {
	return Frame{ObjectID: object, Opcode: opRelease}
}

func seatBindRequest(seat, capabilities uint64) Frame {
	var e Encoder
	e.Uint64(capabilities)
	return e.Frame(seat, opSeatBind)
}

func deviceStartEmulatingRequest(device uint64, lastSerial, sequence uint32) Frame {
	var e Encoder
	e.Uint32(lastSerial)
	e.Uint32(sequence)
	return e.Frame(device, opDeviceStartEmulating)
}

func deviceStopEmulatingRequest(device uint64, lastSerial uint32) Frame {
	var e Encoder
	e.Uint32(lastSerial)
	return e.Frame(device, opDeviceStopEmulating)
}

func deviceFrameRequest(device uint64, lastSerial uint32, timestamp uint64) Frame {
	var e Encoder
	e.Uint32(lastSerial)
	e.Uint64(timestamp)
	return e.Frame(device, opDeviceFrame)
}

func deviceReadyRequest(device uint64) Frame {
	return Frame{ObjectID: device, Opcode: opDeviceReady}
}

func pointerMotionRelativeRequest(pointer uint64, x, y float32) Frame {
	var e Encoder
	e.Float32(x)
	e.Float32(y)
	return e.Frame(pointer, opPointerMotionRelative)
}

func pointerMotionAbsoluteRequest(pointer uint64, x, y float32) Frame {
	var e Encoder
	e.Float32(x)
	e.Float32(y)
	return e.Frame(pointer, opPointerAbsoluteMotionAbsolute)
}

func scrollRequest(scroll uint64, x, y float32) Frame {
	var e Encoder
	e.Float32(x)
	e.Float32(y)
	return e.Frame(scroll, opScrollScroll)
}

func scrollDiscreteRequest(scroll uint64, x, y int32) Frame {
	var e Encoder
	e.Int32(x)
	e.Int32(y)
	return e.Frame(scroll, opScrollScrollDiscrete)
}

func scrollStopRequest(scroll uint64, x, y, isCancel uint32) Frame {
	var e Encoder
	e.Uint32(x)
	e.Uint32(y)
	e.Uint32(isCancel)
	return e.Frame(scroll, opScrollScrollStop)
}

func buttonRequest(button uint64, code uint32, state ButtonState) Frame {
	var e Encoder
	e.Uint32(code)
	e.Uint32(uint32(state))
	return e.Frame(button, opButtonButton)
}

func keyRequest(keyboard uint64, code uint32, state KeyState) Frame {
	var e Encoder
	e.Uint32(code)
	e.Uint32(uint32(state))
	return e.Frame(keyboard, opKeyboardKey)
}
