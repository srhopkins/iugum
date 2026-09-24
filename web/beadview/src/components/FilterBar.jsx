import React, { useState, useRef, useEffect } from 'react'
import { PRIORITIES } from '../filters'

function MultiSelect({ label, options, selected, onChange }) {
  const [open, setOpen] = useState(false)
  const ref = useRef(null)

  useEffect(() => {
    function handleClick(e) {
      if (ref.current && !ref.current.contains(e.target)) {
        setOpen(false)
      }
    }
    document.addEventListener('mousedown', handleClick)
    return () => document.removeEventListener('mousedown', handleClick)
  }, [])

  function toggle(value) {
    if (selected.includes(value)) {
      onChange(selected.filter(s => s !== value))
    } else {
      onChange([...selected, value])
    }
  }

  const displayText = selected.length > 0
    ? `${label} (${selected.length})`
    : label

  return (
    <div className="multi-select-dropdown" ref={ref}>
      <button
        className="multi-select-trigger"
        onClick={() => setOpen(o => !o)}
        type="button"
      >
        {displayText}
        <span style={{ marginLeft: 'auto', fontSize: 10 }}>{open ? '\u25B2' : '\u25BC'}</span>
      </button>
      {open && (
        <div className="multi-select-menu">
          {options.map(opt => (
            <label key={opt} className="multi-select-option">
              <input
                type="checkbox"
                checked={selected.includes(opt)}
                onChange={() => toggle(opt)}
              />
              {opt}
            </label>
          ))}
          {options.length === 0 && (
            <div style={{ padding: '8px 12px', fontSize: 12, color: 'var(--text-muted)' }}>
              No options
            </div>
          )}
        </div>
      )}
    </div>
  )
}

export default function FilterBar({
  filters,
  onFilterChange,
  assignees,
  types,
  labels = [],
  searchInputRef,
  showingCount,
  totalCount,
}) {
  function setField(field, value) {
    onFilterChange({ ...filters, [field]: value })
  }

  const showCount =
    typeof showingCount === 'number' && typeof totalCount === 'number'
  const filtered = showCount && showingCount !== totalCount

  return (
    <div className="filter-bar-wrap">
      <div className="filter-bar">
        <input
          ref={searchInputRef}
          type="text"
          placeholder="Search beads... ( / )"
          value={filters.search}
          onChange={e => setField('search', e.target.value)}
        />

        <MultiSelect
          label="Status"
          options={['open', 'in_progress', 'blocked', 'closed']}
          selected={filters.status}
          onChange={v => setField('status', v)}
        />

        <MultiSelect
          label="Priority"
          options={PRIORITIES}
          selected={filters.priority}
          onChange={v => setField('priority', v)}
        />

        {types.length > 0 && (
          <MultiSelect
            label="Type"
            options={types}
            selected={filters.type}
            onChange={v => setField('type', v)}
          />
        )}

        {labels.length > 0 && (
          <MultiSelect
            label="Labels"
            options={labels}
            selected={filters.labels || []}
            onChange={v => setField('labels', v)}
          />
        )}

        {assignees.length > 0 && (
          <MultiSelect
            label="Assignee"
            options={assignees}
            selected={filters.assignee}
            onChange={v => setField('assignee', v)}
          />
        )}

        <div className="sort-controls">
          <span style={{ fontSize: 12, color: 'var(--text-muted)' }}>Sort:</span>
          <select
            value={filters.sort}
            onChange={e => setField('sort', e.target.value)}
          >
            <option value="updated">Updated</option>
            <option value="created">Created</option>
            <option value="priority">Priority</option>
            <option value="status">Status</option>
          </select>
        </div>
      </div>
      {showCount && (
        <div className="filter-results-count" aria-live="polite">
          {filtered
            ? `Showing ${showingCount} of ${totalCount} issues`
            : `Showing ${showingCount} issues`}
        </div>
      )}
    </div>
  )
}
