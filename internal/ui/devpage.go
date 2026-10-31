package ui

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/LYJW131/anker-prime-mac/internal/link"
	"github.com/LYJW131/anker-prime-mac/internal/model"
	"github.com/LYJW131/anker-prime-mac/internal/settings"
	"github.com/LYJW131/anker-prime-mac/locales"
)

// devForm is the device pages' state between frames.
type devForm struct {
	// name is the nickname being edited for nameFor; nameFocus whether its
	// field had the focus last frame.
	name      string
	nameFor   string
	nameFocus bool
	// removing is the device the remove dialog asks about.
	removing   string
	removeOpen bool
}

// tabsOf are the tabs of a device's page, by its model.
func tabsOf(m model.Model) []Tab {
	if m.Covers {
		return []Tab{TabLive, TabCovers, TabSettings}
	}
	return []Tab{TabLive, TabSettings}
}

func tabName(t Tab) string {
	switch t {
	case TabCovers:
		return locales.TabCovers()
	case TabSettings:
		return locales.TabSettings()
	}
	return locales.TabLive()
}

// tab is the tab a device's page shows: the one last chosen, if the model
// has it.
func (a *App) tab(d dev) Tab {
	t := a.tabs[d.ID()]
	for _, x := range tabsOf(d.m) {
		if x == t {
			return t
		}
	}
	return TabLive
}

// devicePage is an added device's page: its name and state, and the tab
// chosen.
func (a *App) devicePage(c *ui.Context, k *Tokens) {
	d, ok := a.device(a.Page.Device)
	if !ok {
		// Removed from under the page.
		a.Page = Page{}
		a.overviewPage(c, k)
		return
	}
	s := a.state(c, d)
	t := a.tab(d)
	a.pageScroll(c, k, fmt.Sprintf("device-%s-%d", d.ID(), t), func() {
		pageHead(c, k, d.name(), func() { a.statePill(c, k, s) }, func() {
			if t == TabCovers {
				a.addCoverButton(c, k)
			}
			tabs := tabsOf(d.m)
			names := make([]string, len(tabs))
			sel := 0
			for i, x := range tabs {
				names[i] = tabName(x)
				if x == t {
					sel = i
				}
			}
			prev := sel
			segmented(c, k, "tabs", &sel, names)
			if sel != prev {
				a.GoDevice(d.ID(), tabs[sel])
			}
		})
		switch t {
		case TabCovers:
			a.coversTab(c, k, d, s)
		case TabSettings:
			a.deviceSettings(c, k, d, s)
		default:
			switch {
			case s.dead:
				a.deadCard(c, k, d, s)
			case d.m.Family == model.FamilyPowerBank:
				a.bankLive(c, k, d, s)
			default:
				a.chargerLive(c, k, d, s)
			}
		}
	})
}

// deviceSettings is a device's own settings: its name, whether it stays
// connected, idle sleep where the model allows, its connection, what it
// is, and the way to remove it.
func (a *App) deviceSettings(c *ui.Context, k *Tokens, d dev, s devState) {
	id := d.ID()
	cols := 2
	if a.compact {
		cols = 1
	}
	ui.Grid(c.Key("dev-settings")).Columns(cols).Gap(14).Children(func() {
		card(c.Key("general"), k).Gap(10).Children(func() {
			ic, col := familyLook(k, d.m.Family)
			sectionHead(c, k, ic, col, locales.DevGeneral(), nil)
			optionRow(c, k, "name", locales.DevName(), func() { a.nicknameField(c, k, d) })
			optionRow(c, k, "keep", locales.DevKeepConnected(), func() {
				on := d.set.Enabled
				if toggle(c, k, "enabled", &on, locales.DevKeepConnected()).Changed() {
					a.setEnabled(id, on)
				}
			})
			if d.m.IdleSleep {
				optionRow(c, k, "idle", locales.DevIdleSleep(), func() {
					on := d.set.IdleSleep
					if toggle(c, k, "idle-sleep", &on, locales.DevIdleSleep()).Changed() {
						a.apply(func(n *settings.Settings) {
							if sd, ok := n.Device(id); ok {
								sd.IdleSleep = on
							}
						})
					}
				})
			}
			optionRow(c, k, "conn", locales.DevConnection(), func() {
				a.statePill(c, k, s)
				// One slot for whichever button the phase calls for, so the
				// row keeps its height and the pill its place.
				switch {
				case !d.set.Enabled:
				case s.phase == link.PhaseConnected:
					if button(c, k, buttonPlain, locales.ConnDisconnect(), nil).Clicked() {
						a.Dev.Disconnect(id)
					}
				case s.phase == link.PhaseDisconnected || s.phase == link.PhaseSleeping:
					if button(c, k, buttonPlain, locales.ConnConnect(), icSync).Clicked() {
						a.Dev.Connect(id)
					}
				}
			})
		})
		card(c.Key("about"), k).Gap(10).Children(func() {
			sectionHead(c, k, icChip, k.TileSettings, locales.DevAbout(), nil)
			factList(c, k, []fact{
				{nil, locales.FactProduct(), ModelName(d.m.ID), false},
				{nil, locales.FactModel(), string(d.m.ID), true},
				{nil, locales.FactSerial(), orNone(d.set.Name), true},
				{nil, locales.FactBluetoothID(), shortID(id), true},
			}).BorderWidth(1, 0, 0, 0).BorderColor(k.Line)
		})
	})
	ui.Row(c.Key("remove-row")).Children(func() {
		if button(c, k, buttonDestructive, locales.DevRemove(), icTrash).Clicked() {
			a.dev.removing, a.dev.removeOpen = id, true
		}
	})
}

// nicknameField edits a device's nickname, saved on Return or when the
// field loses the focus; empty goes back to the model's name.
func (a *App) nicknameField(c *ui.Context, k *Tokens, d dev) {
	f := &a.dev
	id := d.ID()
	if f.nameFor != id {
		f.nameFor, f.name, f.nameFocus = id, d.set.Nickname, false
	}
	box, in := fieldBox(c, k, "nickname", &f.name, ModelShort(d.m.ID), false)
	box.Grow(0).Shrink(0).Width(260)
	focused := in.Focused()
	commit := in.Submitted() || (f.nameFocus && !focused)
	f.nameFocus = focused
	name := strings.TrimSpace(f.name)
	switch {
	case commit && name != d.set.Nickname:
		a.apply(func(n *settings.Settings) {
			if sd, ok := n.Device(id); ok {
				sd.Nickname = name
			}
		})
	case !focused:
		f.name = d.set.Nickname
	}
}

// deviceDialogs is the dialog that removes a device.
func (a *App) deviceDialogs(c *ui.Context, k *Tokens) {
	f := &a.dev
	if f.removing == "" {
		return
	}
	d, ok := a.device(f.removing)
	if !ok {
		f.removing, f.removeOpen = "", false
		return
	}
	dialog(c, k, "remove-device", &f.removeOpen, locales.ConfirmRemove(d.name()), locales.RemoveDeviceBody(), nil, func() {
		if button(c, k, buttonPlain, locales.Cancel(), nil).Clicked() {
			f.removeOpen = false
		}
		if button(c, k, buttonDanger, locales.Remove(), nil).Clicked() {
			id := d.ID()
			f.removeOpen = false
			a.Go(Page{})
			a.apply(func(n *settings.Settings) { removeDevice(n, id) })
		}
	})
	if !f.removeOpen {
		f.removing = ""
	}
}
