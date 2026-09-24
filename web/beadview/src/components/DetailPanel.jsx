import React, { useState, useEffect, useCallback } from 'react'
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import StatusBadge from './StatusBadge'
import PriorityBadge from './PriorityBadge'
import { apiFetch, apiWrite } from '../api'
import { PRIORITIES, depIds, normalizePriority } from '../filters'

function DepRow({ depId, allBeads, onSelectBead }) {
  const depBead = allBeads.find((b) => b.id === depId)

  return (
    <div
      className="dep-item"
      onClick={() => onSelectBead?.(depId)}
      role="button"
      tabIndex={0}
      onKeyDown={(e) => {
        if (e.key === 'Enter' || e.key === ' ') onSelectBead?.(depId)
      }}
      title={depId}
    >
      <span className="bead-id">{depId}</span>
      {depBead && <StatusBadge status={depBead.status} />}
      <span style={{ fontSize: 12, color: 'var(--text-secondary)' }}>
        {depBead?.title || ''}
      </span>
    </div>
  )
}

export default function DetailPanel({
  bead,
  open,
  onClose,
  onRefresh,
  allBeads,
  onSelectBead,
  project,
  readOnly = false,
}) {
  const [editing, setEditing] = useState(false)
  const [editDesc, setEditDesc] = useState('')
  const [editStatus, setEditStatus] = useState('')
  const [editPriority, setEditPriority] = useState('')
  const [comments, setComments] = useState([])
  const [newComment, setNewComment] = useState('')
  const [saving, setSaving] = useState(false)
  const [copyState, setCopyState] = useState('idle') // idle | copied | error
  const [writeError, setWriteError] = useState(null)

  useEffect(() => {
    if (bead) {
      setEditDesc(bead.description || bead.body || '')
      setEditStatus(bead.status || 'open')
      setEditPriority(normalizePriority(bead.priority))
      setEditing(false)
      setNewComment('')
      setCopyState('idle')
      setWriteError(null)
      fetchComments(bead.id)
    }
  }, [bead?.id, project])

  function ticketUrl(beadId) {
    if (typeof window === 'undefined') return ''
    const params = new URLSearchParams()
    if (project) params.set('project', project)
    params.set('bead', beadId)
    return `${window.location.origin}${window.location.pathname}?${params.toString()}`
  }

  async function handleCopyUrl() {
    if (!bead) return
    const url = ticketUrl(bead.id)
    try {
      if (navigator.clipboard?.writeText) {
        await navigator.clipboard.writeText(url)
      } else {
        const ta = document.createElement('textarea')
        ta.value = url
        ta.setAttribute('readonly', '')
        ta.style.position = 'fixed'
        ta.style.left = '-9999px'
        document.body.appendChild(ta)
        ta.select()
        document.execCommand('copy')
        document.body.removeChild(ta)
      }
      setCopyState('copied')
      window.setTimeout(() => setCopyState('idle'), 1600)
    } catch {
      setCopyState('error')
      window.setTimeout(() => setCopyState('idle'), 2000)
    }
  }

  const fetchComments = useCallback(
    async (beadId) => {
      try {
        const resp = await apiFetch(`/api/bead/${beadId}/comments`, {
          project,
        })
        if (resp.ok) {
          const data = await resp.json()
          setComments(Array.isArray(data) ? data : [])
        }
      } catch {
        setComments([])
      }
    },
    [project]
  )

  /** Run one write. Show the server's error text when it refuses. */
  async function write(path, method, body) {
    setWriteError(null)
    const res = await apiWrite(path, { method, body: { ...body, project }, project })
    if (!res.ok) setWriteError(res.error)
    return res.ok
  }

  async function handleSave() {
    if (!bead) return
    setSaving(true)
    try {
      const payload = { project }
      if (editStatus !== bead.status) payload.status = editStatus
      if (editPriority && editPriority !== normalizePriority(bead.priority)) {
        payload.priority = editPriority
      }
      if (editDesc !== (bead.description || bead.body || '')) {
        payload.description = editDesc
      }

      if (Object.keys(payload).length > 1) {
        const ok = await write(`/api/bead/${bead.id}`, 'PATCH', payload)
        onRefresh()
        if (!ok) return
      }
      setEditing(false)
    } finally {
      setSaving(false)
    }
  }

  async function handleClaim() {
    if (!bead) return
    await write(`/api/bead/${bead.id}`, 'PATCH', { claim: true })
    onRefresh()
  }

  async function handleClose() {
    if (!bead) return
    await write(`/api/bead/${bead.id}/close`, 'POST', {})
    onRefresh()
  }

  async function handleReopen() {
    if (!bead) return
    await write(`/api/bead/${bead.id}/reopen`, 'POST', {})
    onRefresh()
  }

  async function handleAddComment() {
    if (!bead || !newComment.trim()) return
    const ok = await write(`/api/bead/${bead.id}/comments`, 'POST', { text: newComment })
    if (ok) setNewComment('')
    fetchComments(bead.id)
  }

  const deps = bead ? depIds(bead) : []
  const dependents = bead ? allBeads.filter((b) => depIds(b).includes(bead.id)) : []

  return (
    <div className={`detail-panel${open ? ' open' : ''}`}>
      {bead && (
        <>
          <div className="detail-panel-header">
            <h2>
              <button
                type="button"
                className="bead-id bead-id-copy"
                onClick={handleCopyUrl}
                title="Copy ticket URL"
              >
                {bead.id}
              </button>
              {bead.title}
            </h2>
            <div className="detail-panel-header-actions">
              <button
                type="button"
                className="btn btn-sm detail-copy-btn"
                onClick={handleCopyUrl}
                title="Copy ticket URL"
              >
                {copyState === 'copied'
                  ? 'Copied'
                  : copyState === 'error'
                    ? 'Copy failed'
                    : 'Copy link'}
              </button>
              <button className="btn-icon" onClick={onClose} title="Close (Esc)">
                &#x2715;
              </button>
            </div>
          </div>

          <div className="detail-panel-body">
            <div className="detail-section">
              <h3>Status</h3>
              {editing ? (
                <div className="detail-field">
                  <select value={editStatus} onChange={(e) => setEditStatus(e.target.value)}>
                    <option value="open">Open</option>
                    <option value="in_progress">In Progress</option>
                    <option value="blocked">Blocked</option>
                    <option value="closed">Closed</option>
                  </select>
                </div>
              ) : (
                <StatusBadge status={bead.status} />
              )}
            </div>

            <div className="detail-section">
              <h3>Priority</h3>
              {editing ? (
                <div className="detail-field">
                  <select value={editPriority} onChange={(e) => setEditPriority(e.target.value)}>
                    {!editPriority && <option value="">None</option>}
                    {PRIORITIES.map((p) => (
                      <option key={p} value={p}>
                        {p.toUpperCase()}
                      </option>
                    ))}
                  </select>
                </div>
              ) : (
                <PriorityBadge priority={bead.priority} />
              )}
            </div>

            {bead.labels && bead.labels.length > 0 && (
              <div className="detail-section">
                <h3>Labels</h3>
                <div style={{ display: 'flex', gap: 6, flexWrap: 'wrap' }}>
                  {bead.labels.map((label) => (
                    <span
                      key={label}
                      className="badge"
                      style={{
                        background: 'var(--bg-tertiary)',
                        color: 'var(--text-secondary)',
                      }}
                    >
                      {label}
                    </span>
                  ))}
                </div>
              </div>
            )}

            <div className="detail-section">
              <h3>Assignee</h3>
              <p>{bead.assignee || bead.actor || 'Unassigned'}</p>
            </div>

            <div className="detail-section">
              <h3>Dates</h3>
              <div style={{ fontSize: 13, color: 'var(--text-secondary)' }}>
                {bead.created_at && (
                  <div>Created: {new Date(bead.created_at).toLocaleString()}</div>
                )}
                {bead.updated_at && (
                  <div>Updated: {new Date(bead.updated_at).toLocaleString()}</div>
                )}
              </div>
            </div>

            <div className="detail-section">
              <h3>Description</h3>
              {editing ? (
                <div className="detail-field">
                  <textarea
                    value={editDesc}
                    onChange={(e) => setEditDesc(e.target.value)}
                    placeholder="Description (Markdown supported)"
                  />
                </div>
              ) : (
                <div className="description-content">
                  {bead.description || bead.body ? (
                    <ReactMarkdown remarkPlugins={[remarkGfm]}>
                      {bead.description || bead.body}
                    </ReactMarkdown>
                  ) : (
                    <span style={{ color: 'var(--text-muted)', fontStyle: 'italic' }}>
                      No description
                    </span>
                  )}
                </div>
              )}
            </div>

            {deps.length > 0 && (
              <div className="detail-section">
                <h3>Dependencies ({deps.length})</h3>
                <div className="dep-list">
                  {deps.map((depId) => (
                    <DepRow
                      key={depId}
                      depId={depId}
                      allBeads={allBeads}
                      onSelectBead={onSelectBead}
                    />
                  ))}
                </div>
              </div>
            )}

            {dependents.length > 0 && (
              <div className="detail-section">
                <h3>Dependents ({dependents.length})</h3>
                <div className="dep-list">
                  {dependents.map((dep) => (
                    <div
                      key={dep.id}
                      className="dep-item"
                      onClick={() => onSelectBead(dep.id)}
                    >
                      <span className="bead-id">{dep.id}</span>
                      <StatusBadge status={dep.status} />
                      <span style={{ fontSize: 12, color: 'var(--text-secondary)' }}>
                        {dep.title}
                      </span>
                    </div>
                  ))}
                </div>
              </div>
            )}

            <div className="detail-section">
              <h3>Comments</h3>
              <div className="comments-list">
                {Array.isArray(comments) && comments.length > 0 ? (
                  comments.map((c, i) => (
                    <div key={i} className="comment-item">
                      {c.author && (
                        <div className="comment-header">
                          {c.author} {c.created_at && `- ${c.created_at}`}
                        </div>
                      )}
                      <div className="comment-body">
                        {c.text || c.body || c.content || JSON.stringify(c)}
                      </div>
                    </div>
                  ))
                ) : (
                  <span style={{ fontSize: 13, color: 'var(--text-muted)' }}>
                    No comments yet
                  </span>
                )}
              </div>
              {!readOnly && (
              <div className="comment-input">
                <input
                  type="text"
                  placeholder="Add a comment..."
                  value={newComment}
                  onChange={(e) => setNewComment(e.target.value)}
                  onKeyDown={(e) => {
                    if (e.key === 'Enter') handleAddComment()
                  }}
                />
                <button
                  className="btn btn-sm btn-primary"
                  onClick={handleAddComment}
                  disabled={!newComment.trim()}
                >
                  Add
                </button>
              </div>
              )}
            </div>
          </div>

          {writeError && (
            <div className="write-error" role="alert">
              {writeError}
            </div>
          )}

          {!readOnly && (
          <div className="detail-actions">
            {editing ? (
              <>
                <button
                  className="btn btn-primary btn-sm"
                  onClick={handleSave}
                  disabled={saving}
                >
                  {saving ? 'Saving...' : 'Save'}
                </button>
                <button className="btn btn-sm" onClick={() => setEditing(false)}>
                  Cancel
                </button>
              </>
            ) : (
              <>
                <button className="btn btn-sm" onClick={() => setEditing(true)}>
                  Edit
                </button>
                <button className="btn btn-sm" onClick={handleClaim}>
                  Claim
                </button>
                {bead.status === 'closed' ? (
                  <button className="btn btn-sm btn-primary" onClick={handleReopen}>
                    Reopen
                  </button>
                ) : (
                  <button className="btn btn-sm btn-success" onClick={handleClose}>
                    Close
                  </button>
                )}
              </>
            )}
          </div>
          )}
        </>
      )}
    </div>
  )
}
