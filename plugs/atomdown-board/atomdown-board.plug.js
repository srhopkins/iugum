// atomdown-board — hand-authored SilverBullet plug worker bundle.
//
// This file is NOT the output of `plug-compile` / esbuild. It is written by
// hand in the same shape a real compile would produce, following the pattern
// documented in silverbullet-treeview's FORK.md and scripts/patch-plug-js.py
// (see README.md in this directory for why plug-compile is not available
// here). Keep this file plain ES2020 JS with no imports — the worker loads
// it directly, there is no bundler step.
//
// Manifest source of truth for humans: atomdown-board.plug.yaml (kept in
// sync by hand, not read at runtime).
//
// Attribute policy: this plug is a generic viewer/editor for whatever XML
// attributes happen to sit on an <atom .../> directive. It does not know
// the name of any application-level attribute (no "audited", no "lock",
// nothing) and must never be changed to special-case one — see
// iugum-w6y for why. The only name this file treats specially is "id",
// because Atomdown Core itself requires every atom to have one (SPEC.md
// "Identity"); that is a Core structural rule, not a domain attribute.
// Same policy for the drag-to-reorder code below: a "locked" atom (whatever
// that ends up meaning at the application level) still drags normally here.
// Steve was explicit that lock protects an atom's CONTENT, not its
// position — see iugum-w6y's design notes — so reordering never consults
// any attribute value, locked or not.

// ---------------------------------------------------------------------------
// Worker <-> host runtime shim.
//
// Every SilverBullet plug worker needs this same boilerplate to receive its
// manifest request, dispatch invoked functions, and make syscalls back to
// the host. Copied (functionally, not byte-for-byte) from the compiled
// mermaid.plug.js / treeview.plug.js bundles already installed in this
// space — this plug's own logic starts at "PLUG LOGIC" below.
// ---------------------------------------------------------------------------

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
              "atomdown-board: function threw",
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

// ---------------------------------------------------------------------------
// PLUG LOGIC
// ---------------------------------------------------------------------------

// Tracks whether this plug instance believes the modal is currently showing.
// Updated both when this plug's own toggle command hides the panel, and when
// the panel's own close button calls back into notifyClosed() below — so the
// two ways of closing the board can't leave this flag out of sync.
let boardOpen = false;

function escapeHtml(text) {
  return text
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;");
}

// XML attribute value escaping (for writing a directive line back out).
// Only & and " and < need escaping inside a double-quoted XML attribute
// value; this deliberately does not touch anything else about the value,
// since this plug must not interpret or reshape attribute content.
function escapeAttrValue(value) {
  return String(value)
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll('"', "&quot;");
}

// Matches a self-closing <atom .../> directive, alone on a line, wrapped in
// an HTML comment. Captures the raw attribute text between "<atom" and
// "/>" — every attribute on the tag, generically, in source order. This
// plug never looks at what those names are.
const ATOM_TAG_RE = /^(\s*<!--\s*<atom\s+)([^>]*?)(\/>\s*-->\s*)$/;

// Matches the opening marker of an <atom-group id="...">, and its closing
// </atom-group> marker. Atom groups do not get their own card in this
// spike; each atom inside one still gets a card, tagged with the group id.
const GROUP_OPEN_RE = /^\s*<!--\s*<atom-group\s+([^>]*?)>\s*-->\s*$/;
const GROUP_CLOSE_RE = /^\s*<!--\s*<\/atom-group>\s*-->\s*$/;

// The document-level <atomdown version="1"/> marker, if present. Not an
// atom; just skipped.
const DOC_MARKER_RE = /^\s*<!--\s*<atomdown\b[^>]*\/>\s*-->\s*$/;

// Generic XML-attribute-list parser: `name="value"` or `name='value'`
// pairs, in source order. Does not know or care what any name means.
const ATTR_PAIR_RE = /([A-Za-z_][\w:.-]*)\s*=\s*(?:"([^"]*)"|'([^']*)')/g;

function parseAttrs(attrText) {
  const attrs = [];
  let m;
  ATTR_PAIR_RE.lastIndex = 0;
  while ((m = ATTR_PAIR_RE.exec(attrText)) !== null) {
    const value = m[2] !== undefined ? m[2] : m[3];
    attrs.push({ name: m[1], value });
  }
  return attrs;
}

/**
 * Straightforward line-based scan for Atomdown atoms, per the spec at
 * atomdown/SPEC.md. Not a full parser: it does not validate group balance,
 * ID uniqueness, or nesting, and it does not shell out to the atomdown
 * binary (a browser worker cannot run a subprocess). Good enough to render
 * a board for this spike.
 *
 * Blocks are delimited by blank lines, same as CommonMark top-level blocks.
 * An <atom> marker applies only to the next block; any further
 * blank-line-separated blocks before the next marker are implicit atoms
 * (per SPEC.md: "A tool must not discard the block or attach it to the
 * previous atom").
 */
function parseAtoms(sourceText) {
  const lines = sourceText.split("\n");
  const atoms = [];

  let pendingId = null;
  let pendingAttrs = null;
  let currentGroupId = null;
  let bufferLines = [];
  let implicitCounter = 0;

  function flush() {
    const blocks = [];
    let block = [];
    for (const line of bufferLines) {
      if (line.trim() === "") {
        if (block.length > 0) {
          blocks.push(block);
          block = [];
        }
      } else {
        block.push(line);
      }
    }
    if (block.length > 0) blocks.push(block);

    blocks.forEach((blockLines, i) => {
      const isFirst = i === 0;
      let id = isFirst ? pendingId : null;
      let attrs = isFirst ? pendingAttrs : null;
      let implicit = false;
      if (!id) {
        implicitCounter++;
        id = `implicit-${implicitCounter}`;
        implicit = true;
        attrs = [];
      }
      atoms.push({
        id,
        implicit,
        groupId: currentGroupId,
        text: blockLines.join("\n"),
        attrs,
      });
    });

    bufferLines = [];
    pendingId = null;
    pendingAttrs = null;
  }

  for (const line of lines) {
    if (DOC_MARKER_RE.test(line)) {
      flush();
      continue;
    }
    const atomMatch = line.match(ATOM_TAG_RE);
    if (atomMatch) {
      const attrs = parseAttrs(atomMatch[2]);
      const idAttr = attrs.find((a) => a.name === "id");
      if (idAttr) {
        flush();
        pendingId = idAttr.value;
        pendingAttrs = attrs;
        continue;
      }
      // No id attribute: not a valid atom directive by SPEC.md ("Identity").
      // Fall through and treat the line as ordinary content rather than
      // silently dropping it.
    }
    const groupOpenMatch = line.match(GROUP_OPEN_RE);
    if (groupOpenMatch) {
      flush();
      const groupAttrs = parseAttrs(groupOpenMatch[1]);
      const groupIdAttr = groupAttrs.find((a) => a.name === "id");
      currentGroupId = groupIdAttr ? groupIdAttr.value : null;
      continue;
    }
    if (GROUP_CLOSE_RE.test(line)) {
      flush();
      currentGroupId = null;
      continue;
    }
    bufferLines.push(line);
  }
  flush();

  return atoms;
}

/**
 * Locates the single source line holding the <atom .../> directive for
 * atomId, for a targeted single-line rewrite. Returns the line's prefix
 * (everything through "<atom "), its parsed attributes, and suffix
 * (the "/> -->" close), or null if no explicit directive with that id
 * exists (e.g. it is an implicit atom, or the document changed since the
 * board was opened).
 */
function findAtomDirectiveLine(sourceText, atomId) {
  const lines = sourceText.split("\n");
  for (let i = 0; i < lines.length; i++) {
    const m = lines[i].match(ATOM_TAG_RE);
    if (!m) continue;
    const attrs = parseAttrs(m[2]);
    const idAttr = attrs.find((a) => a.name === "id");
    if (idAttr && idAttr.value === atomId) {
      return { lineIndex: i, prefix: m[1], attrs, suffix: m[3], lines };
    }
  }
  return null;
}

// Rebuilds an <atom .../> directive line from an attribute list, preserving
// the line's own indentation/comment wrapper (prefix/suffix) untouched and
// keeping the whole directive on one source line, per SPEC.md.
function serializeAtomLine(prefix, attrs, suffix) {
  const attrText = attrs
    .map((a) => `${a.name}="${escapeAttrValue(a.value)}"`)
    .join(" ");
  return `${prefix}${attrText} ${suffix}`;
}

