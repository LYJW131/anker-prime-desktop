// Command linkprobe runs the app's device link without its window: it
// connects one device, prints its status and decoded state as JSON lines,
// and can push a cover to a charger. A development tool for checking a
// model against the real device (docs/adding-a-device.md).
//
// Run it from a terminal with Bluetooth access; CoreBluetooth aborts a
// process without it. The account ID comes from $ANKER_USER_ID.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"time"

	"github.com/LYJW131/anker-prime-mac/internal/ble"
	"github.com/LYJW131/anker-prime-mac/internal/cover"
	"github.com/LYJW131/anker-prime-mac/internal/link"
	"github.com/LYJW131/anker-prime-mac/internal/model"
	"github.com/LYJW131/anker-prime-mac/internal/settings"
)

func main() {
	id := flag.String("id", "", "CoreBluetooth identifier of the device (from the add-device list or bleprobe -scan)")
	modelID := flag.String("model", "", "model number, e.g. A2687")
	seconds := flag.Duration("for", 20*time.Second, "how long to stay connected")
	coverPath := flag.String("cover", "", "an image to push to the charger as cover -cover-id")
	coverID := flag.Int("cover-id", 0, "the id to store the cover under (below 65536)")
	flag.Parse()

	m, ok := model.ByID(model.ID(*modelID))
	if *id == "" || !ok {
		fmt.Fprintln(os.Stderr, "usage: linkprobe -id <uuid> -model <A2687|A110G> [-for 20s] [-cover img -cover-id 1003]")
		os.Exit(2)
	}
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	central, err := ble.NewCentral()
	if err != nil {
		log.Error("bluetooth", "error", err)
		os.Exit(1)
	}
	defer central.Close()

	out := json.NewEncoder(os.Stdout)
	connected := make(chan struct{})
	var once bool
	dev := link.New(link.Config{
		Model:   m,
		Central: central,
		Scanner: link.NewScanner(central),
		UserID:  func() string { return os.Getenv("ANKER_USER_ID") },
		Log:     log,
		OnStatus: func(st link.Status) {
			if st.Phase == link.PhaseConnected && !once {
				once = true
				close(connected)
			}
		},
		OnState: func(state any) { out.Encode(map[string]any{"t": time.Now().Format(time.TimeOnly), "state": state}) },
	})
	dev.Configure(settings.Device{ID: *id, Model: *modelID, Enabled: true})

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, *seconds)
	defer cancel()
	go dev.Run(ctx)

	if *coverPath != "" {
		select {
		case <-connected:
		case <-ctx.Done():
			log.Error("not connected")
			os.Exit(1)
		}
		img, err := cover.DecodeFile(*coverPath)
		if err == nil {
			var jpeg []byte
			if jpeg, err = cover.EncodeCover(img); err == nil {
				hash := cover.CRC(jpeg)
				log.Info("pushing cover", "id", *coverID, "bytes", len(jpeg), "hash", fmt.Sprintf("0x%08x", hash))
				start := time.Now()
				err = dev.SelectCover(ctx, *coverID, hash, jpeg, func(done, total int) {
					if done%10 == 0 || done == total {
						log.Info("chunks", "done", done, "total", total)
					}
				})
				log.Info("cover done", "took", time.Since(start).Round(time.Millisecond), "error", err)
			}
		}
		if err != nil {
			log.Error("cover", "error", err)
		}
	}
	<-ctx.Done()
}
