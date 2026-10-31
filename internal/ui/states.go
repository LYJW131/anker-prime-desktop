package ui

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/LYJW131/anker-prime-desktop/internal/link"
	"github.com/LYJW131/anker-prime-desktop/internal/settings"
	"github.com/LYJW131/anker-prime-desktop/locales"
)

// phaseShown is the phase a device's state shows and since when the
// store's phase has been the one it holds now.
type phaseShown struct {
	phase link.Phase
	store link.Phase
	since time.Time
}

// transient phases pass in a moment on a good link: the state keeps the
// phase it showed until one of them lasts statusDelay.
func transient(p link.Phase) bool {
	return p == link.PhaseConnecting || p == link.PhaseHandshaking
}

// shownPhase is the phase device id's state shows: the store's, except
// that a transient one shows only after statusDelay.
func (a *App) shownPhase(c *ui.Context, id string, st link.Status) link.Phase {
	if a.shown == nil {
		a.shown = map[string]phaseShown{}
	}
	ps, ok := a.shown[id]
	if !ok {
		ps = phaseShown{phase: st.Phase, store: st.Phase, since: c.Now()}
	}
	if ps.store != st.Phase {
		ps.store, ps.since = st.Phase, c.Now()
	}
	if !transient(st.Phase) || waited(c, true, ps.since) || !ok {
		ps.phase = st.Phase
	}
	a.shown[id] = ps
	return ps.phase
}

// devState is how a device's connection reads: a word, a tone, and
// whether its numbers are live.
type devState struct {
	// phase is the phase shown (shownPhase).
	phase link.Phase
	word  string
	tone  pillKind
	ic    *ui.SVG
	// live is set while the numbers are fresh; stale numbers still show,
	// dimmed.
	live bool
	// dead is set when there is nothing to show but the state (no frame
	// yet, or the device can't be reached at all).
	dead bool
}

// state is a device's connection as the views show it.
func (a *App) state(c *ui.Context, d dev) devState {
	st := d.st.Status
	if !d.set.Enabled {
		return devState{phase: link.PhaseOff, word: locales.StateOff(), tone: pillWait, dead: true}
	}
	ph := a.shownPhase(c, d.ID(), st)
	noData := st.LastFrame.IsZero()
	s := devState{phase: ph, dead: noData}
	switch ph {
	case link.PhaseOff:
		s.word, s.tone, s.dead = locales.StateOff(), pillWait, true
	case link.PhaseBluetoothOff:
		s.word, s.tone, s.ic, s.dead = locales.StateBluetoothOff(), pillBad, icBluetooth, true
	case link.PhaseUnauthorized:
		s.word, s.tone, s.ic, s.dead = locales.StateUnauthorized(), pillBad, icCross, true
	case link.PhaseUnpaired:
		s.word, s.tone, s.dead = locales.StateUnpaired(), pillWarn, true
	case link.PhaseConnecting:
		s.word, s.tone = locales.StateConnecting(), pillRun
	case link.PhaseHandshaking:
		s.word, s.tone = locales.StateHandshaking(), pillRun
	case link.PhaseConnected:
		s.word, s.tone, s.ic, s.live = locales.StateConnected(), pillOK, icCheck, !noData
		if st.Stale(c.Now()) {
			s.word, s.tone, s.ic, s.live = locales.StateStale(), pillWarn, icClock, false
		} else if noData {
			s.word, s.tone, s.ic = locales.StateWaiting(), pillRun, nil
		}
		if !noData {
			// The moment it goes stale.
			c.After(max(time.Millisecond, st.LastFrame.Add(link.StaleAfter).Sub(c.Now())+time.Millisecond))
		}
	case link.PhaseDisconnected:
		s.word, s.tone, s.ic = locales.StateDisconnected(), pillWarn, icSync
	case link.PhaseSleeping:
		s.word, s.tone = locales.StateSleeping(), pillWait
	}
	return s
}

