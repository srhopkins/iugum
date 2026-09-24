package beadview

import (
	"fmt"
	"sort"
	"strings"
)

// TreeNode is one bead plus its children, grouped by Bead.Parent (epic
// hierarchy), not by dependency edges. See BuildTree.
type TreeNode struct {
	Bead     Bead
	Children []*TreeNode
}

// BuildTree groups beads into a forest keyed on Bead.Parent. A bead whose
// Parent is empty, or whose parent is not present in the given set (e.g. a
// closed parent excluded upstream), becomes a root.
func BuildTree(beads []Bead) []*TreeNode {
	nodes := make(map[string]*TreeNode, len(beads))
	for _, b := range beads {
		nodes[b.ID] = &TreeNode{Bead: b}
	}
	var roots []*TreeNode
	for _, b := range beads {
		n := nodes[b.ID]
		if b.Parent != "" {
			if p, ok := nodes[b.Parent]; ok && p != n {
				p.Children = append(p.Children, n)
				continue
			}
		}
		roots = append(roots, n)
	}
	sortTree(roots)
	return roots
}

func sortTree(nodes []*TreeNode) {
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].Bead.ID < nodes[j].Bead.ID })
	for _, n := range nodes {
		sortTree(n.Children)
	}
}

// ChildProgressLabel returns "closed/total closed" for a node with children,
// or "" for a leaf. Only counts direct children, not the whole subtree.
func (n *TreeNode) ChildProgressLabel() string {
	if len(n.Children) == 0 {
		return ""
	}
	var closed int
	for _, c := range n.Children {
		if c.Bead.Status == "closed" {
			closed++
		}
	}
	return fmt.Sprintf("%d/%d closed", closed, len(n.Children))
}

func (n *TreeNode) matches(q string) bool {
	if q == "" {
		return true
	}
	hay := strings.ToLower(n.Bead.ID + " " + n.Bead.Title + " " + n.Bead.Description)
	return strings.Contains(hay, q)
}

// PruneTree keeps a node if it matches q itself or any descendant does,
// so a search always keeps the ancestor chain down to every real hit
// instead of hiding it out of context. q is matched case-insensitively.
func PruneTree(nodes []*TreeNode, q string) []*TreeNode {
	q = strings.ToLower(strings.TrimSpace(q))
	if q == "" {
		return nodes
	}
	var out []*TreeNode
	for _, n := range nodes {
		kids := PruneTree(n.Children, q)
		if n.matches(q) || len(kids) > 0 {
			out = append(out, &TreeNode{Bead: n.Bead, Children: kids})
		}
	}
	return out
}
