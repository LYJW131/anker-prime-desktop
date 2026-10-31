package ui

import (
	"encoding/json"
	"errors"
	"image/png"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"

	"github.com/LYJW131/anker-prime-desktop/internal/demo"
	"github.com/LYJW131/anker-prime-desktop/internal/link"
	"github.com/LYJW131/anker-prime-desktop/internal/model"
	"github.com/LYJW131/anker-prime-desktop/internal/settings"
	"github.com/LYJW131/anker-prime-desktop/internal/store"
	"github.com/LYJW131/anker-prime-desktop/locales"
)

// The tests run in the catalogs' source language, English: no app sets
// the language, and the messages are said through locales, so they read
// the same in any language.

// fakeShell is the app around the views: settings go to the backend and
// the store, as the app's ApplySettings does.
type fakeShell struct {
	a       *App
	applied int
	fits    []float32
	shown   int
}

func (f *fakeShell) ApplySettings(s settings.Settings) {
	f.applied++
	f.a.Dev.SaveSettings(s)
	f.a.Store.Settings = s
}
func (f *fakeShell) SetSidebarHidden(bool)    {}
func (f *fakeShell) ShowWindow()              { f.shown++ }
func (f *fakeShell) FitPanel(h float32)       { f.fits = append(f.fits, h) }
func (f *fakeShell) ChooseImage(func(string)) {}
func (f *fakeShell) Quit()                    {}

// rig is an App on the demo backend, whose delayed steps queue up until
// flush runs them, as the UI thread would.
type rig struct {
	a     *App
	be    *demo.Backend
	sh    *fakeShell
	tt    *ui.Tester
	queue []func()
}

func (r *rig) flush() {
	for len(r.queue) > 0 {
		fn := r.queue[0]
		r.queue = r.queue[1:]
		fn()
	}
}

// newRig renders view (the window's or the panel's) at w×h in theme, with
// the demo's charger and power bank added.
func newRig(t *testing.T, theme string, w, h int, panel bool) *rig {
	t.Helper()
	r := &rig{}
	s := store.New(func(fn func()) { r.queue = append(r.queue, fn) })
	r.be = demo.New()
	r.be.Instant = true
	r.sh = &fakeShell{}
	r.a = &App{Store: s, Dev: r.be, Act: r.sh, Theme: theme, Opaque: true}
	r.sh.a = r.a
	view := r.a.View
	if panel {
		view = r.a.Panel
	}
	r.tt = ui.NewTester(view, w, h)
	s.SetClock(r.tt.Now)
	r.be.Start(s)
	r.flush()
	r.tt.Frame()
	return r
}

