/**
 * RULE 8 — THE CARD'S CONTROLS.
 *
 * One rule per defect Steve found on the live page (`iugum-caj`), each stated
 * as the property that was violated rather than as the fix:
 *
 *   8a. THE CONTROL OPENS ITS OWN POPOVER, NOT THE HOST'S PICKER. The three-dot
 *       control used to call `editor.filterBox`, which is the command palette's
 *       own surface: a centred box with a search field, listing the card's
 *       identity and its actions as palette rows. So this asserts a popover
 *       element exists, anchored to the card, and that NO filter box appeared.
 *   8b. THE POPOVER'S FIRST CHILD IS AN INERT NAME-AND-ID LABEL, and clicking
 *       it keeps the popover open. That is the panel's shape, and the label is
 *       the first thing a reader clicks, because it names the card.
 *   8c. THE GLYPH IS VERTICAL, asserted by CHARACTER (U+22EE) and by class -
 *       never from an image, because a screenshot cannot tell a reader which
 *       character it is looking at and a diff on one is unreadable.
 *   8d. BOTH CONTROLS ARE OUTSIDE THE CARD, asserted BY MEASUREMENT against
 *       the card head's own border rect, at every width and both densities.
 *   8e. BOTH ARE HIDDEN AT REST and shown on hover AND on keyboard focus.
 *   8f. NOTHING HERE WRITES A DOCUMENT BYTE. Hovering, opening a menu,
 *       clicking the label and closing again leave the page identical.
 *
 * WHY THERE IS NO TEXT INPUT IN THE POPOVER, and why that is asserted rather
 * than merely absent. The seam's `widgetPressGuard` calls `preventDefault` on
 * a plain `mousedown` inside any widget, so an input in there cannot take
 * focus from a click - and the keystrokes that follow go INTO THE DOCUMENT.
 * Measured, in `input-probe.test.ts`. An attribute form inside this popover
 * would therefore type the reader's attribute values into the page, which is
 * the one thing this view may never do. 8g asserts the popover holds no input,
 * so the day someone adds one, this rule says why they cannot.
 *
 * THE GROUP CONTROL IS THE SAME CONTROL. Its hover state is checked here for
 * geometry and by rule 9 for colour: no behavioural assertion can see that a
 * hover paints a white hole in a saturated blue bar, which is exactly what
 * Steve saw, so that one is a visual baseline.
 */

import { test } from "@playwright/test";
import {
  type Combo,
  combos,
  type Locator,
  comboName,
  DENSITIES,
  expect,
  failWithArtifacts,
  gotoFixture,
  openInline,
  readPageBytes,
  type SBServer,
  setDensity,
  settle,
  setWidth,
  startSpace,
  type View,
  WIDTHS,
} from "./harness.ts";

let server: SBServer;
test.beforeAll(async () => (server = await startSpace()));
test.afterAll(async () => await server?.stop());

/** U+22EE VERTICAL ELLIPSIS, the character the board panel's button carries. */
const VERTICAL_ELLIPSIS = "⋮";
/** U+22EF MIDLINE HORIZONTAL ELLIPSIS: what the inline view used to carry. */
const HORIZONTAL_ELLIPSIS = "⋯";

/** Every surface the host's own picker could appear on. */
const PICKER_SELECTOR = [
  ".sb-modal-box",
  ".sb-filter-box",
  ".sb-mini-editor",
  "[class*='filter-box']",
].join(",");

async function pickerCount(view: View): Promise<number> {
  return await view.ev.evaluate(
    (sel) => document.querySelectorAll(sel).length,
    PICKER_SELECTOR,
  );
}

type PopoverState = {
  popovers: number;
  rows: string[];
  labelText: string | null;
  labelIsFirstChild: boolean;
  inputs: number;
  glyph: string | null;
};

async function popoverState(view: View): Promise<PopoverState> {
  return await view.ev.evaluate(() => {
    const pop = document.querySelector(".atomdown-menu-popover");
    const label = pop?.querySelector(".atomdown-menu-label");
    const btn = document.querySelector(
      ".atomdown-menu-open .atomdown-card-menu, .atomdown-menu-open .atomdown-group-menu",
    );
    return {
      popovers: document.querySelectorAll(".atomdown-menu-popover").length,
      rows: Array.from(document.querySelectorAll(".atomdown-menu-item")).map(
        (e) => (e.textContent ?? "").trim(),
      ),
      labelText: (label?.textContent ?? null) as string | null,
      labelIsFirstChild: !!label && pop!.firstElementChild === label,
      inputs: pop
        ? pop.querySelectorAll("input,textarea,select,[contenteditable]").length
        : 0,
      glyph: btn?.textContent ?? null,
    };
  });
}

/** The first card header with a control on screen, scrolled into view. */
async function firstCardMenu(view: View) {
  const menu = view.page
    .locator(".sb-decoration-widget.atomdown-card-header .atomdown-card-menu")
    .first();
  await menu.scrollIntoViewIfNeeded();
  await settle(view.page);
  return menu;
}

