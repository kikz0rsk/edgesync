package wincom

import (
	"errors"
	"fmt"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	TymedHGlobal uint32 = 1
	TymedIStream uint32 = 4

	DVAspectContent uint32 = 1

	DropEffectNone uint32 = 0
	DropEffectCopy uint32 = 1
	DropEffectMove uint32 = 2
	DropEffectLink uint32 = 4

	CFUnicodeText uint16 = 13
	CFHDrop       uint16 = 15

	FDAttributes uint32 = 0x00000004
	FDWritesTime uint32 = 0x00000020
	FDFileSize   uint32 = 0x00000040
	FDProgressUI uint32 = 0x00004000
	FDUnicode    uint32 = 0x80000000

	FormatNameFileGroupDescriptorW = "FileGroupDescriptorW"
	FormatNameFileContents         = "FileContents"
	FormatNamePreferredDropEffect  = "Preferred DropEffect"
	FormatNamePerformedDropEffect  = "Performed DropEffect"
	FormatNamePasteSucceeded       = "Paste Succeeded"
	FormatNameHTML                 = "HTML Format"
	FormatNameRTF                  = "Rich Text Format"
	FormatNamePNG                  = "PNG"
	FormatNameExcludeFromMonitor   = "ExcludeClipboardContentFromMonitorProcessing"
	FormatNameCanIncludeInHistory  = "CanIncludeInClipboardHistory"

	dataDirGet uint32 = 1
	dataDirSet uint32 = 2

	gmemMoveable = 0x0002
)

var (
	ErrEmptyMedium = errors.New("wincom: empty data cannot be placed in an HGLOBAL")
	ErrMediumType  = errors.New("wincom: medium does not hold the requested storage type")
)

var (
	procRegisterClipboardFormat = user32.NewProc("RegisterClipboardFormatW")
	procGlobalAlloc             = kernel32.NewProc("GlobalAlloc")
	procGlobalLock              = kernel32.NewProc("GlobalLock")
	procGlobalUnlock            = kernel32.NewProc("GlobalUnlock")
	procGlobalSize              = kernel32.NewProc("GlobalSize")
	procReleaseStgMedium        = ole32.NewProc("ReleaseStgMedium")
)

// FormatEtc mirrors FORMATETC (32 bytes on amd64).
type FormatEtc struct {
	Format       uint16
	TargetDevice uintptr
	Aspect       uint32
	Index        int32
	Tymed        uint32
}

// Medium mirrors STGMEDIUM (24 bytes on amd64). Handle is an HGLOBAL for TymedHGlobal and an IStream pointer for
// TymedIStream.
type Medium struct {
	Tymed        uint32
	Handle       uintptr
	ReleaseOwner uintptr
}

// FileDescriptorW mirrors FILEDESCRIPTORW (592 bytes).
type FileDescriptorW struct {
	Flags          uint32
	Clsid          windows.GUID
	SizeL          [2]int32
	PointL         [2]int32
	FileAttributes uint32
	CreationTime   windows.Filetime
	LastAccessTime windows.Filetime
	LastWriteTime  windows.Filetime
	FileSizeHigh   uint32
	FileSizeLow    uint32
	FileName       [windows.MAX_PATH]uint16
}

// DropFiles mirrors DROPFILES (20 bytes).
type DropFiles struct {
	FilesOffset uint32
	PointX      int32
	PointY      int32
	NonClient   int32
	Wide        int32
}

// DataSource supplies the content of a hosted IDataObject. GetData and SetData run on the thread that Windows calls
// the object on (the STA thread). SetData must copy what it needs before returning: the medium is released right
// after the call when the caller transferred ownership.
type DataSource interface {
	Formats() []FormatEtc
	GetData(f FormatEtc) (Medium, error)
	SetData(f FormatEtc, m Medium) error
}

func RegisterFormat(name string) (uint16, error) {
	namePtr, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return 0, fmt.Errorf("clipboard format name %q: %w", name, err)
	}
	id, _, callErr := procRegisterClipboardFormat.Call(uintptr(unsafe.Pointer(namePtr)))
	if id == 0 {
		return 0, fmt.Errorf("RegisterClipboardFormatW(%q): %w", name, callErr)
	}
	return uint16(id), nil
}

