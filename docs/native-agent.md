# Native agent workspace

Run a configured agent as one iugum process:

```sh
iugum agent run --config /path/to/chief/agent.yaml
```

The command runs in the foreground. A supervisor such as launchd owns restart behavior. Existing `wiki`, `beads`, `observe`, `job`, and container-agent commands retain their separate uses.

The wiki is the main application. A bundled plug places continuous chat in SilverBullet’s native right-hand panel and a scoped search row below its title bar. Commitments live in a wiki page. No outer wiki iframe or SilverBullet core patch is used; see `plugs/chief/README.md`.

## Configuration

Copy `examples/chief` into a private agent home. Keep runtime data and credentials out of public repositories. Paths resolve relative to the agent configuration file.

- `name`: agent identity; policy subject is `agent:<name>`.
- `runtime: native`: bounded Go model/tool orchestration.
- `listen`: loopback workspace address, default `127.0.0.1:3850`.
- `wiki_url`: wiki upstream served through the agent listener.
- `wiki_dir`: agent wiki space, searched by Chief and given its opt-in native UI plug.
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

## Conversation and work

Chat messages persist across restarts. Native context includes bounded recent messages and current workspace state. Older conversation text is available through `conversation_search`. This is bounded context retrieval, not automatic semantic consolidation.

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

Native tools let the model search evidence, inspect indexed passages, save durable facts, and record explicit commitments. The user controls commitments. Commitments live in an Atomdown Markdown document with stable IDs and due dates. Direct document edits are read on the next status or context request. Beads tasks are a separate, read-only cached source; Chief does not invent synchronization between them.

Every iugum-controlled model step receives a fresh timestamp and timezone. External Claude resume receives a timestamp at invocation; iugum cannot control every internal request made by Claude.

## Models and allowance

Provider pools have separate monthly soft targets. Chief's initial OpenAI pool has a $50 target covering all models in that pool. Configured fallback profiles can select a cheaper/local route after the target. Crossing the target without a fallback remains allowed and produces a warning.

The ledger records model, pool, run, token counts, estimated cost, and whether usage/pricing are known. It is not a provider billing reconciliation service. Month boundaries use the configured timezone. `usage_path` can be shared by Chief and its clones; cross-process locking prevents lost usage records. Alerts default to 75% and 90% of each configured target. Routing decisions and native profile selection are available as tools.

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

`status` includes the latest dated, policy-authorized transcript message across projects. `status ffai` restricts session metadata to FFAI/FutureFit; platform names also narrow the lookup. Status reports recorded activity, its source timestamp, and index freshness; it does not assert live completion. Undated messages cannot establish recency. Incomplete index coverage is disclosed. This deterministic lookup does not spend model credits.

For multiple selectable agents and Ollama-backed ACP chat, see [Reusable wiki chat](chat-agents.md).
