/**
 * RULE 10 — THE HANGING INDENT, AND WHAT A DENSITY IS ALLOWED TO MOVE.
 *
 * Three properties, each one a defect Steve reported on the live page
 * (`iugum-3ad`).
 *
 * ---------------------------------------------------------------------------
 * 10a. THE HANGING INDENT SURVIVES THE CARD'S HORIZONTAL INSET.
 *
 * "bullets don't look good, usually on bullets the indentation stays
 * consistent". A wrapped list item's continuation rows aligned UNDER the
 * marker instead of after it, because the card took its horizontal inset from
 * `padding-left` with `!important` and then had to zero `text-indent` as well:
 * `client/codemirror/list_indent.ts` writes `padding-left:Nch;
 * text-indent:-Nch` as an INLINE style, and without that second override an
 * ordered list's `1.` was pulled N characters out of the card. The inset is a
 * transparent BORDER now, which composes with both, and this rule is what says
 * so.
 *
 * MEASURED PER VISUAL ROW, not per element. A soft-wrapped line is ONE element
 * with several rows, so `getBoundingClientRect` cannot see the rows at all - it
 * returns their union. A `Range` over the line's contents returns one client
 * rect per row, and that is the only reading of "the x of the first glyph on
 * each visual row" the DOM offers. Nothing here is judged by eye and nothing is
 * inferred from a class.
 *
 * THE PROPERTY, stated once and applied to every construct:
 *
 *   row 0's first glyph starts at `content box + text-indent`, and
 *   every later row's first glyph starts at `content box`.
 *
 * That covers both shapes without a special case. Where a marker IS rendered
 * the client makes `text-indent` the negative of `padding-left`, so row 0
 * starts at the marker and the rest start after it - the hanging indent. Where
 * no marker is rendered the indent is zero and every row starts on the same x.
 *
 * AND THE CLIENT'S PAIR IS ASSERTED SEPARATELY on the lines that carry it,
 * because that pair is the exact thing the old override destroyed: a card
 * whose CSS silently clobbered it again would still satisfy the row arithmetic
 * above, on rows that no longer hang.
 *
 * NON-VACUOUS BY CONSTRUCTION. A line with one visual row has no continuation
 * row to align, so this counts the WRAPPED lines it measured of each kind and
 * fails when any kind reached zero. The fixture's list items and blockquotes
 * are written long enough to wrap at all four editor widths for exactly that
 * reason - see `wrapTail` in `fixture/make-fixture.mjs`.
 *
 * ---------------------------------------------------------------------------
 * 10b. A DENSITY MOVES THE VERTICAL AXIS ONLY.
 *
 * Compact used to halve `--board-card-padding` and `--board-group-padding`,
 * and each of those carried both axes - so the distance from a card's border
 * to its first glyph moved with the density, and so did the distance from a
 * group's border to a member card's border. Each knob is two now, `-x` and
 * `-y`, and compact takes the `-y` half.
 *
 * BOTH HALVES ARE ASSERTED. Equal horizontal distances alone would pass on a
 * density that had stopped doing anything at all, so the vertical distances
 * are asserted to DIFFER in the same test. The same test also asserts the
 * group outline's WIDTH is unchanged across densities, which is the half of
 * the old "the group outline does not change" rule that is kept - see "How
 * quiet works at compact" on the library page for the trade.
 *
 * ---------------------------------------------------------------------------
 * 10c. COMPACT IS AN OUTLINE, AND ITS QUIET COSTS NO GEOMETRY.
 *
 * Steve: "just make the dotted border same as background theme so no size for
 * border, only headers go away." So the stroke is always present and always
 * the same width; only its COLOUR changes, from the page's own background
 * token at rest to its visible colour under the pointer.
 *
 * WHAT "THE PAGE BACKGROUND" IS MEASURED AGAINST: `#sb-root`. That is the
 * element the client paints `--root-background-color` on (client/styles/
 * main.scss and colors.scss), so it is the page background as the theme
 * defines it. `document.documentElement` is NOT it - it carries the app
 * chrome's own colour - and comparing against the wrong element is how this
 * assertion would pass while the card was the wrong shade.
 */

