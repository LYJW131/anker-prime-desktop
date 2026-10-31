GO ?= go
SHOTS ?= build/shots
APP = build/darwin-$(shell $(GO) env GOARCH)/Anker Prime Desktop.app

.PHONY: build app run demo test vet shots

build:
	CGO_ENABLED=0 $(GO) build ./...

# The macOS app, ad-hoc signed, without a disk image.
app:
	$(GO) tool mygo build -skip-dmg

run:
	$(GO) tool mygo dev

demo: app
	"$(APP)/Contents/MacOS/Anker Prime" -demo

test:
	$(GO) test ./...

vet:
	$(GO) vet ./...

# Every screen, light and dark, wide and narrow, and the menu bar panel, in
# Chinese; the main pages again in English and Japanese, for how they fit.
shots: app
	"$(APP)/Contents/MacOS/Anker Prime" -demo -lang zh-Hans -shots "$(abspath $(SHOTS))"
	for l in en ja; do "$(APP)/Contents/MacOS/Anker Prime" -demo -lang $$l -shots "$(abspath $(SHOTS))/$$l"; done
