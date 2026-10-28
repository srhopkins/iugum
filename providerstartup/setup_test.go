package providerstartup

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestSetupAndRestartReuseCredential(t *testing.T) {
	d := t.TempDir()
	s := validSettings()
	s.Root = filepath.Join(d, "state")
	s.PasswordFile = filepath.Join(d, "saved-password")
	destination := filepath.Join(s.Root, "startup.json")
	if e := Setup(s, destination, "/reviewed/iugum"); e != nil {
		t.Fatal(e)
	}
	before, _ := os.ReadFile(s.PasswordFile)
	loaded, e := Load(destination)
	if e != nil {
		t.Fatal(e)
	}
	if loaded.HostArgs[0] != "/reviewed/iugum" {
		t.Fatal("adapter not wired to same application")
	}
	if e = Setup(loaded, destination, "/reviewed/iugum"); e != nil {
		t.Fatal(e)
	}
	after, _ := os.ReadFile(s.PasswordFile)
	if !bytes.Equal(before, after) {
		t.Fatal("restart/setup rotated password")
	}
	b, _ := os.ReadFile(destination)
	if bytes.Contains(b, before) {
		t.Fatal("credential leaked into settings")
	}
	st, _ := os.Stat(destination)
	if st.Mode().Perm() != 0600 {
		t.Fatal("manifest not private")
	}
}
func TestSetupExistingNeverCreatesOrRotatesCredential(t *testing.T) {
	d := t.TempDir()
	s := validSettings()
	s.Mode = "existing"
	s.ExistingURL = "http://127.0.0.1:4097"
	s.Root = filepath.Join(d, "state")
	s.PasswordFile = filepath.Join(d, "user-owned")
	destination := filepath.Join(s.Root, "startup.json")
	if Setup(s, destination, "/reviewed/iugum") == nil {
		t.Fatal("created absent existing credential")
	}
	if _, e := os.Stat(s.PasswordFile); !os.IsNotExist(e) {
		t.Fatal("existing credential appeared")
	}
	os.WriteFile(s.PasswordFile, []byte("saved-private"), 0600)
	if e := Setup(s, destination, "/reviewed/iugum"); e != nil {
		t.Fatal(e)
	}
	b, _ := os.ReadFile(s.PasswordFile)
	if string(b) != "saved-private" {
		t.Fatal("existing credential changed")
	}
}
func TestManifestDefaultsToManagedAndRejectsPlainSettings(t *testing.T) {
	s := validSettings()
	s.Mode = ""
	b, _ := json.Marshal(s)
	p := filepath.Join(t.TempDir(), "manifest")
	os.WriteFile(p, b, 0600)
	got, e := Load(p)
	if e != nil || got.Mode != "managed" {
		t.Fatal("default mode missing", e)
	}
	os.Chmod(p, 0644)
	if _, e = Load(p); e == nil {
		t.Fatal("unprotected manifest accepted")
	}
}

func TestGeneratedStartupQuotesPathsAndRegistersOptIn(t *testing.T) {
	d := t.TempDir()
	s := validSettings()
	s.Root = filepath.Join(d, "space's state")
	s.PasswordFile = filepath.Join(d, "password")
	destination := filepath.Join(s.Root, "startup.json")
	binary := filepath.Join(d, "Steve's iugum")
	os.WriteFile(binary, []byte("#!/bin/sh\nprintf '%s\\n' \"$@\"\n"), 0700)
	if e := Setup(s, destination, binary); e != nil {
		t.Fatal(e)
	}
	out, e := exec.Command("/bin/sh", filepath.Join(s.Root, "service/run")).CombinedOutput()
	if e != nil {
		t.Fatal(string(out), e)
	}
	if string(out) != "agent\nsessions\nstart\n--config\n"+destination+"\n" {
		t.Fatal("quoted arguments changed", string(out))
	}
	if _, e = os.Stat(filepath.Join(s.Root, "enabled")); e != nil {
		t.Fatal("setup omitted startup opt-in")
	}
}

func TestNativeBootDependencyPreservesLauncher(t *testing.T) {
	d := t.TempDir()
	s := validSettings()
	s.Root = filepath.Join(d, "state")
	s.PasswordFile = filepath.Join(d, "password")
	s.NativeRunScript = filepath.Join(d, "existing native")
	original := []byte("#!/bin/sh\nexec /existing/native --unchanged\n")
	os.WriteFile(s.NativeRunScript, original, 0700)
	destination := filepath.Join(s.Root, "startup.json")
	if e := Setup(s, destination, "/reviewed/iugum"); e != nil {
		t.Fatal(e)
	}
	saved, _ := os.ReadFile(s.NativeRunScript)
	if !bytes.Equal(original, saved) {
		t.Fatal("existing native launcher changed")
	}
	wrapper, _ := os.ReadFile(filepath.Join(s.Root, "service/native-run"))
	if !bytes.Contains(wrapper, []byte("sessions wait-ready")) || !bytes.Contains(wrapper, []byte("exec '"+s.NativeRunScript+"'")) {
		t.Fatal("native dependency omitted")
	}
	if e := exec.Command("/bin/sh", "-n", filepath.Join(s.Root, "service/native-run")).Run(); e != nil {
		t.Fatal(e)
	}
}
