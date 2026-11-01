// Package backend is the real Backend: the added devices over Bluetooth,
// the cover library and the Anker account ID on disk.
package backend

import (
	"context"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo"

	"github.com/LYJW131/anker-prime-desktop/internal/ble"
	"github.com/LYJW131/anker-prime-desktop/internal/cloud"
	"github.com/LYJW131/anker-prime-desktop/internal/cover"
	"github.com/LYJW131/anker-prime-desktop/internal/device"
	"github.com/LYJW131/anker-prime-desktop/internal/link"
	"github.com/LYJW131/anker-prime-desktop/internal/model"
	"github.com/LYJW131/anker-prime-desktop/internal/settings"
	"github.com/LYJW131/anker-prime-desktop/internal/store"
)

// userIDKey is the secret store item earlier versions kept the Anker
// account ID in, which Start moves to userIDFile.
const userIDKey = "anker-user-id"

// The telemetry app's Keychain item, which ImportUserID copies.
const (
	telemetryService = "com.liangyangjunwei.MacTelemetryHub"
	telemetryAccount = "anker-user-id"
)

// scanFor is how long an add-device scan runs.
const scanFor = 30 * time.Second

// Backend drives the devices. Its exported methods run on the UI thread;
// everything slow runs on goroutines that post their results.
type Backend struct {
	log *slog.Logger
	s   *store.Store

	central *ble.Central
	scanner *link.Scanner
	links   map[string]*running // by device id
	lib     *cover.Library

	dir      string // the user data directory
	mu       sync.Mutex
	userID   string
	stopScan func()
	scanGen  int
	jpegs    map[int][]byte // CoverData's cache
}

// running is one device's link and the way to stop it.
type running struct {
	link *link.Device
	stop context.CancelFunc
}

// New returns the backend.
func New(log *slog.Logger) *Backend {
	if log == nil {
		log = slog.Default()
	}
	return &Backend{log: log, links: map[string]*running{}, jpegs: map[int][]byte{}}
}

// Start opens the library, reads the account ID and starts a link per
// added device.
func (b *Backend) Start(s *store.Store) {
	b.s = s
	dir, err := mygo.App.Path(mygo.PathUserData)
	if err == nil {
		b.dir = dir
		b.lib, err = cover.Open(filepath.Join(dir, "covers"))
	}
	if err != nil {
		b.log.Error("cover library", "error", err)
		s.Note(store.NoticeLibraryUnavailable, "", err)
	} else {
		s.Covers = b.lib.List()
	}

	// ANKER_USER_ID overrides the stored ID, for development.
	id := os.Getenv("ANKER_USER_ID")
	if id == "" && b.dir != "" {
		id = loadUserID(b.dir, func() ([]byte, error) { return mygo.Secrets.Get(userIDKey) },
			func() error { return mygo.Secrets.Delete(userIDKey) })
	}
	b.userID = id
	s.HasUserID = len(id) == 40

	b.central, err = ble.NewCentral()
	if err != nil {
		b.log.Error("bluetooth", "error", err)
		s.Note(store.NoticeBluetoothUnavailable, "", err)
		return
	}
	b.scanner = link.NewScanner(b.central)
	b.SaveSettings(s.Settings)
}

// SaveSettings starts links for added devices, stops those of removed
// ones and has the others follow their settings.
func (b *Backend) SaveSettings(next settings.Settings) {
	if b.central == nil {
		return
	}
	keep := map[string]bool{}
	for _, d := range next.Devices {
		m, ok := model.ByID(model.ID(d.Model))
		if !ok {
			continue
		}
		keep[d.ID] = true
		r := b.links[d.ID]
		if r == nil {
			ctx, cancel := context.WithCancel(context.Background())
			r = &running{link: b.newLink(d.ID, m), stop: cancel}
			b.links[d.ID] = r
			go r.link.Run(ctx)
		}
		if !m.IdleSleep {
			d.IdleSleep = false
		}
		r.link.Configure(d)
	}
	for id, r := range b.links {
		if !keep[id] {
			r.stop()
			delete(b.links, id)
			delete(b.s.Devices, id)
		}
	}
}

