// Run against the isolated synthetic wiki fixture, never a personal message store.
import {chromium} from '../../silverbullet/node_modules/playwright/index.mjs';
import assert from 'node:assert/strict';
const base=process.env.CHIEF_UI_TEST_URL;
if(!base||!base.startsWith('http://127.0.0.1:'))throw Error('Set isolated CHIEF_UI_TEST_URL');
const browser=await chromium.launch({headless:true});
try {
 const page=await browser.newPage({viewport:{width:1440,height:960}});
 await page.goto(base);
 await page.getByRole('textbox',{name:'Message agent',exact:true}).waitFor({timeout:60000});
 const messages=page.locator('.chief-messages');
 assert(await messages.locator('strong').count()>0);
 assert(await messages.locator('pre code').count()>0);
 assert(await messages.locator('table').count()>0);
 const details=messages.locator('details').filter({has:page.locator('summary',{hasText:'Last recorded message'})}).last();
 assert.equal(await details.getAttribute('open'),null);
 await details.locator('summary').click();
 assert.match(await details.locator('pre code').innerText(),/\n  errorType/);
 await page.screenshot({path:'/private/tmp/chief-markdown-preview.png'});
 await messages.evaluate(el=>{el.scrollTop=100});
 const before=await messages.evaluate(el=>el.scrollTop);
 await page.getByRole('textbox',{name:'Message agent',exact:true}).fill('status');await page.getByRole('textbox',{name:'Message agent',exact:true}).press('Enter');
 await page.waitForResponse(r=>r.url().endsWith('/api/messages'));
 await page.getByRole('button',{name:'Send',exact:true}).isEnabled();
 assert(Math.abs(await messages.evaluate(el=>el.scrollTop)-before)<5);
 assert.notEqual(await details.getAttribute('open'),null);
 console.log('Markdown, code indentation, tables, collapsed evidence, expansion retention, and reading position pass.');
}finally{await browser.close()}
