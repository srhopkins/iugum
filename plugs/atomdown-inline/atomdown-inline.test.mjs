// Unit tests for the atomdown-inline plug's pure functions.
//
// Run directly:   node --test plugs/atomdown-inline/
// Run from Go:    go test ./plugs/atomdown-inline
//
// These test the real module, not a reimplementation: the plug file is
// imported as-is, with only the worker globals it expects stubbed. There is no
// bundler and no package.json here on purpose — the plug is hand-authored
// ES2020 with no imports, so node loads it directly.
//
// What is worth testing here is the decoration payload. Everything the reader
// sees is that one object, so a wrong offset in it is the whole bug class this
// feature can have, and it is testable without a browser.

import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

globalThis.self = {
  addEventListener() {},
  postMessage() {},
};

const { plug } = await import("./atomdown-inline.plug.js");
const {
  computeUnits,
  computeCards,
  reorderUnit,
  insertGroupMarkers,
  removeGroupMarkers,
  setGroupSlugInSource,
  setAtomSlugInSource,
  firstBoxKey,
  cardMenuItems,
  minimalEdit,
  newAtomdownId,
  existingIds,
  sanitizeSlug,
  slugConflict,
  deriveGroupSlug,
  slugOrId,
  dedupeKeys,
  isContiguousUnitSelection,
  lineStarts,
  gripLine,
  contentFirstLine,
  hasNoContent,
  cardHeaderHtml,
  directivePeekHtml,
  toggleAction,
  toggleCollapsed,
  collapsedKey,
  buildDecorations,
  emptyDecorations,
  firstUnitKey,
  dragToReorder,
  lassoToUnitKeys,
  menuState,
  inlineOnKey,
  groupHeaderHtml,
} = plug.internals;

// A page with the shapes that matter: the document marker, a standalone atom,
// a named group of two atoms, and an implicit atom with no directive.
const PAGE = [
  '<!-- <atomdown version="1"/> -->',
  "",
  '<!-- <atom id="4P8W2H6K" slug="claim"/> -->',
  "# Claim",
  "",
  '<!-- <atom-group id="7K3M9X2D" slug="findings"> -->',
  '<!-- <atom id="AAAAAAAA"/> -->',
  "- first finding",
  "",
  '<!-- <atom id="BBBBBBBB"/> -->',
  "- second finding",
  "<!-- </atom-group> -->",
  "",
  "A paragraph nobody gave an id.",
  "",
].join("\n");

function offsetOf(text, needle) {
  const at = text.indexOf(needle);
  assert.notEqual(at, -1, `page has no ${needle}`);
  return at;
}

// ---------------------------------------------------------------------------
// Scanning
// ---------------------------------------------------------------------------

test("computeUnits finds the three movable units in document order", () => {
  const { units, preambleEndLine } = computeUnits(PAGE);
  assert.equal(preambleEndLine, 0);
  assert.deepEqual(units.map((u) => u.unitKey), [
    "atom:4P8W2H6K",
    "group:7K3M9X2D",
    "atom:implicit-1",
  ]);
  assert.deepEqual(units.map((u) => u.kind), ["atom", "group", "atom"]);
});

test("a group unit spans its own markers and nothing else", () => {
  const { lines, units } = computeUnits(PAGE);
  const group = units.find((u) => u.unitKey === "group:7K3M9X2D");
  assert.match(lines[group.startLine], /<atom-group id="7K3M9X2D"/);
  assert.match(lines[group.endLine], /<\/atom-group>/);
  assert.deepEqual(group.atomIds, ["AAAAAAAA", "BBBBBBBB"]);
  assert.equal(group.groupSlug, "findings");
});

test("computeCards gives each atom inside a group its own card", () => {
  const { cards } = computeCards(PAGE);
  assert.deepEqual(cards.map((c) => c.cardKey), [
    "atom:4P8W2H6K",
    "atom:AAAAAAAA",
    "atom:BBBBBBBB",
    "atom:implicit-1",
  ]);
  const members = cards.filter((c) => c.groupUnitKey === "group:7K3M9X2D");
  assert.equal(members.length, 2);
  assert.deepEqual(members.map((c) => c.atomIds[0]), ["AAAAAAAA", "BBBBBBBB"]);
});

test("a member card's mark is a box name, never a unit name", () => {
  // The drag code reads only `unit:` names, so a member card's own box mark
  // can never be mistaken for something draggable: its group is what moves.
  const payload = buildDecorations(PAGE, []);
  const memberBoxes = payload.marks
    .filter((m) => m.class === "atomdown-card" && m.id.startsWith("box:"))
    .map((m) => m.id);
  memberBoxes.forEach((id) => assert.equal(firstUnitKey([id]), null));
});

test("an empty page produces no units and no cards", () => {
  assert.deepEqual(computeUnits("").units, []);
  assert.deepEqual(computeCards("").cards, []);
});

test("lineStarts is the running character offset of each line", () => {
  assert.deepEqual(lineStarts(["ab", "c", "", "de"]), [0, 3, 5, 6]);
});

test("a grip sits on the first content line, not the directive line", () => {
  const { units } = computeUnits(PAGE);
  const atom = units[0];
  assert.equal(gripLine(atom), atom.startLine + 1);
  const implicit = units[2];
  assert.equal(gripLine(implicit), implicit.startLine);
});

