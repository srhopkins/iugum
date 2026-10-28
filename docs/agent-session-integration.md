# Shared agent sessions for the code-server chat extension

## Goal

Use Iugum Agent Chat in stock code-server to work with Claude Code, Codex,
OpenCode, Cursor Agent, and the native iugum agent. Keep agent execution and
session ownership in iugum's Go backend so the editor, wiki, ticket interface,
and voice interface can share the same control service.

An iugum conversation is continuous across changes of the thinking provider or
model. It owns the visible history and handoff context; a provider-native session
is one execution connection behind it, not the identity of the whole
conversation. This requires an explicit context-handoff design; it does not
imply that providers can share hidden internal reasoning or native runtime state.

## Conversation and delegation modes

- **Direct mode:** the selected agent works in the conversation itself.
  Delegating to subagents is optional and requires the user's instruction or
  an applicable authorization. Ordinary chat must not require subagents.
- **Command-and-control mode:** the user explicitly selects coordination.
  The thinking provider can dispatch work to other configured agents, follow
  progress, direct them, and bring their results into the conversation.
- Mode and thinking provider are separate choices. Changing provider or model
  must not enable delegation by itself. Voice is an input/output interface and
  must not force command-and-control behavior.
- The user can create new conversations, reopen existing conversations, and
  switch the thinking provider/model while keeping the conversation's context.
- Claude, Codex, OpenCode, Cursor, and the native iugum agent are first-class
  targets. Configured local/LiteLLM agents can also be delegation targets through
  the appropriate native iugum or OpenCode connection; no new provider is implied.

Background work continues when the thinking provider changes. Its identity,
progress, approval requests, and results remain addressable from the same
conversation. Selecting a different session must not terminate another task.

Delegation is a separate module, potentially exposed through MCP from the same
Go program. The core provider contract must also work without that module.
Implementation resumed with Steve's authorization after these clarifications.
The earlier prototype does not yet meet the complete conversation requirements.

The first delivery is the code-server extension and the backend needed by it.
Wiki and ticket integration are future clients, not required deliverables here.
T3 is a source reference for the adapter boundary and lifecycle behavior.
Using T3 itself or copying its complete implementation is not a requirement.

## User requirements

1. Offer all five platforms. Retain the native iugum agent's existing chat,
   history, separate conversations, model controls, and commitments.
2. Browse available external sessions and resume the exact selected native
   conversation. Session loading must not silently create, replace, or fork it.
   History that cannot safely resume must be identified as read-only.
3. Restore the selected session and unsent draft after an editor reload.
   Keep identity and drafts separate by backend, provider, and conversation.
4. Show provider progress and distinguish working, waiting for a decision,
   stop requested, finished, disconnected, and uncertain outcomes.
5. Present provider approval and user-input requests explicitly. Closing a
   dialog cancels; unsupported requests receive a clear denial or error.
6. Stop the selected turn without targeting a different session. A stop request
   is not proof of completion; show the provider's resulting state.
7. Closing or reloading the chat view must not own or terminate a backend task.
   Reconnect to its state rather than automatically sending its prompt again.
8. Retain the existing dictation, push-to-talk, reply playback, editor theme,
   transcript export, and chat layout behavior.
9. Keep provider logins and project files on the machine running the backend.
   Editor-specific actions stay in the editor host adapter.
10. Treat voice as a first-class interface: push-to-talk, speech recognition,
    spoken replies, and typed text. Spoken application controls include model
    and provider changes, explicit approval replies, starting work, and checking
    another task while the current conversation continues. Approvals bind to a
    specific pending request; general conversational agreement is not approval.
11. Keep audio capture, turn detection, transcription, command routing, and
    playback separate. Push-to-talk is primary; adding VAD or conversational
    turn detection later must not require new provider adapters.
12. On a phone, open the same code-server installation and use voice in the
    same extension. A separate Murphy-style mobile app is not required.
    The extension must adapt to phone widths, touch input, and the on-screen
    keyboard while keeping push-to-talk and playback accessible. Validate
    microphone access and playback in a mobile browser; desktop success alone
    does not establish mobile voice support.

## Architecture

The public Go session contract defines provider capabilities, conversation
identity, history, prompt submission, events, approval callbacks, and
interruption. Each external provider adapter translates that contract into its
own supported protocol. Unsupported capabilities remain explicit.

