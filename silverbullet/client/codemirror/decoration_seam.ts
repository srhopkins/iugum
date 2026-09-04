/**
 * The editor decoration seam.
 *
 * ONE extension point that lets code outside the client decorate the rendered
 * page. It reads plain data from the `editorDecorations` config key, so any
 * writer that can reach config can drive it: a Space Lua block (`config.set`)
 * or a plug (the `config.set` syscall followed by `editor.rebuildEditorState`).
 * Plugs run in a web worker with no CodeMirror and no DOM, so they cannot hand
 * the client a CodeMirror extension; config is data, and data crosses that
 * boundary. See `docs/Editor Decorations.md`.
 *
 * The seam supplies four capabilities:
 *
 *  1. `lines`   - CSS classes on the lines of arbitrary top-level blocks,
 *                 selected by Lezer node name. Same shape the client already
 *                 uses for its own built-in line classes.
 *  2. `marks`   - a class over an arbitrary source range, plus per-line
 *                 first/middle/last classes so a caller can draw one
 *                 continuous outline across a multi-line group.
 *  3. `widgets` - a rendered element attached before or after a block, with no
 *                 fenced code block and no `${}` directive required.
 *  4. `events`  - `editor:decorationClick` and `editor:decorationSelect` app
 *                 events, so interactive features do not each need their own
 *                 DOM listener.
 *
 * `marks` and `widgets` carry source offsets. Those offsets are read once when
 * the editor state is built and are then mapped through later edits, so they
 * stay attached to the text the caller pointed at.
 *
 * No CSS ships with the seam. Classes are styled from `space-style`.
 */
import {
  type EditorState,
  type Extension,
  type Range,
  StateField,
  type Transaction,
} from "@codemirror/state";
import {
  Decoration,
  type DecorationSet,
  EditorView,
  ViewPlugin,
  type ViewUpdate,
  WidgetType,
} from "@codemirror/view";
import { safeRun } from "@silverbulletmd/silverbullet/lib/async";
import type { Client } from "../client.ts";
import { lineWrapper } from "./line_wrapper.ts";

/** The config key the seam reads. */
export const decorationConfigKey = "editorDecorations";

/** A line-class rule: apply `class` to every line of every `selector` block. */
export type DecorationLineRule = {
  /** Lezer node name, e.g. "ListItem", "ATXHeading2", "FencedCode". */
  selector: string;
  /** CSS class added to each line of the block. */
  class: string;
  /** Also add `<class>-<depth>` for nested blocks of the same selector. */
  nesting?: boolean;
};

/** A range mark: one class over one source range. */
export type DecorationMarkRule = {
  /** Source offset the mark starts at. */
  from: number;
  /** Source offset the mark ends at. */
  to: number;
  /** CSS class for the mark, and the stem of the per-line classes. */
  class: string;
  /** Optional caller-side name, reported back on events. */
  id?: string;
  /**
   * Add `<class>-line` to every covered line and `<class>-first`,
   * `<class>-mid`, `<class>-last` by position, so a caller can draw a
   * continuous outline around a group that spans several blocks.
   * A single-line range gets both `-first` and `-last`.
   */
  lineClasses?: boolean;
};

/** A block widget: a rendered element before or after the block at `at`. */
export type DecorationWidgetRule = {
  /** Any source offset inside the target block's first or last line. */
  at: number;
  /** HTML for the element's content. */
  html: string;
  /** CSS class on the element. */
  class?: string;
  /** Place the element above (default) or below the line at `at`. */
  side?: "before" | "after";
  /** Caller-side name, reported back on a click inside the widget. */
  id?: string;
};

/** Which app events the seam dispatches. Both default to off. */
export type DecorationEventConfig = {
  /** Dispatch `editor:decorationClick`. */
  click?: boolean;
  /** Dispatch `editor:decorationSelect` when the selection moves. */
  selection?: boolean;
};

export type DecorationConfig = {
  lines: DecorationLineRule[];
  marks: DecorationMarkRule[];
  widgets: DecorationWidgetRule[];
  events: DecorationEventConfig;
};

export const emptyDecorationConfig: DecorationConfig = {
  lines: [],
  marks: [],
  widgets: [],
  events: {},
};

/** Payload of the `editor:decorationClick` app event. */
export type DecorationClickEvent = {
  page: string;
  pos: number;
  /** 1-based line number of the clicked line. */
  line: number;
  /** Classes on the clicked line's DOM element. */
  lineClasses: string[];
  /** `id` (or `class`) of every configured mark covering `pos`. */
  marks: string[];
  /** `id` (or `class`) of the widget clicked, when the click was in one. */
  widget?: string;
  metaKey: boolean;
  ctrlKey: boolean;
  altKey: boolean;
};

