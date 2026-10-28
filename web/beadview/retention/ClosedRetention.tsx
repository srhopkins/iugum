import {useEffect,useId,useState} from 'react'

export const CLOSED_RETENTION_STORAGE='beadview-board-all-closed-v1'
export const CLOSED_RETENTION_MS=14*24*60*60*1000
type CloseDateCard={status:string;closed_at?:string}
type RetentionState<T>={cards:T[];hiddenOld:number;uncertainDates:number;allClosed:boolean;setAllClosed:(value:boolean)=>void}

// Unknown or future dates remain visible. Only a valid, past closed_at can
// hide history. updated_at is deliberately never read.
export function retainedClosedCards<T extends CloseDateCard>(cards:T[],allClosed:boolean,now:number){
 let hiddenOld=0,uncertainDates=0
 const kept=cards.filter(card=>{
  if(card.status!=='closed')return true
  const text=card.closed_at||''
  const syntax=/^\d{4}-\d{2}-\d{2}T(?:[01]\d|2[0-3]):[0-5]\d:[0-5]\d(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})$/.test(text)
  const calendar=syntax?new Date(text.slice(0,10)+'T00:00:00Z'):null
  const calendarValid=calendar!==null&&Number.isFinite(calendar.getTime())&&calendar.toISOString().slice(0,10)===text.slice(0,10)
  const at=syntax&&calendarValid?Date.parse(text):NaN
  if(!Number.isFinite(at)||at>now){uncertainDates++;return true}
  if(!allClosed&&at<now-CLOSED_RETENTION_MS){hiddenOld++;return false}
  return true
 })
 return {cards:kept,hiddenOld,uncertainDates}
}
function readAllClosed(){try{return localStorage.getItem(CLOSED_RETENTION_STORAGE)==='true'}catch{return false}}
export function useBoardClosedRetention<T extends CloseDateCard>(cards:T[],clock:()=>number=Date.now):RetentionState<T>{
 const [allClosed,update]=useState(readAllClosed)
 const [now,setNow]=useState(clock)
 useEffect(()=>{const timer=setInterval(()=>setNow(clock()),60000);return()=>clearInterval(timer)},[clock])
 const setAllClosed=(value:boolean)=>{update(value);try{localStorage.setItem(CLOSED_RETENTION_STORAGE,String(value))}catch{/* current-tab preference still works */}}
 return {...retainedClosedCards(cards,allClosed,now),allClosed,setAllClosed}
}
export function ClosedRetentionControls({state,shown,total}:{state:RetentionState<CloseDateCard>;shown:number;total:number}){
 const summary=useId()
 return <div className='flex min-w-0 flex-col gap-1 text-sm'>
  <div className='flex flex-wrap items-center gap-3'>
   <span>Closed: {state.allClosed?'all dates':'last 14 days'}</span>
   <label className='flex items-center gap-2'><input type='checkbox' checked={state.allClosed} aria-describedby={summary} onChange={e=>state.setAllClosed(e.target.checked)}/>All closed</label>
  </div>
  <p id={summary} className='text-xs text-muted-foreground'>Board: {shown} of {total} matching cards{state.hiddenOld>0?` · ${state.hiddenOld} older closed ${state.hiddenOld===1?'card':'cards'} hidden`:''}</p>
  {state.uncertainDates>0&&<p className='text-xs text-muted-foreground'>{state.uncertainDates} closed {state.uncertainDates===1?'card has':'cards have'} an uncertain close date; kept visible.</p>}
 </div>
}
