import { actionMenu } from "./ui-controls.js";
import { panelCSS, hostCSS } from "./ui-styles.js";
// Native SilverBullet companion panel; sanitized Markdown is rendered by the server.
const API = "/.proxy/iugum/api";
let current;
export function searchCards(data) {
	const field = (x, k) => x[k] ?? x[k.toLowerCase()] ?? "";
	const cards = [];
	for (const t of data.transcripts || [])
		cards.push({
			title:
				String(field(t, "Project")).split("/").filter(Boolean).pop() ||
				field(t, "Session") ||
				"Transcript",
			source: [field(t, "Platform"), field(t, "Account"), field(t, "Timestamp")]
				.filter(Boolean)
				.join(" · "),
			text: field(t, "Text"),
            html: t.html,
			provenance:
				field(t, "Path") +
				":" +
				field(t, "Line") +
				"\nSession: " +
				field(t, "Session"),
		});
	for (const w of data.wiki || [])
		cards.push({
			title: w.path,
			source: "Wiki",
			text: w.text,
            html: w.html,
			provenance: w.path,
			page: w.path.replace(/\.md$/, ""),
		});
	for (const m of data.memory || [])
		cards.push({
			title: field(m, "Key"),
			source: "Memory · " + field(m, "NS"),
			text: field(m, "Value"),
            html: m.html,
			provenance: field(m, "NS") + "/" + field(m, "Key"),
		});
	for (const t of data.tasks || [])
		cards.push({
			title: t.title || t.id,
			source: [t.id, t.status, t.checked_at ? "Checked " + t.checked_at : ""]
				.filter(Boolean)
				.join(" · "),
			text: t.description,
            html: t.html,
			provenance:
				t.repo + "\nDependencies: " + JSON.stringify(t.dependencies || []),
		});
	return cards;
}
export function sendsOnEnter(event) {
	return event.key === "Enter" && !event.shiftKey && !event.isComposing;
}
function node(tag, text, cls) {
	const n = document.createElement(tag);
	if (text !== undefined) n.textContent = text;
	if (cls) n.className = cls;
	return n;
}

