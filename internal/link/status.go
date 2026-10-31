// Package link keeps one device connected: it finds the peripheral, runs the
// FF09 handshake, decodes the stream, re-arms it when it stalls, reconnects,
// and carries the cover commands.
package link

import (
	"errors"
	"fmt"
	"time"
)

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
	Err       error     // the last failure, cleared on connect; see the errors below
	LastFrame time.Time // any decoded frame
	Name      string    // advertised name, which is the serial number
}

// Stale reports whether a connected device's numbers are out of date.
func (s Status) Stale(now time.Time) bool {
	return s.Phase == PhaseConnected && !s.LastFrame.IsZero() && now.Sub(s.LastFrame) > StaleAfter
}

// Found is an Anker device seen while adding one.
type Found struct {
	ID        string // CoreBluetooth identifier, per Mac
	Name      string
	RSSI      int
	Model     string // a model.ID; "" when the model is not supported yet
	ModelByte int    // the advertised model byte, -1 when the advert has none
	Added     bool   // already in the device list
}

// CoverJob is a cover being selected or pushed to the charger.
type CoverJob struct {
	ID    int // the picture id
	Done  int // chunks sent
	Total int // chunks in all; 0 while only selecting
	Err   error
}

// Errors a Status or a cover job can carry. The views say them in the
// app's language; any other error shows its own text.
var (
	ErrManual           = errors.New("disconnected by the user")
	ErrNoUserID         = errors.New("no Anker account ID")
	ErrStalled          = errors.New("the stream stalled")
	ErrNoResponse       = errors.New("the device did not answer")
	ErrNotConnected     = errors.New("the device is not connected")
	ErrCoverNoData      = errors.New("the charger lacks the picture and there are no bytes to push")
	ErrCoverNotSwitched = errors.New("the pixels arrived but the screen did not switch")
)

// HandshakeError is a failed session setup.
type HandshakeError struct{ Err error }

func (e *HandshakeError) Error() string { return "handshake: " + e.Err.Error() }
func (e *HandshakeError) Unwrap() error { return e.Err }

// CoverRejectedError is a cover command the charger refused.
type CoverRejectedError struct {
	Command uint16
	Result  byte
}

func (e *CoverRejectedError) Error() string {
	return fmt.Sprintf("the charger refused 0x%04X with result 0x%02X", e.Command, e.Result)
}
