package ei

import (
	"bytes"
	"errors"
	"io"
	"math"
	"os"
	"testing"
)

func TestWire_HeaderEncodeDecode(t *testing.T) {
	cases := []struct {
		name string
		head Header
		want []byte
	}{
		{
			name: "handshake object",
			head: Header{ObjectID: 0, Length: 20, Opcode: 0},
			want: []byte{0, 0, 0, 0, 0, 0, 0, 0, 0x14, 0, 0, 0, 0, 0, 0, 0},
		},
		{
			name: "server allocated id",
			head: Header{ObjectID: 0xff00000000000002, Length: 32, Opcode: 7},
			want: []byte{0x02, 0, 0, 0, 0, 0, 0, 0xff, 0x20, 0, 0, 0, 0x07, 0, 0, 0},
		},
		{
			name: "header only",
			head: Header{ObjectID: 1, Length: HeaderSize, Opcode: 0xdeadbeef},
			want: []byte{0x01, 0, 0, 0, 0, 0, 0, 0, 0x10, 0, 0, 0, 0xef, 0xbe, 0xad, 0xde},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.head.Append(nil)
			if !bytes.Equal(got, tc.want) {
				t.Fatalf("encode: got % x, want % x", got, tc.want)
			}
			back, err := DecodeHeader(got)
			if err != nil {
				t.Fatalf("decode: %v", err)
			}
			if back != tc.head {
				t.Fatalf("decode: got %+v, want %+v", back, tc.head)
			}
		})
	}
}

func TestWire_FrameRoundTrip(t *testing.T) {
	var e Encoder
	e.Uint32(0xa1b2c3d4)
	e.Int32(-2)
	e.Uint64(0x0102030405060708)
	e.Int64(-3)
	e.Float32(1.5)
	if err := e.String("hello"); err != nil {
		t.Fatal(err)
	}
	e.NullString()
	e.NewID(0xff00000000000009)
	e.ObjectID(0xff0000000000000a)
	frame := e.Frame(42, 3)

	wire, err := frame.AppendTo(nil)
	if err != nil {
		t.Fatal(err)
	}
	got, n, err := DecodeFrame(wire)
	if err != nil {
		t.Fatal(err)
	}
	if n != len(wire) || got.ObjectID != 42 || got.Opcode != 3 {
		t.Fatalf("decoded header mismatch: n=%d frame=%+v", n, got)
	}

	d := NewDecoder(got.Payload, nil)
	if v := d.Uint32(); v != 0xa1b2c3d4 {
		t.Errorf("uint32: %#x", v)
	}
	if v := d.Int32(); v != -2 {
		t.Errorf("int32: %d", v)
	}
	if v := d.Uint64(); v != 0x0102030405060708 {
		t.Errorf("uint64: %#x", v)
	}
	if v := d.Int64(); v != -3 {
		t.Errorf("int64: %d", v)
	}
	if v := d.Float32(); v != 1.5 {
		t.Errorf("float32: %v", v)
	}
	if v := d.String(); v != "hello" {
		t.Errorf("string: %q", v)
	}
	if v, ok := d.NullableString(); ok || v != "" {
		t.Errorf("null string: %q %v", v, ok)
	}
	if v := d.NewID(); v != 0xff00000000000009 {
		t.Errorf("new_id: %#x", v)
	}
	if v := d.ObjectID(); v != 0xff0000000000000a {
		t.Errorf("object: %#x", v)
	}
	if err := d.Finish(); err != nil {
		t.Fatalf("finish: %v", err)
	}
}

