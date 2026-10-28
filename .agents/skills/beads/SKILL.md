---
name: beads
description: Use when working in a repository that uses bd or Beads for durable project task tracking, issue dependencies, blocker management, multi-session handoff, or shared work memory. Trigger when the user asks to find ready work, claim or close tasks, create follow-up work, inspect blockers, recover project context, or choose between local planning and persistent project tracking.
---

# Beads

Use Beads as the shared project task system. Local plans, scratch files, and personal memories are useful, but they are not the durable source of truth for project work.

## First Step

Run:

```bash
bd prime
```

If that prints nothing, check whether the repository has an active Beads workspace:

```bash
bd where
```

## Preferred Route

Use the `bd` CLI when shell access is available. It is the most compact and direct Beads interface.

## Core CLI Workflow

1. Find work:

```bash
bd ready --exclude-type epic
bd list --status=open
bd list --status=in_progress
```

2. Inspect before editing:

```bash
bd show <id>
```

3. Claim work atomically:

```bash
bd update <id> --claim
```

4. Create durable follow-up work when implementation reveals new tasks:

```bash
bd create "Short title" --description="Why this exists and what needs to be done" --type=task --priority=2
```

5. Close completed work:

```bash
bd close <id> --reason="Completed"
```

## Ready and blocking graph

- A standalone deliverable is one task. Use an epic and `--parent` only when distinct child deliverables need a shared rollup; agents claim ready children, not the epic.
- `--parent` groups work but does not sequence it. Parallel children need no dependency edge.
- For a real prerequisite, run `bd dep <blocker-id> --blocks <waiting-id>`. The waiting task becomes ready when the blocker closes; claiming the blocker is not enough. Soft links, including `waits-for` added with `bd dep add -t`, do not block ready work.
- Before claiming, check `bd ready --exclude-type epic`, then `bd show <id>` for the chosen task. After wiring work, check `bd children <parent-id>`, `bd dep list <id>`, and `bd ready --explain` when readiness looks wrong.
- Keep human decisions out of agent work selection: label them `human` and use `bd human list`. Do not add `blocks` edges merely because tasks share a topic.

## Goals and incoming requests

- Use the `goal` label for a durable desired outcome on Steve's punch list. Use `epic` only when the outcome needs multiple distinct deliverables; otherwise use one task. Children inherit the goal label and represent the work that can be claimed.
- Use the existing `side-request` label for an ad-hoc or drive-by ask with one concrete deliverable. Keep it as a standalone task. If it grows into a durable outcome with multiple deliverables, create or promote a `goal` epic and move the work under it.
- These labels are the visual and filterable distinction in beadview's issue list. Keep this taxonomy to `goal` and `side-request`; use existing issue types for shape and `human` for a decision Steve must make.
- Parent-child edges group deliverables under an outcome; they do not sequence work. Add a `blocks` dependency only for a true prerequisite. Do not attach a drive-by request to a goal just because the topics are related.

## Claiming and ownership

- The person or agent doing the work owns its claim and close. Set `BEADS_ACTOR` to your own stable identity, then run `bd update <id> --claim` before starting. Claiming is idempotent for the same actor; do not claim as Steve or another worker.
- Managers and request intake leave work open and unclaimed for the actual worker. Do not pre-assign or mark work `in_progress` on a worker's behalf.
- The worker who claimed the issue closes it with `bd close <id> --reason="..."`. Beads rejects a close from an actor that does not match the assignee; hand off by having the current owner release the claim before another worker claims it.

## What Belongs In Beads

Use Beads for:

- shared project tasks
- blockers and dependencies
- discovered follow-up work
- work that must survive thread reset, compaction, or handoff
- status that another person or agent should be able to resume

Use agent-local planning tools only for the current turn's execution checklist. Do not treat them as shared project state.

## Rules

- Do not create markdown TODO files as the source of truth when Beads is available.
- Do not use `bd edit`; it opens an interactive editor. Use `bd update` flags instead.
- Prefer `--json` when parsing `bd` output programmatically.
- If hooks are installed, `bd prime` may already be injected. Run it manually when context is missing.
- Do not auto-close or mutate tasks unless the work is actually complete.
