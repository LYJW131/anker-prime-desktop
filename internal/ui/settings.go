package ui

import (
	"errors"
	"fmt"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/LYJW131/anker-prime-desktop/internal/settings"
	"github.com/LYJW131/anker-prime-desktop/internal/store"
	"github.com/LYJW131/anker-prime-desktop/locales"
)

// settingsForm is the Settings page's state between frames.
type settingsForm struct {
	// acct is the account card's way in, among the tabs shown: import
	// (macOS only), paste or sign in.
	acct              int
	pasteID           string
	account, password string
	// busy is the account request on its way since busyFrom.
	busy     bool
	busyFrom time.Time
	// done and failed are the last account request's outcome, kept until
	// the next one's, and worded as it shows, in the language of the
	// moment.
	done   acctResult
	failed error
}

// acctResult is an account request that went through.
type acctResult int

const (
	acctNone acctResult = iota
	acctImported
	acctSaved
	acctLoggedIn
)

var themeNames = []string{"system", "light", "dark"}

// languages are the languages Settings offers, "" following the system.
// Each is named in itself.
var languages = []struct{ tag, name string }{
	{"", ""},
	{"en", "English"},
	{"zh-Hans", "简体中文"},
	{"ja", "日本語"},
}

func (a *App) settingsPage(c *ui.Context, k *Tokens) {
	cols := 2
	if a.compact {
		cols = 1
	}
	a.pageScroll(c, k, "settings", func() {
		pageHead(c, k, locales.NavSettings(), nil, nil)
		ui.Grid(c.Key("more")).Columns(cols).Gap(14).Children(func() {
			a.accountCard(c, k)
			a.generalCard(c, k)
		})
		ui.Grid(c.Key("custom")).Columns(cols).Gap(14).Children(func() {
			a.menuBarCard(c, k)
			a.panelSettingsCard(c, k)
		})
	})
}

// sectionHead is a settings card's first line.
func sectionHead(c *ui.Context, k *Tokens, ic *ui.SVG, col ui.Color, title string, right func()) {
	ui.Row(c.Key("section-head")).Height(26).Gap(10).AlignItems(ui.Center).Children(func() {
		tile(c, ic, col, 22)
		ui.Text(c, title).FontSize(13.5).FontWeight(650).SingleLine().Shrink(1).MinWidth(0).Ellipsis("…")
		ui.Spacer(c)
		if right != nil {
			right()
		}
	})
}

// optionRow is a setting's name with its control at the right.
func optionRow(c *ui.Context, k *Tokens, key, name string, control func()) {
	ui.Row(c.Key(key)).Height(40).Gap(12).AlignItems(ui.Center).BorderWidth(1, 0, 0, 0).BorderColor(k.Line).Children(func() {
		ui.Text(c, name).FontSize(13).SingleLine().Shrink(1).MinWidth(0).Ellipsis("…")
		ui.Spacer(c)
		control()
	})
}

// fieldsList is the menu bar's list in n, the defaults written out when
// it was nil, so an edit starts from what shows.
func fieldsList(n *settings.Settings) *[]string {
	if n.MenuBar.Fields == nil {
		n.MenuBar.Fields = append([]string{}, EffectiveFields(n)...)
	}
	return &n.MenuBar.Fields
}

// cardsList is the panel's list in n, the defaults written out when it was
// nil.
func cardsList(n *settings.Settings) *[]string {
	if n.Panel.Cards == nil {
		n.Panel.Cards = append([]string{}, EffectiveCards(n)...)
	}
	return &n.Panel.Cards
}

// menuBarCard chooses and orders the values the menu bar shows, from any
// added device, with the title as it shows now.
func (a *App) menuBarCard(c *ui.Context, k *Tokens) {
	st := &a.Store.Settings
	mb := st.MenuBar
	shown := EffectiveFields(st)
	devs := a.devices()
	card(c.Key("menubar"), k).Gap(10).Children(func() {
		sectionHead(c, k, icMenuBar, k.TileCharger, locales.SetMenuBar(), nil)
		a.menuBarPreview(c, k)
		optionRow(c, k, "icon", locales.SetShowIcon(), func() {
			on := mb.ShowIcon
			if toggle(c, k, "show-icon", &on, locales.SetShowIcon()).Changed() {
				a.apply(func(n *settings.Settings) { n.MenuBar.ShowIcon = on })
			}
		})
		label(c, k, locales.SetShownValues()).Margin(6, 0, 0, 0)
		ui.Column(c.Key("chosen")).Gap(2).Children(func() {
			n := 0
			for i, r := range shown {
				id, fid := splitRef(r)
				d, ok := a.device(id)
				if !ok {
					continue
				}
				f, ok := fieldOf(d.m, fid)
				if !ok {
					continue
				}
				n++
				a.orderRow(c, k, r, d.shortName(), f.Label, sampleText(a.Store, r, c.Now()), i, len(shown), fieldsList)
			}
			if n == 0 {
				emptyRow(c, k, locales.SetIconOnly())
			}
		})
		a.catalog(c, k, "field-catalog", devs, func(d dev) []Field {
			var rest []Field
			for _, f := range FieldsOf(d.m) {
				if !slices.Contains(shown, ref(d.ID(), f.ID)) {
					rest = append(rest, f)
				}
			}
			return rest
		}, fieldsList)
	})
}

