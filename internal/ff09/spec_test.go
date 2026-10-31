package ff09

import (
	"encoding/hex"
	"testing"
	"time"
)

func eq[T comparable](t *testing.T, name string, got, want T) {
	t.Helper()
	if got != want {
		t.Errorf("%s: got %v, spec has %v", name, got, want)
	}
}

// TestConstantsMatchSpec fails when a Go constant drifts from the value the
// Python modules export into spec/protocol.json.
func TestConstantsMatchSpec(t *testing.T) {
	s := loadSpec(t)
	tr := s.Transport
	eq(t, "ServiceUUID", ServiceUUID, tr.ServiceUUID)
	eq(t, "WriteCharUUID", WriteCharUUID, tr.WriteUUID)
	eq(t, "NotifyCharUUID", NotifyCharUUID, tr.NotifyUUID)
	eq(t, "AdvertisedServiceUUID", AdvertisedServiceUUID, tr.AdvertisedUUID)
	eq(t, "InitialKey", hex.EncodeToString(InitialKey), tr.InitialKey)
	eq(t, "InitialIV", hex.EncodeToString(InitialIV), tr.InitialIV)
	eq(t, "GCMAAD", hex.EncodeToString(GCMAAD), tr.GCMAAD)
	eq(t, "FlagEncrypted", FlagEncrypted, tr.FlagEncrypted)
	eq(t, "FlagAck", FlagAck, tr.FlagAck)
	eq(t, "GroupSession", GroupSession, tr.GroupSession)
	eq(t, "GroupTelemetry", GroupTelemetry, tr.GroupTelemetry)
	eq(t, "CmdStatus", CmdStatus, tr.CommandStatus)
	eq(t, "CmdRealtime", CmdRealtime, tr.CommandRealtim)
	eq(t, "ChargerNamePrefix", ChargerNamePrefix, s.Devices.Charger.NamePrefix)

	h := s.Handshake
	eq(t, "Cmd0022A3", hex.EncodeToString(Cmd0022A3), h.Cmd0022A3)
	eq(t, "Cmd0022A5", string(Cmd0022A5), h.Cmd0022A5)
	eq(t, "Cmd020AA2", hex.EncodeToString(Cmd020AA2), h.Cmd020AA2)
	eq(t, "tx header", hex.EncodeToString(BuildFrame(0, 0, nil)[4:6]), h.TxHeader)
	eq(t, "account id length", 40, h.AccountIDLength)
	eq(t, "public key length", len(mustEcdh(t).PublicCoordinates()), h.PublicKeyLength)

	c := s.Cover
	eq(t, "CmdSelectCover", CmdSelectCover, c.CommandSelect)
	eq(t, "CmdTransferStart", CmdTransferStart, c.CommandTransferStart)
	eq(t, "CmdTransferData", CmdTransferData, c.CommandTransferData)
	eq(t, "CoverTypeCustom", CoverTypeCustom, c.TypeCustom)
	eq(t, "ChunkSize", ChunkSize, c.ChunkSize)
	eq(t, "AckEvery", AckEvery, c.AckEvery)
	eq(t, "cover url key", string(SelectCoverTLVs(0, 0, 0)[4].Value[1:]), c.URLKey)

	codes := map[string]byte{
		"ok": ResultOK, "bad_body": ResultBadBody, "transfer_complete": ResultTransferComplete,
		"not_on_device": ResultNotOnDevice, "overflow": ResultOverflow,
	}
	if len(codes) != len(c.ResultCodes) {
		t.Errorf("result codes: Go has %d, spec has %d", len(codes), len(c.ResultCodes))
	}
	for name, want := range c.ResultCodes {
		got, ok := codes[name]
		if !ok || int(got) != want {
			t.Errorf("result code %s: Go %v (present %v), spec %d", name, got, ok, want)
		}
	}

	ms := func(d time.Duration) int { return int(d / time.Millisecond) }
	timing := map[string]int{
		"pause_handshake": ms(PauseHandshake), "pause_session": ms(PauseSession),
		"pause_status": ms(PauseStatus), "pause_select": ms(PauseSelect),
		"pause_transfer": ms(PauseTransfer), "pause_chunk": ms(PauseChunk),
		"pause_after_chunks": ms(PauseAfterChunks), "reply_timeout": ms(ReplyTimeout),
		"chunk_ack_timeout": ms(ChunkAckTimeout),
	}
	for name, want := range s.TimingMS {
		if got, ok := timing[name]; !ok || got != want {
			t.Errorf("timing %s: Go %d (present %v), spec %d", name, got, ok, want)
		}
	}
	if len(timing) != len(s.TimingMS) {
		t.Errorf("timing: Go has %d entries, spec has %d", len(timing), len(s.TimingMS))
	}
}

