package device

import "testing"

func TestChargerForDisplayQuietsFlappingPort(t *testing.T) {
	out := "out"
	c := NewCharger()
	c.Ports = []Port{
		{Name: "C2", Mode: 1, Active: true, Direction: &out, VoltageV: 5.05, Sampled: true},
		{Name: "C3", Mode: 1, Active: true, Direction: &out, VoltageV: 5.05, CurrentA: 0.43, PowerW: 2.1, Sampled: true},
	}
	d := c.ForDisplay()
	if p := d.Ports[0]; p.Active || p.Direction != nil || !p.Energized {
		t.Errorf("C2 at 0 A: active=%v direction=%v energized=%v, want idle with voltage", p.Active, p.Direction, p.Energized)
	}
	if p := d.Ports[1]; !p.Active || p.PowerW != 2.1 {
		t.Errorf("C3 under load changed: %+v", p)
	}
	if !c.Ports[0].Active {
		t.Error("ForDisplay changed the decoder's state")
	}
}

func TestPowerBankForDisplayDropsStaleDockAndStickyPower(t *testing.T) {
	p := NewPowerBank()
	p.Ports = []Port{{Name: "C1", Mode: 0, PowerW: 91.3, Sampled: true}}
	p.Dock = &Port{Name: "DOCK", Energized: true, VoltageV: 20.6, Sampled: true}
	d := p.ForDisplay()
	if d.Ports[0].PowerW != 0 {
		t.Errorf("idle C1 kept its sticky %v W", d.Ports[0].PowerW)
	}
	if d.Dock.Energized || d.Dock.VoltageV != 0 {
		t.Errorf("stale dock shown as %+v", *d.Dock)
	}
	if p.Dock.VoltageV != 20.6 {
		t.Error("ForDisplay changed the decoder's state")
	}
}
