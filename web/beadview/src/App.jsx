import React, { Suspense, lazy, useState, useEffect, useCallback, useRef, useMemo } from 'react'
import Header from './components/Header'
import StatsRow from './components/StatsRow'
import FilterBar from './components/FilterBar'
import TreeView from './components/TreeView'
import ActivityView from './components/ActivityView'
import KanbanView from './components/KanbanView'
import DetailPanel from './components/DetailPanel'
import NewBeadModal from './components/NewBeadModal'
import HelpOverlay from './components/HelpOverlay'
import { apiFetch } from './api'
import { resolveMermaidSource } from './mermaidGraph'
import { filterAndSortBeads, uniqueLabelsFromBeads } from './filters'

// Mermaid is most of the bundle. Load it only when the Graph tab opens.
const MermaidDiagram = lazy(() => import('./components/MermaidDiagram'))

const VIEWS = ['tree', 'activity', 'pipeline', 'graph']
const PROJECT_STORAGE_KEY = 'beadview-project'
const THEME_STORAGE_KEY = 'beadview-theme'

function readStorage(key) {
  try {
    return localStorage.getItem(key)
  } catch {
    return null
  }
}

function writeStorage(key, value) {
  try {
    localStorage.setItem(key, value)
  } catch {
    /* ignore */
  }
}

/** Project from the URL, then localStorage. Empty until /api/projects answers. */
function getInitialProject() {
  if (typeof window === 'undefined') return ''
  const params = new URLSearchParams(window.location.search)
  return (params.get('project') || readStorage(PROJECT_STORAGE_KEY) || '').trim()
}

function getInitialBeadId() {
  if (typeof window === 'undefined') return null
  const params = new URLSearchParams(window.location.search)
  return params.get('bead') || params.get('issueId') || null
}

function getInitialFilters() {
  if (typeof window === 'undefined') {
    return {
      search: '',
      status: [],
      priority: [],
      type: [],
      assignee: [],
      labels: [],
      sort: 'updated',
    }
  }
  const params = new URLSearchParams(window.location.search)
  return {
    search: params.get('q') || params.get('search') || '',
    status: params.getAll('status'),
    priority: params.getAll('priority'),
    type: params.getAll('type'),
    assignee: params.getAll('assignee'),
    labels: params.getAll('label'),
    sort: params.get('sort') || 'updated',
  }
}

function getInitialView() {
  if (typeof window === 'undefined') return 'tree'
  const params = new URLSearchParams(window.location.search)
  const v = params.get('view')
  return VIEWS.includes(v) ? v : 'tree'
}

