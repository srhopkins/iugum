import React from 'react'

const STATUS_CARDS = [
  { key: 'total', label: 'Total', status: null, valueKey: 'total' },
  { key: 'open', label: 'Open', status: 'open', valueKey: 'open' },
  {
    key: 'in_progress',
    label: 'In Progress',
    status: 'in_progress',
    valueKey: 'inProgress',
  },
  { key: 'blocked', label: 'Blocked', status: 'blocked', valueKey: 'blocked' },
  { key: 'closed', label: 'Closed', status: 'closed', valueKey: 'closed' },
]

export default function StatsRow({ stats, activeStatuses = [], onStatusFilter }) {
  const only =
    activeStatuses.length === 1 ? activeStatuses[0] : null
  const totalActive = activeStatuses.length === 0

  return (
    <>
      <div className="stats-row" role="group" aria-label="Issue status filters">
        {STATUS_CARDS.map((card) => {
          const active =
            card.status === null ? totalActive : only === card.status
          const className = [
            'stat-card',
            card.key,
            onStatusFilter ? 'stat-card-clickable' : '',
            active && onStatusFilter ? 'active' : '',
          ]
            .filter(Boolean)
            .join(' ')

          if (!onStatusFilter) {
            return (
              <div key={card.key} className={className}>
                <div className="stat-value">{stats[card.valueKey]}</div>
                <div className="stat-label">{card.label}</div>
              </div>
            )
          }

          return (
            <button
              key={card.key}
              type="button"
              className={className}
              onClick={() => onStatusFilter(card.status)}
              aria-pressed={active}
              title={
                card.status
                  ? `Filter to ${card.label.toLowerCase()} issues`
                  : 'Clear status filter (show all)'
              }
            >
              <div className="stat-value">{stats[card.valueKey]}</div>
              <div className="stat-label">{card.label}</div>
            </button>
          )
        })}
      </div>
      <div className="progress-section">
        <div className="progress-header">
          <span>Completion</span>
          <span>{stats.pct}%</span>
        </div>
        <div className="progress-track">
          <div className="progress-fill" style={{ width: `${stats.pct}%` }} />
        </div>
      </div>
    </>
  )
}
