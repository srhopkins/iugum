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

function recordingSyscall(text) {
  const calls = [];
  const state = { text };
  globalThis.syscall = async function (name, ...args) {
    calls.push({ name, args });
    if (name === "editor.getText") return state.text;
    if (name === "editor.getCurrentPage") return "Board";
    if (name === "editor.replaceRange") {
      const [from, to, insert] = args;
      state.text = state.text.slice(0, from) + insert + state.text.slice(to);
      return;
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
