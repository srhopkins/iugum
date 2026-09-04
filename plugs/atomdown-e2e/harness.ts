/**
 * Front-end harness for the two Atomdown views.
 *
 * The board panel (`plugs/atomdown-board`) and the inline view
 * (`plugs/atomdown-inline`) are the only two things this suite measures. Both
 * draw real boxes in a real browser, so every defect they can have is a
 * geometry or a visibility defect, and neither is reachable from a unit test
 * over a pure function. This file boots a real SilverBullet against a seeded
 * fixture space and hands the tests measurement primitives.
 *
 * It follows `silverbullet/e2e/fixtures.ts` deliberately: same free-port
 * allocation, same `waitForServer` poll, same `?headless=1` boot, same
 * `sbRuntime.ready` gate, same temp-space-per-test isolation. It does not
 * import that file, because `silverbullet/` is a vendored subtree that gets
 * re-pulled from upstream (see CONTRIBUTING.md decision 6) and an import would
 * break on the next `git subtree pull`. It reuses that tree's installed
 * `@playwright/test` and browser download through the
 * `plugs/atomdown-e2e/node_modules` symlink, so there is exactly one browser
 * install on the machine.
 *
 * WHY THE RELEASE BINARY. The inline view needs the editor decoration seam,
 * iugum's one patch to the vendored client. `target/release/silverbullet`
 * carries it (it is built from this tree) and embeds the client bundle through
 * rust-embed, so the suite needs no `npm run build` and no 90-second client
 * compile. The binary is treated as a cache: `scripts/atomdown-fe-check.sh`
 * builds it only when it is missing or older than `silverbullet/client/`.
 */

import { type ChildProcess, spawn } from "node:child_process";
import { mkdir, mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import { existsSync } from "node:fs";
import net from "node:net";
import { platform, tmpdir } from "node:os";
import { dirname, join, resolve } from "node:path";
import {
  expect,
  type Frame,
  type Locator,
  type Page,
} from "@playwright/test";

/** The platform-appropriate modifier key: Meta on macOS, Control elsewhere. */
export const mod = platform() === "darwin" ? "Meta" : "Control";

/** Repo root, from `plugs/atomdown-e2e/`. */
export const REPO_ROOT = resolve(import.meta.dirname, "..", "..");

/** The server binary the suite drives. */
export const SB_BINARY = join(
  REPO_ROOT,
  "silverbullet",
  "target",
  "release",
  "silverbullet",
);

/** Where a failure writes its screenshot and its measured numbers. */
export const ARTIFACT_DIR = process.env.ATOMDOWN_FE_ARTIFACTS
  ? resolve(process.env.ATOMDOWN_FE_ARTIFACTS)
  : join(REPO_ROOT, "scratchpad", "atomdown-fe-out");

/** The page name the fixture is served as. */
export const FIXTURE_PAGE = "Todo/running";

// ---------------------------------------------------------------------------
// The matrix
// ---------------------------------------------------------------------------

/**
 * The four editor widths from `Library/Styles/EditorWidth.md`. The names are
 * the values that page's `space-lua` writes to `html[data-editor-width]`, and
 * its `space-style` block is what turns each into a `--editor-width`. The
 * suite sets the attribute rather than clicking the cycle button so a test can
 * land on one width without walking through the other three.
 */
export const WIDTHS = ["narrow", "comfort", "wide", "full"] as const;
export type Width = (typeof WIDTHS)[number];

/** The nominal pixel width each step asks for, for a sanity assertion. */
export const WIDTH_PX: Record<Width, number> = {
  narrow: 720,
  comfort: 900,
  wide: 1280,
  full: 1600, // min(1600px, 96%) — capped by the viewport
};

/**
 * The board panel's two densities. This is a board-only knob: the inline view
 * draws in the page's own type scale and has no density of its own, so an
 * inline test parameterised over density would run the same assertion twice.
 * Persisted by the plug in clientStore under `atomdown-board-density` and
 * reflected on the panel root as `data-board-density`.
 */
export const DENSITIES = ["comfortable", "compact"] as const;
export type Density = (typeof DENSITIES)[number];

export const THEMES = ["light", "dark"] as const;
export type Theme = (typeof THEMES)[number];

export type Combo = { width: Width; theme: Theme; density: Density };

/**
 * The FAST subset, which is what the pre-push hook runs. One cell per axis
 * value, not the cross product: every width appears once, both themes appear,
 * both densities appear. A defect that needs a *pair* of axis values to show
 * up escapes this and is caught by `--full`.
 */
export const FAST_COMBOS: Combo[] = [
  { width: "comfort", theme: "light", density: "comfortable" },
  { width: "narrow", theme: "dark", density: "compact" },
  { width: "wide", theme: "light", density: "compact" },
  { width: "full", theme: "dark", density: "comfortable" },
];

/** The FULL matrix: 4 widths x 2 themes x 2 densities = 16 cells. */
export const FULL_COMBOS: Combo[] = WIDTHS.flatMap((width) =>
  THEMES.flatMap((theme) =>
    DENSITIES.map((density) => ({ width, theme, density })),
  ),
);

/** `ATOMDOWN_FE_FULL=1` (set by `--full`) switches the matrix. */
export function combos(): Combo[] {
  return process.env.ATOMDOWN_FE_FULL === "1" ? FULL_COMBOS : FAST_COMBOS;
}

export function comboName(c: Combo): string {
  return `${c.width}/${c.theme}/${c.density}`;
}

// ---------------------------------------------------------------------------
// Server
// ---------------------------------------------------------------------------

export type SBServer = {
  url: string;
  port: number;
  spaceDir: string;
  stop: () => Promise<void>;
};

export async function getFreePort(): Promise<number> {
  return new Promise((resolve, reject) => {
    const srv = net.createServer();
    srv.listen(0, "127.0.0.1", () => {
      const addr = srv.address() as net.AddressInfo;
      srv.close(() => resolve(addr.port));
    });
    srv.on("error", reject);
  });
}

export async function waitForServer(url: string, timeoutMs = 30_000) {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    try {
      if ((await fetch(url)).ok) return;
    } catch {
      // not up yet
    }
    await new Promise((r) => setTimeout(r, 200));
  }
  throw new Error(`Server did not become ready at ${url} in ${timeoutMs}ms`);
}

