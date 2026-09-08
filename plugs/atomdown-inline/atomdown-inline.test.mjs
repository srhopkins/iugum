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
  setAtomAttrsInSource,
  attrFormHtml,
  firstBoxKey,
  widgetUnitKey,
  widgetBoxKey,
  cardMenuItems,
  groupMenuItems,
  menuPopoverHtml,
  menuActionFromClasses,
  menuOpenForCard,
  menuOpenForGroup,
  cardOpenKey,
  MENU_GLYPH,
  minimalEdit,
  newAtomdownId,
  existingIds,
  sanitizeSlug,
  slugConflict,
  deriveGroupSlug,
  slugOrId,
  dedupeKeys,
  isContiguousUnitSelection,
  applyClickSelection,
  lineStarts,
  gripLine,
  contentFirstLine,
  hasNoContent,
  cardHeaderHtml,
  directivePeekHtml,
  toggleAction,
  toggleCollapsed,
  collapsedKey,
  densityKey,
  normalizeDensity,
  otherDensity,
  densityTitle,
  densityClass,
  buildDecorations,
  emptyDecorations,
  firstUnitKey,
  dragToReorder,
  lassoToUnitKeys,
  menuState,
  inlineOnKey,
  groupHeaderHtml,
  groupIdentity,
  groupIdText,
  groupLabel,
  groupFaultText,
  materializeHint,
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
    w.class.split(" ").includes("atomdown-group-header")
  );
  assert.equal(widget.id, "unit:group:7K3M9X2D");
  assert.equal(widget.side, "before");
  assert.equal(widget.at, offsetOf(PAGE, '<!-- <atom-group id="7K3M9X2D"'));
  assert.match(widget.html, /findings/);
  assert.match(widget.html, /7K3M9X2D/);
  assert.match(widget.html, /atomdown-group-count-n">2</);
  assert.match(widget.html, /atomdown-group-count-word"> cards</);
  assert.match(widget.html, /atomdown-grip/);
  assert.match(widget.html, /atomdown-group-collapse/);
  assert.match(widget.html, /atomdown-group-kind/);
  assert.match(widget.html, /atomdown-group-menu/);
});

