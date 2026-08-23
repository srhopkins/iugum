package app

import (
	"context"
	"strings"
	"testing"

	"github.com/srhopkins/iugum/config"
	"github.com/srhopkins/iugum/contract"
)

func TestE2EExecJobTrue(t *testing.T) {
	cfg := e2eConfig(t)
	cfg.Jobs = []config.JobSpec{
		{Name: "noop", Kind: "exec", Command: []string{"true"}},
	}
	a := newE2EApp(t, cfg)

	if err := a.FireHook(context.Background(), contract.Event{Name: "noop"}); err != nil {
		t.Fatal(err)
	}
}

func TestE2EExecJobEcho(t *testing.T) {
	cfg := e2eConfig(t)
	cfg.Jobs = []config.JobSpec{
		{Name: "say", Kind: "exec", Command: []string{"echo", "iugum-e2e-ok"}},
	}
	a := newE2EApp(t, cfg)

	if err := a.FireHook(context.Background(), contract.Event{Name: "say"}); err != nil {
		t.Fatal(err)
	}
}

func TestE2EExecJobFails(t *testing.T) {
	cfg := e2eConfig(t)
	cfg.Jobs = []config.JobSpec{
		{Name: "fail", Kind: "exec", Command: []string{"false"}},
	}
	a := newE2EApp(t, cfg)

	err := a.FireHook(context.Background(), contract.Event{Name: "fail"})
	if err == nil {
		t.Fatal("want exec failure")
	}
	if !strings.Contains(err.Error(), "exec job") {
		t.Fatalf("unexpected error: %v", err)
	}
}
