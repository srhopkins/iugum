import React from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'

vi.mock('mermaid', () => ({
  default: { initialize: vi.fn(), render: vi.fn().mockResolvedValue({ svg: '<svg></svg>' }) },
}))

import App from '../App'

const BEADS = [
  { id: 'demo-1', title: 'Epic', status: 'open', priority: 'p0', type: 'epic', depends_on: [], labels: ['ui'], assignee: 'alice', updated_at: '2026-01-02' },
  { id: 'demo-1.1', title: 'Child done', status: 'closed', priority: 'p2', type: 'task', parent: 'demo-1', depends_on: ['demo-1'], labels: [], assignee: '', updated_at: '2026-01-01' },
  { id: 'demo-1.2', title: 'Child open', status: 'in_progress', priority: 'p1', type: 'task', parent: 'demo-1', depends_on: ['demo-1'], labels: [], assignee: 'bob', updated_at: '2026-01-03' },
  { id: 'demo-2', title: 'Blocked thing', status: 'blocked', priority: 'p4', type: 'bug', depends_on: [], labels: [], assignee: '', updated_at: '2026-01-04' },
]

function mockApi({ readOnly = false } = {}) {
  const calls = []
  const fetchMock = vi.fn(async (url, opts = {}) => {
    calls.push({ url, method: opts.method || 'GET' })
    const path = url.split('?')[0]
    const json = (v) => new Response(JSON.stringify(v), { headers: { 'Content-Type': 'application/json' } })
    if (path === '/api/beads') return json(BEADS)
    if (path === '/api/mermaid') return new Response('')
    if (path === '/api/config') return json({ ws_port: null, read_only: readOnly, project: 'demo' })
    if (path === '/api/projects') return json({ projects: [{ slug: 'demo', description: '/tmp/demo' }] })
    if (/^\/api\/bead\/[^/]+\/comments$/.test(path)) return json([])
    return new Response('{"error":"no"}', { status: 404 })
  })
  vi.stubGlobal('fetch', fetchMock)
  return calls
}

beforeEach(() => {
  window.history.replaceState(null, '', '/')
  try {
    localStorage.clear()
  } catch {
    /* ignore */
  }
})

afterEach(() => {
  cleanup()
  vi.unstubAllGlobals()
})

describe('App', () => {
  it('renders the header, stats, tabs, filters and tree', async () => {
    const calls = mockApi()
    render(<App />)
    await screen.findByText('Epic')

    expect(screen.getByRole('heading', { name: 'iugum beads' })).toBeTruthy()
    for (const label of ['Total', 'Open', 'In Progress', 'Blocked', 'Closed', 'Completion']) {
      expect(screen.getAllByText(label).length).toBeGreaterThan(0)
    }
    for (const tab of ['Tree', 'Activity', 'Pipeline', 'Graph']) {
      expect(screen.getByRole('button', { name: new RegExp(`^${tab}`) })).toBeTruthy()
    }
    for (const f of ['Status', 'Priority', 'Type', 'Labels', 'Assignee']) {
      expect(screen.getByRole('button', { name: new RegExp(`^${f}`) })).toBeTruthy()
    }
    expect(screen.getByText('Showing 4 issues')).toBeTruthy()
    expect(screen.getByText('Polling')).toBeTruthy()
    expect(screen.getByRole('button', { name: '+ New Bead' })).toBeTruthy()
    await waitFor(() => expect(screen.getByRole('combobox', { name: 'Project' }).value).toBe('demo'))

    // Epic is a parent of two children, one closed: 50%.
    expect(screen.getByText('50%')).toBeTruthy()
    expect(screen.queryByText('Child done')).toBeNull()
    fireEvent.click(screen.getByRole('button', { name: 'Expand All' }))
    expect(screen.getByText('Child done')).toBeTruthy()
    fireEvent.click(screen.getByRole('button', { name: 'Collapse All' }))
    expect(screen.queryByText('Child done')).toBeNull()

    const paths = new Set(calls.map((c) => c.url.split('?')[0]))
    expect(paths).toEqual(new Set(['/api/beads', '/api/mermaid', '/api/config', '/api/projects']))
  })

  it('filters on P0 from the priority menu', async () => {
    mockApi()
    render(<App />)
    await screen.findByText('Epic')
    fireEvent.click(screen.getByRole('button', { name: /^Priority/ }))
    fireEvent.click(screen.getByRole('checkbox', { name: 'p0' }))
    expect(screen.getByText('Showing 1 of 4 issues')).toBeTruthy()
  })

  it('switches views with number keys and focuses search with /', async () => {
    mockApi()
    render(<App />)
    await screen.findByText('Epic')
    act(() => {
      fireEvent.keyDown(document.body, { key: '3' })
    })
    expect(screen.getByText('Blocked thing').closest('.kanban-card')).toBeTruthy()
    act(() => {
      fireEvent.keyDown(document.body, { key: '1' })
    })
    expect(screen.getByRole('button', { name: 'Expand All' })).toBeTruthy()
    act(() => {
      fireEvent.keyDown(document.body, { key: '/' })
    })
    expect(document.activeElement).toBe(screen.getByPlaceholderText('Search beads... ( / )'))
  })

  it('hides write controls when the server is read-only', async () => {
    mockApi({ readOnly: true })
    render(<App />)
    await screen.findByText('Epic')
    await waitFor(() => expect(screen.queryByRole('button', { name: '+ New Bead' })).toBeNull())
  })
})
