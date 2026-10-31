package ui

import (
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/LYJW131/anker-prime-desktop/locales"
)

// sidebar is the window's left column: the traffic lights and the sidebar
// button over its top, the overview, a row per added device with its state
// and headline, the way to add one, and Settings at the foot. It is w wide
// as it slides open or closed; its content keeps its width meanwhile,
// clipped, so nothing in it reflows.
//
// Where the window shows its material (vib), the sidebar paints no ground
// of its own: the system's sidebar material is the ground, and what sits
// on it fills with neutral washes (Wash, Track).
func (a *App) sidebar(c *ui.Context, k *Tokens, vib bool, w float32) {
	if w < 0.5 {
		return
	}
	ui.Column(c.Key("sidebar")).Width(w).Clip().Children(func() {
		col := ui.Column(c).Width(sidebarWidth).Grow(1).Padding(0, 10, 12, 10).Gap(14).BorderWidth(0, 1, 0, 0).BorderColor(k.Line)
		if !vib || a.Opaque {
			col.Background(k.Ground.Mix(k.Raise, 0.55))
		}
		col.Children(func() {
			ui.Box(c).Height(controlsBand(c.TitleBar())).Margin(0, -10, -6, -10).DragWindow()
			ui.Column(c).Gap(2).Children(func() {
				a.navItem(c, k, Page{Kind: PageOverview}, icOverview, k.TileOverview, locales.NavOverview())
			})
			ui.Column(c.Key("devices")).Gap(2).Children(func() {
				label(c, k, locales.NavDevices()).Margin(0, 10, 4, 10)
				for _, d := range a.devices() {
					a.deviceRow(c, k, d)
				}
				a.addRow(c, k)
			})
			ui.Spacer(c)
			a.navItem(c, k, Page{Kind: PageSettings}, icSettings, k.TileSettings, locales.NavSettings())
		})
	})
}

// deviceRow is an added device in the sidebar: its family's tile, its
// name over its state, and its headline at the right while live.
func (a *App) deviceRow(c *ui.Context, k *Tokens, d dev) {
	s := a.state(c, d)
	on := a.Page.Kind == PageDevice && a.Page.Device == d.ID()
	row := ui.ButtonBase(c.Key(d.ID())).Height(44).PaddingX(8).Gap(10).Radius(radiusControl).
		AlignItems(ui.Center).Justify(ui.Start).Label(d.name() + " " + s.word)
	rest, hover := ui.Transparent, k.Wash
	if on {
		rest, hover = k.Track, k.Track
	}
	row.Background(sideFill(row, k, rest, hover))
	if row.Clicked() {
		a.Go(Page{Kind: PageDevice, Device: d.ID()})
	}
	_, fg := s.tone.colors(k)
	if s.tone == pillWait {
		fg = k.Faint
	}
	row.Cursor(ui.CursorPointer).Children(func() {
		ic, col := familyLook(k, d.m.Family)
		tile(c, ic, col, 26)
		ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(2).Children(func() {
			ui.Text(c, d.shortName()).FontSize(12.5).FontWeight(600).SingleLine().Ellipsis("…")
			// One height with or without the headline, so the name keeps
			// its place as the number comes and goes.
			ui.Row(c).Height(16).Gap(5).AlignItems(ui.Center).Children(func() {
				ui.Box(c).Size(8, 8).Center().Children(func() {
					dt := dot(c, fg, 6)
					if s.tone == pillRun {
						dt.Opacity(0.45 + 0.55*dt.Loop("pulse", 1200*time.Millisecond, ui.Bounce(ui.EaseInOut)))
					}
				})
				word := k.Muted
				if s.tone == pillBad || s.tone == pillWarn {
					word = fg
				}
				ui.Text(c, s.word).FontSize(11).TextColor(word).SingleLine().Shrink(1).MinWidth(0).Ellipsis("…")
				ui.Spacer(c)
				if v, vc := a.headline(k, d, s); v != "" {
					fgv, wt := k.Muted, 500
					if vc.A != 0 {
						fgv, wt = vc, 600
					}
					mono(c, v, 11).FontWeight(wt).TextColor(fgv)
				}
			})
		})
	})
}

