import React, { useState, useMemo } from 'react'
import StatusBadge from './StatusBadge'
import PriorityBadge from './PriorityBadge'
import { depIds } from '../filters'

/**
 * A bead nests under its `parent` when it has one. A bead with no parent
 * nests under every bead it depends on (the original viewer's rule).
 */
export function isChildOf(bead, parentId) {
  if (bead.parent) return bead.parent === parentId
  return depIds(bead).includes(parentId)
}

/** A bead is a root when the bead(s) it would nest under are not in `ids`. */
export function isRoot(bead, ids) {
  if (bead.parent) return !ids.has(bead.parent)
  const deps = depIds(bead)
  return deps.length === 0 || !deps.some((d) => ids.has(d))
}

function TreeNode({ bead, allBeads, expandedSet, toggleExpand, onSelect, focusedId, depth = 0, ancestors = new Set() }) {
  const isExpanded = expandedSet.has(bead.id)
  // Skip any bead already on the path from the root, so a dependency cycle
  // cannot recurse forever.
  const children = allBeads.filter(b => !ancestors.has(b.id) && b.id !== bead.id && isChildOf(b, bead.id))

  // Calculate completion percentage for nodes with children
  const childCount = children.length
  const closedChildren = children.filter(c => c.status === 'closed').length
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
          allBeads={allBeads}
          expandedSet={expandedSet}
          toggleExpand={toggleExpand}
          onSelect={onSelect}
          focusedId={focusedId}
          depth={depth + 1}
          ancestors={new Set([...ancestors, bead.id])}
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
    const roots = beads.filter(b => isRoot(b, filteredIds))
    return roots.length > 0 ? roots : beads
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