// TestRequestsMatchPython builds every request vector spec/protocol.json holds
// (the Python's output for fixed inputs: epoch 1, and 2 for 0x0005) with the
// Go builders and compares the plaintext bodies byte for byte.
func TestRequestsMatchPython(t *testing.T) {
	s := loadSpec(t)
	acct := s.Vectors.AccountID
	pub := unhex(t, s.Vectors.PublicKey)

	built := map[string]Step{}
	add := func(name string, st Step) { built[name] = st }

	for _, st := range HandshakeSteps(1, 2) {
		add("handshake_"+cmdHex(st.Command), st)
	}
	ke, err := KeyExchangeStep(pub)
	if err != nil {
		t.Fatal(err)
	}
	add("key_exchange", ke)
	sess, err := SessionSteps(1, acct)
	if err != nil {
		t.Fatal(err)
	}
	for _, st := range sess {
		add("session_"+cmdHex(st.Command), st)
	}
	arm, err := ArmSteps(1, acct, true)
	if err != nil {
		t.Fatal(err)
	}
	add("status_probe", arm[0])
	add("realtime_probe", arm[1])

	chunk := make([]byte, ChunkSize)
	for i := range chunk {
		chunk[i] = byte(i)
	}
	body := func(b []byte, err error) Step {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
		return Step{Group: GroupTelemetry, Body: b}
	}
	add("cover_select", Step{Group: GroupTelemetry, Command: CmdSelectCover, Body: SelectCoverBody(24551, 0x1DA2DDCA, 1)})
	add("cover_transfer_start", Step{Group: GroupTelemetry, Command: CmdTransferStart, Body: TransferStartBody(45470, 0xC126BFEF, 25397, 163, 1)})
	st := body(ChunkBody(5, chunk))
	st.Command = CmdTransferData
	add("cover_chunk", st)
	st = body(ChunkBody(162, chunk[:10]))
	st.Command = CmdTransferData
	add("cover_chunk_short", st)

	if len(s.Vectors.Requests) != len(built) {
		t.Errorf("spec has %d request vectors, Go built %d", len(s.Vectors.Requests), len(built))
	}
	for _, v := range s.Vectors.Requests {
		got, ok := built[v.Name]
		if !ok {
			t.Errorf("%s: no Go builder", v.Name)
			continue
		}
		if hex.EncodeToString(got.Body) != v.Body {
			t.Errorf("%s: body\n got  %x\n want %s", v.Name, got.Body, v.Body)
		}
		eq(t, v.Name+" group", got.Group, v.Group)
		eq(t, v.Name+" command", got.Command, v.Command)
		if v.ExpectsResp != nil {
			eq(t, v.Name+" expects reply", got.ExpectReply, *v.ExpectsResp)
		}
	}
}

func cmdHex(c uint16) string {
	const d = "0123456789abcdef"
	return string([]byte{d[c>>12&15], d[c>>8&15], d[c>>4&15], d[c&15]})
}

func mustEcdh(t testing.TB) *Ecdh {
	t.Helper()
	e, err := NewEcdh()
	if err != nil {
		t.Fatal(err)
	}
	return e
}
