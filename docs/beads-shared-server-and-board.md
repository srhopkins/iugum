# Spec: shared Beads server and the all-projects board

Status: draft for Steve's review (2026-10-13). Nothing here is built yet.

## Goal

Every machine and every agent reads and writes the same Beads data live, with no push or pull. Each project stays isolated for the agents that work on it. Steve gets one board that shows every project at once, and a full page for each ticket.

## Terms

- **Dolt server**: one `dolt sql-server` process on the homelab tower. It speaks the MySQL protocol. One server holds many databases.
- **Project**: one Beads database on that server, for example `pr_reviews`, `resea`, `homelab`. Ticket IDs keep a per-project prefix.
- **Client**: anything that talks to the server: `bd` on the Mac or the agent box, and the iugum beads viewer.
- **Scoped agent**: an agent whose `bd` points at one project database, with a database user that can only reach that database.
- **All-projects board**: a Kanban view in the viewer that shows tickets from every project together.

## Requirements

### R1. One shared server

1. One Dolt server runs on the tower and listens on the home network only.
2. It holds one database per project. Upstream Beads shared server mode (`shared-server: true`, `BEADS_DOLT_SHARED_SERVER`) is the model.
3. `bd` on the Mac and on the agent box connects to it with `bd init --server --server-host --server-port --server-user`, or `bd dolt set host|port|user|database` on an existing project.
4. The server starts on boot and restarts on failure.
5. A nightly job pushes every database to its git remote (`bd dolt push` or `dolt push`) as a backup.
6. No password or key appears in a repo, a ticket, or a chat. Credentials come from files with mode 600.

### R2. Project isolation for agents

1. Each project database has its own database user with rights on that database only.
2. A scoped agent gets that user. It cannot read or write any other project.
3. A read-only scoped agent gets a user with SELECT only, or `bd --readonly`.
4. Steve's own client and the viewer use a user that can reach every project.
5. `bd ready` for a scoped agent lists only that project's ready work. Deferred tickets never appear (deferred is the backlog).

### R3. Memories: one librarian process, database first

1. Memories are shared. Any agent on any machine reads and writes the same store, live.
2. One **librarian** process runs on the tower. It owns a SQLite database with a hard schema: memory page, comment, log entry, tag, link, author, created and updated times, `verified_by`, `status`, `stale_after`, project. Only this process opens the database file.
3. Agents never touch the files. They call the librarian over HTTP or MCP: `query`, `add`, `update`, `comment`. Each call names the project. Scoped agents read `global` plus their own project and write only their own project.
4. The librarian serializes writes. Two agents cannot edit the same page at once. It searches by title and tags before `add` and refuses a near-duplicate.
5. Every write regenerates the Markdown for that page, its folder `index.md`, and `log.md`. The folder is output only. Layout: `global/` plus one folder per project. The front matter on each page is the schema columns printed at the top (OKF keys: `type`, `title`, `description`, `tags`, `generated`, `verified`, `status`, `stale_after`). OKF is the Open Knowledge Format, a Google Cloud Markdown-plus-front-matter spec, Apache-2.0. Using its key names costs nothing and keeps the pages readable by any OKF tool.
6. The generated folder is served by the iugum wiki and committed to git by the librarian after each write batch. Git is the audit trail.
7. A file watcher on the folder reports any write that did not come from the librarian. The folder is read-only to every other user.
8. Freshness does not depend on an agent remembering. Hooks on commit, PR merge, bead close, and session end call `add` with a one-line fact. A session-start hook injects a short digest plus the stale pages for the current repo. A scheduled job lists pages past `stale_after` and files one bead per page. Agents never set `verified_by`; only Steve does.
9. `bd remember` becomes a thin client of the librarian. One store, not two.
10. FFAI memory lives in its own project, apart from personal memory, because FFAI data is Confidential by policy.
11. Later, optional: a model step inside the librarian that decides where a fact belongs and what it should merge into. The schema and the write path do not change for that.

