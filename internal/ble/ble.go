// Package ble is a small Bluetooth LE central, without cgo: CoreBluetooth
// through the Objective-C runtime (purego) on macOS, WinRT through
// tinygo.org/x/bluetooth on Windows, BlueZ over D-Bus on Linux.
//
// Nothing touches the system's Bluetooth stack until NewCentral is called,
// so importing the package, and testing it, never creates a manager or
// raises the Bluetooth permission prompt. The stack's callbacks copy what
// they need into Go values, hand it over without blocking and return. Scan
// results and notifications reach the caller in order on a goroutine of
// their own.
//
// A peripheral's ID is its CoreBluetooth identifier on macOS and its
// address ("AA:BB:CC:DD:EE:FF") on Windows and Linux.
//
// Characteristic and service UUIDs are strings in any form
// ("FF09", "0000ff09-0000-1000-8000-00805f9b34fb", any case); the package
// reports them lowercase in the full 128-bit form.
//
// On other systems every call returns errors.ErrUnsupported.
package ble

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

// WriteTimeout bounds Peripheral.Write: the wait for a response, or for
// room to send without one.
const WriteTimeout = 10 * time.Second

// State is the radio's state, CBManagerState.
type State int

const (
	StateUnknown      State = iota // not reported yet
	StateResetting                 // the connection with the system service was lost
	StateUnsupported               // this Mac has no Bluetooth LE
	StateUnauthorized              // the app may not use Bluetooth
	StatePoweredOff                // Bluetooth is off
	StatePoweredOn                 // ready
)

func (s State) String() string {
	switch s {
	case StateUnknown:
		return "unknown"
	case StateResetting:
		return "resetting"
	case StateUnsupported:
		return "unsupported"
	case StateUnauthorized:
		return "unauthorized"
	case StatePoweredOff:
		return "poweredOff"
	case StatePoweredOn:
		return "poweredOn"
	}
	return fmt.Sprintf("State(%d)", int(s))
}

// Authorization is the app's Bluetooth permission, CBManagerAuthorization.
type Authorization int

const (
	AuthNotDetermined Authorization = iota // the user has not been asked
	AuthRestricted                         // a profile or parental control forbids it
	AuthDenied                             // the user said no
	AuthAllowed                            // allowed always
)

func (a Authorization) String() string {
	switch a {
	case AuthNotDetermined:
		return "notDetermined"
	case AuthRestricted:
		return "restricted"
	case AuthDenied:
		return "denied"
	case AuthAllowed:
		return "allowed"
	}
	return fmt.Sprintf("Authorization(%d)", int(a))
}

// Advert is one advertisement seen while scanning.
type Advert struct {
	ID               string            // CoreBluetooth identifier, stable per Mac; the address elsewhere
	Name             string            // peripheral name, else the advertised local name
	RSSI             int               // dBm; 127 when unavailable
	Connectable      bool              // advertised as connectable
	ServiceUUIDs     []string          // advertised services, normalized
	ManufacturerData map[uint16][]byte // by company id; nil when absent
	ServiceData      map[string][]byte // by normalized service UUID; nil when absent
}

// Device is a peripheral the system knows, without an advertisement.
type Device struct {
	ID   string
	Name string
}

// Property is a characteristic's properties, CBCharacteristicProperties.
type Property uint

const (
	PropBroadcast            Property = 0x01
	PropRead                 Property = 0x02
	PropWriteWithoutResponse Property = 0x04
	PropWrite                Property = 0x08
	PropNotify               Property = 0x10
	PropIndicate             Property = 0x20
)

func (p Property) String() string {
	names := []struct {
		p Property
		s string
	}{
		{PropBroadcast, "broadcast"}, {PropRead, "read"},
		{PropWriteWithoutResponse, "write-without-response"}, {PropWrite, "write"},
		{PropNotify, "notify"}, {PropIndicate, "indicate"},
	}
	var out []string
	for _, n := range names {
		if p&n.p != 0 {
			out = append(out, n.s)
		}
	}
	return strings.Join(out, ",")
}

