package ui

import (
	"fmt"
	"math"

	"github.com/egoist/mygo/ui"

	"github.com/LYJW131/anker-prime-mac/internal/device"
	"github.com/LYJW131/anker-prime-mac/internal/model"
	"github.com/LYJW131/anker-prime-mac/internal/settings"
	"github.com/LYJW131/anker-prime-mac/internal/store"
	"github.com/LYJW131/anker-prime-mac/locales"
)

// dev is an added device as the views use it: its settings, its model and
// its live state.
type dev struct {
	set settings.Device
	m   model.Model
	st  *store.Device
}

// ID is the device's id, its CoreBluetooth identifier.
func (d dev) ID() string { return d.set.ID }

// devices are the added devices of a supported model, in the sidebar's
// order.
func (a *App) devices() []dev {
	out := make([]dev, 0, len(a.Store.Settings.Devices))
	for _, sd := range a.Store.Settings.Devices {
		m, ok := model.ByID(model.ID(sd.Model))
		if !ok {
			continue
		}
		out = append(out, dev{set: sd, m: m, st: a.Store.Device(sd.ID)})
	}
	return out
}

// device is the added device id.
func (a *App) device(id string) (dev, bool) {
	sd, ok := a.Store.Settings.Device(id)
	if !ok {
		return dev{}, false
	}
	m, ok := model.ByID(model.ID(sd.Model))
	if !ok {
		return dev{}, false
	}
	return dev{set: *sd, m: m, st: a.Store.Device(id)}, true
}

// name is what the device is called: its nickname, else its model's name.
func (d dev) name() string {
	if d.set.Nickname != "" {
		return d.set.Nickname
	}
	return ModelName(d.m.ID)
}

// shortName is the name where room is short, as the sidebar and the
// panel: the nickname, else the model's short name.
func (d dev) shortName() string {
	if d.set.Nickname != "" {
		return d.set.Nickname
	}
	return ModelShort(d.m.ID)
}

// charger is a charger's numbers, empty before the first frame, so the
// views read one shape.
func (d dev) charger() *device.Charger {
	if d.st.Charger != nil {
		return d.st.Charger
	}
	return &device.Charger{}
}

// bank is a power bank's numbers, empty before the first frame.
func (d dev) bank() *device.PowerBank {
	if d.st.PowerBank != nil {
		return d.st.PowerBank
	}
	return &device.PowerBank{}
}

// familyLook is a family's icon and tile color.
func familyLook(k *Tokens, f model.Family) (*ui.SVG, ui.Color) {
	if f == model.FamilyPowerBank {
		return icPowerBank, k.TilePowerBank
	}
	return icCharger, k.TileCharger
}

// headline is the device's one number, as the sidebar shows it while
// live, with the color it takes (zero: the muted ink).
func (a *App) headline(k *Tokens, d dev, s devState) (string, ui.Color) {
	if !s.live {
		return "", ui.Color{}
	}
	switch d.m.Family {
	case model.FamilyPowerBank:
		pb := d.bank()
		if pb.BatteryPercent == nil {
			return "", ui.Color{}
		}
		col := ui.Color{}
		if pb.Charging {
			col = k.OK
		}
		return fmt.Sprintf("%.0f%%", math.Round(*pb.BatteryPercent)), col
	default:
		return fmt.Sprintf("%.0f W", deref(d.charger().TotalOutputPowerW)), ui.Color{}
	}
}

// ports are a device's ports by its model, as decoded, or idle
// placeholders before the first frame: the cards keep their places.
func (d dev) ports() []device.Port {
	out := make([]device.Port, len(d.m.Ports))
	for i, name := range d.m.Ports {
		out[i] = device.Port{Name: name}
	}
	switch d.m.Family {
	case model.FamilyPowerBank:
		pb := d.bank()
		for i := range out {
			if out[i].Name == "DOCK" {
				if pb.Dock != nil {
					out[i] = *pb.Dock
					out[i].Name = "DOCK"
				}
			} else if p := portByName(pb.Ports, out[i].Name); p != nil {
				out[i] = *p
			}
		}
	default:
		ch := d.charger()
		for i := range out {
			if p := portByName(ch.Ports, out[i].Name); p != nil {
				out[i] = *p
			}
		}
	}
	return out
}

// removeDevice drops a device and what the menu bar and the panel show of
// it. A list it leaves empty goes back to its defaults.
func removeDevice(s *settings.Settings, id string) {
	out := s.Devices[:0]
	for _, d := range s.Devices {
		if d.ID != id {
			out = append(out, d)
		}
	}
	s.Devices = out
	s.MenuBar.Fields = dropRefs(s.MenuBar.Fields, id)
	s.Panel.Cards = dropRefs(s.Panel.Cards, id)
}

func dropRefs(refs []string, id string) []string {
	if refs == nil {
		return nil
	}
	out := make([]string, 0, len(refs))
	for _, r := range refs {
		if rid, _ := splitRef(r); rid != id {
			out = append(out, r)
		}
	}
	if len(out) == 0 && len(refs) > 0 {
		return nil
	}
	return out
}

// ModelName is a model's name in the app's language.
func ModelName(id model.ID) string {
	switch id {
	case model.A2687:
		return locales.ModelA2687()
	case model.A110G:
		return locales.ModelA110G()
	}
	return string(id)
}

// ModelShort is a model's short name in the app's language.
func ModelShort(id model.ID) string {
	switch id {
	case model.A2687:
		return locales.ModelShortA2687()
	case model.A110G:
		return locales.ModelShortA110G()
	}
	return string(id)
}
