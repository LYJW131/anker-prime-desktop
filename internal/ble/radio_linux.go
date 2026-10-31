//go:build linux

package ble

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/godbus/dbus/v5"
)

// BlueZ's D-Bus API: https://git.kernel.org/pub/scm/bluetooth/bluez.git/tree/doc
const (
	bluez        = "org.bluez"
	ifAdapter    = "org.bluez.Adapter1"
	ifDevice     = "org.bluez.Device1"
	ifService    = "org.bluez.GattService1"
	ifChar       = "org.bluez.GattCharacteristic1"
	ifProperties = "org.freedesktop.DBus.Properties"
	ifObjects    = "org.freedesktop.DBus.ObjectManager"
)

// bluezRadio talks to BlueZ over its own system bus connection, whose
// signals arrive in order on one goroutine (dbus's default handler may
// reorder them, which would break frames that span notifications).
type bluezRadio struct {
	bus *dbus.Conn
	set func(State)

	mu       sync.Mutex
	adapter  dbus.ObjectPath // "" while there is none
	powered  bool
	devices  map[dbus.ObjectPath]map[string]dbus.Variant
	report   func(Advert)  // while scanning
	scanEnd  chan struct{} // closed when the adapter stops discovering
	conns    map[dbus.ObjectPath]*bluezConn
	handlers map[dbus.ObjectPath]func([]byte) // by characteristic
}

func newRadio() radio {
	return &bluezRadio{
		devices:  map[dbus.ObjectPath]map[string]dbus.Variant{},
		conns:    map[dbus.ObjectPath]*bluezConn{},
		handlers: map[dbus.ObjectPath]func([]byte){},
	}
}

func (r *bluezRadio) open(set func(State)) error {
	bus, err := dbus.ConnectSystemBus(dbus.WithSignalHandler(dbus.NewSequentialSignalHandler()))
	if err != nil {
		return fmt.Errorf("ble: system bus: %w", err)
	}
	r.bus, r.set = bus, set
	for _, m := range [][]dbus.MatchOption{
		{dbus.WithMatchSender(bluez), dbus.WithMatchInterface(ifProperties), dbus.WithMatchMember("PropertiesChanged")},
		{dbus.WithMatchSender(bluez), dbus.WithMatchInterface(ifObjects)},
	} {
		if err := bus.AddMatchSignal(m...); err != nil {
			bus.Close()
			return fmt.Errorf("ble: bluez signals: %w", err)
		}
	}
	signals := make(chan *dbus.Signal, 64)
	bus.Signal(signals)
	go r.dispatch(signals)

	objects, err := r.objects(context.Background())
	if err != nil {
		var de dbus.Error
		switch {
		case errors.As(err, &de) && de.Name == "org.freedesktop.DBus.Error.AccessDenied":
			set(StateUnauthorized)
		default:
			set(StateUnsupported) // bluetoothd is not running
		}
		return nil
	}
	r.mu.Lock()
	paths := make([]string, 0, len(objects))
	for p := range objects {
		paths = append(paths, string(p))
	}
	sort.Strings(paths)
	for _, p := range paths {
		if props, ok := objects[dbus.ObjectPath(p)][ifAdapter]; ok && r.adapter == "" {
			r.adapter = dbus.ObjectPath(p)
			r.powered, _ = props["Powered"].Value().(bool)
		}
	}
	r.mu.Unlock()
	r.publish()
	return nil
}

func (r *bluezRadio) objects(ctx context.Context) (map[dbus.ObjectPath]map[string]map[string]dbus.Variant, error) {
	var out map[dbus.ObjectPath]map[string]map[string]dbus.Variant
	err := r.bus.Object(bluez, "/").CallWithContext(ctx, ifObjects+".GetManagedObjects", 0).Store(&out)
	return out, err
}

// publish reports the adapter's state; it never blocks.
func (r *bluezRadio) publish() {
	r.mu.Lock()
	s := StateUnsupported
	if r.adapter != "" {
		s = StatePoweredOff
		if r.powered {
			s = StatePoweredOn
		}
	}
	r.mu.Unlock()
	r.set(s)
}

func (r *bluezRadio) dispatch(signals <-chan *dbus.Signal) {
	for sig := range signals {
		switch sig.Name {
		case ifProperties + ".PropertiesChanged":
			iface, _ := sig.Body[0].(string)
			changed, _ := sig.Body[1].(map[string]dbus.Variant)
			r.propertiesChanged(sig.Path, iface, changed)
		case ifObjects + ".InterfacesAdded":
			path, _ := sig.Body[0].(dbus.ObjectPath)
			ifaces, _ := sig.Body[1].(map[string]map[string]dbus.Variant)
			r.interfacesAdded(path, ifaces)
		case ifObjects + ".InterfacesRemoved":
			path, _ := sig.Body[0].(dbus.ObjectPath)
			ifaces, _ := sig.Body[1].([]string)
			r.interfacesRemoved(path, ifaces)
		}
	}
}

