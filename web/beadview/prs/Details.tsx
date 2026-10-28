import {useState} from 'react'
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import rehypeRaw from 'rehype-raw'
import rehypeSanitize from 'rehype-sanitize'
import {CircleCheck, CircleMinus, FileDiff, CircleX, Clock3, MessageSquare, Users, Layers, CircleHelp} from 'lucide-react'
import {useInfiniteQuery, useQuery} from '@tanstack/react-query'
import {Dialog,DialogContent,DialogDescription,DialogHeader,DialogTitle,DialogTrigger} from '@/components/ui/dialog'
import {Button} from '@/components/ui/button'

export type PRIdentity={repository:string;number:number;title:string}
export type Summary={repository:string;number:number;review:string;checks:{total:number;passed:number;failed:number;pending:number;skipped:number}|null;comments:number;assignees:string[];stack?:{position:number;total:number}|null;stack_known?:boolean;error?:unknown}
type Item={id:string|number;author?:string;bot?:boolean;body?:string;state?:string;name?:string;url?:string;created_at?:string;path?:string}
type DetailPage={items:Item[];has_next:boolean;next_cursor?:string;head?:string}
type Kind='reviews'|'checks'|'comments'|'code-comments'|'assignees'|'stack'
async function get<T>(url:string,signal:AbortSignal):Promise<T>{const r=await fetch(url,{signal});const data=await r.json();if(!r.ok)throw new Error(data.error?.message||'Unable to load details');return data}
const key=(p:Pick<PRIdentity,'repository'|'number'>)=>`${p.repository}/${p.number}`
export function useSummaries(rows:PRIdentity[],revision:number){
 const ids=rows.map(key)
 return useQuery({queryKey:['pr-summaries',ids,revision],queryFn:async({signal})=>{
  const items:Summary[]=[]
  for(let i=0;i<ids.length;i+=30){const params=new URLSearchParams();ids.slice(i,i+30).forEach(id=>params.append('pr',id));items.push(...(await get<{items:Summary[]}>('/api/v1/prs/summaries?'+params,signal)).items)}
  return Object.fromEntries(items.map(item=>[key(item),item]))
 },enabled:!!ids.length,retry:false,staleTime:30000,refetchOnWindowFocus:false})
}
export const summaryKey=key
function link(value?:string){try{const u=new URL(value||'');return u.protocol==='https:'&&!u.username&&!u.password?u.href:undefined}catch{return undefined}}
const titles:Record<Kind,string>={reviews:'Reviews',checks:'Checks',comments:'Comments','code-comments':'Code comments',assignees:'Assignees',stack:'Stacked PRs'}
export function reviewLabel(value?:string){return ({APPROVED:'Approved',CHANGES_REQUESTED:'Changes requested',REVIEW_REQUIRED:'Review required',approved:'Approved',changes_requested:'Changes requested',review_required:'Review required'} as Record<string,string>)[value||'']||'No review decision'}
function statusStyle(state:string){
 const value=state.toUpperCase()
 if(['APPROVED','SUCCESS'].includes(value))return {Icon:CircleCheck,tone:'success'}
 if(value==='CHANGES_REQUESTED')return {Icon:FileDiff,tone:'danger'}
 if(['FAILURE','ERROR','TIMED_OUT','CANCELLED','ACTION_REQUIRED','STARTUP_FAILURE'].includes(value))return {Icon:CircleX,tone:'danger'}
 if(value==='REVIEW_REQUIRED')return {Icon:CircleMinus,tone:'pending'}
 if(['PENDING','QUEUED','IN_PROGRESS','WAITING','REQUESTED','EXPECTED'].includes(value))return {Icon:Clock3,tone:'pending'}
 return {Icon:CircleMinus,tone:'muted'}
}
function metadataStyle(kind:Kind,summary?:Summary){
 if(!summary)return {Icon:CircleHelp,tone:'muted'}
 if(kind==='reviews')return statusStyle(summary.review)
 if(kind==='checks'){const c=summary.checks;return statusStyle(!c?.total?'NONE':c.failed?'FAILURE':c.pending?'PENDING':c.passed?'SUCCESS':'SKIPPED')}
 return {Icon:kind==='comments'||kind==='code-comments'?MessageSquare:kind==='assignees'?Users:Layers,tone:'muted'}
}
export function DetailButton({pr,kind,summary,label,loading=false}:{pr:PRIdentity;kind:Kind;summary?:Summary;label:string;loading?:boolean}){
 const [open,setOpen]=useState(false)
 const {Icon,tone}=metadataStyle(kind,loading?undefined:summary)
 return <Dialog open={open} onOpenChange={setOpen}><DialogTrigger asChild><button className={`pr-detail-trigger pr-status-${tone}`} title={`${titles[kind]}: ${label}. Open details.`} aria-label={`${titles[kind]} for ${pr.title}: ${label}`}><Icon size={16} aria-hidden='true'/><span>{loading?'Loading…':label.replace(/^[✕◷✓] /,'')}</span></button></DialogTrigger><DialogContent className='pr-detail-dialog'><DialogHeader><DialogTitle>{titles[kind]}</DialogTitle><DialogDescription>{pr.repository} · #{pr.number} · {pr.title}</DialogDescription></DialogHeader>{open&&<Details pr={pr} kind={kind} summary={summary}/>}</DialogContent></Dialog>
}
function Details({pr,kind,summary}:{pr:PRIdentity;kind:Kind;summary?:Summary}){
 const [tab,setTab]=useState(kind)
 const remote=tab!=='assignees'&&tab!=='stack'
 const query=useInfiniteQuery({queryKey:['pr-detail',key(pr),tab],initialPageParam:{page:1,cursor:'',head:''},queryFn:({signal,pageParam})=>{
  const p=new URLSearchParams({page:String(pageParam.page)});if(pageParam.cursor)p.set('cursor',pageParam.cursor);if(pageParam.head)p.set('head',pageParam.head)
  return get<DetailPage>(`/api/v1/prs/${key(pr)}/${tab}?${p}`,signal)
 },getNextPageParam:(last,pages)=>last.has_next?{page:pages.length+1,cursor:last.next_cursor||'',head:last.head||''}:undefined,enabled:remote,retry:false,staleTime:0,refetchOnWindowFocus:false})
 const items=query.data?.pages.flatMap(p=>p.items)||[]
 return <>
 {kind==='comments'&&<div className='pr-detail-tabs' aria-label='Comment type'><Button variant={tab==='comments'?'default':'outline'} onClick={()=>setTab('comments')}>Conversation</Button><Button variant={tab==='code-comments'?'default':'outline'} onClick={()=>setTab('code-comments')}>Code comments</Button></div>}
 {kind==='reviews'&&<p className='pr-help'>Current decision: {summary?reviewLabel(summary.review):'Unavailable'}. Review history below can include earlier or dismissed reviews.</p>}
 {kind==='checks'&&<p className='pr-help'>Checks for the current head commit. {summary?.checks&&`${summary.checks.passed} passing · ${summary.checks.failed} failing · ${summary.checks.pending} pending · ${summary.checks.skipped} skipped or neutral.`}</p>}
 {kind==='comments'&&<p className='pr-help'>Conversation includes comments from people and automation. Code comments are shown separately.</p>}
 <div className='pr-detail-scroll' tabIndex={0} aria-label={`${titles[tab]} details`}>
 {tab==='assignees'?<><p>Assignees are people explicitly responsible for this pull request. The author is not assigned automatically.</p>{summary?summary.assignees.length?<ul>{summary.assignees.map(a=><li key={a}>{a}</li>)}</ul>:<p>Unassigned</p>:<p>Assignees unavailable. Refresh the results to try again.</p>}</>:tab==='stack'?<><p>A registered stack links dependent pull requests in merge order.</p><p>{summary?.stack?`Position ${summary.stack.position} of ${summary.stack.total}`:summary?.stack_known?'Not in a registered stack. This does not rule out dependencies.':'Stack membership is unavailable.'}</p></>:<>
 {query.isPending&&<p role='status'>Loading {titles[tab].toLowerCase()}…</p>}
 {query.isError&&<div role='alert'><p>{query.error.message}</p><Button variant='outline' onClick={()=>void query.refetch()}>Retry</Button></div>}
 {!query.isPending&&!query.isError&&!items.length&&<p>No {titles[tab].toLowerCase()}.</p>}
 {items.map((item,index)=><article className='pr-detail-item' key={`${item.id}-${index}`}><header><strong>{item.name||item.author||'Unknown author'}</strong>{item.bot&&<span className='pr-chip'>Bot</span>}{item.state&&<Status state={item.state}/>}{item.created_at&&<time dateTime={item.created_at}>{new Date(item.created_at).toLocaleString()}</time>}</header>{item.path&&<p className='pr-help'>{item.path}</p>}{item.body&&<div className='pr-comment-body description-content'><ReactMarkdown remarkPlugins={[remarkGfm]} rehypePlugins={[rehypeRaw,rehypeSanitize]} components={{a:({href,children})=><a href={link(href)} target='_blank' rel='noreferrer'>{children}</a>,img:({src,alt,width,height})=><img src={link(src)} alt={alt||''} width={width} height={height} loading='lazy' referrerPolicy='no-referrer'/>}}>{item.body}</ReactMarkdown></div>}{link(item.url)&&<a href={link(item.url)} target='_blank' rel='noreferrer'>Open on source ↗</a>}</article>)}
 {query.hasNextPage&&<Button variant='outline' disabled={query.isFetchingNextPage} onClick={()=>void query.fetchNextPage()}>{query.isFetchingNextPage?'Loading…':'Load more'}</Button>}
 </>}
 </div></>
}

function Status({state}:{state:string}){const {Icon,tone}=statusStyle(state);return <span className={`pr-status pr-status-${tone}`}><Icon size={16} aria-hidden='true'/>{state.replaceAll('_',' ').toLowerCase()}</span>}