The local session service owns provider processes independently of a webview.
Its API is limited to loopback and authenticated with a private token. Every
operation passes the configured iugum policy gate. Existing native iugum routes
remain supported; this work does not replace its model/tool loop.

The extension handles chat display, session selection, user dialogs, and
editor actions. It calls backend APIs instead of owning external agent
subprocesses. It must continue working with a stock code-server installation.

## Initial provider choices

| Platform | Connection | Rationale |
|---|---|---|
| Codex | Native app-server protocol | Existing extension path; structured thread and turn operations. |
| Claude Code | Existing structured Go-controlled Claude sessions, enhanced from the official SDK reference | Anthropic's official Agent SDK is Python/TypeScript. Keep the Go subprocess/control-protocol boundary and fill its capability gaps. The editor's current ACP path is the baseline, not the target architecture. |
| OpenCode | Structured HTTP API and event stream | Target the native API from Go. Connect only to an explicitly configured, already running authenticated loopback server. No automatic ACP fallback. Live installed-version and LiteLLM credential compatibility remains unverified. |
| Cursor Agent | Official ACP | Add managed session control through the Go interface without introducing a separate SDK runtime. |
| Native iugum | Existing in-process runtime and backend API | Preserve the personal agent and its existing features. |

These are initial implementation choices, not a claim that ACP or an SDK is
universally better. A provider SDK can be introduced behind the same contract
when a required capability justifies it.

The session-service CLI now wires Claude to the Go `agentcontrol.Manager` and
OpenCode to its HTTP/event-stream adapter. Codex app-server and Cursor ACP keep
their existing adapters. The service supports explicit creation and saved-session
resume through each provider's advertised capabilities. Canonical conversation
switching and bounded context handoff use the shared service described below.
Explicit delegation uses the separate optional coordination module documented
below; normal chat does not enable it. Source support does not establish that a
provider is configured or has passed live validation in a particular deployment.

Start the service with `iugum agent sessions serve`. Codex and Cursor use
`--codex-bin` and `--cursor-bin`. Claude stays unavailable until all of these
are supplied: `--claude-actor`, `--claude-account` (the actual subscription login
email), `--claude-config-dir` (absolute), `--claude-model`, and `--claude-policy`
(an existing Casbin CSV file). `--claude-bin` selects its executable. Discovery
reads only that configuration directory. The manager retains its live Claude
auth-status identity and subscription checks; startup never assumes a login.
Incomplete configuration fails before listening or starting provider processes.

Claude session actions pass both the application's policy and the supplied
Claude policy. Both must permit the actor's `agent/session/<id>` action.
Actions include `start`, `send`, `read`, `interrupt`, `stop`, and `approve`.
The explicit `approve` action controls whether a pending tool decision reaches
the client; a client response is still required to allow a tool. The policy CSV
is reloaded for each manager operation. Service HTTP requests separately pass
the application's `agent/sessions<HTTP-path>` `read`/`write` checks (for example
`agent/sessions/api/sessions`). This is session and
approval routing, not interception of every provider-internal tool operation.

OpenCode requires `--opencode-url` (or `IUGUM_OPENCODE_URL`) with a loopback IP
HTTP(S) URL and a server password from `OPENCODE_SERVER_PASSWORD` or
`--opencode-password-file`. Password files must be regular files accessible only
to their owner; a supplied file takes precedence over the environment.
`--opencode-username` defaults to `opencode`. Password values are not accepted as
flags or included in diagnostics or browser responses. Discovery queries that
same authenticated server instead of ambient OpenCode databases. The service
does not install, start, or authenticate an OpenCode server, and does not fall
back to ACP. Configure and log into provider tools separately on the backend
host. Synthetic protocol tests do not prove live provider credential compatibility.

## Reliability requirements

- Keep the provider's native conversation and turn identifiers.
- Reject concurrent writers and session switches that would redirect active work.
- Give prompt commands identities and record delivery state before dispatch.
  Duplicate commands must not repeat the native action.
- Mark interrupted delivery as uncertain. Inspect native history before any
  deliberate resubmission; do not automatically replay prompts after reconnect.
- Route decisions to the original request and validate the offered choices.
- Preserve progress ordering and report a replay gap instead of implying that
  a truncated event buffer is complete history.
- Keep tokens and runtime logs out of the webview and published artifacts.

