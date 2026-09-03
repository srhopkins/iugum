// Package beadview is a read-mostly HTTP viewer for the beads work graph.
// It never shells out to an external bd binary. Every query re-execs the
// running iugum binary itself in the "beads" subcommand, which runs the
// vendored Beads CLI in-process (see adapter/tracker/beadsadapt). See the
// package README for why a subprocess, not a direct in-process call.
package beadview

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"sort"
	"strings"
)

// Dependency is one edge on a bead, as bd export/list/show report it.
type Dependency struct {
	IssueID     string `json:"issue_id"`
	DependsOnID string `json:"depends_on_id"`
	Type        string `json:"type"`
}

// Bead is the subset of bd's issue JSON the viewer renders. Extra fields in
// the real payload are ignored by encoding/json.
type Bead struct {
	ID            string       `json:"id"`
	Title         string       `json:"title"`
	Description   string       `json:"description"`
	Notes         string       `json:"notes"`
	Status        string       `json:"status"`
	Priority      json.Number  `json:"priority"`
	IssueType     string       `json:"issue_type"`
	Assignee      string       `json:"assignee"`
	Owner         string       `json:"owner"`
	CreatedAt     string       `json:"created_at"`
	UpdatedAt     string       `json:"updated_at"`
	ClosedAt      string       `json:"closed_at"`
	Labels        []string     `json:"labels"`
	Parent        string       `json:"parent"`
	Dependencies  []Dependency `json:"dependencies"`
	DependencyCnt int          `json:"dependency_count"`
	DependentCnt  int          `json:"dependent_count"`
	CommentCnt    int          `json:"comment_count"`
}

// PriorityLabel renders "p0".."p4" (or the raw value if it is not an int).
func (b Bead) PriorityLabel() string {
	if b.Priority == "" {
		return ""
	}
	if n, err := b.Priority.Int64(); err == nil {
		return fmt.Sprintf("p%d", n)
	}
	return string(b.Priority)
}

// Blocks returns the IDs this bead depends on via a real "blocks" edge,
// excluding parent-child (epic hierarchy) and soft links (relates-to,
// supersedes, discovered-from). The graph view renders these edges; the
// table view does not walk them.
func (b Bead) Blocks() []string {
	var out []string
	for _, d := range b.Dependencies {
		if d.Type == "blocks" {
			out = append(out, d.DependsOnID)
		}
	}
	return out
}

// Comment is one entry from `bd comments <id> --json`.
type Comment struct {
	ID        string `json:"id"`
	IssueID   string `json:"issue_id"`
	Author    string `json:"author"`
	Text      string `json:"text"`
	CreatedAt string `json:"created_at"`
}

// Fetcher is how the HTTP handler gets data. The production implementation
// (execFetcher) re-execs iugum's own "beads" subcommand. Tests inject a
// fake so handler tests never need a built binary or a real .beads dir.
type Fetcher interface {
	FetchBeads(ctx context.Context) ([]Bead, error)
	FetchBead(ctx context.Context, id string) (*Bead, error)
	FetchComments(ctx context.Context, id string) ([]Comment, error)
	FetchGraphHTML(ctx context.Context) (string, error)
	FetchStatus(ctx context.Context) (string, error)
	Dir() string
}

// execFetcher runs `<exe> beads -C <dir> <args...>` as a child process and
// parses its stdout. exe is normally os.Executable() (iugum's own path).
type execFetcher struct {
	exe string
	dir string
}

// NewExecFetcher builds the production Fetcher. exe is the path to the
// running iugum binary; dir is the target beads repo (must contain .beads).
func NewExecFetcher(exe, dir string) Fetcher {
	return &execFetcher{exe: exe, dir: dir}
}

func (f *execFetcher) Dir() string { return f.dir }

// run execs `<exe> beads -C <dir> <args...>` and returns stdout. A non-zero
// exit is an error carrying stderr, never a process-wide os.Exit: bd's own
// Execute() can call os.Exit on error paths, which is why this is a
// subprocess and not a direct in-process call (see README "Data path").
func (f *execFetcher) run(ctx context.Context, args ...string) ([]byte, error) {
	full := append([]string{"beads", "-C", f.dir}, args...)
	cmd := exec.CommandContext(ctx, f.exe, full...)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = strings.TrimSpace(stdout.String())
		}
		return nil, fmt.Errorf("iugum beads %s: %w: %s", strings.Join(args, " "), err, msg)
	}
	return []byte(stdout.String()), nil
}

