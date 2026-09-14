# Agent workspace

`agentdesk` supplies a local workspace with a wiki on the left and one durable conversation on the right. It is an outer workspace with a wiki iframe, not a native SilverBullet plug. The wiki must permit embedding from the workspace origin.

Construct a server with `New(Config{...})`, then call `Run(ctx)` or mount `Handler()`. `DataDir` is required. `Listen` defaults to `127.0.0.1:3850` and must use loopback. `WikiURL` is optional. `Chat`, `Search`, and `Check` connect the workspace to the agent runtime and policy gate. An omitted policy callback permits local requests; production composition should supply the policy gate.

The workspace supports these operations without a model:

- `status`: summarize the first open commitment and count the remaining work.
- `/commit title`: save an explicit commitment.
- `/done c1`: complete the commitment identified by `c1`.
- `/focus c1`: bring an open commitment to the front and remove its deferred flag.
- `/defer c1`: retain a commitment for later, excluding it from immediate status.

Chat and commitments persist in `workspace.json` with private file permissions and atomic replacement. This is one local process's store; do not run multiple processes against the same data directory. External Beads and Atomdown synchronization are not implemented here.

The chat callback receives the current user text. The runtime owns model context preparation, including fresh current timestamps and memory retrieval. The workspace timestamps each displayed message separately.

## HTTP endpoints

| Method | Path | Purpose |
|---|---|---|
| GET | `/` | Workspace |
| GET | `/api/status` | Agent name, current timestamp, wiki URL, commitments, chat callback availability |
| GET | `/api/messages` | Durable conversation |
| POST | `/api/chat` | Submit `{"text":"status"}` or a message |
| GET | `/api/search?q=...&scope=...` | Delegate scoped search |

Every request calls `Check(ctx, "agentdesk" + path, "read" or "write")` when configured. Requests require a loopback Host and reject foreign Origin headers. There is no public hosting mode or remote authentication in this package.

`Context()` gives the runtime a timestamped, bounded snapshot of commitments and recent messages. `AddCommitment(ctx, title, explicit)` supports authorized runtime integration; callers must establish an explicit user commitment before passing `true`. Search results show short evidence cards with expandable source paths and full passages.

## Working documents and approvals

Set `CommitmentsPath` to a wiki Markdown working document. The default is `DataDir/Commitments.md`. Startup migrates legacy JSON commitments, preserving their IDs, then removes the duplicate JSON records. `/due c1 YYYY-MM-DD` sets an explicit date; `/due c1 none` removes it. The document uses Atomdown identity and visible fields, described in `agentdocuments`.

`RequestApproval(ctx, ApprovalRequest)` creates a durable review item containing an action, payload, evidence, and a digest. `GET /api/approvals` lists review items. The workspace submits `POST /api/approvals/{id}` with a decision and the reviewed digest. Approval rechecks evidence files and calls `ApprovalExecute` at most once, with execution recorded before the callback. Results persist as complete or failed. An interrupted execution remains executing/uncertain; it is never automatically repeated. Callers must inspect the action's real outcome before requesting a new attempt. A rejected or stale item never invokes the callback.
