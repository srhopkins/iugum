package main

import (
	"fmt"
	"github.com/srhopkins/iugum/agentsessions"
	"path/filepath"
	"strings"
	"time"
)

func formatLatestSession(h agentsessions.Hit) string {
	stamp := h.Timestamp
	if t, err := time.Parse(time.RFC3339Nano, stamp); err == nil {
		stamp = t.Local().Format("Mon Jan 2, 3:04 PM MST")
	}
	// A bounded extract is explicit evidence, not an invented completion verdict.

	return fmt.Sprintf("Latest session: %s — %s (%s).\nLast recorded message: %s\nSession: %s\nSource: %s", h.Platform, filepath.Base(h.Project), stamp, sessionExcerpt(h.Text, 3000), h.Session, h.Path)
}

func sessionExcerpt(text string, limit int) string {
	excerpt := []rune(strings.TrimSpace(text))
	if len(excerpt) > limit {
		excerpt = append(excerpt[:limit], '…')
	}
	return string(excerpt)
}
