# Pull requests in Beads viewer

Run `iugum beadview` and open **Pull requests** beside **Beads**. The existing Beads project startup requirement still applies. Both sections use the same server, normally `http://127.0.0.1:3849`.

## Filters

Use Organization, Repository, and Author dropdowns to select multiple values. Choices come from the loaded result page, selected values, and pinned shortcuts; they are not a complete GitHub inventory. Enter an exact name to add another choice. The star pins a choice as a quick toggle. Pins persist in this browser. Removing an active chip clears the same selection everywhere. Clear all resets to open PRs and preserves pins.

All filters opens a drawer with state, draft, sort, and order. Matching uses OR within each category and AND between categories. The backend requests GitHub advanced search explicitly. This first pass permits five OR operators across selections and limits retrieval to GitHub's first 1,000 matches. Narrow the search when results are incomplete. Text search is a literal phrase; use structured controls for qualifiers. Beads project filters never change GitHub scope.

Selections and pagination live in the URL. Back/Forward restores them. Each section remembers its own last view in this browser. Opening a PR title goes to GitHub. Review decisions and checks are explicitly not loaded; this version does not infer them from search records.

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

# Details (body and merged flag; review/check enrichment is deferred)
curl 'http://127.0.0.1:3849/api/v1/prs/cli/cli/1'

curl 'http://127.0.0.1:3849/api/v1/meta'
curl 'http://127.0.0.1:3849/api/v1/health'
```

List parameters: `q` (literal text), repeated `org`, `repo`, `author`, `state` (open/closed/all), `draft` (any/yes/no), `sort` (updated/created/comments), `order` (asc/desc), `page` (1-based), `per_page` (1–100, default 30). Unknown or invalid parameters return 400. Organization names denote GitHub organizations; use repository selection for repositories owned by individuals.

List responses contain `items`, `total_count`, `applied_filters`, `page`, `per_page`, `has_next`, `fetched_at`, `incomplete_results`, and `truncated`. Total count describes GitHub search matches, not proof of an exhaustive accessible-repository scan. Normalized items include repository, number, title, URL, author, state, draft, updated time, and `review`/`checks` set to `not_loaded`. Closed does not imply merged; use the detail route for the merged flag.

Errors use `{"error":{"code":"…","message":"…"}}`. Authentication returns 401, permission failures 403, inaccessible/missing details 404, GitHub query rejection 422, rate limits 429, and upstream failures 502. Rate-limit errors include `retry_after` and/or `rate_limit_reset` when GitHub supplies them. Health means the local route works, not that GitHub authentication has succeeded. Meta describes defaults and supported capabilities.

## Build and checks

Run `scripts/build-beadview.sh`, then `scripts/build.sh --cgo` (or `--static`). Use `ICU_PREFIX` if Homebrew's default formula is not the installed version. Install through `scripts/install.sh` when ready to replace the local binary.

Backend: `go test -race ./prview`. Browser: `node web/beadview/prs/browser.e2e.mjs`. The browser fixture uses synthetic API responses; it never reads personal tickets or credentials. Existing activity and board checks remain required for navigation changes.

The feature reuses the pinned viewer's React, TanStack, Radix, Tailwind, theme provider, and build. The Iugum handler wraps the existing viewer. No module-cache source is changed. No PR database, background polling, MCP server, or GitHub mutation is added.

References: [GitHub advanced search API](https://github.blog/changelog/2025-03-06-github-issues-projects-api-support-for-issues-advanced-search-and-more/), [search limits](https://docs.github.com/en/rest/search/search).
