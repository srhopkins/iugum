package agentwork

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
)

type savedSource struct {
	Path    string       `json:"path"`
	Records []Record     `json:"records"`
	Status  SourceStatus `json:"status"`
}

// Save persists only currently authorized configured repositories atomically.
func (c *Cache) Save(ctx context.Context, path string) error {
	saved := map[string]savedSource{}
	for _, name := range c.names() {
		s := c.sources[name]
		if c.authorized(ctx, s) != nil {
			continue
		}
		c.mu.RLock()
		saved[name] = savedSource{s.path, append([]Record{}, s.records...), s.status}
		c.mu.RUnlock()
	}
	data, e := json.Marshal(saved)
	if e != nil {
		return e
	}
	if e = os.MkdirAll(filepath.Dir(path), 0700); e != nil {
		return e
	}
	f, e := os.CreateTemp(filepath.Dir(path), ".work-cache-*")
	if e != nil {
		return e
	}
	defer os.Remove(f.Name())
	if _, e = f.Write(data); e != nil {
		f.Close()
		return e
	}
	if e = f.Close(); e != nil {
		return e
	}
	return os.Rename(f.Name(), path)
}

// Load never trusts a cached repository name alone: configured canonical paths
// must match, and current policy must permit the source. No fetch is triggered.
func (c *Cache) Load(ctx context.Context, path string) error {
	data, e := os.ReadFile(path)
	if os.IsNotExist(e) {
		return nil
	}
	if e != nil {
		return e
	}
	var saved map[string]savedSource
	if e = json.Unmarshal(data, &saved); e != nil {
		return e
	}
	for name, v := range saved {
		s, ok := c.sources[name]
		if !ok || s.path != v.Path || c.authorized(ctx, s) != nil {
			continue
		}
		for n := range v.Records {
			v.Records[n].Repo = name
		}
		v.Status.Repo = name
		c.mu.Lock()
		s.records = v.Records
		s.status = v.Status
		c.mu.Unlock()
	}
	return nil
}
