package sqlitemem

import (
	"context"
	"database/sql"
	"crypto/sha1"
	"encoding/binary"
	"encoding/hex"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	_ "modernc.org/sqlite"

	"github.com/srhopkins/iugum/adapter/memory/embedhttp"
	"github.com/srhopkins/iugum/adapter/memory/graphgloss"
	"github.com/srhopkins/iugum/contract"
	"github.com/srhopkins/iugum/plugin"
)

func embedderFrom(cfg map[string]string) contract.Embedder {
	kind := cfg["embed_kind"]
	if kind == "" || kind == "off" {
		return nil
	}
	return embedhttp.Client{Kind: kind, URL: cfg["embed_url"], Model: cfg["embed_model"]}
}

func init() {
	plugin.RegisterMemory("sqlite", func(cfg map[string]string) (contract.Memory, error) {
		dir := cfg["data_dir"]
		if dir == "" {
			return nil, os.ErrInvalid
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
		emb := embedderFrom(cfg)
		gloss, _ := graphgloss.Load(cfg["glossary"])
		return OpenOpts(filepath.Join(dir, "memory.db"), Opts{
			Embedder:  emb,
			Glossary:  gloss,
			Extractor: cfg["extractor"],
		})
	})
}

// Store is SQLite memories. One truth row. Namespaces bind to it.
type Store struct {
	db        *sql.DB
	embedder  contract.Embedder
	gloss     graphgloss.File
	extractor string
	mu        sync.Mutex
}

// Opts is optional glossary + embedder for Open.
type Opts struct {
	Embedder  contract.Embedder
	Glossary  graphgloss.File
	Extractor string // off | rules
}

func Open(path string, embedder contract.Embedder) (*Store, error) {
	return OpenOpts(path, Opts{Embedder: embedder, Glossary: graphgloss.Default(), Extractor: "rules"})
}

func OpenOpts(path string, opt Opts) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS namespaces (
			path TEXT PRIMARY KEY
		);
		CREATE TABLE IF NOT EXISTS memories (
			id TEXT PRIMARY KEY,
			typ TEXT NOT NULL,
			key TEXT NOT NULL,
			value TEXT NOT NULL,
			origin_ns TEXT NOT NULL,
			embedding BLOB
		);
		CREATE TABLE IF NOT EXISTS bindings (
			ns TEXT NOT NULL,
			memory_id TEXT NOT NULL,
			bind TEXT NOT NULL,
			PRIMARY KEY (ns, memory_id)
		);
		CREATE TABLE IF NOT EXISTS edges (
			id TEXT PRIMARY KEY,
			from_id TEXT NOT NULL,
			rel TEXT NOT NULL,
			to_id TEXT NOT NULL,
			value TEXT
		);
		INSERT OR IGNORE INTO namespaces(path) VALUES ('default');
	`); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := ensureFTS(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	if opt.Extractor == "" {
		opt.Extractor = "rules"
	}
	if opt.Glossary.Name == "" && len(opt.Glossary.Rels) == 0 {
		opt.Glossary = graphgloss.Default()
	}
	return &Store{db: db, embedder: opt.Embedder, gloss: opt.Glossary, extractor: opt.Extractor}, nil
}

func ensureFTS(db *sql.DB) error {
	if _, err := db.Exec(`
		CREATE VIRTUAL TABLE IF NOT EXISTS memories_fts USING fts5(
			key, value,
			content='memories',
			content_rowid='rowid'
		);
		CREATE TRIGGER IF NOT EXISTS memories_ai AFTER INSERT ON memories BEGIN
			INSERT INTO memories_fts(rowid, key, value) VALUES (new.rowid, new.key, new.value);
		END;
		CREATE TRIGGER IF NOT EXISTS memories_ad AFTER DELETE ON memories BEGIN
			INSERT INTO memories_fts(memories_fts, rowid, key, value) VALUES('delete', old.rowid, old.key, old.value);
		END;
		CREATE TRIGGER IF NOT EXISTS memories_au AFTER UPDATE ON memories BEGIN
			INSERT INTO memories_fts(memories_fts, rowid, key, value) VALUES('delete', old.rowid, old.key, old.value);
			INSERT INTO memories_fts(rowid, key, value) VALUES (new.rowid, new.key, new.value);
		END;
	`); err != nil {
		return err
	}
	_, _ = db.Exec(`INSERT INTO memories_fts(memories_fts) VALUES('rebuild')`)
	return nil
}

func (s *Store) Name() string { return "sqlite" }

func (s *Store) SetEmbedder(e contract.Embedder) { s.embedder = e }

func (s *Store) Close() error { return s.db.Close() }

func normNS(ns string) string {
	if ns == "" {
		return "default"
	}
	return ns
}

func normType(t string) string {
	if t == "" {
		return "fact"
	}
	return t
}

func memID(ns, key string) string { return normNS(ns) + "/" + key }

func (s *Store) ensureNS(ctx context.Context, ns string) error {
	_, err := s.db.ExecContext(ctx, `INSERT OR IGNORE INTO namespaces(path) VALUES (?)`, ns)
	return err
}

func (s *Store) Remember(ctx context.Context, rec contract.MemoryRec) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	ns := normNS(rec.NS)
	typ := normType(rec.Type)
	if err := s.ensureNS(ctx, ns); err != nil {
		return err
	}
	id := memID(ns, rec.Key)
	var blob []byte
	if s.embedder != nil && rec.Value != "" {
		vecs, err := s.embedder.Embed(ctx, []string{rec.Value})
		if err != nil {
			return err
		}
		if len(vecs) > 0 {
			blob = encodeVec(vecs[0])
		}
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO memories(id, typ, key, value, origin_ns, embedding) VALUES(?,?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET value=excluded.value, embedding=excluded.embedding, typ=excluded.typ
	`, id, typ, rec.Key, rec.Value, ns, blob)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT OR IGNORE INTO bindings(ns, memory_id, bind) VALUES(?,?, 'own')
	`, ns, id)
	return err
}

func (s *Store) visibleID(ctx context.Context, ns, key string) (string, error) {
	var id string
	err := s.db.QueryRowContext(ctx, `
		SELECT m.id FROM memories m
		JOIN bindings b ON b.memory_id = m.id
		WHERE b.ns = ? AND m.key = ?
	`, ns, key).Scan(&id)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return id, err
}

func (s *Store) Recall(ctx context.Context, ns, key string) (contract.MemoryRec, bool, error) {
	ns = normNS(ns)
	var rec contract.MemoryRec
	err := s.db.QueryRowContext(ctx, `
		SELECT m.typ, m.key, m.value, m.origin_ns FROM memories m
		JOIN bindings b ON b.memory_id = m.id
		WHERE b.ns = ? AND m.key = ?
	`, ns, key).Scan(&rec.Type, &rec.Key, &rec.Value, &rec.NS)
	if err == sql.ErrNoRows {
		return contract.MemoryRec{}, false, nil
	}
	if err != nil {
		return contract.MemoryRec{}, false, err
	}
	return rec, true, nil
}

func (s *Store) Forget(ctx context.Context, ns, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	ns = normNS(ns)
	id, err := s.visibleID(ctx, ns, key)
	if err != nil || id == "" {
		return err
	}
	_, err = s.db.ExecContext(ctx, `DELETE FROM bindings WHERE ns=? AND memory_id=?`, ns, id)
	if err != nil {
		return err
	}
	var n int
	_ = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM bindings WHERE memory_id=?`, id).Scan(&n)
	if n == 0 {
		_, _ = s.db.ExecContext(ctx, `DELETE FROM edges WHERE from_id=? OR to_id=?`, id, id)
		_, err = s.db.ExecContext(ctx, `DELETE FROM memories WHERE id=?`, id)
	}
	return err
}

