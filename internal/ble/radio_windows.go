//go:build windows

package ble

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"time"
	"unsafe"

	"github.com/go-ole/go-ole"
	"github.com/saltosystems/winrt-go"
	"github.com/saltosystems/winrt-go/windows/foundation"
	"tinygo.org/x/bluetooth"

	btadapter "github.com/LYJW131/anker-prime-desktop/internal/ble/winrt/bluetooth"
	"github.com/LYJW131/anker-prime-desktop/internal/ble/winrt/radios"
)

// connectAttempt bounds one WinRT connection attempt; Connect retries
// until its context ends, as CoreBluetooth waits.
const connectAttempt = 15 * time.Second

// winRadio drives WinRT through tinygo's bluetooth package, plus the
// adapter's radio for the state.
type winRadio struct {
	adapter *bluetooth.Adapter
	quit    chan struct{}
	closed  chan struct{}

	mu    sync.Mutex
	conns map[bluetooth.MAC]*winConn

	// Only the apartment's goroutine touches these.
	radio   *radios.Radio
	handler *foundation.TypedEventHandler
	token   foundation.EventRegistrationToken
}

func newRadio() radio {
	return &winRadio{adapter: bluetooth.DefaultAdapter, quit: make(chan struct{}), closed: make(chan struct{}), conns: map[bluetooth.MAC]*winConn{}}
}

// open runs on the caller's thread, which may be MyGo's single-threaded UI
// thread: the WinRT work happens on a goroutine parked on a thread of the
// multithreaded apartment until close. While that thread exists every
// other thread of the process may call WinRT, and callbacks run on the
// apartment's thread pool.
func (r *winRadio) open(set func(State)) error {
	ready := make(chan error, 1)
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		defer close(r.closed)
		err := ole.RoInitialize(1) // RO_INIT_MULTITHREADED
		var oe *ole.OleError
		if errors.As(err, &oe) && oe.Code() == 1 { // S_FALSE: already initialized
			err = nil
		}
		if err != nil {
			ready <- fmt.Errorf("ble: windows runtime: %w", err)
			return
		}
		defer ole.CoUninitialize()
		if err := r.watch(set); err != nil {
			ready <- err
			return
		}
		ready <- nil
		<-r.quit
		r.unwatch()
	}()
	return <-ready
}

// watch reports the adapter's radio state now and on every change.
func (r *winRadio) watch(set func(State)) error {
	r.adapter.SetConnectHandler(r.connectionChanged)
	op, err := btadapter.BluetoothAdapterGetDefaultAsync()
	if err != nil {
		return fmt.Errorf("ble: bluetooth adapter: %w", err)
	}
	res, err := await(op, btadapter.SignatureBluetoothAdapter)
	if err != nil {
		return fmt.Errorf("ble: bluetooth adapter: %w", err)
	}
	if res == nil {
		set(StateUnsupported) // no adapter
		return nil
	}
	ad := (*btadapter.BluetoothAdapter)(res)
	defer ad.Release()
	le, err1 := ad.GetIsLowEnergySupported()
	central, err2 := ad.GetIsCentralRoleSupported()
	if err := errors.Join(err1, err2); err != nil {
		return fmt.Errorf("ble: bluetooth adapter: %w", err)
	}
	if !le || !central {
		set(StateUnsupported)
		return nil
	}

	op, err = ad.GetRadioAsync()
	if err != nil {
		return fmt.Errorf("ble: bluetooth radio: %w", err)
	}
	res, err = await(op, radios.SignatureRadio)
	if err != nil {
		return fmt.Errorf("ble: bluetooth radio: %w", err)
	}
	if res == nil {
		set(StatePoweredOn) // no radio object to watch: assume on
		return nil
	}
	rd := (*radios.Radio)(res)
	report := func() {
		if s, err := rd.GetState(); err == nil {
			set(radioState(s))
		}
	}
	// TypedEventHandler<Radio, Object>
	guid := winrt.ParameterizedInstanceGUID(foundation.GUIDTypedEventHandler, radios.SignatureRadio, "cinterface(IInspectable)")
	h := foundation.NewTypedEventHandler(ole.NewGUID(guid), func(*foundation.TypedEventHandler, unsafe.Pointer, unsafe.Pointer) { report() })
	token, err := rd.AddStateChanged(h)
	if err != nil {
		h.Release()
		rd.Release()
		return fmt.Errorf("ble: bluetooth radio: %w", err)
	}
	r.radio, r.handler, r.token = rd, h, token
	report()
	return nil
}

