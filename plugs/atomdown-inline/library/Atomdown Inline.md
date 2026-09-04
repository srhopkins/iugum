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

## Why the group's hover scope needs a class from the client

The panel's group chrome is subdued at rest and comes forward when the pointer
is anywhere inside that group, including over a member card. Inline, a group is
a run of sibling line elements with **nothing wrapping them**, and CSS has no
previous-sibling combinator, so `:hover` cannot reach the lines above the
pointer. The seam therefore puts `atomdown-group-hover` on every line of the
group the pointer is in, and the rules below use that. The header bar reaches
the same state through `:has(+ .atomdown-group-hover)`, because the bar's next
sibling is its own group's first line.

```space-style
html {
  --board-accent-color: var(--ui-accent-color, #4a7dc7);
  --board-card-radius: 6px;
  --board-card-border-width: 1px;
  --board-card-padding: 8px;
  --board-card-header-padding: 4px 8px;
  --board-card-gap: 14px;
  --board-group-border-width: 2px;
  --board-group-padding: 8px;
  --board-group-header-padding: 5px 8px;
  --board-group-quiet-border: 40%;
  --board-group-quiet-header: 16%;
  --board-grip-size: 14px;
  --board-id-size: 11px;
  --board-header-quiet-color: var(--subtle-color, #888);
  --board-header-active-color: var(--root-color, #222);
  --board-card-surface: var(--ui-surface-section-background-color, #f7f7f7);
  --board-card-border-color: var(--ui-surface-border-color, #ddd);
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
}

/* A card inside a group is inset by the group's interior padding. Two
   classes, so this beats the single-class rule above. */
.cm-line.atomdown-card-line.atomdown-group-line {
  --ad-inset: var(--board-group-padding);
}

/* THE PADDING NEEDS THE ID PREFIX, and that is not a style choice.
   client/styles/editor.scss carries `#sb-main .cm-editor .cm-line { padding: 0 }`,
   which is specificity (1,0,2) and beats any two-class rule no matter how late
   it is injected. Without this prefix the body text of every card sits flush
   against - and for a member card, LEFT OF - its own border, while the header
   widget (not a .cm-line, so not covered by that rule) is correctly inset. */
#sb-main .cm-editor .cm-line.atomdown-card-line {
  padding-left: calc(var(--ad-inset) + var(--board-card-padding));
  padding-right: calc(var(--ad-inset) + var(--board-card-padding));
}

#sb-main .cm-editor .cm-line.atomdown-card-line.atomdown-card-first {
  padding-top: var(--board-card-padding);
}

#sb-main .cm-editor .cm-line.atomdown-card-line.atomdown-card-last {
  padding-bottom: var(--board-card-padding);
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
   directly above this line - so -first adds padding only, above. */
.cm-line.atomdown-card-line.atomdown-card-last::before,
.cm-line.atomdown-card-line.atomdown-card-first.atomdown-card-last::before {
  border-bottom: var(--board-card-border-width) solid
    var(--board-card-border-color);
  border-bottom-left-radius: var(--board-card-radius);
  border-bottom-right-radius: var(--board-card-radius);
}

/* ------------------------------------------------------------------ */
/* A WIDE TABLE STAYS INSIDE ITS CARD.                                 */
/*                                                                     */
/* Fixed layout with wrapped cells, not a horizontal scroller. A card's */
/* body is a run of .cm-line elements that CodeMirror owns and measures; */
/* making a line scroll horizontally would put text outside the box     */
/* posAtCoords reads from and break cursor placement. Wrapping keeps    */
/* every column reachable with no scrolling at all, and the table can    */
/* then never cross the card's border or the group's.                   */
/* ------------------------------------------------------------------ */

#sb-main .cm-editor .cm-line.atomdown-card-line table {
  table-layout: fixed;
  width: 100%;
  max-width: 100%;
}

#sb-main .cm-editor .cm-line.atomdown-card-line th,
#sb-main .cm-editor .cm-line.atomdown-card-line td {
  white-space: normal;
  overflow-wrap: anywhere;
  padding: 6px;
  vertical-align: top;
}

/* Belt and braces: the widget SilverBullet wraps a table in keeps its own
   scrollbar for anything the fixed layout still cannot fit. */
#sb-main .cm-editor .cm-line.atomdown-card-line .sb-table-widget {
  max-width: 100%;
  overflow-x: auto;
}

/* ------------------------------------------------------------------ */
/* THE CARD HEADER ROW: a strip at the top of the box, with the name   */
/* in body text and the id in small grey monospace. It is also the     */
/* box's top edge and top corners.                                     */
/* ------------------------------------------------------------------ */

.sb-decoration-widget.atomdown-card-header {
  --ad-inset: 0px;
  padding: 0;
  margin: 0;
}

.sb-decoration-widget.atomdown-card-header.atomdown-nested {
  --ad-inset: var(--board-group-padding);
  border-left: var(--board-group-border-width) solid
    color-mix(
      in srgb,
      var(--board-accent-color) var(--board-group-quiet-border),
      transparent
    );
  border-right: var(--board-group-border-width) solid
    color-mix(
      in srgb,
      var(--board-accent-color) var(--board-group-quiet-border),
      transparent
    );
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
  /* A hairline under the strip. A shadow, so it costs no height and cannot
     break the box's own border run. */
  box-shadow: inset 0 calc(-1 * var(--board-card-border-width)) 0
    var(--board-card-border-color);
}

