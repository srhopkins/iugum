import {chromium} from '../../silverbullet/node_modules/playwright/index.mjs';
import assert from 'node:assert/strict';
const base=process.env.CHIEF_UI_TEST_URL;
const browser=await chromium.launch({headless:true});
try {
 const page=await browser.newPage();page.on("pageerror",e=>console.error("PAGE ERROR",e.message));page.on("console",m=>{if(m.type()==="error")console.error("BROWSER",m.text())});await page.goto(base);
 const search=page.getByRole('searchbox');await search.waitFor({timeout:60000});
 assert.equal(await page.getByRole('button',{name:'Chat',exact:true}).isVisible(),false);
 assert.equal(await page.getByRole('textbox',{name:'Message agent',exact:true}).count(),0);
 await search.fill('navigation');await page.getByRole('button',{name:'Search',exact:true}).click();
 await page.locator('#iugum-chief-results article').first().waitFor();
 await page.locator('#iugum-chief-results article>button').first().click();
 await page.waitForURL('**/SearchTarget');
 const status=await page.request.get(base+'/api/agents');assert.equal(status.status(),404);
 console.log('Standalone search works without agent chat; result navigates and chat endpoint is disabled.');
} finally {await browser.close()}
