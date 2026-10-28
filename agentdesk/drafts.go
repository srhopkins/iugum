package agentdesk

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"
)

type preparedKey struct{}
type preparedValue struct {
	preparing bool
	value     any
}
type preparedChat struct {
	id, hold, text string
	revision       uint64
	base           [32]byte
	expires        time.Time
	payload        *preparedValue
	cancel         context.CancelFunc
	ready          bool
}

func Preparing(ctx context.Context) bool {
	p, _ := ctx.Value(preparedKey{}).(*preparedValue)
	return p != nil && p.preparing
}
func SetPreparedValue(ctx context.Context, v any) error {
	p, _ := ctx.Value(preparedKey{}).(*preparedValue)
	if p == nil || !p.preparing {
		return errors.New("not a preparation context")
	}
	p.value = v
	return nil
}
func PreparedValue(ctx context.Context) any {
	p, _ := ctx.Value(preparedKey{}).(*preparedValue)
	if p == nil || p.preparing {
		return nil
	}
	return p.value
}

func (s *Server) prepareChat(w http.ResponseWriter, r *http.Request) {
	if !s.cfg.AllowPTTDrafts || s.cfg.Chat == nil {
		http.NotFound(w, r)
		return
	}
	var in struct {
		Operation string `json:"operation"`
		Hold      string `json:"hold_id"`
		Revision  uint64 `json:"revision"`
		Text      string `json:"text"`
	}
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 20<<10))
	d.DisallowUnknownFields()
	if d.Decode(&in) != nil || d.Decode(new(any)) != io.EOF || !conversationName.MatchString(in.Hold) {
		fail(w, errors.New("valid hold identity required"), 400)
		return
	}
	if in.Operation == "cancel" {
		s.draftMu.Lock()
		defer s.draftMu.Unlock()
		if !s.closeHoldLocked(in.Hold) {
			fail(w, errors.New("too many recent holds"), 429)
			return
		}
		if s.draft != nil && s.draft.hold == in.Hold {
			s.draft.cancel()
			s.draft = nil
		}
		respond(w, map[string]bool{"cancelled": true})
		return
	}
	in.Text = strings.TrimSpace(in.Text)
	if in.Operation != "prepare" || in.Revision == 0 || in.Text == "" || len(in.Text) > 16<<10 {
		fail(w, errors.New("bounded text and positive revision required"), 400)
		return
	}
	s.chatMu.Lock()
	snapshot, base, err := s.draftSnapshot(in.Text)
	s.chatMu.Unlock()
	if err != nil {
		fail(w, err, 500)
		return
	}
	var nonce [16]byte
	if _, err = rand.Read(nonce[:]); err != nil {
		fail(w, err, 500)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
	defer cancel()
	slot := &preparedChat{id: hex.EncodeToString(nonce[:]), hold: in.Hold, text: in.Text, revision: in.Revision, base: base, expires: time.Now().Add(30 * time.Second), payload: &preparedValue{preparing: true}, cancel: cancel}
	s.draftMu.Lock()
	s.pruneHoldsLocked()
	if _, closed := s.closedHolds[in.Hold]; closed {
		s.draftMu.Unlock()
		fail(w, errors.New("hold already closed"), 409)
		return
	}
	if s.draft != nil && s.draft.hold == in.Hold && s.draft.revision >= in.Revision {
		s.draftMu.Unlock()
		fail(w, errors.New("stale draft revision"), 409)
		return
	}
	if s.draft != nil {
		s.draft.cancel()
	}
	s.draft = slot
	s.draftMu.Unlock()
	id := s.cfg.ConversationID
	if id == "" {
		id = "main"
	}
	ctx = context.WithValue(context.WithValue(context.WithValue(ctx, conversationKey{}, id), snapshotKey{}, snapshot), preparedKey{}, slot.payload)
	_, err = s.cfg.Chat(ctx, in.Text)
	s.draftMu.Lock()
	defer s.draftMu.Unlock()
	if s.draft != slot {
		fail(w, errors.New("draft superseded"), 409)
		return
	}
	if err != nil || ctx.Err() != nil || slot.payload.value == nil {
		s.draft = nil
		if err == nil {
			err = errors.New("draft cancelled or unsupported")
		}
		fail(w, err, 409)
		return
	}
	slot.ready = true
	slot.expires = time.Now().Add(30 * time.Second)
	// Remove an idle preparation even if no further HTTP request arrives.
	time.AfterFunc(30*time.Second, func() {
		s.draftMu.Lock()
		defer s.draftMu.Unlock()
		if s.draft == slot {
			slot.cancel()
			s.draft = nil
		}
	})
	respond(w, map[string]any{"draft_id": slot.id, "ready": true, "revision": slot.revision})
}

func (s *Server) pruneHoldsLocked() {
	if s.closedHolds == nil {
		s.closedHolds = map[string]time.Time{}
	}
	now := time.Now()
	for k, v := range s.closedHolds {
		if !now.Before(v) {
			delete(s.closedHolds, k)
		}
	}
	if s.draft != nil && !now.Before(s.draft.expires) {
		s.draft.cancel()
		s.draft = nil
	}
}
func (s *Server) closeHoldLocked(id string) bool {
	s.pruneHoldsLocked()
	if _, ok := s.closedHolds[id]; !ok && len(s.closedHolds) >= 64 {
		return false
	}
	s.closedHolds[id] = time.Now().Add(30 * time.Second)
	return true
}

// Called under chatMu, BEFORE the real user message is appended. Draft text or
// model output can never append to the durable conversation on its own.
func (s *Server) takePrepared(hold, id, text string) (*preparedValue, error) {
	if hold == "" && id == "" {
		return nil, nil
	}
	if !s.cfg.AllowPTTDrafts || !conversationName.MatchString(hold) {
		return nil, errors.New("draft release unavailable or invalid")
	}
	s.draftMu.Lock()
	defer s.draftMu.Unlock()
	s.pruneHoldsLocked()
	if _, closed := s.closedHolds[hold]; closed {
		return nil, errors.New("hold already submitted or cancelled")
	}
	if !s.closeHoldLocked(hold) {
		return nil, errors.New("too many recent holds")
	}
	slot := s.draft
	if slot == nil || slot.hold != hold {
		return nil, nil
	}
	s.draft = nil
	slot.cancel()
	if !slot.ready || slot.id != id || slot.text != text {
		return nil, nil
	}
	_, base, err := s.draftSnapshot("")
	if err != nil {
		return nil, err
	}
	if base != slot.base {
		return nil, nil
	}
	return &preparedValue{value: slot.payload.value}, nil
}

func (s *Server) draftSnapshot(text string) (string, [32]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.refreshCommitmentsLocked(); err != nil {
		return "", [32]byte{}, err
	}
	raw, err := json.Marshal(s.data)
	if err != nil {
		return "", [32]byte{}, err
	}
	messages := append([]Message(nil), s.data.Messages...)
	if text != "" {
		messages = append(messages, Message{Role: "user", Text: text, At: time.Now()})
	}
	return renderContext(s.data.Commitments, messages, time.Now()), sha256.Sum256(raw), nil
}
