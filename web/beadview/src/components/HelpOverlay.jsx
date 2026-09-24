import React from 'react'

const SHORTCUTS = [
  ['?', 'Toggle this help overlay'],
  ['/', 'Focus search input'],
  ['n', 'Open new bead form'],
  ['1', 'Switch to Tree view'],
  ['2', 'Switch to Activity view'],
  ['3', 'Switch to Pipeline view'],
  ['4', 'Switch to Graph view'],
  ['j / \u2193', 'Move down in list'],
  ['k / \u2191', 'Move up in list'],
  ['Enter', 'Open detail panel for selected bead'],
  ['Esc', 'Close panel / modal / overlay'],
]

export default function HelpOverlay({ onClose, readOnly = false }) {
  // Read-only servers have no New Bead form, so "n" does nothing there.
  const shortcuts = readOnly ? SHORTCUTS.filter(([key]) => key !== 'n') : SHORTCUTS
  return (
    <div className="help-overlay" onClick={onClose}>
      <div className="help-content" onClick={e => e.stopPropagation()}>
        <h2>Keyboard Shortcuts</h2>
        <div className="shortcut-list">
          {shortcuts.map(([key, desc]) => (
            <React.Fragment key={key}>
              <span className="shortcut-key">{key}</span>
              <span className="shortcut-desc">{desc}</span>
            </React.Fragment>
          ))}
        </div>
        <div style={{ marginTop: 20, textAlign: 'right' }}>
          <button className="btn btn-sm" onClick={onClose}>Close</button>
        </div>
      </div>
    </div>
  )
}