// save writes the frame where UI_SHOTS names a directory, for a look.
func (r *rig) save(t *testing.T, name string) {
	dir := os.Getenv("UI_SHOTS")
	if dir == "" {
		return
	}
	f, err := os.Create(filepath.Join(dir, name+".png"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, r.tt.Image()); err != nil {
		t.Fatal(err)
	}
}

func (r *rig) want(t *testing.T, texts ...string) {
	t.Helper()
	for _, s := range texts {
		if !r.tt.HasText(s) {
			t.Errorf("no %q in %q", s, r.tt.Texts())
		}
	}
}

// noNoise fails on decoded placeholders leaking into the screen.
func (r *rig) noNoise(t *testing.T) {
	t.Helper()
	for _, s := range r.tt.Texts() {
		if strings.Contains(s, "N/A") || strings.Contains(s, "<nil>") || strings.Contains(s, "%!") {
			t.Errorf("noise %q on screen", s)
		}
	}
}

func TestEveryPageInBothThemes(t *testing.T) {
	pages := []struct {
		name string
		go_  func(a *App)
		want []string
	}{
		{"overview", func(a *App) { a.Go(Page{}) }, []string{locales.NavOverview(), locales.ModelShortA2687(), "A2687", "A110G", locales.OverviewPorts()}},
		{"charger", func(a *App) { a.GoDevice(demo.ChargerID, TabLive) }, []string{strings.ToUpper(locales.ChargerTotal()), "USB-C1", "USB-C3", locales.FactSerial(), demo.ChargerName, "MacBook Pro series", "/ 160 W"}},
		{"covers", func(a *App) { a.GoDevice(demo.ChargerID, TabCovers) }, []string{locales.CoversAdd(), "晚霞", locales.CoverCurrent()}},
		{"charger-settings", func(a *App) { a.GoDevice(demo.ChargerID, TabSettings) }, []string{locales.DevKeepConnected(), locales.DevRemove(), "A2687"}},
		{"powerbank", func(a *App) { a.GoDevice(demo.PowerBankID, TabLive) }, []string{strings.ToUpper(locales.BankBattery()), "USB-A", locales.PortDock(), locales.FactHealth(), demo.PowerBankName}},
		{"powerbank-settings", func(a *App) { a.GoDevice(demo.PowerBankID, TabSettings) }, []string{locales.DevIdleSleep()}},
		{"settings", func(a *App) { a.GoSettings() }, []string{locales.SetMenuBar(), locales.SetPanel(), locales.SetAccount(), locales.SetLanguage(), "日本語"}},
	}
	for _, theme := range []string{"dark", "light"} {
		for _, p := range pages {
			r := newRig(t, theme, 1280, 800, false)
			p.go_(r.a)
			r.tt.Frame()
			r.want(t, p.want...)
			r.noNoise(t)
			r.save(t, theme+"-"+p.name)
		}
	}
	// A power bank has no covers tab.
	r := newRig(t, "dark", 1280, 800, false)
	r.a.GoDevice(demo.PowerBankID, TabCovers)
	r.tt.Frame()
	if r.tt.HasText(locales.CoversAdd()) {
		t.Error("a power bank's page shows covers")
	}
}

func TestSidebarListsTheAddedDevices(t *testing.T) {
	r := newRig(t, "dark", 1280, 800, false)
	r.want(t, locales.ModelShortA2687(), locales.ModelShortA110G(), locales.NavAddDevice(), locales.NavSettings())
	// A nickname replaces the model's name.
	r.a.apply(func(n *settings.Settings) { n.Devices[0].Nickname = "Desk" })
	r.flush()
	r.tt.Frame()
	r.want(t, "Desk")
	if err := r.tt.Click("Desk " + locales.StateConnected()); err != nil {
		t.Fatal(err)
	}
	if r.a.Page != (Page{Kind: PageDevice, Device: demo.ChargerID}) {
		t.Errorf("the sidebar row opened %+v", r.a.Page)
	}
}

func TestNoDevicesShowsTheWayToAddOne(t *testing.T) {
	r := newRig(t, "light", 1280, 800, false)
	r.a.apply(func(n *settings.Settings) { n.Devices = nil })
	r.flush()
	r.tt.Frame()
	r.want(t, locales.EmptyTitle())
	if r.tt.HasText(locales.ModelShortA2687()) {
		t.Error("a removed device stays in the sidebar")
	}
	r.save(t, "empty")
	// The empty card's button; the sidebar has the other.
	if err := r.tt.Click(locales.NavAddDevice()); err != nil {
		t.Fatal(err)
	}
	r.flush()
	if r.a.Page.Kind != PageAddDevice || !r.a.Store.Scanning {
		t.Errorf("the empty state opened %+v, scanning %v", r.a.Page, r.a.Store.Scanning)
	}
}

func TestAddDeviceListsWhatTheScanFinds(t *testing.T) {
	r := newRig(t, "dark", 1280, 800, false)
	r.a.Go(Page{Kind: PageAddDevice})
	r.flush()
	r.tt.Frame()
	// The first finds wait a moment, then show together.
	if r.tt.HasText(demo.OtherChargerName) {
		t.Fatal("finds show before the scan ran a moment")
	}
	r.tt.Advance(statusDelay)
	r.tt.Frame()
	r.want(t, locales.AddScanning(), demo.OtherChargerName, locales.AddAdded(), locales.ModelByte("0x0B"), locales.Unsupported())
	r.save(t, "add-device")
	// Strongest first, and a row keeps its place as signals change.
	y := func(s string) float32 { b, _ := r.tt.Find(s); return b.Y }
	if !(y(demo.ChargerName+" · -52 dBm") < y(demo.OtherChargerName+" · -71 dBm")) {
		t.Errorf("rows out of order: %q", r.tt.Texts())
	}
	before := y(locales.ModelByte("0x0B"))
	r.a.Store.Found[3].RSSI = -40
	r.tt.Frame()
	if y(locales.ModelByte("0x0B")) != before {
		t.Error("a row moved when its signal changed")
	}
	// Adding one opens its page and ends the scan.
	if err := r.tt.Click(locales.AddAdd()); err != nil {
		t.Fatal(err)
	}
	r.flush()
	r.tt.Frame()
	got := r.a.Store.Settings.Devices
	if len(got) != 3 || got[2].ID != demo.OtherChargerID || got[2].Model != string(model.A2687) || !got[2].Enabled {
		t.Fatalf("devices %+v", got)
	}
	if r.a.Page != (Page{Kind: PageDevice, Device: demo.OtherChargerID}) || r.a.Store.Scanning {
		t.Errorf("after adding: page %+v, scanning %v", r.a.Page, r.a.Store.Scanning)
	}
}

func TestRemovingADeviceDropsItsMenuBarValues(t *testing.T) {
	r := newRig(t, "dark", 1280, 800, false)
	r.a.apply(func(n *settings.Settings) {
		n.MenuBar.Fields = []string{ref(demo.ChargerID, "total"), ref(demo.PowerBankID, "battery")}
	})
	r.a.GoDevice(demo.ChargerID, TabSettings)
	r.tt.Frame()
	if err := r.tt.Click(locales.DevRemove()); err != nil {
		t.Fatal(err)
	}
	r.tt.Frame()
	r.want(t, locales.ConfirmRemove(locales.ModelA2687()))
	if err := r.tt.Click(locales.Remove()); err != nil {
		t.Fatal(err)
	}
	r.flush()
	r.tt.Frame()
	s := r.a.Store.Settings
	if len(s.Devices) != 1 || !slices.Equal(s.MenuBar.Fields, []string{ref(demo.PowerBankID, "battery")}) {
		t.Errorf("after removing: devices %+v, fields %q", s.Devices, s.MenuBar.Fields)
	}
	if r.a.Page.Kind != PageOverview {
		t.Errorf("the page of a removed device stays open: %+v", r.a.Page)
	}
}

func TestPanelFitsItsCards(t *testing.T) {
	for _, theme := range []string{"dark", "light"} {
		r := newRig(t, theme, PanelWidth, 520, true)
		r.a.apply(func(n *settings.Settings) {
			n.Panel.Cards = []string{ref(demo.ChargerID, "main"), ref(demo.PowerBankID, "main"), ref(demo.ChargerID, "ports"), ref(demo.ChargerID, "cover")}
		})
		r.tt.Frame()
		r.want(t, locales.TrayOpen(), locales.ModelShortA2687(), locales.ModelShortA110G(), locales.OverviewPorts())
		if len(r.sh.fits) == 0 {
			t.Fatal("the panel never asked to fit its content")
		}
		h := r.sh.fits[len(r.sh.fits)-1]
		if h < 200 || h > 760 {
			t.Errorf("panel fits %v tall", h)
		}
		r.tt.SetSize(PanelWidth, int(h+0.999))
		r.tt.Frame()
		r.save(t, "panel-"+theme)
		// Every card the settings list, and only those.
		r.a.Store.Settings.Panel.Cards = []string{ref(demo.ChargerID, "cover")}
		r.tt.Frame()
		if r.tt.HasText(locales.OverviewPorts()) {
			t.Error("a card the settings dropped still shows")
		}
	}
	// Nil shows each device's main card.
	r := newRig(t, "dark", PanelWidth, 520, true)
	r.want(t, locales.ModelShortA2687(), locales.ModelShortA110G())
}

func TestNarrowWindowClosesTheSidebar(t *testing.T) {
	r := newRig(t, "dark", 820, 560, false)
	r.tt.Advance(sidebarSlide)
	r.tt.Frame()
	if r.a.SidebarShown() {
		t.Fatal("an 820 window keeps the sidebar")
	}
	r.save(t, "narrow-overview")
	if err := r.tt.Click(locales.SidebarShow()); err != nil {
		t.Fatal(err)
	}
	r.tt.Advance(sidebarSlide)
	r.tt.Frame()
	if !r.a.SidebarShown() {
		t.Error("the sidebar button does not open it in a narrow window")
	}
}

func TestEveryPhaseHasAState(t *testing.T) {
	phases := map[link.Phase]string{
		link.PhaseOff:          locales.StateOff(),
		link.PhaseBluetoothOff: locales.StateBluetoothOff(),
		link.PhaseUnauthorized: locales.StateUnauthorized(),
		link.PhaseUnpaired:     locales.StateUnpaired(),
		link.PhaseConnecting:   locales.StateConnecting() + "…",
		link.PhaseHandshaking:  locales.StateHandshaking() + "…",
		link.PhaseDisconnected: locales.StateDisconnected(),
		link.PhaseSleeping:     locales.StateSleeping(),
	}
	for ph, word := range phases {
		r := newRig(t, "dark", 1280, 800, false)
		r.be.Hold(demo.ChargerID, true)
		r.a.GoDevice(demo.ChargerID, TabLive)
		r.a.Store.Device(demo.ChargerID).Status = link.Status{Phase: ph, Name: demo.ChargerName, Err: link.ErrStalled}
		r.tt.Frame()
		r.tt.Advance(statusDelay)
		r.tt.Frame()
		r.want(t, word)
	}
	// A device turned off says so, with the way to turn it on.
	r := newRig(t, "dark", 1280, 800, false)
	r.a.setEnabled(demo.ChargerID, false)
	r.flush()
	r.a.GoDevice(demo.ChargerID, TabLive)
	r.tt.Frame()
	r.want(t, locales.DeadOffTitle(locales.ModelShortA2687()), locales.DeadTurnOn())
	// Stale numbers stay, marked.
	r = newRig(t, "dark", 1280, 800, false)
	r.be.Hold(demo.PowerBankID, true)
	r.a.GoDevice(demo.PowerBankID, TabLive)
	r.tt.Advance(link.StaleAfter + time.Second)
	r.tt.Frame()
	r.want(t, locales.StateStale(), locales.PortDock())
}

func TestTransientPhaseWaits(t *testing.T) {
	r := newRig(t, "dark", 1280, 800, false)
	r.a.GoDevice(demo.ChargerID, TabLive)
	r.tt.Frame()
	r.a.Store.Device(demo.ChargerID).Status.Phase = link.PhaseConnecting
	r.tt.Frame()
	if r.tt.HasText(locales.StateConnecting()) {
		t.Fatal("a connection that may finish at once shows its waiting state")
	}
	r.tt.Advance(statusDelay)
	r.tt.Frame()
	r.want(t, locales.StateConnecting())
}

func TestTrayTitleKeepsItsWidth(t *testing.T) {
	r := newRig(t, "dark", 1280, 800, false)
	s := r.a.Store
	refs := []string{ref(demo.PowerBankID, "battery"), ref(demo.ChargerID, "total"), ref(demo.PowerBankID, "flow"),
		ref(demo.PowerBankID, "temp"), ref(demo.PowerBankID, "time_to_full"), ref(demo.ChargerID, "c3"), ref(demo.PowerBankID, "input")}
	wide := TrayTitle(s, refs, r.tt.Now())
	pct := 7.0
	s.Device(demo.PowerBankID).PowerBank.BatteryPercent = &pct
	low := TrayTitle(s, refs, r.tt.Now())
	s.Device(demo.ChargerID).Status.Phase = link.PhaseDisconnected
	s.Device(demo.PowerBankID).Status.Phase = link.PhaseDisconnected
	off := TrayTitle(s, refs, r.tt.Now())
	n := utf8.RuneCountInString(wide)
	if utf8.RuneCountInString(low) != n || utf8.RuneCountInString(off) != n {
		t.Errorf("title widths differ:\n%q\n%q\n%q", wide, low, off)
	}
	if !strings.Contains(off, figDash) {
		t.Errorf("an offline title %q shows numbers", off)
	}
	// Nil fields show each device's headline; a removed device shows none.
	if got := EffectiveFields(&s.Settings); !slices.Equal(got, []string{ref(demo.ChargerID, "total"), ref(demo.PowerBankID, "battery")}) {
		t.Errorf("default fields %q", got)
	}
	if FieldText(s, ref("gone", "total"), r.tt.Now()) != "" {
		t.Error("a device not added has a value")
	}
}

func TestMenuBarFieldsAddAndReorder(t *testing.T) {
	r := newRig(t, "dark", 1280, 1400, false)
	r.a.GoSettings()
	r.tt.Frame()
	if err := r.tt.Click(locales.SetAddItem(locales.ModelShortA110G() + " · " + locales.FactTemperature())); err != nil {
		t.Fatal(err)
	}
	r.tt.Frame()
	got := r.a.Store.Settings.MenuBar.Fields
	want := []string{ref(demo.ChargerID, "total"), ref(demo.PowerBankID, "battery"), ref(demo.PowerBankID, "temp")}
	if !slices.Equal(got, want) {
		t.Fatalf("fields %q", got)
	}
	if err := r.tt.Click(locales.SetLight()); err != nil {
		t.Fatal(err)
	}
	if r.a.Store.Settings.Theme != "light" {
		t.Errorf("theme %q", r.a.Store.Settings.Theme)
	}
	if err := r.tt.Click("日本語"); err != nil {
		t.Fatal(err)
	}
	if r.a.Store.Settings.Language != "ja" {
		t.Errorf("language %q", r.a.Store.Settings.Language)
	}
	r.save(t, "settings-tall")
}

func TestErrorsAndNoticesAreWorded(t *testing.T) {
	cases := map[string]string{
		ErrText(link.ErrStalled):                                                            locales.ErrStalled(),
		ErrText(&link.HandshakeError{Err: link.ErrNoResponse}):                              locales.ErrHandshake(locales.ErrNoResponse()),
		ErrText(&link.CoverRejectedError{Command: 0x21F, Result: 1}):                        locales.ErrCoverRejected("0x021F", "0x01"),
		ErrText(errors.New("odd")):                                                          "odd",
		NoticeText(&store.Notice{Kind: store.NoticeCoverFailed, Err: link.ErrNotConnected}): locales.ReasonJoin(locales.NoticeCoverFailed(), locales.ErrNotConnected()),
		NoticeText(&store.Notice{Kind: store.NoticeCoverShown, Subject: "A"}):               locales.NoticeCoverShown("A"),
	}
	for got, want := range cases {
		if got != want {
			t.Errorf("%q, want %q", got, want)
		}
	}
	if locales.In("zh-Hans").NavAddDevice() != "添加设备" || locales.In("ja").NavSettings() != "設定" {
		t.Error("the translations are not linked")
	}
}

// Every language has every message of the source.
func TestCatalogsAreComplete(t *testing.T) {
	read := func(name string) map[string]any {
		data, err := os.ReadFile(filepath.Join("..", "..", "locales", name+".json"))
		if err != nil {
			t.Fatal(err)
		}
		m := map[string]any{}
		if err := json.Unmarshal(data, &m); err != nil {
			t.Fatal(err)
		}
		return m
	}
	en := read("en")
	for _, lang := range []string{"zh-Hans", "ja"} {
		tr := read(lang)
		for key := range en {
			if strings.HasPrefix(key, "@") {
				continue
			}
			if _, ok := tr[key]; !ok {
				t.Errorf("%s lacks %s", lang, key)
			}
		}
	}
}
