package app

import (
	"bytes"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func TestTrayIconDecodes(t *testing.T) {
	for _, c := range []struct {
		level    float64
		charging bool
	}{{-1, false}, {0.05, true}, {0.47, true}, {1, false}} {
		data := TrayIcon(c.level, c.charging)
		img, err := png.Decode(bytes.NewReader(data))
		if err != nil {
			t.Fatal(err)
		}
		if b := img.Bounds(); b.Dx() != iconW || b.Dy() != iconH {
			t.Fatalf("icon is %v", b)
		}
		if dir := os.Getenv("UI_SHOTS"); dir != "" {
			name := "tray-" + map[bool]string{true: "charging", false: "idle"}[c.charging] + ".png"
			if c.level < 0 {
				name = "tray-none.png"
			}
			_ = os.WriteFile(filepath.Join(dir, name), data, 0o644)
		}
	}
	if _, err := png.Decode(bytes.NewReader(blankIcon())); err != nil {
		t.Fatal(err)
	}
}
