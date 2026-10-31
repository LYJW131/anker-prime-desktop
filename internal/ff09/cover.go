package ff09

import (
	"errors"
	"fmt"
	"hash/crc32"
)

// JPEGHash is the hash_code the charger knows a picture by: the IEEE CRC-32 of
// the JPEG bytes.
func JPEGHash(jpeg []byte) uint32 { return crc32.ChecksumIEEE(jpeg) }

// SelectCoverTLVs is the 47-byte 0x021F body that shows a picture whose pixels
// are on the device. The reply says whether they are (ResultNotOnDevice if
// not), but only the E1 id in later telemetry proves the screen changed.
func SelectCoverTLVs(id, hash, epoch uint32) []TLV {
	return []TLV{
		{0xA1, []byte{0x21}},
		{0xA3, []byte{0x01, CoverTypeCustom}},
		{0xA4, append([]byte{0x04}, u32le(id)...)},
		{0xA5, append([]byte{0x04}, u32le(hash)...)},
		{0xFD, append([]byte{0x00}, "SmallChargingUrl"...)},
		{0xFE, append([]byte{0x03}, EpochBytes(epoch)...)},
	}
}

// SelectCoverBody is SelectCoverTLVs encoded.
func SelectCoverBody(id, hash, epoch uint32) []byte {
	return mustBody(SelectCoverTLVs(id, hash, epoch))
}

// TransferStartTLVs is the 49-byte 0x0220 body that opens a pixel transfer of
// size bytes in count chunks of ChunkSize, acked every AckEvery chunks.
func TransferStartTLVs(id, hash, size uint32, count uint16, epoch uint32) []TLV {
	return []TLV{
		{0xA1, []byte{0x21}},
		{0xA2, []byte{0x01, 0x01}}, // meaning unknown
		{0xA3, append([]byte{0x04}, u32le(id)...)},
		{0xA4, append([]byte{0x04}, u32le(hash)...)},
		{0xA5, append([]byte{0x03}, u32le(size)...)},
		{0xA6, []byte{0x01, AckEvery}},
		{0xA7, append([]byte{0x02}, u16le(ChunkSize)...)},
		{0xA8, append([]byte{0x02}, u16le(count)...)},
		{0xFE, append([]byte{0x03}, EpochBytes(epoch)...)},
	}
}

// TransferStartBody is TransferStartTLVs encoded.
func TransferStartBody(id, hash, size uint32, count uint16, epoch uint32) []byte {
	return mustBody(TransferStartTLVs(id, hash, size, count, epoch))
}

// ChunkTLVs is the 167-byte 0x0221 body: A2 the chunk index, A3 the JPEG slice
// zero-padded to ChunkSize so every frame is 193 bytes on the wire.
func ChunkTLVs(seq uint16, data []byte) ([]TLV, error) {
	if len(data) > ChunkSize {
		return nil, fmt.Errorf("ff09: chunk of %d bytes exceeds %d", len(data), ChunkSize)
	}
	payload := make([]byte, 1+ChunkSize)
	payload[0] = 0x04
	copy(payload[1:], data)
	return []TLV{
		{0xA1, []byte{0x21}},
		{0xA2, append([]byte{0x02}, u16le(seq)...)},
		{0xA3, payload},
	}, nil
}

// ChunkBody is ChunkTLVs encoded.
func ChunkBody(seq uint16, data []byte) ([]byte, error) {
	tlvs, err := ChunkTLVs(seq, data)
	if err != nil {
		return nil, err
	}
	return BuildTLV(tlvs)
}

// SplitJPEG cuts the picture into ChunkSize slices; the last may be shorter.
func SplitJPEG(jpeg []byte) ([][]byte, error) {
	if len(jpeg) == 0 {
		return nil, errors.New("ff09: empty JPEG")
	}
	var out [][]byte
	for i := 0; i < len(jpeg); i += ChunkSize {
		out = append(out, jpeg[i:min(i+ChunkSize, len(jpeg))])
	}
	return out, nil
}

// ChunkNeedsAck reports whether chunk seq of count is sent with a wait for the
// device's ack: every AckEvery-th chunk and the last. Sending the others
// unpaced is fine, but sending acked ones without waiting makes the device
// answer ResultOverflow.
func ChunkNeedsAck(seq, count int) bool {
	return seq+1 == count || (seq+1)%AckEvery == 0
}

// CoverSteps lay out a whole push: optionally 0x021F first, then 0x0220 and
// every 0x0221 with the pacing the official app uses. The caller still checks
// the ACK of each step that expects one (ParseAck, Ack.OK).
func CoverSteps(jpeg []byte, id, hash, epoch uint32, selectFirst bool) ([]Step, error) {
	chunks, err := SplitJPEG(jpeg)
	if err != nil {
		return nil, err
	}
	if len(chunks) > 0xFFFF {
		return nil, fmt.Errorf("ff09: %d chunks do not fit a u16", len(chunks))
	}
	g := GroupTelemetry
	var steps []Step
	if selectFirst {
		s, err := newStep(g, CmdSelectCover, SelectCoverTLVs(id, hash, epoch), true, PauseSelect)
		if err != nil {
			return nil, err
		}
		steps = append(steps, s)
	}
	start, err := newStep(g, CmdTransferStart, TransferStartTLVs(id, hash, uint32(len(jpeg)), uint16(len(chunks)), epoch), true, PauseTransfer)
	if err != nil {
		return nil, err
	}
	steps = append(steps, start)
	for i, data := range chunks {
		tlvs, err := ChunkTLVs(uint16(i), data)
		if err != nil {
			return nil, err
		}
		s, err := newStep(g, CmdTransferData, tlvs, false, PauseChunk)
		if err != nil {
			return nil, err
		}
		if ChunkNeedsAck(i, len(chunks)) {
			s.ExpectReply, s.Timeout, s.Pause = true, ChunkAckTimeout, 0
		}
		if i == len(chunks)-1 {
			s.Pause = PauseAfterChunks
		}
		steps = append(steps, s)
	}
	return steps, nil
}

// Ack is a reply to a write command. Its first plaintext byte is a result
// code, not a status flag, so it is read before the TLVs.
type Ack struct {
	Result byte
	TLVs   []TLV
}

// ParseAck decodes the plaintext of a 0x021F, 0x0220 or 0x0221 reply.
func ParseAck(plain []byte) (Ack, error) {
	if len(plain) == 0 {
		return Ack{}, errors.New("ff09: empty ack")
	}
	return Ack{Result: plain[0], TLVs: ParseTLV(plain, 1)}, nil
}

// OK reports a success code: ResultOK, or ResultTransferComplete on the last
// chunk.
func (a Ack) OK() bool { return a.Result == ResultOK || a.Result == ResultTransferComplete }

// Index is the chunk index in A2 of a 0x0221 ack: the next one expected, or the
// chunk count on the last ack.
func (a Ack) Index() (uint16, bool) {
	for _, t := range a.TLVs {
		if t.Type == 0xA2 {
			if v := ReadTypedValue(t.Value); v.Tag == 0x02 && v.HasInt {
				return uint16(v.U), true
			}
		}
	}
	return 0, false
}

// ResultName names a result code for logs and messages.
func ResultName(code byte) string {
	switch code {
	case ResultOK:
		return "ok"
	case ResultBadBody:
		return "bad body"
	case ResultTransferComplete:
		return "transfer complete"
	case ResultNotOnDevice:
		return "not on device"
	case ResultOverflow:
		return "overflow"
	}
	return fmt.Sprintf("unknown 0x%02X", code)
}
