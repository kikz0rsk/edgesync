package x11

import (
	"fmt"

	"github.com/jezek/xgb/randr"
)

// Monitor is one active RandR 1.5 monitor in the server's physical pixel space (D-89).
type Monitor struct {
	Name        string
	Primary     bool
	Automatic   bool
	X           int16
	Y           int16
	Width       uint16
	Height      uint16
	WidthMM     uint32
	HeightMM    uint32
	OutputCount uint16
}

func (c *Conn) Monitors() ([]Monitor, error) {
	reply, err := randr.GetMonitors(c.x, c.root, true).Reply()
	if err != nil {
		return nil, fmt.Errorf("x11: RandR GetMonitors: %w", err)
	}
	monitors := make([]Monitor, 0, len(reply.Monitors))
	for _, m := range reply.Monitors {
		name, err := c.atomName(m.Name)
		if err != nil {
			return nil, err
		}
		monitors = append(monitors, Monitor{
			Name:        name,
			Primary:     m.Primary,
			Automatic:   m.Automatic,
			X:           m.X,
			Y:           m.Y,
			Width:       m.Width,
			Height:      m.Height,
			WidthMM:     m.WidthInMillimeters,
			HeightMM:    m.HeightInMillimeters,
			OutputCount: m.NOutput,
		})
	}
	return monitors, nil
}

// WatchMonitorChanges asks the server for ScreenChangeNotify and CrtcChangeNotify events on the root window.
func (c *Conn) WatchMonitorChanges() error {
	mask := uint16(randr.NotifyMaskScreenChange | randr.NotifyMaskCrtcChange)
	if err := randr.SelectInputChecked(c.x, c.root, mask).Check(); err != nil {
		return fmt.Errorf("x11: RandR SelectInput: %w", err)
	}
	return nil
}
