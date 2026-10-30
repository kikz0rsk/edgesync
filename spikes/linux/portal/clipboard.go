package portal

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/godbus/dbus/v5"
)

const (
	interfaceClipboard = "org.freedesktop.portal.Clipboard"

	methodClipRequest            = interfaceClipboard + ".RequestClipboard"
	methodClipSetSelection       = interfaceClipboard + ".SetSelection"
	methodClipSelectionWrite     = interfaceClipboard + ".SelectionWrite"
	methodClipSelectionWriteDone = interfaceClipboard + ".SelectionWriteDone"
	methodClipSelectionRead      = interfaceClipboard + ".SelectionRead"

	signalSelectionOwnerChanged = "SelectionOwnerChanged"
	signalSelectionTransfer     = "SelectionTransfer"

	optionMimeTypes      = "mime_types"
	optionSessionIsOwner = "session_is_owner"

	clipboardSessionVersion = 2
	maxSelectionBytes       = 64 << 20
)

var (
	ErrClipboardVersion    = errors.New("portal: clipboard needs interface version 2 of the session portal")
	ErrClipboardNotEnabled = errors.New("portal: clipboard access not granted (clipboard_enabled absent or false)")
	ErrSelectionTooLarge   = errors.New("portal: selection exceeds the read limit")
)

// Clipboard drives org.freedesktop.portal.Clipboard, which attaches to a RemoteDesktop or InputCapture session
// (it creates no session of its own). None of its methods returns a Request object, so there is no handle_token and
// no Response: every call answers directly or through the SelectionOwnerChanged and SelectionTransfer signals.
type Clipboard struct {
	bus Bus
}

func NewClipboard(caller *Caller) *Clipboard {
	return &Clipboard{bus: caller.Bus()}
}

func (c *Clipboard) Version(ctx context.Context) (uint32, error) {
	return interfaceVersion(ctx, c.bus, interfaceClipboard)
}

// Request must run before the session starts; OpenWithClipboard places it at the only points the portal accepts.
func (c *Clipboard) Request(ctx context.Context, session dbus.ObjectPath) error {
	_, err := c.bus.Call(ctx, DesktopPath, methodClipRequest, session, Options{})
	return err
}

func (c *Clipboard) SetSelection(ctx context.Context, session dbus.ObjectPath, mimeTypes []string) error {
	_, err := c.bus.Call(ctx, DesktopPath, methodClipSetSelection, session, Options{
		optionMimeTypes: dbus.MakeVariant(mimeTypes),
	})
	return err
}

// Write returns the descriptor the caller writes the content for serial into; the caller closes it, then reports
// WriteDone.
func (c *Clipboard) Write(ctx context.Context, session dbus.ObjectPath, serial uint32) (*os.File, error) {
	return c.bus.CallFD(ctx, DesktopPath, methodClipSelectionWrite, session, serial)
}

func (c *Clipboard) WriteDone(ctx context.Context, session dbus.ObjectPath, serial uint32, success bool) error {
	_, err := c.bus.Call(ctx, DesktopPath, methodClipSelectionWriteDone, session, serial, success)
	return err
}

func (c *Clipboard) Read(ctx context.Context, session dbus.ObjectPath, mime string) (*os.File, error) {
	return c.bus.CallFD(ctx, DesktopPath, methodClipSelectionRead, session, mime)
}

// ReadAll blocks until the writer closes its end of the descriptor returned by SelectionRead.
func (c *Clipboard) ReadAll(ctx context.Context, session dbus.ObjectPath, mime string) ([]byte, error) {
	f, err := c.Read(ctx, session, mime)
	if err != nil {
		return nil, err
	}
	data, readErr := io.ReadAll(io.LimitReader(f, maxSelectionBytes+1))
	if err := errors.Join(readErr, f.Close()); err != nil {
		return nil, fmt.Errorf("portal: read %s: %w", mime, err)
	}
	if len(data) > maxSelectionBytes {
		return nil, fmt.Errorf("%w: %s", ErrSelectionTooLarge, mime)
	}
	return data, nil
}

type SelectionOwnerChangedEvent struct {
	MimeTypes         []string
	HasMimeTypes      bool
	SessionIsOwner    bool
	HasSessionIsOwner bool
}

type SelectionTransferEvent struct {
	MimeType string
	Serial   uint32
}

func (SelectionOwnerChangedEvent) isEvent() {}
func (SelectionTransferEvent) isEvent()     {}

// Events subscribes to the clipboard signals of the session plus Session.Closed.
func (c *Clipboard) Events(ctx context.Context, session dbus.ObjectPath) (*EventStream, error) {
	return newEventStream(ctx, c.bus, eventSource{
		match:  SignalMatch{Path: DesktopPath, Interface: interfaceClipboard},
		decode: decodeClipboardSignal(session),
	}, closedSource(session))
}

