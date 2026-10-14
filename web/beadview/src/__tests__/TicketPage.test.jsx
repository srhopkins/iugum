import React from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { cleanup, render, screen } from '@testing-library/react'
import TicketPage from '../components/TicketPage'

afterEach(() => {
  cleanup()
  vi.unstubAllGlobals()
})

describe('TicketPage', () => {
  it('loads from one request and shows comments, children and blockees from it', async () => {
    const bead = {
      id: 'demo-1', title: 'Epic', status: 'open', priority: 'p1', type: 'epic', labels: [],
      parent: '', blocked_by: [], blocks: ['demo-3'], depends_on: [], dependency_edges: [],
      comments: [{ id: 'c1', author: 'amy', text: 'first comment', created_at: '' }],
      dependents: [
        { id: 'demo-1.1', title: 'Child task', status: 'open', dependency_type: 'parent-child' },
        { id: 'demo-3', title: 'Blocked task', status: 'blocked', dependency_type: 'blocks' },
      ],
    }
    const fetchMock = vi.fn(async (url) => {
      const path = String(url).split('?')[0]
      if (path === '/api/config') return new Response(JSON.stringify({ read_only: true }))
      if (path === '/api/bead/demo/demo-1') return new Response(JSON.stringify(bead))
      return new Response('{"error":"no"}', { status: 404 })
    })
    vi.stubGlobal('fetch', fetchMock)
    render(<TicketPage project="demo" id="demo-1" />)
    await screen.findByText('first comment')
    expect(screen.getByText('Child task')).toBeTruthy()
    expect(screen.getByText('Blocked task')).toBeTruthy()
    const paths = fetchMock.mock.calls.map((c) => String(c[0]).split('?')[0])
    expect(paths).not.toContain('/api/beads')
    expect(paths.filter((p) => p.endsWith('/comments'))).toEqual([])
  })
})