// The number and the word are two elements, because compact removes the word
// and keeps the number. So the count reads "1 card" only when both are shown.
test("a group with one card says card, not cards", () => {
  const html = groupHeaderHtml({ groupId: "AAAAAAAA", groupSlug: "x" }, 1);
  assert.match(html, /atomdown-group-count-n">1</);
  assert.match(html, /atomdown-group-count-word"> card</);
  const three = groupHeaderHtml({ groupId: "AAAAAAAA", groupSlug: "x" }, 3);
  assert.match(three, /atomdown-group-count-n">3</);
  assert.match(three, /atomdown-group-count-word"> cards</);
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
    // "Edit attributes" opens the form (iugum-etz); "Show the directive
    // line" is what that row used to do and keeps doing under its own name.
    "attrs",
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

// ---------------------------------------------------------------------------
// THE POPOVER (iugum-caj). The card's own menu, not the host's picker.
// ---------------------------------------------------------------------------

test("the three-dot glyph is U+22EE, the VERTICAL ellipsis", () => {
  // By code point, not by eye: U+22EE and U+22EF differ by one bit and look
  // like the same three dots in a diff.
  assert.equal(MENU_GLYPH, "&#8942;");
  assert.equal(String.fromCodePoint(8942), "\u22ee");
  // And the horizontal form it replaced is nowhere in the bundle's markup.
  const bundle = readFileSync(
    new URL("./atomdown-inline.plug.js", import.meta.url),
    "utf8",
  );
  assert.equal(bundle.includes("&#8943;"), false);
});

test("a group's menu leads with identity and offers its two actions", () => {
  const items = groupMenuItems("decisions", "KATZ94NM");
  assert.equal(items[0].action, "label");
  assert.match(items[0].name, /decisions/);
  assert.match(items[0].name, /KATZ94NM/);
  assert.deepEqual(items.map((i) => i.action), [
    "label",
    "rename-group",
    "ungroup-group",
  ]);
});

test("the popover's first child is the label, and it is not an action row", () => {
  const html = menuPopoverHtml("card", cardMenuItems("claim", "4P8W2H6K", false, false));
  const firstTag = html.indexOf("atomdown-menu-label");
  const firstItem = html.indexOf("atomdown-menu-item");
  assert.ok(firstTag > 0, "the label is rendered");
  assert.ok(firstTag < firstItem, "and it comes before every action row");
  // The label carries NO `atomdown-menu-item` and NO `atomdown-mi-` class, so
  // a click on it matches no action and the popover stays open.
  const label = html.slice(firstTag - 20, firstItem);
  assert.equal(label.includes("atomdown-mi-"), false);
});

test("every action row names its own action in a class", () => {
  const items = cardMenuItems("claim", "4P8W2H6K", false, true);
  const html = menuPopoverHtml("card", items);
  for (const item of items) {
    if (item.action === "label") continue;
    assert.ok(
      html.includes("atomdown-mi-" + item.action),
      "row " + item.action + " carries atomdown-mi-" + item.action,
    );
  }
});

test("the popover holds no input, because one cannot take focus in a widget", () => {
  // The seam's widgetPressGuard preventDefaults a plain mousedown inside a
  // widget, so a click focuses nothing and the keystrokes reach the DOCUMENT.
  // Measured in plugs/atomdown-e2e/input-probe.test.ts.
  const html = menuPopoverHtml("card", cardMenuItems("claim", "4P8W2H6K", false, false)) +
    menuPopoverHtml("group", groupMenuItems("decisions", "KATZ94NM"));
  for (const tag of ["<input", "<textarea", "<select", "contenteditable"]) {
    assert.equal(html.includes(tag), false, "no " + tag + " in a popover");
  }
});

test("a row's action is read back from the click's class list", () => {
  assert.equal(
    menuActionFromClasses(["atomdown-menu-item", "atomdown-mi-copy-id"]),
    "copy-id",
  );
  assert.equal(menuActionFromClasses(["atomdown-menu-label"]), null);
  assert.equal(menuActionFromClasses(["atomdown-card-menu"]), null);
  assert.equal(menuActionFromClasses([]), null);
  assert.equal(menuActionFromClasses(undefined), null);
});

test("a popover is remembered under the key a click reports", () => {
  // The widget is named `box:atom:XXXX`; a click reports `atom:XXXX`, because
  // widgetBoxKey strips the prefix. Comparing the two forms directly is the
  // bug that stopped every card popover from opening.
  assert.equal(cardOpenKey("box:atom:4P8W2H6K"), "atom:4P8W2H6K");
  assert.equal(cardOpenKey("atom:4P8W2H6K"), "atom:4P8W2H6K");
  assert.equal(
    menuOpenForCard({ kind: "card", boxKey: "atom:4P8W2H6K" }, cardOpenKey("box:atom:4P8W2H6K")),
    true,
  );
  assert.equal(
    menuOpenForCard({ kind: "card", boxKey: "atom:OTHER123" }, cardOpenKey("box:atom:4P8W2H6K")),
    false,
  );
  assert.equal(menuOpenForCard(null, "atom:4P8W2H6K"), false);
  assert.equal(
    menuOpenForGroup({ kind: "group", unitKey: "group:G1" }, "group:G1"),
    true,
  );
  // A card's open menu never opens a group's popover, and the reverse.
  assert.equal(
    menuOpenForGroup({ kind: "card", boxKey: "atom:4P8W2H6K" }, "group:G1"),
    false,
  );
  assert.equal(
    menuOpenForCard({ kind: "group", unitKey: "group:G1" }, "atom:4P8W2H6K"),
    false,
  );
});

test("exactly one popover is drawn, on the card whose menu is open", () => {
  const cards = computeCards(PAGE).cards;
  const target = "box:atom:" + cards[0].atomIds[0];
  const open = { kind: "card", boxKey: cardOpenKey(target) };
  const closed = buildDecorations(PAGE, [], [], "comfortable", null);
  const opened = buildDecorations(PAGE, [], [], "comfortable", open);

  const count = (dec) =>
    dec.widgets.filter((w) => w.html.includes("atomdown-menu-popover")).length;
  assert.equal(count(closed), 0, "no popover with nothing open");
  assert.equal(count(opened), 1, "one popover with one menu open");
  const host = opened.widgets.find((w) =>
    w.html.includes("atomdown-menu-popover")
  );
  assert.equal(host.id, target, "and it is on the card whose menu was opened");
  assert.ok(
    host.class.includes("atomdown-menu-open"),
    "the host widget is marked open, so its button can stay visible",
  );
});

test("opening a menu changes widgets only - no mark, fold or gesture moves", () => {
  const cards = computeCards(PAGE).cards;
  const open = { kind: "card", boxKey: cardOpenKey("box:atom:" + cards[0].atomIds[0]) };
  const closed = buildDecorations(PAGE, [], [], "comfortable", null);
  const opened = buildDecorations(PAGE, [], [], "comfortable", open);
  // THE WHOLE POINT: a popover is presentation. If a mark or a fold moved,
  // opening a menu could change the document's own layout or fold state.
  assert.deepEqual(opened.marks, closed.marks);
  assert.deepEqual(opened.folds, closed.folds);
  assert.deepEqual(opened.gestures, closed.gestures);
  assert.deepEqual(opened.lines, closed.lines);
  assert.equal(opened.widgets.length, closed.widgets.length);
  // ...and the widgets are at the same offsets, so nothing moves on screen.
  assert.deepEqual(
    opened.widgets.map((w) => [w.id, w.at, w.side]),
    closed.widgets.map((w) => [w.id, w.at, w.side]),
  );
});

test("a group's popover is drawn on the group bar, not on a card", () => {
  const unit = computeUnits(PAGE).units.find((u) => u.kind === "group");
  const opened = buildDecorations(PAGE, [], [], "comfortable", {
    kind: "group",
    unitKey: unit.unitKey,
  });
  const hosts = opened.widgets.filter((w) =>
    w.html.includes("atomdown-menu-popover")
  );
  assert.equal(hosts.length, 1);
  assert.equal(hosts[0].id, "unit:" + unit.unitKey);
  assert.ok(hosts[0].html.includes("atomdown-menu-group"));
});

test("the grip and the menu are keyboard reachable", () => {
  // Both carry tabindex, or their :focus rules in the stylesheet are dead and
  // a tab user lands on a control at opacity 0.
  const html = cardHeaderHtml(
    { atomIds: ["4P8W2H6K"], atomSlug: "claim", implicit: false },
    false,
    "",
    "",
  );
  const grip = html.slice(html.indexOf("atomdown-grip"));
  const menu = html.slice(html.indexOf("atomdown-card-menu"));
  assert.ok(grip.includes('tabindex="0"'), "the grip is focusable");
  assert.ok(menu.includes('tabindex="0"'), "the menu button is focusable");
  assert.ok(menu.includes('aria-haspopup="true"'), "and it announces a popup");
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

// ---------------------------------------------------------------------------
// THE ATTRIBUTE FORM (iugum-etz).
//
// The write half is pure, so every refusal is testable here rather than only
// through a browser. The FOCUS half cannot be tested here - it is a property
// of a real DOM - and lives in rule 8j of the front-end suite.
// ---------------------------------------------------------------------------

const ATTR_PAGE = '<!-- <atom id="AAAAAAAA" slug="old" acme-x="1" ' +
  'digest="sha256:abc"/> -->\nhi\n';

test("saving attributes rewrites one line, id first then slug", () => {
  const result = setAtomAttrsInSource(ATTR_PAGE, "AAAAAAAA", [
    { name: "slug", value: "New Name!" },
    { name: "acme-x", value: "2" },
    { name: "digest", value: "sha256:abc" },
  ]);
  assert.equal(result.ok, true);
  assert.match(
    result.text,
    /<atom id="AAAAAAAA" slug="new-name" acme-x="2" digest="sha256:abc" \/>/,
  );
  const before = ATTR_PAGE.split("\n");
  const changed = result.text.split("\n").filter((l, i) => l !== before[i]);
  assert.equal(changed.length, 1, "one line changed");
  assert.ok(result.text.includes("hi"), "the block's own text is untouched");
});

test("editing an unrelated attribute changes neither the id nor the digest", () => {
  const result = setAtomAttrsInSource(ATTR_PAGE, "AAAAAAAA", [
    { name: "slug", value: "old" },
    { name: "acme-x", value: "9" },
    { name: "digest", value: "sha256:abc" },
  ]);
  assert.equal(result.ok, true);
  assert.match(result.text, /id="AAAAAAAA"/);
  assert.match(result.text, /digest="sha256:abc"/);
});

test("the form cannot edit the id: an id row in the payload is ignored", () => {
  const result = setAtomAttrsInSource(ATTR_PAGE, "AAAAAAAA", [
    { name: "id", value: "ZZZZZZZZ" },
    { name: "slug", value: "old" },
  ]);
  assert.equal(result.ok, true);
  assert.match(result.text, /<atom id="AAAAAAAA" slug="old" \/>/);
  assert.equal(result.text.includes("ZZZZZZZZ"), false);
});

test("adding and removing an attribute both work", () => {
  const added = setAtomAttrsInSource(ATTR_PAGE, "AAAAAAAA", [
    { name: "slug", value: "old" },
    { name: "acme-x", value: "1" },
    { name: "digest", value: "sha256:abc" },
    { name: "owner", value: "steve" },
  ]);
  assert.match(added.text, /owner="steve"/);
  const removed = setAtomAttrsInSource(ATTR_PAGE, "AAAAAAAA", [
    { name: "slug", value: "old" },
    { name: "digest", value: "sha256:abc" },
  ]);
  assert.equal(removed.text.includes("acme-x"), false);
  assert.match(removed.text, /id="AAAAAAAA" slug="old" digest="sha256:abc"/);
});

test("an emptied attribute set still keeps the atom's id", () => {
  // The inline counterpart of the board's "an empty block is rejected": the
  // directive can never be emptied of its identity.
  const result = setAtomAttrsInSource(ATTR_PAGE, "AAAAAAAA", []);
  assert.equal(result.ok, true);
  assert.match(result.text, /<atom id="AAAAAAAA" \/>/);
});

test("a pasted directive in a value is REFUSED, not escaped", () => {
  for (
    const bad of [
      '<!-- <atom id="BBBBBBBB"/> -->',
      "a > b",
      'say "hi"',
      "en--dash",
    ]
  ) {
    const result = setAtomAttrsInSource(ATTR_PAGE, "AAAAAAAA", [
      { name: "acme-x", value: bad },
    ]);
    assert.equal(result.ok, false, JSON.stringify(bad) + " must be refused");
    assert.match(result.error, /attribute value|line break/);
  }
});

test("a line break in a value is refused: an attribute is one line", () => {
  const result = setAtomAttrsInSource(ATTR_PAGE, "AAAAAAAA", [
    { name: "acme-x", value: "one\ntwo" },
  ]);
  assert.equal(result.ok, false);
});

test("an invalid attribute name and a duplicate name are both refused", () => {
  assert.equal(
    setAtomAttrsInSource(ATTR_PAGE, "AAAAAAAA", [{ name: "9bad", value: "" }])
      .ok,
    false,
  );
  assert.equal(
    setAtomAttrsInSource(ATTR_PAGE, "AAAAAAAA", [
      { name: "x", value: "1" },
      { name: "x", value: "2" },
    ]).ok,
    false,
  );
});

test("a blank row is dropped rather than failing the save", () => {
  const result = setAtomAttrsInSource(ATTR_PAGE, "AAAAAAAA", [
    { name: "slug", value: "old" },
    { name: "  ", value: "" },
  ]);
  assert.equal(result.ok, true);
  assert.match(result.text, /<atom id="AAAAAAAA" slug="old" \/>/);
});

test("an empty slug removes the slug attribute", () => {
  const result = setAtomAttrsInSource(ATTR_PAGE, "AAAAAAAA", [
    { name: "slug", value: "   " },
    { name: "acme-x", value: "1" },
  ]);
  assert.match(result.text, /<atom id="AAAAAAAA" acme-x="1" \/>/);
});

test("saving attributes for an atom that is gone fails instead of guessing", () => {
  const result = setAtomAttrsInSource(ATTR_PAGE, "NOSUCHID", []);
  assert.equal(result.ok, false);
  assert.equal(setAtomAttrsInSource(ATTR_PAGE, "AAAAAAAA", "nope").ok, false);
});

test("the form presents the slug FIRST and labelled, and the id as inert", () => {
  const form = attrFormHtml({
    id: "AAAAAAAA",
    slug: "old",
    attrs: [
      { name: "id", value: "AAAAAAAA" },
      { name: "slug", value: "old" },
      { name: "acme-x", value: "1" },
    ],
  });
  const slugAt = form.html.indexOf("ad-attr-slug-input");
  const listAt = form.html.indexOf("ad-attr-list");
  assert.ok(slugAt > 0 && slugAt < listAt, "the slug row comes before the list");
  assert.match(form.html, /Name \(slug\) - readable alias, not the id/);
  assert.match(form.html, /value="old"/);
  // The id and the slug are not ALSO generic rows: the id is added by the
  // script as a disabled row and the slug has its own labelled field.
  assert.match(form.script, /addRow\("id", DATA\.id, true\)/);
  assert.equal(/"acme-x"/.test(form.script), true);
  assert.equal(form.script.includes('{"name":"slug"'), false);
  // It focuses its own field, which is the whole reason it is not in the
  // popover: in a widget a click focuses nothing and the typing goes into the
  // document (rule 8g, input-probe.test.ts).
  assert.match(form.script, /slugEl\.focus\(\)/);
});

test("the form's markup escapes the values it renders", () => {
  const form = attrFormHtml({ id: 'A"B', slug: "<script>", attrs: [] });
  assert.equal(form.html.includes("<script>"), false);
  assert.match(form.html, /&lt;script&gt;/);
  assert.equal(form.html.includes('value="A"B"'), false);
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
    w.class.split(" ").includes("atomdown-group-header")
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

test("no header widget carries directive text as text, only as an attribute", () => {
  // The defect: a card header's own text read
  // `⠿running-todoZE5AMAB7⋯<!-- <atom id="ZE5AMAB7" digest="sha256:…`, so the
  // directive was back in the page's text for everything that reads text
  // rather than pixels. The peek paints it with `content: attr(...)` instead,
  // so the characters are in an attribute value and in no text node.
  const payload = buildDecorations(PAGE, []);
  for (const widget of payload.widgets) {
    // Strip every tag, which leaves exactly the widget's text nodes.
    const text = widget.html.replaceAll(/<[^>]*>/g, "");
    assert.equal(
      /<!--|atom id=|atom-group id=|sha256:/.test(text),
      false,
      `widget ${widget.id} carries directive text in a text node: ${text}`,
    );
  }
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
  // The flags own the state: the seam reconciles the editor's fold set to them
  // on every update, so a fold made from the gutter is put back and one press
  // always flips exactly one group. Nothing reads the editor here, which is
  // what makes a press deterministic instead of a race against the seam.
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

test("a widget control names its own unit, whatever the click's marks say", () => {
  // THE DEFECT. A header widget is a block element with no text, so the seam
  // builds its click's mark list from the nearest text position — which is
  // outside the fold once a neighbouring group is collapsed. The caret then
  // read the wrong group's `unit:` mark and toggled the wrong group.
  const wrong = ["unit:group:NEIGHBOUR", "box:atom:AAAAAAAA"];
  assert.equal(
    widgetUnitKey({ widget: "unit:group:7K3M9X2D", marks: wrong }),
    "group:7K3M9X2D",
  );
  assert.equal(
    widgetBoxKey({ widget: "box:atom:AAAAAAAA", marks: wrong }),
    "atom:AAAAAAAA",
  );
  // A click that was not in a widget says so, and the mark list is then the
  // only answer available.
  assert.equal(widgetUnitKey({ marks: wrong }), null);
  assert.equal(widgetUnitKey({ widget: "" }), null);
  assert.equal(widgetBoxKey({ widget: "unit:group:X" }), null);
  assert.equal(widgetUnitKey({ widget: "box:atom:X" }), null);
  assert.equal(widgetUnitKey(null), null);
});

test("every widget the plug emits is named after the unit or box it draws", () => {
  // The identity above only works if the widget carries it, so this is the
  // other half: no header widget may be anonymous.
  const payload = buildDecorations(PAGE, [], []);
  for (const widget of payload.widgets) {
    assert.ok(
      /^(unit|box):/.test(widget.id),
      `widget ${widget.class} has no unit or box name: ${widget.id}`,
    );
  }
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

/** The units of PAGE, the second argument every drag request is read against. */
const PAGE_UNITS = computeUnits(PAGE).units;

test("a drag reports the unit under the handle and the unit under the drop", () => {
  const request = dragToReorder({
    marks: ["unit:atom:4P8W2H6K"],
    targetMarks: ["unit:group:7K3M9X2D", "card:7K3M9X2D:1"],
    placement: "after",
  }, PAGE_UNITS);
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
  }, PAGE_UNITS);
  assert.equal(request.movedUnitKey, "group:7K3M9X2D");
});

test("a drop on the block it came from asks for nothing", () => {
  assert.equal(
    dragToReorder({
      marks: ["unit:atom:4P8W2H6K"],
      targetMarks: ["unit:atom:4P8W2H6K"],
      placement: "after",
    }, PAGE_UNITS),
    null,
  );
});

test("a drag with no unit under the handle asks for nothing", () => {
  assert.equal(
    dragToReorder({
      marks: [],
      targetMarks: ["unit:atom:4P8W2H6K"],
    }, PAGE_UNITS),
    null,
  );
});

// --- A RELEASE THAT COVERS NO UNIT (iugum-uuv) -----------------------------
//
// The grip lives in the page gutter, so the natural gesture — press the grip
// and travel straight down — releases over the blank line between two cards
// as often as over a card. A blank line is a unit BOUNDARY and carries no
// unit mark, so `targetMarks` comes back empty. That used to mean "the end of
// the document", and Steve dragged the first card down one position and
// watched it land at the bottom of an 84-card page.
//
// The answer is the board panel's `pickDropTarget` rule, stated in lines
// instead of pixels: the drop goes BEFORE the first unit that begins at or
// after the release line. Card order IS document order in this view, so the
// next unit in source order and the next unit down the page are the same unit,
// and no rectangle has to be measured to find it.

test("a release on the blank line between two units drops before the next one", () => {
  // PAGE line 5 (1-based) is the blank line between the claim and the group.
  assert.deepEqual(
    dragToReorder({
      marks: ["unit:atom:implicit-1"],
      targetMarks: [],
      targetLine: 5,
      placement: "after",
    }, PAGE_UNITS),
    {
      movedUnitKey: "atom:implicit-1",
      targetUnitKey: "group:7K3M9X2D",
      placement: "before",
    },
  );
});

test("a release on the blank line after a group drops before the next unit", () => {
  // Line 13: between the group's closing marker and the implicit paragraph.
  assert.deepEqual(
    dragToReorder({
      marks: ["unit:atom:4P8W2H6K"],
      targetMarks: [],
      targetLine: 13,
      placement: "before",
    }, PAGE_UNITS),
    {
      movedUnitKey: "atom:4P8W2H6K",
      targetUnitKey: "atom:implicit-1",
      placement: "before",
    },
  );
});

test("a release above every unit drops before the first one", () => {
  // Line 2: the blank line under the document marker. The claim is first, so
  // this is the start of the page — and it must not reach the marker itself.
  assert.deepEqual(
    dragToReorder({
      marks: ["unit:group:7K3M9X2D"],
      targetMarks: [],
      targetLine: 2,
      placement: "before",
    }, PAGE_UNITS),
    {
      movedUnitKey: "group:7K3M9X2D",
      targetUnitKey: "atom:4P8W2H6K",
      placement: "before",
    },
  );
});

test("a release past every unit still lands at the end of the page", () => {
  // Line 16: the trailing blank line, below the last unit.
  assert.deepEqual(
    dragToReorder({
      marks: ["unit:atom:4P8W2H6K"],
      targetMarks: [],
      targetLine: 16,
      placement: "after",
    }, PAGE_UNITS),
    {
      movedUnitKey: "atom:4P8W2H6K",
      targetUnitKey: null,
      placement: "end",
    },
  );
});

test("a release in the gap right above the dragged unit asks for nothing", () => {
  // Line 13 resolves to the implicit paragraph, which is the unit being
  // dragged: the reader dropped it where it already is.
  assert.equal(
    dragToReorder({
      marks: ["unit:atom:implicit-1"],
      targetMarks: [],
      targetLine: 13,
      placement: "before",
    }, PAGE_UNITS),
    null,
  );
});

test("a release the seam could not place at all lands at the end", () => {
  // No `targetLine`: nothing to resolve against, so the old fallback stands.
  assert.deepEqual(
    dragToReorder({
      marks: ["unit:atom:4P8W2H6K"],
      targetMarks: [],
      placement: "before",
    }, PAGE_UNITS),
    {
      movedUnitKey: "atom:4P8W2H6K",
      targetUnitKey: null,
      placement: "end",
    },
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
// WHAT A CLICK DOES TO THE SELECTION (`applyClickSelection`), the whole rule
// as a pure function.
//
// `iugum-oip`: on the live page a plain click on a card selected nothing and
// an alt-drag lost its selection a millisecond later. The decision used to be
// three inline `if`s inside the click handler, where the only branch for a
// plain click on a card CLEARED — so a click could unselect and never select.
// It is a function with these tests now, because the branch that was wrong is
// the branch no test could reach without a browser and a pointer.
// ---------------------------------------------------------------------------

const ORDER = ["atom:A", "atom:B", "group:G", "atom:D"];

test("a plain click selects that one unit and anchors there", () => {
  const out = applyClickSelection(ORDER, [], null, "atom:B", {});
  assert.deepEqual(out.selected, ["atom:B"]);
  assert.equal(out.anchor, "atom:B");
});

test("a plain click REPLACES a selection rather than clearing it", () => {
  // The defect, stated as a test: this used to return an empty selection.
  const out = applyClickSelection(ORDER, ["atom:A"], "atom:A", "atom:B", {});
  assert.deepEqual(out.selected, ["atom:B"]);
});

test("a modifier click adds, and the same click again removes", () => {
  const added = applyClickSelection(
    ORDER,
    ["atom:A"],
    "atom:A",
    "atom:B",
    { metaKey: true },
  );
  assert.deepEqual(added.selected, ["atom:A", "atom:B"]);
  const removed = applyClickSelection(
    ORDER,
    added.selected,
    added.anchor,
    "atom:B",
    { ctrlKey: true },
  );
  assert.deepEqual(removed.selected, ["atom:A"]);
});

test("shift extends from the anchor and takes the units between", () => {
  const out = applyClickSelection(
    ORDER,
    ["atom:A"],
    "atom:A",
    "atom:D",
    { shiftKey: true },
  );
  // group:G is in the range and nobody clicked it. That is the property.
  assert.deepEqual(out.selected, ["atom:A", "atom:B", "group:G", "atom:D"]);
  assert.equal(out.anchor, "atom:A", "the anchor does not move");
});

test("shift extends backwards too", () => {
  const out = applyClickSelection(
    ORDER,
    ["group:G"],
    "group:G",
    "atom:A",
    { shiftKey: true },
  );
  assert.deepEqual(out.selected, ["atom:A", "atom:B", "group:G"]);
});

test("a second shift-click re-extends from the same anchor", () => {
  const first = applyClickSelection(
    ORDER,
    ["atom:A"],
    "atom:A",
    "atom:D",
    { shiftKey: true },
  );
  const second = applyClickSelection(
    ORDER,
    first.selected,
    first.anchor,
    "atom:B",
    { shiftKey: true },
  );
  assert.deepEqual(second.selected, ["atom:A", "atom:B"]);
});

test("shift with no anchor yet is a plain click", () => {
  const out = applyClickSelection(ORDER, [], null, "atom:B", {
    shiftKey: true,
  });
  assert.deepEqual(out.selected, ["atom:B"]);
  assert.equal(out.anchor, "atom:B");
});

test("no unit under the pointer clears, and never selects", () => {
  const out = applyClickSelection(ORDER, ["atom:A", "atom:B"], "atom:A", null, {
    metaKey: true,
  });
  assert.deepEqual(out.selected, []);
  assert.equal(out.anchor, null);
});

test("an anchor the order no longer knows is a plain click", () => {
  // After an ungroup the anchor can name a unit that is gone. A stale key
  // cannot anchor a range, so the click means what an unmodified one means.
  const out = applyClickSelection(ORDER, [], "group:GONE", "atom:B", {
    shiftKey: true,
  });
  assert.deepEqual(out.selected, ["atom:B"]);
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

// ---------------------------------------------------------------------------
// Display density
//
// Two densities, the panel's own two, and the switch is PRESENTATIONAL: the
// only difference between the two payloads is a set of CSS class names. Every
// offset, every id, every widget and every fold is identical, which is what
// makes "no document byte changes with the density" provable here rather than
// only in a browser.
// ---------------------------------------------------------------------------

test("an unknown density reads as comfortable, the default", () => {
  assert.equal(normalizeDensity(undefined), "comfortable");
  assert.equal(normalizeDensity(null), "comfortable");
  assert.equal(normalizeDensity("cosy"), "comfortable");
  assert.equal(normalizeDensity("comfortable"), "comfortable");
  assert.equal(normalizeDensity("compact"), "compact");
});

test("the switch alternates, and its tooltip names where it would take you", () => {
  assert.equal(otherDensity("comfortable"), "compact");
  assert.equal(otherDensity("compact"), "comfortable");
  assert.equal(otherDensity(otherDensity("compact")), "compact");
  assert.match(densityTitle("comfortable"), /^Compact:/);
  assert.match(densityTitle("compact"), /^Comfortable:/);
});

test("the density class is one class, and there is one per density", () => {
  assert.equal(densityClass("comfortable"), "atomdown-comfortable");
  assert.equal(densityClass("compact"), "atomdown-compact");
  assert.equal(densityClass(undefined), "atomdown-comfortable");
  assert.equal(densityClass("compact").split(" ").length, 1);
});

test("the density is remembered per page, in the same store as the rest", () => {
  assert.equal(
    densityKey("Todo/running"),
    "atomdown-inline.density:Todo/running",
  );
  assert.notEqual(densityKey("a"), densityKey("b"));
  // Same prefix as the view's on/off flag and its collapsed set, so there is
  // one storage mechanism rather than a second one for the density.
  assert.match(densityKey("p"), /^atomdown-inline\./);
});

test("every decorated line carries the density class, at both densities", () => {
  ["comfortable", "compact"].forEach((density) => {
    const payload = buildDecorations(PAGE, [], [], density);
    const dens = payload.marks.filter((m) => m.id.startsWith("dens:"));
    assert.ok(dens.length > 0, `${density} emitted no density mark`);
    dens.forEach((mark) => {
      assert.equal(mark.class, densityClass(density));
      assert.equal(mark.lineClasses, true);
    });
  });
});

test("exactly one density mark per unit, spanning the whole unit", () => {
  // NO OVERLAP. Two density marks over one line make the seam derive
  // `-line` twice and `-first`, `-mid` and `-last` together, and how many of
  // the overlapping marks the editor has realised varies with the scroll
  // position — so the same correct state produced two different class
  // strings. A unit's span already covers every card inside it.
  const payload = buildDecorations(PAGE, [], [], "compact");
  const dens = payload.marks.filter((m) => m.id.startsWith("dens:"));
  const units = payload.marks.filter((m) => m.id.startsWith("unit:"));
  assert.equal(dens.length, units.length);
  units.forEach((unit) => {
    const twin = dens.find((m) =>
      m.id === "dens:" + unit.id.slice("unit:".length)
    );
    assert.ok(twin, `no density mark for ${unit.id}`);
    assert.equal(twin.from, unit.from);
    assert.equal(twin.to, unit.to);
  });
  // And no two of them overlap.
  const sorted = dens.slice().sort((a, b) => a.from - b.from);
  for (let i = 1; i < sorted.length; i++) {
    assert.ok(
      sorted[i].from > sorted[i - 1].to,
      `${sorted[i - 1].id} and ${sorted[i].id} overlap`,
    );
  }
});

test("every card line is inside a density mark, so the knobs reach it", () => {
  const payload = buildDecorations(PAGE, [], [], "compact");
  const dens = payload.marks.filter((m) => m.id.startsWith("dens:"));
  payload.marks
    .filter((m) => m.id.startsWith("box:"))
    .forEach((box) => {
      const covering = dens.find((m) => m.from <= box.from && m.to >= box.to);
      assert.ok(covering, `${box.id} is not covered by a density mark`);
    });
});

test("every widget carries the density class, at both densities", () => {
  ["comfortable", "compact"].forEach((density) => {
    const payload = buildDecorations(PAGE, [], [], density);
    assert.ok(payload.widgets.length > 0);
    payload.widgets.forEach((widget) => {
      assert.ok(
        widget.class.split(" ").includes(densityClass(density)),
        `${widget.id} at ${density} has class "${widget.class}"`,
      );
    });
  });
});

test("compact keeps the card header widget, so the grip and the menu stay", () => {
  // The row is lifted out of the layout by CSS rather than dropped from the
  // payload: the grip, the three-dot menu and the directive peek all live in
  // that widget, and a density that removed it would have to rebuild all
  // three somewhere else.
  const payload = buildDecorations(PAGE, [], [], "compact");
  const head = payload.widgets.find((w) => w.id === "box:atom:4P8W2H6K");
  assert.ok(head);
  assert.match(head.html, /atomdown-grip/);
  assert.match(head.html, /atomdown-card-menu/);
  assert.match(head.html, /atomdown-directive-peek/);
});

test("the group's collapse control is identical at both densities", () => {
  // It is the control that turns a long page into a list of group names, so
  // it must not thin out with the rest of the bar.
  const bars = ["comfortable", "compact"].map((density) => {
    const payload = buildDecorations(PAGE, [], [], density);
    return payload.widgets.find((w) =>
      w.class.split(" ").includes("atomdown-group-header")
    ).html;
  });
  assert.match(bars[0], /atomdown-group-collapse/);
  assert.equal(bars[0], bars[1]);
});

test("switching density changes CSS classes and nothing else", () => {
  // THE PROOF THAT THE SWITCH IS PRESENTATIONAL. Strip the density class out
  // of both payloads and they must be byte-identical: same offsets, same ids,
  // same widget HTML, same folds. There is no source text in the payload at
  // all, so nothing here can reach the document.
  const strip = (payload, density) =>
    JSON.stringify(payload)
      .replaceAll('"class":"' + densityClass(density) + '"', '"class":"D"')
      .replaceAll(" " + densityClass(density), "")
      .replaceAll('"dens:', '"DENS:');
  const comfortable = strip(
    buildDecorations(PAGE, ["atom:4P8W2H6K"], ["7K3M9X2D"], "comfortable"),
    "comfortable",
  );
  const compact = strip(
    buildDecorations(PAGE, ["atom:4P8W2H6K"], ["7K3M9X2D"], "compact"),
    "compact",
  );
  assert.equal(comfortable, compact);
});

test("the remembered flag is keyed by page name", () => {
  assert.equal(inlineOnKey("Todo/running"), "atomdown-inline.on:Todo/running");
  assert.notEqual(inlineOnKey("a"), inlineOnKey("b"));
});

// ---------------------------------------------------------------------------
// The manifest
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// The header buttons' contract
//
// The buttons are registered BY THE PLUG now (`registerActionButtons`), not by
// an actionButton.define block on the library page. A button still names its
// command as a STRING that the client resolves with runCommandByName, and
// nothing else in the build checks the two strings match: a rename on either
// side leaves a button that silently does nothing, which is exactly the
// failure that was reported.
// ---------------------------------------------------------------------------

const LIBRARY_PAGE = readFileSync(
  new URL("./library/Atomdown Inline.md", import.meta.url),
  "utf8",
);

test("every header button names a command this plug actually registers", () => {
  const buttons = plug.internals.ACTION_BUTTONS;
  assert.equal(buttons.length, 2, "the card view and the density switch");
  const commands = Object.values(plug.manifest.functions)
    .filter((fn) => fn.command)
    .map((fn) => fn.command.name);
  buttons.forEach((button) => {
    assert.ok(
      commands.includes(button.command),
      `the button names "${button.command}", not one of ${commands.join(", ")}`,
    );
    // And that command must resolve to a real function, or runCommandByName
    // finds an entry whose run() throws.
    const entry = Object.entries(plug.manifest.functions)
      .find(([, fn]) => fn.command && fn.command.name === button.command);
    assert.equal(typeof plug.functionMapping[entry[0]], "function");
  });
});

test("every button carries an icon, or the client filters it out entirely", () => {
  // client/editor_ui.tsx drops any actionButton without an icon.
  plug.internals.ACTION_BUTTONS.forEach((button) => {
    assert.equal(typeof button.icon, "string");
    assert.ok(button.icon.length > 0, `${button.command} has no icon`);
  });
});

test("the buttons fit the actionButtons schema, which forbids extra keys", () => {
  // libraries/Library/Std/Config.md defines the schema with
  // additionalProperties = false, so an unknown key makes config.insert throw
  // and the button never appears.
  const allowed = [
    "icon",
    "description",
    "command",
    "priority",
    "mobile",
    "standalone",
    "dropdown",
    "run",
  ];
  plug.internals.ACTION_BUTTONS.forEach((button) => {
    Object.keys(button).forEach((key) => {
      assert.ok(allowed.includes(key), `${key} is not in the schema`);
    });
  });
});

test("the heartbeat looks every fifth tick, not every tick", () => {
  const { shouldReassertButtons, BUTTON_CHECK_TICKS } = plug.internals;
  assert.equal(BUTTON_CHECK_TICKS, 5);
  const looked = [];
  for (let tick = 1; tick <= 12; tick++) {
    if (shouldReassertButtons(tick)) looked.push(tick);
  }
  assert.deepEqual(looked, [5, 10]);
});

test("the heartbeat is wired to the client's own cron tick", () => {
  // The self-healing half of the button registration. Without it a config
  // clear after `editor:init` — which is what the first index of a space
  // causes — leaves the commands registered and the header bar empty.
  assert.deepEqual(
    plug.manifest.functions.heartbeatActionButtons.events,
    ["cron:secondPassed"],
  );
  assert.deepEqual(
    plug.manifest.functions.registerActionButtons.events,
    ["editor:init"],
  );
});

test("the library page registers no action button of its own", () => {
  // Two buttons would appear, and one of them would be the stale copy that
  // arrives only when the client's page index happens to hold this page. The
  // page's PROSE still names actionButton.define, to say why it is gone, so
  // this looks inside the space-lua fences and nowhere else.
  const luaBlocks = [
    ...LIBRARY_PAGE.matchAll(/```space-lua\n([\s\S]*?)```/g),
  ].map((m) => m[1]);
  luaBlocks.forEach((block) => {
    assert.equal(/actionButton\.define/.test(block), false);
  });
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


// --- iugum-39j: an invalid group marker ------------------------------------
//
// Measured on a real page: seven atom-group markers with no `id` attribute.
// The group's identity was its id, so all seven resolved to the key
// `group:null`, every card was attributed to every group, and the view emitted
// the card chrome once per atom-group pair - seven copies of every card. The
// bar printed the literal word `null` where the id belongs and a count of 44
// against 50 real atoms. Only the chrome duplicated, which is the tell that
// the fault is in group membership rather than in rendering.

const TWO_IDLESS_GROUPS = [
  '<!-- <atomdown version="1"/> -->',
  "",
  '<!-- <atom-group slug="spf"> -->',
  '<!-- <atom id="AAAAAAAA"/> -->',
  "- first",
  "",
  '<!-- <atom id="BBBBBBBB"/> -->',
  "- second",
  "<!-- </atom-group> -->",
  "",
  '<!-- <atom-group slug="dkim"> -->',
  '<!-- <atom id="CCCCCCCC"/> -->',
  "- third",
  "",
  '<!-- <atom id="DDDDDDDD"/> -->',
  "- fourth",
  "<!-- </atom-group> -->",
  "",
].join("\n");

const DUPLICATE_GROUP_IDS = [
  '<!-- <atomdown version="1"/> -->',
  "",
  '<!-- <atom-group id="7K3M9X2D" slug="spf"> -->',
  '<!-- <atom id="AAAAAAAA"/> -->',
  "- first",
  "<!-- </atom-group> -->",
  "",
  '<!-- <atom-group id="7K3M9X2D" slug="dkim"> -->',
  '<!-- <atom id="BBBBBBBB"/> -->',
  "- second",
  "<!-- </atom-group> -->",
  "",
].join("\n");

test("groupIdentity: a missing id is a fault and a line-anchored key", () => {
  assert.deepEqual(groupIdentity(null, 4, []), {
    groupKey: "@4",
    fault: "no-id",
  });
  assert.deepEqual(groupIdentity("", 7, []), {
    groupKey: "@7",
    fault: "no-id",
  });
  assert.deepEqual(groupIdentity("7K3M9X2D", 2, []), {
    groupKey: "7K3M9X2D",
    fault: null,
  });
  assert.deepEqual(groupIdentity("7K3M9X2D", 9, ["7K3M9X2D"]), {
    groupKey: "@9",
    fault: "duplicate-id",
  });
});

test("two id-less groups are two units with two distinct keys", () => {
  const { units } = computeUnits(TWO_IDLESS_GROUPS);
  const groups = units.filter((u) => u.kind === "group");
  assert.equal(groups.length, 2);
  assert.notEqual(groups[0].unitKey, groups[1].unitKey);
  assert.deepEqual(groups.map((g) => g.groupFault), ["no-id", "no-id"]);
});

test("every card belongs to exactly one group", () => {
  const { cards } = computeCards(TWO_IDLESS_GROUPS);
  const ids = cards.map((c) => c.cardKey);
  assert.deepEqual(ids, [
    "atom:AAAAAAAA",
    "atom:BBBBBBBB",
    "atom:CCCCCCCC",
    "atom:DDDDDDDD",
  ]);
  const perGroup = {};
  cards.forEach((c) => {
    perGroup[c.groupUnitKey] = (perGroup[c.groupUnitKey] || 0) + 1;
  });
  assert.deepEqual(Object.values(perGroup), [2, 2]);
});

test("a card's chrome is emitted once, not once per group", () => {
  const payload = buildDecorations(TWO_IDLESS_GROUPS, []);
  const cardBoxes = payload.marks.filter((m) => m.class === "atomdown-card");
  assert.equal(cardBoxes.length, 4);
  const headerNames = payload.widgets
    .filter((w) => w.class.includes("atomdown-card-header"))
    .map((w) => w.id);
  assert.equal(headerNames.length, 4);
  assert.equal(new Set(headerNames).size, 4);
  // And every mark name is unique, which is what a colliding key destroyed.
  const names = payload.marks.map((m) => m.id);
  assert.equal(new Set(names).size, names.length);
});

test("two groups sharing one id stay two groups", () => {
  const { units, cards } = computeCards(DUPLICATE_GROUP_IDS);
  const groups = units.filter((u) => u.kind === "group");
  assert.equal(groups.length, 2);
  assert.notEqual(groups[0].unitKey, groups[1].unitKey);
  assert.deepEqual(groups.map((g) => g.groupFault), [null, "duplicate-id"]);
  assert.equal(cards.length, 2);
  const payload = buildDecorations(DUPLICATE_GROUP_IDS, []);
  assert.equal(
    payload.marks.filter((m) => m.class === "atomdown-card").length,
    2,
  );
});

test("the group bar counts the cards it actually shows", () => {
  const payload = buildDecorations(TWO_IDLESS_GROUPS, []);
  const bars = payload.widgets.filter((w) =>
    w.class.includes("atomdown-group-header")
  );
  assert.equal(bars.length, 2);
  for (const bar of bars) {
    assert.ok(bar.html.includes('class="atomdown-group-count-n">2<'));
  }
});

test("the group bar never prints the word null", () => {
  for (const density of ["comfortable", "compact"]) {
    const payload = buildDecorations(TWO_IDLESS_GROUPS, [], [], density);
    const bars = payload.widgets.filter((w) =>
      w.class.includes("atomdown-group-header")
    );
    assert.equal(bars.length, 2);
    for (const bar of bars) {
      assert.ok(!bar.html.includes("null"));
      assert.ok(bar.html.includes('class="atomdown-group-id"'));
      assert.ok(bar.html.includes(">no id<"));
    }
  }
});

test("the bar names the fault and the command that repairs it", () => {
  const payload = buildDecorations(
    TWO_IDLESS_GROUPS,
    [],
    [],
    "comfortable",
    null,
    "Reference/email-sending-domain-status",
  );
  const bars = payload.widgets.filter((w) =>
    w.class.includes("atomdown-group-header")
  );
  for (const bar of bars) {
    assert.ok(bar.html.includes('class="atomdown-group-fault"'));
    assert.ok(bar.html.includes("INVALID GROUP"));
    assert.ok(bar.html.includes("has no id"));
    assert.ok(
      bar.html.includes(
        "atomdown materialize -w Reference/email-sending-domain-status",
      ),
    );
  }
});

test("a valid document gains no fault chrome at all", () => {
  for (const density of ["comfortable", "compact"]) {
    const payload = buildDecorations(PAGE, [], [], density);
    for (const widget of payload.widgets) {
      assert.ok(!widget.html.includes("atomdown-group-fault"));
      assert.ok(!widget.html.includes("INVALID GROUP"));
    }
  }
});

test("groupLabel names a group with neither slug nor id", () => {
  assert.equal(
    groupLabel({ groupId: null, groupSlug: null, startLine: 4 }),
    "the group at line 5",
  );
  assert.equal(groupLabel({ groupId: "7K3M9X2D", groupSlug: null }), "7K3M9X2D");
  assert.equal(groupLabel({ groupId: "7K3M9X2D", groupSlug: "spf" }), "spf");
});

test("groupIdText and materializeHint say missing rather than null", () => {
  assert.equal(groupIdText({ groupId: null, groupFault: "no-id" }), "no id");
  assert.equal(groupIdText({ groupId: "7K3M9X2D", groupFault: null }), "7K3M9X2D");
  assert.equal(groupFaultText({ groupFault: null }, "Page"), "");
  assert.equal(materializeHint(null), "atomdown materialize -w <file>");
  assert.equal(
    materializeHint("Todo/running"),
    "atomdown materialize -w Todo/running",
  );
});

test("removeGroupMarkers cannot be aimed at a faulted group by its slug", () => {
  // The key of a faulted group is line-anchored, so nothing that spells an id
  // reaches it, and the group-level commands refuse it outright.
  assert.equal(removeGroupMarkers(TWO_IDLESS_GROUPS, "spf").ok, false);
  assert.equal(setGroupSlugInSource(TWO_IDLESS_GROUPS, "@2", "x").ok, false);
});
