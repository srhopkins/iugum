let dispatchToHost = () => {
  throw new Error("Not initialized yet");
};

const isWorker = typeof window > "u" &&
  typeof globalThis.WebSocketPair > "u";

const pendingSyscalls = new Map();
let syscallReqId = 0;

if (isWorker) {
  globalThis.syscall = async (name, ...args) => {
    return await new Promise((resolve, reject) => {
      syscallReqId++;
      pendingSyscalls.set(syscallReqId, { resolve, reject });
      dispatchToHost({ type: "sys", id: syscallReqId, name, args });
    });
  };
}

function wireWorker(functionMapping, manifest, postMessage) {
  if (!isWorker) return;
  dispatchToHost = postMessage;
  self.addEventListener("message", (event) => {
    (async () => {
      const data = event.data;
      switch (data.type) {
        case "inv": {
          const fn = functionMapping[data.name];
          if (!fn) throw new Error(`Function not loaded: ${data.name}`);
          try {
            const result = await Promise.resolve(fn(...(data.args || [])));
            dispatchToHost({ type: "invr", id: data.id, result });
          } catch (e) {
            console.error(
              "agent-chat: function threw",
              data.name,
              "error:",
              e.message,
            );
            dispatchToHost({ type: "invr", id: data.id, error: e.message });
          }
          break;
        }
        case "sysr": {
          const waiter = pendingSyscalls.get(data.id);
          if (!waiter) throw new Error("Invalid request id");
          pendingSyscalls.delete(data.id);
          if (data.error) waiter.reject(new Error(data.error));
          else waiter.resolve(data.result);
          break;
        }
      }
    })().catch(console.error);
  });
  dispatchToHost({ type: "manifest", manifest });
}

function syscall(name, ...args) {
  return globalThis.syscall(name, ...args);
}

// The plug loads by file discovery, avoiding dependence on indexed Space Lua.
async function mountChat(force) {
 for(let attempt=0;attempt<60;attempt++){
 const ready=await syscall("lua.evalExpression", `(function()
 if editor == nil then return false end
 local host=editor
 local ui = js.import("/.proxy/iugum/assets/agent-chat.js")
 ui.mount(function(node) return host.showPanel("rhs", 1, node) end,
 function() return host.hidePanel("rhs") end,
 function(page) return host.navigate(page) end, "chat")
 ${force ? "ui.open()" : ""}
 return true
 end)()`);
 if(ready)return;
 await new Promise(resolve=>setTimeout(resolve,100));
 }
 throw Error("Chat is waiting for wiki APIs; use Chat: Open to retry");
}
async function openChat(){return mountChat(true);}
async function restoreChat() {
 setTimeout(()=>mountChat(false).catch(e=>console.debug("Chat unavailable on this wiki origin",e.message)),100);
}
const manifest={name:"iugum-agent-chat",version:1,functions:{
 openChat:{path:"./chief.js:openChat",command:{name:"Chat: Open"}},
 restoreChat:{path:"./chief.js:restoreChat",events:["system:ready","plugs:loaded","editor:pageLoaded","editor:pageReloaded"]}
}};
const functionMapping={openChat,restoreChat};
wireWorker(functionMapping,manifest,self.postMessage);
const plugExport={manifest,functionMapping};
export {plugExport as plug};
