package fswatch

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWriteEmitsEvent(t *testing.T) {
	dir := t.TempDir()
	w, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	if err := w.Add(dir); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "n.md")
	if err := os.WriteFile(path, []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	select {
	case ev := <-w.Events():
		if ev.Name != "watch.changed" || ev.Path != path {
			t.Fatalf("%+v", ev)
		}
	case <-time.After(4 * time.Second):
		t.Fatal("no watch event")
	}
}
