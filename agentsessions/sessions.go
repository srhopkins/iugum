package agentsessions

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

// SessionSummary identifies one source transcript. The key includes source and
// account, so equal native IDs from different sources never merge.
type SessionSummary struct {
	ID        string `json:"id"`
	Provider  string `json:"provider"`
	SessionID string `json:"sessionId"`
	Title     string `json:"title"`
	Snippet   string `json:"snippet"`
}

func transcriptKey(h Hit) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(h.Platform+"\x00"+h.Account+"\x00"+h.Session+"\x00"+sourceTranscriptPath(h.Path))))
}
func excerpt(text, query string) string {
	text = strings.Join(strings.Fields(text), " ")
	start := strings.Index(strings.ToLower(text), strings.ToLower(query))
	if start < 0 {
		start = 0
	}
	if start > 80 {
		start -= 80
	} else {
		start = 0
	}
	// Slice runes after locating the byte offset to preserve UTF-8.
	prefix := text[:start]
	start = len([]rune(prefix))
	runes := []rune(text)
	end := start + 240
	if end > len(runes) {
		end = len(runes)
	}
	result := string(runes[start:end])
	if start > 0 {
		result = "…" + result
	}
	if end < len(runes) {
		result += "…"
	}
	return result
}

// Sessions searches all indexed messages using literal, case-insensitive terms.
// Authorization runs for every stored row before it contributes to a result.
func (i *Index) Sessions(ctx context.Context, query string, authorize func(Hit) error) ([]SessionSummary, error) {
	if authorize == nil {
		return nil, ErrAuthorizationRequired
	}
	rows, err := i.db.QueryContext(ctx, `SELECT platform,account,session,project,path,line,text,timestamp FROM transcript_fts ORDER BY path,CAST(line AS INTEGER)`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SessionSummary{}
	positions := map[string]int{}
	matched := map[string]bool{}
	terms := strings.Fields(strings.ToLower(query))
	for rows.Next() {
		var h Hit
		if err = rows.Scan(&h.Platform, &h.Account, &h.Session, &h.Project, &h.Path, &h.Line, &h.Text, &h.Timestamp); err != nil {
			return nil, err
		}
		if authorize(h) != nil {
			continue
		}
		key := transcriptKey(h)
		n, ok := positions[key]
		if !ok {
			n = len(out)
			positions[key] = n
			out = append(out, SessionSummary{ID: key, Provider: h.Platform, SessionID: h.Session, Title: sessionTitle(h)})
		}
		match := true
		for _, term := range terms {
			if !strings.Contains(strings.ToLower(h.Text), term) {
				match = false
				break
			}
		}
		if match && !matched[key] {
			out[n].Snippet = excerpt(h.Text, query)
			matched[key] = true
		}
	}
	result := []SessionSummary{}
	for _, v := range out {
		titleMatch := true
		for _, term := range terms {
			if !strings.Contains(strings.ToLower(v.Title), term) {
				titleMatch = false
				break
			}
		}
		if len(terms) == 0 || matched[v.ID] || titleMatch {
			result = append(result, v)
		}
	}
	return result, rows.Err()
}

// Transcript reads only existing indexed rows and rechecks policy. It cannot
// accept a source path or open a provider. The bound is explicit to callers.
func (i *Index) Transcript(ctx context.Context, key string, authorize func(Hit) error) ([]Hit, bool, error) {
	if authorize == nil {
		return nil, false, ErrAuthorizationRequired
	}
	rows, err := i.db.QueryContext(ctx, `SELECT platform,account,session,project,path,line,text,timestamp FROM transcript_fts ORDER BY CAST(line AS INTEGER)`)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	out := []Hit{}
	size := 0
	for rows.Next() {
		var h Hit
		if err = rows.Scan(&h.Platform, &h.Account, &h.Session, &h.Project, &h.Path, &h.Line, &h.Text, &h.Timestamp); err != nil {
			return nil, false, err
		}
		if transcriptKey(h) != key || authorize(h) != nil {
			continue
		}
		size += len(h.Text)
		if size > 4<<20 {
			return out, true, nil
		}
		out = append(out, h)
	}
	return out, false, rows.Err()
}

func sourceTranscriptPath(path string) string {
	if at := strings.Index(path, "#bubble="); at >= 0 {
		return path[:at]
	}
	if at := strings.Index(path, "#part="); at >= 0 {
		return path[:at]
	}
	return path
}
func sessionTitle(h Hit) string {
	path := sourceTranscriptPath(h.Path)
	if h.Platform != "opencode" && !(h.Platform == "cursor" && strings.HasSuffix(path, ".vscdb")) {
		return excerpt(h.Text, "")
	}
	db, err := sql.Open("sqlite", (&url.URL{Scheme: "file", Path: path, RawQuery: "mode=ro"}).String())
	if err != nil {
		return excerpt(h.Text, "")
	}
	defer db.Close()
	title := ""
	if h.Platform == "opencode" {
		_ = db.QueryRow(`SELECT title FROM session WHERE id=?`, h.Session).Scan(&title)
	} else {
		var raw []byte
		if db.QueryRow(`SELECT value FROM cursorDiskKV WHERE key=?`, "composerData:"+h.Session).Scan(&raw) == nil {
			var value struct {
				Name  string `json:"name"`
				Title string `json:"title"`
			}
			if json.Unmarshal(raw, &value) == nil {
				title = value.Name
				if title == "" {
					title = value.Title
				}
			}
		}
	}
	if title != "" {
		return excerpt(title, "")
	}
	return excerpt(h.Text, "")
}
