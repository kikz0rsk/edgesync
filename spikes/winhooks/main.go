//go:build windows

// Command winhooks measures low-level mouse and keyboard hooks on Windows under optional GC stress: callback
// duration percentiles, event rate, ring drops, watchdog re-installs and Raw Input deltas against hook deltas at
// the virtual screen edges.
package main

import (
	"context"
	"encoding/csv"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"strconv"
	"sync"
	"time"

	"github.com/kikz0r/edgesync/spikes/internal/stats"
	"github.com/kikz0r/edgesync/spikes/internal/winhook"
)

const (
	stallDuration       = 1500 * time.Millisecond
	gcStressWorkers     = 8
	gcStressBytesPerSec = 64 << 20
	gcStressRetained    = 64
	gcStressInterval    = 100 * time.Millisecond
	drainInterval       = time.Millisecond
	pageSize            = 4096
)

type config struct {
	duration time.Duration
	gcStress bool
	stall    bool
	csvPath  string
}

func parseFlags(args []string) (config, error) {
	var cfg config
	fs := flag.NewFlagSet("winhooks", flag.ContinueOnError)
	fs.DurationVar(&cfg.duration, "duration", 60*time.Second, "run time")
	fs.BoolVar(&cfg.gcStress, "gcstress", false, "run 8 allocating goroutines (64 MiB/s) and runtime.GC() every 100 ms")
	fs.BoolVar(&cfg.stall, "stall", false, "block one hook callback for 1500 ms to force hook removal")
	fs.StringVar(&cfg.csvPath, "csv", "", "write one CSV row per second to this file")
	if err := fs.Parse(args); err != nil {
		return config{}, err
	}
	if cfg.duration <= 0 {
		return config{}, fmt.Errorf("-duration must be positive, got %v", cfg.duration)
	}
	return cfg, nil
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "winhooks:", err)
		os.Exit(1)
	}
}

func run(args []string) (err error) {
	cfg, err := parseFlags(args)
	if err != nil {
		return err
	}
	if err := winhook.SetPerMonitorV2(); err != nil {
		return err
	}
	screen, err := winhook.VirtualScreen()
	if err != nil {
		return err
	}
	ring, err := winhook.NewRing(winhook.DefaultRingCapacity)
	if err != nil {
		return err
	}

	opts := winhook.Options{}
	if cfg.stall {
		opts.StallOnce = stallDuration
	}
	hook := winhook.New(opts)

	var rows *csvReport
	if cfg.csvPath != "" {
		if rows, err = newCSVReport(cfg.csvPath); err != nil {
			return err
		}
		defer func() { err = errors.Join(err, rows.close()) }()
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, cfg.duration)
	defer cancel()

	fmt.Printf("virtual screen: x=%d y=%d w=%d h=%d  ring=%d  GOMAXPROCS=%d  gcstress=%v  stall=%v\n",
		screen.X, screen.Y, screen.W, screen.H, ring.Capacity(), runtime.GOMAXPROCS(0), cfg.gcStress, cfg.stall)
	fmt.Println("Scroll Lock toggles suppression (keyboard and mouse blocked while ON; Scroll Lock itself always passes)")
	fmt.Println("push the pointer against each screen edge while this runs")

	if err := hook.Start(ctx, ring); err != nil {
		return err
	}

	var workers sync.WaitGroup
	if cfg.gcStress {
		startGCStress(ctx, &workers)
	}

	col := newCollector(screen)
	started := time.Now()
	loop(ctx, hook, ring, col, rows, started)
	cancel()
	workers.Wait()
	if err := hook.Close(); err != nil {
		return err
	}
	drain(ring, col)
	printSummary(hook, col, time.Since(started))
	return nil
}

func loop(ctx context.Context, hook *winhook.Hook, ring *winhook.Ring, col *collector, rows *csvReport, started time.Time) {
	drainTicker := time.NewTicker(drainInterval)
	defer drainTicker.Stop()
	reportTicker := time.NewTicker(time.Second)
	defer reportTicker.Stop()

	var previousEvents uint64
	for {
		select {
		case <-ctx.Done():
			return
		case <-hook.Done():
			return
		case <-drainTicker.C:
			drain(ring, col)
		case <-reportTicker.C:
			drain(ring, col)
			current := hook.Stats()
			line := newReportLine(time.Since(started), current, current.Events-previousEvents, hook.Callback(), hook.Suppressed())
			previousEvents = current.Events
			fmt.Println(line.text())
			if rows != nil {
				rows.write(line)
			}
		}
	}
}

