package portal

import (
	"errors"
	"io"
	"os"
	"slices"
	"testing"

	"github.com/godbus/dbus/v5"
)

const testClipboardSession = dbus.ObjectPath("/org/freedesktop/portal/desktop/session/a/b")

// clipboardFake wraps the fake portal so Start reports clipboard_enabled as requested and the Clipboard methods
// are recorded on the fake bus. The SelectionWrite and SelectionRead descriptors come from fds.
type clipboardFDs struct {
	writeFile *os.File
	writeErr  error
	readFile  *os.File
	readErr   error
}

func clipboardFake(t *testing.T, version uint32, enabled bool) (*fakeBus, *clipboardFDs) {
	t.Helper()
	bus := newFakeBus(t)
	p := newFakePortal(t, bus, version)
	fds := &clipboardFDs{}
	base := bus.handler
	bus.handler = func(c fakeCall) ([]any, []Signal, error) {
		switch c.Method {
		case methodRDStart:
			return p.respond(optionsOf(t, c.Args[2]), 0, Options{
				"devices":           dbus.MakeVariant(uint32(DeviceKeyboard | DevicePointer)),
				"clipboard_enabled": dbus.MakeVariant(enabled),
				"restore_token":     dbus.MakeVariant("new-token"),
			})
		case methodICStart:
			return p.respond(optionsOf(t, c.Args[2]), 0, Options{
				"capabilities":      dbus.MakeVariant(uint32(CapabilityKeyboard | CapabilityPointer)),
				"clipboard_enabled": dbus.MakeVariant(enabled),
				"restore_token":     dbus.MakeVariant("new-token"),
			})
		case methodClipRequest, methodClipSetSelection, methodClipSelectionWriteDone:
			return nil, nil, nil
		case methodClipSelectionWrite:
			if fds.writeErr != nil {
				return nil, nil, fds.writeErr
			}
			return []any{fds.writeFile}, nil, nil
		case methodClipSelectionRead:
			if fds.readErr != nil {
				return nil, nil, fds.readErr
			}
			return []any{fds.readFile}, nil, nil
		default:
			return base(c)
		}
	}
	return bus, fds
}

func TestClipboard_RemoteDesktopOpenOrder(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	bus, _ := clipboardFake(t, 2, true)
	rd := NewRemoteDesktop(newCaller(bus))
	rs, err := rd.OpenWithClipboard(ctx, RemoteParams{Types: DeviceKeyboard | DevicePointer})
	if err != nil {
		t.Fatalf("OpenWithClipboard: %v", err)
	}
	want := []string{methodRDCreateSession, methodRDSelectDevices, methodClipRequest, methodRDStart}
	if got := bus.methods(); !slices.Equal(got, want) {
		t.Fatalf("calls = %v, want %v", got, want)
	}
	if !rs.ClipboardEnabled || rs.Devices != DeviceKeyboard|DevicePointer {
		t.Fatalf("session = %+v", rs)
	}
	if got := bus.callsTo(methodClipRequest)[0].Args[0]; got != rs.Path() {
		t.Fatalf("RequestClipboard session = %v, want %v", got, rs.Path())
	}
}

func TestClipboard_RemoteDesktopRejections(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	bus, _ := clipboardFake(t, 1, true)
	if _, err := NewRemoteDesktop(newCaller(bus)).OpenWithClipboard(ctx, RemoteParams{Types: DeviceKeyboard}); !errors.Is(err, ErrClipboardVersion) {
		t.Fatalf("version 1 = %v, want ErrClipboardVersion", err)
	}
	if len(bus.callsTo(methodRDCreateSession)) != 0 {
		t.Fatal("version 1 reached CreateSession")
	}

	bus, _ = clipboardFake(t, 2, false)
	if _, err := NewRemoteDesktop(newCaller(bus)).OpenWithClipboard(ctx, RemoteParams{Types: DeviceKeyboard}); !errors.Is(err, ErrClipboardNotEnabled) {
		t.Fatalf("disabled = %v, want ErrClipboardNotEnabled", err)
	}
	if len(bus.callsTo(methodSessionClose)) != 1 {
		t.Fatalf("disabled clipboard did not close the session: %v", bus.methods())
	}

	bus, _ = clipboardFake(t, 2, true)
	if _, err := NewRemoteDesktop(newCaller(bus)).OpenWithClipboard(ctx, RemoteParams{}); !errors.Is(err, ErrNoDevices) {
		t.Fatalf("no types = %v, want ErrNoDevices", err)
	}
}

