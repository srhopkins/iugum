# Live worker activity in Beads

Assignment and ticket status describe ownership and workflow. They do not prove that a worker is running.

Iugum stores explicit activity in the canonical Beads issue metadata. The viewer reads it through a project-scoped, read-only endpoint. No side database or transcript file server exists.

## Worker use

The actual worker claims the issue with its stable `BEADS_ACTOR`. The worker then records its actual execution identity:

```sh
bd --actor my-worker update ISSUE --claim
iugum bead-activity --actor my-worker --run ACTUAL_RUN_ID \
  --platform codex --session ACTUAL_THREAD_ID --label 'Backend worker' \
  ISSUE working
```

For Codex, copy a verified `CODEX_THREAD_ID` into `--run` and `--session`. If present, `CODEX_SESSION_ID` can be recorded separately with `--context-session`; do not infer that it is a parent. Record an actual subagent identifier with `--agent-id` only when the tool exposes it. A task name or friendly assignee is not a session identifier.

For Claude, use the actual session UUID reported by the current Claude session/transcript metadata. `--run` identifies this execution. A resumed session can start a new run with a new explicit execution identifier while retaining the actual session UUID. Record `--parent-session`, `--parent-agent`, `--container`, `--transcript`, and `--thread-url` only when verified. A transcript locator is displayed as text; the browser never reads that path.

When the platform does not expose session or agent identity, give an explicit run identifier and explain the limitation with `--identity-unavailable`. Do not copy an assignee into a session field.

Repeat Working before the expiry while the worker is actively handling the task. The default expiry is two minutes; `--ttl` allows one second through ten minutes. Do not run an unattended renewal loop: it would make a paused agent appear live. Working is a bounded self-report, not a process monitor.

Use the same identity flags for subsequent reports:

```sh
iugum bead-activity [identity flags] --reason 'Waiting for independent review' ISSUE paused
iugum bead-activity [identity flags] --reason 'Review passed; handing off' ISSUE completed
```

Flags precede the two positional arguments. `--dir` selects the canonical tracker directory, including inside a container configured for the shared Dolt server. The helper never starts a database. It checks the current assignee before writing and changes only activity metadata. A racing reassignment invalidates the old actor's signal in the viewer.

Paused and Completed stop motion immediately after refresh. Missing, invalid or expired Working becomes a static unknown/stale state. The browser also stops stale motion if polling fails. Reduced-motion users see the same Working label with a static indicator. Activity does not move a ticket between lanes or change ownership.

Each run has its own versioned metadata record. The helper retains previous run records, preserves the first start time, and rejects reuse of one run identifier for a different session/agent. A completed execution needs a new run identifier before work resumes. Work history remains in ticket comments; heartbeat reports do not add comments.

## Viewer build and checks

The React viewer is supplied by the pinned private Beads fork. `web/beadview/prepare.mjs` copies its sources into a disposable directory, adds Iugum's component and patches three explicit mount points. It fails if those source anchors change. It never edits the module cache or upstream checkout.

Run `scripts/build-beadview.sh` after changing the component. It uses the fork's pinned dependencies and creates the embedded assets. Then build Iugum normally. Go builds use these assets; a missing generated build falls back to the existing viewer.

Run `node web/beadview/activity/browser.e2e.mjs` for the isolated fixture checks. The fixture calls no models and reads no personal tracker or transcript. Screenshots go to `/private/tmp/iugum-activity-*.png`.

The existing viewer supports Light, Dark and System. The activity check verifies narrow/wide layouts, both modes, reduced motion, and actual theme-variable overrides. This feature does not add the wiki's separate warm-neutral theme selector to beadview.

## Board display preferences

The board defaults to closed tickets from the last 14 days, using the actual `closed_at` timestamp and including the exact cutoff. Missing, invalid, or future closure dates stay visible with an uncertain-date count. **All closed** restores every matching closed ticket and persists in this browser. Counts distinguish displayed cards from the full matching total. List, search, ticket details, and history retain all dates.

In project swimlanes, each project header shows its visible card count and a disclosure button. Click, Enter, or Space hides or restores that project's entire row. Each stable project slug has its own browser preference, which survives reload and project deselection. New projects start expanded. These controls do not write to tickets.

Run `node web/beadview/board/browser.e2e.mjs` after building the viewer for disposable two-project checks of retention, counts, keyboard focus, persistence, complete List/details, and narrow/wide Light/Dark layouts. Screenshots go to `/private/tmp/iugum-board-*.png`. Beadview uses its existing theme variables; the wiki-only warm-neutral selector does not apply.

## Existing lifecycle hooks

The pinned Beads fork supports `.beads/hooks/on_create`, `on_update`, and `on_close` through its hook runner and storage wrapper. These run asynchronously after mutations, are best effort, and can be disabled. The wrapper does not currently override `ClaimIssue`; a direct claim does not establish a covered hook event. The local repository currently has Git hook scripts, not these lifecycle scripts.

Hooks can remind a worker or mirror a completed event. They cannot prove a current agent is executing, validate an identity before a write, or reliably start a heartbeat on every claim. This activity feature therefore uses explicit self-reports. No lifecycle hook or Beads fork source was changed.