// ---------------------------------------------------------------------------
// The decoration payload
// ---------------------------------------------------------------------------

test("the payload turns the view on: marks, widgets, folds, events, gestures", () => {
  const payload = buildDecorations(PAGE, []);
  assert.equal(payload.activeLine, true);
  assert.equal(payload.events.click, true);
  assert.equal(payload.events.selection, true);
  assert.deepEqual(payload.gestures.drag, { handleClass: "atomdown-grip" });
  assert.deepEqual(payload.gestures.lasso, { modifier: "alt" });
  assert.deepEqual(
    payload.lines.map((l) => l.selector),
    ["CommentBlock", "Comment"],
  );
  payload.lines.forEach((l) => assert.equal(l.class, "atomdown-directive"));
});

test("every unit gets one identity mark that draws nothing", () => {
  const payload = buildDecorations(PAGE, []);
  const unitMarks = payload.marks.filter((m) => m.id.startsWith("unit:"));
  assert.deepEqual(unitMarks.map((m) => m.id), [
    "unit:atom:4P8W2H6K",
    "unit:group:7K3M9X2D",
    "unit:atom:implicit-1",
  ]);
  // No line classes, so no border, no background, nothing.
  unitMarks.forEach((m) => {
    assert.equal(m.class, "atomdown-unit");
    assert.equal(m.lineClasses, undefined);
  });
});

test("an identity mark covers the unit's directive lines too", () => {
  const payload = buildDecorations(PAGE, []);
  const mark = payload.marks.find((m) => m.id === "unit:atom:4P8W2H6K");
  assert.equal(mark.from, offsetOf(PAGE, '<!-- <atom id="4P8W2H6K"'));
});

test("a card box starts BELOW the directive line, so its top edge is visible", () => {
  const payload = buildDecorations(PAGE, []);
  const box = payload.marks.find((m) => m.id === "box:atom:4P8W2H6K");
  assert.equal(box.class, "atomdown-card");
  assert.equal(box.lineClasses, true);
  assert.equal(box.from, offsetOf(PAGE, "# Claim"));
  assert.equal(PAGE.slice(box.from, box.to), "# Claim");
});

test("no box mark covers a blank line, which is what makes the gap", () => {
  const payload = buildDecorations(PAGE, []);
  payload.marks
    .filter((m) => m.lineClasses && m.class === "atomdown-card")
    .forEach((m) => {
      const covered = PAGE.slice(m.from, m.to).split("\n");
      covered.forEach((line) =>
        assert.notEqual(line.trim(), "", "card box covers a blank line: " + m.id)
      );
    });
});

test("a card box never covers a directive line", () => {
  const payload = buildDecorations(PAGE, []);
  payload.marks
    .filter((m) => m.class === "atomdown-card")
    .forEach((m) => {
      PAGE.slice(m.from, m.to).split("\n").forEach((line) =>
        assert.equal(
          line.includes("<!--"),
          false,
          "card box covers a directive: " + m.id,
        )
      );
    });
});

test("a one-line block's box is first and last on the same line", () => {
  // "# Claim" is one line, so its box mark starts and ends on that line and
  // the stylesheet's first+last rule has to close the whole box there.
  const payload = buildDecorations(PAGE, []);
  const box = payload.marks.find((m) => m.id === "box:atom:4P8W2H6K");
  assert.equal(PAGE.slice(box.from, box.to).includes("\n"), false);
});

test("a multi-line block's box spans every one of its lines", () => {
  const page = [
    '<!-- <atom id="AAAAAAAA"/> -->',
    "line one",
    "line two",
    "line three",
    "",
  ].join("\n");
  const box = buildDecorations(page, []).marks.find(
    (m) => m.id === "box:atom:AAAAAAAA",
  );
  assert.equal(page.slice(box.from, box.to), "line one\nline two\nline three");
});

test("an atom with a directive but no content gets no box and no header", () => {
  const page = '<!-- <atom id="AAAAAAAA"/> -->\n\n';
  const payload = buildDecorations(page, []);
  assert.equal(payload.marks.some((m) => m.id.startsWith("box:")), false);
  assert.deepEqual(payload.widgets, []);
});

test("a group box runs marker to marker, so both markers are inside it", () => {
  const payload = buildDecorations(PAGE, []);
  const box = payload.marks.find((m) => m.id === "box:group:7K3M9X2D");
  assert.equal(box.class, "atomdown-group");
  assert.equal(box.lineClasses, true);
  const covered = PAGE.slice(box.from, box.to);
  assert.equal(covered.startsWith('<!-- <atom-group id="7K3M9X2D"'), true);
  assert.equal(covered.endsWith("<!-- </atom-group> -->"), true);
});

test("each atom inside a group gets its own card box, inside the group's", () => {
  const payload = buildDecorations(PAGE, []);
  const group = payload.marks.find((m) => m.id === "box:group:7K3M9X2D");
  const members = payload.marks.filter((m) =>
    m.id === "box:atom:AAAAAAAA" || m.id === "box:atom:BBBBBBBB"
  );
  assert.deepEqual(members.map((m) => m.id), [
    "box:atom:AAAAAAAA",
    "box:atom:BBBBBBBB",
  ]);
  members.forEach((m) => {
    assert.equal(m.class, "atomdown-card");
    assert.equal(m.lineClasses, true);
    assert.equal(m.from > group.from, true);
    assert.equal(m.to < group.to, true);
  });
});

