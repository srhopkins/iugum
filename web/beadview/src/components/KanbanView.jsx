import React, { useState, useMemo } from 'react'
import StatusBadge from './StatusBadge'
import PriorityBadge from './PriorityBadge'

const COLUMNS = [
  { key: 'open', label: 'Open', color: 'var(--accent-blue)' },
  { key: 'in_progress', label: 'In Progress', color: 'var(--accent-yellow)' },
  { key: 'blocked', label: 'Blocked', color: 'var(--accent-red)' },
  { key: 'closed', label: 'Closed', color: 'var(--accent-green)' },
]

function KanbanCard({ bead, onSelect, onDragStart, canDrag }) {
  const blockerCount = (bead.depends_on || bead.dependencies || []).length

  return (
    <div
      className="kanban-card"
      draggable={canDrag}
      onDragStart={e => {
        e.dataTransfer.setData('text/plain', bead.id)
        if (!canDrag) return
        e.target.classList.add('dragging')
        onDragStart(bead.id)
      }}
      onDragEnd={e => e.target.classList.remove('dragging')}
      onClick={() => onSelect(bead.id)}
    >
      <div className="kanban-card-title">
        <span className="bead-id">{bead.id}</span>{' '}
        {bead.title}
      </div>
      <div className="kanban-card-meta">
        <PriorityBadge priority={bead.priority} />
        {blockerCount > 0 && (
          <span style={{ fontSize: 11, color: 'var(--text-muted)' }}>
            {blockerCount} dep{blockerCount !== 1 ? 's' : ''}
          </span>
        )}
        {(bead.assignee || bead.actor) && (
          <span style={{ fontSize: 11, color: 'var(--text-muted)', marginLeft: 'auto' }}>
            {bead.assignee || bead.actor}
          </span>
        )}
      </div>
    </div>
  )
}

export default function KanbanView({ beads, onSelect, onStatusChange }) {
  const [dragOverCol, setDragOverCol] = useState(null)
  const [draggingId, setDraggingId] = useState(null)

  const grouped = useMemo(() => {
    const groups = {}
    COLUMNS.forEach(c => { groups[c.key] = [] })
    beads.forEach(b => {
      const status = b.status || 'open'
      if (groups[status]) {
        groups[status].push(b)
      } else {
        groups.open.push(b)
      }
    })
    return groups
  }, [beads])

  function handleDrop(e, targetStatus) {
    e.preventDefault()
    const beadId = e.dataTransfer.getData('text/plain')
    setDragOverCol(null)
    setDraggingId(null)
    if (beadId && onStatusChange) {
      const bead = beads.find(b => b.id === beadId)
      if (bead && bead.status !== targetStatus) {
        onStatusChange(beadId, targetStatus)
      }
    }
  }

  return (
    <div className="kanban-board">
      {COLUMNS.map(col => (
        <div
          key={col.key}
          className={`kanban-column${dragOverCol === col.key ? ' drag-over' : ''}`}
          onDragOver={e => { e.preventDefault(); setDragOverCol(col.key) }}
          onDragLeave={() => setDragOverCol(null)}
          onDrop={e => handleDrop(e, col.key)}
        >
          <div className="kanban-column-header">
            <span style={{ color: col.color }}>{col.label}</span>
            <span className="count">{grouped[col.key].length}</span>
          </div>
          <div className="kanban-column-body">
            {grouped[col.key].map(bead => (
              <KanbanCard
                key={bead.id}
                bead={bead}
                onSelect={onSelect}
                onDragStart={setDraggingId}
                canDrag={!!onStatusChange}
              />
            ))}
          </div>
        </div>
      ))}
    </div>
  )
}
