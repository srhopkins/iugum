/**
 * SCAFFOLDING, NOT A RULE. `--project=probe` only.
 *
 * Prints what the two moved controls and the new popover actually measure, at
 * all four editor widths and both densities, so the numbers rule 1 and rule 7
 * assert against are written from reality rather than from the stylesheet.
 */

import { test } from "@playwright/test";
import {
  DENSITIES,
  expect,
  gotoFixture,
  openInline,
  type SBServer,
  setDensity,
  settle,
  setWidth,
  startSpace,
  WIDTHS,
} from "./harness.ts";

let server: SBServer;
test.beforeAll(async () => (server = await startSpace()));
test.afterAll(async () => await server?.stop());

test("probe: where the gutter controls land", async ({ page }) => {
  await gotoFixture(page, server);
  const view = await openInline(page);

  for (const density of DENSITIES) {
    await setDensity(view, density);
    for (const width of WIDTHS) {
      await setWidth(page, width);
      await settle(page);
      const out = await page.evaluate(() => {
        function pick(sel: string) {
          const els = Array.from(document.querySelectorAll(sel));
          // The first one whose card line is on screen.
          for (const el of els) {
            const r = el.getBoundingClientRect();
            if (r.top > 40 && r.bottom < innerHeight - 40 && r.width > 0) {
              return el as HTMLElement;
            }
          }
          return (els[0] as HTMLElement) ?? null;
        }
        const scroller = document.querySelector(".cm-scroller")!;
        const sr = scroller.getBoundingClientRect();

        function report(header: HTMLElement | null, label: string) {
          if (!header) return { label, missing: true };
          const head = header.querySelector(".atomdown-card-head") as HTMLElement;
          const grip = header.querySelector(".atomdown-grip") as HTMLElement;
          const menu = header.querySelector(".atomdown-card-menu") as HTMLElement;
          const hr = (head ?? header).getBoundingClientRect();
          const gr = grip?.getBoundingClientRect();
          const mr = menu?.getBoundingClientRect();
          return {
            label,
            head: { l: +hr.left.toFixed(1), r: +hr.right.toFixed(1) },
            gripRightOfHeadLeft: gr ? +(gr.right - hr.left).toFixed(1) : null,
            gripClearOfScrollerLeft: gr ? +(gr.left - sr.left).toFixed(1) : null,
            menuLeftOfHeadRight: mr ? +(mr.left - hr.right).toFixed(1) : null,
            menuClearOfScrollerRight: mr ? +(sr.right - mr.right).toFixed(1) : null,
            gripGlyph: grip?.textContent,
            menuGlyph: menu?.textContent,
            menuCode: menu?.textContent?.codePointAt(0)?.toString(16),
          };
        }

        return {
          scroller: { l: +sr.left.toFixed(1), r: +sr.right.toFixed(1) },
          scrollWidthVsClient:
            scroller.scrollWidth - (scroller as HTMLElement).clientWidth,
          topLevel: report(
            pick(".sb-decoration-widget.atomdown-card-header:not(.atomdown-nested)"),
            "top-level",
          ),
          nested: report(
            pick(".sb-decoration-widget.atomdown-card-header.atomdown-nested"),
            "nested",
          ),
          groupMenuGlyph:
            document.querySelector(".atomdown-group-menu")?.textContent ?? null,
        };
      });
      console.log(`${density}/${width}: ` + JSON.stringify(out, null, 2));
    }
  }

  await view.close();
  expect(true).toBe(true);
});
