package ff09

import (
	"bytes"
	"testing"
	"time"
)

// TestParseAck uses the replies recorded on the real charger when a picture
// was pushed under a locally chosen id.
func TestParseAck(t *testing.T) {
	for _, tc := range []struct {
		name   string
		plain  string
		result byte
		ok     bool
		index  uint16
		hasIdx bool
	}{
		{"select, not on device", "11A10131", ResultNotOnDevice, false, 0, false},
		{"transfer start", "00A10131", ResultOK, true, 0, false},
		{"chunk ack, next index 10", "00A10131A203020A00", ResultOK, true, 10, true},
		{"last chunk ack", "10A10131A203023B00", ResultTransferComplete, true, 59, true},
		{"overflow", "12A10131", ResultOverflow, false, 0, false},
		{"bad body", "04A10131", ResultBadBody, false, 0, false},
	} {
		a, err := ParseAck(unhex(t, tc.plain))
		if err != nil {
			t.Fatal(tc.name, err)
		}
		eq(t, tc.name+" result", a.Result, tc.result)
		eq(t, tc.name+" ok", a.OK(), tc.ok)
		idx, has := a.Index()
		eq(t, tc.name+" has index", has, tc.hasIdx)
		eq(t, tc.name+" index", idx, tc.index)
	}
	if _, err := ParseAck(nil); err == nil {
		t.Error("empty ack accepted")
	}
	// The result code is not skipped like a status byte: a non-zero first
	// byte must not be taken for a TLV type.
	a, _ := ParseAck(unhex(t, "11A10131"))
	if len(a.TLVs) != 1 || a.TLVs[0].Type != 0xA1 {
		t.Errorf("TLVs %+v", a.TLVs)
	}
}

func TestResultName(t *testing.T) {
	eq(t, "ok", ResultName(0x00), "ok")
	eq(t, "not on device", ResultName(0x11), "not on device")
	eq(t, "unknown", ResultName(0x7F), "unknown 0x7F")
}

func TestSplitJPEG(t *testing.T) {
	jpeg := bytes.Repeat([]byte{0xAB}, 25397)
	chunks, err := SplitJPEG(jpeg)
	if err != nil {
		t.Fatal(err)
	}
	eq(t, "chunks of the 25397-byte sample", len(chunks), 163)
	eq(t, "first chunk", len(chunks[0]), ChunkSize)
	eq(t, "last chunk", len(chunks[162]), 25397-162*ChunkSize)
	if _, err := SplitJPEG(nil); err == nil {
		t.Error("empty JPEG accepted")
	}
	// The 59-chunk push that worked on the charger.
	chunks, _ = SplitJPEG(make([]byte, 59*ChunkSize-30))
	eq(t, "59 chunks", len(chunks), 59)
}

func TestChunkNeedsAck(t *testing.T) {
	var acked []int
	for i := range 25 {
		if ChunkNeedsAck(i, 25) {
			acked = append(acked, i)
		}
	}
	want := []int{9, 19, 24}
	if len(acked) != 3 || acked[0] != want[0] || acked[1] != want[1] || acked[2] != want[2] {
		t.Errorf("acked %v, want %v", acked, want)
	}
	if !ChunkNeedsAck(0, 1) {
		t.Error("a lone chunk is the last one")
	}
	if !ChunkNeedsAck(19, 20) || ChunkNeedsAck(18, 20) {
		t.Error("count that is a multiple of 10: ack at 9, 19 only")
	}
}

func TestCoverSteps(t *testing.T) {
	jpeg := make([]byte, 25*ChunkSize-50) // 25 chunks
	steps, err := CoverSteps(jpeg, 1001, JPEGHash(jpeg), 1, true)
	if err != nil {
		t.Fatal(err)
	}
	eq(t, "step count", len(steps), 2+25)
	sel, start := steps[0], steps[1]
	eq(t, "select command", sel.Command, CmdSelectCover)
	eq(t, "select expects reply", sel.ExpectReply, true)
	eq(t, "select pause", sel.Pause, 150*time.Millisecond)
	eq(t, "start command", start.Command, CmdTransferStart)
	eq(t, "start expects reply", start.ExpectReply, true)
	eq(t, "start pause", start.Pause, 200*time.Millisecond)
	for i, s := range steps[2:] {
		eq(t, "chunk command", s.Command, CmdTransferData)
		eq(t, "chunk group", s.Group, GroupTelemetry)
		eq(t, "chunk body", len(s.Body), 167)
		wantAck := i == 9 || i == 19 || i == 24
		eq(t, "chunk ack", s.ExpectReply, wantAck)
		switch {
		case i == 24:
			eq(t, "last pause", s.Pause, 400*time.Millisecond)
			eq(t, "last timeout", s.ReplyWait(), 5*time.Second)
		case wantAck:
			eq(t, "acked pause", s.Pause, time.Duration(0))
			eq(t, "acked timeout", s.ReplyWait(), 5*time.Second)
		default:
			eq(t, "paced pause", s.Pause, 40*time.Millisecond)
		}
	}
	// Without the select, the transfer starts at once.
	steps, _ = CoverSteps(jpeg, 1001, 1, 1, false)
	eq(t, "no select", steps[0].Command, CmdTransferStart)
	if _, err := CoverSteps(nil, 1, 1, 1, true); err == nil {
		t.Error("empty JPEG accepted")
	}
}

func TestJPEGHash(t *testing.T) {
	// The IEEE CRC-32 check value.
	eq(t, "crc32 of 123456789", JPEGHash([]byte("123456789")), 0xCBF43926)
}
