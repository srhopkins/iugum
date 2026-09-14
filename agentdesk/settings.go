package agentdesk

import (
	"context"
	"encoding/json"
	"net/http"
)

type SettingField struct {
	ID       string `json:"id"`
	Label    string `json:"label"`
	Value    string `json:"value"`
	Editable bool   `json:"editable"`
}
type SettingsState struct {
	Fields []SettingField `json:"fields"`
	Reason string         `json:"reason,omitempty"`
}
type SettingsControl func(context.Context, string, *string) (SettingsState, error)

func (s *Server) settingsHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" && r.Method != "POST" {
		w.WriteHeader(405)
		return
	}
	if s.cfg.Settings == nil {
		if r.Method != http.MethodGet {
			http.Error(w, "settings editing unavailable", http.StatusNotImplemented)
			return
		}
		respond(w, SettingsState{Fields: []SettingField{}, Reason: "This runtime does not expose editable settings."})
		return
	}
	var q struct {
		ID    string  `json:"id"`
		Value *string `json:"value"`
	}
	if r.Method == "POST" {
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 131072)).Decode(&q); err != nil || q.ID == "" || q.Value == nil {
			http.Error(w, "setting id and value required", 400)
			return
		}
	}
	state, err := s.cfg.Settings(r.Context(), q.ID, q.Value)
	if err != nil {
		fail(w, err, 400)
		return
	}
	respond(w, state)
}
