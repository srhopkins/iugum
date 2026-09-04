/**
 * RULE 2 — DIRECTIVE INVISIBILITY, WITH EXACTLY ONE LEGITIMATE REVEAL.
 *
 * A directive comment is bookkeeping. The reader must never see one by
 * accident, and must always be able to see the one they put the cursor in.
 *
 * At rest this asserts every `atomdown-directive` line contributes at most 4px
 * of height and shows no visible text. Then it sweeps the pointer across every
 * card's top border, hovers every card header and every group header, and
 * clicks every collapse caret, asserting after each that nothing appeared.
 * Then it puts the TEXT CURSOR in a directive line with the editor focused and
 * asserts the reveal does appear, and renders inside its card's borders —
 * never above or beside the box.
 *
 * WHY THIS IS THE SECOND RULE. On a real page every atom carries a
 * 64-character `sha256` digest that wraps over three or four rows. Eighty-two
 * of those is the single biggest reason a decorated page stops reading as
 * cards, so a directive that leaks on hover is not a cosmetic problem — it is
 * the feature failing.
 *
 * The two halves are both necessary. Hiding is easy to get right and easy to
 * regress on one interaction out of six; revealing is what keeps hiding
 * honest, because an edit must never land in a line nobody can see. A test
 * that only checked hiding would pass a build that hid the directive forever,
 * which is a worse bug.
 *
 * THE BOARD IS DIFFERENT, AND STRONGER. The panel never renders a directive at
 * all — a card body holds the block's content lines only. So for that view
 * this asserts the absolute property: no directive text exists anywhere in the
 * panel, in any state. That is checked here rather than only in rule 5 because
 * the states rule 5 does not visit are exactly the ones a leak hides in.
 */

import { test } from "@playwright/test";
import {
  type Combo,
  combos,
  comboName,
  DIRECTIVE_MAX_HEIGHT,
  directiveStates,
  expect,
  failWithArtifacts,
  FIXTURE,
  gotoFixture,
  openBoard,
  openInline,
  putCursorOnLine,
  cursorIsOnDirective,
  revealedCount,
  revealedDirectives,
  type SBServer,
  setDensity,
  settle,
  setWidth,
  startSpace,
  sweepEach,
  THEMES,
  type View,
} from "./harness.ts";

let server: SBServer;
test.beforeAll(async () => (server = await startSpace()));
test.afterAll(async () => await server?.stop());

/** Strings that only ever appear inside a directive comment. */
const DIRECTIVE_MARKERS = ["<!-- <atom", "sha256:", "atom-group id="];

/**
 * Assert nothing is revealed, and say what the reader would have seen.
 * `where` names the interaction, which is the thing a failure has to identify.
 */
async function assertNothingRevealed(
  view: View,
  combo: Combo,
  where: string,
) {
  // The cheap check first: one evaluate, no frames. Only a positive result
  // pays for the detailed measurement that builds the failure message.
  const count = await revealedCount(view.page);
  if (count === 0) return;

  // One reveal is correct when the text cursor is on a directive line with the
  // editor focused — that is the whole second half of this rule. Clicking a
  // collapse caret does exactly that: it focuses the editor and puts the
  // cursor on the group's own opening marker. Granting the exemption here is
  // what keeps the sweep from crying wolf on every collapse.
  if (count === 1 && (await cursorIsOnDirective(view.page))) return;

  const { lines, peeks } = await revealedDirectives(view.page);
  if (lines.length || peeks.length) {
    await failWithArtifacts(
      view.page,
      2,
      "directive invisibility — a directive appeared without the cursor",
      combo,
      { where, lines: lines.slice(0, 6), peeks: peeks.slice(0, 6) },
      `${where}: ${lines.length} directive line(s) and ${peeks.length} header ` +
        `reveal(s) became visible with no text cursor in them. ` +
        (lines[0]
          ? `First: ${lines[0].height.toFixed(1)}px of "${lines[0].text}".`
          : `First peek: "${peeks[0].text}".`),
    );
  }
}