func (r *winRadio) unwatch() {
	if r.radio != nil {
		_ = r.radio.RemoveStateChanged(r.token)
		r.handler.Release()
		r.radio.Release()
		r.radio, r.handler = nil, nil
	}
}

func radioState(s radios.RadioState) State {
	switch s {
	case radios.RadioStateOn:
		return StatePoweredOn
	case radios.RadioStateOff, radios.RadioStateDisabled:
		return StatePoweredOff
	}
	return StateUnknown
}

// await waits for a WinRT IAsyncOperation and returns its result, nil for
// a null one.
func await(op *foundation.IAsyncOperation, resultSignature string) (unsafe.Pointer, error) {
	defer op.Release()
	iid := winrt.ParameterizedInstanceGUID(foundation.GUIDAsyncOperationCompletedHandler, resultSignature)
	done := make(chan foundation.AsyncStatus, 1)
	h := foundation.NewAsyncOperationCompletedHandler(ole.NewGUID(iid), func(_ *foundation.AsyncOperationCompletedHandler, _ *foundation.IAsyncOperation, status foundation.AsyncStatus) {
		done <- status
	})
	defer h.Release()
	if err := op.SetCompleted(h); err != nil {
		return nil, err
	}
	if status := <-done; status != foundation.AsyncStatusCompleted {
		return nil, fmt.Errorf("async operation ended with status %d", status)
	}
	return op.GetResults()
}

// connectionChanged runs on a WinRT thread.
func (r *winRadio) connectionChanged(d bluetooth.Device, connected bool) {
	if connected {
		return
	}
	r.mu.Lock()
	c := r.conns[d.Address.MAC]
	delete(r.conns, d.Address.MAC)
	r.mu.Unlock()
	if c != nil {
		c.lost(nil)
	}
}

func (r *winRadio) scan(stop <-chan struct{}, report func(Advert)) error {
	done := make(chan error, 1)
	go func() {
		done <- r.adapter.Scan(func(_ *bluetooth.Adapter, res bluetooth.ScanResult) {
			report(advertOf(res))
		})
	}()
	select {
	case err := <-done:
		return err
	case <-stop:
	}
	// StopScan fails until Scan has created its watcher.
	for r.adapter.StopScan() != nil {
		select {
		case err := <-done:
			return err
		case <-time.After(20 * time.Millisecond):
		}
	}
	return <-done
}

func advertOf(res bluetooth.ScanResult) Advert {
	var services []string
	for _, u := range res.ServiceUUIDs() {
		services = append(services, u.String())
	}
	var mfr map[uint16][]byte
	for _, m := range res.ManufacturerData() {
		if mfr == nil {
			mfr = map[uint16][]byte{}
		}
		mfr[m.CompanyID] = m.Data
	}
	var svcData map[string][]byte
	for _, s := range res.ServiceData() {
		if svcData == nil {
			svcData = map[string][]byte{}
		}
		svcData[s.UUID.String()] = s.Data
	}
	return newAdvert(formatAddress(res.Address.MAC.Address()), res.LocalName(), int(res.RSSI), services, mfr, svcData)
}

