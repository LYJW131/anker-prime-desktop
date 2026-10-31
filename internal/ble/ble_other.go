//go:build !darwin && !windows && !linux

package ble

import (
	"context"
	"errors"
)

// Central is unavailable on this system.
type Central struct{}

// Peripheral is unavailable on this system.
type Peripheral struct{}

// ScanOption changes how Scan reports advertisements.
type ScanOption func()

// AllowDuplicates reports every advertisement of a peripheral.
func AllowDuplicates() ScanOption { return func() {} }

// NewCentral returns errors.ErrUnsupported.
func NewCentral() (*Central, error) { return nil, errors.ErrUnsupported }

// CurrentAuthorization returns errors.ErrUnsupported.
func CurrentAuthorization() (Authorization, error) { return AuthNotDetermined, errors.ErrUnsupported }

func (c *Central) State() State                 { return StateUnsupported }
func (c *Central) Authorization() Authorization { return AuthNotDetermined }

func (c *Central) WatchState() (<-chan State, func()) {
	ch := make(chan State, 1)
	ch <- StateUnsupported
	return ch, func() {}
}

func (c *Central) WaitPoweredOn(context.Context) error { return errors.ErrUnsupported }

func (c *Central) Scan([]string, func(Advert), ...ScanOption) error { return errors.ErrUnsupported }
func (c *Central) StopScan()                                        {}

func (c *Central) RetrieveConnected(string) ([]Device, error) { return nil, errors.ErrUnsupported }

func (c *Central) Connect(context.Context, string) (*Peripheral, error) {
	return nil, errors.ErrUnsupported
}

func (c *Central) Close() {}

func (p *Peripheral) ID() string   { return "" }
func (p *Peripheral) Name() string { return "" }

func (p *Peripheral) Done() <-chan struct{} {
	ch := make(chan struct{})
	close(ch)
	return ch
}

func (p *Peripheral) Err() error { return errors.ErrUnsupported }

func (p *Peripheral) DiscoverServices(context.Context, ...string) ([]string, error) {
	return nil, errors.ErrUnsupported
}

func (p *Peripheral) DiscoverCharacteristics(context.Context, string, ...string) ([]Characteristic, error) {
	return nil, errors.ErrUnsupported
}

func (p *Peripheral) Subscribe(context.Context, string, func([]byte)) error {
	return errors.ErrUnsupported
}

func (p *Peripheral) Unsubscribe(context.Context, string) error { return errors.ErrUnsupported }

func (p *Peripheral) MaxWriteLen(bool) int { return 0 }

func (p *Peripheral) Write(string, []byte, bool) error { return errors.ErrUnsupported }

func (p *Peripheral) WriteContext(context.Context, string, []byte, bool) error {
	return errors.ErrUnsupported
}

func (p *Peripheral) Disconnect() {}