// ---------------------------------------------------------------------------
// Drag-to-reorder: source-line-range scan + move.
//
// parseAtoms() above answers "what atoms exist and what attributes do they
// carry" for rendering. It deliberately throws away line-range information
// (an atom's .text is just its content, joined). Reordering needs the
// opposite: not the content, but exactly which source lines make up a
// movable unit, so a move can cut and reinsert those lines verbatim and
// leave everything else byte-identical.
//
// A "unit" is the thing a drag actually moves:
//   - a standalone atom (explicit or implicit): its directive line, if any,
//     plus its one content block.
//   - a whole atom-group: every line from "<atom-group ...>" through
//     "</atom-group>", regardless of how many atoms or blank lines are
//     inside it.
//
// Group contiguity decision (iugum-w6y): a group always moves as one
// indivisible unit. emit.go rejects a discontiguous group
// (TestEmitRejectsDiscontiguousGroup) — dragging a single member out from
// between other members would produce exactly that. Moving the whole span
// verbatim, as one cut/paste, makes a discontiguous result structurally
// impossible: the lines between "<atom-group ...>" and "</atom-group>"
// never separate from each other, so the invariant holds by construction,
// not by a check this code has to remember to run. It also matches what an
// atom-group means per SPEC.md ("Extensions") — a deliberately grouped set
// of related atoms, most often materialize --split's split list items,
// which are meant to stay together. Refusing the drag instead was
// rejected: it would make roughly a third of a split-list document
// (testdata/valid/split-list.md) immovable for no benefit to the user, who
// dragged one card meaning "move this content," not "move this one list
// item out of its list." Dissolving the group on drag was also rejected:
// silently deleting group structure as a side effect of reordering is a
// surprising, hard-to-notice content change — the group markers are
// meaningful data (see split.go), not incidental formatting.
// ---------------------------------------------------------------------------

/**
 * Scans sourceText into an ordered list of top-level reorderable units, plus
 * the line range of any fixed preamble (currently just the <atomdown
 * version="1"/> marker, if present) that always stays first and is never
 * itself draggable.
 *
 * Returns { lines, preambleEndLine, units }, where each unit is
 * { unitKey, kind: "atom" | "group", startLine, endLine, atomIds, groupId,
 *   implicit }. startLine/endLine are inclusive 0-based indices into `lines`
 * and cover exactly that unit's own lines — no leading/trailing blank line,
 * no neighboring unit's content.
 *
 * unitKey is "atom:<id>" for a standalone atom (implicit atoms use their
 * "implicit-N" id, numbered in the same left-to-right document order
 * parseAtoms() uses, so a card's data-atom-id always resolves to the right
 * unit) or "group:<groupId>" for a whole group — every atom inside a group
 * shares that one unit, which is exactly what makes moving the group by its
 * unit key keep it contiguous.
 */
function computeUnits(sourceText) {
  const lines = sourceText.split("\n");
  const n = lines.length;
  const units = [];

  function isBoundary(line) {
    return line.trim() === "" ||
      ATOM_TAG_RE.test(line) ||
      GROUP_OPEN_RE.test(line) ||
      GROUP_CLOSE_RE.test(line) ||
      DOC_MARKER_RE.test(line);
  }

  let i = 0;
  let preambleEndLine = -1;

  // Fixed preamble: leading blank lines, then the doc marker line if
  // present. Never reordered — it is document-level metadata, not an atom.
  while (i < n) {
    if (lines[i].trim() === "") { i++; continue; }
    if (DOC_MARKER_RE.test(lines[i])) {
      preambleEndLine = i;
      i++;
      continue;
    }
    break;
  }

  let implicitCounter = 0;

  while (i < n) {
    if (lines[i].trim() === "") { i++; continue; }

    const groupOpenMatch = lines[i].match(GROUP_OPEN_RE);
    if (groupOpenMatch) {
      const startLine = i;
      const groupAttrs = parseAttrs(groupOpenMatch[1]);
      const groupIdAttr = groupAttrs.find((a) => a.name === "id");
      const groupId = groupIdAttr ? groupIdAttr.value : null;
      i++;
      const atomIds = [];
      while (i < n && !GROUP_CLOSE_RE.test(lines[i])) {
        const am = lines[i].match(ATOM_TAG_RE);
        if (am) {
          const attrs = parseAttrs(am[2]);
          const idAttr = attrs.find((a) => a.name === "id");
          if (idAttr) atomIds.push(idAttr.value);
        }
        i++;
      }
      // If the group never closes (malformed document), fall back to
      // treating whatever remains as this unit's span rather than
      // throwing — findAtomDirectiveLine-style leniency.
      const endLine = i < n ? i : n - 1;
      units.push({
        unitKey: `group:${groupId}`,
        kind: "group",
        startLine,
        endLine,
        atomIds,
        groupId,
      });
      if (i < n) i++; // step past the close marker line itself
      continue;
    }

    const atomMatch = lines[i].match(ATOM_TAG_RE);
    if (atomMatch) {
      const attrs = parseAttrs(atomMatch[2]);
      const idAttr = attrs.find((a) => a.name === "id");
      if (idAttr) {
        const startLine = i;
        i++;
        let endLine = startLine;
        while (i < n && !isBoundary(lines[i])) { endLine = i; i++; }
        units.push({
          unitKey: `atom:${idAttr.value}`,
          kind: "atom",
          startLine,
          endLine,
          atomIds: [idAttr.value],
          groupId: null,
        });
        continue;
      }
      // No id: not a valid directive by SPEC.md ("Identity"). Fall through
      // and treat the line as ordinary content, same as parseAtoms() does.
    }

    // Implicit atom: a content block with no directive of its own.
    const startLine = i;
    let endLine = i;
    i++;
    while (i < n && !isBoundary(lines[i])) { endLine = i; i++; }
    implicitCounter++;
    units.push({
      unitKey: `atom:implicit-${implicitCounter}`,
      kind: "atom",
      startLine,
      endLine,
      atomIds: [`implicit-${implicitCounter}`],
      groupId: null,
      implicit: true,
    });
  }

  return { lines, preambleEndLine, units };
}

/**
 * Moves one unit (an atom, or a whole group — see computeUnits() above) to
 * a new position relative to another unit, and returns the rewritten
 * document text. Never called with cached state: the caller (reorderAtom
 * below) always re-scans a freshly-read copy of the page first, the same
 * "re-read, don't trust the client" pattern saveAttrs() uses.
 *
 * movedUnitKey/targetUnitKey are unit keys as produced by computeUnits()
 * ("atom:<id>" or "group:<id>"). targetUnitKey may be null with
 * placement "end" (drop past the last card) or "start" (drop before the
 * first); otherwise placement is "before" or "after" the target unit.
 *
 * Returns { ok: true, text } on a real change, { ok: true, unchanged: true }
 * if the drop would not change the order (e.g. dropped adjacent to its own
 * current position, or on itself), or { ok: false, error } if either unit
 * can no longer be found — the document changed since the board was drawn.
 */
function reorderUnit(sourceText, movedUnitKey, targetUnitKey, placement) {
  const { lines, preambleEndLine, units } = computeUnits(sourceText);

  if (units.length === 0) {
    return { ok: false, error: "No reorderable blocks found in this document" };
  }

  const movedIndex = units.findIndex((u) => u.unitKey === movedUnitKey);
  if (movedIndex === -1) {
    return {
      ok: false,
      error: "Could not find the dragged block (document changed since the board opened?)",
    };
  }
  const moved = units[movedIndex];

  let targetIndex;
  if (targetUnitKey == null) {
    targetIndex = placement === "start" ? 0 : units.length;
  } else {
    targetIndex = units.findIndex((u) => u.unitKey === targetUnitKey);
    if (targetIndex === -1) {
      return {
        ok: false,
        error: "Could not find the drop target (document changed since the board opened?)",
      };
    }
    if (targetIndex === movedIndex) return { ok: true, unchanged: true };
    if (placement === "after") targetIndex += 1;
  }

  const remaining = units.filter((_, idx) => idx !== movedIndex);
  let insertAt = movedIndex < targetIndex ? targetIndex - 1 : targetIndex;
  insertAt = Math.max(0, Math.min(insertAt, remaining.length));
  const newOrder = remaining.slice(0, insertAt)
    .concat([moved], remaining.slice(insertAt));

  const sameOrder = newOrder.length === units.length &&
    newOrder.every((u, idx) => u.unitKey === units[idx].unitKey);
  if (sameOrder) return { ok: true, unchanged: true };

  // Gap map: the exact original blank-line run between each pair of units
  // that were adjacent in the original document, keyed by
  // "beforeKey|afterKey". A pair that stays adjacent in the same order
  // after the move reuses its original gap line-for-line — that is how a
  // user's own extra blank lines, or a genuinely zero-blank-line seam like
  // the one between the <atomdown version="1"/> marker and the first atom
  // in atomdown/testdata/valid/split-list.md, survive a drag that never
  // touched that seam. A brand new seam (created by the move) gets exactly
  // one blank line, matching every top-level separator already used in
  // atomdown/testdata/valid/{groups,split-list}.md.
  const PREAMBLE_KEY = " preamble";
  const gapMap = new Map();
  if (preambleEndLine >= 0) {
    gapMap.set(
      `${PREAMBLE_KEY}|${units[0].unitKey}`,
      lines.slice(preambleEndLine + 1, units[0].startLine),
    );
  }
  for (let i = 0; i + 1 < units.length; i++) {
    gapMap.set(
      `${units[i].unitKey}|${units[i + 1].unitKey}`,
      lines.slice(units[i].endLine + 1, units[i + 1].startLine),
    );
  }

  const resultLines = [];
  if (preambleEndLine >= 0) {
    resultLines.push(...lines.slice(0, preambleEndLine + 1));
    const gap = gapMap.get(`${PREAMBLE_KEY}|${newOrder[0].unitKey}`);
    resultLines.push(...(gap !== undefined ? gap : [""]));
  }
  newOrder.forEach((unit, idx) => {
    if (idx > 0) {
      const prev = newOrder[idx - 1];
      const gap = gapMap.get(`${prev.unitKey}|${unit.unitKey}`);
      resultLines.push(...(gap !== undefined ? gap : [""]));
    }
    resultLines.push(...lines.slice(unit.startLine, unit.endLine + 1));
  });

  // Whatever trailed the original last unit (typically just the file's
  // final newline, possibly extra trailing blank lines) stays at the very
  // end of the document regardless of which unit now ends it.
  const lastOriginalUnit = units[units.length - 1];
  resultLines.push(...lines.slice(lastOriginalUnit.endLine + 1));

  return { ok: true, text: resultLines.join("\n") };
}

