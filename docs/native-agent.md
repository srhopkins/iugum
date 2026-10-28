# Native agent workspace

Run a configured agent as one iugum process:

```sh
iugum agent run --config /path/to/agent/agent.yaml
```

The command runs in the foreground. A supervisor such as launchd owns restart behavior. Existing `wiki`, `beads`, `observe`, `job`, and container-agent commands retain their separate uses.

The wiki is the main application. A bundled plug places continuous chat in SilverBullet’s native right-hand panel and a scoped search row below its title bar. Commitments live in a wiki page. No outer wiki iframe or SilverBullet core patch is used; see `plugs/agentchat/README.md`.

## Configuration

Copy `examples/agent` into a private agent home. Keep runtime data and credentials out of public repositories. Paths resolve relative to the agent configuration file.

- `name`: agent identity; policy subject is `agent:<name>`.
- `runtime: native`: bounded Go model/tool orchestration.
- `ptt_drafts: true`: experimental private preparation for push-to-talk; disabled by default and unavailable for subscription transports.
- `listen`: loopback workspace address, default `127.0.0.1:3850`.
- `wiki_url`: wiki upstream served through the agent listener.
- `wiki_dir`: agent wiki space, searched by Iugum Agent and given its opt-in native UI plug.
- `wiki_port`: optional embedded wiki startup on loopback. Requires `wiki_dir`; port 3737 is reserved.
- `data_dir`: private conversation, transcript index, and usage records.
- `instructions_file`: persona and operating instructions.
- `policy_file`: Casbin policy reloaded on each operation.
- `models`: provider pools and model profiles; see `agentcore/README.md`.
- `code_repo`: source repository used for named candidate worktrees.
- `usage_path`: optional shared allowance ledger.
- `claude_accounts`: explicitly allowed Claude account email to configuration-directory mapping.
- `mcp`: optional named Streamable HTTP endpoints, with optional Bearer-token environment references.
- `sources`: explicit transcript roots with platform and optional account labels; see `agentsessions/README.md`.

The native configuration selects its own SQLite memory database under `data_dir`, independent of `IUGUM_DATA`. Facts are scoped by agent name. Model credentials use file or environment references; configuration contains no key values.

For a project coordinator, select the replaceable example role with
`instructions_file: roles/coordinator.md` in the copied agent configuration.
The role covers tool discovery, Beads tracking, wiki use, and authorized session
coordination. It grants no additional permissions and assumes no particular host
or account. The existing `soul.md` remains an alternative role. The native server
reads the selected file at startup and again for each chat request; adding or
changing its configured path requires restarting that native server.

## Conversation and work

Chat messages persist across restarts. Native HTTP chat automatically summarizes older
conversation turns when estimated input reaches 70% of the active model's configured
context window. Recent complete turns and the current request remain verbatim.
The full conversation stays in `context.json` and remains searchable through
`conversation_search`; a derived `context.json.summary.json` cache holds the older
summary, prefix count and digest. Each conversation has its own files. A valid
cache survives restarts. Edited history invalidates the cache and rebuilds it.

Before enabling the updated native HTTP path, add verified
`models.profiles.<profile>.context_window_tokens` for each profile that can be
selected, including budget fallbacks. Use the actual deployed limit: a local
server or alias may expose less context than the upstream model advertises.
Model catalogue discovery supplies IDs only, so discovered models need an explicit
profile with their verified window. Missing capacity produces an actionable error
before inference. No context window is guessed from model names.

Under `models.compaction`, `threshold_percent` defaults to 70 and
`keep_recent_turns` defaults to 4 (including the current turn). The percentage
applies to the selected profile's window on every turn, including model switches.
A smaller destination can retain fewer older turns to fit. Output reservation
also limits the trigger: `max_completion_tokens`, or 20% of the window capped at
4096 when omitted. The default 70% leaves space for output, tool continuations and
estimation error; raising it reduces that margin.

Input uses serialized UTF-8 bytes plus framing allowances as a conservative token
proxy, including instructions, workspace state and tool definitions. This is not a
model tokenizer or an exact utilization display. It can summarize much earlier
than a provider token count would. Summary calls use the selected routed model,
execute no tools, enforce normal model policy and appear in the usage ledger with
`/compact` in their run identifier. Large old messages are summarized in bounded
fragments. Summary quality remains model-dependent; exact evidence stays in the
full conversation.

If instructions, tool schemas or the current request cannot fit, the call stops
with an actionable error instead of truncating them. Summary failure, cancellation
or an oversized summary leaves full history and the previous cache unchanged.
Summaries must finish normally (`finish_reason: stop`); truncated, filtered or
missing completion status is rejected even when nonempty text was returned.
The cache is saved only after a successful actual turn; private drafts never save
it. Each native model continuation checks estimated input plus reserved output;
an oversized current tool result stops explicitly rather than silently losing it.
Transient tool-loop messages are not added to the conversation ledger by this
change. Text compaction does not summarize image-bearing history. Existing transient
image tools continue to work; their model-specific token cost is unknown, so the
continuation guard covers text only and returns a warning for those calls. Existing
image size and count limits still apply.

For an explicit compatibility opt-out, set `models.compaction.disabled: true`.
Native chat then retains the former last-24-messages policy without automatic
summaries or these context guards. Codex subscription chat retains that policy;
the external CLI owns its model window and compaction behavior. Restart the agent
to load changes to these model settings.

Explicit commands work without a model:

```text
status
/commit Finish the agreed draft today
/focus c1
/defer c1
/done c1
/search atomdown
/usage
```

Native tools let the model search evidence, inspect indexed passages, save durable facts, and record explicit commitments. The user controls commitments. Commitments live in an Atomdown Markdown document with stable IDs and due dates. Direct document edits are read on the next status or context request. Beads tasks are a separate, read-only cached source; Iugum Agent does not invent synchronization between them.

