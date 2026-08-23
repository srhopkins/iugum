package dirs

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestResolveData_EnvWins(t *testing.T) {
	t.Setenv("IUGUM_DATA", "/tmp/iugum-env-data")
	got, err := ResolveData("/ignored")
	if err != nil {
		t.Fatal(err)
	}
	if got != "/tmp/iugum-env-data" {
		t.Fatalf("got %q", got)
	}
}

func TestResolveData_ConfigOverride(t *testing.T) {
	t.Setenv("IUGUM_DATA", "")
	got, err := ResolveData("/tmp/iugum-cfg-data")
	if err != nil {
		t.Fatal(err)
	}
	if got != "/tmp/iugum-cfg-data" {
		t.Fatalf("got %q", got)
	}
}

func TestDataDir_HostDefault(t *testing.T) {
	t.Setenv("IUGUM_DATA", "")
	got, err := DataDir()
	if err != nil {
		t.Fatal(err)
	}
	switch runtime.GOOS {
	case "darwin":
		if filepath.Base(got) != "iugum" || filepath.Base(filepath.Dir(got)) != "Application Support" {
			t.Fatalf("macOS data dir %q", got)
		}
	case "linux":
		if filepath.Base(got) != "iugum" {
			t.Fatalf("linux data dir %q", got)
		}
	}
	_ = os.MkdirAll(got, 0o755)
}
