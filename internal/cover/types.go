// Package cover keeps the charger's custom covers: a local library of
// 240×240 JPEGs, each under an id this app picks, and the CRC-32 the
// charger knows it by. No cloud is needed: the charger takes any id and
// stores the pixels pushed over BLE (verified on the device, see
// docs/screensaver.md).
package cover

import "time"

// Item is one picture in the library.
type Item struct {
	ID      int       `json:"id"`   // sent as u32; the charger reports back only the low 16 bits
	Hash    uint32    `json:"hash"` // IEEE CRC-32 of the JPEG bytes
	Name    string    `json:"name"`
	File    string    `json:"file"` // JPEG file name inside the library directory
	Size    int       `json:"size"`
	AddedAt time.Time `json:"added_at"`
}
