// Package agentwork maintains a read-only, policy-checked snapshot of Beads work.
package agentwork

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/srhopkins/iugum/adapter/beadview"
)

type Record struct {
	Repo         string                `json:"repo"`
	ID           string                `json:"id"`
	Title        string                `json:"title"`
	Status       string                `json:"status"`
	Priority     json.Number           `json:"priority"`
	Description  string                `json:"description"`
	Dependencies []beadview.Dependency `json:"dependencies"`
	CheckedAt    time.Time             `json:"checked_at"`
}
type SourceStatus struct {
	Repo        string    `json:"repo"`
	CheckedAt   time.Time `json:"checked_at"`
	AttemptedAt time.Time `json:"attempted_at"`
	Error       string    `json:"error,omitempty"`
	Count       int       `json:"count"`
}
type fetcher interface {
	FetchBeads(context.Context) ([]beadview.Bead, error)
}
type source struct {
	path    string
	fetch   fetcher
	records []Record
	status  SourceStatus
}
type Cache struct {
	mu      sync.RWMutex
	refresh sync.Mutex
	sources map[string]*source
	check   func(context.Context, string, string) error
}

func New(exe string, repos map[string]string, check func(context.Context, string, string) error) *Cache {
	c := &Cache{sources: map[string]*source{}, check: check}
	for name, path := range repos {
		if absolute, err := filepath.Abs(path); err == nil {
			path = absolute
		}
		if canonical, err := filepath.EvalSymlinks(path); err == nil {
			path = canonical
		}
		c.sources[name] = &source{path: path, fetch: beadview.NewExecFetcher(exe, path), status: SourceStatus{Repo: name}}
	}
	return c
}
func (c *Cache) authorized(ctx context.Context, s *source) error {
	if c.check == nil {
		return errors.New("work cache requires a policy checker")
	}
	if err := c.check(ctx, "project:"+s.path, "read"); err != nil {
		return err
	}
	return c.check(ctx, "tracker", "read")
}

// Refresh serializes updates while allowing cached reads during subprocess work.
// Failed sources retain their previous checked_at and records; Error marks staleness.
func (c *Cache) Refresh(ctx context.Context) []SourceStatus {
	c.refresh.Lock()
	defer c.refresh.Unlock()
	names := c.names()
	for _, name := range names {
		if ctx.Err() != nil {
			break
		}
		s := c.sources[name]
		attempt := time.Now().UTC()
		err := c.authorized(ctx, s)
		var beads []beadview.Bead
		if err == nil {
			sourceCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			beads, err = s.fetch.FetchBeads(sourceCtx)
			cancel()
		}
		c.mu.Lock()
		s.status.AttemptedAt = attempt
		if err != nil {
			s.status.Error = err.Error()
		} else {
			checked := time.Now().UTC()
			records := make([]Record, 0, len(beads))
			for _, b := range beads {
				records = append(records, Record{name, b.ID, b.Title, b.Status, b.Priority, b.Description, append([]beadview.Dependency{}, b.Dependencies...), checked})
			}
			s.records = records
			s.status.CheckedAt = checked
			s.status.Error = ""
			s.status.Count = len(records)
		}
		c.mu.Unlock()
	}
	return c.Status(ctx)
}
func (c *Cache) names() []string {
	names := make([]string, 0, len(c.sources))
	for n := range c.sources {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}
func (c *Cache) Status(ctx context.Context) []SourceStatus {
	result := []SourceStatus{}
	for _, n := range c.names() {
		s := c.sources[n]
		if c.authorized(ctx, s) != nil {
			continue
		}
		c.mu.RLock()
		result = append(result, s.status)
		c.mu.RUnlock()
	}
	return result
}

// Search is immediate and never fetches. Empty text returns unfinished work.
func (c *Cache) Search(ctx context.Context, text, scope string, exclude []string) []Record {
	records := []Record{}
	terms := strings.Fields(strings.ToLower(text))
	scope = strings.ToLower(scope)
	for _, n := range c.names() {
		s := c.sources[n]
		if c.authorized(ctx, s) != nil {
			continue
		}
		if scope != "" && !strings.Contains(strings.ToLower(n+" "+s.path), scope) {
			continue
		}
		c.mu.RLock()
		for _, r := range s.records {
			if len(terms) == 0 && (r.Status == "closed" || r.Status == "tombstone") {
				continue
			}
			hay := strings.ToLower(n + " " + s.path + " " + r.ID + " " + r.Title + " " + r.Description + " " + r.Status)
			ok := true
			for _, term := range terms {
				if !strings.Contains(hay, term) {
					ok = false
					break
				}
			}
			for _, term := range exclude {
				if term != "" && strings.Contains(hay, strings.ToLower(term)) {
					ok = false
					break
				}
			}
			if ok {
				r.Dependencies = append([]beadview.Dependency{}, r.Dependencies...)
				records = append(records, r)
			}
		}
		c.mu.RUnlock()
	}
	score := func(status string) int {
		switch status {
		case "in_progress":
			return 0
		case "blocked":
			return 1
		case "open":
			return 2
		default:
			return 3
		}
	}
	sort.SliceStable(records, func(a, b int) bool {
		x, y := records[a], records[b]
		if score(x.Status) != score(y.Status) {
			return score(x.Status) < score(y.Status)
		}
		px, ex := x.Priority.Int64()
		py, ey := y.Priority.Int64()
		if ex != nil {
			px = 99
		}
		if ey != nil {
			py = 99
		}
		if px != py {
			return px < py
		}
		if x.Repo != y.Repo {
			return x.Repo < y.Repo
		}
		return x.ID < y.ID
	})
	if len(records) > 10 {
		records = records[:10]
	}
	return records
}
