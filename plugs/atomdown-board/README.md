# atomdown-board (spike)

A SilverBullet plug that opens a full-screen modal showing one card per
Atomdown atom in the current document, and lets you view and edit the
generic XML attributes on each atom's directive. Built for bead
`iugum-w6y` ("SilverBullet board view for Atomdown documents").

This is a spike. It proves the rendering, edit-write, and drag-to-reorder
surfaces work end to end. It is **not** the finished board — see "What is
NOT implemented" below.

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
5. Drag a card by its header (the grip icon, id, or badges — anywhere in
   the header strip, not the prose body below it) to reorder it. Dropping
   on the top half of another card moves it before that card; the bottom
   half moves it after. Dropping in the empty space below the last card
   moves it to the very end. See "Drag-to-reorder" below for exactly what
   this rewrites in the file and how atom groups are handled.
6. Closing the modal (the **Close** button, or running the toggle command
   again) returns to the normal page. The document is otherwise
   unchanged unless you explicitly clicked Save on an attribute edit or
   dragged a card.

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

## Drag-to-reorder

Dragging a card moves that block in the source file. There are no
coordinates and no layout attributes anywhere — the card's position in the
column IS its position in the document, per Steve's design direction in
`iugum-w6y`. This is implemented separately from the rendering path above:

- `computeUnits(sourceText)` scans the document into an ordered list of
  "units" — a standalone atom (explicit or implicit) is one unit; a whole
  `<atom-group>...</atom-group>` span is one unit, regardless of how many
  atoms or blank lines are inside it. Each unit records its exact
  `[startLine, endLine]` in the source, not just its content.
- `reorderUnit(sourceText, movedUnitKey, targetUnitKey, placement)` removes
  one unit from that list and reinserts it before/after another, then
  rebuilds the document. A pair of units that stays adjacent in the same
  order across the move reuses its *original* blank-line gap byte for
  byte — including a genuinely zero-blank-line seam, like the one between
  `<!-- <atomdown version="1"/> -->` and the first atom in
  `atomdown/testdata/valid/split-list.md` — so a move that never touches a
  given seam cannot change its formatting. A brand new seam (created by the
  move) gets exactly one blank line, matching every top-level separator
  already used in `atomdown/testdata/valid/{groups,split-list}.md`.
- `reorderAtom(movedUnitKey, targetUnitKey, placement)` is the exported
  plug function the panel's drop handler calls. Like `saveAttrs`, it
  re-reads the page fresh via `space.readPage` rather than trusting
  whatever the client last rendered, then writes the whole rewritten
  document back via `space.writePage`. On success it re-renders the
  still-open panel with the new order in place (calling `editor.showPanel`
  again) instead of closing it — a successful drop should not feel like
  the board closed on you.
- A drop that would not change the order (dropped on itself, or already
  adjacent in that exact position) is a no-op: `reorderUnit` detects this
  and returns without writing the file.

A locked atom (whatever "locked" ends up meaning at the application level
— see `iugum-w6y`'s design notes) still drags normally. Steve was explicit
that a lock protects an atom's *content*, not its position, so this code
never reads any attribute value, locked or not, before allowing a drag.

### Group contiguity decision

`emit.go` rejects a discontiguous group (`TestEmitRejectsDiscontiguousGroup`
in `atomdown/emit_test.go`), and `materialize --split list-item` wraps
split list items in one `atom-group`. Dragging a single member out from
between its siblings would produce exactly the discontiguous shape that
test rejects.

**Decision: a group always moves as one indivisible unit.** Dragging any
card that belongs to a group — grabbing it by its header, same as any
other card — moves the whole group: every member, in its existing
internal order, relocates together to the new position. This is not a
check this code has to remember to run on every drop; it falls out of
`computeUnits()` treating the whole `<atom-group>...</atom-group>` span as
one unit, so `reorderUnit()` only ever knows how to move that span as one
contiguous slice of lines. A discontiguous result is structurally
impossible to produce through this code path, not merely disallowed.

Two alternatives were considered and rejected:

- **Refuse the drag.** This would make a large fraction of a real
  split-list document (see `atomdown/testdata/valid/split-list.md`, where
  every list item is a group member) permanently immovable, for no benefit
  to a user who dragged a card meaning "move this content."
- **Dissolve the group on drag.** Silently deleting group structure as a
  side effect of a reorder is a surprising, easy-to-miss content change —
  the group markers are meaningful data produced by `materialize --split`
  (see `atomdown/split.go`), not incidental formatting a reorder should be
  allowed to erase.

Content inside a group's span that has no directive of its own (an
implicit block that happens to sit between two group markers) moves with
the group rather than becoming its own independently draggable unit —
consistent with the same contiguity reasoning: extracting it would need to
invent a new position for it that could put it outside the group's own
markers.

