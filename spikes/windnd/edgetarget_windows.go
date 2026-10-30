//go:build windows

package main

import (
	"encoding/binary"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/kikz0r/edgesync/spikes/internal/wincom"
)

const (
	wsPopup            uint32 = 0x80000000
	wsChild            uint32 = 0x40000000
	wsVisible          uint32 = 0x10000000
	wsOverlappedWindow uint32 = 0x00CF0000
	ssCenter           uint32 = 0x00000001

	wsExTopmost     uintptr = 0x00000008
	wsExTransparent uintptr = 0x00000020
	wsExToolWindow  uintptr = 0x00000080
	wsExLayered     uintptr = 0x00080000
	wsExNoActivate  uintptr = 0x08000000

	lwaAlpha                = 0x00000002
	layeredAlpha            = 1
	gwlExStyle              = ^uintptr(19)
	swpNoSize               = 0x0001
	swpNoMove               = 0x0002
	swpNoZOrder             = 0x0004
	swpNoActivate           = 0x0010
	swpFrameChanged         = 0x0020
	swpAsyncWindowPos       = 0x4000
	swShowNoActivate        = 4
	monitorDefaultToPrimary = 1
	stockBlackBrush         = 4
	idcArrow                = 32512

	wmMouseMove   = 0x0200
	wmLButtonDown = 0x0201
	wmLButtonUp   = 0x0202

	mkLButton uint32 = 0x0001
	mkRButton uint32 = 0x0002
	mkShift   uint32 = 0x0004
	mkControl uint32 = 0x0008
	mkMButton uint32 = 0x0010

	dropEffectScroll uint32 = 0x80000000

	cpACP                 = 0
	maxDescriptorEntries  = 100_000
	groupHeaderSize       = 4
	formatNameBufferUnits = 256
	firstRegisteredFormat = 0xC000
	edgeWindowClassName   = "EdgeSyncWindndEdge"
	edgeWindowTitle       = "EdgeSync windnd edge drop target"
)

const errNotRegistered = wincom.HResult(windows.DRAGDROP_E_NOTREGISTERED)

var (
	ErrDragSequence       = errors.New("drag notification out of order")
	ErrMalformedDropFiles = errors.New("malformed CF_HDROP data")
	ErrMalformedGroup     = errors.New("malformed FILEGROUPDESCRIPTORW data")
	ErrInvalidEdge        = errors.New("invalid edge geometry")
)

var (
	user32 = windows.NewLazySystemDLL("user32.dll")
	gdi32  = windows.NewLazySystemDLL("gdi32.dll")
	ole32  = windows.NewLazySystemDLL("ole32.dll")

	procRegisterClassEx            = user32.NewProc("RegisterClassExW")
	procUnregisterClass            = user32.NewProc("UnregisterClassW")
	procCreateWindowEx             = user32.NewProc("CreateWindowExW")
	procDestroyWindow              = user32.NewProc("DestroyWindow")
	procDefWindowProc              = user32.NewProc("DefWindowProcW")
	procSetLayeredWindowAttributes = user32.NewProc("SetLayeredWindowAttributes")
	procSetWindowLongPtr           = user32.NewProc("SetWindowLongPtrW")
	procSetWindowPos               = user32.NewProc("SetWindowPos")
	procShowWindow                 = user32.NewProc("ShowWindow")
	procMonitorFromPoint           = user32.NewProc("MonitorFromPoint")
	procGetMonitorInfo             = user32.NewProc("GetMonitorInfoW")
	procLoadCursor                 = user32.NewProc("LoadCursorW")
	procGetClipboardFormatName     = user32.NewProc("GetClipboardFormatNameW")
	procGetStockObject             = gdi32.NewProc("GetStockObject")
	procGetModuleHandle            = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetModuleHandleW")
	procRegisterDragDrop           = ole32.NewProc("RegisterDragDrop")
	procRevokeDragDrop             = ole32.NewProc("RevokeDragDrop")
)

