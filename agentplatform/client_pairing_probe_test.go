package agentplatform

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"sync"
	"testing"
	"time"
)

// Disposable protocol experiment, deliberately disconnected from live routes.
// Mock event sources cannot establish real code-server iframe/CSP feasibility.
func TestClientPairingNonceOnlyProbe(t *testing.T) {
	type binding struct {
		Instance  string `json:"instance"`
		Client    string `json:"client"`
		Document  string `json:"document"`
		Challenge string `json:"challenge,omitempty"`
	}
	type pending struct {
		value   binding
		expires time.Time
	}
	var mu sync.Mutex
	challenges := map[string]pending{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fail := func() { w.WriteHeader(http.StatusForbidden); _, _ = w.Write([]byte(`{"denied":true}`)) }
		if r.Method != http.MethodPost {
			fail()
			return
		}
		var value binding
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2048))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&value) != nil || !uuid.MatchString(value.Instance) || !uuid.MatchString(value.Client) || !uuid.MatchString(value.Document) {
			fail()
			return
		}
		mu.Lock()
		defer mu.Unlock()
		switch r.URL.Path {
		case "/challenge":
			if value.Challenge != "" || len(challenges) >= 8 {
				fail()
				return
			}
			var random [32]byte
			if _, err := rand.Read(random[:]); err != nil {
				t.Error(err)
				fail()
				return
			}
			value.Challenge = hex.EncodeToString(random[:])
			challenges[value.Challenge] = pending{value, time.Now().Add(time.Minute)}
			_ = json.NewEncoder(w).Encode(value)
		case "/attest":
			p, exists := challenges[value.Challenge]
			if !exists || p.value != value || !time.Now().Before(p.expires) {
				fail()
				return
			}
			delete(challenges, value.Challenge)
			_, _ = w.Write([]byte(`{"paired":true}`))
		default:
			fail()
		}
	}))
	defer server.Close()
	command := exec.Command("node", "testdata/client-pairing-probe.cjs")
	command.Env = append(os.Environ(), "PAIR_PROBE_URL="+server.URL)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("%s: %v", output, err)
	} else {
		t.Log(string(output))
	}
}
