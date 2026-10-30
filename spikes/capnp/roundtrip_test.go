package capnp_test

import (
	"bytes"
	"fmt"
	"testing"

	"capnproto.org/go/capnp/v3"

	"github.com/kikz0r/edgesync/spikes/capnp/gen"
)

const (
	controlTraverseLimitWords = 8 << 20
	controlMaxMessageBytes    = 16 << 20
	controlDepthLimit         = 32
)

type monitor struct {
	id            string
	x, y, w, h    int32
	scalePermille uint16
}

type hello struct {
	protocolVersion uint16
	buildVersion    string
	role            gen.Hello_Role
	name            string
	peerID          []byte
	macs            [][]byte
	monitors        []monitor
}

func (h hello) equal(o hello) bool {
	if h.protocolVersion != o.protocolVersion || h.buildVersion != o.buildVersion || h.role != o.role ||
		h.name != o.name || !bytes.Equal(h.peerID, o.peerID) ||
		len(h.macs) != len(o.macs) || len(h.monitors) != len(o.monitors) {
		return false
	}
	for i := range h.macs {
		if !bytes.Equal(h.macs[i], o.macs[i]) {
			return false
		}
	}
	for i := range h.monitors {
		if h.monitors[i] != o.monitors[i] {
			return false
		}
	}
	return true
}

type clipFormat struct {
	mime string
	size int64
}

type clipAnnounce struct {
	origin  []byte
	seq     uint64
	formats []clipFormat
}

func (c clipAnnounce) equal(o clipAnnounce) bool {
	if !bytes.Equal(c.origin, o.origin) || c.seq != o.seq || len(c.formats) != len(o.formats) {
		return false
	}
	for i := range c.formats {
		if c.formats[i] != o.formats[i] {
			return false
		}
	}
	return true
}

func sampleHello() hello {
	return hello{
		protocolVersion: 1,
		buildVersion:    "0.0.1-spike",
		role:            gen.Hello_Role_server,
		name:            "desk-a",
		peerID:          bytes.Repeat([]byte{0x11}, 16),
		macs:            [][]byte{{0x00, 0x15, 0x5d, 0x01, 0x02, 0x03}, {0x00, 0x15, 0x5d, 0x0a, 0x0b, 0x0c}},
		monitors: []monitor{
			{id: "DISPLAY-1", x: 0, y: 0, w: 2560, h: 1440, scalePermille: 1000},
			{id: "DISPLAY-2", x: 2560, y: -120, w: 1920, h: 1080, scalePermille: 1250},
		},
	}
}

func sampleClipAnnounce() clipAnnounce {
	return clipAnnounce{
		origin: bytes.Repeat([]byte{0x22}, 16),
		seq:    42,
		formats: []clipFormat{
			{mime: "text/plain;charset=utf-8", size: 1024},
			{mime: "image/png", size: 1 << 20},
		},
	}
}

func decodeLimited(data []byte, traverseWords uint64, depth uint) (*capnp.Message, error) {
	if len(data) > controlMaxMessageBytes {
		return nil, fmt.Errorf("%w: %d bytes", errMessageTooLarge, len(data))
	}
	msg, err := capnp.Unmarshal(data)
	if err != nil {
		return nil, fmt.Errorf("unmarshal: %w", err)
	}
	msg.TraverseLimit = traverseWords * 8
	msg.DepthLimit = depth
	return msg, nil
}

func decodeHelloLimited(data []byte) (hello, error) {
	msg, err := decodeLimited(data, controlTraverseLimitWords, controlDepthLimit)
	if err != nil {
		return hello{}, err
	}
	root, err := gen.ReadRootHello(msg)
	if err != nil {
		return hello{}, fmt.Errorf("root hello: %w", err)
	}
	var h hello
	h.protocolVersion = root.ProtocolVersion()
	h.role = root.Role()
	if h.buildVersion, err = root.BuildVersion(); err != nil {
		return hello{}, fmt.Errorf("build version: %w", err)
	}
	if h.name, err = root.Name(); err != nil {
		return hello{}, fmt.Errorf("name: %w", err)
	}
	if h.peerID, err = root.PeerId(); err != nil {
		return hello{}, fmt.Errorf("peer id: %w", err)
	}
	macs, err := root.Macs()
	if err != nil {
		return hello{}, fmt.Errorf("macs: %w", err)
	}
	if h.macs, err = readDataList(macs); err != nil {
		return hello{}, fmt.Errorf("macs: %w", err)
	}
	desktop, err := root.Desktop()
	if err != nil {
		return hello{}, fmt.Errorf("desktop: %w", err)
	}
	monitors, err := desktop.Monitors()
	if err != nil {
		return hello{}, fmt.Errorf("monitors: %w", err)
	}
	h.monitors = make([]monitor, 0, monitors.Len())
	for i := range monitors.Len() {
		m := monitors.At(i)
		var mon monitor
		if mon.id, err = m.Id(); err != nil {
			return hello{}, fmt.Errorf("monitor %d id: %w", i, err)
		}
		mon.x, mon.y, mon.w, mon.h = m.X(), m.Y(), m.W(), m.H()
		mon.scalePermille = m.ScalePermille()
		h.monitors = append(h.monitors, mon)
	}
	return h, nil
}

