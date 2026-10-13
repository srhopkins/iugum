import React from 'react'
import { projectColorIndex } from '../filters'

/** Small coloured tag with a project name. */
export function ProjectTag({ project }) {
  if (!project) return null
  return (
    <span className={`project-tag project-c${projectColorIndex(project)}`} title={`Project ${project}`}>
      {project}
    </span>
  )
}

/**
 * One chip per project. A chip is on unless its project is in `hidden`.
 * Clicking toggles it. Shift-click shows only that project.
 */
export default function ProjectChips({ projects, hidden, counts = {}, onChange }) {
  if (!projects || projects.length < 2) return null
  const hiddenSet = new Set(hidden || [])

  function toggle(name, e) {
    if (e.shiftKey) {
      onChange(projects.filter((p) => p !== name))
      return
    }
    const next = new Set(hiddenSet)
    if (next.has(name)) next.delete(name)
    else next.add(name)
    onChange(Array.from(next))
  }

  return (
    <div className="project-chips" role="group" aria-label="Projects">
      {projects.map((p) => {
        const on = !hiddenSet.has(p)
        return (
          <button
            key={p}
            type="button"
            aria-pressed={on}
            className={`project-chip project-c${projectColorIndex(p)}${on ? ' on' : ' off'}`}
            onClick={(e) => toggle(p, e)}
            title="Click to show or hide. Shift-click to show only this project."
          >
            {p}
            {counts[p] != null && <span className="project-chip-count">{counts[p]}</span>}
          </button>
        )
      })}
      {hiddenSet.size > 0 && (
        <button type="button" className="btn btn-sm" onClick={() => onChange([])}>
          Show all
        </button>
      )}
    </div>
  )
}
