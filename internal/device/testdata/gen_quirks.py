#!/usr/bin/env python3
"""Regenerate quirks.json: hand-built frames for the decoder behaviours the
captures never exercise, run through the Python decoders. The Go test
TestQuirks replays them and must produce the same states.

    debugger/.venv/bin/python internal/device/testdata/gen_quirks.py
"""

import json
import pathlib
import sys

ROOT = pathlib.Path(__file__).resolve().parents[3]
sys.path.insert(0, str(ROOT / "debugger"))

from anker_prime_ble import charger, ff09, powerbank  # noqa: E402

PROFILES = {"charger": charger.PROFILE, "powerbank": powerbank.PROFILE}


def le16(v):
    return v.to_bytes(2, "little")


def tlv(*items):
    """Status byte 0x00, then the TLVs; each value is raw bytes with its tag."""
    return b"\x00" + ff09.build_tlv(list(items))


def cport(mode, mv, ma, cw, tail=b""):
    return b"\x04" + bytes([mode]) + le16(mv) + le16(ma) + le16(cw) + tail


def pbport(mode, v10, a10, w10, tail=b""):
    return b"\x04" + bytes([mode]) + le16(v10) + le16(a10) + le16(w10) + tail


APPLE = bytes.fromhex("0501")  # Apple PD in the second byte
CASES = {
    "charger: cable codes": ("charger", [
        (0x020A, tlv((0xA5, cport(1, 20088, 4455, 8935)), (0xAC, b"\x04\x00\x02\x02\x01"))),
        (0x020A, tlv((0xAD, b"\x04\x03\x00"), (0xAE, b"\x04\x01\x00"))),
        (0x020A, tlv((0xAC, b"\x04\x02\x00"), (0xAD, b"\x04\xAB\xCD"), (0xAE, b"\x04"))),
        (0x020A, tlv((0xA6, cport(2, 9000, 3000, 2700)), (0xAD, b"\x04\x00\x01\x02"), (0xAE, b"\x04\x00\x00\x00\x00"))),
        (0x020A, tlv((0xAC, b"\x00ab"), (0xAD, b"\x04\x00\x02\x03"))),
    ]),
    "charger: totals only count active ports, rounded": ("charger", [
        (0x020A, tlv((0xA5, cport(1, 5000, 1000, 1)), (0xA6, cport(1, 5000, 1000, 2)), (0xA7, cport(0, 5000, 0, 9999)))),
        (0x020A, tlv((0xA5, cport(1, 5000, 1000, 29), ), (0xA6, cport(1, 5000, 1000, 29)), (0xA7, cport(1, 5000, 1000, 29)))),
        (0x020A, tlv((0xA7, cport(0, 100, 0, 0)))),
        (0x020A, tlv((0xA5, b"\x04\x01\x01\x02"))),
        (0x020A, tlv((0xA5, cport(0, 100, 0, 7)))),
    ]),
    "charger: identity blocks": ("charger", [
        (0x020A, tlv((0xB4, b"\x04" + bytes.fromhex("ac051975" "fffafffb" "ac05feff")))),
        (0x020A, tlv((0xB5, b"\x04" + bytes.fromhex("0b000000" "ffffffff" "0c112233")))),
        (0x020A, tlv((0xB4, b"\x04" + bytes.fromhex("1a290b11" "0000ffff" "34120000")))),
        (0x020A, tlv((0xB5, b"\x04" + bytes.fromhex("01000000" "02000000" "00000000")))),
        (0x020A, tlv((0xB4, b"\x04" + bytes.fromhex("ac051975" "ac057319" "ac051771")))),
        (0x020A, tlv((0xB4, b"\x04" + bytes.fromhex("ac051975")))),
    ]),
    "charger: snapshot": ("charger", [
        (0x0029, b"\x03\x00\x00\x00\x00\x00" + ff09.build_tlv([(0xA3, b"v0.0.5.1"), (0xA4, b"ASHDJW7CF49200487")])),
        (0x0200, tlv((0xA2, b"\x00ASHDJWSERIAL001"), (0xA4, b"\x00A2687"), (0xA9, b"\x01\x37"), (0xFD, b"\x00tag...."),
                     (0xE1, b"\x04\x80\x03\x07\x5f\x00\x00\x00\x00\x00\x00"))),
        (0x0200, tlv((0xA2, b"\x00OTHER"), (0xA9, b"\x02\x64\x00"), (0xE1, b"\x04\x00\x03"))),
        (0x0200, tlv((0xA9, b"\x00ab"), (0xA4, b"\x00"))),
        (0x0A00, tlv((0xA5, cport(1, 12000, 2000, 2400)))),
        (0x0300, b"\xA5\x08" + cport(0, 0, 0, 0)[0:1] + bytes(7)),
    ]),
    "powerbank: ports, sticky power, stale dock": ("powerbank", [
        (0x0300, tlv((0xA7, pbport(1, 200, 30, 600)), (0xA8, pbport(2, 201, 34, 688, bytes([0xFF, 0x07]) + b"\xff" * 4)),
                     (0xA9, pbport(0, 0, 0, 79, bytes([0xFF, 0x00]) + b"\xff" * 4)), (0xAC, pbport(0, 50, 0, 0)),
                     (0xA5, b"\x04" + le16(688)), (0xA6, b"\x04" + le16(0)))),
        (0x0300, tlv((0xA5, b"\x04" + le16(0)), (0xA8, pbport(0, 0, 0, 688, bytes([0xFF, 0x00]) + b"\xff" * 4)))),
        (0x0300, tlv((0xAC, pbport(3, 50, 1, 2)), (0xA6, b"\x04\x01"))),
        (0x0300, tlv((0xA5, b"\x04" + le16(1)), (0xA7, b"\x04\x01\x02"))),
        (0x0300, tlv((0xA2, b"\x04" + bytes([54, 43])), (0xA3, b"\x04\x01\x09\x30"), (0xA4, b"\x01\x02"), (0xAF, b"\x01\x2d"), (0xB0, b"\x01"))),
        (0x0300, tlv((0xA2, b"\x04" + bytes([54, 100])), (0xA3, b"\x04\x01\x09"), (0xA4, b"\x31"))),
        (0x0300, tlv((0xA1, b"\x04\x2a"), (0xA4, b"\x01\x01"), (0xA2, b"\x04\x64\x00"))),
        (0x0300, tlv((0xA2, b"\x04\x01"), (0xFE, b"\x01\x00"), (0xB1, b"\x01\x05"))),
    ]),
    "powerbank: snapshot remap": ("powerbank", [
        (0x0200, tlv((0xA1, b"\x04\x01"), (0xA6, b"\x04" + bytes([81, 7])), (0xA7, b"\x04\x01\x00\x25"),
                     (0xA9, b"\x01\x5f"), (0xAA, b"\x01\x1e"), (0xAC, b"\x02" + le16(1200)),
                     (0xE2, b"\x04\x01" + le16(1500)), (0xA2, b"\x04\x01\x02\x03"), (0xA3, b"\x01\x07"), (0xAB, b"\x01\x00"),
                     (0xB2, b"\x04\x00" + le16(1)), (0xB3, b"\x04\x00" + le16(2)),
                     (0xB4, pbport(0, 0, 0, 1)), (0xB5, pbport(1, 201, 35, 700, bytes([0xFF, 0x07]) + b"\xff" * 4)),
                     (0xB6, pbport(0, 0, 0, 3, bytes([0xFF, 0x00]) + b"\xff" * 4)), (0xB9, pbport(0, 50, 0, 0)),
                     (0xBC, b"\x01\x2c"), (0xBD, b"\x01\x2d"), (0xBE, b"\x01\x05"), (0xE0, b"\x01\x01"))),
        (0x0200, tlv((0xA9, b"\x04\x01\x02"), (0xAA, b"\x00ab"), (0xAC, b"\x01"), (0xE2, b"\x04\x00\x01"))),
        (0x0200, tlv((0xA9, b"\x02" + le16(77)), (0xAA, b"\x03" + (65).to_bytes(4, "little")), (0xAC, b"\x01\x14"))),
        (0x0200, b""),
        (0x0200, tlv((0xB2, b"\x04" + le16(0)))),
    ]),
    "powerbank: identity": ("powerbank", [
        (0x0029, tlv((0xA2, b"Charging"), (0xA3, b"v0.0.5.2"), (0xA4, b"AJ7DLCH0F49600286"), (0xA5, bytes.fromhex("7ce91378cada") + b"AJ7DLCH0F49"))),
        (0x0029, tlv((0xA3, b"zzz"), (0xA4, b"short"), (0xA5, b"ABCDEFGHIJ"))),
        (0x0029, b"\x03\x00\xff\xff\xff\xff" + ff09.build_tlv([(0xA3, b"9.9.9")])),
    ]),
}


def main() -> None:
    out = []
    for name, (device, frames) in CASES.items():
        profile = PROFILES[device]
        state = profile.new_state()
        recorded = []
        for command, plain in frames:
            if command == 0x0029:
                profile.parse_identity(plain, state)
            elif command in profile.snapshot_commands:
                profile.parse_snapshot(plain, state)
            else:
                profile.parse_realtime(plain, state)
            recorded.append({"command": command, "plain": plain.hex(), "state": state.to_dict()})
        out.append({"name": name, "device": device, "frames": recorded})
    path = pathlib.Path(__file__).with_name("quirks.json")
    path.write_text(json.dumps(out, indent=1) + "\n")
    print(f"wrote {path.relative_to(ROOT)}: {sum(len(c['frames']) for c in out)} frames")


if __name__ == "__main__":
    main()
