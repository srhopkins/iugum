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
 * The seam supplies six capabilities:
 *
 *  1. `lines`    - CSS classes on the lines of arbitrary top-level blocks,
 *                  selected by Lezer node name. Same shape the client already
 *                  uses for its own built-in line classes.
 *  2. `marks`    - a class over an arbitrary source range, plus per-line
 *                  first/middle/last classes so a caller can draw one
 *                  continuous outline across a multi-line group.
 *  3. `widgets`  - a rendered element attached before or after a block, or
 *                  inline at one offset, with no fenced code block and no
 *                  `${}` directive required.
 *  4. `events`   - `editor:decorationClick` and `editor:decorationSelect` app
 *                  events, so interactive features do not each need their own
 *                  DOM listener.
 *  5. `folds`    - a source range the editor's own folding can collapse, for a
 *                  region that is not a syntax node.
 *  6. `gestures` - a pointer drag of one decorated range onto another
 *                  (`editor:decorationDrag`) and a rubber-band sweep over
 *                  decorated ranges (`editor:decorationLasso`). The seam owns
 *                  the pointer tracking and the drag/band feedback; it never
 *                  changes the document. Whatever receives the event decides
 *                  what the gesture means and writes the document itself.
 *
 * `marks` and `widgets` carry source offsets. Those offsets are read once when
 * the editor state is built and are then mapped through later edits, so they
 * stay attached to the text the caller pointed at.
 *
 * A writer can also replace the whole `editorDecorations` value while the page
 * stays open. The seam notices the new value on the next transaction and
 * rebuilds marks and widgets from it, so a caller that just rewrote the
 * document does NOT have to call `editor.rebuildEditorState`. That matters:
 * `rebuildEditorState` calls `setState`, which discards the undo history, and
 * a caller whose own edit went through the editor as one transaction wants
 * that edit to stay undoable.
 *
 * No CSS ships with the seam. Classes are styled from `space-style`.
 */
