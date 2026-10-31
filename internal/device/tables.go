package device

// Tables and constants of the charger decoder. They are copies of the values
// in debugger/anker_prime_ble/charger.py; spec/protocol.json pins them and
// tables_test.go fails when they drift.

// Commands whose payload is a snapshot or a realtime push. The charger runs
// the realtime decoder on snapshots too, since without an account id the
// snapshot is its only data.
var (
	chargerSnapshotCommands = []uint16{0x0200, 0x0A00, 0x0040, 0x0405}
	chargerRealtimeCommands = []uint16{0x020A, 0x0207, 0x0206, 0x4300, 0x0300, 0x0303, 0x0410}
)

const (
	chargerTLVIdentity   = 0xB4 // 3 x (VID u16le, PID u16le)
	chargerTLVBrandModel = 0xB5 // 3 x 4 bytes of brand/model codes
	chargerTLVScreensave = 0xE1
	chargerTLVBrightness = 0xA9 // snapshot only
)

var chargerPortOrder = [3]string{"C1", "C2", "C3"}

// The port-struct and cable-descriptor TLVs of C1, C2 and C3, in port order.
// The order matters: the total is summed in it.
var (
	chargerPortStruct = [3]byte{0xA5, 0xA6, 0xA7}
	chargerPortCable  = [3]byte{0xAC, 0xAD, 0xAE}
)

var cableProfiles = map[string]cableProfileEntry{
	"0100": {"5A-100W MAX", ""},
	"0200": {"EPR-240W MAX", ""},
	"0201": {"EPR-240W MAX", "Apple PD Fast Charging"},
}

type cableProfileEntry struct{ label, info string }

var cableCapabilityLabels = map[string]string{
	"00": "3A-60W MAX",
	"01": "5A-100W MAX",
	"02": "EPR-240W MAX",
}

var chargingInfoLabels = map[string]string{
	"01": "Apple PD Fast Charging",
	"02": "Samsung Fast Charging",
	"03": "Samsung Super Fast Charging",
}

var identitySentinels = map[uint16]bool{
	0x0000: true, 0xFFFA: true, 0xFFFB: true, 0xFFFC: true, 0xFFFD: true, 0xFFFE: true, 0xFFFF: true,
}

var usbVendorNames = map[uint16]string{
	0x05AC: "Apple",
	0x04E8: "Samsung",
	0x2717: "Xiaomi",
	0x12D1: "Huawei",
	0x18D1: "Google",
	0x1004: "LG",
	0x0451: "Texas Instruments",
	0x291A: "Anker",
	0x03F0: "HP",
	0x413C: "Dell",
	0x17EF: "Lenovo",
	0x045E: "Microsoft",
	0x0B05: "ASUS",
	0x0DB0: "MSI",
	0x1532: "Razer",
}

type vidPid struct{ vid, pid uint16 }

var deviceModelNames = map[vidPid]string{
	{0x05AC, 0x7519}: "iPhone 17 series",
	{0x05AC, 0x7319}: "MacBook Pro series",
	{0x05AC, 0x7117}: "iPad Pro series",
	{0x291A, 0x110B}: "20K Prime Power Bank",
}

var brandCodes = map[byte]string{
	0x01: "Apple", 0x02: "Samsung", 0x03: "Xiaomi", 0x04: "Huawei", 0x05: "Google",
	0x06: "LG", 0x07: "IDT", 0x08: "TI", 0x09: "YBZ", 0x0A: "Anker", 0x0B: "Honor",
	0x0C: "HP", 0x0D: "Dell", 0x0E: "Lenovo", 0x0F: "Microsoft", 0x10: "ASUS",
	0x11: "ASUS", 0x12: "MSI", 0x13: "Razer",
}

// Power bank TLVs of the 0x0300 push; the 0x0200 snapshot carries the same
// block shifted by pbSnapshotShift.
const (
	pbTLVState      = 0xA1 // always 0xA1 01 31: tag 0x31, no payload, so state_code stays null
	pbTLVBattery    = 0xA2 // decimal pair
	pbTLVTimeLeft   = 0xA3 // [flag, hours, minutes]
	pbTLVThermal    = 0xA4 // 1 normal, 2 too hot to charge
	pbTLVInput      = 0xA5 // [flag, watts x10], a true sum
	pbTLVOutput     = 0xA6 // [flag, watts x10], a true sum
	pbTLVDock       = 0xA7
	pbTLVPortC1     = 0xA8
	pbTLVPortC2     = 0xA9
	pbTLVPortA      = 0xAC
	pbTLVTemp1      = 0xAF
	pbTLVTemp2      = 0xB0
	pbSnapshotShift = 0x0D

	pbSnapState      = 0xA1
	pbSnapBattery    = 0xA6
	pbSnapTimeLeft   = 0xA7
	pbSnapPomodoro   = 0xAC // typed u16 seconds
	pbSnapPomoEnable = 0xE2 // [enabled, seconds u16le]
	pbSnapHealth     = 0xA9 // typed u8 percent
	pbSnapBrightness = 0xAA // typed u8 percent
	pbSnapShiftFirst = 0xB2
	pbSnapShiftLast  = 0xBE
)

var (
	pbSnapshotCommands = []uint16{0x0200}
	pbRealtimeCommands = []uint16{0x0300}
	pbDirection        = map[byte]string{1: "out", 2: "in"}
)
