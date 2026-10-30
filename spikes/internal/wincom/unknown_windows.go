package wincom

import (
	"fmt"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

type HResult uint32

const (
	SOK                    HResult = 0
	SFalse                 HResult = 1
	DataSSameFormatEtc     HResult = HResult(windows.DATA_S_SAMEFORMATETC)
	DragDropSDrop          HResult = HResult(windows.DRAGDROP_S_DROP)
	DragDropSCancel        HResult = HResult(windows.DRAGDROP_S_CANCEL)
	DragDropSUseDefaultCur HResult = HResult(windows.DRAGDROP_S_USEDEFAULTCURSORS)

	ENotImpl               HResult = HResult(windows.E_NOTIMPL)
	ENoInterface           HResult = HResult(windows.E_NOINTERFACE)
	EPointer               HResult = HResult(windows.E_POINTER)
	EFail                  HResult = HResult(windows.E_FAIL)
	EUnexpected            HResult = HResult(windows.E_UNEXPECTED)
	EInvalidArg            HResult = HResult(windows.E_INVALIDARG)
	OleEAdviseNotSupported HResult = HResult(windows.OLE_E_ADVISENOTSUPPORTED)
	DVEFormatEtc           HResult = HResult(windows.DV_E_FORMATETC)
	DVELindex              HResult = HResult(windows.DV_E_LINDEX)
	DVETymed               HResult = HResult(windows.DV_E_TYMED)
	DVEAspect              HResult = HResult(windows.DV_E_DVASPECT)
	STGEInvalidFunction    HResult = HResult(windows.STG_E_INVALIDFUNCTION)
	STGEAccessDenied       HResult = HResult(windows.STG_E_ACCESSDENIED)
	STGEInvalidPointer     HResult = HResult(windows.STG_E_INVALIDPOINTER)
	STGEReadFault          HResult = HResult(windows.STG_E_READFAULT)
	STGEInvalidFlag        HResult = 0x800300FF
	STGEInsufficientMemory HResult = HResult(windows.STG_E_INSUFFICIENTMEMORY)
	STGEMediumFull         HResult = HResult(windows.STG_E_MEDIUMFULL)
)

func (h HResult) Error() string {
	return fmt.Sprintf("HRESULT 0x%08X", uint32(h))
}

func (h HResult) Failed() bool {
	return int32(h) < 0
}

func (h HResult) ret() uintptr {
	return uintptr(h)
}

var (
	iidIUnknown          = windows.GUID{Data1: 0x00000000, Data2: 0x0000, Data3: 0x0000, Data4: [8]byte{0xC0, 0, 0, 0, 0, 0, 0, 0x46}}
	iidIDataObject       = windows.GUID{Data1: 0x0000010e, Data2: 0x0000, Data3: 0x0000, Data4: [8]byte{0xC0, 0, 0, 0, 0, 0, 0, 0x46}}
	iidIEnumFormatEtc    = windows.GUID{Data1: 0x00000103, Data2: 0x0000, Data3: 0x0000, Data4: [8]byte{0xC0, 0, 0, 0, 0, 0, 0, 0x46}}
	iidIStream           = windows.GUID{Data1: 0x0000000c, Data2: 0x0000, Data3: 0x0000, Data4: [8]byte{0xC0, 0, 0, 0, 0, 0, 0, 0x46}}
	iidISequentialStream = windows.GUID{Data1: 0x0c733a30, Data2: 0x2a1c, Data3: 0x11ce, Data4: [8]byte{0xad, 0xe5, 0x00, 0xaa, 0x00, 0x44, 0x77, 0x3d}}
	iidIDropSource       = windows.GUID{Data1: 0x00000121, Data2: 0x0000, Data3: 0x0000, Data4: [8]byte{0xC0, 0, 0, 0, 0, 0, 0, 0x46}}
	iidIDropTarget       = windows.GUID{Data1: 0x00000122, Data2: 0x0000, Data3: 0x0000, Data4: [8]byte{0xC0, 0, 0, 0, 0, 0, 0, 0x46}}
)

const (
	vtblQueryInterface = 0
	vtblAddRef         = 1
	vtblRelease        = 2
)

type object struct {
	iids    []windows.GUID
	impl    any
	block   uintptr
	cleanup func()
}

func (o *object) answers(iid windows.GUID) bool {
	for _, known := range o.iids {
		if known == iid {
			return true
		}
	}
	return false
}

var objects = NewTable[*object]()

// ObjectCount reports how many hosted COM objects are alive.
func ObjectCount() int {
	return objects.Len()
}

func createObject(vtbl uintptr, iids []windows.GUID, impl any, cleanup func()) (uintptr, error) {
	obj := &object{iids: append([]windows.GUID{iidIUnknown}, iids...), impl: impl, cleanup: cleanup}
	block, err := allocObjectBlock(vtbl, 0)
	if err != nil {
		return 0, err
	}
	obj.block = block
	(*objectBlock)(ptr(block)).handle = objects.Add(obj)
	return block, nil
}

func lookup(this uintptr) (*object, bool) {
	if this == 0 {
		return nil, false
	}
	return objects.Get(blockHandle(this))
}

func implOf[T any](this uintptr) (T, bool) {
	obj, ok := lookup(this)
	if !ok {
		var zero T
		return zero, false
	}
	impl, ok := obj.impl.(T)
	return impl, ok
}

var unknownMethods = sync.OnceValue(func() [3]uintptr {
	return [3]uintptr{
		newCallback(queryInterfaceCallback),
		newCallback(addRefCallback),
		newCallback(releaseCallback),
	}
})

func newInterfaceVtable(own ...uintptr) (uintptr, error) {
	unknown := unknownMethods()
	return allocVtable(append(unknown[:], own...))
}

func queryInterfaceCallback(this, riid, ppv uintptr) uintptr {
	if ppv == 0 {
		return EPointer.ret()
	}
	putUintptr(ppv, 0)
	if riid == 0 {
		return EPointer.ret()
	}
	obj, ok := lookup(this)
	if !ok {
		return EUnexpected.ret()
	}
	if !obj.answers(*(*windows.GUID)(ptr(riid))) {
		return ENoInterface.ret()
	}
	if _, ok := objects.AddRef(blockHandle(this)); !ok {
		return EUnexpected.ret()
	}
	putUintptr(ppv, this)
	return SOK.ret()
}

func addRefCallback(this uintptr) uintptr {
	if this == 0 {
		return 0
	}
	count, _ := objects.AddRef(blockHandle(this))
	return uintptr(count)
}

func releaseCallback(this uintptr) uintptr {
	if this == 0 {
		return 0
	}
	count, obj, freed, ok := objects.Release(blockHandle(this))
	if !ok || !freed {
		return uintptr(count)
	}
	if obj.cleanup != nil {
		obj.cleanup()
	}
	if err := localFree(obj.block); err != nil {
		panic(fmt.Sprintf("wincom: releasing COM object memory: %v", err))
	}
	return 0
}

func AddRef(obj uintptr) uint32 {
	return uint32(callMethod(obj, vtblAddRef))
}

func Release(obj uintptr) uint32 {
	return uint32(callMethod(obj, vtblRelease))
}

// QueryInterface returns the interface pointer for iid with an added reference; the caller must Release it.
func QueryInterface(obj uintptr, iid windows.GUID) (uintptr, error) {
	var out uintptr
	hr := callMethod(obj, vtblQueryInterface, uintptr(unsafe.Pointer(&iid)), uintptr(unsafe.Pointer(&out)))
	if result := HResult(hr); result.Failed() {
		return 0, fmt.Errorf("QueryInterface: %w", result)
	}
	return out, nil
}
