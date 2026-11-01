GO ?= go
SHOTS ?= build/shots
BIN = Anker Prime Desktop
APP = build/darwin-$(shell $(GO) env GOARCH)/$(BIN).app
# The self-signed identity releases are signed with, when the login Keychain
# has it, else ad hoc. One identity keeps the Keychain and Bluetooth grants
# across rebuilds.
SIGN ?= $(shell security find-identity -p codesigning 2>/dev/null | grep -q '"$(BIN)"' && echo "$(BIN)" || echo -)

.PHONY: build app run demo test vet shots

build:
	CGO_ENABLED=0 $(GO) build ./...

# The macOS app, without a disk image.
app:
	$(GO) tool mygo build -skip-dmg -sign "$(SIGN)"

run:
	$(GO) tool mygo dev

demo: app
	"$(APP)/Contents/MacOS/$(BIN)" -demo

test:
	$(GO) test ./...

vet:
	$(GO) vet ./...

# Every screen, light and dark, wide and narrow, and the menu bar panel, in
# Chinese; the main pages again in English and Japanese, for how they fit.
shots: app
	"$(APP)/Contents/MacOS/$(BIN)" -demo -lang zh-Hans -shots "$(abspath $(SHOTS))"
	for l in en ja; do "$(APP)/Contents/MacOS/$(BIN)" -demo -lang $$l -shots "$(abspath $(SHOTS))/$$l"; done
