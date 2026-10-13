import React, { useMemo, useState } from 'react'
import StatusBadge from './StatusBadge'
import PriorityBadge from './PriorityBadge'
import { ProjectTag } from './ProjectChips'
import { ticketHref } from '../api'
import { priorityRank } from '../filters'

const COLS = [
  { key: 'project', label: 'Project' },
  { key: 'id', label: 'ID' },
  { key: 'title', label: 'Title' },
  { key: 'status', label: 'Status' },
  { key: 'priority', label: 'Priority' },
  { key: 'type', label: 'Type' },
  { key: 'assignee', label: 'Assignee' },
  { key: 'labels', label: 'Labels' },
  { key: 'updated_at', label: 'Updated' },
]

function sortValue(bead, key) {
  switch (key) {
    case 'priority':
      return priorityRank(bead.priority)
    case 'labels':
      return (bead.labels || []).join(',')
    case 'assignee':
      return bead.assignee || bead.actor || ''
    default:
      return bead[key] || ''
  }
}

/**
 * A table of beads. The filters (status, project chips, priority, labels,
 * assignee, search) come from the parent: `beads` is already filtered.
 * A click on a column header sorts by it. A second click reverses it.
 */
export default function ListView({ beads, onSelect, showProject = true }) {
  const [sort, setSort] = useState({ key: '', dir: 1 })

  const rows = useMemo(() => {
    if (!sort.key) return beads
    return [...beads].sort((a, b) => {
      const x = sortValue(a, sort.key)
      const y = sortValue(b, sort.key)
      const c = typeof x === 'number' && typeof y === 'number' ? x - y : String(x).localeCompare(String(y))
      return c * sort.dir
    })
  }, [beads, sort])

  function toggleSort(key) {
    setSort((cur) => (cur.key === key ? { key, dir: -cur.dir } : { key, dir: 1 }))
  }

  const cols = showProject ? COLS : COLS.filter((c) => c.key !== 'project')

  return (
    <div className="list-wrap">
      <table className="list-table">
        <thead>
          <tr>
            {cols.map((c) => (
              <th
                key={c.key}
                onClick={() => toggleSort(c.key)}
                aria-sort={sort.key === c.key ? (sort.dir === 1 ? 'ascending' : 'descending') : 'none'}
                className="list-th"
              >
                {c.label}
                {sort.key === c.key ? (sort.dir === 1 ? ' ▲' : ' ▼') : ''}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {rows.map((b) => (
            <tr key={`${b.project}/${b.id}`} className="list-row" onClick={() => onSelect(b.id, b.project)}>
              {showProject && (
                <td>
                  <ProjectTag project={b.project} />
                </td>
              )}
              <td>
                <a
                  className="bead-id"
                  href={ticketHref(b.project, b.id)}
                  onClick={(e) => e.stopPropagation()}
                  title="Open the full page"
                >
                  {b.id}
                </a>
              </td>
              <td className="list-title">{b.title}</td>
              <td>
                <StatusBadge status={b.status} />
              </td>
              <td>
                <PriorityBadge priority={b.priority} />
              </td>
              <td>{b.type}</td>
              <td>{b.assignee || b.actor || ''}</td>
              <td>{(b.labels || []).join(', ')}</td>
              <td className="list-date">{(b.updated_at || '').slice(0, 10)}</td>
            </tr>
          ))}
        </tbody>
      </table>
      {rows.length === 0 && <div className="loading">No beads match the filters.</div>}
    </div>
  )
}
