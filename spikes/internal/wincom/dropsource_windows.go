package wincom

import (
	"fmt"
	"sync"

	"golang.org/x/sys/windows"
)

const (
	dropSourceQueryContinueDrag = iota + 3
	dropSourceGiveFeedback
)

// DropSource answers the two IDropSource questions asked by the DoDragDrop modal loop. QueryContinueDrag returns S_OK
// to continue, DragDropSDrop to drop or DragDropSCancel to cancel; GiveFeedback returns S_OK after setting its own
// cursor or DragDropSUseDefaultCur to let OLE set the default cursors.
type DropSource interface {
	QueryContinueDrag(escapePressed bool, keyState uint32) HResult
	GiveFeedback(effect uint32) HResult
}

var dropSourceVtable = sync.OnceValues(func() (uintptr, error) {
	return newInterfaceVtable(
		newCallback(dropSourceQueryContinueDragCallback),
		newCallback(dropSourceGiveFeedbackCallback),
	)
})

// NewDropSource returns an IDropSource backed by source with one reference held by the caller.
func NewDropSource(source DropSource) (uintptr, error) {
	vtbl, err := dropSourceVtable()
	if err != nil {
		return 0, fmt.Errorf("IDropSource vtable: %w", err)
	}
	return createObject(vtbl, []windows.GUID{iidIDropSource}, source, nil)
}

func dropSourceQueryContinueDragCallback(this, escapePressed, keyState uintptr) uintptr {
	source, ok := implOf[DropSource](this)
	if !ok {
		return EUnexpected.ret()
	}
	return source.QueryContinueDrag(uint32(escapePressed) != 0, uint32(keyState)).ret()
}

func dropSourceGiveFeedbackCallback(this, effect uintptr) uintptr {
	source, ok := implOf[DropSource](this)
	if !ok {
		return EUnexpected.ret()
	}
	return source.GiveFeedback(uint32(effect)).ret()
}
