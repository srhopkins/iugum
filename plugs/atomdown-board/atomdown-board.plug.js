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
  let currentGroupSlug = null;
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
      // slug is SPEC.md's optional readable alias ("The slug is not
      // identity"). It is lifted out of the generic attribute list here so
      // the board can label a card with it, but it is NOT removed from
      // `attrs`: the attribute editor still owns it like any other
      // attribute, which is what makes a rename a plain attribute save.
      const slugAttr = (attrs || []).find((a) => a.name === "slug");
      atoms.push({
        id,
        slug: slugAttr ? slugAttr.value : null,
        implicit,
        groupId: currentGroupId,
        groupSlug: currentGroupSlug,
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
      const groupSlugAttr = groupAttrs.find((a) => a.name === "slug");
      currentGroupId = groupIdAttr ? groupIdAttr.value : null;
      currentGroupSlug = groupSlugAttr ? groupSlugAttr.value : null;
      continue;
    }
    if (GROUP_CLOSE_RE.test(line)) {
      flush();
      currentGroupId = null;
      currentGroupSlug = null;
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
      const groupSlugAttr = groupAttrs.find((a) => a.name === "slug");
      const groupId = groupIdAttr ? groupIdAttr.value : null;
      const groupSlug = groupSlugAttr ? groupSlugAttr.value : null;
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
        groupSlug,
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
  const PREAMBLE_KEY = "\u0000preamble";
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

// ---------------------------------------------------------------------------
// Slugs: the readable alias, not identity.
//
// SPEC.md ("Identity") makes `slug` an optional readable alias on both `atom`
// and `atom-group`, and says outright that "the slug is not identity". So
// everything structural in this file still keys on `id` — unit keys, group
// lookups, the drop decision — and a slug only ever changes what a human
// reads. Steve's reason for wanting them (iugum-w6y.4) is that he cannot
// group by eight-character ids.
//
// The two functions below are deliberately the ONLY places this plug decides
// slug shape and slug collision. atomdown is growing a `materialize --slugs`
// generator and a duplicate-slug lint diagnostic; when those land, each of
// these becomes a one-line delegation to the binary's answer rather than a
// second opinion scattered through the plug.
// ---------------------------------------------------------------------------

/**
 * Sanitizes typed text into the shape atomdown generates: lowercase
 * kebab-case ASCII. Accented letters fold to their ASCII base ("Décisions"
 * -> "decisions") rather than being dropped; every other run of characters
 * outside [a-z0-9] becomes one hyphen, and leading, trailing and doubled
 * hyphens are removed.
 *
 * Returns "" when nothing usable survives, which every caller reads as
 * "no slug" — writing an empty slug attribute is never right.
 *
 * DELEGATION POINT: replace the body with the atomdown binary's own slug
 * generator once `materialize --slugs` exists. Keep the signature.
 */
function sanitizeSlug(input) {
  // Long enough for "plumbing-research" with room to spare, short enough that
  // a slug stays readable in a card header. Declared inside the function on
  // purpose: this function is injected into the panel script by source (see
  // CLIENT_SHARED_FUNCTIONS), so it must not depend on a module constant the
  // panel would not have.
  const SLUG_MAX_LENGTH = 48;
  let text = String(input == null ? "" : input);
  if (typeof text.normalize === "function") {
    // NFKD splits an accented letter into base + combining mark, so removing
    // the marks leaves the ASCII base letter.
    text = text.normalize("NFKD").replace(/[\u0300-\u036f]/g, "");
  }
  let slug = text
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "");
  if (slug.length > SLUG_MAX_LENGTH) {
    // Cut at a word boundary when there is one inside the limit, so a long
    // heading truncates to whole words rather than mid-word.
    const cut = slug.slice(0, SLUG_MAX_LENGTH);
    const atBoundary = cut.replace(/-[^-]*$/, "");
    slug = (atBoundary !== "" ? atBoundary : cut).replace(/-+$/, "");
  }
  return slug;
}

/**
 * Reports whether `slug` is already carried by some OTHER atom or group in
 * this document, by which ids, and as one ready-to-show sentence in
 * `warning` (null when there is no conflict), so every caller phrases a
 * duplicate the same way.
 *
 * This warns; it never blocks. Atomdown Core permits duplicate slugs — a slug
 * is not identity, so two blocks sharing one is legal, merely unhelpful. The
 * board therefore writes the slug the user typed and says what it noticed,
 * rather than refusing an edit the format allows.
 *
 * ownerId is the id of the atom or group being named, so renaming something
 * to the slug it already has is not a conflict with itself.
 *
 * DELEGATION POINT: replace the body with the atomdown binary's duplicate-slug
 * lint diagnostic once that lands. Keep the signature.
 */
function slugConflict(sourceText, slug, ownerId) {
  const wanted = String(slug || "");
  if (!wanted) return { duplicate: false, ids: [], warning: null };
  const ids = [];
  String(sourceText || "").split("\n").forEach(function (line) {
    const atomMatch = line.match(ATOM_TAG_RE);
    const groupMatch = line.match(GROUP_OPEN_RE);
    if (!atomMatch && !groupMatch) return;
    const attrs = parseAttrs(atomMatch ? atomMatch[2] : groupMatch[1]);
    const idAttr = attrs.find(function (a) { return a.name === "id"; });
    const slugAttr = attrs.find(function (a) { return a.name === "slug"; });
    if (!slugAttr || slugAttr.value !== wanted) return;
    const id = idAttr ? idAttr.value : "";
    if (ownerId && id === ownerId) return;
    if (ids.indexOf(id) === -1) ids.push(id);
  });
  return {
    duplicate: ids.length > 0,
    ids,
    warning: ids.length > 0
      ? 'The name "' + wanted + '" is already used in this page (' +
        ids.join(", ") + "). Atomdown permits that, and the name was written. " +
        "A name used once is easier to read."
      : null,
  };
}

/**
 * The default name to offer for a new group, derived from the blocks the user
 * selected so one confirm is enough.
 *
 * `texts` is each selected block's source text, in document order. The first
 * heading inside the selection wins — an ATX heading ("## Decisions") first,
 * then a setext heading, then the first non-blank line of any kind. That is
 * the line a reader would call the section's name.
 *
 * Falls back to "group", never to "", so the confirm button is never offered
 * with an empty field.
 */
function deriveGroupSlug(texts) {
  const lines = [];
  (texts || []).forEach(function (text) {
    String(text == null ? "" : text).split("\n").forEach(function (line) {
      lines.push(line);
    });
  });
  for (let i = 0; i < lines.length; i++) {
    const atx = lines[i].match(/^\s{0,3}#{1,6}\s+(.*?)\s*#*\s*$/);
    if (atx) {
      const slug = sanitizeSlug(atx[1]);
      if (slug) return slug;
    }
  }
  for (let i = 0; i + 1 < lines.length; i++) {
    if (lines[i].trim() === "") continue;
    if (/^\s{0,3}(=+|-{2,})\s*$/.test(lines[i + 1])) {
      const slug = sanitizeSlug(lines[i]);
      if (slug) return slug;
    }
  }
  for (let i = 0; i < lines.length; i++) {
    if (lines[i].trim() === "") continue;
    const slug = sanitizeSlug(lines[i]);
    if (slug) return slug;
  }
  return "group";
}

/**
 * What a human should read for one card or group: its slug when it has one,
 * its id when it does not. The id never stops being the identity — the board
 * keeps it visible next to the slug and in the tooltip (see buildBoardHtml),
 * because Steve needs it when citing an atom from another page.
 */
function slugOrId(slug, id) {
  const trimmed = String(slug == null ? "" : slug).trim();
  return trimmed !== "" ? trimmed : String(id == null ? "" : id);
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
 *
 * `slug` is the optional readable name the user typed. It is sanitized here
 * (sanitizeSlug) and written immediately after the id, which is the attribute
 * order `atomdown emit` itself uses for a group marker (emit.go: id, then
 * slug, then everything else) — so emitting the document afterwards does not
 * reshuffle the line. A slug already used elsewhere in the document is
 * written anyway and reported in `warning`; the format permits duplicates.
 */
function insertGroupMarkers(sourceText, unitKeys, groupId, slug) {
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

  const cleanSlug = sanitizeSlug(slug);
  const conflict = slugConflict(sourceText, cleanSlug, groupId);
  const marker = cleanSlug === ""
    ? '<!-- <atom-group id="' + groupId + '"> -->'
    : '<!-- <atom-group id="' + groupId + '" slug="' +
      escapeAttrValue(cleanSlug) + '"> -->';

  const first = units[positions[0]];
  const last = units[positions[positions.length - 1]];
  const out = lines.slice(0, first.startLine)
    .concat([marker])
    .concat(lines.slice(first.startLine, last.endLine + 1))
    .concat(["<!-- </atom-group> -->"])
    .concat(lines.slice(last.endLine + 1));
  return {
    ok: true,
    text: out.join("\n"),
    groupId,
    slug: cleanSlug,
    warning: conflict.warning,
  };
}

/**
 * Renames one group: rewrites its opening marker line and nothing else.
 *
 * The id stays exactly as it was — a slug is not identity (SPEC.md) — so the
 * group keeps every reference to it, and no atom inside it is touched, so no
 * `digest` can go stale. An empty slug removes the attribute rather than
 * writing slug="".
 *
 * Attribute order on the rewritten line is id, slug, then whatever else the
 * marker carried, matching emit.go so a later `atomdown emit` is a no-op.
 */
function setGroupSlugInSource(sourceText, groupId, slug) {
  const lines = String(sourceText || "").split("\n");
  let lineIndex = -1;
  let attrs = null;
  for (let i = 0; i < lines.length; i++) {
    const m = lines[i].match(GROUP_OPEN_RE);
    if (!m) continue;
    const parsed = parseAttrs(m[1]);
    const idAttr = parsed.find(function (a) { return a.name === "id"; });
    if (idAttr && idAttr.value === groupId) {
      lineIndex = i;
      attrs = parsed;
      break;
    }
  }
  if (lineIndex === -1) {
    return {
      ok: false,
      error: "Could not find that group (document changed since the board opened?)",
    };
  }

  const cleanSlug = sanitizeSlug(slug);
  const rest = attrs.filter(function (a) {
    return a.name !== "id" && a.name !== "slug";
  });
  const ordered = [{ name: "id", value: groupId }];
  if (cleanSlug !== "") ordered.push({ name: "slug", value: cleanSlug });
  const attrText = ordered.concat(rest)
    .map(function (a) { return a.name + '="' + escapeAttrValue(a.value) + '"'; })
    .join(" ");
  lines[lineIndex] = "<!-- <atom-group " + attrText + "> -->";

  const conflict = slugConflict(sourceText, cleanSlug, groupId);
  return {
    ok: true,
    text: lines.join("\n"),
    slug: cleanSlug,
    warning: conflict.warning,
  };
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

// ---------------------------------------------------------------------------
// RENDERED MARKDOWN
//
// A card shows its block RENDERED by default — a heading as a heading, a table
// as a table, a link as a link — because the board is supposed to read as the
// document it came from. Raw markdown is still one click away (see the
// Raw/Rendered toggle in buildBoardHtml's toolbar), but it is not the default.
//
// The rendering is SilverBullet's OWN markdown pipeline, reached through the
// `markdown.markdownToHtml` syscall (silverbullet client/plugos/syscalls/
// markdown.ts, registered for plugs in client/client_system.ts). That syscall
// parses with the same extended CommonMark grammar the editor uses and renders
// with client/markdown_renderer/markdown_render.ts, which is the same renderer
// upstream's own configuration-manager plug uses for library descriptions. So
// there is NO markdown library bundled into this file and none should ever be
// added: reusing the host's pipeline is what makes a card's table, task list
// and wiki link look like the editor's, and it follows the host's own syntax
// extensions for free.
//
// Escaping, precisely.
//   renderMarkdownToHtml builds a Tag tree and serializes it with renderHtml
//   (client/markdown_renderer/html_render.ts). Every text node and every
//   attribute VALUE goes through htmlEscape, so markdown text can never inject
//   markup. The one hole is deliberate on upstream's side: a raw HTML tag in
//   the markdown source is re-emitted verbatim as a RawHtml tag. That is the
//   whole reason sanitizeRenderedHtml exists below — the input to it is
//   already-escaped HTML plus whatever raw HTML the document happened to carry,
//   and it filters exactly that.
// ---------------------------------------------------------------------------

// Tags a card body may contain. Everything CommonMark and SilverBullet's
// extensions produce, and nothing that can run code, load a remote document,
// or take over the page.
const SAFE_TAGS = new Set([
  "p", "br", "hr", "span", "div",
  "h1", "h2", "h3", "h4", "h5", "h6",
  "strong", "em", "b", "i", "u", "s", "del", "ins", "mark", "small",
  "sub", "sup", "code", "pre", "kbd", "samp", "var", "abbr", "time",
  "a", "img",
  "ul", "ol", "li", "dl", "dt", "dd",
  "blockquote", "figure", "figcaption",
  "table", "thead", "tbody", "tfoot", "tr", "th", "td", "caption",
  "colgroup", "col",
  "input",
]);

// Tags with no closing tag of their own. Never pushed on the balance stack.
const VOID_TAGS = new Set([
  "br", "hr", "img", "input", "col", "wbr", "area", "base", "embed",
  "link", "meta", "param", "source", "track",
]);

// Tags that cannot directly contain one of their own peers. A second one
// closes the first, the way a browser's parser does, so raw HTML in the
// document that leaves list items or table cells open still renders as a flat
// list rather than as a chain of ever-deeper nesting.
const IMPLIED_CLOSE = {
  li: ["li"],
  dt: ["dt", "dd"],
  dd: ["dt", "dd"],
  p: ["p"],
  tr: ["tr"],
  td: ["td", "th"],
  th: ["td", "th"],
};

// A disallowed tag normally drops the TAG and keeps its text, because the text
// is content the user wrote. For these, the text is not content — it is code or
// a stylesheet — so the tag and everything up to its close tag go.
const STRIP_CONTENT_TAGS = new Set([
  "script", "style", "iframe", "object", "embed", "noscript", "template",
  "svg", "math", "applet", "frame", "frameset", "noembed", "xmp", "title",
]);

// Attributes a card body may carry. No `style` (a fixed-position overlay is a
// clickjack, and this panel is same-origin with the app), no `id` (it would
// collide with the panel's own element ids), and no `on*` of any kind — an
// allowlist means those need no separate rule.
const SAFE_ATTRS = new Set([
  "class", "href", "src", "alt", "title", "colspan", "rowspan", "start",
  "reversed", "type", "checked", "disabled", "dir", "lang", "width",
  "height", "align", "datetime", "cite",
]);

// URL schemes a card body's href or src may use. A relative URL (no scheme at
// all) is allowed too. Everything else — javascript:, data:, vbscript:, blob:,
// file: — is dropped along with the attribute.
const SAFE_URL_SCHEMES = new Set(["http", "https", "mailto", "tel", "ftp"]);

// The named entities a browser decodes inside an attribute value that could
// hide a scheme. Numeric entities are handled generically alongside these.
const URL_ENTITIES = {
  amp: "&", lt: "<", gt: ">", quot: '"', apos: "'", colon: ":",
  newline: "\n", tab: "\t", sol: "/", nbsp: " ",
};

/**
 * Decodes an attribute value the way a browser would before it resolves a URL,
 * so a scheme hidden as `&#106;avascript:` or `java&Tab;script:` cannot slip
 * past the scheme test. Used ONLY for the safety decision — the value written
 * back out is the original, untouched.
 */
function decodeUrlEntities(value) {
  return String(value).replace(
    /&(#[Xx]?[0-9A-Fa-f]+|[A-Za-z][A-Za-z0-9]*);?/g,
    (whole, body) => {
      if (body[0] === "#") {
        const hex = body[1] === "x" || body[1] === "X";
        const code = parseInt(hex ? body.slice(2) : body.slice(1), hex ? 16 : 10);
        if (!isFinite(code) || code < 0 || code > 0x10ffff) return whole;
        try {
          return String.fromCodePoint(code);
        } catch (e) {
          return whole;
        }
      }
      const named = URL_ENTITIES[body.toLowerCase()];
      return named === undefined ? whole : named;
    },
  );
}

/**
 * Drops every space and every control character, by code point rather than by
 * a regular expression, so this file stays plain ASCII with no control bytes
 * of its own. A browser's URL parser ignores exactly these before it reads a
 * scheme, which is why `java<tab>script:x` is a javascript: URL.
 */
function stripBlankAndControl(value) {
  let out = "";
  for (const ch of String(value)) {
    const code = ch.codePointAt(0);
    if (code <= 0x20 || code === 0x7f) continue;
    out += ch;
  }
  return out;
}

/** True when this href/src value is safe to keep. */
function isSafeUrl(value) {
  // Control characters and whitespace are ignored by a browser's URL parser,
  // so `java\nscript:x` is a javascript: URL. Strip them before testing.
  const bare = stripBlankAndControl(decodeUrlEntities(value));
  const scheme = bare.match(/^([A-Za-z][A-Za-z0-9+.\-]*):/);
  if (!scheme) return true; // relative, fragment, or query-only
  return SAFE_URL_SCHEMES.has(scheme[1].toLowerCase());
}

/**
 * Reads one tag's attributes out of the text between the tag name and its `>`.
 * Quote-aware, because a raw HTML tag copied from the document has NOT had its
 * attribute values escaped, so a `>` can legitimately sit inside a quoted
 * value.
 */
function readTagAttrs(text) {
  const out = [];
  const re = /([A-Za-z_:][-\w:.]*)(?:\s*=\s*("[^"]*"|'[^']*'|[^\s"'=<>`]+))?/g;
  let m;
  while ((m = re.exec(text)) !== null) {
    let value = m[2];
    if (value === undefined) {
      out.push({ name: m[1], value: null });
      continue;
    }
    if (value[0] === '"' || value[0] === "'") value = value.slice(1, -1);
    out.push({ name: m[1], value });
  }
  return out;
}

/**
 * Rebuilds one allowed tag with only its allowed attributes.
 *
 * An absolute link also gets target="_blank" and a rel that blocks
 * window.opener. The panel already calls preventDefault on a card link (see
 * the click handler in buildBoardHtml, which is what keeps a link from
 * hijacking a card-selecting click), so this never normally fires — it is the
 * fallback that keeps a click which escapes that handler from navigating the
 * panel iframe away and taking the board with it.
 */
function safeTagHtml(name, attrText, selfClosing) {
  const parts = [name];
  let href = null;
  for (const attr of readTagAttrs(attrText)) {
    const lower = attr.name.toLowerCase();
    if (!SAFE_ATTRS.has(lower)) continue;
    if (attr.value === null) {
      parts.push(lower);
      continue;
    }
    if ((lower === "href" || lower === "src") && !isSafeUrl(attr.value)) continue;
    if (lower === "href") href = attr.value;
    parts.push(`${lower}="${escapeAttrValue(attr.value)}"`);
  }
  if (name === "a" && href && /^[A-Za-z][A-Za-z0-9+.\-]*:/.test(href.trim())) {
    parts.push('target="_blank"', 'rel="noopener noreferrer"');
  }
  const close = selfClosing || VOID_TAGS.has(name) ? "/>" : ">";
  return `<${parts.join(" ")}${close}`;
}

/**
 * Filters one card body's rendered HTML down to a safe, BALANCED fragment.
 *
 * Two jobs, and the second one matters as much as the first:
 *
 *  1. Safety. Only SAFE_TAGS survive, only SAFE_ATTRS on them, only
 *     SAFE_URL_SCHEMES in an href or src. A disallowed tag is dropped but its
 *     text is kept, because that text is the user's content; a
 *     STRIP_CONTENT_TAGS tag takes its contents with it.
 *  2. Balance. Every close tag must match an open tag this function itself
 *     emitted, and anything still open at the end is closed here. A stray
 *     `</div>` in the document would otherwise close the CARD's own element,
 *     which would break the card rectangles pickDropTarget reads and put the
 *     rest of the board inside one card.
 *
 * The input contract: already-escaped HTML from markdown.markdownToHtml. Text
 * runs are passed through untouched rather than re-escaped, because escaping
 * them twice would show `&amp;` to the user. A `<` that does not begin a tag
 * IS escaped, so an unterminated or malformed tag degrades to visible text.
 */
function sanitizeRenderedHtml(html) {
  if (typeof html !== "string") return "";
  const out = [];
  const openStack = [];
  let i = 0;

  while (i < html.length) {
    const lt = html.indexOf("<", i);
    if (lt === -1) {
      out.push(html.slice(i));
      break;
    }
    if (lt > i) out.push(html.slice(i, lt));

    // A comment. Atomdown's own markers are comments, and a comment carries
    // nothing a card should show, so it goes entirely.
    if (html.startsWith("<!--", lt)) {
      const end = html.indexOf("-->", lt + 4);
      i = end === -1 ? html.length : end + 3;
      continue;
    }
    // A doctype, CDATA or processing instruction.
    if (html.startsWith("<!", lt) || html.startsWith("<?", lt)) {
      const end = html.indexOf(">", lt);
      i = end === -1 ? html.length : end + 1;
      continue;
    }

    const closing = html[lt + 1] === "/";
    const nameStart = lt + (closing ? 2 : 1);
    const nameMatch = html.slice(nameStart).match(/^[A-Za-z][A-Za-z0-9]*/);
    if (!nameMatch) {
      // Not a tag at all: a bare `<` in the text. Show it.
      out.push("&lt;");
      i = lt + 1;
      continue;
    }
    const name = nameMatch[0].toLowerCase();

    // Find this tag's `>`, skipping any inside a quoted attribute value.
    let j = nameStart + nameMatch[0].length;
    let quote = null;
    while (j < html.length) {
      const ch = html[j];
      if (quote) {
        if (ch === quote) quote = null;
      } else if (ch === '"' || ch === "'") {
        quote = ch;
      } else if (ch === ">") {
        break;
      }
      j++;
    }
    if (j >= html.length) {
      // Unterminated tag. Nothing after it can be trusted to be markup, so
      // show the rest as text rather than guess.
      out.push("&lt;", escapeHtml(html.slice(lt + 1)));
      i = html.length;
      break;
    }
    const inner = html.slice(nameStart + nameMatch[0].length, j);
    i = j + 1;

    if (closing) {
      if (!SAFE_TAGS.has(name)) continue;
      const at = openStack.lastIndexOf(name);
      if (at === -1) continue; // a stray close tag: drop it, keep the card intact
      while (openStack.length > at) out.push(`</${openStack.pop()}>`);
      continue;
    }

    if (STRIP_CONTENT_TAGS.has(name)) {
      // Skip to this tag's own close tag, or to the end of the input.
      const closeAt = html.toLowerCase().indexOf(`</${name}`, i);
      if (closeAt === -1) {
        i = html.length;
        break;
      }
      const closeEnd = html.indexOf(">", closeAt);
      i = closeEnd === -1 ? html.length : closeEnd + 1;
      continue;
    }

    if (!SAFE_TAGS.has(name)) continue; // drop the tag, keep the text inside it

    const peers = IMPLIED_CLOSE[name];
    while (
      peers && openStack.length &&
      peers.indexOf(openStack[openStack.length - 1]) !== -1
    ) {
      out.push(`</${openStack.pop()}>`);
    }

    const selfClosing = /\/\s*$/.test(inner);
    out.push(safeTagHtml(name, inner, selfClosing));
    if (!selfClosing && !VOID_TAGS.has(name)) openStack.push(name);
  }

  while (openStack.length) out.push(`</${openStack.pop()}>`);
  return out.join("");
}

/**
 * Renders every atom's block to safe HTML through the host's markdown
 * pipeline, returning a NEW atom list carrying a `renderedHtml` string.
 *
 * Failure is per card and never fatal: an older host with no
 * markdown.markdownToHtml syscall, or one block the renderer chokes on, leaves
 * `renderedHtml` null and that card falls back to its raw markdown (see
 * buildCardHtml). The board still draws.
 */
async function renderAtomBodies(atoms) {
  const out = [];
  for (const atom of atoms) {
    let renderedHtml = null;
    try {
      const html = await syscall("markdown.markdownToHtml", atom.text);
      if (typeof html === "string") renderedHtml = sanitizeRenderedHtml(html);
    } catch (e) {
      // No markdown syscall on this host, or this block would not render.
    }
    out.push(Object.assign({}, atom, { renderedHtml }));
  }
  return out;
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

/**
 * Which body a card shows: "rendered" (the default) or "raw".
 *
 * Two levels, and the board-wide one is the level Steve will actually use — he
 * wants to read the document, and only occasionally inspect one block's
 * syntax. So the toolbar carries the board-wide switch, and a card's three-dot
 * menu carries an override for that one card.
 *
 * RENDERED IS THE DEFAULT AT BOTH LEVELS. An absent boardView, an absent
 * override, and any value neither side recognises all mean rendered.
 *
 * Pure and self-contained: it is injected into the panel script, so the
 * toolbar, a card's menu and the initial markup cannot disagree about what a
 * card is showing.
 */
function effectiveCardView(atomId, viewState) {
  const state = viewState || {};
  const overrides = state.cardViews || {};
  const own = overrides[atomId];
  if (own === "raw" || own === "rendered") return own;
  return state.boardView === "raw" ? "raw" : "rendered";
}

// The pure decision functions the panel script needs. They are injected into
// that script by source (Function.prototype.toString) rather than duplicated,
// so the panel and the worker cannot disagree about where a drop lands or
// whether a selection can be grouped. Every function listed here must stay
// self-contained — see the PURE DECISION FUNCTIONS block above, and the
// same rule stated on sanitizeSlug in the Slugs block.
const CLIENT_SHARED_FUNCTIONS = [
  pickDropTarget,
  unitKeyForCard,
  unitOrderFromCards,
  dedupeKeys,
  isContiguousUnitSelection,
  groupMenuState,
  rectsIntersect,
  sanitizeSlug,
  deriveGroupSlug,
  effectiveCardView,
];

function injectSharedFunctions() {
  return CLIENT_SHARED_FUNCTIONS.map(function (fn) { return fn.toString(); })
    .join("\n\n");
}

/**
 * One card's HTML. A member of an atom-group gets NO group marking of its own
 * — no accent stripe, no `group <slug>` badge. The group container drawn
 * around it (see buildGroupHtml) is what says "these belong together", and a
 * per-card repeat of that reads as many objects rather than one.
 *
 * The one per-card treatment that survives inside a container is contrast: a
 * member card keeps the ordinary card background, and the container's own
 * field is the plainer surface behind it, so the cards read as the group's
 * contents. That is a background difference, not a second border, so it
 * cannot compete with either the container edge or the selection ring.
 *
 * THE BODY IS RENDERED COMMONMARK BY DEFAULT, and the raw markdown ships
 * alongside it in a hidden <pre>. Both are in the markup rather than one being
 * fetched on demand, for three reasons:
 *   - toggling is then instant and needs no round trip to the worker, so Raw
 *     and back is a CSS-level change, not a redraw;
 *   - a card the renderer could not handle (renderedHtml null) falls back to
 *     the raw body with no extra path;
 *   - the panel still has each block's EXACT original text, which is what
 *     deriveGroupSlug reads when it defaults a new group's name. Reading a
 *     rendered heading would have lost the `##` it looks for.
 */
function buildCardHtml(atom, viewState) {
  const view = effectiveCardView(atom.id, viewState);
  const hasRendered = typeof atom.renderedHtml === "string" &&
    atom.renderedHtml !== "";
  // A card with nothing to render shows raw, whatever the board is set to.
  const showRaw = view === "raw" || !hasRendered;
  const classes = ["board-card"];
  if (atom.implicit) classes.push("board-card-implicit");
  const badges = [];
  if (atom.implicit) {
    badges.push('<span class="board-badge board-badge-implicit">implicit</span>');
  }
  // Name first, identity second. A slug gets the readable name span; the id
  // stays on the line either way, in small subtle monospace, so it is never
  // more than a glance away.
  const nameHtml = atom.slug
    ? `<span class="board-card-slug" title="${escapeHtml(`Name (slug) "${atom.slug}" — the atom's id is ${atom.id}`)}">${escapeHtml(atom.slug)}</span>`
    : "";
  const idTitle = atom.implicit
    ? "This block has no directive yet, so it has no id of its own."
    : `Atom id ${atom.id} — this is the identity. A name (slug) is only an alias.`;
  return `
      <div class="${classes.join(" ")}" data-atom-id="${escapeHtml(atom.id)}" data-card-view="${showRaw ? "raw" : "rendered"}"${hasRendered ? "" : ' data-no-rendered="1"'}>
        <div class="board-card-header" draggable="true" data-drag-atom="${escapeHtml(atom.id)}" title="Drag to move${atom.groupId ? " (moves the whole group)" : ""}">
          <span class="board-drag-handle" aria-hidden="true">&#10021;&#10021;</span>
          ${nameHtml}
          <span class="board-card-id" title="${escapeHtml(idTitle)}">${escapeHtml(atom.id)}</span>
          ${badges.join("")}
          <div class="board-card-menu">
            <button type="button" class="board-menu-btn" data-menu-toggle="${escapeHtml(atom.id)}" title="Attributes" aria-haspopup="true">&#8942;</button>
            <div class="board-menu-popover" data-menu-popover="${escapeHtml(atom.id)}" hidden></div>
          </div>
        </div>
        <div class="board-card-body board-card-rendered" data-card-rendered="${escapeHtml(atom.id)}"${showRaw ? " hidden" : ""}>${hasRendered ? atom.renderedHtml : ""}</div>
        <pre class="board-card-body board-card-raw" data-card-raw="${escapeHtml(atom.id)}"${showRaw ? "" : " hidden"}>${escapeHtml(atom.text)}</pre>
      </div>`;
}

/**
 * One atom-group as ONE object: a single bordered container in
 * --ui-accent-color (the blue already used by the drop indicator and the
 * selection ring — never a second hue) with a header bar and the member cards
 * inside it.
 *
 * The header is where every group-level thing lives, because a group finally
 * has a surface of its own:
 *   - the NAME (the slug), with the real id beside it in small subtle
 *     monospace and in the tooltip, exactly the slug-then-id order a card
 *     already uses. No slug means the id IS the label, same fallback as a card.
 *   - Rename and Ungroup. Those used to sit in a member card's menu purely
 *     because a group had no UI of its own; that workaround is gone.
 *   - a collapse toggle, and a drag handle so a collapsed group is still
 *     movable.
 *
 * Nothing here is written to the document. The container, the header, the
 * collapse state and the selection are all presentation. `collapsed` arrives
 * from the client-local key-value store (see loadCollapsedGroups) and is
 * rendered into the markup rather than applied by the panel script afterwards,
 * so a collapsed group never flashes open on the way in.
 */
function buildGroupHtml(groupId, groupSlug, members, collapsed, viewState) {
  const nameHtml = groupSlug
    ? `<span class="board-group-name" title="${escapeHtml(`Name (slug) "${groupSlug}" — the group's id is ${groupId}`)}">${escapeHtml(groupSlug)}</span>`
    : "";
  const idTitle = groupSlug
    ? `Group id ${groupId} — this is the identity. A name (slug) is only an alias.`
    : `Group id ${groupId} (no name yet) — click Rename to give it one.`;
  const headerTitle = groupSlug
    ? `Group "${groupSlug}" (id ${groupId}) — click to select the whole group`
    : `Group ${groupId} — click to select the whole group`;
  const count = members.length === 1 ? "1 card" : members.length + " cards";
  const isCollapsed = collapsed === true;
  return `
      <div class="board-group${isCollapsed ? " board-group-collapsed" : ""}" data-group-id="${escapeHtml(groupId)}">
        <div class="board-group-header" data-group-header="${escapeHtml(groupId)}" title="${escapeHtml(headerTitle)}">
          <button type="button" class="board-group-collapse" data-group-collapse="${escapeHtml(groupId)}" aria-expanded="${isCollapsed ? "false" : "true"}" title="${isCollapsed ? "Expand this group" : "Collapse this group"}">${isCollapsed ? "&#9656;" : "&#9662;"}</button>
          <span class="board-drag-handle board-group-drag" draggable="true" data-drag-unit="group:${escapeHtml(groupId)}" title="Drag to move the whole group">&#10021;&#10021;</span>
          <span class="board-group-kind">group</span>
          ${nameHtml}
          <span class="board-group-id" title="${escapeHtml(idTitle)}">${escapeHtml(groupId)}</span>
          <span class="board-group-count">${count}</span>
          <div class="board-group-actions">
            <button type="button" class="board-group-btn" data-group-rename="${escapeHtml(groupId)}" title="Give this group a readable name. Its id (${escapeHtml(groupId)}) does not change - a name is an alias, not the identity.">Rename</button>
            <button type="button" class="board-group-btn" data-group-ungroup="${escapeHtml(groupId)}" title="Remove this group's markers. Every atom inside it stays.">Ungroup</button>
          </div>
        </div>
        <div class="board-group-cards" data-group-cards="${escapeHtml(groupId)}"${isCollapsed ? " hidden" : ""}>${members.map((m) => buildCardHtml(m, viewState)).join("\n")}</div>
      </div>`;
}

/**
 * Walks the atom list into the panel's top-level strip: a standalone atom
 * renders as a bare card, and a run of consecutive atoms sharing one groupId
 * renders as one group container holding those cards.
 *
 * The cards stay in document order in the DOM, nested or not, which is what
 * keeps the drop geometry, the lasso and the unit order working unchanged —
 * every one of those reads `.board-card[data-atom-id]` in document order.
 */
function buildStripHtml(atoms, collapsedIds, viewState) {
  const collapsed = collapsedIds || [];
  const parts = [];
  let i = 0;
  while (i < atoms.length) {
    if (!atoms[i].groupId) {
      parts.push(buildCardHtml(atoms[i], viewState));
      i++;
      continue;
    }
    const groupId = atoms[i].groupId;
    const groupSlug = atoms[i].groupSlug;
    const members = [];
    while (i < atoms.length && atoms[i].groupId === groupId) {
      members.push(atoms[i]);
      i++;
    }
    parts.push(
      buildGroupHtml(
        groupId,
        groupSlug,
        members,
        collapsed.indexOf(groupId) !== -1,
        viewState,
      ),
    );
  }
  return parts.join("\n");
}

function buildBoardHtml(atoms, pageName, collapsedIds, viewState) {
  const collapsed = Array.isArray(collapsedIds) ? collapsedIds : [];
  const view = {
    boardView: viewState && viewState.boardView === "raw" ? "raw" : "rendered",
    cardViews: (viewState && viewState.cardViews) || {},
  };
  const cardsHtml = buildStripHtml(atoms, collapsed, view);

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
    /* Spacing between top-level items, whichever kind they are. Scoped to
       direct children so the tighter spacing inside a group container (below)
       does not have to fight it. */
    .board-cards > .board-card + .board-card,
    .board-cards > .board-card + .board-group,
    .board-cards > .board-group + .board-card,
    .board-cards > .board-group + .board-group { margin-top: 14px; }
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
    /* One group, one object.
       ------------------------------------------------------------------
       The members of an atom-group sit inside a single container outlined in
       --ui-accent-color — the SAME token as the drop indicator and the
       selection ring, so the board still has exactly one blue. The container
       is the group's identity on screen, which is why a member card carries
       no accent stripe and no group badge any more.
       The container's own field is the plain surface, and a member card keeps
       the slightly tinted card background, so the cards read as contents ON
       the group rather than as siblings of it. That is the only per-card
       treatment left, and being a background it cannot be mistaken for
       either the container edge or a selection.
       Deliberately NO overflow:hidden here: a member card's attribute
       popover is absolutely positioned inside the card, and clipping it
       would hide the menu for every grouped card. The header rounds its own
       top corners instead. */
    .board-group {
      border: 2px solid var(--ui-accent-color);
      border-radius: 6px;
      background: var(--ui-surface-background-color);
    }
    .board-group-header {
      display: flex;
      align-items: center;
      gap: 6px;
      flex-wrap: wrap;
      padding: 5px 8px;
      background: var(--ui-accent-color);
      color: var(--ui-accent-contrast-color);
      border-top-left-radius: 4px;
      border-top-right-radius: 4px;
      cursor: pointer;
      user-select: none;
    }
    /* Name-then-identity, the same order and the same weights a card uses. */
    .board-group-kind {
      font-size: 10px;
      text-transform: uppercase;
      letter-spacing: 0.06em;
      opacity: 0.75;
    }
    .board-group-name { font-size: 13px; font-weight: 600; }
    .board-group-id {
      font-family: ui-monospace, monospace;
      font-size: 11px;
      opacity: 0.8;
    }
    .board-group-count { font-size: 11px; opacity: 0.8; }
    .board-group-actions { display: flex; gap: 6px; margin-left: auto; }
    .board-group-btn, .board-group-collapse {
      cursor: pointer;
      font-size: 11px;
      line-height: 1.2;
      padding: 2px 8px;
      border-radius: 4px;
      border: 1px solid currentColor;
      background: transparent;
      color: inherit;
    }
    /* Hover inverts the two accent tokens rather than mixing in a new value. */
    .board-group-btn:hover, .board-group-collapse:hover {
      background: var(--ui-accent-contrast-color);
      color: var(--ui-accent-color);
    }
    .board-group-collapse {
      border-color: transparent;
      font-size: 12px;
      padding: 1px 5px;
    }
    .board-group-cards { padding: 8px; }
    .board-group-cards[hidden] { display: none; }
    .board-group-cards .board-card + .board-card { margin-top: 8px; }
    /* The header's rename form: same inputs and the same classes as the
       naming form in a card's popover, so there is one look for naming. */
    .board-group-rename {
      padding: 8px;
      border-bottom: 1px solid var(--ui-surface-border-color);
    }
    .board-group-rename[hidden] { display: none; }
    /* Drag state, applied by the client script below. board-card-dragging
       marks every card sharing the dragged unit (a whole group drags
       together, see computeUnits() in the worker code); dropbefore/after
       mark the card the pointer is currently hovering, to show where the
       drop would land. */
    .board-card-dragging { opacity: 0.4; }
    .board-card-dropbefore { box-shadow: inset 0 3px 0 0 var(--ui-accent-color); }
    .board-card-dropafter { box-shadow: inset 0 -3px 0 0 var(--ui-accent-color); }
    /* Selection. The colour is the SAME blue as the drop indicator two lines
       up and as the group container's edge: --ui-accent-color, SilverBullet's
       own accent token, copied live from the parent document by
       applyParentTheme(). Selection must not introduce a second blue.
       So it separates itself from the container by SHAPE, not by hue: a
       DOUBLE ring (the card's own 2px border, then a second 2px ring set 2px
       outside it, with the container's field showing through the gap) plus a
       lifted background. A group container is a single continuous 2px edge;
       a selected card is a banded edge with a gap in it. The two cannot be
       read as the same thing even when a selected card sits right against
       the container's own border.
       An outline on purpose rather than a second box-shadow: box-shadow is
       already spoken for by the drop indicator, and outline takes no space,
       so a selection never nudges the layout the drop geometry just read. */
    .board-card-selected {
      border: 2px solid var(--ui-accent-color);
      outline: 2px solid var(--ui-accent-color);
      outline-offset: 2px;
      background: var(--ui-surface-hover-background-color);
    }
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
    /* The readable name (slug) is the primary label: normal body font, full
       contrast, first on the line. The id keeps the small subtle monospace
       treatment it always had, so name-then-identity reads in that order at a
       glance without the id ever disappearing. */
    .board-card-slug {
      font-size: 13px;
      font-weight: 600;
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
    .board-toolbar-actions { display: flex; gap: 8px; align-items: center; }
    .board-card-body {
      margin: 0;
      padding: 8px;
      word-break: break-word;
      flex: 1;
    }
    .board-card-body[hidden] { display: none; }
    /* RAW markdown. Monospace and pre-wrap, exactly as before, because the
       point of this view is to see the source byte for byte. */
    .board-card-raw {
      font-family: ui-monospace, monospace;
      font-size: 12px;
      white-space: pre-wrap;
    }
    /* RENDERED CommonMark — the default.
       ------------------------------------------------------------------
       The rule here is "look like the document, not like a widget", so this
       block sets structure and rhythm and takes every colour and the body
       font from the parent theme's own tokens (applyParentTheme copies them
       in live). There is no palette of its own.
       First and last child margins are collapsed so a card whose block is a
       single heading or paragraph is not padded twice. */
    .board-card-rendered {
      font-size: 14px;
      line-height: 1.5;
    }
    .board-card-rendered > :first-child { margin-top: 0; }
    .board-card-rendered > :last-child { margin-bottom: 0; }
    .board-card-rendered p { margin: 0.5em 0; }
    .board-card-rendered h1,
    .board-card-rendered h2,
    .board-card-rendered h3,
    .board-card-rendered h4,
    .board-card-rendered h5,
    .board-card-rendered h6 {
      margin: 0.4em 0 0.3em;
      line-height: 1.25;
      font-weight: 600;
    }
    /* A card is a block, not a page, so a heading inside one is sized by its
       LEVEL relative to the card rather than at document scale — a document
       h1 at 2em would dwarf the card it sits in. */
    .board-card-rendered h1 { font-size: 1.5em; }
    .board-card-rendered h2 { font-size: 1.3em; }
    .board-card-rendered h3 { font-size: 1.15em; }
    .board-card-rendered h4,
    .board-card-rendered h5,
    .board-card-rendered h6 { font-size: 1em; }
    .board-card-rendered ul,
    .board-card-rendered ol { margin: 0.4em 0; padding-left: 1.5em; }
    .board-card-rendered li { margin: 0.15em 0; }
    .board-card-rendered a {
      color: var(--link-color);
      text-decoration: underline;
      text-underline-offset: 2px;
    }
    .board-card-rendered code {
      font-family: ui-monospace, monospace;
      font-size: 0.88em;
      padding: 1px 4px;
      border-radius: 3px;
      background: var(--subtle-background-color);
    }
    .board-card-rendered pre {
      margin: 0.5em 0;
      padding: 8px;
      border-radius: 4px;
      background: var(--subtle-background-color);
      overflow-x: auto;
    }
    .board-card-rendered pre code { background: none; padding: 0; }
    .board-card-rendered blockquote {
      margin: 0.5em 0;
      padding: 0 0 0 10px;
      border-left: 3px solid var(--ui-surface-border-color);
      color: var(--subtle-color);
    }
    .board-card-rendered hr {
      border: none;
      border-top: 1px solid var(--ui-surface-border-color);
      margin: 0.7em 0;
    }
    .board-card-rendered img { max-width: 100%; height: auto; }
    /* A TABLE must not widen the card.
       ------------------------------------------------------------------
       pickDropTarget reads each card's own rectangle, so a card that grows
       wider than the column would change the geometry a drop is decided
       from. The table scrolls INSIDE its own wrapper instead, which leaves
       the card's box exactly where the flex column put it. */
    .board-card-rendered table {
      border-collapse: collapse;
      margin: 0.5em 0;
      font-size: 0.92em;
      display: block;
      max-width: 100%;
      overflow-x: auto;
    }
    .board-card-rendered th,
    .board-card-rendered td {
      border: 1px solid var(--ui-surface-border-color);
      padding: 3px 7px;
      text-align: left;
      vertical-align: top;
    }
    .board-card-rendered th {
      background: var(--ui-surface-hover-background-color);
      font-weight: 600;
    }
    /* A rendered task-list checkbox is a picture of the document's state, not
       a control: this board never writes a byte from a card body, so letting
       it be clicked would promise an edit that cannot happen. */
    .board-card-rendered input[type="checkbox"] {
      pointer-events: none;
      margin-right: 4px;
    }
    /* Selecting a card is a click gesture, so text selection inside a card
       body would fight it. The RAW view keeps text selectable, because
       copying the source is the reason to open it. */
    .board-card-rendered { user-select: none; }
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
    /* Naming a group. This is a form INSIDE the existing popover, not a new
       modal and not window.prompt: window.prompt is blocked in a sandboxed
       iframe in some browsers and looks like a browser dialog rather than
       part of the app, and the popover already holds inputs (the attribute
       editor), so this reuses that pattern. */
    .board-slug-form[hidden] { display: none; }
    .board-slug-label {
      display: block;
      font-size: 11px;
      color: var(--subtle-color);
      margin-bottom: 3px;
    }
    .board-slug-input {
      width: 100%;
      box-sizing: border-box;
      font-size: 12px;
      padding: 3px 5px;
      background: var(--ui-surface-section-background-color);
      border: 1px solid var(--ui-surface-border-color);
      color: inherit;
      border-radius: 3px;
      font-family: ui-monospace, monospace;
    }
    .board-slug-hint {
      font-size: 11px;
      margin-top: 4px;
      color: var(--subtle-color);
      min-height: 14px;
    }
    /* The slug row of the attribute editor. It is the first row and it is
       labelled, because it is the only attribute a human reads. */
    .board-attr-slug { margin-bottom: 8px; }
  `;

  const html = `
    <style>${style}</style>
    <div class="board-toolbar">
      <div class="board-title">Atomdown Board${pageName ? " — " + escapeHtml(pageName) : ""}</div>
      <div class="board-toolbar-actions">
        <button class="board-close" id="atomdown-board-view" data-board-view="${view.boardView}" title="${view.boardView === "raw" ? "Show every card as rendered CommonMark" : "Show every card's raw markdown source"}">${view.boardView === "raw" ? "Rendered" : "Raw markdown"}</button>
        <button class="board-close" id="atomdown-board-close">Close</button>
      </div>
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

    // Every readable name already in this page, mapped to the ids that carry
    // it, so a naming form can warn about a duplicate as the user types. The
    // worker checks the live buffer again before it writes; this copy is only
    // for the hint, and neither check ever blocks the write.
    var KNOWN_SLUGS = {};
    ATOMDOWN_BOARD_DATA.forEach(function (a) {
      if (a.slug) {
        KNOWN_SLUGS[a.slug] = KNOWN_SLUGS[a.slug] || [];
        if (KNOWN_SLUGS[a.slug].indexOf(a.id) === -1) KNOWN_SLUGS[a.slug].push(a.id);
      }
      if (a.groupSlug && a.groupId) {
        KNOWN_SLUGS[a.groupSlug] = KNOWN_SLUGS[a.groupSlug] || [];
        if (KNOWN_SLUGS[a.groupSlug].indexOf(a.groupId) === -1) {
          KNOWN_SLUGS[a.groupSlug].push(a.groupId);
        }
      }
    });

    // --- Rendered or raw ------------------------------------------------
    //
    // Both bodies are already in the DOM (see buildCardHtml), so switching is
    // a hidden flag, not a redraw and not a round trip to the worker. Nothing
    // here touches the document: which body a card shows is presentation, the
    // same class of state as a collapsed group, and it lives in the same
    // client-local key-value store.
    //
    // The board-wide switch is the master. Flipping it CLEARS every per-card
    // override, so "show me the whole document as markdown" means the whole
    // document and not "the whole document except the four cards I poked".
    var VIEW = {
      boardView: (ATOMDOWN_BOARD_VIEW && ATOMDOWN_BOARD_VIEW.boardView) === "raw"
        ? "raw"
        : "rendered",
      cardViews: (ATOMDOWN_BOARD_VIEW && ATOMDOWN_BOARD_VIEW.cardViews) || {},
    };

    async function persistView() {
      try {
        await syscall(
          "clientStore.set",
          "atomdown-board.view:" + ATOMDOWN_BOARD_PAGE,
          { boardView: VIEW.boardView, cardViews: VIEW.cardViews },
        );
      } catch (e) {
        // No store (a stub host, private browsing, an older SilverBullet). The
        // view applied for this session; only remembering it failed, and that
        // is not worth an error in the user's face.
      }
    }

    // Applies one card's effective view to its two bodies. A card whose block
    // the renderer could not handle carries data-no-rendered and stays raw
    // whatever the board is set to — there is nothing else to show it.
    function applyCardView(cardEl) {
      var atomId = cardEl.getAttribute("data-atom-id");
      var rendered = cardEl.querySelector("[data-card-rendered]");
      var raw = cardEl.querySelector("[data-card-raw]");
      var view = effectiveCardView(atomId, VIEW);
      if (cardEl.getAttribute("data-no-rendered") === "1") view = "raw";
      if (rendered) rendered.hidden = view !== "rendered";
      if (raw) raw.hidden = view !== "raw";
      cardEl.setAttribute("data-card-view", view);
      return view;
    }

    function applyEveryCardView() {
      CARD_ELS.forEach(applyCardView);
    }

    var viewBtn = document.getElementById("atomdown-board-view");

    function refreshViewButton() {
      if (!viewBtn) return;
      var raw = VIEW.boardView === "raw";
      viewBtn.textContent = raw ? "Rendered" : "Raw markdown";
      viewBtn.title = raw
        ? "Show every card as rendered CommonMark"
        : "Show every card's raw markdown source";
      viewBtn.setAttribute("data-board-view", VIEW.boardView);
    }

    if (viewBtn) {
      viewBtn.addEventListener("click", function (e) {
        e.stopPropagation();
        VIEW.boardView = VIEW.boardView === "raw" ? "rendered" : "raw";
        VIEW.cardViews = {};
        applyEveryCardView();
        refreshViewButton();
        persistView();
      });
    }

    // Sets, or clears, ONE card's override. Setting it to the board-wide value
    // clears the override rather than storing a redundant copy, so a later flip
    // of the board switch still moves that card.
    function setCardOverride(atomId, view) {
      if (view === VIEW.boardView) delete VIEW.cardViews[atomId];
      else VIEW.cardViews[atomId] = view;
      var cardEl = document.querySelector(
        '.board-card[data-atom-id="' + atomId + '"]',
      );
      if (cardEl) applyCardView(cardEl);
      persistView();
    }

    // A rendered link is a real <a> with a real href, so it looks and hovers
    // like the link in the document. It must not NAVIGATE, though: this panel
    // is the board, and following a link inside it would replace the board with
    // the target page. So a click on a card link is swallowed here and falls
    // through to the card's own selection handler — selection wins, which is
    // the gesture the click was for. The sanitizer also puts target="_blank"
    // on an absolute link, so a click that somehow escapes this opens a tab
    // instead of destroying the board.
    document.addEventListener("click", function (e) {
      var anchor = e.target && e.target.closest && e.target.closest("a");
      if (anchor && anchor.closest(".board-card-rendered")) e.preventDefault();
    }, true);

    // --- Group containers ----------------------------------------------
    //
    // A group is one object on screen, so the header bar is where its own
    // actions live and clicking it selects the whole thing. Everything in
    // this block is presentation: not one line of it reaches the document
    // except through setGroupSlug (Rename) and ungroupAtoms (Ungroup), which
    // are the two content changes the user asked for by name.
    var GROUP_ELS = Array.prototype.slice.call(
      document.querySelectorAll(".board-group[data-group-id]"),
    );

    function memberCardsOf(groupEl) {
      return Array.prototype.slice.call(
        groupEl.querySelectorAll(".board-card[data-atom-id]"),
      );
    }

    function isCollapsedGroup(groupEl) {
      return groupEl.classList.contains("board-group-collapsed");
    }

    // Clicking the header selects every member card. That is the whole
    // integration with grouping: the selection is still a set of cards, so
    // selectedUnitKeys() collapses them to the one "group:<id>" unit and the
    // existing Group / Ungroup decision (groupMenuState) applies with no
    // special case — Ungroup acts on the group, and Group stays disabled
    // because Atomdown Core 1 permits no group inside a group.
    function selectGroup(groupEl, additive) {
      if (!additive) clearSelection();
      var members = memberCardsOf(groupEl);
      members.forEach(function (card) {
        card.classList.add("board-card-selected");
      });
      if (members.length) {
        var first = CARD_ELS.indexOf(members[0]);
        if (first >= 0) selectionAnchor = first;
      }
    }

    // Collapse / expand.
    //
    // THIS IS PRESENTATION STATE AND IS NEVER WRITTEN TO THE DOCUMENT. There
    // is no "collapsed" attribute on any directive and there never will be:
    // Atomdown carries no layout, position, card or board metadata. It lives
    // in SilverBullet's own client-local key-value store (clientStore, which
    // is per-browser, not per-file), under a key scoped to this page, so a
    // long page stays scanned the way it was left without the file changing
    // by one byte.
    var COLLAPSED = {};
    (ATOMDOWN_BOARD_COLLAPSED || []).forEach(function (id) { COLLAPSED[id] = true; });

    function collapsedIdList() {
      return Object.keys(COLLAPSED).filter(function (id) { return COLLAPSED[id]; });
    }

    async function persistCollapsed() {
      try {
        await syscall(
          "clientStore.set",
          "atomdown-board.collapsed:" + ATOMDOWN_BOARD_PAGE,
          collapsedIdList(),
        );
      } catch (e) {
        // No store (a stub host, private browsing, an older SilverBullet).
        // The collapse still applied for this session; only remembering it
        // failed, and that is not worth an error in the user's face.
      }
    }

    function setCollapsed(groupEl, collapsed) {
      var groupId = groupEl.getAttribute("data-group-id");
      var cards = groupEl.querySelector("[data-group-cards]");
      var btn = groupEl.querySelector("[data-group-collapse]");
      if (cards) cards.hidden = collapsed;
      if (collapsed) groupEl.classList.add("board-group-collapsed");
      else groupEl.classList.remove("board-group-collapsed");
      if (btn) {
        btn.innerHTML = collapsed ? "\\u25B8" : "\\u25BE";
        btn.setAttribute("aria-expanded", collapsed ? "false" : "true");
        btn.title = collapsed ? "Expand this group" : "Collapse this group";
      }
      COLLAPSED[groupId] = collapsed;
      persistCollapsed();
    }

    // The header's Rename form. Same classes, same duplicate-name hint and the
    // same "sanitize, then write anyway" rule as the naming form in a card's
    // popover — one look for naming, in the one place a group now owns.
    function buildGroupRenameForm(groupId, currentSlug) {
      var form = el("div", "board-group-rename");
      form.hidden = true;
      var label = el("label", "board-slug-label");
      label.textContent = "Name for this group (id " + groupId + ")";
      var input = el("input", "board-slug-input");
      input.type = "text";
      input.setAttribute("spellcheck", "false");
      input.placeholder = "unnamed - the header shows " + groupId;
      var actions = el("div", "board-menu-actions");
      var confirmBtn = el("button", "board-attr-save");
      confirmBtn.type = "button";
      confirmBtn.textContent = "Rename";
      var cancelBtn = el("button", "board-attr-add");
      cancelBtn.type = "button";
      cancelBtn.textContent = "Cancel";
      actions.appendChild(confirmBtn);
      actions.appendChild(cancelBtn);
      var hint = el("div", "board-slug-hint");
      form.appendChild(label);
      form.appendChild(input);
      form.appendChild(actions);
      form.appendChild(hint);

      function updateHint() {
        var clean = sanitizeSlug(input.value);
        if (!clean) {
          hint.textContent = "No name. The group will show its id instead.";
          return;
        }
        var owners = (KNOWN_SLUGS[clean] || []).filter(function (id) {
          return id !== groupId;
        });
        hint.textContent = 'Writes slug="' + clean + '".' +
          (owners.length
            ? " Already used by " + owners.join(", ") +
              " - allowed, just harder to read."
            : "");
      }

      function close() { form.hidden = true; }

      function open() {
        input.value = currentSlug || "";
        confirmBtn.disabled = false;
        confirmBtn.textContent = "Rename";
        form.hidden = false;
        updateHint();
        input.focus();
        input.select();
      }

      input.addEventListener("input", updateHint);
      input.addEventListener("keydown", function (e) {
        e.stopPropagation();
        if (e.key === "Enter") { e.preventDefault(); confirmBtn.click(); }
        if (e.key === "Escape") { e.preventDefault(); close(); }
      });
      cancelBtn.addEventListener("click", function (e) {
        e.stopPropagation();
        close();
      });
      confirmBtn.addEventListener("click", async function (e) {
        e.stopPropagation();
        confirmBtn.disabled = true;
        confirmBtn.textContent = "Renaming...";
        try {
          var result = await syscall(
            "system.invokeFunction",
            "atomdown-board.setGroupSlug",
            groupId,
            sanitizeSlug(input.value),
          );
          if (!result || !result.ok) {
            hint.textContent = "Failed: " +
              ((result && result.error) || "unknown error");
            confirmBtn.disabled = false;
            confirmBtn.textContent = "Rename";
            return;
          }
          // On success the worker redraws the whole panel, so this form is
          // already gone.
        } catch (err) {
          hint.textContent = "Failed: " + err.message;
          confirmBtn.disabled = false;
          confirmBtn.textContent = "Rename";
        }
      });

      return { form: form, open: open, close: close };
    }

    GROUP_ELS.forEach(function (groupEl) {
      var groupId = groupEl.getAttribute("data-group-id");
      var header = groupEl.querySelector("[data-group-header]");
      if (!header) return;
      var firstMember = memberCardsOf(groupEl)[0];
      var datum = firstMember ? cardDatum(firstMember) : null;
      var rename = buildGroupRenameForm(groupId, datum ? datum.groupSlug : null);
      groupEl.insertBefore(rename.form, header.nextSibling);

      header.addEventListener("click", function (e) {
        // The header's own buttons have their own handlers; a click on one of
        // them is not a selection gesture.
        if (e.target.closest && e.target.closest("button")) return;
        e.stopPropagation();
        closeAllPopovers(null);
        selectGroup(groupEl, e.metaKey || e.ctrlKey || e.shiftKey);
      });

      var collapseBtn = groupEl.querySelector("[data-group-collapse]");
      if (collapseBtn) {
        collapseBtn.addEventListener("click", function (e) {
          e.stopPropagation();
          setCollapsed(groupEl, !isCollapsedGroup(groupEl));
        });
      }

      var renameBtn = groupEl.querySelector("[data-group-rename]");
      if (renameBtn) {
        renameBtn.addEventListener("click", function (e) {
          e.stopPropagation();
          closeAllPopovers(null);
          if (rename.form.hidden) rename.open();
          else rename.close();
        });
      }

      var ungroupBtn = groupEl.querySelector("[data-group-ungroup]");
      if (ungroupBtn) {
        ungroupBtn.addEventListener("click", async function (e) {
          e.stopPropagation();
          ungroupBtn.disabled = true;
          ungroupBtn.textContent = "Ungrouping...";
          try {
            var result = await syscall(
              "system.invokeFunction",
              "atomdown-board.ungroupAtoms",
              groupId,
            );
            if (!result || !result.ok) {
              window.alert(
                "Ungroup failed: " + ((result && result.error) || "unknown error"),
              );
              ungroupBtn.disabled = false;
              ungroupBtn.textContent = "Ungroup";
            }
            // On success the worker redraws the panel, so this button is gone.
          } catch (err) {
            window.alert("Ungroup failed: " + err.message);
            ungroupBtn.disabled = false;
            ungroupBtn.textContent = "Ungroup";
          }
        });
      }
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
      // Never on a card, on the toolbar, or on a group's own header or its
      // rename form — those are controls, and dragging out of one should not
      // start a rubber band.
      if (e.target.closest && (e.target.closest(".board-card") ||
        e.target.closest(".board-toolbar") ||
        e.target.closest(".board-group-header") ||
        e.target.closest(".board-group-rename"))) return;
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
      // A collapsed group's cards have no rectangle to hit, so the band is
      // tested against the container instead. Dragging over a collapsed group
      // still selects it, which is what the user drew the band around.
      GROUP_ELS.forEach(function (groupEl) {
        if (!isCollapsedGroup(groupEl)) return;
        if (rectsIntersect(groupEl.getBoundingClientRect(), band)) {
          selectGroup(groupEl, true);
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

    // The selected blocks' own text, in document order, which is what
    // deriveGroupSlug() reads to find the first heading. Taken from the
    // rendered card bodies rather than shipped in ATOMDOWN_BOARD_DATA, so the
    // panel payload does not carry a second copy of the whole page.
    // Deliberately the RAW body, never the rendered one. deriveGroupSlug looks
    // for the first markdown heading, and a rendered heading no longer carries
    // the "##" it matches on — so reading the rendered node would silently
    // stop defaulting a new group's name.
    function selectedCardTexts() {
      return selectedCards().map(function (card) {
        var body = card.querySelector(".board-card-raw");
        return body ? body.textContent : "";
      });
    }

    function groupMenuStateFor(atom) {
      return groupMenuState(UNIT_ORDER, selectedUnitKeys(), unitKeyForCard(atom));
    }

    function refreshGroupItem(atom, popoverEl) {
      var btn = popoverEl.boardGroupBtn;
      if (!btn) return;
      if (popoverEl.boardCloseSlugForm) popoverEl.boardCloseSlugForm();
      // The board-wide switch may have moved this card since the menu was
      // last open, so the view item is re-decided here too.
      if (popoverEl.boardRefreshCardView) popoverEl.boardRefreshCardView();
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

      // There is deliberately no group-rename item here any more. That item
      // only existed because a group had no UI of its own; the group container
      // now has a header bar, and Rename and Ungroup live on it. A card's menu
      // holds card-level things: Group, and the attribute editor below.
      //
      // The naming form for Group.
      //
      // Prompt affordance: this is a form INSIDE the popover that is already
      // open, not window.prompt and not a second modal. window.prompt is
      // suppressed in some browsers inside an iframe, and even where it works
      // it renders as browser chrome rather than as part of the page. The
      // popover already holds text inputs for the attribute editor, so this
      // reuses that pattern rather than inventing one.
      var slugForm = el("div", "board-slug-form");
      slugForm.setAttribute("hidden", "");
      var slugLabel = el("label", "board-slug-label");
      var slugInput = el("input", "board-slug-input");
      slugInput.type = "text";
      slugInput.setAttribute("spellcheck", "false");
      var slugActions = el("div", "board-menu-actions");
      var slugConfirm = el("button", "board-attr-save");
      slugConfirm.type = "button";
      var slugCancel = el("button", "board-attr-add");
      slugCancel.type = "button";
      slugCancel.textContent = "Cancel";
      slugActions.appendChild(slugConfirm);
      slugActions.appendChild(slugCancel);
      var slugHint = el("div", "board-slug-hint");
      slugForm.appendChild(slugLabel);
      slugForm.appendChild(slugInput);
      slugForm.appendChild(slugActions);
      slugForm.appendChild(slugHint);
      groupRow.appendChild(slugForm);

      popoverEl.appendChild(groupRow);
      popoverEl.boardGroupBtn = groupBtn;

      // This ONE card's view. The board-wide switch in the toolbar is the one
      // Steve asked for and the one he will use; this is the exception for
      // "show me the syntax of just this block", which is why it sits in the
      // card's own menu rather than adding a second toolbar control.
      //
      // A card the renderer could not handle gets a disabled item with the
      // reason in its tooltip, the same rule the Group item follows: an action
      // that is not offered stays visible and says why.
      var viewRow = el("div", "board-menu-group-row");
      var cardViewBtn = el("button", "board-menu-item");
      cardViewBtn.type = "button";
      var noRendered = false;

      function refreshCardViewBtn() {
        var cardEl = popoverEl.closest(".board-card");
        noRendered = !!cardEl &&
          cardEl.getAttribute("data-no-rendered") === "1";
        var showing = cardEl
          ? cardEl.getAttribute("data-card-view")
          : effectiveCardView(atom.id, VIEW);
        if (noRendered) {
          cardViewBtn.textContent = "Raw markdown";
          cardViewBtn.disabled = true;
          cardViewBtn.title =
            "This block did not render, so the card is already showing its raw markdown.";
          return;
        }
        cardViewBtn.disabled = false;
        cardViewBtn.textContent = showing === "raw"
          ? "Show rendered"
          : "Show raw markdown";
        cardViewBtn.title = showing === "raw"
          ? "Render this card's CommonMark. Only this card."
          : "Show this card's markdown source. Only this card.";
      }

      cardViewBtn.addEventListener("click", function (e) {
        e.stopPropagation();
        if (cardViewBtn.disabled) return;
        var cardEl = popoverEl.closest(".board-card");
        var showing = cardEl
          ? cardEl.getAttribute("data-card-view")
          : effectiveCardView(atom.id, VIEW);
        setCardOverride(atom.id, showing === "raw" ? "rendered" : "raw");
        refreshCardViewBtn();
      });

      viewRow.appendChild(cardViewBtn);
      popoverEl.appendChild(viewRow);
      popoverEl.boardRefreshCardView = refreshCardViewBtn;
      refreshCardViewBtn();

      // Live preview of exactly what will be written, plus the duplicate
      // warning. The warning never disables the button: Atomdown permits two
      // blocks with the same slug, so the board reports it and writes anyway.
      function updateSlugHint() {
        var clean = sanitizeSlug(slugInput.value);
        if (!clean) {
          slugHint.textContent =
            "No name. The group will show its id instead.";
          return;
        }
        // This form only ever names a NEW group, so every existing owner of
        // the name is somebody else.
        var owners = KNOWN_SLUGS[clean] || [];
        slugHint.textContent = 'Writes slug="' + clean + '".' +
          (owners.length
            ? " Already used by " + owners.join(", ") +
              " - allowed, just harder to read."
            : "");
      }

      function closeSlugForm() {
        slugForm.setAttribute("hidden", "");
        groupBtn.removeAttribute("hidden");
      }

      function openSlugForm() {
        slugLabel.textContent = "Name for this group";
        // Defaulted from the first heading in the selection so one confirm
        // is enough - the user is not made to invent a name.
        slugInput.value = deriveGroupSlug(selectedCardTexts());
        slugConfirm.textContent = "Group";
        groupBtn.setAttribute("hidden", "");
        slugForm.removeAttribute("hidden");
        updateSlugHint();
        slugInput.focus();
        slugInput.select();
      }

      // Reopening the menu must not show a naming form the user walked away
      // from, so refreshGroupItem() closes it (see the call site below).
      popoverEl.boardCloseSlugForm = closeSlugForm;

      slugInput.addEventListener("input", updateSlugHint);
      slugInput.addEventListener("keydown", function (e) {
        e.stopPropagation();
        if (e.key === "Enter") { e.preventDefault(); slugConfirm.click(); }
        if (e.key === "Escape") { e.preventDefault(); closeSlugForm(); }
      });
      slugCancel.addEventListener("click", function (e) {
        e.stopPropagation();
        closeSlugForm();
      });

      slugConfirm.addEventListener("click", async function (e) {
        e.stopPropagation();
        var clean = sanitizeSlug(slugInput.value);
        slugConfirm.disabled = true;
        slugConfirm.textContent = "Grouping...";
        try {
          var result = await syscall(
            "system.invokeFunction",
            "atomdown-board.groupAtoms",
            JSON.stringify(selectedUnitKeys()),
            clean,
          );
          if (!result || !result.ok) {
            slugHint.textContent = "Failed: " +
              ((result && result.error) || "unknown error");
            slugConfirm.disabled = false;
            slugConfirm.textContent = "Group";
            return;
          }
          // On success the worker re-renders this whole panel, so this
          // popover and this form no longer exist.
        } catch (err) {
          slugHint.textContent = "Failed: " + err.message;
          slugConfirm.disabled = false;
          slugConfirm.textContent = "Group";
        }
      });

      groupBtn.addEventListener("click", async function (e) {
        e.stopPropagation();
        if (groupBtn.disabled) return;
        var state = groupMenuStateFor(atom);
        if (!state.enabled) return;
        // Grouping asks for a name first. Ungrouping does not - there is
        // nothing to name, and the group id is already known.
        if (state.action === "group") {
          openSlugForm();
          return;
        }
        groupBtn.disabled = true;
        groupBtn.textContent = "Ungrouping...";
        try {
          var result = await syscall(
            "system.invokeFunction",
            "atomdown-board.ungroupAtoms",
            atom.groupId,
          );
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

      // Renaming an atom: slug is an ordinary directive attribute, so this
      // is the attribute editor, promoted rather than duplicated. It gets its
      // own labelled row FIRST because it is the one attribute a human reads;
      // every other attribute keeps the generic name/value row below. There
      // is deliberately no second rename editor for an atom.
      var slugRow = el("div", "board-attr-slug");
      var atomSlugLabel = el("label", "board-slug-label");
      atomSlugLabel.textContent = "Name (slug) - readable alias, not the id";
      var atomSlugInput = el("input", "board-slug-input");
      atomSlugInput.type = "text";
      atomSlugInput.setAttribute("spellcheck", "false");
      atomSlugInput.placeholder = "unnamed - the card shows " + atom.id;
      atomSlugInput.value = atom.slug || "";
      slugRow.appendChild(atomSlugLabel);
      slugRow.appendChild(atomSlugInput);
      popoverEl.appendChild(slugRow);

      var listEl = el("div", "board-attrs-list");
      (atom.attrs || []).forEach(function (a) {
        // id travels separately (disabled row), slug has its own row above.
        if (a.name === "slug") return;
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
        // slug goes first, so the rewritten directive reads id, slug, then
        // the rest - the order emit.go itself writes. The worker sanitizes it
        // and drops it when it is empty; an empty slug attribute is never
        // written.
        var typedSlug = sanitizeSlug(atomSlugInput.value);
        if (typedSlug !== "") attrs.unshift({ name: "slug", value: typedSlug });
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
      // The field inside a group container is that group's own surface, not
      // board background. It clears nothing: a click in the gap between two
      // member cards must not throw away a selection the user just built.
      if (target && target.closest && target.closest(".board-group")) return;
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

    // Fades every card in the unit being dragged, and the group container
    // itself when the unit is a group, so the whole object reads as in flight.
    function markUnitDragging(unitKey) {
      CARD_ELS.forEach(function (c) {
        if (unitKeyForCard(cardDatum(c)) === unitKey) {
          c.classList.add("board-card-dragging");
        }
      });
      GROUP_ELS.forEach(function (groupEl) {
        if ("group:" + groupEl.getAttribute("data-group-id") === unitKey) {
          groupEl.classList.add("board-card-dragging");
        }
      });
    }

    function clearDragging() {
      document.querySelectorAll(".board-card-dragging").forEach(function (c) {
        c.classList.remove("board-card-dragging");
      });
      clearDropMarkers();
      dragState = null;
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
        markUnitDragging(unitKey);
      });

      header.addEventListener("dragend", clearDragging);
    });

    // The group header's own drag handle. Without it a collapsed group would
    // be unmovable, because every member card's drag handle is hidden. It
    // carries the group's unit key, which is the same key a member card's
    // header resolves to, so the worker sees no difference.
    document.querySelectorAll("[data-drag-unit]").forEach(function (handle) {
      var unitKey = handle.getAttribute("data-drag-unit");
      handle.addEventListener("dragstart", function (e) {
        e.stopPropagation();
        dragState = { unitKey: unitKey };
        e.dataTransfer.effectAllowed = "move";
        try { e.dataTransfer.setData("text/plain", unitKey); } catch (err) {}
        markUnitDragging(unitKey);
      });
      handle.addEventListener("dragend", clearDragging);
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
    //
    // A COLLAPSED group contributes its container's rectangle once, under the
    // group's unit key, instead of its hidden member cards. A hidden element
    // reports an all-zero rectangle, which would otherwise sit above every
    // real card and make every drop land before it. pickDropTarget itself
    // stays untouched and pure — it is fed the right rectangles.
    function cardGeometry() {
      var out = [];
      var seenCollapsed = {};
      CARD_ELS.forEach(function (card) {
        var groupEl = card.closest(".board-group[data-group-id]");
        if (groupEl && isCollapsedGroup(groupEl)) {
          var groupId = groupEl.getAttribute("data-group-id");
          if (seenCollapsed[groupId]) return;
          seenCollapsed[groupId] = true;
          var groupRect = groupEl.getBoundingClientRect();
          out.push({
            unitKey: "group:" + groupId,
            top: groupRect.top,
            bottom: groupRect.bottom,
            el: groupEl,
          });
          return;
        }
        var rect = card.getBoundingClientRect();
        out.push({
          unitKey: unitKeyForCard(cardDatum(card)),
          top: rect.top,
          bottom: rect.bottom,
          el: card,
        });
      });
      return out;
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
    slug: atom.slug || null,
    implicit: atom.implicit,
    groupId: atom.groupId || null,
    groupSlug: atom.groupSlug || null,
    attrs: atom.attrs || [],
  }));

  // The page name only scopes this panel's presentation state (which groups
  // are collapsed, and whether the board was open). It never reaches the
  // document.
  const script = `var ATOMDOWN_BOARD_DATA = ${JSON.stringify(clientData)};\n` +
    `var ATOMDOWN_BOARD_PAGE = ${JSON.stringify(pageName || "")};\n` +
    `var ATOMDOWN_BOARD_COLLAPSED = ${JSON.stringify(collapsed)};\n` +
    `var ATOMDOWN_BOARD_VIEW = ${JSON.stringify(view)};\n` +
    clientScript;

  return { html, script };
}

// ---------------------------------------------------------------------------
// Presentation state: the client's key-value store, never the document.
//
// Two things are remembered between visits: whether the board was open on a
// page, and which of that page's groups are collapsed. Neither is content.
// Atomdown carries no layout, position, card or board metadata, so NONE of
// this may ever become an attribute on a directive — see iugum-w6y.
//
// The store is SilverBullet's own `clientStore` (client/plugos/syscalls/
// clientStore.ts): a per-browser key-value store, durable across a reload,
// invisible to the space's files and to every other device. It is used rather
// than localStorage because a plug's code runs in a Web Worker, and a worker
// has no localStorage at all — clientStore is reachable from both the worker
// and the panel iframe through the one syscall bridge, so there is a single
// persistence mechanism in this file rather than two. It is durable rather
// than session-scoped on purpose: "do not lose it on refresh" is what Steve
// asked for, and Close deletes the key, so closed stays closed.
//
// Every read and every write is failure-tolerant. A store that is missing or
// throwing (private browsing, an older host, a test stub) degrades to "not
// remembered", which means a closed board and expanded groups — never an
// error in the user's face.
// ---------------------------------------------------------------------------

function boardOpenKey(pageName) {
  return "atomdown-board.open:" + (pageName || "");
}

function collapsedKey(pageName) {
  return "atomdown-board.collapsed:" + (pageName || "");
}

function viewKey(pageName) {
  return "atomdown-board.view:" + (pageName || "");
}

/**
 * This page's remembered rendered/raw choice, board-wide and per card.
 *
 * Always returns a usable state, and that state DEFAULTS TO RENDERED: a page
 * that was never toggled, a store that is missing or throwing, and a stored
 * value in any shape this function does not recognise all come back as
 * rendered with no overrides. Raw is only ever the answer when the user
 * asked for it and the store still says so.
 */
async function loadViewState(pageName) {
  const fallback = { boardView: "rendered", cardViews: {} };
  if (!pageName) return fallback;
  let stored;
  try {
    stored = await syscall("clientStore.get", viewKey(pageName));
  } catch (e) {
    return fallback;
  }
  if (!stored || typeof stored !== "object") return fallback;
  const cardViews = {};
  const raw = stored.cardViews;
  if (raw && typeof raw === "object") {
    for (const id of Object.keys(raw)) {
      if (raw[id] === "raw" || raw[id] === "rendered") cardViews[id] = raw[id];
    }
  }
  return {
    boardView: stored.boardView === "raw" ? "raw" : "rendered",
    cardViews,
  };
}

/** Remembers, or forgets, that the board is showing for this page. */
async function rememberBoardOpen(pageName, open) {
  if (!pageName) return;
  try {
    if (open) await syscall("clientStore.set", boardOpenKey(pageName), true);
    else await syscall("clientStore.delete", boardOpenKey(pageName));
  } catch (e) {
    // Not remembering is acceptable; failing the user's action is not.
  }
}

/** True only when this exact page was left with the board showing. */
async function wasBoardOpen(pageName) {
  if (!pageName) return false;
  try {
    return (await syscall("clientStore.get", boardOpenKey(pageName))) === true;
  } catch (e) {
    return false;
  }
}

/** The ids of this page's collapsed groups. Always an array. */
async function loadCollapsedGroups(pageName) {
  if (!pageName) return [];
  try {
    const stored = await syscall("clientStore.get", collapsedKey(pageName));
    return Array.isArray(stored) ? stored.filter((id) => typeof id === "string") : [];
  } catch (e) {
    return [];
  }
}

/**
 * Draws the board for one page's text. Shared by the toggle command and by
 * the reopen-on-load path, so those cannot drift apart.
 */
async function showBoard(sourceText, pageName) {
  // Rendering happens HERE rather than inside buildBoardHtml, because
  // buildBoardHtml is pure markup assembly and rendering needs a syscall.
  const atoms = await renderAtomBodies(parseAtoms(sourceText));
  const collapsed = await loadCollapsedGroups(pageName);
  const viewState = await loadViewState(pageName);
  const { html, script } = buildBoardHtml(atoms, pageName, collapsed, viewState);

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

/**
 * The "Atomdown: Toggle Board" command. Opening remembers that this page is
 * showing the board; closing forgets it, so Close means closed and stays
 * closed across a reload.
 */
async function toggleBoard() {
  const currentPage = await syscall("editor.getCurrentPage").catch(() => undefined);
  if (boardOpen) {
    await syscall("editor.hidePanel", "modal");
    boardOpen = false;
    await rememberBoardOpen(currentPage, false);
    return;
  }
  const sourceText = await syscall("editor.getText");
  await showBoard(sourceText, currentPage);
  await rememberBoardOpen(currentPage, true);
}

/**
 * Reopens the board after a page load, but ONLY on a page it was left open on.
 * Wired to editor:pageLoaded and editor:pageReloaded (see the manifest).
 *
 * This is the answer to "every refresh and I have to go re-apply the atomdown
 * view". It is not a default: a page whose key was never written, or whose key
 * Close deleted, gets nothing, and the key is scoped to the page name, so
 * opening the board on one page never opens it on another.
 *
 * Three things keep a reopen from racing the editor:
 *   - SilverBullet dispatches these events AFTER it has set the editor state
 *     for the new page (client/content_manager.ts), so editor.getText already
 *     holds that page's text.
 *   - the text is read once and checked: an empty buffer means the editor is
 *     not ready after all, and the board stays closed rather than drawing an
 *     empty one. The remembered state survives, so the next load reopens.
 *   - the page is re-read just before drawing, so a second navigation that
 *     overtakes this one cannot leave the previous page's board on screen.
 *
 * Every failure path ends with the board closed, never with an error dialog.
 */
async function restoreBoard(pageName) {
  try {
    const page = pageName ||
      await syscall("editor.getCurrentPage").catch(() => undefined);
    if (!(await wasBoardOpen(page))) {
      // Navigated to a page the board was not open on. A panel still showing
      // from the previous page would now be describing the wrong document.
      if (boardOpen) {
        try {
          await syscall("editor.hidePanel", "modal");
        } catch (e) {
          // Nothing to hide.
        }
        boardOpen = false;
      }
      return { ok: true, opened: false };
    }
    const sourceText = await syscall("editor.getText");
    if (!sourceText) return { ok: true, opened: false, reason: "no text yet" };
    const stillHere = await syscall("editor.getCurrentPage").catch(() => page);
    if (stillHere !== page) return { ok: true, opened: false, reason: "navigated away" };
    await showBoard(sourceText, page);
    return { ok: true, opened: true };
  } catch (e) {
    boardOpen = false;
    return { ok: false, opened: false, error: e.message };
  }
}

// Called back from the panel's own close button (see buildBoardHtml's
// script above) so a click-to-close and re-running the toggle command agree
// on whether the board is open. Closing by the button must forget the page
// too, or the next reload would bring back a board the user just dismissed.
// Not a user-facing command itself.
async function notifyClosed() {
  boardOpen = false;
  const currentPage = await syscall("editor.getCurrentPage").catch(() => undefined);
  await rememberBoardOpen(currentPage, false);
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
 * Shows a warning to the user without blocking the write that produced it.
 *
 * A duplicate slug is the only case today: Atomdown Core permits two blocks
 * with the same slug, so refusing the edit would be the tooling overruling the
 * format. The write happens, and this says what was noticed. Failure to show
 * the notification is never allowed to fail the action.
 */
async function warnUser(message) {
  if (!message) return;
  try {
    await syscall("editor.flashNotification", message, "error");
  } catch (e) {
    // No notification surface (a test stub, an older host). The action itself
    // already succeeded; there is nothing to roll back.
  }
}

/**
 * Redraws the still-open board from the document text a write just produced,
 * so a successful action does not feel like the board closed on you.
 */
async function rerenderBoard(sourceText, pageName) {
  // Through showBoard, so a redraw after Rename or Ungroup re-reads the
  // collapse state instead of springing every group open again.
  await showBoard(sourceText, pageName);
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

  const cleaned = requested.filter((a) => a && a.name && a.name !== "id")
    .map((a) =>
      a.name === "slug" ? { name: "slug", value: sanitizeSlug(a.value) } : a
    )
    .filter((a) => a.name !== "slug" || a.value !== "");
  const newAttrs = [{ name: "id", value: atomId }, ...cleaned];
  const newLine = serializeAtomLine(found.prefix, newAttrs, found.suffix);

  found.lines[found.lineIndex] = newLine;
  const newText = found.lines.join("\n");

  await applyBufferEdit(currentText, newText);

  const slugAttr = cleaned.find((a) => a.name === "slug");
  const newSlug = slugAttr ? slugAttr.value : "";
  const oldSlugAttr = found.attrs.find((a) => a.name === "slug");
  const oldSlug = oldSlugAttr ? oldSlugAttr.value : "";
  await warnUser(slugConflict(newText, newSlug, atomId).warning);

  // A renamed atom must relabel its card, and the card's label comes from the
  // rendered panel, so redraw. Only when the name actually changed: an
  // ordinary attribute save keeps the popover open, which is what lets the
  // user save twice in a row.
  if (newSlug !== oldSlug) {
    const pageName = await syscall("editor.getCurrentPage").catch(() => undefined);
    await rerenderBoard(newText, pageName);
    return { ok: true, slug: newSlug, rerendered: true };
  }

  return { ok: true, slug: newSlug };
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
 *
 * `slug` is the readable name the user typed in the popover's naming form. It
 * is optional: an empty one just means the group shows its id. It is written
 * as a `slug` attribute next to the id, never instead of it, because a slug
 * is not identity (SPEC.md).
 */
async function groupAtoms(unitKeysJson, slug) {
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

  const result = insertGroupMarkers(currentText, unitKeys, groupId, slug);
  if (!result.ok) return result;

  await applyBufferEdit(currentText, result.text);
  await warnUser(result.warning);
  await rerenderBoard(result.text, pageName);

  return { ok: true, groupId, slug: result.slug, warning: result.warning };
}

/**
 * Renames one group, called from the Rename group item in a member card's
 * menu. Rewrites that group's opening marker line and nothing else.
 *
 * The group's id is untouched, so nothing that cites the group breaks, and no
 * atom inside it is rewritten, so no `digest` can go stale. An empty name
 * removes the slug attribute; the group then shows its id again.
 */
async function setGroupSlug(groupId, slug) {
  if (!groupId) return { ok: false, error: "No group id" };

  const pageName = await syscall("editor.getCurrentPage");
  const currentText = await syscall("editor.getText");

  const result = setGroupSlugInSource(currentText, groupId, slug);
  if (!result.ok) return result;
  if (result.text === currentText) return { ok: true, unchanged: true };

  await applyBufferEdit(currentText, result.text);
  await warnUser(result.warning);
  await rerenderBoard(result.text, pageName);

  return { ok: true, slug: result.slug, warning: result.warning };
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
  restoreBoard,
  notifyClosed,
  saveAttrs,
  reorderAtom,
  groupAtoms,
  ungroupAtoms,
  setGroupSlug,
};

const manifest = {
  name: "atomdown-board",
  version: 0.1,
  functions: {
    toggleBoard: {
      path: "./atomdown-board.js:toggleBoard",
      command: { name: "Atomdown: Toggle Board" },
    },
    // Reopens the board after a page load, on a page it was left open on.
    // Both events matter: pageLoaded fires for a browser reload and for
    // navigating to a different page, pageReloaded for reloading the page
    // already open (silverbullet client/content_manager.ts).
    restoreBoard: {
      path: "./atomdown-board.js:restoreBoard",
      events: ["editor:pageLoaded", "editor:pageReloaded"],
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
    setGroupSlug: {
      path: "./atomdown-board.js:setGroupSlug",
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
  setGroupSlugInSource,
  sanitizeSlug,
  slugConflict,
  deriveGroupSlug,
  slugOrId,
  removeLineCollapsingSeam,
  parseAtoms,
  injectSharedFunctions,
  buildBoardHtml,
  buildStripHtml,
  boardOpenKey,
  collapsedKey,
  viewKey,
  loadViewState,
  effectiveCardView,
  sanitizeRenderedHtml,
  isSafeUrl,
  decodeUrlEntities,
  renderAtomBodies,
};

const plugExport = { manifest, functionMapping, internals };

wireWorker(functionMapping, manifest, self.postMessage);

export { plugExport as plug };
