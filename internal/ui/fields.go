package ui

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/LYJW131/anker-prime-desktop/internal/device"
	"github.com/LYJW131/anker-prime-desktop/internal/link"
	"github.com/LYJW131/anker-prime-desktop/internal/model"
	"github.com/LYJW131/anker-prime-desktop/internal/settings"
	"github.com/LYJW131/anker-prime-desktop/internal/store"
	"github.com/LYJW131/anker-prime-desktop/locales"
)

// Field is one value of a device the menu bar's title can show.
// settings.MenuBar.Fields keeps it as "<device id>/<ID>".
type Field struct {
	ID    string
	Label string
}

// FieldsOf is the catalog of a model's values, in the order Settings
// offers them; the first is its headline.
func FieldsOf(m model.Model) []Field {
	switch m.Family {
	case model.FamilyPowerBank:
		return []Field{
			{"battery", locales.FieldBattery()},
			{"flow", locales.FieldFlow()},
			{"input", locales.FieldInput()},
			{"output", locales.FieldOutput()},
			{"temp", locales.FactTemperature()},
			{"time_to_full", locales.FieldTimeToFull()},
		}
	default:
		out := []Field{{"total", locales.FieldTotal()}}
		for _, p := range m.Ports {
			out = append(out, Field{strings.ToLower(p), locales.FieldPort(p)})
		}
		return out
	}
}

// fieldOf finds a field of a model's catalog.
func fieldOf(m model.Model, id string) (Field, bool) {
	for _, f := range FieldsOf(m) {
		if f.ID == id {
			return f, true
		}
	}
	return Field{}, false
}

// Card is one card of a device the menu bar panel can show.
// settings.Panel.Cards keeps it as "<device id>/<ID>".
type Card struct {
	ID    string
	Label string
}

// CardsOf is the catalog of a model's panel cards; the first is its main
// card.
func CardsOf(m model.Model) []Card {
	out := []Card{{"main", locales.CardMain()}, {"ports", locales.OverviewPorts()}}
	if m.Covers {
		out = append(out, Card{"cover", locales.FactCover()})
	}
	return out
}

func cardOf(m model.Model, id string) (Card, bool) {
	for _, c := range CardsOf(m) {
		if c.ID == id {
			return c, true
		}
	}
	return Card{}, false
}

// ref joins a device id and a field or card id.
func ref(deviceID, id string) string { return deviceID + "/" + id }

// splitRef splits "<device id>/<id>".
func splitRef(r string) (deviceID, id string) {
	i := strings.LastIndexByte(r, '/')
	if i < 0 {
		return "", r
	}
	return r[:i], r[i+1:]
}

// modelOf is the model of an added device.
func modelOf(s *settings.Settings, id string) (model.Model, bool) {
	d, ok := s.Device(id)
	if !ok {
		return model.Model{}, false
	}
	return model.ByID(model.ID(d.Model))
}

// EffectiveFields are the values the menu bar shows: the settings' list,
// or each device's headline while it is nil.
func EffectiveFields(s *settings.Settings) []string {
	if s.MenuBar.Fields != nil {
		return s.MenuBar.Fields
	}
	var out []string
	for _, d := range s.Devices {
		if m, ok := model.ByID(model.ID(d.Model)); ok {
			out = append(out, ref(d.ID, FieldsOf(m)[0].ID))
		}
	}
	return out
}

// EffectiveCards are the cards the panel shows: the settings' list, or
// each device's main card while it is nil.
func EffectiveCards(s *settings.Settings) []string {
	if s.Panel.Cards != nil {
		return s.Panel.Cards
	}
	var out []string
	for _, d := range s.Devices {
		if _, ok := model.ByID(model.ID(d.Model)); ok {
			out = append(out, ref(d.ID, "main"))
		}
	}
	return out
}

// Figure space and figure dash: as wide as a digit in the menu bar's
// font, whose digits are of one width, so a value keeps its width as its
// digits come and go.
const (
	figSpace = " "
	figDash  = "‒"
)

// pad right-aligns s in n digit widths.
func pad(s string, n int) string {
	if k := n - len([]rune(s)); k > 0 {
		return strings.Repeat(figSpace, k) + s
	}
	return s
}

// TrayTitle is the menu bar item's title: the values refs name, in order,
// each a fixed width, so the item keeps its width as the numbers change.
func TrayTitle(s *store.Store, refs []string, now time.Time) string {
	parts := make([]string, 0, len(refs))
	for _, r := range refs {
		if t := FieldText(s, r, now); t != "" {
			parts = append(parts, t)
		}
	}
	return strings.Join(parts, "  ")
}

// live reports whether device id's numbers are fresh.
func live(s *store.Store, id string, now time.Time) bool {
	sd, ok := s.Settings.Device(id)
	d := s.Devices[id]
	if !ok || !sd.Enabled || d == nil {
		return false
	}
	st := d.Status
	return st.Phase == link.PhaseConnected && !st.LastFrame.IsZero() && !st.Stale(now)
}

