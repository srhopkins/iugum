import {test} from 'node:test';
import assert from 'node:assert/strict';
import {readFile} from 'node:fs/promises';
import postcss from '../../silverbullet/node_modules/postcss/lib/postcss.mjs';
import {panelCSS,hostCSS} from './ui-styles.js';

test('feature modules keep static presentation in shared styles',async()=>{
 for(const file of ['chief.js','ui-controls.js']){
  const source=await readFile(new URL(file,import.meta.url),'utf8');
  assert.doesNotMatch(source,/\.style\.cssText\s*=|\.style\.\w+\s*=\s*['"]|setAttribute\(\s*['"]style['"]|\bstyle\s*=\s*['"]/,
   `${file}: use shared styles; calculated geometry may use style.setProperty`);
 }
});
test('shared CSS has no duplicate properties within a rule',()=>{
 for(const [name,css] of Object.entries({panelCSS,hostCSS})){
  postcss.parse(css).walkRules(rule=>{
   const properties=new Set();
   for(const declaration of rule.nodes.filter(node=>node.type==='decl')){
    assert.ok(!properties.has(declaration.prop),`${name}: ${rule.selector} repeats ${declaration.prop}`);
    properties.add(declaration.prop);
   }
  });
 }
});
