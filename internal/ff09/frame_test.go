package ff09

import (
	"bytes"
	"math/rand"
	"testing"
)

func TestParseFrameFlags(t *testing.T) {
	// A reply (0x4A) to 0x0200 in the telemetry group: encrypted and acked.
	raw := finishFrame([]byte{0xFF, 0x09, 0, 0, 0x03, 0x01, 0x11, 0x4A, 0x00, 0xDE, 0xAD})
	f, ok := ParseFrame(raw)
	if !ok {
		t.Fatal("not parsed")
	}
	if f.Command != 0x0200 || !f.Encrypted || !f.Ack || f.Group != 0x11 || !f.ChecksumOK {
		t.Errorf("got %+v", f)
	}
	if !bytes.Equal(f.Body, []byte{0xDE, 0xAD}) {
		t.Errorf("body %x", f.Body)
	}

	// The power bank's push: plaintext, no ack bit; the group and b5 differ
	// from the host's and are not interpreted.
	raw = finishFrame([]byte{0xFF, 0x09, 0, 0, 0x03, 0x01, 0x11, 0x03, 0x00, 0xA1})
	f, _ = ParseFrame(raw)
	if f.Command != 0x0300 || f.Encrypted || f.Ack {
		t.Errorf("push: got %+v", f)
	}

	if _, ok := ParseFrame(raw[:9]); ok {
		t.Error("short frame parsed")
	}
}

func TestBuildFrameRoundTrip(t *testing.T) {
	for _, body := range [][]byte{nil, {1}, bytes.Repeat([]byte{7}, 183)} {
		raw := BuildFrame(GroupSession, CmdKeyExchange, body)
		f, ok := ParseFrame(raw)
		if !ok || f.Command != CmdKeyExchange || !f.Encrypted || f.Ack || f.Group != GroupSession ||
			!f.ChecksumOK || !bytes.Equal(f.Body, body) || len(raw) != len(body)+10 {
			t.Fatalf("body %d: %+v", len(body), f)
		}
	}
}

func finishFrame(b []byte) []byte {
	b[2], b[3] = byte(len(b)+1), byte((len(b)+1)>>8)
	return append(b, Checksum(b))
}

func TestAssemblerSplitMergedAndGarbage(t *testing.T) {
	a := BuildFrame(1, 0x0001, bytes.Repeat([]byte{1}, 30))
	b := BuildFrame(15, 0x0300, bytes.Repeat([]byte{2}, 200))
	c := BuildFrame(1, 0x0029, bytes.Repeat([]byte{3}, 5))
	stream := append(append(append([]byte{}, a...), b...), c...)

	t.Run("byte at a time", func(t *testing.T) {
		var asm Assembler
		var got [][]byte
		for _, x := range stream {
			got = append(got, asm.Feed([]byte{x})...)
		}
		if len(got) != 3 || !bytes.Equal(got[0], a) || !bytes.Equal(got[1], b) || !bytes.Equal(got[2], c) {
			t.Fatalf("got %d frames", len(got))
		}
	})
	t.Run("all at once", func(t *testing.T) {
		var asm Assembler
		if got := asm.Feed(stream); len(got) != 3 {
			t.Fatalf("got %d frames", len(got))
		}
	})
	t.Run("resync after garbage", func(t *testing.T) {
		var asm Assembler
		junk := []byte{0x00, 0xFF, 0x01, 0xFF, 0x09, 0x02, 0x00} // the last two are a bogus header, len 2
		got := asm.Feed(append(append(junk, a...), b...))
		if len(got) != 2 || !bytes.Equal(got[0], a) || !bytes.Equal(got[1], b) {
			t.Fatalf("got %d frames", len(got))
		}
	})
	t.Run("oversized length is dropped", func(t *testing.T) {
		var asm Assembler
		bad := []byte{0xFF, 0x09, 0x01, 0x20} // len 0x2001 > 4096
		got := asm.Feed(append(bad, a...))
		if len(got) != 1 || !bytes.Equal(got[0], a) {
			t.Fatalf("got %d frames", len(got))
		}
	})
	t.Run("reset drops a partial frame", func(t *testing.T) {
		var asm Assembler
		asm.Feed(a[:10])
		asm.Reset()
		if got := asm.Feed(b); len(got) != 1 || !bytes.Equal(got[0], b) {
			t.Fatalf("got %d frames", len(got))
		}
	})
	t.Run("random cuts", func(t *testing.T) {
		rng := rand.New(rand.NewSource(1))
		for range 200 {
			var asm Assembler
			var got [][]byte
			for rest := stream; len(rest) > 0; {
				n := min(1+rng.Intn(60), len(rest))
				got = append(got, asm.Feed(rest[:n])...)
				rest = rest[n:]
			}
			if len(got) != 3 || !bytes.Equal(got[1], b) {
				t.Fatalf("got %d frames", len(got))
			}
		}
	})
}