// ---------------------------------------------------------------------------
// PURE DECISION FUNCTIONS
//
// Everything in this block is a plain function of its arguments: no syscall,
// no DOM, no module state. Two reasons they live here rather than inline in
// the client script string:
//
//   1. They are unit-tested directly (atomdown-board.test.mjs). The first
//      version of the drop handler decided "where does this land" inline
//      inside an event listener, where nothing could reach it — which is how
//      it shipped always dropping at the end of the document.
//   2. The client script gets them by source injection
//      (`injectFunctions()` below stringifies them into the panel script), so
//      the panel and the worker share one copy rather than two that drift.
//
// A function in this block must therefore stay self-contained: it may call
// another function in this block, but nothing else.
// ---------------------------------------------------------------------------

/**
 * Picks the drop target for a pointer at clientY.
 *
 * cards is every rendered card's [{unitKey, top, bottom}] in document order,
 * as read from getBoundingClientRect(). The rule: drop BEFORE the first card
 * whose vertical midpoint sits below the pointer. Past the last card's
 * midpoint means AFTER that last card, and past its bottom edge means the
 * end of the document.
 *
 * This is why the drop no longer always lands at the end: the previous
 * version had one handler on the cards container that hardcoded
 * (null, "end"), and the flex gap between cards plus the container's own
 * padding are container hit-targets, so a release in the space between two
 * cards was read as "past the last card". Geometry does not care which
 * element the pointer technically landed on.
 */
function pickDropTarget(clientY, cards) {
  if (!cards || cards.length === 0) {
    return { targetUnitKey: null, placement: "end" };
  }
  for (let i = 0; i < cards.length; i++) {
    const card = cards[i];
    if (clientY < (card.top + card.bottom) / 2) {
      return { targetUnitKey: card.unitKey, placement: "before" };
    }
  }
  const last = cards[cards.length - 1];
  if (clientY <= last.bottom) {
    return { targetUnitKey: last.unitKey, placement: "after" };
  }
  return { targetUnitKey: null, placement: "end" };
}

/**
 * The unit key of one card. Mirrors computeUnits(): a card that belongs to a
 * group resolves to that whole group's key, because a group is one unit.
 */
function unitKeyForCard(card) {
  return card && card.groupId
    ? "group:" + card.groupId
    : "atom:" + (card ? card.id : "");
}

/**
 * The document's unit order, derived from the rendered card list. Consecutive
 * cards that share a unit key (the members of one group) collapse to that one
 * unit, so this is the same order computeUnits() produces from the source.
 */
function unitOrderFromCards(cards) {
  const order = [];
  (cards || []).forEach(function (card) {
    const key = unitKeyForCard(card);
    if (order.length === 0 || order[order.length - 1] !== key) order.push(key);
  });
  return order;
}

/** Deduplicates a key list, keeping first-seen order. */
function dedupeKeys(keys) {
  const out = [];
  (keys || []).forEach(function (key) {
    if (out.indexOf(key) === -1) out.push(key);
  });
  return out;
}

/**
 * True when every selected unit is already adjacent to the next one in
 * document order — no unselected unit sits between two selected ones.
 *
 * An atom-group wraps a contiguous span, so this is the whole contiguity
 * rule: Group is offered only for a selection that is already adjacent in
 * source order. Steve decided this directly (iugum-w6y.3): the board does
 * not silently reorder a document to make a selection groupable, because a
 * reorder is a real content change the user did not ask for.
 */
function isContiguousUnitSelection(unitOrder, selectedKeys) {
  const positions = [];
  for (let i = 0; i < unitOrder.length; i++) {
    if (selectedKeys.indexOf(unitOrder[i]) !== -1) positions.push(i);
  }
  if (positions.length === 0) return false;
  return positions[positions.length - 1] - positions[0] === positions.length - 1;
}

/**
 * Decides what the card menu's group item says and whether it is enabled.
 *
 * menuUnitKey is the unit of the card whose menu is open. When that card is
 * already in a group the item reads Ungroup and is always enabled — the group
 * under the cursor is what it acts on, so no selection is needed. Otherwise
 * the item reads Group and is enabled only for a selection that Atomdown
 * Core 1 actually permits: two or more units, none of them already a group
 * (Core 1 does not permit nested groups), the menu's own card among them, and
 * every one of them adjacent in source order.
 *
 * A disabled item keeps a reason, which the panel puts in the item's tooltip.
 * Refusing silently would look like a broken button.
 */
function groupMenuState(unitOrder, selectedKeys, menuUnitKey) {
  if (menuUnitKey && menuUnitKey.indexOf("group:") === 0) {
    return {
      action: "ungroup",
      label: "Ungroup",
      enabled: true,
      reason: "Remove this group's markers. Every atom inside it stays.",
    };
  }
  const keys = dedupeKeys(selectedKeys);
  if (keys.length < 2) {
    return {
      action: "group",
      label: "Group",
      enabled: false,
      reason: "Select two or more cards to group them.",
    };
  }
  for (let i = 0; i < keys.length; i++) {
    if (keys[i].indexOf("group:") === 0) {
      return {
        action: "group",
        label: "Group",
        enabled: false,
        reason: "Atomdown Core 1 does not permit a group inside a group.",
      };
    }
  }
  if (menuUnitKey && keys.indexOf(menuUnitKey) === -1) {
    return {
      action: "group",
      label: "Group",
      enabled: false,
      reason: "This card is not in the selection. Open the menu on a selected card.",
    };
  }
  if (!isContiguousUnitSelection(unitOrder, keys)) {
    return {
      action: "group",
      label: "Group",
      enabled: false,
      reason:
        "These cards are not next to each other in the document. A group wraps a " +
        "run of blocks, so grouping them would have to move blocks, and the board " +
        "will not move a block you did not drag.",
    };
  }
  return {
    action: "group",
    label: "Group",
    enabled: true,
    reason: "Wrap these " + keys.length + " cards in one atom-group.",
  };
}

/** True when two client rects overlap. Used by the lasso. */
function rectsIntersect(a, b) {
  return a.left < b.right && a.right > b.left &&
    a.top < b.bottom && a.bottom > b.top;
}

/**
 * The smallest single replacement that turns oldText into newText: the common
 * prefix and common suffix are left alone.
 *
 * This is what makes the native undo shortcut work. A group, an ungroup and a
 * reorder all rewrite the document by whole lines, but they reach the editor
 * as ONE editor.replaceRange call, which becomes one CodeMirror transaction
 * and therefore one entry in the editor's own undo history. There is no
 * private undo stack in this plug.
 *
 * Returns null when the texts are identical.
 */
function minimalEdit(oldText, newText) {
  if (oldText === newText) return null;
  let start = 0;
  const shortest = Math.min(oldText.length, newText.length);
  while (start < shortest && oldText[start] === newText[start]) start++;
  let endOld = oldText.length;
  let endNew = newText.length;
  while (
    endOld > start && endNew > start &&
    oldText[endOld - 1] === newText[endNew - 1]
  ) {
    endOld--;
    endNew--;
  }
  return { from: start, to: endOld, insert: newText.slice(start, endNew) };
}

/** An eight-character Crockford Base32 id, the shape Atomdown Core requires. */
const CROCKFORD_BASE32 = "0123456789ABCDEFGHJKMNPQRSTVWXYZ";
const CROCKFORD_ID_RE = /^[0-9A-HJKMNP-TV-Z]{8}$/;

/**
 * Generates one Atomdown id: eight characters of the uppercase Crockford
 * Base32 alphabet, 40 random bits — the same shape and the same entropy as
 * atomdown's own NewID (atomdown/id.go), which the `atomdown id` CLI prints.
 * A worker cannot run that binary, so this reproduces it rather than shelling
 * out. Each byte is masked to five bits, and 256 divides by 32 exactly, so
 * there is no modulo bias.
 */
function newAtomdownId() {
  const raw = new Uint8Array(8);
  crypto.getRandomValues(raw);
  let out = "";
  for (let i = 0; i < raw.length; i++) {
    out += CROCKFORD_BASE32[raw[i] & 31];
  }
  return out;
}

/** Every id already used in the document, so a new one cannot collide. */
function existingIds(sourceText) {
  const found = [];
  const re = /\bid\s*=\s*(?:"([^"]*)"|'([^']*)')/g;
  let m;
  while ((m = re.exec(sourceText)) !== null) {
    found.push(m[1] !== undefined ? m[1] : m[2]);
  }
  return found;
}

