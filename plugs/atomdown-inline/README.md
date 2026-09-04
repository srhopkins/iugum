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
| **Cards** | A thin continuous outline down the lines of each atom's block. |
| **Groups** | A 2px accent outline around each `atom-group`, quiet until the page is hovered. |
| **Group header** | A bar above the group's marker line: drag grip, collapse caret, the group's readable name, how many atoms it holds, and a menu. |
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
| `lines` | `CommentBlock` and `Comment` → `atomdown-directive` |
| `marks` | one per unit, `unit:atom:<id>` / `unit:group:<id>`, plus one `card:<group>:<n>` per atom inside a group, plus one `sel:<unit>` per lassoed unit. Every one has `lineClasses`, which is what draws a continuous outline across a multi-line block. |
| `widgets` | one `inline` grip per movable atom, one block header bar per group |
| `folds` | one per group: everything after its opening marker line |
| `events` | `click` and `selection` |
| `gestures` | `drag` by the `atomdown-grip` handle, `lasso` on `alt` |

Mark ids are namespaced, and that is load-bearing. The seam reports covering
marks outermost first, so a drag that starts inside a group reads
`unit:group:...` before `card:...` and moves the whole group — which is what an
`atom-group` means, and what keeps a group contiguous by construction. The
`card:` and `sel:` names are never read as units.

## The directive comments

They are **dimmed and shrunk to 78%, not hidden.** Hiding them was rejected:
the page is the editor, so a reader can still put a cursor in a line that is not
on screen, and an edit would then land somewhere invisible. A hovered or active
directive line comes back to near full strength, so reading an `id` costs one
hover.

The rule is by Lezer node name, so a non-Atomdown HTML comment on the page is
dimmed too while the view is on.

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

- The header bar's `⋯` menu sits at the right edge of the content column. In a
  narrow pane it can be clipped by the scrollbar.
- The collapse caret alternates fold and unfold from memory, because the host
  offers `editor.fold` and `editor.unfold` but no read of the fold state. Fold a
  group from the gutter instead and the caret's next press is out of step once.
- A new block typed into the page gets its card on the next autosave
  (`editor:pageSaved`), not on the keystroke.
