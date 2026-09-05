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
scripts/atomdown-fe-check.sh --visual   # rule 9, the visual baselines, only
scripts/atomdown-fe-check.sh --visual --update-snapshots   # re-take them
```

The default runs **two Playwright projects**: `atomdown` (rules 1 to 8) and
`visual` (rule 9). They are separate because rule 9 needs its own browser
policy - see "Rule 9" below.

## Rule 1 has two halves: content and chrome

Rule 1 used to say "nothing a card draws may leave the box that draws it". The
inline view's per-card controls now sit outside the card's border on purpose
(the drag grip in the left page gutter, the vertical three-dot menu in the
right one), so the rule was split rather than relaxed:

- **CONTENT** - lines, list markers, blockquote bars, table cells, fenced code,
  links - must stay inside the box. That is `CHILD_SELECTOR` in the harness and
  it has not lost an entry.
- **CHROME** - the grip, the two three-dot buttons, the collapse caret and the
  popover - may sit outside, and must instead be (a) PAINTED where its rect
  says it is, (b) inside the editor's own scroll container, and (c) clear of
  the group's 2px accent outline. That is `chromeViolations`.

(a) is a hit test, not a measurement: a clipped element still reports its full
unclipped rect, so only `document.elementFromPoint` can tell the difference.

Board chrome stays inside its card and is still checked as content, so nothing
about the board's half of rule 1 changed.

Four cases in the negative control prove the chrome half fails: a gutter wide
enough to push a control off the scroller, an overlay on top of one, a control
on the group outline, and the clean page. The first draft of the checker passed
all three broken pages - it counted an ancestor on top as "painted", and it
gated the geometric checks behind the paint test's own on-screen guard.

## Rule 9: visual regression, in one pinned environment

`9-visual.test.ts`. It exists because no behavioural assertion can see a
colour: Steve reported a hover that painted a white hole in a saturated blue
bar while every other rule was green.

**The environment is a hard requirement, not a default.** Headless only,
Playwright's own bundled Chromium at the version in
`silverbullet/package-lock.json`, never the system Chrome and never an
`executablePath`. `--headed` or `PWDEBUG` refuses the run before the first
capture, so a headed run can neither produce nor update a baseline; nothing
opens a window, takes focus or appears in anyone's browser. Viewport,
`deviceScaleFactor`, `colorScheme` per theme case, `reducedMotion`, animations,
the caret, the timezone and the locale are all pinned, and every wait is on a
condition rather than a clock.

`ATOMDOWN_FE_CHANNEL`, which switches rules 1 to 8 to a system browser, is
refused here.

**Fonts.** Not pinned - the space's CSS resolves to whatever the host provides
and this repo ships no font file - so every glyph in the chrome is coloured
transparent before a capture. That removes the text's pixels and leaves every
fill, border, radius and shadow where it was, which is where this defect lives.
Playwright's own `mask` option is deliberately unused: it paints a box over the
region, and the first draft masked the two CONTROLS, so a solid white chip on
the accent fill passed all twelve baselines. Residual limit: transparent text
still takes its advance width, so a chip's width is font-dependent even though
its fill is not. Hence per-platform baselines (`-darwin.png`), a recorded
browser build in `visual-baselines/browser.txt`, and rule 8h asserting the
same property numerically - composited colours, which run anywhere.

**Zero tolerance** (`maxDiffPixels: 0`), and updating is one command, so nobody
is tempted to raise a threshold instead:

```sh
scripts/atomdown-fe-check.sh --visual --update-snapshots
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

## The rules, and the defect each one reproduces

| # | Rule | The defect it exists for |
|---|------|--------------------------|
| 1 | **Containment.** Every child rect inside its card box and its group box, allowing for border width. | Ordered-list markers `1.`-`6.` left of the card's left border. A wide table across the card border AND the group border. Group header controls clipped at the content column edge. |
| 2 | **Directive invisibility, one legitimate reveal.** At rest every directive contributes at most 4px and shows no text; nothing appears on any hover, any card top border, or any collapse click; the text cursor with the editor focused DOES reveal one, inside its card's borders. | 82 `sha256:` digests back on the page. A reveal that leaked on hover and passed every at-rest check. |
| 3 | **Layout stability.** A reference card's y never moves. Edit mode is the one exception and may grow its own card DOWNWARD only, with content below moving by exactly the height delta. | Hover states that resized a box instead of recolouring it, so reading the page moved it. |
| 4 | **State machine round trips.** Collapse, view on/off, raw/rendered, density (through the command AND through the header button) and the four editor widths each return to an identical DOM signature; reload persistence keeps on ON, off OFF and the density where it was left, scoped per page. | A group that would not expand after collapse. The header toggle doing nothing on first press while the command worked. Close-then-reload reopening the board. |
| 5 | **Rendering fidelity.** No `<!-- <atom`, no `sha256:`, no `](http`, no bare `##` or `**` outside code. Positively: one `<ol>` with six `<li>`, one `<table>` with 10 rows, an `<a href>` in every ticket cell. | Raw markdown reaching the reader. An ordered list rendering as a run-on paragraph. |
| 6 | **Document immutability.** After every interaction the page's bytes are unchanged and `atomdown lint` and `atomdown verify` both pass. An edit then one undo returns the same bytes. | A silent id, slug or digest rewrite: the file still lints, still renders, and the diff is churn nobody can evaluate. |

