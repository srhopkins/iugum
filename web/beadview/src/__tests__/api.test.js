import { afterEach, describe, expect, it, vi } from 'vitest'
import { apiFetch, apiPath, withProject } from '../api'

describe('withProject', () => {
  it('appends project query param', () => {
    expect(withProject('/api/beads', 'demo')).toBe('/api/beads?project=demo')
  })

  it('replaces existing project param', () => {
    expect(withProject('/api/beads?project=old&q=x', 'demo')).toBe('/api/beads?project=demo&q=x')
  })

  it('no-ops without project', () => {
    expect(withProject('/api/beads', '')).toBe('/api/beads')
  })
})

describe('apiPath', () => {
  it('keeps /api paths as-is and never adds a /bd segment', () => {
    expect(apiPath('/api/beads')).toBe('/api/beads')
    expect(apiPath('/api/bead/iugum-puu/comments')).toBe('/api/bead/iugum-puu/comments')
  })

  it('prefixes short paths with /api', () => {
    expect(apiPath('/beads')).toBe('/api/beads')
    expect(apiPath('config')).toBe('/api/config')
  })
})

describe('apiFetch', () => {
  afterEach(() => vi.unstubAllGlobals())

  it('calls a relative same-origin /api URL', async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response('[]'))
    vi.stubGlobal('fetch', fetchMock)
    await apiFetch('/api/beads', { project: 'demo' })
    expect(fetchMock).toHaveBeenCalledTimes(1)
    const [url, opts] = fetchMock.mock.calls[0]
    expect(url).toBe('/api/beads?project=demo')
    expect(opts.credentials).toBe('same-origin')
  })

  it('sends JSON Content-Type and project in write bodies', async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response('{}'))
    vi.stubGlobal('fetch', fetchMock)
    await apiFetch('/api/bead/iugum-puu', {
      method: 'PATCH',
      body: JSON.stringify({ status: 'closed' }),
      project: 'demo',
    })
    const [, opts] = fetchMock.mock.calls[0]
    expect(opts.headers.get('Content-Type')).toBe('application/json')
    expect(JSON.parse(opts.body)).toEqual({ status: 'closed', project: 'demo' })
    expect(opts.project).toBeUndefined()
  })
})
