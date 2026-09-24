import React, { useMemo } from 'react'
import StatusBadge from './StatusBadge'

function AgentCards({ beads }) {
  const agents = useMemo(() => {
    const map = {}
    beads.forEach(b => {
      const name = b.assignee || b.actor || ''
      if (!name) return
      if (!map[name]) {
        map[name] = { name, claimed: [], inProgress: null }
      }
      map[name].claimed.push(b)
      if (b.status === 'in_progress') {
        map[name].inProgress = b
      }
    })
    return Object.values(map)
  }, [beads])

  if (agents.length === 0) return null

  return (
    <div className="agent-cards">
      {agents.map(agent => (
        <div key={agent.name} className="agent-card">
          <div className="agent-card-header">
            <span className="agent-name">{agent.name}</span>
            <span className={`agent-status-dot${agent.inProgress ? ' active' : ''}`} />
          </div>
          <div className="agent-meta">
            {agent.inProgress ? (
              <span>
                Working on <span className="bead-id">{agent.inProgress.id}</span>
                {' '}&mdash; {agent.inProgress.title}
              </span>
            ) : (
              <span>{agent.claimed.length} bead{agent.claimed.length !== 1 ? 's' : ''} claimed</span>
            )}
          </div>
        </div>
      ))}
    </div>
  )
}

function formatTime(dateStr) {
  if (!dateStr) return ''
  try {
    const d = new Date(dateStr)
    const now = new Date()
    const diffMs = now - d
    const diffMins = Math.floor(diffMs / 60000)
    const diffHrs = Math.floor(diffMs / 3600000)
    const diffDays = Math.floor(diffMs / 86400000)

    if (diffMins < 1) return 'just now'
    if (diffMins < 60) return `${diffMins}m ago`
    if (diffHrs < 24) return `${diffHrs}h ago`
    if (diffDays < 7) return `${diffDays}d ago`
    return d.toLocaleDateString()
  } catch {
    return dateStr
  }
}

export default function ActivityView({ beads, allBeads, onSelect }) {
  // Sort beads by updated_at descending to create an activity feed
  const sortedBeads = useMemo(() => {
    return [...beads].sort((a, b) => {
      const aTime = a.updated_at || a.created_at || ''
      const bTime = b.updated_at || b.created_at || ''
      return bTime.localeCompare(aTime)
    })
  }, [beads])

  return (
    <div>
      <AgentCards beads={allBeads} />

      <div className="activity-feed">
        {sortedBeads.length === 0 ? (
          <div className="empty-state">
            <h3>No activity to show</h3>
            <p>Activity will appear here when beads are updated.</p>
          </div>
        ) : (
          sortedBeads.map(bead => (
            <div
              key={bead.id}
              className="activity-item"
              onClick={() => onSelect(bead.id)}
            >
              <span className="activity-timestamp">
                {formatTime(bead.updated_at || bead.created_at)}
              </span>
              <div className="activity-content">
                <span className="bead-id">{bead.id}</span>{' '}
                <StatusBadge status={bead.status} />{' '}
                <span>&mdash; {bead.title}</span>
                {(bead.assignee || bead.actor) && (
                  <span style={{ color: 'var(--text-muted)', fontSize: 12, marginLeft: 8 }}>
                    ({bead.assignee || bead.actor})
                  </span>
                )}
              </div>
            </div>
          ))
        )}
      </div>
    </div>
  )
}
