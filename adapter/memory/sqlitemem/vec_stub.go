//go:build !(linux || darwin || freebsd || netbsd || openbsd || windows)

package sqlitemem

import (
	"context"
	"database/sql"
	"errors"
)

var errVecUnsupported = errors.New("sqlite-vec unsupported on this platform")

type vecIndex struct {
	enabled bool
}

func (v *vecIndex) configure(want bool, hasEmbedder bool) {
	v.enabled = want && hasEmbedder
}

func (v *vecIndex) active() bool { return false }

func (v *vecIndex) probe(*sql.DB) error { return errVecUnsupported }

func (v *vecIndex) upsertBlob(*sql.DB, string, []byte) error { return nil }

func (v *vecIndex) deleteMemory(*sql.DB, string) {}

func (v *vecIndex) vecScores(context.Context, *sql.DB, []float32, []string, string, int) (map[string]float64, error) {
	return nil, errVecUnsupported
}
