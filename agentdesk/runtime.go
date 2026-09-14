package agentdesk

import (
	"crypto/subtle"
	"net/http"
)

// Runtime control is local and requires a random per-process capability. It is
// deliberately not controlled by the agent's permission-changing tools.
func (s *Server) runtimeControl(w http.ResponseWriter, r *http.Request) bool {
	if r.URL.Path != "/api/runtime/stop" {
		return false
	}
	if r.Method != "POST" || s.cfg.Stop == nil || s.cfg.ControlToken == "" {
		http.NotFound(w, r)
		return true
	}
	if subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+s.cfg.ControlToken)) != 1 {
		http.Error(w, "runtime control denied", 403)
		return true
	}
	respond(w, map[string]bool{"stopping": true})
	go s.cfg.Stop()
	return true
}
