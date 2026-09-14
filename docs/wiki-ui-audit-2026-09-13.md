---
tags: [design, audit, iugum]
---

<!-- <atomdown version="1"/> -->

# Wiki experience review — September 13, 2026

<!-- <atom id="B0000001" slug="audit-summary" iugum-kind="finding"/> -->
**The features are useful, but the UI implementation needs consolidation before more visual changes.** We reused SilverBullet panels, navigation, theme preferences, and Markdown libraries. We also accumulated custom CSS, menus, state handling, and direct DOM integration without a shared component contract.

This is an audit for discussion, not approval to refactor. Beads **iugum-c7a — Audit wiki UI additions and reusable interaction patterns** records this review. Existing product decisions remain in [[Design/Agents and wiki add-ons]].

## What “since Friday” can establish

<!-- <atom id="B0000002" slug="audit-evidence" iugum-kind="finding"/> -->
Friday means September 11, 2026, Arizona time. The newest commit is September 8: `f24b7158` (selection-layer test refinement). There are no Friday-to-Sunday commits to compare. At inspection, Git reports eight modified tracked files and 155 untracked entries, many of them directories. Ordinary `git diff` omits those new files.

The tracked diff contains 121 insertions and 10 deletions: CLI routing, add-on entry points, dependency changes, policy wiring, and wiki process cancellation. Most new UI and agent code is untracked. This review includes that source, the conversation requests, existing tests, and the supplied screenshots. It cannot prove the exact creation date of every uncommitted change. File modification dates would not repair that evidence gap.

The inventory below covers the visible wiki additions discussed since Friday, including earlier features revised during that period. It is not a claim that all backend capabilities were delivered during those three days. No fresh end-to-end backend certification was performed for this document.

## Feature inventory: chat

<!-- <atom id="B0000003" slug="chat-inventory" iugum-kind="finding"/> -->
| Feature | Expected experience | Current implementation and limits | Reuse and coverage |
|---|---|---|---|
| Native chat panel | Chat belongs inside the wiki; closing detaches | Native right panel containing a custom DOM interface; drafts survive panel close | Reuses `editor.showPanel`; mount/reopen tests |
| Generic agents | Any configured agent; no Chief-only controls | Agent drawer selects configured connections/homes; Status and Commitments buttons removed | Shared frontend/API; historical internal Chief names and detached controls remain |
| Model picker | Choose model independently from agent | Disabled select displays configured model only | Runtime switching is unfinished; tracked in iugum-zv4 |
| Header | Title, New, History, More, drawer toggle | Correct control order; New disabled | Custom header, hand-built SVGs; layout assertions |
| Conversation history | Return to earlier chats/topics | Clock lists turns from the current loaded conversation, not separate saved conversations | Reuses navigator entries; session support remains iugum-z2g |
| Rename title | Double-click, edit, save or cancel | Enter/blur save; Escape cancels; blank restores first-user-message title | Browser localStorage per agent, not a server-owned conversation title; partial tests |
| Export | Download conversation transcript | Markdown assembled from loaded history | Browser Blob download; filename tested, contents and completeness not tested |
| Agent settings | Inspect/change configuration | Read-only name/model and explanation; no editor | Reuses drawer shell; backend editing remains iugum-z2g |
| Agent drawer | Browse without leaving chat | Searchable overlay up to 85% panel width; selection changes agent | Custom aside; selection, draft isolation and dismissal tests |
| Text editing | Native cursor, deletion and multiline editing | Composer stops wiki keyboard interception; Enter sends, Shift+Enter adds newline | Browser keyboard regression and negative-control option |
| Immediate send | Prompt appears and composer clears while waiting | Optimistic prompt card; Working indicator; canonical reply replaces history | Delayed-response, duplicate and failure tests; not token streaming |
| Markdown | Readable lists, code, tables and disclosure details | Goldmark renders; Bluemonday sanitizes; client receives safe HTML | Good shared backend reuse; separate formatting test omitted from required runner |
| Pinned prompts | Latest relevant prompt stays above its replies | Sticky prompt per turn; next turn replaces it; opaque backing masks corners | Custom CSS/geometry; pin/resize/expansion tests |
| Long prompt | Fade at bottom, click to expand | Six-em collapsed height, mask fade; click/keyboard expansion | Custom overflow detection; no visible Expand button |
| Reply fade | Text disappears behind prompt | Opaque backing plus a 24px fade below prompt | Theme-dependent CSS; structural tests do not prove every corner pixel |
| Conversation ticks | Smooth hover, preview and jump | Uniform resting widths; nearest tick owns hover through gaps; right anchored growth; centered preview | Custom geometry/state; width, hover and position tests |
| Panel resize | Drag edge without harming wiki | Left-edge pointer handle with keyboard resize | CSS variable controls width; responsive lower bounds; browser test |
| Prompt colors | Theme-owned day/night colors | Light blue default, dark blue override; opaque pinned surface | Shared CSS variables; theme override test |

