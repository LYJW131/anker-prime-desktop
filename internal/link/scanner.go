package link

import (
	"sync"

	"github.com/LYJW131/anker-prime-mac/internal/ble"
	"github.com/LYJW131/anker-prime-mac/internal/ff09"
	"github.com/LYJW131/anker-prime-mac/internal/model"
)

// Scanner shares the central's one scan between everyone who wants
// adverts: pairing, and links looking for a peripheral the system forgot.
// The radio scans only while someone listens.
type Scanner struct {
	central *ble.Central

	mu   sync.Mutex
	next int
	subs map[int]func(ble.Advert)
}

// NewScanner returns a Scanner over c.
func NewScanner(c *ble.Central) *Scanner {
	return &Scanner{central: c, subs: map[int]func(ble.Advert){}}
}

// Listen delivers every Anker Prime advert to fn until the returned stop is
// called.
func (s *Scanner) Listen(fn func(ble.Advert)) (stop func(), err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.subs) == 0 {
		if err := s.central.Scan([]string{ff09.AdvertisedServiceUUID}, s.deliver); err != nil {
			return nil, err
		}
	}
	id := s.next
	s.next++
	s.subs[id] = fn
	var once sync.Once
	return func() {
		once.Do(func() {
			s.mu.Lock()
			defer s.mu.Unlock()
			delete(s.subs, id)
			if len(s.subs) == 0 {
				s.central.StopScan()
			}
		})
	}, nil
}

func (s *Scanner) deliver(a ble.Advert) {
	s.mu.Lock()
	fns := make([]func(ble.Advert), 0, len(s.subs))
	for _, fn := range s.subs {
		fns = append(fns, fn)
	}
	s.mu.Unlock()
	for _, fn := range fns {
		fn(a)
	}
}

// Classify turns an advert into a Found: its supported model, or the
// model byte of a Prime device this app does not support yet.
func Classify(a ble.Advert) (Found, bool) {
	m, prime, ok := model.Match(a.ManufacturerData, a.Name)
	if !prime {
		return Found{}, false
	}
	f := Found{ID: a.ID, Name: a.Name, RSSI: a.RSSI, ModelByte: -1}
	if b, has := model.AdvertModelByte(a.ManufacturerData); has {
		f.ModelByte = int(b)
	}
	if ok {
		f.Model = string(m.ID)
	}
	return f, true
}