func (b *Backend) newLink(id string, m model.Model) *link.Device {
	post := func(fn func(*store.Device)) {
		b.s.Post(func() {
			if _, added := b.s.Settings.Device(id); added {
				fn(b.s.Device(id))
			}
		})
	}
	return link.New(link.Config{
		Model:    m,
		Central:  b.central,
		Scanner:  b.scanner,
		UserID:   b.currentUserID,
		Log:      b.log.With("device", id[:min(8, len(id))], "model", string(m.ID)),
		OnStatus: func(st link.Status) { post(func(d *store.Device) { d.Status = st }) },
		OnState: func(state any) {
			post(func(d *store.Device) {
				switch st := state.(type) {
				case device.Charger:
					st = st.ForDisplay()
					d.Charger = &st
					if st.TotalOutputPowerW != nil {
						d.Watts = store.Push(d.Watts, store.Sample{At: b.s.Now(), Value: *st.TotalOutputPowerW})
					}
				case device.PowerBank:
					st = st.ForDisplay()
					d.PowerBank = &st
					if st.InputPowerW != nil && st.OutputPowerW != nil {
						d.Watts = store.Push(d.Watts, store.Sample{At: b.s.Now(), Value: *st.InputPowerW - *st.OutputPowerW})
					}
				}
			})
		},
	})
}

func (b *Backend) currentUserID() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.userID
}

// Connect connects a device again now.
func (b *Backend) Connect(id string) {
	if r := b.links[id]; r != nil {
		r.link.Connect()
	}
}

// Disconnect drops a device until Connect.
func (b *Backend) Disconnect(id string) {
	if r := b.links[id]; r != nil {
		r.link.Disconnect()
	}
}

// Scan lists the Anker Prime devices in range, supported or not.
func (b *Backend) Scan() {
	b.StopScan()
	if b.scanner == nil {
		b.s.Note(store.NoticeBluetoothUnavailable, "", link.ErrNotConnected)
		return
	}
	b.s.Scanning, b.s.Found = true, nil
	stop, err := b.scanner.Listen(func(a ble.Advert) {
		f, ok := link.Classify(a)
		if !ok {
			return
		}
		b.s.Post(func() {
			if !b.s.Scanning {
				return
			}
			_, f.Added = b.s.Settings.Device(f.ID)
			if i := slices.IndexFunc(b.s.Found, func(x link.Found) bool { return x.ID == f.ID }); i >= 0 {
				b.s.Found[i] = f
				return
			}
			b.s.Found = append(b.s.Found, f)
		})
	})
	if err != nil {
		b.s.Scanning = false
		b.s.Note(store.NoticeBluetoothUnavailable, "", err)
		return
	}
	b.mu.Lock()
	b.stopScan = stop
	b.scanGen++
	gen := b.scanGen
	b.mu.Unlock()
	time.AfterFunc(scanFor, func() {
		b.s.Post(func() {
			b.mu.Lock()
			current := b.scanGen == gen
			b.mu.Unlock()
			if current && b.s.Scanning {
				b.StopScan()
			}
		})
	})
}

// StopScan ends the add-device scan.
func (b *Backend) StopScan() {
	b.mu.Lock()
	stop := b.stopScan
	b.stopScan = nil
	b.mu.Unlock()
	if stop != nil {
		stop()
	}
	b.s.Scanning = false
}

// SetUserID stores the account ID.
func (b *Backend) SetUserID(id string, done func(error)) {
	id = strings.TrimSpace(id)
	go func() {
		err := b.storeUserID(id)
		b.s.Post(func() { done(err) })
	}()
}

func (b *Backend) storeUserID(id string) error {
	if !validUserID(id) {
		return store.ErrUserIDFormat
	}
	if b.dir == "" {
		return store.ErrNoDataDir
	}
	if err := writeUserID(b.dir, id); err != nil {
		return err
	}
	b.mu.Lock()
	b.userID = id
	b.mu.Unlock()
	b.s.Post(func() {
		b.s.HasUserID = true
		for _, r := range b.links {
			r.link.Wake()
		}
	})
	return nil
}

