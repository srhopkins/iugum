import assert from 'node:assert/strict';
import {wav16,SpeechGate,RecognitionLane} from './web/core.mjs';
const base=process.argv[2]||'http://127.0.0.1:3862/';
const post=(path,body,type='application/json',signal)=>fetch(new URL('api/'+path,base),{method:'POST',headers:{'Content-Type':type,'X-Voice-Lab':'1'},body:type==='application/json'?JSON.stringify(body):body,signal});
const checked=async r=>{if(!r.ok)throw Error(await r.text());return r};
const phrase='The blue notebook contains seven pages. Please check the calendar for tomorrow morning. The local voice connection is working.';
let t=performance.now(),first,parts=[],length=0,chunks=0;
const speech=await checked(await post('speak',{text:phrase})),reader=speech.body.getReader();
while(true){const r=await reader.read();if(r.done)break;if(first===undefined)first=performance.now()-t;parts.push(r.value);length+=r.value.length;chunks++}
const complete=performance.now()-t,bytes=new Uint8Array(length);let offset=0;for(const p of parts){bytes.set(p,offset);offset+=p.length}
const pcm=new Float32Array(bytes.length/2),v=new DataView(bytes.buffer);for(let i=0;i<pcm.length;i++)pcm[i]=v.getInt16(i*2,true)/32768;
const audio=new Float32Array(Math.floor(pcm.length*2/3));for(let i=0;i<audio.length;i++)audio[i]=pcm[Math.floor(i*1.5)];
const stt=async a=>(await checked(await post('transcribe',wav16(a),'audio/wav'))).json();
const partial=await stt(audio.slice(0,Math.min(audio.length,3*16000))),full=await stt(audio);
assert(full.text.toLowerCase().includes('blue notebook'));assert(full.text.toLowerCase().includes('calendar'));assert(full.text.toLowerCase().includes('working'));
console.log(JSON.stringify({check:'speech_roundtrip',durationSeconds:audio.length/16000,firstAudioMs:Math.round(first),completeAudioMs:Math.round(complete),streamChunks:chunks,earlyAudioSavedMs:Math.round(complete-first),partialText:partial.text,partialSTTMs:partial.milliseconds,finalText:full.text,finalSTTMs:full.milliseconds}));
// Replay generated PCM at real-time pace through the actual utterance gate and
// single recognition lane. No browser or microphone is involved.
let finalResolve,finalReject;const finalPromise=new Promise((resolve,reject)=>{finalResolve=resolve;finalReject=reject});let maxConcurrent=0,current=0,partials=0,firstPartialAt;
const lane=new RecognitionLane(async a=>{current++;maxConcurrent=Math.max(maxConcurrent,current);try{return await stt(a)}finally{current--}});
const began=performance.now();
const gate=new SpeechGate({threshold:.009,onPartial(a){lane.partial(a,result=>{partials++;if(firstPartialAt===undefined)firstPartialAt=performance.now()-began})},async onEnd(a){try{finalResolve(await lane.final(a))}catch(e){finalReject(e)}}});
for(let i=0;i<audio.length;i+=1600){gate.feed(audio.slice(i,i+1600));await new Promise(r=>setTimeout(r,100))}
for(let i=0;i<10;i++){gate.feed(new Float32Array(1600));await new Promise(r=>setTimeout(r,100))}
const final=await Promise.race([finalPromise,new Promise((_,reject)=>setTimeout(()=>reject(Error('no final transcript')),15000))]);assert(maxConcurrent===1);assert(partials>0);assert(final.text.trim());
console.log(JSON.stringify({check:'generated_realtime_gate',partials,firstPartialMs:Math.round(firstPartialAt),maxConcurrentSTT:maxConcurrent,finalText:final.text}));
const startModel=performance.now(),response=await checked(await post('respond',{text:'Say exactly: The voice loop is ready.',history:[]}));let buffer='',answer='',modelFirst;
const decoder=new TextDecoder();for await(const chunk of response.body){buffer+=decoder.decode(chunk,{stream:true});let i;while((i=buffer.indexOf('\n'))>=0){const line=buffer.slice(0,i);buffer=buffer.slice(i+1);if(!line)continue;const event=JSON.parse(line);if(event.error)throw Error(event.error);if(event.text){modelFirst??=performance.now()-startModel;answer+=event.text}}}
assert(answer.trim());console.log(JSON.stringify({check:'local_model',firstWordsMs:Math.round(modelFirst),completeMs:Math.round(performance.now()-startModel),reply:answer}));
