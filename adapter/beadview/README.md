# beadview

A beads viewer: a React UI with a JSON API, plus the older server-rendered
ticket table and bd's own interactive dependency graph under `/legacy/`.
`iugum beadview` starts it.

```
iugum beadview [--port N] [--hostname ADDR] [--dir DIR] [--read-only]
```

- `--port` default `3849`.
- `--hostname` default `127.0.0.1` (like every other slot, binds local-only
  by default).
- `--dir` the beads repo to view (must contain `.beads/`). Default: the
  process's current directory.
- `--read-only` turns off every write endpoint. They answer `405` with
  `Allow: GET`, and `/api/config` reports `"read_only": true` so the UI can
  hide its write controls.

## Routes

| Path | What it serves |
|---|---|
| `/` and any other unknown path | The React UI (`index.html`), so client-side routes work |
| `/assets/*` | The UI's built files. A missing file here is a real `404`, not `index.html` |
| `/api/*` | The JSON API below. Also mounted at `/api/bd/*`, the ayo viewer's prefix. Unknown `/api` paths are a JSON `404` |
| `/legacy/` | The server-rendered ticket table (`html/template`) |
| `/legacy/tree` | Beads nested by parent (epic hierarchy), with search |
| `/legacy/bead/{id}` | One ticket, server-rendered |
| `/legacy/graph` | `bd graph --all --html`, served as-is |
| `/bead/{id}` | Redirects to `/?bead={id}` (the UI opens that bead) |
| `/graph`, `/tree` | Redirect to `/legacy/graph` and `/legacy/tree` |

## JSON API

One project, no auth, no API key. Paths match the ayo viewer's
`/api/bd/*` routes (`AvantOpsIO/ayo-agent` `src/app/api/bd/`), so the
ported React code runs unchanged. The UI's `?project=` query and `project`
body field are accepted and ignored.

