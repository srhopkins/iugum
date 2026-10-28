package agentplatform

import (
	"context"
	"errors"
	"github.com/srhopkins/iugum/contract"
)

type acpConfigOption struct {
	ID       string `json:"id"`
	Category string `json:"category"`
	Type     string `json:"type"`
	Current  string `json:"currentValue"`
	Options  []struct {
		Value   string `json:"value"`
		Name    string `json:"name"`
		Options []struct {
			Value string `json:"value"`
			Name  string `json:"name"`
		} `json:"options"`
	} `json:"options"`
}

func (p Provider) Create(ctx context.Context, q contract.AgentSessionCreate, o contract.AgentSessionObserver) (contract.AgentSessionRef, contract.AgentSession, error) {
	if q.Provider != p.Provider || p.Provider != "Codex" && p.Provider != "Cursor" {
		return contract.AgentSessionRef{}, nil, errors.New("provider does not support creation")
	}
	ref := contract.AgentSessionRef{Provider: p.Provider, Cwd: q.Cwd, Title: q.Title}
	client, err := p.connect(ctx, ref, o, true, q.Model)
	if err != nil {
		return ref, nil, err
	}
	return client.(*session).ref, client, nil
}
func (s *session) modelOrNil() any {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.model == "" {
		return nil
	}
	return s.model
}
func (s *session) Models(ctx context.Context) (contract.AgentSessionModels, error) {
	if s.ref.Provider == "Codex" {
		var r struct {
			Data []struct {
				ID, Model, DisplayName string
				IsDefault              bool `json:"isDefault"`
			}
			NextCursor *string `json:"nextCursor"`
		}
		out := contract.AgentSessionModels{Options: []contract.AgentSessionModel{}, CanSelect: true, Reason: "Provider catalog; model access is verified by a successful turn."}
		cursor := ""
		for {
			if err := s.rpc.call(ctx, "model/list", map[string]any{"cursor": cursor}, &r); err != nil {
				return out, err
			}
			for _, v := range r.Data {
				id := v.Model
				if id == "" {
					id = v.ID
				}
				out.Options = append(out.Options, contract.AgentSessionModel{ID: id, Name: v.DisplayName})
			}
			if r.NextCursor == nil || *r.NextCursor == "" {
				break
			}
			if *r.NextCursor == cursor {
				return out, errors.New("repeated model catalog cursor")
			}
			cursor = *r.NextCursor
		}
		s.mu.Lock()
		out.Current = s.model
		s.mu.Unlock()
		return out, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	out := contract.AgentSessionModels{Options: []contract.AgentSessionModel{}, Reason: "Agent did not advertise model config options."}
	for _, c := range s.configOptions {
		if c.Category != "model" || c.Type != "select" {
			continue
		}
		out.Current = c.Current
		out.CanSelect = true
		out.Reason = ""
		for _, v := range c.Options {
			if v.Value != "" {
				out.Options = append(out.Options, contract.AgentSessionModel{ID: v.Value, Name: v.Name})
			}
			for _, v := range v.Options {
				out.Options = append(out.Options, contract.AgentSessionModel{ID: v.Value, Name: v.Name})
			}
		}
		break
	}
	return out, nil
}
func (s *session) SelectModel(ctx context.Context, id string) error {
	models, err := s.Models(ctx)
	if err != nil {
		return err
	}
	valid := false
	for _, v := range models.Options {
		if v.ID == id && id != "" {
			valid = true
		}
	}
	if !valid || !models.CanSelect {
		return errors.New("unknown or unsupported model")
	}
	s.mu.Lock()
	if s.busy {
		s.mu.Unlock()
		return errors.New("session busy")
	}
	if s.ref.Provider == "Codex" {
		s.model = id
		s.mu.Unlock()
		return nil
	}
	configID := ""
	for _, c := range s.configOptions {
		if c.Category == "model" && c.Type == "select" {
			configID = c.ID
			break
		}
	}
	s.mu.Unlock()
	var result struct {
		ConfigOptions []acpConfigOption `json:"configOptions"`
	}
	if err = s.rpc.call(ctx, "session/set_config_option", map[string]any{"sessionId": s.ref.ID, "configId": configID, "value": id}, &result); err != nil {
		return err
	}
	s.mu.Lock()
	s.configOptions = result.ConfigOptions
	s.mu.Unlock()
	return nil
}
