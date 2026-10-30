//go:build windows

package main

import (
	"encoding/binary"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"path"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"
	"unsafe"

	"github.com/kikz0r/edgesync/spikes/internal/wincom"
	"golang.org/x/sys/windows"
)

const (
	fileAttributeDirectory uint32 = 0x00000010
	fileAttributeNormal    uint32 = 0x00000080

	descriptorFlags = wincom.FDAttributes | wincom.FDFileSize | wincom.FDWritesTime | wincom.FDProgressUI | wincom.FDUnicode

	groupHeaderSize    = 4
	maxPathUnits       = 259
	forbiddenNameRunes = `<>:"|?*\`

	filetimeTicksPerSecond  = 10_000_000
	unixToFiletimeEpochSecs = 11_644_473_600
	maxFiletimeSeconds      = math.MaxUint64/filetimeTicksPerSecond - 1

	standardFormatText    = 1
	standardFormatUnicode = 13
	standardFormatHDrop   = 15
	formatNameBufferUnits = 256
)

var ErrNotRepresentable = errors.New("entry is not representable as a Windows virtual file")

var procGetClipboardFormatName = windows.NewLazySystemDLL("user32.dll").NewProc("GetClipboardFormatNameW")

// buildFileGroupDescriptor returns the bytes of a FILEGROUPDESCRIPTORW: a UINT item count followed by one 592-byte
// FILEDESCRIPTORW per entry.
func buildFileGroupDescriptor(entries []entry) ([]byte, error) {
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

func newFileDescriptor(e entry) (wincom.FileDescriptorW, error) {
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
// of D-99 (reserved characters, control characters) and the structural rules of D-45.
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

func (f shellFormats) name(id uint16) string {
	switch id {
	case f.Descriptor:
		return wincom.FormatNameFileGroupDescriptorW
	case f.Contents:
		return wincom.FormatNameFileContents
	case f.Preferred:
		return wincom.FormatNamePreferredDropEffect
	case f.Performed:
		return wincom.FormatNamePerformedDropEffect
	case f.PasteSucceeded:
		return wincom.FormatNamePasteSucceeded
	case standardFormatText:
		return "CF_TEXT"
	case standardFormatUnicode:
		return "CF_UNICODETEXT"
	case standardFormatHDrop:
		return "CF_HDROP"
	}
	buf := make([]uint16, formatNameBufferUnits)
	n, _, _ := procGetClipboardFormatName.Call(uintptr(id), uintptr(unsafe.Pointer(&buf[0])), formatNameBufferUnits)
	if n == 0 {
		return fmt.Sprintf("format#%d", id)
	}
	return windows.UTF16ToString(buf[:n])
}

func tymedName(tymed uint32) string {
	names := []struct {
		bit  uint32
		name string
	}{{1, "HGLOBAL"}, {2, "FILE"}, {4, "ISTREAM"}, {8, "ISTORAGE"}, {16, "GDI"}, {32, "MFPICT"}, {64, "ENHMF"}}
	var parts []string
	for _, candidate := range names {
		if tymed&candidate.bit != 0 {
			parts = append(parts, candidate.name)
		}
	}
	if len(parts) == 0 {
		return fmt.Sprintf("0x%X", tymed)
	}
	return strings.Join(parts, "|")
}

func aspectName(aspect uint32) string {
	names := []struct {
		bit  uint32
		name string
	}{{1, "CONTENT"}, {2, "THUMBNAIL"}, {4, "ICON"}, {8, "DOCPRINT"}}
	var parts []string
	for _, candidate := range names {
		if aspect&candidate.bit != 0 {
			parts = append(parts, candidate.name)
		}
	}
	if len(parts) == 0 {
		return fmt.Sprintf("0x%X", aspect)
	}
	return strings.Join(parts, "|")
}

func effectName(effect uint32) string {
	if effect == wincom.DropEffectNone {
		return "NONE"
	}
	var parts []string
	for _, candidate := range []struct {
		bit  uint32
		name string
	}{{wincom.DropEffectCopy, "COPY"}, {wincom.DropEffectMove, "MOVE"}, {wincom.DropEffectLink, "LINK"}} {
		if effect&candidate.bit != 0 {
			parts = append(parts, candidate.name)
		}
	}
	if len(parts) == 0 {
		return fmt.Sprintf("0x%X", effect)
	}
	return strings.Join(parts, "|")
}

// dropOutcomes holds what the paste target reported back through IDataObject::SetData.
type dropOutcomes struct {
	PerformedSet   bool
	Performed      uint32
	PasteSucceeded bool
	PasteEffect    uint32
}

// clipboardData is the wincom.DataSource behind the clipboard data object.
type clipboardData struct {
	log        *slog.Logger
	source     *syntheticSource
	formats    shellFormats
	descriptor []byte

	mu       sync.Mutex
	outcomes dropOutcomes
}

var _ wincom.DataSource = (*clipboardData)(nil)

func newClipboardData(source *syntheticSource, formats shellFormats, log *slog.Logger) (*clipboardData, error) {
	descriptor, err := buildFileGroupDescriptor(source.Entries())
	if err != nil {
		return nil, fmt.Errorf("file group descriptor: %w", err)
	}
	return &clipboardData{log: log, source: source, formats: formats, descriptor: descriptor}, nil
}

func (d *clipboardData) Outcomes() dropOutcomes {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.outcomes
}

func (d *clipboardData) Formats() []wincom.FormatEtc {
	d.log.Info("IDataObject formats consulted", "call", "Formats", "via", "EnumFormatEtc, QueryGetData or the GetData pre-check")
	return []wincom.FormatEtc{
		{Format: d.formats.Descriptor, Aspect: wincom.DVAspectContent, Index: -1, Tymed: wincom.TymedHGlobal},
		{Format: d.formats.Contents, Aspect: wincom.DVAspectContent, Index: -1, Tymed: wincom.TymedIStream},
		{Format: d.formats.Preferred, Aspect: wincom.DVAspectContent, Index: -1, Tymed: wincom.TymedHGlobal},
	}
}

func (d *clipboardData) logCall(call string, f wincom.FormatEtc) *slog.Logger {
	logger := d.log.With(
		"call", call,
		"format", d.formats.name(f.Format),
		"lindex", f.Index,
		"tymed", tymedName(f.Tymed),
		"aspect", aspectName(f.Aspect),
	)
	logger.Info("IDataObject call")
	return logger
}

func (d *clipboardData) GetData(f wincom.FormatEtc) (wincom.Medium, error) {
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

func (d *clipboardData) fileContents(f wincom.FormatEtc, logger *slog.Logger) (wincom.Medium, error) {
	reader, err := d.source.Open(int(f.Index))
	if err != nil {
		if errors.Is(err, ErrIndexOutOfRange) || errors.Is(err, ErrDirectory) {
			logger.Warn("FileContents refused", "reason", err)
			return wincom.Medium{}, fmt.Errorf("lindex %d: %w: %w", f.Index, wincom.DVELindex, err)
		}
		return wincom.Medium{}, err
	}
	entryPath := d.source.entries[f.Index].Path
	stream, err := wincom.NewStream(reader, reader.size, path.Base(entryPath), func() {
		summary := reader.Close()
		logger.Info("stream released",
			"path", summary.Path,
			"served", summary.BytesServed,
			"size", summary.Size,
			"reads", summary.Reads,
			"completed", summary.Completed,
			"first_byte_after", summary.FirstByteAfter,
			"lifetime", summary.Lifetime,
		)
	})
	if err != nil {
		reader.Close()
		return wincom.Medium{}, fmt.Errorf("IStream for %q: %w", entryPath, err)
	}
	logger.Info("stream created, nothing read yet", "path", entryPath, "size", reader.size)
	return wincom.NewStreamMedium(stream), nil
}

func (d *clipboardData) SetData(f wincom.FormatEtc, m wincom.Medium) error {
	logger := d.logCall("SetData", f)
	if f.Format != d.formats.Performed && f.Format != d.formats.PasteSucceeded {
		logger.Warn("SetData format not handled")
		return wincom.DVEFormatEtc
	}
	raw, err := m.HGlobalBytes()
	if err != nil {
		return fmt.Errorf("%s medium: %w", d.formats.name(f.Format), err)
	}
	if len(raw) < 4 {
		return fmt.Errorf("%s medium holds %d bytes, want a DWORD: %w", d.formats.name(f.Format), len(raw), wincom.EInvalidArg)
	}
	effect := binary.LittleEndian.Uint32(raw)
	d.mu.Lock()
	if f.Format == d.formats.Performed {
		d.outcomes.PerformedSet, d.outcomes.Performed = true, effect
	} else {
		d.outcomes.PasteSucceeded, d.outcomes.PasteEffect = true, effect
	}
	d.mu.Unlock()
	logger.Info("paste outcome reported", "effect", effectName(effect))
	return nil
}
