// Package agentdocuments stores human-authoritative work in Atomdown Markdown.
package agentdocuments

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	atom "github.com/srhopkins/atomdown"
)

type Commitment struct {
	ID       string    `json:"id"`
	AtomID   string    `json:"atom_id"`
	Title    string    `json:"title"`
	Due      string    `json:"due,omitempty"`
	Done     bool      `json:"done"`
	Deferred bool      `json:"deferred"`
	Focus    bool      `json:"focus"`
	Updated  time.Time `json:"updated"`
}
type Store struct {
	Path string
	mu   sync.Mutex
}

func Open(path string) (*Store, error) {
	if path == "" {
		return nil, errors.New("commitments path required")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	s := &Store{Path: path}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		if err = s.write([]byte("# Commitments\n\n<!-- <atomdown version=\"1\"/> -->\n")); err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	}
	_, err := s.List()
	return s, err
}
func (s *Store) read() ([]byte, atom.Document, error) {
	b, e := os.ReadFile(s.Path)
	if e != nil {
		return nil, atom.Document{}, e
	}
	d := atom.Parse(b)
	if d.HasErrors() {
		return nil, d, fmt.Errorf("invalid Atomdown in %s: %v", s.Path, d.Diagnostics)
	}
	return b, d, nil
}
func attr(a atom.Atom, name string) string {
	for _, v := range a.Attributes {
		if v.Name == name {
			return v.Value
		}
	}
	return ""
}
func decode(a atom.Atom) (Commitment, error) {
	c := Commitment{ID: a.ID, AtomID: a.ID}
	if alias := attr(a, "iugum-ref"); alias != "" {
		c.ID = alias
	}
	for _, line := range strings.Split(a.Text, "\n") {
		k, v, ok := strings.Cut(strings.TrimSpace(line), ":")
		if !ok {
			continue
		}
		v = strings.TrimSpace(v)
		switch k {
		case "Title":
			c.Title = v
		case "Due":
			if v != "none" {
				c.Due = v
			}
		case "Status":
			switch v {
			case "done":
				c.Done = true
			case "deferred":
				c.Deferred = true
			case "focus":
				c.Focus = true
			case "open":
			default:
				return c, fmt.Errorf("invalid commitment status %q", v)
			}
		case "Updated":
			c.Updated, _ = time.Parse(time.RFC3339Nano, v)
		}
	}
	if c.Title == "" {
		return c, errors.New("commitment requires visible Title")
	}
	if c.Due != "" {
		if _, e := time.Parse("2006-01-02", c.Due); e != nil {
			return c, errors.New("Due must be YYYY-MM-DD or none")
		}
	}
	return c, nil
}
func (s *Store) List() ([]Commitment, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, d, e := s.read()
	if e != nil {
		return nil, e
	}
	out := []Commitment{}
	seen := map[string]bool{}
	for _, a := range d.Atoms {
		if attr(a, "iugum-kind") == "commitment" {
			c, e := decode(a)
			if e != nil {
				return nil, e
			}
			if seen[c.ID] {
				return nil, fmt.Errorf("duplicate commitment reference %s", c.ID)
			}
			seen[c.ID] = true
			out = append(out, c)
		}
	}
	return out, nil
}
func encode(c Commitment, directive, existing string) ([]byte, error) {
	status := "open"
	if c.Focus {
		status = "focus"
	}
	if c.Deferred {
		status = "deferred"
	}
	if c.Done {
		status = "done"
	}
	due := c.Due
	if due == "" {
		due = "none"
	}
	if directive == "" {
		var alias bytes.Buffer
		_ = xml.EscapeText(&alias, []byte(c.ID))
		directive = fmt.Sprintf("<!-- <atom id=%q iugum-kind=\"commitment\" iugum-ref=\"%s\"/> -->", c.AtomID, alias.String())
	}
	text := fmt.Sprintf("Title: %s  \nStatus: %s  \nDue: %s  \nUpdated: %s", c.Title, status, due, c.Updated.Format(time.RFC3339Nano))
	if existing != "" {
		values := map[string]string{"Title": c.Title, "Status": status, "Due": due, "Updated": c.Updated.Format(time.RFC3339Nano)}
		lines := strings.Split(strings.TrimRight(existing, "\r\n"), "\n")
		for i, line := range lines {
			k, _, ok := strings.Cut(strings.TrimSpace(line), ":")
			if v, found := values[k]; ok && found {
				lines[i] = k + ": " + v + "  "
				delete(values, k)
			}
		}
		for _, k := range []string{"Title", "Status", "Due", "Updated"} {
			if v, found := values[k]; found {
				lines = append(lines, k+": "+v+"  ")
			}
		}
		text = strings.Join(lines, "\n")
	}
	return []byte(directive + "\n" + text), nil
}
func (s *Store) Put(c Commitment) (Commitment, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if strings.TrimSpace(c.Title) == "" || strings.ContainsAny(c.Title, "\r\n") {
		return c, errors.New("title must be one nonempty line")
	}
	if c.Due != "" {
		if _, e := time.Parse("2006-01-02", c.Due); e != nil {
			return c, e
		}
	}
	original, d, e := s.read()
	if e != nil {
		return c, e
	}
	if c.Updated.IsZero() {
		c.Updated = time.Now()
	}
	start, end := -1, -1
	directive := ""
	existing := ""
	for _, a := range d.Atoms {
		if attr(a, "iugum-kind") == "commitment" && (a.ID == c.ID || attr(a, "iugum-ref") == c.ID) {
			directive = string(original[a.Marker.Start.Offset:a.Marker.End.Offset])
			existing = a.Text
			c.AtomID = a.ID
			start = a.Marker.Start.Offset
			end = a.Content.End.Offset
			break
		}
	}
	if c.AtomID == "" {
		c.AtomID, e = atom.NewID()
		if e != nil {
			return c, e
		}
	}
	if c.ID == "" {
		c.ID = c.AtomID
	}
	block, e := encode(c, directive, existing)
	if e != nil {
		return c, e
	}
	block = bytes.TrimSpace(block)
	var next []byte
	if start >= 0 {
		next = append(append(append([]byte{}, original[:start]...), block...), original[end:]...)
	} else {
		next = append(append(original, []byte("\n\n")...), append(block, '\n')...)
	}
	current, e := os.ReadFile(s.Path)
	if e != nil {
		return c, e
	}
	if !bytes.Equal(current, original) {
		return c, errors.New("document changed during update; retry")
	}
	return c, s.write(next)
}
func (s *Store) write(b []byte) error {
	f, e := os.CreateTemp(filepath.Dir(s.Path), ".commitments-*")
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
	return os.Rename(f.Name(), s.Path)
}
