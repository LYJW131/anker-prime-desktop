package device

import (
	"encoding/binary"
	"fmt"
	"reflect"
	"strings"

	"github.com/LYJW131/anker-prime-mac/internal/ff09"
)

// ChargerDecoder fills a Charger from decrypted FF09 payloads. It ports
// charger.py; spec/fixtures replay through it frame by frame. Not safe for
// concurrent use: the link owns it and hands copies (Snapshot) to the UI.
type ChargerDecoder struct {
	State Charger
}

// NewChargerDecoder returns a decoder with an empty state.
func NewChargerDecoder() *ChargerDecoder { return &ChargerDecoder{State: NewCharger()} }

// Reset forgets everything read so far, as on a new connection.
func (d *ChargerDecoder) Reset() { d.State = NewCharger() }

// Snapshot returns a deep copy of the state.
func (d *ChargerDecoder) Snapshot() Charger { return d.State.Clone() }

// Handles reports whether Apply decodes the command: the 0x0029 identity reply
// and the snapshot and realtime sets.
func (d *ChargerDecoder) Handles(command uint16) bool {
	return command == ff09.CmdIdentity || contains(chargerSnapshotCommands, command) ||
		contains(chargerRealtimeCommands, command)
}

// Apply decodes one frame's plaintext into the state and reports whether any
// field changed. A command that is not Handled leaves the state alone and
// reports false; a handled frame that carries nothing new reports false too.
func (d *ChargerDecoder) Apply(command uint16, plain []byte) (changed bool) {
	if !d.Handles(command) {
		return false
	}
	if len(d.State.Ports) != len(chargerPortOrder) {
		d.State = NewCharger() // a zero-value decoder
	}
	before := d.State.Clone()
	switch {
	case command == ff09.CmdIdentity:
		applyIdentity(&d.State.Identity, plain, true)
	case contains(chargerSnapshotCommands, command):
		d.applySnapshot(plain)
	default:
		d.applyRealtime(plain)
	}
	return !reflect.DeepEqual(before, d.State)
}

func (d *ChargerDecoder) port(i int) *Port { return &d.State.Ports[i] }

// applyRealtime decodes a 0x020A-family payload. The ports are type-04 structs
// of [mode, mV, mA, cW], scaled differently from the power bank's tenths.
func (d *ChargerDecoder) applyRealtime(plain []byte) {
	fields := tlvMap(ff09.ParseTLV(plain, ff09.TLVOffset(plain)))
	s := &d.State

	total, sawPort := 0.0, false
	for i, typ := range chargerPortStruct {
		v, ok := fields[typ]
		if !ok || v.Tag != 0x04 || len(v.Payload) < 7 {
			continue
		}
		b := v.Payload
		p := d.port(i)
		p.Mode = int(b[0])
		p.Direction = nil
		if b[0] != 0 {
			p.Direction = ptr("out")
		}
		p.VoltageV = float64(binary.LittleEndian.Uint16(b[1:])) / 1000.0
		p.CurrentA = float64(binary.LittleEndian.Uint16(b[3:])) / 1000.0
		p.PowerW = float64(binary.LittleEndian.Uint16(b[5:])) / 100.0
		p.Sampled = true
		if b[0] != 0 {
			total += p.PowerW
		}
		sawPort = true
	}
	if sawPort {
		s.TotalOutputPowerW = ptr(round2(total))
	}

	for i, typ := range chargerPortCable {
		if v, ok := fields[typ]; ok {
			applyCable(d.port(i), v)
		}
	}
	d.applyIdentityBlocks(fields)

	if e1, ok := fields[chargerTLVScreensave]; ok && len(e1.Payload) >= 4 {
		s.ScreensaverFlags = ptr(int(binary.LittleEndian.Uint16(e1.Payload)))
		s.ScreensaverID = ptr(int(binary.LittleEndian.Uint16(e1.Payload[2:])))
	}
	for i := range s.Ports {
		s.Ports[i].refresh()
	}
}

