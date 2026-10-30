// Package tap is the EdgeSync spike for a CGEventTap driven from a dedicated CFRunLoop thread through purego.
package tap

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"unsafe"

	"github.com/ebitengine/purego"

	"github.com/kikz0r/edgesync/spikes/macos/fw"
)

var (
	ErrTapCreate       = errors.New("CGEventTapCreate returned NULL")
	ErrSourceCreate    = errors.New("CFMachPortCreateRunLoopSource returned NULL")
	ErrInvalidLocation = errors.New("invalid event tap location")
	ErrListenOnly      = errors.New("a listen-only tap cannot suppress events")
	ErrDictionary      = errors.New("CFDictionaryCreate returned NULL")
)

type Location uint32

const (
	LocationHID              Location = 0
	LocationSession          Location = 1
	LocationAnnotatedSession Location = 2
)

type EventType uint32

const (
	EventLeftMouseDown          EventType = 1
	EventLeftMouseUp            EventType = 2
	EventRightMouseDown         EventType = 3
	EventRightMouseUp           EventType = 4
	EventMouseMoved             EventType = 5
	EventLeftMouseDragged       EventType = 6
	EventRightMouseDragged      EventType = 7
	EventKeyDown                EventType = 10
	EventKeyUp                  EventType = 11
	EventFlagsChanged           EventType = 12
	EventScrollWheel            EventType = 22
	EventOtherMouseDown         EventType = 25
	EventOtherMouseUp           EventType = 26
	EventOtherMouseDragged      EventType = 27
	EventTapDisabledByTimeout   EventType = 0xFFFFFFFE
	EventTapDisabledByUserInput EventType = 0xFFFFFFFF
)

const (
	placementHeadInsert uint32 = 0
	optionDefault       uint32 = 0
	optionListenOnly    uint32 = 1

	fieldMouseEventDeltaX     uint32 = 4
	fieldMouseEventDeltaY     uint32 = 5
	fieldKeyboardEventKeycode uint32 = 9
	FieldEventSourceUserData  uint32 = 42
)

var maskedEventTypes = [...]EventType{
	EventLeftMouseDown, EventLeftMouseUp, EventRightMouseDown, EventRightMouseUp,
	EventMouseMoved, EventLeftMouseDragged, EventRightMouseDragged,
	EventKeyDown, EventKeyUp, EventFlagsChanged,
	EventScrollWheel,
	EventOtherMouseDown, EventOtherMouseUp, EventOtherMouseDragged,
}

func eventMask() uint64 {
	var mask uint64
	for _, eventType := range maskedEventTypes {
		mask |= 1 << eventType
	}
	return mask
}

type Config struct {
	Location   Location
	ListenOnly bool
	// Buffer is the capacity of the Events channel; zero disables event delivery and only counts events.
	Buffer int
}

type Event struct {
	Type     EventType
	Location fw.CGPoint
	DeltaX   int64
	DeltaY   int64
	Keycode  int64
	UserData int64
}

type Stats struct {
	Events              uint64
	Dropped             uint64
	Suppressed          uint64
	DisabledByTimeout   uint64
	DisabledByUserInput uint64
	Reenabled           uint64
	ReenableFailures    uint64
}

type counters struct {
	events              atomic.Uint64
	dropped             atomic.Uint64
	suppressed          atomic.Uint64
	disabledByTimeout   atomic.Uint64
	disabledByUserInput atomic.Uint64
	reenabled           atomic.Uint64
	reenableFailures    atomic.Uint64
}

type tapAPI struct {
	create                    func(location, placement, options uint32, mask uint64, callback, refcon uintptr) uintptr
	enable                    func(port uintptr, enable bool)
	isEnabled                 func(port uintptr) bool
	eventGetLocation          func(event uintptr) fw.CGPoint
	eventGetIntegerValueField func(event uintptr, field uint32) int64
}

