import { describe, expect, it } from 'vitest'
import { PRIORITIES, filterAndSortBeads, normalizePriority, priorityRank } from '../filters'
import { isChildOf, isRoot } from '../components/TreeView'

const beads = [
  { id: 'a', title: 'low', status: 'open', priority: 'p3', updated_at: '2026-01-03' },
  { id: 'b', title: 'critical', status: 'open', priority: 'p0', updated_at: '2026-01-01' },
  { id: 'c', title: 'backlog', status: 'closed', priority: 'p4', updated_at: '2026-01-02' },
  { id: 'd', title: 'none', status: 'blocked', updated_at: '2026-01-04' },
  { id: 'e', title: 'high', status: 'in_progress', priority: 'p1', updated_at: '2026-01-05' },
]

describe('priority', () => {
  it('offers p0 through p4', () => {
    expect(PRIORITIES).toEqual(['p0', 'p1', 'p2', 'p3', 'p4'])
  })

  it('normalizes integers and strings', () => {
    expect(normalizePriority(0)).toBe('p0')
    expect(normalizePriority('0')).toBe('p0')
    expect(normalizePriority('P0')).toBe('p0')
    expect(normalizePriority(undefined)).toBe('')
  })

  it('ranks p0 first and a missing priority last', () => {
    expect(priorityRank('p0')).toBe(0)
    expect(priorityRank('p4')).toBe(4)
    expect(priorityRank(undefined)).toBeGreaterThan(4)
  })
})

describe('filterAndSortBeads', () => {
  const base = { search: '', status: [], priority: [], type: [], assignee: [], labels: [] }

  it('sorts by priority with p0 first (the ayo viewer sorted p0 last)', () => {
    const out = filterAndSortBeads(beads, { ...base, sort: 'priority' })
    expect(out.map((b) => b.id)).toEqual(['b', 'e', 'a', 'c', 'd'])
  })

  it('filters on p0 (the ayo viewer could not)', () => {
    const out = filterAndSortBeads(beads, { ...base, priority: ['p0'], sort: 'updated' })
    expect(out.map((b) => b.id)).toEqual(['b'])
  })

  it('filters on p4 and matches an integer priority', () => {
    const out = filterAndSortBeads([...beads, { id: 'f', priority: 4 }], {
      ...base,
      priority: ['p4'],
    })
    expect(out.map((b) => b.id).sort()).toEqual(['c', 'f'])
  })

  it('sorts by updated, newest first, by default', () => {
    const out = filterAndSortBeads(beads, base)
    expect(out.map((b) => b.id)).toEqual(['e', 'd', 'a', 'c', 'b'])
  })

  it('searches id, title and labels', () => {
    expect(filterAndSortBeads(beads, { ...base, search: 'critical' }).map((b) => b.id)).toEqual(['b'])
  })
})

describe('tree nesting', () => {
  it('nests by parent when a bead has one', () => {
    const child = { id: 'x.1', parent: 'x', depends_on: ['x', 'y'] }
    expect(isChildOf(child, 'x')).toBe(true)
    expect(isChildOf(child, 'y')).toBe(false)
  })

  it('falls back to dependency edges without a parent', () => {
    const b = { id: 'z', depends_on: ['y'] }
    expect(isChildOf(b, 'y')).toBe(true)
  })

  it('is a root when its parent is filtered out', () => {
    const child = { id: 'x.1', parent: 'x', depends_on: ['x'] }
    expect(isRoot(child, new Set(['x.1']))).toBe(true)
    expect(isRoot(child, new Set(['x', 'x.1']))).toBe(false)
  })
})
