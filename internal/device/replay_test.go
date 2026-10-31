package device

import (
	"bufio"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const repoRoot = "../.."

type captureLine struct {
	Dir   string  `json:"dir"`
	Cmd   uint16  `json:"cmd"`
	Plain *string `json:"plain"`
}

type fixture struct {
	Capture string `json:"capture"`
	Device  string `json:"device"`
	Frames  []struct {
		Command uint16 `json:"command"`
		State   any    `json:"state"`
	} `json:"frames"`
}

// decoder is what both decoders offer the replay.
type decoder interface {
	Handles(command uint16) bool
	Apply(command uint16, plain []byte) bool
	state() any
}

func (d *ChargerDecoder) state() any   { return d.State }
func (d *PowerBankDecoder) state() any { return d.State }

// registered adapts a Decoder from the registry to the test's interface.
type registered struct{ Decoder }

func (r registered) state() any { return r.Snapshot() }

func readCapturePlain(t *testing.T, path string) []captureLine {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var out []captureLine
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		if strings.TrimSpace(sc.Text()) == "" {
			continue
		}
		var l captureLine
		if err := json.Unmarshal(sc.Bytes(), &l); err != nil {
			t.Fatal(err)
		}
		out = append(out, l)
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

// jsonOf marshals a state and reads it back as generic JSON.
func jsonOf(t *testing.T, v any) any {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var out any
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

// diffJSON returns the first difference between two generic JSON values, "" if
// none. Numbers must agree within 1e-9; null never equals zero or false.
func diffJSON(path string, want, got any) string {
	switch w := want.(type) {
	case map[string]any:
		g, ok := got.(map[string]any)
		if !ok {
			return fmt.Sprintf("%s: want object, got %T", path, got)
		}
		for k, wv := range w {
			gv, present := g[k]
			if !present {
				return fmt.Sprintf("%s.%s: missing", path, k)
			}
			if d := diffJSON(path+"."+k, wv, gv); d != "" {
				return d
			}
		}
		for k := range g {
			if _, present := w[k]; !present {
				return fmt.Sprintf("%s.%s: unexpected field", path, k)
			}
		}
	case []any:
		g, ok := got.([]any)
		if !ok || len(g) != len(w) {
			return fmt.Sprintf("%s: want %d elements, got %v", path, len(w), got)
		}
		for i := range w {
			if d := diffJSON(fmt.Sprintf("%s[%d]", path, i), w[i], g[i]); d != "" {
				return d
			}
		}
	case float64:
		g, ok := got.(float64)
		if !ok || math.Abs(g-w) > 1e-9 {
			return fmt.Sprintf("%s: want %v, got %v", path, want, got)
		}
	default:
		if !reflect.DeepEqual(want, got) {
			return fmt.Sprintf("%s: want %v, got %v", path, want, got)
		}
	}
	return ""
}

// TestReplayFixtures replays every capture through the decoder the way
// tools/contract.py build_fixture does and compares the state, as JSON, with
// spec/fixtures after every frame.
func TestReplayFixtures(t *testing.T) {
	files, _ := filepath.Glob(filepath.Join(repoRoot, "spec", "fixtures", "*.json"))
	if len(files) != 14 {
		t.Fatalf("found %d fixtures, expected 14", len(files))
	}
	frames := 0
	for _, file := range files {
		name := strings.TrimSuffix(filepath.Base(file), ".json")
		t.Run(name, func(t *testing.T) {
			raw, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			var fx fixture
			if err := json.Unmarshal(raw, &fx); err != nil {
				t.Fatal(err)
			}
			// The fixture names its kind; NewDecoder picks the decoder, so a
			// new family's fixtures replay once its decoder is registered.
			reg, ok := NewDecoder(Kind(fx.Device))
			if !ok {
				t.Fatalf("no decoder registered for device %q", fx.Device)
			}
			var dec decoder = registered{reg}

			n := 0
			for _, l := range readCapturePlain(t, filepath.Join(repoRoot, "captures", name+".jsonl")) {
				if l.Dir != "rx" || l.Plain == nil || !dec.Handles(l.Cmd) {
					continue
				}
				plain, err := hex.DecodeString(*l.Plain)
				if err != nil {
					t.Fatal(err)
				}
				dec.Apply(l.Cmd, plain)
				if n >= len(fx.Frames) {
					t.Fatalf("decoded more frames than the fixture's %d", len(fx.Frames))
				}
				want := fx.Frames[n]
				if want.Command != l.Cmd {
					t.Fatalf("frame %d: command %#x, fixture %#x", n, l.Cmd, want.Command)
				}
				if d := diffJSON("state", want.State, jsonOf(t, dec.state())); d != "" {
					t.Fatalf("frame %d (cmd %#x): %s", n, l.Cmd, d)
				}
				n++
			}
			if n != len(fx.Frames) {
				t.Fatalf("decoded %d frames, fixture has %d", n, len(fx.Frames))
			}
			frames += n
		})
	}
	if frames < 700 {
		t.Errorf("only %d frames replayed", frames)
	}
}

// TestApplyReportsChange replays a capture and checks Apply's bool against the
// state itself, and that a second delivery of the same frame changes nothing.
func TestApplyReportsChange(t *testing.T) {
	for _, name := range []string{"charger-03", "powerbank-09"} {
		var dec decoder = NewPowerBankDecoder()
		if strings.Contains(name, "charger") {
			dec = NewChargerDecoder()
		}
		changes := 0
		for _, l := range readCapturePlain(t, filepath.Join(repoRoot, "captures", name+".jsonl")) {
			if l.Dir != "rx" || l.Plain == nil || !dec.Handles(l.Cmd) {
				continue
			}
			plain, _ := hex.DecodeString(*l.Plain)
			before := jsonOf(t, dec.state())
			changed := dec.Apply(l.Cmd, plain)
			after := jsonOf(t, dec.state())
			if visible := !reflect.DeepEqual(before, after); changed != visible {
				t.Fatalf("%s cmd %#x: Apply says %v, JSON %v", name, l.Cmd, changed, visible)
			}
			if again := dec.Apply(l.Cmd, plain); again {
				t.Fatalf("%s cmd %#x: the same frame twice reported a change", name, l.Cmd)
			}
			if changed {
				changes++
			}
		}
		if changes < 10 {
			t.Errorf("%s: only %d changes", name, changes)
		}
	}
}

func TestUnhandledCommandsAreIgnored(t *testing.T) {
	c, p := NewChargerDecoder(), NewPowerBankDecoder()
	for _, cmd := range []uint16{0x0021, 0x0204, 0x021F, 0x0221, 0xFFFF} {
		if c.Handles(cmd) || p.Handles(cmd) || c.Apply(cmd, []byte{0, 0xA1, 1, 1}) || p.Apply(cmd, []byte{0, 0xA1, 1, 1}) {
			t.Errorf("command %#x handled", cmd)
		}
	}
	if !reflect.DeepEqual(c.State, NewCharger()) || !reflect.DeepEqual(p.State, NewPowerBank()) {
		t.Error("an ignored command changed the state")
	}
	// A zero-value decoder works too.
	var zero ChargerDecoder
	zero.Apply(0x020A, []byte{0xA5, 8, 0x04, 1, 0x78, 0x4E, 0x67, 0x11, 0xE7})
	if len(zero.State.Ports) != 3 {
		t.Error("zero-value decoder has no ports")
	}
}

// TestJSONRoundTrip checks that what the states marshal to unmarshals back to
// the same value, so a state cached as JSON keeps the null-until-read fields.
func TestJSONRoundTrip(t *testing.T) {
	var lastC Charger
	var lastP PowerBank
	cd, pd := NewChargerDecoder(), NewPowerBankDecoder()
	check := func(label string, in, out any) {
		t.Helper()
		b, err := json.Marshal(in)
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(b, out); err != nil {
			t.Fatal(err)
		}
		if d := diffJSON(label, jsonOf(t, in), jsonOf(t, reflect.ValueOf(out).Elem().Interface())); d != "" {
			t.Errorf("%s", d)
		}
	}
	check("fresh charger", cd.State, &lastC)
	check("fresh power bank", pd.State, &lastP)
	for _, l := range readCapturePlain(t, filepath.Join(repoRoot, "captures", "charger-03.jsonl")) {
		if plain, ok := plainOf(l); ok && cd.Handles(l.Cmd) {
			cd.Apply(l.Cmd, plain)
		}
	}
	for _, l := range readCapturePlain(t, filepath.Join(repoRoot, "captures", "powerbank-09.jsonl")) {
		if plain, ok := plainOf(l); ok && pd.Handles(l.Cmd) {
			pd.Apply(l.Cmd, plain)
		}
	}
	lastC, lastP = Charger{}, PowerBank{}
	check("charger", cd.State, &lastC)
	check("power bank", pd.State, &lastP)
	if !lastP.ChargingKnown || lastP.Dock == nil || !lastC.Ports[0].Sampled {
		t.Error("derived flags lost in the round trip")
	}
}

func plainOf(l captureLine) ([]byte, bool) {
	if l.Dir != "rx" || l.Plain == nil {
		return nil, false
	}
	b, err := hex.DecodeString(*l.Plain)
	return b, err == nil
}

// TestSnapshotIsIndependent checks Snapshot is a deep copy.
func TestSnapshotIsIndependent(t *testing.T) {
	d := NewPowerBankDecoder()
	d.Apply(0x0300, []byte{0xA8, 14, 4, 1, 0xC8, 0, 0x14, 0, 0xF4, 1, 2, 7, 0xFF, 0xFF, 0xFF, 0xFF})
	snap := d.Snapshot()
	want := jsonOf(t, snap)
	d.Apply(0x0300, []byte{0xA8, 14, 4, 0, 0, 0, 0, 0, 0, 0, 0xFF, 0, 0xFF, 0xFF, 0xFF, 0xFF})
	d.State.Ports[0].Name = "changed"
	if diff := diffJSON("snap", want, jsonOf(t, snap)); diff != "" {
		t.Errorf("snapshot followed the decoder: %s", diff)
	}
}