// screenRect mirrors RECT and is half-open: Right and Bottom are the first pixel outside.
type screenRect struct {
	Left, Top, Right, Bottom int32
}

type monitorInfo struct {
	Size    uint32
	Monitor screenRect
	Work    screenRect
	Flags   uint32
}

type wndClassEx struct {
	Size       uint32
	Style      uint32
	WndProc    uintptr
	ClsExtra   int32
	WndExtra   int32
	Instance   uintptr
	Icon       uintptr
	Cursor     uintptr
	Background uintptr
	MenuName   *uint16
	ClassName  *uint16
	IconSm     uintptr
}

var (
	_ [unsafe.Sizeof(screenRect{}) - 16]struct{}
	_ [16 - unsafe.Sizeof(screenRect{})]struct{}
	_ [unsafe.Sizeof(monitorInfo{}) - 40]struct{}
	_ [40 - unsafe.Sizeof(monitorInfo{})]struct{}
	_ [unsafe.Sizeof(wndClassEx{}) - 80]struct{}
	_ [80 - unsafe.Sizeof(wndClassEx{})]struct{}
)

func (r screenRect) width() int32  { return r.Right - r.Left }
func (r screenRect) height() int32 { return r.Bottom - r.Top }

func (r screenRect) contains(p wincom.Point) bool {
	return p.X >= r.Left && p.X < r.Right && p.Y >= r.Top && p.Y < r.Bottom
}

func (r screenRect) local(p wincom.Point) wincom.Point {
	return wincom.Point{X: p.X - r.Left, Y: p.Y - r.Top}
}

// rightEdgeRect returns the strip of the given thickness along the right edge of monitor.
func rightEdgeRect(monitor screenRect, thickness int32) (screenRect, error) {
	switch {
	case monitor.width() <= 0 || monitor.height() <= 0:
		return screenRect{}, fmt.Errorf("%w: empty monitor rectangle %+v", ErrInvalidEdge, monitor)
	case thickness <= 0 || thickness > monitor.width():
		return screenRect{}, fmt.Errorf("%w: thickness %d for a monitor %d px wide", ErrInvalidEdge, thickness, monitor.width())
	}
	return screenRect{Left: monitor.Right - thickness, Top: monitor.Top, Right: monitor.Right, Bottom: monitor.Bottom}, nil
}

func win32Err(name string, callErr error) error {
	return fmt.Errorf("%s: %w", name, callErr)
}

func win32Bool(name string, ok uintptr, callErr error) error {
	if ok != 0 {
		return nil
	}
	return win32Err(name, callErr)
}

func hresultErr(name string, hr uintptr) error {
	if result := wincom.HResult(uint32(hr)); result.Failed() {
		return fmt.Errorf("%s: %w", name, result)
	}
	return nil
}

func primaryMonitorRect() (screenRect, error) {
	monitor, _, callErr := procMonitorFromPoint.Call(0, 0, monitorDefaultToPrimary)
	if monitor == 0 {
		return screenRect{}, win32Err("MonitorFromPoint", callErr)
	}
	info := monitorInfo{Size: uint32(unsafe.Sizeof(monitorInfo{}))}
	ok, _, callErr := procGetMonitorInfo.Call(monitor, uintptr(unsafe.Pointer(&info)))
	if err := win32Bool("GetMonitorInfoW", ok, callErr); err != nil {
		return screenRect{}, err
	}
	return info.Monitor, nil
}

type windowClass struct {
	name     *uint16
	instance uintptr
}

func registerWindowClass(name string, wndProc, background uintptr) (*windowClass, error) {
	namePtr, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return nil, fmt.Errorf("window class name %q: %w", name, err)
	}
	instance, _, callErr := procGetModuleHandle.Call(0)
	if instance == 0 {
		return nil, win32Err("GetModuleHandleW", callErr)
	}
	cursor, _, callErr := procLoadCursor.Call(0, idcArrow)
	if cursor == 0 {
		return nil, win32Err("LoadCursorW", callErr)
	}
	class := wndClassEx{WndProc: wndProc, Instance: instance, Cursor: cursor, Background: background, ClassName: namePtr}
	class.Size = uint32(unsafe.Sizeof(class))
	atom, _, callErr := procRegisterClassEx.Call(uintptr(unsafe.Pointer(&class)))
	if atom == 0 {
		return nil, win32Err("RegisterClassExW", callErr)
	}
	return &windowClass{name: namePtr, instance: instance}, nil
}

