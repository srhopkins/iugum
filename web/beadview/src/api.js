/** Fetch helper for the iugum beadview JSON API, served on the same origin under /api. */

export const API_BASE = '/api'

/** Append or replace the `project` query param. */
export function withProject(path, project) {
  if (!project) return path
  const [base, qs = ''] = path.split('?')
  const params = new URLSearchParams(qs)
  params.set('project', project)
  const out = params.toString()
  return out ? `${base}?${out}` : base
}

/** Resolve a short path (`/beads`) or a full one (`/api/beads`) to `/api/...`. */
export function apiPath(path) {
  const p = path.startsWith('/') ? path : `/${path}`
  if (p === API_BASE || p.startsWith(`${API_BASE}/`) || p.startsWith(`${API_BASE}?`)) return p
  return `${API_BASE}${p}`
}

/**
 * fetch() against the beadview API. Optional `project` goes into the query
 * string, and into JSON bodies for POST/PATCH/PUT when the body has none.
 */
export async function apiFetch(path, options = {}) {
  const headers = new Headers(options.headers || {})
  if (options.body && !headers.has('Content-Type')) {
    headers.set('Content-Type', 'application/json')
  }
  let p = apiPath(path)

  const project = options.project
  if (project) {
    p = withProject(p, project)
    if (
      options.body &&
      typeof options.body === 'string' &&
      (options.method === 'POST' || options.method === 'PATCH' || options.method === 'PUT')
    ) {
      try {
        const body = JSON.parse(options.body)
        if (body && typeof body === 'object' && body.project == null) {
          body.project = project
          options = { ...options, body: JSON.stringify(body) }
        }
      } catch {
        /* leave body as-is */
      }
    }
  }

  const { project: _drop, ...fetchOpts } = options
  return fetch(p, { ...fetchOpts, headers, credentials: 'same-origin' })
}