/* QUIET AT REST, like the panel's card header. A token, never opacity:
   opacity applies to an element and all its descendants, so on a group it
   would fade every member card inside it. */
.atomdown-card-slug,
.atomdown-card-id {
  color: var(--board-header-quiet-color);
}

.atomdown-card-slug {
  font-size: 13px;
  font-weight: 600;
}

.atomdown-card-id {
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  font-size: var(--board-id-size);
}

/* Full contrast when the pointer is on the header, or anywhere in the card
   the header belongs to. `atomdown-card-hover` is put on that card's lines by
   the seam, because CSS cannot reach a previous sibling. */
.atomdown-card-head:hover .atomdown-card-slug,
.sb-decoration-widget.atomdown-card-header:has(
    + .cm-line.atomdown-card-hover
  ) .atomdown-card-slug {
  color: var(--board-header-active-color);
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
/*                                                                     */
/* SUBDUED AT REST at --board-group-quiet-border of the accent, full   */
/* strength when the pointer is anywhere inside the group. color-mix,  */
/* never opacity, for the reason above. A browser with no color-mix()  */
/* drops the resting declaration and the group is simply always at     */
/* full strength, never at an unreadable half state.                   */
/* ------------------------------------------------------------------ */

.cm-line.atomdown-group-line {
  border-left: var(--board-group-border-width) solid
    var(--board-accent-color);
  border-right: var(--board-group-border-width) solid
    var(--board-accent-color);
}

.cm-line.atomdown-group-line:not(.atomdown-group-hover):not(.atomdown-selected-line) {
  border-color: color-mix(
    in srgb,
    var(--board-accent-color) var(--board-group-quiet-border),
    transparent
  );
}

/* No border-top: the header bar above the opening marker line is the box's
   top edge. The opening marker is a hidden directive, so its collapsed height
   becomes the group's interior top padding. */
#sb-main .cm-editor .cm-line.atomdown-group-line.atomdown-group-first {
  padding-top: var(--board-group-padding);
}

#sb-main .cm-editor .cm-line.atomdown-group-line.atomdown-group-last {
  padding-bottom: var(--board-group-padding);
}

.cm-line.atomdown-group-line.atomdown-group-last {
  border-bottom: var(--board-group-border-width) solid
    var(--board-accent-color);
  border-bottom-left-radius: var(--board-card-radius);
  border-bottom-right-radius: var(--board-card-radius);
}

/* The member cards' own nested-header side borders follow the group. */
.sb-decoration-widget.atomdown-card-header.atomdown-nested:has(
    + .cm-line.atomdown-group-hover
  ) {
  border-left-color: var(--board-accent-color);
  border-right-color: var(--board-accent-color);
}

/* ----------------------------------------------------- */
/* THE GROUP HEADER BAR. The group box's top edge, and    */
/* the panel's own two states: a --board-group-quiet-      */
/* header tint at rest, solid accent when the group is    */
/* under the pointer.                                     */
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
  color: var(--board-header-active-color);
  border: var(--board-group-border-width) solid
    color-mix(
      in srgb,
      var(--board-accent-color) var(--board-group-quiet-border),
      transparent
    );
  border-bottom: none;
  border-top-left-radius: var(--board-card-radius);
  border-top-right-radius: var(--board-card-radius);
  font-size: 13px;
  line-height: 1.3;
  user-select: none;
}

/* Full strength for THIS group only. The bar's next sibling is its group's
   own first line, so if that line carries the seam's hover class the pointer
   is inside this group - including over a member card, which is exactly the
   scope the panel has and plain :hover cannot reach. */
.sb-decoration-widget.atomdown-group-header:hover,
.sb-decoration-widget.atomdown-group-header:has(
    + .cm-line.atomdown-group-hover
  ),
.sb-decoration-widget.atomdown-group-header:has(
    + .cm-line.atomdown-selected-line
  ) {
  background: var(--board-accent-color);
  color: var(--ui-accent-contrast-color, #fff);
  border-color: var(--board-accent-color);
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
/* HIDDEN at rest - every one of them, including the document-level    */
/* `<atomdown version="1"/>` marker, which is the same Lezer node and  */
/* takes the same rule.                                                */
/*                                                                    */
/* Collapsed, not `display: none`: the line element stays in the        */
/* layout so CodeMirror's own coordinate and cursor maths are          */
/* untouched, and the text is revealed in full the moment the cursor   */
/* is ON that line WITH THE EDITOR FOCUSED. The focus condition        */
/* matters: SilverBullet puts the cursor at offset 0 on a page load,   */
/* which is the document marker's own line, so without it that one     */
/* directive would always be revealed on arrival and look like a bug.  */
/* ------------------------------------------------------------------ */

#sb-main .cm-editor .cm-line.atomdown-directive {
  font-size: 1px;
  line-height: 3px;
  color: transparent;
  overflow: hidden;
  padding-top: 0;
  padding-bottom: 0;
  text-indent: 0;
}

#sb-main .cm-editor .cm-focused .cm-line.atomdown-directive.cm-activeLine,
#sb-main .cm-editor .cm-line.atomdown-directive:hover {
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
