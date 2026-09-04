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

// options:
//   page          - what editor.getCurrentPage reports (default "Board")
//   store         - the clientStore contents to start from
//   storeThrows   - every clientStore call rejects, the way it would in a
//                   browser with site data blocked
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
  assert.ok(html.includes("border: 2px solid var(--ui-accent-color)"));
  assert.ok(html.includes("background: var(--ui-accent-color)"));
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
  assert.ok(rule.includes("border: 2px solid var(--ui-accent-color)"));
  assert.ok(rule.includes("outline: 2px solid var(--ui-accent-color)"));
  assert.ok(rule.includes("outline-offset: 2px"));
  assert.ok(rule.includes("background: var(--ui-surface-hover-background-color)"));
  // The old "grouped card keeps a thicker left edge" special case is gone.
  assert.ok(!html.includes(".board-card-grouped.board-card-selected"));
});

test("a collapsed group's cards are hidden, and nothing else changes", () => {
  const open = boardHtml(TIGHT_GROUP, []);
  const shut = boardHtml(TIGHT_GROUP, ["KF53ASNE"]);
  assert.ok(!open.includes("board-group-collapsed"));
  assert.ok(open.includes('data-group-cards="KF53ASNE">'));
  assert.ok(shut.includes("board-group-collapsed"));
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
