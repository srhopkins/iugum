/**
 * RULE 5 — RENDERING FIDELITY.
 *
 * The views exist to render the document. This rule checks both halves of
 * that: no raw markdown leaks through, and the structures actually render.
 *
 * NEGATIVE: the rendered page contains no `<!-- <atom`, no `sha256:`, no
 * `](http`, and no bare `##` or `**` outside code blocks.
 *
 * POSITIVE: the decisions atom is one `<ol>` with six `<li>`; the RESEA atom
 * is a `<table>` with its expected row count; every ticket cell holds an
 * `<a href>`.
 *
 * The positive half is not decoration. A "no raw markdown" assertion passes
 * perfectly on a blank page, and it also passes on a page that renders an
 * ordered list as a run-on paragraph — which was reported as a real defect in
 * the board twice and turned out both times to be a test rig's markdown stub
 * rather than the plug. Asserting the shape positively is what tells those two
 * situations apart.
 *
 * ONE KNOWN-GOOD EXCEPTION, ENCODED AS SUCH. The FFAI-62019 row is genuinely
 * raw in the source: its link label contains unescaped square brackets, which
 * close the label early, so the construct is not a link. Plain SilverBullet
 * with both views off renders that row raw too, because all three use one
 * parser. So this asserts that the row STAYS RAW rather than treating it as a
 * failure — and it asserts it positively, so a future change that starts
 * rendering it (by silently repairing the markdown) also fails here.
 */

import { test } from "@playwright/test";
import {
  type Combo,
  combos,
  comboName,
  expect,
  failWithArtifacts,
  FIXTURE,
  gotoFixture,
  openBoard,
  openInline,
  type SBServer,
  setDensity,
  settle,
  setWidth,
  startSpace,
  sweepEach,
  visibleText,
  THEMES,
  type View,
} from "./harness.ts";

let server: SBServer;
test.beforeAll(async () => (server = await startSpace()));
test.afterAll(async () => await server?.stop());

/**
 * Raw markdown that must never reach the reader.
 *
 * `](http` is the link-syntax leak. It has exactly one legitimate occurrence
 * in the fixture — the FFAI-62019 row — which is why the check counts hits
 * against that allowance rather than requiring zero.
 */
const FORBIDDEN = [
  { needle: "<!-- <atom", what: "an atom directive comment" },
  { needle: "sha256:", what: "a content digest" },
  { needle: "](http", what: "unrendered link syntax" },
];

/** The one row whose raw link markdown is correct. */
const RAW_ROW_ALLOWANCE = 1;

/**
 * Read the visible text of the rendered region, and separately the text of
 * anything inside a code block.
 *
 * The split matters for `##` and `**`: a fenced code block or an inline
 * `code` span may legitimately contain either, and a check that did not
 * exclude them would fail on the fixture's own shell snippets.
 */
const CODE_SELECTOR = [
  "code",
  "pre",
  ".cm-fenced-code",
  ".sb-line-code",
  ".sb-line-fenced-code",
  ".sb-code",
].join(",");

async function renderedText(view: View): Promise<{
  visible: string;
  outsideCode: string;
}> {
  const root =
    view.kind === "inline" ? ".cm-content" : "#atomdown-board-root .board-cards";
  await settle(view.page);
  const [visible, outsideCode] = await Promise.all([
    visibleText(view.ev, root),
    visibleText(view.ev, root, CODE_SELECTOR),
  ]);
  return { visible, outsideCode };
}

async function checkFidelity(view: View, combo: Combo, where: string) {
  const { visible, outsideCode } = await renderedText(view);

  expect(
    visible.length,
    `${where}: the rendered region is empty, so this rule checked nothing`,
  ).toBeGreaterThan(500);

  // Every needle is checked OUTSIDE code constructs, not just `##` and `**`.
  //
  // The fixture holds fenced code, and `atomdown materialize` puts an atom
  // directive on the first line INSIDE a fence rather than before it — so that
  // one directive is part of the document's own code content and renders as
  // literal code, correctly and visibly. It is the document, not a leak. The
  // leak this rule exists to catch is a directive line rendered as ordinary
  // text in the body, which is never inside a `code` or `pre`, so excluding
  // code costs the rule nothing and stops it reporting the fixture's own
  // content as a defect.
  for (const { needle, what } of FORBIDDEN) {
    const hits = outsideCode.split(needle).length - 1;
    const allowance = needle === "](http" ? RAW_ROW_ALLOWANCE : 0;
    if (hits > allowance) {
      await failWithArtifacts(
        view.page,
        5,
        "rendering fidelity — raw markdown reached the reader",
        combo,
        {
          where,
          needle,
          what,
          hits,
          allowance,
          sample: outsideCode.slice(
            Math.max(0, outsideCode.indexOf(needle) - 60),
            outsideCode.indexOf(needle) + 120,
          ),
        },
        `${where}: found ${hits} occurrence(s) of ${JSON.stringify(needle)} ` +
          `(${what}) in the rendered text, allowance ${allowance}.`,
      );
    }
  }

  // Bare `##` and `**` outside code blocks. A heading that rendered as a
  // heading has no hashes left in its text; a bold run has no asterisks.
  for (const needle of ["##", "**"]) {
    const hits = outsideCode.split(needle).length - 1;
    if (hits > 0) {
      await failWithArtifacts(
        view.page,
        5,
        "rendering fidelity — markdown syntax left in the text",
        combo,
        {
          where,
          needle,
          hits,
          sample: outsideCode.slice(
            Math.max(0, outsideCode.indexOf(needle) - 60),
            outsideCode.indexOf(needle) + 120,
          ),
        },
        `${where}: ${JSON.stringify(needle)} appears ${hits} time(s) outside ` +
          `any code block. A heading or a bold run did not render.`,
      );
    }
  }
}

