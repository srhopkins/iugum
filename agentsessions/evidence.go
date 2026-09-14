package agentsessions

import (
	"context"
	"database/sql"
	"errors"
	"unicode/utf8"
)

var ErrNotIndexed = errors.New("evidence is not in the current transcript index")
var ErrAuthorizationRequired = errors.New("evidence retrieval requires a current policy check")

type EvidenceOptions struct {
	Before, After, MaxBytes int
	Authorize               func(Hit) error
}
type Evidence struct {
	Match     Hit
	Context   []Hit
	Truncated bool
	// Snapshot indicates this is indexed text, not a fresh read of the native source.
	Snapshot bool
}

// Get resolves an existing index record, never reads a caller-specified file, and
// checks the current policy against stored metadata before disclosing any text.
// Before/After count native JSONL lines (up to 20); OpenCode has no line context.
func (i *Index) Get(ctx context.Context, reference Hit, options EvidenceOptions) (Evidence, error) {
	result := Evidence{Snapshot: true, Context: []Hit{}}
	if options.Authorize == nil {
		return result, ErrAuthorizationRequired
	}
	var h Hit
	err := i.db.QueryRowContext(ctx, `SELECT platform,account,session,project,path,line,text,timestamp FROM transcript_fts WHERE platform=? AND account=? AND session=? AND path=? AND line=? LIMIT 1`, reference.Platform, reference.Account, reference.Session, reference.Path, reference.Line).Scan(&h.Platform, &h.Account, &h.Session, &h.Project, &h.Path, &h.Line, &h.Text, &h.Timestamp)
	if errors.Is(err, sql.ErrNoRows) {
		return result, ErrNotIndexed
	}
	if err != nil {
		return result, err
	}
	if err = options.Authorize(h); err != nil {
		return result, err
	}
	if options.MaxBytes <= 0 {
		options.MaxBytes = 16384
	}
	if options.MaxBytes > 65536 {
		options.MaxBytes = 65536
	}
	remaining := options.MaxBytes
	trim := func(h Hit) Hit {
		if len(h.Text) > remaining {
			h.Text = h.Text[:remaining]
			for !utf8.ValidString(h.Text) && len(h.Text) > 0 {
				h.Text = h.Text[:len(h.Text)-1]
			}
			result.Truncated = true
		}
		remaining -= len(h.Text)
		return h
	}
	result.Match = trim(h)
	if options.Before < 0 {
		options.Before = 0
	}
	if options.After < 0 {
		options.After = 0
	}
	if options.Before > 20 {
		options.Before = 20
	}
	if options.After > 20 {
		options.After = 20
	}
	if h.Line == 0 || remaining == 0 || (options.Before == 0 && options.After == 0) {
		return result, nil
	}
	rows, err := i.db.QueryContext(ctx, `SELECT platform,account,session,project,path,line,text,timestamp FROM transcript_fts WHERE platform=? AND account=? AND session=? AND path=? AND CAST(line AS INTEGER) BETWEEN ? AND ? AND CAST(line AS INTEGER)<>? ORDER BY CAST(line AS INTEGER) LIMIT 40`, h.Platform, h.Account, h.Session, h.Path, h.Line-options.Before, h.Line+options.After, h.Line)
	if err != nil {
		return result, err
	}
	defer rows.Close()
	for rows.Next() {
		var adjacent Hit
		if err = rows.Scan(&adjacent.Platform, &adjacent.Account, &adjacent.Session, &adjacent.Project, &adjacent.Path, &adjacent.Line, &adjacent.Text, &adjacent.Timestamp); err != nil {
			return result, err
		}
		if options.Authorize(adjacent) != nil {
			continue
		}
		if remaining == 0 {
			result.Truncated = true
			break
		}
		result.Context = append(result.Context, trim(adjacent))
	}
	return result, rows.Err()
}

type SearchResult struct {
	Hits      []Hit
	MatchMode string
}

// SearchFallback first requires all words, then tries any word only if there
// are no authorized results. MatchMode lets the UI label the relaxed match.
func (i *Index) SearchFallback(ctx context.Context, q Query) (SearchResult, error) {
	q.AnyTerms = false
	hits, err := i.Search(ctx, q)
	if err != nil {
		return SearchResult{}, err
	}
	if len(hits) > 0 {
		return SearchResult{hits, "all_terms"}, nil
	}
	q.AnyTerms = true
	hits, err = i.Search(ctx, q)
	return SearchResult{hits, "any_terms"}, err
}