test("every card gets a header widget carrying its name and its id", () => {
  const payload = buildDecorations(PAGE, []);
  const heads = payload.widgets.filter((w) =>
    w.class.startsWith("atomdown-card-header")
  );
  assert.deepEqual(heads.map((w) => w.id), [
    "box:atom:4P8W2H6K",
    "box:atom:AAAAAAAA",
    "box:atom:BBBBBBBB",
    "box:atom:implicit-1",
  ]);
  const named = heads[0];
  assert.match(named.html, /atomdown-card-slug/);
  assert.match(named.html, /claim/);
  assert.match(named.html, /atomdown-card-id/);
  assert.match(named.html, /4P8W2H6K/);
  assert.match(named.html, /atomdown-grip/);
});

test("a card header sits at the top of the box it belongs to", () => {
  const payload = buildDecorations(PAGE, []);
  const head = payload.widgets.find((w) => w.id === "box:atom:4P8W2H6K");
  const box = payload.marks.find((m) => m.id === "box:atom:4P8W2H6K");
  assert.equal(head.side, "before");
  assert.equal(head.at, box.from);
});

test("a member card's header is marked nested, a top-level card's is not", () => {
  const payload = buildDecorations(PAGE, []);
  const nested = payload.widgets.find((w) => w.id === "box:atom:AAAAAAAA");
  const top = payload.widgets.find((w) => w.id === "box:atom:4P8W2H6K");
  assert.match(nested.class, /atomdown-nested/);
  assert.equal(/atomdown-nested/.test(top.class), false);
  assert.match(nested.html, /atomdown-nested/);
});

test("an implicit block is badged 'no id' rather than given a fake one", () => {
  const payload = buildDecorations(PAGE, []);
  const head = payload.widgets.find((w) => w.id === "box:atom:implicit-1");
  assert.match(head.html, /atomdown-card-badge/);
  assert.match(head.html, /no id/);
  // No card-id element, so the synthetic key is never shown as an Atomdown id.
  assert.equal(/atomdown-card-id/.test(head.html), false);
});

test("a group gets a header widget on its opening marker line", () => {
  const payload = buildDecorations(PAGE, []);
  const widget = payload.widgets.find((w) =>
    w.class === "atomdown-group-header"
  );
  assert.equal(widget.id, "unit:group:7K3M9X2D");
  assert.equal(widget.side, "before");
  assert.equal(widget.at, offsetOf(PAGE, '<!-- <atom-group id="7K3M9X2D"'));
  assert.match(widget.html, /findings/);
  assert.match(widget.html, /7K3M9X2D/);
  assert.match(widget.html, /2 cards/);
  assert.match(widget.html, /atomdown-grip/);
  assert.match(widget.html, /atomdown-group-collapse/);
  assert.match(widget.html, /atomdown-group-kind/);
  assert.match(widget.html, /atomdown-group-menu/);
});

test("a group with one card says card, not cards", () => {
  const html = groupHeaderHtml({ groupId: "AAAAAAAA", groupSlug: "x" }, 1);
  assert.match(html, /1 card</);
  assert.match(groupHeaderHtml({ groupId: "AAAAAAAA", groupSlug: "x" }, 3), /3 cards</);
});

test("a group with no slug shows its id as the name", () => {
  const html = groupHeaderHtml({ groupId: "7K3M9X2D", groupSlug: null }, 0);
  assert.match(html, /7K3M9X2D/);
});

// ---------------------------------------------------------------------------
// The card's controls
// ---------------------------------------------------------------------------

test("a card header carries a grip and a three-dot menu", () => {
  const payload = buildDecorations(PAGE, []);
  const head = payload.widgets.find((w) => w.id === "box:atom:4P8W2H6K");
  assert.match(head.html, /atomdown-grip/);
  assert.match(head.html, /atomdown-card-menu/);
  // The menu is reachable by keyboard, not pointer-only.
  assert.match(head.html, /role="button"/);
  assert.match(head.html, /tabindex="0"/);
});

test("the grip comes before the slug in the markup, so it renders left", () => {
  const payload = buildDecorations(PAGE, []);
  const head = payload.widgets.find((w) => w.id === "box:atom:4P8W2H6K");
  assert.equal(
    head.html.indexOf("atomdown-grip") < head.html.indexOf("atomdown-card-slug"),
    true,
  );
  assert.equal(
    head.html.indexOf("atomdown-card-slug") <
      head.html.indexOf("atomdown-card-menu"),
    true,
  );
});

test("a group header carries one menu, not two buttons", () => {
  const payload = buildDecorations(PAGE, []);
  const head = payload.widgets.find((w) =>
    w.class.startsWith("atomdown-group-header")
  );
  assert.match(head.html, /atomdown-group-menu/);
  assert.equal(/atomdown-group-btn/.test(head.html), false);
  assert.equal(/>Rename</.test(head.html), false);
  assert.equal(/>Ungroup</.test(head.html), false);
});

