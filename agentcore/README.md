# Native agent model loop

`agentcore` is a UI-independent OpenAI-compatible model/tool loop. It calls the
existing `contract.Policy` before every model call and every tool execution.
Tools can additionally authorize their argument-dependent resources immediately
before execution. Use this callback for memory scope and transcript access checks.

Create a runtime with `New(config, usagePath, policy)`, then call `Run` with an
actor, optional profile override, run identifier, bounded conversation messages,
and tools. The caller owns context retrieval, conversation persistence, and
argument-level authorization. Use a cancellable context for every run.

Configuration is YAML or JSON. Credentials are environment variables or local
file references; never place key values in configuration. `base_url` is the API
root, for example `https://api.openai.com/v1` or a local compatible server URL.
The transport appends `/chat/completions`. Native Anthropic message format needs an adapter; optional Codex subscription transport is described below.

```yaml
instructions: Be concise. Retrieve evidence before reporting historical facts.
default_profile: mini
timezone: America/Phoenix
max_steps: 8
pools:
  openai:
    provider: openai-compatible
    base_url: https://api.openai.com/v1
    api_key_env: CHIEF_OPENAI_API_KEY
    monthly_target_usd: 50
    # fallback_profile: local
profiles:
  mini:
    pool: openai
    model: gpt-5-mini
    max_completion_tokens: 4096
    # Required for native HTTP automatic compaction: set the actual deployed
    # context_window_tokens value after verifying the provider/server limit.
    # Configure current input/output USD per million tokens to estimate cost.
```

The clock is refreshed before every model request, including tool continuations.
Routing checks the selected pool's current calendar-month estimated spending in the configured timezone
before each request. At its soft target, a configured fallback is preferred;
without a fallback the original profile continues with a warning. This is a
configurable primitive, not a capability or task-complexity classifier.

The private JSON usage ledger records calls, run IDs, pools, models, token counts,
and estimated cost. `usage_known` and `price_known` distinguish missing evidence
from zero cost. Prices must be configured; invoice reconciliation, provider balance
polling, cached-token discounts, and subscription usage require collector adapters.
The ledger uses atomic replacement and cross-process file locking; Iugum Agent and its clones can share the same usage path. Failed requests may still incur provider charges and are not
reconciled by this initial ledger. Step limits bound tool continuations, not total
provider charges. No hard spending cap is implied.

## Optional subscription synthesis

`Runtime.RunSubscription(ctx, request, binary, model)` can use an authenticated
Codex CLI for text-only synthesis without an API key. Enable it explicitly. It
runs in a temporary directory with user configuration ignored, a read-only
sandbox, and known integration/tool features disabled. Supplied messages and a
fresh timestamp go through stdin. The launch is policy-gated; unexpected tool
items stop the process. Detection happens after an event, so it is not a
substitute for native argument-level policy enforcement. Tool availability can
change across CLI versions. This is a synthesis transport, not a delegated
execution interface. The clock is injected at invocation; the wrapper cannot
control internal CLI model requests.

Completed-turn token counts are recorded under `codex-subscription`. Dollar cost
remains unknown and must not be displayed as a zero-dollar bill. Authentication
uses the existing Codex account. No unrelated provider API environment variables
are forwarded. Tests use a fake executable, not an authenticated model call.

`RunSubscriptionTools(ctx, request, binary, model, tools)` adds native Go
orchestration around that synthesis adapter. Each bounded step starts a fresh CLI
invocation with the current timestamp and requires a strict JSON decision. The
model either replies or proposes named tool calls with object arguments. If it
includes prose alongside tool calls, that provisional prose is not published;
the loop waits for a final response after the tool results. Go
validates the batch, checks policy and argument authorization immediately before
each execution, and supplies results as untrusted data on the next step. Malformed
decisions execute nothing. Each CLI invocation has its own usage entry. This does
not remove the underlying CLI-version isolation caveat.

The current human request is captured before the tool loop and supplied explicitly
on every step. Tool results cannot replace it merely because they use a user-role
message. Historical messages and retrieved evidence supply context, not permission
for new actions. The host still checks policy and each tool's argument authority.

The normal test suite uses fake CLIs. To evaluate actual Iugum Agent task-list behavior,
run `IUGUM_CHIEF_EVAL_MODEL=<configured-model> go test . -run '^TestChiefCurrentTaskListEvaluation$' -v`
from the repository root. This opt-in test consumes subscription allowance. It uses
synthetic names, temporary commitment storage, and a fake read tool; it cannot
merge projects or send messages. It checks capture despite a distracting earlier
session question and requires a research attempt after capture.

## Shared pool accounting and routing