func decodeClipboardSignal(session dbus.ObjectPath) func(Signal) (Event, bool, error) {
	return func(sig Signal) (Event, bool, error) {
		_, member := splitSignalName(sig.Name)
		switch member {
		case signalSelectionOwnerChanged:
			return decodeOwnerChanged(sig.Body, session)
		case signalSelectionTransfer:
			return decodeTransfer(sig.Body, session)
		default:
			return nil, false, nil
		}
	}
}

func decodeOwnerChanged(body []any, session dbus.ObjectPath) (Event, bool, error) {
	if len(body) != 2 {
		return nil, false, fmt.Errorf("%w: SelectionOwnerChanged carries %d values, want 2", ErrBadResponse, len(body))
	}
	target, ok := body[0].(dbus.ObjectPath)
	if !ok {
		return nil, false, fmt.Errorf("%w: session handle is %T", ErrBadResponse, body[0])
	}
	if target != session {
		return nil, false, nil
	}
	opts, ok := body[1].(map[string]dbus.Variant)
	if !ok {
		return nil, false, fmt.Errorf("%w: options are %T", ErrBadResponse, body[1])
	}
	r := Results(opts)
	ev := SelectionOwnerChangedEvent{}
	if r.Has(optionMimeTypes) {
		mimes, err := r.Strings(optionMimeTypes)
		if err != nil {
			return nil, false, fmt.Errorf("selection owner changed: %w", err)
		}
		ev.MimeTypes, ev.HasMimeTypes = mimes, true
	}
	if r.Has(optionSessionIsOwner) {
		owner, err := r.Bool(optionSessionIsOwner)
		if err != nil {
			return nil, false, fmt.Errorf("selection owner changed: %w", err)
		}
		ev.SessionIsOwner, ev.HasSessionIsOwner = owner, true
	}
	return ev, true, nil
}

func decodeTransfer(body []any, session dbus.ObjectPath) (Event, bool, error) {
	if len(body) != 3 {
		return nil, false, fmt.Errorf("%w: SelectionTransfer carries %d values, want 3", ErrBadResponse, len(body))
	}
	target, ok := body[0].(dbus.ObjectPath)
	if !ok {
		return nil, false, fmt.Errorf("%w: session handle is %T", ErrBadResponse, body[0])
	}
	if target != session {
		return nil, false, nil
	}
	mime, ok := body[1].(string)
	if !ok {
		return nil, false, fmt.Errorf("%w: mime type is %T", ErrBadResponse, body[1])
	}
	serial, ok := body[2].(uint32)
	if !ok {
		return nil, false, fmt.Errorf("%w: serial is %T", ErrBadResponse, body[2])
	}
	return SelectionTransferEvent{MimeType: mime, Serial: serial}, true, nil
}

// ServeTransfer answers one SelectionTransfer. The write descriptor is requested first, so a refused request is
// reported with success false. Otherwise the content for the requested MIME type is written after delay, the
// descriptor is closed and the outcome goes back through SelectionWriteDone. The report uses an uncancelled context so
// the portal learns the outcome even when the run is shutting down.
func (c *Clipboard) ServeTransfer(ctx context.Context, session dbus.ObjectPath, ev SelectionTransferEvent, delay time.Duration, content func(mime string) ([]byte, error)) error {
	report := context.WithoutCancel(ctx)
	f, err := c.Write(ctx, session, ev.Serial)
	if err != nil {
		return errors.Join(fmt.Errorf("portal: selection write for serial %d: %w", ev.Serial, err),
			c.WriteDone(report, session, ev.Serial, false))
	}
	writeErr := writeSelection(ctx, f, ev.MimeType, delay, content)
	return errors.Join(writeErr, c.WriteDone(report, session, ev.Serial, writeErr == nil))
}

func writeSelection(ctx context.Context, f *os.File, mime string, delay time.Duration, content func(string) ([]byte, error)) error {
	err := writeContent(ctx, f, mime, delay, content)
	return errors.Join(err, f.Close())
}

func writeContent(ctx context.Context, f *os.File, mime string, delay time.Duration, content func(string) ([]byte, error)) error {
	if err := sleepContext(ctx, delay); err != nil {
		return err
	}
	data, err := content(mime)
	if err != nil {
		return fmt.Errorf("portal: content for %s: %w", mime, err)
	}
	if _, err := f.Write(data); err != nil {
		return fmt.Errorf("portal: write %s: %w", mime, err)
	}
	return nil
}

func sleepContext(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return context.Cause(ctx)
	}
}

