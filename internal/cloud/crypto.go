package cloud

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strconv"
	"strings"
)

// ServerPublicKeyHex is the fixed P-256 point the passport service wraps
// login passwords against (same on CN, COM and EU).
const ServerPublicKeyHex = "04c5c00c4f8d1197cc7c3167c52bf7acb054d722f0ef08dcd7e0883236e0d72a" +
	"3868d9750cb47fa4619248f3d83f0f662671dadc6e2d31c2f41db0161651c7c076"

// Envelope is the wrapped password plus the ephemeral client public key.
type Envelope struct {
	ClientPublicKeyHex string
	EncryptedPassword  string // base64
}

// PasswordEnvelope does ECDH P-256 against the server key, then
// AES-256-CBC with the shared secret as key and its first 16 bytes as IV.
func PasswordEnvelope(password string) (Envelope, error) {
	raw, err := hex.DecodeString(ServerPublicKeyHex)
	if err != nil {
		return Envelope{}, err
	}
	srv, err := ecdh.P256().NewPublicKey(raw)
	if err != nil {
		return Envelope{}, err
	}
	priv, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		return Envelope{}, err
	}
	shared, err := priv.ECDH(srv)
	if err != nil {
		return Envelope{}, err
	}
	ct, err := EncryptAES256CBC(shared, shared[:16], []byte(password))
	if err != nil {
		return Envelope{}, err
	}
	return Envelope{
		ClientPublicKeyHex: hex.EncodeToString(priv.PublicKey().Bytes()),
		EncryptedPassword:  base64.StdEncoding.EncodeToString(ct),
	}, nil
}

// EncryptAES256CBC encrypts with PKCS7 padding.
func EncryptAES256CBC(key, iv, plaintext []byte) ([]byte, error) {
	if len(key) != 32 || len(iv) != 16 {
		return nil, errors.New("cloud: AES-256-CBC needs a 32-byte key and 16-byte iv")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	pad := aes.BlockSize - len(plaintext)%aes.BlockSize
	buf := make([]byte, len(plaintext)+pad)
	copy(buf, plaintext)
	for i := len(plaintext); i < len(buf); i++ {
		buf[i] = byte(pad)
	}
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(buf, buf)
	return buf, nil
}

// ParseHashCode reads a cloud hash_code: optional 0x prefix means hex;
// otherwise hex first, then decimal (matches the Swift client).
func ParseHashCode(raw string) (uint32, bool) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return 0, false
	}
	if len(s) > 2 && (s[:2] == "0x" || s[:2] == "0X") {
		v, err := strconv.ParseUint(s[2:], 16, 32)
		return uint32(v), err == nil
	}
	if v, err := strconv.ParseUint(s, 16, 32); err == nil {
		return uint32(v), true
	}
	v, err := strconv.ParseUint(s, 10, 32)
	return uint32(v), err == nil
}
