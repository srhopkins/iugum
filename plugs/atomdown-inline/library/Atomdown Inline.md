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

## Why both boxes are pseudo-elements and the real border is the inset

A line inside a group carries **both** ranges' line classes, and one element
can only have one `border-left`. So neither box can have it - and the line's
real border is spent on something else now: it IS the card's horizontal inset.
See "The horizontal inset is a transparent border" below for why it has to be.

That leaves a line's two pseudo-elements, and a line has exactly two:

* the **card** is the `::before` it always was, pulled back out of the inset by
  the card's own horizontal padding,
* the **group** is the `::after`, pulled back out by the whole inset, so its
  left edge lands on the content column where the real border used to draw.

A member card therefore floats inside the group with a gap on all four sides,
and a top-level card has no group frame and is otherwise the identical box.

## The three horizontal measurements, by name

Every rule below uses these, and they are set on the line:

| | |
|---|---|
| `--ad-inset` | the group's interior padding: `0` for a top-level card, `--board-group-padding-x` for a member card |
| `--ad-frame` | the group box's outer edge to the card box's own border: the group's stroke plus that padding |
| `--ad-lead` | the line's own transparent border width: the frame plus `--board-card-padding-x`. `0` on a group's own marker lines, which carry no card |

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

**Compact compresses the VERTICAL axis only.** `--board-card-padding` and
`--board-group-padding` each carried both axes, so compact used to move the
horizontal distance from a card's border to its first glyph as well - 14px to
6px - and the distance from a group's border to a member card's border with it.
Each knob is now two, `-x` and `-y`, both derived from the original, and
compact overrides the `-y` half only. The horizontal distances are IDENTICAL at
both densities, because that distance is what a reader's eye uses to find the
start of a line: moving it makes compact read as a different document rather
than as the same one closer together. `--board-card-gap` is vertical by
definition and may keep shrinking.

The original knob names still work and still mean what they meant. Set
`--board-card-padding` on `html` and both axes and both views move together;
compact then still takes its own vertical value.

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
  at the right. The GROUP label and the group id fold into that menu. The bar
  keeps its resting tint at both densities - it is the only thing left naming a
  group, so a group with an invisible resting outline is still findable.
* **Compact is an outline, not a surface.** No fill: `--board-card-surface`
  becomes the theme's own page background token, `--root-background-color`, so
  it is right in light, in dark and under a third theme, rather than a colour
  that happens to match today. The strokes turn DOTTED.
* **The collapse chevron does not shrink**, at either density. It is the
  control that turns a long page into a list of group names, so its size is
  restated in the compact block rather than left to inheritance.

## How quiet works at compact, and what it costs

Steve's rule: *"just make the dotted border same as background theme so no size
for border, only headers go away."*

So the stroke is **always present and always the same width**. At rest its
colour is the page background, which makes it invisible while it still occupies
its space; the pointer gives it its visible colour. Nothing about the geometry
changes between rest and hover, so nothing below a card can move - the same
trick that already reserves the box for the hover-only controls. What recedes
at compact is the card HEADER ROW, not any geometry.

Comfortable shares the mechanism rather than opting out of it: its resting
colour IS its visible colour, so hover changes nothing there.

**Room for a stale state, and how.** Every stroke's colour arrives through
exactly two knobs per box - `--board-card-border-rest-color` for the resting
colour and `--board-card-border-color` for the visible one, plus
`--board-accent-color` for selection - and no rule below sets a colour any
other way. A per-card stale state (amber, when a digest no longer matches its
content) is therefore one declaration on that card's lines, at either density,
with no geometry to reconsider: width and style belong to the density, colour
belongs to the state. The inline view has **no** stale indicator today - that
was built for the `atomdown-board` panel only - so nothing here designs for
it; see `iugum-abi`.

**THE TRADE, RECORDED.** The rule this replaces read: *"the group outline does
not change - the outline is structure, not chrome, and it is identical at both
densities."* Compact now draws it dotted, and quiet at rest. The rule is
narrowed rather than dropped:

> A group's outline keeps its PRESENCE and its GEOMETRY at every density - same
> width, same position, never absent. Its STROKE STYLE and its RESTING COLOUR
> may vary with the density.

The reason the original rule existed is intact: the outline never disappears
and never moves, so it cannot stop being the thing that says "these cards are
one group", and no density change can reflow the page. What varies is how
loudly it says it, which is chrome. A dotted, background-coloured stroke is
still a stroke at the same 2px in the same place - `getComputedStyle` proves
both, and the front-end suite asserts them.

```space-style
html {
  --board-accent-color: var(--ui-accent-color, #4a7dc7);
  --board-card-radius: 6px;
  --board-card-border-width: 1px;
  --board-card-padding: 14px;
  /* THE TWO AXES OF ONE KNOB. `--board-card-padding` is still the knob a
     space-style page sets and still means "a card's padding"; these two are
     derived from it, so overriding it on `html` still moves both views, both
     axes and both densities. The compact block overrides the `-y` half ONLY -
     the horizontal distance from a card's border to its first glyph is
     identical at both densities. See "Compact compresses the VERTICAL axis
     only" above. */
  --board-card-padding-x: var(--board-card-padding);
  --board-card-padding-y: var(--board-card-padding);
  /* Declared because the panel has it. The inline card head reads
     --board-card-padding-x horizontally and its own vertical value, for the
     same reason the padding knob is split - see the head's own rule. */
  --board-card-header-padding: 4px 8px;
  /* Vertical by definition, so the density may keep shrinking it. */
  --board-card-gap: 14px;
  /* Room reserved at the top right of a compact card's first line, so the
     three-dot button never lands on top of the content. The panel's own knob,
     same name and same default. */
  --board-card-chrome-space: 24px;
  --board-group-border-width: 2px;
  --board-group-padding: 8px;
  /* Same split, same reason: the distance from a group's border to a member
     card's border is identical at both densities. */
  --board-group-padding-x: var(--board-group-padding);
  --board-group-padding-y: var(--board-group-padding);
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

  /* ---- THE STROKES: style, resting colour, surfaces --------------------
     Three knobs per box, and every rule below reads them rather than naming
     a colour of its own. That is what leaves room for a per-card state - a
     stale digest, later - to be one declaration with no geometry to redo.

     STYLE belongs to the density: solid at comfortable, dotted at compact.
     The WIDTH never changes, at either density or in any state.
     The RESTING COLOUR belongs to the density too: the visible colour at
     comfortable, the page's own background token at compact, so the stroke
     is present, is the same width, and is invisible until the pointer
     arrives. See "How quiet works at compact" above. */
  --board-card-border-style: solid;
  --board-group-border-style: solid;
  --board-card-border-rest-color: var(--board-card-border-color);
  --board-group-border-rest-color: color-mix(
    in srgb,
    var(--board-accent-color) var(--board-group-quiet-border),
    transparent
  );
  /* A group has no fill of its own at comfortable. At compact both boxes take
     the page background token, which is what makes compact an outline rather
     than a surface. */
  --board-group-surface: transparent;
  --board-card-selected-surface: var(--ui-surface-hover-background-color, #eaeaea);
}

/* ------------------------------------------------------------------ */
/* THE CARD BOX.                                                       */
/* One closed rounded box per atom, drawn by a ::before pulled back    */
/* out of the line's own transparent inset. The three measurements are  */
/* --ad-inset, --ad-frame and --ad-lead; see "The three horizontal      */
/* measurements, by name" above.                                        */
/* ------------------------------------------------------------------ */

.cm-line.atomdown-card-line,
.cm-line.atomdown-group-line {
  --ad-inset: 0px;
  --ad-frame: 0px;
  --ad-lead: 0px;
  position: relative;
}

.cm-line.atomdown-card-line {
  --ad-lead: var(--board-card-padding-x);
}

/* A card inside a group sits inside the group's stroke AND its interior
   padding. Two classes, so this beats the rules above. A group's own marker
   lines are not card lines, so they keep --ad-lead: 0 - they carry no card
   and therefore no inset. */
.cm-line.atomdown-card-line.atomdown-group-line {
  --ad-inset: var(--board-group-padding-x);
  --ad-frame: calc(var(--board-group-border-width) + var(--ad-inset));
  --ad-lead: calc(var(--ad-frame) + var(--board-card-padding-x));
}

/* ===================================================================== */
/* THE HORIZONTAL INSET IS A TRANSPARENT BORDER, NOT PADDING.            */
/* iugum-3ad. This is the whole reason the two boxes moved to pseudo-     */
/* elements, so it is worth the paragraphs.                              */
/*                                                                       */
/* WHAT WENT WRONG WITH PADDING. client/codemirror/list_indent.ts writes  */
/* `padding-left:Nch;text-indent:-Nch` as an INLINE STYLE on every line   */
/* of every list item, where N is that item's own marker width. That pair */
/* IS the hanging indent: the marker starts at the line's own left edge   */
/* and the wrapped rows start after it. The card used to take its         */
/* horizontal inset from `padding-left` with `!important`, which had to    */
/* clobber the client's inline `padding-left` - and then `text-indent: 0   */
/* !important` as well, or an ordered list's `1.` was pulled N characters  */
/* left, into the page gutter, OUTSIDE the card's left border. Rule 1      */
/* CONTENT catches exactly that. The price was the hanging indent: every   */
/* wrapped list row aligned under the marker instead of after it. Steve    */
/* says the price is too high.                                            */
/*                                                                       */
/* WHY A BORDER COMPOSES WHERE PADDING CANNOT. list_indent.ts sets         */
/* `padding` and `text-indent` and NOTHING ELSE - no margin, no border. A  */
/* border is a length the card can own outright, so the two never meet:    */
/*                                                                       */
/*   - the line's border-left and border-right are --ad-lead of            */
/*     TRANSPARENT, which is the whole inset: the group's frame plus the   */
/*     card's own horizontal padding;                                      */
/*   - `padding-left` is left to the client, so a list line keeps its      */
/*     `Nch` and its `-Nch` and a plain line keeps the client's `padding:  */
/*     0`;                                                                 */
/*   - the BORDER BOX is unchanged, so the line still spans the content    */
/*     column and every rect the front-end suite measures still means      */
/*     what it meant. Margin would have moved it.                          */
/*                                                                       */
/* So a card's first glyph sits at frame + card padding, its list marker   */
/* sits there too, and its wrapped rows hang N characters further in.      */
/*                                                                       */
/* !important ON THE WIDTHS, and only there. The inset is what keeps       */
/* content inside the card, so no client rule may shorten it -             */
/* `.sb-admonition` sets `border-left-width: 4px !important`, which would   */
/* otherwise leave an admonition's text 4px from the card's border. Among   */
/* !important declarations specificity decides, and the id prefix wins.     */
/* The client's own 4px bar is redrawn as a shadow, below, so nothing is    */
/* lost.                                                                   */
/*                                                                       */
/* THE ID PREFIX is still needed for everything else here: `#sb-main        */
/* .cm-editor .cm-line { padding: 0 }` in the client's editor.scss is       */
/* (1,0,2) and beats any two-class rule however late it is injected.        */
/* ===================================================================== */
#sb-main .cm-editor .cm-line.atomdown-card-line {
  border-left-style: solid;
  border-right-style: solid;
  border-left-color: transparent;
  border-right-color: transparent;
  border-left-width: var(--ad-lead) !important;
  border-right-width: var(--ad-lead) !important;
  /* A line background - a blockquote's, a fenced code block's - is the
     client's, and `background-clip: border-box` would paint it across the
     transparent inset and out over the card's border. Clip it to the padding
     box and it stops at the card's text origin. */
  background-clip: padding-box;
}

