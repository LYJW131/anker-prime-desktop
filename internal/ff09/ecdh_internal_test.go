package ff09

import (
	"crypto/ecdh"
	"testing"
)

func ecdhFromScalar(t testing.TB, scalar []byte) *Ecdh {
	t.Helper()
	priv, err := ecdh.P256().NewPrivateKey(scalar)
	if err != nil {
		t.Fatal(err)
	}
	return &Ecdh{priv: priv}
}
