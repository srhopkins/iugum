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
        cardKey: "card:" + unit.groupId + ":" + index,
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

/** The drag handle, the same six-dot glyph and hover rule the board uses. */
function gripHtml(title) {
  return '<span class="atomdown-grip" title="' + escapeHtml(title) +
    '">&#10303;</span>';
}

/**
 * A card's header row: the grip, the readable name, and the id.
 *
 * This is the board's card header, and it exists inline for a second reason:
 * the directive is hidden at rest, so with no header row the atom's id would
 * have nowhere at all to appear. Name in body text, id in small grey
 * monospace, in that order - identity stays visible, the name reads first.
 */
function cardHeaderHtml(unit, nested) {
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
  return '<span class="atomdown-card-head' + (nested ? " atomdown-nested" : "") +
    '">' +
    gripHtml(
      unit.implicit
        ? "Drag to move this block"
        : "Drag to move " + slugOrId(slug, id),
    ) +
    name + idLabel + "</span>";
}

/**
 * The group header bar: collapse, grip, the kind, the group's readable name,
 * its id, how many atoms it holds, and a menu.
 *
 * The bar is also the group box's TOP EDGE. The group's opening marker line is
 * a directive and therefore collapsed, so it can carry no visible border, and
 * this widget sits directly above it. That is why the accent border-top and
 * the top corner radii are on this element in the stylesheet.
 *
 * Every control carries its own class. The seam reports the classes of the
 * element that was clicked, so one widget can carry several controls without
 * needing a widget each.
 */
