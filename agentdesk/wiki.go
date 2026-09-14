package agentdesk

import (
	"fmt"
	chiefui "github.com/srhopkins/iugum/plugs/chief"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
)

func wikiProxy(raw string) (*httputil.ReverseProxy, error) {
	u, e := url.Parse(raw)
	if e != nil || u.Host == "" {
		return nil, fmt.Errorf("invalid wiki upstream")
	}
	p := httputil.NewSingleHostReverseProxy(u)
	p.ErrorHandler = func(w http.ResponseWriter, r *http.Request, e error) {
		fail(w, fmt.Errorf("wiki is starting or unavailable: %w", e), http.StatusBadGateway)
	}
	return p, nil
}

// serveWikiExtension keeps the wiki as the top-level application. Chief's API
// occupies a reserved namespace; SilverBullet's file and WebSocket routes pass through.
func (s *Server) serveWikiExtension(w http.ResponseWriter, r *http.Request) bool {
	if r.URL.Path == "/.proxy/iugum/agents" && r.Method == "GET" && !s.cfg.DisableChat {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, agentManagerPage)
		return true
	}
	if (r.URL.Path == "/.proxy/iugum/assets/ui-controls.js" || r.URL.Path == "/.proxy/iugum/assets/ui-styles.js" || r.URL.Path == "/.proxy/iugum/assets/agent-chat.js" || r.URL.Path == "/.proxy/iugum/assets/chief.js") && r.Method == http.MethodGet {
		asset := "chief.js"
		if r.URL.Path == "/.proxy/iugum/assets/ui-controls.js" {
			asset = "ui-controls.js"
		}
		if r.URL.Path == "/.proxy/iugum/assets/ui-styles.js" {
			asset = "ui-styles.js"
		}
		b, e := chiefui.Files.ReadFile(asset)
		if e != nil {
			fail(w, e, 500)
			return true
		}
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		w.Write(b)
		return true
	}
	if strings.HasPrefix(r.URL.Path, "/.proxy/iugum/") {
		http.NotFound(w, r)
		return true
	}
	if s.wiki != nil && !strings.HasPrefix(r.URL.Path, "/api/") {
		if s.cfg.Check != nil {
			act := "read"
			if r.Method != "GET" && r.Method != "HEAD" {
				act = "write"
			}
			if e := s.cfg.Check(r.Context(), "wiki", act); e != nil {
				fail(w, e, 403)
				return true
			}
		}
		s.wiki.ServeHTTP(w, r)
		return true
	}
	return false
}
