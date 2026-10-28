package agentplatform

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/srhopkins/iugum/agentsessions"
	"github.com/srhopkins/iugum/contract"
)

// sessionSearch is read-only: neither indexing nor retrieval opens provider owners.
func (s *Server) sessionSearch(r *http.Request) (any, error) {
	s.searchMu.Lock()
	defer s.searchMu.Unlock()
	index, err := agentsessions.Open(filepath.Join(s.cfg.DataDir, "transcript-search.sqlite"))
	if err != nil {
		return nil, err
	}
	defer index.Close()
	sources := agentsessions.DefaultSources(s.cfg.Home)
	if p, ok := s.providers["Claude"].(ClaudeProvider); ok && filepath.IsAbs(p.ConfigDir) {
		for n := range sources {
			if sources[n].Platform == "claude" {
				sources[n].Root = filepath.Join(p.ConfigDir, "projects")
				sources[n].Account = p.Account
			}
		}
	}
	warnings := []string{}
	allowed := sources[:0]
	for _, source := range sources {
		if s.cfg.Check != nil && s.cfg.Check(r.Context(), "agent/sessions/transcript:"+source.Platform+":"+source.Account, "search") != nil {
			warnings = append(warnings, source.Platform+" transcript source denied by policy")
			continue
		}
		allowed = append(allowed, source)
	}
	sources = allowed
	available := map[string]bool{}
	for _, source := range sources {
		if _, err := os.Stat(source.Root); err == nil {
			available[source.Platform] = true
		}
	}
	for _, provider := range []string{"codex", "claude", "opencode", "cursor"} {
		if !available[provider] {
			warnings = append(warnings, provider+" local transcripts unavailable")
		}
	}
	report, err := index.Sync(r.Context(), sources, agentsessions.Limits{})
	if err != nil {
		return nil, err
	}
	warnings = append(warnings, report.Warnings...)
	if report.Truncated {
		warnings = append(warnings, "Transcript indexing reached a configured limit; results are incomplete")
	}
	authorize := func(h agentsessions.Hit) error {
		if s.cfg.Check == nil {
			return nil
		}
		for _, action := range []string{"search", "read"} {
			if err := s.cfg.Check(r.Context(), "agent/sessions/transcript:"+h.Platform+":"+h.Account, action); err != nil {
				return err
			}
		}
		if err := s.cfg.Check(r.Context(), "agent/sessions/"+sessionProviderName(h.Platform)+"/"+h.Session, "read"); err != nil {
			return err
		}
		return s.cfg.Check(r.Context(), "agent/sessions/project:"+h.Project, "read")
	}
	if r.URL.Path == "/api/session-transcript" {
		key := r.URL.Query().Get("id")
		if len(key) != 64 {
			return nil, errors.New("invalid transcript identity")
		}
		messages, truncated, err := index.Transcript(r.Context(), key, authorize)
		if err != nil {
			return nil, err
		}
		if len(messages) == 0 {
			if p, ok := s.providers["OpenCode"].(OpenCodeHTTPProvider); ok {
				ctx, cancel := context.WithTimeout(r.Context(), 12*time.Second)
				defer cancel()
				entries, err := configuredOpenCodeCatalog(ctx, p)
				if err != nil {
					return nil, err
				}
				for _, entry := range entries {
					if remoteTranscriptID(entry.AgentSessionRef) == key && s.remoteReadable(ctx, entry.AgentSessionRef) {
						history, err := s.remoteHistory(ctx, p, entry.AgentSessionRef)
						if err != nil {
							return nil, err
						}
						return map[string]any{"nativeMessages": history, "title": entry.Title, "readOnly": true}, nil
					}
				}
			}
			return nil, errors.New("transcript unavailable or denied")
		}
		return map[string]any{"messages": messages, "truncated": truncated, "readOnly": true}, nil
	}
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if len(query) > 512 {
		return nil, errors.New("search query too long")
	}
	results, err := index.Sessions(r.Context(), "", authorize)
	if err != nil {
		return nil, err
	}
	matches, err := index.Sessions(r.Context(), query, authorize)
	if err != nil {
		return nil, err
	}
	matched := map[string]string{}
	for _, hit := range matches {
		matched[hit.ID] = hit.Snippet
	}
	counts := map[string]int{}
	for _, hit := range results {
		counts[hit.Provider+":"+hit.SessionID]++
	}
	catalogue := s.searchCatalogue(r.Context())
	if notices, ok := catalogue["warnings"].([]string); ok {
		warnings = append(warnings, notices...)
	}
	titles := map[string]string{}
	for _, entry := range catalogue["sessions"].([]CatalogEntry) {
		if s.cfg.Check != nil && (s.cfg.Check(r.Context(), "agent/sessions/project:"+entry.Cwd, "read") != nil || s.cfg.Check(r.Context(), "agent/sessions/"+entry.Provider+"/"+entry.ID, "read") != nil) {
			continue
		}
		titles[strings.ToLower(entry.Provider)+":"+entry.ID] = entry.Title
	}
	filtered := []agentsessions.SessionSummary{}
	for _, hit := range results {
		key := hit.Provider + ":" + hit.SessionID
		if counts[key] == 1 && titles[key] != "" {
			hit.Title = titles[key]
		}
		titleMatch := true
		for _, term := range strings.Fields(strings.ToLower(query)) {
			if !strings.Contains(strings.ToLower(hit.Title), term) {
				titleMatch = false
				break
			}
		}
		snippet, contentMatch := matched[hit.ID]
		if !titleMatch && !contentMatch {
			continue
		}
		if contentMatch {
			hit.Snippet = snippet
		}
		filtered = append(filtered, hit)
	}
	if p, ok := s.providers["OpenCode"].(OpenCodeHTTPProvider); ok {
		ctx, cancel := context.WithTimeout(r.Context(), 12*time.Second)
		defer cancel()
		for _, entry := range catalogue["sessions"].([]CatalogEntry) {
			if entry.Provider != "OpenCode" || !s.remoteReadable(ctx, entry.AgentSessionRef) {
				continue
			}
			history, err := s.remoteHistory(ctx, p, entry.AgentSessionRef)
			if err != nil {
				warnings = append(warnings, "OpenCode HTTP history unavailable for "+entry.ID)
				continue
			}
			text := entry.Title
			snippet := ""
			for _, message := range history {
				text += "\n" + message.Text
				if strings.Contains(strings.ToLower(message.Text), strings.ToLower(query)) {
					snippet = clip(message.Text)
				}
			}
			match := true
			for _, term := range strings.Fields(strings.ToLower(query)) {
				if !strings.Contains(strings.ToLower(text), term) {
					match = false
					break
				}
			}
			if !match {
				continue
			}
			filtered = append(filtered, agentsessions.SessionSummary{ID: remoteTranscriptID(entry.AgentSessionRef), Provider: "OpenCode", SessionID: entry.ID, Title: entry.Title, Snippet: snippet})
		}
	}
	if len(filtered) > 500 {
		filtered = filtered[:500]
		warnings = append(warnings, "Showing the first 500 indexed matches; narrow the search")
	}
	return map[string]any{"sessions": filtered, "warnings": warnings, "readOnly": true}, nil
}