func (c *windowClass) createWindow(exStyle uintptr, style uint32, title string, bounds screenRect, parent uintptr) (uintptr, error) {
	titlePtr, err := windows.UTF16PtrFromString(title)
	if err != nil {
		return 0, fmt.Errorf("window title %q: %w", title, err)
	}
	hwnd, _, callErr := procCreateWindowEx.Call(
		exStyle, uintptr(unsafe.Pointer(c.name)), uintptr(unsafe.Pointer(titlePtr)), uintptr(style),
		uintptr(bounds.Left), uintptr(bounds.Top), uintptr(bounds.width()), uintptr(bounds.height()),
		parent, 0, c.instance, 0)
	if hwnd == 0 {
		return 0, win32Err("CreateWindowExW", callErr)
	}
	return hwnd, nil
}

func (c *windowClass) unregister() error {
	ok, _, callErr := procUnregisterClass.Call(uintptr(unsafe.Pointer(c.name)), c.instance)
	return win32Bool("UnregisterClassW", ok, callErr)
}

func destroyWindow(hwnd uintptr) error {
	ok, _, callErr := procDestroyWindow.Call(hwnd)
	return win32Bool("DestroyWindow", ok, callErr)
}

type flagName struct {
	bit  uint32
	name string
}

func joinFlags(value uint32, names []flagName) string {
	var parts []string
	rest := value
	for _, flag := range names {
		if value&flag.bit != 0 {
			parts = append(parts, flag.name)
			rest &^= flag.bit
		}
	}
	if rest != 0 {
		parts = append(parts, fmt.Sprintf("0x%X", rest))
	}
	if len(parts) == 0 {
		return "NONE"
	}
	return strings.Join(parts, "|")
}

func effectName(effect uint32) string {
	return joinFlags(effect, []flagName{
		{wincom.DropEffectCopy, "COPY"},
		{wincom.DropEffectMove, "MOVE"},
		{wincom.DropEffectLink, "LINK"},
		{dropEffectScroll, "SCROLL"},
	})
}

func keyStateName(keyState uint32) string {
	return joinFlags(keyState, []flagName{
		{mkLButton, "LBUTTON"}, {mkRButton, "RBUTTON"}, {mkMButton, "MBUTTON"}, {mkShift, "SHIFT"}, {mkControl, "CONTROL"},
	})
}

func tymedName(tymed uint32) string {
	return joinFlags(tymed, []flagName{
		{1, "HGLOBAL"}, {2, "FILE"}, {4, "ISTREAM"}, {8, "ISTORAGE"}, {16, "GDI"}, {32, "MFPICT"}, {64, "ENHMF"},
	})
}

func aspectName(aspect uint32) string {
	return joinFlags(aspect, []flagName{{1, "CONTENT"}, {2, "THUMBNAIL"}, {4, "ICON"}, {8, "DOCPRINT"}})
}

// effectForKeys maps modifier keys to the drop effect that the OLE documentation prescribes for DragEnter and
// DragOver: Ctrl+Shift is link, Ctrl is copy, no key or Shift is move.
func effectForKeys(keyState uint32) uint32 {
	switch {
	case keyState&mkControl != 0 && keyState&mkShift != 0:
		return wincom.DropEffectLink
	case keyState&mkControl != 0:
		return wincom.DropEffectCopy
	default:
		return wincom.DropEffectMove
	}
}

// negotiateEffect is the only effect this target ever returns. Move is never returned: the source would delete the
// original files although this spike copied nothing.
func negotiateEffect(allowed uint32) uint32 {
	if allowed&wincom.DropEffectCopy != 0 {
		return wincom.DropEffectCopy
	}
	return wincom.DropEffectNone
}

