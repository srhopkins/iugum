/**
 * RULE 3 — LAYOUT STABILITY.
 *
 * Reading a page must not move it. A card that shifts when the pointer passes
 * over a neighbour is the defect that makes a view feel broken even when every
 * pixel is individually correct.
 *
 * This records the y-position of a fixed reference card, performs each
 * interaction in turn — hover every card, reveal a directive, enter and leave
 * edit mode, collapse and expand a group, toggle raw/rendered, switch density
 * — and after each asserts the reference y is unchanged.
 *
 * THE ONE DECLARED EXCEPTION is edit mode, which may grow the edited card
 * DOWNWARD only. For that case this asserts content below moves down by
 * exactly the card's height delta, and never up. "Exactly" is the point: a
 * card that grows by 120px and pushes its neighbour by 130px has a second bug
 * hiding inside the legitimate one.
 *
 * WHY Y AND NOT A SCREENSHOT. A screenshot diff over this matrix would fail on
 * font hinting and on the theme, and would need a baseline per cell that
 * somebody has to re-bless. One number per interaction says the same thing and
 * says which interaction broke it.
 *
 * The reference y is measured relative to the SCROLL CONTAINER, not the
 * viewport, because several of these interactions also scroll. A
 * viewport-relative measurement would answer a different question and would
 * report every scroll as a layout bug.
 */

import { test } from "@playwright/test";
import {
  cardTop,
  type Combo,
  combos,
  comboName,
  expect,
  failWithArtifacts,
  FIXTURE,
  gotoFixture,
  hoverBox,
  openBoard,
  openInline,
  putCursorOnLine,
  type SBServer,
  setDensity,
  settle,
  setWidth,
  startSpace,
  sweepEach,
  type View,
} from "./harness.ts";
import { THEMES } from "./harness.ts";

let server: SBServer;
test.beforeAll(async () => (server = await startSpace()));
test.afterAll(async () => await server?.stop());

/**
 * The reference card: the first atom of the `resea` group, which sits below
 * the `decisions` group and above everything else.
 *
 * It has to be BELOW some of the interactions and ABOVE others, or the test
 * cannot tell "nothing moved" from "nothing above it changed". This one is
 * below the six-item ordered list and above nine groups.
 */
const REFERENCE_SLUG = "RESEA tickets";

/** Sub-pixel drift is browser rounding. A card moving 1px is not. */
const Y_TOLERANCE = 0.75;

async function referenceId(view: View): Promise<string> {
  // Resolve the reference card's Atomdown id once, from the document, so the
  // test names a card rather than an index that a reorder would invalidate.
  const id = await view.page.evaluate((needle) => {
    const view = (globalThis as any).client.editorView;
    const doc = view.state.doc;
    for (let i = 1; i <= doc.lines; i++) {
      if (doc.line(i).text.includes(needle)) {
        // Walk back to this block's own atom directive and read its id.
        for (let j = i - 1; j > 0 && j > i - 4; j--) {
          const m = doc.line(j).text.match(/<atom id="([0-9A-Z]{8})"/);
          if (m) return m[1];
        }
      }
    }
    return null;
  }, REFERENCE_SLUG);
  if (!id) throw new Error(`no atom found for reference ${REFERENCE_SLUG}`);
  return id;
}

type Step = { what: string; run: () => Promise<void> };