test("firstBoxKey finds the card the pointer is on, ignoring units", () => {
  assert.equal(
    firstBoxKey(["unit:group:G", "box:group:G", "box:atom:AAAAAAAA"]),
    "atom:AAAAAAAA",
  );
  assert.equal(firstBoxKey(["unit:atom:A", "box:group:G"]), null);
  assert.equal(firstBoxKey([]), null);
  assert.equal(firstBoxKey(undefined), null);
});

test("the card menu leads with identity and that entry does nothing", () => {
  const items = cardMenuItems("claim", "4P8W2H6K", false, false);
  assert.equal(items[0].action, "label");
  assert.match(items[0].name, /claim/);
  assert.match(items[0].name, /4P8W2H6K/);
  assert.deepEqual(items.map((i) => i.action), [
    "label",
    "copy-id",
    "copy-slug",
    "rename",
    "reveal",
    "group",
  ]);
});

test("a card inside a group offers Ungroup instead of Group", () => {
  const items = cardMenuItems("claim", "4P8W2H6K", false, true);
  assert.equal(items[items.length - 1].action, "ungroup");
});

test("a slugless atom is not offered Copy name", () => {
  const items = cardMenuItems("4P8W2H6K", "4P8W2H6K", false, false);
  assert.equal(items.some((i) => i.action === "copy-slug"), false);
});

test("an implicit block's menu offers no id action at all", () => {
  const items = cardMenuItems("implicit-1", null, true, false);
  assert.deepEqual(items.map((i) => i.action), ["label", "group"]);
  assert.match(items[0].name, /no atom directive/);
});

test("renaming an atom rewrites only its directive line", () => {
  const result = setAtomSlugInSource(PAGE, "4P8W2H6K", "The Claim!");
  assert.equal(result.ok, true);
  assert.equal(result.slug, "the-claim");
  const before = PAGE.split("\n");
  const changed = result.text.split("\n").filter((line, i) => line !== before[i]);
  assert.equal(changed.length, 1);
  assert.match(changed[0], /id="4P8W2H6K" slug="the-claim"/);
  // The block's own text is untouched, so no digest can go stale.
  assert.equal(result.text.includes("# Claim"), true);
});

test("renaming an atom keeps its other attributes, id first", () => {
  const page = '<!-- <atom id="AAAAAAAA" slug="old" acme-x="1"/> -->\nhi\n';
  const result = setAtomSlugInSource(page, "AAAAAAAA", "new");
  assert.match(result.text, /id="AAAAAAAA" slug="new" acme-x="1"/);
});

test("renaming an atom to nothing removes the slug attribute", () => {
  const result = setAtomSlugInSource(PAGE, "4P8W2H6K", "  ");
  assert.match(result.text, /<atom id="4P8W2H6K" \/>/);
});

test("renaming an atom that is gone fails instead of guessing", () => {
  const result = setAtomSlugInSource(PAGE, "NOSUCHID", "x");
  assert.equal(result.ok, false);
});

test("no inline grip widget: the grip lives in the card header row", () => {
  const payload = buildDecorations(PAGE, []);
  assert.equal(payload.widgets.some((w) => w.inline), false);
});

test("a group is one foldable region: everything after its opening marker", () => {
  const payload = buildDecorations(PAGE, []);
  assert.equal(payload.folds.length, 1);
  const fold = payload.folds[0];
  const group = payload.marks.find((m) => m.id === "box:group:7K3M9X2D");
  assert.equal(PAGE[fold.from], "\n");
  assert.equal(fold.to, group.to);
});

test("a page with no group has nothing to fold", () => {
  const payload = buildDecorations('<!-- <atom id="4P8W2H6K"/> -->\nHi\n', []);
  assert.deepEqual(payload.folds, []);
});

test("a selected unit gets one extra mark over the box it draws", () => {
  const plain = buildDecorations(PAGE, []);
  const picked = buildDecorations(PAGE, ["atom:4P8W2H6K", "atom:4P8W2H6K"]);
  assert.equal(picked.marks.length, plain.marks.length + 1);
  const sel = picked.marks.find((m) => m.id === "sel:atom:4P8W2H6K");
  const box = picked.marks.find((m) => m.id === "box:atom:4P8W2H6K");
  assert.equal(sel.class, "atomdown-selected");
  assert.equal(sel.lineClasses, true);
  assert.equal(sel.from, box.from);
  assert.equal(sel.to, box.to);
});

test("a selection key that names nothing adds no mark", () => {
  const plain = buildDecorations(PAGE, []);
  const bogus = buildDecorations(PAGE, ["atom:NOSUCHID"]);
  assert.equal(bogus.marks.length, plain.marks.length);
});

test("no mark or widget offset can fall outside the page", () => {
  const payload = buildDecorations(PAGE, ["group:7K3M9X2D"]);
  payload.marks.forEach((m) => {
    assert.equal(m.from >= 0 && m.to <= PAGE.length, true, m.id);
    assert.equal(m.to > m.from, true, m.id);
  });
  payload.widgets.forEach((w) => {
    assert.equal(w.at >= 0 && w.at <= PAGE.length, true, w.id);
  });
});

test("content starts below a directive, and at the block for an implicit one", () => {
  const { units } = computeUnits(PAGE);
  assert.equal(contentFirstLine(units[0]), units[0].startLine + 1);
  assert.equal(contentFirstLine(units[2]), units[2].startLine);
  assert.equal(gripLine(units[0]), contentFirstLine(units[0]));
});

