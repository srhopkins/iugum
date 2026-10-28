import {chromium} from '../../../silverbullet/node_modules/playwright/index.mjs'
import {createServer} from 'node:http'
import {readFileSync,existsSync} from 'node:fs'
import path from 'node:path'
import assert from 'node:assert/strict'
const root=path.resolve(import.meta.dirname,'../assets')
const server=createServer((req,res)=>{let file=path.join(root,new URL(req.url,'http://localhost').pathname);if(!existsSync(file)||!path.extname(file))file=path.join(root,'index.html');const types={'.js':'application/javascript','.css':'text/css','.html':'text/html'};res.setHeader('Content-Type',types[path.extname(file)]||'application/octet-stream');res.end(readFileSync(file))})
await new Promise(r=>server.listen(0,'127.0.0.1',r));const base=`http://127.0.0.1:${server.address().port}`
const browser=await chromium.launch({headless:true,channel:'chrome'})
try{
 for(const colorScheme of ['light','dark'])for(const width of [390,1200]){
  const context=await browser.newContext({viewport:{width,height:850},colorScheme})
  const page=await context.newPage();let state='working';let calls=0
  const bead={project:'alpha',id:'same',title:'Verify active worker',status:'in_progress',priority:'p2',assignee:'worker',labels:[],updated_at:'2026-10-20T00:00:00Z'}
  await page.route('**/api/**',async route=>{
   const u=new URL(route.request().url());let body
   if(u.pathname==='/api/projects')body={projects:[{slug:'alpha',name:'alpha'},{slug:'beta',name:'beta'}]}
   else if(u.pathname==='/api/config')body={read_only:true,multi:true,ws_port:null}
   else if(u.pathname==='/api/beads')body=[bead]
   else if(u.pathname.startsWith('/api/bead/'))body={...bead,comments:[],dependents:[]}
   else if(u.pathname==='/api/iugum/activity'){
    assert.equal(u.searchParams.get('project'),'alpha');calls++
    body={project:'alpha',issues:[{id:'same',state,actor:'worker',seen_at:new Date().toISOString(),expires_at:new Date(Date.now()+60000).toISOString(),reason:state==='paused'?'Waiting for review':'',executions:[{version:1,run_id:'verified-run',actor:'worker',label:'Verified worker',platform:'codex',host:'test',session_id:'actual-session',agent_id:'actual-agent',cwd:'/fixture',started_at:'2026-10-20T00:00:00Z',last_seen:'2026-10-20T00:01:00Z',transcript:'/fixture/transcript.jsonl'}]}]}
   } else return route.fulfill({status:404,body:'{}'})
   await route.fulfill({contentType:'application/json',body:JSON.stringify(body)})
  })
  await page.goto(base+'/?view=board&project=alpha')
  await page.getByText('Working',{exact:true}).waitFor()
  const wheel=page.locator('[data-working="true"]')
  assert.notEqual(await wheel.evaluate(e=>getComputedStyle(e).animationName),'none')
  await page.emulateMedia({reducedMotion:'reduce'})
  assert.equal(await wheel.evaluate(e=>getComputedStyle(e).animationName),'none')
  // Theme variables, not feature-owned colors, control readable status text.
  const before=await wheel.evaluate(e=>getComputedStyle(e).color)
  await page.addStyleTag({content:':root{--muted-foreground:oklch(.6 .08 75);--border:oklch(.6 .08 75)}'})
  assert.notEqual(await wheel.evaluate(e=>getComputedStyle(e).color),before)
  await wheel.scrollIntoViewIfNeeded()
  await page.screenshot({path:`/private/tmp/iugum-activity-${colorScheme}-${width}.png`})
  state='paused'
  await page.getByText('Paused',{exact:true}).waitFor({timeout:20000})
  assert.equal(await wheel.count(),0)
  assert.match(await page.getByText('Paused',{exact:true}).locator('..').getAttribute('title'),/Waiting for review/)
  assert.ok(calls>=2,'activity did not poll')
  await page.getByRole('button',{name:'Verify active worker',exact:true}).click()
  await page.locator('summary').filter({hasText:'Verified worker'}).waitFor()
  await page.locator('summary').filter({hasText:'Verified worker'}).click()
  await page.getByText('actual-session',{exact:true}).waitFor()
  await page.getByText('actual-agent',{exact:true}).waitFor()
  assert.equal(await page.locator('a[href^="file:"]').count(),0)
  await context.close()
 }
 console.log('PASS activity browser: light/dark, narrow/wide, reduced motion, variable override, actual polling, pause and worker references')
}finally{await browser.close();server.close()}
