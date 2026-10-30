package ui

import (
	"errors"
	"fmt"
	"time"

	"github.com/ebitengine/purego/objc"

	"github.com/kikz0r/edgesync/spikes/macos/fw"
)

// Apple's pages name these AppKit constants without stating their values (second-source: SDK headers as transcribed
// by progrium/macdriver enumtypes.gen.go (MIT); the collection behaviour bits and the backing store also match
// servo/core-foundation-rs cocoa appkit.rs). NSWindowStyleMaskNonactivatingPanel is 128 only in the macdriver table.
// NSTextAlignmentCenter is 2 on macOS (0 left, 1 right), unlike iOS.
const (
	styleMaskBorderless         = 0
	styleMaskNonactivatingPanel = 1 << 7
	backingStoreBuffered        = 2
	floatingWindowLevel         = 3

	collectionCanJoinAllSpaces    = 1 << 0
	collectionStationary          = 1 << 4
	collectionIgnoresCycle        = 1 << 6
	collectionFullScreenAuxiliary = 1 << 8

	textAlignmentCenter = 2
)

const (
	overlayWidth      = 360.0
	overlayHeight     = 96.0
	overlayLabelH     = 40.0
	overlayFontSize   = 28.0
	overlayBackground = 0.08
	overlayOpacity    = 0.85
)

type overlaySelectors struct {
	initPanel, setHidesOnDeactivate, setLevel, setCollectionBehavior, setIgnoresMouseEvents objc.SEL
	setOpaque, setBackgroundColor, setHasShadow, setReleasedWhenClosed                      objc.SEL
	contentView, addSubview, center, orderFrontRegardless, orderOut, isVisible              objc.SEL
	srgbColor, whiteColor, systemFont                                                       objc.SEL
	initWithFrame, setBezeled, setDrawsBackground, setEditable, setSelectable               objc.SEL
	setStringValue, setAlignment, setTextColor, setFont                                     objc.SEL
}

func newOverlaySelectors() overlaySelectors {
	return overlaySelectors{
		initPanel:             fw.Sel("initWithContentRect:styleMask:backing:defer:"),
		setHidesOnDeactivate:  fw.Sel("setHidesOnDeactivate:"),
		setLevel:              fw.Sel("setLevel:"),
		setCollectionBehavior: fw.Sel("setCollectionBehavior:"),
		setIgnoresMouseEvents: fw.Sel("setIgnoresMouseEvents:"),
		setOpaque:             fw.Sel("setOpaque:"),
		setBackgroundColor:    fw.Sel("setBackgroundColor:"),
		setHasShadow:          fw.Sel("setHasShadow:"),
		setReleasedWhenClosed: fw.Sel("setReleasedWhenClosed:"),
		contentView:           fw.Sel("contentView"),
		addSubview:            fw.Sel("addSubview:"),
		center:                fw.Sel("center"),
		orderFrontRegardless:  fw.Sel("orderFrontRegardless"),
		orderOut:              fw.Sel("orderOut:"),
		isVisible:             fw.Sel("isVisible"),
		srgbColor:             fw.Sel("colorWithSRGBRed:green:blue:alpha:"),
		whiteColor:            fw.Sel("whiteColor"),
		systemFont:            fw.Sel("systemFontOfSize:"),
		initWithFrame:         fw.Sel("initWithFrame:"),
		setBezeled:            fw.Sel("setBezeled:"),
		setDrawsBackground:    fw.Sel("setDrawsBackground:"),
		setEditable:           fw.Sel("setEditable:"),
		setSelectable:         fw.Sel("setSelectable:"),
		setStringValue:        fw.Sel("setStringValue:"),
		setAlignment:          fw.Sel("setAlignment:"),
		setTextColor:          fw.Sel("setTextColor:"),
		setFont:               fw.Sel("setFont:"),
	}
}

// Overlay is a borderless, non-activating, click-through NSPanel that floats above other windows on every space,
// including full-screen ones, and shows one line of text for a limited time.
type Overlay struct {
	app   *App
	sel   overlaySelectors
	panel objc.ID
	label objc.ID

	// generation and timer are only touched on the main thread.
	generation uint64
	timer      *time.Timer
}

