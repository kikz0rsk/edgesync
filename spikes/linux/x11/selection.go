package x11

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xfixes"
	"github.com/jezek/xgb/xproto"
)

var (
	ErrSelectionRefused  = errors.New("x11: owner refused the selection conversion")
	ErrSelectionLost     = errors.New("x11: selection ownership lost")
	ErrUnexpectedType    = errors.New("x11: selection data has an unexpected property type")
	ErrPropertyTruncated = errors.New("x11: property does not fit one read")
)

const (
	requestSlackBytes = 1024
	atomTargets       = "TARGETS"
	atomINCR          = "INCR"
	readPropertyName  = "EDGESYNC_SELECTION"
)

// Payload is the data an Owner serves for one target. The property type of the reply is the target atom.
type Payload struct {
	Target string
	Data   []byte
}

// Owner serves a selection to other clients. Payloads that do not fit one request are sent with INCR (ICCCM 2.7.2).
type Owner struct {
	conn       *Conn
	window     xproto.Window
	selection  xproto.Atom
	targets    xproto.Atom
	incr       xproto.Atom
	targetList []byte
	payloads   map[xproto.Atom][]byte
	chunkLimit int
	transfers  map[transferKey]*outgoing
}

type transferKey struct {
	window   xproto.Window
	property xproto.Atom
}

// outgoing is the sending side of an INCR transfer: one chunk is written each time the requestor deletes the
// property, and a zero-length chunk ends the data. The transfer finishes when the requestor deletes that last chunk.
type outgoing struct {
	target xproto.Atom
	data   []byte
	offset int
	ended  bool
}

func (t *outgoing) next(limit int) (chunk []byte, finished bool) {
	if t.ended {
		return nil, true
	}
	if t.offset < len(t.data) {
		end := min(t.offset+limit, len(t.data))
		chunk = t.data[t.offset:end]
		t.offset = end
		return chunk, false
	}
	t.ended = true
	return nil, false
}

func (c *Conn) Own(selectionName string, payloads []Payload) (*Owner, error) {
	selection, err := c.atom(selectionName)
	if err != nil {
		return nil, err
	}
	targets, err := c.atom(atomTargets)
	if err != nil {
		return nil, err
	}
	incr, err := c.atom(atomINCR)
	if err != nil {
		return nil, err
	}
	window, err := c.newWindow(xproto.WindowClassInputOutput, 1, 1, 0, nil)
	if err != nil {
		return nil, err
	}
	o := &Owner{
		conn:       c,
		window:     window,
		selection:  selection,
		targets:    targets,
		incr:       incr,
		payloads:   make(map[xproto.Atom][]byte, len(payloads)),
		chunkLimit: maxChunkBytes(c.maxRequestBytes),
		transfers:  make(map[transferKey]*outgoing),
	}
	advertised := []xproto.Atom{targets}
	for _, p := range payloads {
		atom, err := c.atom(p.Target)
		if err != nil {
			return nil, err
		}
		o.payloads[atom] = p.Data
		advertised = append(advertised, atom)
	}
	o.targetList = encodeAtoms(advertised)
	if err := xproto.SetSelectionOwnerChecked(c.x, window, selection, xproto.TimeCurrentTime).Check(); err != nil {
		return nil, fmt.Errorf("x11: claim %s: %w", selectionName, err)
	}
	owner, err := xproto.GetSelectionOwner(c.x, selection).Reply()
	if err != nil {
		return nil, fmt.Errorf("x11: read owner of %s: %w", selectionName, err)
	}
	if owner.Owner != window {
		return nil, fmt.Errorf("%w: claim of %s did not take effect", ErrSelectionLost, selectionName)
	}
	return o, nil
}

// Serve answers selection requests until ctx is done or the selection is taken by another client.
func (o *Owner) Serve(ctx context.Context) error {
	for {
		ev, err := o.conn.WaitEvent(ctx, isServing)
		if err != nil {
			return err
		}
		switch e := ev.(type) {
		case xproto.SelectionRequestEvent:
			if err := o.handleRequest(e); err != nil {
				return err
			}
		case xproto.PropertyNotifyEvent:
			if err := o.advance(e); err != nil {
				return err
			}
		case xproto.SelectionClearEvent:
			if e.Selection == o.selection {
				return ErrSelectionLost
			}
		}
	}
}

