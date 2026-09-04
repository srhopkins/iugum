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

**The target is the `atomdown-board` panel.** Every value below is one of that
panel's own named custom properties, with the same default, so the two views
cannot drift: `--board-card-radius`, `--board-card-padding`,
`--board-accent-color`, `--board-group-padding` and the rest. Override any of
them on `html` in your own `space-style` page and both views move together.

Nothing here hardcodes a colour that has a SilverBullet theme token, so the view
follows the light and the dark theme on its own.

## How a box is drawn out of lines

A CodeMirror line is a block element, so borders, padding, radius and
background all work on it. The seam's `lineClasses` gives `-first`, `-mid` and
`-last` per decorated range, and that is what closes a box:

* `-first` takes the top edge and the top corners,
* `-mid` takes the sides only,
* `-last` takes the bottom edge and the bottom corners,
* a one-line block is **both** `-first` and `-last`, so it draws all four edges
  on its own line. That case has its own rule below.

A soft-wrapped paragraph is ONE line element with several visual rows, and a
border on a block element encloses the whole box, so a wrapped block is
enclosed by construction rather than by a special case.

## Why the card is a pseudo-element and the group is a border

A line inside a group carries **both** ranges' line classes, and one element
can only have one `border-left`. The group is the outer box, so the group takes
the real `border`; the card is drawn by an absolutely positioned `::before`
inset by `--board-group-padding`, which is what makes a member card float
inside the group with a gap on all four sides. A top-level card sets that inset
to zero and is otherwise the identical box.

