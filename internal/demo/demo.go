// Package demo is a made-up backend: a charger and a power bank added and
// connected, numbers that move every second, a scan that finds them and
// two more, covers that push with progress, and a library of pictures
// drawn in code. It reads no radio and writes no files, for trying the
// app, the screenshots and the tests. It words nothing: failures are the
// sentinels of package link and store, toasts store.NoticeKinds.
package demo

import (
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/LYJW131/anker-prime-mac/internal/cloud"
	"github.com/LYJW131/anker-prime-mac/internal/cover"
	"github.com/LYJW131/anker-prime-mac/internal/link"
	"github.com/LYJW131/anker-prime-mac/internal/model"
	"github.com/LYJW131/anker-prime-mac/internal/settings"
	"github.com/LYJW131/anker-prime-mac/internal/store"
)

// The demo's peripherals, named as the real devices advertise.
const (
	ChargerName   = "ASHDJW7CF49200487"
	PowerBankName = "AJ7DLCH0F49600286"
	ChargerID     = "6F1C2A40-8B7E-4C1D-9E0A-3A5B7C2D1E01"
	PowerBankID   = "0B9D4E21-5A3C-47F2-8C6E-1D2F3A4B5C02"

	// OtherChargerName and OtherChargerID are a charger the scan finds
	// that is not added yet.
	OtherChargerName = "ASHDJW7CF49200112"
	OtherChargerID   = "C4A12A40-8B7E-4C1D-9E0A-3A5B7C2D1E07"
	// UnknownName and UnknownID are an Anker Prime device of a model the
	// app does not support, model byte 0x0B.
	UnknownName = "AK0BQ5E2F49100753"
	UnknownID   = "9E3B7C11-2D4F-4A6B-8C0E-5F1A2B3C4D0B"
)

// Backend is the demo. Everything it changes, it changes on the UI thread,
// through the store's Post; its goroutines only sleep and post.
type Backend struct {
	s   *store.Store
	now func() time.Time
	t0  time.Time

	// devs are the demo's state of each added device.
	devs map[string]*demoDev
	// held devices keep their state as it is: the screenshots set a
	// phase and the ticks leave it.
	held map[string]bool
	// gen counts connection sequences and scans, so a late step of an
	// older one is dropped.
	gen map[string]int

	jpegs  map[int][]byte
	nextID int
	stop   chan struct{}
	// Instant posts every delayed step at once, without waiting, and
	// starts no ticks: for tests, whose post decides when steps run.
	Instant bool
}

// demoDev is what the demo makes up for one device.
type demoDev struct {
	m       model.Model
	phase   float64 // seconds its waves are shifted by
	battery float64
	// onDevice are the cover ids a charger holds, newest last.
	onDevice []int
}

// New returns the demo backend.
func New() *Backend {
	return &Backend{now: time.Now, devs: map[string]*demoDev{}, held: map[string]bool{}, gen: map[string]int{},
		jpegs: map[int][]byte{}, nextID: 1001}
}

// Start adds the demo's charger and power bank when no device is added,
// fills the store with them connected, a couple of minutes of history and
// the cover library, and moves the numbers every second.
func (b *Backend) Start(s *store.Store) {
	b.s = s
	b.now = s.Now
	b.t0 = b.now()
	s.HasUserID = true
	for _, art := range arts {
		data := art.jpeg()
		id := b.nextID
		b.nextID++
		b.jpegs[id] = data
		s.Covers = append(s.Covers, cover.Item{ID: id, Hash: crc(data), Name: art.name, File: art.file(), Size: len(data),
			AddedAt: b.t0.Add(-time.Duration(len(arts)-len(s.Covers)) * 26 * time.Hour)})
	}
	if len(s.Settings.Devices) == 0 {
		s.Settings.Devices = []settings.Device{
			{ID: ChargerID, Model: string(model.A2687), Name: ChargerName, Enabled: true},
			{ID: PowerBankID, Model: string(model.A110G), Name: PowerBankName, Enabled: true, IdleSleep: true},
		}
	}
	now := b.now()
	for _, d := range s.Settings.Devices {
		dd := b.add(d)
		if dd == nil {
			continue
		}
		b.seed(d.ID)
		// A couple of minutes of history, so the sparklines open full.
		for i := store.HistoryLen; i > 0; i-- {
			b.at(d.ID, now.Add(-time.Duration(i)*time.Second), false)
		}
		b.follow(d, true)
	}
	b.stop = make(chan struct{})
	if !b.Instant {
		go b.run()
	}
}

// add makes up a device of a supported model.
func (b *Backend) add(d settings.Device) *demoDev {
	m, ok := model.ByID(model.ID(d.Model))
	if !ok {
		return nil
	}
	if dd := b.devs[d.ID]; dd != nil {
		return dd
	}
	n := float64(len(b.devs))
	dd := &demoDev{m: m, phase: 37 * n, battery: 47.39 - 9*n}
	if m.Covers {
		for i, it := range b.s.Covers {
			if i < 4 {
				dd.onDevice = append(dd.onDevice, it.ID)
			}
		}
	}
	b.devs[d.ID] = dd
	return dd
}

