package agentdesk

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
)

type conversationKey struct{}
type snapshotKey struct{}

func ConversationID(ctx context.Context) string {
	if id, ok := ctx.Value(conversationKey{}).(string); ok {
		return id
	}
	return "main"
}
func ConversationSnapshot(ctx context.Context) string {
	v, _ := ctx.Value(snapshotKey{}).(string)
	return v
}

var conversationName = regexp.MustCompile(`^[a-f0-9]{32}$`)

func (s *Server) conversationServer(id string) (*Server, error) {
	if id == "main" || id == "" {
		return s, nil
	}
	if !conversationName.MatchString(id) {
		return nil, errors.New("invalid conversation")
	}
	s.conversationMu.Lock()
	defer s.conversationMu.Unlock()
	if child := s.conversations[id]; child != nil {
		return child, nil
	}
	dir := filepath.Join(s.cfg.DataDir, "conversations", id)
	if _, err := os.Stat(filepath.Join(dir, "workspace.json")); err != nil {
		return nil, err
	}
	cfg := s.cfg
	cfg.DataDir = dir
	cfg.ConversationID = id
	cfg.WikiURL = ""
	cfg.WikiIntegrated = false
	cfg.HideDefaultAgent = false
	cfg.Agents = nil
	cfg.Connections = nil
	child, err := New(cfg)
	if err != nil {
		return nil, err
	}
	s.conversations[id] = child
	return child, nil
}
func (s *Server) conversationsHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == "POST" {
		if !s.cfg.SeparateConversations {
			fail(w, errors.New("runtime does not support separate conversations"), 409)
			return
		}
		var bytes [16]byte
		if _, err := rand.Read(bytes[:]); err != nil {
			fail(w, err, 500)
			return
		}
		id := hex.EncodeToString(bytes[:])
		dir := filepath.Join(s.cfg.DataDir, "conversations", id)
		if err := os.MkdirAll(dir, 0700); err != nil {
			fail(w, err, 500)
			return
		}
		if err := os.WriteFile(filepath.Join(dir, "workspace.json"), []byte(`{"messages":[]}`), 0600); err != nil {
			fail(w, err, 500)
			return
		}
		respond(w, map[string]string{"id": id})
		return
	}
	if r.Method != "GET" {
		w.WriteHeader(405)
		return
	}
	entries := []map[string]string{}
	ids := []string{"main"}
	dirs, _ := os.ReadDir(filepath.Join(s.cfg.DataDir, "conversations"))
	for _, d := range dirs {
		if d.IsDir() && conversationName.MatchString(d.Name()) {
			ids = append(ids, d.Name())
		}
	}
	sort.Strings(ids)
	for _, id := range ids {
		child, err := s.conversationServer(id)
		if err != nil {
			continue
		}
		child.mu.Lock()
		title := child.data.ConversationTitle
		if title == "" && len(child.data.Messages) > 0 {
			title = child.data.Messages[0].Text
		}
		child.mu.Unlock()
		entries = append(entries, map[string]string{"id": id, "title": title})
	}
	respond(w, entries)
}
