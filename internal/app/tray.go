package app

import (
	"math"

	"github.com/egoist/mygo"

	appui "github.com/LYJW131/anker-prime-desktop/internal/ui"
	"github.com/LYJW131/anker-prime-desktop/locales"
)

// trayItem is the menu bar item: its icon, its title of the values the
// settings choose, and what it shows now, so the system is touched only
// when that changes.
type trayItem struct {
	t *mygo.Tray

	shownTitle string
	// shownIcon is the icon drawn last: its quantized level, the bolt, and
	// whether there is one at all; -2 before the first.
	shownLevel    int
	shownCharging bool
	shownHidden   bool
	shownTip      string
}

// newTray makes the menu bar item: a left click opens the panel, a right
// click the menu.
func newTray(a *App) *trayItem {
	it := &trayItem{shownLevel: -2}
	t, err := mygo.NewTray(mygo.TrayOptions{
		ID:             "anker-prime",
		Icon:           TrayIcon(-1, false),
		IconIsTemplate: true,
		ToolTip:        locales.AppName(),
	})
	if err != nil {
		a.log.Warn("no menu bar item", "error", err)
		return it
	}
	it.t = t
	// The menu is made on each click, in the language of the moment.
	menu := func() *mygo.Menu {
		return mygo.NewMenu([]*mygo.MenuItem{
			{Label: locales.TrayOpen(), Click: func(*mygo.MenuItem, *mygo.Window) { a.ShowWindow() }},
			{Label: locales.TraySettings(), Accelerator: "Cmd+,", Click: func(*mygo.MenuItem, *mygo.Window) {
				a.view.GoSettings()
				a.ShowWindow()
			}},
			mygo.Separator(),
			{Label: locales.TrayQuit(), Accelerator: "Cmd+Q", Click: func(*mygo.MenuItem, *mygo.Window) { a.Quit() }},
		})
	}
	// A tray with a menu sends clicks to the menu, so the menu is set only
	// for the right click's popup and the left click opens the panel.
	t.OnClick(a.togglePanel)
	t.OnRightClick(func() {
		t.SetMenu(menu())
		defer t.SetMenu(nil)
		t.PopUpMenu()
	})
	return it
}

func (it *trayItem) destroy() {
	if it.t != nil {
		it.t.Destroy()
	}
}

// syncTray shows the store on the menu bar item.
func (a *App) syncTray() {
	it := a.tray
	if it == nil || it.t == nil || a.store == nil {
		return
	}
	now := a.store.Now()
	mb := a.store.Settings.MenuBar
	title := appui.TrayTitle(a.store, appui.EffectiveFields(&a.store.Settings), now)
	if title != it.shownTitle {
		it.shownTitle = title
		it.t.SetTitle(title)
	}
	// Without a title the icon shows whatever the settings say: the item
	// has to show something.
	hidden := !mb.ShowIcon && title != ""
	level := -1
	if l := appui.TrayLevel(a.store, now); l >= 0 {
		level = int(math.Round(l * levelSteps))
	}
	charging := appui.TrayCharging(a.store, now)
	if hidden != it.shownHidden || level != it.shownLevel || charging != it.shownCharging {
		it.shownHidden, it.shownLevel, it.shownCharging = hidden, level, charging
		icon := blankIcon()
		if !hidden {
			l := -1.0
			if level >= 0 {
				l = float64(level) / levelSteps
			}
			icon = TrayIcon(l, charging)
		}
		if err := it.t.SetIcon(icon, true); err != nil {
			a.log.Warn("menu bar icon", "error", err)
		}
	}
	if tip := appui.ToolTip(a.store, now); tip != it.shownTip {
		it.shownTip = tip
		it.t.SetToolTip(tip)
	}
}
