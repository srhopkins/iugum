const controlCSS = `
.iugum-icon-button {
  display:inline-flex; align-items:center; justify-content:center;
  min-width:var(--iugum-control-size,30px); min-height:var(--iugum-control-size,30px);
  cursor:pointer;
}
.iugum-icon-button svg { width:var(--iugum-icon-size,18px); height:var(--iugum-icon-size,18px); }
.iugum-icon-button[aria-expanded="true"], .iugum-icon-button[aria-pressed="true"] {
  color:var(--action-button-hover-color); background:var(--iugum-control-active,color-mix(in srgb,currentColor 10%,transparent));
}
button:focus-visible, input:focus-visible, select:focus-visible, textarea:focus-visible, [tabindex]:focus-visible {
  outline:2px solid var(--iugum-focus-color,var(--link-color,#286dc0)); outline-offset:2px;
}
`;

// Shared styling for the native host and shadow panel. Runtime geometry stays in components.
export const panelCSS = controlCSS + `.chief-chat-title {
  display:flex;
  align-items:center;
  gap:var(--iugum-space-8,8px);
  flex:1;
  min-width:0
}
.chief-title-input {
  min-width:0; width:100%; font:inherit; color:inherit;
  background:var(--root-background-color,Canvas);
}
.chief-chat-title span {
  overflow:hidden;
  text-overflow:ellipsis;
  white-space:nowrap
}
.chief-head>button {
  border:0;
  background:transparent
}
.chief-header-menus {
  flex-shrink:0;
  margin-right:0;
  display:flex;
  position:relative;
}
.chief-panel .chief-head .iugum-icon-button {
  border:0;
  background:transparent;
  color:var(--action-button-color)
}
.chief-panel .chief-head .iugum-icon-button:hover {
  color:var(--action-button-hover-color)
}
#iugum-chat-menu-popup {
  position:absolute;
  left:0;
  top:100%;
  width:250px;
  max-height:50vh;
  overflow:auto;
  background:var(--root-background-color,Canvas);
  border:1px solid #8885;
  border-radius:8px;
  padding:10px;
  z-index:30
}
#iugum-chat-menu-popup[hidden] {
  display:none
}
#iugum-chat-menu-popup button {
  display:block;
  width:100%;
  text-align:left;
  margin:var(--iugum-space-4,4px) 0
}
.chief-panel {
  position:relative
}
.chief-head {
  justify-content:flex-end
}
.chief-agent-drawer {
  position:absolute;
  right:0;
  top:0;
  bottom:0;
  width:85%;
  max-width:380px;
  z-index:20;
  padding:14px;
  overflow:auto;
  background:var(--root-background-color,Canvas);
  border-left:1px solid #8885;
  box-shadow:-8px 0 24px #0003;
  display:flex;
  flex-direction:column;
  gap:14px
}
.chief-agent-drawer[hidden] {
  display:none
}
.chief-agent-drawer>button {
  align-self:flex-end
}
.chief-agent-drawer input {
  width:100%;
  box-sizing:border-box;
  padding:9px;
  border:1px solid #8885;
  border-radius:6px;
  background:transparent;
  color:inherit;
  font:inherit
}
.chief-agent-drawer>div button {
  display:block;
  width:100%;
  text-align:left;
  margin:6px 0;
  padding:10px
}
.chief-agent-drawer button[aria-pressed=true] {
  background:color-mix(in srgb,currentColor 12%,transparent)
}
.chief-rail:hover button[aria-current=true]::before,.chief-rail:focus-within button[aria-current=true]::before {
  opacity:.3
}
.chief-rail button.tick-hover::before {
  opacity:1!important
}
.chief-history {
  position:relative;
  display:flex;
  flex:1;
  min-height:80px
}
.chief-history .chief-messages {
  padding-right:28px
}
.chief-rail {
  position:absolute;
  right:3px;
  top:4px;
  bottom:4px;
  width:18px;
  display:flex;
  flex-direction:column;
  justify-content:center;
  overflow:auto;
  gap:3px;
  z-index:4
}
.chief-panel .chief-rail button {
  flex:0 0 10px;
  min-height:10px;
  width:18px;
  padding:0;
  border:0;
  border-radius:0;
  background:transparent;
  position:relative
}
.chief-rail button[aria-current=true]::before {
  opacity:1
}
.chief-rail button::before {
  margin-left:auto;
  content:"";
  display:block;
  height:2px;
  background:currentColor;
  opacity:.3;
  width:var(--tick-width,8px)
}
.chief-turn-preview {
  display:-webkit-box;
  -webkit-line-clamp:5;
  -webkit-box-orient:vertical;
  line-height:1.4;
  position:absolute;
  right:26px;
  top:8px;
  max-width:280px;
  max-height:160px;
  overflow:hidden;
  padding:var(--iugum-space-12,12px);
  border:1px solid #8886;
  border-radius:10px;
  background:var(--root-background-color,Canvas);
  color:inherit;
  box-shadow:0 4px 18px #0005;
  z-index:5;
  white-space:pre-wrap;
  overflow-wrap:anywhere;
  pointer-events:none
}
.chief-turn-preview[hidden] {
  display:none
}
.chief-turn .chief-message.user::before {
  content:"";
  position:absolute;
  inset:0;
  border-radius:inherit;
  background:var(--iugum-chat-prompt-background,#e7f2ff);
  box-shadow:0 0 0 14px var(--root-background-color,Canvas);
  z-index:-1;
  pointer-events:none
}
.chief-message.user.truncated:not(.expanded) .chief-markdown {
  font-family:var(--iugum-reading-font,var(--ui-font,system-ui,sans-serif));
  mask-image:linear-gradient(to bottom,#000 55%,transparent 100%)
}
.chief-message.user.truncated {
  cursor:pointer
}
.chief-message.user:focus-visible {
  outline:2px solid var(--link-color,#286dc0);
  outline-offset:-2px
}
.chief-turn .chief-message.user:not(.expanded)::after {
  content:"";
  position:absolute;
  left:-1px;
  right:-1px;
  top:100%;
  height:24px;
  pointer-events:none;
  background:linear-gradient(to bottom,var(--root-background-color,Canvas),transparent)
}
.chief-resize {
  position:absolute;
  left:0;
  top:0;
  bottom:0;
  width:8px;
  z-index:10;
  cursor:col-resize;
  touch-action:none
}
.chief-resize:hover,.chief-resize:focus-visible {
  background:var(--link-color,#286dc0);
  opacity:.4
}
@media(max-width:650px) {
  .chief-resize {
  display:none
}

}
.chief-turn {
  position:relative;
  display:flow-root
}
.chief-turn .chief-message.user {
  border-radius:var(--iugum-chat-prompt-radius,6px);
  position:sticky;
  top:0;
  z-index:2;
  background:var(--iugum-chat-prompt-background,#e7f2ff);
  color:var(--iugum-chat-prompt-color,#17365b);
  border:var(--iugum-chat-prompt-border,1px solid color-mix(in srgb,currentColor 18%,transparent));
  box-shadow:0 2px 5px #0001
}
.chief-message.user .chief-markdown {
  font-family:var(--iugum-reading-font,var(--ui-font,system-ui,sans-serif));
  max-height:6em;
  overflow:hidden
}
.chief-message.user.expanded {
  position:relative;
  top:auto
}
.chief-message.user.expanded .chief-markdown {
  font-family:var(--iugum-reading-font,var(--ui-font,system-ui,sans-serif));
  max-height:none;
  overflow:visible
}
.chief-markdown {
  font-family:var(--iugum-reading-font,var(--ui-font,system-ui,sans-serif));
  line-height:var(--iugum-reading-leading,1.5);
  font-size:var(--iugum-reading-size,16px)
}
.chief-markdown p {
  margin:.45em 0 .8em
}
.chief-markdown>:first-child {
  margin-top:0
}
.chief-markdown h1,.chief-markdown h2,.chief-markdown h3 {
  font-size:1.1em;
  margin:1em 0 .4em
}
.chief-markdown ul,.chief-markdown ol {
  padding-left:1.5em;
  margin:.5em 0
}
.chief-markdown li {
  margin:.25em 0
}
.chief-markdown pre {
  overflow:auto;
  white-space:pre;
  padding:10px;
  background:color-mix(in srgb,currentColor 6%,transparent);
  border-radius:var(--iugum-chat-prompt-radius,6px);
  font-size:.9em
}
.chief-markdown code {
  font-family:var(--iugum-code-font,ui-monospace,monospace);
  background:color-mix(in srgb,currentColor 5%,transparent);
  border-radius:3px;
  padding:.1em .2em
}
.chief-markdown pre code {
  padding:0;
  background:none
}
.chief-markdown blockquote {
  margin:.7em 0;
  padding-left:var(--iugum-space-12,12px);
  border-left:3px solid color-mix(in srgb,currentColor 20%,transparent)
}
.chief-markdown table {
  display:block;
  overflow:auto;
  border-collapse:collapse;
  max-width:100%
}
.chief-markdown td,.chief-markdown th {
  border:1px solid color-mix(in srgb,currentColor 20%,transparent);
  padding:5px var(--iugum-space-8,8px)
}
.chief-markdown a {
  color:var(--link-color,#286dc0)
}
.chief-markdown details {
  margin:.6em 0
}
.chief-markdown summary {
  cursor:pointer;
  font-weight:500;
  opacity:.8
}
.chief-markdown details[open] {
  padding-bottom:6px
}
.chief-markdown img {
  max-width:100%
}
:host {
  position:relative;
  flex:var(--iugum-chat-panel-size,1)!important;
  height:100%;
  min-width:290px;
  border-left:1px solid color-mix(in srgb,currentColor 15%,transparent)
}
#panel-root {
  height:100%
}
@media(max-width:650px) {
  :host {
  position:fixed;
  inset:54px 0 0;
  z-index:900;
  width:100%;
  min-width:0
}

}
.chief-panel {
  font:var(--iugum-ui-size,14px) var(--ui-font,system-ui,sans-serif);
  color:inherit;
  height:100%;
  min-height:0;
  display:flex;
  flex-direction:column;
  background:var(--root-background-color,Canvas);
  box-sizing:border-box
}
.chief-panel * {
  box-sizing:border-box
}
.chief-panel button,.chief-panel textarea {
  font:inherit;
  color:inherit
}
.chief-panel button {
  cursor:pointer;
  border:1px solid color-mix(in srgb,currentColor 20%,transparent);
  border-radius:5px;
  background:transparent;
  padding:5px 9px
}
.chief-panel button:disabled {
  opacity:.5
}
.chief-head {
  display:flex;
  align-items:center;
  gap:var(--iugum-space-8,8px);
  padding:10px;
  border-bottom:1px solid color-mix(in srgb,currentColor 15%,transparent)
}
.chief-actions select {
  flex:1;
  min-width:0;
  max-width:100%;
  font:inherit;
  padding:5px;
  background:var(--root-background-color,Canvas);
  color:inherit;
  border:1px solid #8885;
  border-radius:5px
}
.chief-messages {
  overflow:auto;
  flex:1;
  min-height:80px;
  padding:0 var(--iugum-space-12,12px) var(--iugum-space-12,12px)
}
.chief-message {
  white-space:normal;
  overflow-wrap:anywhere;
  margin:0 0 18px
}
.chief-message.user {
  padding:9px;
  background:color-mix(in srgb,currentColor 6%,transparent);
  border-radius:var(--iugum-chat-prompt-radius,6px)
}
.chief-message small {
  display:block;
  opacity:.65;
  font-size:.75em;
  margin-bottom:5px
}
.chief-compose {
  padding:10px;
  border-top:1px solid color-mix(in srgb,currentColor 15%,transparent)
}
.chief-compose textarea {
  width:100%;
  min-height:74px;
  max-height:220px;
  resize:vertical;
  background:transparent;
  border:1px solid color-mix(in srgb,currentColor 25%,transparent);
  border-radius:5px;
  padding:var(--iugum-space-8,8px)
}
.chief-actions {
  display:flex;
  gap:var(--iugum-space-12,12px);
  justify-content:space-between;
  margin-top:6px
}
.chief-error {
  color:var(--error-color,#b34b38);
  white-space:pre-wrap;
  margin:0;
  padding:0 10px
}
.chief-pending {
  font-size:.85em;
  opacity:.7;
  padding:0 10px
}
.chief-reviews {
  max-height:35%;
  overflow:auto;
  padding:0 10px
}
.chief-reviews article {
  padding:10px 0;
  border-bottom:1px solid color-mix(in srgb,currentColor 15%,transparent)
}
.chief-reviews pre {
  white-space:pre-wrap;
  overflow-wrap:anywhere;
  font:inherit;
  font-size:.85em
}
.chief-reviews button {
  margin:5px 6px 0 0
}
`;

