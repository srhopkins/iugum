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
      gap: 14px;
      padding: 16px;
      max-width: 820px;
      margin: 0 auto;
      align-items: stretch;
    }
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
        removeBtn.addEventListener("click", function () { row.remove(); });
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

    function buildPopover(atom, popoverEl) {
      if (popoverEl.dataset.built === "1") return;
      popoverEl.dataset.built = "1";

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
          popover.removeAttribute("hidden");
        } else {
          popover.setAttribute("hidden", "");
        }
      });
    });

    document.addEventListener("click", function () { closeAllPopovers(null); });

    var closeBtn = document.getElementById("atomdown-board-close");
    if (closeBtn) {
      closeBtn.addEventListener("click", async function () {
        try { await syscall("editor.hidePanel", "modal"); } catch (e) {}
        try { await syscall("system.invokeFunction", "atomdown-board.notifyClosed"); } catch (e) {}
      });
    }

    // --- Drag to reorder -----------------------------------------------
    //
    // A unit key mirrors computeUnits() in the worker code: an atom that
    // belongs to a group resolves to that whole group's key, so dragging
    // (or dropping onto) any one member of a group always means "the whole
    // group" - the actual cut/paste of source lines happens fresh on the
    // worker side against the real document, this is only the client's
    // notion of "which cards move/highlight together" and "is this drop
    // target my own current unit" for the UI.
    function unitKeyFor(atom) {
      return atom.groupId ? ("group:" + atom.groupId) : ("atom:" + atom.id);
    }

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
      var atom = ATOMDOWN_BOARD_DATA.find(function (a) { return a.id === atomId; });
      if (!atom) return;
      var unitKey = unitKeyFor(atom);

      header.addEventListener("dragstart", function (e) {
        dragState = { unitKey: unitKey };
        e.dataTransfer.effectAllowed = "move";
        try { e.dataTransfer.setData("text/plain", unitKey); } catch (err) {}
        document.querySelectorAll(".board-card").forEach(function (c) {
          var cAtom = ATOMDOWN_BOARD_DATA.find(function (a) { return a.id === c.getAttribute("data-atom-id"); });
          if (cAtom && unitKeyFor(cAtom) === unitKey) c.classList.add("board-card-dragging");
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

    document.querySelectorAll(".board-card[data-atom-id]").forEach(function (card) {
      var atomId = card.getAttribute("data-atom-id");
      var atom = ATOMDOWN_BOARD_DATA.find(function (a) { return a.id === atomId; });
      if (!atom) return;
      var targetUnitKey = unitKeyFor(atom);

      card.addEventListener("dragover", function (e) {
        if (!dragState || dragState.unitKey === targetUnitKey) return;
        e.preventDefault();
        e.dataTransfer.dropEffect = "move";
        var rect = card.getBoundingClientRect();
        var before = (e.clientY - rect.top) < rect.height / 2;
        clearDropMarkers();
        card.classList.add(before ? "board-card-dropbefore" : "board-card-dropafter");
      });

      card.addEventListener("drop", async function (e) {
        if (!dragState || dragState.unitKey === targetUnitKey) return;
        e.preventDefault();
        var rect = card.getBoundingClientRect();
        var before = (e.clientY - rect.top) < rect.height / 2;
        var movedUnitKey = dragState.unitKey;
        clearDropMarkers();
        await performDrop(movedUnitKey, targetUnitKey, before ? "before" : "after");
      });
    });

    // Dropping in the empty space below the last card (not over any card)
    // moves the dragged unit to the very end of the document.
    var cardsContainer = document.querySelector(".board-cards");
    if (cardsContainer) {
      cardsContainer.addEventListener("dragover", function (e) {
        if (!dragState || e.target !== cardsContainer) return;
        e.preventDefault();
        e.dataTransfer.dropEffect = "move";
      });
      cardsContainer.addEventListener("drop", async function (e) {
        if (!dragState || e.target !== cardsContainer) return;
        e.preventDefault();
        var movedUnitKey = dragState.unitKey;
        clearDropMarkers();
        await performDrop(movedUnitKey, null, "end");
      });
    }
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

  const pageName = await syscall("editor.getCurrentPage");
  const currentText = await syscall("space.readPage", pageName);

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

  await syscall("space.writePage", pageName, newText);

  // Best-effort: refresh the live editor buffer behind the modal so it
  // does not show stale content if the user closes the board. Not fatal
  // if this throws (e.g. a different page is now open).
  try {
    await syscall("editor.reloadPage");
  } catch (e) {
    // ignore
  }

  return { ok: true };
}

/**
 * Moves one card's block (or, if it belongs to a group, the whole group —
 * see the "Drag-to-reorder" comment above computeUnits()) to a new position
 * in the source document, called from a card's drop handler in the panel
 * above.
 *
 * Re-reads the page fresh, same as saveAttrs(), rather than trusting
 * whatever the client last rendered — the document may have changed since
 * the board was opened (another edit, another save). On success, rewrites
 * the whole page and re-renders the still-open panel in place with the new
 * order, so the board does not need a separate "refresh" round trip and
 * does not close as a side effect of a successful drop.
 */
async function reorderAtom(movedUnitKey, targetUnitKey, placement) {
  const pageName = await syscall("editor.getCurrentPage");
  const currentText = await syscall("space.readPage", pageName);

  const result = reorderUnit(currentText, movedUnitKey, targetUnitKey, placement);
  if (!result.ok) return result;
  if (result.unchanged) return { ok: true, unchanged: true };

  await syscall("space.writePage", pageName, result.text);

  try {
    await syscall("editor.reloadPage");
  } catch (e) {
    // ignore, same as saveAttrs()
  }

  const atoms = parseAtoms(result.text);
  const { html, script } = buildBoardHtml(atoms, pageName);
  await syscall("editor.showPanel", "modal", 0, html, script);

  return { ok: true };
}

const functionMapping = { toggleBoard, notifyClosed, saveAttrs, reorderAtom };

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
  },
};

const plugExport = { manifest, functionMapping };

wireWorker(functionMapping, manifest, self.postMessage);

export { plugExport as plug };
