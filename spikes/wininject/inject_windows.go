package main

import (
	"fmt"
	"syscall"
	"time"
	"unsafe"
)

const (
	inputMouse    = 0
	inputKeyboard = 1

	mouseeventfMove        = 0x0001
	mouseeventfXDown       = 0x0080
	mouseeventfXUp         = 0x0100
	mouseeventfWheel       = 0x0800
	mouseeventfHWheel      = 0x1000
	mouseeventfVirtualDesk = 0x4000
	mouseeventfAbsolute    = 0x8000

	xbutton1 = 0x0001
	xbutton2 = 0x0002

	keyeventfExtendedKey = 0x0001
	keyeventfKeyUp       = 0x0002
	keyeventfScanCode    = 0x0008
)

var (
	procSendInput    = user32.NewProc("SendInput")
	procGetCursorPos = user32.NewProc("GetCursorPos")
)

type mouseInput struct {
	Dx        int32
	Dy        int32
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

type mouseEvent struct {
	Type uint32
	MI   mouseInput
}

type keyboardEvent struct {
	Type uint32
	KI   keybdInput
	_    [8]byte
}

var (
	_ [unsafe.Sizeof(mouseEvent{}) - 40]struct{}
	_ [40 - unsafe.Sizeof(mouseEvent{})]struct{}
	_ [unsafe.Sizeof(keyboardEvent{}) - 40]struct{}
	_ [40 - unsafe.Sizeof(keyboardEvent{})]struct{}
)

func sendInputs(events unsafe.Pointer, count uint32, eventSize uintptr) (time.Duration, error) {
	start := time.Now()
	inserted, _, errno := syscall.SyscallN(procSendInput.Addr(), uintptr(count), uintptr(events), eventSize)
	elapsed := time.Since(start)
	if inserted != uintptr(count) {
		return elapsed, sendInputError(inserted, count, errno)
	}
	return elapsed, nil
}

func sendInputError(inserted uintptr, count uint32, errno syscall.Errno) error {
	if errno == 0 {
		return fmt.Errorf("SendInput inserted %d of %d events (blocked by UIPI or another thread)", inserted, count)
	}
	return fmt.Errorf("SendInput inserted %d of %d events: %w", inserted, count, errno)
}

func sendMouse(flags uint32, dx, dy int32, data uint32) (time.Duration, error) {
	ev := mouseEvent{
		Type: inputMouse,
		MI:   mouseInput{Dx: dx, Dy: dy, MouseData: data, Flags: flags},
	}
	return sendInputs(unsafe.Pointer(&ev), 1, unsafe.Sizeof(ev))
}

func sendKeyTap(scan uint16, extended bool) (time.Duration, error) {
	flags := uint32(keyeventfScanCode)
	if extended {
		flags |= keyeventfExtendedKey
	}
	events := [2]keyboardEvent{
		{Type: inputKeyboard, KI: keybdInput{Scan: scan, Flags: flags}},
		{Type: inputKeyboard, KI: keybdInput{Scan: scan, Flags: flags | keyeventfKeyUp}},
	}
	return sendInputs(unsafe.Pointer(&events[0]), uint32(len(events)), unsafe.Sizeof(events[0]))
}

func cursorPosition() (Point, error) {
	var p Point
	ok, _, errno := syscall.SyscallN(procGetCursorPos.Addr(), uintptr(unsafe.Pointer(&p)))
	if ok == 0 {
		return Point{}, callError("GetCursorPos", errno)
	}
	return p, nil
}