```space-style
html {
  --board-accent-color: var(--ui-accent-color, #4a7dc7);
  --board-card-radius: 6px;
  --board-card-border-width: 1px;
  --board-card-padding: 8px;
  --board-card-header-padding: 4px 8px;
  --board-group-border-width: 2px;
  --board-group-padding: 8px;
  --board-group-header-padding: 5px 8px;
  --board-card-gap: 14px;
  --board-group-quiet-header: 16%;
  --board-grip-size: 14px;
  --board-id-size: 11px;
  --board-header-quiet-color: var(--subtle-color, #888);
  --board-card-surface: var(--ui-surface-section-background-color, #f7f7f7);
  --board-card-border-color: var(--ui-surface-border-color, #ddd);
  --board-group-field: var(--ui-surface-background-color, #fff);
}

/* ------------------------------------------------------------------ */
/* THE CARD BOX.                                                       */
/* One closed rounded box per atom, drawn by a ::before inset by       */
/* --ad-inset. --ad-inset is 0 for a top-level card and the group's    */
/* padding for a member card, so a member floats inside the group.     */
/* ------------------------------------------------------------------ */

.cm-line.atomdown-card-line {
  --ad-inset: 0px;
  position: relative;
  padding-left: calc(var(--ad-inset) + var(--board-card-padding));
  padding-right: calc(var(--ad-inset) + var(--board-card-padding));
}

/* A card inside a group is inset by the group's interior padding. Two
   classes, so this beats the single-class rule above. */
.cm-line.atomdown-card-line.atomdown-group-line {
  --ad-inset: var(--board-group-padding);
}

.cm-line.atomdown-card-line::before {
  content: "";
  position: absolute;
  z-index: -1;
  left: var(--ad-inset);
  right: var(--ad-inset);
  top: 0;
  bottom: 0;
  background: var(--board-card-surface);
  border-left: var(--board-card-border-width) solid
    var(--board-card-border-color);
  border-right: var(--board-card-border-width) solid
    var(--board-card-border-color);
  pointer-events: none;
}

/* The card's top edge and top corners live on the header widget, which sits
   directly above this line - so -first adds padding only. */
.cm-line.atomdown-card-line.atomdown-card-first {
  padding-top: var(--board-card-padding);
}

.cm-line.atomdown-card-line.atomdown-card-last {
  padding-bottom: var(--board-card-padding);
}

.cm-line.atomdown-card-line.atomdown-card-last::before {
  border-bottom: var(--board-card-border-width) solid
    var(--board-card-border-color);
  border-bottom-left-radius: var(--board-card-radius);
  border-bottom-right-radius: var(--board-card-radius);
}

/* THE ONE-LINE BLOCK. Both -first and -last, so it must close the box by
   itself. It still has no top edge, because its header widget carries it. */
.cm-line.atomdown-card-line.atomdown-card-first.atomdown-card-last::before {
  border-bottom: var(--board-card-border-width) solid
    var(--board-card-border-color);
  border-bottom-left-radius: var(--board-card-radius);
  border-bottom-right-radius: var(--board-card-radius);
}

/* ------------------------------------------------------------------ */
/* THE CARD HEADER ROW: a shaded strip at the top of the box, with the */
/* name in body text and the id in small grey monospace. It is also    */
/* the box's top edge and top corners.                                 */
/* ------------------------------------------------------------------ */

.sb-decoration-widget.atomdown-card-header {
  --ad-inset: 0px;
  padding: 0;
  margin: 0;
}

.sb-decoration-widget.atomdown-card-header.atomdown-nested {
  --ad-inset: var(--board-group-padding);
  border-left: var(--board-group-border-width) solid
    var(--board-accent-color);
  border-right: var(--board-group-border-width) solid
    var(--board-accent-color);
}

.atomdown-card-head {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-left: var(--ad-inset);
  margin-right: var(--ad-inset);
  padding: var(--board-card-header-padding);
  background: var(--board-card-surface);
  border: var(--board-card-border-width) solid var(--board-card-border-color);
  border-bottom: none;
  border-top-left-radius: var(--board-card-radius);
  border-top-right-radius: var(--board-card-radius);
  line-height: 1.3;
}

/* A hairline under the strip, separating it from the body. Drawn as a shadow
   so it costs no height and cannot break the box's own border run. */
.atomdown-card-head {
  box-shadow: inset 0 calc(-1 * var(--board-card-border-width)) 0
    var(--board-card-border-color);
}

.atomdown-card-slug {
  font-size: 13px;
  font-weight: 600;
}

.atomdown-card-id {
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  font-size: var(--board-id-size);
  color: var(--board-header-quiet-color);
}

.atomdown-card-badge {
  font-size: 10px;
  padding: 1px 6px;
  border-radius: 999px;
  background: var(--ui-surface-hover-background-color, #eee);
  color: var(--board-header-quiet-color);
}

/* ------------------------------------------------------------------ */
/* THE GROUP BOX: one closed rounded 2px accent box around its cards.  */
/* The real border, because the group is the outer of the two boxes.   */
/* ------------------------------------------------------------------ */

/* Deliberately NO background here. The card box is an absolutely positioned
   ::before at z-index -1, and a negative-z child paints BELOW every other
   descendant's background - so a background on this line would hide the card
   surface of every member card. The group's field is therefore the page's own
   background, which is what --ui-surface-background-color resolves to anyway. */
.cm-line.atomdown-group-line {
  border-left: var(--board-group-border-width) solid
    var(--board-accent-color);
  border-right: var(--board-group-border-width) solid
    var(--board-accent-color);
}

/* No border-top: the header bar above the opening marker line is the box's
   top edge. The opening marker is a hidden directive, so its collapsed height
   becomes the group's interior top padding. */
.cm-line.atomdown-group-line.atomdown-group-first {
  padding-top: var(--board-group-padding);
}

.cm-line.atomdown-group-line.atomdown-group-last {
  padding-bottom: var(--board-group-padding);
  border-bottom: var(--board-group-border-width) solid
    var(--board-accent-color);
  border-bottom-left-radius: var(--board-card-radius);
  border-bottom-right-radius: var(--board-card-radius);
}

/* ----------------------------------------------------- */
/* THE GROUP HEADER BAR. Solid accent, contrast text, and */
/* the group box's top edge, exactly as the panel draws it. */
/* ----------------------------------------------------- */

.sb-decoration-widget.atomdown-group-header {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-wrap: wrap;
  padding: var(--board-group-header-padding);
  margin-top: var(--board-card-gap);
  background: color-mix(
    in srgb,
    var(--board-accent-color) var(--board-group-quiet-header),
    transparent
  );
  color: var(--root-color, #222);
  border: var(--board-group-border-width) solid var(--board-accent-color);
  border-bottom: none;
  border-top-left-radius: var(--board-card-radius);
  border-top-right-radius: var(--board-card-radius);
  font-size: 13px;
  line-height: 1.3;
  user-select: none;
}

/* Full strength when the pointer is on the bar itself, per group, the same
   way the panel's group bar comes forward on hover. Written after the resting
   rule, so a browser with no color-mix() drops the resting one and the bar is
   simply always at full strength rather than left unreadable. */
.sb-decoration-widget.atomdown-group-header:hover {
  background: var(--board-accent-color);
  color: var(--ui-accent-contrast-color, #fff);
}

.atomdown-group-kind {
  font-size: 10px;
  text-transform: uppercase;
  letter-spacing: 0.06em;
  opacity: 0.75;
}

.atomdown-group-name {
  font-size: 13px;
  font-weight: 600;
}

.atomdown-group-id {
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  font-size: var(--board-id-size);
  opacity: 0.8;
}

.atomdown-group-count {
  font-size: 11px;
  opacity: 0.8;
}

.atomdown-group-actions {
  display: flex;
  gap: 6px;
  margin-left: auto;
}

.atomdown-group-btn {
  cursor: pointer;
  font-size: 11px;
  line-height: 1.2;
  padding: 2px 8px;
  border-radius: 4px;
  border: 1px solid currentColor;
  user-select: none;
}

.atomdown-group-collapse,
.atomdown-group-menu {
  cursor: pointer;
  font-size: 12px;
  line-height: 1.2;
  padding: 1px 5px;
  border-radius: 4px;
  user-select: none;
}

/* Hover inverts the two accent tokens rather than mixing in a new value. */
.atomdown-group-btn:hover,
.atomdown-group-collapse:hover,
.atomdown-group-menu:hover {
  background: var(--ui-accent-contrast-color, #fff);
  color: var(--board-accent-color);
}

/* --------------------------------------------------------- */
/* THE DRAG GRIP. Hidden until its own row is hovered, with   */
/* its box still laid out, so nothing reflows. Same as the    */
/* panel's rule.                                              */
/* --------------------------------------------------------- */

.atomdown-grip {
  opacity: 0;
  font-size: var(--board-grip-size);
  line-height: 1;
  letter-spacing: -0.15em;
  cursor: grab;
  user-select: none;
}

.atomdown-card-head:hover .atomdown-grip,
.atomdown-group-header:hover .atomdown-grip {
  opacity: 0.5;
}

/* ------------------------------------------------------------------ */
/* THE DIRECTIVE COMMENTS.                                            */
/*                                                                    */
/* HIDDEN at rest. On a real page every atom carries a 64-character   */
/* sha256 digest that wraps over three or four rows, and 93 of those   */
/* are the single biggest reason this stopped reading as cards.        */
/*                                                                    */
/* Collapsed, not `display: none`: the line element stays in the       */
/* layout so CodeMirror's own coordinate and cursor maths are          */
/* untouched, and the text is revealed in full the moment the cursor   */
/* is ON that line. So an edit can never land somewhere invisible -    */
/* which was the reason for dimming instead of hiding in the first     */
/* place. That property is kept; only the resting state changed.       */
/* ------------------------------------------------------------------ */

.cm-line.atomdown-directive {
  font-size: 1px;
  line-height: 3px;
  color: transparent;
  overflow: hidden;
  padding-top: 0;
  padding-bottom: 0;
}

.cm-line.atomdown-directive.cm-activeLine,
.cm-line.atomdown-directive:hover {
  font-size: 0.72em;
  line-height: 1.35;
  color: var(--board-header-quiet-color);
  overflow: visible;
}

/* CodeMirror's own active-line background would fight the card surface. */
.cm-line.cm-activeLine {
  background-color: transparent;
}

/* ------------------------------- */
/* Selection, lasso, drag feedback. */
/* ------------------------------- */

.cm-line.atomdown-selected-line::before {
  background: var(--ui-surface-hover-background-color, #eaeaea);
  border-color: var(--board-accent-color);
}

.cm-line.atomdown-selected-line {
  box-shadow: inset 0 0 0 1px
    color-mix(in srgb, var(--board-accent-color) 45%, transparent);
}

.sb-decoration-lasso {
  border: 1px solid var(--board-accent-color);
  border-radius: 2px;
  background: var(--board-accent-color);
  opacity: 0.18;
  z-index: 40;
}

.cm-line.sb-decoration-dragging {
  opacity: 0.4;
}

.cm-line.sb-decoration-drop-before {
  box-shadow: inset 0 3px 0 0 var(--board-accent-color);
}

.cm-line.sb-decoration-drop-after {
  box-shadow: inset 0 -3px 0 0 var(--board-accent-color);
}
```
