// One active native preparation and the latest waiting transcript. No answer or
// tool proposal is returned to this browser; only an opaque preparation ID.
export class DraftLane {
 constructor(hold,prepare,onState=()=>{}){Object.assign(this,{hold,prepare,onState});this.revision=0;this.latest='';this.active=null;this.ready=null;this.closed=false;this.controller=null}
 update(text){if(this.closed)return;text=text.trim();if(!text||text===this.latest)return;this.latest=text;this.ready=null;if(!this.active)this.launch(text)}
 launch(text){const controller=new AbortController();this.controller=controller;const revision=++this.revision;this.onState('Preparing a private native-agent draft…');const task=this.prepare({operation:'prepare',hold_id:this.hold,revision,text},controller.signal);this.active=task;
  task.then(value=>{if(!this.closed&&this.latest===text){this.ready={text,id:value.draft_id};this.onState('Private draft ready; still waiting for release.')}}).catch(()=>{if(!this.closed)this.onState('No reusable draft yet; final send remains available.')}).finally(()=>{if(this.active===task){this.active=null;this.controller=null;if(!this.closed&&this.latest!==text)this.launch(this.latest)}})
 }
 take(finalText){this.closed=true;this.controller?.abort();const ready=this.ready;this.ready=null;return ready?.text===finalText.trim()?ready.id:''}
 cancel(){this.closed=true;this.controller?.abort();this.latest='';this.ready=null}
}
