// Package store is the app's state as the views read it. Its fields change
// only on the UI thread: background work hands results over through the
// post function the app gives New, which runs them there and redraws.
package store

import (
	"time"

	"github.com/LYJW131/anker-prime-mac/internal/cover"
	"github.com/LYJW131/anker-prime-mac/internal/device"
	"github.com/LYJW131/anker-prime-mac/internal/link"
	"github.com/LYJW131/anker-prime-mac/internal/settings"
)

// Store is the whole state.
type Store struct {
	Charger         device.Charger
	ChargerStatus   link.Status
	PowerBank       device.PowerBank
	PowerBankStatus link.Status

	// History of the headline numbers, one sample a second, newest last,
	// for the sparklines.
	ChargerWatts   []Sample
	PowerBankWatts []Sample // signed: positive in, negative out

	Covers   []cover.Item
	CoverJob *link.CoverJob

	Scanning device.Kind // the device a pairing scan runs for, "" when none
	Found    []link.Found

	Settings  settings.Settings
	HasUserID bool // an Anker account ID is in the Keychain

	Notice   string // a short message for the toast, cleared after a while
	NoticeAt time.Time

	post func(func())
	now  func() time.Time
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
	return &Store{post: post, now: time.Now, Settings: settings.Default()}
}

// Post runs fn on the UI thread.
func (s *Store) Post(fn func()) { s.post(fn) }

// Now is the store's clock; tests replace it to match the ui.Tester.
func (s *Store) Now() time.Time { return s.now() }

// SetClock replaces the clock.
func (s *Store) SetClock(now func() time.Time) { s.now = now }

// Push appends a sample, keeping the last HistoryLen.
func Push(h []Sample, v Sample) []Sample {
	h = append(h, v)
	if len(h) > HistoryLen {
		h = append(h[:0:0], h[len(h)-HistoryLen:]...)
	}
	return h
}

// Note shows a toast.
func (s *Store) Note(msg string) {
	s.Notice, s.NoticeAt = msg, s.now()
}
