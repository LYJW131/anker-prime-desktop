package ui

import (
	"fmt"
	"runtime"

	"github.com/egoist/mygo/ui"

	"github.com/LYJW131/anker-prime-desktop/internal/model"
	"github.com/LYJW131/anker-prime-desktop/locales"
)

// PanelWidth is the tray panel's width in DIPs.
const PanelWidth = 340

// Panel builds the tray icon's panel: the cards the settings list, in
// their order, and the way into the app.
func (a *App) Panel(c *ui.Context) {
	k := a.tokens(c)
	applyTheme(c, k)
	c.Root().Background(ui.Transparent)
	box := ui.Column(c.Key("panel")).Padding(padCard - 4).Gap(8)
	switch {
	case c.Vibrancy() && !a.Opaque: // the popover's material on macOS
	case c.Vibrancy(), runtime.GOOS == "windows":
		// The popover or the flyout rounds and outlines itself; a capture,
		// which draws no material, and Reduce Transparency get a ground.
		c.Root().Background(k.Raise)
		box.Background(k.Raise)
	default:
		box.Background(k.Ground).Radius(radiusCard).Border(1, k.Line2)
	}
	box.Children(func() {
		for _, r := range EffectiveCards(&a.Store.Settings) {
			id, cid := splitRef(r)
			d, ok := a.device(id)
			if !ok {
				continue
			}
			s := a.state(c, d)
			switch cid {
			case "main":
				if d.m.Family == model.FamilyPowerBank {
					a.panelBank(c, k, d, s)
				} else {
					a.panelCharger(c, k, d, s)
				}
			case "ports":
				a.panelPorts(c, k, d, s)
			case "cover":
				if d.m.Covers {
					a.panelCover(c, k, d)
				}
			}
		}
		ui.Row(c.Key("actions")).Gap(8).Margin(2, 0, 0, 0).Children(func() {
			if iconButton(c, k, icSettings, locales.NavSettings(), false).Size(32, 32).Radius(radiusControl).Clicked() {
				a.GoSettings()
				a.Act.ShowWindow()
			}
			b := ui.ButtonBase(c.Key("open")).Grow(1).Height(32).Radius(radiusControl).AlignItems(ui.Center).Justify(ui.Center).
				Background(k.Pill).Label(locales.TrayOpen())
			if b.Hovered() {
				b.Opacity(0.92)
			}
			if b.Clicked() {
				a.Act.ShowWindow()
			}
			b.Cursor(ui.CursorPointer).Children(func() {
				ui.Text(c, locales.TrayOpen()).FontSize(12.5).FontWeight(700).TextColor(k.PillInk)
			})
		})
	})
	// The flyout does not size itself to native content: fit it to what
	// the last frame laid out. Bounds are a frame late, so the frame that
	// lays out another height asks for the next one as it paints.
	if h := box.Bounds().H; h > 0 && h != a.panelH {
		a.panelH = h
		a.Act.FitPanel(h)
	}
	box.Draw(func(_ *ui.Painter, r ui.Rect) {
		if r.H != a.panelH {
			c.Invalidate()
		}
	})
}

// panelCard is one of the panel's cards: a translucent box that reads on
// the popover's material. A click opens its device's page at tab.
func (a *App) panelCard(c *ui.Context, k *Tokens, key string, id string, tab Tab, body func()) {
	b := ui.ButtonBase(c.Key(key)).Padding(padTile).Gap(8).Radius(radiusMenu).Column().
		AlignItems(ui.Stretch).Justify(ui.Start).Background(k.Fg.Alpha(0.05)).Label(key)
	if b.Hovered() {
		b.Background(k.Fg.Alpha(0.08))
	}
	if b.Clicked() {
		a.GoDevice(id, tab)
		a.Act.ShowWindow()
	}
	b.Children(body)
}

// padTile is the room inside a panel card.
const padTile = 12

func (a *App) panelHead(c *ui.Context, k *Tokens, d dev, s devState) {
	ui.Row(c.Key("head")).Height(20).Gap(8).AlignItems(ui.Center).Children(func() {
		ic, col := familyLook(k, d.m.Family)
		tile(c, ic, col, 18)
		ui.Text(c, d.shortName()).FontSize(12.5).FontWeight(650).SingleLine().Shrink(1).MinWidth(0).Ellipsis("…")
		ui.Spacer(c)
		a.statePill(c, k, s).Shrink(0)
	})
}

