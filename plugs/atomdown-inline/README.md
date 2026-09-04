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

## Install

Three files, all copies into the space:

```sh
cp atomdown-inline.plug.js         /path/to/space/_plug/atomdown-inline.plug.js
cp "library/Atomdown Inline.md"    "/path/to/space/Library/Atomdown/Inline.md"
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
| `marks` | **two kinds.** An identity mark per unit, `unit:atom:<id>` / `unit:group:<id>`, over the unit's whole source span, with no line classes and no CSS. And a box mark with `lineClasses` over just the unit's visible lines: `box:atom:<id>`, `box:group:<id>`, `card:<group>:<n>` for an atom inside a group, `sel:<unit>` for a lassoed one. |
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

They are **hidden at rest**, collapsed to a 3px sliver, and revealed in full
the moment the cursor is on that line. On a real page every atom carries a
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

## Known limits

- The collapse caret alternates fold and unfold from memory, because the host
  offers `editor.fold` and `editor.unfold` but no read of the fold state. Fold a
  group from the gutter instead and the caret's next press is out of step once.
- A new block typed into the page gets its card on the next autosave
  (`editor:pageSaved`), not on the keystroke.
