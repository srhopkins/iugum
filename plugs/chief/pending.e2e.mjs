import {chromium} from '../../silverbullet/node_modules/playwright/index.mjs';
import assert from 'node:assert/strict';
const base=process.env.CHIEF_UI_TEST_URL;if(!base?.startsWith('http://127.0.0.1:'))throw Error('isolated URL required');
const b=await chromium.launch();try{
 const p=await b.newPage({serviceWorkers:'block'});let history=[],release;let failed=false;
 await p.route('**/.proxy/iugum/api/messages',r=>r.fulfill({json:history}));
 await p.route('**/.proxy/iugum/api/chat',async r=>{await new Promise(resolve=>release=resolve);if(failed){await r.fulfill({status:500,json:{error:'Test failure'}});return;}history=[{role:'user',text:'First prompt',at:new Date().toISOString()},{role:'assistant',text:'Reply',at:new Date().toISOString()}];await r.fulfill({json:{text:'Reply'}});});
 await p.goto(base);const composer=p.getByRole('textbox',{name:'Message agent',exact:true});await composer.waitFor({timeout:60000});
 await composer.fill('First prompt');await composer.press('Enter');
 await p.getByText('Working…',{exact:true}).waitFor();assert.equal(await composer.inputValue(),'');
 assert.equal(await p.locator('.chief-message.user').count(),1);assert.match(await p.locator('.chief-message.user').innerText(),/First prompt/);
 await composer.fill('Next draft');release();await p.getByText('Reply',{exact:true}).waitFor();
 assert.equal(await composer.inputValue(),'Next draft');assert.equal(await p.locator('.chief-message.user').count(),1);
 failed=true;await composer.press('Enter');await p.getByText('Working…',{exact:true}).waitFor();release();
 await p.locator('.chief-error').filter({hasText:'Test failure'}).waitFor();assert.equal(await composer.inputValue(),'Next draft');
 console.log('Prompt appears immediately, composer clears while pending, reply preserves next draft, no duplicate, failure restores draft.');
}finally{await b.close()}
