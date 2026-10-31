package app

import (
	"math"

	"github.com/egoist/mygo"

	"github.com/LYJW131/anker-prime-desktop/internal/settings"
	"github.com/LYJW131/anker-prime-desktop/internal/store"
	appui "github.com/LYJW131/anker-prime-desktop/internal/ui"
	"github.com/LYJW131/anker-prime-desktop/locales"
)

// The methods below implement ui.Shell.

// ApplySettings saves the settings, puts them in the store, and has the
// backend, the menu bar item, the theme and the Dock follow them. A run
// capturing screens keeps them in memory.
func (a *App) ApplySettings(s settings.Settings) {
	langChanged := s.Language != a.store.Settings.Language
	if a.opts.Shots == "" {
		if err := settings.Save(a.dir, s); err != nil {
			a.log.Error("settings not saved", "error", err)
			a.store.Note(store.NoticeSettingsNotSaved, "", err)
		}
	}
	// The backend compares the settings it is handed with the store's.
	a.be.SaveSettings(s)
	a.store.Settings = s
	if langChanged {
		a.followLanguage(s.Language)
	}
	a.syncTray()
	a.syncDock()
	if a.panel != nil {
		a.panel.Invalidate()
	}
}

// SetSidebarHidden keeps the sidebar's state for the next launch.
func (a *App) SetSidebarHidden(bool) { a.saveViewState() }

// ShowWindow brings the main window forward and closes the tray panel.
func (a *App) ShowWindow() {
	if a.panel != nil {
		a.panel.Close()
	}
	a.showWindow()
}

// FitPanel sizes the tray panel to the height its content laid out.
func (a *App) FitPanel(height float32) {
	if a.panel != nil {
		a.panelH = int(math.Ceil(float64(height)))
		a.panel.SetContentSize(appui.PanelWidth, a.panelH)
	}
}

// ChooseImage shows the system's file chooser for a picture, off the UI
// thread.
func (a *App) ChooseImage(done func(string)) {
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent:  a.win,
			Title:   locales.ChooseCoverTitle(),
			Filters: []mygo.FileFilter{{Name: locales.ChooseCoverFilter(), Extensions: []string{"jpg", "jpeg", "png", "gif", "webp"}}},
		})
		if err != nil {
			a.log.Warn("file chooser", "error", err)
			return
		}
		if len(paths) > 0 {
			a.post(func() { done(paths[0]) })
		}
	}()
}

// Quit ends the app.
func (a *App) Quit() {
	a.quitting = true
	mygo.App.Quit()
}