// Close stops the ticks.
func (b *Backend) Close() {
	if b.stop != nil {
		close(b.stop)
	}
}

func (b *Backend) run() {
	t := time.NewTicker(time.Second)
	defer t.Stop()
	for {
		select {
		case <-b.stop:
			return
		case <-t.C:
			b.s.Post(b.tick)
		}
	}
}

// Tick moves the numbers one second on.
func (b *Backend) Tick() { b.tick() }

func (b *Backend) tick() {
	now := b.now()
	for id := range b.devs {
		if b.flowing(id) {
			b.at(id, now, true)
			b.s.Device(id).Status.LastFrame = now
		}
	}
	if b.s.Scanning {
		for i := range b.s.Found {
			b.s.Found[i].RSSI += int(math.Round(math.Sin(float64(now.UnixNano())/7e8+float64(i)) * 2))
			b.s.Found[i].RSSI = min(max(b.s.Found[i].RSSI, -95), -38)
		}
	}
}

func (b *Backend) flowing(id string) bool {
	d, ok := b.s.Devices[id]
	return ok && !b.held[id] && d.Status.Phase == link.PhaseConnected
}

// Hold keeps a device's state as it is, or lets the ticks move it again.
func (b *Backend) Hold(id string, hold bool) { b.held[id] = hold }

// later runs fn on the UI thread after d, unless gen moved on.
func (b *Backend) later(d time.Duration, key string, fn func()) {
	g := b.gen[key]
	step := func() {
		if b.gen[key] == g {
			fn()
		}
	}
	if b.Instant {
		// Timeouts never come in an instant run.
		if d < 10*time.Second {
			b.s.Post(step)
		}
		return
	}
	go func() {
		time.Sleep(d)
		b.s.Post(step)
	}()
}

// follow puts a device in the phase its settings call for, connecting it
// when it should be. warm skips the steps of a connection, as at launch.
func (b *Backend) follow(d settings.Device, warm bool) {
	if b.add(d) == nil {
		return
	}
	st := &b.s.Device(d.ID).Status
	key := "link-" + d.ID
	b.gen[key]++
	switch {
	case !d.Enabled:
		*st = link.Status{Phase: link.PhaseOff, Name: d.Name}
	case warm:
		*st = link.Status{Phase: link.PhaseConnected, LastFrame: b.now(), Name: d.Name}
	default:
		b.connect(d.ID, d.Name)
	}
}

// connect runs a connection's steps: connecting, the handshake, then the
// first frame.
func (b *Backend) connect(id, name string) {
	st := &b.s.Device(id).Status
	key := "link-" + id
	b.gen[key]++
	last := st.LastFrame
	*st = link.Status{Phase: link.PhaseConnecting, Name: name, LastFrame: last}
	b.later(900*time.Millisecond, key, func() {
		b.s.Device(id).Status.Phase = link.PhaseHandshaking
		b.later(700*time.Millisecond, key, func() {
			s := &b.s.Device(id).Status
			s.Phase, s.LastFrame, s.Err = link.PhaseConnected, b.now(), nil
			b.seed(id)
			b.at(id, b.now(), true)
		})
	})
}

// Connect connects a device again.
func (b *Backend) Connect(id string) {
	d, ok := b.s.Settings.Device(id)
	if !ok || !d.Enabled {
		return
	}
	b.connect(id, d.Name)
}

// Disconnect drops a device until the next Connect.
func (b *Backend) Disconnect(id string) {
	b.gen["link-"+id]++
	st := &b.s.Device(id).Status
	st.Phase, st.Err = link.PhaseDisconnected, link.ErrManual
}

// Scan finds the demo's two devices, then a charger not added yet, then a
// Prime device of a model the app does not know.
func (b *Backend) Scan() {
	b.gen["scan"]++
	b.s.Scanning, b.s.Found = true, nil
	found := func(f link.Found) {
		_, f.Added = b.s.Settings.Device(f.ID)
		b.s.Found = append(b.s.Found, f)
	}
	b.later(150*time.Millisecond, "scan", func() {
		found(link.Found{ID: ChargerID, Name: ChargerName, RSSI: -52, Model: string(model.A2687), ModelByte: 0x05})
		found(link.Found{ID: PowerBankID, Name: PowerBankName, RSSI: -58, Model: string(model.A110G), ModelByte: 0x0A})
		b.later(900*time.Millisecond, "scan", func() {
			found(link.Found{ID: OtherChargerID, Name: OtherChargerName, RSSI: -71, Model: string(model.A2687), ModelByte: 0x05})
			b.later(800*time.Millisecond, "scan", func() {
				found(link.Found{ID: UnknownID, Name: UnknownName, RSSI: -79, ModelByte: 0x0B})
			})
		})
	})
	b.later(30*time.Second, "scan", b.StopScan)
}

// StopScan ends the scan; what it found stays listed.
func (b *Backend) StopScan() {
	b.gen["scan"]++
	b.s.Scanning = false
}

