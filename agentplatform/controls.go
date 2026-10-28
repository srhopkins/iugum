package agentplatform

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/srhopkins/iugum/contract"
	"os"
	"path/filepath"
)

func sessionModels(ctx context.Context, m *managed) (contract.AgentSessionModels, error) {
	if c, ok := m.client.(contract.AgentSessionModelController); ok {
		return c.Models(ctx)
	}
	return contract.AgentSessionModels{Options: []contract.AgentSessionModel{}, Reason: "Provider does not expose model controls."}, nil
}
func (s *Server) selectModel(ctx context.Context, m *managed, id string) (any, error) {
	if s.cfg.Check != nil {
		if err := s.cfg.Check(ctx, "agent/sessions/"+m.ref.Provider+"/"+m.ref.ID, "select-model"); err != nil {
			return nil, errors.New("model policy denied")
		}
	}
	m.mu.Lock()
	if m.busy {
		m.mu.Unlock()
		return nil, errors.New("cannot change model during a turn")
	}
	m.busy = true
	m.mu.Unlock()
	defer func() { m.mu.Lock(); m.busy = false; m.mu.Unlock() }()
	c, ok := m.client.(contract.AgentSessionModelController)
	if !ok {
		return nil, errors.New("provider does not support model selection")
	}
	models, err := c.Models(ctx)
	if err != nil {
		return nil, err
	}
	valid := false
	for _, v := range models.Options {
		if v.ID == id && id != "" {
			valid = true
		}
	}
	if !valid || !models.CanSelect {
		return nil, errors.New("unknown or unsupported model")
	}
	if err = c.SelectModel(ctx, id); err != nil {
		return nil, err
	}
	b, _ := json.Marshal(struct {
		Ref   contract.AgentSessionRef `json:"ref"`
		Model string                   `json:"model"`
	}{m.ref, id})
	if err = atomicFile(filepath.Join(s.cfg.DataDir, "model-"+m.ref.Provider+"-"+m.ref.ID+".json"), b); err != nil {
		return nil, err
	}
	return c.Models(ctx)
}
func (s *Server) createSession(ctx context.Context, q contract.AgentSessionCreate) (any, error) {
	p := s.providers[q.Provider]
	if p == nil {
		return nil, errors.New("provider not configured")
	}
	creator, ok := p.(contract.AgentSessionCreator)
	if !ok {
		return nil, errors.New("provider does not support explicit creation")
	}
	if !filepath.IsAbs(q.Cwd) {
		return nil, errors.New("absolute working directory required")
	}
	if st, err := os.Stat(q.Cwd); err != nil || !st.IsDir() {
		return nil, errors.New("working directory does not exist")
	}
	if s.cfg.Check != nil {
		if err := s.cfg.Check(ctx, "agent/sessions/"+q.Provider, "create"); err != nil {
			return nil, errors.New("create policy denied")
		}
	}
	s.mu.Lock()
	if s.closed || len(s.sessions)+len(s.opening) >= 32 {
		s.mu.Unlock()
		return nil, errors.New("session service closed or owner limit reached")
	}
	reservation := "create:" + newControlID()
	s.opening[reservation] = true
	s.wg.Add(1)
	s.mu.Unlock()
	defer s.wg.Done()
	defer func() { s.mu.Lock(); delete(s.opening, reservation); s.mu.Unlock() }()
	m := &managed{done: s.ctx, requests: map[string]*approval{}, state: "Creating"}
	ref, client, err := creator.Create(s.ctx, q, m)
	if err != nil {
		if client != nil {
			client.Close()
		}
		return nil, err
	}
	if client == nil {
		return nil, errors.New("provider returned no session")
	}
	if err = validRef(ref); err != nil || ref.Provider != q.Provider || ref.Cwd != q.Cwd {
		client.Close()
		return nil, errors.New("created session identity mismatch")
	}
	m.ref = ref
	m.client = client
	m.ready = true
	m.state = "Ready"
	s.mu.Lock()
	defer s.mu.Unlock()
	key := ref.Provider + ":" + ref.ID
	if s.closed || s.sessions[key] != nil {
		client.Close()
		return nil, errors.New("created session conflicts with existing owner")
	}
	// Persist native identity before exposing this owner. Resume never falls back to creation.
	b, _ := json.Marshal(ref)
	if err = atomicFile(filepath.Join(s.cfg.DataDir, "created-"+ref.Provider+"-"+ref.ID+".json"), b); err != nil {
		client.Close()
		return nil, err
	}
	if q.Model != "" && ref.Provider != "Claude" {
		modelBytes, _ := json.Marshal(struct {
			Ref   contract.AgentSessionRef `json:"ref"`
			Model string                   `json:"model"`
		}{ref, q.Model})
		if err = atomicFile(filepath.Join(s.cfg.DataDir, "model-"+ref.Provider+"-"+ref.ID+".json"), modelBytes); err != nil {
			client.Close()
			return nil, err
		}
	}
	s.sessions[key] = m
	s.selected[ref.Provider] = ref.ID
	return map[string]any{"agent": providerRoute(ref.Provider), "session": ref}, nil
}

func newControlID() string {
	var id [16]byte
	_, _ = rand.Read(id[:])
	return hex.EncodeToString(id[:])
}
func (s *Server) restoreModel(ctx context.Context, m *managed) error {
	b, err := os.ReadFile(filepath.Join(s.cfg.DataDir, "model-"+m.ref.Provider+"-"+m.ref.ID+".json"))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	var saved struct {
		Ref   contract.AgentSessionRef `json:"ref"`
		Model string                   `json:"model"`
	}
	if err = json.Unmarshal(b, &saved); err != nil {
		return err
	}
	if saved.Ref.Provider != m.ref.Provider || saved.Ref.ID != m.ref.ID || saved.Ref.Cwd != m.ref.Cwd {
		return errors.New("saved model identity mismatch")
	}
	c, ok := m.client.(contract.AgentSessionModelController)
	if !ok {
		return errors.New("saved model unsupported by provider")
	}
	models, err := c.Models(ctx)
	if err != nil {
		return err
	}
	if !models.CanSelect && models.Current == saved.Model {
		return nil
	}
	return c.SelectModel(ctx, saved.Model)
}
