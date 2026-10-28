import {expect,it,vi,afterEach} from 'vitest'
import {fireEvent,render,screen,cleanup} from '@testing-library/react'
import {retainedClosedCards,ClosedRetentionControls,useBoardClosedRetention,CLOSED_RETENTION_STORAGE} from '@/components/iugum-closed-retention'
const now=Date.parse('2026-10-20T00:00:00Z')
const day=24*60*60*1000
const stamp=(offset:number)=>new Date(now+offset).toISOString()
const rows=[
 {id:'recent',project:'a',status:'closed',closed_at:stamp(-day)},
 {id:'boundary',project:'b',status:'closed',closed_at:stamp(-14*day)},
 {id:'old',project:'a',status:'closed',closed_at:stamp(-14*day-1),updated_at:stamp(-100)},
 {id:'future',project:'b',status:'closed',closed_at:stamp(day)},
 {id:'missing',project:'a',status:'closed'},
 {id:'invalid',project:'b',status:'closed',closed_at:'not a date'},
 {id:'working',project:'a',status:'in_progress',closed_at:stamp(-90*day)},
]
afterEach(()=>{cleanup();localStorage.clear();vi.useRealTimers()})
it('retains the exact14day boundary and uncertain dates, using closed_at not updated_at',()=>{
 const result=retainedClosedCards(rows,false,now)
 expect(result.cards.map(b=>b.id)).toEqual(['recent','boundary','future','missing','invalid','working'])
 expect(result.hiddenOld).toBe(1)
 expect(result.uncertainDates).toBe(3)
 expect(rows).toHaveLength(7)
})
it('All closed restores history across projects without changing source data',()=>{
 expect(retainedClosedCards(rows,true,now).cards).toEqual(rows)
 expect(retainedClosedCards(rows,true,now).hiddenOld).toBe(0)
})
function Example(){const state=useBoardClosedRetention(rows,()=>now);return <><ClosedRetentionControls state={state} shown={state.cards.length} total={rows.length}/><div>{state.cards.map(b=><span key={b.id}>{b.id}</span>)}</div></>}
it('defaults to14days, persists All closed after remount, and keeps source counts clear',()=>{
 const first=render(<Example/>);
 expect(screen.queryByText('old',{exact:true})).toBeNull()
 expect(screen.getByText(/^Board: 6 of 7 matching cards/)).toBeInTheDocument()
 expect(screen.getByText(/3 closed cards have an uncertain/)).toBeInTheDocument()
 fireEvent.click(screen.getByRole('checkbox',{name:'All closed'}))
 expect(screen.getByText('old',{exact:true})).toBeInTheDocument()
 expect(localStorage.getItem(CLOSED_RETENTION_STORAGE)).toBe('true')
 first.unmount();render(<Example/>);
 expect(screen.getByRole('checkbox',{name:'All closed'})).toBeChecked()
 expect(screen.getByText('old',{exact:true})).toBeInTheDocument()
})
it('invalid stored preference and old URLs preserve the default',()=>{
 window.history.replaceState(null,'','/?project=a&lanes=1&view=board')
 localStorage.setItem(CLOSED_RETENTION_STORAGE,'corrupt')
 render(<Example/>);
 expect(screen.getByRole('checkbox',{name:'All closed'})).not.toBeChecked()
 expect(window.location.search).toBe('?project=a&lanes=1&view=board')
})

it('keeps impossible calendar timestamps instead of normalizing and hiding them',()=>{
 const invalid=[{id:'bad-calendar',status:'closed',closed_at:'2026-02-31T00:00:00Z'}]
 const result=retainedClosedCards(invalid,false,now)
 expect(result.cards).toEqual(invalid)
 expect(result.uncertainDates).toBe(1)
})
