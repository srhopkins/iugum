/**
 * RULE 8i — WHERE THE GRIP'S DROP LANDS.
 *
 * The second half of rule 8 ("the card's controls",
 * `8-card-controls.test.ts`), in its own file because every test in here
 * WRITES the document and 8f asserts that nothing in that file writes a byte.
 * So each test gets its own space.
 *
 * THE DEFECT (`iugum-uuv`). Steve dragged the first card down by one position
 * on the live page and it landed at the bottom of an 84-card document. The
 * board panel had the same defect once and fixed it with geometry; the inline
 * view got it when both card controls moved out of the card and into the page
 * gutter.
 *
 * Measured cause, from `drag-probe.test.ts` on this fixture: the seam maps the
 * release point to a document position, and the vertical band of the NEXT
 * card's header widget maps to the blank line between the two cards. A blank
 * line is a unit boundary and carries no unit mark, so `targetMarks` arrives
 * empty — and an empty target used to mean "the start or the end of the whole
 * document", chosen from which side of that blank line's own midpoint the
 * pointer sat on. Releasing 80px below the first card's grip moved it from
 * index 0 to index 81.
 *
 * WHY THE GUARD MATTERS MORE THAN THE FIX. This regressed with nothing
 * noticing, because the drag test that existed (`7-components.test.ts`, 4b)
 * asserts that the file CHANGED and that no line was added or lost. A card
 * that jumps to the bottom of the document satisfies both. So every test here
 * asserts THE RESULTING UNIT ORDER, and the expected order is computed from
 * the order the document had before the drag.
 *
 * The property, stated as the plug states it: a drop goes where the pointer
 * is. The seam either names the unit under the release point, or the release
 * fell in the seam between two units — and then the drop belongs at that
 * seam, never at the end of the page. Only a release genuinely past the last
 * unit is the end.
 *
 * All of it runs at all four editor widths and both densities, because the
 * gutter the grip lives in is as wide as the page's margin, so its width
 * moves with the content column, and a card's height moves with the density.
 */

import { execFile } from "node:child_process";
import { existsSync } from "node:fs";
import { readFile } from "node:fs/promises";
import { join } from "node:path";
import { promisify } from "node:util";
import { test } from "@playwright/test";
import {
  DENSITIES,
  expect,
  failWithArtifacts,
  FIXTURE_PAGE,
  gotoFixture,
  mod,
  openInline,
  type Page,
  readPageBytes,
  type SBServer,
  setDensity,
  settle,
  setWidth,
  startSpace,
  type View,
  WIDTHS,
} from "./harness.ts";

const run = promisify(execFile);

// The plug's own unit scanner, imported rather than reimplemented: the order
// this rule asserts has to be the order the plug means by "unit", and a second
// copy of that rule in the test would let both drift together.
//
// It arrives as a data URL because the plug is a bare `.js` file with no
// `package.json` of its own, so Node loads it as CommonJS from here and
// chokes on its `export`. The bytes are the plug's own, unmodified — this is
// the same module `atomdown-inline.test.mjs` imports, not a copy.
(globalThis as any).self = { addEventListener() {}, postMessage() {} };
const { plug } = await import(
  "data:text/javascript," +
    encodeURIComponent(
      await readFile(
        join(import.meta.dirname, "..", "atomdown-inline", "atomdown-inline.plug.js"),
        "utf8",
      ),
    )
);
const { computeUnits } = (plug as any).internals;

/** The unit keys of a page's markdown, in document order. */
function unitKeys(markdown: string): string[] {
  return computeUnits(markdown).units.map((u: any) => u.unitKey);
}

/** Find the `atomdown` binary. The same candidate list rule 6 uses. */
function atomdownBin(): string | null {
  const home = process.env.HOME ?? "";
  return [
    process.env.ATOMDOWN_BIN,
    join(home, "go", "bin", "atomdown"),
    "/usr/local/bin/atomdown",
    "/opt/homebrew/bin/atomdown",
    join(home, "projects/github/srhopkins/atomdown/atomdown"),
  ].filter(Boolean).find((c) => existsSync(c as string)) as string ?? null;
}

