// Command wloverlay shows the switch-indicator candidates for GNOME (P0-SP3-007). GNOME's compositor has no
// layer-shell, so the candidates are a Notification portal banner (-mode notify), an override-redirect X11 window
// under XWayland (-mode x11) and an undecorated Gio window (-mode gio, built with cgo on the GNOME box). The program
// logs timestamps and the placement it can measure; stacking, focus and look are observed by the user.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"time"

	"github.com/godbus/dbus/v5"
	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"

	"github.com/kikz0r/edgesync/spikes/linux/portal"
)

const (
	interfaceNotification = "org.freedesktop.portal.Notification"
	methodAddNotification = interfaceNotification + ".AddNotification"
	methodRemove          = interfaceNotification + ".RemoveNotification"
	signalActionInvoked   = "ActionInvoked"
	propertyVersion       = "version"
	propertySupported     = "SupportedOptions"
	methodRegister        = "org.freedesktop.host.portal.Registry.Register"

	notificationVersionWithHints = 2
	fontName                     = "fixed"
	fixedFontCellWidth           = 6
	x11PanelHeight               = 44
	x11PanelTop                  = 48
)

var (
	ErrVersionType = errors.New("wloverlay: portal property has an unexpected type")
	ErrNeedsCgo    = errors.New("wloverlay: -mode gio needs a Linux build with cgo (see the README)")
)

type gioOptions struct {
	title    string
	text     string
	width    int
	height   int
	duration time.Duration
}

type notifyOptions struct {
	title    string
	body     string
	priority string
	hint     string
	appID    string
	count    int
	interval time.Duration
	sameID   bool
	duration time.Duration
}

