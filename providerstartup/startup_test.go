package providerstartup

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestPasswordReuseAndBoundaries(t *testing.T) {
	d := t.TempDir()
	p := filepath.Join(d, "password")
	a, e := password(p)
	if e != nil || len(a) != 96 {
		t.Fatal("creation failed", e)
	}
	b, e := password(p)
	if e != nil || !bytes.Equal(a, b) {
		t.Fatal("existing password rotated")
	}
	if e = os.Chmod(p, 0644); e != nil {
		t.Fatal(e)
	}
	if _, e = password(p); e == nil {
		t.Fatal("insecure file accepted")
	}
	os.Chmod(p, 0600)
	link := filepath.Join(d, "link")
	os.Symlink(p, link)
	if _, e = password(link); e == nil {
		t.Fatal("symlink accepted")
	}
	c, _ := os.ReadFile(p)
	if !bytes.Equal(a, c) {
		t.Fatal("credential modified")
	}
}
func validSettings() Settings {
	return Settings{Mode: "managed", Root: "/config/root", PasswordFile: "/config/password", TokenFile: "/config/token", ModelKeyFile: "/config/key", ComparisonRoot: "/config/comparison", HostConfig: "/config/host", HostArgs: []string{"/config/iugum", "agent", "sessions", "serve", "--listen", "127.0.0.1:3853", "--host-tools", "--data-dir", "/config/.local/share/iugum/chat-sessions", "--claude-policy", "/config/.config/iugum-chat/session-policy.csv"}}
}
func TestModesAndRemoteBoundary(t *testing.T) {
	s := validSettings()
	if e := validate(s); e != nil {
		t.Fatal(e)
	}
	s.Mode = "existing"
	s.ExistingURL = "http://127.0.0.1:4097"
	if e := validate(s); e != nil {
		t.Fatal(e)
	}
	for _, url := range []string{"http://192.168.1.42:4097", "http://user:pw@127.0.0.1:4097", "http://127.0.0.1:4097/?token=bad"} {
		s.ExistingURL = url
		if validate(s) == nil {
			t.Fatal("unsafe endpoint accepted", url)
		}
	}
	s = validSettings()
	s.HostArgs = append(s.HostArgs, "--coordination")
	if validate(s) == nil {
		t.Fatal("coordination expansion accepted")
	}
}
func TestExistingAuthErrorsDoNotModifyCredential(t *testing.T) {
	p := filepath.Join(t.TempDir(), "pw")
	os.WriteFile(p, []byte("saved"), 0600)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, p, ok := r.BasicAuth()
		if !ok || u != "opencode" || p != "saved" {
			w.WriteHeader(401)
			return
		}
		w.Write([]byte(`{"healthy":true}`))
	}))
	defer server.Close()
	if e := verifyExisting(context.Background(), server.URL, []byte("saved")); e != nil {
		t.Fatal(e)
	}
	if e := verifyExisting(context.Background(), server.URL, []byte("wrong")); e == nil || !strings.Contains(e.Error(), "rejected credential") {
		t.Fatal("auth error unclear", e)
	}
	b, _ := os.ReadFile(p)
	if string(b) != "saved" {
		t.Fatal("existing credential changed")
	}
}
func TestExistingRejectsRedirect(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Error("followed redirect") }))
	defer target.Close()
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, http.StatusFound) }))
	defer source.Close()
	if verifyExisting(context.Background(), source.URL, []byte("pw")) == nil {
		t.Fatal("redirect accepted")
	}
}
func TestOwnedChildAlreadyExitedStopsPromptly(t *testing.T) {
	c, e := start([]string{"/bin/sh", "-c", "exit 0"}, os.Environ(), t.TempDir(), filepath.Join(t.TempDir(), "log"))
	if e != nil {
		t.Fatal(e)
	}
	<-c.done
	done := make(chan bool)
	go func() { stop(c); done <- true }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("stop hung on consumed exit result")
	}
}

func TestAlternateFlagsCannotExpandHostFeatures(t *testing.T) {
	for _, flag := range []string{"--coordination=true", "--browser-grants=true", "--listen=0.0.0.0:3853", "--host-tools=false"} {
		s := validSettings()
		s.HostArgs = append(s.HostArgs, flag)
		if validate(s) == nil {
			t.Fatal("alternate flag accepted", flag)
		}
	}
	s := validSettings()
	s.HostArgs = append(s.HostArgs, "--listen", "127.0.0.1:3853")
	if validate(s) == nil {
		t.Fatal("duplicate listen accepted")
	}
}

func TestPublicHealthDoesNotAcceptWrongCredential(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/global/health" {
			w.Write([]byte(`{"healthy":true}`))
			return
		}
		_, pw, ok := r.BasicAuth()
		if !ok || pw != "saved" {
			w.WriteHeader(401)
			return
		}
		w.Write([]byte(`{}`))
	}))
	defer server.Close()
	if verifyExisting(context.Background(), server.URL, []byte("wrong")) == nil {
		t.Fatal("wrong credential accepted using public health")
	}
	if e := verifyExisting(context.Background(), server.URL, []byte("saved")); e != nil {
		t.Fatal(e)
	}
}
func TestLeaderExitRetainsOwnershipAndKillsDescendant(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("provider process-group ownership requires Linux waitid and /proc")
	}
	d := t.TempDir()
	pidfile := filepath.Join(d, "descendant")
	script := `/bin/sh -c 'trap "" TERM; echo $$ > "$PIDFILE"; while :; do sleep 1; done' & while [ ! -s "$PIDFILE" ]; do :; done; exit 0`
	c, e := start([]string{"/bin/sh", "-c", script}, append(os.Environ(), "PIDFILE="+pidfile), d, filepath.Join(d, "log"))
	if e != nil {
		t.Fatal(e)
	}
	select {
	case <-c.done:
	case <-time.After(3 * time.Second):
		t.Fatal("leader not observed")
	}
	if e = syscall.Kill(c.cmd.Process.Pid, 0); e != nil {
		t.Fatal("leader was prematurely reaped")
	}
	b, e := os.ReadFile(pidfile)
	if e != nil {
		t.Fatal(e)
	}
	desc, _ := strconv.Atoi(strings.TrimSpace(string(b)))
	if desc <= 0 {
		t.Fatal("missing descendant")
	}
	stop(c)
	if groupAlive(c.cmd.Process.Pid) {
		t.Fatal("owned descendant survived cleanup")
	}
	if e = syscall.Kill(c.cmd.Process.Pid, 0); e == nil {
		t.Fatal("leader not reaped after cleanup")
	}
}
