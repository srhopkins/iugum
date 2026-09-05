/**
 * RULE 1 — CONTAINMENT.
 *
 * Nothing a card or a group draws may leave the box that draws it.
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
  containmentViolations,
  expect,
  failWithArtifacts,
  gotoFixture,
  openBoard,
  openInline,
  type SBServer,
  setDensity,
  setWidth,
  startSpace,
  sweepBoxes,
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

async function checkContainment(view: View, combo: Combo) {
  for (const kind of kindsFor(view)) {
    const sweep = await sweepBoxes(view, kind.spec);

    // Count the boxes that carry an Atomdown ID, not every box.
    //
    // The sweep's job here is to prove it reached the end of the document, and
    // 82 distinct atom ids proves that completely. Counting every box dragged
    // a fragile key into the assertion: the two implicit cards a fenced code
    // block produces have no id, so they are keyed by their own text, and at
    // narrow width that text wraps differently enough that one of them keyed
    // twice and the sweep reported 85 of 84. The implicit cards are still
    // MEASURED — every box goes through the containment check below — they
    // just no longer decide whether the sweep was complete.
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
        await checkContainment(view, combo);
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
