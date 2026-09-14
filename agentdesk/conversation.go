package agentdesk

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"unicode/utf8"
)

// Each server namespace currently owns one continuous conversation. A stable ID
// identifies that legacy conversation without pretending the runtime can fork it.
func (s *Server) conversation(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var in struct {
		Title string `json:"title"`
	}
	if r.Method == http.MethodPost {
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&in); err != nil {
			fail(w, err, 400)
			return
		}
		in.Title = strings.TrimSpace(in.Title)
		if utf8.RuneCountInString(in.Title) > 200 {
			fail(w, errors.New("title must be at most 200 characters"), 400)
			return
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if r.Method == http.MethodPost {
		previous := s.data.ConversationTitle
		s.data.ConversationTitle = in.Title
		if err := s.saveLocked(); err != nil {
			s.data.ConversationTitle = previous
			fail(w, err, 500)
			return
		}
	}
	id := s.cfg.ConversationID
	if id == "" {
		id = "main"
	}
	respond(w, map[string]any{"id": id, "title": s.data.ConversationTitle, "can_create": s.cfg.SeparateConversations})
}
