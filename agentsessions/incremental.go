package agentsessions

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type inputFile struct {
	s               Source
	path, key, mark string
}

func watermark(path string) string {
	var marks []string
	for _, p := range []string{path, path + "-wal"} {
		if st, e := os.Stat(p); e == nil {
			marks = append(marks, fmt.Sprintf("%s:%d:%d", p, st.Size(), st.ModTime().UnixNano()))
		}
	}
	return strings.Join(marks, "|")
}

// Sync atomically replaces changed files only, deleting disappeared or removed sources.
func (i *Index) Sync(ctx context.Context, sources []Source, l Limits) (r SyncReport, err error) {
	if l.MaxFiles <= 0 {
		l.MaxFiles = 2000
	}
	if l.MaxMessages <= 0 {
		l.MaxMessages = 100000
	}
	if l.MaxFileBytes <= 0 {
		l.MaxFileBytes = 32 << 20
	}
	var files []inputFile
	seen := map[string]bool{}
	discoveryOK := true
	for _, s := range sources {
		if err = ctx.Err(); err != nil {
			return r, err
		}
		var paths []string
		if s.Platform == "opencode" || (s.Platform == "cursor" && strings.HasSuffix(s.Root, ".vscdb")) {
			if _, e := os.Stat(s.Root); e == nil {
				paths = []string{s.Root}
			} else if !os.IsNotExist(e) {
				r.Warnings = append(r.Warnings, e.Error())
				discoveryOK = false
				r.Truncated = true
			}
		} else {
			e := filepath.WalkDir(s.Root, func(p string, d fs.DirEntry, e error) error {
				if e != nil {
					return e
				}
				if ctx.Err() != nil {
					return ctx.Err()
				}
				if !d.IsDir() && d.Type()&os.ModeSymlink == 0 && strings.HasSuffix(p, ".jsonl") && (s.Platform != "cursor" || strings.Contains(p, "agent-transcripts")) {
					paths = append(paths, p)
				}
				return nil
			})
			if e != nil && !os.IsNotExist(e) {
				r.Warnings = append(r.Warnings, e.Error())
				discoveryOK = false
				r.Truncated = true
			}
		}
		for _, p := range paths {
			key := s.Platform + "\x00" + s.Account + "\x00" + s.Root + "\x00" + p
			seen[key] = true
			files = append(files, inputFile{s, p, key, watermark(p)})
		}
	}
	sort.SliceStable(files, func(a, b int) bool {
		sa, ea := os.Stat(files[a].path)
		sb, eb := os.Stat(files[b].path)
		if ea != nil || eb != nil {
			return files[a].path < files[b].path
		}
		return sa.ModTime().After(sb.ModTime())
	})
	tx, e := i.db.BeginTx(ctx, nil)
	if e != nil {
		return r, e
	}
	defer tx.Rollback()
	// Remove old pre-watermark index rows once; all new rows have explicit ownership.
	if _, e = tx.ExecContext(ctx, `DELETE FROM transcript_fts WHERE rowid NOT IN (SELECT row_id FROM indexed_rows)`); e != nil {
		return r, e
	}
	remove := func(key string) error {
		if _, e := tx.ExecContext(ctx, `DELETE FROM transcript_fts WHERE rowid IN (SELECT row_id FROM indexed_rows WHERE file_id=?)`, key); e != nil {
			return e
		}
		_, e := tx.ExecContext(ctx, `DELETE FROM indexed_rows WHERE file_id=?`, key)
		return e
	}
	if discoveryOK {
		rows, e := tx.QueryContext(ctx, `SELECT id FROM indexed_files`)
		if e != nil {
			return r, e
		}
		var gone []string
		for rows.Next() {
			var key string
			if e = rows.Scan(&key); e != nil {
				rows.Close()
				return r, e
			}
			if !seen[key] {
				gone = append(gone, key)
			}
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return r, e
		}
		for _, key := range gone {
			if e = remove(key); e != nil {
				return r, e
			}
			if _, e = tx.ExecContext(ctx, `DELETE FROM indexed_files WHERE id=?`, key); e != nil {
				return r, e
			}
		}
	}
	for _, f := range files {
		var old string
		_ = tx.QueryRowContext(ctx, `SELECT watermark FROM indexed_files WHERE id=?`, f.key).Scan(&old)
		fingerprint := fmt.Sprintf("%s|linecap:%d", f.mark, l.MaxFileBytes)
		var progress struct {
			Mark  string
			Count int
		}
		_ = json.Unmarshal([]byte(old), &progress)
		skip := 0
		if progress.Mark == fingerprint {
			skip = progress.Count
		}
		if old == fingerprint && old != "" {
			r.Unchanged++
			continue
		}
		if r.Files >= l.MaxFiles || r.Messages >= l.MaxMessages {
			r.Truncated = true
			break
		}
		r.Files++
		if _, e = tx.ExecContext(ctx, "SAVEPOINT changed_file"); e != nil {
			return r, e
		}
		if skip == 0 {
			if e = remove(f.key); e != nil {
				return r, e
			}
		}
		before := r.Messages
		local := SyncReport{}
		scanLimits := l
		scanLimits.MaxMessages = skip + l.MaxMessages - before
		add := func(h Hit) error {
			local.Messages++
			if local.Messages <= skip {
				return nil
			}
			if r.Messages >= l.MaxMessages {
				r.Truncated = true
				return nil
			}
			res, e := tx.ExecContext(ctx, `INSERT INTO transcript_fts(text,platform,account,session,project,path,line,timestamp) VALUES(?,?,?,?,?,?,?,?)`, h.Text, h.Platform, h.Account, h.Session, h.Project, h.Path, h.Line, h.Timestamp)
			if e != nil {
				return e
			}
			id, e := res.LastInsertId()
			if e != nil {
				return e
			}
			if _, e = tx.ExecContext(ctx, `INSERT INTO indexed_rows VALUES(?,?)`, f.key, id); e == nil {
				r.Messages++
			}
			return e
		}
		s := f.s
		s.Root = f.path
		switch {
		case s.Platform == "opencode":
			e = i.scanOpenCode(ctx, s, scanLimits, &local, add)
		case s.Platform == "cursor" && strings.HasSuffix(f.path, ".vscdb"):
			e = scanCursor(ctx, s, scanLimits, &local, add)
		default:
			e = scanJSONL(ctx, s, f.path, scanLimits, &local, add)
		}
		r.Truncated = r.Truncated || local.Truncated
		r.Warnings = append(r.Warnings, local.Warnings...)
		if e != nil {
			if ctx.Err() != nil {
				return r, ctx.Err()
			}
			tx.ExecContext(ctx, "ROLLBACK TO changed_file")
			r.Messages = before
			r.Truncated = true
			r.Warnings = append(r.Warnings, fmt.Sprintf("%s: %v", f.path, e))
		} else {
			mark := fingerprint
			if local.Messages >= scanLimits.MaxMessages {
				progress.Mark = fingerprint
				progress.Count = skip + r.Messages - before
				encoded, _ := json.Marshal(progress)
				mark = string(encoded)
			}
			if watermark(f.path) != f.mark {
				mark = ""
			}
			if _, e = tx.ExecContext(ctx, `INSERT OR REPLACE INTO indexed_files VALUES(?,?)`, f.key, mark); e != nil {
				return r, e
			}
		}
		if _, e = tx.ExecContext(ctx, "RELEASE changed_file"); e != nil {
			return r, e
		}
	}
	if err = ctx.Err(); err != nil {
		return r, err
	}
	if e = tx.QueryRowContext(ctx, `SELECT count(*) FROM transcript_fts`).Scan(&r.TotalMessages); e != nil {
		return r, e
	}
	return r, tx.Commit()
}

type boundedLines struct {
	reader *bufio.Reader
	limit  int
	data   []byte
	err    error
	report *SyncReport
}

func newBoundedLines(r io.Reader, limit int, report *SyncReport) *boundedLines {
	return &boundedLines{reader: bufio.NewReaderSize(r, 64<<10), limit: limit, report: report}
}
func (s *boundedLines) Scan() bool {
	if s.err != nil {
		return false
	}
	s.data = nil
	tooBig := false
	for {
		fragment, e := s.reader.ReadSlice('\n')
		if !tooBig {
			if len(s.data)+len(fragment) > s.limit {
				tooBig = true
				s.data = nil
				s.report.Truncated = true
				s.report.Warnings = append(s.report.Warnings, "JSONL line exceeds byte limit; skipped line")
			} else {
				s.data = append(s.data, fragment...)
			}
		}
		if e == bufio.ErrBufferFull {
			continue
		}
		if e != nil {
			s.err = e
		}
		if tooBig {
			return true
		}
		return len(s.data) > 0
	}
}
func (s *boundedLines) Bytes() []byte { return s.data }
func (s *boundedLines) Err() error {
	if errors.Is(s.err, io.EOF) {
		return nil
	}
	return s.err
}
