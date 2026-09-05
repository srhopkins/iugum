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
| **The icon** | An action button in the header bar, next to home and terminal. Per page, off until you press it, and remembered across a reload. |
| **Cards** | One closed rounded box per atom: 1px border, the card surface, padding, and a clear gap to the next card. |
| **Card header** | A strip at the top of each box: drag grip, the readable name in body text, the id in small grey monospace. |
| **Groups** | One closed rounded 2px accent box around the member cards, which are inset inside it on all four sides. |
| **Group header** | A bar inside the top of that box: collapse caret, drag grip, GROUP, the name, the id, the card count, Rename and Ungroup. |
| **Editing** | Ordinary typing. There is nothing to open and nothing to save. |
| **Drag to reorder** | Drag the grip that appears at the left of a hovered block. |
| **Lasso** | Alt-drag a band over several blocks to select them. |
| **Group / Ungroup** | `Atomdown: Group Selection` on a lassoed run, `Atomdown: Ungroup` with the cursor in a group, or the group header's menu. |
| **Collapse** | The header caret, through the editor's own folding. |

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
| module memory | the lasso selection, and which groups this session folded |

The selection and the fold state are deliberately not persisted: a stale
selection on reload would be a lie about what is selected, and a page draws
every group open after a rebuild, which is the truthful state.

## Tests

```sh
node --test plugs/atomdown-inline/    # directly
go test ./plugs/atomdown-inline       # same tests, through go test ./...
```

59 cases. They import the real plug file with only the worker globals stubbed,
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
document immutability, and each primary component's existence, position and
behaviour - across both densities, all four editor widths and both themes.

A pre-push hook runs it automatically when a push touches this directory. Full
detail, the matrix split and the escape hatch: `plugs/atomdown-e2e/README.md`.