for (const theme of THEMES) {
  test.describe(`theme=${theme}`, () => {
    test.use({ colorScheme: theme });

    for (const combo of combos().filter((c) => c.theme === theme)) {
      test(`inline: renders the document and leaks no markdown [${comboName(combo)}]`, async ({
        page,
      }) => {
        await gotoFixture(page, server);
        await setWidth(page, combo.width);
        const view = await openInline(page);

        // Inline is virtualised, so "the rendered page" is whatever is on
        // screen. Check at every scroll stop, which is the only way to see a
        // leak sixty atoms down.
        const scroller = ".cm-scroller";
        const metrics = await page.evaluate((sel) => {
          const el = document.querySelector(sel)!;
          return { sh: el.scrollHeight, ch: el.clientHeight };
        }, scroller);
        const step = Math.max(200, Math.floor(metrics.ch * 0.6));
        for (let y = 0; ; y += step) {
          await page.evaluate(
            ({ sel, top }) => {
              document.querySelector(sel)!.scrollTop = top;
            },
            { sel: scroller, top: y },
          );
          await settle(page, 4);
          await checkFidelity(view, combo, `inline at scroll ${y}`);
          if (y + metrics.ch >= metrics.sh) break;
        }

        await assertStructures(view, combo);
      });

      test(`board: renders the document and leaks no markdown [${comboName(combo)}]`, async ({
        page,
      }) => {
        await gotoFixture(page, server);
        await setWidth(page, combo.width);
        const view = await openBoard(page);
        await setDensity(view, combo.density);
        // The panel builds every card, so one pass covers the document.
        await checkFidelity(view, combo, "board, rendered mode");
        await assertStructures(view, combo);
      });
    }
  });
}

/**
 * The positive half: the structures are actually there.
 *
 * Scoped to the two atoms whose shape is known, found by their group id, so a
 * failure names a specific card rather than "something on the page".
 */
/**
 * Bring one group into view before measuring it.
 *
 * Inline virtualises, so a group is only in the DOM when it is on screen, and
 * the scroll loop above finishes at the BOTTOM of the document. Scrolling to
 * the top was not enough either: the decisions group was realised there and
 * the table group was not, so the two named groups have to be measured at
 * their own scroll positions rather than in one pass.
 */
async function bringGroupIntoView(
  view: View,
  groupId: string,
  which: "decisions" | "table",
) {
  if (view.kind !== "inline") return;

  // Scroll until the STRUCTURE is realised, not until the group's header is.
  //
  // Stopping at the header was not enough: the decisions group is taller than
  // the viewport, so the header and five of the six list items were on screen
  // and the sixth was not, and the rule reported five markers as a rendering
  // defect. The property is "the six items render", so the search condition
  // has to be the six items — with a fine step, because the window that holds
  // all of them is only a little taller than the list itself.
  const found = await view.page.evaluate(
    async ({ id, which }) => {
      const scroller = document.querySelector(".cm-scroller")!;
      const groupSeen = () =>
        Array.from(document.querySelectorAll(".atomdown-group-id")).some(
          (c) => c.textContent?.trim() === id,
        );
      const ready = () => {
        if (!groupSeen()) return false;
        if (which === "table") {
          return Array.from(document.querySelectorAll("table")).some(
            (t) => t.querySelectorAll("tr").length >= 11,
          );
        }
        const markers = Array.from(
          document.querySelectorAll(".cm-content *"),
        ).filter(
          (el) =>
            !el.children.length &&
            /^\d+[.)]$/.test((el.textContent ?? "").trim()),
        );
        return markers.length >= 6;
      };
      for (let y = 0; y <= scroller.scrollHeight; y += 100) {
        scroller.scrollTop = y;
        await new Promise((r) => requestAnimationFrame(() => r(null)));
        await new Promise((r) => requestAnimationFrame(() => r(null)));
        if (ready()) return true;
      }
      return ready();
    },
    { id: groupId, which },
  );
  await settle(view.page, 6);
  expect(
    found,
    `no scroll position realises the ${which} structure in group ${groupId}, ` +
      `so it cannot be measured. Either the fixture changed shape or the ` +
      `structure genuinely does not render.`,
  ).toBe(true);
}

