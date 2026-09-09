/**
 * RULE 11 — SELECTION AND CARD HOVER, IN THE INLINE VIEW.
 *
 * THE DEFECT (`iugum-oip`). Steve, on the live page: he could not select a
 * card, could not lasso, and the CARD's border did not change on hover while
 * the GROUP's did. The inline view was read-only in practice.
 *
 * WHY NOTHING NOTICED. The only selection assertion in the suite,
 * `7-components.test.ts` 1b, runs on the BOARD only and says so: "its
 * documented selection gesture is alt-drag rather than a click, so a click
 * assertion there would be asserting a behaviour the plug does not claim."
 * That sentence was the gap. Steve clicks a card to select it in both views,
 * so the inline view does claim it, and the gate covered the board's half of
 * a behaviour both views have.
 *
 * WHAT THIS RULE ASSERTS, and the last one is the guard rail:
 *
 *   11a  A plain click on a card selects THAT card and nothing else.
 *   11b  Modifier-click adds to the selection and removes from it.
 *   11c  Shift-click extends a contiguous range, including a card in the
 *        middle that was never clicked.
 *   11d  A click on empty background clears the selection.
 *   11e  Alt-drag a band over two cards selects both, and "Group selection"
 *        is then enabled: it asks for a name instead of refusing. A group in
 *        the band is refused WITH ITS REASON, because Atomdown Core 1 permits
 *        no nesting.
 *   11f  Hovering a card changes the CARD's own border colour.
 *   11g  Selection still works after the gutter controls have been hovered,
 *        after a popover has been opened and closed, and for a card inside a
 *        group.
 *   11h  THE MOUSEDOWN PATH ITSELF. A plain press on a card body is not
 *        `defaultPrevented` by the time it leaves the editor's content.
 *   11i  None of it writes a document byte.
 *   11j  Dragging across a card's text selects the TEXT, not the card - and a
 *        press with no travel still selects the card.
 *   11k  THE SELECTION IS ALSO VISIBLE. 11j passed for a whole day while the
 *        card surface painted over every selection rectangle, so the text was
 *        selected, the clipboard was right, and the user saw no highlight at
 *        all - only a band in the margins the surface does not reach. Both
 *        surfaces must therefore paint BEHIND CodeMirror's selection layer.
 *
 * 11h IS THE POINT OF THIS FILE. The seam's `widgetPressGuard` calls
 * `preventDefault` on a plain `mousedown` inside a widget, on purpose: a press
 * on widget chrome must not place the text cursor. Selection and the lasso
 * both BEGIN with a mousedown, so a guard that grew to cover the card's own
 * body would kill both — and every end-state assertion above would then fail
 * with no clue as to why. 11h names the mechanism, so the next guard cannot
 * eat the press silently.
 *
 * Both densities and all four editor widths, from the shared matrix: the
 * controls live in the page gutter, whose width moves with the content column,
 * and a card's height moves with the density.
 */

import { test } from "@playwright/test";
import {
  type Combo,
  combos,
  comboName,
  expect,
  failWithArtifacts,
  gotoFixture,
  mod,
  openInline,
  type Page,
  readPageBytes,
  type SBServer,
  setDensity,
  settle,
  setWidth,
  startSpace,
  THEMES,
  type View,
} from "./harness.ts";

let server: SBServer;

test.beforeAll(async () => (server = await startSpace()));
test.afterAll(async () => await server?.stop());

/** A point on one on-screen card, and the text that names it. */
type CardTarget = {
  /** The card's first line of text, so a failure names the card. */
  text: string;
  /** Whether this card sits inside a group. */
  nested: boolean;
  x: number;
  y: number;
};

/**
 * Click points for the cards currently on screen, in document order.
 *
 * THE POINT IS ON THE CARD'S OWN FIRST LINE, a little in from its left edge.
 * Not on the header widget: at compact density the header is a
 * `pointer-events: none` layer with no height of its own, so a pointer never
 * reaches it (the same reason `8-drag-drop.test.ts` hovers the card body to
 * reveal the grip). A card body line is a `.cm-line`, which is where a reader
 * clicks anyway.
 */
async function cardTargets(page: Page): Promise<CardTarget[]> {
  return await page.evaluate(() => {
    const firsts = Array.from(
      document.querySelectorAll(
        ".cm-line.atomdown-card-line.atomdown-card-first",
      ),
    ) as HTMLElement[];
    const out: CardTarget[] = [];
    for (const el of firsts) {
      const r = el.getBoundingClientRect();
      // Fully on screen, clear of the top bar and the bottom edge.
      if (r.top < 120 || r.bottom > globalThis.innerHeight - 80) continue;
      if (r.height < 4) continue;
      out.push({
        text: (el.textContent ?? "").trim().slice(0, 60),
        nested: el.classList.contains("atomdown-group-line"),
        x: +(r.left + Math.min(24, r.width / 3)).toFixed(1),
        y: +(r.top + r.height / 2).toFixed(1),
      });
    }
    return out as unknown as CardTarget[];
  }) as unknown as CardTarget[];
}

