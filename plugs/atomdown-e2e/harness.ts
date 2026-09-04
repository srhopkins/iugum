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

/**
 * What is in the fixture, as numbers the tests assert against.
 *
 * These are not decoration. A containment sweep that measured 6 of 84 cards
 * would report zero violations and pass, so the counts are what prove the
 * sweep reached the end of the document.
 *
 * The fixture holds two fenced code blocks, because Steve named fenced code as
 * a containment case, and that is why `cards` is 84 rather than 82.
 */
export const FIXTURE = {
  atoms: 82,
  groups: 11,
  /**
   * Cards drawn, in BOTH views: the 82 atoms plus one per fenced code block.
   * `atomdown materialize` leaves a fence's opening line outside the atom it
   * creates, so each fence is an uncovered block that both views draw as a
   * card marked implicit. `atomdown` counts 82 atoms for the same file. Both
   * numbers are right; they count different things.
   */
  cards: 84,
  implicitCards: 2,
  /** The 10-row table's group, and the six-item ordered list's group. */
  tableGroupId: "NS67J8K5",
  decisionsGroupId: "KATZ94NM",
  /** The one row whose link markdown is genuinely raw. See rule 5. */
  rawLinkTicket: "FFAI-62019",
} as const;

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
  /**
   * The box's own Atomdown id, read from the DOM.
   *
   * This is the dedupe key for a scroll sweep, and it has to come from the
   * document rather than from the label. Deriving it from the label loses the
   * two implicit cards — neither has an Atomdown id, both hold the same fenced
   * code, so their labels are byte-identical and the sweep counted 83 cards
   * instead of 84. An id that is missing here falls back to the DOM position,
   * which is unique by construction.
   */
  id: string;
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
      /**
       * The Atomdown id of one box, from the DOM.
       *
       * Four places carry it, in falling order of directness: the board's own
       * `data-atom-id` / `data-group-id`, and the id chip both views print in
       * the header. An implicit card has `implicit-1` there, which is still
       * unique. When nothing carries one — a bare group line — fall back to
       * the element's index among its siblings, which cannot collide.
       */
      const idOf = (els: Element[]): string => {
        for (const el of els) {
          const own =
            el.getAttribute?.("data-atom-id") ??
            el.getAttribute?.("data-group-id");
          if (own) return own;
          const chip = el.querySelector?.(
            ".board-card-id,.board-group-id,.atomdown-card-id,.atomdown-group-id",
          );
          const text = chip?.textContent?.trim();
          if (text) return text;
        }
        // A card with no Atomdown id is real, not a bug. `atomdown
        // materialize` always leaves the OPENING line of a fenced code block
        // outside the atom it creates, so a page with fenced code has one
        // uncovered block per fence, and both views draw it as a card marked
        // implicit. The board gives those `data-atom-id="implicit-N"`; inline
        // prints "no id" and offers nothing to key on.
        //
        // So key them by their own text. DOM position is what a first version
        // used, and it is wrong: CodeMirror recycles line elements, so the
        // same card had a different position at every scroll stop and one
        // implicit card counted five or six times. Text is stable across a
        // scroll, which is the whole requirement.
        // Two following siblings are included because a header widget
        // measured on its own says only "no id" — identical for both implicit
        // cards — while the line just below it is that card's own fenced code
        // and tells them apart.
        const near: Element[] = [...els];
        let next = els[els.length - 1]?.nextElementSibling ?? null;
        for (let i = 0; next && i < 2; i++) {
          near.push(next);
          next = next.nextElementSibling;
        }
        const text = near
          .map((el) => el.textContent ?? "")
          .join(" ")
          .replace(/\s+/g, " ")
          .trim()
          .slice(0, 120);
        return `text:${text}`;
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
        // Only a run that actually STARTED in this viewport is a box. At a
        // scroll boundary CodeMirror realises a `-last` line whose `-first` is
        // above the rendered window; accumulating that into a box produced
        // seven phantom cards per sweep with a wrong top edge, and made the
        // count differ run to run. A dropped fragment costs nothing: the sweep
        // overlaps by 40%, so the whole run is realised at another stop.
        let started = false;
        for (const line of lines) {
          if (line.classList.contains(`${p}-first`)) {
            run = [];
            started = true;
          }
          if (!started) continue;
          run.push(line);
          if (line.classList.contains(`${p}-last`) && run.length) {
            // The header widget carries the box's top edge and its side
            // borders; without it the run's sides come from `.cm-line`, which
            // spans the whole content column and would make every side check
            // vacuous.
            let edge: Element | null = spec.headerSelector ? null : run[0];
            if (spec.headerSelector) {
              // The header widget sits just above the run, usually separated
              // from it by the atom's own hidden directive line.
              let prev = run[0].previousElementSibling;
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
            // A run whose header is not realised is another kind of fragment:
            // the header carries the box's top edge and its id, so without it
            // there is neither a rect to measure against nor a key to dedupe
            // by, and the sweep counted five phantom cards per pass. Skip it;
            // the overlap means the complete run is measured at another stop.
            if (edge) units.push({ members: run.slice(), edge });
            run = [];
            started = false;
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
          id: idOf([edge, ...members]),
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
 * The Atomdown id is the only stable thing: a box's rect moves when the page
 * scrolls and its DOM position changes as CodeMirror recycles line elements.
 * `measureBoxes` reads it out of the document; see `Box.id` for why it is not
 * derived from the label.
 */
export function boxIdentity(box: Box): string {
  return box.id;
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

/**
 * Do something to EVERY element matching `selector` in the whole document, not
 * just the ones on screen.
 *
 * The interaction counterpart of `sweepBoxes`, and needed for the same reason.
 * `page.locator(".atomdown-group-header").count()` returns 2 on a fresh
 * 1440x900 viewport because CodeMirror has only realised the top of the page,
 * so a loop over `nth(i)` visits two of eleven group headers and its indices
 * shift under it as scrolling realises more. This scrolls stop by stop and
 * calls `fn` once per distinct element, keyed by the element's own id chip or
 * its text.
 *
 * Returns the keys visited, so a caller can assert it reached all of them.
 */
export async function sweepEach(
  view: View,
  selector: string,
  fn: (locator: Locator, key: string, index: number) => Promise<void>,
): Promise<string[]> {
  const scroller = view.kind === "inline" ? ".cm-scroller" : ".board-cards";
  const metrics = await view.ev.evaluate((sel) => {
    const el = document.querySelector(sel) ?? document.scrollingElement!;
    return { scrollHeight: el.scrollHeight, clientHeight: el.clientHeight };
  }, scroller);
  const step = Math.max(200, Math.floor(metrics.clientHeight * 0.6));
  const seen: string[] = [];

  for (let y = 0; ; y += step) {
    await view.ev.evaluate(
      ({ sel, top }) => {
        const el = document.querySelector(sel) ?? document.scrollingElement!;
        el.scrollTop = top;
      },
      { sel: scroller, top: y },
    );
    await settle(view.page, 4);

    const readKeys = (): Promise<string[]> =>
      view.ev.evaluate((sel) =>
        Array.from(document.querySelectorAll(sel)).map((el) => {
          // Look at the element AND the unit it belongs to.
          //
          // A control keyed only on itself collides with every other copy of
          // itself: every collapse caret reads "caret" and nothing else, so a
          // sweep over `.atomdown-group-collapse` visited 1 of 11 groups and
          // reported it. The caret's identity is its GROUP's identity, which
          // lives on the header the caret sits in.
          const unit =
            el.closest(
              "[data-atom-id],[data-group-id],.atomdown-card-header,.atomdown-group-header,.board-card,.board-group",
            ) ?? el;
          const chip = unit.querySelector(
            ".board-card-id,.board-group-id,.atomdown-card-id,.atomdown-group-id",
          );
          const own =
            el.getAttribute("data-atom-id") ??
            el.getAttribute("data-group-id") ??
            el.getAttribute("data-group-collapse") ??
            unit.getAttribute("data-atom-id") ??
            unit.getAttribute("data-group-id");
          if (own) return own;
          const id = chip?.textContent?.trim();
          if (id) return id;
          // A card with no Atomdown id — the implicit card a fenced code
          // block produces — has a header that reads only "no id", the same
          // for every one of them. Two following siblings make it unique,
          // because the line below the header is that card's own content.
          // Without this the sweep visited 83 of 84 cards and said so.
          const near: Element[] = [unit];
          let next = unit.nextElementSibling;
          for (let i = 0; next && i < 2; i++) {
            near.push(next);
            next = next.nextElementSibling;
          }
          return `text:${near
            .map((n) => n.textContent ?? "")
            .join(" ")
            .replace(/\s+/g, " ")
            .trim()
            .slice(0, 80)}`;
        }),
        selector,
      );

    // Re-read the key list after EVERY interaction, and act on the first
    // unseen index rather than iterating a snapshot.
    //
    // The snapshot version deadlocked. Hovering a card puts `hoverClasses` on
    // its group's lines, which is a CodeMirror transaction, which rebuilds
    // line elements — so `nth(i)` for the next card pointed at a detached
    // node and `boundingBox()` sat there until the 180-second test timeout.
    for (;;) {
      const keys: string[] = await readKeys();
      const i = keys.findIndex((k) => !seen.includes(k));
      if (i < 0) break;
      seen.push(keys[i]);
      await fn(view.ev.locator(selector).nth(i), keys[i], seen.length - 1);
    }

    if (y + metrics.clientHeight >= metrics.scrollHeight) break;
    if (y > metrics.scrollHeight + metrics.clientHeight) break;
  }
  return seen;
}

// ---------------------------------------------------------------------------
// Directive visibility (rule 2)
// ---------------------------------------------------------------------------

/**
 * The height a hidden directive line is allowed to contribute.
 *
 * The inline plug collapses a directive to a sliver rather than
 * `display: none`, so the line element stays in the layout and CodeMirror's
 * cursor and coordinate maths are untouched. On this build the sliver measures
 * 0px; 4px is the budget, which leaves room for a border or a rounding without
 * letting a 64-character sha256 digest back onto the page.
 */
export const DIRECTIVE_MAX_HEIGHT = 4;

export type DirectiveState = {
  index: number;
  height: number;
  /** The height this line is allowed to be, and whether it exceeds it. */
  budget: number;
  overBudget: boolean;
  /** Any legible text at all: non-zero font, non-transparent, laid out. */
  showsText: boolean;
  text: string;
  rect: Rect;
};

/** Measure every directive line currently realised in the inline view. */
export async function directiveStates(page: Page): Promise<DirectiveState[]> {
  await settle(page);
  return page.evaluate(() => {
    const alpha = (c: string) => {
      const m = c.match(/rgba?\(([^)]+)\)/);
      if (!m) return 1;
      const parts = m[1].split(",").map((s) => Number.parseFloat(s));
      return parts.length > 3 ? parts[3] : 1;
    };
    return Array.from(document.querySelectorAll(".atomdown-directive")).map(
      (el, index) => {
        const r = el.getBoundingClientRect();
        const cs = getComputedStyle(el);
        const font = Number.parseFloat(cs.fontSize) || 0;
        const px = (v: string) => Number.parseFloat(v) || 0;
        // A directive line's height budget.
        //
        // Normally 4px. A line that is ALSO a group box edge gets the group's
        // padding on top, and that allowance is not a loosening — it is the
        // difference between the directive and the box drawn on the same
        // element. A group's opening marker measures 8px on a correct build,
        // and every pixel of it is the group's own top inset: the group's top
        // edge is drawn on that line because the marker is the group's first
        // source line. Charging that to the directive reported a visible
        // directive on a build where a reader could see nothing, which is a
        // test that gets switched off rather than a defect that gets fixed.
        // `showsText` is what actually guards the text, and it stays absolute.
        const isGroupEdge =
          el.classList.contains("atomdown-group-first") ||
          el.classList.contains("atomdown-group-last");
        const chrome =
          px(cs.paddingTop) +
          px(cs.paddingBottom) +
          px(cs.borderTopWidth) +
          px(cs.borderBottomWidth);
        const groupPad = isGroupEdge
          ? px(cs.getPropertyValue("--board-group-padding")) +
            px(cs.getPropertyValue("--board-group-border-width"))
          : 0;
        const budget = 4 + chrome + groupPad;
        return {
          index,
          height: r.height,
          budget,
          overBudget: r.height > budget,
          showsText:
            r.height > 0.5 &&
            font > 0.5 &&
            alpha(cs.color) > 0.01 &&
            cs.visibility !== "hidden" &&
            Number.parseFloat(cs.opacity || "1") > 0.01,
          text: (el.textContent ?? "").slice(0, 60),
          rect: {
            x: r.x,
            y: r.y,
            width: r.width,
            height: r.height,
            top: r.top,
            right: r.right,
            bottom: r.bottom,
            left: r.left,
          },
        };
      },
    );
  });
}

/**
 * How many directive reveals are showing.
 *
 * Two things can reveal one: the directive line itself un-collapsing, or the
 * `atomdown-directive-peek` span the plug puts in the card header. Both count.
 */
export async function revealedDirectives(page: Page): Promise<{
  lines: DirectiveState[];
  peeks: { text: string; rect: Rect }[];
}> {
  const lines = (await directiveStates(page)).filter(
    (d) => d.overBudget || d.showsText,
  );
  const peeks = await page.evaluate(() =>
    Array.from(document.querySelectorAll(".atomdown-directive-peek"))
      .filter((el) => {
        const r = el.getBoundingClientRect();
        return (
          r.width > 0.5 &&
          r.height > 0.5 &&
          getComputedStyle(el).display !== "none"
        );
      })
      .map((el) => {
        const r = el.getBoundingClientRect();
        return {
          text: (el.textContent ?? "").slice(0, 80),
          rect: {
            x: r.x,
            y: r.y,
            width: r.width,
            height: r.height,
            top: r.top,
            right: r.right,
            bottom: r.bottom,
            left: r.left,
          },
        };
      }),
  );
  return { lines, peeks };
}

/**
 * Is anything revealed right now? One evaluate, no settle, no rect payload.
 *
 * Rule 2 asks this once per card, and the detailed version costs two
 * round trips plus three animation frames each — 84 cards took long enough
 * that the test hit its own timeout. This is the hot-path form: it returns a
 * count, and the caller reaches for `revealedDirectives` only to build the
 * failure message. The interaction that precedes it has already settled.
 */
/**
 * Is the text cursor sitting on a directive line, with the editor focused?
 *
 * This is the ONE condition under which a reveal is correct, so it is also
 * the one exemption rule 2's hover sweep has to grant. Clicking a collapse
 * caret focuses the editor and moves the cursor onto the group's own opening
 * marker, which reveals that marker legitimately — the first version of rule 2
 * reported it as a leak, which would have been a false alarm on every run.
 */
export async function cursorIsOnDirective(page: Page): Promise<boolean> {
  return page.evaluate(() => {
    const view = (globalThis as any).client?.editorView;
    if (!view) return false;
    const focused =
      view.hasFocus ||
      document.activeElement === view.contentDOM ||
      view.contentDOM.contains(document.activeElement);
    if (!focused) return false;
    const line = view.state.doc.lineAt(view.state.selection.main.head);
    return /^\s*<!--\s*<\/?atom/.test(line.text);
  });
}

export async function revealedCount(page: Page): Promise<number> {
  return page.evaluate(
    ({ maxHeight }) => {
      const alpha = (c: string) => {
        const m = c.match(/rgba?\(([^)]+)\)/);
        if (!m) return 1;
        const p = m[1].split(",").map((s) => Number.parseFloat(s));
        return p.length > 3 ? p[3] : 1;
      };
      let n = 0;
      for (const el of document.querySelectorAll(".atomdown-directive")) {
        const r = el.getBoundingClientRect();
        const px = (v: string) => Number.parseFloat(v) || 0;
        const cs0 = getComputedStyle(el);
        const isGroupEdge =
          el.classList.contains("atomdown-group-first") ||
          el.classList.contains("atomdown-group-last");
        const budget =
          maxHeight +
          px(cs0.paddingTop) +
          px(cs0.paddingBottom) +
          px(cs0.borderTopWidth) +
          px(cs0.borderBottomWidth) +
          (isGroupEdge
            ? px(cs0.getPropertyValue("--board-group-padding")) +
              px(cs0.getPropertyValue("--board-group-border-width"))
            : 0);
        if (r.height > budget) {
          n++;
          continue;
        }
        const cs = getComputedStyle(el);
        if (
          r.height > 0.5 &&
          (Number.parseFloat(cs.fontSize) || 0) > 0.5 &&
          alpha(cs.color) > 0.01 &&
          cs.visibility !== "hidden" &&
          Number.parseFloat(cs.opacity || "1") > 0.01
        ) {
          n++;
        }
      }
      for (const el of document.querySelectorAll(".atomdown-directive-peek")) {
        const r = el.getBoundingClientRect();
        if (
          r.width > 0.5 &&
          r.height > 0.5 &&
          getComputedStyle(el).display !== "none"
        ) {
          n++;
        }
      }
      return n;
    },
    { maxHeight: DIRECTIVE_MAX_HEIGHT },
  );
}

/**
 * Put the text cursor on the first line matching `needle`, with the editor
 * focused, and return that line's own card rect.
 *
 * The focus half matters. The plug reveals a directive only when the cursor is
 * on its line AND the editor has focus, because SilverBullet parks the cursor
 * at offset 0 on a page load — which is the document marker's own line — and
 * without the focus condition that one directive would always be revealed on
 * arrival and read as a bug.
 *
 * `client.editorView` is the same handle `silverbullet/e2e/cursor-reset.test.ts`
 * drives, so this is the real selection path and not a plug internal.
 */
export async function putCursorOnLine(
  page: Page,
  needle: string,
): Promise<{ lineNumber: number; cardRect: Rect | null }> {
  const result = await page.evaluate((n) => {
    const view = (globalThis as any).client.editorView;
    const doc = view.state.doc;
    for (let i = 1; i <= doc.lines; i++) {
      const line = doc.line(i);
      if (line.text.includes(n)) {
        // `scrollIntoView` is not optional. CodeMirror only puts
        // `cm-activeLine` on a line it has realised, and the reveal is keyed
        // on that class, so setting the selection on an off-screen line left
        // the directive hidden and rule 2 reported its own reveal as broken.
        view.dispatch({
          selection: { anchor: line.from },
          scrollIntoView: true,
        });
        view.focus();
        return i;
      }
    }
    return -1;
  }, needle);
  if (result < 0) throw new Error(`no line contains ${JSON.stringify(needle)}`);
  await settle(page, 5);

  // The card that line belongs to, as the reader sees it: the header widget
  // that draws its top edge, plus its body lines.
  const cardRect = await page.evaluate(() => {
    const active = document.querySelector(".cm-line.cm-activeLine");
    if (!active) return null;
    const parts: Element[] = [];
    // Walk back to the card header, then forward to the `-last` line.
    let el: Element | null = active;
    while (el && !el.classList.contains("atomdown-card-header")) {
      el = el.previousElementSibling;
    }
    if (!el) return null;
    parts.push(el);
    let cur = el.nextElementSibling;
    while (cur) {
      parts.push(cur);
      if (cur.classList.contains("atomdown-card-last")) break;
      cur = cur.nextElementSibling;
    }
    const rs = parts.map((p) => p.getBoundingClientRect());
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
  });
  return { lineNumber: result, cardRect };
}

// ---------------------------------------------------------------------------
// Visible text (rule 5)
// ---------------------------------------------------------------------------

/**
 * The text a reader can actually see under `rootSelector`.
 *
 * NOT `textContent`, and the difference is the whole point. Both views keep
 * text in the DOM that nobody can see:
 *
 *   - The inline view hides a directive by collapsing its line to 0px with
 *     `font-size: 0` and a transparent colour, deliberately, so the line
 *     stays in the layout and CodeMirror's coordinate maths is untouched. Its
 *     characters are still in `textContent`.
 *   - The board builds BOTH a rendered body and a raw body for every card and
 *     shows one of them. The hidden one's markdown is still in `textContent`.
 *
 * So a rule 5 written against `textContent` reported 13 atom directives and 14
 * unrendered links on a page where a reader could see none of them. Walking
 * the tree and skipping invisible subtrees is what makes the assertion mean
 * "the reader sees no raw markdown" — and it stays strict in the direction
 * that matters, because the moment a directive becomes visible its text enters
 * this collection and the rule fails.
 *
 * `dropSelector` removes matching subtrees before the walk. Rule 5 uses it for
 * code constructs, which may legitimately contain `##` or `**`.
 */
export async function visibleText(
  ev: Page | Frame,
  rootSelector: string,
  dropSelector = "",
): Promise<string> {
  return ev.evaluate(
    ({ root, drop }) => {
      const start = document.querySelector(root);
      if (!start) return "";
      const dropped = new Set<Element>();
      if (drop) {
        for (const el of start.querySelectorAll(drop)) dropped.add(el);
      }
      const out: string[] = [];
      const walk = (node: Node) => {
        if (node.nodeType === Node.TEXT_NODE) {
          out.push(node.textContent ?? "");
          return;
        }
        if (node.nodeType !== Node.ELEMENT_NODE) return;
        const el = node as Element;
        if (dropped.has(el)) return;
        const cs = getComputedStyle(el);
        if (
          cs.display === "none" ||
          cs.visibility === "hidden" ||
          Number.parseFloat(cs.opacity || "1") < 0.01 ||
          (Number.parseFloat(cs.fontSize) || 0) < 0.5
        ) {
          return;
        }
        const r = el.getBoundingClientRect();
        // A zero-height box shows nothing. This is what excludes a collapsed
        // directive line without needing to know its class name.
        if (r.height < 0.5 || r.width < 0.5) return;
        for (const child of Array.from(el.childNodes)) walk(child);
      };
      for (const child of Array.from(start.childNodes)) walk(child);
      return out.join("");
    },
    { root: rootSelector, drop: dropSelector },
  );
}

// ---------------------------------------------------------------------------
// DOM signatures (rules 3 and 4)
// ---------------------------------------------------------------------------

export type Signature = {
  /** One entry per box: its classes, its rect rounded to a pixel, its text. */
  entries: string[];
  count: number;
};

/**
 * A comparable fingerprint of one view's rendered state.
 *
 * Class lists, box rects and visible text, exactly as rule 4 specifies. Rects
 * are rounded to whole pixels: a sub-pixel difference after a toggle is
 * browser rounding, not a state-machine bug, and a signature that reported it
 * would be a test nobody trusts.
 *
 * Rects are recorded RELATIVE to the scroll container, so a signature taken
 * before a toggle and one taken after are comparable even if the toggle
 * changed the scroll position.
 */
export async function signature(view: View): Promise<Signature> {
  const selector =
    view.kind === "inline"
      ? ".atomdown-card-header,.atomdown-group-header,.atomdown-card-line,.atomdown-group-line"
      : ".board-card,.board-group,.board-card-header,.board-group-header";
  const scroller = view.kind === "inline" ? ".cm-scroller" : ".board-cards";
  await settle(view.page);
  const entries = await view.ev.evaluate(
    ({ sel, scr }) => {
      const base = document.querySelector(scr);
      const origin = base?.getBoundingClientRect() ?? { left: 0, top: 0 };
      const offset = base?.scrollTop ?? 0;
      return Array.from(document.querySelectorAll(sel)).map((el) => {
        const r = el.getBoundingClientRect();
        const classes = String(el.className).trim().split(/\s+/).sort().join(".");
        const text = (el.textContent ?? "").replace(/\s+/g, " ").trim().slice(0, 80);
        const round = (v: number) => Math.round(v);
        return [
          classes,
          round(r.left - origin.left),
          round(r.top - origin.top + offset),
          round(r.width),
          round(r.height),
          text,
        ].join("|");
      });
    },
    { sel: selector, scr: scroller },
  );
  return { entries, count: entries.length };
}

/** The first difference between two signatures, for a failure message. */
export function signatureDiff(a: Signature, b: Signature): string[] {
  const out: string[] = [];
  if (a.count !== b.count) {
    out.push(`element count ${a.count} -> ${b.count}`);
  }
  const max = Math.min(a.entries.length, b.entries.length);
  for (let i = 0; i < max && out.length < 10; i++) {
    if (a.entries[i] !== b.entries[i]) {
      out.push(`[${i}] before: ${a.entries[i]}`);
      out.push(`[${i}] after:  ${b.entries[i]}`);
    }
  }
  return out;
}

/**
 * The top of one named card, relative to the document rather than the
 * viewport, so it survives a scroll.
 *
 * Rule 3's whole question is "did this card move", and a viewport-relative
 * measurement answers a different question whenever an interaction also
 * scrolls.
 */
export async function cardTop(view: View, id: string): Promise<number | null> {
  const scroller = view.kind === "inline" ? ".cm-scroller" : ".board-cards";

  // Find the card first, by scrolling to it.
  //
  // Inline virtualises, so a reference card 60 atoms down the page is simply
  // not in the DOM until it is on screen, and the first version of rule 3
  // reported "reference card not found" rather than measuring anything. The
  // returned position is scroll-independent, so scrolling to find the card
  // does not change the number this reports — which is exactly why the
  // measurement is document-relative in the first place.
  if (view.kind === "inline") {
    await view.ev.evaluate(
      async ({ id, scr }) => {
        const el = document.querySelector(scr)!;
        const has = () =>
          Array.from(document.querySelectorAll(".atomdown-card-id")).some(
            (c) => c.textContent?.trim() === id,
          );
        if (has()) return;
        for (let y = 0; y <= el.scrollHeight; y += 300) {
          el.scrollTop = y;
          await new Promise((r) => requestAnimationFrame(() => r(null)));
          await new Promise((r) => requestAnimationFrame(() => r(null)));
          if (has()) return;
        }
      },
      { id, scr: scroller },
    );
  }

  await settle(view.page, 4);
  return view.ev.evaluate(
    ({ id, scr, kind }) => {
      const base = document.querySelector(scr);
      const origin = base?.getBoundingClientRect().top ?? 0;
      const offset = base?.scrollTop ?? 0;
      let el: Element | null = null;
      if (kind === "board") {
        el = document.querySelector(`.board-card[data-atom-id="${id}"]`);
      } else {
        const chip = Array.from(
          document.querySelectorAll(".atomdown-card-id"),
        ).find((c) => c.textContent?.trim() === id);
        el = chip?.closest(".atomdown-card-header") ?? null;
      }
      if (!el) return null;
      return el.getBoundingClientRect().top - origin + offset;
    },
    { id, scr: scroller, kind: view.kind },
  );
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