#sb-main .cm-editor .cm-line.atomdown-card-line.atomdown-card-first {
  padding-top: var(--board-card-padding-y);
}

#sb-main .cm-editor .cm-line.atomdown-card-line.atomdown-card-last {
  padding-bottom: var(--board-card-padding-y);
}

/* --- THE CLIENT'S OTHER HANGING INDENTS, COMPLETED ------------------
   list_indent.ts writes a MATCHED pair. Two of the client's stylesheet
   outdents do not: `.sb-line-blockquote` takes `text-indent: -2ch` with no
   padding at all (`.sb-blockquote-outside` narrows that to -1ch once the
   quote mark is hidden), and a heading whose `#` markers are showing takes
   -2ch to -7ch the same way. Outside a card those hang into the page's own
   margin, which is empty. Inside a card that margin is the card's border, so
   the marker crossed it - which is what `text-indent: 0 !important` used to
   suppress, at the cost of the hanging indent everywhere else.

   Give each one the padding it is missing and both properties hold: the
   marker sits at the card's text origin and the wrapped rows sit after it.

   NO `!important`, deliberately. A blockquote that is also a list item
   carries the client's inline pair, and an inline style has to keep winning
   there - `!important` here would take the list's own marker width away
   again. */
#sb-main .cm-editor .cm-line.atomdown-card-line.sb-line-blockquote {
  padding-left: 2ch;
  /* The client draws the blockquote's bar as the LINE's own `border-left`,
     and that border is the card's inset now. Redraw it as an inset shadow at
     the line's padding edge - which is a better place than the client's own,
     because the client's put the bar underneath the card's border. */
  box-shadow: inset 1px 0 0 0 var(--editor-blockquote-border-color);
}

