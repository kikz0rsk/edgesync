package fw

import (
	"errors"
	"fmt"
	"structs"
)

var (
	ErrCoreGraphics   = errors.New("CoreGraphics call failed")
	ErrInvalidDisplay = errors.New("display reports no bounds or pixels")
	ErrNullEvent      = errors.New("CGEventCreate returned NULL")
)

type CGPoint struct {
	_    structs.HostLayout
	X, Y float64
}

type CGSize struct {
	_             structs.HostLayout
	Width, Height float64
}

type CGRect struct {
	_      structs.HostLayout
	Origin CGPoint
	Size   CGSize
}

type CoreGraphics struct {
	getActiveDisplayList    func(maxDisplays uint32, displays *uint32, count *uint32) int32
	mainDisplayID           func() uint32
	displayBounds           func(display uint32) CGRect
	displayPixelsWide       func(display uint32) uintptr
	displayPixelsHigh       func(display uint32) uintptr
	warpMouseCursorPosition func(point CGPoint) int32
	eventCreate             func(source uintptr) uintptr
	eventGetLocation        func(event uintptr) CGPoint
}

type Display struct {
	ID         uint32
	Main       bool
	Bounds     CGRect
	PixelsWide uintptr
	PixelsHigh uintptr
	Scale      float64
}

func bindCoreGraphics(handle uintptr) (*CoreGraphics, error) {
	b := binder{handle: handle}
	cg := &CoreGraphics{}
	b.bind(&cg.getActiveDisplayList, "CGGetActiveDisplayList")
	b.bind(&cg.mainDisplayID, "CGMainDisplayID")
	b.bind(&cg.displayBounds, "CGDisplayBounds")
	b.bind(&cg.displayPixelsWide, "CGDisplayPixelsWide")
	b.bind(&cg.displayPixelsHigh, "CGDisplayPixelsHigh")
	b.bind(&cg.warpMouseCursorPosition, "CGWarpMouseCursorPosition")
	b.bind(&cg.eventCreate, "CGEventCreate")
	b.bind(&cg.eventGetLocation, "CGEventGetLocation")
	if b.err != nil {
		return nil, b.err
	}
	return cg, nil
}

func (cg *CoreGraphics) ActiveDisplays() ([]Display, error) {
	var count uint32
	if err := checkCGError("CGGetActiveDisplayList", cg.getActiveDisplayList(0, nil, &count)); err != nil {
		return nil, err
	}
	if count == 0 {
		return nil, nil
	}

	ids := make([]uint32, count)
	var listed uint32
	if err := checkCGError("CGGetActiveDisplayList", cg.getActiveDisplayList(count, &ids[0], &listed)); err != nil {
		return nil, err
	}
	if listed > count {
		return nil, fmt.Errorf("CGGetActiveDisplayList listed %d displays for a buffer of %d: %w", listed, count, ErrCoreGraphics)
	}

	mainID := cg.mainDisplayID()
	displays := make([]Display, 0, listed)
	for _, id := range ids[:listed] {
		d, err := cg.display(id, id == mainID)
		if err != nil {
			return nil, err
		}
		displays = append(displays, d)
	}
	return displays, nil
}

func (cg *CoreGraphics) display(id uint32, main bool) (Display, error) {
	bounds := cg.displayBounds(id)
	wide := cg.displayPixelsWide(id)
	high := cg.displayPixelsHigh(id)
	if bounds.Size.Width <= 0 || bounds.Size.Height <= 0 || wide == 0 || high == 0 {
		return Display{}, fmt.Errorf("display %d: %w", id, ErrInvalidDisplay)
	}
	return Display{
		ID:         id,
		Main:       main,
		Bounds:     bounds,
		PixelsWide: wide,
		PixelsHigh: high,
		Scale:      float64(wide) / bounds.Size.Width,
	}, nil
}

func (cg *CoreGraphics) WarpCursor(to CGPoint) error {
	return checkCGError("CGWarpMouseCursorPosition", cg.warpMouseCursorPosition(to))
}

func (cg *CoreGraphics) CursorLocation(cf *CoreFoundation) (CGPoint, error) {
	event := cg.eventCreate(0)
	if event == 0 {
		return CGPoint{}, ErrNullEvent
	}
	defer cf.Release(event)
	return cg.eventGetLocation(event), nil
}

func checkCGError(call string, code int32) error {
	if code == 0 {
		return nil
	}
	return fmt.Errorf("%s: CGError %d: %w", call, code, ErrCoreGraphics)
}
