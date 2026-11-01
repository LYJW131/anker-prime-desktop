package link

import (
	"testing"

	"github.com/LYJW131/anker-prime-desktop/internal/settings"
)

func TestConfigureOnlyInterruptsForConnectionChanges(t *testing.T) {
	d := New(Config{})
	initial := settings.Device{ID: "device", Enabled: true, IdleSleep: true}
	d.Configure(initial)
	_, _, changed := d.snapshot()

	renamed := initial
	renamed.Nickname = "Desk"
	d.Configure(renamed)
	select {
	case <-changed:
		t.Fatal("a nickname change interrupted the connection")
	default:
	}

	renamed.IdleSleep = false
	d.Configure(renamed)
	select {
	case <-changed:
	default:
		t.Fatal("an idle sleep change did not interrupt the connection")
	}
}
