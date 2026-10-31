package ui

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/LYJW131/anker-prime-desktop/internal/link"
	"github.com/LYJW131/anker-prime-desktop/internal/model"
	"github.com/LYJW131/anker-prime-desktop/internal/settings"
	"github.com/LYJW131/anker-prime-desktop/internal/store"
	"github.com/LYJW131/anker-prime-desktop/locales"
)

// addForm is Add Device's state between frames.
type addForm struct {
	// order is the found devices in the order they show: a row never
	// moves once it shows, so the list only grows downward. The finds of
	// a scan's first moment show together, strongest first; later ones
	// join at the foot.
	order []string
	// from is when the scan started; restart is set until its first
	// finds show, while the rows of the scan before (last) stay.
	from    time.Time
	restart bool
	last    []link.Found
}

// startScan starts a scan for Add Device. What the last one found stays
// until this one's first finds show.
func (a *App) startScan() {
	a.add.from, a.add.restart = a.Store.Now(), true
	a.Dev.Scan()
}

// rowHeight is a found device's row: fixed, so rows arriving move nothing.
const rowHeight = 56

// addPage is Add Device: what the scan finds, each supported model with
// the way to add it.
func (a *App) addPage(c *ui.Context, k *Tokens) {
	s := a.Store
	found := a.foundShown(c)
	a.pageScroll(c, k, "add", func() {
		pageHead(c, k, locales.NavAddDevice(), nil, func() {
			if s.Scanning {
				if button(c, k, buttonGhost, locales.AddStop(), nil).Clicked() {
					a.Dev.StopScan()
				}
			} else if button(c, k, buttonPlain, locales.AddRescan(), icSync).Clicked() {
				a.startScan()
			}
		})
		ui.Column(c.Key("found")).Padding(padControl).Gap(padControl).Radius(radiusMenu).Background(k.Raise).Children(func() {
			ui.Row(c.Key("scan-head")).Height(40).Gap(10).AlignItems(ui.Center).PaddingX(12).Children(func() {
				ui.Box(c).Size(10, 10).Center().Children(func() {
					if s.Scanning {
						d := dot(c, k.Accent, 8)
						d.Opacity(0.35 + 0.65*d.Loop("scan", 1200*time.Millisecond, ui.Bounce(ui.EaseInOut)))
					} else {
						dot(c, k.Track.Mix(k.Fg, 0.2), 8)
					}
				})
				text := locales.AddScanning()
				switch {
				case s.Scanning:
				case len(found) == 0:
					text = locales.AddNoneFound()
				default:
					text = locales.AddStopped()
				}
				ui.Text(c, text).FontSize(12.5).TextColor(k.Muted).SingleLine().Shrink(1).Ellipsis("…")
			})
			for _, fd := range found {
				a.foundRow(c, k, fd)
			}
		})
	})
}

// foundShown is what the scan found, in the order the rows show
// (addForm.order).
func (a *App) foundShown(c *ui.Context) []link.Found {
	f := &a.add
	scanning := a.Store.Scanning
	reveal := !scanning || waited(c, true, f.from)
	if f.restart {
		if !reveal {
			return f.last
		}
		f.order, f.restart = nil, false
	}
	byID := map[string]link.Found{}
	for _, fd := range a.Store.Found {
		byID[fd.ID] = fd
	}
	if reveal {
		var fresh []link.Found
		for _, fd := range a.Store.Found {
			if !slices.Contains(f.order, fd.ID) {
				fresh = append(fresh, fd)
			}
		}
		slices.SortStableFunc(fresh, func(x, y link.Found) int { return y.RSSI - x.RSSI })
		for _, fd := range fresh {
			f.order = append(f.order, fd.ID)
		}
	}
	out := make([]link.Found, 0, len(f.order))
	for _, id := range f.order {
		if fd, ok := byID[id]; ok {
			out = append(out, fd)
		}
	}
	f.last = out
	return out
}

