package ff09

import "encoding/binary"

// Frame layout: FF 09 | len u16le | b4 b5 | group | cmd_hi | cmd_lo | body | xor.
// len counts the whole frame, xor covers every byte before it.

// Host-to-device frames carry b4 b5 = 03 00 and always set FlagEncrypted.
// Device frames vary in b4, b5 and group, so receivers ignore them and
// dispatch on command and flags only.
const (
	frameHeaderLen = 4  // FF 09 len16
	frameFixedLen  = 10 // header + b4 b5 group cmd_hi cmd_lo + xor
	maxFrameLen    = 4096
)

// Checksum is the XOR of all bytes.
func Checksum(data []byte) byte {
	var x byte
	for _, b := range data {
		x ^= b
	}
	return x
}

// BuildFrame wraps an encrypted body in a host-to-device frame.
func BuildFrame(group uint8, command uint16, ciphertext []byte) []byte {
	out := make([]byte, 0, frameFixedLen+len(ciphertext))
	out = append(out, 0xFF, 0x09, 0, 0, 0x03, 0x00, group,
		byte(command>>8)|FlagEncrypted, byte(command))
	out = append(out, ciphertext...)
	binary.LittleEndian.PutUint16(out[2:], uint16(len(out)+1))
	return append(out, Checksum(out))
}

// Frame is a decoded transport frame.
type Frame struct {
	Command    uint16
	Group      uint8
	Encrypted  bool
	Ack        bool
	Body       []byte // ciphertext when Encrypted, else the plaintext payload
	ChecksumOK bool   // informational: receivers do not reject on it
	Raw        []byte
}

// ParseFrame decodes one complete frame. It reports false when raw is too
// short to classify. Only the command and the two flags are interpreted.
func ParseFrame(raw []byte) (Frame, bool) {
	if len(raw) < frameFixedLen {
		return Frame{}, false
	}
	payload := raw[frameHeaderLen : len(raw)-1]
	if len(payload) < 5 {
		return Frame{}, false
	}
	hi, lo := payload[3], payload[4]
	return Frame{
		Command:    uint16(hi&^(FlagEncrypted|FlagAck))<<8 | uint16(lo),
		Group:      payload[2],
		Encrypted:  hi&FlagEncrypted != 0,
		Ack:        hi&FlagAck != 0,
		Body:       payload[5:],
		ChecksumOK: Checksum(raw[:len(raw)-1]) == raw[len(raw)-1],
		Raw:        raw,
	}, true
}

// Assembler rebuilds frames from BLE notification chunks. CoreBluetooth may
// split a frame across notifications or deliver several in one, so it cuts on
// the length field. Not safe for concurrent use.
type Assembler struct {
	buf []byte
}

// Feed adds a notification and returns the frames it completed.
func (a *Assembler) Feed(chunk []byte) [][]byte {
	a.buf = append(a.buf, chunk...)
	var frames [][]byte
	for len(a.buf) >= frameHeaderLen {
		if a.buf[0] != 0xFF || a.buf[1] != 0x09 {
			a.buf = a.buf[1:] // lost sync: drop a byte and retry
			continue
		}
		n := int(binary.LittleEndian.Uint16(a.buf[2:]))
		if n < frameFixedLen || n > maxFrameLen {
			a.buf = a.buf[1:]
			continue
		}
		if len(a.buf) < n {
			break
		}
		frames = append(frames, append([]byte(nil), a.buf[:n]...))
		a.buf = a.buf[n:]
	}
	if len(a.buf) == 0 {
		a.buf = nil
	}
	return frames
}

// Reset drops any partial frame, as on reconnect.
func (a *Assembler) Reset() { a.buf = nil }