## Scope boundaries

This delivery supports both new conversations and reopening existing ones.
It must distinguish creating a native provider session, resuming one, and
changing the thinking provider for an iugum-owned conversation. Resuming saved
history must not be described as taking over an arbitrary running terminal or
desktop process. Native iugum's existing conversation behavior remains supported.

tmux, SSH, Docker, and inbox skills remain useful independent tools. They are
not the conversation transport inside these provider adapters. This delivery
does not establish remote access between the Mac and homelab or change provider
authentication, subscriptions, or installed extensions.

## Completion evidence

The delivery is complete when:

- Protocol tests exercise history replay, exact-session resume, progress,
  approval replies, cancellation, provider failure, and disconnect behavior.
- Backend tests prove request authentication, policy checks, duplicate-command
  handling, stale-session rejection, and explicit uncertain recovery.
- Extension tests cover native iugum compatibility and all four external routes.
- A disposable code-server browser test proves selection, history, progress,
  stop, editor reload, saved drafts, and the existing voice controls without
  touching personal sessions or credentials.
- Phone-sized browser tests prove the chat and voice controls remain usable
  with touch input and a reduced visible area. Actual mobile microphone and
  playback verification is reported separately from browser emulation.
- Applicable Go tests and static compilation pass; the extension's tests and
  VSIX packaging pass. Live-provider verification is reported separately from
  synthetic protocol tests.

Implementation progress and follow-up work are tracked in iugum Beads:
`iugum-q56` owns the shared backend; `iugum-5al` owns the editor integration.
The extension repository's `iugumchat-5x4` is a maintained cross-reference.

## Native model setup

The extension header **Settings** button and **Iugum Agent: Configure Native
Models** command open the native listener's model setup form. External providers
keep their existing session model controls. The native backend owns endpoint
pools, profiles and the default profile; the editor keeps no second configuration.

`GET /api/model-setup` returns the saved default, pool endpoints, profile models,
and credential-configured flags. `POST` accepts that editable structure:

```json
{"default_profile":"fast","pools":{"proxy":{"base_url":"http://127.0.0.1:4000/v1"}},"profiles":{"fast":{"pool":"proxy","model":"configured-model"}}}
```

Pool and profile names must be unique, nonempty identifiers. Endpoints must use
HTTP(S) without user information, query strings or fragments. Existing pool and
profile names cannot be removed through this form. Profiles retain their prices,
budget settings and other backend metadata. New pools have no credential until
the administrator configures an environment or private-file reference on the
backend. The form accepts no credential values, environment names or file paths.

Requests pass the agentdesk route policy and `agent/model-setup` `read` or
`write` checks. Writes additionally require each profile's existing `select` and
model `call` permissions. Policies are not edited by setup. Named native agent
connections support the same route under `/api/agents/<id>/model-setup`.

Saving atomically replaces only the editable YAML fields with owner-only file
permissions. It preserves credential references, policy configuration, usage
records and conversation history. Active chat work rejects setup requests with
a busy response. Success reports `restart_required: true`; restart the native
backend to apply the saved settings. The running runtime and selected profile
continue using their prior configuration until restart.

## Canonical conversation handoff

The session service saves Iugum-owned conversations in private `conversations.json`.
Each conversation retains visible messages, exact provider-session links, a selection
revision and durable command outcomes. The editor offers explicit new conversations,
reopening and switching the thinking provider inside the current conversation.
Importing an existing session asks whether to copy visible history; copied messages
carry `imported` provenance. Session links belong to one canonical conversation.

`GET /api/conversations` lists policy-visible conversations. `POST` creates one
with `{title,session,importHistory}`. `GET /api/conversations/<id>` reopens its
saved state without sending a prompt. `POST /<id>/switch` requires
`{revision,session}`. `POST /<id>/chat` requires
`{revision,session,text,commandId}` and returns acceptance before completion.
Poll the conversation's `commands` for `running`, `complete` or `uncertain` and
refresh `messages`. Every command retains its original session reference.

A prompt includes at most 24 recent whole visible messages and 32 KiB of encoded
JSON history. A message that exceeds the remaining bound stops older history
inclusion. Older visible history remains saved and displayed. The history is
marked as untrusted context; old requests and tool results give no current
permission. This transfers neither hidden reasoning nor provider-native runtime
state, credentials, policies or permissions. Native deterministic commitment
commands retain their existing command path without a history wrapper.

