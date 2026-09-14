import {test,expect} from '../../silverbullet/node_modules/@playwright/test/index.mjs';
for(const family of ['Default','Warm neutral'])for(const mode of ['Day','Night'])for(const width of [900,1440]){
 test(`${family} ${mode} ${width}`,async({page})=>{
 await page.setViewportSize({width,height:900});
 await page.route(/\/api\/conversation(?:\?|$)/,r=>r.fulfill({json:{id:'main',title:'UI regression fixture',can_create:true}}));
 await page.route('**/.proxy/iugum/api/messages*',r=>r.fulfill({json:[{role:'user',text:'Review this change',html:'<p>Review this change</p>',at:'2026-09-13T12:00:00Z'},{role:'assistant',text:'Ready for review',html:'<p>Ready for review.</p><ul><li>Search and chat share a theme.</li><li>Code stays readable.</li></ul><pre><code>const ready = true;</code></pre>',at:'2026-09-13T12:01:00Z'}]}));
 await page.goto('/');
 const appearance=page.getByTitle('Appearance: Day, Night, or System',{exact:true});await appearance.waitFor();
 await appearance.click();await page.getByText(mode,{exact:true}).click();
 await expect(page.locator('html')).toHaveAttribute('data-theme',mode==='Day'?'light':'dark');
 await appearance.waitFor();
 if(family==='Warm neutral'){await appearance.click();await page.getByText('Theme family…',{exact:true}).click();await page.getByText(family,{exact:true}).click();await expect(page.locator('html')).toHaveAttribute('data-iugum-theme','warm');}
 await page.getByText('Review this change',{exact:true}).waitFor();
 await expect.poll(async()=>{
  const searchBounds=await page.locator('#iugum-chief-tools').boundingBox();
  const actionBounds=await page.locator('#sb-top .sb-actions').boundingBox();
  return !!searchBounds && !!actionBounds && searchBounds.x+searchBounds.width<=actionBounds.x;
 }).toBe(true);
 // Fixed local fixtures; no transcript content or model calls enter snapshots.
 await expect(page).toHaveScreenshot(`${family.replaceAll(' ','-').toLowerCase()}-${mode.toLowerCase()}-${width}.png`);
 });
}
