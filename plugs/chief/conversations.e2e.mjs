import {chromium} from '../../silverbullet/node_modules/playwright/index.mjs';
import assert from 'node:assert/strict';
const browser=await chromium.launch({headless:true});
try {
 const page=await browser.newPage();await page.goto(process.env.CHIEF_UI_TEST_URL);
 const input=page.getByRole('textbox',{name:'Message agent',exact:true});await input.waitFor({timeout:60000});
 await input.fill('Original draft');
 const create=page.getByRole('button',{name:'New chat',exact:true});
 await create.click();await page.waitForFunction(()=>[...document.querySelectorAll('.sb-panel')].some(h=>h.shadowRoot?.querySelector('textarea')?.value===''));
 await input.fill('Separate draft');
 await page.locator('.chief-chat-title').dblclick();const title=page.getByRole('textbox',{name:'Chat title',exact:true});await title.fill('Separate conversation');await title.press('Enter');await title.waitFor({state:'hidden'});
 await page.getByRole('button',{name:'Chat history',exact:true}).click();await page.getByRole('menuitem',{name:'Separate conversation',exact:true}).waitFor();
 const list=await page.evaluate(async()=>await (await fetch('/.proxy/iugum/api/conversations')).json());assert(list.length>=2);
 const original=list.find(x=>x.id==='main');await page.getByRole('menuitem',{name:original.title||'New conversation',exact:true}).click();
 await page.waitForFunction(()=>[...document.querySelectorAll('.sb-panel')].some(h=>h.shadowRoot?.querySelector('textarea')?.value==='Original draft'));
 await page.getByRole('button',{name:'Chat history',exact:true}).click();await page.getByRole('menuitem',{name:'Separate conversation',exact:true}).click();
 await page.waitForFunction(()=>[...document.querySelectorAll('.sb-panel')].some(h=>h.shadowRoot?.querySelector('textarea')?.value==='Separate draft'));
 await page.reload();await input.waitFor({timeout:60000});
 await page.waitForFunction(()=>[...document.querySelectorAll('.sb-panel')].some(h=>h.shadowRoot?.querySelector('textarea')?.value==='Separate draft'));
 console.log('New conversation, server title, history switching and separate drafts pass.');
}finally{await browser.close()}