func encodeHello(h hello) ([]byte, error) {
	msg, seg, err := capnp.NewMessage(capnp.SingleSegment(nil))
	if err != nil {
		return nil, fmt.Errorf("new message: %w", err)
	}
	root, err := gen.NewRootHello(seg)
	if err != nil {
		return nil, fmt.Errorf("root hello: %w", err)
	}
	root.SetProtocolVersion(h.protocolVersion)
	root.SetRole(h.role)
	if err := root.SetBuildVersion(h.buildVersion); err != nil {
		return nil, fmt.Errorf("build version: %w", err)
	}
	if err := root.SetName(h.name); err != nil {
		return nil, fmt.Errorf("name: %w", err)
	}
	if err := root.SetPeerId(h.peerID); err != nil {
		return nil, fmt.Errorf("peer id: %w", err)
	}
	macs, err := root.NewMacs(int32(len(h.macs)))
	if err != nil {
		return nil, fmt.Errorf("macs: %w", err)
	}
	for i, mac := range h.macs {
		if err := macs.Set(i, mac); err != nil {
			return nil, fmt.Errorf("mac %d: %w", i, err)
		}
	}
	desktop, err := root.NewDesktop()
	if err != nil {
		return nil, fmt.Errorf("desktop: %w", err)
	}
	monitors, err := desktop.NewMonitors(int32(len(h.monitors)))
	if err != nil {
		return nil, fmt.Errorf("monitors: %w", err)
	}
	for i, m := range h.monitors {
		mon := monitors.At(i)
		if err := mon.SetId(m.id); err != nil {
			return nil, fmt.Errorf("monitor %d id: %w", i, err)
		}
		mon.SetX(m.x)
		mon.SetY(m.y)
		mon.SetW(m.w)
		mon.SetH(m.h)
		mon.SetScalePermille(m.scalePermille)
	}
	return marshalCapped(msg)
}

func marshalCapped(msg *capnp.Message) ([]byte, error) {
	out, err := msg.Marshal()
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}
	if len(out) > controlMaxMessageBytes {
		return nil, fmt.Errorf("%w: %d bytes", errMessageTooLarge, len(out))
	}
	return out, nil
}

func readDataList(l capnp.DataList) ([][]byte, error) {
	out := make([][]byte, 0, l.Len())
	for i := range l.Len() {
		b, err := l.At(i)
		if err != nil {
			return nil, fmt.Errorf("element %d: %w", i, err)
		}
		out = append(out, b)
	}
	return out, nil
}

func encodeClipAnnounce(c clipAnnounce) ([]byte, error) {
	msg, seg, err := capnp.NewMessage(capnp.SingleSegment(nil))
	if err != nil {
		return nil, fmt.Errorf("new message: %w", err)
	}
	root, err := gen.NewRootClipAnnounce(seg)
	if err != nil {
		return nil, fmt.Errorf("root clip announce: %w", err)
	}
	if err := root.SetOrigin(c.origin); err != nil {
		return nil, fmt.Errorf("origin: %w", err)
	}
	root.SetSeq(c.seq)
	formats, err := root.NewFormats(int32(len(c.formats)))
	if err != nil {
		return nil, fmt.Errorf("formats: %w", err)
	}
	for i, f := range c.formats {
		fm := formats.At(i)
		if err := fm.SetMime(f.mime); err != nil {
			return nil, fmt.Errorf("format %d mime: %w", i, err)
		}
		fm.SetSize(f.size)
	}
	return marshalCapped(msg)
}

func decodeClipAnnounce(data []byte) (clipAnnounce, error) {
	msg, err := decodeLimited(data, controlTraverseLimitWords, controlDepthLimit)
	if err != nil {
		return clipAnnounce{}, err
	}
	root, err := gen.ReadRootClipAnnounce(msg)
	if err != nil {
		return clipAnnounce{}, fmt.Errorf("root clip announce: %w", err)
	}
	var c clipAnnounce
	if c.origin, err = root.Origin(); err != nil {
		return clipAnnounce{}, fmt.Errorf("origin: %w", err)
	}
	c.seq = root.Seq()
	formats, err := root.Formats()
	if err != nil {
		return clipAnnounce{}, fmt.Errorf("formats: %w", err)
	}
	c.formats = make([]clipFormat, 0, formats.Len())
	for i := range formats.Len() {
		fm := formats.At(i)
		mime, err := fm.Mime()
		if err != nil {
			return clipAnnounce{}, fmt.Errorf("format %d mime: %w", i, err)
		}
		c.formats = append(c.formats, clipFormat{mime: mime, size: fm.Size()})
	}
	return c, nil
}

func TestRoundTrip_Hello(t *testing.T) {
	want := sampleHello()
	data, err := encodeHello(want)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	got, err := decodeHelloLimited(data)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !got.equal(want) {
		t.Fatalf("round trip mismatch:\n got %+v\nwant %+v", got, want)
	}
}

func TestRoundTrip_ClipAnnounce(t *testing.T) {
	want := sampleClipAnnounce()
	data, err := encodeClipAnnounce(want)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	got, err := decodeClipAnnounce(data)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !got.equal(want) {
		t.Fatalf("round trip mismatch:\n got %+v\nwant %+v", got, want)
	}
}
