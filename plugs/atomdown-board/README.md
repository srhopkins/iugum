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
   the header strip, not the prose body below it) to reorder it. The drop
   lands on the seam the pointer is over: releasing anywhere above a card's
   vertical midpoint puts the dragged block before that card, and releasing
   below the last card's bottom edge puts it at the very end. The space
   *between* two cards is the seam between them, not "the end" — see
   "Where a drop lands" below. See "Drag-to-reorder" for exactly what this
   rewrites in the file and how atom groups are handled.
6. Click a card to select it; modifier-click or shift-click to select
   several, or lasso them by dragging on empty background. With a valid
   selection, the three-dot menu's **Group** item wraps those blocks in an
   Atomdown `atom-group`; on a card that is already in a group the item
   reads **Ungroup**. Cmd-Z undoes either. See "Selection and grouping".
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

## How the write path works — the editor buffer, so Cmd-Z works

Every change this plug makes — an attribute edit, a reorder, a group, an
ungroup — reaches the document as **one `editor.replaceRange` call**, and
nothing writes the space file directly any more.

That syscall dispatches one CodeMirror transaction on the live editor view
(`silverbullet/client/plugos/syscalls/editor.ts`), and CodeMirror's
`history()` extension is installed
(`silverbullet/client/codemirror/editor_state.ts`), so the change becomes one
entry in the editor's own undo history. **The user's native Cmd-Z / Cmd-Shift-Z
undo and redo a group, an ungroup, a reorder or an attribute edit like any
other edit.** SilverBullet then autosaves the buffer as usual.

There is therefore **no undo stack in this plug and no "Undo group" button**.
That is the point: a private stack would be a second history the editor's own
one knows nothing about, and Cmd-Z would still not reach a group.

The earlier path was `space.readPage` + `space.writePage` +
`editor.reloadPage`. That wrote *around* the editor, so the change was
invisible to the undo history and Cmd-Z could not reach it — and a write could
silently discard an unsaved buffer. `applyBufferEdit()` replaces it:

1. Read the live buffer with `editor.getText` (not the file on disk — the
   offsets must be offsets into what the editor actually holds).
2. Compute the whole new document text with a pure function
   (`insertGroupMarkers`, `removeGroupMarkers`, `reorderUnit`, or the
   directive-line rewrite in `saveAttrs`).
3. `minimalEdit(oldText, newText)` trims the common prefix and suffix, so the
   one transaction covers only the part that actually changed.
4. `editor.replaceRange(from, to, insert)`.

For an attribute edit specifically, step 2 rebuilds just that one atom's
directive line from the id (always first, always preserved) plus the attribute
list you edited in the panel — every attribute you did not touch is still
there, because the panel starts pre-populated with all of them and only drops
what you explicitly remove.

The directive is always rebuilt onto one source line, since `SPEC.md`
requires that ("Each directive must occupy one source line") and the
`inline-directive` lint diagnostic (`atomdown/parser.go`) enforces it.

## Selection and grouping

Clicking a card selects it. A selected card gets a 2px border in
`var(--ui-accent-color)` — SilverBullet's own accent token, the *same* blue the
drop indicator (`.board-card-dropbefore`) and the grouped-card left edge
already use. Selection never introduces a second blue and never invents a hex.

- **Modifier-click** adds or removes one card (`metaKey` on a Mac, `ctrlKey`
  elsewhere; the code tests both, so either works).
- **Shift-click** selects the contiguous range from the anchor.
- **Lasso**: drag on empty board background to rubber-band a range. It starts
  only on background, never on a card, so it cannot compete with a card drag —
  a drag starts on the card's own header.
- **Clicking empty background** with no modifier clears the selection.

Selection is pure UI state. **Nothing about it reaches the document**: no
coordinate, no index, no attribute. The only things that ever reach the file
are a block move and a group marker, and both of those *are* the document's
content.

The three-dot card menu gains one item: **Group**, or **Ungroup** when the card
whose menu is open is already in a group. A disabled item stays **visible and
grayed**, never hidden, with the reason in its tooltip — a user who selected
the wrong set needs to see that the action exists and read why it is not
offered. `groupMenuState()` is the whole decision, and it is unit-tested:

| Situation | Item | Reason in the tooltip |
|---|---|---|
| The menu's card is in a group | `Ungroup`, enabled | — |
| Fewer than two units selected | `Group`, disabled | Select two or more cards |
| The selection contains a group | `Group`, disabled | Core 1 does not permit a group inside a group |
| The menu's card is not selected | `Group`, disabled | Open the menu on a selected card |
| The selection is not contiguous | `Group`, disabled | Grouping would have to move blocks |
| Two or more adjacent units | `Group`, enabled | — |

### Contiguity: Group is offered only for an already-adjacent selection

An `atom-group` wraps a contiguous span, so `Group` is enabled **only** for a
selection whose units are already adjacent in source order. A non-contiguous
selection leaves the item disabled with a tooltip saying that grouping would
have to move blocks.

