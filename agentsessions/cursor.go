package agentsessions

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// scanCursor reads every canonical bubble row, including bubbles omitted from
// composerData.fullConversationHeadersOnly. A read transaction includes WAL.
func scanCursor(ctx context.Context, s Source, l Limits, r *SyncReport, add func(Hit) error) error {
	db, e := sql.Open("sqlite", (&url.URL{Scheme: "file", Path: s.Root, RawQuery: "mode=ro"}).String())
	if e != nil {
		return e
	}
	defer db.Close()
	tx, e := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if e != nil {
		return e
	}
	defer tx.Rollback()
	workspaces := map[string]string{}
	headers, e := tx.QueryContext(ctx, `SELECT composerId,COALESCE(workspaceId,'') FROM composerHeaders`)
	if e == nil {
		for headers.Next() {
			var id, w string
			if e = headers.Scan(&id, &w); e != nil {
				headers.Close()
				return e
			}
			workspaces[id] = w
		}
		if e = headers.Err(); e != nil {
			headers.Close()
			return e
		}
		headers.Close()
	}
	rows, e := tx.QueryContext(ctx, `SELECT key,value FROM cursorDiskKV WHERE key LIKE 'bubbleId:%' ORDER BY key`)
	if e != nil {
		return e
	}
	defer rows.Close()
	for rows.Next() {
		if r.Messages >= l.MaxMessages {
			r.Truncated = true
			break
		}
		var key string
		var raw []byte
		if e = rows.Scan(&key, &raw); e != nil {
			return e
		}
		if int64(len(raw)) > l.MaxFileBytes {
			r.Truncated = true
			r.Warnings = append(r.Warnings, "Cursor bubble exceeds byte limit: "+key)
			continue
		}
		parts := strings.SplitN(key, ":", 3)
		if len(parts) != 3 {
			continue
		}
		session := parts[1]
		var bubble struct {
			Text      string `json:"text"`
			CreatedAt string `json:"createdAt"`
			Thinking  *struct {
				Text string `json:"text"`
			} `json:"thinking"`
			Tool *struct {
				Name   string          `json:"name"`
				Tool   string          `json:"tool"`
				Params json.RawMessage `json:"params"`
				Result json.RawMessage `json:"result"`
			} `json:"toolFormerData"`
		}
		if json.Unmarshal(raw, &bubble) != nil {
			r.Warnings = append(r.Warnings, "Invalid Cursor bubble: "+key)
			continue
		}
		var text []string
		if bubble.Text != "" {
			text = append(text, bubble.Text)
		}
		if bubble.Thinking != nil && bubble.Thinking.Text != "" {
			text = append(text, "[thinking]\n"+bubble.Thinking.Text)
		}
		if t := bubble.Tool; t != nil {
			name := t.Name
			if name == "" {
				name = t.Tool
			}
			text = append(text, fmt.Sprintf("[tool %s]\nparams: %s\nresult: %s", name, t.Params, t.Result))
		}
		if len(text) == 0 {
			continue
		}
		workspace := workspaces[session]
		project := "cursor-workspace:" + workspace
		if workspace == "" {
			project = "cursor-session:" + session
		} else if filepath.Base(workspace) == workspace && workspace != "." && workspace != ".." {
			workspaceFile := filepath.Join(filepath.Dir(filepath.Dir(s.Root)), "workspaceStorage", workspace, "workspace.json")
			var w struct {
				Folder    string `json:"folder"`
				Workspace string `json:"workspace"`
			}
			if b, e := os.ReadFile(workspaceFile); e == nil && json.Unmarshal(b, &w) == nil {
				folder := w.Folder
				if folder == "" {
					folder = w.Workspace
				}
				if u, e := url.Parse(folder); e == nil && u.Scheme == "file" && u.Host == "" {
					project = u.Path
				}
			}
		}
		if e = add(Hit{s.Platform, s.Account, session, project, s.Root + "#cursorDiskKV/" + key, 0, strings.Join(text, "\n\n"), bubble.CreatedAt}); e != nil {
			return e
		}
	}
	return rows.Err()
}
