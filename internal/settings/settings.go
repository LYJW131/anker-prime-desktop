// Package settings is the app's saved configuration, one JSON file in the
// user data directory. The Anker account ID is not here: it lives in the
// Keychain (mygo.Secrets).
package settings

// Settings is everything the user can change.
type Settings struct {
	Theme     string         `json:"theme"` // "system", "light" or "dark"
	ShowDock  bool           `json:"show_dock"`
	Charger   Device         `json:"charger"`
	PowerBank Device         `json:"powerbank"`
	MenuBar   MenuBar        `json:"menu_bar"`
	Panel     Panel          `json:"panel"`
}

// Device is one device's connection settings.
type Device struct {
	Enabled    bool   `json:"enabled"`
	Peripheral string `json:"peripheral"` // CoreBluetooth identifier; "" until paired
	Name       string `json:"name"`       // advertised name when paired
	IdleSleep  bool   `json:"idle_sleep"` // power bank only: release it while idle so it can sleep
}

// MenuBar is what the menu bar item shows.
type MenuBar struct {
	Fields   []string `json:"fields"`    // field ids, in order, joined into the title
	ShowIcon bool     `json:"show_icon"` // the template icon before the title
}

// Panel is what the menu bar panel shows.
type Panel struct {
	Cards []string `json:"cards"` // card ids, in order; a card not listed is hidden
}

// Default is the configuration of a first launch.
func Default() Settings {
	return Settings{
		Theme:     "system",
		Charger:   Device{Enabled: true},
		PowerBank: Device{Enabled: true, IdleSleep: true},
		MenuBar:   MenuBar{Fields: []string{"powerbank.battery", "charger.total"}, ShowIcon: true},
		Panel:     Panel{Cards: []string{"charger", "powerbank", "charger.ports", "powerbank.ports"}},
	}
}
