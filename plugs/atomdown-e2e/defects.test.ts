/**
 * THE DEFECT-INJECTION SUITE — proof that the rules fail.
 *
 * A test that has never failed is not a test. This file reintroduces real
 * defects from the evening the six rules were written for, and asserts that
 * the corresponding rule's own checker REPORTS them. It is the negative
 * control: if the rules ever stop failing on these, the rules are broken even
 * when the views are fine.
 *
 * HOW A DEFECT IS INJECTED. Not by editing the plugs — another agent owns
 * those files, and a suite that has to patch production code to test itself is
 * a suite nobody runs. Each defect is a stylesheet injected into the real page
 * once the view is open, so the broken rule is genuinely in the document and
 * disappears with the browser context.
 *
 * That has one honest limit, stated here rather than buried: CSS reproduces
 * every GEOMETRY and VISIBILITY defect faithfully, and cannot reproduce a
 * state-machine defect — a group that will not expand is a JavaScript bug, not
 * a style. So rules 1, 2, 3 and 5 and the grip's side are proven here against
 * injected CSS, and the state-machine halves of rules 4 and 7 are covered by
 * the "the toggle did nothing" guard inside rule 4's own round-trip helper,
 * which fires when a toggle leaves the DOM byte-identical.
 *
 * Run it on its own — the pre-push gate does not, because these tests are
 * SUPPOSED to see violations and would read as failures:
 *
 *   scripts/atomdown-fe-check.sh --defects
 */

import { test, type Page } from "@playwright/test";
import {
  containmentViolations,
  directiveStates,
  expect,
  gotoFixture,
  openInline,
  revealedCount,
  type SBServer,
  settle,
  startSpace,
  sweepBoxes,
  visibleText,
} from "./harness.ts";

/**
 * Inject the defect's CSS straight into the page.
 *
 * The first version wrote a `space-style` page into the test's temporary
 * space, which is the mechanism a real style change would use. It did not
 * work reliably: SilverBullet picks a space-style block up through its own
 * index, and a page seeded milliseconds before the first load is not indexed
 * yet, so four of these tests silently exercised an unbroken page and
 * reported the RULES as broken. That is the worst possible failure mode for a
 * negative control.
 *
 * `addStyleTag` puts the rule in the document with no indexing in between, so
 * the defect is either present or the call threw. The CSS is genuinely in the
 * page either way; only the delivery changed.
 */
async function injectDefect(page: Page, css: string) {
  await page.addStyleTag({ content: css });
  await settle(page, 4);
}

const CARD_LINE = "#sb-main .cm-editor .cm-line.atomdown-card-line";

test.describe("rule 1 — containment", () => {
  test("fails when ordered-list markers render left of the card border", async ({
    page,
  }) => {
    // The defect verbatim: markers `1.`-`6.` pulled outside the card's left
    // edge. A negative margin puts the marker's rect outside its line's rect
    // while the DOM still says the marker is inside the line, which is why no
    // structural assertion sees it and a rect comparison does.
    const server: SBServer = await startSpace();
    const css = (
        `${CARD_LINE} .cm-list-bullet,
         ${CARD_LINE} .sb-line-ol .cm-list-bullet,
         ${CARD_LINE} span.cm-list-bullet {
           margin-left: -40px !important;
           position: relative !important;
           left: -40px !important;
         }`
    );
    try {
      await gotoFixture(page, server);
      const view = await openInline(page);
      await injectDefect(page, css);
      const sweep = await sweepBoxes(view, {
        name: "inline card",
        runPrefix: "atomdown-card",
        headerSelector: ".atomdown-card-header",
      });
      const violations = containmentViolations(sweep.boxes);
      const left = violations.filter((v) => v.side === "left");
      expect(
        left.length,
        "rule 1 must report a left-side overflow when list markers are pulled " +
          "outside the card border. If this is zero, rule 1 has stopped " +
          "measuring the thing it exists for.",
      ).toBeGreaterThan(0);
      expect(
        Math.max(...left.map((v) => v.overflowPx)),
        "and the reported overflow must be the real distance, not a rounding",
      ).toBeGreaterThan(10);
    } finally {
      await server.stop();
    }
  });

  test("fails when a wide table crosses the card and group borders", async ({
    page,
  }) => {
    // The other containment defect: a table wider than its card. Forcing an
    // explicit over-wide table with `table-layout: auto` and no wrapping is
    // exactly the state the plugs' `table-layout: fixed` plus wrapped cells
    // exists to prevent.
    const server: SBServer = await startSpace();
    const css = (
        `#sb-main .cm-editor table {
           table-layout: auto !important;
           width: 2000px !important;
           max-width: none !important;
         }
         #sb-main .cm-editor table td,
         #sb-main .cm-editor table th {
           white-space: nowrap !important;
           overflow: visible !important;
         }`
    );
    try {
      await gotoFixture(page, server);
      const view = await openInline(page);
      await injectDefect(page, css);
      // The group's SIDE borders are what a wide table crosses.
      const groups = await sweepBoxes(view, {
        name: "inline group",
        selector: ".atomdown-group-line",
        sidesOnly: true,
      });
      const cards = await sweepBoxes(view, {
        name: "inline card",
        runPrefix: "atomdown-card",
        headerSelector: ".atomdown-card-header",
      });
      const all = [
        ...containmentViolations(groups.boxes),
        ...containmentViolations(cards.boxes),
      ];
      const right = all.filter((v) => v.side === "right");
      expect(
        right.length,
        "rule 1 must report a right-side overflow when the table is forced " +
          "wider than its card and its group",
      ).toBeGreaterThan(0);
      expect(
        right.some((v) => /table|td|th|tr/i.test(v.child)),
        `and it must name the table as the child that escaped, not something ` +
          `incidental. Got: ${right.slice(0, 3).map((v) => v.child).join(" | ")}`,
      ).toBe(true);
    } finally {
      await server.stop();
    }
  });
});

