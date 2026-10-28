// Protocol probe only. No browser install, debugger, capture, or input API.
const assert = require('node:assert/strict');
const { randomUUID } = require('node:crypto');
const origin = 'https://iugum-agent-code.local';
const bridge = (frame, binding, relay) => async event => {
  if (event.source !== frame || event.origin !== origin) return false;
  const value = event.data;
  if (!value || Object.keys(value).sort().join(',') !== 'challenge,client,document,instance') return false;
  if (Object.keys(binding).some(key => value[key] !== binding[key])) return false;
  return relay(value);
};
(async () => {
  const request = async (route, value) => {
    const r = await fetch(process.env.PAIR_PROBE_URL + route, {
      method: 'POST', headers: {'Content-Type':'application/json'}, body: JSON.stringify(value),
    });
    return {ok:r.ok, value:await r.json()};
  };
  const a = {instance:randomUUID(),client:randomUUID(),document:randomUUID()};
  const b = {instance:randomUUID(),client:randomUUID(),document:randomUUID()};
  const ca = (await request('/challenge',a)).value;
  const cb = (await request('/challenge',b)).value;
  const frameA = {}, frameB = {}; // WindowProxy stand-ins: actual CSP remains unproved.
  let relays = 0;
  const receive = bridge(frameA, a, async value => { relays++;return (await request('/attest',value)).ok; });
  assert.equal(await receive({source:frameB,origin,data:{...a,challenge:ca.challenge}}),false);
  assert.equal(await receive({source:frameA,origin:'https://other.invalid',data:{...a,challenge:ca.challenge}}),false);
  assert.equal(await receive({source:frameA,origin,data:{...b,challenge:cb.challenge}}),false);
  assert.equal(relays,0);
  assert.equal(await receive({source:frameA,origin,data:{...a,challenge:cb.challenge}}),false);
  assert.equal(await receive({source:frameA,origin,data:{...a,challenge:ca.challenge}}),true);
  assert.equal(await receive({source:frameA,origin,data:{...a,challenge:ca.challenge}}),false);
  assert.equal((await request('/attest',{...b,document:randomUUID(),challenge:cb.challenge})).ok,false);
  assert.equal((await request('/attest',{...b,challenge:cb.challenge})).ok,true);
  console.log('nonce-only two-window protocol PASS; real WindowProxy/CSP not verified');
})().catch(error=>{console.error(error);process.exitCode=1;});