#sb-main .cm-editor .cm-line.atomdown-card-line.sb-blockquote-outside {
  padding-left: 1ch;
}

/* Same treatment for an admonition's 4px bar, for the reason in the
   !important paragraph above. */
#sb-main .cm-editor .cm-line.atomdown-card-line.sb-admonition {
  box-shadow: inset 4px 0 0 0 var(--admonition-color);
}

#sb-main .cm-editor .cm-line.atomdown-card-line.sb-header-inside.sb-line-h1 {
  padding-left: 2ch;
}
#sb-main .cm-editor .cm-line.atomdown-card-line.sb-header-inside.sb-line-h2 {
  padding-left: 3ch;
}
#sb-main .cm-editor .cm-line.atomdown-card-line.sb-header-inside.sb-line-h3 {
  padding-left: 4ch;
}
#sb-main .cm-editor .cm-line.atomdown-card-line.sb-header-inside.sb-line-h4 {
  padding-left: 5ch;
}
#sb-main .cm-editor .cm-line.atomdown-card-line.sb-header-inside.sb-line-h5 {
  padding-left: 6ch;
}
#sb-main .cm-editor .cm-line.atomdown-card-line.sb-header-inside.sb-line-h6 {
  padding-left: 7ch;
}

/* The client renders those `#` markers INLINE below its own default editor
   width, where the page has no margin left to hang them in - so it zeroes the
   text-indent there. Mirror that, or the padding would indent a heading with
   nothing hanging back into it. The threshold tracks the client's own, which
   cannot read a custom property either. */
@media (max-width: 800px) {
  #sb-main .cm-editor .cm-line.atomdown-card-line.sb-header-inside.sb-line-h1,
  #sb-main .cm-editor .cm-line.atomdown-card-line.sb-header-inside.sb-line-h2,
  #sb-main .cm-editor .cm-line.atomdown-card-line.sb-header-inside.sb-line-h3,
  #sb-main .cm-editor .cm-line.atomdown-card-line.sb-header-inside.sb-line-h4,
  #sb-main .cm-editor .cm-line.atomdown-card-line.sb-header-inside.sb-line-h5,
  #sb-main .cm-editor .cm-line.atomdown-card-line.sb-header-inside.sb-line-h6 {
    padding-left: 0;
  }
}

