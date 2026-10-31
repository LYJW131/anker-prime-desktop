// Package ff09 is the FF09 BLE transport both Anker Prime devices speak:
// framing, TLV bodies, AES-128-GCM, the ECDH P-256 session handshake and the
// cover-transfer commands. It knows nothing about ports or watts; package
// device decodes what the TLVs carry.
//
// It is a port of debugger/anker_prime_ble/ff09.py and the cover commands of
// charger.py. spec/protocol.json, generated from the Python, pins every
// constant here; the tests fail if the two drift.
//
// Nothing in this package does I/O or reads the clock: builders take the epoch
// as an argument, and Step lists are run by the link package.
package ff09

import (
	"encoding/hex"
	"time"
)

// GATT.
const (
	ServiceUUID    = "8c850001-0302-41c5-b46e-cf057c562025"
	WriteCharUUID  = "8c850002-0302-41c5-b46e-cf057c562025" // write without response
	NotifyCharUUID = "8c850003-0302-41c5-b46e-cf057c562025"

	// AdvertisedServiceUUID is 0xFF09 on the Bluetooth base UUID. The whole
	// Prime line advertises it, so it names the family, not a product.
	AdvertisedServiceUUID = "0000ff09-0000-1000-8000-00805f9b34fb"

	// ChargerNamePrefix starts the A2687's advertised name. The power bank
	// advertises its bare serial, so anything else on FF09 is a power bank.
	ChargerNamePrefix = "ASHDJW"
)

// Crypto constants. They are fixed in the firmware of every unit and only
// protect the handshake; the session key comes from ECDH.
var (
	InitialKey = mustHex("b8ff7422955d4eb6d554a2c470280559")
	InitialIV  = mustHex("6ba3e3f2f3a60f2971ce5d1f")
	GCMAAD     = mustHex("3322110077665544bbaa9988ffeeddcc")
)

// Constant TLV values of the handshake.
var (
	// Cmd0022A3 is the 0x0022 A3 value, Cmd0022A5 its A5 (a POSIX time zone,
	// fixed whatever zone this Mac is in), Cmd020AA2 the 0x020A A2.
	Cmd0022A3 = mustHex("808fffff")
	Cmd0022A5 = []byte("CST-8")
	Cmd020AA2 = mustHex("045553")
)

// Command groups.
const (
	GroupSession   uint8 = 0x01 // handshake and session
	GroupTelemetry uint8 = 0x0F // telemetry, settings, covers
)

// Frame flags in the high byte of the command.
const (
	FlagEncrypted uint8 = 0x40 // the body is AES-GCM ciphertext
	FlagAck       uint8 = 0x08 // a reply
)

// Commands.
const (
	CmdHello         uint16 = 0x0001
	CmdParams        uint16 = 0x0003
	CmdSetup         uint16 = 0x0005
	CmdKeyExchange   uint16 = 0x0021
	CmdTimezone      uint16 = 0x0022
	CmdAuth          uint16 = 0x0027
	CmdIdentity      uint16 = 0x0029
	CmdStatus        uint16 = 0x0200
	CmdRealtime      uint16 = 0x020A
	CmdSelectCover   uint16 = 0x021F
	CmdTransferStart uint16 = 0x0220
	CmdTransferData  uint16 = 0x0221
	CmdPush          uint16 = 0x0300 // pushed by the device about once a second
)

// Cover transfer parameters.
const (
	CoverTypeCustom uint8 = 3   // 0x021F A3: a custom picture
	ChunkSize             = 156 // JPEG bytes per 0x0221 frame
	AckEvery              = 10  // the device acks every Nth chunk and the last
)

// Write-ACK result codes: the first plaintext byte of a reply to 0x021F,
// 0x0220 and 0x0221.
const (
	ResultOK               byte = 0x00
	ResultBadBody          byte = 0x04 // empty or malformed body
	ResultTransferComplete byte = 0x10 // on the last chunk's ack
	ResultNotOnDevice      byte = 0x11 // 0x021F: no pixels under that id
	ResultOverflow         byte = 0x12 // chunks sent without waiting for acks
)

// Pauses and timeouts of the session driver, from debugger session.py and
// cover.py.
const (
	PauseHandshake   = 100 * time.Millisecond // after steps 0x0001..0x0005
	PauseSession     = 120 * time.Millisecond // after 0x0021 and the post-session steps
	PauseStatus      = 200 * time.Millisecond // after the 0x0200 probe
	PauseSelect      = 150 * time.Millisecond // after 0x021F
	PauseTransfer    = 200 * time.Millisecond // after 0x0220
	PauseChunk       = 40 * time.Millisecond  // after a chunk that is not acked
	PauseAfterChunks = 400 * time.Millisecond // after the last chunk
	ReplyTimeout     = 6 * time.Second        // default wait for a reply
	ChunkAckTimeout  = 5 * time.Second        // wait for an acked chunk
	LastAckTimeout   = 10 * time.Second       // wait for the last chunk's ack: the charger writes flash first (3.6 s seen for 8.6 KB)
)

func mustHex(s string) []byte {
	b, err := hex.DecodeString(s)
	if err != nil {
		panic(err)
	}
	return b
}
