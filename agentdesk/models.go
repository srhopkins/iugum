package agentdesk

import (
	"context"
	"encoding/json"
	"net/http"
)

// Model choices are supplied by the active runtime, never an arbitrary model ID.
type ModelOption struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}
type ModelState struct {
	Selected string        `json:"selected"`
	Options  []ModelOption `json:"options"`
	Reason   string        `json:"reason,omitempty"`
}
type ModelControl func(context.Context, string) (ModelState, error)

func (s *Server) modelsHTTP(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Models == nil {
		if r.Method != http.MethodGet {
			http.Error(w, "model selection unavailable", http.StatusNotImplemented)
			return
		}
		respond(w, ModelState{Options: []ModelOption{}, Reason: "This runtime does not expose model selection."})
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		w.WriteHeader(405)
		return
	}
	var q struct {
		ID string `json:"id"`
	}
	if r.Method == http.MethodPost {
		if e := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&q); e != nil || q.ID == "" {
			http.Error(w, "model id required", 400)
			return
		}
	}
	state, e := s.cfg.Models(r.Context(), q.ID)
	if e != nil {
		fail(w, e, 400)
		return
	}
	respond(w, state)
}
