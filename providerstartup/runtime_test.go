package providerstartup

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestLifecycleFixtureProcess(t *testing.T) {
	kind := os.Getenv("PROVIDER_FIXTURE")
	if kind == "" {
		return
	}
	addr := os.Getenv("FIXTURE_ADDR")
	if kind == "host" {
		os.WriteFile(os.Getenv("FIXTURE_TOKEN"), []byte("fresh-"+time.Now().String()), 0600)
	}
	server := http.Server{Addr: addr, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if kind == "backend" {
			_, pw, ok := r.BasicAuth()
			if !ok || pw != os.Getenv("OPENCODE_SERVER_PASSWORD") {
				w.WriteHeader(401)
				return
			}
		}
		if r.URL.Path == "/global/health" {
			w.Write([]byte(`{"healthy":true}`))
			return
		}
		w.Write([]byte(`[]`))
	})}
	if e := server.ListenAndServe(); e != nil {
		os.Exit(3)
	}
}
func fixtureAddress(t *testing.T) string {
	t.Helper()
	l, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	a := l.Addr().String()
	l.Close()
	return a
}
func runtimeFixture(t *testing.T) (Settings, runtimeConfig) {
	t.Helper()
	d := t.TempDir()
	s := validSettings()
	s.Root = filepath.Join(d, "state")
	s.PasswordFile = filepath.Join(d, "password")
	s.TokenFile = filepath.Join(d, "token")
	s.ModelKeyFile = filepath.Join(d, "key")
	s.ComparisonRoot = d
	s.HostConfig = filepath.Join(d, "host-config")
	os.WriteFile(s.ModelKeyFile, []byte("fixture-model-key"), 0600)
	os.WriteFile(s.TokenFile, []byte("old-token"), 0600)
	cfg := map[string]any{"provider": map[string]any{"litellm-homelab": map[string]any{"options": map[string]any{}}}, "mcp": map[string]any{"editor": map[string]any{"url": "http://127.0.0.1:3853/api/host/mcp"}}}
	b, _ := json.Marshal(cfg)
	os.WriteFile(filepath.Join(d, "reviewed-editor-control-config.json"), b, 0600)
	r := runtimeConfig{hostAddr: fixtureAddress(t), backendAddr: fixtureAddress(t)}
	r.spawn = func(args, env []string, cwd, log string) (*child, error) {
		kind := "host"
		addr := r.hostAddr
		if args[0] == "opencode" {
			kind = "backend"
			addr = r.backendAddr
		}
		env = envSet(envSet(envSet(env, "PROVIDER_FIXTURE", kind), "FIXTURE_ADDR", addr), "FIXTURE_TOKEN", s.TokenFile)
		return start([]string{os.Args[0], "-test.run=^TestLifecycleFixtureProcess$"}, env, cwd, log)
	}
	return s, r
}
func waitFixture(t *testing.T, addr string) {
	t.Helper()
	for i := 0; i < 80; i++ {
		c, e := net.DialTimeout("tcp", addr, 100*time.Millisecond)
		if e == nil {
			c.Close()
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("fixture service did not start")
}
func TestManagedLifecycleRestartAndDuplicateGuard(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("managed provider supervisor requires Linux")
	}
	s, r := runtimeFixture(t)
	var saved []byte
	for round := 0; round < 2; round++ {
		ctx, cancel := context.WithCancel(context.Background())
		result := make(chan error, 1)
		go func() { result <- runWith(ctx, s, r) }()
		waitFixture(t, r.backendAddr)
		if e := waitReadyWith(ctx, s, "http://"+r.hostAddr, "http://"+r.backendAddr); e != nil {
			t.Fatal("native dependency failed", e)
		}
		pw, _ := os.ReadFile(s.PasswordFile)
		if round == 0 {
			saved = pw
			if e := runWith(ctx, s, r); e == nil || !strings.Contains(e.Error(), "already running") {
				t.Fatal("duplicate supervisor accepted", e)
			}
		} else if !bytes.Equal(saved, pw) {
			t.Fatal("restart rotated credential")
		}
		token, _ := os.ReadFile(s.TokenFile)
		if string(token) == "old-token" {
			t.Fatal("old host token reused")
		}
		cancel()
		select {
		case e := <-result:
			if e != nil {
				t.Fatal(e)
			}
		case <-time.After(12 * time.Second):
			t.Fatal("owned services failed to stop")
		}
		for _, addr := range []string{r.hostAddr, r.backendAddr} {
			if c, e := net.DialTimeout("tcp", addr, 50*time.Millisecond); e == nil {
				c.Close()
				t.Fatal("owned listener remained", addr)
			}
		}
	}
}
func TestExistingLifecycleLeavesServerRunning(t *testing.T) {
	s, r := runtimeFixture(t)
	s.Mode = "existing"
	os.WriteFile(s.PasswordFile, []byte("existing-password"), 0600)
	external := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, q *http.Request) {
		_, p, ok := q.BasicAuth()
		if !ok || p != "existing-password" {
			w.WriteHeader(401)
			return
		}
		if q.URL.Path == "/global/health" {
			w.Write([]byte(`{"healthy":true}`))
			return
		}
		w.Write([]byte(`{}`))
	}))
	defer external.Close()
	s.ExistingURL = external.URL
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() { result <- runWith(ctx, s, r) }()
	waitFixture(t, r.hostAddr)
	cancel()
	select {
	case e := <-result:
		if e != nil {
			t.Fatal(e)
		}
	case <-time.After(12 * time.Second):
		t.Fatal("adapter did not stop")
	}
	if e := verifyExisting(context.Background(), external.URL, []byte("existing-password")); e != nil {
		t.Fatal("user-owned server changed", e)
	}
	b, _ := os.ReadFile(s.PasswordFile)
	if string(b) != "existing-password" {
		t.Fatal("existing credential changed")
	}
}
