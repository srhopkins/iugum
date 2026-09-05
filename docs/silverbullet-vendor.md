# Vendored SilverBullet tree

`silverbullet/` is a **git subtree** of Steve's fork of SilverBullet.
The tree stays in upstream form except for one patch: the editor decoration
seam. That patch adds three files and edits two.
See [`silverbullet-decoration-seam.md`](silverbullet-decoration-seam.md) for the
config shape and for the two hunks to re-apply after a conflict.

| Item | Value |
|---|---|
| Prefix | `silverbullet` |
| Upstream | `https://github.com/silverbulletmd/silverbullet.git` (remote `sb-upstream`) |
| Fork | `https://github.com/srhopkins/silverbullet.git` (remote `sb-fork`) |
| Pinned ref | tag `2.10.0` |
| Pinned commit | `2b2a7c719bb3546df8c78ddeaf95256535ee2dd3` (2026-07-28) |

The same values are in `scripts/vendor/silverbullet.pin` in key=value form.
Update that file in the same commit as a subtree pull.

## History

The first drop of this path (commit `ba08233`) was a directory copy, not a subtree.
It had no upstream record. `version.ts` said `2.10.0`, but the content was upstream
commit `8d74993` from 2026-08-21, which is 24 commits after the 2.10.0 release.
A file-by-file compare showed the copy was an unmodified upstream tree, so nothing
was lost when the subtree replaced it.

The pin is the 2.10.0 release tag, not that edge commit. A tag is reproducible,
and a release gets upstream bug fixes.

## Remotes

```sh
git remote add sb-upstream https://github.com/silverbulletmd/silverbullet.git
git remote add sb-fork     https://github.com/srhopkins/silverbullet.git
git fetch sb-upstream --tags
git fetch sb-fork --tags
```

The fork is a plain GitHub fork. It holds no iugum commits.
Keep it as the pull source. The decoration seam patch lives in this repo, not in
the fork.

## Move the pin

```sh
git fetch sb-fork --tags
git subtree pull --prefix=silverbullet sb-fork <new-tag> --squash
```

Then edit `scripts/vendor/silverbullet.pin` and the table above, and rebuild.

Then re-apply the decoration seam edits if the pull dropped them
(`silverbullet-decoration-seam.md` holds the two hunks), and run
`cd silverbullet && npm test && npm run check`.

The working tree must be clean. `git subtree` refuses to run on modified files.
If another change is in progress, do the pull in a separate worktree
(`git worktree add`) and merge that branch back.

## Run the wiki from source

`iugum wiki` runs the **embedded release binary** by default. That path needs no
toolchain and does not read `silverbullet/`. Two environment variables switch it:

| Variable | Effect |
|---|---|
| `IUGUM_WIKI_SB_BIN=<path>` | run this SilverBullet binary |
| `IUGUM_WIKI_SB_SRC=<dir>` | build the source tree in `<dir>`, then run the result |
| `IUGUM_WIKI_SB_REBUILD=1` | build again even when a release artifact is present |

`IUGUM_WIKI_SB_BIN` wins over `IUGUM_WIKI_SB_SRC`.

```sh
IUGUM_WIKI_SB_SRC=./silverbullet ./iugum wiki --port 3010 ./wiki
```

The source build runs `npm install` when `node_modules` is absent, then
`npm run build`, then `iugum`'s own space-asset staging step, then
`cargo build --release -p silverbullet`, and runs
`silverbullet/target/release/silverbullet`. It needs `npm` and `cargo` on PATH.
Both build outputs (`target/`, `node_modules/`) are ignored by
`silverbullet/.gitignore`.

Those are the two halves of `make build-rs` with the staging step between them.
The atomdown plugs and their CSS go into `client_bundle/base_fs`, which
`npm run build` writes and `cargo build` compiles into the binary, so the
staging has to land in the middle. See
[`wiki-space-assets.md`](wiki-space-assets.md).

The build takes minutes. It is cached: a second run reuses the artifact unless
you set `IUGUM_WIKI_SB_REBUILD=1`.

## Why the adapter asks the binary about --single

SilverBullet 2.10.0 replaced the Go server with a Rust server (upstream PR #2010)
and added a `--single` flag for one-space mode. Version 2.9.0 and earlier reject
that flag with `unknown flag: --single`.

The embedded binary is still a 2.9.0 build (`silverbullet version` prints
`2.9.0-0-g72bba94`), so an adapter that always passed `--single` could not start
it. `adapter/wiki/sbadapt` now reads `--help` from whichever binary it is about
to run and adds `--single` only when that binary lists it. One adapter then
serves the 2.9.0 blob and a 2.10.0 source build.

Replace the embedded blob with a 2.10.0 build to remove the skew:

```sh
scripts/build-wiki-blob.sh     # builds the blob and installs it
scripts/build.sh --cgo         # rebuilds iugum around the new blob
```

`silverbullet/silverbullet` is the `//go:embed` target in `main.go` and is
ignored by git, so that copy changes the built program, not the repository.

Use `scripts/build-wiki-blob.sh` rather than a bare `make build-rs` plus a
`cp`. The script stages iugum's space assets into `client_bundle/base_fs`
between the npm and cargo halves of the build. A blob built without that step
serves the atomdown card view with no CSS and no header button, and
`iugum wiki` says so at startup.
