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
   Each card shows its readable name (its `slug`) when it has one, its id
   when it does not, and its block **rendered as CommonMark** — a heading
   as a heading, a table as a table, a link as a link. The id stays on the
   card either way -- see "Names (slugs)" and "Rendered CommonMark, and the
   raw option".
3. The toolbar carries two switches beside **Close**: one flips the whole
   board between rendered CommonMark and raw markdown, and one flips it
   between the **comfortable** and **compact** densities. Compact drops the
   card header row and thins the group bars, and changes no content size at
   all. Both choices are remembered per page — see "Two densities:
   comfortable and compact".
4. Click the three-dot (`⋮`) menu on a card to see the atom's name and id,
   then every XML attribute
   present on that atom's directive, as plain name/value pairs — whatever
   they happen to be. There is no list of known attribute names anywhere
   in this plug; it renders and edits generically, the same way Atomdown
   Core itself preserves attributes without interpreting them (see
   `atomdown/SPEC.md`, "Extensions").
5. In that menu you can change a value, add a new name/value pair, or
   remove one, then click **Save**. Saving rewrites *only* that atom's
   one directive line in the document (see "How the write path works"),
   and leaves everything else byte-identical.
6. Drag a card by its header (the grip icon, id, or badges — anywhere in
   the header strip, not the prose body below it) to reorder it. At compact
   density there is no header strip, so the floating grip in the card's
   top-left corner is the drag source — see "Two densities" below. The drop
   lands on the seam the pointer is over: releasing anywhere above a card's
   vertical midpoint puts the dragged block before that card, and releasing
   below the last card's bottom edge puts it at the very end. The space
   *between* two cards is the seam between them, not "the end" — see
   "Where a drop lands" below. See "Drag-to-reorder" for exactly what this
   rewrites in the file and how atom groups are handled.
7. Click a card to select it; modifier-click or shift-click to select
   several, or lasso them by dragging on empty background. With a valid
   selection, the three-dot menu's **Group** item wraps those blocks in an
   Atomdown `atom-group`; on a card that is already in a group the item
   reads **Ungroup**. **Group** first asks for a readable name for the
   group, defaulted from the first heading in the selection, so one
   confirm is usually enough. Cmd-Z undoes either. See "Selection and
   grouping" and "Names (slugs)".
8. An `atom-group`'s cards are wrapped in one bordered container with a
   header bar carrying the group's name, its id, and **Rename** and
   **Ungroup**. Clicking the header selects the whole group; the toggle at
   its left collapses it. See "One group, one object".
9. Closing the modal (the **Close** button, or running the toggle command
   again) returns to the normal page. The document is otherwise
   unchanged unless you explicitly clicked Save on an attribute edit or
   dragged a card.
10. If the board was open on a page when you reload it, it reopens by itself.
   It never opens on a page you did not open it on, and Close means closed.
   See "Remembering the view".
11. The toolbar's **Raw markdown** button switches every card to its
   markdown source, and back. One card's three-dot menu carries **Show raw
   markdown** for that card alone. Rendered is the default at both levels.
   See "Rendered CommonMark, and the raw option".

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
  -->` tags every atom between the markers with that group's id and, if
  the marker carries one, its `slug` (the badge on the card shows the
  slug, or the id when there is none). Groups do not get their own card.
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

## Names (slugs)

Steve's problem, in his words: "need the ability to have human friendly
grouping slugs, as a human it is hard for me to group ids". An eight-character
Crockford Base32 id is the right identity and the wrong label.

Atomdown already has the answer. `SPEC.md` ("Identity") makes `slug` an
optional attribute on both `atom` and `atom-group`, and says outright that
"the slug is not identity". So the board writes and reads that attribute and
nothing else -- no sidecar file, no new attribute, no change to the format.

**Identity never moves.** Every structural decision in this plug still keys on
the id: unit keys (`atom:<id>`, `group:<id>`), the group lookup an ungroup
does, the drop target, the collision check on a generated id. A slug only ever
changes what a human reads. Naming or renaming therefore cannot change any
`id` and cannot change any `digest`: creating a group inserts two lines and
touches no block text, renaming a group rewrites that group's one opening
marker line, and naming an atom rewrites that atom's one directive line with
its id and digest carried through unchanged.

### Where the name shows, and how the id stays reachable

- A card's header shows the slug first, in the body font, then the id in small
  subtle monospace. A card with no slug shows just the id, as before. The id
  is on the card either way, because it is the value Steve needs when citing
  an atom from another page, and its tooltip says so.
- A group's header bar shows the slug first, in the body font, then the id in
  small subtle monospace -- the same order and the same weights a card uses. A
  group with no slug shows just the id. The real group id is always in the
  header's tooltip too. See "One group, one object".

### Naming a group at creation

**Group** does not act immediately any more. It swaps the popover's group row
for a small naming form: a label, one text input, **Group** and **Cancel**,
and a hint line that previews the exact `slug="..."` that will be written.

The prompt affordance is a form *inside the popover that is already open*.
Not `window.prompt`: some browsers suppress it inside an iframe, and where it
does run it renders as browser chrome rather than as part of the page. Not a
second modal either -- the board itself is already a modal panel. The popover
already holds text inputs for the attribute editor, so this reuses that
pattern.

The input is prefilled by `deriveGroupSlug()`, which reads the selected
blocks' own text and takes the first heading it finds -- an ATX heading
(`## Email PRs`) first, then a setext heading, then the first non-blank line
of any kind, and `group` if nothing survives. So the usual gesture is select,
Group, Enter. The user is never made to invent a name.

Enter confirms, Escape cancels.

### Renaming

