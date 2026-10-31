package device

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

type specDevices struct {
	Devices struct {
		Charger struct {
			RealtimeCommands []uint16 `json:"realtime_commands"`
			SnapshotCommands []uint16 `json:"snapshot_commands"`
			Ports            []string `json:"ports"`
			TLV              struct {
				PortStruct map[string]string `json:"port_struct"`
				PortCable  map[string]string `json:"port_cable"`
				Identity   string            `json:"identity"`
				BrandModel string            `json:"brand_model"`
			} `json:"tlv"`
			Tables struct {
				CableCapability   map[string]string   `json:"cable_capability"`
				ChargingInfo      map[string]string   `json:"charging_info"`
				CableProfiles     map[string][]string `json:"cable_profiles"`
				VendorNames       map[string]string   `json:"vendor_names"`
				DeviceModels      map[string]string   `json:"device_models"`
				BrandCodes        map[string]string   `json:"brand_codes"`
				IdentitySentinels []string            `json:"identity_sentinels"`
			} `json:"tables"`
		} `json:"charger"`
		PowerBank struct {
			RealtimeCommands []uint16          `json:"realtime_commands"`
			SnapshotCommands []uint16          `json:"snapshot_commands"`
			Ports            []string          `json:"ports"`
			TLV              map[string]string `json:"tlv"`
			Snapshot         struct {
				Shift          int    `json:"shift"`
				State          string `json:"state"`
				Battery        string `json:"battery"`
				TimeLeft       string `json:"time_left"`
				PomodoroSecond string `json:"pomodoro_seconds"`
				PomodoroEnable string `json:"pomodoro_enable"`
			} `json:"snapshot"`
			Direction map[string]string `json:"direction"`
		} `json:"powerbank"`
	} `json:"devices"`
}

func num(t *testing.T, s string) int {
	t.Helper()
	n, err := strconv.ParseInt(strings.TrimPrefix(s, "0x"), 16, 32)
	if err != nil {
		t.Fatalf("bad hex %q", s)
	}
	return int(n)
}

func sameCommands(t *testing.T, name string, got, want []uint16) {
	t.Helper()
	g, w := slices.Clone(got), slices.Clone(want)
	slices.Sort(g)
	slices.Sort(w)
	if !slices.Equal(g, w) {
		t.Errorf("%s: Go %#x, spec %#x", name, g, w)
	}
}

func sameMap[K comparable, V comparable](t *testing.T, name string, got map[K]V, want map[K]V) {
	t.Helper()
	if len(got) != len(want) {
		t.Errorf("%s: Go has %d entries, spec has %d", name, len(got), len(want))
	}
	for k, w := range want {
		if g, ok := got[k]; !ok || g != w {
			t.Errorf("%s[%v]: Go %v (present %v), spec %v", name, k, g, ok, w)
		}
	}
}

