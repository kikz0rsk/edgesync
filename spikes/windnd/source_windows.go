//go:build windows

package main

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"path"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unicode"
	"unicode/utf8"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/kikz0r/edgesync/spikes/internal/wincom"
)

const (
	demoLargeFileSize = 200 << 20
	demoSmallFileSize = 10 << 10
	bytesPerMiB       = 1 << 20
	readChunk         = 64 << 10

	fileAttributeDirectory uint32 = 0x00000010
	fileAttributeNormal    uint32 = 0x00000080
	descriptorFlags               = wincom.FDAttributes | wincom.FDFileSize | wincom.FDWritesTime | wincom.FDProgressUI | wincom.FDUnicode
	maxPathUnits                  = 259
	forbiddenNameRunes            = `<>:"|?*\`

	filetimeTicksPerSecond  = 10_000_000
	unixToFiletimeEpochSecs = 11_644_473_600
	maxFiletimeSeconds      = math.MaxUint64/filetimeTicksPerSecond - 1

	wmDestroy = 0x0002

	colorWindowBrush   = 6
	sourceClassName    = "EdgeSyncWindndSource"
	sourceTitle        = "EdgeSync windnd drag source"
	sourceLabel        = "Press the left mouse button here and drag.\r\nThe payload is a virtual folder, a 200 MiB file and a 10 KiB file."
	sourceWindowLeft   = 200
	sourceWindowTop    = 200
	sourceWindowWidth  = 520
	sourceWindowHeight = 220
	sourceLabelMargin  = 20
)

var (
	ErrNotRepresentable = errors.New("entry is not representable as a Windows virtual file")
	ErrIndexOutOfRange  = errors.New("entry index out of range")
	ErrDirectory        = errors.New("entry is a directory and has no content")
	ErrNegativeOffset   = errors.New("negative offset")
	ErrInvalidWhence    = errors.New("invalid seek origin")
	ErrDragFailed       = errors.New("DoDragDrop failed")
	ErrSourceExists     = errors.New("a drag source window already exists in this process")
)

var procDoDragDrop = ole32.NewProc("DoDragDrop")

var (
	activeSource   atomic.Pointer[sourceWindow]
	sourceProcOnce = sync.OnceValue(func() uintptr { return syscall.NewCallback(sourceWndProc) })
)

type payloadEntry struct {
	Path    string
	Dir     bool
	Size    int64
	ModTime time.Time
}

func demoEntries(modTime time.Time) []payloadEntry {
	return []payloadEntry{
		{Path: "demo", Dir: true, ModTime: modTime},
		{Path: "demo/a.bin", Size: demoLargeFileSize, ModTime: modTime},
		{Path: "b.txt", Size: demoSmallFileSize, ModTime: modTime},
	}
}

func contentByte(offset int64) byte {
	return byte(offset*31 + 7)
}

// streamOptions configure every content stream: a zero rate means unthrottled, latency delays the first byte.
type streamOptions struct {
	rateBytesPerSec int64
	latency         time.Duration
}

// contentReader serves the deterministic synthetic content of one file lazily, at the configured rate.
type contentReader struct {
	size int64
	opts streamOptions

	pos       int64
	started   bool
	begin     time.Time
	delivered int64

	reads  atomic.Int64
	served atomic.Int64
}

func newContentReader(size int64, opts streamOptions) *contentReader {
	return &contentReader{size: size, opts: opts}
}

func (r *contentReader) Read(p []byte) (int, error) {
	if r.pos >= r.size {
		return 0, io.EOF
	}
	if !r.started {
		r.started = true
		time.Sleep(r.opts.latency)
		r.begin = time.Now()
	}
	n := int(min(int64(len(p)), r.size-r.pos, readChunk))
	for i := range n {
		p[i] = contentByte(r.pos + int64(i))
	}
	r.pos += int64(n)
	r.delivered += int64(n)
	r.reads.Add(1)
	r.served.Add(int64(n))
	if r.opts.rateBytesPerSec > 0 {
		due := r.begin.Add(time.Duration(r.delivered * int64(time.Second) / r.opts.rateBytesPerSec))
		time.Sleep(time.Until(due))
	}
	return n, nil
}

func (r *contentReader) Seek(offset int64, whence int) (int64, error) {
	var base int64
	switch whence {
	case io.SeekStart:
	case io.SeekCurrent:
		base = r.pos
	case io.SeekEnd:
		base = r.size
	default:
		return 0, fmt.Errorf("%w: %d", ErrInvalidWhence, whence)
	}
	target := base + offset
	if target < 0 {
		return 0, fmt.Errorf("%w: %d", ErrNegativeOffset, target)
	}
	r.pos = target
	return target, nil
}

// buildFileGroupDescriptor returns the bytes of a FILEGROUPDESCRIPTORW: a UINT item count followed by one 592-byte
// FILEDESCRIPTORW per entry.
func buildFileGroupDescriptor(entries []payloadEntry) ([]byte, error) {
	descriptorSize := int(unsafe.Sizeof(wincom.FileDescriptorW{}))
	out := make([]byte, groupHeaderSize, groupHeaderSize+len(entries)*descriptorSize)
	binary.LittleEndian.PutUint32(out, uint32(len(entries)))
	for i, e := range entries {
		descriptor, err := newFileDescriptor(e)
		if err != nil {
			return nil, fmt.Errorf("entry %d: %w", i, err)
		}
		out = append(out, unsafe.Slice((*byte)(unsafe.Pointer(&descriptor)), descriptorSize)...)
	}
	return out, nil
}

func newFileDescriptor(e payloadEntry) (wincom.FileDescriptorW, error) {
	var descriptor wincom.FileDescriptorW
	name, err := windowsName(e.Path)
	if err != nil {
		return descriptor, err
	}
	units, err := windows.UTF16FromString(name)
	if err != nil {
		return descriptor, fmt.Errorf("%w: %q: %w", ErrNotRepresentable, e.Path, err)
	}
	if len(units)-1 > maxPathUnits {
		return descriptor, fmt.Errorf("%w: %d UTF-16 units exceed %d", ErrNotRepresentable, len(units)-1, maxPathUnits)
	}
	writeTime, err := fileTime(e.ModTime)
	if err != nil {
		return descriptor, err
	}
	if e.Size < 0 || (e.Dir && e.Size != 0) {
		return descriptor, fmt.Errorf("%w: size %d for %q", ErrNotRepresentable, e.Size, e.Path)
	}
	descriptor.Flags = descriptorFlags
	descriptor.FileAttributes = fileAttributeNormal
	if e.Dir {
		descriptor.FileAttributes = fileAttributeDirectory
	}
	descriptor.LastWriteTime = writeTime
	descriptor.FileSizeHigh = uint32(e.Size >> 32)
	descriptor.FileSizeLow = uint32(e.Size)
	copy(descriptor.FileName[:], units)
	return descriptor, nil
}

// windowsName converts a slash-separated relative path to backslash form after applying the virtual-file name rules
// of D-99 (reserved and control characters, length) and the structural rules of D-45.
func windowsName(relative string) (string, error) {
	if relative == "" || !utf8.ValidString(relative) {
		return "", fmt.Errorf("%w: empty or invalid UTF-8 path", ErrNotRepresentable)
	}
	for _, component := range strings.Split(relative, "/") {
		if component == "" || component == "." || component == ".." {
			return "", fmt.Errorf("%w: %q has an empty, dot or dot-dot component", ErrNotRepresentable, relative)
		}
		if i := strings.IndexFunc(component, isForbiddenNameRune); i >= 0 {
			r, _ := utf8.DecodeRuneInString(component[i:])
			return "", fmt.Errorf("%w: %q contains reserved or control character %U", ErrNotRepresentable, relative, r)
		}
	}
	return strings.ReplaceAll(relative, "/", `\`), nil
}

func isForbiddenNameRune(r rune) bool {
	return unicode.IsControl(r) || strings.ContainsRune(forbiddenNameRunes, r)
}

func fileTime(t time.Time) (windows.Filetime, error) {
	seconds := t.Unix() + unixToFiletimeEpochSecs
	if seconds < 0 || uint64(seconds) > maxFiletimeSeconds {
		return windows.Filetime{}, fmt.Errorf("%w: modification time %v outside the FILETIME range", ErrNotRepresentable, t)
	}
	ticks := uint64(seconds)*filetimeTicksPerSecond + uint64(t.Nanosecond()/100)
	return windows.Filetime{LowDateTime: uint32(ticks), HighDateTime: uint32(ticks >> 32)}, nil
}

type shellFormats struct {
	Descriptor     uint16
	Contents       uint16
	Preferred      uint16
	Performed      uint16
	PasteSucceeded uint16
}

func registerShellFormats() (shellFormats, error) {
	var formats shellFormats
	for _, target := range []struct {
		name string
		id   *uint16
	}{
		{wincom.FormatNameFileGroupDescriptorW, &formats.Descriptor},
		{wincom.FormatNameFileContents, &formats.Contents},
		{wincom.FormatNamePreferredDropEffect, &formats.Preferred},
		{wincom.FormatNamePerformedDropEffect, &formats.Performed},
		{wincom.FormatNamePasteSucceeded, &formats.PasteSucceeded},
	} {
		id, err := wincom.RegisterFormat(target.name)
		if err != nil {
			return shellFormats{}, err
		}
		*target.id = id
	}
	return formats, nil
}

type streamRecord struct {
	Path      string
	Size      int64
	Served    int64
	Reads     int64
	Completed bool
}

type dropOutcomes struct {
	PerformedSet   bool
	Performed      uint32
	PasteSucceeded bool
	PasteEffect    uint32
}

// dragData is the wincom.DataSource behind the drag source's data object: FileGroupDescriptorW, one FileContents
// IStream per file and Preferred DropEffect, the same shape as the clipboard offer of the winvfiles spike.
type dragData struct {
	log        *slog.Logger
	formats    shellFormats
	entries    []payloadEntry
	descriptor []byte
	opts       streamOptions

	mu       sync.Mutex
	streams  []streamRecord
	outcomes dropOutcomes
}

var _ wincom.DataSource = (*dragData)(nil)

func newDragData(log *slog.Logger, formats shellFormats, entries []payloadEntry, opts streamOptions) (*dragData, error) {
	descriptor, err := buildFileGroupDescriptor(entries)
	if err != nil {
		return nil, fmt.Errorf("file group descriptor: %w", err)
	}
	return &dragData{log: log, formats: formats, entries: entries, descriptor: descriptor, opts: opts}, nil
}

func (d *dragData) Streams() []streamRecord {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]streamRecord(nil), d.streams...)
}

func (d *dragData) Outcomes() dropOutcomes {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.outcomes
}

func (d *dragData) Formats() []wincom.FormatEtc {
	return []wincom.FormatEtc{
		{Format: d.formats.Descriptor, Aspect: wincom.DVAspectContent, Index: -1, Tymed: wincom.TymedHGlobal},
		{Format: d.formats.Contents, Aspect: wincom.DVAspectContent, Index: -1, Tymed: wincom.TymedIStream},
		{Format: d.formats.Preferred, Aspect: wincom.DVAspectContent, Index: -1, Tymed: wincom.TymedHGlobal},
	}
}

func (d *dragData) logCall(call string, f wincom.FormatEtc) *slog.Logger {
	logger := d.log.With("call", call, "format", formatName(f.Format), "lindex", f.Index,
		"tymed", tymedName(f.Tymed), "aspect", aspectName(f.Aspect))
	logger.Info("IDataObject call")
	return logger
}

func (d *dragData) GetData(f wincom.FormatEtc) (wincom.Medium, error) {
	logger := d.logCall("GetData", f)
	switch f.Format {
	case d.formats.Descriptor:
		return wincom.NewHGlobalMedium(d.descriptor)
	case d.formats.Preferred:
		return wincom.NewHGlobalMedium(binary.LittleEndian.AppendUint32(nil, wincom.DropEffectCopy))
	case d.formats.Contents:
		return d.fileContents(f, logger)
	default:
		return wincom.Medium{}, wincom.DVEFormatEtc
	}
}

func (d *dragData) fileContents(f wincom.FormatEtc, logger *slog.Logger) (wincom.Medium, error) {
	if f.Index < 0 || int(f.Index) >= len(d.entries) {
		logger.Warn("FileContents refused", "reason", ErrIndexOutOfRange)
		return wincom.Medium{}, fmt.Errorf("lindex %d: %w: %w", f.Index, wincom.DVELindex, ErrIndexOutOfRange)
	}
	entry := d.entries[f.Index]
	if entry.Dir {
		logger.Warn("FileContents refused", "reason", ErrDirectory)
		return wincom.Medium{}, fmt.Errorf("lindex %d: %w: %w", f.Index, wincom.DVELindex, ErrDirectory)
	}
	reader := newContentReader(entry.Size, d.opts)
	stream, err := wincom.NewStream(reader, entry.Size, path.Base(entry.Path), func() {
		record := streamRecord{
			Path: entry.Path, Size: entry.Size, Served: reader.served.Load(), Reads: reader.reads.Load(),
			Completed: reader.served.Load() == entry.Size,
		}
		d.mu.Lock()
		d.streams = append(d.streams, record)
		d.mu.Unlock()
		logger.Info("stream released", "path", record.Path, "served", record.Served, "size", record.Size,
			"reads", record.Reads, "completed", record.Completed)
	})
	if err != nil {
		return wincom.Medium{}, fmt.Errorf("IStream for %q: %w", entry.Path, err)
	}
	logger.Info("stream created, nothing read yet", "path", entry.Path, "size", entry.Size)
	return wincom.NewStreamMedium(stream), nil
}

func (d *dragData) SetData(f wincom.FormatEtc, m wincom.Medium) error {
	logger := d.logCall("SetData", f)
	if f.Format != d.formats.Performed && f.Format != d.formats.PasteSucceeded {
		logger.Warn("SetData format not handled")
		return wincom.DVEFormatEtc
	}
	raw, err := m.HGlobalBytes()
	if err != nil {
		return fmt.Errorf("%s medium: %w", formatName(f.Format), err)
	}
	if len(raw) < 4 {
		return fmt.Errorf("%s medium holds %d bytes, want a DWORD: %w", formatName(f.Format), len(raw), wincom.EInvalidArg)
	}
	effect := binary.LittleEndian.Uint32(raw)
	d.mu.Lock()
	if f.Format == d.formats.Performed {
		d.outcomes.PerformedSet, d.outcomes.Performed = true, effect
	} else {
		d.outcomes.PasteSucceeded, d.outcomes.PasteEffect = true, effect
	}
	d.mu.Unlock()
	logger.Info("drop outcome reported", "effect", effectName(effect))
	return nil
}

// continueDecision is the standard IDropSource rule: Escape cancels, releasing the left button drops.
func continueDecision(escapePressed bool, keyState uint32) wincom.HResult {
	switch {
	case escapePressed:
		return wincom.DragDropSCancel
	case keyState&mkLButton == 0:
		return wincom.DragDropSDrop
	default:
		return wincom.SOK
	}
}

func decisionName(decision wincom.HResult) string {
	switch decision {
	case wincom.SOK:
		return "continue"
	case wincom.DragDropSDrop:
		return "drop"
	case wincom.DragDropSCancel:
		return "cancel"
	default:
		return decision.Error()
	}
}

// dragOutcome classifies the HRESULT of DoDragDrop. The effect is only meaningful when the drag was not cancelled.
func dragOutcome(result wincom.HResult, effect uint32) (string, error) {
	switch {
	case result == wincom.DragDropSDrop:
		return "dropped with effect " + effectName(effect), nil
	case result == wincom.DragDropSCancel:
		return "cancelled", nil
	case result == wincom.SOK:
		return "completed with effect " + effectName(effect), nil
	default:
		return "", fmt.Errorf("%w: %w", ErrDragFailed, result)
	}
}

type sourcePhase int

const (
	sourceIdle sourcePhase = iota
	sourceDragging
	sourceDropped
	sourceCancelled
)

func (p sourcePhase) String() string {
	switch p {
	case sourceIdle:
		return "idle"
	case sourceDragging:
		return "dragging"
	case sourceDropped:
		return "dropped"
	case sourceCancelled:
		return "cancelled"
	default:
		return fmt.Sprintf("phase#%d", int(p))
	}
}

type sessionSummary struct {
	Phase    sourcePhase
	Queries  int
	Feedback []string
}

// sourceSession is the wincom.DropSource of one drag and tracks its state: idle, dragging, then dropped or
// cancelled. Its methods run on the STA thread inside the DoDragDrop modal loop.
type sourceSession struct {
	log *slog.Logger

	mu          sync.Mutex
	phase       sourcePhase
	queries     int
	lastEffect  uint32
	hasFeedback bool
	feedback    []string
}

var _ wincom.DropSource = (*sourceSession)(nil)

func (s *sourceSession) begin() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.phase != sourceIdle {
		return fmt.Errorf("%w: a drag session cannot be started twice", ErrDragSequence)
	}
	s.phase = sourceDragging
	return nil
}

func (s *sourceSession) summary() sessionSummary {
	s.mu.Lock()
	defer s.mu.Unlock()
	return sessionSummary{Phase: s.phase, Queries: s.queries, Feedback: append([]string(nil), s.feedback...)}
}

func (s *sourceSession) QueryContinueDrag(escapePressed bool, keyState uint32) wincom.HResult {
	decision := continueDecision(escapePressed, keyState)
	s.mu.Lock()
	s.queries++
	switch {
	case decision == wincom.DragDropSDrop:
		s.phase = sourceDropped
	case decision == wincom.DragDropSCancel:
		s.phase = sourceCancelled
	}
	s.mu.Unlock()
	if decision != wincom.SOK {
		s.log.Info("QueryContinueDrag", "decision", decisionName(decision), "escape", escapePressed, "keys", keyStateName(keyState))
	}
	return decision
}

func (s *sourceSession) GiveFeedback(effect uint32) wincom.HResult {
	s.mu.Lock()
	changed := !s.hasFeedback || s.lastEffect != effect
	if changed {
		s.hasFeedback, s.lastEffect = true, effect
		s.feedback = append(s.feedback, effectName(effect))
	}
	s.mu.Unlock()
	if changed {
		s.log.Info("GiveFeedback effect changed", "effect", effectName(effect))
	}
	return wincom.DragDropSUseDefaultCur
}

type dragRecord struct {
	Result  string
	Failure error
	Session sessionSummary
	Data    *dragData
}

// sourceWindow is a small visible window that starts an OLE drag of the virtual-file payload when the left button goes
// down in it. It lives on the STA thread, whose message pump dispatches its messages.
type sourceWindow struct {
	log     *slog.Logger
	formats shellFormats
	opts    streamOptions

	hwnd      uintptr
	class     *windowClass
	dragging  atomic.Bool
	closed    chan struct{}
	closeOnce sync.Once

	mu      sync.Mutex
	history []dragRecord
}

func newSourceWindow(log *slog.Logger, formats shellFormats, opts streamOptions) (window *sourceWindow, err error) {
	class, err := registerWindowClass(sourceClassName, sourceProcOnce(), colorWindowBrush)
	if err != nil {
		return nil, err
	}
	w := &sourceWindow{log: log, formats: formats, opts: opts, class: class, closed: make(chan struct{})}
	defer func() {
		if err != nil {
			err = errors.Join(err, w.close())
		}
	}()
	bounds := screenRect{
		Left: sourceWindowLeft, Top: sourceWindowTop,
		Right: sourceWindowLeft + sourceWindowWidth, Bottom: sourceWindowTop + sourceWindowHeight,
	}
	if w.hwnd, err = class.createWindow(0, wsOverlappedWindow|wsVisible, sourceTitle, bounds, 0); err != nil {
		return nil, err
	}
	if err = w.addLabel(); err != nil {
		return nil, err
	}
	if !activeSource.CompareAndSwap(nil, w) {
		return nil, ErrSourceExists
	}
	return w, nil
}

// addLabel covers the client area with a STATIC control. Without SS_NOTIFY it answers WM_NCHITTEST with
// HTTRANSPARENT, so the mouse messages reach this window.
func (w *sourceWindow) addLabel() error {
	className, err := windows.UTF16PtrFromString("STATIC")
	if err != nil {
		return fmt.Errorf("label class name: %w", err)
	}
	text, err := windows.UTF16PtrFromString(sourceLabel)
	if err != nil {
		return fmt.Errorf("label text: %w", err)
	}
	label, _, callErr := procCreateWindowEx.Call(0, uintptr(unsafe.Pointer(className)), uintptr(unsafe.Pointer(text)),
		uintptr(wsChild|wsVisible|ssCenter), sourceLabelMargin, sourceLabelMargin,
		sourceWindowWidth-4*sourceLabelMargin, sourceWindowHeight-6*sourceLabelMargin, w.hwnd, 0, w.class.instance, 0)
	if label == 0 {
		return win32Err("CreateWindowExW(STATIC)", callErr)
	}
	return nil
}

func (w *sourceWindow) Closed() <-chan struct{} {
	return w.closed
}

func (w *sourceWindow) History() []dragRecord {
	w.mu.Lock()
	defer w.mu.Unlock()
	return append([]dragRecord(nil), w.history...)
}

func sourceWndProc(hwnd, message, wParam, lParam uintptr) uintptr {
	if w := activeSource.Load(); w != nil && w.hwnd == hwnd {
		switch message {
		case wmLButtonDown:
			w.onLButtonDown()
			return 0
		case wmDestroy:
			w.hwnd = 0
			w.closeOnce.Do(func() { close(w.closed) })
		}
	}
	result, _, _ := procDefWindowProc.Call(hwnd, message, wParam, lParam)
	return result
}

func (w *sourceWindow) onLButtonDown() {
	if !w.dragging.CompareAndSwap(false, true) {
		return
	}
	defer w.dragging.Store(false)
	record, err := w.drag()
	if err != nil {
		record.Failure = err
		w.log.Error("drag failed", "err", err)
	}
	w.mu.Lock()
	w.history = append(w.history, record)
	w.mu.Unlock()
}

// drag runs the DoDragDrop modal loop. While it runs, the STA thread keeps dispatching OLE traffic, including calls
// into this process's own drop target when the cursor crosses the edge window.
func (w *sourceWindow) drag() (dragRecord, error) {
	var record dragRecord
	data, err := newDragData(w.log, w.formats, demoEntries(time.Now()), w.opts)
	if err != nil {
		return record, err
	}
	record.Data = data
	dataObject, err := wincom.NewDataObject(data)
	if err != nil {
		return record, err
	}
	defer wincom.Release(dataObject)

	session := &sourceSession{log: w.log}
	if err = session.begin(); err != nil {
		return record, err
	}
	dropSource, err := wincom.NewDropSource(session)
	if err != nil {
		return record, err
	}
	defer wincom.Release(dropSource)

	w.log.Info("DoDragDrop starting", "ok_effects", effectName(wincom.DropEffectCopy))
	started := time.Now()
	effect := new(uint32)
	hr, _, _ := procDoDragDrop.Call(dataObject, dropSource, uintptr(wincom.DropEffectCopy), uintptr(unsafe.Pointer(effect)))
	record.Session = session.summary()
	result := wincom.HResult(uint32(hr))
	outcome, err := dragOutcome(result, *effect)
	if err != nil {
		return record, err
	}
	record.Result = outcome
	w.log.Info("DoDragDrop returned", "hresult", result.Error(), "outcome", outcome, "elapsed", time.Since(started),
		"queries", record.Session.Queries, "feedback", record.Session.Feedback)
	return record, nil
}

func (w *sourceWindow) close() error {
	var errs []error
	if w.hwnd != 0 {
		errs = append(errs, destroyWindow(w.hwnd))
	}
	activeSource.CompareAndSwap(w, nil)
	errs = append(errs, w.class.unregister())
	return errors.Join(errs...)
}
