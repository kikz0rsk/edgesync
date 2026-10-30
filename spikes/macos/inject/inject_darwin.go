// Package inject is the EdgeSync spike for synthetic mouse, keyboard and scroll input posted with CGEventPost
// through purego, without cgo.
package inject

import (
	"errors"
	"fmt"
	"sync/atomic"

	"github.com/ebitengine/purego"

	"github.com/kikz0r/edgesync/spikes/macos/fw"
)

var (
	ErrSourceCreate      = errors.New("CGEventSourceCreate returned NULL")
	ErrEventCreate       = errors.New("CGEvent creation returned NULL")
	ErrInvalidLocation   = errors.New("invalid event tap location")
	ErrInvalidButton     = errors.New("invalid mouse button")
	ErrInvalidScrollUnit = errors.New("invalid scroll unit")
	ErrClosed            = errors.New("injector is closed")
)

type Location uint32

const (
	LocationHID     Location = 0
	LocationSession Location = 1
)

type Button uint32

const (
	ButtonLeft   Button = 0
	ButtonRight  Button = 1
	ButtonCenter Button = 2
)

type ScrollUnit uint32

const (
	ScrollUnitPixel ScrollUnit = 0
	ScrollUnitLine  ScrollUnit = 1
)

type Flags uint64

const (
	FlagShift   Flags = 0x00020000
	FlagControl Flags = 0x00040000
	FlagOption  Flags = 0x00080000
	FlagCommand Flags = 0x00100000
)

const KeyA uint16 = 0x00

const (
	stateCombinedSessionState int32 = 0

	eventLeftMouseDown  uint32 = 1
	eventLeftMouseUp    uint32 = 2
	eventRightMouseDown uint32 = 3
	eventRightMouseUp   uint32 = 4
	eventMouseMoved     uint32 = 5
	eventOtherMouseDown uint32 = 25
	eventOtherMouseUp   uint32 = 26

	fieldMouseEventNumber    uint32 = 0
	fieldMouseClickState     uint32 = 1
	fieldMouseButtonNumber   uint32 = 3
	fieldMouseEventDeltaX    uint32 = 4
	fieldMouseEventDeltaY    uint32 = 5
	fieldKeyboardKeycode     uint32 = 9
	fieldEventSourceUserData uint32 = 42

	scrollWheelCount uint32 = 2
)

type api struct {
	sourceCreate   func(stateID int32) uintptr
	mouseCreate    func(source uintptr, eventType uint32, at fw.CGPoint, button uint32) uintptr
	keyboardCreate func(source uintptr, keycode uint16, keyDown bool) uintptr
	scrollCreate   func(source uintptr, units uint32, wheelCount uint32, wheel1, wheel2, wheel3 int32) uintptr
	setFlags       func(event uintptr, flags uint64)
	getFlags       func(event uintptr) uint64
	setInt         func(event uintptr, field uint32, value int64)
	getInt         func(event uintptr, field uint32) int64
	post           func(location uint32, event uintptr)
}

func bindAPI(lib *fw.Lib) (*api, error) {
	s := symbols{handle: lib.Handles.CoreGraphics}
	a := &api{}
	s.bind(&a.sourceCreate, "CGEventSourceCreate")
	s.bind(&a.mouseCreate, "CGEventCreateMouseEvent")
	s.bind(&a.keyboardCreate, "CGEventCreateKeyboardEvent")
	s.bind(&a.scrollCreate, "CGEventCreateScrollWheelEvent2")
	s.bind(&a.setFlags, "CGEventSetFlags")
	s.bind(&a.getFlags, "CGEventGetFlags")
	s.bind(&a.setInt, "CGEventSetIntegerValueField")
	s.bind(&a.getInt, "CGEventGetIntegerValueField")
	s.bind(&a.post, "CGEventPost")
	if s.err != nil {
		return nil, s.err
	}
	return a, nil
}

type symbols struct {
	handle uintptr
	err    error
}

func (s *symbols) bind(fptr any, name string) {
	if s.err != nil {
		return
	}
	addr, err := purego.Dlsym(s.handle, name)
	if err != nil {
		s.err = fmt.Errorf("resolve %s: %w", name, err)
		return
	}
	purego.RegisterFunc(fptr, addr)
}

type Config struct {
	Location Location
	UserData int64
}

type Injector struct {
	api         *api
	release     func(ref uintptr)
	source      uintptr
	config      Config
	eventNumber atomic.Int64
	closed      atomic.Bool
}

