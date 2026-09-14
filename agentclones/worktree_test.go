package agentclones

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestCandidateIsolationAndPromotion(t *testing.T) {
	ctx := context.Background()
	parent := t.TempDir()
	_, e := git(ctx, parent, "init")
	if e != nil {
		t.Fatal(e)
	}
	git(ctx, parent, "config", "core.hooksPath", "/dev/null")
	for p, v := range map[string]string{"go.mod": "module fixture\n\ngo 1.23\n", "x.go": "package fixture\nconst X=1\n"} {
		if e = os.WriteFile(filepath.Join(parent, p), []byte(v), 0644); e != nil {
			t.Fatal(e)
		}
	}
	if _, e = git(ctx, parent, "add", "."); e != nil {
		t.Fatal(e)
	}
	if _, e = git(ctx, parent, "-c", "core.hooksPath=/dev/null", "-c", "commit.gpgsign=false", "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "-m", "fixture"); e != nil {
		t.Fatal(e)
	}
	os.WriteFile(filepath.Join(parent, "x.go"), []byte("package fixture\nconst X=2\n"), 0644)
	dest := filepath.Join(t.TempDir(), "source")
	if _, e = SnapshotWorktree(ctx, parent, dest); e != nil {
		t.Fatal(e)
	}
	if e = Write(dest, "x.go", []byte("package fixture\nconst X=3\n")); e != nil {
		t.Fatal(e)
	}
	v, _ := Read(parent, "x.go")
	if string(v) != "package fixture\nconst X=2\n" {
		t.Fatal("parent changed")
	}
	if e = Write(dest, "../outside", []byte("no")); e == nil {
		t.Fatal("escape accepted")
	}
	d, e := Digest(ctx, dest)
	if e != nil {
		t.Fatal(e)
	}
	b, _ := json.Marshal(Evaluation{Digest: d, Passed: true})
	os.WriteFile(filepath.Join(filepath.Dir(dest), "evaluation.json"), b, 0600)
	if _, paths, e := PromotionPlan(ctx, dest); e != nil || len(paths) != 1 {
		t.Fatalf("plan %v %v", paths, e)
	}
	if _, e = Promote(ctx, dest, parent); e != nil {
		t.Fatal(e)
	}
	v, _ = Read(parent, "x.go")
	if string(v) != "package fixture\nconst X=3\n" {
		t.Fatal("promotion missing")
	}
	if _, _, e = PromotionPlan(ctx, dest); e == nil {
		t.Fatal("parent drift accepted")
	}
}
