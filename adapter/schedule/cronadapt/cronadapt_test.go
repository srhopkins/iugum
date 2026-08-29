package cronadapt

import (
	"context"
	"testing"
	"time"

	"github.com/srhopkins/iugum/contract"
)

func TestTriggerAdhoc(t *testing.T) {
	s := New()
	ran := make(chan struct{}, 1)
	if err := s.Add("@triggered", "ping", func(context.Context, contract.Event) error {
		ran <- struct{}{}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.Start(); err != nil {
		t.Fatal(err)
	}
	defer s.Stop()
	if err := s.Trigger("ping"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-ran:
	case <-time.After(2 * time.Second):
		t.Fatal("job did not run")
	}
}

func TestAddAfterStart(t *testing.T) {
	s := New()
	if err := s.Start(); err != nil {
		t.Fatal(err)
	}
	defer s.Stop()
	ran := make(chan struct{}, 1)
	if err := s.Add("@triggered", "late", func(context.Context, contract.Event) error {
		ran <- struct{}{}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.Trigger("late"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-ran:
	case <-time.After(2 * time.Second):
		t.Fatal("live-added job did not run")
	}
}