/**
 * The fixture page's markdown.
 *
 * The committed fixture is synthetic. It reproduces the SHAPE of Steve's real
 * page — 82 atoms, 11 named groups, a 10-row table with a link in every ticket
 * cell, a six-item ordered list, nested lists, a blockquote, a fenced code
 * block, inline code, long links, and the one row whose link markdown is
 * genuinely raw — with none of its content, because this repo has a
 * public-repo gate (CONTRIBUTING.md) and the real page carries client ticket
 * titles and colleague names.
 *
 * Set `ATOMDOWN_FE_PAGE=/abs/path/to/running.md` to run the same suite against
 * the real page locally. Nothing in the suite writes to that path, and test 6
 * proves it.
 */
export async function fixtureMarkdown(): Promise<string> {
  const override = process.env.ATOMDOWN_FE_PAGE;
  const path = override
    ? resolve(override)
    : join(import.meta.dirname, "fixture", "running.md");
  return readFile(path, "utf8");
}

/**
 * Seed a space and boot a server against it.
 *
 * The space gets both plugs, the inline view's library page (which carries its
 * action button and its `space-style`, per that plug's README), and the editor
 * width styles, because three of the six tests are parameterised over widths
 * and the width variable only exists if that page is in the space.
 */
export async function startSpace(
  extraFiles: Record<string, string> = {},
): Promise<SBServer> {
  if (!existsSync(SB_BINARY)) {
    throw new Error(
      `Missing ${SB_BINARY}.\n` +
        `Run scripts/atomdown-fe-check.sh, which builds it when absent.`,
    );
  }

  const spaceDir = await mkdtemp(join(tmpdir(), "atomdown-fe-"));
  const plugDir = join(REPO_ROOT, "plugs");

  const files: Record<string, string> = {
    [`${FIXTURE_PAGE}.md`]: await fixtureMarkdown(),
    "_plug/atomdown-inline.plug.js": await readFile(
      join(plugDir, "atomdown-inline", "atomdown-inline.plug.js"),
      "utf8",
    ),
    "_plug/atomdown-board.plug.js": await readFile(
      join(plugDir, "atomdown-board", "atomdown-board.plug.js"),
      "utf8",
    ),
    "Library/Atomdown/Inline.md": await readFile(
      join(plugDir, "atomdown-inline", "library", "Atomdown Inline.md"),
      "utf8",
    ),
    "Library/Styles/EditorWidth.md": await readFile(
      join(import.meta.dirname, "fixture", "EditorWidth.md"),
      "utf8",
    ),
    ...extraFiles,
  };

  for (const [rel, content] of Object.entries(files)) {
    const full = join(spaceDir, rel);
    await mkdir(dirname(full), { recursive: true });
    await writeFile(full, content);
  }

  const port = await getFreePort();
  const proc: ChildProcess = spawn(
    SB_BINARY,
    [spaceDir, "-p", String(port), "-L", "127.0.0.1", "--single"],
    {
      cwd: REPO_ROOT,
      stdio: ["ignore", "pipe", "pipe"],
      env: {
        ...process.env,
        // No server-side headless Chrome: the client runs its own in-page
        // runtime under `?headless=1`.
        SB_RUNTIME_API: "0",
        SB_DISABLE_SERVICE_WORKER: "1",
      },
    },
  );

  let output = "";
  proc.stdout?.on("data", (d: Buffer) => (output += d.toString()));
  proc.stderr?.on("data", (d: Buffer) => (output += d.toString()));

  const url = `http://127.0.0.1:${port}`;

  let stopped = false;
  const stop = (): Promise<void> => {
    if (stopped) return Promise.resolve();
    stopped = true;
    return new Promise<void>((res) => {
      const t = setTimeout(() => {
        proc.kill("SIGKILL");
        res();
      }, 5000);
      proc.on("exit", () => {
        clearTimeout(t);
        res();
      });
      proc.kill("SIGTERM");
    }).then(() => rm(spaceDir, { recursive: true, force: true }));
  };

  try {
    await waitForServer(`${url}/.ping`);
  } catch (err) {
    await stop();
    throw new Error(`Server failed to start. Output:\n${output}\n${err}`);
  }

  return { url, port, spaceDir, stop };
}

