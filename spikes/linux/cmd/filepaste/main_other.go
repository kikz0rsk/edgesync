//go:build !linux

package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "filepaste: Linux only (X11 and the Clipboard portal)")
	os.Exit(2)
}
