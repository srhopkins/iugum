/**
 * Not a rule. A scaffolding check that prints what the two views actually
 * render, so the six rules are written against the real DOM rather than
 * against a README. Run with `--project=probe`; the gate never runs it.
 */

import { test } from "@playwright/test";
import {
  containmentViolations,
  gotoFixture,
  measureBoxes,
  openBoard,
  openInline,
  type SBServer,
  startSpace,
  sweepBoxes,
} from "./harness.ts";

let server: SBServer;

test.beforeAll(async () => {
  server = await startSpace();
});
test.afterAll(async () => await server?.stop());

const SPECS = {
  inlineCard: {
    name: "inline card",
    runPrefix: "atomdown-card",
    headerSelector: ".atomdown-card-header",
  },
  inlineGroup: {
    name: "inline group",
    runPrefix: "atomdown-group",
    headerSelector: ".atomdown-group-header",
  },
  inlineCardHeader: { name: "inline card header", selector: ".atomdown-card-header" },
  inlineGroupHeader: { name: "inline group header", selector: ".atomdown-group-header" },
};

test("probe: inline", async ({ page }) => {
  page.on("console", (m) => {
    if (m.type() === "error") console.log("  [browser error]", m.text());
  });
  await gotoFixture(page, server);
  const view = await openInline(page);

  for (const [name, spec] of Object.entries(SPECS)) {
    const sw = await sweepBoxes(view, spec as any);
    const v = containmentViolations(sw.boxes);
    console.log(
      `${name}: ${sw.boxes.length} boxes over ${sw.stops} stops, ${sw.ids.length} ids, ${v.length} violations`,
    );
    // Group violations by child label so the shape is visible, not a wall.
    const byKind = new Map<string, number>();
    for (const x of v) {
      const k = `${x.child.split(' "')[0]} ${x.side}`;
      byKind.set(k, (byKind.get(k) ?? 0) + 1);
    }
    console.log("   ", JSON.stringify([...byKind.entries()].slice(0, 12)));
    if (v.length) console.log("    worst:", JSON.stringify(v.sort((a, b) => b.overflowPx - a.overflowPx)[0], null, 2));
  }

  // A card that lives inside a group: is its drawn edge inset from the line?
  const grouped = await page.evaluate(() => {
    const line = document.querySelector(
      ".atomdown-card-line.atomdown-group-line",
    );
    if (!line) return "no grouped card line found";
    const cs = getComputedStyle(line);
    const bf = getComputedStyle(line, "::before");
    const r = line.getBoundingClientRect();
    return {
      cls: String(line.className),
      rect: { l: +r.left.toFixed(1), r: +r.right.toFixed(1) },
      groupPadding: cs.getPropertyValue("--board-group-padding"),
      ownBorderLR: `${cs.borderLeftWidth}/${cs.borderRightWidth}`,
      beforeBorderLR: `${bf.borderLeftWidth}/${bf.borderRightWidth}`,
      beforeInset: `${bf.left}/${bf.right}`,
      beforeContent: bf.content,
      padding: `${cs.paddingLeft}/${cs.paddingRight}`,
    };
  });
  console.log("grouped card line:", JSON.stringify(grouped, null, 2));

  // Where does the reveal happen, and how big is a directive at rest?
  const peek = await page.evaluate(() => {
    const ds = Array.from(document.querySelectorAll(".atomdown-directive"));
    const heights = ds.map((d) => d.getBoundingClientRect().height);
    const peeks = Array.from(
      document.querySelectorAll(".atomdown-directive-peek"),
    ).map((p) => {
      const r = p.getBoundingClientRect();
      const cs = getComputedStyle(p);
      return {
        h: +r.height.toFixed(2),
        w: +r.width.toFixed(2),
        display: cs.display,
        fontSize: cs.fontSize,
        color: cs.color,
      };
    });
    return {
      directives: ds.length,
      maxHeight: Math.max(...heights, 0),
      peeks: peeks.length,
      peekSample: peeks.slice(0, 2),
    };
  });
  console.log("directives at rest:", JSON.stringify(peek, null, 2));

  await view.close();
});

test("probe: board", async ({ page }) => {
  page.on("console", (m) => {
    if (m.type() === "error") console.log("  [browser error]", m.text());
  });
  await gotoFixture(page, server);
  const view = await openBoard(page);

  const info = await view.ev.evaluate(() => {
    const n = (s: string) => document.querySelectorAll(s).length;
    return {
      cards: n(".board-card"),
      groups: n(".board-group"),
      tables: n("table"),
      tr: n("tr"),
      ol: n("ol"),
      li: n("li"),
      links: n("a[href]"),
      density: document.querySelector("#atomdown-board-density")?.getAttribute("data-board-density"),
      viewMode: document.querySelector("#atomdown-board-view")?.getAttribute("data-board-view"),
      rootDensity: document.querySelector("#atomdown-board-root")?.getAttribute("data-density"),
      bodyOverflow: (() => {
        const b = document.querySelector(".board-card-body");
        if (!b) return null;
        const cs = getComputedStyle(b);
        return { x: cs.overflowX, y: cs.overflowY };
      })(),
      tableWrapOverflow: (() => {
        const t = document.querySelector("table");
        if (!t) return null;
        let el = t.parentElement;
        const chain: string[] = [];
        for (let i = 0; el && i < 4; i++) {
          const cs = getComputedStyle(el);
          chain.push(`${el.tagName.toLowerCase()}.${String(el.className)} ox=${cs.overflowX}`);
          el = el.parentElement;
        }
        return chain;
      })(),
      hasSha: document.body.innerHTML.includes("sha256:"),
      hasAtomComment: document.body.innerHTML.includes("<!-- <atom"),
      hasRawLink: document.body.innerHTML.includes("](http"),
      scroller: (() => {
        const el = document.querySelector(".board-cards");
        return el ? { sh: el.scrollHeight, ch: el.clientHeight } : null;
      })(),
    };
  });
  console.log("BOARD:", JSON.stringify(info, null, 2));

  for (const [name, sel] of Object.entries({
    card: ".board-card",
    group: ".board-group",
    cardHeader: ".board-card-header",
    groupHeader: ".board-group-header",
  })) {
    const sw = await sweepBoxes(view, { name, selector: sel });
    const v = containmentViolations(sw.boxes);
    console.log(`board ${name}: ${sw.boxes.length} boxes, ${sw.stops} stops, ${v.length} violations`);
    const byKind = new Map<string, number>();
    for (const x of v) {
      const k = `${x.child.split(' "')[0]} ${x.side}`;
      byKind.set(k, (byKind.get(k) ?? 0) + 1);
    }
    console.log("   ", JSON.stringify([...byKind.entries()].slice(0, 12)));
    if (v.length) console.log("    worst:", JSON.stringify(v.sort((a, b) => b.overflowPx - a.overflowPx)[0], null, 2));
  }

  await view.close();
});
