//go:build darwin

package ble

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
)

// Central is a CBCentralManager with its delegate on a private serial
// queue. Its methods are safe for concurrent use.
type Central struct {
	queue     uintptr
	mgr       id // CBCentralManager, owned
	delegate  id // central delegate, owned; the manager holds it weakly
	pdelegate id // peripheral delegate shared by all peripherals, owned

	state atomic.Int32

	mu       sync.Mutex
	closed   bool
	watchers map[chan State]struct{}
	known    map[string]id     // identifier -> CBPeripheral, retained
	names    map[string]string // identifier -> last advertised name
	periphs  map[id]*Peripheral
	pending  map[id]*connectReq
	scan     *scanReq
}

type connectReq struct {
	p      *Peripheral
	result chan error // capacity 1
}

type scanReq struct {
	uuids []string
	dup   bool
	q     *fifo[Advert]
}

var (
	registryMu sync.Mutex
	centrals   = map[id]*Central{} // by central or peripheral delegate
)

// NewCentral creates the CBCentralManager, the first thing in the package
// that touches CoreBluetooth; the first call in a process may raise the
// Bluetooth permission prompt. The radio's state arrives asynchronously:
// use WaitPoweredOn or WatchState.
func NewCentral() (*Central, error) {
	if err := load(); err != nil {
		return nil, err
	}
	c := &Central{
		watchers: map[chan State]struct{}{},
		known:    map[string]id{},
		names:    map[string]string{},
		periphs:  map[id]*Peripheral{},
		pending:  map[id]*connectReq{},
	}
	label := []byte("anker-prime.ble\x00")
	c.queue = dispatchQueueCreate(&label[0], 0) // serial
	withPool(func() {
		c.delegate = send(send(class(centralDelegateClass), "alloc"), "init")
		c.pdelegate = send(send(class(peripheralDelegateClass), "alloc"), "init")
		registryMu.Lock()
		centrals[c.delegate] = c
		centrals[c.pdelegate] = c
		registryMu.Unlock()
		// No system alert when Bluetooth is off: the app shows the state.
		opts := send(class("NSDictionary"), "dictionaryWithObject:forKey:",
			uintptr(send(class("NSNumber"), "numberWithBool:", 0)),
			uintptr(cbString("CBCentralManagerOptionShowPowerAlertKey", "kCBInitOptionShowPowerAlert")))
		c.mgr = send(send(class("CBCentralManager"), "alloc"), "initWithDelegate:queue:options:",
			uintptr(c.delegate), c.queue, uintptr(opts))
	})
	if c.mgr == 0 {
		return nil, errors.New("ble: cannot create CBCentralManager")
	}
	return c, nil
}

// withPool runs fn on one thread inside an autorelease pool.
func withPool(fn func()) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	p := poolPush()
	defer poolPop(p)
	fn()
}

// State returns the radio's latest state.
func (c *Central) State() State { return State(c.state.Load()) }

// Authorization returns the app's Bluetooth permission.
func (c *Central) Authorization() Authorization { return authorization() }

// CurrentAuthorization returns the app's Bluetooth permission without
// creating a manager, so it never prompts. It loads CoreBluetooth.
func CurrentAuthorization() (Authorization, error) {
	if err := load(); err != nil {
		return AuthNotDetermined, err
	}
	return authorization(), nil
}

func authorization() Authorization {
	var a Authorization
	withPool(func() { a = Authorization(sendInt(class("CBManager"), "authorization")) })
	return a
}

// WatchState returns a channel that receives the current state at once and
// then every change; a slow reader sees only the latest. cancel stops it.
func (c *Central) WatchState() (states <-chan State, cancel func()) {
	ch := make(chan State, 1)
	c.mu.Lock()
	c.watchers[ch] = struct{}{}
	ch <- c.State()
	c.mu.Unlock()
	return ch, func() {
		c.mu.Lock()
		delete(c.watchers, ch)
		c.mu.Unlock()
	}
}

