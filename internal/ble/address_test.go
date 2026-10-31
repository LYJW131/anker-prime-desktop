package ble

import (
	"reflect"
	"testing"
)

func TestAddressRoundTrip(t *testing.T) {
	for in, want := range map[string]string{
		"aa:bb:cc:01:02:03": "AA:BB:CC:01:02:03",
		"F4:9C:2A:00:9F:E1": "F4:9C:2A:00:9F:E1",
	} {
		a, err := parseAddress(in)
		if err != nil {
			t.Fatalf("parseAddress(%q): %v", in, err)
		}
		if got := formatAddress(a); got != want {
			t.Errorf("formatAddress(parseAddress(%q)) = %q, want %q", in, got, want)
		}
	}
	if a, _ := parseAddress("01:02:03:04:05:06"); a != [6]byte{1, 2, 3, 4, 5, 6} {
		t.Errorf("byte order: %x", a)
	}
	for _, bad := range []string{
		"", "AA:BB:CC:DD:EE", "AA:BB:CC:DD:EE:FG", "AA-BB-CC-DD-EE-FF",
		"5C0F6D4E-8A3B-4B8F-9C1D-2E3F4A5B6C7D", // a CoreBluetooth identifier
	} {
		if _, err := parseAddress(bad); err == nil {
			t.Errorf("parseAddress(%q) accepted", bad)
		}
	}
}

func TestNewAdvert(t *testing.T) {
	mfr := []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x26}
	svc := []byte{0xAA}
	a := newAdvert("AA:BB:CC:DD:EE:FF", "ASHDJW7CF49200487", -60,
		[]string{"FF09", "0000ff09-0000-1000-8000-00805f9b34fb", "8C850001-0302-41C5-B46E-CF057C562025"},
		map[uint16][]byte{0xFFFF: mfr}, map[string][]byte{"180F": svc})
	want := Advert{
		ID: "AA:BB:CC:DD:EE:FF", Name: "ASHDJW7CF49200487", RSSI: -60, Connectable: true,
		ServiceUUIDs:     []string{"0000ff09-0000-1000-8000-00805f9b34fb", "8c850001-0302-41c5-b46e-cf057c562025"},
		ManufacturerData: map[uint16][]byte{0xFFFF: mfr},
		ServiceData:      map[string][]byte{"0000180f-0000-1000-8000-00805f9b34fb": svc},
	}
	if !reflect.DeepEqual(a, want) {
		t.Fatalf("got %+v\nwant %+v", a, want)
	}
	mfr[9] = 0 // the stack reuses its buffer
	if a.ManufacturerData[0xFFFF][9] != 0x26 {
		t.Fatal("manufacturer data shares the stack's buffer")
	}
	if e := newAdvert("X", "", 127, nil, nil, nil); e.ManufacturerData != nil || e.ServiceData != nil || e.ServiceUUIDs != nil {
		t.Fatalf("empty advert: %+v", e)
	}
}

func TestOffersAny(t *testing.T) {
	a := newAdvert("X", "", 0, []string{"ff09"}, nil, nil)
	if !offersAny(a, nil) || !offersAny(a, []string{NormalizeUUID("FF09")}) || offersAny(a, []string{NormalizeUUID("180F")}) {
		t.Fatal("offersAny")
	}
}

func TestMatchUUIDs(t *testing.T) {
	chars := []Characteristic{{UUID: "8c850002-0302-41c5-b46e-cf057c562025"}, {UUID: "8c850003-0302-41c5-b46e-cf057c562025"}}
	got := matchUUIDs(chars, func(c Characteristic) string { return c.UUID }, []string{"8C850003-0302-41C5-B46E-CF057C562025"})
	if len(got) != 1 || got[0].UUID != chars[1].UUID {
		t.Fatalf("got %+v", got)
	}
	if got := matchUUIDs(chars, func(c Characteristic) string { return c.UUID }, nil); len(got) != 2 {
		t.Fatalf("no filter: %+v", got)
	}
}