**The board never silently reorders the document to make a selection
groupable.** Steve decided this directly (`iugum-w6y.3`): a reorder is a real
content change, worth reviewing in a diff, and the board only makes one when
the user drags a card. Inventing one as a side effect of a grouping click would
be exactly the kind of invisible edit this design exists to avoid.

`isContiguousUnitSelection()` is the check, and it works on *units*, not cards:
a group is one unit, so selecting two members of the same group is one unit,
not two.

### What grouping writes

Atomdown's `atom-group`, per `atomdown/SPEC.md`: an opening marker and a
closing marker, each on its own source line, with the member atoms between
them, and an eight-character Crockford Base32 `id` on the opening marker.

```markdown
<!-- <atom-group id="ZZZZZZZZ"> -->
<!-- <atom id="QX69DE00" digest="sha256:…"/> -->
First member.

<!-- <atom id="RCF2B8FF" digest="sha256:…"/> -->
Second member.
<!-- </atom-group> -->
```

The id comes from `newAtomdownId()`, which reproduces `atomdown`'s own
`NewID` (`atomdown/id.go`, printed by the `atomdown id` CLI): eight characters
of the uppercase Crockford Base32 alphabet, 40 random bits, no `I`/`L`/`O`/`U`.
A worker cannot shell out to that binary, so the generator is reproduced rather
than called; the id is then checked against every id already in the document,
so it cannot collide.

**Nesting is refused.** Atomdown Core 1 does not permit nested groups
(`SPEC.md`: "Atomdown Core 1 does not permit nested groups"), so a selection
containing a group unit disables `Group` — and `insertGroupMarkers()` refuses
it again server-side, because the document may have changed since the board
was drawn.

**The two markers are the only bytes added.** No block's text moves, so no
atom's `digest` can go stale, and no atom's directive line is rewritten, so
every `id` and every unknown extension attribute survives byte for byte. That
is why grouping inserts two lines instead of re-emitting the atoms. Ungrouping
removes exactly those two lines, so **group then ungroup restores the document
byte for byte** — there is a test for that identity. A group written loosely by
hand (`atomdown/testdata/valid/groups.md` puts a blank line after the open
marker) would leave a doubled blank line behind, so `removeLineCollapsingSeam`
drops one blank when removing a marker leaves two against each other.

The markers go hard against the run, with no blank line of their own, which is
the shape `atomdown materialize --split list-item` already writes
(`atomdown/testdata/valid/split-list.md`).

## Drag-to-reorder

Dragging a card moves that block in the source file. There are no
coordinates and no layout attributes anywhere — the card's position in the
column IS its position in the document, per Steve's design direction in
`iugum-w6y`. This is implemented separately from the rendering path above:

### Where a drop lands

`pickDropTarget(clientY, cards)` decides, from the cards' own
`getBoundingClientRect()` values: **drop before the first card whose vertical
midpoint sits below the pointer**; past the last card's midpoint means after
that card, and past its bottom edge means the end of the document. There is
one `dragover`/`drop` pair for the whole panel and no per-card or per-container
handler.

This replaces a handler that decided from *which element* the pointer landed
on. The flex `gap` between cards, and the container's own 16px padding, belong
to the cards container, not to either card — so a release in the space between
two cards fired the container's handler, which hardcoded `(null, "end")`, and
the block went to the end of the document instead of between the two cards.
Geometry does not care which element the pointer technically hit. The container
now uses `gap: 0` with a card `margin-top` as well, so the strip has no holes
in it, but the geometric decision is what actually fixes the bug.

`pickDropTarget` is a standalone pure function with unit tests
(`atomdown-board.test.mjs`), including one for exactly the gap case above.
Its absence as a seam is why the bug shipped: the first version made that
decision inline inside an event listener, where no test could reach it.

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

## Tests

```sh
node --test plugs/atomdown-board/     # directly
go test ./plugs/atomdown-board        # same tests, through go test ./...
```

`atomdown-board.test.mjs` imports the **real** plug file (only `self` and the
`syscall` bridge are stubbed) and covers two layers:

- **The pure decision functions**, exported for tests as `plug.internals`:
  `pickDropTarget`, `unitOrderFromCards`, `isContiguousUnitSelection`,
  `groupMenuState`, `rectsIntersect`, `minimalEdit`, `newAtomdownId`,
  `insertGroupMarkers`, `removeGroupMarkers`. These are the seams whose
  absence let the drop bug ship.
- **The exported plug functions**, driven with a recording `syscall` stub, so
  the tests assert the real syscall sequence: exactly one
  `editor.replaceRange` per action, and no `space.writePage`, `space.readPage`
  or `editor.reloadPage` at all. That is the check that keeps Cmd-Z working —
  a future change that reaches for `space.writePage` fails the suite.

