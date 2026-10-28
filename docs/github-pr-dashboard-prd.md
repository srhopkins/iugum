# Product Requirements Document: Pull Requests in Iugum Beadview

**Status:** First pass implemented; usage and remaining limits are in [github-pr-api.md](github-pr-api.md)
**Version:** 1.0 (MVP)
**Product:** Iugum Beads viewer — Pull requests section
**Primary audience:** Individual developer, with read-only access for local AI agents

## 1. Objective

Add a read-only GitHub pull-request section to `iugum beadview`. It shares the viewer server, navigation, theme, and embedded assets. There is no separate application, executable, port, or deployment.

The user can search across repositories, toggle common filters quickly, and bookmark a view. Agents can query the same data through a read-only JSON API.

## 2. Existing architecture and integration

Verified against the current checkout and pinned Beads fork:

- `adapter/tracker/beadsadapt/web.go` composes the viewer HTTP handler and defaults to loopback port 3849. Add the PR handlers here through composition, without changing ticket endpoints.
- The fork already supplies React, TypeScript, Vite, Tailwind, Radix controls, TanStack Table, and TanStack Query. Reuse its components, dependencies, theme, and query conventions.
- `web/beadview/prepare.mjs` assembles Iugum extensions in a disposable copy of the pinned viewer. `scripts/build-beadview.sh` builds embedded assets.
- Keep GitHub-specific services in Iugum. If a reusable navigation seam requires changes to the Beads fork, make them in its own repository and update the pin. Never edit the module cache.
- Use Go HTTP calls for GitHub access. Resolve credentials through environment overrides or the existing GitHub CLI login.
- Preserve the viewer's existing ticket writes and authorization. Read-only describes the new GitHub feature, not the entire viewer.

### Recommended navigation

Use visible primary links **Beads | Pull requests**, following GitHub's Issues/Pull requests separation. Within Beads, retain the existing Tree, Activity, Board, Graph, and List views. Pull requests represents another data source, not another layout of Beads tickets.

Keep both primary destinations visible at narrow widths. Do not put the only PR entry inside a menu. Preserve existing ticket links and default startup behavior. Each section retains its own filters and view state; browser Back/Forward restores them. Use URL state for shareable selections. Proposed URL: `?section=prs`; absence of `section` retains current Beads behavior. Final parameter names must be checked against existing URL parsing.

