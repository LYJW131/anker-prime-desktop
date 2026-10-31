package ff09

import (
	"bufio"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

const repoRoot = "../.."

func unhex(t testing.TB, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatalf("bad hex %q: %v", s, err)
	}
	return b
}

// capEvent is one line of captures/*.jsonl.
type capEvent struct {
	T     float64 `json:"t"`
	Dir   string  `json:"dir"`
	Cmd   uint16  `json:"cmd"`
	Group uint8   `json:"group"`
	Enc   bool    `json:"enc"`
	Ack   bool    `json:"ack"`
	Plain *string `json:"plain"`
	Raw   string  `json:"raw"`
}

func captureFiles(t testing.TB) []string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(repoRoot, "captures", "*.jsonl"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no captures found: %v", err)
	}
	return files
}

func readCapture(t testing.TB, path string) []capEvent {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var out []capEvent
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		if len(sc.Bytes()) == 0 {
			continue
		}
		var e capEvent
		if err := json.Unmarshal(sc.Bytes(), &e); err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		out = append(out, e)
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

// specJSON is the part of spec/protocol.json the tests read.
type specJSON struct {
	Transport struct {
		ServiceUUID    string `json:"service_uuid"`
		WriteUUID      string `json:"write_characteristic_uuid"`
		NotifyUUID     string `json:"notify_characteristic_uuid"`
		AdvertisedUUID string `json:"advertised_service_uuid"`
		InitialKey     string `json:"initial_key"`
		InitialIV      string `json:"initial_iv"`
		GCMAAD         string `json:"gcm_aad"`
		FlagEncrypted  uint8  `json:"flag_encrypted"`
		FlagAck        uint8  `json:"flag_ack"`
		GroupSession   uint8  `json:"group_session"`
		GroupTelemetry uint8  `json:"group_telemetry"`
		CommandStatus  uint16 `json:"command_status"`
		CommandRealtim uint16 `json:"command_realtime"`
	} `json:"transport"`
	Handshake struct {
		TxHeader        string `json:"tx_header"`
		Cmd0022A3       string `json:"cmd_0022_a3"`
		Cmd0022A5       string `json:"cmd_0022_a5"`
		Cmd020AA2       string `json:"cmd_020a_a2"`
		AccountIDLength int    `json:"account_id_length"`
		PublicKeyLength int    `json:"public_key_length"`
	} `json:"handshake"`
	Cover struct {
		CommandSelect        uint16         `json:"command_select"`
		CommandTransferStart uint16         `json:"command_transfer_start"`
		CommandTransferData  uint16         `json:"command_transfer_data"`
		TypeCustom           uint8          `json:"type_custom"`
		ChunkSize            int            `json:"chunk_size"`
		AckEvery             int            `json:"ack_every"`
		URLKey               string         `json:"url_key"`
		ResultCodes          map[string]int `json:"result_codes"`
	} `json:"cover"`
	TimingMS map[string]int `json:"timing_ms"`
	Vectors  struct {
		AccountID string `json:"account_id"`
		PublicKey string `json:"public_key"`
		Requests  []struct {
			Name        string `json:"name"`
			Group       uint8  `json:"group"`
			Command     uint16 `json:"command"`
			ExpectsResp *bool  `json:"expects_response"`
			Body        string `json:"body"`
		} `json:"requests"`
	} `json:"vectors"`
	Devices struct {
		Charger struct {
			NamePrefix string `json:"name_prefix"`
		} `json:"charger"`
	} `json:"devices"`
}

func loadSpec(t testing.TB) specJSON {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(repoRoot, "spec", "protocol.json"))
	if err != nil {
		t.Fatal(err)
	}
	var s specJSON
	if err := json.Unmarshal(b, &s); err != nil {
		t.Fatal(err)
	}
	return s
}
