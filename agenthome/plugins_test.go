package agenthome

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPortableSkillsAndTraversal(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "skills", "review")
	os.MkdirAll(dir, 0700)
	os.WriteFile(filepath.Join(root, "plugin.json"), []byte(`{"name":"review-tools"}`), 0600)
	os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: review\ndescription: Review work\n---\nRead the evidence."), 0600)
	skills, e := Skills(root)
	if e != nil || len(skills) != 1 {
		t.Fatalf("%v %v", skills, e)
	}
	if _, e = ReadAsset(dir, "../../plugin.json"); e == nil {
		t.Fatal("path escaped")
	}
	os.Symlink(filepath.Join(root, "plugin.json"), filepath.Join(dir, "escape"))
	if _, e = ReadAsset(dir, "escape"); e == nil {
		t.Fatal("symlink escaped")
	}
}
