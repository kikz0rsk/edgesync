package ei

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
)

var (
	ErrUnknownObject = errors.New("ei: unknown object")
	ErrWrongObject   = errors.New("ei: object has a different interface")
	ErrWrongRole     = errors.New("ei: request not allowed for this context type")
	ErrDisconnected  = errors.New("ei: disconnected by EIS")
	ErrPeerClosed    = errors.New("ei: connection closed by peer")
	ErrClientClosed  = errors.New("ei: client closed")
)

type DisconnectError struct {
	Reason         DisconnectReason
	Explanation    string
	HasExplanation bool
}

func (e *DisconnectError) Error() string {
	if e.HasExplanation {
		return fmt.Sprintf("ei: disconnected by EIS (reason %d): %s", e.Reason, e.Explanation)
	}
	return fmt.Sprintf("ei: disconnected by EIS (reason %d)", e.Reason)
}

func (e *DisconnectError) Unwrap() error {
	return ErrDisconnected
}

type Inbound struct {
	Object    uint64
	Interface Interface
	Event     Event
}

type objectInfo struct {
	iface   Interface
	version uint32
}

type Client struct {
	t      Transport
	rd     *bufio.Reader
	role   Role
	result HandshakeResult

	events   chan Inbound
	done     chan struct{}
	loopDone chan struct{}

	writeMu sync.Mutex

	mu      sync.Mutex
	objects map[uint64]objectInfo
	waiters map[uint64]chan struct{}
	nextID  uint64
	loopErr error
	closing bool

	lastSerial atomic.Uint32
	sequence   atomic.Uint32
	closeOnce  sync.Once
	closeErr   error
}

func Connect(ctx context.Context, t Transport, cfg Config) (*Client, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	closed := make(chan error, 1)
	stop := context.AfterFunc(ctx, func() { closed <- t.Close() })
	rd := bufio.NewReader(t)
	result, err := performHandshake(rd, t, cfg)
	if !stop() {
		return nil, errors.Join(context.Cause(ctx), err, <-closed)
	}
	if err != nil {
		return nil, errors.Join(err, t.Close())
	}

	c := &Client{
		t:        t,
		rd:       rd,
		role:     cfg.Role,
		result:   result,
		events:   make(chan Inbound, 64),
		done:     make(chan struct{}),
		loopDone: make(chan struct{}),
		objects:  map[uint64]objectInfo{result.Connection: {iface: InterfaceConnection, version: result.ConnectionVersion}},
		waiters:  map[uint64]chan struct{}{},
		nextID:   FirstClientObjectID,
	}
	c.lastSerial.Store(result.Serial)
	go c.run()
	return c, nil
}

func (c *Client) Handshake() HandshakeResult {
	return c.result
}

func (c *Client) Role() Role {
	return c.role
}

func (c *Client) Events() <-chan Inbound {
	return c.events
}

func (c *Client) Err() error {
	<-c.loopDone
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.loopErr
}

func (c *Client) Close() error {
	c.closeOnce.Do(func() {
		c.mu.Lock()
		c.closing = true
		c.mu.Unlock()

		var sendErr error
		select {
		case <-c.loopDone:
		default:
			sendErr = c.send(connectionDisconnectRequest(c.result.Connection))
		}
		close(c.done)
		c.closeErr = errors.Join(sendErr, c.t.Close())
		<-c.loopDone
	})
	return c.closeErr
}

func (c *Client) run() {
	defer close(c.loopDone)
	defer close(c.events)
	err := c.dispatch()
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closing {
		return
	}
	c.loopErr = err
}

func (c *Client) dispatch() error {
	for {
		f, err := ReadFrame(c.rd)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return fmt.Errorf("%w: %w", ErrPeerClosed, err)
			}
			return fmt.Errorf("ei: read: %w", err)
		}
		if err := c.handleFrame(f); err != nil {
			return err
		}
	}
}

func (c *Client) lookup(id uint64) (objectInfo, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	info, ok := c.objects[id]
	return info, ok
}