Every call runs one `iugum beads -C <dir> ...` child process (see "Data
path" below). "Bead" in the table means this shape:

- The ayo fields: `id`, `title`, `description`, `status`, `priority` as the
  string `"p0"`..`"p4"`, `type` (bd's `issue_type`), `labels` (always a
  list), `assignee`, `created_at`, `updated_at`, and `depends_on` and
  `dependencies` (the same list of depended-on IDs, any edge type).
- Extra fields the ayo UI does not need: `issue_type`, `notes`, `owner`,
  `created_by`, `started_at`, `closed_at`, `close_reason`, `parent`,
  `blocked_by` (IDs this bead waits on through a `blocks` edge), `blocks`
  (IDs that wait on this bead), `dependency_edges`
  (`[{issue_id, depends_on_id, type}]`, the edges with their types), and the
  `dependency_count`, `dependent_count` and `comment_count` counters.

| Method | Path | bd subcommand | Response |
|---|---|---|---|
| GET | `/api/beads` | `list --json --all --limit 0` | `Bead[]` |
| GET | `/api/bead/{id}` | `show --json -- <id>` (plus `list` to fill `blocks`) | `Bead`; `404` if unknown |
| GET | `/api/bead/{id}/comments` | `comments --json -- <id>` | `[{id, issue_id, author, text, created_at}]` |
| GET | `/api/mermaid?root=<id>` | `dep tree --format=mermaid --direction=both -- <id>` | Mermaid text. With no `root`: empty. On error: a one-node flowchart, still `200` |
| GET | `/api/config` | none | `{ws_port: null, ws_path: null, has_websockets: false, read_only, project}` |
| GET | `/api/projects` | none | `{projects: [{slug, description, enabled, registered}]}`: the one `--dir`, `slug` is its base name |
| POST | `/api/bead` | `create --json --title= ...`, then `dep add --depends-on=<dep> -- <new>` per dependency | `201 {id, issue: Bead, dependency_errors?}` |
| PATCH | `/api/bead/{id}` | `update --json [--status= --priority= --description= --title= --type= --claim] -- <id>` | `Bead` |
| POST | `/api/bead/{id}/close` | `close --json [--reason=] -- <id>` | `Bead`; `409` if open blockers stop the close |
| POST | `/api/bead/{id}/reopen` | `reopen --json [--reason=] -- <id>` | `Bead` |
| POST | `/api/bead/{id}/comments` | `comment --json --stdin -- <id>` (text on stdin) | `201 Comment` |

Write request bodies:

- Create: `{title, description?, type?, priority?, dependencies?: [id]}`.
  `title` is required. `priority` is `"p0"`..`"p4"`, `"0"`..`"4"`, or a
  number.
- Update: any of `{status, priority, description, title, type, claim}`.
  Only the fields present change. `"description": ""` clears it. An empty
  body is a `400`.
- Close and reopen: optional `{reason}`. The body may be empty.
- Comment: `{text}`, required.

A dependency that fails to add does not undo the create. It is listed in
`dependency_errors` and in the log. This matches the ayo route.

Errors are `{"error": "..."}`: `400` for a bad body or ID, `404` when bd
reports the bead is not found, `502` for any other bd failure.

### Write safety

Every write goes through these checks, in this order:

1. `--read-only`: `405`.
2. Cross-site check: the request needs `Content-Type: application/json`
   (else `415`), and an `Origin` header, if present, must match the `Host`
   (else `403`). There is no auth, so this stops another web page open in
   the same browser from writing to a local bead database. A plain HTML form
   cannot send a JSON content type, and a script from another origin needs a
   CORS preflight that this server never answers.
3. Casbin: the action `beadview/write`. The default policy allows it. Add
   `p, *, beadview, write, deny` to lock writes by policy.

Then the handler checks the body. IDs must match
`^[A-Za-z0-9][A-Za-z0-9._-]*$`, so they can never be read as flags. Every
value goes to bd as `--flag=value`, and comment text goes on stdin, so a
value that starts with `-` is stored as written.

Each attempt is logged to stderr with the action, the bead ID and the
result. Refused writes are logged too. Description and comment text are not
logged; only their length is.

## The UI build

The React source and its build steps are in `web/beadview/`. See
`../../web/beadview/README.md`. `npm run build` writes
`web/beadview/dist/` (`index.html` plus `assets/`). The build is committed,
the same as `web/observe/dist`. `beadview_ui.go` in the repo root embeds it
with `//go:embed all:web/beadview/dist`, so rebuild iugum after a UI build.
The embed lives in package `main` because `web/beadview/` holds no Go code.

If a program calls `NewHandler` without a UI (`Options.UI` is nil), `/`
serves a short placeholder page that links to `/legacy/` and `/api/beads`.

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

Child processes run one at a time per server (a mutex in `execFetcher`).
Concurrent children can fail with "database is locked". Other iugum
processes on the same host can also hold iugum's own startup store for a
moment. That failure (`iugum: database is locked`) happens in `app.New`,
before any bd code runs, so `execFetcher` retries it up to three times.

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

Given that, beadview did not vendor or rebuild beads-dashboard at the time.
`/legacy/graph` still serves `bd graph --all --html` verbatim (see
`data.go` `FetchGraphHTML`), and the ticket table (`/legacy/`,
`/legacy/bead/<id>`) is original Go: `html/template` pages this package
owns outright.

Update (bead `iugum-puu`, "vendor the ayo React beads UI"): the UI at `/`
is now a React port of the ayo `/beads` viewer, in `web/beadview/`. Its
provenance and build are described in `../../web/beadview/README.md`. It
is built with Vite and committed as `dist/`, like `web/observe`.

## What this does not do

- **No epic/parent hierarchy in the legacy ticket table.** `/legacy/` is
  flat. `/legacy/tree` nests by `Bead.Parent`, and `/legacy/graph` shows
  parent-child edges because that comes straight from
  `bd graph --all --html`.
- **Writes are on by default.** The JSON API can create, update, close,
  reopen and comment. Use `--read-only` (or the Casbin deny rule above) to
  view a repository without any write path.
- **No push updates.** No websocket (`/api/config` reports `ws_port: null`).
  The UI polls `/api/beads`. The server does not cache: each call reads
  bd's own database.
- **One project.** `/api/projects` returns the one `--dir`.
- **No auth.** Same posture as `iugum wiki`/`iugum observe`: binds
  `127.0.0.1` by default, meant for one operator on one machine. Do not put
  `--hostname 0.0.0.0` behind anything without adding a real gate first.