A provider switch never cancels existing work. Old links keep their own progress,
approval requests and results. Revision and exact-session checks reject stale
senders. Canonical command IDs are reserved on disk before dispatch. Duplicate
and uncertain commands are rejected, including after restart; reloading never
replays a prompt. Restarted running commands become uncertain and require native
history inspection. The service does not claim to recover an arbitrary externally
running terminal process. HTTP failures and persistence failures report uncertainty.

Add `--native-url http://127.0.0.1:3850 --native-cwd /absolute/runtime/directory`
to connect the existing native listener. The bridge does not start or modify that
listener, change credentials or select a new model automatically. Native catalog,
creation and exact resume use the listener's existing conversation API. Native
approval replies retain the original conversation, request and evidence digest.
Native chat progress is not streamed. Stop cancels only the original active HTTP
request context; it does not stop the whole runtime, and completion remains
uncertain until inspected. Native model options are visible, but selection is
read-only through this bridge because the existing profile control affects the
whole native runtime. Existing native model setup and model controls stay available
through the native listener and editor fallback.

## Optional coordination tools

Editor control is a separate optional module. It does not require coordination
or delegate a conversation. Its design and current limits follow below.

Normal chat does not load coordination tools. Start the existing session service
with `--coordination` only when using the separate coordination module. The module
uses the same control-token authentication, loopback/origin checks and application
policy. It exposes stateless Streamable HTTP MCP at `/api/coordination/mcp` using
the existing Go SDK. Configuring this MCP source is explicit; changing thinking
provider, model or voice input does not configure it or enable delegation.

Each owner canonical conversation starts in direct mode. An authenticated user
explicitly enables coordination with
`POST /api/conversations/<owner-id>/coordination` and `{"enabled":true}`.
MCP exposes no enable tool. Disabling this mode prevents further coordination
operations without cancelling existing workers; their canonical history and
results remain available through the normal conversation API.

The five tools are `coordination_start`, `coordination_status`,
`coordination_follow_up`, `coordination_stop` and `coordination_approvals`.
All require `owner` and a UUID `taskId`. Policy rechecks
`agent/coordination/<owner>/<taskId>` using the operation name, plus the owner
conversation's `coordinate` action and the actual existing target API policies.
The explicit mode change requires `enable` on its coordination policy object.
No policy callback means coordination fails closed.

A start accepts `text`, a UUID `commandId`, and either:

- `conversation`, `revision` and exact `session` for a prepared worker conversation;
- `create: {provider,cwd,title?,model?}` to explicitly create a new provider
  session and a new canonical worker conversation through the existing APIs.

`create` does not change provider logins or choose a model automatically. Provider
capabilities and their existing limitations still apply. The task reservation is
saved before native creation. The returned native and canonical identities are
saved before prompt dispatch. A failed or interrupted create is uncertain; inspect
the native catalog and recorded task before any deliberate new attempt. Retrying
the same task ID never creates or sends again. `coordination.json` contains the
private task identities, links and command IDs; canonical history owns results.

Follow-up requires the original `conversation` and exact `session`, current
`revision`, a new command UUID and `text`. It never redirects to another selected
provider. Stop and approval reporting use the task's original execution session.
Status returns canonical command outcomes, saved results and bounded provider
progress; progress gaps remain explicit. Stop remains a request, subject to each
bridge's capabilities and uncertainty semantics. Reporting approval requests
never approves them. The human responds through the existing exact-session
approval UI/API; general agreement and old context grant no permission.

The module is a target-specific control surface. It does not schedule agents,
interpret subscription balances, transfer permissions, retry uncertain work or
install a second orchestration runtime. There is no automatic result injection
into the owner's prompt; the coordinating agent explicitly reads task status.

## Optional editor host tools

`--host-tools` adds authenticated Streamable HTTP MCP at `/api/host/mcp` to
the existing session service. It is off by default and independent of coordination.
Configure this source explicitly in the native runtime's existing `mcp` list or
an external provider's supported MCP configuration. The native client supports
an `auth_env` reference; no credential or provider configuration is changed
automatically. The endpoint uses the session service bearer token.

