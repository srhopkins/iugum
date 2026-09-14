package main

import (
	"bytes"
	"context"
	"errors"
	"github.com/srhopkins/iugum/agentsessions"
	"github.com/srhopkins/iugum/app"
	"github.com/srhopkins/iugum/contract"
	"strings"
	"testing"
)

type sessionTestGate struct {
	requests []contract.Request
	deny     string
}

func (g *sessionTestGate) Enforce(_ context.Context, r contract.Request) error {
	g.requests = append(g.requests, r)
	if r.Act == g.deny {
		return errors.New("denied")
	}
	return nil
}

type sessionFakeClaude struct {
	called  bool
	request agentsessions.ResumeRequest
}

func (c *sessionFakeClaude) Resume(_ context.Context, r agentsessions.ResumeRequest) (agentsessions.ResumeResult, error) {
	c.called = true
	c.request = r
	return agentsessions.ResumeResult{Output: "synthetic"}, nil
}
func TestAgentSessionResumeGate(t *testing.T) {
	gate := &sessionTestGate{}
	a := &app.App{Actor: "agent:chief", Gate: gate}
	fake := &sessionFakeClaude{}
	var out, errout bytes.Buffer
	args := []string{"resume", "--session", "abc", "--project", t.TempDir(), "--account", "personal", "--config-dir", t.TempDir(), "--prompt", "synthetic test"}
	if code := runAgentSessionWith(context.Background(), a, args, &out, &errout, fake); code != 0 {
		t.Fatal(code, errout.String())
	}
	if !fake.called || len(gate.requests) != 3 || gate.requests[0].Sub != "agent:chief" || gate.requests[0].Obj != "transcript:claude:personal" {
		t.Fatal(gate.requests)
	}
	gate.deny = "execute"
	fake.called = false
	if code := runAgentSessionWith(context.Background(), a, args, &out, &errout, fake); code != 1 || fake.called {
		t.Fatal("dispatch after denial")
	}
}
func TestAgentSessionCapabilities(t *testing.T) {
	a := &app.App{Actor: "human", Gate: &sessionTestGate{}}
	var out, errout bytes.Buffer
	if code := runAgentSession(context.Background(), a, []string{"capabilities"}, &out, &errout); code != 0 || !strings.Contains(out.String(), `"running_session_steer":false`) {
		t.Fatal(code, out.String())
	}
	fake := &sessionFakeClaude{}
	if code := runAgentSessionWith(context.Background(), a, []string{"resume", "--session", "abc"}, &out, &errout, fake); code != 2 || fake.called {
		t.Fatal("missing arguments dispatched")
	}
}
