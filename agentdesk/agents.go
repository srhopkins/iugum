package agentdesk

import (
	"net/http"
	"net/http/httputil"
	"net/url"
	"sort"
	"strings"
)

type AgentOption struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Model     string `json:"model,omitempty"`
	Transport string `json:"transport,omitempty"`
}

func (s *Server) serveAgentRoute(w http.ResponseWriter, r *http.Request) bool {
	if r.URL.Path == "/api/agents" && r.Method == "GET" {
		options := []AgentOption{{"default", s.cfg.Name, s.cfg.Metadata["model"], s.cfg.Metadata["transport"]}}
		if s.cfg.HideDefaultAgent {
			options = []AgentOption{}
		}
		ids := []string{}
		for id := range s.cfg.Agents {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		for _, id := range ids {
			a := s.cfg.Agents[id]
			if a.cfg.Check != nil && a.cfg.Check(r.Context(), "agent/select", "read") != nil {
				continue
			}
			options = append(options, AgentOption{id, a.cfg.Name, a.cfg.Metadata["model"], a.cfg.Metadata["transport"]})
		}
		for id := range s.cfg.Connections {
			if s.cfg.Check != nil && s.cfg.Check(r.Context(), "agent:"+id, "attach") != nil {
				continue
			}
			options = append(options, AgentOption{ID: id, Name: id, Transport: "iugum"})
		}
		respond(w, options)
		return true
	}
	if !strings.HasPrefix(r.URL.Path, "/api/agents/") {
		return false
	}
	parts := strings.SplitN(strings.TrimPrefix(r.URL.Path, "/api/agents/"), "/", 2)
	if len(parts) != 2 {
		http.NotFound(w, r)
		return true
	}
	if resolve := s.cfg.Connections[parts[0]]; resolve != nil {
		if (parts[1] == "start" || parts[1] == "stop") && r.Method == "POST" && s.cfg.ConnectionControl != nil {
			if s.cfg.Check != nil {
				if e := s.cfg.Check(r.Context(), "agent:"+parts[0], parts[1]); e != nil {
					fail(w, e, 403)
					return true
				}
			}
			if e := s.cfg.ConnectionControl(r.Context(), parts[0], parts[1]); e != nil {
				fail(w, e, 500)
			} else {
				respond(w, map[string]bool{"ok": true})
			}
			return true
		}
		switch parts[1] {
		case "status", "messages", "chat", "approvals", "conversation", "conversations":
		default:
			http.NotFound(w, r)
			return true
		}
		if s.cfg.Check != nil {
			if err := s.cfg.Check(r.Context(), "agent:"+parts[0], "attach"); err != nil {
				fail(w, err, 403)
				return true
			}
		}
		raw, err := resolve(r.Context())
		if err != nil {
			fail(w, err, 503)
			return true
		}
		u, err := url.Parse(raw)
		if err != nil {
			fail(w, err, 503)
			return true
		}
		proxy := httputil.NewSingleHostReverseProxy(u)
		director := proxy.Director
		proxy.Director = func(req *http.Request) {
			director(req)
			req.URL.Path = "/api/" + parts[1]
			req.Host = u.Host
			req.Header.Del("Origin")
		}
		proxy.ServeHTTP(w, r)
		return true
	}
	a := s.cfg.Agents[parts[0]]
	if a == nil {
		http.NotFound(w, r)
		return true
	}
	if a.cfg.Check != nil {
		if err := a.cfg.Check(r.Context(), "agent/select", "read"); err != nil {
			fail(w, err, 403)
			return true
		}
	}
	// Only conversation surfaces are selectable. Wiki/search remain shared and
	// independently authorized; no arbitrary child route or path forwarding.
	switch parts[1] {
	case "status", "messages", "chat", "approvals", "conversation", "conversations":
	default:
		http.NotFound(w, r)
		return true
	}
	clone := r.Clone(r.Context())
	urlCopy := *r.URL
	urlCopy.Path = "/api/" + parts[1]
	clone.URL = &urlCopy
	a.serve(w, clone)
	return true
}
