package ui

import (
	"time"

	"github.com/egoist/mygo/ui"
)

// The main window's columns, in DIPs.
const (
	// sidebarWidth is the sidebar's width.
	sidebarWidth = 232
	// compactContent is the page width below which the pages use their
	// compact layout.
	compactContent = 680
	// sidebarFits is the narrowest window the sidebar stays open in by
	// itself; a narrower one closes it until the user opens it there.
	sidebarFits = sidebarWidth + compactContent
	// sidebarSlide is how long the sidebar takes to open or close.
	sidebarSlide = 200 * time.Millisecond
	// pagePad is the room around a page's content.
	pagePad = 28
)

// View builds the main window.
func (a *App) View(c *ui.Context) {
	k := a.tokens(c)
	applyTheme(c, k)
	a.shortcuts(c)
	w, _ := c.Size()
	a.followWidth(w)
	shown := a.SidebarShown()
	target := float32(0)
	if shown {
		target = sidebarWidth
	}
	a.compact = w-target < compactContent
	vib := c.Vibrancy()
	root := ui.Box(c).Fill()
	side := root.AnimateWith("sidebar", target, sidebarSlide, ui.EaseOut)
	root.Children(func() {
		ui.Row(c).Fill().AlignItems(ui.Stretch).Children(func() {
			a.sidebar(c, k, vib, side)
			ui.Column(c.Key("main")).Grow(1).Shrink(1).MinWidth(0).Background(k.Ground).Clip().Children(func() {
				a.titleBar(c, k, shown)
				ui.Column(c.Key("page")).Grow(1).MinHeight(0).Children(func() {
					switch a.Page.Kind {
					case PageDevice:
						a.devicePage(c, k)
					case PageAddDevice:
						a.addPage(c, k)
					case PageSettings:
						a.settingsPage(c, k)
					default:
						a.overviewPage(c, k)
					}
				})
			})
		})
		a.sidebarToggle(c, k)
	})
	a.coverDialogs(c, k)
	a.deviceDialogs(c, k)
	a.toast(c, k)
	a.scrollbars(c, k)
	// Last: SetTheme, which scrollbars calls, gives the root the theme's
	// ground, which would cover the window's material under the sidebar.
	if vib && !a.Opaque {
		c.Root().Background(ui.Transparent)
	} else {
		c.Root().Background(k.Ground)
	}
}

// shortcuts are the window's keys: Cmd+1 the overview, Cmd+2 to Cmd+9 the
// devices in the sidebar's order, Cmd+N Add Device, Cmd+, Settings. The
// sidebar's, Control-Command-S, is the View menu's.
func (a *App) shortcuts(c *ui.Context) {
	keys := []ui.Key{ui.Key1, ui.Key2, ui.Key3, ui.Key4, ui.Key5, ui.Key6, ui.Key7, ui.Key8, ui.Key9}
	c.OnShortcut(ui.Cmd, keys[0], func() { a.Go(Page{}) })
	for i, d := range a.devices() {
		if i+1 >= len(keys) {
			break
		}
		id := d.ID()
		c.OnShortcut(ui.Cmd, keys[i+1], func() { a.Go(Page{Kind: PageDevice, Device: id}) })
	}
	c.OnShortcut(ui.Cmd, ui.KeyN, func() { a.Go(Page{Kind: PageAddDevice}) })
	c.OnShortcut(ui.Cmd, ui.KeyComma, a.GoSettings)
}

// titleBar is the content's top strip, which drags the window, clear of
// the window controls; with the sidebar closed it keeps the traffic lights
// and the sidebar button (sidebarToggle) on the content's own ground.
func (a *App) titleBar(c *ui.Context, k *Tokens, sidebar bool) {
	bar := c.TitleBar()
	ui.Row(c.Key("titlebar")).Height(controlsBand(bar)).FillWidth().DragWindow()
}

// pageScroll is a page's scrolling column, padded and as wide as the
// window allows.
func (a *App) pageScroll(c *ui.Context, k *Tokens, key string, children func()) {
	a.scrollArea(c, key, k.Ground, 0, func() {
		ui.Column(c).Padding(0, pagePad, pagePad, pagePad).Gap(16).Children(children)
	}).Grow(1)
}

// pageHead is a page's title row: the title, a pill, and actions at the
// right.
func pageHead(c *ui.Context, k *Tokens, title string, after func(), actions func()) {
	ui.Row(c.Key("head")).Height(36).Gap(12).AlignItems(ui.Center).Margin(0, 0, 4, 0).Children(func() {
		display(c, title, 26).SingleLine().Shrink(1).MinWidth(0).Ellipsis("…")
		if after != nil {
			after()
		}
		ui.Spacer(c)
		if actions != nil {
			actions()
		}
	})
}

// toast shows the store's latest notice for a few seconds.
func (a *App) toast(c *ui.Context, k *Tokens) {
	n := a.Store.Notice
	if n == nil {
		return
	}
	age := c.Now().Sub(n.At)
	if age > 4*time.Second || age < 0 {
		return
	}
	c.After(4*time.Second - age)
	ui.Overlay(c, func() {
		ui.Row(c).Attach(ui.AnchorBottom, ui.AnchorBottom).Bottom(24).
			PaddingX(14).PaddingY(9).Gap(8).Radius(radiusFull).AlignItems(ui.Center).
			Background(k.Pill).Shadow(0, 12, 32, 0, ui.RGBA(0, 0, 0, 0.35)).Children(func() {
			ui.Text(c, NoticeText(n)).FontSize(12.5).FontWeight(600).MaxWidth(560).SingleLine().Ellipsis("…").TextColor(k.PillInk)
		})
	})
}