func TestWire_StringEncoding(t *testing.T) {
	cases := []struct {
		name string
		put  func(*Encoder) error
		want []byte
	}{
		{"null", func(e *Encoder) error { e.NullString(); return nil }, []byte{0, 0, 0, 0}},
		{"empty", func(e *Encoder) error { return e.String("") }, []byte{1, 0, 0, 0, 0, 0, 0, 0}},
		{"hello", func(e *Encoder) error { return e.String("hello") }, []byte{6, 0, 0, 0, 'h', 'e', 'l', 'l', 'o', 0, 0, 0}},
		{"exact multiple", func(e *Encoder) error { return e.String("abc") }, []byte{4, 0, 0, 0, 'a', 'b', 'c', 0}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var e Encoder
			if err := tc.put(&e); err != nil {
				t.Fatal(err)
			}
			if got := e.Frame(0, 0).Payload; !bytes.Equal(got, tc.want) {
				t.Fatalf("got % x, want % x", got, tc.want)
			}
		})
	}

	var e Encoder
	if err := e.String("a\x00b"); !errors.Is(err, ErrInvalidString) {
		t.Fatalf("embedded NUL: got %v, want ErrInvalidString", err)
	}
}

func TestWire_DecoderRejectsBadStrings(t *testing.T) {
	cases := []struct {
		name    string
		payload []byte
		want    error
	}{
		{"missing terminator", []byte{2, 0, 0, 0, 'a', 'b', 0, 0}, ErrInvalidString},
		{"embedded NUL", []byte{4, 0, 0, 0, 'a', 0, 'b', 0}, ErrInvalidString},
		{"length beyond payload", []byte{9, 0, 0, 0, 'a', 0, 0, 0}, ErrTruncatedArgument},
		{"huge length", []byte{0xff, 0xff, 0xff, 0xff, 0, 0, 0, 0}, ErrTruncatedArgument},
		{"null where string required", []byte{0, 0, 0, 0}, ErrInvalidString},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := NewDecoder(tc.payload, nil)
			_ = d.String()
			if err := d.Err(); !errors.Is(err, tc.want) {
				t.Fatalf("got %v, want %v", err, tc.want)
			}
		})
	}
}

func TestWire_DecoderTruncationAndTrailing(t *testing.T) {
	d := NewDecoder([]byte{1, 0, 0}, nil)
	d.Uint32()
	if err := d.Finish(); !errors.Is(err, ErrTruncatedArgument) {
		t.Fatalf("truncated: got %v", err)
	}
	d = NewDecoder([]byte{1, 0, 0, 0, 2, 0, 0, 0}, nil)
	d.Uint32()
	if err := d.Finish(); !errors.Is(err, ErrTrailingBytes) {
		t.Fatalf("trailing: got %v", err)
	}
	d = NewDecoder(nil, nil)
	if d.FD(); !errors.Is(d.Err(), ErrNoFD) {
		t.Fatalf("fd without source: got %v", d.Err())
	}
}

func TestWire_FloatBitPatterns(t *testing.T) {
	var e Encoder
	e.Float32(float32(math.Inf(-1)))
	e.Float32(float32(math.Copysign(0, -1)))
	got := e.Frame(0, 0).Payload
	want := []byte{0x00, 0x00, 0x80, 0xff, 0x00, 0x00, 0x00, 0x80}
	if !bytes.Equal(got, want) {
		t.Fatalf("got % x, want % x", got, want)
	}
}

