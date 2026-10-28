package agentdesk

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
)

var ErrModelSetupDenied = errors.New("model setup denied by policy")
var ErrModelSetupBusy = errors.New("agent is busy; retry model setup when the turn finishes")

// ModelSetup deliberately excludes credential names, paths and values.
type SetupPool struct {
	DiscoverModels       bool   `json:"discover_models,omitempty"`
	BaseURL              string `json:"base_url"`
	CredentialConfigured bool   `json:"credential_configured,omitempty"`
}
type SetupProfile struct {
	Pool  string `json:"pool"`
	Model string `json:"model"`
}
type ModelSetup struct {
	DefaultProfile string                  `json:"default_profile"`
	Pools          map[string]SetupPool    `json:"pools"`
	Profiles       map[string]SetupProfile `json:"profiles"`
}
type ModelSetupState struct {
	ModelSetup
	RestartRequired bool   `json:"restart_required"`
	Reason          string `json:"reason,omitempty"`
}
type ModelSetupControl func(context.Context, *ModelSetup) (ModelSetupState, error)

func (s *Server) modelSetupHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" && r.Method != "POST" {
		w.WriteHeader(405)
		return
	}
	if s.cfg.ModelSetup == nil {
		fail(w, errors.New("This backend does not expose native model setup."), 501)
		return
	}
	var q *ModelSetup
	if r.Method == "POST" {
		q = &ModelSetup{}
		d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 65536))
		d.DisallowUnknownFields()
		if d.Decode(q) != nil || d.Decode(new(any)) != io.EOF {
			http.Error(w, "invalid model setup", 400)
			return
		}
	}
	state, e := s.cfg.ModelSetup(r.Context(), q)
	if e != nil {
		code := http.StatusBadRequest
		if errors.Is(e, ErrModelSetupDenied) {
			code = http.StatusForbidden
		}
		if errors.Is(e, ErrModelSetupBusy) {
			code = http.StatusConflict
		}
		fail(w, e, code)
		return
	}
	respond(w, state)
}
