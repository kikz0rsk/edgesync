package wincom

import (
	"fmt"
	"sync/atomic"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

const pointerSize = unsafe.Sizeof(uintptr(0))

var (
	kernel32 = windows.NewLazySystemDLL("kernel32.dll")
	ole32    = windows.NewLazySystemDLL("ole32.dll")
	user32   = windows.NewLazySystemDLL("user32.dll")
)

var callbackCount atomic.Int32

// CallbackCount reports how many syscall callbacks this package has created; callbacks are never freed by Go.
func CallbackCount() int {
	return int(callbackCount.Load())
}

func newCallback(fn any) uintptr {
	callbackCount.Add(1)
	return syscall.NewCallback(fn)
}

// ptr converts a non-Go address to an unsafe.Pointer without tripping the unsafeptr vet check.
func ptr(addr uintptr) unsafe.Pointer {
	return *(*unsafe.Pointer)(unsafe.Pointer(&addr))
}

func localAlloc(size uintptr) (uintptr, error) {
	mem, err := windows.LocalAlloc(windows.LPTR, uint32(size))
	if err != nil {
		return 0, fmt.Errorf("LocalAlloc(%d): %w", size, err)
	}
	return mem, nil
}

func localFree(mem uintptr) error {
	if _, err := windows.LocalFree(windows.Handle(mem)); err != nil {
		return fmt.Errorf("LocalFree: %w", err)
	}
	return nil
}

func allocVtable(methods []uintptr) (uintptr, error) {
	mem, err := localAlloc(uintptr(len(methods)) * pointerSize)
	if err != nil {
		return 0, err
	}
	copy(unsafe.Slice((*uintptr)(ptr(mem)), len(methods)), methods)
	return mem, nil
}

type objectBlock struct {
	vtbl   uintptr
	handle uintptr
}

func allocObjectBlock(vtbl, handle uintptr) (uintptr, error) {
	mem, err := localAlloc(unsafe.Sizeof(objectBlock{}))
	if err != nil {
		return 0, err
	}
	block := (*objectBlock)(ptr(mem))
	block.vtbl = vtbl
	block.handle = handle
	return mem, nil
}

func blockHandle(this uintptr) uintptr {
	return (*objectBlock)(ptr(this)).handle
}

func vtblOf(this uintptr) uintptr {
	return (*objectBlock)(ptr(this)).vtbl
}

// methodAddr returns the address of the index-th entry of the vtable of the COM object at obj.
func methodAddr(obj uintptr, index int) uintptr {
	vtbl := *(*uintptr)(ptr(obj))
	return *(*uintptr)(ptr(vtbl + uintptr(index)*pointerSize))
}

// callMethod invokes the index-th method of the COM object obj. uintptrescapes keeps arguments converted from
// unsafe.Pointer on the heap: the callee may re-enter Go on this goroutine, and a stack copy would invalidate them.
//
//go:uintptrescapes
func callMethod(obj uintptr, index int, args ...uintptr) uintptr {
	ret, _, _ := syscall.SyscallN(methodAddr(obj, index), append([]uintptr{obj}, args...)...)
	return ret
}

func putUintptr(addr, value uintptr) {
	*(*uintptr)(ptr(addr)) = value
}

func putUint32(addr uintptr, value uint32) {
	*(*uint32)(ptr(addr)) = value
}

func putUint64(addr uintptr, value uint64) {
	*(*uint64)(ptr(addr)) = value
}

func getUint32(addr uintptr) uint32 {
	return *(*uint32)(ptr(addr))
}
