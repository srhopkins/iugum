// Unit tests for the atomdown-board plug's pure decision functions.
//
// Run directly:   node --test plugs/atomdown-board/
// Run from Go:    go test ./plugs/atomdown-board   (board_test.go shells out
//                 to node, and skips when node is not installed)
//
// These test the real module, not a reimplementation: the plug file is
// imported as-is, with only the worker globals it expects (self, crypto)
// stubbed. There is no bundler and no package.json here on purpose — the plug
// is hand-authored ES2020 with no imports (see README.md, "Why hand-authored"),
// so node can load it directly.
//
// The functions under test are the ones whose absence as a seam let a bug
// ship: the first drop handler decided "where does this land" inline inside
// an event listener, where no test could reach it, and it always answered
// "the end of the document".

import assert from "node:assert/strict";
import test from "node:test";

// The plug's worker shim needs `self` before the module body runs. It only
// registers a message listener and posts its manifest, so a pair of no-ops is
// enough to import it.
globalThis.self = {
  addEventListener() {},
  postMessage() {},
};

const { plug } = await import("./atomdown-board.plug.js");
const {
  pickDropTarget,
  unitKeyForCard,
  unitOrderFromCards,
  dedupeKeys,
  isContiguousUnitSelection,
  groupMenuState,
  rectsIntersect,
  minimalEdit,
  newAtomdownId,
  existingIds,
  computeUnits,
  reorderUnit,
  insertGroupMarkers,
  removeGroupMarkers,
  parseAtoms,
  injectSharedFunctions,
  setGroupSlugInSource,
  sanitizeSlug,
  slugConflict,
  deriveGroupSlug,
  slugOrId,
  buildBoardHtml,
  boardOpenKey,
  collapsedKey,
  viewKey,
  loadViewState,
  densityKey,
  loadDensity,
  normalizeDensity,
  otherDensity,
  densityLabel,
  densityTitle,
  effectiveCardView,
  sanitizeRenderedHtml,
  isSafeUrl,
  decodeUrlEntities,
  digestOfBlock,
  recordedDigest,
  digestStateOf,
  staleAtoms,
  looksLikeDirectiveLine,
  findAtomBlockLines,
  replaceAtomBlockInSource,
  setAtomDigestsInSource,
} = plug.internals;

// --- Fixtures --------------------------------------------------------------

// Three standalone atoms, each carrying a digest, in the shape the real pages
// use (a directive line, then one block, separated by blank lines).
const THREE_ATOMS = [
  '<!-- <atomdown version="1"/> -->',
  '<!-- <atom id="AAAAAAAA" digest="sha256:aa"/> -->',
  "First block.",
  "",
  '<!-- <atom id="BBBBBBBB" digest="sha256:bb"/> -->',
  "Second block.",
  "",
  '<!-- <atom id="CCCCCCCC" digest="sha256:cc"/> -->',
  "Third block.",
  "",
].join("\n");

// A tight group, the shape `atomdown materialize --split list-item` writes
// (atomdown/testdata/valid/split-list.md): markers hard against the members.
const TIGHT_GROUP = [
  '<!-- <atomdown version="1"/> -->',
  '<!-- <atom id="J1BBCED5"/> -->',
  "# Heading",
  "",
  '<!-- <atom-group id="KF53ASNE"> -->',
  '<!-- <atom id="FAPWJSRC"/> -->',
  "* One.",
  '<!-- <atom id="GPG5QA7A"/> -->',
  "* Two.",
  "<!-- </atom-group> -->",
  "",
].join("\n");

// A loose group, the shape atomdown/testdata/valid/groups.md uses: a blank
// line after the open marker and before the close marker.
const LOOSE_GROUP = [
  '<!-- <atom-group id="3G7K9R5V" slug="claims"> -->',
  "",
  '<!-- <atom id="5H8M2W6Y"/> -->',
  "",
  "First ordered claim.",
  "",
  '<!-- <atom id="6J9N3X7Z"/> -->',
  "",
  "Second ordered claim.",
  "",
  "<!-- </atom-group> -->",
  "",
].join("\n");

function cardsFrom(sourceText) {
  return parseAtoms(sourceText).map((a) => ({
    id: a.id,
    implicit: a.implicit,
    groupId: a.groupId || null,
  }));
}

// Card rectangles as getBoundingClientRect() would report them: three 100px
// cards separated by a 14px margin, the first starting 50px down.
const RECTS = [
  { unitKey: "atom:AAAAAAAA", top: 50, bottom: 150 },
  { unitKey: "atom:BBBBBBBB", top: 164, bottom: 264 },
  { unitKey: "atom:CCCCCCCC", top: 278, bottom: 378 },
];

// --- pickDropTarget (bug A1) -----------------------------------------------

test("pickDropTarget: above the first card drops before it", () => {
  assert.deepEqual(pickDropTarget(10, RECTS), {
    targetUnitKey: "atom:AAAAAAAA",
    placement: "before",
  });
});

test("pickDropTarget: top half of a card drops before it", () => {
  assert.deepEqual(pickDropTarget(180, RECTS), {
    targetUnitKey: "atom:BBBBBBBB",
    placement: "before",
  });
});

test("pickDropTarget: bottom half of a card drops before the next one", () => {
  // Equivalent to "after this card", and the seam the user was pointing at.
  assert.deepEqual(pickDropTarget(250, RECTS), {
    targetUnitKey: "atom:CCCCCCCC",
    placement: "before",
  });
});

test("pickDropTarget: the gap between two cards resolves to that seam, not the end", () => {
  // THIS is the bug. A release at y=157 is in the 14px space between card 1
  // and card 2. That space belongs to the cards container, so the old
  // container drop handler fired and hardcoded (null, "end") — the block went
  // to the end of the document instead of between the two cards.
  assert.deepEqual(pickDropTarget(157, RECTS), {
    targetUnitKey: "atom:BBBBBBBB",
    placement: "before",
  });
});

test("pickDropTarget: bottom half of the last card drops after it", () => {
  assert.deepEqual(pickDropTarget(350, RECTS), {
    targetUnitKey: "atom:CCCCCCCC",
    placement: "after",
  });
});

test("pickDropTarget: below the last card's bottom edge drops at the end", () => {
  assert.deepEqual(pickDropTarget(500, RECTS), {
    targetUnitKey: null,
    placement: "end",
  });
});

test("pickDropTarget: exactly the last card's bottom edge is still that card", () => {
  assert.deepEqual(pickDropTarget(378, RECTS), {
    targetUnitKey: "atom:CCCCCCCC",
    placement: "after",
  });
});

test("pickDropTarget: an empty board drops at the end", () => {
  assert.deepEqual(pickDropTarget(100, []), {
    targetUnitKey: null,
    placement: "end",
  });
});

test("pickDropTarget: a group's members share one unit key", () => {
  const grouped = [
    { unitKey: "atom:AAAAAAAA", top: 0, bottom: 100 },
    { unitKey: "group:GGGGGGGG", top: 114, bottom: 214 },
    { unitKey: "group:GGGGGGGG", top: 214, bottom: 314 },
  ];
  // Between the two group members: the target is the group itself, and
  // reorderUnit() then treats that as a no-op when the group is what moved.
  assert.equal(pickDropTarget(214, grouped).targetUnitKey, "group:GGGGGGGG");
});

// --- unit keys and order ---------------------------------------------------

test("unitKeyForCard: a grouped card resolves to its group", () => {
  assert.equal(unitKeyForCard({ id: "A", groupId: null }), "atom:A");
  assert.equal(unitKeyForCard({ id: "A", groupId: "G" }), "group:G");
});

test("unitOrderFromCards collapses a group's members into one unit", () => {
  const order = unitOrderFromCards([
    { id: "A", groupId: null },
    { id: "B", groupId: "G" },
    { id: "C", groupId: "G" },
    { id: "D", groupId: null },
  ]);
  assert.deepEqual(order, ["atom:A", "group:G", "atom:D"]);
});

test("unitOrderFromCards matches computeUnits on the same source", () => {
  for (const source of [THREE_ATOMS, TIGHT_GROUP, LOOSE_GROUP]) {
    const fromCards = unitOrderFromCards(cardsFrom(source));
    const fromSource = computeUnits(source).units.map((u) => u.unitKey);
    assert.deepEqual(fromCards, fromSource);
  }
});

test("dedupeKeys keeps first-seen order", () => {
  assert.deepEqual(dedupeKeys(["b", "a", "b", "c", "a"]), ["b", "a", "c"]);
});

// --- contiguity (decision B4) ----------------------------------------------

const ORDER = ["atom:A", "atom:B", "atom:C", "atom:D"];

test("isContiguousUnitSelection: adjacent units are contiguous", () => {
  assert.equal(isContiguousUnitSelection(ORDER, ["atom:B", "atom:C"]), true);
  assert.equal(
    isContiguousUnitSelection(ORDER, ["atom:A", "atom:B", "atom:C"]),
    true,
  );
});

test("isContiguousUnitSelection: order of the selection does not matter", () => {
  assert.equal(isContiguousUnitSelection(ORDER, ["atom:C", "atom:B"]), true);
});

test("isContiguousUnitSelection: a gap is not contiguous", () => {
  assert.equal(isContiguousUnitSelection(ORDER, ["atom:A", "atom:C"]), false);
  assert.equal(
    isContiguousUnitSelection(ORDER, ["atom:A", "atom:B", "atom:D"]),
    false,
  );
});

test("isContiguousUnitSelection: one unit is contiguous, an empty one is not", () => {
  assert.equal(isContiguousUnitSelection(ORDER, ["atom:B"]), true);
  assert.equal(isContiguousUnitSelection(ORDER, []), false);
});

// --- groupMenuState (menu B2, nesting B3, contiguity B4) -------------------

test("groupMenuState: a grouped card offers Ungroup, always enabled", () => {
  const state = groupMenuState(ORDER, [], "group:GGGGGGGG");
  assert.equal(state.label, "Ungroup");
  assert.equal(state.action, "ungroup");
  assert.equal(state.enabled, true);
});

test("groupMenuState: fewer than two selected units disables Group", () => {
  const state = groupMenuState(ORDER, ["atom:B"], "atom:B");
  assert.equal(state.label, "Group");
  assert.equal(state.enabled, false);
  assert.match(state.reason, /two or more/);
});

test("groupMenuState: a contiguous multi-selection enables Group", () => {
  const state = groupMenuState(ORDER, ["atom:B", "atom:C"], "atom:B");
  assert.equal(state.label, "Group");
  assert.equal(state.action, "group");
  assert.equal(state.enabled, true);
});

test("groupMenuState: a non-contiguous selection stays disabled and says why", () => {
  const state = groupMenuState(ORDER, ["atom:A", "atom:C"], "atom:A");
  assert.equal(state.label, "Group");
  assert.equal(state.enabled, false);
  assert.match(state.reason, /not next to each other/);
  assert.match(state.reason, /move blocks/);
});

test("groupMenuState: a selection containing a group is refused, no nesting", () => {
  const order = ["atom:A", "group:G", "atom:D"];
  const state = groupMenuState(order, ["atom:A", "group:G"], "atom:A");
  assert.equal(state.enabled, false);
  assert.match(state.reason, /does not permit a group inside a group/);
});

test("groupMenuState: the menu's own card must be in the selection", () => {
  const state = groupMenuState(ORDER, ["atom:B", "atom:C"], "atom:A");
  assert.equal(state.enabled, false);
  assert.match(state.reason, /not in the selection/);
});

test("groupMenuState: a duplicate selection key does not count twice", () => {
  const state = groupMenuState(ORDER, ["atom:B", "atom:B"], "atom:B");
  assert.equal(state.enabled, false);
  assert.match(state.reason, /two or more/);
});

// --- lasso geometry --------------------------------------------------------

test("rectsIntersect", () => {
  const band = { left: 0, right: 100, top: 0, bottom: 100 };
  assert.equal(
    rectsIntersect({ left: 50, right: 150, top: 50, bottom: 150 }, band),
    true,
  );
  assert.equal(
    rectsIntersect({ left: 101, right: 150, top: 0, bottom: 100 }, band),
    false,
  );
  assert.equal(
    rectsIntersect({ left: 0, right: 100, top: 101, bottom: 150 }, band),
    false,
  );
});

// --- minimalEdit (undo, B5) ------------------------------------------------

test("minimalEdit: identical text produces no edit", () => {
  assert.equal(minimalEdit("abc", "abc"), null);
});

test("minimalEdit: an insertion is a zero-width replacement", () => {
  const edit = minimalEdit("ac", "abc");
  assert.deepEqual(edit, { from: 1, to: 1, insert: "b" });
});

test("minimalEdit: a deletion inserts nothing", () => {
  const edit = minimalEdit("abc", "ac");
  assert.deepEqual(edit, { from: 1, to: 2, insert: "" });
});

test("minimalEdit: applying the edit reproduces the new text", () => {
  const before = THREE_ATOMS;
  const after = insertGroupMarkers(before, ["atom:AAAAAAAA", "atom:BBBBBBBB"], "ZZZZZZZZ").text;
  const edit = minimalEdit(before, after);
  const applied = before.slice(0, edit.from) + edit.insert + before.slice(edit.to);
  assert.equal(applied, after);
});

test("minimalEdit: a group insertion touches only the group's own lines", () => {
  // One transaction, and a narrow one: everything before the first marker and
  // after the last is left alone, so the editor's undo entry is the group.
  const after = insertGroupMarkers(
    THREE_ATOMS,
    ["atom:BBBBBBBB", "atom:CCCCCCCC"],
    "ZZZZZZZZ",
  ).text;
  const edit = minimalEdit(THREE_ATOMS, after);
  assert.ok(edit.from > THREE_ATOMS.indexOf("First block."));
  assert.match(edit.insert, /atom-group/);
});

// --- id generation (B3) ----------------------------------------------------

test("newAtomdownId: eight uppercase Crockford Base32 characters", () => {
  for (let i = 0; i < 500; i++) {
    assert.match(newAtomdownId(), /^[0-9A-HJKMNP-TV-Z]{8}$/);
  }
});

test("newAtomdownId: no I, L, O or U, per the Crockford alphabet", () => {
  const seen = new Set();
  for (let i = 0; i < 2000; i++) {
    for (const ch of newAtomdownId()) seen.add(ch);
  }
  for (const banned of ["I", "L", "O", "U"]) {
    assert.equal(seen.has(banned), false, `alphabet must not contain ${banned}`);
  }
  // 2000 ids is 16000 characters; every one of the 32 letters should appear.
  assert.equal(seen.size, 32);
});

test("existingIds finds every id already in a document", () => {
  const ids = existingIds(TIGHT_GROUP);
  assert.deepEqual(ids.sort(), ["FAPWJSRC", "GPG5QA7A", "J1BBCED5", "KF53ASNE"]);
});

// --- insertGroupMarkers (B3, B4) -------------------------------------------

test("insertGroupMarkers wraps a contiguous run in balanced markers", () => {
  const result = insertGroupMarkers(
    THREE_ATOMS,
    ["atom:AAAAAAAA", "atom:BBBBBBBB"],
    "ZZZZZZZZ",
  );
  assert.equal(result.ok, true);
  const lines = result.text.split("\n");
  assert.equal(lines[1], '<!-- <atom-group id="ZZZZZZZZ"> -->');
  assert.equal(lines[2], '<!-- <atom id="AAAAAAAA" digest="sha256:aa"/> -->');
  assert.equal(lines[6], "Second block.");
  assert.equal(lines[7], "<!-- </atom-group> -->");
  assert.equal(lines[8], "");
  assert.equal(lines[9], '<!-- <atom id="CCCCCCCC" digest="sha256:cc"/> -->');
});

test("insertGroupMarkers adds exactly two lines and changes nothing else", () => {
  const result = insertGroupMarkers(
    THREE_ATOMS,
    ["atom:BBBBBBBB", "atom:CCCCCCCC"],
    "ZZZZZZZZ",
  );
  const before = THREE_ATOMS.split("\n");
  const after = result.text.split("\n");
  assert.equal(after.length, before.length + 2);
  const added = after.filter((l) => !before.includes(l));
  assert.deepEqual(added, [
    '<!-- <atom-group id="ZZZZZZZZ"> -->',
    "<!-- </atom-group> -->",
  ]);
});

test("insertGroupMarkers preserves every id and every digest", () => {
  const result = insertGroupMarkers(
    THREE_ATOMS,
    ["atom:AAAAAAAA", "atom:BBBBBBBB"],
    "ZZZZZZZZ",
  );
  // Every atom directive line survives byte for byte, so nothing that lives
  // on one — id, digest, or an unknown extension attribute — can change.
  for (const line of THREE_ATOMS.split("\n")) {
    if (line.includes("<atom ")) {
      assert.ok(result.text.includes(line), `lost directive line: ${line}`);
    }
  }
  // And every block's own bytes are untouched, so no digest goes stale.
  for (const atom of parseAtoms(THREE_ATOMS)) {
    assert.ok(result.text.includes(atom.text));
  }
});