// WaitPoweredOn waits until the radio is on. It returns ErrUnauthorized or
// errors.ErrUnsupported at once when the radio can never come on, and
// ctx.Err() when ctx ends first. Powered off it keeps waiting.
func (c *Central) WaitPoweredOn(ctx context.Context) error {
	states, cancel := c.WatchState()
	defer cancel()
	for {
		select {
		case s := <-states:
			if err := stateErr(s); err != nil || s == StatePoweredOn {
				return err
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func stateErr(s State) error {
	switch s {
	case StateUnauthorized:
		return ErrUnauthorized
	case StateUnsupported:
		return fmt.Errorf("ble: bluetooth LE: %w", errors.ErrUnsupported)
	}
	return nil
}

// ScanOption changes how Scan reports advertisements.
type ScanOption func(*scanReq)

// AllowDuplicates asks CoreBluetooth for every advertisement of a
// peripheral, as for RSSI or manufacturer data that changes. Without it
// macOS still repeats adverts (advert plus scan response, and more), so
// callers key results by Advert.ID either way.
func AllowDuplicates() ScanOption { return func(r *scanReq) { r.dup = true } }

// Scan starts reporting advertisements of peripherals offering any of
// serviceUUIDs (all peripherals when empty) to cb, in order on a goroutine
// of its own, until StopScan or another Scan. It returns at once; the scan
// runs whenever the radio is on, restarting after it is turned back on.
func (c *Central) Scan(serviceUUIDs []string, cb func(Advert), opts ...ScanOption) error {
	if err := stateErr(c.State()); err != nil {
		return err
	}
	r := &scanReq{q: newFIFO[Advert]()}
	for _, u := range serviceUUIDs {
		r.uuids = append(r.uuids, NormalizeUUID(u))
	}
	for _, o := range opts {
		o(r)
	}
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return ErrClosed
	}
	old := c.scan
	c.scan = r
	c.mu.Unlock()
	if old != nil {
		old.q.close()
	}
	go r.q.run(cb)
	onQueue(c.queue, func() {
		if c.State() == StatePoweredOn {
			c.startScan(c.mgr, r)
		}
	})
	return nil
}

// startScan runs on the queue.
func (c *Central) startScan(mgr id, r *scanReq) {
	opts := send(class("NSDictionary"), "dictionaryWithObject:forKey:",
		uintptr(send(class("NSNumber"), "numberWithBool:", boolArg(r.dup))),
		uintptr(cbString("CBCentralManagerScanOptionAllowDuplicatesKey", "kCBScanOptionAllowDuplicates")))
	send(mgr, "scanForPeripheralsWithServices:options:", uintptr(cbUUIDs(r.uuids)), uintptr(opts))
}

// StopScan stops the scan; advertisements already queued are still
// delivered.
func (c *Central) StopScan() {
	c.mu.Lock()
	r := c.scan
	c.scan = nil
	c.mu.Unlock()
	if r != nil {
		r.q.close()
	}
	onQueue(c.queue, func() {
		if c.State() == StatePoweredOn {
			send(c.mgr, "stopScan")
		}
	})
}

// RetrieveConnected returns the peripherals offering serviceUUID that are
// connected to this Mac, by any app; Connect accepts their IDs.
func (c *Central) RetrieveConnected(serviceUUID string) ([]Device, error) {
	if s := c.State(); s != StatePoweredOn {
		if err := stateErr(s); err != nil {
			return nil, err
		}
		return nil, ErrNotPoweredOn
	}
	var out []Device
	onQueue(c.queue, func() {
		arr := send(c.mgr, "retrieveConnectedPeripheralsWithServices:", uintptr(cbUUIDs([]string{serviceUUID})))
		for _, p := range arrayItems(arr) {
			ident := identifierOf(p)
			c.remember(ident, p)
			out = append(out, Device{ID: ident, Name: c.nameOf(ident, p)})
		}
	})
	return out, nil
}

// remember keeps a retained reference to a peripheral; on the queue.
func (c *Central) remember(ident string, p id) id {
	ident = strings.ToUpper(ident)
	c.mu.Lock()
	defer c.mu.Unlock()
	if old, ok := c.known[ident]; ok {
		return old
	}
	c.known[ident] = retain(p)
	return p
}

// nameOf returns a peripheral's name, else its last advertised name.
func (c *Central) nameOf(ident string, p id) string {
	ident = strings.ToUpper(ident)
	if n := goString(send(p, "name")); n != "" {
		return n
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.names[ident]
}

// lookup finds a peripheral seen this run or known to the system; on the
// queue.
func (c *Central) lookup(ident string) id {
	ident = strings.ToUpper(ident)
	c.mu.Lock()
	p := c.known[ident]
	c.mu.Unlock()
	if p != 0 {
		return p
	}
	u := send(send(class("NSUUID"), "alloc"), "initWithUUIDString:", uintptr(nsString(ident)))
	if u == 0 {
		return 0
	}
	defer release(u)
	arr := send(c.mgr, "retrievePeripheralsWithIdentifiers:", uintptr(nsArray([]id{u})))
	items := arrayItems(arr)
	if len(items) == 0 {
		return 0
	}
	return c.remember(ident, items[0])
}

// Connect connects to the peripheral with identifier id, seen in a scan this
// run or remembered by the system from an earlier one. CoreBluetooth
// connect requests do not time out: Connect waits, through radio power
// cycles, until the peripheral shows up or ctx ends, which cancels the
// request. It returns ErrUnknownPeripheral when the system does not know
// the identifier, so the caller should scan.
func (c *Central) Connect(ctx context.Context, ident string) (*Peripheral, error) {
	for {
		if err := c.WaitPoweredOn(ctx); err != nil {
			return nil, err
		}
		p, err := c.connectOnce(ctx, ident)
		if errors.Is(err, ErrNotPoweredOn) {
			continue // the radio went off meanwhile; wait for it again
		}
		return p, err
	}
}

func (c *Central) connectOnce(ctx context.Context, ident string) (*Peripheral, error) {
	var obj id
	var name string
	onQueue(c.queue, func() {
		if obj = c.lookup(ident); obj != 0 {
			name = c.nameOf(ident, obj)
		}
	})
	if obj == 0 {
		return nil, ErrUnknownPeripheral
	}

	req := &connectReq{p: newPeripheral(c, obj, ident, name), result: make(chan error, 1)}
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil, ErrClosed
	}
	if c.periphs[obj] != nil || c.pending[obj] != nil {
		c.mu.Unlock()
		return nil, ErrBusy
	}
	c.pending[obj] = req
	c.mu.Unlock()

	states, cancel := c.WatchState()
	defer cancel()
	onQueue(c.queue, func() {
		if c.State() != StatePoweredOn {
			return // the state change fails the request
		}
		send(obj, "setDelegate:", uintptr(c.pdelegate))
		send(c.mgr, "connectPeripheral:options:", uintptr(obj), 0)
	})

	abandon := func() bool {
		c.mu.Lock()
		defer c.mu.Unlock()
		if c.pending[obj] == req {
			delete(c.pending, obj)
			return true
		}
		return false // already resolved
	}
	for {
		select {
		case err := <-req.result:
			if err != nil {
				req.p.finish(err)
				return nil, err
			}
			return req.p, nil
		case s := <-states:
			if s == StatePoweredOn {
				continue
			}
			if abandon() {
				req.p.finish(ErrNotPoweredOn)
				return nil, ErrNotPoweredOn
			}
		case <-ctx.Done():
			if abandon() {
				onQueue(c.queue, func() { send(c.mgr, "cancelPeripheralConnection:", uintptr(obj)) })
				req.p.finish(ctx.Err())
				return nil, ctx.Err()
			}
			// Connected just now: hand it back rather than drop it.
		}
	}
}

// Close stops the scan, disconnects every peripheral and releases the
// manager. The Central cannot be used afterwards.
func (c *Central) Close() {
	c.StopScan()
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	c.closed = true
	var ps []*Peripheral
	for _, p := range c.periphs {
		ps = append(ps, p)
	}
	for obj, r := range c.pending {
		delete(c.pending, obj)
		r.result <- ErrClosed
		ps = append(ps, r.p)
	}
	c.mu.Unlock()
	for _, p := range ps {
		p.Disconnect()
	}
	onQueue(c.queue, func() {
		send(c.mgr, "setDelegate:", 0)
		release(c.mgr)
		c.mu.Lock()
		for _, p := range c.known {
			release(p)
		}
		c.known = map[string]id{}
		c.mu.Unlock()
	})
	registryMu.Lock()
	delete(centrals, c.delegate)
	delete(centrals, c.pdelegate)
	registryMu.Unlock()
	// The delegates stay allocated: CoreBluetooth may still have callbacks
	// queued for them.
}

// The handlers below run on the queue, called by the delegates.

func (c *Central) didUpdateState(mgr id) {
	// The manager comes from the callback: the first state may arrive
	// before initWithDelegate:queue:options: has returned.
	s := State(sendInt(mgr, "state"))
	c.state.Store(int32(s))
	c.mu.Lock()
	for ch := range c.watchers {
		select {
		case <-ch:
		default:
		}
		ch <- s
	}
	scan := c.scan
	var lost []*Peripheral
	if s != StatePoweredOn {
		// Powered off, CoreBluetooth drops every connection and pending
		// request without necessarily saying so.
		for obj, p := range c.periphs {
			delete(c.periphs, obj)
			lost = append(lost, p)
		}
		for obj, r := range c.pending {
			delete(c.pending, obj)
			r.result <- ErrNotPoweredOn
		}
	}
	c.mu.Unlock()
	for _, p := range lost {
		p.finishOnQueue(fmt.Errorf("%w: bluetooth %v", ErrDisconnected, s))
	}
	if s == StatePoweredOn && scan != nil {
		c.startScan(mgr, scan)
	}
}

func (c *Central) didDiscover(p, adv, rssi id) {
	ident := identifierOf(p)
	c.remember(ident, p)
	a := Advert{ID: ident, RSSI: 127}
	if rssi != 0 {
		a.RSSI = sendInt(rssi, "integerValue")
	}
	get := func(key, fallback string) id {
		if adv == 0 {
			return 0
		}
		return send(adv, "objectForKey:", uintptr(cbString(key, fallback)))
	}
	local := ""
	if v := get("CBAdvertisementDataLocalNameKey", "kCBAdvDataLocalName"); isKind(v, "NSString") {
		local = goString(v)
	}
	if v := get("CBAdvertisementDataIsConnectable", "kCBAdvDataIsConnectable"); isKind(v, "NSNumber") {
		a.Connectable = sendBool(v, "boolValue")
	}
	if v := get("CBAdvertisementDataManufacturerDataKey", "kCBAdvDataManufacturerData"); isKind(v, "NSData") {
		if company, data, ok := splitManufacturerData(goBytes(v)); ok {
			a.ManufacturerData = map[uint16][]byte{company: data}
		}
	}
	if v := get("CBAdvertisementDataServiceUUIDsKey", "kCBAdvDataServiceUUIDs"); isKind(v, "NSArray") {
		for _, u := range arrayItems(v) {
			a.ServiceUUIDs = append(a.ServiceUUIDs, cbUUIDString(u))
		}
	}
	if v := get("CBAdvertisementDataServiceDataKey", "kCBAdvDataServiceData"); isKind(v, "NSDictionary") {
		for _, k := range arrayItems(send(v, "allKeys")) {
			if d := send(v, "objectForKey:", uintptr(k)); isKind(d, "NSData") {
				if a.ServiceData == nil {
					a.ServiceData = map[string][]byte{}
				}
				a.ServiceData[cbUUIDString(k)] = goBytes(d)
			}
		}
	}
	a.Name = goString(send(p, "name"))
	if a.Name == "" {
		a.Name = local
	}
	c.mu.Lock()
	if local != "" {
		c.names[ident] = local
	} else if a.Name != "" {
		c.names[ident] = a.Name
	}
	scan := c.scan
	c.mu.Unlock()
	if scan != nil {
		scan.q.push(a)
	}
}

func (c *Central) didConnect(mgr, obj id) {
	c.mu.Lock()
	r := c.pending[obj]
	if r != nil {
		delete(c.pending, obj)
		c.periphs[obj] = r.p
	}
	c.mu.Unlock()
	if r == nil {
		// Nobody waits any more (cancelled just before): let it go.
		send(mgr, "cancelPeripheralConnection:", uintptr(obj))
		return
	}
	r.result <- nil
}

func (c *Central) didFailToConnect(obj id, err error) {
	c.mu.Lock()
	r := c.pending[obj]
	delete(c.pending, obj)
	c.mu.Unlock()
	if r != nil {
		if err == nil {
			err = errors.New("ble: connection failed")
		}
		r.result <- err
	}
}

func (c *Central) didDisconnect(obj id, err error) {
	c.mu.Lock()
	p := c.periphs[obj]
	delete(c.periphs, obj)
	r := c.pending[obj]
	delete(c.pending, obj)
	c.mu.Unlock()
	if r != nil {
		r.result <- fmt.Errorf("%w: %v", ErrDisconnected, err)
	}
	if p != nil {
		switch {
		case p.local.Load():
			p.finishOnQueue(ErrClosed)
		case err == nil:
			p.finishOnQueue(fmt.Errorf("%w: by the peripheral", ErrDisconnected))
		default:
			p.finishOnQueue(fmt.Errorf("%w: %v", ErrDisconnected, err))
		}
	}
}

// peripheral returns the connected or connecting peripheral for obj.
func (c *Central) peripheral(obj id) *Peripheral {
	c.mu.Lock()
	defer c.mu.Unlock()
	if p := c.periphs[obj]; p != nil {
		return p
	}
	if r := c.pending[obj]; r != nil {
		return r.p
	}
	return nil
}

func (c *Central) forget(obj id, p *Peripheral) {
	c.mu.Lock()
	if c.periphs[obj] == p {
		delete(c.periphs, obj)
	}
	c.mu.Unlock()
}
