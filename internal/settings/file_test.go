package settings

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestSaveThenLoad(t *testing.T) {
	dir := t.TempDir()
	s, err := Load(dir)
	if err != nil || !reflect.DeepEqual(s, Default()) {
		t.Fatalf("a missing file loads %+v, %v", s, err)
	}
	s.Theme, s.ShowDock = "dark", true
	s.MenuBar.Fields = []string{"ABC/flow"}
	s.Devices = []Device{{ID: "ABC", Model: "A110G", Name: "AJ7DLCH0F49600286", Enabled: true, IdleSleep: true}}
	if err := Save(dir, s); err != nil {
		t.Fatal(err)
	}
	got, err := Load(dir)
	if err != nil || !reflect.DeepEqual(got, s) {
		t.Fatalf("loaded %+v, %v", got, err)
	}
	// A file without a field keeps that field's default.
	if err := os.WriteFile(filepath.Join(dir, FileName), []byte(`{"theme":"light"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	got, _ = Load(dir)
	if got.Theme != "light" || !got.MenuBar.ShowIcon || got.Devices != nil || got.Panel.Cards != nil {
		t.Errorf("partial file loads %+v", got)
	}
}

// An emptied list stays empty after a round trip: nil stands for the
// defaults, and the user chose none.
func TestEmptyListSurvives(t *testing.T) {
	dir := t.TempDir()
	s := Default()
	s.MenuBar.Fields = []string{}
	if err := Save(dir, s); err != nil {
		t.Fatal(err)
	}
	got, err := Load(dir)
	if err != nil || got.MenuBar.Fields == nil || got.Panel.Cards != nil {
		t.Fatalf("loaded fields %#v, cards %#v, %v", got.MenuBar.Fields, got.Panel.Cards, err)
	}
}