/**
 * Wraps a contiguous run of units in one atom-group, and returns the whole
 * rewritten document.
 *
 * The two markers are the ONLY bytes this adds. No block's text changes, so
 * no atom's `digest` can go stale, and no atom's directive line is touched,
 * so every `id` and every extension attribute survives untouched. That is the
 * point of writing the group as two inserted lines rather than re-emitting
 * the atoms.
 *
 * The markers go directly against the run — the open marker immediately
 * before the first unit's first line, the close marker immediately after the
 * last unit's last line — which is the shape `materialize --split list-item`
 * already writes (atomdown/testdata/valid/split-list.md). It also makes an
 * ungroup an exact inverse: removing those two lines restores the original
 * bytes.
 */
function insertGroupMarkers(sourceText, unitKeys, groupId) {
  const { lines, units } = computeUnits(sourceText);
  const keys = dedupeKeys(unitKeys);
  if (keys.length < 2) {
    return { ok: false, error: "Select two or more cards to group them." };
  }
  if (!CROCKFORD_ID_RE.test(groupId)) {
    return { ok: false, error: "A group id must be eight Crockford Base32 characters." };
  }
  if (existingIds(sourceText).indexOf(groupId) !== -1) {
    return { ok: false, error: "That group id is already used in this document." };
  }
  const positions = [];
  for (let i = 0; i < units.length; i++) {
    if (keys.indexOf(units[i].unitKey) !== -1) positions.push(i);
  }
  if (positions.length !== keys.length) {
    return {
      ok: false,
      error: "Could not find every selected card (document changed since the board opened?)",
    };
  }
  for (let i = 0; i < positions.length; i++) {
    if (units[positions[i]].kind === "group") {
      return { ok: false, error: "Atomdown Core 1 does not permit a group inside a group." };
    }
  }
  if (positions[positions.length - 1] - positions[0] !== positions.length - 1) {
    return {
      ok: false,
      error: "Those cards are not next to each other in the document, so a group cannot wrap them.",
    };
  }

  const first = units[positions[0]];
  const last = units[positions[positions.length - 1]];
  const out = lines.slice(0, first.startLine)
    .concat(['<!-- <atom-group id="' + groupId + '"> -->'])
    .concat(lines.slice(first.startLine, last.endLine + 1))
    .concat(["<!-- </atom-group> -->"])
    .concat(lines.slice(last.endLine + 1));
  return { ok: true, text: out.join("\n"), groupId };
}

/**
 * Removes one line and, when that leaves two blank lines against each other,
 * one of those blanks. A group written by insertGroupMarkers() has no blank
 * line inside its markers, so nothing collapses and an ungroup is byte-exact;
 * a group written loosely by hand (atomdown/testdata/valid/groups.md puts a
 * blank line after the open marker) would otherwise leave a doubled blank
 * line behind.
 */
function removeLineCollapsingSeam(lines, index) {
  const out = lines.slice(0, index).concat(lines.slice(index + 1));
  const before = index - 1 >= 0 ? out[index - 1] : null;
  const after = index < out.length ? out[index] : null;
  if (
    before !== null && after !== null &&
    before.trim() === "" && after.trim() === ""
  ) {
    return out.slice(0, index).concat(out.slice(index + 1));
  }
  return out;
}

/**
 * Removes one group's two marker lines and nothing else, and returns the whole
 * rewritten document. Every atom that was inside the group stays exactly where
 * it was, with its directive line, its id and its digest untouched.
 */
function removeGroupMarkers(sourceText, groupId) {
  const { lines, units } = computeUnits(sourceText);
  const unit = units.find(function (u) { return u.unitKey === "group:" + groupId; });
  if (!unit) {
    return {
      ok: false,
      error: "Could not find that group (document changed since the board opened?)",
    };
  }
  if (!GROUP_CLOSE_RE.test(lines[unit.endLine])) {
    return {
      ok: false,
      error: "That group has no closing marker. Fix the document before ungrouping it.",
    };
  }
  let out = removeLineCollapsingSeam(lines, unit.endLine);
  out = removeLineCollapsingSeam(out, unit.startLine);
  return { ok: true, text: out.join("\n") };
}

// CSS custom property names SilverBullet's own theme defines on the
// PARENT document's <html> element. A plug panel renders in an iframe, and
// CSS custom properties do not cross that boundary on their own — see
// applyParentTheme() in the client script below, which is what actually
// copies these across at runtime. Listed once here so the fallback values
// baked into buildBoardHtml()'s <style> block and this list can't drift
// apart silently.
const THEME_VAR_NAMES = [
  "--root-background-color",
  "--root-color",
  "--ui-surface-background-color",
  "--ui-surface-color",
  "--ui-surface-border-color",
  "--ui-surface-section-background-color",
  "--ui-surface-hover-background-color",
  "--ui-accent-color",
  "--ui-accent-contrast-color",
  "--subtle-color",
  "--subtle-background-color",
  "--link-color",
];

// The pure decision functions the panel script needs. They are injected into
// that script by source (Function.prototype.toString) rather than duplicated,
// so the panel and the worker cannot disagree about where a drop lands or
// whether a selection can be grouped. Every function listed here must stay
// self-contained — see the PURE DECISION FUNCTIONS block above.
const CLIENT_SHARED_FUNCTIONS = [
  pickDropTarget,
  unitKeyForCard,
  unitOrderFromCards,
  dedupeKeys,
  isContiguousUnitSelection,
  groupMenuState,
  rectsIntersect,
];

function injectSharedFunctions() {
  return CLIENT_SHARED_FUNCTIONS.map(function (fn) { return fn.toString(); })
    .join("\n\n");
}