### R4. Viewer: all-projects board

1. A Kanban view with an "All projects" mode. Columns are Beads statuses: deferred (Backlog), open (Ready), in_progress, blocked, closed.
2. Each card shows its project tag, ID, title, priority, and assignee.
3. Optional swimlanes: one row per project.
4. Project chips filter the board to a subset of projects. The project switcher becomes a filter, not the only way in.
5. Dragging a card to another column updates that ticket in its own project database, through the same code path as `bd update -s`.
6. Changes made by any client (an agent on the box, `bd` on the Mac) appear on the board without a manual refresh, within 5 seconds.
7. The single-project board is the same view with one project selected.

### R5. Viewer: full-page ticket

1. Each ticket has its own page at `/bead/<project>/<id>`, linkable and bookmarkable.
2. It shows title, description, status, priority, type, assignee, labels, dates, notes, parent and children, dependencies (blocks and blocked by), and comments.
3. Fields can be edited in place. Edits go through the viewer API to `bd`.
4. The side drawer stays for quick looks and links to the full page.

### R6. Viewer: list view

1. A table across all projects or a chosen subset, with sort and filter on status, project, priority, labels, assignee.
2. Correct parent and child nesting, matching `bd show`.

### R7. API

1. The existing viewer API (`/api/beads`, `/api/bead/{id}`, writes) gains a project parameter and an all-projects list endpoint.
2. Every write goes through `bd`, so Beads rules (IDs, ready logic, dependency blocking) stay in one place.
3. The viewer runs read-only unless started with writes enabled. Writes need a token.

### R8. Components

1. Kanban: Kibo UI Kanban (MIT, dnd-kit), copied in with the shadcn CLI.
2. Table: TanStack Table.
3. Ticket page: shadcn layout parts.

## End-to-end test scenarios

Run each on a copy of the tracker, never on the live databases, until all pass.

### E1. Two machines, one server

1. Start the Dolt server on the tower with three project databases: `pr_reviews`, `resea`, `homelab`.
2. On the Mac, create a ticket in `resea`.
3. On the agent box, run `bd show` for that ID against the same server.
- Pass: the box sees the ticket at once. No push or pull ran.

### E2. All-projects board shows every project

1. Create two tickets in each of the three projects, in different statuses.
2. Open the viewer's All projects board.
- Pass: all six cards show, in the right columns, each with the right project tag. Swimlanes show three rows.

### E3. Update another project from the one view

1. On the All projects board, drag a `homelab` card from Ready to In progress.
2. Run `bd show <id>` on the Mac against `homelab`.
- Pass: status is in_progress. No other project changed.
3. On the box, run `bd update <id> -s closed` for a `resea` ticket.
- Pass: the board moves that card to Done within 5 seconds, with no manual refresh.

### E4. Scoped agent sees only its project

1. Start an agent on the box with the `resea` database user.
2. Have it run `bd ready`, `bd list`, and `bd show` on a `homelab` ID.
- Pass: it sees only `resea` tickets. The `homelab` lookup fails with a permission error.
3. Have it try `bd update` on a `homelab` ticket.
- Pass: refused. The `homelab` ticket is unchanged.

### E5. Read-only agent

1. Start an agent with the read-only `resea` user.
2. Have it run `bd ready` and then `bd update -s in_progress` on a ready ticket.
- Pass: the read works. The write is refused.

### E6. Backlog stays out of reach

1. Create a deferred ticket in `resea`.
2. Run `bd ready` as the scoped `resea` agent.
- Pass: the deferred ticket is not listed. It shows in the Backlog column on the board.
3. Steve drags it to Ready on the board.
- Pass: the next `bd ready` lists it.

### E7. Concurrent writers

1. Run four agents (two on the Mac, two on the box), each creating 25 tickets and 25 comments across two projects at full speed.
- Pass: all 200 writes land. No lock errors. Ticket IDs are unique. The board count matches `bd list` per project.

