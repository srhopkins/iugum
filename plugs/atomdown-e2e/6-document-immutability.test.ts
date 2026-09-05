/**
 * RULE 6 — DOCUMENT IMMUTABILITY.
 *
 * Reading a document must not change it. This is the rule that protects the
 * thing the other five are only decorating.
 *
 * After running every interaction the other rules perform, this asserts the
 * page file's bytes on disk are unchanged, and that the real `atomdown lint`
 * and `atomdown verify` both pass. It also covers the edit path: make an edit,
 * undo it, and assert the bytes are identical again.
 *
 * WHY THE BYTES AND NOT A PARSE. An id, a slug or a digest changing is a
 * silent data loss: the document still lints, still renders, still looks
 * right, and a reviewer sees a diff full of churn they cannot evaluate. Only
 * a byte comparison catches it. `atomdown verify` is then the second, stronger
 * question — not "did the file change" but "does every recorded digest still
 * match its content" — because a digest is a claim that a person reviewed that
 * exact text, and nothing may refresh it as a side effect.
 *
 * WHY THE UNDO HALF. Every write in these plugs is computed as a whole new
 * document, reduced to the smallest single replacement, and applied as ONE
 * CodeMirror transaction, precisely so one Cmd-Z reverts the whole thing. That
 * design claim is untested by anything else here, and an edit that undoes to
 * "almost the original" is the worst kind of corruption: invisible, and
 * committed.
 *
 * The atomdown binary is located, not assumed. If none is present the two
 * tool checks are skipped with a loud reason and the byte checks still run —
 * a gate that fails because a sibling checkout is missing gets switched off.
 */

import { execFile } from "node:child_process";
import { existsSync } from "node:fs";
import { join } from "node:path";
import { promisify } from "node:util";
import { test } from "@playwright/test";
import {
  expect,
  failWithArtifacts,
  FIXTURE,
  FIXTURE_PAGE,
  gotoFixture,
  hoverBox,
  mod,
  openBoard,
  openInline,
  putCursorOnLine,
  readPageBytes,
  type SBServer,
  setDensity,
  settle,
  setWidth,
  startSpace,
  sweepEach,
  WIDTHS,
} from "./harness.ts";

const run = promisify(execFile);

let server: SBServer;
test.beforeAll(async () => (server = await startSpace()));
test.afterAll(async () => await server?.stop());

/**
 * Find the `atomdown` binary.
 *
 * Ordered from most explicit to most incidental. The last candidate is the
 * sibling checkout, which is where it lives on Steve's machine; depending on
 * it alone would make this rule fail for anyone else, so it is the fallback
 * rather than the assumption.
 */
function atomdownBin(): string | null {
  const candidates = [
    process.env.ATOMDOWN_BIN,
    join(process.env.HOME ?? "", "go", "bin", "atomdown"),
    "/usr/local/bin/atomdown",
    "/opt/homebrew/bin/atomdown",
    join(process.env.HOME ?? "", "projects/github/srhopkins/atomdown/atomdown"),
  ].filter(Boolean) as string[];
  return candidates.find((c) => existsSync(c)) ?? null;
}

/** The fixture page's path inside the running server's space. */
function pagePath(): string {
  return join(server.spaceDir, `${FIXTURE_PAGE}.md`);
}

