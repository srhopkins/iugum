/**
 * RULE 1 — CONTAINMENT.
 *
 * A card's or a group's CONTENT may not leave the box that draws it. Its
 * CHROME may, and is checked for three other things instead.
 *
 * THE SPLIT, AND WHY IT IS NOT A WEAKENING (iugum-caj). This rule used to say
 * "nothing a card draws may leave the box that draws it", full stop. The
 * inline view's per-card controls — the drag grip and the vertical three-dot
 * button — now sit deliberately OUTSIDE the card's border, in the page
 * gutter, and its popover hangs below the card. So the rule now has two
 * halves:
 *
 *   CONTENT — every line, list marker, blockquote bar, table cell, fenced
 *   code block and link — must stay inside the box, exactly as before. The
 *   list is `CHILD_SELECTOR` and it has not lost an entry.
 *
 *   CHROME — the grip, the three-dot buttons, the collapse caret and the
 *   popover — may sit outside, and must instead: be actually PAINTED where
 *   its rect says it is (a clipped element still reports its full rect, so
 *   this is hit-tested rather than measured), stay inside the editor's own
 *   scroll container, and not land on the group's 2px accent outline. See
 *   `chromeViolations` in the harness.
 *
 * The board panel's chrome stays inside its card, so it is still checked as
 * content and nothing about the board's half of this rule changed.
 *
 * For every card box and every group box, in both views, this measures
 * `getBoundingClientRect()` on the box and on every element inside it — lines,
 * ordered and unordered list markers, nested list markers, blockquote bars,
 * fenced code blocks, table cells and the longest link — and asserts each
 * child rect lies inside the parent rect, allowing for border width.
 *
 * THE DEFECTS THIS REPRODUCES. All three were found by Steve looking at a
 * screenshot while the unit tests were green:
 *
 *   - Ordered-list markers `1.`-`6.` rendering LEFT of the card's left border.
 *     A marker is positioned by a negative offset, so its rect starts outside
 *     its line's rect while staying inside the line as far as the DOM is
 *     concerned. Nothing but a rect comparison sees it. The fixture's
 *     `decisions` group carries the six-item ordered list it happened on.
 *   - A wide table crossing the card border AND the group border. The
 *     `resea` group's 10-row table is that table; both boxes are measured, so
 *     a table that clears the card and still crosses the group is caught.
 *   - The group header's controls clipping at the content column edge. Header
 *     widgets are measured as boxes in their own right, with Rename, Ungroup
 *     and the menu button as children, so a control pushed past the column
 *     fails here rather than in a screenshot three days later.
 *
 * The sweep is not optional. CodeMirror renders about 5 of the fixture's 82
 * cards on a 1440x900 viewport, so a version of this test without
 * `sweepBoxes` would measure a handful of cards near the top of the page and
 * pass while a defect sat 60 atoms down. The expected-count assertions are
 * what prove the sweep reached the end of the document.
 */

import { test } from "@playwright/test";
import {
  type Combo,
  combos,
  comboName,
  FIXTURE,
  chromeViolations,
  containmentViolations,
  expect,
  failWithArtifacts,
  gotoFixture,
  measureChrome,
  openBoard,
  openInline,
  type SBServer,
  setDensity,
  setWidth,
  startSpace,
  sweepBoxes,
  sweepEach,
  THEMES,
  type View,
} from "./harness.ts";

let server: SBServer;
test.beforeAll(async () => (server = await startSpace()));
test.afterAll(async () => await server?.stop());

/** The fixture's shape. A mismatch means the sweep missed part of the page. */
const EXPECT_CARDS = FIXTURE.cards;
const EXPECT_GROUPS = FIXTURE.groups;

type Kind = { name: string; spec: any; expect?: number };

function kindsFor(view: View): Kind[] {
  if (view.kind === "inline") {
    return [
      {
        name: "card box",
        expect: EXPECT_CARDS,
        spec: {
          name: "inline card",
          runPrefix: "atomdown-card",
          headerSelector: ".atomdown-card-header",
        },
      },
      {
        // Per group LINE, sides only. A group can be taller than the viewport,
        // so a stitched run would be a fragment with a wrong top and bottom;
        // its side borders are identical on every line, and the sides are what
        // a wide table crosses. See `BoxSpec.sidesOnly`.
        name: "group box (sides)",
        spec: {
          name: "inline group",
          selector: ".atomdown-group-line",
          sidesOnly: true,
        },
      },
      {
        name: "card header",
        expect: EXPECT_CARDS,
        spec: { name: "inline card header", selector: ".atomdown-card-header" },
      },
      {
        name: "group header",
        expect: EXPECT_GROUPS,
        spec: {
          name: "inline group header",
          selector: ".atomdown-group-header",
        },
      },
    ];
  }
  return [
    {
      name: "card box",
      expect: EXPECT_CARDS,
      spec: { name: "board card", selector: ".board-card" },
    },
    {
      name: "group box",
      expect: EXPECT_GROUPS,
      spec: { name: "board group", selector: ".board-group" },
    },
    {
      name: "card header",
      expect: EXPECT_CARDS,
      spec: { name: "board card header", selector: ".board-card-header" },
    },
    {
      name: "group header",
      expect: EXPECT_GROUPS,
      spec: { name: "board group header", selector: ".board-group-header" },
    },
  ];
}