// applySnapshot decodes a 0x0200 snapshot: the identifiers, the brightness and,
// since this firmware also packs live port structs in it, the realtime pass.
func (d *ChargerDecoder) applySnapshot(plain []byte) {
	s := &d.State
	for _, t := range ff09.ParseTLV(plain, ff09.TLVOffset(plain)) {
		v := ff09.ReadTypedValue(t.Value)
		switch {
		case t.Type == 0xA2 && v.HasText && v.Text != "":
			if deref(s.Serial) == "" {
				s.Serial = ptr(v.Text)
			}
		case t.Type == 0xA4 && v.HasText && v.Text != "":
			s.ProductCode = ptr(v.Text)
		case t.Type == 0xFD && v.HasText && v.Text != "":
			s.FirmwareTag = ptr(v.Text)
		case t.Type == chargerTLVBrightness && v.HasInt:
			s.ScreenBrightness = intPtr(v.U)
		}
	}
	d.applyRealtime(plain)
}

// applyIdentityBlocks decodes the per-port connected-device blocks: 0xB4 holds
// USB vendor and product ids (inferred from captures), 0xB5 brand codes.
func (d *ChargerDecoder) applyIdentityBlocks(fields map[byte]ff09.TypedValue) {
	if v, ok := fields[chargerTLVIdentity]; ok && v.Tag == 0x04 && len(v.Payload) >= 12 {
		for i := range chargerPortOrder {
			vid := binary.LittleEndian.Uint16(v.Payload[i*4:])
			pid := binary.LittleEndian.Uint16(v.Payload[i*4+2:])
			p := d.port(i)
			if identitySentinels[vid] {
				p.Vendor, p.DeviceName = nil, nil
				continue
			}
			name, ok := usbVendorNames[vid]
			if !ok {
				name = fmt.Sprintf("VID %04X", vid)
			}
			p.Vendor = ptr(name)
			p.DeviceName = nil
			if !identitySentinels[pid] {
				if model, ok := deviceModelNames[vidPid{vid, pid}]; ok {
					p.DeviceName = ptr(model)
				}
			}
		}
	}
	if v, ok := fields[chargerTLVBrandModel]; ok && v.Tag == 0x04 && len(v.Payload) >= 12 {
		for i := range chargerPortOrder {
			slot := v.Payload[i*4 : i*4+4]
			if allBytes(slot, 0xFF) || allBytes(slot, 0x00) {
				continue
			}
			p := d.port(i)
			if brand, ok := brandCodes[slot[0]]; ok && deref(p.Vendor) == "" {
				p.Vendor = ptr(brand)
			}
		}
	}
}

func allBytes(b []byte, v byte) bool {
	for _, c := range b {
		if c != v {
			return false
		}
	}
	return true
}

// cableProfile looks a four-hex-digit cable code up: the exact table first,
// then the capability by the first byte and the fast-charge protocol by the
// second. ok is false for a code that is not four hex digits or whose
// capability is unknown.
func cableProfile(code string) (label, info string, ok bool) {
	if code == "" {
		return "", "", false
	}
	if e, hit := cableProfiles[code]; hit {
		return e.label, e.info, true
	}
	n := strings.ToUpper(code)
	if len(n) != 4 || strings.Trim(n, "0123456789ABCDEF") != "" {
		return "", "", false
	}
	label, hit := cableCapabilityLabels[n[0:2]]
	if !hit {
		return "", "", false
	}
	return label, chargingInfoLabels[n[2:4]], true
}

// applyCable decodes a cable descriptor: the code is its last two bytes.
func applyCable(p *Port, v ff09.TypedValue) {
	code := ""
	if v.Tag == 0x04 && len(v.Payload) >= 2 {
		n := len(v.Payload)
		code = fmt.Sprintf("%02X%02X", v.Payload[n-2], v.Payload[n-1])
	}
	connected := p.Mode != 0
	label, info, found := cableProfile(code)
	switch {
	case code == "":
		p.Cable = ptr("N/A")
		if connected {
			p.Cable = ptr("Connected")
		}
	case strings.HasPrefix(strings.ToUpper(code), "03"):
		p.Cable = ptr("N/A")
	case found:
		p.Cable = ptr(label)
	default:
		p.Cable = ptr("UNKNOWN (" + code + ")")
	}
	p.ChargingInfo = ptr("N/A")
	if connected && found && info != "" {
		p.ChargingInfo = ptr(info)
	}
}
