package prview

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strings"
)

// Handler composes with the existing Beads handler; GitHub routes are loopback
// only even if the operator explicitly exposes the ticket viewer on the LAN.
func Handler(next http.Handler, s *Service) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/v1/") {
			next.ServeHTTP(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		fail := func(err error) {
			var e *APIError
			if !errors.As(err, &e) {
				e = &APIError{Status: 502, Code: "github_error", Message: "GitHub request failed"}
			}
			if e.RetryAfter != "" {
				w.Header().Set("Retry-After", e.RetryAfter)
			}
			w.WriteHeader(e.Status)
			json.NewEncoder(w).Encode(map[string]any{"error": e})
		}
		remote, _, _ := net.SplitHostPort(r.RemoteAddr)
		host := r.Host
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		}
		host = strings.Trim(host, "[]")
		ip := net.ParseIP(remote)
		hostIP := net.ParseIP(host)
		if ip == nil || !ip.IsLoopback() || (host != "localhost" && (hostIP == nil || !hostIP.IsLoopback())) {
			fail(&APIError{Status: 403, Code: "local_only", Message: "GitHub data is available only through the loopback viewer address"})
			return
		}
		if origin := r.Header.Get("Origin"); origin != "" {
			u, err := url.Parse(origin)
			if err != nil || u.Host != r.Host || (u.Scheme != "http" && u.Scheme != "https") {
				fail(&APIError{Status: 403, Code: "origin_denied", Message: "Use the viewer's own origin"})
				return
			}
		}
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", "GET")
			fail(&APIError{Status: 405, Code: "read_only", Message: "This API supports GET only"})
			return
		}
		switch r.URL.Path {
		case "/api/v1/health":
			json.NewEncoder(w).Encode(map[string]any{"status": "ok", "github": "credentials_checked_on_request", "read_only": true})
		case "/api/v1/meta":
			json.NewEncoder(w).Encode(map[string]any{"version": 1, "defaults": map[string]any{"state": "open", "draft": "any", "sort": "updated", "order": "desc", "per_page": 30}, "filters": []string{"q", "org", "repo", "author", "state", "draft", "sort", "order", "page", "per_page"}, "capabilities": map[string]any{"q": "plain_text", "multi_select": []string{"org", "repo", "author"}, "within_category": "OR", "between_categories": "AND", "max_or_operators": 5, "max_results": 1000, "max_per_page": 100, "review_checks": false, "discovery": "current_results_and_exact_input", "scope": "GitHub search under the authenticated account; not an exhaustive repository inventory"}})
		case "/api/v1/prs":
			f, err := Parse(r.URL.Query())
			if err != nil {
				fail(err)
				return
			}
			data, err := s.Search(r.Context(), f)
			if err != nil {
				fail(err)
				return
			}
			json.NewEncoder(w).Encode(data)
		default:
			parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/v1/prs/"), "/")
			if strings.HasPrefix(r.URL.Path, "/api/v1/prs/") && len(parts) == 3 {
				data, err := s.Detail(r.Context(), parts[0], parts[1], parts[2])
				if err != nil {
					fail(err)
					return
				}
				json.NewEncoder(w).Encode(data)
				return
			}
			fail(&APIError{Status: 404, Code: "not_found", Message: "Unknown API route"})
		}
	})
}