// Provider transcripts do not pass through agentdesk's Goldmark renderer. This
// fallback deliberately creates DOM nodes instead of accepting provider HTML.
// It covers the Markdown that agents normally return while keeping untrusted
// markup inert.
function markdownLink(url) {
	try {
		const parsed = new URL(url, window.location.href);
		return ["http:", "https:", "mailto:"].includes(parsed.protocol) ? parsed.href : "";
	} catch { return ""; }
}
function appendInline(target, source) {
	const token = /(\`[^`]*\`|\*\*[^*]+\*\*|__[^_]+__|\[[^\]]+\]\([^\s)]+\)|\*[^*]+\*|_[^_]+_)/g;
	let cursor = 0;
	for (const match of String(source || "").matchAll(token)) {
		target.append(document.createTextNode(source.slice(cursor, match.index)));
		const value = match[0];
		if (value.startsWith("`")) target.append(node("code", value.slice(1, -1)));
		else if (value.startsWith("**") || value.startsWith("__")) {
			const strong = node("strong"); appendInline(strong, value.slice(2, -2)); target.append(strong);
		} else if (value.startsWith("[")) {
			const close = value.indexOf("]("); const href = markdownLink(value.slice(close + 2, -1));
			if (!href) target.append(document.createTextNode(value));
			else { const link = node("a"); link.href = href; link.rel = "noopener noreferrer"; appendInline(link, value.slice(1, close)); target.append(link); }
		} else { const em = node("em"); appendInline(em, value.slice(1, -1)); target.append(em); }
		cursor = match.index + value.length;
	}
	target.append(document.createTextNode(source.slice(cursor)));
}
function markdownCell(text) { const cell = node("td"); appendInline(cell, text.trim()); return cell; }
function isTableRule(line) { return /^\s*\|?\s*:?-{3,}:?(?:\s*\|\s*:?-{3,}:?)+\s*\|?\s*$/.test(line); }
function tableCells(line) { return line.trim().replace(/^\||\|$/g, "").split("|"); }
function renderMarkdownFallback(text) {
	const root = document.createDocumentFragment(), lines = String(text || "").replace(/\r\n?/g, "\n").split("\n");
	for (let i = 0; i < lines.length;) {
		const line = lines[i];
		if (!line.trim()) { i++; continue; }
		if (/^```/.test(line)) {
			const code = [], language = line.slice(3).trim(); i++;
			while (i < lines.length && !/^```/.test(lines[i])) code.push(lines[i++]);
			if (i < lines.length) i++;
			const pre = node("pre"), block = node("code", code.join("\n")); if (language) block.dataset.language = language; pre.append(block); root.append(pre); continue;
		}
		const heading = line.match(/^(#{1,3})\s+(.+)$/);
		if (heading) { const el = node("h" + heading[1].length); appendInline(el, heading[2]); root.append(el); i++; continue; }
		if (i + 1 < lines.length && isTableRule(lines[i + 1])) {
			const table = node("table"), head = node("thead"), row = node("tr");
			for (const value of tableCells(line)) { const cell = node("th"); appendInline(cell, value.trim()); row.append(cell); }
			head.append(row); table.append(head); i += 2; const body = node("tbody");
			while (i < lines.length && /\|/.test(lines[i]) && lines[i].trim()) { const bodyRow = node("tr"); for (const value of tableCells(lines[i++])) bodyRow.append(markdownCell(value)); body.append(bodyRow); }
			table.append(body); root.append(table); continue;
		}
		const list = line.match(/^\s*([-*+] |\d+\. )(.+)$/);
		if (list) {
			const ordered = /^\s*\d+\. /.test(line), element = node(ordered ? "ol" : "ul");
			while (i < lines.length) { const item = lines[i].match(ordered ? /^\s*\d+\. (.+)$/ : /^\s*[-*+] (.+)$/); if (!item) break; const li = node("li"); appendInline(li, item[1]); element.append(li); i++; }
			root.append(element); continue;
		}
		if (/^>\s?/.test(line)) { const quote = node("blockquote"), paragraph = node("p"); while (i < lines.length && /^>\s?/.test(lines[i])) { if (paragraph.childNodes.length) paragraph.append(" "); appendInline(paragraph, lines[i++].replace(/^>\s?/, "")); } quote.append(paragraph); root.append(quote); continue; }
		const paragraph = node("p");
		while (i < lines.length && lines[i].trim() && !/^```|^(#{1,3})\s+|^\s*([-*+] |\d+\. )|^>\s?/.test(lines[i]) && !(i + 1 < lines.length && isTableRule(lines[i + 1]))) { if (paragraph.childNodes.length) paragraph.append(" "); appendInline(paragraph, lines[i++]); }
		root.append(paragraph);
	}
	return root;
}
async function api(path, options) {
	const r = await fetch(API + path, options);
	const data = await r.json();
	if (!r.ok) throw Error(data.error || "Request failed");
	return data;
}
const post = (path, data) =>
	api(path, {
		method: "POST",
		headers: { "Content-Type": "application/json" },
		body: JSON.stringify(data),
	});
export async function mount(showPanel, hidePanel, navigate, feature = "both") {
	if (current) {
		current.showPanel = showPanel;
		current.hidePanel = hidePanel;
		current.navigate = navigate;
		await current.enable(feature);
		current.attach();
		if (current.chatEnabled && current.open) await showPanel(current.panel);
		return current;
	}
	const s = { showPanel, hidePanel, navigate, open: false, busy: false, chatEnabled: false, searchEnabled: false };
	current = s;
	const panel = node("section", undefined, "chief-panel");
	s.panel = panel;
	panel.setAttribute("aria-label", "Agent conversation");
	panel.append(node("style", panelCSS));
    const resize = node("div", undefined, "chief-resize");
    resize.setAttribute("role", "separator");
    resize.setAttribute("aria-label", "Resize chat");
    resize.setAttribute("aria-orientation", "vertical");
    resize.setAttribute("aria-valuenow", "0");
    resize.setAttribute("aria-valuemin", "0");
    resize.setAttribute("aria-valuemax", "100");
    resize.tabIndex = 0;
    resize.title = "Drag to resize chat; arrow keys adjust width";
    const setWidth = width => {
        const bounded = Math.round(Math.max(290, Math.min(innerWidth - 240, width)));
        document.documentElement.style.setProperty("--iugum-chat-panel-size", "0 0 " + bounded + "px");
        resize.setAttribute("aria-valuenow", String(Math.round(bounded / innerWidth * 100)));
        resize.setAttribute("aria-valuetext", bounded + " pixels wide");
    };
    resize.onpointerdown = event => {
        if (event.button !== 0) return;
        event.preventDefault();
        resize.setPointerCapture(event.pointerId);
    };
    resize.onpointermove = event => {
        if (resize.hasPointerCapture(event.pointerId)) setWidth(innerWidth - event.clientX);
    };
    resize.onpointerup = event => {
        if (resize.hasPointerCapture(event.pointerId)) resize.releasePointerCapture(event.pointerId);
    };
    resize.onkeydown = event => {
        if (!["ArrowLeft", "ArrowRight"].includes(event.key)) return;
        event.preventDefault();event.stopPropagation();
        setWidth(panel.getBoundingClientRect().width + (event.key === "ArrowLeft" ? 20 : -20));
    };
    panel.append(resize);

	const head = node("div", undefined, "chief-head"),
		name = node("select"),
		work = node("button", "Commitments"),
		close = node("button", "×");
	work.type = close.type = "button";
	close.setAttribute("aria-label", "Close chat");
	work.onclick = () => s.navigate("Commitments");
	close.onclick = async () => {
		s.open = false;
        open.setAttribute("aria-pressed","false");
		await s.hidePanel();
	};
	name.setAttribute("aria-label", "Agent");
    name.title = "Choose the agent for this conversation";
    let selectedAgent = "default";
    let activeName = "Agent";
    let refreshSequence = 0;
    const drafts = new Map();
    let conversationID="main",savedTitle="",metadataAvailable=false,draftLoaded="";
    const draftKey=()=>"iugum.chat.draft."+selectedAgent+"."+conversationID;
    const persistDraft=()=>{drafts.set(selectedAgent,text.value);try{localStorage.setItem(draftKey(),text.value);}catch{error.textContent="Could not save the draft in this browser.";}};
    const route = path => {const base=selectedAgent === "default" ? path : "/agents/" + encodeURIComponent(selectedAgent) + path;return conversationID!=="main" && path!=="/conversations" ? base+"?conversation="+encodeURIComponent(conversationID):base;};
    const agentAPI = path => api(route(path));
    const agentPost = (path, value) => post(route(path), value);
    const startAgent=node("button","Start agent"), stopAgent=node("button","Stop agent");
    startAgent.type=stopAgent.type="button";startAgent.hidden=stopAgent.hidden=true;
    startAgent.title="Start the selected managed agent in the background";stopAgent.title="Stop the selected managed agent; closing chat only detaches";
    const manage=node("button","Agents");manage.type="button";manage.title="Browse agents";manage.setAttribute("aria-expanded","false");
    head.append(startAgent, stopAgent, manage, close);
    const drawer=node("aside",undefined,"chief-agent-drawer");drawer.hidden=true;drawer.setAttribute("aria-label","Agents drawer");
    const drawerClose=node("button","×");drawerClose.type="button";drawerClose.setAttribute("aria-label","Close agents");
    const agentSearch=node("input");agentSearch.type="search";agentSearch.placeholder="Search agents…";agentSearch.setAttribute("aria-label","Search agents");
    const agentList=node("div");
    drawer.append(drawerClose,node("strong","Agents"),agentSearch,agentList);panel.append(drawer);
    const closeDrawer=()=>{drawer.hidden=true;manage.setAttribute("aria-expanded","false");manage.focus();};
    drawerClose.onclick=closeDrawer;
    const renderAgentList=()=>{
        agentList.replaceChildren();
        const matches=Array.from(name.options || []).filter(o=>o.textContent.toLowerCase().includes(agentSearch.value.toLowerCase()));
        for(const option of matches){
            const row=node("button",option.textContent);row.type="button";row.disabled=s.busy;row.setAttribute("aria-pressed",String(option.value===selectedAgent));
            row.onclick=async()=>{if(s.busy)return;name.value=option.value;await name.onchange();closeDrawer();};agentList.append(row);
        }
        if(!matches.length)agentList.append(node("p","No matching agents."));
    };
    const toggleDrawer=()=>{if(!drawer.hidden){closeDrawer();return;}menus.close(false);agentSearch.hidden=false;drawer.hidden=false;manage.setAttribute("aria-expanded",String(!drawer.hidden));if(!drawer.hidden){renderAgentList();agentSearch.focus();}};
    manage.onclick=toggleDrawer;agentSearch.oninput=renderAgentList;
    drawer.onkeydown=e=>{e.stopPropagation();if(e.key==="Escape"){e.preventDefault();closeDrawer();}};
    let agentCatalog=[];
    let modelRequest=0;
    const updateModel=async()=>{
        const request=++modelRequest,agent=selectedAgent;
        modelPicker.disabled=true;
        modelPicker.replaceChildren(node("option",agentCatalog.find(a=>a.id===agent)?.model || "Provider default"));
        try {
            const state=await agentAPI("/models");
            if(request!==modelRequest || agent!==selectedAgent)return;
            if(state.options?.length){
                modelPicker.replaceChildren(...state.options.map(choice=>{const option=node("option",choice.name);option.value=choice.id;return option;}));
                modelPicker.value=state.selected;modelPicker.disabled=s.busy;
                modelPicker.title="Choose the model profile for the next turn";
            }else modelPicker.title=state.reason || "Model selection is unavailable";
        }catch{modelPicker.title="This runtime does not expose model selection.";}
    };
    let managedAgents=new Set();
    const updateControls=()=>{startAgent.hidden=stopAgent.hidden=!managedAgents.has(selectedAgent);};
    for(const [button,verb] of [[startAgent,"start"],[stopAgent,"stop"]]) button.onclick=async()=>{
      button.disabled=true;
      try{await agentPost("/"+verb,{});error.textContent=verb==="stop"?"Agent stopped.":"";if(verb==="start")await refresh();}catch(e){error.textContent=e.message}finally{button.disabled=false}
    };
    const policyNotice=node("div");policyNotice.setAttribute("role","status");panel.append(policyNotice);
	const messages = node("div", undefined, "chief-messages");
    const updatePromptOverflow = () => {
        const panelWidth = Math.round(panel.getBoundingClientRect().width);
        resize.setAttribute("aria-valuenow", String(Math.round(panelWidth / innerWidth * 100)));
        resize.setAttribute("aria-valuetext", panelWidth + " pixels wide");
        for (const card of messages.querySelectorAll(".chief-message.user")) {
            const body = card.querySelector(".chief-markdown");
            const long = body.scrollHeight > parseFloat(getComputedStyle(body).fontSize) * 6 + 1;
            card.classList.toggle("truncated", long);
            if (long) {
                card.tabIndex = 0;card.setAttribute("role", "button");
                const expanded = card.classList.contains("expanded");
                card.setAttribute("aria-expanded", String(expanded));
                card.setAttribute("aria-label", expanded ? "Collapse prompt" : "Expand prompt");
                card.title = "Click to expand or collapse this prompt";
            } else {
                card.removeAttribute("tabindex");card.removeAttribute("role");
                card.removeAttribute("aria-expanded");card.removeAttribute("aria-label");card.title = "";
            }
        }
    };
    const promptObserver = typeof ResizeObserver !== "undefined" ? new ResizeObserver(updatePromptOverflow) : null;
    promptObserver?.observe(messages);

	messages.setAttribute("aria-live", "polite");
	const reviews = node("div", undefined, "chief-reviews"),
		error = node("p", "", "chief-error"),
		pending = node("p", "", "chief-pending");
	error.setAttribute("role", "alert");
	const form = node("form", undefined, "chief-compose"),
		text = node("textarea");
	text.placeholder = "Message the selected agent…";
	text.setAttribute("aria-label", "Message agent");
	const actions = node("div", undefined, "chief-actions"),
		status = node("button", "Status"),
		send = node("button", "Send");
	status.type = "button";
	send.type = "submit";
	const modelPicker=node("select");modelPicker.setAttribute("aria-label","Model");modelPicker.disabled=true;modelPicker.title="Configured model; runtime model switching is not available yet";
    actions.append(modelPicker, send);
    modelPicker.onchange=async()=>{
        const agent=selectedAgent,id=modelPicker.value;modelPicker.disabled=true;
        try { await agentPost("/models",{id}); }
        catch(err){if(agent===selectedAgent)error.textContent="Could not change model: "+err.message;}
        finally{if(agent===selectedAgent)await updateModel();}
    };
	form.append(text, actions);
    text.oninput=persistDraft;
    const historyArea=node("div",undefined,"chief-history");
    const rail=node("nav",undefined,"chief-rail");rail.setAttribute("aria-label","Conversation navigator");
    const preview=node("div",undefined,"chief-turn-preview");preview.hidden=true;
    const latest=node("button","Jump to latest","chief-latest");latest.type="button";latest.hidden=true;
    const updateLatest=()=>{latest.hidden=messages.scrollHeight-messages.scrollTop-messages.clientHeight<100;};
    latest.onclick=()=>{messages.scrollTop=messages.scrollHeight;updateLatest();text.focus();};
    historyArea.append(messages,rail,preview,latest);
    panel.append(head, reviews, historyArea, error, pending, form);
    let turnLinks=[];
    const updateTurn=()=>{
        const top=messages.getBoundingClientRect().top;
        let active=0;
        turnLinks.forEach((item,i)=>{if(item.turn.getBoundingClientRect().top<=top+2)active=i;});
        turnLinks.forEach((item,i)=>item.button.setAttribute("aria-current",String(i===active)));
    };
    messages.onscroll=()=>{updateTurn();updateLatest();preview.hidden=true;};
    const clearTickHover=()=>{preview.hidden=true;turnLinks.forEach(item=>{item.button.style.removeProperty("--tick-width");item.button.classList.remove("tick-hover");});};
    rail.onmouseleave=clearTickHover;
    rail.onmousemove=event=>{
        let nearest, distance=Infinity;
        for(const item of turnLinks){const rect=item.button.getBoundingClientRect();const d=Math.abs(event.clientY-rect.top-rect.height/2);if(d<distance){distance=d;nearest=item;}}
        nearest?.show();
    };
    function addTurnLink(turn, prompt) {
        const button=node("button");button.type="button";
        const label=String(prompt || "Conversation start");
        button.setAttribute("aria-label","Jump to prompt "+(turnLinks.length+1)+": "+label.slice(0,100));
        const show=()=>{preview.textContent=label.length>600?label.slice(0,597)+"…":label;preview.hidden=false;
            const marker=button.getBoundingClientRect(),area=historyArea.getBoundingClientRect();
            preview.style.top=Math.max(4,Math.min(area.height-preview.offsetHeight-4,marker.top+marker.height/2-area.top-preview.offsetHeight/2))+"px";
            turnLinks.forEach(item=>item.button.classList.toggle("tick-hover",item.button===button));
            const center=turnLinks.findIndex(item=>item.button===button);
            turnLinks.forEach((item,i)=>item.button.style.setProperty("--tick-width",[18,14,11,9][Math.abs(i-center)]? [18,14,11,9][Math.abs(i-center)]+"px":"8px"));
        };
        button.onmouseenter=show;button.onfocus=show;
        button.onblur=()=>{if(!rail.matches(":hover"))clearTickHover();};
        button.onclick=()=>{messages.scrollTop+=turn.getBoundingClientRect().top-messages.getBoundingClientRect().top;preview.hidden=true;updateTurn();};
        button.onkeydown=event=>{if(["Enter"," "].includes(event.key)){event.preventDefault();event.stopPropagation();button.onclick();}};
        turnLinks.push({turn,button,show});rail.append(button);
    }

	let renderedHistory = "";
    let displayedHistory=[];
    const expandedPrompts = new Set();
	function showMessages(items) {
        const signature = JSON.stringify(items);
        if (signature === renderedHistory) return;
        renderedHistory = signature;
        displayedHistory=items || [];
        const title=displayedHistory.find(item=>item.role==="user")?.text || "New conversation";
        updateChatTitle(title);
        const scroll = messages.scrollTop;
        const expanded = Array.from(messages.querySelectorAll?.("details") || []).map(d => d.open);
		const bottom =
			messages.scrollHeight - messages.scrollTop - messages.clientHeight < 100;
		messages.replaceChildren();
        rail.replaceChildren();turnLinks=[];preview.hidden=true;
		let turn;
        let messageIndex = 0;
		for (const m of items || []) {
            const key = String(messageIndex++);
            if (!turn || m.role === "user") {
                turn = node("section", undefined, "chief-turn");
                messages.append(turn);
                addTurnLink(turn,m.text);
            }
			const el = node(
				"div",
				undefined,
				"chief-message " + (m.role === "user" ? "user" : "assistant"),
			);
			el.append(
				node(
					"small",
					(m.role === "user" ? "You" : activeName) +
						" · " +
						new Date(m.at).toLocaleString(),
				),
				(() => {
                    const body = node("div", undefined, "chief-markdown");
					// Server HTML is Goldmark + Bluemonday output. Attached provider
					// transcripts have no server HTML, so render their Markdown with
					// DOM nodes rather than trusting their raw text as HTML.
					if (typeof m.html === "string") body.innerHTML = m.html;
					else body.replaceChildren(renderMarkdownFallback(m.text));
                    return body;
                })(),
			);
			if (m.role === "user") {
                if (expandedPrompts.has(key)) el.classList.add("expanded");
                const toggle = () => {
                    if (!el.classList.contains("truncated")) return;
                    const before = el.getBoundingClientRect().top;
                    const expanded = el.classList.toggle("expanded");
                    if (expanded) expandedPrompts.add(key); else expandedPrompts.delete(key);
                    el.setAttribute("aria-expanded", String(expanded));
                    el.setAttribute("aria-label", expanded ? "Collapse prompt" : "Expand prompt");
                    messages.scrollTop += el.getBoundingClientRect().top - before;
                };
                el.onclick = event => {
                    if (event.target.closest?.("a,button,input,summary") || window.getSelection?.()?.toString()) return;
                    toggle();
                };
                el.onkeydown = event => {
                    if (event.target !== el || !["Enter", " "].includes(event.key)) return;
                    event.preventDefault();event.stopPropagation();toggle();
                };

            }
            turn.append(el);
		}
		Array.from(messages.querySelectorAll?.("details") || []).forEach((d, i) => { d.open = expanded[i] || false; });
        if (typeof requestAnimationFrame !== "undefined") requestAnimationFrame(()=>{updatePromptOverflow();updateTurn();});
        messages.scrollTop = bottom ? messages.scrollHeight : scroll;
        updateLatest();
	}
	function renderReviews(items) {
		reviews.replaceChildren();
		for (const a of [
			...(items || []).filter(
				(a) =>
					a.status === "pending" ||
					a.status === "executing" ||
					a.status === "failed",
			),
			...(items || [])
				.filter((a) => a.status === "complete" || a.status === "rejected")
				.slice(-3),
		]) {
			const box = node("article");
			box.append(node("strong", a.title), node("div", a.id + " · " + a.status));
			const detail = node("details");
			detail.append(
				node("summary", "Review action and evidence"),
				node(
					"pre",
					"Action: " +
						a.action +
						"\n" +
						JSON.stringify(a.payload || {}, null, 2) +
						"\n\n" +
						(a.evidence || [])
							.map((e) =>
								[e.label, e.path, e.text, e.digest].filter(Boolean).join("\n"),
							)
							.join("\n\n"),
				),
			);
			box.append(detail);
			if (a.result) box.append(node("p", a.result));
			if (a.status === "executing")
				box.append(
					node("p", "Execution started. Check its result before retrying."),
				);
			if (a.status === "pending")
				for (const decision of ["approve", "reject"]) {
					const b = node(
						"button",
						decision === "approve" ? "Approve once" : "Reject",
					);
					b.type = "button";
					b.onclick = async () => {
						b.disabled = true;
						try {
							const result = await post(
								"/approvals/" + encodeURIComponent(a.id),
								{ decision, digest: a.digest },
							);
							pending.textContent =
								result.result ||
								(result.status === "rejected"
									? "Request rejected."
									: "Decision saved.");
							await refresh();
						} catch (e) {
							error.textContent = e.message;
							b.disabled = false;
						}
					};
					box.append(b);
				}
			reviews.append(box);
		}
	}
	async function refresh() {
        const sequence = ++refreshSequence;
		try {
			const [st, history, a, conversation] = await Promise.all([
				agentAPI("/status"),
				agentAPI("/messages"),
				agentAPI("/approvals"),
                agentAPI("/conversation").catch(()=>null),
			]);
			if (sequence !== refreshSequence) return;
            metadataAvailable=!!conversation?.id;newChat.disabled=!conversation?.can_create;newChat.title=newChat.disabled?"This runtime does not support separate conversations":"New chat";
            conversationID=conversation?.id || "main";savedTitle=conversation?.title || "";
            if(draftLoaded!==draftKey()){draftLoaded=draftKey();if(!text.value){try{text.value=localStorage.getItem(draftKey()) || "";}catch{}}}
            updateChatTitle();
            activeName = st.name || "Agent";
            policyNotice.textContent = st.metadata?.open_mode === "true" ? "Open mode: agent policy bypass is active for this run." : "";
			showMessages(history);
			renderReviews(a);
		} catch (e) {
			error.textContent = e.message;
		}
	}
	async function submit(value) {
		if (s.busy || !value.trim()) return;
		s.busy = true;modelPicker.disabled=true;
        name.disabled = true;
		send.disabled = true;
		status.disabled = true;
		error.textContent = "";
		pending.textContent = "Working…";
        const priorHistory=displayedHistory;
        ++refreshSequence;
        showMessages([...priorHistory,{role:"user",text:value,at:new Date().toISOString()}]);
        messages.scrollTop=messages.scrollHeight;
        if(text.value===value)text.value="";
        persistDraft();
        try {
			await agentPost("/chat", { text: value });
			await refresh();
        } catch (e) {
            error.textContent = e.message;
            if(!text.value){text.value=value;persistDraft();showMessages(priorHistory);}
        } finally {
			s.busy = false;updateModel();
            name.disabled = false;
			send.disabled = false;
			status.disabled = false;
			pending.textContent = "";
		}
	}
	form.onsubmit = (e) => {
		e.preventDefault();
		submit(text.value);
	};
	text.onkeydown = (e) => {
        // Shadow DOM retargets this textarea to the panel host. Keep editing
        // events away from SilverBullet's global editor shortcut handler.
        // Do not preventDefault: the browser must still move/delete/select text.
        e.stopPropagation();
		if (sendsOnEnter(e)) {
			e.preventDefault();
			submit(text.value);
		}
	};
	status.onclick = () => submit("status");
	const top = node("form");
	top.id = "iugum-chief-tools";
	const query = node("input");
	query.type = "search";
	query.placeholder = "Search work…";
	query.setAttribute("aria-label", "Search wiki, sessions, and tasks");
	const scope = node("input");
	scope.className = "chief-scope";
	scope.value = "all";
	scope.setAttribute("list", "iugum-chief-scopes");
	scope.setAttribute("aria-label", "Source or project scope");
	const choices = node("datalist");
	choices.id = "iugum-chief-scopes";
	for (const v of [
		"all",
		"transcripts",
		"wiki",
		"memory",
		"tasks",
		"claude",
		"codex",
		"cursor",
		"opencode",
		"ffai",
	]) {
		const option = node("option");
		option.value = v;
		choices.append(option);
	}
	const find = node("button"),
		open = node("button", "Chat");
	find.type = "submit";
    function searchIcon() {
        const svg = document.createElementNS("http://www.w3.org/2000/svg", "svg");
        svg.setAttribute("width", "18"); svg.setAttribute("height", "18"); svg.setAttribute("viewBox", "0 0 24 24");
        svg.setAttribute("fill", "none"); svg.setAttribute("stroke", "currentColor"); svg.setAttribute("stroke-width", "2");
        svg.setAttribute("aria-hidden", "true");
        const circle = document.createElementNS(svg.namespaceURI, "circle");
        circle.setAttribute("cx", "10.5"); circle.setAttribute("cy", "10.5"); circle.setAttribute("r", "6.5");
        const line = document.createElementNS(svg.namespaceURI, "path");line.setAttribute("d", "m16 16 5 5");
        svg.append(circle,line);find.replaceChildren(svg);
        find.setAttribute("aria-label", "Search");
    }
    searchIcon();
    find.title = "Search the selected sources or project. Use -word to exclude a term.";
    open.title = "Open chat and focus the message box";
    query.title = "Search wiki pages, transcripts, memory, and tasks";
    scope.title = "Restrict search to a source or project, such as wiki or ffai";
	open.type = "button";
    const menuTools=node("div");menuTools.id="iugum-chat-menu";
    const historyButton=node("button"),moreButton=node("button");
    const menuPopup=node("div");menuPopup.id="iugum-chat-menu-popup";menuPopup.hidden=true;
    const icon=(button,label,path)=>{
        button.type="button";button.className="iugum-icon-button";button.setAttribute("aria-label",label);button.title=label;
        const svg=document.createElementNS("http://www.w3.org/2000/svg","svg");
        svg.setAttribute("viewBox","0 0 24 24");svg.setAttribute("width","18");svg.setAttribute("height","18");svg.setAttribute("fill","none");svg.setAttribute("stroke","currentColor");svg.setAttribute("stroke-width","2");svg.setAttribute("aria-hidden","true");
        const line=document.createElementNS(svg.namespaceURI,"path");line.setAttribute("d",path);svg.append(line);button.replaceChildren(svg);
    };
    icon(open,"Chat","M3 4h18v16H3z M15 4v16");open.title="Toggle chat panel";
    icon(historyButton,"Chat history","M12 3a9 9 0 1 0 0 18 9 9 0 0 0 0-18 M12 7v5l4 2");
    icon(moreButton,"More chat options","M4 12h2 M11 12h2 M18 12h2");
    menuTools.append(open);
    const chatTitle=node("div",undefined,"chief-chat-title"),chatTitleText=node("span","New conversation"),chatGlyph=node("span");
    let automaticTitle="New conversation";
    const titleKey=()=>"iugum.chat.title."+selectedAgent+(conversationID==="main"?"":"."+conversationID);
    const updateChatTitle=(fallback=automaticTitle)=>{
        automaticTitle=fallback;
        let saved=savedTitle;try{if(!saved)saved=localStorage.getItem(titleKey()) || "";}catch{}
        chatTitleText.textContent=saved || automaticTitle;chatTitle.title=chatTitleText.textContent;
    };
    chatTitle.tabIndex=0;chatTitle.setAttribute("aria-label","Conversation title; double-click or press Enter to rename");
    const editTitle=()=>{
        if(chatTitle.querySelector("input"))return;
        const input=node("input");input.value=chatTitleText.textContent;input.maxLength=200;input.setAttribute("aria-label","Chat title");
        input.classList.add("chief-title-input");
        chatTitleText.hidden=true;chatTitle.append(input);input.focus();input.select();
        let finished=false;
        const finish=async save=>{
            if(finished)return;finished=true;
            if(save){try{if(!metadataAvailable)throw new Error("This agent does not support saved titles yet.");const value=input.value.trim();const result=await agentPost("/conversation",{title:value});savedTitle=result.title;localStorage.removeItem(titleKey());}catch(e){error.textContent=e.message;finished=false;input.focus();return;}}
            input.remove();chatTitleText.hidden=false;updateChatTitle();chatTitle.focus();
        };
        input.onkeydown=e=>{e.stopPropagation();if(e.key==="Enter"||e.key==="Escape"){e.preventDefault();finish(e.key==="Enter");}};
        input.onblur=()=>finish(true);
    };
    chatTitle.ondblclick=editTitle;
    chatTitle.onkeydown=e=>{if(e.key==="Enter"&&e.target===chatTitle){e.preventDefault();e.stopPropagation();editTitle();}};
    icon(chatGlyph,"Conversation","M4 3h16v14H8l-5 4V4z");chatGlyph.removeAttribute("aria-label");chatGlyph.setAttribute("aria-hidden","true");chatTitle.append(chatGlyph,chatTitleText);
    const newChat=node("button");icon(newChat,"New chat","M12 4v16 M4 12h16");newChat.disabled=true;newChat.title="New chat requires separate conversation support (not yet available)";
    icon(manage,"Agents","M3 4h18v16H3z M15 4v16");
    close.hidden=true;
    const headerMenus=node("div",undefined,"chief-header-menus");headerMenus.append(newChat,historyButton,moreButton,menuPopup);head.insertBefore(headerMenus,head.firstChild);head.insertBefore(chatTitle,headerMenus);
    open.onclick = async () => {
        results.hidden=true;menuPopup.hidden=true;
        s.open=!(panel.isConnected && panel.getBoundingClientRect().height>0);open.setAttribute("aria-pressed",String(s.open));
        if(s.open){await s.showPanel(panel);setTimeout(()=>{if(s.open)text.focus();},100);}else await s.hidePanel();
    };
    const menus=actionMenu(menuPopup,[historyButton,moreButton]);
    const switchConversation=async id=>{
        if(s.busy)return;
        persistDraft();try{localStorage.setItem("iugum.chat.active."+selectedAgent,id);}catch{}conversationID=id;draftLoaded="";text.value="";savedTitle="";renderedHistory="";expandedPrompts.clear();messages.replaceChildren();
        await refresh();
    };
    newChat.onclick=async()=>{if(s.busy)return;newChat.disabled=true;try{const created=await agentPost("/conversations",{});await switchConversation(created.id);text.focus();}catch(e){error.textContent=e.message;}finally{newChat.disabled=false;}};
    historyButton.onclick=async()=>{
        if(historyButton.getAttribute("aria-expanded")==="true"){menus.close();return;}
        try{
            const conversations=await agentAPI("/conversations");
            menus.toggle(historyButton,()=>{
                menuPopup.replaceChildren(node("strong","Conversation history"));
                for(const item of conversations){const entry=node("button",item.title || "New conversation");entry.type="button";entry.onclick=()=>{menus.close();switchConversation(item.id);};menuPopup.append(entry);}
            });
        }catch(e){error.textContent=e.message;}
    };
    moreButton.onclick=()=>menus.toggle(moreButton,()=>{
        menuPopup.replaceChildren();
        for(const [label,run] of [["Export transcript",()=>{
            const transcript=displayedHistory.map(item=>"## "+(item.role==="user"?"You":activeName)+(item.at?" · "+item.at:"")+"\n\n"+item.text).join("\n\n");
            const url=URL.createObjectURL(new Blob([transcript],{type:"text/markdown;charset=utf-8"}));const link=node("a");link.href=url;link.download="chat-transcript.md";link.click();setTimeout(()=>URL.revokeObjectURL(url),1000);
        }],["Agent settings",async()=>{
            const agent=selectedAgent;
            drawer.hidden=false;manage.setAttribute("aria-expanded","true");agentSearch.hidden=true;
            agentList.replaceChildren(node("strong",activeName),node("p","Loading settings…"));
            try {
                const settings=await agentAPI("/settings");
                if(agent!==selectedAgent || drawer.hidden || !agentSearch.hidden)return;
                agentList.replaceChildren(node("strong",activeName));
                if(settings.reason)agentList.append(node("p",settings.reason));
                for(const field of settings.fields || []){
                    const label=node("label",field.label),input=node("textarea"),save=node("button","Save "+field.label.toLowerCase()),notice=node("p");
                    input.value=field.value;input.setAttribute("aria-label",field.label);input.readOnly=!field.editable;label.append(input);
                    save.type="button";save.disabled=!field.editable;notice.setAttribute("role","status");
                    save.onclick=async()=>{save.disabled=true;notice.textContent="Saving…";
                        try{await post((agent==="default"?"":"/agents/"+encodeURIComponent(agent))+"/settings",{id:field.id,value:input.value});notice.textContent="Saved. Applies to the next turn.";}
                        catch(err){notice.textContent="Could not save: "+err.message;}
                        finally{save.disabled=!field.editable;}
                    };
                    agentList.append(label,save,notice);
                }
            }catch(err){if(agent===selectedAgent && !drawer.hidden)agentList.replaceChildren(node("p","Could not load settings: "+err.message));}
        }]]){
            const entry=node("button",label);entry.type="button";entry.onclick=()=>{menus.close();run();};menuPopup.append(entry);
        }
    });

	const results = node("div");
	results.id = "iugum-chief-results";
	results.hidden = true;
	const options = node("div"); options.id="iugum-search-options";options.hidden=true;
    options.append(node("label", "Search within"),scope,choices,node("p","Use -word in your search to exclude a term."));
    const plus=node("button","+");plus.type="button";plus.title="Search options";plus.setAttribute("aria-label","Search options");plus.setAttribute("aria-expanded","false");
    plus.onclick=()=>{top.classList.add("search-expanded");options.hidden=!options.hidden;plus.setAttribute("aria-expanded",String(!options.hidden));if(!options.hidden){results.hidden=true;scope.focus();}};
    const sourceRow=node("div");sourceRow.id="iugum-search-sources";
    const sourceButtons=[];
    for(const [label,value] of [["all","all"],["wiki","wiki"],["chats","transcripts"]]){
        const button=node("button",label);button.type="button";button.setAttribute("aria-pressed",String(value==="all"));
        button.onclick=()=>{scope.value=value;options.hidden=true;sourceButtons.forEach(([b,v])=>b.setAttribute("aria-pressed",String(v===value)));results.hidden=true;if(query.value.trim())top.requestSubmit();};
        sourceButtons.push([button,value]);sourceRow.append(button);
    }
    const filters=node("button","filters");filters.type="button";filters.onclick=()=>plus.onclick();sourceRow.append(filters);
    top.append(query, find, sourceRow, options, results);
    const searchElements = [query, find, results];
	let searchSequence = 0;
    let lastSearch = "";
    const searchKey = () => JSON.stringify([query.value.trim(), scope.value.trim()]);
    const reopenResults = () => {
        top.classList.add("search-expanded");
        options.hidden=true;plus.setAttribute("aria-expanded","false");
        if (lastSearch && lastSearch === searchKey()) results.hidden = false;
    };
    query.onfocus = reopenResults;
    query.onclick = reopenResults;
    query.oninput = () => { if (lastSearch !== searchKey()) results.hidden = true; };
    scope.oninput = () => { results.hidden = true; };
    const dismissSearch = (event) => {
        if (!event.composedPath().includes(top)) {top.classList.remove("search-expanded");results.hidden = true;options.hidden=true;plus.setAttribute("aria-expanded","false");}
    };
    document.addEventListener?.("pointerdown", dismissSearch);
    const dismissDrawer=event=>{if(!drawer.hidden&&!event.composedPath().includes(drawer)&&!event.composedPath().includes(manage)){event.preventDefault();closeDrawer();}};
    document.addEventListener?.("pointerdown",dismissDrawer);

	top.onsubmit = async (e) => {
		e.preventDefault();
		if (!query.value.trim()) { query.focus(); return; }
		options.hidden=true;plus.setAttribute("aria-expanded","false");
        const seq = ++searchSequence;
        const requestedKey = searchKey();
        find.disabled = true;
        find.textContent = "Searching…";
        find.setAttribute("aria-label", "Searching…");
        top.setAttribute("aria-busy", "true");
		results.hidden = false;
		results.replaceChildren(node("p", "Searching…"));
		try {
			const data = await api(
				"/search?q=" +
					encodeURIComponent(query.value) +
					"&scope=" +
					encodeURIComponent(scope.value),
			);
			if (seq !== searchSequence) return;
            lastSearch = requestedKey;
			results.replaceChildren();
			const dismiss = node("button", "Close results");
			dismiss.type = "button";
			dismiss.onclick = () => {
				results.hidden = true;
			};
			results.append(dismiss);
			const stamp = new Date(data.indexed_at);
			if ("indexed_at" in data)
				results.append(
					node(
						"p",
						(data.indexed_at && stamp.getFullYear() > 1
							? "Index checked " + stamp.toLocaleString()
							: "Index freshness unavailable") +
							(data.partial ? " · Partial index" : ""),
					),
				);
			if (data.warnings?.length) {
				const notices = node("details");
				notices.append(
					node("summary", "Source notices (" + data.warnings.length + ")"),
					node("pre", data.warnings.join("\n")),
				);
				results.append(notices);
			}
			const cards = searchCards(data);
            results.append(node("p", cards.length + " results", "chief-search-count"));
			if (!cards.length) results.append(node("p", "No matching passages."));
			let shown = 0;
			const more = node("button", "Show more");
			more.type = "button";
			const next = () => {
				for (const c of cards.slice(shown, shown + 5)) {
					const row = node("article");
                    const title = node("button", c.title);
                    title.type = "button";
                    title.title = c.page ? "Open wiki page " + c.page : "View indexed passage and source";
                    const preview = node("div", undefined, "chief-search-markdown chief-search-preview");
                    const passage = node("div", undefined, "chief-search-markdown");
                    for (const body of [preview, passage]) {
                        if (typeof c.html === "string") body.innerHTML = c.html;
                        else body.textContent = c.text || "";
                    }
                    const d = node("details");
                    d.append(node("summary", "Indexed passage and source"), passage, node("pre", c.provenance));
                    title.onclick = async () => {
                        if (c.page) {
                            try { results.hidden = true; await s.navigate(c.page); }
                            catch (err) { results.hidden = false; row.append(node("p", "Could not open page: " + err.message)); }
                        } else {
                            d.open = !d.open;
                            if (d.open) d.scrollIntoView({block:"nearest"});
                        }
                    };
                    row.append(title, node("small", " " + c.source), preview, d);
					results.insertBefore(row, more);
				}
				shown += 5;
				more.hidden = shown >= cards.length;
			};
			more.onclick = next;
			results.append(more);
			next();
            if (searchKey() !== requestedKey) results.hidden = true;
		} catch (err) {
			if (seq === searchSequence)
				results.replaceChildren(node("p", err.message));
        } finally {
            if (seq === searchSequence) {
                find.disabled = false;
                searchIcon();
                top.setAttribute("aria-busy", "false");
            }
        }
	};
	top.onkeydown = (e) => {
		if (e.key === "Escape") { e.preventDefault(); e.stopPropagation(); results.hidden = true;options.hidden=true;plus.setAttribute("aria-expanded","false"); }
	};
	s.attach = () => {
		if (!document.getElementById("iugum-chief-host-style")) {
			const style = node("style", hostCSS);
			style.id = "iugum-chief-host-style";
			document.head.append(style);
		}
		const nav = document.querySelector("#sb-top .wrapper");
        const nativeActions=nav?.querySelector?.(".sb-actions");
        if(nativeActions && menuTools.parentNode!==nativeActions)nativeActions.append(menuTools);
        if (nav && top.parentNode !== nav) {
            const actions = nav.querySelector?.(".sb-actions");
            if (actions) nav.insertBefore(top, actions); else nav.append(top);
        }
	};
	s.attach();
	s.observer = new MutationObserver(() => s.attach());
	s.observer.observe(document.body, { childList: true, subtree: true });
	let chatStarted = false;
    s.enable = async feature => {
        if(feature === "search" || feature === "both") s.searchEnabled = true;
        if(feature === "chat" || feature === "both") s.chatEnabled = true;
        for(const el of searchElements) { if(el !== results) el.hidden = !s.searchEnabled; }
        menuTools.hidden = !s.chatEnabled;open.hidden = !s.chatEnabled;open.setAttribute("aria-pressed",String(s.open));
        if (!s.chatEnabled || chatStarted) return;
        chatStarted = true; s.open = true;
        await s.showPanel(panel);
try {
        const options = await api("/agents");
        agentCatalog=options;
        if(!options.length){text.disabled=send.disabled=status.disabled=true;error.textContent="No agents configured or permitted. Add an ACP connection or agent home to the wiki configuration.";return;}
        managedAgents=new Set(options.filter(a=>a.transport==="iugum").map(a=>a.id));
        for (const agent of options) {
            const option = node("option", agent.name + (agent.model ? " · " + agent.model : ""));
            option.value = agent.id; name.append(option);
        }
        if (!options.some(a => a.id === selectedAgent) && options.length) selectedAgent = options[0].id;
        name.value = selectedAgent;try{conversationID=localStorage.getItem("iugum.chat.active."+selectedAgent)||"main";}catch{}updateControls();updateModel();
    } catch (err) { error.textContent = "Could not load agents: " + err.message; }
    name.onchange = async () => {
        if (s.busy) return;
        persistDraft();
        selectedAgent = name.value;conversationID="main";try{conversationID=localStorage.getItem("iugum.chat.active."+selectedAgent)||"main";}catch{}draftLoaded="";savedTitle="";updateControls();updateModel();
        text.value = "";
        renderedHistory = "";
        expandedPrompts.clear();
        messages.replaceChildren(); rail.replaceChildren();turnLinks=[];preview.hidden=true; reviews.replaceChildren(); error.textContent = "";
        work.hidden = selectedAgent !== "default";
        await refresh();
    };
    await refresh();
    };
    s.dispose=async()=>{document.removeEventListener("pointerdown",dismissSearch);document.removeEventListener("pointerdown",dismissDrawer);menus.dispose();promptObserver?.disconnect();s.observer.disconnect();top.remove();menuTools.remove();document.getElementById("iugum-chief-host-style")?.remove();await s.hidePanel();if(current===s)current=null;};
    await s.enable(feature);
	return s;
}

// Explicit command reopens the existing panel; navigation mount preserves closure.
export async function open() {
	if (!current || !current.chatEnabled) return;
	current.open = true;
	await current.showPanel(current.panel);
}
