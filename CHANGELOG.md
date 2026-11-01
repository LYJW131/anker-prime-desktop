# Changelog

## 1.0.2

- The account ID moves from the Keychain to a file of the app's data folder
  that only you can read: macOS no longer asks for your password after an
  update. The first launch moves it, which may ask one last time.
- The macOS app is signed with one certificate from now on, so it keeps its
  Bluetooth permission across updates. After this update macOS asks once
  more.
- The charger shows "—" for a port until its first reading, instead of
  idle and 0 W, and leaves out the cable, protocol and device lines it does
  not know.
- Changing the language, theme or menu bar no longer reconnects the
  devices.

## 1.0.1

- Windows: saving settings while a device is connecting no longer drops
  that device's connection a moment later.
- Windows: without a Bluetooth adapter the app shows Bluetooth as off and
  picks one up when it appears; scans always stop.
- Linux: with no secret store the account ID is kept for the session, and
  Settings says so. The `.deb` now depends on `libsecret-1-0`.
- Windows and Linux: Settings no longer offers the macOS Keychain import or
  the Dock option.
- Faster, bounded Bluetooth calls on Linux, so a stuck BlueZ cannot hang a
  device.

## 1.0.0

The first release, for macOS, Windows and Linux.

- Add Anker Prime devices from a Bluetooth scan: the Anker Prime Charger
  (160W, A2687) and the Anker Prime Power Bank (20K, 220W, A110G). Other
  Prime devices show as not supported yet.
- Live ports, power, battery, temperatures and health in a window, and in a
  menu bar panel whose title fields and cards you choose.
- Custom covers for the charger's screen from your own pictures, over
  Bluetooth, without an Anker login or the cloud.
- Devices stay connected on their own: they reconnect, re-arm a quiet
  stream, and the power bank is released while idle so it can sleep.
- English, Simplified Chinese and Japanese.
- Bluetooth through CoreBluetooth on macOS, WinRT on Windows and BlueZ on
  Linux. Linux has not been tried on real hardware yet.
