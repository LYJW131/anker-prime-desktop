package ff09

import (
	"errors"
	"fmt"
	"time"
)

// Step is one host-to-device command a session driver sends. The driver
// encrypts Body with the cipher that is current when the step is sent, frames
// it with Group and Command, writes it, waits for a reply when ExpectReply is
// set, then sleeps Pause.
type Step struct {
	Group       uint8
	Command     uint16
	TLVs        []TLV
	Body        []byte // TLVs encoded: the plaintext to encrypt
	ExpectReply bool
	Timeout     time.Duration // reply wait; 0 means ReplyTimeout
	Pause       time.Duration // wait after the step
}

// Frame encrypts the step's body with c and wraps it for the wire.
func (s Step) Frame(c *Crypto) []byte {
	return c.BuildFrame(s.Group, s.Command, s.Body)
}

// ReplyWait is how long to wait for the reply.
func (s Step) ReplyWait() time.Duration {
	if s.Timeout > 0 {
		return s.Timeout
	}
	return ReplyTimeout
}

func newStep(group uint8, command uint16, tlvs []TLV, expect bool, pause time.Duration) (Step, error) {
	body, err := BuildTLV(tlvs)
	if err != nil {
		return Step{}, err
	}
	return Step{Group: group, Command: command, TLVs: tlvs, Body: body, ExpectReply: expect, Pause: pause}, nil
}

// ErrAccountID reports an account id that is not 40 ASCII characters.
var ErrAccountID = errors.New("ff09: account id must be exactly 40 ASCII characters")

// ErrNoAccountID reports a request that needs the account id when none is set.
var ErrNoAccountID = errors.New("ff09: this request needs the account id")

func checkAccountID(id string) error {
	if len(id) != 40 {
		return ErrAccountID
	}
	for i := 0; i < len(id); i++ {
		if id[i] >= 0x80 {
			return ErrAccountID
		}
	}
	return nil
}

// HandshakeSteps are the four commands sent on the initial key: 0x0001,
// 0x0003 and 0x0029 share epoch1, 0x0005 carries epoch2. Each expects a reply
// (the 0x0029 reply is the device identity).
func HandshakeSteps(epoch1, epoch2 uint32) []Step {
	ts1, ts2 := EpochBytes(epoch1), EpochBytes(epoch2)
	g := GroupSession
	return mustSteps(
		stepOf(g, CmdHello, []TLV{{0xA1, ts1}}, true, PauseHandshake),
		stepOf(g, CmdParams, []TLV{{0xA1, ts1}, {0xA3, []byte{0x20}}, {0xA4, []byte{0x00, 0xF0}}}, true, PauseHandshake),
		stepOf(g, CmdIdentity, []TLV{{0xA1, ts1}}, true, PauseHandshake),
		stepOf(g, CmdSetup, []TLV{{0xA1, ts2}, {0xA3, []byte{0x20}}, {0xA4, []byte{0x29, 0x01}}, {0xA5, []byte{0x44}}, {0xA6, []byte{0x02}}}, true, PauseHandshake),
	)
}

// KeyExchangeStep is 0x0021 carrying our 64-byte X||Y. It is sent on the
// initial key. When its reply arrives, derive the session key from the A1 of
// the reply (FindDevicePublicKey) and switch the cipher before the next step.
func KeyExchangeStep(publicCoordinates []byte) (Step, error) {
	if len(publicCoordinates) != 64 {
		return Step{}, fmt.Errorf("ff09: public key must be 64 bytes, got %d", len(publicCoordinates))
	}
	return newStep(GroupSession, CmdKeyExchange, []TLV{{0xA1, publicCoordinates}}, true, PauseSession)
}

// SessionSteps are sent on the session key: 0x0022 (time zone) and, when
// accountID is not empty, 0x0027 carrying it. Neither expects a reply. The
// charger streams nothing without the account id, and the power bank drops the
// link after 26 seconds without it.
func SessionSteps(epoch uint32, accountID string) ([]Step, error) {
	ts := EpochBytes(epoch)
	g := GroupSession
	s22, err := newStep(g, CmdTimezone, []TLV{{0xA1, ts}, {0xA3, Cmd0022A3}, {0xA5, Cmd0022A5}}, false, PauseSession)
	if err != nil {
		return nil, err
	}
	steps := []Step{s22}
	if accountID != "" {
		if err := checkAccountID(accountID); err != nil {
			return nil, err
		}
		s27, err := newStep(g, CmdAuth, []TLV{{0xA1, ts}, {0xA2, []byte(accountID)}}, false, PauseSession)
		if err != nil {
			return nil, err
		}
		steps = append(steps, s27)
	}
	return steps, nil
}

// StatusProbeBody is the 0x0200 request: A1 21, FE the bare epoch.
func StatusProbeBody(epoch uint32) []byte {
	return mustBody([]TLV{{0xA1, []byte{0x21}}, {0xFE, EpochBytes(epoch)}})
}

// RealtimeProbeBody is the charger's 0x020A request.
func RealtimeProbeBody(accountID string, epoch uint32) ([]byte, error) {
	tlvs, err := realtimeProbeTLVs(accountID, epoch)
	if err != nil {
		return nil, err
	}
	return BuildTLV(tlvs)
}

func realtimeProbeTLVs(accountID string, epoch uint32) ([]TLV, error) {
	if err := checkAccountID(accountID); err != nil {
		return nil, err
	}
	return []TLV{
		{0xA1, []byte{0x21}},
		{0xA2, Cmd020AA2},
		{0xA3, append([]byte{0x04}, accountID...)},
		{0xA5, []byte{0x01, 0x01}},
		{0xFE, EpochBytes(epoch)},
	}, nil
}

// ArmSteps ask for a snapshot (0x0200) and, with realtime set, the charger's
// realtime frame (0x020A). The driver sends them at the end of the handshake
// and again when the stream goes quiet; both are reads. The charger needs the
// account id for 0x020A: without one, ArmSteps still returns the 0x0200 step
// along with ErrNoAccountID.
func ArmSteps(epoch uint32, accountID string, realtime bool) ([]Step, error) {
	g := GroupTelemetry
	status, err := newStep(g, CmdStatus, []TLV{{0xA1, []byte{0x21}}, {0xFE, EpochBytes(epoch)}}, false, PauseStatus)
	if err != nil {
		return nil, err
	}
	steps := []Step{status}
	if !realtime {
		return steps, nil
	}
	if accountID == "" {
		return steps, ErrNoAccountID
	}
	tlvs, err := realtimeProbeTLVs(accountID, epoch)
	if err != nil {
		return steps, err
	}
	rt, err := newStep(g, CmdRealtime, tlvs, false, 0)
	if err != nil {
		return steps, err
	}
	return append(steps, rt), nil
}

type stepResult struct {
	step Step
	err  error
}

func stepOf(group uint8, command uint16, tlvs []TLV, expect bool, pause time.Duration) stepResult {
	s, err := newStep(group, command, tlvs, expect, pause)
	return stepResult{s, err}
}

func mustSteps(rs ...stepResult) []Step {
	out := make([]Step, len(rs))
	for i, r := range rs {
		if r.err != nil {
			panic(r.err) // only fixed values reach here
		}
		out[i] = r.step
	}
	return out
}

func mustBody(tlvs []TLV) []byte {
	b, err := BuildTLV(tlvs)
	if err != nil {
		panic(err)
	}
	return b
}