## Feature inventory: search and wiki controls

<!-- <atom id="B0000004" slug="search-inventory" iugum-kind="finding"/> -->
| Feature | Expected experience | Current implementation and limits | Reuse and coverage |
|---|---|---|---|
| Search placement | Centered in existing top bar | Direct attachment into native nav; absolute centered width | Custom DOM/CSS, not a third-party search plug; geometry test |
| Search expansion | Same width; expand downward | All, Wiki, Chats, Filters row under input; divider before results | Custom state and CSS; scope/layout tests |
| Scope filters | Narrow sources without clutter | Tabs set scopes; Filters exposes source/project text; minus words exclude | Reuses backend search; embeddings not implemented |
| Search results | Distinct readable items with useful actions | Cards, sanitized Markdown, source metadata, five-item paging | Same backend Markdown renderer as chat; separate CSS rules |
| Result navigation | Click reaches relevant content | Wiki titles navigate; transcript titles expand indexed passage/provenance | Not native platform session navigation; tested wiki route/passages |
| Search feedback | Busy state, accessible icon, reopen last results | Icon/title, disabled busy button; outside/Escape close; focus restores cached query | Custom popup lifecycle; browser regression |
| Search/chat independence | Enable either add-on | Independent configuration and endpoint gates, shared frontend module | Standalone mode tests; startup race has occurred intermittently |
| Body width | Cycle three widths | Comfort 900px, Wide 1280px, Full capped at 1600px/96% | Standard Space Lua action/command + native CSS variable; reload test |
| Appearance | Day, Night, System | Native filterBox picker, saved native preference, UI reload | Good native reuse; live System and reload tests |
| Existing Atomdown/density controls | One coherent top menu | Existing controls remain alongside new custom controls | Not newly audited as Friday features; no shared interaction contract across all controls |
| Wiki documents | Decisions and commitments stay readable/editable | Ordinary Markdown/Atomdown wiki pages | Strong file ownership; docs now contain stale UI descriptions |

## Why the experience feels inconsistent

<!-- <atom id="B0000005" slug="typography" iugum-kind="finding"/> -->
**Fonts:** SilverBullet intentionally has separate UI and editor fonts. Native styles use `--ui-font` and `--editor-font`; the default editor font is iA-Mono/Menlo. Chat inherits the UI font but hard-codes Markdown to 14px. Search separately hard-codes 14px. The screenshot's difference is therefore not just one incorrect font: it mixes typography roles, sizes, and density.

Different editor and interface fonts can be reasonable. We have not agreed whether wiki prose and chat prose should share a reading font. That should be a deliberate setting, with code remaining monospace, rather than an accidental consequence of which container rendered the content.

**Menus:** Appearance uses native `editor.filterBox`, which resembles the command palette. Width uses a direct cycle plus native notification. History and More use a custom popup. Agents uses a custom drawer. Search uses a custom dropdown. These differences can serve different jobs, but there is no written rule defining when to use each.

