//go:build windows || linux

package ble

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// radio is the system's Bluetooth stack under a Central: WinRT on Windows,
// BlueZ on Linux. Its callbacks never block.
type radio interface {
	// open starts using the stack and reports the radio's state to set,
	// at once and on every change.
	open(set func(State)) error
	// scan reports advertisements until stop is closed or the stack stops
	// scanning by itself, and returns once it has stopped.
	scan(stop <-chan struct{}, report func(Advert)) error
	// connect connects to the peripheral at addr until ctx ends. lost is
	// called once when the connection drops afterwards, but not after
	// conn.close.
	connect(ctx context.Context, addr [6]byte, lost func(error)) (conn, error)
	close()
}

// conn is one connection. UUIDs are normalized.
type conn interface {
	services(ctx context.Context) ([]string, error)
	characteristics(ctx context.Context, service string) ([]Characteristic, error)
	// subscribe delivers values in order; fn never blocks.
	subscribe(ctx context.Context, char string, fn func([]byte)) error
	unsubscribe(ctx context.Context, char string) error
	write(ctx context.Context, char string, data []byte, withResponse bool) error
	// mtu is the ATT MTU, 0 when unknown.
	mtu() int
	// close disconnects; it may be called more than once.
	close()
}

// disconnectTimeout bounds Disconnect's wait for the stack.
const disconnectTimeout = 5 * time.Second

// scanRetry is the pause before scanning again after the stack stopped a
// scan by itself while the radio stayed on.
const scanRetry = 2 * time.Second

// longWriteLen is the longest value a write with response carries: the
// stack splits it with prepared writes.
const longWriteLen = 512

// Central is the system's Bluetooth central. Its methods are safe for
// concurrent use.
type Central struct {
	r     radio
	state atomic.Int32

	mu        sync.Mutex
	closed    bool
	watchers  map[chan State]struct{}
	names     map[string]string        // ID -> last advertised name
	busy      map[string]*Peripheral   // connecting or connected, by ID
	abandoned map[string]chan struct{} // cancelled connects still unwinding
	scan      *scanReq

	scanMu sync.Mutex // one stack scan at a time
}

type scanReq struct {
	uuids []string
	q     *fifo[Advert]
	stop  chan struct{}
	once  sync.Once
}

func (r *scanReq) end() {
	r.once.Do(func() {
		close(r.stop)
		r.q.close()
	})
}

// NewCentral opens the system's Bluetooth stack, the first thing in the
// package that touches it. The radio's state arrives asynchronously: use
// WaitPoweredOn or WatchState.
func NewCentral() (*Central, error) {
	c := &Central{
		r:         newRadio(),
		watchers:  map[chan State]struct{}{},
		names:     map[string]string{},
		busy:      map[string]*Peripheral{},
		abandoned: map[string]chan struct{}{},
	}
	if err := c.r.open(c.setState); err != nil {
		return nil, err
	}
	return c, nil
}

// State returns the radio's latest state.
func (c *Central) State() State { return State(c.state.Load()) }

// Authorization is always AuthAllowed: Windows and Linux ask no permission
// of desktop apps; a stack that refuses reports StateUnauthorized.
func (c *Central) Authorization() Authorization { return AuthAllowed }

// CurrentAuthorization returns AuthAllowed without touching the stack.
func CurrentAuthorization() (Authorization, error) { return AuthAllowed, nil }

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

// setState runs on the stack's thread.
func (c *Central) setState(s State) {
	if State(c.state.Swap(int32(s))) == s {
		return
	}
	c.mu.Lock()
	for ch := range c.watchers {
		select {
		case <-ch:
		default:
		}
		ch <- s
	}
	var lost []*Peripheral
	if s != StatePoweredOn {
		for _, p := range c.busy {
			if p.connected() {
				lost = append(lost, p)
			}
		}
	}
	c.mu.Unlock()
	for _, p := range lost {
		p.finish(fmt.Errorf("%w: bluetooth %v", ErrDisconnected, s))
	}
}

// ScanOption changes how Scan reports advertisements.
type ScanOption func(*scanReq)

// AllowDuplicates reports every advertisement of a peripheral. Windows and
// Linux always do: callers key results by Advert.ID.
func AllowDuplicates() ScanOption { return func(*scanReq) {} }

// Scan starts reporting advertisements of peripherals offering any of
// serviceUUIDs (all peripherals when empty) to cb, in order on a goroutine
// of its own, until StopScan or another Scan. It returns at once; the scan
// runs whenever the radio is on, restarting after it is turned back on.
func (c *Central) Scan(serviceUUIDs []string, cb func(Advert), opts ...ScanOption) error {
	if err := stateErr(c.State()); err != nil {
		return err
	}
	r := &scanReq{q: newFIFO[Advert](), stop: make(chan struct{})}
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
		old.end()
	}
	go r.q.run(cb)
	go c.runScan(r)
	return nil
}

