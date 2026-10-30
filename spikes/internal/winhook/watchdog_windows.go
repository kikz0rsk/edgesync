package winhook

import (
	"context"
	"fmt"
	"syscall"
	"time"
	"unsafe"
)

const (
	WatchdogInterval = time.Second
	WatchdogTimeout  = 2 * time.Second

	inputTypeMouse    = 0
	inputTypeKeyboard = 1

	mouseeventfMove = 0x0001
	keyeventfKeyUp  = 0x0002
	vkF24           = 0x87
)

type mouseInput struct {
	DX        int32
	DY        int32
	MouseData uint32
	Flags     uint32
	Time      uint32
	ExtraInfo uintptr
}

type keybdInput struct {
	Vk        uint16
	Scan      uint16
	Flags     uint32
	Time      uint32
	ExtraInfo uintptr
}

type inputMouse struct {
	Type uint32
	MI   mouseInput
}

type inputKeyboard struct {
	Type uint32
	KI   keybdInput
	_    [8]byte
}

var (
	_ [unsafe.Sizeof(inputMouse{}) - 40]struct{}
	_ [40 - unsafe.Sizeof(inputMouse{})]struct{}
	_ [unsafe.Sizeof(inputKeyboard{}) - 40]struct{}
	_ [40 - unsafe.Sizeof(inputKeyboard{})]struct{}
)

func sendMouseMarker() error {
	input := inputMouse{
		Type: inputTypeMouse,
		MI:   mouseInput{Flags: mouseeventfMove, ExtraInfo: MarkerExtraInfo},
	}
	return sendInput(unsafe.Pointer(&input), unsafe.Sizeof(input), "zero relative mouse move")
}

func sendKeyMarker() error {
	input := inputKeyboard{
		Type: inputTypeKeyboard,
		KI:   keybdInput{Vk: vkF24, Flags: keyeventfKeyUp, ExtraInfo: MarkerExtraInfo},
	}
	return sendInput(unsafe.Pointer(&input), unsafe.Sizeof(input), "F24 key-up")
}

func sendInput(input unsafe.Pointer, size uintptr, what string) error {
	inserted, _, errno := syscall.SyscallN(procSendInput.Addr(), 1, uintptr(input), size)
	if inserted != 1 {
		return fmt.Errorf("SendInput(%s): inserted %d of 1 events: %w", what, inserted, errno)
	}
	return nil
}

func (h *Hook) runWatchdog(ctx context.Context) {
	defer h.wg.Done()
	ticker := time.NewTicker(WatchdogInterval)
	defer ticker.Stop()

	h.noteMarker()
	var lastRepost int64
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}

		if err := sendMouseMarker(); err != nil {
			h.fail(err)
			return
		}
		if err := sendKeyMarker(); err != nil {
			h.fail(err)
			return
		}

		now := int64(time.Since(h.epoch))
		if ctx.Err() == nil && now-max(h.lastMarkerNs.Load(), lastRepost) > int64(WatchdogTimeout) {
			if err := postThreadMessage(h.threadID.Load(), wmReinstall); err != nil {
				h.fail(err)
				return
			}
			lastRepost = now
		}
	}
}