Every iugum-controlled model step receives a fresh timestamp and timezone. External Claude resume receives a timestamp at invocation; iugum cannot control every internal request made by Claude.

## Experimental push-to-talk preparation

With `ptt_drafts: true`, `POST /api/draft` accepts a `prepare` operation,
32-character hexadecimal `hold_id`, positive `revision`, and bounded `text`.
It uses the same native context and tool definitions as a normal turn, but
stops after the first model decision. It records model usage, not conversation
messages, and never executes tools. The response contains an opaque `draft_id`.

Release sends final `text`, `hold_id`, and optional `draft_id` to `/api/chat`.
The server adopts only a ready, unexpired preparation whose final text, workspace,
native history, model configuration and tool schemas still match. It rechecks
current policy before model or tool work. Otherwise it runs a normal turn.
`cancel` discards a hold. One draft stays in memory per conversation; ready drafts
expire after 30 seconds. Recent hold identities reject duplicate releases, but
this is not durable idempotency. Do not automatically retry uncertain submissions.

Use a separate native instance and data directory for experiments. The installed
Chat extension does not use this API yet. See `experiments/voice-loop/README.md`
for the isolated browser harness and validation limits.

## Models and allowance

Provider pools have separate monthly soft targets. Iugum Agent's initial OpenAI pool has a $50 target covering all models in that pool. Configured fallback profiles can select a cheaper/local route after the target. Crossing the target without a fallback remains allowed and produces a warning.

The ledger records model, pool, run, token counts, estimated cost, and whether usage/pricing are known. It is not a provider billing reconciliation service. Month boundaries use the configured timezone. `usage_path` can be shared by Iugum Agent and its clones; cross-process locking prevents lost usage records. Alerts default to 75% and 90% of each configured target. Routing decisions and native profile selection are available as tools.

For existing Codex subscription access, opt in:

```yaml
chat_transport: codex-subscription
subscription_model: gpt-5.6-luna
```

The CLI supplies text decisions while Go validates and executes the tool loop. Known CLI tools and integrations are disabled. Each decision is a separate invocation with a fresh timestamp. This adapter requires an installed, authenticated Codex CLI; it is optional and does not replace the native HTTP path. CLI isolation depends on the installed version. Token usage represents subscription consumption; billed cost remains unknown.

## Retrieval and policy

Transcript full-text indexes are rebuildable local snapshots. Initial indexing and ten-minute refreshes are bounded; truncation and source warnings are returned with search results. All-word matching can fall back to explicitly labeled any-word matching. Source scope and negative terms narrow results.

Authorization occurs before source collection and again before search hits and evidence are returned. Project denials filter co-mingled sources. Account labels must be configured; the code does not guess account ownership. Previously returned context cannot be retroactively revoked from a provider.

Wiki search rejects symlinks and searches Markdown within the configured root. Memory uses the existing typed, namespaced store. No embedding service is enabled by this example.

## Extension boundaries and remaining work

`agentcore`, `agentsessions`, and `agentdesk` expose iugum-owned Go types with no provider SDK types in their interfaces. Policy callbacks govern access. Native HTTP currently supports OpenAI-compatible endpoints; other provider protocols need adapters.

Configured MCP servers provide native-loop tools through the official Go SDK; see `agentmcp/README.md`. Managed Claude sessions support queued steering, interrupt, saved-session resume, and durable events. Starting work requires a concrete approval and an explicit account. Existing unmanaged terminal sessions cannot be attached.

Named candidates include code worktrees and authorized memory snapshots. Code and prompt promotions require a human review; see `native-clones.md`. Approval records survive restarts and reject changed evidence. An interrupted action is marked uncertain rather than retried automatically.

Semantic transcript embeddings, automatic promotion, provider billing reconciliation, and automatic Beads/document synchronization remain future extensions. Email, calendar, browser, and Jira integrations are opt-in capabilities, not built-in account access.

### Choosing a subscription model

For `chat_transport: codex-subscription`, configure `subscription_profiles` as a mapping from friendly names to installed CLI model identifiers. The `model_select` tool can choose only these configured names. Selection is policy-checked using `agent/model/codex-subscription/<model>` / `call`, persists in `routing-state.json`, and applies on the next conversation turn. `subscription_model` remains the default when no profile has been selected. `routing_status` and `/usage` report the selected subscription model separately from native API routing. Selecting a profile never changes the transport or starts metered API calls.

`status` includes the latest dated, policy-authorized transcript message across projects. `status <scope>` restricts session metadata to projects, accounts or paths that match the scope (a plain substring, or a name from the `scopes:` map in `iugum.yaml`, see `iugum.example.yaml`); platform names also narrow the lookup. Status reports recorded activity, its source timestamp, and index freshness; it does not assert live completion. Undated messages cannot establish recency. Incomplete index coverage is disclosed. This deterministic lookup does not spend model credits.

For multiple selectable agents and Ollama-backed ACP chat, see [Reusable wiki chat](chat-agents.md).
# Endpoint model discovery

Set `models.pools.<name>.discover_models: true` to populate the native model
selector from an OpenAI-compatible endpoint such as LiteLLM. The native binary
requests `<base_url>/models` at startup, using that pool's backend credential
reference. The editor Settings form exposes the endpoint and discovery switch.
Save settings and restart the native backend to apply changes or refresh the list.
An unavailable catalogue reports a startup error rather than showing stale models.

Explicit profiles keep their names, prices and limits. Additional models receive
stable generated profile identifiers. Existing model and profile policy checks
still govern selection and execution. Discovery does not grant permissions or
assume prices, free usage, tool support, or vision capability. Credentials and
catalogue requests remain on the backend; the extension only displays choices.