test("the whole suite's interactions leave the page byte-identical, and atomdown still passes", async ({
  page,
}) => {
  const before = await readPageBytes(server);
  expect(
    before.length,
    "the fixture page came back empty from the server",
  ).toBeGreaterThan(1000);
  expect(
    (before.match(/<atom id="/g) ?? []).length,
    "the fixture does not have the atom count the suite was written for",
  ).toBe(FIXTURE.atoms);

  // --- Every interaction the other five rules perform ---------------------
  await gotoFixture(page, server);

  // Inline: open, collapse and expand every group, hover every card, put the
  // cursor in a directive line, walk all four widths, close.
  const inline = await openInline(page);
  // Two sweeps rather than one caret clicked twice: a press rewrites the
  // decorations, so the second click through the same index is not necessarily
  // the same caret.
  for (let pass = 0; pass < 2; pass++) {
    await sweepEach(inline, ".atomdown-group-collapse", async (caret) => {
      await caret.click();
      await settle(page, 3);
    });
  }
  await sweepEach(inline, ".atomdown-card-header", async (card) => {
    // A real mouse move, not `hover`: see `hoverBox` in the harness. `hover`
    // waits out its timeout on an element CodeMirror rebuilt under it, and at
    // 84 cards that is longer than the test.
    await hoverBox(inline, card);
  });
  await putCursorOnLine(page, '<!-- <atom id="');
  for (const w of WIDTHS) await setWidth(page, w);
  await inline.close();

  // Board: open, collapse and expand every group, flip raw/rendered, flip
  // density both ways, close.
  const board = await openBoard(page);
  for (let pass = 0; pass < 2; pass++) {
    await sweepEach(board, ".board-group-collapse", async (caret) => {
      await caret.click();
      await settle(page);
    });
  }
  await board.ev.locator("#atomdown-board-view").click();
  await settle(page);
  await board.ev.locator("#atomdown-board-view").click();
  await settle(page);
  await setDensity(board, "compact");
  await setDensity(board, "comfortable");
  await board.close();

  // A save is asynchronous. Give the client every chance to write before
  // claiming it did not — a test that reads too early proves nothing.
  await settle(page, 10);
  await page.waitForTimeout(1500);

  const after = await readPageBytes(server);
  if (after !== before) {
    await failWithArtifacts(
      page,
      6,
      "document immutability — reading the page changed it",
      "all interactions",
      {
        beforeLength: before.length,
        afterLength: after.length,
        firstDiff: firstDifference(before, after),
      },
      `the page file changed after interactions that only read it. ` +
        `Length ${before.length} -> ${after.length}. ` +
        `First difference at offset ${firstDifference(before, after).offset}.`,
    );
  }

  // --- The real tools ----------------------------------------------------
  const bin = atomdownBin();
  test.skip(
    !bin,
    "no atomdown binary found (set ATOMDOWN_BIN); the byte checks above still ran",
  );

  for (const cmd of ["lint", "verify"] as const) {
    const { stdout } = await run(bin!, [cmd, pagePath()]);
    const out = stdout.trim();
    if (!out.startsWith("ok")) {
      await failWithArtifacts(
        page,
        6,
        `document immutability — atomdown ${cmd} failed`,
        "all interactions",
        { command: `${bin} ${cmd} ${pagePath()}`, output: out },
        `\`atomdown ${cmd}\` says: ${out}. ` +
          (cmd === "verify"
            ? `A digest no longer matches its content, which means something ` +
              `rewrote a block or refreshed a digest as a side effect.`
            : `The document is no longer valid Atomdown.`),
      );
    }
  }
});

test("an edit, then an undo, returns the page to the same bytes", async ({
  page,
}) => {
  const original = await readPageBytes(server);

  await gotoFixture(page, server);
  const view = await openInline(page);

  // Type into a card body — a content line, never a directive. The plugs are
  // built so no reorder, group, ungroup or rename touches a directive line,
  // and only typing into a block produces drift, so typing is the honest test.
  const marker = "Answer the six above in order";
  await putCursorOnLine(page, marker);
  await page.keyboard.press("End");
  await page.keyboard.type(" EDITED-BY-RULE-SIX");
  await settle(page, 4);
  await page.waitForTimeout(1500);

  const edited = await readPageBytes(server);
  // The edit must actually land, or the undo below proves nothing.
  if (edited === original) {
    await failWithArtifacts(
      page,
      6,
      "document immutability — the edit never reached the file",
      "edit then undo",
      { length: original.length },
      `typing into a card body did not change the page on disk, so the undo ` +
        `half of this rule would pass without testing anything. Either the ` +
        `save did not fire or the keystrokes went somewhere else.`,
    );
  }
  expect(edited).toContain("EDITED-BY-RULE-SIX");

  // One Cmd-Z. The whole edit is one CodeMirror transaction by design, so one
  // undo is the correct number and asserting more would hide a regression.
  await page.keyboard.press(`${mod}+z`);
  await settle(page, 4);
  await page.waitForTimeout(1500);

  const undone = await readPageBytes(server);
  if (undone !== original) {
    await failWithArtifacts(
      page,
      6,
      "document immutability — undo did not restore the original bytes",
      "edit then undo",
      {
        originalLength: original.length,
        undoneLength: undone.length,
        firstDiff: firstDifference(original, undone),
        stillContainsEdit: undone.includes("EDITED-BY-RULE-SIX"),
      },
      `after one undo the file differs from the original. ` +
        (undone.includes("EDITED-BY-RULE-SIX")
          ? `The edit is still there: the edit was more than one transaction.`
          : `The edit is gone but the bytes differ at offset ` +
            `${firstDifference(original, undone).offset} — undo restored ` +
            `almost the original, which is the worst kind of corruption.`),
    );
  }

  await view.close();
});

/** Where two strings first differ, with a little context each side. */
function firstDifference(
  a: string,
  b: string,
): { offset: number; a: string; b: string } {
  const n = Math.min(a.length, b.length);
  let i = 0;
  while (i < n && a[i] === b[i]) i++;
  return {
    offset: i,
    a: a.slice(Math.max(0, i - 40), i + 80),
    b: b.slice(Math.max(0, i - 40), i + 80),
  };
}
