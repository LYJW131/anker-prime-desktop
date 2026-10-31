package ff09

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestHandshakeStepsShape(t *testing.T) {
	steps := HandshakeSteps(11, 22)
	wantCmds := []uint16{CmdHello, CmdParams, CmdIdentity, CmdSetup}
	eq(t, "count", len(steps), len(wantCmds))
	for i, s := range steps {
		eq(t, "command", s.Command, wantCmds[i])
		eq(t, "group", s.Group, GroupSession)
		eq(t, "expects reply", s.ExpectReply, true)
		eq(t, "pause", s.Pause, 100*time.Millisecond)
		eq(t, "timeout", s.ReplyWait(), ReplyTimeout)
		if got, _ := BuildTLV(s.TLVs); string(got) != string(s.Body) {
			t.Errorf("0x%04X: Body does not match TLVs", s.Command)
		}
	}
	// 0x0001, 0x0003 and 0x0029 share an epoch; 0x0005 has its own.
	epoch := func(s Step) uint32 { return leU32(t, s.TLVs[0].Value) }
	if epoch(steps[0]) != 11 || epoch(steps[1]) != 11 || epoch(steps[2]) != 11 || epoch(steps[3]) != 22 {
		t.Error("epochs not shared as in the Python")
	}
}

func TestSessionSteps(t *testing.T) {
	id := strings.Repeat("ab", 20)
	steps, err := SessionSteps(5, id)
	if err != nil {
		t.Fatal(err)
	}
	eq(t, "count", len(steps), 2)
	eq(t, "0x0022", steps[0].Command, CmdTimezone)
	eq(t, "0x0027", steps[1].Command, CmdAuth)
	for _, s := range steps {
		eq(t, cmdHex(s.Command)+" expects reply", s.ExpectReply, false)
		eq(t, cmdHex(s.Command)+" pause", s.Pause, 120*time.Millisecond)
	}
	if string(steps[1].TLVs[1].Value) != id || steps[1].TLVs[1].Type != 0xA2 {
		t.Error("the account id is A2, bare")
	}

	steps, err = SessionSteps(5, "")
	if err != nil || len(steps) != 1 {
		t.Errorf("no account id: %d steps, %v", len(steps), err)
	}
	for _, bad := range []string{"short", strings.Repeat("a", 41), strings.Repeat("é", 20)} {
		if _, err := SessionSteps(5, bad); !errors.Is(err, ErrAccountID) {
			t.Errorf("%q: %v", bad, err)
		}
	}
}

func TestArmSteps(t *testing.T) {
	id := strings.Repeat("0", 40)
	steps, err := ArmSteps(9, id, false)
	if err != nil || len(steps) != 1 || steps[0].Command != CmdStatus || steps[0].Pause != 200*time.Millisecond {
		t.Errorf("power bank: %v %v", steps, err)
	}
	steps, err = ArmSteps(9, id, true)
	if err != nil || len(steps) != 2 || steps[1].Command != CmdRealtime || steps[1].Group != GroupTelemetry {
		t.Errorf("charger: %v %v", steps, err)
	}
	steps, err = ArmSteps(9, "", true)
	if !errors.Is(err, ErrNoAccountID) || len(steps) != 1 {
		t.Errorf("charger without id: %d steps, %v", len(steps), err)
	}
	if _, err = ArmSteps(9, "bad", true); !errors.Is(err, ErrAccountID) {
		t.Errorf("bad id: %v", err)
	}
}

func TestKeyExchangeStep(t *testing.T) {
	s, err := KeyExchangeStep(make([]byte, 64))
	if err != nil {
		t.Fatal(err)
	}
	eq(t, "command", s.Command, CmdKeyExchange)
	eq(t, "expects reply", s.ExpectReply, true)
	eq(t, "pause", s.Pause, 120*time.Millisecond)
	eq(t, "body", len(s.Body), 66)
	if _, err := KeyExchangeStep(make([]byte, 65)); err == nil {
		t.Error("65-byte key accepted")
	}
}
