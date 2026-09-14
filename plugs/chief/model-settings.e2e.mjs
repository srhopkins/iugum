import {chromium} from '../../silverbullet/node_modules/playwright/index.mjs';
import assert from 'node:assert/strict';
import AxeBuilder from '@axe-core/playwright';
const browser=await chromium.launch({headless:true});
try {
 const context=await browser.newContext({serviceWorkers:'block'}),page=await context.newPage();
 let selected='fast',mission='Original mission',deny=false;
 await page.route('**/api/models*',async route=>{
  if(route.request().method()==='POST')selected=route.request().postDataJSON().id;
  await route.fulfill({json:{selected,options:[{id:'fast',name:'Fast model'},{id:'deep',name:'Deep model'}]}});
 });
 await page.route('**/api/settings*',async route=>{
  if(route.request().method()==='POST'){
   if(deny){await route.fulfill({status:403,json:{error:'Permission revoked'}});return}
   mission=route.request().postDataJSON().value;
  }
  await route.fulfill({json:{fields:[{id:'mission',label:'Mission',value:mission,editable:true}]}});
 });
 await page.goto(process.env.CHIEF_UI_TEST_URL);
 const model=page.getByRole('combobox',{name:'Model',exact:true});
 await model.locator('option[value="deep"]').waitFor({state:'attached'});
 await Promise.all([page.waitForResponse(r=>r.url().includes('/api/models') && r.request().method()==='POST'),model.selectOption('deep')]);
 await page.reload();
 await model.locator('option[value="deep"]').waitFor({state:'attached'});
 assert.equal(await model.inputValue(),'deep');assert.equal(selected,'deep');
 await page.getByRole('button',{name:'More chat options',exact:true}).click();
 await page.getByRole('menuitem',{name:'Agent settings',exact:true}).click();
 const field=page.getByRole('textbox',{name:'Mission',exact:true});await field.fill('Updated mission');
 await page.getByRole('button',{name:'Save mission',exact:true}).click();
 await page.getByText('Saved. Applies to the next turn.',{exact:true}).waitFor();assert.equal(mission,'Updated mission');
 deny=true;await field.fill('Keep this unsaved draft');await page.getByRole('button',{name:'Save mission',exact:true}).click();
 await page.getByText('Could not save: Permission revoked',{exact:true}).waitFor();
 assert.equal(await field.inputValue(),'Keep this unsaved draft');assert.equal(mission,'Updated mission');
 const result=await new AxeBuilder({page}).include('.sb-panel').withTags(['wcag2a','wcag2aa','wcag21aa']).analyze();assert.deepEqual(result.violations.map(v=>v.id),[]);
 console.log('Model selection, reload, editable settings, denied-save draft recovery and settings accessibility pass.');
}finally{await browser.close()}
