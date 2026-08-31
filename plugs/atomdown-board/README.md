# atomdown-board (spike)

A SilverBullet plug that opens a full-screen modal showing one card per
Atomdown atom in the current document, and lets you view and edit the
generic XML attributes on each atom's directive. Built for bead
`iugum-w6y` ("SilverBullet board view for Atomdown documents").

This is a spike. It proves the rendering and edit-write surfaces work
end to end. It is **not** the finished board — see "What is NOT
implemented" below.

## What it does

1. Run the command **"Atomdown: Toggle Board"** (Command Palette, or bind
   a key the way `treeview.plug.yaml` binds `Ctrl-alt-b`/`Cmd-alt-b` —
   this plug does not bind a key by default).
2. A full-screen modal opens over the current page, with one card per
   Atomdown atom found in the document (see "How parsing works" below).
   Each card shows the atom's id and its raw Markdown text.
3. Click the three-dot (`⋮`) menu on a card to see every XML attribute
   present on that atom's directive, as plain name/value pairs — whatever
   they happen to be. There is no list of known attribute names anywhere
   in this plug; it renders and edits generically, the same way Atomdown
   Core itself preserves attributes without interpreting them (see
   `atomdown/SPEC.md`, "Extensions").
4. In that menu you can change a value, add a new name/value pair, or
   remove one, then click **Save**. Saving rewrites *only* that atom's
   one directive line in the document (see "How the write path works"),
   and leaves everything else byte-identical.
5. Closing the modal (the **Close** button, or running the toggle command
   again) returns to the normal page. The document is otherwise
   unchanged unless you explicitly clicked Save on an attribute edit.

The `id` attribute is shown but is never editable and can never be
removed — Atomdown Core requires every atom to have one
(`SPEC.md`, "Identity"). That is the one name this plug treats
specially, and it is a Core structural rule, not a domain attribute.

## How parsing works

`atomdown-board.plug.js` does a straightforward line-based scan of the
document text (see `parseAtoms()`), per `atomdown/SPEC.md`:

- A line matching `<!-- <atom id="..." .../> -->` starts a new atom. All
  attributes on that tag are captured generically.
- A line matching `<!-- <atom-group id="..."> -->` / `<!-- </atom-group>
  -->` tags every atom between the markers with that group's id (shown
  as a badge on the card). Groups do not get their own card.
- Any blank-line-delimited block that has no atom marker in front of it
  (for example a `---` divider) becomes an "implicit atom" card, per
  SPEC.md: "A tool must not discard the block or attach it to the
  previous atom." Implicit atoms have no directive, so their attribute
  menu is empty and read-only — there is nothing to edit.

This is not a full parser: it does not validate group balance, id
uniqueness, or nesting, and it cannot shell out to the real `atomdown`
binary (a browser worker cannot run a subprocess). It was verified
against the real test document (see "What was verified" below), not
just synthetic input.

## How the write path works

Saving an attribute edit calls the plug's `saveAttrs(atomId, attrsJson)`
function, which:

1. Re-reads the current page text fresh via `space.readPage`.
2. Finds that one atom's `<atom .../>` directive line by id.
3. Rebuilds just that line from the id (always first, always preserved)
   plus the attribute list you edited in the panel — every attribute you
   did not touch is still there, because the panel starts pre-populated
   with all of them and only drops what you explicitly remove.
4. Writes the line back into the full document text via
   `space.writePage`, and reloads the live editor buffer via
   `editor.reloadPage` so the page behind the modal doesn't show stale
   content — the rest of the document is untouched.

The directive is always rebuilt onto one source line, since `SPEC.md`
requires that ("Each directive must occupy one source line") and the
`inline-directive` lint diagnostic (`atomdown/parser.go`) enforces it.

## Build

There is no build step. This plug is hand-authored JavaScript, not the
output of `plug-compile`/esbuild — see "Why hand-authored" below.
`atomdown-board.plug.js` is what SilverBullet loads directly.

`atomdown-board.plug.yaml` is a logical manifest for humans (and for a
future real `plug-compile`, if that ever becomes available on this
machine). It is **not** read by SilverBullet at runtime — keep it in
sync by hand with the `manifest` object at the bottom of
`atomdown-board.plug.js` whenever you add or rename a function.

### Why hand-authored

The vendored SilverBullet tree's `plug-compile` path needs a `silverbullet`
CLI this machine doesn't have (pinned to a legacy release), and the
vendored tree's own Node tooling has no `node_modules` — and `iugum`'s
`AGENTS.md` forbids editing `silverbullet/` or running `npm install`
there. So this plug follows the same hand-authored pattern Steve already
uses for `~/projects/tools/silverbullet-treeview`'s local fork (see that
repo's `FORK.md` and `scripts/patch-plug-js.py`): write the compiled
worker-bundle shape directly. The runtime shim at the top of
`atomdown-board.plug.js` (postMessage wiring, `syscall()`) is copied
functionally from the already-installed `mermaid.plug.js` bundle in this
space, which is the smallest real example of that shape.

