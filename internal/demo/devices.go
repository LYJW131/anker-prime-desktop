package demo

import (
	"math"
	"time"

	"github.com/LYJW131/anker-prime-desktop/internal/device"
	"github.com/LYJW131/anker-prime-desktop/internal/model"
	"github.com/LYJW131/anker-prime-desktop/internal/store"
)

func sp(s string) *string         { return &s }
func ip(v int) *int               { return &v }
func fp(v float64) *float64       { return &v }
func bp(v bool) *bool             { return &v }
func round(v, to float64) float64 { return math.Round(v/to) * to }

// wave is a smooth wobble of period seconds, phase-shifted, in -1…1.
func wave(t, period, phase float64) float64 { return math.Sin(2*math.Pi*t/period + phase) }

// seed fills what a device's handshake and snapshot tell.
func (b *Backend) seed(id string) {
	dd := b.devs[id]
	if dd == nil {
		return
	}
	name := id
	if sd, ok := b.s.Settings.Device(id); ok && sd.Name != "" {
		name = sd.Name
	}
	if dd.m.Family == model.FamilyPowerBank {
		b.seedPowerBank(id, name)
	} else {
		b.seedCharger(id, name)
	}
}

// at moves a device's numbers to time at; advance moves a power bank's
// battery a second on.
func (b *Backend) at(id string, at time.Time, advance bool) {
	dd := b.devs[id]
	if dd == nil {
		return
	}
	if dd.m.Family == model.FamilyPowerBank {
		b.powerBankAt(id, dd, at, advance)
	} else {
		b.chargerAt(id, dd, at)
	}
}

// seedCharger fills what a charger's handshake and snapshot tell.
func (b *Backend) seedCharger(id, name string) {
	d := b.s.Device(id)
	if d.Charger == nil {
		d.Charger = &device.Charger{}
	}
	ch := d.Charger
	ch.Serial, ch.Firmware, ch.MACAddress, ch.Label = sp(name), sp("v0.0.5.1"), sp("7C:E9:13:77:C3:64"), sp("Charging")
	ch.ScreenBrightness, ch.ScreensaverFlags = ip(100), ip(896)
	if ch.ScreensaverID == nil && len(b.s.Covers) > 0 {
		ch.ScreensaverID = ip(b.s.Covers[0].ID & 0xffff)
	}
}

// chargerAt is a charger at time at: a MacBook Pro on C1 drawing about
// 90 W, an iPhone on C2, and a phone that comes and goes on C3.
func (b *Backend) chargerAt(id string, dd *demoDev, at time.Time) {
	t := at.Sub(b.t0).Seconds() + dd.phase
	d := b.s.Device(id)
	if d.Charger == nil {
		d.Charger = &device.Charger{}
	}
	ch := d.Charger
	mac := 86 + 6*wave(t, 47, 0) + 2.5*wave(t, 7.3, 1)
	phone := 19 + 2*wave(t, 23, 2) + 0.8*wave(t, 3.1, 0)
	c3On := math.Mod(t+600, 120) < 70
	c3 := 24 + 3*wave(t, 13, 1)
	ports := []device.Port{
		port("C1", 20.07, mac, "EPR-240W MAX", "Apple PD Fast Charging", "MacBook Pro series", "Apple"),
		port("C2", 9.01, phone, "USB-C 60W", "USB PD 3.0", "iPhone", "Apple"),
		{Name: "C3", Cable: sp("N/A"), ChargingInfo: sp("N/A")},
	}
	if c3On {
		ports[2] = port("C3", 9.02, c3, "USB-C 100W", "USB PD 3.0 PPS", "Galaxy S24", "Samsung")
	}
	ch.Ports = ports
	total := 0.0
	for _, p := range ports {
		total += p.PowerW
	}
	ch.TotalOutputPowerW = fp(round(total, 0.01))
	d.Watts = store.Push(d.Watts, store.Sample{At: at, Value: total})
}