function buildBoardHtml(atoms, pageName) {
  const cardsHtml = atoms.map((atom) => {
    const classes = ["board-card"];
    if (atom.implicit) classes.push("board-card-implicit");
    if (atom.groupId) classes.push("board-card-grouped");
    const badges = [];
    if (atom.implicit) {
      badges.push('<span class="board-badge board-badge-implicit">implicit</span>');
    }
    if (atom.groupId) {
      badges.push(
        `<span class="board-badge board-badge-group">group ${escapeHtml(atom.groupId)}</span>`,
      );
    }
    return `
      <div class="${classes.join(" ")}" data-atom-id="${escapeHtml(atom.id)}">
        <div class="board-card-header" draggable="true" data-drag-atom="${escapeHtml(atom.id)}" title="Drag to move${atom.groupId ? " (moves the whole group)" : ""}">
          <span class="board-drag-handle" aria-hidden="true">&#10021;&#10021;</span>
          <span class="board-card-id">${escapeHtml(atom.id)}</span>
          ${badges.join("")}
          <div class="board-card-menu">
            <button type="button" class="board-menu-btn" data-menu-toggle="${escapeHtml(atom.id)}" title="Attributes" aria-haspopup="true">&#8942;</button>
            <div class="board-menu-popover" data-menu-popover="${escapeHtml(atom.id)}" hidden></div>
          </div>
        </div>
        <pre class="board-card-body">${escapeHtml(atom.text)}</pre>
      </div>`;
  }).join("\n");

  const style = `
    :root {
      /* Light-theme snapshot as the fallback for every var() below — never
         a dark guess. applyParentTheme() in the client script overwrites
         these with the real live values (light or dark) read from the
         parent document the moment this panel loads, so the fallback only
         shows for the instant before that runs, or if reading the parent
         ever fails. */
      --root-background-color: #ffffff;
      --root-color: #37352f;
      --ui-surface-background-color: #ffffff;
      --ui-surface-color: #37352f;
      --ui-surface-border-color: #e9e9e7;
      --ui-surface-section-background-color: #f7f6f3;
      --ui-surface-hover-background-color: #f1f0ee;
      --ui-accent-color: #2383e2;
      --ui-accent-contrast-color: #ffffff;
      --subtle-color: #787774;
      --subtle-background-color: #f7f6f3;
      --link-color: #0330cb;
    }
    body {
      margin: 0;
      padding: 0;
      font-family: var(--board-font-family, system-ui, -apple-system, sans-serif);
      background: var(--root-background-color);
      color: var(--root-color);
    }
    .board-toolbar {
      display: flex;
      align-items: center;
      justify-content: space-between;
      padding: 10px 16px;
      border-bottom: 1px solid var(--ui-surface-border-color);
      position: sticky;
      top: 0;
      background: inherit;
      z-index: 20;
    }
    .board-title { font-weight: 600; font-size: 14px; }
    .board-close {
      cursor: pointer;
      border: 1px solid var(--ui-surface-border-color);
      background: transparent;
      color: inherit;
      border-radius: 4px;
      padding: 4px 12px;
      font-size: 13px;
    }
    .board-close:hover { background: var(--ui-surface-hover-background-color); }
    /* A single vertical column, like TiddlyWiki's story river. The column IS
       the document's order, which is what makes drag-to-reorder meaningful:
       moving a card up or down moves that block in the source file. A grid
       would have no natural sequence to map onto. */
    .board-cards {
      display: flex;
      flex-direction: column;
      /* gap: 0 on purpose. A flex gap is a hole in the card strip that
         belongs to the container, so a pointer released between two cards
         landed on the container rather than on either card. The separation is
         a card margin instead, and the drop target comes from geometry
         (pickDropTarget) rather than from which element the pointer hit, so
         neither the gap nor this padding can be mistaken for "past the last
         card" any more. */
      gap: 0;
      padding: 16px;
      max-width: 820px;
      margin: 0 auto;
      align-items: stretch;
    }
    .board-card + .board-card { margin-top: 14px; }
    .board-card {
      border: 1px solid var(--ui-surface-border-color);
      border-radius: 6px;
      background: var(--ui-surface-section-background-color);
      display: flex;
      flex-direction: column;
      min-height: 60px;
      width: 100%;
      transition: box-shadow 0.1s ease-out;
    }
    .board-card-implicit { border-style: dashed; }
    .board-card-grouped { border-left: 3px solid var(--ui-accent-color); }
    /* Drag state, applied by the client script below. board-card-dragging
       marks every card sharing the dragged unit (a whole group drags
       together, see computeUnits() in the worker code); dropbefore/after
       mark the card the pointer is currently hovering, to show where the
       drop would land. */
    .board-card-dragging { opacity: 0.4; }
    .board-card-dropbefore { box-shadow: inset 0 3px 0 0 var(--ui-accent-color); }
    .board-card-dropafter { box-shadow: inset 0 -3px 0 0 var(--ui-accent-color); }
    /* Selection. The border is the SAME blue as the drop indicator two lines
       up and as the grouped-card left edge: --ui-accent-color, SilverBullet's
       own accent token, copied live from the parent document by
       applyParentTheme(). Selection must not introduce a second blue. */
    .board-card-selected {
      border: 2px solid var(--ui-accent-color);
    }
    /* A grouped card keeps its thicker left edge when it is also selected,
       so the group marking does not disappear under the selection border. */
    .board-card-grouped.board-card-selected { border-left-width: 3px; }
    .board-lasso {
      position: fixed;
      z-index: 40;
      border: 1px solid var(--ui-accent-color);
      background: var(--ui-accent-color);
      opacity: 0.18;
      pointer-events: none;
    }
    .board-card-header {
      display: flex;
      align-items: center;
      gap: 6px;
      flex-wrap: wrap;
      padding: 6px 8px;
      border-bottom: 1px solid var(--ui-surface-border-color);
      position: relative;
      cursor: grab;
    }
    .board-card-header:active { cursor: grabbing; }
    .board-drag-handle {
      opacity: 0.45;
      font-size: 11px;
      line-height: 1;
      user-select: none;
    }
    .board-card-id {
      font-family: ui-monospace, monospace;
      font-size: 11px;
      color: var(--subtle-color);
    }
    .board-badge {
      font-size: 10px;
      padding: 1px 6px;
      border-radius: 999px;
      background: var(--ui-surface-hover-background-color);
      color: var(--subtle-color);
    }
    .board-badge-group { background: var(--ui-accent-color); color: var(--ui-accent-contrast-color); }
    .board-card-body {
      margin: 0;
      padding: 8px;
      font-family: ui-monospace, monospace;
      font-size: 12px;
      white-space: pre-wrap;
      word-break: break-word;
      flex: 1;
    }
    .board-card-menu { position: relative; margin-left: auto; }
    .board-menu-btn {
      cursor: pointer;
      background: transparent;
      border: none;
      color: inherit;
      font-size: 16px;
      line-height: 1;
      padding: 2px 6px;
      border-radius: 4px;
    }
    .board-menu-btn:hover { background: var(--ui-surface-hover-background-color); }
    .board-menu-popover {
      position: absolute;
      top: 100%;
      right: 0;
      z-index: 30;
      background: var(--ui-surface-background-color);
      border: 1px solid var(--ui-surface-border-color);
      border-radius: 6px;
      padding: 8px;
      min-width: 240px;
      box-shadow: 0 4px 12px rgba(0,0,0,0.2);
    }
    .board-menu-popover[hidden] { display: none; }
    .board-menu-title {
      font-size: 12px;
      font-weight: 600;
      margin-bottom: 6px;
      color: var(--subtle-color);
    }
    .board-attr-empty { font-size: 12px; color: var(--subtle-color); padding: 4px 0; }
    .board-attr-row { display: flex; gap: 4px; margin-bottom: 4px; }
    .board-attr-name, .board-attr-value {
      flex: 1;
      min-width: 0;
      font-size: 12px;
      padding: 3px 5px;
      background: var(--ui-surface-section-background-color);
      border: 1px solid var(--ui-surface-border-color);
      color: inherit;
      border-radius: 3px;
      font-family: ui-monospace, monospace;
    }
    .board-attr-name:disabled, .board-attr-value:disabled { opacity: 0.6; }
    .board-attr-remove {
      cursor: pointer;
      border: none;
      background: transparent;
      color: inherit;
      opacity: 0.7;
      width: 20px;
    }
    .board-attr-remove:hover { opacity: 1; }
    .board-attr-remove:disabled { visibility: hidden; }
    .board-menu-actions { display: flex; gap: 6px; margin-top: 6px; }
    .board-attr-add, .board-attr-save {
      cursor: pointer;
      font-size: 12px;
      border: 1px solid var(--ui-surface-border-color);
      background: transparent;
      color: inherit;
      border-radius: 4px;
      padding: 3px 10px;
    }
    .board-attr-add:hover, .board-attr-save:hover { background: var(--ui-surface-hover-background-color); }
    .board-menu-status { font-size: 11px; margin-top: 4px; color: var(--subtle-color); min-height: 14px; }
    /* The Group / Ungroup item. A disabled item stays VISIBLE and grayed,
       never hidden: a user who selected the wrong set needs to see that the
       action exists and read the tooltip saying why it is not offered. */
    .board-menu-group-row {
      margin-bottom: 8px;
      padding-bottom: 8px;
      border-bottom: 1px solid var(--ui-surface-border-color);
    }
    .board-menu-item {
      width: 100%;
      text-align: left;
      cursor: pointer;
      font-size: 12px;
      border: 1px solid var(--ui-surface-border-color);
      background: transparent;
      color: inherit;
      border-radius: 4px;
      padding: 4px 10px;
    }
    .board-menu-item:hover:not(:disabled) { background: var(--ui-surface-hover-background-color); }
    .board-menu-item:disabled {
      cursor: not-allowed;
      opacity: 0.45;
      color: var(--subtle-color);
    }
  `;

  const html = `
    <style>${style}</style>
    <div class="board-toolbar">
      <div class="board-title">Atomdown Board${pageName ? " — " + escapeHtml(pageName) : ""}</div>
      <button class="board-close" id="atomdown-board-close">Close</button>
    </div>
    <div class="board-cards">${cardsHtml || "<p style=\"padding:16px;color:var(--subtle-color);\">No atoms found in this document.</p>"}</div>
  `;

  // Everything below runs inside the panel iframe (see
  // client/components/panel_html.ts). That bootstrap page already defines
  // a global `syscall()` that round-trips through postMessage to the host,
  // so this script can call syscalls directly with no plug-side wiring.
  //
  // This client script is deliberately attribute-agnostic: it renders
  // whatever {name, value} pairs ATOMDOWN_BOARD_DATA hands it, with no
  // knowledge of what any name means. The only name it treats specially is
  // "id" (disabled, not removable) because Atomdown Core requires one on
  // every atom — see the header comment in this file.
  const clientScript = `
    // --- Shared pure decision functions -------------------------------
    // Injected by source from the worker module (see injectSharedFunctions)
    // so there is exactly one copy of each rule. Do not edit them here.
${injectSharedFunctions()}
    // --- end shared ----------------------------------------------------

    // Theme: SilverBullet's CSS custom properties live on the PARENT
    // document's <html>, and custom properties do not cross an iframe
    // boundary on their own. This panel iframe has no "sandbox" attribute
    // and is loaded via srcDoc (see silverbullet client/components/
    // panel.tsx), which makes it same-origin with the parent — so read the
    // parent's live computed values directly and copy them onto this
    // document's own root. If that ever fails (cross-origin, parent gone),
    // the :root fallback values baked into the <style> block above are the
    // light-theme snapshot, never a dark guess.
    var THEME_VAR_NAMES = ${JSON.stringify(THEME_VAR_NAMES)};

    function applyParentTheme() {
      try {
        var parentDoc = window.parent && window.parent.document;
        if (!parentDoc || !parentDoc.documentElement) return;
        var cs = window.parent.getComputedStyle(parentDoc.documentElement);
        THEME_VAR_NAMES.forEach(function (name) {
          var value = cs.getPropertyValue(name);
          if (value && value.trim()) {
            document.documentElement.style.setProperty(name, value.trim());
          }
        });
        // SilverBullet never styles <body>, so its computed fontFamily is the
        // browser default ("Times" on this machine). The font the app actually
        // renders with lives in the --editor-font custom property; fall back to
        // the editor element's own computed font, and only then to <body>.
        var fontFamily = cs.getPropertyValue("--editor-font").trim();
        if (!fontFamily) {
          var edEl = parentDoc.querySelector("#sb-editor .cm-content") ||
            parentDoc.querySelector("#sb-editor");
          if (edEl) fontFamily = window.parent.getComputedStyle(edEl).fontFamily;
        }
        if (!fontFamily) {
          var bodyEl = parentDoc.body || parentDoc.documentElement;
          fontFamily = window.parent.getComputedStyle(bodyEl).fontFamily;
        }
        if (fontFamily && !/^Times\b/.test(fontFamily)) {
          document.documentElement.style.setProperty("--board-font-family", fontFamily);
        }
      } catch (e) {
        // Cross-origin or otherwise unreachable - leave the light-theme
        // fallback in place rather than throw.
      }
    }

    applyParentTheme();
    // panel_html.ts's own top-level listener (outside this eval'd script)
    // already updates data-theme on this document when the parent toggles
    // theme; it does not carry the actual color values, so re-read them
    // here on the same message so a live toggle is reflected, not just the
    // moment this panel first opened.
    window.addEventListener("message", function (e) {
      if (e.data && e.data.type === "theme") applyParentTheme();
    });

    function el(tag, className) {
      var e = document.createElement(tag);
      if (className) e.className = className;
      return e;
    }

    function addAttrRow(listEl, name, value, isId) {
      var row = el("div", "board-attr-row");
      var nameInput = el("input", "board-attr-name");
      nameInput.placeholder = "name";
      nameInput.value = name || "";
      var valueInput = el("input", "board-attr-value");
      valueInput.placeholder = "value";
      valueInput.value = value || "";
      var removeBtn = el("button", "board-attr-remove");
      removeBtn.type = "button";
      removeBtn.textContent = "\\u2715";
      removeBtn.title = "Remove";
      if (isId) {
        nameInput.disabled = true;
        valueInput.disabled = true;
        removeBtn.disabled = true;
      } else {
        removeBtn.addEventListener("click", function (e) {
          // Stop the click here. Removing the row detaches this button from
          // the document, so by the time the click reaches the document
          // listener its target has no ancestors any more and the
          // "is this click inside a popover" test below cannot see that it
          // was — which closed the popover the user was still editing.
          e.stopPropagation();
          row.remove();
        });
      }
      row.appendChild(nameInput);
      row.appendChild(valueInput);
      row.appendChild(removeBtn);
      listEl.appendChild(row);
    }

    function closeAllPopovers(except) {
      document.querySelectorAll(".board-menu-popover").forEach(function (p) {
        if (p !== except) p.setAttribute("hidden", "");
      });
    }

    // --- Selection -----------------------------------------------------
    //
    // Selection is a pure UI state. It is never written to the document: no
    // coordinate, no index, no attribute — nothing about the board's state
    // reaches the file. The only thing that ever reaches the file is a block
    // move or a group marker, and both of those ARE the document's content.
    //
    // A selected card carries .board-card-selected, so the DOM is the single
    // source of truth for what is selected and there is no parallel array to
    // fall out of step with it.
    var CARD_ELS = Array.prototype.slice.call(
      document.querySelectorAll(".board-card[data-atom-id]"),
    );
    var cardDataFor = {};
    ATOMDOWN_BOARD_DATA.forEach(function (a) { cardDataFor[a.id] = a; });

    var selectionAnchor = -1;
    var lassoJustSelected = false;

    function cardDatum(cardEl) {
      return cardDataFor[cardEl.getAttribute("data-atom-id")];
    }

    function selectedCards() {
      return CARD_ELS.filter(function (c) {
        return c.classList.contains("board-card-selected");
      });
    }

    function selectedUnitKeys() {
      return dedupeKeys(selectedCards().map(function (c) {
        return unitKeyForCard(cardDatum(c));
      }));
    }

    function clearSelection() {
      CARD_ELS.forEach(function (c) { c.classList.remove("board-card-selected"); });
    }

    CARD_ELS.forEach(function (card, index) {
      card.addEventListener("click", function (e) {
        // The three-dot button and its popover have their own handlers; a
        // click in there is not a selection gesture.
        if (e.target.closest && e.target.closest(".board-card-menu")) return;
        // A real drag never produces a click, so this listener cannot fire
        // for a drag of the card header. Dragging still drags.
        e.stopPropagation();
        closeAllPopovers(null);
        var additive = e.metaKey || e.ctrlKey;
        if (e.shiftKey && selectionAnchor >= 0) {
          var from = Math.min(selectionAnchor, index);
          var to = Math.max(selectionAnchor, index);
          if (!additive) clearSelection();
          for (var i = from; i <= to; i++) {
            CARD_ELS[i].classList.add("board-card-selected");
          }
        } else if (additive) {
          card.classList.toggle("board-card-selected");
          selectionAnchor = index;
        } else {
          clearSelection();
          card.classList.add("board-card-selected");
          selectionAnchor = index;
        }
      });
    });

    // Lasso (rubber band). Starts only on empty board background — never on
    // a card, so it cannot compete with a card drag, which starts on the
    // card's own header.
    var lasso = null;

    function lassoRect(state, clientX, clientY) {
      return {
        left: Math.min(state.x0, clientX),
        right: Math.max(state.x0, clientX),
        top: Math.min(state.y0, clientY),
        bottom: Math.max(state.y0, clientY),
      };
    }

    document.addEventListener("mousedown", function (e) {
      if (e.button !== 0) return;
      if (e.target.closest && (e.target.closest(".board-card") ||
        e.target.closest(".board-toolbar"))) return;
      lasso = {
        x0: e.clientX,
        y0: e.clientY,
        additive: e.metaKey || e.ctrlKey || e.shiftKey,
        moved: false,
        box: el("div", "board-lasso"),
      };
      document.body.appendChild(lasso.box);
    });

    document.addEventListener("mousemove", function (e) {
      if (!lasso) return;
      var r = lassoRect(lasso, e.clientX, e.clientY);
      if ((r.right - r.left) > 3 || (r.bottom - r.top) > 3) lasso.moved = true;
      lasso.box.style.left = r.left + "px";
      lasso.box.style.top = r.top + "px";
      lasso.box.style.width = (r.right - r.left) + "px";
      lasso.box.style.height = (r.bottom - r.top) + "px";
    });

    document.addEventListener("mouseup", function (e) {
      if (!lasso) return;
      var state = lasso;
      lasso = null;
      state.box.remove();
      if (!state.moved) return; // a plain click, not a drag: leave it to the click handler
      var band = lassoRect(state, e.clientX, e.clientY);
      if (!state.additive) clearSelection();
      CARD_ELS.forEach(function (card, index) {
        if (rectsIntersect(card.getBoundingClientRect(), band)) {
          card.classList.add("board-card-selected");
          if (selectionAnchor < 0) selectionAnchor = index;
        }
      });
      // The mouseup also produces a click on the background, which would
      // otherwise clear what the lasso just selected.
      lassoJustSelected = true;
    });

    // The document's unit order, and the group-item decision for one card.
    // Both come from the injected pure functions above, so the panel and the
    // worker apply the same contiguity and nesting rules.
    var UNIT_ORDER = unitOrderFromCards(ATOMDOWN_BOARD_DATA);

    function groupMenuStateFor(atom) {
      return groupMenuState(UNIT_ORDER, selectedUnitKeys(), unitKeyForCard(atom));
    }

    function refreshGroupItem(atom, popoverEl) {
      var btn = popoverEl.boardGroupBtn;
      if (!btn) return;
      var state = groupMenuStateFor(atom);
      btn.textContent = state.label;
      btn.disabled = !state.enabled;
      btn.title = state.reason;
    }

    function buildPopover(atom, popoverEl) {
      if (popoverEl.dataset.built === "1") return;
      popoverEl.dataset.built = "1";

      // Group / Ungroup. Built once, but its label, its enabled state and its
      // tooltip are refreshed on every open by refreshGroupItem(), because
      // they depend on the current selection, not on this card alone. A
      // disabled item stays visible and grayed with the reason in its
      // tooltip — see groupMenuState() in the worker code.
      var groupRow = el("div", "board-menu-group-row");
      var groupBtn = el("button", "board-menu-item");
      groupBtn.type = "button";
      groupRow.appendChild(groupBtn);
      popoverEl.appendChild(groupRow);
      popoverEl.boardGroupBtn = groupBtn;

      groupBtn.addEventListener("click", async function (e) {
        e.stopPropagation();
        if (groupBtn.disabled) return;
        var state = groupMenuStateFor(atom);
        if (!state.enabled) return;
        var busyLabel = state.action === "ungroup" ? "Ungrouping..." : "Grouping...";
        groupBtn.disabled = true;
        groupBtn.textContent = busyLabel;
        try {
          var result;
          if (state.action === "ungroup") {
            result = await syscall(
              "system.invokeFunction",
              "atomdown-board.ungroupAtoms",
              atom.groupId,
            );
          } else {
            result = await syscall(
              "system.invokeFunction",
              "atomdown-board.groupAtoms",
              JSON.stringify(selectedUnitKeys()),
            );
          }
          if (!result || !result.ok) {
            window.alert(
              state.label + " failed: " + ((result && result.error) || "unknown error"),
            );
            refreshGroupItem(atom, popoverEl);
          }
          // On success the worker re-renders this whole panel (same as a
          // successful drop), so this popover no longer exists.
        } catch (err) {
          window.alert(state.label + " failed: " + err.message);
          refreshGroupItem(atom, popoverEl);
        }
      });

      var title = el("div", "board-menu-title");
      title.textContent = "Attributes";
      popoverEl.appendChild(title);

      if (atom.implicit) {
        var empty = el("div", "board-attr-empty");
        empty.textContent = "No directive on this block yet (implicit atom) - nothing to edit.";
        popoverEl.appendChild(empty);
        return;
      }

      var listEl = el("div", "board-attrs-list");
      (atom.attrs || []).forEach(function (a) {
        addAttrRow(listEl, a.name, a.value, a.name === "id");
      });
      popoverEl.appendChild(listEl);

      var actions = el("div", "board-menu-actions");
      var addBtn = el("button", "board-attr-add");
      addBtn.type = "button";
      addBtn.textContent = "+ Add attribute";
      addBtn.addEventListener("click", function () {
        addAttrRow(listEl, "", "", false);
      });
      var saveBtn = el("button", "board-attr-save");
      saveBtn.type = "button";
      saveBtn.textContent = "Save";
      actions.appendChild(addBtn);
      actions.appendChild(saveBtn);
      popoverEl.appendChild(actions);

      var status = el("div", "board-menu-status");
      popoverEl.appendChild(status);

      saveBtn.addEventListener("click", async function () {
        var rows = Array.prototype.slice.call(listEl.querySelectorAll(".board-attr-row"));
        var attrs = [];
        var seenNames = {};
        var ok = true;
        rows.forEach(function (row) {
          var nameInput = row.querySelector(".board-attr-name");
          var valueInput = row.querySelector(".board-attr-value");
          if (nameInput.disabled) return; // the id row travels separately
          var name = nameInput.value.trim();
          if (!name) return; // silently drop blank rows rather than fail
          if (!/^[A-Za-z_][\\w.:-]*$/.test(name)) {
            status.textContent = "Skipping invalid attribute name: " + name;
            ok = false;
            return;
          }
          if (seenNames[name]) {
            status.textContent = "Duplicate attribute name: " + name;
            ok = false;
            return;
          }
          seenNames[name] = true;
          attrs.push({ name: name, value: valueInput.value });
        });
        if (!ok) return;
        status.textContent = "Saving...";
        try {
          var result = await syscall(
            "system.invokeFunction",
            "atomdown-board.saveAttrs",
            atom.id,
            JSON.stringify(attrs),
          );
          if (result && result.ok) {
            status.textContent = "Saved.";
          } else {
            status.textContent = "Save failed: " + ((result && result.error) || "unknown error");
          }
        } catch (e) {
          status.textContent = "Save failed: " + e.message;
        }
      });
    }

    document.querySelectorAll("[data-menu-toggle]").forEach(function (btn) {
      var atomId = btn.getAttribute("data-menu-toggle");
      var atom = ATOMDOWN_BOARD_DATA.find(function (a) { return a.id === atomId; });
      var popover = document.querySelector('[data-menu-popover="' + CSS.escape(atomId) + '"]');
      if (!atom || !popover) return;
      btn.addEventListener("click", function (e) {
        e.stopPropagation();
        var isHidden = popover.hasAttribute("hidden");
        closeAllPopovers(isHidden ? popover : null);
        if (isHidden) {
          buildPopover(atom, popover);
          // The group item depends on the current selection, so re-decide it
          // every time the menu opens rather than once when it was built.
          refreshGroupItem(atom, popover);
          popover.removeAttribute("hidden");
        } else {
          popover.setAttribute("hidden", "");
        }
      });
    });

    // One document-level click handler for "close the open menu" and "clear
    // the selection". It is position-aware: a click that landed anywhere
    // inside a popover or on a menu button is the user working IN the menu,
    // so it closes nothing. The previous version closed every popover on
    // every click with no such test, which is why clicking "+ Add attribute"
    // or into an attribute input shut the panel.
    document.addEventListener("click", function (e) {
      var target = e.target;
      var inPopover = target && target.closest &&
        (target.closest(".board-menu-popover") || target.closest(".board-card-menu"));
      if (inPopover) return;

      closeAllPopovers(null);

      // A click on empty board background, with no modifier, clears the
      // selection. A click on a card is handled by the card's own listener,
      // which stops propagation before it reaches here.
      var onCard = target && target.closest && target.closest(".board-card");
      if (onCard) return;
      if (lassoJustSelected) { lassoJustSelected = false; return; }
      if (e.metaKey || e.ctrlKey || e.shiftKey) return;
      clearSelection();
    });

    var closeBtn = document.getElementById("atomdown-board-close");
    if (closeBtn) {
      closeBtn.addEventListener("click", async function () {
        try { await syscall("editor.hidePanel", "modal"); } catch (e) {}
        try { await syscall("system.invokeFunction", "atomdown-board.notifyClosed"); } catch (e) {}
      });
    }

    // --- Drag to reorder -----------------------------------------------
    //
    // unitKeyForCard() (injected above, shared with the worker) mirrors
    // computeUnits(): an atom that belongs to a group resolves to that whole
    // group's key, so dragging or dropping onto any one member of a group
    // always means "the whole group". The actual cut and paste of source
    // lines happens fresh on the worker side against the real document; this
    // is only the client's notion of which cards move and highlight together.
    var dragState = null;

    function clearDropMarkers() {
      document.querySelectorAll(".board-card-dropbefore, .board-card-dropafter").forEach(function (c) {
        c.classList.remove("board-card-dropbefore", "board-card-dropafter");
      });
    }

    async function performDrop(movedUnitKey, targetUnitKey, placement) {
      try {
        var result = await syscall(
          "system.invokeFunction",
          "atomdown-board.reorderAtom",
          movedUnitKey,
          targetUnitKey,
          placement,
        );
        if (!result || !result.ok) {
          window.alert("Reorder failed: " + ((result && result.error) || "unknown error"));
        }
        // On success the worker re-renders this panel itself (see
        // reorderAtom in the worker code) - nothing to do here.
      } catch (e) {
        window.alert("Reorder failed: " + e.message);
      }
    }

    document.querySelectorAll(".board-card-header[data-drag-atom]").forEach(function (header) {
      var atomId = header.getAttribute("data-drag-atom");
      var atom = cardDataFor[atomId];
      if (!atom) return;
      var unitKey = unitKeyForCard(atom);

      header.addEventListener("dragstart", function (e) {
        dragState = { unitKey: unitKey };
        e.dataTransfer.effectAllowed = "move";
        try { e.dataTransfer.setData("text/plain", unitKey); } catch (err) {}
        CARD_ELS.forEach(function (c) {
          if (unitKeyForCard(cardDatum(c)) === unitKey) {
            c.classList.add("board-card-dragging");
          }
        });
      });

      header.addEventListener("dragend", function () {
        document.querySelectorAll(".board-card-dragging").forEach(function (c) {
          c.classList.remove("board-card-dragging");
        });
        clearDropMarkers();
        dragState = null;
      });
    });

    // One dragover/drop pair for the whole panel, decided by geometry.
    //
    // There are deliberately no per-card and no per-container drop handlers
    // any more. Those made the answer depend on WHICH element the pointer
    // happened to land on, and the space between two cards belongs to their
    // container, so a release there was read as "past the last card" and the
    // block went to the end of the document. pickDropTarget() reads the
    // cards' own rectangles instead, so the space between two cards resolves
    // to the seam between them, which is what the pointer was over.
    function cardGeometry() {
      return CARD_ELS.map(function (card) {
        var rect = card.getBoundingClientRect();
        return {
          unitKey: unitKeyForCard(cardDatum(card)),
          top: rect.top,
          bottom: rect.bottom,
          el: card,
        };
      });
    }

    // Marks the seam the drop would land on. A group is several cards sharing
    // one unit key, so "before" marks that unit's FIRST card and "after"
    // marks its LAST — one line at one seam, never a line per member.
    function markDropTarget(cards, decision) {
      clearDropMarkers();
      var chosen = null;
      for (var i = 0; i < cards.length; i++) {
        if (decision.targetUnitKey === null) {
          chosen = cards[cards.length - 1];
          break;
        }
        if (cards[i].unitKey !== decision.targetUnitKey) continue;
        chosen = cards[i];
        if (decision.placement === "before") break;
      }
      if (!chosen) return;
      chosen.el.classList.add(
        decision.targetUnitKey !== null && decision.placement === "before"
          ? "board-card-dropbefore"
          : "board-card-dropafter",
      );
    }

    document.addEventListener("dragover", function (e) {
      if (!dragState) return;
      e.preventDefault();
      if (e.dataTransfer) e.dataTransfer.dropEffect = "move";
      var cards = cardGeometry();
      var decision = pickDropTarget(e.clientY, cards);
      // Hovering over its own unit is not a move, so do not draw a line
      // promising one.
      if (decision.targetUnitKey === dragState.unitKey) {
        clearDropMarkers();
        return;
      }
      markDropTarget(cards, decision);
    });

    document.addEventListener("drop", async function (e) {
      if (!dragState) return;
      e.preventDefault();
      var movedUnitKey = dragState.unitKey;
      var decision = pickDropTarget(e.clientY, cardGeometry());
      clearDropMarkers();
      // Dropping onto its own unit is a no-op; reorderUnit() reports that
      // too, but there is no reason to make a round trip for it.
      if (decision.targetUnitKey === movedUnitKey) return;
      await performDrop(movedUnitKey, decision.targetUnitKey, decision.placement);
    });
  `;

  const clientData = atoms.map((atom) => ({
    id: atom.id,
    implicit: atom.implicit,
    groupId: atom.groupId || null,
    attrs: atom.attrs || [],
  }));

  const script = `var ATOMDOWN_BOARD_DATA = ${JSON.stringify(clientData)};\n${clientScript}`;

  return { html, script };
}

