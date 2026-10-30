// Command wininject measures Windows injection on the controlled machine: it reports the monitor topology, per-monitor
// DPI and the real edges (D-51, D-89), moves the pointer with absolute SendInput to every monitor probe and reads the
// position back, types a scancode pattern including extended keys, and injects wheel and X-button events. Run by the
// user only (D-74); agents build it and never execute it.
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"time"
)

const (
	settleDelay = 50 * time.Millisecond
	keyGap      = 20 * time.Millisecond
)

type keyStep struct {
	name     string
	scan     uint16
	extended bool
}

type wheelStep struct {
	name  string
	flags uint32
	data  uint32
}

type latency struct {
	count    int
	min, max time.Duration
	total    time.Duration
}

func (l *latency) add(d time.Duration) {
	if l.count == 0 || d < l.min {
		l.min = d
	}
	if d > l.max {
		l.max = d
	}
	l.total += d
	l.count++
}

func (l latency) String() string {
	if l.count == 0 {
		return "SendInput latency: no calls"
	}
	return fmt.Sprintf("SendInput calls=%d min=%s mean=%s max=%s",
		l.count, l.min, l.total/time.Duration(l.count), l.max)
}

func typingPattern() []keyStep {
	return []keyStep{
		{"e", 0x12, false},
		{"d", 0x20, false},
		{"g", 0x22, false},
		{"e", 0x12, false},
		{"s", 0x1F, false},
		{"y", 0x15, false},
		{"n", 0x31, false},
		{"c", 0x2E, false},
		{"space", 0x39, false},
		{"1", 0x02, false},
		{"2", 0x03, false},
		{"3", 0x04, false},
		{"4", 0x05, false},
		{"5", 0x06, false},
		{"6", 0x07, false},
		{"7", 0x08, false},
		{"8", 0x09, false},
		{"9", 0x0A, false},
		{"0", 0x0B, false},
		{"numpad enter", 0x1C, true},
		{"right ctrl", 0x1D, true},
		{"left", 0x4B, true},
		{"right", 0x4D, true},
		{"up", 0x48, true},
		{"down", 0x50, true},
		{"enter", 0x1C, false},
	}
}

func wheelDelta(v int32) uint32 {
	return uint32(v)
}

func wheelSteps() []wheelStep {
	return []wheelStep{
		{"wheel +120", mouseeventfWheel, wheelDelta(120)},
		{"wheel -120", mouseeventfWheel, wheelDelta(-120)},
		{"wheel +30", mouseeventfWheel, wheelDelta(30)},
		{"wheel -30", mouseeventfWheel, wheelDelta(-30)},
		{"hwheel +120", mouseeventfHWheel, wheelDelta(120)},
		{"hwheel -120", mouseeventfHWheel, wheelDelta(-120)},
		{"hwheel +30", mouseeventfHWheel, wheelDelta(30)},
		{"hwheel -30", mouseeventfHWheel, wheelDelta(-30)},
		{"xbutton1 down", mouseeventfXDown, xbutton1},
		{"xbutton1 up", mouseeventfXUp, xbutton1},
		{"xbutton2 down", mouseeventfXDown, xbutton2},
		{"xbutton2 up", mouseeventfXUp, xbutton2},
	}
}

func rectString(r Rect) string {
	return fmt.Sprintf("(%d,%d %dx%d)", r.X, r.Y, r.W, r.H)
}

func boundingBox(rects []Rect) Rect {
	left, top := rects[0].X, rects[0].Y
	right, bottom := rects[0].Right(), rects[0].Bottom()
	for _, r := range rects[1:] {
		left = min(left, r.X)
		top = min(top, r.Y)
		right = max(right, r.Right())
		bottom = max(bottom, r.Bottom())
	}
	return Rect{X: left, Y: top, W: right - left, H: bottom - top}
}

func abs32(v int32) int32 {
	if v < 0 {
		return -v
	}
	return v
}