// port is an active output port drawing w at v volts.
func port(name string, v, w float64, cable, info, dev, vendor string) device.Port {
	out := "out"
	return device.Port{Name: name, Mode: 1, Direction: &out, Active: true, Attached: true, Energized: true,
		VoltageV: v, CurrentA: round(w/v, 0.001), PowerW: round(w, 0.01),
		Cable: sp(cable), ChargingInfo: sp(info), DeviceName: sp(dev), Vendor: sp(vendor)}
}

// seedPowerBank fills what a power bank's handshake and snapshot tell.
func (b *Backend) seedPowerBank(id, name string) {
	d := b.s.Device(id)
	if d.PowerBank == nil {
		d.PowerBank = &device.PowerBank{}
	}
	pb := d.PowerBank
	pb.Serial, pb.Firmware, pb.MACAddress, pb.Label = sp(name), sp("v0.0.5.1"), sp("7C:E9:13:78:CA:DA"), sp("Charging")
	pb.BatteryHealthPercent, pb.ScreenBrightness = ip(100), ip(100)
	pb.PomodoroSeconds, pb.PomodoroEnabled = ip(1500), bp(true)
	pb.ThermalState = ip(1)
}

// powerBankAt is a power bank at time at: on its dock taking about
// 70 W, feeding AirPods on C2. advance moves its battery a second on.
func (b *Backend) powerBankAt(id string, dd *demoDev, at time.Time, advance bool) {
	t := at.Sub(b.t0).Seconds() + dd.phase
	d := b.s.Device(id)
	if d.PowerBank == nil {
		d.PowerBank = &device.PowerBank{}
	}
	pb := d.PowerBank
	in := 68 + 3*wave(t, 31, 0.5) + wave(t, 4.7, 0)
	out := 0.0
	c2On := math.Mod(t+600, 150) < 95
	if c2On {
		out = 4.6 + 0.6*wave(t, 9, 1)
	}
	if advance {
		// 20 000 mAh at 3.7 V is 74 Wh; a demo hour is a minute.
		dd.battery = math.Min(100, dd.battery+(in-out)/74/60*0.4)
		if dd.battery >= 100 {
			dd.battery = 32
		}
	}
	pb.BatteryPercent = fp(round(dd.battery, 0.01))
	pb.Charging = in > 0.5
	pb.InputPowerW, pb.OutputPowerW = fp(round(in, 0.1)), fp(round(out, 0.1))
	left := (100 - dd.battery) / 100 * 74 / math.Max(in-out, 1) * 60
	pb.TimeLeftHours, pb.TimeLeftMinutes = ip(int(left)/60), ip(int(left)%60)
	pb.Temperature1C = ip(int(math.Round(38 + 2*wave(t, 90, 0))))
	pb.Temperature2C = ip(int(math.Round(39 + 2*wave(t, 90, 0.6))))
	idle := func(name string) device.Port { return device.Port{Name: name} }
	c2 := idle("C2")
	if c2On {
		out := "out"
		c2 = device.Port{Name: "C2", Mode: 1, Direction: &out, Active: true, Attached: true, Energized: true,
			VoltageV: 5.02, CurrentA: round(deref(pb.OutputPowerW)/5.02, 0.001), PowerW: deref(pb.OutputPowerW)}
	}
	pb.Ports = []device.Port{idle("C1"), c2, idle("A")}
	dirIn := "in"
	pb.Dock = &device.Port{Name: "DOCK", Mode: 2, Direction: &dirIn, Active: true, Attached: true, Energized: true,
		VoltageV: 20.5, CurrentA: round(in/20.5, 0.01), PowerW: round(in, 0.1)}
	d.Watts = store.Push(d.Watts, store.Sample{At: at, Value: in - out})
}

func deref(v *float64) float64 {
	if v == nil {
		return 0
	}
	return *v
}
