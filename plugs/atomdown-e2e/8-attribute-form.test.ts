/**
 * RULE 8j / 8k / 8l — THE ATTRIBUTE FORM (iugum-etz).
 *
 * Steve: "the menu used to show attributes like a form, now its not there, and
 * if you click edit attributes you just see the raw atomdown in the card."
 *
 * WHY THIS IS A SEPARATE FILE. It WRITES, and 8f in `8-card-controls.test.ts`
 * asserts that nothing in that file changes a document byte. Same split, and
 * the same reason, as 8i's drag file. Each test here boots its own space.
 *
 * 8j IS THE ONE THAT MATTERS, and it is the defect this feature exists to
 * avoid rather than the feature itself. The decoration seam's
 * `widgetPressGuard` calls `preventDefault` on a plain `mousedown` inside any
 * widget, so an input in the card's popover takes no focus from a click and
 * the characters typed next go INTO THE DOCUMENT - measured, in
 * `input-probe.test.ts`. Rule 8g still asserts the popover holds no input.
 * The form therefore lives in a SilverBullet panel, which is an iframe outside
 * the editor's DOM, and 8j proves that choice with the two assertions the bead
 * names: while typing, `activeElement` is the form's own field, and the
 * document's bytes are UNCHANGED until Save.
 *
 * THE KEYSTROKES ARE REAL. `locator.fill()` sets a value through the DOM and
 * would pass on the broken arrangement too, because the broken arrangement's
 * failure is where the KEYS go. So every value here is typed with
 * `page.keyboard.type` after a real click.
 */

import { test } from "@playwright/test";
import {
  BOARD_FRAME_SELECTOR,
  type Combo,
  expect,
  failWithArtifacts,
  gotoFixture,
  mod,
  openInline,
  readPageBytes,
  type SBServer,
  settle,
  startSpace,
  type View,
} from "./harness.ts";

/** The card whose attributes the form is opened for, and its popover. */
async function openAttrForm(view: View) {
  const menu = view.page
    .locator(".sb-decoration-widget.atomdown-card-header .atomdown-card-menu")
    .first();
  await menu.scrollIntoViewIfNeeded();
  await settle(view.page);
  await menu.click({ force: true });
  await view.page
    .locator(".atomdown-menu-popover")
    .first()
    .waitFor({ state: "visible", timeout: 15_000 });

  const row = view.page.locator(".atomdown-menu-item.atomdown-mi-attrs").first();
  await expect(
    row,
    "the card's menu offers Edit attributes as its own row",
  ).toHaveCount(1);
  await row.click({ force: true });

  // The panel iframe, waited for rather than slept on.
  await view.page
    .locator(BOARD_FRAME_SELECTOR)
    .waitFor({ state: "visible", timeout: 15_000 });
  const frame = view.page.frameLocator(BOARD_FRAME_SELECTOR);
  await frame.locator("#ad-attr-slug-input").waitFor({
    state: "visible",
    timeout: 15_000,
  });
  await settle(view.page);
  return frame;
}

