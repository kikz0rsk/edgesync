package x11

import (
	"context"
	"errors"
	"fmt"

	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
)

var ErrGrabRefused = errors.New("x11: grab refused by server")

const pointerGrabMask = xproto.EventMaskPointerMotion | xproto.EventMaskButtonPress | xproto.EventMaskButtonRelease

// Capture holds the pointer and the keyboard on a fullscreen override-redirect InputOnly window. Each pointer motion
// is reported as its offset from the screen centre and the pointer is warped back to the centre.
type Capture struct {
	conn    *Conn
	window  xproto.Window
	centreX int32
	centreY int32
}

type Delta struct {
	DX int32
	DY int32
}

func (c *Conn) StartCapture() (*Capture, error) {
	window, err := c.newWindow(xproto.WindowClassInputOnly, c.width, c.height, xproto.CwOverrideRedirect, []uint32{1})
	if err != nil {
		return nil, err
	}
	capture := &Capture{
		conn:    c,
		window:  window,
		centreX: int32(c.width) / 2,
		centreY: int32(c.height) / 2,
	}
	if err := xproto.MapWindowChecked(c.x, window).Check(); err != nil {
		return nil, capture.abort(fmt.Errorf("x11: map capture window: %w", err))
	}
	pointer, err := xproto.GrabPointer(c.x, false, window, pointerGrabMask, xproto.GrabModeAsync, xproto.GrabModeAsync, 0, 0, xproto.TimeCurrentTime).Reply()
	if err != nil {
		return nil, capture.abort(fmt.Errorf("x11: grab pointer: %w", err))
	}
	if pointer.Status != xproto.GrabStatusSuccess {
		return nil, capture.abort(fmt.Errorf("%w: pointer status %d", ErrGrabRefused, pointer.Status))
	}
	keyboard, err := xproto.GrabKeyboard(c.x, false, window, xproto.TimeCurrentTime, xproto.GrabModeAsync, xproto.GrabModeAsync).Reply()
	if err != nil {
		return nil, capture.abort(fmt.Errorf("x11: grab keyboard: %w", err))
	}
	if keyboard.Status != xproto.GrabStatusSuccess {
		return nil, capture.abort(fmt.Errorf("%w: keyboard status %d", ErrGrabRefused, keyboard.Status))
	}
	if err := capture.warpToCentre(); err != nil {
		return nil, capture.abort(err)
	}
	return capture, nil
}

// Deltas reports every pointer motion until ctx is done. Cancelling ctx closes the connection, which releases the
// grabs; the returned error is then the context error.
func (cp *Capture) Deltas(ctx context.Context, emit func(Delta)) error {
	for {
		ev, err := cp.conn.WaitEvent(ctx, isMotion)
		if err != nil {
			return err
		}
		motion, ok := ev.(xproto.MotionNotifyEvent)
		if !ok {
			return fmt.Errorf("x11: unexpected event %T", ev)
		}
		dx := int32(motion.RootX) - cp.centreX
		dy := int32(motion.RootY) - cp.centreY
		if dx == 0 && dy == 0 {
			continue
		}
		if err := cp.warpToCentre(); err != nil {
			return err
		}
		emit(Delta{DX: dx, DY: dy})
	}
}

func (cp *Capture) warpToCentre() error {
	if err := xproto.WarpPointerChecked(cp.conn.x, 0, cp.conn.root, 0, 0, 0, 0, int16(cp.centreX), int16(cp.centreY)).Check(); err != nil {
		return fmt.Errorf("x11: warp pointer to centre: %w", err)
	}
	return nil
}

func (cp *Capture) abort(cause error) error {
	c := cp.conn
	ungrabErr := errors.Join(
		xproto.UngrabPointerChecked(c.x, xproto.TimeCurrentTime).Check(),
		xproto.UngrabKeyboardChecked(c.x, xproto.TimeCurrentTime).Check(),
		xproto.UnmapWindowChecked(c.x, cp.window).Check(),
	)
	return errors.Join(cause, ungrabErr, c.destroyWindow(cp.window))
}

func isMotion(ev xgb.Event) bool {
	_, ok := ev.(xproto.MotionNotifyEvent)
	return ok
}
