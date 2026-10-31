package ff09

import (
	"bytes"
	"testing"
)

func TestBuildAndParseTLV(t *testing.T) {
	items := []TLV{{0xA2, []byte{1, 2}}, {0xA1, nil}, {0xFE, []byte{9}}}
	body, err := BuildTLV(items)
	if err != nil {
		t.Fatal(err)
	}
	eq(t, "order is kept", string(body), string([]byte{0xA2, 2, 1, 2, 0xA1, 0, 0xFE, 1, 9}))
	got := ParseTLV(body, 0)
	if len(got) != 3 || got[0].Type != 0xA2 || got[1].Type != 0xA1 || len(got[1].Value) != 0 || got[2].Type != 0xFE {
		t.Fatalf("got %+v", got)
	}
	if _, err := BuildTLV([]TLV{{0xA1, make([]byte, 256)}}); err == nil {
		t.Error("256-byte value accepted")
	}
}

func TestParseTLVStopsOnOverflow(t *testing.T) {
	// The second element claims 5 bytes but only 2 follow: stop, do not guess.
	got := ParseTLV([]byte{0xA1, 1, 7, 0xA2, 5, 1, 2}, 0)
	if len(got) != 1 || got[0].Type != 0xA1 {
		t.Errorf("got %+v", got)
	}
	if got := ParseTLV([]byte{0xA1}, 0); len(got) != 0 {
		t.Errorf("lone type byte parsed: %+v", got)
	}
	if got := ParseTLV([]byte{0xA1, 0}, 7); len(got) != 0 {
		t.Errorf("offset past the end parsed: %+v", got)
	}
}

func TestTLVOffset(t *testing.T) {
	eq(t, "status byte", TLVOffset([]byte{0x00, 0xA1}), 1)
	eq(t, "push", TLVOffset([]byte{0xA1, 0x01}), 0)
	eq(t, "empty", TLVOffset(nil), 0)
}

func TestReadTypedValue(t *testing.T) {
	v := ReadTypedValue([]byte{0x00, 'a', 'b', '.', '.'})
	if !v.HasText || v.Text != "ab" {
		t.Errorf("text: %+v", v)
	}
	v = ReadTypedValue([]byte{0x00, 'A', 0xC3, 0xA9, 0x01, 'B'})
	if v.Text != "A..B" {
		t.Errorf("non-ASCII text %q", v.Text)
	}
	v = ReadTypedValue([]byte{0x01, 0xFE})
	if !v.HasInt || v.U != 254 || v.I != -2 {
		t.Errorf("u8: %+v", v)
	}
	v = ReadTypedValue([]byte{0x02, 0x9C, 0xFF})
	if v.U != 0xFF9C || v.I != -100 {
		t.Errorf("u16: %+v", v)
	}
	v = ReadTypedValue([]byte{0x03, 0xFF, 0xFF, 0xFF, 0xFF})
	if v.U != 0xFFFFFFFF || v.I != -1 {
		t.Errorf("u32: %+v", v)
	}
	if v := ReadTypedValue([]byte{0x02, 0x01}); v.HasInt {
		t.Errorf("short u16 read as a number: %+v", v)
	}
	v = ReadTypedValue([]byte{0x04, 1, 2, 3})
	if v.HasInt || v.HasText || !bytes.Equal(v.Payload, []byte{1, 2, 3}) {
		t.Errorf("raw: %+v", v)
	}
	if v := ReadTypedValue(nil); v.Tag != 0xFF || len(v.Payload) != 0 {
		t.Errorf("empty: %+v", v)
	}
}