type x11Options struct {
	display  string
	text     string
	x        int
	y        int
	duration time.Duration
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "wloverlay:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("wloverlay", flag.ContinueOnError)
	mode := fs.String("mode", "notify", "notify, x11 or gio")
	text := fs.String("text", "EdgeSync: Laptop", "indicator text (the machine name)")
	duration := fs.Duration("duration", 5*time.Second, "how long the indicator stays")
	priority := fs.String("priority", "normal", "notify: low, normal, high or urgent")
	hint := fs.String("hint", "", "notify: display-hint (transient, tray, persistent, hide-on-lockscreen, hide-content-on-lockscreen, show-as-new); needs portal version 2")
	appID := fs.String("app-id", "", "notify: register this application id with the portal first (needs a matching .desktop file)")
	count := fs.Int("count", 1, "notify: notifications to send")
	interval := fs.Duration("interval", time.Second, "notify: pause between notifications")
	sameID := fs.Bool("same-id", false, "notify: reuse one id so each notification replaces the previous one")
	display := fs.String("display", "", "x11: X display (default $DISPLAY)")
	x := fs.Int("x", -1, "x11: window x (-1 centres horizontally)")
	y := fs.Int("y", x11PanelTop, "x11: window y")
	width := fs.Int("w", 420, "gio: window width in dp")
	height := fs.Int("h", 96, "gio: window height in dp")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *duration <= 0 || *count < 1 || *width <= 0 || *height <= 0 {
		return errors.New("-duration, -count, -w and -h must be positive")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	log := slog.New(slog.NewTextHandler(os.Stdout, nil))

	switch *mode {
	case "notify":
		return runNotify(ctx, log, notifyOptions{
			title: *text, body: "control moved to this machine", priority: *priority, hint: *hint, appID: *appID,
			count: *count, interval: *interval, sameID: *sameID, duration: *duration,
		})
	case "x11":
		return runX11(ctx, log, x11Options{display: *display, text: *text, x: *x, y: *y, duration: *duration})
	case "gio":
		return runGio(ctx, log, gioOptions{title: "EdgeSync indicator", text: *text, width: *width, height: *height, duration: *duration})
	default:
		return fmt.Errorf("unknown -mode %q", *mode)
	}
}

func runNotify(ctx context.Context, log *slog.Logger, o notifyOptions) error {
	bus, err := portal.ConnectSessionBus()
	if err != nil {
		return err
	}
	defer bus.Close()

	if o.appID != "" {
		if _, err := bus.Call(ctx, portal.DesktopPath, methodRegister, o.appID, map[string]dbus.Variant{}); err != nil {
			return fmt.Errorf("register application id %q: %w", o.appID, err)
		}
		log.Info("registered with the portal", "app_id", o.appID)
	}
	version, err := notificationVersion(ctx, bus)
	if err != nil {
		return err
	}
	log.Info("Notification portal", "version", version)
	if version >= notificationVersionWithHints {
		supported, err := bus.Property(ctx, portal.DesktopPath, interfaceNotification, propertySupported)
		if err != nil {
			return fmt.Errorf("read SupportedOptions: %w", err)
		}
		log.Info("Notification portal SupportedOptions", "value", fmt.Sprint(supported.Value()))
	} else if o.hint != "" {
		return fmt.Errorf("-hint needs Notification portal version %d, the portal reports %d", notificationVersionWithHints, version)
	}

	watchCtx, stopWatching := context.WithCancel(ctx)
	var watchers sync.WaitGroup
	defer watchers.Wait()
	defer stopWatching()
	sub, err := bus.Subscribe(watchCtx, portal.SignalMatch{Path: portal.DesktopPath, Interface: interfaceNotification, Member: signalActionInvoked})
	if err != nil {
		return err
	}
	defer sub.Close()
	watchers.Go(func() { logActions(watchCtx, log, sub) })

	ids := make([]string, 0, o.count)
	for i := range o.count {
		id := notificationID(o, i)
		if !o.sameID || i == 0 {
			ids = append(ids, id)
		}
		started := time.Now()
		if _, err := bus.Call(ctx, portal.DesktopPath, methodAddNotification, id, notificationBody(o)); err != nil {
			return fmt.Errorf("AddNotification %s: %w", id, err)
		}
		log.Info("AddNotification returned", "id", id, "call_took", time.Since(started))
		if i < o.count-1 {
			if err := sleep(ctx, o.interval); err != nil {
				return err
			}
		}
	}
	waitErr := sleep(ctx, o.duration)
	var removeErrs []error
	for _, id := range ids {
		if _, err := bus.Call(context.WithoutCancel(ctx), portal.DesktopPath, methodRemove, id); err != nil {
			removeErrs = append(removeErrs, fmt.Errorf("RemoveNotification %s: %w", id, err))
			continue
		}
		log.Info("RemoveNotification returned", "id", id)
	}
	return errors.Join(append(removeErrs, ignoreCancel(ctx, waitErr))...)
}

func notificationVersion(ctx context.Context, bus portal.Bus) (uint32, error) {
	v, err := bus.Property(ctx, portal.DesktopPath, interfaceNotification, propertyVersion)
	if err != nil {
		return 0, fmt.Errorf("read the Notification portal version: %w", err)
	}
	version, ok := v.Value().(uint32)
	if !ok {
		return 0, fmt.Errorf("%w: version is %T", ErrVersionType, v.Value())
	}
	return version, nil
}

func notificationID(o notifyOptions, i int) string {
	if o.sameID {
		return "edgesync-indicator"
	}
	return fmt.Sprintf("edgesync-indicator-%d", i+1)
}

func notificationBody(o notifyOptions) map[string]dbus.Variant {
	body := map[string]dbus.Variant{
		"title":    dbus.MakeVariant(o.title),
		"body":     dbus.MakeVariant(o.body),
		"priority": dbus.MakeVariant(o.priority),
	}
	if o.hint != "" {
		body["display-hint"] = dbus.MakeVariant([]string{o.hint})
	}
	return body
}

func logActions(ctx context.Context, log *slog.Logger, sub portal.Subscription) {
	for {
		select {
		case <-ctx.Done():
			return
		case sig, open := <-sub.Signals():
			if !open {
				return
			}
			log.Info("ActionInvoked", "body", fmt.Sprint(sig.Body...))
		}
	}
}

func runX11(ctx context.Context, log *slog.Logger, o x11Options) error {
	x, err := xgb.NewConnDisplay(o.display)
	if err != nil {
		return fmt.Errorf("connect to the X display %q: %w", o.display, err)
	}
	screen := xproto.Setup(x).DefaultScreen(x)
	width := len(o.text)*fixedFontCellWidth + 2*fixedFontCellWidth*2
	left := o.x
	if left < 0 {
		left = (int(screen.WidthInPixels) - width) / 2
	}

	events := make(chan xgb.Event, 16)
	stopPump := make(chan struct{})
	pumpDone := make(chan struct{})
	go func() {
		defer close(pumpDone)
		defer close(events)
		for {
			ev, xerr := x.WaitForEvent()
			if ev == nil && xerr == nil {
				return
			}
			if xerr != nil {
				log.Error("X error event", "error", xerr)
				continue
			}
			select {
			case events <- ev:
			case <-stopPump:
				return
			}
		}
	}()
	err = showX11Panel(ctx, log, x, screen, o, left, width, events)
	close(stopPump)
	x.Close()
	<-pumpDone
	return err
}

func showX11Panel(ctx context.Context, log *slog.Logger, x *xgb.Conn, screen *xproto.ScreenInfo, o x11Options, left, width int, events <-chan xgb.Event) error {
	wid, err := newID(x)
	if err != nil {
		return err
	}
	window := xproto.Window(wid)
	mask := uint32(xproto.CwBackPixel | xproto.CwOverrideRedirect | xproto.CwEventMask)
	values := []uint32{screen.BlackPixel, 1, xproto.EventMaskExposure | xproto.EventMaskStructureNotify}
	created := time.Now()
	if err := xproto.CreateWindowChecked(x, screen.RootDepth, window, screen.Root, int16(left), int16(o.y), uint16(width), x11PanelHeight, 0, xproto.WindowClassInputOutput, screen.RootVisual, mask, values).Check(); err != nil {
		return fmt.Errorf("create the override-redirect window: %w", err)
	}
	gc, err := newTextGC(x, window, screen)
	if err != nil {
		return err
	}
	if err := xproto.MapWindowChecked(x, window).Check(); err != nil {
		return fmt.Errorf("map the window: %w", err)
	}
	log.Info("window mapped", "requested_x", left, "requested_y", o.y, "width", width, "height", x11PanelHeight, "took", time.Since(created))
	if err := logPlacement(log, x, window, screen.Root); err != nil {
		return err
	}

	timer := time.NewTimer(o.duration)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return ignoreCancel(ctx, ctx.Err())
		case <-timer.C:
			err := xproto.DestroyWindowChecked(x, window).Check()
			log.Info("panel destroyed", "after", time.Since(created))
			return err
		case ev, open := <-events:
			if !open {
				return errors.New("the X connection closed")
			}
			switch e := ev.(type) {
			case xproto.ExposeEvent:
				if err := drawText(x, window, gc, o.text); err != nil {
					return err
				}
				log.Info("Expose handled, text drawn", "since_create", time.Since(created), "count", e.Count)
			case xproto.ConfigureNotifyEvent:
				log.Info("ConfigureNotify", "x", e.X, "y", e.Y, "width", e.Width, "height", e.Height, "override_redirect", e.OverrideRedirect)
			}
		}
	}
}