/** Every `id=` and `digest=` in the document, as one comparable string. */
const idsAndDigests = (s: string) =>
  (s.match(/(id|digest)="[^"]*"/g) ?? []).join("\n");

// ---------------------------------------------------------------------------
// 8j — focus, and the document untouched until Save
// ---------------------------------------------------------------------------

test("8j: typing in the attribute form keeps focus in the form and writes no document byte until Save", async ({
  page,
}) => {
  const own = await startSpace();
  try {
    const original = await readPageBytes(own);
    await gotoFixture(page, own);
    const view = await openInline(page);
    const frame = await openAttrForm(view);

    // THE FORM TAKES FOCUS ON OPEN. In the popover a click focused nothing.
    const slug = frame.locator("#ad-attr-slug-input");
    expect(
      await slug.evaluate((el) => el.ownerDocument.activeElement === el),
      "the form's own field has focus the moment the form opens",
    ).toBe(true);

    // ...and the EDITOR does not. This is the other half of the same fact:
    // the keystrokes reach the panel, so `.cm-content` must not be the
    // active element in the top document.
    const topActive = await page.evaluate(() => {
      const a = document.activeElement;
      return {
        tag: a ? a.tagName.toLowerCase() : null,
        isEditorContent: !!a && a.classList.contains("cm-content"),
      };
    });
    expect(
      topActive.isEditorContent,
      "the editor's content element does NOT hold focus while the form is open",
    ).toBe(false);
    expect(
      topActive.tag,
      "the top document's active element is the panel's iframe",
    ).toBe("iframe");

    // REAL KEYSTROKES, after a real click. `fill()` would pass on the broken
    // arrangement, because the broken arrangement's failure is where the KEYS
    // go, not what the value ends up as.
    await slug.click();
    await page.keyboard.press(`${mod}+a`);
    await page.keyboard.type("renamed-by-attr-form");
    await settle(page);

    expect(
      await slug.evaluate((el) => el.ownerDocument.activeElement === el),
      "focus is still inside the form after typing",
    ).toBe(true);
    expect(
      await slug.inputValue(),
      "the characters landed in the form's field",
    ).toBe("renamed-by-attr-form");

    // THE DEFECT THIS EXISTS TO PREVENT. If the guard had eaten the click,
    // these characters would be in the page.
    await settle(page, 6);
    await page.waitForTimeout(2000);
    const midway = await readPageBytes(own);
    if (midway !== original) {
      await failWithArtifacts(
        page,
        8,
        "typing in the attribute form changed the document",
        { width: "comfort", theme: "light", density: "comfortable" } as Combo,
        {
          originalLength: original.length,
          midwayLength: midway.length,
          leaked: midway.includes("renamed-by-attr-form"),
        },
        `the page's bytes changed while the form was still open. The form is ` +
          `in a panel iframe precisely so a keystroke cannot reach the ` +
          `document; a change here means the field took no focus and the ` +
          `characters went into the page (see input-probe.test.ts).`,
      );
    }
    expect(
      midway.includes("renamed-by-attr-form"),
      "and nothing typed in the form appears in the page before Save",
    ).toBe(false);
  } finally {
    await own.stop();
  }
});

// ---------------------------------------------------------------------------
// 8k — the form's shape, and Save as ONE transaction
// ---------------------------------------------------------------------------

test("8k: the slug comes first and labelled, add and remove work, and Save is one undo step", async ({
  page,
}) => {
  const own = await startSpace();
  try {
    const original = await readPageBytes(own);
    await gotoFixture(page, own);
    const view = await openInline(page);
    const frame = await openAttrForm(view);

    // THE SLUG IS FIRST AND LABELLED, matching the board panel's own form.
    const label = frame.locator(".ad-attr-slug .ad-attr-label");
    await expect(
      label,
      "the slug row carries the board's own label wording",
    ).toHaveText("Name (slug) - readable alias, not the id");
    const order = await frame.locator(".ad-attr-form").evaluate((form) => {
      const slug = form.querySelector("#ad-attr-slug-input")!;
      const list = form.querySelector("#ad-attr-list")!;
      return {
        slugBeforeList: !!(slug.compareDocumentPosition(list) &
          Node.DOCUMENT_POSITION_FOLLOWING),
        // The id is shown and is not editable. Identity, never edited here.
        idRowDisabled: (form.querySelector(".ad-attr-name") as
          HTMLInputElement).disabled,
        idRowName: (form.querySelector(".ad-attr-name") as HTMLInputElement)
          .value,
      };
    });
    expect(order.slugBeforeList, "the slug field precedes the attribute list")
      .toBe(true);
    expect(order.idRowName, "the first generic row is the id").toBe("id");
    expect(order.idRowDisabled, "and it is disabled: an id is not editable")
      .toBe(true);

    const rowsBefore = await frame.locator(".ad-attr-row").count();

    // ADD one attribute, typed.
    await frame.locator("#ad-attr-add").click();
    await expect(frame.locator(".ad-attr-row")).toHaveCount(rowsBefore + 1);
    const newRow = frame.locator(".ad-attr-row").last();
    await newRow.locator(".ad-attr-name").click();
    await page.keyboard.type("acme-owner");
    await newRow.locator(".ad-attr-value").click();
    await page.keyboard.type("steve");

    // ADD a second one and REMOVE it again, so removal is exercised on a row
    // this test owns rather than on one the fixture needs.
    await frame.locator("#ad-attr-add").click();
    await expect(frame.locator(".ad-attr-row")).toHaveCount(rowsBefore + 2);
    const doomed = frame.locator(".ad-attr-row").last();
    await doomed.locator(".ad-attr-name").click();
    await page.keyboard.type("acme-doomed");
    await doomed.locator(".ad-attr-remove").click();
    await expect(
      frame.locator(".ad-attr-row"),
      "the remove button removes its own row",
    ).toHaveCount(rowsBefore + 1);

    await frame.locator("#ad-attr-save").click();
    // The panel closes itself on a successful save.
    await page
      .locator(BOARD_FRAME_SELECTOR)
      .waitFor({ state: "detached", timeout: 15_000 });
    await settle(page, 6);
    await page.waitForTimeout(2000);

    const saved = await readPageBytes(own);
    expect(saved, "Save wrote the new attribute").toContain(
      'acme-owner="steve"',
    );
    expect(
      saved.includes("acme-doomed"),
      "and the removed row was never written",
    ).toBe(false);

    // ONE LINE CHANGED, and it is a directive line.
    const before = original.split("\n");
    const changed = saved.split("\n").filter((l, i) => l !== before[i]);
    expect(changed.length, "exactly one line changed: " + changed.join(" | "))
      .toBe(1);
    expect(changed[0]).toMatch(/<atom /);

    // NO ID AND NO DIGEST CHANGED, which is the "editing an unrelated
    // attribute" half of the bead's DoD.
    const idsBefore = idsAndDigests(original);
    const idsAfter = idsAndDigests(saved);
    if (idsAfter !== idsBefore) {
      await failWithArtifacts(
        page,
        8,
        "the attribute form changed an id or a digest",
        { width: "comfort", theme: "light", density: "comfortable" } as Combo,
        { idsBefore: idsBefore.slice(0, 400), idsAfter: idsAfter.slice(0, 400) },
        `adding one attribute changed an id or a digest attribute. An id is ` +
          `identity and a digest is a claim that someone reviewed that exact ` +
          `content: neither may move as a side effect of an attribute edit.`,
      );
    }

    // ONE UNDO REVERTS THE WHOLE SAVE, because the write is one CodeMirror
    // transaction by design.
    await page.evaluate(() => {
      const el = document.querySelector(".cm-content") as HTMLElement | null;
      el?.focus();
    });
    await page.keyboard.press(`${mod}+z`);
    await settle(page, 6);
    await page.waitForTimeout(2000);
    const undone = await readPageBytes(own);
    if (undone !== original) {
      await failWithArtifacts(
        page,
        8,
        "one undo did not revert the attribute save",
        { width: "comfort", theme: "light", density: "comfortable" } as Combo,
        { originalLength: original.length, undoneLength: undone.length },
        `after one Cmd-Z the file still differs from the original. The save ` +
          `is reduced to the smallest single replacement and applied as ONE ` +
          `transaction precisely so one undo reverts it; more than one ` +
          `transaction is the regression.`,
      );
    }
  } finally {
    await own.stop();
  }
});

// ---------------------------------------------------------------------------
// 8l — the refusals
// ---------------------------------------------------------------------------

test("8l: a pasted directive is refused and the document is left alone", async ({
  page,
}) => {
  const own = await startSpace();
  try {
    const original = await readPageBytes(own);
    await gotoFixture(page, own);
    const view = await openInline(page);
    const frame = await openAttrForm(view);

    await frame.locator("#ad-attr-add").click();
    const row = frame.locator(".ad-attr-row").last();
    await row.locator(".ad-attr-name").click();
    await page.keyboard.type("acme-x");
    await row.locator(".ad-attr-value").click();
    await page.keyboard.type('<!-- <atom id="BBBBBBBB"/> -->');

    await frame.locator("#ad-attr-save").click();
    // A REFUSAL, and it says so IN THE FORM. The panel stays open, because
    // the reader has something to fix.
    await expect(
      frame.locator("#ad-attr-status"),
      "the form refuses a directive in a value and explains why",
    ).toContainText("directive", { timeout: 15_000 });
    await expect(
      page.locator(BOARD_FRAME_SELECTOR),
      "and the form stays open rather than closing on a failed save",
    ).toBeVisible();

    await settle(page, 6);
    await page.waitForTimeout(2000);
    expect(
      await readPageBytes(own),
      "a refused save changes no document byte",
    ).toBe(original);
  } finally {
    await own.stop();
  }
});