func TestClipboard_InputCaptureOpenOrder(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	bus, _ := clipboardFake(t, 2, true)
	cs, err := NewInputCapture(newCaller(bus)).OpenWithClipboard(ctx, CaptureParams{
		Capabilities: CapabilityKeyboard | CapabilityPointer,
		PersistMode:  PersistUntilRevoked,
		RestoreToken: "old",
	})
	if err != nil {
		t.Fatalf("OpenWithClipboard: %v", err)
	}
	want := []string{methodICCreateSession2, methodClipRequest, methodICStart}
	if got := bus.methods(); !slices.Equal(got, want) {
		t.Fatalf("calls = %v, want %v", got, want)
	}
	if !cs.ClipboardEnabled || cs.Granted != CapabilityKeyboard|CapabilityPointer || cs.RestoreToken != "new-token" {
		t.Fatalf("session = %+v", cs)
	}
	startOpts := optionsOf(t, bus.callsTo(methodICStart)[0].Args[2])
	if uintOption(t, startOpts, "persist_mode") != 2 || stringOption(t, startOpts, "restore_token") != "old" {
		t.Fatalf("Start options = %v", startOpts)
	}
}

func TestClipboard_InputCaptureRejections(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	bus, _ := clipboardFake(t, 1, true)
	if _, err := NewInputCapture(newCaller(bus)).OpenWithClipboard(ctx, CaptureParams{Capabilities: CapabilityPointer}); !errors.Is(err, ErrClipboardVersion) {
		t.Fatalf("version 1 = %v, want ErrClipboardVersion", err)
	}
	if len(bus.callsTo(methodICCreateSession2)) != 0 {
		t.Fatal("version 1 reached CreateSession2")
	}

	bus, _ = clipboardFake(t, 2, false)
	if _, err := NewInputCapture(newCaller(bus)).OpenWithClipboard(ctx, CaptureParams{Capabilities: CapabilityPointer}); !errors.Is(err, ErrClipboardNotEnabled) {
		t.Fatalf("disabled = %v, want ErrClipboardNotEnabled", err)
	}
	if len(bus.callsTo(methodSessionClose)) != 1 {
		t.Fatalf("disabled clipboard did not close the session: %v", bus.methods())
	}
}

func TestClipboard_SetSelectionAndWriteDone(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	bus, _ := clipboardFake(t, 2, true)
	clip := NewClipboard(newCaller(bus))
	mimes := []string{"text/plain;charset=utf-8", "text/html", "image/png", "text/uri-list", "x-special/gnome-copied-files"}
	if err := clip.SetSelection(ctx, testClipboardSession, mimes); err != nil {
		t.Fatalf("SetSelection: %v", err)
	}
	set := bus.callsTo(methodClipSetSelection)[0]
	if set.Args[0] != testClipboardSession {
		t.Fatalf("SetSelection session = %v", set.Args[0])
	}
	if list, ok := optionsOf(t, set.Args[1])[optionMimeTypes].Value().([]string); !ok || !slices.Equal(list, mimes) {
		t.Fatalf("mime_types = %v", optionsOf(t, set.Args[1]))
	}
	if err := clip.WriteDone(ctx, testClipboardSession, 9, true); err != nil {
		t.Fatalf("WriteDone: %v", err)
	}
	done := bus.callsTo(methodClipSelectionWriteDone)[0]
	if done.Args[1] != uint32(9) || done.Args[2] != true {
		t.Fatalf("WriteDone args = %#v", done.Args)
	}
}