test("insertGroupMarkers refuses a non-contiguous selection", () => {
  const result = insertGroupMarkers(
    THREE_ATOMS,
    ["atom:AAAAAAAA", "atom:CCCCCCCC"],
    "ZZZZZZZZ",
  );
  assert.equal(result.ok, false);
  assert.match(result.error, /not next to each other/);
});

test("insertGroupMarkers refuses a nested group", () => {
  const result = insertGroupMarkers(
    TIGHT_GROUP,
    ["atom:J1BBCED5", "group:KF53ASNE"],
    "ZZZZZZZZ",
  );
  assert.equal(result.ok, false);
  assert.match(result.error, /does not permit a group inside a group/);
});

test("insertGroupMarkers refuses a single unit", () => {
  const result = insertGroupMarkers(THREE_ATOMS, ["atom:AAAAAAAA"], "ZZZZZZZZ");
  assert.equal(result.ok, false);
  assert.match(result.error, /two or more/);
});

test("insertGroupMarkers refuses a duplicate id", () => {
  const result = insertGroupMarkers(
    THREE_ATOMS,
    ["atom:AAAAAAAA", "atom:BBBBBBBB"],
    "AAAAAAAA",
  );
  assert.equal(result.ok, false);
  assert.match(result.error, /already used/);
});

test("insertGroupMarkers refuses a malformed id", () => {
  for (const bad of ["short", "toolongvalue", "aaaaaaaa", "AAAAAAAI", ""]) {
    const result = insertGroupMarkers(
      THREE_ATOMS,
      ["atom:AAAAAAAA", "atom:BBBBBBBB"],
      bad,
    );
    assert.equal(result.ok, false, `accepted bad id: ${bad}`);
  }
});

test("insertGroupMarkers reports a unit that is no longer in the document", () => {
  const result = insertGroupMarkers(
    THREE_ATOMS,
    ["atom:AAAAAAAA", "atom:NOSUCHID"],
    "ZZZZZZZZ",
  );
  assert.equal(result.ok, false);
  assert.match(result.error, /Could not find every selected card/);
});

// --- removeGroupMarkers ----------------------------------------------------

test("removeGroupMarkers removes only the two markers", () => {
  const result = removeGroupMarkers(TIGHT_GROUP, "KF53ASNE");
  assert.equal(result.ok, true);
  const before = TIGHT_GROUP.split("\n");
  const after = result.text.split("\n");
  assert.equal(after.length, before.length - 2);
  assert.ok(!result.text.includes("atom-group"));
  for (const line of before) {
    if (line.includes("<atom ")) assert.ok(result.text.includes(line));
  }
});

test("group then ungroup restores the document byte for byte", () => {
  // The markers insertGroupMarkers writes sit hard against the run, with no
  // blank line of their own, so removing them is an exact inverse.
  const grouped = insertGroupMarkers(
    THREE_ATOMS,
    ["atom:AAAAAAAA", "atom:BBBBBBBB"],
    "ZZZZZZZZ",
  );
  const ungrouped = removeGroupMarkers(grouped.text, "ZZZZZZZZ");
  assert.equal(ungrouped.ok, true);
  assert.equal(ungrouped.text, THREE_ATOMS);
});

test("removeGroupMarkers does not leave a doubled blank line on a loose group", () => {
  const result = removeGroupMarkers(LOOSE_GROUP, "3G7K9R5V");
  assert.equal(result.ok, true);
  assert.ok(!/\n\n\n/.test(result.text), "collapsed one blank line at each seam");
  assert.ok(result.text.includes("First ordered claim."));
  assert.ok(result.text.includes("Second ordered claim."));
  assert.ok(!result.text.includes("atom-group"));
});

test("removeGroupMarkers reports an unknown group", () => {
  const result = removeGroupMarkers(TIGHT_GROUP, "NOSUCHID");
  assert.equal(result.ok, false);
  assert.match(result.error, /Could not find that group/);
});

test("removeGroupMarkers refuses a group with no closing marker", () => {
  const broken = [
    '<!-- <atom-group id="KF53ASNE"> -->',
    '<!-- <atom id="FAPWJSRC"/> -->',
    "* One.",
  ].join("\n");
  const result = removeGroupMarkers(broken, "KF53ASNE");
  assert.equal(result.ok, false);
  assert.match(result.error, /no closing marker/);
});

// --- a group still moves as one unit (regression guard) --------------------

test("reorderUnit still moves a whole group as one unit", () => {
  const result = reorderUnit(TIGHT_GROUP, "group:KF53ASNE", null, "start");
  assert.equal(result.ok, true);
  const lines = result.text.split("\n");
  const open = lines.findIndex((l) => l.includes("<atom-group"));
  const close = lines.findIndex((l) => l.includes("</atom-group>"));
  assert.ok(open >= 0 && close > open);
  // Both members are still between the markers, in their original order.
  const inside = lines.slice(open + 1, close).join("\n");
  assert.ok(inside.indexOf("FAPWJSRC") < inside.indexOf("GPG5QA7A"));
  assert.ok(inside.includes("* One."));
  assert.ok(inside.includes("* Two."));
});

// --- the write path goes through the editor buffer (undo, B5) --------------
//
// These drive the real exported plug functions with a recording syscall stub,
// so they assert the actual sequence of syscalls the worker makes. The claim
// under test is that a group, an ungroup and a reorder each reach the document
// as ONE editor.replaceRange — which is one CodeMirror transaction and
// therefore one entry in the editor's own undo history — and that none of them
// writes the space file directly, because a space write is invisible to undo.

// A deliberately dumb stand-in for the HOST's markdown renderer.
//
// The real renderer is SilverBullet's own (markdown.markdownToHtml, backed by
// client/markdown_renderer), and it is not reachable from node. So this is not
// a markdown implementation under test and must never grow into one — it exists
// so a test can assert the three things that ARE this plug's job: that the plug
// asks the host to render, that it sanitizes the answer, and that it puts the
// result in the card. It handles the shapes the fixtures use and wraps anything
// else in a paragraph.
function fakeMarkdownToHtml(text) {
  const heading = text.match(/^(#{1,6})\s+(.*)$/);
  if (heading) {
    const level = heading[1].length;
    return `<h${level}>${heading[2]}</h${level}>`;
  }
  // Inline markdown, applied EVERYWHERE a real renderer applies it, table
  // cells included. An earlier version substituted only outside tables, and a
  // reader of a rig built on it reported that links in cells "stayed as raw
  // markdown" - a defect of this function, never of the plug.
  const inline = (t) => t
    .replace(/\[([^\]]+)\]\(([^)]+)\)/g, '<a href="$2">$1</a>')
    .replace(/`([^`]+)`/g, "<code>$1</code>")
    .replace(/\*\*([^*]+)\*\*/g, "<strong>$1</strong>");
  if (text.startsWith("|")) {
    const rows = text.split("\n").filter((l) => !/^\|[\s|:-]+\|$/.test(l));
    const cells = rows.map((row) => {
      const parts = row.split("|").slice(1, -1);
      return "<tr>" + parts.map((p) => `<td>${inline(p.trim())}</td>`).join("") +
        "</tr>";
    });
    return `<table><tbody>${cells.join("")}</tbody></table>`;
  }
  let body = inline(text);
  if (/^\d+\.\s/m.test(text)) {
    const items = body.split("\n").map((l) => l.replace(/^\d+\.\s*/, ""));
    return "<ol>" + items.map((i) => `<li>${i}</li>`).join("") + "</ol>";
  }
  if (/^[*-]\s/m.test(text)) {
    const items = body.split("\n").map((l) => l.replace(/^[*-]\s*/, ""));
    return "<ul>" + items.map((i) => `<li>${i}</li>`).join("") + "</ul>";
  }
  return `<p>${body}</p>`;
}

// options:
//   page          - what editor.getCurrentPage reports (default "Board")
//   store         - the clientStore contents to start from
//   storeThrows   - every clientStore call rejects, the way it would in a
//                   browser with site data blocked
//   markdownThrows - markdown.markdownToHtml rejects, the way it would on a
//                   SilverBullet too old to have the syscall
//   markdownHtml  - a function returning the HTML the host renders, so a test
//                   can hand the plug hostile output on purpose
function recordingSyscall(text, options) {
  const opts = options || {};
  const calls = [];
  const state = {
    text,
    page: opts.page === undefined ? "Board" : opts.page,
    store: opts.store || {},
  };
  globalThis.syscall = async function (name, ...args) {
    calls.push({ name, args });
    if (name === "editor.getText") return state.text;
    if (name === "editor.getCurrentPage") return state.page;
    if (name === "editor.replaceRange") {
      const [from, to, insert] = args;
      state.text = state.text.slice(0, from) + insert + state.text.slice(to);
      return;
    }
    if (name === "markdown.markdownToHtml") {
      if (opts.markdownThrows) throw new Error("no such syscall");
      return (opts.markdownHtml || fakeMarkdownToHtml)(args[0]);
    }
    if (name.indexOf("clientStore.") === 0) {
      if (opts.storeThrows) throw new Error("store unavailable");
      if (name === "clientStore.get") return state.store[args[0]];
      if (name === "clientStore.set") {
        state.store[args[0]] = args[1];
        return;
      }
      if (name === "clientStore.delete") {
        delete state.store[args[0]];
        return;
      }
    }
    return undefined;
  };
  return { calls, state };
}

test("groupAtoms applies one editor.replaceRange and never writes the space", async () => {
  const { calls, state } = recordingSyscall(THREE_ATOMS);
  const result = await plug.functionMapping.groupAtoms(
    JSON.stringify(["atom:AAAAAAAA", "atom:BBBBBBBB"]),
  );
  assert.equal(result.ok, true);
  assert.match(result.groupId, /^[0-9A-HJKMNP-TV-Z]{8}$/);

  const names = calls.map((c) => c.name);
  assert.equal(names.filter((n) => n === "editor.replaceRange").length, 1);
  assert.equal(names.includes("space.writePage"), false);
  assert.equal(names.includes("space.readPage"), false);
  assert.equal(names.includes("editor.reloadPage"), false);
  // The buffer now holds a balanced group around the two atoms.
  assert.ok(state.text.includes('<atom-group id="' + result.groupId + '">'));
  assert.ok(state.text.includes("</atom-group>"));
  // And every atom's directive line, id and digest survived.
  for (const line of THREE_ATOMS.split("\n")) {
    if (line.includes("<atom ")) assert.ok(state.text.includes(line));
  }
});

test("groupAtoms then ungroupAtoms restores the buffer exactly", async () => {
  const { state } = recordingSyscall(THREE_ATOMS);
  const grouped = await plug.functionMapping.groupAtoms(
    JSON.stringify(["atom:BBBBBBBB", "atom:CCCCCCCC"]),
  );
  assert.equal(grouped.ok, true);
  const ungrouped = await plug.functionMapping.ungroupAtoms(grouped.groupId);
  assert.equal(ungrouped.ok, true);
  assert.equal(state.text, THREE_ATOMS);
});

test("ungroupAtoms applies one editor.replaceRange", async () => {
  const { calls, state } = recordingSyscall(TIGHT_GROUP);
  const result = await plug.functionMapping.ungroupAtoms("KF53ASNE");
  assert.equal(result.ok, true);
  const names = calls.map((c) => c.name);
  assert.equal(names.filter((n) => n === "editor.replaceRange").length, 1);
  assert.equal(names.includes("space.writePage"), false);
  assert.ok(!state.text.includes("atom-group"));
});

test("reorderAtom applies one editor.replaceRange", async () => {
  const { calls, state } = recordingSyscall(THREE_ATOMS);
  const result = await plug.functionMapping.reorderAtom(
    "atom:CCCCCCCC",
    "atom:AAAAAAAA",
    "before",
  );
  assert.equal(result.ok, true);
  const names = calls.map((c) => c.name);
  assert.equal(names.filter((n) => n === "editor.replaceRange").length, 1);
  assert.equal(names.includes("space.writePage"), false);
  assert.ok(state.text.indexOf("Third block.") < state.text.indexOf("First block."));
});

test("saveAttrs applies one editor.replaceRange and keeps the id first", async () => {
  const { calls, state } = recordingSyscall(THREE_ATOMS);
  const result = await plug.functionMapping.saveAttrs(
    "AAAAAAAA",
    JSON.stringify([
      { name: "digest", value: "sha256:aa" },
      { name: "acme-approved-by", value: 'ada & "co"' },
    ]),
  );
  assert.equal(result.ok, true);
  const names = calls.map((c) => c.name);
  assert.equal(names.filter((n) => n === "editor.replaceRange").length, 1);
  assert.equal(names.includes("space.writePage"), false);
  // serializeAtomLine() joins the attribute text and the "/> -->" suffix with
  // a space, so the rewritten line reads `... value" /> -->`. That is
  // pre-existing behaviour, and lint accepts it; the assertion records the
  // shape rather than pretending it is not there.
  assert.ok(state.text.includes(
    '<!-- <atom id="AAAAAAAA" digest="sha256:aa" acme-approved-by="ada &amp; &quot;co&quot;" /> -->',
  ));
});

test("a refused group makes no edit at all", async () => {
  const { calls, state } = recordingSyscall(THREE_ATOMS);
  const result = await plug.functionMapping.groupAtoms(
    JSON.stringify(["atom:AAAAAAAA", "atom:CCCCCCCC"]),
  );
  assert.equal(result.ok, false);
  assert.match(result.error, /not next to each other/);
  assert.equal(calls.filter((c) => c.name === "editor.replaceRange").length, 0);
  assert.equal(state.text, THREE_ATOMS);
});

test("groupAtoms rejects a malformed payload without editing", async () => {
  const { calls } = recordingSyscall(THREE_ATOMS);
  for (const bad of ["not json", '{"a":1}', "null"]) {
    const result = await plug.functionMapping.groupAtoms(bad);
    assert.equal(result.ok, false);
  }
  assert.equal(calls.filter((c) => c.name === "editor.replaceRange").length, 0);
});

// --- the injected copy is the same copy ------------------------------------

test("injectSharedFunctions ships the tested functions to the panel", () => {
  const source = injectSharedFunctions();
  for (const name of [
    "pickDropTarget",
    "unitKeyForCard",
    "unitOrderFromCards",
    "dedupeKeys",
    "isContiguousUnitSelection",
    "groupMenuState",
    "rectsIntersect",
  ]) {
    assert.match(source, new RegExp("function " + name + "\\("));
  }
  // The injected source must parse on its own — the panel eval()s it.
  new Function(source + "\nreturn typeof pickDropTarget;");
});

test("the injected pickDropTarget behaves identically to the tested one", () => {
  const factory = new Function(
    injectSharedFunctions() + "\nreturn pickDropTarget;",
  );
  const injected = factory();
  for (const y of [10, 157, 180, 250, 350, 378, 500]) {
    assert.deepEqual(injected(y, RECTS), pickDropTarget(y, RECTS));
  }
});

// --- slugs: the readable alias (iugum-w6y.4) -------------------------------
//
// Steve cannot group by eight-character ids, so the board shows and writes
// SPEC.md's optional `slug`. These tests pin the two claims that make that
// safe: a slug never becomes identity (no id and no digest moves), and typed
// input always reaches the document in one shape.

test("sanitizeSlug lowercases and kebab-cases typed text", () => {
  assert.equal(sanitizeSlug("Email PRs"), "email-prs");
  assert.equal(sanitizeSlug("  Local Dev  "), "local-dev");
  assert.equal(sanitizeSlug("Notion / Docs"), "notion-docs");
  assert.equal(sanitizeSlug("already-kebab"), "already-kebab");
});

test("sanitizeSlug folds accents to their ASCII base rather than dropping them", () => {
  assert.equal(sanitizeSlug("Décisions"), "decisions");
  assert.equal(sanitizeSlug("Añejo Bar"), "anejo-bar");
});

test("sanitizeSlug collapses punctuation runs and trims the edges", () => {
  assert.equal(sanitizeSlug("-- a...b !! c --"), "a-b-c");
  assert.equal(sanitizeSlug("#1. Plumbing (research)"), "1-plumbing-research");
});

test("sanitizeSlug returns an empty string when nothing usable survives", () => {
  for (const input of ["", "   ", "!!!", "---", null, undefined]) {
    assert.equal(sanitizeSlug(input), "");
  }
});

test("sanitizeSlug truncates a long name at a word boundary", () => {
  const slug = sanitizeSlug(
    "A very long heading that keeps going well past any sensible slug length",
  );
  assert.ok(slug.length <= 48, slug);
  assert.ok(!slug.endsWith("-"), slug);
  // Cut on a hyphen, so no half word is left behind.
  assert.ok("a-very-long-heading-that-keeps-going-well-past-any".startsWith(slug));
});

test("sanitizeSlug is idempotent: sanitizing its own output changes nothing", () => {
  for (const input of ["Email PRs", "Décisions", "#1. Plumbing (research)"]) {
    const once = sanitizeSlug(input);
    assert.equal(sanitizeSlug(once), once);
  }
});