// FieldText is one value's text in the title, "" for a device or a field
// that is not there.
func FieldText(s *store.Store, r string, now time.Time) string {
	id, fid := splitRef(r)
	m, ok := modelOf(&s.Settings, id)
	if !ok {
		return ""
	}
	if _, ok := fieldOf(m, fid); !ok {
		return ""
	}
	on := live(s, id, now)
	w := func(v float64, ok bool) string {
		if !on || !ok {
			return pad(figDash, 3) + "W"
		}
		return pad(fmt.Sprint(int(math.Round(math.Abs(v)))), 3) + "W"
	}
	ch, pb := &device.Charger{}, &device.PowerBank{}
	if d := s.Devices[id]; d != nil {
		if d.Charger != nil {
			ch = d.Charger
		}
		if d.PowerBank != nil {
			pb = d.PowerBank
		}
	}
	switch m.Family {
	case model.FamilyPowerBank:
		switch fid {
		case "battery":
			if !on || pb.BatteryPercent == nil {
				return pad(figDash, 3) + "%"
			}
			return pad(fmt.Sprint(int(math.Round(*pb.BatteryPercent))), 3) + "%"
		case "flow":
			if !on {
				return figSpace + w(0, false)
			}
			net := flow(pb)
			sign := figSpace
			switch {
			case math.Round(net) > 0:
				sign = "+"
			case math.Round(net) < 0:
				sign = "−"
			}
			return pad(sign+fmt.Sprint(int(math.Round(math.Abs(net)))), 4) + "W"
		case "input":
			return locales.TrayIn(w(deref(pb.InputPowerW), pb.InputPowerW != nil))
		case "output":
			return locales.TrayOut(w(deref(pb.OutputPowerW), pb.OutputPowerW != nil))
		case "temp":
			t := hottest(pb)
			if !on || t == nil {
				return pad(figDash, 2) + "°"
			}
			return pad(fmt.Sprint(*t), 2) + "°"
		case "time_to_full":
			if !on || !pb.Charging || pb.TimeLeftHours == nil || pb.TimeLeftMinutes == nil ||
				*pb.TimeLeftHours+*pb.TimeLeftMinutes == 0 {
				return figDash + ":" + figDash + figDash
			}
			return fmt.Sprintf("%d:%02d", *pb.TimeLeftHours, *pb.TimeLeftMinutes)
		}
	default:
		if fid == "total" {
			return w(deref(ch.TotalOutputPowerW), ch.TotalOutputPowerW != nil)
		}
		name := strings.ToUpper(fid)
		p := portByName(ch.Ports, name)
		v := 0.0
		if p != nil && p.Active {
			v = p.PowerW
		}
		return name + " " + w(v, true)
	}
	return ""
}

// sampleText is a field's text as Settings lists it, without the padding.
func sampleText(s *store.Store, r string, now time.Time) string {
	return strings.TrimLeft(FieldText(s, r, now), figSpace)
}

func deref(v *float64) float64 {
	if v == nil {
		return 0
	}
	return *v
}

// flow is the power bank's net power: positive in, negative out.
func flow(pb *device.PowerBank) float64 {
	return deref(pb.InputPowerW) - deref(pb.OutputPowerW)
}

// hottest is the higher of the power bank's two temperatures.
func hottest(pb *device.PowerBank) *int {
	t1, t2 := pb.Temperature1C, pb.Temperature2C
	switch {
	case t1 == nil:
		return t2
	case t2 == nil || *t1 >= *t2:
		return t1
	}
	return t2
}

func portByName(ports []device.Port, name string) *device.Port {
	for i := range ports {
		if strings.EqualFold(ports[i].Name, name) {
			return &ports[i]
		}
	}
	return nil
}

// trayBank is the power bank the tray icon fills to its battery: the
// first live one of the added devices.
func trayBank(s *store.Store, now time.Time) *device.PowerBank {
	for _, sd := range s.Settings.Devices {
		m, ok := model.ByID(model.ID(sd.Model))
		if !ok || m.Family != model.FamilyPowerBank || !live(s, sd.ID, now) {
			continue
		}
		if d := s.Devices[sd.ID]; d != nil && d.PowerBank != nil && d.PowerBank.BatteryPercent != nil {
			return d.PowerBank
		}
	}
	return nil
}

// TrayLevel is a power bank's battery as the tray icon fills it, 0…1, or
// -1 while no power bank is live.
func TrayLevel(s *store.Store, now time.Time) float64 {
	pb := trayBank(s, now)
	if pb == nil {
		return -1
	}
	return min(max(*pb.BatteryPercent/100, 0), 1)
}

// TrayCharging reports whether that power bank charges, which the tray
// icon marks with a bolt.
func TrayCharging(s *store.Store, now time.Time) bool {
	pb := trayBank(s, now)
	return pb != nil && pb.Charging
}

// ToolTip is the menu bar item's tooltip: each device's headline.
func ToolTip(s *store.Store, now time.Time) string {
	lines := []string{locales.AppName()}
	for _, sd := range s.Settings.Devices {
		m, ok := model.ByID(model.ID(sd.Model))
		if !ok {
			continue
		}
		d := dev{set: sd, m: m}
		f := FieldsOf(m)[0]
		v := strings.Trim(FieldText(s, ref(sd.ID, f.ID), now), figSpace)
		lines = append(lines, d.shortName()+" · "+f.Label+" "+v)
	}
	return strings.Join(lines, "\n")
}