func bindTapAPI(lib *fw.Lib) (*tapAPI, error) {
	s := symbols{handle: lib.Handles.CoreGraphics}
	api := &tapAPI{}
	s.bind(&api.create, "CGEventTapCreate")
	s.bind(&api.enable, "CGEventTapEnable")
	s.bind(&api.isEnabled, "CGEventTapIsEnabled")
	s.bind(&api.eventGetLocation, "CGEventGetLocation")
	s.bind(&api.eventGetIntegerValueField, "CGEventGetIntegerValueField")
	if s.err != nil {
		return nil, s.err
	}
	return api, nil
}

type Tap struct {
	api        *tapAPI
	loopAPI    *loopAPI
	loop       *loop
	refcon     uintptr
	port       uintptr
	listenOnly bool
	suppress   atomic.Bool
	events     chan Event
	stats      counters
	closeOnce  sync.Once
}

// dispatch exists because purego callbacks are a finite, never freed resource (D-80): one trampoline is created
// for the whole process and finds its tap through the refcon handle.
var dispatch struct {
	once     sync.Once
	callback uintptr
	mu       sync.RWMutex
	next     uintptr
	taps     map[uintptr]*Tap
}

func callbackAddress() uintptr {
	dispatch.once.Do(func() {
		dispatch.callback = purego.NewCallback(trampoline)
		dispatch.taps = make(map[uintptr]*Tap)
	})
	return dispatch.callback
}

func register(t *Tap) uintptr {
	callbackAddress()
	dispatch.mu.Lock()
	defer dispatch.mu.Unlock()
	dispatch.next++
	dispatch.taps[dispatch.next] = t
	return dispatch.next
}

func unregister(handle uintptr) {
	dispatch.mu.Lock()
	defer dispatch.mu.Unlock()
	delete(dispatch.taps, handle)
}

func registeredTaps() int {
	callbackAddress()
	dispatch.mu.RLock()
	defer dispatch.mu.RUnlock()
	return len(dispatch.taps)
}

func trampoline(_ uintptr, eventType uint32, event, refcon uintptr) uintptr {
	dispatch.mu.RLock()
	t := dispatch.taps[refcon]
	dispatch.mu.RUnlock()
	if t == nil {
		return event
	}
	return t.handleEvent(EventType(eventType), event)
}

func Start(ctx context.Context, lib *fw.Lib, cfg Config) (*Tap, error) {
	if cfg.Location > LocationAnnotatedSession {
		return nil, fmt.Errorf("location %d: %w", cfg.Location, ErrInvalidLocation)
	}
	api, err := bindTapAPI(lib)
	if err != nil {
		return nil, err
	}
	loops, err := bindLoopAPI(lib)
	if err != nil {
		return nil, err
	}

	t := &Tap{api: api, loopAPI: loops, listenOnly: cfg.ListenOnly}
	if cfg.Buffer > 0 {
		t.events = make(chan Event, cfg.Buffer)
	}
	t.refcon = register(t)

	l, err := startLoop(ctx, loops, func() (uintptr, func(), error) { return t.install(cfg) })
	if err != nil {
		unregister(t.refcon)
		return nil, err
	}
	t.loop = l
	return t, nil
}

func (t *Tap) install(cfg Config) (uintptr, func(), error) {
	options := optionDefault
	if cfg.ListenOnly {
		options = optionListenOnly
	}
	port := t.api.create(uint32(cfg.Location), placementHeadInsert, options, eventMask(), callbackAddress(), t.refcon)
	if port == 0 {
		return 0, nil, ErrTapCreate
	}
	source := t.loopAPI.createRunLoopSource(0, port, 0)
	if source == 0 {
		t.loopAPI.machPortInvalidate(port)
		t.loopAPI.release(port)
		return 0, nil, ErrSourceCreate
	}
	t.port = port
	teardown := func() {
		t.loopAPI.machPortInvalidate(port)
		t.loopAPI.release(source)
		t.loopAPI.release(port)
	}
	return source, teardown, nil
}

