package ui

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/LYJW131/anker-prime-desktop/internal/device"
	"github.com/LYJW131/anker-prime-desktop/internal/model"
	"github.com/LYJW131/anker-prime-desktop/locales"
)

// ratedW is a charger's scale: its model's rated output.
func ratedW(m model.Model) float64 {
	if m.RatedW > 0 {
		return m.RatedW
	}
	return 100
}

// cardHead is a device card's first line: its tile, name and model, and
// the state pill at the right.
func (a *App) cardHead(c *ui.Context, k *Tokens, d dev, s devState) {
	ui.Row(c.Key("card-head")).Height(24).Gap(10).AlignItems(ui.Center).Children(func() {
		ic, col := familyLook(k, d.m.Family)
		tile(c, ic, col, 22)
		// The short name: two cards share a row. The model and the state
		// keep their width; only the name gives way.
		ui.Text(c, d.shortName()).FontSize(13.5).FontWeight(650).SingleLine().Shrink(1).MinWidth(0).Ellipsis("…")
		mono(c, string(d.m.ID), 11).TextColor(k.Faint).Shrink(0)
		ui.Spacer(c)
		a.statePill(c, k, s).Shrink(0)
	})
}

// shareBar is a charger's rated output split among its ports, each in its
// own color, the rest empty.
func shareBar(c *ui.Context, k *Tokens, key string, ports []device.Port, rated float64, h float32) ui.Element {
	bar := ui.Row(c.Key(key)).Height(h).Radius(radiusFull).Background(k.Track).AlignItems(ui.Stretch).Clip()
	bar.Children(func() {
		used := float32(0)
		for i, p := range ports {
			v := float32(0)
			if p.Active {
				v = float32(min(max(p.PowerW/rated, 0), 1))
			}
			v = bar.AnimateTo(fmt.Sprint("p", i), v, ui.Snappy)
			if v < 0.002 {
				continue
			}
			used += v
			ui.Box(c.Key(i)).Grow(v).Background(k.Ports[i%len(k.Ports)]).BorderWidth(0, 1.5, 0, 0).BorderColor(k.Raise)
		}
		if used < 1 {
			ui.Box(c.Key("rest")).Grow(1 - used)
		}
	})
	return bar
}

// shareLegend names the share bar's colors: each port's name and power.
func shareLegend(c *ui.Context, k *Tokens, ports []device.Port, dim bool) {
	ui.Row(c.Key("legend")).Gap(18).Wrap().Children(func() {
		for i, p := range ports {
			ui.Row(c.Key(i)).Gap(6).AlignItems(ui.Center).Children(func() {
				col := k.Ports[i%len(k.Ports)]
				if !p.Active {
					col = k.Track.Mix(k.Fg, 0.15)
				}
				dot(c, col, 8)
				mono(c, p.Name, 11.5).FontWeight(600).TextColor(k.Muted)
				v := none
				if p.Active && !dim {
					v = watts(p.PowerW) + " W"
				}
				slotText(c, func(s string) ui.Element { return mono(c, s, 11.5).TextColor(k.Fg) }, v, "000.0 W", ui.Start)
			})
		}
	})
}

// factRow is one fact of a device: its name at the left, the value at the
// right, in the monospace face when it is a number or a code.
func factRow(c *ui.Context, k *Tokens, ic *ui.SVG, name, value string, monoValue, last bool) {
	row := ui.Row(c.Key(name)).Height(34).Gap(10).AlignItems(ui.Center)
	if !last {
		row.BorderWidth(0, 0, 1, 0).BorderColor(k.Line)
	}
	row.Children(func() {
		if ic != nil {
			ui.Icon(c, ic).FontSize(14).TextColor(k.Faint)
		}
		ui.Text(c, name).FontSize(12.5).TextColor(k.Muted).SingleLine()
		ui.Spacer(c)
		var v ui.Element
		if monoValue {
			v = mono(c, value, 12)
		} else {
			v = ui.Text(c, value).FontSize(12.5).SingleLine()
		}
		v.TextAlign(ui.End).Shrink(1).Ellipsis("…")
		if value == none {
			v.TextColor(k.Faint)
		}
	})
}

// facts is a column of factRows.
type fact struct {
	ic    *ui.SVG
	name  string
	value string
	mono  bool
}

func factList(c *ui.Context, k *Tokens, list []fact) ui.Element {
	return ui.Column(c.Key("facts")).Children(func() {
		for i, f := range list {
			factRow(c, k, f.ic, f.name, f.value, f.mono, i == len(list)-1)
		}
	})
}

// metric is a small cell of a port: a name over a value and its unit.
func metric(c *ui.Context, k *Tokens, name, value, unit string, dim bool) ui.Element {
	return ui.Column(c.Key(name)).Grow(1).Basis(0).Padding(8, 10).Gap(3).Radius(radiusControl).Background(k.Wash).Children(func() {
		ui.Text(c, name).FontSize(11).TextColor(k.Muted).SingleLine()
		ui.Row(c).AlignItems(ui.Baseline).Gap(3).Children(func() {
			col := k.Fg
			if dim {
				col = k.Faint
			}
			// The cell keeps its width; only its own unit follows the number.
			mono(c, value, 15).FontWeight(600).TextColor(col)
			ui.Text(c, unit).FontSize(11).TextColor(k.Muted)
		})
	})
}

