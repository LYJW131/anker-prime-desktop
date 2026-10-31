package app

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/egoist/mygo"

	"github.com/LYJW131/anker-prime-mac/internal/link"
	"github.com/LYJW131/anker-prime-mac/internal/model"
	appui "github.com/LYJW131/anker-prime-mac/internal/ui"
)

// holder is a backend whose devices can be held in a phase, as the
// demo's, for the shots of the states.
type holder interface {
	Hold(id string, hold bool)
}

// captureScreens walks the screens and writes each as a PNG into dir with
// Window.CapturePage, which renders the view without screen recording
// rights, then quits. It runs on its own goroutine.
func (a *App) captureScreens(dir string) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		a.log.Error("shots", "error", err)
		mygo.RunOnMain(mygo.App.Quit)
		return
	}
	time.Sleep(1500 * time.Millisecond)
	if a.opts.Live {
		a.waitStreaming(30 * time.Second)
	}
	shot := func(name string, setup func(), settle time.Duration) {
		a.post(setup)
		time.Sleep(settle)
		img, err := a.win.CapturePage()
		if err == nil {
			err = os.WriteFile(filepath.Join(dir, name+".png"), img, 0o644)
		}
		if err != nil {
			a.log.Error("capture", "shot", name, "error", err)
			return
		}
		fmt.Println("wrote", filepath.Join(dir, name+".png"))
	}
	size := func(w, h int) func() { return func() { a.win.SetContentSize(w, h) } }
	// The demo's devices, by family.
	var charger, bank string
	for _, d := range a.store.Settings.Devices {
		m, _ := model.ByID(model.ID(d.Model))
		switch {
		case m.Family == model.FamilyCharger && charger == "":
			charger = d.ID
		case m.Family == model.FamilyPowerBank && bank == "":
			bank = d.ID
		}
	}
	page := func(p appui.Page) func() { return func() { a.view.Go(p) } }
	dev := func(id string, t appui.Tab) func() { return func() { a.view.GoDevice(id, t) } }
	settingsPage := appui.Page{Kind: appui.PageSettings}
	addPage := appui.Page{Kind: appui.PageAddDevice}

	themes := []string{"dark", "light"}
	other := a.opts.Lang != "" && a.opts.Lang != "zh-Hans"
	if other {
		// Another language: the main pages, for how its words fit.
		themes = []string{"dark"}
	}
	for _, theme := range themes {
		a.post(func() { a.view.Theme = theme })
		a.post(size(1280, 800))
		shot(theme+"-01-overview", page(appui.Page{}), 900*time.Millisecond)
		shot(theme+"-02-charger", dev(charger, appui.TabLive), 900*time.Millisecond)
		shot(theme+"-03-charger-covers", dev(charger, appui.TabCovers), 900*time.Millisecond)
		shot(theme+"-04-charger-settings", dev(charger, appui.TabSettings), 900*time.Millisecond)
		shot(theme+"-05-powerbank", dev(bank, appui.TabLive), 900*time.Millisecond)
		if !other {
			shot(theme+"-06-powerbank-settings", dev(bank, appui.TabSettings), 900*time.Millisecond)
		}
		// The scan's finds arrive over two seconds.
		shot(theme+"-07-add-device", page(addPage), 2800*time.Millisecond)
		shot(theme+"-08-settings", page(settingsPage), 900*time.Millisecond)
		shot(theme+"-09-settings-custom", func() {
			a.win.SetContentSize(1280, 1300)
		}, 1200*time.Millisecond)
		a.post(size(820, 560))
		shot(theme+"-10-narrow-overview", page(appui.Page{}), 1200*time.Millisecond)
		shot(theme+"-11-narrow-charger", dev(charger, appui.TabLive), 900*time.Millisecond)
		if !other {
			shot(theme+"-12-narrow-add-device", page(addPage), 2800*time.Millisecond)
		}
		a.shotPanel(filepath.Join(dir, theme+"-13-panel.png"))
	}
	if other {
		a.quitShots()
		return
	}

	// The states, in the dark theme at the usual size.
	a.post(func() { a.view.Theme = "dark"; a.win.SetContentSize(1280, 800) })
	if hold, _ := a.be.(holder); hold != nil {
		shot("state-1-pushing", func() {
			a.view.GoDevice(charger, appui.TabCovers)
			var cur *int
			if ch := a.store.Device(charger).Charger; ch != nil {
				cur = ch.ScreensaverID
			}
			for _, it := range a.store.Covers {
				if cur == nil || it.ID&0xffff != *cur {
					a.store.CoverJob = &link.CoverJob{ID: it.ID, Done: 23, Total: 59}
					break
				}
			}
		}, 900*time.Millisecond)
		shot("state-2-charger-connecting", func() {
			a.store.CoverJob = nil
			hold.Hold(charger, true)
			a.store.Device(charger).Status = link.Status{Phase: link.PhaseConnecting, Name: "ASHDJW7CF49200487"}
			a.view.GoDevice(charger, appui.TabLive)
		}, 900*time.Millisecond)
		shot("state-3-powerbank-stale", func() {
			hold.Hold(bank, true)
			a.store.Device(bank).Status.LastFrame = a.store.Now().Add(-40 * time.Second)
			a.view.GoDevice(bank, appui.TabLive)
		}, 900*time.Millisecond)
		shot("state-4-overview-states", func() {
			a.store.Device(charger).Status = link.Status{Phase: link.PhaseBluetoothOff}
			a.store.Device(bank).Status = link.Status{Phase: link.PhaseSleeping, Name: "AJ7DLCH0F49600286"}
			a.view.Go(appui.Page{})
		}, 900*time.Millisecond)
		shot("state-5-powerbank-sleeping", dev(bank, appui.TabLive), 900*time.Millisecond)
		shot("state-6-charger-disconnected", func() {
			a.store.Device(charger).Status = link.Status{Phase: link.PhaseDisconnected, Err: link.ErrStalled, Name: "ASHDJW7CF49200487"}
			a.view.GoDevice(charger, appui.TabLive)
		}, 900*time.Millisecond)
		a.shotPanel(filepath.Join(dir, "state-7-panel.png"))
	}
	// No devices: the overview's way in, both themes.
	saved := a.store.Settings
	a.post(func() {
		next := saved
		next.Devices = nil
		next.MenuBar.Fields, next.Panel.Cards = nil, nil
		a.ApplySettings(next)
	})
	for _, theme := range []string{"dark", "light"} {
		shot(theme+"-14-empty", func() { a.view.Theme = theme; a.view.Go(appui.Page{}) }, 900*time.Millisecond)
	}
	a.quitShots()
}