func formatName(id uint16) string {
	switch id {
	case 1:
		return "CF_TEXT"
	case 2:
		return "CF_BITMAP"
	case 7:
		return "CF_OEMTEXT"
	case 8:
		return "CF_DIB"
	case wincom.CFUnicodeText:
		return "CF_UNICODETEXT"
	case 14:
		return "CF_ENHMETAFILE"
	case wincom.CFHDrop:
		return "CF_HDROP"
	case 16:
		return "CF_LOCALE"
	case 17:
		return "CF_DIBV5"
	}
	if id < firstRegisteredFormat {
		return fmt.Sprintf("CF_%d", id)
	}
	buf := make([]uint16, formatNameBufferUnits)
	n, _, _ := procGetClipboardFormatName.Call(uintptr(id), uintptr(unsafe.Pointer(&buf[0])), formatNameBufferUnits)
	if n == 0 {
		return fmt.Sprintf("registered#%d", id)
	}
	return windows.UTF16ToString(buf[:n])
}

func describeFormat(f wincom.FormatEtc) string {
	return fmt.Sprintf("%s(tymed=%s,aspect=%s,lindex=%d)", formatName(f.Format), tymedName(f.Tymed), aspectName(f.Aspect), f.Index)
}

type mouseEvent struct {
	Message uint32
	Pt      wincom.Point
}

type gestureReport struct {
	CrossedEdge bool
	DragEntered bool
}

type observation struct {
	Transparent bool
	Changed     bool
	Finished    *gestureReport
}

// edgeObserver decides when the edge window must accept mouse input. The window is click-through while idle. A left
// button press outside the strip makes it hit-testable so a drag that starts there can enter it; the release restores
// click-through. A press inside the strip lands on the window underneath and leaves the state alone.
type edgeObserver struct {
	mu             sync.Mutex
	edge           screenRect
	transparent    bool
	pressedOutside bool
	crossed        bool
	entered        bool
	gestures       int
	missedEntries  int
}

func newEdgeObserver(edge screenRect) *edgeObserver {
	return &edgeObserver{edge: edge, transparent: true}
}

func (o *edgeObserver) observe(ev mouseEvent) observation {
	o.mu.Lock()
	defer o.mu.Unlock()
	before := o.transparent
	var report *gestureReport
	switch ev.Message {
	case wmLButtonDown:
		o.pressedOutside = !o.edge.contains(ev.Pt)
		o.crossed, o.entered = false, false
		if o.pressedOutside {
			o.transparent = false
		}
	case wmMouseMove:
		if o.pressedOutside && o.edge.contains(ev.Pt) {
			o.crossed = true
		}
	case wmLButtonUp:
		if o.pressedOutside {
			report = &gestureReport{CrossedEdge: o.crossed, DragEntered: o.entered}
			o.gestures++
			if o.crossed && !o.entered {
				o.missedEntries++
			}
		}
		o.pressedOutside = false
		o.transparent = true
	}
	return observation{Transparent: o.transparent, Changed: o.transparent != before, Finished: report}
}

func (o *edgeObserver) dragEntered() {
	o.mu.Lock()
	o.entered = true
	o.mu.Unlock()
}

func (o *edgeObserver) totals() (gestures, missedEntries int) {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.gestures, o.missedEntries
}

type sessionPhase int

const (
	phaseIdle sessionPhase = iota
	phaseInside
)

// dragSession validates the order of the IDropTarget notifications: DragEnter, any number of DragOver, then exactly
// one of DragLeave or Drop.
type dragSession struct {
	phase sessionPhase
}

func (s *dragSession) enter() error {
	if s.phase != phaseIdle {
		return fmt.Errorf("%w: DragEnter while a drag is already inside", ErrDragSequence)
	}
	s.phase = phaseInside
	return nil
}

func (s *dragSession) over() error {
	if s.phase != phaseInside {
		return fmt.Errorf("%w: DragOver without DragEnter", ErrDragSequence)
	}
	return nil
}