test("hasNoContent spots a directive with nothing after it", () => {
  const page = '<!-- <atom id="AAAAAAAA"/> -->\n\nreal block\n';
  const scan = computeUnits(page);
  assert.equal(hasNoContent(scan.units[0], scan.lines), true);
  assert.equal(hasNoContent(scan.units[1], scan.lines), false);
});

test("a card header for a slugless explicit atom still shows the id", () => {
  const html = cardHeaderHtml(
    { atomIds: ["AAAAAAAA"], atomSlug: null, implicit: false },
    false,
  );
  assert.match(html, /AAAAAAAA/);
  assert.equal(/atomdown-card-slug/.test(html), false);
});

// ---------------------------------------------------------------------------
// The directive peek: the ONLY reveal, and it costs no layout
// ---------------------------------------------------------------------------

test("every card header carries a peek of its own directive line", () => {
  const payload = buildDecorations(PAGE, []);
  const head = payload.widgets.find((w) => w.id === "box:atom:4P8W2H6K");
  assert.match(head.html, /atomdown-directive-peek/);
  // The exact source line, escaped, so a reader sees the bytes on disk.
  assert.match(head.html, /&lt;!-- &lt;atom id=&quot;4P8W2H6K&quot;/);
});

test("a group header carries a peek of its opening marker", () => {
  const payload = buildDecorations(PAGE, []);
  const head = payload.widgets.find((w) =>
    w.class === "atomdown-group-header"
  );
  assert.match(head.html, /atomdown-directive-peek/);
  assert.match(head.html, /atom-group id=&quot;7K3M9X2D&quot;/);
});

test("an implicit block has no directive, so its header has no peek", () => {
  const payload = buildDecorations(PAGE, []);
  const head = payload.widgets.find((w) => w.id === "box:atom:implicit-1");
  assert.equal(/atomdown-directive-peek/.test(head.html), false);
});

test("a peek escapes its text, so a directive cannot inject markup", () => {
  const html = directivePeekHtml('<!-- <atom id="A" x="<b>&y"/> -->');
  assert.equal(html.includes("<b>"), false);
  assert.match(html, /&lt;b&gt;/);
  assert.match(html, /&amp;y/);
});

test("an empty or blank directive line produces no peek at all", () => {
  assert.equal(directivePeekHtml(""), "");
  assert.equal(directivePeekHtml("   "), "");
  assert.equal(directivePeekHtml(null), "");
  assert.equal(directivePeekHtml(undefined), "");
});

// ---------------------------------------------------------------------------
// The toggle's decision - the button's own code path
// ---------------------------------------------------------------------------

test("the toggle follows what is on screen, not the remembered flag", () => {
  // The bug this exists for: a page load that arrived before the editor had
  // text leaves the flag set and draws nothing. A press must then turn the
  // view ON, or the button looks dead.
  assert.equal(toggleAction(true, false), "on");
  // The ordinary cases.
  assert.equal(toggleAction(false, false), "on");
  assert.equal(toggleAction(true, true), "off");
  assert.equal(toggleAction(false, true), "off");
});

// ---------------------------------------------------------------------------
// Collapse: declarative, idempotent, persisted
// ---------------------------------------------------------------------------

test("collapsing is a set, so the caret can never desynchronise", () => {
  // The bug this replaces alternated fold/unfold from memory, so one fold
  // from the gutter left the caret stuck.
  assert.deepEqual(toggleCollapsed([], "AAAAAAAA"), ["AAAAAAAA"]);
  assert.deepEqual(toggleCollapsed(["AAAAAAAA"], "AAAAAAAA"), []);
  assert.deepEqual(
    toggleCollapsed(["AAAAAAAA"], "BBBBBBBB"),
    ["AAAAAAAA", "BBBBBBBB"],
  );
  assert.deepEqual(toggleCollapsed(undefined, "AAAAAAAA"), ["AAAAAAAA"]);
  // Idempotent in the sense that matters: two flips return to the start.
  const once = toggleCollapsed([], "AAAAAAAA");
  assert.deepEqual(toggleCollapsed(once, "AAAAAAAA"), []);
});