func (c *Central) runScan(r *scanReq) {
	states, cancel := c.WatchState()
	defer cancel()
	report := func(a Advert) {
		if a.Name != "" {
			c.mu.Lock()
			c.names[a.ID] = a.Name
			c.mu.Unlock()
		}
		if offersAny(a, r.uuids) {
			r.q.push(a)
		}
	}
	for {
		for c.State() != StatePoweredOn {
			select {
			case <-states:
			case <-r.stop:
				return
			}
		}
		c.scanMu.Lock()
		select {
		case <-r.stop:
			c.scanMu.Unlock()
			return
		default:
		}
		_ = c.r.scan(r.stop, report)
		c.scanMu.Unlock()
		select {
		case <-r.stop:
			return
		case <-states:
		case <-time.After(scanRetry):
		}
	}
}

// StopScan stops the scan; advertisements already queued are still
// delivered.
func (c *Central) StopScan() {
	c.mu.Lock()
	r := c.scan
	c.scan = nil
	c.mu.Unlock()
	if r != nil {
		r.end()
	}
}

// RetrieveConnected returns no peripherals: neither stack lists the
// connections of other apps by service.
func (c *Central) RetrieveConnected(serviceUUID string) ([]Device, error) {
	if s := c.State(); s != StatePoweredOn {
		if err := stateErr(s); err != nil {
			return nil, err
		}
		return nil, ErrNotPoweredOn
	}
	return nil, nil
}

// Connect connects to the peripheral with the given ID, its address. It
// waits, through radio power cycles, until the peripheral shows up or ctx
// ends. It returns ErrUnknownPeripheral when the system has not seen the
// address, so the caller should scan.
func (c *Central) Connect(ctx context.Context, ident string) (*Peripheral, error) {
	addr, err := parseAddress(ident)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnknownPeripheral, err)
	}
	ident = formatAddress(addr)
	for {
		if err := c.WaitPoweredOn(ctx); err != nil {
			return nil, err
		}
		p, err := c.connectOnce(ctx, ident, addr)
		if errors.Is(err, ErrNotPoweredOn) {
			continue
		}
		return p, err
	}
}

func (c *Central) connectOnce(ctx context.Context, ident string, addr [6]byte) (*Peripheral, error) {
	c.mu.Lock()
	unwinding := c.abandoned[ident]
	c.mu.Unlock()
	if unwinding != nil {
		select {
		case <-unwinding:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil, ErrClosed
	}
	if c.busy[ident] != nil || c.abandoned[ident] != nil {
		c.mu.Unlock()
		return nil, ErrBusy
	}
	p := newPeripheral(c, ident, c.names[ident])
	c.busy[ident] = p
	c.mu.Unlock()

	type result struct {
		cn  conn
		err error
	}
	res := make(chan result, 1)
	cctx, cancel := context.WithCancel(ctx)
	go func() {
		cn, err := c.r.connect(cctx, addr, p.lost)
		res <- result{cn, err}
	}()
	states, unwatch := c.WatchState()
	defer unwatch()

	abandon := func(err error) error {
		cancel()
		gone := make(chan struct{})
		c.mu.Lock()
		if c.busy[ident] == p {
			delete(c.busy, ident)
		}
		c.abandoned[ident] = gone
		c.mu.Unlock()
		p.finish(err)
		go func() {
			if r := <-res; r.err == nil {
				r.cn.close()
			}
			c.mu.Lock()
			if c.abandoned[ident] == gone {
				delete(c.abandoned, ident)
			}
			c.mu.Unlock()
			close(gone)
		}()
		return err
	}
	for {
		select {
		case r := <-res:
			cancel()
			if r.err != nil {
				p.finish(r.err)
				return nil, r.err
			}
			if !p.attach(r.cn) {
				r.cn.close()
				return nil, p.Err()
			}
			return p, nil
		case s := <-states:
			if s != StatePoweredOn {
				return nil, abandon(ErrNotPoweredOn)
			}
		case <-ctx.Done():
			return nil, abandon(ctx.Err())
		}
	}
}

func (c *Central) release(ident string, p *Peripheral) {
	c.mu.Lock()
	if c.busy[ident] == p {
		delete(c.busy, ident)
	}
	c.mu.Unlock()
}

// Close stops the scan, disconnects every peripheral and closes the stack.
// The Central cannot be used afterwards.
func (c *Central) Close() {
	c.StopScan()
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	c.closed = true
	var ps []*Peripheral
	for _, p := range c.busy {
		ps = append(ps, p)
	}
	c.mu.Unlock()
	for _, p := range ps {
		p.Disconnect()
	}
	c.r.close()
}

// Peripheral is one connection to a peripheral, from Connect until it
// disconnects (Done). Reconnecting returns a new Peripheral. Its methods
// are safe for concurrent use.
type Peripheral struct {
	c     *Central
	ident string

	local atomic.Bool // Disconnect was called

	mu       sync.Mutex
	name     string
	cn       conn
	services map[string]bool
	chars    map[string]Characteristic
	handlers map[string]func([]byte)

	opMu    sync.Mutex // one discovery or subscription at a time
	writeMu sync.Mutex
	notes   *fifo[note]

	doneOnce sync.Once
	done     chan struct{}
	err      error
}

type note struct {
	uuid string
	data []byte
}

func newPeripheral(c *Central, ident, name string) *Peripheral {
	p := &Peripheral{
		c: c, ident: ident, name: name,
		services: map[string]bool{},
		chars:    map[string]Characteristic{},
		handlers: map[string]func([]byte){},
		notes:    newFIFO[note](),
		done:     make(chan struct{}),
	}
	go p.notes.run(func(n note) {
		p.mu.Lock()
		h := p.handlers[n.uuid]
		p.mu.Unlock()
		if h != nil {
			h(n.data)
		}
	})
	return p
}

// attach hands the peripheral its connection; false when it ended first.
func (p *Peripheral) attach(cn conn) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	select {
	case <-p.done:
		return false
	default:
	}
	p.cn = cn
	return true
}

