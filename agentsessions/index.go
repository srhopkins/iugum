// Package agentsessions provides a rebuildable local transcript index with source provenance.
package agentsessions

import (
	"context"
	"database/sql"
	"encoding/json"
	_ "modernc.org/sqlite"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Source struct {
	Platform string `json:"platform"`
	Root     string `json:"root"`
	Account  string `json:"account,omitempty"`
}
type Limits struct {
	MaxFiles     int   `yaml:"max_files"`
	MaxMessages  int   `yaml:"max_messages"`
	MaxFileBytes int64 `yaml:"max_line_bytes"`
}
type SyncReport struct {
	Files, Messages          int
	TotalMessages, Unchanged int
	Truncated                bool
	Warnings                 []string
}
type Query struct {
	// Authorize is evaluated per result before inclusion. Denied hits do not consume Limit.
	Authorize          func(Hit) error
	AnyTerms           bool
	Text, Scope        string
	Exclude, Platforms []string
	Limit              int
}
type Hit struct {
	Platform, Account, Session, Project, Path string
	Line                                      int
	Text                                      string
	Timestamp                                 string
}
type Index struct{ db *sql.DB }

func DefaultSources(home string) []Source {
	return []Source{{"cursor", filepath.Join(home, "Library/Application Support/Cursor/User/globalStorage/state.vscdb"), ""}, {"claude", filepath.Join(home, ".claude/projects"), ""}, {"codex", filepath.Join(home, ".codex/sessions"), ""}, {"cursor", filepath.Join(home, ".cursor/projects"), ""}, {"opencode", filepath.Join(home, ".local/share/opencode/opencode.db"), ""}, {"opencode", filepath.Join(home, ".local/share/opencode/opencode-local.db"), ""}}
}
func Open(path string) (*Index, error) {
	if path != ":memory:" {
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			return nil, err
		}
	}
	dsn := path
	if path != ":memory:" {
		absolute, e := filepath.Abs(path)
		if e != nil {
			return nil, e
		}
		dsn = (&url.URL{Scheme: "file", Path: absolute, RawQuery: "_pragma=busy_timeout%281000%29"}).String()
	}
	db, e := sql.Open("sqlite", dsn)
	if e != nil {
		return nil, e
	}
	if path == ":memory:" {
		db.SetMaxOpenConns(1)
	} else {
		db.SetMaxOpenConns(4)
		db.SetMaxIdleConns(4)
		if _, e = db.Exec("PRAGMA journal_mode=WAL"); e != nil {
			db.Close()
			return nil, e
		}
	}
	_, e = db.Exec(`CREATE VIRTUAL TABLE IF NOT EXISTS transcript_fts USING fts5(text, platform UNINDEXED, account UNINDEXED, session UNINDEXED, project, path UNINDEXED, line UNINDEXED, timestamp UNINDEXED)`)
	if e != nil {
		db.Close()
		return nil, e
	}
	_, e = db.Exec(`CREATE TABLE IF NOT EXISTS indexed_files (id TEXT PRIMARY KEY, watermark TEXT); CREATE TABLE IF NOT EXISTS indexed_rows(file_id TEXT, row_id INTEGER); CREATE INDEX IF NOT EXISTS indexed_rows_file ON indexed_rows(file_id)`)
	if e != nil {
		db.Close()
		return nil, e
	}
	return &Index{db}, nil
}
func (i *Index) Close() error { return i.db.Close() }

