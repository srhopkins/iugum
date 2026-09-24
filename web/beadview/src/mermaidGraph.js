/**
 * Mermaid flowcharts treat `-` in node IDs as operators, so bd output like
 * `iugum-dus["…"]` fails to render. Sanitize IDs and/or build from beads.
 */

const HYPHEN_ID = /[A-Za-z][A-Za-z0-9]*(?:-[A-Za-z0-9]+)+/g

function safeId(id) {
  return `n_${String(id).replace(/[^a-zA-Z0-9]/g, '_')}`
}

function escapeLabel(text) {
  return String(text || '')
    .replace(/"/g, "'")
    .replace(/[\[\]]/g, '')
    .replace(/\n/g, ' ')
    .slice(0, 72)
}

/** Rewrite hyphenated node IDs outside of quoted labels. */
export function sanitizeMermaidFlowchart(source) {
  if (!source || typeof source !== 'string') return ''
  const parts = source.split('"')
  for (let i = 0; i < parts.length; i += 2) {
    parts[i] = parts[i].replace(HYPHEN_ID, (id) => safeId(id))
  }
  return parts.join('"')
}

function depList(bead) {
  const raw = bead?.depends_on || bead?.dependencies || []
  if (!Array.isArray(raw)) return []
  return raw
    .map((d) => {
      if (typeof d === 'string') return d
      if (d && typeof d === 'object') {
        return d.depends_on_id || d.id || ''
      }
      return ''
    })
    .filter(Boolean)
}

/**
 * Build a Mermaid flowchart from viewer beads.
 * @param {object[]} beads
 * @param {{ root?: string, openOnly?: boolean }} [opts]
 */
export function buildMermaidFromBeads(beads, opts = {}) {
  const { root, openOnly = true } = opts
  if (!Array.isArray(beads) || beads.length === 0) {
    return 'flowchart TD\n  empty["No issues to graph"]'
  }

  const openStatuses = new Set(['open', 'in_progress', 'blocked', 'deferred'])
  let pool = openOnly
    ? beads.filter((b) => openStatuses.has(String(b.status || '').toLowerCase()))
    : beads.slice()

  if (pool.length === 0) pool = beads.slice()

  if (root) {
    const byId = new Map(beads.map((b) => [b.id, b]))
    if (!byId.has(root)) {
      return `flowchart TD\n  empty["Issue ${escapeLabel(root)} not found"]`
    }
    const keep = new Set([root])
    const queue = [root]
    while (queue.length) {
      const id = queue.shift()
      for (const d of depList(byId.get(id))) {
        if (!keep.has(d) && byId.has(d)) {
          keep.add(d)
          queue.push(d)
        }
      }
      for (const other of beads) {
        if (depList(other).includes(id) && !keep.has(other.id)) {
          keep.add(other.id)
          queue.push(other.id)
        }
      }
    }
    pool = beads.filter((b) => keep.has(b.id))
  }

  const lines = ['flowchart TD']
  for (const b of pool) {
    const status =
      b.status === 'closed' ? '✓' : b.status === 'in_progress' ? '◐' : '○'
    const label = escapeLabel(`${status} ${b.id}: ${b.title || ''}`)
    lines.push(`  ${safeId(b.id)}["${label}"]`)
  }

  const poolIds = new Set(pool.map((b) => b.id))
  let edges = 0
  for (const b of pool) {
    for (const d of depList(b)) {
      if (poolIds.has(d)) {
        lines.push(`  ${safeId(b.id)} --> ${safeId(d)}`)
        edges += 1
      }
    }
  }

  if (edges === 0 && !root && pool.length > 12) {
    // Prefer connected subset so the board isn't a wall of isolates
    const withDeps = pool.filter((b) => depList(b).some((d) => poolIds.has(d)))
    const targets = new Set()
    for (const b of withDeps) {
      targets.add(b.id)
      for (const d of depList(b)) {
        if (poolIds.has(d)) targets.add(d)
      }
    }
    if (targets.size > 0) {
      return buildMermaidFromBeads(
        beads.filter((b) => targets.has(b.id)),
        { openOnly: false }
      )
    }
  }

  return lines.join('\n')
}

/** Prefer a beads-built graph; fall back to sanitized API mermaid. */
export function resolveMermaidSource(beads, apiMermaid, opts = {}) {
  const built = buildMermaidFromBeads(beads, opts)
  if (built.includes('-->')) return built
  if (apiMermaid && apiMermaid.trim()) {
    return sanitizeMermaidFlowchart(apiMermaid)
  }
  return built
}