import { test } from "@playwright/test";
import {
  type Combo,
  combos,
  comboName,
  DENSITIES,
  type Density,
  expect,
  failWithArtifacts,
  gotoFixture,
  openInline,
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

/** Sub-pixel slack. Browsers report fractional rects; a quarter pixel is not a bug. */
const EPS = 0.75;

/** The element the theme paints the page background on. See the header. */
const PAGE = "#sb-root";

/** One measured line: its boxes, the client's indent, and its rows. */
type LineRows = {
  kind: string;
  hasInlineIndent: boolean;
  text: string;
  paddingLeft: number;
  textIndent: number;
  paddingBoxLeft: number;
  contentBoxLeft: number;
  cardInnerLeft: number;
  rowLefts: number[];
};

/**
 * Read every decorated list and blockquote line on screen, row by row.
 *
 * One `evaluate`, so every number in one reading comes from one layout pass.
 */
async function readRows(view: View): Promise<LineRows[]> {
  return view.page.evaluate(() => {
    const px = (v: string) => Number.parseFloat(v) || 0;

    /** The x of the first glyph on each visual row of one line element. */
    const rowsOf = (line: Element): number[] => {
      const range = document.createRange();
      range.selectNodeContents(line);
      const rects = Array.from(range.getClientRects()).filter(
        (q) => q.width > 0.5 && q.height > 0.5,
      );
      const rows: { top: number; left: number }[] = [];
      for (const q of rects) {
        // A row is a band of rects sharing a top edge. 2px of slack, because a
        // row containing a taller inline element reports two tops.
        const row = rows.find((z) => Math.abs(z.top - q.top) < 2);
        if (row) row.left = Math.min(row.left, q.left);
        else rows.push({ top: q.top, left: q.left });
      }
      rows.sort((a, b) => a.top - b.top);
      return rows.map((z) => z.left);
    };

    const kindOf = (el: Element): string | null => {
      const c = el.classList;
      if (c.contains("sb-line-blockquote")) return "blockquote";
      if (c.contains("sb-line-ol")) return "ordered";
      if (c.contains("sb-line-ul")) return "bullet";
      return null;
    };

    const out: LineRows[] = [];
    for (
      const line of Array.from(
        document.querySelectorAll(".cm-line.atomdown-card-line"),
      )
    ) {
      const kind = kindOf(line);
      if (!kind) continue;
      const cs = getComputedStyle(line);
      const before = getComputedStyle(line, "::before");
      const r = line.getBoundingClientRect();
      const bl = px(cs.borderLeftWidth);
      const pl = px(cs.paddingLeft);
      out.push({
        kind,
        // The client's list indent arrives as an INLINE style and nothing else
        // in either tree writes one, so its presence is what says "this line
        // has a rendered marker with a width".
        hasInlineIndent: (line.getAttribute("style") ?? "").includes(
          "text-indent",
        ),
        text: (line.textContent ?? "").replace(/\s+/g, " ").slice(0, 40),
        paddingLeft: pl,
        textIndent: px(cs.textIndent),
        paddingBoxLeft: r.left + bl,
        contentBoxLeft: r.left + bl + pl,
        // The first pixel inside the card's own left border.
        cardInnerLeft: r.left + bl + px(before.left) +
          px(before.borderLeftWidth),
        rowLefts: rowsOf(line),
      });
    }
    return out;
  });
}

/** Scroll the editor to a stop and settle. */
async function scrollTo(view: View, top: number) {
  await view.page.evaluate((t) => {
    const el = document.querySelector(".cm-scroller")!;
    el.scrollTop = t;
  }, top);
  await settle(view.page, 4);
}

/**
 * Scroll until a plain line of a card INSIDE A GROUP is realised, and stamp it.
 *
 * Both frames - the group's stroke and the card's - are measurable on one
 * member line, so every horizontal distance in 10b comes from one element in
 * one layout pass. A plain line, so the client's list indent is not part of
 * the distance under test. Stamped rather than indexed, for the reason the
 * harness's own sweep gives: CodeMirror recycles line elements, so an index is
 * not an identity.
 */
async function stampMemberLine(view: View): Promise<boolean> {
  for (let y = 300; y < 6000; y += 400) {
    await scrollTo(view, y);
    const found = await view.page.evaluate(() => {
      document
        .querySelectorAll("[data-fe-r10]")
        .forEach((el) => el.removeAttribute("data-fe-r10"));
      const el = Array.from(
        document.querySelectorAll<HTMLElement>(
          ".cm-line.atomdown-card-line.atomdown-group-line" +
            ":not(.sb-line-ul):not(.sb-line-ol):not(.sb-line-blockquote)" +
            ":not(.atomdown-directive)",
        ),
      ).find((el) => {
        const r = el.getBoundingClientRect();
        return r.height > 4 && r.top > 120 && r.bottom < innerHeight - 120 &&
          (el.textContent ?? "").trim().length > 0;
      });
      if (!el) return false;
      el.setAttribute("data-fe-r10", "1");
      return true;
    });
    if (found) return true;
  }
  return false;
}

// ---------------------------------------------------------------------------
// 10a — the hanging indent
// ---------------------------------------------------------------------------

for (const theme of THEMES) {
  test.describe(`theme=${theme}`, () => {
    test.use({ colorScheme: theme });

    for (const combo of combos().filter((c) => c.theme === theme)) {
      test(`10a: a wrapped list's continuation rows align after the marker, and no marker crosses the card border [${comboName(combo)}]`, async ({
        page,
      }) => {
        await gotoFixture(page, server);
        await setWidth(page, combo.width);
        const view = await openInline(page);
        await setDensity(view, combo.density);

        // Sweep the document. CodeMirror realises about five of the fixture's
        // 84 cards at a time and the constructs are spread over eleven groups,
        // so a check that read one screen would pass with a defect sixty atoms
        // down - the same reason rule 1 sweeps.
        const lines = new Map<string, LineRows>();
        const height = await page.evaluate(
          () => document.querySelector(".cm-scroller")!.scrollHeight,
        );
        for (let y = 0; y <= height + 600; y += 500) {
          await scrollTo(view, y);
          for (const line of await readRows(view)) {
            // Keyed on the line's own text, which is stable across a scroll;
            // a DOM index is not, because line elements are recycled.
            lines.set(`${line.kind}|${line.text}`, line);
          }
        }

        const problems: string[] = [];
        const bulletLevels = new Set<number>();
        for (const line of lines.values()) {
          if (line.kind === "bullet" && line.hasInlineIndent) {
            bulletLevels.add(Math.round(line.paddingLeft));
          }
        }
        const shallowest = Math.min(...bulletLevels);
        const wrapped: Record<string, number> = {
          bullet: 0,
          "bullet (nested)": 0,
          ordered: 0,
          blockquote: 0,
        };

        for (const line of lines.values()) {
          if (!line.rowLefts.length) continue;

          // THE CLIENT'S INDENT PAIR IS INTACT on every line that has one.
          // This is the property the old override destroyed.
          if (line.hasInlineIndent) {
            if (!(line.paddingLeft > 0)) {
              problems.push(
                `${line.kind}: padding-left is ${line.paddingLeft} on a line ` +
                  `the client gave a marker width - the hanging indent has ` +
                  `been overridden away. "${line.text}"`,
              );
              continue;
            }
            if (Math.abs(line.textIndent + line.paddingLeft) > EPS) {
              problems.push(
                `${line.kind}: text-indent ${line.textIndent} is not the ` +
                  `negative of padding-left ${line.paddingLeft}, so the ` +
                  `marker and the wrapped rows cannot both be right. ` +
                  `"${line.text}"`,
              );
              continue;
            }
          }

          // NO GLYPH CROSSES THE CARD BORDER. Rule 1 CONTENT says this about
          // elements; this says it about the first glyph of the FIRST row,
          // which is the marker - the thing that used to escape.
          if (line.rowLefts[0] < line.cardInnerLeft - EPS) {
            problems.push(
              `${line.kind}: the first glyph is at ${
                line.rowLefts[0].toFixed(2)
              }, ${
                (line.cardInnerLeft - line.rowLefts[0]).toFixed(2)
              }px OUTSIDE the card's left border at ${
                line.cardInnerLeft.toFixed(2)
              }. "${line.text}"`,
            );
          }

          // ROW 0 STARTS AT `content box + text-indent`.
          const want0 = line.contentBoxLeft + line.textIndent;
          if (Math.abs(line.rowLefts[0] - want0) > EPS) {
            problems.push(
              `${line.kind}: row 0 starts at ${
                line.rowLefts[0].toFixed(2)
              } but its own indent puts the marker's origin at ${
                want0.toFixed(2)
              }. "${line.text}"`,
            );
          }

          // EVERY CONTINUATION ROW STARTS AT THE CONTENT BOX: after the
          // marker. That is the whole of Steve's complaint.
          for (let k = 1; k < line.rowLefts.length; k++) {
            if (Math.abs(line.rowLefts[k] - line.contentBoxLeft) > EPS) {
              problems.push(
                `${line.kind}: row ${k} starts at ${
                  line.rowLefts[k].toFixed(2)
                } but the text after the marker starts at ${
                  line.contentBoxLeft.toFixed(2)
                } (off by ${
                  (line.rowLefts[k] - line.contentBoxLeft).toFixed(2)
                }px). "${line.text}"`,
              );
            }
          }

          if (line.rowLefts.length > 1) {
            const key = line.kind === "bullet" &&
                Math.round(line.paddingLeft) > shallowest
              ? "bullet (nested)"
              : line.kind;
            if (key in wrapped) wrapped[key]++;
          }
        }

        // NON-VACUOUS. Every kind the DoD names has to have been measured with
        // at least two visual rows, or nothing was proved about it.
        const missing = Object.entries(wrapped)
          .filter(([, n]) => n === 0)
          .map(([k]) => k);
        if (missing.length) {
          problems.push(
            `no WRAPPED line was measured for: ${
              missing.join(", ")
            }. A line with one visual row has no continuation row to align, ` +
              `so this rule proved nothing about those kinds. The fixture's ` +
              `items are written long enough to wrap at every width - see ` +
              `wrapTail in fixture/make-fixture.mjs.`,
          );
        }
        if (bulletLevels.size < 2) {
          problems.push(
            `only ${bulletLevels.size} bullet indent level(s) seen - the ` +
              `two-level nested list the DoD names was not measured.`,
          );
        }

        if (problems.length) {
          const unique = [...new Set(problems)];
          await failWithArtifacts(
            page,
            10,
            "hanging indent - a row does not start where its own indent says",
            combo,
            {
              measured: lines.size,
              wrapped,
              bulletLevels: [...bulletLevels],
              problems: unique.slice(0, 30),
            },
            `inline: ${unique.length} problem(s) over ${lines.size} decorated ` +
              `list and blockquote line(s). First: ${unique[0]}`,
          );
        }
      });
    }
  });
}

// ---------------------------------------------------------------------------
// 10b — the density split
// ---------------------------------------------------------------------------

/** Everything one density has to say about itself, in one reading. */
type DensityReading = {
  cardBorderToGlyph: number;
  groupBorderToCardBorder: number;
  cardPaddingTop: number;
  cardPaddingBottom: number;
  groupPaddingTop: number;
  cardBorderStyle: string;
  cardBorderWidth: string;
  groupBorderStyle: string;
  groupBorderWidth: string;
  cardSurface: string;
  groupSurface: string;
  pageBackground: string;
};

async function readDensity(view: View, page: string): Promise<DensityReading> {
  return view.page.evaluate((pageSel) => {
    const px = (v: string) => Number.parseFloat(v) || 0;
    const member = document.querySelector('[data-fe-r10="1"]');
    if (!member) throw new Error("rule 10: no member card line was stamped");
    const cs = getComputedStyle(member);
    const before = getComputedStyle(member, "::before");
    const after = getComputedStyle(member, "::after");
    const r = member.getBoundingClientRect();
    const bl = px(cs.borderLeftWidth);
    const glyph = r.left + bl + px(cs.paddingLeft);
    const cardBorder = r.left + bl + px(before.left);
    const groupBorder = r.left + bl + px(after.left);

    const first = document.querySelector(
      ".cm-line.atomdown-card-line.atomdown-card-first",
    );
    const last = document.querySelector(
      ".cm-line.atomdown-card-line.atomdown-card-last",
    );
    const gFirst = document.querySelector(
      ".cm-line.atomdown-group-line.atomdown-group-first",
    );

    return {
      cardBorderToGlyph: glyph - cardBorder,
      // The group's stroke is drawn inside its own box, so the gap a reader
      // sees starts at the far side of that stroke.
      groupBorderToCardBorder: cardBorder -
        (groupBorder + px(after.borderLeftWidth)),
      cardPaddingTop: first ? px(getComputedStyle(first).paddingTop) : -1,
      cardPaddingBottom: last ? px(getComputedStyle(last).paddingBottom) : -1,
      groupPaddingTop: gFirst ? px(getComputedStyle(gFirst).paddingTop) : -1,
      cardBorderStyle: before.borderLeftStyle,
      cardBorderWidth: before.borderLeftWidth,
      groupBorderStyle: after.borderLeftStyle,
      groupBorderWidth: after.borderLeftWidth,
      cardSurface: before.backgroundColor,
      groupSurface: after.backgroundColor,
      pageBackground: getComputedStyle(document.querySelector(pageSel)!)
        .backgroundColor,
    };
  }, page);
}

for (const width of ["narrow", "comfort", "wide", "full"] as const) {
  for (const theme of THEMES) {
    test(`10b: horizontal distances are identical across densities and vertical ones are not [${width}/${theme}]`, async ({
      page,
    }) => {
      await page.emulateMedia({ colorScheme: theme });
      await gotoFixture(page, server);
      await setWidth(page, width);
      const view = await openInline(page);

      const seen: Partial<Record<Density, DensityReading>> = {};
      for (const density of DENSITIES) {
        await setDensity(view, density);
        expect(
          await stampMemberLine(view),
          "no plain line of a card inside a group was realised anywhere in " +
            "the document - the fixture's shape has changed",
        ).toBe(true);
        seen[density] = await readDensity(view, PAGE);
      }
      const a = seen.comfortable!;
      const b = seen.compact!;
      const combo: Combo = { width, theme, density: "compact" };

      const fail = async (why: string) =>
        await failWithArtifacts(
          page,
          10,
          "the density split - a distance moved on the wrong axis",
          combo,
          { comfortable: a, compact: b },
          why,
        );

      // HORIZONTAL: identical.
      if (Math.abs(a.cardBorderToGlyph - b.cardBorderToGlyph) > EPS) {
        await fail(
          `card border to first glyph is ${a.cardBorderToGlyph.toFixed(2)}px ` +
            `at comfortable and ${
              b.cardBorderToGlyph.toFixed(2)
            }px at compact. A density may not move a horizontal distance.`,
        );
      }
      if (
        Math.abs(a.groupBorderToCardBorder - b.groupBorderToCardBorder) > EPS
      ) {
        await fail(
          `group border to member card border is ${
            a.groupBorderToCardBorder.toFixed(2)
          }px at comfortable and ${
            b.groupBorderToCardBorder.toFixed(2)
          }px at compact.`,
        );
      }
      // The distances have to be REAL, or two zeroes would pass.
      expect(a.cardBorderToGlyph, "card border to first glyph")
        .toBeGreaterThan(2);
      expect(a.groupBorderToCardBorder, "group border to member card border")
        .toBeGreaterThan(1);

      // VERTICAL: different, or compact has stopped meaning anything.
      if (!(b.cardPaddingTop < a.cardPaddingTop)) {
        await fail(
          `a card's top padding is ${a.cardPaddingTop} at comfortable and ` +
            `${b.cardPaddingTop} at compact - compact is not compressing the ` +
            `vertical axis, so it has stopped doing anything.`,
        );
      }
      if (!(b.cardPaddingBottom < a.cardPaddingBottom)) {
        await fail(
          `a card's bottom padding is ${a.cardPaddingBottom} at comfortable ` +
            `and ${b.cardPaddingBottom} at compact.`,
        );
      }
      if (!(b.groupPaddingTop < a.groupPaddingTop)) {
        await fail(
          `a group's interior top padding is ${a.groupPaddingTop} at ` +
            `comfortable and ${b.groupPaddingTop} at compact.`,
        );
      }

      // THE OUTLINE'S GEOMETRY IS THE SAME AT BOTH DENSITIES: the half of the
      // old "the group outline does not change" rule that is kept.
      expect(b.groupBorderWidth, "the group's stroke width at compact").toBe(
        a.groupBorderWidth,
      );
      expect(b.cardBorderWidth, "the card's stroke width at compact").toBe(
        a.cardBorderWidth,
      );

      // THE REGISTER: dotted and unfilled at compact, solid and filled at
      // comfortable.
      expect(b.cardBorderStyle, "the card's stroke at compact").toBe("dotted");
      expect(b.groupBorderStyle, "the group's stroke at compact").toBe(
        "dotted",
      );
      expect(a.cardBorderStyle, "the card's stroke at comfortable").toBe(
        "solid",
      );
      expect(a.groupBorderStyle, "the group's stroke at comfortable").toBe(
        "solid",
      );
      expect(b.cardSurface, "a compact card's surface is the page's own").toBe(
        b.pageBackground,
      );
      expect(b.groupSurface, "a compact group's surface is the page's own")
        .toBe(b.pageBackground);
      expect(a.cardSurface, "a comfortable card has a surface of its own")
        .not.toBe(a.pageBackground);
    });
  }
}

// ---------------------------------------------------------------------------
// 10c — how quiet works: colour only, no geometry, three distinct states
// ---------------------------------------------------------------------------

/**
 * Find an on-screen card line that has another card line BELOW it, and return
 * both lines' TEXT.
 *
 * ADDRESSED BY TEXT, NOT BY A STAMPED ATTRIBUTE. Putting the hover class on a
 * card rebuilds every line element of it, which takes any attribute this test
 * wrote with it - so a stamp is gone by the time the hovered state can be
 * read, and a wait on `[data-fe-r10]` waits forever. That is the harness's own
 * lesson, "an index is not an identity", one step further: neither is an
 * attribute, across a CodeMirror transaction. Text is stable across both.
 *
 * The second line is what proves the hover moved nothing: it is BELOW the
 * hovered card, so a hover that changed geometry shows up as a moved y.
 */
async function findHoverTarget(
  view: View,
): Promise<{ target: string; below: string } | null> {
  for (let y = 300; y < 6000; y += 400) {
    await scrollTo(view, y);
    const found = await view.page.evaluate(() => {
      const lines = Array.from(
        document.querySelectorAll<HTMLElement>(".cm-line.atomdown-card-line"),
      ).filter((el) => {
        const r = el.getBoundingClientRect();
        return r.height > 4 && r.top > 160 && r.bottom < innerHeight - 200 &&
          (el.textContent ?? "").trim().length > 20;
      });
      if (lines.length < 2) return null;
      const key = (el: Element) =>
        (el.textContent ?? "").replace(/\s+/g, " ").trim().slice(0, 60);
      const target = key(lines[0]);
      const below = key(lines[lines.length - 1]);
      if (target === below) return null;
      return { target, below };
    });
    if (found) return found;
  }
  return null;
}

/** The card box's computed state, plus the y of the line below it. */
async function hoverState(
  view: View,
  keys: { target: string; below: string },
  page: string,
) {
  return view.page.evaluate(({ keys, pageSel }) => {
    const find = (text: string) =>
      Array.from(
        document.querySelectorAll<HTMLElement>(".cm-line.atomdown-card-line"),
      ).find((el) =>
        (el.textContent ?? "").replace(/\s+/g, " ").trim().startsWith(text)
      );
    const el = find(keys.target);
    const below = find(keys.below);
    if (!el || !below) {
      throw new Error("rule 10: the target line is no longer realised");
    }
    const b = getComputedStyle(el, "::before");
    const r = el.getBoundingClientRect();
    return {
      borderWidth: b.borderLeftWidth,
      borderStyle: b.borderLeftStyle,
      borderColor: b.borderLeftColor,
      surface: b.backgroundColor,
      shadow: b.boxShadow,
      hovering: el.classList.contains("atomdown-card-hover"),
      selected: el.classList.contains("atomdown-selected-line"),
      left: Math.round(r.left * 100) / 100,
      width: Math.round(r.width * 100) / 100,
      box: { x: r.left, y: r.top, w: r.width, h: r.height },
      belowY: Math.round(below.getBoundingClientRect().top * 100) / 100,
      pageBackground: getComputedStyle(document.querySelector(pageSel)!)
        .backgroundColor,
    };
  }, { keys, pageSel: page });
}

for (const theme of THEMES) {
  test(`10c: at compact the stroke changes colour and nothing else, and rest, hover and selected stay three states [${theme}]`, async ({
    page,
  }) => {
    await page.emulateMedia({ colorScheme: theme });
    await gotoFixture(page, server);
    await setWidth(page, "comfort");
    const view = await openInline(page);
    await setDensity(view, "compact");
    const keys = await findHoverTarget(view);
    expect(
      keys,
      "no two card lines were realised together anywhere in the document",
    ).not.toBeNull();

    await page.mouse.move(2, 2);
    await settle(page, 4);
    const rest = await hoverState(view, keys!, PAGE);

    // AT REST THE STROKE IS INVISIBLE BECAUSE ITS COLOUR IS THE PAGE'S OWN.
    // Not `display: none` and not zero width: the box is still there, which is
    // exactly why the reveal below cannot reflow anything.
    expect(rest.borderColor, "a compact card's resting stroke colour").toBe(
      rest.pageBackground,
    );
    expect(
      Number.parseFloat(rest.borderWidth),
      "a compact card's resting stroke width",
    ).toBeGreaterThan(0);
    expect(rest.hovering, "the card is not hovered at rest").toBe(false);

    // The real pointer, on the line's own box, read a moment ago in the same
    // layout pass as `rest`.
    const box = rest.box;
    await page.mouse.move(box.x + box.w / 2, box.y + box.h / 2);
    await page.waitForFunction(
      (text) =>
        Array.from(
          document.querySelectorAll(".cm-line.atomdown-card-line"),
        ).some((el) =>
          (el.textContent ?? "").replace(/\s+/g, " ").trim().startsWith(text) &&
          el.classList.contains("atomdown-card-hover")
        ),
      keys!.target,
      // Bounded, so a hover that never lands reads as "the class did not
      // appear" rather than as a three-minute test timeout with no message.
      { timeout: 15_000 },
    );
    await settle(page, 4);
    const hovered = await hoverState(view, keys!, PAGE);

    // ONLY THE COLOUR DIFFERS.
    expect(hovered.borderWidth, "the stroke width under the pointer").toBe(
      rest.borderWidth,
    );
    expect(hovered.borderStyle, "the stroke style under the pointer").toBe(
      rest.borderStyle,
    );
    expect(hovered.borderColor, "the stroke colour under the pointer")
      .not.toBe(rest.borderColor);
    // AND NOTHING MOVED: the card's own box, and the y of a card below it.
    expect(hovered.left, "the card's left edge on hover").toBe(rest.left);
    expect(hovered.width, "the card's width on hover").toBe(rest.width);
    expect(
      Math.abs(hovered.belowY - rest.belowY),
      "the y of a card below the hovered one",
    ).toBeLessThanOrEqual(EPS);

    // SELECTED IS A THIRD STATE, with no surface fill to differ by.
    //
    // THE CLASS IS APPLIED DIRECTLY, and that is a stated limit rather than a
    // shortcut. What is under test here is the STYLESHEET: at compact neither
    // hover nor selection has a surface to differ by, so the question is
    // whether the two still compute to different values. The inline view's
    // selection gesture is an alt-drag lasso - a plain click selects in the
    // BOARD panel, which has real card elements, and rule 7 makes exactly that
    // distinction in the other direction - and a synthetic pointer drag does
    // not reliably satisfy the seam's gesture handler. That is the same
    // non-determinism the suite already records for the grip drag ("what is
    // not deterministic" in the README), so driving it here would make a
    // colour assertion flaky for a reason that has nothing to do with colour.
    //
    // `atomdown-selected-line` is the class the plug's own `selectedKeys` mark
    // puts on a lassoed card's lines; the harness names it for the same
    // reason. This asserts what the CSS does with it, and claims nothing about
    // the gesture that applies it.
    const selected = await page.evaluate(
      ({ pageSel, text }) => {
        const el = Array.from(
          document.querySelectorAll<HTMLElement>(".cm-line.atomdown-card-line"),
        ).find((el) =>
          (el.textContent ?? "").replace(/\s+/g, " ").trim().startsWith(text)
        );
        if (!el) throw new Error("rule 10: the target line is gone");
        el.classList.add("atomdown-selected-line");
        const b = getComputedStyle(el, "::before");
        const out = {
          borderWidth: b.borderLeftWidth,
          borderColor: b.borderLeftColor,
          surface: b.backgroundColor,
          shadow: b.boxShadow,
          pageBackground: getComputedStyle(document.querySelector(pageSel)!)
            .backgroundColor,
        };
        el.classList.remove("atomdown-selected-line");
        return out;
      },
      { pageSel: PAGE, text: keys!.target },
    );

    expect(selected.borderWidth, "a selected card's stroke width").toBe(
      rest.borderWidth,
    );
    expect(selected.surface, "a selected compact card still has no fill").toBe(
      selected.pageBackground,
    );
    const colours = new Set([
      rest.borderColor,
      hovered.borderColor,
      selected.borderColor,
    ]);
    expect(
      colours.size,
      `rest, hovered and selected must be distinguishable at compact with no ` +
        `surface fill; got ${[...colours].join(" / ")}`,
    ).toBe(3);
    // The ring is the other half of that distinction.
    expect(selected.shadow, "a selected card's ring").not.toBe("none");
    expect(hovered.shadow, "a hovered card has no ring").toBe("none");
  });
}