func TestClipboard_ServeTransferWritesAndReports(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	bus, fds := clipboardFake(t, 2, true)
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = r.Close() })
	fds.writeFile = w
	clip := NewClipboard(newCaller(bus))
	err = clip.ServeTransfer(ctx, testClipboardSession, SelectionTransferEvent{MimeType: "text/html", Serial: 4}, 0,
		func(mime string) ([]byte, error) {
			if mime != "text/html" {
				t.Errorf("content asked for %q", mime)
			}
			return []byte("<p>hi</p>"), nil
		})
	if err != nil {
		t.Fatalf("ServeTransfer: %v", err)
	}
	data, err := io.ReadAll(r)
	if err != nil || string(data) != "<p>hi</p>" {
		t.Fatalf("read %q, %v", data, err)
	}
	done := bus.callsTo(methodClipSelectionWriteDone)[0]
	if done.Args[1] != uint32(4) || done.Args[2] != true {
		t.Fatalf("WriteDone = %#v", done.Args)
	}
}

func TestClipboard_ServeTransferContentFailureReportsFalse(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	bus, fds := clipboardFake(t, 2, true)
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = r.Close() })
	fds.writeFile = w
	clip := NewClipboard(newCaller(bus))
	err = clip.ServeTransfer(ctx, testClipboardSession, SelectionTransferEvent{MimeType: "application/x", Serial: 5}, 0,
		func(string) ([]byte, error) { return nil, errors.New("no payload") })
	if err == nil {
		t.Fatal("ServeTransfer returned nil for failing content")
	}
	done := bus.callsTo(methodClipSelectionWriteDone)[0]
	if done.Args[1] != uint32(5) || done.Args[2] != false {
		t.Fatalf("WriteDone = %#v, want serial 5 and success false", done.Args)
	}
}

func TestClipboard_ServeTransferRefusedWriteReportsFalse(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	bus, fds := clipboardFake(t, 2, true)
	fds.writeErr = errors.New("refused")
	clip := NewClipboard(newCaller(bus))
	if err := clip.ServeTransfer(ctx, testClipboardSession, SelectionTransferEvent{MimeType: "text/plain", Serial: 6}, 0,
		func(string) ([]byte, error) { return nil, nil }); err == nil {
		t.Fatal("refused write returned nil")
	}
	done := bus.callsTo(methodClipSelectionWriteDone)[0]
	if done.Args[1] != uint32(6) || done.Args[2] != false {
		t.Fatalf("WriteDone = %#v, want serial 6 and success false", done.Args)
	}
}

func TestClipboard_ReadAllReturnsSelection(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	bus, fds := clipboardFake(t, 2, true)
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.WriteString("copy\nfile:///tmp/x\n"); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	fds.readFile = r
	data, err := NewClipboard(newCaller(bus)).ReadAll(ctx, testClipboardSession, "x-special/gnome-copied-files")
	if err != nil || string(data) != "copy\nfile:///tmp/x\n" {
		t.Fatalf("ReadAll = %q, %v", data, err)
	}
	if got := bus.callsTo(methodClipSelectionRead)[0].Args[1]; got != "x-special/gnome-copied-files" {
		t.Fatalf("SelectionRead mime = %v", got)
	}
}

