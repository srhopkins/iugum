package hookaudit

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
)

func TestAppendExactAndConcurrent(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "audit")
	raw := append(bytes.Repeat([]byte("{\"body\":\"line\\n\"}\n"), 10000), 0xff, 0, 10)
	argv := []string{"ticket\nquoted\"", "create"}
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := Append(dir, "on_create", argv, bytes.NewReader(raw)); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	data, err := os.ReadFile(filepath.Join(dir, "events.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	lines := bytes.Split(bytes.TrimSuffix(data, []byte("\n")), []byte("\n"))
	if len(lines) != 12 {
		t.Fatalf("records %d", len(lines))
	}
	for _, line := range lines {
		var record struct {
			Timestamp, Hook string
			Argv            []string
			Body            string `json:"stdin_base64"`
			Encoding        string `json:"stdin_encoding"`
		}
		if err := json.Unmarshal(line, &record); err != nil {
			t.Fatal(err)
		}
		got, err := base64.StdEncoding.DecodeString(record.Body)
		if err != nil || !bytes.Equal(got, raw) {
			t.Fatal("raw body mismatch")
		}
		if record.Hook != "on_create" || record.Timestamp == "" || record.Encoding != "base64" || len(record.Argv) != 2 || record.Argv[0] != argv[0] {
			t.Fatal("metadata mismatch")
		}
	}
	for path, mode := range map[string]os.FileMode{dir: 0700, filepath.Join(dir, "events.jsonl"): 0600} {
		info, err := os.Stat(path)
		if err != nil || info.Mode().Perm() != mode {
			t.Fatalf("mode %s", path)
		}
	}
}

type failedReader struct{}

func (failedReader) Read([]byte) (int, error) { return 0, errors.New("fixture reader failed") }

func TestInputFailureLeavesNoRecord(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "audit")
	input := io.MultiReader(bytes.NewReader([]byte("partial private bytes")), failedReader{})
	if Append(dir, "on_create", nil, input) == nil {
		t.Fatal("reader failure ignored")
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 0 {
		t.Fatalf("partial record retained: %v", err)
	}
}

func TestRejectPublicLogAndDirectoryLink(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "audit")
	os.Mkdir(dir, 0700)
	log := filepath.Join(dir, "events.jsonl")
	os.WriteFile(log, []byte("existing\n"), 0644)
	if Append(dir, "on_close", nil, bytes.NewReader(nil)) == nil {
		t.Fatal("public log accepted")
	}
	data, _ := os.ReadFile(log)
	if string(data) != "existing\n" {
		t.Fatal("existing log changed")
	}
	link := filepath.Join(t.TempDir(), "link")
	os.Symlink(dir, link)
	if Append(link, "on_update", nil, bytes.NewReader(nil)) == nil {
		t.Fatal("directory symlink accepted")
	}
}

// Subprocess writers exercise the same OS locking used by independent hooks.
func TestConcurrentProcesses(t *testing.T) {
	if dir := os.Getenv("IUGUM_AUDIT_TEST_CHILD"); dir != "" {
		if err := Append(dir, "on_update", []string{"child"}, bytes.NewReader(bytes.Repeat([]byte("raw\n"), 20000))); err != nil {
			t.Fatal(err)
		}
		return
	}
	dir := filepath.Join(t.TempDir(), "audit")
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cmd := exec.Command(os.Args[0], "-test.run=^TestConcurrentProcesses$")
			cmd.Env = append(os.Environ(), "IUGUM_AUDIT_TEST_CHILD="+dir)
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Errorf("child failed: %v %s", err, out)
			}
		}()
	}
	wg.Wait()
	data, err := os.ReadFile(filepath.Join(dir, "events.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	lines := bytes.Split(bytes.TrimSuffix(data, []byte("\n")), []byte("\n"))
	if len(lines) != 8 {
		t.Fatalf("records %d", len(lines))
	}
	for _, line := range lines {
		var r struct {
			Body string `json:"stdin_base64"`
		}
		if json.Unmarshal(line, &r) != nil {
			t.Fatal("interleaved JSON")
		}
		raw, err := base64.StdEncoding.DecodeString(r.Body)
		if err != nil || !bytes.Equal(raw, bytes.Repeat([]byte("raw\n"), 20000)) {
			t.Fatal("interleaved body")
		}
	}
}

func TestRejectUnsafe(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "audit")
	if Append(dir, "post-update", nil, bytes.NewReader(nil)) == nil {
		t.Fatal("unknown hook accepted")
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatal("unknown hook wrote directory")
	}
	if err := os.Mkdir(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if Append(dir, "on_close", nil, bytes.NewReader(nil)) == nil {
		t.Fatal("public directory accepted")
	}
	os.Chmod(dir, 0700)
	target := filepath.Join(t.TempDir(), "target")
	os.WriteFile(target, nil, 0600)
	os.Symlink(target, filepath.Join(dir, "events.jsonl"))
	if Append(dir, "on_update", nil, bytes.NewReader(nil)) == nil {
		t.Fatal("symlink log accepted")
	}
}
