# The wiki space assets

Every SilverBullet space that `iugum wiki` serves has the atomdown plugs and
the page that carries their header button and their CSS. No space holds a copy
of them. They are compiled into the SilverBullet binary, which iugum embeds in
turn.

Before this, the assets were source files under `plugs/`, and every space needed
a hand copy. That broke four times. The last time, a space had both plug
bundles and no `Library/` directory at all, so the CSS and the header button
were absent and the feature looked broken rather than uninstalled.

## Where the assets live

| Path in every space | Source in this repository | What it does |
|---|---|---|
| `Library/Atomdown/Plugs/atomdown-board.plug.js` | `plugs/atomdown-board/atomdown-board.plug.js` | the full-screen board panel |
| `Library/Atomdown/Plugs/atomdown-inline.plug.js` | `plugs/atomdown-inline/atomdown-inline.plug.js` | the card view on the page |
| `Library/Atomdown/Inline.md` | `plugs/atomdown-inline/library/Atomdown Inline.md` | the header-bar button (`space-lua`) and the card CSS (`space-style`) |

`Library/Atomdown` is outside upstream's `Library/Std` tree on purpose. A
`git subtree pull` of SilverBullet rewrites `Library/Std`, so it can never
collide with this namespace, and the name says whose pages these are.

Plug discovery does not care about the directory. `client/space.ts` picks every
file in the space list whose name ends in `.plug.js`, so a plug under
`Library/Atomdown/Plugs` loads the same way one in `_plug` would.

**A copy in `_plug` is a SECOND plug, not an override.** SilverBullet's
`Space.listPlugs` returns every `*.plug.js` a space can see and loads all of
them, and it has no way to know two files are the same plug. So a hand-copied
bundle left over from the old install instructions runs alongside the compiled
one: two instances, each with its own memory, both answering every click and
both writing the same config key. Measured on an 11-group page, collapsing
every group and expanding every group left nine of them shut, and which nine
moved between runs. `iugum wiki` warns and names the files. Delete them.

To override a compiled asset deliberately, put your copy at the SAME path the
binary uses - `Library/Atomdown/Plugs/atomdown-inline.plug.js`, or
`Library/Atomdown/Inline.md` - so the space file shadows the underlay instead of
joining it.

The `*.plug.yaml` files are build inputs and `plugs/atomdown-e2e` is a test
suite, so neither one ships.

## The mechanism

SilverBullet has a read-only space underlay called `base_fs`.
`bin/silverbullet/src/embed.rs` compiles `client_bundle/base_fs` into the server
binary with `rust-embed`, and both boot modes layer it under the space folder
with `FallthroughSpacePrimitives` (`server-common/src/space/embed.rs`). Upstream
ships its own standard library that way.

Reads try the space folder first and fall through to the underlay. So:

- **A space cannot lose the assets.** They are not files in the space.
- **A space cannot delete them.** A path that exists only in the underlay
  refuses a write and a delete, and the HTTP layer answers `403`.
- **A space can override them.** Write `Library/Atomdown/Inline.md` in the space
  folder and that page wins. This is the escape hatch, and it is verified: the
  bundled CSS goes away and the space copy takes over within one sync cycle.

## Do you need `Space: Reindex`?

**No.** This was measured on a fresh space in a fresh browser.

`space-lua` and `space-style` load from the client-side index, not from the
file. The client keeps an index version in IndexedDB. A browser that has never
opened the space has no version, so `ensureFullIndex` runs a full reindex on its
own (`client/data/object_index.ts`), and that pass reads
`space.deduplicatedFileList()`, which includes the underlay. The bundled pages
are therefore indexed with no command.

Two details worth knowing:

- The very first paint can come before that first index finishes. The button and
  the CSS appear on the next load. Nothing is broken; the index is still
  building.
- Adding, changing or removing a space-level override is picked up by the normal
  sync path, again with no manual reindex. Bundled to override to bundled was
  tested as a round trip.

## How the assets get into the binary

`base_fs` is a build output, gitignored by the vendored tree. The assets have to
be staged between the two halves of the SilverBullet build:

```
npm run build                    writes client_bundle/{client,base_fs}
iugum stage-wiki-assets <dir>    adds client_bundle/base_fs/Library/Atomdown
cargo build --release            compiles base_fs into the binary
```

`make build-rs` runs the first and third steps together, so nothing calls it
here. One script drives the whole sequence and installs the result as the
`//go:embed` target:

```sh
scripts/build-wiki-blob.sh      # then: scripts/build.sh
```

The staging step is a command in this program (`iugum stage-wiki-assets`),
because this program holds the only copy of the assets. A shell script with its
own copy of the paths would drift from it.

The source-build path does the same thing on its own. `IUGUM_WIKI_SB_SRC=<dir>`
makes the adapter run `npm run build`, stage, then `cargo build --release`, in
place of the single `make build-rs`.

**No hunk lands in the vendored tree.** Staging only adds files to a build
output directory. The upstream surface stays at the one editor decoration seam
patch recorded in `silverbullet-decoration-seam.md`.

## When the binary is stale

The assets are in the SilverBullet binary, so a binary built before they existed
draws the cards with no CSS and shows no header button — the exact failure this
change exists to end. `iugum wiki` therefore checks the binary it is about to
run for the marker path `Library/Atomdown/Inline.md`, which `rust-embed` keeps
as a string literal, and prints what to do when it is absent:

```
wiki: this SilverBullet binary carries no Library/Atomdown assets, so the
      atomdown card view has no CSS and no header button.
wiki: rebuild the embedded binary with scripts/build-wiki-blob.sh, then rebuild
      iugum.
```

It is a warning, not an error. SilverBullet itself is fine, and a space that
does not use atomdown does not care.

## Why there is no runtime seeder

An earlier version of this work wrote the same files into each space folder on
startup, with a digest manifest to tell an old seed from a hand edit. That was
removed. Copies in spaces are the drift that caused the four breakages: a copy
goes stale, or somebody deletes half of it, and the space then disagrees with
the program. The underlay has no copy to go stale.

## What the bundle does not carry

Only files the atomdown views cannot work without. The bundle exists so the
feature can never look broken, not as a place to ship conveniences.

**No page that changes global editor behaviour.** A `base_fs` page can be
overridden only at its own exact path, so a page that also exists in a space
under a different name gives two live copies with no way to turn either off.

This was measured, not guessed. An editor-width page
(`Library/Styles/EditorWidth.md`, a four-step `--editor-width` cycle from
Steve's FFAI space) was bundled first and then removed. A space that already
had its own copy showed **two** width buttons in the top bar, and the two
`system:ready` listeners raced over one `html` attribute and one `clientStore`
key, so which width survived a reload depended on load order. The two spaces
that already own such a page are Steve's FFAI space and the front-end test
fixture. The page stays in the spaces that want it.

The atomdown pages carry no such risk, because nothing else defines them, and
a space that wants to change one writes the same path and wins.

`Library/Styles/Notion.md` is not bundled for a second reason as well: it
restyles every theme token with `!important` and embeds a company logo as a data
URI. That is one person's taste and one company's brand, not a product default.

**A leftover hand copy is a duplicate, not an override.** A plug bundle still
sitting in a space's `_plug/` does not shadow the compiled one — the loader has
no idea the two files are the same plug, so it runs both. `iugum wiki` warns
about that and names the files to delete.
