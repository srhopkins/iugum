package sqladapt

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"

	"github.com/srhopkins/iugum/adapter/observe/ql"
	"github.com/srhopkins/iugum/contract"
	"github.com/srhopkins/iugum/dirs"
	"github.com/srhopkins/iugum/plugin"
)

const (
	metricsFile = "observe-metrics.db"
	logsFile    = "observe-logs.db"
	legacyFile  = "observe.db"
)

func init() {
	plugin.RegisterObserver("sqlite", func(cfg map[string]string) (contract.Observer, error) {
		dir := cfg["data_dir"]
		if dir == "" {
			var err error
			dir, err = dirs.ResolveData("")
			if err != nil {
				return nil, err
			}
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
		return OpenDir(dir)
	})
}

// Store is metrics and logs in two SQLite files (observe-metrics.db, observe-logs.db).
type Store struct {
	metrics   *sql.DB
	logs      *sql.DB
	metricsMu sync.Mutex
	logsMu    sync.Mutex
}

// Open treats a .db path as its parent directory, then opens the split files.
func Open(path string) (*Store, error) {
	dir := path
	if strings.HasSuffix(strings.ToLower(path), ".db") {
		dir = filepath.Dir(path)
	}
	return OpenDir(dir)
}

func OpenDir(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	if err := migrateLegacy(dir); err != nil {
		return nil, err
	}
	metrics, err := openFile(filepath.Join(dir, metricsFile), metricsSchema)
	if err != nil {
		return nil, err
	}
	logsDB, err := openFile(filepath.Join(dir, logsFile), logsSchema)
	if err != nil {
		_ = metrics.Close()
		return nil, err
	}
	if err := ensureLogFTS(logsDB); err != nil {
		_ = metrics.Close()
		_ = logsDB.Close()
		return nil, err
	}
	return &Store{metrics: metrics, logs: logsDB}, nil
}

func openFile(path, schema string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`PRAGMA journal_mode=WAL; PRAGMA synchronous=NORMAL; PRAGMA busy_timeout=5000;`); err != nil {
		_ = db.Close()
		return nil, err
	}
	if _, err := db.Exec(schema); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

const metricsSchema = `
		CREATE TABLE IF NOT EXISTS samples (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			labels TEXT NOT NULL DEFAULT '{}',
			value REAL NOT NULL,
			time_us INTEGER NOT NULL
		);
		CREATE INDEX IF NOT EXISTS samples_name_time ON samples(name, time_us);
`

const logsSchema = `
		CREATE TABLE IF NOT EXISTS logs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			time_us INTEGER NOT NULL,
			stream TEXT NOT NULL DEFAULT '',
			level TEXT NOT NULL DEFAULT '',
			message TEXT NOT NULL,
			attrs TEXT NOT NULL DEFAULT '{}'
		);
		CREATE INDEX IF NOT EXISTS logs_stream_time ON logs(stream, time_us);
`

func migrateLegacy(dir string) error {
	old := filepath.Join(dir, legacyFile)
	if _, err := os.Stat(old); err != nil {
		return nil
	}
	if fileExists(filepath.Join(dir, metricsFile)) && fileExists(filepath.Join(dir, logsFile)) {
		return nil
	}
	src, err := sql.Open("sqlite", old)
	if err != nil {
		return err
	}
	defer src.Close()
	src.SetMaxOpenConns(1)

	metrics, err := openFile(filepath.Join(dir, metricsFile), metricsSchema)
	if err != nil {
		return err
	}
	if err := copySamples(src, metrics); err != nil {
		_ = metrics.Close()
		return err
	}
	_ = metrics.Close()

	logsDB, err := openFile(filepath.Join(dir, logsFile), logsSchema)
	if err != nil {
		return err
	}
	if err := copyLogs(src, logsDB); err != nil {
		_ = logsDB.Close()
		return err
	}
	if err := ensureLogFTS(logsDB); err != nil {
		_ = logsDB.Close()
		return err
	}
	_ = logsDB.Close()
	return os.Rename(old, old+".bak")
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func copySamples(src, dst *sql.DB) error {
	rows, err := src.Query(`SELECT name, labels, value, time_us FROM samples`)
	if err != nil {
		return err
	}
	defer rows.Close()
	tx, err := dst.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.Prepare(`INSERT INTO samples(name, labels, value, time_us) VALUES (?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for rows.Next() {
		var name, labels string
		var value float64
		var timeUS int64
		if err := rows.Scan(&name, &labels, &value, &timeUS); err != nil {
			return err
		}
		if _, err := stmt.Exec(name, labels, value, timeUS); err != nil {
			return err
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	return tx.Commit()
}

func copyLogs(src, dst *sql.DB) error {
	rows, err := src.Query(`SELECT time_us, stream, level, message, attrs FROM logs`)
	if err != nil {
		return err
	}
	defer rows.Close()
	tx, err := dst.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.Prepare(`INSERT INTO logs(time_us, stream, level, message, attrs) VALUES (?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for rows.Next() {
		var timeUS int64
		var stream, level, message, attrs string
		if err := rows.Scan(&timeUS, &stream, &level, &message, &attrs); err != nil {
			return err
		}
		if _, err := stmt.Exec(timeUS, stream, level, message, attrs); err != nil {
			return err
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	return tx.Commit()
}

func ensureLogFTS(db *sql.DB) error {
	if _, err := db.Exec(`
		CREATE VIRTUAL TABLE IF NOT EXISTS logs_fts USING fts5(
			message, stream,
			content='logs',
			content_rowid='id'
		);
		CREATE TRIGGER IF NOT EXISTS logs_ai AFTER INSERT ON logs BEGIN
			INSERT INTO logs_fts(rowid, message, stream) VALUES (new.id, new.message, new.stream);
		END;
		CREATE TRIGGER IF NOT EXISTS logs_ad AFTER DELETE ON logs BEGIN
			INSERT INTO logs_fts(logs_fts, rowid, message, stream) VALUES('delete', old.id, old.message, old.stream);
		END;
		CREATE TRIGGER IF NOT EXISTS logs_au AFTER UPDATE ON logs BEGIN
			INSERT INTO logs_fts(logs_fts, rowid, message, stream) VALUES('delete', old.id, old.message, old.stream);
			INSERT INTO logs_fts(rowid, message, stream) VALUES (new.id, new.message, new.stream);
		END;
	`); err != nil {
		return err
	}
	_, _ = db.Exec(`INSERT INTO logs_fts(logs_fts) VALUES('rebuild')`)
	return nil
}

func (Store) Name() string { return "sqlite" }

func (s *Store) Close() error {
	err1 := s.metrics.Close()
	err2 := s.logs.Close()
	if err1 != nil {
		return err1
	}
	return err2
}

func (s *Store) IngestMetrics(_ context.Context, samples []contract.Sample) error {
	s.metricsMu.Lock()
	defer s.metricsMu.Unlock()
	now := time.Now().UnixMicro()
	tx, err := s.metrics.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.Prepare(`INSERT INTO samples(name, labels, value, time_us) VALUES (?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, raw := range samples {
		sm := ql.NormalizeSample(raw)
		if sm.TimeUS == 0 {
			sm.TimeUS = now
		}
		labs, err := json.Marshal(sm.Labels)
		if err != nil {
			return err
		}
		if sm.Labels == nil {
			labs = []byte("{}")
		}
		if _, err := stmt.Exec(sm.Name, string(labs), sm.Value, sm.TimeUS); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) QueryMetrics(_ context.Context, q contract.MetricQuery) ([]contract.Series, error) {
	if q.Expr != "" {
		p, err := ql.ParsePromQL(q.Expr)
		if err != nil {
			return nil, err
		}
		ql.ApplyPromQL(&q, p)
	}
	s.metricsMu.Lock()
	defer s.metricsMu.Unlock()
	rows, err := s.metrics.Query(`
		SELECT name, labels, value, time_us FROM samples
		WHERE (? = '' OR name = ?)
		  AND (? = 0 OR time_us >= ?)
		  AND (? = 0 OR time_us <= ?)
		ORDER BY time_us ASC
	`, q.Name, q.Name, q.StartUS, q.StartUS, q.EndUS, q.EndUS)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type key struct{ name, labs string }
	by := map[key]*contract.Series{}
	order := []key{}
	for rows.Next() {
		var name, labsJSON string
		var value float64
		var timeUS int64
		if err := rows.Scan(&name, &labsJSON, &value, &timeUS); err != nil {
			return nil, err
		}
		labs := map[string]string{}
		if labsJSON != "" && labsJSON != "{}" {
			_ = json.Unmarshal([]byte(labsJSON), &labs)
		}
		if !labelsMatch(q.Labels, labs) {
			continue
		}
		k := key{name: name, labs: labsJSON}
		ser, ok := by[k]
		if !ok {
			ser = &contract.Series{Name: name, Labels: labs}
			by[k] = ser
			order = append(order, k)
		}
		ser.Points = append(ser.Points, [2]float64{float64(timeUS) / 1e6, value})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]contract.Series, 0, len(order))
	for _, k := range order {
		ser := *by[k]
		if q.MaxPoints > 0 && len(ser.Points) > q.MaxPoints {
			ser.Points = downsample(ser.Points, q.MaxPoints)
		}
		out = append(out, ser)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// QueryMetricRange evaluates a PromQL range query (selectors, rate, avg by).
// Samples are fetched with Prometheus lookback, then step-aligned.
func (s *Store) QueryMetricRange(ctx context.Context, q contract.MetricRangeQuery) ([]contract.Series, error) {
	rx, err := ql.RangeFromQuery(q)
	if err != nil {
		return nil, err
	}
	series, err := s.QueryMetrics(ctx, ql.SampleQuery(rx, q.StartUS, q.EndUS))
	if err != nil {
		return nil, err
	}
	return ql.EvalRange(series, rx, q.StartUS, q.EndUS, q.StepUS), nil
}

func (s *Store) IngestLogs(_ context.Context, recs []contract.Log) error {
	s.logsMu.Lock()
	defer s.logsMu.Unlock()
	now := time.Now().UnixMicro()
	tx, err := s.logs.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.Prepare(`INSERT INTO logs(time_us, stream, level, message, attrs) VALUES (?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, r := range recs {
		if r.TimeUS == 0 {
			r.TimeUS = now
		}
		attrs, err := json.Marshal(r.Attrs)
		if err != nil {
			return err
		}
		if r.Attrs == nil {
			attrs = []byte("{}")
		}
		if _, err := stmt.Exec(r.TimeUS, r.Stream, r.Level, r.Message, string(attrs)); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) SearchLogs(_ context.Context, q contract.LogQuery) ([]contract.Log, error) {
	if q.Expr != "" {
		p, err := ql.ParseLogQL(q.Expr)
		if err != nil {
			return nil, err
		}
		ql.ApplyLogQL(&q, p)
	}
	if q.Limit <= 0 {
		q.Limit = 200
	}
	return s.queryLogs(logFilter{
		stream: q.Stream,
		text:   q.Text,
		start:  q.StartUS,
		end:    q.EndUS,
		limit:  q.Limit,
	})
}

// SearchLogRange is a Loki-style range read for a Perses log panel:
// stream labels + |= over StartUS/EndUS. FTS5 runs when a word is set.
// StepUS is ignored (log lines, not Loki metric queries).
func (s *Store) SearchLogRange(_ context.Context, q contract.LogRangeQuery) ([]contract.Log, error) {
	var labs map[string]string
	if q.Expr != "" {
		p, err := ql.ParseLogQL(q.Expr)
		if err != nil {
			return nil, err
		}
		ql.ApplyLogQLRange(&q, p)
		labs = p.Labels
	}
	if q.Limit <= 0 {
		q.Limit = 200
	}
	level, attrs := ql.LogStreamAttrs(labs)
	f := logFilter{
		stream: q.Stream,
		level:  level,
		text:   q.Text,
		start:  q.StartUS,
		end:    q.EndUS,
		limit:  q.Limit,
	}
	if len(attrs) > 0 {
		f.limit = -1
	}
	out, err := s.queryLogs(f)
	if err != nil {
		return nil, err
	}
	if len(attrs) == 0 {
		return out, nil
	}
	kept := out[:0]
	for _, r := range out {
		if labelsMatch(attrs, r.Attrs) {
			kept = append(kept, r)
			if len(kept) >= q.Limit {
				break
			}
		}
	}
	return kept, nil
}

type logFilter struct {
	stream string
	level  string
	text   string
	start  int64
	end    int64
	limit  int
}

func (s *Store) queryLogs(f logFilter) ([]contract.Log, error) {
	if f.limit == 0 {
		f.limit = 200
	}
	s.logsMu.Lock()
	defer s.logsMu.Unlock()
	var rows *sql.Rows
	var err error
	if f.text != "" {
		match := ftsQuote(f.text)
		rows, err = s.logs.Query(`
			SELECT l.time_us, l.stream, l.level, l.message, l.attrs
			FROM logs l
			JOIN logs_fts f ON f.rowid = l.id
			WHERE f MATCH ?
			  AND (? = '' OR l.stream = ?)
			  AND (? = '' OR l.level = ?)
			  AND (? = 0 OR l.time_us >= ?)
			  AND (? = 0 OR l.time_us <= ?)
			ORDER BY l.time_us DESC
			LIMIT ?
		`, match, f.stream, f.stream, f.level, f.level, f.start, f.start, f.end, f.end, f.limit)
		if err != nil {
			rows, err = s.logs.Query(`
				SELECT time_us, stream, level, message, attrs FROM logs
				WHERE (? = '' OR stream = ?)
				  AND (? = '' OR level = ?)
				  AND message LIKE ?
				  AND (? = 0 OR time_us >= ?)
				  AND (? = 0 OR time_us <= ?)
				ORDER BY time_us DESC
				LIMIT ?
			`, f.stream, f.stream, f.level, f.level, "%"+f.text+"%", f.start, f.start, f.end, f.end, f.limit)
		}
	} else {
		rows, err = s.logs.Query(`
			SELECT time_us, stream, level, message, attrs FROM logs
			WHERE (? = '' OR stream = ?)
			  AND (? = '' OR level = ?)
			  AND (? = 0 OR time_us >= ?)
			  AND (? = 0 OR time_us <= ?)
			ORDER BY time_us DESC
			LIMIT ?
		`, f.stream, f.stream, f.level, f.level, f.start, f.start, f.end, f.end, f.limit)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []contract.Log
	for rows.Next() {
		var r contract.Log
		var attrs string
		if err := rows.Scan(&r.TimeUS, &r.Stream, &r.Level, &r.Message, &attrs); err != nil {
			return nil, err
		}
		if attrs != "" && attrs != "{}" {
			_ = json.Unmarshal([]byte(attrs), &r.Attrs)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func ftsQuote(text string) string {
	return `"` + strings.ReplaceAll(text, `"`, `""`) + `"`
}

func labelsMatch(want, have map[string]string) bool {
	if len(want) == 0 {
		return true
	}
	for k, v := range want {
		if have == nil || have[k] != v {
			return false
		}
	}
	return true
}

func downsample(pts [][2]float64, max int) [][2]float64 {
	if max <= 0 || len(pts) <= max {
		return pts
	}
	out := make([][2]float64, 0, max)
	step := float64(len(pts)-1) / float64(max-1)
	for i := 0; i < max; i++ {
		out = append(out, pts[int(float64(i)*step+0.5)])
	}
	return out
}