func (c *Client) handleFrame(f Frame) error {
	info, ok := c.lookup(f.ObjectID)
	if !ok {
		return fmt.Errorf("%w: id %#x opcode %d", ErrUnknownObject, f.ObjectID, f.Opcode)
	}
	ev, err := DecodeEvent(info.iface, f, c.t)
	if err != nil {
		return fmt.Errorf("ei: decode %s %#x: %w", info.iface, f.ObjectID, err)
	}
	if c.role == RoleSender && isReceiverOnly(ev) {
		closeEventFiles(ev)
		return fmt.Errorf("%w: %T sent to a sender client", ErrProtocol, ev)
	}
	if s, ok := ev.(serialEvent); ok {
		c.lastSerial.Store(s.serialNumber())
	}
	return c.apply(f.ObjectID, info, ev)
}

func (c *Client) apply(id uint64, info objectInfo, ev Event) error {
	switch ev := ev.(type) {
	case ConnectionSeatEvent:
		if err := c.register(ev.Seat, InterfaceSeat, ev.Version); err != nil {
			return err
		}
	case SeatDeviceEvent:
		if err := c.register(ev.Device, InterfaceDevice, ev.Version); err != nil {
			return err
		}
	case DeviceInterfaceEvent:
		iface, ok := InterfaceByName(ev.Name)
		if !ok {
			return fmt.Errorf("%w: device announced %q", ErrUnsupportedInterface, ev.Name)
		}
		if err := c.register(ev.Object, iface, ev.Version); err != nil {
			return err
		}
	case ConnectionPingEvent:
		return c.answerPing(ev)
	case CallbackDoneEvent:
		c.completeCallback(id)
		return nil
	case DeviceDoneEvent:
		if info.version >= 3 {
			if err := c.send(deviceReadyRequest(id)); err != nil {
				return err
			}
		}
	}

	if err := c.deliver(Inbound{Object: id, Interface: info.iface, Event: ev}); err != nil {
		return err
	}

	switch ev := ev.(type) {
	case DestroyedEvent:
		c.unregister(id)
	case ConnectionDisconnectedEvent:
		return &DisconnectError{Reason: ev.Reason, Explanation: ev.Explanation, HasExplanation: ev.HasExplanation}
	}
	return nil
}

func (c *Client) deliver(in Inbound) error {
	select {
	case c.events <- in:
		return nil
	case <-c.done:
		return ErrClientClosed
	}
}

func (c *Client) register(id uint64, iface Interface, version uint32) error {
	if id < FirstServerObjectID {
		return fmt.Errorf("%w: new %s id %#x is below the EIS id range", ErrProtocol, iface, id)
	}
	limit := c.result.negotiatedLimit(iface)
	if version < 1 || version > limit {
		return fmt.Errorf("%w: %s version %d outside 1..%d", ErrProtocol, iface, version, limit)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, exists := c.objects[id]; exists {
		return fmt.Errorf("%w: object id %#x announced twice", ErrProtocol, id)
	}
	c.objects[id] = objectInfo{iface: iface, version: version}
	return nil
}

func (c *Client) unregister(id uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.objects, id)
}

func (c *Client) answerPing(ev ConnectionPingEvent) error {
	if err := c.register(ev.Ping, InterfacePingpong, ev.Version); err != nil {
		return err
	}
	defer c.unregister(ev.Ping)
	return c.send(pingpongDoneRequest(ev.Ping, 0))
}

func (c *Client) completeCallback(id uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.objects, id)
	if ch, ok := c.waiters[id]; ok {
		close(ch)
		delete(c.waiters, id)
	}
}

func (c *Client) send(f Frame) error {
	buf, err := f.AppendTo(nil)
	if err != nil {
		return err
	}
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if len(f.FDs) > 0 {
		return c.t.WriteFDs(buf, f.FDs)
	}
	if _, err := c.t.Write(buf); err != nil {
		return fmt.Errorf("ei: write: %w", err)
	}
	return nil
}

func (c *Client) expect(id uint64, want Interface) error {
	info, ok := c.lookup(id)
	if !ok {
		return fmt.Errorf("%w: id %#x", ErrUnknownObject, id)
	}
	if info.iface != want {
		return fmt.Errorf("%w: id %#x is %s, want %s", ErrWrongObject, id, info.iface, want)
	}
	return nil
}

func (c *Client) requireSender() error {
	if c.role != RoleSender {
		return ErrWrongRole
	}
	return nil
}

