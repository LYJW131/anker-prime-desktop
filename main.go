// Command anker-prime shows and controls Anker devices over Bluetooth, in
// a window and from the menu bar: the devices the user adds, of the models
// internal/model supports.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/LYJW131/anker-prime-desktop/internal/app"
	"github.com/LYJW131/anker-prime-desktop/internal/backend"
	"github.com/LYJW131/anker-prime-desktop/internal/demo"
)

func main() {
	demoFlag := flag.Bool("demo", false, "run on a made-up backend instead of Bluetooth")
	shots := flag.String("shots", "", "capture every screen into this directory as PNGs, then quit")
	theme := flag.String("theme", "", "force the theme: light or dark")
	data := flag.String("data", "", "keep settings and caches under this directory")
	lang := flag.String("lang", "", "show this language for one run: en, zh-Hans or ja")
	live := flag.Bool("live", false, "with -shots, capture the real devices instead of the demo")
	flag.Parse()

	if *lang != "" {
		// Read by MyGo as the app starts; a language chosen this way is
		// kept nowhere.
		os.Setenv("MYGO_LOCALE", *lang)
	}
	opts := app.Options{Shots: *shots, Theme: *theme, DataDir: *data, Lang: *lang, Live: *live}
	if *demoFlag || (*shots != "" && !*live) {
		opts.Backend = demo.New()
	} else {
		opts.Backend = backend.New(nil)
	}
	if err := app.Run(opts); err != nil {
		fmt.Fprintln(os.Stderr, "anker-prime:", err)
		os.Exit(1)
	}
}