// NewHGlobalMedium copies data into a new movable HGLOBAL owned by the returned medium.
func NewHGlobalMedium(data []byte) (Medium, error) {
	if len(data) == 0 {
		return Medium{}, ErrEmptyMedium
	}
	handle, _, callErr := procGlobalAlloc.Call(gmemMoveable, uintptr(len(data)))
	if handle == 0 {
		return Medium{}, fmt.Errorf("GlobalAlloc(%d): %w", len(data), callErr)
	}
	locked, _, callErr := procGlobalLock.Call(handle)
	if locked == 0 {
		return Medium{}, fmt.Errorf("GlobalLock: %w", callErr)
	}
	copy(unsafe.Slice((*byte)(ptr(locked)), len(data)), data)
	procGlobalUnlock.Call(handle)
	return Medium{Tymed: TymedHGlobal, Handle: handle}, nil
}

// NewStreamMedium wraps an IStream pointer; the medium owns the caller's reference.
func NewStreamMedium(stream uintptr) Medium {
	return Medium{Tymed: TymedIStream, Handle: stream}
}

// HGlobalBytes copies the content of an HGLOBAL medium without releasing it.
func (m Medium) HGlobalBytes() ([]byte, error) {
	if m.Tymed != TymedHGlobal {
		return nil, ErrMediumType
	}
	size, _, _ := procGlobalSize.Call(m.Handle)
	locked, _, callErr := procGlobalLock.Call(m.Handle)
	if locked == 0 {
		return nil, fmt.Errorf("GlobalLock: %w", callErr)
	}
	defer procGlobalUnlock.Call(m.Handle)
	out := make([]byte, size)
	copy(out, unsafe.Slice((*byte)(ptr(locked)), size))
	return out, nil
}

// ReleaseMedium frees the storage held by m (ReleaseStgMedium) and clears it.
func ReleaseMedium(m *Medium) {
	procReleaseStgMedium.Call(uintptr(unsafe.Pointer(m)))
	*m = Medium{}
}

func toHResult(err error) HResult {
	if err == nil {
		return SOK
	}
	var hr HResult
	if errors.As(err, &hr) {
		return hr
	}
	return EFail
}

type dataObject struct {
	source DataSource
}

const (
	dataGetData = iota + 3
	dataGetDataHere
	dataQueryGetData
	dataGetCanonicalFormatEtc
	dataSetData
	dataEnumFormatEtc
	dataDAdvise
	dataDUnadvise
	dataEnumDAdvise
)

var dataObjectVtable = sync.OnceValues(func() (uintptr, error) {
	return newInterfaceVtable(
		newCallback(dataGetDataCallback),
		newCallback(dataGetDataHereCallback),
		newCallback(dataQueryGetDataCallback),
		newCallback(dataGetCanonicalFormatEtcCallback),
		newCallback(dataSetDataCallback),
		newCallback(dataEnumFormatEtcCallback),
		newCallback(dataDAdviseCallback),
		newCallback(dataDUnadviseCallback),
		newCallback(dataEnumDAdviseCallback),
	)
})

// NewDataObject returns an IDataObject backed by source with one reference held by the caller.
func NewDataObject(source DataSource) (uintptr, error) {
	vtbl, err := dataObjectVtable()
	if err != nil {
		return 0, fmt.Errorf("IDataObject vtable: %w", err)
	}
	return createObject(vtbl, []windows.GUID{iidIDataObject}, &dataObject{source: source}, nil)
}

func checkSupported(formats []FormatEtc, want FormatEtc) HResult {
	result := DVEFormatEtc
	for _, offered := range formats {
		if offered.Format != want.Format {
			continue
		}
		if offered.Aspect&want.Aspect == 0 {
			if result == DVEFormatEtc {
				result = DVEAspect
			}
			continue
		}
		if offered.Tymed&want.Tymed == 0 {
			result = DVETymed
			continue
		}
		return SOK
	}
	return result
}

func dataGetDataCallback(this, pformatetc, pmedium uintptr) uintptr {
	if pformatetc == 0 || pmedium == 0 {
		return EPointer.ret()
	}
	*(*Medium)(ptr(pmedium)) = Medium{}
	data, ok := implOf[*dataObject](this)
	if !ok {
		return EUnexpected.ret()
	}
	want := *(*FormatEtc)(ptr(pformatetc))
	if hr := checkSupported(data.source.Formats(), want); hr != SOK {
		return hr.ret()
	}
	medium, err := data.source.GetData(want)
	if err != nil {
		return toHResult(err).ret()
	}
	if medium.Tymed&want.Tymed == 0 {
		ReleaseMedium(&medium)
		return DVETymed.ret()
	}
	*(*Medium)(ptr(pmedium)) = medium
	return SOK.ret()
}

func dataGetDataHereCallback(this, pformatetc, pmedium uintptr) uintptr {
	return ENotImpl.ret()
}

