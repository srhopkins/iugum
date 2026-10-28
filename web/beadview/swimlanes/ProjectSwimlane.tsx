import {useId,useState,type ReactNode} from 'react'
import {ChevronDown,ChevronRight} from 'lucide-react'
import {ProjectTag} from '@/components/bead-badges'
const STORAGE='beadview-project-collapsed-v1:'
function key(project:string){return STORAGE+encodeURIComponent(project)}
function read(project:string){try{return localStorage.getItem(key(project))==='true'}catch{return false}}

export function ProjectSwimlane({project,count,children}:{project:string;count:number;children:ReactNode}){
 const [collapsed,setCollapsed]=useState(()=>read(project))
 const body=useId();const countID=useId()
 const toggle=()=>{const next=!collapsed;setCollapsed(next);try{localStorage.setItem(key(project),String(next))}catch{/* current-tab choice still works */}}
 return <div data-lane={project} className='flex min-w-0 flex-col gap-2'>
  <button type='button' aria-expanded={!collapsed} aria-controls={body} aria-describedby={countID} aria-label={`${collapsed?'Expand':'Collapse'} project ${project}`} onClick={toggle}
   className='flex w-full min-w-0 flex-wrap items-center gap-2 rounded-md border border-border bg-card p-2 text-start text-card-foreground hover:bg-accent focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring'>
   {collapsed?<ChevronRight aria-hidden='true' className='size-4 shrink-0'/>:<ChevronDown aria-hidden='true' className='size-4 shrink-0'/>}
   <ProjectTag project={project}/>
   <span id={countID} className='text-xs text-muted-foreground'>{count} {count===1?'card':'cards'}</span>
  </button>
  <div id={body} hidden={collapsed}>{children}</div>
 </div>
}
