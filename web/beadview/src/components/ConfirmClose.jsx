import React from 'react'

/** Inline confirm for closing a ticket. Closing is not a one-click action. */
export default function ConfirmClose({ id, onConfirm, onCancel }) {
  return (
    <div className="confirm-close" role="alertdialog" aria-label={`Confirm closing ${id}`}>
      <span className="confirm-close-text">Close {id}? This marks the ticket closed.</span>
      <button type="button" className="btn btn-sm" onClick={onCancel} autoFocus>
        Cancel
      </button>
      <button type="button" className="btn btn-sm btn-success" onClick={onConfirm}>
        Close ticket
      </button>
    </div>
  )
}