func (s *dragSession) finish(call string) error {
	if s.phase != phaseInside {
		return fmt.Errorf("%w: %s without DragEnter", ErrDragSequence, call)
	}
	s.phase = phaseIdle
	return nil
}

type descriptorInfo struct {
	Name       string
	Flags      uint32
	Attributes uint32
	Size       int64
	HasSize    bool
}

func parseDropFiles(raw []byte) ([]string, error) {
	headerSize := int(unsafe.Sizeof(wincom.DropFiles{}))
	if len(raw) < headerSize {
		return nil, fmt.Errorf("%w: %d bytes is shorter than the %d byte header", ErrMalformedDropFiles, len(raw), headerSize)
	}
	offset := int(binary.LittleEndian.Uint32(raw))
	wide := binary.LittleEndian.Uint32(raw[headerSize-4:]) != 0
	if offset < headerSize || offset > len(raw) {
		return nil, fmt.Errorf("%w: file list offset %d outside [%d, %d]", ErrMalformedDropFiles, offset, headerSize, len(raw))
	}
	list := raw[offset:]
	if wide {
		return parseWideList(list)
	}
	return parseNarrowList(list)
}

func parseWideList(list []byte) ([]string, error) {
	if len(list)%2 != 0 {
		return nil, fmt.Errorf("%w: wide list has an odd byte count %d", ErrMalformedDropFiles, len(list))
	}
	units := make([]uint16, len(list)/2)
	for i := range units {
		units[i] = binary.LittleEndian.Uint16(list[i*2:])
	}
	var files []string
	for start := 0; ; {
		if start >= len(units) {
			return nil, fmt.Errorf("%w: wide list is not double-NUL terminated", ErrMalformedDropFiles)
		}
		end := start
		for end < len(units) && units[end] != 0 {
			end++
		}
		if end == len(units) {
			return nil, fmt.Errorf("%w: wide list ends inside a name", ErrMalformedDropFiles)
		}
		if end == start {
			return files, nil
		}
		files = append(files, windows.UTF16ToString(units[start:end]))
		start = end + 1
	}
}

func parseNarrowList(list []byte) ([]string, error) {
	var files []string
	for start := 0; ; {
		if start >= len(list) {
			return nil, fmt.Errorf("%w: ANSI list is not double-NUL terminated", ErrMalformedDropFiles)
		}
		end := start
		for end < len(list) && list[end] != 0 {
			end++
		}
		if end == len(list) {
			return nil, fmt.Errorf("%w: ANSI list ends inside a name", ErrMalformedDropFiles)
		}
		if end == start {
			return files, nil
		}
		name, err := decodeANSI(list[start:end])
		if err != nil {
			return nil, err
		}
		files = append(files, name)
		start = end + 1
	}
}

func decodeANSI(raw []byte) (string, error) {
	length, err := windows.MultiByteToWideChar(cpACP, 0, &raw[0], int32(len(raw)), nil, 0)
	if err != nil {
		return "", fmt.Errorf("MultiByteToWideChar(size): %w", err)
	}
	units := make([]uint16, length)
	if _, err := windows.MultiByteToWideChar(cpACP, 0, &raw[0], int32(len(raw)), &units[0], length); err != nil {
		return "", fmt.Errorf("MultiByteToWideChar: %w", err)
	}
	return windows.UTF16ToString(units), nil
}