func (s *Store) Attach(ctx context.Context, fromNS, toNS, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	fromNS, toNS = normNS(fromNS), normNS(toNS)
	if err := s.ensureNS(ctx, toNS); err != nil {
		return err
	}
	if key != "" {
		id, err := s.visibleID(ctx, fromNS, key)
		if err != nil || id == "" {
			return err
		}
		_, err = s.db.ExecContext(ctx, `INSERT OR IGNORE INTO bindings(ns, memory_id, bind) VALUES(?,?, 'attach')`, toNS, id)
		return err
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT OR IGNORE INTO bindings(ns, memory_id, bind)
		SELECT ?, memory_id, 'attach' FROM bindings WHERE ns=?
	`, toNS, fromNS)
	return err
}

func (s *Store) Detach(ctx context.Context, ns, key string) error {
	return s.Forget(ctx, ns, key)
}

func (s *Store) Slice(ctx context.Context, fromNS, toNS, filter string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	fromNS, toNS = normNS(fromNS), normNS(toNS)
	if err := s.ensureNS(ctx, toNS); err != nil {
		return err
	}
	filter = strings.ToLower(strings.TrimSpace(filter))
	rows, err := s.db.QueryContext(ctx, `
		SELECT m.id, m.key, m.value FROM memories m
		JOIN bindings b ON b.memory_id = m.id
		WHERE b.ns = ?
	`, fromNS)
	if err != nil {
		return err
	}
	var ids []string
	for rows.Next() {
		var id, key, value string
		if err := rows.Scan(&id, &key, &value); err != nil {
			rows.Close()
			return err
		}
		if filter != "" && !strings.Contains(strings.ToLower(key), filter) && !strings.Contains(strings.ToLower(value), filter) {
			continue
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, id := range ids {
		if _, err := s.db.ExecContext(ctx, `INSERT OR IGNORE INTO bindings(ns, memory_id, bind) VALUES(?,?, 'slice')`, toNS, id); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) Search(ctx context.Context, q contract.MemoryQuery) ([]contract.MemoryHit, error) {
	limit := q.Limit
	if limit <= 0 {
		limit = 50
	}
	nss := q.NS
	if len(nss) == 0 {
		nss = []string{"default"}
	}
	for i := range nss {
		nss[i] = normNS(nss[i])
	}

	args := make([]any, 0, len(nss)+1)
	ph := make([]string, len(nss))
	for i, ns := range nss {
		ph[i] = "?"
		args = append(args, ns)
	}
	sqlStr := `
		SELECT b.ns, m.typ, m.key, m.value, m.embedding FROM memories m
		JOIN bindings b ON b.memory_id = m.id
		WHERE b.ns IN (` + strings.Join(ph, ",") + `)`
	if q.Type != "" {
		sqlStr += ` AND m.typ = ?`
		args = append(args, q.Type)
	}
	rows, err := s.db.QueryContext(ctx, sqlStr, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type row struct {
		ns, typ, key, value string
		vec                 []float32
	}
	var all []row
	for rows.Next() {
		var r row
		var blob []byte
		if err := rows.Scan(&r.ns, &r.typ, &r.key, &r.value, &blob); err != nil {
			return nil, err
		}
		r.vec = decodeVec(blob)
		all = append(all, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	needle := strings.ToLower(strings.TrimSpace(q.Text))
	ftsHits := map[string]float64{}
	if mq := ftsQuery(q.Text); mq != "" {
		frows, ferr := s.db.QueryContext(ctx, `
			SELECT m.key, rank FROM memories_fts
			JOIN memories m ON m.rowid = memories_fts.rowid
			WHERE memories_fts MATCH ? ORDER BY rank LIMIT ?
		`, mq, limit)
		if ferr == nil {
			defer frows.Close()
			for frows.Next() {
				var key string
				var rank float64
				if err := frows.Scan(&key, &rank); err != nil {
					return nil, err
				}
				ftsHits[key] = 0.55 + 0.45/(1+math.Abs(rank))
			}
		}
	}
	var qvec []float32
	if s.embedder != nil && needle != "" {
		vecs, err := s.embedder.Embed(ctx, []string{q.Text})
		if err != nil {
			return nil, err
		}
		if len(vecs) > 0 {
			qvec = vecs[0]
		}
	}

	var hits []contract.MemoryHit
	seen := map[string]bool{}
	for _, r := range all {
		id := r.ns + "/" + r.key
		if seen[id] {
			continue
		}
		seen[id] = true
		score := 0.0
		if needle != "" && (strings.Contains(strings.ToLower(r.key), needle) || strings.Contains(strings.ToLower(r.value), needle)) {
			score = 0.5
		}
		if fs, ok := ftsHits[r.key]; ok && fs > score {
			score = fs
		}
		if len(qvec) > 0 && len(r.vec) == len(qvec) {
			if c := cosine(qvec, r.vec); c > score {
				score = c
			}
		}
		if needle == "" {
			score = 1
		}
		if needle != "" && score == 0 {
			continue
		}
		hits = append(hits, contract.MemoryHit{NS: r.ns, Type: r.typ, Key: r.key, Value: r.value, Score: score})
	}
	for i := 0; i < len(hits); i++ {
		for j := i + 1; j < len(hits); j++ {
			if hits[j].Score > hits[i].Score || (hits[j].Score == hits[i].Score && hits[j].Key < hits[i].Key) {
				hits[i], hits[j] = hits[j], hits[i]
			}
		}
	}
	if len(hits) > limit {
		hits = hits[:limit]
	}
	return hits, nil
}

func (s *Store) Link(ctx context.Context, ns string, e contract.MemoryEdge) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.linkLocked(ctx, ns, e)
}

func (s *Store) linkLocked(ctx context.Context, ns string, e contract.MemoryEdge) error {
	ns = normNS(ns)
	if e.From == "" || e.To == "" || e.Rel == "" {
		return nil
	}
	rel := e.Rel
	if s.glossHasRels() {
		c, ok := s.gloss.CanonRel(rel)
		if !ok {
			return nil
		}
		rel = c
	}
	if err := s.ensureNode(ctx, ns, e.From, ""); err != nil {
		return err
	}
	if err := s.ensureNode(ctx, ns, e.To, ""); err != nil {
		return err
	}
	fromID := memID(ns, e.From)
	toID := memID(ns, e.To)
	// nodes may have been created in another origin ns; resolve visible ids
	if id, err := s.visibleID(ctx, ns, e.From); err == nil && id != "" {
		fromID = id
	}
	if id, err := s.visibleID(ctx, ns, e.To); err == nil && id != "" {
		toID = id
	}
	eid := fromID + "|" + rel + "|" + toID
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO edges(id, from_id, rel, to_id, value) VALUES(?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET value=excluded.value
	`, eid, fromID, rel, toID, e.Value)
	return err
}

