/**
 * SCAFFOLDING, NOT A RULE. Runs under `--project=probe`; the gate never does.
 *
 * The one question this file answers, before any code is written for
 * `iugum-caj`: if a card's chrome is moved OUTSIDE the card's own box, does
 * the editor's overflow clip it?
 *
 * A card is a run of `.cm-line` elements inside CodeMirror's `.cm-content`,
 * which sits inside `.cm-scroller`. Either of those can carry an `overflow`
 * that hides a child sticking out sideways. The answer cannot be read from the
 * CSS, because `overflow: hidden` on ANY ancestor clips, and the client's own
 * stylesheet is not the only writer. So this measures:
 *
 *   1. every ancestor's computed `overflow-x` / `overflow-y`, up to `body`
 *   2. the content column's own left and right edges
 *   3. the same for a probe element pushed 28px past each side of a card, at
 *      all four editor widths, both top-level and inside a group
 *   4. whether that probe is actually PAINTED there, by hit-testing the point
 *      with `document.elementFromPoint`
 *
 * A rect alone is not proof: a clipped element still reports its unclipped
 * rect. `elementFromPoint` is the proof, because a clipped pixel belongs to
 * whatever is behind it.
 */

import { test } from "@playwright/test";
import {
  expect,
  gotoFixture,
  openInline,
  type SBServer,
  settle,
  setWidth,
  startSpace,
  WIDTHS,
} from "./harness.ts";

let server: SBServer;
test.beforeAll(async () => (server = await startSpace()));
test.afterAll(async () => await server?.stop());

/** How far outside the card the probe is pushed. */
const OUT = 28;

test("probe: is a control outside the card clipped?", async ({ page }) => {
  await gotoFixture(page, server);
  const view = await openInline(page);

  const ancestors = await page.evaluate(() => {
    const line = document.querySelector(".cm-line.atomdown-card-line");
    const chain: Record<string, string>[] = [];
    let el: Element | null = line;
    while (el && el !== document.documentElement) {
      const cs = getComputedStyle(el);
      chain.push({
        tag: el.tagName.toLowerCase(),
        cls: (el.className || "").toString().slice(0, 80),
        overflowX: cs.overflowX,
        overflowY: cs.overflowY,
        contain: cs.contain,
        clipPath: cs.clipPath,
      });
      el = el.parentElement;
    }
    return chain;
  });
  console.log("ANCESTOR OVERFLOW CHAIN:\n" + JSON.stringify(ancestors, null, 2));

  for (const width of WIDTHS) {
    await setWidth(page, width);
    await settle(page);

    const result = await page.evaluate((out) => {
      function probe(host: Element, side: "left" | "right") {
        const el = document.createElement("span");
        el.className = "atomdown-clip-probe";
        el.textContent = "X";
        el.setAttribute(
          "style",
          "position:absolute;top:0;width:16px;height:16px;" +
            "background:#f0f;z-index:99;pointer-events:auto;" +
            (side === "left" ? `left:-${out}px;` : `right:-${out}px;`),
        );
        (host as HTMLElement).appendChild(el);
        const r = el.getBoundingClientRect();
        const cx = r.left + r.width / 2;
        const cy = r.top + r.height / 2;
        const hit = document.elementFromPoint(cx, cy);
        const painted = hit === el || (hit != null && el.contains(hit));
        el.remove();
        return {
          side,
          rect: { l: r.left, r: r.right, t: r.top, w: r.width },
          hit: hit
            ? hit.tagName.toLowerCase() +
              "." + (hit.className || "").toString().split(" ")[0]
            : null,
          painted,
        };
      }

      // A top-level card line, and a card line inside a group.
      const top = document.querySelector(
        ".cm-line.atomdown-card-line:not(.atomdown-group-line)",
      );
      const nested = document.querySelector(
        ".cm-line.atomdown-card-line.atomdown-group-line",
      );
      const content = document.querySelector(".cm-content");
      const scroller = document.querySelector(".cm-scroller");
      const cr = content!.getBoundingClientRect();
      const sr = scroller!.getBoundingClientRect();

      function measure(host: Element | null) {
        if (!host) return null;
        const hr = host.getBoundingClientRect();
        return {
          host: { l: hr.left, r: hr.right, w: hr.width },
          left: probe(host, "left"),
          right: probe(host, "right"),
        };
      }

      return {
        content: { l: cr.left, r: cr.right, w: cr.width },
        scroller: { l: sr.left, r: sr.right, w: sr.width },
        viewport: { w: innerWidth },
        top: measure(top),
        nested: measure(nested),
      };
    }, OUT);

    console.log(
      `WIDTH=${width}\n` + JSON.stringify(result, null, 2),
    );
  }

  await view.close();
  expect(true).toBe(true);
});