// SaveSettings follows the devices added, removed and switched.
func (b *Backend) SaveSettings(next settings.Settings) {
	prev := b.s.Settings
	b.s.Settings = next
	for _, d := range next.Devices {
		p, had := prev.Device(d.ID)
		switch {
		case !had:
			b.follow(d, false)
		case p.Enabled != d.Enabled:
			b.follow(d, false)
		}
	}
	for _, p := range prev.Devices {
		if _, ok := next.Device(p.ID); !ok {
			b.gen["link-"+p.ID]++
			delete(b.s.Devices, p.ID)
			delete(b.devs, p.ID)
		}
	}
	for i := range b.s.Found {
		_, b.s.Found[i].Added = next.Device(b.s.Found[i].ID)
	}
}

// SetUserID takes an ID of 40 characters.
func (b *Backend) SetUserID(id string, done func(error)) {
	b.later(300*time.Millisecond, "account", func() {
		if len(id) != 40 {
			done(store.ErrUserIDFormat)
			return
		}
		b.s.HasUserID = true
		done(nil)
	})
}

// ImportUserID always finds the old app's ID.
func (b *Backend) ImportUserID(done func(error)) {
	b.later(400*time.Millisecond, "account", func() {
		b.s.HasUserID = true
		done(nil)
	})
}

// Login refuses the password "wrong" and takes any other.
func (b *Backend) Login(account, password string, done func(error)) {
	b.later(1200*time.Millisecond, "account", func() {
		if password == "wrong" {
			done(&cloud.Error{Code: 26006, Message: "wrong account or password"})
			return
		}
		b.s.HasUserID = true
		done(nil)
	})
}

// CoverData is a library picture's JPEG.
func (b *Backend) CoverData(it cover.Item) []byte { return b.jpegs[it.ID] }

// SelectCover shows a picture on a charger: at once when the charger
// holds it, else after pushing its chunks.
func (b *Backend) SelectCover(deviceID string, it cover.Item) {
	if j := b.s.CoverJob; j != nil && j.Err == nil {
		return
	}
	dd := b.devs[deviceID]
	if dd == nil || !dd.m.Covers {
		return
	}
	if !b.flowing(deviceID) && b.s.Device(deviceID).Status.Phase != link.PhaseConnected {
		b.s.Note(store.NoticeCoverFailed, it.Name, link.ErrNotConnected)
		return
	}
	b.gen["cover"]++
	held := slices.Contains(dd.onDevice, it.ID)
	done := func() {
		id := it.ID & 0xffff
		if ch := b.s.Device(deviceID).Charger; ch != nil {
			ch.ScreensaverID = &id
		}
		b.s.CoverJob = nil
		b.remember(dd, it.ID)
		b.s.Note(store.NoticeCoverShown, it.Name, nil)
	}
	if held {
		b.s.CoverJob = &link.CoverJob{ID: it.ID}
		b.later(350*time.Millisecond, "cover", done)
		return
	}
	total := max(1, (it.Size+479)/480)
	b.s.CoverJob = &link.CoverJob{ID: it.ID, Total: total}
	var step func()
	step = func() {
		j := b.s.CoverJob
		if j == nil {
			return
		}
		j.Done = min(j.Total, j.Done+3)
		if j.Done == j.Total {
			b.later(200*time.Millisecond, "cover", done)
			return
		}
		b.later(120*time.Millisecond, "cover", step)
	}
	b.later(300*time.Millisecond, "cover", step)
}

// remember notes that a charger holds id, forgetting the oldest beyond
// four.
func (b *Backend) remember(dd *demoDev, id int) {
	out := dd.onDevice[:0]
	for _, x := range dd.onDevice {
		if x != id {
			out = append(out, x)
		}
	}
	dd.onDevice = append(out, id)
	if len(dd.onDevice) > 4 {
		dd.onDevice = dd.onDevice[len(dd.onDevice)-4:]
	}
}

// AddCover reads a picture file off the UI thread and adds it, cropped to
// a square of 240.
func (b *Backend) AddCover(path string) {
	work := func() ([]byte, error) {
		f, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		return squareJPEG(f)
	}
	add := func(data []byte, err error) {
		if err != nil {
			b.s.Note(store.NoticeCoverAddFailed, "", err)
			return
		}
		id := b.nextID
		b.nextID++
		b.jpegs[id] = data
		name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		b.s.Covers = append(b.s.Covers, cover.Item{ID: id, Hash: crc(data), Name: name, File: coverFile(id), Size: len(data), AddedAt: b.now()})
		b.s.Note(store.NoticeCoverAdded, name, nil)
	}
	if b.Instant {
		data, err := work()
		b.s.Post(func() { add(data, err) })
		return
	}
	go func() {
		data, err := work()
		b.s.Post(func() { add(data, err) })
	}()
}

// RemoveCover drops a picture from the library.
func (b *Backend) RemoveCover(id int) {
	out := b.s.Covers[:0]
	for _, it := range b.s.Covers {
		if it.ID != id {
			out = append(out, it)
		}
	}
	b.s.Covers = out
	delete(b.jpegs, id)
}

// RenameCover renames a picture.
func (b *Backend) RenameCover(id int, name string) {
	for i := range b.s.Covers {
		if b.s.Covers[i].ID == id {
			b.s.Covers[i].Name = name
		}
	}
}
