package device

import (
	"encoding/binary"
	"math"
	"slices"
	"strconv"

	"github.com/LYJW131/anker-prime-desktop/internal/ff09"
)

// u16 reads a little-endian u16 at offset, if the data is long enough.
func u16(data []byte, offset int) (uint16, bool) {
	if len(data) < offset+2 {
		return 0, false
	}
	return binary.LittleEndian.Uint16(data[offset:]), true
}

// scaled reads a u16 at offset divided by div; nil when short.
func scaled(data []byte, offset int, div float64) *float64 {
	v, ok := u16(data, offset)
	if !ok {
		return nil
	}
	return ptr(float64(v) / div)
}

// tenths is the power bank's encoding of volts, amps and watts.
func tenths(data []byte, offset int) *float64 { return scaled(data, offset, 10) }

// decimalPair reads [integer, remainder out of 100], the power bank's battery
// encoding. It is not a u16. A remainder above 99 reads as nothing.
func decimalPair(data []byte) *float64 {
	if len(data) < 2 || data[1] > 99 {
		return nil
	}
	return ptr(float64(data[0]) + float64(data[1])/100.0)
}

// round2 rounds half-to-even on the exact binary value, as Python's round(x, 2).
func round2(x float64) float64 {
	r, err := strconv.ParseFloat(strconv.FormatFloat(x, 'f', 2, 64), 64)
	if err != nil || math.IsNaN(r) {
		return x
	}
	return r
}

func intPtr(v uint32) *int { return ptr(int(v)) }

// tlvMap keys the TLVs of a payload by type; a repeated type keeps the last.
func tlvMap(tlvs []ff09.TLV) map[byte]ff09.TypedValue {
	m := make(map[byte]ff09.TypedValue, len(tlvs))
	for _, t := range tlvs {
		m[t.Type] = ff09.ReadTypedValue(t.Value)
	}
	return m
}

func contains(set []uint16, v uint16) bool { return slices.Contains(set, v) }

// applyIdentity merges a 0x0029 reply into id.
func applyIdentity(id *Identity, plain []byte, charger bool) {
	cur := ff09.Identity{
		Serial:   deref(id.Serial),
		Firmware: deref(id.Firmware),
		MAC:      deref(id.MACAddress),
		Label:    deref(id.Label),
	}
	cur.Apply0029(plain, charger)
	id.Serial, id.Firmware = strPtr(cur.Serial), strPtr(cur.Firmware)
	id.MACAddress, id.Label = strPtr(cur.MAC), strPtr(cur.Label)
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
