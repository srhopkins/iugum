/**
 * SCAFFOLDING, NOT A RULE. `--project=probe` only.
 * Prints what the card and group popovers actually do to a click.
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

async function state(page: any) {
  return await page.evaluate(() => ({
    popovers: document.querySelectorAll(".atomdown-menu-popover").length,
    rows: document.querySelectorAll(".atomdown-menu-item").length,
    labels: Array.from(
      document.querySelectorAll(".atomdown-menu-label-text"),
    ).map((e: any) => e.textContent),
    items: Array.from(document.querySelectorAll(".atomdown-menu-item")).map(
      (e: any) => e.textContent,
    ),
    filterBox: document.querySelectorAll(
      ".sb-modal-box, .sb-filter-box, [class*=filter]",
    ).length,
    inputs: document.querySelectorAll(".atomdown-menu-popover input").length,
  }));
}

test("probe: the card popover", async ({ page }) => {
  await gotoFixture(page, server);
  const view = await openInline(page);

  const menu = page
    .locator(".sb-decoration-widget.atomdown-card-header .atomdown-card-menu")
    .first();
  await menu.scrollIntoViewIfNeeded();
  await settle(page);
  console.log("BEFORE: " + JSON.stringify(await state(page)));

  await menu.click({ force: true });
  await settle(page);
  console.log("AFTER OPEN: " + JSON.stringify(await state(page)));

  // The label: must keep it open.
  const label = page.locator(".atomdown-menu-label-text").first();
  if (await label.count()) {
    await label.click({ force: true });
    await settle(page);
    console.log("AFTER LABEL CLICK: " + JSON.stringify(await state(page)));
  }

  // A second press on the button: must close it.
  await menu.click({ force: true });
  await settle(page);
  console.log("AFTER TOGGLE OFF: " + JSON.stringify(await state(page)));

  // The group control.
  const gmenu = page.locator(".atomdown-group-menu").first();
  await gmenu.scrollIntoViewIfNeeded();
  await settle(page);
  await gmenu.click({ force: true });
  await settle(page);
  console.log("AFTER GROUP OPEN: " + JSON.stringify(await state(page)));

  await view.close();
  expect(true).toBe(true);
});
