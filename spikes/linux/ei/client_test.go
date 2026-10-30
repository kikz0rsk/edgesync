package ei

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"testing"
	"time"
)

const (
	testConnectionID uint64 = FirstServerObjectID + 1
	testSeatID       uint64 = FirstServerObjectID + 2
	testDeviceID     uint64 = FirstServerObjectID + 3
	testPointerID    uint64 = FirstServerObjectID + 4
	testKeyboardID   uint64 = FirstServerObjectID + 5
	testPingID       uint64 = FirstServerObjectID + 6
	testTimeout             = 10 * time.Second
)

type scriptedServer struct {
	conn net.Conn
	rd   *bufio.Reader
}

func newScriptedServer(conn net.Conn) *scriptedServer {
	_ = conn.SetDeadline(time.Now().Add(testTimeout))
	return &scriptedServer{conn: conn, rd: bufio.NewReader(conn)}
}

func serverEvent(object uint64, opcode uint32, build func(*Encoder)) Frame {
	var e Encoder
	if build != nil {
		build(&e)
	}
	return e.Frame(object, opcode)
}

func (s *scriptedServer) send(f Frame) error {
	wire, err := f.AppendTo(nil)
	if err != nil {
		return err
	}
	_, err = s.conn.Write(wire)
	return err
}

func (s *scriptedServer) expect(want Frame) error {
	got, err := ReadFrame(s.rd)
	if err != nil {
		return fmt.Errorf("expected object %#x opcode %d: %w", want.ObjectID, want.Opcode, err)
	}
	if got.ObjectID != want.ObjectID || got.Opcode != want.Opcode || !bytes.Equal(got.Payload, want.Payload) {
		return fmt.Errorf("got object %#x opcode %d payload % x, want object %#x opcode %d payload % x",
			got.ObjectID, got.Opcode, got.Payload, want.ObjectID, want.Opcode, want.Payload)
	}
	return nil
}

func (s *scriptedServer) handshake(role Role, name string) error {
	if err := s.send(serverEvent(HandshakeObjectID, evHandshakeVersion, func(e *Encoder) { e.Uint32(1) })); err != nil {
		return err
	}
	expected := []Frame{handshakeVersionRequest(1), handshakeContextTypeRequest(role)}
	nameFrame, err := handshakeNameRequest(name)
	if err != nil {
		return err
	}
	expected = append(expected, nameFrame)
	for _, iface := range negotiableInterfaces() {
		f, err := handshakeInterfaceVersionRequest(iface.String(), iface.MaxVersion())
		if err != nil {
			return err
		}
		expected = append(expected, f)
	}
	expected = append(expected, handshakeFinishRequest())
	for _, want := range expected {
		if err := s.expect(want); err != nil {
			return fmt.Errorf("handshake request: %w", err)
		}
	}

	for _, iv := range []struct {
		name    string
		version uint32
	}{{"ei_connection", 1}, {"ei_seat", 2}, {"ei_device", 3}, {"ei_touchscreen", 2}} {
		if err := s.send(serverEvent(HandshakeObjectID, evHandshakeInterfaceVersion, func(e *Encoder) {
			_ = e.String(iv.name)
			e.Uint32(iv.version)
		})); err != nil {
			return err
		}
	}
	return s.send(serverEvent(HandshakeObjectID, evHandshakeConnection, func(e *Encoder) {
		e.Uint32(7)
		e.NewID(testConnectionID)
		e.Uint32(1)
	}))
}

func newPipeClient(t *testing.T, role Role, script func(*scriptedServer) error) (*Client, <-chan error) {
	t.Helper()
	clientConn, serverConn := net.Pipe()
	errCh := make(chan error, 1)
	go func() {
		srv := newScriptedServer(serverConn)
		err := script(srv)
		errCh <- errors.Join(err, serverConn.Close())
	}()
	ctx, cancel := context.WithTimeout(t.Context(), testTimeout)
	defer cancel()
	c, err := Connect(ctx, NewStreamTransport(clientConn), Config{Name: "edgesync", Role: role})
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(func() {
		if err := c.Close(); err != nil && !errors.Is(err, ErrDisconnected) {
			t.Errorf("Close: %v", err)
		}
	})
	return c, errCh
}