async function assertStructures(view: View, combo: Combo) {
  await bringGroupIntoView(view, FIXTURE.decisionsGroupId, "decisions");
  await assertShape(view, combo, "decisions");
  await bringGroupIntoView(view, FIXTURE.tableGroupId, "table");
  await assertShape(view, combo, "table");
}

async function assertShape(
  view: View,
  combo: Combo,
  which: "decisions" | "table",
) {
  const shape = await view.ev.evaluate(
    ({ which, kind, tableGroup, decisionsGroup, rawTicket }) => {
      const scope = (groupId: string): Element | null => {
        if (kind === "board") {
          return document.querySelector(`.board-group[data-group-id="${groupId}"]`);
        }
        // Inline has no container element for a group: it is a run of sibling
        // lines. Find the group header carrying this id, then walk forward to
        // the last line of the group and collect what is between.
        const chip = Array.from(
          document.querySelectorAll(".atomdown-group-id"),
        ).find((c) => c.textContent?.trim() === groupId);
        const header = chip?.closest(".atomdown-group-header");
        if (!header) return null;
        const holder = document.createElement("div");
        let cur = header.nextElementSibling;
        while (cur) {
          holder.appendChild(cur.cloneNode(true));
          if (cur.classList.contains("atomdown-group-last")) break;
          cur = cur.nextElementSibling;
        }
        return holder;
      };

      // Only the group this call is about needs to be realised. Requiring
      // both at once was the bug: inline never has both on screen together.
      const wanted = which === "decisions" ? decisionsGroup : tableGroup;
      const host = scope(wanted);
      if (!host) return { found: false, which, wanted };

      // The ordered list is counted differently in the two views, and that is
      // the renderer's doing, not a defect.
      //
      // The board renders markdown to real HTML, so there is one `<ol>` with
      // six `<li>` and both are assertable. CodeMirror does NOT produce an
      // `<ol>` wrapper: it draws every source line as its own `.cm-line` and
      // decorates the list items inside them, so `document.querySelectorAll
      // ("ol")` is legitimately empty inline while 21 `<li>` exist. Asserting
      // the `<ol>` there would be asserting a fact about the renderer that has
      // never been true, so inline asserts the six items instead — which is
      // the property that actually broke when a list rendered as a run-on
      // paragraph.
      const ols = which === "decisions" ? Array.from(host.querySelectorAll("ol")) : [];
      const listItems =
        which === "decisions" ? Array.from(host.querySelectorAll("li")) : [];
      // Inline: count the ORDERED-LIST MARKERS, which is both what CodeMirror
      // actually produces and the exact thing that broke. `<li>` is not it —
      // the decisions group has none inline, because CodeMirror decorates the
      // marker inside the source line rather than building list elements. The
      // markers `1.`-`6.` rendering left of the card border is defect number
      // one in rule 1, so counting them here ties the two rules to the same
      // six objects.
      const orderedMarkers =
        which === "decisions"
          ? Array.from(host.querySelectorAll("*")).filter((el) => {
              if (el.children.length) return false;
              return /^\d+[.)]$/.test((el.textContent ?? "").trim());
            }).length
          : 0;
      const orderedLines =
        which === "decisions"
          ? host.querySelectorAll(".sb-line-ol, .sb-line-ordered-list").length
          : 0;
      const tables = which === "table" ? Array.from(host.querySelectorAll("table")) : [];
      const rows = tables[0]
        ? Array.from(tables[0].querySelectorAll("tbody tr")).length ||
          Array.from(tables[0].querySelectorAll("tr")).length - 1
        : 0;

      // The first cell of every body row is the ticket cell. Every one of
      // them holds a link, except the one row whose brackets break the label.
      const ticketCells = tables[0]
        ? Array.from(tables[0].querySelectorAll("tr"))
            .slice(1)
            .map((tr) => tr.querySelector("td,th"))
            .filter(Boolean)
            .map((cell) => ({
              text: (cell!.textContent ?? "").replace(/\s+/g, " ").trim(),
              hasLink: !!cell!.querySelector("a[href]"),
            }))
        : [];

      return {
        found: true,
        olCount: ols.length,
        liCounts: ols.map((ol) => ol.children.length),
        listItemCount: listItems.length,
        orderedMarkers,
        orderedLines,
        tableCount: tables.length,
        rows,
        ticketCells,
        rawRow: ticketCells.find((c) => c.text.includes(rawTicket)) ?? null,
      };
    },
    {
      which,
      kind: view.kind,
      tableGroup: FIXTURE.tableGroupId,
      decisionsGroup: FIXTURE.decisionsGroupId,
      rawTicket: FIXTURE.rawLinkTicket,
    },
  );

  if (!shape.found) {
    await failWithArtifacts(
      view.page,
      5,
      "rendering fidelity — the named atoms were not found",
      combo,
      shape,
      `could not locate the  group in the  view even ` +
        `after scrolling it in. Inline virtualises, so each named group is ` +
        `measured at its own scroll position.`,
    );
    return;
  }

  // --- The decisions atom: ONE ordered list of exactly SIX items ---------
  const listOk =
    view.kind === "board"
      ? shape.olCount === 1 && shape.liCounts![0] === 6
      : shape.orderedLines === 6 || shape.orderedMarkers === 6;
  if (which === "decisions" && !listOk) {
    await failWithArtifacts(
      view.page,
      5,
      "rendering fidelity — the six-item ordered list did not render",
      combo,
      shape,
      (view.kind === "board"
        ? `the decisions group holds ${shape.olCount} <ol> with ` +
          `${JSON.stringify(shape.liCounts)} items; expected exactly one ` +
          `<ol> with six <li>.`
        : `the decisions group holds  ordered-list ` +
          `line(s) and  numbered marker(s); expected ` +
          `six of one of them. CodeMirror draws no <ol> wrapper, so the ` +
          `markers are the property here.`) +
        ` An ordered list rendered as a run-on paragraph looks like this.`,
    );
  }

  // --- The table: one table, ten body rows -------------------------------
  if (which === "table" && (shape.tableCount !== 1 || shape.rows !== 10)) {
    await failWithArtifacts(
      view.page,
      5,
      "rendering fidelity — the table did not render with its rows",
      combo,
      shape,
      `the table group holds ${shape.tableCount} <table> with ${shape.rows} ` +
        `body row(s); expected one table with 10.`,
    );
  }

  // Everything below is about the table, and only the table call measured it.
  if (which !== "table") return;

  // --- Every ticket cell is a link, except the one that cannot be ---------
  const missing = shape.ticketCells!.filter(
    (c) => !c.hasLink && !c.text.includes(FIXTURE.rawLinkTicket),
  );
  if (missing.length) {
    await failWithArtifacts(
      view.page,
      5,
      "rendering fidelity — a ticket cell lost its link",
      combo,
      { missing, cells: shape.ticketCells },
      `${missing.length} ticket cell(s) hold no <a href>: ` +
        missing.map((m) => JSON.stringify(m.text)).join(", "),
    );
  }

  // --- The known-good exception, asserted positively ---------------------
  // The FFAI-62019 row must still be RAW. If a future change starts rendering
  // it, that change repaired the markdown behind the reader's back, and this
  // is where we find out.
  const raw = shape.rawRow;
  if (!raw) {
    await failWithArtifacts(
      view.page,
      5,
      "rendering fidelity — the known-raw row is missing",
      combo,
      { cells: shape.ticketCells },
      `no ticket cell mentions ${FIXTURE.rawLinkTicket}. That row is the ` +
        `documented exception to the link rule and the fixture must keep it.`,
    );
  } else if (!/\]\(http/.test(raw.text)) {
    // Rawness is asserted on the TEXT, not on the absence of an `<a href>`.
    //
    // That distinction cost a false failure: the cell does contain a link
    // element, because SilverBullet auto-links the bare URL sitting in the
    // broken construct. The link is real; the LABEL is not rendered. So the
    // property that says "this row is still raw" is that its visible text
    // still carries the `](http` syntax — which is precisely what a reader
    // sees, and precisely what would disappear if something started repairing
    // the markdown behind their back.
    await failWithArtifacts(
      view.page,
      5,
      "rendering fidelity — the known-raw row stopped being raw",
      combo,
      { raw },
      `the ${FIXTURE.rawLinkTicket} row no longer shows its raw link ` +
        `markdown: ${JSON.stringify(raw.text)}. Its label contains ` +
        `unescaped square brackets, which close the label early, so it is ` +
        `NOT a link and plain SilverBullet renders it raw too. A view that ` +
        `renders it cleanly is repairing the markdown silently.`,
    );
  }
}
