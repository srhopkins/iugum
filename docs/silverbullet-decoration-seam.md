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

## The four capabilities

All four are delivered.

1. **Line classes.** `lines` puts CSS classes on the lines of any top-level
   block, selected by Lezer node name. Nesting depth is available.
2. **Range marks.** `marks` puts a class on a source range. `lineClasses` adds
   `-first`, `-mid` and `-last` per line, so a caller can draw one continuous
   outline across several blocks.
3. **Block widgets.** `widgets` attaches a rendered element above or below a
   block. No fenced code block and no `${}` directive is needed.
4. **Events.** `events` turns on the `editor:decorationClick` and
   `editor:decorationSelect` app events. Interactive features share one DOM
   listener.

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
  events = { click = true, selection = true },
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
| `widgets[].at` | integer | any source offset in the target block |
| `widgets[].html` | string | HTML for the element's content |
| `widgets[].side` | `before` or `after` | above (default) or below the line at `at` |
| `widgets[].class` | string | class on the element, next to `sb-decoration-widget` |
| `widgets[].id` | string | name reported on a click in the widget |
| `events.click` | boolean | dispatch `editor:decorationClick` |
| `events.selection` | boolean | dispatch `editor:decorationSelect` |

Event payloads:

| Event | Fields |
|---|---|
| `editor:decorationClick` | `page`, `pos`, `line`, `lineClasses`, `marks`, `widget`, `metaKey`, `ctrlKey`, `altKey` |
| `editor:decorationSelect` | `page`, `from`, `to`, `marks` |

Mark and widget offsets are read once, at editor state build time. The seam then
maps them through your edits, so a mark stays on the text it was put on.

The seam ships no CSS. Style the classes from `space-style`.
Feature logic stays in the plug and in that CSS.

## Files the patch touches

| File | Change |
|---|---|
| `silverbullet/client/codemirror/decoration_seam.ts` | new. The whole seam. |
| `silverbullet/client/codemirror/decoration_seam.test.ts` | new. 10 vitest cases. |
| `silverbullet/docs/Editor Decorations.md` | new. The user-facing page. |
| `silverbullet/client/codemirror/editor_state.ts` | 2 hunks: 1 import line, 1 spread line with a comment. |
| `silverbullet/plug-api/types/client.ts` | 1 hunk: 2 event names appended to the `AppEvent` union. |

Only the last two files can conflict on a subtree pull. Both are appends.

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
  | "editor:decorationSelect";
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

## Upstream

The seam holds no iugum and no atomdown concept, so it stays upstreamable.
Steve decided not to open a pull request or an upstream issue now.
