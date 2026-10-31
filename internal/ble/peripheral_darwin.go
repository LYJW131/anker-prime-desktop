//go:build darwin

package ble

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

// disconnectTimeout bounds Disconnect's wait for CoreBluetooth to confirm.
const disconnectTimeout = 5 * time.Second

// Peripheral is one connection to a peripheral, from Connect until it
// disconnects (Done). Reconnecting returns a new Peripheral. Its methods
// are safe for concurrent use.
type Peripheral struct {
	c     *Central
	obj   id
	ident string

	local atomic.Bool // Disconnect was called

	mu       sync.Mutex
	name     string
	services map[string]id // uuid -> CBService, retained
	chars    map[string]charRef
	handlers map[string]func([]byte)
	waits    map[string]chan error

	opMu    sync.Mutex // one discovery or subscription at a time
	writeMu sync.Mutex
	ready   chan struct{} // peripheralIsReadyToSendWriteWithoutResponse:
	notes   *fifo[note]

	doneOnce sync.Once
	done     chan struct{}
	err      error
}

type charRef struct {
	obj id // CBCharacteristic, retained
	Characteristic
}

type note struct {
	uuid string
	data []byte
}

func newPeripheral(c *Central, obj id, ident, name string) *Peripheral {
	p := &Peripheral{
		c: c, obj: obj, ident: ident, name: name,
		services: map[string]id{},
		chars:    map[string]charRef{},
		handlers: map[string]func([]byte){},
		waits:    map[string]chan error{},
		ready:    make(chan struct{}, 1),
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

// ID returns the CoreBluetooth identifier.
func (p *Peripheral) ID() string { return p.ident }

// Name returns the peripheral's name, else its last advertised name.
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

// do runs fn on the queue unless the connection has ended.
func (p *Peripheral) do(fn func()) error {
	var err error
	onQueue(p.c.queue, func() {
		select {
		case <-p.done:
			err = p.err
		default:
			fn()
		}
	})
	return err
}

// await registers a waiter under key, starts the operation on the queue and
// waits for the delegate to resolve it.
func (p *Peripheral) await(ctx context.Context, key string, start func()) error {
	ch := make(chan error, 1)
	p.mu.Lock()
	p.waits[key] = ch
	p.mu.Unlock()
	drop := func() {
		p.mu.Lock()
		if p.waits[key] == ch {
			delete(p.waits, key)
		}
		p.mu.Unlock()
	}
	if err := p.do(start); err != nil {
		drop()
		return err
	}
	select {
	case err := <-ch:
		return err
	case <-p.done:
		drop()
		return p.err
	case <-ctx.Done():
		drop()
		return ctx.Err()
	}
}

// resolve answers the waiter under key; on the queue.
func (p *Peripheral) resolve(key string, err error) {
	p.mu.Lock()
	ch := p.waits[key]
	delete(p.waits, key)
	p.mu.Unlock()
	if ch != nil {
		ch <- err
	}
}

// DiscoverServices discovers the peripheral's services, all of them when
// uuids is empty, and returns the UUIDs of the services discovered so far.
func (p *Peripheral) DiscoverServices(ctx context.Context, uuids ...string) ([]string, error) {
	p.opMu.Lock()
	defer p.opMu.Unlock()
	err := p.await(ctx, "svc", func() {
		send(p.obj, "discoverServices:", uintptr(cbUUIDs(uuids)))
	})
	if err != nil {
		return nil, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]string, 0, len(p.services))
	for u := range p.services {
		out = append(out, u)
	}
	return out, nil
}

// DiscoverCharacteristics discovers the characteristics of a discovered
// service, all of them when uuids is empty, and returns that service's
// characteristics discovered so far.
func (p *Peripheral) DiscoverCharacteristics(ctx context.Context, service string, uuids ...string) ([]Characteristic, error) {
	service = NormalizeUUID(service)
	p.opMu.Lock()
	defer p.opMu.Unlock()
	p.mu.Lock()
	svc := p.services[service]
	p.mu.Unlock()
	if svc == 0 {
		return nil, ErrNotFound
	}
	err := p.await(ctx, "chr:"+service, func() {
		send(p.obj, "discoverCharacteristics:forService:", uintptr(cbUUIDs(uuids)), uintptr(svc))
	})
	if err != nil {
		return nil, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	var out []Characteristic
	for _, c := range p.chars {
		if c.Service == service {
			out = append(out, c.Characteristic)
		}
	}
	return out, nil
}

func (p *Peripheral) char(uuid string) (charRef, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	c, ok := p.chars[NormalizeUUID(uuid)]
	if !ok {
		return charRef{}, ErrNotFound
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
	err = p.await(ctx, "ntf:"+c.UUID, func() {
		send(p.obj, "setNotifyValue:forCharacteristic:", 1, uintptr(c.obj))
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
	err = p.await(ctx, "ntf:"+c.UUID, func() {
		send(p.obj, "setNotifyValue:forCharacteristic:", 0, uintptr(c.obj))
	})
	p.mu.Lock()
	delete(p.handlers, c.UUID)
	p.mu.Unlock()
	return err
}

// MaxWriteLen returns the longest value one write may carry, which follows
// the negotiated ATT MTU.
func (p *Peripheral) MaxWriteLen(withoutResponse bool) int {
	var n int
	_ = p.do(func() {
		n = sendInt(p.obj, "maximumWriteValueLengthForType:", boolArg(withoutResponse))
	})
	return n
}

// Write writes data to a discovered characteristic, waiting at most
// WriteTimeout. With a response it waits for the peripheral's
// acknowledgement; without one it waits only until CoreBluetooth has room
// to queue the write. Writes are serialized; data must fit MaxWriteLen.
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
	if withResponse {
		return p.await(ctx, "wr:"+c.UUID, func() {
			send(p.obj, "writeValue:forCharacteristic:type:", uintptr(nsData(data)), uintptr(c.obj), 0)
		})
	}
	for {
		select {
		case <-p.ready:
		default:
		}
		sent := false
		err := p.do(func() {
			if sendBool(p.obj, "canSendWriteWithoutResponse") {
				send(p.obj, "writeValue:forCharacteristic:type:", uintptr(nsData(data)), uintptr(c.obj), 1)
				sent = true
			}
		})
		if err != nil || sent {
			return err
		}
		select {
		case <-p.ready:
		case <-p.done:
			return p.err
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// Disconnect ends the connection and waits, briefly, until CoreBluetooth
// confirms. Err then returns ErrClosed.
func (p *Peripheral) Disconnect() {
	p.local.Store(true)
	select {
	case <-p.done:
		return
	default:
	}
	onQueue(p.c.queue, func() { send(p.c.mgr, "cancelPeripheralConnection:", uintptr(p.obj)) })
	select {
	case <-p.done:
	case <-time.After(disconnectTimeout):
		p.c.forget(p.obj, p)
		p.finish(ErrClosed)
	}
}

// finish ends the connection from a goroutine.
func (p *Peripheral) finish(err error) {
	onQueue(p.c.queue, func() { p.finishOnQueue(err) })
}

// finishOnQueue ends the connection once: Done closes, waiters return err,
// queued notifications are still delivered.
func (p *Peripheral) finishOnQueue(err error) {
	p.doneOnce.Do(func() {
		p.err = err
		p.mu.Lock()
		for _, s := range p.services {
			release(s)
		}
		for _, c := range p.chars {
			release(c.obj)
		}
		p.services = map[string]id{}
		p.chars = map[string]charRef{}
		p.mu.Unlock()
		close(p.done)
		p.notes.close()
	})
}

// The handlers below run on the queue, called by the delegate.

func (p *Peripheral) didDiscoverServices(err error) {
	if err == nil {
		p.mu.Lock()
		for _, s := range arrayItems(send(p.obj, "services")) {
			u := cbUUIDString(send(s, "UUID"))
			if old, ok := p.services[u]; ok {
				if old == s {
					continue
				}
				release(old)
			}
			p.services[u] = retain(s)
		}
		p.mu.Unlock()
	}
	p.resolve("svc", err)
}

func (p *Peripheral) didDiscoverCharacteristics(svc id, err error) {
	service := cbUUIDString(send(svc, "UUID"))
	if err == nil {
		p.mu.Lock()
		for _, ch := range arrayItems(send(svc, "characteristics")) {
			u := cbUUIDString(send(ch, "UUID"))
			if old, ok := p.chars[u]; ok {
				if old.obj == ch {
					continue
				}
				release(old.obj)
			}
			p.chars[u] = charRef{obj: retain(ch), Characteristic: Characteristic{
				UUID: u, Service: service, Properties: Property(sendInt(ch, "properties")),
			}}
		}
		p.mu.Unlock()
	}
	p.resolve("chr:"+service, err)
}

func (p *Peripheral) didUpdateValue(ch id, err error) {
	if err != nil {
		return
	}
	p.notes.push(note{uuid: cbUUIDString(send(ch, "UUID")), data: goBytes(send(ch, "value"))})
}

func (p *Peripheral) didUpdateNotificationState(ch id, err error) {
	p.resolve("ntf:"+cbUUIDString(send(ch, "UUID")), err)
}

func (p *Peripheral) didWrite(ch id, err error) {
	p.resolve("wr:"+cbUUIDString(send(ch, "UUID")), err)
}

func (p *Peripheral) readyToWrite() {
	select {
	case p.ready <- struct{}{}:
	default:
	}
}

func (p *Peripheral) didUpdateName() {
	if n := goString(send(p.obj, "name")); n != "" {
		p.mu.Lock()
		p.name = n
		p.mu.Unlock()
	}
}
