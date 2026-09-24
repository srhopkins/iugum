
import React, { useEffect, useRef, useState } from 'react'
import mermaid from 'mermaid'
import { sanitizeMermaidFlowchart } from '../mermaidGraph'

function ensureMermaid(theme) {
  const mode = theme === 'light' ? 'neutral' : 'dark'
  mermaid.initialize({
    startOnLoad: false,
    securityLevel: 'loose',
    theme: mode,
    flowchart: {
      useMaxWidth: false,
      htmlLabels: true,
      curve: 'basis',
    },
  })
}

export default function MermaidDiagram({ data, beads, onSelect, theme = 'light' }) {
  const svgRef = useRef(null)
  const [zoom, setZoom] = useState(1)
  const [pan, setPan] = useState({ x: 0, y: 0 })
  const [isPanning, setIsPanning] = useState(false)
  const [panStart, setPanStart] = useState({ x: 0, y: 0 })
  const [error, setError] = useState(null)

  useEffect(() => {
    if (!data || !svgRef.current) return

    let cancelled = false

    async function render() {
      try {
        ensureMermaid(theme)
        const source = sanitizeMermaidFlowchart(data)
        const id = 'mermaid-graph-' + Date.now()
        const { svg } = await mermaid.render(id, source)
        if (cancelled || !svgRef.current) return
        svgRef.current.innerHTML = svg

        const nodes = svgRef.current.querySelectorAll('.node')
        nodes.forEach((node) => {
          node.style.cursor = 'pointer'
          node.addEventListener('click', () => {
            const text = node.textContent || ''
            const match = text.match(/([a-z]+-[a-z0-9-]+)/i)
            if (match && beads?.length) {
              const potentialId = match[1].toLowerCase()
              const bead = beads.find(
                (b) => b.id === potentialId || b.id.startsWith(potentialId)
              )
              if (bead) onSelect(bead.id)
            }
          })
        })
        setError(null)
      } catch (err) {
        console.error('Mermaid render error:', err)
        if (!cancelled) {
          setError(err?.message || 'Failed to render diagram')
          if (svgRef.current) svgRef.current.innerHTML = ''
        }
      }
    }

    render()
    return () => {
      cancelled = true
    }
  }, [data, beads, onSelect, theme])

  function handleWheel(e) {
    e.preventDefault()
    const delta = e.deltaY > 0 ? -0.1 : 0.1
    setZoom((z) => Math.max(0.2, Math.min(3, z + delta)))
  }

  function handleMouseDown(e) {
    if (e.button === 0) {
      setIsPanning(true)
      setPanStart({ x: e.clientX - pan.x, y: e.clientY - pan.y })
    }
  }

  function handleMouseMove(e) {
    if (isPanning) {
      setPan({ x: e.clientX - panStart.x, y: e.clientY - panStart.y })
    }
  }

  function handleMouseUp() {
    setIsPanning(false)
  }

  function resetView() {
    setZoom(1)
    setPan({ x: 0, y: 0 })
  }

  if (!data) {
    return (
      <div className="empty-state">
        <h3>No dependency graph available</h3>
        <p>Open an issue with dependencies, or add links with bd dep.</p>
      </div>
    )
  }

  return (
    <div className="diagram-container">
      <div className="diagram-controls">
        <button type="button" className="btn btn-sm" onClick={() => setZoom((z) => Math.min(3, z + 0.2))}>
          +
        </button>
        <button type="button" className="btn btn-sm" onClick={() => setZoom((z) => Math.max(0.2, z - 0.2))}>
          -
        </button>
        <button type="button" className="btn btn-sm" onClick={resetView}>
          Reset
        </button>
      </div>
      <div
        style={{
          overflow: 'hidden',
          cursor: isPanning ? 'grabbing' : 'grab',
          minHeight: 400,
        }}
        onWheel={handleWheel}
        onMouseDown={handleMouseDown}
        onMouseMove={handleMouseMove}
        onMouseUp={handleMouseUp}
        onMouseLeave={handleMouseUp}
      >
        <div
          ref={svgRef}
          style={{
            transform: `translate(${pan.x}px, ${pan.y}px) scale(${zoom})`,
            transformOrigin: '0 0',
            transition: isPanning ? 'none' : 'transform 0.1s',
          }}
        />
      </div>
      {error && (
        <div style={{ padding: 16, color: 'var(--accent-red)', fontSize: 13 }}>
          {error}
        </div>
      )}
    </div>
  )
}
