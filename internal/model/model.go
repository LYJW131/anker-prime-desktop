// Package model is the registry of supported device models. Supporting a
// new Anker device means adding one Model here, its decoder in
// internal/device and its views in internal/ui; docs/adding-a-device.md
// walks through the whole path.
package model

import (
	"bytes"
	"strings"

	"github.com/LYJW131/anker-prime-desktop/internal/device"
)

// ID is a model number, as Anker prints it on the device.
type ID string

const (
	A2687 ID = "A2687" // Anker Prime Charger (160W, 3 ports)
	A110G ID = "A110G" // Anker Prime Power Bank (20K, 220W)
)

// Family groups models that share a page layout and an icon.
type Family string

const (
	FamilyCharger   Family = "charger"
	FamilyPowerBank Family = "powerbank"
)

// Model is what the app knows about one model.
type Model struct {
	ID     ID
	Family Family
	Kind   device.Kind // which decoder in internal/device

	// Recognising it while adding a device: the model byte of the
	// manufacturer data every Prime device advertises under company
	// 0xFFFF (02, MAC(6), 02 B4, model), then the name as a fallback.
	ModelByte  byte
	NamePrefix string // "" when the name is a bare serial number

	// Session differences on the shared FF09 transport.
	Realtime bool // send the 0x020A realtime request after the status probe

	// Capabilities the views and the backend offer.
	Covers    bool    // custom screen covers over 0x021F/0x0220/0x0221
	IdleSleep bool    // can be released while idle so it sleeps
	RatedW    float64 // headline power scale, 0 when not a charger
	Ports     []string
}

// Models are the supported models, in the order the add-device list shows
// them.
var Models = []Model{
	{
		ID: A2687, Family: FamilyCharger, Kind: device.KindCharger,
		ModelByte: 0x05, NamePrefix: "ASHDJW",
		Realtime: true, Covers: true, RatedW: 160,
		Ports: []string{"C1", "C2", "C3"},
	},
	{
		ID: A110G, Family: FamilyPowerBank, Kind: device.KindPowerBank,
		ModelByte: 0x0A,
		IdleSleep: true,
		Ports:     []string{"C1", "C2", "A", "DOCK"},
	},
}

// ByID returns the model with id.
func ByID(id ID) (Model, bool) {
	for _, m := range Models {
		if m.ID == id {
			return m, true
		}
	}
	return Model{}, false
}

// AdvertModelByte returns the model byte of an Anker Prime advert's
// manufacturer data, if it has the Prime layout.
func AdvertModelByte(mfr map[uint16][]byte) (byte, bool) {
	md, ok := mfr[0xFFFF]
	if !ok || len(md) < 10 || md[0] != 0x02 || !bytes.Equal(md[7:9], []byte{0x02, 0xB4}) {
		return 0, false
	}
	return md[9], true
}

// Match tells which supported model an advert of the FF09 service comes
// from. A Prime advert whose model byte is unknown returns ok false with
// prime true: a device this app could support but does not yet.
func Match(mfr map[uint16][]byte, name string) (m Model, prime, ok bool) {
	if b, has := AdvertModelByte(mfr); has {
		for _, m := range Models {
			if m.ModelByte == b {
				return m, true, true
			}
		}
		return Model{}, true, false
	}
	for _, m := range Models {
		if m.NamePrefix != "" && strings.HasPrefix(name, m.NamePrefix) {
			return m, true, true
		}
	}
	return Model{}, false, false
}
