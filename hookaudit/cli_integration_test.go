package hookaudit

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// This opt-in test writes only to an explicitly supplied disposable fixture.
// Initialize that fixture with bd init --skip-hooks --skip-agents first.
func TestRealCLIEvents(t *testing.T) {
	fixture, logger := os.Getenv("IUGUM_AUDIT_FIXTURE"), os.Getenv("IUGUM_AUDIT_BINARY")
	if fixture == "" || logger == "" {
		t.Skip("requires disposable fixture and logger binary")
	}
	hooks := filepath.Join(fixture, ".beads", "hooks")
	dir := filepath.Join(fixture, ".beads", "hook-audit")
	if err := os.MkdirAll(hooks, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"on_create", "on_update", "on_close"} {
		p := filepath.Join(hooks, name)
		if _, err := os.Lstat(p); !os.IsNotExist(err) {
			t.Fatalf("refuse existing hook %s", p)
		}
		// Paths are supplied by the test operator, never by hook stdin.
		quote := func(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'" }
		body := "#!/bin/sh\nexec " + quote(logger) + " bead-hook-audit " + quote(dir) + "/\"$IUGUM_AUDIT_OPERATION\" " + quote(name) + " \"$@\"\n"
		if err := os.WriteFile(p, []byte(body), 0700); err != nil {
			t.Fatal(err)
		}
	}
	type row struct {
		Name   string         `json:"scenario"`
		Exit   int            `json:"exit"`
		Hooks  map[string]int `json:"hooks"`
		RawLog string         `json:"raw_log"`
	}
	var matrix []row
	count := func(path string) []string {
		data, err := os.ReadFile(path)
		if err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
		var found []string
		for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
			var r struct{ Hook string }
			if line == "" {
				continue
			}
			if err := json.Unmarshal([]byte(line), &r); err != nil {
				t.Fatalf("malformed audit record in %s: %v", path, err)
			}
			found = append(found, r.Hook)
		}
		return found
	}
	run := func(name string, args ...string) string {
		operation := fmt.Sprintf("%02d", len(matrix)+1)
		rawLog := filepath.Join(dir, operation, "events.jsonl")
		cmd := exec.Command("bd", args...)
		cmd.Dir = fixture
		cmd.Env = append(os.Environ(), "BEADS_ACTOR=hook-fixture", "IUGUM_AUDIT_OPERATION="+operation)
		out, err := cmd.CombinedOutput()
		exit := 0
		if err != nil {
			exit = 1
		}
		// Wait after the CLI has exited: this observes delivery rather than RunSync.
		time.Sleep(3 * time.Second)
		got := count(rawLog)
		r := row{name, exit, map[string]int{}, rawLog}
		for _, h := range got {
			r.Hooks[h]++
		}
		matrix = append(matrix, r)
		if exit != 0 {
			t.Logf("%s command failed: %s", name, out)
		}
		expectedFailure := strings.HasPrefix(name, "failed ")
		if (exit != 0) != expectedFailure {
			t.Errorf("%s exit=%d, expected failure=%v", name, exit, expectedFailure)
		}
		return string(out)
	}
	issueID := func(out string) string {
		var v struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal([]byte(out), &v); err != nil || v.ID == "" {
			t.Fatalf("create output invalid: %s", out)
		}
		return v.ID
	}
	a := issueID(run("create", "create", "audit fixture A", "--description", "disposable fixture", "--json"))
	b := issueID(run("create initial labels", "create", "audit fixture B", "--description", "disposable fixture", "--labels", "one,two", "--json"))
	run("claim only", "update", a, "--claim")
	run("claim plus fields", "update", b, "--claim", "--notes", "claim combined fixture")
	run("create parent dependency", "create", "audit child", "--description", "disposable fixture", "--parent", b, "--json")
	run("update fields", "update", a, "--description", "raw\nbody \"quote\" fixture")
	run("status in progress", "update", a, "--status", "in_progress")
	run("label add", "label", "add", a, "extra")
	run("label remove", "label", "remove", a, "extra")
	run("comment add", "comments", "add", a, "disposable fixture comment")
	run("dependency add", "dep", "add", b, a)
	run("dependency remove", "dep", "remove", b, a)
	run("close command", "close", a, "--reason", "fixture complete")
	run("reopen", "reopen", a)
	run("close via update", "update", a, "--status", "closed")
	run("failed missing issue", "update", "hooktest-no-such-issue", "--notes", "must fail")
	run("failed invalid status", "update", b, "--status", "invalid-fixture-status")
	run("read only", "show", b)
	t.Setenv("BD_NO_HOOKS", "1")
	run("disabled hooks", "update", b, "--notes", "hooks disabled fixture")
	t.Setenv("BD_NO_HOOKS", "0")
	os.Chmod(filepath.Join(hooks, "on_update"), 0600)
	run("non executable hook", "update", b, "--notes", "no hook executable")
	os.Chmod(filepath.Join(hooks, "on_update"), 0700)
	os.Rename(filepath.Join(hooks, "on_update"), filepath.Join(hooks, "on_update.saved"))
	run("missing hook", "update", b, "--notes", "no hook file")
	os.Rename(filepath.Join(hooks, "on_update.saved"), filepath.Join(hooks, "on_update"))
	batch := filepath.Join(fixture, "batch.md")
	os.WriteFile(batch, []byte("## Batch one\n\nfixture one\n\n## Batch two\n\nfixture two\n"), 0600)
	run("batch markdown create", "create", "--file", batch, "--json")
	// Separate CLI processes exercise real database contention and hook delivery.
	var parallel []*exec.Cmd
	for i := 0; i < 2; i++ {
		op := fmt.Sprintf("%02d", len(matrix)+1)
		cmd := exec.Command("bd", "update", b, "--notes", fmt.Sprintf("parallel fixture %d", i))
		cmd.Dir = fixture
		cmd.Env = append(os.Environ(), "BEADS_ACTOR=hook-fixture", "IUGUM_AUDIT_OPERATION="+op)
		matrix = append(matrix, row{fmt.Sprintf("parallel update %d", i+1), 0, map[string]int{}, filepath.Join(dir, op, "events.jsonl")})
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		parallel = append(parallel, cmd)
	}
	for i, cmd := range parallel {
		if err := cmd.Wait(); err != nil {
			matrix[len(matrix)-2+i].Exit = 1
			t.Errorf("parallel update %d failed: %v", i+1, err)
		}
	}
	time.Sleep(3 * time.Second)
	// Re-read every operation log after all CLI processes have exited. Each hook
	// inherits its operation directory, so late deliveries cannot contaminate rows.
	for i := range matrix {
		matrix[i].Hooks = map[string]int{}
		for _, h := range count(matrix[i].RawLog) {
			matrix[i].Hooks[h]++
		}
	}
	data, _ := json.MarshalIndent(matrix, "", "  ")
	if err := os.WriteFile(filepath.Join(fixture, "matrix.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	t.Logf("Actual CLI matrix: %s; private per-operation raw logs: %s", filepath.Join(fixture, "matrix.json"), dir)
}
