//go:build windows || linux

package ble

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

type fakeRadio struct {
	set     func(State)
	adverts []Advert
	block   chan struct{} // connect waits on it when set

	mu   sync.Mutex
	last *fakeConn
}

func (r *fakeRadio) open(set func(State)) error { r.set = set; set(StatePoweredOn); return nil }

func (r *fakeRadio) scan(stop <-chan struct{}, report func(Advert)) error {
	for _, a := range r.adverts {
		report(a)
	}
	<-stop
	return nil
}

func (r *fakeRadio) connect(ctx context.Context, addr [6]byte, lost func(error)) (conn, error) {
	if r.block != nil {
		select {
		case <-r.block:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	c := &fakeConn{lost: lost}
	r.mu.Lock()
	r.last = c
	r.mu.Unlock()
	return c, nil
}

func (r *fakeRadio) close() {}

type fakeConn struct {
	lost   func(error)
	mu     sync.Mutex
	notify func([]byte)
	writes [][]byte
	closed bool
}

func (c *fakeConn) services(context.Context) ([]string, error) {
	return []string{NormalizeUUID("8c850001-0302-41c5-b46e-cf057c562025"), NormalizeUUID("180F")}, nil
}

func (c *fakeConn) characteristics(_ context.Context, service string) ([]Characteristic, error) {
	return []Characteristic{
		{UUID: NormalizeUUID("8c850002-0302-41c5-b46e-cf057c562025"), Service: service, Properties: PropWriteWithoutResponse},
		{UUID: NormalizeUUID("8c850003-0302-41c5-b46e-cf057c562025"), Service: service, Properties: PropNotify},
	}, nil
}

func (c *fakeConn) subscribe(_ context.Context, _ string, fn func([]byte)) error {
	c.mu.Lock()
	c.notify = fn
	c.mu.Unlock()
	return nil
}

func (c *fakeConn) unsubscribe(context.Context, string) error { return nil }

func (c *fakeConn) write(_ context.Context, _ string, data []byte, _ bool) error {
	c.mu.Lock()
	c.writes = append(c.writes, data)
	c.mu.Unlock()
	return nil
}

func (c *fakeConn) mtu() int { return 247 }

func (c *fakeConn) close() {
	c.mu.Lock()
	c.closed = true
	c.mu.Unlock()
}

func newFakeCentral(t *testing.T, r *fakeRadio) *Central {
	t.Helper()
	c := &Central{r: r, watchers: map[chan State]struct{}{}, names: map[string]string{}, busy: map[string]*Peripheral{}, abandoned: map[string]chan struct{}{}}
	if err := r.open(c.setState); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.Close)
	return c
}

func TestScanFiltersAndRemembersNames(t *testing.T) {
	r := &fakeRadio{adverts: []Advert{
		newAdvert("AA:BB:CC:DD:EE:01", "other", -50, []string{"180F"}, nil, nil),
		newAdvert("AA:BB:CC:DD:EE:02", "ASHDJW7CF49200487", -40, []string{"FF09"}, map[uint16][]byte{0xFFFF: {9: 0x26}}, nil),
	}}
	c := newFakeCentral(t, r)
	got := make(chan Advert, 4)
	if err := c.Scan([]string{"FF09"}, func(a Advert) { got <- a }); err != nil {
		t.Fatal(err)
	}
	select {
	case a := <-got:
		if a.ID != "AA:BB:CC:DD:EE:02" || a.ManufacturerData[0xFFFF][9] != 0x26 {
			t.Fatalf("advert %+v", a)
		}
	case <-time.After(time.Second):
		t.Fatal("no advert")
	}
	c.StopScan()
	select {
	case a := <-got:
		t.Fatalf("unfiltered advert %+v", a)
	case <-time.After(50 * time.Millisecond):
	}
	p, err := c.Connect(context.Background(), "aa:bb:cc:dd:ee:02")
	if err != nil {
		t.Fatal(err)
	}
	if p.ID() != "AA:BB:CC:DD:EE:02" || p.Name() != "ASHDJW7CF49200487" {
		t.Fatalf("id %q name %q", p.ID(), p.Name())
	}
}

func TestSessionNotificationsInOrder(t *testing.T) {
	r := &fakeRadio{}
	c := newFakeCentral(t, r)
	p, err := c.Connect(context.Background(), "AA:BB:CC:DD:EE:FF")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Connect(context.Background(), "AA:BB:CC:DD:EE:FF"); !errors.Is(err, ErrBusy) {
		t.Fatalf("second connect: %v", err)
	}
	ctx := context.Background()
	svcs, err := p.DiscoverServices(ctx, "8c850001-0302-41c5-b46e-cf057c562025")
	if err != nil || len(svcs) != 1 {
		t.Fatalf("services %v %v", svcs, err)
	}
	if _, err := p.DiscoverCharacteristics(ctx, "FFFF"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("undiscovered service: %v", err)
	}
	chars, err := p.DiscoverCharacteristics(ctx, svcs[0], "8c850002-0302-41c5-b46e-cf057c562025", "8c850003-0302-41c5-b46e-cf057c562025")
	if err != nil || len(chars) != 2 {
		t.Fatalf("chars %v %v", chars, err)
	}
	if n := p.MaxWriteLen(true); n != 244 {
		t.Fatalf("MaxWriteLen = %d", n)
	}
	var got []int
	done := make(chan struct{})
	const n = 5000
	err = p.Subscribe(ctx, "8C850003-0302-41C5-B46E-CF057C562025", func(b []byte) {
		got = append(got, int(b[0])<<8|int(b[1]))
		if len(got) == n {
			close(done)
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	cn := r.last
	buf := make([]byte, 2)
	for i := range n {
		buf[0], buf[1] = byte(i>>8), byte(i)
		cn.notify(buf) // the stack reuses its buffer
	}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatalf("delivered %d", len(got))
	}
	for i, v := range got {
		if v != i {
			t.Fatalf("got[%d] = %d", i, v)
		}
	}
	if err := p.Write("8c850002-0302-41c5-b46e-cf057c562025", make([]byte, 245), false); !errors.Is(err, ErrTooLong) {
		t.Fatalf("long write: %v", err)
	}
	if err := p.Write("8c850002-0302-41c5-b46e-cf057c562025", make([]byte, 193), false); err != nil {
		t.Fatal(err)
	}

	cn.lost(nil)
	<-p.Done()
	if !errors.Is(p.Err(), ErrDisconnected) {
		t.Fatalf("Err = %v", p.Err())
	}
	if err := p.Write("8c850002-0302-41c5-b46e-cf057c562025", []byte{1}, false); !errors.Is(err, ErrDisconnected) {
		t.Fatalf("write after loss: %v", err)
	}
	p2, err := c.Connect(ctx, "AA:BB:CC:DD:EE:FF")
	if err != nil {
		t.Fatalf("reconnect: %v", err)
	}
	p2.Disconnect()
	if !errors.Is(p2.Err(), ErrClosed) || !r.last.closed {
		t.Fatalf("after Disconnect: %v closed=%v", p2.Err(), r.last.closed)
	}
}

func TestConnectCancelAndPowerOff(t *testing.T) {
	r := &fakeRadio{block: make(chan struct{})}
	c := newFakeCentral(t, r)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if _, err := c.Connect(ctx, "AA:BB:CC:DD:EE:FF"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("cancelled connect: %v", err)
	}
	close(r.block)
	p, err := c.Connect(context.Background(), "AA:BB:CC:DD:EE:FF")
	if err != nil {
		t.Fatalf("connect after cancel: %v", err)
	}
	r.set(StatePoweredOff)
	select {
	case <-p.Done():
	case <-time.After(time.Second):
		t.Fatal("still connected with the radio off")
	}
	if !errors.Is(p.Err(), ErrDisconnected) {
		t.Fatalf("Err = %v", p.Err())
	}
	if _, err := c.Connect(context.Background(), "not-an-address"); !errors.Is(err, ErrUnknownPeripheral) {
		t.Fatalf("bad id: %v", err)
	}
}