func TestClipboard_EventsDecodeAndFilterBySession(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	bus, _ := clipboardFake(t, 2, true)
	other := dbus.ObjectPath("/org/freedesktop/portal/desktop/session/a/c")
	stream, err := NewClipboard(newCaller(bus)).Events(ctx, testClipboardSession)
	if err != nil {
		t.Fatalf("Events: %v", err)
	}
	t.Cleanup(func() { _ = stream.Close() })

	bus.emit(Signal{Path: DesktopPath, Name: interfaceClipboard + ".SelectionTransfer", Body: []any{other, "text/html", uint32(1)}})
	bus.emit(Signal{Path: DesktopPath, Name: interfaceClipboard + ".SelectionTransfer", Body: []any{testClipboardSession, "image/png", uint32(2)}})
	if tr, ok := nextEvent(t, stream).(SelectionTransferEvent); !ok || tr.MimeType != "image/png" || tr.Serial != 2 {
		t.Fatalf("transfer event = %#v, want the image/png transfer of this session only", tr)
	}

	bus.emit(Signal{Path: DesktopPath, Name: interfaceClipboard + ".SelectionOwnerChanged", Body: []any{testClipboardSession, map[string]dbus.Variant{
		"mime_types":       dbus.MakeVariant([]string{"text/html", "image/png"}),
		"session_is_owner": dbus.MakeVariant(false),
	}}})
	owner, ok := nextEvent(t, stream).(SelectionOwnerChangedEvent)
	if !ok || !slices.Equal(owner.MimeTypes, []string{"text/html", "image/png"}) || owner.SessionIsOwner || !owner.HasSessionIsOwner {
		t.Fatalf("owner event = %#v", owner)
	}

	bus.emit(Signal{Path: testClipboardSession, Name: interfaceSession + ".Closed", Body: []any{}})
	if _, ok := nextEvent(t, stream).(ClosedEvent); !ok {
		t.Fatal("Session.Closed not delivered")
	}
}

func TestClipboard_VersionProperty(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	bus, _ := clipboardFake(t, 2, true)
	bus.props[interfaceClipboard+".version"] = dbus.MakeVariant(uint32(1))
	v, err := NewClipboard(newCaller(bus)).Version(ctx)
	if err != nil || v != 1 {
		t.Fatalf("Version = %d, %v", v, err)
	}
}

func TestCopiedFilesPayload(t *testing.T) {
	tests := []struct {
		name  string
		op    string
		paths []string
		want  string
		err   error
	}{
		{name: "one file copy", op: "copy", paths: []string{"/home/u/a.txt"}, want: "copy\nfile:///home/u/a.txt"},
		{name: "several files copy", op: "copy", paths: []string{"/a/one.txt", "/a/two.txt"}, want: "copy\nfile:///a/one.txt\nfile:///a/two.txt"},
		{name: "one file cut", op: "cut", paths: []string{"/a/one.txt"}, want: "cut\nfile:///a/one.txt"},
		{name: "percent-encoded name", op: "copy", paths: []string{"/a/b c%d.txt"}, want: "copy\nfile:///a/b%20c%25d.txt"},
		{name: "empty list rejected", op: "copy", paths: nil, err: ErrNoPaths},
		{name: "unknown operation rejected", op: "move", paths: []string{"/a/one.txt"}, err: ErrBadCopyOp},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := CopiedFilesPayload(tt.op, tt.paths)
			if !errors.Is(err, tt.err) {
				t.Fatalf("CopiedFilesPayload error = %v, want %v", err, tt.err)
			}
			if string(got) != tt.want {
				t.Errorf("CopiedFilesPayload = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestURIListPayload(t *testing.T) {
	tests := []struct {
		name  string
		paths []string
		want  string
		err   error
	}{
		{name: "one file", paths: []string{"/a/one.txt"}, want: "file:///a/one.txt\r\n"},
		{name: "several files", paths: []string{"/a/one.txt", "/a/two.txt"}, want: "file:///a/one.txt\r\nfile:///a/two.txt\r\n"},
		{name: "percent-encoded name", paths: []string{"/a/b c%d.txt"}, want: "file:///a/b%20c%25d.txt\r\n"},
		{name: "empty list rejected", paths: nil, err: ErrNoPaths},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := URIListPayload(tt.paths)
			if !errors.Is(err, tt.err) {
				t.Fatalf("URIListPayload error = %v, want %v", err, tt.err)
			}
			if string(got) != tt.want {
				t.Errorf("URIListPayload = %q, want %q", got, tt.want)
			}
		})
	}
}
