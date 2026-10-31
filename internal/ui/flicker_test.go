package ui

import (
	"crypto/sha256"
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/LYJW131/anker-prime-desktop/internal/demo"
	"github.com/LYJW131/anker-prime-desktop/locales"
)

// The flicker tests. A request that answers within a frame must not show
// its waiting state, drop what the screen showed, or move anything: the
// frame the user sees between the click and the answer has to be the one
// before or the one after. noFlash asserts that for any click; a screen
// with an async action comes with a test that calls it.

// frameSnap is what one frame shows: its texts, the box of each, and a hash
// of its pixels, which also catches a control that greys or dims.
type frameSnap struct {
	texts  []string
	rects  map[string]ui.Rect
	pixels [sha256.Size]byte
}

func snapFrame(tt *ui.Tester) frameSnap {
	s := frameSnap{texts: tt.Texts(), rects: map[string]ui.Rect{}}
	for _, x := range s.texts {
		if r, ok := tt.Find(x); ok {
			s.rects[x] = r
		}
	}
	s.pixels = sha256.Sum256(tt.Image().Pix)
	return s
}

// differs says how two frames differ, or returns "" when they match.
func (a frameSnap) differs(b frameSnap) string {
	if x, y := strings.Join(a.texts, "\x00"), strings.Join(b.texts, "\x00"); x != y {
		gone, came := textDiff(a.texts, b.texts)
		return fmt.Sprintf("texts: %q gone, %q new", gone, came)
	}
	for text, r := range a.rects {
		o := b.rects[text]
		if abs32(r.X-o.X) > 0.5 || abs32(r.Y-o.Y) > 0.5 || abs32(r.W-o.W) > 0.5 || abs32(r.H-o.H) > 0.5 {
			return fmt.Sprintf("the box of %q: %v vs %v", text, r, o)
		}
	}
	if a.pixels != b.pixels {
		return "pixels (something greyed, dimmed or moved without a text change)"
	}
	return ""
}

func textDiff(a, b []string) (gone, came []string) {
	count := map[string]int{}
	for _, x := range a {
		count[x]++
	}
	for _, x := range b {
		count[x]--
	}
	for x, n := range count {
		for ; n > 0; n-- {
			gone = append(gone, x)
		}
		for ; n < 0; n++ {
			came = append(came, x)
		}
	}
	sort.Strings(gone)
	sort.Strings(came)
	return gone, came
}

func abs32(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}

// noFlash clicks the element showing click and lets answer deliver the
// answer the click waits for, at once. It fails the test when the frame
// rendered by the click is neither the frame before the click nor the
// frame after the answer. The pointer rests on the element first, so a
// hover fill is in the frame before as well.
func noFlash(t *testing.T, tt *ui.Tester, click string, answer func()) (before, mid, after frameSnap) {
	t.Helper()
	if r, ok := tt.Find(click); ok {
		tt.Move(r.X+r.W/2, r.Y+r.H/2)
	}
	tt.Frame()
	before = snapFrame(tt)
	if err := tt.Click(click); err != nil {
		t.Fatal(err)
	}
	mid = snapFrame(tt)
	answer()
	tt.Frame()
	after = snapFrame(tt)
	d1, d2 := mid.differs(before), mid.differs(after)
	if d1 != "" && d2 != "" {
		t.Errorf("clicking %q flashes: the frame before its answer is neither the one before the click (%s) nor the one after the answer (%s)", click, d1, d2)
	}
	return before, mid, after
}

func TestRescanDoesNotFlash(t *testing.T) {
	r := newRig(t, "dark", 1280, 800, false)
	r.a.Go(Page{Kind: PageAddDevice})
	r.flush()
	r.tt.Advance(statusDelay)
	r.tt.Frame()
	if err := r.tt.Click(locales.AddStop()); err != nil {
		t.Fatal(err)
	}
	r.tt.Frame()
	r.want(t, locales.AddStopped(), demo.OtherChargerName)
	// Scanning again keeps the rows until the new finds show.
	_, mid, _ := noFlash(t, r.tt, locales.AddRescan(), r.flush)
	if !strings.Contains(strings.Join(mid.texts, "\n"), demo.OtherChargerName) {
		t.Errorf("a new scan dropped the rows before its finds: %q", mid.texts)
	}
	r.tt.Advance(statusDelay)
	r.tt.Frame()
	r.want(t, locales.AddScanning(), demo.UnknownName+" · -79 dBm")
}

func TestSelectingACoverDoesNotFlash(t *testing.T) {
	// The fourth picture is on the charger already: a selection the
	// charger answers at once.
	r := newRig(t, "dark", 1280, 800, false)
	r.a.GoDevice(demo.ChargerID, TabCovers)
	r.tt.Frame()
	d, _ := r.a.device(demo.ChargerID)
	name := r.a.Store.Covers[3].Name
	noFlash(t, r.tt, name, r.flush)
	if cur, ok := r.a.currentCover(d); !ok || cur.Name != name {
		t.Fatalf("the charger shows %q, not %q", cur.Name, name)
	}
	// One beyond the charger's four pushes its pixels: progress shows once
	// the push has run a moment, then the picture is current.
	far := r.a.Store.Covers[5]
	if err := r.tt.Click(far.Name); err != nil {
		t.Fatal(err)
	}
	r.tt.Advance(statusDelay)
	r.tt.Frame()
	r.want(t, locales.CoverSending())
	r.flush()
	r.tt.Frame()
	if cur, _ := r.a.currentCover(d); cur.ID != far.ID {
		t.Errorf("after the push the charger shows %q", cur.Name)
	}
}

func TestImportingTheAccountIDDoesNotFlash(t *testing.T) {
	r := newRig(t, "dark", 1280, 1400, false)
	r.a.GoSettings()
	r.a.Store.HasUserID = false
	r.tt.Frame()
	noFlash(t, r.tt, locales.AcctImport(), r.flush)
	r.want(t, locales.AcctImported(), locales.AcctSaved())
}

func TestReconnectDoesNotFlash(t *testing.T) {
	r := newRig(t, "dark", 1280, 800, false)
	r.a.GoDevice(demo.ChargerID, TabSettings)
	r.tt.Frame()
	if err := r.tt.Click(locales.ConnDisconnect()); err != nil {
		t.Fatal(err)
	}
	r.tt.Frame()
	r.want(t, locales.StateDisconnected())
	noFlash(t, r.tt, locales.ConnConnect(), r.flush)
	r.want(t, locales.StateConnected())
}
