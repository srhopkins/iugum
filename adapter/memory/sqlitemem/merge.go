package sqlitemem

import (
	"context"
	"database/sql"
	"strings"
)

// CosineMergeThreshold merges graph nodes when embedding similarity is at least this.
const CosineMergeThreshold = 0.85

// slugMergeTarget returns the shortest existing key that is a slug prefix of key
// (e.g. steve-hopkins → steve when steve exists). Empty means no slug alias match.
func slugMergeTarget(key string, existing []string) string {
	var best string
	for _, e := range existing {
		if key == e {
			continue
		}
		if strings.HasPrefix(key, e+"-") {
			if best == "" || len(e) < len(best) {
				best = e
			}
		}
	}
	return best
}

func pickCanonicalKey(a, b string) string {
	if len(a) != len(b) {
		if len(a) < len(b) {
			return a
		}
		return b
	}
	if a < b {
		return a
	}
	return b
}

func resolveMergeTarget(key, target string, merges map[string]string) string {
	for {
		next, ok := merges[target]
		if !ok || next == target {
			break
		}
		target = next
	}
	return target
}

func (s *Store) graphNodeKeys(ctx context.Context, ns string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT DISTINCT m.key FROM memories m
		JOIN bindings b ON b.memory_id = m.id
		WHERE b.ns = ? AND m.typ = 'graph'
		ORDER BY m.key
	`, ns)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var keys []string
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			return nil, err
		}
		keys = append(keys, k)
	}
	return keys, rows.Err()
}

func (s *Store) nodeVec(ctx context.Context, ns, key string) ([]float32, error) {
	var blob []byte
	err := s.db.QueryRowContext(ctx, `
		SELECT m.embedding FROM memories m
		JOIN bindings b ON b.memory_id = m.id
		WHERE b.ns = ? AND m.key = ? AND m.typ = 'graph'
	`, ns, key).Scan(&blob)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if v := decodeVec(blob); len(v) > 0 {
		return v, nil
	}
	if s.embedder == nil {
		return nil, nil
	}
	vecs, err := s.embedder.Embed(ctx, []string{key})
	if err != nil {
		return nil, err
	}
	if len(vecs) == 0 {
		return nil, nil
	}
	return vecs[0], nil
}

func (s *Store) mergeGraphNodesLocked(ctx context.Context, ns string) error {
	keys, err := s.graphNodeKeys(ctx, ns)
	if err != nil || len(keys) < 2 {
		return err
	}

	merges := map[string]string{}
	for _, k := range keys {
		if target := slugMergeTarget(k, keys); target != "" {
			merges[k] = target
		}
	}

	if s.embedder != nil {
		vecs := make(map[string][]float32, len(keys))
		for _, k := range keys {
			v, err := s.nodeVec(ctx, ns, k)
			if err != nil {
				return err
			}
			if len(v) > 0 {
				vecs[k] = v
			}
		}
		for i := 0; i < len(keys); i++ {
			for j := i + 1; j < len(keys); j++ {
				a, b := keys[i], keys[j]
				va, vb := vecs[a], vecs[b]
				if len(va) == 0 || len(vb) == 0 {
					continue
				}
				if cosine(va, vb) < CosineMergeThreshold {
					continue
				}
				if slugMergeTarget(a, []string{b}) != "" || slugMergeTarget(b, []string{a}) != "" {
					continue
				}
				canonical := pickCanonicalKey(a, b)
				dup := b
				if canonical == b {
					dup = a
				}
				if existing, ok := merges[dup]; ok && existing == canonical {
					continue
				}
				if _, ok := merges[canonical]; !ok {
					merges[dup] = canonical
				}
			}
		}
	}

	if len(merges) == 0 {
		return nil
	}

	// Merge longer / alias keys before their canonical targets when chained.
	for dup, target := range merges {
		target = resolveMergeTarget(dup, target, merges)
		if dup == target {
			continue
		}
		if err := s.mergeNodeLocked(ctx, ns, dup, target); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) mergeNodeLocked(ctx context.Context, ns, fromKey, toKey string) error {
	if fromKey == "" || toKey == "" || fromKey == toKey {
		return nil
	}
	fromID, err := s.visibleID(ctx, ns, fromKey)
	if err != nil || fromID == "" {
		return err
	}
	toID, err := s.visibleID(ctx, ns, toKey)
	if err != nil || toID == "" {
		return err
	}
	if fromID == toID {
		return nil
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT id, from_id, rel, to_id, value FROM edges
		WHERE from_id = ? OR to_id = ?
	`, fromID, fromID)
	if err != nil {
		return err
	}
	type edgeRow struct {
		id, fromID, rel, toID, value string
	}
	var edges []edgeRow
	for rows.Next() {
		var e edgeRow
		if err := rows.Scan(&e.id, &e.fromID, &e.rel, &e.toID, &e.value); err != nil {
			rows.Close()
			return err
		}
		edges = append(edges, e)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()

	for _, e := range edges {
		newFrom, newTo := e.fromID, e.toID
		if newFrom == fromID {
			newFrom = toID
		}
		if newTo == fromID {
			newTo = toID
		}
		if newFrom == newTo {
			if _, err := s.db.ExecContext(ctx, `DELETE FROM edges WHERE id = ?`, e.id); err != nil {
				return err
			}
			continue
		}
		newID := newFrom + "|" + e.rel + "|" + newTo
		if _, err := s.db.ExecContext(ctx, `DELETE FROM edges WHERE id = ?`, e.id); err != nil {
			return err
		}
		_, err := s.db.ExecContext(ctx, `
			INSERT INTO edges(id, from_id, rel, to_id, value) VALUES(?,?,?,?,?)
			ON CONFLICT(id) DO UPDATE SET value=excluded.value
		`, newID, newFrom, e.rel, newTo, e.value)
		if err != nil {
			return err
		}
	}

	if _, err := s.db.ExecContext(ctx, `
		INSERT OR IGNORE INTO bindings(ns, memory_id, bind)
		SELECT ns, ?, bind FROM bindings WHERE memory_id = ?
	`, toID, fromID); err != nil {
		return err
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM bindings WHERE memory_id = ?`, fromID); err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `DELETE FROM memories WHERE id = ?`, fromID)
	return err
}