export default function App() {
  const [beads, setBeads] = useState([])
  const [mermaidData, setMermaidData] = useState('')
  const [loading, setLoading] = useState(true)
  const [connected, setConnected] = useState(false)
  const [loadError, setLoadError] = useState(null)
  const [wsConnected, setWsConnected] = useState(false)
  const [activeView, setActiveView] = useState(getInitialView)
  const [filters, setFilters] = useState(getInitialFilters)
  const [project, setProject] = useState(getInitialProject)
  const [projects, setProjects] = useState([])
  const [selectedBeadId, setSelectedBeadId] = useState(getInitialBeadId)
  const [detailOpen, setDetailOpen] = useState(() => !!getInitialBeadId())
  const [showNewBead, setShowNewBead] = useState(false)
  const [showHelp, setShowHelp] = useState(false)
  const [readOnly, setReadOnly] = useState(false)
  const [theme, setTheme] = useState(() => readStorage(THEME_STORAGE_KEY) || 'dark')
  const [focusedIndex, setFocusedIndex] = useState(-1)
  const searchInputRef = useRef(null)
  const wsRef = useRef(null)
  const pollTimerRef = useRef(null)
  const pendingBeadRef = useRef(getInitialBeadId())

  useEffect(() => {
    document.documentElement.setAttribute('data-theme', theme)
    writeStorage(THEME_STORAGE_KEY, theme)
  }, [theme])

  // Sync filters + project + bead to URL
  useEffect(() => {
    const params = new URLSearchParams()
    if (project) params.set('project', project)
    if (filters.search) params.set('q', filters.search)
    filters.status.forEach((s) => params.append('status', s))
    filters.priority.forEach((p) => params.append('priority', p))
    filters.type.forEach((t) => params.append('type', t))
    filters.assignee.forEach((a) => params.append('assignee', a))
    ;(filters.labels || []).forEach((l) => params.append('label', l))
    if (filters.sort !== 'updated') params.set('sort', filters.sort)
    if (activeView !== 'tree') params.set('view', activeView)
    if (selectedBeadId) params.set('bead', selectedBeadId)
    const str = params.toString()
    const newUrl = str ? `?${str}` : window.location.pathname
    window.history.replaceState(null, '', newUrl)
  }, [filters, activeView, project, selectedBeadId])

  useEffect(() => {
    if (project) writeStorage(PROJECT_STORAGE_KEY, project)
  }, [project])

  // GET /api/projects drives the project selector. iugum beadview serves
  // one project, so a stored or URL project it does not know falls back to
  // the first one the server lists.
  const fetchProjects = useCallback(async () => {
    try {
      const res = await apiFetch('/api/projects')
      if (!res.ok) return
      const data = await res.json()
      const list = Array.isArray(data.projects) ? data.projects : []
      setProjects(list)
      if (list.length > 0) {
        setProject((cur) => (cur && list.some((p) => p.slug === cur) ? cur : list[0].slug))
      }
    } catch {
      /* keep the current list */
    }
  }, [])

  const fetchData = useCallback(async () => {
    try {
      const [beadsRes, mermaidRes] = await Promise.all([
        apiFetch('/api/beads', { project }),
        apiFetch('/api/mermaid', { project }),
      ])
      if (beadsRes.ok) {
        const data = await beadsRes.json()
        if (Array.isArray(data)) {
          setBeads(data)
          setLoadError(null)
          setConnected(true)
          const want = pendingBeadRef.current
          if (want) {
            const found = data.some((b) => b.id === want)
            if (found) {
              setSelectedBeadId(want)
              setDetailOpen(true)
              pendingBeadRef.current = null
            }
          }
        } else if (data?.error) {
          setBeads([])
          setLoadError({ kind: 'api', message: String(data.error) })
          setConnected(false)
        }
      } else {
        let message = `Failed to load beads (${beadsRes.status})`
        try {
          const errBody = await beadsRes.json()
          if (errBody?.error) message = String(errBody.error)
        } catch {
          /* ignore */
        }
        setBeads([])
        setLoadError({ kind: 'api', message })
        setConnected(false)
      }
      if (mermaidRes.ok) {
        const text = await mermaidRes.text()
        setMermaidData(text)
      }
    } catch (err) {
      setConnected(false)
      setBeads([])
      setLoadError({
        kind: 'api',
        message: err instanceof Error ? err.message : String(err),
      })
    } finally {
      setLoading(false)
    }
  }, [project])

  const graphRoot = useMemo(() => {
    if (selectedBeadId) return selectedBeadId
    const q = (filters.search || '').trim()
    if (q && beads.some((b) => b.id === q)) return q
    return undefined
  }, [selectedBeadId, filters.search, beads])

  const graphMermaid = useMemo(
    () => resolveMermaidSource(beads, mermaidData, { root: graphRoot }),
    [beads, mermaidData, graphRoot]
  )

  useEffect(() => {
    fetchProjects()
  }, [fetchProjects])

  // WebSocket (optional)
  useEffect(() => {
    let ws = null
    let reconnectTimer = null

    async function connectWs() {
      try {
        const configRes = await apiFetch('/api/config')
        const config = await configRes.json()
        setReadOnly(!!config.read_only)
        if (!config.ws_port) {
          setWsConnected(false)
          return
        }
        const proto = location.protocol === 'https:' ? 'wss' : 'ws'
        const wsUrl = config.ws_path
          ? `${proto}://${location.host}${config.ws_path}`
          : `${proto}://${location.hostname}:${config.ws_port}`
        ws = new WebSocket(wsUrl)
        wsRef.current = ws
        ws.onopen = () => setWsConnected(true)
        ws.onmessage = (event) => {
          try {
            const msg = JSON.parse(event.data)
            if (msg.type === 'refresh') fetchData()
          } catch {
            /* ignore */
          }
        }
        ws.onclose = () => {
          setWsConnected(false)
          wsRef.current = null
          reconnectTimer = setTimeout(connectWs, 3000)
        }
        ws.onerror = () => ws.close()
      } catch {
        setWsConnected(false)
        reconnectTimer = setTimeout(connectWs, 5000)
      }
    }

    connectWs()
    return () => {
      if (ws) ws.close()
      if (reconnectTimer) clearTimeout(reconnectTimer)
    }
  }, [fetchData])

  useEffect(() => {
    setLoading(true)
    fetchData()
    pollTimerRef.current = setInterval(fetchData, wsConnected ? 30000 : 10000)
    return () => clearInterval(pollTimerRef.current)
  }, [fetchData, wsConnected])

  const filteredBeads = useMemo(
    () => filterAndSortBeads(beads, filters),
    [beads, filters]
  )

  useEffect(() => {
    function handleKeyDown(e) {
      const tag = (e.target?.tagName || '').toLowerCase()
      if (tag === 'input' || tag === 'textarea' || tag === 'select') {
        if (e.key === 'Escape') e.target.blur()
        return
      }
      switch (e.key) {
        case '?':
          setShowHelp((h) => !h)
          break
        case 'Escape':
          if (showHelp) setShowHelp(false)
          else if (showNewBead) setShowNewBead(false)
          else if (detailOpen) {
            setDetailOpen(false)
            setSelectedBeadId(null)
          }
          break
        case '/':
          e.preventDefault()
          searchInputRef.current?.focus()
          break
        case 'n':
          if (!readOnly) setShowNewBead(true)
          break
        case '1':
          setActiveView('tree')
          break
        case '2':
          setActiveView('activity')
          break
        case '3':
          setActiveView('pipeline')
          break
        case '4':
          setActiveView('graph')
          break
        case 'j':
        case 'ArrowDown':
          e.preventDefault()
          setFocusedIndex((i) => Math.min(i + 1, filteredBeads.length - 1))
          break
        case 'k':
        case 'ArrowUp':
          e.preventDefault()
          setFocusedIndex((i) => Math.max(i - 1, 0))
          break
        case 'Enter':
          if (focusedIndex >= 0 && focusedIndex < filteredBeads.length) {
            const bead = filteredBeads[focusedIndex]
            setSelectedBeadId(bead.id)
            setDetailOpen(true)
          }
          break
      }
    }
    window.addEventListener('keydown', handleKeyDown)
    return () => window.removeEventListener('keydown', handleKeyDown)
  })

  const stats = useMemo(() => {
    const total = beads.length
    const open = beads.filter((b) => b.status === 'open').length
    const inProgress = beads.filter((b) => b.status === 'in_progress').length
    const blocked = beads.filter((b) => b.status === 'blocked').length
    const closed = beads.filter((b) => b.status === 'closed').length
    const pct = total > 0 ? Math.round((closed / total) * 100) : 0
    return { total, open, inProgress, blocked, closed, pct }
  }, [beads])

  const allAssignees = useMemo(() => {
    const set = new Set()
    beads.forEach((b) => {
      const a = b.assignee || b.actor || ''
      if (a) set.add(a)
    })
    return Array.from(set).sort()
  }, [beads])

  const allTypes = useMemo(() => {
    const set = new Set()
    beads.forEach((b) => {
      if (b.type) set.add(b.type)
    })
    return Array.from(set).sort()
  }, [beads])

  const allLabels = useMemo(() => uniqueLabelsFromBeads(beads), [beads])

  function openDetail(beadId) {
    setSelectedBeadId(beadId)
    setDetailOpen(true)
  }

  function closeDetail() {
    setDetailOpen(false)
    setSelectedBeadId(null)
  }

  function handleProjectChange(next) {
    const slug = (next || '').trim()
    if (!slug || slug === project) return
    setProject(slug)
    setSelectedBeadId(null)
    setDetailOpen(false)
    pendingBeadRef.current = null
    setLoadError(null)
    setLoading(true)
    setBeads([])
  }

  async function handleStatusChange(beadId, newStatus) {
    try {
      const resp = await apiFetch(`/api/bead/${beadId}`, {
        method: 'PATCH',
        body: JSON.stringify({ status: newStatus }),
        project,
      })
      if (resp.ok) fetchData()
    } catch {
      /* silent */
    }
  }

  async function handleCreateBead(data) {
    try {
      const resp = await apiFetch('/api/bead', {
        method: 'POST',
        body: JSON.stringify({ ...data, project }),
        project,
      })
      if (resp.ok) {
        setShowNewBead(false)
        fetchData()
      }
    } catch {
      /* silent */
    }
  }

  const selectedBead = beads.find((b) => b.id === selectedBeadId) || null

  let emptyMessage = null
  if (!loading && loadError?.kind === 'api') {
    emptyMessage = loadError.message
  } else if (!loading && beads.length === 0 && !loadError) {
    emptyMessage = `No issues in this project${project ? ` (${project})` : ''}. Create one with \`iugum beads create\`.`
  }

  return (
    <div className="app-layout">
      <Header
        connected={connected}
        wsConnected={wsConnected}
        theme={theme}
        onToggleTheme={() => setTheme((t) => (t === 'dark' ? 'light' : 'dark'))}
        onNewBead={() => setShowNewBead(true)}
        onRefresh={fetchData}
        onHelp={() => setShowHelp(true)}
        project={project}
        projects={projects}
        onProjectChange={handleProjectChange}
        readOnly={readOnly}
      />

      <div className="app-main">
        <div className={`main-content${detailOpen ? ' with-panel' : ''}`}>
          <StatsRow
            stats={stats}
            activeStatuses={filters.status || []}
            onStatusFilter={(status) => {
              setFilters((prev) => {
                const current = prev.status || []
                if (status === null) {
                  return { ...prev, status: [] }
                }
                // Toggle off if already the sole status filter
                if (current.length === 1 && current[0] === status) {
                  return { ...prev, status: [] }
                }
                return { ...prev, status: [status] }
              })
            }}
          />

          <div className="view-tabs">
            {[
              { key: 'tree', label: 'Tree', num: '1' },
              { key: 'activity', label: 'Activity', num: '2' },
              { key: 'pipeline', label: 'Pipeline', num: '3' },
              { key: 'graph', label: 'Graph', num: '4' },
            ].map((v) => (
              <button
                key={v.key}
                className={`view-tab${activeView === v.key ? ' active' : ''}`}
                onClick={() => setActiveView(v.key)}
              >
                {v.label}
                <span className="shortcut-hint">{v.num}</span>
              </button>
            ))}
          </div>

          <FilterBar
            filters={filters}
            onFilterChange={setFilters}
            assignees={allAssignees}
            types={allTypes}
            labels={allLabels}
            searchInputRef={searchInputRef}
            showingCount={filteredBeads.length}
            totalCount={beads.length}
          />

          {loading ? (
            <div className="loading">Loading beads...</div>
          ) : emptyMessage ? (
            <div className="empty-state" style={{ padding: 24, color: 'var(--text-secondary)' }}>
              <h3 style={{ marginTop: 0 }}>No beads to show</h3>
              <p style={{ whiteSpace: 'pre-wrap' }}>{emptyMessage}</p>
            </div>
          ) : (
            <>
              {activeView === 'tree' && (
                <TreeView
                  beads={filteredBeads}
                  allBeads={beads}
                  focusedIndex={focusedIndex}
                  onSelect={openDetail}
                />
              )}
              {activeView === 'activity' && (
                <ActivityView
                  beads={filteredBeads}
                  allBeads={beads}
                  onSelect={openDetail}
                />
              )}
              {activeView === 'pipeline' && (
                <KanbanView
                  beads={filteredBeads}
                  onSelect={openDetail}
                  onStatusChange={readOnly ? undefined : handleStatusChange}
                />
              )}
              {activeView === 'graph' && (
                <Suspense fallback={<div className="loading">Loading graph...</div>}>
                  <MermaidDiagram
                    data={graphMermaid}
                    beads={beads}
                    onSelect={openDetail}
                    theme={theme}
                  />
                </Suspense>
              )}
            </>
          )}
        </div>

        <DetailPanel
          bead={selectedBead}
          open={detailOpen}
          onClose={closeDetail}
          onRefresh={fetchData}
          allBeads={beads}
          onSelectBead={openDetail}
          project={project}
          readOnly={readOnly}
        />
      </div>

      {showNewBead && !readOnly && (
        <NewBeadModal
          onClose={() => setShowNewBead(false)}
          onCreate={handleCreateBead}
          allBeads={beads}
          project={project}
        />
      )}

      {showHelp && <HelpOverlay onClose={() => setShowHelp(false)} />}
    </div>
  )
}
