import React from 'react'

export default function Header({
  connected,
  wsConnected,
  theme,
  onToggleTheme,
  onNewBead,
  onRefresh,
  onHelp,
  project,
  projects,
  onProjectChange,
  readOnly = false,
}) {
  const list = Array.isArray(projects) ? projects : []

  return (
    <header className="app-header">
      <h1>iugum beads</h1>
      <div className="header-controls">
        <label
          className="project-select-wrap"
          style={{ display: 'flex', alignItems: 'center', gap: 6, fontSize: 13 }}
          title="Beads project served by this iugum beadview"
        >
          <span style={{ color: 'var(--text-muted)' }}>Project</span>
          <select
            className="project-select"
            aria-label="Project"
            value={project || ''}
            onChange={(e) => onProjectChange?.(e.target.value)}
            style={{
              background: 'var(--bg-secondary)',
              color: 'var(--text-primary)',
              border: '1px solid var(--border)',
              borderRadius: 4,
              padding: '4px 8px',
              maxWidth: 200,
            }}
          >
            {list.length === 0 && <option value={project || ''}>{project || '...'}</option>}
            {list.map((p) => (
              <option key={p.slug} value={p.slug} title={p.description || p.slug}>
                {p.slug}
              </option>
            ))}
          </select>
        </label>
        <div className="connection-status">
          <span
            className={`connection-dot${!connected || !wsConnected ? ' disconnected' : ''}`}
          />
          <span>{wsConnected ? 'Live' : connected ? 'Polling' : 'Disconnected'}</span>
        </div>
        <button className="btn btn-sm" onClick={onRefresh} title="Refresh data">
          Refresh
        </button>
        {!readOnly && (
          <button className="btn btn-primary btn-sm" onClick={onNewBead} title="New bead (n)">
            + New Bead
          </button>
        )}
        <button className="theme-toggle" onClick={onToggleTheme} title="Toggle theme">
          {theme === 'dark' ? '☀' : '☾'}
        </button>
        <button className="btn-icon" onClick={onHelp} title="Keyboard shortcuts (?)">
          ?
        </button>
      </div>
    </header>
  )
}
