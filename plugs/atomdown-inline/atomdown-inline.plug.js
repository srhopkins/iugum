// atomdown-inline — hand-authored SilverBullet plug worker bundle.
//
// The atomdown card view, drawn ON the normal wiki page instead of in a panel.
// A header-bar icon turns it on for one page; the page then shows a card
// outline per atom, a group outline with a header bar per atom-group, a drag
// handle per movable block, and a rubber-band selection. Everything else about
// the page is unchanged, because the page IS the editor: typing in a card is
// just typing in the document, so this file contains no editor of its own.
//
// This file is NOT the output of `plug-compile` / esbuild. It is written by
// hand in the same shape a real compile would produce, the same way
// plugs/atomdown-board/atomdown-board.plug.js is (see that directory's
// README.md for why plug-compile is not available here). Keep this file plain
// ES2020 JS with no imports — the worker loads it directly.
//
// How the drawing reaches the screen: the vendored client carries one patch,
// the editor decoration seam (docs/silverbullet-decoration-seam.md). The seam
// reads the `editorDecorations` config key, which is plain data, so a plug in a
// web worker can drive it. This file writes that key and nothing else; it never
// touches CodeMirror.
//
// Attribute policy, inherited from atomdown-board: this plug knows no
// application-level attribute name. The only names it reads are `id` and
// `slug`, both of which Atomdown Core itself defines (SPEC.md "Identity").

// ---------------------------------------------------------------------------
// Worker <-> host runtime shim. Same boilerplate every plug worker needs.
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
              "atomdown-inline: function threw",
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
// ATOMDOWN SOURCE SCANNING
//
// Ported from plugs/atomdown-board/atomdown-board.plug.js, which is the tested
// implementation of the Atomdown format's line shapes (atomdown/SPEC.md). The
// format is not re-derived here. Keep the two in step: a change to a scan rule
// belongs in both files, or in a shared file if a bundler ever arrives.
// ---------------------------------------------------------------------------

function escapeHtml(text) {
  return String(text == null ? "" : text)
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;");
}

function escapeAttrValue(value) {
  return String(value)
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll('"', "&quot;");
}

const ATOM_TAG_RE = /^(\s*<!--\s*<atom\s+)([^>]*?)(\/>\s*-->\s*)$/;
const GROUP_OPEN_RE = /^\s*<!--\s*<atom-group\s+([^>]*?)>\s*-->\s*$/;
const GROUP_CLOSE_RE = /^\s*<!--\s*<\/atom-group>\s*-->\s*$/;
const DOC_MARKER_RE = /^\s*<!--\s*<atomdown\b[^>]*\/>\s*-->\s*$/;
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
 * Scans sourceText into the ordered list of top-level reorderable units, plus
 * the line range of the fixed preamble (the `<atomdown version="1"/>` marker)
 * that always stays first.
 *
 * A unit is what one drag moves: a standalone atom, or a whole atom-group. A
 * group always moves as one indivisible span, so a discontiguous group is
 * structurally impossible rather than checked for. See the long note in
 * atomdown-board.plug.js for why that is the right call.
 */
function computeUnits(sourceText) {
  const lines = String(sourceText == null ? "" : sourceText).split("\n");
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
      if (i < n) i++;
      continue;
    }

    const atomMatch = lines[i].match(ATOM_TAG_RE);
    if (atomMatch) {
      const attrs = parseAttrs(atomMatch[2]);
      const idAttr = attrs.find((a) => a.name === "id");
      const slugAttr = attrs.find((a) => a.name === "slug");
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
          atomSlug: slugAttr ? slugAttr.value : null,
          groupId: null,
        });
        continue;
      }
    }

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
 * Moves one unit to a new position relative to another and returns the whole
 * rewritten document. Ported verbatim in behavior from atomdown-board: the
 * blank-line run between two units that stay adjacent is reused line for line,
 * so a drag never reflows a seam it did not touch.
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
      error: "Could not find the dragged block (did the page change?)",
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
        error: "Could not find the drop target (did the page change?)",
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
  const lastOriginalUnit = units[units.length - 1];
  resultLines.push(...lines.slice(lastOriginalUnit.endLine + 1));

  return { ok: true, text: resultLines.join("\n") };
}

/** Deduplicates a key list, keeping first-seen order. */
function dedupeKeys(keys) {
  const out = [];
  (keys || []).forEach(function (key) {
    if (out.indexOf(key) === -1) out.push(key);
  });
  return out;
}

/** True when the selected units are already adjacent in document order. */
function isContiguousUnitSelection(unitOrder, selectedKeys) {
  const positions = [];
  for (let i = 0; i < unitOrder.length; i++) {
    if (selectedKeys.indexOf(unitOrder[i]) !== -1) positions.push(i);
  }
  if (positions.length === 0) return false;
  return positions[positions.length - 1] - positions[0] ===
    positions.length - 1;
}

/**
 * WHAT A CLICK DOES TO THE SELECTION. The whole rule, as a pure function.
 *
 * This is the fix for `iugum-oip`: on the live page a plain click on a card
 * selected nothing at all, and an alt-drag selected two cards and then lost
 * them again a millisecond later. Both came out of the same place — the
 * decision was three inline `if`s inside `onDecorationClick`, and the branch
 * for a plain click on a card only ever CLEARED. So a click could unselect
 * and never select, which is precisely "the inline view is read-only".
 *
 * It is a pure function now for the second half of that defect: the branch
 * that was wrong is the branch no test could reach, because reaching it needed
 * a browser, a page and a pointer. This one takes four values and returns two.
 *
 * The rules, and they are the board panel's own so the two views cannot drift:
 *
 *  - NO UNIT under the pointer (the page gutter, the margin, the blank seam
 *    between two cards) CLEARS the selection. It never selects: the gutter is
 *    where a reader clicks to put the cursor at the start of a line, and
 *    turning that into "select this card" would take an ordinary editing
 *    gesture away.
 *  - PLAIN click: the selection becomes that one unit, and it is the anchor.
 *  - MOD click (cmd on a Mac, ctrl elsewhere): add it, or remove it if it is
 *    already in. Either way it becomes the anchor.
 *  - SHIFT click: extend from the anchor to here, taking every unit BETWEEN
 *    them in document order — including units nobody clicked. The anchor does
 *    not move, so a second shift-click re-extends from the same place rather
 *    than walking. With no anchor yet, shift is a plain click.
 *
 * `order` is the unit keys in document order. A key the order does not know
 * is treated as a plain click, because a stale key cannot anchor a range.
 */
function applyClickSelection(order, selected, anchor, unitKey, mods) {
  const keys = dedupeKeys(selected);
  const list = order || [];
  if (!unitKey) return { selected: [], anchor: null };
  const additive = !!(mods && (mods.metaKey || mods.ctrlKey));
  const extend = !!(mods && mods.shiftKey);
  const here = list.indexOf(unitKey);
  const from = anchor ? list.indexOf(anchor) : -1;

  if (extend && here !== -1 && from !== -1) {
    const lo = Math.min(from, here);
    const hi = Math.max(from, here);
    const range = list.slice(lo, hi + 1);
    // Additive shift keeps what was already selected, the way a file list
    // does. Plain shift replaces the selection with the range.
    return {
      selected: additive ? dedupeKeys(keys.concat(range)) : range,
      anchor: anchor,
    };
  }

  if (additive) {
    const at = keys.indexOf(unitKey);
    return {
      selected: at === -1
        ? keys.concat([unitKey])
        : keys.filter(function (_, i) {
          return i !== at;
        }),
      anchor: unitKey,
    };
  }

  return { selected: [unitKey], anchor: unitKey };
}

/**
 * The smallest single replacement that turns oldText into newText.
 *
 * This is what makes native Cmd-Z work. A reorder, a group and an ungroup all
 * rewrite the document by whole lines, but they reach the editor as ONE
 * editor.replaceRange call, which is one CodeMirror transaction and therefore
 * one entry in the editor's own undo history. There is no private undo stack.
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

const CROCKFORD_BASE32 = "0123456789ABCDEFGHJKMNPQRSTVWXYZ";
const CROCKFORD_ID_RE = /^[0-9A-HJKMNP-TV-Z]{8}$/;

/** One Atomdown id: eight uppercase Crockford Base32 characters, 40 bits. */
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
  while ((m = re.exec(String(sourceText || ""))) !== null) {
    found.push(m[1] !== undefined ? m[1] : m[2]);
  }
  return found;
}

/** Lowercase kebab-case ASCII, the shape atomdown itself generates. */
function sanitizeSlug(input) {
  const SLUG_MAX_LENGTH = 48;
  let text = String(input == null ? "" : input);
  if (typeof text.normalize === "function") {
    text = text.normalize("NFKD").replace(/[\u0300-\u036f]/g, "");
  }
  let slug = text
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "");
  if (slug.length > SLUG_MAX_LENGTH) {
    const cut = slug.slice(0, SLUG_MAX_LENGTH);
    const atBoundary = cut.replace(/-[^-]*$/, "");
    slug = (atBoundary !== "" ? atBoundary : cut).replace(/-+$/, "");
  }
  return slug;
}

/** Warns about a duplicate slug. Never blocks: Core permits duplicates. */
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

/** The default name to offer for a new group, from the blocks selected. */
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

/** What a human reads for one card or group: its slug, else its id. */
function slugOrId(slug, id) {
  const trimmed = String(slug == null ? "" : slug).trim();
  return trimmed !== "" ? trimmed : String(id == null ? "" : id);
}

