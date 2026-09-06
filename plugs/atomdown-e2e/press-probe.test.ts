/**
 * PRESS PROBE — who calls `preventDefault` on a plain mousedown, and where.
 *
 * Scaffolding for `iugum-oip`, not a rule. Rule 10h asserts that a press on a
 * card body reaches the selection handler; this prints what the browser
 * actually reports for a press on a card body, on an undecorated line, and
 * with the inline view off, so 10h asserts the seam's guard rather than
 * CodeMirror's own normal behaviour.
 */

import { test } from "@playwright/test";
import {
  gotoFixture,
  openInline,
  type Page,
  type SBServer,
  settle,
  startSpace,
} from "./harness.ts";

let server: SBServer;
test.beforeAll(async () => (server = await startSpace()));
test.afterAll(async () => await server?.stop());

async function press(page: Page, x: number, y: number) {
  await page.evaluate(() => {
    (globalThis as any).__p = [];
    if (!(globalThis as any).__pOn) {
      (globalThis as any).__pOn = true;
      globalThis.addEventListener("mousedown", (e: MouseEvent) => {
        (globalThis as any).__p.push({
          phase: "bubble-window",
          prevented: e.defaultPrevented,
          target: (e.target as Element)?.className ?? "",
        });
      }, false);
      globalThis.addEventListener("mousedown", (e: MouseEvent) => {
        (globalThis as any).__p.push({
          phase: "capture-window",
          prevented: e.defaultPrevented,
          target: (e.target as Element)?.className ?? "",
        });
      }, true);
    }
  });
  await page.mouse.move(4, 4);
  await page.mouse.move(x, y);
  await page.mouse.down();
  await page.mouse.up();
  await settle(page, 3);
  return await page.evaluate(() => (globalThis as any).__p);
}

