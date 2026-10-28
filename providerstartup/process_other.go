//go:build !linux

package providerstartup

import "errors"

func waitExited(pid int) error { return errors.New("provider service supervisor requires Linux") }
func groupAlive(pgid int) bool { return false }