func (f *execFetcher) FetchBeads(ctx context.Context) ([]Bead, error) {
	out, err := f.run(ctx, "list", "--json", "--all", "--limit", "0")
	if err != nil {
		return nil, err
	}
	var beads []Bead
	if err := json.Unmarshal(out, &beads); err != nil {
		return nil, fmt.Errorf("parsing bd list --json: %w", err)
	}
	return beads, nil
}

func (f *execFetcher) FetchBead(ctx context.Context, id string) (*Bead, error) {
	out, err := f.run(ctx, "show", id, "--json")
	if err != nil {
		return nil, err
	}
	var beads []Bead
	if err := json.Unmarshal(out, &beads); err != nil {
		return nil, fmt.Errorf("parsing bd show --json: %w", err)
	}
	if len(beads) == 0 {
		return nil, fmt.Errorf("bead %s not found", id)
	}
	return &beads[0], nil
}

func (f *execFetcher) FetchComments(ctx context.Context, id string) ([]Comment, error) {
	out, err := f.run(ctx, "comments", id, "--json")
	if err != nil {
		return nil, err
	}
	var comments []Comment
	if err := json.Unmarshal(out, &comments); err != nil {
		return nil, fmt.Errorf("parsing bd comments --json: %w", err)
	}
	return comments, nil
}

// FetchGraphHTML returns bd's own self-contained interactive D3 dependency
// graph (`bd graph --all --html`). It is served as-is: real edges, real
// epic children, no reimplementation. See README "Why bd graph --html".
func (f *execFetcher) FetchGraphHTML(ctx context.Context) (string, error) {
	out, err := f.run(ctx, "graph", "--all", "--html")
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func (f *execFetcher) FetchStatus(ctx context.Context) (string, error) {
	out, err := f.run(ctx, "status")
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// Filter narrows a bead list for the table view.
type Filter struct {
	Status string // exact match, empty = any
	Type   string // exact match, empty = any
	Search string // case-insensitive substring over ID, title, description
	Sort   string // updated (default) | created | priority | status | id | title
}

func ApplyFilter(beads []Bead, f Filter) []Bead {
	out := make([]Bead, 0, len(beads))
	q := strings.ToLower(strings.TrimSpace(f.Search))
	for _, b := range beads {
		if f.Status != "" && b.Status != f.Status {
			continue
		}
		if f.Type != "" && b.IssueType != f.Type {
			continue
		}
		if q != "" {
			hay := strings.ToLower(b.ID + " " + b.Title + " " + b.Description)
			if !strings.Contains(hay, q) {
				continue
			}
		}
		out = append(out, b)
	}
	sortBeads(out, f.Sort)
	return out
}

func sortBeads(beads []Bead, by string) {
	switch by {
	case "created":
		sort.Slice(beads, func(i, j int) bool { return beads[i].CreatedAt > beads[j].CreatedAt })
	case "priority":
		sort.Slice(beads, func(i, j int) bool { return priorityRank(beads[i]) < priorityRank(beads[j]) })
	case "status":
		sort.Slice(beads, func(i, j int) bool { return beads[i].Status < beads[j].Status })
	case "id":
		sort.Slice(beads, func(i, j int) bool { return beads[i].ID < beads[j].ID })
	case "title":
		sort.Slice(beads, func(i, j int) bool { return beads[i].Title < beads[j].Title })
	default: // "updated"
		sort.Slice(beads, func(i, j int) bool { return beads[i].UpdatedAt > beads[j].UpdatedAt })
	}
}

func priorityRank(b Bead) int64 {
	n, err := b.Priority.Int64()
	if err != nil {
		return 99
	}
	return n
}

// Statuses/Types are the fixed vocabularies the filter dropdowns offer.
// Beads itself defines these; this list is display order, not validation.
var Statuses = []string{"open", "in_progress", "blocked", "deferred", "closed"}
var Types = []string{"bug", "feature", "task", "epic", "chore", "decision"}