// ---------------------------------------------------------------------------
// Navigation and the matrix knobs
// ---------------------------------------------------------------------------

export async function waitForEditorReady(page: Page) {
  await page.waitForFunction(
    () => (globalThis as any).sbRuntime?.ready === true,
    undefined,
    { timeout: 20_000 },
  );
}

export async function gotoFixture(
  page: Page,
  server: SBServer,
  pagePath = FIXTURE_PAGE,
) {
  const encoded = pagePath.split("/").map(encodeURIComponent).join("/");
  await page.goto(`${server.url}/${encoded}?headless=1`);
  await page
    .locator("#sb-editor .cm-editor")
    .waitFor({ state: "visible", timeout: 30_000 });
  await waitForEditorReady(page);
}

/**
 * Set the editor width. `Library/Styles/EditorWidth.md` reads
 * `html[data-editor-width]`, so writing the attribute drives the real CSS.
 * Also writes clientStore so a reload keeps the width, which test 4 needs.
 */
export async function setWidth(page: Page, width: Width) {
  await page.evaluate((w) => {
    document.documentElement.setAttribute("data-editor-width", w);
  }, width);
  await page
    .evaluate(async (w) => {
      await (globalThis as any).sbRuntime?.clientStore?.set?.("editorWidth", w);
    }, width)
    .catch(() => {});
  // Let the width change settle through CodeMirror's own measure cycle before
  // anything reads a rect.
  await settle(page);
}

/**
 * Wait for layout to be stable: two consecutive animation frames, then a
 * CodeMirror measure pass. Every rect read in this suite goes through here,
 * because a rect read mid-measure is the one thing that makes a geometry test
 * flaky rather than wrong.
 */
export async function settle(page: Page, frames = 3) {
  await page.evaluate(async (n) => {
    for (let i = 0; i < n; i++) {
      await new Promise((r) => requestAnimationFrame(() => r(null)));
    }
  }, frames);
}

/**
 * Run a SilverBullet command by name.
 *
 * `client.runCommandByName` is the same entry point the command palette uses,
 * and the same one `silverbullet/e2e/baked-sections.test.ts` drives, so this
 * exercises the real command path rather than the plug's internals.
 */
export async function runCommand(page: Page, name: string) {
  await page.evaluate(
    async (n) => await (globalThis as any).client.runCommandByName(n),
    name,
  );
  await settle(page);
}

// ---------------------------------------------------------------------------
// The two views
// ---------------------------------------------------------------------------

/**
 * The two views draw the same document in two very different places, and a
 * test that only knows one of them only catches half the defects.
 *
 * - INLINE decorates the real page in the main document. Its boxes are runs of
 *   `.cm-line` elements carrying `lineClasses`, so the "box" element is the
 *   line run, and the card's top edge lives on the header widget above it.
 * - BOARD is a full-screen modal panel that SilverBullet renders as an
 *   `iframe` with a `srcDoc` (`editor.showPanel("modal", ...)` →
 *   `client/components/panel.tsx`). Its boxes are ordinary `div`s inside that
 *   frame's own document.
 *
 * So every measurement has to run against a chosen *evaluation target*: the
 * page for inline, the frame for the board. `View.ev` is that target. Both
 * `Page` and `Frame` carry `evaluate`, `locator` and `mouse`-free DOM access,
 * which is all the measurement code needs.
 */