/**
 * `atomdown lint` and `atomdown verify` on the page as it is now.
 *
 * A reorder that moved bytes it should not have touched, or that refreshed a
 * digest, is valid-looking markdown with the wrong content — and only the real
 * tool can say so. Skipped, loudly, when the binary is not on the machine.
 */
async function atomdownClean(server: SBServer, where: string) {
  const bin = atomdownBin();
  if (!bin) return;
  const path = join(server.spaceDir, `${FIXTURE_PAGE}.md`);
  for (const cmd of ["lint", "verify"] as const) {
    // A finding makes the tool exit non-zero, which `execFile` throws on, so
    // the output is read from either side of that.
    const out = await run(bin, [cmd, path])
      .then((r) => r.stdout.trim())
      .catch((e) => String(e.stdout ?? e.message).trim());
    expect(
      out,
      `after ${where}: atomdown ${cmd} must still say ok`,
    ).toMatch(/^ok/);
  }
}

/** Wait until the page's bytes are what they should be, or give up. */
async function waitForBytes(
  server: SBServer,
  want: (text: string) => boolean,
  timeoutMs = 6000,
): Promise<string> {
  const deadline = Date.now() + timeoutMs;
  let last = await readPageBytes(server);
  while (!want(last) && Date.now() < deadline) {
    await new Promise((r) => setTimeout(r, 200));
    last = await readPageBytes(server);
  }
  return last;
}

/**
 * One measured band per TOP-LEVEL unit on screen, in document order.
 *
 * THE DISTINCTION THAT MATTERS HERE is `contentBottom` against `bottom`.
 * `contentBottom` is the bottom of the unit's own last non-blank line: a
 * release there is ON the unit. `bottom` is the next header's top, and the
 * space between the two is the blank SEAM: a release there is BETWEEN the two
 * units, and that seam is exactly where the defect lived. A card header
 * inside a group is skipped, because a group is one unit.
 */
type Band = {
  index: number;
  top: number;
  contentBottom: number;
  bottom: number;
  headerTop: number;
  headerBottom: number;
  left: number;
  isGroup: boolean;
};

async function bands(page: Page): Promise<Band[]> {
  return await page.evaluate(() => {
    const heads = Array.from(
      document.querySelectorAll(
        ".atomdown-card-header:not(.atomdown-nested), .atomdown-group-header",
      ),
    ) as HTMLElement[];
    // Only lines with text: the blank line between two units is a `.cm-line`
    // too, and it is the seam rather than part of either unit.
    const lines = (Array.from(
      document.querySelectorAll(".cm-line"),
    ) as HTMLElement[])
      .filter((l) => (l.textContent ?? "").trim() !== "")
      .map((l) => l.getBoundingClientRect());
    return heads.map((el, i) => {
      const r = el.getBoundingClientRect();
      const nextTop = i + 1 < heads.length
        ? heads[i + 1].getBoundingClientRect().top
        : Infinity;
      const own = lines.filter((l) => l.top >= r.top && l.top < nextTop);
      const contentBottom = own.length
        ? own[own.length - 1].bottom
        : r.bottom;
      return {
        index: i,
        top: +r.top.toFixed(1),
        contentBottom: +contentBottom.toFixed(1),
        bottom: nextTop === Infinity ? +contentBottom.toFixed(1) : +nextTop.toFixed(1),
        headerTop: +r.top.toFixed(1),
        headerBottom: +r.bottom.toFixed(1),
        left: +r.left.toFixed(1),
        isGroup: el.classList.contains("atomdown-group-header"),
      };
    });
  });
}

/**
 * The grip's own rect for one on-screen unit, revealed by a hover.
 *
 * THE POINTER GOES ON THE CARD, not on the header. `locator.hover()` on a
 * compact card header waits forever: at that density the header is a
 * `pointer-events: none` layer with no height of its own, so it never
 * receives the pointer (the suite README says the same about `hoverBox`).
 * Hovering the card's own last line reveals the grip at both densities,
 * because the reveal hangs off the whole card's hover state.
 */