`editor_windows` lists only live registered extension-host windows and their
capabilities. `editor_call` queues one typed action against an exact `instance`
UUID with a fresh `commandId` UUID. `editor_status` reads its original receipt.
Available operations are `inspect`, `open_file`, `focus` and `open_beads` when
the registered adapter advertises them. `open_file` accepts a workspace-relative
`path`, a one-based `line`, an explicit `group` from 1 through 9, and optional
`column`, `endLine` and `endColumn`. The extension must validate real paths and
positions against its actual workspace. `focus` accepts only `editor`, `sidebar`,
`panel`, `terminal` or `chat`. There is no general command execution operation.

The extension registers a fresh window identity, receives a private lease and
polls for commands. A lease expires after one minute without polling. Each action
expires after 30 seconds and is delivered at most once. A missing result,
re-registration or service restart leaves an explicit uncertain receipt;
mutations are never replayed. The pending queue is bounded to 32 commands per
window, arguments to 16 KiB and results to 64 KiB. Command receipts persist in the service's private
`host-commands.json`; live registrations and leases do not survive restart.
Receipt history is not automatically pruned, so command UUIDs cannot be reused.

All host routes retain session authentication and the actual application policy
gate. Additional `agent/host/transport`, `agent/host/register`, `poll`, `result`,
`windows`, `status` and operation checks apply, including immediately before
delivery. Extension workspace trust and its typed action checks remain required.

The target is the code-server window opened directly in the user's Mac or phone
browser. VS Code APIs can control that window without a container browser. This
identity does not claim access to every VS Code window, a verified browser tab,
the workbench DOM or cross-origin Simple Browser content. Mouse capability is
explicitly unavailable until a consented connector binds the actual client.
Container Chrome's CDP/Playwriter connection is a different browser and cannot
serve as a substitute. A browser-side connector is separate tracked work;
normal chat and API editor actions do not depend on it.

The optional own-webview pairing diagnostic uses two additional authenticated
POST routes under host tools: `/api/host/pairing/challenge` and
`/api/host/pairing/attest`. The actual application gate checks their normal
session transport routes, `agent/host/transport` write, and respectively
`agent/host/pairing_challenge` create or `agent/host/pairing_attest` write.
Each challenge binds the original live editor instance and its private lease
to exact client/document UUIDs. It expires after 30 seconds, consumes once,
never persists across service restart, and gives no browser-control authority.
The pending challenge map is bounded to 32 entries. A new lease cannot consume
an earlier window's challenge. Receipts explicitly report
`own-webview-private-channel` and `mouse: false`.

The extension command is disabled by default through
`iugumChat.clientPairingProbe`. It passes only a public nonce binding through
its own `acquireVsCodeApi` channel; the service token and lease remain in the
extension host. This does not establish outer-tab or Mac/phone companion
pairing. Source review precedes disposable candidate installation; real
code-server content-security-policy proof is separate from the generic
sandbox-frame fixture.

## Conditional canonical controls

The session service supports authenticated conditional controls for clients that
need to prepare an exact model or request decision before confirming it. These
are source APIs; they do not enable spoken decisions in the editor by themselves.
Existing manual controls and the installed first-five spoken controls retain
their existing routes. The extension proposal UI is separate work.

`GET /api/control-incarnation` returns `{ "incarnation": "..." }` through the
existing bearer-authenticated, loopback and policy checks. The service generates
a fresh random incarnation on every start. The trusted extension host obtains
it; a transcript must never supply a replacement incarnation.

`POST /api/conversations/{id}/control` accepts a typed preparation:

```json
{
  "phase": "prepare",
  "incarnation": "FROM_AUTHENTICATED_DISCOVERY",
  "window": "EXACT_EXTENSION_WINDOW_GENERATION",
  "captureEpoch": 0,
  "actionId": "FRESH_UUID",
  "revision": 1,
  "session": {"provider": "Codex", "sessionId": "EXACT_NATIVE_ID", "cwd": "/workspace", "title": "Displayed title"},
  "kind": "model",
  "model": "ADVERTISED_MODEL_ID"
}
```

Preparation never selects a model or answers a request. It returns that complete
binding with server-owned `modelsDigest` (for model controls), `expiresAt` and
`digest`. Echo every returned field unchanged, replacing only `phase` with
`commit`. Clients do not reproduce the digest algorithm or set the expiry.
The server freezes the entire action, window/capture generation, native target,
canonical ID/revision, model catalogue/current selection or exact request reply,
and a maximum 30-second lifetime. The host remains responsible for verifying
that the UI window/capture epoch and explicit confirmation are still current.

