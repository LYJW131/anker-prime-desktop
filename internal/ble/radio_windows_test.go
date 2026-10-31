//go:build windows

package ble

import (
	"errors"
	"reflect"
	"testing"
	"time"
	"unsafe"

	"tinygo.org/x/bluetooth"
)

// connected returns a Device with its own link state, as Connect makes.
func connected(t *testing.T) bluetooth.Device {
	t.Helper()
	var d bluetooth.Device
	f := reflect.ValueOf(&d).Elem().FieldByName("link")
	if f.Kind() != reflect.Pointer {
		t.Fatalf("bluetooth.Device.link is %v, want a pointer", f.Kind())
	}
	reflect.NewAt(f.Type(), unsafe.Pointer(f.UnsafeAddr())).Elem().Set(reflect.New(f.Type().Elem()))
	return d
}

func TestSameDevice(t *testing.T) {
	a, b := connected(t), connected(t)
	if !sameDevice(a, a) {
		t.Error("a Device is not itself")
	}
	if sameDevice(a, b) {
		t.Error("two Connects are one Device")
	}
	if !sameDevice(a, bluetooth.Device{}) {
		t.Error("an unreadable Device does not fall back to the MAC")
	}
}

func TestConnectionChangedIgnoresStaleDevice(t *testing.T) {
	r := &winRadio{conns: map[bluetooth.MAC]*winConn{}}
	mac := bluetooth.MAC{1, 2, 3, 4, 5, 6}
	old, cur := connected(t), connected(t)
	old.Address.MAC, cur.Address.MAC = mac, mac
	lost := 0
	r.conns[mac] = &winConn{r: r, d: cur, mac: mac, lost: func(error) { lost++ }}

	r.connectionChanged(old, false)
	if lost != 0 || r.conns[mac] == nil {
		t.Fatal("a stale Device's disconnect tore down the newer connection")
	}
	r.connectionChanged(cur, false)
	if lost != 1 || r.conns[mac] != nil {
		t.Fatalf("lost %d times, conns %v; want the connection dropped once", lost, r.conns)
	}
}

func TestStopScan(t *testing.T) {
	r := &winRadio{adapter: &bluetooth.Adapter{}}
	done := make(chan error, 1)
	want := errors.New("stopped")
	done <- want
	if err := r.stopScan(done, time.Second); err != want {
		t.Errorf("stopScan = %v, want %v", err, want)
	}
	if err := r.stopScan(make(chan error), 50*time.Millisecond); !errors.Is(err, errScanStuck) {
		t.Errorf("stopScan = %v, want errScanStuck", err)
	}
}