/**
 * Press a control and WAIT FOR THE POPOVER, not for three frames.
 *
 * The plug's click handler is asynchronous: the seam dispatches
 * `editor:decorationClick` to a web worker, the worker re-reads the document
 * through a syscall, writes the decoration config and nudges the editor. None
 * of that is bounded by an animation frame, so `settle()` after the click is a
 * race - it passed on a quiet machine and failed once under a loaded one, with
 * zero popovers found. Waiting on the element itself is the same assertion
 * with no clock in it.
 */
async function openPopover(view: View, control: Locator) {
  await control.click({ force: true });
  await view.page
    .locator(".atomdown-menu-popover")
    .first()
    .waitFor({ state: "visible", timeout: 15_000 });
  await settle(view.page);
}

/** The mirror: press and wait for the popover to be GONE. */
async function closePopover(view: View, control: Locator) {
  await control.click({ force: true });
  await view.page
    .locator(".atomdown-menu-popover")
    .waitFor({ state: "detached", timeout: 15_000 });
  await settle(view.page);
}

// ---------------------------------------------------------------------------
// 8a / 8b / 8g — the popover, and no picker
// ---------------------------------------------------------------------------

test("8a: the card's three-dot control opens a popover and no picker", async ({
  page,
}) => {
  await gotoFixture(page, server);
  const view = await openInline(page);

  const before = await popoverState(view);
  expect(before.popovers, "no popover exists at rest").toBe(0);

  const menu = await firstCardMenu(view);
  await openPopover(view, menu);

  const open = await popoverState(view);
  if (open.popovers !== 1) {
    await failWithArtifacts(
      page,
      8,
      "the three-dot control did not open a popover",
      { width: "comfort", theme: "light", density: "comfortable" } as Combo,
      open,
      `pressing .atomdown-card-menu produced ${open.popovers} ` +
        `.atomdown-menu-popover element(s), expected exactly 1. The control ` +
        `used to call editor.filterBox instead, which is the command palette.`,
    );
  }

  expect(
    await pickerCount(view),
    "no filter box, mini editor or modal appeared: the popover is the " +
      "card's own element, not the host's picker",
  ).toBe(0);

  // The popover is ANCHORED TO THE CARD: its own box overlaps the card head's
  // horizontal span, rather than being centred over the window the way the
  // picker is.
  const anchored = await view.ev.evaluate(() => {
    const pop = document.querySelector(".atomdown-menu-popover")!;
    const head = pop.closest(".atomdown-card-head")!;
    const p = pop.getBoundingClientRect();
    const h = head.getBoundingClientRect();
    return {
      insideHost: pop.closest(".sb-decoration-widget") !== null,
      // Right edges aligned: the popover hangs from the button.
      rightDelta: Math.abs(p.right - h.right),
      // Below the head, which is what "hangs from it" means.
      below: p.top >= h.bottom - 1,
      windowCentred: Math.abs((p.left + p.right) / 2 - innerWidth / 2) < 4,
    };
  });
  expect(anchored.insideHost, "the popover lives in the card's widget").toBe(
    true,
  );
  expect(anchored.rightDelta, "it hangs from the button, right-aligned to " +
    "the card").toBeLessThan(2);
  expect(anchored.below, "it hangs below the header row").toBe(true);
  expect(
    anchored.windowCentred,
    "it is NOT centred over the window, which is what the picker did",
  ).toBe(false);
});

test("8b: the popover's first child is an inert name-and-id label", async ({
  page,
}) => {
  await gotoFixture(page, server);
  const view = await openInline(page);
  const menu = await firstCardMenu(view);
  await openPopover(view, menu);

  const open = await popoverState(view);
  expect(open.labelIsFirstChild, "the identity label is the FIRST child").toBe(
    true,
  );
  expect(
    open.labelText,
    "the label carries the readable name and the id, the way the panel's does",
  ).toMatch(/[a-z0-9-]+\s+-\s+[0-9A-Z]{8}/);
  expect(
    open.rows.includes(open.labelText?.split("  -  ")[0] ?? "@@"),
    "the label is not also an action row",
  ).toBe(false);

  // CLICKING THE LABEL KEEPS THE POPOVER OPEN. It is the first thing a reader
  // clicks, and closing there would read as the menu refusing to work.
  await page.locator(".atomdown-menu-label-text").first().click({ force: true });
  await settle(page);
  expect(
    (await popoverState(view)).popovers,
    "a click on the identity label leaves the popover open",
  ).toBe(1);

  // ...and so does a click on its note line and on the popover's own padding.
  await page.locator(".atomdown-menu-label-note").first().click({ force: true });
  await settle(page);
  expect(
    (await popoverState(view)).popovers,
    "a click on the label's note line leaves the popover open",
  ).toBe(1);

  // A second press on the button closes it, and so does a click elsewhere.
  await closePopover(view, menu);
  expect(
    (await popoverState(view)).popovers,
    "a second press on the control closes the popover",
  ).toBe(0);

  await openPopover(view, menu);
  expect((await popoverState(view)).popovers).toBe(1);
  await page.locator(".cm-line.atomdown-card-line").first().click({
    force: true,
    position: { x: 200, y: 4 },
  });
  await page
    .locator(".atomdown-menu-popover")
    .waitFor({ state: "detached", timeout: 15_000 });
  expect(
    (await popoverState(view)).popovers,
    "a click outside the popover closes it",
  ).toBe(0);
});

