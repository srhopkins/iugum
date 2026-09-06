/**
 * RULE 4 — STATE MACHINE ROUND TRIPS.
 *
 * Every toggle in these two views is a state machine, and a state machine that
 * does not return to where it started is the defect class Steve hit three
 * separate times in one evening.
 *
 * For collapse/expand, view on/off, raw/rendered, density, and the four editor
 * widths from `Library/Styles/EditorWidth.md`, this captures a DOM signature —
 * class lists, box rects, visible text — toggles away, toggles back, and
 * asserts the signature is identical. Then it checks reload persistence: on
 * stays on, off stays off, scoped per page.
 *
 * THE DEFECTS THIS REPRODUCES:
 *
 *   - A group that would not expand after collapse. The forward transition
 *     worked, the reverse did not, and every unit test over the fold
 *     calculation passed because the calculation was right — the state was
 *     wrong. A signature comparison is the only thing that sees this: it
 *     compares the state you came back to against the state you left.
 *   - The header toggle button doing nothing on first press while the command
 *     worked. Two entry points to one state machine, one of them out of step.
 *     This is why the round trips are driven through the REAL buttons and the
 *     REAL command, not by writing clientStore directly.
 *   - Close-then-reload reopening the board. The close path forgot the page
 *     after hiding the panel instead of before, so the restore event fired on
 *     stale state. Only a reload after a close catches it, which is why the
 *     persistence half exists and why it asserts OFF stays off — the easy
 *     half to leave out.
 */

import { test } from "@playwright/test";
import {
  boardFrame,
  boardViewMode,
  type Combo,
  combos,
  comboName,
  expect,
  failWithArtifacts,
  FIXTURE,
  gotoFixture,
  inlineDensity,
  openBoard,
  openInline,
  runCommand,
  type SBServer,
  setDensity,
  settle,
  setWidth,
  signature,
  signatureDiff,
  startSpace,
  sweepEach,
  THEMES,
  type View,
  type Width,
  WIDTH_PX,
  WIDTHS,
} from "./harness.ts";

let server: SBServer;
test.beforeAll(async () => (server = await startSpace()));
test.afterAll(async () => await server?.stop());

/**
 * Toggle away, toggle back, and require the signature to be byte-identical.
 *
 * `away` and `back` are separate closures rather than one "toggle twice"
 * because the two directions are different code paths and the whole point is
 * that they can disagree.
 */
async function assertRoundTrip(
  view: View,
  combo: Combo,
  what: string,
  away: () => Promise<void>,
  back: () => Promise<void>,
) {
  const before = await signature(view);
  expect(
    before.count,
    `${what}: the signature was empty, so this round trip measured nothing`,
  ).toBeGreaterThan(0);

  await away();
  const middle = await signature(view);
  // The state must actually CHANGE on the way out. Without this, a toggle that
  // silently does nothing round-trips perfectly and passes.
  if (
    middle.count === before.count &&
    middle.entries.join("\n") === before.entries.join("\n")
  ) {
    await failWithArtifacts(
      view.page,
      4,
      "state round trip — the toggle did nothing",
      combo,
      { what, count: before.count },
      `${what}: toggling away left the DOM byte-identical. Either the toggle ` +
        `is inert — the header button that did nothing on first press while ` +
        `the command worked — or this test is driving the wrong control.`,
    );
  }

  await back();
  const same = (a: typeof before, b: typeof before) =>
    a.count === b.count && a.entries.join("\n") === b.entries.join("\n");

  // RE-MEASURED ONCE BEFORE IT IS REPORTED, and that stabilises the
  // MEASUREMENT rather than relaxing the assertion.
  //
  // A signature is a sweep over a virtualised editor whose syntax tree is
  // parsed INCREMENTALLY. A sweep can reach the far end of the document
  // before Lezer has: those lines come back with their `sb-line-h3` and
  // `sb-blockquote-outside` classes missing and raw `##`, `>` and
  // `[label](url)` markers in their text, and a signature keys on exactly
  // class list and text. Measured: 307 entries against 415 for the same
  // correct state, with every difference of that shape. The same class of
  // artefact `signature` already re-reads `scrollHeight` for.
  //
  // A REAL failure is a property of the state, so it is still there on the
  // second reading - the state after `back()` does not change while it is
  // measured. Nothing about the comparison is loosened: it is still
  // byte-identical entries or a failure.
  let after = await signature(view);
  let reread = false;
  if (!same(before, after)) {
    reread = true;
    after = await signature(view);
  }
  if (!same(before, after)) {
    await failWithArtifacts(
      view.page,
      4,
      "state round trip — coming back did not restore the state",
      combo,
      {
        what,
        reread,
        diff: signatureDiff(before, after),
        before: before.count,
        after: after.count,
      },
      `${what}: the DOM after the round trip differs from the DOM before it` +
        (reread ? ", on two consecutive readings" : "") + ". " +
        signatureDiff(before, after).slice(0, 4).join(" / "),
    );
  }
}

