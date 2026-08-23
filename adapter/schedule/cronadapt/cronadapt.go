package cronadapt

import (
	"context"
	"fmt"
	"strings"

	cron "github.com/netresearch/go-cron"

	"github.com/srhopkins/iugum/contract"
	"github.com/srhopkins/iugum/plugin"
)

func init() {
	plugin.RegisterScheduler("cron", func(map[string]string) (contract.Scheduler, error) {
		return New(), nil
	})
}

type pendingJob struct {
	spec string
	name string
	fn   contract.JobFunc
}

type pendingDep struct {
	child  string
	parent string
	on     string
}

// Scheduler wraps netresearch/go-cron. Spec is 5-field cron, @every, or @triggered.
type Scheduler struct {
	c       *cron.Cron
	pending []pendingJob
	deps    []pendingDep
	dagJobs map[string]struct{}
	started bool
}

func New() *Scheduler {
	return &Scheduler{
		c:       cron.New(),
		dagJobs: map[string]struct{}{},
	}
}

func newWithClock(clock cron.Clock) *Scheduler {
	return &Scheduler{
		c:       cron.New(cron.WithClock(clock)),
		dagJobs: map[string]struct{}{},
	}
}

func (s *Scheduler) Name() string { return "cron" }

func (s *Scheduler) Add(spec, jobName string, fn contract.JobFunc) error {
	if s.started {
		return fmt.Errorf("cronadapt: Add after Start")
	}
	if spec == "" {
		spec = "@triggered"
	}
	s.pending = append(s.pending, pendingJob{spec: spec, name: jobName, fn: fn})
	return nil
}

func (s *Scheduler) AddAfter(childName, parentName, on string) error {
	if s.started {
		return fmt.Errorf("cronadapt: AddAfter after Start")
	}
	if _, err := parseOn(on); err != nil {
		return err
	}
	s.deps = append(s.deps, pendingDep{child: childName, parent: parentName, on: on})
	s.dagJobs[childName] = struct{}{}
	s.dagJobs[parentName] = struct{}{}
	return nil
}

func (s *Scheduler) wrapJob(name string, fn contract.JobFunc) cron.Job {
	run := func() error {
		return fn(context.Background(), contract.Event{Name: name, Source: "cron"})
	}
	if _, inDAG := s.dagJobs[name]; inDAG {
		return cron.FuncErrorJob(run)
	}
	return cron.FuncJob(func() { _ = run() })
}

func (s *Scheduler) installPending() error {
	for _, p := range s.pending {
		if _, err := s.c.AddJob(p.spec, s.wrapJob(p.name, p.fn), cron.WithName(p.name)); err != nil {
			return err
		}
	}
	for _, d := range s.deps {
		cond, err := parseOn(d.on)
		if err != nil {
			return err
		}
		if err := s.c.AddDependencyByName(d.child, d.parent, cond); err != nil {
			return err
		}
	}
	return nil
}

func parseOn(on string) (cron.TriggerCondition, error) {
	switch strings.ToLower(strings.TrimSpace(on)) {
	case "", "success":
		return cron.OnSuccess, nil
	case "failure":
		return cron.OnFailure, nil
	case "skipped":
		return cron.OnSkipped, nil
	case "complete":
		return cron.OnComplete, nil
	default:
		return 0, fmt.Errorf("cronadapt: unknown on %q (want success, failure, skipped, complete)", on)
	}
}

func (s *Scheduler) Trigger(jobName string) error {
	s.c.TriggerEntryByName(jobName)
	return nil
}

func (s *Scheduler) Start() error {
	if !s.started {
		if err := s.installPending(); err != nil {
			return err
		}
		s.started = true
	}
	s.c.Start()
	return nil
}

func (s *Scheduler) Stop() error {
	s.c.StopAndWait()
	return nil
}
