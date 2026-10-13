import React, { useMemo, useRef, useState } from 'react'
import PriorityBadge from './PriorityBadge'
import { ProjectTag } from './ProjectChips'

/** Board columns, left to right. `deferred` is the Backlog, `open` is Ready. */
export const COLUMNS = [
  { key: 'deferred', label: 'Backlog', color: 'var(--accent-purple)' },
  { key: 'open', label: 'Ready', color: 'var(--accent-blue)' },
  { key: 'in_progress', label: 'In Progress', color: 'var(--accent-yellow)' },
  { key: 'blocked', label: 'Blocked', color: 'var(--accent-red)' },
  { key: 'closed', label: 'Closed', color: 'var(--accent-green)' },
]

const COLUMN_KEYS = COLUMNS.map((c) => c.key)

/** Column for a bead. A status the board does not know lands in Ready. */
export function columnFor(bead) {
  const s = bead.status || 'open'
  return COLUMN_KEYS.includes(s) ? s : 'open'
}

function BoardCard({ bead, showProject, onSelect, onDragStart, onDragEnd, canDrag }) {
  const who = bead.assignee || bead.actor
  return (
    <div
      className="kanban-card"
      data-project={bead.project || ''}
      data-id={bead.id}
      draggable={canDrag}
      onDragStart={(e) => {
        e.dataTransfer.setData('text/plain', `${bead.project || ''}/${bead.id}`)
        e.dataTransfer.effectAllowed = 'move'
        e.currentTarget.classList.add('dragging')
        onDragStart(bead)
      }}
      onDragEnd={(e) => {
        e.currentTarget.classList.remove('dragging')
        onDragEnd()
      }}
      onClick={() => onSelect(bead.id, bead.project)}
    >
      <div className="kanban-card-top">
        {showProject && <ProjectTag project={bead.project} />}
        <span className="bead-id">{bead.id}</span>
        <PriorityBadge priority={bead.priority} />
      </div>
      <div className="kanban-card-title">{bead.title}</div>
      <div className="kanban-card-meta">
        <span style={{ fontSize: 11, color: 'var(--text-muted)' }}>{who || 'unassigned'}</span>
      </div>
    </div>
  )
}

/**
 * The board. With `swimlanes` there is one row per project. A drop calls
 * onStatusChange(bead, status) for the bead's own project. In a swimlane, a
 * drop into another project's row is ignored.
 */
export default function BoardView({ beads, projects, swimlanes, onSelect, onStatusChange }) {
  const [dragOver, setDragOver] = useState(null)
  const dragging = useRef(null)
  const canDrag = !!onStatusChange
  const showProject = !swimlanes && projects.length > 1

  const groups = useMemo(() => {
    const g = {}
    for (const b of beads) {
      const lane = swimlanes ? b.project || '' : ''
      const key = `${lane}|${columnFor(b)}`
      ;(g[key] = g[key] || []).push(b)
    }
    return g
  }, [beads, swimlanes])

  const lanes = swimlanes ? projects : ['']

  function cellKey(lane, col) {
    return `${lane}|${col}`
  }

  function handleDrop(e, lane, col) {
    e.preventDefault()
    const bead = dragging.current
    dragging.current = null
    setDragOver(null)
    if (!bead || !onStatusChange) return
    if (swimlanes && (bead.project || '') !== lane) return
    if (columnFor(bead) === col) return
    onStatusChange(bead, col)
  }

  function renderCell(lane, col) {
    const k = cellKey(lane, col.key)
    const list = groups[k] || []
    return (
      <div
        key={k}
        className={`kanban-column${dragOver === k ? ' drag-over' : ''}`}
        data-column={col.key}
        data-lane={lane}
        onDragOver={(e) => {
          e.preventDefault()
          setDragOver(k)
        }}
        onDragLeave={() => setDragOver((cur) => (cur === k ? null : cur))}
        onDrop={(e) => handleDrop(e, lane, col.key)}
      >
        <div className="kanban-column-body">
          {list.map((bead) => (
            <BoardCard
              key={`${bead.project}/${bead.id}`}
              bead={bead}
              showProject={showProject}
              onSelect={onSelect}
              onDragStart={(b) => {
                dragging.current = b
              }}
              onDragEnd={() => {
                dragging.current = null
                setDragOver(null)
              }}
              canDrag={canDrag}
            />
          ))}
        </div>
      </div>
    )
  }

  const totals = {}
  for (const b of beads) totals[columnFor(b)] = (totals[columnFor(b)] || 0) + 1

  return (
    <div className={`board${swimlanes ? ' board-lanes' : ''}`}>
      <div className="board-head">
        {COLUMNS.map((col) => (
          <div key={col.key} className="kanban-column-header" data-column-header={col.key}>
            <span style={{ color: col.color }}>{col.label}</span>
            <span className="count">{totals[col.key] || 0}</span>
          </div>
        ))}
      </div>
      {lanes.map((lane) => (
        <div key={lane || 'all'} className="board-lane" data-lane-row={lane}>
          {swimlanes && (
            <div className="board-lane-title">
              <ProjectTag project={lane} />
            </div>
          )}
          <div className="board-row">{COLUMNS.map((col) => renderCell(lane, col))}</div>
        </div>
      ))}
    </div>
  )
}