func (s *Store) Unlink(ctx context.Context, ns, from, rel, to string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	ns = normNS(ns)
	fromID, err := s.visibleID(ctx, ns, from)
	if err != nil || fromID == "" {
		return err
	}
	toID, err := s.visibleID(ctx, ns, to)
	if err != nil || toID == "" {
		return err
	}
	_, err = s.db.ExecContext(ctx, `DELETE FROM edges WHERE from_id=? AND rel=? AND to_id=?`, fromID, rel, toID)
	return err
}

func (s *Store) Walk(ctx context.Context, q contract.WalkQuery) ([]contract.WalkHit, error) {
	ns := normNS(q.NS)
	hops := q.Hops
	if hops <= 0 {
		hops = 2
	}
	fromID, err := s.visibleID(ctx, ns, q.From)
	if err != nil || fromID == "" {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `
		WITH RECURSIVE walk(from_id, rel, to_id, hops) AS (
			SELECT e.from_id, e.rel, e.to_id, 1
			FROM edges e
			JOIN bindings bf ON bf.memory_id = e.from_id AND bf.ns = ?
			JOIN bindings bt ON bt.memory_id = e.to_id AND bt.ns = ?
			WHERE e.from_id = ? AND (? = '' OR e.rel = ?)
			UNION ALL
			SELECT e.from_id, e.rel, e.to_id, w.hops + 1
			FROM walk w
			JOIN edges e ON e.from_id = w.to_id
			JOIN bindings bf ON bf.memory_id = e.from_id AND bf.ns = ?
			JOIN bindings bt ON bt.memory_id = e.to_id AND bt.ns = ?
			WHERE w.hops < ? AND (? = '' OR e.rel = ?)
		)
		SELECT mf.key, w.rel, mt.key, w.hops
		FROM walk w
		JOIN memories mf ON mf.id = w.from_id
		JOIN memories mt ON mt.id = w.to_id
	`, ns, ns, fromID, q.Rel, q.Rel, ns, ns, hops, q.Rel, q.Rel)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []contract.WalkHit
	for rows.Next() {
		var h contract.WalkHit
		if err := rows.Scan(&h.From, &h.Rel, &h.To, &h.Hops); err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

func (s *Store) Ingest(ctx context.Context, ns, text string) ([]contract.MemoryEdge, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ns = normNS(ns)
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, nil
	}
	key := "ingest-" + shortHash(text)
	if err := s.rememberLocked(ctx, contract.MemoryRec{NS: ns, Type: "fact", Key: key, Value: text}); err != nil {
		return nil, err
	}
	if s.extractor == "off" {
		return nil, nil
	}
	edges := s.gloss.Extract(text)
	for _, e := range edges {
		if err := s.linkLocked(ctx, ns, e); err != nil {
			return nil, err
		}
	}
	return edges, nil
}

func (s *Store) rememberLocked(ctx context.Context, rec contract.MemoryRec) error {
	ns := normNS(rec.NS)
	typ := normType(rec.Type)
	if err := s.ensureNS(ctx, ns); err != nil {
		return err
	}
	id := memID(ns, rec.Key)
	var blob []byte
	if s.embedder != nil && rec.Value != "" {
		vecs, err := s.embedder.Embed(ctx, []string{rec.Value})
		if err != nil {
			return err
		}
		if len(vecs) > 0 {
			blob = encodeVec(vecs[0])
		}
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO memories(id, typ, key, value, origin_ns, embedding) VALUES(?,?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET value=excluded.value, embedding=excluded.embedding, typ=excluded.typ
	`, id, typ, rec.Key, rec.Value, ns, blob)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT OR IGNORE INTO bindings(ns, memory_id, bind) VALUES(?,?, 'own')
	`, ns, id)
	return err
}

func (s *Store) ensureNode(ctx context.Context, ns, key, value string) error {
	if id, err := s.visibleID(ctx, ns, key); err != nil {
		return err
	} else if id != "" {
		return nil
	}
	return s.rememberLocked(ctx, contract.MemoryRec{NS: ns, Type: "graph", Key: key, Value: value})
}

func (s *Store) glossHasRels() bool {
	return len(s.gloss.Rels) > 0
}

var ftsTok = regexp.MustCompile(`[A-Za-z0-9_]+`)

func ftsQuery(text string) string {
	words := ftsTok.FindAllString(text, -1)
	if len(words) == 0 {
		return ""
	}
	for i, w := range words {
		words[i] = `"` + w + `"`
	}
	return strings.Join(words, " AND ")
}

func shortHash(s string) string {
	h := sha1.Sum([]byte(s))
	return hex.EncodeToString(h[:6])
}

func encodeVec(v []float32) []byte {
	b := make([]byte, 4*len(v))
	for i, x := range v {
		binary.LittleEndian.PutUint32(b[i*4:], math.Float32bits(x))
	}
	return b
}

func decodeVec(b []byte) []float32 {
	if len(b) < 4 || len(b)%4 != 0 {
		return nil
	}
	v := make([]float32, len(b)/4)
	for i := range v {
		v[i] = math.Float32frombits(binary.LittleEndian.Uint32(b[i*4:]))
	}
	return v
}

func cosine(a, b []float32) float64 {
	if len(a) == 0 || len(a) != len(b) {
		return 0
	}
	var dot, na, nb float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		na += float64(a[i]) * float64(a[i])
		nb += float64(b[i]) * float64(b[i])
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return dot / (math.Sqrt(na) * math.Sqrt(nb))
}
