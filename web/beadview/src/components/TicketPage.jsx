import React, { useCallback, useEffect, useRef, useState } from 'react'
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import ConfirmClose from './ConfirmClose'
import StatusBadge from './StatusBadge'
import PriorityBadge from './PriorityBadge'
import { ProjectTag } from './ProjectChips'
import { apiFetch, apiWrite, beadPath, hashText, ticketHref } from '../api'
import { PRIORITIES, depIds, normalizePriority } from '../filters'

const STATUSES = [
  ['deferred', 'Deferred'],
  ['open', 'Ready'],
  ['in_progress', 'In Progress'],
  ['blocked', 'Blocked'],
  ['closed', 'Closed'],
]

function fmtDate(s) {
  if (!s) return ''
  const d = new Date(s)
  return Number.isNaN(d.getTime()) ? s : d.toLocaleString()
}

function BeadLink({ project, id, bead }) {
  return (
    <li className="ticket-link-row">
      <a className="bead-id" href={ticketHref(project, id)}>
        {id}
      </a>
      {bead && <StatusBadge status={bead.status} />}
      <span className="ticket-link-title">{bead?.title || ''}</span>
    </li>
  )
}

function Field({ label, children }) {
  return (
    <div className="ticket-field">
      <dt>{label}</dt>
      <dd>{children}</dd>
    </div>
  )
}

/**
 * Full-page ticket at /bead/<project>/<id>. Status and priority edit in
 * place: picking a new value sends the write at once. The page polls every
 * 3 seconds and re-renders only when the data changed.
 */