For a request decision, use `kind: "request"`, omit model fields and provide
`requestId`, `requestDigest` and the exact method-specific `result` object.
The pending request must have a known original canonical origin matching this
conversation, its originating canonical command must still be running, and its
native session must match the selected canonical target.
The existing approvals projection now includes `origin.conversation` and
`origin.commandId` only when the request arose during an accepted canonical
command. Background or legacy requests with unknown origin remain usable through
the existing exact-session manual dialog; conditional controls never guess.
The request digest remains its method/parameters content version; no backend
request version is invented.

Conditional decisions accept existing once-only permission choices, unchanged
original tool input, and one listed nonsecret single-choice question answer.
Plans, free-form or secret answers, multiple questions/options, persistent grants,
unknown methods and unoffered choices remain in the manual dialog. Existing
manual route validation is unchanged. The same native request lock ensures a
competing manual and conditional decision deliver at most one answer.

Commit validates incarnation, immutable binding, expiry, current canonical
revision/selected native ref and policy before reserving the exact operation.
Policy, proposal expiry and request cancellation are rechecked at the atomic
reservation boundary after waiting for all target locks; an earlier handler
check alone does not authorize a delayed mutation.
Model selection also checks idle state, capabilities and an unchanged advertised
catalogue/current selection. Conversation/session locks are released before
provider catalogue or selection calls. A canonical switch after reservation can
proceed; it cannot change the accepted operation's native target.

Results contain `actionId`, `status` and a generic `error`. Status is `running`,
`completed`, `rejected` or `uncertain`. A duplicate commit returns the recorded
status without repeating provider work. A provider delivery/persistence error
after model selection begins is uncertain; inspect native state rather than
retrying. An action ID cannot be prepared again or reused with another payload,
window or target. The bounded process-local ledger retains up to 4096 identities
for the service incarnation and rejects new preparation at capacity.

Restart rotates the incarnation and invalidates every previous capture,
proposal and operation. A late old commit rejects even when the old receipt was
lost and the canonical revision survived. No old capture can be upgraded to the
new incarnation. Clients must invalidate pending proposals on reconnect and
never replay uncertain operations. These APIs do not send audio, contact speech
services, establish provider credentials or prove physical-device behavior.

### Conditional existing linked-session selection

A spoken session change uses the same prepared control endpoint with
`kind: "switch"` and `targetSession`. `session` remains the captured **source**
native reference, and `revision` remains that source canonical revision. For
example, preparation contains these action fields alongside the existing
incarnation, window, capture epoch and action identity:

```json
{
  "phase": "prepare",
  "kind": "switch",
  "revision": 3,
  "session": {"provider": "Codex", "sessionId": "SOURCE_NATIVE_ID", "cwd": "/workspace"},
  "targetSession": {"provider": "Claude", "sessionId": "LINKED_TARGET_NATIVE_ID", "cwd": "/workspace"}
}
```

The target must already occur in this canonical conversation's saved `links`,
matching provider, native ID and working directory. Preparation returns the
server's authoritative saved target reference and title. Both source and target
native owners must already be present and ready in the service; the conditional
route never calls provider `Open`, session selection/resume, import, creation or
linking. A missing owner requires the existing manual session UI first. Owner
presence does not prove live-provider compatibility or connection health.

Echo the complete prepared response, changing only `phase` to `commit`. The
existing service incarnation, one-time digest, window/capture generation,
30-second expiry and source identity/revision protections apply. A delayed old
commit after restart fails even if the canonical revision survived. Bare legacy
`/switch` is not a substitute for this spoken confirmation contract.

At atomic reservation, both exact source and target native references require
`select` permission; the canonical and route permissions remain the existing
`write` actions. Current policies, context and expiry are checked after all target
locks, immediately before changing the saved selection. Both owners can be busy:
selection changes neither their work nor their lifecycle, and sends no interrupt,
close or provider request. A successful commit saves the selected reference and
increments canonical revision exactly once, retaining links, history and owners.
The client can reload canonical state after `completed`.

