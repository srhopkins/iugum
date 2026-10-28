package browsergrant

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

type healthRecord struct {
	Version     int    `json:"version"`
	Clean       bool   `json:"clean"`
	StoreDigest string `json:"storeDigest"`
}
type storeHealth struct {
	store, path string
	healthy     bool
}

// A durable dirty marker exists BEFORE any command is accepted. If the process
// crashes or any write has unknown outcome, automatic restart is quarantined:
// an UUID that never reached disk cannot be reconstructed from surviving bytes.
func openStoreHealth(store string) (*storeHealth, error) {
	h := &storeHealth{store: store, path: store + ".health", healthy: true}
	b, e := os.ReadFile(h.path)
	if e == nil {
		m, parseError := Decode(bytes.NewReader(b))
		clean, cleanOK := m["clean"].(bool)
		digest, digestOK := m["storeDigest"].(string)
		if parseError != nil || !exactFields(m, []string{"version", "clean", "storeDigest"}) || m["version"] != 1 || !cleanOK || !clean || !digestOK || !digestID.MatchString(digest) {
			return nil, errors.New("browser receipt store quarantined: unclean or invalid health marker; explicit audited recovery required")
		}
		data, e := os.ReadFile(store)
		if e != nil {
			return nil, e
		}
		storeSum := sha256.Sum256(data)
		if m["storeDigest"] != hex.EncodeToString(storeSum[:]) {
			return nil, errors.New("browser receipt store quarantined: clean digest mismatch")
		}
	} else if !os.IsNotExist(e) {
		return nil, e
	} else if _, e = os.Stat(store); e == nil {
		return nil, errors.New("browser receipt store quarantined: existing store has no recovery marker")
	} else if !os.IsNotExist(e) {
		return nil, e
	}
	if e = h.mark(false, ""); e != nil {
		return nil, e
	}
	return h, nil
}
func (h *storeHealth) mark(clean bool, digest string) error {
	b, e := json.Marshal(healthRecord{1, clean, digest})
	if e != nil {
		return e
	}
	return atomicHealth(h.path, b)
}
func atomicHealth(path string, b []byte) error {
	f, e := os.CreateTemp(filepath.Dir(path), ".browser-health-")
	if e != nil {
		return e
	}
	defer os.Remove(f.Name())
	if e = f.Chmod(0600); e == nil {
		_, e = f.Write(b)
	}
	if e == nil {
		e = f.Sync()
	}
	ce := f.Close()
	if e == nil {
		e = ce
	}
	if e != nil {
		return e
	}
	if e = os.Rename(f.Name(), path); e != nil {
		return e
	}
	dir, e := os.Open(filepath.Dir(path))
	if e != nil {
		return e
	}
	defer dir.Close()
	return dir.Sync()
}
func (h *storeHealth) close() error {
	if !h.healthy {
		return errors.New("browser receipt store remains quarantined after uncertain write")
	}
	b, e := os.ReadFile(h.store)
	if e != nil {
		return e
	}
	sum := sha256.Sum256(b)
	return h.mark(true, hex.EncodeToString(sum[:]))
}
func (a *Authority) persist() error {
	e := a.save(a.receipts)
	if e != nil {
		a.health.healthy = false
	}
	return e
}