func parseFileGroupDescriptor(raw []byte) ([]descriptorInfo, error) {
	if len(raw) < groupHeaderSize {
		return nil, fmt.Errorf("%w: %d bytes is shorter than the item count", ErrMalformedGroup, len(raw))
	}
	count := binary.LittleEndian.Uint32(raw)
	descriptorSize := uint64(unsafe.Sizeof(wincom.FileDescriptorW{}))
	if count > maxDescriptorEntries {
		return nil, fmt.Errorf("%w: %d entries exceed the limit %d", ErrMalformedGroup, count, maxDescriptorEntries)
	}
	if need := groupHeaderSize + uint64(count)*descriptorSize; need > uint64(len(raw)) {
		return nil, fmt.Errorf("%w: %d entries need %d bytes, got %d", ErrMalformedGroup, count, need, len(raw))
	}
	infos := make([]descriptorInfo, 0, count)
	for i := range int(count) {
		var d wincom.FileDescriptorW
		start := groupHeaderSize + i*int(descriptorSize)
		copy(unsafe.Slice((*byte)(unsafe.Pointer(&d)), descriptorSize), raw[start:start+int(descriptorSize)])
		info := descriptorInfo{Name: windows.UTF16ToString(d.FileName[:]), Flags: d.Flags}
		if d.Flags&wincom.FDAttributes != 0 {
			info.Attributes = d.FileAttributes
		}
		if d.Flags&wincom.FDFileSize != 0 {
			info.HasSize = true
			info.Size = int64(d.FileSizeHigh)<<32 | int64(d.FileSizeLow)
		}
		infos = append(infos, info)
	}
	return infos, nil
}

type payload struct {
	Formats       []wincom.FormatEtc
	HasHDrop      bool
	Files         []string
	HasDescriptor bool
	Descriptors   []descriptorInfo
}

func requestHGlobal(dataObject uintptr, format uint16) ([]byte, error) {
	medium, err := wincom.GetData(dataObject, wincom.FormatEtc{
		Format: format, Aspect: wincom.DVAspectContent, Index: -1, Tymed: wincom.TymedHGlobal,
	})
	if err != nil {
		return nil, err
	}
	defer wincom.ReleaseMedium(&medium)
	return medium.HGlobalBytes()
}

// readPayload lists every format of a data object handed to DragEnter or Drop and decodes CF_HDROP and
// FileGroupDescriptorW when they are offered as HGLOBAL. A format that is absent is not an error; one that is offered
// but cannot be read is.
func readPayload(dataObject uintptr, descriptorFormat uint16) (payload, error) {
	formats, err := wincom.EnumFormats(dataObject)
	if err != nil {
		return payload{}, err
	}
	result := payload{Formats: formats}
	for _, f := range formats {
		if f.Tymed&wincom.TymedHGlobal == 0 {
			continue
		}
		switch {
		case f.Format == wincom.CFHDrop && !result.HasHDrop:
			raw, err := requestHGlobal(dataObject, f.Format)
			if err != nil {
				return result, fmt.Errorf("CF_HDROP: %w", err)
			}
			if result.Files, err = parseDropFiles(raw); err != nil {
				return result, err
			}
			result.HasHDrop = true
		case f.Format == descriptorFormat && !result.HasDescriptor:
			raw, err := requestHGlobal(dataObject, f.Format)
			if err != nil {
				return result, fmt.Errorf("FileGroupDescriptorW: %w", err)
			}
			if result.Descriptors, err = parseFileGroupDescriptor(raw); err != nil {
				return result, err
			}
			result.HasDescriptor = true
		}
	}
	return result, nil
}

func (p payload) logAttrs() []any {
	formats := make([]string, len(p.Formats))
	for i, f := range p.Formats {
		formats[i] = describeFormat(f)
	}
	return []any{"formats", formats, "has_hdrop", p.HasHDrop, "files", p.Files, "has_descriptor", p.HasDescriptor}
}

type overTrace struct {
	seen     bool
	keyState uint32
	allowed  uint32
}

func (t *overTrace) changed(keyState, allowed uint32) bool {
	if t.seen && t.keyState == keyState && t.allowed == allowed {
		return false
	}
	t.seen, t.keyState, t.allowed = true, keyState, allowed
	return true
}

type edgeTotals struct {
	Enters      int
	Overs       int
	Leaves      int
	Drops       int
	OverOutside int
	Failures    int
}

// edgeDropTarget implements wincom.DropTarget. Its methods run on the STA thread. The data object passed to DragEnter
// and Drop is read during the call only and never retained.
type edgeDropTarget struct {
	log              *slog.Logger
	edge             screenRect
	observer         *edgeObserver
	descriptorFormat uint16

	mu      sync.Mutex
	session dragSession
	trace   overTrace
	totals  edgeTotals
	overs   int
}