async function checkStable(
  view: View,
  combo: Combo,
  id: string,
  baseline: number,
  step: Step,
) {
  await step.run();
  const now = await cardTop(view, id);
  if (now === null) {
    await failWithArtifacts(
      view.page,
      3,
      "layout stability — the reference card disappeared",
      combo,
      { step: step.what, id },
      `after "${step.what}" the reference card ${id} is no longer in the ` +
        `document. An interaction that removes an unrelated card is a worse ` +
        `bug than one that moves it.` +
        (step.what.includes("collapse")
          ? ` A group left FOLDED does this: the folded lines leave the DOM, ` +
            `so the card inside them cannot be measured. Check whether every ` +
            `group actually reopened before looking for a layout bug. Four ` +
            `separate causes have produced exactly this symptom: a click in a ` +
            `widget naming the wrong group, CodeMirror clearing a fold under ` +
            `the text cursor, two presses racing on one collapsed set, and a ` +
            `hand-copied plug bundle in the space's _plug directory running ` +
            `the plug a second time. See the inline plug's README, "The ` +
            `collapse caret".`
          : ``),
    );
  }
  if (Math.abs(now! - baseline) > Y_TOLERANCE) {
    await failWithArtifacts(
      view.page,
      3,
      "layout stability — an interaction moved an unrelated card",
      combo,
      { step: step.what, id, baseline, now, delta: now! - baseline },
      `after "${step.what}" the reference card ${id} moved ` +
        `${(now! - baseline).toFixed(2)}px (from ${baseline.toFixed(2)} to ` +
        `${now!.toFixed(2)}). Only edit mode may move content, and only down.`,
    );
  }
}