async function gripBox(page: Page, band: Band) {
  await page.mouse.move(band.left + 40, band.contentBottom - 4);
  await settle(page);
  const grip = page.locator(
    ".atomdown-card-header:not(.atomdown-nested), .atomdown-group-header",
  ).nth(band.index).locator(".atomdown-grip").first();
  const box = await grip.boundingBox({ timeout: 10_000 });
  expect(box, "the grip has no rect to drag from").not.toBeNull();
  return box!;
}

/** Press the grip and travel to (x, y) in steps, then release. */
async function dragGrip(
  page: Page,
  grip: { x: number; y: number; width: number; height: number },
  x: number,
  y: number,
) {
  const sx = grip.x + grip.width / 2;
  const sy = grip.y + grip.height / 2;
  await page.mouse.move(sx, sy);
  await page.mouse.down();
  for (let i = 1; i <= 6; i++) {
    await page.mouse.move(sx + ((x - sx) * i) / 6, sy + ((y - sy) * i) / 6, {
      steps: 2,
    });
  }
  await page.mouse.up();
  await settle(page, 4);
}

/** Scroll the editor to the very end and let it settle. */
async function scrollToEnd(page: Page) {
  await page.evaluate(async () => {
    const el = document.querySelector(".cm-scroller")!;
    for (let i = 0; i < 40; i++) {
      el.scrollTop = el.scrollHeight;
      await new Promise((r) => requestAnimationFrame(() => r(null)));
    }
  });
  await settle(page, 4);
}

/** Move one entry of a list to a new index, the expected-order oracle. */
function moved(order: string[], from: number, to: number): string[] {
  const out = order.slice();
  const [item] = out.splice(from, 1);
  out.splice(to, 0, item);
  return out;
}

/**
 * Drive one drag, then assert the whole unit order, `atomdown`, and that one
 * undo puts every byte back.
 */
