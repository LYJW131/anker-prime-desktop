// Package link keeps one device connected: it finds the peripheral, runs the
// FF09 handshake, decodes the stream, re-arms it when it stalls, reconnects,
// and carries the cover commands.
package link

import "time"

// Phase is where a device's connection stands.
type Phase int

const (
	PhaseOff          Phase = iota // the device is turned off in settings
	PhaseBluetoothOff              // the radio is off
	PhaseUnauthorized              // the app has no Bluetooth permission
	PhaseUnpaired                  // no peripheral chosen yet
	PhaseConnecting                // a connect request is pending
	PhaseHandshaking               // connected, setting up the encrypted session
	PhaseConnected                 // streaming
	PhaseDisconnected              // lost, a retry is scheduled
	PhaseSleeping                  // the power bank was idle, released so it can sleep
)

// StaleAfter is how long a connected device may go without a frame before
// its numbers count as out of date.
const StaleAfter = 15 * time.Second

// Status is a device's connection, as the views show it.
type Status struct {
	Phase     Phase
	Err       string    // the last failure, cleared on connect
	LastFrame time.Time // any decoded frame
	Name      string    // advertised name, which is the serial number
}

// Stale reports whether a connected device's numbers are out of date.
func (s Status) Stale(now time.Time) bool {
	return s.Phase == PhaseConnected && !s.LastFrame.IsZero() && now.Sub(s.LastFrame) > StaleAfter
}

// Found is a peripheral seen while pairing.
type Found struct {
	ID   string // CoreBluetooth identifier, per Mac
	Name string
	RSSI int
}

// CoverJob is a cover being selected or pushed to the charger.
type CoverJob struct {
	ID    int // the picture id
	Done  int // chunks sent
	Total int // chunks in all; 0 while only selecting
	Err   string
}