func TestWire_GoldenHandshakeFrames(t *testing.T) {
	zeroID := []byte{0, 0, 0, 0, 0, 0, 0, 0}
	join := func(parts ...[]byte) []byte { return bytes.Join(parts, nil) }
	name, err := handshakeNameRequest("edgesync")
	if err != nil {
		t.Fatal(err)
	}
	ifv, err := handshakeInterfaceVersionRequest("ei_connection", 1)
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name  string
		frame Frame
		want  []byte
	}{
		{
			"handshake_version(1)",
			handshakeVersionRequest(1),
			join(zeroID, []byte{0x14, 0, 0, 0, 0, 0, 0, 0, 0x01, 0, 0, 0}),
		},
		{
			"context_type(sender)",
			handshakeContextTypeRequest(RoleSender),
			join(zeroID, []byte{0x14, 0, 0, 0, 0x02, 0, 0, 0, 0x02, 0, 0, 0}),
		},
		{
			"context_type(receiver)",
			handshakeContextTypeRequest(RoleReceiver),
			join(zeroID, []byte{0x14, 0, 0, 0, 0x02, 0, 0, 0, 0x01, 0, 0, 0}),
		},
		{
			"name(edgesync)",
			name,
			join(zeroID, []byte{0x20, 0, 0, 0, 0x03, 0, 0, 0, 0x09, 0, 0, 0},
				[]byte{'e', 'd', 'g', 'e', 's', 'y', 'n', 'c', 0, 0, 0, 0}),
		},
		{
			"interface_version(ei_connection,1)",
			ifv,
			join(zeroID, []byte{0x28, 0, 0, 0, 0x04, 0, 0, 0, 0x0e, 0, 0, 0},
				[]byte{'e', 'i', '_', 'c', 'o', 'n', 'n', 'e', 'c', 't', 'i', 'o', 'n', 0, 0, 0},
				[]byte{0x01, 0, 0, 0}),
		},
		{
			"finish",
			handshakeFinishRequest(),
			join(zeroID, []byte{0x10, 0, 0, 0, 0x01, 0, 0, 0}),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.frame.AppendTo(nil)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, tc.want) {
				t.Fatalf("got % x\nwant % x", got, tc.want)
			}
		})
	}

	versionEvent := join(zeroID, []byte{0x14, 0, 0, 0, 0, 0, 0, 0, 0x01, 0, 0, 0})
	f, _, err := DecodeFrame(versionEvent)
	if err != nil {
		t.Fatal(err)
	}
	ev, err := DecodeEvent(InterfaceHandshake, f, nil)
	if err != nil {
		t.Fatal(err)
	}
	if ev != (HandshakeVersionEvent{Version: 1}) {
		t.Fatalf("handshake_version event: %+v", ev)
	}

	connEvent := join(zeroID, []byte{0x20, 0, 0, 0, 0x02, 0, 0, 0, 0x07, 0, 0, 0},
		[]byte{0x01, 0, 0, 0, 0, 0, 0, 0xff}, []byte{0x01, 0, 0, 0})
	f, _, err = DecodeFrame(connEvent)
	if err != nil {
		t.Fatal(err)
	}
	ev, err = DecodeEvent(InterfaceHandshake, f, nil)
	if err != nil {
		t.Fatal(err)
	}
	if ev != (HandshakeConnectionEvent{Serial: 7, Connection: 0xff00000000000001, Version: 1}) {
		t.Fatalf("connection event: %+v", ev)
	}
}

