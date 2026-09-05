# The SilverBullet editor decoration seam

This is the one local patch iugum carries in the vendored `silverbullet/` tree.
It is an extension point. External code decorates the rendered wiki page.
The client learns nothing about the feature that uses it.

The user-facing page is `silverbullet/docs/Editor Decorations.md`.
This page is the maintenance record. Read it after a `git subtree pull`
conflict.

## Why the patch exists

Block-level interactive features need to change how a page looks in the editor.
Examples are a colored outline around a group of list items, a header bar on a
block, and a selection state that spans several blocks.

Upstream SilverBullet has no general way to do that:

- The line-class list in `createEditorState` is hard coded.
- Widgets exist for fenced code blocks and for `${}` Lua directives only.
- A plug runs in a web worker. That worker holds no CodeMirror and no DOM.
  A plug cannot give the client a CodeMirror extension. An upstream developer's
  reason: the client and a plug cannot both bundle CodeMirror and Lezer,
  because that breaks `instanceof` checks.

One seam is cheaper than several patches. The cost of a fork scales with the
number of places the fork touches. So the seam is built to carry later features
without a second patch.

## What is fed, and by whom

The seam reads the `editorDecorations` config key. The value is plain data.
Plain data crosses the worker boundary, so two writers can drive the seam:

| Writer | How |
|---|---|
| Space Lua | `config.set("editorDecorations", { ... })` in `CONFIG` or any Space Lua block |
| A plug | the `config.set` syscall, then the `editor.rebuildEditorState` syscall |

The client re-reads the key on each editor state build. A build happens on page
load, on the `editor:reloadState` event, and on `editor.rebuildEditorState`.

A malformed entry is dropped. The other entries still apply. No JSON schema is
registered for the key, so a writer is never blocked by validation.

## The eight capabilities

All eight are delivered.

1. **Line classes.** `lines` puts CSS classes on the lines of any top-level
   block, selected by Lezer node name. Nesting depth is available.
2. **Range marks.** `marks` puts a class on a source range. `lineClasses` adds
   `-first`, `-mid` and `-last` per line, so a caller can draw one continuous
   outline across several blocks.
3. **Widgets.** `widgets` attaches a rendered element above or below a block, or
   `inline` at one offset. No fenced code block and no `${}` directive is
   needed.
4. **Events.** `events` turns on the `editor:decorationClick` and
   `editor:decorationSelect` app events. Interactive features share one DOM
   listener.
5. **Folds.** `folds` names a source range the editor's own folding can
   collapse, for a region that is not a syntax node. Collapsing is
   **declarative in both directions**: `folds[].collapsed` says what the caller
   wants and the seam makes the editor's fold set match it, and
   `foldedFroms` on the click event says what the editor actually has. A
   collapse control needs both. With only the write half, a control has to
   remember what it last did, and it is wrong from the first fold anyone makes
   from the gutter.
6. **Gestures.** `gestures` turns on a drag of one decorated range onto another
   (`editor:decorationDrag`) and a rubber-band sweep over decorated ranges
   (`editor:decorationLasso`).
7. **Hover classes.** `marks[].hoverClasses` adds `<class>-hover` to every line
   of a mark while the pointer is inside its range. CSS cannot do this: a range
   spanning several blocks is several sibling line elements with nothing
   wrapping them, and CSS has no previous-sibling combinator.
8. **Active line.** `activeLine` installs CodeMirror's own active-line
   highlighter, so `cm-activeLine` marks the cursor's line. A caller that hides
   something at rest needs one condition to reveal it, or a cursor can land in
   a line nobody can see.

Capabilities 5 to 8 were added for the inline atomdown card view
(`plugs/atomdown-inline/`). They are the reason the seam was built generic: the
feature needed drag-to-reorder and lasso selection in the page, and the seam
grew one section each instead of the fork growing a second patch.

## Exact config shape

