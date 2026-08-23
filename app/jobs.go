package app

import (
	jobexec "github.com/srhopkins/iugum/adapter/job"
	"github.com/srhopkins/iugum/config"
)

func (a *App) registerExecJobs(jobs []config.JobSpec) {
	for _, j := range jobs {
		if j.Kind != "exec" || len(j.Command) == 0 {
			continue
		}
		a.Hooks.Register(j.Name, jobexec.Exec(j.Command))
	}
}
