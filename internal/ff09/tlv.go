package ff09

import (
	"encoding/binary"
	"fmt"
	"strings"
	"time"
)

// TLV is one type|len|value element of a frame body.
type TLV struct {
	Type  byte
	Value []byte
}

// BuildTLV concatenates items as type|len|value, in order.
func BuildTLV(items []TLV) ([]byte, error) {
	var out []byte
	for _, it := range items {
		if len(it.Value) > 0xFF {
			return nil, fmt.Errorf("ff09: TLV 0x%02X value too long: %d", it.Type, len(it.Value))
		}
		out = append(out, it.Type, byte(len(it.Value)))
		out = append(out, it.Value...)
	}
	return out, nil
}

// ParseTLV splits payload[offset:] into elements. A declared length that runs
// past the end stops the walk instead of guessing. The values alias payload.
func ParseTLV(payload []byte, offset int) []TLV {
	var out []TLV
	for i := offset; i >= 0 && i < len(payload)-1; {
		n := int(payload[i+1])
		if i+2+n > len(payload) {
			break
		}
		out = append(out, TLV{Type: payload[i], Value: payload[i+2 : i+2+n]})
		i += 2 + n
	}
	return out
}

// TLVOffset is where the TLVs of a push or snapshot start: a leading 0x00
// status byte is skipped. Safe because every TLV type is at least 0xA1. Write
// ACKs must use ParseAck instead, whose first byte is a result code that is
// not always 0.
func TLVOffset(payload []byte) int {
	if len(payload) > 0 && payload[0] == 0x00 {
		return 1
	}
	return 0
}

// TypedValue is a TLV value whose first byte is a type tag: 00 ASCII text,
// 01 u8, 02 u16le, 03 u32le, 04 raw bytes. Not every TLV is typed; the
// handshake's epoch and account id are bare.
type TypedValue struct {
	Tag     byte
	Payload []byte // the value without its tag
	Text    string // set when HasText
	HasText bool
	U       uint32 // set when HasInt
	I       int32
	HasInt  bool
}

// ReadTypedValue decodes raw as a typed value. Empty input has tag 0xFF.
func ReadTypedValue(raw []byte) TypedValue {
	if len(raw) == 0 {
		return TypedValue{Tag: 0xFF}
	}
	v := TypedValue{Tag: raw[0], Payload: raw[1:]}
	p := v.Payload
	switch {
	case v.Tag == 0x00:
		v.Text, v.HasText = asciiText(p), true
	case v.Tag == 0x01 && len(p) >= 1:
		v.U, v.I, v.HasInt = uint32(p[0]), int32(int8(p[0])), true
	case v.Tag == 0x02 && len(p) >= 2:
		u := binary.LittleEndian.Uint16(p)
		v.U, v.I, v.HasInt = uint32(u), int32(int16(u)), true
	case v.Tag == 0x03 && len(p) >= 4:
		u := binary.LittleEndian.Uint32(p)
		v.U, v.I, v.HasInt = u, int32(u), true
	}
	return v
}

// asciiText replaces everything outside printable ASCII with '.' and drops
// trailing dots, like the Python _ascii().rstrip(".").
func asciiText(b []byte) string {
	var sb strings.Builder
	for _, r := range string(b) {
		if r >= 0x20 && r < 0x7F {
			sb.WriteRune(r)
		} else {
			sb.WriteByte('.')
		}
	}
	return strings.TrimRight(sb.String(), ".")
}

// EpochOf is the Unix-seconds timestamp the protocol sends, truncated to 32
// bits.
func EpochOf(t time.Time) uint32 { return uint32(t.Unix()) }

// EpochBytes is epoch as the bare u32le the handshake and probes carry.
func EpochBytes(epoch uint32) []byte { return u32le(epoch) }

func u32le(v uint32) []byte { return binary.LittleEndian.AppendUint32(nil, v) }
func u16le(v uint16) []byte { return binary.LittleEndian.AppendUint16(nil, v) }