// Characteristic is a discovered characteristic.
type Characteristic struct {
	UUID       string
	Service    string
	Properties Property
}

var (
	// ErrNotPoweredOn: the radio left the poweredOn state while waiting,
	// or a call needs it on.
	ErrNotPoweredOn = errors.New("ble: bluetooth is not powered on")
	// ErrUnauthorized: the app has no Bluetooth permission.
	ErrUnauthorized = errors.New("ble: bluetooth is not authorized")
	// ErrUnknownPeripheral: the identifier was neither seen in a scan nor
	// known to the system; scan for it.
	ErrUnknownPeripheral = errors.New("ble: unknown peripheral")
	// ErrBusy: a connection to this peripheral is already pending or open.
	ErrBusy = errors.New("ble: peripheral already connecting or connected")
	// ErrNotFound: a service or characteristic was not discovered.
	ErrNotFound = errors.New("ble: not discovered")
	// ErrTooLong: a write exceeds MaxWriteLen.
	ErrTooLong = errors.New("ble: write longer than the maximum length")
	// ErrTimeout: a write got no answer within WriteTimeout.
	ErrTimeout = errors.New("ble: timed out")
	// ErrDisconnected: the peripheral disconnected; Err wraps the reason.
	ErrDisconnected = errors.New("ble: disconnected")
	// ErrClosed: the peripheral or central was closed by this process.
	ErrClosed = errors.New("ble: closed")
)

// NormalizeUUID returns u lowercase in the full 128-bit form, expanding
// 16- and 32-bit UUIDs onto the Bluetooth base UUID.
func NormalizeUUID(u string) string {
	u = strings.ToLower(strings.TrimSpace(u))
	u = strings.TrimPrefix(u, "0x")
	switch len(u) {
	case 4:
		return "0000" + u + baseSuffix
	case 8:
		return u + baseSuffix
	}
	return u
}

const baseSuffix = "-0000-1000-8000-00805f9b34fb"

// shortUUID returns the form handed to CBUUID UUIDWithString:, which keeps
// UUIDs on the base UUID short.
func shortUUID(u string) string {
	n := NormalizeUUID(u)
	if len(n) == 36 && strings.HasSuffix(n, baseSuffix) {
		if strings.HasPrefix(n, "0000") {
			return strings.ToUpper(n[4:8])
		}
		return strings.ToUpper(n[:8])
	}
	return strings.ToUpper(n)
}

// splitManufacturerData splits CoreBluetooth's manufacturer data, a little
// endian company id followed by the payload.
func splitManufacturerData(b []byte) (uint16, []byte, bool) {
	if len(b) < 2 {
		return 0, nil, false
	}
	return uint16(b[0]) | uint16(b[1])<<8, append([]byte(nil), b[2:]...), true
}

// fifo is an unbounded queue with a single consumer: producers on the
// CoreBluetooth queue never block, and nothing is dropped, which would break
// frames that span notifications.
type fifo[T any] struct {
	mu     sync.Mutex
	items  []T
	closed bool
	wake   chan struct{}
}

func newFIFO[T any]() *fifo[T] { return &fifo[T]{wake: make(chan struct{}, 1)} }

// push appends v; it never blocks. Pushes after close are dropped.
func (q *fifo[T]) push(v T) {
	q.mu.Lock()
	if q.closed {
		q.mu.Unlock()
		return
	}
	q.items = append(q.items, v)
	q.mu.Unlock()
	select {
	case q.wake <- struct{}{}:
	default:
	}
}

// close lets run return once the queued items are delivered.
func (q *fifo[T]) close() {
	q.mu.Lock()
	q.closed = true
	q.mu.Unlock()
	select {
	case q.wake <- struct{}{}:
	default:
	}
}

// run delivers items in order until the queue is closed and drained.
func (q *fifo[T]) run(fn func(T)) {
	for {
		q.mu.Lock()
		items := q.items
		q.items = nil
		closed := q.closed
		q.mu.Unlock()
		for _, v := range items {
			fn(v)
		}
		if len(items) == 0 {
			if closed {
				return
			}
			<-q.wake
		}
	}
}
