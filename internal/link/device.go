package link

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/LYJW131/anker-prime-desktop/internal/ble"
	"github.com/LYJW131/anker-prime-desktop/internal/model"
	"github.com/LYJW131/anker-prime-desktop/internal/settings"
)

// Timing of the connection loop.
const (
	retryDelay    = 5 * time.Second  // after a failed or lost connection
	rearmAfter    = 10 * time.Second // of silence: ask for the stream again, once
	stallAfter    = 20 * time.Second // of silence: drop the connection
	idleAfter     = 5 * time.Minute  // power bank with nothing flowing: release it
	probeIdle     = 20 * time.Second // a wake-up probe that finds it still idle
	sleepFirst    = 5 * time.Minute  // first sleep, doubled up to sleepLongest
	sleepLongest  = 30 * time.Minute
	selectSettle  = 4 * time.Second // a select of pixels already on the charger
	pushSettle    = 6 * time.Second // the screen switching after a push
	busyThreshold = 0.05            // watts that count as flowing
)

var (
	errIdle    = errors.New("idle")
	errChanged = errors.New("settings changed")
)

// Config wires a Device to the radio and to whoever shows its state. The
// callbacks run on the Device's goroutine and must not block.
type Config struct {
	Model   model.Model
	Central *ble.Central
	Scanner *Scanner
	UserID  func() string // the 40-character Anker account ID, "" when unknown
	Log     *slog.Logger

	OnStatus func(Status)
	OnState  func(state any) // a new snapshot from the model's decoder (device.Charger, device.PowerBank)
}

// Device keeps one device connected while its settings ask for it.
type Device struct {
	cfg Config

	mu      sync.Mutex
	want    settings.Device
	manual  bool          // the user disconnected it
	changed chan struct{} // closed and replaced on every change
	status  Status
	jobs    chan job
}

// New returns a Device; Run starts it.
func New(cfg Config) *Device {
	if cfg.Log == nil {
		cfg.Log = slog.Default()
	}
	return &Device{cfg: cfg, changed: make(chan struct{}), jobs: make(chan job)}
}

// Configure follows new settings for this device.
func (d *Device) Configure(s settings.Device) {
	d.mu.Lock()
	moved := s.ID != d.want.ID || s.Enabled != d.want.Enabled
	d.want = s
	if moved {
		d.manual = false
	}
	d.mu.Unlock()
	d.poke()
}

// Connect connects again now, after a Disconnect or while a retry waits.
func (d *Device) Connect() {
	d.mu.Lock()
	d.manual = false
	d.mu.Unlock()
	d.poke()
}

// Disconnect drops the connection until Connect or a settings change.
func (d *Device) Disconnect() {
	d.mu.Lock()
	d.manual = true
	d.mu.Unlock()
	d.poke()
}

// Wake reconnects at once if the device is waiting to retry or asleep.
func (d *Device) Wake() { d.poke() }

func (d *Device) poke() {
	d.mu.Lock()
	close(d.changed)
	d.changed = make(chan struct{})
	d.mu.Unlock()
}

func (d *Device) snapshot() (settings.Device, bool, <-chan struct{}) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.want, d.manual, d.changed
}

func (d *Device) setStatus(fn func(*Status)) {
	d.mu.Lock()
	fn(&d.status)
	s := d.status
	d.mu.Unlock()
	if d.cfg.OnStatus != nil {
		d.cfg.OnStatus(s)
	}
}

func (d *Device) phase(p Phase, err error) {
	d.setStatus(func(s *Status) { s.Phase, s.Err = p, err })
}

