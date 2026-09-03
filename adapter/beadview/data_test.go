package beadview

import (
	"encoding/json"
	"strconv"
	"testing"
)

func numBead(id, status, issueType string, priority int, updated string) Bead {
	return Bead{
		ID:        id,
		Status:    status,
		IssueType: issueType,
		Priority:  json.Number(strconv.Itoa(priority)),
		UpdatedAt: updated,
	}
}

func TestPriorityLabel(t *testing.T) {
	cases := []struct {
		priority json.Number
		want     string
	}{
		{"", ""},
		{"0", "p0"},
		{"3", "p3"},
		{"p2", "p2"}, // non-numeric passes through unchanged
	}
	for _, c := range cases {
		b := Bead{Priority: c.priority}
		if got := b.PriorityLabel(); got != c.want {
			t.Errorf("PriorityLabel(%q) = %q, want %q", c.priority, got, c.want)
		}
	}
}

func TestBeadBlocksExcludesParentChildAndSoftLinks(t *testing.T) {
	b := Bead{
		ID: "a",
		Dependencies: []Dependency{
			{IssueID: "a", DependsOnID: "epic", Type: "parent-child"},
			{IssueID: "a", DependsOnID: "b", Type: "blocks"},
			{IssueID: "a", DependsOnID: "c", Type: "relates-to"},
			{IssueID: "a", DependsOnID: "d", Type: "blocks"},
		},
	}
	got := b.Blocks()
	want := []string{"b", "d"}
	if len(got) != len(want) {
		t.Fatalf("Blocks() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Blocks() = %v, want %v", got, want)
		}
	}
}

func TestApplyFilterStatus(t *testing.T) {
	beads := []Bead{
		numBead("a", "open", "task", 1, "2026-01-01"),
		numBead("b", "closed", "task", 1, "2026-01-02"),
		numBead("c", "open", "bug", 2, "2026-01-03"),
	}
	got := ApplyFilter(beads, Filter{Status: "open"})
	if len(got) != 2 {
		t.Fatalf("expected 2 open beads, got %d: %+v", len(got), got)
	}
	for _, b := range got {
		if b.Status != "open" {
			t.Errorf("unexpected status %q leaked through filter", b.Status)
		}
	}
}

func TestApplyFilterTypeAndSearch(t *testing.T) {
	beads := []Bead{
		{ID: "x-1", Title: "fix the widget", IssueType: "bug"},
		{ID: "x-2", Title: "add a widget", IssueType: "feature"},
		{ID: "x-3", Title: "unrelated", IssueType: "bug"},
	}
	got := ApplyFilter(beads, Filter{Type: "bug", Search: "widget"})
	if len(got) != 1 || got[0].ID != "x-1" {
		t.Fatalf("expected only x-1, got %+v", got)
	}
}

func TestApplyFilterSortPriority(t *testing.T) {
	beads := []Bead{
		numBead("low", "open", "task", 3, "2026-01-01"),
		numBead("high", "open", "task", 0, "2026-01-01"),
		numBead("mid", "open", "task", 1, "2026-01-01"),
	}
	got := ApplyFilter(beads, Filter{Sort: "priority"})
	order := []string{got[0].ID, got[1].ID, got[2].ID}
	want := []string{"high", "mid", "low"}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("priority sort order = %v, want %v", order, want)
		}
	}
}

func TestApplyFilterSortUpdatedDefault(t *testing.T) {
	beads := []Bead{
		numBead("old", "open", "task", 1, "2026-01-01T00:00:00Z"),
		numBead("new", "open", "task", 1, "2026-06-01T00:00:00Z"),
	}
	got := ApplyFilter(beads, Filter{})
	if got[0].ID != "new" || got[1].ID != "old" {
		t.Fatalf("default sort should be newest-updated-first, got %v", []string{got[0].ID, got[1].ID})
	}
}

func TestApplyFilterDoesNotMutateInput(t *testing.T) {
	beads := []Bead{
		numBead("a", "open", "task", 1, "2026-01-01"),
		numBead("b", "closed", "task", 1, "2026-01-02"),
	}
	_ = ApplyFilter(beads, Filter{Sort: "id"}) // would reorder a copy if it touched the original
	if beads[0].ID != "a" || beads[1].ID != "b" {
		t.Fatalf("ApplyFilter mutated caller's slice order: %v", []string{beads[0].ID, beads[1].ID})
	}
}