export type ViewKind = "inline" | "board";

export type View = {
  kind: ViewKind;
  /** Where DOM lives: the page, or the board's iframe. */
  ev: Page | Frame;
  /** The page, always — for the mouse, the keyboard and screenshots. */
  page: Page;
  /** Box selectors. */
  cardBox: string;
  groupBox: string;
  cardHeader: string;
  groupHeader: string;
  /** The collapse caret for every group. */
  collapseCaret: string;
  /** A directive line, where the view draws one at all. */
  directive: string | null;
  /** Turn the view off again. */
  close: () => Promise<void>;
};

export const BOARD_FRAME_SELECTOR = ".sb-modal .sb-panel iframe";

/**
 * Turn on the inline view and return its handle.
 *
 * The inline card box is a run of line elements. `.atomdown-card-line` is on
 * every line of a card; `-first` and `-last` mark the ends. For containment we
 * want ONE rect per card, so `inlineBoxes()` below stitches a card's line runs
 * into a single rect rather than measuring each line separately — a per-line
 * check would pass even when a marker sits left of the card's own border,
 * because that marker is inside its own line.
 */
export async function openInline(page: Page): Promise<View> {
  await runCommand(page, "Atomdown: Toggle Inline View");
  await page
    .locator(".atomdown-card-line")
    .first()
    .waitFor({ state: "attached", timeout: 15_000 });
  await settle(page);
  return {
    kind: "inline",
    ev: page,
    page,
    cardBox: ".atomdown-card-line",
    groupBox: ".atomdown-group-line",
    cardHeader: ".atomdown-card-header",
    groupHeader: ".atomdown-group-header",
    collapseCaret: ".atomdown-group-collapse",
    directive: ".atomdown-directive",
    close: async () => {
      await runCommand(page, "Atomdown: Toggle Inline View");
      await settle(page);
    },
  };
}

/**
 * Turn on the board panel and return its handle, with `ev` bound to the panel
 * iframe's frame.
 *
 * The panel never renders a directive line — a card body holds the block's
 * content lines only — so `directive` is `null` and test 2 asserts the
 * stronger property for this view: no directive text exists in the panel at
 * all, in any state.
 */
export async function openBoard(page: Page): Promise<View> {
  await runCommand(page, "Atomdown: Toggle Board");
  const iframe = page.locator(BOARD_FRAME_SELECTOR);
  await iframe.waitFor({ state: "attached", timeout: 15_000 });
  const handle = await iframe.elementHandle();
  const frame = await handle!.contentFrame();
  if (!frame) throw new Error("Board panel iframe has no content frame");
  await frame
    .locator("#atomdown-board-root")
    .waitFor({ state: "attached", timeout: 15_000 });
  await frame.locator(".board-card").first().waitFor({ timeout: 15_000 });
  await settle(page);
  return {
    kind: "board",
    ev: frame,
    page,
    cardBox: ".board-card",
    groupBox: ".board-group",
    cardHeader: ".board-card-header",
    groupHeader: ".board-group-header",
    collapseCaret: ".board-group-collapse",
    directive: null,
    close: async () => {
      await frame.locator("#atomdown-board-close").click();
      await page
        .locator(BOARD_FRAME_SELECTOR)
        .waitFor({ state: "detached", timeout: 10_000 });
      await settle(page);
    },
  };
}

/**
 * Set the board's density through the real toolbar button, then wait for the
 * attribute to agree.
 *
 * The button's label is the density it will GIVE you and its
 * `data-board-density` is the density you HAVE, which is exactly the kind of
 * inversion that makes a test assert the opposite of what it means. So this
 * reads the attribute, never the label.
 */
export async function setDensity(view: View, density: Density) {
  if (view.kind !== "board") return; // inline has no density knob
  const btn = view.ev.locator("#atomdown-board-density");
  for (let i = 0; i < 3; i++) {
    const now = await btn.getAttribute("data-board-density");
    if (now === density) break;
    await btn.click();
    await view.ev
      .locator(`#atomdown-board-density[data-board-density="${density}"]`)
      .waitFor({ timeout: 5000 });
  }
  await expect(btn).toHaveAttribute("data-board-density", density);
  await settle(view.page);
}

