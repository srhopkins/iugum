package agentplatform

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/srhopkins/iugum/contract"
)

type CatalogEntry struct {
	contract.AgentSessionRef
	Key       string `json:"id"`
	Summary   string `json:"summary,omitempty"`
	UpdatedAt string `json:"updatedAt,omitempty"`
	Resume    bool   `json:"resume"`
	Disabled  bool   `json:"disabled,omitempty"`
	Worker    bool   `json:"worker,omitempty"`
}

func entry(ref contract.AgentSessionRef) CatalogEntry {
	return CatalogEntry{AgentSessionRef: ref, Key: strings.ToLower(ref.Provider) + ":" + ref.ID, Resume: true}
}
func transcriptFiles(root string) []string {
	out := []string{}
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if len(out) >= 500 {
			return fs.SkipAll
		}
		if d.IsDir() && d.Name() == "subagents" {
			return fs.SkipDir
		}
		if !d.IsDir() && d.Type().IsRegular() && strings.HasSuffix(path, ".jsonl") {
			out = append(out, path)
		}
		return nil
	})
	return out
}
func stringField(v map[string]any, key string) string { value, _ := v[key].(string); return value }
func clip(text string) string {
	text = strings.Join(strings.Fields(text), " ")
	if len(text) > 240 {
		return text[:240]
	}
	return text
}
func textContent(v any) string {
	switch value := v.(type) {
	case string:
		return value
	case []any:
		parts := []string{}
		for _, p := range value {
			if item, ok := p.(map[string]any); ok {
				parts = append(parts, stringField(item, "text"))
			}
		}
		return strings.Join(parts, " ")
	}
	return ""
}
func claudeConfigCatalog(configDir string) []CatalogEntry {
	out := []CatalogEntry{}
	for _, file := range transcriptFiles(filepath.Join(configDir, "projects")) {
		id := strings.TrimSuffix(filepath.Base(file), ".jsonl")
		if !uuid.MatchString(id) {
			continue
		}
		info, err := os.Stat(file)
		if err != nil || info.Size() > 32*1024*1024 {
			continue
		}
		f, err := os.Open(file)
		if err != nil {
			continue
		}
		item := entry(contract.AgentSessionRef{Provider: "Claude", ID: id})
		item.UpdatedAt = info.ModTime().UTC().Format(time.RFC3339)
		scanner := bufio.NewScanner(f)
		scanner.Buffer(make([]byte, 65536), 8*1024*1024)
		human := false
		for scanner.Scan() {
			var v map[string]any
			if json.Unmarshal(scanner.Bytes(), &v) != nil {
				continue
			}
			if cwd := stringField(v, "cwd"); item.Cwd == "" && filepath.IsAbs(cwd) {
				item.Cwd = cwd
			}
			if v["type"] == "custom-title" && stringField(v, "customTitle") != "" {
				item.Title = clip(stringField(v, "customTitle"))
			}
			if stamp := stringField(v, "timestamp"); stamp != "" {
				if _, err := time.Parse(time.RFC3339, stamp); err == nil {
					item.UpdatedAt = stamp
				}
			}
			if message, ok := v["message"].(map[string]any); ok && v["type"] == "user" && item.Summary == "" {
				item.Summary = clip(textContent(message["content"]))
			}
			if v["isSidechain"] == true || v["entrypoint"] == "sdk-cli" || v["promptSource"] == "sdk" {
				item.Worker = true
			}
			if v["entrypoint"] == "cli" || v["entrypoint"] == "claude-desktop" {
				human = true
			}
		}
		f.Close()
		if scanner.Err() != nil || item.Cwd == "" {
			continue
		}
		if human {
			item.Worker = false
		}
		if item.Title == "" {
			item.Title = item.Summary
		}
		if item.Title == "" {
			item.Title = "Claude session"
		}
		out = append(out, item)
	}
	return out
}
func (s *Server) codexCatalog(ctx context.Context) ([]CatalogEntry, error) {
	p, ok := s.providers["Codex"].(Provider)
	if !ok {
		return nil, nil
	}
	r, err := startRPC(ctx, p.Binary, []string{"app-server", "--listen", "stdio://"}, s.cfg.Home, nil, nil)
	if err != nil {
		return nil, err
	}
	defer r.close()
	if err = r.call(ctx, "initialize", map[string]any{"clientInfo": map[string]string{"name": "iugum_catalog", "version": "0.1.0"}}, nil); err != nil {
		return nil, err
	}
	if err = r.notification("initialized", map[string]any{}); err != nil {
		return nil, err
	}
	var result struct {
		Data []struct {
			ID      string `json:"id"`
			Name    string `json:"name"`
			Preview string `json:"preview"`
			Cwd     string `json:"cwd"`
			Updated int64  `json:"updatedAt"`
		} `json:"data"`
	}
	if err = r.call(ctx, "thread/list", map[string]any{"limit": 200, "sortKey": "updated_at", "sortDirection": "desc"}, &result); err != nil {
		return nil, err
	}
	out := []CatalogEntry{}
	for _, v := range result.Data {
		ref := contract.AgentSessionRef{Provider: "Codex", ID: v.ID, Cwd: v.Cwd, Title: clip(v.Name)}
		if validRef(ref) != nil {
			continue
		}
		item := entry(ref)
		item.Summary = clip(v.Preview)
		if item.Title == "" {
			item.Title = item.Summary
		}
		if item.Title == "" {
			item.Title = "Codex session"
		}
		item.UpdatedAt = time.Unix(v.Updated, 0).UTC().Format(time.RFC3339)
		out = append(out, item)
	}
	return out, nil
}
func (s *Server) catalog(ctx context.Context) any {
	ctx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	out := []CatalogEntry{}
	if p, ok := s.providers["Claude"].(ClaudeProvider); ok && filepath.IsAbs(p.ConfigDir) {
		out = append(out, claudeConfigCatalog(p.ConfigDir)...)
	}
	warnings := []string{}
	if p, ok := s.providers["Iugum"].(NativeHTTPProvider); ok {
		rows, err := p.Catalog(ctx)
		if err != nil {
			warnings = append(warnings, "Native discovery unavailable")
		} else {
			out = append(out, rows...)
		}
	}
	for _, fn := range []struct {
		name string
		load func(context.Context) ([]CatalogEntry, error)
	}{{"Codex", s.codexCatalog}, {"OpenCode", func(ctx context.Context) ([]CatalogEntry, error) {
		p, ok := s.providers["OpenCode"].(OpenCodeHTTPProvider)
		if !ok {
			return nil, nil
		}
		return configuredOpenCodeCatalog(ctx, p)
	}}} {
		rows, err := fn.load(ctx)
		out = append(out, rows...)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("%s discovery unavailable: %s", fn.name, err))
		}
	}
	files, _ := filepath.Glob(filepath.Join(s.cfg.DataDir, "created-*.json"))
	for _, file := range files {
		b, err := os.ReadFile(file)
		if err != nil {
			continue
		}
		var ref contract.AgentSessionRef
		if json.Unmarshal(b, &ref) == nil && validRef(ref) == nil && s.providers[ref.Provider] != nil {
			out = append(out, entry(ref))
		}
	}
	seen := map[string]bool{}
	unique := []CatalogEntry{}
	for _, v := range out {
		if !seen[v.Key] {
			unique = append(unique, v)
			seen[v.Key] = true
		}
	}
	out = unique
	s.mu.Lock()
	for _, m := range s.sessions {
		item := entry(m.ref)
		if !seen[item.Key] {
			out = append(out, item)
		}
	}
	s.mu.Unlock()
	sort.SliceStable(out, func(i, j int) bool { return out[i].UpdatedAt > out[j].UpdatedAt })
	return map[string]any{"sessions": out, "warnings": warnings}
}

// Discovery uses the same server and credentials as session execution. The
// adapter's HTTP helper strips redirects and returns sanitized errors.
func configuredOpenCodeCatalog(ctx context.Context, p OpenCodeHTTPProvider) ([]CatalogEntry, error) {
	c := http.Client{}
	if p.Client != nil {
		c = *p.Client
	}
	c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	session := openCodeHTTPSession{p: p, client: &c}
	var rows []struct {
		ID, Title, Directory string
		Time                 struct{ Updated int64 }
	}
	if err := session.call(ctx, "GET", "/session", nil, &rows); err != nil {
		return nil, err
	}
	out := []CatalogEntry{}
	for _, r := range rows {
		ref := contract.AgentSessionRef{Provider: "OpenCode", ID: r.ID, Cwd: r.Directory, Title: clip(r.Title)}
		if validRef(ref) != nil {
			continue
		}
		item := entry(ref)
		item.UpdatedAt = time.UnixMilli(r.Time.Updated).UTC().Format(time.RFC3339)
		out = append(out, item)
	}
	return out, nil
}
