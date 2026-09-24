# web/beadview

The React UI for `iugum beadview`. It is a Vite + React app, built the same
way as `web/observe`. The Go binary embeds the built `dist/` folder
(`beadview_ui.go` at the repo root), so `iugum beadview` serves this page at
`/` and its JSON API at `/api`.

## Build

```
cd web/beadview
npm install
npm run build      # writes dist/index.html and dist/assets/
npm test           # vitest, jsdom
```

Then rebuild iugum (`go build .`) so the new `dist/` is embedded. Commit
`dist/` with the source change. iugum ships as one binary, and
`web/observe/dist` is committed the same way.

Asset URLs in `dist/index.html` are rooted at `/` (`/assets/...`), so the
page must be served from the root of the beadview server.

## Develop

1. Start a beadview server: `iugum beadview --dir <beads-repo>` (default port
   3849).
2. In another shell: `cd web/beadview && npm run dev`.
3. Open the URL Vite prints. Vite forwards every `/api` request to the
   beadview server.

To use a different server, set `BEADVIEW_PORT=4000` (local port) or
`BEADVIEW_URL=http://host:port` (full origin) before `npm run dev`.

## What the UI calls

All paths are relative to the page origin. Every call also sends
`?project=<slug>`, and every write body also carries `"project"`. The server
ignores both, because it serves one project.

| Call | Used for |
|---|---|
| `GET /api/beads` | The bead list. Polled every 10 s. |
| `GET /api/mermaid` | Graph fallback text, used only when the client-built graph has no edges. |
| `GET /api/config` | `read_only` hides every write control. `ws_port` stays null, so the header shows "Polling". |
| `GET /api/projects` | Fills the project selector (`{projects:[{slug,description}]}`). |
| `GET /api/bead/{id}/comments` | Comments in the detail panel. |
| `POST /api/bead/{id}/comments` | `{text}` |
| `POST /api/bead` | New Bead: `{title, priority, type, description, dependencies[]}` |
| `PATCH /api/bead/{id}` | Edit, Claim and Pipeline drag: `{status?, priority?, description?, claim?}` |
| `POST /api/bead/{id}/close`, `POST /api/bead/{id}/reopen` | `{}` |

Bead fields the UI reads: `id`, `title`, `status`, `priority` (`"p0"`..`"p4"`),
`type`, `description`, `depends_on` (ID strings), `parent`, `labels`,
`assignee`, `created_at`, `updated_at`. Comment fields: `author`, `text`,
`created_at`.

## Changes from the source viewer

The code started as the beads viewer in the Ayo app. Removed:

- Sign-in (Ayo used Google Workspace SAML sessions; the upstream fork used
  Clerk). beadview has no auth. It binds to `127.0.0.1` by default, and the
  Go side checks writes for same origin and against policy.
- The Ayo page frame and branding. The page title is "iugum beads".
- Next.js: the `next/dynamic` loader and `'use client'` markers. `src/main.jsx`
  is a plain entry.
- The `/api/bd` prefix rewrite. The base URL is the relative `/api`.
- Multi-project sidecar support: the hosted API URL and key settings, project
  aliases, hardcoded fallback projects, `external:` cross-project links and
  the "viewing another project" banner. The project selector stays, filled
  from `GET /api/projects`.

Changed:

- Priority filter, sort, badges, edit and New Bead cover P0 to P4. The
  source viewer offered only P1 to P3 and sorted P0 last.
- The tree nests a bead under its `parent` when it has one. A bead with no
  parent nests under the beads it depends on, as before. A dependency cycle
  no longer recurses forever when you press Expand All.
- The Graph tab loads Mermaid on first use, so the first page load is about
  340 kB of JS instead of about 1 MB.
- When `/api/config` reports `read_only`, the UI hides New Bead, Edit, Claim,
  Close, Reopen, the comment box and Pipeline drag.
