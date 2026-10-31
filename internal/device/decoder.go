package device

// Decoder fills one model family's state from decrypted FF09 payloads.
// The link owns it; only the link's goroutine calls it.
type Decoder interface {
	// Handles reports whether Apply decodes command.
	Handles(command uint16) bool
	// Apply decodes one frame and reports whether a visible field changed.
	Apply(command uint16, plain []byte) (changed bool)
	// Snapshot returns a deep copy of the state, safe to hand to the UI:
	// a Charger or a PowerBank today.
	Snapshot() any
	Reset()
}

// decoders are the decoder of each kind. A new device family adds its
// decoder here; spec/fixtures name their kind in "device".
var decoders = map[Kind]func() Decoder{
	KindCharger:   func() Decoder { return chargerDecoder{NewChargerDecoder()} },
	KindPowerBank: func() Decoder { return powerBankDecoder{NewPowerBankDecoder()} },
}

// NewDecoder returns a fresh decoder for kind.
func NewDecoder(kind Kind) (Decoder, bool) {
	f, ok := decoders[kind]
	if !ok {
		return nil, false
	}
	return f(), true
}

type chargerDecoder struct{ *ChargerDecoder }

func (d chargerDecoder) Snapshot() any { return d.ChargerDecoder.Snapshot() }

type powerBankDecoder struct{ *PowerBankDecoder }

func (d powerBankDecoder) Snapshot() any { return d.PowerBankDecoder.Snapshot() }
