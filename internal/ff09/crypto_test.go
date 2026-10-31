package ff09

import (
	"bytes"
	"testing"
)

func TestCryptoRoundTripAndFixedNonce(t *testing.T) {
	c := NewCrypto()
	a := c.Encrypt([]byte("hello"))
	b := c.Encrypt([]byte("hello"))
	if !bytes.Equal(a, b) {
		t.Error("the nonce must not advance: identical frames encrypt identically")
	}
	if len(a) != 5+16 {
		t.Errorf("ciphertext+tag is %d bytes", len(a))
	}
	got, err := c.Decrypt(a)
	if err != nil || string(got) != "hello" {
		t.Fatalf("decrypt: %q %v", got, err)
	}
	a[0] ^= 1
	if _, err := c.Decrypt(a); err == nil {
		t.Error("tampered ciphertext accepted")
	}
}

func TestSetSession(t *testing.T) {
	c := NewCrypto()
	initial := c.Encrypt([]byte("x"))
	if err := c.SetSession(make([]byte, 15), make([]byte, 12)); err == nil {
		t.Error("short key accepted")
	}
	if err := c.SetSession(make([]byte, 16), make([]byte, 11)); err == nil {
		t.Error("short nonce accepted")
	}
	if c.InSession() {
		t.Error("failed SetSession switched the context")
	}
	if err := c.SetSession(bytes.Repeat([]byte{9}, 16), bytes.Repeat([]byte{8}, 12)); err != nil {
		t.Fatal(err)
	}
	if !c.InSession() || bytes.Equal(initial, c.Encrypt([]byte("x"))) {
		t.Error("session key not in use")
	}
	c.Reset()
	if c.InSession() || !bytes.Equal(initial, c.Encrypt([]byte("x"))) {
		t.Error("Reset did not return to the initial key")
	}
}

func TestOpenFollowsTheFrameFlag(t *testing.T) {
	c := NewCrypto()
	enc, _ := ParseFrame(c.BuildFrame(GroupSession, CmdHello, []byte("body")))
	if got, err := c.Open(enc); err != nil || string(got) != "body" {
		t.Fatalf("encrypted: %q %v", got, err)
	}
	plain, _ := ParseFrame(finishFrame([]byte{0xFF, 0x09, 0, 0, 0x03, 0x01, 0x11, 0x03, 0x00, 'o', 'k'}))
	if got, err := c.Open(plain); err != nil || string(got) != "ok" {
		t.Fatalf("plaintext: %q %v", got, err)
	}
}

// TestEcdhMatchesPython checks Derive against a shared secret computed by
// Python's cryptography library for two fixed private scalars (0x11.. and
// 0x22..): key = shared[0:16], nonce = shared[16:28], no KDF.
func TestEcdhMatchesPython(t *testing.T) {
	const (
		aPub  = "0217e617f0b6443928278f96999e69a23a4f2c152bdf6d6cdf66e5b80282d4ed194a7debcb97712d2dda3ca85aa8765a56f45fc758599652f2897c65306e5794"
		bPub  = "d65a93977caa3d1b081852ff57a79e465f1660577304baead505dd3a48589cf350185e895372df6221ea3a137557e473fddb6755f05bd507c3c533fce9c91285"
		key   = "ccfc261f58193c98ca4ad4a53bbac6f0"
		nonce = "ee29bc4d4843809044690862"
	)
	a := ecdhFromScalar(t, bytes.Repeat([]byte{0x11}, 32))
	b := ecdhFromScalar(t, bytes.Repeat([]byte{0x22}, 32))
	if got := a.PublicCoordinates(); !bytes.Equal(got, unhex(t, aPub)) {
		t.Fatalf("a public: %x", got)
	}
	for name, pair := range map[string]struct {
		e   *Ecdh
		pub string
	}{"a from b": {a, bPub}, "b from a": {b, aPub}} {
		k, n, err := pair.e.Derive(unhex(t, pair.pub))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(k, unhex(t, key)) || !bytes.Equal(n, unhex(t, nonce)) {
			t.Errorf("%s: key %x nonce %x", name, k, n)
		}
	}
	if _, _, err := a.Derive(make([]byte, 63)); err == nil {
		t.Error("short device key accepted")
	}
	if _, _, err := a.Derive(make([]byte, 64)); err == nil {
		t.Error("point off the curve accepted")
	}
}

func TestFindDevicePublicKey(t *testing.T) {
	key := bytes.Repeat([]byte{0x5A}, 64)
	reply := append([]byte{0x00, 0xA1, 0x40}, key...)
	if got, ok := FindDevicePublicKey(reply); !ok || !bytes.Equal(got, key) {
		t.Errorf("not found in %x", reply[:8])
	}
	if _, ok := FindDevicePublicKey([]byte{0x00, 0xA1, 0x01, 0x01}); ok {
		t.Error("a short A1 taken for a key")
	}
}

func TestEstablishSession(t *testing.T) {
	host := ecdhFromScalar(t, bytes.Repeat([]byte{0x11}, 32))
	dev := ecdhFromScalar(t, bytes.Repeat([]byte{0x22}, 32))
	reply := append([]byte{0x00, 0xA1, 0x40}, dev.PublicCoordinates()...)

	hostCrypto, devCrypto := NewCrypto(), NewCrypto()
	if err := hostCrypto.EstablishSession(host, reply); err != nil {
		t.Fatal(err)
	}
	if !hostCrypto.InSession() {
		t.Fatal("not in session")
	}
	// The device derives the same key from our public key.
	if err := devCrypto.EstablishSession(dev, append([]byte{0x00, 0xA1, 0x40}, host.PublicCoordinates()...)); err != nil {
		t.Fatal(err)
	}
	got, err := devCrypto.Decrypt(hostCrypto.Encrypt([]byte("ping")))
	if err != nil || string(got) != "ping" {
		t.Fatalf("%q %v", got, err)
	}
	if err := NewCrypto().EstablishSession(host, []byte{0x00, 0xA1, 0x01, 0x01}); err == nil {
		t.Error("reply without a key accepted")
	}
}