### E8. Full-page ticket

1. Open `/bead/resea/<id>` directly in a new tab.
- Pass: the page loads with all fields, dependencies, children and comments.
2. Edit the priority and add a comment on the page.
- Pass: `bd show` on the Mac shows both changes.

### E9. Server down

1. Stop the Dolt server.
2. Run `bd ready` on the Mac and open the board.
- Pass: both show a clear connection error. Nothing is written to a local fallback database by mistake.
3. Start the server again.
- Pass: both work again with no data loss.

### E10. Backup and restore

1. Run the nightly backup push.
2. Restore one project database from its remote into a scratch server.
- Pass: ticket count and the latest ticket match the live database.

## Out of scope for now

- Access from outside the home network (would need a VPN such as Tailscale).
- Migrating existing Beads data into project databases. Planned after E1 to E10 pass.
- Replacing the epic-based grouping already in use.

## Open questions for Steve

1. Swimlanes on by default, or a toggle?
2. Which projects to create first besides `pr_reviews`, `resea`, `homelab`?
3. Live updates: is polling every few seconds fine, or should the viewer push changes?
4. Does an agent-written memory count as trusted before you review it, or only after you set `verified_by`?
5. Atomdown block IDs in memory pages from day one, or only on PM docs?

## Proved on 2026-10-13 (throwaway Dolt 2.4.2 server on the tower)

- E1, E4, E5, E9 and `--readonly` pass with the iugum `bd` on the Mac and the agent box. Init needs `--external` as well as `--server`. The password comes only from `BEADS_DOLT_PASSWORD`.
- E4: a user without rights on a database gets "database not found", not a permission error. Fine.
- E5: a SELECT-only user fails at `bd` schema setup on any write. Reads work.
- E7 fails with 12 of 100 creates lost to `SQLITE_BUSY` from the iugum memory store when two `bd` calls run at once on one machine. Dolt had no errors. Bead `iugum-9eq`.
- The hint `Try: bd dolt start` starts a local empty server on the same port. Bead `iugum-b26`.
- Memories never reach the server. `bd remember` on each machine writes its own SQLite file. The other machine cannot see it. The Dolt database has no memories table.
- Upstream Beads keeps memories as `kv.memory.*` rows in its `config` table, with substring search in Go and a known sync-conflict problem.
- Dolt 2.4.2 has FULLTEXT (natural-language mode only, no Boolean or prefix search), VECTOR columns and indexes, JSON, triggers, procedures, CTEs. Writers do not wait for each other: the second committer fails with error 1213 and must retry. Database size grows fast and needs scheduled `dolt_gc`.
- Decision: memories stay in SQLite behind the librarian process on the tower (R3). Dolt holds tickets only.

## Backup by server-side replication (proved 2026-10-13, Dolt 2.4.2)

- The server pushes every write to a backup remote on its own. Clients never push; `dolt.auto-push` stays unset everywhere.
- Settings go in a file under `/etc/dolt/servercfg.d/` (it replaces the image default, so it must also set `listener: {host: 0.0.0.0, port: 3306}`): `dolt_transaction_commit: 1` and `dolt_replicate_to_remote: backup`. `SET @@PERSIST` for `dolt_transaction_commit` is ignored on restart. Replication changes need a server restart.
- Each project database needs a remote named `backup`, or set `dolt_replication_remote_url_template` so new databases replicate automatically.
- Folder remote and git remote both worked: three concurrent writers across two databases; the remote clone matched the server exactly (issues, comments, statuses, commit count). Async mode kept the remote within 2 seconds.
- A git remote must have an initial commit, or the server silently disables replication for that database (log only). Async mode also hides a dead remote. So run a periodic check that the backup's commit count matches the server.
- GitHub needs an ssh key inside the server container; not tested. Options: a deploy key for one backup repo, or push from a sidecar.
- History per create: one "Transaction commit"; updates and closes add a `bd:` commit; beads logs harmless "nothing to commit" warnings.
