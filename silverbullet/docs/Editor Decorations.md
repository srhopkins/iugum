---
description: One config-driven seam for decorating blocks in the editor with classes, marks, widgets and events.
status: Complete
tags: glossary
references:
- client/codemirror/decoration_seam.ts
- client/codemirror/editor_state.ts
---

> **Local addition.** This page and `client/codemirror/decoration_seam.ts`
> are the only iugum changes to the vendored SilverBullet tree, plus two
> event names in `plug-api/types/client.ts` and one call in
> `client/codemirror/editor_state.ts`. Carry them across a
> `git subtree pull`. The iugum-side copy of this document is
> `docs/silverbullet-decoration-seam.md`.

Editor decorations let code outside the client change how a page looks in the
editor. You set the `editorDecorations` config key to plain data. The client
reads that key when it builds the editor state, and turns the data into
CodeMirror decorations.

# Why the seam is config
A [[Plug]] runs in a web worker. That worker has no CodeMirror and no DOM, so a
plug cannot give the client a CodeMirror extension. Config is plain data, and
plain data crosses the worker boundary. Both writers can therefore drive the
seam:

* [[Space Lua]]: `config.set("editorDecorations", { ... })` in your
  [[CONFIG]] page or any Space Lua block.
* A plug: the `config.set` syscall, then the `editor.rebuildEditorState`
  syscall to make the change visible.

The client re-reads the key on every editor state rebuild. A rebuild happens on
page load, on `editor:reloadState`, and on the `editor.rebuildEditorState`
syscall.

# Config shape
```lua
config.set("editorDecorations", {
  -- 1. CSS classes on the lines of a block, by Lezer node name
  lines = {
    { selector = "ListItem", class = "my-item", nesting = true },
    { selector = "ATXHeading2", class = "my-section" },
  },
  -- 2. A class over a source range, for selection state or a group
  marks = {
    { id = "group-1", from = 120, to = 245, class = "my-group",
      lineClasses = true },
  },
  -- 3. A rendered element attached to a block
  widgets = {
    { id = "bar-1", at = 120, side = "before", class = "my-bar",
      html = "<span>3 items</span>" },
  },
  -- 4. App events about the decorated page
  events = { click = true, selection = true },
})
```

Every key is optional. A malformed entry is dropped and the rest still apply,
because this data comes from user code and one typo must not stop the editor.
No JSON schema is registered for the key on purpose, so a writer is never
blocked by validation.

## lines
| Field | Meaning |
|---|---|
| `selector` | Lezer node name, e.g. `ListItem`, `ATXHeading2`, `FencedCode`, `Task` |
| `class` | class added to every line of the block |
| `nesting` | also add `<class>-<depth>`, e.g. `my-item-2` for a nested list item |

This is the same rule shape the client uses for its own `sb-line-*` classes.

## marks
| Field | Meaning |
|---|---|
| `from`, `to` | source offsets |
| `class` | class on the marked text, and the stem of the line classes |
| `id` | name reported back on events, defaults to `class` |
| `lineClasses` | add `<class>-line` to every covered line, plus `<class>-first`, `<class>-mid` and `<class>-last` by position |

Use `lineClasses` to draw one continuous outline around a group that spans
several blocks. A single-line range gets both `-first` and `-last`.

Offsets are read once, when the editor state is built. After that they are
mapped through your edits, so a mark stays on the text it was put on. A mark
past the end of the document is clamped.

## widgets
| Field | Meaning |
|---|---|
| `at` | any source offset in the target block |
| `html` | HTML for the element's content |
| `side` | `before` (default) puts the element above the line at `at`, `after` puts it below |
| `class` | class on the element, in addition to `sb-decoration-widget` |
| `id` | name reported back on a click in the widget |

The element is a block-level `<div>`. It needs no fenced code block and no
`${}` [[Space Lua/Lua Directive]].

`html` is inserted as written. Only trust the writer of your own config.

## events
Set `events.click` or `events.selection` to `true`, then handle these
[[Event]]s:

| Event | Payload |
|---|---|
| `editor:decorationClick` | `page`, `pos`, `line`, `lineClasses`, `marks`, `widget`, `metaKey`, `ctrlKey`, `altKey` |
| `editor:decorationSelect` | `page`, `from`, `to`, `marks` |

`lineClasses` holds the CSS classes on the clicked line, so a handler learns
which block kind was clicked without keeping its own map. `marks` holds the
`id` of every configured mark the position or the selection touches. `widget`
is present only when the click landed in a seam widget.

The click handler never stops the event. The seam observes; it does not take
the click away from the editor.

# Styling
The seam ships no CSS. Style your classes from [[Space Style]]:

```space-style
.my-group-line { border-left: 3px solid var(--editor-directive-color); }
.my-group-first { border-top-left-radius: 4px; }
.my-group-last { border-bottom-left-radius: 4px; }
```

# Cost when unused
`decorationSeam()` returns no extensions when the key is absent. An
unconfigured space pays one config read per editor state build.