func TestWire_GoldenInputRequests(t *testing.T) {
	obj := func(id byte) []byte { return []byte{id, 0, 0, 0, 0, 0, 0, 0} }
	join := func(parts ...[]byte) []byte { return bytes.Join(parts, nil) }
	cases := []struct {
		name  string
		frame Frame
		want  []byte
	}{
		{
			"pointer.motion_relative(1.5,-2.0)",
			pointerMotionRelativeRequest(5, 1.5, -2.0),
			join(obj(5), []byte{0x18, 0, 0, 0, 0x01, 0, 0, 0, 0x00, 0x00, 0xc0, 0x3f, 0x00, 0x00, 0x00, 0xc0}),
		},
		{
			"button.button(BTN_LEFT,pressed)",
			buttonRequest(6, 0x110, ButtonPressed),
			join(obj(6), []byte{0x18, 0, 0, 0, 0x01, 0, 0, 0, 0x10, 0x01, 0, 0, 0x01, 0, 0, 0}),
		},
		{
			"keyboard.key(KEY_A,released)",
			keyRequest(7, 30, KeyReleased),
			join(obj(7), []byte{0x18, 0, 0, 0, 0x01, 0, 0, 0, 0x1e, 0, 0, 0, 0x00, 0, 0, 0}),
		},
		{
			"scroll.scroll_discrete(0,-120)",
			scrollDiscreteRequest(8, 0, -120),
			join(obj(8), []byte{0x18, 0, 0, 0, 0x02, 0, 0, 0, 0, 0, 0, 0, 0x88, 0xff, 0xff, 0xff}),
		},
		{
			"scroll.scroll_stop(1,0,0)",
			scrollStopRequest(8, 1, 0, 0),
			join(obj(8), []byte{0x1c, 0, 0, 0, 0x03, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}),
		},
		{
			"device.frame(serial 9, ts 0x1_0000_0002)",
			deviceFrameRequest(9, 9, 0x100000002),
			join(obj(9), []byte{0x1c, 0, 0, 0, 0x03, 0, 0, 0, 0x09, 0, 0, 0, 0x02, 0, 0, 0, 0x01, 0, 0, 0}),
		},
		{
			"device.start_emulating(serial 4, seq 1)",
			deviceStartEmulatingRequest(9, 4, 1),
			join(obj(9), []byte{0x18, 0, 0, 0, 0x01, 0, 0, 0, 0x04, 0, 0, 0, 0x01, 0, 0, 0}),
		},
		{
			"seat.bind(0x5)",
			seatBindRequest(3, 5),
			join(obj(3), []byte{0x18, 0, 0, 0, 0x01, 0, 0, 0, 0x05, 0, 0, 0, 0, 0, 0, 0}),
		},
		{
			"connection.sync(callback 1, version 1)",
			connectionSyncRequest(0xff00000000000001, 1, 1),
			join([]byte{0x01, 0, 0, 0, 0, 0, 0, 0xff}, []byte{0x1c, 0, 0, 0, 0x00, 0, 0, 0},
				[]byte{0x01, 0, 0, 0, 0, 0, 0, 0}, []byte{0x01, 0, 0, 0}),
		},
		{
			"device.ready",
			deviceReadyRequest(9),
			join(obj(9), []byte{0x10, 0, 0, 0, 0x04, 0, 0, 0}),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.frame.AppendTo(nil)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, tc.want) {
				t.Fatalf("got % x\nwant % x", got, tc.want)
			}
		})
	}
}

func TestWire_GoldenEventDecoding(t *testing.T) {
	obj := func(id byte) []byte { return []byte{id, 0, 0, 0, 0, 0, 0, 0xff} }
	join := func(parts ...[]byte) []byte { return bytes.Join(parts, nil) }
	cases := []struct {
		name  string
		iface Interface
		wire  []byte
		want  Event
	}{
		{
			"seat.capability(0x4, ei_keyboard)",
			InterfaceSeat,
			join(obj(2), []byte{0x28, 0, 0, 0, 0x02, 0, 0, 0}, []byte{4, 0, 0, 0, 0, 0, 0, 0},
				[]byte{0x0c, 0, 0, 0, 'e', 'i', '_', 'k', 'e', 'y', 'b', 'o', 'a', 'r', 'd', 0}),
			SeatCapabilityEvent{Mask: 4, Interface: "ei_keyboard"},
		},
		{
			"device.region(10,20,1920,1080,1.0)",
			InterfaceDevice,
			join(obj(3), []byte{0x24, 0, 0, 0, 0x04, 0, 0, 0},
				[]byte{10, 0, 0, 0, 20, 0, 0, 0, 0x80, 0x07, 0, 0, 0x38, 0x04, 0, 0, 0x00, 0x00, 0x80, 0x3f}),
			DeviceRegionEvent{OffsetX: 10, OffsetY: 20, Width: 1920, Height: 1080, Scale: 1},
		},
		{
			"device.interface(ei_pointer v1)",
			InterfaceDevice,
			join(obj(3), []byte{0x2c, 0, 0, 0, 0x05, 0, 0, 0}, obj(4),
				[]byte{0x0b, 0, 0, 0, 'e', 'i', '_', 'p', 'o', 'i', 'n', 't', 'e', 'r', 0, 0}, []byte{1, 0, 0, 0}),
			DeviceInterfaceEvent{Object: 0xff00000000000004, Name: "ei_pointer", Version: 1},
		},
		{
			"connection.disconnected(null explanation)",
			InterfaceConnection,
			join(obj(1), []byte{0x1c, 0, 0, 0, 0x00, 0, 0, 0}, []byte{9, 0, 0, 0, 3, 0, 0, 0, 0, 0, 0, 0}),
			ConnectionDisconnectedEvent{LastSerial: 9, Reason: DisconnectReasonProtocol},
		},
		{
			"pointer.motion_relative(3,4)",
			InterfacePointer,
			join(obj(5), []byte{0x18, 0, 0, 0, 0x01, 0, 0, 0, 0x00, 0x00, 0x40, 0x40, 0x00, 0x00, 0x80, 0x40}),
			PointerMotionRelativeEvent{X: 3, Y: 4},
		},
		{
			"keyboard.modifiers",
			InterfaceKeyboard,
			join(obj(6), []byte{0x24, 0, 0, 0, 0x03, 0, 0, 0}, []byte{1, 0, 0, 0, 2, 0, 0, 0, 3, 0, 0, 0, 4, 0, 0, 0, 5, 0, 0, 0}),
			KeyboardModifiersEvent{Serial: 1, Depressed: 2, Locked: 3, Latched: 4, Group: 5},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f, n, err := DecodeFrame(tc.wire)
			if err != nil {
				t.Fatal(err)
			}
			if n != len(tc.wire) {
				t.Fatalf("consumed %d of %d bytes", n, len(tc.wire))
			}
			ev, err := DecodeEvent(tc.iface, f, nil)
			if err != nil {
				t.Fatal(err)
			}
			if ev != tc.want {
				t.Fatalf("got %+v, want %+v", ev, tc.want)
			}
		})
	}
}

