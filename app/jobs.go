package app

import (
	"context"
	"fmt"

	jobexec "github.com/srhopkins/iugum/adapter/job"
	"github.com/srhopkins/iugum/config"
	"github.com/srhopkins/iugum/contract"
)

func (a *App) registerExecJobs(jobs []config.JobSpec) {
	a.registerJobBodies(jobs)
}

func (a *App) registerHTTPJobs(jobs []config.JobSpec) {
	// registerJobBodies covers http, exec, and session.
}

func (a *App) registerJobBodies(jobs []config.JobSpec) {
	for _, j := range jobs {
		a.registerJobBody(j)
	}
}

func (a *App) registerJobBody(j config.JobSpec) {
	switch j.Kind {
	case "exec":
		if len(j.Command) > 0 {
			a.Hooks.Register(j.Name, jobexec.Exec(j.Command))
		}
	case "http":
		if j.URL != "" {
			a.Hooks.Register(j.Name, jobexec.HTTPPost(j.URL))
		}
	case "session":
		if j.Prompt != "" {
			a.Hooks.Register(j.Name, jobexec.SessionPrompt(j.Prompt, j.Timeout, j.IdleTimeout))
		}
	}
}

func (a *App) rememberJobs(jobs []config.JobSpec) {
	a.jobsMu.Lock()
	defer a.jobsMu.Unlock()
	if a.knownJobs == nil {
		a.knownJobs = map[string]config.JobSpec{}
	}
	for _, j := range jobs {
		if j.Name != "" {
			a.knownJobs[j.Name] = j
		}
	}
}

// SyncJobs registers any job that is not already known. Used when jobs.yaml changes.
func (a *App) SyncJobs(jobs []config.JobSpec) error {
	a.jobsMu.Lock()
	defer a.jobsMu.Unlock()
	if a.knownJobs == nil {
		a.knownJobs = map[string]config.JobSpec{}
	}
	for _, j := range jobs {
		if j.Name == "" {
			continue
		}
		if _, ok := a.knownJobs[j.Name]; ok {
			continue
		}
		a.registerJobBody(j)
		spec := j.Spec
		if spec == "" {
			spec = "@triggered"
		}
		name := j.Name
		if err := a.Scheduler.Add(spec, name, func(ctx context.Context, ev contract.Event) error {
			return a.FireHook(ctx, contract.Event{Name: name, Source: "cron", Path: ev.Path, Attrs: ev.Attrs})
		}); err != nil {
			return err
		}
		a.knownJobs[j.Name] = j
	}
	return nil
}

// KnownJobs is a copy of jobs loaded or synced so far.
func (a *App) KnownJobs() []config.JobSpec {
	a.jobsMu.Lock()
	defer a.jobsMu.Unlock()
	out := make([]config.JobSpec, 0, len(a.knownJobs))
	for _, j := range a.knownJobs {
		out = append(out, j)
	}
	return out
}

// RunJobBody runs one job kind now (exec, http, or session). Used by `iugum job run`.
func RunJobBody(ctx context.Context, j config.JobSpec) error {
	var fn contract.JobFunc
	switch j.Kind {
	case "exec":
		fn = jobexec.Exec(j.Command)
	case "http":
		fn = jobexec.HTTPPost(j.URL)
	case "session":
		fn = jobexec.SessionPrompt(j.Prompt, j.Timeout, j.IdleTimeout)
	default:
		return fmt.Errorf("iugum: job %q: kind %q cannot run from the CLI", j.Name, j.Kind)
	}
	return fn(ctx, contract.Event{Name: j.Name, Source: "cli"})
}