for (const theme of THEMES) {
  test.describe(`theme=${theme}`, () => {
    test.use({ colorScheme: theme });

    for (const combo of combos().filter((c) => c.theme === theme)) {
      test(`inline: directives stay hidden until the cursor lands in one [${comboName(combo)}]`, async ({
        page,
      }) => {
        await gotoFixture(page, server);
        await setWidth(page, combo.width);
        const view = await openInline(page);

        // --- At rest -----------------------------------------------------
        // The page loads with the cursor at offset 0, which is the document
        // marker's own line, so "at rest" is only meaningful with the editor
        // blurred. Blurring is also the honest resting state: the reader has
        // not touched the page yet.
        await page.evaluate(() =>
          (globalThis as any).client.editorView.contentDOM.blur(),
        );
        await settle(page, 4);

        const atRest = await directiveStates(page);
        expect(
          atRest.length,
          "no directive lines were realised at all, so this rule measured nothing",
        ).toBeGreaterThan(0);

        const tooTall = atRest.filter((d) => d.overBudget);
        const legible = atRest.filter((d) => d.showsText);
        if (tooTall.length || legible.length) {
          await failWithArtifacts(
            view.page,
            2,
            "directive invisibility — a directive is visible at rest",
            combo,
            { tooTall: tooTall.slice(0, 6), legible: legible.slice(0, 6) },
            `at rest: ${tooTall.length} directive line(s) taller than ` +
              `${DIRECTIVE_MAX_HEIGHT}px and ${legible.length} showing legible text.`,
          );
        }

        // --- The pointer, everywhere it can go ---------------------------
        // Every card's top border, every card header, every group header. The
        // top border is its own case: it is the boundary between the header
        // widget and the body, and a rule keyed on the wrong element reveals
        // there and nowhere else.
        const visitedCards = await sweepEach(
          view,
          ".atomdown-card-header",
          async (card, key) => {
            const box = await card.boundingBox({ timeout: 8000 });
            if (!box) return;
            // Across the top border at three points, then over the header.
            for (const fraction of [0.05, 0.5, 0.95]) {
              await page.mouse.move(box.x + box.width * fraction, box.y + 0.5);
            }
            await card.hover({ timeout: 8000 }).catch(() => {});
            await assertNothingRevealed(
              view,
              combo,
              `pointer over card ${key} and its top border`,
            );
          },
        );
        expect(
          visitedCards.length,
          "the pointer sweep did not reach every card",
        ).toBe(FIXTURE.cards);

        const visitedGroups = await sweepEach(
          view,
          ".atomdown-group-header",
          async (header, key) => {
            await header.hover({ timeout: 8000 }).catch(() => {});
            await assertNothingRevealed(
              view,
              combo,
              `pointer over group header ${key}`,
            );
          },
        );
        expect(
          visitedGroups.length,
          "the pointer sweep did not reach every group header",
        ).toBe(FIXTURE.groups);

        // --- Every collapse caret ---------------------------------------
        // Collapsing rebuilds the decorations for a shorter document, which is
        // exactly when an offset error puts a directive back on screen.
        const visitedCarets = await sweepEach(
          view,
          ".atomdown-group-collapse",
          async (caret, key) => {
            await caret.click();
            await settle(page, 4);
            await assertNothingRevealed(
              view,
              combo,
              `after collapsing group ${key}`,
            );
            await caret.click(); // put it back
            await settle(page, 4);
            await assertNothingRevealed(
              view,
              combo,
              `after re-expanding group ${key}`,
            );
          },
        );
        expect(
          visitedCarets.length,
          "the collapse sweep did not reach every group",
        ).toBe(FIXTURE.groups);

        // --- The one legitimate reveal -----------------------------------
        const { cardRect } = await putCursorOnLine(page, '<!-- <atom id="');
        const revealed = await revealedDirectives(page);
        if (!revealed.lines.length && !revealed.peeks.length) {
          await failWithArtifacts(
            view.page,
            2,
            "directive invisibility — the cursor's own directive stayed hidden",
            combo,
            { cardRect },
            `the text cursor is in a directive line with the editor focused ` +
              `and nothing was revealed. An edit can then land in a line ` +
              `nobody can see, which is the reason the reveal exists.`,
          );
        }

        // ...and it renders inside its card, never above or beside it.
        if (cardRect) {
          const parts = [
            ...revealed.peeks.map((p) => ({ what: `peek "${p.text}"`, rect: p.rect })),
            ...revealed.lines.map((l) => ({ what: `line "${l.text}"`, rect: l.rect })),
          ];
          const escaped = parts.filter(
            (p) =>
              p.rect.top < cardRect.top - 1 ||
              p.rect.bottom > cardRect.bottom + 1 ||
              p.rect.left < cardRect.left - 1 ||
              p.rect.right > cardRect.right + 1,
          );
          if (escaped.length) {
            await failWithArtifacts(
              view.page,
              2,
              "directive invisibility — the reveal rendered outside its card",
              combo,
              { cardRect, escaped },
              `the revealed directive is drawn outside its own card box: ` +
                `${escaped[0].what} at top ${escaped[0].rect.top.toFixed(1)} ` +
                `against a card from ${cardRect.top.toFixed(1)} to ` +
                `${cardRect.bottom.toFixed(1)}.`,
            );
          }
        }
      });

      test(`board: the panel never shows a directive, in any state [${comboName(combo)}]`, async ({
        page,
      }) => {
        await gotoFixture(page, server);
        await setWidth(page, combo.width);
        const view = await openBoard(page);
        await setDensity(view, combo.density);

        const check = async (where: string) => {
          const html = await view.ev.evaluate(() => document.body.innerHTML);
          const hits = DIRECTIVE_MARKERS.filter((m) => html.includes(m));
          if (hits.length) {
            await failWithArtifacts(
              view.page,
              2,
              "directive invisibility — the board panel leaked a directive",
              combo,
              { where, markers: hits },
              `${where}: the panel's own DOM contains ${hits.join(", ")}. ` +
                `The board renders card bodies only; a directive reaching it ` +
                `means the block splitter handed over the wrong range.`,
            );
          }
        };

        await check("at rest");

        const cards = view.ev.locator(".board-card-header");
        const cardCount = await cards.count();
        expect(cardCount).toBe(FIXTURE.cards);
        // Hovering 84 cards and re-reading the panel HTML each time is the
        // slow way; the panel is one document, so a leak shows up wherever it
        // is. Sample the hovers, check the whole DOM after each.
        for (const i of [0, 1, Math.floor(cardCount / 2), cardCount - 1]) {
          await cards.nth(i).scrollIntoViewIfNeeded();
          await cards.nth(i).hover();
          await check(`pointer over card ${i + 1}`);
        }

        const carets = view.ev.locator(".board-group-collapse");
        const caretCount = await carets.count();
        expect(caretCount).toBe(FIXTURE.groups);
        for (let i = 0; i < caretCount; i++) {
          await carets.nth(i).scrollIntoViewIfNeeded();
          await carets.nth(i).click();
          await settle(page);
          await check(`after collapsing group ${i + 1}`);
          await carets.nth(i).click();
          await settle(page);
          await check(`after re-expanding group ${i + 1}`);
        }

        // Raw mode is the state most likely to leak: it prints the block's own
        // markdown, and the block's range is what a directive sits next to.
        await view.ev.locator("#atomdown-board-view").click();
        await settle(page);
        await check("in raw markdown mode");
      });
    }
  });
}