```lua
config.set("editorDecorations", {
  lines = {
    { selector = "ListItem", class = "my-item", nesting = true },
  },
  marks = {
    { id = "group-1", from = 120, to = 245, class = "my-group",
      lineClasses = true },
  },
  widgets = {
    { id = "bar-1", at = 120, side = "before", class = "my-bar",
      html = "<span>3 items</span>" },
  },
  folds = {
    { from = 152, to = 245 },
  },
  activeLine = true,
  events = { click = true, selection = true },
  gestures = {
    drag = { handleClass = "my-grip" },
    lasso = { modifier = "alt" },
  },
})
```

| Path | Type | Meaning |
|---|---|---|
| `lines[].selector` | string | Lezer node name, e.g. `ListItem`, `ATXHeading2`, `Task` |
| `lines[].class` | string | class on every line of the block |
| `lines[].nesting` | boolean | also add `<class>-<depth>` |
| `marks[].from`, `.to` | integer | source offsets |
| `marks[].class` | string | class on the marked text, and the stem of the line classes |
| `marks[].id` | string | name reported on events, defaults to `class` |
| `marks[].lineClasses` | boolean | add `<class>-line` plus `-first`, `-mid`, `-last` |
| `marks[].hoverClasses` | boolean | add `<class>-hover` while the pointer is in the range |
| `widgets[].at` | integer | any source offset in the target block |
| `widgets[].html` | string | HTML for the element's content |
| `widgets[].side` | `before` or `after` | above (default) or below the line at `at` |
| `widgets[].inline` | boolean | render at exactly `at`, on the same line as the text |
| `widgets[].class` | string | class on the element, next to `sb-decoration-widget` |
| `widgets[].id` | string | name reported on a click in the widget |
| `activeLine` | boolean | put `cm-activeLine` on the cursor's line |
| `folds[].from`, `.to` | integer | source range the editor's folding can collapse |
| `folds[].collapsed` | boolean | fold it now. The seam reconciles the editor's fold set to this |
| `events.click` | boolean | dispatch `editor:decorationClick` |
| `events.selection` | boolean | dispatch `editor:decorationSelect` |
| `gestures.drag.handleClass` | string | a press on an element with this class starts a drag |
| `gestures.drag.modifier` | modifier | a press with this modifier held, inside a decorated range, starts a drag |
| `gestures.lasso.modifier` | modifier | arms the rubber band. Defaults to `alt` |

A modifier is `alt`, `shift`, `ctrl`, `meta` or `none`. A `drag` with neither a
handle nor a usable modifier is dropped, because it could never fire.

Event payloads:

| Event | Fields |
|---|---|
| `editor:decorationClick` | `page`, `pos`, `line`, `lineClasses`, `classes`, `marks`, `widget`, `foldedFroms`, `metaKey`, `ctrlKey`, `altKey` |
| `editor:decorationSelect` | `page`, `from`, `to`, `marks` |
| `editor:decorationDrag` | `page`, `from`, `to`, `marks`, `targetFrom`, `targetTo`, `targetMarks`, `targetLine`, `placement`, modifier flags |
| `editor:decorationLasso` | `page`, `from`, `to`, `fromLine`, `toLine`, `marks`, `ranges`, modifier flags |

`marks` lists are **outermost first**, so a caller that nests ranges reads
element 0 and gets the outer one.

`foldedFroms` is the `from` offset of every configured fold range that is
folded at the moment of the click. Offsets rather than indices, because the
caller's own list can be rebuilt between the click and the handler while a
`from` is a document position both sides compute the same way. A caret that
reads it decides fold-or-unfold from the editor rather than from memory, so it
can never go one press out of step.

Mark, widget and fold offsets are read once, at editor state build time. The
seam then maps them through your edits, so a mark stays on the text it was put
on.

## Two things a caller must know

