//go:build !(linux && cgo)

// Command giodroplinux probes file drops onto a Gio window on Linux. It is built on the GNOME box with cgo; on every
// other target this file keeps the package buildable and exits with an explanation.
package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "giodroplinux: Linux with cgo only (Gio needs cgo on Linux, see the README)")
	os.Exit(2)
}
