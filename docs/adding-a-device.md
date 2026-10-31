# Adding a device

How an Anker device goes from "on the desk" to "in the app's add-device
list". The two supported models, the A2687 charger and the A110G power bank,
went this way; follow the same steps for the next one.

The path has two halves. The first happens in the Python debugger
(`debugger/`), where a device is understood: its advert, its session, the
meaning of every byte. The second ports what was learned into the Go app,
where the contract files make the port checkable frame by frame.

```
scan → capture → decode (Python) → contract (spec/) → decoder (Go) → model → views → device test
```

## 0. Is it on the shared transport?

Every Anker Prime device seen so far speaks the same FF09 protocol: the
same GATT service, framing, AES-GCM and ECDH handshake
([debugger/README.md](../debugger/README.md#why-one-repository)). Only the
TLVs inside differ. Check first:

```bash
cd debugger && .venv/bin/python -m anker_prime_ble scan
```

A Prime device advertises service `0xFF09` and manufacturer data under
company `0xFFFF` laid out as `02 | MAC(6) | 02 B4 | model`. The last byte is
the model byte (`0x05` charger, `0x0A` power bank). The app's add-device
list already shows such a device, as not supported yet with its model byte.

Run the debugger from a terminal that has Bluetooth access, and quit the
Anker phone app first: a device connected to the phone does not advertise.

A device that does not advertise FF09 is a different protocol family. It
needs its own transport before anything below applies.

## 1. Capture

```bash
.venv/bin/python -m anker_prime_ble session <address> --device powerbank --record ../captures/<family>-01.jsonl
```

The handshake is shared, so `--device powerbank` (which sends no
device-specific requests) is a safe start for an unknown model. Every device
seen so far needs the Anker account ID in `0x0027` (`$ANKER_USER_ID`): the
charger streams nothing without it and the power bank drops the link after
26 seconds.

Record one capture per situation the device can be in (idle, charging,
discharging, each port, thermal limits) and change one thing at a time. A
capture is the evidence for a field, so keep them: `captures/README.md` says
what each one shows. Captures carry the device's serial number and MAC; this
repository keeps them on purpose.

## 2. Decode in Python

- `replay <capture> --decode` and `watch` print what changes between frames.
  Change one thing on the device, see which bytes move.
- Write the field map in `docs/<family>.md` with the evidence for each field,
  and the dead ends in [NOTES.md](../NOTES.md).
- Add `debugger/anker_prime_ble/<family>.py`: a `DeviceProfile` and a decoder
  whose state has a `to_dict()`. The transport, session and CLI do not change.

Things that bit before: telemetry frames may be plaintext while handshake
frames are encrypted (decrypt by the frame's `0x40` flag, never by device);
scales differ per model (mV/mA/cW on the charger, tenths on the power bank);
some fields are `[integer, hundredths]` pairs, not u16; some slots are sticky
or stale. [NOTES.md](../NOTES.md) has the full list.

## 3. Contract

```bash
cd .. && debugger/.venv/bin/python tools/contract.py export
```

`tools/contract.py` turns the Python modules into the contract every other
implementation is tested against: constants and tables in
`spec/protocol.json`, and per capture a fixture `spec/fixtures/<capture>.json`
of the decoded state after every frame. Teach `build_fixture` the new family
(it picks the profile by the capture's file name) and give the fixtures'
`"device"` the family's kind. `contract.py verify` fails when the contract
is stale.

## 4. Decoder in Go

- `internal/device/<family>.go`: the state struct, JSON tags equal to the
  Python `to_dict()` keys (pointers where Python can say `null`), and a
  decoder with `Handles`, `Apply`, `Snapshot` and `Reset`.
- Register it in `internal/device/decoder.go` (`decoders`), under a new
  `Kind`.
- `go test ./internal/device/...` now replays the new fixtures through it
  frame by frame (`replay_test.go` picks the decoder by the fixture's
  `"device"`) and compares the marshalled state with the fixture. Until every
  frame matches, the Go decoder and the Python one disagree on a byte.

New requests or writes (anything besides the handshake, `0x0200` and
`0x020A`) go in `internal/ff09` as step builders with byte-level test
vectors taken from a capture.

## 5. Model

Add one entry to `Models` in `internal/model/model.go`: the model number,
family, decoder kind, advertised model byte, whether the session sends the
`0x020A` realtime request, and the capabilities the app offers (covers, idle
sleep, rated power, ports). `go test ./internal/model/...` checks that every
model has a decoder and a unique model byte.

From here the app recognises the device: the add-device list offers it, and
the backend starts a link for it with the shared session in
`internal/link` (handshake, re-arm, reconnect, idle sleep).

If the family is new, `internal/link/session.go#decode` and
`internal/backend/backend.go#newLink` route its snapshot to a new field of
`store.Device`, next to `Charger` and `PowerBank`.

## 6. Views

A device page, its menu-bar fields and its panel cards live in
`internal/ui`, chosen by the model's family. A new family adds:

- the device page and its port cards;
- its entries in the menu-bar field catalog and the panel card catalog;
- its model name and every new string in `locales/en.json`,
  `locales/zh-Hans.json` and `locales/ja.json`, then `go tool mygo generate`;
- demo data in `internal/demo`, so screenshots and tests cover it.

Check every page in both themes and all three languages with
`-demo -shots <dir>` (add `-lang en` or `-lang ja`).

## 7. Test on the device

Two development tools run the Go stack without the window, from a terminal
with Bluetooth access:

```bash
go run ./cmd/bleprobe -scan                                   # adverts: name, id, RSSI, manufacturer data
go run ./cmd/bleprobe -connect <name>                         # GATT, write length, first handshake step
go run ./cmd/linkprobe -id <id> -model <model> -for 60s       # the app's link: decoded state as JSON lines
go run ./cmd/linkprobe -id <id> -model A2687 -cover pic.png -cover-id 1003
```

Then the app itself. Build it and run it from that terminal, so the process
inherits the permission:

```bash
go tool mygo build -skip-dmg
"build/darwin-arm64/Anker Prime Desktop.app/Contents/MacOS/Anker Prime Desktop" -data /tmp/anker-dev
```

Add the device from the add-device list, watch the log for `connected`,
leave it running long enough to see a re-arm and, for a power bank, idle
sleep, and compare every number on the page with the device's own screen or
the Anker app.

## Checklist

- [ ] `scan` shows FF09 and a model byte
- [ ] captures for each situation, described in `captures/README.md`
- [ ] `docs/<family>.md` field map with evidence; `NOTES.md` dead ends
- [ ] Python profile and decoder; `contract.py export` and `verify`
- [ ] Go decoder registered; fixture replay passes
- [ ] model entry; `go test ./internal/model/...` passes
- [ ] views, catalogs in three languages, demo data, shots looked at
- [ ] tested against the real device
