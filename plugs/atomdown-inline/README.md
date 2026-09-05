# atomdown-inline

The Atomdown card view drawn **on the normal SilverBullet page**, in the normal
theme, turned on by an icon in the page header bar.

`plugs/atomdown-board` is the other view of the same document: a full-screen
panel with its own cards and its own editor. This one has no editor and no
panel. The page *is* the editor, so typing in a card is typing in the document,
and the plug only decorates what is already there.

## What you get

| | |
|---|---|
| **The icons** | Two action buttons in the header bar, next to home and terminal: `grid` turns the card view on, `align-justify` switches the display density. Both are per page, and both are remembered across a reload. The **plug registers them itself** - see "The header buttons" below. |
| **Cards** | One closed rounded box per atom: 1px border, the card surface, padding, and a clear gap to the next card. |
| **Card header** | At comfortable density, a strip at the top of each box: the readable name in body text, the id in small grey monospace. Compact removes it. |
| **The card's two controls** | OUTSIDE the card, in the page gutter: the braille drag grip on the left, a vertical three-dot menu on the right. Hover-only, at both densities. See "The controls sit outside the card" below. |
| **Groups** | One closed rounded 2px accent box around the member cards, which are inset inside it on all four sides. |
| **Group header** | A bar inside the top of that box: collapse caret, drag grip, GROUP, the name, the id, the card count, and a menu holding Rename and Ungroup. Compact thins it to caret, name, a bare count and the menu. The group's menu stays ON the bar, because the bar is the group's own top edge. |
| **The menu** | A popover anchored to the card or the bar it belongs to, with an inert name-and-id label as its first row. NOT the host's filter picker. See "The popover" below. |
| **Density** | Comfortable (the default) and compact, the panel's own two. `Atomdown: Toggle Inline Density`, or the `align-justify` button. |
| **Editing** | Ordinary typing. There is nothing to open and nothing to save. |
| **Drag to reorder** | Drag the grip that appears in the gutter left of a hovered block. |
| **Lasso** | Alt-drag a band over several blocks to select them. |
| **Group / Ungroup** | `Atomdown: Group Selection` on a lassoed run, `Atomdown: Ungroup` with the cursor in a group, or the group header's menu. |
| **Collapse** | The header caret, through the editor's own folding. |

## The two densities

The same comfortable and compact the `atomdown-board` panel has, with the same
names, the same knob values and the same `--board-*` CSS custom property names,
so the two views cannot drift. Comfortable is the default.

**Compact compresses chrome only.** Not one content size changes: a heading is
a heading at its full rendered size at both densities, because the point is to
fit more of the document on screen, not to shrink the document.

| | comfortable | compact |
|---|---|---|
| Card header row | a strip with the grip, the name and the id | **gone** - no header, no background, no border, and deliberately no dotted line standing in for it. There is no seam. |
| The card's top edge | on the header strip | on the card's own first line |
| The name and the id | in the header | in the three-dot menu, whose picker leads with a `name  -  id` label that does nothing when chosen |
| The grip and the menu | in the header strip's padding gutters | a zero-height layer pinned across the card's top edge, `pointer-events: none` except on the two controls, so a click on the top strip falls through and still selects the card |
| Group bar | caret, grip, GROUP, name, id, count, menu | caret, grip, name, a bare count, menu |
| Group outline | 2px accent | **identical** - the outline is structure, not chrome |
| Collapse caret | full size | **identical** - it is the control that turns a long page into a list of group names |
| `--board-card-padding` | 14px | 6px |
| `--board-card-radius` | 6px | 4px |
| `--board-card-gap` | 14px | 6px |
| `--board-group-padding` | 8px | 4px |
| `--board-group-header-padding` | 5px 8px | 1px 4px |
| `--board-card-header-padding` | 4px 8px | 0 |

**How the density reaches the line elements.** A `.cm-line` gets a class only
from a mark's `lineClasses`, and the seam uses `marks[].class` as the STEM of
the classes it derives - so a second class in that string would produce
`atomdown-card atomdown-compact-line` rather than two classes. The plug
therefore emits a THIRD kind of mark, `dens:<unit>`, carrying nothing but
`atomdown-comfortable` or `atomdown-compact`. Widgets take the class directly,
because `widgets[].class` is used verbatim. The class is present at both
densities, so the DOM says which density is showing instead of leaving it to an
absence.

