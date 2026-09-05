# atomdown-e2e

The front-end suite for the two Atomdown views: the board panel
(`plugs/atomdown-board`) and the inline view (`plugs/atomdown-inline`).

**A board or inline change is not done until this passes.**

```sh
scripts/atomdown-fe-check.sh            # fast matrix - what the pre-push hook runs
scripts/atomdown-fe-check.sh --full     # all 16 matrix cells
scripts/atomdown-fe-check.sh --rule 1   # one rule
scripts/atomdown-fe-check.sh --defects  # the negative control
scripts/atomdown-fe-check.sh --probe    # print what the views render
```

## Why it exists

Both plugs already have unit tests over pure functions: 285 for the board, 70
for the inline view. Those tests passed through a whole evening of visual
defects, every one of which was found by a person looking at a screenshot after
an agent had reported the work done. The defects were geometry and visibility:
list markers left of a card's border, a table across two borders, a directive
comment appearing on hover, a group that would not expand. No test over a pure
function can see any of them, because none of them is a wrong return value.

So this suite measures the rendered document in a real browser.

## The six rules, and the defect each one reproduces

| # | Rule | The defect it exists for |
|---|------|--------------------------|
| 1 | **Containment.** Every child rect inside its card box and its group box, allowing for border width. | Ordered-list markers `1.`-`6.` left of the card's left border. A wide table across the card border AND the group border. Group header controls clipped at the content column edge. |
| 2 | **Directive invisibility, one legitimate reveal.** At rest every directive contributes at most 4px and shows no text; nothing appears on any hover, any card top border, or any collapse click; the text cursor with the editor focused DOES reveal one, inside its card's borders. | 82 `sha256:` digests back on the page. A reveal that leaked on hover and passed every at-rest check. |
| 3 | **Layout stability.** A reference card's y never moves. Edit mode is the one exception and may grow its own card DOWNWARD only, with content below moving by exactly the height delta. | Hover states that resized a box instead of recolouring it, so reading the page moved it. |
| 4 | **State machine round trips.** Collapse, view on/off, raw/rendered, density and the four editor widths each return to an identical DOM signature; reload persistence keeps on ON and off OFF, scoped per page. | A group that would not expand after collapse. The header toggle doing nothing on first press while the command worked. Close-then-reload reopening the board. |
| 5 | **Rendering fidelity.** No `<!-- <atom`, no `sha256:`, no `](http`, no bare `##` or `**` outside code. Positively: one `<ol>` with six `<li>`, one `<table>` with 10 rows, an `<a href>` in every ticket cell. | Raw markdown reaching the reader. An ordered list rendering as a run-on paragraph. |
| 6 | **Document immutability.** After every interaction the page's bytes are unchanged and `atomdown lint` and `atomdown verify` both pass. An edit then one undo returns the same bytes. | A silent id, slug or digest rewrite: the file still lints, still renders, and the diff is churn nobody can evaluate. |

## Area 7: the components

Rules 1-6 are cross-cutting invariants. Area 7 (`7-components.test.ts`) is the
other axis: each primary component in turn, asserted to EXIST, sit WHERE IT
BELONGS, and BEHAVE, in both views - because the two drifting apart is the
problem the whole suite exists for.

Card, group, card menu (three-dot), drag handle (grip), group header controls,
card editor, stale-digest indicator.

**Position is measured, never inferred from a class.** Every placement
assertion compares `getBoundingClientRect()` values. The grip regressed to the
wrong side of the card in the inline view with its class entirely correct, so
`.atomdown-grip` being present proves nothing about where the grip is.

**Hidden-until-hover is asserted in both states.** Absent or transparent at
rest, and visible after a real pointer hover. One half alone passes on a
control that is always visible and on one that is never visible, and both are
bugs.

## The matrix, and the split

Three axes: two densities (board only - the inline view has no density knob),
four editor widths from `Library/Styles/EditorWidth.md`, light and dark theme.