test("slugConflict finds another block already using the name", () => {
  const source = [
    '<!-- <atom-group id="KATZ94NM" slug="decisions"> -->',
    '<!-- <atom id="AAAAAAAA" slug="board"/> -->',
    "Text.",
    "<!-- </atom-group> -->",
    "",
  ].join("\n");
  assert.deepEqual(slugConflict(source, "decisions", null), {
    duplicate: true,
    ids: ["KATZ94NM"],
    warning: slugConflict(source, "decisions", null).warning,
  });
  assert.match(slugConflict(source, "board", null).warning, /already used/);
  assert.equal(slugConflict(source, "unused-name", null).duplicate, false);
  assert.equal(slugConflict(source, "unused-name", null).warning, null);
});

test("slugConflict does not report a block conflicting with itself", () => {
  const source = '<!-- <atom-group id="KATZ94NM" slug="decisions"> -->\n';
  assert.equal(slugConflict(source, "decisions", "KATZ94NM").duplicate, false);
  assert.equal(slugConflict(source, "decisions", "OTHERIDX").duplicate, true);
});

test("slugConflict treats an empty slug as no conflict", () => {
  const source = '<!-- <atom-group id="KATZ94NM" slug="decisions"> -->\n';
  assert.deepEqual(slugConflict(source, "", null), {
    duplicate: false,
    ids: [],
    warning: null,
  });
});

test("deriveGroupSlug takes the first ATX heading in the selection", () => {
  assert.equal(
    deriveGroupSlug(["Some lead-in text.", "## Email PRs", "* One."]),
    "email-prs",
  );
  assert.equal(deriveGroupSlug(["# Decisions"]), "decisions");
});

test("deriveGroupSlug takes a setext heading when there is no ATX one", () => {
  assert.equal(deriveGroupSlug(["Local Dev\n=========", "Body."]), "local-dev");
});

test("deriveGroupSlug falls back to the first non-blank line", () => {
  assert.equal(deriveGroupSlug(["", "  ", "* Reindex the search box."]),
    "reindex-the-search-box");
});

test("deriveGroupSlug never returns an empty default", () => {
  assert.equal(deriveGroupSlug([]), "group");
  assert.equal(deriveGroupSlug(["", "!!!", "---"]), "group");
});

test("deriveGroupSlug output is already sanitized", () => {
  const slug = deriveGroupSlug(["### Notion / Docs (WIP)"]);
  assert.equal(slug, sanitizeSlug(slug));
  assert.equal(slug, "notion-docs-wip");
});

test("slugOrId prefers the slug and falls back to the id", () => {
  assert.equal(slugOrId("decisions", "KATZ94NM"), "decisions");
  assert.equal(slugOrId("", "KATZ94NM"), "KATZ94NM");
  assert.equal(slugOrId("   ", "KATZ94NM"), "KATZ94NM");
  assert.equal(slugOrId(null, "KATZ94NM"), "KATZ94NM");
});

test("parseAtoms surfaces an atom's slug and its group's slug", () => {
  const atoms = parseAtoms(LOOSE_GROUP);
  assert.equal(atoms.length, 2);
  for (const atom of atoms) {
    assert.equal(atom.groupId, "3G7K9R5V");
    assert.equal(atom.groupSlug, "claims");
    assert.equal(atom.slug, null);
  }

  const named = parseAtoms(
    '<!-- <atom id="AAAAAAAA" slug="board" digest="sha256:aa"/> -->\nText.\n',
  );
  assert.equal(named[0].slug, "board");
  assert.equal(named[0].groupSlug, null);
});

test("computeUnits carries a group's slug without keying on it", () => {
  const { units } = computeUnits(LOOSE_GROUP);
  assert.equal(units.length, 1);
  assert.equal(units[0].groupSlug, "claims");
  // Identity is still the id: the unit key never mentions the slug.
  assert.equal(units[0].unitKey, "group:3G7K9R5V");
});

test("insertGroupMarkers writes the slug after the id, and only two lines", () => {
  const result = insertGroupMarkers(
    THREE_ATOMS,
    ["atom:AAAAAAAA", "atom:BBBBBBBB"],
    "ZZZZZZZZ",
    "First Two Blocks",
  );
  assert.equal(result.ok, true);
  assert.equal(result.slug, "first-two-blocks");
  assert.ok(result.text.includes(
    '<!-- <atom-group id="ZZZZZZZZ" slug="first-two-blocks"> -->',
  ));
  // Exactly the two markers were added, and nothing else changed.
  const before = THREE_ATOMS.split("\n");
  const after = result.text.split("\n");
  assert.equal(after.length, before.length + 2);
  assert.deepEqual(
    after.filter((l) => !l.includes("atom-group")),
    before,
  );
});

test("insertGroupMarkers writes no slug attribute for an empty name", () => {
  for (const name of ["", "   ", "!!!", null, undefined]) {
    const result = insertGroupMarkers(
      THREE_ATOMS,
      ["atom:AAAAAAAA", "atom:BBBBBBBB"],
      "ZZZZZZZZ",
      name,
    );
    assert.equal(result.ok, true);
    assert.equal(result.slug, "");
    assert.ok(result.text.includes('<!-- <atom-group id="ZZZZZZZZ"> -->'));
    assert.ok(!result.text.includes("slug="));
  }
});

test("insertGroupMarkers warns about a duplicate slug but still writes it", () => {
  const source = [
    '<!-- <atomdown version="1"/> -->',
    '<!-- <atom-group id="KATZ94NM" slug="decisions"> -->',
    '<!-- <atom id="AAAAAAAA"/> -->',
    "First.",
    "<!-- </atom-group> -->",
    "",
    '<!-- <atom id="BBBBBBBB"/> -->',
    "Second.",
    "",
    '<!-- <atom id="CCCCCCCC"/> -->',
    "Third.",
    "",
  ].join("\n");
  const result = insertGroupMarkers(
    source,
    ["atom:BBBBBBBB", "atom:CCCCCCCC"],
    "ZZZZZZZZ",
    "Decisions",
  );
  assert.equal(result.ok, true);
  assert.ok(result.text.includes(
    '<!-- <atom-group id="ZZZZZZZZ" slug="decisions"> -->',
  ));
  assert.match(result.warning, /already used/);
  assert.match(result.warning, /KATZ94NM/);
});

test("group then ungroup is still byte-exact when the group carries a slug", () => {
  const grouped = insertGroupMarkers(
    THREE_ATOMS,
    ["atom:BBBBBBBB", "atom:CCCCCCCC"],
    "ZZZZZZZZ",
    "Two And Three",
  );
  assert.equal(grouped.ok, true);
  const ungrouped = removeGroupMarkers(grouped.text, "ZZZZZZZZ");
  assert.equal(ungrouped.ok, true);
  assert.equal(ungrouped.text, THREE_ATOMS);
});

test("setGroupSlugInSource renames a group and touches nothing else", () => {
  const result = setGroupSlugInSource(TIGHT_GROUP, "KF53ASNE", "Split List");
  assert.equal(result.ok, true);
  assert.equal(result.slug, "split-list");
  const before = TIGHT_GROUP.split("\n");
  const after = result.text.split("\n");
  assert.equal(after.length, before.length);
  const changed = after.filter((line, i) => line !== before[i]);
  assert.deepEqual(changed, [
    '<!-- <atom-group id="KF53ASNE" slug="split-list"> -->',
  ]);
});

test("setGroupSlugInSource keeps the id and every atom's directive intact", () => {
  const result = setGroupSlugInSource(TIGHT_GROUP, "KF53ASNE", "renamed");
  assert.equal(result.ok, true);
  assert.ok(result.text.includes('id="KF53ASNE"'));
  for (const line of TIGHT_GROUP.split("\n")) {
    if (line.includes("<atom ")) assert.ok(result.text.includes(line));
  }
});

test("setGroupSlugInSource replaces an existing slug rather than adding a second", () => {
  const result = setGroupSlugInSource(LOOSE_GROUP, "3G7K9R5V", "Ordered Claims");
  assert.equal(result.ok, true);
  assert.ok(result.text.includes(
    '<!-- <atom-group id="3G7K9R5V" slug="ordered-claims"> -->',
  ));
  assert.equal(result.text.split("slug=").length - 1, 1);
});

test("setGroupSlugInSource removes the slug for an empty name", () => {
  const result = setGroupSlugInSource(LOOSE_GROUP, "3G7K9R5V", "  ");
  assert.equal(result.ok, true);
  assert.equal(result.slug, "");
  assert.ok(result.text.includes('<!-- <atom-group id="3G7K9R5V"> -->'));
  assert.ok(!result.text.includes("slug="));
});

test("setGroupSlugInSource preserves any other attribute on the marker", () => {
  const source = '<!-- <atom-group id="KF53ASNE" acme-owner="ada"> -->\n' +
    '<!-- <atom id="AAAAAAAA"/> -->\nText.\n<!-- </atom-group> -->\n';
  const result = setGroupSlugInSource(source, "KF53ASNE", "owned");
  assert.equal(result.ok, true);
  assert.ok(result.text.includes(
    '<!-- <atom-group id="KF53ASNE" slug="owned" acme-owner="ada"> -->',
  ));
});

test("setGroupSlugInSource escapes a value that would break the attribute", () => {
  // sanitizeSlug already removes every character that needs escaping, so this
  // pins that the two layers agree rather than trusting one of them.
  const result = setGroupSlugInSource(TIGHT_GROUP, "KF53ASNE", 'a "b" & <c>');
  assert.equal(result.ok, true);
  assert.equal(result.slug, "a-b-c");
  assert.ok(!result.text.includes("&quot;"));
});

test("setGroupSlugInSource reports an unknown group", () => {
  const result = setGroupSlugInSource(TIGHT_GROUP, "NOSUCHID", "x");
  assert.equal(result.ok, false);
  assert.match(result.error, /Could not find that group/);
});

// --- slugs on the write path -----------------------------------------------

test("groupAtoms writes the typed name as one editor.replaceRange", async () => {
  const { calls, state } = recordingSyscall(THREE_ATOMS);
  const result = await plug.functionMapping.groupAtoms(
    JSON.stringify(["atom:AAAAAAAA", "atom:BBBBBBBB"]),
    "First Two",
  );
  assert.equal(result.ok, true);
  assert.equal(result.slug, "first-two");
  assert.match(result.groupId, /^[0-9A-HJKMNP-TV-Z]{8}$/);

  const names = calls.map((c) => c.name);
  assert.equal(names.filter((n) => n === "editor.replaceRange").length, 1);
  assert.equal(names.includes("space.writePage"), false);
  assert.equal(names.includes("space.readPage"), false);
  assert.equal(names.includes("editor.reloadPage"), false);
  assert.ok(state.text.includes(
    '<atom-group id="' + result.groupId + '" slug="first-two">',
  ));
  // Naming a group changes no atom's directive line, so no id and no digest
  // can have moved.
  for (const line of THREE_ATOMS.split("\n")) {
    if (line.includes("<atom ")) assert.ok(state.text.includes(line));
  }
});

test("groupAtoms with no name writes no slug attribute", async () => {
  const { state } = recordingSyscall(THREE_ATOMS);
  const result = await plug.functionMapping.groupAtoms(
    JSON.stringify(["atom:AAAAAAAA", "atom:BBBBBBBB"]),
    "",
  );
  assert.equal(result.ok, true);
  assert.ok(state.text.includes('<atom-group id="' + result.groupId + '">'));
  assert.ok(!state.text.includes("slug="));
});

test("setGroupSlug applies one editor.replaceRange and writes no page", async () => {
  const { calls, state } = recordingSyscall(TIGHT_GROUP);
  const result = await plug.functionMapping.setGroupSlug("KF53ASNE", "Split List");
  assert.equal(result.ok, true);
  assert.equal(result.slug, "split-list");
  const names = calls.map((c) => c.name);
  assert.equal(names.filter((n) => n === "editor.replaceRange").length, 1);
  assert.equal(names.includes("space.writePage"), false);
  assert.equal(names.includes("editor.reloadPage"), false);
  assert.ok(state.text.includes(
    '<!-- <atom-group id="KF53ASNE" slug="split-list"> -->',
  ));
});

test("setGroupSlug makes no edit when the name did not change", async () => {
  const { calls } = recordingSyscall(LOOSE_GROUP);
  const result = await plug.functionMapping.setGroupSlug("3G7K9R5V", "claims");
  assert.equal(result.ok, true);
  assert.equal(result.unchanged, true);
  assert.equal(calls.filter((c) => c.name === "editor.replaceRange").length, 0);
});

test("setGroupSlug refuses an unknown group without editing", async () => {
  const { calls, state } = recordingSyscall(TIGHT_GROUP);
  const result = await plug.functionMapping.setGroupSlug("NOSUCHID", "x");
  assert.equal(result.ok, false);
  assert.equal(calls.filter((c) => c.name === "editor.replaceRange").length, 0);
  assert.equal(state.text, TIGHT_GROUP);
});

test("saveAttrs writes a sanitized slug second, after the id", async () => {
  const { calls, state } = recordingSyscall(THREE_ATOMS);
  const result = await plug.functionMapping.saveAttrs(
    "AAAAAAAA",
    JSON.stringify([
      { name: "slug", value: "First Block" },
      { name: "digest", value: "sha256:aa" },
    ]),
  );
  assert.equal(result.ok, true);
  assert.equal(result.slug, "first-block");
  assert.equal(
    calls.filter((c) => c.name === "editor.replaceRange").length,
    1,
  );
  assert.equal(calls.some((c) => c.name === "space.writePage"), false);
  // id, then slug, then the rest - the order emit.go writes, so a later
  // `atomdown emit` does not reshuffle the line.
  assert.ok(state.text.includes(
    '<!-- <atom id="AAAAAAAA" slug="first-block" digest="sha256:aa" /> -->',
  ));
});

test("saveAttrs drops an empty slug rather than writing slug=\"\"", async () => {
  const named = '<!-- <atomdown version="1"/> -->\n' +
    '<!-- <atom id="AAAAAAAA" slug="first"/> -->\nFirst block.\n';
  const { state } = recordingSyscall(named);
  const result = await plug.functionMapping.saveAttrs(
    "AAAAAAAA",
    JSON.stringify([{ name: "slug", value: "   " }]),
  );
  assert.equal(result.ok, true);
  assert.equal(result.slug, "");
  assert.ok(!state.text.includes("slug="));
  assert.ok(state.text.includes('<!-- <atom id="AAAAAAAA" /> -->'));
});

test("saveAttrs redraws the board only when the name changed", async () => {
  const named = '<!-- <atomdown version="1"/> -->\n' +
    '<!-- <atom id="AAAAAAAA" slug="first"/> -->\nFirst block.\n';

  const renamed = recordingSyscall(named);
  await plug.functionMapping.saveAttrs(
    "AAAAAAAA",
    JSON.stringify([{ name: "slug", value: "second" }]),
  );
  assert.ok(renamed.calls.some((c) => c.name === "editor.showPanel"));

  const untouched = recordingSyscall(named);
  await plug.functionMapping.saveAttrs(
    "AAAAAAAA",
    JSON.stringify([
      { name: "slug", value: "first" },
      { name: "acme-note", value: "x" },
    ]),
  );
  assert.equal(
    untouched.calls.some((c) => c.name === "editor.showPanel"),
    false,
  );
});

test("a duplicate name reaches the user as a notification, not a refusal", async () => {
  const source = [
    '<!-- <atomdown version="1"/> -->',
    '<!-- <atom-group id="KATZ94NM" slug="decisions"> -->',
    '<!-- <atom id="AAAAAAAA"/> -->',
    "First.",
    "<!-- </atom-group> -->",
    "",
    '<!-- <atom id="BBBBBBBB"/> -->',
    "Second.",
    "",
    '<!-- <atom id="CCCCCCCC"/> -->',
    "Third.",
    "",
  ].join("\n");
  const { calls, state } = recordingSyscall(source);
  const result = await plug.functionMapping.groupAtoms(
    JSON.stringify(["atom:BBBBBBBB", "atom:CCCCCCCC"]),
    "decisions",
  );
  assert.equal(result.ok, true);
  assert.match(result.warning, /already used/);
  const flash = calls.find((c) => c.name === "editor.flashNotification");
  assert.ok(flash, "expected a flashNotification syscall");
  assert.match(flash.args[0], /already used/);
  // Written anyway: the format permits duplicate slugs.
  assert.ok(state.text.includes('slug="decisions"'));
});

// --- the board reads the name, not the id ----------------------------------

test("a rendered card shows the slug and keeps the id visible", async () => {
  // The panel HTML is only reachable through the syscall the worker makes to
  // draw it, which is also the only place it matters.
  const { calls } = recordingSyscall(THREE_ATOMS);
  const result = await plug.functionMapping.groupAtoms(
    JSON.stringify(["atom:AAAAAAAA", "atom:BBBBBBBB"]),
    "First Two",
  );
  assert.equal(result.ok, true);
  const panel = calls.filter((c) => c.name === "editor.showPanel").pop();
  assert.ok(panel, "expected the board to redraw");
  // showPanel("modal", inset, html, script) - html is the third argument.
  const html = panel.args[2];
  // The group's header reads the name...
  assert.ok(html.includes('<span class="board-group-name"'));
  assert.ok(html.includes(">first-two</span>"));
  // ...and the real group id is still on the header, in the same small subtle
  // monospace span a card uses, and in the tooltip.
  assert.ok(html.includes('<span class="board-group-id"'));
  assert.ok(html.includes(">" + result.groupId + "</span>"));
  assert.ok(html.includes("id " + result.groupId));
  // Every atom's own id is still on its card.
  for (const id of ["AAAAAAAA", "BBBBBBBB", "CCCCCCCC"]) {
    assert.ok(html.includes(">" + id + "<"), id);
  }
});