// Run keeps the device connected until ctx ends.
func (d *Device) Run(ctx context.Context) {
	sleep := sleepFirst
	probing := false
	for ctx.Err() == nil {
		want, manual, changed := d.snapshot()
		wait := func(dur time.Duration) {
			var t <-chan time.Time
			if dur > 0 {
				timer := time.NewTimer(dur)
				defer timer.Stop()
				t = timer.C
			}
			select {
			case <-ctx.Done():
			case <-changed:
			case <-t:
			}
		}
		switch {
		case !want.Enabled:
			d.phase(PhaseOff, nil)
			wait(0)
			continue
		case want.ID == "":
			d.phase(PhaseUnpaired, nil)
			wait(0)
			continue
		case manual:
			d.phase(PhaseDisconnected, ErrManual)
			wait(0)
			continue
		}
		uid := ""
		if d.cfg.UserID != nil {
			uid = d.cfg.UserID()
		}
		if len(uid) != 40 {
			d.phase(PhaseDisconnected, ErrNoUserID)
			wait(0)
			continue
		}
		if p, ok := d.radioBlocked(); ok {
			d.phase(p, nil)
			states, cancel := d.cfg.Central.WatchState()
			select {
			case <-ctx.Done():
			case <-changed:
			case <-states:
				<-time.After(200 * time.Millisecond)
			}
			cancel()
			continue
		}

		err := d.connectOnce(ctx, changed, want, uid, probing)
		switch {
		case ctx.Err() != nil:
			return
		case errors.Is(err, errChanged):
			continue
		case errors.Is(err, errIdle):
			d.cfg.Log.Info("idle, releasing", "sleep", sleep)
			d.phase(PhaseSleeping, nil)
			wait(sleep)
			sleep = min(sleep*2, sleepLongest)
			probing = true
			continue
		}
		sleep, probing = sleepFirst, false
		if err != nil {
			d.cfg.Log.Info("connection ended", "err", err)
		}
		d.phase(PhaseDisconnected, err)
		wait(retryDelay)
	}
}

func (d *Device) radioBlocked() (Phase, bool) {
	switch d.cfg.Central.Authorization() {
	case ble.AuthDenied, ble.AuthRestricted:
		return PhaseUnauthorized, true
	}
	switch d.cfg.Central.State() {
	case ble.StatePoweredOn:
		return 0, false
	case ble.StateUnauthorized:
		return PhaseUnauthorized, true
	case ble.StateUnknown, ble.StateResetting:
		// Not reported yet: still on the way to connecting.
		return PhaseConnecting, true
	}
	return PhaseBluetoothOff, true
}

// connectOnce connects, runs the session and returns why it ended.
func (d *Device) connectOnce(ctx context.Context, changed <-chan struct{}, want settings.Device, uid string, probing bool) error {
	cctx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() {
		select {
		case <-changed:
			cancel()
		case <-cctx.Done():
		}
	}()

	d.setStatus(func(s *Status) { s.Phase, s.Name = PhaseConnecting, want.Name })
	p, err := d.cfg.Central.Connect(cctx, want.ID)
	if errors.Is(err, ble.ErrUnknownPeripheral) {
		// The system forgot it (bluetoothd restarted, another Mac):
		// find it by its advertised identifier, then connect.
		if err = d.find(cctx, want.ID); err == nil {
			p, err = d.cfg.Central.Connect(cctx, want.ID)
		}
	}
	if err != nil {
		if cctx.Err() != nil && ctx.Err() == nil {
			return errChanged
		}
		return err
	}
	defer p.Disconnect()

	s := newSession(d, p, uid)
	err = s.run(cctx, probing)
	if cctx.Err() != nil && ctx.Err() == nil && !errors.Is(err, errIdle) {
		return errChanged
	}
	return err
}

func (d *Device) find(ctx context.Context, id string) error {
	found := make(chan struct{}, 1)
	stop, err := d.cfg.Scanner.Listen(func(a ble.Advert) {
		if strings.EqualFold(a.ID, id) {
			select {
			case found <- struct{}{}:
			default:
			}
		}
	})
	if err != nil {
		return err
	}
	defer stop()
	select {
	case <-found:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// job is work that must run inside the session, between frames.
type job struct {
	run  func(ctx context.Context, s *session) error
	done chan error
}

// do runs fn inside the connected session, or fails when there is none.
func (d *Device) do(ctx context.Context, fn func(ctx context.Context, s *session) error) error {
	j := job{run: fn, done: make(chan error, 1)}
	select {
	case d.jobs <- j:
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(2 * time.Second):
		return ErrNotConnected
	}
	select {
	case err := <-j.done:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// SelectCover shows picture id on the charger. When the charger answers
// that it lacks the pixels, or the screen does not switch, jpeg is pushed
// first. progress reports chunks sent of the total; it runs on the link's
// goroutine.
func (d *Device) SelectCover(ctx context.Context, id int, hash uint32, jpeg []byte, progress func(done, total int)) error {
	if !d.cfg.Model.Covers {
		return fmt.Errorf("model %s has no covers", d.cfg.Model.ID)
	}
	return d.do(ctx, func(ctx context.Context, s *session) error {
		return s.selectCover(ctx, uint32(id), hash, jpeg, progress)
	})
}
