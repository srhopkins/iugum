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
          // THE GROUP'S OWN TWO CONTROLS (iugum-938), measured against the
          // group header widget's border box - which is the group container's
          // top edge and carries the 2px accent outline.
          group: (function () {
            const bar = pick(".sb-decoration-widget.atomdown-group-header");
            if (!bar) return { missing: true };
            const grip = bar.querySelector(
              ":scope > .atomdown-group-grip",
            ) as HTMLElement;
            const menu = bar.querySelector(
              ":scope > .atomdown-group-menu",
            ) as HTMLElement;
            const caret = bar.querySelector(
              ".atomdown-group-collapse",
            ) as HTMLElement;
            const br = bar.getBoundingClientRect();
            const gr = grip?.getBoundingClientRect();
            const mr = menu?.getBoundingClientRect();
            const memberHead = pick(".atomdown-card-head.atomdown-nested");
            const mg = memberHead
              ?.querySelector(".atomdown-grip")
              ?.getBoundingClientRect();
            const mm = memberHead
              ?.querySelector(".atomdown-card-menu")
              ?.getBoundingClientRect();
            return {
              bar: { l: +br.left.toFixed(1), r: +br.right.toFixed(1) },
              gripClearOfBarLeft: gr ? +(br.left - gr.right).toFixed(1) : null,
              menuClearOfBarRight: mr ? +(mr.left - br.right).toFixed(1) : null,
              gripClearOfScrollerLeft: gr ? +(gr.left - sr.left).toFixed(1) : null,
              menuClearOfScrollerRight: mr
                ? +(sr.right - mr.right).toFixed(1)
                : null,
              // The lane gap against a member card's own gutter controls.
              memberGripLaneGap: gr && mg ? +(mg.left - gr.left).toFixed(1) : null,
              memberMenuLaneGap: mr && mm
                ? +(mr.right - mm.right).toFixed(1)
                : null,
              caretInBar: caret
                ? +(caret.getBoundingClientRect().left - br.left).toFixed(1)
                : null,
              caretOpacity: caret ? getComputedStyle(caret).opacity : null,
            };
          })(),
        };
      });
      console.log(`${density}/${width}: ` + JSON.stringify(out, null, 2));
    }
  }

  await view.close();
  expect(true).toBe(true);
});
