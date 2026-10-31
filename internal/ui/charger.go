package ui

import (
	"fmt"

	"github.com/egoist/mygo/ui"

	"github.com/LYJW131/anker-prime-desktop/internal/device"
	"github.com/LYJW131/anker-prime-desktop/locales"
)

// chargerLive is a charger's live tab: its total output split among its
// ports, the facts, and a card per port.
func (a *App) chargerLive(c *ui.Context, k *Tokens, d dev, s devState) {
	ch := d.charger()
	ports := d.ports()
	dim := !s.live
	rated := ratedW(d.m)
	hero := card(c, k).Padding(padCard + 4).Gap(20)
	if a.compact {
		hero.Column()
	} else {
		// The facts' column starts at the top, its rule the card's height.
		hero.Row().AlignItems(ui.Stretch)
	}
	hero.Children(func() {
		ui.Column(c.Key("output")).Grow(1).Basis(0).MinWidth(0).Gap(14).Children(func() {
			label(c, k, locales.ChargerTotal())
			bigValueTail(c, k, fixed(ch.TotalOutputPowerW, 1), "W", "000.0", 56, dimIf(k, dim), fmt.Sprintf("/ %.0f W", rated))
			shareBar(c, k, "share", ports, rated, 8)
			shareLegend(c, k, ports, dim)
			sparkline(c, k, "spark", d.st.Watts, 92, k.Accent, k.Accent, false, 20).Margin(6, 0, 0, 0)
		})
		facts := ui.Column(c.Key("facts-col")).Gap(4).Justify(ui.Start)
		if !a.compact {
			facts.Width(300).BorderWidth(0, 0, 0, 1).BorderColor(k.Line).PaddingStart(20)
		}
		facts.Children(func() {
			label(c, k, locales.SectionDevice())
			list := []fact{
				{icChip, locales.FactSerial(), orNone(str(ch.Serial)), true},
				{icSync, locales.FactFirmware(), orNone(str(ch.Firmware)), true},
				{icBluetooth, locales.FactMAC(), orNone(str(ch.MACAddress)), true},
				{icSun, locales.FactBrightness(), intStr(ch.ScreenBrightness, "%"), true},
			}
			if d.m.Covers {
				list = append(list, fact{icCover, locales.FactCover(), a.currentCoverName(d), false})
			}
			factList(c, k, list)
		})
	})
	cols := len(ports)
	if a.compact {
		cols = 1
	}
	ui.Grid(c.Key("ports")).Columns(max(cols, 1)).Gap(14).Children(func() {
		for i, p := range ports {
			a.chargerPort(c, k, i, p, dim)
		}
	})
}

// chargerPort is one of a charger's port cards: its power, voltage and
// current, and what is plugged in.
func (a *App) chargerPort(c *ui.Context, k *Tokens, i int, p device.Port, dim bool) {
	ui.Column(c.Key(p.Name)).Padding(padCard).Gap(12).Radius(radiusCard).Background(k.Raise).MinWidth(0).Children(func() {
		on := p.Active && !dim
		kind, word, ic := pillWait, locales.PortIdle(), (*ui.SVG)(nil)
		if p.Active {
			kind, word, ic = pillRun, locales.PortOutput(), icArrowOut
		}
		portHead(c, k, k.Ports[i%len(k.Ports)], p.Name, kind, ic, word)
		col := ui.Color{}
		if !on {
			col = k.Faint
		}
		bigValue(c, k, watts(p.PowerW), "W", "000.0", 34, col)
		ui.Row(c).Gap(8).Children(func() {
			metric(c, k, locales.MetricVoltage(), fmt.Sprintf("%.2f", p.VoltageV), "V", !on)
			metric(c, k, locales.MetricCurrent(), fmt.Sprintf("%.2f", p.CurrentA), "A", !on)
		})
		// Three lines of what is plugged in, or one line in the same room.
		ui.Column(c.Key("lines")).Height(3 * 26).Children(func() {
			if !p.Active && str(p.Cable) == "" && str(p.DeviceName) == "" {
				ui.Column(c).Grow(1).Center().Children(func() {
					ui.Text(c, locales.PortNothing()).FontSize(12).TextColor(k.Faint)
				})
				return
			}
			devName := str(p.DeviceName)
			if devName == "" {
				devName = str(p.Vendor)
			}
			portLine(c, k, icCable, locales.PortCable(), str(p.Cable))
			portLine(c, k, icBolt, locales.PortProtocol(), str(p.ChargingInfo))
			portLine(c, k, icLaptop, locales.PortDevice(), devName)
		})
	})
}

// currentCoverName names the cover a charger shows.
func (a *App) currentCoverName(d dev) string {
	if it, ok := a.currentCover(d); ok {
		return it.Name
	}
	if id := d.charger().ScreensaverID; id != nil {
		return fmt.Sprintf("#%d", *id)
	}
	return none
}