/** Payload of the `editor:decorationSelect` app event. */
export type DecorationSelectEvent = {
  page: string;
  from: number;
  to: number;
  /** `id` (or `class`) of every configured mark the selection overlaps. */
  marks: string[];
};

function isRecord(value: unknown): value is Record<string, unknown> {
  return !!value && typeof value === "object" && !Array.isArray(value);
}

function asArray(value: unknown): unknown[] {
  return Array.isArray(value) ? value : [];
}

function nonEmptyString(value: unknown): string | undefined {
  return typeof value === "string" && value.length > 0 ? value : undefined;
}

function offset(value: unknown): number | undefined {
  return typeof value === "number" && Number.isInteger(value) && value >= 0
    ? value
    : undefined;
}

/**
 * Turn whatever sits at the config key into a usable config. Bad entries are
 * dropped, not thrown on: this data comes from user Lua, and one typo must not
 * take the editor down.
 */
export function normalizeDecorationConfig(value: unknown): DecorationConfig {
  if (!isRecord(value)) {
    return { ...emptyDecorationConfig };
  }

  const lines: DecorationLineRule[] = [];
  for (const entry of asArray(value.lines)) {
    if (!isRecord(entry)) continue;
    const selector = nonEmptyString(entry.selector);
    const cls = nonEmptyString(entry.class);
    if (!selector || !cls) continue;
    lines.push({
      selector,
      class: cls,
      ...(entry.nesting === true ? { nesting: true } : {}),
    });
  }

  const marks: DecorationMarkRule[] = [];
  for (const entry of asArray(value.marks)) {
    if (!isRecord(entry)) continue;
    const from = offset(entry.from);
    const to = offset(entry.to);
    const cls = nonEmptyString(entry.class);
    if (from === undefined || to === undefined || !cls || to < from) continue;
    marks.push({
      from,
      to,
      class: cls,
      ...(nonEmptyString(entry.id) ? { id: entry.id as string } : {}),
      ...(entry.lineClasses === true ? { lineClasses: true } : {}),
    });
  }

  const widgets: DecorationWidgetRule[] = [];
  for (const entry of asArray(value.widgets)) {
    if (!isRecord(entry)) continue;
    const at = offset(entry.at);
    const html = typeof entry.html === "string" ? entry.html : undefined;
    if (at === undefined || html === undefined) continue;
    widgets.push({
      at,
      html,
      side: entry.side === "after" ? "after" : "before",
      ...(nonEmptyString(entry.class) ? { class: entry.class as string } : {}),
      ...(nonEmptyString(entry.id) ? { id: entry.id as string } : {}),
    });
  }

  const eventsValue = isRecord(value.events) ? value.events : {};
  const events: DecorationEventConfig = {
    ...(eventsValue.click === true ? { click: true } : {}),
    ...(eventsValue.selection === true ? { selection: true } : {}),
  };

  return { lines, marks, widgets, events };
}

/** A block-level element rendered from caller-supplied HTML. */
export class DecorationWidget extends WidgetType {
  constructor(
    readonly html: string,
    readonly cssClass: string | undefined,
    readonly id: string | undefined,
  ) {
    super();
  }

  toDOM(): HTMLElement {
    const el = document.createElement("div");
    el.className = ["sb-decoration-widget", this.cssClass]
      .filter(Boolean)
      .join(" ");
    if (this.id) {
      el.dataset.decorationId = this.id;
    }
    el.innerHTML = this.html;
    return el;
  }

  override eq(other: WidgetType): boolean {
    return (
      other instanceof DecorationWidget &&
      other.html === this.html &&
      other.cssClass === this.cssClass &&
      other.id === this.id
    );
  }
}

/** Name a mark or widget carries on an event. */
function ruleName(rule: { id?: string; class?: string }): string {
  return rule.id ?? rule.class ?? "";
}

/**
 * Build the mark and widget decorations for one document state. Exported for
 * tests.
 */