func (r *bluezRadio) propertiesChanged(path dbus.ObjectPath, iface string, changed map[string]dbus.Variant) {
	switch iface {
	case ifChar:
		v, ok := changed["Value"].Value().([]byte)
		r.mu.Lock()
		h := r.handlers[path]
		r.mu.Unlock()
		if ok && h != nil {
			h(v)
		}
	case ifAdapter:
		p, ok := changed["Powered"].Value().(bool)
		r.mu.Lock()
		mine := path == r.adapter
		if mine && ok {
			r.powered = p
		}
		end := r.scanEnd
		if d, has := changed["Discovering"].Value().(bool); mine && has && !d && end != nil {
			r.scanEnd = nil
			close(end)
		}
		r.mu.Unlock()
		if mine && ok {
			r.publish()
		}
	case ifDevice:
		r.mu.Lock()
		props := r.devices[path]
		if props == nil {
			props = map[string]dbus.Variant{}
			r.devices[path] = props
		}
		for k, v := range changed {
			props[k] = v
		}
		report := r.report
		c := r.conns[path]
		r.mu.Unlock()
		if report != nil && r.underAdapter(path) && seen(changed) {
			report(advertFromBlueZ(props))
		}
		if c != nil {
			if v, ok := changed["Connected"].Value().(bool); ok && !v {
				c.drop(nil)
			}
			if v, ok := changed["ServicesResolved"].Value().(bool); ok && v {
				c.markResolved()
			}
		}
	}
}

// seen reports whether a device's property change comes from an
// advertisement received now.
func seen(changed map[string]dbus.Variant) bool {
	for _, k := range []string{"RSSI", "ManufacturerData", "ServiceData", "Name", "UUIDs"} {
		if _, ok := changed[k]; ok {
			return true
		}
	}
	return false
}

func (r *bluezRadio) underAdapter(path dbus.ObjectPath) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.adapter != "" && strings.HasPrefix(string(path), string(r.adapter)+"/")
}

func (r *bluezRadio) interfacesAdded(path dbus.ObjectPath, ifaces map[string]map[string]dbus.Variant) {
	if props, ok := ifaces[ifAdapter]; ok {
		r.mu.Lock()
		adopt := r.adapter == ""
		if adopt {
			r.adapter = path
			r.powered, _ = props["Powered"].Value().(bool)
		}
		r.mu.Unlock()
		if adopt {
			r.publish()
		}
	}
	if props, ok := ifaces[ifDevice]; ok {
		r.mu.Lock()
		r.devices[path] = props
		report := r.report
		r.mu.Unlock()
		if report != nil && r.underAdapter(path) {
			report(advertFromBlueZ(props))
		}
	}
}

func (r *bluezRadio) interfacesRemoved(path dbus.ObjectPath, ifaces []string) {
	for _, iface := range ifaces {
		switch iface {
		case ifAdapter:
			r.mu.Lock()
			gone := path == r.adapter
			if gone {
				r.adapter, r.powered = "", false
			}
			r.mu.Unlock()
			if gone {
				r.publish()
			}
		case ifDevice:
			r.mu.Lock()
			delete(r.devices, path)
			c := r.conns[path]
			r.mu.Unlock()
			if c != nil {
				c.drop(errors.New("the system removed the device"))
			}
		}
	}
}

// advertFromBlueZ converts a Device1's properties.
func advertFromBlueZ(props map[string]dbus.Variant) Advert {
	id, _ := props["Address"].Value().(string)
	if a, err := parseAddress(id); err == nil {
		id = formatAddress(a)
	}
	name, _ := props["Name"].Value().(string)
	rssi := 127
	if v, ok := props["RSSI"].Value().(int16); ok {
		rssi = int(v)
	}
	services, _ := props["UUIDs"].Value().([]string)
	var mfr map[uint16][]byte
	if m, ok := props["ManufacturerData"].Value().(map[uint16]dbus.Variant); ok {
		mfr = map[uint16][]byte{}
		for k, v := range m {
			if b, ok := v.Value().([]byte); ok {
				mfr[k] = b
			}
		}
	}
	var svcData map[string][]byte
	if m, ok := props["ServiceData"].Value().(map[string]dbus.Variant); ok {
		svcData = map[string][]byte{}
		for k, v := range m {
			if b, ok := v.Value().([]byte); ok {
				svcData[k] = b
			}
		}
	}
	return newAdvert(id, name, rssi, services, mfr, svcData)
}