**One density mark per TOP-LEVEL UNIT, never one per card.** A unit's span
already covers every line of every card inside it. One per card as well put
`atomdown-comfortable-line` on a member card's line twice, with `-first`,
`-mid` and `-last` together, and how many of the overlapping marks the editor
had realised varied with the scroll position - so an editor-width round trip
came back with a different class string for a state that had not changed.

Switching density needs **no** `editor.rebuildEditorState`: mark classes and
widget classes live in one StateField the seam rebuilds whenever the config
value changes, so the empty transaction is enough and the undo history survives
a switch.

## The header buttons

The plug registers both of them, with `config.insert` on the `actionButtons`
key. There is no `actionButton.define` block on the library page any more.

Why it moved. An action button is plain configuration - `actionButton.define`
in Space Lua is three lines that read that key, append to it and write it back
- and the `config.insert` syscall is open to a plug
(`silverbullet/client/plugos/syscalls/config.ts`). A `space-lua` block, though,
only runs when the client's own page index holds that page, and that index is
per browser origin and can be silently stale: the same server showed a button
whose command was "not found" in one tab and a page with no styling at all in
another. A plug loads by file discovery with no index involved, which is why
the COMMANDS always worked when the button did not.

Registration runs on `editor:init`, on every page load, and on a **heartbeat**
every five seconds, and it skips a button whose command is already in the list.
All of that is needed, and each piece is a measured failure rather than
caution:

* `Config.clear()` wipes every config value each time Space Lua reloads
  (`silverbullet/client/client_system.ts`, `loadLuaScripts`), and the boot
  order is plugs → `plugs:loaded` → Space Lua → `editor:init`, so a button
  registered on the earlier event is gone before the header bar reads the key.
* Space Lua reloads AGAIN when a space finishes its first index
  (`client/data/object_index.ts` dispatches `editor:reloadState`), which is
  after `editor:init` has been and gone. Hence the heartbeat. Listening to
  `editor:reloadState` cannot serve instead: `EventHook.dispatchEvent` queues
  plug handlers and the client's own local listeners as concurrent promises, so
  this plug's first syscall and that listener's `config.clear()` race, and the
  clear wins.
* **The whole array cannot cross the worker boundary.** The standard library
  defines the back and forward buttons with a `run` CALLBACK, so a
  `config.get("actionButtons")` had to structuredClone Lua functions into the
  worker and threw `could not be cloned`. The plug caught its own failure and
  registered nothing, and the header bar had no Atomdown button while both
  commands worked. It reads `actionButtons.length` and
  `actionButtons.<i>.command` instead - one clonable scalar at a time.
* **Nothing subscribes to config**, so the header bar renders from a value it
  read earlier. The plug nudges exactly one re-render (`editor.showProgress`
  with no arguments, which sets the progress indicator to what it already is)
  and only when it actually appended something.

The skip is what stops a reload, or the heartbeat, from producing two grid
icons; the appends are also serialized, so a page load and a tick in flight
together cannot both append.

The CSS still lives on the library page, in one `space-style` block, and the
density's rules are in that same block - so they cannot arrive separately from
the rules they modify.

## The collapse caret, and why it cannot go out of step

The collapsed set is the state, and one press flips one entry of it. The seam
then makes the editor's fold set match, on EVERY editor update - so the flag
owns the state of a group, and a fold made from the gutter, by the fold command,
or by CodeMirror's own "clear the folds covering the cursor" rule is put back to
what the flag says. The caret cannot end up one press behind. The one exception
is the reader's: a group holding the text cursor is not folded, because that
would hide the cursor inside it.

Three separate defects made the caret look dead before, and all three are gone:

* **The wrong group.** A click in a block widget reports the position of the
  nearest TEXT, and once a neighbouring group is folded that position is in the
  neighbour's range. Reading the click's `unit:` mark therefore named the wrong
  group. A control in a widget now takes its unit from the widget's own name,
  which this plug put there.
