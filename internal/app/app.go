// Package app runs Anker Prime: it loads the settings, owns the store, the
// main window, the menu bar item and its panel, starts the backend, and
// does what the views ask of it.
package app

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/LYJW131/anker-prime-mac/internal/settings"
	"github.com/LYJW131/anker-prime-mac/internal/store"
	appui "github.com/LYJW131/anker-prime-mac/internal/ui"
	"github.com/LYJW131/anker-prime-mac/locales"
)

// Backend drives the devices; see ui.Backend for its contract. The demo
// (package demo) is one; the BLE backend is the other.
type Backend = appui.Backend

// Options are the command line's choices.
type Options struct {
	// Backend drives the devices. Required.
	Backend Backend
	// DataDir replaces the user data directory, to keep a test run apart.
	DataDir string
	// Theme forces "light" or "dark".
	Theme string
	// Shots captures every screen into this directory as PNGs, then quits.
	// It keeps the settings in memory.
	Shots string
	// Lang is the language the run shows, set for one run through
	// MYGO_LOCALE; the shots of another language than the usual capture
	// the main pages only.
	Lang string
	// Live keeps the real backend while capturing screens: the shots
	// wait until every added device streams.
	Live bool
}

// App is the running app. Its fields belong to the UI thread.
type App struct {
	opts  Options
	log   *slog.Logger
	dir   string
	store *store.Store
	view  *appui.App
	be    Backend

	win *mygo.Window
	// panel is the tray item's open panel, or nil.
	panel *mygo.Window
	// panelH is the height the panel last fitted its content to, which
	// the next one opens at.
	panelH int
	tray   *trayItem

	quitting bool
	// dock is the activation policy last set; menuSet and menuSidebar what
	// the menu bar was last built for; appearance the theme source last
	// handed to MyGo.
	dock        mygo.ActivationPolicy
	menuSet     bool
	menuSidebar bool
	appearance  mygo.ThemeSource
}

// viewState is what the window remembers that is not a setting.
type viewState struct {
	HideSidebar bool `json:"hide_sidebar"`
}

const viewStateFile = "view.json"

// Run runs the app until it quits.
func Run(opts Options) error {
	a := &App{opts: opts, be: opts.Backend, log: slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))}
	mygo.App.SetName("Anker Prime")
	if opts.DataDir != "" {
		mygo.App.SetPath(mygo.PathUserData, filepath.Join(opts.DataDir, "data"))
		mygo.App.SetPath(mygo.PathCache, filepath.Join(opts.DataDir, "cache"))
		mygo.App.SetPath(mygo.PathLogs, filepath.Join(opts.DataDir, "logs"))
	}
	if opts.Shots == "" && !mygo.App.RequestSingleInstanceLock() {
		return nil
	}
	// A menu bar app: no Dock icon until the window shows or the settings
	// ask for one (syncDock).
	a.dock = mygo.ActivationPolicyAccessory
	mygo.App.SetActivationPolicy(a.dock)
	mygo.App.OnBeforeQuit(func(*mygo.QuitEvent) { a.quitting = true })
	mygo.App.OnWindowAllClosed(func() {})
	mygo.App.OnActivate(func(visible bool) {
		if !visible && a.win != nil {
			a.showWindow()
		}
	})
	mygo.App.OnSecondInstance(func([]string, string) { mygo.RunOnMain(a.ShowWindow) })
	mygo.App.WhenReady(a.ready)
	return mygo.App.Run()
}

func (a *App) ready() {
	if err := appui.RegisterFonts(); err != nil {
		// The system's monospace face stands in; the app still works.
		a.log.Warn("embedded fonts unreadable", "error", err)
	}
	dir, err := mygo.App.Path(mygo.PathUserData)
	if err != nil {
		a.log.Warn("no user data directory", "error", err)
	}
	a.dir = dir
	a.store = store.New(a.post)
	var vs viewState
	if a.opts.Shots == "" || a.opts.Live {
		s, err := settings.Load(a.dir)
		if err != nil {
			a.log.Warn("settings unreadable; starting from the defaults", "error", err)
		}
		a.store.Settings = s
		a.followLanguage(s.Language)
		if data, err := os.ReadFile(filepath.Join(a.dir, viewStateFile)); err == nil {
			_ = json.Unmarshal(data, &vs)
		}
	}
	a.view = &appui.App{Store: a.store, Dev: a.be, Act: a, Theme: a.opts.Theme, SidebarHidden: vs.HideSidebar}
	a.view.ScrollbarsAlways = scrollbarsAlways()
	a.view.Opaque = a.opaque()
	a.appearance = appearanceOf(a.view)
	mygo.Theme.SetSource(a.appearance)

	a.win = mygo.NewWindow(mygo.WindowOptions{
		Title:           locales.AppName(),
		Width:           1100,
		Height:          720,
		MinWidth:        820,
		MinHeight:       560,
		StateKey:        "main",
		Hidden:          true,
		TitleBarStyle:   mygo.TitleBarHidden,
		Vibrancy:        mygo.VibrancySidebar,
		BackgroundColor: "light-dark(#f6f7fa, #111219)",
		Content: ui.View(func(c *ui.Context) {
			a.view.View(c)
			a.syncAppearance()
			a.syncMenu()
		}),
	})
	a.win.OnClose(func(e *mygo.CloseEvent) {
		// Closing the window keeps the app in the menu bar; the menu's
		// Quit ends it.
		if !a.quitting {
			e.PreventDefault()
			a.win.Hide()
		}
	})
	mygo.App.OnLocaleChanged(func(string) {
		// What was set once says the new language: the window's title, the
		// menu bar's menus; the menu bar item's menu is made on each click.
		a.win.SetTitle(locales.AppName())
		a.menuSet = false
		a.syncMenu()
		a.syncTray()
	})
	a.win.OnShow(a.syncDock)
	a.win.OnHide(a.syncDock)
	mygo.Theme.OnUpdated(func() {
		opaque := a.opaque()
		a.post(func() { a.view.Opaque = opaque })
	})
	mygo.App.OnWillQuit(func(*mygo.QuitEvent) {
		if a.tray != nil {
			a.tray.destroy()
		}
	})
	a.tray = newTray(a)
	a.be.Start(a.store)
	a.syncTray()
	// The store's clock moves without a frame: the title follows it.
	go func() {
		for range time.Tick(time.Second) {
			mygo.RunOnMain(a.syncTray)
		}
	}()
	a.showWindow()
	if a.opts.Shots != "" {
		go a.captureScreens(a.opts.Shots)
	}
}