- **FAST** (default, what the hook runs): four cells, one per axis value. Every
  width appears once, both themes appear, both densities appear.
- **FULL** (`--full`): the cross product, 16 cells.

A defect that needs a *pair* of specific axis values escapes the fast subset.
That is the trade, stated plainly: the fast subset is a gate people keep, and
`--full` is what you run before a release or when chasing something that only
shows up in one combination.

Area 7 is in the fast subset on purpose - those are the assertions most likely
to catch a regression on an ordinary change.

## The negative control

`defects.test.ts` reintroduces real defects and asserts the rules REPORT them.
A test that has never failed is not a test.

Each defect is a stylesheet injected into the real page once the view is open,
so the broken rule is genuinely in the document and never touches the plugs.
It has one honest limit: CSS reproduces every geometry and visibility defect and cannot reproduce
a state-machine defect. So rules 1, 2, 3 and 5 and the grip's side are proven
against injected CSS, and the state-machine halves are covered by the "the
toggle did nothing" guard inside rule 4's own round-trip helper.

## The fixture

`fixture/running.md` is generated by `fixture/make-fixture.mjs`. It reproduces
the SHAPE of Steve's real page - 82 atoms, 11 named groups, a 10-row table,
ordered lists, nested lists, a blockquote, fenced code, inline code, long links
- and none of its content, because this repo has a public-repo gate and the
real page carries client ticket titles and colleague names.

Ids and digests come from the real `atomdown materialize --digest`, so rule 6
runs `atomdown lint` and `atomdown verify` against a document neither tool can
be fooled by.

Two things in it look wrong and are not:

- **84 cards, 82 atoms.** `atomdown materialize` leaves a fenced code block's
  opening line outside the atom it creates, so each fence is an uncovered
  block that both views draw as one extra card marked implicit.
- **One row is raw.** The `FFAI-62019` row's link label contains unescaped
  square brackets, which close the label early, so it is not a link. Plain
  SilverBullet renders it raw too. Rule 5 asserts it STAYS raw, so a change
  that starts rendering it - by repairing the markdown behind the reader's
  back - fails.

To run the suite against the real page locally:

```sh
ATOMDOWN_FE_PAGE=/path/to/running.md scripts/atomdown-fe-check.sh
```

Nothing in the suite writes to that path, and rule 6 proves it.

## How the gate decides to run

`.githooks/pre-push` calls `.githooks/atomdown-fe-gate.sh`, which reads git's
ref list and runs the suite only when the push touches:

- `plugs/atomdown-board/**`
- `plugs/atomdown-inline/**`
- `silverbullet/client/**` (the editor decoration seam the inline view needs)
- `plugs/atomdown-e2e/**` or the check script itself

A push of docs or unrelated Go code pays nothing. **Pre-push, not pre-commit**
- commits stay instant.

**Escape hatch**, for a genuine emergency:

```sh
ATOMDOWN_FE_SKIP=1 git push
```

It prints that it skipped, so a bypass is visible in the terminal rather than
silent.

## What is not deterministic

Stated rather than hidden.

- **The grip drag** (`7-components.test.ts`). A synthetic pointer drag does not
  always satisfy the gesture handler, so that test SKIPS with a reason when the
  drag produces no change rather than failing. Its assertions about the result
  - that a reorder moves lines and changes no id, slug or digest, and that one
  undo reverts it - are real whenever the drag lands.
- **The browser.** The suite prefers Playwright's pinned Chromium and falls
  back to the installed Google Chrome when that download is missing. Chrome
  auto-updates, so pixel measurements are marginally less stable on the
  fallback. `ATOMDOWN_FE_CHANNEL=chromium` forces the pinned build; the runner
  prints which one it used.
- **Save timing.** Rules 6 and 7 wait for a fixed period after an edit before
  reading the file, because SilverBullet's autosave has no completion event a
  test can await from outside. The waits are generous; a slow machine could
  still read early, which would show up as an unexpected byte comparison
  rather than a wrong pass.
