import {chromium} from '../../silverbullet/node_modules/playwright/index.mjs';
import assert from 'node:assert/strict';
const base=process.env.CHIEF_UI_TEST_URL;
if(!base?.startsWith('http://127.0.0.1:'))throw Error('Set isolated CHIEF_UI_TEST_URL');
const browser=await chromium.launch({headless:true});
try{
 const page=await browser.newPage({colorScheme:'light',serviceWorkers:'block'});
 await page.route('**/.proxy/iugum/api/messages',r=>r.fulfill({json:[{role:'user',text:'Theme fixture',html:'<p>Theme fixture</p>',at:'2026-09-14T02:00:00Z'}]}));
 await page.goto(base);
 const button=page.getByTitle('Appearance: Day, Night, or System',{exact:true});
 const expectTheme=theme=>page.waitForFunction(t=>document.documentElement.dataset.theme===t,theme);
 async function choose(name){
  await button.waitFor({timeout:60000});await button.click();
  await page.getByText(name,{exact:true}).click();
  await page.waitForTimeout(400);await button.waitFor({timeout:60000});
 }
 await choose('Night');await expectTheme('dark');
 const prompt=page.locator('.chief-message.user').first();await prompt.waitFor();
 await page.waitForFunction(()=>{const host=[...document.querySelectorAll('.sb-panel')].find(e=>e.shadowRoot?.querySelector('.chief-message.user'));return host&&getComputedStyle(host.shadowRoot.querySelector('.chief-message.user')).backgroundColor==='rgb(34, 61, 114)';});
 assert.equal(await prompt.evaluate(e=>getComputedStyle(e).color),'rgb(247, 250, 254)');
 await page.reload();await button.waitFor();await expectTheme('dark');
 await choose('Day');await expectTheme('light');await prompt.waitFor();
 assert.equal(await prompt.evaluate(e=>getComputedStyle(e).backgroundColor),'rgb(231, 242, 255)');
 await page.emulateMedia({colorScheme:'dark'});await expectTheme('light');
 await choose('System');await expectTheme('dark');
 await page.emulateMedia({colorScheme:'light'});await expectTheme('light');
 await page.emulateMedia({colorScheme:'dark'});await expectTheme('dark');
 await page.reload();await button.waitFor();await expectTheme('dark');
 assert.equal(await button.locator('svg').count(),1);
 await choose('Theme family…');await page.getByText('Warm neutral',{exact:true}).click();
 await page.waitForFunction(()=>document.documentElement.dataset.iugumTheme==='warm');
 await prompt.waitFor();
 assert.equal(await prompt.evaluate(e=>getComputedStyle(e).backgroundColor),'rgb(72, 57, 35)');
 assert.equal(await prompt.evaluate(e=>getComputedStyle(e).borderTopColor),'rgb(156, 128, 83)');
 assert.equal(await prompt.evaluate(e=>getComputedStyle(e).borderTopLeftRadius),'3px');
 await page.reload();await button.waitFor();await expectTheme('dark');
 await page.waitForFunction(()=>document.documentElement.dataset.iugumTheme==='warm');
 await page.emulateMedia({colorScheme:'light'});await expectTheme('light');await prompt.waitFor();
 assert.equal(await prompt.evaluate(e=>getComputedStyle(e).backgroundColor),'rgb(238, 224, 196)');
 assert.equal(await prompt.evaluate(e=>getComputedStyle(e).borderTopColor),'rgb(186, 160, 120)');
 await choose('Theme family…');await page.getByText('Default',{exact:true}).click();
 await page.waitForFunction(()=>document.documentElement.dataset.iugumTheme==='default');
 assert.equal(await prompt.evaluate(e=>getComputedStyle(e).backgroundColor),'rgb(231, 242, 255)');
 console.log('Day/Night overrides, System live changes, and reload persistence pass.');
}finally{await browser.close()}