## Rule 8: the card's controls

`8-card-controls.test.ts`, one assertion per defect Steve found on the live
page, each stated as the property that was violated:

| | |
|---|---|
| 8a | The three-dot control opens a POPOVER anchored to the card, and no filter picker appears. It used to call `editor.filterBox`, which is the command palette's own surface. |
| 8b | The popover's first child is an inert name-and-id label. Clicking it, or the popover's own padding, keeps the menu open; a click elsewhere closes it. |
| 8c | The glyph is U+22EE VERTICAL ELLIPSIS, asserted by CODE POINT. Never from an image: a screenshot cannot say which character it is, and U+22EE and U+22EF differ by one bit. |
| 8d | Both card controls sit OUTSIDE the card's border, measured against the card head's own border rect, at all four widths and both densities, for a top-level card and a card inside a group. |
| 8e | Both are hidden at rest and revealed by hover AND by keyboard focus. An open popover keeps its own button lit. |
| 8f | None of it writes a document byte: hover, open, click the label, close, switch density twice, compare the file. |
| 8g | The popover holds NO text input. |
| 8h | The group control's hover is a translucent wash, not an opaque chip, measured as a composited colour rather than as pixels. |

**8g is a negative assertion with a measured reason.** The decoration seam's
`widgetPressGuard` calls `preventDefault` on a plain `mousedown` inside any
widget, so an input in a popover takes no focus from a click - and the
keystrokes that follow go INTO THE DOCUMENT. `input-probe.test.ts` records
that: after clicking an input inside a card header widget,
`document.activeElement` was `.cm-content` and the typed text appeared in the
page. So the popover is buttons only, and 8g is the guard rail that says why.

**8h and rule 9 cover the same defect from two sides.** 8h is font-free and
runs on any machine; rule 9 sees the thing a number cannot describe.

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

Three axes: two densities (**both views**, since the inline view gained its
own), four editor widths from `Library/Styles/EditorWidth.md`, light and dark
theme.

The two views report their density differently and `setDensity` in the harness
hides the difference. The panel has a root element, so it carries
`data-board-density`. The inline view has none - it decorates the real page -
so the plug puts `atomdown-comfortable` or `atomdown-compact` on every
decorated line and every widget, and `inlineDensity` reads that class. The
class is deliberately the readout rather than clientStore: it is what the CSS
keys off, so it is the only value that can disagree with what is on screen.

- **FAST** (default, what the hook runs): four cells, one per axis value. Every
  width appears once, both themes appear, both densities appear.
- **FULL** (`--full`): the cross product, 16 cells.

A defect that needs a *pair* of specific axis values escapes the fast subset.
That is the trade, stated plainly: the fast subset is a gate people keep, and
`--full` is what you run before a release or when chasing something that only
shows up in one combination.

Area 7 is in the fast subset on purpose - those are the assertions most likely
to catch a regression on an ordinary change. So is rule 9: twelve captures on
the fast matrix cost about 24 seconds, and the defect it exists for is
invisible to everything else. `--full` takes rule 9 to the full 16 cells with
the rest.

Rule 8d walks all four widths and both densities on its own, outside the combo
matrix, because a control's clearance from the card border is exactly the
measurement that changes with width.

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
- **The browser, for rules 1 to 8 only.** Those rules prefer Playwright's
  pinned Chromium and fall back to the installed Google Chrome when that
  download is missing. Chrome auto-updates, so measurements are marginally
  less stable on the fallback. `ATOMDOWN_FE_CHANNEL=chromium` forces the
  pinned build; the runner prints which one it used. **Rule 9 has no
  fallback**: it refuses to run without the bundled build, because a browser
  that auto-updates is how two machines silently disagree about a pixel. The
  runner checks for it and prints the one-time install command.
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

**Green on the fast matrix, both views, both densities: every rule, every
component test, rule 8's eight properties and rule 9's twelve baselines.** No
`test.fixme` is left in the suite. One test still skips with a reason rather
than failing — the grip drag, when a synthetic pointer drag produces no change;
see "what is not deterministic" above.

**Three things the density work found, worth knowing before changing the
suite.** A page load resets `html[data-editor-width]`, so any test that
reloads has to call `setWidth` again before it measures a width. `locator
.hover()` cannot put a pointer on a compact card header — the header is a
`pointer-events: none` layer with no height of its own — so the pointer goes
onto the CARD and `hoverBox` does it. And `sweepEach` addresses the element it
chose by a stamped attribute rather than by `nth(index)`: the interaction
before it rebuilds the line elements, so the index no longer named the element
the key named, and a sweep over eleven carets clicked one twice and another
never.

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
