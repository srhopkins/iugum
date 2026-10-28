package agentplatform

import (
	"context"
	"errors"
	"github.com/srhopkins/iugum/contract"
	"strings"
)

func (p OpenCodeHTTPProvider) Create(ctx context.Context, q contract.AgentSessionCreate, o contract.AgentSessionObserver) (contract.AgentSessionRef, contract.AgentSession, error) {
	ref := contract.AgentSessionRef{Provider: "OpenCode", Cwd: q.Cwd, Title: q.Title}
	if q.Provider != "OpenCode" {
		return ref, nil, errors.New("provider mismatch")
	}
	s, err := p.connection(ctx, ref, o)
	if err != nil {
		return ref, nil, err
	}
	defer s.Close()
	if q.Model != "" {
		if err = s.SelectModel(ctx, q.Model); err != nil {
			return ref, nil, err
		}
	}
	var info struct{ ID, Directory, Title string }
	if err = s.call(ctx, "POST", "/session", map[string]string{"title": q.Title}, &info); err != nil {
		return ref, nil, err
	}
	ref.ID = info.ID
	if info.Directory != q.Cwd {
		return ref, nil, errors.New("created session directory mismatch")
	}
	if err = validRef(ref); err != nil {
		return ref, nil, err
	}
	client, err := p.Open(ctx, ref, o)
	if err == nil {
		client.(*openCodeHTTPSession).model = q.Model
	}
	return ref, client, err
}
func (s *openCodeHTTPSession) Models(ctx context.Context) (contract.AgentSessionModels, error) {
	var result struct {
		All []struct {
			ID, Name string
			Models   map[string]struct{ ID, Name string }
		}
		Connected []string
	}
	out := contract.AgentSessionModels{Options: []contract.AgentSessionModel{}, CanSelect: true}
	if err := s.call(ctx, "GET", "/provider", nil, &result); err != nil {
		return out, err
	}
	connected := map[string]bool{}
	for _, id := range result.Connected {
		connected[id] = true
	}
	for _, p := range result.All {
		if !connected[p.ID] {
			continue
		}
		for key, m := range p.Models {
			out.Options = append(out.Options, contract.AgentSessionModel{ID: p.ID + "/" + key, Name: m.Name})
		}
	}
	s.mu.Lock()
	out.Current = s.model
	s.mu.Unlock()
	if out.Current == "" {
		out.Reason = "Uses provider default until a model is selected for the next prompt."
	}
	return out, nil
}
func (s *openCodeHTTPSession) SelectModel(ctx context.Context, id string) error {
	if !strings.Contains(id, "/") {
		return errors.New("provider/model identity required")
	}
	models, err := s.Models(ctx)
	if err != nil {
		return err
	}
	valid := false
	for _, m := range models.Options {
		if m.ID == id {
			valid = true
		}
	}
	if !valid {
		return errors.New("unknown or disconnected model")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.done != nil {
		return errors.New("session busy")
	}
	s.model = id
	return nil
}