func (p *Peripheral) connected() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.cn != nil
}

// lost is the stack's report that the connection dropped; it never blocks.
func (p *Peripheral) lost(err error) {
	if err == nil {
		err = fmt.Errorf("%w: by the peripheral", ErrDisconnected)
	} else if !errors.Is(err, ErrDisconnected) {
		err = fmt.Errorf("%w: %v", ErrDisconnected, err)
	}
	p.finish(err)
}

// finish ends the connection once: Done closes, waiters return err (or
// ErrClosed after Disconnect), queued notifications are still delivered,
// and the stack's connection is closed off the caller's thread.
func (p *Peripheral) finish(err error) {
	p.doneOnce.Do(func() {
		if p.local.Load() {
			err = ErrClosed
		}
		p.mu.Lock()
		p.err = err
		cn := p.cn
		p.mu.Unlock()
		close(p.done)
		p.notes.close()
		p.c.release(p.ident, p)
		if cn != nil {
			go cn.close()
		}
	})
}

// ID returns the peripheral's address, "AA:BB:CC:DD:EE:FF".
func (p *Peripheral) ID() string { return p.ident }

// Name returns the peripheral's last advertised name.
func (p *Peripheral) Name() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.name
}

// Done is closed when the connection ends.
func (p *Peripheral) Done() <-chan struct{} { return p.done }

// Err returns nil while connected; after Done, ErrClosed for a Disconnect
// by this process, or an error wrapping ErrDisconnected with the reason.
func (p *Peripheral) Err() error {
	select {
	case <-p.done:
		return p.err
	default:
		return nil
	}
}