func newID(x *xgb.Conn) (uint32, error) {
	id, err := x.NewId()
	if err != nil {
		return 0, fmt.Errorf("allocate an X id: %w", err)
	}
	return id, nil
}

func newTextGC(x *xgb.Conn, window xproto.Window, screen *xproto.ScreenInfo) (xproto.Gcontext, error) {
	fid, err := newID(x)
	if err != nil {
		return 0, err
	}
	if err := xproto.OpenFontChecked(x, xproto.Font(fid), uint16(len(fontName)), fontName).Check(); err != nil {
		return 0, fmt.Errorf("open the core font %q (install the X11 misc fonts): %w", fontName, err)
	}
	gid, err := newID(x)
	if err != nil {
		return 0, err
	}
	gc := xproto.Gcontext(gid)
	if err := xproto.CreateGCChecked(x, gc, xproto.Drawable(window), xproto.GcForeground|xproto.GcBackground|xproto.GcFont, []uint32{screen.WhitePixel, screen.BlackPixel, fid}).Check(); err != nil {
		return 0, fmt.Errorf("create the graphics context: %w", err)
	}
	return gc, nil
}

func drawText(x *xgb.Conn, window xproto.Window, gc xproto.Gcontext, text string) error {
	if err := xproto.ImageText8Checked(x, byte(len(text)), xproto.Drawable(window), gc, 2*fixedFontCellWidth, x11PanelHeight/2+4, text).Check(); err != nil {
		return fmt.Errorf("draw the indicator text: %w", err)
	}
	return nil
}

// logPlacement prints where the server put the window in root coordinates; under XWayland this is what the X client
// can know, the compositor may still stack or hide the window differently.
func logPlacement(log *slog.Logger, x *xgb.Conn, window, root xproto.Window) error {
	geometry, err := xproto.GetGeometry(x, xproto.Drawable(window)).Reply()
	if err != nil {
		return fmt.Errorf("read the window geometry: %w", err)
	}
	origin, err := xproto.TranslateCoordinates(x, window, root, 0, 0).Reply()
	if err != nil {
		return fmt.Errorf("translate the window origin: %w", err)
	}
	log.Info("window placement", "geometry_x", geometry.X, "geometry_y", geometry.Y, "width", geometry.Width, "height", geometry.Height, "root_x", origin.DstX, "root_y", origin.DstY)
	return nil
}

func sleep(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// ignoreCancel treats an interrupt as the normal end of a mode.
func ignoreCancel(ctx context.Context, err error) error {
	if ctx.Err() != nil && errors.Is(err, ctx.Err()) {
		return nil
	}
	return err
}