/**
 * Click points for the top-level UNITS on screen, in document order.
 *
 * A unit, not a card, and the difference is the point. The plug's selection is
 * per unit: a top-level card is one, and a whole GROUP is one — a card inside
 * a group is not selectable on its own, because the thing that moves and the
 * thing that groups is the unit. The fixture's first three units are the page
 * heading, the intro paragraph and the "decisions" group, so a rule that only
 * counted cards had two targets on the first screen and could not test a
 * three-unit range at all.
 */
async function unitTargets(
  page: Page,
): Promise<(CardTarget & { kind: "card" | "group" })[]> {
  return await page.evaluate(() => {
    const rows = Array.from(
      document.querySelectorAll(
        ".cm-line.atomdown-card-line.atomdown-card-first," +
          ".cm-line.atomdown-group-line.atomdown-group-first",
      ),
    ) as HTMLElement[];
    const out: any[] = [];
    // THE SCROLLER'S OWN RECT IS THE WINDOW, not the viewport with magic
    // margins. CodeMirror scrolls inside `.cm-scroller`, which sits below
    // SilverBullet's top bar, so a line at viewport y=100 can be under the
    // bar at one width and clear of it at another. Measured against the
    // scroller, "on screen" means the same thing at every width.
    const view = document.querySelector(".cm-scroller")!.getBoundingClientRect();
    for (const el of rows) {
      const group = el.classList.contains("atomdown-group-first");
      // A member card is inside a group, so it is not a unit of its own.
      if (!group && el.classList.contains("atomdown-group-line")) continue;
      const r = el.getBoundingClientRect();
      if (r.top < view.top + 4) continue;
      // A group's own first line is its opening marker: a directive, hidden
      // at rest, so it is not a place a pointer can go. The point moves to
      // the first following line with TEXT — not merely the first with a
      // height, because the blank source line between the marker and the
      // first member card has a height and no text, and a click there
      // reports no unit at all.
      let point: HTMLElement | null = el;
      if (group || (el.textContent ?? "").trim() === "") {
        let next: Element | null = el;
        point = null;
        for (let i = 0; i < 12 && next; i++) {
          next = next.nextElementSibling;
          if (!(next instanceof HTMLElement)) continue;
          if (!next.classList.contains("cm-line")) continue;
          if ((next.textContent ?? "").trim() === "") continue;
          if (next.getBoundingClientRect().height < 8) continue;
          point = next;
          break;
        }
      }
      if (!point) continue;
      const pr = point.getBoundingClientRect();
      if (pr.bottom > view.bottom - 4) continue;
      out.push({
        text: (point.textContent ?? "").trim().slice(0, 60),
        nested: false,
        kind: group ? "group" : "card",
        x: +(pr.left + Math.min(24, pr.width / 3)).toFixed(1),
        y: +(pr.top + pr.height / 2).toFixed(1),
      });
    }
    return out;
  }) as any;
}

/**
 * Scroll until at least `need` top-level units are on screen, then return
 * them.
 *
 * THE FIRST SCREEN IS NOT ENOUGH, and that is the fixture rather than a
 * shortcut. It opens with the page heading, the intro paragraph and then an
 * eleven-atom group whose own first line is below the fold at three of the
 * four editor widths, so a rule about a three-unit RANGE has two units to
 * work with unless it moves. Scrolling in editor-height steps also exercises
 * a realised-then-unrealised region, which is where a decoration bug hides.
 */
async function unitsOnScreen(
  page: Page,
  need: number,
): Promise<(CardTarget & { kind: "card" | "group" })[]> {
  // ALIGN FIRST, THEN STEP. The fixture's own page title and the editor's top
  // padding put the first card at y=619 of a 900px viewport, so two units fit
  // on the first screen and the third is under the fold at every width. One
  // scroll that puts the first unit just below the top bar fits three;
  // stepping blind by a screen height overshot the whole group.
  await page.evaluate(() => {
    const first = document.querySelector(
      ".cm-line.atomdown-card-line.atomdown-card-first," +
        ".cm-line.atomdown-group-line.atomdown-group-first",
    );
    if (!first) return;
    const el = document.querySelector(".cm-scroller")!;
    el.scrollTop += first.getBoundingClientRect().top -
      el.getBoundingClientRect().top - 24;
  });
  await settle(page, 6);
  let found = await unitTargets(page);
  for (let step = 0; step < 12 && found.length < need; step++) {
    await page.evaluate(() => {
      const el = document.querySelector(".cm-scroller")!;
      el.scrollTop += Math.round(el.clientHeight * 0.5);
    });
    await settle(page, 6);
    found = await unitTargets(page);
  }
  return found;
}

/**
 * Do two line texts name the same block?
 *
 * NOT `===`, and that is a real property of this editor rather than a
 * looseness. SilverBullet reveals a heading's `#` markers and a list item's
 * marker on the line the text cursor is on, so the same card reads "Running
 * todo" before the click and "# Running todo" after it. Comparing the raw
 * strings made a passing selection look like a failure.
 */