test("press probe: card body, undecorated line, and the view off", async ({
  page,
}) => {
  await gotoFixture(page, server);

  // The view OFF first: CodeMirror's own baseline.
  const plain = await page.evaluate(() => {
    const el = Array.from(document.querySelectorAll(".cm-line"))
      .find((l) => (l.textContent ?? "").trim() !== "")!;
    const r = el.getBoundingClientRect();
    return { x: +(r.left + 20).toFixed(1), y: +(r.top + r.height / 2).toFixed(1) };
  });
  console.log("VIEW OFF, text line:", JSON.stringify(await press(page, plain.x, plain.y)));

  const view = await openInline(page);

  const points = await page.evaluate(() => {
    const cardFirst = document.querySelector(
      ".cm-line.atomdown-card-line.atomdown-card-first:not(.atomdown-group-line)",
    ) as HTMLElement;
    const blank = Array.from(document.querySelectorAll(".cm-line"))
      .find((l) =>
        (l.textContent ?? "").trim() === "" &&
        !l.classList.contains("atomdown-card-line") &&
        l.getBoundingClientRect().height > 4
      ) as HTMLElement | undefined;
    const box = (el: HTMLElement) => {
      const r = el.getBoundingClientRect();
      return {
        x: +(r.left + 20).toFixed(1),
        y: +(r.top + r.height / 2).toFixed(1),
        cls: el.className,
      };
    };
    return {
      card: box(cardFirst),
      blank: blank ? box(blank) : null,
    };
  });

  console.log("VIEW ON, card body:", JSON.stringify(points.card), JSON.stringify(await press(page, points.card.x, points.card.y)));
  if (points.blank) {
    console.log("VIEW ON, undecorated line:", JSON.stringify(points.blank), JSON.stringify(await press(page, points.blank.x, points.blank.y)));
  }

  // And what the plug thinks is selected after that card press.
  console.log("selected lines:", await page.evaluate(() =>
    document.querySelectorAll(".atomdown-selected-line").length
  ));

  // --- THE LASSO ---------------------------------------------------------
  //
  // Does the band element ever appear, and does the app event carry any unit
  // marks? Both are recorded, because "no band" and "a band that swept
  // nothing" are different defects.
  await page.evaluate(() => {
    (globalThis as any).__lasso = [];
    const client = (globalThis as any).client;
    const orig = client.dispatchAppEvent.bind(client);
    client.dispatchAppEvent = function (name: string, ...rest: any[]) {
      if (String(name).indexOf("decoration") === 0 || String(name).indexOf("editor:decoration") === 0) {
        (globalThis as any).__lasso.push({ name, payload: rest[0] });
      }
      return orig(name, ...rest);
    };
    (globalThis as any).__bandSeen = 0;
    const obs = new MutationObserver(() => {
      if (document.querySelector(".sb-decoration-lasso")) {
        (globalThis as any).__bandSeen++;
      }
    });
    obs.observe(document.body, { childList: true, subtree: true });
  });

  const two = await page.evaluate(() => {
    const firsts = Array.from(document.querySelectorAll(
      ".cm-line.atomdown-card-line.atomdown-card-first:not(.atomdown-group-line)",
    )) as HTMLElement[];
    const on = firsts.filter((el) => {
      const r = el.getBoundingClientRect();
      return r.top > 120 && r.bottom < globalThis.innerHeight - 80;
    }).slice(0, 2).map((el) => {
      const r = el.getBoundingClientRect();
      return { x: +(r.left + 20).toFixed(1), y: +(r.top + r.height / 2).toFixed(1) };
    });
    return on;
  });
  console.log("band from/to:", JSON.stringify(two));

  await page.keyboard.down("Alt");
  await page.mouse.move(two[0].x, two[0].y);
  await page.mouse.down();
  for (let i = 1; i <= 6; i++) {
    await page.mouse.move(
      two[0].x + ((two[1].x + 60 - two[0].x) * i) / 6,
      two[0].y + ((two[1].y - two[0].y) * i) / 6,
      { steps: 2 },
    );
  }
  await page.mouse.up();
  await page.keyboard.up("Alt");
  await settle(page, 4);

  console.log("band element seen:", await page.evaluate(() => (globalThis as any).__bandSeen));
  console.log(
    "decoration events:",
    JSON.stringify(await page.evaluate(() => (globalThis as any).__lasso), null, 1)
      .slice(0, 3000),
  );
  console.log("selected lines after lasso:", await page.evaluate(() =>
    document.querySelectorAll(".atomdown-selected-line").length
  ));

  // --- SHIFT-CLICK -------------------------------------------------------
  // Scroll until three units are on screen, the way rule 10 does.
  for (let i = 0; i < 12; i++) {
    const n = await page.evaluate(() =>
      Array.from(document.querySelectorAll(
        ".cm-line.atomdown-card-line.atomdown-card-first," +
          ".cm-line.atomdown-group-line.atomdown-group-first",
      )).filter((el) => {
        const group = el.classList.contains("atomdown-group-first");
        if (!group && el.classList.contains("atomdown-group-line")) return false;
        const r = el.getBoundingClientRect();
        return r.top > 110 && r.top < globalThis.innerHeight - 120;
      }).length
    );
    if (n >= 3) break;
    await page.evaluate(() => {
      const el = document.querySelector(".cm-scroller")!;
      el.scrollTop += Math.round(el.clientHeight * 0.6);
    });
    await settle(page, 4);
  }

  const units = await page.evaluate(() => {
    const rows = Array.from(document.querySelectorAll(
      ".cm-line.atomdown-card-line.atomdown-card-first," +
        ".cm-line.atomdown-group-line.atomdown-group-first",
    )) as HTMLElement[];
    const out: any[] = [];
    for (const el of rows) {
      const group = el.classList.contains("atomdown-group-first");
      if (!group && el.classList.contains("atomdown-group-line")) continue;
      let point: HTMLElement | null = el;
      if (group || (el.textContent ?? "").trim() === "") {
        point = null;
        let next: Element | null = el;
        for (let i = 0; i < 12 && next; i++) {
          next = next.nextElementSibling;
          if (!(next instanceof HTMLElement)) continue;
          if (!next.classList.contains("cm-line")) continue;
          if ((next.textContent ?? "").trim() === "") continue;
          point = next;
          break;
        }
      }
      if (!point) continue;
      const r = point.getBoundingClientRect();
      if (r.top < 110 || r.bottom > globalThis.innerHeight - 24) continue;
      out.push({
        text: (point.textContent ?? "").trim().slice(0, 40),
        kind: group ? "group" : "card",
        x: +(r.left + 20).toFixed(1),
        y: +(r.top + r.height / 2).toFixed(1),
      });
    }
    return out;
  });
  console.log("units on screen:", JSON.stringify(units));

  await page.evaluate(() => ((globalThis as any).__lasso = []));
  await page.mouse.click(units[0].x, units[0].y);
  await settle(page, 6);
  await page.waitForTimeout(1200);
  const again = await page.evaluate(() => {
    const rows = Array.from(document.querySelectorAll(
      ".cm-line.atomdown-card-line.atomdown-card-first," +
        ".cm-line.atomdown-group-line.atomdown-group-first",
    )) as HTMLElement[];
    const out: any[] = [];
    for (const el of rows) {
      const group = el.classList.contains("atomdown-group-first");
      if (!group && el.classList.contains("atomdown-group-line")) continue;
      let point: HTMLElement | null = el;
      if (group || (el.textContent ?? "").trim() === "") {
        point = null;
        let next: Element | null = el;
        for (let i = 0; i < 12 && next; i++) {
          next = next.nextElementSibling;
          if (!(next instanceof HTMLElement)) continue;
          if (!next.classList.contains("cm-line")) continue;
          if ((next.textContent ?? "").trim() === "") continue;
          point = next;
          break;
        }
      }
      if (!point) continue;
      const r = point.getBoundingClientRect();
      if (r.top < 110 || r.bottom > globalThis.innerHeight - 24) continue;
      out.push({
        text: (point.textContent ?? "").trim().slice(0, 40),
        kind: group ? "group" : "card",
        x: +(r.left + 20).toFixed(1),
        y: +(r.top + r.height / 2).toFixed(1),
      });
    }
    return out;
  });
  console.log("units after first click:", JSON.stringify(again));
  const head = again[again.length - 1];
  console.log("shift target:", JSON.stringify(head));
  console.log("what is at that point:", await page.evaluate((p) => {
    const el = document.elementFromPoint(p.x, p.y) as HTMLElement | null;
    const line = el?.closest(".cm-line") as HTMLElement | null;
    return {
      el: el?.className ?? null,
      line: line?.className ?? null,
      text: (line?.textContent ?? "").trim().slice(0, 40),
    };
  }, head));
  await page.keyboard.down("Shift");
  await page.mouse.click(head.x, head.y);
  await page.keyboard.up("Shift");
  await settle(page, 4);
  await page.waitForTimeout(1200);
  console.log(
    "click events:",
    JSON.stringify(
      (await page.evaluate(() => (globalThis as any).__lasso))
        .filter((e: any) => e.name === "editor:decorationClick")
        .map((e: any) => ({
          marks: e.payload.marks,
          shiftKey: e.payload.shiftKey,
          metaKey: e.payload.metaKey,
          line: e.payload.line,
        })),
    ),
  );
  console.log("selected runs after shift:", await page.evaluate(() => {
    const lines = Array.from(document.querySelectorAll(".cm-line"));
    const runs: string[] = [];
    let open = false;
    for (const l of lines) {
      const on = l.classList.contains("atomdown-selected-line");
      if (on && !open) runs.push((l.textContent ?? "").trim().slice(0, 40));
      open = on;
    }
    return runs;
  }));
  await view.close();
});
