package providerstartup

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// Load reads the reviewed non-secret lifecycle manifest from a protected file.
func Load(path string) (Settings, error) {
	var s Settings
	b, e := privateRead(path)
	if e != nil {
		return s, e
	}
	if e = json.Unmarshal(b, &s); e != nil {
		return s, errors.New("invalid provider startup manifest")
	}
	if s.Mode == "" {
		s.Mode = "managed"
	}
	return s, validate(s)
}

// Setup configures both the adapter and managed server using references only.
// Existing mode never creates or modifies the server credential.
func Setup(s Settings, destination, executable string) error {
	if s.Mode == "" {
		s.Mode = "managed"
	}
	if !filepath.IsAbs(destination) || !filepath.IsAbs(executable) {
		return errors.New("setup paths must be absolute")
	}
	s.HostArgs = append([]string{}, s.HostArgs...)
	if len(s.HostArgs) == 0 {
		return errors.New("existing host arguments required")
	}
	s.HostArgs[0] = executable
	if e := validate(s); e != nil {
		return e
	}
	if s.NativeRunScript != "" {
		resolved, e := filepath.EvalSymlinks(s.NativeRunScript)
		if e != nil {
			return errors.New("native launcher reference cannot be resolved")
		}
		for _, target := range []string{filepath.Join(s.Root, "service", "native-run"), filepath.Join(s.Root, "service", "run"), "/custom-services.d/iugum-native-chat/run"} {
			if filepath.Clean(s.NativeRunScript) == target || resolved == target {
				return errors.New("native launcher must reference preserved original, not generated service entry")
			}
		}
		st, e := os.Stat(s.NativeRunScript)
		if e != nil || !st.Mode().IsRegular() || st.Mode().Perm()&0111 == 0 {
			return errors.New("existing native launcher missing or not executable")
		}
	}
	if e := os.MkdirAll(s.Root, 0700); e != nil {
		return e
	}
	st, e := os.Lstat(s.Root)
	if e != nil || !st.IsDir() || st.Mode().Perm()&0077 != 0 {
		return errors.New("provider root must be private directory")
	}
	if s.Mode == "managed" {
		if _, e = password(s.PasswordFile); e != nil {
			return e
		}
	} else {
		if _, e = privateRead(s.PasswordFile); e != nil {
			return e
		}
	}
	parent := filepath.Dir(destination)
	if e = os.MkdirAll(parent, 0700); e != nil {
		return e
	}
	st, e = os.Lstat(parent)
	if e != nil || !st.IsDir() || st.Mode().Perm()&0077 != 0 {
		return errors.New("manifest parent must be private directory")
	}
	b, e := json.MarshalIndent(s, "", "  ")
	if e != nil {
		return e
	}
	tmp, e := os.CreateTemp(parent, ".startup-*")
	if e != nil {
		return e
	}
	name := tmp.Name()
	defer os.Remove(name)
	if e = tmp.Chmod(0600); e == nil {
		_, e = tmp.Write(append(b, '\n'))
	}
	if e == nil {
		e = tmp.Sync()
	}
	closeErr := tmp.Close()
	if e != nil {
		return e
	}
	if closeErr != nil {
		return closeErr
	}
	if e = os.Rename(name, destination); e != nil {
		return e
	}
	serviceDir := filepath.Join(s.Root, "service")
	if e = os.MkdirAll(serviceDir, 0700); e != nil {
		return e
	}
	quote := func(v string) string { return "'" + strings.ReplaceAll(v, "'", "'\\''") + "'" }
	script := "#!/bin/sh\nexport IUGUM_CONFIG=" + quote(s.HostConfig) + "\nexec " + quote(executable) + " agent sessions start --config " + quote(destination) + "\n"
	if e = writePrivate(filepath.Join(serviceDir, "run"), []byte(script), 0700); e != nil {
		return e
	}
	if s.NativeRunScript != "" {
		if !filepath.IsAbs(s.NativeRunScript) {
			return errors.New("native launcher reference must be absolute")
		}
		st, e := os.Stat(s.NativeRunScript)
		if e != nil || !st.Mode().IsRegular() {
			return errors.New("existing native launcher missing")
		}
		native := "#!/bin/sh\nset -eu\nIUGUM_CONFIG=" + quote(s.HostConfig) + " " + quote(executable) + " agent sessions wait-ready --config " + quote(destination) + "\nexec " + quote(s.NativeRunScript) + "\n"
		if e = writePrivate(filepath.Join(serviceDir, "native-run"), []byte(native), 0700); e != nil {
			return e
		}
	}
	return writePrivate(filepath.Join(s.Root, "enabled"), []byte("configured\n"), 0600)
}

func writePrivate(path string, b []byte, mode os.FileMode) error {
	parent := filepath.Dir(path)
	st, e := os.Lstat(parent)
	if e != nil || !st.IsDir() || st.Mode().Perm()&0077 != 0 {
		return errors.New("startup artifact directory must be private")
	}
	if raw, ok := st.Sys().(*syscall.Stat_t); !ok || int(raw.Uid) != os.Getuid() {
		return errors.New("startup artifact ownership mismatch")
	}
	f, e := os.CreateTemp(parent, ".private-*")
	if e != nil {
		return e
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if e = f.Chmod(mode); e == nil {
		_, e = f.Write(b)
	}
	if e == nil {
		e = f.Sync()
	}
	closeErr := f.Close()
	if e != nil {
		return e
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(tmp, path)
}
