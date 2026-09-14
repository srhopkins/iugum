// Package chiefui carries the opt-in wiki extension for configured native agents.
package chiefui

import (
	"bytes"
	"embed"
	"fmt"
	"os"
	"path/filepath"
)

//go:embed *.js
var Files embed.FS

// Install writes the managed plug into this agent's own wiki, not every wiki.
// User-edited copies are backed up before an extension upgrade.
func Install(space string) error { return InstallFeatures(space, true, true) }

// InstallFeatures independently installs or retires the two managed add-ons.
func InstallFeatures(space string, chat, search bool) error {
	for _, f := range []struct {
		name, source string
		enabled      bool
	}{{"iugum-agent-chat", "chief.plug.js", chat}, {"iugum-search", "search.plug.js", search}} {
		if err := installFeature(space, f.name, f.source, f.enabled); err != nil {
			return err
		}
	}
	return nil
}
func installFeature(space, name, source string, enabled bool) error {
	dir := filepath.Join(space, "_plug")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	st, err := os.Lstat(dir)
	if err != nil {
		return err
	}
	if st.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("Agent chat plug directory cannot be a symlink")
	}
	data, err := Files.ReadFile(source)
	if err != nil {
		return err
	}
	path := filepath.Join(dir, name+".plug.js")
	legacy := filepath.Join(dir, "iugum-chief.plug.js")
	if _, err := os.Lstat(legacy); err == nil {
		if err = os.Rename(legacy, legacy+".previous"); err != nil {
			return err
		}
	}
	if st, e := os.Lstat(path); e == nil && st.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("Agent chat plug cannot be a symlink")
	}
	if !enabled {
		if _, err := os.Lstat(path); os.IsNotExist(err) {
			return nil
		}
		return os.Rename(path, path+".disabled")
	}
	old, e := os.ReadFile(path)
	if e == nil && bytes.Equal(old, data) {
		return nil
	}
	if e != nil && !os.IsNotExist(e) {
		return e
	}
	if len(old) > 0 {
		if e = os.WriteFile(path+".previous", old, 0600); e != nil {
			return e
		}
	}
	temp, e := os.CreateTemp(dir, ".chief-*")
	if e != nil {
		return e
	}
	defer os.Remove(temp.Name())
	if _, e = temp.Write(data); e != nil {
		temp.Close()
		return e
	}
	if e = temp.Close(); e != nil {
		return e
	}
	return os.Rename(temp.Name(), path)
}
