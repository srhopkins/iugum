package hookbus

import (
	"context"
	"errors"
	"testing"

	"github.com/srhopkins/iugum/contract"
)

func TestFireRunsJob(t *testing.T) {
	b := New()
	var got string
	b.Register("echo", func(_ context.Context, ev contract.Event) error {
		got = ev.Path
		return nil
	})
	b.On("watch.changed", "echo")
	if err := b.Fire(context.Background(), contract.Event{Name: "watch.changed", Path: "/tmp/x"}); err != nil {
		t.Fatal(err)
	}
	if got != "/tmp/x" {
		t.Fatalf("got %q", got)
	}
}

func TestWorkflowStopsOnError(t *testing.T) {
	b := New()
	var order []string
	b.Register("ok", func(context.Context, contract.Event) error {
		order = append(order, "ok")
		return nil
	})
	b.Register("bad", func(context.Context, contract.Event) error {
		order = append(order, "bad")
		return errors.New("no")
	})
	b.Register("never", func(context.Context, contract.Event) error {
		order = append(order, "never")
		return nil
	})
	b.AddWorkflow("pipe", []string{"ok", "bad", "never"})
	err := b.Fire(context.Background(), contract.Event{Name: "pipe"})
	if err == nil {
		t.Fatal("want error")
	}
	if len(order) != 2 || order[1] != "bad" {
		t.Fatalf("order %v", order)
	}
}

func TestListenHTTPStub(t *testing.T) {
	err := New().ListenHTTP("127.0.0.1:0")
	var stub contract.HTTPHookStub
	if !errors.As(err, &stub) || stub.Path != "/hooks/{name}" {
		t.Fatalf("%v", err)
	}
}
