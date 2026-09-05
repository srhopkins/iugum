---
description: The header-bar icon and the styling for the inline Atomdown card view
tags: meta
---

The `atomdown-inline` plug draws the Atomdown card view on the page itself. This
page supplies the two things a plug cannot: the header-bar button, and the CSS.

This page and the plug bundle are compiled into the SilverBullet binary that
`iugum wiki` runs, so a space needs no copy of either. To override one, put your
copy at the same path the binary uses - this page at `Library/Atomdown/Inline`,
the bundle at `Library/Atomdown/Plugs/atomdown-inline.plug.js` - and reload the
browser once. Do NOT put the bundle in `_plug/`: SilverBullet loads every
`*.plug.js` a space can see, so a copy there runs the plug a second time
alongside the compiled one.

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
  --board-card-padding: 14px;
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

/* THE HORIZONTAL PADDING NEEDS BOTH THE ID PREFIX AND !important, and neither
   is a style choice.
     - `#sb-main .cm-editor .cm-line { padding: 0 }` in the client's own
       editor.scss is specificity (1,0,2) and beats any two-class rule however
       late it is injected.
     - client/codemirror/list_indent.ts writes `padding-left:Nch;
       text-indent:-Nch` as an INLINE STYLE on every line of every list item,
       and an inline style beats any stylesheet rule that is not !important.
       That hanging indent is what put the `1.` of an ordered list in the
       gutter, OUTSIDE the card's left border, while the wrapped text sat
       correctly inside.
   `text-indent: 0` is the price: wrapped list text now aligns under the
   marker instead of after it. The alternative would need the marker's width in
   CSS, which is per line and not knowable there. The same override also pulls
   in the negative text-indent the client puts on a blockquote line and on a
   heading whose `#` markers are showing. */
#sb-main .cm-editor .cm-line.atomdown-card-line {
  padding-left: calc(var(--ad-inset) + var(--board-card-padding)) !important;
  padding-right: calc(var(--ad-inset) + var(--board-card-padding)) !important;
  text-indent: 0 !important;
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
  position: relative;
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
  position: relative;
  padding-top: 4px;
  padding-bottom: 4px;
  padding-left: var(--board-card-padding);
  padding-right: var(--board-card-padding);
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

/* THE GROUP'S TOP AND BOTTOM INTERIOR PADDING.
   No border-top here: the header bar above the opening marker line is the
   box's top edge. The opening and closing markers are the group's -first and
   -last lines, and both are directives, so this padding is what puts a gap
   between the header bar and the first card, and between the last card and
   the group's bottom border.

   `!important` and four classes, and BOTH are needed. The directive rule
   further down zeroes every side's padding with `!important` - it has to, or
   a collapsed directive would still reserve space - and these two lines are
   directives. Among `!important` declarations specificity decides, so this
   4-class selector beats that 3-class one. Without it the first and last
   member cards butt against the group's inner edges while the ones in the
   middle look right, because the middle gaps come from blank source lines and
   the two ends have no blank line to come from. The value is
   --board-group-padding, the same one that insets the sides, so all four
   sides match and match the panel. */
#sb-main .cm-editor .cm-line.atomdown-group-line.atomdown-group-first {
  padding-top: var(--board-group-padding) !important;
}

#sb-main .cm-editor .cm-line.atomdown-group-line.atomdown-group-last {
  padding-bottom: var(--board-group-padding) !important;
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
  position: relative;
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

/* ONE MENU, NOT TWO BUTTONS. The panel shows Rename and Ungroup as buttons at
   comfortable density and folds them into a menu at compact. The inline view
   has ONE density - there is no density switch on a page - and its narrowest
   editor width is 720px, where two text buttons plus the kind, the name, the
   id and the count do not fit on one row. So the inline view always uses the
   menu, which is also the same control the cards use, so there is one
   mechanism rather than two. */
.atomdown-group-collapse {
  cursor: pointer;
  font-size: 12px;
  line-height: 1.2;
  padding: 1px 5px;
  border-radius: 4px;
  user-select: none;
}

/* Hover inverts the two accent tokens rather than mixing in a new value. */
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
.sb-decoration-widget.atomdown-card-header:has(+ .cm-line.atomdown-card-hover)
  .atomdown-grip,
.atomdown-group-header:hover .atomdown-grip,
.atomdown-grip:focus-visible {
  opacity: 0.5;
}

/* ------------------------------------------------------------------ */
/* THE CARD'S TWO CONTROLS: grip top-LEFT, three-dot menu top-RIGHT,   */
/* the same sides the panel uses.                                      */
/*                                                                     */
/* Both are absolutely positioned inside the header's own padding, and  */
/* that is what reconciles two rules that otherwise fight: the grip has */
/* to be on the left (this rule) while the slug still starts on the     */
/* same left edge as the body text (R2). An in-flow grip ahead of the   */
/* slug pushes the slug right by the grip's width even at opacity 0,    */
/* because its box is still laid out. Out of flow, it pushes nothing -  */
/* which is also why neither control moves anything when it appears.    */
/* --board-card-padding is 14px so the gutter is wide enough to hold    */
/* the glyph without it crossing the card's border.                    */
/* ------------------------------------------------------------------ */

.atomdown-card-head .atomdown-grip,
.atomdown-card-head .atomdown-card-menu {
  position: absolute;
  top: 50%;
  transform: translateY(-50%);
  line-height: 1;
}

.atomdown-card-head .atomdown-grip {
  left: 1px;
}

.atomdown-card-head .atomdown-card-menu {
  right: 1px;
}