for (const theme of THEMES) {
  test.describe(`theme=${theme}`, () => {
    test.use({ colorScheme: theme });

    for (const combo of combos().filter((c) => c.theme === theme)) {
      test(`inline: no interaction moves a reference card [${comboName(combo)}]`, async ({
        page,
      }) => {
        await gotoFixture(page, server);
        await setWidth(page, combo.width);
        const view = await openInline(page);
        const id = await referenceId(view);
        const baseline = await cardTop(view, id);
        expect(baseline, `reference card ${id} was not found`).not.toBeNull();

        const steps: Step[] = [
          {
            what: "hover every card",
            run: async () => {
              const visited = await sweepEach(
                view,
                ".atomdown-card-header",
                async (card) => {
                  await hoverBox(view, card);
                },
              );
              expect(visited.length).toBe(FIXTURE.cards);
            },
          },
          {
            what: "reveal a directive with the text cursor",
            run: async () => {
              await putCursorOnLine(page, '<!-- <atom id="');
            },
          },
          {
            what: "hover every group header",
            run: async () => {
              const visited = await sweepEach(
                view,
                ".atomdown-group-header",
                async (h) => {
                  await hoverBox(view, h);
                },
              );
              expect(visited.length).toBe(FIXTURE.groups);
            },
          },
          {
            // Collapsing the LAST group cannot move the reference card, which
            // is near the top. Collapsing the FIRST one can, and is the case
            // that matters — so this collapses and re-expands every group and
            // only compares once both are back open.
            what: "collapse and expand every group",
            run: async () => {
              // TWO SWEEPS, not one caret clicked twice. Collapsing a group
              // shortens the document, which realises more group headers, so
              // `nth(i)` between the two clicks no longer points at the same
              // caret and the second click collapsed a DIFFERENT group. One
              // sweep that collapses everything and a second that expands
              // everything drives each caret by its own group's key.
              const collapsed = await sweepEach(
                view,
                ".atomdown-group-collapse",
                async (caret) => {
                  await caret.click();
                  await settle(page, 4);
                },
              );
              expect(collapsed.length).toBe(FIXTURE.groups);
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
          },
          {
            what: "turn the view off and on again",
            run: async () => {
              await view.close();
              await openInline(page);
            },
          },
        ];

        for (const step of steps) {
          await checkStable(view, combo, id, baseline!, step);
        }
      });

      test(`board: no interaction moves a reference card, and edit mode only grows down [${comboName(combo)}]`, async ({
        page,
      }) => {
        await gotoFixture(page, server);
        await setWidth(page, combo.width);
        const view = await openBoard(page);
        await setDensity(view, combo.density);
        const id = await referenceId(view);
        const baseline = await cardTop(view, id);
        expect(baseline, `reference card ${id} was not found`).not.toBeNull();

        const steps: Step[] = [
          {
            what: "hover every card",
            run: async () => {
              const visited = await sweepEach(
                view,
                ".board-card-header",
                async (c) => {
                  // A real mouse move, not `hover`. At compact density the
                  // header is a `pointer-events: none` overlay by design, so
                  // the hit-target check can never pass; and the element is
                  // rebuilt under the locator anyway. The move lands where a
                  // reader's pointer lands, and the browser delivers the event
                  // to whatever is topmost, exactly as it does for a reader.
                  await hoverBox(view, c);
                },
              );
              expect(visited.length).toBe(FIXTURE.cards);
            },
          },
          {
            what: "collapse and expand every group",
            run: async () => {
              // Two sweeps, for the same reason as the inline case above: a
              // caret clicked twice through an index is not necessarily the
              // same caret both times.
              await sweepEach(view, ".board-group-collapse", async (caret) => {
                await caret.click();
                await settle(page);
              });
              await sweepEach(view, ".board-group-collapse", async (caret) => {
                await caret.click();
                await settle(page);
              });
            },
          },
          {
            what: "toggle raw markdown and back to rendered",
            run: async () => {
              const btn = view.ev.locator("#atomdown-board-view");
              await btn.click();
              await settle(page);
              await btn.click();
              await settle(page);
            },
          },
          {
            what: "switch density to compact and back",
            run: async () => {
              const start = combo.density;
              const other = start === "compact" ? "comfortable" : "compact";
              await setDensity(view, other);
              await setDensity(view, start);
            },
          },
        ];

        for (const step of steps) {
          await checkStable(view, combo, id, baseline!, step);
        }

        // --- The declared exception: edit mode grows DOWNWARD only --------
        //
        // Open the editor on a card ABOVE the reference card, so the reference
        // is the "content below" whose movement is being measured. Then assert
        // it moved down by exactly the edited card's height delta.
        const editTarget = view.ev.locator(".board-card").first();
        const editId = await editTarget.getAttribute("data-atom-id");
        const before = {
          card: (await editTarget.boundingBox())!.height,
          reference: (await cardTop(view, id))!,
        };

        await editTarget.locator("[data-card-rendered]").dblclick();
        await view.ev
          .locator(`.board-card[data-atom-id="${editId}"].board-card-editing`)
          .waitFor({ timeout: 10_000 });
        await settle(page, 4);

        const during = {
          card: (await editTarget.boundingBox())!.height,
          reference: (await cardTop(view, id))!,
        };
        const cardGrew = during.card - before.card;
        const contentMoved = during.reference - before.reference;

        if (contentMoved < -Y_TOLERANCE) {
          await failWithArtifacts(
            view.page,
            3,
            "layout stability — edit mode moved content UP",
            combo,
            { editId, before, during, cardGrew, contentMoved },
            `opening the editor on card ${editId} moved the reference card ` +
              `${(-contentMoved).toFixed(2)}px UP. Edit mode may only grow ` +
              `its own card downward.`,
          );
        }
        if (Math.abs(contentMoved - cardGrew) > 1.5) {
          await failWithArtifacts(
            view.page,
            3,
            "layout stability — edit mode moved content by the wrong amount",
            combo,
            { editId, before, during, cardGrew, contentMoved },
            `card ${editId} grew ${cardGrew.toFixed(2)}px but content below ` +
              `moved ${contentMoved.toFixed(2)}px. Those must be the same ` +
              `number: any difference is a second layout change riding along ` +
              `with the legitimate one.`,
          );
        }

        // Leaving edit mode must put everything back exactly.
        await page.keyboard.press("Escape");
        await view.ev
          .locator(".board-card-editing")
          .waitFor({ state: "detached", timeout: 10_000 })
          .catch(async () => {
            // Some builds commit on blur rather than Escape.
            await view.ev.locator(".board-toolbar").click();
            await settle(page, 4);
          });
        await checkStable(view, combo, id, baseline!, {
          what: "leave edit mode",
          run: async () => await settle(page, 4),
        });
      });
    }
  });
}
