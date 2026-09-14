import {chromium} from '../../silverbullet/node_modules/playwright/index.mjs';
import AxeBuilder from '@axe-core/playwright';
import assert from 'node:assert/strict';
const browser=await chromium.launch({headless:true});
try {
 const context=await browser.newContext({serviceWorkers:'block'});
 const page=await context.newPage();
 await page.goto(process.env.CHIEF_UI_TEST_URL);
 await page.getByRole('button',{name:'More chat options',exact:true}).waitFor({timeout:60000});
 // Scan only owned UI; upstream editor findings must not obscure add-on failures.
 async function scan(){const result=await new AxeBuilder({page}).include('.sb-panel').include('#iugum-chief-tools').withTags(['wcag2a','wcag2aa','wcag21aa']).analyze();assert.deepEqual(result.violations.map(v=>({id:v.id,nodes:v.nodes.map(n=>n.target)})),[]);}
 await scan();
 await page.getByRole('button',{name:'More chat options',exact:true}).click();await scan();
 await page.getByRole('menuitem',{name:'Export transcript',exact:true}).press('Escape');
 await page.getByRole('button',{name:'Agents',exact:true}).click();await scan();
 console.log('axe accessibility checks pass for chat, search, menu and drawer.');
}finally{await browser.close();}
