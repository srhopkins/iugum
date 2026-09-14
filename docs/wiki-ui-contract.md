---
tags: [design, contract, iugum]
---

<!-- <atomdown version="1"/> -->

# Shared wiki UI contract

<!-- <atom id="C0000001" slug="authority" iugum-kind="decision"/> -->
Approved by Steve in the September 13, 2026 interview. These rules govern wiki, chat, search, menus, and drawers. They describe required behavior, not a claim that the current implementation meets it. The September 13 UI audit remains historical evidence. This contract supersedes its proposed rules where they overlap.

Canonical source: `docs/wiki-ui-contract.md` in iugum. The wiki copy is published at **Design/UI design contract**. Update both together. Beads owns implementation status: **iugum-phj — Apply shared UI contract and verify two theme families**. This document contains design decisions, not a task checklist.

## Typography

<!-- <atom id="C0000002" slug="typography" iugum-kind="decision"/> -->
Wiki prose and chat use one proportional reading font and the same base size. Menus and labels use the same family, with smaller sizes where appropriate. Code remains monospace. A monospace wiki-editor preference remains available. Font family and size belong to shared theme/preference values, not individual feature styles.

## Actions and menus

<!-- <atom id="C0000003" slug="actions" iugum-kind="decision"/> -->
Small settings, such as appearance, use one consistent native picker. Immediate actions, such as opening chat, happen directly. Search results keep their dropdown; agent browsing keeps its drawer. Actions are also available through the command palette where practical. Use the same action implementation from each entry point.

## Icons and buttons

<!-- <atom id="C0000004" slug="controls" iugum-kind="decision"/> -->
Toolbar icons share size, spacing, hover treatment, and a visible keyboard-focus outline. Icons have tooltips and accessible names. Selected controls show a persistent active state. Use text when an icon would be unclear. Hide unavailable features unless showing them explains a useful requirement; then disable them and explain why.

## Dismissal and focus

<!-- <atom id="C0000005" slug="dismissal" iugum-kind="decision"/> -->
Clicking an activating icon again closes its menu or drawer. Escape closes the topmost temporary menu or drawer. Clicking outside closes temporary menus and drawers while preserving drafts. Closing returns keyboard focus to the opening control. Opening another temporary menu closes the previous one. The main chat panel stays open until explicitly closed.

## Spacing and density

<!-- <atom id="C0000006" slug="density" iugum-kind="decision"/> -->
Use one shared spacing scale across chat, search, menus, and drawers. Comfortable is the default, with clear separation and easy click targets. Compact reduces padding while preserving readability and usable controls. Density changes spacing, not font size. Text size is a separate preference.

## Theme ownership

<!-- <atom id="C0000007" slug="themes" iugum-kind="decision"/> -->
Day, Night, and System apply consistently to every component. Shared theme values control backgrounds, text, borders, hover, selection, and focus. Color must accompany text or icons when communicating status or errors. Features must not invent their own colors.

Light-blue day prompts and dark-blue night prompts are defaults of the initial theme only. Every theme can override prompt background, text, and borders. Prompt surfaces must still support readable content and hide material behind pinned cards.

## Second theme as a design test

<!-- <atom id="C0000008" slug="second-theme" iugum-kind="decision"/> -->
Develop a warm neutral theme with cream/charcoal surfaces, muted amber accents, different prompt colors, and different border treatment. Provide Day and Night variants. Both theme families use the same components; no theme-specific feature code. System follows device appearance within the selected theme family.

Include this theme in regression checks to reveal hard-coded colors, borders, and other blocked overrides. Exact color values are not fixed by this interview; implementation must preserve readability. The second theme is required work, not an already available option.

## Loading, errors, and success

<!-- <atom id="C0000009" slug="feedback" iugum-kind="decision"/> -->
Show feedback immediately when an action starts. Keep unrelated controls usable. Preserve drafts and selections on failure. Show errors near the affected control with a clear recovery action. Do not show popups for routine success.

## Scrolling and motion

<!-- <atom id="C0000010" slug="scrolling" iugum-kind="decision"/> -->
New replies follow the bottom only when the reader is already there. When the reader scrolls up, preserve their reading position and show Jump to latest. Retain pinned prompts, click-to-expand, and the tick navigator. Resizing and expansion preserve reading position. Use subtle transitions and honor the system reduced-motion preference.

## Keyboard behavior

<!-- <atom id="C0000011" slug="keyboard" iugum-kind="decision"/> -->
Every control works without a mouse and shows visible focus. Tab follows visual order. Menus and pickers use standard arrow-key navigation. Text inputs keep normal editing shortcuts; the wiki must not intercept them. Enter sends chat; Shift+Enter inserts a line. Double-click actions, including rename, also have a discoverable keyboard or menu entry.

## Persistence

<!-- <atom id="C0000012" slug="persistence" iugum-kind="decision"/> -->
Theme, density, text size, and panel width persist for that browser. Chat titles belong to the conversation and persist on the server. Unsent drafts survive refresh and remain separate for each conversation. Switching agents, closing drawers, or navigating the wiki never discards a draft.

The current per-agent browser title and in-memory drafts do not satisfy this target. Do not treat an agent identifier as a permanent substitute for a conversation identifier.

## Reusable implementation

<!-- <atom id="C0000013" slug="implementation" iugum-kind="decision"/> -->
Reuse SilverBullet controls and extension APIs where practical. Define shared styles and components for custom UI. Keep static styling out of inline code. Calculated resizing and positioning values are allowed. Each add-on has a clean mount/disable lifecycle and removes its listeners when disposed. Record exceptions with their reason; one-off fixes must not silently become the pattern.

## Required regression checks

<!-- <atom id="C0000014" slug="regression" iugum-kind="decision"/> -->
Shared controls require keyboard, focus, and accessibility tests. Critical layouts require visual checks in Day and Night, at narrow and wide sizes, for both the initial and warm neutral theme families. Verify actual overrides, not merely a theme label change. Include persistence across refresh, navigation, and conversation switching.

Automated checks flag duplicate styles and static inline styling. Dynamic geometry must be distinguishable from static presentation. Visual baselines require review; agents must not replace them merely to make failing tests pass. These are required checks for UI changes, with documented exceptions.

Existing tests remain useful, but a green existing suite does not establish compliance with tests that are not implemented yet. Reports must distinguish checks passed, checks missing, and any documented exception. Tool selection such as axe integration is an implementation choice; the accepted requirement is the behavior and coverage above.

## Keeping the contract useful

<!-- <atom id="C0000015" slug="maintenance" iugum-kind="decision"/> -->
Future UI changes must reference the relevant rule and preserve the others. Record proposed departures explicitly for discussion. Keep historical audit findings intact; update implementation status in Beads rather than rewriting the audit to imply earlier compliance.
