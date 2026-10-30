//go:build darwin

// Command tap installs a CGEventTap on a dedicated CFRunLoop thread and prints the permission state, every
// event when asked, and the tap counters once per second.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"time"

	"github.com/kikz0r/edgesync/spikes/macos/fw"
	"github.com/kikz0r/edgesync/spikes/macos/tap"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "tap:", err)
		os.Exit(1)
	}
}

func run() error {
	location := flag.String("location", "session", "tap location: session, hid or annotated")
	listenOnly := flag.Bool("listen-only", false, "create a listen-only tap instead of an active one")
	suppress := flag.Bool("suppress", false, "consume every event (active tap only)")
	duration := flag.Duration("duration", 10*time.Second, "how long to keep the tap installed")
	prompt := flag.Bool("prompt", false, "request Accessibility, listen and post access, which may show system dialogs")
	printEvents := flag.Bool("events", false, "print every received event")
	flag.Parse()

	loc, err := parseLocation(*location)
	if err != nil {
		return err
	}
	lib, err := fw.Load()
	if err != nil {
		return err
	}
	perms, err := tap.NewPermissions(lib)
	if err != nil {
		return err
	}
	fmt.Println("before:", perms.Preflight())
	if *prompt {
		trusted, err := perms.AccessibilityTrustedPrompting()
		if err != nil {
			return err
		}
		fmt.Printf("AXIsProcessTrustedWithOptions(prompt)=%t CGRequestListenEventAccess=%t CGRequestPostEventAccess=%t\n",
			trusted, perms.RequestListenAccess(), perms.RequestPostAccess())
		fmt.Println("after:", perms.Preflight())
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, *duration)
	defer cancel()

	t, err := tap.Start(ctx, lib, tap.Config{Location: loc, ListenOnly: *listenOnly, Buffer: 1024})
	if err != nil {
		return err
	}
	defer t.Close()
	if err := t.SetSuppress(*suppress); err != nil {
		return err
	}
	fmt.Printf("tap installed: location=%s listenOnly=%t suppress=%t\n", *location, *listenOnly, *suppress)

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case e := <-t.Events():
			if *printEvents {
				fmt.Printf("event type=%d location=(%g,%g) delta=(%d,%d) keycode=%d userData=%#x\n",
					e.Type, e.Location.X, e.Location.Y, e.DeltaX, e.DeltaY, e.Keycode, e.UserData)
			}
		case <-ticker.C:
			fmt.Printf("stats: %+v enabled=%t\n", t.Stats(), t.Enabled())
		case <-ctx.Done():
			fmt.Printf("final stats: %+v\n", t.Stats())
			return nil
		}
	}
}

func parseLocation(name string) (tap.Location, error) {
	switch name {
	case "session":
		return tap.LocationSession, nil
	case "hid":
		return tap.LocationHID, nil
	case "annotated":
		return tap.LocationAnnotatedSession, nil
	}
	return 0, fmt.Errorf("unknown location %q: %w", name, tap.ErrInvalidLocation)
}