/** Read the board's raw/rendered state from the toolbar's own attribute. */
export async function boardViewMode(view: View): Promise<string | null> {
  return view.ev.locator("#atomdown-board-view").getAttribute("data-board-view");
}

/** Flip the board between rendered and raw through the toolbar button. */
export async function toggleBoardViewMode(view: View) {
  await view.ev.locator("#atomdown-board-view").click();
  await settle(view.page);
}

// ---------------------------------------------------------------------------
// Measurement
// ---------------------------------------------------------------------------

export type Rect = {
  x: number;
  y: number;
  width: number;
  height: number;
  top: number;
  right: number;
  bottom: number;
  left: number;
};

export type Edges = { top: number; right: number; bottom: number; left: number };

export type MeasuredChild = {
  /** A human-readable label: tag, classes and a text snippet. */
  label: string;
  rect: Rect;
  /** What this child had to fit inside: the box, or a clipper within it. */
  bound: string;
  boundRect: Rect;
  boundBorder: Edges;
};

export type Box = {
  label: string;
  rect: Rect;
  /** Border widths, in px, read from the computed style of the edge element. */
  border: Edges;
  children: MeasuredChild[];
  /** Set by a `sidesOnly` spec: skip the top and bottom checks. */
  sidesOnly?: boolean;
};

/**
 * The things that must stay inside a box.
 *
 * Deliberately a list rather than `*`. A box's own chrome is not a child that
 * has to fit, a zero-area or `display: contents` node produces a rect that is
 * inside everything and proves nothing, and the interesting cases are named
 * explicitly so a reader can see that list markers, table cells, blockquote
 * bars, fenced code and links are all actually covered.
 */
export const CHILD_SELECTOR = [
  ".cm-line",
  "li",
  "ol",
  "ul",
  "blockquote",
  "table",
  "thead",
  "tbody",
  "tr",
  "th",
  "td",
  "pre",
  "code",
  "a",
  "span.cm-list-bullet",
  ".sb-line-blockquote",
  ".sb-line-code",
  ".sb-line-ul",
  ".sb-line-ol",
  ".cm-fenced-code",
  ".board-card-body",
  ".board-group-cards",
  ".board-card-header",
  ".board-group-actions",
  ".board-group-btn",
  ".board-card-menu",
].join(",");

/**
 * How to find one kind of box.
 *
 * `runPrefix` is the inline case. An inline card is not an element: it is a
 * run of `.cm-line` elements carrying `<prefix>-line`, delimited by
 * `<prefix>-first` and `<prefix>-last`, with the box's top edge drawn on the
 * header widget immediately above the run (see the inline plug's README, "How
 * a box is drawn out of lines"). Measuring those lines one at a time is the
 * mistake that lets Steve's ordered-list defect through: a marker rendered
 * left of the card's border is still inside its own line's rect. So a run is
 * stitched into ONE box whose left and right edges come from the header
 * widget, which is a real element carrying the card's real side borders.
 */
export type BoxSpec = {
  /** A label for failure output. */
  name: string;
  /** Element selector, for the board's ordinary divs. */
  selector?: string;
  /** Line-class prefix, for an inline line run. */
  runPrefix?: string;
  /** The header widget that carries an inline run's top and side edges. */
  headerSelector?: string;
  /**
   * Check the left and right edges only.
   *
   * For an inline GROUP this is the honest thing to do. A group can be taller
   * than the viewport, so `-first` and `-last` are never realised at the same
   * scroll position and a stitched run would be a fragment with a wrong top
   * and bottom. Its SIDE borders, though, are identical on every line of the
   * group, so measuring each group line as its own box and checking left and
   * right catches the defect that matters — a wide table crossing the group
   * border — with no fragment arithmetic at all. Vertical containment of a
   * group is satisfied by construction: its lines stack.
   */
  sidesOnly?: boolean;
};

/**
 * Measure every box of one kind, with the rect of every child that must stay
 * inside it.
 *
 * Two things here are load-bearing.
 *
 * ONE LAYOUT. It all runs inside a single `evaluate`, so every rect in one box
 * comes from one layout pass. Rects read across two round trips can straddle a
 * reflow, and a geometry suite that does that is flaky rather than wrong.
 *
 * CLIPPERS. A child inside a scroll container legitimately exceeds that
 * container's rect — `getBoundingClientRect` on a wide `<table>` returns the
 * table's full width even when its parent clips it, and the board deliberately
 * scrolls a wide table inside its own box. Comparing that table straight
 * against the card would report a defect that is not one. So each child is
 * bound by the nearest ancestor between it and the box whose overflow is not
 * `visible`, and every such clipper is itself added to the child list, so the
 * chain "table inside its scroller inside the card" is checked link by link.
 * If the scroller is ever removed, the table's bound becomes the card again
 * and a real overflow fails — which is the behaviour we want.
 */