test("8g: the popover holds no text input, because one cannot work here", async ({
  page,
}) => {
  // Not a style preference. `widgetPressGuard` in the decoration seam calls
  // preventDefault on a plain mousedown inside a widget, so an input in here
  // takes no focus from a click and the keystrokes go into the DOCUMENT.
  // Measured in input-probe.test.ts. This assertion is the guard rail.
  await gotoFixture(page, server);
  const view = await openInline(page);
  const menu = await firstCardMenu(view);
  await openPopover(view, menu);
  expect(
    (await popoverState(view)).inputs,
    "no input, textarea, select or contenteditable inside the popover: a " +
      "click cannot focus one and the typing would land in the document",
  ).toBe(0);

  // Every row IS a click target, so the menu is still usable.
  const rows = await popoverState(view);
  expect(rows.rows.length, "the popover offers action rows").toBeGreaterThan(2);
  const roles = await view.ev.evaluate(() =>
    Array.from(document.querySelectorAll(".atomdown-menu-item")).map((e) => ({
      role: e.getAttribute("role"),
      tabindex: e.getAttribute("tabindex"),
    }))
  );
  expect(
    roles.every((r) => r.role === "button" && r.tabindex === "0"),
    "every row is a keyboard-reachable button",
  ).toBe(true);
});

test("8a-group: the group control opens its own popover too", async ({
  page,
}) => {
  await gotoFixture(page, server);
  const view = await openInline(page);
  const gmenu = page.locator(".atomdown-group-menu").first();
  await gmenu.scrollIntoViewIfNeeded();
  await settle(page);
  await openPopover(view, gmenu);

  const open = await popoverState(view);
  expect(open.popovers, "the group control opens a popover").toBe(1);
  expect(await pickerCount(view), "and no picker").toBe(0);
  expect(open.labelIsFirstChild, "identity first, as the card's is").toBe(true);
  expect(open.rows, "the group's two actions, as rows and not palette items")
    .toEqual(["Rename group", "Ungroup"]);
});

// ---------------------------------------------------------------------------
// 8c — the glyph
// ---------------------------------------------------------------------------

test("8c: the three-dot glyph is the VERTICAL ellipsis, by character", async ({
  page,
}) => {
  await gotoFixture(page, server);
  const view = await openInline(page);

  const glyphs = await view.ev.evaluate(() => {
    const read = (sel: string) =>
      Array.from(document.querySelectorAll(sel)).map((e) =>
        (e.textContent ?? "").trim()
      );
    return {
      card: read(".atomdown-card-menu"),
      group: read(".atomdown-group-menu"),
      grip: read(".atomdown-grip"),
    };
  });

  expect(glyphs.card.length, "there are card controls to check")
    .toBeGreaterThan(0);
  expect(glyphs.group.length, "there are group controls to check")
    .toBeGreaterThan(0);

  for (const [name, list] of [
    ["card", glyphs.card],
    ["group", glyphs.group],
  ] as const) {
    expect(
      list.every((g) => g === VERTICAL_ELLIPSIS),
      `every ${name} control carries U+22EE VERTICAL ELLIPSIS. ` +
        `Got: ${JSON.stringify([...new Set(list)])}`,
    ).toBe(true);
    expect(
      list.some((g) => g === HORIZONTAL_ELLIPSIS),
      `and none carries U+22EF, the horizontal form it replaced`,
    ).toBe(false);
  }

  // The grip is unchanged: U+283F BRAILLE PATTERN DOTS-123456.
  expect(
    glyphs.grip.every((g) => g === "⠿"),
    "the drag grip is still the braille six-dot glyph",
  ).toBe(true);

  // NARROWER IS THE POINT, and it is measurable: the vertical glyph has to fit
  // in a 28.8px page margin at the `full` width.
  const width = await view.ev.evaluate(() => {
    const el = document.querySelector(".atomdown-card-menu")!;
    return el.getBoundingClientRect().width;
  });
  expect(width, "the glyph is narrow enough for the gutter").toBeLessThan(12);
});

// ---------------------------------------------------------------------------
// 8d — outside the card, by measurement, over the whole matrix
// ---------------------------------------------------------------------------

/**
 * Measure both controls against the card head's own border box.
 *
 * The head, not the line: the head IS the card's top edge and carries the
 * card's real side borders, so its rect is the border a control has to be
 * outside of. A member card's head is inset by the group's padding, which is
 * why one measurement covers both nesting cases.
 */
async function controlOffsets(view: View) {
  return await view.ev.evaluate(() => {
    const out: any[] = [];
    const heads = Array.from(
      document.querySelectorAll(".atomdown-card-head"),
    ) as HTMLElement[];
    for (const head of heads) {
      const grip = head.querySelector(".atomdown-grip") as HTMLElement | null;
      const menu = head.querySelector(".atomdown-card-menu") as HTMLElement | null;
      if (!grip || !menu) continue;
      const h = head.getBoundingClientRect();
      const g = grip.getBoundingClientRect();
      const m = menu.getBoundingClientRect();
      if (h.width === 0 || g.width === 0 || m.width === 0) continue;
      const nested = head.closest(".atomdown-nested") !== null;
      out.push({
        nested,
        // Positive means clear of the border, outside it.
        gripClear: +(h.left - g.right).toFixed(2),
        menuClear: +(m.left - h.right).toFixed(2),
        gripIsLeft: g.left < h.left,
        menuIsRight: m.right > h.right,
      });
    }
    return out;
  });
}

