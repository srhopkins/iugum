import React from 'react'
import { normalizePriority } from '../filters'

export default function PriorityBadge({ priority }) {
  const p = normalizePriority(priority)
  if (!p) return null
  return <span className={`badge badge-${p}`}>{p.toUpperCase()}</span>
}
