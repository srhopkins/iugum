package app

import (
	"context"
	"fmt"

	"github.com/srhopkins/iugum/config"
	"github.com/srhopkins/iugum/contract"
)

// RegisterJobs wires config jobs into the scheduler and hook workflows.
// Linear workflow steps use Hooks.AddWorkflow. after + on use go-cron
// AddDependencyByName (OnSuccess and siblings).
func RegisterJobs(a *App, jobs []config.JobSpec) error {
	for _, j := range jobs {
		if len(j.Workflow) > 0 {
			a.Hooks.AddWorkflow(j.Name, j.Workflow)
		}
	}
	for _, j := range jobs {
		spec := j.Spec
		if spec == "" {
			spec = "@triggered"
		}
		name := j.Name
		if err := a.Scheduler.Add(spec, name, func(ctx context.Context, ev contract.Event) error {
			return a.FireHook(ctx, contract.Event{Name: name, Source: "cron", Path: ev.Path, Attrs: ev.Attrs})
		}); err != nil {
			return fmt.Errorf("schedule %q: %w", name, err)
		}
	}
	for _, j := range jobs {
		if len(j.After) == 0 {
			continue
		}
		on := j.On
		if on == "" {
			on = "success"
		}
		for _, parent := range j.After {
			if err := a.Scheduler.AddAfter(j.Name, parent, on); err != nil {
				return fmt.Errorf("after %q depends on %q: %w", j.Name, parent, err)
			}
		}
	}
	return nil
}
