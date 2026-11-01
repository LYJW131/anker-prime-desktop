# anker-prime-desktop

A desktop app (Go, MyGo native UI) that shows and controls Anker devices over
Bluetooth, on macOS, Windows and Linux, plus the Python debugger the protocol was reverse-engineered with.
README.md has the layout; docs/adding-a-device.md the path for a new model.

## Rules

- The Python debugger is the reference for what a byte means. A decoding
  change goes into `debugger/anker_prime_ble` first, then
  `debugger/.venv/bin/python tools/contract.py export`, then the Go decoder
  until `go test ./internal/device/...` replays every fixture again.
  `contract.py verify` fails on a stale contract.
- Decrypt a frame by its `0x40` flag, never by device: some telemetry is
  plaintext (`internal/ff09#Crypto.Open`).
- The firmware reuses one GCM nonce for every frame in both directions;
  `internal/ff09` does the same on purpose.
- Every model is one entry in `internal/model#Models` with a decoder
  registered in `internal/device#decoders`. Views and the backend read
  capabilities from the model, never from a model number.
- Store fields change only on the UI thread. Link goroutines hand results to
  the backend's callbacks, which `Store.Post` them (`internal/backend#newLink`).
- Nothing touches the Bluetooth stack (CoreBluetooth, WinRT, BlueZ) until
  `ble.NewCentral`: tests never raise the permission prompt. No cgo:
  `CGO_ENABLED=0` builds everything. Code above `internal/ble` is the same on
  every system; only `internal/ble` has per-system files.
- User-visible text comes from `locales/` (en is the source; zh-Hans and ja
  must have every key). Run `go tool mygo generate` after editing a catalog
  and commit `locales/catalog_gen.go`. Links and the backend return sentinel
  errors (`internal/link/status.go`, `internal/store`) and notice kinds; the
  views word them.
- The Anker account ID lives in the file `anker-user-id` of the user data
  directory, mode 0600, not the Keychain (`internal/backend#userIDFile`
  says why). Never log or print it. `ANKER_USER_ID` overrides it for
  development builds.
- Logging in to the Anker cloud can sign the phone app out: only on the
  user's click, never in tests (cloud tests use `httptest`).
- Covers need no cloud: any id below 65536 works, and the charger keeps the
  last four pushed (docs/screensaver.md).

## Release

Bump `version` in `mygo.json`, add its section to `CHANGELOG.md` (the
update window shows it), commit, then push the tag `v<version>`:
`.github/workflows/release.yml` builds every platform, signs the updates
with the `MYGO_UPDATER_PRIVATE_KEY` secret and publishes the release.
Installed apps update from the latest release. The secret's key also lives
in `~/Library/Application Support/mygo/update-keys` on lyjw's Mac; losing
it strands installed apps.

The macOS app is signed with a self-signed identity, "Anker Prime
Desktop": the `MACOS_SIGNING_IDENTITY` secret in CI, the login Keychain and
`~/Library/Application Support/mygo/codesign` on lyjw's Mac. macOS keeps an
app's Bluetooth grant for one signing identity, so an ad-hoc or other
build asks for it again.

## Verify

```bash
go vet ./... && go test ./...
GOOS=windows go vet ./... && GOOS=linux go vet ./...
debugger/.venv/bin/python tools/contract.py verify
make app
"build/darwin-arm64/Anker Prime Desktop.app/Contents/MacOS/Anker Prime Desktop" -demo -shots /tmp/anker-shots
```

`make app` signs with the identity when the Keychain has it, so rebuilds
keep the Bluetooth grant. Look at the shots in both themes (add `-lang en`, `-lang ja`). CoreBluetooth
aborts a process without Bluetooth permission: run the built binary from a
terminal that has it, or `open` the app, to test against real devices.
