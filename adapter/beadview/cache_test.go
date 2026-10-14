package beadview

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// countingExe writes a fake "iugum" that appends each call to a log file
// and prints a fixed bd JSON answer.
func countingExe(t *testing.T) (exe, log string) {
	t.Helper()
	dir := t.TempDir()
	log = filepath.Join(dir, "calls.log")
	exe = filepath.Join(dir, "fake")
	script := "#!/bin/sh\necho \"$@\" >> " + log + "\n" +
		"case \"$4\" in\n" +
		"  list) echo '[{\"id\":\"a-1\",\"title\":\"one\"}]' ;;\n" +
		"  show) echo '[{\"id\":\"a-1\",\"title\":\"one\",\"comments\":[{\"id\":\"c1\",\"text\":\"hi\"}],\"dependents\":[{\"id\":\"a-2\",\"dependency_type\":\"blocks\"}]}]' ;;\n" +
		"  *) echo '[{\"id\":\"a-1\",\"title\":\"one\"}]' ;;\n" +
		"esac\n"
	if err := os.WriteFile(exe, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return exe, log
}

func callCount(t *testing.T, log string) int {
	t.Helper()
	b, err := os.ReadFile(log)
	if os.IsNotExist(err) {
		return 0
	}
	if err != nil {
		t.Fatal(err)
	}
	return len(strings.Split(strings.TrimSpace(string(b)), "\n"))
}

func TestFetchBeadsCachesAndSharesConcurrentReads(t *testing.T) {
	exe, log := countingExe(t)
	f := NewExecFetcher(exe, t.TempDir())
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := f.FetchBeads(context.Background()); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if n := callCount(t, log); n != 1 {
		t.Fatalf("5 concurrent list reads ran %d bd calls, want 1", n)
	}
}

func TestWriteInvalidatesListCache(t *testing.T) {
	exe, log := countingExe(t)
	f := NewExecFetcher(exe, t.TempDir())
	ctx := context.Background()
	_, _ = f.FetchBeads(ctx)
	_, _ = f.FetchBeads(ctx)
	if n := callCount(t, log); n != 1 {
		t.Fatalf("second read should hit the cache, got %d calls", n)
	}
	st := "closed"
	_, _ = f.UpdateBead(ctx, "a-1", UpdateInput{Status: &st})
	_, _ = f.FetchBeads(ctx)
	if n := callCount(t, log); n != 3 {
		t.Fatalf("want list, update, list = 3 calls, got %d", n)
	}
}

func TestFetchBeadReadsCommentsAndDependentsInOneCall(t *testing.T) {
	exe, log := countingExe(t)
	f := NewExecFetcher(exe, t.TempDir())
	b, err := f.FetchBead(context.Background(), "a-1")
	if err != nil {
		t.Fatal(err)
	}
	if n := callCount(t, log); n != 1 {
		t.Fatalf("FetchBead ran %d calls, want 1", n)
	}
	if len(b.Comments) != 1 || len(b.Dependents) != 1 || !b.DetailLoaded {
		t.Fatalf("detail not decoded: %+v", b)
	}
	got, _ := os.ReadFile(log)
	if !strings.Contains(string(got), "--include-comments") || !strings.Contains(string(got), "--include-dependents") {
		t.Fatalf("flags missing: %s", got)
	}
}