func New(lib *fw.Lib, cfg Config) (*Injector, error) {
	if cfg.Location > LocationSession {
		return nil, fmt.Errorf("location %d: %w", cfg.Location, ErrInvalidLocation)
	}
	a, err := bindAPI(lib)
	if err != nil {
		return nil, err
	}
	source := a.sourceCreate(stateCombinedSessionState)
	if source == 0 {
		return nil, ErrSourceCreate
	}
	return &Injector{api: a, release: lib.CoreFoundation.Release, source: source, config: cfg}, nil
}

func (in *Injector) Close() {
	if in.closed.CompareAndSwap(false, true) {
		in.release(in.source)
	}
}

func (in *Injector) Move(to fw.CGPoint, dx, dy int64) error {
	return in.send(in.mouseMove(to, dx, dy))
}

func (in *Injector) Button(at fw.CGPoint, b Button, down bool, clickState int64) error {
	return in.send(in.mouseButton(at, b, down, clickState))
}

func (in *Injector) Key(keycode uint16, down bool, flags Flags) error {
	return in.send(in.key(keycode, down, flags))
}

func (in *Injector) Scroll(unit ScrollUnit, dx, dy int32) error {
	return in.send(in.scroll(unit, dx, dy))
}

func (in *Injector) send(event uintptr, err error) error {
	if err != nil {
		return err
	}
	in.api.post(uint32(in.config.Location), event)
	in.release(event)
	return nil
}

func (in *Injector) check() error {
	if in.closed.Load() {
		return ErrClosed
	}
	return nil
}

func (in *Injector) tag(event uintptr) {
	in.api.setInt(event, fieldEventSourceUserData, in.config.UserData)
}

func (in *Injector) mouseEvent(eventType uint32, at fw.CGPoint, button Button) (uintptr, error) {
	if err := in.check(); err != nil {
		return 0, err
	}
	event := in.api.mouseCreate(in.source, eventType, at, uint32(button))
	if event == 0 {
		return 0, fmt.Errorf("CGEventCreateMouseEvent: %w", ErrEventCreate)
	}
	in.tag(event)
	return event, nil
}

func (in *Injector) mouseMove(to fw.CGPoint, dx, dy int64) (uintptr, error) {
	event, err := in.mouseEvent(eventMouseMoved, to, ButtonLeft)
	if err != nil {
		return 0, err
	}
	in.api.setInt(event, fieldMouseEventDeltaX, dx)
	in.api.setInt(event, fieldMouseEventDeltaY, dy)
	return event, nil
}

func (in *Injector) mouseButton(at fw.CGPoint, b Button, down bool, clickState int64) (uintptr, error) {
	eventType, err := buttonEventType(b, down)
	if err != nil {
		return 0, err
	}
	event, err := in.mouseEvent(eventType, at, b)
	if err != nil {
		return 0, err
	}
	in.api.setInt(event, fieldMouseClickState, clickState)
	in.api.setInt(event, fieldMouseEventNumber, in.eventNumber.Add(1))
	in.api.setInt(event, fieldMouseButtonNumber, int64(b))
	return event, nil
}

func buttonEventType(b Button, down bool) (uint32, error) {
	switch b {
	case ButtonLeft:
		if down {
			return eventLeftMouseDown, nil
		}
		return eventLeftMouseUp, nil
	case ButtonRight:
		if down {
			return eventRightMouseDown, nil
		}
		return eventRightMouseUp, nil
	case ButtonCenter:
		if down {
			return eventOtherMouseDown, nil
		}
		return eventOtherMouseUp, nil
	}
	return 0, fmt.Errorf("button %d: %w", b, ErrInvalidButton)
}

func (in *Injector) key(keycode uint16, down bool, flags Flags) (uintptr, error) {
	if err := in.check(); err != nil {
		return 0, err
	}
	event := in.api.keyboardCreate(in.source, keycode, down)
	if event == 0 {
		return 0, fmt.Errorf("CGEventCreateKeyboardEvent: %w", ErrEventCreate)
	}
	in.api.setFlags(event, uint64(flags))
	in.tag(event)
	return event, nil
}

func (in *Injector) scroll(unit ScrollUnit, dx, dy int32) (uintptr, error) {
	if err := in.check(); err != nil {
		return 0, err
	}
	if unit != ScrollUnitPixel && unit != ScrollUnitLine {
		return 0, fmt.Errorf("scroll unit %d: %w", unit, ErrInvalidScrollUnit)
	}
	event := in.api.scrollCreate(in.source, uint32(unit), scrollWheelCount, dy, dx, 0)
	if event == 0 {
		return 0, fmt.Errorf("CGEventCreateScrollWheelEvent2: %w", ErrEventCreate)
	}
	in.tag(event)
	return event, nil
}
