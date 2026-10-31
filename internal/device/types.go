// Package device holds the decoded state of the two Anker Prime devices and
// the decoders that fill it from FF09 TLV payloads.
//
// The JSON shape is the cross-language contract: every field and tag matches
// the Python debugger's to_dict() output that spec/fixtures/*.json records,
// so a decoder test can marshal a state and compare it with a fixture frame.
// Nullable fields are pointers.
package device

// Kind names one of the supported devices.
type Kind string

const (
	KindCharger   Kind = "charger"   // Anker Prime 160W charger, A2687
	KindPowerBank Kind = "powerbank" // Anker Prime Power Bank 20K 220W, A110G
)

// Port is one physical port. The charger's ports always point out; the
// power bank's C ports go both ways.
type Port struct {
	Name         string  `json:"name"`
	Mode         int     `json:"mode"`
	Direction    *string `json:"direction"` // "out", "in", or null when idle
	Active       bool    `json:"active"`
	Attached     bool    `json:"attached"`
	Energized    bool    `json:"energized"`
	VoltageV     float64 `json:"voltage_v"`
	CurrentA     float64 `json:"current_a"`
	PowerW       float64 `json:"power_w"` // sticky on the power bank: read it only when Active
	Cable        *string `json:"cable"`
	ChargingInfo *string `json:"charging_info"`
	DeviceName   *string `json:"device_name"`
	Vendor       *string `json:"vendor"`

	// Sampled is set once VoltageV, CurrentA and PowerW have been read from
	// the device. Until then the JSON carries null for the three, as the
	// Python contract does, and the Go fields hold zero.
	Sampled bool `json:"-"`
}

// Identity comes from the 0x0029 handshake reply.
type Identity struct {
	Serial     *string `json:"serial"`
	Firmware   *string `json:"firmware"`
	MACAddress *string `json:"mac_address"`
	Label      *string `json:"label"` // a fixed product label ("Charging"), not a live status
}

// Charger is the A2687's state.
type Charger struct {
	Identity
	Ports             []Port   `json:"ports"`
	ProductCode       *string  `json:"product_code"`
	FirmwareTag       *string  `json:"firmware_tag"`
	ScreenBrightness  *int     `json:"screen_brightness"`
	ScreensaverID     *int     `json:"screensaver_id"` // low 16 bits of the shown cover's id
	ScreensaverFlags  *int     `json:"screensaver_flags"`
	TotalOutputPowerW *float64 `json:"total_output_power_w"`
}

// PowerBank is the A110G's state.
type PowerBank struct {
	Identity
	StateCode            *int     `json:"state_code"`
	ThermalState         *int     `json:"thermal_state"` // 1 normal, 2 too hot to charge
	ThermalLimited       bool     `json:"thermal_limited"`
	BatteryPercent       *float64 `json:"battery_percent"`
	Charging             bool     `json:"charging"`        // see ChargingKnown
	TimeLeftHours        *int     `json:"time_left_hours"` // time to full; 0h00m while discharging
	TimeLeftMinutes      *int     `json:"time_left_minutes"`
	InputPowerW          *float64 `json:"input_power_w"`
	OutputPowerW         *float64 `json:"output_power_w"`
	Ports                []Port   `json:"ports"` // C1, C2, A in that order, only those seen
	Dock                 *Port    `json:"dock"`  // stale once off the dock: trust it only when Active
	Temperature1C        *int     `json:"temperature_1_c"`
	Temperature2C        *int     `json:"temperature_2_c"`
	PomodoroSeconds      *int     `json:"pomodoro_seconds"`
	PomodoroEnabled      *bool    `json:"pomodoro_enabled"`
	BatteryHealthPercent *int     `json:"battery_health_percent"` // sent once per connection, in the snapshot
	ScreenBrightness     *int     `json:"screen_brightness"`

	// ChargingKnown is set after the first telemetry frame. Until then the
	// JSON carries null for charging, as the Python contract does.
	ChargingKnown bool `json:"-"`
}
