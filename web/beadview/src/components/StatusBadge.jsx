import React from 'react'

const STATUS_LABELS = {
  open: 'Open',
  in_progress: 'In Progress',
  blocked: 'Blocked',
  closed: 'Closed',
}

export default function StatusBadge({ status }) {
  if (!status) return null
  const label = STATUS_LABELS[status] || status
  return <span className={`badge badge-${status}`}>{label}</span>
}