import {
  type EditorState,
  type Extension,
  type Range,
  StateEffect,
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
import { foldService } from "@codemirror/language";
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

/** A widget: a rendered element before or after the block at `at`, or inline. */
export type DecorationWidgetRule = {
  /** Any source offset inside the target block's first or last line. */
  at: number;
  /** HTML for the element's content. */
  html: string;
  /** CSS class on the element. */
  class?: string;
  /** Place the element above (default) or below the line at `at`. */
  side?: "before" | "after";
  /**
   * Render the element inline at exactly `at`, on the same line as the text,
   * instead of as its own block above or below the line. `side` then decides
   * whether it sits before or after the character at that offset. An inline
   * widget is the only way to attach a per-block affordance - a drag handle,
   * for example - without spending a line on it.
   */
  inline?: boolean;
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

/** A keyboard modifier that arms a pointer gesture. */
export type DecorationModifier = "alt" | "shift" | "ctrl" | "meta" | "none";

/**
 * Drag one decorated range onto another. Off unless configured.
 *
 * At least one of `handleClass` and `modifier` must be usable, or no drag can
 * ever start. A handle is the kinder route: the caller renders an inline
 * widget with that class and the user drags it, so no plain text drag is
 * intercepted and no modifier has to be discovered.
 */
export type DecorationDragConfig = {
  /** A press on an element carrying this class starts a drag. */
  handleClass?: string;
  /** A press with this modifier held, inside a decorated range, starts a drag. */
  modifier?: DecorationModifier;
};

/** Sweep a rubber band over decorated ranges. Off unless configured. */
export type DecorationLassoConfig = {
  /** The modifier that arms the band. Defaults to `alt`. */
  modifier?: DecorationModifier;
};

export type DecorationGestureConfig = {
  drag?: DecorationDragConfig;
  lasso?: DecorationLassoConfig;
};

/**
 * A source range the reader can collapse.
 *
 * CodeMirror already folds a syntax node it knows how to fold. A caller whose
 * regions are not syntax nodes - a run of blocks it grouped itself, say - has
 * no way to say "this is one collapsible region". A `folds` entry says exactly
 * that, and the region then folds through the editor's own folding: the fold
 * gutter, the fold and unfold commands, and the `editor.fold` syscall. The
 * seam adds no collapse machinery of its own.
 */
export type DecorationFoldRule = {
  /** Where the hidden part starts. Normally the end of the region's first line. */
  from: number;
  /** Where the hidden part ends. */
  to: number;
};

export type DecorationConfig = {
  lines: DecorationLineRule[];
  marks: DecorationMarkRule[];
  widgets: DecorationWidgetRule[];
  folds: DecorationFoldRule[];
  events: DecorationEventConfig;
  gestures: DecorationGestureConfig;
};

export const emptyDecorationConfig: DecorationConfig = {
  lines: [],
  marks: [],
  widgets: [],
  folds: [],
  events: {},
  gestures: {},
};

/** Payload of the `editor:decorationClick` app event. */
export type DecorationClickEvent = {
  page: string;
  pos: number;
  /** 1-based line number of the clicked line. */
  line: number;
  /** Classes on the clicked line's DOM element. */
  lineClasses: string[];
  /**
   * Classes on the element that was clicked, nearest first, up to the line.
   * This is what lets one widget carry several controls: the receiver reads
   * the class it put on the control instead of needing a widget each.
   */
  classes: string[];
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

/** One decorated range, at its position in the document right now. */
export type DecorationRangeRef = {
  from: number;
  to: number;
  /** `id` (or `class`) of the mark. */
  name: string;
};

/**
 * Payload of the `editor:decorationDrag` app event: a decorated range was
 * dragged and released at a new position.
 *
 * `marks` and `targetMarks` are ordered outermost first, so a receiver that
 * nests ranges (a group of blocks around each block) reads element 0 and gets
 * the outer one. The document is unchanged when this event fires - the seam
 * reports the gesture and nothing else.
 */
export type DecorationDragEvent = {
  page: string;
  /** Source range that was picked up. */
  from: number;
  to: number;
  /** Names of the marks covering the pick-up point, outermost first. */
  marks: string[];
  /** Source range released on. Equals the line range when no mark covers it. */
  targetFrom: number;
  targetTo: number;
  /** Names of the marks covering the release point, outermost first. */
  targetMarks: string[];
  /** 1-based line number released on. */
  targetLine: number;
  /** Which side of the target range the pointer was released on. */
  placement: "before" | "after";
  metaKey: boolean;
  ctrlKey: boolean;
  altKey: boolean;
  shiftKey: boolean;
};

/**
 * Payload of the `editor:decorationLasso` app event: a rubber band was swept
 * over the page and released.
 *
 * The band selects whole lines, because the ranges a caller decorates are
 * blocks. `ranges` holds every decorated range the swept lines touch, ordered
 * outermost first, and `marks` is the same list by name.
 */
export type DecorationLassoEvent = {
  page: string;
  /** Start of the first swept line. */
  from: number;
  /** End of the last swept line. */
  to: number;
  /** 1-based line numbers of the swept span. */
  fromLine: number;
  toLine: number;
  marks: string[];
  ranges: DecorationRangeRef[];
  metaKey: boolean;
  ctrlKey: boolean;
  altKey: boolean;
  shiftKey: boolean;
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
      ...(entry.inline === true ? { inline: true } : {}),
      ...(nonEmptyString(entry.class) ? { class: entry.class as string } : {}),
      ...(nonEmptyString(entry.id) ? { id: entry.id as string } : {}),
    });
  }

  const folds: DecorationFoldRule[] = [];
  for (const entry of asArray(value.folds)) {
    if (!isRecord(entry)) continue;
    const from = offset(entry.from);
    const to = offset(entry.to);
    if (from === undefined || to === undefined || to <= from) continue;
    folds.push({ from, to });
  }

  const eventsValue = isRecord(value.events) ? value.events : {};
  const events: DecorationEventConfig = {
    ...(eventsValue.click === true ? { click: true } : {}),
    ...(eventsValue.selection === true ? { selection: true } : {}),
  };

  return {
    lines,
    marks,
    widgets,
    folds,
    events,
    gestures: gestures(value.gestures),
  };
}