// ImportUserID copies the account ID from the telemetry app's Keychain
// item. macOS may ask once whether this app may read it.
func (b *Backend) ImportUserID(done func(error)) {
	if runtime.GOOS != "darwin" {
		b.s.Post(func() { done(store.ErrImportNotFound) })
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		out, err := exec.CommandContext(ctx, "/usr/bin/security", "find-generic-password", "-s", telemetryService, "-a", telemetryAccount, "-w").Output()
		if err != nil {
			err = store.ErrImportNotFound
		} else {
			err = b.storeUserID(strings.TrimSpace(string(out)))
		}
		b.s.Post(func() { done(err) })
	}()
}

// Login signs in once to learn the account ID. The password and token are
// not kept. Signing in can sign the phone app out.
func (b *Backend) Login(account, password string, done func(error)) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
		defer cancel()
		sess, err := (&cloud.Client{}).Login(ctx, account, password)
		if err == nil {
			err = b.storeUserID(sess.UserID)
		}
		b.s.Post(func() { done(err) })
	}()
}

// CoverData is a library picture's JPEG, read once.
func (b *Backend) CoverData(it cover.Item) []byte {
	b.mu.Lock()
	data, ok := b.jpegs[it.ID]
	b.mu.Unlock()
	if ok || b.lib == nil {
		return data
	}
	data, _ = b.lib.Bytes(it.ID)
	b.mu.Lock()
	b.jpegs[it.ID] = data
	b.mu.Unlock()
	return data
}

// SelectCover shows it on the device, pushing its pixels first if needed.
func (b *Backend) SelectCover(deviceID string, it cover.Item) {
	if j := b.s.CoverJob; j != nil && j.Err == nil {
		return
	}
	r := b.links[deviceID]
	if r == nil || b.lib == nil {
		b.s.Note(store.NoticeCoverFailed, it.Name, link.ErrNotConnected)
		return
	}
	data := b.CoverData(it)
	b.s.CoverJob = &link.CoverJob{ID: it.ID}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		err := r.link.SelectCover(ctx, it.ID, it.Hash, data, func(done, total int) {
			b.s.Post(func() {
				if j := b.s.CoverJob; j != nil && j.ID == it.ID {
					j.Done, j.Total = done, total
				}
			})
		})
		b.s.Post(func() {
			b.s.CoverJob = nil
			if err != nil {
				b.log.Warn("cover", "id", it.ID, "error", err)
				b.s.Note(store.NoticeCoverFailed, it.Name, err)
				return
			}
			b.s.Note(store.NoticeCoverShown, it.Name, nil)
		})
	}()
}

// AddCover adds a picture file to the library.
func (b *Backend) AddCover(path string) {
	if b.lib == nil {
		b.s.Note(store.NoticeLibraryUnavailable, "", nil)
		return
	}
	go func() {
		it, err := b.lib.Add(path, "")
		b.s.Post(func() {
			if err != nil {
				b.s.Note(store.NoticeCoverAddFailed, filepath.Base(path), err)
				return
			}
			b.s.Covers = b.lib.List()
			b.s.Note(store.NoticeCoverAdded, it.Name, nil)
		})
	}()
}

// RemoveCover drops a picture from the library. The device keeps its
// pixels until newer pushes take its slot.
func (b *Backend) RemoveCover(id int) {
	if b.lib == nil {
		return
	}
	if err := b.lib.Remove(id); err != nil {
		b.s.Note(store.NoticeCoverRemoveFailed, "", err)
		return
	}
	b.mu.Lock()
	delete(b.jpegs, id)
	b.mu.Unlock()
	b.s.Covers = b.lib.List()
}

// RenameCover renames a library picture.
func (b *Backend) RenameCover(id int, name string) {
	if b.lib == nil {
		return
	}
	if err := b.lib.Rename(id, name); err != nil {
		b.s.Note(store.NoticeCoverRenameFailed, "", err)
		return
	}
	b.s.Covers = b.lib.List()
}
