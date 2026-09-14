# Managed Claude sessions

This package owns a Claude process with bidirectional JSON input/output. It can
start a new session, resume a saved session, queue follow-up instructions while
work runs, interrupt a turn, read persisted events, and reconnect after stopping.
It cannot attach stdin to arbitrary Claude terminal processes started elsewhere.

## Integration

Create `New(privateStateDirectory, policy)` and optionally set `Executable`.
Call `Start(serverLifetimeContext, StartRequest)` with an explicit expected login email,
Claude configuration directory, trusted project directory, and actor. The actor
is checked against `agent/session/<managed-id>` with actions `start`, `send`,
`interrupt`, `stop`, and `read`. Default process lifetime is eight hours; callers
can configure up to 24 hours and reconnect a saved session afterward.

Use `Send` for a fresh or ongoing conversation. It returns immediately after
queueing while the agent is busy. The next message is delivered after a `result`
event, with a fresh timestamp and a new policy check. `Interrupt` is a separate
explicit operation using the official control protocol, with an acknowledgement
timeout. An interrupt acknowledges receipt, not that all tools have already
stopped. Use `Snapshot` or `Events` to observe the resulting turn completion.

`Snapshot` reads current or persisted state; `List` filters inaccessible sessions.
`Events` returns recent input, output, and queued control records. Keep this
private directory out of source control: records contain prompts and results.
`Reconnect` starts the saved Claude session ID again. It does not automatically
replay pending messages after an uncertain disconnect; the event journal retains
them for deliberate recovery. No automatic replay means it cannot silently repeat
consequential work whose delivery is uncertain.

## Permissions and scope

The adapter retains Claude's Manual permission mode and uses
`--permission-prompts none`: operations needing unanswered permission are denied,
not left hanging. Explicit `AllowedTools` rules can preauthorize work agreed by
the user. Never pass blanket allowances without the corresponding authorization.
Claude's normal selected-account/project configuration is loaded, including its
hooks and integrations. Only start it in trusted, authorized projects. Casbin
gates dispatch; it does not intercept every internal Claude tool operation.
Before each start or reconnect, the bridge runs a ten-second-bounded `claude auth
status --json` using the selected configuration directory and project. The logged-in
email must match the configured account exactly (case-insensitive), with verified
subscription authentication and the first-party provider. Missing identity, a
different account, API-key authentication, or another provider blocks launch.
The account registry must use actual login emails, for example
`steve@avantops.com: /absolute/path/to/claude-config`; friendly aliases are refused.
Approval preparation does not query authentication or launch a process. No ambient
Anthropic API keys, OAuth-token overrides, or provider endpoints are passed.
`Manager.VerifyAccount` is an injection seam for tests or a stricter host verifier;
leaving it nil uses the required built-in check.

Timestamps are refreshed on every delivered user message. This bridge cannot
inject time into Claude's own internal model/tool steps. The native `agentcore`
loop provides that stronger guarantee.

## Protocol evidence and validation

Installed `claude --help` exposes streaming JSON input/output, native permission
modes, and resume. The official [Python SDK query implementation](https://github.com/anthropics/claude-agent-sdk-python/blob/main/src/claude_agent_sdk/_internal/query.py)
uses `control_request` with `initialize` and `interrupt` subtypes, followed by a
matching `control_response`. The [streaming input documentation](https://code.claude.com/docs/en/agent-sdk/streaming-vs-single-mode)
describes sequential messages and interruption. The [headless CLI documentation](https://code.claude.com/docs/en/headless)
documents result events, resume, and denied unattended permission prompts.

Integration tests run a fake bidirectional subprocess through real pipes. They
exercise two turns, queueing, timestamps, durable events, interruption
acknowledgement, saved-session reconnect, and policy denial. They do not consume
a Claude account or run project commands.

## Delegated usage

Managed session status/list includes `usage`, the latest Claude result snapshot:
reported input/output/cache token counts, provider-estimated `total_cost_usd`, and
the observation timestamp. Missing fields are null with explicit unknown flags.
These snapshots are replaced, never summed: provider fields can be cumulative
across turns or resumed work. Estimated dollar value is not subscription billing;
`billed_known` remains false. Raw result events retain the original evidence.
Delegated Claude usage is available through session status/events separately from
Chief's `/usage` model-loop ledger; it is not charged into Chief's OpenAI pool.
