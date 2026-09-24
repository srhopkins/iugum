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
	"sync"
	"time"
)

// Dependency is one edge on a bead, as bd export/list/show report it.
type Dependency struct {
	IssueID     string `json:"issue_id"`
	DependsOnID string `json:"depends_on_id"`
	Type        string `json:"type"`
}

// UnmarshalJSON accepts both dependency shapes bd emits. `bd list --json`
// gives edges ({issue_id, depends_on_id, type}); `bd show --json` gives
// the depended-on issue itself ({id, title, ..., dependency_type}). Both
// become the same edge; IssueID stays empty for the show shape.
func (d *Dependency) UnmarshalJSON(b []byte) error {
	var raw struct {
		IssueID        string `json:"issue_id"`
		DependsOnID    string `json:"depends_on_id"`
		Type           string `json:"type"`
		ID             string `json:"id"`
		DependencyType string `json:"dependency_type"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	d.IssueID, d.DependsOnID, d.Type = raw.IssueID, raw.DependsOnID, raw.Type
	if d.DependsOnID == "" && raw.ID != "" {
		d.DependsOnID = raw.ID
		d.Type = raw.DependencyType
	}
	return nil
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
	CreatedBy     string       `json:"created_by"`
	CreatedAt     string       `json:"created_at"`
	UpdatedAt     string       `json:"updated_at"`
	StartedAt     string       `json:"started_at"`
	ClosedAt      string       `json:"closed_at"`
	CloseReason   string       `json:"close_reason"`
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
	// FetchMermaid returns `bd dep tree <root> --format mermaid` text.
	FetchMermaid(ctx context.Context, root string) (string, error)

	// Write methods. Each one is exactly one bd write subcommand. The
	// handler gates them (read-only flag, Casbin, request checks) and logs
	// them; the Fetcher itself does no policy.
	CreateBead(ctx context.Context, in CreateInput) (*Bead, error)
	AddDependency(ctx context.Context, id, dependsOn string) error
	UpdateBead(ctx context.Context, id string, in UpdateInput) (*Bead, error)
	CloseBead(ctx context.Context, id, reason string) (*Bead, error)
	ReopenBead(ctx context.Context, id, reason string) (*Bead, error)
	AddComment(ctx context.Context, id, text string) (*Comment, error)

	Dir() string
}

// CreateInput is one `bd create`. Priority is bd's own form ("0".."4").
// Empty fields are not passed, so bd applies its defaults.
type CreateInput struct {
	Title       string
	Description string
	Type        string
	Priority    string
}

// UpdateInput is one `bd update`. A nil pointer means "leave unchanged";
// a non-nil empty Description clears it.
type UpdateInput struct {
	Status      *string
	Priority    *string
	Description *string
	Title       *string
	Type        *string
	Claim       bool
}

// Empty reports whether the update would change nothing.
func (u UpdateInput) Empty() bool {
	return u.Status == nil && u.Priority == nil && u.Description == nil &&
		u.Title == nil && u.Type == nil && !u.Claim
}

// Args renders the update as bd flags. Every value uses --flag=value form
// so a value that starts with "-" is never parsed as a flag.
func (u UpdateInput) Args() []string {
	var a []string
	add := func(name string, v *string) {
		if v != nil {
			a = append(a, "--"+name+"="+*v)
		}
	}
	add("status", u.Status)
	add("priority", u.Priority)
	add("description", u.Description)
	add("title", u.Title)
	add("type", u.Type)
	if u.Claim {
		a = append(a, "--claim")
	}
	return a
}

// execFetcher runs `<exe> beads -C <dir> <args...>` as a child process and
// parses its stdout. exe is normally os.Executable() (iugum's own path).
type execFetcher struct {
	exe string
	dir string
	// mu serializes child processes. The UI fetches /api/beads and
	// /api/mermaid in parallel, and concurrent children can fail with
	// "database is locked" (embedded store, and iugum's own startup
	// store). Other iugum processes on the host can still hold the
	// startup store; runStdin retries that case.
	mu sync.Mutex
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
	return f.runStdin(ctx, "", args...)
}

// runStdin is run with stdin set to input (empty means no stdin).
func (f *execFetcher) runStdin(ctx context.Context, input string, args ...string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for attempt := 1; ; attempt++ {
		out, msg, err := f.once(ctx, input, args)
		if err == nil {
			return out, nil
		}
		// iugum's own startup (app.New, before any bd code runs) opens a
		// SQLite store shared with every other iugum process on the host.
		// Under contention it fails with "iugum: database is locked". The
		// bd command never started, so retrying is safe for reads and
		// writes alike.
		if attempt < startupRetries && isStartupLock(msg) {
			select {
			case <-ctx.Done():
			case <-time.After(time.Duration(attempt) * 150 * time.Millisecond):
				continue
			}
		}
		return nil, fmt.Errorf("iugum beads %s: %w: %s", strings.Join(args, " "), err, msg)
	}
}

const startupRetries = 4

func isStartupLock(msg string) bool {
	return strings.HasPrefix(msg, "iugum: ") && strings.Contains(msg, "database is locked")
}

// once runs one child process and returns stdout, the trimmed error text
// (stderr, or stdout if stderr is empty), and the exit error.
func (f *execFetcher) once(ctx context.Context, input string, args []string) ([]byte, string, error) {
	full := append([]string{"beads", "-C", f.dir}, args...)
	cmd := exec.CommandContext(ctx, f.exe, full...)
	if input != "" {
		cmd.Stdin = strings.NewReader(input)
	}
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = strings.TrimSpace(stdout.String())
		}
		return nil, msg, err
	}
	return []byte(stdout.String()), "", nil
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
	out, err := f.run(ctx, "show", "--json", "--", id)
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
	out, err := f.run(ctx, "comments", "--json", "--", id)
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

func (f *execFetcher) FetchMermaid(ctx context.Context, root string) (string, error) {
	out, err := f.run(ctx, "dep", "tree", "--format=mermaid", "--direction=both", "--", root)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// parseOne decodes bd --json output that is either one object or a
// one-element array (create returns an object; update/close/reopen/show
// return an array).
func parseOne(out []byte, what string) (*Bead, error) {
	trimmed := strings.TrimSpace(string(out))
	if strings.HasPrefix(trimmed, "[") {
		var beads []Bead
		if err := json.Unmarshal(out, &beads); err != nil {
			return nil, fmt.Errorf("parsing bd %s --json: %w", what, err)
		}
		if len(beads) == 0 {
			return nil, fmt.Errorf("bd %s returned no issue", what)
		}
		return &beads[0], nil
	}
	var b Bead
	if err := json.Unmarshal(out, &b); err != nil {
		return nil, fmt.Errorf("parsing bd %s --json: %w", what, err)
	}
	if b.ID == "" {
		return nil, fmt.Errorf("bd %s returned no issue id: %s", what, trimmed)
	}
	return &b, nil
}

func (f *execFetcher) CreateBead(ctx context.Context, in CreateInput) (*Bead, error) {
	args := []string{"create", "--json", "--title=" + in.Title}
	if in.Description != "" {
		args = append(args, "--description="+in.Description)
	}
	if in.Type != "" {
		args = append(args, "--type="+in.Type)
	}
	if in.Priority != "" {
		args = append(args, "--priority="+in.Priority)
	}
	out, err := f.run(ctx, args...)
	if err != nil {
		return nil, err
	}
	return parseOne(out, "create")
}

func (f *execFetcher) AddDependency(ctx context.Context, id, dependsOn string) error {
	_, err := f.run(ctx, "dep", "add", "--json", "--depends-on="+dependsOn, "--", id)
	return err
}

func (f *execFetcher) UpdateBead(ctx context.Context, id string, in UpdateInput) (*Bead, error) {
	args := append([]string{"update", "--json"}, in.Args()...)
	out, err := f.run(ctx, append(args, "--", id)...)
	if err != nil {
		return nil, err
	}
	return parseOne(out, "update")
}

func (f *execFetcher) CloseBead(ctx context.Context, id, reason string) (*Bead, error) {
	args := []string{"close", "--json"}
	if reason != "" {
		args = append(args, "--reason="+reason)
	}
	out, err := f.run(ctx, append(args, "--", id)...)
	if err != nil {
		return nil, err
	}
	return parseOne(out, "close")
}

func (f *execFetcher) ReopenBead(ctx context.Context, id, reason string) (*Bead, error) {
	args := []string{"reopen", "--json"}
	if reason != "" {
		args = append(args, "--reason="+reason)
	}
	out, err := f.run(ctx, append(args, "--", id)...)
	if err != nil {
		return nil, err
	}
	return parseOne(out, "reopen")
}

// AddComment passes the text on stdin, never as an argument, so any text
// (including a leading "-") is stored as written.
func (f *execFetcher) AddComment(ctx context.Context, id, text string) (*Comment, error) {
	out, err := f.runStdin(ctx, text, "comment", "--json", "--stdin", "--", id)
	if err != nil {
		return nil, err
	}
	var c Comment
	if err := json.Unmarshal(out, &c); err != nil {
		return nil, fmt.Errorf("parsing bd comment --json: %w", err)
	}
	return &c, nil
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