.cm-line.atomdown-card-line::before {
  content: "";
  position: absolute;
  z-index: -1;
  /* NEGATIVE, and by exactly the card's own horizontal padding. An absolutely
     positioned box is placed against its containing block's PADDING box,
     which is inside the transparent inset, so this puts the card's border
     back where the inset started: --ad-frame from the group's outer edge, and
     the content column's edge for a top-level card. */
  left: calc(-1 * var(--board-card-padding-x));
  right: calc(-1 * var(--board-card-padding-x));
  top: 0;
  bottom: 0;
  background: var(--board-card-surface);
  /* WIDTH AND STYLE HERE, COLOUR IN ONE PLACE, and that is Steve's rule for
     how quiet works rather than a tidiness preference: the state rules below
     change `border-color` and nothing else, so rest and hover are identical
     in geometry and nothing can reflow. A `border-left: W S C` shorthand in a
     state rule would reset the width with it. */
  border-left-width: var(--board-card-border-width);
  border-left-style: var(--board-card-border-style);
  border-right-width: var(--board-card-border-width);
  border-right-style: var(--board-card-border-style);
  border-color: var(--board-card-border-rest-color);
  pointer-events: none;
}

/* The card's top edge and top corners live on the header widget, which sits
   directly above this line - so -first adds padding only, above. */
.cm-line.atomdown-card-line.atomdown-card-last::before,
.cm-line.atomdown-card-line.atomdown-card-first.atomdown-card-last::before {
  border-bottom-width: var(--board-card-border-width);
  border-bottom-style: var(--board-card-border-style);
  border-bottom-left-radius: var(--board-card-radius);
  border-bottom-right-radius: var(--board-card-radius);
}

/* THE POINTER GIVES THE STROKE ITS COLOUR BACK. At comfortable the resting
   colour already IS this one, so hover changes nothing there and both
   densities share one mechanism. At compact the resting colour is the page
   background, so this is the whole of the reveal - and it is a colour change
   only, so the card does not move and neither does anything under it. */
.cm-line.atomdown-card-line.atomdown-card-hover::before {
  border-color: var(--board-card-border-color);
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
  --ad-frame: 0px;
  padding: 0;
  margin: 0;
  position: relative;
}

/* The group's outline continues through a member card's header widget, and
   here it IS the real border: a widget carries no list indent, so nothing is
   contesting it. Width and style from the same two knobs the group's ::after
   reads, so the density moves both together. */
.sb-decoration-widget.atomdown-card-header.atomdown-nested {
  --ad-inset: var(--board-group-padding-x);
  --ad-frame: calc(var(--board-group-border-width) + var(--ad-inset));
  border-left-width: var(--board-group-border-width);
  border-left-style: var(--board-group-border-style);
  border-right-width: var(--board-group-border-width);
  border-right-style: var(--board-group-border-style);
  border-left-color: var(--board-group-border-rest-color);
  border-right-color: var(--board-group-border-rest-color);
}