export async function measureBoxes(
  ev: Page | Frame,
  spec: BoxSpec,
  childSelector = CHILD_SELECTOR,
): Promise<Box[]> {
  return ev.evaluate(
    ({ spec, childSel }) => {
      const asRect = (r: DOMRect): Rect => ({
        x: r.x,
        y: r.y,
        width: r.width,
        height: r.height,
        top: r.top,
        right: r.right,
        bottom: r.bottom,
        left: r.left,
      });
      const union = (rs: DOMRect[]): Rect => {
        const left = Math.min(...rs.map((r) => r.left));
        const right = Math.max(...rs.map((r) => r.right));
        const top = Math.min(...rs.map((r) => r.top));
        const bottom = Math.max(...rs.map((r) => r.bottom));
        return {
          x: left,
          y: top,
          width: right - left,
          height: bottom - top,
          top,
          right,
          bottom,
          left,
        };
      };
      const label = (el: Element) => {
        const cls =
          typeof el.className === "string" && el.className.trim()
            ? "." + el.className.trim().split(/\s+/).join(".")
            : "";
        const text = (el.textContent ?? "").replace(/\s+/g, " ").trim();
        return (
          `${el.tagName.toLowerCase()}${cls}` +
          (text ? ` "${text.slice(0, 60)}"` : "")
        );
      };
      const px = (v: string) => Number.parseFloat(v) || 0;
      const borderOf = (el: Element) => {
        const cs = getComputedStyle(el);
        return {
          top: px(cs.borderTopWidth),
          right: px(cs.borderRightWidth),
          bottom: px(cs.borderBottomWidth),
          left: px(cs.borderLeftWidth),
        };
      };
      const clips = (el: Element) => {
        const cs = getComputedStyle(el);
        return (
          cs.overflowX !== "visible" ||
          cs.overflowY !== "visible" ||
          cs.contain.includes("paint")
        );
      };

      /** Each unit: the elements that make up the box, plus its edge element. */
      type Unit = { members: Element[]; edge: Element };
      const units: Unit[] = [];

      if (spec.runPrefix) {
        // Inline: stitch runs of decorated lines into one box each.
        const p = spec.runPrefix;
        const lines = Array.from(
          document.querySelectorAll<HTMLElement>(`.${p}-line`),
        );
        let run: Element[] = [];
        for (const line of lines) {
          if (line.classList.contains(`${p}-first`)) run = [];
          run.push(line);
          if (line.classList.contains(`${p}-last`) && run.length) {
            // The header widget carries the box's top edge and its side
            // borders; without it the run's sides come from `.cm-line`, which
            // spans the whole content column and would make every side check
            // vacuous.
            let edge: Element = run[0];
            if (spec.headerSelector) {
              let prev = run[0].previousElementSibling;
              // A widget may be wrapped by the seam's own container.
              for (let i = 0; prev && i < 3; i++) {
                const hit = prev.matches(spec.headerSelector)
                  ? prev
                  : prev.querySelector(spec.headerSelector);
                if (hit) {
                  edge = hit;
                  break;
                }
                prev = prev.previousElementSibling;
              }
            }
            units.push({ members: run.slice(), edge });
            run = [];
          }
        }
      } else {
        for (const el of Array.from(
          document.querySelectorAll(spec.selector!),
        )) {
          units.push({ members: [el], edge: el });
        }
      }

      return units.map(({ members, edge }) => {
        // The box is the union of its member elements. For an inline run the
        // members are the decorated lines, which is where the drawn edge is:
        // the sides and bottom come from each line's `::before`, and the top
        // corners from the header widget above (measured as its own box kind,
        // because that is where a clipped group control shows up).
        const rect = union(members.map((m) => m.getBoundingClientRect()));
        // A card inside a group is drawn by a `::before` inset from the line,
        // not by the line's own border, so take the widest claim on each side.
        // `edge` is the header widget when there is one; include it so a
        // border declared there counts too.
        const border = (() => {
          const sides = ["top", "right", "bottom", "left"] as const;
          const out: Record<string, number> = {};
          for (const side of sides) {
            let max = 0;
            for (const el of [...members, edge]) {
              const cs = getComputedStyle(el);
              const bf = getComputedStyle(el, "::before");
              max = Math.max(
                max,
                px(cs.getPropertyValue(`border-${side}-width`)),
                px(bf.getPropertyValue(`border-${side}-width`)),
              );
            }
            out[side] = max;
          }
          return out as Edges;
        })();

        const seen = new Set<Element>();
        const children: MeasuredChild[] = [];
        const push = (el: Element, bound: Element | null) => {
          if (seen.has(el)) return;
          seen.add(el);
          const r = el.getBoundingClientRect();
          if (r.width <= 0.5 || r.height <= 0.5) return;
          const boundRect = bound ? asRect(bound.getBoundingClientRect()) : rect;
          const boundBorder = bound ? borderOf(bound) : border;
          children.push({
            label: label(el),
            rect: asRect(r),
            bound: bound ? label(bound) : "the box",
            boundRect,
            boundBorder,
          });
        };

        for (const member of members) {
          for (const el of Array.from(member.querySelectorAll(childSel))) {
            // Walk up to the box, looking for the nearest clipping ancestor.
            let clipper: Element | null = null;
            let cur = el.parentElement;
            while (cur && !members.includes(cur)) {
              if (clips(cur)) {
                clipper = cur;
                break;
              }
              cur = cur.parentElement;
            }
            if (clipper) push(clipper, null);
            push(el, clipper);
          }
        }
        // Label from the edge element when there is one — it carries the id,
        // which is what `boxIdentity` needs to dedupe across scroll stops.
        return {
          label: label(edge),
          rect,
          border,
          children,
          sidesOnly: !!spec.sidesOnly,
        };
      });
    },
    { spec, childSel: childSelector },
  );
}

