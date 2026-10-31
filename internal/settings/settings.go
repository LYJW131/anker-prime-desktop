// Package settings is the app's saved configuration, one JSON file in the
// user data directory. The Anker account ID is not here: it lives in the
// Keychain (mygo.Secrets).
package settings

// Settings is everything the user can change.
type Settings struct {
	Theme    string   `json:"theme"`    // "system", "light" or "dark"
	Language string   `json:"language"` // "" follows the system; else "en", "zh-Hans" or "ja"
	ShowDock bool     `json:"show_dock"`
	Devices  []Device `json:"devices"` // in the order the sidebar lists them
	MenuBar  MenuBar  `json:"menu_bar"`
	Panel    Panel    `json:"panel"`
}

// Device is one added device.
type Device struct {
	ID        string `json:"id"`         // the CoreBluetooth identifier, stable on this Mac
	Model     string `json:"model"`      // a model.ID
	Name      string `json:"name"`       // advertised name, which is the serial number
	Nickname  string `json:"nickname"`   // the user's name for it; "" shows the model's name
	Enabled   bool   `json:"enabled"`    // keep it connected
	IdleSleep bool   `json:"idle_sleep"` // release it while idle so it can sleep, where the model allows
}

// MenuBar is what the menu bar item shows.
type MenuBar struct {
	// Fields are "<device id>/<field id>", in order, joined into the
	// title. Nil shows each device's headline field.
	Fields   []string `json:"fields"`
	ShowIcon bool     `json:"show_icon"` // the template icon before the title
}

// Panel is what the menu bar panel shows.
type Panel struct {
	// Cards are "<device id>/<card id>", in order. Nil shows each
	// device's main card.
	Cards []string `json:"cards"`
}

// Default is the configuration of a first launch: no devices yet.
func Default() Settings {
	return Settings{Theme: "system", MenuBar: MenuBar{ShowIcon: true}}
}

// Device returns the added device with id.
func (s *Settings) Device(id string) (*Device, bool) {
	for i := range s.Devices {
		if s.Devices[i].ID == id {
			return &s.Devices[i], true
		}
	}
	return nil, false
}
