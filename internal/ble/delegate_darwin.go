//go:build darwin

package ble

import (
	"fmt"
	"os"

	"github.com/ebitengine/purego/objc"
)

const (
	centralDelegateClass    = "AnkerPrimeBLECentralDelegate"
	peripheralDelegateClass = "AnkerPrimeBLEPeripheralDelegate"
)

// callback wraps every delegate method: CoreBluetooth calls them on its
// queue's thread, where a panic would take the process down, and the
// objects they create are autoreleased.
func callback(self id, fn func(c *Central)) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(os.Stderr, "ble: delegate panic: %v\n", r)
		}
	}()
	registryMu.Lock()
	c := centrals[self]
	registryMu.Unlock()
	if c == nil {
		return
	}
	pool := poolPush()
	defer poolPop(pool)
	fn(c)
}

// peripheralCallback is callback for a connected or connecting peripheral.
func peripheralCallback(self, obj id, fn func(p *Peripheral)) {
	callback(self, func(c *Central) {
		if p := c.peripheral(obj); p != nil {
			fn(p)
		}
	})
}

func registerDelegates() error {
	type cmd = objc.SEL
	central := []objc.MethodDef{
		method("centralManagerDidUpdateState:", func(self id, _ cmd, cm id) {
			callback(self, func(c *Central) { c.didUpdateState(cm) })
		}),
		method("centralManager:didDiscoverPeripheral:advertisementData:RSSI:", func(self id, _ cmd, _, p, adv, rssi id) {
			callback(self, func(c *Central) { c.didDiscover(p, adv, rssi) })
		}),
		method("centralManager:didConnectPeripheral:", func(self id, _ cmd, cm, p id) {
			callback(self, func(c *Central) { c.didConnect(cm, p) })
		}),
		method("centralManager:didFailToConnectPeripheral:error:", func(self id, _ cmd, _, p, err id) {
			callback(self, func(c *Central) { c.didFailToConnect(p, nsErr(err)) })
		}),
		method("centralManager:didDisconnectPeripheral:error:", func(self id, _ cmd, _, p, err id) {
			callback(self, func(c *Central) { c.didDisconnect(p, nsErr(err)) })
		}),
	}
	peripheral := []objc.MethodDef{
		method("peripheral:didDiscoverServices:", func(self id, _ cmd, p, err id) {
			peripheralCallback(self, p, func(pp *Peripheral) { pp.didDiscoverServices(nsErr(err)) })
		}),
		method("peripheral:didDiscoverCharacteristicsForService:error:", func(self id, _ cmd, p, svc, err id) {
			peripheralCallback(self, p, func(pp *Peripheral) { pp.didDiscoverCharacteristics(svc, nsErr(err)) })
		}),
		method("peripheral:didUpdateValueForCharacteristic:error:", func(self id, _ cmd, p, ch, err id) {
			peripheralCallback(self, p, func(pp *Peripheral) { pp.didUpdateValue(ch, nsErr(err)) })
		}),
		method("peripheral:didUpdateNotificationStateForCharacteristic:error:", func(self id, _ cmd, p, ch, err id) {
			peripheralCallback(self, p, func(pp *Peripheral) { pp.didUpdateNotificationState(ch, nsErr(err)) })
		}),
		method("peripheral:didWriteValueForCharacteristic:error:", func(self id, _ cmd, p, ch, err id) {
			peripheralCallback(self, p, func(pp *Peripheral) { pp.didWrite(ch, nsErr(err)) })
		}),
		method("peripheralIsReadyToSendWriteWithoutResponse:", func(self id, _ cmd, p id) {
			peripheralCallback(self, p, func(pp *Peripheral) { pp.readyToWrite() })
		}),
		method("peripheralDidUpdateName:", func(self id, _ cmd, p id) {
			peripheralCallback(self, p, func(pp *Peripheral) { pp.didUpdateName() })
		}),
	}
	if err := classDef(centralDelegateClass, "CBCentralManagerDelegate", central); err != nil {
		return err
	}
	return classDef(peripheralDelegateClass, "CBPeripheralDelegate", peripheral)
}

func classDef(name, protocol string, methods []objc.MethodDef) error {
	var ps []*objc.Protocol
	if p := objc.GetProtocol(protocol); p != nil {
		ps = append(ps, p)
	}
	if _, err := objc.RegisterClass(name, objc.GetClass("NSObject"), ps, nil, methods); err != nil {
		return fmt.Errorf("ble: cannot register %s: %w", name, err)
	}
	return nil
}

func method(selector string, fn any) objc.MethodDef {
	return objc.MethodDef{Cmd: sel(selector), Fn: fn}
}