func TestWire_KeymapFDComesFromSource(t *testing.T) {
	var e Encoder
	e.Uint32(uint32(KeymapTypeXKB))
	e.Uint32(4096)
	frame := e.Frame(0xff00000000000006, evKeyboardKeymap)

	if _, err := DecodeEvent(InterfaceKeyboard, frame, nil); !errors.Is(err, ErrNoFD) {
		t.Fatalf("without a source: got %v, want ErrNoFD", err)
	}
	file, err := os.CreateTemp(t.TempDir(), "keymap")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	ev, err := DecodeEvent(InterfaceKeyboard, frame, fixedFD{file: file})
	if err != nil {
		t.Fatal(err)
	}
	km, ok := ev.(KeyboardKeymapEvent)
	if !ok || km.Type != KeymapTypeXKB || km.Size != 4096 || km.FD != file {
		t.Fatalf("unexpected keymap event %+v", ev)
	}
}

type fixedFD struct{ file *os.File }

func (f fixedFD) PopFD() (*os.File, error) { return f.file, nil }

func TestWire_RejectsShortAndOversized(t *testing.T) {
	valid, err := handshakeVersionRequest(1).AppendTo(nil)
	if err != nil {
		t.Fatal(err)
	}
	withLength := func(length uint32) []byte {
		return Header{ObjectID: 1, Length: length, Opcode: 0}.Append(make([]byte, 0, HeaderSize))
	}

	cases := []struct {
		name string
		in   []byte
		want error
	}{
		{"empty", nil, ErrShortFrame},
		{"partial header", valid[:HeaderSize-1], ErrShortFrame},
		{"missing payload", valid[:HeaderSize+2], ErrShortFrame},
		{"length below header size", withLength(HeaderSize - 4), ErrInvalidFrameLength},
		{"length zero", withLength(0), ErrInvalidFrameLength},
		{"length not multiple of four", withLength(HeaderSize + 2), ErrInvalidFrameLength},
		{"length above limit", withLength(MaxFrameSize + 4), ErrOversizedFrame},
		{"length max uint32", withLength(math.MaxUint32), ErrOversizedFrame},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, _, err := DecodeFrame(tc.in); !errors.Is(err, tc.want) {
				t.Fatalf("DecodeFrame: got %v, want %v", err, tc.want)
			}
		})
	}

	t.Run("ReadFrame truncated payload", func(t *testing.T) {
		_, err := ReadFrame(bytes.NewReader(valid[:HeaderSize+2]))
		if !errors.Is(err, io.ErrUnexpectedEOF) {
			t.Fatalf("got %v, want io.ErrUnexpectedEOF", err)
		}
	})
	t.Run("ReadFrame empty stream", func(t *testing.T) {
		if _, err := ReadFrame(bytes.NewReader(nil)); !errors.Is(err, io.EOF) {
			t.Fatalf("got %v, want io.EOF", err)
		}
	})
	t.Run("ReadFrame oversized", func(t *testing.T) {
		if _, err := ReadFrame(bytes.NewReader(withLength(MaxFrameSize + 4))); !errors.Is(err, ErrOversizedFrame) {
			t.Fatalf("got %v, want ErrOversizedFrame", err)
		}
	})
	t.Run("AppendTo oversized payload", func(t *testing.T) {
		f := Frame{ObjectID: 1, Payload: make([]byte, MaxFrameSize)}
		if _, err := f.AppendTo(nil); !errors.Is(err, ErrOversizedFrame) {
			t.Fatalf("got %v, want ErrOversizedFrame", err)
		}
	})
	t.Run("AppendTo unaligned payload", func(t *testing.T) {
		f := Frame{ObjectID: 1, Payload: make([]byte, 3)}
		if _, err := f.AppendTo(nil); !errors.Is(err, ErrInvalidFrameLength) {
			t.Fatalf("got %v, want ErrInvalidFrameLength", err)
		}
	})
}

