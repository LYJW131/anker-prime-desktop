package device

import (
	"encoding/binary"
	"reflect"

	"github.com/LYJW131/anker-prime-mac/internal/ff09"
)

// PowerBankDecoder fills a PowerBank from decrypted FF09 payloads. It ports
// powerbank.py; spec/fixtures replay through it frame by frame. Not safe for
// concurrent use: the link owns it and hands copies (Snapshot) to the UI.
type PowerBankDecoder struct {
	State PowerBank
}

// NewPowerBankDecoder returns a decoder with an empty state.
func NewPowerBankDecoder() *PowerBankDecoder { return &PowerBankDecoder{State: NewPowerBank()} }

// Reset forgets everything read so far, as on a new connection.
func (d *PowerBankDecoder) Reset() { d.State = NewPowerBank() }

// Snapshot returns a deep copy of the state.
func (d *PowerBankDecoder) Snapshot() PowerBank { return d.State.Clone() }

// Handles reports whether Apply decodes the command: the 0x0029 identity reply,
// the 0x0200 snapshot and the 0x0300 push.
func (d *PowerBankDecoder) Handles(command uint16) bool {
	return command == ff09.CmdIdentity || contains(pbSnapshotCommands, command) ||
		contains(pbRealtimeCommands, command)
}

// Apply decodes one frame's plaintext into the state and reports whether any
// field changed. A command that is not Handled leaves the state alone and
// reports false; a handled frame that carries nothing new reports false too.
func (d *PowerBankDecoder) Apply(command uint16, plain []byte) (changed bool) {
	if !d.Handles(command) {
		return false
	}
	if d.State.Ports == nil {
		d.State.Ports = []Port{} // a zero-value decoder: Clone makes the same, so DeepEqual stays honest
	}
	before := d.State.Clone()
	switch {
	case command == ff09.CmdIdentity:
		applyIdentity(&d.State.Identity, plain, false)
	case contains(pbSnapshotCommands, command):
		d.applySnapshot(plain)
	default:
		d.applyRealtime(plain)
	}
	return !reflect.DeepEqual(before, d.State)
}

// pbPort decodes a port block: [mode, V, A, W] and, on the C ports, a tail.
// The W slot is sticky: an idle port keeps the last power it carried, so it
// means nothing unless the mode byte is non-zero.
func pbPort(name string, b []byte) Port {
	p := Port{Name: name}
	if len(b) >= 7 {
		p.Mode = int(b[0])
		if dir, ok := pbDirection[b[0]]; ok {
			p.Direction = ptr(dir)
		}
		p.VoltageV = float64(binary.LittleEndian.Uint16(b[1:])) / 10
		p.CurrentA = float64(binary.LittleEndian.Uint16(b[3:])) / 10
		p.PowerW = float64(binary.LittleEndian.Uint16(b[5:])) / 10
		p.Sampled = true
		if len(b) >= 9 {
			// [7] is the source role; [8] is 0x07 with a cable in the port,
			// which separates "attached, nothing negotiated" from "empty".
			p.Attached = b[8] == 0x07
		}
	}
	p.refresh()
	return p
}

// applyRealtime decodes a 0x0300 payload and leaves untouched fields alone.
func (d *PowerBankDecoder) applyRealtime(plain []byte) {
	s := &d.State
	fields := map[byte][]byte{}
	for _, t := range ff09.ParseTLV(plain, ff09.TLVOffset(plain)) {
		fields[t.Type] = ff09.ReadTypedValue(t.Value).Payload
	}

	if b := fields[pbTLVState]; len(b) > 0 {
		s.StateCode = ptr(int(b[0]))
	}
	if b := fields[pbTLVThermal]; len(b) > 0 {
		s.ThermalState = ptr(int(b[0]))
	}
	if b, ok := fields[pbTLVBattery]; ok {
		s.BatteryPercent = decimalPair(b)
	}
	if b := fields[pbTLVTimeLeft]; len(b) >= 3 {
		s.TimeLeftHours, s.TimeLeftMinutes = ptr(int(b[1])), ptr(int(b[2]))
	}
	if b, ok := fields[pbTLVInput]; ok {
		s.InputPowerW = tenths(b, 1)
	}
	if b, ok := fields[pbTLVOutput]; ok {
		s.OutputPowerW = tenths(b, 1)
	}
	if b, ok := fields[pbTLVDock]; ok {
		dock := pbPort("DOCK", b)
		s.Dock = &dock
	}

	// Direction comes from the total input, which matched the active port in
	// every capture. The thermal byte is not directional and the dock goes stale.
	in := 0.0
	if s.InputPowerW != nil {
		in = *s.InputPowerW
	}
	s.Charging, s.ChargingKnown = in > 0.05, true

	var ports []Port
	for _, pt := range []struct {
		name string
		typ  byte
	}{{"C1", pbTLVPortC1}, {"C2", pbTLVPortC2}, {"A", pbTLVPortA}} {
		if b, ok := fields[pt.typ]; ok {
			ports = append(ports, pbPort(pt.name, b))
		}
	}
	if len(ports) > 0 {
		s.Ports = ports
	}

	if b := fields[pbTLVTemp1]; len(b) > 0 {
		s.Temperature1C = ptr(int(b[0]))
	}
	if b := fields[pbTLVTemp2]; len(b) > 0 {
		s.Temperature2C = ptr(int(b[0]))
	}
	s.ThermalLimited = s.ThermalState != nil && *s.ThermalState == 2
}

// applySnapshot decodes a 0x0200 snapshot. Its realtime block sits at a +0x0D
// TLV offset, with three exceptions (0xA1 state, 0xA6 battery, 0xA7 time left),
// so it is shifted back into a 0x0300-shaped payload and run through
// applyRealtime. The settings only the snapshot has are read on the way.
func (d *PowerBankDecoder) applySnapshot(plain []byte) {
	s := &d.State
	var shifted []byte
	for _, t := range ff09.ParseTLV(plain, ff09.TLVOffset(plain)) {
		var mapped byte
		switch {
		case t.Type == pbSnapBattery:
			mapped = pbTLVBattery
		case t.Type == pbSnapTimeLeft:
			mapped = pbTLVTimeLeft
		case t.Type == pbSnapState:
			mapped = pbTLVState
		case t.Type == pbSnapPomoEnable:
			if p := ff09.ReadTypedValue(t.Value).Payload; len(p) >= 3 {
				s.PomodoroEnabled = ptr(p[0] != 0)
				s.PomodoroSeconds = ptr(int(binary.LittleEndian.Uint16(p[1:])))
			}
			continue
		case t.Type == pbSnapHealth:
			if v := ff09.ReadTypedValue(t.Value); v.HasInt {
				s.BatteryHealthPercent = intPtr(v.U)
			}
			continue
		case t.Type == pbSnapBrightness:
			if v := ff09.ReadTypedValue(t.Value); v.HasInt {
				s.ScreenBrightness = intPtr(v.U)
			}
			continue
		case t.Type == pbSnapPomodoro:
			if v := ff09.ReadTypedValue(t.Value); v.HasInt {
				s.PomodoroSeconds = intPtr(v.U)
			}
			continue
		case t.Type >= pbSnapShiftFirst && t.Type <= pbSnapShiftLast:
			mapped = t.Type - pbSnapshotShift
		default:
			continue // the counters and flags nobody has decoded
		}
		shifted = append(shifted, mapped, byte(len(t.Value)))
		shifted = append(shifted, t.Value...)
	}
	d.applyRealtime(shifted)
}