For an **atom**, the slug is the first row of the attribute editor that
already exists, with a real label ("Name (slug) - readable alias, not the
id") and its own input rather than a generic name/value pair. There is
deliberately no second rename editor: `slug` *is* an ordinary directive
attribute, and the attribute editor already rewrites exactly one directive
line. Saving a changed name redraws the board so the card relabels itself;
saving with the name unchanged leaves the popover open, as it always did.

For a **group**, **Rename** on the group's own header bar opens the same
naming form, prefilled with the current name, directly under the header.
Clearing the field removes the `slug` attribute; the group then shows its id
again.

That used to be a **Rename group** item in a member card's menu. It has been
removed. The item only ever existed because a group had no UI of its own; a
group now has a header bar, so a group-level action lives there and a card's
menu holds card-level things.

## One group, one object

Steve, seeing the earlier per-card marking: *"I like the blue we use on
grouped items can we make it lasso the entire group not just individual
cards"*, and *"yes to header bar"*.

An `atom-group`'s members render inside ONE bordered container. The border is
`--ui-accent-color` -- the same SilverBullet accent token already used by the
drop indicator and the selection ring, so the board still has exactly one
blue. There is no second hue anywhere in this design.

Because the container is what says "group", the two per-card group markings
are gone: no left accent stripe on a member card, and no `group <slug>` badge.
Repeating the group's identity on every member is what made a group read as
several objects.

**The per-card treatment that survives is contrast, not a second border.** The
container's own field is the plain surface (`--ui-surface-background-color`)
and a member card keeps the ordinary tinted card background, so the cards read
as contents *on* the group. A background difference cannot be confused with
either the container edge or a selection ring, which a second border could.

### The header bar

One row across the top of the container, in the accent colour with
`--ui-accent-contrast-color` text. Left to right:

- a collapse toggle,
- a drag handle,
- the word `GROUP`, small and uppercase,
- the **name** (the slug), then the id in monospace, then the card count,
- **Rename** and **Ungroup**, right-aligned.

**Clicking the header selects every member card.** That is the whole
integration with the existing logic: the selection is still a set of cards, so
`selectedUnitKeys()` collapses them to the one `group:<id>` unit and
`groupMenuState()` applies unchanged -- Ungroup acts on the group, and Group
stays disabled with "Atomdown Core 1 does not permit a group inside a group"
when a group is in the selection. No special case was added anywhere.

### Selection stays legible against the container

The container edge and the selected-card border are the same token, so
selection separates itself by **shape**, not by colour: a selected card is a
double ring -- its own 2px border, then a second 2px ring set 2px outside it
with the container's field showing through the gap -- plus a lifted
background. A container is one continuous edge; a selected card is a banded
edge with a gap in it.

The second ring is an `outline`, not a second `box-shadow`: `box-shadow` is
already spoken for by the drop indicator, and an outline takes no layout
space, so selecting a card never nudges the geometry the drop decision just
measured.

### Collapse and expand

The collapse toggle hides a group's cards, leaving its header. On the real
291-line page that turns 82 cards into a scannable list of the 11 named
groups, which is the point.

**This is presentation state and is never written to the document.** There is
no `collapsed` attribute on any directive and there never will be: Atomdown
carries no layout, position, card or board metadata. It lives in
SilverBullet's own client-local key-value store -- see "Remembering the view".

A collapsed group is still draggable, which is why the header has a drag
handle: every member card's own handle is hidden. And `cardGeometry()` feeds
`pickDropTarget()` the *container's* rectangle once for a collapsed group,
instead of its hidden cards. A hidden element reports an all-zero rectangle,
which would otherwise sort above every real card and make every drop land at
the top. `pickDropTarget()` itself is untouched and still pure; it is simply
given the right rectangles.

## Remembering the view

Steve: *"every refresh to the page and I have to go re-apply the atomdown view
can we make that view persist on refresh if I am on that view (not make it
default, just don't lose it)"*.

Two pieces of presentation state are remembered, both keyed by page name:

| key | value |
|-----|-------|
| `atomdown-board.open:<page>` | `true` while the board is showing on that page |
| `atomdown-board.collapsed:<page>` | the ids of that page's collapsed groups |

`restoreBoard()` is wired to `editor:pageLoaded` and `editor:pageReloaded`
(the manifest declares the events; SilverBullet dispatches them from
`client/content_manager.ts`). It opens the board only for a page whose key is
set. It is **never a default**: a page whose key was never written gets
nothing, and Close deletes the key, so closed stays closed.

**The store is `clientStore`, not `localStorage`.** A plug's code runs in a
Web Worker, and a worker has no `localStorage` at all, so the worker could not
read a flag the panel wrote there. `clientStore`
(`client/plugos/syscalls/clientStore.ts`) is reachable from both the worker
and the panel iframe through the one syscall bridge, so there is a single
persistence mechanism in this file rather than two. It is per-browser and
durable across a reload -- not session-scoped, because "do not lose it on
refresh" is the requirement and Close already clears it. None of it is
visible to the space's files or to any other device.

Three things stop a reopen racing the editor:

1. SilverBullet dispatches those events *after* it has set the editor state
   for the new page, so `editor.getText` already holds that page's text.
2. The text is checked: an empty buffer means the editor is not ready after
   all, so the board stays closed rather than drawing an empty one. The
   remembered flag survives, so the next load reopens.
3. The page is re-read immediately before drawing, so a second navigation that
   overtakes the first cannot leave the previous page's board on screen.
   Navigating to a page with no remembered board also takes down a panel still
   showing from the previous one.

Every read and write is failure-tolerant. A store that is missing or throwing
(private browsing, site data blocked, an older host, a test stub) degrades to
"not remembered" -- a closed board and expanded groups -- never to an error.

### Shape and collisions

Typed text is sanitized by `sanitizeSlug()` into the shape atomdown itself
generates: lowercase kebab-case ASCII. Accented letters fold to their base
("Décisions" -> `decisions`) rather than being dropped, every other run of
characters outside `[a-z0-9]` becomes one hyphen, leading, trailing and
doubled hyphens go, and a very long name truncates at a word boundary. The
function is idempotent, so sanitizing an already-clean slug changes nothing.
An empty result means "no slug" -- `slug=""` is never written.

`slugConflict()` reports a name already carried by another atom or group in
the same document. It **warns and still writes**: the format permits duplicate
slugs, because a slug is not identity, so refusing the edit would be the
tooling overruling the format. The naming form previews the clash as you type,
and after the write the worker re-checks the live buffer and shows an
`editor.flashNotification`.

These two are deliberately the **only** places this plug decides slug shape
and slug collision, and each carries a `DELEGATION POINT` comment. atomdown is
growing a `materialize --slugs` generator and a duplicate-slug lint
diagnostic; when those land, each function becomes a one-line delegation
rather than a second opinion scattered through the plug. Nothing here depends
on them landing.

Both `sanitizeSlug()` and `deriveGroupSlug()` are in `CLIENT_SHARED_FUNCTIONS`,
so the panel and the worker share one copy by source injection and cannot
disagree about what a typed name becomes. A test drives the injected copies
against the tested ones.

### Attribute order

A slug is written immediately after the id -- `id`, `slug`, then everything
else -- which is the order `emit.go` itself uses for both an atom directive
and a group marker. So running the document through `atomdown` afterwards does
not reshuffle the line the board wrote.

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

## Rendered CommonMark, and the raw option

Steve: *"the view I have does not render the markdown in the individual cards,
which I do want an option to view raw markdown but it should also display
rendered markdown/commonmark by default"*.

Before this, a card showed its block's raw markdown in a monospace font. A
heading read as `## RESEA tickets - due tonight`, a table read as pipes, and a
link read as bracket-paren syntax. On a 291-line page that reads as a cluster
rather than as the document it came from.

### The renderer is the host's, not ours

Rendering goes through **SilverBullet's own markdown pipeline**, reached with
the `markdown.markdownToHtml` syscall:

| piece | where |
|-------|-------|
| syscall declaration | `silverbullet/plug-api/syscalls/markdown.ts` |
| implementation | `silverbullet/client/plugos/syscalls/markdown.ts` |
| registered for plugs | `silverbullet/client/client_system.ts` |
| parser | `client/markdown_parser/`, with the space's own `syntaxExtensions` |
| renderer | `client/markdown_renderer/markdown_render.ts` |

That is the same call upstream's own configuration-manager plug uses to render
library descriptions, so this is the supported path and not a private door.
Reusing it buys three things a hand-written renderer could not: correct
CommonMark, the space's own syntax extensions, and a card that looks like the
editor.

**No markdown library is bundled into this plug, and none should ever be.**
There was no need to investigate a fallback — the syscall exists, is registered
for plug workers, and works. `renderAtomBodies` calls it once per atom in
`showBoard`, because rendering needs a syscall and `buildBoardHtml` is pure
markup assembly.

A host that does not have the syscall, or one block the renderer refuses, is
not fatal: that card carries `data-no-rendered="1"`, falls back to its raw
markdown, and the board still draws.

### Sanitizing

`renderMarkdownToHtml` serializes through `renderHtml`
(`client/markdown_renderer/html_render.ts`), which puts every **text node** and
every **attribute value** through `htmlEscape`. So markdown *text* can never
inject markup, and that part needs nothing from us.

The hole is deliberate on upstream's side: a **raw HTML tag in the markdown
source** is re-emitted verbatim as a `RawHtml` tag. The page content is Steve's
own today, but it will hold pasted text, and this panel iframe is loaded via
`srcDoc` with no `sandbox` attribute — it is same-origin with the app. So
`sanitizeRenderedHtml` filters the host's output before it reaches the markup:

- a **tag allowlist** (`SAFE_TAGS`) — everything CommonMark produces, nothing
  that can run code or load a remote document;
- an **attribute allowlist** (`SAFE_ATTRS`) — which makes every `on*` handler
  impossible without a rule of its own. No `style` (a fixed-position overlay is
  a clickjack) and no `id` (it would collide with the panel's own element ids);
- a **URL scheme allowlist** (`SAFE_URL_SCHEMES`) on `href` and `src`, tested
  after decoding entities and stripping control characters, so neither
  `&#106;avascript:` nor `java<tab>script:` survives;
- **strip-with-contents** for `script`, `style`, `iframe`, `svg` and friends;
  any other disallowed tag loses the tag but keeps its text, because that text
  is content the user wrote;
- **balance**. Every close tag must match an open tag the sanitizer itself
  emitted, and anything still open at the end is closed. A stray `</div>` in
  the document would otherwise close the *card's* own element and put the rest
  of the board inside one card — which would break the card rectangles
  `pickDropTarget` reads.

Text runs are passed through untouched rather than re-escaped, because the
input is already-escaped HTML and escaping it twice would show `&amp;` to the
user. A `<` that does not begin a tag *is* escaped, so a malformed or
unterminated tag degrades to visible text.

### The raw affordance: board-wide, plus a per-card override

Both, and the board-wide one is the primary. Steve wants to read the document
and *occasionally* inspect one block's syntax, so:

| control | scope | where |
|---------|-------|-------|
| **Raw markdown** / **Rendered** | the whole board | the toolbar, beside Close |
| **Show raw markdown** / **Show rendered** | one card | that card's three-dot menu |

**Rendered is the default at both levels.** An absent stored value, a stored
value in a shape `loadViewState` does not recognise, and a store that throws
all mean rendered.

The board-wide switch is the master: flipping it **clears every per-card
override**, so "show me the whole document as markdown" means the whole
document and not "the whole document except the four cards I poked". Setting a
card's override to the board-wide value clears it rather than storing a
redundant copy, so a later flip of the board switch still moves that card.

Both bodies are in the markup — a visible one and a hidden one — rather than
one being fetched on demand. Three reasons:

- toggling is a hidden flag, not a redraw and not a round trip to the worker;
- a card the renderer could not handle falls back with no extra path;
- the panel still holds each block's **exact** original text, which is what
  `deriveGroupSlug` reads when it defaults a new group's name. Reading a
  rendered heading would have lost the `##` it matches on. `selectedCardTexts`
  reads `.board-card-raw` for exactly this reason.

### Keeping the interactions alive over a much richer DOM

A rendered card is a far richer DOM than a `<pre>`, and this is where it would
break:

- **A rendered link must not hijack a card-selecting click.** A card link is a
  real `<a>` with a real `href`, so it looks and hovers like the link in the
  document, but a capture-phase handler calls `preventDefault()` on it. The
  click then falls through to the card's own handler and **selection wins**.
  Following the link would have replaced the board with the target page. The
  sanitizer also puts `target="_blank"` and a rel that blocks `window.opener`
  on an absolute link, so a click that somehow escapes that handler opens a tab
  instead of destroying the board.
- **A rendered table must not change the card's rectangle.** `pickDropTarget`
  decides a drop from each card's own rectangle, so a card that grew wider than
  the column would change the geometry. The table scrolls inside its own box
  (`max-width: 100%; overflow-x: auto`), which leaves the card's box exactly
  where the flex column put it.
- **A rendered task checkbox is a picture, not a control** — `pointer-events:
  none`. This board never writes a byte from a card body, so letting it be
  clicked would promise an edit that cannot happen.
- **Text selection is off in the rendered body**, because selecting a card is a
  click gesture and dragging text would fight it. The *raw* body keeps text
  selectable, since copying the source is the reason to open it.

### It is presentation, like the collapse state

Which body a card shows is presentation and **never reaches the document**. It
lives in the same client-local key-value store as the collapse state and the
remembered view, under one more page-scoped key:

| key | value |
|-----|-------|
| `atomdown-board.view:<page>` | `{ boardView: "rendered" \| "raw", cardViews: { <atomId>: "rendered" \| "raw" } }` |

`clientStore` and not `localStorage`: a plug's code runs in a Web Worker, which
has no `localStorage` at all, and `clientStore` is reachable from both the
worker and the panel iframe through the one syscall bridge. There is one
persistence mechanism in this file, not two.

## Two densities: comfortable and compact

The board has two display densities and no third. **Comfortable** is the
default. **Compact** compresses the chrome and nothing else. There is no
"bare" density, because reading the document with no chrome at all is what
closing the board does — it needs no mode.

The switch is a button in the toolbar, beside the raw/rendered switch and in
the same idiom: it is labelled with the state it would *give* you, so
`Compact` means "press this for compact". The same control repeats in a
group's three-dot menu, with the current density as a state readout.

### What compact changes

| | comfortable | compact |
|---|---|---|
| card header row | present, quiet at rest, full on hover | **gone** |
| card slug and id | on the header | in the three-dot menu |
| grip and three-dot button | in the header row, hover-only | floating in the card's top corners, hover-only |
| group bar | chevron, grip, `GROUP`, name, id, count, Rename, Ungroup | chevron, grip, name, bare count, three-dot menu |
| group outline | 2px accent | **identical** |
| collapse chevron | full size, always visible | **identical** |
| content (headings, tables, body text) | full rendered size | **identical** |

**No seam.** The header row is not replaced by a dotted line, a dashed line,
or a rule of any kind. The card border is the card. The rendered content
already carries the name, because a heading renders as a heading, which is
what made the header redundant at this density in the first place.

**The collapse chevron is the one thing compact does not compress.** It is the
control that turns a 291-line page into 11 lines, so it keeps its full size
and stays visible at both densities. That is restated explicitly in the
compact rules rather than left to inheritance, so a later change to the bar's
padding cannot shrink it by accident.

### How compact keeps every interaction alive

The markup is **identical at both densities** — the card strip is byte for
byte the same string — so switching is a CSS-level change, needs no redraw and
no round trip to the worker, and cannot move, add or remove an element that
the geometry, the drag, the selection or the lasso reads.

What is left of the card header in compact is a chrome layer pinned across the
card's top edge, carrying the grip and the three-dot button and nothing else:

- `position: absolute`, so it adds **no height**. A card's rectangle is its
  body's rectangle, which is exactly what `cardGeometry()` hands
  `pickDropTarget()`. Shorter cards, same decision function.
- `pointer-events: none` on the layer and `auto` on the two controls, so a
  click on the top strip of a card falls **through** to the card and still
  selects it. Click-to-select, modifier-click, shift-range and
  lasso-from-empty-background are untouched.
- the grip is the header's first child and the menu carries
  `margin-left: auto`, so the flex row puts them in the top-left and top-right
  corners with no per-control positioning to get wrong.
- the body reserves `--board-card-chrome-space` at its top right, so the menu
  never sits on top of content. The grip gets an opaque chip behind it so it
  never smears the first line of text it floats over on the way in.

**What drags in compact: the grip.** In comfortable the whole header row is the
drag source (`draggable="true"` on `.board-card-header`) and it still is. That
row has no pointer events in compact, so the grip carries `draggable="true"`
and its own `data-drag-unit` — the resolved unit key, so a grouped card's grip
drags the whole group, the same key a member card's header resolves to. The
existing generic `[data-drag-unit]` handler picks it up and stops propagation,
so one drag is one `dragstart` with one unit key at either density. The card
body is deliberately **not** draggable: a card whose whole surface starts a
drag turns every mis-aimed click into a move, and the body is the click target
for selection.

The grip is `role="button" tabindex="0"` with an `aria-label`, so the
`:focus-visible` escape in the hover-only rules genuinely reaches it — a
hover-only drag source that no keyboard can focus is a drag source a keyboard
user does not have.

### Identity moved into the menu

With the header gone, a card's slug and id are not on screen. So the three-dot
menu's popover opens with the **name and the id as a non-clickable label**,
before any action — if the menu is the only place identity lives, the menu has
to show it. That label is built at **both** densities, so the menu reads the
same wherever it is opened. An implicit atom has no directive and so no id of
its own, and says exactly that.

A group's menu carries the same label, plus the two actions the thin bar folded
away (Rename, Ungroup) and the density readout.

### Comfortable recedes too

A card you are not pointing at is quiet: the header text drops to the theme's
own muted text token at rest and returns to the full text token on hover,
focus, or selection, while the grip and the three-dot button stay fully hidden
until then. So the header gets out of the way without switching density.

A group's own chrome recedes the same way: at rest the container's border and
the header bar's background soften, and they come to full strength when the
pointer is anywhere inside the container — **hovering a member card counts**,
because `:hover` is tested on the container, which a descendant's hover
satisfies. Four states never recede: a hovered group, a group with focus in it,
a group holding a selected card (which is also what clicking the header
produces), and a **collapsed** group — collapsed, the bar is the only thing on
screen representing its contents, so it must stay findable.

**A token or a `color-mix`, never `opacity`.** Two reasons, and the second one
is not a preference:

1. `opacity` multiplies against whatever is behind the element, so one value
   reads differently on the plain card surface, on a selected card's lifted
   background, on the group container's field, and again in the dark theme.
   `--subtle-color` is the colour SilverBullet itself uses for secondary text,
   so it is legible by construction in both themes and on all of those.
2. `opacity` applies to an element **and all its descendants**. An opacity on
   `.board-group` would fade every member card inside it. Only the container's
   `border-color` and the header bar's `background` move (with the header's
   text colour following its background, or light-on-pale would be
   unreadable). A member card's surface, border and content are identical
   resting and active.

Both are colour-only changes, so nothing reflows between the two states.

The resting override is written **after** the full-strength rules, so a browser
without `color-mix()` or `:has()` drops the block and the group simply never
recedes — it is never left at an unreadable half state.

### It is presentation, like everything else here

The density never reaches the document. It lives in the same client-local
key-value store, under one more page-scoped key:

| key | value |
|-----|-------|
| `atomdown-board.density:<page>` | `"comfortable"` \| `"compact"` |

A page that was never switched, a store that is missing or throwing, and a
stored value in any shape `loadDensity()` does not recognise all come back as
comfortable. The density is rendered into the markup rather than applied by the
panel script afterwards, so a board left compact draws compact with no frame of
the wrong layout — the same rule a collapsed group follows.

## Customizing the board's CSS

Every tweakable value is a named CSS custom property with a default. Set any of
them on `html` in your own `space-style` page and the board picks it up:

```css
html {
  --board-card-padding: 4px;
  --board-accent-color: #b5651d;
  --board-card-radius: 0px;
}
```

That indirection is not a style choice, it is the only route in. The panel
renders in an iframe, so a stylesheet in the parent document cannot select
anything inside it. Named custom properties are copied across by
`applyParentTheme()` — exactly the seam that already carries SilverBullet's
theme tokens, so there is one mechanism here, not two. A property you do not
set keeps its default; a property you do set lands as an inline declaration on
the panel's root, which beats both the defaults **and** the compact
overrides, so a value you asked for wins at either density.

The stable class names matter as much as the properties. `.board-card`,
`.board-card-header`, `.board-card-body`, `.board-group`,
`.board-group-header` and the rest are part of this surface and are not
renamed.

| property | default | what it sizes |
|---|---|---|
| `--board-card-padding` | `8px` | a card body's padding (compact: `6px`) |
| `--board-card-header-padding` | `6px 8px` | the card header row's padding, which is what gives it its height (compact: `0`) |
| `--board-card-header-height` | `auto` | a floor under that row's height (compact: `0`) |
| `--board-card-border-width` | `1px` | a card's border |
| `--board-card-radius` | `6px` | a card's and a group container's corner radius (compact: `4px`) |
| `--board-accent-color` | `var(--ui-accent-color)` | the one blue: the group outline, the drop indicator, the selection ring, the lasso |
| `--board-grip-size` | `14px` | the six-dot drag grip |
| `--board-id-size` | `11px` | the monospace id, on a card and on a group bar |
| `--board-header-quiet-color` | `var(--subtle-color)` | a resting card header's text |
| `--board-header-active-color` | `var(--root-color)` | a hovered, focused or selected card header's text |
| `--board-card-gap` | `14px` | the space between two top-level items (compact: `6px`) |
| `--board-card-chrome-space` | `24px` | room reserved at a compact card body's top right, so the menu never overlaps content |
| `--board-group-padding` | `8px` | a group container's inner padding (compact: `4px`) |
| `--board-group-card-gap` | `8px` | the space between two member cards (compact: `4px`) |
| `--board-group-header-padding` | `5px 8px` | the group bar's padding, which is what thins it (compact: `1px 4px`) |
| `--board-group-border-width` | `2px` | the group outline. **The same at both densities** — it is structure, not chrome |
| `--board-group-quiet-border` | `40%` | how much accent a resting group's outline keeps |
| `--board-group-quiet-header` | `16%` | how much accent a resting group's header bar keeps |

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
  `insertGroupMarkers`, `removeGroupMarkers`, `setGroupSlugInSource`,
  `sanitizeSlug`, `slugConflict`, `deriveGroupSlug`, `slugOrId`,
  `effectiveCardView`, `normalizeDensity`, `otherDensity`, `densityLabel`,
  `densityTitle`, `sanitizeRenderedHtml`, `isSafeUrl`, `decodeUrlEntities`.
  These are the seams whose absence let the drop bug ship.
- **The exported plug functions**, driven with a recording `syscall` stub, so
  the tests assert the real syscall sequence: exactly one
  `editor.replaceRange` per action, and no `space.writePage`, `space.readPage`
  or `editor.reloadPage` at all. That is the check that keeps Cmd-Z working —
  a future change that reaches for `space.writePage` fails the suite.
- **The rendered panel**, read back from the `editor.showPanel` call the worker
  makes to redraw the board, or from `buildBoardHtml` directly. That is what
  asserts one container per group, a header carrying the name and the id, no
  per-card group stripe or badge, a selection ring that differs from the
  container by shape, a collapsed group's cards hidden, and that every literal
  colour in the stylesheet is a `:root` theme fallback -- rather than
  eyeballing any of it.
- **The remembered view**, driven with a `clientStore` stub: reopen when it was
  open, stay closed when it was closed, per-page isolation, Close clearing the
  flag, an empty buffer and an overtaking navigation both drawing nothing, and
  a store that throws degrading to a closed board rather than an error.
- **The stylesheet, parsed rather than grepped.** The density tests split the
  `<style>` block into real `{selector, body}` rules with comments stripped
  (`cssRules()` in the test file), because a crude split on `}` drags
  neighbouring rules and comment prose into the answer — which is how a test
  passes while asserting nothing. On top of that: the compact rules add no
  seam and no `font-size` except the collapse chevron's full one, no compact
  rule reaches a card body or the group outline, the resting-chrome rules name
  only the container and its own header bar, no rule puts `opacity` on
  `.board-group`, and the card strip is byte-identical between the two
  densities.

The panel script is not duplicated for testing. `injectSharedFunctions()`
stringifies those same functions into the panel script at render time, and a
test evaluates the injected source and checks it answers identically, so the
panel and the worker cannot drift.

`board_test.go` is the only Go file in this directory. It shells out to
`node --test`, checks the summary reports zero failures and at least a floor
of passing tests (so a suite that silently stops loading is caught, not just
one that fails), and skips when node is absent, so the JavaScript is covered by
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

## What was verified — the two densities and the CSS knobs

**In a real browser**, driving the real panel markup and the real panel script
with real DOM events, against a copy of the real 291-line `Todo/running` (82
cards, 11 named groups, a 10-row markdown table). The panel ran inside a real
iframe, which matters: a harness that ran it in the parent document itself
would find the panel's own defaults in the "parent" and pin every custom
property, so the customization surface would not have been under test at all.

Comfortable, at rest:

- the card header is 33px tall, its slug and id draw in `#787774`
  (`--subtle-color`), and the grip and the three-dot button are at `opacity: 0`.
- a group's outline is `2px` of accent at `0.4` alpha; its bar is a pale accent
  tint with full-contrast `#37352f` text; the collapse chevron is
  `#37352f` at `12px`, `opacity: 1` — legible at rest.
- a member card is `#f7f6f3` with a `1px #e9e9e7` border.

Comfortable, pointer on a **member card** (not the header):

- the group's outline goes to solid `rgb(35, 131, 226)`, its bar to full accent
  with white text and a white chevron. A different group on the same screen
  stays receded.
- the hovered card's slug and id go to `#37352f`, its grip to `0.5`, its menu
  button to `1`.
- **no layout shift**: the group is 1323px, its bar 30px and the card 93px in
  both states.
- **the member card does not change**: `#f7f6f3`, `1px #e9e9e7`, identical.

Interactions, at **both** densities, with dispatched `MouseEvent`s and
`DragEvent`s through the real listeners:

- a plain click on a card body selects exactly that card; `meta`-click adds a
  second; `shift`-click ranges three from the anchor; a plain click on empty
  board background clears; a `mousedown`/`mousemove`/`mouseup` band from empty
  background lassoes the two cards it crossed.
- `dragstart` on the grip fades one unit, `dragover` at 75% of a card's height
  draws exactly one drop marker, and `drop` calls
  `atomdown-board.reorderAtom` with the same unit key and placement at either
  density.
- every card reports a non-zero rectangle and the rectangles stay in document
  order, so `cardGeometry()` feeds `pickDropTarget()` correctly with shorter
  cards.

Compact, after clicking the toolbar switch:

- the card header row computes `position: absolute`, `border-bottom: none`,
  `pointer-events: none`, and its slug and id `display: none`. The card goes
  from 93px to 56px.
- the grip lands 1px from the card's left edge and 4px from its top; the menu
  1px from the right edge and 1px from the top.
- the body reserves 30px of right padding (`6px` + `24px`).
- `document.elementFromPoint()` 4px below a card's top edge returns the **card
  body**, and clicking it selects that card — the strip the header occupies is
  click-through.
- the group bar hides `GROUP`, the id and both buttons, the count reads a bare
  `7`, and the chevron is still `12px` / `1px 5px` / `opacity: 1`.
- content is unchanged: `14px` body text, a `21px` `h1`.
- the density is written to `clientStore` under
  `atomdown-board.density:Todo/running` and to nothing else, and a reload of
  the page draws compact from the markup with no frame of comfortable.

The customization surface, by setting properties on the parent's `html` the
way a `space-style` page would:

- `--board-card-padding: 14px` → a compact card body computes
  `14px 38px 14px 14px`. The value the user asked for beats the compact
  override.
- `--board-accent-color: #b5651d` → the group outline becomes that colour at
  `0.4` alpha, and the lasso, the drop indicator and the selection ring follow,
  because none of them names the theme token directly any more.
- `--board-card-radius: 0px` → square cards.

Dark theme, by switching the parent's `data-theme` and re-copying: the resting
slug is `#9b9b98` on a `#2a2a29` card, the resting chevron `#e8e8e6` — receded
but readable, with no value hand-tuned per theme.

**In the real app**, `iugum wiki` serving a scratchpad copy of the page on a
high port with this plug in the space's `_plug/`, the board opened through
`client.runCommandByName("Atomdown: Toggle Board")` and the panel iframe's DOM
read directly. Through SilverBullet's own `markdown.markdownToHtml`:

- the six-item ordered list card renders **one `<ol>` with six `<li>`**, not a
  run-on paragraph. A block's newlines are what make it a list, and nothing in
  the parse path joins, trims or reflows a block's lines.
- the 10-row ticket table renders one `<table>` with 10 `<tr>` and **nine real
  `<a href>` inside `<td>`**, each with `target="_blank"`. The sanitizer is
  context-free on purpose, so a cell is not a special case: an anchor in a
  `<td>` is treated exactly like one in a `<p>`.
- switching to compact takes that card from 114px to 77px, and the list and
  the cell links are still a list and still links.
- a real browser reload reopened the board **compact**, from the markup.

Both of those were once reported as defects from a test rig whose markdown stub
had no ordered-list branch and did not substitute inline markdown inside table
cells. Neither was ever a defect in this plug. A rig like that also carries a
**hardcoded fallback palette** so it can render standalone; that is a test
fixture and not a theme. The live board takes every colour from the parent
document's own theme tokens — see "Theme".

**The document, with the real `atomdown` binary.** A group, a rename, an
ungroup and a reorder run at compact density against the copy of
`Todo/running`:

- four actions, four `editor.replaceRange` calls, zero `space.*` calls, zero
  `editor.reloadPage`.
- group → rename → ungroup restores the buffer **byte for byte**.
- 82 atoms before and after, the same set of `id|slug|groupId` fingerprints,
  and every `digest` attribute unchanged.
- `atomdown lint` → `ok`, `atomdown verify` → `ok - no drift`, the same answers
  the untouched page gives.
- the words `density`, `compact` and `comfortable` appear nowhere in the
  document.

## What was verified — rendered CommonMark and the raw option

**In a real browser**, driving the panel with real DOM events against the real
291-line `Todo/running` (82 cards, 11 named groups, a 10-row markdown table),
served by `iugum wiki` with the plug installed. The **host's own renderer**, not
a stub:

- A heading card's rendered body is `<h2>RESEA tickets - due tonight</h2>` — a
  real `H2` element, with the raw `<pre>` hidden beside it.
- The table card holds a real `<table>`: 10 `<tr>`, 30 `<td>`. Its rendered
  width is 784px inside an 802px card, so it does **not** widen the card the
  drop geometry measures.
- The link card holds one `<a href="https://ffai.atlassian.net/browse/
  FFAI-62016" target="_blank">`. Bold, inline code and the ordered list all
  render (6 `<strong>`, 3 `<code>`, 6 `<li>` in one card). Zero `<script>`
  elements anywhere in a card body.
- **A click on a rendered link selects the card and does not navigate.**
  `dispatchEvent` returns `false` (so `preventDefault` ran), `defaultPrevented`
  is `true`, `location.href` is unchanged, and the card is the one selected
  card afterwards.
- Selection over the rich DOM: plain click replaces the selection,
  modifier-click adds (2 selected), shift-click selects the range (3 cards),
  and a lasso drawn from empty background selects both cards it crossed.
- Drag geometry over the rendered table: `dragstart` marks one card dragging,
  `dragover` in a top-level card's upper half draws `dropbefore` on that card
  and in its lower half draws it on the next unit, and `dragover` over the
  table card draws one indicator on its group's first card (the whole group is
  one unit). `dragend` clears every marker.
- The three-dot menu opens and now carries **Ungroup** and **Show raw
  markdown**. Clicking the view item flips that card to `data-card-view="raw"`,
  showing `## RESEA tickets - due tonight`, and leaves its neighbour rendered.
  Clicking it again flips back.
- The group header still selects all 7 member cards, and its collapse toggle
  still collapses and expands.
- The toolbar switch moves **all 82** cards to raw and back, and the button
  relabels itself `Rendered` / `Raw markdown` each time.
- The raw bodies of all 82 cards, compared against the page fetched from the
  server, are **byte-exact** matches for the page's 82 blocks.
- Left in raw, then the browser reloaded: the board reopened by itself, all 82
  cards raw, button reading `Rendered`. Persistence works.
- Dark theme: the rendered body's link is `rgb(126, 153, 252)` and its inline
  code background `rgb(32, 32, 32)` — both read from the parent's live tokens,
  so the card has no palette of its own.

**Document integrity.** The live page is **byte-identical** to the copy taken
before any of this, and the real `atomdown` binary reports `lint` **ok** and
`verify` **ok - no drift** on it.

Separately, against a scratch copy of the same page driven through the plug's
exported functions: opening the board rendered, flipping the stored view to raw
and back, and applying a per-card override made **zero** `editor.replaceRange`
calls and **zero** space writes. A real group reorder and its reverse on that
copy kept `lint` ok and `verify` ok - no drift at every step, left every `id`,
`slug` and `digest` untouched, and the round trip came back byte-identical.

## What was verified — the group container, the header bar and view persistence

Against a copy of the real 291-line page (`Todo/running`, 82 atoms, 11 named
groups), driving the exported plug functions through a recording syscall stub:

- Open, Group two adjacent atoms with a name, Rename that group, two reopens
  (one with a group collapsed), Ungroup, Close. **Three `editor.replaceRange`
  calls** for the three content actions, **zero** space writes and **zero**
  `editor.reloadPage`. Opening, reopening and collapsing wrote nothing at all.
- The real `atomdown` binary on the result: `lint` **ok**, `verify` **ok - no
  drift**. The diff against the original is exactly the two group marker
  lines. 93 ids before and after, and all 82 `digest` values byte-identical.
- Ungroup returned the buffer to the original **byte for byte**.
- No banned attribute (`collapsed`, `selected`, `open=`, `x=`, `y=`) appears
  anywhere in the written text.

In a real browser (Chromium via Playwright), driving the panel script with
real DOM events on that page's rendered panel:

- 11 group containers for 11 groups, 82 cards, **zero** `board-card-grouped`
  elements, **zero** `board-badge-group` elements, **zero** "Rename group"
  items. Cards stay in document order, nested or not.
- Header reads `▾ ✥✥ GROUP decisions KATZ94NM 3 cards Rename Ungroup`. Header
  background and container border both resolve to `rgb(35, 131, 226)` -- the
  one accent value.
- Clicking the header selected exactly the group's 3 member cards and nothing
  else. A member card's menu then read **Ungroup**, enabled. Adding a
  standalone card to that selection and opening *its* menu read **Group**,
  disabled, "Atomdown Core 1 does not permit a group inside a group".
- A selected card inside the container computed to `border 2px` +
  `outline 2px` at `outline-offset 2px` with the hover background, against the
  container's single `border 2px` and plain background.
- Collapse hid the cards (`height 0`) while the container stayed visible,
  flipped `aria-expanded` to `false`, and wrote one
  `clientStore.set("atomdown-board.collapsed:Todo/running", ["KATZ94NM"])` --
  and no `editor.replaceRange` and no space write. With a group collapsed,
  **zero** all-zero rectangles reached the drop geometry. Expanding restored
  the cards.
- Rename prefilled `decisions`, previewed `Writes slug="big-decisions".` for
  typed `Big Decisions!!`, and invoked
  `atomdown-board.setGroupSlug("KATZ94NM", "big-decisions")`. Ungroup invoked
  `atomdown-board.ungroupAtoms("KATZ94NM")`.
- Collapsing all 11 groups turned the 291-line page into a one-screen list of
  its 11 group names.

## What was verified — names (slugs)

Against a copy of Steve's real live page (`Todo/running.md`, 291 lines, 82
atoms, 11 hand-written group slugs) and the real `atomdown` binary. The copy
was driven through the plug's own exported functions with the recording
syscall stub, then handed to the binary.

