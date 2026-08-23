package job

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"

	"github.com/srhopkins/iugum/contract"
)

// Exec returns a JobFunc that runs command with CommandContext.
// Stdout and stderr go to the process; stderr is also captured for error text.
func Exec(command []string) contract.JobFunc {
	cmd := append([]string(nil), command...)
	return func(ctx context.Context, _ contract.Event) error {
		if len(cmd) == 0 {
			return fmt.Errorf("iugum: exec job: empty command")
		}
		c := exec.CommandContext(ctx, cmd[0], cmd[1:]...)
		c.Stdin = os.Stdin
		c.Stdout = os.Stdout
		var stderr bytes.Buffer
		c.Stderr = io.MultiWriter(os.Stderr, &stderr)
		if err := c.Run(); err != nil {
			if stderr.Len() > 0 {
				return fmt.Errorf("iugum: exec job: %w: %s", err, stderr.String())
			}
			return fmt.Errorf("iugum: exec job: %w", err)
		}
		return nil
	}
}