// emptyRow stands in an ordered list with nothing in it.
func emptyRow(c *ui.Context, k *Tokens, text string) {
	ui.Row(c).Height(34).PaddingX(10).AlignItems(ui.Center).Radius(radiusControl).Background(k.Wash).Children(func() {
		ui.Text(c, text).FontSize(12.5).TextColor(k.Faint).SingleLine()
	})
}

// catalog offers what is not chosen yet, grouped by device: a chip each,
// which a click appends to the list.
func (a *App) catalog(c *ui.Context, k *Tokens, key string, devs []dev, rest func(dev) []Field, list func(*settings.Settings) *[]string) {
	any := false
	for _, d := range devs {
		if len(rest(d)) > 0 {
			any = true
		}
	}
	if !any {
		return
	}
	label(c, k, locales.AddAdd()).Margin(6, 0, 0, 0)
	ui.Column(c.Key(key)).Gap(10).Children(func() {
		for _, d := range devs {
			fs := rest(d)
			if len(fs) == 0 {
				continue
			}
			ui.Column(c.Key(d.ID())).Gap(6).Children(func() {
				ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
					ic, col := familyLook(k, d.m.Family)
					tile(c, ic, col, 16)
					ui.Text(c, d.shortName()).FontSize(12).FontWeight(600).TextColor(k.Muted).SingleLine().Shrink(1).Ellipsis("…")
				})
				ui.Row(c).Gap(6).Wrap().Children(func() {
					for _, f := range fs {
						r := ref(d.ID(), f.ID)
						b := ui.ButtonBase(c.Key(r)).Height(26).PaddingX(9).Gap(5).Radius(radiusFull).AlignItems(ui.Center).
							Border(1, k.Line2).Label(locales.SetAddItem(d.shortName() + " · " + f.Label))
						if b.Hovered() {
							b.Background(k.Wash)
						}
						if b.Clicked() {
							a.apply(func(n *settings.Settings) {
								l := list(n)
								*l = append(*l, r)
							})
						}
						b.Cursor(ui.CursorPointer).Children(func() {
							ui.Icon(c, icPlus).FontSize(11).TextColor(k.Muted)
							ui.Text(c, f.Label).FontSize(12).TextColor(k.Muted).SingleLine()
						})
					}
				})
			})
		}
	})
}

// menuBarPreview draws the menu bar item as the menu bar shows it now.
func (a *App) menuBarPreview(c *ui.Context, k *Tokens) {
	mb := a.Store.Settings.MenuBar
	fields := EffectiveFields(&a.Store.Settings)
	bg, fg := ui.Hex("#2a2b33"), ui.Hex("#f2f3f7")
	if !k.Dark {
		bg, fg = ui.Hex("#e7e8ee"), ui.Hex("#1b1c22")
	}
	title := TrayTitle(a.Store, fields, c.Now())
	ui.Row(c.Key("preview")).Height(40).PaddingX(14).Gap(14).Radius(radiusControl).Background(bg).AlignItems(ui.Center).Justify(ui.End).Clip().Children(func() {
		ui.Row(c.Key("item")).Height(24).PaddingX(8).Gap(6).Radius(radiusInner).Background(fg.Alpha(0.12)).AlignItems(ui.Center).Shrink(1).MinWidth(0).Children(func() {
			if mb.ShowIcon || title == "" {
				a.trayGlyph(c, fg)
			}
			if title != "" {
				ui.Text(c, title).FontSize(13).FontWeight(500).FontFeatures("tnum").TextColor(fg).SingleLine().Shrink(1).MinWidth(0).Ellipsis("…")
			}
		})
	})
}