/** Wraps a contiguous run of units in one atom-group. Two added lines only. */
function insertGroupMarkers(sourceText, unitKeys, groupId, slug) {
  const { lines, units } = computeUnits(sourceText);
  const keys = dedupeKeys(unitKeys);
  if (keys.length < 2) {
    return { ok: false, error: "Select two or more blocks to group them." };
  }
  if (!CROCKFORD_ID_RE.test(groupId)) {
    return {
      ok: false,
      error: "A group id must be eight Crockford Base32 characters.",
    };
  }
  if (existingIds(sourceText).indexOf(groupId) !== -1) {
    return { ok: false, error: "That group id is already used in this page." };
  }
  const positions = [];
  for (let i = 0; i < units.length; i++) {
    if (keys.indexOf(units[i].unitKey) !== -1) positions.push(i);
  }
  if (positions.length !== keys.length) {
    return {
      ok: false,
      error: "Could not find every selected block (did the page change?)",
    };
  }
  for (let i = 0; i < positions.length; i++) {
    if (units[positions[i]].kind === "group") {
      return {
        ok: false,
        error: "Atomdown Core 1 does not permit a group inside a group.",
      };
    }
  }
  if (positions[positions.length - 1] - positions[0] !== positions.length - 1) {
    return {
      ok: false,
      error: "Those blocks are not next to each other, so a group cannot " +
        "wrap them.",
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

/** Renames one group: rewrites its opening marker line and nothing else. */
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
    return { ok: false, error: "Could not find that group (did the page change?)" };
  }

  const cleanSlug = sanitizeSlug(slug);
  const rest = attrs.filter(function (a) {
    return a.name !== "id" && a.name !== "slug";
  });
  const ordered = [{ name: "id", value: groupId }];
  if (cleanSlug !== "") ordered.push({ name: "slug", value: cleanSlug });
  const attrText = ordered.concat(rest)
    .map(function (a) {
      return a.name + '="' + escapeAttrValue(a.value) + '"';
    })
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
 * Renames one atom: rewrites the `slug` attribute on its own directive line.
 *
 * The block's text is untouched, so the atom's `digest` cannot go stale, and
 * the id stays first - a slug is not identity (SPEC.md). Attribute order is
 * id, then slug, then whatever else the directive carried, which is the order
 * `atomdown emit` writes, so emitting afterwards is a no-op.
 */
function setAtomSlugInSource(sourceText, atomId, slug) {
  const lines = String(sourceText || "").split("\n");
  let lineIndex = -1;
  let match = null;
  for (let i = 0; i < lines.length; i++) {
    const m = lines[i].match(ATOM_TAG_RE);
    if (!m) continue;
    const parsed = parseAttrs(m[2]);
    const idAttr = parsed.find(function (a) { return a.name === "id"; });
    if (idAttr && idAttr.value === atomId) {
      lineIndex = i;
      match = m;
      break;
    }
  }
  if (lineIndex === -1) {
    return { ok: false, error: "Could not find that atom (did the page change?)" };
  }
  const attrs = parseAttrs(match[2]);
  const cleanSlug = sanitizeSlug(slug);
  const rest = attrs.filter(function (a) {
    return a.name !== "id" && a.name !== "slug";
  });
  const ordered = [{ name: "id", value: atomId }];
  if (cleanSlug !== "") ordered.push({ name: "slug", value: cleanSlug });
  lines[lineIndex] = serializeAtomLine(
    match[1],
    ordered.concat(rest),
    match[3],
  );
  const conflict = slugConflict(sourceText, cleanSlug, atomId);
  return {
    ok: true,
    text: lines.join("\n"),
    slug: cleanSlug,
    warning: conflict.warning,
  };
}

/** An attribute name this plug will write. The board's rule, restated. */
const ATTR_NAME_RE = /^[A-Za-z_][\w.:-]*$/;

/**
 * Rewrites the WHOLE attribute list on one atom's directive line.
 *
 * This is the write half of the attribute form (iugum-etz). Pure, so the
 * refusals below are unit-testable without a browser.
 *
 * THE ID IS TAKEN FROM THE SOURCE LINE, never from the caller. The form shows
 * the id in a disabled row and never sends it back, so an id cannot be edited
 * by any route through this function. Order is id, then slug, then whatever
 * else the caller sent in the order it sent it - the order `atomdown emit`
 * writes, so emitting afterwards is a no-op.
 *
 * A VALUE THAT CARRIES DIRECTIVE SYNTAX IS REFUSED, NOT ESCAPED, matching the
 * board's rule for a pasted directive in a card body and for the same stated
 * reason: escaping would silently store something other than what the reader
 * typed, so the form would then disagree with the file about its own content,
 * and a refusal a reader can act on beats a rewrite they cannot see.
 *
 * AND THE RESULT IS RE-PARSED BEFORE IT IS RETURNED. The refusals above are a
 * blocklist, and a blocklist is a guess about what breaks a parser. Reading
 * the rewritten line back with this plug's own `ATOM_TAG_RE` and `parseAttrs`
 * and requiring the same id and the same name/value pairs is the property
 * itself: whatever gets past the blocklist still cannot produce a line that
 * means something different from what the form was showing.
 */
function setAtomAttrsInSource(sourceText, atomId, attrs) {
  if (!Array.isArray(attrs)) {
    return { ok: false, error: "Invalid attribute payload" };
  }
  const lines = String(sourceText || "").split("\n");
  let lineIndex = -1;
  let match = null;
  for (let i = 0; i < lines.length; i++) {
    const m = lines[i].match(ATOM_TAG_RE);
    if (!m) continue;
    const idAttr = parseAttrs(m[2]).find(function (a) {
      return a.name === "id";
    });
    if (idAttr && idAttr.value === atomId) {
      lineIndex = i;
      match = m;
      break;
    }
  }
  if (lineIndex === -1) {
    return {
      ok: false,
      error:
        "Could not find this atom's directive (implicit atom, or the page changed)",
    };
  }

  const seen = {};
  const cleaned = [];
  for (let i = 0; i < attrs.length; i++) {
    const a = attrs[i] || {};
    const name = String(a.name == null ? "" : a.name).trim();
    // A blank row is the form's own artefact - pressing Add and then saving -
    // and dropping it is not a silent rewrite of anything the reader wrote.
    if (name === "") continue;
    if (name === "id") continue; // the id travels with the source line
    if (!ATTR_NAME_RE.test(name)) {
      return { ok: false, error: 'Not an attribute name: "' + name + '"' };
    }
    if (seen[name]) {
      return { ok: false, error: "Two attributes named " + name };
    }
    seen[name] = true;
    const value = String(a.value == null ? "" : a.value);
    if (/[\n\r]/.test(value)) {
      return {
        ok: false,
        error: "An attribute value is one line: " + name + " has a line break",
      };
    }
    if (/<|>|"|--/.test(value)) {
      return {
        ok: false,
        error:
          "An Atomdown directive cannot be typed into an attribute value. " +
          name + ' may not contain <, >, " or --.',
      };
    }
    cleaned.push(
      name === "slug"
        ? { name: "slug", value: sanitizeSlug(value) }
        : { name: name, value: value },
    );
  }
  const withSlug = cleaned.filter(function (a) {
    return a.name !== "slug" || a.value !== "";
  });
  const slugAttr = withSlug.find(function (a) { return a.name === "slug"; });
  const rest = withSlug.filter(function (a) { return a.name !== "slug"; });
  const ordered = [{ name: "id", value: atomId }]
    .concat(slugAttr ? [slugAttr] : [])
    .concat(rest);

  const newLine = serializeAtomLine(match[1], ordered, match[3]);
  // THE RE-PARSE. An emptied directive, or one a value broke out of, fails
  // here rather than reaching the document.
  const back = newLine.match(ATOM_TAG_RE);
  if (!back) {
    return {
      ok: false,
      error: "That attribute set does not read back as an atom directive",
    };
  }
  const reparsed = parseAttrs(back[2]);
  const sameId = reparsed.find(function (a) { return a.name === "id"; });
  if (!sameId || sameId.value !== atomId) {
    return { ok: false, error: "That attribute set loses the atom's id" };
  }
  if (
    reparsed.length !== ordered.length ||
    ordered.some(function (a, i) {
      // Against the ESCAPED value, because that is what the line holds: an
      // `&` is written as `&amp;` and `parseAttrs` does not decode it. The
      // board's directive lines carry the same form.
      return reparsed[i].name !== a.name ||
        reparsed[i].value !== escapeAttrValue(a.value);
    })
  ) {
    return {
      ok: false,
      error: "That attribute set does not read back as it was written",
    };
  }

  lines[lineIndex] = newLine;
  const newSlug = slugAttr ? slugAttr.value : "";
  const text = lines.join("\n");
  const conflict = slugConflict(sourceText, newSlug, atomId);
  return {
    ok: true,
    text: text,
    unchanged: text === sourceText,
    slug: newSlug,
    warning: conflict.warning,
  };
}

/** Rebuilds an `<atom .../>` directive line from an attribute list. */
function serializeAtomLine(prefix, attrs, suffix) {
  const attrText = attrs
    .map(function (a) {
      return a.name + '="' + escapeAttrValue(a.value) + '"';
    })
    .join(" ");
  return prefix + attrText + " " + suffix;
}

/** Removes one line, and one of two blank lines it leaves against each other. */
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

/** Removes one group's two marker lines and nothing else. */
function removeGroupMarkers(sourceText, groupId) {
  const { lines, units } = computeUnits(sourceText);
  const unit = units.find(function (u) {
    return u.unitKey === "group:" + groupId;
  });
  if (!unit) {
    return { ok: false, error: "Could not find that group (did the page change?)" };
  }
  if (!GROUP_CLOSE_RE.test(lines[unit.endLine])) {
    return {
      ok: false,
      error: "That group has no closing marker. Fix the page before " +
        "ungrouping it.",
    };
  }
  let out = removeLineCollapsingSeam(lines, unit.endLine);
  out = removeLineCollapsingSeam(out, unit.startLine);
  return { ok: true, text: out.join("\n") };
}

// ---------------------------------------------------------------------------
// THE DECORATION PAYLOAD
//
// Everything the inline view looks like is one plain-data object written to the
// `editorDecorations` config key. These functions build it. They are pure, so
// the payload for a given page text is testable without a browser.
// ---------------------------------------------------------------------------

/** Character offset of the first character of each line. */
function lineStarts(lines) {
  const starts = new Array(lines.length);
  let offset = 0;
  for (let i = 0; i < lines.length; i++) {
    starts[i] = offset;
    offset += lines[i].length + 1;
  }
  return starts;
}

/**
 * The units, plus the individual atom cards inside each group.
 *
 * computeUnits() treats a whole group as one unit, which is right for a drag.
 * The view also wants an outline around each atom inside the group, so the
 * group's inner lines are scanned again as their own little document. Atomdown
 * Core 1 forbids a nested group, so every unit found in there is an atom.
 */
function computeCards(sourceText) {
  const scan = computeUnits(sourceText);
  const cards = [];
  scan.units.forEach(function (unit) {
    if (unit.kind !== "group") {
      cards.push({
        cardKey: unit.unitKey,
        unitKey: unit.unitKey,
        groupUnitKey: null,
        startLine: unit.startLine,
        endLine: unit.endLine,
        atomIds: unit.atomIds,
        atomSlug: unit.atomSlug ?? null,
        implicit: unit.implicit === true,
      });
      return;
    }
    const innerFirst = unit.startLine + 1;
    const inner = scan.lines.slice(innerFirst, unit.endLine);
    computeUnits(inner.join("\n")).units.forEach(function (member, index) {
      cards.push({
        // A member card is not a movable unit — its group is. So its key is
        // namespaced away from the unit keys, and the drag code ignores it.
        cardKey: member.implicit === true
          ? "card:" + unit.groupId + ":" + index
          : "atom:" + member.atomIds[0],
        unitKey: unit.unitKey,
        groupUnitKey: unit.unitKey,
        startLine: member.startLine + innerFirst,
        endLine: member.endLine + innerFirst,
        atomIds: member.atomIds,
        atomSlug: member.atomSlug ?? null,
        implicit: member.implicit === true,
      });
    });
  });
  return { lines: scan.lines, units: scan.units, cards };
}

/**
 * The first line of a unit's VISIBLE content.
 *
 * An explicit atom's `startLine` is its directive line, and a directive is
 * hidden at rest, so the card box must start below it or the box's top edge
 * lands on a collapsed line. An implicit atom has no directive, so its content
 * starts where the unit starts.
 */
function contentFirstLine(unit) {
  return unit.endLine > unit.startLine ? unit.startLine + 1 : unit.startLine;
}

/** True when this unit has no visible content line at all. */
function hasNoContent(unit, lines) {
  const first = contentFirstLine(unit);
  if (lines[first] === undefined) return true;
  // An explicit atom's directive is alone on its line, so a unit that is one
  // line long is a directive with nothing under it.
  if (!unit.implicit) return unit.endLine <= unit.startLine;
  return lines[first].trim() === "";
}

/** Kept for the tests and for callers that still speak of a grip line. */
function gripLine(unit) {
  return contentFirstLine(unit);
}

/**
 * The drag handle, the same six-dot glyph and hover rule the board uses.
 *
 * `tabindex="0"`, so the grip can take keyboard focus. The stylesheet has had
 * a `.atomdown-grip:focus` rule since it was written, and without a tabindex
 * that rule was unreachable: `.focus()` on a plain span does nothing, so a
 * hover-only control had no keyboard route to being visible at all. The grip
 * has no keyboard ACTION - a drag needs a pointer - but it names itself, so a
 * reader tabbing through the page is told the block is movable rather than
 * landing on something invisible.
 */
function gripHtml(title, extraClass) {
  const extra = extraClass ? " " + extraClass : "";
  return '<span class="atomdown-grip' + extra +
    '" role="button" tabindex="0" title="' +
    escapeHtml(title) + '">&#10303;</span>';
}

/**
 * The three-dot glyph, VERTICAL.
 *
 * `&#8942;` is U+22EE VERTICAL ELLIPSIS, the same character the board panel's
 * `.board-menu-btn` carries. The horizontal form this replaced, U+22EF MIDLINE
 * HORIZONTAL ELLIPSIS, is about three times as wide, which mattered once both
 * controls moved into the page gutter: the gutter is 28.8px at the `full`
 * editor width and a horizontal glyph does not fit in it.
 */
const MENU_GLYPH = "&#8942;";

/**
 * THE POPOVER, drawn by this plug rather than shown in the host's picker.
 *
 * Why a hand-built element and not `editor.filterBox`. The picker is the
 * command palette's own surface: it opens centred over the whole window with a
 * search field, and it lists a card's identity and its actions as palette
 * rows. That is the defect Steve reported — the control read as "a command
 * palette opened", not "this card's menu opened", and it is not what the board
 * panel does. The board anchors a popover to the card it belongs to
 * (`.board-menu-popover`), with the name and the id as its first, inert child.
 * This is that popover.
 *
 * How it can work with no script. A seam widget's HTML is set with
 * `innerHTML`, so it carries no listeners — but a click inside a widget comes
 * back to the plug as `editor:decorationClick` carrying the CLASS LIST of the
 * element that was hit (see the seam's `events.click`). So every row names its
 * own action in a class, `atomdown-mi-<action>`, and `onDecorationClick` reads
 * it. One widget carries several controls; that is the shape the collapse
 * caret and the three-dot button already use.
 *
 * WHY NO TEXT INPUT IS IN HERE, measured rather than assumed. The seam's
 * `widgetPressGuard` returns true for a plain press anywhere inside a widget,
 * which makes CodeMirror call `preventDefault` on `mousedown`; a prevented
 * `mousedown` moves no focus. Measured on the real fixture
 * (`plugs/atomdown-e2e/input-probe.test.ts`): a click on an input inside a
 * card header widget left `document.activeElement` on `.cm-content`, and the
 * characters typed next went INTO THE DOCUMENT. So an in-popover attribute
 * form would silently type the reader's attribute values into the page, which
 * is the one thing this whole view may never do. Every row here is a button,
 * and text entry goes through `editor.prompt`, which is a modal dialog with a
 * focused field of its own. The row that edits attributes puts the cursor on
 * the directive line instead, where the peek shows the raw bytes.
 */
function menuPopoverHtml(kind, items) {
  const rows = items.map(function (item) {
    if (item.action === "label") {
      // INERT ON PURPOSE, and it is the popover's first child, the way the
      // panel's is. It carries no `atomdown-menu-item` class, so a click on it
      // matches no action and the popover stays open.
      return '<span class="atomdown-menu-label">' +
        '<span class="atomdown-menu-label-text">' + escapeHtml(item.name) +
        "</span>" +
        '<span class="atomdown-menu-label-note">' +
        escapeHtml(item.description) + "</span>" +
        "</span>";
    }
    return '<span class="atomdown-menu-item atomdown-mi-' +
      escapeHtml(item.action) + '" role="button" tabindex="0" title="' +
      escapeHtml(item.description) + '">' + escapeHtml(item.name) + "</span>";
  }).join("");
  return '<span class="atomdown-menu-popover atomdown-menu-' + kind + '">' +
    rows + "</span>";
}

/**
 * A card's header row: the grip, the readable name, and the id.
 *
 * This is the board's card header, and it exists inline for a second reason:
 * the directive is hidden at rest, so with no header row the atom's id would
 * have nowhere at all to appear. Name in body text, id in small grey
 * monospace, in that order - identity stays visible, the name reads first.
 */
function cardHeaderHtml(unit, nested, directiveText, popover) {
  const id = unit.atomIds[0];
  const slug = unit.implicit ? null : unit.atomSlug;
  const name = slug
    ? '<span class="atomdown-card-slug" title="' +
      escapeHtml('Name (slug) "' + slug + '" - the atom\'s id is ' + id) +
      '">' + escapeHtml(slug) + "</span>"
    : "";
  const idLabel = unit.implicit
    ? '<span class="atomdown-card-badge" title="This block has no atom directive of its own.">no id</span>'
    : '<span class="atomdown-card-id" title="' +
      escapeHtml("Atomdown id " + id) + '">' + escapeHtml(id) + "</span>";
  const peek = directivePeekHtml(directiveText);
  // GRIP LEFT, MENU RIGHT, and both are absolutely positioned OUTSIDE the
  // card's own border, in the page gutter, rather than inside the header's
  // padding. Steve's change: it gives the card its full width back, it is what
  // makes the compact density actually compact, and it leaves the header row
  // free for whatever goes there next.
  //
  // Out of flow is still what makes it work: an in-flow grip ahead of the slug
  // would push the slug right by the grip's width even while the grip is
  // invisible, so the slug would no longer start on the same left edge as the
  // body text (R2). Out of flow, neither control moves anything when it
  // appears on hover.
  //
  // A CONTROL IN THE GUTTER IS NOT PART OF THE CARD'S CLICK TARGET, and that
  // is deliberate. See `onDecorationClick`.
  return '<span class="atomdown-card-head' + (nested ? " atomdown-nested" : "") +
    '">' +
    gripHtml(
      unit.implicit
        ? "Drag to move this block"
        : "Drag to move " + slugOrId(slug, id),
    ) +
    name + idLabel +
    '<span class="atomdown-card-menu" role="button" tabindex="0"' +
    ' aria-haspopup="true" aria-expanded="' + (popover ? "true" : "false") +
    '" title="' +
    escapeHtml("Actions for " + slugOrId(slug, id)) + '">' + MENU_GLYPH +
    "</span>" +
    (popover || "") +
    "</span>" + peek;
}

/**
 * The directive peek: the raw directive text, rendered inside the card box and
 * shown only while the text cursor is in that directive's own line.
 *
 * Why a copy in the header rather than revealing the line itself. A directive
 * line is collapsed to nothing at rest. Revealing the line would grow it, which
 * moves the card and everything below it; and the line sits ABOVE the card's
 * top edge, so its text would appear outside the box. A copy in the header is
 * absolutely positioned, so it costs no layout at all and cannot move anything,
 * and it is clipped to the card's own left and right padding, so it can never
 * cross a border.
 *
 * Nothing but the cursor shows it. Hover does not, because the only reason to
 * read a directive is that you are editing it.
 *
 * THE TEXT IS AN ATTRIBUTE, NOT A TEXT NODE, and that is the whole point of
 * this shape. A directive rendered as a child text node is part of the header
 * widget's own `textContent`: a card header then reads
 * `⠿running-todoZE5AMAB7⋯<!-- <atom id="ZE5AMAB7" digest="sha256:…`, so the
 * plumbing is back in the page's text for anything that reads text rather than
 * pixels — a copy of the page, a screen reader, the `title` of a hovered
 * header, and every DOM signature the front-end suite takes. `content:
 * attr(data-directive)` paints the same characters in the same box while
 * leaving the widget's text content to be the name and the id only.
 */
function directivePeekHtml(directiveText) {
  const text = String(directiveText == null ? "" : directiveText).trim();
  if (text === "") return "";
  return '<span class="atomdown-directive-peek" data-directive="' +
    escapeHtml(text) + '"></span>';
}

/**
 * Display density: "comfortable" (the default) or "compact".
 *
 * The SAME two densities the board panel has, with the same names, the same
 * meaning and the same CSS custom property names, so the two views cannot
 * drift. See atomdown-board.plug.js `normalizeDensity` for the panel's copy of
 * these three functions.
 *
 * COMPACT COMPRESSES CHROME ONLY. Not one content size changes: a heading is a
 * heading at its full rendered size at both densities, because the point is to
 * fit more of the document on screen, not to shrink the document. What compact
 * removes is the card header row, the GROUP label, the group id and the word
 * after the member count, and most of the padding.
 *
 * Pure, so the command, the button's tooltip and the decoration payload cannot
 * disagree about which density is showing.
 */
function normalizeDensity(value) {
  return value === "compact" ? "compact" : "comfortable";
}

/** The other density: what one press of the switch would take you to. */
function otherDensity(value) {
  return normalizeDensity(value) === "compact" ? "comfortable" : "compact";
}

/**
 * The switch's tooltip.
 *
 * Same idiom as the panel's toolbar button: it names the state pressing would
 * GIVE you, not the state you are in, so reading it tells you what the press
 * does.
 */
function densityTitle(value) {
  return otherDensity(value) === "compact"
    ? "Compact: no card headers, a thin group bar, the same content size"
    : "Comfortable: the card header comes back, with roomier spacing";
}

/**
 * The class the density puts on every decorated line and every widget.
 *
 * One class, present at BOTH densities rather than only at compact, for two
 * reasons. The knob overrides have somewhere to be declared either way, and a
 * reader (or a test) can ask the DOM which density is showing instead of
 * inferring it from an absence.
 */
function densityClass(value) {
  return "atomdown-" + normalizeDensity(value);
}

/**
 * The group header bar: the collapse chevron, the kind, the group's readable
 * name, its id and how many atoms it holds. The grip and the three-dot menu
 * are children of this widget too, but they are NOT on the bar - see below.
 *
 * The bar is also the group box's TOP EDGE. The group's opening marker line is
 * a directive and therefore collapsed, so it can carry no visible border, and
 * this widget sits directly above it. That is why the accent border-top and
 * the top corner radii are on this element in the stylesheet.
 *
 * Every control carries its own class. The seam reports the classes of the
 * element that was clicked, so one widget can carry several controls without
 * needing a widget each.
 *
 * THE GRIP AND THE MENU SIT OUTSIDE THE GROUP CONTAINER, in the same two page
 * gutters the card's controls use (iugum-938). They are the FIRST and the last
 * child, so the DOM order reads left to right the way the screen does, and the
 * stylesheet takes both out of the bar's flex flow and pins them to the bar's
 * own border box. Three things follow from that, and each was a reason:
 *
 *  - the two views agree. A card's controls are in the gutter, hover-only; the
 *    group's were inside a saturated accent bar, always visible. Steve's
 *    report was exactly that disagreement.
 *  - the bar gets its width back for the name, the id and the count, which is
 *    what it is for.
 *  - a control on the page ground needs no hover treatment of its own. The
 *    translucent wash exists because an opaque chip on the accent fill reads
 *    as a hole punched through the bar; off the bar there is no fill to punch.
 *    The collapse chevron stays on the bar and keeps the wash.
 *
 * VERTICAL ALIGNMENT IS THE BAR'S ROW, not the container's centre, and that is
 * also the collision rule. A group's controls on the bar's row cannot meet a
 * member card's controls, because the bar is a row of its own above every
 * member; centring on the whole container would park the group's grip beside
 * an arbitrary member card, where two grips in one gutter say nothing about
 * which one moves the group.
 */
function groupHeaderHtml(unit, memberCount, directiveText, collapsed, popover) {
  const name = slugOrId(unit.groupSlug, unit.groupId);
  const word = memberCount === 1 ? "card" : "cards";
  return [
    gripHtml("Drag to move the whole group " + name, "atomdown-group-grip"),
    '<span class="atomdown-group-collapse" title="' +
    (collapsed ? "Expand this group" : "Collapse this group") + '">' +
    (collapsed ? "&#9656;" : "&#9662;") + "</span>",
    '<span class="atomdown-group-kind">group</span>',
    '<span class="atomdown-group-name" title="' +
    escapeHtml(
      'Name (slug) "' + name + '" - the group\'s id is ' + unit.groupId,
    ) + '">' + escapeHtml(name) + "</span>",
    '<span class="atomdown-group-id" title="' +
    escapeHtml("Atomdown id " + unit.groupId) + '">' +
    escapeHtml(String(unit.groupId)) + "</span>",
    // THE COUNT IS SPLIT INTO THE NUMBER AND THE WORD, and the split is the
    // only reason compact can keep a bare count: the word is what `display:
    // none` removes there. Same two class names the panel uses.
    '<span class="atomdown-group-count">' +
    '<span class="atomdown-group-count-n">' + memberCount + "</span>" +
    '<span class="atomdown-group-count-word"> ' + word + "</span>" +
    "</span>",
    '<span class="atomdown-group-menu" role="button" tabindex="0"' +
    ' aria-haspopup="true" aria-expanded="' + (popover ? "true" : "false") +
    '" title="' +
    escapeHtml("Actions for group " + name) + '">' + MENU_GLYPH + "</span>",
    popover || "",
    directivePeekHtml(directiveText),
  ].join("");
}

/**
 * What one press of the toggle should do.
 *
 * `remembered` is the per-page flag in clientStore. `applied` is whether the
 * decorations for this page are actually on screen right now.
 *
 * The press follows what the READER CAN SEE, not the flag. The flag can say
 * "on" while nothing is drawn - a page load that arrived before the editor had
 * text leaves the flag set and draws nothing - and in that state a press that
 * trusted the flag would turn the view "off" and look like a dead button. That
 * is the bug this function exists to make impossible, and why it is a pure
 * function with a test rather than an `if` inside the command.
 */
function toggleAction(remembered, applied) {
  return applied ? "off" : "on";
}

/**
 * The whole `editorDecorations` value for one page text.
 *
 * TWO KINDS OF MARK, and the split is the whole design.
 *
 *  - an IDENTITY mark per movable unit, `unit:<key>`, over the unit's entire
 *    source span including its directive lines. It has no line classes and no
 *    CSS. Its only job is to answer "what did the pointer land on" for a
 *    click, a drag and a lasso.
 *  - a BOX mark, `box:<key>`, over just the unit's VISIBLE lines, with
 *    `lineClasses`. That is what draws a card or a group as one closed
 *    rounded box: `-first` takes the top edge, `-mid` the sides, `-last` the
 *    bottom edge, and a one-line block takes `-first` and `-last` together and
 *    so draws the whole box on its own line.
 *
 * They have to be separate ranges. A directive line is hidden at rest, so a
 * box whose first line is the directive would put its top edge on a collapsed
 * line; and a blank source line between two blocks belongs to no box at all,
 * which is exactly what makes the gap between two cards.
 *
 * `selectedKeys` are the unit keys the reader lassoed. They add one more mark
 * per selected unit, so a selection is purely visual and never reaches the
 * document.
 *
 * `density` adds a THIRD kind of mark, `dens:<key>`, over exactly the same
 * span as each box mark and carrying nothing but the density class. It exists
 * because the density's knob values have to reach the LINE elements, and a
 * line's only route to a class is a mark's `lineClasses`. It could not be
 * folded into the box mark's own class: the seam uses `marks[].class` as the
 * STEM of the line classes it derives, so a second class in that string would
 * produce `atomdown-card atomdown-compact-line` rather than two classes. Its
 * `-first`, `-mid` and `-last` variants are unused on purpose — a card inside
 * a group is covered by two of these marks, so those would be ambiguous. The
 * CSS keys off `atomdown-<density>-line` together with the card's or the
 * group's own `-first` / `-last`.
 */
function buildDecorations(
  sourceText,
  selectedKeys,
  collapsedGroupIds,
  density,
  open,
) {
  const scan = computeCards(sourceText);
  const lines = scan.lines;
  const starts = lineStarts(lines);
  const selected = dedupeKeys(selectedKeys);
  const collapsed = dedupeKeys(collapsedGroupIds);
  const dens = normalizeDensity(density);
  const densClass = densityClass(dens);
  const marks = [];
  const widgets = [];
  const folds = [];

  function span(startLine, endLine) {
    return {
      from: starts[startLine],
      to: starts[endLine] + lines[endLine].length,
    };
  }

  /**
   * The density mark for one span: the class, and nothing else.
   *
   * ONE PER TOP-LEVEL UNIT, NEVER ONE PER CARD, and that is a fix rather than
   * a saving. The seam derives `-line`, `-first`, `-mid` and `-last` from
   * every mark and CodeMirror concatenates the class strings of all the line
   * decorations on one line. A member card covered by both its group's mark
   * and its own therefore carried `atomdown-comfortable-line` TWICE and
   * `-first`, `-mid` and `-last` together, and how many of those overlapping
   * marks the editor had realised varied with the scroll position — so a
   * width round trip came back with a different class string for the same
   * correct state. A unit's span already covers every line of every card
   * inside it, so one mark per unit gives every line exactly one.
   */
  function addDensity(key, box) {
    marks.push({
      id: "dens:" + key,
      from: box.from,
      to: box.to,
      class: densClass,
      lineClasses: true,
    });
  }

  /** One atom's card: the box mark and the header row above it. */
  function addCard(unit, boxKey, nested) {
    if (hasNoContent(unit, lines)) return null;
    const first = contentFirstLine(unit);
    // The directive's own source line, when the atom has one. It is the text
    // the header's peek shows while the cursor is in it.
    const directiveText = first > unit.startLine ? lines[unit.startLine] : "";
    const box = span(first, unit.endLine);
    marks.push({
      id: boxKey,
      from: box.from,
      to: box.to,
      class: "atomdown-card",
      lineClasses: true,
      // So the card's header text can be muted at rest and normal when the
      // pointer is anywhere in the card, not only on the header row itself.
      hoverClasses: true,
    });
    // THE HEADER WIDGET IS EMITTED AT BOTH DENSITIES, and at compact it keeps
    // the grip and the three-dot menu only — the name and the id are removed
    // by CSS and the row is lifted out of the layout, so it has no height and
    // draws no seam. The card's top edge and top corners move to the card's
    // own `-first` line there. Emitting the same widget either way is what
    // keeps the grip, the menu and the directive peek working identically at
    // both densities; a density that dropped the widget would have to rebuild
    // those three controls somewhere else.
    // The OPEN key is the widget name with its `box:` prefix off, because
    // that is the form `widgetBoxKey` hands the click handler. Comparing the
    // raw widget name against it never matched, so no card popover opened.
    const isOpen = menuOpenForCard(open, cardOpenKey(boxKey));
    widgets.push({
      id: boxKey,
      at: box.from,
      side: "before",
      class: "atomdown-card-header" + (nested ? " atomdown-nested" : "") +
        (isOpen ? " atomdown-menu-open" : "") +
        " " + densClass,
      html: cardHeaderHtml(
        unit,
        nested,
        directiveText,
        isOpen
          ? menuPopoverHtml(
            "card",
            cardMenuItems(
              slugOrId(unit.atomSlug, unit.atomIds[0]),
              unit.atomIds[0],
              unit.implicit === true,
              nested,
            ),
          )
          : "",
      ),
    });
    return box;
  }

  scan.units.forEach(function (unit) {
    const unitSpan = span(unit.startLine, unit.endLine);
    // The identity mark. No line classes: it draws nothing.
    marks.push({
      id: "unit:" + unit.unitKey,
      from: unitSpan.from,
      to: unitSpan.to,
      class: "atomdown-unit",
    });
    // The density, once, over the whole unit. See addDensity.
    addDensity(unit.unitKey, unitSpan);

    let visible = null;

    if (unit.kind === "group") {
      const members = scan.cards.filter(function (card) {
        return card.groupUnitKey === unit.unitKey;
      });
      // The group box runs marker to marker. Both markers are directives and
      // so are collapsed, which is what turns them into the box's interior
      // padding: the header widget above the opening one carries the top edge,
      // and the closing one carries the bottom edge.
      marks.push({
        id: "box:" + unit.unitKey,
        from: unitSpan.from,
        to: unitSpan.to,
        class: "atomdown-group",
        lineClasses: true,
        // The whole reason the seam grew hover classes: the group's chrome is
        // subdued at rest and comes forward when the pointer is anywhere
        // inside the group, including over a member card. A group is a run of
        // sibling line elements with nothing wrapping them, so CSS :hover
        // cannot express that.
        hoverClasses: true,
      });
      const isCollapsed = collapsed.indexOf(unit.groupId) !== -1;
      widgets.push({
        id: "unit:" + unit.unitKey,
        at: unitSpan.from,
        side: "before",
        class: "atomdown-group-header" +
          (isCollapsed ? " atomdown-group-collapsed" : "") +
          (menuOpenForGroup(open, unit.unitKey) ? " atomdown-menu-open" : "") +
          " " + densClass,
        html: groupHeaderHtml(
          unit,
          members.length,
          lines[unit.startLine],
          isCollapsed,
          menuOpenForGroup(open, unit.unitKey)
            ? menuPopoverHtml(
              "group",
              groupMenuItems(
                slugOrId(unit.groupSlug, unit.groupId),
                unit.groupId,
              ),
            )
            : "",
        ),
      });
      const openLineEnd = starts[unit.startLine] + lines[unit.startLine].length;
      if (unitSpan.to > openLineEnd) {
        // Declarative: the seam makes the editor's fold set match this flag,
        // so the caret is idempotent and never has to move the cursor.
        folds.push({
          from: openLineEnd,
          to: unitSpan.to,
          ...(isCollapsed ? { collapsed: true } : {}),
        });
      }
      // Each atom inside the group gets its own card, inset by the group's
      // interior padding.
      members.forEach(function (card) {
        addCard(
          {
            startLine: card.startLine,
            endLine: card.endLine,
            atomIds: card.atomIds,
            atomSlug: card.atomSlug,
            implicit: card.implicit,
          },
          "box:" + card.cardKey,
          true,
        );
      });
      visible = unitSpan;
    } else {
      visible = addCard(unit, "box:" + unit.unitKey, false) ?? unitSpan;
    }

    if (selected.indexOf(unit.unitKey) !== -1) {
      marks.push({
        id: "sel:" + unit.unitKey,
        from: visible.from,
        to: visible.to,
        class: "atomdown-selected",
        lineClasses: true,
      });
    }
  });

  return {
    // The cursor's own line is the one condition that reveals a hidden
    // directive, so a cursor can never land in a line nobody can see.
    activeLine: true,
    // The directive comments are the format's plumbing, not the reader's
    // content, and on a real page every atom carries a 64-character digest
    // that wraps over three or four rows. Hidden at rest, revealed on the
    // cursor's line. `CommentBlock` is the Lezer node for a block-level HTML
    // comment; `Comment` catches the inline form.
    lines: [
      { selector: "CommentBlock", class: "atomdown-directive" },
      { selector: "Comment", class: "atomdown-directive" },
    ],
    marks,
    widgets,
    folds,
    events: { click: true, selection: true },
    gestures: {
      // A handle rather than a modifier: dragging text in a page you are also
      // editing must stay ordinary text dragging.
      drag: { handleClass: "atomdown-grip" },
      lasso: { modifier: "alt" },
    },
  };
}

/** The `editorDecorations` value that means "the view is off on this page". */
function emptyDecorations() {
  return {
    activeLine: false,
    lines: [],
    marks: [],
    widgets: [],
    folds: [],
    events: {},
    gestures: {},
  };
}

/** The first `box:` name in a seam mark list: the card the pointer is on. */
function firstBoxKey(names) {
  const found = (names || []).find(function (name) {
    return typeof name === "string" && name.indexOf("box:atom:") === 0;
  });
  return found ? found.slice("box:".length) : null;
}

/** The first `unit:` name in a seam mark list, as a unit key. */
function firstUnitKey(names) {
  const found = (names || []).find(function (name) {
    return typeof name === "string" && name.indexOf("unit:") === 0;
  });
  return found ? found.slice("unit:".length) : null;
}

/**
 * The unit key a control in a WIDGET belongs to, from the widget's own name.
 *
 * A MARK LIST CANNOT ANSWER THIS FOR A WIDGET, and that is a defect, not a
 * preference. The seam builds a click's mark list from `posAtCoords`, and a
 * header widget is a block element with no text of its own, so the position it
 * reports is the nearest text — which is the line above it when that line is
 * visible, and the nearest line OUTSIDE the fold when it is not. Collapse two
 * or three groups and a caret's click reported a position inside the previous
 * group's folded range, so the plug read the previous group's `unit:` mark and
 * collapsed or expanded the WRONG GROUP. Measured: pressing every caret in
 * turn to reopen eleven collapsed groups reopened one, re-shut two others and
 * did nothing at all six times.
 *
 * The widget's name is the identity the plug itself put on the widget, so it is
 * exact and independent of where the click landed.
 */
function widgetUnitKey(event) {
  const name = event && typeof event.widget === "string" ? event.widget : "";
  return name.indexOf("unit:") === 0 ? name.slice("unit:".length) : null;
}

/** The same for a card header widget, whose name is its `box:` key. */
function widgetBoxKey(event) {
  const name = event && typeof event.widget === "string" ? event.widget : "";
  return name.indexOf("box:") === 0 ? name.slice("box:".length) : null;
}

/**
 * Turns one `editor:decorationDrag` payload into a reorder request.
 *
 * The origin is whichever unit mark covered the handle. The target is
 * whichever unit mark covered the release point.
 *
 * A RELEASE THAT COVERS NO UNIT IS RESOLVED BY POSITION, and that is the fix
 * for the defect Steve found on the live page: he dragged the first card down
 * one position and it landed at the bottom of an 84-card document.
 *
 * Why the miss happens at all. Both controls now sit in the page gutter, so
 * the natural gesture is to press the grip and travel straight down the
 * gutter. The seam maps the release point to a document position with
 * `posAtCoords`, and the vertical band of the next card's header widget maps
 * to the BLANK LINE between the two cards. A blank line is a unit boundary, so
 * no unit mark covers it and `targetMarks` arrives empty. Measured on the real
 * fixture (`plugs/atomdown-e2e/drag-probe.test.ts`): releasing 80px and 100px
 * below the first card's grip both reported `targetMarks: []`, and the card
 * moved from index 0 to index 81.
 *
 * The old fallback then read `placement` — a side computed against the blank
 * LINE's own midpoint, which says nothing about which two units the pointer
 * was between — and asked for the start or the end of the whole document.
 *
 * The rule now is the board panel's `pickDropTarget` rule, stated in lines
 * rather than pixels: THE DROP GOES BEFORE THE FIRST UNIT THAT BEGINS AT OR
 * AFTER THE RELEASE LINE. Card order IS document order in this view, so the
 * next unit down the page and the next unit in source order are the same unit,
 * and the seam's reported line is already the geometric answer — no rectangle
 * has to be measured, and the seam needs no new field. Past the last unit is
 * still the end of the document; above the first unit resolves to "before the
 * first unit", which is the start of the page and cannot reach the document
 * marker above it.
 *
 * `units` is `computeUnits(text).units` — the units in document order, each
 * with the 0-based `startLine` this reads. Returns null when the gesture asks
 * for nothing: no origin, or a drop on the unit that was picked up.
 */
function dragToReorder(event, units) {
  const moved = firstUnitKey(event && event.marks);
  if (!moved) return null;
  const target = firstUnitKey(event && event.targetMarks);
  if (target === moved) return null;
  if (target) {
    return {
      movedUnitKey: moved,
      targetUnitKey: target,
      placement: event.placement === "before" ? "before" : "after",
    };
  }
  // The seam's `targetLine` is 1-based.
  const line = event && typeof event.targetLine === "number"
    ? event.targetLine - 1
    : null;
  if (line !== null) {
    const next = (units || []).find(function (unit) {
      return unit && typeof unit.startLine === "number" &&
        unit.startLine >= line;
    });
    if (next) {
      if (next.unitKey === moved) return null;
      return {
        movedUnitKey: moved,
        targetUnitKey: next.unitKey,
        placement: "before",
      };
    }
  }
  return {
    movedUnitKey: moved,
    targetUnitKey: null,
    placement: "end",
  };
}

/** The unit keys an `editor:decorationLasso` payload swept over. */
function lassoToUnitKeys(event) {
  const keys = [];
  ((event && event.marks) || []).forEach(function (name) {
    if (typeof name !== "string" || name.indexOf("unit:") !== 0) return;
    const key = name.slice("unit:".length);
    if (keys.indexOf(key) === -1) keys.push(key);
  });
  return keys;
}

/**
 * Decides what the group menu offers for one unit, given the selection.
 *
 * A group under the cursor offers Ungroup and Rename, always. Anything else
 * offers Group, and only for a selection Atomdown Core 1 permits: two or more
 * units, none of them already a group, and every one of them adjacent in
 * source order. A refusal carries its reason, because a menu item that does
 * nothing looks broken.
 */
function menuState(unitOrder, selectedKeys, menuUnitKey) {
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
      reason: "Alt-drag a band over two or more blocks first.",
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
  if (!isContiguousUnitSelection(unitOrder, keys)) {
    return {
      action: "group",
      label: "Group",
      enabled: false,
      reason: "Those blocks are not next to each other in the page. A group " +
        "wraps a run of blocks, so grouping them would have to move a block " +
        "you did not drag.",
    };
  }
  return {
    action: "group",
    label: "Group",
    enabled: true,
    reason: "Wrap these " + keys.length + " blocks in one atom-group.",
  };
}

// ---------------------------------------------------------------------------
// PER-PAGE STATE
//
// Whether the view is on is remembered per page in clientStore, so it survives
// a reload and never turns itself on for a page the reader never asked. The
// lasso selection is deliberately NOT remembered: it is a transient pointer
// state, and a stale one on reload would be a lie about what is selected.
// ---------------------------------------------------------------------------

function inlineOnKey(pageName) {
  return "atomdown-inline.on:" + (pageName || "");
}

function collapsedKey(pageName) {
  return "atomdown-inline.collapsed:" + (pageName || "");
}

/**
 * The display density's key. Same store and same `atomdown-inline.<what>:<page>`
 * shape as the view's own on/off flag and its collapsed set, so there is ONE
 * storage mechanism for every piece of this view's presentation state. The
 * panel's own key is `atomdown-board.density:<page>`.
 */
function densityKey(pageName) {
  return "atomdown-inline.density:" + (pageName || "");
}

/** This page's remembered density. Comfortable unless it says otherwise. */
async function loadDensity(pageName) {
  if (!pageName) return "comfortable";
  try {
    return normalizeDensity(
      await syscall("clientStore.get", densityKey(pageName)),
    );
  } catch (e) {
    return "comfortable";
  }
}

async function rememberDensity(pageName, density) {
  if (!pageName) return;
  try {
    if (normalizeDensity(density) === "compact") {
      await syscall("clientStore.set", densityKey(pageName), "compact");
    } else {
      // Comfortable is the default, so it is the ABSENCE of a value rather
      // than a value. A page that was never switched and a page switched back
      // then read identically.
      await syscall("clientStore.delete", densityKey(pageName));
    }
  } catch (e) {
    // Not remembering is acceptable; failing the reader's action is not.
  }
}

/** The ids of this page's collapsed groups. Always an array. */
async function loadCollapsed(pageName) {
  if (!pageName) return [];
  try {
    const stored = await syscall("clientStore.get", collapsedKey(pageName));
    return Array.isArray(stored)
      ? stored.filter(function (id) { return typeof id === "string"; })
      : [];
  } catch (e) {
    return [];
  }
}

async function rememberCollapsed(pageName, ids) {
  if (!pageName) return;
  try {
    if (ids.length > 0) {
      await syscall("clientStore.set", collapsedKey(pageName), ids);
    } else {
      await syscall("clientStore.delete", collapsedKey(pageName));
    }
  } catch (e) {
    // Not remembering is acceptable; failing the reader's action is not.
  }
}

/**
 * Flip one group's collapsed state.
 *
 * Pure, so the control's behaviour is testable without a browser. The set is
 * the whole state: there is no alternation from memory, which is what used to
 * desynchronise the caret and leave a collapsed group stuck shut.
 */
function toggleCollapsed(ids, groupId) {
  const out = dedupeKeys(ids);
  const at = out.indexOf(groupId);
  if (at === -1) return out.concat([groupId]);
  return out.filter(function (_, i) { return i !== at; });
}

/** True only when this exact page was left with the inline view on. */
async function isInlineOn(pageName) {
  if (!pageName) return false;
  try {
    return (await syscall("clientStore.get", inlineOnKey(pageName))) === true;
  } catch (e) {
    return false;
  }
}

async function rememberInlineOn(pageName, on) {
  if (!pageName) return;
  try {
    if (on) await syscall("clientStore.set", inlineOnKey(pageName), true);
    else await syscall("clientStore.delete", inlineOnKey(pageName));
  } catch (e) {
    // Not remembering is acceptable; failing the reader's action is not.
  }
}

/** The page the decorations on screen were built for, and its selection. */
let activePage = null;
let selectedUnitKeys = [];
/**
 * The unit a shift-click extends FROM: the last unit clicked without shift.
 *
 * Held next to the selection rather than derived from it, because "the end of
 * the selection" is not the same thing. A range built from card 5 back to card
 * 2 anchors at 5, so the next shift-click extends from 5 again instead of
 * walking the anchor down the page with every press.
 */
let selectionAnchorKey = null;
/**
 * The group ids collapsed on the page being shown.
 *
 * Persisted per page, like the view's own on/off flag, so a collapsed group is
 * still collapsed after a reload. It is presentation: no byte of it reaches
 * the document.
 */
let collapsedGroupIds = [];
/**
 * The density the decorations on screen were built at.
 *
 * Presentation, like the two above: not one byte of the document changes with
 * it. Persisted per page in the same store, so a page left compact draws
 * compact after a reload.
 */
let activeDensity = "comfortable";
/**
 * Which control's popover is open, or null. Presentation, like the three
 * above: not one byte of the document changes with it, and it is deliberately
 * NOT persisted — a menu left open across a reload would be a surprise.
 *
 * Shape: `{ kind: "card", boxKey }` or `{ kind: "group", unitKey }`.
 */
let openMenu = null;

/**
 * The key a card's popover is remembered under: its widget name without the
 * `box:` prefix, which is the form `widgetBoxKey` produces from a click.
 */
function cardOpenKey(boxKey) {
  const key = String(boxKey == null ? "" : boxKey);
  return key.indexOf("box:") === 0 ? key.slice("box:".length) : key;
}

/** Is this card's popover the open one? */
function menuOpenForCard(open, boxKey) {
  return !!open && open.kind === "card" && open.boxKey === boxKey;
}

/** Is this group's popover the open one? */
function menuOpenForGroup(open, unitKey) {
  return !!open && open.kind === "group" && open.unitKey === unitKey;
}

async function warnUser(message, level) {
  if (!message) return;
  try {
    await syscall("editor.flashNotification", message, level || "error");
  } catch (e) {
    // No notification surface. The action itself already ran.
  }
}

/**
 * Writes the decorations for `text` and makes the client pick them up.
 *
 * `rebuild` is the important argument.
 *
 *  - `false` (the normal case) writes the config and nudges the editor with an
 *    empty transaction. The seam notices the new value and rebuilds marks and
 *    widgets from it. The undo history is untouched, which is what keeps a
 *    reorder revertible with one Cmd-Z.
 *  - `true` calls editor.rebuildEditorState, which the client needs to install
 *    or remove the line classes, the event handlers and the gesture handlers.
 *    That call discards the undo history, so it is used only when the reader
 *    turns the view on or off or loads the page — never after an edit.
 */
async function writeDecorations(text, rebuild) {
  await syscall(
    "config.set",
    "editorDecorations",
    buildDecorations(
      text,
      selectedUnitKeys,
      collapsedGroupIds,
      activeDensity,
      openMenu,
    ),
  );
  if (rebuild) {
    await syscall("editor.rebuildEditorState");
    return;
  }
  try {
    await syscall("editor.dispatch", {});
  } catch (e) {
    // An older host with no editor.dispatch: the next keystroke picks the new
    // value up instead. Never fail the action over the nudge.
  }
}

async function clearDecorations(rebuild) {
  selectedUnitKeys = [];
  selectionAnchorKey = null;
  collapsedGroupIds = [];
  openMenu = null;
  activeDensity = "comfortable";
  activePage = null;
  await syscall("config.set", "editorDecorations", emptyDecorations());
  if (rebuild) await syscall("editor.rebuildEditorState");
}

/**
 * Applies newText to the live editor buffer as ONE undoable change.
 *
 * The decorations for the new text are written first, so the seam rebuilds them
 * from the post-edit document in the same transaction. Returns true when
 * something changed.
 */
async function applyEdit(oldText, newText) {
  const edit = minimalEdit(oldText, newText);
  if (!edit) return false;
  // An edit invalidates whatever popover was open: it was drawn from the
  // pre-edit document.
  openMenu = null;
  await syscall(
    "config.set",
    "editorDecorations",
    buildDecorations(
      newText,
      selectedUnitKeys,
      collapsedGroupIds,
      activeDensity,
      openMenu,
    ),
  );
  await syscall("editor.replaceRange", edit.from, edit.to, edit.insert);
  return true;
}

// ---------------------------------------------------------------------------
// COMMANDS AND EVENTS
// ---------------------------------------------------------------------------

/**
 * "Atomdown: Toggle Inline View" — the command the header-bar icon runs.
 *
 * The icon needs no client change: action buttons are config, and Space Lua's
 * actionButton.define appends one. See Library/Atomdown/Inline.md.
 */
async function toggleInline() {
  const page = await syscall("editor.getCurrentPage").catch(() => undefined);
  const remembered = await isInlineOn(page);
  const applied = await isInlineApplied();
  if (toggleAction(remembered, applied) === "off") {
    await rememberInlineOn(page, false);
    await clearDecorations(true);
    return { ok: true, on: false };
  }
  const text = await syscall("editor.getText");
  await rememberInlineOn(page, true);
  activePage = page;
  selectedUnitKeys = [];
  selectionAnchorKey = null;
  openMenu = null;
  collapsedGroupIds = await loadCollapsed(page);
  activeDensity = await loadDensity(page);
  await writeDecorations(text, true);
  return { ok: true, on: true };
}

/**
 * "Atomdown: Toggle Inline Density" — comfortable <-> compact for this page.
 *
 * PRESENTATIONAL, and provably so: it re-reads the page text, rebuilds the
 * decoration payload from it and writes nothing else. No `editor.replaceRange`
 * is reachable from here, so a density switch cannot change a document byte.
 *
 * No `editor.rebuildEditorState` either. The density changes mark classes and
 * widget classes, and the seam keeps both in one StateField that it rebuilds
 * whenever the config value changes — so the empty transaction is enough, and
 * the undo history survives a switch. `rebuildEditorState` calls `setState`,
 * which would throw that history away.
 *
 * The switch works with the view OFF too: it records the choice for the page,
 * so turning the view on afterwards draws the density asked for. It does not
 * turn the view on by itself — a density button is not a view button.
 */
async function toggleDensity() {
  const page = await syscall("editor.getCurrentPage").catch(() => undefined);
  const current = await loadDensity(page);
  const next = otherDensity(current);
  await rememberDensity(page, next);
  activeDensity = next;
  // A popover is anchored to a header whose size the density changes, so it
  // shuts rather than being redrawn in the wrong place.
  openMenu = null;
  if (!(await isInlineApplied())) {
    await warnUser(
      "Atomdown density is now " + next + " for this page. The card view is " +
        "off, so press the grid button to see it.",
      "info",
    );
    return { ok: true, density: next, on: false };
  }
  const text = await syscall("editor.getText");
  activePage = page;
  await writeDecorations(text, false);
  return { ok: true, density: next, on: true };
}

/**
 * The header-bar buttons, registered BY THE PLUG rather than by the library
 * page's `space-lua` block.
 *
 * WHY IT MOVED. An action button is plain configuration — `actionButton.define`
 * in Space Lua is three lines that read `actionButtons`, append to it and write
 * it back (silverbullet/libraries/Library/Std/APIs/Action Button.md), and the
 * `config.insert` syscall is open to a plug (client/plugos/syscalls/config.ts).
 * A `space-lua` block, though, only runs when the client's own page index has
 * that page in it, and that index is per browser origin and can be silently
 * stale — so the same server showed a button whose command was "not found" in
 * one tab and no button at all in another. A plug is loaded by file discovery
 * with no index involved, which is exactly why the COMMANDS always worked when
 * the button did not. Registering the buttons here removes that whole failure
 * class: the button and the command it runs now arrive together or not at all.
 *
 * IDEMPOTENT AND SELF-HEALING, because it has to run more than once and cannot
 * know when.
 *
 * `Config.clear()` wipes every config value each time Space Lua reloads
 * (silverbullet/client/client_system.ts, `loadLuaScripts`), and Space Lua then
 * writes the whole `actionButtons` array back from
 * `Library/Std/Config.md` — so anything a plug appended is gone twice over.
 * Registering on `editor:init` and on every page load is not enough on its
 * own: the client dispatches `editor:reloadState` when the FIRST INDEX of a
 * space completes (client/data/object_index.ts, the `mq:emptyQueue:indexQueue`
 * handler) and again after a version-bump reindex, both of which land after
 * `editor:init` has already been and gone. Measured on a fresh space: the two
 * commands were registered and the header bar had neither button.
 *
 * Listening to `editor:reloadState` cannot fix it either, and that is a race
 * rather than an oversight: `EventHook.dispatchEvent` queues plug handlers and
 * local listeners as concurrent promises, so this plug's first syscall and
 * `reloadState`'s `config.clear()` are in flight together, and the clear wins.
 *
 * So there is a heartbeat as well. A tick re-reads the key and appends only
 * what is missing, which costs one syscall every few seconds and makes the
 * button survive a config clear whenever and however it happens. A button
 * whose command is already in the list is skipped, so no reload can produce
 * two grid icons.
 */
const ACTION_BUTTONS = [
  {
    icon: "grid",
    description: "Atomdown card view on this page",
    command: "Atomdown: Toggle Inline View",
  },
  {
    // `align-justify` is a feather icon: four flush rows, which is the
    // clearest picture of "how tightly is this page packed" in that set.
    icon: "align-justify",
    description: "Atomdown display density on this page",
    command: "Atomdown: Toggle Inline Density",
  },
];

/**
 * The command of every action button already configured.
 *
 * READ ONE FIELD AT A TIME, NEVER THE WHOLE ARRAY, and that is a hard
 * constraint rather than a style. `Library/Std/Config.md` defines the back and
 * forward buttons with a `run` CALLBACK, so `actionButtons` holds Lua
 * functions; a syscall that returned the array had to structuredClone it into
 * the worker and threw
 * `Failed to execute 'postMessage' on 'Worker': (...) could not be cloned`.
 * The plug then caught its own failure, registered nothing, and the header bar
 * had no Atomdown button while both commands worked — the exact symptom this
 * whole move was supposed to end.
 *
 * `config.get` takes a dot path, and an array is an object, so
 * `actionButtons.length` and `actionButtons.<i>.command` each return one
 * clonable scalar.
 */
async function configuredButtonCommands() {
  const count = await syscall("config.get", "actionButtons.length", 0);
  const total = typeof count === "number" && count > 0 ? count : 0;
  const commands = [];
  for (let i = 0; i < total; i++) {
    commands.push(
      await syscall("config.get", "actionButtons." + i + ".command", ""),
    );
  }
  return commands;
}

/**
 * Make the header bar re-read the `actionButtons` key.
 *
 * WHY A NUDGE IS NEEDED AT ALL. The header bar reads that key while it renders
 * (silverbullet/client/editor_ui.tsx, `const actionButtons = client.config.get`)
 * and nothing subscribes to config, so a button appended after the last render
 * sits in the config and not on screen. Space Lua never noticed, because its
 * `actionButton.define` runs during the boot sequence and a page load renders
 * afterwards anyway. A plug appending later gets no render for free. Measured:
 * both buttons in `config.get("actionButtons")`, neither in the header bar.
 *
 * `editor.showProgress` with no arguments is the cheapest honest re-render.
 * It sets the progress indicator to nothing, which is what it already is, and
 * the reducer builds a new state object either way, so the UI re-renders. It
 * touches nothing else — no panel is shown or hidden, no UI option is written,
 * no editor state is rebuilt, so no undo history is lost. The alternatives
 * were worse: `editor.hidePanel` would clobber a panel another plug owns, and
 * `editor.reloadConfigAndCommands` reloads Space Lua, which starts with the
 * `config.clear()` that removed these buttons in the first place.
 *
 * Called only when a button was actually added, so it runs about once per
 * config generation rather than on every heartbeat.
 */
async function nudgeHeaderBar() {
  try {
    await syscall("editor.showProgress");
  } catch (e) {
    // An older host, or no progress indicator. The button is in the config
    // either way and the next render will pick it up.
  }
}

/**
 * The registration itself, serialized.
 *
 * Two calls in flight together — a page load and a heartbeat tick — would both
 * read the key before either inserted, and the second would append a duplicate
 * icon. Same lost-update shape as the collapse presses, same queue.
 */
let buttonQueue = Promise.resolve();

function registerActionButtons() {
  const work = applyRegisterActionButtons;
  const next = buttonQueue.then(work, work);
  buttonQueue = next.then(function () {}, function () {});
  return next;
}

async function applyRegisterActionButtons() {
  try {
    const present = await configuredButtonCommands();
    const added = [];
    for (let i = 0; i < ACTION_BUTTONS.length; i++) {
      const button = ACTION_BUTTONS[i];
      if (present.indexOf(button.command) !== -1) continue;
      await syscall("config.insert", "actionButtons", button);
      added.push(button.command);
    }
    if (added.length > 0) await nudgeHeaderBar();
    return { ok: true, added };
  } catch (e) {
    // A host with no config.insert still gets the commands, which are the
    // plug's real surface. Never fail a page load over an icon.
    return { ok: false, error: e.message };
  }
}

/**
 * How often the heartbeat actually looks: every fifth tick.
 *
 * Pure, so the interval is testable and stated in one place. One second is the
 * client's own cron tick and is more often than a config clear can happen; a
 * five-second window is imperceptible for an icon coming back and cuts the
 * syscall traffic by four fifths.
 */
const BUTTON_CHECK_TICKS = 5;

function shouldReassertButtons(tick) {
  return tick % BUTTON_CHECK_TICKS === 0;
}

let buttonTick = 0;

/** The heartbeat. Wired to `cron:secondPassed`; never throws. */
async function heartbeatActionButtons() {
  buttonTick++;
  if (!shouldReassertButtons(buttonTick)) return { ok: true, checked: false };
  const result = await registerActionButtons();
  return { ok: true, checked: true, added: result.added ?? [] };
}

/**
 * Whether decorations for the inline view are on screen right now.
 *
 * Read from the config key the seam actually renders from, not from this
 * plug's own memory: a worker can be restarted while the page stays up, and
 * then module state says "off" while the page still shows cards.
 */
async function isInlineApplied() {
  try {
    const current = await syscall("config.get", "editorDecorations");
    return !!(current && Array.isArray(current.marks) &&
      current.marks.length > 0);
  } catch (e) {
    // No config read available: fall back to what this worker remembers.
    return activePage !== null;
  }
}

/**
 * Restores the view after a page load, but ONLY on a page it was left on.
 *
 * A page whose key was never written gets nothing. This is the answer to
 * "every refresh and I have to switch the view back on", and it is not a
 * default: the key is per page, and turning the view off deletes it.
 */
async function restoreInline(pageName) {
  // The buttons first, and unconditionally: they belong to the header bar, not
  // to a page the view happens to be on.
  await registerActionButtons();
  try {
    const page = pageName ||
      await syscall("editor.getCurrentPage").catch(() => undefined);
    if (!(await isInlineOn(page))) {
      // Navigated to a page the view is off on. Decorations still on screen
      // would now be describing the wrong document.
      if (activePage !== null) await clearDecorations(true);
      return { ok: true, on: false };
    }
    const text = await syscall("editor.getText");
    if (!text) return { ok: true, on: false, reason: "no text yet" };
    const stillHere = await syscall("editor.getCurrentPage").catch(() => page);
    if (stillHere !== page) {
      return { ok: true, on: false, reason: "navigated away" };
    }
    activePage = page;
    selectedUnitKeys = [];
    selectionAnchorKey = null;
    openMenu = null;
    collapsedGroupIds = await loadCollapsed(page);
    activeDensity = await loadDensity(page);
    await writeDecorations(text, true);
    return { ok: true, on: true };
  } catch (e) {
    return { ok: false, on: false, error: e.message };
  }
}

/**
 * Re-derives the decorations from the page as it is now.
 *
 * Wired to editor:pageSaved, so a block the reader typed into the page gets its
 * own card once the page settles. No rebuild, so this never disturbs undo.
 */
async function refreshInline() {
  try {
    const page = await syscall("editor.getCurrentPage").catch(() => undefined);
    if (!(await isInlineOn(page))) return { ok: true, on: false };
    const text = await syscall("editor.getText");
    if (!text) return { ok: true, on: false };
    activePage = page;
    await writeDecorations(text, false);
    return { ok: true, on: true };
  } catch (e) {
    return { ok: false, error: e.message };
  }
}

/**
 * The units of the page as it is now: their keys in order for the contiguity
 * rule, and the units themselves for the line a drag was released on.
 */
async function currentUnitOrder() {
  const text = await syscall("editor.getText");
  const units = computeUnits(text).units;
  return {
    text,
    units,
    order: units.map(function (unit) {
      return unit.unitKey;
    }),
  };
}

/**
 * A click anywhere in the page, from the seam.
 *
 * Two things are handled: the group header's own controls, and a plain click on
 * a card, which sets the selection to that one card. Everything else is left
 * alone — the seam never swallows a click, so a click in a card still places
 * the cursor, which is the whole point of the view being the page.
 */
async function onDecorationClick(event) {
  if (!event) return { ok: true };
  const page = event.page;
  if (!(await isInlineOn(page))) return { ok: true };
  const classes = event.classes || [];

  // Every control below lives in a WIDGET, so its unit comes from the
  // widget's own name and only falls back to the mark list. See
  // `widgetUnitKey`: a click in a block widget reports the position of the
  // nearest text, which is the wrong group's range once a neighbouring group
  // is folded.
  if (classes.indexOf("atomdown-group-collapse") !== -1) {
    return await collapseGroup(widgetUnitKey(event) ?? firstUnitKey(event.marks));
  }

  // A ROW OF AN OPEN POPOVER. Checked before the two buttons, because a row
  // lives inside the same widget the button does.
  const action = menuActionFromClasses(classes);
  if (action !== null) {
    if (openMenu && openMenu.kind === "group") {
      return await runGroupMenuAction(action, openMenu.unitKey);
    }
    return await runCardMenuAction(
      action,
      openMenu && openMenu.kind === "card"
        ? openMenu.boxKey
        : widgetBoxKey(event) ?? firstBoxKey(event.marks),
      firstUnitKey(event.marks),
    );
  }

  // ANYWHERE ELSE INSIDE THE OPEN POPOVER: the identity label, or the box's
  // own padding. The popover stays open. Without this a click on the label —
  // the thing a reader clicks first, because it names the card — would fall
  // through to the "click outside closes it" rule below and shut the menu.
  if (
    classes.indexOf("atomdown-menu-popover") !== -1 ||
    classes.indexOf("atomdown-menu-label") !== -1 ||
    classes.indexOf("atomdown-menu-label-text") !== -1 ||
    classes.indexOf("atomdown-menu-label-note") !== -1
  ) {
    return { ok: true, open: true };
  }

  if (classes.indexOf("atomdown-group-menu") !== -1) {
    return await toggleGroupMenu(
      widgetUnitKey(event) ?? firstUnitKey(event.marks),
    );
  }

  if (classes.indexOf("atomdown-card-menu") !== -1) {
    return await toggleCardMenu(
      widgetBoxKey(event) ?? firstBoxKey(event.marks),
    );
  }

  // A CLICK ANYWHERE ELSE SHUTS AN OPEN POPOVER, which is what a popover is
  // and what the panel does. It happens before the selection rules below, so
  // the first click after opening a menu dismisses it rather than also
  // changing the selection.
  if (await closeMenu()) return { ok: true, closed: true };

  // A CLICK CARRYING ALT IS THE TAIL OF A LASSO RELEASE, NOT A GESTURE OF ITS
  // OWN — and ignoring it is half the fix for `iugum-oip`.
  //
  // The browser fires `click` after the `mouseup` that ended the alt-drag, so
  // the band's own release arrived here as an ordinary click a millisecond
  // after `onDecorationLasso` had set the selection. The old rule below then
  // read "a click with no modifier and something selected" and cleared it.
  // Measured on the fixture: the lasso reported two units correctly and the
  // page came back with nothing selected, which is exactly Steve's "cannot
  // lasso, so cannot group". The band already decided; this leaves it alone.
  if (event.altKey) return { ok: true };

  // A CLICK THE POINTER TRAVELLED THROUGH IS A TEXT SELECTION, NOT A CARD
  // SELECTION — and this is the other half of `iugum-oip`.
  //
  // Steve: "if I try to highlight text nothing happens to text but underneath
  // I can see something happening". Dragging across a card's text produced a
  // `click` at the end of the drag, this handler selected the CARD, and the
  // redraw that followed rebuilt every line element and threw the reader's
  // text selection away with them. So the text could not be selected or
  // copied, which made the inline view worse than a plain page.
  //
  // The seam reports the pointer's TRAVEL rather than `getSelection()`,
  // because a plain click inside text also collapses a selection there, so
  // "is there a selection" answers yes for both gestures. This is the board
  // panel's own rule (`wasTextDrag`), so the two views cannot drift.
  //
  // A widget is exempt: chrome is `user-select: none`, a drag from the grip
  // produces no click at all, and a few pixels of slip on a menu button must
  // still press the button.
  if (event.moved === true && event.widget === undefined) {
    return { ok: true, textDrag: true };
  }

  const unitKey = firstUnitKey(event.marks);
  // NO UNIT UNDER THE POINTER CLEARS THE SELECTION, and never selects.
  //
  // That is the answer to "is a control outside the card still inside the
  // card's click target?" — no, explicitly. The card's click target is its own
  // line run, the region the seam covers with the card's mark. Both controls
  // now sit in the page gutter, which no mark covers, so a click there reports
  // no unit. The gutter is also where a reader clicks to put the cursor at the
  // start of a line, and turning that into "select this card" would take an
  // ordinary editing gesture away. Clearing is the other half: a click on
  // empty background means "never mind", the same as it does on the board.
  // The document order is only needed to resolve a RANGE, so a click that
  // cannot start one does not pay for a re-read of the page.
  const order = unitKey ? (await currentUnitOrder()).order : [];
  const next = applyClickSelection(
    order,
    selectedUnitKeys,
    selectionAnchorKey,
    unitKey,
    event,
  );
  const unchanged = next.selected.length === selectedUnitKeys.length &&
    next.selected.every(function (key, i) {
      return key === selectedUnitKeys[i];
    });
  selectedUnitKeys = next.selected;
  selectionAnchorKey = next.anchor;
  // A redraw the reader cannot see is still a transaction, and a transaction
  // on every click in the page is what makes an editor feel slow.
  if (unchanged) return { ok: true, selected: selectedUnitKeys.slice() };
  const text = await syscall("editor.getText");
  await writeDecorations(text, false);
  return { ok: true, selected: selectedUnitKeys.slice() };
}

/**
 * The rubber band was released: the units it swept become the selection.
 *
 * The selection is presentation only. It changes one extra mark per unit and
 * never touches the document.
 */
async function onDecorationLasso(event) {
  if (!event) return { ok: true };
  if (!(await isInlineOn(event.page))) return { ok: true };
  selectedUnitKeys = lassoToUnitKeys(event);
  // The band's LAST unit anchors the next shift-click, so a lasso and a
  // shift-click compose the way they do in a file list.
  selectionAnchorKey = selectedUnitKeys.length > 0
    ? selectedUnitKeys[selectedUnitKeys.length - 1]
    : null;
  const text = await syscall("editor.getText");
  await writeDecorations(text, false);
  return { ok: true, selected: selectedUnitKeys.slice() };
}

/**
 * A block was dragged to a new position.
 *
 * The document is re-read here rather than trusted from whatever was on screen,
 * the same "re-read, do not trust the client" rule the board's write path uses.
 * The rewrite then reaches the editor as one transaction, so Cmd-Z reverts the
 * whole move in one step.
 */
async function onDecorationDrag(event) {
  if (!event) return { ok: true };
  if (!(await isInlineOn(event.page))) return { ok: true };
  const current = await currentUnitOrder();
  const request = dragToReorder(event, current.units);
  if (!request) return { ok: true, unchanged: true };

  const result = reorderUnit(
    current.text,
    request.movedUnitKey,
    request.targetUnitKey,
    request.placement,
  );
  if (!result.ok) {
    await warnUser(result.error);
    return result;
  }
  if (result.unchanged) return { ok: true, unchanged: true };
  await applyEdit(current.text, result.text);
  return { ok: true };
}

/** Selection moves are observed but need no action today. */
function onDecorationSelect() {
  return { ok: true };
}

/**
 * Collapses or expands one group, through the editor's own folding.
 *
 * THE FLAGS ARE THE STATE, AND ONE PRESS FLIPS ONE FLAG. Nothing here reads
 * the editor's fold set, and that is the fix rather than a shortcut. The seam
 * reconciles the editor to these flags on EVERY update, so the two can only
 * disagree for a microtask; a press that read the editor inside that window
 * decided from a stale answer and undid the press before it. Measured on the
 * fixture's 11 groups: reopening them one press each left between zero and
 * nine shut, differently on every run.
 *
 * The same continuous reconciliation is what makes a fold from the gutter,
 * from the fold command, or from CodeMirror's own "clear the folds covering
 * the cursor" rule harmless: the seam puts it back to what the flags say. So
 * the caret cannot end up one press behind - the drift that used to leave a
 * collapsed group stuck shut is impossible by construction now, rather than
 * corrected after the fact.
 *
 * The new flag goes into the decorations and the seam does the folding. The
 * cursor is never moved: moving it into the marker line is what used to reveal
 * the directive on a collapse click.
 *
 * ONE PRESS AT A TIME. Every press reads the collapsed set, changes one entry
 * and writes the set back, with four syscalls in between - the page name, the
 * page text, the store write and the config write. Two presses in flight
 * together both read the set as it was before either of them, so the second
 * write loses the first press entirely. The presses are therefore queued,
 * which costs nothing a reader can perceive.
 */
async function collapseGroup(unitKey) {
  return await queueCollapse(function () {
    return applyCollapse(unitKey);
  });
}

/**
 * The press queue: each press waits for the one before it to finish writing.
 *
 * A rejection is swallowed for the CHAIN only, never for the caller — a press
 * that failed must not stop the next one from running.
 */
let collapseQueue = Promise.resolve();
function queueCollapse(work) {
  const next = collapseQueue.then(work, work);
  collapseQueue = next.then(function () {}, function () {});
  return next;
}

async function applyCollapse(unitKey) {
  const groupId = unitKey && unitKey.indexOf("group:") === 0
    ? unitKey.slice("group:".length)
    : null;
  if (!groupId) return { ok: false, error: "No group under that control" };
  const page = await syscall("editor.getCurrentPage").catch(() => undefined);
  const text = await syscall("editor.getText");
  collapsedGroupIds = toggleCollapsed(collapsedGroupIds, groupId);
  await rememberCollapsed(page, collapsedGroupIds);
  await writeDecorations(text, false);
  return {
    ok: true,
    groupId,
    collapsed: collapsedGroupIds.indexOf(groupId) !== -1,
  };
}

/**
 * What a card's three-dot menu offers, as a pure list.
 *
 * The first entry is the card's identity, name then id, and choosing it does
 * nothing: it is the label, the same way the panel's popover puts identity at
 * the top of the menu rather than making you hunt for it.
 *
 * "Edit attributes" OPENS A FORM, the way the panel's does (iugum-etz). It
 * used to put the cursor on the directive line and stop there, which is what
 * Steve reported: choosing it showed the raw atomdown in the card and no form.
 * The form lives in a panel, not in the popover - see attrFormHtml for the
 * measured reason. "Show the directive line" keeps the old behaviour as its
 * own row, because reading the raw bytes in place is still useful and it is
 * what the peek exists for.
 */
function cardMenuItems(name, id, implicit, inGroup) {
  const items = [{
    name: implicit ? "This block has no atom directive" : name + "  -  " + id,
    description: "Identity. Choosing this does nothing.",
    action: "label",
  }];
  if (!implicit) {
    items.push({
      name: "Copy id",
      description: id,
      action: "copy-id",
    });
    if (name && name !== id) {
      items.push({ name: "Copy name", description: name, action: "copy-slug" });
    }
    items.push({
      name: "Rename",
      description: "Change this atom's readable name. Its id does not change.",
      action: "rename",
    });
    items.push({
      name: "Edit attributes",
      description:
        "Open this atom's attributes in a form. The name (slug) comes first, and Save is one undo step.",
      action: "attrs",
    });
    items.push({
      name: "Show the directive line",
      description:
        "Put the cursor on this atom's directive line. The line IS the attributes, and the peek shows it while the cursor is there.",
      action: "reveal",
    });
  }
  items.push(
    inGroup
      ? {
        name: "Ungroup",
        description: "Remove this group's markers. Every atom inside it stays.",
        action: "ungroup",
      }
      : {
        name: "Group selection",
        description: "Wrap the lassoed blocks in one atom-group.",
        action: "group",
      },
  );
  return items;
}

/** What a group's popover offers, as a pure list. Same shape as a card's. */
function groupMenuItems(name, groupId) {
  return [
    {
      name: name + "  -  " + groupId,
      description: "Identity. Choosing this does nothing.",
      action: "label",
    },
    {
      name: "Rename group",
      description: "Change this group's readable name. Its id does not change.",
      action: "rename-group",
    },
    {
      name: "Ungroup",
      description: "Remove the two markers; every atom inside stays.",
      action: "ungroup-group",
    },
  ];
}

/**
 * Open or close a card's popover. PRESENTATION ONLY: it writes the decoration
 * config and nothing else, so opening a menu cannot change a document byte.
 */
async function toggleCardMenu(boxKey) {
  if (!boxKey) return { ok: false, error: "No card under that control" };
  openMenu = menuOpenForCard(openMenu, boxKey)
    ? null
    : { kind: "card", boxKey };
  const text = await syscall("editor.getText");
  await writeDecorations(text, false);
  return { ok: true, open: openMenu !== null };
}

/** Open or close a group's popover. Presentation only, as above. */
async function toggleGroupMenu(unitKey) {
  if (!unitKey || unitKey.indexOf("group:") !== 0) {
    return { ok: false, error: "No group under that control" };
  }
  openMenu = menuOpenForGroup(openMenu, unitKey)
    ? null
    : { kind: "group", unitKey };
  const text = await syscall("editor.getText");
  await writeDecorations(text, false);
  return { ok: true, open: openMenu !== null };
}

/** Shut whatever popover is open, and redraw. Cheap when none is. */
async function closeMenu() {
  if (!openMenu) return false;
  openMenu = null;
  const text = await syscall("editor.getText");
  await writeDecorations(text, false);
  return true;
}

/** The action class a click carried, or null. `atomdown-mi-<action>`. */
function menuActionFromClasses(classes) {
  const list = classes || [];
  for (let i = 0; i < list.length; i++) {
    if (list[i].indexOf("atomdown-mi-") === 0) {
      return list[i].slice("atomdown-mi-".length);
    }
  }
  return null;
}

/**
 * Run one row of a card's popover.
 *
 * The document is re-read here rather than trusted from whatever was drawn,
 * the same "re-read, do not trust the client" rule the drag path uses: the
 * popover may have been open while the reader typed.
 */
async function runCardMenuAction(action, boxKey, unitKey) {
  const atomId = boxKey && boxKey.indexOf("atom:") === 0
    ? boxKey.slice("atom:".length)
    : null;
  const text = await syscall("editor.getText");
  const scan = computeCards(text);
  const card = scan.cards.find(function (c) {
    return c.atomIds[0] === atomId;
  });
  const slug = card ? card.atomSlug : null;
  const name = slugOrId(slug, atomId);
  // The card's OWN record says which unit holds it. The caller's `unitKey`
  // comes from the click's mark list, which names the wrong unit when the
  // click landed in a widget next to a folded range, so it is only the
  // fallback for a card this scan cannot find.
  const owner = card ? card.unitKey : unitKey;
  // The popover shuts BEFORE the action runs. Every action either opens a
  // modal of its own, moves the cursor or rewrites the document, and a popover
  // still on screen over any of those is stale by definition.
  await closeMenu();
  switch (action) {
    case "copy-id":
      await copyText(atomId);
      return { ok: true, copied: atomId };
    case "copy-slug":
      await copyText(name);
      return { ok: true, copied: name };
    case "rename":
      return await renameAtomHere(atomId, slug);
    case "attrs":
      return await editAttrsHere(atomId);
    case "reveal":
      return await revealDirective(atomId);
    case "ungroup":
      if (!owner || owner.indexOf("group:") !== 0) {
        return { ok: false, error: "That card is not in a group" };
      }
      return await ungroupHere(owner.slice("group:".length));
    case "group":
      return await groupSelection();
  }
  return { ok: true };
}

/** Run one row of a group's popover. */
async function runGroupMenuAction(action, unitKey) {
  const groupId = unitKey && unitKey.indexOf("group:") === 0
    ? unitKey.slice("group:".length)
    : null;
  if (!groupId) return { ok: false, error: "No group under that menu" };
  await closeMenu();
  if (action === "ungroup-group") return await ungroupHere(groupId);
  if (action === "rename-group") return await renameGroupHere(groupId);
  return { ok: true };
}

/** Copies text, and says so, without letting a missing clipboard fail. */
async function copyText(value) {
  try {
    await syscall("editor.copyToClipboard", String(value == null ? "" : value));
    await warnUser('Copied "' + value + '"', "info");
  } catch (e) {
    await warnUser("Could not copy: " + e.message);
  }
}

/** Renames one atom from its own menu. */
async function renameAtomHere(atomId, currentSlug) {
  if (!atomId) return { ok: false, error: "This block has no atom directive" };
  const current = await syscall("editor.getText");
  const typed = await syscall(
    "editor.prompt",
    "Atom name",
    currentSlug || "",
  );
  if (typed === undefined) return { ok: true, cancelled: true };
  const result = setAtomSlugInSource(current, atomId, typed);
  if (!result.ok) {
    await warnUser(result.error);
    return result;
  }
  if (result.text === current) return { ok: true, unchanged: true };
  await applyEdit(current, result.text);
  await warnUser(result.warning);
  return { ok: true, slug: result.slug };
}

// ---------------------------------------------------------------------------
// THE ATTRIBUTE FORM (iugum-etz)
//
// WHERE IT LIVES, AND WHY IT IS NOT IN THE POPOVER. The seam's
// `widgetPressGuard` calls preventDefault on a plain mousedown inside any
// widget, so an input in the popover takes no focus from a click and the
// characters typed next go INTO THE DOCUMENT. That is measured, in
// `plugs/atomdown-e2e/input-probe.test.ts`, and rule 8g is the guard rail that
// says so. The popover still holds no input; 8g is unchanged.
//
// So the form needs a surface that CAN hold focus, and one already exists in
// this host: a SilverBullet panel. `editor.showPanel` renders its HTML in an
// iframe outside the editor's DOM entirely (silverbullet client/components/
// panel.tsx), the guard cannot reach into it, and it is the very surface the
// board panel's own attribute form runs in and has worked in all along. So
// this reuses a proven focusable surface and changes no vendored code.
//
// WHY NOT THE BOARD PANEL ITSELF, which was the recommendation to start from.
// Two measured costs. The board panel renders all 84 cards of the fixture and
// covers the whole window, so an attribute edit means leaving the page, losing
// the reading position and waiting for a full re-render; and reaching one
// card's popover from another plug needs a message into the board's iframe
// that does not exist yet, which is more new surface than this form is. This
// form is one atom's attributes and about 120 lines of panel script.
//
// WHY NOT `editor.prompt` PER FIELD. It is one field per modal, so it cannot
// present the slug first alongside the rest, and it has nowhere to put Add and
// Remove. The bead says so and the measurement agrees.
//
// THE PANEL SLOT IS "modal", the same slot the board uses. That is deliberate
// rather than an oversight: a modal panel is the one slot that does not resize
// the editor, so opening this form moves no card. The board occupies the same
// slot when it is open, and when it is open it covers the page, so no inline
// card is on screen to open this form from.
// ---------------------------------------------------------------------------

/** Which atom the form is open for, or null. Presentation state. */
let attrFormAtomId = null;

/** The theme tokens the panel iframe copies from the parent document. */
const ATTR_THEME_VARS = [
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
  "--link-color",
];

/**
 * The form's markup and its panel script, as a pure pair.
 *
 * `atom` is `{ id, slug, attrs }`, where `attrs` is every attribute the
 * directive line carries, in source order, INCLUDING id and slug: the form
 * decides how to present them rather than being handed a pre-filtered list, so
 * what it shows and what the file holds cannot disagree.
 *
 * THE SLUG IS FIRST AND LABELLED, matching the board exactly - same wording,
 * same placeholder shape - because a slug is the one attribute a human reads
 * and the two views may not disagree about it. The id is a disabled row: it is
 * identity, it is never editable, and hiding it would leave the reader with no
 * way to see which atom the form is for.
 */
function attrFormHtml(atom) {
  const id = String(atom && atom.id ? atom.id : "");
  const slug = String(atom && atom.slug ? atom.slug : "");
  const rows = ((atom && atom.attrs) || []).filter(function (a) {
    return a.name !== "id" && a.name !== "slug";
  });
  const data = JSON.stringify({ id: id, slug: slug, attrs: rows });
  const style = `
    :root {
      --root-background-color: #ffffff;
      --root-color: #37352f;
      --ui-surface-background-color: #ffffff;
      --ui-surface-border-color: #e9e9e7;
      --ui-surface-section-background-color: #f7f6f3;
      --ui-surface-hover-background-color: #f1f0ee;
      --ui-accent-color: #2383e2;
      --ui-accent-contrast-color: #ffffff;
      --subtle-color: #787774;
    }
    * { box-sizing: border-box; }
    html, body { margin: 0; padding: 0; height: 100%; }
    body {
      background: var(--root-background-color);
      color: var(--root-color);
      font-family: var(--attr-font-family, system-ui, sans-serif);
      font-size: 13px;
      display: flex;
      align-items: flex-start;
      justify-content: center;
      overflow: auto;
    }
    .ad-attr-form {
      width: min(560px, 100%);
      margin: 8vh 0 24px;
      padding: 16px;
      background: var(--ui-surface-background-color);
      border: 1px solid var(--ui-surface-border-color);
      border-radius: 8px;
    }
    .ad-attr-title { font-size: 15px; font-weight: 600; margin-bottom: 2px; }
    .ad-attr-sub {
      font-size: 11px;
      color: var(--subtle-color);
      font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
      margin-bottom: 12px;
    }
    .ad-attr-slug { margin-bottom: 12px; }
    .ad-attr-label {
      display: block;
      font-size: 11px;
      color: var(--subtle-color);
      margin-bottom: 3px;
    }
    .ad-attr-row { display: flex; gap: 6px; margin-bottom: 6px; }
    .ad-attr-name, .ad-attr-value, .ad-attr-slug-input {
      font-family: inherit;
      font-size: 13px;
      padding: 4px 6px;
      color: var(--root-color);
      background: var(--ui-surface-section-background-color);
      border: 1px solid var(--ui-surface-border-color);
      border-radius: 4px;
      min-width: 0;
    }
    .ad-attr-slug-input { width: 100%; }
    .ad-attr-name { flex: 0 0 34%; }
    .ad-attr-value { flex: 1 1 auto; }
    .ad-attr-name:disabled, .ad-attr-value:disabled { opacity: 0.6; }
    .ad-attr-remove {
      flex: 0 0 auto;
      font: inherit;
      cursor: pointer;
      color: var(--subtle-color);
      background: none;
      border: 1px solid transparent;
      border-radius: 4px;
      padding: 0 7px;
    }
    .ad-attr-remove:hover { color: var(--root-color); }
    .ad-attr-remove:disabled { visibility: hidden; }
    .ad-attr-actions { display: flex; gap: 6px; margin-top: 12px; }
    .ad-attr-actions button {
      font: inherit;
      cursor: pointer;
      padding: 5px 10px;
      color: var(--root-color);
      background: var(--ui-surface-section-background-color);
      border: 1px solid var(--ui-surface-border-color);
      border-radius: 4px;
    }
    .ad-attr-actions button:hover {
      background: var(--ui-surface-hover-background-color);
    }
    .ad-attr-save {
      margin-left: auto;
      font-weight: 600;
    }
    .ad-attr-status {
      min-height: 16px;
      margin-top: 8px;
      font-size: 12px;
      color: var(--subtle-color);
    }
  `;
  const html = "<style>" + style + "</style>" +
    '<div class="ad-attr-form" role="dialog" aria-label="Atomdown attributes">' +
    '<div class="ad-attr-title">Attributes</div>' +
    '<div class="ad-attr-sub" id="ad-attr-id">' + escapeHtml(id) + "</div>" +
    '<div class="ad-attr-slug">' +
    '<label class="ad-attr-label" for="ad-attr-slug-input">' +
    "Name (slug) - readable alias, not the id</label>" +
    '<input class="ad-attr-slug-input" id="ad-attr-slug-input" type="text" ' +
    'spellcheck="false" placeholder="unnamed - the card shows ' +
    escapeHtml(id) + '" value="' + escapeHtml(slug) + '">' +
    "</div>" +
    '<label class="ad-attr-label">Attributes</label>' +
    '<div class="ad-attr-list" id="ad-attr-list"></div>' +
    '<div class="ad-attr-actions">' +
    '<button type="button" class="ad-attr-add" id="ad-attr-add">+ Add attribute</button>' +
    '<button type="button" class="ad-attr-cancel" id="ad-attr-cancel">Cancel</button>' +
    '<button type="button" class="ad-attr-save" id="ad-attr-save">Save</button>' +
    "</div>" +
    '<div class="ad-attr-status" id="ad-attr-status"></div>' +
    "</div>";

  const script = `
    var DATA = ${data};
    var THEME_VARS = ${JSON.stringify(ATTR_THEME_VARS)};

    // The theme tokens live on the PARENT document's <html> and custom
    // properties do not cross an iframe boundary. This panel is srcDoc and
    // same-origin, so read the parent's computed values and copy them, the
    // same road the board panel's applyParentTheme() uses. On failure the
    // :root fallbacks above are a light-theme snapshot, never a dark guess.
    function applyParentTheme() {
      try {
        var pd = window.parent && window.parent.document;
        if (!pd || !pd.documentElement) return;
        var cs = window.parent.getComputedStyle(pd.documentElement);
        THEME_VARS.forEach(function (n) {
          var v = cs.getPropertyValue(n);
          if (v && v.trim()) {
            document.documentElement.style.setProperty(n, v.trim());
          }
        });
        var font = cs.getPropertyValue("--editor-font").trim();
        if (!font) {
          var ed = pd.querySelector("#sb-editor .cm-content");
          if (ed) font = window.parent.getComputedStyle(ed).fontFamily;
        }
        if (font && !/^Times\\b/.test(font)) {
          document.documentElement.style.setProperty("--attr-font-family", font);
        }
      } catch (e) {}
    }
    applyParentTheme();
    window.addEventListener("message", function (e) {
      if (e.data && e.data.type === "theme") applyParentTheme();
    });

    var listEl = document.getElementById("ad-attr-list");
    var slugEl = document.getElementById("ad-attr-slug-input");
    var statusEl = document.getElementById("ad-attr-status");

    function addRow(name, value, locked) {
      var row = document.createElement("div");
      row.className = "ad-attr-row";
      var n = document.createElement("input");
      n.className = "ad-attr-name";
      n.type = "text";
      n.spellcheck = false;
      n.placeholder = "name";
      n.value = name || "";
      var v = document.createElement("input");
      v.className = "ad-attr-value";
      v.type = "text";
      v.spellcheck = false;
      v.placeholder = "value";
      v.value = value == null ? "" : value;
      var rm = document.createElement("button");
      rm.className = "ad-attr-remove";
      rm.type = "button";
      rm.textContent = "\\u00d7";
      rm.title = "Remove this attribute";
      if (locked) {
        n.disabled = true;
        v.disabled = true;
        rm.disabled = true;
      }
      rm.addEventListener("click", function () { row.remove(); });
      row.appendChild(n);
      row.appendChild(v);
      row.appendChild(rm);
      listEl.appendChild(row);
      return row;
    }

    // The id row is first and disabled: identity is shown and never editable.
    addRow("id", DATA.id, true);
    DATA.attrs.forEach(function (a) { addRow(a.name, a.value, false); });

    document.getElementById("ad-attr-add").addEventListener("click", function () {
      var row = addRow("", "", false);
      row.querySelector(".ad-attr-name").focus();
    });

    function close() {
      syscall("system.invokeFunction", "atomdown-inline.closeAttrForm");
    }
    document.getElementById("ad-attr-cancel").addEventListener("click", close);
    document.addEventListener("keydown", function (e) {
      if (e.key === "Escape") close();
    });

    document.getElementById("ad-attr-save").addEventListener("click", async function () {
      var attrs = [];
      Array.prototype.slice.call(listEl.querySelectorAll(".ad-attr-row"))
        .forEach(function (row) {
          var n = row.querySelector(".ad-attr-name");
          var v = row.querySelector(".ad-attr-value");
          // The id row travels with the source line, never through here.
          if (n.disabled) return;
          attrs.push({ name: n.value, value: v.value });
        });
      // Slug FIRST, so the rewritten directive reads id, slug, then the rest.
      // The worker sanitizes it and drops it when it is empty.
      attrs.unshift({ name: "slug", value: slugEl.value });
      statusEl.textContent = "Saving...";
      try {
        var result = await syscall(
          "system.invokeFunction",
          "atomdown-inline.saveAttrs",
          DATA.id,
          JSON.stringify(attrs)
        );
        if (result && result.ok) {
          statusEl.textContent = "Saved.";
          close();
        } else {
          statusEl.textContent =
            (result && result.error) || "Save failed";
        }
      } catch (e) {
        statusEl.textContent = "Save failed: " + e.message;
      }
    });

    // FOCUS THE SLUG FIELD, and this is the whole reason the form is in a
    // panel. In the popover a click focused nothing and the keystrokes went
    // into the document; here the field takes focus on open.
    slugEl.focus();
    slugEl.select();
  `;
  return { html: html, script: script };
}

/**
 * "Edit attributes" for one atom: open the form.
 *
 * Reads the atom's directive line fresh rather than trusting whatever the
 * popover was drawn from, the same "re-read, do not trust the client" rule
 * every write path here uses.
 */
async function editAttrsHere(atomId) {
  if (!atomId) return { ok: false, error: "This block has no atom directive" };
  const text = await syscall("editor.getText");
  const lines = String(text || "").split("\n");
  let attrs = null;
  for (let i = 0; i < lines.length; i++) {
    const m = lines[i].match(ATOM_TAG_RE);
    if (!m) continue;
    const parsed = parseAttrs(m[2]);
    const idAttr = parsed.find(function (a) { return a.name === "id"; });
    if (idAttr && idAttr.value === atomId) {
      attrs = parsed;
      break;
    }
  }
  if (!attrs) {
    const message =
      "Could not find this atom's directive (implicit atom, or the page changed)";
    await warnUser(message);
    return { ok: false, error: message };
  }
  const slugAttr = attrs.find(function (a) { return a.name === "slug"; });
  const form = attrFormHtml({
    id: atomId,
    slug: slugAttr ? slugAttr.value : "",
    attrs: attrs,
  });
  attrFormAtomId = atomId;
  await syscall("editor.showPanel", "modal", 0, form.html, form.script);
  return { ok: true, id: atomId };
}

/** Shut the attribute form. Called by its Cancel button and after a save. */
async function closeAttrForm() {
  attrFormAtomId = null;
  try {
    await syscall("editor.hidePanel", "modal");
  } catch (e) {
    // Nothing to hide is not a failure.
  }
  return { ok: true };
}

/**
 * The form's Save. ONE editor transaction, so one undo reverts the whole save
 * however many attributes it changed.
 */
async function saveAttrs(atomId, attrsJson) {
  let requested;
  try {
    requested = JSON.parse(attrsJson);
  } catch (e) {
    return { ok: false, error: "Invalid attribute payload" };
  }
  const current = await syscall("editor.getText");
  const result = setAtomAttrsInSource(current, atomId, requested);
  if (!result.ok) return result;
  if (result.unchanged) return { ok: true, unchanged: true };
  await applyEdit(current, result.text);
  await warnUser(result.warning);
  return { ok: true, slug: result.slug };
}

/**
 * Puts the cursor on one atom's directive line.
 *
 * That is the inline answer to "edit attributes": the line IS the attributes,
 * and the cursor being there is exactly the condition that reveals the peek,
 * so the reader can see and edit the bytes with no form in between.
 */
async function revealDirective(atomId) {
  if (!atomId) return { ok: false, error: "This block has no atom directive" };
  const text = await syscall("editor.getText");
  const scan = computeUnits(text);
  const starts = lineStarts(scan.lines);
  for (let i = 0; i < scan.lines.length; i++) {
    const m = scan.lines[i].match(ATOM_TAG_RE);
    if (!m) continue;
    const idAttr = parseAttrs(m[2]).find(function (a) { return a.name === "id"; });
    if (idAttr && idAttr.value === atomId) {
      await syscall("editor.moveCursor", starts[i]);
      try {
        await syscall("editor.focus");
      } catch (e) {
        // Focus is what reveals the peek, but not being able to ask for it is
        // not a reason to fail the action.
      }
      return { ok: true, line: i + 1 };
    }
  }
  return { ok: false, error: "Could not find that atom's directive" };
}

async function renameGroupHere(groupId) {
  const current = await syscall("editor.getText");
  const unit = computeUnits(current).units.find(function (u) {
    return u.unitKey === "group:" + groupId;
  });
  const typed = await syscall(
    "editor.prompt",
    "Group name",
    unit && unit.groupSlug ? unit.groupSlug : "",
  );
  if (typed === undefined) return { ok: true, cancelled: true };
  const result = setGroupSlugInSource(current, groupId, typed);
  if (!result.ok) {
    await warnUser(result.error);
    return result;
  }
  if (result.text === current) return { ok: true, unchanged: true };
  await applyEdit(current, result.text);
  await warnUser(result.warning);
  return { ok: true, slug: result.slug };
}

async function ungroupHere(groupId) {
  const current = await syscall("editor.getText");
  const result = removeGroupMarkers(current, groupId);
  if (!result.ok) {
    await warnUser(result.error);
    return result;
  }
  selectedUnitKeys = selectedUnitKeys.filter(function (key) {
    return key !== "group:" + groupId;
  });
  await applyEdit(current, result.text);
  return { ok: true };
}

/**
 * "Atomdown: Group Selection" — wraps the lassoed blocks in one atom-group.
 *
 * Only the two marker lines are added. No block's text moves, so no atom's
 * `digest` can go stale, and no atom's directive line is rewritten, so every
 * `id` and every extension attribute survives byte for byte.
 */
async function groupSelection() {
  const current = await currentUnitOrder();
  const state = menuState(current.order, selectedUnitKeys, null);
  if (!state.enabled) {
    await warnUser(state.reason);
    return { ok: false, error: state.reason };
  }
  const keys = dedupeKeys(selectedUnitKeys);

  const scan = computeUnits(current.text);
  const texts = keys.map(function (key) {
    const unit = scan.units.find(function (u) { return u.unitKey === key; });
    if (!unit) return "";
    return scan.lines.slice(unit.startLine, unit.endLine + 1).join("\n");
  });
  const suggested = deriveGroupSlug(texts);
  const typed = await syscall("editor.prompt", "Group name", suggested);
  if (typed === undefined) return { ok: true, cancelled: true };

  const used = existingIds(current.text);
  let groupId = newAtomdownId();
  for (let attempt = 0; attempt < 32 && used.indexOf(groupId) !== -1; attempt++) {
    groupId = newAtomdownId();
  }

  const result = insertGroupMarkers(current.text, keys, groupId, typed);
  if (!result.ok) {
    await warnUser(result.error);
    return result;
  }
  selectedUnitKeys = ["group:" + groupId];
  selectionAnchorKey = "group:" + groupId;
  await applyEdit(current.text, result.text);
  await warnUser(result.warning);
  return { ok: true, groupId, slug: result.slug };
}

/**
 * "Atomdown: Ungroup" — removes the markers of the group the cursor is in.
 * Every atom that was inside keeps its position, its directive, its id and its
 * digest.
 */
async function ungroupSelection() {
  const current = await syscall("editor.getText");
  const cursor = await syscall("editor.getCursor");
  const scan = computeUnits(current);
  const starts = lineStarts(scan.lines);
  const unit = scan.units.find(function (u) {
    if (u.kind !== "group") return false;
    const from = starts[u.startLine];
    const to = starts[u.endLine] + scan.lines[u.endLine].length;
    return cursor >= from && cursor <= to;
  });
  if (!unit) {
    const message = "Put the cursor inside a group first.";
    await warnUser(message);
    return { ok: false, error: message };
  }
  return await ungroupHere(unit.groupId);
}

const functionMapping = {
  toggleInline,
  toggleDensity,
  registerActionButtons,
  heartbeatActionButtons,
  restoreInline,
  refreshInline,
  onDecorationClick,
  onDecorationSelect,
  onDecorationDrag,
  onDecorationLasso,
  groupSelection,
  ungroupSelection,
  saveAttrs,
  closeAttrForm,
};

const manifest = {
  name: "atomdown-inline",
  version: 0.1,
  functions: {
    toggleInline: {
      path: "./atomdown-inline.js:toggleInline",
      command: { name: "Atomdown: Toggle Inline View" },
    },
    toggleDensity: {
      path: "./atomdown-inline.js:toggleDensity",
      command: { name: "Atomdown: Toggle Inline Density" },
    },
    // The header-bar buttons. `editor:init` and not `plugs:loaded`: the boot
    // sequence loads plugs, dispatches `plugs:loaded`, THEN loads Space Lua —
    // and loading Space Lua calls `Config.clear()`, so a button registered on
    // the earlier event is wiped before the header bar ever reads the key
    // (silverbullet/client/client.ts, `init`).
    registerActionButtons: {
      path: "./atomdown-inline.js:registerActionButtons",
      events: ["editor:init"],
    },
    // The self-healing half. `editor:reloadState` is deliberately NOT the
    // event here: the client queues plug handlers and its own local listeners
    // as concurrent promises, so this plug's first syscall and that handler's
    // `config.clear()` race, and the clear wins.
    heartbeatActionButtons: {
      path: "./atomdown-inline.js:heartbeatActionButtons",
      events: ["cron:secondPassed"],
    },
    // Both events matter: pageLoaded fires for a browser reload and for
    // navigating to another page, pageReloaded for reloading the page already
    // open (silverbullet client/content_manager.ts).
    restoreInline: {
      path: "./atomdown-inline.js:restoreInline",
      events: ["editor:pageLoaded", "editor:pageReloaded"],
    },
    refreshInline: {
      path: "./atomdown-inline.js:refreshInline",
      events: ["editor:pageSaved"],
    },
    onDecorationClick: {
      path: "./atomdown-inline.js:onDecorationClick",
      events: ["editor:decorationClick"],
    },
    onDecorationSelect: {
      path: "./atomdown-inline.js:onDecorationSelect",
      events: ["editor:decorationSelect"],
    },
    onDecorationDrag: {
      path: "./atomdown-inline.js:onDecorationDrag",
      events: ["editor:decorationDrag"],
    },
    onDecorationLasso: {
      path: "./atomdown-inline.js:onDecorationLasso",
      events: ["editor:decorationLasso"],
    },
    groupSelection: {
      path: "./atomdown-inline.js:groupSelection",
      command: { name: "Atomdown: Group Selection" },
    },
    ungroupSelection: {
      path: "./atomdown-inline.js:ungroupSelection",
      command: { name: "Atomdown: Ungroup" },
    },
    // The attribute form's two callbacks. No command: the form is reached
    // from a card's own menu, and its panel script invokes these by name.
    saveAttrs: {
      path: "./atomdown-inline.js:saveAttrs",
    },
    closeAttrForm: {
      path: "./atomdown-inline.js:closeAttrForm",
    },
  },
};

// Test-only surface. atomdown-inline.test.mjs imports this to unit-test the
// pure functions directly. Nothing in SilverBullet reads it.
const internals = {
  computeUnits,
  computeCards,
  reorderUnit,
  insertGroupMarkers,
  removeGroupMarkers,
  setGroupSlugInSource,
  setAtomSlugInSource,
  setAtomAttrsInSource,
  attrFormHtml,
  serializeAtomLine,
  firstBoxKey,
  widgetUnitKey,
  widgetBoxKey,
  cardMenuItems,
  groupMenuItems,
  cardOpenKey,
  menuPopoverHtml,
  menuActionFromClasses,
  menuOpenForCard,
  menuOpenForGroup,
  MENU_GLYPH,
  removeLineCollapsingSeam,
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
  ACTION_BUTTONS,
  shouldReassertButtons,
  BUTTON_CHECK_TICKS,
  buildDecorations,
  emptyDecorations,
  firstUnitKey,
  dragToReorder,
  lassoToUnitKeys,
  menuState,
  inlineOnKey,
  groupHeaderHtml,
  gripHtml,
  escapeHtml,
};

const plugExport = { manifest, functionMapping, internals };

wireWorker(functionMapping, manifest, self.postMessage);

export { plugExport as plug };