// addRow opens Add Device.
func (a *App) addRow(c *ui.Context, k *Tokens) {
	on := a.Page.Kind == PageAddDevice
	row := ui.ButtonBase(c.Key("add-device")).Height(36).PaddingX(8).Gap(10).Radius(radiusControl).
		AlignItems(ui.Center).Justify(ui.Start).Label(locales.NavAddDevice())
	rest, hover := ui.Transparent, k.Wash
	if on {
		rest, hover = k.Track, k.Track
	}
	row.Background(sideFill(row, k, rest, hover))
	if row.Clicked() {
		a.Go(Page{Kind: PageAddDevice})
	}
	row.Cursor(ui.CursorPointer).Children(func() {
		ui.Box(c).Size(26, 26).Radius(radiusInner).Border(1, k.Line2).Center().Children(func() {
			ui.Icon(c, icPlus).FontSize(14).TextColor(k.Muted)
		})
		fg := k.Muted
		if on {
			fg = k.Fg
		}
		ui.Text(c, locales.NavAddDevice()).FontSize(12.5).TextColor(fg).SingleLine()
	})
}

// controlsBand is the height of the band the window controls sit in, and
// the sidebar button beside them.
func controlsBand(bar ui.TitleBar) float32 {
	if bar.Height > 0 {
		return max(bar.Height, 44)
	}
	return 44
}

func toggleLeft(bar ui.TitleBar) float32 {
	if bar.Left > 0 {
		return bar.Left + 4
	}
	return 12
}

// toggleSize is the sidebar button's side.
const toggleSize = 28

// sidebarToggle is the sidebar button. It floats over the window's top-left
// corner beside the traffic lights, so it stays put while the sidebar
// opens and closes under it, as the system apps' does.
func (a *App) sidebarToggle(c *ui.Context, k *Tokens) {
	bar := c.TitleBar()
	name := locales.SidebarHide()
	if !a.SidebarShown() {
		name = locales.SidebarShow()
	}
	top := (controlsBand(bar) - toggleSize) / 2
	b := ui.ButtonBase(c.Key("sidebar-toggle")).Size(toggleSize, toggleSize).Radius(radiusControl).Center().
		Absolute().Left(toggleLeft(bar)).Top(top).Label(name).Tooltip(name + "  ⌃⌘S")
	switch {
	case b.Pressed():
		b.Background(k.Track.Mix(k.Fg, 0.08))
	case b.Hovered():
		b.Background(k.Track)
	}
	if b.Clicked() {
		a.ToggleSidebar()
	}
	b.Cursor(ui.CursorPointer).Children(func() {
		ui.Icon(c, icSidebar).FontSize(18).TextColor(k.Muted)
	})
}

// followWidth keeps the window's width, and lets the sidebar close by
// itself as the window narrows past sidebarFits: an opening the user asked
// for in a narrow window lasts until the window is wide again.
func (a *App) followWidth(w float32) {
	a.winW = w
	if w >= sidebarFits {
		a.openNarrow = false
	}
}

// SidebarShown reports whether the sidebar is open: as the user last left
// it, unless the window is too narrow for it and the user has not opened it
// there.
func (a *App) SidebarShown() bool {
	if a.SidebarHidden {
		return false
	}
	return a.winW >= sidebarFits || a.openNarrow
}

// ToggleSidebar opens or closes the sidebar, and keeps the choice for the
// next launch.
func (a *App) ToggleSidebar() {
	if a.SidebarShown() {
		a.SidebarHidden, a.openNarrow = true, false
	} else {
		a.SidebarHidden = false
		a.openNarrow = a.winW < sidebarFits
	}
	a.Act.SetSidebarHidden(a.SidebarHidden)
}

func (a *App) navItem(c *ui.Context, k *Tokens, p Page, ic *ui.SVG, col ui.Color, name string) {
	on := a.Page.Kind == p.Kind
	item := ui.ButtonBase(c.Key(name)).Height(30).PaddingX(10).Gap(10).Radius(radiusControl).AlignItems(ui.Center).Justify(ui.Start).Label(name)
	rest, hover := ui.Transparent, k.Wash
	if on {
		rest, hover = k.Track, k.Track
	}
	item.Background(sideFill(item, k, rest, hover))
	if item.Clicked() {
		a.Go(p)
	}
	item.Cursor(ui.CursorPointer).Children(func() {
		tile(c, ic, col, 22)
		ui.Text(c, name).FontSize(13).TextColor(k.Fg).Grow(1).SingleLine()
	})
}

// sideFill is the fill of a row or a card in the sidebar: rest, hover
// under the pointer and a stronger wash while pressed, each a translucent
// neutral that reads over the window's material.
func sideFill(b ui.Element, k *Tokens, rest, hover ui.Color) ui.Color {
	switch {
	case b.Pressed():
		return k.Track.Mix(k.Fg, 0.08)
	case b.Hovered():
		return hover
	}
	return rest
}
