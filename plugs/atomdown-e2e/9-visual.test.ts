/**
 * RULE 9 — VISUAL REGRESSION.
 *
 * The rule that exists because the other eight cannot see a colour.
 *
 * Steve's defect 2 was "hovering the three-dot control inside the saturated
 * blue group bar paints a WHITE background, which reads as a hole punched in
 * the bar". Every assertion in rules 1 to 8 was green while that was on
 * screen: the control was the right size, in the right place, with the right
 * class, and it opened the right thing. Only an image sees a hole.
 *
 * WHAT IS CAPTURED. Element screenshots of the chrome, not the page: the group
 * bar at rest and with each of its two controls hovered, a card's header row
 * at rest and hovered, and an open popover. A full-page baseline would fail on
 * every unrelated change to the fixture, and a rule that fails for unrelated
 * reasons is a rule people delete.
 *
 * ===========================================================================
 * THE ENVIRONMENT. Steve's hard requirement, 2026-09-05, quoted in the bead:
 * "I would go further and require it to be playwright headless so we pin
 * everything and use the same browser that also won't interfere with my screen
 * too."
 * ===========================================================================
 *
 * So: one pinned environment, and nothing about a developer's machine or
 * session may influence a baseline.
 *
 *  - HEADLESS ONLY. `guardEnvironment` below refuses to run at all if the
 *    project is headed, if `--headed` is on the command line, or if `PWDEBUG`
 *    is set. It refuses before the first screenshot, so a headed run can
 *    neither produce nor update a baseline.
 *  - PLAYWRIGHT'S OWN BUNDLED CHROMIUM, at the version in
 *    `silverbullet/package-lock.json`. The `visual` project sets
 *    `channel: undefined`, and `guardEnvironment` refuses if
 *    `ATOMDOWN_FE_CHANNEL` names a channel or if the bundled build is not
 *    installed - the default project's fallback to the system Chrome is
 *    deliberately NOT available here, because an auto-updating browser is how
 *    two machines silently disagree.
 *  - NO INTERFERENCE WITH HIS SCREEN. Headless opens no window, takes no
 *    focus and puts no tab in his browser.
 *  - THE RESOLVED BROWSER VERSION IS RECORDED next to the baselines, in
 *    `visual-baselines/browser.txt`, and compared on every later run. A
 *    mismatch is reported in words rather than as a mysterious pixel diff.
 *  - PINNED: viewport per width case, `deviceScaleFactor: 1`, `colorScheme`
 *    forced per theme case rather than inherited, `reducedMotion: "reduce"`,
 *    `animations: "disabled"` and `caret: "hide"` on every capture, and a
 *    fixed `timezoneId` and `locale` (a date or a number in the fixture would
 *    otherwise render differently in another region).
 *  - DETERMINISTIC WAITS. `document.fonts.ready` plus the decoration classes
 *    being present, never a timeout.
 *
 * FONTS, which are the classic cross-machine diff. They are NOT pinned: the
 * space's CSS resolves to `ui-monospace` and the theme's own body font, both
 * of which are whatever the host provides, and this repo ships no font file.
 * So every glyph in the chrome is coloured TRANSPARENT before a capture (see
 * `hideChromeText`), which removes the text's pixels while leaving every box,
 * fill, border, radius and shadow where it was. What the baseline compares is
 * the thing under test, because a white chip on a blue bar is a fill.
 *
 * Playwright's own `mask` option is deliberately NOT used: it paints a flat
 * box over each region, and the first draft of this rule masked the two
 * controls - which are the subject. Reintroducing the defect proved it: all
 * twelve baselines passed with a solid white chip on the accent fill, because
 * the mask sat on top of the chip.
 *
 * The residual limit, stated rather than hidden: transparent text still takes
 * its own advance width, so a chip's WIDTH depends on the installed font even
 * though its FILL does not. Hence per-platform baselines, a recorded browser
 * build, and rule 8h asserting the same property numerically - composited
 * colours, which are font-free and run anywhere.
 *
 * Nothing here asserts WHICH character the button carries. Rule 8c does that,
 * by code point.
 *
 * Baselines are still platform-suffixed by Playwright (`-darwin.png`), so a
 * Linux CI run writes its own set rather than failing against a Mac's.
 *
 * -------------------------------------------------------------------------
 * TO UPDATE A BASELINE after an intended change - ONE command:
 *
 *     scripts/atomdown-fe-check.sh --visual --update-snapshots
 *
 * That is cheap on purpose. A baseline that is expensive to update is a
 * baseline someone eventually silences by raising the threshold, and the
 * threshold here is zero tolerance (`maxDiffPixels: 0`) precisely so it
 * cannot drift quietly.
 * -------------------------------------------------------------------------
 */

