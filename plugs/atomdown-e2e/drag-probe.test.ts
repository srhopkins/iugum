/**
 * SCAFFOLDING, NOT A RULE. `--project=probe` only.
 *
 * Prints the `editor:decorationDrag` payload a REAL grip drag produces, plus
 * the document order before and after, so the diagnosis for
 * "inline drag drops at the document end when released in the gutter"
 * (iugum-uuv) is measured rather than assumed.
 */

import { test } from "@playwright/test";
import {
  gotoFixture,
  mod,
  openInline,
  readPageBytes,
  type SBServer,
  settle,
  setWidth,
  startSpace,
} from "./harness.ts";

let server: SBServer;
test.beforeAll(async () => (server = await startSpace()));
test.afterAll(async () => await server?.stop());

function atomOrder(text: string): string[] {
  return [...text.matchAll(/<atom id="([0-9A-Z]+)"/g)].map((m) => m[1]);
}

test("probe: what a gutter release reports", async ({ page }) => {
  await gotoFixture(page, server);
  await setWidth(page, "comfort");
  const view = await openInline(page);

  await page.evaluate(() => {
    const c = (globalThis as any).client;
    (globalThis as any).__drags = [];
    const real = c.dispatchAppEvent.bind(c);
    c.dispatchAppEvent = (name: string, ...rest: any[]) => {
      if (name === "editor:decorationDrag") {
        (globalThis as any).__drags.push(rest[0]);
      }
      return real(name, ...rest);
    };
  });

  const before = await readPageBytes(server);
  const first = atomOrder(before).slice(0, 6);
  console.log("ORDER   ", first.join(" "));

  // Every top-level card header, with its own rect, so a release point can be
  // aimed at a gap, a body, or a header.
  const rects = await page.evaluate(() => {
    return Array.from(
      document.querySelectorAll(
        ".atomdown-card-header:not(.atomdown-nested), .atomdown-group-header",
      ),
    ).slice(0, 6).map((el) => {
      const r = el.getBoundingClientRect();
      return {
        cls: el.className,
        top: +r.top.toFixed(1),
        bottom: +r.bottom.toFixed(1),
        left: +r.left.toFixed(1),
      };
    });
  });
  console.log("HEADERS ", JSON.stringify(rects));

  const grip = await (async () => {
    const h = page.locator(".atomdown-card-header:not(.atomdown-nested)")
      .first();
    await h.hover();
    await settle(page);
    return await h.locator(".atomdown-grip").first().boundingBox();
  })();
  console.log("GRIP    ", JSON.stringify(grip));

  const startX = grip!.x + grip!.width / 2;
  const startY = grip!.y + grip!.height / 2;

  // Walk down the gutter in 10px steps and report what each release reports.
  for (let dy = 20; dy <= 220; dy += 20) {
    await page.evaluate(() => ((globalThis as any).__drags = []));
    const targetY = startY + dy;
    await page.mouse.move(startX, startY);
    await page.mouse.down();
    for (let i = 1; i <= 6; i++) {
      await page.mouse.move(startX, startY + (dy * i) / 6, { steps: 2 });
    }
    await page.mouse.up();
    await settle(page, 4);
    await page.waitForTimeout(1200);
    const p = (await page.evaluate(() => (globalThis as any).__drags))[0];
    const after = await readPageBytes(server);
    const b = atomOrder(after);
    console.log(
      `dy=${dy} y=${targetY.toFixed(0)}`,
      "targetMarks=" + JSON.stringify(p?.targetMarks ?? null),
      "placement=" + p?.placement,
      "line=" + p?.targetLine,
      "| order:",
      b.slice(0, 6).join(" "),
      "| firstAtIndex=" + b.indexOf(first[0]),
      "| changed=" + (after !== before),
    );
    if (after !== before) {
      await page.keyboard.press(`${mod}+z`);
      await settle(page, 6);
      await page.waitForTimeout(1500);
      const undone = await readPageBytes(server);
      console.log("   undo restored:", undone === before);
      if (undone !== before) break;
    }
  }
  await view.close();
});
