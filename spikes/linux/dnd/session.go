package dnd

import (
	"encoding/binary"
	"errors"
	"fmt"
	"sync"

	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
)

var (
	ErrConnClosed       = errors.New("dnd: X connection closed")
	ErrNoScreen         = errors.New("dnd: server reports no screens")
	ErrPayloadTooLarge  = errors.New("dnd: URI list does not fit one X request")
	ErrINCRNotSupported = errors.New("dnd: the source answered with INCR, which this spike does not receive")
	ErrSelectionRefused = errors.New("dnd: the source refused to convert XdndSelection")
	ErrSelectionTimeout = errors.New("dnd: the source did not answer the XdndSelection conversion")
	ErrTruncated        = errors.New("dnd: selection data does not fit one read")
	ErrGrabRefused      = errors.New("dnd: pointer grab refused by the server")
	ErrBadMessage       = errors.New("dnd: malformed XDND ClientMessage")
)

const (
	copyFromParent    = 0
	requestSlackBytes = 1024
	typeListWords     = 4096
	whitePixel        = 0xFFFFFF
	atomNameINCR      = "INCR"
)

type displayEvent struct {
	ev  xgb.Event
	err xgb.Error
}

// Display is one X connection with the XDND atoms interned. A goroutine owned by the Display reads events into a
// channel until Close.
type Display struct {
	x          *xgb.Conn
	root       xproto.Window
	atoms      Atoms
	incr       uint32
	maxRequest int

	events    chan displayEvent
	done      chan struct{}
	pumpDone  chan struct{}
	closeOnce sync.Once
}

func Open(display string) (*Display, error) {
	x, err := xgb.NewConnDisplay(display)
	if err != nil {
		return nil, fmt.Errorf("dnd: connect to display %q: %w", display, err)
	}
	setup := xproto.Setup(x)
	if len(setup.Roots) == 0 {
		x.Close()
		return nil, ErrNoScreen
	}
	d := &Display{
		x:          x,
		root:       setup.Roots[0].Root,
		maxRequest: int(setup.MaximumRequestLength) * 4,
		events:     make(chan displayEvent, 64),
		done:       make(chan struct{}),
		pumpDone:   make(chan struct{}),
	}
	if d.atoms, err = NewAtoms(d.intern); err != nil {
		x.Close()
		return nil, err
	}
	if d.incr, err = d.intern(atomNameINCR); err != nil {
		x.Close()
		return nil, err
	}
	go d.pump()
	return d, nil
}

func (d *Display) Atoms() Atoms {
	return d.atoms
}

// Close closes the connection, which also destroys every window and selection the Display created.
func (d *Display) Close() {
	d.closeOnce.Do(func() {
		close(d.done)
		d.x.Close()
		<-d.pumpDone
	})
}

func (d *Display) pump() {
	defer close(d.pumpDone)
	defer close(d.events)
	for {
		ev, xerr := d.x.WaitForEvent()
		if ev == nil && xerr == nil {
			return
		}
		select {
		case d.events <- displayEvent{ev: ev, err: xerr}:
		case <-d.done:
			return
		}
	}
}

func (d *Display) intern(name string) (uint32, error) {
	reply, err := xproto.InternAtom(d.x, false, uint16(len(name)), name).Reply()
	if err != nil {
		return 0, fmt.Errorf("dnd: intern atom %s: %w", name, err)
	}
	return uint32(reply.Atom), nil
}

// AtomName resolves an atom id, for printing the types a source offers.
func (d *Display) AtomName(id uint32) (string, error) {
	reply, err := xproto.GetAtomName(d.x, xproto.Atom(id)).Reply()
	if err != nil {
		return "", fmt.Errorf("dnd: name of atom %d: %w", id, err)
	}
	return reply.Name, nil
}

func unwrap(de displayEvent, open bool) (xgb.Event, error) {
	switch {
	case !open:
		return nil, ErrConnClosed
	case de.err != nil:
		return nil, fmt.Errorf("dnd: X error: %w", de.err)
	}
	return de.ev, nil
}

