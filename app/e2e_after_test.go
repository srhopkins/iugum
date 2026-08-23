package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/srhopkins/iugum/config"
	"github.com/srhopkins/iugum/contract"
)

func startScheduled(t *testing.T, a *App, jobs []config.JobSpec) {
	t.Helper()
	if err := RegisterJobs(a, jobs); err != nil {
		t.Fatal(err)
	}
	if err := a.Scheduler.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = a.Scheduler.Stop() })
}

func TestE2EAfterSuccessRunsChild(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "step-b.ran")
	cfg := e2eConfig(t)
	cfg.Jobs = []config.JobSpec{
		{Name: "step-a", Kind: "exec", Spec: "@triggered", Command: []string{"true"}},
		{Name: "step-b", Kind: "exec", Spec: "@triggered", After: []string{"step-a"}, On: "success", Command: []string{"touch", marker}},
	}
	a := newE2EApp(t, cfg)
	startScheduled(t, a, cfg.Jobs)

	if err := a.TriggerJob(context.Background(), "step-a"); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(marker); err == nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("step-b did not run after step-a success")
}

func TestE2EAfterFailureSkipsChild(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "step-b.ran")
	cfg := e2eConfig(t)
	cfg.Jobs = []config.JobSpec{
		{Name: "step-a", Kind: "exec", Spec: "@triggered", Command: []string{"false"}},
		{Name: "step-b", Kind: "exec", Spec: "@triggered", After: []string{"step-a"}, On: "success", Command: []string{"touch", marker}},
	}
	a := newE2EApp(t, cfg)
	startScheduled(t, a, cfg.Jobs)

	_ = a.TriggerJob(context.Background(), "step-a")

	time.Sleep(300 * time.Millisecond)
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("step-b ran after step-a failure")
	} else if !os.IsNotExist(err) {
		t.Fatal(err)
	}
}

func TestE2EWorkflowStillLinear(t *testing.T) {
	cfg := e2eConfig(t)
	cfg.Jobs = []config.JobSpec{
		{Name: "pipe", Spec: "@triggered", Workflow: []string{"ok", "bad", "never"}},
	}
	a := newE2EApp(t, cfg)

	var order []string
	a.Hooks.Register("ok", func(context.Context, contract.Event) error {
		order = append(order, "ok")
		return nil
	})
	a.Hooks.Register("bad", func(context.Context, contract.Event) error {
		order = append(order, "bad")
		return errors.New("stop")
	})
	a.Hooks.Register("never", func(context.Context, contract.Event) error {
		order = append(order, "never")
		return nil
	})

	startScheduled(t, a, cfg.Jobs)
	_ = a.TriggerJob(context.Background(), "pipe")

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && len(order) < 2 {
		time.Sleep(10 * time.Millisecond)
	}
	if len(order) != 2 || order[1] != "bad" {
		t.Fatalf("order %v", order)
	}
}