Duplicate commits return the original receipt without another save. Persistence
failure reports `uncertain`, preserves the receipt, and cannot be retried by
resending the old action. Inspect state before preparing another action. The
existing manual `/switch` route is unchanged, including its explicit resume/link
behavior; this narrower conditional route adds no session/link capability.

### Optional browser authority source integration

`agent sessions serve --host-tools --browser-grants` explicitly opts into the
new paired browser protocol source. It is disabled by default. Startup requires
the actual immutable application Casbin gate, its explicit actor, the existing
outer application check and host registry; `agent/browser` `serve` must pass.
Claude's separately live-reloaded tool policy is not browser authority. Policy
replacement/revoke uses the authority's serialized actual-gate writer boundary;
editing a static policy file does not mutate an already running gate.

Authenticated private extension requests use `{instance,lease,request}` under
`/api/host/browser/bootstrap/{offer,commit,reveal,confirm,companion-confirm,status,cancel}`.
The private lease and service token never enter a page. Browser records carry a
nonsecret window-generation digest. Exact registered host lease is held stable
through reservation, and actual Casbin checks exact
`agent/browser/<pair>/<observe|click|scroll>` with action `write` before durable
single-use reservation/signing. Transport remains separately gated by existing
session/host policy. Typed `/api/host/browser/{grant,observation,result}` carries
signed v2 records and bounded PNG bytes, never arbitrary CDP/scripts/commands.

This source is not deployed. The trusted native editor confirmation, companion
background key/tab provenance, coherent document-safe Chrome driver and actual
Mac end-to-end proof have separate source/review/physical validation requirements.
Phone input remains unsupported: no validated mobile Safari/Chrome companion or
outer-document bridge exists. No service flag alone establishes mouse capability.
The previously reviewed frozen validation bundle remains unchanged.

Browser receipt durability uses an exclusive lifetime owner plus a separately
fsynced health marker set UNCLEAN before any command is accepted. Graceful Close
marks clean only if every receipt write is known healthy and records the exact
store digest. Crash, unknown write outcome, invalid/missing health for an existing
store, or digest mismatch quarantines that exact browser store on restart. It
never resets history, silently changes namespace or falls back to permissive
mode. Normal native sessions without the browser flag remain a separate option.

There is no automatic recovery/reset command. Unblocking requires an explicitly
audited recovery that reconciles every potentially accepted client command UUID
with a durable consumed receipt and establishes no outstanding prior requests,
then restores a matching clean health record under exclusive ownership. If lost
UUIDs cannot be reconstructed, the old store stays quarantined. A fresh filename
is not an automatic substitute for the lost at-most-once history.

Signed client capture/result/companion-confirmation timestamps tolerate at most
100ms in either clock direction. Capture must lie within the original server
observe-grant interval expanded only for this clock validation tolerance; this
never extends execution authority. Terminal completion may follow dispatch.
Stored observation freshness conservatively subtracts100ms from the signed
client expiry, while the server pairing expiry remains an absolute upper bound.

The optional browser MCP endpoint is `/api/host/browser/mcp`, with only
`browser_windows`, `browser_call`, and `browser_status`. No provider profile is
changed automatically. Agent calls select an exact confirmed pair and typed
operation. Queueing reserves a UUID without issuing a grant; private `poll`
returns only the five fixed command fields, once. Private `grant` accepts only
that exact dispatched command and rechecks current policy and window. Queue
expiry, cancellation after delivery, restart, or uncertain storage never replay
an action. Dispatcher storage has the same exclusive-owner and unclean-store
quarantine rules as the authority ledger; there is no automatic history reset.

Private `clock-probe` consumes one backend-owned observe UUID per pair and echoes
a fresh privileged-background nonce in a separate signed clock domain. Its
internal grant signature is never returned. The proof cannot authorize capture
or input. The companion still checks its measured clock interval, local consent,
calibration, current document and monotonic execution deadline.

`browser_windows` distinguishes `awaiting-first-observation` from
`fresh-observation`. Pairing permits an initial observation request; it does not
prove driver readiness. Only a completed signed observation enables input grants.
An input grant immediately consumes the prior proof. `browser_status` returns a
verified PNG as MCP image content and bounded text metadata, with no duplicate
base64 in text or structured content. Its `fresh` field distinguishes an exact
current proof from retained historical evidence. Raster cache storage is bounded
and transient; durable UUID tombstones contain no screenshots.