// trayGlyph is the tray icon as a view: a power bank's battery, filled to
// its level, as TrayIcon draws it.
func (a *App) trayGlyph(c *ui.Context, fg ui.Color) {
	level := TrayLevel(a.Store, c.Now())
	ui.Row(c.Key("glyph")).Gap(1).AlignItems(ui.Center).Children(func() {
		ui.Box(c).Size(19, 10).Radius(3).Border(1.2, fg.Alpha(0.9)).Padding(1.6).Children(func() {
			if level > 0 {
				ui.Box(c).Height(4.4).Width(15.4 * float32(level)).Radius(1.2).Background(fg.Alpha(0.9))
			}
		})
		ui.Box(c).Size(1.6, 4).Radius(1).Background(fg.Alpha(0.9))
	})
}

// orderRow is an item of an ordered list of refs: its device and name, a
// sample, and buttons moving it up, down or out.
func (a *App) orderRow(c *ui.Context, k *Tokens, r, device, name, sample string, i, n int, list func(*settings.Settings) *[]string) {
	move := func(to int) {
		a.apply(func(s *settings.Settings) {
			l := *list(s)
			if to >= 0 && to < len(l) && i < len(l) {
				l[i], l[to] = l[to], l[i]
			}
		})
	}
	ui.Row(c.Key(r)).Height(34).Gap(4).Padding(0, 2, 0, 10).AlignItems(ui.Center).Radius(radiusControl).Background(k.Wash).Children(func() {
		mono(c, fmt.Sprint(i+1), 11).TextColor(k.Faint).Width(16)
		ui.Row(c).Gap(6).Shrink(1).MinWidth(0).AlignItems(ui.Baseline).Children(func() {
			ui.Text(c, name).FontSize(12.5).SingleLine()
			ui.Text(c, device).FontSize(11.5).TextColor(k.Faint).SingleLine().Shrink(1).MinWidth(0).Ellipsis("…")
		})
		ui.Spacer(c)
		if sample != "" {
			mono(c, sample, 11.5).TextColor(k.Muted).Margin(0, 6, 0, 0)
		}
		if iconButton(c, k, icUp, locales.MoveUp(), i == 0).Clicked() {
			move(i - 1)
		}
		if iconButton(c, k, icDown, locales.MoveDown(), i == n-1).Clicked() {
			move(i + 1)
		}
		if iconButton(c, k, icCross, locales.Remove(), false).Clicked() {
			a.apply(func(s *settings.Settings) {
				l := list(s)
				if i < len(*l) {
					*l = slices.Delete(*l, i, i+1)
				}
			})
		}
	})
}

// panelSettingsCard chooses the menu bar panel's cards and their order,
// from any added device.
func (a *App) panelSettingsCard(c *ui.Context, k *Tokens) {
	st := &a.Store.Settings
	cards := EffectiveCards(st)
	card(c.Key("panel"), k).Gap(10).Children(func() {
		sectionHead(c, k, icPanel, k.TileOverview, locales.SetPanel(), nil)
		label(c, k, locales.SetShownCards())
		ui.Column(c.Key("cards")).Gap(2).Children(func() {
			n := 0
			for i, r := range cards {
				id, cid := splitRef(r)
				d, ok := a.device(id)
				if !ok {
					continue
				}
				cd, ok := cardOf(d.m, cid)
				if !ok {
					continue
				}
				n++
				a.orderRow(c, k, r, d.shortName(), cd.Label, "", i, len(cards), cardsList)
			}
			if n == 0 {
				emptyRow(c, k, locales.SetOpenOnly())
			}
		})
		a.catalog(c, k, "card-catalog", a.devices(), func(d dev) []Field {
			var rest []Field
			for _, cd := range CardsOf(d.m) {
				if !slices.Contains(cards, ref(d.ID(), cd.ID)) {
					rest = append(rest, Field(cd))
				}
			}
			return rest
		}, cardsList)
	})
}

