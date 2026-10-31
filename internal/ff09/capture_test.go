package ff09

import (
	"bytes"
	"encoding/hex"
	"math/rand"
	"path/filepath"
	"strings"
	"testing"
)

// TestCaptureFramesRoundTrip feeds every recorded device frame through the
// Assembler and ParseFrame, once as recorded and once with the stream of the
// whole capture cut at random places, and checks what the recorder saw: the
// command, the encrypted and ack flags and, for plaintext frames, the payload.
// Encrypted frames go through the initial key while it is in force.
func TestCaptureFramesRoundTrip(t *testing.T) {
	decrypted, plaintext := 0, 0
	defer func() {
		if decrypted < 60 || plaintext < 400 {
			t.Errorf("only %d decrypted and %d plaintext frames checked; are the captures there?", decrypted, plaintext)
		}
	}()
	for _, file := range captureFiles(t) {
		t.Run(filepath.Base(file), func(t *testing.T) {
			var rx []capEvent
			for _, e := range readCapture(t, file) {
				if e.Dir == "rx" {
					rx = append(rx, e)
				}
			}
			var stream []byte
			for _, e := range rx {
				stream = append(stream, unhex(t, e.Raw)...)
			}

			check := func(label string, frames [][]byte) {
				t.Helper()
				if len(frames) != len(rx) {
					t.Fatalf("%s: %d frames, capture has %d", label, len(frames), len(rx))
				}
				crypto := NewCrypto()
				sessionKeyLive := false // from the 0x0021 reply on, frames use a key no one has
				for i, raw := range frames {
					e := rx[i]
					f, ok := ParseFrame(raw)
					if !ok {
						t.Fatalf("%s #%d: not parsed", label, i)
					}
					if f.Command != e.Cmd || f.Encrypted != e.Enc || f.Ack != e.Ack {
						t.Fatalf("%s #%d: got cmd %#x enc %v ack %v, capture cmd %#x enc %v ack %v",
							label, i, f.Command, f.Encrypted, f.Ack, e.Cmd, e.Enc, e.Ack)
					}
					if e.Plain == nil {
						continue
					}
					want := unhex(t, *e.Plain)
					if !f.Encrypted {
						if !bytes.Equal(f.Body, want) {
							t.Fatalf("%s #%d: plaintext body\n got  %x\n want %x", label, i, f.Body, want)
						}
						plaintext++
						continue
					}
					if sessionKeyLive {
						continue
					}
					got, err := crypto.Open(f)
					if err != nil || !bytes.Equal(got, want) {
						t.Fatalf("%s #%d (cmd %#x): decrypt %x err %v, want %x", label, i, f.Command, got, err, want)
					}
					decrypted++
					if f.Command == CmdKeyExchange {
						sessionKeyLive = true
					}
				}
			}

			var asm Assembler
			var whole [][]byte
			for _, e := range rx {
				whole = append(whole, asm.Feed(unhex(t, e.Raw))...)
			}
			check("as recorded", whole)

			rng := rand.New(rand.NewSource(42))
			for round := range 20 {
				var split [][]byte
				var asm Assembler
				for rest := stream; len(rest) > 0; {
					n := min(1+rng.Intn(70), len(rest))
					split = append(split, asm.Feed(rest[:n])...)
					rest = rest[n:]
				}
				check("random cut "+string(rune('A'+round)), split)
			}
		})
	}
}

// TestCaptureHandshakeRebuilds takes the host's own frames from every capture,
// reads the epochs back out of the ciphertext and rebuilds the frames with the
// Go step builders. AES-GCM with a fixed nonce is deterministic, so this pins
// the TLV order, the framing, the checksum and the cipher in one comparison.
// 0x0022 onward is under a session key no one has.
func TestCaptureHandshakeRebuilds(t *testing.T) {
	rebuilt := 0
	for _, file := range captureFiles(t) {
		var tx []capEvent
		for _, e := range readCapture(t, file) {
			if e.Dir == "tx" {
				tx = append(tx, e)
			}
		}
		crypto := NewCrypto()
		bodyOf := map[uint16][]byte{}
		var e1, e2 uint32
		for _, e := range tx {
			if e.Cmd == CmdTimezone {
				break
			}
			raw := unhex(t, e.Raw)
			f, ok := ParseFrame(raw)
			if !ok || !f.Encrypted || f.Command != e.Cmd {
				t.Fatalf("%s: tx %#x not parsed as encrypted", file, e.Cmd)
			}
			plain, err := crypto.Decrypt(f.Body)
			if err != nil {
				t.Fatalf("%s: tx %#x: %v", file, e.Cmd, err)
			}
			bodyOf[e.Cmd] = plain
			tlvs := ParseTLV(plain, 0)
			switch e.Cmd {
			case CmdHello:
				e1 = leU32(t, tlvs[0].Value)
			case CmdSetup:
				e2 = leU32(t, tlvs[0].Value)
			}
		}
		steps := HandshakeSteps(e1, e2)
		var pubKey []byte
		for _, tlv := range ParseTLV(bodyOf[CmdKeyExchange], 0) {
			if tlv.Type == 0xA1 {
				pubKey = tlv.Value
			}
		}
		ke, err := KeyExchangeStep(pubKey)
		if err != nil {
			t.Fatalf("%s: %v", file, err)
		}
		steps = append(steps, ke)
		recorded := map[uint16]string{}
		for _, e := range tx {
			recorded[e.Cmd] = e.Raw
		}
		for _, st := range steps {
			want, ok := recorded[st.Command]
			if !ok {
				continue // e.g. a capture that stops before the key exchange
			}
			got := strings.ToUpper(hex.EncodeToString(st.Frame(crypto)))
			if got != strings.ToUpper(want) {
				t.Errorf("%s: 0x%04X\n got  %s\n want %s", filepath.Base(file), st.Command, got, want)
			}
			rebuilt++
		}
	}
	if rebuilt < 40 {
		t.Errorf("only %d frames rebuilt; are the captures there?", rebuilt)
	}
}

func leU32(t testing.TB, b []byte) uint32 {
	t.Helper()
	if len(b) != 4 {
		t.Fatalf("epoch is %d bytes", len(b))
	}
	return uint32(b[0]) | uint32(b[1])<<8 | uint32(b[2])<<16 | uint32(b[3])<<24
}