func (o *Owner) handleRequest(req xproto.SelectionRequestEvent) error {
	if req.Selection != o.selection {
		return o.reply(req, 0)
	}
	property := req.Property
	if property == 0 {
		property = req.Target
	}
	if req.Target == o.targets {
		return o.writeWhole(req, property, o.targetList, xproto.AtomAtom, 32)
	}
	data, ok := o.payloads[req.Target]
	if !ok {
		return o.reply(req, 0)
	}
	if len(data) > o.chunkLimit {
		return o.startINCR(req, property, data)
	}
	return o.writeWhole(req, property, data, req.Target, 8)
}

func (o *Owner) writeWhole(req xproto.SelectionRequestEvent, property xproto.Atom, data []byte, typ xproto.Atom, format byte) error {
	if err := xproto.ChangePropertyChecked(o.conn.x, xproto.PropModeReplace, req.Requestor, property, typ, format, unitsOf(len(data), format), data).Check(); err != nil {
		return fmt.Errorf("x11: write property for requestor %d: %w", req.Requestor, err)
	}
	return o.reply(req, property)
}

func (o *Owner) startINCR(req xproto.SelectionRequestEvent, property xproto.Atom, data []byte) error {
	if err := xproto.ChangeWindowAttributesChecked(o.conn.x, req.Requestor, xproto.CwEventMask, []uint32{xproto.EventMaskPropertyChange}).Check(); err != nil {
		return fmt.Errorf("x11: watch requestor %d properties: %w", req.Requestor, err)
	}
	size := make([]byte, 4)
	binary.LittleEndian.PutUint32(size, uint32(len(data)))
	if err := xproto.ChangePropertyChecked(o.conn.x, xproto.PropModeReplace, req.Requestor, property, o.incr, 32, 1, size).Check(); err != nil {
		return fmt.Errorf("x11: write INCR marker for requestor %d: %w", req.Requestor, err)
	}
	o.transfers[transferKey{window: req.Requestor, property: property}] = &outgoing{target: req.Target, data: data}
	return o.reply(req, property)
}

func (o *Owner) advance(ev xproto.PropertyNotifyEvent) error {
	key := transferKey{window: ev.Window, property: ev.Atom}
	t, ok := o.transfers[key]
	if !ok {
		return nil
	}
	chunk, finished := t.next(o.chunkLimit)
	if finished {
		delete(o.transfers, key)
		return nil
	}
	if err := xproto.ChangePropertyChecked(o.conn.x, xproto.PropModeReplace, ev.Window, ev.Atom, t.target, 8, unitsOf(len(chunk), 8), chunk).Check(); err != nil {
		return fmt.Errorf("x11: write INCR chunk for requestor %d: %w", ev.Window, err)
	}
	return nil
}

func (o *Owner) reply(req xproto.SelectionRequestEvent, property xproto.Atom) error {
	notify := xproto.SelectionNotifyEvent{
		Time:      req.Time,
		Requestor: req.Requestor,
		Selection: req.Selection,
		Target:    req.Target,
		Property:  property,
	}
	if err := xproto.SendEventChecked(o.conn.x, false, req.Requestor, 0, string(notify.Bytes())).Check(); err != nil {
		return fmt.Errorf("x11: send SelectionNotify to %d: %w", req.Requestor, err)
	}
	return nil
}

// Transfer is the data read from a selection. Incremental reports whether the owner used INCR.
type Transfer struct {
	Data        []byte
	Incremental bool
}

func (c *Conn) ReadSelection(ctx context.Context, selectionName, targetName string) (Transfer, error) {
	selection, err := c.atom(selectionName)
	if err != nil {
		return Transfer{}, err
	}
	target, err := c.atom(targetName)
	if err != nil {
		return Transfer{}, err
	}
	property, err := c.atom(readPropertyName)
	if err != nil {
		return Transfer{}, err
	}
	incr, err := c.atom(atomINCR)
	if err != nil {
		return Transfer{}, err
	}
	window, err := c.newWindow(xproto.WindowClassInputOutput, 1, 1, xproto.CwEventMask, []uint32{xproto.EventMaskPropertyChange})
	if err != nil {
		return Transfer{}, err
	}
	if err := xproto.ConvertSelectionChecked(c.x, window, selection, target, property, xproto.TimeCurrentTime).Check(); err != nil {
		return Transfer{}, errors.Join(fmt.Errorf("x11: convert %s: %w", selectionName, err), c.destroyWindow(window))
	}
	transfer, err := c.collect(ctx, window, property, target, incr)
	return transfer, errors.Join(err, c.destroyWindow(window))
}

