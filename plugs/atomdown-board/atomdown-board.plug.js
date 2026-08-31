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
        <div class="board-card-header">
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
    body {
      margin: 0;
      padding: 0;
      font-family: system-ui, -apple-system, sans-serif;
      background: var(--root-background-color, #1e1e1e);
      color: var(--root-color, #ddd);
    }
    .board-toolbar {
      display: flex;
      align-items: center;
      justify-content: space-between;
      padding: 10px 16px;
      border-bottom: 1px solid rgba(128,128,128,0.3);
      position: sticky;
      top: 0;
      background: inherit;
      z-index: 20;
    }
    .board-title { font-weight: 600; font-size: 14px; }
    .board-close {
      cursor: pointer;
      border: 1px solid rgba(128,128,128,0.5);
      background: transparent;
      color: inherit;
      border-radius: 4px;
      padding: 4px 12px;
      font-size: 13px;
    }
    .board-close:hover { background: rgba(128,128,128,0.2); }
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
      border: 1px solid rgba(128,128,128,0.4);
      border-radius: 6px;
      background: rgba(128,128,128,0.08);
      display: flex;
      flex-direction: column;
      min-height: 60px;
      width: 100%;
    }
    .board-card-implicit { border-style: dashed; }
    .board-card-grouped { border-left: 3px solid #7aa2f7; }
    .board-card-header {
      display: flex;
      align-items: center;
      gap: 6px;
      flex-wrap: wrap;
      padding: 6px 8px;
      border-bottom: 1px solid rgba(128,128,128,0.3);
      position: relative;
    }
    .board-card-id {
      font-family: ui-monospace, monospace;
      font-size: 11px;
      opacity: 0.7;
    }
    .board-badge {
      font-size: 10px;
      padding: 1px 6px;
      border-radius: 999px;
      background: rgba(128,128,128,0.25);
    }
    .board-badge-group { background: rgba(122,162,247,0.25); }
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
    .board-menu-btn:hover { background: rgba(128,128,128,0.25); }
    .board-menu-popover {
      position: absolute;
      top: 100%;
      right: 0;
      z-index: 30;
      background: var(--root-background-color, #2a2a2a);
      border: 1px solid rgba(128,128,128,0.4);
      border-radius: 6px;
      padding: 8px;
      min-width: 240px;
      box-shadow: 0 4px 12px rgba(0,0,0,0.35);
    }
    .board-menu-popover[hidden] { display: none; }
    .board-menu-title {
      font-size: 12px;
      font-weight: 600;
      margin-bottom: 6px;
      opacity: 0.8;
    }
    .board-attr-empty { font-size: 12px; opacity: 0.7; padding: 4px 0; }
    .board-attr-row { display: flex; gap: 4px; margin-bottom: 4px; }
    .board-attr-name, .board-attr-value {
      flex: 1;
      min-width: 0;
      font-size: 12px;
      padding: 3px 5px;
      background: rgba(128,128,128,0.15);
      border: 1px solid rgba(128,128,128,0.3);
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
      border: 1px solid rgba(128,128,128,0.5);
      background: transparent;
      color: inherit;
      border-radius: 4px;
      padding: 3px 10px;
    }
    .board-attr-add:hover, .board-attr-save:hover { background: rgba(128,128,128,0.2); }
    .board-menu-status { font-size: 11px; margin-top: 4px; opacity: 0.8; min-height: 14px; }
  `;

  const html = `
    <style>${style}</style>
    <div class="board-toolbar">
      <div class="board-title">Atomdown Board${pageName ? " — " + escapeHtml(pageName) : ""}</div>
      <button class="board-close" id="atomdown-board-close">Close</button>
    </div>
    <div class="board-cards">${cardsHtml || "<p style=\"padding:16px;opacity:0.7;\">No atoms found in this document.</p>"}</div>
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
  `;

  const clientData = atoms.map((atom) => ({
    id: atom.id,
    implicit: atom.implicit,
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

  // A small inset keeps this close to full-viewport (see
  // client/styles/main.scss .sb-modal, which insets a fixed backdrop).
  await syscall("editor.showPanel", "modal", 24, html, script);
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

const functionMapping = { toggleBoard, notifyClosed, saveAttrs };

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
  },
};

const plugExport = { manifest, functionMapping };

wireWorker(functionMapping, manifest, self.postMessage);

export { plugExport as plug };
