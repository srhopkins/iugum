import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
let source = await readFile(new URL("./chief.js", import.meta.url), "utf8");
source = source.replace('"./ui-controls.js"', JSON.stringify(new URL('./ui-controls.js', import.meta.url).href));
source = source.replace('"./ui-styles.js"', JSON.stringify(new URL('./ui-styles.js', import.meta.url).href));
const { searchCards, sendsOnEnter } = await import(
	"data:text/javascript;base64," + Buffer.from(source).toString("base64")
);
assert.equal(sendsOnEnter({ key: "Enter" }), true);
assert.equal(sendsOnEnter({ key: "Enter", shiftKey: true }), false);
assert.equal(sendsOnEnter({ key: "Enter", isComposing: true }), false);
const cards = searchCards({
	transcripts: [
		{
			Project: "/work/project",
			Platform: "claude",
			Text: "<script>unsafe</script>",
			Path: "/path",
			Line: 4,
		},
	],
	wiki: [{ path: "Work/Plan.md", text: "Body" }],
	tasks: [{ id: "t1", title: "Ship clear ticket", dependencies: ["t2"] }],
	memory: [{ NS: "chief", Key: "preference", Value: "brief" }],
});
assert.equal(cards.length, 4);
assert.equal(cards[0].title, "project");
assert.equal(cards[0].text, "<script>unsafe</script>");
assert.equal(cards[1].page, "Work/Plan");
assert(source.includes("body.innerHTML = m.html"));
assert(source.includes("/.proxy/iugum/api"));
console.log("Chief native panel data and keyboard tests passed");

// Exercise singleton mount, navigation restore, and explicit reopen without a browser.
class Element {
 addEventListener() {}
 removeEventListener() {}

	constructor(tag) {
		this.tag = tag;
		this.children = [];
		this.style = {};
		this.value = "";
		this.scrollHeight = 0;
		this.clientHeight = 0;
		this.scrollTop = 0;
	}
	append(...items) {
		for (const item of items) {
			if (item.parent)
				item.parent.children = item.parent.children.filter((x) => x !== item);
			item.parent = this;
			this.children.push(item);
		}
	}
	replaceChildren(...items) {
		this.children = [];
		this.append(...items);
	}
	removeAttribute(k) { delete this[k]; }
	setAttribute(k, v) {
		this[k] = v;
	}
	get parentNode() {
		return this.parent;
	}
	insertBefore(item, before) {
		if (item.parent)
			item.parent.children = item.parent.children.filter((x) => x !== item);
		item.parent = this;
		this.children.splice(this.children.indexOf(before), 0, item);
	}
	get isConnected() {
		return !!this.parent;
	}
}
const head = new Element("head"),
	body = new Element("body"),
	wrapper = new Element("div"),
	main = new Element("main");
body.append(wrapper);
wrapper.append(main);
const find = (root, id) =>
	root.id === id ? root : root.children.map((c) => find(c, id)).find(Boolean);
globalThis.document = {
 addEventListener() {}, removeEventListener() {},
	head,
	body,
	createElement: (t) => new Element(t),
 createElementNS: (ns,t) => Object.assign(new Element(t), {namespaceURI:ns}),
	createTextNode: (text) =>
		Object.assign(new Element("text"), { textContent: text }),
	getElementById: (id) => find(head, id) || find(body, id),
	querySelector: () => main,
};
globalThis.MutationObserver = class {
	observe() {}
};
globalThis.fetch = async (url) => ({
	ok: true,
	json: async () => (url.endsWith("/status") ? { name: "Chief" } : []),
});
const module = await import(
	"data:text/javascript;base64," + Buffer.from(source).toString("base64")
);
let shows = 0,
	hides = 0;
const show = async () => {
		shows++;
	},
	hide = async () => {
		hides++;
	};
const state = await module.mount(show, hide, async () => {});
assert.equal(main.children.length, 1);
const findClose = root => root["aria-label"] === "Close chat" ? root : root.children.map(findClose).find(Boolean);
await findClose(state.panel).onclick();
assert.equal(hides, 1);
await module.mount(show, hide, async () => {});
assert.equal(shows, 1);
assert.equal(main.children.length, 1);
await module.open();
assert.equal(shows, 2);
console.log("Chief native mount, restore, and reopen tests passed");
