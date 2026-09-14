package agentdesk

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Evidence struct {
	Label  string `json:"label"`
	Text   string `json:"text,omitempty"`
	Path   string `json:"path,omitempty"`
	Digest string `json:"digest,omitempty"`
}
type ApprovalRequest struct {
	Title    string          `json:"title"`
	Action   string          `json:"action"`
	Payload  json.RawMessage `json:"payload,omitempty"`
	Evidence []Evidence      `json:"evidence"`
}
type Approval struct {
	ID string `json:"id"`
	ApprovalRequest
	Digest   string    `json:"digest"`
	Status   string    `json:"status"`
	Created  time.Time `json:"created"`
	Resolved time.Time `json:"resolved,omitempty"`
	Result   string    `json:"result,omitempty"`
}

func digestBytes(b []byte) string { sum := sha256.Sum256(b); return hex.EncodeToString(sum[:]) }
func (s *Server) approvalsLocked() ([]Approval, error) {
	b, e := os.ReadFile(filepath.Join(s.cfg.DataDir, "approvals.json"))
	if os.IsNotExist(e) {
		return []Approval{}, nil
	}
	if e != nil {
		return nil, e
	}
	var items []Approval
	e = json.Unmarshal(b, &items)
	return items, e
}
func (s *Server) saveApprovalsLocked(items []Approval) error {
	b, e := json.MarshalIndent(items, "", "  ")
	if e != nil {
		return e
	}
	f, e := os.CreateTemp(s.cfg.DataDir, ".approvals-*")
	if e != nil {
		return e
	}
	defer os.Remove(f.Name())
	if _, e = f.Write(b); e != nil {
		f.Close()
		return e
	}
	if e = f.Sync(); e != nil {
		f.Close()
		return e
	}
	if e = f.Close(); e != nil {
		return e
	}
	return os.Rename(f.Name(), filepath.Join(s.cfg.DataDir, "approvals.json"))
}

// RequestApproval snapshots reviewable content. File evidence is rechecked before execution.
func (s *Server) RequestApproval(ctx context.Context, r ApprovalRequest) (Approval, error) {
	if r.Title == "" || r.Action == "" {
		return Approval{}, errors.New("approval requires title and action")
	}
	if s.cfg.Check != nil {
		if e := s.cfg.Check(ctx, "agentdesk/api/approvals", "write"); e != nil {
			return Approval{}, e
		}
	}
	for i := range r.Evidence {
		v := &r.Evidence[i]
		if v.Path != "" {
			if s.cfg.Check != nil {
				if e := s.cfg.Check(ctx, "file:"+v.Path, "read"); e != nil {
					return Approval{}, e
				}
			}
			b, e := os.ReadFile(v.Path)
			if e != nil {
				return Approval{}, e
			}
			v.Digest = digestBytes(b)
			if v.Text == "" {
				if len(b) <= 64000 {
					v.Text = string(b)
				} else {
					v.Text = string(b[:64000]) + "\n[Preview truncated; review full file at the evidence path.]"
				}
			}
		}
	}
	b, e := json.Marshal(r)
	if e != nil {
		return Approval{}, e
	}
	a := Approval{ID: fmt.Sprintf("approval-%d", time.Now().UnixNano()), ApprovalRequest: r, Digest: digestBytes(b), Status: "pending", Created: time.Now()}
	s.mu.Lock()
	defer s.mu.Unlock()
	items, e := s.approvalsLocked()
	if e != nil {
		return a, e
	}
	items = append(items, a)
	return a, s.saveApprovalsLocked(items)
}
func (s *Server) approvalHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" && r.URL.Path == "/api/approvals" {
		s.mu.Lock()
		defer s.mu.Unlock()
		items, e := s.approvalsLocked()
		if e != nil {
			fail(w, e, 500)
			return
		}
		respond(w, items)
		return
	}
	if r.Method != "POST" {
		http.NotFound(w, r)
		return
	}
	var in struct {
		Decision string `json:"decision"`
		Digest   string `json:"digest"`
	}
	if e := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&in); e != nil {
		fail(w, e, 400)
		return
	}
	if in.Decision != "approve" && in.Decision != "reject" {
		fail(w, errors.New("decision must be approve or reject"), 400)
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/approvals/")
	s.mu.Lock()
	items, e := s.approvalsLocked()
	if e != nil {
		s.mu.Unlock()
		fail(w, e, 500)
		return
	}
	idx := -1
	for i := range items {
		if items[i].ID == id {
			idx = i
			break
		}
	}
	if idx < 0 {
		s.mu.Unlock()
		http.NotFound(w, r)
		return
	}
	a := items[idx]
	if a.Status != "pending" || a.Digest != in.Digest {
		s.mu.Unlock()
		fail(w, errors.New("approval is stale or already resolved"), 409)
		return
	}
	if in.Decision == "approve" {
		for _, v := range a.Evidence {
			if v.Path != "" {
				if s.cfg.Check != nil {
					if e = s.cfg.Check(r.Context(), "file:"+v.Path, "read"); e != nil {
						break
					}
				}
				var b []byte
				b, e = os.ReadFile(v.Path)
				if e != nil || digestBytes(b) != v.Digest {
					e = errors.New("evidence changed; request a new approval")
					break
				}
			}
		}
		if e != nil {
			s.mu.Unlock()
			fail(w, e, 409)
			return
		}
		if s.cfg.ApprovalExecute == nil {
			s.mu.Unlock()
			fail(w, errors.New("approval executor is not configured"), 503)
			return
		}
		a.Status = "executing"
	} else {
		a.Status = "rejected"
		a.Resolved = time.Now()
	}
	items[idx] = a
	e = s.saveApprovalsLocked(items)
	s.mu.Unlock()
	if e != nil {
		fail(w, e, 500)
		return
	}
	if in.Decision == "approve" {
		result, execErr := s.cfg.ApprovalExecute(r.Context(), a)
		a.Result = result
		a.Resolved = time.Now()
		a.Status = "complete"
		if execErr != nil {
			a.Status = "failed"
			a.Result = execErr.Error()
		}
		s.mu.Lock()
		items, e = s.approvalsLocked()
		if e == nil {
			for i := range items {
				if items[i].ID == id {
					items[i] = a
				}
			}
			e = s.saveApprovalsLocked(items)
		}
		s.mu.Unlock()
		if e != nil {
			fail(w, e, 500)
			return
		}
	}
	respond(w, a)
}