// accountCard holds the Anker account ID the devices' sessions need: its
// state, and three ways to set it.
func (a *App) accountCard(c *ui.Context, k *Tokens) {
	f := &a.set
	card(c.Key("account"), k).Gap(10).Children(func() {
		sectionHead(c, k, icKey, k.TileCover, locales.SetAccount(), func() {
			if a.Store.HasUserID {
				pill(c, k, pillOK, icCheck, locales.AcctSaved())
			} else {
				pill(c, k, pillWarn, nil, locales.AcctNotSet())
			}
		})
		// The Telemetry Center keeps the ID in the macOS Keychain only.
		ways, skip := []string{locales.AcctWayImport(), locales.AcctWayPaste(), locales.AcctWayLogin()}, 0
		if runtime.GOOS != "darwin" {
			ways, skip = ways[1:], 1
		}
		segmented(c, k, "acct-way", &f.acct, ways).AlignSelf(ui.Start)
		slow := waited(c, f.busy, f.busyFrom)
		done := func(ok acctResult) func(error) {
			return func(err error) {
				f.busy = false
				if err != nil && !errors.Is(err, store.ErrSessionOnly) {
					f.done, f.failed = acctNone, err
					return
				}
				f.done, f.failed = ok, err
				f.pasteID, f.password = "", ""
			}
		}
		start := func() {
			f.busy, f.busyFrom = true, c.Now()
		}
		// Each way in takes the same room, so switching moves nothing below.
		ui.Column(c.Key("acct-body")).Height(76).Gap(8).Justify(ui.Start).Children(func() {
			switch f.acct + skip {
			case 0:
				ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
					text := locales.AcctImport()
					if slow {
						text = locales.AcctImporting()
					}
					if buttonOff(c, k, buttonPlain, text, icArrowIn, slow).Clicked() && !f.busy {
						start()
						a.Dev.ImportUserID(done(acctImported))
					}
				})
			case 1:
				ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
					_, in := fieldBox(c, k, "paste-id", &f.pasteID, locales.AcctIDPlaceholder(), true)
					in.Password()
					text := locales.Save()
					if slow {
						text = locales.AcctSaving()
					}
					id := strings.TrimSpace(f.pasteID)
					if (buttonOff(c, k, buttonPrimary, text, nil, slow || id == "").Clicked() || in.Submitted()) && id != "" && !f.busy {
						start()
						a.Dev.SetUserID(id, done(acctSaved))
					}
				})
			case 2:
				ui.Row(c).Gap(8).Children(func() {
					fieldBox(c, k, "account", &f.account, locales.AcctAccount(), false)
					_, pw := fieldBox(c, k, "password", &f.password, locales.AcctPassword(), false)
					pw.Password()
				})
				ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
					text := locales.AcctLogin()
					if slow {
						text = locales.AcctLoggingIn()
					}
					off := slow || f.account == "" || f.password == ""
					if buttonOff(c, k, buttonPrimary, text, nil, off).Clicked() && !f.busy {
						start()
						a.Dev.Login(strings.TrimSpace(f.account), f.password, done(acctLoggedIn))
					}
					ui.Text(c, locales.AcctLoginNote()).FontSize(11.5).TextColor(k.Faint).SingleLine().Shrink(1).MinWidth(0).Ellipsis("…")
				})
			}
		})
		ui.Row(c.Key("acct-result")).Height(18).AlignItems(ui.Center).Children(func() {
			text, col := "", k.OK
			switch {
			case f.failed != nil:
				text, col = ErrText(f.failed), k.Bad
				if errors.Is(f.failed, store.ErrSessionOnly) {
					col = k.Warn
				}
			case f.done == acctImported:
				text = locales.AcctImported()
			case f.done == acctSaved:
				text = locales.AcctSaved()
			case f.done == acctLoggedIn:
				text = locales.AcctLoggedIn()
			}
			if text != "" {
				ui.Text(c, text).FontSize(12).TextColor(col).SingleLine().Ellipsis("…")
			}
		})
	})
}

// generalCard is the language, the theme and the Dock icon.
func (a *App) generalCard(c *ui.Context, k *Tokens) {
	st := a.Store.Settings
	card(c.Key("general"), k).Gap(10).Children(func() {
		sectionHead(c, k, icPalette, k.TilePowerBank, locales.SetGeneral(), nil)
		optionRow(c, k, "language", locales.SetLanguage(), func() {
			sel := 0
			names := make([]string, len(languages))
			tags := make([]string, len(languages))
			for i, l := range languages {
				names[i], tags[i] = l.name, l.tag
				if l.tag == "" {
					names[i] = locales.SetSystem()
				}
				if l.tag == st.Language {
					sel = i
				}
			}
			prev := sel
			segmentedLang(c, k, "language-seg", &sel, names, tags)
			if sel != prev {
				tag := languages[sel].tag
				a.apply(func(n *settings.Settings) { n.Language = tag })
			}
		})
		optionRow(c, k, "theme", locales.SetTheme(), func() {
			sel := slices.Index(themeNames, st.Theme)
			if sel < 0 {
				sel = 0
			}
			prev := sel
			segmented(c, k, "theme-seg", &sel, []string{locales.SetSystem(), locales.SetLight(), locales.SetDark()})
			if sel != prev {
				a.apply(func(n *settings.Settings) { n.Theme = themeNames[sel] })
			}
		})
		if runtime.GOOS == "darwin" {
			optionRow(c, k, "dock", locales.SetDock(), func() {
				on := st.ShowDock
				if toggle(c, k, "show-dock", &on, locales.SetDock()).Changed() {
					a.apply(func(n *settings.Settings) { n.ShowDock = on })
				}
			})
		}
	})
}
