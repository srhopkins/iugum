export function wav16(samples, rate=16000) {
 const data=new ArrayBuffer(44+samples.length*2),v=new DataView(data);
 const put=(i,s)=>[...s].forEach((c,n)=>v.setUint8(i+n,c.charCodeAt(0)));
 put(0,'RIFF');v.setUint32(4,data.byteLength-8,true);put(8,'WAVEfmt ');v.setUint32(16,16,true);v.setUint16(20,1,true);v.setUint16(22,1,true);v.setUint32(24,rate,true);v.setUint32(28,rate*2,true);v.setUint16(32,2,true);v.setUint16(34,16,true);put(36,'data');v.setUint32(40,samples.length*2,true);
 for(let i=0;i<samples.length;i++)v.setInt16(44+2*i,Math.max(-1,Math.min(1,samples[i]))*32767,true);
 return data;
}
export function join(parts){const a=new Float32Array(parts.reduce((n,p)=>n+p.length,0));let i=0;for(const p of parts){a.set(p,i);i+=p.length}return a}
export function rms(a){return Math.sqrt(a.reduce((s,x)=>s+x*x,0)/Math.max(1,a.length))}

// One active recognition call. Intermediate snapshots are skipped, never queued.
export class RecognitionLane {
 constructor(request){this.request=request;this.active=null;this.controller=null;this.generation=0}
 partial(audio,onResult){if(this.active)return false;const generation=this.generation;this.run(audio).then(result=>{if(generation===this.generation)onResult(result)}).catch(()=>{});return true}
 async run(audio){const c=new AbortController();this.controller=c;const p=this.request(audio,c.signal);this.active=p;try{return await p}finally{if(this.active===p){this.active=null;this.controller=null}}}
 async final(audio){const generation=this.generation;if(this.active){try{await this.active}catch{}}if(generation!==this.generation)throw new DOMException('Interrupted','AbortError');return this.run(audio)}
 cancel(){this.generation++;this.controller?.abort()}
}

// A bounded utterance gate. Recognition is independent of end-of-speech timing.
export class SpeechGate {
 constructor({threshold=.018,silence=.7,maxSeconds=30,onStart=()=>{},onPartial=()=>{},onEnd=()=>{}}={}){Object.assign(this,{threshold,silence,maxSeconds,onStart,onPartial,onEnd});this.reset()}
 reset(){this.parts=[];this.pre=[];this.active=false;this.time=0;this.lastVoice=0;this.lastPartial=0;this.onset=0;this.cooldown=false}
 feed(a){const dt=a.length/16000;this.time+=dt;const voice=rms(a)>=this.threshold;
  if(this.cooldown){if(voice)this.lastVoice=this.time;if(this.time-this.lastVoice>=this.silence)this.cooldown=false;return}
  if(!this.active){this.pre.push(a);while(this.pre.reduce((n,p)=>n+p.length,0)>6400)this.pre.shift();this.onset=voice?this.onset+dt:0;if(this.onset<.12)return;this.active=true;this.parts=this.pre.slice();this.pre=[];this.lastVoice=this.time;this.began=this.time;this.lastPartial=this.time;this.onStart();return}
  this.parts.push(a);if(voice)this.lastVoice=this.time;
  if(this.time-this.lastVoice>=this.silence||this.time-this.began>=this.maxSeconds-.5){const data=join(this.parts);this.cooldown=this.time-this.lastVoice<this.silence;this.active=false;this.parts=[];this.pre=[];this.onset=0;this.onEnd(data,this.time-this.lastVoice);return}
  if(this.time-this.lastPartial>=1.2){this.lastPartial=this.time;this.onPartial(join(this.parts))}
 }
}

// Break responses at natural boundaries, with a small first phrase for early audio.
export class PhraseBuffer {
 constructor(){this.text=''}
 push(token,done=false){this.text+=token;const out=[];let m;
  while((m=this.text.match(/^([\s\S]*?[.!?](?:\s|$))/))){out.push(m[1].trim());this.text=this.text.slice(m[1].length)}
  if(done&&this.text.trim()){out.push(this.text.trim());this.text=''}
  return out.filter(Boolean)
 }
}

// Scheduled playback is bounded; interruption aborts fetch and all queued nodes.
export class PCMPlayer {
 constructor(context,onMetric=()=>{}){this.context=context;this.onMetric=onMetric;this.nodes=new Set();this.epoch=0;this.end=0;this.controller=null;this.firstPlayed=null}
 stop(){this.epoch++;this.controller?.abort();this.controller=null;for(const n of this.nodes){try{n.stop()}catch{}}this.nodes.clear();this.end=this.context.currentTime;this.firstPlayed=null}
 get buffered(){return Math.max(0,this.end-this.context.currentTime)}
 async drain(signal){const epoch=this.epoch;while(this.buffered>0){if(epoch!==this.epoch||signal?.aborted)throw new DOMException('Interrupted','AbortError');await pause(25,signal)}}
 async play(text,signal,onFirst=()=>{}){
  const epoch=this.epoch,c=new AbortController();this.controller=c;const abort=()=>c.abort();signal?.addEventListener('abort',abort,{once:true});if(signal?.aborted)c.abort();const start=performance.now();let first=true,carry=new Uint8Array(0),bytes=0;
  try{const r=await fetch('api/speak',{method:'POST',headers:{'Content-Type':'application/json','X-Voice-Lab':'1'},body:JSON.stringify({text}),signal:c.signal});if(!r.ok)throw Error('Speech playback request failed');if(r.headers.get('X-Audio-Sample-Rate')!=='24000')throw Error('Unsupported speech sample rate');const reader=r.body.getReader();
   while(true){if(epoch!==this.epoch||c.signal.aborted)throw new DOMException('Interrupted','AbortError');const result=await reader.read();if(epoch!==this.epoch||c.signal.aborted)throw new DOMException('Interrupted','AbortError');if(result.done)break;bytes+=result.value.length;if(bytes>8*1024*1024)throw Error('Speech response too large');const combined=new Uint8Array(carry.length+result.value.length);combined.set(carry);combined.set(result.value,carry.length);const count=Math.floor(combined.length/2);carry=combined.slice(count*2);if(!count)continue;const audio=this.context.createBuffer(1,count,24000),v=new DataView(combined.buffer);const out=audio.getChannelData(0);for(let i=0;i<count;i++)out[i]=v.getInt16(i*2,true)/32768;
    while(this.buffered>1.0){await pause(25,c.signal);if(epoch!==this.epoch||c.signal.aborted)throw new DOMException('Interrupted','AbortError')}
    const source=this.context.createBufferSource();source.buffer=audio;source.connect(this.context.destination);this.nodes.add(source);source.onended=()=>{this.nodes.delete(source);source.disconnect()};const when=Math.max(this.context.currentTime+.025,this.end);source.start(when);this.end=when+audio.duration;
    this.onMetric('bufferMs',Math.round(this.buffered*1000));if(first){first=false;this.onMetric('ttsMs',Math.round(performance.now()-start));onFirst(when-this.context.currentTime)}
   }
   if(carry.length)throw Error('Incomplete PCM sample');
  }finally{signal?.removeEventListener('abort',abort);if(this.controller===c)this.controller=null}
 }
}
function pause(ms,signal){return new Promise((resolve,reject)=>{const abort=()=>{clearTimeout(timer);signal?.removeEventListener('abort',abort);reject(new DOMException('Interrupted','AbortError'))};const timer=setTimeout(()=>{signal?.removeEventListener('abort',abort);resolve()},ms);signal?.addEventListener('abort',abort,{once:true});if(signal?.aborted)abort()})}