function sameBlock(a: string, b: string): boolean {
  const strip = (s: string) =>
    s.replace(/^[#>*\-\s]+/, "").replace(/^\d+[.)]\s*/, "").trim();
  return strip(a) === strip(b) && strip(a) !== "";
}

/**
 * The selection as the reader sees it: one entry per selected RUN of lines.
 *
 * A run, not a line count. The plug marks a selected unit with
 * `atomdown-selected` and `lineClasses`, so every visible line of that unit
 * carries `atomdown-selected-line` and the blank line between two units
 * carries nothing. So consecutive selected lines are one unit, and the number
 * of runs is the number of selected units — which is the thing every
 * assertion here is about.
 */
async function selectedRuns(page: Page): Promise<string[]> {
  return await page.evaluate(() => {
    const lines = Array.from(
      document.querySelectorAll(".cm-line"),
    ) as HTMLElement[];
    const runs: string[] = [];
    let open = false;
    for (const line of lines) {
      const on = line.classList.contains("atomdown-selected-line");
      if (on && !open) runs.push((line.textContent ?? "").trim().slice(0, 60));
      open = on;
    }
    return runs;
  });
}

/**
 * Wait until the selection holds `want` runs, then return them.
 *
 * NOT A CONVENIENCE. A click reaches the plug as an app event, the plug reads
 * the document, writes a new decoration config and the client rebuilds the
 * decorations from it — several async hops after `mouse.click` resolved. And
 * the rebuild MOVES THE PAGE: the click put the text cursor in the card, which
 * reveals that atom's directive line, and a revealed 64-character digest wraps
 * over three or four rows. A point measured before that landed on the unit
 * ABOVE the one it named, which is how a passing shift-click read as "only the
 * anchor is selected". So every step waits for the selection it asked for
 * before measuring the next point.
 */
async function waitForRuns(
  page: Page,
  want: number,
  timeoutMs = 8000,
): Promise<string[]> {
  const deadline = Date.now() + timeoutMs;
  let runs = await selectedRuns(page);
  while (runs.length !== want && Date.now() < deadline) {
    await page.waitForTimeout(150);
    runs = await selectedRuns(page);
  }
  await settle(page, 3);
  return runs;
}

/**
 * Record every decoration app event the seam dispatches, for the artifacts.
 *
 * A selection failure has two possible causes and they need different fixes:
 * the seam reported the wrong thing, or the plug did the wrong thing with a
 * correct report. Without the payloads a failure cannot say which, and the
 * first two hours of `iugum-oip` went on exactly that ambiguity.
 */
async function recordEvents(page: Page) {
  await page.evaluate(() => {
    const w = globalThis as any;
    w.__adEvents = [];
    if (w.__adRecording) return;
    w.__adRecording = true;
    const client = w.client;
    const original = client.dispatchAppEvent.bind(client);
    client.dispatchAppEvent = function (name: string, ...rest: any[]) {
      if (String(name).indexOf("editor:decoration") === 0) {
        w.__adEvents.push({ name, payload: rest[0] });
      }
      return original(name, ...rest);
    };
    // THE RAW DOM CLICK TOO, so an artifact can tell "the browser never sent
    // a click" apart from "the seam did not report one". They need different
    // fixes and they look identical from the selection alone.
    globalThis.addEventListener("click", (e: MouseEvent) => {
      w.__adEvents.push({
        name: "dom:click",
        payload: {
          shiftKey: e.shiftKey,
          metaKey: e.metaKey,
          prevented: e.defaultPrevented,
          target: (e.target as Element)?.className ?? "",
        },
      });
    }, false);
  });
}

/** The recorded clicks and lassos, trimmed to what a failure needs. */
async function recordedEvents(page: Page) {
  return await page.evaluate(() =>
    ((globalThis as any).__adEvents ?? [])
      .filter((e: any) => e.name !== "editor:decorationSelect")
      .map((e: any) => ({
        name: e.name,
        line: e.payload?.line,
        marks: e.payload?.marks,
        target: e.payload?.target,
        prevented: e.payload?.prevented,
        shiftKey: e.payload?.shiftKey,
        metaKey: e.payload?.metaKey,
        ctrlKey: e.payload?.ctrlKey,
        altKey: e.payload?.altKey,
      }))
  );
}

/**
 * Click the Nth top-level unit, re-measuring first, and retry a LOST click.
 *
 * THE PAGE MOVES BETWEEN THE PRESS AND THE RELEASE, and Chromium then sends
 * no `click` at all — it only fires one when the press and the release land
 * on the same element. The press puts the text cursor in the card, the plug
 * reveals that atom's directive line in response, and a 64-character digest
 * wrapping over three rows pushes the line out from under the pointer while
 * the button is still down. Measured: the shift-click of rule 11c produced no
 * DOM click event whatever, four cells out of four, while every other click
 * in the same test arrived.
 *
 * So a click is retried, but ONLY when the selection did not change at all —
 * which is what a lost click looks like. A click that changed the selection to
 * the wrong thing is a real failure and is returned as it is, because
 * retrying a modifier-click would toggle it back and hide the defect.
 */
async function clickUnit(
  page: Page,
  index: number,
  modifiers: ("Alt" | "Control" | "Meta" | "Shift")[],
  want: number,
): Promise<string[]> {
  const before = (await selectedRuns(page)).length;
  let runs = await selectedRuns(page);
  for (let attempt = 0; attempt < 3; attempt++) {
    const now = await unitTargets(page);
    if (!now[index]) break;
    await clickAt(page, now[index], modifiers);
    runs = await waitForRuns(page, want, 3000);
    if (runs.length === want) return runs;
    if (runs.length !== before) return runs;
  }
  return runs;
}

/** Move the pointer somewhere no card is, so no hover state is left behind. */
async function parkPointer(page: Page) {
  await page.mouse.move(4, 4);
  await settle(page);
}

/** Click at a point and let the plug's redraw land. */
async function clickAt(
  page: Page,
  at: { x: number; y: number },
  modifiers: ("Alt" | "Control" | "Meta" | "Shift")[] = [],
) {
  for (const m of modifiers) await page.keyboard.down(m);
  await page.mouse.click(at.x, at.y);
  for (const m of modifiers.slice().reverse()) await page.keyboard.up(m);
  // A FIXED WAIT, and it is not laziness. A click travels to the plug as an
  // app event, the plug re-reads the document and writes a new decoration
  // config, and the client rebuilds from it — and the rebuild MOVES THE PAGE,
  // because the click also put the text cursor in the card and that reveals
  // the atom's directive line, which wraps over three or four rows. There is
  // no DOM condition that says "the plug has finished", and every point this
  // rule measures afterwards is a point in the new layout. Frames are not
  // enough: `settle` returned while the config write was still in flight, and
  // the next click then landed one unit above the one it named.
  await page.waitForTimeout(300);
  await settle(page, 4);
}

/** Sweep an alt-held band from one point to another. */
async function altBand(
  page: Page,
  from: { x: number; y: number },
  to: { x: number; y: number },
) {
  await page.keyboard.down("Alt");
  await page.mouse.move(from.x, from.y);
  await page.mouse.down();
  for (let i = 1; i <= 6; i++) {
    await page.mouse.move(
      from.x + ((to.x - from.x) * i) / 6,
      from.y + ((to.y - from.y) * i) / 6,
      { steps: 2 },
    );
  }
  await page.mouse.up();
  await page.keyboard.up("Alt");
  await settle(page, 4);
}

/** Whatever the notification bar is saying right now. */
async function notices(page: Page): Promise<string[]> {
  return await page.evaluate(() =>
    Array.from(document.querySelectorAll(".sb-notifications *"))
      .map((n) => (n.textContent ?? "").trim())
      .filter((t) => t !== "")
  );
}

/** Dismiss whatever notification is up, so the next read is fresh. */
async function clearNotices(page: Page) {
  await page.evaluate(() => {
    document
      .querySelectorAll<HTMLElement>(".sb-notification-dismiss")
      .forEach((b) => b.click());
  });
  await settle(page);
}

/**
 * Open one card's three-dot popover.
 *
 * The control is in the right-hand page gutter and hidden at rest, so the
 * pointer goes on the card first to reveal it. Returns the popover locator.
 */
async function openCardMenu(view: View, index: number) {
  const line = view.page
    .locator(".cm-line.atomdown-card-line.atomdown-card-first")
    .nth(index);
  const box = await line.boundingBox({ timeout: 10_000 });
  expect(box, "the card has no rect to hover").not.toBeNull();
  await view.page.mouse.move(box!.x + 24, box!.y + box!.height / 2);
  await settle(view.page);
  const button = view.page.locator(".atomdown-card-menu").nth(index);
  const bb = await button.boundingBox({ timeout: 10_000 });
  expect(bb, "the card's three-dot control has no rect").not.toBeNull();
  await view.page.mouse.click(bb!.x + bb!.width / 2, bb!.y + bb!.height / 2);
  await settle(view.page, 4);
  return view.page.locator(".atomdown-menu-popover").first();
}

for (const theme of THEMES) {
  test.describe(`theme=${theme}`, () => {
    test.use({ colorScheme: theme });

    for (const combo of combos().filter((c) => c.theme === theme)) {
      // ---------------------------------------------------------------- 11a
      // ---------------------------------------------------------------- 11b
      // ---------------------------------------------------------------- 11c
      // ---------------------------------------------------------------- 11d
      test(`11a-d: click selects one card, modifier adds and removes, shift extends, background clears [${comboName(combo)}]`, async ({
        page,
      }) => {
        await gotoFixture(page, server);
        await setWidth(page, combo.width);
        const view = await openInline(page);
        await setDensity(view, combo.density);
        await recordEvents(page);
        const before = await readPageBytes(server);

        const targets = await unitsOnScreen(page, 3);
        if (targets.length < 3) {
          await failWithArtifacts(
            page,
            10,
            "11a-d - the fixture must show three units",
            combo,
            { targets },
            `need at least three top-level units on screen; found ` +
              `${targets.length}`,
          );
        }
        const first = targets[0];

        // 11a — a plain click selects that unit, and only that unit.
        let runs = await clickUnit(page, 0, [], 1);
        if (runs.length !== 1 || !sameBlock(runs[0], first.text)) {
          await failWithArtifacts(
            page,
            10,
            "11a - a plain click selects one card",
            combo,
            {
              clicked: first,
              selectedRuns: runs,
              events: await recordedEvents(page),
            },
            `a plain click on a card must select exactly that card. ` +
              `Clicked "${first.text}"; ${runs.length} run(s) selected.`,
          );
        }

        // 11b — modifier-click adds, then removes again.
        const modKey = mod === "Meta" ? "Meta" : "Control";
        runs = await clickUnit(page, 1, [modKey], 2);
        if (runs.length !== 2) {
          await failWithArtifacts(
            page,
            10,
            "11b - modifier-click adds to the selection",
            combo,
            { selectedRuns: runs, events: await recordedEvents(page) },
            `${mod}-click must ADD to the selection; ${runs.length} run(s) ` +
              `selected.`,
          );
        }
        runs = await clickUnit(page, 1, [modKey], 1);
        if (runs.length !== 1) {
          await failWithArtifacts(
            page,
            10,
            "11b - modifier-click removes from the selection",
            combo,
            { selectedRuns: runs, events: await recordedEvents(page) },
            `${mod}-click again must REMOVE it; ${runs.length} run(s) ` +
              `selected.`,
          );
        }

        // 11c — shift-click extends a contiguous range. The MIDDLE unit is
        // never clicked, which is the whole property.
        await clickUnit(page, 0, [], 1);
        runs = await clickUnit(page, 2, ["Shift"], 3);
        if (runs.length !== 3) {
          const seen = await unitTargets(page);
          await failWithArtifacts(
            page,
            10,
            "11c - shift-click extends a contiguous range",
            combo,
            {
              units: seen,
              selectedRuns: runs,
              events: await recordedEvents(page),
            },
            `shift-click must extend a contiguous range of three units; ` +
              `${runs.length} run(s) selected.`,
          );
        }

        // 11d — a click on empty background clears it. The background is the
        // page's own left margin: inside the editor's scroller, outside the
        // content column, so it is on no card and on no text. The y comes
        // from the SCROLLER's visible rect, not from the content element's —
        // a scrolled `.cm-content` has a negative top, and a point 200px
        // below that is off the top of the window.
        const margin = await page.evaluate(() => {
          const content = document.querySelector(".cm-content")!
            .getBoundingClientRect();
          const scroller = document.querySelector(".cm-scroller")!
            .getBoundingClientRect();
          return {
            x: +((scroller.left + content.left) / 2).toFixed(1),
            y: +(scroller.top + scroller.height / 2).toFixed(1),
            room: +(content.left - scroller.left).toFixed(1),
          };
        });
        expect(margin.room, "no page margin to click in").toBeGreaterThan(8);
        await clickAt(page, margin);
        runs = await waitForRuns(page, 0);
        if (runs.length !== 0) {
          await failWithArtifacts(
            page,
            10,
            "11d - a click on empty background clears the selection",
            combo,
            { margin, selectedRuns: runs, events: await recordedEvents(page) },
            `a click on empty background must clear the selection; ` +
              `${runs.length} run(s) still selected.`,
          );
        }

        // 11i — none of the above wrote a byte.
        expect(
          await readPageBytes(server),
          "selecting must not change one document byte",
        ).toBe(before);
        await view.close();
      });

      // ---------------------------------------------------------------- 11e
      test(`11e: alt-drag selects the band, Group is enabled, a nested group is refused [${comboName(combo)}]`, async ({
        page,
      }) => {
        await gotoFixture(page, server);
        await setWidth(page, combo.width);
        const view = await openInline(page);
        await setDensity(view, combo.density);
        const before = await readPageBytes(server);

        const targets = (await cardTargets(page)).filter((t) => !t.nested);
        expect(targets.length).toBeGreaterThanOrEqual(2);

        await altBand(
          page,
          { x: targets[0].x, y: targets[0].y },
          { x: targets[1].x + 40, y: targets[1].y },
        );
        let runs = await selectedRuns(page);
        if (runs.length < 2) {
          await failWithArtifacts(
            page,
            10,
            "11e - alt-drag selects the band",
            combo,
            { band: [targets[0], targets[1]], selectedRuns: runs },
            `an alt-drag over two cards must select both; ` +
              `${runs.length} run(s) selected.`,
          );
        }

        // GROUP IS ENABLED, asserted without writing: the action asks for a
        // name through `editor.prompt` instead of flashing its refusal.
        await clearNotices(page);
        const popover = await openCardMenu(view, 0);
        await popover.locator(".atomdown-mi-group").first().click();
        await settle(page, 4);
        const prompt = page.locator(".sb-prompt");
        const asked = await prompt.count();
        const said = await notices(page);
        if (asked === 0) {
          await failWithArtifacts(
            page,
            10,
            "11e - Group is enabled once two cards are selected",
            combo,
            { selectedRuns: runs, notifications: said },
            `with two cards selected, Group must be enabled and ask for a ` +
              `name. No prompt appeared.`,
          );
        }
        await page.keyboard.press("Escape");
        await settle(page, 4);

        // A GROUP IN THE SELECTION IS REFUSED WITH THE REASON. Atomdown Core
        // 1 permits no nesting, and a refusal with no reason reads as broken.
        const withGroup = await cardTargets(page);
        const groupLine = await page.evaluate(() => {
          const el = document.querySelector(
            ".cm-line.atomdown-group-line",
          ) as HTMLElement | null;
          if (!el) return null;
          const r = el.getBoundingClientRect();
          return { x: +(r.left + 24).toFixed(1), y: +(r.top + 4).toFixed(1) };
        });
        expect(groupLine, "no group on screen to band over").not.toBeNull();
        await altBand(
          page,
          { x: withGroup[0].x, y: withGroup[0].y },
          { x: groupLine!.x + 40, y: groupLine!.y + 20 },
        );
        await clearNotices(page);
        const popover2 = await openCardMenu(view, 0);
        await popover2.locator(".atomdown-mi-group").first().click();
        await settle(page, 4);
        const refusal = (await notices(page)).join(" | ");
        expect(
          refusal,
          "a group inside a group must be refused with its reason shown",
        ).toContain("group inside a group");

        expect(
          await readPageBytes(server),
          "lassoing and a refused Group must not change one document byte",
        ).toBe(before);
        await view.close();
      });

      // ---------------------------------------------------------------- 11f
      test(`11f: hovering a card changes the CARD's own border [${comboName(combo)}]`, async ({
        page,
      }) => {
        await gotoFixture(page, server);
        await setWidth(page, combo.width);
        const view = await openInline(page);
        await setDensity(view, combo.density);
        const before = await readPageBytes(server);

        const targets = (await cardTargets(page)).filter((t) => !t.nested);
        expect(targets.length).toBeGreaterThanOrEqual(1);

        // THE BORDER IS ON A ::before, so it is read through the pseudo
        // element. Reading the line's own `borderLeftColor` measures nothing:
        // the line has no border of its own, and that is why a class-only
        // assertion here would pass on a card that never changes.
        const cardEdge = async () =>
          await page.evaluate(() => {
            const el = document.querySelector(
              ".cm-line.atomdown-card-line.atomdown-card-first:not(.atomdown-group-line)",
            )!;
            const s = getComputedStyle(el, "::before");
            return {
              color: s.borderLeftColor,
              width: s.borderLeftWidth,
            };
          });

        await parkPointer(page);
        const rest = await cardEdge();
        await page.mouse.move(targets[0].x, targets[0].y);
        await settle(page, 4);
        const hot = await cardEdge();

        if (rest.color === hot.color) {
          await failWithArtifacts(
            page,
            10,
            "11f - hover changes the CARD's own border",
            combo,
            { rest, hovered: hot },
            `hovering a card must change the CARD's own border colour. ` +
              `At rest ${rest.color} @${rest.width}; hovered ${hot.color} ` +
              `@${hot.width}.`,
          );
        }

        expect(
          await readPageBytes(server),
          "hovering must not change one document byte",
        ).toBe(before);
        await view.close();
      });

      // ---------------------------------------------------------------- 11g
      test(`11g: selection survives the gutter controls, a popover, and a card inside a group [${comboName(combo)}]`, async ({
        page,
      }) => {
        await gotoFixture(page, server);
        await setWidth(page, combo.width);
        const view = await openInline(page);
        await setDensity(view, combo.density);
        const before = await readPageBytes(server);

        const targets = await cardTargets(page);
        const top = targets.filter((t) => !t.nested);
        const nested = targets.filter((t) => t.nested);
        expect(top.length).toBeGreaterThanOrEqual(1);

        // After the gutter controls have been hovered. The grip and the menu
        // both live outside the card now, and hovering them is what a reader
        // does immediately before clicking the card.
        const grip = view.page.locator(".atomdown-grip").first();
        const gb = await grip.boundingBox({ timeout: 10_000 }).catch(() => null);
        if (gb) {
          await page.mouse.move(gb.x + gb.width / 2, gb.y + gb.height / 2);
          await settle(page);
        }
        await clickAt(page, top[0]);
        expect(
          (await selectedRuns(page)).length,
          "a click after hovering the gutter controls must still select",
        ).toBe(1);

        // After a popover has been opened and closed.
        const popover = await openCardMenu(view, 0);
        await expect(popover).toBeVisible();
        await clickAt(page, top[0]); // the first click closes the popover
        await clickAt(page, top[0]);
        expect(
          (await selectedRuns(page)).length,
          "a click after opening and closing a popover must still select",
        ).toBe(1);

        // A card inside a group.
        if (nested.length > 0) {
          await clickAt(page, nested[0]);
          const runs = await selectedRuns(page);
          expect(
            runs.length,
            `a click on a card inside a group must select something; got ` +
              JSON.stringify(runs),
          ).toBeGreaterThanOrEqual(1);
        }

        expect(
          await readPageBytes(server),
          "none of this may change a document byte",
        ).toBe(before);
        await view.close();
      });

      // ---------------------------------------------------------------- 11h
      test(`11h: a plain mousedown on a card body is not swallowed [${comboName(combo)}]`, async ({
        page,
      }) => {
        await gotoFixture(page, server);
        await setWidth(page, combo.width);
        const view = await openInline(page);
        await setDensity(view, combo.density);

        // WHAT THIS MEASURES, AND WHY IT IS NOT `defaultPrevented`.
        //
        // The obvious assertion is "the press is not `defaultPrevented`", and
        // it is the wrong one. Measured on this fixture
        // (`press-probe.test.ts`): a plain mousedown reaches `window` in the
        // bubble phase ALREADY prevented in every case — on a card body, on
        // an undecorated line, and with the inline view switched off
        // altogether. That is CodeMirror's own `runHandlers`, which calls
        // `preventDefault` whenever a handler returns true, and one of its
        // built-in mousedown handlers always does. So `defaultPrevented`
        // cannot tell a guarded press from an ordinary one, and a rule built
        // on it would fail on a healthy editor.
        //
        // THE CURSOR IS THE GUARD'S SIGNATURE. `widgetPressGuard` exists to
        // stop a press placing the text cursor — that is its whole stated
        // purpose, and `DecorationWidget.movesCursor` is where it decides. So
        // a press the guard claims leaves the cursor where it was, and a
        // press it lets through puts the cursor on the pressed line. The
        // cursor's line carries `cm-activeLine`, so the DOM answers it — and
        // it is read during the MOUSEDOWN, before the release, so what the
        // click did afterwards cannot stand in for the press.
        const activeLine = async () =>
          await page.evaluate(() => {
            const el = document.querySelector(".cm-line.cm-activeLine");
            if (!el) return null;
            return {
              text: (el.textContent ?? "").trim().slice(0, 60),
              onCard: el.classList.contains("atomdown-card-line"),
            };
          });

        // Put the cursor somewhere that is NOT a card first, so "the cursor
        // is on the card" cannot already be true before the press.
        const away = await page.evaluate(() => {
          const el = Array.from(document.querySelectorAll(".cm-line"))
            .find((l) =>
              !l.classList.contains("atomdown-card-line") &&
              l.getBoundingClientRect().height > 4
            ) as HTMLElement | undefined;
          if (!el) return null;
          const r = el.getBoundingClientRect();
          return {
            x: +(r.left + 20).toFixed(1),
            y: +(r.top + r.height / 2).toFixed(1),
          };
        });
        if (away) await clickAt(page, away);

        const targets = (await cardTargets(page)).filter((t) => !t.nested);
        expect(targets.length).toBeGreaterThanOrEqual(1);

        await parkPointer(page);
        await page.mouse.move(targets[0].x, targets[0].y);
        await page.mouse.down();
        await settle(page, 2);
        const onPress = await activeLine();
        await page.mouse.up();
        await settle(page, 4);

        if (!onPress || !onPress.onCard) {
          await failWithArtifacts(
            page,
            10,
            "11h - the mousedown path itself",
            combo,
            { activeLineAtPress: onPress, pressed: targets[0] },
            `a plain mousedown on a card BODY must reach the editor and ` +
              `place the cursor there. It did not, which is what a widget ` +
              `press guard does - see widgetPressGuard and ` +
              `DecorationWidget.movesCursor in ` +
              `silverbullet/client/codemirror/decoration_seam.ts. Pressed ` +
              `"${targets[0].text}"; the cursor ended on ` +
              `${onPress ? `"${onPress.text}"` : "no line at all"}.`,
          );
        }

        // AND THE GESTURE THE PRESS BEGINS FINISHED. Both halves matter: the
        // press reaching the editor with nothing selected is the plug
        // ignoring it, which is the other half of this defect.
        const runs = await selectedRuns(page);
        if (runs.length !== 1 || !sameBlock(runs[0], targets[0].text)) {
          await failWithArtifacts(
            page,
            10,
            "11h - the press must end in a selection",
            combo,
            { pressed: targets[0], selectedRuns: runs },
            `the press reached the editor but selected the wrong thing: ` +
              `${runs.length} run(s) selected.`,
          );
        }
        await view.close();
      });
      // ---------------------------------------------------------------- 11j
      test(`11j: dragging across a card's text selects the TEXT, not the card [${comboName(combo)}]`, async ({
        page,
      }) => {
        await gotoFixture(page, server);
        await setWidth(page, combo.width);
        const view = await openInline(page);
        await setDensity(view, combo.density);
        await recordEvents(page);
        const before = await readPageBytes(server);

        const targets = await unitsOnScreen(page, 2);
        const card = targets.find((t) => t.kind === "card");
        expect(card, "need a top-level card on screen").toBeTruthy();

        // A LINE WITH ENOUGH TEXT TO DRAG ACROSS. The heading is one short
        // word at some widths, and a drag shorter than the 4px travel
        // threshold is a click, which would make this test assert the
        // opposite of what it means to.
        const line = await page.evaluate(() => {
          const el = (Array.from(
            document.querySelectorAll(
              ".cm-line.atomdown-card-line",
            ),
          ) as HTMLElement[]).find((l) => {
            const r = l.getBoundingClientRect();
            const viewRect = document.querySelector(".cm-scroller")!
              .getBoundingClientRect();
            return (l.textContent ?? "").trim().length > 30 &&
              r.top > viewRect.top + 4 && r.bottom < viewRect.bottom - 4;
          });
          if (!el) return null;
          const r = el.getBoundingClientRect();
          return {
            text: (el.textContent ?? "").trim().slice(0, 40),
            y: +(r.top + r.height / 2).toFixed(1),
            from: +(r.left + 8).toFixed(1),
            to: +(r.left + Math.min(240, r.width - 8)).toFixed(1),
          };
        });
        expect(line, "no card line long enough to drag across").not.toBeNull();

        await parkPointer(page);
        await page.mouse.move(line!.from, line!.y);
        await page.mouse.down();
        for (let i = 1; i <= 8; i++) {
          await page.mouse.move(
            line!.from + ((line!.to - line!.from) * i) / 8,
            line!.y,
            { steps: 2 },
          );
        }
        await page.mouse.up();
        await settle(page, 4);
        await page.waitForTimeout(400);

        // THE BROWSER'S OWN SELECTION HOLDS TEXT, and it is text from the
        // card that was dragged across.
        const picked = await page.evaluate(() => {
          const sel = globalThis.getSelection();
          const node = sel?.anchorNode ?? null;
          const el = node instanceof Element ? node : node?.parentElement;
          return {
            text: (sel?.toString() ?? "").trim(),
            collapsed: sel?.isCollapsed ?? true,
            inCard: el?.closest(".cm-line.atomdown-card-line") != null,
          };
        });
        const runs = await selectedRuns(page);
        if (picked.text.length < 3 || picked.collapsed || runs.length !== 0) {
          await failWithArtifacts(
            page,
            10,
            "11j - a text drag selects text, not the card",
            combo,
            {
              line,
              picked,
              selectedRuns: runs,
              events: await recordedEvents(page),
            },
            `dragging across a card's text must select THAT TEXT and must ` +
              `not select the card. Selected text ` +
              `${JSON.stringify(picked.text.slice(0, 40))}; ` +
              `${runs.length} card run(s) selected.`,
          );
        }
        expect(
          picked.inCard,
          "the selection must be inside the card the drag crossed",
        ).toBe(true);

        // A PRESS WITH NO TRAVEL STILL SELECTS THE CARD. Both halves, in one
        // test, because a fix that killed card selection to save text
        // selection would pass either half alone.
        const after = await clickUnit(page, 0, [], 1);
        expect(
          after.length,
          `a press with no travel must still select the card; got ` +
            JSON.stringify(after),
        ).toBe(1);

        expect(
          await readPageBytes(server),
          "selecting text must not change one document byte",
        ).toBe(before);
        await view.close();
      });
      // ---------------------------------------------------------------- 11k
      test(`11k: the selection highlight paints in front of the card surface [${comboName(combo)}]`, async ({
        page,
      }) => {
        await gotoFixture(page, server);
        await setWidth(page, combo.width);
        const view = await openInline(page);
        await setDensity(view, combo.density);

        // Z-ORDER, NOT A PIXEL COMPARE. The card surface, the group surface
        // and CodeMirror's selection layer all sit at negative z-index in one
        // shared stacking context, so their order alone decides whether a
        // highlight is visible - and reading the three numbers says WHICH one
        // covered the selection when this fails. A screenshot diff would only
        // say that something changed colour.
        const z = await page.evaluate(() => {
          const num = (v: string) => v === "auto" ? 0 : Number(v);
          const cardEl = document.querySelector(
            ".cm-line.atomdown-card-line",
          );
          const groupEl = document.querySelector(
            ".cm-line.atomdown-group-line",
          );
          const layerEl = document.querySelector(".cm-selectionLayer");
          return {
            card: cardEl
              ? num(getComputedStyle(cardEl, "::before").zIndex)
              : null,
            group: groupEl
              ? num(getComputedStyle(groupEl, "::after").zIndex)
              : null,
            layer: layerEl ? num(getComputedStyle(layerEl).zIndex) : null,
          };
        });

        expect(z.card, "no card line on screen to measure").not.toBeNull();
        expect(
          z.layer,
          "CodeMirror draws no selection layer, so this rule cannot be " +
            "measured - the client's selection drawing changed",
        ).not.toBeNull();

        expect(
          z.card! < z.layer!,
          `the card surface must paint behind the selection layer, or a text ` +
            `selection is invisible on the card's own text. ` +
            `card ::before z-index ${z.card}, ` +
            `.cm-selectionLayer z-index ${z.layer}.`,
        ).toBe(true);

        if (z.group !== null) {
          expect(
            z.group! < z.layer!,
            `the group surface must paint behind the selection layer too. ` +
              `group ::after z-index ${z.group}, ` +
              `.cm-selectionLayer z-index ${z.layer}.`,
          ).toBe(true);

          // AND STILL BEHIND THE CARD, so a selected member card's own
          // surface is not painted over by the group's.
          expect(
            z.group! < z.card!,
            `the group surface must stay behind the card surface. ` +
              `group ${z.group}, card ${z.card}.`,
          ).toBe(true);
        }
        await view.close();
      });
    }
  });
}

// Referenced so the type import is not unused when the matrix is filtered.
export type { Combo };
