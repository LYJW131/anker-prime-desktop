package ui

import (
	"fmt"
	"sort"

	"github.com/egoist/mygo/ui"

	"github.com/LYJW131/anker-prime-mac/internal/device"
	"github.com/LYJW131/anker-prime-mac/internal/model"
	"github.com/LYJW131/anker-prime-mac/locales"
)

// overviewPage is every added device at a glance: a card of its headline
// and one of its ports, each opening its page. Without devices it is the
// way to add one.
func (a *App) overviewPage(c *ui.Context, k *Tokens) {
	devs := a.devices()
	a.pageScroll(c, k, "overview", func() {
		pageHead(c, k, locales.NavOverview(), nil, func() {
			if len(devs) > 0 && button(c, k, buttonPlain, locales.NavAddDevice(), icPlus).Clicked() {
				a.Go(Page{Kind: PageAddDevice})
			}
		})
		if len(devs) == 0 {
			a.emptyState(c, k)
			return
		}
		cols := 2
		if a.compact || len(devs) == 1 {
			cols = 1
		}
		ui.Grid(c.Key("devices")).Columns(cols).Gap(14).Children(func() {
			for _, d := range devs {
				s := a.state(c, d)
				ui.Column(c.Key(d.ID())).Gap(14).MinWidth(0).Children(func() {
					a.heroCard(c, k, "hero", d.ID(), func() {
						a.cardHead(c, k, d, s)
						ui.Column(c.Key("body")).Gap(12).Height(176).Children(func() {
							switch {
							case s.dead:
								a.heroState(c, k, d, s)
							case d.m.Family == model.FamilyPowerBank:
								a.overviewBank(c, k, d, s)
							default:
								a.overviewCharger(c, k, d, s)
							}
						})
					})
					a.heroCard(c, k, "ports", d.ID(), func() {
						ui.Text(c, locales.OverviewPorts()).FontSize(12).FontWeight(600).TextColor(k.Muted)
						portCells(c, k, d.ports(), s.live, 16)
					})
				})
			}
		})
	})
}

// emptyState is the overview before the first device: one way on.
func (a *App) emptyState(c *ui.Context, k *Tokens) {
	ui.Column(c.Key("empty")).Padding(56, padCard).Gap(10).Radius(radiusCard).Background(k.Raise).AlignItems(ui.Center).Children(func() {
		ui.Row(c).Gap(8).Margin(0, 0, 6, 0).Children(func() {
			tile(c, icCharger, k.TileCharger, 44)
			tile(c, icPowerBank, k.TilePowerBank, 44)
		})
		ui.Text(c, locales.EmptyTitle()).FontSize(17).FontWeight(700)
		if button(c, k, buttonPrimary, locales.NavAddDevice(), icPlus).Margin(6, 0, 0, 0).Clicked() {
			a.Go(Page{Kind: PageAddDevice})
		}
	})
}

func dimIf(k *Tokens, dim bool) ui.Color {
	if dim {
		return k.Faint
	}
	return ui.Color{}
}

// heroCard is an overview card: it opens its device's page.
func (a *App) heroCard(c *ui.Context, k *Tokens, key, id string, body func()) {
	b := ui.ButtonBase(c.Key(key)).Padding(padCard + 4).Gap(14).Radius(radiusCard).Background(k.Raise).
		AlignItems(ui.Stretch).Justify(ui.Start).MinWidth(0).Column().Label(key)
	if b.Hovered() {
		b.Background(k.Raise.Mix(k.Fg, 0.025))
	}
	if b.Clicked() {
		a.GoDevice(id, TabLive)
	}
	b.Cursor(ui.CursorPointer).Children(body)
}

func (a *App) overviewCharger(c *ui.Context, k *Tokens, d dev, s devState) {
	ch := d.charger()
	ports := d.ports()
	rated := ratedW(d.m)
	bigValueTail(c, k, fixed(ch.TotalOutputPowerW, 1), "W", "000.0", 44, dimIf(k, !s.live), fmt.Sprintf("/ %.0f W", rated))
	shareBar(c, k, "share", ports, rated, 6)
	sparkline(c, k, "spark", d.st.Watts, 56, k.Accent, k.Accent, false, 20)
	ui.Text(c, chargerNote(ports, !s.live)).FontSize(12).TextColor(k.Muted).SingleLine().Ellipsis("…")
}

// chargerNote is the line under a charger: what draws power, busiest
// first.
func chargerNote(ports []device.Port, dim bool) string {
	type use struct {
		name, what string
		w          float64
	}
	var uses []use
	for _, p := range ports {
		if !p.Active || dim {
			continue
		}
		what := str(p.DeviceName)
		if what == "" {
			what = str(p.Vendor)
		}
		uses = append(uses, use{p.Name, what, p.PowerW})
	}
	if len(uses) == 0 {
		return locales.ChargerNothing()
	}
	sort.SliceStable(uses, func(i, j int) bool { return uses[i].w > uses[j].w })
	u := uses[0]
	line := u.name
	if u.what != "" {
		line += " · " + u.what
	}
	if len(uses) > 1 {
		line = locales.ChargerMore(line, len(uses)-1)
	}
	return line
}

func (a *App) overviewBank(c *ui.Context, k *Tokens, d dev, s devState) {
	pb := d.bank()
	ui.Row(c).AlignItems(ui.Center).Gap(10).Children(func() {
		bigValue(c, k, fixed(pb.BatteryPercent, 1), "%", "100.0", 44, dimIf(k, !s.live))
		ui.Spacer(c)
		word, tone, ic := flowWord(pb)
		pill(c, k, tone, ic, word)
	})
	frac := 0.0
	if pb.BatteryPercent != nil {
		frac = *pb.BatteryPercent / 100
	}
	meter(c, k, "level", frac, 6, batteryColor(k, pb))
	sparkline(c, k, "spark", d.st.Watts, 56, k.OK, k.Accent, true, 15)
	note := signedWatts(flow(pb)) + " W"
	if t := timeToFull(pb); t != "" {
		note += " · " + locales.FullIn(t)
	}
	if !s.live {
		note = none
	}
	ui.Text(c, note).FontSize(12).FontFeatures("tnum").TextColor(k.Muted).SingleLine().Ellipsis("…")
}

// heroState fills an overview card whose device has nothing to show but
// its state.
func (a *App) heroState(c *ui.Context, k *Tokens, d dev, s devState) {
	ui.Column(c).Grow(1).Center().Gap(6).Children(func() {
		ui.Text(c, s.word).FontSize(16).FontWeight(700).TextColor(k.Muted)
		if n := d.set.Name; n != "" {
			mono(c, n, 11).TextColor(k.Faint)
		}
	})
}