function groupHeaderHtml(unit, memberCount) {
  const name = slugOrId(unit.groupSlug, unit.groupId);
  const word = memberCount === 1 ? "card" : "cards";
  return [
    '<span class="atomdown-group-collapse" title="Collapse or expand this group">&#9662;</span>',
    '<span class="atomdown-grip atomdown-group-grip" title="Drag to move the whole group">&#10303;</span>',
    '<span class="atomdown-group-kind">group</span>',
    '<span class="atomdown-group-name" title="' +
    escapeHtml(
      'Name (slug) "' + name + '" - the group\'s id is ' + unit.groupId,
    ) + '">' + escapeHtml(name) + "</span>",
    '<span class="atomdown-group-id" title="' +
    escapeHtml("Atomdown id " + unit.groupId) + '">' +
    escapeHtml(String(unit.groupId)) + "</span>",
    '<span class="atomdown-group-count">' + memberCount + " " + word +
    "</span>",
    '<span class="atomdown-group-actions">' +
    '<span class="atomdown-group-btn atomdown-group-rename" title="' +
    escapeHtml(
      "Give this group a readable name. Its id (" + unit.groupId +
      ") does not change - a name is an alias, not the identity.",
    ) + '">Rename</span>' +
    '<span class="atomdown-group-btn atomdown-group-ungroup" title="Remove this group\'s markers. Every atom inside it stays.">Ungroup</span>' +
    "</span>",
  ].join("");
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
 */
function buildDecorations(sourceText, selectedKeys) {
  const scan = computeCards(sourceText);
  const lines = scan.lines;
  const starts = lineStarts(lines);
  const selected = dedupeKeys(selectedKeys);
  const marks = [];
  const widgets = [];
  const folds = [];

  function span(startLine, endLine) {
    return {
      from: starts[startLine],
      to: starts[endLine] + lines[endLine].length,
    };
  }

  /** One atom's card: the box mark and the header row above it. */
  function addCard(unit, boxKey, nested) {
    if (hasNoContent(unit, lines)) return null;
    const box = span(contentFirstLine(unit), unit.endLine);
    marks.push({
      id: boxKey,
      from: box.from,
      to: box.to,
      class: "atomdown-card",
      lineClasses: true,
    });
    widgets.push({
      id: boxKey,
      at: box.from,
      side: "before",
      class: "atomdown-card-header" + (nested ? " atomdown-nested" : ""),
      html: cardHeaderHtml(unit, nested),
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
      });
      widgets.push({
        id: "unit:" + unit.unitKey,
        at: unitSpan.from,
        side: "before",
        class: "atomdown-group-header",
        html: groupHeaderHtml(unit, members.length),
      });
      const openLineEnd = starts[unit.startLine] + lines[unit.startLine].length;
      if (unitSpan.to > openLineEnd) {
        folds.push({ from: openLineEnd, to: unitSpan.to });
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
          card.cardKey,
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

/** The first `unit:` name in a seam mark list, as a unit key. */
function firstUnitKey(names) {
  const found = (names || []).find(function (name) {
    return typeof name === "string" && name.indexOf("unit:") === 0;
  });
  return found ? found.slice("unit:".length) : null;
}

/**
 * Turns one `editor:decorationDrag` payload into a reorder request.
 *
 * The origin is whichever unit mark covered the handle. The target is
 * whichever unit mark covered the release point; a release outside every unit
 * (the document marker, or the blank space past the last block) becomes a drop
 * at the start or the end of the document, decided by which end of the page
 * the pointer was nearer.
 *
 * Returns null when the gesture asks for nothing: no origin, or a drop on the
 * unit that was picked up.
 */
function dragToReorder(event, unitOrder) {
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
  const movedIndex = (unitOrder || []).indexOf(moved);
  const atTop = event.placement === "before" && movedIndex !== 0;
  return {
    movedUnitKey: moved,
    targetUnitKey: null,
    placement: atTop ? "start" : "end",
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
/** Groups this session folded from the header caret. Never persisted. */
const collapsedGroups = new Set();

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
    buildDecorations(text, selectedUnitKeys),
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
  await syscall(
    "config.set",
    "editorDecorations",
    buildDecorations(newText, selectedUnitKeys),
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
  if (await isInlineOn(page)) {
    await rememberInlineOn(page, false);
    await clearDecorations(true);
    return { ok: true, on: false };
  }
  const text = await syscall("editor.getText");
  await rememberInlineOn(page, true);
  activePage = page;
  selectedUnitKeys = [];
  await writeDecorations(text, true);
  return { ok: true, on: true };
}

/**
 * Restores the view after a page load, but ONLY on a page it was left on.
 *
 * A page whose key was never written gets nothing. This is the answer to
 * "every refresh and I have to switch the view back on", and it is not a
 * default: the key is per page, and turning the view off deletes it.
 */
async function restoreInline(pageName) {
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

/** The unit order of the page as it is now, for the contiguity rule. */
async function currentUnitOrder() {
  const text = await syscall("editor.getText");
  return {
    text,
    order: computeUnits(text).units.map(function (unit) {
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

  if (classes.indexOf("atomdown-group-collapse") !== -1) {
    return await collapseGroup(firstUnitKey(event.marks));
  }

  if (classes.indexOf("atomdown-group-rename") !== -1) {
    const key = firstUnitKey(event.marks);
    return key && key.indexOf("group:") === 0
      ? await renameGroupHere(key.slice("group:".length))
      : { ok: false, error: "No group under that button" };
  }

  if (classes.indexOf("atomdown-group-ungroup") !== -1) {
    const key = firstUnitKey(event.marks);
    return key && key.indexOf("group:") === 0
      ? await ungroupHere(key.slice("group:".length))
      : { ok: false, error: "No group under that button" };
  }

  if (classes.indexOf("atomdown-group-menu") !== -1) {
    return await openGroupMenu(firstUnitKey(event.marks));
  }

  const unitKey = firstUnitKey(event.marks);
  if (!unitKey) return { ok: true };
  if (event.metaKey || event.ctrlKey) {
    // Add to, or remove from, the selection.
    const at = selectedUnitKeys.indexOf(unitKey);
    if (at === -1) selectedUnitKeys = selectedUnitKeys.concat([unitKey]);
    else selectedUnitKeys = selectedUnitKeys.filter(function (_, i) {
      return i !== at;
    });
  } else if (selectedUnitKeys.length > 0) {
    selectedUnitKeys = [];
  } else {
    return { ok: true };
  }
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
  const request = dragToReorder(event, current.order);
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
 * The cursor goes to the group's OPENING MARKER LINE, computed from the page
 * text, not to wherever the pointer landed. The header bar is a block widget
 * above that line, so a click in it reports a position on the line before the
 * group, and folding there would fold the wrong thing or nothing. The seam's
 * `folds` entry makes exactly that one line foldable; `editor.fold` and
 * `editor.unfold` then do the collapsing, so there is no collapse state in
 * this plug and none in the document.
 */
async function collapseGroup(unitKey) {
  const groupId = unitKey && unitKey.indexOf("group:") === 0
    ? unitKey.slice("group:".length)
    : null;
  if (!groupId) return { ok: false, error: "No group under that control" };
  const text = await syscall("editor.getText");
  const scan = computeUnits(text);
  const unit = scan.units.find(function (u) {
    return u.unitKey === "group:" + groupId;
  });
  if (!unit) return { ok: false, error: "Could not find that group" };
  const starts = lineStarts(scan.lines);
  await syscall("editor.moveCursor", starts[unit.startLine]);
  // The host offers fold and unfold but no read of the fold state, so which
  // way the caret goes is remembered here. It is not persisted: a reload draws
  // every group open, which is the truthful state after a rebuild.
  const collapsing = !collapsedGroups.has(groupId);
  try {
    await syscall(collapsing ? "editor.fold" : "editor.unfold");
  } catch (e) {
    await warnUser("Could not collapse that group: " + e.message);
    return { ok: false, error: e.message };
  }
  if (collapsing) collapsedGroups.add(groupId);
  else collapsedGroups.delete(groupId);
  return { ok: true, collapsed: collapsing, groupId };
}

/**
 * The group header's menu, shown in SilverBullet's own filterable picker
 * rather than in a popover of this plug's own. A widget carries HTML with no
 * script, so a hand-built menu inside one could not react to a click anyway,
 * and the host's picker already handles the keyboard and the theme.
 */
async function openGroupMenu(unitKey) {
  const groupId = unitKey && unitKey.indexOf("group:") === 0
    ? unitKey.slice("group:".length)
    : null;
  if (!groupId) return { ok: false, error: "No group under that menu" };
  const choice = await syscall(
    "editor.filterBox",
    "Group",
    [
      { name: "Rename group", description: "Change this group's readable name" },
      { name: "Ungroup", description: "Remove the two markers; every atom stays" },
    ],
    "Group " + groupId,
  );
  if (!choice) return { ok: true, cancelled: true };
  if (choice.name === "Ungroup") return await ungroupHere(groupId);
  return await renameGroupHere(groupId);
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
  restoreInline,
  refreshInline,
  onDecorationClick,
  onDecorationSelect,
  onDecorationDrag,
  onDecorationLasso,
  groupSelection,
  ungroupSelection,
};

const manifest = {
  name: "atomdown-inline",
  version: 0.1,
  functions: {
    toggleInline: {
      path: "./atomdown-inline.js:toggleInline",
      command: { name: "Atomdown: Toggle Inline View" },
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
  lineStarts,
  gripLine,
  contentFirstLine,
  hasNoContent,
  cardHeaderHtml,
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
