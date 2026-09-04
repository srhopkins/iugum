// Package atomdownboard holds the atomdown-board SilverBullet plug.
//
// The plug itself is hand-authored JavaScript, not Go: a SilverBullet plug is
// a worker bundle the browser loads, and there is no plug-compile step on this
// machine (see README.md, "Why hand-authored"). So this package holds no Go
// code — only this test, which runs the plug's own JavaScript unit tests so
// that `go test ./...` covers them like everything else in the repo.
package atomdownboard

import (
	"os/exec"
	"testing"
)

// TestPlugJavaScript runs plugs/atomdown-board/atomdown-board.test.mjs with
// node's built-in test runner.
//
// node is not a build dependency of iugum, and CONTRIBUTING.md's first rule is
// that new work is Go or part of the one program, so this test skips rather
// than fails when node is absent. It is a gate on a machine that has node and
// a no-op elsewhere; the JavaScript tests still run directly with
// `node --test plugs/atomdown-board/`.
func TestPlugJavaScript(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed; run the plug tests with `node --test plugs/atomdown-board/`")
	}

	cmd := exec.Command(node, "--test", "atomdown-board.test.mjs")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("plug JavaScript tests failed: %v\n%s", err, output)
	}
	t.Logf("plug JavaScript tests passed:\n%s", output)
}

// TestPlugParses is the cheapest possible guard against a syntax error in the
// hand-authored bundle. A broken plug file does not fail any build: nothing
// compiles it, and SilverBullet only discovers the problem when a user runs
// the command.
func TestPlugParses(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed")
	}

	cmd := exec.Command(node, "--check", "atomdown-board.plug.js")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("atomdown-board.plug.js is not valid JavaScript: %v\n%s", err, output)
	}
}
