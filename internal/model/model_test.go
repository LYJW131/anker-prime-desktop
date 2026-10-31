package model

import (
	"encoding/hex"
	"testing"

	"github.com/LYJW131/anker-prime-mac/internal/device"
)

func mfr(t *testing.T, h string) map[uint16][]byte {
	t.Helper()
	b, err := hex.DecodeString(h)
	if err != nil {
		t.Fatal(err)
	}
	return map[uint16][]byte{0xFFFF: b}
}

// The manufacturer data of the two devices on lyjw's desk, as scanned.
func TestMatchRealAdverts(t *testing.T) {
	for _, tc := range []struct {
		name, data string
		want       ID
	}{
		{"ASHDJW7CF49200487", "027ce91377c36402b405444a573704", A2687},
		{"AJ7DLCH0F49600286", "027ce91378cada02b40a3559550004", A110G},
	} {
		m, prime, ok := Match(mfr(t, tc.data), tc.name)
		if !prime || !ok || m.ID != tc.want {
			t.Errorf("%s: got %q prime=%v ok=%v, want %s", tc.name, m.ID, prime, ok, tc.want)
		}
	}
}

func TestMatchUnsupportedPrime(t *testing.T) {
	m, prime, ok := Match(mfr(t, "027ce91377c36402b40b444a573704"), "AXXXX")
	if !prime || ok || m.ID != "" {
		t.Errorf("model byte 0x0B: got %q prime=%v ok=%v, want an unsupported Prime device", m.ID, prime, ok)
	}
	if b, has := AdvertModelByte(mfr(t, "027ce91377c36402b40b444a573704")); !has || b != 0x0B {
		t.Errorf("AdvertModelByte = %#x, %v", b, has)
	}
}

func TestMatchNameFallbackAndStranger(t *testing.T) {
	if m, _, ok := Match(nil, "ASHDJW0000000000"); !ok || m.ID != A2687 {
		t.Errorf("charger by name: got %q ok=%v", m.ID, ok)
	}
	if _, prime, _ := Match(nil, "Some Speaker"); prime {
		t.Error("a stranger without Prime manufacturer data matched")
	}
}

// Every model needs a decoder for its kind, or the backend cannot run it.
func TestEveryModelHasADecoder(t *testing.T) {
	seen := map[byte]ID{}
	for _, m := range Models {
		if _, ok := device.NewDecoder(m.Kind); !ok {
			t.Errorf("%s: no decoder registered for kind %q", m.ID, m.Kind)
		}
		if other, dup := seen[m.ModelByte]; dup {
			t.Errorf("%s and %s share model byte %#x", m.ID, other, m.ModelByte)
		}
		seen[m.ModelByte] = m.ID
	}
}
