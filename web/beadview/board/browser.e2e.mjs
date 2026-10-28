import {chromium} from '../../../silverbullet/node_modules/playwright/index.mjs'
import {createServer} from 'node:http'
import {readFileSync,existsSync} from 'node:fs'
import path from 'node:path'
import assert from 'node:assert/strict'
const root=path.resolve(import.meta.dirname,'../assets')
const server=createServer((req,res)=>{let file=path.join(root,new URL(req.url,'http://localhost').pathname);if(!existsSync(file)||!path.extname(file))file=path.join(root,'index.html');const types={'.js':'application/javascript','.css':'text/css','.html':'text/html'};res.setHeader('Content-Type',types[path.extname(file)]||'application/octet-stream');res.end(readFileSync(file))})
await new Promise(r=>server.listen(0,'127.0.0.1',r));const base=`http://127.0.0.1:${server.address().port}`
const now=Date.now(),day=86400000
const cards=[
 ['alpha','a-old','Old alpha','closed',-30],['alpha','a-recent','Recent alpha','closed',-1],['alpha','a-missing','Unknown date alpha','closed',null],['alpha','a-working','Working alpha','in_progress',null],
 ['beta','b-old','Old beta','closed',-45],['beta','b-recent','Recent beta','closed',-2],['beta','b-ready','Ready beta','open',null],
].map(([project,id,title,status,days])=>({project,id,title,status,priority:'p2',assignee:'worker',labels:[],updated_at:new Date(now).toISOString(),closed_at:days===null?'':new Date(now+days*day).toISOString(),description:'Retained complete ticket history'}))
const original='/?view=board&project=alpha&project=beta&lanes=1'
const browser=await chromium.launch({headless:true,channel:'chrome'})
try{
 for(const colorScheme of ['light','dark'])for(const width of [390,1200]){
  const context=await browser.newContext({viewport:{width,height:900},colorScheme,reducedMotion:'reduce'})
  const page=await context.newPage();let writes=0
  await page.route('**/api/**',async route=>{
   const req=route.request(),u=new URL(req.url());if(req.method()!=='GET')writes++
   let body
   if(u.pathname==='/api/projects')body={projects:[{slug:'alpha',name:'alpha'},{slug:'beta',name:'beta'}]}
   else if(u.pathname==='/api/config')body={read_only:true,multi:true,ws_port:null}
   else if(u.pathname==='/api/beads')body=u.searchParams.get('project')?cards.filter(c=>c.project===u.searchParams.get('project')):cards
   else if(u.pathname==='/api/iugum/activity')body={project:u.searchParams.get('project'),issues:cards.filter(c=>c.project===u.searchParams.get('project')).map(c=>({id:c.id,state:c.status==='closed'?'completed':'unknown',actor:'worker',executions:[]}))}
   else if(u.pathname.startsWith('/api/bead/')){const parts=u.pathname.split('/');body={...cards.find(c=>c.project===parts[3]&&c.id===parts[4]),comments:[],dependents:[]}}
   else return route.fulfill({status:404,body:'{}'})
   await route.fulfill({contentType:'application/json',body:JSON.stringify(body)})
  })
  await page.goto(base+original)
  const allClosed=page.getByRole('checkbox',{name:'All closed',exact:true})
  await allClosed.waitFor();assert.equal(await allClosed.isChecked(),false)
  await page.getByRole('button',{name:'Recent alpha',exact:true}).waitFor()
  assert.equal(await page.getByRole('button',{name:'Old alpha',exact:true}).count(),0)
  assert.equal(await page.getByRole('button',{name:'Old beta',exact:true}).count(),0)
  await page.getByText('Board: 5 of 7 matching cards · 2 older closed cards hidden',{exact:true}).waitFor()
  await page.getByText('1 closed card has an uncertain close date; kept visible.',{exact:true}).waitFor()
  const alpha=page.locator('[data-lane="alpha"] > button')
  const beta=page.locator('[data-lane="beta"] > button')
  assert.match(await alpha.innerText(),/3 cards/);assert.match(await beta.innerText(),/2 cards/)
  const betaBefore=(await beta.boundingBox()).y
  await alpha.click();assert.equal(await alpha.getAttribute('aria-expanded'),'false')
  assert.equal(await page.getByRole('button',{name:'Recent alpha',exact:true}).count(),0)
  assert.equal(await page.getByRole('button',{name:'Recent beta',exact:true}).isVisible(),true)
  assert.ok((await beta.boundingBox()).y<betaBefore-100,'next project did not move upward')
  assert.equal(await alpha.evaluate(e=>document.activeElement===e),true)
  await page.keyboard.press('Enter');assert.equal(await alpha.getAttribute('aria-expanded'),'true')
  await page.keyboard.press('Space');assert.equal(await alpha.getAttribute('aria-expanded'),'false')
  await allClosed.check()
  await page.getByText('Board: 7 of 7 matching cards',{exact:true}).waitFor()
  assert.match(await alpha.innerText(),/4 cards/);assert.match(await beta.innerText(),/3 cards/)
  await page.reload();await page.getByRole('button',{name:'Expand project alpha',exact:true}).waitFor()
  assert.equal(await page.getByRole('checkbox',{name:'All closed',exact:true}).isChecked(),true)
  await page.goto(base+'/?view=board&project=beta&lanes=1')
  await page.getByRole('button',{name:'Old beta',exact:true}).waitFor()
  assert.equal(await page.getByRole('button',{name:/project beta$/}).count(),0,'normal board gained a project disclosure')
  await page.goto(base+original);await page.getByRole('button',{name:'Expand project alpha',exact:true}).waitFor()
  await page.getByRole('button',{name:'Expand project alpha',exact:true}).click()
  await page.getByRole('button',{name:'Old alpha',exact:true}).waitFor()
  await page.getByRole('checkbox',{name:'All closed',exact:true}).uncheck()
  assert.equal(await page.getByRole('button',{name:'Old alpha',exact:true}).count(),0)
  await page.getByRole('button',{name:'Collapse project alpha',exact:true}).scrollIntoViewIfNeeded()
  await page.screenshot({path:`/private/tmp/iugum-board-${colorScheme}-${width}.png`})
  await page.getByRole('tab',{name:/List/}).click()
  await page.getByRole('button',{name:'Old alpha',exact:true}).waitFor()
  assert.equal(await page.getByRole('checkbox',{name:'All closed',exact:true}).count(),0)
  await page.goto(base+'/bead/alpha/a-old')
  await page.getByRole('heading',{name:'Old alpha',exact:true}).waitFor()
  await page.getByText('Retained complete ticket history',{exact:true}).waitFor()
  assert.equal(writes,0,'display controls mutated tracker')
  await context.close()
 }
 console.log('PASS board retention + collapse: two projects, inclusive source logic, counts, keyboard/focus, reload/filter persistence, full List/detail, narrow/wide light/dark, no writes')
}finally{await browser.close();server.close()}