for (const density of DENSITIES) {
  for (const width of WIDTHS) {
    test(`8d: both controls sit outside the card border [${width}/${density}]`, async ({
      page,
    }) => {
      await gotoFixture(page, server);
      await setWidth(page, width);
      const view = await openInline(page);
      await setDensity(view, density);
      await settle(page);

      const offsets = await controlOffsets(view);
      expect(
        offsets.length,
        "there are card controls on screen to measure",
      ).toBeGreaterThan(0);

      const bad = offsets.filter(
        (o) => !o.gripIsLeft || !o.menuIsRight || o.gripClear <= 0 ||
          o.menuClear <= 0,
      );
      if (bad.length) {
        await failWithArtifacts(
          page,
          8,
          "a control is not outside the card's border",
          { width, theme: "light", density } as Combo,
          { measured: offsets.length, bad: bad.slice(0, 20) },
          `${bad.length} of ${offsets.length} card(s): the grip must be ` +
            `LEFT of the card head's left border and the three-dot menu ` +
            `RIGHT of its right border, with a positive gap. Worst: ` +
            `gripClear=${bad[0].gripClear}px menuClear=${bad[0].menuClear}px.`,
        );
      }

      // BOTH NESTING CASES ARE ACTUALLY COVERED at every width, or the
      // "one measurement covers both" claim above is untested.
      expect(
        offsets.some((o) => o.nested) && offsets.some((o) => !o.nested),
        "the sample holds a top-level card and a card inside a group",
      ).toBe(true);
    });
  }
}

// ---------------------------------------------------------------------------
// 8d-group — the GROUP's two controls, outside the group container (iugum-938)
//
// The same property as 8d, measured against the group header widget's own
// border box, which is the group box's top edge and carries the 2px accent
// outline. Steve: "the cards and groups were supposed to have same behavior,
// groups still show inside the container."
//
// AND THE COLLISION RULE, asserted rather than described. A member card's
// controls are in the same two gutters, so the two sets must not meet. The
// rule the plug states is "the group's controls are on the BAR's row, member
// cards keep theirs on their own rows", and the bar is a row of its own above
// every member - so the vertical bands are disjoint. There is a second,
// independent separation worth asserting because it is what makes the first
// one forgiving: a member card's head is inset by the group's padding plus the
// outline, so its controls sit in a lane about 10px right of the group's.
// ---------------------------------------------------------------------------

async function groupControlOffsets(view: View) {
  return await view.ev.evaluate(() => {
    const out: any[] = [];
    const bars = Array.from(
      document.querySelectorAll(".sb-decoration-widget.atomdown-group-header"),
    ) as HTMLElement[];
    for (const bar of bars) {
      const grip = bar.querySelector(":scope > .atomdown-group-grip") as
        | HTMLElement
        | null;
      const menu = bar.querySelector(":scope > .atomdown-group-menu") as
        | HTMLElement
        | null;
      const caret = bar.querySelector(".atomdown-group-collapse") as
        | HTMLElement
        | null;
      if (!grip || !menu) continue;
      const b = bar.getBoundingClientRect();
      const g = grip.getBoundingClientRect();
      const m = menu.getBoundingClientRect();
      if (b.width === 0 || g.width === 0 || m.width === 0) continue;
      const scroller = document.querySelector(".cm-scroller") as HTMLElement;
      const sBox = scroller.getBoundingClientRect();
      const clipLeft = sBox.left;
      const clipRight = sBox.left + scroller.clientWidth;
      // The hit test, because a clipped element still reports its full rect.
      const hit = (el: HTMLElement) => {
        const r = el.getBoundingClientRect();
        const x = r.left + r.width / 2;
        const y = r.top + r.height / 2;
        if (x < clipLeft + 2 || x > clipRight - 2) return "off-scroller";
        const probe = document.elementFromPoint(x, y);
        if (!probe || !probe.closest(".cm-editor")) return "not-measurable";
        return probe === el || el.contains(probe) ? "painted" : "covered";
      };
      out.push({
        groupId: (bar.querySelector(".atomdown-group-id")?.textContent ?? "")
          .trim(),
        // Positive means clear of the group's border, outside it.
        gripClear: +(b.left - g.right).toFixed(2),
        menuClear: +(m.left - b.right).toFixed(2),
        gripIsLeft: g.left < b.left,
        menuIsRight: m.right > b.right,
        gripPaint: hit(grip),
        menuPaint: hit(menu),
        // Clearance from the scroller's own client edges, which is the number
        // that shrinks as the content column widens.
        gripFromScroller: +(g.left - clipLeft).toFixed(2),
        menuFromScroller: +(clipRight - m.right).toFixed(2),
        // The bar keeps the chevron, at full size and always visible.
        caretVisible: caret
          ? parseFloat(getComputedStyle(caret).opacity) > 0.99
          : false,
        caretFontSize: caret ? getComputedStyle(caret).fontSize : null,
        caretInsideBar: caret
          ? caret.getBoundingClientRect().left >= b.left &&
            caret.getBoundingClientRect().right <= b.right
          : false,
        // Rows and lanes, for the collision assertion below.
        gripRect: { top: g.top, bottom: g.bottom, left: g.left, right: g.right },
        menuRect: { top: m.top, bottom: m.bottom, left: m.left, right: m.right },
      });
    }
    return out;
  });
}

