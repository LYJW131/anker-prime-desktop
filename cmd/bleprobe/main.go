// Command bleprobe exercises internal/ble against a real Anker Prime
// device: it lists FF09 advertisements, or connects to one by name, sends the
// first handshake step under the fixed initial key and prints the reply.
//
// On macOS it needs Bluetooth permission: run it from a terminal app that
// has it.
package main

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/LYJW131/anker-prime-desktop/internal/ble"
)

const (
	advertisedService = "FF09"
	service           = "8c850001-0302-41c5-b46e-cf057c562025"
	writeChar         = "8c850002-0302-41c5-b46e-cf057c562025"
	notifyChar        = "8c850003-0302-41c5-b46e-cf057c562025"

	// The longest frame the app writes, 0x0221.
	longestFrame = 193
)

// The handshake's fixed initial key, nonce and AAD: firmware constants, the
// same on every unit.
var (
	initialKey   = mustHex("b8ff7422955d4eb6d554a2c470280559")
	initialNonce = mustHex("6ba3e3f2f3a60f2971ce5d1f")
	gcmAAD       = mustHex("3322110077665544bbaa9988ffeeddcc")
)

func main() {
	scan := flag.Bool("scan", false, "list FF09 advertisements for -seconds")
	connect := flag.String("connect", "", "connect to the peripheral advertising this name and send handshake step 0x0001")
	ident := flag.String("id", "", "like -connect, by ID without scanning (a CoreBluetooth identifier on macOS, the address elsewhere)")
	seconds := flag.Int("seconds", 10, "scan duration for -scan")
	flag.Parse()
	if *ident != "" && *connect != "" || *scan == (*connect != "" || *ident != "") {
		flag.Usage()
		os.Exit(2)
	}

	c, err := ble.NewCentral()
	if err != nil {
		fail(err)
	}
	defer c.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	err = c.WaitPoweredOn(ctx)
	cancel()
	fmt.Printf("state=%v authorization=%v\n", c.State(), c.Authorization())
	if err != nil {
		fail(err)
	}
	if *scan {
		err = runScan(c, time.Duration(*seconds)*time.Second)
	} else {
		err = runConnect(c, *connect, *ident)
	}
	if err != nil {
		fail(err)
	}
}

func runScan(c *ble.Central, d time.Duration) error {
	err := c.Scan([]string{advertisedService}, func(a ble.Advert) {
		fmt.Printf("%s  name=%q rssi=%d connectable=%v services=%v mfr=%s svcdata=%s\n",
			time.Now().Format("15:04:05.000"), a.Name, a.RSSI, a.Connectable, a.ServiceUUIDs,
			fmtMfr(a.ManufacturerData), fmtSvcData(a.ServiceData))
		fmt.Printf("    id=%s\n", a.ID)
	})
	if err != nil {
		return err
	}
	time.Sleep(d)
	c.StopScan()
	return nil
}

func runConnect(c *ble.Central, name, ident string) error {
	if ident == "" {
		var err error
		if ident, err = scanFor(c, name); err != nil {
			return err
		}
	}
	return session(c, ident)
}

// scanFor scans until a peripheral advertises name and returns its id.
func scanFor(c *ble.Central, name string) (string, error) {
	found := make(chan ble.Advert, 1)
	err := c.Scan([]string{advertisedService}, func(a ble.Advert) {
		if a.Name == name {
			select {
			case found <- a:
			default:
			}
		}
	})
	if err != nil {
		return "", err
	}
	defer c.StopScan()
	select {
	case adv := <-found:
		fmt.Printf("found %s id=%s rssi=%d\n", adv.Name, adv.ID, adv.RSSI)
		return adv.ID, nil
	case <-time.After(30 * time.Second):
		return "", fmt.Errorf("%s not seen in 30 s", name)
	}
}

