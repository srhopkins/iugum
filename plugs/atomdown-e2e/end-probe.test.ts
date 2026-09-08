/**
 * SCAFFOLDING, NOT A RULE. `--project=probe` only.
 *
 * Prints what rule 8i's LAST case — the release below the last unit — actually
 * does, in every width/density combination, for both scroll strategies:
 *
 *  - `max`: scroll to the maximum `scrollTop`, which is what `scrollToEnd`
 *    does. This is the geometry the case had when it failed in seven of eight
 *    cells (`iugum-hkk`), and the probe reports the failure directly: the
 *    mousedown lands at a NEGATIVE client y on no element, `grip` is false,
 *    and NO `editor:decorationDrag` is dispatched at all. The document is
 *    unchanged because the gesture never happened.
 *  - `tail`: pull the source unit's header just below the top of the scroller,
 *    which is what `scrollToTail` in `8-drag-drop.test.ts` does. Every cell
 *    then presses the grip, reports `targetLine` past the last unit's
 *    `startLine`, and moves the unit to the last position.
 *
 * So the placement logic in `dragToReorder` was never the defect. This probe
 * is the measurement that says so, and it is kept for the next time a tail
 * gesture reads as "the drop did not move the unit".
 */

import { readFile } from "node:fs/promises";
import { join } from "node:path";
import { test } from "@playwright/test";
import {
  DENSITIES,
  gotoFixture,
  openInline,
  type Page,
  readPageBytes,
  type SBServer,
  setDensity,
  settle,
  setWidth,
  startSpace,
  WIDTHS,
} from "./harness.ts";

(globalThis as any).self = { addEventListener() {}, postMessage() {} };
const { plug } = await import(
  "data:text/javascript," +
    encodeURIComponent(
      await readFile(
        join(
          import.meta.dirname,
          "..",
          "atomdown-inline",
          "atomdown-inline.plug.js",
        ),
        "utf8",
      ),
    )
);
const { computeUnits } = (plug as any).internals;

/** The top-level units on screen, in document order. `8i`'s own reading. */
async function bands(page: Page) {
  return await page.evaluate(() => {
    const heads = Array.from(
      document.querySelectorAll(
        ".atomdown-card-header:not(.atomdown-nested), .atomdown-group-header",
      ),
    ) as HTMLElement[];
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
      return {
        index: i,
        headerTop: +r.top.toFixed(1),
        contentBottom: +(own.length ? own[own.length - 1].bottom : r.bottom)
          .toFixed(1),
        left: +r.left.toFixed(1),
      };
    });
  });
}

function scrollerBox(page: Page) {
  return page.evaluate(() => {
    const el = document.querySelector(".cm-scroller")! as HTMLElement;
    const r = el.getBoundingClientRect();
    return { top: r.top, bottom: r.bottom, scrollTop: el.scrollTop };
  });
}

async function scrollToMax(page: Page) {
  await page.evaluate(async () => {
    const el = document.querySelector(".cm-scroller")!;
    for (let i = 0; i < 40; i++) {
      el.scrollTop = el.scrollHeight;
      await new Promise((r) => requestAnimationFrame(() => r(null)));
    }
  });
  await settle(page, 4);
}

async function scrollToTail(page: Page) {
  await scrollToMax(page);
  for (let i = 0; i < 6; i++) {
    const list = await bands(page);
    if (list.length < 2) break;
    const box = await scrollerBox(page);
    const wanted = box.top + 12 - list[list.length - 2].headerTop;
    if (wanted <= 0) break;
    await page.evaluate((dy) => {
      const el = document.querySelector(".cm-scroller")! as HTMLElement;
      el.scrollTop -= dy;
    }, wanted);
    await settle(page, 4);
  }
}

for (const width of WIDTHS) {
  for (const density of DENSITIES) {
    for (const strategy of ["max", "tail"] as const) {
      test(`probe end: ${width}/${density} ${strategy}`, async ({ page }) => {
        const tag = `[${width}/${density} ${strategy}]`;
        const server: SBServer = await startSpace();
        try {
          await gotoFixture(page, server);
          await setWidth(page, width);
          const view = await openInline(page);
          await setDensity(view, density);
          await settle(page, 4);

          await page.evaluate(() => {
            const c = (globalThis as any).client;
            (globalThis as any).__drags = [];
            (globalThis as any).__downs = [];
            const real = c.dispatchAppEvent.bind(c);
            c.dispatchAppEvent = (name: string, ...rest: any[]) => {
              if (name === "editor:decorationDrag") {
                (globalThis as any).__drags.push(rest[0]);
              }
              return real(name, ...rest);
            };
            globalThis.addEventListener("mousedown", (e: any) => {
              const t = e.target as Element | null;
              (globalThis as any).__downs.push({
                grip: !!(t && t.closest && t.closest(".atomdown-grip")),
                y: +e.clientY.toFixed(0),
              });
            }, true);
          });

          if (strategy === "max") await scrollToMax(page);
          else await scrollToTail(page);

          const list = await bands(page);
          const box = await scrollerBox(page);
          const src = list[list.length - 2];
          const last = list[list.length - 1];
          const before = await readPageBytes(server);
          const keysBefore = computeUnits(before).units.map((u: any) =>
            u.unitKey
          );
          console.log(
            `${tag} scroller=${box.top}..${box.bottom} ` +
              `srcHeaderTop=${src.headerTop} lastContentBottom=${last.contentBottom}`,
          );

          await page.mouse.move(src.left + 40, src.contentBottom - 4);
          await settle(page);
          const grip = await page.locator(
            ".atomdown-card-header:not(.atomdown-nested), .atomdown-group-header",
          ).nth(src.index).locator(".atomdown-grip").first().boundingBox({
            timeout: 10_000,
          });
          const sx = grip!.x + grip!.width / 2;
          const sy = grip!.y + grip!.height / 2;
          const ry = last.contentBottom + 6;
          await page.mouse.move(sx, sy);
          await page.mouse.down();
          for (let i = 1; i <= 6; i++) {
            await page.mouse.move(sx, sy + ((ry - sy) * i) / 6, { steps: 2 });
          }
          await page.mouse.up();
          await settle(page, 4);
          await page.waitForTimeout(1500);

          const trace = await page.evaluate(() => ({
            downs: (globalThis as any).__downs,
            drags: (globalThis as any).__drags,
          }));
          const p = trace.drags.slice(-1)[0];
          const after = await readPageBytes(server);
          const keysAfter = computeUnits(after).units.map((u: any) => u.unitKey);
          console.log(
            `${tag} press=${JSON.stringify(trace.downs)} ` +
              `release=${ry.toFixed(0)} drags=${trace.drags.length}`,
          );
          console.log(
            `${tag} payload targetMarks=${
              JSON.stringify(p?.targetMarks ?? null)
            } placement=${p?.placement} targetLine=${p?.targetLine}`,
          );
          console.log(
            `${tag} sourceLandedAtIndex=` +
              `${keysAfter.indexOf(keysBefore[keysBefore.length - 2])} of ` +
              `${keysAfter.length} changed=${after !== before}`,
          );
          await view.close();
        } finally {
          await server.stop();
        }
      });
    }
  }
}
