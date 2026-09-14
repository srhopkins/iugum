package agentsessions

import (
	"context"
	"strings"
)

// Latest returns the newest dated message in an authorized session. Scope matches
// source metadata, never transcript prose (which can mention unrelated projects).
// Undated records cannot establish recency and are deliberately excluded.
func (i *Index) Latest(ctx context.Context, scope string, authorize func(Hit) error) (*Hit, error) {
	if authorize == nil {
		return nil, ErrAuthorizationRequired
	}
	scope = strings.ToLower(strings.TrimSpace(scope))
	sqlq := `SELECT platform,account,session,project,path,line,text,timestamp FROM transcript_fts WHERE julianday(timestamp) IS NOT NULL`
	var args []any
	switch scope {
	case "", "all":
	case "claude", "cursor", "codex", "opencode":
		sqlq += ` AND platform=?`
		args = append(args, scope)
	case "ffai", "futurefit", "futurefit ai":
		sqlq += ` AND (instr(lower(project || ' ' || account || ' ' || path),'ffai')>0 OR instr(lower(project || ' ' || account || ' ' || path),'futurefit')>0)`
	default:
		sqlq += ` AND instr(lower(project || ' ' || account || ' ' || path),?)>0`
		args = append(args, scope)
	}
	sqlq += ` ORDER BY julianday(timestamp) DESC, CAST(line AS INTEGER) DESC`
	rows, err := i.db.QueryContext(ctx, sqlq, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var h Hit
		if err = rows.Scan(&h.Platform, &h.Account, &h.Session, &h.Project, &h.Path, &h.Line, &h.Text, &h.Timestamp); err != nil {
			return nil, err
		}
		if authorize(h) != nil {
			continue
		}
		return &h, nil
	}
	return nil, rows.Err()
}

// PreviousMessage finds the preceding indexed message even when tool events
// occupy many native lines. It never crosses a session or source boundary.
func (i *Index) PreviousMessage(ctx context.Context, h Hit, authorize func(Hit) error) (*Hit, error) {
	if authorize == nil {
		return nil, ErrAuthorizationRequired
	}
	if err := authorize(h); err != nil {
		return nil, err
	}
	rows, err := i.db.QueryContext(ctx, `SELECT platform,account,session,project,path,line,text,timestamp FROM transcript_fts WHERE platform=? AND account=? AND session=? AND path=? AND text<>? AND (julianday(timestamp)<julianday(?) OR (julianday(timestamp)=julianday(?) AND CAST(line AS INTEGER)<?)) ORDER BY julianday(timestamp) DESC, CAST(line AS INTEGER) DESC`, h.Platform, h.Account, h.Session, h.Path, h.Text, h.Timestamp, h.Timestamp, h.Line)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var p Hit
		if err = rows.Scan(&p.Platform, &p.Account, &p.Session, &p.Project, &p.Path, &p.Line, &p.Text, &p.Timestamp); err != nil {
			return nil, err
		}
		if authorize(p) == nil {
			return &p, nil
		}
	}
	return nil, rows.Err()
}