func (r *bluezRadio) scan(stop <-chan struct{}, report func(Advert)) error {
	r.mu.Lock()
	adapter := r.adapter
	r.mu.Unlock()
	if adapter == "" {
		return ErrNotPoweredOn
	}
	ctx := context.Background()
	if objects, err := r.objects(ctx); err == nil {
		r.mu.Lock()
		for p, ifaces := range objects {
			if props, ok := ifaces[ifDevice]; ok {
				r.devices[p] = props
			}
		}
		r.mu.Unlock()
	}
	obj := r.bus.Object(bluez, adapter)
	filter := map[string]dbus.Variant{
		"Transport":     dbus.MakeVariant("le"),
		"DuplicateData": dbus.MakeVariant(true),
	}
	if err := obj.CallWithContext(ctx, ifAdapter+".SetDiscoveryFilter", 0, filter).Err; err != nil {
		return err
	}
	end := make(chan struct{})
	r.mu.Lock()
	r.report, r.scanEnd = report, end
	r.mu.Unlock()
	defer func() {
		r.mu.Lock()
		r.report = nil
		if r.scanEnd == end {
			r.scanEnd = nil
		}
		r.mu.Unlock()
	}()
	if err := obj.CallWithContext(ctx, ifAdapter+".StartDiscovery", 0).Err; err != nil {
		return err
	}
	select {
	case <-stop:
		sctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		return obj.CallWithContext(sctx, ifAdapter+".StopDiscovery", 0).Err
	case <-end:
		return errors.New("ble: discovery stopped")
	}
}

func (r *bluezRadio) connect(ctx context.Context, addr [6]byte, lost func(error)) (conn, error) {
	r.mu.Lock()
	adapter := r.adapter
	r.mu.Unlock()
	if adapter == "" {
		return nil, ErrNotPoweredOn
	}
	path := dbus.ObjectPath(string(adapter) + "/dev_" + strings.ReplaceAll(formatAddress(addr), ":", "_"))
	obj := r.bus.Object(bluez, path)
	c := &bluezConn{r: r, path: path, obj: obj, lost: lost, resolved: make(chan struct{}), dropped: make(chan struct{})}
	r.mu.Lock()
	r.conns[path] = c
	r.mu.Unlock()
	fail := func(err error) (conn, error) {
		c.close()
		return nil, err
	}

	if err := obj.CallWithContext(ctx, ifDevice+".Connect", 0).Err; err != nil {
		var de dbus.Error
		switch {
		case ctx.Err() != nil:
			return fail(ctx.Err())
		case errors.As(err, &de) && de.Name == "org.freedesktop.DBus.Error.UnknownObject":
			r.mu.Lock()
			delete(r.conns, path)
			r.mu.Unlock()
			return nil, ErrUnknownPeripheral
		case errors.As(err, &de) && de.Name == "org.bluez.Error.NotReady":
			return fail(ErrNotPoweredOn)
		}
		return fail(err)
	}
	if v, err := obj.GetProperty(ifDevice + ".ServicesResolved"); err == nil {
		if ok, _ := v.Value().(bool); ok {
			c.markResolved()
		}
	}
	select {
	case <-c.resolved:
		return c, nil
	case <-c.dropped:
		return fail(fmt.Errorf("%w: before its services were resolved", ErrDisconnected))
	case <-ctx.Done():
		return fail(ctx.Err())
	}
}

func (r *bluezRadio) close() {
	r.bus.Close()
}

// bluezConn is one connection to a Device1.
type bluezConn struct {
	r    *bluezRadio
	path dbus.ObjectPath
	obj  dbus.BusObject
	lost func(error)

	mu       sync.Mutex
	svcs     map[string]dbus.ObjectPath
	chars    map[string]dbus.ObjectPath
	mtuByChr map[string]int

	resolveOnce sync.Once
	resolved    chan struct{}
	dropOnce    sync.Once
	dropped     chan struct{}
	closeOnce   sync.Once
}

func (c *bluezConn) markResolved() { c.resolveOnce.Do(func() { close(c.resolved) }) }

// drop runs on the dispatch goroutine when the device disconnects.
func (c *bluezConn) drop(err error) {
	c.dropOnce.Do(func() { close(c.dropped) })
	c.r.mu.Lock()
	mine := c.r.conns[c.path] == c
	if mine {
		delete(c.r.conns, c.path)
	}
	c.r.mu.Unlock()
	select {
	case <-c.resolved:
		if mine {
			c.lost(err)
		}
	default: // still connecting: connect reports it
	}
}

func (c *bluezConn) services(ctx context.Context) ([]string, error) {
	objects, err := c.r.objects(ctx)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.svcs = map[string]dbus.ObjectPath{}
	var out []string
	for p, ifaces := range objects {
		props, ok := ifaces[ifService]
		if !ok || !strings.HasPrefix(string(p), string(c.path)+"/") {
			continue
		}
		u, _ := props["UUID"].Value().(string)
		u = NormalizeUUID(u)
		if _, dup := c.svcs[u]; !dup {
			out = append(out, u)
		}
		c.svcs[u] = p
	}
	sort.Strings(out)
	return out, nil
}

