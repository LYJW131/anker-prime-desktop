//go:build darwin

package ble

import "testing"

// TestLoad registers the delegate classes and runs a function on a private
// queue. It loads CoreBluetooth but never creates a manager, so it needs no
// Bluetooth permission.
func TestLoad(t *testing.T) {
	if err := load(); err != nil {
		t.Fatal(err)
	}
	for cls, s := range map[string]string{
		centralDelegateClass:    "centralManager:didDiscoverPeripheral:advertisementData:RSSI:",
		peripheralDelegateClass: "peripheral:didUpdateValueForCharacteristic:error:",
	} {
		if !sendBool(class(cls), "instancesRespondToSelector:", uintptr(sel(s))) {
			t.Errorf("%s does not answer %s", cls, s)
		}
	}
	label := []byte("anker-prime.ble.test\x00")
	q := dispatchQueueCreate(&label[0], 0)
	ran := false
	onQueue(q, func() { ran = goString(nsString("ok")) == "ok" })
	if !ran {
		t.Fatal("onQueue did not run")
	}
	defer func() {
		if r := recover(); r != "boom" {
			t.Fatalf("panic not re-raised: %v", r)
		}
	}()
	onQueue(q, func() { panic("boom") })
}
