# beadview

A read-mostly HTML viewer for the beads work graph: a filterable ticket
table plus bd's own interactive dependency graph. `iugum beadview` starts it.

```
iugum beadview [--port N] [--hostname ADDR] [--dir DIR]
```

- `--port` default `3849`.
- `--hostname` default `127.0.0.1` (like every other slot, binds local-only
  by default).
- `--dir` the beads repo to view (must contain `.beads/`). Default: the
  process's current directory.

Open `http://127.0.0.1:3849/` for the ticket table, `/graph` for the
dependency graph, `/bead/<id>` for one ticket.

## Data path: subprocess self-exec, not a direct in-process call

The task this package was built for offered two ways to reach beads data
without shelling out to an external `bd`:

1. Call the vendored Beads Go packages directly.
2. Invoke the vendored cobra command in-process (`bdcmd.Execute()`, the same
   call `adapter/tracker/beadsadapt` makes for `iugum beads ...`) and capture
   stdout.

Neither is safe to use from inside a long-running HTTP server, so beadview
uses a third option in the spirit of (2): **it re-execs iugum's own binary**
(`os.Executable()`) as `<iugum> beads -C <dir> <args...>`, once per request,
and parses stdout. This still never touches an external `bd` -- the child
process is iugum itself, running the same in-process vendored Beads CLI that
`iugum beads` already uses. It is proven at `../../app/beadview_test.go` and
by hand: `env -i PATH=/usr/bin:/bin ./iugum beadview --dir <repo>` serves
real data with no `bd` reachable anywhere on PATH.

Why not option 1: `beads/internal/storage` (and everything else beads uses
to answer a structured query) lives under `internal/`. Go's compiler enforces
import visibility on the import path text, not on physical repo layout, so a
package outside `github.com/steveyegge/beads/...` cannot import it even
though the tree is vendored locally. Reaching it would mean either widening
`beads/cmd/bd`'s edit boundary past "export Execute" (see `AGENTS.md`
Boundaries) or moving internal beads code out of `internal/`, which
`docs/beads-vendor.md`'s re-vendor procedure does not do and should not
start doing for one UI.

Why not a direct in-process call to `bdcmd.Execute()`: it is built to run
once per OS process and exit with it, not to be re-entered. Two concrete
reasons, both in `beads/cmd/bd/main.go`:

- `Execute()` calls `os.Exit()` on several error paths (see its body). In a
  CLI process that just ends the program, which is fine. In an HTTP server
  it would kill every open connection and the server itself the first time a
  request hit an ordinary error -- an unknown bead ID, a bad flag, anything.
- The command tree carries package-level mutable state (`store`, `changeDir`,
  `rootCtx`/`rootCancel`, several `atomic.Bool`s) that `PersistentPreRun`/
  `PersistentPostRun` set up and tear down assuming exactly one command runs
  per process. Calling `Execute()` twice in the same process without that
  guarantee is unsupported and untested upstream.

A subprocess sidesteps both: a bad command exits that one child with a
non-zero status, which becomes a normal Go `error` the handler renders as a
502, and every request gets a fresh process with fresh global state --
exactly what the CLI itself gets. The cost is a fork+exec per request, which
is a non-issue for a single-operator local viewer.

## Why `bd graph --all --html` and not a rebuilt beads-dashboard frontend

The original plan for this package was to reuse
`github.com/AvantOpsIO/beads-dashboard` (a fork of the open-source
`ntellis/beads-dashboard`) as the frontend, with only this Go backend being
new. That plan was dropped after two findings during a review pass:

1. The fork's prebuilt bundle
   (`src/beads_dashboard/static/assets/index-DYX39sTL.js`) has a **live
   Clerk publishable key baked in at build time**
   (`pk_live_...` decoding to `clerk.avantops.dev`). Embedding it as-is would
   point any iugum deployment's sign-in at Steve's personal AvantOps Clerk
   tenant. Fixing this needs a real rebuild (Node/npm, Clerk code stripped
   out) every time the vendored copy changes, not a one-time edit.
2. **Neither the fork nor upstream ships a LICENSE file, copyright line, or
   license text.** `pyproject.toml` claims `license = {text = "MIT"}` and the
   README says MIT, but that is a claim, not a grant. Vendoring that code
   into iugum, even for local use, would carry an unresolved licensing
   question this package should not need to answer.

Separately, the vendored Beads CLI already ships its own dependency
visualization: `bd graph --all --html` (`beads/cmd/bd/graph.go`,
`graph_visual.go`) renders a **self-contained interactive D3.js page** --
zoom, pan, drag, click-for-details, a status-color legend, and (unlike the
beads-dashboard frontend) real epic/parent-child edges alongside `blocks`
edges. It is upstream Beads code under `beads/LICENSE` (MIT, an actual file
this time), already vetted by `docs/beads-vendor.md`, and needs no frontend
build step at all.

Given that, beadview does not vendor or rebuild beads-dashboard. `/graph`
serves `bd graph --all --html` verbatim (see `data.go` `FetchGraphHTML`).
The ticket table (`/`, `/bead/<id>`) is original Go: `html/template` pages
this package owns outright, so there is no license question and no Node/npm
build dependency inside iugum (`NORTHSTARS.md` star 1: one Go program, no
sidecar stack).

## What this does not do

- **No epic/parent hierarchy in the ticket table.** The table view is flat;
  `Bead.Parent` links to the parent ticket but the table does not nest
  children under it. `/graph` does show parent-child edges (dashed, per its
  legend) because that comes straight from `bd graph --all --html`.
- **Read-only.** No create/comment/close/reopen/edit from the browser. bd's
  own CLI (`iugum beads ...`) is still the way to write. Verification against
  a real repo (`/Users/steve/projects/github/FutureFit-ai`) never wrote to
  that repo's beads database for this reason as much as courtesy: there is no
  code path in this package that could.
- **No live updates.** No websocket, no polling. Reload the page to see new
  data (`bd`'s own DB is the source of truth, iugum does not cache it).
- **No auth.** Same posture as `iugum wiki`/`iugum observe`: binds
  `127.0.0.1` by default, meant for one operator on one machine. Do not put
  `--hostname 0.0.0.0` behind anything without adding a real gate first.
