package device

// The decoders reproduce the Python contract byte for byte, including
// readings that are not what they seem. ForDisplay corrects those for
// people, in one place, before the state reaches the views.

// idle thresholds: below both, a port moves nothing.
const (
	idleAmps  = 0.02
	idleWatts = 0.2
)

// ForDisplay returns the charger's state as the views should show it: a
// port whose switch bit is set while no current flows (the bit flaps at
// 0 A with a cable plugged in) is not outputting.
func (c Charger) ForDisplay() Charger {
	c = c.Clone()
	for i := range c.Ports {
		quiet(&c.Ports[i])
	}
	return c
}

// ForDisplay returns the power bank's state as the views should show it:
// an idle port's sticky power reading is dropped, and the dock's stale
// register does not count as power on the pins.
func (p PowerBank) ForDisplay() PowerBank {
	p = p.Clone()
	for i := range p.Ports {
		quiet(&p.Ports[i])
	}
	if p.Dock != nil {
		quiet(p.Dock)
		if !p.Dock.Active {
			p.Dock.Energized = false
			p.Dock.VoltageV, p.Dock.CurrentA = 0, 0
		}
	}
	return p
}

func quiet(port *Port) {
	if port.Active && port.CurrentA <= idleAmps && port.PowerW <= idleWatts {
		port.Active, port.Direction = false, nil
		port.Energized = port.VoltageV > 0.1 // a cable with voltage and no load
	}
	if !port.Active {
		port.PowerW = 0
	}
}
