// Package atomdowninline holds the atomdown-inline SilverBullet plug.
//
// The plug itself is hand-authored JavaScript, not Go: a SilverBullet plug is
// a worker bundle the browser loads, and there is no plug-compile step on this
// machine (see plugs/atomdown-board/README.md, "Why hand-authored"). So this
// package holds no Go code — only this test, which runs the plug's own
// JavaScript unit tests so that `go test ./...` covers them like everything
// else in the repo.
package atomdowninline

import (
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// minimumPlugTests is a floor, not the exact count, so adding a test does not
// break the build. It exists because `node --test` exits 0 for a file that
// registers no tests at all: a bad import or a stray early return would make
// this Go test pass while covering nothing.
const minimumPlugTests = 90

// TestPlugJavaScript runs atomdown-inline.test.mjs with node's built-in test
// runner. node is not a build dependency of iugum, so this skips rather than
// fails when node is absent.
func TestPlugJavaScript(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed; run the plug tests with `node --test plugs/atomdown-inline/`")
	}

	cmd := exec.Command(node, "--test", "atomdown-inline.test.mjs")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("plug JavaScript tests failed: %v\n%s", err, output)
	}

	summary := string(output)
	if fail := countFromSummary(t, summary, "fail"); fail != 0 {
		t.Fatalf("plug JavaScript tests reported %d failures:\n%s", fail, summary)
	}
	if pass := countFromSummary(t, summary, "pass"); pass < minimumPlugTests {
		t.Fatalf("only %d plug JavaScript tests ran, want at least %d (did the suite stop loading?):\n%s",
			pass, minimumPlugTests, summary)
	}
}

// countFromSummary reads one counter out of node --test's trailing summary
// ("# pass 97" in TAP form, "ℹ pass 97" in the spec reporter).
func countFromSummary(t *testing.T, output, name string) int {
	t.Helper()
	pattern := regexp.MustCompile(`(?m)^\D*` + name + `\s+(\d+)\s*$`)
	matches := pattern.FindAllStringSubmatch(output, -1)
	if len(matches) == 0 {
		t.Fatalf("no %q counter in node --test output:\n%s", name, output)
	}
	last := matches[len(matches)-1][1]
	value, err := strconv.Atoi(strings.TrimSpace(last))
	if err != nil {
		t.Fatalf("unreadable %q counter %q: %v", name, last, err)
	}
	return value
}

// TestPlugParses is the cheapest guard against a syntax error in the
// hand-authored bundle. A broken plug file fails no build: nothing compiles
// it, and SilverBullet only discovers the problem when the reader clicks the
// icon.
func TestPlugParses(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed")
	}

	cmd := exec.Command(node, "--check", "atomdown-inline.plug.js")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("atomdown-inline.plug.js is not valid JavaScript: %v\n%s", err, output)
	}
}
