package wincom

import (
	"fmt"
	"sync"

	"golang.org/x/sys/windows"
)

const (
	dropTargetDragEnter = iota + 3
	dropTargetDragOver
	dropTargetDragLeave
	dropTargetDrop
)

type Point struct {
	X, Y int32
}

// DropTarget receives the IDropTarget notifications. The data object passed to DragEnter and Drop is only guaranteed
// valid during the call; AddRef it to keep it. The returned effect is written back as the drop effect; on error the
// effect is DropEffectNone.
type DropTarget interface {
	DragEnter(data uintptr, keyState uint32, pt Point, effect uint32) (uint32, error)
	DragOver(keyState uint32, pt Point, effect uint32) (uint32, error)
	DragLeave() error
	Drop(data uintptr, keyState uint32, pt Point, effect uint32) (uint32, error)
}

var dropTargetVtable = sync.OnceValues(func() (uintptr, error) {
	return newInterfaceVtable(
		newCallback(dropTargetDragEnterCallback),
		newCallback(dropTargetDragOverCallback),
		newCallback(dropTargetDragLeaveCallback),
		newCallback(dropTargetDropCallback),
	)
})

// NewDropTarget returns an IDropTarget backed by target with one reference held by the caller.
func NewDropTarget(target DropTarget) (uintptr, error) {
	vtbl, err := dropTargetVtable()
	if err != nil {
		return 0, fmt.Errorf("IDropTarget vtable: %w", err)
	}
	return createObject(vtbl, []windows.GUID{iidIDropTarget}, target, nil)
}

// A POINTL passed by value occupies one 64-bit argument register: x in the low half, y in the high half.
func pointFromArg(arg uintptr) Point {
	return Point{X: int32(uint32(arg)), Y: int32(uint32(arg >> 32))}
}

func finishEffect(pdwEffect uintptr, effect uint32, err error) uintptr {
	if err != nil {
		putUint32(pdwEffect, DropEffectNone)
		return toHResult(err).ret()
	}
	putUint32(pdwEffect, effect)
	return SOK.ret()
}

func dropTargetDragEnterCallback(this, data, keyState, pt, pdwEffect uintptr) uintptr {
	if pdwEffect == 0 {
		return EPointer.ret()
	}
	target, ok := implOf[DropTarget](this)
	if !ok {
		return EUnexpected.ret()
	}
	effect, err := target.DragEnter(data, uint32(keyState), pointFromArg(pt), getUint32(pdwEffect))
	return finishEffect(pdwEffect, effect, err)
}

func dropTargetDragOverCallback(this, keyState, pt, pdwEffect uintptr) uintptr {
	if pdwEffect == 0 {
		return EPointer.ret()
	}
	target, ok := implOf[DropTarget](this)
	if !ok {
		return EUnexpected.ret()
	}
	effect, err := target.DragOver(uint32(keyState), pointFromArg(pt), getUint32(pdwEffect))
	return finishEffect(pdwEffect, effect, err)
}

func dropTargetDragLeaveCallback(this uintptr) uintptr {
	target, ok := implOf[DropTarget](this)
	if !ok {
		return EUnexpected.ret()
	}
	return toHResult(target.DragLeave()).ret()
}

func dropTargetDropCallback(this, data, keyState, pt, pdwEffect uintptr) uintptr {
	if pdwEffect == 0 {
		return EPointer.ret()
	}
	target, ok := implOf[DropTarget](this)
	if !ok {
		return EUnexpected.ret()
	}
	effect, err := target.Drop(data, uint32(keyState), pointFromArg(pt), getUint32(pdwEffect))
	return finishEffect(pdwEffect, effect, err)
}