Browser observation/result acceptance uses the separate signed
`iugum/browser/acceptance/2` domain. Those private routes return only an exact
`{record,signature}` envelope after actual core and dispatcher acceptance;
unsigned `accepted` booleans cannot prove completion. The 22 scalar fields bind
the original grant/identity, submitted record digest, original and newly accepted
observation IDs, lifecycle and a server expiry of at most one second. Observation
ACK expiry also respects the conservative server proof expiry (signed capture
expiry minus 100 ms). Only the existing sealed pair's pinned server key verifies
these receipts. Capture acceptance alone never makes an observation ready.

The dispatcher serializes acceptance and its one signature under its transition
lock. A terminal receipt is persisted by both the authority and dispatcher before
signing. Signing, storage, cancellation or expiry failure returns no envelope;
the consumed action remains consumed. A lost response cannot be retried to obtain
another receipt. Missing or invalid acceptance means local uncertainty and detach,
while the agent can inspect the original server receipt without replay. Raster
cache bytes remain bounded and transient. These new source changes require their
own review and package; they do not alter the currently installed 0.1.13 artifact.

## Editor session content search

The editor conversation menu queries all native listener conversation stores,
local indexed provider transcripts, configured OpenCode HTTP history and canonical
Iugum histories independently of its current provider. Native `/conversations?q=`
searches titles and saved messages, including user/assistant `context.json`
records. `/conversations?transcript=<id>` reads those context records for a
read-only result. Tool records are excluded from native context search.

Authenticated session-service `GET /api/session-search?q=` and
`GET /api/session-transcript?id=` expose read-only indexed history. Source policy
is checked before indexing; transcript and project policy is checked before each
result and transcript read. Stable result identities include provider, account,
native session and source, with database row fragments normalized to the parent
source. Catalogue titles are merged only for unambiguous provider/session
identities. Cursor and OpenCode database titles are read from their metadata.
Configured remote OpenCode history uses its existing HTTP credentials and GET
requests, without opening a runtime owner. Coverage warnings report missing
sources, failed discovery and indexing limits.

Native workspace results reopen their exact listener and conversation. External,
canonical and context-only results open read-only editor documents; they cannot
implicitly select, create, import or resume a provider session. Explicit Session
Hub selection retains its separate execution behavior. Typing cancels older host
queries, and stale menu or selection replies cannot reopen dismissed results or
switch the currently selected agent.

Search policy belongs to the session service domain. Source checks use
`agent/sessions/transcript:<platform>:<account>` (`search` before indexing,
`search` and `read` before disclosure). Project checks use
`agent/sessions/project:<cwd>` (`read`). Exact native execution identity checks
also enforce `agent/sessions/<Provider>/<sessionID>` (`read`, using the displayed
provider spelling). Existing `agent/sessions*` grants cover this service feature;
explicit source, project and session denies override that broad grant. There is
no fallback to another permission namespace after denial. These checks do not
change the separate native agent search policy, which uses bare `transcript:`
and `project:` resources. Configured Claude sources retain their explicit account
in the index and checks. No policy file changes are required or performed.


### FFAI worker discovery

The existing authenticated host MCP endpoint also exposes `ffai_workers` when
`--host-tools` is enabled. Native Iugum and OpenCode use the same explicitly
configured `/api/host/mcp` source. Reconnect their MCP clients after upgrading
the session service so discovery includes the new tool.

This parameter-free tool executes only `docker exec` against the exact
`ffai-agent` container on the session service Docker socket, as user `abc`.
It invokes Bash with `--noprofile --norc` and clears `BASH_ENV` and `ENV`.
It reads hostname, user, and tmux pane metadata; it never captures pane content,
reads transcripts, starts workers, or sends instructions. Limits are eight
seconds, 16 KiB per output stream, and 64 panes. Errors return bounded status
messages without echoing Docker diagnostics.

Results identify unmanaged tmux sessions and panes, not IDs accepted by
`claude_send` or coordination tools. A `claude_process` field reports only the
pane's current command. It does not establish idle state or permission to
steer. Any handoff must use the separately authorized exact session route,
check its current state and owner, then verify receipt and returned result.
`empty` means no abc tmux server or panes; `unavailable` means the named
container/user could not be verified; `tmux_unavailable` means identity passed
but its tmux inventory could not be read. There is no root-socket fallback.
