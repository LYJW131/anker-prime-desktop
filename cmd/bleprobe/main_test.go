package main

import (
	"bytes"
	"testing"
)

// The vector comes from the Python debugger: ff09.build_frame(1, 1,
// CryptoContext().encrypt(build_tlv([(0xA1, pack("<I", 1700000000))]))).
func TestHandshake0001(t *testing.T) {
	want := mustHex("FF09200003000140010A823F23B55F79135D1B2D3C46C64BF78438C2A6340701")
	got, err := handshake0001(1700000000)
	if err != nil || !bytes.Equal(got, want) {
		t.Fatalf("got %X, %v", got, err)
	}
	f, rest, ok := cutFrame(append(append([]byte{0x00}, got...), 0xFF))
	if !ok || !bytes.Equal(f, want) || !bytes.Equal(rest, []byte{0xFF}) {
		t.Fatalf("cutFrame: %X %X %v", f, rest, ok)
	}
	cmd, enc, body, err := parseFrame(f)
	if err != nil || cmd != 0x0001 || !enc {
		t.Fatalf("parseFrame: %#04x %v %v", cmd, enc, err)
	}
	plain, err := gcm().Open(nil, initialNonce, body, gcmAAD)
	if err != nil || !bytes.Equal(plain, []byte{0xA1, 0x04, 0x00, 0xF1, 0x53, 0x65}) {
		t.Fatalf("plain %X, %v", plain, err)
	}
}