var _ wincom.DropTarget = (*edgeDropTarget)(nil)

func newEdgeDropTarget(log *slog.Logger, edge screenRect, observer *edgeObserver, descriptorFormat uint16) *edgeDropTarget {
	return &edgeDropTarget{log: log, edge: edge, observer: observer, descriptorFormat: descriptorFormat}
}

func (t *edgeDropTarget) Totals() edgeTotals {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.totals
}

func (t *edgeDropTarget) position(pt wincom.Point) []any {
	local := t.edge.local(pt)
	return []any{"x", pt.X, "y", pt.Y, "local_x", local.X, "local_y", local.Y, "inside_edge", t.edge.contains(pt)}
}

func (t *edgeDropTarget) fail(call string, err error) error {
	t.mu.Lock()
	t.totals.Failures++
	t.mu.Unlock()
	t.log.Error(call+" failed", "err", err)
	return err
}

func (t *edgeDropTarget) DragEnter(data uintptr, keyState uint32, pt wincom.Point, effect uint32) (uint32, error) {
	t.mu.Lock()
	err := t.session.enter()
	if err == nil {
		t.totals.Enters++
		t.overs = 0
		t.trace = overTrace{}
		t.trace.changed(keyState, effect)
	}
	t.mu.Unlock()
	if err != nil {
		return wincom.DropEffectNone, t.fail("DragEnter", err)
	}
	t.observer.dragEntered()

	result := negotiateEffect(effect)
	attrs := append(t.position(pt),
		"keys", keyStateName(keyState), "allowed", effectName(effect), "requested", effectName(effectForKeys(keyState)),
		"returned", effectName(result))
	content, err := readPayload(data, t.descriptorFormat)
	attrs = append(attrs, content.logAttrs()...)
	t.log.Info("DragEnter", attrs...)
	t.logDescriptors(content)
	if err != nil {
		return wincom.DropEffectNone, t.fail("DragEnter payload", err)
	}
	return result, nil
}

func (t *edgeDropTarget) DragOver(keyState uint32, pt wincom.Point, effect uint32) (uint32, error) {
	t.mu.Lock()
	err := t.session.over()
	var changed bool
	if err == nil {
		t.totals.Overs++
		t.overs++
		if !t.edge.contains(pt) {
			t.totals.OverOutside++
		}
		changed = t.trace.changed(keyState, effect)
	}
	t.mu.Unlock()
	if err != nil {
		return wincom.DropEffectNone, t.fail("DragOver", err)
	}
	result := negotiateEffect(effect)
	if changed {
		t.log.Info("DragOver state changed", append(t.position(pt),
			"keys", keyStateName(keyState), "allowed", effectName(effect), "returned", effectName(result))...)
	}
	return result, nil
}

func (t *edgeDropTarget) DragLeave() error {
	t.mu.Lock()
	err := t.session.finish("DragLeave")
	overs := t.overs
	if err == nil {
		t.totals.Leaves++
	}
	t.mu.Unlock()
	if err != nil {
		return t.fail("DragLeave", err)
	}
	t.log.Info("DragLeave", "dragover_calls", overs)
	return nil
}

func (t *edgeDropTarget) Drop(data uintptr, keyState uint32, pt wincom.Point, effect uint32) (uint32, error) {
	t.mu.Lock()
	err := t.session.finish("Drop")
	overs := t.overs
	if err == nil {
		t.totals.Drops++
	}
	t.mu.Unlock()
	if err != nil {
		return wincom.DropEffectNone, t.fail("Drop", err)
	}

	result := negotiateEffect(effect)
	attrs := append(t.position(pt),
		"keys", keyStateName(keyState), "allowed", effectName(effect), "returned", effectName(result), "dragover_calls", overs)
	content, err := readPayload(data, t.descriptorFormat)
	attrs = append(attrs, content.logAttrs()...)
	t.log.Info("Drop", attrs...)
	t.logDescriptors(content)
	if err != nil {
		return wincom.DropEffectNone, t.fail("Drop payload", err)
	}
	return result, nil
}

