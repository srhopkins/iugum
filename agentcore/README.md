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
The ledger uses atomic replacement and cross-process file locking; Chief and its clones can share the same usage path. Failed requests may still incur provider charges and are not
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
model either replies or proposes named tool calls with object arguments. Go
validates the batch, checks policy and argument authorization immediately before
each execution, and supplies results as untrusted data on the next step. Malformed
decisions execute nothing. Each CLI invocation has its own usage entry. This does
not remove the underlying CLI-version isolation caveat.

## Shared pool accounting and routing

Parent and clone runtimes can share the same usage ledger by passing the same absolute path to `New`, or configuring `usage_path` when the constructor path is empty. Writes take a cross-process sidecar file lock, reload the latest ledger, append, sync, and atomically replace it. `UsageFresh()` reads current shared records and returns errors; legacy `Usage()` can return its prior cached data on read failure. Model routing uses the error-returning method and does not proceed against a corrupt ledger.

`RoutingStatus(profile)` exposes the requested and selected profiles, all configured pool totals, unknown usage/price counts, and warnings. Monthly totals use the agent's configured timezone. Pool `alert_fractions` defaults to `[0.75, 0.9]`; use an empty list to disable threshold flags. These are routing-state flags, not a notification delivery mechanism. A pool above its soft target tries its configured `fallback_profile`. Fallback cycles and missing profiles produce warnings rather than recursion. No target imposes a hard spending stop. Concurrent calls can cross a soft threshold before their completed usage records arrive; there is no cost reservation system.

Set `zero_cost: true` on an explicitly free local profile to distinguish known zero pricing from missing pricing. Unknown token counts or pricing remain visible in pool status. Subscription allowances and actual provider billing still require separate collectors; this ledger records calls observed by the runtime and cannot infer spending from unrelated processes.