/** Every member card's own gutter controls, with the group they sit in. */
async function memberControlRects(view: View) {
  return await view.ev.evaluate(() => {
    const out: any[] = [];
    const heads = Array.from(
      document.querySelectorAll(".atomdown-card-head.atomdown-nested"),
    ) as HTMLElement[];
    for (const head of heads) {
      const grip = head.querySelector(".atomdown-grip") as HTMLElement | null;
      const menu = head.querySelector(".atomdown-card-menu") as
        | HTMLElement
        | null;
      if (!grip || !menu) continue;
      const g = grip.getBoundingClientRect();
      const m = menu.getBoundingClientRect();
      if (g.width === 0 || m.width === 0) continue;
      out.push({
        gripRect: { top: g.top, bottom: g.bottom, left: g.left, right: g.right },
        menuRect: { top: m.top, bottom: m.bottom, left: m.left, right: m.right },
      });
    }
    return out;
  });
}

for (const density of DENSITIES) {
  for (const width of WIDTHS) {
    test(`8d-group: the group's controls sit outside the group container [${width}/${density}]`, async ({
      page,
    }) => {
      await gotoFixture(page, server);
      await setWidth(page, width);
      const view = await openInline(page);
      await setDensity(view, density);
      // A hover inside the group, so the hover-only controls are laid out
      // where a reader would see them. Their boxes exist at opacity 0 either
      // way, but measuring the revealed state is measuring what is used.
      const bar = page.locator(".atomdown-group-header").first();
      await bar.scrollIntoViewIfNeeded();
      await bar.hover({ force: true });
      await settle(page);

      const offsets = await groupControlOffsets(view);
      expect(offsets.length, "there are group controls on screen to measure")
        .toBeGreaterThan(0);

      const bad = offsets.filter(
        (o) =>
          !o.gripIsLeft || !o.menuIsRight || o.gripClear <= 0 ||
          o.menuClear <= 0,
      );
      if (bad.length) {
        await failWithArtifacts(
          page,
          8,
          "a group control is not outside the group container",
          { width, theme: "light", density } as Combo,
          { measured: offsets.length, bad: bad.slice(0, 10) },
          `${bad.length} of ${offsets.length} group(s): the grip's RIGHT ` +
            `edge must be left of the group container's left border and the ` +
            `menu's LEFT edge right of its right border. Worst: ` +
            `gripClear=${bad[0].gripClear}px menuClear=${bad[0].menuClear}px.`,
        );
      }

      // NOT CLIPPED, by hit test rather than by rect: a clipped element still
      // reports its full unclipped rect.
      const clipped = offsets.filter(
        (o) => o.gripPaint === "covered" || o.menuPaint === "covered",
      );
      if (clipped.length) {
        await failWithArtifacts(
          page,
          8,
          "a group control is not painted where its rect says it is",
          { width, theme: "light", density } as Combo,
          clipped.slice(0, 10),
          `document.elementFromPoint at the control's own centre returned ` +
            `something else, so the control is covered or clipped.`,
        );
      }
      const offScroller = offsets.filter(
        (o) => o.gripFromScroller < 0 || o.menuFromScroller < 0,
      );
      expect(
        offScroller.length,
        "no group control leaves the editor's scroll container",
      ).toBe(0);

      // THE BAR KEEPS THE CHEVRON, inside it, full size and always visible at
      // both densities. That rule predates this change and must survive it.
      const chevron = offsets.filter((o) => !o.caretVisible || !o.caretInsideBar);
      expect(
        chevron.length,
        "the collapse chevron stays inside the bar and stays visible: " +
          JSON.stringify(chevron.slice(0, 3)),
      ).toBe(0);
      expect(
        new Set(offsets.map((o) => o.caretFontSize)).size,
        "the chevron is one size, whatever the density",
      ).toBe(1);

      // THE COLLISION RULE. Two independent separations, both asserted.
      const members = await memberControlRects(view);
      expect(members.length, "there are member cards with their own controls")
        .toBeGreaterThan(0);
      const overlap = (
        a: { top: number; bottom: number; left: number; right: number },
        b: typeof a,
      ) =>
        Math.min(a.bottom, b.bottom) - Math.max(a.top, b.top) > 0.75 &&
        Math.min(a.right, b.right) - Math.max(a.left, b.left) > 0.75;
      const collisions: any[] = [];
      for (const g of offsets) {
        for (const m of members) {
          if (overlap(g.gripRect, m.gripRect)) {
            collisions.push({ what: "grip vs member grip", g: g.gripRect, m: m.gripRect });
          }
          if (overlap(g.menuRect, m.menuRect)) {
            collisions.push({ what: "menu vs member menu", g: g.menuRect, m: m.menuRect });
          }
        }
      }
      if (collisions.length) {
        await failWithArtifacts(
          page,
          8,
          "a group control collides with a member card's control",
          { width, theme: "light", density } as Combo,
          collisions.slice(0, 6),
          `${collisions.length} overlap(s). The rule is that a group's ` +
            `controls sit on the group header BAR's row and a member card ` +
            `keeps its own on its own row, so the two can never share a row ` +
            `- not for the first member and not for the last.`,
        );
      }

      // ...and the lanes differ, which is the second separation: a member
      // card's head is inset by the group's padding plus its outline, so its
      // controls sit to the RIGHT of the group's in the left gutter and to
      // the LEFT of them in the right gutter.
      const lanes = members.every((m) =>
        offsets.every((g) =>
          m.gripRect.left > g.gripRect.left &&
          m.menuRect.right < g.menuRect.right
        )
      );
      expect(
        lanes,
        "a member card's controls sit in a different lane from the group's",
      ).toBe(true);
    });
  }
}