func nextEvent(t *testing.T, c *Client) Inbound {
	t.Helper()
	select {
	case in, ok := <-c.Events():
		if !ok {
			t.Fatalf("event channel closed: %v", c.Err())
		}
		return in
	case <-time.After(testTimeout):
		t.Fatal("timed out waiting for an event")
		return Inbound{}
	}
}

func expectEvent[T Event](t *testing.T, c *Client, object uint64, want T) {
	t.Helper()
	in := nextEvent(t, c)
	got, ok := in.Event.(T)
	if !ok || in.Object != object || any(got) != any(want) {
		t.Fatalf("got object %#x %T %+v, want object %#x %T %+v", in.Object, in.Event, in.Event, object, want, want)
	}
}

func waitServer(t *testing.T, errCh <-chan error) {
	t.Helper()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("scripted server: %v", err)
		}
	case <-time.After(testTimeout):
		t.Fatal("timed out waiting for the scripted server")
	}
}

func TestClient_HandshakeAgainstScriptedServer(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), testTimeout)
	defer cancel()

	script := func(s *scriptedServer) error {
		if err := s.handshake(RoleSender, "edgesync"); err != nil {
			return err
		}
		steps := []Frame{
			serverEvent(testConnectionID, evConnectionSeat, func(e *Encoder) { e.NewID(testSeatID); e.Uint32(2) }),
			serverEvent(testSeatID, evSeatName, func(e *Encoder) { _ = e.String("seat0") }),
			serverEvent(testSeatID, evSeatCapability, func(e *Encoder) { e.Uint64(1); _ = e.String("ei_pointer") }),
			serverEvent(testSeatID, evSeatCapability, func(e *Encoder) { e.Uint64(4); _ = e.String("ei_keyboard") }),
			serverEvent(testSeatID, evSeatDone, nil),
		}
		for _, f := range steps {
			if err := s.send(f); err != nil {
				return err
			}
		}
		if err := s.expect(seatBindRequest(testSeatID, 5)); err != nil {
			return err
		}
		device := []Frame{
			serverEvent(testSeatID, evSeatDevice, func(e *Encoder) { e.NewID(testDeviceID); e.Uint32(3) }),
			serverEvent(testDeviceID, evDeviceName, func(e *Encoder) { _ = e.String("edgesync virtual") }),
			serverEvent(testDeviceID, evDeviceDeviceType, func(e *Encoder) { e.Uint32(uint32(DeviceTypeVirtual)) }),
			serverEvent(testDeviceID, evDeviceRegion, func(e *Encoder) {
				e.Uint32(0)
				e.Uint32(0)
				e.Uint32(1920)
				e.Uint32(1080)
				e.Float32(1)
			}),
			serverEvent(testDeviceID, evDeviceInterface, func(e *Encoder) {
				e.NewID(testPointerID)
				_ = e.String("ei_pointer")
				e.Uint32(1)
			}),
			serverEvent(testDeviceID, evDeviceInterface, func(e *Encoder) {
				e.NewID(testKeyboardID)
				_ = e.String("ei_keyboard")
				e.Uint32(1)
			}),
			serverEvent(testDeviceID, evDeviceDone, nil),
		}
		for _, f := range device {
			if err := s.send(f); err != nil {
				return err
			}
		}
		if err := s.expect(deviceReadyRequest(testDeviceID)); err != nil {
			return err
		}
		if err := s.send(serverEvent(testDeviceID, evDeviceResumed, func(e *Encoder) { e.Uint32(11) })); err != nil {
			return err
		}
		for _, want := range []Frame{
			deviceStartEmulatingRequest(testDeviceID, 11, 1),
			pointerMotionRelativeRequest(testPointerID, 4, -3.5),
			keyRequest(testKeyboardID, 30, KeyPressed),
			deviceFrameRequest(testDeviceID, 11, 1234567),
			deviceStopEmulatingRequest(testDeviceID, 11),
			connectionSyncRequest(testConnectionID, FirstClientObjectID, 1),
		} {
			if err := s.expect(want); err != nil {
				return err
			}
		}
		if err := s.send(serverEvent(FirstClientObjectID, evCallbackDone, func(e *Encoder) { e.Uint64(0) })); err != nil {
			return err
		}
		if err := s.send(serverEvent(testConnectionID, evConnectionPing, func(e *Encoder) { e.NewID(testPingID); e.Uint32(1) })); err != nil {
			return err
		}
		if err := s.expect(pingpongDoneRequest(testPingID, 0)); err != nil {
			return err
		}
		if err := s.send(serverEvent(testConnectionID, evConnectionInvalidObject, func(e *Encoder) {
			e.Uint32(12)
			e.Uint64(99)
		})); err != nil {
			return err
		}
		return s.expect(connectionDisconnectRequest(testConnectionID))
	}

	c, errCh := newPipeClient(t, RoleSender, script)

	hs := c.Handshake()
	if hs.Serial != 7 || hs.Connection != testConnectionID || hs.ConnectionVersion != 1 || hs.EISInterfaceVersions["ei_touchscreen"] != 2 {
		t.Fatalf("unexpected handshake result %+v", hs)
	}

	expectEvent(t, c, testConnectionID, ConnectionSeatEvent{Seat: testSeatID, Version: 2})
	expectEvent(t, c, testSeatID, SeatNameEvent{Name: "seat0"})
	expectEvent(t, c, testSeatID, SeatCapabilityEvent{Mask: 1, Interface: "ei_pointer"})
	expectEvent(t, c, testSeatID, SeatCapabilityEvent{Mask: 4, Interface: "ei_keyboard"})
	expectEvent(t, c, testSeatID, SeatDoneEvent{})
	if err := c.Bind(testSeatID, 5); err != nil {
		t.Fatalf("Bind: %v", err)
	}

	expectEvent(t, c, testSeatID, SeatDeviceEvent{Device: testDeviceID, Version: 3})
	expectEvent(t, c, testDeviceID, DeviceNameEvent{Name: "edgesync virtual"})
	expectEvent(t, c, testDeviceID, DeviceTypeEvent{Type: DeviceTypeVirtual})
	expectEvent(t, c, testDeviceID, DeviceRegionEvent{Width: 1920, Height: 1080, Scale: 1})
	expectEvent(t, c, testDeviceID, DeviceInterfaceEvent{Object: testPointerID, Name: "ei_pointer", Version: 1})
	expectEvent(t, c, testDeviceID, DeviceInterfaceEvent{Object: testKeyboardID, Name: "ei_keyboard", Version: 1})
	expectEvent(t, c, testDeviceID, DeviceDoneEvent{})
	expectEvent(t, c, testDeviceID, DeviceResumedEvent{Serial: 11})

	if err := c.StartEmulating(testDeviceID); err != nil {
		t.Fatalf("StartEmulating: %v", err)
	}
	if err := c.MotionRelative(testPointerID, 4, -3.5); err != nil {
		t.Fatalf("MotionRelative: %v", err)
	}
	if err := c.Key(testKeyboardID, 30, KeyPressed); err != nil {
		t.Fatalf("Key: %v", err)
	}
	if err := c.SendFrame(testDeviceID, 1234567); err != nil {
		t.Fatalf("SendFrame: %v", err)
	}
	if err := c.StopEmulating(testDeviceID); err != nil {
		t.Fatalf("StopEmulating: %v", err)
	}
	if err := c.Sync(ctx); err != nil {
		t.Fatalf("Sync: %v", err)
	}

	expectEvent(t, c, testConnectionID, ConnectionInvalidObjectEvent{LastSerial: 12, InvalidID: 99})

	if err := c.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := c.Err(); err != nil {
		t.Fatalf("Err after Close: %v", err)
	}
	waitServer(t, errCh)
}