async function expectDrop(
  page: Page,
  server: SBServer,
  label: string,
  combo: string,
  plan: (b: Band[]) => {
    source: number;
    /** The release point. `gutter` is the x of the source's own grip. */
    release: (gutter: number) => { x: number; y: number };
    /**
     * The acceptable unit orders after the drop.
     *
     * USUALLY EXACTLY ONE. Two only where the release is over a unit TALLER
     * THAN THE VIEWPORT — a group — because the seam decides `before` or
     * `after` from `coordsAtPos` on both ends of the range, and the far end
     * of such a range is not in the layout to measure. Both answers put the
     * card at the group's own seam, above it or below it, which is what this
     * rule is about; which of the two is the seam's business, not this
     * rule's. Landing anywhere else, the end of the document above all, still
     * fails.
     */
    expected: (before: string[]) => string[][];
  },
) {
  // MEASURED FRESH FOR EVERY DROP. The previous drop and its undo both
  // rewrote the document, so a rect read before them is a rect from another
  // layout — and a geometry test that does that is flaky rather than wrong.
  await settle(page, 4);
  const bandList = await bands(page);
  const step = plan(bandList);

  const beforeBytes = await readPageBytes(server);
  const beforeOrder = unitKeys(beforeBytes);
  const accept = step.expected(beforeOrder).map((o) => o.join("\n"));

  const grip = await gripBox(page, bandList[step.source]);
  const release = step.release(grip.x + grip.width / 2);
  await dragGrip(page, grip, release.x, release.y);

  // A DROP THAT ASKS FOR NO MOVE HAS NOTHING TO POLL FOR, so it gets a fixed
  // wait instead: the release fell in the seam the unit already occupies, and
  // the page must be byte-identical afterwards. Waiting for a change here is
  // what would make "it did not move" indistinguishable from "it has not
  // written yet".
  const noMove = accept.length === 1 &&
    accept[0] === beforeOrder.join("\n");
  if (noMove) await page.waitForTimeout(2500);
  const afterBytes = noMove ? await readPageBytes(server) : await waitForBytes(
    server,
    (t) => accept.indexOf(unitKeys(t).join("\n")) !== -1,
  );
  const afterOrder = unitKeys(afterBytes);
  if (accept.indexOf(afterOrder.join("\n")) === -1) {
    const want = accept[0].split("\n");
    const firstDiff = afterOrder.findIndex((k, i) => k !== want[i]);
    await failWithArtifacts(
      page,
      8,
      `8i: the grip's drop landed in the wrong place — ${label}`,
      combo,
      {
        release,
        firstDifferenceAtIndex: firstDiff,
        wantAround: want.slice(Math.max(0, firstDiff - 1), firstDiff + 3),
        gotAround: afterOrder.slice(Math.max(0, firstDiff - 1), firstDiff + 3),
        movedUnitWantedAt: want.indexOf(beforeOrder[step.source]),
        acceptableOrders: accept.length,
        unitsBefore: beforeOrder.length,
        unitsAfter: afterOrder.length,
      },
      `${label}: the unit order after the drop is not the order the ` +
        `release point asks for. A drop goes where the pointer is: either ` +
        `the unit under it, or the seam between the two units it fell ` +
        `between. Landing at the end of the document is the defect this ` +
        `rule exists for (iugum-uuv).`,
    );
  }

  // A reorder moves units. It may not add, lose or rewrite a line.
  const lines = (s: string) => s.split("\n").filter((l) => l.trim());
  expect(
    lines(afterBytes).length,
    `${label}: a reorder moves lines, it does not add or remove them`,
  ).toBe(lines(beforeBytes).length);
  expect(
    [...lines(afterBytes)].sort().join("\n"),
    `${label}: a reorder changes the ORDER of lines and nothing else — an ` +
      `id, a slug or a digest that changed would show up here`,
  ).toBe([...lines(beforeBytes)].sort().join("\n"));

  await atomdownClean(server, label);

  if (noMove) {
    expect(
      afterBytes === beforeBytes,
      `${label}: the drop asked for no move, so not one byte may change`,
    ).toBe(true);
    return;
  }

  // ONE transaction, so ONE undo reverts the whole move. The editor gets the
  // focus back first: the seam's press guard calls `preventDefault` on a
  // mousedown inside a widget, and a prevented mousedown moves no focus, so
  // after a grip drag the keystroke would otherwise go nowhere.
  await page.evaluate(() =>
    (document.querySelector(".cm-content") as HTMLElement | null)?.focus()
  );
  await page.keyboard.press(`${mod}+z`);
  const undone = await waitForBytes(server, (t) => t === beforeBytes);
  expect(
    undone === beforeBytes,
    `${label}: one undo must put the page back byte for byte — the write is ` +
      `one CodeMirror transaction by design`,
  ).toBe(true);
  await atomdownClean(server, `${label} + undo`);
}