// session connects, discovers, subscribes, sends handshake step 0x0001
// and prints the reply.
func session(c *ble.Central, ident string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	start := time.Now()
	p, err := c.Connect(ctx, ident)
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer func() {
		p.Disconnect()
		fmt.Printf("disconnected: done=%v err=%v\n", isClosed(p.Done()), p.Err())
	}()
	fmt.Printf("connected in %v name=%q\n", time.Since(start).Round(time.Millisecond), p.Name())

	svcs, err := p.DiscoverServices(ctx, service)
	if err != nil {
		return fmt.Errorf("discover services: %w", err)
	}
	fmt.Printf("services: %v\n", svcs)
	chars, err := p.DiscoverCharacteristics(ctx, service, writeChar, notifyChar)
	if err != nil {
		return fmt.Errorf("discover characteristics: %w", err)
	}
	for _, ch := range chars {
		fmt.Printf("characteristic %s [%v]\n", ch.UUID, ch.Properties)
	}

	frames := make(chan []byte, 16)
	var buf []byte
	err = p.Subscribe(ctx, notifyChar, func(b []byte) {
		fmt.Printf("notify %d bytes: %X\n", len(b), b)
		buf = append(buf, b...)
		for {
			f, rest, ok := cutFrame(buf)
			if !ok {
				break
			}
			buf = rest
			select {
			case frames <- f:
			default:
			}
		}
	})
	if err != nil {
		return fmt.Errorf("subscribe: %w", err)
	}
	fmt.Println("subscribed")

	maxNR, maxR := p.MaxWriteLen(true), p.MaxWriteLen(false)
	fmt.Printf("MaxWriteLen(withoutResponse)=%d MaxWriteLen(withResponse)=%d (need >= %d) ok=%v\n",
		maxNR, maxR, longestFrame, maxNR >= longestFrame)

	frame, err := handshake0001(uint32(time.Now().Unix()))
	if err != nil {
		return err
	}
	fmt.Printf("tx 0x0001 %d bytes: %X\n", len(frame), frame)
	if err := p.Write(writeChar, frame, false); err != nil {
		return fmt.Errorf("write: %w", err)
	}
	select {
	case f := <-frames:
		fmt.Printf("rx frame %d bytes: %X\n", len(f), f)
		cmd, enc, body, err := parseFrame(f)
		if err != nil {
			return err
		}
		fmt.Printf("rx command=0x%04X encrypted=%v body=%X\n", cmd, enc, body)
		if enc {
			plain, err := gcm().Open(nil, initialNonce, body, gcmAAD)
			if err != nil {
				return fmt.Errorf("decrypt: %w", err)
			}
			fmt.Printf("rx plaintext: %X (expected 00A10101: %v)\n", plain, strings.EqualFold(hex.EncodeToString(plain), "00a10101"))
		}
	case <-p.Done():
		return fmt.Errorf("disconnected while waiting: %w", p.Err())
	case <-time.After(6 * time.Second):
		return errors.New("no reply in 6 s")
	}
	return nil
}

// handshake0001 builds FF 09 len16le 03 00 01 40 01 ct||tag xor, the
// encrypted session step 0x0001 carrying TLV A1 04 <epoch u32le>.
func handshake0001(epoch uint32) ([]byte, error) {
	plain := []byte{0xA1, 0x04, 0, 0, 0, 0}
	binary.LittleEndian.PutUint32(plain[2:], epoch)
	ct := gcm().Seal(nil, initialNonce, plain, gcmAAD)
	payload := append([]byte{0x03, 0x00, 0x01, 0x40, 0x01}, ct...)
	msg := []byte{0xFF, 0x09, 0, 0}
	binary.LittleEndian.PutUint16(msg[2:], uint16(len(payload)+5))
	msg = append(msg, payload...)
	var x byte
	for _, b := range msg {
		x ^= b
	}
	return append(msg, x), nil
}

// cutFrame splits one complete FF09 frame off the front of buf, by its
// length field; notifications may split frames.
func cutFrame(buf []byte) (frame, rest []byte, ok bool) {
	for len(buf) >= 4 {
		if buf[0] != 0xFF || buf[1] != 0x09 {
			buf = buf[1:]
			continue
		}
		n := int(binary.LittleEndian.Uint16(buf[2:]))
		if n < 10 || n > 4096 {
			buf = buf[1:]
			continue
		}
		if len(buf) < n {
			return nil, buf, false
		}
		return buf[:n], buf[n:], true
	}
	return nil, buf, false
}

func parseFrame(f []byte) (cmd uint16, encrypted bool, body []byte, err error) {
	if len(f) < 10 {
		return 0, false, nil, fmt.Errorf("short frame %X", f)
	}
	var x byte
	for _, b := range f[:len(f)-1] {
		x ^= b
	}
	if x != f[len(f)-1] {
		return 0, false, nil, fmt.Errorf("bad checksum in %X", f)
	}
	payload := f[4 : len(f)-1]
	hi, lo := payload[3], payload[4]
	cmd = uint16(hi&^(0x40|0x08))<<8 | uint16(lo)
	return cmd, hi&0x40 != 0, payload[5:], nil
}

func gcm() cipher.AEAD {
	block, err := aes.NewCipher(initialKey)
	if err != nil {
		panic(err)
	}
	a, err := cipher.NewGCM(block)
	if err != nil {
		panic(err)
	}
	return a
}

func fmtMfr(m map[uint16][]byte) string {
	if len(m) == 0 {
		return "-"
	}
	var parts []string
	for k, v := range m {
		parts = append(parts, fmt.Sprintf("%04X:%X", k, v))
	}
	sort.Strings(parts)
	return strings.Join(parts, ",")
}

func fmtSvcData(m map[string][]byte) string {
	if len(m) == 0 {
		return "-"
	}
	var parts []string
	for k, v := range m {
		parts = append(parts, fmt.Sprintf("%s:%X", k, v))
	}
	sort.Strings(parts)
	return strings.Join(parts, ",")
}

func isClosed(ch <-chan struct{}) bool {
	select {
	case <-ch:
		return true
	default:
		return false
	}
}

func mustHex(s string) []byte {
	b, err := hex.DecodeString(s)
	if err != nil {
		panic(err)
	}
	return b
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "bleprobe:", err)
	os.Exit(1)
}
