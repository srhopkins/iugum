package app

import (
	"context"
	"testing"

	"github.com/srhopkins/iugum/config"
	"github.com/srhopkins/iugum/contract"
	_ "github.com/srhopkins/iugum/defaults"
)

func TestRegisterExecJob(t *testing.T) {
	t.Setenv("IUGUM_DATA", t.TempDir())
	cfg := config.Defaults()
	cfg.Jobs = []config.JobSpec{
		{
			Name:    "ping",
			Kind:    "exec",
			Command: []string{"true"},
		},
	}
	a, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.FireHook(context.Background(), contract.Event{Name: "ping"}); err != nil {
		t.Fatal(err)
	}
}