.atomdown-card-head {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-left: var(--ad-inset);
  margin-right: var(--ad-inset);
  position: relative;
  /* Vertical from the density, horizontal from the constant. The two gutter
     controls are offset from THIS box, so a horizontal value that moved with
     the density would move them too. Same requirement-2 split the card lines
     use. */
  padding-top: 4px;
  padding-bottom: 4px;
  padding-left: var(--board-card-padding-x);
  padding-right: var(--board-card-padding-x);
  background: var(--board-card-surface);
  border-width: var(--board-card-border-width);
  border-style: var(--board-card-border-style);
  border-color: var(--board-card-border-rest-color);
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
/* THE GROUP BOX: one closed rounded 2px accent box around its cards,  */
/* drawn by the line's ::after.                                        */
/*                                                                     */
/* WHY ::after AND NOT THE REAL BORDER ANY MORE (iugum-3ad). The group  */
/* used to take the real `border`, because a line inside a group        */
/* carries both ranges' line classes and one element has one           */
/* `border-left`. The card's horizontal inset IS that border now - see  */
/* the card box above for why it has to be - so the group moved to the  */
/* line's other pseudo-element. A line has both, so the two boxes and   */
/* the inset all fit on one element with nothing left contested.        */
/*                                                                     */
/* IT DRAWS WHERE IT ALWAYS DREW. `-1 * --ad-lead` puts its left edge   */
/* back on the content column, which is where the real border was, and  */
/* --ad-lead is 0 on a group's own marker lines. A member card's        */
/* --ad-frame then puts the card's border 2px + the group's padding     */
/* inside it, the same two numbers as before.                           */
/*                                                                     */
/* z-index -2, ONE BEHIND the card's ::before, so a selected member     */
/* card's own surface is not painted over by the group's.               */
/*                                                                     */
/* SUBDUED AT REST at --board-group-quiet-border of the accent, full    */
/* strength when the pointer is anywhere inside the group; at compact   */
/* the resting colour is the page background instead. color-mix,        */
/* never opacity, for the reason above. A browser with no color-mix()   */
/* drops the resting declaration and the group is simply always at      */
/* full strength, never at an unreadable half state.                    */
/* ------------------------------------------------------------------ */

.cm-line.atomdown-group-line::after {
  content: "";
  position: absolute;
  z-index: -2;
  left: calc(-1 * var(--ad-lead));
  right: calc(-1 * var(--ad-lead));
  top: 0;
  bottom: 0;
  background: var(--board-group-surface);
  border-left-width: var(--board-group-border-width);
  border-left-style: var(--board-group-border-style);
  border-right-width: var(--board-group-border-width);
  border-right-style: var(--board-group-border-style);
  border-color: var(--board-accent-color);
  pointer-events: none;
}

.cm-line.atomdown-group-line:not(.atomdown-group-hover):not(.atomdown-selected-line)::after {
  border-color: var(--board-group-border-rest-color);
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
  padding-top: var(--board-group-padding-y) !important;
}

#sb-main .cm-editor .cm-line.atomdown-group-line.atomdown-group-last {
  padding-bottom: var(--board-group-padding-y) !important;
}