export default function TicketPage({ project, id }) {
  const [bead, setBead] = useState(null)
  const [error, setError] = useState(null)
  const [writeError, setWriteError] = useState(null)
  const [readOnly, setReadOnly] = useState(true)
  const [newComment, setNewComment] = useState('')
  const [confirmingClose, setConfirmingClose] = useState(false)
  const hashes = useRef({})

  const load = useCallback(async () => {
    try {
      // One request: the bead response carries its comments, dependents
      // and the titles of what it links to.
      const b = await apiFetch(beadPath(project, id))
      if (!b.ok) {
        let msg = `Could not load ${project}/${id} (${b.status})`
        try {
          msg = (await b.json()).error || msg
        } catch {
          /* not JSON */
        }
        setError(msg)
        return
      }
      setError(null)
      const text = await b.text()
      const h = hashText(text)
      if (hashes.current.bead === h) return
      hashes.current.bead = h
      setBead(JSON.parse(text))
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    }
  }, [project, id])

  useEffect(() => {
    document.title = `${id} - iugum beads`
    load()
    const t = setInterval(load, 3000)
    return () => clearInterval(t)
  }, [load, id])

  useEffect(() => {
    apiFetch('/api/config')
      .then((r) => r.json())
      .then((c) => setReadOnly(!!c.read_only))
      .catch(() => {})
  }, [])

  async function patch(fields) {
    setWriteError(null)
    const res = await apiWrite(beadPath(project, id), { method: 'PATCH', body: fields, project })
    if (!res.ok) setWriteError(res.error)
    hashes.current = {}
    load()
  }

  async function addComment() {
    if (!newComment.trim()) return
    setWriteError(null)
    const res = await apiWrite(`${beadPath(project, id)}/comments`, {
      method: 'POST',
      body: { text: newComment },
      project,
    })
    if (!res.ok) setWriteError(res.error)
    else setNewComment('')
    hashes.current = {}
    load()
  }

  // Titles and statuses for linked beads come from the bead response itself.
  const comments = bead?.comments || []
  const byId = new Map()
  for (const e of bead?.dependency_edges || []) {
    if (e.depends_on_id) byId.set(e.depends_on_id, { id: e.depends_on_id, title: e.title, status: e.status })
  }
  for (const d of bead?.dependents || []) byId.set(d.id, d)
  const children = (bead?.dependents || []).filter((d) => d.dependency_type === 'parent-child')
  const parent = bead?.parent ? byId.get(bead.parent) || { id: bead.parent } : null
  const blockedBy = bead ? bead.blocked_by || [] : []
  const blocks = bead ? bead.blocks || [] : []
  const otherDeps = bead ? depIds(bead).filter((d) => !blockedBy.includes(d) && d !== bead.parent) : []

  return (
    <div className="ticket-page">
      <nav className="ticket-nav">
        <a href={`/?project=${encodeURIComponent(project)}&view=pipeline`}>&larr; Board</a>
        <ProjectTag project={project} />
      </nav>
      {error && !bead && (
        <div className="write-error" role="alert">
          {error}
        </div>
      )}
      {!bead && !error && <div className="loading">Loading...</div>}
      {bead && (
        <>
          <div className="ticket-ident">
            <span className="bead-id">{bead.id}</span>
            <ProjectTag project={project} />
          </div>
          <h1 className="ticket-title">{bead.title}</h1>
          {confirmingClose && (
            <ConfirmClose
              id={bead.id}
              onCancel={() => setConfirmingClose(false)}
              onConfirm={() => {
                setConfirmingClose(false)
                patch({ status: 'closed' })
              }}
            />
          )}
          {writeError && (
            <div className="write-error" role="alert">
              {writeError}
            </div>
          )}
          <div className="ticket-grid">
            <section className="ticket-main">
              <h3>Description</h3>
              <div className="description-content">
                {bead.description ? (
                  <ReactMarkdown remarkPlugins={[remarkGfm]}>{bead.description}</ReactMarkdown>
                ) : (
                  <span className="muted">No description</span>
                )}
              </div>
              {bead.notes && (
                <>
                  <h3>Notes</h3>
                  <div className="description-content">
                    <ReactMarkdown remarkPlugins={[remarkGfm]}>{bead.notes}</ReactMarkdown>
                  </div>
                </>
              )}
              {(parent || children.length > 0) && (
                <>
                  <h3>Parent and children</h3>
                  <ul className="ticket-links">
                    {parent && <BeadLink project={project} id={parent.id} bead={parent.title ? parent : null} />}
                    {children.map((c) => (
                      <BeadLink key={c.id} project={project} id={c.id} bead={c} />
                    ))}
                  </ul>
                </>
              )}
              {(blockedBy.length > 0 || blocks.length > 0 || otherDeps.length > 0) && (
                <>
                  <h3>Dependencies</h3>
                  <ul className="ticket-links">
                    {blockedBy.map((d) => (
                      <li key={`by-${d}`} className="ticket-dep-kind">
                        Blocked by
                        <ul>
                          <BeadLink project={project} id={d} bead={byId.get(d)} />
                        </ul>
                      </li>
                    ))}
                    {blocks.map((d) => (
                      <li key={`bl-${d}`} className="ticket-dep-kind">
                        Blocks
                        <ul>
                          <BeadLink project={project} id={d} bead={byId.get(d)} />
                        </ul>
                      </li>
                    ))}
                    {otherDeps.map((d) => (
                      <li key={`ot-${d}`} className="ticket-dep-kind">
                        Related
                        <ul>
                          <BeadLink project={project} id={d} bead={byId.get(d)} />
                        </ul>
                      </li>
                    ))}
                  </ul>
                </>
              )}
              <h3>Comments ({comments.length})</h3>
              <div className="comments-list">
                {comments.length === 0 && <span className="muted">No comments yet</span>}
                {comments.map((c, i) => (
                  <div key={c.id || i} className="comment-item">
                    <div className="comment-header">
                      {c.author} {c.created_at && `- ${fmtDate(c.created_at)}`}
                    </div>
                    <div className="comment-body">{c.text}</div>
                  </div>
                ))}
              </div>
              {!readOnly && (
                <div className="comment-input">
                  <input
                    type="text"
                    placeholder="Add a comment..."
                    value={newComment}
                    onChange={(e) => setNewComment(e.target.value)}
                    onKeyDown={(e) => e.key === 'Enter' && addComment()}
                  />
                  <button className="btn btn-sm btn-primary" disabled={!newComment.trim()} onClick={addComment}>
                    Add
                  </button>
                </div>
              )}
            </section>
            <aside className="ticket-side">
              <dl>
                <Field label="Status">
                  {readOnly ? (
                    <StatusBadge status={bead.status} />
                  ) : (
                    <select
                      aria-label="Status"
                      value={bead.status}
                      onChange={(e) =>
                        e.target.value === 'closed'
                          ? setConfirmingClose(true)
                          : patch({ status: e.target.value })
                      }
                    >
                      {STATUSES.map(([v, l]) => (
                        <option key={v} value={v}>
                          {l}
                        </option>
                      ))}
                    </select>
                  )}
                </Field>
                <Field label="Priority">
                  {readOnly ? (
                    <PriorityBadge priority={bead.priority} />
                  ) : (
                    <select
                      aria-label="Priority"
                      value={normalizePriority(bead.priority)}
                      onChange={(e) => patch({ priority: e.target.value })}
                    >
                      {PRIORITIES.map((p) => (
                        <option key={p} value={p}>
                          {p.toUpperCase()}
                        </option>
                      ))}
                    </select>
                  )}
                </Field>
                <Field label="Type">{bead.type || ''}</Field>
                <Field label="Assignee">{bead.assignee || 'Unassigned'}</Field>
                <Field label="Labels">
                  {(bead.labels || []).length === 0
                    ? 'None'
                    : bead.labels.map((l) => (
                        <span key={l} className="badge label-badge">
                          {l}
                        </span>
                      ))}
                </Field>
                <Field label="Created">{fmtDate(bead.created_at)}</Field>
                <Field label="Updated">{fmtDate(bead.updated_at)}</Field>
                {bead.closed_at && <Field label="Closed">{fmtDate(bead.closed_at)}</Field>}
                {bead.close_reason && <Field label="Close reason">{bead.close_reason}</Field>}
                {bead.owner && <Field label="Owner">{bead.owner}</Field>}
              </dl>
            </aside>
          </div>
        </>
      )}
    </div>
  )
}