test.describe("rule 2 — directive invisibility", () => {
  test("fails when a directive line is visible at rest", async ({ page }) => {
    // The defect: 82 sha256 digests back on the page. Undoing the collapse is
    // a two-line style change, which is roughly how it regressed.
    const server: SBServer = await startSpace();
    const css = (
        // `line-height` is the one that matters, and leaving it out is why
        // this injection first failed to reproduce anything. The plug hides a
        // directive with `font-size: 0; line-height: 0`, so overriding height
        // and font size alone still leaves a 0px line: the line box is sized
        // by line-height.
        `#sb-main .cm-editor .cm-line.atomdown-directive {
           font-size: 13px !important;
           line-height: 1.5 !important;
           color: #333 !important;
           height: auto !important;
           min-height: 18px !important;
           max-height: none !important;
           overflow: visible !important;
           opacity: 1 !important;
         }`
    );
    try {
      await gotoFixture(page, server);
      await openInline(page);
      await injectDefect(page, css);
      await page.evaluate(() =>
        (globalThis as any).client.editorView.contentDOM.blur(),
      );
      await settle(page, 4);

      const atRest = await directiveStates(page);
      expect(atRest.length, "directives must be realised to be measured")
        .toBeGreaterThan(0);
      expect(
        atRest.filter((d) => d.overBudget).length,
        "rule 2 must report directive lines over their height budget once the " +
          "collapse is undone",
      ).toBeGreaterThan(0);
      expect(
        atRest.filter((d) => d.showsText).length,
        "and it must report them as showing legible text, which is the half " +
          "that matters to a reader",
      ).toBeGreaterThan(0);
      expect(
        await revealedCount(page),
        "the fast hot-path check must agree with the detailed one — rule 2's " +
          "hover sweep uses only the fast one, so a disagreement means the " +
          "sweep is blind",
      ).toBeGreaterThan(0);
    } finally {
      await server.stop();
    }
  });

  test("fails when hovering a card reveals its directive", async ({ page }) => {
    // The interaction half. A directive that leaks only on hover passes an
    // at-rest check completely, which is why rule 2 sweeps the pointer over
    // every card rather than measuring once.
    const server: SBServer = await startSpace();
    const css = (
        `#sb-main .cm-editor .cm-line.atomdown-card-line:hover
           + .cm-line.atomdown-directive,
         #sb-main .cm-editor .atomdown-card-header:hover
           + .cm-line.atomdown-directive,
         #sb-main .cm-editor .atomdown-card-header:hover
           .atomdown-directive-peek {
           display: inline !important;
           font-size: 13px !important;
           color: #333 !important;
           height: auto !important;
           opacity: 1 !important;
         }`
    );
    try {
      await gotoFixture(page, server);
      await openInline(page);
      await injectDefect(page, css);
      await page.evaluate(() =>
        (globalThis as any).client.editorView.contentDOM.blur(),
      );
      await settle(page, 4);
      expect(
        await revealedCount(page),
        "nothing may be revealed before the pointer moves",
      ).toBe(0);

      let leaked = 0;
      const headers = page.locator(".atomdown-card-header");
      const n = Math.min(await headers.count(), 8);
      for (let i = 0; i < n; i++) {
        await headers.nth(i).hover().catch(() => {});
        await settle(page, 2);
        leaked += await revealedCount(page);
      }
      expect(
        leaked,
        "rule 2's pointer sweep must see a hover-only reveal. If this is " +
          "zero, hovering every card is measuring nothing and the rule only " +
          "checks the resting state.",
      ).toBeGreaterThan(0);
    } finally {
      await server.stop();
    }
  });
});

