package agentclones

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/srhopkins/iugum/adapter/memory/graphgloss"
	"github.com/srhopkins/iugum/adapter/memory/sqlitemem"
	"github.com/srhopkins/iugum/contract"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"time"
)

type SnapshotReport struct {
	Records       int    `json:"records"`
	Edges         int    `json:"edges"`
	Skipped       int    `json:"skipped"`
	SourceMissing bool   `json:"source_missing"`
	ManifestPath  string `json:"manifest_path"`
}
type memoryManifest struct {
	SourceDB  string                `json:"source_db"`
	FromNS    string                `json:"from_ns"`
	ToNS      string                `json:"to_ns"`
	CreatedAt time.Time             `json:"created_at"`
	Records   []contract.MemoryRec  `json:"records"`
	Edges     []contract.MemoryEdge `json:"edges"`
}
type MemoryChange struct {
	Before contract.MemoryRec `json:"before"`
	After  contract.MemoryRec `json:"after"`
}
type MemoryDiff struct {
	Incomplete   bool                  `json:"incomplete"`
	Added        []contract.MemoryRec  `json:"added"`
	Changed      []MemoryChange        `json:"changed"`
	Deleted      []contract.MemoryRec  `json:"deleted"`
	AddedEdges   []contract.MemoryEdge `json:"added_edges"`
	DeletedEdges []contract.MemoryEdge `json:"deleted_edges"`
}

