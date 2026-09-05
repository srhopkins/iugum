/**
 * SCAFFOLDING, NOT A RULE. `--project=probe` only.
 *
 * The second question `iugum-caj` turns on: can a plug-drawn popover inside a
 * seam widget hold a WORKING text input?
 *
 * Why it is in doubt. A seam widget's HTML is set with `innerHTML`, so it
 * carries no script — that is fine, because a click inside a widget comes back
 * to the plug as `editor:decorationClick` and the plug can read which class was
 * hit. An input is different. Two things in the seam stand between a click and
 * a focused input:
 *
 *   - `widgetPressGuard` returns true for a plain press inside any widget,
 *     which makes CodeMirror call `preventDefault` on `mousedown`. A
 *     prevented `mousedown` does not move focus.
 *   - `DecorationWidget.ignoreEvent` lets `mousedown` and `click` reach the
 *     editor and ignores every other event, `keydown` included.
 *
 * So this measures the real thing: put an input inside a real card header
 * widget, click it, and read `document.activeElement`; then type and read the
 * value back. A design decision rests on the answer, so it is measured rather
 * than argued.
 */

import { test } from "@playwright/test";
import {
  expect,
  gotoFixture,
  openInline,
  type SBServer,
  settle,
  startSpace,
} from "./harness.ts";

let server: SBServer;
test.beforeAll(async () => (server = await startSpace()));
test.afterAll(async () => await server?.stop());

test("probe: can an input inside a seam widget take focus and text?", async ({
  page,
}) => {
  await gotoFixture(page, server);
  const view = await openInline(page);

  // Put a probe input inside a real card header widget, positioned where a
  // popover would be so the click lands on it and on nothing else.
  const box = await page.evaluate(() => {
    const host = document.querySelector(".sb-decoration-widget.atomdown-card-header");
    if (!host) return null;
    const wrap = document.createElement("span");
    wrap.setAttribute(
      "style",
      "position:absolute;left:40px;top:0;z-index:200;background:#fff;" +
        "border:1px solid #000;padding:4px;",
    );
    const input = document.createElement("input");
    input.id = "atomdown-input-probe";
    input.type = "text";
    input.setAttribute("style", "width:120px;");
    wrap.appendChild(input);
    (host as HTMLElement).appendChild(wrap);
    const r = input.getBoundingClientRect();
    return { x: r.left + r.width / 2, y: r.top + r.height / 2 };
  });
  expect(box).not.toBeNull();

  // 1. A plain click, the way a reader would.
  await page.mouse.click(box!.x, box!.y);
  await settle(page);
  const afterClick = await page.evaluate(() => ({
    active: document.activeElement?.id ?? document.activeElement?.tagName ?? null,
    activeClass: (document.activeElement?.className || "").toString().slice(0, 60),
  }));
  console.log("AFTER PLAIN CLICK: " + JSON.stringify(afterClick));

  // 2. Typing after that click — did the characters land in the input, or in
  //    the document?
  await page.keyboard.type("hello");
  await settle(page);
  const afterType = await page.evaluate(() => {
    const el = document.getElementById("atomdown-input-probe") as HTMLInputElement;
    return {
      value: el?.value ?? null,
      active: document.activeElement?.id ?? document.activeElement?.tagName ?? null,
    };
  });
  console.log("AFTER TYPING: " + JSON.stringify(afterType));

  // 3. Programmatic focus, to separate "cannot be focused at all" from "a
  //    click cannot focus it".
  const afterFocus = await page.evaluate(() => {
    const el = document.getElementById("atomdown-input-probe") as HTMLInputElement;
    el.focus();
    return {
      active: document.activeElement?.id ?? document.activeElement?.tagName ?? null,
    };
  });
  console.log("AFTER PROGRAMMATIC FOCUS: " + JSON.stringify(afterFocus));
  await page.keyboard.type("xy");
  const afterFocusType = await page.evaluate(() => {
    const el = document.getElementById("atomdown-input-probe") as HTMLInputElement;
    return { value: el?.value ?? null };
  });
  console.log("AFTER FOCUS+TYPE: " + JSON.stringify(afterFocusType));

  // 4. Did any of this change the document? The whole feature is presentational.
  const docChanged = await page.evaluate(
    () => (globalThis as any).client?.editorView?.state?.doc?.toString()?.includes("hello"),
  );
  console.log("DOC CONTAINS TYPED TEXT: " + JSON.stringify(docChanged));

  await page.evaluate(() => {
    document.getElementById("atomdown-input-probe")?.parentElement?.remove();
  });
  await view.close();
  expect(true).toBe(true);
});