func (t *edgeDropTarget) logDescriptors(content payload) {
	for i, d := range content.Descriptors {
		t.log.Info("FileGroupDescriptorW entry",
			"index", i, "name", d.Name, "flags", fmt.Sprintf("0x%08X", d.Flags),
			"attributes", fmt.Sprintf("0x%X", d.Attributes), "has_size", d.HasSize, "size", d.Size)
	}
}

// edgeWindow is the 2 px topmost layered tool window registered as an OLE drop target. It must be created, and later
// closed, on the STA thread. Only setClickThrough may be called from another goroutine, and only from one at a time.
type edgeWindow struct {
	hwnd    uintptr
	class   *windowClass
	bounds  screenRect
	exStyle uintptr
}

func newEdgeWindow(bounds screenRect, target wincom.DropTarget) (window *edgeWindow, err error) {
	class, err := registerWindowClass(edgeWindowClassName, procDefWindowProc.Addr(), blackBrush())
	if err != nil {
		return nil, err
	}
	w := &edgeWindow{class: class, bounds: bounds, exStyle: wsExLayered | wsExTopmost | wsExToolWindow | wsExNoActivate | wsExTransparent}
	defer func() {
		if err != nil {
			err = errors.Join(err, w.close())
		}
	}()
	if w.hwnd, err = class.createWindow(w.exStyle, wsPopup, edgeWindowTitle, bounds, 0); err != nil {
		return nil, err
	}
	ok, _, callErr := procSetLayeredWindowAttributes.Call(w.hwnd, 0, layeredAlpha, lwaAlpha)
	if err = win32Bool("SetLayeredWindowAttributes", ok, callErr); err != nil {
		return nil, err
	}
	if err = w.register(target); err != nil {
		return nil, err
	}
	procShowWindow.Call(w.hwnd, swShowNoActivate)
	return w, nil
}

func blackBrush() uintptr {
	brush, _, _ := procGetStockObject.Call(stockBlackBrush)
	return brush
}

// register hands the drop target to OLE, which adds its own reference; the creation reference is released here.
func (w *edgeWindow) register(target wincom.DropTarget) error {
	dropTarget, err := wincom.NewDropTarget(target)
	if err != nil {
		return err
	}
	defer wincom.Release(dropTarget)
	hr, _, _ := procRegisterDragDrop.Call(w.hwnd, dropTarget)
	return hresultErr("RegisterDragDrop", hr)
}

func (w *edgeWindow) setClickThrough(on bool) error {
	next := w.exStyle &^ wsExTransparent
	if on {
		next |= wsExTransparent
	}
	if next == w.exStyle {
		return nil
	}
	previous, _, callErr := procSetWindowLongPtr.Call(w.hwnd, gwlExStyle, next)
	if previous == 0 {
		return win32Err("SetWindowLongPtrW(GWL_EXSTYLE)", callErr)
	}
	ok, _, callErr := procSetWindowPos.Call(w.hwnd, 0, 0, 0, 0, 0,
		swpNoMove|swpNoSize|swpNoZOrder|swpNoActivate|swpFrameChanged|swpAsyncWindowPos)
	if err := win32Bool("SetWindowPos", ok, callErr); err != nil {
		return err
	}
	w.exStyle = next
	return nil
}

func (w *edgeWindow) close() error {
	var errs []error
	if w.hwnd != 0 {
		hr, _, _ := procRevokeDragDrop.Call(w.hwnd)
		if result := wincom.HResult(uint32(hr)); result != wincom.SOK && result != errNotRegistered {
			errs = append(errs, hresultErr("RevokeDragDrop", hr))
		}
		errs = append(errs, destroyWindow(w.hwnd))
		w.hwnd = 0
	}
	errs = append(errs, w.class.unregister())
	return errors.Join(errs...)
}