Reference: [GitHub issue and pull-request navigation](https://docs.github.com/en/issues/tracking-your-work-with-issues/using-issues/filtering-and-searching-issues-and-pull-requests).

### Recommended filter interaction

Use three complementary controls above the table:

1. **Quick toggles:** persistent chips for pinned organizations and repositories. Clicking toggles selection; clicking again clears it. Unselected chips remain visible. Include an accessible pressed state, not color alone.
2. **Searchable dropdowns:** Organization, Repository, and Author open multi-select lists with checkboxes. They serve the full available choices without filling the toolbar with chips.
3. **Visible state selector:** keep Open, Closed, and All states beside the active chips so removing a state chip is reversible without hunting through the drawer.
4. **All filters:** open a dedicated filter drawer beside the table for state, draft, and less-used controls. Keep results visible. A separate full-page filter screen is not recommended for the first version.

Show removable active-filter chips separately from pinned quick toggles. Removing an active chip clears the matching dropdown selection and quick toggle. Clear all resets selections to open PRs; it does not remove pinned shortcuts. Store shortcut preferences per browser; put applied filters in the URL. No database is needed.

Use OR within a filter category and AND between categories only where the backend can produce equivalent, complete search semantics. Reject unsupported combinations explicitly. Never offer a multi-select combination that silently narrows or drops results. Reset pagination after filter changes, debounce text entry, and discard stale responses. Filter controls remain usable while loading. Label displayed stale data during refresh.

Organization and repository suggestions must have explicit scope and provenance. Search-result values alone are not a complete discovery catalog. Support entering an exact organization/repository/author when suggestions are incomplete. The Beads project selector does not silently limit GitHub scope. Automatic local-project-to-repository mapping is deferred.

Do not add OpenStatus or another table framework. TanStack Table supplies table mechanics; the application still owns filter controls and GitHub query semantics.

## 3. In-scope features

### 3.1 Unified PR list

- Default to **open pull requests** accessible to the authenticated user, across repositories and organizations.
- Display columns: organization/repository, PR number, title (link to GitHub), author, state/draft indicator, review summary where available, CI summary where available, updated time.
- Provide sorting by updated time (newest first by default) and other supported columns.
- Show loading, empty, authentication-error, rate-limit, and API-error states.
- Provide explicit Refresh action and last-refreshed timestamp.
- Include server-driven pagination; do not silently suggest that the first page represents all results.

### 3.2 Quick filters

- Controls near the top for **Organization**, **Repository**, **Author**, **PR state** (open/closed/all, default open), **Draft** (any/yes/no), and text query.
- Organization, repository, and author filters should support multi-selection when GitHub search syntax/API capabilities permit it; otherwise compose an equivalent bounded set of requests and merge/deduplicate safely. Document unsupported combinations explicitly rather than showing incomplete results.
- Display every active filter as a removable chip; include **Clear all** and an obvious default-state reset.
- Keep URL query parameters synchronized with the active filters, enabling bookmarkable views and browser back/forward navigation.
- Apply filtering on the server through GitHub search queries when result sets exceed the loaded page. Avoid filtering only a partial page client-side and presenting it as a global match.
- Optional filters for review/CI may be added after core filtering works, with accurate labels and data semantics.

### 3.3 Agent-facing API

- Provide JSON endpoints under `/api/v1/`; use the **same search service** for UI and agent requests.
- Minimum endpoints:
  - `GET /api/v1/prs`: list/search PRs. Query parameters include `q`, `org`, `repo`, `author`, `state`, `draft`, `sort`, `page`, and `per_page` as applicable.
  - `GET /api/v1/prs/{owner}/{repo}/{number}`: fetch full PR details using the pull-request endpoint, including reliable review/check details when requested/implemented.
  - `GET /api/v1/health`: health and basic readiness without secrets.
  - `GET /api/v1/meta`: supported filters, defaults, and capabilities so an agent can discover the API.
- JSON list responses must include `items`, `total_count` when available, page metadata, applied filters, `fetched_at`, and `incomplete_results`/truncation signals where applicable.
- Return standard HTTP status codes and structured error objects. Never print credentials or return them in API responses.
- Supply a short `API.md` with copy/paste `curl` examples (including equivalents of `is:pr is:open author:srhopkins`).
- REST is the MVP agent interface. **Do not build an MCP server for v1**; an MCP adapter can be added later if useful. Any MCP/developer documentation published by UI libraries is unrelated to this application's agent data access.

## 4. GitHub data and search behavior

- Use `GET https://api.github.com/search/issues?q=<encoded query>` with `is:pr` and appropriate qualifiers. For example, `is:pr is:open author:srhopkins`.
- Search results are **issue-shaped** records representing PRs; normalize the response for the UI and API. Fetch pull-specific fields via appropriate PR/review/check endpoints if required. **Do not assume search results contain approval decisions or complete CI details.**
- Use authenticated requests with `Accept: application/vnd.github+json` and a supported `X-GitHub-Api-Version` header.
- Respect `per_page <= 100`, search's effective 1,000-result retrieval ceiling, rate-limit headers, and secondary rate limits. Expose truncated/incomplete states clearly.
- If needed for larger result sets, implement deliberate query partitioning (for example by org/repo/date), while accounting for duplicates and maintaining predictable sorting. This is an enhancement, not a requirement for MVP.
- Never claim "all PRs" if permissions, search limits, or indexing prevent exhaustive results.
- Private organization repositories should appear only when the user's token has permission; surface actionable but non-sensitive permission errors.
- Avoid API-heavy per-row detail requests: batch or lazily fetch detail/CI/review data, cache briefly, and keep requests bounded.

## 5. Authentication and security

- Run **on loopback only** (`127.0.0.1`) by default; bind to another address only through an explicit configuration option, with appropriate authentication required before remote exposure.
- At startup resolve credentials in this order: explicit `GH_TOKEN`, `GITHUB_TOKEN`, then `gh auth token` (captured by Go as stdout, never printed).
- `gh` is an optional runtime dependency for credential retrieval; **the application itself uses direct GitHub API HTTP calls**, not shelling out for each PR query.
- Cache the token in process memory. On authorization failure, optionally re-resolve credentials once and retry safely; distinguish authorization errors from missing permissions and rate limits.
- Do not persist tokens to files, local storage, browser state, logs, or analytics. Browser uses same-origin `/api/v1` endpoints only and never receives the token.
- Do not log authorization headers or full sensitive API responses. No telemetry by default.
- Read-only operations only. No ability to merge, approve, comment, close, or modify PRs in MVP.
- Note that a loopback API can still expose private repository metadata to other local processes; use restrictive bind defaults and consider optional local API protection for shared hosts.

## 6. UI behavior

- Pull requests section within the existing viewer, desktop-first and usable on narrower screens.
- Header: app title, Refresh, fetch status, and last-updated time.
- Filter toolbar with compact multi-select popovers. Active selections appear as removable chips directly below or within the toolbar.
- Main table with clickable PR title and repo, compact status indicators, sortable headers, and pagination controls.
- Keep visual design restrained, prioritize information density and clarity over dashboard chrome.
- Use accessible labels, keyboard navigation, and visible focus states.
- No Kanban board or swimlanes (explicitly out of scope).

## 7. Build and packaging

Reuse `scripts/build-beadview.sh`, then `scripts/build.sh --cgo` and the existing Iugum installation workflow. Preserve static compilation with `scripts/build.sh --static`. No new executable or Node runtime is required. Development uses the viewer's existing Vite workflow with same-origin API proxying as needed.

The current viewer initializes a Beads project before serving. This feature does not introduce a standalone PR-only startup mode. GitHub authentication or API failure must leave Beads functional.

## 8. Integration boundaries

Proposed Go packages separate GitHub authentication/client code from normalized PR search and HTTP handlers. Exact names are implementation choices. Add UI components under the existing Iugum beadview extension and compose API handlers ahead of the fallback viewer handler. Audit `/api/v1/` routes for collisions before implementation. Keep the original minimum API contracts unless a collision requires documented namespacing.

Read and follow `docs/wiki-ui-contract.md` for shared interaction rules. Beadview retains its existing Light/Dark/System themes; the wiki-only warm-neutral theme is outside this feature, consistent with `docs/beadview-activity.md`.

## 9. Acceptance criteria

1. The existing viewer build and Iugum build produce one executable containing both Beads and PR assets.
2. `iugum beadview` serves both sections on its existing localhost address with no Node process required.
3. With an existing `gh auth login`, the app loads accessible open PRs without another login prompt.
4. Supplying `GH_TOKEN` works even if `gh` is not installed.
5. The user can select multiple organizations/repositories where supported, remove individual filter chips, clear all filters, and bookmark the resulting URL.
6. UI and REST API show consistent normalized PR results for the same filters.
7. An agent can use documented `curl` requests to list/search PRs and retrieve individual PR details without scraping the UI.
8. Pagination, rate limiting, auth errors, and incomplete/1,000-result-limited searches are clearly indicated.
9. GitHub tokens never appear in browser payloads, API output, application logs, or persisted application files.
10. CI/review labels are not invented from fields absent in GitHub search results.
11. Viewer documentation explains GitHub requirements, development, builds, authentication, and security implications.
12. Primary navigation preserves all existing ticket links, views, filters, and authorized write behavior.
13. Pinned quick toggles, dropdown selections, and removable active chips stay synchronized through refresh and browser Back/Forward.
14. GitHub failures do not disable Beads; Beads project filters do not silently narrow PR results.
15. Keyboard, focus, narrow/wide layouts, and Light/Dark/System behavior pass isolated browser checks. Existing visual baselines are preserved.
16. UI and API expose the same supported filter combinations and incomplete-result semantics.

## 10. Out of scope for MVP

- Kanban/swimlanes.
- Write actions on GitHub, notifications, webhooks, background synchronization.
- Database or persistent cache.
- Full OAuth login flow or account management UI.
- MCP server or separate agent process.
- OpenStatus Data Table Filters unless implementation shows a clear need.
- Organization-wide administrative discovery beyond what authenticated GitHub Search returns.

## 11. Implementation order

1. Go GitHub client, credentials, normalized PR model, and REST endpoints.
2. Shared navigation and PR table in the existing viewer, with pagination and refresh.
3. Pinned quick toggles, searchable multi-select dropdowns, All filters drawer, active chips, and URL synchronization.
4. Auth/error/rate-limit handling, optional memory cache, detail endpoint enrichment.
5. Existing embedded build integration, documentation, and regression tests.

## 12. Validation and scope

This document specifies a feature; it does not itself authorize implementation or deployment. Steve authorized the first pass with primary sections and the filter drawer. Steve subsequently authorized clickable review, check, comment, assignee, and stack details. Complete repository discovery remains deferred.

Implementation validation must cover query construction, normalized records, credential redaction, stale-request handling, unsupported combinations, pagination limits, and error isolation. Use isolated fixtures without real tokens for automated tests. Smoke-test the embedded viewer and existing Beads routes. Obtain independent review of the implemented code before closing implementation work.

Source: Steve's supplied `github-pr-dashboard-prd.md`. The standalone packaging, project layout, and new-application instructions are superseded by this integration design. GitHub API limits and supported query combinations require verification against official documentation during implementation.

## Read-only row details

Reviews, Checks, Comments, Assignees, and Stacked PRs have labeled columns and clickable controls with hover descriptions. Each opens a scrollable dialog with Escape dismissal and focus returned to its trigger. Batched summaries avoid per-row API requests; detailed history loads only when opened.

Checks show passing/total plus a state indicator, with failing, pending, and skipped counts explained inside. Pages stay tied to the same head revision. Comments default to conversation comments, label bots, and provide a separate code-comments tab. Review history does not override GitHub's current review decision. Comment bodies render GitHub-flavored Markdown and sanitized HTML, including tables and expandable details. Scripts, event handlers, unsafe URLs and inline styles are removed.

Empty assignees read **Unassigned**. Empty registered stack membership reads **Not in stack**, with an explanation that unregistered dependencies can still exist. API errors display **Unavailable**, never a successful or empty state.

Review and check states use icons plus text: green approval/success, amber waiting/review required, and red changes requested/failure. Comments, assignees, and stack use neutral icons. Shared semantic status CSS variables provide light/dark colors; this extends the viewer theme because it has no success/warning tokens.