* **The cursor clearing a fold.** CodeMirror places the text cursor on
  mousedown, and its fold state drops any fold covering the selection head, so
  pressing a caret with several groups shut reopened a group nobody pressed. The
  seam now stops a press on widget chrome from placing the cursor at all.
* **A lost update.** Each press reads the set, changes one entry and writes it
  back, with four syscalls in between. Two presses in flight together both read
  the set as it was before either, so the second write lost the first press.
  Presses are queued.

A fourth cause was not in this plug at all, and it is worth knowing because a
space can still have it: a hand-copied bundle in `_plug/` runs the plug a SECOND
time next to the compiled one, and the two instances undo each other. See
Install below; `iugum wiki` warns and names the files.

The set is remembered per page in `clientStore`, so a reload draws the groups
the reader left shut.

## Install

**Nothing to copy.** The plug bundle and the library page are compiled into the
SilverBullet binary `iugum wiki` runs, under `Library/Atomdown/`, and every
space reads them from there. See `docs/wiki-space-assets.md`.

**Delete a hand-copied bundle if you have one.** SilverBullet loads EVERY
`*.plug.js` the space can see, so a leftover `_plug/atomdown-inline.plug.js`
does not override the compiled one — it adds a second instance, and the two
answer every click and write the same config key. `iugum wiki` names the files
when it finds them:

```sh
rm /path/to/space/_plug/atomdown-inline.plug.js
```

To run a working-tree copy instead of the compiled one, put it where the binary
keeps it, so the space file shadows the underlay rather than joining it:

```sh
cp atomdown-inline.plug.js "/path/to/space/Library/Atomdown/Plugs/atomdown-inline.plug.js"
cp "library/Atomdown Inline.md" "/path/to/space/Library/Atomdown/Inline.md"
```

Then reload the browser once. The library page carries the two things a plug
cannot supply: the action button (`actionButton.define`, so no client change is
needed for the icon) and the CSS (`space-style`, so the view follows the space's
light and dark theme).

The plug needs a SilverBullet client that carries the **editor decoration
seam** — the one patch iugum keeps in `silverbullet/`. See
`docs/silverbullet-decoration-seam.md`. The embedded release binary does not
have it; run `IUGUM_WIKI_SB_SRC=./silverbullet iugum wiki`.

## How it draws

The plug writes one plain-data object to the `editorDecorations` config key and
the seam turns it into CodeMirror decorations. The plug never touches
CodeMirror; it runs in a web worker that has none.

For the page in `atomdown/testdata`-shape, one atom and one two-atom group:

| | |
|---|---|
| `activeLine` | `true`, so the cursor's line carries `cm-activeLine` |
| `lines` | `CommentBlock` and `Comment` → `atomdown-directive` |
| `marks` | **two kinds**, and the box kind carries `hoverClasses`. An identity mark per unit, `unit:atom:<id>` / `unit:group:<id>`, over the unit's whole source span, with no line classes and no CSS. And a box mark with `lineClasses` over just the unit's visible lines: `box:atom:<id>`, `box:group:<id>`, `card:<group>:<n>` for an atom inside a group, `sel:<unit>` for a lassoed one. |
| `widgets` | one header row per card, one header bar per group |
| `folds` | one per group: everything after its opening marker line |
| `events` | `click` and `selection` |
| `gestures` | `drag` by the `atomdown-grip` handle, `lasso` on `alt` |

**Why the marks come in two kinds.** A box has to start on a line the reader
can see, and an atom's first source line is its directive, which is hidden. A
box also must not cover the blank line between two blocks, because that blank
line IS the gap between two cards. So the range that draws the box is not the
range that identifies the unit, and separating them is what makes both correct.