// conn returns the connection unless it has ended.
func (p *Peripheral) conn() (conn, error) {
	select {
	case <-p.done:
		return nil, p.err
	default:
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cn == nil {
		return nil, ErrClosed
	}
	return p.cn, nil
}

// call runs fn, which talks to the stack, until ctx ends or the connection
// does; fn keeps running in the background then.
func (p *Peripheral) call(ctx context.Context, fn func(cn conn) error) error {
	cn, err := p.conn()
	if err != nil {
		return err
	}
	res := make(chan error, 1)
	go func() { res <- fn(cn) }()
	select {
	case err := <-res:
		if err != nil {
			select {
			case <-p.done:
				return p.err
			default:
			}
		}
		return err
	case <-p.done:
		return p.err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// DiscoverServices discovers the peripheral's services and returns the
// UUIDs of those among uuids, all of them when uuids is empty.
func (p *Peripheral) DiscoverServices(ctx context.Context, uuids ...string) ([]string, error) {
	p.opMu.Lock()
	defer p.opMu.Unlock()
	var found []string
	err := p.call(ctx, func(cn conn) error {
		var err error
		found, err = cn.services(ctx)
		return err
	})
	if err != nil {
		return nil, err
	}
	p.mu.Lock()
	for _, u := range found {
		p.services[u] = true
	}
	p.mu.Unlock()
	return matchUUIDs(found, func(s string) string { return s }, uuids), nil
}

// DiscoverCharacteristics discovers the characteristics of a discovered
// service and returns those among uuids, all of them when uuids is empty.
func (p *Peripheral) DiscoverCharacteristics(ctx context.Context, service string, uuids ...string) ([]Characteristic, error) {
	service = NormalizeUUID(service)
	p.opMu.Lock()
	defer p.opMu.Unlock()
	p.mu.Lock()
	known := p.services[service]
	p.mu.Unlock()
	if !known {
		return nil, ErrNotFound
	}
	var found []Characteristic
	err := p.call(ctx, func(cn conn) error {
		var err error
		found, err = cn.characteristics(ctx, service)
		return err
	})
	if err != nil {
		return nil, err
	}
	p.mu.Lock()
	for _, ch := range found {
		p.chars[ch.UUID] = ch
	}
	p.mu.Unlock()
	return matchUUIDs(found, func(c Characteristic) string { return c.UUID }, uuids), nil
}

func (p *Peripheral) char(uuid string) (Characteristic, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	c, ok := p.chars[NormalizeUUID(uuid)]
	if !ok {
		return Characteristic{}, ErrNotFound
	}
	return c, nil
}

// Subscribe enables notifications (or indications) of a discovered
// characteristic and delivers their values to fn, in order, on one
// goroutine shared by all of the peripheral's subscriptions. fn must not
// block for long: later values wait behind it.
func (p *Peripheral) Subscribe(ctx context.Context, characteristic string, fn func([]byte)) error {
	c, err := p.char(characteristic)
	if err != nil {
		return err
	}
	p.opMu.Lock()
	defer p.opMu.Unlock()
	p.mu.Lock()
	p.handlers[c.UUID] = fn
	p.mu.Unlock()
	err = p.call(ctx, func(cn conn) error {
		return cn.subscribe(ctx, c.UUID, func(b []byte) {
			p.notes.push(note{uuid: c.UUID, data: bytes.Clone(b)})
		})
	})
	if err != nil {
		p.mu.Lock()
		delete(p.handlers, c.UUID)
		p.mu.Unlock()
	}
	return err
}

// Unsubscribe disables notifications of a characteristic.
func (p *Peripheral) Unsubscribe(ctx context.Context, characteristic string) error {
	c, err := p.char(characteristic)
	if err != nil {
		return err
	}
	p.opMu.Lock()
	defer p.opMu.Unlock()
	err = p.call(ctx, func(cn conn) error { return cn.unsubscribe(ctx, c.UUID) })
	p.mu.Lock()
	delete(p.handlers, c.UUID)
	p.mu.Unlock()
	return err
}

// MaxWriteLen returns the longest value one write may carry: the
// negotiated ATT MTU less 3 without a response, 512 with one. It is 0 when
// the stack does not report the MTU.
func (p *Peripheral) MaxWriteLen(withoutResponse bool) int {
	cn, err := p.conn()
	if err != nil {
		return 0
	}
	m := cn.mtu()
	if m <= 3 {
		return 0
	}
	if !withoutResponse {
		return longWriteLen
	}
	return m - 3
}

// Write writes data to a discovered characteristic, waiting at most
// WriteTimeout for the stack (and, with a response, the peripheral) to
// accept it. Writes are serialized; data must fit MaxWriteLen.
func (p *Peripheral) Write(characteristic string, data []byte, withResponse bool) error {
	ctx, cancel := context.WithTimeout(context.Background(), WriteTimeout)
	defer cancel()
	err := p.WriteContext(ctx, characteristic, data, withResponse)
	if err == context.DeadlineExceeded {
		return ErrTimeout
	}
	return err
}

// WriteContext is Write bounded by ctx instead of WriteTimeout.
func (p *Peripheral) WriteContext(ctx context.Context, characteristic string, data []byte, withResponse bool) error {
	c, err := p.char(characteristic)
	if err != nil {
		return err
	}
	p.writeMu.Lock()
	defer p.writeMu.Unlock()
	if max := p.MaxWriteLen(!withResponse); max > 0 && len(data) > max {
		return ErrTooLong
	}
	data = bytes.Clone(data)
	return p.call(ctx, func(cn conn) error { return cn.write(ctx, c.UUID, data, withResponse) })
}

// Disconnect ends the connection and waits, briefly, for the stack. Err
// then returns ErrClosed.
func (p *Peripheral) Disconnect() {
	p.local.Store(true)
	select {
	case <-p.done:
		return
	default:
	}
	p.mu.Lock()
	cn := p.cn
	p.mu.Unlock()
	if cn != nil {
		closed := make(chan struct{})
		go func() {
			cn.close()
			close(closed)
		}()
		select {
		case <-closed:
		case <-time.After(disconnectTimeout):
		}
	}
	p.finish(ErrClosed)
}
