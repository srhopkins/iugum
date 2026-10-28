// Assemble Iugum's small UI extension in a disposable copy of the pinned fork.
// Exact source anchors fail closed on upstream drift; module-cache files stay read-only.
import {cpSync,readFileSync,writeFileSync,mkdirSync,existsSync,symlinkSync,chmodSync,readdirSync} from 'node:fs'
import {execFileSync} from 'node:child_process'
import path from 'node:path'
const root=path.resolve(import.meta.dirname,'../..')
const target=process.argv[2]
if(!target) throw new Error('temporary build directory required')
const mod=execFileSync('go',['list','-m','-f','{{.Dir}}','github.com/steveyegge/beads'],{cwd:root,encoding:'utf8'}).trim()
cpSync(path.join(mod,'beadview/web'),target,{recursive:true,filter:p=>!p.split(path.sep).some(s=>s==='node_modules'||s==='dist')})
function writable(p){chmodSync(p,0o755);for(const e of readdirSync(p,{withFileTypes:true})){const q=path.join(p,e.name);if(e.isDirectory())writable(q);else chmodSync(q,0o644)}}
writable(target)
mkdirSync(path.join(target,'src/components'),{recursive:true})
cpSync(path.join(import.meta.dirname,'activity/Activity.tsx'),path.join(target,'src/components/iugum-activity.tsx'))
cpSync(path.join(import.meta.dirname,'activity/activity.css'),path.join(target,'src/components/iugum-activity.css'))
cpSync(path.join(import.meta.dirname,'activity/activity.test.tsx'),path.join(target,'src/__tests__/iugum-activity.test.tsx'))
cpSync(path.join(import.meta.dirname,'retention/ClosedRetention.tsx'),path.join(target,'src/components/iugum-closed-retention.tsx'))
cpSync(path.join(import.meta.dirname,'retention/retention.test.tsx'),path.join(target,'src/__tests__/iugum-retention.test.tsx'))
cpSync(path.join(import.meta.dirname,'swimlanes/ProjectSwimlane.tsx'),path.join(target,'src/components/iugum-project-swimlane.tsx'))
cpSync(path.join(import.meta.dirname,'swimlanes/swimlanes.test.tsx'),path.join(target,'src/__tests__/iugum-swimlanes.test.tsx'))
function patch(file,from,to){const p=path.join(target,'src',file);const s=readFileSync(p,'utf8');if(s.split(from).length!==2)throw new Error(`upstream UI seam drift: ${file}`);writeFileSync(p,s.replace(from,to))}
patch('views/BoardView.tsx',"import { useMemo, useState } from 'react'","import { useMemo, useState } from 'react'\nimport { ActivityBadge } from '@/components/iugum-activity'")
patch('views/BoardView.tsx',"<span className='text-xs text-muted-foreground'>{bead.assignee || 'unassigned'}</span>","<span className='text-xs text-muted-foreground'>{bead.assignee || 'unassigned'}</span>\n      <ActivityBadge bead={bead} />")
patch('views/BoardView.tsx',"import { ActivityBadge } from '@/components/iugum-activity'","import { ActivityBadge } from '@/components/iugum-activity'\nimport { useBoardClosedRetention, ClosedRetentionControls } from '@/components/iugum-closed-retention'")
patch('views/BoardView.tsx','  const [personal, setPersonal] = useState(readLanePreferences)','  const retained = useBoardClosedRetention(beads)\n  const [personal, setPersonal] = useState(readLanePreferences)')
patch('views/BoardView.tsx','return beads.filter(b => !hidden.has(b.status))','return retained.cards.filter(b => !hidden.has(b.status))')
patch('views/BoardView.tsx','}, [beads, lanes])','}, [retained.cards, lanes])')
patch('views/BoardView.tsx',">{backlogToggle}<details>",">{backlogToggle}<ClosedRetentionControls state={retained} shown={onBoard.length} total={beads.length} /><details>")
patch('App.tsx','          <StatsRow',"          {url.view === 'board' && <p className='text-xs text-muted-foreground'>Totals include all dates; the board uses the Closed filter below.</p>}\n          <StatsRow")
patch('views/BoardView.tsx',"import { useBoardClosedRetention, ClosedRetentionControls } from '@/components/iugum-closed-retention'","import { useBoardClosedRetention, ClosedRetentionControls } from '@/components/iugum-closed-retention'\nimport { ProjectSwimlane } from '@/components/iugum-project-swimlane'")
for(const file of ['TicketPage.tsx','components/bead-drawer.tsx']) {
 patch(file,"import { BeadFacts, Fact } from '@/components/bead-facts'","import { BeadFacts, Fact } from '@/components/bead-facts'\nimport { WorkerReferences } from '@/components/iugum-activity'")
 const anchor=file==='TicketPage.tsx'?"<h2 className='mb-1 text-sm font-semibold'>Description</h2>":'<BeadFacts bead={bead} />'
 patch(file,anchor,anchor+'\n                <WorkerReferences bead={bead} />')
}
// Inherited isolated BoardView tests predate the activity query. Supply the
// same query context as the application without changing their assertions.
patch('__tests__/board.test.tsx',"import { render, screen, waitFor, within } from '@testing-library/react'", "import { render as renderBare, screen, waitFor, within } from '@testing-library/react'\nimport { QueryClient, QueryClientProvider } from '@tanstack/react-query'\nconst render = (ui: Parameters<typeof renderBare>[0]) => renderBare(<QueryClientProvider client={new QueryClient({defaultOptions:{queries:{retry:false}}})}>{ui}</QueryClientProvider>)")
patch('views/BoardView.tsx',"          <div key={project} data-lane={project} className='flex flex-col gap-2'>\n            <div>\n              <ProjectTag project={project} />\n            </div>\n            <Lane columns={columns} parents={parents} beads={own} showProject={false} onSelect={onSelect} onMove={safeMove} counts={laneCounts} laneLabel={project} />\n          </div>","          <ProjectSwimlane key={project} project={project} count={own.length}>\n            <Lane columns={columns} parents={parents} beads={own} showProject={false} onSelect={onSelect} onMove={safeMove} counts={laneCounts} laneLabel={project} />\n          </ProjectSwimlane>")
const deps=path.join(import.meta.dirname,'node_modules')
if(existsSync(deps))symlinkSync(deps,path.join(target,'node_modules'),'dir')

// PR navigation and API-backed table share the fork's existing providers.
cpSync(path.join(import.meta.dirname,'prs/PullRequests.tsx'),path.join(target,'src/components/iugum-prs.tsx'))
cpSync(path.join(import.meta.dirname,'prs/prs.css'),path.join(target,'src/components/iugum-prs.css'))
patch('main.tsx',"import App from './App'","import App from './App'\nimport { ViewerShell } from './components/iugum-prs'")
patch('main.tsx',': <App />',': <ViewerShell><App /></ViewerShell>')

cpSync(path.join(import.meta.dirname,'prs/Details.tsx'),path.join(target,'src/components/iugum-pr-details.tsx'))

// Iugum's locked dependency overlay adds safe PR comment HTML rendering.
for(const name of ['package.json','package-lock.json'])cpSync(path.join(import.meta.dirname,name),path.join(target,name))