## Install

Copy the built file into a SilverBullet space's `_plug/` directory,
additively — do not touch the other files there:

```sh
cp atomdown-board.plug.js /path/to/space/_plug/atomdown-board.plug.js
```

It has already been installed this way into the FFAI space at
`~/projects/github/FutureFit-ai/_silverbullet/_plug/atomdown-board.plug.js`.
SilverBullet auto-loads every `*.plug.js` file it finds in the space
(`client/space.ts` `listPlugs()`); nothing needs to be added to
`CONFIG.md` for it to load. Then in SilverBullet: **System: Reload** (or
hard-refresh the browser tab).

## The `Plugs: Update` hazard — read this before running that command

SilverBullet's **Plugs: Update** command re-fetches every plug listed in
a space's `CONFIG.md` `plugs = { ... }` list from GitHub and **overwrites**
the corresponding local file in `_plug/`. Steve's
`~/projects/tools/silverbullet-treeview/FORK.md` documents hitting this
with the treeview fork.

This plug is **not published to GitHub** and is **not** listed in either
space's `CONFIG.md`. As long as it stays out of that list, `Plugs:
Update` should leave `atomdown-board.plug.js` alone — but if anyone
ever adds a `"github:.../atomdown-board.plug.js"` entry to `CONFIG.md`
for convenience, the next `Plugs: Update` will destroy this hand-built
copy and there is no upstream repo to re-fetch it from. Don't add it to
that list.

## What was verified vs. what needs Steve to click

Verified without a browser, using Node to import and run the actual
`atomdown-board.plug.js` module (not a reimplementation of its logic —
the real file, with only `syscall()` mocked):

- `atomdown-board.plug.yaml` is valid YAML.
- `atomdown-board.plug.js` passes `node --check` (valid JS) and loads as
  an ES module.
- Against the real test document
  `_silverbullet/Reference/email-domain-cdk-rollout.md` (copied to a
  scratch file, not modified in place): `toggleBoard()` renders one card
  per each of the document's 34 explicit atom directives, plus 3 more
  cards for its unmarked `---` dividers (implicit atoms) — 37 cards
  total, with balanced HTML (`<div>`/`</div>` counts match).
- `saveAttrs()` on atom `XGXD4X4K` (which starts with only an `id`
  attribute): adding two attributes, including one with `"`, `&`, and
  spaces in the value, changed exactly one line in the file, kept the
  directive on that one line, and preserved `id="XGXD4X4K"`. A second
  `saveAttrs()` call removed one of those attributes while keeping the
  other, confirming edits are additive/subtractive rather than
  reserializing.
- The document, after both of those writes, was re-checked with the real
  binary: `go run ./cmd/atomdown lint <file>` against the mutated scratch
  copy — printed `ok`, exit code 0. The `atomdown` repo itself was not
  modified; only the scratch copy of the test document was.
- `saveAttrs()` on an implicit atom (no directive) returns a clean
  `{ok: false, error: ...}` rather than corrupting the document or
  throwing.

**Not verified — needs Steve to click, because this environment has no
browser access to a running SilverBullet instance:**

- That "Atomdown: Toggle Board" actually appears in the real Command
  Palette and opens the modal on screen.
- That the modal visually renders as a full-screen overlay, that cards
  lay out sensibly, and that the three-dot menu popover positions and
  opens/closes correctly by mouse.
- That clicking Save in the real browser round-trips through the actual
  `system.invokeFunction` / postMessage bridge (the Node harness mocked
  `syscall` directly rather than exercising that bridge) and that the
  page visibly updates / the toolbar's Close button and re-running the
  toggle command agree on open/closed state.
- Cross-browser behavior of `CSS.escape`, used once in the popover
  wiring (broadly supported in modern browsers, but not exercised here).

## What is NOT implemented (out of scope for this spike)

- **Dragging or reordering cards.** Cards render in document order and
  do not move. No drag handles, no drop targets.
- **Writing the file from a drag.** There is no drag, so there is
  nothing to write for that; the only write path is the attribute
  editor described above.
- **Locking.** No atom is treated as non-draggable or protected — there
  is no dragging yet to protect against.
- **Any application-level attribute behavior.** This plug does not know
  about `audited`, `audit-source`, `lock`, or any other specific
  attribute name. It is a generic viewer/editor of whatever attributes
  are present. Do not add attribute-name-specific logic here without
  revisiting the design direction in bead `iugum-w6y`.
- **Creating a directive on an implicit atom.** You can view (empty) and
  not edit an implicit atom's attributes; there is no "materialize this
  block as an explicit atom" action yet.
- **Concurrent-edit safety.** `saveAttrs` re-reads the page fresh at
  save time and only rewrites the one target line, but if the document
  changes between opening the board and clicking Save, the save is still
  last-write-wins on that one line. Fine for a single-user spike.