**Which means a release can land on no mark at all, and that is where a drop
goes wrong.** The blank line between two cards belongs to neither box, so a
drag released there reports an empty `targetMarks`. It is not a rare miss: the
grip is in the gutter, so the natural gesture is to press it and travel
straight down, and the seam under a card is exactly what the pointer crosses.
`dragToReorder` therefore resolves such a release BY POSITION — the drop goes
before the first unit that begins at or after the release line, and only a
release past the last unit is the end of the document. Card order IS document
order here, so the next unit down the page and the next unit in source order
are the same unit, and no rectangle has to be measured. Reading the empty
target as "the end of the page" instead is `iugum-uuv`: the first card,
dragged down one position, landed at the bottom of an 84-card page. Guarded by
rule 8i (`plugs/atomdown-e2e/8-drag-drop.test.ts`), which asserts the whole
resulting unit order.

Mark ids are namespaced, and that is load-bearing. The seam reports covering
marks outermost first, so a drag that starts inside a group reads
`unit:group:...` before `card:...` and moves the whole group — which is what an
`atom-group` means, and what keeps a group contiguous by construction. The
`box:`, `card:` and `sel:` names are never read as units.

**How the group's chrome knows the pointer is inside it.** The panel's group
border and header bar are subdued at rest and come forward when the pointer is
anywhere in that group, including over a member card. Inline, a group is a run
of sibling line elements with nothing wrapping them, and CSS has no
previous-sibling combinator, so `:hover` cannot reach the lines above the
pointer. The seam's `hoverClasses` puts `atomdown-group-hover` on every line of
the group the pointer is in; the header bar reaches the same state through
`:has(+ .atomdown-group-hover)`, because its next sibling is its own group's
first line. The quiet state is a `color-mix` on border and background - never
`opacity`, which would fade every member card inside the group.

**How a box is drawn out of lines.** `lineClasses` gives `-first`, `-mid` and
`-last`. `-mid` takes the sides, `-last` takes the bottom edge and the bottom
corners, and the card's top edge and top corners are on the header widget
directly above the box. A one-line block is both `-first` and `-last`, so it
closes the whole box on its own line and has its own rule. A soft-wrapped
paragraph is ONE line element with several visual rows, and a border on a block
element encloses the whole box, so a wrapped block is enclosed by construction.

The group is the outer of two boxes on the same line elements, and one element
can only have one `border-left`. So the group takes the real `border` and the
card is drawn by a `::before` inset by `--board-group-padding` — which is also
what insets a member card inside its group on all four sides.

## The directive comments

They are **hidden at rest** - every one of them, including the document-level
`<atomdown version="1"/>` marker, which is the same Lezer node and takes the
same rule. Collapsed to a 3px sliver, and revealed in full the moment the
cursor is on that line **with the editor focused**. The focus condition matters:
SilverBullet puts the cursor at offset 0 on a page load, which is the document
marker's own line, so without it that one directive would always be revealed on
arrival and look like a bug. On a real page every atom carries a
64-character `sha256` digest that wraps over three or four rows, and 93 of
those is the single biggest reason a decorated page stops reading as cards.

Collapsed rather than `display: none`, so the line element stays in the layout
and CodeMirror's own cursor and coordinate maths are untouched. That is also
what keeps the original reason for dimming: an edit can never land in a line
nobody can see, because putting the cursor there reveals it.

With the directive hidden, the atom's `id` would have nowhere to appear, which
is why every card has a header row carrying the name and the id.

The rule is by Lezer node name, so a non-Atomdown HTML comment on the page is
hidden too while the view is on.

## The write path, and undo

Every change — a reorder, a group, an ungroup, a rename — is computed as a whole
new document, then reduced to the smallest single replacement (`minimalEdit`)
and applied with `editor.replaceRange`. That is ONE CodeMirror transaction and
therefore ONE entry in the editor's own undo history: native Cmd-Z reverts a
whole reorder in one step. There is no undo stack in this plug.

The decorations for the new text are written to config **before** the edit, so
the seam rebuilds them from the post-edit document inside that same
transaction. `editor.rebuildEditorState` is deliberately **not** called after an
edit: it calls `setState`, which throws the undo history away. It is called only
when the view is turned on or off, or on a page load.

No reorder, group, ungroup or rename ever rewrites an `atom` directive line, so
no `id`, no `slug` and no `digest` can change. Verified with the real binary:
`atomdown lint` stays `ok` and `atomdown verify` reports `no drift` after each
of those flows; only typing into a block produces drift, and only for that atom.

