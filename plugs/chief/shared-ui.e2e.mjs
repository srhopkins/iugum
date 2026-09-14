import {chromium} from '../../silverbullet/node_modules/playwright/index.mjs';
import assert from 'node:assert/strict';
const browser=await chromium.launch({headless:true});
try {
 const page=await browser.newPage({serviceWorkers:'block'});
 await page.route('**/.proxy/iugum/api/messages',r=>r.fulfill({json:Array.from({length:30},(_,i)=>({role:i%2?'assistant':'user',text:'Message '+i,html:'<p>Message '+i+'</p>',at:'2026-09-14T00:00:00Z'}))}));
 await page.goto(process.env.CHIEF_UI_TEST_URL);
 const more=page.getByRole('button',{name:'More chat options',exact:true});
 await more.waitFor({timeout:60000});
 await more.click();
 assert.equal(await more.getAttribute('aria-expanded'),'true');
 const first=page.getByRole('menuitem',{name:'Export transcript',exact:true});
 assert(await first.evaluate(e=>e.getRootNode().activeElement===e));
 await first.press('ArrowDown');
 const settings=page.getByRole('menuitem',{name:'Agent settings',exact:true});
 assert(await settings.evaluate(e=>e.getRootNode().activeElement===e));
 await settings.press('Escape');
 assert.equal(await more.getAttribute('aria-expanded'),'false');
 assert(await more.evaluate(e=>e.getRootNode().activeElement===e));
 const messages=page.locator('.chief-messages');
 await messages.evaluate(e=>{e.scrollTop=0;e.dispatchEvent(new Event('scroll'));});
 const latest=page.getByRole('button',{name:'Jump to latest',exact:true});await latest.waitFor();
 await latest.click();assert(await latest.isHidden());
 const draft=page.getByRole('textbox',{name:'Message agent',exact:true});await draft.fill('Keep my draft');
 const agents=page.getByRole('button',{name:'Agents',exact:true});await agents.click();
 await page.locator('#sb-main .cm-editor').click({position:{x:20,y:20}});
 assert(await page.getByRole('complementary',{name:'Agents drawer'}).isHidden());
 assert.equal(await draft.inputValue(),'Keep my draft');
 assert(await agents.evaluate(e=>e.getRootNode().activeElement===e));
 // Shared tokens cross the native/shadow boundary without feature-specific rules.
 await page.addStyleTag({content:':root { --iugum-reading-size: 19px; --iugum-reading-font: Georgia, serif; --iugum-icon-size: 23px; }'});
 assert.equal(await page.locator('#sb-main .cm-editor').evaluate(e=>getComputedStyle(e).fontSize),'19px');
 assert.equal(await more.locator('svg').evaluate(e=>getComputedStyle(e).width),'23px');
 const hasStaticStyle=await page.locator('.chief-chat-title').evaluate(e=>e.hasAttribute('style'));
 assert.equal(hasStaticStyle,false);
 console.log('Shared menu focus, arrow navigation, Escape, and cross-boundary theme tokens pass.');
} finally {await browser.close();}
