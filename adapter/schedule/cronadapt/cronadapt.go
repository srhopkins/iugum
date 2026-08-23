package cronadapt

import (
	"context"

	cron "github.com/netresearch/go-cron"

	"github.com/srhopkins/iugum/contract"
	"github.com/srhopkins/iugum/plugin"
)

func init() {
	plugin.RegisterScheduler("cron", func(map[string]string) (contract.Scheduler, error) {
		return New(), nil
	})
}

// Scheduler wraps netresearch/go-cron. Spec is 5-field cron, @every, or @triggered.
type Scheduler struct {
	c *cron.Cron
}

func New() *Scheduler {
	return &Scheduler{c: cron.New()}
}

func (s *Scheduler) Name() string { return "cron" }

func (s *Scheduler) Add(spec, jobName string, fn contract.JobFunc) error {
	if spec == "" {
		spec = "@triggered"
	}
	_, err := s.c.AddFunc(spec, func() {
		_ = fn(context.Background(), contract.Event{Name: jobName, Source: "cron"})
	}, cron.WithName(jobName))
	return err
}

func (s *Scheduler) Trigger(jobName string) error {
	s.c.TriggerEntryByName(jobName)
	return nil
}

func (s *Scheduler) Start() error {
	s.c.Start()
	return nil
}

func (s *Scheduler) Stop() error {
	s.c.StopAndWait()
	return nil
}