func TestClient_SenderRejectsReceiverOnlyEvents(t *testing.T) {
	script := func(s *scriptedServer) error {
		if err := s.handshake(RoleSender, "edgesync"); err != nil {
			return err
		}
		if err := s.send(serverEvent(testConnectionID, evConnectionSeat, func(e *Encoder) { e.NewID(testSeatID); e.Uint32(1) })); err != nil {
			return err
		}
		if err := s.send(serverEvent(testSeatID, evSeatDevice, func(e *Encoder) { e.NewID(testDeviceID); e.Uint32(1) })); err != nil {
			return err
		}
		return s.send(serverEvent(testDeviceID, evDeviceStartEmulating, func(e *Encoder) { e.Uint32(1); e.Uint32(1) }))
	}
	c, errCh := newPipeClient(t, RoleSender, script)
	for range c.Events() {
	}
	if err := c.Err(); !errors.Is(err, ErrProtocol) {
		t.Fatalf("got %v, want ErrProtocol", err)
	}
	waitServer(t, errCh)
}

func TestClient_ReceiverDeliversInputAndRejectsRequests(t *testing.T) {
	script := func(s *scriptedServer) error {
		if err := s.handshake(RoleReceiver, "edgesync"); err != nil {
			return err
		}
		for _, f := range []Frame{
			serverEvent(testConnectionID, evConnectionSeat, func(e *Encoder) { e.NewID(testSeatID); e.Uint32(1) }),
			serverEvent(testSeatID, evSeatDevice, func(e *Encoder) { e.NewID(testDeviceID); e.Uint32(2) }),
			serverEvent(testDeviceID, evDeviceInterface, func(e *Encoder) {
				e.NewID(testPointerID)
				_ = e.String("ei_pointer")
				e.Uint32(1)
			}),
			serverEvent(testDeviceID, evDeviceDone, nil),
			serverEvent(testPointerID, evPointerMotionRelative, func(e *Encoder) { e.Float32(2); e.Float32(-1) }),
			serverEvent(testDeviceID, evDeviceFrame, func(e *Encoder) { e.Uint32(5); e.Uint64(77) }),
		} {
			if err := s.send(f); err != nil {
				return err
			}
		}
		return s.expect(connectionDisconnectRequest(testConnectionID))
	}
	c, errCh := newPipeClient(t, RoleReceiver, script)

	expectEvent(t, c, testConnectionID, ConnectionSeatEvent{Seat: testSeatID, Version: 1})
	expectEvent(t, c, testSeatID, SeatDeviceEvent{Device: testDeviceID, Version: 2})
	expectEvent(t, c, testDeviceID, DeviceInterfaceEvent{Object: testPointerID, Name: "ei_pointer", Version: 1})
	expectEvent(t, c, testDeviceID, DeviceDoneEvent{})
	expectEvent(t, c, testPointerID, PointerMotionRelativeEvent{X: 2, Y: -1})
	expectEvent(t, c, testDeviceID, DeviceFrameEvent{Serial: 5, Timestamp: 77})

	if err := c.MotionRelative(testPointerID, 1, 1); !errors.Is(err, ErrWrongRole) {
		t.Fatalf("MotionRelative on a receiver: got %v, want ErrWrongRole", err)
	}
	if err := c.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	waitServer(t, errCh)
}