## Implementation findings

<!-- <atom id="B0000006" slug="implementation-findings" iugum-kind="finding"/> -->
1. **CSS has accumulated overrides.** `plugs/chief/chief.js:76–77` stores panel and host styling in two long strings. Search radius changes from 9 to 22 to 18 and back to 9px through later rules. Header margin rules conflict and use `!important`. This makes the final appearance depend on edit order.
2. **Static inline styling exists.** The title editor uses `input.style.cssText` at line 526. That belongs in a reusable class. By contrast, calculated panel width, tick width, and preview position are legitimate dynamic values; those should remain explicit inputs to a component, not be banned indiscriminately.
3. **There is no reusable UI component layer.** One mount function owns composer, history, navigation ticks, drawer, search, menus, model display, API state, and attachment. Shared `node`/`icon` helpers are useful but do not define common behavior or accessibility.
4. **The add-ons are configurable separately but share implementation.** That is not inherently wrong. The shared module, direct native selectors, and whole-body MutationObserver make changes harder to isolate. There is no clear teardown contract for all listeners and observers. The retained singleton prevents ordinary duplicate mounting; this is a maintenance risk, not proof of a current navigation memory leak.
5. **Menu semantics and focus behavior are incomplete.** Click/outside/Escape handlers exist, but there is no shared focus-return, focus-containment, arrow-navigation, or expanded-state contract. A button styled like other buttons does not supply that behavior.
6. **Presentation and runtime capability are out of step.** Model selection, New chat, and settings are unfinished. Clock means current-turn history, not a conversation list. These distinctions must remain visible rather than being obscured by familiar icons.
7. **State ownership is mixed.** Messages are server-owned, drafts live in memory, titles use browser localStorage per agent, theme/width use native clientStore. Those choices are not yet documented as a common persistence policy.
8. **Documentation has drifted.** `plugs/chief/README.md` still mentions a search row below the bar and Expand/Pin buttons. `docs/wiki-addons.md` describes an Agents link to a full page. Neither matches the current drawer/header.

## Regression coverage: what passing means

<!-- <atom id="B0000007" slug="test-contract" iugum-kind="finding"/> -->
The latest implementation turn ran the full `scripts/chief-fe-check.sh` successfully. This audit inspected that coverage; it did not rerun the suite. Earlier standalone startup timeouts remain recorded in **iugum-s4f — startup reliability follow-up**. One successful run does not resolve that intermittent behavior.

The gate covers useful interactions: native editing, search, pinning, resizing, width, appearance, agent switching, pending sends, and isolated add-on modes. However, `formatting.e2e.mjs` and the broader `chief.e2e.mjs` are outside the required runner. No shared typography, spacing, focus, or cross-component visual contract is enforced.

Specific gaps: title tests assert save/cancel and stored value, not actual reload/blur/per-agent isolation. Export tests assert the download filename, not text fidelity. Geometry tests cannot guarantee that rounded pinned corners hide every pixel. The older `chief.e2e.mjs` still expects the now-hidden Close chat button, so merely adding it to the runner would require updating its intended behavior. Search scope-to-query correctness, keyboard result navigation, interrupted requests, many-turn tick overflow, and drawer focus return need stronger coverage. There are no chat/search screenshot baselines across representative widths and both themes. Existing Atomdown visual tests do not cover this new chat shell.

A green behavior suite therefore means those scenarios passed. It does not mean the interface is visually cohesive, every control has full semantics, or the whole MVP is complete.

## Proposed rules for discussion — not adopted yet

<!-- <atom id="B0000008" slug="proposed-ui-rules" iugum-kind="proposal"/> -->
**Recommendation: keep native commands as the common action system, while choosing presentation by the job.** Do not force search results or a persistent agent browser into a command picker solely to reduce custom code.

