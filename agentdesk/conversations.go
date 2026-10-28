package agentdesk

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
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
	if id := r.URL.Query().Get("transcript"); id != "" {
		if id != "main" && !conversationName.MatchString(id) {
			fail(w, errors.New("invalid conversation"), 400)
			return
		}
		dir := s.cfg.DataDir
		if id != "main" {
			dir = filepath.Join(dir, "conversations", id)
		}
		messages, err := readConversationContext(dir)
		if err != nil {
			fail(w, err, 500)
			return
		}
		respond(w, messages)
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
		dir := s.cfg.DataDir
		if id != "main" {
			dir = filepath.Join(dir, "conversations", id)
		}
		contextMessages, contextErr := readConversationContext(dir)
		if contextErr != nil {
			fail(w, errors.New("saved conversation context is unreadable"), 500)
			return
		}
		child, err := s.conversationServer(id)
		if err != nil {
			if len(contextMessages) == 0 {
				continue
			}
			child = &Server{}
		}
		child.mu.Lock()
		title := child.data.ConversationTitle
		if title == "" && len(child.data.Messages) > 0 {
			title = child.data.Messages[0].Text
		}
		query := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
		snippet := ""
		matched := query == "" || strings.Contains(strings.ToLower(title), query)
		for _, message := range child.data.Messages {
			if query != "" && strings.Contains(strings.ToLower(message.Text), query) {
				matched = true
				snippet = conversationSnippet(message.Text, query)
				break
			}
		}
		contextOnly := err != nil
		if !matched {
			for _, message := range contextMessages {
				if strings.Contains(strings.ToLower(message.Content), query) {
					matched = true
					contextOnly = true
					snippet = conversationSnippet(message.Content, query)
					break
				}
			}
		}
		if title == "" && len(contextMessages) > 0 {
			title = conversationSnippet(contextMessages[0].Content, "")
		}
		child.mu.Unlock()
		if matched && contextOnly {
			entries = append(entries, map[string]string{"id": id, "title": title, "snippet": snippet, "readOnly": "context"})
			continue
		}
		if matched {
			entries = append(entries, map[string]string{"id": id, "title": title, "snippet": snippet})
		}
	}
	respond(w, entries)
}

// conversationSnippet keeps the matching content visible without returning the transcript.
func conversationSnippet(text, query string) string {
	runes := []rune(strings.Join(strings.Fields(text), " "))
	lower := []rune(strings.ToLower(string(runes)))
	pos := strings.Index(string(lower), query)
	start := 0
	if pos > 0 {
		start = len([]rune(string(lower)[:pos])) - 60
		if start < 0 {
			start = 0
		}
	}
	end := start + 240
	if end > len(runes) {
		end = len(runes)
	}
	result := string(runes[start:end])
	if start > 0 {
		result = "…" + result
	}
	if end < len(runes) {
		result += "…"
	}
	return result
}

type conversationContextMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

func readConversationContext(dir string) ([]conversationContextMessage, error) {
	f, err := os.Open(filepath.Join(dir, "context.json"))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var messages []conversationContextMessage
	if err = json.NewDecoder(io.LimitReader(f, 32<<20)).Decode(&messages); err != nil {
		return nil, err
	}
	out := []conversationContextMessage{}
	for _, m := range messages {
		if m.Role == "user" || m.Role == "assistant" {
			out = append(out, m)
		}
	}
	return out, nil
}