const MODIFIERS: DecorationModifier[] = [
  "alt",
  "shift",
  "ctrl",
  "meta",
  "none",
];

function modifier(value: unknown): DecorationModifier | undefined {
  return MODIFIERS.includes(value as DecorationModifier)
    ? (value as DecorationModifier)
    : undefined;
}

/** Normalize the `gestures` section. An unusable entry is dropped whole. */
function gestures(value: unknown): DecorationGestureConfig {
  if (!isRecord(value)) return {};
  const out: DecorationGestureConfig = {};

  if (isRecord(value.drag)) {
    const handleClass = nonEmptyString(value.drag.handleClass);
    const mod = modifier(value.drag.modifier);
    // A drag config with neither a handle nor a modifier could never fire, so
    // it is dropped rather than installing a plugin that does nothing.
    if (handleClass || (mod && mod !== "none")) {
      out.drag = {
        ...(handleClass ? { handleClass } : {}),
        ...(mod ? { modifier: mod } : {}),
      };
    }
  }

  if (isRecord(value.lasso)) {
    const mod = modifier(value.lasso.modifier) ?? "alt";
    if (mod !== "none") out.lasso = { modifier: mod };
  }

  return out;
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

  /**
   * Let a press and a click through to the editor's handlers.
   *
   * CodeMirror ignores every DOM event that starts inside a widget, which is
   * the right default for a widget that renders content. It is the wrong
   * default here: a seam widget is an affordance, so a click on it and a drag
   * from it are the only reasons it exists, and with the default the click
   * event and the drag gesture never reach the seam's own handlers at all.
   * Every other event type stays ignored, so typing and pasting inside a
   * widget still are not editor input.
   */
  override ignoreEvent(event: Event): boolean {
    return event.type !== "mousedown" && event.type !== "click";
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
    const at = Math.min(widget.at, docLength);
    const before = widget.side !== "after";
    const spec = {
      widget: new DecorationWidget(widget.html, widget.class, widget.id),
      side: before ? -1 : 1,
    };
    if (widget.inline) {
      decorations.push(
        Decoration.widget({ ...spec, block: false }).range(at),
      );
      continue;
    }
    const line = state.doc.lineAt(at);
    decorations.push(
      Decoration.widget({ ...spec, block: true }).range(
        before ? line.from : line.to,
      ),
    );
  }

  return Decoration.set(decorations, true);
}

/**
 * The configured marks as live ranges, clamped to the document. Exported for
 * tests. Kept next to the decorations in one StateField so both are mapped
 * through the same edits and can never disagree about where a mark is.
 */
export function buildMarkRanges(
  state: EditorState,
  config: DecorationConfig,
): DecorationRangeRef[] {
  const docLength = state.doc.length;
  return config.marks.map((mark) => ({
    from: Math.min(mark.from, docLength),
    to: Math.min(mark.to, docLength),
    name: ruleName(mark),
  }));
}

/**
 * Everything the seam derives from one `editorDecorations` value: the config it
 * came from, the decorations, and the mark ranges as they sit right now.
 */
type SeamState = {
  config: DecorationConfig;
  /** The raw config value this was built from, for change detection. */
  source: unknown;
  decorations: DecorationSet;
  marks: DecorationRangeRef[];
  folds: DecorationFoldRule[];
};

/** The configured fold regions, clamped to the document. Exported for tests. */
export function buildFoldRanges(
  state: EditorState,
  config: DecorationConfig,
): DecorationFoldRule[] {
  const docLength = state.doc.length;
  return config.folds
    .map((fold) => ({
      from: Math.min(fold.from, docLength),
      to: Math.min(fold.to, docLength),
    }))
    .filter((fold) => fold.to > fold.from);
}