func scanJSONL(ctx context.Context, s Source, p string, l Limits, r *SyncReport, add func(Hit) error) error {
	f, e := os.Open(p)
	if e != nil {
		return e
	}
	defer f.Close()
	sc := newBoundedLines(f, int(l.MaxFileBytes), r)
	project := filepath.Base(filepath.Dir(p))
	session := strings.TrimSuffix(filepath.Base(p), ".jsonl")
	line := 0
	for sc.Scan() {
		if e = ctx.Err(); e != nil {
			return e
		}
		line++
		var v map[string]any
		if json.Unmarshal(sc.Bytes(), &v) != nil {
			continue
		}
		if x, ok := v["cwd"].(string); ok {
			project = x
		}
		if x, ok := v["sessionId"].(string); ok {
			session = x
		}
		payload, _ := v["payload"].(map[string]any)
		if v["type"] == "session_meta" {
			if x, ok := payload["cwd"].(string); ok {
				project = x
			}
			if x, ok := payload["id"].(string); ok {
				session = x
			}
			continue
		}
		var content any
		if m, ok := v["message"].(map[string]any); ok {
			content = m["content"]
		} else if payload != nil {
			if payload["type"] == "message" {
				content = payload["content"]
			} else if payload["type"] == "user_message" || payload["type"] == "agent_message" {
				content = payload["message"]
			}
		} else {
			content = v["content"]
		}
		text := extractText(content)
		if strings.TrimSpace(text) == "" {
			continue
		}
		stamp, _ := v["timestamp"].(string)
		if e = add(Hit{s.Platform, s.Account, session, project, p, line, text, stamp}); e != nil {
			return e
		}
		if r.Messages >= l.MaxMessages {
			r.Truncated = true
			break
		}
	}
	return sc.Err()
}
func extractText(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case []any:
		var a []string
		for _, v := range x {
			if m, ok := v.(map[string]any); ok {
				if t, ok := m["text"].(string); ok {
					a = append(a, t)
				}
			}
		}
		return strings.Join(a, "\n")
	}
	return ""
}
func (i *Index) scanOpenCode(ctx context.Context, s Source, l Limits, r *SyncReport, add func(Hit) error) error {
	db, e := sql.Open("sqlite", "file:"+filepath.ToSlash(s.Root)+"?mode=ro")
	if e != nil {
		return e
	}
	defer db.Close()
	rows, e := db.QueryContext(ctx, `SELECT p.session_id,s.directory,p.id,json_extract(p.data,'$.text'),p.time_created FROM part p JOIN session s ON s.id=p.session_id WHERE json_extract(p.data,'$.type')='text' ORDER BY s.time_updated DESC,p.id LIMIT ?`, l.MaxMessages-r.Messages+1)
	if e != nil {
		return e
	}
	defer rows.Close()
	r.Files++
	for rows.Next() {
		if r.Messages >= l.MaxMessages {
			r.Truncated = true
			break
		}
		var session, project, id, txt string
		var stamp int64
		if e = rows.Scan(&session, &project, &id, &txt, &stamp); e != nil {
			return e
		}
		if e = add(Hit{s.Platform, s.Account, session, project, s.Root + "#part=" + id, 0, txt, time.UnixMilli(stamp).UTC().Format(time.RFC3339)}); e != nil {
			return e
		}
	}
	return rows.Err()
}

// Search uses literal AND terms; callers cannot inject FTS syntax. Scope limits results.
func (i *Index) Search(ctx context.Context, q Query) ([]Hit, error) {
	if q.Limit <= 0 || q.Limit > 100 {
		q.Limit = 20
	}
	terms := strings.Fields(q.Text)
	if len(terms) == 0 {
		return []Hit{}, nil
	}
	for n, t := range terms {
		terms[n] = "\"" + strings.ReplaceAll(t, "\"", "\"\"") + "\""
	}
	sqlq := `SELECT platform,account,session,project,path,line,snippet(transcript_fts,0,'','', ' … ',40),timestamp FROM transcript_fts WHERE transcript_fts MATCH ?`
	join := " AND "
	if q.AnyTerms {
		join = " OR "
	}
	args := []any{strings.Join(terms, join)}
	if q.Scope != "" {
		sqlq += ` AND instr(lower(project || ' ' || account || ' ' || path),lower(?))>0`
		args = append(args, q.Scope)
	}
	for _, x := range q.Exclude {
		sqlq += ` AND instr(lower(project || ' ' || account || ' ' || path || ' ' || text),lower(?))=0`
		args = append(args, x)
	}
	if len(q.Platforms) > 0 {
		sqlq += " AND platform IN (" + strings.TrimRight(strings.Repeat("?,", len(q.Platforms)), ",") + ")"
		for _, p := range q.Platforms {
			args = append(args, p)
		}
	}
	sqlq += " ORDER BY rank"
	rows, e := i.db.QueryContext(ctx, sqlq, args...)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	hits := []Hit{}
	for rows.Next() {
		var h Hit
		if e = rows.Scan(&h.Platform, &h.Account, &h.Session, &h.Project, &h.Path, &h.Line, &h.Text, &h.Timestamp); e != nil {
			return nil, e
		}
		if q.Authorize != nil && q.Authorize(h) != nil {
			continue
		}
		hits = append(hits, h)
		if len(hits) >= q.Limit {
			break
		}
	}
	return hits, rows.Err()
}