export function buildRangeDecorations(
  state: EditorState,
  config: DecorationConfig,
): DecorationSet {
  const decorations: Range<Decoration>[] = [];
  const docLength = state.doc.length;

  for (const mark of config.marks) {
    const from = Math.min(mark.from, docLength);
    const to = Math.min(mark.to, docLength);
    if (to > from) {
      decorations.push(
        Decoration.mark({
          class: mark.class,
          ...(mark.id ? { attributes: { "data-decoration-id": mark.id } } : {}),
        }).range(from, to),
      );
    }
    if (!mark.lineClasses) continue;
    const firstLine = state.doc.lineAt(from).number;
    const lastLine = state.doc.lineAt(to).number;
    for (let n = firstLine; n <= lastLine; n++) {
      const classes = [`${mark.class}-line`];
      if (n === firstLine) classes.push(`${mark.class}-first`);
      if (n === lastLine) classes.push(`${mark.class}-last`);
      if (n !== firstLine && n !== lastLine) {
        classes.push(`${mark.class}-mid`);
      }
      decorations.push(
        Decoration.line({ class: classes.join(" ") }).range(
          state.doc.line(n).from,
        ),
      );
    }
  }

  for (const widget of config.widgets) {
    const line = state.doc.lineAt(Math.min(widget.at, docLength));
    const before = widget.side !== "after";
    decorations.push(
      Decoration.widget({
        widget: new DecorationWidget(widget.html, widget.class, widget.id),
        side: before ? -1 : 1,
        block: true,
      }).range(before ? line.from : line.to),
    );
  }

  return Decoration.set(decorations, true);
}

/**
 * Mark and widget decorations live in their own StateField so their offsets can
 * be mapped through edits, instead of being recomputed from now-stale config on
 * every transaction.
 */
function rangeDecorationField(config: DecorationConfig) {
  return StateField.define<DecorationSet>({
    create: (state: EditorState) => buildRangeDecorations(state, config),
    update: (value: DecorationSet, tr: Transaction) =>
      tr.docChanged ? value.map(tr.changes) : value,
    provide: (f) => EditorView.decorations.from(f),
  });
}

/** Marks overlapping [from, to), by the name they report on events. */
function marksAt(config: DecorationConfig, from: number, to: number): string[] {
  return config.marks
    .filter((mark) => mark.from <= to && from <= mark.to)
    .map(ruleName)
    .filter((name) => name.length > 0);
}

function clickHandler(
  client: Client,
  pageName: string,
  config: DecorationConfig,
) {
  return EditorView.domEventHandlers({
    click: (event: MouseEvent, view: EditorView) => {
      if (event.button !== 0) return;
      const pos = view.posAtCoords({ x: event.clientX, y: event.clientY });
      if (pos === null) return;
      let lineClasses: string[] = [];
      let widget: string | undefined;
      if (event.target instanceof Element) {
        const lineEl = event.target.closest(".cm-line");
        if (lineEl) {
          lineClasses = Array.from(lineEl.classList);
        }
        const widgetEl = event.target.closest(".sb-decoration-widget");
        if (widgetEl instanceof HTMLElement) {
          widget = widgetEl.dataset.decorationId ?? "";
        }
      }
      const payload: DecorationClickEvent = {
        page: pageName,
        pos,
        line: view.state.doc.lineAt(pos).number,
        lineClasses,
        marks: marksAt(config, pos, pos),
        ...(widget !== undefined ? { widget } : {}),
        metaKey: event.metaKey,
        ctrlKey: event.ctrlKey,
        altKey: event.altKey,
      };
      safeRun(async () => {
        await client.dispatchAppEvent("editor:decorationClick", payload);
      });
      // Never swallow the click: the seam observes, it does not intercept.
      return false;
    },
  });
}

function selectionWatcher(
  client: Client,
  pageName: string,
  config: DecorationConfig,
) {
  return ViewPlugin.fromClass(
    class {
      update(update: ViewUpdate) {
        if (!update.selectionSet) return;
        if (update.startState.selection.eq(update.state.selection)) return;
        const { from, to } = update.state.selection.main;
        const payload: DecorationSelectEvent = {
          page: pageName,
          from,
          to,
          marks: marksAt(config, from, to),
        };
        safeRun(async () => {
          await client.dispatchAppEvent("editor:decorationSelect", payload);
        });
      }
    },
  );
}

/**
 * The seam's extensions. Returns an empty array when nothing is configured, so
 * the cost of the seam on an unconfigured space is one config read.
 */
export function decorationSeam(client: Client, pageName: string): Extension[] {
  const config = normalizeDecorationConfig(
    client.config.get<unknown>(decorationConfigKey, undefined),
  );
  const extensions: Extension[] = [];
  if (config.lines.length > 0) {
    extensions.push(lineWrapper(config.lines));
  }
  if (config.marks.length > 0 || config.widgets.length > 0) {
    extensions.push(rangeDecorationField(config));
  }
  if (config.events.click) {
    extensions.push(clickHandler(client, pageName, config));
  }
  if (config.events.selection) {
    extensions.push(selectionWatcher(client, pageName, config));
  }
  return extensions;
}
