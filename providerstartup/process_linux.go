//go:build linux

package providerstartup

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"unsafe"
)

// WNOWAIT leaves the leader unreaped so its PID/PGID cannot be reused while
// stop terminates the whole owned process group, including surviving descendants.
func waitExited(pid int) error {
	var info [16]uint64
	for {
		_, _, errno := syscall.Syscall6(syscall.SYS_WAITID, 1, uintptr(pid), uintptr(unsafe.Pointer(&info[0])), 4|0x01000000, 0, 0)
		if errno == syscall.EINTR {
			continue
		}
		if errno != 0 {
			return errno
		}
		return nil
	}
}
func groupAlive(pgid int) bool {
	files, err := filepath.Glob("/proc/[0-9]*/stat")
	if err != nil {
		return true
	}
	for _, p := range files {
		b, e := os.ReadFile(p)
		if e != nil {
			continue
		}
		s := string(b)
		end := strings.LastIndex(s, ")")
		if end < 0 {
			continue
		}
		fields := strings.Fields(s[end+1:])
		if len(fields) < 3 {
			continue
		}
		group, e := strconv.Atoi(fields[2])
		if e == nil && group == pgid && fields[0] != "Z" && fields[0] != "X" {
			return true
		}
	}
	return false
}
