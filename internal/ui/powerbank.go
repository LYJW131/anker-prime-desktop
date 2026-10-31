package ui

import (
	"fmt"

	"github.com/egoist/mygo/ui"

	"github.com/LYJW131/anker-prime-desktop/internal/device"
	"github.com/LYJW131/anker-prime-desktop/locales"
)

// bankLive is a power bank's live tab: its battery and the energy moving
// through it, the facts, and a card per port and its dock.
func (a *App) bankLive(c *ui.Context, k *Tokens, d dev, s devState) {
	pb := d.bank()
	dim := !s.live
	hero := card(c, k).Padding(padCard + 4).Gap(20)
	if a.compact {
		hero.Column()
	} else {
		// The facts' column starts at the top, its rule the card's height.
		hero.Row().AlignItems(ui.Stretch)
	}
	hero.Children(func() {
		ui.Column(c.Key("battery")).Grow(1).Basis(0).MinWidth(0).Gap(14).Children(func() {
			label(c, k, locales.BankBattery())
			ui.Row(c).AlignItems(ui.Center).Gap(12).Children(func() {
				bigValue(c, k, fixed(pb.BatteryPercent, 1), "%", "100.0", 56, dimIf(k, dim))
				ui.Spacer(c)
				ui.Column(c).Gap(6).AlignItems(ui.End).Children(func() {
					word, tone, ic := flowWord(pb)
					pill(c, k, tone, ic, word)
					slotText(c, func(s string) ui.Element { return mono(c, s, 15).FontWeight(600) }, signedWatts(flow(pb))+" W", "+000.0 W", ui.End)
				})
			})
			frac := 0.0
			if pb.BatteryPercent != nil {
				frac = *pb.BatteryPercent / 100
			}
			meter(c, k, "level", frac, 10, batteryColor(k, pb))
			ui.Row(c).Gap(18).Children(func() {
				legend := func(col ui.Color, name string) {
					ui.Row(c.Key(name)).Gap(6).AlignItems(ui.Center).Children(func() {
						dot(c, col, 8)
						ui.Text(c, name).FontSize(11.5).TextColor(k.Muted)
					})
				}
				legend(k.OK, locales.PortInput())
				legend(k.Accent, locales.PortOutput())
				ui.Spacer(c)
				if t := timeToFull(pb); t != "" {
					ui.Text(c, locales.FullIn(t)).FontSize(11.5).FontFeatures("tnum").TextColor(k.Muted).SingleLine()
				}
			})
			sparkline(c, k, "spark", d.st.Watts, 92, k.OK, k.Accent, true, 15)
		})
		facts := ui.Column(c.Key("facts-col")).Gap(4).Justify(ui.Start)
		if !a.compact {
			facts.Width(300).BorderWidth(0, 0, 0, 1).BorderColor(k.Line).PaddingStart(20)
		}
		facts.Children(func() {
			label(c, k, locales.SectionDevice())
			temps := none
			if pb.Temperature1C != nil && pb.Temperature2C != nil {
				temps = fmt.Sprintf("%d° · %d°", *pb.Temperature1C, *pb.Temperature2C)
			}
			factList(c, k, []fact{
				{icThermo, locales.FactTemperature(), temps, true},
				{icHeart, locales.FactHealth(), intStr(pb.BatteryHealthPercent, "%"), true},
				{icSun, locales.FactBrightness(), intStr(pb.ScreenBrightness, "%"), true},
				{icChip, locales.FactSerial(), orNone(str(pb.Serial)), true},
				{icSync, locales.FactFirmware(), orNone(str(pb.Firmware)), true},
				{icBluetooth, locales.FactMAC(), orNone(str(pb.MACAddress)), true},
			})
		})
	})
	ports := d.ports()
	cols := len(ports)
	if a.compact {
		cols = 2
	}
	ui.Grid(c.Key("ports")).Columns(max(cols, 1)).Gap(14).Children(func() {
		for i, p := range ports {
			a.bankPort(c, k, i, p, dim)
		}
	})
}

// timeToFull is how long a charging power bank needs to be full, as
// "1:05", or "" when it does not charge or does not say.
func timeToFull(pb *device.PowerBank) string {
	if !pb.Charging || deref32(pb.TimeLeftHours)+deref32(pb.TimeLeftMinutes) == 0 {
		return ""
	}
	return hm(pb.TimeLeftHours, pb.TimeLeftMinutes)
}

func deref32(v *int) int {
	if v == nil {
		return 0
	}
	return *v
}

// bankPort is one of a power bank's port cards. Its power is sticky on
// the wire, so it shows only while the port is active.
func (a *App) bankPort(c *ui.Context, k *Tokens, i int, p device.Port, dim bool) {
	ui.Column(c.Key(p.Name)).Padding(padCard).Gap(12).Radius(radiusCard).Background(k.Raise).MinWidth(0).Children(func() {
		dock := p.Name == "DOCK"
		in := p.Direction != nil && *p.Direction == "in"
		kind, word, ic := pillWait, locales.PortIdle(), (*ui.SVG)(nil)
		switch {
		case p.Active && in:
			kind, word, ic = pillOK, locales.PortInput(), icArrowIn
		case p.Active:
			kind, word, ic = pillRun, locales.PortOutput(), icArrowOut
		case p.Energized:
			kind, word = pillWait, locales.PortPowered()
		case dock && p.Attached:
			kind, word = pillWait, locales.PortDocked()
		}
		portHead(c, k, k.Ports[i%len(k.Ports)], p.Name, kind, ic, word)
		on := p.Active && !dim
		col := ui.Color{}
		if !on {
			col = k.Faint
		}
		w := 0.0
		if p.Active {
			w = p.PowerW
		}
		bigValue(c, k, watts(w), "W", "000.0", 30, col)
		ui.Row(c).Gap(8).Children(func() {
			v, amps := 0.0, 0.0
			if p.Active || p.Energized {
				v = p.VoltageV
			}
			if p.Active {
				amps = p.CurrentA
			}
			metric(c, k, locales.MetricVoltage(), fmt.Sprintf("%.2f", v), "V", !on)
			metric(c, k, locales.MetricCurrent(), fmt.Sprintf("%.2f", amps), "A", !on)
		})
		role := locales.RoleBoth()
		switch {
		case dock:
			role = locales.RoleIn()
		case p.Name == "A":
			role = locales.RoleOut()
		}
		state := locales.PortStateNone()
		switch {
		case p.Active && in:
			state = locales.PortStateDrawing()
		case p.Active:
			state = locales.PortStateSupplying()
		case p.Energized:
			state = locales.PortStateNoLoad()
		case dock && p.Attached:
			state = locales.PortStateDocked()
		}
		ui.Column(c.Key("lines")).Children(func() {
			portLine(c, k, icSync, locales.PortDirection(), role)
			portLine(c, k, icBolt, locales.PortState(), state)
		})
	})
}