func (a *App) quitShots() {
	time.Sleep(300 * time.Millisecond)
	mygo.RunOnMain(func() { a.quitting = true; mygo.App.Quit() })
}

// shotPanel opens the tray panel, writes it to path and closes it.
func (a *App) shotPanel(path string) {
	opened := make(chan *mygo.Window, 1)
	a.post(func() {
		if a.panel == nil {
			a.togglePanel()
		}
		opened <- a.panel
	})
	p := <-opened
	if p == nil {
		return
	}
	time.Sleep(2 * time.Second)
	img, err := p.CapturePage()
	if err == nil {
		err = os.WriteFile(path, img, 0o644)
	}
	if err != nil {
		a.log.Error("capture", "shot", "panel", "error", err)
	} else {
		fmt.Println("wrote", path)
	}
	a.post(func() {
		if a.panel != nil {
			a.panel.Close()
		}
	})
	time.Sleep(500 * time.Millisecond)
}

// waitStreaming waits until every added device has sent a frame, or d.
func (a *App) waitStreaming(d time.Duration) {
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		ready := make(chan bool, 1)
		a.post(func() {
			ok := true
			for _, dev := range a.store.Settings.Devices {
				st := a.store.Device(dev.ID).Status
				ok = ok && st.Phase == link.PhaseConnected && !st.LastFrame.IsZero()
			}
			ready <- ok
		})
		if <-ready {
			time.Sleep(3 * time.Second) // a few seconds of history for the sparklines
			return
		}
		time.Sleep(300 * time.Millisecond)
	}
	a.log.Warn("shots: devices not streaming yet, capturing anyway")
}
