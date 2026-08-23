package job

import (
	"context"
	"strings"
	"testing"

	"github.com/srhopkins/iugum/contract"
)

func TestExecEmptyCommand(t *testing.T) {
	fn := Exec(nil)
	err := fn(context.Background(), contract.Event{})
	if err == nil || !strings.Contains(err.Error(), "empty command") {
		t.Fatalf("want empty command error, got %v", err)
	}
}

func TestExecTrue(t *testing.T) {
	fn := Exec([]string{"true"})
	if err := fn(context.Background(), contract.Event{}); err != nil {
		t.Fatal(err)
	}
}

func TestExecEcho(t *testing.T) {
	fn := Exec([]string{"echo", "iugum-exec-ok"})
	if err := fn(context.Background(), contract.Event{}); err != nil {
		t.Fatal(err)
	}
}