async function toggleBoard() {
  if (boardOpen) {
    await syscall("editor.hidePanel", "modal");
    boardOpen = false;
    return;
  }

  const [sourceText, pageName] = await Promise.all([
    syscall("editor.getText"),
    syscall("editor.getCurrentPage").catch(() => undefined),
  ]);

  const atoms = parseAtoms(sourceText);
  const { html, script } = buildBoardHtml(atoms, pageName);

  // Inset 0: this reads as a page VIEW, not a floating dialog, matching
  // Steve's expectation ("an option in the UI" that switches the current
  // page's display, not a popup over it). SilverBullet's own
  // client/styles/main.scss .sb-modal class still applies a border-radius,
  // a box-shadow, and a 1px border to the panel wrapper, and
  // .sb-modal-backdrop still exists as a sibling element behind it — both
  // are compiled into the PARENT document and are not reachable from this
  // plug's iframe content, so a faint rounded/shadowed edge at the screen
  // border is an unavoidable residual, not a bug in this file. Inset 0
  // does remove the floating margin and (once the background matches the
  // real page background, via applyParentTheme() above) the visible dim
  // behind it, since the panel now fully covers the backdrop.
  await syscall("editor.showPanel", "modal", 0, html, script);
  boardOpen = true;
}

// Called back from the panel's own close button (see buildBoardHtml's
// script above) so a click-to-close and re-running the toggle command agree
// on whether the board is open. Not a user-facing command itself.
function notifyClosed() {
  boardOpen = false;
}

