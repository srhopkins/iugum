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

## Changing the value while the page stays open
`marks`, `widgets` and `folds` also refresh **without** a rebuild. Write the new
value, then let any transaction reach the editor; the seam notices the value is
a different object and rebuilds those three from the new state.

Prefer that route after your own edit, and write the config **before** the edit:

```lua
config.set("editorDecorations", decorationsFor(newText))
editor.replaceRange(from, to, insert)
```

`editor.rebuildEditorState` calls `setState`, which **discards the undo
history**. So a caller that rewrites the document and then rebuilds makes its
own edit un-undoable. Writing the config first and letting the edit's own
transaction carry the refresh keeps native undo working: one edit, one
transaction, one undo step.

`lines`, `events` and `gestures` are installed as extensions, so those do need
a rebuild. Rebuild when you turn a feature on or off, not after an edit. Use
`editor.dispatch({})` to nudge the editor when the config changed and no edit
followed.

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
  -- 4. Source ranges the editor's own folding can collapse
  folds = {
    { from = 152, to = 245 },
  },
  -- 5. App events about the decorated page
  events = { click = true, selection = true },
  -- 6. CodeMirror's own class on the cursor's line
  activeLine = true,
  -- 7. Pointer gestures over the decorated ranges
  gestures = {
    drag = { handleClass = "my-grip" },
    lasso = { modifier = "alt" },
  },
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
| `hoverClasses` | add `<class>-hover` to every covered line while the pointer is inside the range |

Use `lineClasses` to draw one continuous outline around a group that spans
several blocks. A single-line range gets both `-first` and `-last`.

Offsets are read once, when the editor state is built. After that they are
mapped through your edits, so a mark stays on the text it was put on. A mark
past the end of the document is clamped.

`hoverClasses` exists because CSS cannot express it. A range that spans several
blocks is several sibling line elements with nothing wrapping them, and CSS has
no previous-sibling combinator, so a line cannot react to a pointer on the line
below it. Only the client knows which decorated range the pointer is in, so it
says so as a class. Write the quiet state on `<class>-line` and the active one
on `<class>-hover`. A block widget attached above the range can reach the same
state with `:has(+ .your-class-hover)`, because its next sibling is the range's
own first line.

## widgets
| Field | Meaning |
|---|---|
| `at` | any source offset in the target block |
| `html` | HTML for the element's content |
| `side` | `before` (default) puts the element above the line at `at`, `after` puts it below |
| `inline` | render at exactly `at`, on the same line as the text, instead of as its own block |
| `class` | class on the element, in addition to `sb-decoration-widget` |
| `id` | name reported back on a click in the widget |

The element is a `<div>`. It needs no fenced code block and no
`${}` [[Space Lua/Lua Directive]].

An `inline` widget sits between two characters of a line, so it spends no line
of its own. Give it `display: inline-block` in your style. This is how you
attach a per-block affordance - a drag handle, a status dot - to a document with
hundreds of blocks.

A press and a click inside a seam widget reach the editor's handlers, so
`events` and `gestures` work on your controls. Every other event type stays
ignored, the way CodeMirror ignores events in a widget by default, so typing
and pasting in one are still not editor input.

`html` is inserted as written. Only trust the writer of your own config.

## folds
| Field | Meaning |
|---|---|
| `from` | where the hidden part starts. Normally the end of the region's first line |
| `to` | where the hidden part ends |

CodeMirror folds a syntax node it knows how to fold. A `folds` entry names a
region that is **not** a syntax node - a run of blocks you grouped yourself, for
example - and the editor's own folding then handles it: the fold and unfold
commands, the `editor.fold` and `editor.unfold` syscalls, and the fold gutter.

The seam adds no collapse state of its own. Nothing is folded until something
folds it, and the editor owns the fold from then on. Move the cursor to the
region's **first line** before calling `editor.fold`: that is the line the entry
makes foldable.

## events
Set `events.click` or `events.selection` to `true`, then handle these
[[Event]]s:

| Event | Payload |
|---|---|
| `editor:decorationClick` | `page`, `pos`, `line`, `lineClasses`, `classes`, `marks`, `widget`, `metaKey`, `ctrlKey`, `altKey` |
| `editor:decorationSelect` | `page`, `from`, `to`, `marks` |

`lineClasses` holds the CSS classes on the clicked line, so a handler learns
which block kind was clicked without keeping its own map. `classes` holds the
classes of the element that was clicked, nearest first: that is what lets one
widget carry several controls, because the handler reads the class you put on
the control instead of needing a widget each. `marks` holds the `id` of every
configured mark the position or the selection touches, **outermost first**, so a
handler that nests ranges reads element 0 and gets the outer one. `widget` is
present only when the click landed in a seam widget.

The click handler never stops the event. The seam observes; it does not take
the click away from the editor.

# gestures
Two pointer gestures the browser has no primitive for. Both are off unless
configured, and **neither one changes the document**: the seam does the pointer
tracking and the visual feedback, then reports what happened. Whatever handles
the event decides what the gesture means and writes the document itself, which
is what keeps the edit outside the client and lets it be one undo step.

| Field | Meaning |
|---|---|
| `drag.handleClass` | a press on an element with this class starts a drag |
| `drag.modifier` | a press with this modifier held, inside a decorated range, starts a drag. `alt`, `shift`, `ctrl`, `meta` or `none` |
| `lasso.modifier` | the modifier that arms the rubber band. Defaults to `alt` |

A `drag` with neither a handle nor a usable modifier is dropped, since it could
never fire. Prefer a handle: it intercepts no ordinary text drag and asks the
reader to discover no modifier. Render it as an `inline` widget.

| Event | Payload |
|---|---|
| `editor:decorationDrag` | `page`, `from`, `to`, `marks`, `targetFrom`, `targetTo`, `targetMarks`, `targetLine`, `placement`, and the modifier flags |
| `editor:decorationLasso` | `page`, `from`, `to`, `fromLine`, `toLine`, `marks`, `ranges`, and the modifier flags |

`from`/`to` is the range that was picked up, and `marks` names it - the covering
marks, outermost first. `targetFrom`/`targetTo` is the range it was released on,
and `placement` is `before` or `after` by which side of that range's vertical
midpoint the pointer was on. A release outside every mark reports the line's own
range and an empty `targetMarks`.

The band selects whole lines, because the ranges a caller decorates are blocks.
`ranges` holds every decorated range the swept lines touch, as
`{ from, to, name }`, outermost first.

While a drag runs, the seam puts `sb-decoration-dragging` on the lines being
carried and `sb-decoration-drop-before` or `sb-decoration-drop-after` on the
line the drop would land against. The band is a `sb-decoration-lasso` element.
All three are yours to style, and all three disappear when the gesture ends.

# activeLine
`activeLine = true` installs CodeMirror's own active-line highlighter, so the
line holding the cursor carries `cm-activeLine`.

It is here for one reason: a caller that hides something at rest needs one
condition to reveal it again, or the reader can put a cursor in a line nobody
can see and an edit lands out of sight. The cursor's own line is that
condition. Off by default, because it also paints CodeMirror's active-line
background - override that in your own style if you do not want it.

# Styling
The seam ships no CSS. Style your classes from [[Space Style]]:

```space-style
.my-group-line { border-left: 3px solid var(--editor-directive-color); }
.my-group-first { border-top-left-radius: 4px; }
.my-group-last { border-bottom-left-radius: 4px; }
.sb-decoration-widget.my-grip { display: inline-block; cursor: grab; }
.sb-decoration-drop-before { box-shadow: inset 0 2px 0 var(--ui-accent-color); }
.sb-decoration-lasso { border: 1px solid var(--ui-accent-color); }
```

One line can carry the line classes of two nested marks, and one element can
only have one `border-left`. Draw the outer outline with a real `border` and the
inner one with an inset `box-shadow`, and both are visible at once.

# Cost when unused
`decorationSeam()` returns no extensions when the key is absent. An
unconfigured space pays one config read per editor state build.