/**
 * The CHROME half of rule 1: the inline controls that sit outside their box.
 *
 * Swept the same way the content half is, because CodeMirror realises about
 * five of the fixture's 82 cards on a 1440x900 viewport and a chrome check
 * that only saw those five would pass with a clipped control sixty atoms down.
 */
async function checkChrome(view: View, combo: Combo) {
  if (view.kind !== "inline") return;
  let seen = 0;
  const all: ReturnType<typeof chromeViolations> = [];
  await sweepEach(view, ".atomdown-card-header", async () => {
    const items = await measureChrome(view);
    seen += items.length;
    all.push(...chromeViolations(items));
  });

  if (seen === 0) {
    await failWithArtifacts(
      view.page,
      1,
      "containment (chrome) — no chrome was measured at all",
      combo,
      { seen },
      `inline: the chrome sweep measured nothing. Either the controls stopped ` +
        `being rendered or CHROME_SELECTOR no longer names them — a chrome ` +
        `pass over zero elements proves nothing.`,
    );
  }

  if (all.length) {
    // Dedupe: the sweep re-measures whatever is still on screen at the next
    // stop, so one clipped control reports once per stop it was visible at.
    const unique = new Map<string, (typeof all)[number]>();
    for (const v of all) unique.set(`${v.chrome}|${v.kind}|${v.detail}`, v);
    const worst = [...unique.values()].sort((a, b) => b.px - a.px);
    await failWithArtifacts(
      view.page,
      1,
      "containment (chrome) — a control is clipped, off the scroller, or on the group outline",
      combo,
      { count: unique.size, violations: worst.slice(0, 40) },
      `inline chrome: ${unique.size} violation(s). Worst: ${worst[0].chrome} ` +
        `(${worst[0].kind}) — ${worst[0].detail}.`,
    );
  }
}

async function checkContainment(view: View, combo: Combo) {
  for (const kind of kindsFor(view)) {
    const sweep = await sweepBoxes(view, kind.spec);

    // Count the boxes that carry an Atomdown ID, not every box.
    //
    // The sweep's job here is to prove it reached the end of the document, and
    // 82 distinct atom ids proves that completely. Counting every box dragged
    // a fragile key into the assertion: a card with no id is keyed by its own
    // text, and at narrow width that text wrapped differently enough that one
    // implicit card keyed twice and the sweep reported 85 of 84. Such a card
    // is still MEASURED — every box goes through the containment check below —
    // it just no longer decides whether the sweep was complete.
    //
    // The fixture no longer HAS one: the atomdown build that regenerated it
    // covers a fenced code block's opening line, so there are no uncovered
    // blocks. The id-only count is the right assertion either way. See
    // `FIXTURE.cards` in the harness and `iugum-zaw`.
    const identified = sweep.ids.filter((id) => /^[0-9A-Z]{8}$/.test(id));
    const wanted =
      kind.expect === EXPECT_CARDS ? FIXTURE.atoms : kind.expect;

    if (wanted !== undefined && identified.length !== wanted) {
      await failWithArtifacts(
        view.page,
        1,
        "containment — the sweep did not see the whole document",
        combo,
        {
          kind: kind.name,
          boxes: sweep.boxes.length,
          identified: identified.length,
          wanted,
          stops: sweep.stops,
          ids: sweep.ids,
        },
        `${view.kind} ${kind.name}: found ${identified.length} box(es) with an ` +
          `Atomdown id, expected ${wanted} (${sweep.boxes.length} boxes in all). ` +
          `Either the fixture changed shape or the scroll sweep stopped early — ` +
          `a containment pass over the wrong number of boxes proves nothing.`,
      );
    }

    const violations = containmentViolations(sweep.boxes);
    if (violations.length) {
      const worst = [...violations].sort((a, b) => b.overflowPx - a.overflowPx);
      await failWithArtifacts(
        view.page,
        1,
        "containment — a child left its box",
        combo,
        { kind: kind.name, count: violations.length, violations: worst.slice(0, 40) },
        `${view.kind} ${kind.name}: ${violations.length} child rect(s) outside their box. ` +
          `Worst: ${worst[0].child} is ${worst[0].overflowPx}px past the ${worst[0].side} ` +
          `edge of ${worst[0].bound} in ${worst[0].box}.`,
      );
    }
  }
}

for (const theme of THEMES) {
  test.describe(`theme=${theme}`, () => {
    test.use({ colorScheme: theme });

    for (const combo of combos().filter((c) => c.theme === theme)) {
      test(`inline: everything stays inside its box [${comboName(combo)}]`, async ({
        page,
      }) => {
        await gotoFixture(page, server);
        await setWidth(page, combo.width);
        const view = await openInline(page);
        await setDensity(view, combo.density);
        await checkContainment(view, combo);
        await checkChrome(view, combo);
      });

      test(`board: everything stays inside its box [${comboName(combo)}]`, async ({
        page,
      }) => {
        await gotoFixture(page, server);
        await setWidth(page, combo.width);
        const view = await openBoard(page);
        await setDensity(view, combo.density);
        await checkContainment(view, combo);
      });
    }
  });
}
