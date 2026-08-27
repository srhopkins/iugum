package beadsadapt

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Regression tests for iugum-wuz.
//
// Tracker.Run sets BD_NAME=iugum. That renames the cobra root command.
// The old no-database check in beads/cmd/bd/main.go compared the parent
// command name with the literal "bd". With the renamed root, every top-level
// command looked like a nested one, so "init", "version", "where" and the
// other no-database commands tried to open a store that did not exist.
//
// bd calls os.Exit on failure, so each case runs in a child test process.
// The child runs Tracker.Run in an empty git repo. The parent checks the
// exit code and the file system.

const childEnv = "IUGUM_BEADSADAPT_CHILD_ARGS"

// TestBeadsChild is the child body. It only runs when childEnv is set.
func TestBeadsChild(t *testing.T) {
	args := os.Getenv(childEnv)
	if args == "" {
		t.Skip("child helper")
	}
	if err := (Tracker{}).Run(context.Background(), strings.Fields(args)); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if os.Getenv("BD_NAME") != "iugum" {
		t.Fatalf("BD_NAME = %q, want iugum", os.Getenv("BD_NAME"))
	}
}

func runChild(t *testing.T, dir string, args ...string) (string, error) {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run", "^TestBeadsChild$", "-test.v")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		childEnv+"="+strings.Join(args, " "),
		"HOME="+dir, // keep the child away from the developer's global beads state
		"BEADS_DIR=",
		"BD_NAME=",
	)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func newRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	dir := t.TempDir()
	if out, err := exec.Command("git", "-C", dir, "init", "-q", ".").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
	return dir
}

// TestRunNoDBCommandsOutsideWorkspace runs commands from noDbCommands in a
// repo with no .beads directory. Each one must exit 0.
// "where" is not here: like Homebrew bd, it exits 1 outside a workspace
// with a hint. It still must not fail with "no beads database found".
func TestRunNoDBCommandsOutsideWorkspace(t *testing.T) {
	cases := [][]string{
		{"version"},
		{"prime"},
		{"quickstart", "--help"},
		{"setup", "--help"},
		{"onboard", "--help"},
	}
	for _, args := range cases {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			dir := newRepo(t)
			out, err := runChild(t, dir, args...)
			if err != nil {
				t.Fatalf("%v exited with error: %v\n%s", args, err, out)
			}
			if strings.Contains(out, "no beads database found") {
				t.Fatalf("%v tried to open a store:\n%s", args, out)
			}
		})
	}
}

// TestRunInitCreatesBeadsDir runs "init" in a fresh repo and expects .beads.
// Init writes an embedded Dolt database. That needs CGO. The test skips on a
// CGO_ENABLED=0 build; the no-database classification is still covered by
// TestRunNoDBCommandsOutsideWorkspace.
func TestRunInitCreatesBeadsDir(t *testing.T) {
	dir := newRepo(t)
	out, err := runChild(t, dir, "init", "--prefix", "zz", "--non-interactive", "--quiet")
	if strings.Contains(out, "embedded-mode support") {
		t.Skip("embedded Dolt not compiled in (CGO_ENABLED=0)")
	}
	if strings.Contains(out, "no beads database found") {
		t.Fatalf("init tried to open a store before creating one:\n%s", out)
	}
	if err != nil {
		t.Fatalf("init exited with error: %v\n%s", err, out)
	}
	if st, statErr := os.Stat(filepath.Join(dir, ".beads")); statErr != nil || !st.IsDir() {
		t.Fatalf(".beads not created by init: %v\n%s", statErr, out)
	}
}
