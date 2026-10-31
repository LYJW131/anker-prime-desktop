//go:build linux

package ble

import (
	"testing"

	"github.com/godbus/dbus/v5"
)

func TestAdvertFromBlueZ(t *testing.T) {
	mfr := []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 0x26}
	a := advertFromBlueZ(map[string]dbus.Variant{
		"Address":          dbus.MakeVariant("f4:9c:2a:00:9f:e1"),
		"Name":             dbus.MakeVariant("ASHDJW7CF49200487"),
		"RSSI":             dbus.MakeVariant(int16(-61)),
		"UUIDs":            dbus.MakeVariant([]string{"0000ff09-0000-1000-8000-00805f9b34fb"}),
		"ManufacturerData": dbus.MakeVariant(map[uint16]dbus.Variant{0xFFFF: dbus.MakeVariant(mfr)}),
	})
	if a.ID != "F4:9C:2A:00:9F:E1" || a.Name != "ASHDJW7CF49200487" || a.RSSI != -61 {
		t.Fatalf("got %+v", a)
	}
	if !offersAny(a, []string{NormalizeUUID("FF09")}) || a.ManufacturerData[0xFFFF][9] != 0x26 {
		t.Fatalf("got %+v", a)
	}
	// A device that advertises neither UUIDs nor a name.
	b := advertFromBlueZ(map[string]dbus.Variant{"Address": dbus.MakeVariant("AA:BB:CC:DD:EE:FF")})
	if b.ID != "AA:BB:CC:DD:EE:FF" || b.RSSI != 127 || b.ServiceUUIDs != nil || b.ManufacturerData != nil {
		t.Fatalf("got %+v", b)
	}
}
