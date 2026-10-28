# Pull requests in Beads viewer

Run `iugum beadview` and open **Pull requests** beside **Beads**. The existing Beads project startup requirement still applies. Both sections use the same server, normally `http://127.0.0.1:3849`.

## Filters

Use Organization, Repository, and Author dropdowns to select multiple values. Choices come from the loaded result page, selected values, and pinned shortcuts; they are not a complete GitHub inventory. Enter an exact name to add another choice. The star pins a choice as a quick toggle. Pins persist in this browser. Removing an active chip clears the same selection everywhere. Clear all resets to open PRs and preserves pins.

All filters opens a drawer with state, draft, sort, and order. Matching uses OR within each category and AND between categories. The backend requests GitHub advanced search explicitly. This first pass permits five OR operators across selections and limits retrieval to GitHub's first 1,000 matches. Narrow the search when results are incomplete. Text search is a literal phrase; use structured controls for qualifiers. Beads project filters never change GitHub scope.

Selections and pagination live in the URL. Back/Forward restores them. Each section remembers its own last view in this browser. Opening a PR title goes to GitHub. The loaded page fetches review decisions, checks, comments, assignees, and stack positions in one bounded batch. Detail sections load on demand.

## Credentials and access

The server resolves `GH_TOKEN`, then `GITHUB_TOKEN`, then `gh auth token --hostname github.com`. Run `gh auth login` if needed. Tokens stay in process memory and are never sent to the browser. After a 401, Refresh resolves credentials again. There is no GitHub write endpoint.

PR API routes require a loopback client and loopback Host, including when Beads itself binds a LAN address. Open the viewer through `localhost` or `127.0.0.1` to use PRs. Cross-origin requests are rejected. Local processes can read the API; this is a personal workstation interface, not authenticated shared hosting. Credentials are checked lazily so a GitHub problem does not prevent Beads startup.

## Agent API

All routes are GET-only, same-origin, and return JSON with `Cache-Control: no-store`.

```sh
# Equivalent to GitHub is:pr is:open author:srhopkins
curl 'http://127.0.0.1:3849/api/v1/prs?author=srhopkins&state=open'

# OR between repositories; AND with the selected state
curl 'http://127.0.0.1:3849/api/v1/prs?repo=golang/go&repo=cli/cli&state=open'

# Details (body and merged flag)
curl 'http://127.0.0.1:3849/api/v1/prs/cli/cli/1'

curl 'http://127.0.0.1:3849/api/v1/meta'
curl 'http://127.0.0.1:3849/api/v1/health'
```

List parameters: `q` (literal text), repeated `org`, `repo`, `author`, `state` (open/closed/all), `draft` (any/yes/no), `sort` (updated/created/comments), `order` (asc/desc), `page` (1-based), `per_page` (1–100, default 30). Unknown or invalid parameters return 400. Organization names denote GitHub organizations; use repository selection for repositories owned by individuals.

List responses contain `items`, `total_count`, `applied_filters`, `page`, `per_page`, `has_next`, `fetched_at`, `incomplete_results`, and `truncated`. Total count describes GitHub search matches, not proof of an exhaustive accessible-repository scan. Normalized items include repository, number, title, URL, author, state, draft, updated time, and `review`/`checks` set to `not_loaded`. Closed does not imply merged; use the detail route for the merged flag.

Errors use `{"error":{"code":"…","message":"…"}}`. Authentication returns 401, permission failures 403, inaccessible/missing details 404, GitHub query rejection 422, rate limits 429, and upstream failures 502. Rate-limit errors include `retry_after` and/or `rate_limit_reset` when GitHub supplies them. Health means the local route works, not that GitHub authentication has succeeded. Meta describes defaults and supported capabilities.

## Summaries and detail sections

`GET /api/v1/prs/summaries?pr=owner/repo/number&pr=owner/repo/number` accepts 1–30 distinct references. It returns `{"items":[...]}` in request order. Each item has `repository`, `number`, `review`, `checks`, `comments`, `assignees`, and `stack`. A fixed, read-only GraphQL query batches the entire page. Only this internal GitHub request uses POST; the local API remains GET-only.

`review` is GitHub's `reviewDecision`: `APPROVED`, `CHANGES_REQUESTED`, or `REVIEW_REQUIRED`. Null decisions become `NONE`. `checks` contains `total`, `passed`, `failed`, `pending`, and `skipped`. Counts come from the current head's status rollup, including check runs and legacy status contexts. Success passes; failure, error, timeout, cancellation, action required, and startup failure fail. Neutral and skipped count as skipped. Other states remain pending. No rollup means zero checks. `comments` is the conversation comment count from GitHub's `comments.totalCount`; inline code comments stay separate. `assignees` contains up to 100 logins. `stack` is null when GitHub reports no stack, or `{position,total}` from `stackEntry.position` and `stack.size`.

An inaccessible item has `error:"not_found_or_inaccessible"` and `review:"UNKNOWN"`. Its other values are placeholders, never evidence of no stack or successful checks. Scoped GraphQL errors preserve other items. Unscoped failures return a sanitized API error.

`GET /api/v1/prs/owner/repo/number/{reviews|comments|code-comments}?page=1` returns submitted review history, conversation comments, or inline code comments separately. Pages contain at most 100 items. `has_next` follows GitHub's next-page link.

`GET /api/v1/prs/owner/repo/number/checks` returns current-head checks. Continue with `?cursor=NEXT_CURSOR&head=HEAD_SHA`. The response includes `head`, `next_cursor`, and `has_next`. A changed head returns HTTP 409 with `head_changed`; discard earlier pages and reload. Optional `page` is validated but does not control check pagination.

Every section returns `{items,has_next}`. Items have `id`, `author`, `bot`, `body`, `state`, `name`, `url`, `created_at`, and `path`; inapplicable fields are empty. Check runs use their application slug as author. REST comment authors retain GitHub's bot identity. Checks include both check runs and legacy status contexts. No write or rerun control exists.

## Build and checks

Run `scripts/build-beadview.sh`, then `scripts/build.sh --cgo` (or `--static`). Use `ICU_PREFIX` if Homebrew's default formula is not the installed version. Install through `scripts/install.sh` when ready to replace the local binary.

Backend: `go test -race ./prview`. Browser: `node web/beadview/prs/browser.e2e.mjs`. The browser fixture uses synthetic API responses; it never reads personal tickets or credentials. Existing activity and board checks remain required for navigation changes.

The feature reuses the pinned viewer's React, TanStack, Radix, Tailwind, theme provider, and build. The Iugum handler wraps the existing viewer. No module-cache source is changed. No PR database, background polling, MCP server, or GitHub mutation is added.

References: [GitHub advanced search API](https://github.blog/changelog/2025-03-06-github-issues-projects-api-support-for-issues-advanced-search-and-more/), [search limits](https://docs.github.com/en/rest/search/search).