// ---------------------------------------------------------------------------
// The write path: the editor buffer, not the space file.
//
// Every change this plug makes — an attribute edit, a reorder, a group, an
// ungroup — goes through editor.replaceRange, which dispatches ONE CodeMirror
// transaction on the live editor view (silverbullet client/plugos/syscalls/
// editor.ts). CodeMirror's history extension is installed
// (client/codemirror/editor_state.ts), so that transaction becomes one entry
// in the editor's own undo history and the user's native Cmd-Z / Cmd-Shift-Z
// undo and redo it like any other edit. SilverBullet then autosaves the
// buffer.
//
// This is why there is no undo stack in this plug and no "Undo group" button.
// The earlier path (space.readPage + space.writePage + editor.reloadPage)
// wrote around the editor, so the change was invisible to the undo history
// and Cmd-Z could not reach it — and it could also silently discard an
// unsaved buffer.
// ---------------------------------------------------------------------------

/**
 * Applies newText to the live editor buffer as one undoable change.
 * Returns true when something changed.
 */
async function applyBufferEdit(oldText, newText) {
  const edit = minimalEdit(oldText, newText);
  if (!edit) return false;
  await syscall("editor.replaceRange", edit.from, edit.to, edit.insert);
  return true;
}

/**
 * Redraws the still-open board from the document text a write just produced,
 * so a successful action does not feel like the board closed on you.
 */