.cm-line.atomdown-group-line.atomdown-group-last::after {
  border-bottom-width: var(--board-group-border-width);
  border-bottom-style: var(--board-group-border-style);
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
  /* The bar is the group box's top edge, so it takes the group's own stroke
     knobs: same width at every density, dotted at compact, and the density's
     resting colour. */
  border-width: var(--board-group-border-width);
  border-style: var(--board-group-border-style);
  border-color: var(--board-group-border-rest-color);
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
  padding: 3px var(--board-card-padding-x);
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
  border-bottom-width: var(--board-group-border-width);
  border-bottom-style: var(--board-group-border-style);
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
/* VERTICAL ONLY, and that is iugum-3ad requirement 2. Not one         */
/* horizontal distance below moves: --board-card-padding-x and         */
/* --board-group-padding-x are deliberately absent, so the distance    */
/* from a card's border to its first glyph, and from a group's border  */
/* to a member card's border, are IDENTICAL at both densities. The     */
/* `-y` halves are what compact takes. See "Compact compresses the     */
/* VERTICAL axis only" above.                                          */
/*                                                                     */
/* Every value here is derived from one of the panel's own named        */
/* properties, so overriding that property on `html` still moves both  */
/* views and both densities together.                                  */
/*                                                                     */
/* --board-group-border-width and --board-card-border-width are        */
/* deliberately absent, and that rule is NARROWED rather than dropped: */
/* a group's outline keeps its PRESENCE and its GEOMETRY at every      */
/* density - same width, same position, never absent - while its       */
/* STROKE STYLE and RESTING COLOUR may vary with the density. The      */
/* trade and the reason are written out in "How quiet works at         */
/* compact" above. The original rule's purpose survives: the outline    */
/* never disappears and never moves, so no density change can reflow    */
/* the page and the outline never stops saying "these cards are one    */
/* group".                                                             */
/* ================================================================== */

.cm-line.atomdown-compact-line,
.sb-decoration-widget.atomdown-compact {
  --board-card-radius: 4px;
  --board-card-padding-y: 6px;
  --board-card-gap: 6px;
  --board-group-padding-y: 4px;
  --board-group-header-padding: 1px 4px;

  /* --- THE COMPACT REGISTER: AN OUTLINE, NOT A SURFACE --------------
     Dotted strokes, no fill, and every colour here is the theme's own page
     background TOKEN rather than a value that happens to match today - so it
     is right in light, in dark, and under a third theme.

     The resting colours are what make the strokes invisible at rest while
     they still occupy their space. The pointer restores them; see the hover
     rules on the two boxes. Nothing about the geometry differs between the
     two states, so nothing can move. */
  --board-card-border-style: dotted;
  --board-group-border-style: dotted;
  --board-card-surface: var(--root-background-color, #fff);
  --board-group-surface: var(--root-background-color, #fff);
  --board-card-border-rest-color: var(--root-background-color, #fff);
  --board-group-border-rest-color: var(--root-background-color, #fff);
  /* A selected compact card is an ACCENT OUTLINE plus its ring, not a fill:
     compact has no surface, and adding one back for one state would be the
     one place the density stopped meaning what it means. Rest, hover and
     selected are still three different computed border colours. */
  --board-card-selected-surface: var(--root-background-color, #fff);
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
  /* --ad-frame, not --ad-inset: the compact header widget sets `border: none`
     below, so its padding box starts on the content column and the offset has
     to carry the group's stroke as well. With --ad-inset it landed 2px inside
     a member card's own border. */
  left: var(--ad-frame);
  right: var(--ad-frame);
  z-index: 6;
  margin: 0;
  /* VERTICAL ZERO, HORIZONTAL CONSTANT. The head's box is what the two gutter
     controls are offset from, so its horizontal padding has to be the same at
     both densities or the controls move with the density.
     `--board-card-header-padding` stays declared as the panel's own knob and
     is no longer read here, the way `--board-card-chrome-space` is not. */
  padding-top: 0;
  padding-bottom: 0;
  padding-left: var(--board-card-padding-x);
  padding-right: var(--board-card-padding-x);
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
/* Width and style only - the colour comes from the one place that sets it, so
   the top edge takes the resting, the hover and the selected colour with the
   other three sides instead of pinning its own. */
.cm-line.atomdown-compact-line.atomdown-card-first::before {
  border-top-width: var(--board-card-border-width);
  border-top-style: var(--board-card-border-style);
  border-top-left-radius: var(--board-card-radius);
  border-top-right-radius: var(--board-card-radius);
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
   and the group id fold into that menu. The outline around the group keeps
   its 2px and its position; the density block above changes its stroke style
   and its resting colour and nothing else. The bar keeps its resting tint at
   both densities, because it is the only thing left naming a group and a
   group whose resting outline is invisible still has to be findable. */
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

/* SELECTED, and it has to beat the hover rule, so it carries the card class
   too and comes after it. The card's TOP border at compact is drawn by a
   two-class rule above but takes its colour from here with the other three,
   because that rule sets width and style only. */
.cm-line.atomdown-card-line.atomdown-selected-line::before {
  background: var(--board-card-selected-surface);
  border-color: var(--board-accent-color);
  /* THE RING MOVED ONTO THE CARD BOX. It was an inset shadow on the LINE,
     which drew it at the content column's edge - outside a member card's own
     border and outside the group's. An inset shadow paints inside the padding
     box, and the line's padding box is inside the transparent inset now, so
     leaving it there would have drawn it through the text. On the ::before it
     is one pixel inside the card's own border, which is where a selection
     ring belongs. It is also what keeps SELECTED distinguishable from HOVERED
     at compact, where neither state has a surface to differ by. */
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