// TestTablesMatchSpec fails when a table or constant of the decoders drifts
// from what the Python exports into spec/protocol.json.
func TestTablesMatchSpec(t *testing.T) {
	b, err := os.ReadFile(filepath.Join(repoRoot, "spec", "protocol.json"))
	if err != nil {
		t.Fatal(err)
	}
	var s specDevices
	if err := json.Unmarshal(b, &s); err != nil {
		t.Fatal(err)
	}
	c, p := s.Devices.Charger, s.Devices.PowerBank

	sameCommands(t, "charger snapshot commands", chargerSnapshotCommands, c.SnapshotCommands)
	sameCommands(t, "charger realtime commands", chargerRealtimeCommands, c.RealtimeCommands)
	sameCommands(t, "power bank snapshot commands", pbSnapshotCommands, p.SnapshotCommands)
	sameCommands(t, "power bank realtime commands", pbRealtimeCommands, p.RealtimeCommands)
	if !slices.Equal(chargerPortOrder[:], c.Ports) {
		t.Errorf("charger ports: Go %v, spec %v", chargerPortOrder, c.Ports)
	}

	for i, name := range chargerPortOrder {
		if got := fmt.Sprintf("0x%02X", chargerPortStruct[i]); c.TLV.PortStruct[got] != name {
			t.Errorf("port struct %s: spec has %q for %s", name, c.TLV.PortStruct[got], got)
		}
		if got := fmt.Sprintf("0x%02X", chargerPortCable[i]); c.TLV.PortCable[got] != name {
			t.Errorf("port cable %s: spec has %q for %s", name, c.TLV.PortCable[got], got)
		}
	}
	if len(c.TLV.PortStruct) != 3 || len(c.TLV.PortCable) != 3 {
		t.Error("the spec has a different number of charger port TLVs")
	}
	if num(t, c.TLV.Identity) != chargerTLVIdentity || num(t, c.TLV.BrandModel) != chargerTLVBrandModel {
		t.Error("charger identity TLV numbers")
	}

	tb := c.Tables
	sameMap(t, "cable capability", cableCapabilityLabels, tb.CableCapability)
	sameMap(t, "charging info", chargingInfoLabels, tb.ChargingInfo)
	wantProfiles := map[string]cableProfileEntry{}
	for code, v := range tb.CableProfiles {
		e := cableProfileEntry{label: v[0]}
		if v[1] != "" {
			e.info = v[1]
		}
		wantProfiles[code] = e
	}
	sameMap(t, "cable profiles", cableProfiles, wantProfiles)
	vendors := map[uint16]string{}
	for k, v := range tb.VendorNames {
		vendors[uint16(num(t, k))] = v
	}
	sameMap(t, "vendor names", usbVendorNames, vendors)
	models := map[vidPid]string{}
	for k, v := range tb.DeviceModels {
		vid, pid, _ := strings.Cut(k, ":")
		models[vidPid{uint16(num(t, vid)), uint16(num(t, pid))}] = v
	}
	sameMap(t, "device models", deviceModelNames, models)
	brands := map[byte]string{}
	for k, v := range tb.BrandCodes {
		brands[byte(num(t, k))] = v
	}
	sameMap(t, "brand codes", brandCodes, brands)
	sentinels := map[uint16]bool{}
	for _, k := range tb.IdentitySentinels {
		sentinels[uint16(num(t, k))] = true
	}
	sameMap(t, "identity sentinels", identitySentinels, sentinels)

	if !slices.Equal(p.Ports, []string{"C1", "C2", "A"}) {
		t.Errorf("power bank ports: spec %v", p.Ports)
	}
	for name, got := range map[string]int{
		"state": pbTLVState, "battery": pbTLVBattery, "time_left": pbTLVTimeLeft,
		"thermal_state": pbTLVThermal, "input_total": pbTLVInput, "output_total": pbTLVOutput,
		"dock": pbTLVDock, "port_c1": pbTLVPortC1, "port_c2": pbTLVPortC2, "port_a": pbTLVPortA,
		"temperature_1": pbTLVTemp1, "temperature_2": pbTLVTemp2,
	} {
		if want := num(t, p.TLV[name]); got != want {
			t.Errorf("power bank TLV %s: Go %#x, spec %#x", name, got, want)
		}
	}
	if len(p.TLV) != 12 {
		t.Errorf("the spec has %d power bank TLVs, Go checks 12", len(p.TLV))
	}
	sn := p.Snapshot
	for name, pair := range map[string][2]int{
		"shift": {pbSnapshotShift, sn.Shift}, "state": {pbSnapState, num(t, sn.State)},
		"battery": {pbSnapBattery, num(t, sn.Battery)}, "time_left": {pbSnapTimeLeft, num(t, sn.TimeLeft)},
		"pomodoro_seconds": {pbSnapPomodoro, num(t, sn.PomodoroSecond)},
		"pomodoro_enable":  {pbSnapPomoEnable, num(t, sn.PomodoroEnable)},
	} {
		if pair[0] != pair[1] {
			t.Errorf("power bank snapshot %s: Go %#x, spec %#x", name, pair[0], pair[1])
		}
	}
	dirs := map[string]string{}
	for k, v := range pbDirection {
		dirs[strconv.Itoa(int(k))] = v
	}
	sameMap(t, "direction", dirs, p.Direction)
}