for (const theme of THEMES) {
  test.describe(`theme=${theme}`, () => {
    test.use({ colorScheme: theme });

    for (const combo of combos().filter((c) => c.theme === theme)) {
      test(`inline: collapse, view and width all round-trip [${comboName(combo)}]`, async ({
        page,
      }) => {
        await gotoFixture(page, server);
        await setWidth(page, combo.width);
        let view = await openInline(page);
        await setDensity(view, combo.density);

        // --- Collapse and expand, every group ----------------------------
        // Driven through the header caret, which is the control that was out
        // of step with the command.
        await assertRoundTrip(
          view,
          combo,
          "collapse every group, then expand every group",
          async () => {
            const collapsed = await sweepEach(
              view,
              ".atomdown-group-collapse",
              async (caret) => {
                await caret.click();
                await settle(page, 4);
              },
            );
            expect(collapsed.length).toBe(FIXTURE.groups);
          },
          async () => {
            // ASSERT THE COUNT ON THE WAY BACK TOO. Without it a sweep that
            // expanded nine of eleven groups returned quietly and the failure
            // arrived as an unexplained signature diff rather than as "the
            // sweep reached nine carets".
            const expanded = await sweepEach(
              view,
              ".atomdown-group-collapse",
              async (caret) => {
                await caret.click();
                await settle(page, 4);
              },
            );
            expect(expanded.length).toBe(FIXTURE.groups);
          },
        );

        // --- The density, both ways ---------------------------------------
        // Presentational, so the DOM has to come back byte-identical. It is
        // also the half of "presentational" a signature can prove: rule 6
        // proves the other half, that the document's bytes never move.
        const otherDens = combo.density === "compact"
          ? "comfortable"
          : "compact";
        await assertRoundTrip(
          view,
          combo,
          `inline density ${combo.density} -> ${otherDens} -> ${combo.density}`,
          async () => await setDensity(view, otherDens),
          async () => await setDensity(view, combo.density),
        );

        // --- The density through the header BUTTON -------------------------
        // The other entry point, and the one that used to arrive only when
        // the client's page index happened to hold the library page. The plug
        // registers it now, so it is present whenever the command is.
        const densityButton = page.locator(
          'button[title*="Atomdown display density"]',
        );
        // WAITED FOR, NOT COUNTED. The plug appends the button to the
        // `actionButtons` config, and Space Lua's own reload clears that key
        // when the first index of a space completes — so the plug re-asserts
        // it on a heartbeat and the icon can be a few seconds behind a fresh
        // load. A count taken immediately is a race, not an assertion.
        await densityButton.first().waitFor({
          state: "visible",
          timeout: 30_000,
        });
        await assertRoundTrip(
          view,
          combo,
          "switch the inline density with the header button, twice",
          async () => {
            await densityButton.first().click();
            await page
              .locator(`.cm-line.atomdown-${otherDens}-line`)
              .first()
              .waitFor({ state: "attached", timeout: 10_000 });
            await settle(page, 4);
          },
          async () => {
            await densityButton.first().click();
            await page
              .locator(`.cm-line.atomdown-${combo.density}-line`)
              .first()
              .waitFor({ state: "attached", timeout: 10_000 });
            await settle(page, 4);
          },
        );

        // --- Reload persistence: the density survives ---------------------
        await setDensity(view, otherDens);
        await gotoFixture(page, server);
        await page
          .locator(".atomdown-card-line")
          .first()
          .waitFor({ state: "attached", timeout: 15_000 });
        await settle(page, 6);
        expect(
          await inlineDensity(view),
          `the density is remembered per page, so ${otherDens} must survive a reload`,
        ).toBe(otherDens);
        await setDensity(view, combo.density);
        // THE EDITOR WIDTH DOES NOT SURVIVE A RELOAD, and the width round
        // trips are next. `setWidth` writes `html[data-editor-width]`, which
        // a page load resets to the default, so without this the width loop
        // took its "before" signature at the default width and its "after"
        // at this cell's width and reported 292 elements gone.
        await setWidth(page, combo.width);
        await settle(page, 4);

        // --- The view off and on, through the command --------------------
        await assertRoundTrip(
          view,
          combo,
          "turn the inline view off and on with the command",
          async () => {
            await runCommand(page, "Atomdown: Toggle Inline View");
            await page
              .locator(".atomdown-card-line")
              .first()
              .waitFor({ state: "detached", timeout: 10_000 });
          },
          async () => {
            view = await openInline(page);
          },
        );

        // --- The view off and on, through the header BUTTON --------------
        // The other entry point, and the one that did nothing on first press
        // while the command worked. Named by its own description rather than
        // by "Atomdown": there are TWO Atomdown buttons now, and `.first()`
        // over both would silently start driving the density switch if their
        // registration order ever changed.
        const button = page.locator(
          'button[title*="Atomdown card view"], button[aria-label*="Atomdown card view"]',
        );
        // NO LONGER CONDITIONAL. It used to be `if (await button.count())`,
        // which skipped silently whenever the library page's `space-lua` had
        // not reached this browser's page index — so the very failure this
        // guards was the one it could not see. The plug registers the button
        // now, so it is always there, and this waits for it like the density
        // one above.
        await button.first().waitFor({ state: "visible", timeout: 30_000 });
        {
          await assertRoundTrip(
            view,
            combo,
            "turn the inline view off and on with the header button",
            async () => {
              await button.first().click();
              await page
                .locator(".atomdown-card-line")
                .first()
                .waitFor({ state: "detached", timeout: 10_000 });
            },
            async () => {
              await button.first().click();
              await page
                .locator(".atomdown-card-line")
                .first()
                .waitFor({ state: "attached", timeout: 15_000 });
              await settle(page, 4);
            },
          );
        }

        // --- The four editor widths --------------------------------------
        for (const other of WIDTHS.filter((w) => w !== combo.width)) {
          await assertRoundTrip(
            view,
            combo,
            `editor width ${combo.width} -> ${other} -> ${combo.width}`,
            async () => await setWidth(page, other),
            async () => await setWidth(page, combo.width),
          );
        }

        // --- Reload persistence: ON stays on ------------------------------
        await gotoFixture(page, server);
        await page
          .locator(".atomdown-card-line")
          .first()
          .waitFor({ state: "attached", timeout: 15_000 });

        // --- Reload persistence: OFF stays off ---------------------------
        // The half that is easy to leave out, and the half the close-then-
        // reload defect lived in.
        await runCommand(page, "Atomdown: Toggle Inline View");
        await page
          .locator(".atomdown-card-line")
          .first()
          .waitFor({ state: "detached", timeout: 10_000 });
        await gotoFixture(page, server);
        await settle(page, 6);
        const reappeared = await page.locator(".atomdown-card-line").count();
        if (reappeared > 0) {
          await failWithArtifacts(
            page,
            4,
            "state round trip — the view reopened itself after a reload",
            combo,
            { cardLines: reappeared },
            `the inline view was turned OFF, then the page was reloaded, and ` +
              `${reappeared} card line(s) came back. The restore event fired ` +
              `on stale state — the same shape as close-then-reload reopening ` +
              `the board.`,
          );
        }

        // --- Scoped per page ---------------------------------------------
        // Turn it on here, visit another page, and it must be off there.
        await gotoFixture(page, server);
        await openInline(page);
        await gotoFixture(page, server, "Library/Styles/EditorWidth");
        await settle(page, 6);
        const leaked = await page.locator(".atomdown-card-line").count();
        expect(
          leaked,
          "the inline view is remembered per page, so it must be off on a page it was never turned on for",
        ).toBe(0);
      });

      test(`board: collapse, raw/rendered, density, view and width all round-trip [${comboName(combo)}]`, async ({
        page,
      }) => {
        await gotoFixture(page, server);
        await setWidth(page, combo.width);
        let view = await openBoard(page);
        await setDensity(view, combo.density);

        await assertRoundTrip(
          view,
          combo,
          "collapse every group, then expand every group",
          async () => {
            const collapsed = await sweepEach(
              view,
              ".board-group-collapse",
              async (caret) => {
                await caret.click();
                await settle(page);
              },
            );
            expect(collapsed.length).toBe(FIXTURE.groups);
          },
          async () => {
            await sweepEach(view, ".board-group-collapse", async (caret) => {
              await caret.click();
              await settle(page);
            });
          },
        );

        const startMode = await boardViewMode(view);
        await assertRoundTrip(
          view,
          combo,
          `board view ${startMode} -> other -> ${startMode}`,
          async () => {
            await view.ev.locator("#atomdown-board-view").click();
            await settle(page, 4);
          },
          async () => {
            await view.ev.locator("#atomdown-board-view").click();
            await settle(page, 4);
          },
        );
        expect(
          await boardViewMode(view),
          "the raw/rendered toggle did not come back to where it started",
        ).toBe(startMode);

        const other = combo.density === "compact" ? "comfortable" : "compact";
        await assertRoundTrip(
          view,
          combo,
          `density ${combo.density} -> ${other} -> ${combo.density}`,
          async () => await setDensity(view, other),
          async () => await setDensity(view, combo.density),
        );

        // --- The four editor widths, as an INDEPENDENCE property ----------
        //
        // The board is a full-screen modal panel with its own layout: the
        // editor's content-column width is not one of its state machines, and
        // `plugs/atomdown-board` reads neither `--editor-width` nor
        // `html[data-editor-width]`. So the round-trip form of this assertion
        // is not the right one — it requires the DOM to CHANGE on the way out,
        // and reported "the toggle did nothing" on a panel that is correct.
        //
        // What is worth asserting is the property that actually holds, and it
        // is not weaker: every width leaves the panel byte-identical. That
        // fails the moment the panel starts depending on the editor width by
        // accident, which is the regression this cell of the matrix can see.
        const atStart = await signature(view);
        for (const w of WIDTHS.filter((x) => x !== combo.width)) {
          await setWidth(page, w);
          const now = await signature(view);
          if (
            now.count !== atStart.count ||
            now.entries.join("\n") !== atStart.entries.join("\n")
          ) {
            await failWithArtifacts(
              view.page,
              4,
              "state round trip — the editor width moved the board panel",
              combo,
              {
                width: w,
                diff: signatureDiff(atStart, now),
                before: atStart.count,
                after: now.count,
              },
              `setting the editor width to ${w} changed the board panel's ` +
                `DOM. The panel is full-screen and independent of the ` +
                `editor's content column, so nothing in it may follow that ` +
                `width. ` + signatureDiff(atStart, now).slice(0, 4).join(" / "),
            );
          }
        }
        await setWidth(page, combo.width);

        // --- Reload persistence: ON stays on ------------------------------
        await gotoFixture(page, server);
        await page
          .locator(".sb-modal .sb-panel iframe")
          .waitFor({ state: "attached", timeout: 15_000 });

        // --- Close, then reload: it must STAY closed ---------------------
        // This is the close-then-reload defect verbatim. The close path used
        // to forget the page AFTER hiding the panel, so `restoreBoard` on the
        // next `editor:pageLoaded` still saw the open flag and reopened it.
        // TAKE THE FRAME, DO NOT RE-OPEN. The panel is already open after the
        // reload — the assertion above just proved it — and `openBoard` runs
        // the toggle command, so calling it here CLOSED the panel and then
        // waited three minutes for an iframe that was never coming back.
        const openFrame = await boardFrame(page);
        if (openFrame) {
          view = { ...view, ev: openFrame };
        } else {
          view = await openBoard(page);
        }
        await view.ev.locator("#atomdown-board-close").click();
        await page
          .locator(".sb-modal .sb-panel iframe")
          .waitFor({ state: "detached", timeout: 10_000 });

        await gotoFixture(page, server);
        await settle(page, 8);
        const reopened = await page
          .locator(".sb-modal .sb-panel iframe")
          .count();
        if (reopened > 0) {
          await failWithArtifacts(
            page,
            4,
            "state round trip — the board reopened itself after a reload",
            combo,
            { iframes: reopened },
            `the board was closed with its own Close button, then the page ` +
              `was reloaded, and the panel came back. The open flag was not ` +
              `cleared before the panel was hidden.`,
          );
        }
      });
    }
  });
}