func (t *Tap) handleEvent(eventType EventType, event uintptr) uintptr {
	switch eventType {
	case EventTapDisabledByTimeout:
		t.stats.disabledByTimeout.Add(1)
		t.reenable()
		return event
	case EventTapDisabledByUserInput:
		t.stats.disabledByUserInput.Add(1)
		t.reenable()
		return event
	}

	t.stats.events.Add(1)
	if t.events != nil {
		t.publish(eventType, event)
	}
	if t.suppress.Load() {
		t.stats.suppressed.Add(1)
		return 0
	}
	return event
}

func (t *Tap) reenable() {
	t.api.enable(t.port, true)
	if t.api.isEnabled(t.port) {
		t.stats.reenabled.Add(1)
		return
	}
	t.stats.reenableFailures.Add(1)
}

func (t *Tap) publish(eventType EventType, event uintptr) {
	e := Event{
		Type:     eventType,
		Location: t.api.eventGetLocation(event),
		DeltaX:   t.api.eventGetIntegerValueField(event, fieldMouseEventDeltaX),
		DeltaY:   t.api.eventGetIntegerValueField(event, fieldMouseEventDeltaY),
		Keycode:  t.api.eventGetIntegerValueField(event, fieldKeyboardEventKeycode),
		UserData: t.api.eventGetIntegerValueField(event, FieldEventSourceUserData),
	}
	select {
	case t.events <- e:
	default:
		t.stats.dropped.Add(1)
	}
}

func (t *Tap) Events() <-chan Event {
	return t.events
}

func (t *Tap) Enabled() bool {
	return t.api.isEnabled(t.port)
}

func (t *Tap) SetSuppress(on bool) error {
	if on && t.listenOnly {
		return ErrListenOnly
	}
	t.suppress.Store(on)
	return nil
}

func (t *Tap) Stats() Stats {
	return Stats{
		Events:              t.stats.events.Load(),
		Dropped:             t.stats.dropped.Load(),
		Suppressed:          t.stats.suppressed.Load(),
		DisabledByTimeout:   t.stats.disabledByTimeout.Load(),
		DisabledByUserInput: t.stats.disabledByUserInput.Load(),
		Reenabled:           t.stats.reenabled.Load(),
		ReenableFailures:    t.stats.reenableFailures.Load(),
	}
}

func (t *Tap) Close() {
	t.closeOnce.Do(func() {
		t.loop.stop()
		unregister(t.refcon)
	})
}

type cursorAPI struct {
	associate  func(connected int32) int32
	hideCursor func(display uint32) int32
	showCursor func(display uint32) int32
}

// Cursor wraps the cursor detach and hide calls; every method returns the raw CGError.
type Cursor struct {
	api *cursorAPI
}

func NewCursor(lib *fw.Lib) (*Cursor, error) {
	s := symbols{handle: lib.Handles.CoreGraphics}
	api := &cursorAPI{}
	s.bind(&api.associate, "CGAssociateMouseAndMouseCursorPosition")
	s.bind(&api.hideCursor, "CGDisplayHideCursor")
	s.bind(&api.showCursor, "CGDisplayShowCursor")
	if s.err != nil {
		return nil, s.err
	}
	return &Cursor{api: api}, nil
}

func (c *Cursor) Associate(connected bool) int32 {
	var flag int32
	if connected {
		flag = 1
	}
	return c.api.associate(flag)
}

func (c *Cursor) Hide(display uint32) int32 {
	return c.api.hideCursor(display)
}

func (c *Cursor) Show(display uint32) int32 {
	return c.api.showCursor(display)
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

func (s *symbols) address(name string) uintptr {
	if s.err != nil {
		return 0
	}
	addr, err := purego.Dlsym(s.handle, name)
	if err != nil {
		s.err = fmt.Errorf("resolve %s: %w", name, err)
		return 0
	}
	return addr
}

// pointerVariable reads an exported pointer-typed global such as kCFRunLoopCommonModes.
func (s *symbols) pointerVariable(name string) uintptr {
	addr := s.address(name)
	if addr == 0 {
		return 0
	}
	p := *(*unsafe.Pointer)(unsafe.Pointer(&addr))
	return *(*uintptr)(p)
}