for (const width of WIDTHS) {
  for (const density of DENSITIES) {
    const combo = `${width}/${density}`;

    test(`8i: a drop lands at the seam the pointer is on, not at the end of the page [${combo}]`, async ({
      page,
    }) => {
      // ITS OWN SPACE: this test rewrites the fixture seven times.
      const server = await startSpace();
      let view: View | null = null;
      try {
        await gotoFixture(page, server);
        await setWidth(page, width);
        view = await openInline(page);
        await setDensity(view, density);
        await settle(page, 4);

        const opening = await bands(page);
        expect(
          opening.length,
          "the top of the fixture must render at least three top-level units",
        ).toBeGreaterThanOrEqual(3);
        expect(
          opening.some((b) => b.isGroup),
          "the fixture's first group is not on screen",
        ).toBe(true);

        // 1. THE REGRESSION ITSELF, and it is the plainest gesture there is:
        //    press the FIRST card's grip and let go a little way down the
        //    gutter. That release lands in the blank SEAM under the card —
        //    which is the seam the card already occupies, so the answer is
        //    "nothing moves". The old code answered "the end of the document"
        //    and Steve watched the first card land at the bottom of the page.
        await expectDrop(
          page,
          server,
          "gutter, released in the seam under the first card",
          combo,
          (b) => ({
            source: 0,
            release: (gutter) => ({
              x: gutter,
              y: (b[0].contentBottom + b[1].headerTop) / 2,
            }),
            expected: (o) => [o],
          }),
        );

        // 1b. THE SAME SEAM, from below: the third unit, released in the seam
        //     between the first two, moves up to exactly the second place.
        //     Same empty-mark release, and this one has to MOVE, so it pins
        //     the resolved target rather than only "not the end".
        await expectDrop(
          page,
          server,
          "gutter, released in the seam between the first two units",
          combo,
          (b) => ({
            source: 2,
            release: (gutter) => ({
              x: gutter,
              y: (b[0].contentBottom + b[1].headerTop) / 2,
            }),
            expected: (o) => [moved(o, 2, 1)],
          }),
        );

        // 1c. THE DoD GESTURE. Release in the GUTTER beside the second unit,
        //     below its own midpoint: the first unit moves down exactly one
        //     position.
        await expectDrop(page, server, "gutter, one position down", combo, (
          b,
        ) => ({
          source: 0,
          release: (gutter) => ({ x: gutter, y: b[1].contentBottom - 2 }),
          expected: (o) => [moved(o, 0, 1)],
        }));

        // 2. The same release over the CARD BODY rather than the gutter.
        await expectDrop(
          page,
          server,
          "over the card body, one position down",
          combo,
          (b) => ({
            source: 0,
            release: () => ({ x: b[1].left + 40, y: b[1].contentBottom - 2 }),
            expected: (o) => [moved(o, 0, 1)],
          }),
        );

        // 3. Over a GROUP HEADER: the drop lands at the group's own seam,
        //    above it or below it. A group is one unit, so those are the only
        //    two places it can land, and it may never land inside the group
        //    or at the end of the page. Which of the two the seam picks is
        //    the seam's business — see `expected` in `expectDrop`.
        await expectDrop(page, server, "over the group header", combo, (b) => {
          const g = b.find((x) => x.isGroup)!;
          return {
            source: 0,
            release: () => ({ x: g.left + 40, y: g.headerTop + 3 }),
            expected: (o) => [moved(o, 0, g.index - 1), moved(o, 0, g.index)],
          };
        });

        // 4. Over a GROUP'S INTERIOR: the same two answers, because a group
        //    is ONE unit and a card may not land between its members.
        await expectDrop(
          page,
          server,
          "over the group's interior",
          combo,
          (b) => {
            const g = b.find((x) => x.isGroup)!;
            return {
              source: 0,
              release: () => ({ x: g.left + 40, y: g.headerBottom + 8 }),
              expected: (o) => [moved(o, 0, g.index - 1), moved(o, 0, g.index)],
            };
          },
        );

        // 5. ABOVE THE FIRST UNIT is the start of the page, and it must not
        //    reach the document marker above it.
        await expectDrop(page, server, "above the first unit", combo, (b) => ({
          source: 1,
          release: (gutter) => ({ x: gutter, y: b[0].headerTop - 8 }),
          expected: (o) => [moved(o, 1, 0)],
        }));

        // --- The other end of the document ------------------------------

        // 6. BELOW THE LAST UNIT is still the end of the page.
        await scrollToEnd(page);
        await expectDrop(page, server, "below the last unit", combo, (b) => {
          expect(
            b.length,
            "the end of the fixture must render at least two top-level units",
          ).toBeGreaterThanOrEqual(2);
          return {
            source: b.length - 2,
            release: (gutter) => ({
              x: gutter,
              y: b[b.length - 1].contentBottom + 6,
            }),
            expected: (o) => [moved(o, o.length - 2, o.length - 1)],
          };
        });

        // 7. THE LAST UNIT, dragged up by one position.
        await scrollToEnd(page);
        await expectDrop(
          page,
          server,
          "the last unit, up one position",
          combo,
          (b) => ({
            source: b.length - 1,
            release: (gutter) => ({
              x: gutter,
              y: b[b.length - 2].headerTop + 3,
            }),
            expected: (o) => [moved(o, o.length - 1, o.length - 2)],
          }),
        );
      } finally {
        await view?.close().catch(() => {});
        await server.stop();
      }
    });
  }
}
