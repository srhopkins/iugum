import {chromium} from '../../silverbullet/node_modules/playwright/index.mjs';
import assert from 'node:assert/strict';
const base=process.env.CHIEF_UI_TEST_URL;
if(!base?.startsWith('http://127.0.0.1:'))throw Error('Set isolated CHIEF_UI_TEST_URL');
const browser=await chromium.launch({headless:true});
try {
 const page=await browser.newPage({viewport:{width:1728,height:1000},serviceWorkers:'block'});
 // Use server-rendered saved Markdown, not preformatted HTML in a route stub.
 const response=await page.request.get(base+'/api/messages?conversation=ffffffffffffffffffffffffffffffff');
 assert.equal(response.status(),200);
 const saved=await response.json();
 await page.route('**/.proxy/iugum/api/messages*',r=>r.fulfill({json:saved}));
 await page.goto(base+'/LayoutRegression');
 const composer=page.getByRole('textbox',{name:'Message agent',exact:true});
 await composer.waitFor({timeout:60000});
 await page.locator('.chief-markdown strong').filter({hasText:'Formatted answer'}).waitFor();
 assert.equal(await page.locator('.chief-markdown ul li').count(),2);
 assert.match(await page.locator('.chief-markdown pre code').innerText(),/fmt.Println/);
 assert.equal(await page.locator('.chief-markdown table tbody tr').count(),1);
 assert.equal(await page.locator('.chief-markdown script').count(),0);
 const evidence=page.locator('.chief-markdown details').filter({has:page.getByText('Last recorded message',{exact:true})});
 assert.equal(await evidence.getAttribute('open'),null);
 await evidence.locator('summary').click();
 assert.equal(await evidence.locator('strong').innerText(),'Saved evidence');
 const toggle=page.getByRole('button',{name:'Chat',exact:true});
 const fits=()=>page.evaluate(()=>{
  const editor=document.querySelector('#sb-editor').getBoundingClientRect();
  const root=document.querySelector('#sb-root').getBoundingClientRect();
  const content=document.querySelector('#sb-editor .cm-content').getBoundingClientRect();
  return root.x===0 && document.body.scrollWidth<=innerWidth && editor.x>=0 && editor.right<=innerWidth && content.x>=editor.x-1 && content.right<=editor.right+1;
 });
 for(const viewport of [1728,900]) {
  await page.setViewportSize({width:viewport,height:1000});
  for(const mode of ['comfort','wide','full']) {
   for(const size of [290,500]) {
    await page.evaluate(({mode,size})=>{
     document.documentElement.setAttribute('data-editor-width',mode);
     document.documentElement.style.setProperty('--iugum-chat-panel-size',`0 0 ${size}px`);
    },{mode,size});
    await toggle.click();await toggle.click();
    // Reopen deliberately focuses the composer after 100ms; catch document scroll.
    await page.waitForTimeout(250);
    assert(await fits(),`${viewport}/${mode}/${size}: wiki remains inside viewport after chat opens`);
    assert(await composer.isVisible());
   }
  }
 }
 // Prove this fixture detects the original automatic-minimum overflow.
 await page.setViewportSize({width:1728,height:1000});
 await page.evaluate(()=>{
  document.documentElement.setAttribute('data-editor-width','full');
  document.documentElement.style.setProperty('--iugum-chat-panel-size','0 0 290px');
 });
 const broken=await page.addStyleTag({content:'#sb-editor { min-width:auto !important; }'});
 await toggle.click();await toggle.click();await page.waitForTimeout(250);
 assert.equal(await fits(),false,'negative control reproduces the original page displacement');
 await broken.evaluate(e=>e.remove());
 await composer.focus();
 console.log('Long wiki layouts stay in view across width modes; saved Markdown renders through the real server. Original overflow is detected.');
} finally {await browser.close();}
