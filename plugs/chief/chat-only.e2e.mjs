import {chromium} from '../../silverbullet/node_modules/playwright/index.mjs';
import assert from 'node:assert/strict';
const browser=await chromium.launch({headless:true});
try{const page=await browser.newPage();page.on("pageerror",e=>console.error("PAGE ERROR",e.message));page.on("console",m=>{if(m.type()==="error")console.error("BROWSER",m.text())});await page.goto(process.env.CHIEF_UI_TEST_URL);
 await page.getByRole('button',{name:'Chat',exact:true}).waitFor();
 await page.getByRole('textbox',{name:'Message agent',exact:true}).waitFor({timeout:60000});
 await page.reload();
 await page.getByRole('textbox',{name:'Message agent',exact:true}).waitFor({timeout:60000});
 await page.getByRole('button',{name:'Chat',exact:true}).click();
 await page.getByRole('textbox',{name:'Message agent',exact:true}).waitFor({state:'hidden'});
 await page.getByRole('button',{name:'Chat',exact:true}).click();
 await page.getByRole('textbox',{name:'Message agent',exact:true}).waitFor();
 assert.equal(await page.getByRole('searchbox').isVisible(),false);
 await page.getByText('No agents configured or permitted. Add an ACP connection or agent home to the wiki configuration.').waitFor();
 assert.equal(await page.getByRole('textbox',{name:'Message agent',exact:true}).isDisabled(),true);
 assert.equal((await page.request.get(process.env.CHIEF_UI_TEST_URL+'/api/search')).status(),404);
 console.log('Chat-only wiki hides search, blocks its endpoint, and explains missing agent configuration.');
}finally{await browser.close()}
