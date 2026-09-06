---
description: The styling for the inline Atomdown card view
tags: meta
---

The `atomdown-inline` plug draws the Atomdown card view on the page itself. This
page supplies the one thing a plug cannot: the CSS.

This page and the plug bundle are compiled into the SilverBullet binary that
`iugum wiki` runs, so a space needs no copy of either. To override one, put your
copy at the same path the binary uses - this page at `Library/Atomdown/Inline`,
the bundle at `Library/Atomdown/Plugs/atomdown-inline.plug.js` - and reload the
browser once. Do NOT put the bundle in `_plug/`: SilverBullet loads every
`*.plug.js` a space can see, so a copy there runs the plug a second time
alongside the compiled one.

# The header-bar icons come from the plug, not from here

Two buttons - `grid` for the card view and `align-justify` for the display
density - and **the plug registers both itself**. There is no
`actionButton.define` block on this page any more.

Why it moved. An action button is plain configuration: `actionButton.define` is
three lines that read the `actionButtons` key, append to it and write it back,
and the `config.insert` syscall is open to a plug. A `space-lua` block, though,
only runs when the client's own page index holds this page, and that index is
per browser origin and can be silently stale - so the same server showed a
button whose command was "not found" in one tab and no styling at all in
another. A plug loads by file discovery with no index involved, which is why the
COMMANDS always worked when the button did not. The button and the command it
runs now arrive together or not at all.

The card view is off until you press the grid button, and both choices are
remembered per page: pressing either on one page never changes another, and the
view stays off across a reload once you turn it off.

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

## The two densities

Comfortable and compact, the same two the panel has, with the same names and
the same knob values. The plug puts `atomdown-comfortable` or
`atomdown-compact` on every decorated line and on every widget, so the whole
density is a set of overrides on those two classes - and because they are the
same `--board-*` property names, overriding one on `html` still moves both
views and both densities together.

**Compact compresses chrome only.** Not one content size is touched below: no
`font-size` on a card body, none on a heading. A heading is a heading at its
full rendered size at both densities, because the point is to fit more of the
document on screen, not to shrink the document.

What compact does:

* **The card header row is gone.** No header, no background, no border and
  deliberately **no dotted or dashed line standing in for it** - there is no
  seam. The card's border is the card, and the rendered content already carries
  the name, because a heading renders as a heading. The card's top edge and top
  corners move from the header widget down onto the card's own first line.
* **The name and the id move to the three-dot menu**, whose picker already
  leads with a non-clickable `name  -  id` label.
* **What is left of the header** is a zero-height layer pinned across the
  card's top edge carrying the grip and the three-dot button and nothing else.
  It is absolutely positioned, so it adds no height and cannot move a card.
  `pointer-events: none` on the layer and `auto` on the two controls, so a
  click on a card's top strip falls THROUGH to the card and still selects it.
* **The group header bar thins**: chevron, name, a bare member count, one menu
  at the right. The GROUP label and the group id fold into that menu.
* **The group outline does not change.** `--board-group-border-width` is
  deliberately absent from the compact block: the outline is structure, not
  chrome, and it is identical at both densities.
* **The collapse chevron does not shrink**, at either density. It is the
  control that turns a long page into a list of group names, so its size is
  restated in the compact block rather than left to inheritance.

