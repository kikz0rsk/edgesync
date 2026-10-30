package x11

import (
	"fmt"

	"github.com/jezek/xgb/xproto"
	"github.com/jezek/xgb/xtest"
)

// Core protocol event numbers accepted by XTest FakeInput.
const (
	eventKeyPress      = 2
	eventKeyRelease    = 3
	eventButtonPress   = 4
	eventButtonRelease = 5
	eventMotionNotify  = 6
)

// Injector sends XTest fake input; the server delivers it like hardware input, so grabs held by other clients see it.
type Injector struct {
	conn *Conn
}

func (c *Conn) Injector() *Injector {
	return &Injector{conn: c}
}

func (in *Injector) MoveRelative(dx, dy int16) error {
	return in.fake(eventMotionNotify, 1, in.conn.root, dx, dy)
}

func (in *Injector) MoveAbsolute(x, y int16) error {
	return in.fake(eventMotionNotify, 0, in.conn.root, x, y)
}

func (in *Injector) Button(button byte, down bool) error {
	return in.fake(buttonType(down), button, 0, 0, 0)
}

func (in *Injector) Key(code xproto.Keycode, down bool) error {
	return in.fake(keyType(down), byte(code), 0, 0, 0)
}

func (in *Injector) fake(typ, detail byte, root xproto.Window, x, y int16) error {
	if err := xtest.FakeInputChecked(in.conn.x, typ, detail, 0, root, x, y, 0).Check(); err != nil {
		return fmt.Errorf("x11: XTest fake input type %d: %w", typ, err)
	}
	return nil
}

func buttonType(down bool) byte {
	if down {
		return eventButtonPress
	}
	return eventButtonRelease
}

func keyType(down bool) byte {
	if down {
		return eventKeyPress
	}
	return eventKeyRelease
}