test("8e-group: the group's controls are hidden at rest and revealed by a hover anywhere inside the group", async ({
  page,
}) => {
  await gotoFixture(page, server);
  const view = await openInline(page);
  await page.mouse.move(2, 2);
  await settle(page);

  const read = async () =>
    await view.ev.evaluate(() => {
      const bar = document.querySelector(
        ".sb-decoration-widget.atomdown-group-header",
      )!;
      const grip = bar.querySelector(":scope > .atomdown-group-grip")!;
      const menu = bar.querySelector(":scope > .atomdown-group-menu")!;
      const caret = bar.querySelector(".atomdown-group-collapse")!;
      return {
        grip: parseFloat(getComputedStyle(grip).opacity),
        menu: parseFloat(getComputedStyle(menu).opacity),
        caret: parseFloat(getComputedStyle(caret).opacity),
      };
    });

  const bar = page.locator(".atomdown-group-header").first();
  await bar.scrollIntoViewIfNeeded();
  await page.mouse.move(2, 2);
  await settle(page);
  const rest = await read();
  expect(rest.grip, "the group grip is invisible at rest").toBe(0);
  expect(rest.menu, "the group's three-dot control is invisible at rest").toBe(
    0,
  );
  expect(rest.caret, "the chevron is NOT hover-only: it is always visible")
    .toBe(1);

  // A HOVER OVER A MEMBER CARD, not over the bar. That is the scope the
  // group's chrome already follows, and plain :hover on the bar cannot reach
  // it - the seam's hover class on the group's first line does.
  const member = page
    .locator(".cm-line.atomdown-card-line.atomdown-group-line")
    .first();
  await member.scrollIntoViewIfNeeded();
  await bar.scrollIntoViewIfNeeded();
  await member.hover({ force: true });
  await settle(page);
  const overCard = await read();
  expect(
    overCard.grip,
    "hovering a member card reveals the GROUP's grip",
  ).toBeGreaterThan(0);
  expect(
    overCard.menu,
    "hovering a member card reveals the GROUP's menu",
  ).toBeGreaterThan(0);

  // KEYBOARD FOCUS TOO, or a tab user moves through an invisible control.
  await page.mouse.move(2, 2);
  await settle(page);
  expect((await read()).menu, "back to invisible off hover").toBe(0);
  await view.ev.evaluate(() => {
    (document.querySelector(
      ".sb-decoration-widget.atomdown-group-header > .atomdown-group-menu",
    ) as HTMLElement).focus();
  });
  await settle(page);
  expect(
    (await read()).menu,
    "a focused group menu is visible",
  ).toBeGreaterThan(0);
  await view.ev.evaluate(() => {
    (document.querySelector(
      ".sb-decoration-widget.atomdown-group-header > .atomdown-group-grip",
    ) as HTMLElement).focus();
  });
  await settle(page);
  expect((await read()).grip, "a focused group grip is visible")
    .toBeGreaterThan(0);
});

// ---------------------------------------------------------------------------
// 8e — hidden at rest, shown on hover AND on keyboard focus
// ---------------------------------------------------------------------------

async function opacities(view: View) {
  return await view.ev.evaluate(() => {
    const head = document.querySelector(".atomdown-card-head")!;
    const grip = head.querySelector(".atomdown-grip")!;
    const menu = head.querySelector(".atomdown-card-menu")!;
    return {
      grip: parseFloat(getComputedStyle(grip).opacity),
      menu: parseFloat(getComputedStyle(menu).opacity),
    };
  });
}