// foundRow is a device the scan found: a supported model's name with the
// way to add it, or the model byte of one this app does not know yet.
func (a *App) foundRow(c *ui.Context, k *Tokens, fd link.Found) {
	m, ok := model.ByID(model.ID(fd.Model))
	_, added := a.Store.Settings.Device(fd.ID)
	added = added || fd.Added
	ui.Row(c.Key(fd.ID)).Height(rowHeight).Gap(12).AlignItems(ui.Center).Padding(0, 10, 0, 10).Radius(radiusControl).Background(k.Wash).Children(func() {
		if ok {
			ic, col := familyLook(k, m.Family)
			tile(c, ic, col, 32)
		} else {
			ui.Box(c).Size(32, 32).Radius(radiusInner).Background(k.Track).Center().Children(func() {
				ui.Icon(c, icChip).FontSize(18).TextColor(k.Faint)
			})
		}
		ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(3).Children(func() {
			title := ""
			if ok {
				title = ModelName(m.ID)
			} else if fd.ModelByte >= 0 {
				title = locales.ModelByte(fmt.Sprintf("0x%02X", fd.ModelByte))
			} else {
				title = orNone(fd.Name)
			}
			t := ui.Text(c, title).FontSize(13).FontWeight(600).SingleLine().Ellipsis("…")
			if !ok {
				t.TextColor(k.Muted)
			}
			sub := []string{}
			if fd.Name != "" {
				sub = append(sub, fd.Name)
			}
			sub = append(sub, fmt.Sprintf("%d dBm", fd.RSSI))
			mono(c, strings.Join(sub, " · "), 11).TextColor(k.Faint).Ellipsis("…")
		})
		rssiBars(c, k, fd.RSSI)
		// The three ends of a row in one slot as wide as the widest, so the
		// signal bars line up down the list.
		ui.Box(c.Key("end")).Children(func() {
			ui.Column(c).AlignItems(ui.End).Children(func() {
				pill(c, k, pillWait, nil, locales.Unsupported()).Invisible().Height(0)
				pill(c, k, pillOK, icCheck, locales.AddAdded()).Invisible().Height(0)
				ui.Row(c).PaddingX(12).Height(0).Children(func() {
					ui.Text(c, locales.AddAdd()).FontSize(12.5).FontWeight(650).SingleLine()
				}).Invisible()
			})
			ui.Row(c).Absolute().Top(0).Bottom(0).Left(0).Right(0).Justify(ui.End).AlignItems(ui.Center).Children(func() {
				switch {
				case !ok:
					pill(c, k, pillWait, nil, locales.Unsupported())
				case added:
					pill(c, k, pillOK, icCheck, locales.AddAdded())
				default:
					if button(c, k, buttonPrimary, locales.AddAdd(), nil).Clicked() {
						a.addDevice(fd, m)
					}
				}
			})
		}).Height(30)
	})
}

// addDevice adds a found device and opens its page, which ends the scan.
func (a *App) addDevice(fd link.Found, m model.Model) {
	d := settings.Device{ID: fd.ID, Model: string(m.ID), Name: fd.Name, Enabled: true, IdleSleep: m.IdleSleep}
	a.apply(func(n *settings.Settings) {
		if _, dup := n.Device(fd.ID); !dup {
			n.Devices = append(n.Devices, d)
		}
	})
	a.Store.Note(store.NoticeDeviceAdded, ModelShort(m.ID), nil)
	a.GoDevice(fd.ID, TabLive)
}

// shortID is a CoreBluetooth identifier's first group, enough to tell two
// apart.
func shortID(id string) string {
	if i := strings.IndexByte(id, '-'); i > 0 {
		return id[:i]
	}
	return id
}

// rssiBars is a signal's strength in four bars.
func rssiBars(c *ui.Context, k *Tokens, rssi int) {
	lit := 0
	for _, t := range []int{-90, -80, -70, -60} {
		if rssi >= t {
			lit++
		}
	}
	ui.Row(c.Key("rssi")).Height(14).Gap(2).AlignItems(ui.End).Children(func() {
		for i := range 4 {
			col := k.Track
			if i < lit {
				col = k.Fg.Alpha(0.75)
			}
			ui.Box(c.Key(i)).Size(3, float32(5+3*i)).Radius(1).Background(col)
		}
	})
}
