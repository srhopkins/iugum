import {chromium} from '../../../silverbullet/node_modules/playwright/index.mjs'
import {createServer} from 'node:http'
import {readFileSync,existsSync} from 'node:fs'
import path from 'node:path'
import assert from 'node:assert/strict'
const root=path.resolve(import.meta.dirname,'../assets')
const server=createServer((req,res)=>{let file=path.join(root,new URL(req.url,'http://localhost').pathname);if(!existsSync(file)||!path.extname(file))file=path.join(root,'index.html');res.setHeader('Content-Type',({'.js':'application/javascript','.css':'text/css','.html':'text/html'})[path.extname(file)]||'application/octet-stream');res.end(readFileSync(file))})
await new Promise(r=>server.listen(0,'127.0.0.1',r));const base=`http://127.0.0.1:${server.address().port}`
const browser=await chromium.launch({headless:true})
try{
 for(const colorScheme of ['light','dark'])for(const width of [390,1280]){
  const context=await browser.newContext({viewport:{width,height:900},colorScheme,reducedMotion:'reduce'})
  const page=await context.newPage();let mode='normal';const requests=[];const errors=[]
  page.on('pageerror',e=>errors.push(e.message))
  await page.route('**/api/**',async route=>{
   const req=route.request(),u=new URL(req.url());assert.equal(req.method(),'GET')
   let body
   if(u.pathname==='/api/v1/prs'){
    requests.push(u.search)
    if(mode==='error')return route.fulfill({status:429,contentType:'application/json',body:JSON.stringify({error:{code:'rate_limited',message:'GitHub rate limit reached. Wait before refreshing.'}})})
    const pageNumber=Number(u.searchParams.get('page')||1)
    body={items:mode==='empty'?[]:[{number:pageNumber===1?7:8,repository:'alpha/api',title:pageNumber===1?'Fix paging':'Next result',url:'https://github.com/alpha/api/pull/7',author:'sam',state:'open',draft:false,updated_at:'2026-10-27T18:00:00Z',review:'not_loaded',checks:'not_loaded'}],total_count:mode==='empty'?0:mode==='incomplete'?1200:31,page:pageNumber,per_page:30,has_next:pageNumber===1,incomplete_results:mode==='incomplete',truncated:mode==='incomplete',fetched_at:new Date().toISOString()}
   }else if(u.pathname==='/api/v1/prs/summaries')body={items:[{repository:'alpha/api',number:7,review:'APPROVED',checks:{total:4,passed:2,failed:1,pending:1,skipped:0},comments:2,assignees:[],stack:null}]}
   else if(u.pathname.endsWith('/reviews'))body={items:[{id:1,author:'reviewer',body:'Looks good',state:'APPROVED'}],has_next:false}
   else if(u.pathname.endsWith('/checks'))body={items:[{id:1,name:'Unit tests',state:'FAILURE',body:'A test failed',url:'https://github.com/alpha/api/actions/runs/1'}],has_next:false,head:'abc'}
   else if(u.pathname.endsWith('/code-comments'))body={items:[{id:3,author:'sam',body:'Please simplify this',path:'main.go'}],has_next:false}
   else if(u.pathname.endsWith('/comments'))body={items:[{id:2,author:'build[bot]',bot:true,body:'**Automation report** ✅ [Run link](https://github.com/alpha/api/actions/runs/1)\n\n<details><summary>Run properties</summary><table><tr><th>Result</th></tr><tr><td>Passed</td></tr></table></details><script>window.badComment=true</script><img src=x onerror=window.badComment=true><a href=javascript:alert(1)>Unsafe</a>\n\n'+('Detail line\n\n'.repeat(100))}],has_next:false}
   else if(u.pathname==='/api/projects')body={projects:[{slug:'fixture',name:'fixture'}]}
   else if(u.pathname==='/api/config')body={read_only:true,multi:false,ws_port:null}
   else if(u.pathname==='/api/beads')body=[]
   else return route.fulfill({status:404,body:'{}'})
   return route.fulfill({contentType:'application/json',body:JSON.stringify(body)})
  })
  await page.goto(base+'/?view=list&label=keep')
  await page.getByRole('link',{name:'Pull requests',exact:true}).click()
  await page.getByRole('link',{name:'#7 Fix paging'}).waitFor()
  await page.getByRole('button',{name:'Remove Open filter',exact:true}).click()
  await page.getByLabel('Quick PR state',{exact:true}).selectOption('open')
  await page.getByRole('button',{name:'Remove Open filter',exact:true}).waitFor()
  const reviews=page.getByRole('button',{name:'Reviews for Fix paging: Approved',exact:true})
  await reviews.waitFor()
  assert.equal(await reviews.locator('svg').count(),1)
  await reviews.click();await page.getByText('Looks good',{exact:true}).waitFor();await page.keyboard.press('Escape')
  assert.equal(await reviews.evaluate(e=>document.activeElement===e),true)
  await page.getByRole('button',{name:'Checks for Fix paging: ✕ 2/4',exact:true}).click()
  await page.getByText('Unit tests',{exact:true}).waitFor();await page.keyboard.press('Escape')
  await page.getByRole('button',{name:'Comments for Fix paging: 2',exact:true}).click()
  await page.getByText('Bot',{exact:true}).waitFor()
  await page.getByRole('link',{name:'Run link',exact:true}).waitFor()
  await page.getByText('Run properties',{exact:true}).click()
  await page.getByRole('cell',{name:'Passed',exact:true}).waitFor()
  assert.equal(await page.locator('.pr-comment-body script,.pr-comment-body [onerror],.pr-comment-body a[href^="javascript:"]').count(),0)

  assert.ok(await page.locator('.pr-detail-scroll').evaluate(e=>e.scrollHeight>e.clientHeight),'comments must scroll inside the dialog')
  await page.getByRole('button',{name:'Code comments',exact:true}).click();await page.getByText('Please simplify this',{exact:true}).waitFor()
  await page.keyboard.press('Escape')
  await page.getByRole('button',{name:'Assignees for Fix paging: Unassigned',exact:true}).click()
  await page.getByText('The author is not assigned automatically.',{exact:false}).waitFor();await page.keyboard.press('Escape')
  await page.getByRole('button',{name:'Organization ▾',exact:true}).click()
  await page.getByRole('button',{name:'Pin organization alpha',exact:true}).click()
  await page.keyboard.press('Escape')
  const quick=page.getByRole('button',{name:'Toggle organization alpha',exact:true})
  assert.equal(await quick.getAttribute('aria-pressed'),'false')
  await quick.click();await page.waitForURL('**org=alpha*');await page.getByRole('link',{name:'#7 Fix paging'}).waitFor()
  assert.equal(await quick.getAttribute('aria-pressed'),'true')
  await page.getByRole('button',{name:'Organization (1) ▾',exact:true}).click()
  const input=page.getByPlaceholder('Search or enter exact name')
  await input.fill('beta');await page.getByRole('button',{name:'Add beta',exact:true}).click()
  await page.keyboard.press('Escape');await page.getByRole('button',{name:'Remove Organization: beta filter',exact:true}).waitFor()
  assert.deepEqual(new URL(page.url()).searchParams.getAll('org'),['alpha','beta'])
  await page.getByRole('button',{name:'Remove Organization: alpha filter',exact:true}).click()
  assert.equal(await quick.getAttribute('aria-pressed'),'false')
  await page.goBack();await page.getByRole('button',{name:'Remove Organization: alpha filter',exact:true}).waitFor()
  await page.getByRole('button',{name:'All filters',exact:true}).click()
  await page.getByLabel('Draft',{exact:true}).selectOption('yes')
  await page.keyboard.press('Escape');await page.getByRole('button',{name:'Remove Draft filter',exact:true}).waitFor()
  assert.equal(await page.getByRole('button',{name:'All filters',exact:true}).evaluate(e=>document.activeElement===e),true)
  await page.getByRole('button',{name:'Next',exact:true}).click();await page.getByRole('link',{name:'#8 Next result'}).waitFor()
  await page.getByRole('button',{name:'Remove Draft filter',exact:true}).click();await page.getByRole('link',{name:'#7 Fix paging'}).waitFor()
  assert.equal(new URL(page.url()).searchParams.has('page'),false)
  await page.getByRole('link',{name:'Beads',exact:true}).click()
  assert.equal(new URL(page.url()).searchParams.get('view'),'list');assert.equal(new URL(page.url()).searchParams.get('label'),'keep')
  await page.getByRole('link',{name:'Pull requests',exact:true}).click();await page.getByRole('button',{name:'Remove Organization: beta filter',exact:true}).waitFor()
  await page.reload();await page.getByRole('button',{name:'Toggle organization alpha',exact:true}).waitFor()
  await page.getByRole('button',{name:'Clear all · reset',exact:true}).click();await page.getByRole('link',{name:'#7 Fix paging'}).waitFor()
  assert.equal(await page.getByRole('button',{name:'Toggle organization alpha',exact:true}).getAttribute('aria-pressed'),'false')
  await page.getByRole('textbox',{name:'Search pull requests',exact:true}).fill('paging')
  await page.waitForURL('**q=paging*');await page.getByRole('button',{name:'Remove Text: paging filter',exact:true}).waitFor()
  mode='incomplete';await page.getByRole('button',{name:'Refresh',exact:true}).click();await page.getByRole('alert').filter({hasText:'Results are incomplete'}).waitFor()
  await page.screenshot({path:`/private/tmp/iugum-prs-${colorScheme}-${width}.png`,fullPage:true})
  assert.ok(await page.evaluate(()=>document.documentElement.scrollWidth<=window.innerWidth),'page overflows viewport')
  mode='error';await page.getByRole('button',{name:'Refresh',exact:true}).click();await page.getByRole('alert').filter({hasText:'GitHub rate limit reached'}).waitFor()
  mode='empty';await page.getByRole('button',{name:'Refresh',exact:true}).click();await page.getByText('No pull requests match these filters.',{exact:true}).waitFor()
  assert.equal(errors.length,0,errors.join('\n'))
  assert.ok(requests.some(s=>new URLSearchParams(s).getAll('org').length===2),'multi-select did not reach server')
  assert.ok(requests.every(s=>!new URLSearchParams(s).has('label')),'Beads filter leaked into PR API')
  await context.close()
 }
 console.log('PASS PR browser: light/dark, narrow/wide, pins, dropdowns, active chips, drawer/focus, history, pagination, text, errors, incomplete/empty states, section isolation, reload')
}finally{await browser.close();server.close()}
