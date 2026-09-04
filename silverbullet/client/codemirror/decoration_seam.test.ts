import { EditorState } from "@codemirror/state";
import { expect, test } from "vitest";
import {
  buildRangeDecorations,
  type DecorationConfig,
  emptyDecorationConfig,
  normalizeDecorationConfig,
} from "./decoration_seam.ts";

function config(partial: Partial<DecorationConfig>): DecorationConfig {
  return { ...emptyDecorationConfig, ...partial };
}

// Collect the decorations of a set as [from, to, spec] triples.
function collect(state: EditorState, cfg: DecorationConfig) {
  const set = buildRangeDecorations(state, cfg);
  const out: { from: number; to: number; class?: string; block: boolean }[] =
    [];
  const cursor = set.iter();
  while (cursor.value) {
    out.push({
      from: cursor.from,
      to: cursor.to,
      class: (cursor.value.spec as any).class,
      block: cursor.value.spec.widget !== undefined,
    });
    cursor.next();
  }
  return out;
}

test("normalize: a non-object is the empty config", () => {
  expect(normalizeDecorationConfig(undefined)).toEqual(emptyDecorationConfig);
  expect(normalizeDecorationConfig("nope")).toEqual(emptyDecorationConfig);
  expect(normalizeDecorationConfig([1, 2])).toEqual(emptyDecorationConfig);
});

test("normalize: line rules keep selector, class and nesting", () => {
  const cfg = normalizeDecorationConfig({
    lines: [
      { selector: "ListItem", class: "ad-item", nesting: true },
      { selector: "ATXHeading2", class: "ad-head" },
    ],
  });
  expect(cfg.lines).toEqual([
    { selector: "ListItem", class: "ad-item", nesting: true },
    { selector: "ATXHeading2", class: "ad-head" },
  ]);
});

test("normalize: a bad entry is dropped, the good ones survive", () => {
  const cfg = normalizeDecorationConfig({
    lines: [
      { selector: "ListItem" },
      { class: "ad-item" },
      "junk",
      { selector: "Task", class: "ad-task" },
    ],
    marks: [
      { from: 5, to: 2, class: "ad-sel" },
      { from: -1, to: 4, class: "ad-sel" },
      { from: 1, to: 4 },
      { from: 1, to: 4, class: "ad-sel", id: "g1" },
    ],
    widgets: [{ at: 3 }, { html: "<b>x</b>" }, { at: 3, html: "<b>x</b>" }],
  });
  expect(cfg.lines).toEqual([{ selector: "Task", class: "ad-task" }]);
  expect(cfg.marks).toEqual([{ from: 1, to: 4, class: "ad-sel", id: "g1" }]);
  expect(cfg.widgets).toEqual([{ at: 3, html: "<b>x</b>", side: "before" }]);
});

test("normalize: events default to off and are opt-in", () => {
  expect(normalizeDecorationConfig({}).events).toEqual({});
  expect(
    normalizeDecorationConfig({ events: { click: true, selection: false } })
      .events,
  ).toEqual({ click: true });
});

test("marks: a single-line range gets one mark and no line classes", () => {
  const state = EditorState.create({ doc: "hello world" });
  const decorations = collect(
    state,
    config({ marks: [{ from: 0, to: 5, class: "ad-sel" }] }),
  );
  expect(decorations).toEqual([
    { from: 0, to: 5, class: "ad-sel", block: false },
  ]);
});

test("marks: lineClasses gives first, mid and last across lines", () => {
  //          0123 4567 89..
  const doc = "aaa\nbbb\nccc\nddd";
  const state = EditorState.create({ doc });
  const decorations = collect(
    state,
    config({
      marks: [{ from: 0, to: 11, class: "ad-grp", lineClasses: true }],
    }),
  );
  const lineClasses = decorations
    .filter((d) => d.from === d.to)
    .map((d) => d.class);
  expect(lineClasses).toEqual([
    "ad-grp-line ad-grp-first",
    "ad-grp-line ad-grp-mid",
    "ad-grp-line ad-grp-last",
  ]);
  // The 4th line is outside the range.
  expect(lineClasses).toHaveLength(3);
});

test("marks: a single-line range with lineClasses is both first and last", () => {
  const state = EditorState.create({ doc: "aaa\nbbb" });
  const decorations = collect(
    state,
    config({ marks: [{ from: 0, to: 3, class: "ad-one", lineClasses: true }] }),
  );
  expect(
    decorations.filter((d) => d.from === d.to).map((d) => d.class),
  ).toEqual(["ad-one-line ad-one-first ad-one-last"]);
});

test("marks: offsets past the end of the document are clamped", () => {
  const state = EditorState.create({ doc: "aaa" });
  const decorations = collect(
    state,
    config({ marks: [{ from: 0, to: 9999, class: "ad-sel" }] }),
  );
  expect(decorations).toEqual([
    { from: 0, to: 3, class: "ad-sel", block: false },
  ]);
});

test("widgets: side decides the start or the end of the target line", () => {
  const state = EditorState.create({ doc: "aaa\nbbb\nccc" });
  const before = collect(
    state,
    config({ widgets: [{ at: 5, html: "<b>h</b>", side: "before" }] }),
  );
  expect(before).toEqual([{ from: 4, to: 4, class: undefined, block: true }]);

  const after = collect(
    state,
    config({ widgets: [{ at: 5, html: "<b>h</b>", side: "after" }] }),
  );
  expect(after).toEqual([{ from: 7, to: 7, class: undefined, block: true }]);
});

test("marks map through an edit instead of going stale", () => {
  // Same mapping the seam's StateField performs on a doc change.
  const state = EditorState.create({ doc: "aaa\nbbb" });
  const set = buildRangeDecorations(
    state,
    config({ marks: [{ from: 4, to: 7, class: "ad-sel" }] }),
  );
  const tr = state.update({ changes: { from: 0, insert: "XX" } });
  const mapped = set.map(tr.changes);
  const cursor = mapped.iter();
  expect(cursor.from).toBe(6);
  expect(cursor.to).toBe(9);
});
