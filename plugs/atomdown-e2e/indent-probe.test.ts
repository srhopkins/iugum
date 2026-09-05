/**
 * SCAFFOLDING, NOT A RULE. `--project=probe` only.
 *
 *   scripts/atomdown-fe-check.sh --probe indent-probe
 *
 * Prints the numbers rule 10 asserts against, so they are written from reality
 * rather than from the stylesheet:
 *
 *  - the x of the first glyph on EVERY visual row of a wrapped bullet, an
 *    ordered item, a nested item and a blockquote, next to the line's own
 *    border box, padding box and content box, and next to its card's border;
 *  - the horizontal and vertical distances at both densities, so the
 *    requirement-2 split can be seen rather than argued about;
 *  - the computed surface, border style, border width and border colour of a
 *    card and a group at rest, hovered and selected, at both densities.
 */

import { test } from "@playwright/test";
import {
  DENSITIES,
  gotoFixture,
  hoverBox,
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

/**
 * The per-row measurement, in the page.
 *
 * A soft-wrapped line is ONE element with several visual rows, so the rows are
 * not elements and cannot be measured with `getBoundingClientRect`. A `Range`
 * over the line's own contents returns one client rect per row, which is the
 * only reading of "the first glyph on each visual row" the DOM offers.
 */
const ROW_PROBE = `
(function () {
  window.__adRows = function (line) {
    const cs = getComputedStyle(line);
    const r = line.getBoundingClientRect();
    const bl = parseFloat(cs.borderLeftWidth) || 0;
    const pl = parseFloat(cs.paddingLeft) || 0;
    const ti = parseFloat(cs.textIndent) || 0;
    const range = document.createRange();
    range.selectNodeContents(line);
    const rects = Array.from(range.getClientRects())
      .filter((q) => q.width > 0.5 && q.height > 0.5);
    range.detach();
    // Group the rects into visual rows by their top edge.
    const rows = [];
    for (const q of rects) {
      const row = rows.find((z) => Math.abs(z.top - q.top) < 2);
      if (row) row.left = Math.min(row.left, q.left);
      else rows.push({ top: q.top, left: q.left });
    }
    rows.sort((a, b) => a.top - b.top);
    return {
      classes: line.className,
      text: (line.textContent || "").replace(/\\s+/g, " ").slice(0, 44),
      borderBoxLeft: r.left,
      borderLeftWidth: bl,
      paddingLeft: pl,
      paddingBoxLeft: r.left + bl,
      contentBoxLeft: r.left + bl + pl,
      textIndent: ti,
      rowLefts: rows.map((z) => Math.round(z.left * 100) / 100),
      rows: rows.length,
    };
  };
})();
`;

/** The card box a line belongs to, as a rect, from its own ::before. */
const CARD_PROBE = `
(function () {
  window.__adCard = function (line) {
    const cs = getComputedStyle(line, "::before");
    const r = line.getBoundingClientRect();
    const bl = parseFloat(getComputedStyle(line).borderLeftWidth) || 0;
    const off = parseFloat(cs.left) || 0;
    return { left: r.left + bl + off, cardBorderWidth: parseFloat(cs.borderLeftWidth) || 0 };
  };
})();
`;

async function install(page: any) {
  await page.evaluate(ROW_PROBE);
  await page.evaluate(CARD_PROBE);
}

test("probe: the hanging indent, row by row", async ({ page }) => {
  await gotoFixture(page, server);
  const view = await openInline(page);

  for (const density of DENSITIES) {
    await setDensity(view, density);
    for (const width of WIDTHS) {
      await setWidth(page, width);
      await settle(page, 4);
      await install(page);
      // Scroll to the `editor` group, which holds a bullet list with a nested
      // level and a blockquote in adjacent cards.
      await page.evaluate(() => {
        const el = document.querySelector(".cm-scroller")!;
        el.scrollTop = 1200;
      });
      await settle(page, 4);
      await install(page);
      const out = await page.evaluate(() => {
        const rows = (window as any).__adRows;
        const card = (window as any).__adCard;
        const kinds: Record<string, string> = {
          bullet: ".cm-line.atomdown-card-line.sb-line-ul",
          ordered: ".cm-line.atomdown-card-line.sb-line-ol",
          blockquote: ".cm-line.atomdown-card-line.sb-line-blockquote",
          heading: ".cm-line.atomdown-card-line.sb-line-h2",
          plain: ".cm-line.atomdown-card-line:not([style])",
        };
        const out: any = {};
        for (const [name, sel] of Object.entries(kinds)) {
          const els = Array.from(document.querySelectorAll(sel));
          const wrapped = els.map((e) => ({
            ...rows(e),
            card: card(e),
          }));
          out[name] = {
            count: els.length,
            firstWrapped: wrapped.find((w: any) => w.rows > 1) ?? wrapped[0],
          };
        }
        return out;
      });
      console.log(
        `\n=== ROWS ${density}/${width} ===\n` + JSON.stringify(out, null, 1),
      );
    }
  }
});

test("probe: the two densities, distance by distance", async ({ page }) => {
  await gotoFixture(page, server);
  const view = await openInline(page);
  await setWidth(page, "comfort");

  const readings: Record<string, unknown> = {};
  for (const density of DENSITIES) {
    await setDensity(view, density);
    await settle(page, 4);
    await page.evaluate(() => {
      const el = document.querySelector(".cm-scroller")!;
      el.scrollTop = 1200;
    });
    await settle(page, 4);
    readings[density] = await page.evaluate(() => {
      const px = (v: string) => parseFloat(v) || 0;
      const member = document.querySelector(
        ".cm-line.atomdown-card-line.atomdown-group-line",
      ) as HTMLElement;
      const first = document.querySelector(
        ".cm-line.atomdown-card-line.atomdown-card-first",
      ) as HTMLElement;
      const last = document.querySelector(
        ".cm-line.atomdown-card-line.atomdown-card-last",
      ) as HTMLElement;
      const gFirst = document.querySelector(
        ".cm-line.atomdown-group-line.atomdown-group-first",
      ) as HTMLElement;
      const cs = (el: Element, p?: string) => getComputedStyle(el, p);
      const rootBg = cs(document.documentElement).backgroundColor;
      const line = member ?? first;
      const lcs = cs(line);
      const before = cs(line, "::before");
      const after = member ? cs(member, "::after") : null;
      return {
        rootBackground: rootBg,
        // HORIZONTAL: must be identical at both densities.
        lineBorderLeftWidth: lcs.borderLeftWidth,
        lineBorderLeftColor: lcs.borderLeftColor,
        linePaddingLeft: lcs.paddingLeft,
        cardBeforeLeft: before.left,
        cardBorderLeftWidth: before.borderLeftWidth,
        cardBorderStyle: before.borderLeftStyle,
        cardBorderColor: before.borderLeftColor,
        cardSurface: before.backgroundColor,
        groupAfterLeft: after?.left ?? null,
        groupBorderLeftWidth: after?.borderLeftWidth ?? null,
        groupBorderStyle: after?.borderLeftStyle ?? null,
        groupBorderColor: after?.borderLeftColor ?? null,
        groupSurface: after?.backgroundColor ?? null,
        // VERTICAL: must DIFFER between the densities.
        cardFirstPaddingTop: first ? px(cs(first).paddingTop) : null,
        cardLastPaddingBottom: last ? px(cs(last).paddingBottom) : null,
        groupFirstPaddingTop: gFirst ? px(cs(gFirst).paddingTop) : null,
      };
    });

    // Hovered and selected, for the compact register.
    const cardLine = page.locator(".cm-line.atomdown-card-line").first();
    await hoverBox(view, cardLine);
    await settle(page, 3);
    (readings[density] as any).hovered = await page.evaluate(() => {
      const el = document.querySelector(
        ".cm-line.atomdown-card-line.atomdown-card-hover",
      );
      if (!el) return null;
      const b = getComputedStyle(el, "::before");
      return {
        width: b.borderLeftWidth,
        style: b.borderLeftStyle,
        color: b.borderLeftColor,
        surface: b.backgroundColor,
        shadow: b.boxShadow,
      };
    });
    await cardLine.click({ force: true });
    await settle(page, 3);
    (readings[density] as any).selected = await page.evaluate(() => {
      const el = document.querySelector(
        ".cm-line.atomdown-card-line.atomdown-selected-line",
      );
      if (!el) return null;
      const b = getComputedStyle(el, "::before");
      return {
        width: b.borderLeftWidth,
        style: b.borderLeftStyle,
        color: b.borderLeftColor,
        surface: b.backgroundColor,
        shadow: b.boxShadow,
      };
    });
  }
  console.log("\n=== DENSITIES ===\n" + JSON.stringify(readings, null, 1));
});