export const hostCSS = controlCSS + `
:root {
  --iugum-reading-font:var(--ui-font,system-ui,sans-serif);
  --iugum-reading-size:16px;
  --iugum-ui-size:14px;
  --iugum-reading-leading:1.5;
  --iugum-code-font:ui-monospace,monospace;
  --iugum-space-4:4px; --iugum-space-8:8px; --iugum-space-12:12px; --iugum-space-16:16px;
}
#sb-main .cm-editor {
  --editor-font:var(--iugum-editor-font,var(--iugum-reading-font));
  font-size:var(--iugum-reading-size);
}
#iugum-chief-results .chief-search-markdown {
  font-family:var(--iugum-reading-font,var(--ui-font,system-ui,sans-serif));
  line-height:var(--iugum-reading-leading,1.5);
  font-size:var(--iugum-reading-size,16px);
  overflow-wrap:anywhere
}
#iugum-chief-results .chief-search-markdown p {
  white-space:normal;
  font:inherit
}
#iugum-chief-results .chief-search-markdown pre {
  white-space:pre;
  overflow:auto;
  font:0.9em var(--iugum-code-font,ui-monospace,monospace);
  padding:var(--iugum-space-8,8px);
  background:#8881
}
#iugum-chief-results .chief-search-markdown table {
  display:block;
  overflow:auto;
  border-collapse:collapse
}
#iugum-chief-results .chief-search-markdown td,#iugum-chief-results .chief-search-markdown th {
  padding:var(--iugum-space-4,4px);
  border:1px solid #8885
}
#iugum-chief-results .chief-search-markdown code {
  font-family:var(--iugum-code-font,ui-monospace,monospace)
}
#iugum-chief-results .chief-search-preview {
  max-height:100px;
  overflow:hidden;
  margin:var(--iugum-space-8,8px) 0
}
#iugum-chief-results .chief-search-markdown img {
  max-width:100%
}
#iugum-chief-tools {
  display:flex;
  align-items:center;
  gap:6px;
  position:relative;
  flex-shrink:0;
  box-sizing:border-box;
  width:100%;
  min-width:0;
  margin:0;
  padding:6px var(--iugum-space-12,12px);
  font:var(--iugum-ui-size,14px) var(--ui-font,system-ui,sans-serif);
  border-bottom:1px solid color-mix(in srgb,currentColor 12%,transparent);
  z-index:30
}
#iugum-chief-tools input,#iugum-chief-tools button {
  font:inherit;
  color:inherit;
  background:transparent;
  border:1px solid color-mix(in srgb,currentColor 20%,transparent);
  border-radius:4px;
  padding:var(--iugum-space-4,4px) 6px
}
#iugum-chief-tools input[type=search] {
  min-width:60px;
  width:100%;
  flex:1
}
#iugum-chief-tools .chief-scope {
  width:95px;
  min-width:50px
}
#iugum-chief-results {
  position:absolute;
  top:calc(100% + 8px);
  right:0;
  width:min(620px,90vw);
  max-height:70vh;
  overflow:auto;
  background:var(--root-background-color,Canvas);
  color:var(--root-color,CanvasText);
  padding:14px;
  border:1px solid color-mix(in srgb,currentColor 25%,transparent);
  border-radius:6px;
  box-shadow:0 5px 20px #0002;
  z-index:1000
}
#iugum-chief-results[hidden] {
  display:none
}
#iugum-chief-results article {
  padding:10px 0;
  border-bottom:1px solid color-mix(in srgb,currentColor 15%,transparent)
}
#iugum-chief-results p,#iugum-chief-results pre {
  white-space:pre-wrap;
  overflow-wrap:anywhere;
  font:inherit;
  font-size:.9em;
  margin:6px 0
}
#iugum-chief-results small {
  opacity:.7
}
#iugum-chief-results summary {
  cursor:pointer;
  font-size:.85em
}
@media(max-width:650px) {
  #iugum-chief-tools {
  padding:5px 6px;
  gap:var(--iugum-space-4,4px)
}
#iugum-chief-tools .chief-scope {
  width:70px
}

}
#sb-top:has(#iugum-chief-tools) {
  z-index:40
}
#sb-top:has(#iugum-chief-tools) .main {
  overflow:visible;
  scrollbar-gutter:auto
}
#sb-top:has(#iugum-chief-tools) .inner {
  max-width:none;
  width:100%
}
#sb-top:has(#iugum-chief-tools) .wrapper {
  align-items:center;
  gap:var(--iugum-space-12,12px)
}
#sb-top:has(#iugum-chief-tools) #sb-current-page {
  flex:1 1 0;
  min-width:70px
}
#sb-top #iugum-chief-tools {
  flex:0 1 560px;
  width:50%;
  padding:var(--iugum-space-4,4px) 6px;
  margin:0 auto;
  border:1px solid color-mix(in srgb,currentColor 20%,transparent);
  border-radius:9px;
  background:var(--root-background-color,Canvas);
  z-index:50
}
#sb-top #iugum-chief-tools:focus-within,#sb-top #iugum-chief-tools:has(#iugum-chief-results:not([hidden])) {
  box-shadow:0 2px 10px #0002;
  border-color:var(--link-color,#286dc0)
}
#sb-top #iugum-chief-tools input[type=search] {
  border:0;
  outline:none
}
#sb-top #iugum-chief-results {
  left:-1px;
  right:auto;
  top:calc(100% + 1px);
  width:calc(100% + 2px);
  box-sizing:border-box;
  border-radius:0 0 10px 10px;
  max-height:70vh;
  padding:10px;
  background:var(--root-background-color,Canvas)
}
#sb-top #iugum-chief-results article {
  padding:14px;
  margin:var(--iugum-space-12,12px) 0;
  border:1px solid color-mix(in srgb,currentColor 20%,transparent);
  border-radius:8px;
  background:color-mix(in srgb,currentColor 3%,var(--root-background-color,Canvas))
}
#sb-top #iugum-chief-results article>button {
  display:block;
  font-weight:600;
  color:var(--link-color,#286dc0);
  margin-bottom:6px
}
#sb-top #iugum-chief-results article>small {
  display:block;
  margin-bottom:var(--iugum-space-8,8px)
}
@media(max-width:800px) {
  #sb-top #iugum-chief-tools {
  flex:1 1 250px;
  min-width:210px
}
#sb-top:has(#iugum-chief-tools) .wrapper {
  padding:0 var(--iugum-space-8,8px);
  gap:5px
}
#sb-top #iugum-chief-tools .chief-scope {
  width:55px
}
#sb-top:has(#iugum-chief-tools) #sb-current-page {
  font-size:18px;
  min-width:40px
}

}
#sb-top:has(#iugum-chief-tools)>.panel {
  display:none
}
#sb-top:has(#iugum-chief-tools) .wrapper {
  min-height:39px
}
#sb-top:has(#iugum-chief-tools) #sb-current-page {
  max-width:calc(50% - 290px)
}
#sb-top:has(#iugum-chief-tools) .sb-actions {
  margin-left:auto
}
#sb-top #iugum-chief-tools {
  position:absolute;
  left:50%;
  top:50%;
  transform:translate(-50%,-50%);
  width:min(560px,48vw)
}
@media(max-width:800px) {
  #sb-top:has(#iugum-chief-tools) #sb-current-page {
  max-width:20%
}
#sb-top #iugum-chief-tools {
  width:52vw;
  min-width:210px
}

}
#sb-top #iugum-chief-tools:has(#iugum-chief-results:not([hidden])) {
  border-bottom-left-radius:0;
  border-bottom-right-radius:0
}
#iugum-search-options {
  position:absolute;
  top:calc(100% + 2px);
  left:0;
  right:0;
  padding:var(--iugum-space-16,16px);
  background:var(--root-background-color,Canvas);
  border:1px solid #8885;
  border-radius:0 0 18px 18px;
  box-shadow:0 8px 20px #0003
}
#iugum-search-options[hidden] {
  display:none
}
#iugum-search-options label {
  display:block;
  margin-bottom:var(--iugum-space-8,8px)
}
#iugum-search-options .chief-scope {
  width:100%
}
#iugum-search-options p {
  font-size:12px;
  opacity:.7
}
#sb-top #iugum-chief-tools,#sb-top #iugum-chief-tools.search-expanded {
  border-radius:9px
}
#sb-top #iugum-chief-tools button {
  border:var(--iugum-search-icon-border,0);
  background:var(--iugum-search-icon-background,transparent);
  color:var(--action-button-color);
  border-radius:4px
}
#sb-top #iugum-chief-tools button:hover {
  color:var(--action-button-hover-color)
}
#iugum-chat-menu {
  display:flex;
  position:relative
}
#iugum-chat-menu[hidden] {
  display:none
}
#iugum-chat-menu-popup {
  position:absolute;
  right:0;
  top:100%;
  min-width:220px;
  max-width:350px;
  max-height:60vh;
  overflow:auto;
  background:var(--root-background-color,Canvas);
  color:var(--root-color,CanvasText);
  border:1px solid #8885;
  border-radius:8px;
  padding:10px;
  z-index:1000;
  box-shadow:0 5px 18px #0004
}
#iugum-chat-menu-popup[hidden] {
  display:none
}
#iugum-chat-menu-popup button {
  display:block;
  width:100%;
  height:auto;
  text-align:left;
  margin:var(--iugum-space-4,4px) 0;
  white-space:normal;
  overflow-wrap:anywhere
}
#iugum-search-sources {
  display:none;
  position:absolute;
  left:-1px;
  right:-1px;
  top:100%;
  height:32px;
  align-items:center;
  justify-content:flex-end;
  gap:10px;
  padding:0 var(--iugum-space-12,12px);
  background:var(--root-background-color,Canvas);
  border:1px solid #8885;
  border-top:0;
  border-radius:0 0 9px 9px
}
#iugum-chief-tools.search-expanded #iugum-search-sources {
  display:flex
}
#sb-top #iugum-search-sources button {
  font-size:12px;
  padding:2px;
  color:var(--action-button-color)
}
#sb-top #iugum-search-sources button[aria-pressed=true] {
  color:var(--link-color,#286dc0);
  text-decoration:underline;
  text-underline-offset:4px
}
#sb-top #iugum-chief-tools.search-expanded {
  border-bottom-left-radius:0;
  border-bottom-right-radius:0
}
#sb-top #iugum-chief-results,#iugum-search-options {
  top:calc(100% + 32px)
}
#sb-top #iugum-chief-results {
  padding-top:18px;
  border-top:1px solid color-mix(in srgb,currentColor 15%,transparent)
}
`;
