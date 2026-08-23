package cronadapt

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/srhopkins/iugum/contract"
)

func TestAfterOnSuccess(t *testing.T) {
	s := New()
	order := make(chan string, 4)

	if err := s.Add("@triggered", "a", func(context.Context, contract.Event) error {
		order <- "a"
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.Add("@triggered", "b", func(context.Context, contract.Event) error {
		order <- "b"
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.AddAfter("b", "a", "success"); err != nil {
		t.Fatal(err)
	}

	if err := s.Start(); err != nil {
		t.Fatal(err)
	}
	defer s.Stop()

	if err := s.Trigger("a"); err != nil {
		t.Fatal(err)
	}

	for _, want := range []string{"a", "b"} {
		select {
		case got := <-order:
			if got != want {
				t.Fatalf("got %q want %q", got, want)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("timeout waiting for %q", want)
		}
	}
}

func TestAfterSkipsOnParentFailure(t *testing.T) {
	s := New()
	ran := make(chan string, 4)

	if err := s.Add("@triggered", "a", func(context.Context, contract.Event) error {
		ran <- "a"
		return errors.New("fail")
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.Add("@triggered", "b", func(context.Context, contract.Event) error {
		ran <- "b"
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.AddAfter("b", "a", "success"); err != nil {
		t.Fatal(err)
	}

	if err := s.Start(); err != nil {
		t.Fatal(err)
	}
	defer s.Stop()

	if err := s.Trigger("a"); err != nil {
		t.Fatal(err)
	}

	select {
	case got := <-ran:
		if got != "a" {
			t.Fatalf("first %q want a", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for a")
	}

	select {
	case got := <-ran:
		t.Fatalf("b should not run, got %q", got)
	case <-time.After(200 * time.Millisecond):
	}
}

func TestParseOnUnknown(t *testing.T) {
	if _, err := parseOn("maybe"); err == nil {
		t.Fatal("want error for unknown on")
	}
}
