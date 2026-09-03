package app

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/srhopkins/iugum/contract"
)

// TestServeBeadViewRequiresBeadsDir checks the fast-fail path: a directory
// with no .beads/ must error before ServeBeadView ever binds a port or
// resolves an executable, so this needs no built iugum binary.
func TestServeBeadViewRequiresBeadsDir(t *testing.T) {
	a := newE2EApp(t, e2eConfig(t))
	dir := t.TempDir()
	err := a.ServeBeadView(context.Background(), 0, "", dir)
	if err == nil {
		t.Fatal("expected an error for a directory with no .beads/")
	}
}

// TestServeBeadViewGate checks the Casbin gate is consulted: a deny policy
// for beadview/serve must stop the server before it binds a port.
func TestServeBeadViewGate(t *testing.T) {
	cfg := e2eConfig(t)
	a := newE2EApp(t, cfg)
	a.Gate = denyGate{obj: "beadview", act: "serve"}
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, ".beads"), 0o755); err != nil {
		t.Fatal(err)
	}
	err := a.ServeBeadView(context.Background(), 0, "", dir)
	if err == nil {
		t.Fatal("expected the gate to deny beadview/serve")
	}
}

// TestServeBeadViewLive builds a real iugum binary (CGO, same as
// scripts/build.sh --cgo) and drives ServeBeadView against this repo's own
// .beads/ directory, over HTTP, with no bd on PATH. It is the same claim
// the package README makes, checked in CI rather than only by hand.
// It skips (not fails) when a CGO/ICU toolchain is not available, since
// go test ./... must still pass on a plain checkout.
func TestServeBeadViewLive(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode")
	}
	repoRoot, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(repoRoot, ".beads")); err != nil {
		t.Skip("repo root has no .beads/ (unexpected checkout layout)")
	}

	bin := buildIugumForTest(t, repoRoot)

	a := newE2EApp(t, e2eConfig(t))
	origExe := osExecutable
	osExecutable = func() (string, error) { return bin, nil }
	t.Cleanup(func() { osExecutable = origExe })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() { errCh <- a.ServeBeadView(ctx, 38491, "127.0.0.1", repoRoot) }()

	url := "http://127.0.0.1:38491/"
	var resp *http.Response
	deadline := time.Now().Add(10 * time.Second)
	for {
		resp, err = http.Get(url)
		if err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("server never came up: %v", err)
		}
		time.Sleep(100 * time.Millisecond)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET / = %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if len(body) == 0 {
		t.Fatal("empty response body")
	}

	cancel()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("ServeBeadView returned: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("server did not shut down after ctx cancel")
	}
}

// buildIugumForTest compiles a throwaway iugum binary the same way
// scripts/build.sh --cgo does, skipping the test if the CGO/ICU toolchain
// this repo needs is not present (e.g. a bare CI runner).
func buildIugumForTest(t *testing.T, repoRoot string) string {
	t.Helper()
	icuPrefix, err := exec.Command("brew", "--prefix", "icu4c").Output()
	if err != nil {
		t.Skip("no CGO/ICU toolchain available (brew --prefix icu4c failed); see scripts/build.sh")
	}
	prefix := string(icuPrefix)
	for len(prefix) > 0 && (prefix[len(prefix)-1] == '\n' || prefix[len(prefix)-1] == '\r') {
		prefix = prefix[:len(prefix)-1]
	}

	out := filepath.Join(t.TempDir(), "iugum-test-bin")
	cmd := exec.Command("go", "build", "-o", out, ".")
	cmd.Dir = repoRoot
	cmd.Env = append(os.Environ(),
		"CGO_ENABLED=1",
		"CGO_CPPFLAGS=-I"+prefix+"/include",
		"CGO_LDFLAGS=-L"+prefix+"/lib",
	)
	out2, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("building test iugum binary: %v\n%s", err, out2)
	}
	return out
}

// denyGate is a contract.Policy that refuses one obj/act pair and allows
// everything else, so gate tests do not need a real Casbin model/policy file.
type denyGate struct {
	obj, act string
}

func (d denyGate) Enforce(_ context.Context, req contract.Request) error {
	if req.Obj == d.obj && req.Act == d.act {
		return fmt.Errorf("denied: %s/%s", req.Obj, req.Act)
	}
	return nil
}