func drain(ring *winhook.Ring, col *collector) {
	for {
		ev, ok := ring.Pop()
		if !ok {
			return
		}
		col.consume(ev)
	}
}

func startGCStress(ctx context.Context, workers *sync.WaitGroup) {
	perWorker := gcStressBytesPerSec / gcStressWorkers
	for range gcStressWorkers {
		workers.Go(func() { allocateAtRate(ctx, perWorker) })
	}
	workers.Go(func() {
		ticker := time.NewTicker(gcStressInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				runtime.GC()
			}
		}
	})
}

func allocateAtRate(ctx context.Context, bytesPerSecond int) {
	const chunk = 64 << 10
	retained := make([][]byte, gcStressRetained)
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	started := time.Now()
	var allocated, next int
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		owed := int(float64(bytesPerSecond) * time.Since(started).Seconds())
		for allocated < owed {
			block := make([]byte, chunk)
			for i := 0; i < len(block); i += pageSize {
				block[i] = 1
			}
			retained[next%len(retained)] = block
			next++
			allocated += chunk
		}
	}
}

type reportLine struct {
	elapsed    time.Duration
	stats      winhook.Stats
	eventsPerS uint64
	p50, p99   string
	max        time.Duration
	suppress   bool
}

func newReportLine(elapsed time.Duration, current winhook.Stats, perSecond uint64, hist *stats.Histogram, suppress bool) reportLine {
	return reportLine{
		elapsed:    elapsed,
		stats:      current,
		eventsPerS: perSecond,
		p50:        percentile(hist, 50),
		p99:        percentile(hist, 99),
		max:        hist.Max(),
		suppress:   suppress,
	}
}

func (l reportLine) text() string {
	return fmt.Sprintf("t=%4.0fs ev/s=%-5d events=%-7d drops=%-4d reinstalls=%d markers(mouse/key)=%d/%d raw=%d cb p50=%s p99=%s max=%v suppress=%v",
		l.elapsed.Seconds(), l.eventsPerS, l.stats.Events, l.stats.Drops, l.stats.Reinstalls,
		l.stats.MarkerMouse, l.stats.MarkerKey, l.stats.RawEvents, l.p50, l.p99, l.max, onOff(l.suppress))
}

func onOff(v bool) string {
	if v {
		return "ON"
	}
	return "off"
}

func percentile(hist *stats.Histogram, p float64) string {
	value, err := hist.Percentile(p)
	if errors.Is(err, stats.ErrEmpty) {
		return "n/a"
	}
	if err != nil {
		return err.Error()
	}
	return value.String()
}

type csvReport struct {
	file   *os.File
	writer *csv.Writer
	err    error
}

func newCSVReport(path string) (*csvReport, error) {
	file, err := os.Create(path)
	if err != nil {
		return nil, fmt.Errorf("create csv: %w", err)
	}
	report := &csvReport{file: file, writer: csv.NewWriter(file)}
	report.writeRecord([]string{
		"t_s", "events", "events_per_s", "drops", "reinstalls", "stale_unhooks", "marker_mouse", "marker_key",
		"raw_events", "raw_non_relative", "raw_errors", "cb_p50", "cb_p99", "cb_max_us", "suppress",
	})
	return report, report.err
}

func (r *csvReport) write(l reportLine) {
	u := func(v uint64) string { return strconv.FormatUint(v, 10) }
	r.writeRecord([]string{
		strconv.FormatFloat(l.elapsed.Seconds(), 'f', 1, 64), u(l.stats.Events), u(l.eventsPerS), u(l.stats.Drops),
		u(l.stats.Reinstalls), u(l.stats.StaleUnhooks), u(l.stats.MarkerMouse), u(l.stats.MarkerKey),
		u(l.stats.RawEvents), u(l.stats.RawNonRelative), u(l.stats.RawErrors), l.p50, l.p99,
		strconv.FormatInt(l.max.Microseconds(), 10), onOff(l.suppress),
	})
}

func (r *csvReport) writeRecord(record []string) {
	if r.err == nil {
		r.err = r.writer.Write(record)
	}
}

func (r *csvReport) close() error {
	r.writer.Flush()
	return errors.Join(r.err, r.writer.Error(), r.file.Close())
}

