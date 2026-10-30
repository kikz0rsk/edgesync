package main

import (
	"errors"
	"fmt"
	"log/slog"
	"time"
	"unsafe"
)

const (
	inputMouse    = 0
	inputKeyboard = 1

	mouseeventfMove        = 0x0001
	mouseeventfVirtualDesk = 0x4000
	mouseeventfAbsolute    = 0x8000

	keyeventfKeyUp    = 0x0002
	keyeventfScanCode = 0x0008

	smXVirtualScreen  = 76
	smYVirtualScreen  = 77
	smCXVirtualScreen = 78
	smCYVirtualScreen = 79

	inputStructSize = 40
	squareSize      = 200
	stepsPerSide    = 10
	stepDelay       = 10 * time.Millisecond
	keyDelay        = 15 * time.Millisecond

	scanEscape = 0x01
)

var (
	ErrInputLayout   = errors.New("INPUT structure layout differs from the Windows 64-bit ABI")
	ErrInputRejected = errors.New("SendInput injected fewer events than requested (UIPI or desktop denial gives no error code)")

	typedText = []uint16{0x12, 0x20, 0x22, 0x12, 0x1F, 0x15, 0x31, 0x2E}
)

type mouseInput struct {
	dx        int32
	dy        int32
	mouseData uint32
	flags     uint32
	time      uint32
	extraInfo uintptr
}

type keybdInput struct {
	vk        uint16
	scan      uint16
	flags     uint32
	time      uint32
	extraInfo uintptr
}

type mouseEvent struct {
	kind uint32
	_    uint32
	mi   mouseInput
}

type keyEvent struct {
	kind uint32
	_    uint32
	ki   keybdInput
	_    [8]byte
}

func verifyInputLayout() error {
	if unsafe.Sizeof(uintptr(0)) != 8 {
		return fmt.Errorf("%w: only 64-bit builds are supported", ErrInputLayout)
	}
	if got := unsafe.Sizeof(mouseEvent{}); got != inputStructSize {
		return fmt.Errorf("%w: mouse INPUT is %d bytes, want %d", ErrInputLayout, got, inputStructSize)
	}
	if got := unsafe.Sizeof(keyEvent{}); got != inputStructSize {
		return fmt.Errorf("%w: keyboard INPUT is %d bytes, want %d", ErrInputLayout, got, inputStructSize)
	}
	return nil
}

func sendOne(event unsafe.Pointer) error {
	injected, _, err := procSendInput.Call(1, uintptr(event), inputStructSize)
	if injected != 1 {
		return fmt.Errorf("%w: %w", ErrInputRejected, err)
	}
	return nil
}

func virtualScreen() (screenRect, error) {
	metric := func(index uintptr) int32 {
		value, _, _ := procGetSystemMetrics.Call(index)
		return int32(value)
	}
	screen := screenRect{
		left:   metric(smXVirtualScreen),
		top:    metric(smYVirtualScreen),
		width:  metric(smCXVirtualScreen),
		height: metric(smCYVirtualScreen),
	}
	if screen.width <= 0 || screen.height <= 0 {
		return screen, fmt.Errorf("GetSystemMetrics virtual screen is %dx%d", screen.width, screen.height)
	}
	return screen, nil
}

func cursorPos() (point, error) {
	var p point
	if r1, _, err := procGetCursorPos.Call(uintptr(unsafe.Pointer(&p))); r1 == 0 {
		return point{}, fmt.Errorf("GetCursorPos: %w", err)
	}
	return p, nil
}

func moveAbsolute(p point, screen screenRect) error {
	nx, ny, err := normalizeAbsolute(p, screen)
	if err != nil {
		return err
	}
	event := mouseEvent{kind: inputMouse, mi: mouseInput{
		dx: nx, dy: ny, flags: mouseeventfMove | mouseeventfAbsolute | mouseeventfVirtualDesk,
	}}
	return sendOne(unsafe.Pointer(&event))
}

func pressScan(scan uint16, flags uint32) error {
	event := keyEvent{kind: inputKeyboard, ki: keybdInput{scan: scan, flags: keyeventfScanCode | flags}}
	return sendOne(unsafe.Pointer(&event))
}

func tapScan(scan uint16) error {
	if err := pressScan(scan, 0); err != nil {
		return fmt.Errorf("scan %#x down: %w", scan, err)
	}
	time.Sleep(keyDelay)
	if err := pressScan(scan, keyeventfKeyUp); err != nil {
		return fmt.Errorf("scan %#x up: %w", scan, err)
	}
	time.Sleep(keyDelay)
	return nil
}

// injectDemo runs on the desktop thread, so the events go to whichever desktop the follower has attached to it. The
// pointer square ends back at its start point; Escape cancels a UAC prompt.
func injectDemo(log *slog.Logger, desktop string) error {
	screen, err := virtualScreen()
	if err != nil {
		return err
	}
	start, err := cursorPos()
	if err != nil {
		return err
	}
	path, err := squarePath(start, squareSize, screen, stepsPerSide)
	if err != nil {
		return err
	}
	for _, p := range path {
		if err := moveAbsolute(p, screen); err != nil {
			return fmt.Errorf("square move to (%d,%d): %w", p.x, p.y, err)
		}
		time.Sleep(stepDelay)
	}
	end, err := cursorPos()
	if err != nil {
		return err
	}
	for _, scan := range typedText {
		if err := tapScan(scan); err != nil {
			return err
		}
	}
	if err := tapScan(scanEscape); err != nil {
		return err
	}
	log.Info("injected demo square, text and Escape",
		"desktop", desktop,
		"start", fmt.Sprintf("%d,%d", start.x, start.y),
		"endReadBack", fmt.Sprintf("%d,%d", end.x, end.y),
		"squareSize", squareSize,
	)
	return nil
}
