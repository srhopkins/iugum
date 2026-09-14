import {chromium} from '../../silverbullet/node_modules/playwright/index.mjs';
import assert from 'node:assert/strict';
const base=process.env.CHIEF_UI_TEST_URL;
if(!base?.startsWith('http://127.0.0.1:'))throw Error('Set isolated CHIEF_UI_TEST_URL');
const browser=await chromium.launch({headless:true});
try {
 const page=await browser.newPage();await page.goto(base);
 await page.getByRole('textbox',{name:'Message agent',exact:true}).waitFor({timeout:60000});
 if (process.env.CHIEF_KEYBOARD_NEGATIVE_CONTROL === '1') {
  await page.getByRole('textbox',{name:'Message agent',exact:true}).evaluate(el => { el.onkeydown = null; });
 }
 const input=page.getByRole('textbox',{name:'Message agent',exact:true});
 await input.waitFor({timeout:60000});
 const wiki=await page.locator('#sb-editor .cm-content').innerText();
 await input.fill('abcd');await input.press('ArrowLeft');
 assert.equal(await input.evaluate(e=>e.selectionStart),3);
 await input.press('Backspace');assert.equal(await input.inputValue(),'abd');
 await input.press('Delete');assert.equal(await input.inputValue(),'ab');
 await input.press('Shift+ArrowLeft');await input.press('Backspace');
 assert.equal(await input.inputValue(),'a');
 await input.press('Shift+Enter');await input.press('b');
 assert.equal(await input.inputValue(),'a\nb');
 await input.press(process.platform==='darwin'?'Meta+a':'Control+a');
 await input.press('Backspace');assert.equal(await input.inputValue(),'');
 assert.equal(await page.locator('#sb-editor .cm-content').innerText(),wiki);
 console.log('Composer arrow, backward/forward deletion, selection, select-all, multiline editing pass; wiki unchanged.');
}finally{await browser.close()}