func TestClient_SenderRequestsValidateObjects(t *testing.T) {
	script := func(s *scriptedServer) error {
		if err := s.handshake(RoleSender, "edgesync"); err != nil {
			return err
		}
		return s.expect(connectionDisconnectRequest(testConnectionID))
	}
	c, errCh := newPipeClient(t, RoleSender, script)

	if err := c.MotionRelative(12345, 1, 1); !errors.Is(err, ErrUnknownObject) {
		t.Fatalf("unknown object: got %v", err)
	}
	if err := c.MotionRelative(testConnectionID, 1, 1); !errors.Is(err, ErrWrongObject) {
		t.Fatalf("wrong interface: got %v", err)
	}
	if err := c.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	waitServer(t, errCh)
}

func TestClient_DisconnectedEventEndsTheClient(t *testing.T) {
	script := func(s *scriptedServer) error {
		if err := s.handshake(RoleSender, "edgesync"); err != nil {
			return err
		}
		return s.send(serverEvent(testConnectionID, evConnectionDisconnected, func(e *Encoder) {
			e.Uint32(3)
			e.Uint32(uint32(DisconnectReasonValue))
			_ = e.String("bad value")
		}))
	}
	c, errCh := newPipeClient(t, RoleSender, script)

	expectEvent(t, c, testConnectionID, ConnectionDisconnectedEvent{
		LastSerial: 3, Reason: DisconnectReasonValue, Explanation: "bad value", HasExplanation: true,
	})
	err := c.Err()
	var de *DisconnectError
	if !errors.As(err, &de) || !errors.Is(err, ErrDisconnected) || de.Reason != DisconnectReasonValue {
		t.Fatalf("got %v, want a DisconnectError", err)
	}
	waitServer(t, errCh)
}

