package agentchat

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestIndependentFeatures(t *testing.T) {
	space := t.TempDir()
	for _, pair := range [][2]bool{{true, false}, {false, true}, {true, true}, {false, false}} {
		if err := InstallFeatures(space, pair[0], pair[1]); err != nil {
			t.Fatal(err)
		}
		for i, name := range []string{"iugum-agent-chat", "iugum-search"} {
			_, err := os.Stat(filepath.Join(space, "_plug", name+".plug.js"))
			if (err == nil) != pair[i] {
				t.Fatalf("%s installed=%v want=%v", name, err == nil, pair[i])
			}
		}
	}
}

// Existing installations keep a backup while the renamed source replaces the plug.
func TestLegacyPlugUpgradePreservesUserCopy(t *testing.T) {
	space := t.TempDir()
	dir := filepath.Join(space, "_plug")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	old := filepath.Join(dir, "iugum-chief.plug.js")
	if err := os.WriteFile(old, []byte("user customization"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := InstallFeatures(space, true, false); err != nil {
		t.Fatal(err)
	}
	backup, err := os.ReadFile(old + ".previous")
	if err != nil || string(backup) != "user customization" {
		t.Fatalf("legacy copy not preserved: %q, %v", backup, err)
	}
	current, err := os.ReadFile(filepath.Join(dir, "iugum-agent-chat.plug.js"))
	if err != nil || !bytes.Contains(current, []byte("./agentchat.js:openChat")) {
		t.Fatalf("renamed plug not installed: %v", err)
	}
}
