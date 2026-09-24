/** Pure helpers for the beads viewer: filter, sort, labels, priority. */

/** Every priority bd allows, highest first. */
export const PRIORITIES = ['p0', 'p1', 'p2', 'p3', 'p4']

/** Normalize bd's integer (1) or a string ("1", "p1", "P1") to "p1". */
export function normalizePriority(priority) {
  if (priority === null || priority === undefined || priority === '') return ''
  if (typeof priority === 'number' && Number.isFinite(priority)) return `p${priority}`
  const s = String(priority).trim().toLowerCase()
  if (/^\d+$/.test(s)) return `p${s}`
  return s
}

/** Sort rank for a priority. Unknown or missing sorts after p4. */
export function priorityRank(priority) {
  const m = /^p(\d+)$/.exec(normalizePriority(priority))
  return m ? Number(m[1]) : 99
}

/** A dependency entry is either an ID string or `{depends_on_id}` / `{id}`. */
export function depRefId(dep) {
  if (typeof dep === 'string') return dep
  if (dep && typeof dep === 'object') {
    if (dep.depends_on_id) return String(dep.depends_on_id)
    if (dep.id) return String(dep.id)
  }
  return ''
}

/** Dependency IDs of a bead, as plain strings. */
export function depIds(bead) {
  const raw = bead?.depends_on || bead?.dependencies || []
  if (!Array.isArray(raw)) return []
  return raw.map(depRefId).filter(Boolean)
}

const STATUS_ORDER = { blocked: 0, in_progress: 1, open: 2, closed: 3 }

/** Client-side filter + sort (label filter uses AND, matching `bd --label`). */
export function filterAndSortBeads(beads, filters) {
  let result = [...beads]

  if (filters.search) {
    const q = filters.search.toLowerCase()
    result = result.filter((b) => {
      const labels = (b.labels || []).join(' ').toLowerCase()
      return (
        (b.id || '').toLowerCase().includes(q) ||
        (b.title || '').toLowerCase().includes(q) ||
        (b.description || b.body || '').toLowerCase().includes(q) ||
        labels.includes(q)
      )
    })
  }

  if (filters.status && filters.status.length > 0) {
    result = result.filter((b) => filters.status.includes(b.status || ''))
  }

  if (filters.priority && filters.priority.length > 0) {
    const want = filters.priority.map(normalizePriority)
    result = result.filter((b) => want.includes(normalizePriority(b.priority)))
  }

  if (filters.type && filters.type.length > 0) {
    result = result.filter((b) => filters.type.includes(b.type || ''))
  }

  if (filters.assignee && filters.assignee.length > 0) {
    result = result.filter((b) => filters.assignee.includes(b.assignee || b.actor || ''))
  }

  if (filters.labels && filters.labels.length > 0) {
    result = result.filter((b) => filters.labels.every((l) => (b.labels || []).includes(l)))
  }

  result.sort((a, b) => {
    switch (filters.sort) {
      case 'priority':
        return priorityRank(a.priority) - priorityRank(b.priority)
      case 'status':
        return (STATUS_ORDER[a.status || ''] ?? 9) - (STATUS_ORDER[b.status || ''] ?? 9)
      case 'created':
        return (b.created_at || '').localeCompare(a.created_at || '')
      case 'updated':
      default:
        return (b.updated_at || b.created_at || '').localeCompare(
          a.updated_at || a.created_at || ''
        )
    }
  })

  return result
}

export function uniqueLabelsFromBeads(beads) {
  const set = new Set()
  for (const b of beads) {
    for (const l of b.labels || []) {
      if (l) set.add(l)
    }
  }
  return Array.from(set).sort()
}
