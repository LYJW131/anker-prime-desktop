// Package ui builds Anker Prime's screens with MyGo's native UI: the
// overview of the added devices, each device's page by its model, adding a
// device, settings, and the menu bar panel, with a designed state for
// every phase of a device's connection, dark and light, in the app's
// language.
package ui

import (
	"slices"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/LYJW131/anker-prime-desktop/internal/cover"
	"github.com/LYJW131/anker-prime-desktop/internal/settings"
	"github.com/LYJW131/anker-prime-desktop/internal/store"
)

// Backend is what drives the devices: the views call it, and it hands
// every result back by changing the store on the UI thread (Store.Post).
// The views note a device they add (store.NoticeDeviceAdded) themselves.
// Its methods are called on the UI thread and must not block; done
// callbacks run on the UI thread too. It words nothing itself: failures
// are errors (the sentinels of package link and store) and toasts are
// store.NoticeKinds, which the views say in the app's language.
type Backend interface {
	Start(s *store.Store)                       // after settings are loaded into s
	Scan()                                      // add-device scan: fills Store.Scanning / Store.Found (every Anker Prime advert, supported or not)
	StopScan()                                  //
	Connect(id string)                          // connect device id again now
	Disconnect(id string)                       // drop it until Connect or a settings change
	SelectCover(deviceID string, it cover.Item) // progress in Store.CoverJob, outcome via Store.Note
	AddCover(path string)
	RemoveCover(id int)
	RenameCover(id int, name string)
	CoverData(it cover.Item) []byte
	SetUserID(id string, done func(error))
	ImportUserID(done func(error))
	Login(account, password string, done func(error))
	SaveSettings(s settings.Settings) // follows devices added/removed/enabled/idle-sleep changes; called before Store.Settings takes s
}

// Shell is what the views ask of the app around them.
type Shell interface {
	// ApplySettings saves the settings, puts them in the store, and has
	// the backend, the tray, the Dock and the language follow them.
	ApplySettings(s settings.Settings)
	// SetSidebarHidden keeps the user's choice of the sidebar for the
	// next launch.
	SetSidebarHidden(hidden bool)
	// ShowWindow brings the main window forward, closing the tray panel.
	ShowWindow()
	// FitPanel sizes the tray panel to its content's height in DIPs.
	FitPanel(height float32)
	// ChooseImage asks for a picture file and hands it to done on the UI
	// thread unless the user cancels.
	ChooseImage(done func(path string))
	// Quit ends the app.
	Quit()
}

// PageKind is which page of the main window shows.
type PageKind int

// The pages. Overview, the zero PageKind, is the one the window opens on.
const (
	PageOverview PageKind = iota
	PageDevice            // Page.Device's page
	PageAddDevice
	PageSettings
)

// Page is a page of the main window.
type Page struct {
	Kind   PageKind
	Device string // the device of PageDevice, a settings.Device.ID
}

// Tab is a part of a device's page.
type Tab int

// The tabs of a device's page. Covers shows only for models with covers.
const (
	TabLive Tab = iota
	TabCovers
	TabSettings
)

// App is the main window's view, the panel's, and the state of their
// pages. Use it on the UI thread.
type App struct {
	Store *store.Store
	Dev   Backend
	Act   Shell
	// Theme overrides the settings' appearance for screenshots: "light",
	// "dark", or "" to follow the settings.
	Theme string
	// Opaque is set while the sidebar draws its own ground over the
	// window's material (Reduce Transparency, screenshots).
	Opaque bool
	// SidebarHidden is the user's choice of the sidebar.
	SidebarHidden bool
	// ScrollbarsAlways shows the scroll thumbs at all times, as the desktop
	// does where its setting says so.
	ScrollbarsAlways bool

	Page Page
	// tabs is the tab each device's page shows.
	tabs map[string]Tab

	// winW is the window's width as last built; compact is set while the
	// page is narrower than compactContent. openNarrow is set while the
	// user keeps the sidebar open in a window too narrow for it.
	winW       float32
	compact    bool
	openNarrow bool

	scrolls    map[string]*ui.ScrollState
	scrollSeen map[string]float32
	scrolledAt time.Time

	// shown is the phase each device's state shows, which follows a
	// transient phase only once it lasted statusDelay (shownPhase).
	shown map[string]phaseShown
	// thumbs are the library's pictures, decoded once.
	thumbs map[thumbKey]*ui.Bitmap

	set    settingsForm
	covers coversForm
	add    addForm
	dev    devForm
	// panelH is the height the tray panel was last fitted to.
	panelH float32
}

// tokens returns the colors of the current appearance.
func (a *App) tokens(c *ui.Context) *Tokens {
	dark := c.Theme().Dark
	switch {
	case a.Theme == "light":
		dark = false
	case a.Theme == "dark":
		dark = true
	case a.Store.Settings.Theme == "light":
		dark = false
	case a.Store.Settings.Theme == "dark":
		dark = true
	}
	if dark {
		return &DarkTokens
	}
	return &LightTokens
}

// Go switches the main window to a page. Opening Add Device starts a scan,
// which leaving it stops.
func (a *App) Go(p Page) {
	from := a.Page.Kind
	a.Page = p
	switch {
	case p.Kind == PageAddDevice && from != PageAddDevice:
		a.startScan()
	case p.Kind != PageAddDevice && from == PageAddDevice:
		a.Dev.StopScan()
	}
}

// GoDevice opens a device's page at a tab.
func (a *App) GoDevice(id string, t Tab) {
	if a.tabs == nil {
		a.tabs = map[string]Tab{}
	}
	a.tabs[id] = t
	a.Go(Page{Kind: PageDevice, Device: id})
}

// GoSettings opens Settings.
func (a *App) GoSettings() { a.Go(Page{Kind: PageSettings}) }

// PanelClosed forgets the tray panel's size, so the next one fits again.
func (a *App) PanelClosed() { a.panelH = 0 }

// apply changes the settings through the app. fn gets a copy whose lists
// it may change in place; nil lists stay nil, so the defaults they stand
// for stay defaults.
func (a *App) apply(fn func(s *settings.Settings)) {
	next := a.Store.Settings
	next.Devices = slices.Clone(next.Devices)
	next.MenuBar.Fields = slices.Clone(next.MenuBar.Fields)
	next.Panel.Cards = slices.Clone(next.Panel.Cards)
	fn(&next)
	a.Act.ApplySettings(next)
}
