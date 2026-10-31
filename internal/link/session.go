package link

import (
	"context"
	"fmt"
	"time"

	"github.com/LYJW131/anker-prime-mac/internal/ble"
	"github.com/LYJW131/anker-prime-mac/internal/device"
	"github.com/LYJW131/anker-prime-mac/internal/ff09"
)

// session is one connection: it owns the cipher, the reassembly buffer and
// the decoder, and only its goroutine touches them.
type session struct {
	d   *Device
	p   *ble.Peripheral
	uid string

	crypto *ff09.Crypto
	asm    ff09.Assembler
	notes  chan []byte

	dec device.Decoder

	lastFrame time.Time // any frame, decoded or not
	rearmed   bool      // re-armed once in the current silence
	idleSince time.Time // power bank: when flow last stopped
	coverID   int       // charger: E1, -1 until reported
}

// frame is one decrypted frame.
type frame struct {
	cmd   uint16
	plain []byte
}

func newSession(d *Device, p *ble.Peripheral, uid string) *session {
	dec, _ := device.NewDecoder(d.cfg.Model.Kind) // the backend starts links only for registered models
	return &session{d: d, p: p, uid: uid, crypto: ff09.NewCrypto(), notes: make(chan []byte, 512), coverID: -1, dec: dec}
}

func (s *session) run(ctx context.Context, probing bool) error {
	s.d.setStatus(func(st *Status) { st.Phase, st.Err = PhaseHandshaking, nil })
	if err := s.open(ctx); err != nil {
		return err
	}
	if err := s.handshake(ctx); err != nil {
		return &HandshakeError{Err: err}
	}
	if err := s.arm(ctx); err != nil {
		return err
	}
	s.d.setStatus(func(st *Status) { st.Phase, st.Err = PhaseConnected, nil })
	s.d.cfg.Log.Info("connected", "id", s.p.ID())

	idle := idleAfter
	if probing {
		idle = probeIdle
	}
	s.idleSince = time.Now()
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-s.p.Done():
			return s.p.Err()
		case chunk := <-s.notes:
			s.feed(chunk)
		case j := <-s.d.jobs:
			j.done <- j.run(ctx, s)
		case now := <-tick.C:
			silent := now.Sub(s.lastFrame)
			switch {
			case silent >= stallAfter:
				return ErrStalled
			case silent >= rearmAfter && !s.rearmed:
				s.rearmed = true
				s.d.cfg.Log.Info("stream quiet, re-arming")
				if err := s.arm(ctx); err != nil {
					return err
				}
			}
			if s.d.cfg.Model.IdleSleep && s.d.idleSleep() && now.Sub(s.idleSince) >= idle {
				return errIdle
			}
		}
	}
}

func (d *Device) idleSleep() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.want.IdleSleep
}

func (s *session) open(ctx context.Context) error {
	dctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if _, err := s.p.DiscoverServices(dctx, ff09.ServiceUUID); err != nil {
		return err
	}
	if _, err := s.p.DiscoverCharacteristics(dctx, ff09.ServiceUUID, ff09.WriteCharUUID, ff09.NotifyCharUUID); err != nil {
		return err
	}
	if n := s.p.MaxWriteLen(true); n < 193 {
		s.d.cfg.Log.Warn("write length below a cover chunk frame", "max", n)
	}
	return s.p.Subscribe(dctx, ff09.NotifyCharUUID, func(b []byte) {
		select {
		case s.notes <- append([]byte(nil), b...):
		case <-s.p.Done():
		}
	})
}

func (s *session) handshake(ctx context.Context) error {
	time.Sleep(300 * time.Millisecond) // the notifications settle, as the official app waits
	now := time.Now()
	for _, st := range ff09.HandshakeSteps(ff09.EpochOf(now), ff09.EpochOf(now.Add(time.Second))) {
		if _, err := s.step(ctx, st); err != nil {
			return err
		}
	}
	ecdh, err := ff09.NewEcdh()
	if err != nil {
		return err
	}
	kx, err := ff09.KeyExchangeStep(ecdh.PublicCoordinates())
	if err != nil {
		return err
	}
	reply, err := s.step(ctx, kx)
	if err != nil {
		return err
	}
	if err := s.crypto.EstablishSession(ecdh, reply); err != nil {
		return err
	}
	steps, err := ff09.SessionSteps(ff09.EpochOf(time.Now()), s.uid)
	if err != nil {
		return err
	}
	for _, st := range steps {
		if _, err := s.step(ctx, st); err != nil {
			return err
		}
	}
	return nil
}

// arm asks for the stream: the status snapshot, and the charger's
// realtime request.
func (s *session) arm(ctx context.Context) error {
	steps, err := ff09.ArmSteps(ff09.EpochOf(time.Now()), s.uid, s.d.cfg.Model.Realtime)
	if err != nil {
		return err
	}
	for _, st := range steps {
		if _, err := s.step(ctx, st); err != nil {
			return err
		}
	}
	return nil
}

// step sends one step, waits for its reply when it expects one, then
// pauses, decoding whatever else arrives meanwhile. It returns the reply's
// plaintext.
func (s *session) step(ctx context.Context, st ff09.Step) ([]byte, error) {
	if err := s.p.WriteContext(ctx, ff09.WriteCharUUID, st.Frame(s.crypto), false); err != nil {
		return nil, err
	}
	var reply []byte
	if st.ExpectReply {
		var err error
		if reply, err = s.await(ctx, st.Command, st.ReplyWait()); err != nil {
			return nil, fmt.Errorf("0x%04X: %w", st.Command, err)
		}
	}
	if st.Pause > 0 {
		s.drain(ctx, st.Pause)
	}
	return reply, nil
}

