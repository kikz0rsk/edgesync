// Package x11 is the Linux X11 spike (P0-SP3-004). It drives an X server through github.com/jezek/xgb, a pure Go
// client without cgo: pointer and keyboard grabs with warp-to-centre deltas, XTest injection, RandR 1.5 monitors,
// XFixes selection owner notifications and CLIPBOARD transfers with INCR in both directions.
package x11

import (
	"context"
	"errors"
	"fmt"

	"github.com/jezek/xgb"
	"github.com/jezek/xgb/randr"
	"github.com/jezek/xgb/xfixes"
	"github.com/jezek/xgb/xproto"
	"github.com/jezek/xgb/xtest"
)

var (
	ErrConnClosed = errors.New("x11: connection closed")
	ErrRandR15    = errors.New("x11: server does not provide RandR 1.5")
)

const (
	xtestMajor  = 2
	xtestMinor  = 2
	randrMajor  = 1
	randrMinor  = 5
	xfixesMajor = 5
	xfixesMinor = 0

	copyFromParent = 0
)

type Conn struct {
	x               *xgb.Conn
	root            xproto.Window
	width           uint16
	height          uint16
	maxRequestBytes int
}

func Open(display string) (*Conn, error) {
	x, err := xgb.NewConnDisplay(display)
	if err != nil {
		return nil, fmt.Errorf("x11: connect to display %q: %w", display, err)
	}
	c := &Conn{x: x}
	if err := c.initExtensions(); err != nil {
		x.Close()
		return nil, err
	}
	setup := xproto.Setup(x)
	if len(setup.Roots) == 0 {
		x.Close()
		return nil, errors.New("x11: server reports no screens")
	}
	screen := setup.Roots[0]
	c.root = screen.Root
	c.width = screen.WidthInPixels
	c.height = screen.HeightInPixels
	c.maxRequestBytes = int(setup.MaximumRequestLength) * 4
	if c.maxRequestBytes <= requestSlackBytes {
		x.Close()
		return nil, fmt.Errorf("x11: server maximum request %d bytes is too small", c.maxRequestBytes)
	}
	return c, nil
}

func (c *Conn) initExtensions() error {
	if err := xtest.Init(c.x); err != nil {
		return fmt.Errorf("x11: init XTEST: %w", err)
	}
	if _, err := xtest.GetVersion(c.x, xtestMajor, xtestMinor).Reply(); err != nil {
		return fmt.Errorf("x11: XTEST version: %w", err)
	}
	if err := randr.Init(c.x); err != nil {
		return fmt.Errorf("x11: init RANDR: %w", err)
	}
	rv, err := randr.QueryVersion(c.x, randrMajor, randrMinor).Reply()
	if err != nil {
		return fmt.Errorf("x11: RANDR version: %w", err)
	}
	if rv.MajorVersion < randrMajor || (rv.MajorVersion == randrMajor && rv.MinorVersion < randrMinor) {
		return fmt.Errorf("%w: got %d.%d", ErrRandR15, rv.MajorVersion, rv.MinorVersion)
	}
	if err := xfixes.Init(c.x); err != nil {
		return fmt.Errorf("x11: init XFIXES: %w", err)
	}
	if _, err := xfixes.QueryVersion(c.x, xfixesMajor, xfixesMinor).Reply(); err != nil {
		return fmt.Errorf("x11: XFIXES version: %w", err)
	}
	return nil
}

func (c *Conn) Close() {
	c.x.Close()
}

func (c *Conn) atom(name string) (xproto.Atom, error) {
	reply, err := xproto.InternAtom(c.x, false, uint16(len(name)), name).Reply()
	if err != nil {
		return 0, fmt.Errorf("x11: intern atom %s: %w", name, err)
	}
	return reply.Atom, nil
}

func (c *Conn) atomName(atom xproto.Atom) (string, error) {
	reply, err := xproto.GetAtomName(c.x, atom).Reply()
	if err != nil {
		return "", fmt.Errorf("x11: name of atom %d: %w", atom, err)
	}
	return reply.Name, nil
}

func (c *Conn) newWindow(class uint16, width, height uint16, valueMask uint32, values []uint32) (xproto.Window, error) {
	id, err := c.x.NewId()
	if err != nil {
		return 0, fmt.Errorf("x11: allocate window id: %w", err)
	}
	window := xproto.Window(id)
	if err := xproto.CreateWindowChecked(c.x, copyFromParent, window, c.root, 0, 0, width, height, 0, class, copyFromParent, valueMask, values).Check(); err != nil {
		return 0, fmt.Errorf("x11: create window: %w", err)
	}
	return window, nil
}

func (c *Conn) destroyWindow(window xproto.Window) error {
	if err := xproto.DestroyWindowChecked(c.x, window).Check(); err != nil {
		return fmt.Errorf("x11: destroy window %d: %w", window, err)
	}
	return nil
}

// WaitEvent returns the next event accepted by match (every event when match is nil). Cancelling ctx closes the
// connection, which also ends any grab and selection held through it, and the context error is returned.
func (c *Conn) WaitEvent(ctx context.Context, match func(xgb.Event) bool) (xgb.Event, error) {
	stop := context.AfterFunc(ctx, c.x.Close)
	defer stop()
	for {
		ev, xerr := c.x.WaitForEvent()
		if xerr != nil {
			return nil, fmt.Errorf("x11: X error: %w", xerr)
		}
		if ev == nil {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			return nil, ErrConnClosed
		}
		if match == nil || match(ev) {
			return ev, nil
		}
	}
}