Proven with a real test document (`scratchpad/reorder-harness.mjs`, not
committed — imports the actual plug module the same way the earlier
`saveAttrs` verification did): dragging the *middle* member of a tight,
no-blank-line group (the exact shape `materialize --split list-item`
produces, from `atomdown/testdata/valid/split-list.md`) moved all three
list items together, in their original relative order, with the group's
open/close markers still directly wrapping them and nothing interleaved.
The same was proven for a *loose* group with blank lines between members
(the shape in `atomdown/testdata/valid/groups.md`). Both mutated documents
were then re-checked with the real binary: `go run ./cmd/atomdown lint`
printed `ok` (exit 0), and `go run ./cmd/atomdown strip` produced the same
prose, reordered, with no doubled blank lines and no lost content.

## Theme

The board renders inside an iframe (`client/components/panel.tsx`'s
`IFramePanel`), and CSS custom properties do not cross an iframe boundary
on their own — SilverBullet's real theme variables
(`--root-background-color` and friends) live only on the *parent*
document's `<html>`. Without doing anything about this, every `var(...,
fallback)` in this plug's own `<style>` block would always resolve to
its JS-authored fallback, regardless of the space's actual light or dark
theme.

The fix lives entirely in the client script that runs inside the panel
iframe (not the plug's worker code, which has no `window`/`document` at
all — see `isWorker` at the top of `atomdown-board.plug.js`). That iframe
is loaded via `srcDoc` with no `sandbox` attribute
(`client/components/panel.tsx`), which makes it same-origin with the
parent, so `applyParentTheme()` reads the parent's live computed custom
properties with `window.parent.getComputedStyle(...)` and copies them onto
this document's own root with `document.documentElement.style.setProperty`
— live values, not a hardcoded palette. It also copies the parent body's
computed `font-family` the same way. If reading the parent ever fails
(cross-origin, parent gone), the `<style>` block's own `:root` values are
used instead — those are a snapshot of the space's real *light*-theme
values (confirmed live from the running instance), never a dark guess.

Verified in the browser: with the space in its default light theme, the
board's background, text, borders, and accent color matched the page
behind it (`--root-background-color: #ffffff`, etc.) rather than the
previous hardcoded dark fallback. Toggling the parent to
`data-theme="dark"` and then closing and reopening the board picked up the
real dark values correctly. A live toggle *while the board stays open* did
not update it in this test: `panel.tsx` does hold a `MutationObserver` on
`document.documentElement`'s `data-theme` attribute that is meant to
`postMessage` a `{type: "theme"}` notice into the iframe on every toggle,
and this plug's client script does listen for that message and re-run
`applyParentTheme()` when it arrives — but toggling `data-theme` in this
test never produced that message (confirmed by counting `message` events
inside the iframe across two toggles: zero), so something about how this
deployed instance updates theme did not trigger it. Manually posting the
same `{type: "theme"}` message *did* make the listener re-apply the
correct colors, proving that half of the mechanism works — the reopen path
is what actually reads live values and is therefore the reliable one.
Reopening the board (Close, then run the command again) after a theme
change is guaranteed correct; whether it updates live while left open is
not.

## Modal chrome — reads as a page view, not a floating dialog

`toggleBoard()` calls `editor.showPanel("modal", 0, html, script)` with
inset `0` (previously `24`), so the panel fills the window edge to edge
instead of floating with a margin, and — once the background actually
matches the real page background (see "Theme" above) — the dim backdrop
behind it is no longer visible, since the panel now fully covers it.