**A gesture never writes the document.** The seam reports the drag or the sweep
and stops. That is deliberate: the document mutation belongs to the plug, so it
can go through `editor.replaceRange` as ONE transaction, which is ONE entry in
the editor's own undo history. Native Cmd-Z then reverts a whole reorder in one
step, and nothing needs a private undo stack.

**Do not rebuild the editor after your own edit.** `marks`, `widgets` and
`folds` refresh on their own: the seam holds them in one StateField that
compares the config value's identity on every transaction and rebuilds from the
new state when it changed. So write the new config FIRST and then apply the
edit, and the edit's own transaction carries the refresh.
`editor.rebuildEditorState` calls `setState`, which discards the undo history,
so calling it after an edit makes that edit un-undoable. Rebuild only when
`lines`, `events` or `gestures` change - when a feature is turned on or off -
and use `editor.dispatch({})` to nudge the editor when the config changed with
no edit behind it.

The seam ships no CSS. Style the classes from `space-style`.
Feature logic stays in the plug and in that CSS.

## Files the patch touches

| File | Change |
|---|---|
| `silverbullet/client/codemirror/decoration_seam.ts` | new. The whole seam. |
| `silverbullet/client/codemirror/decoration_seam.test.ts` | new. 22 vitest cases. |
| `silverbullet/docs/Editor Decorations.md` | new. The user-facing page. |
| `silverbullet/client/codemirror/editor_state.ts` | 2 hunks: 1 import line, 1 spread line with a comment. |
| `silverbullet/plug-api/types/client.ts` | 1 hunk: 4 event names appended to the `AppEvent` union. |

Three new files, two hunks in one upstream file, one hunk in another. A new file
cannot conflict on a subtree pull. Both hunks are appends.

The gesture, fold, hover-class and active-line work of 2026-09 added **no new
file and no new hunk**: it grew `decoration_seam.ts`, which is already ours, and added two more
event names inside the hunk that was already in `plug-api/types/client.ts`. That
is the whole point of the seam's shape.

## Re-derive the patch after a subtree pull

The three new files are additions. A pull does not touch them.
Re-apply the two edits by hand:

In `silverbullet/client/codemirror/editor_state.ts`, next to the other
`./line_wrapper.ts` import:

```ts
import { decorationSeam } from "./decoration_seam.ts";
```

In the same file, after the closing bracket of the `lineWrapper([...])` call in
`createEditorState`:

```ts
      // The editor decoration seam: line classes, range marks, block widgets
      // and click/selection events driven by the `editorDecorations` config
      // key. See client/codemirror/decoration_seam.ts.
      ...decorationSeam(client, pageName),
```

In `silverbullet/plug-api/types/client.ts`, at the end of the `AppEvent` union:

```ts
  // Editor decoration seam, see client/codemirror/decoration_seam.ts
  | "editor:decorationClick"
  | "editor:decorationSelect"
  | "editor:decorationDrag"
  | "editor:decorationLasso";
```

Then verify:

```sh
cd silverbullet
npm test -- client/codemirror/decoration_seam.test.ts
npm run check
```

`npm run check` is `tsc --noEmit` and must stay silent. `biome check` is dirty
in the upstream tree at the pin, so it is not a gate. The seam's own files are
biome clean.

## Two traps worth remembering

A ViewPlugin's `eventHandlers` fired for `mousedown` but never for
`mousemove`. The `EditorView.domEventHandlers` facet form, which the click
handler already used, works for both. Use the facet form for a new pointer
handler.



CodeMirror ignores every DOM event that starts inside a widget
(`WidgetType.ignoreEvent` defaults to `true`). With that default, a click on a
seam widget and a drag from a handle rendered as one never reach the seam's own
handlers at all, and nothing in the console says so. `DecorationWidget` now
overrides `ignoreEvent` to let `mousedown` and `click` through and to keep
ignoring everything else. If a widget control ever goes dead after an upstream
merge, check that override first.

## Upstream

The seam holds no iugum and no atomdown concept, so it stays upstreamable.
Steve decided not to open a pull request or an upstream issue now.
