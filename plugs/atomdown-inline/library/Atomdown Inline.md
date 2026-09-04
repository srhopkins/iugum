---
description: The header-bar icon and the styling for the inline Atomdown card view
tags: meta
---

The `atomdown-inline` plug draws the Atomdown card view on the page itself. This
page supplies the two things a plug cannot: the header-bar button, and the CSS.

Copy this page into your space as `Library/Atomdown/Inline`, and copy
`atomdown-inline.plug.js` into the space's `_plug/` directory. Reload the
browser once so SilverBullet picks up both.

# The header-bar icon

Action buttons are configuration, so no client change is needed. `grid` is a
[feather icon](https://feathericons.com) name, and the button names the plug's
command, so the command's keyboard shortcut shows in the tooltip.

```space-lua
actionButton.define {
  icon = "grid",
  description = "Atomdown card view on this page",
  command = "Atomdown: Toggle Inline View",
}
```

The view is off until you press it, and it is remembered per page: pressing it
on one page never turns it on for another, and pressing it again turns it off
and keeps it off across a reload.

# The styling

Every value is a named custom property with a default, and the names are the
same ones the `atomdown-board` panel uses, so the two views cannot drift apart.
Override any of them on `html` in your own `space-style` page.

Nothing here hardcodes a colour that has a SilverBullet theme token, so the
view follows the light and the dark theme on its own.

Two mechanisms draw the two outlines, on purpose. A line inside a group carries
both the group's line classes and its member card's, and one element can only
have one `border-left`. So the group outline is a real border and the card
outline is an inset `box-shadow`. Both are then visible at once.

```space-style
html {
  --board-accent-color: var(--ui-accent-color, #4a7dc7);
  --board-card-radius: 6px;
  --board-card-border-width: 1px;
  --board-group-border-width: 2px;
  --board-group-quiet-border: 40%;
  --board-group-quiet-header: 16%;
  --board-grip-size: 14px;
  --board-id-size: 11px;
  --board-header-quiet-color: var(--subtle-color, #888);
  --board-header-active-color: var(--root-color, #222);
  --atomdown-card-border-color: var(--subtle-background-color, #d8d8d8);
  --atomdown-card-surface: transparent;
}

/* ------------------------------------------------------------------ */
/* A card: one continuous outline down the lines of one atom's block. */
/* ------------------------------------------------------------------ */

.cm-line.atomdown-card-line {
  background: var(--atomdown-card-surface);
  box-shadow:
    inset var(--board-card-border-width) 0 0 var(--atomdown-card-border-color),
    inset calc(-1 * var(--board-card-border-width)) 0 0
      var(--atomdown-card-border-color);
}

.cm-line.atomdown-card-line.atomdown-card-first {
  border-top-left-radius: var(--board-card-radius);
  border-top-right-radius: var(--board-card-radius);
  box-shadow:
    inset var(--board-card-border-width) 0 0 var(--atomdown-card-border-color),
    inset calc(-1 * var(--board-card-border-width)) 0 0
      var(--atomdown-card-border-color),
    inset 0 var(--board-card-border-width) 0 var(--atomdown-card-border-color);
}

.cm-line.atomdown-card-line.atomdown-card-last {
  border-bottom-left-radius: var(--board-card-radius);
  border-bottom-right-radius: var(--board-card-radius);
  box-shadow:
    inset var(--board-card-border-width) 0 0 var(--atomdown-card-border-color),
    inset calc(-1 * var(--board-card-border-width)) 0 0
      var(--atomdown-card-border-color),
    inset 0 calc(-1 * var(--board-card-border-width)) 0
      var(--atomdown-card-border-color);
}

.cm-line.atomdown-card-line.atomdown-card-first.atomdown-card-last {
  box-shadow:
    inset var(--board-card-border-width) 0 0 var(--atomdown-card-border-color),
    inset calc(-1 * var(--board-card-border-width)) 0 0
      var(--atomdown-card-border-color),
    inset 0 var(--board-card-border-width) 0 var(--atomdown-card-border-color),
    inset 0 calc(-1 * var(--board-card-border-width)) 0
      var(--atomdown-card-border-color);
}

/* --------------------------------------------------------------- */
/* A group: the 2px accent outline, quiet until the group is used. */
/* --------------------------------------------------------------- */

.cm-line.atomdown-group-line {
  border-left: var(--board-group-border-width) solid
    var(--board-accent-color);
  border-right: var(--board-group-border-width) solid
    var(--board-accent-color);
}

.cm-line.atomdown-group-line.atomdown-group-first {
  border-top: var(--board-group-border-width) solid
    var(--board-accent-color);
  border-top-left-radius: var(--board-card-radius);
  border-top-right-radius: var(--board-card-radius);
}

.cm-line.atomdown-group-line.atomdown-group-last {
  border-bottom: var(--board-group-border-width) solid
    var(--board-accent-color);
  border-bottom-left-radius: var(--board-card-radius);
  border-bottom-right-radius: var(--board-card-radius);
}

/* The quiet resting state. Written after the full-strength rules, so a
   browser with no color-mix() drops this block and the group simply never
   recedes, rather than being left at an unreadable half state. */
.cm-content:not(:hover) .cm-line.atomdown-group-line {
  border-color: color-mix(
    in srgb,
    var(--board-accent-color) var(--board-group-quiet-border),
    transparent
  );
}

/* ----------------------------------------- */
/* The group header bar, above its marker.   */
/* ----------------------------------------- */

.sb-decoration-widget.atomdown-group-header {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 5px 8px;
  margin-top: 8px;
  border-top-left-radius: var(--board-card-radius);
  border-top-right-radius: var(--board-card-radius);
  font-size: 0.86em;
  line-height: 1.3;
  color: var(--board-header-active-color);
  background: color-mix(
    in srgb,
    var(--board-accent-color) var(--board-group-quiet-header),
    transparent
  );
}

.cm-content:hover .sb-decoration-widget.atomdown-group-header {
  background: color-mix(
    in srgb,
    var(--board-accent-color) 28%,
    transparent
  );
}

.atomdown-group-name {
  font-weight: 600;
}

.atomdown-group-count {
  color: var(--board-header-quiet-color);
  font-size: var(--board-id-size);
}

.atomdown-group-collapse,
.atomdown-group-menu {
  cursor: pointer;
  color: var(--board-header-quiet-color);
  user-select: none;
}

.atomdown-group-collapse:hover,
.atomdown-group-menu:hover {
  color: var(--board-header-active-color);
}

.atomdown-group-menu {
  margin-left: auto;
}

/* ------------------------------------------------------- */
/* The drag handle. Hidden until its own block is hovered.  */
/* ------------------------------------------------------- */

.sb-decoration-widget.atomdown-grip {
  display: inline-block;
  width: 0;
  height: 0;
  position: relative;
  vertical-align: baseline;
}

.atomdown-grip-dots {
  position: absolute;
  left: -1.2em;
  top: -0.1em;
  font-size: var(--board-grip-size);
  line-height: 1;
  color: var(--board-header-quiet-color);
  cursor: grab;
  opacity: 0;
  transition: opacity 80ms linear;
  user-select: none;
}

.cm-line:hover .atomdown-grip-dots,
.atomdown-group-header .atomdown-grip:hover {
  opacity: 1;
}

.atomdown-group-header .atomdown-grip {
  cursor: grab;
  color: var(--board-header-quiet-color);
}

/* ------------------------------------------------------------------ */
/* The directive comments. Dimmed and shrunk, never hidden: the page   */
/* is the editor, so text you can still put a cursor in must stay      */
/* visible or an edit lands somewhere you cannot see.                  */
/* ------------------------------------------------------------------ */

.cm-line.atomdown-directive {
  opacity: 0.4;
  font-size: 0.78em;
}

.cm-line.atomdown-directive:hover,
.cm-line.atomdown-directive.cm-activeLine {
  opacity: 0.85;
}

/* ------------------------------- */
/* The lasso and the drag feedback. */
/* ------------------------------- */

.cm-line.atomdown-selected-line {
  background: color-mix(in srgb, var(--board-accent-color) 12%, transparent);
}

.sb-decoration-lasso {
  border: 1px solid var(--board-accent-color);
  border-radius: 2px;
  background: color-mix(in srgb, var(--board-accent-color) 10%, transparent);
  z-index: 20;
}

.cm-line.sb-decoration-dragging {
  opacity: 0.45;
}

.cm-line.sb-decoration-drop-before {
  box-shadow: inset 0 2px 0 var(--board-accent-color) !important;
}

.cm-line.sb-decoration-drop-after {
  box-shadow: inset 0 -2px 0 var(--board-accent-color) !important;
}
```