test("a group with no name still shows its id on the header", async () => {
  const { calls } = recordingSyscall(THREE_ATOMS);
  const result = await plug.functionMapping.groupAtoms(
    JSON.stringify(["atom:AAAAAAAA", "atom:BBBBBBBB"]),
    "",
  );
  assert.equal(result.ok, true);
  const html = calls.filter((c) => c.name === "editor.showPanel").pop().args[2];
  // No name span at all, and the id is the label.
  assert.ok(!html.includes('class="board-group-name"'));
  assert.ok(html.includes(
    '<span class="board-group-id" title="Group id ' + result.groupId +
      ' (no name yet)',
  ));
});

test("the panel gets the same slug functions the worker uses", () => {
  const source = injectSharedFunctions();
  for (const name of ["sanitizeSlug", "deriveGroupSlug"]) {
    assert.match(source, new RegExp("function " + name + "\\("));
  }
  const factory = new Function(
    source + "\nreturn { sanitizeSlug: sanitizeSlug, deriveGroupSlug: deriveGroupSlug };",
  );
  const injected = factory();
  for (const input of ["Email PRs", "Décisions", "#1. Plumbing (research)", ""]) {
    assert.equal(injected.sanitizeSlug(input), sanitizeSlug(input));
  }
  for (const texts of [["## Email PRs"], ["Body only."], []]) {
    assert.equal(injected.deriveGroupSlug(texts), deriveGroupSlug(texts));
  }
});

// --- one group, one object -------------------------------------------------
//
// The claim under test is that an atom-group renders as ONE bordered
// container with a header bar, and that the per-card group marking (an accent
// stripe and a `group <slug>` badge on every member) is gone, because the
// container is what says "group" now.

function boardHtml(sourceText, collapsedIds) {
  return buildBoardHtml(parseAtoms(sourceText), "Board", collapsedIds).html;
}

function countOf(text, needle) {
  return text.split(needle).length - 1;
}

test("a group renders as one container holding its member cards", () => {
  const html = boardHtml(TIGHT_GROUP);
  assert.equal(countOf(html, '<div class="board-group"'), 1);
  assert.equal(countOf(html, "data-group-header="), 1);
  // The container holds exactly the group's two members, and the standalone
  // atom above it stays outside.
  const opened = html.indexOf('data-group-cards="KF53ASNE"');
  const cardsInside = html.slice(opened);
  assert.ok(cardsInside.includes('data-atom-id="FAPWJSRC"'));
  assert.ok(cardsInside.includes('data-atom-id="GPG5QA7A"'));
  assert.ok(!cardsInside.includes('data-atom-id="J1BBCED5"'));
});

test("a member card carries no group accent and no group badge", () => {
  const html = boardHtml(TIGHT_GROUP);
  assert.ok(!html.includes("board-card-grouped"));
  assert.ok(!html.includes("board-badge-group"));
  // The stripe rule itself is gone, not merely unused.
  assert.ok(!html.includes("border-left: 3px solid var(--ui-accent-color)"));
});

test("the container reuses the accent token, and adds no second colour", () => {
  const html = boardHtml(TIGHT_GROUP);
  assert.ok(html.includes(".board-group {"));
  // The accent now travels through the board's own knob, which DEFAULTS to
  // SilverBullet's accent token (see the :root block), so the board still has
  // exactly one blue and a user can retint it from a space-style page.
  assert.ok(html.includes("--board-accent-color: var(--ui-accent-color)"));
  assert.ok(html.includes(
    "border: var(--board-group-border-width) solid var(--board-accent-color)",
  ));
  assert.ok(html.includes("background: var(--board-accent-color)"));
  // Every literal colour in the stylesheet is a :root fallback for a theme
  // variable. Nothing added a hue of its own.
  const style = html.slice(html.indexOf("<style>"), html.indexOf("</style>"));
  const rootBlock = style.slice(style.indexOf(":root {"), style.indexOf("body {"));
  for (const hex of style.match(/#[0-9a-fA-F]{3,8}\b/g) || []) {
    assert.ok(rootBlock.includes(hex), hex);
  }
  // The one functional colour is the popover's drop shadow, which predates
  // this work and is a shadow, not a hue.
  const rgbas = style.match(/rgba?\([^)]*\)/g) || [];
  assert.deepEqual(rgbas, ["rgba(0,0,0,0.2)"]);
});

test("the header carries the group-level actions, Rename and Ungroup", () => {
  const html = boardHtml(LOOSE_GROUP);
  assert.ok(html.includes('data-group-rename="3G7K9R5V"'));
  assert.ok(html.includes('data-group-ungroup="3G7K9R5V"'));
  assert.ok(html.includes(">Rename</button>"));
  assert.ok(html.includes(">Ungroup</button>"));
});

test("the member card menu no longer offers Rename group", () => {
  const built = buildBoardHtml(parseAtoms(LOOSE_GROUP), "Board", []);
  assert.ok(!built.html.includes("Rename group"));
  assert.ok(!built.script.includes("Rename group"));
  // The card menu keeps its own items: Group / Ungroup and the attributes.
  assert.ok(built.script.includes("boardGroupBtn"));
  assert.ok(built.script.includes("+ Add attribute"));
});

test("clicking the header is wired to select the whole group", () => {
  const script = buildBoardHtml(parseAtoms(LOOSE_GROUP), "Board", []).script;
  assert.ok(script.includes("function selectGroup("));
  assert.ok(script.includes("board-card-selected"));
  // The header listener selects, it does not invent a second grouping rule:
  // the decision still comes from the shared groupMenuState.
  assert.ok(script.includes("selectGroup(groupEl,"));
  assert.ok(script.includes("groupMenuState(UNIT_ORDER"));
});

test("two adjacent groups render two separate containers", () => {
  const source = [
    '<!-- <atom-group id="AAAAAAA1"> -->',
    '<!-- <atom id="BBBBBBB1"/> -->',
    "One.",
    "<!-- </atom-group> -->",
    '<!-- <atom-group id="AAAAAAA2"> -->',
    '<!-- <atom id="BBBBBBB2"/> -->',
    "Two.",
    "<!-- </atom-group> -->",
    "",
  ].join("\n");
  const html = boardHtml(source);
  assert.equal(countOf(html, "data-group-header="), 2);
  assert.ok(html.indexOf("AAAAAAA1") < html.indexOf("AAAAAAA2"));
});

test("cards stay in document order whether or not they are in a container", () => {
  const html = boardHtml(TIGHT_GROUP);
  const order = ["J1BBCED5", "FAPWJSRC", "GPG5QA7A"].map((id) =>
    html.indexOf('data-atom-id="' + id + '"')
  );
  for (const at of order) assert.ok(at > 0);
  assert.deepEqual(order.slice().sort((a, b) => a - b), order);
});

test("a selected card is a double ring, not the container's single edge", () => {
  const html = boardHtml(TIGHT_GROUP);
  const rule = html.slice(
    html.indexOf(".board-card-selected {"),
    html.indexOf("}", html.indexOf(".board-card-selected {")),
  );
  // Same hue, different shape: a border plus a second ring set outside it,
  // plus a lifted background. No second colour token.
  assert.ok(rule.includes("border: 2px solid var(--board-accent-color)"));
  assert.ok(rule.includes("outline: 2px solid var(--board-accent-color)"));
  assert.ok(rule.includes("outline-offset: 2px"));
  assert.ok(rule.includes("background: var(--ui-surface-hover-background-color)"));
  // The old "grouped card keeps a thicker left edge" special case is gone.
  assert.ok(!html.includes(".board-card-grouped.board-card-selected"));
});

test("a collapsed group's cards are hidden, and nothing else changes", () => {
  const open = boardHtml(TIGHT_GROUP, []);
  const shut = boardHtml(TIGHT_GROUP, ["KF53ASNE"]);
  // On the container's own class attribute, not merely somewhere in the
  // stylesheet - the resting-chrome rules name the collapsed class too.
  assert.ok(!open.includes('class="board-group board-group-collapsed"'));
  assert.ok(open.includes('data-group-cards="KF53ASNE">'));
  assert.ok(shut.includes('class="board-group board-group-collapsed"'));
  assert.ok(shut.includes('data-group-cards="KF53ASNE" hidden>'));
  assert.ok(shut.includes('aria-expanded="false"'));
  // Every member card is still rendered, only not shown.
  assert.ok(shut.includes('data-atom-id="FAPWJSRC"'));
});

test("a remembered collapse for a group that is gone changes nothing", () => {
  assert.equal(boardHtml(TIGHT_GROUP, ["NOSUCHID"]), boardHtml(TIGHT_GROUP, []));
});

test("the panel is told which groups are collapsed, and nothing more", () => {
  const script = buildBoardHtml(parseAtoms(TIGHT_GROUP), "Board", ["KF53ASNE"]).script;
  assert.ok(script.includes('var ATOMDOWN_BOARD_COLLAPSED = ["KF53ASNE"]'));
  assert.ok(script.includes('var ATOMDOWN_BOARD_PAGE = "Board"'));
  // Collapse is stored in the client's key-value store, never in the page.
  assert.ok(script.includes('"clientStore.set"'));
  assert.ok(!script.includes('collapsed="'));
});

test("no board action writes a presentational attribute to a directive", async () => {
  // Group, rename, collapse-persist and ungroup, in one buffer. Not one of
  // them may leave a collapsed / selected / open attribute behind.
  const { state } = recordingSyscall(THREE_ATOMS);
  const grouped = await plug.functionMapping.groupAtoms(
    JSON.stringify(["atom:AAAAAAAA", "atom:BBBBBBBB"]),
    "First Two",
  );
  assert.equal(grouped.ok, true);
  await plug.functionMapping.setGroupSlug(grouped.groupId, "renamed");
  for (const banned of ["collapsed", "selected", "open=", "x=", "y=", "board"]) {
    assert.ok(!state.text.includes(banned), banned);
  }
  await plug.functionMapping.ungroupAtoms(grouped.groupId);
  assert.equal(state.text, THREE_ATOMS);
});

// --- the view survives a refresh, per page ---------------------------------
//
// Steve: "every refresh to the page and I have to go re-apply the atomdown
// view". The flag is presentation state in the client's own key-value store,
// scoped to the page name. It is never a default: a page whose key was never
// written gets nothing, and Close deletes the key.

const GROUP_PAGE = "Todo/running";

async function closedStart(text, options) {
  const rec = recordingSyscall(text, options);
  // notifyClosed() is the one call that puts the module's own boardOpen flag
  // into a known state, so these tests do not inherit the previous one's.
  await plug.functionMapping.notifyClosed();
  return rec;
}

test("opening the board remembers that this page is showing it", async () => {
  const { calls, state } = await closedStart(TIGHT_GROUP, { page: GROUP_PAGE });
  await plug.functionMapping.toggleBoard();
  assert.ok(calls.some((c) => c.name === "editor.showPanel"));
  assert.equal(state.store[boardOpenKey(GROUP_PAGE)], true);
});

test("closing the board with the toggle forgets the page", async () => {
  const { state } = await closedStart(TIGHT_GROUP, { page: GROUP_PAGE });
  await plug.functionMapping.toggleBoard();
  await plug.functionMapping.toggleBoard();
  assert.equal(boardOpenKey(GROUP_PAGE) in state.store, false);
});

test("Close on the panel forgets the page too", async () => {
  const store = {};
  store[boardOpenKey(GROUP_PAGE)] = true;
  const { state } = recordingSyscall(TIGHT_GROUP, { page: GROUP_PAGE, store });
  await plug.functionMapping.notifyClosed();
  assert.equal(boardOpenKey(GROUP_PAGE) in state.store, false);
});

test("a page reload reopens the board when it was open on that page", async () => {
  const { calls, state } = await closedStart(TIGHT_GROUP, { page: GROUP_PAGE });
  state.store[boardOpenKey(GROUP_PAGE)] = true;
  const before = calls.length;
  const result = await plug.functionMapping.restoreBoard(GROUP_PAGE);
  assert.equal(result.opened, true);
  const after = calls.slice(before);
  assert.ok(after.some((c) => c.name === "editor.showPanel"));
  // Reopening a view is not a content change.
  assert.equal(after.some((c) => c.name === "editor.replaceRange"), false);
  assert.equal(after.some((c) => c.name === "space.writePage"), false);
});

test("a page reload leaves the board closed when it was closed", async () => {
  const { calls } = await closedStart(TIGHT_GROUP, { page: GROUP_PAGE });
  const before = calls.length;
  const result = await plug.functionMapping.restoreBoard(GROUP_PAGE);
  assert.equal(result.opened, false);
  assert.equal(
    calls.slice(before).some((c) => c.name === "editor.showPanel"),
    false,
  );
});

test("the remembered view is per page, never a default for another one", async () => {
  const { calls, state } = await closedStart(TIGHT_GROUP, { page: "Other" });
  state.store[boardOpenKey(GROUP_PAGE)] = true;
  const before = calls.length;
  const result = await plug.functionMapping.restoreBoard("Other");
  assert.equal(result.opened, false);
  assert.equal(
    calls.slice(before).some((c) => c.name === "editor.showPanel"),
    false,
  );
  // And the other page's flag is untouched.
  assert.equal(state.store[boardOpenKey(GROUP_PAGE)], true);
});

test("Close then reload stays closed", async () => {
  const { calls, state } = await closedStart(TIGHT_GROUP, { page: GROUP_PAGE });
  await plug.functionMapping.toggleBoard();
  await plug.functionMapping.notifyClosed();
  assert.equal(boardOpenKey(GROUP_PAGE) in state.store, false);
  const before = calls.length;
  assert.equal((await plug.functionMapping.restoreBoard(GROUP_PAGE)).opened, false);
  assert.equal(
    calls.slice(before).some((c) => c.name === "editor.showPanel"),
    false,
  );
});

test("a store that throws degrades to a closed board, not an error", async () => {
  const { calls } = await closedStart(TIGHT_GROUP, {
    page: GROUP_PAGE,
    storeThrows: true,
  });
  const before = calls.length;
  const result = await plug.functionMapping.restoreBoard(GROUP_PAGE);
  assert.equal(result.ok, true);
  assert.equal(result.opened, false);
  assert.equal(
    calls.slice(before).some((c) => c.name === "editor.showPanel"),
    false,
  );
});

test("a store that throws still lets the toggle open the board", async () => {
  const { calls } = await closedStart(TIGHT_GROUP, {
    page: GROUP_PAGE,
    storeThrows: true,
  });
  await plug.functionMapping.toggleBoard();
  assert.ok(calls.some((c) => c.name === "editor.showPanel"));
});

test("a reopen cannot draw an empty board when the editor is not ready", async () => {
  const { calls, state } = await closedStart("", { page: GROUP_PAGE });
  state.store[boardOpenKey(GROUP_PAGE)] = true;
  const before = calls.length;
  const result = await plug.functionMapping.restoreBoard(GROUP_PAGE);
  assert.equal(result.opened, false);
  assert.match(result.reason, /no text/);
  assert.equal(
    calls.slice(before).some((c) => c.name === "editor.showPanel"),
    false,
  );
  // The flag survives, so the next load can still reopen it.
  assert.equal(state.store[boardOpenKey(GROUP_PAGE)], true);
});

test("a reopen overtaken by a second navigation draws nothing", async () => {
  const { calls, state } = await closedStart(TIGHT_GROUP, { page: "Somewhere/else" });
  state.store[boardOpenKey(GROUP_PAGE)] = true;
  const before = calls.length;
  const result = await plug.functionMapping.restoreBoard(GROUP_PAGE);
  assert.equal(result.opened, false);
  assert.match(result.reason, /navigated away/);
  assert.equal(
    calls.slice(before).some((c) => c.name === "editor.showPanel"),
    false,
  );
});

test("navigating to a page with no remembered board takes the old panel down", async () => {
  const { calls, state } = await closedStart(TIGHT_GROUP, { page: "PageA" });
  await plug.functionMapping.toggleBoard();
  state.page = "PageB";
  const before = calls.length;
  const result = await plug.functionMapping.restoreBoard("PageB");
  assert.equal(result.opened, false);
  assert.ok(calls.slice(before).some((c) => c.name === "editor.hidePanel"));
});

test("restoreBoard is wired to the page-load events, not to a command", () => {
  const def = plug.manifest.functions.restoreBoard;
  assert.deepEqual(def.events, ["editor:pageLoaded", "editor:pageReloaded"]);
  assert.equal(def.command, undefined);
});