.atomdown-card-menu,
.atomdown-group-menu {
  opacity: 0;
  cursor: pointer;
  font-size: 14px;
  letter-spacing: 0;
  color: var(--board-header-quiet-color);
  user-select: none;
}

/* Revealed by a hover anywhere on the card - not only on the header row -
   which needs the seam's hover class, because the header is the card's
   previous sibling and CSS cannot look backwards. Keyboard focus reveals it
   too, so the control is reachable without a pointer. */
.atomdown-card-head:hover .atomdown-card-menu,
.sb-decoration-widget.atomdown-card-header:has(+ .cm-line.atomdown-card-hover)
  .atomdown-card-menu,
.atomdown-card-menu:focus-visible {
  opacity: 0.6;
}

.atomdown-card-menu:hover,
.atomdown-card-menu:focus-visible {
  opacity: 1 !important;
  color: var(--board-header-active-color);
}

/* The group's menu sits at the right end of its bar, in flow, because the bar
   has room for it and it is the bar's only action control now. */
.atomdown-group-menu {
  margin-left: auto;
  color: inherit;
  opacity: 0.7;
}

.atomdown-group-header:hover .atomdown-group-menu,
.atomdown-group-menu:focus-visible {
  opacity: 1;
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
  font-size: 0;
  line-height: 0;
  color: transparent;
  overflow: hidden;
  /* Zeroed so a collapsed directive reserves no space. A group's own marker
     lines override the vertical halves of this - see the group padding rule
     above, which carries one more class so it wins among !important. */
  padding-top: 0 !important;
  padding-bottom: 0 !important;
  padding-left: 0 !important;
  padding-right: 0 !important;
  text-indent: 0 !important;
}

/* THE LINE ITSELF NEVER REVEALS. There is no :hover rule here on purpose:
   a pointer passing over a card's top border used to unfold a 64-character
   digest, which moved the card and everything under it. And the line sits
   ABOVE the card's top edge, so its text appeared outside the box.
   The reveal is the peek below instead: a copy of the directive text carried
   by the card's header widget, absolutely positioned, so it costs no layout
   and is clipped to the card's own padding.

   THE TEXT ARRIVES THROUGH `content: attr(data-directive)`, not as a text
   node. A text node would put the whole directive - id, slug and a
   64-character sha256 digest - into the header widget's own `textContent`, so
   the plumbing would be back in the page's text for everything that reads
   text rather than pixels: a copy of the page, a screen reader, and every DOM
   signature the front-end suite takes. A pseudo-element paints the same
   characters in the same box and contributes none of them to the widget's
   text. */
.atomdown-directive-peek::after {
  content: attr(data-directive);
}

.atomdown-directive-peek {
  display: none;
  position: absolute;
  z-index: 6;
  top: 100%;
  left: calc(var(--ad-inset) + var(--board-card-border-width));
  right: calc(var(--ad-inset) + var(--board-card-border-width));
  box-sizing: border-box;
  padding: 3px var(--board-card-padding);
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  font-size: 0.66em;
  line-height: 1.35;
  color: var(--board-header-quiet-color);
  background: var(--board-card-surface);
  border-bottom: var(--board-card-border-width) solid
    var(--board-card-border-color);
  overflow-wrap: anywhere;
  pointer-events: none;
}

/* The ONLY reveal: the text cursor is in that directive's own line and the
   editor has focus. The directive line is the header widget's previous
   sibling, which is why the adjacent-sibling combinator reaches it. */
#sb-main .cm-editor.cm-focused
  .cm-line.atomdown-directive.cm-activeLine
  + .sb-decoration-widget.atomdown-card-header
  .atomdown-directive-peek {
  display: block;
}

/* A group's opening marker sits AFTER its header bar, so the bar looks
   forward for it instead. */
#sb-main .cm-editor.cm-focused
  .sb-decoration-widget.atomdown-group-header:has(
    + .cm-line.atomdown-group-first.cm-activeLine
  )
  .atomdown-directive-peek {
  display: block;
}

.sb-decoration-widget.atomdown-group-header .atomdown-directive-peek {
  --ad-inset: 0px;
  background: var(--board-accent-color);
  color: var(--ui-accent-contrast-color, #fff);
  border-bottom: none;
}

/* CodeMirror's own active-line background would fight the card surface. */
.cm-line.cm-activeLine {
  background-color: transparent;
}

/* ------------------------------------------------------------------ */
/* A COLLAPSED GROUP.                                                  */
/*                                                                     */
/* The editor's fold placeholder lands inside the group's opening       */
/* marker line, which is a directive and therefore collapsed to         */
/* nothing - so the placeholder would be an unreadable sliver, and      */
/* showing it would mean un-hiding a directive line. It is hidden       */
/* instead: the header bar IS the collapsed group's representation,     */
/* its caret turns from a down triangle to a right one, and the bar     */
/* never recedes while the group is shut, because it is then the only   */
/* thing on screen standing for the contents.                          */
/* ------------------------------------------------------------------ */

#sb-main .cm-editor .cm-line.atomdown-directive .cm-foldPlaceholder {
  display: none;
}

.sb-decoration-widget.atomdown-group-header.atomdown-group-collapsed {
  background: var(--board-accent-color);
  color: var(--ui-accent-contrast-color, #fff);
  border-color: var(--board-accent-color);
  border-bottom: var(--board-group-border-width) solid
    var(--board-accent-color);
  border-bottom-left-radius: var(--board-card-radius);
  border-bottom-right-radius: var(--board-card-radius);
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