func TestWire_UnknownOpcodeAndTrailingEventBytes(t *testing.T) {
	if _, err := DecodeEvent(InterfaceSeat, Frame{ObjectID: 0xff00000000000002, Opcode: 99}, nil); !errors.Is(err, ErrUnknownOpcode) {
		t.Fatalf("unknown opcode: got %v", err)
	}
	if _, err := DecodeEvent(InterfaceSeat, Frame{Opcode: evSeatDone, Payload: []byte{1, 0, 0, 0}}, nil); !errors.Is(err, ErrTrailingBytes) {
		t.Fatalf("trailing bytes: got %v", err)
	}
	if _, err := DecodeEvent(InterfaceCallback, Frame{Opcode: evCallbackDone, Payload: []byte{1, 0, 0, 0}}, nil); !errors.Is(err, ErrTruncatedArgument) {
		t.Fatalf("truncated uint64: got %v", err)
	}
}

func FuzzDecodeFrame(f *testing.F) {
	seeds := []Frame{
		handshakeVersionRequest(1),
		handshakeFinishRequest(),
		seatBindRequest(3, 5),
		pointerMotionRelativeRequest(5, 1.5, -2),
		deviceFrameRequest(9, 9, 12345),
	}
	name, err := handshakeNameRequest("edgesync")
	if err != nil {
		f.Fatal(err)
	}
	seeds = append(seeds, name)
	for _, s := range seeds {
		wire, err := s.AppendTo(nil)
		if err != nil {
			f.Fatal(err)
		}
		f.Add(wire)
	}
	f.Add([]byte{})
	f.Add(bytes.Repeat([]byte{0xff}, 40))

	f.Fuzz(func(t *testing.T, data []byte) {
		frame, n, err := DecodeFrame(data)
		if err != nil {
			return
		}
		if n < HeaderSize || n > len(data) {
			t.Fatalf("consumed %d bytes of %d", n, len(data))
		}
		again, err := frame.AppendTo(nil)
		if err != nil {
			t.Fatalf("re-encode: %v", err)
		}
		if !bytes.Equal(again, data[:n]) {
			t.Fatalf("round trip mismatch:\n got % x\nwant % x", again, data[:n])
		}
		for _, iface := range append(negotiableInterfaces(), InterfaceHandshake) {
			if ev, err := DecodeEvent(iface, frame, nil); err == nil {
				closeEventFiles(ev)
			}
		}
	})
}
