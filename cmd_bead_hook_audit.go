package main

import (
	"fmt"
	"github.com/srhopkins/iugum/hookaudit"
	"os"
)

func runBeadHookAudit(args []string) int {
	if len(args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: iugum bead-hook-audit PRIVATE_LOG_DIR on_create|on_update|on_close [hook argv...]")
		return 2
	}
	if err := hookaudit.Append(args[0], args[1], args[2:], os.Stdin); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}