// await decodes frames until one answers cmd.
func (s *session) await(ctx context.Context, cmd uint16, wait time.Duration) ([]byte, error) {
	t := time.NewTimer(wait)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-s.p.Done():
			return nil, s.p.Err()
		case <-t.C:
			return nil, ErrNoResponse
		case chunk := <-s.notes:
			for _, f := range s.feed(chunk) {
				if f.cmd == cmd {
					return f.plain, nil
				}
			}
		}
	}
}

// drain decodes frames for d.
func (s *session) drain(ctx context.Context, d time.Duration) {
	t := time.NewTimer(d)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.p.Done():
			return
		case <-t.C:
			return
		case chunk := <-s.notes:
			s.feed(chunk)
		}
	}
}

// feed reassembles a notification, decrypts and decodes its frames, and
// returns them for reply matching.
func (s *session) feed(chunk []byte) []frame {
	var out []frame
	for _, raw := range s.asm.Feed(chunk) {
		f, ok := ff09.ParseFrame(raw)
		if !ok {
			continue
		}
		plain := f.Body
		if f.Encrypted {
			var err error
			if plain, err = s.crypto.Open(f); err != nil {
				s.d.cfg.Log.Debug("undecryptable frame", "cmd", fmt.Sprintf("0x%04X", f.Command))
				continue
			}
		}
		now := time.Now()
		s.lastFrame, s.rearmed = now, false
		out = append(out, frame{cmd: f.Command, plain: plain})
		s.decode(f.Command, plain, now)
	}
	return out
}

func (s *session) decode(cmd uint16, plain []byte, now time.Time) {
	if s.dec == nil || !s.dec.Handles(cmd) {
		return
	}
	changed := s.dec.Apply(cmd, plain)
	st := s.dec.Snapshot()
	switch st := st.(type) {
	case device.Charger:
		if st.ScreensaverID != nil {
			s.coverID = *st.ScreensaverID
		}
	case device.PowerBank:
		if busy(st) {
			s.idleSince = now
		}
	}
	s.d.setStatus(func(x *Status) { x.LastFrame = now })
	if changed && s.d.cfg.OnState != nil {
		s.d.cfg.OnState(st)
	}
}

// busy reports whether power flows through the power bank. Leftover
// voltage on an idle port and a stale dock reading do not count.
func busy(p device.PowerBank) bool {
	if p.Charging {
		return true
	}
	if p.InputPowerW != nil && *p.InputPowerW > busyThreshold {
		return true
	}
	if p.OutputPowerW != nil && *p.OutputPowerW > busyThreshold {
		return true
	}
	for _, port := range p.Ports {
		if port.Active {
			return true
		}
	}
	return false
}

// selectCover shows id, pushing jpeg first when the charger lacks it.
func (s *session) selectCover(ctx context.Context, id, hash uint32, jpeg []byte, progress func(done, total int)) error {
	want := int(id & 0xFFFF) // E1 carries the low 16 bits
	if s.coverID == want {
		return nil
	}
	sel := ff09.Step{Group: ff09.GroupTelemetry, Command: ff09.CmdSelectCover, Body: ff09.SelectCoverBody(id, hash, ff09.EpochOf(time.Now())), ExpectReply: true, Pause: ff09.PauseSelect}
	reply, err := s.step(ctx, sel)
	if err != nil {
		return err
	}
	if ack, err := ff09.ParseAck(reply); err == nil && ack.Result == ff09.ResultOK {
		// An OK only means the command was taken: the screen switching
		// is the proof.
		if s.awaitCover(ctx, want, selectSettle) {
			return nil
		}
	}
	if len(jpeg) == 0 {
		return ErrCoverNoData
	}
	steps, err := ff09.CoverSteps(jpeg, id, hash, ff09.EpochOf(time.Now()), false)
	if err != nil {
		return err
	}
	total := len(steps) - 1
	sent := 0
	for _, st := range steps {
		reply, err := s.step(ctx, st)
		if err != nil {
			return err
		}
		if st.ExpectReply {
			ack, err := ff09.ParseAck(reply)
			if err != nil {
				return err
			}
			if !ack.OK() {
				return &CoverRejectedError{Command: st.Command, Result: ack.Result}
			}
		}
		if st.Command == ff09.CmdTransferData {
			sent++
			if progress != nil {
				progress(sent, total)
			}
		}
	}
	if err := s.arm(ctx); err != nil {
		return err
	}
	if !s.awaitCover(ctx, want, pushSettle) {
		return ErrCoverNotSwitched
	}
	return nil
}

// awaitCover decodes frames until E1 shows want, re-asking for the
// snapshot that carries it.
func (s *session) awaitCover(ctx context.Context, want int, within time.Duration) bool {
	deadline := time.Now().Add(within)
	next := time.Now()
	for time.Now().Before(deadline) {
		if s.coverID == want {
			return true
		}
		if time.Now().After(next) {
			if err := s.arm(ctx); err != nil {
				return false
			}
			next = time.Now().Add(time.Second)
		}
		s.drain(ctx, 200*time.Millisecond)
	}
	return s.coverID == want
}