Parent and clone runtimes can share the same usage ledger by passing the same absolute path to `New`, or configuring `usage_path` when the constructor path is empty. Writes take a cross-process sidecar file lock, reload the latest ledger, append, sync, and atomically replace it. `UsageFresh()` reads current shared records and returns errors; legacy `Usage()` can return its prior cached data on read failure. Model routing uses the error-returning method and does not proceed against a corrupt ledger.

`RoutingStatus(profile)` exposes the requested and selected profiles, all configured pool totals, unknown usage/price counts, and warnings. Monthly totals use the agent's configured timezone. Pool `alert_fractions` defaults to `[0.75, 0.9]`; use an empty list to disable threshold flags. These are routing-state flags, not a notification delivery mechanism. A pool above its soft target tries its configured `fallback_profile`. Fallback cycles and missing profiles produce warnings rather than recursion. No target imposes a hard spending stop. Concurrent calls can cross a soft threshold before their completed usage records arrive; there is no cost reservation system.

Set `zero_cost: true` on an explicitly free local profile to distinguish known zero pricing from missing pricing. Unknown token counts or pricing remain visible in pool status. Subscription allowances and actual provider billing still require separate collectors; this ledger records calls observed by the runtime and cannot infer spending from unrelated processes.

## Bounded tool images

Tools may return `ToolResult` through optional `ExecuteResult`; existing string
`Execute` tools keep their text behavior. Image results contain one validated PNG,
up to 8 MiB and 16 million pixels. Context permits at most 16 images and 16 MiB
of decoded PNG bytes per run. Overflow fails explicitly; images are never silently
removed. Image bytes are copied and remain ephemeral, outside JSON chat history.

API profiles require `vision: true`. The actual selected profile, including a
budget fallback, must permit images on every continuation. Tool messages keep
text and call IDs. A subsequent untrusted user context carries the PNG data URI,
its provenance and the original human request. Image-bearing requests are capped
at 32 MiB. No remote image URLs are accepted.

Codex subscription attachments require exact model IDs in
`models.subscription_vision_models` (empty by default). Each invocation creates
owned PNG files with mode 0600 in a private temporary directory and supplies fixed
`--image` arguments. Cleanup runs on success, error and cancellation. No image path
comes from the model. This is source-tested with fake executables; actual model
vision support remains an explicit operator configuration choice.

## Private prepared turns

`Prepare` makes the normal first model decision with context and tool schemas,
then returns before tool authorization or execution. The opaque `PreparedTurn`
is held in memory for 30 seconds. `RunPrepared` adopts it once after actual user
submission, with the same actor, profile, messages, configuration and tool schemas.
Model routing and policy remain current; tool authorization still happens in the
normal loop. Changed inputs or an expired/consumed decision return
`ErrStalePrepared`, so callers can fall back to `Run`. Usage is recorded during
preparation. Callers own isolated draft storage and must not persist speculative
conversation messages. Subscription transports are not supported by this API.

## Automatic conversation summaries

Native HTTP callers can use `CompactContext(ctx, request, tools, fullHistory,
previousState)` before `Run` or `Prepare`. `request.Messages` supplies additional
pinned context (for example workspace state) for budget accounting; returned
messages contain summary plus recent history, so the caller prepends pinned
messages afterward. Persist the returned `ContextState` only after a successful
actual turn, alongside the untouched full history. State version, prefix length
and SHA-256 detect stale caches. Errors return the original state.

Set a verified `context_window_tokens` on every selectable `Profile` and fallback.
There is no alias/name-based capacity guess. `Config.Compaction` accepts
`threshold_percent` (default 70, greater than 0 and less than 100),
`keep_recent_turns` (default 4), and `disabled` (default false). Zero numeric values
select defaults. Output headroom is `max_completion_tokens`, or 20% of context
capped at 4096 if omitted. The effective input trigger never exceeds window minus
reserved output. Actual text input uses a deliberately conservative serialized
byte proxy, not an exact model tokenizer. Summaries are bounded, tool-free,
policy-gated calls to the selected routed profile and are metered normally.

`Run` checks estimated input plus output before each HTTP call when the active
profile has a configured window and compaction is enabled. It does not summarize
transient tool outputs: oversized text continuations fail explicitly. Transient
images keep their existing byte/count limits; the guard covers their text only
and adds a warning because image token cost is model-specific and unknown. Generic callers
without configured capacity retain their existing `Run` behavior, while native
conversation preparation requires capacity through `CompactContext`. The native
workspace compatibility opt-out retains its former 24-message policy. See
`docs/native-agent.md` for migration, persistence and failure behavior.
