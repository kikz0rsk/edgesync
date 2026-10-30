//go:build darwin

// Command inject posts a move, a left click, a shifted key press and a pixel scroll at the centre of the main
// display through CGEventPost, then prints the TCC state and the cursor read-back.
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/kikz0r/edgesync/spikes/macos/fw"
	"github.com/kikz0r/edgesync/spikes/macos/inject"
	"github.com/kikz0r/edgesync/spikes/macos/tap"
)

const userData int64 = 0x45444753

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "inject:", err)
		os.Exit(1)
	}
}

func run() error {
	location := flag.String("location", "hid", "event tap location to post at: hid or session")
	flag.Parse()

	var loc inject.Location
	switch *location {
	case "hid":
		loc = inject.LocationHID
	case "session":
		loc = inject.LocationSession
	default:
		return fmt.Errorf("location %q: want hid or session", *location)
	}

	lib, err := fw.Load()
	if err != nil {
		return err
	}
	perms, err := tap.NewPermissions(lib)
	if err != nil {
		return err
	}
	fmt.Printf("TCC state: %s\n", perms.Preflight())

	centre, err := mainDisplayCentre(lib)
	if err != nil {
		return err
	}
	inj, err := inject.New(lib, inject.Config{Location: loc, UserData: userData})
	if err != nil {
		return err
	}
	defer inj.Close()

	if err := inj.Move(centre, 3, -2); err != nil {
		return err
	}
	if err := inj.Button(centre, inject.ButtonLeft, true, 1); err != nil {
		return err
	}
	if err := inj.Button(centre, inject.ButtonLeft, false, 1); err != nil {
		return err
	}
	if err := inj.Key(inject.KeyA, true, inject.FlagShift); err != nil {
		return err
	}
	if err := inj.Key(inject.KeyA, false, inject.FlagShift); err != nil {
		return err
	}
	if err := inj.Scroll(inject.ScrollUnitPixel, 0, -12); err != nil {
		return err
	}

	cursor, err := lib.CoreGraphics.CursorLocation(lib.CoreFoundation)
	if err != nil {
		return err
	}
	fmt.Printf("posted at %s, cursor=(%g,%g) target=(%g,%g)\n", *location, cursor.X, cursor.Y, centre.X, centre.Y)
	return nil
}

func mainDisplayCentre(lib *fw.Lib) (fw.CGPoint, error) {
	displays, err := lib.CoreGraphics.ActiveDisplays()
	if err != nil {
		return fw.CGPoint{}, err
	}
	for _, d := range displays {
		if d.Main {
			return fw.CGPoint{
				X: d.Bounds.Origin.X + d.Bounds.Size.Width/2,
				Y: d.Bounds.Origin.Y + d.Bounds.Size.Height/2,
			}, nil
		}
	}
	return fw.CGPoint{}, errors.New("main display is not among the active displays")
}