// portTitle is a port's name as its card shows it.
func portTitle(name string) string {
	switch strings.ToUpper(name) {
	case "A":
		return "USB-A"
	case "DOCK":
		return locales.PortDock()
	}
	return "USB-" + strings.ToUpper(name)
}

// portLine is a line of a port card: an icon, a name, and the value.
func portLine(c *ui.Context, k *Tokens, ic *ui.SVG, name, value string) {
	ui.Row(c.Key(name)).Height(26).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Icon(c, ic).FontSize(13).TextColor(k.Faint)
		ui.Text(c, name).FontSize(12).TextColor(k.Muted).SingleLine()
		ui.Spacer(c)
		v := ui.Text(c, orNone(value)).FontSize(12).SingleLine().Shrink(1).Ellipsis("…").TextAlign(ui.End)
		if value == "" {
			v.TextColor(k.Faint)
		}
	})
}

// portHead is a port card's first line: a swatch of its color, its name,
// and its state.
func portHead(c *ui.Context, k *Tokens, col ui.Color, name string, kind pillKind, ic *ui.SVG, word string) {
	ui.Row(c.Key("port-head")).Height(22).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Box(c).Size(4, 16).Radius(radiusFull).Background(col)
		ui.Text(c, portTitle(name)).FontSize(13.5).FontWeight(650).SingleLine()
		ui.Spacer(c)
		pill(c, k, kind, ic, word)
	})
}

// smallStat is one of the overview's small cards: a label over a big
// value, or anything body draws.
func smallStat(c *ui.Context, k *Tokens, key string, ic *ui.SVG, name string, body func()) {
	ui.Column(c.Key(key)).Padding(padCard).Gap(10).Radius(radiusCard).Background(k.Raise).MinWidth(0).Children(func() {
		ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, ic).FontSize(13).TextColor(k.Faint)
			ui.Text(c, name).FontSize(12).FontWeight(600).TextColor(k.Muted).SingleLine()
		})
		body()
	})
}

// batteryColor is the power bank's battery bar: green while it charges,
// amber while too hot to, red when low, else the plain ink.
func batteryColor(k *Tokens, pb *device.PowerBank) ui.Color {
	switch {
	case pb.ThermalLimited:
		return k.Warn
	case pb.BatteryPercent != nil && *pb.BatteryPercent <= 20:
		return k.Bad
	}
	return k.OK
}

// flowWord is how energy moves through the power bank.
func flowWord(pb *device.PowerBank) (string, pillKind, *ui.SVG) {
	in, out := deref(pb.InputPowerW), deref(pb.OutputPowerW)
	switch {
	case pb.ThermalLimited:
		return locales.FlowThermal(), pillWarn, icThermo
	case in >= 0.5 && out >= 0.5:
		return locales.FlowBoth(), pillOK, icBolt
	case in >= 0.5 || pb.Charging:
		return locales.FlowCharging(), pillOK, icBolt
	case out >= 0.5:
		return locales.FlowDischarging(), pillRun, icArrowOut
	}
	return locales.PortIdle(), pillWait, nil
}

// signedWatts writes the power bank's net power with its sign.
func signedWatts(v float64) string {
	switch {
	case v >= 0.05:
		return "+" + watts(v)
	case v <= -0.05:
		return "−" + watts(v)
	}
	return "0.0"
}

// portCells is a row of a device's ports, each its name and power, the
// rest muted: the overview's and the panel's glance at the ports.
func portCells(c *ui.Context, k *Tokens, ports []device.Port, live bool, size float32) {
	ui.Grid(c.Key("cells")).Columns(len(ports)).Gap(6).Children(func() {
		for i, p := range ports {
			on := p.Active && live
			ui.Column(c.Key(p.Name)).Gap(3).Padding(size*0.5, size*0.65).Radius(radiusInner).Background(k.Fg.Alpha(0.04)).MinWidth(0).Children(func() {
				ui.Row(c).Gap(5).AlignItems(ui.Center).Children(func() {
					col := k.Ports[i%len(k.Ports)]
					if !on {
						col = k.Track.Mix(k.Fg, 0.15)
					}
					dot(c, col, 6)
					name := p.Name
					if name == "DOCK" {
						name = locales.PortDock()
					}
					ui.Text(c, name).FontSize(10.5).FontWeight(600).TextColor(k.Muted).SingleLine()
				})
				v := none
				if on {
					v = fmt.Sprintf("%.0f W", p.PowerW)
				}
				m := mono(c, v, size).FontWeight(600)
				if !on {
					m.TextColor(k.Faint)
				}
			})
		}
	})
}
