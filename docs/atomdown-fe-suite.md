# The Atomdown front-end suite and its pre-push gate

One command:

```sh
scripts/atomdown-fe-check.sh
```

The suite, the six rules, the component area, the matrix split, the fixture and
the escape hatch are all documented in
[`plugs/atomdown-e2e/README.md`](../plugs/atomdown-e2e/README.md). This file
covers the parts that live outside that directory: the hook wiring and the
build cache.

## Why a front-end suite at all

The two Atomdown views have 355 unit tests between them over pure functions.
Those tests passed through an evening in which every visual defect was found by
a person looking at a screenshot after an agent had reported the work done. The
defects were geometry and visibility - a list marker outside a border, a table
across two borders, a directive comment appearing on hover, a group that would
not expand. None of them is a wrong return value, so none of them is reachable
from a unit test.

The suite measures the rendered document in a real browser instead.

## The hook chain

`core.hooksPath` is `.githooks`. The `pre-push` hook now runs three things, in
this order:

1. `scripts/public-audit.sh --all --no-agent` - the secret gate. Instant, and
   first, because there is no point spending a minute in a browser only to
   refuse the push over a leaked key.
2. `.githooks/atomdown-fe-gate.sh` - the conditional front-end gate.
3. `.beads/hooks/pre-push` - the beads/factory hook, `exec`ed exactly as
   before, with the same arguments and the same exit code.

**Nothing was removed.** The audit and the factory hook behave as they did.

### stdin is shared

git feeds a pre-push hook its ref list on stdin, and there are now two
consumers: the gate's path check, and the factory hook. A stream can only be
read once, so `pre-push` captures stdin to a temp file and replays it to both.
Without that the factory hook would receive an empty ref list and silently stop
checking anything.

### How the gate decides

`atomdown-fe-gate.sh` diffs each pushed ref and runs the suite only when the
changed files touch:

| Path | Why |
|---|---|
| `plugs/atomdown-board/**` | the board panel |
| `plugs/atomdown-inline/**` | the inline view |
| `silverbullet/client/**` | the editor decoration seam the inline view needs |
| `plugs/atomdown-e2e/**` | the suite itself |
| `scripts/atomdown-fe-check.sh` | the runner |

A push of docs or unrelated Go code pays nothing. A branch that does not exist
on the remote yet has no range to diff, so the gate falls back to the merge
base with `origin/main`, and then to the single commit. A branch deletion is
skipped.

**Pre-push, not pre-commit.** Commits are recovery checkpoints and must stay
instant; push is the review gate. Same division the public audit already uses.

## The escape hatch

```sh
ATOMDOWN_FE_SKIP=1 git push
```

It prints that it skipped and names itself, so a bypassed push is visible in a
terminal scrollback rather than silent. For a genuine emergency: a hotfix at
2am, or a machine with no browser.

## The build cache

The suite drives `silverbullet/target/release/silverbullet`. That binary
carries iugum's editor decoration seam - the one patch to the vendored client,
which the inline view cannot work without - and embeds the client bundle
through rust-embed, so the suite needs no separate `npm run build` at run time.

Building it takes about 90 seconds, so the runner treats it as a cache and
rebuilds only when it is missing or older than `silverbullet/client/`. It
prints which of the two happened, and it prints total elapsed time on both
success and failure, so the cost of the gate is always visible.

## Failure output

Every rule fails through one function, so a failure always names:

- the rule number and what it was checking,
- the matrix cell (width, theme, density),
- the measured numbers - how many pixels, which child, which side, which box,
- a screenshot path and a JSON path under `scratchpad/atomdown-fe-out/`.

The runner then lists the newest artifacts in that directory, so the failing
run's files are the first thing on screen.

## Proving the rules can fail

```sh
scripts/atomdown-fe-check.sh --defects
```

`plugs/atomdown-e2e/defects.test.ts` reintroduces real defects as injected
`space-style` CSS in the test's own temporary space, and asserts the rules
report them. It is not part of the gate: those tests are supposed to see
violations.
