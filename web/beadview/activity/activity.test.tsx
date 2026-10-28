import { afterEach, expect, it, vi } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import { renderWithProviders } from '@/test/render'
import { ActivityBadge, WorkerReferences } from '@/components/iugum-activity'

afterEach(()=>vi.unstubAllGlobals())
const bead={project:'alpha',id:'same'}
function response(state:string,expires_at=new Date(Date.now()+60000).toISOString()) {
 return {project:'alpha',issues:[{id:'same',state,actor:'worker',seen_at:new Date().toISOString(),expires_at,reason:state==='paused'?'Waiting for review':'',executions:[]}]}
}
it('polls scoped activity and removes motion when paused',async()=>{
 let data=response('working')
 const fetch=vi.fn(async(_url:string)=>({ok:true,json:async()=>data}))
 vi.stubGlobal('fetch',fetch)
 const {client}=renderWithProviders(<ActivityBadge bead={bead}/>)
 await screen.findByText('Working')
 expect(document.querySelector('[data-working="true"]')).not.toBeNull()
 expect(fetch.mock.calls[0][0]).toContain('project=alpha')
 data=response('paused')
 await client.invalidateQueries({queryKey:['iugum-activity','alpha']})
 await screen.findByText('Paused')
 expect(document.querySelector('[data-working="true"]')).toBeNull()
 expect(screen.getByText('Paused').closest('[title]')).toHaveAttribute('title',expect.stringContaining('Waiting for review'))
})
it('never spins for expired, completed, unknown or unassigned activity',async()=>{
 vi.stubGlobal('fetch',vi.fn(async()=>({ok:true,json:async()=>response('working',new Date(Date.now()-1000).toISOString())})))
 renderWithProviders(<ActivityBadge bead={bead}/>)
 await screen.findByText('Stale')
 expect(document.querySelector('[data-working="true"]')).toBeNull()
})
it('does not expose unsafe execution links',async()=>{
 const data=response('paused')
 data.issues[0].executions=[{run_id:'r',label:'Backend worker',actor:'worker',platform:'codex',host:'mac',session_id:'real-session',agent_id:'real-agent',thread_url:'javascript:alert(1)',transcript:'/private/path',cwd:'/workspace',started_at:'2026-10-20T00:00:00Z',last_seen:'2026-10-20T00:01:00Z'}] as never[]
 vi.stubGlobal('fetch',vi.fn(async()=>({ok:true,json:async()=>data})))
 renderWithProviders(<WorkerReferences bead={bead}/>)
 await screen.findByText('Backend worker')
 expect(screen.queryByRole('link',{name:'Open session'})).toBeNull()
 expect(screen.getByText('real-session')).toBeInTheDocument()
 expect(screen.getByText('real-agent')).toBeInTheDocument()
 expect(screen.getByText('/private/path')).toBeInTheDocument()
 await waitFor(()=>expect(screen.getByText('Copy reference')).toBeInTheDocument())
})

it.each(['paused','completed','unknown','unassigned'])('renders %s without motion',async state=>{
 vi.stubGlobal('fetch',vi.fn(async()=>({ok:true,json:async()=>response(state)})))
 renderWithProviders(<ActivityBadge bead={bead}/>)
 const label={paused:'Paused',completed:'Completed',unknown:'No live signal',unassigned:'Unassigned'}[state]
 await screen.findByText(label!)
 expect(document.querySelector('[data-working="true"]')).toBeNull()
})