func (a *App) panelCharger(c *ui.Context, k *Tokens, d dev, s devState) {
	a.panelCard(c, k, "p-main-"+d.ID(), d.ID(), TabLive, func() {
		a.panelHead(c, k, d, s)
		if s.dead {
			a.panelDead(c, k, d)
			return
		}
		ch := d.charger()
		ports := d.ports()
		rated := ratedW(d.m)
		bigValueTail(c, k, fixed(ch.TotalOutputPowerW, 1), "W", "000.0", 32, dimIf(k, !s.live), fmt.Sprintf("/ %.0f W", rated))
		shareBar(c, k, "share", ports, rated, 5)
		sparkline(c, k, "spark", d.st.Watts, 30, k.Accent, k.Accent, false, 20)
		ui.Text(c, chargerNote(ports, !s.live)).FontSize(11.5).TextColor(k.Muted).SingleLine().Ellipsis("…")
	})
}

func (a *App) panelBank(c *ui.Context, k *Tokens, d dev, s devState) {
	a.panelCard(c, k, "p-main-"+d.ID(), d.ID(), TabLive, func() {
		a.panelHead(c, k, d, s)
		if s.dead {
			a.panelDead(c, k, d)
			return
		}
		pb := d.bank()
		ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
			bigValue(c, k, fixed(pb.BatteryPercent, 1), "%", "100.0", 32, dimIf(k, !s.live))
			ui.Spacer(c)
			word, tone, ic := flowWord(pb)
			_, fg := tone.colors(k)
			ui.Row(c).Gap(4).AlignItems(ui.Center).Children(func() {
				if ic != nil {
					ui.Icon(c, ic).FontSize(11).TextColor(fg)
				}
				ui.Text(c, word).FontSize(11.5).FontWeight(600).TextColor(fg).SingleLine()
			})
		})
		frac := 0.0
		if pb.BatteryPercent != nil {
			frac = *pb.BatteryPercent / 100
		}
		meter(c, k, "level", frac, 5, batteryColor(k, pb))
		ui.Grid(c.Key("mini")).Columns(3).Gap(6).Children(func() {
			mini := func(key, name, value string) {
				ui.Column(c.Key(key)).Gap(2).Padding(6, 8).Radius(radiusInner).Background(k.Fg.Alpha(0.04)).MinWidth(0).Children(func() {
					ui.Text(c, name).FontSize(10.5).TextColor(k.Muted).SingleLine().Ellipsis("…")
					mono(c, value, 12.5).FontWeight(600)
				})
			}
			nw := signedWatts(flow(pb)) + " W"
			full := orNone(timeToFull(pb))
			temp := none
			if t := hottest(pb); t != nil {
				temp = fmt.Sprintf("%d°C", *t)
			}
			if !s.live {
				nw, full, temp = none, none, none
			}
			mini("flow", locales.PanelFlow(), nw)
			mini("full", locales.FieldTimeToFull(), full)
			mini("temp", locales.FactTemperature(), temp)
		})
	})
}

// panelDead is a panel card's body while its device shows only a state.
func (a *App) panelDead(c *ui.Context, k *Tokens, d dev) {
	st := d.st.Status
	ui.Row(c.Key("dead")).Height(28).AlignItems(ui.Center).Children(func() {
		line := d.set.Name
		if st.Err != nil && d.set.Enabled {
			line = ErrText(st.Err)
		}
		if line == "" {
			line = string(d.m.ID)
		}
		ui.Text(c, line).FontSize(11.5).TextColor(k.Faint).SingleLine().Ellipsis("…")
	})
}

// panelPorts lists a device's ports in one row of cells.
func (a *App) panelPorts(c *ui.Context, k *Tokens, d dev, s devState) {
	a.panelCard(c, k, "p-ports-"+d.ID(), d.ID(), TabLive, func() {
		ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
			ui.Text(c, locales.OverviewPorts()).FontSize(11.5).FontWeight(600).TextColor(k.Muted).SingleLine()
			ui.Text(c, d.shortName()).FontSize(11.5).TextColor(k.Faint).SingleLine().Shrink(1).MinWidth(0).Ellipsis("…")
		})
		portCells(c, k, d.ports(), s.live, 12.5)
	})
}

// panelCover is a charger's current cover.
func (a *App) panelCover(c *ui.Context, k *Tokens, d dev) {
	a.panelCard(c, k, "p-cover-"+d.ID(), d.ID(), TabCovers, func() {
		ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
			a.coverChip(c, k, d, 40)
			ui.Spacer(c)
			if job := a.jobShown(c); job != nil && job.Err == nil && job.Total > 0 {
				mono(c, fmt.Sprintf("%d/%d", job.Done, job.Total), 11).TextColor(k.Muted)
			} else {
				ui.Text(c, locales.FactCover()).FontSize(11).TextColor(k.Faint).SingleLine()
			}
		})
	})
}
