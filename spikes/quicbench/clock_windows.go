//go:build windows

package quicbench

import (
	"fmt"
	"syscall"
	"unsafe"
)

// Go's time.Now and time.Since on Windows advance only on timer ticks (about 0.5 ms here), too coarse for
// sub-millisecond latency, so the bench reads QueryPerformanceCounter directly (D-123).
var (
	kernel32                      = syscall.NewLazyDLL("kernel32.dll")
	procQueryPerformanceCounter   = kernel32.NewProc("QueryPerformanceCounter")
	procQueryPerformanceFrequency = kernel32.NewProc("QueryPerformanceFrequency")
)

func counterHz() (int64, error) {
	var hz int64
	ok, _, errno := syscall.SyscallN(procQueryPerformanceFrequency.Addr(), uintptr(unsafe.Pointer(&hz)))
	if ok == 0 {
		return 0, fmt.Errorf("QueryPerformanceFrequency failed: errno %d", errno)
	}
	if hz <= 0 {
		return 0, fmt.Errorf("QueryPerformanceFrequency returned %d", hz)
	}
	return hz, nil
}

// QueryPerformanceCounter is documented to always succeed on Windows XP and later, so the per-read BOOL is not checked.
func rawCounter() int64 {
	var c int64
	syscall.SyscallN(procQueryPerformanceCounter.Addr(), uintptr(unsafe.Pointer(&c)))
	return c
}
