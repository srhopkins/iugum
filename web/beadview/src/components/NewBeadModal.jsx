import React, { useState } from 'react'
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import PriorityBadge from './PriorityBadge'

export default function NewBeadModal({ onClose, onCreate, allBeads, project }) {
  const [title, setTitle] = useState('')
  const [priority, setPriority] = useState('')
  const [type, setType] = useState('')
  const [description, setDescription] = useState('')
  const [dependencies, setDependencies] = useState([])
  const [showPreview, setShowPreview] = useState(false)

  function toggleDep(id) {
    setDependencies(prev =>
      prev.includes(id) ? prev.filter(d => d !== id) : [...prev, id]
    )
  }

  function handleSubmit(e) {
    e.preventDefault()
    if (!title.trim()) return
    onCreate({
      title,
      priority,
      type,
      description,
      dependencies,
      project,
    })
  }

  return (
    <div className="modal-overlay" onClick={onClose}>
      <div className="modal" onClick={e => e.stopPropagation()}>
        <div className="modal-header">
          <h2>New Bead</h2>
          <button className="btn-icon" onClick={onClose}>&#x2715;</button>
        </div>

        <form onSubmit={handleSubmit}>
          <div className="modal-body">
            {project && (
              <p style={{ fontSize: 13, color: 'var(--text-secondary)', marginTop: 0 }}>
                Creating in <strong>{project}</strong>
              </p>
            )}
            <div className="detail-field">
              <label>Title *</label>
              <input
                type="text"
                value={title}
                onChange={e => setTitle(e.target.value)}
                placeholder="What needs to be done?"
                autoFocus
              />
            </div>

            <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 12 }}>
              <div className="detail-field">
                <label>Priority</label>
                <select value={priority} onChange={e => setPriority(e.target.value)}>
                  <option value="">None</option>
                  <option value="p0">P0 - Critical</option>
                  <option value="p1">P1 - High</option>
                  <option value="p2">P2 - Medium</option>
                  <option value="p3">P3 - Low</option>
                  <option value="p4">P4 - Backlog</option>
                </select>
              </div>

              <div className="detail-field">
                <label>Type</label>
                <select value={type} onChange={e => setType(e.target.value)}>
                  <option value="">None</option>
                  <option value="task">Task</option>
                  <option value="bug">Bug</option>
                  <option value="feature">Feature</option>
                  <option value="epic">Epic</option>
                  <option value="chore">Chore</option>
                </select>
              </div>
            </div>

            <div className="detail-field">
              <label>Description (Markdown)</label>
              <textarea
                value={description}
                onChange={e => setDescription(e.target.value)}
                placeholder="Describe the work..."
                style={{ minHeight: 100 }}
              />
            </div>

            <div className="detail-field">
              <label>Dependencies</label>
              <div style={{ maxHeight: 150, overflowY: 'auto', border: '1px solid var(--border)', borderRadius: 'var(--radius)', padding: 4 }}>
                {allBeads.filter(b => b.status !== 'closed').map(b => (
                  <label key={b.id} className="multi-select-option">
                    <input
                      type="checkbox"
                      checked={dependencies.includes(b.id)}
                      onChange={() => toggleDep(b.id)}
                    />
                    <span className="bead-id">{b.id}</span>
                    <span style={{ fontSize: 12, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                      {b.title}
                    </span>
                  </label>
                ))}
              </div>
            </div>

            {/* Preview Toggle */}
            <button
              type="button"
              className="btn btn-sm"
              onClick={() => setShowPreview(p => !p)}
              style={{ marginTop: 8 }}
            >
              {showPreview ? 'Hide Preview' : 'Show Preview'}
            </button>

            {showPreview && (
              <div className="preview-card">
                <h4>Preview</h4>
                <div style={{ display: 'flex', gap: 8, alignItems: 'center', marginBottom: 8 }}>
                  <span style={{ fontWeight: 600, fontSize: 14 }}>{title || '(untitled)'}</span>
                  {priority && <PriorityBadge priority={priority} />}
                  {type && <span className="badge" style={{ background: 'var(--bg-tertiary)', color: 'var(--text-secondary)' }}>{type}</span>}
                </div>
                {description && (
                  <div className="description-content" style={{ fontSize: 13 }}>
                    <ReactMarkdown remarkPlugins={[remarkGfm]}>{description}</ReactMarkdown>
                  </div>
                )}
                {dependencies.length > 0 && (
                  <div style={{ fontSize: 12, color: 'var(--text-muted)', marginTop: 8 }}>
                    Dependencies: {dependencies.join(', ')}
                  </div>
                )}
              </div>
            )}
          </div>

          <div className="modal-footer">
            <button type="button" className="btn" onClick={onClose}>Cancel</button>
            <button type="submit" className="btn btn-primary" disabled={!title.trim()}>Create Bead</button>
          </div>
        </form>
      </div>
    </div>
  )
}