- **Baseline:** `atomdown lint` reports `ok`, `atomdown verify` reports
  `ok - no drift`.
- **Three board actions applied:** grouped two adjacent atoms with the typed
  name "My New Section", renamed the existing `resea` group to "RESEA Work",
  and named one atom "Header Note" through the attribute editor path.
- **After:** `atomdown lint` still `ok`, `atomdown verify` still
  `ok - no drift`.
- **Exactly 3 `editor.replaceRange` calls, one per action, and 0
  `space.writePage` / `space.readPage` / `editor.reloadPage`.** Cmd-Z reaches
  each action.
- **One id added** (the new group's) and **no id lost**. All 82 digests
  byte-identical. Ungrouping the new group afterwards still lints and verifies
  clean.
- **Attribute order confirmed stable:** `atomdown parse | atomdown emit` writes
  the same `id`, `slug`, `digest` order the board wrote. (That round trip is
  not byte-stable on this page even at baseline — emit adds blank lines around
  every directive — so the comparison is the attribute order, not the bytes.)

The panel script itself was verified in a real browser, by serving the exact
`html` and `script` the worker hands `editor.showPanel` for that page and
driving it with real DOM events:

- No load errors. 82 cards. Every group badge reads its slug with the real
  group id in the tooltip; a named atom shows its slug span with the id
  beside it and in the tooltip.
- Selecting two cards and clicking **Group** opens the naming form prefilled
  `running-todo`, derived from the page's own `# Running todo` heading, with
  the hint `Writes slug="running-todo".`
- Typing `Decisions` (already used) changes the hint to
  `Already used by KATZ94NM - allowed, just harder to read.` and leaves the
  confirm button enabled. Typing `!!!` reads
  `No name. The group will show its id instead.`
- Confirming calls `atomdown-board.groupAtoms` with the sanitized
  `my-new-section`, not the raw typed text.
- On a grouped card the menu offers **Ungroup** and **Rename group**; Rename
  opens the same form prefilled `decisions`, and confirming "RESEA Work" calls
  `atomdown-board.setGroupSlug` with `KATZ94NM` and `resea-work`.
- In the attribute editor the labelled slug row is the first row, ahead of the
  generic name/value list.

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
- **Following a link from a card.** A rendered link looks and hovers like a
  link but does not navigate, because a click on it is a card-selecting
  gesture — see "Keeping the interactions alive" above. Open the link from the
  page itself, or from the raw view.
- **Editing in a rendered card.** The rendered body is read-only. Every write
  this plug makes is a directive line or a group marker; a card body is never
  a source of a change.
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
