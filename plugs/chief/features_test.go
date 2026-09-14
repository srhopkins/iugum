package chiefui

import (
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