func reportTopology() ([]monitorInfo, Rect, error) {
	awareness, err := processDPIAwareness()
	if err != nil {
		return nil, Rect{}, err
	}
	fmt.Printf("process DPI awareness: %d (0 unaware, 1 system, 2 per-monitor)\n", awareness)
	fmt.Printf("thread is DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2: %t\n", threadIsPerMonitorV2())

	desk, err := virtualDesktopMetrics()
	if err != nil {
		return nil, Rect{}, err
	}
	monitors, err := enumerateMonitors()
	if err != nil {
		return nil, Rect{}, err
	}
	if len(monitors) == 0 {
		return nil, Rect{}, errors.New("EnumDisplayMonitors reported no monitors")
	}
	rects := make([]Rect, len(monitors))
	for i, m := range monitors {
		rects[i] = m.Bounds
		fmt.Printf("monitor %d %s primary=%t bounds=%s work=%s dpi=%d scale=%d%%\n",
			i, m.Name, m.Primary, rectString(m.Bounds), rectString(m.Work), m.DPI, m.DPI*100/96)
	}
	fmt.Printf("virtual desktop (GetSystemMetrics, used for normalization): %s\n", rectString(desk))
	fmt.Printf("bounding box of monitors (GetMonitorInfoW): %s\n", rectString(boundingBox(rects)))

	segments, err := RealEdges(rects)
	if err != nil {
		return nil, Rect{}, err
	}
	for _, s := range segments {
		fmt.Printf("real edge monitor %d %-6s at=%d from=%d to=%d\n", s.Monitor, s.Side, s.At, s.From, s.To)
	}
	return monitors, desk, nil
}

func absoluteProbes(desk Rect, monitors []monitorInfo) error {
	const flags = mouseeventfMove | mouseeventfAbsolute | mouseeventfVirtualDesk
	probeNames := [...]string{"corner-tl", "corner-tr", "corner-bl", "corner-br", "centre"}
	var lat latency
	var worst int32
	for i, m := range monitors {
		probes, err := Probes(m.Bounds)
		if err != nil {
			return fmt.Errorf("monitor %d: %w", i, err)
		}
		for k, p := range probes {
			n, err := Normalize(desk, p)
			if err != nil {
				return fmt.Errorf("monitor %d %s: %w", i, probeNames[k], err)
			}
			d, err := sendMouse(flags, n.X, n.Y, 0)
			if err != nil {
				return fmt.Errorf("monitor %d %s: %w", i, probeNames[k], err)
			}
			lat.add(d)
			time.Sleep(settleDelay)
			got, err := cursorPosition()
			if err != nil {
				return fmt.Errorf("monitor %d %s: %w", i, probeNames[k], err)
			}
			ex, ey := got.X-p.X, got.Y-p.Y
			worst = max(worst, abs32(ex), abs32(ey))
			fmt.Printf("abs monitor %d %-9s target=(%d,%d) normalized=(%d,%d) read=(%d,%d) error=(%d,%d)px send=%s\n",
				i, probeNames[k], p.X, p.Y, n.X, n.Y, got.X, got.Y, ex, ey, d)
		}
	}
	fmt.Printf("absolute: worst |error| = %d px; %s\n", worst, lat)
	return nil
}

func typeScancodes(countdown time.Duration) error {
	fmt.Printf("typing starts in %s: focus Notepad and keep the pointer over its text area\n", countdown)
	time.Sleep(countdown)
	var lat latency
	for _, s := range typingPattern() {
		d, err := sendKeyTap(s.scan, s.extended)
		if err != nil {
			return fmt.Errorf("key %s: %w", s.name, err)
		}
		lat.add(d)
		fmt.Printf("key %-13s scan=0x%02X extended=%-5t send=%s\n", s.name, s.scan, s.extended, d)
		time.Sleep(keyGap)
	}
	fmt.Printf("typing: %s\n", lat)
	return nil
}

func wheelAndButtons() error {
	var lat latency
	for _, s := range wheelSteps() {
		d, err := sendMouse(s.flags, 0, 0, s.data)
		if err != nil {
			return fmt.Errorf("%s: %w", s.name, err)
		}
		lat.add(d)
		fmt.Printf("%-14s data=0x%08X send=%s\n", s.name, s.data, d)
	}
	fmt.Printf("wheel and buttons: %s\n", lat)
	return nil
}

func run(countdown time.Duration) error {
	monitors, desk, err := reportTopology()
	if err != nil {
		return err
	}
	if err := absoluteProbes(desk, monitors); err != nil {
		return err
	}
	if err := typeScancodes(countdown); err != nil {
		return err
	}
	return wheelAndButtons()
}

func main() {
	countdown := flag.Duration("countdown", 5*time.Second, "delay before typing so the operator can focus Notepad")
	flag.Parse()
	if err := run(*countdown); err != nil {
		fmt.Fprintln(os.Stderr, "wininject:", err)
		os.Exit(1)
	}
}
