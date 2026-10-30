package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"gioui.org/app"
	"gioui.org/op"
	"gioui.org/unit"
)

var errMachineCount = errors.New("-machines must be at least 1")

type frameLog struct {
	out      io.Writer
	machines int
	started  time.Time
	frames   int
	total    time.Duration
	slowest  time.Duration
}

func (f *frameLog) Record(now time.Time, frameTime time.Duration) {
	if f.started.IsZero() {
		f.started = now
	}
	f.frames++
	f.total += frameTime
	f.slowest = max(f.slowest, frameTime)

	elapsed := now.Sub(f.started)
	if elapsed < time.Second {
		return
	}
	fmt.Fprintf(f.out, "machines=%d frames=%d fps=%.1f frame_avg=%.2fms frame_max=%.2fms\n",
		f.machines,
		f.frames,
		float64(f.frames)/elapsed.Seconds(),
		float64(f.total)/float64(f.frames)/float64(time.Millisecond),
		float64(f.slowest)/float64(time.Millisecond),
	)
	f.started = now
	f.frames = 0
	f.total = 0
	f.slowest = 0
}

func run(window *app.Window, machineCount int) error {
	canvas := NewCanvas(machineCount)
	stats := &frameLog{out: os.Stdout, machines: canvas.MachineCount()}
	var ops op.Ops
	for {
		switch e := window.Event().(type) {
		case app.DestroyEvent:
			return e.Err
		case app.FrameEvent:
			begin := time.Now()
			gtx := app.NewContext(&ops, e)
			canvas.Layout(gtx)
			e.Frame(gtx.Ops)
			stats.Record(time.Now(), time.Since(begin))
		}
	}
}

func main() {
	machineCount := flag.Int("machines", 4, "number of synthetic machines (2-3 monitors each)")
	flag.Parse()
	if *machineCount < 1 {
		fmt.Fprintln(os.Stderr, errMachineCount)
		os.Exit(2)
	}

	go func() {
		window := new(app.Window)
		window.Option(app.Title("EdgeSync layout editor spike"), app.Size(unit.Dp(1280), unit.Dp(800)))
		if err := run(window, *machineCount); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		os.Exit(0)
	}()
	app.Main()
}
