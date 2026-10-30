package portal

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"

	"github.com/godbus/dbus/v5"
)

var (
	ErrSessionClosed = errors.New("portal: session closed")
	ErrStreamClosed  = errors.New("portal: event stream closed")
)

const (
	methodSessionClose = interfaceSession + ".Close"
	signalSessionClose = "Closed"
	methodGetProperty  = "org.freedesktop.DBus.Properties.Get"
	propertyVersion    = "version"
)

type Session struct {
	bus  Bus
	path dbus.ObjectPath

	mu     sync.Mutex
	closed bool
}

func newSession(bus Bus, path dbus.ObjectPath) *Session {
	return &Session{bus: bus, path: path}
}

func (s *Session) Path() dbus.ObjectPath {
	return s.path
}

func (s *Session) ensureOpen() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return ErrSessionClosed
	}
	return nil
}

func (s *Session) Close(ctx context.Context) error {
	s.mu.Lock()
	already := s.closed
	s.closed = true
	s.mu.Unlock()
	if already {
		return nil
	}
	if _, err := s.bus.Call(ctx, s.path, methodSessionClose); err != nil {
		return fmt.Errorf("portal: close session %s: %w", s.path, err)
	}
	return nil
}

type Event interface {
	isEvent()
}

type ClosedEvent struct{}

func (ClosedEvent) isEvent() {}

type eventSource struct {
	match  SignalMatch
	decode func(Signal) (Event, bool, error)
}

func closedSource(session dbus.ObjectPath) eventSource {
	return eventSource{
		match: SignalMatch{Path: session, Interface: interfaceSession, Member: signalSessionClose},
		decode: func(Signal) (Event, bool, error) {
			return ClosedEvent{}, true, nil
		},
	}
}

// EventStream merges the signals of one session into typed events. It owns one goroutine per source and
// ends when the context is cancelled, Close is called, a signal cannot be decoded or the bus goes away.
type EventStream struct {
	events chan Event
	cancel context.CancelFunc
	done   chan struct{}
	subs   []Subscription

	mu  sync.Mutex
	err error
}

func newEventStream(ctx context.Context, bus Bus, sources ...eventSource) (*EventStream, error) {
	ctx, cancel := context.WithCancel(ctx)
	s := &EventStream{
		events: make(chan Event, 32),
		cancel: cancel,
		done:   make(chan struct{}),
	}
	for _, src := range sources {
		sub, err := bus.Subscribe(ctx, src.match)
		if err != nil {
			cancel()
			return nil, errors.Join(fmt.Errorf("portal: subscribe %+v: %w", src.match, err), closeAll(s.subs))
		}
		s.subs = append(s.subs, sub)
	}
	var wg sync.WaitGroup
	for i, src := range sources {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.forward(ctx, s.subs[i], src.decode)
		}()
	}
	go func() {
		wg.Wait()
		close(s.events)
		close(s.done)
	}()
	return s, nil
}

func closeAll(subs []Subscription) error {
	var err error
	for _, sub := range subs {
		err = errors.Join(err, sub.Close())
	}
	return err
}

func (s *EventStream) forward(ctx context.Context, sub Subscription, decode func(Signal) (Event, bool, error)) {
	for {
		select {
		case <-ctx.Done():
			return
		case sig, open := <-sub.Signals():
			if !open {
				s.fail(ErrBusClosed)
				return
			}
			ev, ok, err := decode(sig)
			if err != nil {
				s.fail(fmt.Errorf("portal: decode %s: %w", sig.Name, err))
				return
			}
			if !ok {
				continue
			}
			select {
			case s.events <- ev:
			case <-ctx.Done():
				return
			}
		}
	}
}

func (s *EventStream) fail(err error) {
	s.mu.Lock()
	if s.err == nil {
		s.err = err
	}
	s.mu.Unlock()
	s.cancel()
}

func (s *EventStream) Events() <-chan Event {
	return s.events
}

// Err is valid after Events is closed: nil after Close or context cancellation, the decode or bus error otherwise.
func (s *EventStream) Err() error {
	<-s.done
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.err
}

func (s *EventStream) Close() error {
	s.cancel()
	<-s.done
	return closeAll(s.subs)
}

type SessionBus struct {
	conn *dbus.Conn
	in   chan *dbus.Signal
	done chan struct{}

	mu     sync.Mutex
	subs   map[*busSubscription]struct{}
	closed bool
}

type busSubscription struct {
	bus     *SessionBus
	match   SignalMatch
	options []dbus.MatchOption
	ch      chan Signal
	stop    chan struct{}
	once    sync.Once
}

