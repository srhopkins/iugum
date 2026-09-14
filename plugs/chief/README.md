# Chief inside SilverBullet

This opt-in extension makes the wiki itself the agent interface. Chief appears in SilverBullet's native right-hand panel. A compact search row sits below the page-title bar. Commitments remain a Markdown page named `Commitments`.

`iugum agent run --config agent.yaml` installs the bundled `chief.plug.js` into that agent's `wiki_dir/_plug`. It does not install the extension into other wiki spaces. Existing user-edited bundle bytes are backed up before an upgrade.

The plug bootstraps `chief.js` through SilverBullet's Space Lua JavaScript import API. The module supplies a DOM node to `editor.showPanel`, which renders it in the native shadow-root panel. A small DOM attachment supplies the search row. No SilverBullet core or editor-decoration seam modification is required.

The native agent's listener serves the wiki directly through a reverse proxy. SilverBullet owns the page, editor, navigation, assets, file API, and WebSocket traffic. Chief uses the reserved `/.proxy/iugum/api/` namespace and `/.proxy/iugum/assets/chief.js`. The `/.proxy/` prefix is SilverBullet’s existing network-only surface, so its offline service worker never mistakes API responses for cached wiki pages. Existing backend policy and same-origin checks remain active. Open the agent listener, rather than its internal wiki port, to use the integrated interface.

Chat history and commitments stay in their existing stores. Closing the panel preserves the draft; the Chat button and `Chat: Open` command reopen it. Navigation does not create a new conversation. Enter sends; Shift+Enter inserts a line break.

Search accepts sources, platform names, or a project scope. Prefix unwanted words with a minus sign. Results show five passages at a time with source details. Wiki results navigate directly to their pages.

## Verify

Run `node --test plugs/chief/chief.test.mjs` for interaction helpers. Go tests cover the proxy, API namespace, and cross-origin rejection. For a separately running synthetic instance with a wiki containing “Native wiki fixture” and a Commitments page:

```sh
CHIEF_UI_TEST_URL=http://127.0.0.1:3852 node plugs/chief/chief.e2e.mjs
```

The browser test uses the repository's existing Playwright installation. It checks native containment, navigation, search, close/reopen, and persistence without model calls.

Chat messages use server-rendered Goldmark Markdown with Bluemonday sanitization. The browser inserts only this sanitized response HTML. Stored Markdown remains unchanged. Status excerpts, provenance, and index freshness use native disclosure elements. Refresh preserves disclosure state and scroll position while the reader is above the bottom. New transcript excerpts retain whitespace; old flattened excerpts cannot recover their lost line breaks.

Keyboard regression gate: run `scripts/chief-fe-check.sh` from the repository.
It builds current code and creates/removes a disposable wiki automatically.
Coverage: ArrowLeft, Backspace, forward Delete, selection deletion, select-all,
Shift+Enter, and unchanged wiki content. Run with `--prove-regression` to confirm
that disabling the fix reproduces the caret failure. This is a local gate;
it is not currently wired into GitHub Actions.

Search uses iugum's multi-source backend through this native SilverBullet plug.
Search results reuse the sanitized Markdown renderer used by chat. Wiki titles
navigate to pages; other titles expand the indexed passage and provenance (not
an entire native transcript). The Search control exposes busy state and tooltips.
Chief closes search results, opens the panel, and focuses its composer.
The local front-end gate also runs `search.e2e.mjs`; controlled search fixtures
block service workers so Playwright can intercept only the fixture responses.

Prompts stay pinned within their own turn while its replies scroll, then yield
to the next turn. Long prompts are bounded to four lines; Expand prompt makes
the prompt scroll normally and Pin prompt restores pinning. Expansion survives
message refresh. This reproduces Murfy mobile behavior without importing its code.
The search button uses an inline magnifying-glass SVG with an accessible name.
The front-end gate includes sticky turn geometry, expansion, and icon checks.

Search lives in the center of the existing native top navbar. Its dropdown shares
its width and uses separated result cards. Outside pointer clicks and Escape close
it; refocusing an unchanged completed query reopens cached results without fetching.
Changing the query or scope hides stale results. The browser gate checks placement,
attachment geometry, cards, dismissal/reopening, navigation, and sticky prompts.

Configured agents and ACP backends are documented in [Reusable wiki chat](../../docs/chat-agents.md). Agent names appear in the selector and message attribution; controls use generic labels.
