import { useEffect, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import './iugum-activity.css'

type Target={project:string;id:string}
type Execution={version:number;run_id:string;actor:string;label:string;platform:string;host:string;container?:string;session_id?:string;context_session_id?:string;agent_id?:string;parent_session_id?:string;parent_agent_id?:string;thread_url?:string;transcript?:string;identity_unavailable?:string;cwd:string;started_at:string;last_seen:string;ended_at?:string;outcome?:string}
type Activity={id:string;state:string;actor?:string;seen_at?:string;expires_at?:string;reason?:string;executions:Execution[]}
function useActivity(bead:Target){
 const q=useQuery<{project:string;issues:Activity[]}>({queryKey:['iugum-activity',bead.project],queryFn:async()=>{
  const r=await fetch(`/api/iugum/activity?project=${encodeURIComponent(bead.project)}`)
  if(!r.ok)throw new Error('Activity unavailable')
  const body=await r.json()
  if(body.project!==bead.project||!Array.isArray(body.issues))throw new Error('Activity project mismatch')
  return body
 },refetchInterval:15000,staleTime:5000,retry:false})
 return {activity:q.data?.issues.find(i=>i.id===bead.id),failed:q.isError}
}
function useClock(){const [now,setNow]=useState(Date.now());useEffect(()=>{const timer=setInterval(()=>setNow(Date.now()),1000);return()=>clearInterval(timer)},[]);return now}
export function ActivityBadge({bead}:{bead:Target}){
 const {activity:a,failed}=useActivity(bead);const now=useClock()
 if(!a)return <span className='iugum-activity text-muted-foreground'>Activity unavailable</span>
 let state=a.state
 const expires=Date.parse(a.expires_at||'')
 if(state==='working'&&(!Number.isFinite(expires)||expires<=now||failed))state='stale'
 const labels:Record<string,string>={working:'Working',paused:'Paused',completed:'Completed',stale:'Stale',unknown:'No live signal',unassigned:'Unassigned'}
 const title=[a.actor?`Owner: ${a.actor}`:'No assignee',a.seen_at?`Last seen: ${a.seen_at}`:'No heartbeat',a.reason||'',failed?'Activity refresh unavailable':''].filter(Boolean).join('\n')
 return <span className='iugum-activity text-muted-foreground' title={title} aria-label={`${labels[state]||'No live signal'}. ${title}`}>
  {state==='working'&&<span aria-hidden='true' data-working='true' className='iugum-activity-wheel'/>}
  <span>{labels[state]||'No live signal'}</span>
 </span>
}
function safeThreadURL(raw?:string){if(!raw)return null;try{const u=new URL(raw);if(u.protocol==='https:')return u.href;if(u.protocol==='codex:'&&u.hostname==='threads'&&/^\/[A-Za-z0-9-]+$/.test(u.pathname))return u.href}catch{return null}return null}
export function WorkerReferences({bead}:{bead:Target}){
 const {activity:a}=useActivity(bead);const [copied,setCopied]=useState(false)
 const refs=[...(a?.executions||[])].sort((x,y)=>y.started_at.localeCompare(x.started_at))
 return <section aria-label='Worker references' className='iugum-worker-references'>
  <h2 className='text-sm font-semibold'>Worker activity</h2>
  <ActivityBadge bead={bead}/>
  {refs.length===0&&<p className='text-sm text-muted-foreground'>No verified execution reference recorded.</p>}
  {refs.map(e=>{const url=safeThreadURL(e.thread_url);return <details key={e.run_id}>
   <summary>{e.label} <span className='text-muted-foreground'>({e.platform}, {e.host}{e.container?` / ${e.container}`:''})</span></summary>
   <dl>{[['Actor',e.actor],['Run',e.run_id],['Session',e.session_id],['Context session',e.context_session_id],['Agent (not a resumable session)',e.agent_id],['Parent session',e.parent_session_id],['Parent agent',e.parent_agent_id],['Transcript locator',e.transcript],['Identity unavailable',e.identity_unavailable],['Working directory',e.cwd],['Started',e.started_at],['Last seen',e.last_seen],['Ended',e.ended_at],['Outcome',e.outcome]].filter(([,v])=>v).map(([k,v])=><div key={k}><dt>{k}</dt><dd>{v}</dd></div>)}</dl>
   {url&&<a href={url} rel='noreferrer' target='_blank'>Open session</a>}
   <button type='button' onClick={async()=>{try{await navigator.clipboard.writeText(JSON.stringify(e,null,2));setCopied(true)}catch{setCopied(false)}}}>Copy reference</button>
   {copied&&<span role='status'>Reference copied</span>}
  </details>})}
 </section>
}