/**
 * The width steps must actually differ, or every width round trip above is
 * comparing a state to itself.
 *
 * Not parameterised: it is a property of `Library/Styles/EditorWidth.md`, not
 * of a view, and running it sixteen times would say the same thing sixteen
 * times.
 */
test.describe("editor widths", () => {
  test("the four steps resolve to four different content widths", async ({
    page,
  }) => {
    await gotoFixture(page, server);
    const measured: Record<string, number> = {};
    for (const w of WIDTHS) {
      await setWidth(page, w as Width);
      measured[w] = await page.evaluate(
        () =>
          document.querySelector(".cm-content")?.getBoundingClientRect()
            .width ?? 0,
      );
    }
    const values = Object.values(measured);
    expect(
      new Set(values.map((v) => Math.round(v))).size,
      `the four width steps must produce four different widths, got ${JSON.stringify(measured)}. ` +
        `If they are equal, the space-style block in Library/Styles/EditorWidth.md ` +
        `is not in the fixture space and every width round trip is vacuous.`,
    ).toBe(4);
    // Ordering, as declared: narrow < comfort < wide < full.
    expect(measured.narrow).toBeLessThan(measured.comfort);
    expect(measured.comfort).toBeLessThan(measured.wide);
    expect(measured.wide).toBeLessThan(measured.full);
    // And each is within reach of its declared pixel size, allowing for the
    // editor's own padding. `full` is `min(1600px, 96%)`, so on the suite's
    // 1440px viewport it is the 96% branch.
    expect(measured.narrow).toBeLessThanOrEqual(WIDTH_PX.narrow);
    expect(measured.comfort).toBeLessThanOrEqual(WIDTH_PX.comfort);
    expect(measured.wide).toBeLessThanOrEqual(WIDTH_PX.wide);
  });
});