## State

Nothing this plug remembers reaches the document.

| where | what |
|---|---|
| `clientStore`, `atomdown-inline.on:<page>` | `true` while the view is on for that page |
| `clientStore`, `atomdown-inline.collapsed:<page>` | the ids of that page's collapsed groups |
| `clientStore`, `atomdown-inline.density:<page>` | `"compact"` when that page was left compact; absent means comfortable |
| module memory | the lasso selection |

One store, one key shape, one mechanism. Comfortable is the ABSENCE of a
density value rather than a value, so a page never switched and a page switched
back read identically.

The lasso selection is deliberately not persisted: a stale selection on reload
would be a lie about what is selected.

## Tests

```sh
node --test plugs/atomdown-inline/    # directly (needs node 20 or later)
go test ./plugs/atomdown-inline       # same tests, through go test ./...
```

124 cases. They import the real plug file with only the worker globals stubbed,
and most of them are about the decoration payload: every offset in it is
checked against the page text it was built from, because a wrong offset there is
the whole bug class this feature can have.

## Why the page is in a monospace face

It is SilverBullet's, not this plug's. `--editor-font` defaults to
`"iA-Mono", "Menlo"` in the client's own theme, so a markdown page in
SilverBullet's editor is monospace before any plug is installed. Neither
atomdown view sets a family of its own, and rule 5 of the front-end suite holds
that: a card's body font has to be exactly the space's `--editor-font`, so the
two views cannot drift from each other or ignore a space that sets it.

To read prose in a proportional face, set it once in your own `space-style`
page and both views follow:

```css
html { --editor-font: Charter, Georgia, serif; }
```

Monospace stays where it belongs on its own: the id chip, inline code and a
fenced block are all spans inside a card.

## A wide table

A table is constrained to the card's content width - `table-layout: fixed` with
wrapped cells - rather than being given a horizontal scroller. A card's body is
a run of `.cm-line` elements that CodeMirror owns and measures; making a line
scroll horizontally would put text outside the box `posAtCoords` reads from and
break cursor placement. Wrapping keeps every column reachable with no scrolling
at all, and the table can then never cross the card's border or the group's. The
panel wraps its cells too, so the two views agree.

## A row that shows raw link markdown

If a table row shows `[FFAI-1234 "[nice to have] Thing"](https://...)` as
literal text while its neighbours render links, that is the markdown, not this
view. An unescaped `[...]` inside a link label closes the label early, so the
construct is not a link. Plain SilverBullet with the view off and the
`atomdown-board` panel both show the same row raw, because all three use one
parser. The fix is to escape the inner brackets in the page.

## The controls sit outside the card

The drag grip is in the LEFT page gutter and the vertical three-dot menu is in
the RIGHT one, both outside the card's own border. Steve's change: it gives the
card its full column width back, it is what makes the compact density actually
compact, and it leaves the header row free for whatever goes there next.

**The offset is 22px from the card head's border box** (`--board-chrome-gutter`),
and the number is measured rather than chosen. The space outside a card's
border is CodeMirror's own content padding (20px) plus the page margin, and the
page margin is narrowest at the `full` editor width: `min(1600px, 96%)` on a
1440px viewport leaves 28.8px, so the budget at the worst width is 48.8px. A
22px offset with a ~16px control ends 6px clear of the card and 26.8px clear of
the scroller's own edge.

**One rule covers both nesting cases.** A member card's head is inset by the
group's interior padding and sits inside the group's 2px outline, so the same
-22px lands 12px OUTSIDE that outline. A top-level card's head has no inset, so
it lands 22px outside the card border.

**Nothing is clipped, and that was verified rather than assumed.** The controls
are CodeMirror line decorations, so the obvious risk was the editor's own
overflow. Measured on the real fixture at all four widths, by rect AND by
`document.elementFromPoint` - a clipped element still reports its full
unclipped rect, so only a hit test is proof. `.cm-scroller` is the one ancestor
between a card and the body whose overflow is not `visible` (it is `auto`), and
it spans the whole viewport while the content column is centred inside it, so a
control 22px outside the column is well inside the clipper. No horizontal
scrollbar appears either: `scrollWidth` equals `clientWidth` at every width.
The probe that recorded this is `plugs/atomdown-e2e/clip-probe.test.ts`.