type axisSums struct {
	dx, dy int64
}

func (s *axisSums) add(dx, dy int32) {
	s.dx += int64(dx)
	s.dy += int64(dy)
}

const (
	edgeLeft = iota
	edgeRight
	edgeTop
	edgeBottom
	edgeCount
)

var edgeNames = [edgeCount]string{"left", "right", "top", "bottom"}

type collector struct {
	screen        winhook.Rect
	havePoint     bool
	lastX, lastY  int32
	mouse, keys   uint64
	raw           uint64
	hookTotal     axisSums
	rawTotal      axisSums
	hookAtEdge    [edgeCount]axisSums
	rawAtEdge     [edgeCount]axisSums
	hookEdgeMoves [edgeCount]uint64
}

func newCollector(screen winhook.Rect) *collector {
	return &collector{screen: screen}
}

func (c *collector) edgesAt(x, y int32) [edgeCount]bool {
	return [edgeCount]bool{
		edgeLeft:   x <= c.screen.X,
		edgeRight:  x >= c.screen.X+c.screen.W-1,
		edgeTop:    y <= c.screen.Y,
		edgeBottom: y >= c.screen.Y+c.screen.H-1,
	}
}

func (c *collector) consume(ev winhook.Event) {
	switch ev.Kind {
	case winhook.KindMouse:
		c.mouse++
		if c.havePoint && ev.Message == winhook.WMMouseMove && ev.Flags&winhook.LLMHFInjected == 0 {
			dx, dy := ev.X-c.lastX, ev.Y-c.lastY
			c.hookTotal.add(dx, dy)
			for edge, touching := range c.edgesAt(ev.X, ev.Y) {
				if touching {
					c.hookAtEdge[edge].add(dx, dy)
					c.hookEdgeMoves[edge]++
				}
			}
		}
		c.lastX, c.lastY, c.havePoint = ev.X, ev.Y, true
	case winhook.KindRawMouse:
		c.raw++
		c.rawTotal.add(ev.X, ev.Y)
		if c.havePoint {
			for edge, touching := range c.edgesAt(c.lastX, c.lastY) {
				if touching {
					c.rawAtEdge[edge].add(ev.X, ev.Y)
				}
			}
		}
	case winhook.KindKey:
		c.keys++
	}
}

func printSummary(hook *winhook.Hook, col *collector, elapsed time.Duration) {
	current := hook.Stats()
	hist := hook.Callback()
	mean := "n/a"
	if value, err := hist.Mean(); err == nil {
		mean = value.String()
	}

	fmt.Println()
	fmt.Println("=== summary ===")
	fmt.Printf("elapsed: %v  events: %d (%.1f/s)  mouse=%d key=%d raw=%d\n",
		elapsed.Round(time.Millisecond), current.Events, float64(current.Events)/elapsed.Seconds(), col.mouse, col.keys, col.raw)
	fmt.Printf("callback: count=%d p50=%s p99=%s max=%v mean=%s\n",
		hist.Count(), percentile(hist, 50), percentile(hist, 99), current.MaxCallback, mean)
	fmt.Printf("ring drops: %d\n", current.Drops)
	fmt.Printf("watchdog: reinstalls=%d stale_unhooks=%d marker_seen mouse=%d key=%d\n",
		current.Reinstalls, current.StaleUnhooks, current.MarkerMouse, current.MarkerKey)
	fmt.Printf("raw input: events=%d non_relative=%d errors=%d\n", current.RawEvents, current.RawNonRelative, current.RawErrors)
	fmt.Println("delta sums (px): hook = sum of pt deltas of non-injected WM_MOUSEMOVE, raw = sum of lLastX/lLastY")
	fmt.Printf("%-8s %12s %12s %12s %12s %10s\n", "where", "hook_dx", "hook_dy", "raw_dx", "raw_dy", "hook_moves")
	fmt.Printf("%-8s %12d %12d %12d %12d %10s\n", "total", col.hookTotal.dx, col.hookTotal.dy, col.rawTotal.dx, col.rawTotal.dy, "-")
	for edge, name := range edgeNames {
		fmt.Printf("%-8s %12d %12d %12d %12d %10d\n", name,
			col.hookAtEdge[edge].dx, col.hookAtEdge[edge].dy, col.rawAtEdge[edge].dx, col.rawAtEdge[edge].dy, col.hookEdgeMoves[edge])
	}
}
