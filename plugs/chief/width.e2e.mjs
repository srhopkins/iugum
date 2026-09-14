import {chromium} from '../../silverbullet/node_modules/playwright/index.mjs';
import assert from 'node:assert/strict';
const base=process.env.CHIEF_UI_TEST_URL;
if(!base?.startsWith('http://127.0.0.1:'))throw Error('Set isolated CHIEF_UI_TEST_URL');
const browser=await chromium.launch({headless:true});
try{
 const page=await browser.newPage({viewport:{width:2200,height:1000},serviceWorkers:'block'});
 await page.goto(base);
 const button=page.getByTitle('Cycle editor width (comfort -> wide -> full)',{exact:true});
 await button.waitFor({timeout:60000});
 assert.equal(await button.locator('svg path').getAttribute('d'),'M12 3h7a2 2 0 0 1 2 2v14a2 2 0 0 1-2 2h-7m0-18H5a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h7m0-18v18','columns icon renders instead of fallback question mark');
 await page.waitForFunction(()=>document.documentElement.getAttribute('data-editor-width')!==null);
 const mode=()=>page.locator('html').getAttribute('data-editor-width');
 const width=()=>page.locator('#sb-editor .cm-content').evaluate(e=>e.getBoundingClientRect().width);
 assert.equal(await mode(),'comfort');const comfort=await width();
 await button.click();await page.waitForTimeout(100);assert.equal(await mode(),'wide');assert((await width())>comfort+100);
 await button.click();await page.waitForTimeout(100);assert.equal(await mode(),'full');
 await page.reload();await button.waitFor();await page.waitForFunction(()=>document.documentElement.getAttribute('data-editor-width')==='full');assert.equal(await mode(),'full','width survives reload');
 await button.click();await page.waitForTimeout(100);assert.equal(await mode(),'comfort');
 const handle=page.getByRole('separator',{name:'Resize chat'});await handle.waitFor();
 const before=await page.locator('.chief-messages').boundingBox(),grip=await handle.boundingBox();
 await page.mouse.move(grip.x+4,grip.y+100);await page.mouse.down();await page.mouse.move(grip.x-100,grip.y+100,{steps:10});await page.mouse.up();
 assert((await page.locator('.chief-messages').boundingBox()).width>before.width+80);
 await button.click();await page.waitForTimeout(100);assert.equal(await mode(),'wide');
 assert(Math.abs((await page.locator('.chief-messages').boundingBox()).width-before.width-104)<5,'wiki width does not reset chat width');
 console.log('Native width icon cycles three widths, persists on reload and coexists with chat resizing.');
}finally{await browser.close()}