func (c *Client) Sync(ctx context.Context) error {
	c.mu.Lock()
	id := c.nextID
	c.nextID++
	ch := make(chan struct{})
	c.objects[id] = objectInfo{iface: InterfaceCallback, version: 1}
	c.waiters[id] = ch
	c.mu.Unlock()

	if err := c.send(connectionSyncRequest(c.result.Connection, id, 1)); err != nil {
		c.completeCallback(id)
		return err
	}
	select {
	case <-ch:
		return nil
	case <-ctx.Done():
		return context.Cause(ctx)
	case <-c.loopDone:
		if err := c.Err(); err != nil {
			return err
		}
		return ErrClientClosed
	}
}

func (c *Client) Release(object uint64) error {
	info, ok := c.lookup(object)
	if !ok {
		return fmt.Errorf("%w: id %#x", ErrUnknownObject, object)
	}
	switch info.iface {
	case InterfaceSeat, InterfaceDevice, InterfacePointer, InterfacePointerAbsolute,
		InterfaceScroll, InterfaceButton, InterfaceKeyboard:
		return c.send(releaseRequest(object))
	default:
		return fmt.Errorf("%w: %s has no release request", ErrWrongObject, info.iface)
	}
}

func (c *Client) Bind(seat, capabilities uint64) error {
	if err := c.expect(seat, InterfaceSeat); err != nil {
		return err
	}
	return c.send(seatBindRequest(seat, capabilities))
}

func (c *Client) StartEmulating(device uint64) error {
	if err := c.requireSender(); err != nil {
		return err
	}
	if err := c.expect(device, InterfaceDevice); err != nil {
		return err
	}
	return c.send(deviceStartEmulatingRequest(device, c.lastSerial.Load(), c.sequence.Add(1)))
}

func (c *Client) StopEmulating(device uint64) error {
	if err := c.requireSender(); err != nil {
		return err
	}
	if err := c.expect(device, InterfaceDevice); err != nil {
		return err
	}
	return c.send(deviceStopEmulatingRequest(device, c.lastSerial.Load()))
}

func (c *Client) SendFrame(device uint64, timestampMicros uint64) error {
	if err := c.requireSender(); err != nil {
		return err
	}
	if err := c.expect(device, InterfaceDevice); err != nil {
		return err
	}
	return c.send(deviceFrameRequest(device, c.lastSerial.Load(), timestampMicros))
}

func (c *Client) MotionRelative(pointer uint64, x, y float32) error {
	if err := c.requireSender(); err != nil {
		return err
	}
	if err := c.expect(pointer, InterfacePointer); err != nil {
		return err
	}
	return c.send(pointerMotionRelativeRequest(pointer, x, y))
}

func (c *Client) MotionAbsolute(pointer uint64, x, y float32) error {
	if err := c.requireSender(); err != nil {
		return err
	}
	if err := c.expect(pointer, InterfacePointerAbsolute); err != nil {
		return err
	}
	return c.send(pointerMotionAbsoluteRequest(pointer, x, y))
}

func (c *Client) Scroll(scroll uint64, x, y float32) error {
	if err := c.requireSender(); err != nil {
		return err
	}
	if err := c.expect(scroll, InterfaceScroll); err != nil {
		return err
	}
	return c.send(scrollRequest(scroll, x, y))
}

func (c *Client) ScrollDiscrete(scroll uint64, x, y int32) error {
	if err := c.requireSender(); err != nil {
		return err
	}
	if err := c.expect(scroll, InterfaceScroll); err != nil {
		return err
	}
	return c.send(scrollDiscreteRequest(scroll, x, y))
}

func (c *Client) ScrollStop(scroll uint64, x, y, isCancel bool) error {
	if err := c.requireSender(); err != nil {
		return err
	}
	if err := c.expect(scroll, InterfaceScroll); err != nil {
		return err
	}
	return c.send(scrollStopRequest(scroll, boolToUint32(x), boolToUint32(y), boolToUint32(isCancel)))
}

func (c *Client) Button(button uint64, code uint32, state ButtonState) error {
	if err := c.requireSender(); err != nil {
		return err
	}
	if err := c.expect(button, InterfaceButton); err != nil {
		return err
	}
	return c.send(buttonRequest(button, code, state))
}

func (c *Client) Key(keyboard uint64, code uint32, state KeyState) error {
	if err := c.requireSender(); err != nil {
		return err
	}
	if err := c.expect(keyboard, InterfaceKeyboard); err != nil {
		return err
	}
	return c.send(keyRequest(keyboard, code, state))
}

func boolToUint32(b bool) uint32 {
	if b {
		return 1
	}
	return 0
}