func TestClient_PeerCloseIsReported(t *testing.T) {
	script := func(s *scriptedServer) error {
		return s.handshake(RoleSender, "edgesync")
	}
	c, errCh := newPipeClient(t, RoleSender, script)
	waitServer(t, errCh)
	if err := c.Err(); !errors.Is(err, ErrPeerClosed) {
		t.Fatalf("got %v, want ErrPeerClosed", err)
	}
}

func TestClient_HandshakeRejectsViolations(t *testing.T) {
	cases := []struct {
		name   string
		script func(*scriptedServer) error
	}{
		{
			"first message is not handshake_version",
			func(s *scriptedServer) error {
				return s.send(serverEvent(HandshakeObjectID, evHandshakeConnection, func(e *Encoder) {
					e.Uint32(1)
					e.NewID(testConnectionID)
					e.Uint32(1)
				}))
			},
		},
		{
			"connection id below the EIS range",
			func(s *scriptedServer) error {
				if err := s.send(serverEvent(HandshakeObjectID, evHandshakeVersion, func(e *Encoder) { e.Uint32(1) })); err != nil {
					return err
				}
				if err := drainHandshakeRequests(s); err != nil {
					return err
				}
				return s.send(serverEvent(HandshakeObjectID, evHandshakeConnection, func(e *Encoder) {
					e.Uint32(1)
					e.NewID(5)
					e.Uint32(1)
				}))
			},
		},
		{
			"connection version above the client's",
			func(s *scriptedServer) error {
				if err := s.send(serverEvent(HandshakeObjectID, evHandshakeVersion, func(e *Encoder) { e.Uint32(1) })); err != nil {
					return err
				}
				if err := drainHandshakeRequests(s); err != nil {
					return err
				}
				return s.send(serverEvent(HandshakeObjectID, evHandshakeConnection, func(e *Encoder) {
					e.Uint32(1)
					e.NewID(testConnectionID)
					e.Uint32(2)
				}))
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			clientConn, serverConn := net.Pipe()
			errCh := make(chan error, 1)
			go func() {
				err := tc.script(newScriptedServer(serverConn))
				errCh <- errors.Join(err, serverConn.Close())
			}()
			ctx, cancel := context.WithTimeout(t.Context(), testTimeout)
			defer cancel()
			if _, err := Connect(ctx, NewStreamTransport(clientConn), Config{Name: "edgesync", Role: RoleSender}); !errors.Is(err, ErrProtocol) {
				t.Fatalf("Connect: got %v, want ErrProtocol", err)
			}
			waitServer(t, errCh)
		})
	}
}

func TestClient_ConnectHonoursContextCancellation(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	defer serverConn.Close()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := Connect(ctx, NewStreamTransport(clientConn), Config{Role: RoleSender}); !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v, want context.Canceled", err)
	}
}

func TestClient_RejectsInvalidConfig(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()
	if _, err := Connect(t.Context(), NewStreamTransport(clientConn), Config{Name: "x"}); !errors.Is(err, ErrConfig) {
		t.Fatalf("got %v, want ErrConfig", err)
	}
}

func drainHandshakeRequests(s *scriptedServer) error {
	for {
		f, err := ReadFrame(s.rd)
		if err != nil {
			return err
		}
		if f.ObjectID == HandshakeObjectID && f.Opcode == opHandshakeFinish {
			return nil
		}
	}
}
