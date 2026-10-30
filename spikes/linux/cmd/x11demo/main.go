//go:build linux

// Command x11demo exercises the X11 spike (P0-SP3-004) on a running X server: it lists RandR monitors and prints
// their change notifications, holds the pointer and prints warp-to-centre deltas, serves CLIPBOARD with INCR, or
// reads CLIPBOARD with INCR from another client.
package main

import (
	"context"
	"crypto/sha256"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"time"

	"github.com/kikz0r/edgesync/spikes/linux/x11"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "x11demo:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("x11demo", flag.ContinueOnError)
	display := fs.String("display", "", "X display (default $DISPLAY)")
	mode := fs.String("mode", "monitors", "monitors, grab, own or read")
	duration := fs.Duration("duration", 30*time.Second, "how long monitors, grab and own run")
	size := fs.Int("size", 5<<20, "payload size in bytes for own mode")
	selection := fs.String("selection", "CLIPBOARD", "selection name")
	target := fs.String("target", "UTF8_STRING", "selection target for own and read modes")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *duration <= 0 || *size <= 0 {
		return errors.New("-duration and -size must be positive")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, *duration)
	defer cancel()

	c, err := x11.Open(*display)
	if err != nil {
		return err
	}
	defer c.Close()

	switch *mode {
	case "monitors":
		return showMonitors(ctx, c)
	case "grab":
		return showGrab(ctx, c)
	case "own":
		return serveSelection(ctx, c, *selection, *target, *size)
	case "read":
		return readSelection(ctx, c, *selection, *target)
	default:
		return fmt.Errorf("unknown -mode %q", *mode)
	}
}

func showMonitors(ctx context.Context, c *x11.Conn) error {
	monitors, err := c.Monitors()
	if err != nil {
		return err
	}
	for _, m := range monitors {
		fmt.Printf("monitor %s primary=%t automatic=%t origin=%d,%d size=%dx%d mm=%dx%d outputs=%d\n",
			m.Name, m.Primary, m.Automatic, m.X, m.Y, m.Width, m.Height, m.WidthMM, m.HeightMM, m.OutputCount)
	}
	if err := c.WatchMonitorChanges(); err != nil {
		return err
	}
	for {
		ev, err := c.WaitEvent(ctx, nil)
		if err != nil {
			return ignoreContextEnd(ctx, err)
		}
		fmt.Println("event:", ev.String())
	}
}

func showGrab(ctx context.Context, c *x11.Conn) error {
	capture, err := c.StartCapture()
	if err != nil {
		return err
	}
	var totalX, totalY int32
	err = capture.Deltas(ctx, func(d x11.Delta) {
		totalX += d.DX
		totalY += d.DY
		fmt.Printf("delta %d,%d total %d,%d\n", d.DX, d.DY, totalX, totalY)
	})
	return ignoreContextEnd(ctx, err)
}

func serveSelection(ctx context.Context, c *x11.Conn, selection, target string, size int) error {
	payload := make([]byte, size)
	for i := range payload {
		payload[i] = byte(i*131 + i>>8)
	}
	owner, err := c.Own(selection, []x11.Payload{{Target: target, Data: payload}})
	if err != nil {
		return err
	}
	fmt.Printf("serving %d bytes on %s as %s\n", size, selection, target)
	return ignoreContextEnd(ctx, owner.Serve(ctx))
}

func readSelection(ctx context.Context, c *x11.Conn, selection, target string) error {
	transfer, err := c.ReadSelection(ctx, selection, target)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(transfer.Data)
	fmt.Printf("read %d bytes incremental=%t sha256=%x\n", len(transfer.Data), transfer.Incremental, sum)
	return nil
}

// ignoreContextEnd treats the end of the -duration or an interrupt as the normal end of a mode.
func ignoreContextEnd(ctx context.Context, err error) error {
	if ctx.Err() != nil && errors.Is(err, ctx.Err()) {
		return nil
	}
	return err
}
