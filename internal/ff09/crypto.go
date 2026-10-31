package ff09

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/rand"
	"errors"
	"fmt"
)

// Crypto is the frame cipher: AES-128-GCM with a fixed AAD. It starts on the
// initial key and switches to the ECDH-derived session key after the 0x0021
// exchange. The firmware reuses one nonce for every frame in both directions,
// so this does too: the nonce never advances.
//
// Not safe for concurrent use; the session driver owns it.
type Crypto struct {
	aead    cipher.AEAD
	nonce   []byte
	session bool
}

// NewCrypto returns a context on the initial key.
func NewCrypto() *Crypto {
	c := &Crypto{}
	c.Reset()
	return c
}

// Reset returns to the initial key and nonce, as at the start of a connection.
func (c *Crypto) Reset() {
	c.set(InitialKey, InitialIV)
	c.session = false
}

// SetSession switches to the session key (16 bytes) and nonce (12 bytes).
func (c *Crypto) SetSession(key, nonce []byte) error {
	if len(key) != 16 {
		return fmt.Errorf("ff09: session key must be 16 bytes, got %d", len(key))
	}
	if len(nonce) != 12 {
		return fmt.Errorf("ff09: session nonce must be 12 bytes, got %d", len(nonce))
	}
	c.set(key, nonce)
	c.session = true
	return nil
}

// EstablishSession finishes the key exchange: it takes the device's public key
// from the plaintext of the 0x0021 reply, derives the shared secret with e and
// switches to the session key. Call it before sending anything else.
func (c *Crypto) EstablishSession(e *Ecdh, keyExchangeReply []byte) error {
	pub, ok := FindDevicePublicKey(keyExchangeReply)
	if !ok {
		return errors.New("ff09: the 0x0021 reply carries no 64-byte public key")
	}
	key, nonce, err := e.Derive(pub)
	if err != nil {
		return err
	}
	return c.SetSession(key, nonce)
}

// InSession reports whether the session key is active.
func (c *Crypto) InSession() bool { return c.session }

func (c *Crypto) set(key, nonce []byte) {
	block, err := aes.NewCipher(key)
	if err != nil {
		panic(err) // the key length is checked by the callers
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		panic(err)
	}
	c.aead = aead
	c.nonce = append([]byte(nil), nonce...)
}

// Encrypt returns ciphertext followed by the 16-byte tag.
func (c *Crypto) Encrypt(plain []byte) []byte {
	return c.aead.Seal(nil, c.nonce, plain, GCMAAD)
}

// Decrypt opens ciphertext followed by its tag.
func (c *Crypto) Decrypt(ct []byte) ([]byte, error) {
	return c.aead.Open(nil, c.nonce, ct, GCMAAD)
}

// BuildFrame encrypts a plaintext body and wraps it in a frame.
func (c *Crypto) BuildFrame(group uint8, command uint16, plain []byte) []byte {
	return BuildFrame(group, command, c.Encrypt(plain))
}

// Open returns the plaintext body of f, decrypting it when the frame says it
// is encrypted. The decision rests on the frame's flag alone: the power bank
// sends its telemetry in the clear.
func (c *Crypto) Open(f Frame) ([]byte, error) {
	if !f.Encrypted {
		return f.Body, nil
	}
	return c.Decrypt(f.Body)
}

// Ecdh is an ephemeral P-256 key pair, fresh per connection.
type Ecdh struct {
	priv *ecdh.PrivateKey
}

// NewEcdh generates a key pair.
func NewEcdh() (*Ecdh, error) {
	priv, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	return &Ecdh{priv: priv}, nil
}

// PublicCoordinates is our public key as 64 bytes X||Y, the A1 of 0x0021.
func (e *Ecdh) PublicCoordinates() []byte {
	return e.priv.PublicKey().Bytes()[1:] // drop the 0x04 prefix
}

// Derive turns the device's 64-byte X||Y into the session key and nonce: the
// shared secret's x coordinate, bytes 0..15 and 16..27, with no KDF.
func (e *Ecdh) Derive(device []byte) (key, nonce []byte, err error) {
	if len(device) != 64 {
		return nil, nil, fmt.Errorf("ff09: device public key must be 64 bytes, got %d", len(device))
	}
	peer, err := ecdh.P256().NewPublicKey(append([]byte{0x04}, device...))
	if err != nil {
		return nil, nil, err
	}
	shared, err := e.priv.ECDH(peer)
	if err != nil {
		return nil, nil, err
	}
	if len(shared) != 32 {
		return nil, nil, errors.New("ff09: unexpected shared secret length")
	}
	return shared[0:16], shared[16:28], nil
}

// FindDevicePublicKey pulls the 64-byte P-256 key out of the decrypted 0x0021
// reply.
func FindDevicePublicKey(plain []byte) ([]byte, bool) {
	for _, t := range ParseTLV(plain, TLVOffset(plain)) {
		if t.Type == 0xA1 && len(t.Value) == 64 {
			return t.Value, true
		}
	}
	return nil, false
}