import { chromium, test, type TestInfo } from "@playwright/test";
import { existsSync, mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { dirname, join } from "node:path";
import {
  combos,
  comboName,
  expect,
  gotoFixture,
  openInline,
  type Page,
  type SBServer,
  setDensity,
  settle,
  setWidth,
  startSpace,
  type View,
} from "./harness.ts";

/** Where the baselines and the recorded browser version live. */
const BASELINE_DIR = join(import.meta.dirname, "visual-baselines");

let server: SBServer;

/**
 * Refuse to run in any environment that is not the pinned one.
 *
 * Thrown from `beforeAll`, so it fires before the first capture: a headed run
 * cannot produce a baseline and cannot update one, which is the requirement
 * rather than a nicety.
 */
function guardEnvironment(info: TestInfo) {
  const headless = (info.project.use as { headless?: boolean }).headless;
  if (headless === false) {
    throw new Error(
      "RULE 9 REFUSED: the visual project is configured headed.\n" +
        "Visual baselines are produced and compared headless only, so that a " +
        "developer's window manager, display scaling and focus state cannot " +
        "reach the pixels. Remove the headless override and run:\n" +
        "  scripts/atomdown-fe-check.sh --visual",
    );
  }
  if (process.argv.includes("--headed") || process.env.PWDEBUG) {
    throw new Error(
      "RULE 9 REFUSED: --headed or PWDEBUG is set.\n" +
        "A headed run may not produce or update a visual baseline. Debug the " +
        "behaviour with rules 1 to 8, which are headed-safe, or read the " +
        "diff image the failing capture writes.",
    );
  }
  const channel = (info.project.use as { channel?: string }).channel;
  if (channel) {
    throw new Error(
      `RULE 9 REFUSED: the visual project asks for browser channel ` +
        `"${channel}".\nBaselines use Playwright's own bundled Chromium, ` +
        `pinned in silverbullet/package-lock.json. An installed Chrome ` +
        `auto-updates, which is how two machines silently disagree about a ` +
        `pixel.`,
    );
  }
  if (process.env.ATOMDOWN_FE_CHANNEL &&
    process.env.ATOMDOWN_FE_CHANNEL !== "chromium") {
    throw new Error(
      `RULE 9 REFUSED: ATOMDOWN_FE_CHANNEL=` +
        `${process.env.ATOMDOWN_FE_CHANNEL}.\nThat variable switches the ` +
        `behavioural rules to a system browser. It may not switch this one.`,
    );
  }
  let bundled = "";
  try {
    bundled = chromium.executablePath();
  } catch {
    bundled = "";
  }
  if (!bundled || !existsSync(bundled)) {
    throw new Error(
      "RULE 9 REFUSED: Playwright's bundled Chromium is not installed.\n" +
        "The behavioural rules fall back to the system Chrome rather than " +
        "failing over a 150MB download. This rule must not: install it once " +
        "with\n" +
        "  cd silverbullet && npx playwright install chromium",
    );
  }
}

/**
 * Record the resolved browser version beside the baselines, and compare it.
 *
 * A pixel diff caused by a browser upgrade looks identical to a pixel diff
 * caused by a bug. This makes the first one say so.
 */
function checkBrowserVersion(version: string) {
  const path = join(BASELINE_DIR, "browser.txt");
  const line = `chromium ${version}\n`;
  if (!existsSync(path)) {
    mkdirSync(dirname(path), { recursive: true });
    writeFileSync(path, line);
    return;
  }
  const recorded = readFileSync(path, "utf8").trim();
  if (recorded !== line.trim()) {
    throw new Error(
      `RULE 9: the browser does not match the one the baselines were taken ` +
        `with.\n  baselines: ${recorded}\n  this run:  ${line.trim()}\n` +
        `A browser upgrade moves pixels for reasons that are not a defect. ` +
        `Review the diffs, then re-take the baselines in ONE command:\n` +
        `  scripts/atomdown-fe-check.sh --visual --update-snapshots\n` +
        `...which also rewrites ${path}.`,
    );
  }
}

test.beforeAll(async ({ browser }, info) => {
  guardEnvironment(info);
  if (process.env.ATOMDOWN_FE_UPDATE_BROWSER === "1") {
    mkdirSync(BASELINE_DIR, { recursive: true });
    writeFileSync(
      join(BASELINE_DIR, "browser.txt"),
      `chromium ${browser.version()}\n`,
    );
  }
  checkBrowserVersion(browser.version());
  server = await startSpace();
});
test.afterAll(async () => await server?.stop());

/**
 * Wait on CONDITIONS, never on a clock.
 *
 * Fonts loaded, because a capture taken mid-swap shows the fallback face; the
 * decoration classes present, because they are what is being photographed; and
 * the editor's own measure cycle settled, through the harness's `settle`.
 */
async function readyForCapture(view: View) {
  await view.page.waitForFunction(() => document.fonts.status === "loaded");
  await view.page
    .locator(".atomdown-card-line")
    .first()
    .waitFor({ state: "attached" });
  await view.page
    .locator(".atomdown-group-header")
    .first()
    .waitFor({ state: "visible" });
  await settle(view.page, 4);
}

/**
 * Take the FONT out of the picture without taking the FILLS out with it.
 *
 * Every glyph in the chrome is coloured transparent, so the text contributes
 * no pixels while every box, fill, border, radius and shadow stays exactly
 * where it was. That is what the baselines compare.
 *
 * WHY NOT `mask`. Playwright's `mask` paints a flat box over each region, and
 * the first draft masked the two controls - which are the SUBJECT. Verified by
 * reintroducing the defect: with the controls masked, a solid white chip on
 * the accent fill passed all twelve baselines, because the pink mask box sat
 * on top of the chip. A rule that cannot see the defect it was written for is
 * worse than no rule.
 *
 * THE RESIDUAL LIMIT, stated rather than hidden. Transparent text still takes
 * up its own advance width, so a chip's WIDTH depends on the glyph's metrics
 * and therefore on the installed font. The fills, which is where this defect
 * lives, do not. So these baselines are per-platform (Playwright suffixes
 * them, `-darwin.png`) and per-browser-build (recorded in `browser.txt`), and
 * the font-free half of the same property is asserted numerically by rule 8h,
 * which compares composited colours and runs anywhere.
 */
async function hideChromeText(view: View) {
  await view.page.addStyleTag({
    content: `.atomdown-group-kind,
              .atomdown-group-name,
              .atomdown-group-id,
              .atomdown-group-count,
              .atomdown-group-collapse,
              .atomdown-group-menu,
              .atomdown-card-slug,
              .atomdown-card-id,
              .atomdown-card-badge,
              .atomdown-card-menu,
              .atomdown-grip,
              .atomdown-menu-label-text,
              .atomdown-menu-label-note,
              .atomdown-menu-item,
              .cm-line.atomdown-card-line {
                color: transparent !important;
                text-shadow: none !important;
              }`,
  });
  await settle(view.page, 2);
}

/**
 * A clip box around the first card's top strip, wide enough to hold both
 * gutters.
 *
 * Whole pixels: a fractional clip makes Playwright resample, and a resampled
 * edge differs by a pixel between two runs that are otherwise identical.
 */
async function clipAroundCard(view: View) {
  const box = await view.ev.evaluate(() => {
    const head = document.querySelector(".atomdown-card-head")!;
    const r = head.getBoundingClientRect();
    const gutter = parseFloat(
      getComputedStyle(document.documentElement).getPropertyValue(
        "--board-chrome-gutter",
      ),
    ) || 22;
    const pad = Math.ceil(gutter) + 8;
    return {
      x: Math.max(0, Math.floor(r.left - pad)),
      y: Math.max(0, Math.floor(r.top - 4)),
      width: Math.ceil(r.width + pad * 2),
      height: Math.ceil(Math.max(r.height, 20) + 8),
    };
  });
  return box;
}

/**
 * A clip box around the first group bar, wide enough to hold both gutters.
 *
 * WHY THE BAR IS NO LONGER AN ELEMENT SCREENSHOT (iugum-938). The group's grip
 * and its three-dot menu sit OUTSIDE the group container now, in the same two
 * page gutters the card's controls use. An element screenshot of the bar
 * frames both of them out, so the baseline would no longer contain the
 * subject - the same reason the card's captures were already a clip.
 */
async function clipAroundBar(view: View) {
  return await view.ev.evaluate(() => {
    const bar = document.querySelector(
      ".sb-decoration-widget.atomdown-group-header",
    )!;
    const r = bar.getBoundingClientRect();
    const gutter = parseFloat(
      getComputedStyle(document.documentElement).getPropertyValue(
        "--board-chrome-gutter",
      ),
    ) || 22;
    const pad = Math.ceil(gutter) + 8;
    return {
      x: Math.max(0, Math.floor(r.left - pad)),
      y: Math.max(0, Math.floor(r.top - 4)),
      width: Math.ceil(r.width + pad * 2),
      height: Math.ceil(Math.max(r.height, 20) + 8),
    };
  });
}

/**
 * Put the real pointer on the first element matching `sel` WITHOUT scrolling.
 *
 * `locator.hover()` scrolls its target into view, and a scroll after a clip
 * box has been computed photographs the wrong part of the page. This reads the
 * element's own rect and moves the mouse there, so the viewport does not move.
 */
async function hoverInside(page: Page, sel: string) {
  const point = await page.evaluate((s) => {
    const el = document.querySelector(s) as HTMLElement | null;
    if (!el) return null;
    const r = el.getBoundingClientRect();
    if (r.width === 0 || r.height === 0) return null;
    return { x: r.left + r.width / 2, y: r.top + r.height / 2 };
  }, sel);
  if (!point) throw new Error(`rule 9: nothing to hover for ${sel}`);
  await page.mouse.move(point.x, point.y);
}

const SNAPSHOT_OPTS = {
  animations: "disabled" as const,
  caret: "hide" as const,
  maxDiffPixels: 0,
  scale: "device" as const,
};

for (const combo of combos()) {
  test.describe(`${comboName(combo)}`, () => {
    // colorScheme FORCED per theme case, not inherited from the machine.
    test.use({ colorScheme: combo.theme });

    test(`9: the group bar, at rest and with each control hovered [${comboName(combo)}]`, async ({
      page,
    }) => {
      await gotoFixture(page, server);
      await setWidth(page, combo.width);
      const view = await openInline(page);
      await setDensity(view, combo.density);
      await readyForCapture(view);
      // The glyphs go transparent so the CHIP behind them is what is
      // compared. Their identity is rule 8c's job, by code point.
      await hideChromeText(view);

      const bar = page.locator(".atomdown-group-header").first();
      await bar.scrollIntoViewIfNeeded();
      await settle(page, 3);
      // A CLIP, NOT AN ELEMENT SCREENSHOT, since iugum-938: the group's grip
      // and menu are outside the container, so a box tight to the bar frames
      // the subject out. Same reason the card's captures are already clips.
      const clip = await clipAroundBar(view);

      // AT REST: the bar's resting tint, no chip anywhere, and both gutters
      // EMPTY - the two controls are hover-only now, so a control visible in
      // this baseline is itself the regression.
      await page.mouse.move(2, 2);
      await settle(page, 3);
      await expect(page).toHaveScreenshot(
        `group-bar-rest-${comboName(combo).replace(/\//g, "-")}.png`,
        { ...SNAPSHOT_OPTS, clip },
      );

      // HOVERED ANYWHERE INSIDE THE GROUP: the bar goes to full accent. The
      // pointer is on a MEMBER CARD, which is the scope the group's chrome
      // follows and the one plain :hover on the bar cannot reach.
      //
      // WHAT THIS BASELINE COMPARES IS THE BAR'S FILL, not the two controls.
      // Their glyphs are coloured transparent like every other glyph here,
      // and they carry no background of their own by design, so they
      // contribute no pixels - which is the font trade this whole rule makes.
      // That the controls appear on hover, and where, is measured numerically
      // by 8e-group and 8d-group, the same split as 8h and this rule.
      //
      // A REAL MOUSE MOVE TO A COMPUTED POINT, never `locator.hover()`. A
      // locator hover scrolls its target into view, which moved the bar after
      // the clip had been computed - the first draft of this baseline
      // photographed a card three rows further down. The clip is fixed for
      // all four captures on purpose, so what changes between them is the
      // state and nothing else.
      // THIS GROUP's own first member line - the bar's next sibling - and not
      // the document's first `.atomdown-group-line`, which belongs to
      // whichever group comes first in the page rather than to the group in
      // the clip. Hovering the wrong group left the bar at its resting tint,
      // so the "hovered" baseline was byte-identical to the "rest" one.
      await hoverInside(
        page,
        ".sb-decoration-widget.atomdown-group-header + .cm-line",
      );
      await settle(page, 3);
      await expect(page).toHaveScreenshot(
        `group-bar-hover-${comboName(combo).replace(/\//g, "-")}.png`,
        { ...SNAPSHOT_OPTS, clip },
      );

      // THE DEFECT'S OWN STATE, followed to where the control went: the bar
      // at full accent with the three-dot control under the pointer. It is on
      // the page ground now rather than on the fill, so what this proves has
      // shifted from "no hole in the bar" to "no chip in the margin"; both are
      // the same thing a number cannot describe.
      await hoverInside(
        page,
        ".sb-decoration-widget.atomdown-group-header > .atomdown-group-menu",
      );
      await settle(page, 3);
      await expect(page).toHaveScreenshot(
        `group-bar-menu-hover-${comboName(combo).replace(/\//g, "-")}.png`,
        { ...SNAPSHOT_OPTS, clip },
      );

      // The collapse caret is the control STILL ON THE FILL, so it is the one
      // the original defect's baseline now belongs to.
      await hoverInside(
        page,
        ".sb-decoration-widget.atomdown-group-header .atomdown-group-collapse",
      );
      await settle(page, 3);
      await expect(page).toHaveScreenshot(
        `group-bar-caret-hover-${comboName(combo).replace(/\//g, "-")}.png`,
        { ...SNAPSHOT_OPTS, clip },
      );
    });

    test(`9: the group's popover, open [${comboName(combo)}]`, async ({
      page,
    }) => {
      await gotoFixture(page, server);
      await setWidth(page, combo.width);
      const view = await openInline(page);
      await setDensity(view, combo.density);
      await readyForCapture(view);
      await hideChromeText(view);

      const bar = page.locator(".atomdown-group-header").first();
      await bar.scrollIntoViewIfNeeded();
      await settle(page, 3);
      await bar.locator(".atomdown-group-menu").click({ force: true });
      await page
        .locator(".atomdown-menu-popover")
        .waitFor({ state: "visible", timeout: 15_000 });
      await page.mouse.move(2, 2);
      await settle(page, 3);

      const pop = page.locator(".atomdown-menu-popover").first();
      await expect(pop).toHaveScreenshot(
        `group-popover-${comboName(combo).replace(/\//g, "-")}.png`,
        SNAPSHOT_OPTS,
      );
    });

    test(`9: the attribute form [${comboName(combo)}]`, async ({ page }) => {
      // The form is a panel iframe, so its own document is what is captured
      // (iugum-etz). It carries the theme it copied from the parent, which is
      // the thing worth photographing: a form that renders a dark theme's
      // fallback light palette is a defect no behavioural rule can see.
      await gotoFixture(page, server);
      await setWidth(page, combo.width);
      const view = await openInline(page);
      await setDensity(view, combo.density);
      await readyForCapture(view);

      const menu = page
        .locator(".sb-decoration-widget.atomdown-card-header .atomdown-card-menu")
        .first();
      await menu.scrollIntoViewIfNeeded();
      await settle(page, 3);
      await menu.click({ force: true });
      await page
        .locator(".atomdown-menu-popover")
        .waitFor({ state: "visible", timeout: 15_000 });
      await page
        .locator(".atomdown-menu-item.atomdown-mi-attrs")
        .first()
        .click({ force: true });

      const panel = page.locator(".sb-modal .sb-panel iframe");
      await panel.waitFor({ state: "visible", timeout: 15_000 });
      const frame = page.frameLocator(".sb-modal .sb-panel iframe");
      await frame.locator("#ad-attr-slug-input").waitFor({
        state: "visible",
        timeout: 15_000,
      });
      // Same font policy as the rest of rule 9: the glyphs go transparent so
      // the fills, borders and radii are what is compared. Inside the frame,
      // because a style tag on the parent cannot reach it.
      await frame.locator("#ad-attr-slug-input").evaluate(() => {
        const s = document.createElement("style");
        s.textContent = `.ad-attr-form, .ad-attr-form * {
          color: transparent !important;
          text-shadow: none !important;
          caret-color: transparent !important;
        }
        .ad-attr-form input::placeholder { color: transparent !important; }`;
        document.head.appendChild(s);
      });
      await settle(page, 3);

      await expect(frame.locator(".ad-attr-form")).toHaveScreenshot(
        `attr-form-${comboName(combo).replace(/\//g, "-")}.png`,
        SNAPSHOT_OPTS,
      );
    });

    test(`9: a card's header row and its gutter controls [${comboName(combo)}]`, async ({
      page,
    }) => {
      await gotoFixture(page, server);
      await setWidth(page, combo.width);
      const view = await openInline(page);
      await setDensity(view, combo.density);
      await readyForCapture(view);
      await hideChromeText(view);

      const header = page
        .locator(".sb-decoration-widget.atomdown-card-header")
        .first();
      await header.scrollIntoViewIfNeeded();
      await page.mouse.move(2, 2);
      await settle(page, 3);

      // A CLIP, NOT AN ELEMENT SCREENSHOT, and that is forced rather than
      // chosen. At compact density the card header widget is a ZERO-HEIGHT
      // layer pinned across the card's top edge - that is what lets the
      // density drop the header row without moving a card - and an element
      // screenshot of a zero-height box never stabilises: Playwright retries
      // "generating new stable screenshot expectation" until it times out.
      //
      // The clip is the card's top strip PLUS both gutters, which is a better
      // subject anyway: the controls now live outside the card, so a box
      // tight to the card would frame out the very thing under test.
      const clip = await clipAroundCard(view);

      await expect(page).toHaveScreenshot(
        `card-header-rest-${comboName(combo).replace(/\//g, "-")}.png`,
        { ...SNAPSHOT_OPTS, clip },
      );

      // HOVERED: both controls appear, in the gutter, outside the card's
      // border. The baseline is what proves they appear THERE and that
      // nothing under them moved - rule 3 proves the layout is stable, this
      // proves it also LOOKS unchanged.
      // HOVER THE CARD'S FIRST LINE, not the header widget. At compact the
      // widget has zero height, so Playwright reports it "outside of the
      // viewport" and refuses to hover it - and the line is the real reveal
      // path anyway: the seam puts the hover class on the card's lines, which
      // is what makes hovering anywhere in a card light its controls.
      const line = page.locator(".cm-line.atomdown-card-line").first();
      await line.hover({ force: true });
      await settle(page, 3);
      await expect(page).toHaveScreenshot(
        `card-header-hover-${comboName(combo).replace(/\//g, "-")}.png`,
        { ...SNAPSHOT_OPTS, clip },
      );
    });

    test(`9: an open popover [${comboName(combo)}]`, async ({ page }) => {
      await gotoFixture(page, server);
      await setWidth(page, combo.width);
      const view = await openInline(page);
      await setDensity(view, combo.density);
      await readyForCapture(view);
      await hideChromeText(view);

      const menu = page
        .locator(".sb-decoration-widget.atomdown-card-header .atomdown-card-menu")
        .first();
      await menu.scrollIntoViewIfNeeded();
      await settle(page, 3);
      await menu.click({ force: true });
      // Wait for the popover itself. The plug's click handler crosses the
      // worker boundary, so it is not bounded by an animation frame.
      await page
        .locator(".atomdown-menu-popover")
        .waitFor({ state: "visible", timeout: 15_000 });
      await page.mouse.move(2, 2);
      await settle(page, 3);

      const pop = page.locator(".atomdown-menu-popover").first();
      await expect(pop).toHaveScreenshot(
        `popover-${comboName(combo).replace(/\//g, "-")}.png`,
        SNAPSHOT_OPTS,
      );
    });
  });
}