/**
 * Measure every box in the document, not just the ones on screen.
 *
 * THIS IS THE POINT OF THE WHOLE FILE. CodeMirror virtualises: on a 1440x900
 * viewport the fixture renders about 5 of its 82 cards, and even a 12000px-tall
 * viewport only reaches 66. A suite that measures "every card" without
 * scrolling measures whichever handful happens to be on screen, which is how a
 * geometry defect 60 atoms down the page survives a green run. So this scrolls
 * the editor through the document in overlapping steps, measures at each stop,
 * and keys the results by the card's or group's own id so a box seen twice
 * counts once.
 *
 * `expected` is not decoration. Asserting the number of DISTINCT ids found is
 * what proves the sweep actually reached the end of the document — without it,
 * a sweep that silently stopped early would still report zero violations.
 */
export type Sweep = { boxes: Box[]; ids: string[]; stops: number };

export async function sweepBoxes(
  view: View,
  spec: BoxSpec,
  childSelector = CHILD_SELECTOR,
): Promise<Sweep> {
  // The board panel is not virtualised — it builds every card as a real div —
  // but it does scroll, and a rect for an off-screen card is still measurable.
  // One pass is therefore correct and cheap there.
  const scroller = view.kind === "inline" ? ".cm-scroller" : ".board-cards";

  const metrics = await view.ev.evaluate((sel) => {
    const el = document.querySelector(sel) ?? document.scrollingElement!;
    return { scrollHeight: el.scrollHeight, clientHeight: el.clientHeight };
  }, scroller);

  const step = Math.max(200, Math.floor(metrics.clientHeight * 0.6));
  const byId = new Map<string, Box>();
  let stops = 0;

  for (let y = 0; ; y += step) {
    await view.ev.evaluate(
      ({ sel, top }) => {
        const el = document.querySelector(sel) ?? document.scrollingElement!;
        el.scrollTop = top;
      },
      { sel: scroller, top: y },
    );
    await settle(view.page, 4);
    stops++;
    for (const box of await measureBoxes(view.ev, spec, childSelector)) {
      const id = boxIdentity(box);
      // Keep the first sighting. A box measured while half off-screen still
      // has correct geometry, and re-measuring it only churns the map.
      if (!byId.has(id)) byId.set(id, box);
    }
    if (y + metrics.clientHeight >= metrics.scrollHeight) break;
    if (stops > 60) break; // a runaway guard, never reached by this fixture
  }

  // Leave the editor where the test found it.
  await view.ev.evaluate((sel) => {
    const el = document.querySelector(sel) ?? document.scrollingElement!;
    el.scrollTop = 0;
  }, scroller);
  await settle(view.page);

  return { boxes: [...byId.values()], ids: [...byId.keys()], stops };
}