func (r Results) Strings(key string) ([]string, error) {
	return typedResult[[]string](r, key, "array of strings")
}

// OpenWithClipboard runs Open with RequestClipboard between SelectDevices and Start, the only point the portal
// accepts it. Clipboard integration needs interface version 2, which is also the version that reports
// clipboard_enabled in the Start response.
func (rd *RemoteDesktop) OpenWithClipboard(ctx context.Context, p RemoteParams) (*RemoteSession, error) {
	if p.Types == 0 {
		return nil, ErrNoDevices
	}
	version, err := rd.Version(ctx)
	if err != nil {
		return nil, err
	}
	if version < clipboardSessionVersion {
		return nil, fmt.Errorf("%w: RemoteDesktop version %d", ErrClipboardVersion, version)
	}
	sessionToken, err := rd.caller.NewToken()
	if err != nil {
		return nil, err
	}
	created, err := rd.caller.Request(ctx, methodRDCreateSession,
		Options{optionSessionToken: dbus.MakeVariant(sessionToken)},
		func(o Options) []any { return []any{o} })
	if err != nil {
		return nil, err
	}
	path, err := created.Results.PathOrString("session_handle")
	if err != nil {
		return nil, fmt.Errorf("create session: %w", err)
	}
	rs := &RemoteSession{Session: newSession(rd.caller.Bus(), path), caller: rd.caller, Version: version}
	fail := func(err error) (*RemoteSession, error) {
		return nil, errors.Join(err, rs.Close(ctx))
	}
	if err := rs.selectDevices(ctx, p, version); err != nil {
		return fail(err)
	}
	if err := NewClipboard(rd.caller).Request(ctx, path); err != nil {
		return fail(fmt.Errorf("request clipboard: %w", err))
	}
	if err := rs.start(ctx); err != nil {
		return fail(err)
	}
	if rs.Devices == 0 {
		return fail(ErrNoDevices)
	}
	if !rs.ClipboardEnabled {
		return fail(ErrClipboardNotEnabled)
	}
	return rs, nil
}

// OpenWithClipboard runs CreateSession2, RequestClipboard and Start. Version 1 is not an option: its CreateSession
// starts the session immediately, so no clipboard request can precede Start.
func (ic *InputCapture) OpenWithClipboard(ctx context.Context, p CaptureParams) (*CaptureSession, error) {
	if p.Capabilities == 0 {
		return nil, ErrNoCapabilities
	}
	version, err := ic.Version(ctx)
	if err != nil {
		return nil, err
	}
	if version < clipboardSessionVersion {
		return nil, fmt.Errorf("%w: InputCapture version %d", ErrClipboardVersion, version)
	}
	sessionToken, err := ic.caller.NewToken()
	if err != nil {
		return nil, err
	}
	ret, err := ic.caller.Bus().Call(ctx, DesktopPath, methodICCreateSession2,
		Options{optionSessionToken: dbus.MakeVariant(sessionToken)})
	if err != nil {
		return nil, err
	}
	if len(ret) != 1 {
		return nil, fmt.Errorf("%w: CreateSession2 returned %d values, want 1", ErrUnexpectedReturn, len(ret))
	}
	created, ok := ret[0].(map[string]dbus.Variant)
	if !ok {
		return nil, fmt.Errorf("%w: CreateSession2 returned %T, want a{sv}", ErrUnexpectedReturn, ret[0])
	}
	path, err := Results(created).ObjectPath("session_handle")
	if err != nil {
		return nil, fmt.Errorf("create session 2: %w", err)
	}
	cs := ic.newSession(path)
	cs.Version = version
	fail := func(err error) (*CaptureSession, error) {
		return nil, errors.Join(err, cs.Close(ctx))
	}
	if err := NewClipboard(ic.caller).Request(ctx, path); err != nil {
		return fail(fmt.Errorf("request clipboard: %w", err))
	}
	opts := Options{
		"capabilities": dbus.MakeVariant(uint32(p.Capabilities)),
		"persist_mode": dbus.MakeVariant(uint32(p.PersistMode)),
	}
	if p.RestoreToken != "" {
		opts["restore_token"] = dbus.MakeVariant(p.RestoreToken)
	}
	resp, err := ic.caller.Request(ctx, methodICStart, opts, func(o Options) []any {
		return []any{path, "", o}
	})
	if err != nil {
		return fail(err)
	}
	if err := cs.readStartResults(resp.Results); err != nil {
		return fail(fmt.Errorf("start: %w", err))
	}
	if !cs.ClipboardEnabled {
		return fail(ErrClipboardNotEnabled)
	}
	if cs.Granted == 0 {
		return fail(ErrNoCapabilities)
	}
	return cs, nil
}