// remoteHistory reads OpenCode's advertised HTTP transcript without attaching an
// execution owner or subscribing to provider events.
func (s *Server) remoteHistory(ctx context.Context, p OpenCodeHTTPProvider, ref contract.AgentSessionRef) ([]contract.AgentSessionMessage, error) {
	client := http.Client{}
	if p.Client != nil {
		client = *p.Client
	}
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	reader := openCodeHTTPSession{p: p, client: &client, ref: ref}
	return reader.History(ctx)
}
func remoteTranscriptID(ref contract.AgentSessionRef) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte("remote\x00"+ref.Provider+"\x00"+ref.ID+"\x00"+ref.Cwd)))
}
func (s *Server) remoteReadable(ctx context.Context, ref contract.AgentSessionRef) bool {
	if s.cfg.Check == nil {
		return true
	}
	for _, action := range []string{"search", "read"} {
		if s.cfg.Check(ctx, "agent/sessions/transcript:opencode:", action) != nil {
			return false
		}
	}
	return s.cfg.Check(ctx, "agent/sessions/"+ref.Provider+"/"+ref.ID, "read") == nil && s.cfg.Check(ctx, "agent/sessions/project:"+ref.Cwd, "read") == nil
}

func (s *Server) searchCatalogue(ctx context.Context) map[string]any {
	ctx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	rows := []CatalogEntry{}
	warnings := []string{}
	allowed := func(platform string) bool {
		account := ""
		if p, ok := s.providers["Claude"].(ClaudeProvider); ok && platform == "claude" {
			account = p.Account
		}
		return s.cfg.Check == nil || s.cfg.Check(ctx, "agent/sessions/transcript:"+platform+":"+account, "search") == nil
	}
	if p, ok := s.providers["Claude"].(ClaudeProvider); ok && filepath.IsAbs(p.ConfigDir) && allowed("claude") {
		rows = append(rows, claudeConfigCatalog(p.ConfigDir)...)
	}
	if allowed("codex") {
		entries, err := s.codexCatalog(ctx)
		if err != nil {
			warnings = append(warnings, "Codex titles unavailable; indexed content remains searchable")
		} else {
			rows = append(rows, entries...)
		}
	}
	if p, ok := s.providers["OpenCode"].(OpenCodeHTTPProvider); ok && allowed("opencode") {
		entries, err := configuredOpenCodeCatalog(ctx, p)
		if err != nil {
			warnings = append(warnings, "OpenCode HTTP catalogue unavailable; indexed content remains searchable")
		} else {
			rows = append(rows, entries...)
		}
	}
	s.mu.Lock()
	for _, owner := range s.sessions {
		if allowed(strings.ToLower(owner.ref.Provider)) {
			rows = append(rows, entry(owner.ref))
		}
	}
	s.mu.Unlock()
	return map[string]any{"sessions": rows, "warnings": warnings}
}

func sessionProviderName(platform string) string {
	switch strings.ToLower(platform) {
	case "codex":
		return "Codex"
	case "claude":
		return "Claude"
	case "cursor":
		return "Cursor"
	case "opencode":
		return "OpenCode"
	}
	return platform
}
