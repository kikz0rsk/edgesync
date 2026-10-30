//go:build darwin

// Command displays prints every active display with its bounds in points, its pixel size and its scale.
package main

import (
	"fmt"
	"os"

	"github.com/kikz0r/edgesync/spikes/macos/fw"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "displays:", err)
		os.Exit(1)
	}
}

func run() error {
	lib, err := fw.Load()
	if err != nil {
		return err
	}
	displays, err := lib.CoreGraphics.ActiveDisplays()
	if err != nil {
		return err
	}
	for _, d := range displays {
		fmt.Printf("display=%d main=%t origin=(%g,%g) size=%gx%g points pixels=%dx%d scale=%g\n",
			d.ID, d.Main,
			d.Bounds.Origin.X, d.Bounds.Origin.Y,
			d.Bounds.Size.Width, d.Bounds.Size.Height,
			d.PixelsWide, d.PixelsHigh, d.Scale)
	}
	return nil
}