/**
 * Mark and widget decorations live in their own StateField so their offsets can
 * be mapped through edits, instead of being recomputed from now-stale config on
 * every transaction.
 *
 * The field also watches the config key. A writer that replaces the value while
 * the page is open gets picked up on the next transaction, without a
 * `rebuildEditorState` and therefore without losing the undo history. Change
 * detection is by value identity, which is what a `config.set` of a fresh
 * object produces.
 */
function seamStateField(client: Client) {
  const read = () => client.config.get<unknown>(decorationConfigKey, undefined);
  const build = (state: EditorState, source: unknown): SeamState => {
    const config = normalizeDecorationConfig(source);
    return {
      config,
      source,
      decorations: buildRangeDecorations(state, config),
      marks: buildMarkRanges(state, config),
      folds: buildFoldRanges(state, config),
    };
  };
  return StateField.define<SeamState>({
    create: (state: EditorState) => build(state, read()),
    update: (value: SeamState, tr: Transaction) => {
      const source = read();
      if (source !== value.source) return build(tr.state, source);
      if (!tr.docChanged) return value;
      return {
        ...value,
        decorations: value.decorations.map(tr.changes),
        marks: value.marks.map((mark) => ({
          from: tr.changes.mapPos(mark.from, 1),
          to: tr.changes.mapPos(mark.to, -1),
          name: mark.name,
        })),
        folds: value.folds.map((fold) => ({
          from: tr.changes.mapPos(fold.from, 1),
          to: tr.changes.mapPos(fold.to, -1),
        })),
      };
    },
    provide: (f) => EditorView.decorations.from(f, (value) => value.decorations),
  });
}

/**
 * Marks overlapping [from, to), by the name they report on events, outermost
 * first. `live` is the mapped range list when the seam's field is installed;
 * the config offsets are the fallback for a state built without it.
 */
export function marksIn(
  ranges: DecorationRangeRef[],
  from: number,
  to: number,
): DecorationRangeRef[] {
  return ranges
    .filter((mark) => mark.from <= to && from <= mark.to && mark.name.length > 0)
    .sort((a, b) => (b.to - b.from) - (a.to - a.from) || a.from - b.from);
}

function markNames(ranges: DecorationRangeRef[]): string[] {
  return ranges.map((mark) => mark.name);
}

/** The seam's live mark ranges for a state, whatever built that state. */
function liveMarks(
  state: EditorState,
  field: StateField<SeamState>,
  config: DecorationConfig,
): DecorationRangeRef[] {
  return state.field(field, false)?.marks ?? buildMarkRanges(state, config);
}

function clickHandler(
  client: Client,
  pageName: string,
  config: DecorationConfig,
  field: StateField<SeamState>,
) {
  return EditorView.domEventHandlers({
    click: (event: MouseEvent, view: EditorView) => {
      if (event.button !== 0) return;
      const pos = view.posAtCoords({ x: event.clientX, y: event.clientY });
      if (pos === null) return;
      let lineClasses: string[] = [];
      const classes: string[] = [];
      let widget: string | undefined;
      if (event.target instanceof Element) {
        for (
          let el: Element | null = event.target;
          el && !el.classList.contains("cm-line") &&
          !el.classList.contains("cm-content");
          el = el.parentElement
        ) {
          for (const cls of Array.from(el.classList)) {
            if (!classes.includes(cls)) classes.push(cls);
          }
        }
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
        classes,
        marks: markNames(
          marksIn(liveMarks(view.state, field, config), pos, pos),
        ),
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
  field: StateField<SeamState>,
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
          marks: markNames(
            marksIn(liveMarks(update.state, field, config), from, to),
          ),
        };
        safeRun(async () => {
          await client.dispatchAppEvent("editor:decorationSelect", payload);
        });
      }
    },
  );
}