var bluezFlags = map[string]Property{
	"broadcast":              PropBroadcast,
	"read":                   PropRead,
	"write-without-response": PropWriteWithoutResponse,
	"write":                  PropWrite,
	"notify":                 PropNotify,
	"indicate":               PropIndicate,
}

func (c *bluezConn) characteristics(ctx context.Context, service string) ([]Characteristic, error) {
	c.mu.Lock()
	svc, ok := c.svcs[service]
	c.mu.Unlock()
	if !ok {
		return nil, ErrNotFound
	}
	objects, err := c.r.objects(ctx)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.chars == nil {
		c.chars, c.mtuByChr = map[string]dbus.ObjectPath{}, map[string]int{}
	}
	var out []Characteristic
	for p, ifaces := range objects {
		props, ok := ifaces[ifChar]
		if !ok {
			continue
		}
		if s, _ := props["Service"].Value().(dbus.ObjectPath); s != svc {
			continue
		}
		u, _ := props["UUID"].Value().(string)
		ch := Characteristic{UUID: NormalizeUUID(u), Service: service}
		flags, _ := props["Flags"].Value().([]string)
		for _, f := range flags {
			ch.Properties |= bluezFlags[f]
		}
		if m, ok := props["MTU"].Value().(uint16); ok { // BlueZ 5.62 and later
			c.mtuByChr[ch.UUID] = int(m)
		}
		c.chars[ch.UUID] = p
		out = append(out, ch)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UUID < out[j].UUID })
	return out, nil
}

func (c *bluezConn) char(uuid string) (dbus.BusObject, dbus.ObjectPath, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	p, ok := c.chars[uuid]
	if !ok {
		return nil, "", ErrNotFound
	}
	return c.r.bus.Object(bluez, p), p, nil
}

func (c *bluezConn) subscribe(ctx context.Context, uuid string, fn func([]byte)) error {
	obj, p, err := c.char(uuid)
	if err != nil {
		return err
	}
	c.r.mu.Lock()
	c.r.handlers[p] = fn
	c.r.mu.Unlock()
	err = obj.CallWithContext(ctx, ifChar+".StartNotify", 0).Err
	var de dbus.Error
	if errors.As(err, &de) && de.Name == "org.bluez.Error.InProgress" {
		err = nil // already notifying
	}
	if err != nil {
		c.r.mu.Lock()
		delete(c.r.handlers, p)
		c.r.mu.Unlock()
	}
	return err
}

func (c *bluezConn) unsubscribe(ctx context.Context, uuid string) error {
	obj, p, err := c.char(uuid)
	if err != nil {
		return err
	}
	c.r.mu.Lock()
	delete(c.r.handlers, p)
	c.r.mu.Unlock()
	return obj.CallWithContext(ctx, ifChar+".StopNotify", 0).Err
}

func (c *bluezConn) write(ctx context.Context, uuid string, data []byte, withResponse bool) error {
	obj, _, err := c.char(uuid)
	if err != nil {
		return err
	}
	kind := "command"
	if withResponse {
		kind = "request"
	}
	return obj.CallWithContext(ctx, ifChar+".WriteValue", 0, data, map[string]dbus.Variant{"type": dbus.MakeVariant(kind)}).Err
}

// mtu is the characteristics' negotiated ATT MTU, read fresh: BlueZ
// updates it after the exchange.
func (c *bluezConn) mtu() int {
	c.mu.Lock()
	var uuids []string
	for u := range c.chars {
		uuids = append(uuids, u)
	}
	c.mu.Unlock()
	sort.Strings(uuids)
	for _, u := range uuids {
		obj, _, err := c.char(u)
		if err != nil {
			continue
		}
		if v, err := obj.GetProperty(ifChar + ".MTU"); err == nil {
			if m, ok := v.Value().(uint16); ok {
				return int(m)
			}
		}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, m := range c.mtuByChr {
		return m
	}
	return 0
}

func (c *bluezConn) close() {
	c.closeOnce.Do(func() {
		c.mu.Lock()
		var chars []dbus.ObjectPath
		for _, p := range c.chars {
			chars = append(chars, p)
		}
		c.mu.Unlock()
		c.r.mu.Lock()
		if c.r.conns[c.path] == c {
			delete(c.r.conns, c.path)
		}
		for _, p := range chars {
			delete(c.r.handlers, p)
		}
		c.r.mu.Unlock()
		ctx, cancel := context.WithTimeout(context.Background(), disconnectTimeout)
		defer cancel()
		_ = c.obj.CallWithContext(ctx, ifDevice+".Disconnect", 0).Err
	})
}
