package agentwork

import (
	"context"
	"errors"
	"fmt"
	"github.com/srhopkins/iugum/adapter/beadview"
	"path/filepath"
	"strings"
	"testing"
)

type fakeFetcher struct {
	calls int
	err   error
	beads []beadview.Bead
}

func (f *fakeFetcher) FetchBeads(context.Context) ([]beadview.Bead, error) {
	f.calls++
	return f.beads, f.err
}
func TestCacheStalenessAndRevocation(t *testing.T) {
	denied := false
	c := New("unused", map[string]string{"ffai": t.TempDir()}, func(_ context.Context, obj, act string) error {
		if denied {
			return errors.New("revoked")
		}
		return nil
	})
	f := &fakeFetcher{beads: []beadview.Bead{{ID: "one", Title: "Deliver tickets", Status: "in_progress", Priority: "1", Description: "Acceptance criteria", Dependencies: []beadview.Dependency{{DependsOnID: "two", Type: "blocks"}}}}}
	c.sources["ffai"].fetch = f
	s := c.Refresh(context.Background())
	if len(s) != 1 || s[0].CheckedAt.IsZero() {
		t.Fatal(s)
	}
	r := c.Search(context.Background(), "tickets", "ffai", nil)
	if len(r) != 1 || f.calls != 1 {
		t.Fatal(r, f.calls)
	}
	r[0].Dependencies[0].DependsOnID = "mutated"
	if c.Search(context.Background(), "", "", nil)[0].Dependencies[0].DependsOnID != "two" {
		t.Fatal("cache data leaked mutable slice")
	}
	f.err = errors.New("offline")
	s = c.Refresh(context.Background())
	if s[0].Error != "offline" || !s[0].CheckedAt.Equal(r[0].CheckedAt) {
		t.Fatal(s)
	}
	if len(c.Search(context.Background(), "tickets", "", nil)) != 1 {
		t.Fatal("stale evidence lost")
	}
	denied = true
	c.Refresh(context.Background())
	if f.calls != 2 || len(c.Search(context.Background(), "", "", nil)) != 0 || len(c.Status(context.Background())) != 0 {
		t.Fatal("revoked source leaked")
	}
}
func TestCacheScopeAndLimit(t *testing.T) {
	var checks []string
	c := New("unused", map[string]string{"alpha": t.TempDir()}, func(_ context.Context, o, a string) error { checks = append(checks, o+"/"+a); return nil })
	f := &fakeFetcher{}
	for n := 0; n < 20; n++ {
		f.beads = append(f.beads, beadview.Bead{ID: fmt.Sprint(n), Title: "work", Status: "open", Priority: "2"})
	}
	f.beads = append(f.beads, beadview.Bead{ID: "done", Title: "finished", Status: "closed"})
	c.sources["alpha"].fetch = f
	c.Refresh(context.Background())
	if len(c.Search(context.Background(), "", "alpha", nil)) != 10 {
		t.Fatal("limit")
	}
	if len(c.Search(context.Background(), "", "other", nil)) != 0 || len(c.Search(context.Background(), "", "", []string{"alpha"})) != 0 {
		t.Fatal("scope or exclusion")
	}
	if len(c.Search(context.Background(), "finished", "", nil)) != 1 {
		t.Fatal("explicit search should include closed")
	}
	if !strings.Contains(strings.Join(checks, " "), "tracker/read") {
		t.Fatal(checks)
	}
}

func TestPersistedCacheRestoresWithoutFetch(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	allow := func(context.Context, string, string) error { return nil }
	c := New("unused", map[string]string{"repo": dir}, allow)
	f := &fakeFetcher{beads: []beadview.Bead{{ID: "one", Title: "cached", Status: "open"}}}
	c.sources["repo"].fetch = f
	c.Refresh(ctx)
	path := filepath.Join(t.TempDir(), "cache.json")
	if e := c.Save(ctx, path); e != nil {
		t.Fatal(e)
	}
	next := New("unused", map[string]string{"repo": dir}, allow)
	if e := next.Load(ctx, path); e != nil {
		t.Fatal(e)
	}
	if len(next.Search(ctx, "", "", nil)) != 1 {
		t.Fatal("not restored")
	}
	different := New("unused", map[string]string{"repo": t.TempDir()}, allow)
	different.Load(ctx, path)
	if len(different.Search(ctx, "", "", nil)) != 0 {
		t.Fatal("reused cache for different repository")
	}
}
