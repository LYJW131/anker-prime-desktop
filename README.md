# anker-prime-desktop

**Anker Prime Desktop** shows and controls Anker devices over Bluetooth, in
a window and from the menu bar. Add a device, watch its ports,
battery and power live, choose what the menu bar title and its dropdown
panel show, and push your own pictures to a charger's screen — no Anker
account login and no cloud needed.

Supported models today:

| model | device | what the app does |
|---|---|---|
| A2687 | Anker Prime Charger (160W) | per-port V/A/W, cable rating, charging protocol, attached device, custom screen covers |
| A110G | Anker Prime Power Bank (20K, 220W) | battery, flow in and out, time to full, temperatures, health, per-port V/A/W, idle sleep |

Other Anker Prime devices show up in the add-device list as not supported
yet. [docs/adding-a-device.md](docs/adding-a-device.md) is the path from such
a device to a supported model.

The app speaks English, Simplified Chinese and Japanese. It runs on macOS
13 and later, Windows 10 and later, and Linux with BlueZ.

## Install

Get it from [Releases](https://github.com/LYJW131/anker-prime-desktop/releases):

- **macOS 13+**: the disk image for your Mac. The app is not notarized yet,
  so the first launch needs a right-click → Open (or System Settings →
  Privacy & Security → Open Anyway).
- **Windows 10+**: `anker-prime-desktop-setup-<version>-<arch>.exe`. It is
  not code-signed yet, so SmartScreen asks first: More info → Run anyway.
- **Linux** (BlueZ): the `.deb`, or the `.tar.gz` for other distributions.
  Not tested on real hardware yet.

After that it updates itself: it checks GitHub once a day, and on demand
from "Check for Updates…" in its menus.

## Build and run

Needs Go 1.27.3 or later on macOS 13+.

```bash
go tool mygo build -skip-dmg          # build/darwin-arm64/Anker Prime Desktop.app
open "build/darwin-arm64/Anker Prime Desktop.app"
```

The first launch asks for Bluetooth access. The app lives in the menu bar;
it shows a Dock icon while its window is open.

The charger and power bank stream only to a session that presents the
Anker account ID the devices are bound to (40 characters). The app reads it
once — pasted, from a one-time Anker login, or on macOS imported from the
Keychain — and keeps it in a file of its data folder that only you can
read (`anker-user-id`).

`-demo` runs on made-up devices; `-demo -shots <dir>` captures every screen
and quits (`-theme`, `-lang` force a theme or language).

## Layout

```
main.go, mygo.json, locales/   the app, its MyGo packaging, its languages
internal/ff09      FF09 transport: framing, TLV, AES-GCM, ECDH, request builders
internal/device    decoded state per family and the decoders, tested against spec/fixtures
internal/model     the supported models
internal/ble       Bluetooth LE without cgo: CoreBluetooth, WinRT, BlueZ
internal/link      one device's session: connect, handshake, re-arm, reconnect, covers
internal/cover     the local cover library
internal/cloud     optional Anker login, to learn the account ID
internal/backend   ties the above to the app
internal/ui, app   views, window, menu bar item and panel
debugger/          the Python reverse-engineering tool the protocol was worked out with
spec/, captures/   the cross-language contract and the recorded sessions behind it
docs/, NOTES.md    field maps, the screensaver protocol, how it was worked out
```

## Prior work

The protocol work started from
[Hyper-Beast/Anker_Prime_160W_WebBLE](https://github.com/Hyper-Beast/Anker_Prime_160W_WebBLE),
which credits [flip-dots/SolixBLE](https://github.com/flip-dots/SolixBLE) and
[atc1441/Anker_Prime_BLE_hacking](https://github.com/atc1441/Anker_Prime_BLE_hacking).
See [debugger/README.md](debugger/README.md#prior-work).
