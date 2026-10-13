import React from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { cleanup, fireEvent, render, screen, within } from '@testing-library/react'
import BoardView, { COLUMNS, columnFor } from '../components/BoardView'
import ListView from '../components/ListView'
import ProjectChips from '../components/ProjectChips'
import { hashText, beadPath, ticketHref } from '../api'
import { filterAndSortBeads } from '../filters'

afterEach(cleanup)

const BEADS = [
  { id: 'a-1', project: 'alpha', title: 'Alpha backlog', status: 'deferred', priority: 'p3', assignee: 'sam', labels: ['x'] },
  { id: 'a-2', project: 'alpha', title: 'Alpha ready', status: 'open', priority: 'p0', labels: [] },
  { id: 'b-1', project: 'beta', title: 'Beta doing', status: 'in_progress', priority: 'p1', assignee: 'kim', labels: ['x', 'y'] },
  { id: 'b-2', project: 'beta', title: 'Beta done', status: 'closed', priority: 'p2', labels: [] },
]

describe('BoardView', () => {
  it('orders columns deferred, open, in_progress, blocked, closed with their labels', () => {
    expect(COLUMNS.map((c) => c.key)).toEqual(['deferred', 'open', 'in_progress', 'blocked', 'closed'])
    expect(COLUMNS.map((c) => c.label)).toEqual(['Deferred', 'Ready', 'In Progress', 'Blocked', 'Closed'])
    expect(columnFor({ status: 'pinned' })).toBe('open')
  })

  it('shows a project chip, id, title, priority and assignee on each card', () => {
    render(<BoardView beads={BEADS} projects={['alpha', 'beta']} onSelect={() => {}} />)
    const card = screen.getByText('Beta doing').closest('.kanban-card')
    expect(within(card).getByText('beta')).toBeTruthy()
    expect(within(card).getByText('b-1')).toBeTruthy()
    expect(within(card).getByText('P1')).toBeTruthy()
    expect(within(card).getByText('kim')).toBeTruthy()
  })

  it('reports a drop with the bead, so the write goes to its own project', () => {
    const onStatusChange = vi.fn()
    const { container } = render(
      <BoardView beads={BEADS} projects={['alpha', 'beta']} onSelect={() => {}} onStatusChange={onStatusChange} />
    )
    const card = screen.getByText('Alpha ready').closest('.kanban-card')
    const dt = { setData: vi.fn(), effectAllowed: '' }
    fireEvent.dragStart(card, { dataTransfer: dt })
    fireEvent.drop(container.querySelector('[data-column="blocked"]'), { dataTransfer: dt })
    expect(onStatusChange).toHaveBeenCalledTimes(1)
    expect(onStatusChange.mock.calls[0][0]).toMatchObject({ id: 'a-2', project: 'alpha' })
    expect(onStatusChange.mock.calls[0][1]).toBe('blocked')
  })

  it('makes one row per project with swimlanes, and ignores a drop into another project row', () => {
    const onStatusChange = vi.fn()
    const { container } = render(
      <BoardView beads={BEADS} projects={['alpha', 'beta']} swimlanes onSelect={() => {}} onStatusChange={onStatusChange} />
    )
    expect(container.querySelectorAll('.board-lane').length).toBe(2)
    const card = screen.getByText('Alpha ready').closest('.kanban-card')
    const dt = { setData: vi.fn(), effectAllowed: '' }
    fireEvent.dragStart(card, { dataTransfer: dt })
    fireEvent.drop(container.querySelector('[data-lane="beta"][data-column="closed"]'), { dataTransfer: dt })
    expect(onStatusChange).not.toHaveBeenCalled()
    fireEvent.dragStart(card, { dataTransfer: dt })
    fireEvent.drop(container.querySelector('[data-lane="alpha"][data-column="closed"]'), { dataTransfer: dt })
    expect(onStatusChange).toHaveBeenCalledTimes(1)
  })

  it('has no drag handles when read-only', () => {
    render(<BoardView beads={BEADS} projects={['alpha', 'beta']} onSelect={() => {}} />)
    expect(screen.getByText('Alpha ready').closest('.kanban-card').getAttribute('draggable')).toBe('false')
  })
})

describe('ProjectChips', () => {
  it('toggles one project off and shift-click shows only one', () => {
    const onChange = vi.fn()
    render(<ProjectChips projects={['alpha', 'beta', 'gamma']} hidden={[]} onChange={onChange} />)
    fireEvent.click(screen.getByRole('button', { name: /^beta/ }))
    expect(onChange).toHaveBeenLastCalledWith(['beta'])
    fireEvent.click(screen.getByRole('button', { name: /^beta/ }), { shiftKey: true })
    expect(onChange).toHaveBeenLastCalledWith(['alpha', 'gamma'])
  })
})

describe('filters', () => {
  it('hides projects and filters by project-independent fields', () => {
    const base = { search: '', status: [], priority: [], type: [], assignee: [], labels: [], sort: 'priority' }
    const out = filterAndSortBeads(BEADS, { ...base, hiddenProjects: ['alpha'] })
    expect(out.map((b) => b.id)).toEqual(['b-1', 'b-2'])
    const labelled = filterAndSortBeads(BEADS, { ...base, labels: ['x'] })
    expect(labelled.map((b) => b.id).sort()).toEqual(['a-1', 'b-1'])
  })
})

describe('ListView', () => {
  it('sorts by a clicked column and reverses on a second click', () => {
    render(<ListView beads={BEADS} onSelect={() => {}} />)
    const ids = () => screen.getAllByRole('row').slice(1).map((r) => r.querySelector('.bead-id').textContent)
    fireEvent.click(screen.getByText(/^Priority/))
    expect(ids()).toEqual(['a-2', 'b-1', 'b-2', 'a-1'])
    fireEvent.click(screen.getByText(/^Priority/))
    expect(ids()).toEqual(['a-1', 'b-2', 'b-1', 'a-2'])
  })

  it('links each id to its full page', () => {
    render(<ListView beads={BEADS} onSelect={() => {}} />)
    expect(screen.getByText('b-1').getAttribute('href')).toBe('/bead/beta/b-1')
  })
})

describe('api helpers', () => {
  it('builds project paths and hashes text', () => {
    expect(beadPath('beta', 'b-1')).toBe('/api/bead/beta/b-1')
    expect(ticketHref('beta', 'b-1')).toBe('/bead/beta/b-1')
    expect(hashText('abc')).toBe(hashText('abc'))
    expect(hashText('abc')).not.toBe(hashText('abd'))
  })
})
