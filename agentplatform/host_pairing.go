package agentplatform

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"time"
)

// Pairing proves only the own-webview private extension channel. It creates no
// browser control authority and never advertises mouse or outer-tab binding.
type HostPairChallenge struct {
	HostIdentity
	Client   string `json:"client"`
	Document string `json:"document"`
}
type HostPairBinding struct {
	Instance  string `json:"instance"`
	Client    string `json:"client"`
	Document  string `json:"document"`
	Challenge string `json:"challenge"`
	Expires   int64  `json:"expires"`
}
type hostPairPending struct {
	HostPairBinding
	Lease string
}
type HostPairAttest struct {
	HostIdentity
	Binding HostPairBinding `json:"binding"`
}
type HostPairReceipt struct {
	Instance string `json:"instance"`
	Client   string `json:"client"`
	Document string `json:"document"`
	Expires  int64  `json:"expires"`
	Scope    string `json:"scope"`
	Mouse    bool   `json:"mouse"`
	Paired   bool   `json:"paired"`
}

func (s *Server) pairingWindow(q HostIdentity) bool {
	w, ok := s.host.windows[q.Instance]
	return ok && time.Now().Before(w.Expires) && subtle.ConstantTimeCompare([]byte(q.Lease), []byte(w.Lease)) == 1
}
func (s *Server) hostPairChallenge(ctx context.Context, q HostPairChallenge) (HostPairBinding, error) {
	if s.host == nil {
		return HostPairBinding{}, errors.New("editor host tools disabled")
	}
	h := s.host
	h.mu.Lock()
	defer h.mu.Unlock()
	if err := s.hostCheck(ctx, "pairing_challenge", "create"); err != nil {
		return HostPairBinding{}, err
	}
	if !uuid.MatchString(q.Client) || !uuid.MatchString(q.Document) || !s.pairingWindow(q.HostIdentity) {
		return HostPairBinding{}, errors.New("invalid live editor pairing identity")
	}
	for key, p := range h.pairings {
		if p.Expires <= time.Now().UnixMilli() {
			delete(h.pairings, key)
		}
	}
	if len(h.pairings) >= 32 {
		return HostPairBinding{}, errors.New("pairing challenge limit reached")
	}
	var nonce [32]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return HostPairBinding{}, err
	}
	result := HostPairBinding{q.Instance, q.Client, q.Document, hex.EncodeToString(nonce[:]), time.Now().Add(30 * time.Second).UnixMilli()}
	h.pairings[result.Challenge] = hostPairPending{result, q.Lease}
	return result, nil
}
func (s *Server) hostPairAttest(ctx context.Context, q HostPairAttest) (HostPairReceipt, error) {
	if s.host == nil {
		return HostPairReceipt{}, errors.New("editor host tools disabled")
	}
	h := s.host
	h.mu.Lock()
	defer h.mu.Unlock()
	// Actual operation policy is rechecked at consumption, not only transport.
	if err := s.hostCheck(ctx, "pairing_attest", "write"); err != nil {
		return HostPairReceipt{}, err
	}
	p, ok := h.pairings[q.Binding.Challenge]
	if !ok || p.HostPairBinding != q.Binding || p.Instance != q.Instance || subtle.ConstantTimeCompare([]byte(p.Lease), []byte(q.Lease)) != 1 || p.Expires <= time.Now().UnixMilli() || !s.pairingWindow(q.HostIdentity) {
		return HostPairReceipt{}, errors.New("stale or mismatched pairing challenge")
	}
	delete(h.pairings, p.Challenge) // One consumption; no persistence or retries.
	return HostPairReceipt{p.Instance, p.Client, p.Document, p.Expires, "own-webview-private-channel", false, true}, nil
}