// ---------------------------------------------------------------------------
// Gestures.
//
// Two pointer gestures the browser has no primitive for: dragging one
// decorated range onto another, and sweeping a rubber band over several.
//
// The seam does the part only the client can do - hit-testing document
// positions, drawing the drop marker and the band - and then reports what
// happened. It never writes to the document. The receiver of the event owns
// the meaning of the gesture and owns the edit, which is what keeps the
// document mutation outside the client and lets that edit be one transaction
// and therefore one native undo step.
// ---------------------------------------------------------------------------

/** How far the pointer must travel before a press counts as a drag. */
const DRAG_THRESHOLD_PX = 4;

/** Where the drop marker sits, and which lines are being carried. */
type DropHint = {
  at: number;
  placement: "before" | "after";
  originFrom: number;
  originTo: number;
} | null;

const setDropHint = StateEffect.define<DropHint>();

/**
 * Feedback for a drag in progress: one class on the line the drop would land
 * against, and one on every line being carried. Both are presentation only and
 * disappear when the gesture ends.
 */
function dropHintField() {
  return StateField.define<DecorationSet>({
    create: () => Decoration.none,
    update(value: DecorationSet, tr: Transaction) {
      let next = tr.docChanged ? value.map(tr.changes) : value;
      for (const effect of tr.effects) {
        if (!effect.is(setDropHint)) continue;
        const hint = effect.value;
        if (!hint) {
          next = Decoration.none;
          continue;
        }
        const doc = tr.state.doc;
        const ranges: Range<Decoration>[] = [];
        const first = doc.lineAt(Math.min(hint.originFrom, doc.length)).number;
        const last = doc.lineAt(Math.min(hint.originTo, doc.length)).number;
        for (let n = first; n <= last; n++) {
          ranges.push(
            Decoration.line({ class: "sb-decoration-dragging" }).range(
              doc.line(n).from,
            ),
          );
        }
        ranges.push(
          Decoration.line({
            class: hint.placement === "before"
              ? "sb-decoration-drop-before"
              : "sb-decoration-drop-after",
          }).range(doc.lineAt(Math.min(hint.at, doc.length)).from),
        );
        next = Decoration.set(ranges, true);
      }
      return next;
    },
    provide: (f) => EditorView.decorations.from(f),
  });
}

function modifierHeld(mod: DecorationModifier, event: MouseEvent): boolean {
  switch (mod) {
    case "alt":
      return event.altKey;
    case "shift":
      return event.shiftKey;
    case "ctrl":
      return event.ctrlKey;
    case "meta":
      return event.metaKey;
    default:
      return false;
  }
}

/** The decorated range at `pos`, or that whole line when nothing covers it. */
function rangeAt(
  view: EditorView,
  ranges: DecorationRangeRef[],
  pos: number,
): { from: number; to: number; marks: DecorationRangeRef[] } {
  const covering = marksIn(ranges, pos, pos);
  if (covering.length > 0) {
    return {
      from: covering[0].from,
      to: covering[0].to,
      marks: covering,
    };
  }
  const line = view.state.doc.lineAt(pos);
  return { from: line.from, to: line.to, marks: [] };
}

/**
 * Which side of a range the pointer sits on: above its vertical midpoint is
 * `before`, below it is `after`. Vertical because the ranges a caller
 * decorates are blocks, and a block's neighbours are above and below it.
 */
function placementFor(
  view: EditorView,
  range: { from: number; to: number },
  clientY: number,
): "before" | "after" {
  const top = view.coordsAtPos(range.from);
  const bottom = view.coordsAtPos(range.to);
  if (!top || !bottom) return "after";
  return clientY < (top.top + bottom.bottom) / 2 ? "before" : "after";
}

/** Nearest document position for a viewport point, clamped to the document. */
function posAt(view: EditorView, x: number, y: number): number {
  const pos = view.posAtCoords({ x, y }, false);
  return Math.max(0, Math.min(pos, view.state.doc.length));
}