func ConnectSessionBus() (*SessionBus, error) {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return nil, fmt.Errorf("portal: connect to the session bus: %w", err)
	}
	if !conn.SupportsUnixFDs() {
		return nil, errors.Join(ErrNoUnixFDSupport, conn.Close())
	}
	b := &SessionBus{
		conn: conn,
		in:   make(chan *dbus.Signal, 128),
		done: make(chan struct{}),
		subs: map[*busSubscription]struct{}{},
	}
	conn.Signal(b.in)
	go b.dispatch()
	return b, nil
}

func (b *SessionBus) Close() error {
	err := b.conn.Close()
	<-b.done
	return err
}

func (b *SessionBus) UniqueName() string {
	names := b.conn.Names()
	if len(names) == 0 {
		return ""
	}
	return names[0]
}

func (b *SessionBus) dispatch() {
	defer close(b.done)
	for raw := range b.in {
		sig := Signal{Path: raw.Path, Name: raw.Name, Body: raw.Body}
		for _, sub := range b.matching(sig) {
			select {
			case sub.ch <- sig:
			case <-sub.stop:
			}
		}
	}
	b.mu.Lock()
	b.closed = true
	remaining := b.subs
	b.subs = map[*busSubscription]struct{}{}
	b.mu.Unlock()
	for sub := range remaining {
		close(sub.ch)
	}
}

func (b *SessionBus) matching(sig Signal) []*busSubscription {
	b.mu.Lock()
	defer b.mu.Unlock()
	var out []*busSubscription
	for sub := range b.subs {
		if sub.match.Matches(sig) {
			out = append(out, sub)
		}
	}
	return out
}

func (b *SessionBus) Subscribe(ctx context.Context, match SignalMatch) (Subscription, error) {
	sub := &busSubscription{
		bus:   b,
		match: match,
		ch:    make(chan Signal, 32),
		stop:  make(chan struct{}),
	}
	if match.Path != "" {
		sub.options = append(sub.options, dbus.WithMatchObjectPath(match.Path))
	}
	if match.Interface != "" {
		sub.options = append(sub.options, dbus.WithMatchInterface(match.Interface))
	}
	if match.Member != "" {
		sub.options = append(sub.options, dbus.WithMatchMember(match.Member))
	}

	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return nil, ErrBusClosed
	}
	b.subs[sub] = struct{}{}
	b.mu.Unlock()

	if err := b.conn.AddMatchSignalContext(ctx, sub.options...); err != nil {
		b.mu.Lock()
		delete(b.subs, sub)
		b.mu.Unlock()
		return nil, fmt.Errorf("portal: AddMatch: %w", err)
	}
	return sub, nil
}

func (s *busSubscription) Signals() <-chan Signal {
	return s.ch
}

func (s *busSubscription) Close() error {
	var err error
	s.once.Do(func() {
		close(s.stop)
		s.bus.mu.Lock()
		delete(s.bus.subs, s)
		s.bus.mu.Unlock()
		if rmErr := s.bus.conn.RemoveMatchSignalContext(context.Background(), s.options...); rmErr != nil {
			err = fmt.Errorf("portal: RemoveMatch: %w", rmErr)
		}
	})
	return err
}

func (b *SessionBus) Call(ctx context.Context, path dbus.ObjectPath, method string, args ...any) ([]any, error) {
	call := b.conn.Object(Destination, path).CallWithContext(ctx, method, 0, args...)
	if call.Err != nil {
		return nil, fmt.Errorf("portal: %s: %w", method, call.Err)
	}
	return call.Body, nil
}

func (b *SessionBus) CallFD(ctx context.Context, path dbus.ObjectPath, method string, args ...any) (*os.File, error) {
	body, err := b.Call(ctx, path, method, args...)
	if err != nil {
		return nil, err
	}
	if len(body) != 1 {
		return nil, fmt.Errorf("%w: %s returned %d values, want one file descriptor", ErrUnexpectedReturn, method, len(body))
	}
	fd, ok := body[0].(dbus.UnixFD)
	if !ok {
		return nil, fmt.Errorf("%w: %s returned %T, want a file descriptor", ErrUnexpectedReturn, method, body[0])
	}
	return os.NewFile(uintptr(fd), "portal-eis"), nil
}

func (b *SessionBus) Property(ctx context.Context, path dbus.ObjectPath, iface, name string) (dbus.Variant, error) {
	var v dbus.Variant
	call := b.conn.Object(Destination, path).CallWithContext(ctx, methodGetProperty, 0, iface, name)
	if err := call.Store(&v); err != nil {
		return dbus.Variant{}, fmt.Errorf("portal: get property %s.%s: %w", iface, name, err)
	}
	return v, nil
}

func interfaceVersion(ctx context.Context, bus Bus, iface string) (uint32, error) {
	v, err := bus.Property(ctx, DesktopPath, iface, propertyVersion)
	if err != nil {
		return 0, err
	}
	version, ok := v.Value().(uint32)
	if !ok {
		return 0, fmt.Errorf("%w: %s version is %T, want uint32", ErrResultType, iface, v.Value())
	}
	return version, nil
}
