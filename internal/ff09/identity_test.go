package ff09

import (
	"strings"
	"testing"
)

func TestIdentityFromCapturedReplies(t *testing.T) {
	// The 0x0029 replies recorded from the charger and the power bank.
	for _, tc := range []struct {
		name, plain, serial, fw, mac string
	}{
		{"charger",
			"00A10103A2084368617267696E67A30876302E302E352E31A411415348444A573743463439323030343837A5117CE91377C3643743463439323030343837",
			"ASHDJW7CF49200487", "v0.0.5.1", "7C:E9:13:77:C3:64"},
	} {
		for _, charger := range []bool{true, false} {
			var id Identity
			id.Apply0029(unhex(t, tc.plain), charger)
			if id.Label != "Charging" || id.Serial != tc.serial || id.Firmware != tc.fw || id.MAC != tc.mac {
				t.Errorf("%s (charger rule %v): %+v", tc.name, charger, id)
			}
		}
	}
}

func TestIdentityChargerPrefixRule(t *testing.T) {
	// 03 00 + 4 more bytes, then the TLVs: only the charger walk skips them (the
	// others read FF FF as a TLV that overruns the packet and stop).
	body := append([]byte{0x03, 0x00, 0xFF, 0xFF, 0xFF, 0xFF}, unhex(t, "A30876302E302E352E31")...)
	var charger, bank Identity
	charger.Apply0029(body, true)
	bank.Apply0029(body, false)
	if charger.Firmware != "v0.0.5.1" {
		t.Errorf("charger: %+v", charger)
	}
	if bank.Firmware != "" {
		t.Errorf("power bank walked the prefix: %+v", bank)
	}
}

func TestIdentitySniffsByContent(t *testing.T) {
	// Firmware and serial in the wrong TLVs are still found; a mac that starts
	// with printable bytes is text, not an address; all zeros is no address.
	plain := unhex(t, "00"+"A5"+"06"+"414243444546"+"A2"+"0A"+"414A37444C43483046343936"[:20]+"A9"+"06"+"76312E322E33")
	var id Identity
	id.Apply0029(plain, false)
	if id.MAC != "" {
		t.Errorf("text taken for a MAC: %q", id.MAC)
	}
	eq(t, "firmware from a stray TLV", id.Firmware, "v1.2.3")
	eq(t, "serial from the label TLV", id.Serial, "AJ7DLCH0F4")
	eq(t, "label", id.Label, "AJ7DLCH0F4")

	var zero Identity
	zero.Apply0029(append([]byte{0x00, 0xA5, 0x06}, make([]byte, 6)...), false)
	eq(t, "zero MAC", zero.MAC, "")
}

func TestLooksLike(t *testing.T) {
	for s, want := range map[string]bool{
		"v0.0.5.1": true, "1.2": true, "V10.20.30": true, "..": true, "1": false, "v1.a": false, "": false,
	} {
		eq(t, "firmware "+s, looksLikeFirmware(s), want)
	}
	for s, want := range map[string]bool{
		"ASHDJW7CF49200487": true, strings.Repeat("a", 10): true, strings.Repeat("a", 9): false,
		strings.Repeat("a", 31): false, "ab cdefghijk": false, "1234567890.1": false, "ab_cd-efghij": true,
	} {
		eq(t, "serial "+s, looksLikeSerial(s), want)
	}
}