```space-style
html {
  --board-accent-color: var(--ui-accent-color, #4a7dc7);
  --board-card-radius: 6px;
  --board-card-border-width: 1px;
  --board-card-padding: 14px;
  --board-card-header-padding: 4px 8px;
  --board-card-gap: 14px;
  /* Room reserved at the top right of a compact card's first line, so the
     three-dot button never lands on top of the content. The panel's own knob,
     same name and same default. */
  --board-card-chrome-space: 24px;
  --board-group-border-width: 2px;
  --board-group-padding: 8px;
  --board-group-header-padding: 5px 8px;
  --board-group-quiet-border: 40%;
  --board-group-quiet-header: 16%;
  /* THE PAGE GUTTER THE CARD'S TWO CONTROLS SIT IN, outside the card's own
     border. Steve's change, iugum-caj.

     22px, and the number is measured rather than chosen. The space available
     outside a card's border is the editor's own content padding (20px) plus
     the page margin, and the page margin is narrowest at the `full` editor
     width, where `min(1600px, 96%)` on a 1440px viewport leaves 28.8px. So the
     budget at the worst width is 48.8px. A 22px offset with a ~16px control
     ends 6px clear of the card, 26px clear of the scroller's own edge, and
     never reaches the point where CodeMirror's `.cm-scroller` (overflow-x:
     auto) would grow a horizontal scrollbar. Rule 1's chrome check measures
     exactly this at all four widths. */
  --board-chrome-gutter: 22px;
  --board-grip-size: 14px;
  --board-id-size: 11px;
  --board-header-quiet-color: var(--subtle-color, #888);
  --board-header-active-color: var(--root-color, #222);
  --board-card-surface: var(--ui-surface-section-background-color, #f7f7f7);
  --board-card-border-color: var(--ui-surface-border-color, #ddd);
  /* THE CARD'S BORDER WHEN THE POINTER IS IN IT. iugum-oip.

     Steve, on the live page: "the CARD border does not change on hover; only
     the GROUP border does." He was right, and it was not a missing class. The
     seam puts `atomdown-card-hover` on every line of the hovered card
     already, and this stylesheet used it - to un-mute the header's name and
     to reveal the two gutter controls. Nothing anywhere read it for the box's
     own edge, so the one thing a reader looks at to answer "which card am I
     on" was the one thing that never moved.

     The accent, the same colour the group's border uses, so the two boxes
     answer that question in one visual language: the group's edge says which
     group, the card's edge says which card inside it. It is a colour change
     only - no width, no shadow, no inset - because rule 3 says reading the
     page may not move it. */
  --board-card-border-hover-color: var(--board-accent-color);
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
/* THE CARD'S OWN BORDER, UNDER THE POINTER. iugum-oip.                */
/*                                                                     */
/* `atomdown-card-hover` is put on every line of the hovered card by   */
/* the seam's `hoverClasses`, for the reason `hoverClasses` exists at  */
/* all: a card is a RUN of sibling `.cm-line` elements with nothing    */
/* wrapping them, so CSS `:hover` cannot name the run. This is the     */
/* rule that reads it for the box's edge. It sets the colour on the    */
/* `::before` that draws the box, and it has to name every side that   */
/* rule sets - `border-color` alone would be overridden by the         */
/* longhand `border-left`/`border-right` above.                        */
/*                                                                     */
/* WHY IT COMES AFTER, and not with a higher specificity: both         */
/* selectors are two classes plus a pseudo element, so source order    */
/* decides, and later is this one. A `:not(.atomdown-card-hover)`      */
/* resting rule was the alternative and it is worse here: the resting  */
/* colour is also the SELECTED card's, and a card can be selected and  */
/* hovered at once.                                                    */
/* ------------------------------------------------------------------ */

.cm-line.atomdown-card-line.atomdown-card-hover::before {
  border-left-color: var(--board-card-border-hover-color);
  border-right-color: var(--board-card-border-hover-color);
}

.cm-line.atomdown-card-line.atomdown-card-hover.atomdown-card-last::before {
  border-bottom-color: var(--board-card-border-hover-color);
}

/* The card's TOP edge and top corners live on the header widget above the
   card's first line, so the top of the same box is coloured there. `:has(+
   .cm-line.atomdown-card-hover)` is the same next-sibling test this
   stylesheet already uses to un-mute the header's text. `box-shadow` is the
   hairline under the strip and it is part of the same edge, so it moves too;
   leaving it behind drew a resting-grey line across a highlighted box. */
.sb-decoration-widget.atomdown-card-header:has(
    + .cm-line.atomdown-card-hover
  ) {
  border-color: var(--board-card-border-hover-color);
  box-shadow: inset 0 calc(-1 * var(--board-card-border-width)) 0
    var(--board-card-border-hover-color);
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

/* ONE MENU, NOT TWO BUTTONS, AT BOTH DENSITIES. The panel shows Rename and
   Ungroup as buttons at comfortable density and folds them into a menu at
   compact. The inline view uses the menu at both, because its narrowest
   editor width is 720px, where two text buttons plus the kind, the name, the
   id and the count do not fit on one row even in comfortable. The menu is
   also the same control the cards use, so there is one mechanism rather than
   two. */
.atomdown-group-collapse {
  cursor: pointer;
  font-size: 12px;
  line-height: 1.2;
  padding: 1px 5px;
  border-radius: 4px;
  user-select: none;
}

/* HOVER ON THE ACCENT FILL, and this is a fix (iugum-caj item 2).

   It used to paint a SOLID `--ui-accent-contrast-color` chip, which is white
   in both themes. On the group bar's resting tint that is a light pill on a
   pale ground and reads fine; on the saturated accent fill the bar takes when
   the pointer is inside the group, it reads as a hole punched through the bar.
   Steve reported exactly that.

   The treatment is a translucent wash of the contrast colour with the text
   colour left alone, so the control lifts off whatever is behind it instead of
   replacing it. It works on both of the bar's two states and on either theme,
   because it is relative to the fill rather than an absolute colour. A browser
   with no color-mix() drops the declaration and the control is simply not
   tinted - never a white hole. */
.atomdown-group-collapse:hover,
.atomdown-group-collapse:focus-visible,
.atomdown-group-menu:hover,
.atomdown-group-menu:focus-visible {
  background: color-mix(
    in srgb,
    var(--ui-accent-contrast-color, #fff) 24%,
    transparent
  );
  color: inherit;
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

/* :focus AND :focus-visible, the panel's own lesson: :focus-visible does not
   match a focus set by script or by a click, so a control that HAS focus was
   still at opacity 0. A focused control the reader cannot see is a control
   they cannot use, whichever way the focus arrived. */
.atomdown-card-head:hover .atomdown-grip,
.sb-decoration-widget.atomdown-card-header:has(+ .cm-line.atomdown-card-hover)
  .atomdown-grip,
.atomdown-group-header:hover .atomdown-grip,
.atomdown-grip:focus,
.atomdown-grip:focus-visible {
  opacity: 0.5;
}

/* ------------------------------------------------------------------ */
/* THE CARD'S TWO CONTROLS, OUTSIDE THE CARD: grip in the LEFT page    */
/* gutter, vertical three-dot menu in the RIGHT one. Steve's change,   */
/* iugum-caj item 4.                                                   */
/*                                                                     */
/* WHAT MOVED AND WHY. They used to be pinned inside the header's own   */
/* --board-card-padding. That worked but spent 14px of card width on    */
/* each side that the content could not use, and at compact - where the */
/* header row has no height at all - the menu had to float over the     */
/* card's first line, which is why compact reserved                    */
/* --board-card-chrome-space at the top right. Outside the border there */
/* is nothing to float over, so that reservation is gone and a compact  */
/* card is the full column wide.                                        */
/*                                                                      */
/* STILL OUT OF FLOW, and that has not changed for the original reason:  */
/* an in-flow grip ahead of the slug pushes the slug right by the grip's */
/* width even at opacity 0, because its box is still laid out, and the   */
/* slug has to start on the same left edge as the body text (R2). Out of */
/* flow, neither control moves anything when it appears on hover.       */
/*                                                                      */
/* THE OFFSET IS FROM THE CARD HEAD'S OWN BORDER BOX, which is what      */
/* makes one rule right for both a top-level and a member card. A        */
/* member card's head is inset by the group's padding and sits inside    */
/* the group's 2px outline, so the same -22px lands 12px OUTSIDE that    */
/* outline; a top-level card's head has no inset, so it lands 22px       */
/* outside the card border. Neither can touch the group's outline, and   */
/* rule 1's chrome check measures both.                                  */
/*                                                                      */
/* NOT CLIPPED, measured rather than assumed. `.cm-scroller` is the only */
/* ancestor between a card and the body with a non-visible overflow      */
/* (`auto`), and it spans the whole viewport while the content column is  */
/* centred inside it - so a control 22px outside the column is still     */
/* well inside the clipper. Verified at all four editor widths on the    */
/* real fixture, by rect AND by `elementFromPoint`, because a clipped     */
/* element still reports its unclipped rect.                             */
/* ------------------------------------------------------------------ */

.atomdown-card-head .atomdown-grip,
.atomdown-card-head .atomdown-card-menu {
  position: absolute;
  top: 50%;
  transform: translateY(-50%);
  line-height: 1;
  /* The controls are the one thing in the header layer that must stay
     clickable: compact sets `pointer-events: none` on the layer itself so a
     click on a card's top strip falls through to the card. */
  pointer-events: auto;
}

.atomdown-card-head .atomdown-grip {
  left: calc(-1 * var(--board-chrome-gutter));
}

.atomdown-card-head .atomdown-card-menu {
  right: calc(-1 * var(--board-chrome-gutter));
}

/* ------------------------------------------------------------------ */
/* THE POPOVER. The card's own menu, anchored to the card, NOT the      */
/* host's command palette. iugum-caj item 1.                            */
/*                                                                     */
/* It hangs from the three-dot button, which is in the right gutter, so */
/* the popover opens back over the card rather than further out of the  */
/* column: `right: 0` on a box whose containing block is the card head  */
/* puts its right edge on the card's right border and grows it inward.  */
/* At the narrow width that is the only placement that fits.            */
/*                                                                     */
/* IT LEAVES THE CARD ON PURPOSE, downward, the way every popover does. */
/* Rule 1 knows: chrome may sit outside the box it belongs to, and is    */
/* checked for clipping and for overlap instead of for containment.     */
/* ------------------------------------------------------------------ */

.atomdown-menu-popover {
  position: absolute;
  top: calc(100% + 2px);
  right: 0;
  z-index: 40;
  display: flex;
  flex-direction: column;
  min-width: 220px;
  max-width: 320px;
  padding: 6px;
  background: var(--ui-surface-background-color, #fff);
  border: 1px solid var(--board-card-border-color);
  border-radius: 6px;
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.2);
  color: var(--board-header-active-color);
  font-size: 13px;
  line-height: 1.4;
  text-align: left;
  /* The card head is out of flow and its text is centred vertically; the
     popover is a block of rows and inherits neither. */
  transform: none;
  pointer-events: auto;
  user-select: none;
}

/* The identity label: the popover's FIRST child and inert, the same shape the
   panel's popover has. `cursor: default`, no hover state, and the click
   handler treats a hit on it as "keep the menu open" rather than as an
   action - a reader clicks the thing that names the card first. */
.atomdown-menu-label {
  display: flex;
  flex-direction: column;
  gap: 1px;
  padding: 4px 6px 6px;
  margin-bottom: 4px;
  border-bottom: 1px solid var(--board-card-border-color);
  cursor: default;
}

.atomdown-menu-label-text {
  font-weight: 600;
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  font-size: 12px;
  overflow-wrap: anywhere;
}

.atomdown-menu-label-note {
  font-size: 11px;
  color: var(--board-header-quiet-color);
}

.atomdown-menu-item {
  padding: 4px 6px;
  border-radius: 4px;
  cursor: pointer;
}

.atomdown-menu-item:hover,
.atomdown-menu-item:focus-visible {
  background: var(--ui-surface-hover-background-color, #eee);
}

/* An open menu keeps its own button visible, or the control the popover
   belongs to disappears the moment the pointer moves onto the popover. */
.atomdown-menu-open .atomdown-card-menu,
.atomdown-menu-open .atomdown-group-menu {
  opacity: 1 !important;
}

/* The group's popover hangs from the bar's right end, inside the bar's own
   colour rather than inheriting the accent fill's white text. */
.atomdown-group-header .atomdown-menu-popover {
  color: var(--board-header-active-color);
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
.atomdown-card-menu:focus,
.atomdown-card-menu:focus-visible {
  opacity: 0.6;
}

.atomdown-card-menu:hover,
.atomdown-card-menu:focus,
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
.atomdown-group-menu:focus,
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

/* ================================================================== */
/* THE TWO DENSITIES.                                                  */
/*                                                                     */
/* CHROME ONLY. There is no font-size below, on a card body or on a    */
/* heading, and there is none anywhere else in this block either: a    */
/* heading is a heading at its full rendered size at both densities.   */
/*                                                                     */
/* Comfortable needs no overrides - the defaults at the top of this    */
/* block ARE comfortable. `atomdown-comfortable` is still on every     */
/* decorated line and every widget, so the DOM says which density is   */
/* showing instead of leaving it to an absence.                        */
/*                                                                     */
/* Every value here is one of the panel's own named properties with    */
/* the panel's own compact value, so overriding one on `html` moves    */
/* both views and both densities together. --board-group-border-width  */
/* is deliberately absent: the group outline is structure, not chrome, */
/* and it is IDENTICAL at both densities.                              */
/* ================================================================== */

.cm-line.atomdown-compact-line,
.sb-decoration-widget.atomdown-compact {
  --board-card-radius: 4px;
  --board-card-padding: 6px;
  --board-card-header-padding: 0;
  --board-card-gap: 6px;
  --board-group-padding: 4px;
  --board-group-header-padding: 1px 4px;
}

/* --- COMPACT: the card header row is gone --------------------------
   The row is lifted out of the layout entirely. It has no height, no
   padding, no background and NO border - and deliberately no dotted or
   dashed line standing in for it either. There is no seam. The card's
   border is the card, and the rendered content already carries the name,
   because a heading renders as a heading. The name and the id move to the
   three-dot menu, whose picker leads with a `name  -  id` label that does
   nothing when chosen.
   What is left is a zero-height layer pinned across the card's top edge,
   carrying the grip and the three-dot button and nothing else. Absolutely
   positioned, so it adds no height and can move no card - which is why
   rule 3 (layout stability) holds at compact for the same reason it holds
   at comfortable.
   pointer-events: none on the layer and auto on the two controls, so a
   click on a card's top strip falls THROUGH to the card and still selects
   it. Selection, modifier-click, the lasso and the grip drag are all
   untouched. */
.sb-decoration-widget.atomdown-card-header.atomdown-compact {
  height: 0;
  overflow: visible;
  border: none;
  pointer-events: none;
}

.atomdown-compact .atomdown-card-head {
  position: absolute;
  top: 0;
  left: var(--ad-inset);
  right: var(--ad-inset);
  z-index: 6;
  margin: 0;
  padding: var(--board-card-header-padding);
  background: transparent;
  border: none;
  border-radius: 0;
  box-shadow: none;
  pointer-events: none;
}

.atomdown-compact .atomdown-card-slug,
.atomdown-compact .atomdown-card-id,
.atomdown-compact .atomdown-card-badge {
  display: none;
}

/* COMPACT USES THE SAME GUTTER, so there is one rule for both densities and
   nothing to restate here.

   It used to be different: with no header row to pin them into, the two
   controls came back INTO flow and floated over the card's first line, behind
   an opaque chip so they would not smear the text. Outside the border there is
   no text to smear and no chip is needed, so both of those rules are gone -
   and so is the padding compact used to reserve inside the card for the
   three-dot button (--board-card-chrome-space, below). That reservation was
   the last thing spending card width on chrome at compact, which is the
   density where width matters most. */

/* THE CARD'S TOP EDGE AND TOP CORNERS MOVE ONTO THE CARD'S FIRST LINE,
   because the header widget that carried them is out of the layout now.
   Without this the box would be open at the top. */
.cm-line.atomdown-compact-line.atomdown-card-first::before {
  border-top: var(--board-card-border-width) solid
    var(--board-card-border-color);
  border-top-left-radius: var(--board-card-radius);
  border-top-right-radius: var(--board-card-radius);
}

/* And the top edge follows the pointer at this density too. iugum-oip's DoD
   names both densities, and at compact the top edge is HERE rather than on
   the header widget, so the comfortable rule cannot cover it. */
.cm-line.atomdown-compact-line.atomdown-card-first.atomdown-card-hover::before {
  border-top-color: var(--board-card-border-hover-color);
}

/* NO ROOM IS RESERVED AT THE TOP RIGHT ANY MORE. The three-dot button used to
   float over the card's first line at this density, so that line carried an
   extra --board-card-chrome-space of right padding to keep the content out
   from under it. The button is outside the card's border now, so there is
   nothing to reserve for and a compact card's first line is as wide as every
   other line of it. --board-card-chrome-space is kept as a knob because the
   board panel still has it and the two views share their knob names. */

/* --- COMPACT: a thin group bar -------------------------------------
   Chevron, name, a bare member count, one three-dot menu. The GROUP label
   and the group id fold into that menu. The 2px accent outline around the
   group is NOT touched. */
.sb-decoration-widget.atomdown-group-header.atomdown-compact {
  flex-wrap: nowrap;
}

.atomdown-compact .atomdown-group-kind,
.atomdown-compact .atomdown-group-id,
.atomdown-compact .atomdown-group-count-word {
  display: none;
}

.atomdown-compact .atomdown-group-menu {
  margin-left: auto;
}

/* THE COLLAPSE CHEVRON IS THE ONE THING COMPACT DOES NOT COMPRESS.
   It is the control that turns a long page into a list of group names, so
   it keeps its full size and stays visible - never hover-only - at both
   densities. Restated here rather than left to inheritance, so a later
   change to the bar's padding cannot shrink it by accident. */
.atomdown-compact .atomdown-group-collapse {
  font-size: 12px;
  line-height: 1.2;
  padding: 1px 5px;
  opacity: 1;
}

/* ------------------------------- */
/* Selection, lasso, drag feedback. */
/* ------------------------------- */

.cm-line.atomdown-selected-line::before {
  background: var(--ui-surface-hover-background-color, #eaeaea);
  border-color: var(--board-accent-color);
}

/* At compact the card's TOP border is drawn by a two-class rule above, so
   the one-class selection rule cannot recolour it. Restated at matching
   specificity, and after it, so a selected compact card is outlined on all
   four sides like a selected comfortable one. */
.cm-line.atomdown-compact-line.atomdown-selected-line::before {
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
