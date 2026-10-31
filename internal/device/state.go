package device

import (
	"encoding/json"
	"slices"
)

// NewCharger returns a charger state with its three ports and nothing read.
func NewCharger() Charger {
	return Charger{Ports: []Port{{Name: "C1"}, {Name: "C2"}, {Name: "C3"}}}
}

// NewPowerBank returns a power bank state with nothing read. The ports appear
// with the first telemetry frame.
func NewPowerBank() PowerBank {
	return PowerBank{Ports: []Port{}}
}

// Clone returns a deep copy, safe to hand to another goroutine.
func (c Charger) Clone() Charger {
	c.Identity = c.Identity.clone()
	c.Ports = slices.Clone(c.Ports)
	c.ProductCode = clonePtr(c.ProductCode)
	c.FirmwareTag = clonePtr(c.FirmwareTag)
	c.ScreenBrightness = clonePtr(c.ScreenBrightness)
	c.ScreensaverID = clonePtr(c.ScreensaverID)
	c.ScreensaverFlags = clonePtr(c.ScreensaverFlags)
	c.TotalOutputPowerW = clonePtr(c.TotalOutputPowerW)
	for i := range c.Ports {
		c.Ports[i] = c.Ports[i].clone()
	}
	return c
}

// Clone returns a deep copy, safe to hand to another goroutine.
func (p PowerBank) Clone() PowerBank {
	p.Identity = p.Identity.clone()
	p.Ports = slices.Clone(p.Ports)
	if p.Ports == nil {
		p.Ports = []Port{}
	}
	for i := range p.Ports {
		p.Ports[i] = p.Ports[i].clone()
	}
	if p.Dock != nil {
		d := p.Dock.clone()
		p.Dock = &d
	}
	p.StateCode = clonePtr(p.StateCode)
	p.ThermalState = clonePtr(p.ThermalState)
	p.BatteryPercent = clonePtr(p.BatteryPercent)
	p.TimeLeftHours = clonePtr(p.TimeLeftHours)
	p.TimeLeftMinutes = clonePtr(p.TimeLeftMinutes)
	p.InputPowerW = clonePtr(p.InputPowerW)
	p.OutputPowerW = clonePtr(p.OutputPowerW)
	p.Temperature1C = clonePtr(p.Temperature1C)
	p.Temperature2C = clonePtr(p.Temperature2C)
	p.PomodoroSeconds = clonePtr(p.PomodoroSeconds)
	p.PomodoroEnabled = clonePtr(p.PomodoroEnabled)
	p.BatteryHealthPercent = clonePtr(p.BatteryHealthPercent)
	p.ScreenBrightness = clonePtr(p.ScreenBrightness)
	return p
}

func (i Identity) clone() Identity {
	i.Serial = clonePtr(i.Serial)
	i.Firmware = clonePtr(i.Firmware)
	i.MACAddress = clonePtr(i.MACAddress)
	i.Label = clonePtr(i.Label)
	return i
}

func (p Port) clone() Port {
	p.Direction = clonePtr(p.Direction)
	p.Cable = clonePtr(p.Cable)
	p.ChargingInfo = clonePtr(p.ChargingInfo)
	p.DeviceName = clonePtr(p.DeviceName)
	p.Vendor = clonePtr(p.Vendor)
	return p
}

func clonePtr[T any](p *T) *T {
	if p == nil {
		return nil
	}
	v := *p
	return &v
}

func ptr[T any](v T) *T { return &v }

// refresh recomputes the fields derived from the others. Only the mode byte
// says power is flowing: do not fall back on PowerW, which is sticky on the
// power bank and keeps its last value after a port goes idle.
func (p *Port) refresh() {
	p.Active = p.Mode != 0
	p.Energized = !p.Active && p.Sampled && p.VoltageV > 0.1
}

// MarshalJSON writes the Python to_dict shape: voltage_v, current_a and
// power_w are null until Sampled.
func (p Port) MarshalJSON() ([]byte, error) {
	type plain Port
	aux := struct {
		plain
		VoltageV *float64 `json:"voltage_v"`
		CurrentA *float64 `json:"current_a"`
		PowerW   *float64 `json:"power_w"`
	}{plain: plain(p)}
	if p.Sampled {
		aux.VoltageV, aux.CurrentA, aux.PowerW = &p.VoltageV, &p.CurrentA, &p.PowerW
	}
	return json.Marshal(aux)
}

// UnmarshalJSON reads what MarshalJSON writes.
func (p *Port) UnmarshalJSON(b []byte) error {
	type plain Port
	aux := struct {
		*plain
		VoltageV *float64 `json:"voltage_v"`
		CurrentA *float64 `json:"current_a"`
		PowerW   *float64 `json:"power_w"`
	}{plain: (*plain)(p)}
	if err := json.Unmarshal(b, &aux); err != nil {
		return err
	}
	p.Sampled = aux.VoltageV != nil || aux.CurrentA != nil || aux.PowerW != nil
	p.VoltageV, p.CurrentA, p.PowerW = 0, 0, 0
	if aux.VoltageV != nil {
		p.VoltageV = *aux.VoltageV
	}
	if aux.CurrentA != nil {
		p.CurrentA = *aux.CurrentA
	}
	if aux.PowerW != nil {
		p.PowerW = *aux.PowerW
	}
	return nil
}

// MarshalJSON writes the Python to_dict shape: charging is null until
// ChargingKnown, and ports is an array even when empty.
func (p PowerBank) MarshalJSON() ([]byte, error) {
	type plain PowerBank
	aux := struct {
		plain
		Charging *bool `json:"charging"`
	}{plain: plain(p)}
	if aux.Ports == nil {
		aux.Ports = []Port{}
	}
	if p.ChargingKnown {
		aux.Charging = &p.Charging
	}
	return json.Marshal(aux)
}

// UnmarshalJSON reads what MarshalJSON writes.
func (p *PowerBank) UnmarshalJSON(b []byte) error {
	type plain PowerBank
	aux := struct {
		*plain
		Charging *bool `json:"charging"`
	}{plain: (*plain)(p)}
	if err := json.Unmarshal(b, &aux); err != nil {
		return err
	}
	p.ChargingKnown = aux.Charging != nil
	p.Charging = aux.Charging != nil && *aux.Charging
	return nil
}
