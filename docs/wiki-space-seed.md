# The wiki space seed

`iugum wiki` (and the wiki inside `iugum up`) writes the atomdown assets the
program carries into the space it serves. Before this, the assets were source
files under `plugs/`, and every space needed a hand copy. A space that got the
plugs and no library page looked broken: the plug applies classes, and no
stylesheet matches them.

## What the program carries

| File in the space | Source in this repo | What it does |
|---|---|---|
| `_plug/atomdown-board.plug.js` | `plugs/atomdown-board/atomdown-board.plug.js` | the full-screen board panel |
| `_plug/atomdown-inline.plug.js` | `plugs/atomdown-inline/atomdown-inline.plug.js` | the card view on the page |
| `Library/Atomdown Inline.md` | `plugs/atomdown-inline/library/Atomdown Inline.md` | the header-bar button (`space-lua`) and the card CSS (`space-style`) |

Root package `main` embeds the three files and hands them to package
`spaceseed`, the same shape as `embedbin` for the SilverBullet binary. The
`*.plug.yaml` files are build inputs and `plugs/atomdown-e2e` is a test suite,
so neither one ships.

The style pages in Steve's spaces (`Library/Styles/Notion.md`,
`Library/Styles/EditorWidth.md`) do **not** ship. Both change how every page
looks, and `Notion.md` carries a company logo. They are a preference, not
something atomdown needs.

## The rules

The seeder is additive, and it runs once per space per process.

| The file in the space | What happens | The log |
|---|---|---|
| absent | written, with the directory made | `wiki: seeded <file>` |
| the same bytes as the built-in copy | nothing | silence |
| an older seeded copy that nobody changed | replaced | `wiki: updated <file> (was seeded by an older iugum)` |
| different from the built-in copy | kept as it is | `wiki: kept <file> (…iugum changed nothing)` |

Nothing correct prints a line.

## The manifest

`<space>/.iugum-seed.json` records a seed version and the SHA-256 digest of
each file the program wrote. The name starts with a dot, so SilverBullet does
not index it as a page.

```json
{
  "iugum_seed_version": 1,
  "assets": {
    "Library/Atomdown Inline.md": "sha256:…",
    "_plug/atomdown-board.plug.js": "sha256:…"
  }
}
```

The digest is what separates an old seed from a hand edit. A file that matches
its recorded digest is a copy this program wrote, so a newer program replaces
it. A file that matches neither the record nor the built-in copy is somebody's
work, so the seeder keeps it. A space with no manifest keeps every file that is
already there.

`spaceseed.Version` counts the asset set. Raise it when an asset changes. A
space whose manifest holds a lower number reads as stale, and the log says so.

## Reindex

SilverBullet applies a `space-style` block and loads `space-lua` from the
index, and a browser reload does not rebuild it. Indexing is client work, so
the server cannot start it. When the seeder writes or updates a file, it prints
one line:

```
wiki: run "Space: Reindex" from the command palette once, so SilverBullet loads the new space-lua and space-style.
```

It prints that line only then.

## Switches

`IUGUM_WIKI_SEED` controls the seeder.

| Value | Behavior |
|---|---|
| unset | additive: write what is missing, keep what changed |
| `force` | take the built-in copy, over a changed file too |
| `off` (or `0`, `no`) | touch nothing in the space |

Use `force` for a deliberate refresh, after you move your own copy aside.

## Still by hand

The inline plug needs a SilverBullet client with the editor decoration seam
(`docs/silverbullet-decoration-seam.md`). The embedded release binary does not
carry it, so the inline view needs `IUGUM_WIKI_SB_SRC=./silverbullet`. The
board panel works with the embedded binary.
