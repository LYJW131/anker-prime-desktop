package ff09

import (
	"encoding/hex"
	"strings"
	"testing"
)

// The vectors below come from the Swift reference implementation's tests
// (MacTelemetryHub ProtocolTests) and its notes; trust the hex, not any prose
// around it.

func TestStatusProbeFrame(t *testing.T) {
	// buildTLV([A1:{21}, FE:{01 02 03 04}]) under the initial key, group 0F, cmd 0200.
	tlv, err := BuildTLV([]TLV{{0xA1, []byte{0x21}}, {0xFE, []byte{1, 2, 3, 4}}})
	if err != nil {
		t.Fatal(err)
	}
	eq(t, "tlv", hex.EncodeToString(tlv), "a10121fe0401020304")
	c := NewCrypto()
	ct := c.Encrypt(tlv)
	eq(t, "ciphertext", strings.ToUpper(hex.EncodeToString(ct)), "0A871E2CE23B2BB1FA61FE8562D3A86DA44F5E62BC5C2FB0A2")
	frame := BuildFrame(GroupTelemetry, CmdStatus, ct)
	eq(t, "frame", strings.ToUpper(hex.EncodeToString(frame)),
		"FF09230003000F42000A871E2CE23B2BB1FA61FE8562D3A86DA44F5E62BC5C2FB0A2F9")
}

func TestSelectCoverVector(t *testing.T) {
	body := SelectCoverBody(24551, 0x1DA2DDCA, 1)
	eq(t, "body", strings.ToUpper(hex.EncodeToString(body)),
		"A10121A3020103A40504E75F0000A50504CADDA21DFD1100536D616C6C4368617267696E6755726CFE050301000000")
}

func TestTransferStartVector(t *testing.T) {
	body := TransferStartBody(45470, 0xC126BFEF, 25397, 163, 1)
	eq(t, "body", strings.ToUpper(hex.EncodeToString(body)),
		"A10121A2020101A305049EB10000A40504EFBF26C1A5050335630000A602010AA703029C00A80302A300FE050301000000")
}

// TestBodyLengths pins the plaintext sizes and the wire frame sizes the
// charger's firmware expects: 47, 49 and 167 bytes, in frames of 73, 75 and
// 193 bytes (16 for the GCM tag, 10 for the frame).
func TestBodyLengths(t *testing.T) {
	chunk, err := ChunkBody(0, make([]byte, ChunkSize))
	if err != nil {
		t.Fatal(err)
	}
	probe, err := RealtimeProbeBody(strings.Repeat("a", 40), 1)
	if err != nil {
		t.Fatal(err)
	}
	c := NewCrypto()
	for _, tc := range []struct {
		name         string
		cmd          uint16
		body         []byte
		bodyLen, len int
	}{
		{"select", CmdSelectCover, SelectCoverBody(1, 2, 3), 47, 73},
		{"transfer start", CmdTransferStart, TransferStartBody(1, 2, 3, 4, 5), 49, 75},
		{"chunk", CmdTransferData, chunk, 167, 193},
		{"realtime probe", CmdRealtime, probe, 61, 87},
	} {
		eq(t, tc.name+" body", len(tc.body), tc.bodyLen)
		eq(t, tc.name+" frame", len(c.BuildFrame(GroupTelemetry, tc.cmd, tc.body)), tc.len)
	}
}

func TestChunkBodyPadsAndRejects(t *testing.T) {
	short, err := ChunkBody(7, []byte{0xAA, 0xBB})
	if err != nil {
		t.Fatal(err)
	}
	want := "a10121" + "a2030207" + "00" + "a39d04aabb" + strings.Repeat("00", ChunkSize-2)
	eq(t, "padded chunk", hex.EncodeToString(short), want)
	if _, err := ChunkBody(0, make([]byte, ChunkSize+1)); err == nil {
		t.Error("oversized chunk accepted")
	}
}