**Residual constraint, not fixable from inside this plug:** SilverBullet's
own compiled `client/styles/main.scss` still applies a `border-radius:
8px`, a `box-shadow`, and a `1px` border to the `.sb-modal` wrapper
element, and renders `.sb-modal-backdrop` as a sibling behind it — both of
those classes live on elements in the *parent* document
(`client/editor_ui.tsx`), not inside this plug's iframe, so nothing this
plug's `<style>` block does can reach them. Confirmed in the browser via
`getComputedStyle` on the live `.sb-modal` element: `inset: 0px` (this
plug's change took effect), but `borderRadius: 8px` and a visible
`boxShadow` remained. In practice this shows up as a faint rounded/shadowed
edge right at the screen border — everything else (backdrop dimming, the
floating margin) is gone.

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

## What was verified

The rendering/edit spike (commits `a171a7f`, `61d85be`) was verified only
without a browser; the theme fix, modal-chrome fix, and drag-to-reorder
feature (this change) were verified in a real, running browser against the
FFAI SilverBullet space at `http://localhost:3000`, using a scratch page
(`Reference/atomdown-board-test.md`, materialized with the real
`atomdown materialize --split list-item` binary, then deleted before
finishing — never a real document):

- **Theme:** confirmed the board's colors matched the space's live light
  theme, and matched the live dark theme after toggling and reopening the
  board — see "Theme" above for what was and was not confirmed about a
  live in-place toggle.
- **Modal chrome:** confirmed `inset: 0` reached the real `.sb-modal`
  element and the floating margin/backdrop dimming were gone; confirmed
  the residual rounded corner and box-shadow described in "Modal chrome"
  above are real and come from the parent document, not this plug.
- **Drag-to-reorder:** dispatched real `dragstart`/`dragover`/`drop`/
  `dragend` `DragEvent`s (with a `DataTransfer`) inside the panel's
  iframe. Confirmed: (1) dragging a standalone card to after the last card
  moved it there, in both the rendered board and the on-disk file; (2)
  dragging the *middle* card of a real materialized atom-group moved the
  whole group together, in original order, markers intact — the group
  contiguity decision above, exercised for real, not just in the Node
  harness; (3) dropping in the empty space below the last card moved a
  card to the very end; (4) after each drag, `go run ./cmd/atomdown lint`
  on the rewritten file printed `ok` (exit 0); (5) the attribute-editing
  popover (unchanged code) still opened correctly afterward, and the
  browser console showed no errors from this plug throughout.
- Group contiguity was additionally proven against two real atomdown test
  fixture shapes (tight and loose groups) via
  `scratchpad/reorder-harness.mjs` (not committed) — see "Group
  contiguity decision" above for what that harness checked.

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

All of the following, previously listed as "needs Steve to click," were
confirmed in the real browser session described above: "Atomdown: Toggle
Board" appears in the real Command Palette and opens the panel on screen;
cards lay out sensibly and the three-dot menu popover opens/closes
correctly by mouse; clicking Save round-trips through the actual
`system.invokeFunction`/postMessage bridge and the page visibly updates.

**Still not verified:** cross-browser behavior of `CSS.escape` (used once
in the popover wiring) and of the drag-and-drop event handling, beyond the
one Chromium-based browser this was exercised in.

## What is NOT implemented (out of scope for this spike)

- **Reordering members within a group.** Dragging a group card moves the
  whole group (see "Group contiguity decision" above); there is no way to
  change the order of atoms *inside* one group's markers from the board.
- **Locking.** No atom is treated as non-draggable or protected on the
  basis of any attribute — see "Drag-to-reorder" above for why that is
  deliberate, not a gap.
- **Any application-level attribute behavior.** This plug does not know
  about `audited`, `audit-source`, `lock`, or any other specific
  attribute name. It is a generic viewer/editor of whatever attributes
  are present. Do not add attribute-name-specific logic here without
  revisiting the design direction in bead `iugum-w6y`.
- **Creating a directive on an implicit atom.** You can view (empty) and
  not edit an implicit atom's attributes; there is no "materialize this
  block as an explicit atom" action yet.
- **Concurrent-edit safety.** `saveAttrs` and `reorderAtom` both re-read
  the page fresh before writing, but a drop or a save is still
  last-write-wins against whatever the file held at that moment — if the
  document changed since the board was opened, the card the user meant to
  move might land somewhere unexpected, or `reorderUnit` might report "no
  longer found" if the specific unit disappeared. Fine for a single-user
  spike; not safe for two people editing the same page at once.
