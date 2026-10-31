package ble

import (
	"bytes"
	"fmt"
	"slices"
	"strings"
)

// parseAddress parses a Bluetooth device address, "AA:BB:CC:DD:EE:FF" in
// either case, most significant byte first. On Windows and Linux it is the
// peripheral's ID.
func parseAddress(s string) ([6]byte, error) {
	var a [6]byte
	if len(s) != 17 {
		return a, fmt.Errorf("ble: bad address %q", s)
	}
	for i := range a {
		if i > 0 && s[3*i-1] != ':' {
			return a, fmt.Errorf("ble: bad address %q", s)
		}
		hi, ok1 := hexNibble(s[3*i])
		lo, ok2 := hexNibble(s[3*i+1])
		if !ok1 || !ok2 {
			return a, fmt.Errorf("ble: bad address %q", s)
		}
		a[i] = hi<<4 | lo
	}
	return a, nil
}

func hexNibble(c byte) (byte, bool) {
	switch {
	case c >= '0' && c <= '9':
		return c - '0', true
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10, true
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10, true
	}
	return 0, false
}

// formatAddress is the ID form of an address: uppercase, colon separated.
func formatAddress(a [6]byte) string {
	return fmt.Sprintf("%02X:%02X:%02X:%02X:%02X:%02X", a[0], a[1], a[2], a[3], a[4], a[5])
}

// newAdvert builds an Advert from a stack's fields: UUIDs normalized, byte
// slices copied so the stack may reuse its buffers, maps nil when empty.
func newAdvert(id, name string, rssi int, services []string, mfr map[uint16][]byte, svcData map[string][]byte) Advert {
	a := Advert{ID: id, Name: name, RSSI: rssi, Connectable: true}
	for _, u := range services {
		if n := NormalizeUUID(u); !slices.Contains(a.ServiceUUIDs, n) {
			a.ServiceUUIDs = append(a.ServiceUUIDs, n)
		}
	}
	for company, data := range mfr {
		if a.ManufacturerData == nil {
			a.ManufacturerData = map[uint16][]byte{}
		}
		a.ManufacturerData[company] = bytes.Clone(data)
	}
	for u, data := range svcData {
		if a.ServiceData == nil {
			a.ServiceData = map[string][]byte{}
		}
		a.ServiceData[NormalizeUUID(u)] = bytes.Clone(data)
	}
	return a
}

// offersAny reports whether a advertises one of uuids, which are
// normalized; any advert does when uuids is empty.
func offersAny(a Advert, uuids []string) bool {
	if len(uuids) == 0 {
		return true
	}
	for _, u := range a.ServiceUUIDs {
		if slices.Contains(uuids, u) {
			return true
		}
	}
	return false
}

// matchUUIDs keeps the items whose UUID is among want, all of them when
// want is empty.
func matchUUIDs[T any](items []T, uuid func(T) string, want []string) []T {
	if len(want) == 0 {
		return items
	}
	var out []T
	for _, it := range items {
		for _, w := range want {
			if strings.EqualFold(uuid(it), NormalizeUUID(w)) {
				out = append(out, it)
				break
			}
		}
	}
	return out
}