async function rerenderBoard(sourceText, pageName) {
  const atoms = parseAtoms(sourceText);
  const { html, script } = buildBoardHtml(atoms, pageName);
  await syscall("editor.showPanel", "modal", 0, html, script);
}

/**
 * Rewrites one atom's directive line with a new attribute set, called from
 * the panel's Save button. Rewrites only that single source line — the
 * rest of the document is untouched.
 *
 * attrsJson: JSON-encoded array of {name, value}, as chosen by the user in
 * the panel. This function does not interpret any of those names; it only
 * (a) strips any "id" the caller may have slipped in, since id travels
 * separately and is never user-editable here, and (b) puts the real id
 * back as the first attribute, so an atom can never be written back
 * without one (SPEC.md "Identity").
 */
async function saveAttrs(atomId, attrsJson) {
  let requested;
  try {
    requested = JSON.parse(attrsJson);
  } catch (e) {
    return { ok: false, error: "Invalid attribute payload" };
  }
  if (!Array.isArray(requested)) {
    return { ok: false, error: "Invalid attribute payload" };
  }

  // The live buffer, not the file on disk: this write goes back through the
  // editor (see applyBufferEdit) so it is undoable, and the offsets it
  // computes must be offsets into the text the editor actually holds.
  const currentText = await syscall("editor.getText");

  const found = findAtomDirectiveLine(currentText, atomId);
  if (!found) {
    return {
      ok: false,
      error: "Could not find this atom's directive (implicit atom, or the document changed)",
    };
  }

  const cleaned = requested.filter((a) => a && a.name && a.name !== "id");
  const newAttrs = [{ name: "id", value: atomId }, ...cleaned];
  const newLine = serializeAtomLine(found.prefix, newAttrs, found.suffix);

  found.lines[found.lineIndex] = newLine;
  const newText = found.lines.join("\n");

  await applyBufferEdit(currentText, newText);

  return { ok: true };
}

/**
 * Moves one card's block (or, if it belongs to a group, the whole group —
 * see the "Drag-to-reorder" comment above computeUnits()) to a new position
 * in the source document, called from a card's drop handler in the panel
 * above.
 *
 * Re-reads the buffer fresh, same as saveAttrs(), rather than trusting
 * whatever the client last rendered — the document may have changed since
 * the board was opened. On success it applies the change through the editor
 * (one undoable transaction) and re-renders the still-open panel in place
 * with the new order, so the board does not need a separate "refresh" round
 * trip and does not close as a side effect of a successful drop.
 */
async function reorderAtom(movedUnitKey, targetUnitKey, placement) {
  const pageName = await syscall("editor.getCurrentPage");
  const currentText = await syscall("editor.getText");

  const result = reorderUnit(currentText, movedUnitKey, targetUnitKey, placement);
  if (!result.ok) return result;
  if (result.unchanged) return { ok: true, unchanged: true };

  await applyBufferEdit(currentText, result.text);
  await rerenderBoard(result.text, pageName);

  return { ok: true };
}

/**
 * Wraps the selected cards' units in one atom-group, called from the card
 * menu's Group item.
 *
 * unitKeysJson: JSON-encoded array of unit keys ("atom:<id>" or
 * "group:<id>"), which is what the panel's selection resolves to.
 *
 * The group gets a fresh eight-character Crockford Base32 id, generated the
 * same way atomdown's own `NewID` does (see newAtomdownId), and checked
 * against every id already in the document so it cannot collide.
 *
 * Only two lines are added — the two markers. No atom's text moves, so no
 * atom's `digest` can go stale, and no atom's directive line is rewritten,
 * so every `id` and every extension attribute is preserved byte for byte.
 * The contiguity and nesting rules are enforced here as well as in the menu,
 * because the document may have changed since the board was drawn.
 */
async function groupAtoms(unitKeysJson) {
  let unitKeys;
  try {
    unitKeys = JSON.parse(unitKeysJson);
  } catch (e) {
    return { ok: false, error: "Invalid selection payload" };
  }
  if (!Array.isArray(unitKeys)) {
    return { ok: false, error: "Invalid selection payload" };
  }

  const pageName = await syscall("editor.getCurrentPage");
  const currentText = await syscall("editor.getText");

  const used = existingIds(currentText);
  let groupId = newAtomdownId();
  for (let attempt = 0; attempt < 32 && used.indexOf(groupId) !== -1; attempt++) {
    groupId = newAtomdownId();
  }

  const result = insertGroupMarkers(currentText, unitKeys, groupId);
  if (!result.ok) return result;

  await applyBufferEdit(currentText, result.text);
  await rerenderBoard(result.text, pageName);

  return { ok: true, groupId };
}

/**
 * Removes one group's markers, called from the card menu's Ungroup item.
 * The two marker lines are the only bytes removed; every atom that was
 * inside the group keeps its position, its directive, its id and its digest.
 */
async function ungroupAtoms(groupId) {
  if (!groupId) return { ok: false, error: "No group id" };

  const pageName = await syscall("editor.getCurrentPage");
  const currentText = await syscall("editor.getText");

  const result = removeGroupMarkers(currentText, groupId);
  if (!result.ok) return result;

  await applyBufferEdit(currentText, result.text);
  await rerenderBoard(result.text, pageName);

  return { ok: true };
}

const functionMapping = {
  toggleBoard,
  notifyClosed,
  saveAttrs,
  reorderAtom,
  groupAtoms,
  ungroupAtoms,
};

const manifest = {
  name: "atomdown-board",
  version: 0.1,
  functions: {
    toggleBoard: {
      path: "./atomdown-board.js:toggleBoard",
      command: { name: "Atomdown: Toggle Board" },
    },
    notifyClosed: {
      path: "./atomdown-board.js:notifyClosed",
    },
    saveAttrs: {
      path: "./atomdown-board.js:saveAttrs",
    },
    reorderAtom: {
      path: "./atomdown-board.js:reorderAtom",
    },
    groupAtoms: {
      path: "./atomdown-board.js:groupAtoms",
    },
    ungroupAtoms: {
      path: "./atomdown-board.js:ungroupAtoms",
    },
  },
};

// Test-only surface. atomdown-board.test.mjs imports this to unit-test the
// pure decision functions directly, which is the seam whose absence let the
// "every drop lands at the end" bug ship. Nothing in SilverBullet reads it.
const internals = {
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
  removeLineCollapsingSeam,
  parseAtoms,
  injectSharedFunctions,
};

const plugExport = { manifest, functionMapping, internals };

wireWorker(functionMapping, manifest, self.postMessage);

export { plugExport as plug };