func (d *Display) newWindow(class uint16, x, y int16, w, h uint16, mask uint32, values []uint32) (xproto.Window, error) {
	id, err := d.x.NewId()
	if err != nil {
		return 0, fmt.Errorf("dnd: allocate window id: %w", err)
	}
	window := xproto.Window(id)
	if err := xproto.CreateWindowChecked(d.x, copyFromParent, window, d.root, x, y, w, h, 0, class, copyFromParent, mask, values).Check(); err != nil {
		return 0, fmt.Errorf("dnd: create window: %w", err)
	}
	return window, nil
}

func (d *Display) mapVisible(x, y int16, w, h uint16, title string, eventMask uint32) (xproto.Window, error) {
	window, err := d.newWindow(xproto.WindowClassInputOutput, x, y, w, h, xproto.CwBackPixel|xproto.CwEventMask, []uint32{whitePixel, eventMask})
	if err != nil {
		return 0, err
	}
	if err := xproto.ChangePropertyChecked(d.x, xproto.PropModeReplace, window, xproto.AtomWmName, xproto.AtomString, 8, uint32(len(title)), []byte(title)).Check(); err != nil {
		return 0, errors.Join(fmt.Errorf("dnd: set window title: %w", err), d.destroy(window))
	}
	if err := xproto.MapWindowChecked(d.x, window).Check(); err != nil {
		return 0, errors.Join(fmt.Errorf("dnd: map window: %w", err), d.destroy(window))
	}
	return window, nil
}

func (d *Display) destroy(window xproto.Window) error {
	if err := xproto.DestroyWindowChecked(d.x, window).Check(); err != nil {
		return fmt.Errorf("dnd: destroy window %d: %w", window, err)
	}
	return nil
}

func (d *Display) send(o Outbound) error {
	ev := xproto.ClientMessageEvent{
		Format: 32,
		Window: xproto.Window(o.Window),
		Type:   xproto.Atom(d.atoms.MessageType(o.Kind)),
		Data:   xproto.ClientMessageDataUnionData32New(o.Data[:]),
	}
	if err := xproto.SendEventChecked(d.x, false, xproto.Window(o.Destination), 0, string(ev.Bytes())).Check(); err != nil {
		return fmt.Errorf("dnd: send %s to window %d: %w", o.Kind, o.Destination, err)
	}
	return nil
}

func (d *Display) setWords(window xproto.Window, property, typ uint32, words []uint32) error {
	raw := make([]byte, 4*len(words))
	for i, w := range words {
		binary.LittleEndian.PutUint32(raw[4*i:], w)
	}
	if err := xproto.ChangePropertyChecked(d.x, xproto.PropModeReplace, window, xproto.Atom(property), xproto.Atom(typ), 32, uint32(len(words)), raw).Check(); err != nil {
		return fmt.Errorf("dnd: set property %d on window %d: %w", property, window, err)
	}
	return nil
}

// words reads a 32-bit property of the given type; a missing or differently typed property reads as no words.
func (d *Display) words(window xproto.Window, property, typ uint32, limit uint32) ([]uint32, error) {
	reply, err := xproto.GetProperty(d.x, false, window, xproto.Atom(property), xproto.Atom(typ), 0, limit).Reply()
	if err != nil {
		return nil, fmt.Errorf("dnd: read property %d of window %d: %w", property, window, err)
	}
	if reply.Format != 32 || reply.ValueLen == 0 {
		return nil, nil
	}
	out := make([]uint32, reply.ValueLen)
	for i := range out {
		out[i] = binary.LittleEndian.Uint32(reply.Value[4*i:])
	}
	return out, nil
}

func isVanished(err error) bool {
	var window xproto.WindowError
	return errors.As(err, &window)
}

func messageData(ev xproto.ClientMessageEvent) (Data, error) {
	if ev.Format != 32 || len(ev.Data.Data32) < len(Data{}) {
		return Data{}, fmt.Errorf("%w: format %d", ErrBadMessage, ev.Format)
	}
	var data Data
	copy(data[:], ev.Data.Data32)
	return data, nil
}