// statePill is a state as a pill.
func (a *App) statePill(c *ui.Context, k *Tokens, s devState) ui.Element {
	return pill(c, k, s.tone, s.ic, s.word)
}

// setEnabled turns a device on or off.
func (a *App) setEnabled(id string, on bool) {
	a.apply(func(n *settings.Settings) {
		if sd, ok := n.Device(id); ok {
			sd.Enabled = on
		}
	})
}

// deadCard is the page of a device with nothing to show but its state:
// what stands, and the one action that moves it on.
func (a *App) deadCard(c *ui.Context, k *Tokens, d dev, s devState) {
	st := d.st.Status
	title, line := s.word, ""
	var act string
	var do func()
	ph := s.phase
	id := d.ID()
	switch {
	case !d.set.Enabled:
		title, line = locales.DeadOffTitle(d.shortName()), locales.DeadOffLine()
		act, do = locales.DeadTurnOn(), func() { a.setEnabled(id, true) }
	case ph == link.PhaseBluetoothOff:
		line = locales.DeadBluetoothOffLine()
	case ph == link.PhaseUnauthorized:
		line = locales.DeadUnauthorizedLine()
	case ph == link.PhaseUnpaired:
		line = d.set.Name
	case ph == link.PhaseConnecting || ph == link.PhaseHandshaking:
		title = s.word + "…"
		line = d.set.Name
	case ph == link.PhaseSleeping:
		line = locales.DeadSleepingLine()
		act, do = locales.DeadWake(), func() { a.Dev.Connect(id) }
	case ph == link.PhaseDisconnected:
		line = locales.DeadDisconnectedLine()
		act, do = locales.DeadReconnect(), func() { a.Dev.Connect(id) }
	case ph == link.PhaseConnected:
		title, line = locales.DeadWaitingTitle(), d.set.Name
	}
	ui.Column(c.Key("dead")).Padding(36, padCard).Gap(10).Radius(radiusCard).Background(k.Raise).AlignItems(ui.Center).Children(func() {
		ic, col := familyLook(k, d.m.Family)
		tile(c, ic, col, 44).Opacity(0.9).Margin(0, 0, 6, 0)
		ui.Text(c, title).FontSize(17).FontWeight(700).TextAlign(ui.Center)
		if line != "" {
			ui.Text(c, line).FontSize(12.5).TextColor(k.Muted).TextAlign(ui.Center)
		}
		if st.Err != nil && ph != link.PhaseConnected && d.set.Enabled {
			ui.Text(c, ErrText(st.Err)).FontSize(11.5).TextColor(k.Bad).MaxWidth(520).SingleLine().Ellipsis("…")
		}
		if do != nil && act != "" {
			if button(c, k, buttonPrimary, act, nil).Margin(6, 0, 0, 0).Clicked() {
				do()
			}
		}
	})
}

// Numbers.

// none is the empty value.
const none = "—"

// watts writes a power to one decimal.
func watts(v float64) string { return fmt.Sprintf("%.1f", math.Abs(v)) }

// fixed writes v to d decimals, or none.
func fixed(v *float64, d int) string {
	if v == nil {
		return none
	}
	return fmt.Sprintf("%.*f", d, *v)
}

// str is a decoded string, or "" for absent, empty or "N/A".
func str(s *string) string {
	if s == nil {
		return ""
	}
	t := strings.TrimSpace(*s)
	if t == "" || strings.EqualFold(t, "N/A") {
		return ""
	}
	return t
}

func orNone(s string) string {
	if s == "" {
		return none
	}
	return s
}

func intStr(v *int, unit string) string {
	if v == nil {
		return none
	}
	return fmt.Sprintf("%d%s", *v, unit)
}

// hm writes hours and minutes as 1:05.
func hm(h, m *int) string {
	if h == nil || m == nil {
		return none
	}
	return fmt.Sprintf("%d:%02d", *h, *m)
}