test("8e: both controls are hidden at rest, shown on hover and on focus", async ({
  page,
}) => {
  await gotoFixture(page, server);
  const view = await openInline(page);
  // Park the pointer somewhere that is not a card.
  await page.mouse.move(2, 2);
  await settle(page);

  const rest = await opacities(view);
  expect(rest.grip, "the grip is invisible at rest").toBe(0);
  expect(rest.menu, "the three-dot control is invisible at rest").toBe(0);

  // HOVER ANYWHERE ON THE CARD, not only on the header row: that is what the
  // seam's hover classes are for.
  const line = page.locator(".cm-line.atomdown-card-line").first();
  await line.scrollIntoViewIfNeeded();
  await line.hover({ force: true });
  await settle(page);
  const hovered = await opacities(view);
  expect(hovered.grip, "hovering the card reveals the grip").toBeGreaterThan(0);
  expect(hovered.menu, "hovering the card reveals the menu").toBeGreaterThan(0);

  // KEYBOARD FOCUS REVEALS THEM TOO, or a tab user moves through a control
  // they cannot see. `:focus` as well as `:focus-visible`, which is the
  // panel's own lesson: :focus-visible does not match a scripted focus.
  await page.mouse.move(2, 2);
  await settle(page);
  expect((await opacities(view)).menu, "back to invisible off hover").toBe(0);

  await view.ev.evaluate(() => {
    (document.querySelector(".atomdown-card-head .atomdown-card-menu") as
      HTMLElement).focus();
  });
  await settle(page);
  expect(
    (await opacities(view)).menu,
    "a focused three-dot control is visible",
  ).toBeGreaterThan(0);

  await view.ev.evaluate(() => {
    (document.querySelector(".atomdown-card-head .atomdown-grip") as HTMLElement)
      .focus();
  });
  await settle(page);
  expect(
    (await opacities(view)).grip,
    "a focused grip is visible",
  ).toBeGreaterThan(0);
});

test("8e-open: an open popover keeps its own button visible", async ({
  page,
}) => {
  await gotoFixture(page, server);
  const view = await openInline(page);
  const menu = await firstCardMenu(view);
  await openPopover(view, menu);
  // Pointer off the card: the popover is open, so the button must stay lit or
  // the control the popover belongs to vanishes under the reader's hand.
  await page.mouse.move(2, 2);
  await settle(page);
  const lit = await view.ev.evaluate(() =>
    parseFloat(
      getComputedStyle(
        document.querySelector(".atomdown-menu-open .atomdown-card-menu")!,
      ).opacity,
    )
  );
  expect(lit, "the open menu's button stays visible").toBe(1);
});

// ---------------------------------------------------------------------------
// 8h — the hover treatment on the accent fill
// ---------------------------------------------------------------------------

/**
 * Steve's defect 2, asserted WITHOUT pixels.
 *
 * What he saw: hovering the three-dot control inside the saturated blue group
 * bar painted a white background, which reads as a hole punched in the bar.
 * The old rule was `background: var(--ui-accent-contrast-color)`, which is
 * white in both themes: fine on the bar's pale resting tint, wrong on the
 * accent fill the bar takes when the pointer is inside the group.
 *
 * WHY THIS IS HERE AS WELL AS IN RULE 9. A screenshot sees the hole, which is
 * why rule 9 exists at all - but a screenshot is font-dependent and
 * platform-suffixed, and this property is neither. A composited colour is a
 * number: an opaque chip is `alpha == 1` and far from the bar's own fill, and
 * a wash is translucent and near it. That holds on any machine, in either
 * theme, whatever font is installed.
 */