| Job | Proposed presentation |
|---|---|
| Find/run an action | Native command palette; common action also available through its icon |
| Small setting selection | One consistent picker; native filterBox by default unless we deliberately choose a shared anchored menu |
| Immediate reversible action | Direct action/toggle with tooltip and visible state |
| Search information | Search dropdown with query, scopes and results |
| Browse/manage persistent entities | Drawer, with predictable dismissal and focus return |
| Read conversation | Chat panel; shared typography and Markdown tokens |

Use one set of tokens for interface font, reading font, code font, sizes, line heights, spacing, borders, surfaces, focus rings and icon dimensions. Both shadow-panel and host styles consume those tokens. Keep prompt colors themeable. Permit dynamic geometry through CSS variables; put static styling in named stylesheets/classes.

Use small shared components for icon buttons, popup dismissal, picker rows, drawers and Markdown surfaces. Give each mount a cleanup function. Prefer the existing SilverBullet extension API. When a stable native slot is missing, use a narrow documented seam rather than spreading native DOM selectors through every feature.

Use accessible names, keyboard behavior, focus return and explicit unavailable states as component requirements. Define what persists, where it lives, and whether it follows the user across browsers. Do not silently substitute an agent ID for a conversation ID.

## How we could enforce the agreed rules

<!-- <atom id="B0000009" slug="proposed-enforcement" iugum-kind="proposal"/> -->
After discussion, turn approved rules into a component contract and a small regression matrix. Include day/night, narrow/wide panel, keyboard-only use, long content, empty state, pending state and error state.

Add static checks for forbidden static style assignments and duplicated selectors, with explicit exceptions for calculated geometry. Include all relevant existing tests in the required gate. Add visual baselines for the shell and critical pinned/overlay regions, plus focus/keyboard assertions and export/persistence round trips. Keep tests focused on user outcomes rather than copying CSS values as proof of correctness.

A review should answer: Did we reuse the agreed component? Does this change fit an existing token? Is there a new behavior to document? Does the test fail if that behavior breaks? New patterns should be a conscious decision with a recorded reason.

## Discussion starting point

<!-- <atom id="B0000010" slug="discussion-start" iugum-kind="proposal"/> -->
My proposed first decision is typography: **use one proportional reading font for chat and wiki prose, while retaining monospace for code and an optional editor-font preference.** If Steve prefers a monospace wiki editor, retain it deliberately and align the surrounding UI font, scale and spacing.

Then decide whether small settings should always use the native picker. After those choices, we can consolidate the existing implementation in place. This review does not authorize a redesign, framework replacement, or broad backend changes.

## Source guide

<!-- <atom id="B0000011" slug="source-guide" iugum-kind="evidence"/> -->
Paths refer to the current iugum working tree, not a committed release:

- `plugs/chief/chief.js`: styles 76–77; panel/resize 78–220; turn rendering 220 onward; submission near 410; header/rename near 500; search and mount near 590–770.
- `plugs/chief/chief.plug.js`: native panel bootstrap and command registration.
- `agentdesk/markdown.go`: shared Markdown rendering and sanitization.
- `plugs/theme-menu/ThemeMenu.md`: native appearance picker and night prompt tokens.
- `plugs/editor-width/EditorWidth.md`: native action/command and width preference.
- `scripts/chief-fe-check.sh`: required disposable-wiki test entry point.
- `plugs/chief/*.e2e.mjs`: behavioral coverage and additional unintegrated checks.
- `silverbullet/client/styles/theme.scss:163–167`: native UI/editor font distinction.
- `docs/wiki-addons.md`, `docs/chat-agents.md`, and the existing design wiki page: product boundaries and known delivery gaps.

No application code was changed for this audit. No commit or push was made.


## Approved contract

The subsequent interview produced [[Design/UI design contract]]. Its approved rules supersede proposals in this historical audit. See `docs/wiki-ui-contract.md` for the canonical repository source.