func (c *Conn) collect(ctx context.Context, window xproto.Window, property, target, incr xproto.Atom) (Transfer, error) {
	var (
		receiving   incrReceiver
		incremental bool
	)
	for {
		ev, err := c.WaitEvent(ctx, isTransferEvent(window, property))
		if err != nil {
			return Transfer{}, err
		}
		switch e := ev.(type) {
		case xproto.SelectionNotifyEvent:
			if e.Property == 0 {
				return Transfer{}, ErrSelectionRefused
			}
			data, typ, err := c.takeProperty(window, property)
			if err != nil {
				return Transfer{}, err
			}
			if typ == incr {
				incremental = true
				continue
			}
			if typ != target {
				return Transfer{}, fmt.Errorf("%w: got %d, want %d", ErrUnexpectedType, typ, target)
			}
			return Transfer{Data: data}, nil
		case xproto.PropertyNotifyEvent:
			if !incremental {
				continue
			}
			data, _, err := c.takeProperty(window, property)
			if err != nil {
				return Transfer{}, err
			}
			if receiving.add(data) {
				return Transfer{Data: receiving.data, Incremental: true}, nil
			}
		}
	}
}

func (c *Conn) takeProperty(window xproto.Window, property xproto.Atom) ([]byte, xproto.Atom, error) {
	reply, err := xproto.GetProperty(c.x, true, window, property, xproto.AtomAny, 0, uint32(c.maxRequestBytes/4)).Reply()
	if err != nil {
		return nil, 0, fmt.Errorf("x11: read property: %w", err)
	}
	if reply.BytesAfter != 0 {
		return nil, 0, fmt.Errorf("%w: %d bytes after", ErrPropertyTruncated, reply.BytesAfter)
	}
	return reply.Value, reply.Type, nil
}

// incrReceiver is the receiving side of an INCR transfer: chunks accumulate until a zero-length chunk ends the data.
type incrReceiver struct {
	data []byte
}

func (r *incrReceiver) add(chunk []byte) (done bool) {
	if len(chunk) == 0 {
		return true
	}
	r.data = append(r.data, chunk...)
	return false
}

// WatchSelection selects XFixes SetSelectionOwner notifications for a selection on a new window.
func (c *Conn) WatchSelection(selectionName string) (*SelectionWatch, error) {
	selection, err := c.atom(selectionName)
	if err != nil {
		return nil, err
	}
	window, err := c.newWindow(xproto.WindowClassInputOutput, 1, 1, 0, nil)
	if err != nil {
		return nil, err
	}
	if err := xfixes.SelectSelectionInputChecked(c.x, window, selection, xfixes.SelectionEventMaskSetSelectionOwner).Check(); err != nil {
		return nil, errors.Join(fmt.Errorf("x11: XFixes SelectSelectionInput %s: %w", selectionName, err), c.destroyWindow(window))
	}
	return &SelectionWatch{conn: c, selection: selection}, nil
}

// SelectionWatch reports owner changes of one selection.
type SelectionWatch struct {
	conn      *Conn
	selection xproto.Atom
}

// NextOwnerChange blocks until the selection changes owner and returns the new owner window (0 when unowned).
func (w *SelectionWatch) NextOwnerChange(ctx context.Context) (xproto.Window, error) {
	ev, err := w.conn.WaitEvent(ctx, func(ev xgb.Event) bool {
		notify, ok := ev.(xfixes.SelectionNotifyEvent)
		return ok && notify.Subtype == xfixes.SelectionEventSetSelectionOwner && notify.Selection == w.selection
	})
	if err != nil {
		return 0, err
	}
	notify, ok := ev.(xfixes.SelectionNotifyEvent)
	if !ok {
		return 0, fmt.Errorf("x11: unexpected event %T", ev)
	}
	return notify.Owner, nil
}

func isServing(ev xgb.Event) bool {
	switch e := ev.(type) {
	case xproto.SelectionRequestEvent, xproto.SelectionClearEvent:
		return true
	case xproto.PropertyNotifyEvent:
		return e.State == xproto.PropertyDelete
	}
	return false
}

func isTransferEvent(window xproto.Window, property xproto.Atom) func(xgb.Event) bool {
	return func(ev xgb.Event) bool {
		switch e := ev.(type) {
		case xproto.SelectionNotifyEvent:
			return e.Requestor == window
		case xproto.PropertyNotifyEvent:
			return e.Window == window && e.Atom == property && e.State == xproto.PropertyNewValue
		}
		return false
	}
}

func maxChunkBytes(maxRequestBytes int) int {
	return maxRequestBytes - requestSlackBytes
}

func unitsOf(size int, format byte) uint32 {
	return uint32(size * 8 / int(format))
}

func encodeAtoms(atoms []xproto.Atom) []byte {
	out := make([]byte, 4*len(atoms))
	for i, atom := range atoms {
		binary.LittleEndian.PutUint32(out[4*i:], uint32(atom))
	}
	return out
}