test.describe("rule 3 — layout stability", () => {
  test("fails when hovering a card grows it and pushes the page down", async ({
    page,
  }) => {
    // The defect shape: a hover state that changes a box's size rather than
    // only its colour. The board plug's own notes say a card's border may
    // change COLOUR and never WIDTH for exactly this reason, so widening it on
    // hover is the regression that note is guarding against.
    const server: SBServer = await startSpace();
    const css = (
        // `min-height` rather than padding. A hover that changes a card's
        // BOX is the defect shape — the board plug's own notes say a card's
        // border may change colour and never width for exactly this reason —
        // and min-height is the version CodeMirror cannot quietly absorb.
        `#sb-main .cm-editor .cm-line.atomdown-card-line:hover,
         #sb-main .cm-editor .atomdown-card-header:hover
           + .cm-line.atomdown-card-line {
           min-height: 120px !important;
           padding-top: 40px !important;
           padding-bottom: 40px !important;
         }`
    );
    try {
      await gotoFixture(page, server);
      const view = await openInline(page);
      await injectDefect(page, css);

      const topOf = () =>
        page.evaluate(() => {
          const scroller = document.querySelector(".cm-scroller")!;
          const headers = document.querySelectorAll(".atomdown-card-header");
          const ref = headers[headers.length - 1];
          if (!ref) return null;
          return (
            ref.getBoundingClientRect().top -
            scroller.getBoundingClientRect().top +
            scroller.scrollTop
          );
        });

      const before = await topOf();
      expect(before, "a reference card must be measurable").not.toBeNull();

      await page.locator(".atomdown-card-header").first().hover();
      await settle(page, 4);
      const after = await topOf();

      expect(
        Math.abs((after ?? 0) - (before ?? 0)),
        "rule 3 must see a reference card move when a hover state changes a " +
          "card's height. If this is zero, rule 3's y-position comparison is " +
          "not sensitive to the thing it measures.",
      ).toBeGreaterThan(1);
      await view.close();
    } finally {
      await server.stop();
    }
  });
});

test.describe("rule 5 — rendering fidelity", () => {
  test("fails when hidden raw markdown becomes visible", async ({ page }) => {
    // Rule 5 reads VISIBLE text, not `textContent`, so its own sensitivity is
    // worth proving: unhide the directive lines and the digests must show up
    // in what it reads. This is the assertion that would have caught the first
    // version of rule 5, which read `textContent` and reported 13 leaks on a
    // page where a reader could see none.
    const server: SBServer = await startSpace();
    const css = (
        `#sb-main .cm-editor .cm-line.atomdown-directive {
           font-size: 13px !important;
           line-height: 1.5 !important;
           color: #333 !important;
           height: auto !important;
           min-height: 18px !important;
           overflow: visible !important;
         }`
    );
    try {
      await gotoFixture(page, server);
      const view = await openInline(page);
      await injectDefect(page, css);
      const text = await visibleText(view.ev, ".cm-content");
      expect(
        text,
        "rule 5's visible-text reader must pick up a digest once the " +
          "directive is unhidden",
      ).toContain("sha256:");
      expect(
        text,
        "and the directive comment itself",
      ).toContain("<!-- <atom");
      await view.close();
    } finally {
      await server.stop();
    }
  });

  test("passes clean, so the assertion above is not vacuous", async ({
    page,
  }) => {
    // The paired positive control. Without it, a `visibleText` that returned
    // the empty string would fail the test above for the wrong reason and
    // nobody would know.
    const server: SBServer = await startSpace();
    try {
      await gotoFixture(page, server);
      const view = await openInline(page);
      const text = await visibleText(view.ev, ".cm-content");
      expect(text.length, "the page has visible text").toBeGreaterThan(500);
      expect(
        text,
        "and with no defect injected it carries no digest",
      ).not.toContain("sha256:");
      await view.close();
    } finally {
      await server.stop();
    }
  });
});

test.describe("area 7 — components", () => {
  test("fails when the drag grip renders on the right instead of the left", async ({
    page,
  }) => {
    // The grip regression verbatim: the class stayed correct and the element
    // rendered on the wrong side, which is why area 7 measures the side.
    const server: SBServer = await startSpace();
    const css = (
        `#sb-main .cm-editor .atomdown-card-header {
           position: relative !important;
         }
         #sb-main .cm-editor .atomdown-card-header .atomdown-grip {
           position: absolute !important;
           right: 2px !important;
           left: auto !important;
           opacity: 1 !important;
           display: inline-block !important;
         }`
    );
    try {
      await gotoFixture(page, server);
      const view = await openInline(page);
      await injectDefect(page, css);
      await page.locator(".atomdown-card-header").first().hover();
      await settle(page, 3);

      const measured = await page.evaluate(() => {
        const host = document.querySelector(".atomdown-card-header");
        const grip = host?.querySelector(".atomdown-grip");
        if (!host || !grip) return null;
        const h = host.getBoundingClientRect();
        const g = grip.getBoundingClientRect();
        return {
          fromLeft: g.left - h.left,
          fromRight: h.right - g.right,
        };
      });

      expect(measured, "the grip must be measurable").not.toBeNull();
      expect(
        measured!.fromLeft > measured!.fromRight,
        `area 7 must see the grip on the wrong side: it measured ` +
          `${measured!.fromLeft.toFixed(1)}px from the left and ` +
          `${measured!.fromRight.toFixed(1)}px from the right. If this is ` +
          `false, the side assertion is not measuring the side.`,
      ).toBe(true);
      await view.close();
    } finally {
      await server.stop();
    }
  });
});
