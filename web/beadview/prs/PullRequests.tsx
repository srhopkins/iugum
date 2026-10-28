import { useEffect, useMemo, useRef, useState, useSyncExternalStore, type ReactNode } from 'react'
import { useQuery } from '@tanstack/react-query'
import { flexRender, getCoreRowModel, useReactTable, type ColumnDef } from '@tanstack/react-table'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle, SheetTrigger } from '@/components/ui/sheet'
import { useTheme, type Theme } from '@/context/theme-provider'
import './iugum-prs.css'

type Kind = 'org' | 'repo' | 'author'
type Pin = {kind:Kind;value:string}
type PR = {number:number;repository:string;title:string;url:string;author:string;state:string;draft:boolean;updated_at:string;review:string;checks:string}
type Result = {items:PR[];total_count:number;page:number;per_page:number;has_next:boolean;incomplete_results:boolean;truncated:boolean;fetched_at:string}
const changed='beadview:urlchange'
const subscribe=(fn:()=>void)=>{window.addEventListener('popstate',fn);window.addEventListener(changed,fn);return()=>{window.removeEventListener('popstate',fn);window.removeEventListener(changed,fn)}}
const snapshot=()=>window.location.search
const read=(key:string,fallback:string)=>{try{return localStorage.getItem(key)||fallback}catch{return fallback}}
const save=(key:string,value:string)=>{try{localStorage.setItem(key,value)}catch{/* URL remains authoritative. */}}
function navigate(search:string){window.history.pushState(null,'',window.location.pathname+search);window.dispatchEvent(new Event(changed))}
const pinKey='iugum-pr-pins-v1'
const labels:Record<Kind,string>={org:'Organization',repo:'Repository',author:'Author'}
export function readPins():Pin[]{try{const p:unknown=JSON.parse(read(pinKey,'[]'));return Array.isArray(p)?p.filter((v):v is Pin=>!!v&&typeof v==='object'&&['org','repo','author'].includes(v.kind)&&typeof v.value==='string'&&v.value.length<150).slice(0,24):[]}catch{return []}}
function safeLink(value:string){try{const u=new URL(value);return u.protocol==='https:'&&u.hostname==='github.com'&&!u.username&&!u.password?u.href:undefined}catch{return undefined}}
export function ViewerShell({children}:{children:ReactNode}){
 const search=useSyncExternalStore(subscribe,snapshot,()=> '')
 const isPR=new URLSearchParams(search).get('section')==='prs'
 const lastBeads=useRef(read('iugum-last-beads','?view=board'))
 const lastPR=useRef(read('iugum-last-prs','?section=prs'))
 useEffect(()=>{if(isPR){lastPR.current=search;save('iugum-last-prs',search)}else{lastBeads.current=search;save('iugum-last-beads',search)}},[search,isPR])
 return <><nav aria-label='Sections' className='pr-sections'>
  <a href={lastBeads.current||'?view=board'} aria-current={!isPR?'page':undefined} onClick={e=>{if(e.button||e.metaKey||e.ctrlKey||e.shiftKey||e.altKey)return;e.preventDefault();if(isPR)navigate(lastBeads.current||'?view=board')}}>Beads</a>
  <a href={lastPR.current} aria-current={isPR?'page':undefined} onClick={e=>{if(e.button||e.metaKey||e.ctrlKey||e.shiftKey||e.altKey)return;e.preventDefault();if(!isPR)navigate(lastPR.current)}}>Pull requests</a>
 </nav>{isPR?<PullRequests search={search}/>:children}</>
}
function Facet({kind,selected,suggestions,pins,toggle,pin}:{kind:Kind;selected:string[];suggestions:string[];pins:Pin[];toggle:(v:string)=>void;pin:(v:string)=>void}){
 const [text,setText]=useState('')
 const values=[...new Set([...selected,...pins.filter(p=>p.kind===kind).map(p=>p.value),...suggestions])].filter(v=>v.toLowerCase().includes(text.toLowerCase())).sort()
 const valid=(kind==='repo'?/^[A-Za-z0-9][A-Za-z0-9-]{0,38}\/[A-Za-z0-9_.-]{1,100}$/:kind==='author'?/^[A-Za-z0-9][A-Za-z0-9-]{0,38}(\[bot\])?$/:/^[A-Za-z0-9][A-Za-z0-9-]{0,38}$/).test(text.trim())
 return <Popover><PopoverTrigger asChild><Button variant='outline'>{labels[kind]}{selected.length?` (${selected.length})`:''} ▾</Button></PopoverTrigger>
 <PopoverContent className='pr-facet' align='start'>
  <label>{labels[kind]} search<Input value={text} onChange={e=>setText(e.target.value)} placeholder={kind==='repo'?'owner/repository':'Search or enter exact name'}/></label>
  <p className='pr-help'>Choices come from this page and your pins. Enter an exact name to search beyond them.</p>
  <div className='pr-facet-options'>{values.map(v=><div key={v} className='pr-option'><label><input type='checkbox' checked={selected.includes(v)} onChange={()=>toggle(v)}/><span>{v}</span></label><button className='pr-pin' aria-label={`${pins.some(p=>p.kind===kind&&p.value===v)?'Unpin':'Pin'} ${labels[kind].toLowerCase()} ${v}`} aria-pressed={pins.some(p=>p.kind===kind&&p.value===v)} onClick={()=>pin(v)}>☆</button></div>)}</div>
  {valid&&!values.includes(text.trim())&&<Button variant='outline' onClick={()=>{toggle(text.trim());setText('')}}>Add {text.trim()}</Button>}
  {!values.length&&!valid&&<p className='pr-help'>No matching suggestions. Enter a valid {kind==='repo'?'owner/repository':'GitHub name'}.</p>}
 </PopoverContent></Popover>
}
function PullRequests({search}:{search:string}){
 const params=useMemo(()=>new URLSearchParams(search),[search])
 const {theme,setTheme}=useTheme()
 const [pins,setPins]=useState(readPins)
 const [draftText,setDraftText]=useState(params.get('q')||'')
 const canonicalText=params.get('q')||''
 useEffect(()=>setDraftText(canonicalText),[canonicalText])
 const update=(key:string,values:string[])=>{const p=new URLSearchParams(window.location.search);p.set('section','prs');p.delete(key);values.filter(Boolean).forEach(v=>p.append(key,v));if(key!=='page')p.delete('page');navigate('?'+p.toString())}
 useEffect(()=>{if(draftText===canonicalText)return;const timer=setTimeout(()=>update('q',[draftText]),400);return()=>clearTimeout(timer)},[draftText,canonicalText]) // eslint-disable-line react-hooks/exhaustive-deps
 const apiParams=new URLSearchParams(params);apiParams.delete('section')
 const queryString=apiParams.toString()
 const query=useQuery<Result>({queryKey:['iugum-prs',queryString],queryFn:async({signal})=>{const r=await fetch('/api/v1/prs?'+queryString,{signal});const data=await r.json();if(!r.ok)throw new Error(data.error?.message||'Unable to load pull requests');return data},retry:false,staleTime:30000,refetchOnWindowFocus:false})
 const rows=query.data?.items||[]
 const suggestions:Record<Kind,string[]>={org:rows.map(p=>p.repository.split('/')[0]!),repo:rows.map(p=>p.repository),author:rows.map(p=>p.author)}
 const selected=(kind:Kind)=>params.getAll(kind)
 const toggle=(kind:Kind,v:string)=>{const values=selected(kind);update(kind,values.includes(v)?values.filter(x=>x!==v):[...values,v])}
 const pin=(kind:Kind,value:string)=>{const exists=pins.some(p=>p.kind===kind&&p.value===value);const next=exists?pins.filter(p=>p.kind!==kind||p.value!==value):[...pins,{kind,value}].slice(-24);setPins(next);save(pinKey,JSON.stringify(next))}
 const clear=()=>{setDraftText('');navigate('?section=prs')}
 const columns=useMemo<ColumnDef<PR>[]>(()=>[
  {accessorKey:'repository',header:'Repository',cell:({row})=><a href={'https://github.com/'+row.original.repository} target='_blank' rel='noreferrer'>{row.original.repository}</a>},
  {accessorKey:'title',header:'Pull request',cell:({row})=><a href={safeLink(row.original.url)} target='_blank' rel='noreferrer'><span className='pr-number'>#{row.original.number}</span> {row.original.title}</a>},
  {accessorKey:'author',header:'Author'},
  {id:'state',header:'State',cell:({row})=><span>{row.original.draft?'Draft · ':''}{row.original.state}</span>},
  {accessorKey:'updated_at',header:'Updated',cell:({getValue})=>{const value=getValue<string>();return <time dateTime={value}>{new Date(value).toLocaleString()}</time>}},
 ],[])
 const table=useReactTable({data:rows,columns,getCoreRowModel:getCoreRowModel(),manualPagination:true,manualSorting:true})
 const state=params.get('state')||'open',draft=params.get('draft')||'any',sort=params.get('sort')||'updated',order=params.get('order')||'desc'
 const controls=<div className='pr-filter-fields'>
  <label>PR state<select aria-label="PR state" value={state} onChange={e=>update('state',[e.target.value])}><option value='open'>Open</option><option value='closed'>Closed</option><option value='all'>All states</option></select></label>
  <label>Draft<select aria-label="Draft" value={draft} onChange={e=>update('draft',[e.target.value])}><option value='any'>Any</option><option value='yes'>Draft only</option><option value='no'>Ready for review</option></select></label>
  <label>Sort by<select aria-label="Sort by" value={sort} onChange={e=>update('sort',[e.target.value])}><option value='updated'>Updated</option><option value='created'>Created</option><option value='comments'>Comment count</option></select></label>
  <label>Order<select aria-label="Order" value={order} onChange={e=>update('order',[e.target.value])}><option value='desc'>Newest / most first</option><option value='asc'>Oldest / fewest first</option></select></label>
 </div>
 const chips:[string,string,string][]=[]
 for(const kind of ['org','repo','author'] as const)for(const value of selected(kind))chips.push([kind,value,`${labels[kind]}: ${value}`])
 if(state!=='all')chips.push(['state',state,state==='open'?'Open':'Closed'])
 if(draft!=='any')chips.push(['draft',draft,draft==='yes'?'Draft':'Ready for review'])
 if(canonicalText)chips.push(['q',canonicalText,`Text: ${canonicalText}`])
 return <main className='pr-page'>
  <header className='pr-heading'><div><h1>Pull requests</h1><p className='pr-help'>GitHub search · read-only · scoped by your filters and account access</p></div><label className='pr-theme'>Theme<select aria-label='Theme' value={theme} onChange={e=>setTheme(e.target.value as Theme)}><option value='system'>System</option><option value='light'>Light</option><option value='dark'>Dark</option></select></label></header>
  <div className='pr-toolbar'><label className='pr-search'><span className='sr-only'>Search pull requests</span><Input aria-label='Search pull requests' value={draftText} onChange={e=>setDraftText(e.target.value)} placeholder='Search pull requests…'/></label>
  <Sheet><SheetTrigger asChild><Button variant='outline'>All filters</Button></SheetTrigger><SheetContent><SheetHeader><SheetTitle>All filters</SheetTitle><SheetDescription>Changes apply immediately. Match any selection within a category, and every category together.</SheetDescription></SheetHeader><div className='pr-drawer'>{(['org','repo','author'] as const).map(kind=><Facet key={kind} kind={kind} selected={selected(kind)} suggestions={suggestions[kind]} pins={pins} toggle={v=>toggle(kind,v)} pin={v=>pin(kind,v)}/>)}{controls}<p className='pr-help'>GitHub supports up to five OR operators in this search. Review and CI filtering are not loaded in this first pass.</p><Button variant='outline' onClick={clear}>Reset to open PRs</Button></div></SheetContent></Sheet>
  <Button variant='outline' disabled={query.isFetching} onClick={()=>void query.refetch()}>{query.isFetching?'Refreshing…':'Refresh'}</Button></div>
  <div className='pr-quick' aria-label='Quick filters'><span className='pr-help'>Quick filters</span>{pins.map(p=><button key={p.kind+':'+p.value} className='pr-chip' aria-label={`Toggle ${labels[p.kind].toLowerCase()} ${p.value}`} aria-pressed={selected(p.kind).includes(p.value)} onClick={()=>toggle(p.kind,p.value)}>{selected(p.kind).includes(p.value)?'✓ ':''}{p.value}</button>)}{!pins.length&&<span className='pr-help'>Pin an organization, repository, or author from a dropdown.</span>}</div>
  <div className='pr-toolbar'>{(['org','repo','author'] as const).map(kind=><Facet key={kind} kind={kind} selected={selected(kind)} suggestions={suggestions[kind]} pins={pins} toggle={v=>toggle(kind,v)} pin={v=>pin(kind,v)}/>)}</div>
  <div className='pr-active' aria-label='Active filters'>{chips.map(([key,value,label])=><button key={key+value} className='pr-chip' aria-label={`Remove ${label} filter`} onClick={()=>{if(key==='q')setDraftText('');update(key,key==='state'?['all']:params.getAll(key).filter(v=>v!==value))}}>{label} ×</button>)}<button className='pr-clear' onClick={clear}>Clear all · reset</button></div>
  <div role='status' className='pr-help'>{query.isFetching?'Loading results…':query.data?`${query.data.total_count.toLocaleString()} matches · refreshed ${new Date(query.data.fetched_at).toLocaleTimeString()}`:''}</div>
  {query.isError&&<div role='alert' className='pr-notice'>{query.error.message}{query.data?' Showing the last successful response.':''}</div>}
  {(query.data?.incomplete_results||query.data?.truncated)&&<div role='alert' className='pr-notice'>Results are incomplete{query.data.truncated?' — GitHub exposes only the first 1,000 matches':''}. Narrow your filters.</div>}
  {!query.isPending&&!query.isError&&!rows.length&&<p className='pr-empty'>No pull requests match these filters.</p>}
  {!!rows.length&&<div className='pr-table-scroll'><table className='pr-table'><thead>{table.getHeaderGroups().map(group=><tr key={group.id}>{group.headers.map(header=><th key={header.id} scope='col'>{header.id==='updated_at'?<button onClick={()=>{const p=new URLSearchParams(window.location.search);p.set('sort','updated');p.set('order',order==='desc'?'asc':'desc');p.delete('page');navigate('?'+p.toString())}}>Updated {sort==='updated'?(order==='desc'?'↓':'↑'):''}</button>:flexRender(header.column.columnDef.header,header.getContext())}</th>)}</tr>)}</thead><tbody>{table.getRowModel().rows.map(row=><tr key={row.original.repository+':'+row.original.number}>{row.getVisibleCells().map(cell=><td key={cell.id}>{flexRender(cell.column.columnDef.cell,cell.getContext())}</td>)}</tr>)}</tbody></table></div>}
  {query.data&&<footer className='pr-pagination'><Button variant='outline' disabled={query.data.page<=1||query.isFetching} onClick={()=>update('page',[String(query.data!.page-1)])}>Previous</Button><span>Page {query.data.page} · {rows.length} shown</span><Button variant='outline' disabled={!query.data.has_next||query.isFetching} onClick={()=>update('page',[String(query.data!.page+1)])}>Next</Button></footer>}
  <p className='pr-help'>Review decisions and CI checks are not loaded. Open a pull request on GitHub for those details. Search may omit inaccessible or unindexed repositories.</p>
 </main>
}
