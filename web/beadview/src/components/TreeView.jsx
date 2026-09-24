import React, { useState, useMemo } from 'react'
import StatusBadge from './StatusBadge'
import PriorityBadge from './PriorityBadge'

/**
 * A bead nests under its `parent` and nowhere else. Dependencies do not
 * nest; they show in the detail panel and the Graph tab.
 */
export function isChildOf(bead, parentId) {
  return !!bead.parent && bead.parent === parentId
}

/** A bead is a root when it has no parent, or its parent is not in `ids`. */
export function isRoot(bead, ids) {
  return !bead.parent || !ids.has(bead.parent)
}

function TreeNode({ bead, beads, allBeads, expandedSet, toggleExpand, onSelect, focusedId, depth = 0 }) {
  const isExpanded = expandedSet.has(bead.id)
  const isKid = b => isChildOf(b, bead.id)
  // Shown children come from the filtered, sorted list, so the filters and
  // Sort apply at every level. The percent counts every parent-child.
  const children = beads.filter(isKid)
  const allChildren = allBeads.filter(isKid)

  const childCount = allChildren.length
  const closedChildren = allChildren.filter(c => c.status === 'closed').length
  const completionPct = childCount > 0 ? Math.round((closedChildren / childCount) * 100) : null

  return (
    <div className="tree-node" style={{ marginLeft: depth > 0 ? 20 : 0 }}>
      <div
        className={`tree-node-header${focusedId === bead.id ? ' selected' : ''}`}
        onClick={() => onSelect(bead.id)}
      >
        <span
          className="tree-toggle"
          onClick={e => { e.stopPropagation(); toggleExpand(bead.id) }}
          style={{ visibility: children.length > 0 ? 'visible' : 'hidden' }}
        >
          {isExpanded ? '\u25BC' : '\u25B6'}
        </span>
        <span className="bead-id" onClick={e => { e.stopPropagation(); onSelect(bead.id) }}>
          {bead.id}
        </span>
        <span className="tree-title">{bead.title}</span>
        <div className="tree-meta">
          {completionPct !== null && (
            <span className="tree-progress">{completionPct}%</span>
          )}
          <StatusBadge status={bead.status} />
          <PriorityBadge priority={bead.priority} />
          {(bead.assignee || bead.actor) && (
            <span style={{ fontSize: 11, color: 'var(--text-muted)' }}>
              {bead.assignee || bead.actor}
            </span>
          )}
        </div>
      </div>
      {isExpanded && children.map(child => (
        <TreeNode
          key={child.id}
          bead={child}
          beads={beads}
          allBeads={allBeads}
          expandedSet={expandedSet}
          toggleExpand={toggleExpand}
          onSelect={onSelect}
          focusedId={focusedId}
          depth={depth + 1}
        />
      ))}
    </div>
  )
}

export default function TreeView({ beads, allBeads, focusedIndex, onSelect }) {
  const [expandedSet, setExpandedSet] = useState(new Set())

  function toggleExpand(id) {
    setExpandedSet(prev => {
      const next = new Set(prev)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })
  }

  function expandAll() {
    setExpandedSet(new Set(allBeads.map(b => b.id)))
  }

  function collapseAll() {
    setExpandedSet(new Set())
  }

  const rootBeads = useMemo(() => {
    const filteredIds = new Set(beads.map(b => b.id))
    return beads.filter(b => isRoot(b, filteredIds))
  }, [beads])

  const focusedId = focusedIndex >= 0 && focusedIndex < beads.length
    ? beads[focusedIndex].id
    : null

  if (beads.length === 0) {
    return (
      <div className="empty-state">
        <h3>No beads match your filters</h3>
        <p>Try adjusting your search or filter criteria.</p>
      </div>
    )
  }

  return (
    <div>
      <div style={{ display: 'flex', gap: 8, marginBottom: 12 }}>
        <button className="btn btn-sm" onClick={expandAll}>Expand All</button>
        <button className="btn btn-sm" onClick={collapseAll}>Collapse All</button>
      </div>
      {rootBeads.map(bead => (
        <TreeNode
          key={bead.id}
          bead={bead}
          beads={beads}
          allBeads={allBeads}
          expandedSet={expandedSet}
          toggleExpand={toggleExpand}
          onSelect={onSelect}
          focusedId={focusedId}
        />
      ))}
    </div>
  )
}
