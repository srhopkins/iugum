//go:build linux || darwin || freebsd || netbsd || openbsd || windows

package sqlitemem

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"

	_ "modernc.org/sqlite/vec"
)

const vecTable = "memories_vec"

// vecIndex is an optional sqlite-vec vec0 KNN index. Default off.
type vecIndex struct {
	enabled bool
	ready   bool
	dim     int
}

func (v *vecIndex) configure(want bool, hasEmbedder bool) {
	v.enabled = want && hasEmbedder
}

func (v *vecIndex) active() bool {
	return v.enabled && v.ready
}

func (v *vecIndex) probe(db *sql.DB) error {
	if !v.enabled {
		return nil
	}
	if _, err := db.Exec(`CREATE VIRTUAL TABLE IF NOT EXISTS _vec_probe USING vec0(embedding float[1])`); err != nil {
		return err
	}
	_, _ = db.Exec(`DROP TABLE IF EXISTS _vec_probe`)
	v.ready = true
	return nil
}

func (v *vecIndex) ensureDim(db *sql.DB, dim int) error {
	if !v.enabled || dim <= 0 {
		return nil
	}
	if !v.ready {
		if err := v.probe(db); err != nil {
			return err
		}
	}
	if v.dim == dim {
		return nil
	}
	if v.dim > 0 && v.dim != dim {
		if _, err := db.Exec(`DROP TABLE IF EXISTS ` + vecTable); err != nil {
			return err
		}
	}
	ddl := fmt.Sprintf(`CREATE VIRTUAL TABLE IF NOT EXISTS %s USING vec0(embedding float[%d])`, vecTable, dim)
	if _, err := db.Exec(ddl); err != nil {
		return err
	}
	v.dim = dim
	return nil
}

func (v *vecIndex) upsertBlob(db *sql.DB, memoryID string, blob []byte) error {
	if !v.enabled || len(blob) < 4 {
		return nil
	}
	vec := decodeVec(blob)
	if len(vec) == 0 {
		return nil
	}
	if err := v.ensureDim(db, len(vec)); err != nil {
		v.ready = false
		return err
	}
	var rowid int64
	if err := db.QueryRow(`SELECT rowid FROM memories WHERE id=?`, memoryID).Scan(&rowid); err != nil {
		return err
	}
	_, err := db.Exec(`INSERT OR REPLACE INTO `+vecTable+`(rowid, embedding) VALUES(?, ?)`, rowid, vecJSON(vec))
	return err
}

func (v *vecIndex) deleteMemory(db *sql.DB, memoryID string) {
	if !v.active() {
		return
	}
	var rowid int64
	if err := db.QueryRow(`SELECT rowid FROM memories WHERE id=?`, memoryID).Scan(&rowid); err != nil {
		return
	}
	_, _ = db.Exec(`DELETE FROM `+vecTable+` WHERE rowid=?`, rowid)
}

// vecScores returns memory keys ranked by vec0 KNN. Replaces in-Go cosine when active.
func (v *vecIndex) vecScores(ctx context.Context, db *sql.DB, qvec []float32, nss []string, typ string, limit int) (map[string]float64, error) {
	out := map[string]float64{}
	if !v.active() || len(qvec) == 0 || limit <= 0 {
		return out, nil
	}
	if v.dim == 0 || len(qvec) != v.dim {
		if err := v.ensureDim(db, len(qvec)); err != nil {
			return nil, err
		}
	}
	ph := make([]string, len(nss))
	args := make([]any, 0, len(nss)+3)
	args = append(args, vecJSON(qvec), limit)
	for i, ns := range nss {
		ph[i] = "?"
		args = append(args, ns)
	}
	sqlStr := `
		SELECT m.key, v.distance FROM ` + vecTable + ` v
		JOIN memories m ON m.rowid = v.rowid
		JOIN bindings b ON b.memory_id = m.id
		WHERE v.embedding MATCH ? AND k = ? AND b.ns IN (` + strings.Join(ph, ",") + `)`
	if typ != "" {
		sqlStr += ` AND m.typ = ?`
		args = append(args, typ)
	}
	sqlStr += ` ORDER BY v.distance`

	rows, err := db.QueryContext(ctx, sqlStr, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var key string
		var dist float64
		if err := rows.Scan(&key, &dist); err != nil {
			return nil, err
		}
		out[key] = distanceToScore(dist)
	}
	return out, rows.Err()
}

func vecJSON(v []float32) string {
	var b strings.Builder
	b.WriteByte('[')
	for i, x := range v {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(strconv.FormatFloat(float64(x), 'f', -1, 32))
	}
	b.WriteByte(']')
	return b.String()
}

func distanceToScore(dist float64) float64 {
	if dist < 0 {
		return 1
	}
	return 1 / (1 + dist)
}
