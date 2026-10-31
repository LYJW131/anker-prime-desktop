package ff09

import "strings"

// Identity is what the 0x0029 reply tells about the device. Empty strings are
// unset.
type Identity struct {
	Serial   string
	Firmware string
	MAC      string
	Label    string // a fixed product label ("Charging"), not live state
}

// Apply0029 merges a 0x0029 reply into id. Fields are matched on content as
// well as TLV number because firmware revisions move them around. With
// chargerPrefix set, a payload starting 03 00 is walked from offset 6, the
// charger's extra rule.
func (id *Identity) Apply0029(plain []byte, chargerPrefix bool) {
	offset := TLVOffset(plain)
	if chargerPrefix && offset == 0 && len(plain) > 5 && plain[0] == 0x03 && plain[1] == 0x00 {
		offset = 6
	}
	for _, t := range ParseTLV(plain, offset) {
		text := printableASCII(t.Value)
		switch {
		case t.Type == 0xA2 && text != "":
			id.Label = text
		case t.Type == 0xA3 && looksLikeFirmware(text):
			id.Firmware = text
		case t.Type == 0xA4 && looksLikeSerial(text):
			id.Serial = text
		case t.Type == 0xA5 && len(t.Value) >= 6:
			if mac := macFromBytes(t.Value); mac != "" {
				id.MAC = mac
			}
		}
		if id.Firmware == "" && looksLikeFirmware(text) {
			id.Firmware = text
		}
		if id.Serial == "" && looksLikeSerial(text) {
			id.Serial = text
		}
	}
}

// printableASCII keeps bytes 0x20..0x7E and trims the spaces around them.
func printableASCII(b []byte) string {
	var sb strings.Builder
	for _, c := range b {
		if c >= 0x20 && c <= 0x7E {
			sb.WriteByte(c)
		}
	}
	return strings.Trim(sb.String(), " ")
}

func isDigits(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true // an empty string passes, like Python's all() over nothing
}

func looksLikeFirmware(text string) bool {
	parts := strings.Split(strings.TrimLeft(text, "vV"), ".")
	if len(parts) < 2 {
		return false
	}
	for _, p := range parts {
		if p != "" && !isDigits(p) {
			return false
		}
	}
	return true
}

func looksLikeSerial(text string) bool {
	if len(text) < 10 || len(text) > 30 {
		return false
	}
	for i := 0; i < len(text); i++ {
		c := text[i]
		ok := c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c == '_' || c == '-'
		if !ok {
			return false
		}
	}
	return !looksLikeFirmware(text)
}

func macFromBytes(v []byte) string {
	if len(v) < 6 {
		return ""
	}
	zero := true
	for _, b := range v[:6] {
		if b != 0 {
			zero = false
		}
	}
	if zero {
		return ""
	}
	if v[0] >= 0x20 && v[0] <= 0x7E && v[1] >= 0x20 && v[1] <= 0x7E {
		return "" // printable start: this is text, not an address
	}
	const hexd = "0123456789ABCDEF"
	var sb strings.Builder
	for i, b := range v[:6] {
		if i > 0 {
			sb.WriteByte(':')
		}
		sb.WriteByte(hexd[b>>4])
		sb.WriteByte(hexd[b&15])
	}
	return sb.String()
}