/**
 * A stable name for one box across two sightings.
 *
 * The Atomdown id is the only thing that is stable: a box's rect moves when
 * the page scrolls and its DOM position changes as CodeMirror recycles line
 * elements. Both views put the id in the box's own chrome, so the label built
 * by `measureBoxes` carries it. Falling back to the whole label is safe —
 * worst case a box counts twice and the expected-count assertion notices.
 */
export function boxIdentity(box: Box): string {
  const id = box.label.match(/\b([0-9A-Z]{8})\b/);
  return id ? id[1] : box.label;
}

/** One containment violation: a child rect that pokes out of its bound. */
export type Violation = {
  box: string;
  child: string;
  /** What the child had to fit inside: the box, or a clipper within it. */
  bound: string;
  side: "top" | "right" | "bottom" | "left";
  /** How far outside, in px. Positive is a real overflow. */
  overflowPx: number;
  boundRect: Rect;
  childRect: Rect;
};

/** Sub-pixel tolerance. Browsers report fractional rects; 0.75px is not a bug. */
export const EPS = 0.75;

/**
 * A child violates containment when it lies outside its bound's rect, grown by
 * that bound's own border width.
 *
 * Growing by the border is the "allowing for border width" slack: a child that
 * overlaps the border stroke itself is not reported. Every defect this rule
 * exists for — list markers left of the card's left border, a table crossing
 * the card and group borders, header controls past the content column — puts
 * the child fully outside the rect, well beyond a 1px or 2px stroke. Erring
 * lenient here is what keeps the rule from crying wolf on sub-pixel rounding
 * and getting switched off.
 */
export function containmentViolations(boxes: Box[]): Violation[] {
  const out: Violation[] = [];
  for (const box of boxes) {
    for (const child of box.children) {
      const b = child.boundRect;
      const bw = child.boundBorder;
      const c = child.rect;
      const checks: [Violation["side"], number][] = [
        ["left", b.left - bw.left - EPS - c.left],
        ["right", c.right - (b.right + bw.right + EPS)],
        ...(box.sidesOnly
          ? []
          : ([
              ["top", b.top - bw.top - EPS - c.top],
              ["bottom", c.bottom - (b.bottom + bw.bottom + EPS)],
            ] as [Violation["side"], number][])),
      ];
      for (const [side, overflow] of checks) {
        if (overflow > 0) {
          out.push({
            box: box.label,
            child: child.label,
            bound: child.bound,
            side,
            overflowPx: Number(overflow.toFixed(2)),
            boundRect: b,
            childRect: c,
          });
        }
      }
    }
  }
  return out;
}

// ---------------------------------------------------------------------------
// Failure artifacts
// ---------------------------------------------------------------------------

/**
 * Save a screenshot and the measured numbers, then fail with a message that
 * names the rule and prints the path.
 *
 * Every one of the six tests fails through here, so a failing pre-push always
 * tells the reader which rule broke, by how many pixels, and where to look.
 */
export async function failWithArtifacts(
  page: Page,
  rule: number,
  ruleName: string,
  combo: Combo | string,
  detail: unknown,
  summary: string,
): Promise<never> {
  const label = typeof combo === "string" ? combo : comboName(combo);
  const slug = `rule${rule}-${label.replace(/[^a-z0-9]+/gi, "-")}-${Date.now()}`;
  await mkdir(ARTIFACT_DIR, { recursive: true });
  const shot = join(ARTIFACT_DIR, `${slug}.png`);
  const json = join(ARTIFACT_DIR, `${slug}.json`);
  try {
    await page.screenshot({ path: shot, fullPage: true });
  } catch {
    // A crashed page cannot be shot; the numbers still get written.
  }
  await writeFile(
    json,
    JSON.stringify(
      { rule, ruleName, combo: label, summary, detail },
      null,
      2,
    ),
  );
  throw new Error(
    [
      ``,
      `RULE ${rule} FAILED — ${ruleName}`,
      `  combination: ${label}`,
      `  ${summary}`,
      ``,
      `  screenshot: ${shot}`,
      `  measured:   ${json}`,
      ``,
    ].join("\n"),
  );
}

/** Read the fixture page's bytes straight off the server's filesystem API. */
export async function readPageBytes(
  server: SBServer,
  pagePath = FIXTURE_PAGE,
): Promise<string> {
  const resp = await fetch(`${server.url}/.fs/${pagePath}.md`);
  if (!resp.ok) {
    throw new Error(`Cannot read ${pagePath}.md: ${resp.status}`);
  }
  return resp.text();
}

export { expect };
export type { Locator, Page };