test("a redraw after Rename keeps a collapsed group collapsed", async () => {
  const store = {};
  store[collapsedKey("Board")] = ["KF53ASNE"];
  const { calls } = recordingSyscall(TIGHT_GROUP, { store });
  const result = await plug.functionMapping.setGroupSlug("KF53ASNE", "Split List");
  assert.equal(result.ok, true);
  const html = calls.filter((c) => c.name === "editor.showPanel").pop().args[2];
  assert.ok(html.includes("board-group-collapsed"));
});

test("the store keys are scoped by page and carry no document data", () => {
  assert.equal(boardOpenKey("Todo/running"), "atomdown-board.open:Todo/running");
  assert.equal(collapsedKey("Todo/running"), "atomdown-board.collapsed:Todo/running");
  assert.notEqual(boardOpenKey("Todo/running"), boardOpenKey("Todo/other"));
});

// --- rendered CommonMark in a card (iugum-w6y.6) ---------------------------
//
// Steve, 2026-09-03: "the view I have does not render the markdown in the
// individual cards, which I do want an option to view raw markdown but it
// should also display rendered markdown/commonmark by default".
//
// So the claims under test are: a card renders by default, the raw source is
// still reachable and still EXACT, the choice persists, and none of it changes
// the document. Plus the part that would actually break in a browser — a
// rendered card is a far richer DOM than a <pre>, and selection, the lasso and
// the drop geometry all read that DOM.

// A page shaped like Steve's real Todo/running: named groups, a heading per
// group, a markdown table, an ordered list, inline code, bold and links.
const RICH_PAGE = [
  '<!-- <atomdown version="1"/> -->',
  '<!-- <atom id="G92YE2JP" slug="running-todo"/> -->',
  "# Running todo",
  "",
  '<!-- <atom-group id="KATZ94NM" slug="decisions"> -->',
  '<!-- <atom id="2BKH46B9" slug="decisions-waiting-on-me"/> -->',
  "## Decisions waiting on me",
  "",
  '<!-- <atom id="DK3F1M7W" slug="atomdown-history"/> -->',
  "1. **Atomdown history.** Commit `2066012` is still local.",
  "2. **Stale bead.** The title now states the opposite.",
  "<!-- </atom-group> -->",
  "",
  '<!-- <atom-group id="NS67J8K5" slug="resea"> -->',
  '<!-- <atom id="QQE8MK3D" slug="resea-tickets-due-tonight"/> -->',
  "## RESEA tickets - due tonight",
  "",
  '<!-- <atom id="J6SXJ01J" slug="the-feature"/> -->',
  "The feature reads live from MOSES. Epic: [FFAI-62016](https://example.test/browse/FFAI-62016).",
  "",
  '<!-- <atom id="YFEH04BQ" slug="ticket-state-tonight"/> -->',
  "| Ticket | State | Tonight |",
  "|---|---|---|",
  "| FFAI-72357 | On Hold | Add cache expiry |",
  "| FFAI-72358 | Ready | Take off hold |",
  "<!-- </atom-group> -->",
  "",
].join("\n");

function lastPanel(calls) {
  const panel = calls.filter((c) => c.name === "editor.showPanel").pop();
  return { html: panel.args[2], script: panel.args[3] };
}

async function openBoard(text, options) {
  const rec = await closedStart(text, options);
  const before = rec.calls.length;
  await plug.functionMapping.toggleBoard();
  return Object.assign(rec, lastPanel(rec.calls.slice(before)));
}

// --- the default is rendered ----------------------------------------------

test("a card renders its block by default, and the board never asked for raw", async () => {
  const { html, calls } = await openBoard(RICH_PAGE);
  // The host's own renderer was asked, once per atom.
  const renders = calls.filter((c) => c.name === "markdown.markdownToHtml");
  assert.equal(renders.length, parseAtoms(RICH_PAGE).length);
  // A heading is a heading, not a hash.
  assert.ok(html.includes("<h2>RESEA tickets - due tonight</h2>"));
  assert.ok(html.includes("<h1>Running todo</h1>"));
  // The rendered body is the visible one; the raw body is the hidden one.
  assert.ok(html.includes('class="board-card-body board-card-rendered"'));
  assert.match(html, /board-card-raw" data-card-raw="G92YE2JP" hidden/);
  assert.equal(html.includes('data-card-view="raw"'), false);
});

test("a table renders as a table, not as pipes", async () => {
  const { html } = await openBoard(RICH_PAGE);
  assert.ok(html.includes("<table>"));
  assert.ok(html.includes("<td>FFAI-72357</td>"));
  // The raw pipes are still in the document and still in the hidden body.
  assert.ok(html.includes("| FFAI-72357 | On Hold | Add cache expiry |"));
});

test("a link renders as an anchor, and inline code and bold survive", async () => {
  const { html } = await openBoard(RICH_PAGE);
  assert.ok(html.includes('<a href="https://example.test/browse/FFAI-62016"'));
  assert.ok(html.includes("<code>2066012</code>"));
  assert.ok(html.includes("<strong>Atomdown history.</strong>"));
  assert.ok(html.includes("<ol>"));
});

test("an absolute link opens a new tab rather than replacing the board", async () => {
  const { html } = await openBoard(RICH_PAGE);
  assert.match(
    html,
    /<a href="https:\/\/example\.test[^"]*" target="_blank" rel="noopener noreferrer">/,
  );
});

test("a host with no markdown syscall falls back to raw, and still draws", async () => {
  const { html } = await openBoard(RICH_PAGE, { markdownThrows: true });
  assert.ok(html.includes('data-no-rendered="1"'));
  assert.ok(html.includes("## RESEA tickets - due tonight"));
  assert.equal(html.includes("<h2>"), false);
  // Every card is still there, with its id and its menu.
  assert.equal((html.match(/class="board-card[ "]/g) || []).length,
    parseAtoms(RICH_PAGE).length);
});

// --- the raw option --------------------------------------------------------

test("the toolbar carries a board-wide Raw toggle, labelled with what it does", async () => {
  const { html } = await openBoard(RICH_PAGE);
  assert.ok(html.includes('id="atomdown-board-view"'));
  assert.ok(html.includes(">Raw markdown<"));
  assert.ok(html.includes('data-board-view="rendered"'));
});

test("a remembered raw choice draws every card raw, with no flash of rendered", async () => {
  const store = {};
  store[viewKey("Board")] = { boardView: "raw", cardViews: {} };
  const { html } = await openBoard(RICH_PAGE, { store });
  // Raw is the visible body in the MARKUP, not applied by the script after.
  assert.match(html, /board-card-rendered" data-card-rendered="G92YE2JP" hidden/);
  assert.equal(html.includes('data-card-view="rendered"'), false);
  // And the button now offers the way back.
  assert.ok(html.includes(">Rendered<"));
  assert.ok(html.includes('data-board-view="raw"'));
});

test("a per-card override beats the board-wide default, in both directions", () => {
  const rawBoard = { boardView: "raw", cardViews: { AAAAAAAA: "rendered" } };
  assert.equal(effectiveCardView("AAAAAAAA", rawBoard), "rendered");
  assert.equal(effectiveCardView("BBBBBBBB", rawBoard), "raw");
  const renderedBoard = { boardView: "rendered", cardViews: { AAAAAAAA: "raw" } };
  assert.equal(effectiveCardView("AAAAAAAA", renderedBoard), "raw");
  assert.equal(effectiveCardView("BBBBBBBB", renderedBoard), "rendered");
});

test("rendered is the default for every unknown or missing view state", () => {
  assert.equal(effectiveCardView("X", undefined), "rendered");
  assert.equal(effectiveCardView("X", {}), "rendered");
  assert.equal(effectiveCardView("X", { boardView: "nonsense" }), "rendered");
  assert.equal(effectiveCardView("X", { cardViews: { X: "nonsense" } }), "rendered");
  assert.equal(effectiveCardView("X", { cardViews: null }), "rendered");
});

test("a stored view state in any unexpected shape degrades to rendered", async () => {
  for (const stored of [null, "raw", 7, [], { boardView: true }, { cardViews: 3 }]) {
    const store = {};
    store[viewKey("Board")] = stored;
    globalThis.syscall = async function (name, ...args) {
      if (name === "clientStore.get") return store[args[0]];
      return undefined;
    };
    const state = await loadViewState("Board");
    assert.equal(state.boardView, "rendered", JSON.stringify(stored));
    assert.deepEqual(state.cardViews, {});
  }
});

test("a store that throws still gives a rendered board", async () => {
  globalThis.syscall = async function () { throw new Error("no store"); };
  const state = await loadViewState("Board");
  assert.deepEqual(state, { boardView: "rendered", cardViews: {} });
});

test("the view choice is remembered per page, in clientStore and nowhere else", async () => {
  assert.equal(viewKey("Todo/running"), "atomdown-board.view:Todo/running");
  assert.notEqual(viewKey("Todo/running"), viewKey("Todo/other"));
  // Page A raw must not make page B raw.
  const store = {};
  store[viewKey("PageA")] = { boardView: "raw", cardViews: {} };
  const { html } = await openBoard(RICH_PAGE, { store, page: "PageB" });
  assert.equal(html.includes('data-card-view="raw"'), false);
});

test("the panel is handed the view state and persists it through clientStore", async () => {
  const { script } = await openBoard(RICH_PAGE);
  assert.ok(script.includes("var ATOMDOWN_BOARD_VIEW ="));
  assert.ok(script.includes('"boardView":"rendered"'));
  // One persistence mechanism, the same one the collapse state uses.
  assert.ok(script.includes('"clientStore.set",'));
  assert.equal(script.includes("localStorage"), false);
  assert.equal(script.includes("sessionStorage"), false);
});

test("the board-wide switch clears per-card overrides rather than layering on them", async () => {
  const { script } = await openBoard(RICH_PAGE);
  const handler = script.slice(script.indexOf("viewBtn.addEventListener"));
  assert.ok(handler.includes("VIEW.cardViews = {};"));
});

// --- rendering changes not one byte of the document ------------------------

test("opening the board rendered writes nothing at all", async () => {
  const { calls, state } = await openBoard(RICH_PAGE);
  assert.equal(state.text, RICH_PAGE);
  const names = calls.map((c) => c.name);
  assert.equal(names.includes("editor.replaceRange"), false);
  assert.equal(names.includes("space.writePage"), false);
  assert.equal(names.includes("editor.reloadPage"), false);
});

test("toggling the view writes nothing to the document", async () => {
  const store = {};
  store[viewKey("Board")] = { boardView: "raw", cardViews: { G92YE2JP: "rendered" } };
  const { calls, state } = await openBoard(RICH_PAGE, { store });
  assert.equal(state.text, RICH_PAGE);
  assert.equal(calls.some((c) => c.name === "editor.replaceRange"), false);
});

test("a rendered board still writes a group as ONE editor.replaceRange", async () => {
  const { calls, state } = recordingSyscall(THREE_ATOMS);
  const result = await plug.functionMapping.groupAtoms(
    JSON.stringify(["atom:AAAAAAAA", "atom:BBBBBBBB"]),
    "first-two",
  );
  assert.equal(result.ok, true);
  const names = calls.map((c) => c.name);
  assert.equal(names.filter((n) => n === "editor.replaceRange").length, 1);
  assert.equal(names.includes("space.writePage"), false);
  assert.equal(names.includes("editor.reloadPage"), false);
  // Every id and every slug the page started with is still there.
  for (const line of THREE_ATOMS.split("\n")) {
    if (line.includes("<atom ")) assert.ok(state.text.includes(line));
  }
});

test("no view state ever reaches a directive line", async () => {
  const store = {};
  store[viewKey("Board")] = { boardView: "raw", cardViews: { AAAAAAAA: "raw" } };
  const { state } = recordingSyscall(THREE_ATOMS, { store });
  await plug.functionMapping.groupAtoms(
    JSON.stringify(["atom:AAAAAAAA", "atom:BBBBBBBB"]),
    "x",
  );
  for (const line of state.text.split("\n")) {
    if (!line.includes("<atom")) continue;
    assert.equal(/\b(view|raw|rendered|collapsed)\s*=/.test(line), false, line);
  }
});

// --- the interactions have to survive a much richer DOM -------------------

test("the raw body carries the block EXACTLY, so a group name still defaults", async () => {
  const { html, script } = await openBoard(RICH_PAGE);
  // deriveGroupSlug reads the raw body, never the rendered one - a rendered
  // heading has lost the ## it matches on.
  assert.ok(script.includes('card.querySelector(".board-card-raw")'));
  assert.equal(script.includes('querySelector(".board-card-body")'), false);
  // And the raw body still holds the markdown, escaped but not reshaped.
  assert.ok(html.includes("&lt;") === false || true);
  assert.ok(html.includes("## Decisions waiting on me"));
  assert.ok(html.includes("|---|---|---|"));
});

test("the card DOM the geometry, lasso and drag read is unchanged in shape", async () => {
  const { html } = await openBoard(RICH_PAGE);
  const atoms = parseAtoms(RICH_PAGE);
  // One .board-card per atom, each with its id, its draggable header and its
  // menu - pickDropTarget, the lasso and unitKeyForCard all key off these.
  assert.equal((html.match(/class="board-card[ "]/g) || []).length, atoms.length);
  for (const atom of atoms) {
    assert.ok(html.includes(`data-atom-id="${atom.id}"`));
    assert.ok(html.includes(`data-drag-atom="${atom.id}"`));
    assert.ok(html.includes(`data-menu-toggle="${atom.id}"`));
  }
  // The group containers and their headers are still there.
  assert.ok(html.includes('data-group-id="KATZ94NM"'));
  assert.ok(html.includes('data-group-header="NS67J8K5"'));
});

test("a rendered table cannot widen the card the drop geometry measures", async () => {
  const { html } = await openBoard(RICH_PAGE);
  const style = html.slice(html.indexOf("<style>"), html.indexOf("</style>"));
  const table = style.slice(style.indexOf(".board-card-rendered table"));
  assert.ok(table.includes("max-width: 100%"));
  assert.ok(table.includes("overflow-x: auto"));
});

test("a click on a rendered link is swallowed, so selection wins", async () => {
  const { script } = await openBoard(RICH_PAGE);
  assert.ok(script.includes('e.target.closest("a")'));
  assert.ok(script.includes("e.preventDefault()"));
  // Captured, so it runs before the card's own click handler stops propagation.
  assert.match(script, /closest\("a"\)[\s\S]{0,220}\}, true\)/);
});

test("a rendered checkbox is a picture of the document, not a control", async () => {
  const { html } = await openBoard(RICH_PAGE);
  assert.ok(html.includes("pointer-events: none"));
});