test("8h: the group control's hover works on the accent fill, not against it", async ({
  page,
}) => {
  await gotoFixture(page, server);
  const view = await openInline(page);

  const bar = page.locator(".atomdown-group-header").first();
  await bar.scrollIntoViewIfNeeded();
  await settle(page);

  // THE CONTROL THIS MEASURES IS THE COLLAPSE CHEVRON, and that is a
  // retarget rather than a relaxation (iugum-938). The three-dot menu used to
  // sit on the bar and was half of this defect; it is in the page gutter now,
  // on the page's own ground, where there is no accent fill to punch a hole
  // in. The chevron stays on the bar, was the SAME solid white chip, and is
  // now the only control this property can apply to. The property itself is
  // unchanged: a wash, not an opaque chip, measured as a composited colour.
  // The menu's own half is asserted below, as "still not opaque".
  const control = bar.locator(".atomdown-group-collapse");
  await control.hover({ force: true });
  await settle(page);

  const measured = await view.ev.evaluate(() => {
    /**
     * Parse a computed colour, in BOTH forms Chrome produces.
     *
     * `color-mix(in srgb, ...)` does not compute to `rgba()`: Chrome resolves
     * it to `color(srgb 1 1 1 / 0.24)`, with components 0..1 rather than
     * 0..255. A parser that only knew `rgba()` returned null for exactly the
     * declaration this test exists to check.
     */
    function rgba(v: string) {
      const srgb = v.match(
        /color\(\s*srgb\s+([\d.eE+-]+)\s+([\d.eE+-]+)\s+([\d.eE+-]+)(?:\s*\/\s*([\d.eE+-]+))?\s*\)/,
      );
      if (srgb) {
        return {
          r: parseFloat(srgb[1]) * 255,
          g: parseFloat(srgb[2]) * 255,
          b: parseFloat(srgb[3]) * 255,
          a: srgb[4] === undefined ? 1 : parseFloat(srgb[4]),
        };
      }
      const m = v.match(/rgba?\(([^)]+)\)/);
      if (!m) return null;
      const parts = m[1].split(/[,\s/]+/).filter(Boolean).map((n) =>
        parseFloat(n)
      );
      return {
        r: parts[0],
        g: parts[1],
        b: parts[2],
        a: parts.length > 3 ? parts[3] : 1,
      };
    }
    const barEl = document.querySelector(".atomdown-group-header")!;
    const menuEl = barEl.querySelector(".atomdown-group-menu")!;
    const caretEl = barEl.querySelector(".atomdown-group-collapse")!;
    return {
      bar: rgba(getComputedStyle(barEl).backgroundColor),
      // `menu` here is the control STILL ON THE BAR - the chevron. The name
      // is kept so the failure message below reads as it always did.
      menu: rgba(getComputedStyle(caretEl).backgroundColor),
      caret: rgba(getComputedStyle(caretEl).backgroundColor),
      gutterMenu: rgba(getComputedStyle(menuEl).backgroundColor),
    };
  });

  expect(measured.bar, "the bar has a background to sit on").not.toBeNull();
  expect(measured.menu, "the hovered control has a background").not.toBeNull();

  // The bar under the pointer is the SATURATED fill, not the resting tint.
  // Without this the assertion below could pass on the wrong state.
  expect(
    measured.bar!.a,
    "the bar is at full strength while the pointer is inside the group",
  ).toBeGreaterThan(0.9);

  // A WASH, NOT A FILL. Anything opaque replaces the bar rather than lifting
  // off it, and that is the hole.
  if (measured.menu!.a >= 0.9) {
    await failWithArtifacts(
      page,
      8,
      "the hovered control paints an opaque chip on the accent fill",
      { width: "comfort", theme: "light", density: "comfortable" } as Combo,
      measured,
      `the hovered .atomdown-group-menu background is opaque ` +
        `(alpha ${measured.menu!.a}) over a bar at alpha ${measured.bar!.a}. ` +
        `On the saturated accent fill an opaque chip reads as a hole punched ` +
        `through the bar, which is the defect. Use a translucent wash of the ` +
        `contrast colour instead.`,
    );
  }

  // ...and it is a wash of the CONTRAST colour, so it lifts rather than
  // darkening: a translucent black would also pass the alpha test and would
  // read as a smudge.
  const menu = measured.menu!;
  expect(
    (menu.r + menu.g + menu.b) / 3,
    "the wash is light, so the control lifts off the fill",
  ).toBeGreaterThan(160);

  // AND THE CONTROL THAT LEFT THE BAR IS STILL NOT AN OPAQUE CHIP. It is in
  // the page gutter now, where an opaque background would read as a chip
  // floating in the margin rather than as a hole in the bar - the same defect
  // in a different place, so the same assertion follows it out.
  await bar.locator(".atomdown-group-menu").hover({ force: true });
  await settle(page);
  const gutter = await view.ev.evaluate(() => {
    const v = getComputedStyle(
      document.querySelector(
        ".sb-decoration-widget.atomdown-group-header > .atomdown-group-menu",
      )!,
    ).backgroundColor;
    const srgb = v.match(
      /color\(\s*srgb\s+\S+\s+\S+\s+\S+\s*\/\s*([\d.eE+-]+)\s*\)/,
    );
    if (srgb) return parseFloat(srgb[1]);
    const m = v.match(/rgba?\(([^)]+)\)/);
    if (!m) return 1;
    const parts = m[1].split(/[,\s/]+/).filter(Boolean).map((n) =>
      parseFloat(n)
    );
    return parts.length > 3 ? parts[3] : 1;
  });
  expect(
    gutter,
    "the group's three-dot control, now in the gutter, paints no opaque chip",
  ).toBeLessThan(0.9);
});

// ---------------------------------------------------------------------------
// 8f — none of this writes a document byte
// ---------------------------------------------------------------------------

for (const combo of combos()) {
  test(`8f: hovering, opening and closing a menu changes no byte [${comboName(combo)}]`, async ({
    page,
  }) => {
    await gotoFixture(page, server);
    await setWidth(page, combo.width);
    const view = await openInline(page);
    await setDensity(view, combo.density);
    const before = await readPageBytes(server);

    // Hover a card, hover its group bar, open the card menu, click the inert
    // label, close it, open the group menu, close it, switch density twice.
    await page.locator(".cm-line.atomdown-card-line").first().hover({
      force: true,
    });
    await settle(page);
    await page.locator(".atomdown-group-header").first().hover({ force: true });
    await settle(page);

    const menu = await firstCardMenu(view);
    await openPopover(view, menu);
    await page.locator(".atomdown-menu-label-text").first().click({
      force: true,
    });
    await settle(page);
    await closePopover(view, menu);

    const gmenu = page.locator(".atomdown-group-menu").first();
    await gmenu.scrollIntoViewIfNeeded();
    await settle(page);
    await openPopover(view, gmenu);
    await closePopover(view, gmenu);

    await setDensity(view, combo.density === "compact" ? "comfortable" : "compact");
    await setDensity(view, combo.density);
    await settle(page);

    const after = await readPageBytes(server);
    if (after !== before) {
      await failWithArtifacts(
        page,
        8,
        "hovering or opening a menu changed the document",
        combo,
        { beforeLength: before.length, afterLength: after.length },
        `the page's bytes changed. Every control in this rule is ` +
          `presentational: opening a menu writes the decoration config and ` +
          `nothing else.`,
      );
    }
  });
}
