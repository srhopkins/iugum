// Package hookaudit records hook inputs without interpreting or executing them.
package hookaudit

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

// Append stores exact stdin bytes as base64 in one JSONL record. The temporary
// record bounds memory and flock keeps independent processes from interleaving.
func Append(dir, hook string, argv []string, input io.Reader) error {
	switch hook {
	case "on_create", "on_update", "on_close":
	default:
		return fmt.Errorf("unsupported lifecycle hook %q", hook)
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	info, err := os.Lstat(dir)
	if err != nil {
		return err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || !ok || stat.Uid != uint32(os.Geteuid()) || info.Mode().Perm() != 0700 {
		return fmt.Errorf("audit directory must be owned by current user with mode 0700")
	}
	tmp, err := os.CreateTemp(dir, ".record-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()
	prefix, err := json.Marshal(struct {
		Timestamp string   `json:"timestamp"`
		Hook      string   `json:"hook"`
		Argv      []string `json:"argv"`
		Encoding  string   `json:"stdin_encoding"`
	}{time.Now().UTC().Format(time.RFC3339Nano), hook, argv, "base64"})
	if err != nil {
		return err
	}
	if _, err = tmp.Write(append(prefix[:len(prefix)-1], []byte(",\"stdin_base64\":\"")...)); err != nil {
		return err
	}
	enc := base64.NewEncoder(base64.StdEncoding, tmp)
	if _, err = io.Copy(enc, input); err != nil {
		return err
	}
	if err = enc.Close(); err != nil {
		return err
	}
	if _, err = io.WriteString(tmp, "\"}\n"); err != nil {
		return err
	}
	if _, err = tmp.Seek(0, io.SeekStart); err != nil {
		return err
	}
	fd, err := unix.Open(filepath.Join(dir, "events.jsonl"), unix.O_WRONLY|unix.O_APPEND|unix.O_CREAT|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0600)
	if err != nil {
		return err
	}
	log := os.NewFile(uintptr(fd), "events.jsonl")
	defer log.Close()
	var st unix.Stat_t
	if err = unix.Fstat(fd, &st); err != nil {
		return err
	}
	if st.Mode&unix.S_IFMT != unix.S_IFREG || st.Uid != uint32(os.Geteuid()) || st.Mode&0777 != 0600 || st.Nlink != 1 {
		return fmt.Errorf("audit log must be a private owned regular file with mode 0600")
	}
	if err = unix.Flock(fd, unix.LOCK_EX); err != nil {
		return err
	}
	defer unix.Flock(fd, unix.LOCK_UN)
	if _, err = io.Copy(log, tmp); err != nil {
		return err
	}
	return log.Sync()
}