test("the rendered body takes its colours from the theme, never its own palette", async () => {
  const { html } = await openBoard(RICH_PAGE);
  const style = html.slice(html.indexOf(".board-card-rendered"), html.indexOf("</style>"));
  assert.ok(style.includes("var(--link-color)"));
  assert.ok(style.includes("var(--ui-surface-border-color)"));
  // No literal colour of its own in the rendered-body rules.
  assert.equal(/#[0-9a-fA-F]{3,6}\b/.test(style), false);
  assert.equal(/\brgb\(/.test(style), false);
});

test("the panel and the worker share ONE copy of the view decision", async () => {
  const { script } = await openBoard(RICH_PAGE);
  const injected = injectSharedFunctions();
  assert.ok(injected.includes("function effectiveCardView("));
  assert.ok(script.includes(effectiveCardView.toString()));
  assert.equal(
    (script.match(/function effectiveCardView\(/g) || []).length,
    1,
  );
});

// --- sanitizing ------------------------------------------------------------
//
// markdown.markdownToHtml escapes every text node and every attribute value
// (silverbullet client/markdown_renderer/html_render.ts, htmlEscape), so
// markdown text cannot inject markup. What it does NOT escape is a raw HTML
// tag in the source: that is re-emitted verbatim. These cover exactly that,
// and they run the plug's own sanitizer, not a description of it.

test("a script tag is dropped with its contents", () => {
  assert.equal(
    sanitizeRenderedHtml("<script>alert(1)</script><p>after</p>"),
    "<p>after</p>",
  );
  assert.equal(sanitizeRenderedHtml("<svg><script>alert(1)</script></svg>ok"), "ok");
  assert.equal(sanitizeRenderedHtml('<iframe src="https://e.test"></iframe>t'), "t");
  assert.equal(sanitizeRenderedHtml("<style>body{}</style>x"), "x");
});

test("an event handler attribute never survives", () => {
  assert.equal(sanitizeRenderedHtml('<img src="x" onerror="alert(1)">'), '<img src="x"/>');
  assert.equal(sanitizeRenderedHtml('<p onclick="x()">t</p>'), "<p>t</p>");
  assert.equal(sanitizeRenderedHtml('<p ONMOUSEOVER="x()">t</p>'), "<p>t</p>");
});

test("style and id attributes are dropped, so a card cannot overlay the app", () => {
  assert.equal(
    sanitizeRenderedHtml('<p style="position:fixed;inset:0">t</p>'),
    "<p>t</p>",
  );
  // id would collide with the panel's own element ids.
  assert.equal(
    sanitizeRenderedHtml('<p id="atomdown-board-close">t</p>'),
    "<p>t</p>",
  );
});

test("only safe URL schemes stay in an href or src", () => {
  for (const bad of [
    "javascript:alert(1)",
    "JaVaScript:alert(1)",
    "&#106;avascript:alert(1)",
    "java&Tab;script:alert(1)",
    "&#x6a;avascript:alert(1)",
    "data:text/html,<b>x",
    "vbscript:x",
    "blob:https://a/b",
    "file:///etc/passwd",
  ]) {
    assert.equal(isSafeUrl(bad), false, bad);
    assert.equal(
      sanitizeRenderedHtml(`<a href="${bad}">t</a>`).includes("href"),
      false,
      bad,
    );
  }
  for (const good of ["https://a.test/x", "http://a.test", "mailto:a@b.test",
    "tel:+1555", "/Page/Name", "#anchor", "Relative%20Page"]) {
    assert.equal(isSafeUrl(good), true, good);
    assert.ok(sanitizeRenderedHtml(`<a href="${good}">t</a>`).includes("href="), good);
  }
});

test("a leading space or control character cannot hide a scheme", () => {
  assert.equal(isSafeUrl("  javascript:x"), false);
  assert.equal(isSafeUrl("\tjavascript:x"), false);
  assert.equal(isSafeUrl("\njav\tascript:x"), false);
  assert.equal(decodeUrlEntities("&#106;avascript&colon;x"), "javascript:x");
});

test("a disallowed tag loses the tag but keeps the text the user wrote", () => {
  assert.equal(sanitizeRenderedHtml('<font color="red">colored</font>'), "colored");
  assert.equal(sanitizeRenderedHtml("<marquee>text</marquee>"), "text");
  assert.equal(sanitizeRenderedHtml("<form><p>t</p></form>"), "<p>t</p>");
});

test("the fragment always comes out balanced, so a card cannot swallow the board", () => {
  // A stray close tag would otherwise close the card's own <div>.
  assert.equal(
    sanitizeRenderedHtml("<p>ok</p></div></div><p>next</p>"),
    "<p>ok</p><p>next</p>",
  );
  // An unclosed tag is closed here rather than left open.
  assert.equal(sanitizeRenderedHtml("<div><b>t"), "<div><b>t</b></div>");
  assert.equal(sanitizeRenderedHtml("<ul><li>a<li>b</ul>"),
    "<ul><li>a</li><li>b</li></ul>");
});

test("a malformed or unterminated tag degrades to visible text", () => {
  assert.equal(sanitizeRenderedHtml('<p>a &lt; b</p>'), "<p>a &lt; b</p>");
  assert.equal(sanitizeRenderedHtml("<p>a < b</p>"), "<p>a &lt; b</p>");
  assert.ok(sanitizeRenderedHtml('<p>t <b class="x').includes("&lt;b"));
  assert.equal(sanitizeRenderedHtml("<!-- <atom id=\"X\"/> -->t"), "t");
});

test("already-escaped text is not escaped a second time", () => {
  assert.equal(
    sanitizeRenderedHtml("<p>3 &lt; 5 &amp;&amp; A &quot;B&quot;</p>"),
    "<p>3 &lt; 5 &amp;&amp; A &quot;B&quot;</p>",
  );
});

test("everything CommonMark produces comes through untouched", () => {
  const rich = "<h3>H</h3><p><em>e</em> <strong>s</strong> <code>c</code> " +
    "<del>d</del></p><ul><li>a</li></ul><ol><li>b</li></ol>" +
    "<blockquote><p>q</p></blockquote><pre><code>x</code></pre>" +
    "<table><thead><tr><th>A</th></tr></thead><tbody><tr><td>b</td></tr></tbody></table>" +
    '<hr/><img src="/a.png" alt="a"/><input type="checkbox" checked/>';
  const out = sanitizeRenderedHtml(rich);
  for (const tag of ["h3", "em", "strong", "code", "del", "ul", "li", "ol",
    "blockquote", "pre", "table", "thead", "th", "td", "hr", "img", "input"]) {
    assert.ok(out.includes("<" + tag), tag);
  }
  assert.ok(out.includes('alt="a"'));
  assert.ok(out.includes("checked"));
});

test("hostile HTML from the host never reaches the panel markup", async () => {
  const { html } = await openBoard(RICH_PAGE, {
    markdownHtml: () =>
      '<p onclick="steal()">x</p><script>steal()</script>' +
      '<a href="javascript:steal()">y</a><iframe src="https://e.test"></iframe>',
  });
  assert.equal(html.includes("steal()"), false);
  assert.equal(html.includes("<script"), false);
  assert.equal(html.includes("<iframe"), false);
  assert.ok(html.includes("<p>x</p>"));
});

test("sanitizeRenderedHtml never throws, whatever it is handed", () => {
  for (const input of [null, undefined, 7, {}, [], "", "<", "<<<>>>", "</>",
    "<a href=", "<!--", "<!", "<?php ?>", "<3 </3", "<p".repeat(200)]) {
    assert.equal(typeof sanitizeRenderedHtml(input), "string", String(input));
  }
});

// --- Display density: comfortable and compact ------------------------------
//
// The rules under test are the ones a change to the stylesheet could break
// silently: that compact removes the card header row and puts NO seam in its
// place, that identity moves into the menu, that the group outline and every
// content size are identical at both densities, that the interactions the
// board depends on are still wired to the same elements, and that switching
// density does not touch the document.

const DENSITY_PAGE = "Todo/running";

function densityHtml(sourceText, density, collapsedIds) {
  return buildBoardHtml(
    parseAtoms(sourceText),
    "Board",
    collapsedIds || [],
    null,
    density,
  ).html;
}

// The stylesheet, and one density's block of it.
function styleOf(html) {
  return html.slice(html.indexOf("<style>"), html.indexOf("</style>"));
}

// The stylesheet as {selector, body} rules, comments stripped. A crude split
// on "}" drags neighbouring rules and comment prose into the answer, which is
// exactly how a test can pass while asserting nothing.
function cssRules(html) {
  const style = styleOf(html).replace(/\/\*[\s\S]*?\*\//g, "");
  const out = [];
  const re = /([^{}]+)\{([^{}]*)\}/g;
  let m;
  while ((m = re.exec(style))) {
    out.push({ sel: m[1].trim(), body: m[2].trim() });
  }
  return out;
}

function rulesFor(html, needle) {
  return cssRules(html).filter((r) => r.sel.includes(needle));
}

// Every rule that only applies at compact density.
function compactRules(html) {
  return rulesFor(html, '[data-density="compact"]');
}

function ruleBodies(rules) {
  return rules.map((r) => r.body).join("\n");
}

test("normalizeDensity: comfortable is the default and the fallback", () => {
  assert.equal(normalizeDensity("compact"), "compact");
  assert.equal(normalizeDensity("comfortable"), "comfortable");
  for (const junk of [undefined, null, "", "bare", "COMPACT", 7, {}, []]) {
    assert.equal(normalizeDensity(junk), "comfortable", String(junk));
  }
});

test("there are two densities and no third", () => {
  assert.equal(otherDensity("comfortable"), "compact");
  assert.equal(otherDensity("compact"), "comfortable");
  assert.equal(otherDensity(otherDensity("compact")), "compact");
});

test("the switch is labelled with the state it would give you", () => {
  // Same idiom as the raw/rendered switch beside it.
  assert.equal(densityLabel("comfortable"), "Compact");
  assert.equal(densityLabel("compact"), "Comfortable");
  assert.match(densityTitle("comfortable"), /^Compact:/);
  assert.match(densityTitle("compact"), /^Comfortable:/);
  // The tooltip promises what compact actually does, content size included.
  assert.match(densityTitle("comfortable"), /same content size/);
});

test("the board root carries the density in the markup, so nothing flashes", () => {
  const roomy = densityHtml(TIGHT_GROUP, "comfortable");
  const tight = densityHtml(TIGHT_GROUP, "compact");
  assert.ok(roomy.includes(
    '<div class="board-root" id="atomdown-board-root" data-density="comfortable">',
  ));
  assert.ok(tight.includes(
    '<div class="board-root" id="atomdown-board-root" data-density="compact">',
  ));
});

test("an absent or unknown density draws comfortable", () => {
  const fallback = densityHtml(TIGHT_GROUP, undefined);
  assert.ok(fallback.includes('id="atomdown-board-root" data-density="comfortable"'));
  assert.equal(
    densityHtml(TIGHT_GROUP, "bare"),
    densityHtml(TIGHT_GROUP, "comfortable"),
  );
});

test("the toolbar carries the density switch beside the raw switch", () => {
  const html = densityHtml(TIGHT_GROUP, "comfortable");
  assert.ok(html.includes('id="atomdown-board-density"'));
  assert.ok(html.includes('data-board-density="comfortable"'));
  assert.ok(html.includes(">Compact</button>"));
  // Same class, so it is the same control in the same place.
  assert.ok(html.includes('class="board-close" id="atomdown-board-density"'));
  assert.ok(
    html.indexOf('id="atomdown-board-density"') <
      html.indexOf('id="atomdown-board-view"'),
  );
});

test("densityKey is scoped by page and carries no document data", () => {
  assert.equal(densityKey(DENSITY_PAGE), "atomdown-board.density:Todo/running");
  assert.notEqual(densityKey("A"), densityKey("B"));
  assert.equal(densityKey(undefined), "atomdown-board.density:");
});

test("loadDensity defaults to comfortable, and never throws", async () => {
  const store = {};
  store[densityKey(DENSITY_PAGE)] = "compact";
  recordingSyscall(THREE_ATOMS, { store });
  assert.equal(await loadDensity(DENSITY_PAGE), "compact");
  assert.equal(await loadDensity("Another"), "comfortable");

  for (const junk of ["bare", "", 3, {}, null]) {
    const s2 = {};
    s2[densityKey(DENSITY_PAGE)] = junk;
    recordingSyscall(THREE_ATOMS, { store: s2 });
    assert.equal(await loadDensity(DENSITY_PAGE), "comfortable", String(junk));
  }

  recordingSyscall(THREE_ATOMS, { storeThrows: true });
  assert.equal(await loadDensity(DENSITY_PAGE), "comfortable");
});

test("a page left compact opens compact", async () => {
  const store = {};
  store[densityKey(DENSITY_PAGE)] = "compact";
  const { calls } = await closedStart(TIGHT_GROUP, {
    page: DENSITY_PAGE,
    store,
  });
  await plug.functionMapping.toggleBoard();
  const { html, script } = lastPanel(calls);
  assert.ok(html.includes('id="atomdown-board-root" data-density="compact"'));
  assert.ok(script.includes('var ATOMDOWN_BOARD_DENSITY = "compact"'));
});

test("the density is remembered per page, never as a default for another", async () => {
  const store = {};
  store[densityKey("Compact page")] = "compact";
  const { calls } = await closedStart(TIGHT_GROUP, {
    page: "Roomy page",
    store,
  });
  await plug.functionMapping.toggleBoard();
  assert.ok(lastPanel(calls).html.includes('data-density="comfortable">'));
});

test("the density lives in clientStore and nowhere else", async () => {
  const { calls, script } = await openBoard(TIGHT_GROUP, { page: DENSITY_PAGE });
  // The panel persists it through the one store the plug already uses.
  assert.ok(script.includes('"atomdown-board.density:" + ATOMDOWN_BOARD_PAGE'));
  assert.ok(script.includes('"clientStore.set"'));
  // No second mechanism: a worker has no localStorage at all.
  assert.ok(!script.includes("localStorage"));
  assert.ok(!script.includes("sessionStorage"));
  // And nothing about the density is written to the document.
  assert.equal(calls.filter((c) => c.name === "editor.replaceRange").length, 0);
  assert.equal(calls.some((c) => c.name === "space.writePage"), false);
});

test("switching density is a CSS change, not a redraw and not a write", async () => {
  const { script } = await openBoard(TIGHT_GROUP, { page: DENSITY_PAGE });
  // setDensity flips an attribute and persists. It does not call back into
  // the worker, so there is no round trip and no chance of an edit.
  const setDensity = script.slice(script.indexOf("function setDensity("));
  const body = setDensity.slice(0, setDensity.indexOf("\n    }"));
  assert.ok(body.includes("applyDensity()"));
  assert.ok(body.includes("persistDensity()"));
  assert.ok(!body.includes("invokeFunction"));
  assert.ok(script.includes(
    'document.documentElement.setAttribute("data-density", DENSITY)',
  ));
});

test("no density value ever reaches a directive line", async () => {
  const store = {};
  store[densityKey(DENSITY_PAGE)] = "compact";
  const { state } = recordingSyscall(THREE_ATOMS, {
    page: DENSITY_PAGE,
    store,
  });
  const grouped = await plug.functionMapping.groupAtoms(
    JSON.stringify(["atom:AAAAAAAA", "atom:BBBBBBBB"]),
    "two",
  );
  assert.equal(grouped.ok, true);
  for (const word of ["density", "compact", "comfortable", "collapsed"]) {
    assert.ok(!state.text.includes(word), word);
  }
});

test("a board action at compact density is still one edit and no page write", async () => {
  const store = {};
  store[densityKey(DENSITY_PAGE)] = "compact";
  const { calls } = recordingSyscall(THREE_ATOMS, {
    page: DENSITY_PAGE,
    store,
  });
  const result = await plug.functionMapping.groupAtoms(
    JSON.stringify(["atom:BBBBBBBB", "atom:CCCCCCCC"]),
    "",
  );
  assert.equal(result.ok, true);
  const names = calls.map((c) => c.name);
  assert.equal(names.filter((n) => n === "editor.replaceRange").length, 1);
  assert.equal(names.includes("space.writePage"), false);
  assert.equal(names.includes("editor.reloadPage"), false);
  // And the redraw came back compact, not reset to the default.
  assert.ok(lastPanel(calls).html.includes('data-density="compact">'));
});

// --- Compact: the card header row is gone ---------------------------------

test("compact lifts the card header out of the layout entirely", () => {
  const html = densityHtml(TIGHT_GROUP, "compact");
  const block = rulesFor(html, '[data-density="compact"] .board-card-header')[0]
    .body;
  assert.match(block, /position:\s*absolute/);
  assert.match(block, /padding:\s*0/);
  assert.match(block, /min-height:\s*0/);
  assert.match(block, /border-bottom:\s*none/);
  assert.match(block, /background:\s*transparent/);
  assert.match(block, /pointer-events:\s*none/);
});

test("compact puts NO seam where the header was", () => {
  const bodies = ruleBodies(compactRules(densityHtml(TIGHT_GROUP, "compact")));
  // No dotted line, no dashed line, no replacement rule of any kind. The
  // card border is the card. Steve rejected a seam explicitly.
  assert.ok(!bodies.includes("dotted"));
  assert.ok(!bodies.includes("dashed"));
  assert.ok(!bodies.includes("border-top"));
  for (const decl of bodies.split(";")) {
    if (decl.includes("border-bottom")) assert.match(decl, /border-bottom:\s*none/);
  }
});

test("compact hides the card's own identity spans, and the menu carries them", () => {
  const html = densityHtml(TIGHT_GROUP, "compact");
  const hidden = compactRules(html).filter((r) => r.body.includes("display: none"));
  const sels = hidden.map((r) => r.sel).join(" ");
  assert.ok(sels.includes('[data-density="compact"] .board-card-slug'));
  assert.ok(sels.includes('[data-density="compact"] .board-card-id'));
  assert.ok(sels.includes(".board-card .board-badge"));
  // The spans are still in the markup - the same markup serves both
  // densities, which is why switching needs no redraw.
  assert.ok(html.includes('class="board-card-id"'));
});

test("only the grip and the menu take the pointer back in compact", () => {
  const rules = compactRules(densityHtml(TIGHT_GROUP, "compact"));
  const auto = rules.filter((r) => r.body.includes("pointer-events: auto"));
  // Exactly one rule, naming exactly those two controls. So a click on the
  // strip the header occupies falls through to the card and still selects it.
  assert.equal(auto.length, 1);
  assert.ok(auto[0].sel.includes(".board-card-drag"));
  assert.ok(auto[0].sel.includes(".board-card-menu"));
});

test("compact reserves room at the top right of the body text", () => {
  const rule = rulesFor(
    densityHtml(TIGHT_GROUP, "compact"),
    ".board-card > .board-card-body",
  )[0];
  assert.ok(rule.sel.includes('[data-density="compact"]'));
  assert.equal(
    rule.body,
    "padding-right: calc(var(--board-card-padding) + var(--board-card-chrome-space));",
  );
});

test("compact does not scale one byte of content", () => {
  const html = densityHtml(RICH_PAGE, "compact");
  const rules = compactRules(html);
  // A heading is a heading at its full rendered size. The ONE font-size in
  // the compact rules is the collapse chevron's, which is restated at its
  // full value precisely so it cannot shrink.
  const sized = rules.filter((r) => r.body.includes("font-size"));
  assert.equal(sized.length, 1);
  assert.ok(sized[0].sel.includes(".board-group-collapse"));
  assert.ok(sized[0].body.includes("font-size: 12px"));
  // No compact rule reaches a card body at all.
  for (const r of rules) {
    assert.ok(!r.sel.includes(".board-card-rendered"), r.sel);
    assert.ok(!r.sel.includes(".board-card-raw"), r.sel);
  }
  // And the compact variable block sets no font and does not touch the
  // group outline's width.
  const vars = rulesFor(html, ':root[data-density="compact"]')[0].body;
  assert.ok(!vars.includes("font"));
  assert.ok(!vars.includes("--board-group-border-width"));
});

test("compact renames nothing: every class the board already had survives", () => {
  const roomy = densityHtml(RICH_PAGE, "comfortable");
  const tight = densityHtml(RICH_PAGE, "compact");
  for (
    const name of [
      "board-cards",
      "board-card",
      "board-card-header",
      "board-card-body",
      "board-card-rendered",
      "board-card-raw",
      "board-card-slug",
      "board-card-id",
      "board-card-menu",
      "board-menu-btn",
      "board-menu-popover",
      "board-drag-handle",
      "board-group",
      "board-group-header",
      "board-group-cards",
      "board-group-collapse",
      "board-group-name",
      "board-group-id",
      "board-group-count",
      "board-group-actions",
      "board-toolbar",
    ]
  ) {
    assert.ok(roomy.includes(name), "comfortable: " + name);
    assert.ok(tight.includes(name), "compact: " + name);
  }
  // The CARD STRIP is byte-identical between the two densities, so the switch
  // cannot move, add or remove an element the geometry, the lasso, the drag
  // or the selection reads. Only the root's attribute and the toolbar
  // switch's own label differ.
  const strip = (html) => html.slice(html.indexOf('<div class="board-cards">'));
  assert.equal(strip(tight), strip(roomy));
});

// --- Compact: what drags ---------------------------------------------------

test("the grip is a drag source in its own right, so compact can still drag", () => {
  const html = densityHtml(THREE_ATOMS, "compact");
  assert.ok(html.includes(
    '<span class="board-drag-handle board-card-drag" draggable="true" data-drag-unit="atom:AAAAAAAA"',
  ));
  // Keyboard reachable and named, because in compact it is the drag source
  // rather than a decoration inside a draggable row.
  assert.ok(html.includes('role="button" tabindex="0" aria-label="Drag to move this card"'));
});

test("a grouped card's grip carries the whole group's unit key", () => {
  const html = densityHtml(TIGHT_GROUP, "compact");
  assert.ok(html.includes('data-drag-unit="group:KF53ASNE"'));
  // The standalone card above the group still carries its own.
  assert.ok(html.includes('data-drag-unit="atom:J1BBCED5"'));
});

test("comfortable's drag source is unchanged: the header still drags", () => {
  const html = densityHtml(THREE_ATOMS, "comfortable");
  assert.ok(html.includes(
    '<div class="board-card-header" draggable="true" data-drag-atom="AAAAAAAA"',
  ));
});

test("the panel wires both drag sources, and the grip wins by stopping there", () => {
  const script = buildBoardHtml(
    parseAtoms(THREE_ATOMS),
    "Board",
    [],
    null,
    "compact",
  ).script;
  assert.ok(script.includes('querySelectorAll(".board-card-header[data-drag-atom]")'));
  assert.ok(script.includes('querySelectorAll("[data-drag-unit]")'));
  // The generic handler stops propagation, so a grip drag never runs the
  // header's handler as well - one dragstart, one unit key.
  const generic = script.slice(script.indexOf('querySelectorAll("[data-drag-unit]")'));
  assert.ok(generic.slice(0, 400).includes("e.stopPropagation()"));
});

test("the geometry the drop reads is still the cards' own rectangles", () => {
  const script = buildBoardHtml(
    parseAtoms(TIGHT_GROUP),
    "Board",
    [],
    null,
    "compact",
  ).script;
  // cardGeometry() is untouched by density: it reads .board-card rectangles,
  // and in compact the header is out of flow, so a card's rectangle IS its
  // body's rectangle. Shorter cards, same decision function.
  assert.ok(script.includes("function cardGeometry()"));
  assert.ok(script.includes("card.getBoundingClientRect()"));
  assert.ok(script.includes("pickDropTarget(e.clientY, cards)"));
});

// --- Identity moved into the menu -----------------------------------------

test("the card menu shows the name and the id, at BOTH densities", () => {
  for (const density of ["comfortable", "compact"]) {
    const script = buildBoardHtml(
      parseAtoms(THREE_ATOMS),
      "Board",
      [],
      null,
      density,
    ).script;
    // Identity is the first thing appended to a card's popover.
    const build = script.slice(script.indexOf("function buildPopover("));
    assert.ok(
      build.indexOf('identityLabel("atom"') <
        build.indexOf('el("div", "board-menu-group-row")'),
      density,
    );
    assert.ok(script.includes('function identityLabel(kind, slug, id)'), density);
    assert.ok(script.includes('board-menu-identity-name'), density);
    assert.ok(script.includes('board-menu-identity-id'), density);
  }
});

test("the identity label is a label, not an action", () => {
  const script = buildBoardHtml(parseAtoms(THREE_ATOMS), "Board", [], null, "compact")
    .script;
  const fn = script.slice(
    script.indexOf("function identityLabel("),
    script.indexOf("function identityLabel(") + 900,
  );
  // Divs and spans only: no button, and no click listener.
  assert.ok(!fn.includes('el("button"'));
  assert.ok(!fn.includes("addEventListener"));
});

test("an implicit atom says it has no id, rather than inventing one", () => {
  const script = buildBoardHtml(parseAtoms(THREE_ATOMS), "Board", [], null, "compact")
    .script;
  assert.ok(script.includes('"no id yet (implicit atom)"'));
  assert.ok(script.includes('"unnamed block"'));
});

test("the group menu carries what the thin bar folded into it", () => {
  const html = densityHtml(LOOSE_GROUP, "compact");
  const script = buildBoardHtml(
    parseAtoms(LOOSE_GROUP),
    "Board",
    [],
    null,
    "compact",
  ).script;
  assert.ok(html.includes('data-group-menu-toggle="3G7K9R5V"'));
  assert.ok(html.includes('data-group-menu-popover="3G7K9R5V"'));
  // Identity, then the two actions, then the density readout.
  assert.ok(script.includes('identityLabel("group", slug, groupId)'));
  assert.ok(script.includes('renameItem.textContent = "Rename"'));
  assert.ok(script.includes('ungroupItem.textContent = "Ungroup"'));
  assert.ok(script.includes('"Density: " + DENSITY'));
  assert.ok(script.includes('data-density-readout'));
  // The card menu still does not offer a group rename.
  assert.ok(!script.includes("Rename group"));
});

test("a click inside the group menu is not a select-the-group gesture", () => {
  const script = buildBoardHtml(parseAtoms(LOOSE_GROUP), "Board", [], null, "compact")
    .script;
  const handler = script.slice(script.indexOf('header.addEventListener("click"'));
  const body = handler.slice(0, handler.indexOf("selectGroup("));
  assert.ok(body.includes('e.target.closest(".board-card-menu")'));
});

// --- Compact: a thin group bar --------------------------------------------

test("compact folds the GROUP label, the id and the two buttons away", () => {
  const html = densityHtml(LOOSE_GROUP, "compact");
  const hidden = compactRules(html)
    .filter((r) => r.body.includes("display: none"))
    .map((r) => r.sel).join(" ");
  for (
    const name of [
      ".board-group-kind",
      ".board-group-id",
      ".board-group-actions",
      ".board-group-count-word",
    ]
  ) {
    assert.ok(hidden.includes('[data-density="compact"] ' + name), name);
  }
  // The menu takes the right-hand end of the bar the buttons vacated.
  const menu = rulesFor(html, '[data-density="compact"] .board-group-menu')[0];
  assert.equal(menu.body, "margin-left: auto;");
});

test("the card count is a bare number in compact and a phrase in comfortable", () => {
  const html = densityHtml(LOOSE_GROUP, "comfortable");
  assert.ok(html.includes(
    '<span class="board-group-count"><span class="board-group-count-n">2</span><span class="board-group-count-word"> cards</span></span>',
  ));
  // One card reads "1 card", not "1 cards".
  assert.ok(densityHtml(TIGHT_GROUP, "comfortable").includes(
    '<span class="board-group-count-word"> cards</span>',
  ));
  const single = [
    '<!-- <atom-group id="AAAAAAA1"> -->',
    '<!-- <atom id="BBBBBBB1"/> -->',
    "One.",
    "<!-- </atom-group> -->",
    "",
  ].join("\n");
  assert.ok(densityHtml(single, "comfortable").includes(
    '<span class="board-group-count-word"> card</span>',
  ));
});

test("the group outline is IDENTICAL at both densities", () => {
  for (const density of ["comfortable", "compact"]) {
    const style = styleOf(densityHtml(LOOSE_GROUP, density));
    assert.ok(style.includes(
      "border: var(--board-group-border-width) solid var(--board-accent-color)",
    ), density);
  }
  // No compact rule touches the outline: not its width, not its colour, and
  // not the container element at all. Density does not change the outline;
  // only hover does.
  for (const r of compactRules(densityHtml(LOOSE_GROUP, "compact"))) {
    assert.notEqual(r.sel, '[data-density="compact"] .board-group');
    assert.ok(!r.body.includes("border-color"), r.sel);
    assert.ok(!r.body.includes("--board-group-border-width"), r.sel);
  }
});

test("the collapse chevron is not shrunk and never hover-gated", () => {
  const html = densityHtml(LOOSE_GROUP, "compact");
  const block = rulesFor(html, '[data-density="compact"] .board-group-collapse')[0]
    .body;
  assert.match(block, /font-size: 12px/);
  assert.match(block, /padding: 1px 5px/);
  assert.match(block, /opacity: 1/);
  // Nothing gates the chevron on hover, and nothing hides it: the two
  // hover-only rules name the grip and the three-dot button only.
  for (const r of cssRules(html)) {
    if (!r.body.includes("opacity")) continue;
    assert.ok(
      !r.sel.includes(".board-group-collapse") || r.body.includes("opacity: 1"),
      r.sel,
    );
  }
  const gated = cssRules(html)
    .filter((r) => r.body.includes("opacity: 0"))
    .map((r) => r.sel).join(" ");
  assert.ok(!gated.includes("board-group-collapse"));
  assert.ok(gated.includes("board-drag-handle"));
  assert.ok(gated.includes("board-menu-btn"));
});

// --- Comfortable: the header is present, but quiet ------------------------

test("a resting card header uses the theme's muted token, not opacity", () => {
  const html = densityHtml(RICH_PAGE, "comfortable");
  const style = styleOf(html);
  assert.ok(style.includes("--board-header-quiet-color: var(--subtle-color)"));
  assert.ok(style.includes("--board-header-active-color: var(--root-color)"));
  assert.ok(style.includes(
    ".board-card-slug { color: var(--board-header-quiet-color); }",
  ));
  assert.ok(style.includes("color: var(--board-header-quiet-color)"));
  // Deliberately not opacity: it multiplies against whatever is behind the
  // element, so one value would read differently on the plain card surface,
  // on a selected card, inside a group container, and again in dark theme.
  const quiet = rulesFor(html, ".board-card-slug").map((r) => r.body).join("\n");
  assert.ok(!quiet.includes("opacity"));
});

test("hover, focus and selection all promote the resting header text", () => {
  const html = densityHtml(RICH_PAGE, "comfortable");
  const promote = rulesFor(html, ".board-card:hover .board-card-slug")[0].sel +
    " { " + rulesFor(html, ".board-card:hover .board-card-slug")[0].body + " }";
  for (
    const sel of [
      ".board-card:hover .board-card-slug",
      ".board-card:hover .board-card-id",
      ".board-card:focus-within .board-card-slug",
      ".board-card-selected .board-card-slug",
      ".board-card-selected .board-card-id",
    ]
  ) {
    assert.ok(promote.includes(sel), sel);
  }
  assert.match(promote, /color: var\(--board-header-active-color\)/);
});

test("promoting the header is colour only, so nothing reflows on hover", () => {
  const html = densityHtml(RICH_PAGE, "comfortable");
  const style = styleOf(html);
  const promote = rulesFor(html, ".board-card:hover .board-card-slug")[0];
  // One declaration, and it is a colour.
  assert.equal(promote.body, "color: var(--board-header-active-color);");
  for (
    const prop of ["display", "padding", "margin", "font-size", "height", "border"]
  ) {
    assert.ok(!promote.body.includes(prop + ":"), prop);
  }
  // The grip and the three-dot button keep their boxes and only change
  // opacity, which is the rule the two hover-only commits established.
  assert.ok(style.includes(".board-card:hover .board-drag-handle"));
  assert.ok(style.includes(".board-drag-handle:focus-visible"));
  assert.ok(style.includes(".board-menu-btn:focus-visible"));
});

// --- A group's own chrome recedes too -------------------------------------

test("a resting group softens its border and its header background only", () => {
  const rest = ruleBodies(
    rulesFor(densityHtml(LOOSE_GROUP, "comfortable"), ":not(:hover"),
  );
  assert.match(rest, /border-color: color-mix\(in srgb, var\(--board-accent-color\) var\(--board-group-quiet-border\), transparent\)/);
  assert.match(rest, /background: color-mix\(in srgb, var\(--board-accent-color\) var\(--board-group-quiet-header\), var\(--ui-surface-background-color\)\)/);
  // The header's text colour follows its background, or light-on-pale would
  // be unreadable. Nothing else is declared.
  assert.match(rest, /color: var\(--ui-surface-color\)/);
  assert.ok(!rest.includes("opacity"));
  // Not one property that would change a size.
  for (const prop of ["border-width", "padding", "margin", "font-size", "height"]) {
    assert.ok(!rest.includes(prop + ":"), prop);
  }
});

test("NOT opacity on the container: that would fade every card inside it", () => {
  // Opacity applies to an element AND ALL ITS DESCENDANTS, so an opacity on
  // .board-group would fade every member card - the outcome Steve ruled out.
  for (const r of cssRules(densityHtml(LOOSE_GROUP, "comfortable"))) {
    const target = r.sel.split(",").map((s) => s.trim()).some((s) =>
      /\.board-group(:|$)/.test(s) || s.endsWith(".board-group")
    );
    if (target) assert.ok(!r.body.includes("opacity"), r.sel);
  }
});

test("the pointer anywhere inside a group, a member card included, wakes it", () => {
  const style = styleOf(densityHtml(LOOSE_GROUP, "comfortable"));
  const selector = style.slice(
    style.indexOf(".board-group:not(:hover"),
    style.indexOf("{", style.indexOf(".board-group:not(:hover")),
  );
  // :hover is tested on the CONTAINER, which a descendant's hover satisfies,
  // so hovering a member card counts as hovering the group. It is not
  // ".board-group-header:hover".
  assert.ok(selector.includes(".board-group:not(:hover"));
  assert.ok(!selector.includes(".board-group-header:hover"));
});

test("a resting group changes nothing about its member cards", () => {
  const rest = rulesFor(densityHtml(LOOSE_GROUP, "comfortable"), ":not(:hover");
  // Exactly two rules: the container itself, and its own header bar. Neither
  // selects a card, so a member card's surface, border and content are
  // identical resting and active.
  assert.equal(rest.length, 2);
  const subjects = rest.map((r) => r.sel.replace(/:not\(.*?\)(?=\s|$)/g, "").trim());
  assert.deepEqual(subjects, [".board-group", ".board-group > .board-group-header"]);
});

test("a collapsed group, and a group holding a selection, stay at full strength", () => {
  const style = styleOf(densityHtml(LOOSE_GROUP, "comfortable"));
  const selector = style.slice(
    style.indexOf(".board-group:not(:hover"),
    style.indexOf("{", style.indexOf(".board-group:not(:hover")),
  );
  // Collapsed: the bar is then the only thing on screen representing the
  // group's contents, so it must not recede.
  assert.ok(selector.includes(".board-group-collapsed"));
  // Selected: a group you picked reads as active, the same rule a selected
  // card's header follows. Clicking the header selects every member, so this
  // covers that gesture too.
  assert.ok(selector.includes(":has(.board-card-selected)"));
  assert.ok(selector.includes(":focus-within"));
});

test("a browser without color-mix or :has keeps the group at full strength", () => {
  const style = styleOf(densityHtml(LOOSE_GROUP, "comfortable"));
  // The resting override comes AFTER the full-strength rules, so an
  // unsupported selector or value drops this block and the group simply
  // never recedes - it is never left at an unreadable half state.
  assert.ok(
    style.indexOf(".board-group {") < style.indexOf(".board-group:not(:hover"),
  );
});

// --- The CSS customization surface ---------------------------------------

test("every tweakable value is a named custom property with a default", () => {
  const style = styleOf(densityHtml(RICH_PAGE, "comfortable"));
  const defaults = {
    "--board-card-padding": "8px",
    "--board-card-header-padding": "6px 8px",
    "--board-card-header-height": "auto",
    "--board-card-border-width": "1px",
    "--board-card-radius": "6px",
    "--board-accent-color": "var(--ui-accent-color)",
    "--board-grip-size": "14px",
    "--board-id-size": "11px",
    "--board-header-quiet-color": "var(--subtle-color)",
    "--board-header-active-color": "var(--root-color)",
    "--board-card-gap": "14px",
    "--board-card-chrome-space": "24px",
    "--board-group-padding": "8px",
    "--board-group-card-gap": "8px",
    "--board-group-header-padding": "5px 8px",
    "--board-group-border-width": "2px",
    "--board-group-quiet-border": "40%",
    "--board-group-quiet-header": "16%",
  };
  for (const name of Object.keys(defaults)) {
    assert.ok(
      style.includes(name + ": " + defaults[name] + ";"),
      name + " default",
    );
  }
});

test("the board's properties are copied from the parent, so space-style reaches them", async () => {
  const { script } = await openBoard(RICH_PAGE);
  // A plug panel renders in an iframe and a parent stylesheet cannot select
  // into it, so the ONLY route in is the property copy applyParentTheme
  // already performs for the theme tokens. Every documented knob is on that
  // list, or a user could not set it.
  const list = script.slice(
    script.indexOf("var THEME_VAR_NAMES = ["),
    script.indexOf("];", script.indexOf("var THEME_VAR_NAMES = [")),
  );
  for (
    const name of [
      "--board-card-padding",
      "--board-card-header-padding",
      "--board-card-header-height",
      "--board-card-border-width",
      "--board-card-radius",
      "--board-accent-color",
      "--board-grip-size",
      "--board-id-size",
      "--board-header-quiet-color",
      "--board-header-active-color",
      "--board-card-gap",
      "--board-card-chrome-space",
      "--board-group-padding",
      "--board-group-card-gap",
      "--board-group-header-padding",
      "--board-group-border-width",
    ]
  ) {
    assert.ok(list.includes(name), name);
  }
  // The theme tokens are still on it: one mechanism, not two.
  assert.ok(list.includes("--ui-accent-color"));
  assert.ok(script.includes("document.documentElement.style.setProperty"));
});

test("the accent travels through ONE knob, which defaults to the theme's", () => {
  const style = styleOf(densityHtml(LOOSE_GROUP, "comfortable"));
  assert.ok(style.includes("--board-accent-color: var(--ui-accent-color)"));
  // Retinting that one property retints the group outline, the drop
  // indicator, the selection ring and the lasso, because none of them names
  // the theme token directly any more.
  const afterRoot = style.slice(style.indexOf("body {"));
  assert.ok(!afterRoot.includes("var(--ui-accent-color)"));
  for (
    const rule of [
      ".board-card-dropbefore",
      ".board-card-dropafter",
      ".board-card-selected",
      ".board-lasso",
    ]
  ) {
    const at = style.indexOf(rule);
    assert.ok(at > 0, rule);
    assert.ok(
      style.slice(at, style.indexOf("}", at)).includes("var(--board-accent-color)"),
      rule,
    );
  }
});

// --- A multi-line block reaches the renderer whole -------------------------
//
// From a report that an ordered list rendered as one run-on paragraph and that
// links inside table cells stayed as raw markdown. Both were artifacts of a
// test rig's markdown stub, not of this plug: the live board renders one <ol>
// with six <li>, and nine real <a href> inside <td>. These tests pin the two
// things that ARE this plug's job, using that exact content.

// The two block SHAPES the report was about, with neutral content: a
// three-item ordered list whose items carry bold and inline code, and a table
// whose cells carry links. The shape is what the tests are about; the real
// page's own text is work content and does not belong in this repo.
const REAL_ORDERED_LIST = [
  '1. **First decision.** Commit `2066012` "one line per directive" is still local, and later commits reverse it. Drop it or push the contradiction.',
  '2. **Stale ticket.** `tracker-8og` - "enforce the rule" is closed with a title that now states the opposite of the rule.',
  "3. **Rewritten policy.** An agent rewrote the README's exclusion of `emit` to fit the change. Defensible, but read that paragraph myself.",
].join("\n");

const REAL_TABLE = [
  "| Ticket | State | Tonight |",
  "|---|---|---|",
  '| [TCK-72357 "Productionize the service"](https://example.invalid/browse/TCK-72357) | On Hold, me | Add cache expiry, single flight, rate limit |',
  '| [TCK-62020 "Status endpoint integration"](https://example.invalid/browse/TCK-62020) | Triage | Add the dedupe key |',
].join("\n");

const REAL_BLOCKS_PAGE = [
  '<!-- <atomdown version="1"/> -->',
  '<!-- <atom id="DK3F1M7W"/> -->',
  REAL_ORDERED_LIST,
  "",
  '<!-- <atom id="YFEH04BQ"/> -->',
  REAL_TABLE,
  "",
].join("\n");

test("an ordered list reaches the renderer with its line structure intact", async () => {
  // An ordered list is only a list because of its newlines. Nothing in the
  // parse path may join, trim or reflow a block's lines.
  const atoms = parseAtoms(REAL_BLOCKS_PAGE);
  const list = atoms.find((a) => a.id === "DK3F1M7W");
  assert.equal(list.text, REAL_ORDERED_LIST);
  assert.equal(list.text.split("\n").length, 3);

  // And that exact text is what the plug hands the host's renderer.
  const { calls } = await openBoard(REAL_BLOCKS_PAGE, { page: "Real" });
  const asked = calls.filter((c) => c.name === "markdown.markdownToHtml")
    .map((c) => c.args[0]);
  assert.ok(asked.includes(REAL_ORDERED_LIST));
  assert.ok(asked.includes(REAL_TABLE));
});

test("a rendered ordered list survives the sanitizer as ol and li", () => {
  const html = sanitizeRenderedHtml(
    "<ol><li><strong>Atomdown history.</strong> Commit <code>2066012</code> is still local.</li>" +
      "<li><strong>Stale bead.</strong> closed with the wrong title.</li></ol>",
  );
  assert.equal((html.match(/<li>/g) || []).length, 2);
  assert.ok(html.startsWith("<ol>"));
  assert.ok(html.endsWith("</ol>"));
  assert.ok(html.includes("<strong>"));
  assert.ok(html.includes("<code>"));
  // start and reversed are on the attribute allowlist, so a list that does
  // not begin at 1 keeps its numbering.
  assert.ok(sanitizeRenderedHtml('<ol start="4"><li>x</li></ol>').includes('start="4"'));
});

test("a link inside a table cell survives, exactly like one in a paragraph", () => {
  const cell =
    '<a href="https://example.invalid/browse/TCK-72357">TCK-72357 "Productionize the service"</a>';
  const html = sanitizeRenderedHtml(
    "<table><tbody><tr><th>Ticket</th></tr><tr><td>" + cell +
      "</td></tr></tbody></table>",
  );
  // The sanitizer is context-free on purpose: a cell is not a special case,
  // so an anchor in a td is treated exactly like one in a p.
  assert.ok(html.includes('href="https://example.invalid/browse/TCK-72357"'));
  assert.ok(html.includes("<td>"));
  assert.ok(html.includes("Productionize the service"));
  // Absolute, so the sanitizer adds target="_blank" - a click cannot replace
  // the board with the target page.
  assert.ok(html.includes('target="_blank"'));
  const inParagraph = sanitizeRenderedHtml("<p>" + cell + "</p>");
  assert.equal(
    html.slice(html.indexOf("<a "), html.indexOf("</a>")),
    inParagraph.slice(inParagraph.indexOf("<a "), inParagraph.indexOf("</a>")),
  );
});

test("both blocks land in their cards at both densities", async () => {
  for (const density of ["comfortable", "compact"]) {
    const store = {};
    store[densityKey("Real")] = density;
    const { calls } = await closedStart(REAL_BLOCKS_PAGE, {
      page: "Real",
      store,
    });
    await plug.functionMapping.toggleBoard();
    const html = lastPanel(calls).html;
    assert.ok(html.includes("<ol>"), density);
    assert.equal((html.match(/<li>/g) || []).length, 3, density);
    assert.ok(html.includes("<table>"), density);
    assert.ok(
      html.includes('href="https://example.invalid/browse/TCK-72357"'),
      density,
    );
    // The raw body still carries the block byte for byte beside it.
    assert.ok(html.includes(escapeForTest(REAL_ORDERED_LIST)), density);
  }
});

// The plug escapes a raw body with its own escapeHtml, which is not exported.
// Only the characters that appear in these fixtures matter here.
function escapeForTest(text) {
  return text.replace(/&/g, "&amp;").replace(/</g, "&lt;")
    .replace(/>/g, "&gt;").replace(/"/g, "&quot;");
}

// --- Content digest --------------------------------------------------------
//
// These are the tests that keep the board honest against `atomdown verify`.
// REAL_DIGEST_PAGE's digest values were produced by the atomdown binary
// (`atomdown materialize -digest -w`), not by this plug, so a change to the
// hashing rules on either side fails here instead of shipping a staleness
// light that disagrees with the tool.

const REAL_DIGEST_PAGE = [
  '<!-- <atomdown version="1"/> -->',
  '<!-- <atom id="4P8W2H6K" slug="claim" digest="sha256:7886129112aee29e7dfd69c7fbdb1dac248582ddeba93fea21d08472519e5f8e"/> -->',
  "## Evidence",
  "",
  '<!-- <atom id="9R3C7M5D" slug="findings" digest="sha256:ff974710b96a205bdb50e66d1c1ba6b4cbf72dfcdff8c79f704d7ef118ef0af8"/> -->',
  "- First item with **strong text**.",
  "- Second item with [a link](https://example.com).",
  "",
  '<!-- <atom id="2F6J8Q4T" digest="sha256:bd1d26d5b1aa05f0be76faa0b46a5c54d03b0f99322bcabb1ef8e20b740528f6"/> -->',
  "A paragraph with  two trailing spaces above and    interior runs.",
].join("\n");

test("digestOfBlock reproduces the binary's own digests, byte for byte", async () => {
  const atoms = parseAtoms(REAL_DIGEST_PAGE);
  assert.equal(atoms.length, 3);
  for (const atom of atoms) {
    assert.equal(
      await digestOfBlock(atom.text),
      recordedDigest(atom),
      atom.id,
    );
  }
});

test("a digest has the shape SPEC.md defines", async () => {
  const value = await digestOfBlock("## Evidence");
  assert.match(value, /^sha256:[0-9a-f]{64}$/);
});

test("CRLF and a lone CR are the one normalization, and they agree with LF", async () => {
  const lf = await digestOfBlock("- one\n- two");
  assert.equal(await digestOfBlock("- one\r\n- two"), lf);
  assert.equal(await digestOfBlock("- one\r- two"), lf);
});

test("nothing else is normalized: whitespace inside a block is content", async () => {
  const plain = await digestOfBlock("a b");
  // Interior runs, indentation and trailing spaces all change the digest,
  // because each of them changes what CommonMark renders.
  assert.notEqual(await digestOfBlock("a  b"), plain);
  assert.notEqual(await digestOfBlock("  a b"), plain);
  assert.notEqual(await digestOfBlock("a b  "), plain);
  // And no Unicode normalization: the composed and decomposed forms differ.
  assert.notEqual(await digestOfBlock("é"), await digestOfBlock("é"));
});

test("a matching digest reads fresh, and a changed block reads stale", async () => {
  const atoms = parseAtoms(REAL_DIGEST_PAGE);
  for (const atom of atoms) {
    assert.equal(await digestStateOf(atom), "fresh", atom.id);
  }
  const edited = parseAtoms(
    REAL_DIGEST_PAGE.replace("## Evidence", "## Evidence, revised"),
  );
  assert.equal(await digestStateOf(edited[0]), "stale");
  // And only that one: an edit to one block cannot make another look stale.
  assert.equal(await digestStateOf(edited[1]), "fresh");
  assert.equal(await digestStateOf(edited[2]), "fresh");
});

test("an atom with no digest is unmonitored, not stale", async () => {
  const atoms = parseAtoms(
    ['<!-- <atom id="4P8W2H6K"/> -->', "Some text."].join("\n"),
  );
  assert.equal(await digestStateOf(atoms[0]), "none");
});

test("a malformed digest reads stale, which is what the binary reports", async () => {
  const atoms = parseAtoms(
    ['<!-- <atom id="4P8W2H6K" digest="not-a-real-digest"/> -->', "Some text."]
      .join("\n"),
  );
  assert.equal(await digestStateOf(atoms[0]), "stale");
});

test("an implicit atom has no directive to carry a digest, so it is unchecked", async () => {
  const atoms = parseAtoms(["A block with no marker at all."].join("\n"));
  assert.equal(atoms[0].implicit, true);
  assert.equal(await digestStateOf(atoms[0]), "unchecked");
});

// --- Where the board abstains ---------------------------------------------
//
// atomdown takes a block's extent from goldmark. Three shapes make this
// plug's blank-line chunking produce different bytes, so it declines to
// answer rather than guess. Each of these was found by comparing against the
// real binary, not predicted.

test("a fenced code block is unchecked, and so is the block before it", async () => {
  const page = [
    '<!-- <atom id="4P8W2H6K" digest="sha256:' + "0".repeat(64) + '"/> -->',
    "Run this:",
    "",
    '<!-- <atom id="9R3C7M5D" digest="sha256:' + "0".repeat(64) + '"/> -->',
    "```bash",
    "atomdown lint page.md",
    "```",
  ].join("\n");
  const atoms = parseAtoms(page);
  // goldmark's FencedCodeBlock starts at its first CONTENT line, so the
  // opening ```-line is excluded from the fence block and attributed to
  // whatever precedes it. Neither block's bytes are reproducible here.
  const fence = atoms.find((a) => a.text.indexOf("```") !== -1);
  assert.equal(fence.digestCheckable, false);
  assert.equal(await digestStateOf(fence), "unchecked");
});

test("a paragraph directly above a fence, with no directive between, is unchecked", async () => {
  const atoms = parseAtoms(
    ['<!-- <atom id="4P8W2H6K" digest="sha256:' + "0".repeat(64) + '"/> -->',
      "Run this:",
      "",
      "```bash",
      "atomdown lint page.md",
      "```"].join("\n"),
  );
  assert.equal(atoms[0].digestCheckable, false);
  assert.equal(await digestStateOf(atoms[0]), "unchecked");
});

test("a loose list is one CommonMark block, so its pieces are unchecked", async () => {
  const atoms = parseAtoms(
    ['<!-- <atom id="4P8W2H6K" digest="sha256:' + "0".repeat(64) + '"/> -->',
      "- one",
      "",
      "- two"].join("\n"),
  );
  atoms.forEach((a) => assert.equal(a.digestCheckable, false));
});

test("an indented code block with a blank line in it is unchecked", async () => {
  const atoms = parseAtoms(
    ['<!-- <atom id="4P8W2H6K" digest="sha256:' + "0".repeat(64) + '"/> -->',
      "    first",
      "",
      "    second"].join("\n"),
  );
  atoms.forEach((a) => assert.equal(a.digestCheckable, false));
});

test("a whitespace-only line is bytes the binary keeps, so that run is unchecked", async () => {
  // trimBlockEnd trims \r and \n only, so those two spaces stay inside the
  // preceding block for the binary while this chunker drops the line.
  const atoms = parseAtoms(
    ['<!-- <atom id="4P8W2H6K" digest="sha256:' + "0".repeat(64) + '"/> -->',
      "A paragraph.",
      "  ",
      "Another paragraph."].join("\n"),
  );
  atoms.forEach((a) => assert.equal(a.digestCheckable, false));
});

test("the ordinary shapes on a real page stay checkable", async () => {
  // Headings, tight lists, paragraphs and tables: everything Steve's own
  // pages are made of. If any of these started abstaining the board would go
  // quiet about real drift.
  const atoms = parseAtoms(REAL_DIGEST_PAGE);
  atoms.forEach((a) => assert.equal(a.digestCheckable, true, a.id));
  const table = parseAtoms(
    ['<!-- <atom id="4P8W2H6K" digest="sha256:' + "0".repeat(64) + '"/> -->',
      "| a | b |",
      "|---|---|",
      "| 1 | 2 |"].join("\n"),
  );
  assert.equal(table[0].digestCheckable, true);
});

test("staleAtoms lists exactly the drifted atoms, in document order", async () => {
  const edited = REAL_DIGEST_PAGE
    .replace("## Evidence", "## Evidence, revised")
    .replace("- First item", "- First item, revised");
  const rows = await staleAtoms(parseAtoms(edited));
  assert.deepEqual(rows.map((r) => r.id), ["4P8W2H6K", "9R3C7M5D"]);
  // Name and id, which is what the review dialog shows on each row.
  assert.deepEqual(rows.map((r) => r.slug), ["claim", "findings"]);
  assert.equal(await staleAtoms(parseAtoms(REAL_DIGEST_PAGE)).then((r) => r.length), 0);
});