func (r *winRadio) connect(ctx context.Context, addr [6]byte, lost func(error)) (conn, error) {
	mac := bluetooth.MAC{addr[5], addr[4], addr[3], addr[2], addr[1], addr[0]}
	target := bluetooth.Address{MACAddress: bluetooth.MACAddress{MAC: mac}}
	for {
		type result struct {
			d   bluetooth.Device
			err error
		}
		res := make(chan result, 1)
		go func() {
			d, err := r.adapter.Connect(target, bluetooth.ConnectionParams{ConnectionTimeout: bluetooth.NewDuration(connectAttempt)})
			res <- result{d, err}
		}()
		var x result
		select {
		case x = <-res:
		case <-ctx.Done():
			go func() {
				if x := <-res; x.err == nil {
					_ = x.d.Disconnect()
				}
			}()
			return nil, ctx.Err()
		}
		switch {
		case x.err == nil:
		case strings.Contains(x.err.Error(), "was not found"):
			return nil, ErrUnknownPeripheral
		case strings.Contains(x.err.Error(), "timeout on Connect") && ctx.Err() == nil:
			continue
		default:
			return nil, x.err
		}
		c := &winConn{r: r, d: x.d, mac: mac, lost: lost, svcs: map[string]bluetooth.DeviceService{}, chars: map[string]bluetooth.DeviceCharacteristic{}}
		r.mu.Lock()
		r.conns[mac] = c
		r.mu.Unlock()
		if ok, err := x.d.Connected(); err != nil || !ok {
			c.close()
			if ctx.Err() == nil {
				continue
			}
			return nil, ctx.Err()
		}
		return c, nil
	}
}

func (r *winRadio) close() {
	close(r.quit)
	select {
	case <-r.closed:
	case <-time.After(disconnectTimeout):
	}
}

// winConn is one GATT session.
type winConn struct {
	r    *winRadio
	d    bluetooth.Device
	mac  bluetooth.MAC
	lost func(error)

	mu    sync.Mutex
	svcs  map[string]bluetooth.DeviceService
	chars map[string]bluetooth.DeviceCharacteristic
	once  sync.Once
}

func (c *winConn) services(context.Context) ([]string, error) {
	ss, err := c.d.DiscoverServices(nil)
	if err != nil {
		return nil, err
	}
	var out []string
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, s := range ss {
		u := NormalizeUUID(s.UUID().String())
		c.svcs[u] = s
		out = append(out, u)
	}
	return out, nil
}

func (c *winConn) characteristics(_ context.Context, service string) ([]Characteristic, error) {
	c.mu.Lock()
	s, ok := c.svcs[service]
	c.mu.Unlock()
	if !ok {
		return nil, ErrNotFound
	}
	cs, err := s.DiscoverCharacteristics(nil)
	if err != nil {
		return nil, err
	}
	var out []Characteristic
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, ch := range cs {
		u := NormalizeUUID(ch.UUID().String())
		c.chars[u] = ch
		// GattCharacteristicProperties shares CoreBluetooth's low bits.
		out = append(out, Characteristic{UUID: u, Service: service, Properties: Property(ch.Properties() & 0x3f)})
	}
	return out, nil
}

func (c *winConn) char(uuid string) (bluetooth.DeviceCharacteristic, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	ch, ok := c.chars[uuid]
	if !ok {
		return bluetooth.DeviceCharacteristic{}, ErrNotFound
	}
	return ch, nil
}

func (c *winConn) subscribe(_ context.Context, uuid string, fn func([]byte)) error {
	ch, err := c.char(uuid)
	if err != nil {
		return err
	}
	return ch.EnableNotifications(fn)
}

func (c *winConn) unsubscribe(_ context.Context, uuid string) error {
	ch, err := c.char(uuid)
	if err != nil {
		return err
	}
	return ch.EnableNotifications(nil)
}

func (c *winConn) write(_ context.Context, uuid string, data []byte, withResponse bool) error {
	ch, err := c.char(uuid)
	if err != nil {
		return err
	}
	if withResponse {
		_, err = ch.Write(data)
	} else {
		_, err = ch.WriteWithoutResponse(data)
	}
	return err
}

// mtu is the GATT session's MaxPduSize, the negotiated ATT MTU.
func (c *winConn) mtu() int {
	c.mu.Lock()
	var one bluetooth.DeviceCharacteristic
	for _, ch := range c.chars {
		one = ch
		break
	}
	c.mu.Unlock()
	if one == (bluetooth.DeviceCharacteristic{}) {
		return 0
	}
	n, err := one.GetMTU()
	if err != nil {
		return 0
	}
	return int(n)
}

func (c *winConn) close() {
	c.once.Do(func() {
		c.r.mu.Lock()
		if c.r.conns[c.mac] == c {
			delete(c.r.conns, c.mac)
		}
		c.r.mu.Unlock()
		_ = c.d.Disconnect()
	})
}