test("a collapsed group asks the seam to fold it, and says so in its caret", () => {
  const open = buildDecorations(PAGE, [], []);
  const shut = buildDecorations(PAGE, [], ["7K3M9X2D"]);
  assert.equal(open.folds[0].collapsed, undefined);
  assert.equal(shut.folds[0].collapsed, true);
  // Same range either way: collapsing changes a flag, never a geometry.
  assert.equal(shut.folds[0].from, open.folds[0].from);
  assert.equal(shut.folds[0].to, open.folds[0].to);

  const openHead = open.widgets.find((w) =>
    w.class.startsWith("atomdown-group-header")
  );
  const shutHead = shut.widgets.find((w) =>
    w.class.startsWith("atomdown-group-header")
  );
  assert.equal(/atomdown-group-collapsed/.test(openHead.class), false);
  assert.match(shutHead.class, /atomdown-group-collapsed/);
  assert.match(openHead.html, /&#9662;|▾/);
  assert.match(shutHead.html, /&#9656;|▸/);
  assert.match(shutHead.html, /Expand this group/);
  assert.match(openHead.html, /Collapse this group/);
});

test("collapsing changes no mark and no other widget", () => {
  const open = buildDecorations(PAGE, [], []);
  const shut = buildDecorations(PAGE, [], ["7K3M9X2D"]);
  assert.deepEqual(shut.marks, open.marks);
  assert.equal(shut.widgets.length, open.widgets.length);
});

test("an unknown collapsed id collapses nothing", () => {
  const shut = buildDecorations(PAGE, [], ["NOSUCHID"]);
  assert.equal(shut.folds[0].collapsed, undefined);
});

test("the collapsed set is keyed by page, like the on/off flag", () => {
  assert.equal(
    collapsedKey("Todo/running"),
    "atomdown-inline.collapsed:Todo/running",
  );
  assert.notEqual(collapsedKey("a"), collapsedKey("b"));
  assert.notEqual(collapsedKey("a"), inlineOnKey("a"));
});

test("turning the view off writes an empty payload, not a missing key", () => {
  const off = emptyDecorations();
  assert.equal(off.activeLine, false);
  assert.deepEqual(off.marks, []);
  assert.deepEqual(off.widgets, []);
  assert.deepEqual(off.lines, []);
  assert.deepEqual(off.folds, []);
  assert.deepEqual(off.events, {});
  assert.deepEqual(off.gestures, {});
});

// ---------------------------------------------------------------------------
// Reading the seam's events
// ---------------------------------------------------------------------------

test("firstUnitKey reads past a selection mark to the unit mark", () => {
  assert.equal(
    firstUnitKey(["sel:atom:X", "unit:group:G", "card:G:0"]),
    "group:G",
  );
  assert.equal(firstUnitKey(["card:G:0"]), null);
  assert.equal(firstUnitKey([]), null);
  assert.equal(firstUnitKey(undefined), null);
});

test("a drag reports the unit under the handle and the unit under the drop", () => {
  const request = dragToReorder({
    marks: ["unit:atom:4P8W2H6K"],
    targetMarks: ["unit:group:7K3M9X2D", "card:7K3M9X2D:1"],
    placement: "after",
  }, ["atom:4P8W2H6K", "group:7K3M9X2D"]);
  assert.deepEqual(request, {
    movedUnitKey: "atom:4P8W2H6K",
    targetUnitKey: "group:7K3M9X2D",
    placement: "after",
  });
});

test("a drag onto a group member moves the whole group", () => {
  // The seam orders covering marks outermost first, so the group wins.
  const request = dragToReorder({
    marks: ["unit:group:7K3M9X2D", "card:7K3M9X2D:0"],
    targetMarks: ["unit:atom:implicit-1"],
    placement: "before",
  }, ["group:7K3M9X2D", "atom:implicit-1"]);
  assert.equal(request.movedUnitKey, "group:7K3M9X2D");
});

test("a drop on the block it came from asks for nothing", () => {
  assert.equal(
    dragToReorder({
      marks: ["unit:atom:A"],
      targetMarks: ["unit:atom:A"],
      placement: "after",
    }, ["atom:A"]),
    null,
  );
});

test("a drag with no unit under the handle asks for nothing", () => {
  assert.equal(
    dragToReorder({ marks: [], targetMarks: ["unit:atom:A"] }, ["atom:A"]),
    null,
  );
});

test("a drop past every block lands at the end of the page", () => {
  assert.deepEqual(
    dragToReorder({
      marks: ["unit:atom:A"],
      targetMarks: [],
      placement: "after",
    }, ["atom:A", "atom:B"]),
    { movedUnitKey: "atom:A", targetUnitKey: null, placement: "end" },
  );
});

test("a drop above every block lands at the start of the page", () => {
  assert.deepEqual(
    dragToReorder({
      marks: ["unit:atom:B"],
      targetMarks: [],
      placement: "before",
    }, ["atom:A", "atom:B"]),
    { movedUnitKey: "atom:B", targetUnitKey: null, placement: "start" },
  );
});

test("a lasso reports each swept unit once, member cards excluded", () => {
  assert.deepEqual(
    lassoToUnitKeys({
      marks: [
        "unit:atom:A",
        "card:G:0",
        "unit:group:G",
        "unit:atom:A",
        "sel:atom:A",
      ],
    }),
    ["atom:A", "group:G"],
  );
  assert.deepEqual(lassoToUnitKeys({ marks: [] }), []);
  assert.deepEqual(lassoToUnitKeys(undefined), []);
});

// ---------------------------------------------------------------------------
// The menu's rules
// ---------------------------------------------------------------------------

test("the menu on a group offers Ungroup", () => {
  const state = menuState(["group:G"], [], "group:G");
  assert.equal(state.action, "ungroup");
  assert.equal(state.enabled, true);
});

test("Group needs two blocks", () => {
  const state = menuState(["atom:A", "atom:B"], ["atom:A"], null);
  assert.equal(state.enabled, false);
  assert.match(state.reason, /two or more/);
});

test("Group refuses a group inside a group", () => {
  const state = menuState(
    ["atom:A", "group:G"],
    ["atom:A", "group:G"],
    null,
  );
  assert.equal(state.enabled, false);
  assert.match(state.reason, /does not permit a group inside a group/);
});

test("Group refuses blocks that are not next to each other", () => {
  const state = menuState(
    ["atom:A", "atom:B", "atom:C"],
    ["atom:A", "atom:C"],
    null,
  );
  assert.equal(state.enabled, false);
  assert.match(state.reason, /not next to each other/);
});

test("Group accepts an adjacent pair", () => {
  const state = menuState(
    ["atom:A", "atom:B", "atom:C"],
    ["atom:B", "atom:C"],
    null,
  );
  assert.equal(state.enabled, true);
});

test("contiguity does not care what order the keys arrive in", () => {
  assert.equal(
    isContiguousUnitSelection(["a", "b", "c"], ["c", "b"]),
    true,
  );
  assert.equal(isContiguousUnitSelection(["a", "b", "c"], ["c", "a"]), false);
  assert.equal(isContiguousUnitSelection(["a"], []), false);
});

test("dedupeKeys keeps first-seen order", () => {
  assert.deepEqual(dedupeKeys(["b", "a", "b"]), ["b", "a"]);
  assert.deepEqual(dedupeKeys(undefined), []);
});

// ---------------------------------------------------------------------------
// Writing the document
// ---------------------------------------------------------------------------

test("a reorder moves the block and changes nothing else", () => {
  const result = reorderUnit(PAGE, "atom:implicit-1", "atom:4P8W2H6K", "before");
  assert.equal(result.ok, true);
  const before = computeUnits(PAGE).units.map((u) => u.unitKey);
  const after = computeUnits(result.text).units.map((u) => u.unitKey);
  assert.deepEqual(before, [
    "atom:4P8W2H6K",
    "group:7K3M9X2D",
    "atom:implicit-1",
  ]);
  // The moved block is now first; the group and the named atom keep their ids.
  assert.equal(after[1], "atom:4P8W2H6K");
  assert.equal(after[2], "group:7K3M9X2D");
  // Every directive line survives byte for byte, so no id, slug or digest can
  // change from a reorder.
  const directives = (text) =>
    text.split("\n").filter((l) => l.includes("<atom")).sort();
  assert.deepEqual(directives(result.text), directives(PAGE));
});

test("a reorder is one minimal replacement, so one Cmd-Z reverts it", () => {
  const result = reorderUnit(PAGE, "atom:implicit-1", "atom:4P8W2H6K", "before");
  const edit = minimalEdit(PAGE, result.text);
  assert.notEqual(edit, null);
  const rebuilt = PAGE.slice(0, edit.from) + edit.insert + PAGE.slice(edit.to);
  assert.equal(rebuilt, result.text);
});

test("a reorder that changes nothing reports unchanged", () => {
  const result = reorderUnit(PAGE, "atom:4P8W2H6K", "atom:4P8W2H6K", "after");
  assert.deepEqual(result, { ok: true, unchanged: true });
});

test("a reorder naming a block that is gone fails instead of guessing", () => {
  const result = reorderUnit(PAGE, "atom:NOPE", "atom:4P8W2H6K", "after");
  assert.equal(result.ok, false);
  assert.match(result.error, /dragged block/);
});

test("the document marker never moves", () => {
  const result = reorderUnit(PAGE, "atom:implicit-1", null, "start");
  assert.equal(result.ok, true);
  assert.equal(result.text.split("\n")[0], '<!-- <atomdown version="1"/> -->');
});

test("a group adds exactly two lines and touches no directive", () => {
  const page = [
    '<!-- <atom id="AAAAAAAA"/> -->',
    "one",
    "",
    '<!-- <atom id="BBBBBBBB"/> -->',
    "two",
    "",
  ].join("\n");
  const result = insertGroupMarkers(
    page,
    ["atom:AAAAAAAA", "atom:BBBBBBBB"],
    "7K3M9X2D",
    "My Findings",
  );
  assert.equal(result.ok, true);
  assert.equal(result.slug, "my-findings");
  assert.equal(
    result.text.split("\n").length,
    page.split("\n").length + 2,
  );
  assert.equal(result.text.includes('<atom id="AAAAAAAA"/>'), true);
  assert.equal(result.text.includes('<atom id="BBBBBBBB"/>'), true);
});

test("an ungroup is the exact inverse of a group", () => {
  const page = [
    '<!-- <atom id="AAAAAAAA"/> -->',
    "one",
    "",
    '<!-- <atom id="BBBBBBBB"/> -->',
    "two",
    "",
  ].join("\n");
  const grouped = insertGroupMarkers(
    page,
    ["atom:AAAAAAAA", "atom:BBBBBBBB"],
    "7K3M9X2D",
    "findings",
  );
  const back = removeGroupMarkers(grouped.text, "7K3M9X2D");
  assert.equal(back.ok, true);
  assert.equal(back.text, page);
});

test("a group refuses an id that is already used", () => {
  const result = insertGroupMarkers(
    PAGE,
    ["atom:4P8W2H6K", "group:7K3M9X2D"],
    "4P8W2H6K",
    "x",
  );
  assert.equal(result.ok, false);
});

test("a rename rewrites only the group's opening marker", () => {
  const result = setGroupSlugInSource(PAGE, "7K3M9X2D", "Open Questions");
  assert.equal(result.ok, true);
  assert.equal(result.slug, "open-questions");
  const changed = result.text.split("\n").filter(
    (line, i) => line !== PAGE.split("\n")[i],
  );
  assert.equal(changed.length, 1);
  assert.match(changed[0], /id="7K3M9X2D" slug="open-questions"/);
});

test("a rename to nothing removes the slug attribute", () => {
  const result = setGroupSlugInSource(PAGE, "7K3M9X2D", "   ");
  assert.equal(result.ok, true);
  assert.match(result.text, /<atom-group id="7K3M9X2D"> -->/);
});

test("a duplicate slug is written and reported, never refused", () => {
  const result = setGroupSlugInSource(PAGE, "7K3M9X2D", "claim");
  assert.equal(result.ok, true);
  assert.match(result.text, /slug="claim"/);
  assert.match(result.warning, /already used/);
});

test("slugConflict ignores the owner's own slug", () => {
  assert.equal(slugConflict(PAGE, "findings", "7K3M9X2D").duplicate, false);
  assert.equal(slugConflict(PAGE, "findings", null).duplicate, true);
  assert.equal(slugConflict(PAGE, "", null).duplicate, false);
});

test("a slug becomes lowercase kebab-case ASCII", () => {
  assert.equal(sanitizeSlug("Décisions & Notes!"), "decisions-notes");
  assert.equal(sanitizeSlug("   "), "");
  assert.equal(sanitizeSlug(null), "");
});

test("a group name is suggested from the first heading in the selection", () => {
  assert.equal(deriveGroupSlug(["- a", "## Open Questions\ntext"]), "open-questions");
  assert.equal(deriveGroupSlug([]), "group");
});

test("slugOrId falls back to the id", () => {
  assert.equal(slugOrId("findings", "7K3M9X2D"), "findings");
  assert.equal(slugOrId("  ", "7K3M9X2D"), "7K3M9X2D");
});

test("a new id is eight Crockford Base32 characters", () => {
  for (let i = 0; i < 50; i++) {
    assert.match(newAtomdownId(), /^[0-9A-HJKMNP-TV-Z]{8}$/);
  }
});

test("existingIds finds every id in the page", () => {
  assert.deepEqual(existingIds(PAGE).sort(), [
    "4P8W2H6K",
    "7K3M9X2D",
    "AAAAAAAA",
    "BBBBBBBB",
  ]);
});

test("minimalEdit reports null for no change", () => {
  assert.equal(minimalEdit("same", "same"), null);
});

test("the remembered flag is keyed by page name", () => {
  assert.equal(inlineOnKey("Todo/running"), "atomdown-inline.on:Todo/running");
  assert.notEqual(inlineOnKey("a"), inlineOnKey("b"));
});

// ---------------------------------------------------------------------------
// The manifest
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// The header button's contract
//
// The button is not a command call in this file - it is a name in the library
// page's actionButton.define, which the client resolves with
// runCommandByName. That resolution is by STRING, and nothing else in the
// build checks the two strings match: a rename on either side leaves a button
// that silently does nothing, which is exactly the failure that was reported.
// ---------------------------------------------------------------------------

const LIBRARY_PAGE = readFileSync(
  new URL("./library/Atomdown Inline.md", import.meta.url),
  "utf8",
);

test("the header button names a command this plug actually registers", () => {
  const buttons = [...LIBRARY_PAGE.matchAll(/actionButton\.define\s*\{([^}]*)\}/g)]
    .map((m) => m[1]);
  assert.equal(buttons.length, 1, "expected exactly one action button");
  const named = /command\s*=\s*"([^"]+)"/.exec(buttons[0]);
  assert.ok(named, "the action button must name a command");
  const commands = Object.values(plug.manifest.functions)
    .filter((fn) => fn.command)
    .map((fn) => fn.command.name);
  assert.ok(
    commands.includes(named[1]),
    `the button names "${named[1]}", which is not one of ${commands.join(", ")}`,
  );
  // And that command must resolve to a real function, or runCommandByName
  // finds an entry whose run() throws.
  const entry = Object.entries(plug.manifest.functions)
    .find(([, fn]) => fn.command && fn.command.name === named[1]);
  assert.equal(typeof plug.functionMapping[entry[0]], "function");
});

test("the button carries an icon, or the client filters it out entirely", () => {
  // client/editor_ui.tsx drops any actionButton without an icon.
  assert.match(LIBRARY_PAGE, /actionButton\.define\s*\{[^}]*icon\s*=\s*"[^"]+"/);
});

test("the manifest wires one command per user action and one function per event", () => {
  const fns = plug.manifest.functions;
  assert.equal(fns.toggleInline.command.name, "Atomdown: Toggle Inline View");
  assert.equal(fns.groupSelection.command.name, "Atomdown: Group Selection");
  assert.equal(fns.ungroupSelection.command.name, "Atomdown: Ungroup");
  assert.deepEqual(fns.onDecorationDrag.events, ["editor:decorationDrag"]);
  assert.deepEqual(fns.onDecorationLasso.events, ["editor:decorationLasso"]);
  assert.deepEqual(fns.onDecorationClick.events, ["editor:decorationClick"]);
  assert.deepEqual(fns.refreshInline.events, ["editor:pageSaved"]);
  Object.keys(fns).forEach((name) => {
    assert.equal(
      typeof plug.functionMapping[name],
      "function",
      `${name} is in the manifest but not in the function mapping`,
    );
  });
});