// SnapshotMemory copies only authorized records bound into fromNS, never the raw
// database. Source absence is an explicit empty snapshot, not an error.
func SnapshotMemory(ctx context.Context, sourceDB, targetDB, fromNS, toNS string, check func(context.Context, string, string) error) (report SnapshotReport, err error) {
	if check == nil {
		return report, errors.New("memory snapshot requires policy checker")
	}
	if fromNS == "" {
		fromNS = "default"
	}
	if toNS == "" {
		return report, errors.New("target namespace required")
	}
	manifestPath := filepath.Join(filepath.Dir(targetDB), "memory-snapshot.json")
	report.ManifestPath = manifestPath
	if _, e := os.Lstat(manifestPath); !os.IsNotExist(e) {
		return report, fmt.Errorf("snapshot manifest destination must not exist")
	}
	recs, edges, missing, skipped, e := readMemory(ctx, sourceDB, fromNS, check)
	if e != nil {
		return report, e
	}
	report.SourceMissing = missing
	report.Skipped = skipped
	for _, rec := range recs {
		if e = check(ctx, contract.MemoryObj(rec.Type, toNS), "write"); e != nil {
			return report, e
		}
	}
	if len(edges) > 0 {
		if e = check(ctx, contract.MemoryObj("graph", toNS), "write"); e != nil {
			return report, e
		}
	}
	f, e := os.OpenFile(targetDB, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if e != nil {
		return report, e
	}
	f.Close()
	defer func() {
		if err != nil {
			os.Remove(targetDB)
			os.Remove(targetDB + "-wal")
			os.Remove(targetDB + "-shm")
		}
	}()
	target, e := sqlitemem.OpenOpts(targetDB, sqlitemem.Opts{Glossary: graphgloss.File{Name: "snapshot"}})
	if e != nil {
		return report, e
	}
	defer target.Close()
	manifest := memoryManifest{SourceDB: sourceDB, FromNS: fromNS, ToNS: toNS, CreatedAt: time.Now().UTC(), Records: []contract.MemoryRec{}, Edges: edges}
	for _, rec := range recs {
		rec.NS = toNS
		if e = target.Remember(ctx, rec); e != nil {
			return report, e
		}
		manifest.Records = append(manifest.Records, rec)
	}
	for _, edge := range edges {
		if e = target.Link(ctx, toNS, edge); e != nil {
			return report, e
		}
	}
	if e = ctx.Err(); e != nil {
		return report, e
	}
	data, e := json.MarshalIndent(manifest, "", "  ")
	if e != nil {
		return report, e
	}
	mf, e := os.OpenFile(manifestPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if e != nil {
		return report, e
	}
	_, e = mf.Write(data)
	ce := mf.Close()
	if e == nil {
		e = ce
	}
	if e != nil {
		os.Remove(manifestPath)
		return report, e
	}
	report.Records = len(recs)
	report.Edges = len(edges)
	return report, nil
}
func readMemory(ctx context.Context, path, ns string, check func(context.Context, string, string) error) (recs []contract.MemoryRec, edges []contract.MemoryEdge, missing bool, skipped int, err error) {
	recs = []contract.MemoryRec{}
	edges = []contract.MemoryEdge{}
	st, e := os.Stat(path)
	if os.IsNotExist(e) {
		return recs, edges, true, 0, nil
	}
	if e != nil {
		return nil, nil, false, 0, e
	}
	if st.Size() == 0 {
		return recs, edges, false, 0, nil
	}
	db, e := sql.Open("sqlite", (&url.URL{Scheme: "file", Path: path, RawQuery: "mode=ro"}).String())
	if e != nil {
		return nil, nil, false, 0, e
	}
	defer db.Close()
	tx, e := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if e != nil {
		return nil, nil, false, 0, e
	}
	defer tx.Rollback()
	rows, e := tx.QueryContext(ctx, `SELECT DISTINCT m.typ FROM memories m JOIN bindings b ON b.memory_id=m.id WHERE b.ns=?`, ns)
	if e != nil {
		return nil, nil, false, 0, e
	}
	var types []string
	for rows.Next() {
		var typ string
		if e = rows.Scan(&typ); e != nil {
			rows.Close()
			return nil, nil, false, 0, e
		}
		types = append(types, typ)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return nil, nil, false, 0, e
	}
	ids := map[string]string{}
	keys := map[string]bool{}
	for _, typ := range types {
		if check(ctx, contract.MemoryObj(typ, ns), "read") != nil {
			skipped++
			continue
		}
		rows, e = tx.QueryContext(ctx, `SELECT m.id,m.key,m.value FROM memories m JOIN bindings b ON b.memory_id=m.id WHERE b.ns=? AND m.typ=? ORDER BY m.id`, ns, typ)
		if e != nil {
			return nil, nil, false, skipped, e
		}
		for rows.Next() {
			var id, key, value string
			if e = rows.Scan(&id, &key, &value); e != nil {
				rows.Close()
				return nil, nil, false, skipped, e
			}
			if keys[key] {
				rows.Close()
				return nil, nil, false, skipped, fmt.Errorf("ambiguous bound memory key %q", key)
			}
			keys[key] = true
			ids[id] = key
			recs = append(recs, contract.MemoryRec{NS: ns, Type: typ, Key: key, Value: value})
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return nil, nil, false, skipped, e
		}
	}
	if check(ctx, contract.MemoryObj("graph", ns), "read") == nil {
		rows, e = tx.QueryContext(ctx, `SELECT e.from_id,e.rel,e.to_id,COALESCE(e.value,'') FROM edges e JOIN bindings f ON f.memory_id=e.from_id JOIN bindings t ON t.memory_id=e.to_id WHERE f.ns=? AND t.ns=? ORDER BY e.id`, ns, ns)
		if e != nil {
			return nil, nil, false, skipped, e
		}
		for rows.Next() {
			var from, rel, to, value string
			if e = rows.Scan(&from, &rel, &to, &value); e != nil {
				rows.Close()
				return nil, nil, false, skipped, e
			}
			fk, fok := ids[from]
			tk, tok := ids[to]
			if fok && tok {
				edges = append(edges, contract.MemoryEdge{From: fk, Rel: rel, To: tk, Value: value})
			}
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return nil, nil, false, skipped, e
		}
	}
	sort.Slice(recs, func(a, b int) bool { return recs[a].Key < recs[b].Key })
	return recs, edges, false, skipped, nil
}

// DiffMemory compares the authorized current clone to its immutable baseline.
// It never writes the parent, clone, or baseline.
func DiffMemory(ctx context.Context, manifestPath, targetDB string, check func(context.Context, string, string) error) (diff MemoryDiff, err error) {
	if check == nil {
		return diff, errors.New("memory diff requires policy checker")
	}
	data, e := os.ReadFile(manifestPath)
	if e != nil {
		return diff, e
	}
	var m memoryManifest
	if e = json.Unmarshal(data, &m); e != nil {
		return diff, e
	}
	if m.ToNS == "" {
		return diff, errors.New("baseline target namespace missing")
	}
	current, edges, _, skipped, e := readMemory(ctx, targetDB, m.ToNS, check)
	if e != nil {
		return diff, e
	}
	diff.Incomplete = skipped > 0
	old := map[string]contract.MemoryRec{}
	now := map[string]contract.MemoryRec{}
	for _, r := range m.Records {
		if check(ctx, contract.MemoryObj(r.Type, m.ToNS), "read") == nil {
			old[r.Key] = r
		}
	}
	for _, r := range current {
		now[r.Key] = r
		if prior, ok := old[r.Key]; !ok {
			diff.Added = append(diff.Added, r)
		} else if prior.Type != r.Type || prior.Value != r.Value {
			diff.Changed = append(diff.Changed, MemoryChange{prior, r})
		}
	}
	for _, r := range m.Records {
		if _, ok := old[r.Key]; ok {
			if _, exists := now[r.Key]; !exists && !diff.Incomplete {
				diff.Deleted = append(diff.Deleted, r)
			}
		}
	}
	if check(ctx, contract.MemoryObj("graph", m.ToNS), "read") == nil {
		before := map[contract.MemoryEdge]bool{}
		after := map[contract.MemoryEdge]bool{}
		for _, edge := range m.Edges {
			if _, ok := old[edge.From]; !ok {
				continue
			}
			if _, ok := old[edge.To]; !ok {
				continue
			}
			before[edge] = true
		}
		for _, edge := range edges {
			after[edge] = true
			if !before[edge] {
				diff.AddedEdges = append(diff.AddedEdges, edge)
			}
		}
		for _, edge := range m.Edges {
			if before[edge] && !after[edge] && !diff.Incomplete {
				diff.DeletedEdges = append(diff.DeletedEdges, edge)
			}
		}
	}
	return diff, nil
}