**A control in the gutter is NOT part of the card's click target.** Decided
explicitly. The card's click target is its own line run - the region the seam
covers with the card's mark - and the gutter is covered by no mark, so a click
there reports no unit and changes no selection. Two reasons that is right
rather than an omission: the gutter is where a reader clicks to put the cursor
at the start of a line, and turning that into "select this card" would take an
ordinary editing gesture away; and each control's own click is handled before
the selection rules, so widening the target would only change what a MISS does.

Rule 1 of the front-end suite enforces all of it. It distinguishes CONTENT,
which must stay inside the card, from CHROME, which may sit outside and must
instead be painted where its rect says it is, stay inside the editor's scroll
container, and not land on the group's accent outline.

## The popover

The three-dot control opens **this plug's own popover**, anchored to the card,
with an inert `name  -  id` label as its first row. It used to call
`editor.filterBox`, which is the command palette's own surface: a centred box
with a search field, listing the card's identity and its actions as palette
rows. That read as "a command palette opened", not "this card's menu opened",
and it is not what the board panel does.

**How it works with no script.** A seam widget's HTML is set with `innerHTML`,
so it carries no listeners - but a click inside a widget comes back to the plug
as `editor:decorationClick` carrying the class list of the element that was
hit. Every row names its own action in a class, `atomdown-mi-<action>`, and the
click handler reads it. A click on the label, or on the popover's own padding,
matches no action and leaves the menu open; a click anywhere else closes it.

**There is no text input in it, and that is measured, not stylistic.** The
seam's `widgetPressGuard` returns true for a plain press inside any widget,
which makes CodeMirror call `preventDefault` on `mousedown`, and a prevented
`mousedown` moves no focus. Measured on the real fixture
(`plugs/atomdown-e2e/input-probe.test.ts`): a click on an input inside a card
header widget left `document.activeElement` on `.cm-content`, and **the
characters typed next went into the document**. An in-popover attribute form
would therefore type the reader's attribute values into the page, which is the
one thing this view may never do. So every row is a button, and text entry goes
somewhere that can hold focus: `editor.prompt` for a single value, and the
attribute form below for a whole directive line. Rule 8g asserts the popover
holds no input, so the day someone adds one, the suite says why they cannot.

**No Escape key.** A plug cannot listen for a keystroke, and binding a command
to Escape would consume it everywhere else in the editor. The popover closes on
a second press of its button, on any click outside it, and on any action.

## The attribute form

"Edit attributes" in a card's menu opens **a form**, the way the board panel's
own menu does: the name (slug) first in its own labelled field, the id shown in
a disabled row, then one name/value row per remaining attribute, with Add
attribute, Remove, Cancel and Save.

**It is not in the popover, and that is the measured constraint above rather
than a layout preference.** An input inside a seam widget takes no focus from a
click, and the characters typed next go into the document. So the form needs a
surface that can hold focus, and this host already has one: a SilverBullet
panel. `editor.showPanel` renders its HTML in an iframe outside the editor's
DOM entirely, where the press guard cannot reach, and that is the very surface
the board panel's attribute form has always run in. The panel slot is `modal`,
which is the one slot that does not resize the editor, so opening the form
moves no card.

**Why not simply open the board panel**, which was the first suggestion. The
board renders every card on the page and covers the whole window, so an
attribute edit would mean leaving the page and losing the reading position; and
driving one card's popover from another plug needs a message into the board's
iframe that does not exist. The form is one atom's attributes.

**Rules the write path holds, all unit-tested on a pure function
(`setAtomAttrsInSource`) rather than through the browser:**

- the id comes from the SOURCE LINE, never from the form, so no route through
  here can edit an id. An `id` row in the payload is ignored.
- attribute order is id, then slug, then the rest, which is the order `atomdown
  emit` writes.
- the slug is sanitized and an empty one removes the attribute.
- **a value carrying directive syntax is REFUSED, not escaped** - the board's
  rule for a pasted directive, and the same stated reason: escaping would
  silently store something other than what the reader typed.
