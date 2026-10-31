// Package store is the app's state as the views read it. Its fields change
// only on the UI thread: background work hands results over through the
// post function the app gives New, which runs them there and redraws.
package store

import (
	"errors"
	"time"

	"github.com/LYJW131/anker-prime-mac/internal/cover"
	"github.com/LYJW131/anker-prime-mac/internal/device"
	"github.com/LYJW131/anker-prime-mac/internal/link"
	"github.com/LYJW131/anker-prime-mac/internal/settings"
)

// Store is the whole state.
type Store struct {
	// Devices holds the live state of each added device, by its id
	// (settings.Device.ID). Settings.Devices gives the order.
	Devices map[string]*Device

	Covers   []cover.Item // the cover library, shared by every device with covers
	CoverJob *link.CoverJob

	Scanning bool         // an add-device scan runs
	Found    []link.Found // what it found, supported or not

	Settings  settings.Settings
	HasUserID bool // an Anker account ID is in the Keychain

	Notice *Notice // the toast, cleared after a while

	post func(func())
	now  func() time.Time
}

// Device is one added device's live state. Exactly one of the model
// states is set, by the model's family.
type Device struct {
	Status    link.Status
	Charger   *device.Charger
	PowerBank *device.PowerBank

	// Watts is the headline power, one sample a second, newest last,
	// for the sparkline: output for a charger, input minus output for a
	// power bank.
	Watts []Sample
}

// Sample is one point of a sparkline.
type Sample struct {
	At    time.Time
	Value float64
}

// HistoryLen is how many samples each sparkline keeps.
const HistoryLen = 120

// New returns a store whose background results run through post.
func New(post func(func())) *Store {
	return &Store{post: post, now: time.Now, Settings: settings.Default(), Devices: map[string]*Device{}}
}

// Post runs fn on the UI thread.
func (s *Store) Post(fn func()) { s.post(fn) }

// Now is the store's clock; tests replace it to match the ui.Tester.
func (s *Store) Now() time.Time { return s.now() }

// SetClock replaces the clock.
func (s *Store) SetClock(now func() time.Time) { s.now = now }

// Device returns the live state of device id, creating it.
func (s *Store) Device(id string) *Device {
	d, ok := s.Devices[id]
	if !ok {
		d = &Device{}
		s.Devices[id] = d
	}
	return d
}

// Push appends a sample, keeping the last HistoryLen.
func Push(h []Sample, v Sample) []Sample {
	h = append(h, v)
	if len(h) > HistoryLen {
		h = append(h[:0:0], h[len(h)-HistoryLen:]...)
	}
	return h
}

// Note shows a toast.
func (s *Store) Note(kind NoticeKind, subject string, err error) {
	s.Notice = &Notice{Kind: kind, Subject: subject, Err: err, At: s.now()}
}

// Notice is a toast's content; the views word it in the app's language.
type Notice struct {
	Kind    NoticeKind
	Subject string // a picture's or device's name, when the kind has one
	Err     error
	At      time.Time
}

// NoticeKind is what a toast says.
type NoticeKind int

const (
	NoticeCoverShown           NoticeKind = iota // Subject is on the charger's screen
	NoticeCoverFailed                            // Err says why
	NoticeCoverAdded                             // Subject joined the library
	NoticeCoverAddFailed                         // Err says why
	NoticeCoverRemoveFailed                      // Err says why
	NoticeBluetoothUnavailable                   // Err says why
	NoticeLibraryUnavailable                     // Err says why
	NoticeSettingsNotSaved                       // Err says why
	NoticeDeviceAdded                            // Subject is its name
	NoticeUserIDSaved
)

// Errors of the account ID actions, which the views word.
var (
	ErrUserIDFormat   = errors.New("the account ID must be 40 letters and digits")
	ErrImportNotFound = errors.New("no account ID in the telemetry app's Keychain item")
)