- **`atomdown lint` and `atomdown verify`** need the `atomdown` binary. Rule 6
  looks for it on `$ATOMDOWN_BIN`, `~/go/bin`, `/usr/local/bin`,
  `/opt/homebrew/bin`, then the sibling checkout, and SKIPS those two checks
  with a loud reason if none is found. The byte comparisons still run. A gate
  that fails because a sibling repo is missing gets switched off.

## Status

**Green on the fast matrix, both views: every rule and every component test.**
No `test.fixme` is left in the suite. One test still skips with a reason rather
than failing — the grip drag, when a synthetic pointer drag produces no change;
see "what is not deterministic" above.

The six area 7 tests that were pending on the first run are all in. What each
one needed:

| Pending test | What it was |
|---|---|
| The card's closed box | The probe read the header widget and its siblings and never its CHILDREN, and the inline card's TOP edge is drawn on `.atomdown-card-head` inside the widget. It reported `top: 0` on a card whose top border is plainly visible. |
| The card menu | It measured `.board-card-menu`, the positioning box that deliberately stays laid out so nothing reflows when the button appears. The control is `.board-menu-btn` inside it. And `:focus-visible` alone left a focused button invisible, because it does not match a focus set by script or by a click — that one was the panel's, and the panel now uses `:focus` too. |
| The grip's resting state | The duplicate plug. Nothing else. |
| The group header controls | Three things: `.board-slug-input` matches three hidden forms, so the rename input had to be scoped to the group's own; at compact density Rename and Ungroup fold into the menu and the direct buttons are `display: none`, so the test has to check VISIBLE rather than present; and ungroup takes a loose group's marker line WITH its blank line, which is deliberate and unit-tested, so the comparison normalises blank-line runs. |
| The card editor | The duplicate plug. |
| The stale-digest indicator | It needs a page with no drift and it creates drift, and the editor test above had already edited the shared one. It boots its own space now, as the editor and the group-controls tests do. |

**The duplicate plug** was the largest single cause, and it was the suite's own
doing. Both plugs are compiled into the server binary's read-only underlay, and
`Space.listPlugs` returns every `*.plug.js` a space can see, so the copy the
harness wrote into `_plug/` did not override the compiled one — it added a
SECOND instance, with its own memory, answering every click and writing the same
config key. The harness seeds no plug copy now, and `scripts/atomdown-fe-check.sh`
stages this tree's plugs into the binary before it runs. `iugum wiki` warns when
a space still holds a hand-copied bundle.

**Two known-good fixes** from the same triage, worth knowing before changing the
suite: `cardTop` has to scroll a virtualised card into view before it can
measure it, and `.board-card-body` matches THREE elements per card (rendered,
raw, editor) so every locator has to name `[data-card-rendered]`.

## What a virtualised editor does to a measurement

Four failures in this suite were the measurement rather than the view, and all
four have the same root: CodeMirror renders a window of the document and
estimates the rest.

- **A rect in a whole-document signature is not a state.** The same line's
  document-relative top moved 14px between two stops of one sweep, because the
  height estimate for the lines above it improved. `signature` therefore records
  classes, left edge, size and text, and leaves vertical position to rule 3,
  which measures it against a card it scrolls to, with a tolerance.
- **`scrollHeight` is an estimate too.** A sweep that held the first reading
  ended early and missed the last group of the document. Every sweep re-reads it
  at every stop.
- **`locator.hover()` waits out its timeout on a rebuilt element.** Putting
  `hoverClasses` on a group rebuilds every line element in it, so the element a
  locator resolved a moment ago is detached; at 84 cards that is eleven minutes
  and it reads as a three-minute test timeout. Hovers in a sweep are real mouse
  moves — `hoverBox` in the harness.
- **An index is not an identity.** `sweepEach` chooses the next key in the page,
  next to the list it chose from, rather than reading the list in one call and
  acting on `nth(i)` in another.