// NewOverlay must run on the main thread.
func NewOverlay(app *App) (*Overlay, error) {
	if err := app.requireMainThread(); err != nil {
		return nil, err
	}
	o := &Overlay{app: app, sel: newOverlaySelectors()}
	panelClass, err := fw.Class("NSPanel")
	if err != nil {
		return nil, err
	}
	textFieldClass, err := fw.Class("NSTextField")
	if err != nil {
		return nil, err
	}
	colorClass, err := fw.Class("NSColor")
	if err != nil {
		return nil, err
	}
	fontClass, err := fw.Class("NSFont")
	if err != nil {
		return nil, err
	}

	err = app.inPool(func() error {
		frame := fw.CGRect{Size: fw.CGSize{Width: overlayWidth, Height: overlayHeight}}
		o.panel = objc.ID(panelClass).Send(app.base.alloc).Send(o.sel.initPanel,
			frame, uint(styleMaskBorderless|styleMaskNonactivatingPanel), uint(backingStoreBuffered), false)
		if o.panel == 0 {
			return fmt.Errorf("NSPanel initWithContentRect: %w", ErrNilObject)
		}
		// NSPanel hides on deactivate by default, and an accessory app is never active.
		o.panel.Send(o.sel.setHidesOnDeactivate, false)
		o.panel.Send(o.sel.setReleasedWhenClosed, false)
		o.panel.Send(o.sel.setLevel, int(floatingWindowLevel))
		o.panel.Send(o.sel.setCollectionBehavior,
			uint(collectionCanJoinAllSpaces|collectionFullScreenAuxiliary|collectionStationary|collectionIgnoresCycle))
		o.panel.Send(o.sel.setIgnoresMouseEvents, true)
		o.panel.Send(o.sel.setOpaque, false)
		o.panel.Send(o.sel.setHasShadow, false)
		background := objc.ID(colorClass).Send(o.sel.srgbColor,
			float64(overlayBackground), float64(overlayBackground), float64(overlayBackground), float64(overlayOpacity))
		o.panel.Send(o.sel.setBackgroundColor, background)

		labelFrame := fw.CGRect{
			Origin: fw.CGPoint{Y: (overlayHeight - overlayLabelH) / 2},
			Size:   fw.CGSize{Width: overlayWidth, Height: overlayLabelH},
		}
		o.label = objc.ID(textFieldClass).Send(app.base.alloc).Send(o.sel.initWithFrame, labelFrame)
		if o.label == 0 {
			return fmt.Errorf("NSTextField initWithFrame: %w", ErrNilObject)
		}
		o.label.Send(o.sel.setBezeled, false)
		o.label.Send(o.sel.setDrawsBackground, false)
		o.label.Send(o.sel.setEditable, false)
		o.label.Send(o.sel.setSelectable, false)
		o.label.Send(o.sel.setAlignment, int(textAlignmentCenter))
		o.label.Send(o.sel.setTextColor, objc.ID(colorClass).Send(o.sel.whiteColor))
		o.label.Send(o.sel.setFont, objc.ID(fontClass).Send(o.sel.systemFont, float64(overlayFontSize)))
		o.panel.Send(o.sel.contentView).Send(o.sel.addSubview, o.label)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return o, nil
}

// Show displays text for d, replacing a visible overlay. It is safe from any goroutine and returns once the work is
// queued for the main thread.
func (o *Overlay) Show(text string, d time.Duration) error {
	return o.app.Post(func() error { return o.present(text, d) })
}

func (o *Overlay) present(text string, d time.Duration) error {
	value, err := fw.NSString(text)
	if err != nil {
		return err
	}
	o.label.Send(o.sel.setStringValue, value)
	o.panel.Send(o.sel.center)
	o.panel.Send(o.sel.orderFrontRegardless)

	if o.timer != nil {
		o.timer.Stop()
	}
	o.generation++
	generation := o.generation
	o.timer = time.AfterFunc(d, func() {
		err := o.app.Post(func() error { return o.dismiss(generation) })
		if err != nil && !errors.Is(err, ErrStopped) {
			o.app.Fail(err)
		}
	})
	return nil
}

func (o *Overlay) dismiss(generation uint64) error {
	if generation != o.generation {
		return nil
	}
	o.panel.Send(o.sel.orderOut, objc.ID(0))
	return nil
}

// Visible reports NSWindow isVisible; it must run on the main thread.
func (o *Overlay) Visible() (bool, error) {
	if err := o.app.requireMainThread(); err != nil {
		return false, err
	}
	return objc.Send[bool](o.panel, o.sel.isVisible), nil
}

// Close hides and releases the panel; it must run on the main thread after Run has returned or before it starts.
func (o *Overlay) Close() error {
	if err := o.app.requireMainThread(); err != nil {
		return err
	}
	if o.timer != nil {
		o.timer.Stop()
	}
	o.generation++
	o.panel.Send(o.sel.orderOut, objc.ID(0))
	o.label.Send(o.app.base.release)
	o.panel.Send(o.app.base.release)
	o.panel, o.label = 0, 0
	return nil
}
