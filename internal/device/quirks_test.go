package device

import (
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestQuirks replays testdata/quirks.json: hand-built frames for behaviours the
// captures never reach (cable codes, identity sentinels, rounding, the snapshot
// remap, malformed values), with the states the Python decoders produce for
// them. Regenerate with testdata/gen_quirks.py.
func TestQuirks(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("testdata", "quirks.json"))
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Name   string `json:"name"`
		Device string `json:"device"`
		Frames []struct {
			Command uint16 `json:"command"`
			Plain   string `json:"plain"`
			State   any    `json:"state"`
		} `json:"frames"`
	}
	if err := json.Unmarshal(b, &cases); err != nil {
		t.Fatal(err)
	}
	if len(cases) < 7 {
		t.Fatalf("only %d cases", len(cases))
	}
	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			var dec decoder = NewPowerBankDecoder()
			if c.Device == "charger" {
				dec = NewChargerDecoder()
			}
			for i, f := range c.Frames {
				plain, err := hex.DecodeString(f.Plain)
				if err != nil {
					t.Fatal(err)
				}
				if !dec.Handles(f.Command) {
					t.Fatalf("frame %d: command %#x not handled", i, f.Command)
				}
				dec.Apply(f.Command, plain)
				if d := diffJSON("state", f.State, jsonOf(t, dec.state())); d != "" {
					t.Fatalf("frame %d (cmd %#x, %s): %s", i, f.Command, f.Plain, d)
				}
			}
		})
	}
}
