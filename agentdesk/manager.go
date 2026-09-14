package agentdesk

const agentManagerPage = `<!doctype html><html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width"><title>Agents · iugum</title><style>
body{font:16px system-ui;max-width:850px;margin:40px auto;padding:0 20px;color:CanvasText;background:Canvas}article{border:1px solid #8886;border-radius:10px;padding:18px;margin:14px 0}button,a{font:inherit;margin-right:12px}small{display:block;margin:8px 0;color:GrayText}button{padding:6px 12px}#error{color:#c33}
</style></head><body><a href="/">Back to wiki</a><h1>Agents</h1><p>Closing this page leaves running agents active.</p><p id="error" role="alert"></p><main></main><script type="module">
const root=document.querySelector('main'),error=document.querySelector('#error');
async function request(path,method='GET'){const r=await fetch('/.proxy/iugum/api/'+path,{method,headers:method==='POST'?{'Content-Type':'application/json'}:{},body:method==='POST'?'{}':undefined});if(!r.ok)throw Error('Request failed ('+r.status+')');return r.json()}
async function load(){root.replaceChildren();try{const agents=await request('agents');if(!agents.length)root.textContent='No agents configured.';
for(const agent of agents){const card=document.createElement('article'),title=document.createElement('strong'),state=document.createElement('small');title.textContent=agent.name;state.textContent='Checking…';card.append(title,state);root.append(card);
const prefix=agent.id==='default'?'':'agents/'+encodeURIComponent(agent.id)+'/';
request(prefix+'status').then(s=>{state.textContent='Available'+(s.metadata?.model?' · '+s.metadata.model:'')+(s.metadata?.open_mode==='true'?' · OPEN MODE':'')}).catch(()=>{state.textContent='Stopped or unavailable'});
if(agent.transport==='iugum')for(const action of ['start','stop']){const b=document.createElement('button');b.textContent=action==='start'?'Start agent':'Stop agent';b.onclick=async()=>{b.disabled=true;try{await request(prefix+action,'POST');await load()}catch(e){error.textContent=e.message}finally{b.disabled=false}};card.append(b)}
}}catch(e){error.textContent=e.message}}
load();
</script></body></html>`