func dataQueryGetDataCallback(this, pformatetc uintptr) uintptr {
	if pformatetc == 0 {
		return EPointer.ret()
	}
	data, ok := implOf[*dataObject](this)
	if !ok {
		return EUnexpected.ret()
	}
	return checkSupported(data.source.Formats(), *(*FormatEtc)(ptr(pformatetc))).ret()
}

func dataGetCanonicalFormatEtcCallback(this, pformatectIn, pformatetcOut uintptr) uintptr {
	if pformatectIn == 0 || pformatetcOut == 0 {
		return EPointer.ret()
	}
	canonical := *(*FormatEtc)(ptr(pformatectIn))
	canonical.TargetDevice = 0
	*(*FormatEtc)(ptr(pformatetcOut)) = canonical
	return DataSSameFormatEtc.ret()
}

func dataSetDataCallback(this, pformatetc, pmedium, fRelease uintptr) uintptr {
	if pformatetc == 0 || pmedium == 0 {
		return EPointer.ret()
	}
	data, ok := implOf[*dataObject](this)
	if !ok {
		return EUnexpected.ret()
	}
	if err := data.source.SetData(*(*FormatEtc)(ptr(pformatetc)), *(*Medium)(ptr(pmedium))); err != nil {
		return toHResult(err).ret()
	}
	if uint32(fRelease) != 0 {
		ReleaseMedium((*Medium)(ptr(pmedium)))
	}
	return SOK.ret()
}

func dataEnumFormatEtcCallback(this, direction, ppenum uintptr) uintptr {
	if ppenum == 0 {
		return EPointer.ret()
	}
	putUintptr(ppenum, 0)
	data, ok := implOf[*dataObject](this)
	if !ok {
		return EUnexpected.ret()
	}
	switch uint32(direction) {
	case dataDirGet:
		enum, err := NewFormatEnumerator(data.source.Formats())
		if err != nil {
			return EFail.ret()
		}
		putUintptr(ppenum, enum)
		return SOK.ret()
	case dataDirSet:
		return ENotImpl.ret()
	default:
		return EInvalidArg.ret()
	}
}

func dataDAdviseCallback(this, pformatetc, advf, pAdvSink, pdwConnection uintptr) uintptr {
	if pdwConnection != 0 {
		putUint32(pdwConnection, 0)
	}
	return OleEAdviseNotSupported.ret()
}

func dataDUnadviseCallback(this, connection uintptr) uintptr {
	return OleEAdviseNotSupported.ret()
}

func dataEnumDAdviseCallback(this, ppenumAdvise uintptr) uintptr {
	if ppenumAdvise != 0 {
		putUintptr(ppenumAdvise, 0)
	}
	return OleEAdviseNotSupported.ret()
}

// GetData calls IDataObject::GetData on a data object hosted by another party; release the medium with ReleaseMedium.
func GetData(dataObject uintptr, f FormatEtc) (Medium, error) {
	var medium Medium
	hr := callMethod(dataObject, dataGetData, uintptr(unsafe.Pointer(&f)), uintptr(unsafe.Pointer(&medium)))
	if result := HResult(hr); result.Failed() {
		return Medium{}, fmt.Errorf("IDataObject::GetData(format %d, lindex %d): %w", f.Format, f.Index, result)
	}
	return medium, nil
}

// QueryGetData reports a nil error only when the data object answers S_OK.
func QueryGetData(dataObject uintptr, f FormatEtc) error {
	hr := callMethod(dataObject, dataQueryGetData, uintptr(unsafe.Pointer(&f)))
	if result := HResult(hr); result != SOK {
		return fmt.Errorf("IDataObject::QueryGetData(format %d): %w", f.Format, result)
	}
	return nil
}

// EnumFormats lists the formats a data object offers for reading.
func EnumFormats(dataObject uintptr) ([]FormatEtc, error) {
	var enum uintptr
	hr := callMethod(dataObject, dataEnumFormatEtc, uintptr(dataDirGet), uintptr(unsafe.Pointer(&enum)))
	if result := HResult(hr); result.Failed() {
		return nil, fmt.Errorf("IDataObject::EnumFormatEtc: %w", result)
	}
	defer Release(enum)
	var formats []FormatEtc
	for {
		var next FormatEtc
		var fetched uint32
		hr = callMethod(enum, enumNext, 1, uintptr(unsafe.Pointer(&next)), uintptr(unsafe.Pointer(&fetched)))
		result := HResult(hr)
		if result.Failed() {
			return nil, fmt.Errorf("IEnumFORMATETC::Next: %w", result)
		}
		if fetched == 0 {
			return formats, nil
		}
		formats = append(formats, next)
	}
}