- **and the rewritten line is re-parsed before it is returned.** The refusal
  list is a blocklist and a blocklist is a guess; reading the line back with
  this plug's own regex and requiring the same id and the same pairs is the
  property itself. An emptied directive fails here rather than reaching the
  page.
- one `editor.replaceRange`, so one undo reverts a whole save.

"Show the directive line" is the row that used to be called Edit attributes: it
puts the cursor on the directive, where the peek shows the raw bytes. It is
still there because reading the bytes in place is still useful.

Rules 8j, 8k and 8l of the front-end suite assert the focus, the shape and the
refusals.

## The group's two controls, outside the group container

Same treatment as the card's, and for the reason Steve gave: "the cards and
groups were supposed to have same behavior, groups still show inside the
container". The group's drag grip is in the LEFT page gutter and its vertical
three-dot menu in the RIGHT one, both `--board-chrome-gutter` (22px) outside
the group header widget's own border box, which is the group box's top edge and
carries the 2px accent outline. Both are hover-only, revealed by a hover
anywhere inside the group - including over a member card - and by keyboard
focus.

**They sit on the bar's row**, not at the container's vertical centre. That is
also the collision rule with a member card's own gutter controls: the bar is a
row of its own above every member, so the two sets can never share a row. A
second separation makes it forgiving rather than exact - a member card's head
is inset by the group's padding plus the outline, so its controls sit in a lane
about 10px inboard of the group's.

**The bar keeps** the collapse chevron, GROUP, the name, the id and the count.
The chevron stays inside the bar at full size and always visible at both
densities. It is also the only control left on the saturated accent fill, so it
keeps the translucent hover wash; the three-dot menu, now on the page's own
ground, takes the card menu's opacity-and-colour treatment instead.

## Visual baselines, and how to update them

Rule 9 of the front-end suite compares images, because no behavioural
assertion can see that a hover paints a white hole in a coloured bar - which is
exactly the defect it was written for.

**One command, after an intended change:**

```sh
scripts/atomdown-fe-check.sh --visual --update-snapshots
```

That is deliberately cheap. A baseline that is expensive to re-take is a
baseline someone eventually silences by raising the threshold, and the
threshold here is zero (`maxDiffPixels: 0`) precisely so it cannot drift
quietly.

The environment is pinned and is a hard requirement: headless only, Playwright's
own bundled Chromium at the version in `silverbullet/package-lock.json`, never
the system Chrome and never an `executablePath`. A headed run, `--headed` or
`PWDEBUG` refuses before the first capture, so it can neither produce nor
update a baseline. The resolved browser build is recorded beside the images in
`plugs/atomdown-e2e/visual-baselines/browser.txt`, so a browser upgrade reports
itself in words instead of as a mysterious pixel diff. Full detail, including
how fonts are handled and what the residual limit is:
`plugs/atomdown-e2e/9-visual.test.ts`.

## Known limits

- The card padding rules carry a `#sb-main .cm-editor` prefix. The client's own
  `#sb-main .cm-editor .cm-line { padding: 0 }` is specificity (1,0,2) and beats
  any two-class rule however late it is injected, which left card bodies flush
  against their own border.
- A group whose chrome should stay forward because it *holds* a selected member
  card does not: only a selected group itself stays forward. The panel uses
  `:has()` on a real container element, which inline does not exist.
- A new block typed into the page gets its card on the next autosave
  (`editor:pageSaved`), not on the keystroke.

## Not done until the front-end suite passes

The unit tests in this directory cover pure functions. They cannot see a
geometry or a visibility defect, and a whole evening of those got past them.

**A change to this plug is not done until this passes:**

```sh
scripts/atomdown-fe-check.sh
```

It measures the rendered document in a real browser: containment, directive
invisibility, layout stability, state round trips, rendering fidelity,
document immutability, each primary component's existence, position and
behaviour, the card's own two controls, and a set of visual baselines -
across both densities, all four editor widths and both themes.

A pre-push hook runs it automatically when a push touches this directory. Full
detail, the matrix split and the escape hatch: `plugs/atomdown-e2e/README.md`.