The panel script is not duplicated for testing. `injectSharedFunctions()`
stringifies those same functions into the panel script at render time, and a
test evaluates the injected source and checks it answers identically, so the
panel and the worker cannot drift.

`board_test.go` is the only Go file in this directory. It shells out to
`node --test`, and skips when node is absent, so the JavaScript is covered by
`go test ./...` on a machine that has node without making node a build
dependency (CONTRIBUTING.md rule 1). It also runs `node --check` on the plug
file, because nothing else in the repo would catch a syntax error in a
hand-authored bundle — SilverBullet only finds it when a user runs the command.

There is no `package.json` and no bundler here on purpose; see "Why
hand-authored" below.

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

## What was verified — the drop fix, the popover fix, selection and grouping

Bugs `iugum-w6y.1` (drop always landed at the end) and `iugum-w6y.2` (the
popover closed on any click inside itself), and the selection/grouping feature
`iugum-w6y.3`, were verified three ways.

**1. Unit tests** — 57 of them, `node --test plugs/atomdown-board/`. See
"Tests" above for what they cover.

**2. A real browser, against the real panel.** The panel's html and script were
captured from the actual `toggleBoard()` (a `syscall` stub recording what it
passed `editor.showPanel`), served over HTTP and driven with dispatched
`MouseEvent`s and `DragEvent`s. This exercises the shipped client script, not a
copy of it. Confirmed, with the real 231-line page rendering 62 cards:

- Releasing a drag in the 14px space between card 1 and card 2 invoked
  `reorderAtom(moved, "atom:X1R0XMS8", "before")` — the seam under the
  pointer. That is the bug: it previously invoked `(null, "end")`.
- The top half of a card still resolves to `before` it, the bottom half to
  `before` the next one, below the last card's bottom edge to `(null, "end")`,
  and a drop on its own card makes no call at all.
- Clicking `+ Add attribute`, clicking an attribute input, and clicking a
  remove button all left the popover **open** (`rowsAfterAdd: 3`,
  `rowsAfterRemove: 2`); a click on the board background still closed it.
- Selection: plain click selects one; modifier-click adds then removes;
  shift-click selected the contiguous range `[3,4,5,6]`; a plain click
  elsewhere collapsed to one; a background click cleared it.
- The selected border computed to `rgb(35, 131, 226)` at `2px` — that is
  `#2383e2`, the live value of `--ui-accent-color`, the same token the drop
  indicator's rule names. Both CSS rules were read back from the live
  stylesheet to confirm they name the same custom property.
- The lasso appeared on a background drag, selected `[1,2,3]`, was removed on
  mouseup, and survived the trailing background click. It did **not** start on
  a card body or on the toolbar.
- Menu states matched `groupMenuState` exactly: disabled with "Select two or
  more cards" for no selection, enabled for a contiguous three, disabled with
  the "would have to move blocks" reason for a non-contiguous pair, disabled
  when the menu was opened on an unselected card, `Ungroup` enabled on a
  grouped card, and disabled with "does not permit a group inside a group" for
  a selection mixing a group with a neighbour. Clicking the items invoked
  `groupAtoms(["atom:WRVM8B6Q","atom:X1R0XMS8"])` and
  `ungroupAtoms("ZZZZZZZZ")`. A grouped card that is also selected kept its
  3px left edge and gained the 2px top border.

**3. The real `atomdown` binary, against the real 231-line page.** A copy of
`_silverbullet/Todo/running.md` (62 units, every atom carrying a digest) was
worked on in a scratch directory; the live file was never touched.

- After grouping three contiguous atoms: `atomdown lint` → `ok`,
  `atomdown lint --strict` → `ok`, `atomdown verify` → `ok - no drift`.
- Every atom id identical, every `digest` identical, every block's text
  identical — checked by comparing the parsed atoms before and after, not by
  eye. Exactly two lines were added, both of them markers.
- Ungrouping that group produced a file **byte-identical** to the original.
- After a reorder: `atomdown lint` → `ok`, `atomdown verify` →
  `ok - no drift`.
- Grouping across an unmarked block (a `---` divider) also lints clean;
  `lint --strict` warns only about the pre-existing implicit atom, which it
  warned about before the group too.

### Earlier spike verification (rendering, theme, modal chrome, reorder)

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
- **Grouping a non-contiguous selection.** Deliberate, not a gap — see
  "Contiguity" above. The board will not move a block you did not drag.
- **Nested groups.** Atomdown Core 1 does not permit them, so `Group` is
  disabled for a selection containing a group.
- **Adding a `slug` to a new group.** `insertGroupMarkers` writes only the
  required `id`. A slug can be added afterwards through the group's own
  attribute editing — except that groups do not get their own card yet, so
  today it needs an edit in the page itself.
- **Selecting a whole group by clicking one member.** Clicking one member
  selects that card only; its *unit* is the group, which is what the Group and
  Ungroup decision uses.
- **Keyboard selection.** No arrow-key or Ctrl-A selection; mouse only.
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