// post runs fn on the UI thread, then refreshes the menu bar item and
// redraws the window and the panel.
func (a *App) post(fn func()) {
	mygo.RunOnMain(func() {
		fn()
		a.syncTray()
		a.win.Invalidate()
		if a.panel != nil {
			a.panel.Invalidate()
		}
	})
}

// opaque reports whether the sidebar draws its own ground over the
// window's material: the desktop asks for opaque surfaces, or the run
// captures screens, which CapturePage renders without the material.
func (a *App) opaque() bool {
	return a.opts.Shots != "" || mygo.Theme.Preferences().ReduceTransparency
}

// syncAppearance has the window's own appearance, which its material and
// window controls take, follow the theme the view shows, once a frame
// changed it. The call goes through the main loop, not inside the frame.
func (a *App) syncAppearance() {
	if src := appearanceOf(a.view); src != a.appearance {
		a.appearance = src
		go mygo.Theme.SetSource(src)
	}
}

// appearanceOf is the appearance v shows: the screenshots' or the
// settings' choice, else the system's.
func appearanceOf(v *appui.App) mygo.ThemeSource {
	theme := v.Theme
	if theme == "" {
		theme = v.Store.Settings.Theme
	}
	switch theme {
	case "light":
		return mygo.ThemeLight
	case "dark":
		return mygo.ThemeDark
	}
	return mygo.ThemeSystem
}

// syncDock gives the app a Dock icon and its menu bar while the window
// shows or the settings ask for one, and none otherwise.
func (a *App) syncDock() {
	want := mygo.ActivationPolicyAccessory
	if a.store.Settings.ShowDock || (a.win != nil && a.win.IsVisible()) {
		want = mygo.ActivationPolicyRegular
	}
	if want != a.dock {
		a.dock = want
		mygo.App.SetActivationPolicy(want)
		a.menuSet = false
	}
}

// togglePanel opens the tray item's panel below it, or closes it.
func (a *App) togglePanel() {
	if a.panel != nil {
		a.panel.Close()
		return
	}
	if a.tray == nil || a.tray.t == nil {
		a.showWindow()
		return
	}
	h := a.panelH
	if h == 0 {
		h = 520
	}
	a.panel = mygo.NewFlyout(mygo.FlyoutOptions{
		Tray:      a.tray.t,
		Placement: mygo.PlacementBottom,
		Gap:       4,
		Width:     appui.PanelWidth,
		// Opened at the height the last one fitted, it does not show at
		// another for the frames before the view measures its content.
		Height:    h,
		Focusable: true,
		Shadow:    true,
		Popover:   runtime.GOOS == "darwin",
		Content:   ui.View(a.view.Panel),
	})
	a.panel.OnClosed(func() {
		a.panel = nil
		a.view.PanelClosed()
	})
}

func (a *App) showWindow() {
	a.win.Show()
	a.win.Focus()
	mygo.App.Focus()
	a.syncDock()
}

// saveViewState keeps what the window remembers.
func (a *App) saveViewState() {
	if a.opts.Shots != "" {
		return
	}
	data, _ := json.Marshal(viewState{HideSidebar: a.view.SidebarHidden})
	if err := settings.WriteAtomic(filepath.Join(a.dir, viewStateFile), data); err != nil {
		a.log.Warn("view state not saved", "error", err)
	}
}

// followLanguage shows the language the settings choose, "" following
// the system's. A run whose language MYGO_LOCALE chooses (-lang, `mygo dev
// -lang`) keeps it, and a run capturing screens touches nothing kept.
func (a *App) followLanguage(tag string) {
	if a.opts.Shots != "" || os.Getenv("MYGO_LOCALE") != "" {
		return
	}
	mygo.App.SetLocale(tag)
}