function gestureHandlers(
  client: Client,
  pageName: string,
  config: DecorationConfig,
  field: StateField<SeamState>,
) {
  const drag = config.gestures.drag;
  const lasso = config.gestures.lasso;

  return ViewPlugin.fromClass(
    class {
      /** Non-null while a drag is running. */
      private dragging: {
        from: number;
        to: number;
        marks: DecorationRangeRef[];
        startX: number;
        startY: number;
        moved: boolean;
      } | null = null;
      /** Non-null while a band is being swept. */
      private band: {
        startX: number;
        startY: number;
        el: HTMLDivElement;
      } | null = null;
      private onMove = (event: MouseEvent) => this.move(event);
      private onUp = (event: MouseEvent) => this.up(event);

      constructor(readonly view: EditorView) {}

      destroy() {
        this.stop();
      }

      /** Called by the plugin's own mousedown handler. */
      down(event: MouseEvent): boolean {
        if (event.button !== 0 || this.dragging || this.band) return false;
        const target = event.target instanceof Element ? event.target : null;

        if (drag) {
          let origin: number | null = null;
          const handleEl = drag.handleClass && target
            ? target.closest(`.${CSS.escape(drag.handleClass)}`)
            : null;
          if (handleEl) {
            try {
              origin = this.view.posAtDOM(handleEl);
            } catch {
              origin = null;
            }
          } else if (drag.modifier && modifierHeld(drag.modifier, event)) {
            origin = posAt(this.view, event.clientX, event.clientY);
          }
          if (origin !== null) {
            const marks = liveMarks(this.view.state, field, config);
            const range = rangeAt(this.view, marks, origin);
            this.dragging = {
              from: range.from,
              to: range.to,
              marks: range.marks,
              startX: event.clientX,
              startY: event.clientY,
              moved: false,
            };
            this.listen();
            return true;
          }
        }

        if (lasso && modifierHeld(lasso.modifier ?? "alt", event)) {
          const el = document.createElement("div");
          el.className = "sb-decoration-lasso";
          el.style.position = "absolute";
          el.style.pointerEvents = "none";
          this.view.dom.appendChild(el);
          this.band = { startX: event.clientX, startY: event.clientY, el };
          this.paintBand(event.clientX, event.clientY);
          this.listen();
          return true;
        }

        return false;
      }

      private listen() {
        globalThis.addEventListener("mousemove", this.onMove, true);
        globalThis.addEventListener("mouseup", this.onUp, true);
      }

      private stop() {
        globalThis.removeEventListener("mousemove", this.onMove, true);
        globalThis.removeEventListener("mouseup", this.onUp, true);
        if (this.band) {
          this.band.el.remove();
          this.band = null;
        }
        if (this.dragging) {
          this.dragging = null;
          this.view.dispatch({ effects: setDropHint.of(null) });
        }
      }

      private paintBand(x: number, y: number) {
        if (!this.band) return;
        const host = this.view.dom.getBoundingClientRect();
        const left = Math.min(this.band.startX, x) - host.left;
        const top = Math.min(this.band.startY, y) - host.top;
        const style = this.band.el.style;
        style.left = `${left}px`;
        style.top = `${top}px`;
        style.width = `${Math.abs(x - this.band.startX)}px`;
        style.height = `${Math.abs(y - this.band.startY)}px`;
      }

      private move(event: MouseEvent) {
        if (this.band) {
          this.paintBand(event.clientX, event.clientY);
          return;
        }
        const state = this.dragging;
        if (!state) return;
        if (
          !state.moved &&
          Math.abs(event.clientX - state.startX) < DRAG_THRESHOLD_PX &&
          Math.abs(event.clientY - state.startY) < DRAG_THRESHOLD_PX
        ) {
          return;
        }
        state.moved = true;
        const pos = posAt(this.view, event.clientX, event.clientY);
        const marks = liveMarks(this.view.state, field, config);
        const target = rangeAt(this.view, marks, pos);
        const placement = placementFor(this.view, target, event.clientY);
        this.view.dispatch({
          effects: setDropHint.of({
            at: placement === "before" ? target.from : target.to,
            placement,
            originFrom: state.from,
            originTo: state.to,
          }),
        });
      }

      private up(event: MouseEvent) {
        const band = this.band;
        const drop = this.dragging;
        this.stop();
        if (band) {
          this.finishBand(band, event);
          return;
        }
        if (!drop || !drop.moved) return;
        const pos = posAt(this.view, event.clientX, event.clientY);
        const marks = liveMarks(this.view.state, field, config);
        const target = rangeAt(this.view, marks, pos);
        const payload: DecorationDragEvent = {
          page: pageName,
          from: drop.from,
          to: drop.to,
          marks: markNames(drop.marks),
          targetFrom: target.from,
          targetTo: target.to,
          targetMarks: markNames(target.marks),
          targetLine: this.view.state.doc.lineAt(pos).number,
          placement: placementFor(this.view, target, event.clientY),
          metaKey: event.metaKey,
          ctrlKey: event.ctrlKey,
          altKey: event.altKey,
          shiftKey: event.shiftKey,
        };
        safeRun(async () => {
          await client.dispatchAppEvent("editor:decorationDrag", payload);
        });
      }

      private finishBand(
        band: { startX: number; startY: number },
        event: MouseEvent,
      ) {
        if (
          Math.abs(event.clientX - band.startX) < DRAG_THRESHOLD_PX &&
          Math.abs(event.clientY - band.startY) < DRAG_THRESHOLD_PX
        ) {
          return;
        }
        const content = this.view.contentDOM.getBoundingClientRect();
        const x = content.left + 1;
        const top = Math.min(band.startY, event.clientY);
        const bottom = Math.max(band.startY, event.clientY);
        const doc = this.view.state.doc;
        const firstLine = doc.lineAt(posAt(this.view, x, top));
        const lastLine = doc.lineAt(posAt(this.view, x, bottom));
        const ranges = marksIn(
          liveMarks(this.view.state, field, config),
          firstLine.from,
          lastLine.to,
        );
        const payload: DecorationLassoEvent = {
          page: pageName,
          from: firstLine.from,
          to: lastLine.to,
          fromLine: firstLine.number,
          toLine: lastLine.number,
          marks: markNames(ranges),
          ranges,
          metaKey: event.metaKey,
          ctrlKey: event.ctrlKey,
          altKey: event.altKey,
          shiftKey: event.shiftKey,
        };
        safeRun(async () => {
          await client.dispatchAppEvent("editor:decorationLasso", payload);
        });
      }
    },
    {
      eventHandlers: {
        mousedown(event: MouseEvent) {
          // Returning true makes CodeMirror call preventDefault, which is what
          // stops a lasso sweep from also selecting text.
          return this.down(event);
        },
      },
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
  const wantsGestures = config.gestures.drag !== undefined ||
    config.gestures.lasso !== undefined;
  const wantsField = config.marks.length > 0 || config.widgets.length > 0 ||
    config.folds.length > 0 || config.events.click ||
    config.events.selection || wantsGestures;
  // One field, so the marks the events and the gestures hit-test against are
  // the same objects the decorations were drawn from.
  const field = seamStateField(client);
  if (wantsField) {
    extensions.push(field);
  }
  if (config.folds.length > 0) {
    // The editor's own folding does the collapsing. This only tells it that a
    // caller-named range is one region.
    extensions.push(
      foldService.of((state, lineStart, lineEnd) => {
        const folds = state.field(field, false)?.folds ??
          buildFoldRanges(state, config);
        return folds.find(
          (fold) => fold.from >= lineStart && fold.from <= lineEnd,
        ) ?? null;
      }),
    );
  }
  if (config.events.click) {
    extensions.push(clickHandler(client, pageName, config, field));
  }
  if (config.events.selection) {
    extensions.push(selectionWatcher(client, pageName, config, field));
  }
  if (wantsGestures) {
    extensions.push(
      dropHintField(),
      gestureHandlers(client, pageName, config, field),
    );
  }
  return extensions;
}
